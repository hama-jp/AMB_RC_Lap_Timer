/** Format microseconds as seconds with millisecond precision. */
export function formatLapTime(lapTimeUs: bigint | null): string {
  if (lapTimeUs === null) {
    return '—';
  }

  const wholeSeconds = lapTimeUs / 1_000_000n;
  const remainderUs = lapTimeUs % 1_000_000n;
  const milliseconds = remainderUs / 1_000n;

  return `${wholeSeconds.toString()}.${milliseconds.toString().padStart(3, '0')}`;
}

/**
 * Format the difference between this lap and the session best, in seconds.
 *
 * Returns:
 *   - `'Best!'` when `lapTimeUs === bestLapUs` (this lap IS the best)
 *   - `'+S.mmm'` / `'−S.mmm'` (using U+2212 minus, not `-`) for non-best laps
 *   - `''` when either input is `null` or the lap is not the best yet undefined
 */
export function formatLapDelta(lapTimeUs: bigint | null, bestLapUs: bigint | null): string {
  if (lapTimeUs === null || bestLapUs === null) {
    return '';
  }
  const diffUs = lapTimeUs - bestLapUs;
  if (diffUs === 0n) {
    return 'Best!';
  }
  const sign = diffUs > 0n ? '+' : '−';
  const absUs = diffUs > 0n ? diffUs : -diffUs;
  const wholeSeconds = absUs / 1_000_000n;
  const milliseconds = (absUs % 1_000_000n) / 1_000n;
  return `${sign}${wholeSeconds.toString()}.${milliseconds.toString().padStart(3, '0')}`;
}
