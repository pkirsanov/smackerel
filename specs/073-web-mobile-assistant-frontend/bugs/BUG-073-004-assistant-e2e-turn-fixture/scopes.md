# Scopes: BUG-073-004 - Contract-correct transport-hint parity fixture

## Scope 1: Replace reset fixture and isolate shared HTTP identity state

**Status:** In Progress

**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: Web and mobile hints preserve visible response parity (SCN-BUG073004-001)
  Given the shared HTTP identity state is isolated with its prior row saved
  When equivalent ordinary text turns are sent with web and mobile hints
  Then contract-relevant response fields match and response transport is web

Scenario: Exact-row restoration preserves neighboring identity state (SCN-BUG073004-002)
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

### Implementation Files

- `tests/e2e/assistant/transport_hint_parity_test.go`
- `tests/e2e/assistant/conversation_isolation_test.go`
- `specs/073-web-mobile-assistant-frontend/bugs/BUG-073-004-assistant-e2e-turn-fixture/`

### Change Boundary

**Allowed file families:** the two assistant E2E files listed under Implementation Files and this BUG-073-004 packet.

**Excluded surfaces:** `internal/assistant/facade.go`, `internal/assistant/httpadapter/adapter.go`, production reset/transport-hint/dedup semantics, shared assistant package setup outside the two listed tests, web/mobile runtime source, config, deployment, secrets, and release-train artifacts.

### Shared Infrastructure Impact Sweep

- **Downstream contracts:** conversation-key identity, target-row snapshot timing, exact target-row restoration, preservation of neighboring rows, ordinary-turn ordering, and assistant package execution order.
- **Blast radius:** limited to the parity and exact-row isolation E2E fixtures; production auth/session/context semantics and unrelated assistant tests remain unchanged.
- **Independent canary:** run only the parity and exact-row restoration tests on the disposable stack before the assistant package selector.
- **Restore path:** the fixture saves the exact target row before mutation, restores that row during cleanup, and verifies an unrelated row is byte-for-byte unchanged; removing the two test-file changes restores the prior harness behavior.

### Test Plan

