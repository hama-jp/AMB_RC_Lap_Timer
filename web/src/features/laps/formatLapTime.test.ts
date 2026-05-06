import { describe, expect, it } from 'vitest';

import { formatLapDelta, formatLapTime } from './formatLapTime';

describe('formatLapTime', () => {
  it.each([
    [null, '—'],
    [0n, '0.000'],
    [100n, '0.000'],
    [1_000n, '0.001'],
    [999_999n, '0.999'],
    [1_500_000n, '1.500'],
    [21_789_000n, '21.789'],
  ] as const)('formats %s as %s', (input, expected) => {
    expect(formatLapTime(input)).toBe(expected);
  });
});

describe('formatLapDelta', () => {
  it.each([
    // [lap, best, expected]
    [null, null, ''],
    [21_000_000n, null, ''],
    [null, 21_000_000n, ''],
    [21_000_000n, 21_000_000n, 'Best!'],
    [22_234_000n, 21_000_000n, '+1.234'],
    [21_500_000n, 21_500_000n, 'Best!'],
    [21_018_000n, 21_585_000n, '−0.567'],
    [120_999_000n, 60_500_000n, '+60.499'],
    // sub-millisecond differences floor to millisecond resolution
    [21_000_500n, 21_000_000n, '+0.000'],
  ] as const)('formats lap=%s best=%s as %s', (lap, best, expected) => {
    expect(formatLapDelta(lap, best)).toBe(expected);
  });
});
