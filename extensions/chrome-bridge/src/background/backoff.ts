// Exponential backoff schedule per design §4.2 step 7:
//   1s → 2s → 5s → 15s → 60s → 5m → 30m → 24h cap
// with ±10% jitter applied multiplicatively. Attempt count is 1-indexed
// (the first failure returns the first interval, etc.); attempts > schedule
// length clamp at the final 24h cap and signal the dead-letter badge.

export const BACKOFF_SCHEDULE_MS: ReadonlyArray<number> = [
  1_000,
  2_000,
  5_000,
  15_000,
  60_000,
  5 * 60_000,
  30 * 60_000,
  24 * 60 * 60_000,
];

export const DEAD_LETTER_AFTER_ATTEMPTS = BACKOFF_SCHEDULE_MS.length;

export interface BackoffResult {
  delayMs: number;
  deadLetter: boolean;
}

export function nextBackoff(
  attempts: number,
  random: () => number = Math.random,
): BackoffResult {
  if (attempts < 1) attempts = 1;
  const capped = Math.min(attempts, BACKOFF_SCHEDULE_MS.length) - 1;
  const base = BACKOFF_SCHEDULE_MS[capped];
  // ±10% jitter
  const jitter = 1 + (random() * 2 - 1) * 0.1;
  return {
    delayMs: Math.max(0, Math.round(base * jitter)),
    deadLetter: attempts >= DEAD_LETTER_AFTER_ATTEMPTS,
  };
}
