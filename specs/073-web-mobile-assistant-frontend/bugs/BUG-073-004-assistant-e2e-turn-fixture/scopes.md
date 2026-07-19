# Scopes: BUG-073-004 - Contract-correct transport-hint parity fixture

## Scope 1: Replace reset fixture and isolate shared HTTP identity state

**Status:** In Progress

**Scope-Kind:** test-infrastructure

### Gherkin Scenarios

```gherkin
Scenario: Web and mobile hints preserve visible response parity
  Given the shared HTTP identity state is isolated with its prior row saved
  When equivalent ordinary text turns are sent with web and mobile hints
  Then contract-relevant response fields match and response transport is web

Scenario: Exact-row restoration preserves neighboring identity state
  Given target and unrelated assistant conversation rows exist
  When test isolation runs and cleanup executes
  Then the target row is restored exactly and the unrelated row is unchanged
```

### Implementation Plan

1. Capture RED output from the isolated named tests and package-order run.
2. Add exact-key conversation snapshot/clean/restore test support.
3. Replace `/reset` with an ordinary text turn in the parity test.
4. Add adversarial exact-row preservation coverage.
5. Run focused and package E2E plus impacted Go/Python/check/lint/format gates.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| SCN-BUG073004-001 | e2e-api | `tests/e2e/assistant/transport_hint_parity_test.go` | Web and mobile hints preserve visible response parity | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestAssistantTransportHintParity_WebAndMobileShareResponseShape$'` | Yes |
| SCN-BUG073004-002 | e2e-api | `tests/e2e/assistant/conversation_isolation_test.go` | Exact-row restoration preserves neighboring identity state | focused selector in the assistant E2E command | Yes |
| Assistant package order | e2e-api | `tests/e2e/assistant/` | Entire assistant package executes in package order | exact anchored selector generated from package test declarations and passed to `./smackerel.sh test e2e --go-run` | Yes |
| Impacted Go units | unit | `internal/assistant/httpadapter/`, `internal/testsupport/jssource/`, `web/pwa/tests/` | Full impacted Go regression lane | `./smackerel.sh test unit --go` | No |
| Impacted Python units | unit | `ml/tests/` | Full Python regression lane | `./smackerel.sh test unit --python` | No |
| Quality gates | guard | changed files and packet | Check, lint, format, regression and packet gates | repo CLI plus Bubbles guards | No |

### Definition of Done

- [ ] Pre-fix named regression is RED with raw live-stack output.
- [ ] Package-order reproduction is captured and classified.
- [ ] Production reset/transport-hint semantics are unchanged.
- [ ] SCN-BUG073004-001 - Web and mobile hints preserve visible response parity: ordinary-turn parity accepts both hints, compares contract-relevant fields, and keeps canonical response transport `web`.
- [ ] SCN-BUG073004-002 - Exact-row restoration preserves neighboring identity state: target-row snapshot/restore is exact and an unrelated row is unchanged.
- [ ] Focused and assistant-package E2E pass on the disposable stack.
- [ ] Impacted Go and Python unit suites pass.
- [ ] Check, lint, format, regression guard, and packet gates pass.
- [ ] HTTP dedup production defect is routed to BUG-069-004 without changing its semantics here.

All items remain unchecked until current-session execution evidence is recorded.
