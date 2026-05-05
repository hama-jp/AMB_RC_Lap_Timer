/**
 * Streaming frame splitter for the AMB P3 wire protocol.
 *
 * The gateway forwards raw TCP bytes via a binary WebSocket. A single
 * `WebSocket.onmessage` chunk is not aligned with frame boundaries: a
 * chunk may contain multiple complete frames, a partial frame at the
 * end, garbage before the first SOR, etc. (docs/protocol-p3.md §2.1).
 *
 * `FrameStream` accumulates chunks in an internal buffer and emits each
 * completed frame (`SOR ... EOR`, escape sequences kept intact) as soon
 * as it sees the trailing EOR. Bytes outside any frame are discarded.
 *
 * Production code should not import `node:*` — this module is plain ES.
 */
import { ESC, EOR, SOR } from './escape.js';

export interface FrameStream {
  /**
   * Append a chunk and return all newly completed frames.
   * Each returned slice is bracketed by SOR (index 0) and EOR (last index)
   * and still contains its escape sequences (call `unescape` to decode).
   */
  push(chunk: Uint8Array): Uint8Array[];
  /** Drop any in-flight buffer state. Call on WS reconnect. */
  reset(): void;
}

export function createFrameStream(): FrameStream {
  return new FrameStreamImpl();
}

class FrameStreamImpl implements FrameStream {
  // Internal buffer of bytes that haven't yet been emitted. Reallocated
  // on every push; for the modest WebSocket chunk sizes we expect from a
  // P3 decoder this is simpler and fast enough.
  private buf = new Uint8Array(0);

  push(chunk: Uint8Array): Uint8Array[] {
    if (chunk.length === 0) {
      return [];
    }
    // Concat: existing buffer + new chunk.
    const combined = new Uint8Array(this.buf.length + chunk.length);
    combined.set(this.buf, 0);
    combined.set(chunk, this.buf.length);
    this.buf = combined;

    const frames: Uint8Array[] = [];
    let scanFrom = 0;

    while (scanFrom < this.buf.length) {
      // Skip bytes before the next SOR.
      let start = scanFrom;
      while (start < this.buf.length && this.buf[start] !== SOR) {
        start++;
      }
      if (start >= this.buf.length) {
        scanFrom = this.buf.length; // entire remainder is garbage; drop it
        break;
      }

      // Walk forward looking for EOR, honoring escape sequences so that an
      // escaped 0x8F (encoded as 0x8D 0xAF) does not falsely terminate the frame.
      let j = start + 1;
      let endIdx = -1;
      while (j < this.buf.length) {
        const b = this.buf[j]!;
        if (b === ESC) {
          if (j + 1 >= this.buf.length) {
            // Truncated escape at buffer end — wait for more bytes.
            j = -1;
            break;
          }
          j += 2;
          continue;
        }
        if (b === EOR) {
          endIdx = j;
          break;
        }
        j++;
      }

      if (endIdx === -1) {
        // Incomplete frame; keep the SOR-and-onward bytes for the next push.
        scanFrom = start;
        break;
      }

      frames.push(this.buf.slice(start, endIdx + 1));
      scanFrom = endIdx + 1;
    }

    // Drop everything we've consumed (including any garbage skipped).
    this.buf = scanFrom >= this.buf.length ? new Uint8Array(0) : this.buf.slice(scanFrom);
    return frames;
  }

  reset(): void {
    this.buf = new Uint8Array(0);
  }
}
