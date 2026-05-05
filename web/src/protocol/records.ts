/**
 * Record-type dispatch and field decoding for AMB P3.
 *
 * Spec references:
 *   - TOR table:           docs/protocol-p3.md §7.2
 *   - PASSING fields:      §7.3
 *   - STATUS fields:       §7.4
 *   - GENERAL.DECODER_ID:  §7.1
 *   - RTC_TIME unit:       §8 (μs, confirmed in v0.1.2)
 *   - Undocumented 0x001C: §7.9 (treated as `unknown`, dropped)
 */
import { iterateTLVs } from './tlv.js';

export const TOR = {
  RESET: 0x0000,
  PASSING: 0x0001,
  STATUS: 0x0002,
  VERSION: 0x0003,
  RESEND: 0x0004,
  CLEAR_PASSING: 0x0005,
  HANDSHAKE_001C: 0x001c, // §7.9 undocumented; observed at TCP connect
  ERROR: 0xffff,
} as const;

const PASSING_FIELD = {
  PASSING_NUMBER: 0x01,
  TRANSPONDER: 0x03,
  RTC_TIME: 0x04,
  STRENGTH: 0x05,
  HITS: 0x06,
  FLAGS: 0x08,
} as const;

const STATUS_FIELD = {
  NOISE: 0x01,
  GPS: 0x06,
  TEMPERATURE: 0x07,
  INPUT_VOLTAGE: 0x0c,
} as const;

const GENERAL_FIELD = {
  DECODER_ID: 0x81,
} as const;

/**
 * Decoded PASSING record. All required fields are guaranteed present;
 * `decoderId` is best-effort (the GENERAL field is always emitted by the
 * AMB decoder we observed but the spec leaves it strictly optional).
 */
export interface PassingRecord {
  readonly passingNumber: number;
  readonly transponder: number;
  readonly rtcTimeUs: bigint;
  readonly strength: number;
  readonly hits: number;
  readonly flags: number;
  readonly decoderId?: number;
}

/**
 * Decoded STATUS record. All sub-fields are optional because the AMB
 * decoder may emit any subset depending on configuration.
 */
export interface StatusRecord {
  readonly noise?: number;
  readonly gps?: number;
  readonly temperature?: number;
  readonly inputVoltage?: number;
  readonly decoderId?: number;
}

export type MalformedReason =
  | 'truncated'
  | 'frame-length-mismatch'
  | 'invalid-escape'
  | 'missing-required-field'
  | 'unexpected-field-length';

export type ParseResult =
  | { readonly kind: 'passing'; readonly record: PassingRecord }
  | { readonly kind: 'status'; readonly record: StatusRecord }
  | { readonly kind: 'unknown'; readonly tor: number; readonly raw: Uint8Array }
  | { readonly kind: 'malformed'; readonly reason: MalformedReason; readonly raw: Uint8Array };

/** Read a little-endian uint of the given byte width (1, 2, or 4). */
function readUintLE(b: Uint8Array, expected: 1 | 2 | 4): number | null {
  if (b.length !== expected) {
    return null;
  }
  const view = new DataView(b.buffer, b.byteOffset, b.byteLength);
  switch (expected) {
    case 1:
      return view.getUint8(0);
    case 2:
      return view.getUint16(0, true);
    case 4:
      return view.getUint32(0, true);
  }
}

/** Read a little-endian uint64 as bigint. */
function readUint64LE(b: Uint8Array): bigint | null {
  if (b.length !== 8) {
    return null;
  }
  const view = new DataView(b.buffer, b.byteOffset, b.byteLength);
  return view.getBigUint64(0, true);
}

export function decodePassing(unesc: Uint8Array, raw: Uint8Array): ParseResult {
  let passingNumber: number | undefined;
  let transponder: number | undefined;
  let rtcTimeUs: bigint | undefined;
  let strength: number | undefined;
  let hits: number | undefined;
  let flags: number | undefined;
  let decoderId: number | undefined;

  for (const tlv of iterateTLVs(unesc)) {
    switch (tlv.id) {
      case PASSING_FIELD.PASSING_NUMBER: {
        const v = readUintLE(tlv.value, 4);
        if (v === null) return malformed(raw, 'unexpected-field-length');
        passingNumber = v;
        break;
      }
      case PASSING_FIELD.TRANSPONDER: {
        const v = readUintLE(tlv.value, 4);
        if (v === null) return malformed(raw, 'unexpected-field-length');
        transponder = v;
        break;
      }
      case PASSING_FIELD.RTC_TIME: {
        const v = readUint64LE(tlv.value);
        if (v === null) return malformed(raw, 'unexpected-field-length');
        rtcTimeUs = v;
        break;
      }
      case PASSING_FIELD.STRENGTH: {
        const v = readUintLE(tlv.value, 2);
        if (v === null) return malformed(raw, 'unexpected-field-length');
        strength = v;
        break;
      }
      case PASSING_FIELD.HITS: {
        // Spec §7.3 / §9 #5 confirmed: HITS is uint16 LE; values may exceed
        // 1 byte (observed up to 0x0117). Read as 2 byte LE only.
        const v = readUintLE(tlv.value, 2);
        if (v === null) return malformed(raw, 'unexpected-field-length');
        hits = v;
        break;
      }
      case PASSING_FIELD.FLAGS: {
        const v = readUintLE(tlv.value, 2);
        if (v === null) return malformed(raw, 'unexpected-field-length');
        flags = v;
        break;
      }
      case GENERAL_FIELD.DECODER_ID: {
        const v = readUintLE(tlv.value, 4);
        if (v === null) return malformed(raw, 'unexpected-field-length');
        decoderId = v;
        break;
      }
      default:
        // Unknown TLV id — silently skip (forward-compat).
        break;
    }
  }

  if (
    passingNumber === undefined ||
    transponder === undefined ||
    rtcTimeUs === undefined ||
    strength === undefined ||
    hits === undefined ||
    flags === undefined
  ) {
    return malformed(raw, 'missing-required-field');
  }

  const record: PassingRecord =
    decoderId === undefined
      ? { passingNumber, transponder, rtcTimeUs, strength, hits, flags }
      : { passingNumber, transponder, rtcTimeUs, strength, hits, flags, decoderId };
  return { kind: 'passing', record };
}

export function decodeStatus(unesc: Uint8Array): ParseResult {
  const record: {
    noise?: number;
    gps?: number;
    temperature?: number;
    inputVoltage?: number;
    decoderId?: number;
  } = {};

  for (const tlv of iterateTLVs(unesc)) {
    switch (tlv.id) {
      case STATUS_FIELD.NOISE: {
        const v = readUintLE(tlv.value, 2);
        if (v !== null) record.noise = v;
        break;
      }
      case STATUS_FIELD.GPS: {
        const v = readUintLE(tlv.value, 1);
        if (v !== null) record.gps = v;
        break;
      }
      case STATUS_FIELD.TEMPERATURE: {
        const v = readUintLE(tlv.value, 2);
        if (v !== null) record.temperature = v;
        break;
      }
      case STATUS_FIELD.INPUT_VOLTAGE: {
        const v = readUintLE(tlv.value, 1);
        if (v !== null) record.inputVoltage = v;
        break;
      }
      case GENERAL_FIELD.DECODER_ID: {
        const v = readUintLE(tlv.value, 4);
        if (v !== null) record.decoderId = v;
        break;
      }
      default:
        break;
    }
  }

  return { kind: 'status', record };
}

function malformed(raw: Uint8Array, reason: MalformedReason): ParseResult {
  return { kind: 'malformed', reason, raw };
}
