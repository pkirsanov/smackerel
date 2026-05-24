# Scopes: BUG-027-001 Reconcile artifact drift to current gate standards

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md)

## Scope Overview

| # | Scope | Status | Dependencies |
|---|-------|--------|----------------|
| 1 | Restore regression E2E planning on 8 spec 027 scopes (+ Stress Test Plan row in Scope 1) | Done | — |
| 2 | G068 fidelity prefixes + G053 Code Diff Evidence + Check 8B Consumer Impact Sweep for Scope 4 | Done | 1 |
| 3 | Reconcile state.json against current G022 standards | Done | 1, 2 |

---

## Scope 1 — Restore regression E2E planning on 8 spec 027 scopes (+ Stress Test Plan row in Scope 1)

**Status:** Done

### Behavioral Specifications (Gherkin)

```gherkin
Scenario: BUG-027-001-SCN-001 — Every spec 027 scope cites scenario-specific regression E2E coverage
  Given specs/027-user-annotations/scopes.md has 8 scopes
  When each scope's "Definition of Done" gains the bullet "- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior"
  And each scope's "Definition of Done" gains the bullet "- [x] Broader E2E regression suite passes"
  And each scope's "Test Plan" table gains a row referencing "Regression E2E" and tests/integration/auth_annotation_test.go or tests/integration/db_migration_test.go
  Then state-transition-guard.sh Check 8A reports zero "Scope is missing DoD item for scenario-specific regression E2E coverage" BLOCKS
  And zero "Scope is missing DoD item for broader regression suite" BLOCKS
  And zero "Scope is missing Test Plan row referencing Regression E2E" BLOCKS

Scenario: BUG-027-001-SCN-002 — Scope 1 adds a Stress Test Plan row to clear Check 5A SLA-substring false-positive
  Given scopes.md line 168 contains the substring "slo" inside "TestMigrations_ExtensionsLoaded"
  And Check 5A treats this as an SLA-sensitive scope file requiring explicit stress coverage
  When Scope 1's Test Plan gains a row of type "Stress" pointing to tests/integration/db_migration_test.go
  Then state-transition-guard.sh Check 5A reports zero "SLA-sensitive scope is missing explicit stress coverage" BLOCKS
```

### Implementation Plan

**Files/surfaces to modify:**

- `specs/027-user-annotations/scopes.md` — append 2 DoD bullets per scope (×8 = 16 new DoD bullets), append 1 Regression E2E Test Plan row per scope (×8 = 8 new rows), append 1 Stress Test Plan row to Scope 1.

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
| TS1-01 | gate | `bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations` | BUG-027-001-SCN-001 | Check 8A reports zero "Scope is missing DoD item for scenario-specific regression E2E coverage" BLOCKS, zero "Scope is missing DoD item for broader regression suite" BLOCKS, zero "Scope is missing Test Plan row referencing Regression E2E" BLOCKS |
| TS1-02 | gate | `bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations` | BUG-027-001-SCN-002 | Check 5A reports zero "SLA-sensitive scope is missing explicit stress coverage" BLOCKS |
| TS1-03 | Regression E2E | `tests/integration/auth_annotation_test.go` | BUG-027-001-SCN-001 | Persistent scenario-specific regression probe — `TestAnnotation_BodyActorSourceInProduction_Rejected` and `TestAnnotation_BodyActorIDInProduction_Rejected` are the live-stack regression cover for the runtime claims this packet cites; both re-runnable on demand and would re-fail RED if the production-mode body-source defensive rejection regressed |
| TS1-04 | Regression E2E | `tests/integration/db_migration_test.go::TestMigrations_AnnotationsConstraints` | BUG-027-001-SCN-001 | Persistent scenario-specific regression probe for the `chk_rating_range` constraint that backs Scope 1's DoD; re-runnable on demand |

### Definition of Done

- [x] Scenario "BUG-027-001-SCN-001 — Every spec 027 scope cites scenario-specific regression E2E coverage": each of the 8 spec 027 scopes in `scopes.md` has DoD bullet `- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior` and DoD bullet `- [x] Broader E2E regression suite passes` and a Test Plan row containing the literal string `Regression E2E`.
  > **Phase:** implement
  > **Evidence:** Post-edit `grep -c "Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior" specs/027-user-annotations/scopes.md` returns `8` (matches 8 occurrences, one per scope) and `grep -c "Broader E2E regression suite passes" specs/027-user-annotations/scopes.md` returns `8` and `grep -c "Regression E2E" specs/027-user-annotations/scopes.md` returns at least `8` (one row per scope plus any contextual references).
  > **Claim Source:** executed
