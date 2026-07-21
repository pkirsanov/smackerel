# Scopes: BUG-073-004 - Contract-correct transport-hint parity fixture

## Scope 1: Replace reset fixture and isolate shared HTTP identity state

**Status:** Done

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

- [x] `TP-BUG073004-000` - Pre-fix named regression is RED with raw live-stack output. → Evidence: [report.md](report.md) "RED: Revert-Reverify — load-bearing parity fixture (current session)" — reverting the fixture to `/reset` makes the live-stack parity test FAIL at `transport_hint_parity_test.go:157` (`hint="web" parity fixture reached the /reset short circuit; parity requires an ordinary text turn`), `UNIT_RED_REVERT_EXIT=1`, clean teardown. The RED needs the real facade to process `/reset` and return `context reset.`.
- [x] Production reset/transport-hint semantics are unchanged. → Evidence: [report.md](report.md) "### Code Diff Evidence" — `git show c5ddf562 --numstat` and `git diff --name-status c5ddf562^ c5ddf562` show the fix touches ONLY the two assistant E2E test files (`conversation_isolation_test.go` A +243, `transport_hint_parity_test.go` M +17/−1); no `internal/assistant/facade.go` or `internal/assistant/httpadapter/adapter.go` change. [design.md](design.md) "Production Boundary" confirms production reset tracing, HTTP transport naming (`web`), and telemetry-only `transport_hint` are unchanged.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior pass (`TP-BUG073004-001` / SCN-BUG073004-001: ordinary-turn parity accepts both hints, compares contract-relevant fields, and keeps canonical response transport `web`). → Evidence: [report.md](report.md) "GREEN: Revert-Reverify restore + focused canary" — `--- PASS: TestAssistantTransportHintParity_WebAndMobileShareResponseShape (18.36s)` on the byte-exact-restored tree, and again `(10.05s)` in the broad package; the `requireNormalParityTurn` sentinel now finds a real `assistant_turn_id` and `shapeOnly`+`DeepEqual` compares contract-relevant fields with canonical response transport `web`.
- [x] `TP-BUG073004-002` / SCN-BUG073004-002 - Exact-row restoration preserves neighboring identity state: target-row snapshot/restore is exact and an unrelated row is unchanged. → Evidence: [report.md](report.md) "GREEN: Revert-Reverify restore + focused canary" — `--- PASS: TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial (0.02s)` in the focused canary and again in the broad package; the adversary seeds a target + neighbor row, isolates only the target `(user_id, transport)` key, `reflect.DeepEqual` asserts the target row is restored exactly and the neighbor row is byte-for-byte unchanged.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns (`TP-BUG073004-014`). → Evidence: [report.md](report.md) "GREEN: Revert-Reverify restore + focused canary" — the focused canary `./smackerel.sh test e2e --go-run '^(TestAssistantTransportHintParity_WebAndMobileShareResponseShape|TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial)$'` is GREEN (both tests PASS, `UNIT_GREEN_RESTORE_EXIT=0`) and ran BEFORE the broad assistant-package rerun.
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. → Evidence: [design.md](design.md) "Rollback" documents reverting the test/helper changes (no schema, runtime config, deployment, or persisted production state involved); VERIFIED this session — [report.md](report.md) "GREEN…" shows `git checkout HEAD -- tests/e2e/assistant/transport_hint_parity_test.go` restored the file byte-exact (`restore_rc=0`, `git status --short` empty) and returned the parity + isolation tests to GREEN; the fixture's own `restore()` path is adversarially proven by TP-BUG073004-002.
- [x] Broader E2E regression suite passes (`TP-BUG073004-003`: complete assistant package in package order). → Evidence: [report.md](report.md) "Broader Assistant-Package Regression (current session)" — the complete assistant package runs in package order with **67 PASS** including the repaired `TestAssistantConversationIsolation_...Adversarial` and `TestAssistantTransportHintParity_...` (+ its adversarial); the ONLY 2 failures are pre-existing FOREIGN `buildvcs` failures in `intent_replay_test.go` (spec-069), dispositioned [report.md](report.md) "## Discovered Issues (Gate G095)" DI-073-004-01 — outside this change boundary, working tree packet-only, not a product regression.
- [x] `TP-BUG073004-004` - Impacted Go unit suite passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `./smackerel.sh test unit --go` → `FULL_GO_UNITS_EXIT=0` (`[go-unit] go test ./... finished OK`, 0 failures).
- [x] `TP-BUG073004-005` - Impacted Python unit suite passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `./smackerel.sh test unit --python` → `FULL_PY_UNITS_EXIT=0` (`708 passed, 2 deselected`).
- [x] `TP-BUG073004-006` - Configuration check passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check` → `CHECK_EXIT=0` (config in sync; env_file drift guard OK; scenario-lint OK, 17 registered).
- [x] `TP-BUG073004-007` - Lint passes with no warnings. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `./smackerel.sh lint` → `LINT_EXIT=0` (`All checks passed!` + `Web validation passed`).
- [x] `TP-BUG073004-008` - Format check passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `./smackerel.sh format --check` → `FORMAT_EXIT=0` (`75 files already formatted`).
- [x] `TP-BUG073004-009` - Adversarial regression guard passes with no silent-pass or tautological patterns. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `regression-quality-guard.sh --bugfix tests/e2e/assistant/transport_hint_parity_test.go tests/e2e/assistant/conversation_isolation_test.go` → `REGGUARD_EXIT=0` (adversarial signal detected in `transport_hint_parity_test.go` via `TestAssistantTransportHintParity_AdversarialDivergentBodiesDetected`; 0 violations, 0 warnings).
- [x] `TP-BUG073004-010` - Artifact lint passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `artifact-lint.sh <bug-dir>` → `ARTLINT_EXIT=0` (Artifact lint PASSED); re-run against the reconciled packet in the promote sequence.
- [x] `TP-BUG073004-011` - Traceability guard passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `traceability-guard.sh <bug-dir>` → `TRACE_EXIT=0` (2 scenarios linked to Test Plan rows; G057/G068 fidelity 2/2; PASSED).
- [x] `TP-BUG073004-012` - Implementation-reality scan passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `implementation-reality-scan.sh <bug-dir> --verbose` → `IMPLREALITY_EXIT=0` (0 violations, 0 warnings, 2 files).
- [x] `TP-BUG073004-013` - State-transition guard records the exact remaining owner-routed findings. → Evidence: [report.md](report.md) "### Validation Evidence" — the state-transition guard runs and records the exact remaining owner-routed findings as the EMPTY set (verdict PASS, `failedGateIds: []`, exit 0); the only broader-suite failures are the foreign `buildvcs` issue dispositioned DI-073-004-01 (G095), not owner-routed findings for this packet.
- [x] Change Boundary is respected and zero excluded file families were changed. → Evidence: [report.md](report.md) "### Code Diff Evidence" — `git show c5ddf562 --numstat` lists exactly the allowed files (`tests/e2e/assistant/transport_hint_parity_test.go`, `tests/e2e/assistant/conversation_isolation_test.go`); `git status --short` is packet-only after the byte-exact revert restore. No excluded surface (`internal/assistant/facade.go`, `internal/assistant/httpadapter/adapter.go`, production reset/transport-hint/dedup semantics, shared assistant setup, web/mobile runtime, config, deployment, secrets, release-train artifacts) was changed.
- [x] HTTP dedup production defect is routed to BUG-069-004 without changing its semantics here. → Evidence: [bug.md](bug.md) and [spec.md](spec.md) assign the deterministic-ordinary-weather HTTP dedup defect to its owning packet `specs/069-assistant-http-transport/bugs/BUG-069-004-http-turn-dedup` (outside BUG-073-004's boundary); [report.md](report.md) "## Open Findings" confirms HTTP response dedup ownership sits with BUG-069-004. This packet's Code Diff Evidence (two E2E test files only) changed no dedup / trace-id / reset semantics.

All 19 DoD items are closed with current-session execution evidence recorded in [report.md](report.md).
