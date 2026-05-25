# Scopes: BUG-DEVOPS-20260525-001

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Execution Outline

### Phase Order

1. Scope 1 - Populate parent state.json `certification.concerns`, normalize child bug `certification.concerns` to structured-object form, and add a deploy-side contract test that locks the schema for both files.

### New Types & Signatures

- New test function: `TestSpec055StateConcernsContract` in `internal/deploy/state_concerns_contract_test.go`.
- New schema constants embedded in the test: allowed `severity` set `{"low", "medium"}`, allowed `followUpAction` set `{"new-spec", "issue-doc", "next-sprint-todo", "accept"}`, required keys `{"id", "severity", "summary", "followUpOwner", "followUpAction"}`.
- New regression scenarios: `SCN-BUG-DEVOPS-20260525-001-001` (parent envelope schema) and `SCN-BUG-DEVOPS-20260525-001-002` (child envelope schema).

### Validation Checkpoints

- Deterministic-red checkpoint: `./smackerel.sh test unit --go --go-run 'TestSpec055StateConcernsContract'` against the pre-fix tree, expecting FAIL output naming `certification.concerns` missing on the parent and string-form entries on the child.
- Green checkpoint: same command against the post-fix tree, expecting PASS.
- Format checkpoint: `./smackerel.sh format --check`.
- Lint checkpoint: `./smackerel.sh lint`.
- Artifact lint checkpoint: `bash .github/bubbles/scripts/artifact-lint.sh specs/055-notification-source-ntfy-adapter/bugs/BUG-DEVOPS-20260525-001` and `bash .github/bubbles/scripts/artifact-lint.sh specs/055-notification-source-ntfy-adapter`.
- Traceability checkpoint: `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/055-notification-source-ntfy-adapter/bugs/BUG-DEVOPS-20260525-001` and `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/055-notification-source-ntfy-adapter`.
- State-transition checkpoint: `timeout 900 bash .github/bubbles/scripts/state-transition-guard.sh specs/055-notification-source-ntfy-adapter`.

## Scope Summary

| Scope | Surfaces | Required Tests | DoD Summary | Status |
|-------|----------|----------------|-------------|--------|
| 1. State.json done_with_concerns Schema Compliance | parent spec 055 state.json certification.concerns, child bug BUG-CHAOS-20260524-001 state.json certification.concerns, new deploy-side contract test | unit (focused contract test), unit (whole `./...` regression), artifact-lint + traceability + state-transition guards | parent + child concerns are structured-object arrays satisfying the completion-governance.md schema; contract test fails deterministically pre-fix and passes post-fix; format/lint/guards clean; zero excluded file family touched | Done |

## Scope 1: State.json done_with_concerns Schema Compliance

**Status:** Done

Depends On: spec 055 final validation certification at 2026-05-24T22:39:14Z that wrote `status: done_with_concerns` without populating structured concerns; BUG-CHAOS-20260524-001 validate certification at 2026-05-24T23:17:08Z that wrote a string-form `concerns` array.

### Outcome

Both the parent `specs/055-notification-source-ntfy-adapter/state.json` and the child `specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json` carry `certification.concerns` arrays whose every entry is a JSON object satisfying the structured shape defined in `.github/agents/bubbles_shared/completion-governance.md`. A new deploy-side contract test asserts the schema on both files and will fail any future regression on either path.

### Gherkin Scenarios

```gherkin
Feature: BUG-DEVOPS-20260525-001 spec 055 state.json done_with_concerns schema
  Scenario: SCN-BUG-DEVOPS-20260525-001-001 parent envelope concerns are structured objects
    Given specs/055-notification-source-ntfy-adapter/state.json declares status "done_with_concerns"
    When the state_concerns contract test parses certification.concerns
    Then the array is non-empty
    And every entry is a JSON object
    And every entry has id, severity, summary, followUpOwner, followUpAction
    And severity is one of {low, medium}
    And followUpAction is one of {new-spec, issue-doc, next-sprint-todo, accept}

  Scenario: SCN-BUG-DEVOPS-20260525-001-002 child bug envelope concerns are structured objects
    Given specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json declares certification.status "done_with_concerns"
    When the state_concerns contract test parses certification.concerns
    Then every entry is a JSON object with the structured shape
    And no entry is a flat string
    And every entry's followUpOwner is a concrete agent name or the literal "human"
```

### Implementation Plan