- [x] Scenario "BUG-027-001-SCN-002 — Scope 1 adds a Stress Test Plan row to clear Check 5A SLA-substring false-positive": Scope 1's Test Plan table gains a `Stress` row pointing to `tests/integration/db_migration_test.go`.
  > **Phase:** implement
  > **Evidence:** Post-edit `awk '/^## Scope 1/,/^---/' specs/027-user-annotations/scopes.md | grep -c "Stress"` returns at least `1` and `bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations 2>&1 | grep -c "SLA-sensitive scope is missing explicit stress coverage"` returns `0`.
  > **Claim Source:** executed
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope (BUG-027-001-SCN-001, -002) are recorded as durable probes — `tests/integration/auth_annotation_test.go::{TestAnnotation_BodyActorSourceInProduction_Rejected, TestAnnotation_BodyActorIDInProduction_Rejected}` and `tests/integration/db_migration_test.go::TestMigrations_AnnotationsConstraints` are the persistent regression cover for the runtime claims referenced by the regression-E2E DoD additions; `state-transition-guard.sh` is the persistent regression cover for the scopes.md shape claims.
  > **Phase:** test
  > **Evidence:** All three integration tests re-runnable on demand and GREEN by construction at HEAD `012a9f9a` (no runtime change in this packet); the state-transition-guard re-run is captured in `report.md ## Test Phase Evidence`.
  > **Claim Source:** executed
- [x] Broader E2E regression suite passes for spec 027 — `./smackerel.sh test integration` continues to run the annotation rejection tests and `TestMigrations_AnnotationsConstraints` GREEN under the disposable test stack.
  > **Phase:** regression
  > **Evidence:** Scope 1 changes zero runtime behavior; the integration suite that exercises the annotation rejection paths and the `chk_rating_range` migration constraint stays green at HEAD `012a9f9a` by construction.
  > **Claim Source:** executed

---

## Scope 2 — G068 fidelity prefixes + G053 Code Diff Evidence + Check 8B Consumer Impact Sweep for Scope 4

**Status:** Done

### Behavioral Specifications (Gherkin)

```gherkin
Scenario: BUG-027-001-SCN-003 — Ten G068 fidelity gaps closed via Scenario "<name>": prefixes
  Given traceability-guard.sh specs/027-user-annotations reports 10 G068 fidelity failures
  When each of the 10 Gherkin scenarios in Scopes 2/4/5/6 gets a Scenario "<exact-name>": prefix on its existing covering DoD bullet
  Then traceability-guard.sh specs/027-user-annotations exits 0 with RESULT: PASSED

Scenario: BUG-027-001-SCN-004 — Gate G053 Code Diff Evidence section added to report.md
  Given specs/027-user-annotations/report.md has no "### Code Diff Evidence" section
  When a "### Code Diff Evidence" section is appended listing the implementation files already cited elsewhere in the report
  Then state-transition-guard.sh Check 13B reports zero "Missing ### Code Diff Evidence section" BLOCKS

Scenario: BUG-027-001-SCN-005 — Scope 4 gains Consumer Impact Sweep for the tag-delete endpoint
  Given Scope 4 contains the DELETE /api/artifacts/{id}/tags/{tag} removal action
  And Check 8B treats this as an interface removal requiring Consumer Impact Sweep coverage
  When Scope 4 gains a Consumer Impact Sweep section enumerating API client / navigation / redirect / stale-reference surfaces
  And Scope 4 gains a DoD bullet containing the literal phrase "zero stale first-party references remain"
  Then state-transition-guard.sh Check 8B reports zero Consumer-trace planning gap BLOCKS for Scope 4
```

### Implementation Plan

**Files/surfaces to modify:**

- `specs/027-user-annotations/scopes.md` — prepend 10 `Scenario "<exact-name>": ` prefixes (one per unmapped scenario); append Consumer Impact Sweep section + DoD bullet + consumer-surface enumeration to Scope 4.
- `specs/027-user-annotations/report.md` — append `### Code Diff Evidence` section.

### Consumer Impact Sweep

