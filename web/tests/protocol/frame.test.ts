import { describe, expect, it } from 'vitest';
import { createFrameStream } from '../../src/protocol/frame.js';

const f = (...bytes: number[]) => new Uint8Array(bytes);

describe('FrameStream', () => {
  it('emits a single complete frame in one push', () => {
    const s = createFrameStream();
    const out = s.push(f(0x8e, 0x01, 0x02, 0x8f));
    expect(out.length).toBe(1);
    expect(Array.from(out[0]!)).toEqual([0x8e, 0x01, 0x02, 0x8f]);
  });

  it('emits two consecutive frames sharing the 0x8F 0x8E boundary', () => {
    const s = createFrameStream();
    const out = s.push(f(0x8e, 0x01, 0x8f, 0x8e, 0x02, 0x8f));
    expect(out.length).toBe(2);
    expect(Array.from(out[0]!)).toEqual([0x8e, 0x01, 0x8f]);
    expect(Array.from(out[1]!)).toEqual([0x8e, 0x02, 0x8f]);
  });

  it('drops bytes before the first SOR (pre-SOR garbage)', () => {
    const s = createFrameStream();
    const out = s.push(f(0xaa, 0xbb, 0x8e, 0x01, 0x8f));
    expect(out.length).toBe(1);
    expect(Array.from(out[0]!)).toEqual([0x8e, 0x01, 0x8f]);
  });

  it('drops bytes between two frames (inter-frame garbage)', () => {
    const s = createFrameStream();
    const out = s.push(f(0x8e, 0x01, 0x8f, 0xcc, 0xdd, 0x8e, 0x02, 0x8f));
    expect(out.length).toBe(2);
    expect(Array.from(out[1]!)).toEqual([0x8e, 0x02, 0x8f]);
  });

  it('buffers an incomplete frame and emits it after the EOR arrives', () => {
    const s = createFrameStream();
    expect(s.push(f(0x8e, 0x01, 0x02))).toEqual([]);
    const out = s.push(f(0x03, 0x04, 0x8f));
    expect(out.length).toBe(1);
    expect(Array.from(out[0]!)).toEqual([0x8e, 0x01, 0x02, 0x03, 0x04, 0x8f]);
  });

  it('does not split a frame on an escaped 0x8F (0x8D 0xAF) inside the body', () => {
    const s = createFrameStream();
    const out = s.push(f(0x8e, 0x8d, 0xaf, 0x01, 0x8f));
    expect(out.length).toBe(1);
    expect(Array.from(out[0]!)).toEqual([0x8e, 0x8d, 0xaf, 0x01, 0x8f]);
  });

  it('does not split a frame on an escaped 0x8E (0x8D 0xAE) inside the body', () => {
    const s = createFrameStream();
    const out = s.push(f(0x8e, 0x8d, 0xae, 0x01, 0x8f));
    expect(out.length).toBe(1);
    expect(Array.from(out[0]!)).toEqual([0x8e, 0x8d, 0xae, 0x01, 0x8f]);
  });

  it('waits for the second escape byte when 0x8D is at the chunk boundary', () => {
    const s = createFrameStream();
    expect(s.push(f(0x8e, 0x01, 0x8d))).toEqual([]); // dangling escape, more bytes expected
    const out = s.push(f(0xae, 0x02, 0x8f));
    expect(out.length).toBe(1);
    expect(Array.from(out[0]!)).toEqual([0x8e, 0x01, 0x8d, 0xae, 0x02, 0x8f]);
  });

  it('handles a single frame fed byte-by-byte', () => {
    const s = createFrameStream();
    const wire = [0x8e, 0x01, 0x02, 0x03, 0x8f];
    let lastOut: Uint8Array[] = [];
    for (const b of wire) {
      lastOut = s.push(f(b));
    }
    expect(lastOut.length).toBe(1);
    expect(Array.from(lastOut[0]!)).toEqual(wire);
  });

  it('reset() drops in-flight buffer state', () => {
    const s = createFrameStream();
    s.push(f(0x8e, 0x01)); // start a frame
    s.reset();
    const out = s.push(f(0x8e, 0x02, 0x8f));
    expect(out.length).toBe(1);
    expect(Array.from(out[0]!)).toEqual([0x8e, 0x02, 0x8f]);
  });

  it('returns no frames when given an empty chunk', () => {
    const s = createFrameStream();
    expect(s.push(new Uint8Array(0))).toEqual([]);
  });

  it('returns no frames when only garbage (no SOR ever)', () => {
    const s = createFrameStream();
    expect(s.push(f(0x01, 0x02, 0x03))).toEqual([]);
  });
});