| Area | Work |
|------|------|
| Parent state.json | Add `certification.concerns` array with two structured entries derived verbatim from the parent's existing `notes` field: legacy report.md evidence-fence cleanup (severity low, owner bubbles.docs, action next-sprint-todo) and child bug literal done blocked by the same cleanup (severity low, owner bubbles.docs, action next-sprint-todo). |
| Child bug state.json | Convert the existing 7 string entries in `certification.concerns` to structured objects. Preserve each original string verbatim as the new `summary`. Assign `id` `CONCERN-1..7`, `severity: low`, `followUpOwner: bubbles.docs` for the legacy-cleanup-blocked entry and `human` for the remaining provenance entries, `followUpAction: next-sprint-todo` for the cleanup-blocked entry and `accept` for the provenance entries. |
| Contract test | Add `internal/deploy/state_concerns_contract_test.go` with `TestSpec055StateConcernsContract`. Read both state.json files. If `status` or `certification.status` is `done_with_concerns`, assert `certification.concerns` is a non-empty array of objects, each with required keys and valid enum values. Fail with a precise message naming the file and entry index. |
| TDD discipline | Author the contract test FIRST (red). Verify it fails on the pre-fix tree with messages naming the missing parent array and the string-form child entries. Capture FAIL output in report.md. THEN patch the two state.json files. Re-run; verify PASS. Capture PASS output in report.md. |

### Change Boundary