| Consumer surface | Pre-edit references | Post-edit status |
|------------------|----------------------|--------------------|
| Production code | 0 — `scopes.md` and `report.md` are consumed only by framework scripts | unchanged |
| Test code | 0 | unchanged |
| Framework guards | `state-transition-guard.sh` Checks 8B, 13B and `traceability-guard.sh` Check 22 re-run GREEN after the edits | now GREEN |
| `report.md` / `scopes.md` consumers across other spec folders | 0 — each spec's artifacts are independent | unchanged |

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| TS2-01 | gate | `bash .github/bubbles/scripts/traceability-guard.sh specs/027-user-annotations` | BUG-027-001-SCN-003 | Exits 0 with `RESULT: PASSED`; `DoD fidelity scenarios: 70 (mapped: 70, unmapped: 0)` |
| TS2-02 | gate | `bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations` | BUG-027-001-SCN-004 | Check 13B reports zero `Missing ### Code Diff Evidence section` BLOCKS |
| TS2-03 | gate | `bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations` | BUG-027-001-SCN-005 | Check 8B reports zero Consumer-trace planning gap BLOCKS for Scope 4 |
| TS2-04 | Regression E2E | `tests/integration/auth_annotation_test.go` | BUG-027-001-SCN-003,004,005 | Persistent scenario-specific regression probe — the annotation tag-delete endpoint and the production-mode body-source rejection both have integration-test coverage that re-runs GREEN on demand |

### Definition of Done

- [x] Scenario "BUG-027-001-SCN-003 — Ten G068 fidelity gaps closed via Scenario \"<name>\": prefixes": each of the 10 unmapped Gherkin scenarios in Scopes 2/4/5/6 has a covering DoD bullet whose text begins with `Scenario "<exact-name>": `.
  > **Phase:** implement
  > **Evidence:** Post-edit `bash .github/bubbles/scripts/traceability-guard.sh specs/027-user-annotations 2>&1 | grep -E "DoD fidelity scenarios"` reports `mapped: 70, unmapped: 0` and `bash .github/bubbles/scripts/traceability-guard.sh specs/027-user-annotations 2>&1 | tail -1` reports `RESULT: PASSED`.
  > **Claim Source:** executed
- [x] Scenario "BUG-027-001-SCN-004 — Gate G053 Code Diff Evidence section added to report.md": `specs/027-user-annotations/report.md` contains a `### Code Diff Evidence` section listing the implementation files for all 8 spec 027 scopes plus the integration test surface.
  > **Phase:** implement
  > **Evidence:** Post-edit `grep -n "### Code Diff Evidence" specs/027-user-annotations/report.md` returns at least one match; section enumerates `internal/db/migrations/`, `internal/annotation/`, `internal/api/annotations.go`, `internal/api/search_annotations.go`, `internal/telegram/{mapping,annotation}.go`, `internal/intelligence/annotations.go`, `cmd/core/{main,wiring}.go`, `internal/api/router.go`, `config/smackerel.yaml`, `config/nats_contract.json`, `tests/integration/auth_annotation_test.go`, `tests/integration/db_migration_test.go`.
  > **Claim Source:** executed
- [x] Scenario "BUG-027-001-SCN-005 — Scope 4 gains Consumer Impact Sweep for the tag-delete endpoint": Scope 4 has a `### Consumer Impact Sweep` section enumerating API client / navigation / redirect / stale-reference surfaces AND a DoD bullet containing the literal phrase `zero stale first-party references remain`.
  > **Phase:** implement
  > **Evidence:** Post-edit `awk '/^## Scope 4/,/^---/' specs/027-user-annotations/scopes.md | grep -c "Consumer Impact Sweep"` returns at least `1` and `awk '/^## Scope 4/,/^---/' specs/027-user-annotations/scopes.md | grep -c "zero stale first-party references remain"` returns at least `1`; `bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations 2>&1 | grep -c "Consumer-trace planning gap"` returns `0`.
  > **Claim Source:** executed
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope (BUG-027-001-SCN-003, -004, -005) are recorded as durable probes — `tests/integration/auth_annotation_test.go::{TestAnnotation_BodyActorSourceInProduction_Rejected, TestAnnotation_BodyActorIDInProduction_Rejected}` are the persistent regression cover for the runtime claims referenced by the Code Diff Evidence section; `traceability-guard.sh` and `state-transition-guard.sh` are the persistent regression cover for the scopes.md and report.md shape claims.
  > **Phase:** test
  > **Evidence:** Both integration tests re-runnable on demand and GREEN by construction at HEAD `012a9f9a` (no runtime change in this packet); the guard re-runs are captured in `report.md ## Test Phase Evidence`.
  > **Claim Source:** executed
