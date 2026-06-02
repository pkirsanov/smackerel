/**
 * Spec 077 SCOPE-1b — fail-loud SST consumer for the PWA e2e-ui harness.
 *
 * Smackerel's NO-DEFAULTS SST policy forbids silent fallback substitution
 * for runtime config values. The PWA browser harness reads its base URL
 * exclusively from SMACKEREL_BASE_URL, which the disposable test-stack
 * bring-up sources from config/generated/test.env. If the variable is
 * unset or empty, Playwright config loading MUST throw with a clear
 * message naming the variable. No `??`, no `||`, no hardcoded localhost,
 * no port default.
 *
 * Anchors scenario SCN-077-A10 (TP-077-01-03).
 */
export function requireSmackerelBaseUrl(): string {
  const value = process.env.SMACKEREL_BASE_URL;
  if (typeof value !== "string" || value.length === 0) {
    throw new Error(
      "SMACKEREL_BASE_URL is required by the spec 077 PWA e2e-ui harness " +
        "but is unset or empty. The disposable test stack must export " +
        "SMACKEREL_BASE_URL (sourced from config/generated/test.env) before " +
        "invoking Playwright. No silent default (localhost, empty string, " +
        "hardcoded port) is substituted — fail-loud per SCN-077-A10.",
    );
  }
  return value;
}
