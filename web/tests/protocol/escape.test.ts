import { describe, expect, it } from 'vitest';
import { unescape } from '../../src/protocol/escape.js';

describe('unescape', () => {
  it('decodes 0x8D 0xAD into 0x8D', () => {
    const wire = new Uint8Array([0x8e, 0x8d, 0xad, 0x8f]);
    const { bytes, error } = unescape(wire);
    expect(Array.from(bytes)).toEqual([0x8e, 0x8d, 0x8f]);
    expect(error).toBeUndefined();
  });

  it('decodes 0x8D 0xAE into 0x8E', () => {
    const wire = new Uint8Array([0x8e, 0x8d, 0xae, 0x8f]);
    const { bytes } = unescape(wire);
    expect(Array.from(bytes)).toEqual([0x8e, 0x8e, 0x8f]);
  });

  it('decodes 0x8D 0xAF into 0x8F', () => {
    const wire = new Uint8Array([0x8e, 0x8d, 0xaf, 0x8f]);
    const { bytes } = unescape(wire);
    expect(Array.from(bytes)).toEqual([0x8e, 0x8f, 0x8f]);
  });

  it('decodes consecutive escape sequences', () => {
    const wire = new Uint8Array([0x8e, 0x8d, 0xad, 0x8d, 0xae, 0x8d, 0xaf, 0x8f]);
    const { bytes } = unescape(wire);
    expect(Array.from(bytes)).toEqual([0x8e, 0x8d, 0x8e, 0x8f, 0x8f]);
  });

  it('reports invalid-escape when 0x8D dangles at body end', () => {
    // SOR | 0x01 | 0x8D | EOR — the 0x8D before EOR has no follow-on byte.
    const wire = new Uint8Array([0x8e, 0x01, 0x8d, 0x8f]);
    const { bytes, error } = unescape(wire);
    expect(error).toEqual({ reason: 'invalid-escape', offset: 2 });
    // The dangling 0x8D is dropped from the output.
    expect(Array.from(bytes)).toEqual([0x8e, 0x01, 0x8f]);
  });

  it('passes through bodies with no escape sequences', () => {
    const wire = new Uint8Array([0x8e, 0x01, 0x02, 0x03, 0x8f]);
    const { bytes } = unescape(wire);
    expect(Array.from(bytes)).toEqual(Array.from(wire));
  });

  it('preserves SOR and EOR even when only those are present', () => {
    const wire = new Uint8Array([0x8e, 0x8f]);
    const { bytes } = unescape(wire);
    expect(Array.from(bytes)).toEqual([0x8e, 0x8f]);
  });
});
