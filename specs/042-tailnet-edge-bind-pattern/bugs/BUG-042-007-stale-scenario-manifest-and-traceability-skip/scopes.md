# Scopes: BUG-042-007 — Stale scenario-manifest.json + traceability-guard skip

## Scope 1: Reconcile spec 042 scenario manifest and activate the traceability guard (single-scope bugfix-fastlane)

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: SCN-BUG-042-007-001 — Active scopes.md scenarios reformatted so extract_scenarios finds them
  Given specs/042-tailnet-edge-bind-pattern/scopes.md used "- **SCN-042-NNN - title**" bullets
  And traceability-guard.sh extract_scenarios greps "^[[:space:]]*Scenario:" and found 0 scenarios
  When the active SCN-042-001..006 are reformatted to "Scenario: SCN-042-NNN — title" inside gherkin blocks
  And the two stale HTML-commented duplicate "## Scope N:" headings are relabeled "## Superseded Scope N —"
  Then grep -cE "^[[:space:]]*Scenario( Outline)?:" scopes.md returns 6
  And the fail-loud Given/When/Then semantics are preserved with no :-127.0.0.1 form reintroduced

Scenario: SCN-BUG-042-007-002 — scenario-manifest.json realigned to the active fail-loud scopes
  Given the manifest carried the forbidden ${HOST_BIND_ADDRESS:-127.0.0.1} form in SCN-042-001 then
  And SCN-042-003 was titled "Compose default is safe for local runs" with 004/005 titles shuffled
  When all six manifest entries are realigned to the active scopes
  Then grep -rn "HOST_BIND_ADDRESS:-" scenario-manifest.json returns nothing
  And SCN-042-003 is titled "Missing bind address fails loud"
  And requiredTestType is unit for the compose-contract scenarios and doc-lint for the doc scenarios
  And the linked test IDs are real TestComposeContract_* function names

Scenario: SCN-BUG-042-007-003 — traceability-guard goes from silent exit 1 to exit 0 with the cross-check ACTIVE
  Given traceability-guard.sh exited 1 with "scenario manifest cross-check skipped" (0 scenarios found)
  When the guard is re-run after the reformat and manifest realignment
  Then traceability-guard.sh specs/042-tailnet-edge-bind-pattern exits 0
  And the G057/G059 Scenario Manifest Cross-Check is ACTIVE (covers 6 scenario contracts)
  And every scenario maps to a Test Plan row, a concrete test file, report evidence, and a DoD item (G068 6/6)

Scenario: SCN-BUG-042-007-004 — artifact-lint stays PASSED after the edits
  Given artifact-lint.sh specs/042-tailnet-edge-bind-pattern was PASSED before this packet
  When the scopes.md and scenario-manifest.json edits land
  Then artifact-lint.sh specs/042-tailnet-edge-bind-pattern still returns PASSED (exit 0)
  And all DoD checkboxes are checked and all report.md evidence blocks are legitimate

Scenario: SCN-BUG-042-007-005 — Deployment surface intact (compose contract unchanged)
  Given BUG-042-007 changes zero runtime behaviour (planning-artifact reconcile only)
  And deploy/compose.deploy.yml and internal/deploy/compose_contract_test.go are not modified
  When ./smackerel.sh test unit --go --go-run "TestComposeContract" runs
  Then all nine TestComposeContract_* functions PASS and internal/deploy is ok
  And the fail-loud bind contract + infra-no-ports + adversarial coverage stay GREEN by construction
