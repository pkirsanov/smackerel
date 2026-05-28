// Dwell gate for history events per design §4.2 step 3. The browser-observed
// dwell is compared against the operator-configured threshold (validated to
// [0, 3600] seconds in validation.ts). A dwell of exactly the threshold passes;
// anything strictly below is dropped.

export function passesDwellGate(
  dwellSeconds: number,
  thresholdSeconds: number,
): boolean {
  if (!Number.isFinite(dwellSeconds) || dwellSeconds < 0) return false;
  return dwellSeconds >= thresholdSeconds;
}
