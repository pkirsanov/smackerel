/**
 * Spec 077 SCOPE-1b — Content-Security-Policy guard skeleton.
 *
 * Real CSP-violation assertions land in spec 077 SCOPE-3 (login flow +
 * CSP smoke). This skeleton ships in SCOPE-1b so SCOPE-1c (proof-of-life
 * spec) and SCOPE-3 (first real consumer) can import a stable symbol
 * without circular ordering. The `unknown` parameter shape avoids a
 * compile-time dependency on `@playwright/test` types so this module can
 * be imported by Node-level unit tests that do not have Playwright
 * installed.
 *
 * Anchors scenario SCN-077-A10 (the import-compile half of TP-077-01-03)
 * and reserves the contract for SCN-077-A03..A05, SCN-077-A08 (SCOPE-3).
 */
export function attachCSPGuard(_page: unknown): void {
  // SCOPE-3 will wire `console` + `pageerror` listeners that fail the
  // test on any CSP violation message. Skeleton intentionally a no-op so
  // the symbol resolves cleanly for callers that wire it ahead of the
  // real implementation.
}
