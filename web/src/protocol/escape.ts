/**
 * Body byte escaping for AMB P3 frames (docs/protocol-p3.md §4).
 *
 * On the wire, the body of a frame (everything between SOR and EOR) MAY NOT
 * contain raw 0x8D / 0x8E / 0x8F bytes — those would be confused with the
 * escape marker, SOR, and EOR respectively. They are encoded as two-byte
 * sequences `0x8D <byte + 0x20>`. This module decodes that.
 */

/** Wire byte: Start Of Record. */
export const SOR = 0x8e;
/** Wire byte: End Of Record. */
export const EOR = 0x8f;
/** Wire byte: escape marker. The next byte is the encoded form of a special. */
export const ESC = 0x8d;
/** Bias added/subtracted when encoding/decoding an escaped byte. */
export const ESC_BIAS = 0x20;

export interface UnescapeError {
  readonly reason: 'invalid-escape';
  /** Position of the dangling 0x8D within the input frame. */
  readonly offset: number;
}

export interface UnescapeResult {
  /**
   * Decoded frame bracketed by SOR at index 0 and EOR at the last index.
   * The body bytes between them have been decoded.
   */
  readonly bytes: Uint8Array;
  /**
   * If the body ended with a dangling 0x8D escape marker (no following byte
   * before EOR), `error` is set and the dangling 0x8D is dropped from `bytes`.
   * The caller should treat the frame as malformed.
   */
  readonly error?: UnescapeError;
}

/**
 * Unescape the body of a P3 frame.
 *
 * @param frame  Wire-format frame: SOR (0x8E) at index 0, EOR (0x8F) at the
 *               last index, escape sequences in between.
 * @returns      Decoded frame and an optional error for truncated escapes.
 */
export function unescape(frame: Uint8Array): UnescapeResult {
  if (frame.length < 2) {
    return { bytes: frame.slice() };
  }
  const out = new Uint8Array(frame.length);
  let outLen = 0;
  out[outLen++] = frame[0]!; // SOR

  const bodyEnd = frame.length - 1; // index of EOR
  let escapeNext = false;
  let danglingOffset: number | undefined;

  for (let i = 1; i < bodyEnd; i++) {
    const b = frame[i]!;
    if (escapeNext) {
      out[outLen++] = b - ESC_BIAS;
      escapeNext = false;
      continue;
    }
    if (b === ESC) {
      escapeNext = true;
      continue;
    }
    out[outLen++] = b;
  }

  if (escapeNext) {
    danglingOffset = bodyEnd - 1;
  }

  out[outLen++] = frame[bodyEnd]!; // EOR

  const result: { bytes: Uint8Array; error?: UnescapeError } = {
    bytes: out.subarray(0, outLen),
  };
  if (danglingOffset !== undefined) {
    result.error = { reason: 'invalid-escape', offset: danglingOffset };
  }
  return result;
}
