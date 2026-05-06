import { describe, expect, it } from 'vitest';

import { formatLapTime } from './formatLapTime';

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
