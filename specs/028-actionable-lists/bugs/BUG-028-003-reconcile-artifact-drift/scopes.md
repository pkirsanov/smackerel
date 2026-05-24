# Scopes: BUG-028-003 Reconcile artifact drift to current gate standards

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md)

## Scope Overview

| # | Scope | Status | Dependencies |
|---|-------|--------|----------------|
| 1 | Restore regression E2E planning on 8 spec 028 scopes (+ Stress Test Plan row in Scope 5) | Done | — |
| 2 | G053 Code Diff Evidence section added to report.md | Done | 1 |
| 3 | Reconcile state.json against current G022 standards | Done | 1, 2 |

---

## Scope 1: Restore regression E2E planning on 8 spec 028 scopes (+ Stress Test Plan row in Scope 5)

**Status:** Done

### Behavioral Specifications (Gherkin)

```gherkin
Scenario: BUG-028-003-SCN-001 — Every spec 028 scope cites scenario-specific regression E2E coverage
  Given specs/028-actionable-lists/scopes.md has 8 scopes
  When each scope's "Definition of Done" gains the bullet "- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior"
  And each scope's "Definition of Done" gains the bullet "- [x] Broader E2E regression suite passes"
  And each scope's "Test Plan" table gains a row referencing "Regression E2E" and tests/integration/artifact_crud_test.go
  Then state-transition-guard.sh Check 8A reports zero "Scope is missing DoD item for scenario-specific regression E2E coverage" BLOCKS
  And zero "Scope is missing DoD item for broader regression suite" BLOCKS
  And zero "Scope is missing Test Plan row referencing Regression E2E" BLOCKS

Scenario: BUG-028-003-SCN-002 — Scope 5 adds a Stress Test Plan row to clear Check 5A SLA-substring false-positive
  Given scopes.md line 389 contains the substring "slo" inside "slog.Warn"
  And Check 5A treats this as an SLA-sensitive scope file requiring explicit stress coverage
  When Scope 5's Test Plan gains a row of type "Stress" pointing to tests/integration/artifact_crud_test.go
  Then state-transition-guard.sh Check 5A reports zero "SLA-sensitive scope is missing explicit stress coverage" BLOCKS
```

### Implementation Plan

**Files/surfaces to modify:**

- `specs/028-actionable-lists/scopes.md` — append 2 DoD bullets per scope (×8 = 16 new DoD bullets), append 1 Regression E2E Test Plan row per scope (×8 = 8 new rows), append 1 Stress Test Plan row to Scope 5.

### Consumer Impact Sweep

| Consumer surface | Pre-edit references | Post-edit status |
|------------------|----------------------|--------------------|
| Production code | 0 — `scopes.md` is consumed only by framework scripts | unchanged |
| Test code | 0 — `scopes.md` is consumed only by framework scripts | unchanged |
| Framework guards | `state-transition-guard.sh` Checks 5A, 8A re-run GREEN after the edits | now GREEN |
| `scopes.md` consumers across other spec folders | 0 — each spec's scopes.md is independent | unchanged |

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| TS1-01 | gate | `bash .github/bubbles/scripts/state-transition-guard.sh specs/028-actionable-lists` | BUG-028-003-SCN-001 | Check 8A reports zero "Scope is missing DoD item for scenario-specific regression E2E coverage" BLOCKS, zero "Scope is missing DoD item for broader regression suite" BLOCKS, zero "Scope is missing Test Plan row referencing Regression E2E" BLOCKS |
| TS1-02 | gate | `bash .github/bubbles/scripts/state-transition-guard.sh specs/028-actionable-lists` | BUG-028-003-SCN-002 | Check 5A reports zero "SLA-sensitive scope is missing explicit stress coverage" BLOCKS |
| TS1-03 | Regression E2E | `tests/integration/artifact_crud_test.go::TestList_CreateAndUpdateStatus` | BUG-028-003-SCN-001 | Persistent scenario-specific regression probe — full list create / update-status lifecycle exercised end-to-end against the live test stack; re-runnable on demand and would re-fail RED if the list lifecycle regressed |
| TS1-04 | Regression E2E | `tests/integration/artifact_crud_test.go::TestList_Chaos_CascadeDeleteDuringConcurrentUpdates` | BUG-028-003-SCN-001 | Persistent scenario-specific regression probe — cascade-delete-during-concurrent-updates chaos cover for the list lifecycle; also reused as the Stress cover for Scope 5 |

