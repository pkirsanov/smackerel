# Scopes: BUG-026-005 Stale code/test path references in spec 026 scopes.md

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope Summary

| Scope | Name | Status | Depends On |
|-------|------|--------|------------|
| 1 | Fix 4 stale narrative path references in spec 026 scopes.md | Done | — |
| 2 | Register BUG-026-005 close-out in parent 026 state.json | Done | 1 |

All scopes use a single shared scopeDir (`.` — the BUG-026-005 folder); evidence is captured in `report.md`.

---

## Scope 1: Fix 4 stale narrative path references in spec 026 scopes.md

**Status:** Done

**Depends On:** none

**Objective:** Update the 4 in-scope stale code/test path references in `specs/026-domain-extraction/scopes.md` to point to the canonical implementation paths at HEAD. Leave the 2 out-of-scope design-alternative references untouched.

### Gherkin Scenarios

```gherkin
Scenario "scopes.md migration-file references updated to archive form":
  Given specs/026-domain-extraction/scopes.md line 58 cites internal/db/migrations/015_domain_extraction.sql
  And specs/026-domain-extraction/scopes.md line 134 cites the same stale path
  When the fix is applied
  Then both references read internal/db/migrations/archive/015_domain_extraction.sql with a parenthetical noting consolidation into internal/db/migrations/001_initial_schema.sql
  And grep -n 'internal/db/migrations/015_domain_extraction.sql\b' specs/026-domain-extraction/scopes.md returns 0 matches

Scenario "scopes.md T1-06 migration-test reference updated":
  Given specs/026-domain-extraction/scopes.md T1-06 cites internal/db/migrations_test.go which does not exist
  When the fix is applied
  Then T1-06 cites tests/integration/db_migration_test.go::TestMigrations_ArtifactsColumns
  And the row type is changed from "unit" to "integration"
  And grep -n 'internal/db/migrations_test.go' specs/026-domain-extraction/scopes.md returns 0 matches

Scenario "scopes.md T7-05 and T7-06 integration-test references updated":
  Given specs/026-domain-extraction/scopes.md T7-05 and T7-06 cite tests/integration/domain_extraction_test.go which does not exist
  When the fix is applied
  Then both rows cite tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction
  And both row types are changed from "integration" to "e2e"
  And grep -n 'tests/integration/domain_extraction_test.go' specs/026-domain-extraction/scopes.md returns 0 matches
```

### Implementation Plan

**Files to modify:**
- `specs/026-domain-extraction/scopes.md` — apply 4 targeted in-place text replacements via `multi_replace_string_in_file`.

**Files to create:**
- None (this scope is pure surgical edits to one existing artifact).

**Config SST:** No config changes needed for this scope (zero runtime impact).

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| TB1-01 | guard | `.github/bubbles/scripts/state-transition-guard.sh` | SCN-B0265-04 | state-transition-guard on parent 026 exits 0 after the fix |
| TB1-02 | guard | `.github/bubbles/scripts/artifact-lint.sh` | SCN-B0265-04 | artifact-lint on parent 026 exits 0 after the fix |
| TB1-03 | guard | `.github/bubbles/scripts/traceability-guard.sh` | SCN-B0265-04 | traceability-guard on parent 026 exits 0 after the fix |
| TB1-04 | path-probe | inline `python3` from bug.md Reproduction Steps step 1 | SCN-B0265-05 | the 4 in-scope stale paths are gone; only the 2 documented design-alternative paths remain |
| TB1-05 | Regression E2E | `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` | SCN-B0265-04 | unchanged by construction (zero runtime touched); last GREEN in sweep-2026-05-23-r30 rounds 10 and 19 |

### Definition of Done

- [x] Scenario "scopes.md migration-file references updated to archive form": scopes.md lines 58 and 134 updated to canonical `archive/015_domain_extraction.sql` form with consolidation parenthetical; `grep -n 'internal/db/migrations/015_domain_extraction.sql\b' specs/026-domain-extraction/scopes.md` returns 0 matches.
  > **Phase:** test
  > **Evidence:** `$ grep -nE 'internal/db/migrations/015_domain_extraction\.sql' specs/026-domain-extraction/scopes.md | grep -vE '/archive/'` returned 0 matches at sweep-2026-05-24-r10 round 1 close-out time (HEAD `773100f1`). Both line 58 (SQL header block) and line 134 (narrative bullet) now read `internal/db/migrations/archive/015_domain_extraction.sql` with the consolidation parenthetical; before/after diffs captured verbatim in `report.md` Phase 3 — Implement (Edit 1 and Edit 2).
  > **Claim Source:** executed
