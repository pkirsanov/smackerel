# Report: [BUG-073-006] Auth Rejection Leaves Blank Assistant Response

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Summary

Planning artifacts only were initialized on 2026-07-23 and reconciled by `bubbles.plan` on
2026-07-24 against the current [spec.md](spec.md) (10 scenarios, `ASST-UI-001`..`ASST-UI-012`)
and [design.md](design.md). The reconciliation restored `SCN-073-006-10` (assistant fault
profiles consumed from the `BUG-102-001`-owned production-inert, test-only, machine-readable
fault-profile registry), split the plan into two scopes (SCOPE-01 CONSUMES the `BUG-102-001`
registry foundation by `stableId`; SCOPE-02 is the honest-terminal-state fix), and bound the
assistant surface to `BUG-070-001`'s unified claim-bound PASETO session. Realigned on
2026-07-24 to close independent-review finding F1 (MEDIUM): SCOPE-01 is a pure consumer of the
`BUG-102-001` registry — no parallel registry, no nine-field-schema fork. No source, test, HTTP,
production, commit, push, or deployment mutation occurred.

## Completion Statement

Incomplete and non-terminal. Status remains `in_progress`; no owner design, implementation, test, validation, or audit completion is claimed.

## Bug Reproduction - Before Fix

- **Claim Source:** interpreted historical input.
- **Executed by this invocation:** no.
- **Input preserved:** an Assistant POST rejected by auth before facade execution leaves the user message paired with a blank response and no actionable error/retry.
- **Evidence status:** no browser/network/DOM/server output was captured here.

## Decision Record

- A blank terminal node is invalid for every failure class.
- Failure must not be rewritten as capture or success.
- Retry and transcript preservation are part of the same outcome contract.

## Code Diff Evidence

Not applicable to this planning-only invocation.

## Test Evidence

**Phase:** planning  
**Command:** none  
**Exit Code:** not applicable  
**Claim Source:** not-run

No test result is claimed.

## Uncertainty Declarations

- Exact client reducer/render path and envelope parser are not locally confirmed.
- No red or green regression output exists.

## Scenario Contract Evidence

Contract initialized in [scenario-manifest.json](scenario-manifest.json); runtime evidence
references are empty (planning-only — no test has been executed). The table below records the
planned test home for each scenario. Every referenced file EXISTS on disk today and is the
existing test that will host the scenario's real-stack (or unit) coverage; no new test file is
asserted to exist yet.

| Scenario | Scope | Planned test home (existing file) |
|---|---|---|
| SCN-073-006-01 | SCOPE-02 | web/pwa/tests/assistant_chat.spec.ts |
| SCN-073-006-02 | SCOPE-02 | web/pwa/tests/assistant_retry.spec.ts |
| SCN-073-006-03 | SCOPE-02 | web/pwa/tests/assistant_retry.spec.ts |
| SCN-073-006-04 | SCOPE-02 | web/pwa/tests/assistant_retry.spec.ts |
| SCN-073-006-05 | SCOPE-02 | tests/e2e/assistant/web_pwa_retry_e2e_test.go |
| SCN-073-006-06 | SCOPE-02 | web/pwa/tests/assistant_chat.spec.ts |
| SCN-073-006-07 | SCOPE-02 | web/pwa/tests/assistant_chat.spec.ts |
| SCN-073-006-08 | SCOPE-02 | web/pwa/tests/assistant_storage_guard_test.go |
| SCN-073-006-09 | SCOPE-02 | web/pwa/tests/assistant_accessibility.spec.ts |
| SCN-073-006-10 | SCOPE-01 | tests/e2e/assistant/http_error_test.go |

Supporting (non-scenario) coverage homes, also existing files:
tests/e2e/assistant/http_live_stack_test.go,
tests/integration/policy/no_defaults_go_guard_test.go,
tests/config/assistant_config_generate_test.sh,
web/pwa/tests/assistant_robustness_guard_test.go,
tests/integration/api/assistant_http_auth_test.go,
cmd/core/wiring_assistant_http_prefacade_regression_test.go,
tests/e2e/assistant_regression_e2e_test.sh.

## Validation Summary

No completion validation or certification was performed.

## Audit Verdict

Not audited. No terminal verdict is claimed.
