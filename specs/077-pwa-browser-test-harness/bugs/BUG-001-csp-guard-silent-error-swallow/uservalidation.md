# User Validation: BUG-001 CSP Guard Silent Error Swallowing Fix

## Validation Criteria

1. **Expected Error Case**: When `exposeBinding` throws "already exposed" (page re-use), no warning is emitted and tests pass silently
2. **Unexpected Error Case**: When `exposeBinding` throws any other error, a console.warn is emitted so operators see the issue in test output
3. **Regression Prevention**: Existing auth_login.spec.ts and other CSP-guarded tests continue to pass

## Validation Steps

1. Run `./smackerel.sh test unit` and verify csp.test.ts passes including new chaos regression tests
2. Run `./smackerel.sh test e2e-ui` (if test stack available) and verify no unexpected warnings in normal operation
3. Inject a simulated page-close error and verify warning appears in output

## Sign-off

- [x] Unit tests pass (csp.test.ts) — 6 tests including 2 chaos regression tests all pass
- [x] Integration verified (no regression) — spec_077_playwright_config_fail_loud_test.sh passes
- [x] Code review complete — Fix adds isAlreadyBoundError() helper, refines catch handlers
