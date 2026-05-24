# User Validation: BUG-019-002 — DoD scenario fidelity gap (SCN-019-004 + SCN-019-005)

> Spec: [spec.md](spec.md) | Design: [design.md](design.md) | Scopes: [scopes.md](scopes.md) | Report: [report.md](report.md)

---

## Acceptance Checklist

- [x] **AC-01:** Parent `specs/019-connector-wiring/scopes.md` Scope 1 DoD contains a new `Scenario SCN-019-004 (Config entries exist for all 5 connectors in smackerel.yaml): …` bullet preserving all 9 of the scenario's significant words (`scn / 019 / 004 / config / entries / exist / connectors / smackerel / yaml`).
  - **Verified by:** `grep -n "Scenario SCN-019-004" specs/019-connector-wiring/scopes.md` returns the new bullet (single match).
- [x] **AC-02:** Parent `specs/019-connector-wiring/scopes.md` Scope 1 DoD contains a new `Scenario SCN-019-005 (Health endpoint shows all 15 connectors): …` bullet preserving all 7 of the scenario's significant words (`scn / 019 / 005 / health / endpoint / shows / connectors`).
  - **Verified by:** `grep -n "Scenario SCN-019-005" specs/019-connector-wiring/scopes.md` returns the new bullet (single match).
- [x] **AC-03:** No pre-existing DoD bullet in `specs/019-connector-wiring/scopes.md` is deleted, weakened, or rewritten. (Additive-only fix.)
  - **Verified by:** `git diff specs/019-connector-wiring/scopes.md` shows 2 insertions, 0 deletions, 0 modifications to pre-existing lines.
- [x] **AC-04:** `bash .github/bubbles/scripts/artifact-lint.sh specs/019-connector-wiring` returns exit 0.
  - **Verified by:** `report.md` `### Audit Evidence → Parent artifact-lint` block (terminal output captured, exit 0).
- [x] **AC-05:** `bash .github/bubbles/scripts/artifact-lint.sh specs/019-connector-wiring/bugs/BUG-019-002-scn-019-004-005-fidelity-gap` returns exit 0.
  - **Verified by:** `report.md` `### Closure Verification` block.
- [x] **AC-06:** `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/019-connector-wiring` returns `RESULT: PASSED (0 warnings)` with `DoD fidelity: 6 scenarios checked, 6 mapped to DoD, 0 unmapped`.
  - **Verified by:** `report.md` `### Post-fix Validation Evidence` block (full guard tail captured, exit 0).
- [x] **AC-07:** `bash tests/integration/test_connector_wiring.sh` returns exit 0 with last line `SCN-019-004: PASS` (32 PASS / 0 FAIL).
  - **Verified by:** `report.md` `### Test Evidence → SCN-019-004 underlying test (integration)` block.
- [x] **AC-08:** `go test -count=1 -run TestHealthHandler_ConnectorHealth ./internal/api/...` returns `PASS` at exit 0.
  - **Verified by:** `report.md` `### Test Evidence → SCN-019-005 underlying test (unit)` block.
- [x] **AC-09:** Production code unchanged. `git diff --name-only` lists no entries under `internal/`, `cmd/`, `ml/`, `config/`, `scripts/`, or `tests/`.
  - **Verified by:** `report.md` `### Audit Evidence → Boundary preservation` block.
- [x] **AC-10:** Adversarial inverse holds: removing the 2 new DoD bullets reproduces the pre-fix `RESULT: FAILED (3 failures, 0 warnings)`.
  - **Verified by:** `report.md` `### Adversarial Inverse Verification` block (logical inverse argument grounded in the matcher's deterministic per-bullet scoring).

## User Acceptance

This is a documentation-fidelity fix initiated by the stochastic-quality-sweep round 28 test trigger. The underlying behavior (config entries for all 5 connectors; health endpoint listing 15 connectors) was already delivered and tested at the original spec 019 close-out. The fix only restores the trace-ID/word-fidelity linkage that the v3.8.0 G068 matcher requires. No user-facing behavior changes, no API changes, no configuration changes.

**Accepted by:** sweep-2026-05-23-r30 round 28 owner (bubbles.workflow parent-expanded-child-mode execution); acceptance criteria checklist above is fully satisfied.
