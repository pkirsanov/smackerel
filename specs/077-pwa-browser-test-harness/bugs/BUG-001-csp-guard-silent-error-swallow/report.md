# Report: BUG-001 CSP Guard Silent Error Swallowing Fix

## Discovery

- **Date:** 2026-06-17
- **Found By:** bubbles.chaos (stochastic-quality-sweep round 20)
- **Trigger:** chaos-hardening workflow mode
- **Vector:** Error handling resilience probe on CSP guard attachment

## Root Cause Analysis

The CSP guard at `web/pwa/tests/_support/csp.ts` lines 69 and 89 uses `.catch(() => undefined)` which swallows ALL errors, not just the expected "already bound" case.

### Code Before (Problematic)

```typescript
page.exposeBinding("__spec077ReportCSPViolation", ...)
  .catch(() => undefined);  // BUG: Swallows ALL errors
```

### Impact

When the browser crashes, page closes during navigation, or any other unexpected error occurs during guard attachment, the error is silently swallowed. Tests then report pages as CSP-clean when the guard was never successfully attached.

## Fix Implementation

Added `isAlreadyBoundError(err)` helper that checks if the error message matches the expected "already exposed/bound" pattern. Replaced blanket `.catch(() => undefined)` with message-aware handlers that:
- Silently swallow expected "already bound" errors (page re-use)
- Emit `console.warn` for any unexpected error so it surfaces in test output

### Code After (Fixed)

```typescript
function isAlreadyBoundError(err: unknown): boolean {
  if (!(err instanceof Error)) return false;
  const msg = err.message.toLowerCase();
  return (
    (msg.includes("already") &&
      (msg.includes("exposed") || msg.includes("bound"))) ||
    msg.includes("duplicate")
  );
}

page.exposeBinding("__spec077ReportCSPViolation", ...)
  .catch((err: unknown) => {
    if (isAlreadyBoundError(err)) return; // Expected: page re-used
    console.warn("[csp.ts] exposeBinding failed unexpectedly:", err);
  });
```

## Test Evidence

### Chaos Regression Tests (TP-077-BUG-001-01, TP-077-BUG-001-02)

```
$ node --experimental-strip-types web/pwa/tests/_support/csp.test.ts

✔ requireSmackerelBaseUrl throws fail-loud when env is unset (1.518308ms)
✔ requireSmackerelBaseUrl throws fail-loud when env is empty string (0.237501ms)
✔ requireSmackerelBaseUrl returns the value when set (0.175501ms)
✔ attachCSPGuard exposes the SCOPE-3 contract and wires page listeners (1.200706ms)
✔ BUG-001: attachCSPGuard warns on unexpected exposeBinding error (chaos regression) (50.994477ms)
✔ BUG-001: attachCSPGuard silently handles already-bound error (expected case) (51.092778ms)
ℹ tests 6
ℹ pass 6
ℹ fail 0
```

### Existing Suite Regression Check

```
$ bash tests/unit/cli/spec_077_playwright_config_fail_loud_test.sh
[spec_077_playwright_config_fail_loud] node v22.22.0
PASS: spec_077_playwright_config_fail_loud_test (TP-077-01-03 / SCN-077-A10)
```

## Files Changed

- `web/pwa/tests/_support/csp.ts` — Added `isAlreadyBoundError()` helper, refined catch handlers
- `web/pwa/tests/_support/csp.test.ts` — Added 2 chaos regression tests
