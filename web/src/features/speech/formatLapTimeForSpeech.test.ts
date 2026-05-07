import { describe, expect, it } from 'vitest';

import { formatLapTimeForSpeech } from './formatLapTimeForSpeech';

describe('formatLapTimeForSpeech', () => {
  it.each([
    // [input µs, expected output]
    [null, ''],
    [-1n, ''], // negative → no speech
    [0n, '0秒0 0'],
    [10_000_000n, '10秒0 0'], // exactly 10s
    [13_120_000n, '13秒1 2'], // user-provided example (12.34 form, 1+2 split)
    [12_345_000n, '12秒3 5'], // round half-up (12.345 → cs 1235)
    [12_344_000n, '12秒3 4'], // floor (12.344 → cs 1234)
    [19_486_000n, '19秒4 9'], // β-1 B-2 bug case
    [21_789_000n, '21秒7 9'], // existing test value
    [60_000_000n, '60秒0 0'], // exactly 60s
    [999_000n, '1秒0 0'], // 0.999 rounds up to 1.00
    [4_999n, '0秒0 0'], // 0.005 µs short of half rounds down (4 999 ≈ floor)
    [5_000n, '0秒0 1'], // 0.005 ms → cs 0.5 → 1 (half-up)
  ] as const)('formats %s as %s', (input, expected) => {
    expect(formatLapTimeForSpeech(input)).toBe(expected);
  });

  it('always inserts a single ASCII space between the centisecond digits', () => {
    // Regression guard: the ASCII space (U+0020) is the iOS-Safari-friendly
    // separator. A wide space or comma would change the TTS pause length.
    const out = formatLapTimeForSpeech(12_345_000n);
    expect(out).toContain('秒3 5');
    // The byte between '3' and '5' must be a single 0x20.
    const idx = out.indexOf('3');
    expect(out.charCodeAt(idx + 1)).toBe(0x20);
  });
});
