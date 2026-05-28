import { describe, expect, it } from "vitest";
import {
  BACKOFF_SCHEDULE_MS,
  DEAD_LETTER_AFTER_ATTEMPTS,
  nextBackoff,
} from "../../src/background/backoff.js";

describe("backoff schedule", () => {
  it("SCN-058-013 followsDesignedCurve: 7 attempts hit the design §4.2 curve within ±10% jitter", () => {
    // Use midpoint random (0.5) → jitter factor 1.0 → exact base intervals.
    const random = () => 0.5;
    const got = Array.from({ length: 7 }, (_, i) => nextBackoff(i + 1, random));
    const want = BACKOFF_SCHEDULE_MS.slice(0, 7);
    for (let i = 0; i < 7; i++) {
      expect(got[i].delayMs).toBe(want[i]);
      // dead-letter only fires at attempt = schedule length (8)
      expect(got[i].deadLetter).toBe(false);
    }
  });

  it("8th attempt caps at 24h and surfaces dead-letter", () => {
    const random = () => 0.5;
    const b = nextBackoff(8, random);
    expect(b.delayMs).toBe(24 * 60 * 60_000);
    expect(b.deadLetter).toBe(true);
    expect(DEAD_LETTER_AFTER_ATTEMPTS).toBe(8);
  });

  it("attempts beyond the schedule clamp at the final 24h interval", () => {
    const b = nextBackoff(100, () => 0.5);
    expect(b.delayMs).toBe(24 * 60 * 60_000);
    expect(b.deadLetter).toBe(true);
  });

  it("jitter stays within ±10% of the base interval", () => {
    // random=0 → factor 0.9; random=1 → factor 1.1
    const lo = nextBackoff(3, () => 0);
    const hi = nextBackoff(3, () => 1);
    const base = BACKOFF_SCHEDULE_MS[2]; // attempt 3 → index 2 → 5s
    expect(lo.delayMs).toBeGreaterThanOrEqual(Math.round(base * 0.9) - 1);
    expect(lo.delayMs).toBeLessThanOrEqual(Math.round(base * 0.9) + 1);
    expect(hi.delayMs).toBeGreaterThanOrEqual(Math.round(base * 1.1) - 1);
    expect(hi.delayMs).toBeLessThanOrEqual(Math.round(base * 1.1) + 1);
  });
});
