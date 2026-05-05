/**
 * Synthetic frame builders for protocol unit tests.
 *
 * The intent is to mirror the Go test approach in
 * `gateway/internal/p3frame/p3frame_test.go`: build small in-memory
 * frames and round-trip them through the decoder. No fixture files,
 * no I/O.
 */
import { ESC, ESC_BIAS, EOR, SOR } from '../../src/protocol/escape.js';
import { HEADER_SIZE } from '../../src/protocol/header.js';

/**
 * Encode the body of an unescaped frame (everything between SOR and EOR)
 * using the wire escape rules. SOR/EOR themselves are preserved verbatim.
 * Mirrors the Go implementation in `p3frame.Escape`.
 */
export function escapeFrame(unesc: Uint8Array): Uint8Array {
  if (unesc.length < 2) return unesc.slice();
  const out: number[] = [unesc[0]!];
  for (let i = 1; i < unesc.length - 1; i++) {
    const b = unesc[i]!;
    if (b === ESC || b === SOR || b === EOR) {
      out.push(ESC, b + ESC_BIAS);
    } else {
      out.push(b);
    }
  }
  out.push(unesc[unesc.length - 1]!);
  return new Uint8Array(out);
}

export interface UnescapedFrameOptions {
  version?: number;
  crc?: number;
  flags?: number;
  tor: number;
  body: Uint8Array;
  /** Override the Frame Length field (default = actual unescaped length). */
  frameLengthOverride?: number;
}

export function buildUnescapedFrame(opts: UnescapedFrameOptions): Uint8Array {
  const { version = 0x02, crc = 0, flags = 0, tor, body } = opts;
  const totalLen = HEADER_SIZE + body.length + 1; // SOR + 9 header + body + EOR
  const flen = opts.frameLengthOverride ?? totalLen;
  const buf = new Uint8Array(totalLen);
  buf[0] = SOR;
  buf[1] = version;
  buf[2] = flen & 0xff;
  buf[3] = (flen >>> 8) & 0xff;
  buf[4] = crc & 0xff;
  buf[5] = (crc >>> 8) & 0xff;
  buf[6] = flags & 0xff;
  buf[7] = (flags >>> 8) & 0xff;
  buf[8] = tor & 0xff;
  buf[9] = (tor >>> 8) & 0xff;
  buf.set(body, HEADER_SIZE);
  buf[totalLen - 1] = EOR;
  return buf;
}

export function u16leBytes(v: number): number[] {
  return [v & 0xff, (v >>> 8) & 0xff];
}

export function u32leBytes(v: number): number[] {
  return [v & 0xff, (v >>> 8) & 0xff, (v >>> 16) & 0xff, (v >>> 24) & 0xff];
}

export function u64leBytes(v: bigint): number[] {
  const out: number[] = [];
  for (let i = 0; i < 8; i++) {
    out.push(Number((v >> BigInt(8 * i)) & 0xffn));
  }
  return out;
}

export interface PassingFields {
  passingNumber: number;
  transponder: number;
  rtcTimeUs: bigint;
  strength: number;
  hits: number;
  flags: number;
  decoderId?: number;
}

/** Build a wire-encoded (escaped) PASSING frame. */
export function buildPassingFrame(opts: PassingFields): Uint8Array {
  const tlv: number[] = [
    0x01,
    4,
    ...u32leBytes(opts.passingNumber),
    0x03,
    4,
    ...u32leBytes(opts.transponder),
    0x04,
    8,
    ...u64leBytes(opts.rtcTimeUs),
    0x05,
    2,
    ...u16leBytes(opts.strength),
    0x06,
    2,
    ...u16leBytes(opts.hits),
    0x08,
    2,
    ...u16leBytes(opts.flags),
  ];
  if (opts.decoderId !== undefined) {
    tlv.push(0x81, 4, ...u32leBytes(opts.decoderId));
  }
  return escapeFrame(buildUnescapedFrame({ tor: 0x0001, body: new Uint8Array(tlv) }));
}

export interface StatusFields {
  noise?: number;
  gps?: number;
  temperature?: number;
  inputVoltage?: number;
  decoderId?: number;
}

/** Build a wire-encoded (escaped) STATUS frame. */
export function buildStatusFrame(opts: StatusFields): Uint8Array {
  const tlv: number[] = [];
  if (opts.noise !== undefined) tlv.push(0x01, 2, ...u16leBytes(opts.noise));
  if (opts.gps !== undefined) tlv.push(0x06, 1, opts.gps & 0xff);
  if (opts.temperature !== undefined) tlv.push(0x07, 2, ...u16leBytes(opts.temperature));
  if (opts.inputVoltage !== undefined) tlv.push(0x0c, 1, opts.inputVoltage & 0xff);
  if (opts.decoderId !== undefined) tlv.push(0x81, 4, ...u32leBytes(opts.decoderId));
  return escapeFrame(buildUnescapedFrame({ tor: 0x0002, body: new Uint8Array(tlv) }));
}