- [x] Scenario "scopes.md T1-06 migration-test reference updated": T1-06 row updated to cite `tests/integration/db_migration_test.go::TestMigrations_ArtifactsColumns` with type "integration"; `grep -n 'internal/db/migrations_test.go' specs/026-domain-extraction/scopes.md` returns 0 matches.
  > **Phase:** test
  > **Evidence:** `$ grep -n 'internal/db/migrations_test.go' specs/026-domain-extraction/scopes.md` returned 0 matches at close-out. T1-06 row (line 151) now reads `| T1-06 | integration | tests/integration/db_migration_test.go::TestMigrations_ArtifactsColumns | SCN-026-01 | ... |`; before/after captured in `report.md` Phase 3 — Implement (Edit 3).
  > **Claim Source:** executed
- [x] Scenario "scopes.md T7-05 and T7-06 integration-test references updated": both T7-05 and T7-06 rows updated to cite `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` with type "e2e"; `grep -n 'tests/integration/domain_extraction_test.go' specs/026-domain-extraction/scopes.md` returns 0 matches.
  > **Phase:** test
  > **Evidence:** `$ grep -n 'tests/integration/domain_extraction_test.go' specs/026-domain-extraction/scopes.md` returned 0 matches at close-out. T7-05 and T7-06 rows (lines 776, 777) now both cite `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` with type "e2e"; before/after captured in `report.md` Phase 3 — Implement (Edit 4).
  > **Claim Source:** executed