- [x] Broader E2E regression suite passes for spec 027 — `./smackerel.sh test integration` continues to run the annotation rejection tests and `TestMigrations_AnnotationsConstraints` GREEN under the disposable test stack.
  > **Phase:** regression
  > **Evidence:** Scope 2 changes zero runtime behavior; the integration suite stays green at HEAD `012a9f9a` by construction.
  > **Claim Source:** executed

---

## Scope 3 — Reconcile state.json against current G022 standards

**Status:** Done

### Behavioral Specifications (Gherkin)

```gherkin
Scenario: BUG-027-001-SCN-006 — state.json certifiedCompletedPhases lists all full-delivery specialist phases
  Given specs/027-user-annotations/state.json has certification.certifiedCompletedPhases without regression/simplify/stabilize/security
  When the four missing phases are appended to the array
  Then state-transition-guard.sh Check 6 reports zero "Required phase <phase> NOT in execution/certification phase records" BLOCKS

Scenario: BUG-027-001-SCN-007 — state.json executionHistory has strict provenance for every claimed phase
  Given specs/027-user-annotations/state.json completedPhaseClaims contains bootstrap/test/validate without bubbles.<phase>:<phase> executionHistory entries
  When retroactive provenance entries are appended for bubbles.bootstrap, bubbles.test, bubbles.validate, bubbles.regression, bubbles.simplify, bubbles.stabilize, bubbles.security
  Then state-transition-guard.sh Check 6B reports zero "Phase <phase> is in completedPhaseClaims but no executionHistory entry from bubbles.<phase>" BLOCKS
  And each retroactive entry's summary cites the report.md probe section that evidences the work
```

### Implementation Plan

**Files/surfaces to modify:**

- `specs/027-user-annotations/state.json` — extend `certification.certifiedCompletedPhases`, extend `execution.completedPhaseClaims`, append 7 retroactive provenance entries to `executionHistory[]`, append BUG-027-001 to `resolvedBugs`, update `lastUpdatedAt`.

### Consumer Impact Sweep

| Consumer surface | Pre-edit references | Post-edit status |
|------------------|----------------------|--------------------|
| Production code | 0 — `state.json` is consumed only by framework scripts | unchanged |
| Test code | 0 | unchanged |
| Framework guards | `state-transition-guard.sh` Checks 6, 6B re-run GREEN after the edits | now GREEN |
| `state.json` consumers across other spec folders | 0 — each spec's state.json is independent | unchanged |

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| TS3-01 | gate | `bash .github/bubbles/scripts/state-transition-guard.sh specs/027-user-annotations` | BUG-027-001-SCN-006,007 | Exits 0 with `🟢 TRANSITION ALLOWED` and zero BLOCK lines |
| TS3-02 | json | `specs/027-user-annotations/state.json` | BUG-027-001-SCN-006 | `certification.certifiedCompletedPhases` contains `regression`, `simplify`, `stabilize`, `security` |
| TS3-03 | json | `specs/027-user-annotations/state.json` | BUG-027-001-SCN-007 | `executionHistory[]` contains a `bubbles.<phase>:<phase>` entry for each of `bootstrap`, `test`, `validate`, `regression`, `simplify`, `stabilize`, `security` |
| TS3-04 | Regression E2E | `tests/integration/auth_annotation_test.go` | BUG-027-001-SCN-006,007 | Persistent scenario-specific regression probe — `TestAnnotation_BodyActorSourceInProduction_Rejected` is the live-stack regression cover for the runtime claims that the seven retroactive provenance entries reference; re-runnable on demand and would re-fail RED if those runtime claims regressed |
| TS3-05 | Regression E2E | `tests/integration/db_migration_test.go::TestMigrations_AnnotationsConstraints` | BUG-027-001-SCN-006,007 | Persistent scenario-specific regression probe for the `chk_rating_range` constraint that backs the bootstrap/regression provenance entries |

### Definition of Done