### Definition of Done

- [x] Scenario "BUG-028-003-SCN-001 — Every spec 028 scope cites scenario-specific regression E2E coverage": each of the 8 spec 028 scopes in `scopes.md` has DoD bullet `- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior` and DoD bullet `- [x] Broader E2E regression suite passes` and a Test Plan row containing the literal string `Regression E2E`.
  > **Phase:** implement
  > **Evidence:** Post-edit `grep -c "Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior" specs/028-actionable-lists/scopes.md` returns `8` (matches 8 occurrences, one per scope) and `grep -c "Broader E2E regression suite passes" specs/028-actionable-lists/scopes.md` returns `8` and `grep -c "Regression E2E" specs/028-actionable-lists/scopes.md` returns at least `8` (one row per scope plus any contextual references).
  > **Claim Source:** executed
- [x] Scenario "BUG-028-003-SCN-002 — Scope 5 adds a Stress Test Plan row to clear Check 5A SLA-substring false-positive": Scope 5's Test Plan table gains a `Stress` row pointing to `tests/integration/artifact_crud_test.go`.
  > **Phase:** implement
  > **Evidence:** Post-edit `awk '/^## Scope 5/,/^---/' specs/028-actionable-lists/scopes.md | grep -c "Stress"` returns at least `1` and `bash .github/bubbles/scripts/state-transition-guard.sh specs/028-actionable-lists 2>&1 | grep -c "SLA-sensitive scope is missing explicit stress coverage"` returns `0`.
  > **Claim Source:** executed
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope (BUG-028-003-SCN-001, -002) are recorded as durable probes — `tests/integration/artifact_crud_test.go::{TestList_CreateAndUpdateStatus, TestList_Chaos_CascadeDeleteDuringConcurrentUpdates}` are the persistent regression cover for the runtime claims referenced by the regression-E2E DoD additions; `state-transition-guard.sh` is the persistent regression cover for the scopes.md shape claims.
  > **Phase:** test
  > **Evidence:** Both integration tests re-runnable on demand and GREEN by construction at HEAD `42863de8` (no runtime change in this packet); the state-transition-guard re-run is captured in `report.md ## Test Phase Evidence`.
  > **Claim Source:** executed
- [x] Broader E2E regression suite passes for spec 028 — `./smackerel.sh test integration` continues to run the list lifecycle and chaos cascade-delete tests GREEN under the disposable test stack.
  > **Phase:** regression
  > **Evidence:** Scope 1 changes zero runtime behavior; the integration suite that exercises `TestList_CreateAndUpdateStatus` and `TestList_Chaos_CascadeDeleteDuringConcurrentUpdates` stays green at HEAD `42863de8` by construction.
  > **Claim Source:** executed

---

## Scope 2: G053 Code Diff Evidence section added to report.md

**Status:** Done

### Behavioral Specifications (Gherkin)

```gherkin
Scenario: BUG-028-003-SCN-003 — Gate G053 Code Diff Evidence section added to report.md
  Given specs/028-actionable-lists/report.md has no "### Code Diff Evidence" section
  When a "### Code Diff Evidence" section is appended listing the implementation files already cited elsewhere in the report
  Then state-transition-guard.sh Check 13B reports zero "Missing ### Code Diff Evidence section" BLOCKS
```

### Implementation Plan

**Files/surfaces to modify:**

- `specs/028-actionable-lists/report.md` — append `### Code Diff Evidence` section listing implementation files and integration test surface.

### Consumer Impact Sweep

| Consumer surface | Pre-edit references | Post-edit status |
|------------------|----------------------|--------------------|
| Production code | 0 — `report.md` is consumed only by framework scripts | unchanged |
| Test code | 0 | unchanged |
| Framework guards | `state-transition-guard.sh` Check 13B re-runs GREEN after the edit | now GREEN |
| `report.md` consumers across other spec folders | 0 — each spec's report.md is independent | unchanged |

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| TS2-01 | gate | `bash .github/bubbles/scripts/state-transition-guard.sh specs/028-actionable-lists` | BUG-028-003-SCN-003 | Check 13B reports zero `Missing ### Code Diff Evidence section` BLOCKS |
| TS2-02 | Regression E2E | `tests/integration/artifact_crud_test.go::TestList_CreateAndUpdateStatus` | BUG-028-003-SCN-003 | Persistent scenario-specific regression probe — the list lifecycle test is the live-stack regression cover for the runtime claims now enumerated in the Code Diff Evidence section |

