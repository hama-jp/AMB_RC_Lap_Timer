/**
 * Format a lap time for ja-JP TTS that should read seconds + each centisecond
 * digit individually.
 *
 * Why this exists (Issue #98 / β-1 B-2):
 *   iOS Safari ja-JP TTS reads `19.486秒` as 「いちまんきゅうせんよんひゃく
 *   はちじゅうろくびょう」 — the decimal point is consumed and 19486 is
 *   spoken as a single 5-digit integer (≈ 5h24m). Even after dropping the
 *   decimal point (`19秒49`), the trailing two digits are still concatenated
 *   into a 2-digit reading 「さんじゅうよん」 etc.
 *
 *   The reliable workaround on iOS Safari is to **insert an ASCII space
 *   between the tens and ones digit**, which forces digit-by-digit reading
 *   and gives the operator a small audible pause:
 *     19秒4 9  →  「じゅうきゅう びょう よん きゅう」
 *
 * Rounding: half-up to centiseconds, so 12.345 → "12秒3 5" (not 3 4).
 *
 * Returns "" when lapTimeUs is null or negative; callers should skip speech.
 */
export function formatLapTimeForSpeech(lapTimeUs: bigint | null): string {
  if (lapTimeUs === null || lapTimeUs < 0n) {
    return '';
  }
  // Round half-up µs → centiseconds. centiseconds = round(lapTimeUs / 10000).
  const centiseconds = (lapTimeUs + 5_000n) / 10_000n;
  const wholeSeconds = centiseconds / 100n;
  const fraction = Number(centiseconds % 100n);
  const tens = Math.floor(fraction / 10);
  const ones = fraction % 10;
  return `${wholeSeconds.toString()}秒${tens} ${ones}`;
}