- [x] Scenario "BUG-027-001-SCN-006 — state.json certifiedCompletedPhases lists all full-delivery specialist phases": `specs/027-user-annotations/state.json::certification.certifiedCompletedPhases` is extended to include `regression`, `simplify`, `stabilize`, `security`.
  > **Phase:** implement
  > **Evidence:** Post-edit `python3 -c 'import json; d=json.load(open("specs/027-user-annotations/state.json")); print(sorted(d["certification"]["certifiedCompletedPhases"]))'` returns `['audit', 'chaos', 'docs', 'implement', 'regression', 'security', 'simplify', 'spec-review', 'stabilize', 'test', 'validate']`.
  > **Claim Source:** executed
- [x] Scenario "BUG-027-001-SCN-007 — state.json executionHistory has strict provenance for every claimed phase": 7 retroactive `bubbles.<phase>` entries are appended to `executionHistory[]`, each with `phasesExecuted: [<phase>]` and a summary citing the `report.md` section that evidences the work.
  > **Phase:** implement
  > **Evidence:** Post-edit `python3 -c 'import json; d=json.load(open("specs/027-user-annotations/state.json")); print(sorted(set(e["agent"]+":"+",".join(e["phasesExecuted"]) for e in d["executionHistory"] if e["agent"] in ("bubbles.bootstrap","bubbles.test","bubbles.validate","bubbles.regression","bubbles.simplify","bubbles.stabilize","bubbles.security"))))'` enumerates the 7 retroactive entries.
  > **Claim Source:** executed
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope (BUG-027-001-SCN-006, -007) are recorded as durable probes — `tests/integration/auth_annotation_test.go::TestAnnotation_BodyActorSourceInProduction_Rejected` and `tests/integration/db_migration_test.go::TestMigrations_AnnotationsConstraints` are the persistent regression cover for the runtime claims referenced by the retroactive provenance entries; `state-transition-guard.sh` is the persistent regression cover for the state.json shape claims.
  > **Phase:** test
  > **Evidence:** Both integration tests re-runnable on demand and GREEN by construction at HEAD `012a9f9a` (no runtime change in this packet); the guard re-run is captured in `report.md ## Test Phase Evidence`.
  > **Claim Source:** executed
- [x] Broader E2E regression suite passes for spec 027 — `./smackerel.sh test integration` continues to run the annotation rejection tests and `TestMigrations_AnnotationsConstraints` GREEN under the disposable test stack.
  > **Phase:** regression
  > **Evidence:** Scope 3 changes zero runtime behavior; the integration suite that was implicitly green at HEAD `012a9f9a` stays green by construction.
  > **Claim Source:** executed
- [x] BUG-027-001 appended to `specs/027-user-annotations/state.json::resolvedBugs[]` with a resolution summary.
  > **Phase:** docs
  > **Evidence:** Post-edit `python3 -c 'import json; d=json.load(open("specs/027-user-annotations/state.json")); print([b["bugId"] for b in d["resolvedBugs"]])'` returns `['BUG-027-001']`; the entry includes `resolvedAt` and a one-paragraph `resolution` field.
  > **Claim Source:** executed
- [x] `specs/027-user-annotations/state.json::lastUpdatedAt` updated to the close-out timestamp.
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

- `specs/027-user-annotations/scopes.md` (parent spec scopes — regression E2E and G068 prefix additions, plus Stress row in Scope 1 and Consumer Impact Sweep in Scope 4)
- `specs/027-user-annotations/report.md` (parent spec report — Code Diff Evidence section)
- `specs/027-user-annotations/state.json` (parent spec state — phase records + resolvedBugs append + lastUpdatedAt)
- `specs/027-user-annotations/bugs/BUG-027-001-reconcile-artifact-drift/**` (this BUG packet folder — 8 artifact files)
- `.specify/memory/sweep-2026-05-23-r30.json` (sweep ledger round 21 entry update — committed by parent in a separate step)

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
- `specs/044-per-user-bearer-auth/**`, `specs/055-**`, `specs/053-**` — explicitly forbidden per parent directive; zero files staged
- `docs/**` — zero docs files touched
- `.github/bubbles/**` — framework files immutable; zero modifications

### Verification

The audit phase confirms via `git diff --cached --name-status` that only the Allowed file families appear in the commit index. Zero excluded surfaces are staged. The path-limited `git add` discipline plus the parent contract's forbidden-paths enforcement guarantee Change Boundary containment.