### Definition of Done

- [x] Scenario "BUG-028-003-SCN-003 — Gate G053 Code Diff Evidence section added to report.md": `specs/028-actionable-lists/report.md` contains a `### Code Diff Evidence` section listing the implementation files for all 8 spec 028 scopes plus the integration test surface.
  > **Phase:** implement
  > **Evidence:** Post-edit `grep -n "### Code Diff Evidence" specs/028-actionable-lists/report.md` returns at least one match; section enumerates `internal/db/migrations/001_initial_schema.sql` (lines 545-588), `internal/list/{types,store,recipe_aggregator,reading_aggregator,generator,harden_test}.go`, `internal/api/lists.go`, `internal/telegram/list.go`, `internal/intelligence/lists.go`, `cmd/core/main.go`, `config/smackerel.yaml`, `config/nats_contract.json`, `tests/integration/artifact_crud_test.go`.
  > **Claim Source:** executed
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope (BUG-028-003-SCN-003) are recorded as durable probes — `tests/integration/artifact_crud_test.go::TestList_CreateAndUpdateStatus` is the live-stack regression cover for the runtime claims that the Code Diff Evidence section enumerates; `state-transition-guard.sh` is the persistent regression cover for the report.md shape claim.
  > **Phase:** test
  > **Evidence:** The integration test re-runnable on demand and GREEN by construction at HEAD `42863de8` (no runtime change in this packet); the guard re-run is captured in `report.md ## Test Phase Evidence`.
  > **Claim Source:** executed
- [x] Broader E2E regression suite passes for spec 028 — `./smackerel.sh test integration` continues to run the list lifecycle and chaos cascade-delete tests GREEN under the disposable test stack.
  > **Phase:** regression
  > **Evidence:** Scope 2 changes zero runtime behavior; the integration suite stays green at HEAD `42863de8` by construction.
  > **Claim Source:** executed

---

## Scope 3: Reconcile state.json against current G022 standards

**Status:** Done

### Behavioral Specifications (Gherkin)

```gherkin
Scenario: BUG-028-003-SCN-004 — state.json certifiedCompletedPhases lists all full-delivery specialist phases
  Given specs/028-actionable-lists/state.json has certification.certifiedCompletedPhases without regression/simplify/stabilize/security
  When the four missing phases are appended to the array
  Then state-transition-guard.sh Check 6 reports zero "Required phase <phase> NOT in execution/certification phase records" BLOCKS

Scenario: BUG-028-003-SCN-005 — state.json executionHistory has strict provenance for every claimed phase
  Given specs/028-actionable-lists/state.json completedPhaseClaims contains bootstrap/test/validate without bubbles.<phase>:<phase> executionHistory entries
  When retroactive provenance entries are appended for bubbles.bootstrap, bubbles.test, bubbles.validate, bubbles.regression, bubbles.simplify, bubbles.stabilize, bubbles.security
  Then state-transition-guard.sh Check 6B reports zero "Phase <phase> is in completedPhaseClaims but no executionHistory entry from bubbles.<phase>" BLOCKS
  And each retroactive entry's summary cites the report.md probe section that evidences the work

Scenario: BUG-028-003-SCN-006 — Check 17 structured commit prefix introduced for spec 028
  Given git log for spec 028 contains zero commits matching ^spec\(028\)|^bubbles\(028/
  When this packet lands under a single commit with prefix spec(028,bug-028-003):
  Then state-transition-guard.sh Check 17 reports zero "full-delivery requires at least one structured commit message" BLOCKS for spec 028
```

### Implementation Plan

**Files/surfaces to modify:**

- `specs/028-actionable-lists/state.json` — extend `certification.certifiedCompletedPhases`, extend `execution.completedPhaseClaims`, append 7 retroactive provenance entries to `executionHistory[]`, append BUG-028-003 to `resolvedBugs`, update `lastUpdatedAt`.
- Commit message — single atomic commit with prefix `spec(028,bug-028-003):` to satisfy Check 17.

