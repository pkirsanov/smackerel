# Bug: BUG-001 CSP Guard Silent Error Swallowing

**Status:** in_progress
**Severity:** High
**Found By:** bubbles.chaos (stochastic-quality-sweep round 20)
**Date:** 2026-06-17
**Root Cause:** Overly broad error swallowing in CSP guard attachment

---

## Problem Statement

The CSP guard in `web/pwa/tests/_support/csp.ts` uses `.catch(() => undefined)` at lines 69 and 89 to swallow errors from `page.exposeBinding()` and `page.addInitScript()`. The comment claims this handles the "already bound" case, but the catch swallows ALL errors including:

- Page context already closed (navigation race)
- Browser process crashed mid-attachment
- Invalid binding name (hypothetical future bug)
- Out of memory during script injection
- Any other legitimate failure

**Impact:** If attachment fails for a legitimate reason, the CSP guard silently fails to attach. CSP violations on the page would then go undetected. The test suite reports the page as CSP-clean when it may not be, giving false confidence.

---

## Reproduction

```typescript
// Simulated page that rejects exposeBinding with a real error
const brokenPage = {
  on(_event: string, _fn: unknown) {},
  exposeBinding(_name: string, _fn: unknown) {
    return Promise.reject(new Error("Target page closed"));
  },
  addInitScript(_fn: unknown) {
    return Promise.reject(new Error("Execution context was destroyed"));
  },
};

// Current code silently succeeds - BUG
attachCSPGuard(brokenPage as never);
assertNoCSPViolations(brokenPage as never); // PASSES but guard never attached
```

---

## Expected Behavior

The CSP guard should:
1. Swallow ONLY the "already exposed" error (Playwright throws when binding already exists)
2. Log or re-throw any OTHER error so tests fail clearly when the guard can't attach
3. Optionally track attachment state so `assertNoCSPViolations` can warn if called without successful attachment

---

## Fix Plan

1. Refine the catch handlers to check the error message for the expected "already bound" pattern
2. Re-throw (or at minimum console.warn) any unexpected error
3. Add chaos-regression test that verifies unexpected errors are not swallowed

---

## Related Artifacts

- Parent spec: [spec.md](../../spec.md)
- Affected file: `web/pwa/tests/_support/csp.ts`
- Test anchor: TP-077-BUG-001-01