| Test Type | ID | Category | File/Location | Description | Command | Live System |
|---|---|---|---|---|---|---|
| Pre-fix focused regression | `TP-BUG073004-000` | `e2e-api` | `tests/e2e/assistant/transport_hint_parity_test.go` | The ordinary-turn sentinel fails while the stale `/reset` fixture remains | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestAssistantTransportHintParity_WebAndMobileShareResponseShape$'` | Yes |
| Regression E2E: SCN-BUG073004-001 | `TP-BUG073004-001` | `e2e-api` | `tests/e2e/assistant/transport_hint_parity_test.go` | `TestAssistantTransportHintParity_WebAndMobileShareResponseShape` proves web/mobile ordinary-turn response parity | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestAssistantTransportHintParity_WebAndMobileShareResponseShape$'` | Yes |
| Regression E2E: SCN-BUG073004-002 | `TP-BUG073004-002` | `e2e-api` | `tests/e2e/assistant/conversation_isolation_test.go` | `TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial` proves exact target restoration and neighbor preservation | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial$'` | Yes |
| Fixture Canary: parity plus exact-row restoration | `TP-BUG073004-014` | `e2e-api` | `tests/e2e/assistant/transport_hint_parity_test.go`, `tests/e2e/assistant/conversation_isolation_test.go` | Independent shared-fixture canary validates identity, ordering, restoration, and neighboring-row contracts before broad package execution | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^(TestAssistantTransportHintParity_WebAndMobileShareResponseShape|TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial)$'` | Yes |
| Broader E2E regression | `TP-BUG073004-003` | `e2e-api` | `tests/e2e/assistant/` | The complete assistant package executes in package order and preserves the repaired scenarios | exact anchored selector generated from package test declarations and passed to `./smackerel.sh test e2e --go-run` | Yes |
| Impacted Go units | `TP-BUG073004-004` | `unit` | `internal/assistant/httpadapter/`, `internal/testsupport/jssource/`, `web/pwa/tests/` | Full impacted Go regression lane | `./smackerel.sh test unit --go` | No |
| Impacted Python units | `TP-BUG073004-005` | `unit` | `ml/tests/` | Full Python regression lane | `./smackerel.sh test unit --python` | No |
| Configuration check | `TP-BUG073004-006` | `guard` | Repository config contract | SST and generated config remain valid | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check` | No |
| Lint | `TP-BUG073004-007` | `guard` | Changed files | Repository lint reports no warnings | `./smackerel.sh lint` | No |
| Format | `TP-BUG073004-008` | `guard` | Changed Go and packet files | Repository format check remains clean | `./smackerel.sh format --check` | No |
| Adversarial regression guard | `TP-BUG073004-009` | `guard` | `tests/e2e/assistant/transport_hint_parity_test.go`, `tests/e2e/assistant/conversation_isolation_test.go` | Required regressions contain no bailout or tautological bugfix pattern | `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/assistant/transport_hint_parity_test.go tests/e2e/assistant/conversation_isolation_test.go` | No |
| Artifact lint | `TP-BUG073004-010` | `artifact` | BUG-073-004 packet | Packet template and evidence structure remain valid | `bash .github/bubbles/scripts/artifact-lint.sh specs/073-web-mobile-assistant-frontend/bugs/BUG-073-004-assistant-e2e-turn-fixture` | No |
| Traceability | `TP-BUG073004-011` | `artifact` | BUG-073-004 packet | Gherkin, scenario manifest, tests, and DoD remain linked | `bash .github/bubbles/scripts/traceability-guard.sh specs/073-web-mobile-assistant-frontend/bugs/BUG-073-004-assistant-e2e-turn-fixture` | No |
| Implementation reality | `TP-BUG073004-012` | `artifact` | BUG-073-004 packet | Referenced implementation files contain no stub/fake/default regressions | `bash .github/bubbles/scripts/implementation-reality-scan.sh specs/073-web-mobile-assistant-frontend/bugs/BUG-073-004-assistant-e2e-turn-fixture --verbose` | No |
| State-transition guard | `TP-BUG073004-013` | `artifact` | BUG-073-004 packet | The packet reports exact remaining owner-routed findings | `bash .github/bubbles/scripts/state-transition-guard.sh specs/073-web-mobile-assistant-frontend/bugs/BUG-073-004-assistant-e2e-turn-fixture` | No |

### Definition of Done

- [ ] `TP-BUG073004-000` - Pre-fix named regression is RED with raw live-stack output.
- [ ] Production reset/transport-hint semantics are unchanged.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior pass (`TP-BUG073004-001` / SCN-BUG073004-001: ordinary-turn parity accepts both hints, compares contract-relevant fields, and keeps canonical response transport `web`).
- [ ] `TP-BUG073004-002` / SCN-BUG073004-002 - Exact-row restoration preserves neighboring identity state: target-row snapshot/restore is exact and an unrelated row is unchanged.
- [ ] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns (`TP-BUG073004-014`).
- [ ] Rollback or restore path for shared infrastructure changes is documented and verified.
- [ ] Broader E2E regression suite passes (`TP-BUG073004-003`: complete assistant package in package order).
- [ ] `TP-BUG073004-004` - Impacted Go unit suite passes.
- [ ] `TP-BUG073004-005` - Impacted Python unit suite passes.
- [ ] `TP-BUG073004-006` - Configuration check passes.
- [ ] `TP-BUG073004-007` - Lint passes with no warnings.
- [ ] `TP-BUG073004-008` - Format check passes.
- [ ] `TP-BUG073004-009` - Adversarial regression guard passes with no silent-pass or tautological patterns.
- [ ] `TP-BUG073004-010` - Artifact lint passes.
- [ ] `TP-BUG073004-011` - Traceability guard passes.
- [ ] `TP-BUG073004-012` - Implementation-reality scan passes.
- [ ] `TP-BUG073004-013` - State-transition guard records the exact remaining owner-routed findings.
- [ ] Change Boundary is respected and zero excluded file families were changed.
- [ ] HTTP dedup production defect is routed to BUG-069-004 without changing its semantics here.

All items remain unchecked until current-session execution evidence is recorded.
