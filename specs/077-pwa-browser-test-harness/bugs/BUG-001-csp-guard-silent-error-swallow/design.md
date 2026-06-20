# Design: BUG-001 CSP Guard Silent Error Swallowing Fix

## Overview

The fix refines the error handling in `attachCSPGuard()` to only swallow expected "already bound" errors while surfacing any other failure.

## Current Truth

The existing implementation catches all errors from `exposeBinding()` and `addInitScript()`:

```typescript
page.exposeBinding("__spec077ReportCSPViolation", ...)
  .catch(() => undefined);  // Swallows ALL errors

page.addInitScript(() => { ... })
  .catch(() => undefined);  // Swallows ALL errors
```

The comment at line 58 says "exposeBinding throws if called twice on the same page" but the catch swallows far more than just that case.

## Fix Design

### Refined Error Filter

Replace the blanket catch with a message-aware filter:

```typescript
// Helper to check if error is the expected "already exists" case
function isAlreadyBoundError(err: unknown): boolean {
  if (!(err instanceof Error)) return false;
  const msg = err.message.toLowerCase();
  // Playwright throws variations of these messages
  return msg.includes('already') && 
         (msg.includes('exposed') || msg.includes('bound'));
}

page.exposeBinding("__spec077ReportCSPViolation", ...)
  .catch((err) => {
    if (isAlreadyBoundError(err)) return; // Expected case - page re-used
    // Unexpected error - log warning so test output reveals the issue
    console.warn('[csp.ts] exposeBinding failed unexpectedly:', err);
  });

page.addInitScript(() => { ... })
  .catch((err) => {
    if (isAlreadyBoundError(err)) return;
    console.warn('[csp.ts] addInitScript failed unexpectedly:', err);
  });
```

### Why Console.warn Not Throw

Throwing would break existing tests that might legitimately race with page closure during navigation. A console.warn:
- Surfaces the issue in test output (operator sees it)
- Doesn't break test runs that have acceptable races
- Provides actionable information for debugging

If stricter behavior is desired later, the warn can be upgraded to throw with a flag.

## Test Strategy

Add a chaos-regression test in `web/pwa/tests/_support/csp.test.ts`:

```typescript
test("attachCSPGuard warns on unexpected exposeBinding error (chaos regression)", () => {
  // Stub that rejects with an unexpected error
  const warnCalls: string[] = [];
  const originalWarn = console.warn;
  console.warn = (...args) => warnCalls.push(args.join(' '));
  
  const failingPage = {
    on() {},
    exposeBinding() { return Promise.reject(new Error("Target page closed")); },
    addInitScript() { return Promise.reject(new Error("Context destroyed")); },
  };
  
  attachCSPGuard(failingPage as never);
  
  // Must wait for promises to settle
  await new Promise(r => setTimeout(r, 10));
  
  console.warn = originalWarn;
  assert.ok(warnCalls.some(w => w.includes('exposeBinding failed')));
  assert.ok(warnCalls.some(w => w.includes('addInitScript failed')));
});

test("attachCSPGuard silently handles already-bound error (expected case)", () => {
  const warnCalls: string[] = [];
  const originalWarn = console.warn;
  console.warn = (...args) => warnCalls.push(args.join(' '));
  
  const alreadyBoundPage = {
    on() {},
    exposeBinding() { return Promise.reject(new Error("already exposed")); },
    addInitScript() { return Promise.resolve(); },
  };
  
  attachCSPGuard(alreadyBoundPage as never);
  await new Promise(r => setTimeout(r, 10));
  
  console.warn = originalWarn;
  assert.ok(!warnCalls.some(w => w.includes('[csp.ts]')), 
            "should NOT warn for expected already-bound error");
});
```

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| False positive warnings on legitimate races | Low | Only warn, don't throw; message clearly identifies source |
| Playwright changes error message text | Low | Pattern match is broad (`already` + `exposed\|bound`) |
| Performance overhead of catch handler | Negligible | Single string check on error path only |

## Change Boundary

- **Modified:** `web/pwa/tests/_support/csp.ts` (refine catch handlers)
- **Added:** Chaos regression rows in `web/pwa/tests/_support/csp.test.ts`
- **Untouched:** All other files
