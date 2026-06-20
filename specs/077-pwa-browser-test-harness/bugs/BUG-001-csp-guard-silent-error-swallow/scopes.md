# Scopes: BUG-001 CSP Guard Silent Error Swallowing Fix

Single-scope bug fix.

## Scope 1: Refine CSP Guard Error Handling

**Status:** Done
**Priority:** P0
**Scope-Kind:** bugfix

### Gherkin Scenarios

```gherkin
Scenario: SCN-077-BUG-001-01 — CSP guard warns on unexpected attachment error
  Given a page stub that rejects exposeBinding with "Target page closed"
  When attachCSPGuard is called on the stub
  Then a console.warn is emitted containing "[csp.ts] exposeBinding failed"
  And the guard does NOT swallow the error silently

Scenario: SCN-077-BUG-001-02 — CSP guard silently handles expected already-bound error
  Given a page stub that rejects exposeBinding with "already exposed"
  When attachCSPGuard is called on the stub
  Then no console.warn is emitted
  And the expected re-attach case is handled gracefully
```

### Implementation Plan

1. Add `isAlreadyBoundError(err)` helper function to csp.ts
2. Replace `.catch(() => undefined)` with message-aware handlers
3. Add chaos regression tests to csp.test.ts

### Test Plan

| Row | Scenario | Category | File/Location | Planned test title | Command |
|-----|----------|----------|---------------|-------------------|---------|
| TP-077-BUG-001-01 | SCN-077-BUG-001-01 | unit | `web/pwa/tests/_support/csp.test.ts` | `attachCSPGuard warns on unexpected exposeBinding error (chaos regression)` | `./smackerel.sh test unit` |
| TP-077-BUG-001-02 | SCN-077-BUG-001-02 | unit | `web/pwa/tests/_support/csp.test.ts` | `attachCSPGuard silently handles already-bound error (expected case)` | `./smackerel.sh test unit` |

### Definition of Done

- [x] `isAlreadyBoundError()` helper added with pattern matching for "already" + "exposed|bound"
- [x] `exposeBinding` catch handler refactored to warn on unexpected errors
- [x] `addInitScript` catch handler refactored to warn on unexpected errors
- [x] Chaos regression test (TP-077-BUG-001-01) passes
- [x] Expected-case test (TP-077-BUG-001-02) passes
- [x] Existing csp.test.ts tests still green
- [x] No regression in existing auth_login.spec.ts suite
