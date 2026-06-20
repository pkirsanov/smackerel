/**
 * Spec 077 SCOPE-3 — Content-Security-Policy guard for the PWA
 * browser harness.
 *
 * `attachCSPGuard(page)` wires console + pageerror + browser-native
 * `securitypolicyviolation` listeners on the supplied page. Any
 * captured violation is buffered against the page, and
 * `assertNoCSPViolations(page)` (typically called at end of a test)
 * fails the test if the buffer is non-empty.
 *
 * Anchors SCN-077-A05 (TP-077-03-05). The skeleton was shipped in
 * SCOPE-1b; SCOPE-3 wires the real listeners + the assertion.
 *
 * Backwards-compatibility note: the SCOPE-1b skeleton accepted
 * `unknown` for the page parameter so non-Playwright unit tests
 * could import it. SCOPE-3 narrows that to `Page` because every
 * caller in the PWA test suite already imports `@playwright/test`.
 */
import type { Page } from "@playwright/test";

type ViolationBucket = string[];

const VIOLATION_BUCKETS = new WeakMap<Page, ViolationBucket>();

const CSP_PATTERN =
  /content security policy|refused to (load|execute|apply|connect|frame|run)|csp_violation|csp violation/i;

/**
 * BUG-001 fix: Check if an error is the expected "already bound" case.
 * Playwright throws when exposeBinding/addInitScript is called twice
 * on the same page. We want to swallow ONLY this expected case, not
 * all errors (which would mask legitimate failures like page closed,
 * browser crash, etc.).
 */
function isAlreadyBoundError(err: unknown): boolean {
  if (!(err instanceof Error)) return false;
  const msg = err.message.toLowerCase();
  // Playwright error messages for duplicate binding/script
  return (
    (msg.includes("already") &&
      (msg.includes("exposed") || msg.includes("bound"))) ||
    msg.includes("duplicate")
  );
}

function bucketFor(page: Page): ViolationBucket {
  let b = VIOLATION_BUCKETS.get(page);
  if (!b) {
    b = [];
    VIOLATION_BUCKETS.set(page, b);
  }
  return b;
}

export function attachCSPGuard(page: Page): void {
  const bucket = bucketFor(page);

  page.on("console", (msg) => {
    if (msg.type() !== "error") return;
    const text = msg.text();
    if (CSP_PATTERN.test(text)) {
      bucket.push(`console.error: ${text}`);
    }
  });

  page.on("pageerror", (err) => {
    const text = String(err && err.message ? err.message : err);
    if (CSP_PATTERN.test(text)) {
      bucket.push(`pageerror: ${text}`);
    }
  });

  // Browser-native CSP violation event. Forwarded into the harness
  // via an exposed binding so we capture violations the browser
  // generated against a real CSP header (when one is set in a
  // future production-mode test variant). exposeBinding throws if
  // called twice on the same page, so swallow that specific case.
  // BUG-001 fix: Only swallow expected "already bound" errors; warn
  // on unexpected errors so they surface in test output.
  page
    .exposeBinding(
      "__spec077ReportCSPViolation",
      (_source, payload: { directive: string; blockedURI: string }) => {
        bucket.push(
          `securitypolicyviolation: directive=${payload.directive} blockedURI=${payload.blockedURI}`,
        );
      },
    )
    .catch((err: unknown) => {
      if (isAlreadyBoundError(err)) return; // Expected: page re-used
      console.warn("[csp.ts] exposeBinding failed unexpectedly:", err);
    });

  // BUG-001 fix: Only swallow expected errors; warn on unexpected.
  page
    .addInitScript(() => {
      document.addEventListener("securitypolicyviolation", (event) => {
        const ev = event as SecurityPolicyViolationEvent;
        const w = window as unknown as {
          __spec077ReportCSPViolation?: (p: {
            directive: string;
            blockedURI: string;
          }) => void;
        };
        if (typeof w.__spec077ReportCSPViolation === "function") {
          w.__spec077ReportCSPViolation({
            directive: ev.violatedDirective || ev.effectiveDirective || "",
            blockedURI: ev.blockedURI || "",
          });
        }
      });
    })
    .catch((err: unknown) => {
      if (isAlreadyBoundError(err)) return; // Expected: page re-used
      console.warn("[csp.ts] addInitScript failed unexpectedly:", err);
    });
}

export function assertNoCSPViolations(page: Page): void {
  const bucket = VIOLATION_BUCKETS.get(page);
  if (!bucket || bucket.length === 0) return;
  const msg = bucket.slice().join("\n  ");
  // Clear so a re-used page does not double-report.
  bucket.length = 0;
  throw new Error(
    `Spec 077 CSP guard captured violation(s) on this page:\n  ${msg}`,
  );
}
