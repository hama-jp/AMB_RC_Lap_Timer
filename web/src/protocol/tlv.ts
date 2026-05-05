/**
 * TLV (Type-Length-Value) traversal for the body of an unescaped frame.
 * (docs/protocol-p3.md §5)
 *
 * Layout per TLV:
 *   off 0: Field ID (uint8)
 *   off 1: Field Length (uint8) — read as a raw byte; values >= 0x10 are
 *          common (e.g. 4-byte values), beware the reference Python
 *          implementation's hex-string-as-decimal bug.
 *   off 2..2+len: Value
 *
 * Values are little-endian when interpreted as integers; this iterator
 * does NOT decode them — that is the responsibility of `records.ts` /
 * `decoder.ts` which know what kind of integer each field is.
 */
import { HEADER_SIZE } from './header.js';

export interface TLV {
  /** Field ID byte. */
  readonly id: number;
  /** Field Length byte (uint8 — read directly, never as decimal). */
  readonly length: number;
  /**
   * Value bytes. The slice points into the unescaped frame buffer; do not
   * mutate it unless you mean to.
   */
  readonly value: Uint8Array;
}

/**
 * Iterate every TLV in the body of an unescaped frame.
 *
 * The body is the bytes between the fixed 10-byte header and the trailing
 * EOR. Iteration stops silently on a truncated TLV (declared length runs
 * past the end of the body), matching the gateway's lenient behaviour.
 */
export function* iterateTLVs(unesc: Uint8Array): Generator<TLV, void, void> {
  if (unesc.length < HEADER_SIZE + 1) {
    return;
  }
  const body = unesc.subarray(HEADER_SIZE, unesc.length - 1);
  let i = 0;
  while (i < body.length) {
    if (i + 2 > body.length) {
      return;
    }
    const id = body[i]!;
    const length = body[i + 1]!;
    const end = i + 2 + length;
    if (end > body.length) {
      return;
    }
    yield { id, length, value: body.subarray(i + 2, end) };
    i = end;
  }
}
