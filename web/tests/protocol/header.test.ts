import { describe, expect, it } from 'vitest';
import { HEADER_SIZE, parseHeader } from '../../src/protocol/header.js';

describe('parseHeader', () => {
  it('reads all fields little-endian from a typical PASSING frame header', () => {
    // SOR | ver=02 | flen=0x0033=51 | crc=0x1234 | flags=0x0001 | tor=PASSING
    const unesc = new Uint8Array([
      0x8e, 0x02, 0x33, 0x00, 0x34, 0x12, 0x01, 0x00, 0x01, 0x00,
      // dummy body
      0x00, 0x8f,
    ]);
    const h = parseHeader(unesc);
    expect(h).not.toBeNull();
    expect(h!.version).toBe(0x02);
    expect(h!.frameLength).toBe(51);
    expect(h!.crc).toBe(0x1234);
    expect(h!.flags).toBe(0x0001);
    expect(h!.tor).toBe(0x0001);
  });

  it('returns null when the frame is shorter than HEADER_SIZE', () => {
    const tooShort = new Uint8Array(HEADER_SIZE - 1);
    expect(parseHeader(tooShort)).toBeNull();
  });

  it('reads frameLength = 0 verbatim', () => {
    const unesc = new Uint8Array([0x8e, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0x8f]);
    const h = parseHeader(unesc);
    expect(h?.frameLength).toBe(0);
  });

  it('reads the maximum uint16 for frameLength', () => {
    const unesc = new Uint8Array([0x8e, 0x02, 0xff, 0xff, 0, 0, 0, 0, 0, 0, 0x8f]);
    const h = parseHeader(unesc);
    expect(h?.frameLength).toBe(0xffff);
  });
});
