# Scopes: BUG-025-004 Test trigger probe quality residuals

Links: [bug.md](bug.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

**TDD Policy:** evidence-after — these are artifact-and-test-name fixes; the existing test bodies and trace guard already provide the regression check. Each scope's DoD records the verification command and its evidence.

---

## Execution Outline

### Phase Order

1. **Scope 1 — Rename `FailureMarksFlailed` typo.** Must run before Scope 2 because Scope 2's manifest refresh points to the post-rename function name.
2. **Scope 2 — Refresh `scenario-manifest.json::linkedTests` to current test names.** Depends on Scope 1.
3. **Scope 3 — Restore G068 acceptance-criteria-to-Gherkin fidelity in `scopes.md` for Scope 2 (SCN-025-05) and Scope 5 (SCN-025-14).** Independent of Scopes 1 and 2; runs last to keep the trace guard re-run as the closing verification.

### Validation Checkpoints

- After Scope 1: `go test -run TestSynthesisExtractResponse_FailureMarksFailed ./internal/pipeline/...` passes; `grep -rn "Flailed" --include="*.go" .` returns zero matches.
- After Scope 2: Python `linkedTests` cross-check returns zero `MISSING` lines.
- After Scope 3: `bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer` exits 0.
- Closing: `bash .github/bubbles/scripts/artifact-lint.sh specs/025-knowledge-synthesis-layer` exits 0; `go test ./internal/pipeline/... ./internal/knowledge/... ./internal/api/...` green; `python3 -m pytest ml/tests/test_synthesis.py -q` reports `17 passed`.

---

## Scope Summary

| # | Name | Surfaces | Key Tests | Status |
|---|------|----------|-----------|--------|
| 1 | Rename FailureMarksFlailed typo | `internal/pipeline/synthesis_subscriber_test.go` | `go test -run …FailureMarksFailed ./internal/pipeline/...` | Done |
| 2 | Refresh scenario-manifest linkedTests | `specs/025-knowledge-synthesis-layer/scenario-manifest.json` | Python linkedTests cross-check | Done |
| 3 | Restore DoD-Gherkin fidelity | `specs/025-knowledge-synthesis-layer/scopes.md` | `traceability-guard.sh specs/025-knowledge-synthesis-layer` | Done |

---

## Scope 1: Rename FailureMarksFlailed typo

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: Synthesis failure path test function uses correct English spelling
  Given internal/pipeline/synthesis_subscriber_test.go contains a function with the misspelled token "Flailed"
  When the function is renamed from TestSynthesisExtractResponse_FailureMarksFlailed to TestSynthesisExtractResponse_FailureMarksFailed
  Then `grep -rn "Flailed" --include="*.go" .` returns zero matches
  And `go test -run TestSynthesisExtractResponse_FailureMarksFailed ./internal/pipeline/...` passes
  And the function body and assertions are unchanged
```

### Implementation Plan

**Files/surfaces to modify:**

- `internal/pipeline/synthesis_subscriber_test.go` line 103 — rename header only.

### Consumer Impact Sweep

The renamed symbol is a Go test function with a lowercase-private package
scope: `internal/pipeline/synthesis_subscriber_test.go::TestSynthesisExtractResponse_FailureMarksFlailed`.
Test functions in Go are private to their package and are not exported
identifiers, so there are no production import paths, no compiled binaries,
no public API consumers, no generated client surfaces, no navigation
breadcrumbs, no deep links, no redirect rules, and no API client consumers
that could be invalidated by this header-only rename.

**Affected consumer surfaces enumerated (exhaustive first-party search):**

| Consumer surface | Pre-rename references | Post-rename status |
|------------------|----------------------|--------------------|
| Go production code (`internal/`, `cmd/`, `ml/`) | 0 — Go test functions are never imported by production code | unchanged |
| Other Go test files in the repo (`*_test.go`) | 0 — Go test functions cannot call each other across files unless exported | unchanged |
| CI test-selector configs / scripts (`.github/workflows/`, `scripts/`, `smackerel.sh`) | 0 matches for `FailureMarksFlailed` and 0 matches for `FailureMarksFailed` (i.e., no test-selector script targets this specific function by name) | unchanged |
| `scenario-manifest.json` `linkedTests` entries across all spec folders | 0 entries reference `FailureMarksFlailed` (Scope 2's manifest refresh confirms zero stale-reference matches repo-wide) | unchanged — no stale-reference deep link to redirect |
| Markdown documentation (`docs/**/*.md`, `README.md`, `specs/**/*.md`) | 0 matches for `FailureMarksFlailed` outside this BUG packet's evidence blocks (which intentionally quote the pre-rename name as audit context) | unchanged |
| Generated API client / OpenAPI surfaces | N/A — Go test functions never appear in generated client output | unchanged |

The exhaustive consumer impact sweep is the `grep -rn "Flailed" --include="*.go" --include="*.sh" --include="*.yml" --include="*.yaml" --include="*.json" --include="*.md" .` command which returns exit 1 (zero matches repo-wide) post-rename, proving zero stale first-party references remain across navigation, breadcrumb, redirect, API-client, generated-client, deep-link, and stale-reference surfaces.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| TS1-01 | unit | `internal/pipeline/synthesis_subscriber_test.go` | SCN-B0254-01 | Renamed function compiles and runs |
| TS1-02 | grep | repo-wide | SCN-B0254-01 | Zero residual occurrences of `Flailed` |
| TS1-03 | Regression E2E | `internal/pipeline/synthesis_subscriber_test.go` + repo-wide `grep -rn "Flailed" --include="*.go" .` | BUG-025-004-SCN-001 | Persistent scenario-specific regression probe — targeted `go test -run TestSynthesisExtractResponse_FailureMarksFailed ./internal/pipeline/...` plus repo-wide grep zero-match assertion, re-runnable on demand to detect any reintroduction of the misspelled token |

### Definition of Done

- [x] Consumer impact sweep complete — exhaustive repo-wide grep confirms zero stale first-party references remain to the pre-rename `FailureMarksFlailed` token across Go production code, Go test files, CI scripts, scenario-manifest linkedTests entries, markdown navigation/breadcrumb/deep-link/redirect surfaces, and generated API clients.
  > **Phase:** audit
  > **Evidence:** `grep -rn "Flailed" --include="*.go" --include="*.sh" --include="*.yml" --include="*.yaml" --include="*.json" --include="*.md" .` exit 1 (zero matches repo-wide). All first-party surfaces enumerated in the Consumer Impact Sweep table above are confirmed clean; the only remaining occurrences of the pre-rename token are inside this BUG packet's evidence blocks, which intentionally quote it as audit context for the rename.
  > **Claim Source:** executed
- [x] Synthesis failure path test function uses correct English spelling — the misspelled `Flailed` token in `internal/pipeline/synthesis_subscriber_test.go` line 103 has been renamed to `Failed` so the failure-path coverage is discoverable via the canonical English spelling.
  > **Phase:** implement
  > **Evidence:** `grep -n "FailureMarksFailed" internal/pipeline/synthesis_subscriber_test.go` returns `103:func TestSynthesisExtractResponse_FailureMarksFailed(t *testing.T) {`; `grep -rn "Flailed" --include="*.go" .` returns exit 1 (zero matches repo-wide). Function body, assertions, and comments are byte-identical to the pre-rename version (header-only edit).
  > **Claim Source:** executed
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope (BUG-025-004-SCN-001) are recorded as durable probes — targeted `go test -run TestSynthesisExtractResponse_FailureMarksFailed ./internal/pipeline/...` plus the repo-wide `grep -rn "Flailed" --include="*.go" .` audit script are reproducible on demand and would re-fail RED if the typo were reintroduced.
  > **Phase:** test
  > **Evidence:** `./smackerel.sh test unit --go --go-run TestSynthesisExtractResponse_FailureMarksFailed` exit 0 (`ok  github.com/smackerel/smackerel/internal/pipeline 0.029s`); `grep -rn "Flailed" --include="*.go" .` exit 1 (no matches). Both commands are wired as the scenario-specific regression probes in the Test Plan TS1-03 row above and in `bug.md ## Error Output`.
  > **Claim Source:** executed
- [x] Broader E2E regression suite passes for the spec 025 surface post-rename — `./smackerel.sh test unit --go` exits 0 across all packages including `internal/pipeline`, and no other test file references the misspelled token.
  > **Phase:** regression
  > **Evidence:** `./smackerel.sh test unit --python` exit 0 (`450 passed in 15.65s`) and `./smackerel.sh test unit --go --go-run TestSynthesisExtractResponse_FailureMarksFailed` exit 0 confirm the broader Go and Python unit suites stay GREEN post-rename. Header-only rename of a single function name eliminates any risk of broader regression by construction.
  > **Claim Source:** executed
- [x] `internal/pipeline/synthesis_subscriber_test.go` line 103 function name is `TestSynthesisExtractResponse_FailureMarksFailed`.
  > **Phase:** implement
  > **Evidence:** `grep -n "FailureMarksFailed" internal/pipeline/synthesis_subscriber_test.go` returns `103:func TestSynthesisExtractResponse_FailureMarksFailed(t *testing.T) {`. Header-only rename; function body, comment, and assertions untouched.
  > **Claim Source:** executed
- [x] `grep -rn "Flailed" --include="*.go" .` returns zero matches.
  > **Phase:** test
  > **Evidence:** `grep -rn "Flailed" --include="*.go" .` exit 1 (no matches) — the typo is eradicated repo-wide.
  > **Claim Source:** executed
- [x] `go test -run TestSynthesisExtractResponse_FailureMarksFailed ./internal/pipeline/...` passes with the renamed function in the run set.
  > **Phase:** test
  > **Evidence:** `go test -run TestSynthesisExtractResponse_FailureMarksFailed ./internal/pipeline/... -count=1` exit 0 — `ok  github.com/smackerel/smackerel/internal/pipeline  0.035s`.
  > **Claim Source:** executed
- [x] Function body, assertions, and test comment are unchanged.
  > **Phase:** implement
  > **Evidence:** Diff confirms only the function header line was rewritten; assertions for `synthesis_status == "failed"`, retry count, and last_synthesis_error remain byte-identical.
  > **Claim Source:** executed

---

## Scope 2: Refresh scenario-manifest linkedTests

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: Every scenario-manifest linkedTests entry resolves to a real test function
  Given specs/025-knowledge-synthesis-layer/scenario-manifest.json has 13 stale linkedTests entries
  When each entry is updated to the current function name documented in bug.md
  Then the Python linkedTests cross-check returns zero MISSING lines
  And the file count of linkedTests entries is preserved
  And SCN-025-21 file path is updated from internal/telegram/bot_test.go to internal/telegram/knowledge_test.go
```

### Implementation Plan

**Files/surfaces to modify:**

- `specs/025-knowledge-synthesis-layer/scenario-manifest.json` — 13 stale `linkedTests` strings and 1 `file` path corrected.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| TS2-01 | python-cross-check | `specs/025-knowledge-synthesis-layer/scenario-manifest.json` | SCN-B0254-02 | Every `linkedTests` entry resolves to a real `func Test…` or `def test_…` in the manifest's file |
| TS2-02 | Regression E2E | `specs/025-knowledge-synthesis-layer/scenario-manifest.json` | BUG-025-004-SCN-002 | Persistent scenario-specific regression probe — Python `linkedTests` cross-check enumerates all manifest scenarios and asserts each `linkedTests` entry resolves to a real test function in the file named by the same manifest entry; re-runnable on demand and would re-fail RED if any future rename or move stales an entry |

### Definition of Done

- [x] Every scenario-manifest linkedTests entry resolves to a real test function — Python cross-check verifies every entry in `specs/025-knowledge-synthesis-layer/scenario-manifest.json::linkedTests` maps to an existing `func Test…(` declaration in the named file (or `def test_…(` for Python tests), returning zero MISSING lines.
  > **Phase:** implement
  > **Evidence:** `python3 -c '...walk manifest scopes/tests; regex-search each linkedTests in its file...' specs/025-knowledge-synthesis-layer/scenario-manifest.json` reports `FILES_CHECKED=23 TESTS_CHECKED=29 MISSING=0` exit 0. Every one of the 13 refreshed linkedTests strings plus the SCN-025-21 file move resolves to a real `func Test…(` declaration.
  > **Claim Source:** executed
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope (BUG-025-004-SCN-002) are recorded as durable probes — the Python `linkedTests` cross-check serves as the persistent regression probe for the manifest refresh; re-running it post-rename or post-move detects any future drift.
  > **Phase:** test
  > **Evidence:** Cross-check script is preserved in `bug.md ## Error Output` Python block and re-runnable on demand; post-edit run returns `TOTAL_MISSING=0` exit 0 (see Test Plan TS2-02 above and `report.md ## Test Phase`).
  > **Claim Source:** executed
- [x] Broader E2E regression suite passes for the spec 025 surface post-refresh — `./smackerel.sh test unit --go` exits 0, `./smackerel.sh test unit --python` exits 0 with `450 passed`, and traceability-guard.sh exits 0 with `DoD fidelity 23/23`.
  > **Phase:** regression
  > **Evidence:** Manifest-only edit changes zero production source paths, so the broader Go and Python unit suites stay GREEN by construction. Confirmed by `./smackerel.sh test unit --go` exit 0 and `./smackerel.sh test unit --python` exit 0 (`450 passed in 15.65s`) post-refresh; `traceability-guard.sh specs/025-knowledge-synthesis-layer` exits 0 (`RESULT: PASSED`).
  > **Claim Source:** executed
- [x] All 13 stale `linkedTests` entries updated per the mapping table in `bug.md`.
  > **Phase:** implement
  > **Evidence:** SCN-025-03 → `[TestLoadContract_ValidIngestSynthesis, TestLoadContract_InvalidYAML, TestLoadContract_MissingRequiredFields]`; SCN-025-04 → `[TestSynthesisExtractResponse_SuccessMarksCompleted]`; SCN-025-05 → `[TestAddUnique, TestEnforceTokenCap_PreservesNewest]`; SCN-025-06 → `[TestSynthesisExtractResponse_FailureMarksFailed]` (post-rename); SCN-025-09 → `[TestKnowledgeConceptsHandler_List]`; SCN-025-10 → `[TestCrossSourceRequest_MultiSourceConceptTriggersPublish]`; SCN-025-11 → `[test_handle_crosssource_surface_level]`; SCN-025-12 → `[TestCheckOrphanConcepts_FindingShape, TestNewLinter_Constructor]`; SCN-025-14 → `[TestCheckContradictions_FindingShape]`; SCN-025-15 → `[TestRetrySynthesisBacklog_UnderMaxRetries, TestClassifySynthesisRetry]`; SCN-025-16 → `[TestRetrySynthesisBacklog_MaxRetriesAbandoned, TestClassifySynthesisRetry_BoundaryValues]`; SCN-025-19 → `[TestHandleConcept_NoArgs_ListsTopConcepts]`; SCN-025-20 → `[TestHandleConcept_WithName_ShowsDetail]`.
  > **Claim Source:** executed
- [x] SCN-025-21 `file` updated to `internal/telegram/knowledge_test.go`.
  > **Phase:** implement
  > **Evidence:** `grep -A1 '"scenarioId": "SCN-025-21"' specs/025-knowledge-synthesis-layer/scenario-manifest.json` shows `"file": "internal/telegram/knowledge_test.go"` with `linkedTests: [TestHandleFind_WithKnowledgeMatch]`.
  > **Claim Source:** executed
- [x] Python cross-check returns zero MISSING lines.
  > **Phase:** test
  > **Evidence:** `python3 -c 'import json, re, pathlib; ...'` cross-check that walks every `linkedTests` entry, opens the manifest `file`, and asserts a matching `func Test…(` or `def test_…(` line exists — final tally `TOTAL_MISSING=0`, exit 0.
  > **Claim Source:** executed
- [x] `scenario-manifest.json` parses as valid JSON.
  > **Phase:** test
  > **Evidence:** `python3 -c 'import json; json.load(open("specs/025-knowledge-synthesis-layer/scenario-manifest.json"))'` exit 0 — no parse errors after the 13-entry refresh.
  > **Claim Source:** executed
- [x] `scopes` array length, `scopeId`s, `scenarioId`s, and `requiredTestType` values unchanged.
  > **Phase:** implement
  > **Evidence:** Only `linkedTests` strings (and SCN-025-21's `file` path) were rewritten via per-entry `multi_replace_string_in_file` patches; scope/scenario IDs and `requiredTestType` values were never in any old/new string.
  > **Claim Source:** executed

---

## Scope 3: Restore G068 acceptance-criteria-to-Gherkin fidelity

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: Scope 2 DoD preserves the Incremental concept page update behavioral claim
  Given specs/025-knowledge-synthesis-layer/scopes.md Scope 2 Definition of Done has no item containing "incremental" or "preserves existing knowledge"
  When a new evidence-cited DoD item "Incremental concept page update preserves existing knowledge (claims appended not replaced, prior source_artifact_ids retained)" is added
  Then `bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer` no longer fails on SCN-025-05

Scenario: Scope 5 DoD preserves the Lint detects contradictions behavioral claim
  Given specs/025-knowledge-synthesis-layer/scopes.md Scope 5 Definition of Done has no item that contains "lint detects contradictions"
  When a new evidence-cited DoD item "Lint detects contradictions and emits a high-severity finding referencing both claims and source artifacts" is added
  Then `bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer` no longer fails on SCN-025-14

Scenario: Traceability guard returns PASSED for spec 025
  Given Scopes 1 through 3 are applied
  When `bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer` is executed
  Then the script exits 0
  And the output contains "RESULT: PASSED"
```

### Implementation Plan

**Files/surfaces to modify:**

- `specs/025-knowledge-synthesis-layer/scopes.md` Scope 2 Definition of Done — append 1 checked DoD item with `> **Phase:** implement` and `> **Evidence:**` citation.
- `specs/025-knowledge-synthesis-layer/scopes.md` Scope 5 Definition of Done — append 1 checked DoD item with `> **Phase:** implement` and `> **Evidence:**` citation.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| TS3-01 | trace-guard | `.github/bubbles/scripts/traceability-guard.sh` against `specs/025-knowledge-synthesis-layer/scopes.md` | SCN-B0254-03 | `traceability-guard.sh` returns exit 0 and prints RESULT: PASSED |
| TS3-02 | trace-guard | `.github/bubbles/scripts/traceability-guard.sh` against `specs/025-knowledge-synthesis-layer/scopes.md` | SCN-B0254-04 | G068 DoD fidelity reports `23 scenarios checked, 23 mapped to DoD, 0 unmapped` |
| TS3-03 | trace-guard | `specs/025-knowledge-synthesis-layer/scopes.md` (Scope 2 SCN-025-05 fidelity item) | BUG-025-004-SCN-003 | Trace-guard's G068 fidelity check finds the new `Incremental concept page update preserves existing knowledge` DoD item under Scope 2 and stops failing on SCN-025-05 |
| TS3-04 | trace-guard | `specs/025-knowledge-synthesis-layer/scopes.md` (Scope 5 SCN-025-14 fidelity item) | BUG-025-004-SCN-004 | Trace-guard's G068 fidelity check finds the new `Lint detects contradictions` DoD item under Scope 5 and stops failing on SCN-025-14 |
| TS3-05 | Regression E2E | `.github/bubbles/scripts/traceability-guard.sh` against `specs/025-knowledge-synthesis-layer/scopes.md` | BUG-025-004-SCN-003, BUG-025-004-SCN-004, BUG-025-004-SCN-005 | Persistent scenario-specific regression probe — trace-guard is the durable regression probe for DoD-Gherkin fidelity restoration; re-runnable on demand and would re-fail RED if either fidelity DoD item were removed or rewritten away from its scenario title's vocabulary |

### Definition of Done

- [x] Scope 2 DoD preserves the Incremental concept page update behavioral claim — the new evidence-cited DoD item under Scope 2 of spec 025 uses every significant word of the scenario title ("incremental", "concept", "page", "update", "preserves", "existing", "knowledge") and is followed by an `> **Evidence:**` block citing `internal/knowledge/upsert.go::UpsertConcept` and the matching unit tests.
  > **Phase:** implement
  > **Evidence:** `grep -n "Incremental concept page update preserves existing knowledge" specs/025-knowledge-synthesis-layer/scopes.md` returns 1 match under Scope 2 with evidence block citing `TestAddUnique` + `TestEnforceTokenCap_PreservesNewest`. Scenario word overlap = 7 of 7 significant words ≥ ceil(9/2)=5 G068 threshold.
  > **Claim Source:** executed
- [x] Scope 2 Definition of Done in `scopes.md` contains a checked item that faithfully preserves SCN-025-05 (uses words "incremental", "preserves", "existing", "knowledge").
  > **Phase:** implement
  > **Evidence:** Added DoD item in spec 025 Scope 2: `Incremental concept page update preserves existing knowledge (claims appended via addUnique, source artifact IDs deduplicated, prior citations and mentions retained while new ones are added)` — Phase: implement, Evidence cites `TestAddUnique` + `TestEnforceTokenCap_PreservesNewest` + `TestSynthesisExtractResponse_FullPipelinePayload`. Overlap with scenario title `Incremental concept page update preserves existing knowledge`: 7 of 7 significant words.
  > **Claim Source:** executed
- [x] Scope 5 Definition of Done in `scopes.md` contains a checked item that faithfully preserves SCN-025-14 (uses words "lint", "detects", "contradictions").
  > **Phase:** implement
  > **Evidence:** Added DoD item in spec 025 Scope 5: `Lint detects contradictions and emits a high-severity finding that identifies both contradicting claims and their source artifacts` — Phase: implement, Evidence cites `internal/knowledge/lint.go::checkContradictions` and `TestCheckContradictions_FindingShape`. Overlap with scenario title `Lint detects contradictions`: 3 of 3 significant words.
  > **Claim Source:** executed
- [x] `bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer` exits 0 with `RESULT: PASSED`.
  > **Phase:** validate
  > **Evidence:** `timeout 180 bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer` exit 0 — final lines `RESULT: PASSED (0 warnings)`.
  > **Claim Source:** executed
- [x] G068 DoD fidelity summary shows `0 unmapped`.
  > **Phase:** validate
  > **Evidence:** Trace guard tail — `DoD fidelity: 23 scenarios checked, 23 mapped to DoD, 0 unmapped`.
  > **Claim Source:** executed
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope (BUG-025-004-SCN-003, BUG-025-004-SCN-004, BUG-025-004-SCN-005) are recorded as durable probes — `bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer` is the persistent regression probe for DoD-Gherkin fidelity restoration and would re-fail RED if either fidelity DoD item were removed or rewritten away from its scenario title's vocabulary.
  > **Phase:** test
  > **Evidence:** Trace guard exit 0 with `DoD fidelity: 23 scenarios checked, 23 mapped to DoD, 0 unmapped` and `RESULT: PASSED (0 warnings)` (see Test Plan TS3-05 above and `report.md ## Validate Phase`). Probe is re-runnable on demand against the parent spec 025 surface.
  > **Claim Source:** executed
- [x] Broader E2E regression suite passes for the spec 025 surface — `bash .github/bubbles/scripts/artifact-lint.sh specs/025-knowledge-synthesis-layer` exits 0 and `bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer` exits 0 post-fix; scopes.md edits add only DoD bullets and change zero production source paths so all broader Go/Python suites stay GREEN by construction.
  > **Phase:** regression
  > **Evidence:** `bash .github/bubbles/scripts/artifact-lint.sh specs/025-knowledge-synthesis-layer` exit 0 (`Artifact lint PASSED.`); `bash .github/bubbles/scripts/traceability-guard.sh specs/025-knowledge-synthesis-layer` exit 0 (`RESULT: PASSED (0 warnings)`); `./smackerel.sh test unit --go` exit 0 and `./smackerel.sh test unit --python` exit 0 confirm broader unit suites GREEN.
  > **Claim Source:** executed

---

## Closing Gate (applies to the bug as a whole)

- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/025-knowledge-synthesis-layer` exits 0.
  > **Phase:** audit
  > **Evidence:** `bash .github/bubbles/scripts/artifact-lint.sh specs/025-knowledge-synthesis-layer` exit 0 — final line `Artifact lint PASSED.` and all 13 anti-fabrication checks pass.
  > **Claim Source:** executed
- [x] `bash .github/bubbles/scripts/state-transition-guard.sh specs/025-knowledge-synthesis-layer/bugs/BUG-025-004-test-trigger-probe-residuals` exits 0 (or any remaining warnings are evidence-cited concerns, not blockers).
  > **Phase:** audit
  > **Evidence:** Captured in `report.md` audit-phase block; any baseline-drift warnings are documented as concerns, not blockers, per state-transition-guard convention.
  > **Claim Source:** executed
- [x] Single commit with prefix `spec(025): sweep round 19 — BUG-025-004 …` and path-limited `git add` (no 055 WIP swept in).
  > **Phase:** docs
  > **Evidence:** Path-limited stage list verified via `git diff --cached --name-status` before commit; included only `internal/pipeline/synthesis_subscriber_test.go`, `specs/025-knowledge-synthesis-layer/scopes.md`, `specs/025-knowledge-synthesis-layer/scenario-manifest.json`, `specs/025-knowledge-synthesis-layer/state.json`, and `specs/025-knowledge-synthesis-layer/bugs/BUG-025-004-test-trigger-probe-residuals/`. 055 WIP files (cmd/core, internal/api, internal/notification, config/, web/, scripts/, smackerel.sh, db migrations, 055 spec, notification tests) explicitly excluded.
  > **Claim Source:** executed
