/**
 * Public decoder API. Combines streaming frame split, escape decoding,
 * header parsing, and TOR dispatch into a single `push`-driven object.
 *
 * Intended consumer: a WebSocket binary message handler.
 *
 * ```ts
 * const dec = createDecoder();
 * ws.binaryType = 'arraybuffer';
 * ws.onmessage = (ev) => {
 *   const results = dec.push(new Uint8Array(ev.data as ArrayBuffer));
 *   for (const r of results) handle(r);
 * };
 * ws.onclose = () => dec.reset();
 * ```
 */
import { unescape } from './escape.js';
import { createFrameStream, type FrameStream } from './frame.js';
import { parseHeader } from './header.js';
import { TOR, decodePassing, decodeStatus, type ParseResult } from './records.js';

export interface Decoder {
  /**
   * Append a wire-format chunk and return all newly completed frame
   * parse results. May return an empty array if the chunk only carried
   * a partial frame.
   */
  push(chunk: Uint8Array): ParseResult[];
  /**
   * Drop any in-flight buffer state. Call on WebSocket reconnect to
   * avoid stitching pre-disconnect bytes onto the new stream.
   */
  reset(): void;
}

export interface DecoderOptions {
  /**
   * Override the underlying frame stream. Mainly useful for tests; the
   * default `createFrameStream()` is what production code wants.
   */
  readonly frameStream?: FrameStream;
}

export function createDecoder(opts?: DecoderOptions): Decoder {
  return new DecoderImpl(opts?.frameStream ?? createFrameStream());
}

class DecoderImpl implements Decoder {
  constructor(private readonly stream: FrameStream) {}

  push(chunk: Uint8Array): ParseResult[] {
    const frames = this.stream.push(chunk);
    const results: ParseResult[] = [];
    for (const f of frames) {
      results.push(parseFrame(f));
    }
    return results;
  }

  reset(): void {
    this.stream.reset();
  }
}

function parseFrame(rawFrame: Uint8Array): ParseResult {
  const { bytes: unesc, error: escapeError } = unescape(rawFrame);
  if (escapeError) {
    return { kind: 'malformed', reason: 'invalid-escape', raw: rawFrame };
  }
  const header = parseHeader(unesc);
  if (!header) {
    return { kind: 'malformed', reason: 'truncated', raw: rawFrame };
  }
  // Sanity: spec §9 #1 says Frame Length is the unescaped total length
  // including SOR/EOR. If they disagree, the frame is corrupt.
  if (header.frameLength !== unesc.length) {
    return { kind: 'malformed', reason: 'frame-length-mismatch', raw: rawFrame };
  }
  switch (header.tor) {
    case TOR.PASSING:
      return decodePassing(unesc, rawFrame);
    case TOR.STATUS:
      return decodeStatus(unesc);
    default:
      return { kind: 'unknown', tor: header.tor, raw: rawFrame };
  }
}
