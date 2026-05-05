/**
 * Decode the committed AMB P3 fixture and print the expected.json schema
 * defined in Issue #50 to stdout.
 *
 * Run with `npm run dump-expected` (which uses tsx). The output is
 * captured into `gateway/testdata/captured/session-2026-05-05.expected.json`
 * by the maintainer; the file is then loaded by `decoder.test.ts` for the
 * round-trip assertion.
 *
 * BigInt values (rtcTimeUs) are emitted as strings so the JSON is portable
 * across language ecosystems.
 */
import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { createDecoder } from '../src/protocol/decoder.js';
import type { ParseResult, PassingRecord } from '../src/protocol/records.js';

const here = dirname(fileURLToPath(import.meta.url));
const fixturePath = resolve(
  here,
  '..',
  '..',
  'gateway',
  'testdata',
  'captured',
  'session-2026-05-05.bin',
);
const bytes = new Uint8Array(readFileSync(fixturePath));

const dec = createDecoder();
const results: ParseResult[] = dec.push(bytes);

const torDistribution: Record<string, number> = {};
const passingRecords: Array<PassingRecordJson> = [];
let statusRecordCount = 0;
const unknownTors: Array<{ tor: string; frameIndex: number }> = [];
let malformedCount = 0;

interface PassingRecordJson {
  passingNumber: number;
  transponder: number;
  rtcTimeUs: string; // bigint serialized
  strength: number;
  hits: number;
  flags: number;
  decoderId?: number;
}

function torKey(tor: number): string {
  return `0x${tor.toString(16).toUpperCase().padStart(4, '0')}`;
}

function passingToJson(r: PassingRecord): PassingRecordJson {
  const out: PassingRecordJson = {
    passingNumber: r.passingNumber,
    transponder: r.transponder,
    rtcTimeUs: r.rtcTimeUs.toString(),
    strength: r.strength,
    hits: r.hits,
    flags: r.flags,
  };
  if (r.decoderId !== undefined) {
    out.decoderId = r.decoderId;
  }
  return out;
}

results.forEach((r, i) => {
  switch (r.kind) {
    case 'passing': {
      torDistribution[torKey(0x0001)] = (torDistribution[torKey(0x0001)] ?? 0) + 1;
      passingRecords.push(passingToJson(r.record));
      break;
    }
    case 'status': {
      torDistribution[torKey(0x0002)] = (torDistribution[torKey(0x0002)] ?? 0) + 1;
      statusRecordCount++;
      break;
    }
    case 'unknown': {
      torDistribution[torKey(r.tor)] = (torDistribution[torKey(r.tor)] ?? 0) + 1;
      unknownTors.push({ tor: torKey(r.tor), frameIndex: i });
      break;
    }
    case 'malformed': {
      malformedCount++;
      break;
    }
  }
});

const expected = {
  fixture: 'gateway/testdata/captured/session-2026-05-05.bin',
  frameCount: results.length,
  torDistribution,
  passingRecords,
  statusRecordCount,
  unknownTors,
  malformedCount,
};

process.stdout.write(JSON.stringify(expected, null, 2));
process.stdout.write('\n');
