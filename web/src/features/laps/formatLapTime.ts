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
