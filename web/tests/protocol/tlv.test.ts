import { describe, expect, it } from 'vitest';
import { iterateTLVs } from '../../src/protocol/tlv.js';

/**
 * Build an unescaped frame whose body is the given byte array. Header
 * is filled with placeholders; TLV iteration only cares about offsets.
 */
function frameWithBody(body: number[]): Uint8Array {
  const buf = new Uint8Array(10 + body.length + 1);
  buf[0] = 0x8e;
  buf.set(body, 10);
  buf[buf.length - 1] = 0x8f;
  return buf;
}

describe('iterateTLVs', () => {
  it('yields a single TLV', () => {
    const f = frameWithBody([0x01, 0x02, 0xaa, 0xbb]);
    const tlvs = [...iterateTLVs(f)];
    expect(tlvs).toHaveLength(1);
    expect(tlvs[0]!.id).toBe(0x01);
    expect(tlvs[0]!.length).toBe(2);
    expect(Array.from(tlvs[0]!.value)).toEqual([0xaa, 0xbb]);
  });

  it('yields successive TLVs in order', () => {
    const f = frameWithBody([0x01, 0x01, 0x11, 0x02, 0x02, 0x22, 0x33]);
    const tlvs = [...iterateTLVs(f)];
    expect(tlvs).toHaveLength(2);
    expect(tlvs[0]!.id).toBe(0x01);
    expect(tlvs[1]!.id).toBe(0x02);
    expect(Array.from(tlvs[1]!.value)).toEqual([0x22, 0x33]);
  });

  /**
   * Regression for the reference Python implementation's hex-string-as-decimal
   * bug (docs/protocol-p3.md §10): Field Length 0x10 must read as 16, not 10.
   * Read directly as uint8.
   */
  it('reads Field Length 0x10 as 16 (uint8 direct, not hex-as-decimal)', () => {
    const value = new Array(16).fill(0xab) as number[];
    const f = frameWithBody([0x09, 0x10, ...value]);
    const tlvs = [...iterateTLVs(f)];
    expect(tlvs).toHaveLength(1);
    expect(tlvs[0]!.length).toBe(16);
    expect(tlvs[0]!.value.length).toBe(16);
  });

  it('reads Field Length 0xFF as 255', () => {
    const value = new Array(255).fill(0x00) as number[];
    const f = frameWithBody([0x09, 0xff, ...value]);
    const tlvs = [...iterateTLVs(f)];
    expect(tlvs[0]!.length).toBe(255);
    expect(tlvs[0]!.value.length).toBe(255);
  });

  it('handles Field Length = 0 (zero-length value)', () => {
    const f = frameWithBody([0x09, 0x00, 0x0a, 0x01, 0x77]);
    const tlvs = [...iterateTLVs(f)];
    expect(tlvs).toHaveLength(2);
    expect(tlvs[0]!.length).toBe(0);
    expect(tlvs[0]!.value.length).toBe(0);
    expect(tlvs[1]!.id).toBe(0x0a);
  });

  it('stops iterating when the trailing TLV is truncated', () => {
    // id=0x09 declares len=10 but only 2 value bytes follow.
    const f = frameWithBody([0x01, 0x02, 0xaa, 0xbb, 0x09, 0x0a, 0xcc, 0xdd]);
    const tlvs = [...iterateTLVs(f)];
    // Only the first TLV survives; the truncated one is silently dropped.
    expect(tlvs).toHaveLength(1);
    expect(tlvs[0]!.id).toBe(0x01);
  });

  it('returns no TLVs for an empty body', () => {
    const f = frameWithBody([]);
    expect([...iterateTLVs(f)]).toHaveLength(0);
  });

  it('returns nothing when the frame is shorter than header + EOR', () => {
    const tooShort = new Uint8Array(5);
    expect([...iterateTLVs(tooShort)]).toHaveLength(0);
  });
});