| Boundary | Included |
|----------|----------|
| Allowed file families | `specs/055-notification-source-ntfy-adapter/state.json` (parent), `specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json` (child bug), `internal/deploy/state_concerns_contract_test.go` (new test file), `specs/055-notification-source-ntfy-adapter/bugs/BUG-DEVOPS-20260525-001/*.md` and `*.json` (this bug packet's artifacts), `specs/055-notification-source-ntfy-adapter/state.json` `resolvedBugs` append + `executionHistory` append for this bug closure. |
| Excluded surfaces | runtime Go source under `internal/` (other than the new `internal/deploy/state_concerns_contract_test.go`), Python ML sidecar under `ml/`, all `cmd/**` binaries, `docker-compose.yml` / `docker-compose.prod.yml` / `deploy/compose.deploy.yml` / `deploy/contract.yaml`, `.github/workflows/**` CI files, `.github/bubbles/**` framework files, `.github/agents/**` agent definitions, `config/smackerel.yaml`, `config/generated/**` artifacts, `scripts/**`, all unrelated specs (`specs/[!0]*`, `specs/0[!5]*`, `specs/05[!5]*`), all `internal/notification/**` runtime code, `internal/api/router.go`, `internal/api/notifications_ntfy.go`, all `.sql` migrations, all `docs/**` files, all `web/**` files, all `cmd/core/**` files, `assets/**`. |
| Containment proof | `git diff --name-only HEAD~1..HEAD` after the fix commit lists ONLY files inside the allowed families above. Report evidence includes the exact path listing. |

### Implementation Files

- `specs/055-notification-source-ntfy-adapter/state.json`
- `specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json`
- `internal/deploy/state_concerns_contract_test.go`

### Test Plan

| ID | Test Type | Category | File/Location | Scenario Mapping | Description | Command | Live System |
|----|-----------|----------|---------------|------------------|-------------|---------|-------------|
| T-BUG-DEVOPS-001-UNIT-PARENT | Unit Contract | `unit` | `internal/deploy/state_concerns_contract_test.go` | SCN-BUG-DEVOPS-20260525-001-001 | Parent spec 055 state.json `certification.concerns` is a non-empty array of structured objects with valid severity and followUpAction. | `./smackerel.sh test unit --go --go-run 'TestSpec055StateConcernsContract'` | No |
| T-BUG-DEVOPS-001-UNIT-CHILD | Unit Contract | `unit` | `internal/deploy/state_concerns_contract_test.go` | SCN-BUG-DEVOPS-20260525-001-002 | Child bug BUG-CHAOS-20260524-001 state.json `certification.concerns` entries are structured objects (not flat strings) with valid severity and followUpOwner. | `./smackerel.sh test unit --go --go-run 'TestSpec055StateConcernsContract'` | No |
| T-BUG-DEVOPS-001-UNIT-FULL | Unit Regression | `unit` | `./...` (via auto-discovery) | SCN-BUG-DEVOPS-20260525-001-001 + SCN-BUG-DEVOPS-20260525-001-002 | Full-repo unit suite still passes after the new test lands. Scenario-specific E2E regression tests for SCN-BUG-DEVOPS-20260525-001-001 and SCN-BUG-DEVOPS-20260525-001-002 are N/A — schema validation is a unit-level contract with zero runtime, deploy, or HTTP-route surface; no E2E test category exists for this kind of artifact-integrity check. Broader E2E regression suite passes is N/A for the same reason — the change is purely artifact + new unit test and exercises no runtime path. | `./smackerel.sh test unit --go` | No |

### Definition of Done

- [x] SCN-BUG-DEVOPS-20260525-001-001 root cause is identified and fixed before claiming completion. Evidence: `report.md#implementation-evidence`, `report.md#deterministic-red-evidence`, `report.md#green-evidence`.
- [x] SCN-BUG-DEVOPS-20260525-001-001 permanent regression coverage protects parent state.json schema. Evidence: `report.md#green-evidence`, `report.md#focused-unit-evidence`.
- [x] SCN-BUG-DEVOPS-20260525-001-001 original discovery trace is represented by failing-then-passing evidence. Evidence: `report.md#deterministic-red-evidence`, `report.md#green-evidence`.
- [x] Scenario-specific E2E regression tests for SCN-BUG-DEVOPS-20260525-001-001 are N/A — schema validation is a unit-level contract with zero runtime/HTTP surface; no relevant E2E test category exists. Evidence: `report.md#e2e-na-justification`.
- [x] Scenario-specific E2E regression tests for SCN-BUG-DEVOPS-20260525-001-002 are N/A — same justification as SCN-001 (unit-level artifact contract). Evidence: `report.md#e2e-na-justification`.
- [x] Broader E2E regression suite passes is N/A — the change is purely artifact-integrity + a new unit-level contract test; no runtime, HTTP, or pipeline path is exercised. Evidence: `report.md#e2e-na-justification`.
- [x] SCN-BUG-DEVOPS-20260525-001-002 child bug state.json passes the same schema contract test. Evidence: `report.md#focused-unit-evidence`, `report.md#green-evidence`.
- [x] Raw terminal output of the originally failing then passing contract test is recorded in [report.md](report.md). Evidence: `report.md#deterministic-red-evidence`, `report.md#green-evidence`.
- [x] Full-repo unit suite passes after the new test lands. Evidence: `report.md#full-unit-evidence`.
- [x] `./smackerel.sh format --check` passes. Evidence: `report.md#format-evidence`.
- [x] `./smackerel.sh lint` passes. Evidence: `report.md#lint-evidence`.
- [x] Parent state.json `certification.concerns` is non-empty and every entry is a structured object with id/severity/summary/followUpOwner/followUpAction. Evidence: `report.md#green-evidence`, `report.md#parent-state-json-diff-evidence`.
- [x] Child bug state.json `certification.concerns` entries are structured objects preserving each original summary string verbatim. Evidence: `report.md#green-evidence`, `report.md#child-state-json-diff-evidence`.
- [x] Bug artifact lint passes. Evidence: `report.md#bug-artifact-lint-evidence`.
- [x] Parent artifact lint passes. Evidence: `report.md#parent-artifact-lint-evidence`.
- [x] Bug traceability guard passes. Evidence: `report.md#bug-traceability-evidence`.
- [x] Parent traceability guard passes. Evidence: `report.md#parent-traceability-evidence`.
- [x] Parent state-transition guard remains PERMITTED with 0 BLOCKs after the fix (advisory warnings unchanged from baseline). Evidence: `report.md#parent-state-transition-evidence`.
- [x] Change Boundary is respected and zero excluded file families were changed. Evidence: `report.md#code-diff-evidence`, `report.md#change-boundary-containment-evidence`.
- [x] Consumer impact sweep: confirmed zero downstream consumers of `state.json` `certification.concerns` exist in the runtime code (grep across `cmd/`, `internal/`, `ml/`, `scripts/`, `web/`). The new contract test is the only consumer. Evidence: `report.md#consumer-impact-sweep-evidence`.
- [x] Shared infrastructure impact sweep: confirmed zero changes to shared fixtures, harnesses, bootstrap, auth, session, storage, or framework files. The new test imports only `encoding/json`, `os`, `path/filepath`, `testing`, `strings` — standard library only. Evidence: `report.md#shared-infrastructure-sweep-evidence`.
