# User Validation: BUG-031-007 R8 sweep test trigger artifact fidelity drift

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Checklist

- [x] All three R8 test-trigger findings (T-031-001, T-031-002, T-031-003) closed by IDE-tool edits to two production-artifact files (`specs/031-live-stack-testing/scenario-manifest.json`, `specs/031-live-stack-testing/state.json`) and an 8-artifact BUG-031-007 packet.
- [x] `state-transition-guard.sh` PASS on both `specs/031-live-stack-testing/` and the BUG-031-007 packet.
- [x] `artifact-lint.sh` PASS on both `specs/031-live-stack-testing/` and the BUG-031-007 packet.
- [x] Compile sweep (`go vet` + `go build` with `integration,stress` tags) remains EXIT=0 across `tests/integration/...`, `tests/e2e/...`, `tests/stress/...`, `internal/api/...`.
- [x] Change boundary respected: only the 10 paths under the BUG packet + parent state.json + scenario-manifest.json staged for commit. Spec 055 ntfy adapter WIP remained unstaged.
- [x] Structured commit landed with Check 17 prefix `spec(031,bug-031-007): close R8 sweep test trigger artifact-fidelity drift findings`.

## Stakeholder Acceptance

- **Sweep parent (`bubbles.workflow`, sweep-2026-05-23-r30 round 8):** R8 round terminates `done`/`resolved` with state-transition-guard PASS verification once the three artifact-fidelity findings close.
- **Spec 031 owner:** Parent state.json `activeBugs`/`resolvedBugs` reflects truth, scenario-manifest references only real symbols, and new SLA stress test surface is discoverable from the manifest.
- **Future agents:** A scenario-manifest fidelity audit script run against `specs/031-live-stack-testing/scenario-manifest.json` returns zero `FUNC-MISSING` findings, eliminating a class of false-positive trace-guard noise.

## Acceptance Criteria

- [x] T-031-001 closed: SCN-LST-005 `linkedTests` no longer references `test_integration` in `scripts/runtime/go-integration.sh`.
- [x] T-031-002 closed: parent state.json shows `BUG-031-006-strict-guard-gate-drift` in `resolvedBugs` only.
- [x] T-031-003 closed: SCN-LST-004 `linkedTests` includes `TestMLReadinessTimeoutBoundary`, `TestMLReadinessTimeoutSilentBypass`, `TestMLReadinessAlways200Regression` in `tests/stress/ml_readiness_timeout_stress_test.go`, and `evidenceRefs` includes a `stress-test` entry for the same file.
- [x] `state-transition-guard.sh specs/031-live-stack-testing` exits 0 (TRANSITION PERMITTED) after BUG-031-007 closure.
- [x] `artifact-lint.sh` exits 0 on both `specs/031-live-stack-testing` and the BUG packet.
- [x] Single structured commit landed with prefix `spec(031,bug-031-007): ...`.

## Out of Scope (Not Validated Here)

- Live-stack `./smackerel.sh test integration` execution. The change manifest is artifact-edit only and does not modify the integration test runner or any test file.
- Spec 055 ntfy adapter WIP delivery.
