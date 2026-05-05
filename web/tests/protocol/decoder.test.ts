/**
 * End-to-end protocol decoder tests against the committed AMB P3 fixture
 * `gateway/testdata/captured/session-2026-05-05.bin` (anonymized — see
 * `docs/captured-sessions/2026-05-05.md`).
 *
 * These tests run in Node (vitest), so reading from `gateway/testdata/`
 * via `node:fs` is fine. The production parser is fs-free.
 */
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { describe, expect, it } from 'vitest';

import { createDecoder } from '../../src/protocol/decoder.js';
import type { ParseResult, PassingRecord } from '../../src/protocol/records.js';

const here = dirname(fileURLToPath(import.meta.url));
const fixtureRoot = resolve(here, '..', '..', '..', 'gateway', 'testdata', 'captured');
const binPath = resolve(fixtureRoot, 'session-2026-05-05.bin');
const expectedPath = resolve(fixtureRoot, 'session-2026-05-05.expected.json');

interface ExpectedPassing {
  passingNumber: number;
  transponder: number;
  rtcTimeUs: string;
  strength: number;
  hits: number;
  flags: number;
  decoderId?: number;
}
interface Expected {
  fixture: string;
  frameCount: number;
  torDistribution: Record<string, number>;
  passingRecords: ExpectedPassing[];
  statusRecordCount: number;
  unknownTors: Array<{ tor: string; frameIndex: number }>;
  malformedCount: number;
}

const fixtureBytes = new Uint8Array(readFileSync(binPath));
const expected: Expected = JSON.parse(readFileSync(expectedPath, 'utf8')) as Expected;

function decodeAll(bytes: Uint8Array): ParseResult[] {
  const dec = createDecoder();
  return dec.push(bytes);
}

function asExpectedPassing(r: PassingRecord): ExpectedPassing {
  const out: ExpectedPassing = {
    passingNumber: r.passingNumber,
    transponder: r.transponder,
    rtcTimeUs: r.rtcTimeUs.toString(),
    strength: r.strength,
    hits: r.hits,
    flags: r.flags,
  };
  if (r.decoderId !== undefined) out.decoderId = r.decoderId;
  return out;
}

describe('decoder vs captured fixture (2026-05-05)', () => {
  it('emits the expected number of frames', () => {
    expect(decodeAll(fixtureBytes)).toHaveLength(expected.frameCount);
  });

  it('produces zero malformed results', () => {
    const malformed = decodeAll(fixtureBytes).filter((r) => r.kind === 'malformed');
    expect(malformed).toEqual([]);
  });

  it('matches the expected TOR distribution', () => {
    const dist: Record<string, number> = {};
    const torKey = (n: number) => `0x${n.toString(16).toUpperCase().padStart(4, '0')}`;
    for (const r of decodeAll(fixtureBytes)) {
      switch (r.kind) {
        case 'passing':
          dist[torKey(0x0001)] = (dist[torKey(0x0001)] ?? 0) + 1;
          break;
        case 'status':
          dist[torKey(0x0002)] = (dist[torKey(0x0002)] ?? 0) + 1;
          break;
        case 'unknown':
          dist[torKey(r.tor)] = (dist[torKey(r.tor)] ?? 0) + 1;
          break;
        case 'malformed':
          break;
      }
    }
    expect(dist).toEqual(expected.torDistribution);
  });

  it('matches every PASSING record in order', () => {
    const passings = decodeAll(fixtureBytes)
      .filter((r): r is Extract<ParseResult, { kind: 'passing' }> => r.kind === 'passing')
      .map((r) => asExpectedPassing(r.record));
    expect(passings).toEqual(expected.passingRecords);
  });

  it('counts STATUS records correctly', () => {
    const count = decodeAll(fixtureBytes).filter((r) => r.kind === 'status').length;
    expect(count).toBe(expected.statusRecordCount);
  });

  it('flags the undocumented TOR 0x001C as unknown at frame index 0', () => {
    const all = decodeAll(fixtureBytes);
    const seen: Array<{ tor: string; frameIndex: number }> = [];
    all.forEach((r, i) => {
      if (r.kind === 'unknown') {
        seen.push({
          tor: `0x${r.tor.toString(16).toUpperCase().padStart(4, '0')}`,
          frameIndex: i,
        });
      }
    });
    expect(seen).toEqual(expected.unknownTors);
  });

  it('Frame Length sanity check passes for every frame in the fixture', () => {
    // Indirect: a frame-length-mismatch would appear as malformed, which is
    // already covered above. This case re-asserts the spec §9 #1 invariant
    // (Frame Length = unescaped total length) holds for all 58 captured frames.
    const malformed = decodeAll(fixtureBytes).filter((r) => r.kind === 'malformed');
    expect(malformed).toHaveLength(0);
  });

  it('survives ingestion in random small chunks (split robustness)', () => {
    // Feed the fixture through the decoder in 17-byte chunks (a value that
    // doesn't divide any of our frame lengths, so we hit mid-frame splits).
    const dec = createDecoder();
    const all: ParseResult[] = [];
    const step = 17;
    for (let i = 0; i < fixtureBytes.length; i += step) {
      all.push(...dec.push(fixtureBytes.subarray(i, Math.min(i + step, fixtureBytes.length))));
    }
    expect(all).toHaveLength(expected.frameCount);
    expect(all.filter((r) => r.kind === 'malformed')).toEqual([]);
  });

  it('reset() lets a fresh stream re-ingest the fixture cleanly', () => {
    const dec = createDecoder();
    dec.push(fixtureBytes.subarray(0, 200)); // dirty buffer with partial frames
    dec.reset();
    const all = dec.push(fixtureBytes);
    expect(all).toHaveLength(expected.frameCount);
  });
});