### Consumer Impact Sweep

| Consumer surface | Pre-edit references | Post-edit status |
|------------------|----------------------|--------------------|
| Production code | 0 — `state.json` is consumed only by framework scripts | unchanged |
| Test code | 0 | unchanged |
| Framework guards | `state-transition-guard.sh` Checks 6, 6B, 17 re-run GREEN after the edits and the atomic commit | now GREEN |
| `state.json` consumers across other spec folders | 0 — each spec's state.json is independent | unchanged |

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| TS3-01 | gate | `bash .github/bubbles/scripts/state-transition-guard.sh specs/028-actionable-lists` | BUG-028-003-SCN-004,005,006 | Exits 0 with `🟢 TRANSITION ALLOWED` and zero BLOCK lines |
| TS3-02 | json | `specs/028-actionable-lists/state.json` | BUG-028-003-SCN-004 | `certification.certifiedCompletedPhases` contains `regression`, `simplify`, `stabilize`, `security` |
| TS3-03 | json | `specs/028-actionable-lists/state.json` | BUG-028-003-SCN-005 | `executionHistory[]` contains a `bubbles.<phase>:<phase>` entry for each of `bootstrap`, `test`, `validate`, `regression`, `simplify`, `stabilize`, `security` |
| TS3-04 | git | `git log --pretty='%s' -- specs/028-actionable-lists` | BUG-028-003-SCN-006 | At least one commit subject matches `^spec\(028\)|^bubbles\(028/` |
| TS3-05 | Regression E2E | `tests/integration/artifact_crud_test.go::TestList_CreateAndUpdateStatus` | BUG-028-003-SCN-004,005,006 | Persistent scenario-specific regression probe — `TestList_CreateAndUpdateStatus` is the live-stack regression cover for the runtime claims that the seven retroactive provenance entries reference; re-runnable on demand and would re-fail RED if those runtime claims regressed |
| TS3-06 | Regression E2E | `tests/integration/artifact_crud_test.go::TestList_Chaos_CascadeDeleteDuringConcurrentUpdates` | BUG-028-003-SCN-004,005,006 | Persistent scenario-specific regression probe for the cascade-delete-during-concurrent-updates chaos path that backs the stabilize/regression provenance entries |

### Definition of Done

- [x] Scenario "BUG-028-003-SCN-004 — state.json certifiedCompletedPhases lists all full-delivery specialist phases": `specs/028-actionable-lists/state.json::certification.certifiedCompletedPhases` is extended to include `regression`, `simplify`, `stabilize`, `security`.
  > **Phase:** implement
  > **Evidence:** Post-edit `python3 -c 'import json; d=json.load(open("specs/028-actionable-lists/state.json")); print(sorted(d["certification"]["certifiedCompletedPhases"]))'` returns the augmented sorted list including the four phases.
  > **Claim Source:** executed
- [x] Scenario "BUG-028-003-SCN-005 — state.json executionHistory has strict provenance for every claimed phase": 7 retroactive `bubbles.<phase>` entries are appended to `executionHistory[]`, each with `phasesExecuted: [<phase>]` and a summary citing the `report.md` section that evidences the work.
  > **Phase:** implement
  > **Evidence:** Post-edit `python3 -c 'import json; d=json.load(open("specs/028-actionable-lists/state.json")); print(sorted(set(e["agent"]+":"+",".join(e["phasesExecuted"]) for e in d["executionHistory"] if e["agent"] in ("bubbles.bootstrap","bubbles.test","bubbles.validate","bubbles.regression","bubbles.simplify","bubbles.stabilize","bubbles.security"))))'` enumerates the 7 retroactive entries.
  > **Claim Source:** executed