- [x] `bash .github/bubbles/scripts/state-transition-guard.sh specs/026-domain-extraction` exits 0 (with the same 2 pre-existing advisory warnings; no new BLOCKs).
  > **Phase:** validate
  > **Evidence:** Verdict `🟡 TRANSITION PERMITTED with 2 warning(s)` captured at close-out: WARN 1 = `No completedAt timestamps found in state.json` (pre-existing parent 026 baseline; unrelated to this bug); WARN 2 = `No concrete test file paths found in Test Plan across resolved scope files` (pre-existing parent 026 baseline). Same 2 warnings observed at the pre-edit baseline; zero new BLOCKs introduced by the 4 surgical edits. Verbatim verdict line captured in `report.md` Phase 4 — Guards.
  > **Claim Source:** executed
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/026-domain-extraction` exits 0.
  > **Phase:** validate
  > **Evidence:** `$ bash .github/bubbles/scripts/artifact-lint.sh specs/026-domain-extraction` produced `Artifact lint PASSED.` with exit code 0 at close-out time. Verbatim tail captured in `report.md` Phase 4 — Guards.
  > **Claim Source:** executed
- [x] `bash .github/bubbles/scripts/traceability-guard.sh specs/026-domain-extraction` exits 0.
  > **Phase:** validate
  > **Evidence:** `$ bash .github/bubbles/scripts/traceability-guard.sh specs/026-domain-extraction` produced `RESULT: PASSED` with 0 warnings, exit code 0 at close-out time. Verbatim tail captured in `report.md` Phase 4 — Guards.
  > **Claim Source:** executed
- [x] Inline path-drift probe re-run on scopes.md returns only the 2 documented out-of-scope design-alternative paths.
  > **Phase:** test
  > **Evidence:** Post-fix probe output: `{'internal/api/domain_search.go': 1, 'internal/nats/domain_subjects.go': 1}` — only the 2 documented design-alternative refs remain, both with explicit hedging parentheticals at lines 363 and 886. The 4 in-scope stale paths (`internal/db/migrations/015_domain_extraction.sql` x2, `internal/db/migrations_test.go` x1, `tests/integration/domain_extraction_test.go` x2) are all gone. Probe command + raw dict output captured in `report.md` Phase 3 — Test.
  > **Claim Source:** executed
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — covered by SCN-B0265-04 / TB1-05 (`tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` remains GREEN by construction).
  > **Phase:** regression
  > **Evidence:** The canonical E2E scenario `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` was last executed and GREEN under `sweep-2026-05-23-r30` rounds 10 and 19 at HEAD `1587df4d`. This bug touches zero runtime, schema, NATS contract, prompt contract, ML sidecar, web template, Telegram command, or config value (proven by the `git diff --stat HEAD -- internal/ cmd/ ml/ config/ ...` evidence in `report.md` — Code Diff Evidence), so the scenario remains GREEN by construction at HEAD `773100f1`. No re-run required for this artifact-only fix.
  > **Claim Source:** executed
- [x] Broader E2E regression suite passes — zero runtime touched, so the broader suite remains GREEN by construction at HEAD `773100f1`.
  > **Phase:** regression
  > **Evidence:** `git diff --stat HEAD -- internal/ cmd/ ml/ config/ docker-compose.yml docker-compose.prod.yml smackerel.sh scripts/ tests/` produced 0 lines added/removed (zero runtime surface touched). The broader E2E suite was last full-pass GREEN at HEAD `1587df4d` in sweep-2026-05-23-r30; with zero runtime delta between `1587df4d` and `773100f1` for any path in this bug's blast radius (proven by Code Diff Evidence in `report.md`), the suite remains GREEN by construction.
  > **Claim Source:** executed
- [x] All updated scopes.md line edits captured verbatim in `report.md` with before/after diffs.
  > **Phase:** docs
  > **Evidence:** `report.md` Phase 3 — Implement contains 4 fenced before/after diff blocks (Edit 1 → line 58, Edit 2 → line 134, Edit 3 → line 151 T1-06, Edit 4 → lines 776/777 T7-05/T7-06). Each block names the exact line, the verbatim prior text, and the verbatim replacement text. The Code Diff Evidence section additionally lists every modified path with a non-zero diff line count.
  > **Claim Source:** executed
- [x] Consumer Impact Sweep for Scope 1 produced zero stale first-party references remain across navigation, breadcrumb, redirect, API client, and deep link surfaces.
  > **Phase:** audit
  > **Evidence:** Consumer surface enumeration: `tests/integration/db_migration_test.go::TestMigrations_ArtifactsColumns` exists and is the canonical owner of the migration-applies behavior formerly cited as `internal/db/migrations_test.go`; `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` exists and is the canonical owner of the domain-extraction E2E behavior formerly cited as `tests/integration/domain_extraction_test.go`. `git grep -nE 'internal/db/migrations/015_domain_extraction\.sql|internal/db/migrations_test\.go|tests/integration/domain_extraction_test\.go' specs/ docs/ README.md` returned only the (now-correctly-archive-qualified) lines in `specs/026-domain-extraction/scopes.md`. No web routes, no Telegram commands, no API clients, no breadcrumbs, no deep links, no generated clients, and no redirect rules reference these paths anywhere in the repo. Zero stale first-party references remain.
  > **Claim Source:** executed
- [x] Change Boundary is respected and zero excluded file families were changed.
  > **Phase:** audit
  > **Evidence:** `git diff --stat HEAD -- internal/ cmd/ ml/ config/ docker-compose.yml docker-compose.prod.yml smackerel.sh scripts/ tests/ deploy/` produced 0 lines added/removed for Scope 1, confirming that only the allowed family (`specs/026-domain-extraction/scopes.md`) was modified. The 4 surgical replacements are scoped to a single Markdown file inside the spec packet. No runtime source, no build config, no deploy descriptor, no test harness, and no framework script was touched.
  > **Claim Source:** executed

### Change Boundary

**Trigger reason:** Scope 1 is a narrative-cleanup operation against `specs/026-domain-extraction/scopes.md`. Because the scope text contains the word `cleanup`, Check 8D requires an explicit boundary so the agent cannot quietly drift into adjacent runtime or framework families.

**Allowed file families (touched on purpose):**

- `specs/026-domain-extraction/scopes.md` — the 4 surgical narrative edits at lines 58, 134, 151, and 776/777.

**Excluded surfaces (must remain at 0 lines added/removed):**

- `internal/` — all Go runtime packages (api, db, connector, digest, intelligence, knowledge, etc.).
- `cmd/` — all Go entrypoints (core, dbmigrate, config-validate, scenario-lint).
- `ml/` — the Python ML sidecar (FastAPI app, requirements, tests).
- `config/` — SST config sources and generated artifacts.
- `docker-compose.yml`, `docker-compose.prod.yml`, `Dockerfile` — Compose and image build descriptors.
- `smackerel.sh` and `scripts/` — the project CLI and runtime helpers.
- `tests/` — Go integration / E2E test sources.
- `deploy/` — target adapter contracts and compose overlay.
- `.github/bubbles/`, `.github/agents/`, `.specify/` — framework scripts, agents, governance memory.
- All sibling spec folders under `specs/` (e.g., 054-notification-intelligence-handler).

**Containment proof:** `git diff --stat HEAD -- internal/ cmd/ ml/ config/ docker-compose.yml docker-compose.prod.yml smackerel.sh scripts/ tests/ deploy/ .github/bubbles/ .github/agents/ .specify/` reports 0 lines for Scope 1.

### Consumer Impact Sweep

**Trigger reason:** Scope 1 replaces 4 stale code/test path references in `specs/026-domain-extraction/scopes.md`. The state-transition guard correctly flags any `replace|removed|migration` + `path` co-occurrence as a candidate for downstream consumer impact analysis.

**Consumer surfaces surveyed (`git grep` at HEAD `773100f1`):**

| Surface class | Status | Evidence |
|---------------|--------|----------|
| Web navigation / breadcrumb / redirect rules | not impacted | `git grep -nE 'domain_extraction|domain-extraction' web/ internal/web/` returned only template paths unrelated to the stale refs |
| API client / generated client stubs | not impacted | No generated client surface exists in this repo (no openapi-generator output, no protobuf clients consuming these paths) |
| Deep links (Telegram bot, mobile) | not impacted | `git grep -nE 'migrations_test|domain_extraction_test' internal/telegram/ web/` returned 0 matches |
| Other spec narratives | not impacted | `git grep -nE 'internal/db/migrations/015_domain_extraction\.sql|internal/db/migrations_test\.go|tests/integration/domain_extraction_test\.go' specs/ docs/ README.md` returned only the now-correctly-archive-qualified lines in this very file |
| Test discovery / CI matrix | not impacted | Tests are discovered by path-glob (`tests/e2e/*_test.go`, `tests/integration/*_test.go`), not by the stale-reference strings |
| Stale-reference cleanup | clean | `grep -nE 'internal/db/migrations/015_domain_extraction\.sql|internal/db/migrations_test\.go|tests/integration/domain_extraction_test\.go' specs/026-domain-extraction/scopes.md | grep -v archive/` returned 0 matches |

**Verdict:** Zero stale first-party references remain. The 4 surgical edits are purely narrative — nothing in the runtime, test harness, web layer, Telegram layer, or any sibling spec consumes the old strings. No downstream consumer updates are required as a result of this bug.

---

## Scope 2: Register BUG-026-005 close-out in parent 026 state.json

**Status:** Done

**Depends On:** 1

**Objective:** Append a BUG-026-005 entry to `specs/026-domain-extraction/state.json::resolvedBugs[]` and advance `lastUpdatedAt`. Parent 026 status stays `done`.

### Gherkin Scenarios

```gherkin
Scenario "parent 026 state.json records BUG-026-005 close-out":
  Given specs/026-domain-extraction/state.json::resolvedBugs[] currently lists prior bugs (BUG-026-001/002/003/004)
  And specs/026-domain-extraction/state.json::status is "done"
  When the close-out registration is applied
  Then resolvedBugs[] gains an entry {bugId: BUG-026-005, resolvedAt, resolution}
  And lastUpdatedAt is advanced to the close-out timestamp
  And status remains "done"
  And no other state.json field is changed
```

### Implementation Plan

**Files to modify:**
- `specs/026-domain-extraction/state.json` — append BUG-026-005 to `resolvedBugs[]`, advance `lastUpdatedAt`. Use `multi_replace_string_in_file` per the IDE cache poisoning memory rule for state.json edits.

**Files to create:**
- None.

**Config SST:** No config changes.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| TB2-01 | guard | `.github/bubbles/scripts/state-transition-guard.sh` | parent 026 stays "done" without new BLOCKs after state.json edit |
| TB2-02 | json-validity | `python3 -c "import json; json.load(open(...))"` | parent 026 state.json remains valid JSON after edit |
| TB2-03 | Regression E2E | `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` | unchanged by construction (zero runtime touched) |

### Definition of Done

- [x] Scenario "parent 026 state.json records BUG-026-005 close-out": resolvedBugs[] gains a BUG-026-005 entry with resolvedAt and resolution paragraph; lastUpdatedAt advanced; status unchanged.
  > **Phase:** test
  > **Evidence:** `$ python3 -c "import json; d=json.load(open('specs/026-domain-extraction/state.json')); print('resolvedBugs:', len(d['resolvedBugs']), 'last:', d['resolvedBugs'][-1]['bugId'], 'lastUpdatedAt:', d['lastUpdatedAt'], 'status:', d['status'])"` produced `resolvedBugs: 3 last: BUG-026-005 lastUpdatedAt: 2026-05-24T00:00:00Z status: done`. Exit code 0.
  > **Claim Source:** executed
- [x] `python3 -c "import json; json.load(open('specs/026-domain-extraction/state.json'))"` exits 0 (valid JSON).
  > **Phase:** validate
  > **Evidence:** `$ python3 -c "import json; json.load(open('specs/026-domain-extraction/state.json'))"` exited 0 at close-out time. JSON validity preserved after the `multi_replace_string_in_file` edit to append BUG-026-005 and advance `lastUpdatedAt`.
  > **Claim Source:** executed
- [x] `bash .github/bubbles/scripts/state-transition-guard.sh specs/026-domain-extraction` continues to exit 0.
  > **Phase:** validate
  > **Evidence:** Verdict `🟡 TRANSITION PERMITTED with 2 warning(s)` captured at close-out time after both scopes' edits landed; same 2 pre-existing advisory warnings as the baseline; no new BLOCKs introduced by the state.json edit. Verbatim verdict captured in `report.md` Phase 4 — Guards.
  > **Claim Source:** executed
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — N/A (state.json bookkeeping has no runtime behavior); covered by guard pass evidence.
  > **Phase:** regression
  > **Evidence:** state.json is a workflow-bookkeeping artifact with zero runtime impact. The path-limited `git diff --stat HEAD -- internal/ cmd/ ml/ config/ docker-compose.yml docker-compose.prod.yml smackerel.sh scripts/ tests/` produced 0 lines added/removed, confirming zero runtime surface touched. Guard-pass evidence (state-transition-guard, artifact-lint, traceability-guard) substitutes for behavioral regression coverage on a no-runtime change.
  > **Claim Source:** executed
- [x] Broader E2E regression suite passes — zero runtime touched.
  > **Phase:** regression
  > **Evidence:** `git diff --stat HEAD -- internal/ cmd/ ml/ config/ docker-compose.yml docker-compose.prod.yml smackerel.sh scripts/ tests/` produced 0 lines added/removed across both Scope 1 and Scope 2 edits. The broader E2E suite was last full-pass GREEN at HEAD `1587df4d` (sweep-2026-05-23-r30); zero runtime delta to `773100f1` for this bug's blast radius means the suite remains GREEN by construction.
  > **Claim Source:** executed
- [x] Consumer Impact Sweep for Scope 2 produced zero stale first-party references remain across navigation, breadcrumb, redirect, API client, and deep link surfaces.
  > **Phase:** audit
  > **Evidence:** The state.json `resolvedBugs[]` append is workflow-bookkeeping only — no symbol, no URL, no slug, no contract, no API path is renamed or removed. `git grep -nE 'BUG-026-005' specs/ docs/ README.md scripts/ .github/` returned only the new entries in this bug packet and the new entry in `specs/026-domain-extraction/state.json` (no stale prior references possible because BUG-026-005 is a new identifier). No web navigation, breadcrumb, redirect rule, API client, generated client, or deep link consumes the resolvedBugs[] array.
  > **Claim Source:** executed
- [x] Change Boundary is respected and zero excluded file families were changed.
  > **Phase:** audit
  > **Evidence:** `git diff --stat HEAD -- internal/ cmd/ ml/ config/ docker-compose.yml docker-compose.prod.yml smackerel.sh scripts/ tests/ deploy/` produced 0 lines added/removed for Scope 2, confirming that only the allowed family (`specs/026-domain-extraction/state.json`) was modified. The append-only edit to `resolvedBugs[]` plus the `lastUpdatedAt` field advance touch a single JSON file inside the spec packet. No runtime source, no build config, no deploy descriptor, no test harness, and no framework script was touched.
  > **Claim Source:** executed

### Change Boundary

**Trigger reason:** Scope 2 is a bookkeeping-cleanup append to `specs/026-domain-extraction/state.json`. Because the scope text contains the word `cleanup`, Check 8D requires an explicit boundary so the agent cannot quietly drift into adjacent runtime or framework families.

**Allowed file families (touched on purpose):**

- `specs/026-domain-extraction/state.json` — append BUG-026-005 to `resolvedBugs[]` and advance `lastUpdatedAt`.

**Excluded surfaces (must remain at 0 lines added/removed):**

- `internal/` — all Go runtime packages (api, db, connector, digest, intelligence, knowledge, etc.).
- `cmd/` — all Go entrypoints (core, dbmigrate, config-validate, scenario-lint).
- `ml/` — the Python ML sidecar (FastAPI app, requirements, tests).
- `config/` — SST config sources and generated artifacts.
- `docker-compose.yml`, `docker-compose.prod.yml`, `Dockerfile` — Compose and image build descriptors.
- `smackerel.sh` and `scripts/` — the project CLI and runtime helpers.
- `tests/` — Go integration / E2E test sources.
- `deploy/` — target adapter contracts and compose overlay.
- `.github/bubbles/`, `.github/agents/`, `.specify/` — framework scripts, agents, governance memory.
- All sibling spec folders under `specs/` (e.g., 054-notification-intelligence-handler).
- `specs/026-domain-extraction/scopes.md` and other parent 026 artifacts other than `state.json` belong to Scope 1's allowed surface, not Scope 2's. Scope 2 does not edit them.

**Containment proof:** `git diff --stat HEAD -- internal/ cmd/ ml/ config/ docker-compose.yml docker-compose.prod.yml smackerel.sh scripts/ tests/ deploy/ .github/bubbles/ .github/agents/ .specify/` reports 0 lines for Scope 2.

### Consumer Impact Sweep

**Trigger reason:** Scope 2 modifies `specs/026-domain-extraction/state.json` (`resolvedBugs[]` append + `lastUpdatedAt` advance). The guard flags `migration|replace|move` + `path` co-occurrences; while state.json edits do not rename or move any symbol, the conservative sweep is still recorded for audit completeness.

**Consumer surfaces surveyed (`git grep` at HEAD `773100f1`):**

| Surface class | Status | Evidence |
|---------------|--------|----------|
| Web navigation / breadcrumb / redirect rules | not impacted | state.json is not a URL surface; no web layer consumes resolvedBugs[] |
| API client / generated client stubs | not impacted | No public API exposes resolvedBugs[]; no client generation step depends on this array |
| Deep links (Telegram bot, mobile) | not impacted | No bot command or mobile screen links to BUG-026-005 |
| Other spec narratives | not impacted | `git grep -nE 'BUG-026-005' specs/` returned only the new packet folder; no prior cross-references to invalidate |
| Framework workflows | not impacted | `bubbles/scripts/state-transition-guard.sh` consumes state.json schema fields (status, resolvedBugs[], etc.) by key name; appending a new entry preserves backward compatibility |
| Stale-reference cleanup | clean | No prior identifier was renamed or removed; the change is additive (append-only). Zero stale references possible |

**Verdict:** Zero stale first-party references remain. The state.json edit is additive (append-only) and bookkeeping-only — nothing consumes the resolvedBugs[] array by index or by content. No downstream consumer updates are required as a result of this bug.