```

### Test Plan

| Type | Scenario | Test Functions / Targets | Test Files |
|------|----------|--------------------------|------------|
| Guard-verification | SCN-BUG-042-007-001 | `grep -cE '^[[:space:]]*Scenario' scopes.md` == 6; `extract_scenarios` non-empty | `.github/bubbles/scripts/traceability-guard.sh` against `specs/042-tailnet-edge-bind-pattern` |
| Guard-verification | SCN-BUG-042-007-002 | `grep -rn 'HOST_BIND_ADDRESS:-' scenario-manifest.json` empty; titles + requiredTestType realigned | `specs/042-tailnet-edge-bind-pattern/scenario-manifest.json` |
| Guard-verification | SCN-BUG-042-007-003 | `traceability-guard.sh` exit 0, cross-check ACTIVE (6 contracts, 6/6 G068) | `.github/bubbles/scripts/traceability-guard.sh` against `specs/042-tailnet-edge-bind-pattern` |
| Guard-verification | SCN-BUG-042-007-004 | `artifact-lint.sh` exit 0 (PASSED) | `.github/bubbles/scripts/artifact-lint.sh` against `specs/042-tailnet-edge-bind-pattern` |
| Regression E2E | SCN-BUG-042-007-005 | `TestComposeContract_LiveFile, TestComposeContract_AdversarialLiteralBind, TestComposeContract_AdversarialInfraHasPorts, TestComposeContract_AdversarialMultiPortsBypass, TestComposeContract_AdversarialMLMultiPortsBypass, TestComposeContract_AdversarialNetworkModeHostBypass, TestComposeContract_AdversarialOllamaLiteralBind, TestComposeContract_AdversarialDefaultFallbackBind, TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms` | `internal/deploy/compose_contract_test.go` |

### Scenario-First TDD Evidence

This packet was scenario-first authored (red→green discipline preserved): the
**red** executable proof captured BEFORE the fix is `traceability-guard.sh` exit 1
with "scenario manifest cross-check skipped" (0 scenarios found); the **green**
proof captured AFTER is exit 0 with the G057/G059 cross-check ACTIVE (6 contracts,
6/6 scenario→row→file→report, 6/6 G068 DoD fidelity). The deployment-intact claim
is backed by a real `./smackerel.sh test unit --go --go-run TestComposeContract`
run (nine `TestComposeContract_*` PASS, `ok internal/deploy`). All output is
recorded in `report.md` Test Evidence + Validation Evidence.

### Change Boundary

This scope is a **refactor/repair** (planning-artifact reconcile, zero runtime
change). Containment is strict.

**Allowed file families (the ONLY paths this scope may touch):**

- `specs/042-tailnet-edge-bind-pattern/scopes.md` (reformat scenarios + relabel stale headings + bold subheadings + DoD trace IDs)
- `specs/042-tailnet-edge-bind-pattern/scenario-manifest.json` (realign 6 entries)
- `specs/042-tailnet-edge-bind-pattern/report.md` (Planning-Artifact Reconciliation section)
- `specs/042-tailnet-edge-bind-pattern/state.json` (bubbles.plan executionHistory + resolvedBugs + lastUpdatedAt)
- `specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-007-stale-scenario-manifest-and-traceability-skip/` (all 8 packet artifacts)

**Excluded surfaces (this scope MUST NOT touch any of these):**

- `deploy/compose.deploy.yml` (the fail-loud bind contract — correct, verify-only)
- `internal/deploy/compose_contract_test.go` (the contract test — correct, verify-only)
- `specs/042-tailnet-edge-bind-pattern/spec.md`, `design.md`, `uservalidation.md`
- `cmd/`, `internal/`, `ml/`, `scripts/`, `web/`, `config/`, `.github/workflows/`, `smackerel.sh`
- `.github/bubbles/` framework scripts (immutable)
- Any other spec under `specs/`; the ~130 accumulated uncommitted worktree files

Enumerated consumer surfaces (none — planning-artifact reconcile): `navigation`
n/a, `redirect` n/a, `API client` n/a, `deep link` n/a, `stale-reference` n/a —
the scope makes zero behaviour change so there are no consumers to sweep.

### Definition of Done

- [x] BUG-042-007 packet contains 8 artifacts in `specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-007-stale-scenario-manifest-and-traceability-skip/` (bug.md, spec.md, design.md, scopes.md, scenario-manifest.json, report.md, state.json, uservalidation.md). **Phase:** bootstrap **Evidence:** reconcile — `ls -1` lists all 8 files; captured in report.md Implementation Code Diff Evidence. **Claim Source:** executed
- [x] Change Boundary is respected; only artifact paths under `specs/042-tailnet-edge-bind-pattern/` are touched; `deploy/compose.deploy.yml` and `internal/deploy/compose_contract_test.go` are unchanged. **Phase:** implement **Evidence:** reconcile — `git status --short deploy/ internal/deploy` empty; captured in report.md Implementation Code Diff Evidence. **Claim Source:** executed
- [x] Scenario SCN-BUG-042-007-001 (Active scopes.md scenarios reformatted): `grep -cE '^[[:space:]]*Scenario( Outline)?:' scopes.md` returns 6 and the fail-loud semantics are preserved (no `:-127.0.0.1` in the gherkin). **Phase:** implement **Evidence:** reconcile — grep returns 6; captured in report.md Test Evidence. **Claim Source:** executed
- [x] Scenario SCN-BUG-042-007-002 (scenario-manifest.json realigned): `grep -rn 'HOST_BIND_ADDRESS:-' scenario-manifest.json` returns nothing; SCN-042-003 retitled "Missing bind address fails loud"; requiredTestType reconciled to unit/doc-lint; real TestComposeContract_* IDs. **Phase:** implement **Evidence:** reconcile — grep empty + manifest diff captured in report.md Implementation Code Diff Evidence. **Claim Source:** executed
- [x] Scenario SCN-BUG-042-007-003 (traceability-guard exit 1 → exit 0, cross-check ACTIVE): `traceability-guard.sh specs/042-tailnet-edge-bind-pattern` exits 0 with the G057/G059 cross-check ACTIVE (6 scenario contracts, 6/6 scenario→row→file→report, 6/6 G068 DoD fidelity). **Phase:** test **Evidence:** reconcile — before=exit 1 (skipped), after=PASSED; both runs captured in report.md Test + Validation Evidence. **Claim Source:** executed
- [x] Scenario SCN-BUG-042-007-004 (artifact-lint stays PASSED): `artifact-lint.sh specs/042-tailnet-edge-bind-pattern` exits 0 (PASSED) after the edits. **Phase:** validate **Evidence:** reconcile — re-run captured in report.md Validation Evidence (EXIT=0). **Claim Source:** executed
- [x] Scenario SCN-BUG-042-007-005 (deployment surface intact): `./smackerel.sh test unit --go --go-run 'TestComposeContract'` exits 0 — all nine `TestComposeContract_*` functions PASS and `internal/deploy` is ok. **Phase:** regression **Evidence:** reconcile — 9/9 PASS, `ok internal/deploy 0.045s`, captured in report.md Test Evidence. **Claim Source:** executed
- [x] Parent `report.md` gains a `## Planning-Artifact Reconciliation` section with `### Before / After Evidence`; parent `state.json` gains a `bubbles.plan` executionHistory entry + a `resolvedBugs[]` entry; `lastUpdatedAt` advanced. **Phase:** docs **Evidence:** reconcile — `grep -n 'Planning-Artifact Reconciliation' report.md` + `python3 -c` resolvedBugs check captured in report.md Implementation Code Diff Evidence. **Claim Source:** executed
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/042-tailnet-edge-bind-pattern/bugs/BUG-042-007-stale-scenario-manifest-and-traceability-skip` returns PASSED. **Phase:** validate **Evidence:** reconcile — re-run captured in report.md Validation Evidence (EXIT=0). **Claim Source:** executed
- [x] No `${HOST_BIND_ADDRESS:-127.0.0.1}` fallback form is reintroduced anywhere in the active `scopes.md` gherkin or `scenario-manifest.json`; the NO-DEFAULTS / fail-loud SST contract (Gate G028) is preserved. **Phase:** security **Evidence:** reconcile — forbidden-form scan (hits are all pre-existing forbidden-labeled/superseded context) captured in report.md Audit Evidence. **Claim Source:** executed
- [x] Closure leaves the work uncommitted (no commit/push performed by this packet); zero runtime/source change. **Phase:** audit **Evidence:** reconcile — `git status --short` shows working-tree edits only under `specs/042-tailnet-edge-bind-pattern/`; captured in report.md Audit Evidence. **Claim Source:** executed
- [x] Scenario SCN-BUG-042-007-005 regression cover — `internal/deploy/compose_contract_test.go::{TestComposeContract_LiveFile, TestComposeContract_AdversarialLiteralBind, TestComposeContract_AdversarialInfraHasPorts, TestComposeContract_AdversarialMultiPortsBypass, TestComposeContract_AdversarialMLMultiPortsBypass, TestComposeContract_AdversarialNetworkModeHostBypass, TestComposeContract_AdversarialOllamaLiteralBind, TestComposeContract_AdversarialDefaultFallbackBind, TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms}` — is re-runnable on demand and GREEN (deployment surface unchanged). **Phase:** regression **Evidence:** reconcile — `./smackerel.sh test unit --go --go-run TestComposeContract` 9/9 PASS captured in report.md Test Evidence. **Claim Source:** executed