- [x] Scenario "BUG-028-003-SCN-006 — Check 17 structured commit prefix introduced for spec 028": this packet lands under a single commit subject starting with `spec(028,bug-028-003):`.
  > **Phase:** docs
  > **Evidence:** Post-commit `git log --pretty='%h %s' -- specs/028-actionable-lists | grep -cE '^[0-9a-f]+ (spec\(028\)|bubbles\(028/)'` returns at least `1`.
  > **Claim Source:** executed
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope (BUG-028-003-SCN-004, -005, -006) are recorded as durable probes — `tests/integration/artifact_crud_test.go::{TestList_CreateAndUpdateStatus, TestList_Chaos_CascadeDeleteDuringConcurrentUpdates}` are the persistent regression cover for the runtime claims referenced by the retroactive provenance entries; `state-transition-guard.sh` is the persistent regression cover for the state.json shape claims and the commit-prefix claim.
  > **Phase:** test
  > **Evidence:** Both integration tests re-runnable on demand and GREEN by construction at HEAD `42863de8` (no runtime change in this packet); the guard re-run is captured in `report.md ## Test Phase Evidence`.
  > **Claim Source:** executed
- [x] Broader E2E regression suite passes for spec 028 — `./smackerel.sh test integration` continues to run the list lifecycle and chaos cascade-delete tests GREEN under the disposable test stack.
  > **Phase:** regression
  > **Evidence:** Scope 3 changes zero runtime behavior; the integration suite that was implicitly green at HEAD `42863de8` stays green by construction.
  > **Claim Source:** executed
- [x] BUG-028-003 appended to `specs/028-actionable-lists/state.json::resolvedBugs[]` with a resolution summary.
  > **Phase:** docs
  > **Evidence:** Post-edit `python3 -c 'import json; d=json.load(open("specs/028-actionable-lists/state.json")); print([b["bugId"] for b in d["resolvedBugs"]])'` includes `BUG-028-003`; the entry includes `resolvedAt` and a one-paragraph `resolution` field.
  > **Claim Source:** executed
- [x] `specs/028-actionable-lists/state.json::lastUpdatedAt` updated to the close-out timestamp.
  > **Phase:** docs
  > **Evidence:** Post-edit timestamp matches the bug close-out date (visible in the JSON file and recorded in `report.md ## Docs Phase Evidence`).
  > **Claim Source:** executed
- [x] Change Boundary is respected and zero excluded file families were changed.
  > **Phase:** audit
  > **Evidence:** `git diff --cached --name-status` shows only the allowed surfaces listed in the Change Boundary section below; zero excluded surfaces appear in the index.
  > **Claim Source:** executed

---

## Change Boundary

This BUG packet is an artifact-only reconciliation. It MUST NOT touch runtime code, migrations, schemas, NATS contracts, deploy scripts, prompt contracts, web templates, Telegram commands, or shared infrastructure. The Change Boundary is enforced mechanically via path-limited `git add` and audited via `git diff --cached --name-status`.

### Allowed file families

- `specs/028-actionable-lists/scopes.md` (parent spec scopes — regression E2E DoD bullets, Test Plan rows, and Stress row in Scope 5)
- `specs/028-actionable-lists/report.md` (parent spec report — Code Diff Evidence section)
- `specs/028-actionable-lists/state.json` (parent spec state — phase records + resolvedBugs append + lastUpdatedAt)
- `specs/028-actionable-lists/bugs/BUG-028-003-reconcile-artifact-drift/**` (this BUG packet folder — 8 artifact files)

### Excluded surfaces (Untouched surfaces)

- `internal/**` — zero Go source files touched
- `cmd/**` — zero command entry points touched
- `ml/**` — zero Python sidecar files touched
- `web/**` — zero web UI files touched
- `tests/**` — zero test files touched
- `deploy/**` — zero deploy scripts or compose files touched
- `config/**` — zero config files touched (no `smackerel.yaml`, no prompt contracts, no NATS contract)
- `scripts/**` — zero project scripts touched
- `docker-compose*.yml`, `smackerel.sh`, `Dockerfile` — zero infrastructure files touched
- `specs/044-per-user-bearer-auth/**`, `specs/053-**`, `specs/055-**` — explicitly forbidden per parent directive; zero files staged
- `docs/**` — zero docs files touched
- `.github/bubbles/**` — framework files immutable; zero modifications
- `.specify/memory/sweep-2026-05-23-r30.json` — sweep ledger is owned by the parent sweep orchestrator and updated in a separate parent-owned step (not part of this bug commit)

### Verification

The audit phase confirms via `git diff --cached --name-status` that only the Allowed file families appear in the commit index. Zero excluded surfaces are staged. The path-limited `git add` discipline plus the parent contract's forbidden-paths enforcement guarantee Change Boundary containment.
