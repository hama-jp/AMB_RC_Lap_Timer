import { describe, expect, it } from 'vitest';
import { createDecoder } from '../../src/protocol/decoder.js';
import { TOR } from '../../src/protocol/records.js';
import {
  buildPassingFrame,
  buildStatusFrame,
  buildUnescapedFrame,
  escapeFrame,
  u16leBytes,
  u32leBytes,
  u64leBytes,
} from './synthetic.js';

describe('decoder TOR dispatch', () => {
  it('decodes a fully populated PASSING record', () => {
    const dec = createDecoder();
    const frame = buildPassingFrame({
      passingNumber: 0x001185e3,
      transponder: 0x00000001,
      rtcTimeUs: 1_777_985_972_473_000n,
      strength: 167,
      hits: 144,
      flags: 0,
      decoderId: 0x00041d17,
    });
    const out = dec.push(frame);
    expect(out).toHaveLength(1);
    expect(out[0]!.kind).toBe('passing');
    if (out[0]!.kind !== 'passing') return;
    const r = out[0]!.record;
    expect(r.passingNumber).toBe(0x001185e3);
    expect(r.transponder).toBe(0x00000001);
    expect(r.rtcTimeUs).toBe(1_777_985_972_473_000n);
    expect(r.strength).toBe(167);
    expect(r.hits).toBe(144);
    expect(r.flags).toBe(0);
    expect(r.decoderId).toBe(0x00041d17);
  });

  /**
   * Spec §7.3 / §9 #5 confirmed HITS may exceed 1 byte. Make sure the
   * decoder reads the full uint16 LE rather than truncating to a byte.
   */
  it('reads HITS as uint16 LE for values > 0xFF', () => {
    const dec = createDecoder();
    const frame = buildPassingFrame({
      passingNumber: 1,
      transponder: 1,
      rtcTimeUs: 0n,
      strength: 200,
      hits: 0x0117, // 279
      flags: 0,
    });
    const out = dec.push(frame);
    expect(out[0]!.kind).toBe('passing');
    if (out[0]!.kind !== 'passing') return;
    expect(out[0]!.record.hits).toBe(0x0117);
  });

  it('decodes a STATUS record with all observed fields', () => {
    const dec = createDecoder();
    const frame = buildStatusFrame({
      noise: 6,
      gps: 0,
      temperature: 27,
      inputVoltage: 0x79,
      decoderId: 0x00041d17,
    });
    const out = dec.push(frame);
    expect(out[0]!.kind).toBe('status');
    if (out[0]!.kind !== 'status') return;
    const r = out[0]!.record;
    expect(r.noise).toBe(6);
    expect(r.gps).toBe(0);
    expect(r.temperature).toBe(27);
    expect(r.inputVoltage).toBe(0x79);
    expect(r.decoderId).toBe(0x00041d17);
  });

  it('classifies undocumented TOR 0x001C as unknown and keeps the raw bytes', () => {
    const dec = createDecoder();
    // Synthesize a TOR=0x001C frame; body shape is irrelevant for `unknown`.
    const unesc = buildUnescapedFrame({
      tor: TOR.HANDSHAKE_001C,
      body: new Uint8Array([0x01, 0x02, 0xaa, 0xbb]),
    });
    const wire = escapeFrame(unesc);
    const out = dec.push(wire);
    expect(out[0]!.kind).toBe('unknown');
    if (out[0]!.kind !== 'unknown') return;
    expect(out[0]!.tor).toBe(0x001c);
    expect(Array.from(out[0]!.raw)).toEqual(Array.from(wire));
  });

  it('classifies an arbitrary unknown TOR (0x4242) as unknown', () => {
    const dec = createDecoder();
    const unesc = buildUnescapedFrame({ tor: 0x4242, body: new Uint8Array(0) });
    const wire = escapeFrame(unesc);
    const out = dec.push(wire);
    expect(out[0]!.kind).toBe('unknown');
    if (out[0]!.kind !== 'unknown') return;
    expect(out[0]!.tor).toBe(0x4242);
  });

  it('returns malformed.missing-required-field when PASSING lacks TRANSPONDER', () => {
    // Same TLVs as a normal PASSING but omit id=0x03.
    const tlv = [
      0x01,
      4,
      ...u32leBytes(123),
      0x04,
      8,
      ...u64leBytes(1n),
      0x05,
      2,
      ...u16leBytes(100),
      0x06,
      2,
      ...u16leBytes(50),
      0x08,
      2,
      ...u16leBytes(0),
    ];
    const wire = escapeFrame(buildUnescapedFrame({ tor: TOR.PASSING, body: new Uint8Array(tlv) }));
    const dec = createDecoder();
    const out = dec.push(wire);
    expect(out[0]!.kind).toBe('malformed');
    if (out[0]!.kind !== 'malformed') return;
    expect(out[0]!.reason).toBe('missing-required-field');
  });

  it('returns malformed.unexpected-field-length when STRENGTH is the wrong width', () => {
    // STRENGTH (id=0x05) declared as 1 byte, not 2.
    const tlv = [
      0x01,
      4,
      ...u32leBytes(1),
      0x03,
      4,
      ...u32leBytes(1),
      0x04,
      8,
      ...u64leBytes(0n),
      0x05,
      1,
      0x42, // wrong width
      0x06,
      2,
      ...u16leBytes(0),
      0x08,
      2,
      ...u16leBytes(0),
    ];
    const wire = escapeFrame(buildUnescapedFrame({ tor: TOR.PASSING, body: new Uint8Array(tlv) }));
    const dec = createDecoder();
    const out = dec.push(wire);
    expect(out[0]!.kind).toBe('malformed');
    if (out[0]!.kind !== 'malformed') return;
    expect(out[0]!.reason).toBe('unexpected-field-length');
  });

  it('returns malformed.frame-length-mismatch when the header lies about length', () => {
    // Build a frame and override the Frame Length field to a wrong value.
    const unesc = buildUnescapedFrame({
      tor: TOR.STATUS,
      body: new Uint8Array([0x01, 0x02, 0x06, 0x00]),
      frameLengthOverride: 999,
    });
    const wire = escapeFrame(unesc);
    const dec = createDecoder();
    const out = dec.push(wire);
    expect(out[0]!.kind).toBe('malformed');
    if (out[0]!.kind !== 'malformed') return;
    expect(out[0]!.reason).toBe('frame-length-mismatch');
  });

  it('skips unknown PASSING TLV ids (forward compat) without breaking decode', () => {
    // Insert an unrecognised id=0x99 between known TLVs. PASSING decode
    // should still succeed.
    const tlv = [
      0x01,
      4,
      ...u32leBytes(7),
      0x99,
      2,
      0xde,
      0xad, // unknown — should be skipped
      0x03,
      4,
      ...u32leBytes(2),
      0x04,
      8,
      ...u64leBytes(42n),
      0x05,
      2,
      ...u16leBytes(150),
      0x06,
      2,
      ...u16leBytes(80),
      0x08,
      2,
      ...u16leBytes(0),
    ];
    const wire = escapeFrame(buildUnescapedFrame({ tor: TOR.PASSING, body: new Uint8Array(tlv) }));
    const dec = createDecoder();
    const out = dec.push(wire);
    expect(out[0]!.kind).toBe('passing');
    if (out[0]!.kind !== 'passing') return;
    expect(out[0]!.record.transponder).toBe(2);
  });
});
