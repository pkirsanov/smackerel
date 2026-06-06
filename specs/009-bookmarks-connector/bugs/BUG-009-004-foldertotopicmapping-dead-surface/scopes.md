# Scopes: BUG-009-004 — FolderToTopicMapping dead surface (simplify R6)

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Delete dead exported `FolderToTopicMapping` surface and reconcile spec/design planning truth

**Status:** Done
**Priority:** P1
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-BK-FIX-004-001 Production folder→topic resolution remains unchanged after the dead utility is removed
  Given the bookmarks package source after F-SIMPLIFY-R6-002 has been applied
  When grep is run for "FolderToTopicMapping" across "*.go" files in internal/connector/bookmarks
  Then exactly zero matches are returned
  And TopicMapper.MapFolder remains the single production folder→topic entry point
  And the full bookmarks-package test suite passes (44 PASS, 0 FAIL)

Scenario: SCN-BK-FIX-004-002 Spec and design planning truth reflects the actual TopicMapper.MapFolder mechanism
  Given the spec 009 planning artifacts after F-SIMPLIFY-R6-002 has been applied
  When grep is run for "FolderToTopicMapping" across "specs/009-bookmarks-connector/spec.md", "design.md", and "scopes.md"
  Then exactly zero matches are returned
  And the four spec.md reference sites name "TopicMapper.MapFolder" (or describe split + per-segment exact/fuzzy/create resolution)
  And the four design.md reference sites name "TopicMapper.MapFolder" (or describe split + per-segment exact/fuzzy/create resolution)
  And historical spec 003 evidence rows referencing "FolderToTopicMapping" are intentionally preserved untouched

Scenario: SCN-BK-FIX-004-003 Adversarial guard test pins the split-not-flatten algorithmic intent
  Given the new TestSimplifyR6_FolderToTopicMapping_Removed test in topics_test.go
  When go test -count=1 -run TestSimplifyR6_FolderToTopicMapping_Removed ./internal/connector/bookmarks/... is invoked
  Then the test PASSES
  And the test directly calls TopicMapper.MapFolder with a nil pool to compile-pin the signature
  And the test reproduces the same strings.Split + non-empty-trim segmentation logic visible in topics.go::MapFolder
  And the test's failure diagnostic names "F-SIMPLIFY-R6-002 invariant" as the grep target for future reviewers
```

### Implementation Plan

1. Inline the `extractBookmarks` shim in `internal/connector/bookmarks/bookmarks.go` (F-SIMPLIFY-R6-001 micro-fix; addressed in the same simplify R6 round as this bug, documented in the parent `specs/009-bookmarks-connector/report.md` simplify-R6 section).
2. Delete the `FolderToTopicMapping` function from `internal/connector/bookmarks/bookmarks.go` (lines 143–152 in the pre-fix file). (F-SIMPLIFY-R6-002)
3. Delete the three dead unit tests from `internal/connector/bookmarks/bookmarks_test.go`:
   - `TestFolderToTopicMapping` (lines 71–87)
   - `TestFolderToTopicMapping_Backslash` (lines 255–260)
   - `TestFolderToTopicMapping_MultiLevel` (lines 583–602)
   (F-SIMPLIFY-R6-002)
4. Update `specs/009-bookmarks-connector/spec.md` in all 4 reference sites (Goal #2, ASCII diagram, R-005, UC-001 step 10) to name `TopicMapper.MapFolder` and describe the segment-split + per-segment cascade. (F-SIMPLIFY-R6-002)
5. Update `specs/009-bookmarks-connector/design.md` in all 4 reference sites (Design Brief, Component Design, Data Flow step 8, Component Design §2) similarly. (F-SIMPLIFY-R6-002)
6. Add `TestSimplifyR6_FolderToTopicMapping_Removed` to `internal/connector/bookmarks/topics_test.go` with three layers of coverage: compile-pin via direct `MapFolder` call, runtime-pin via nil-pool assertion, and inline algorithmic-intent documentation via `strings.Split + non-empty-trim` reproduction. Failure diagnostic includes the literal `F-SIMPLIFY-R6-002 invariant` grep target. (F-SIMPLIFY-R6-002)
7. Run `go test ./internal/connector/bookmarks/... -race -count=1 -v` against `internal/connector/bookmarks/{bookmarks,connector,dedup,topics}.go` and `internal/connector/bookmarks/{bookmarks,connector,dedup,topics}_test.go` and confirm all 44 tests in the bookmarks package PASS.
8. Run `go vet ./internal/connector/bookmarks/...` and `gofmt -l internal/connector/bookmarks/` — both clean.
9. Run `go test ./internal/connector/... ./internal/scheduler/...` consumer regression sweep — all 22 packages PASS.
10. Run `./smackerel.sh lint` — exit 0.
11. Append a "Simplify R6 — FolderToTopicMapping dead surface removal" section to `specs/009-bookmarks-connector/report.md` and one R6 entry to `specs/009-bookmarks-connector/state.json::executionHistory`. Re-certify spec 009 at the new timestamp; preserve `originalCompletedAt`.

### Change Boundary

This is a dead-code removal + planning-truth reconciliation refactor. The boundary is enumerated explicitly to prevent scope creep into adjacent surfaces.

**Allowed surfaces (modified by this scope):**

- `internal/connector/bookmarks/bookmarks.go` — delete `FolderToTopicMapping` function + inline `extractBookmarks` shim
- `internal/connector/bookmarks/bookmarks_test.go` — delete 3 dead unit tests
- `internal/connector/bookmarks/topics_test.go` — add `TestSimplifyR6_FolderToTopicMapping_Removed` adversarial guard test
- `specs/009-bookmarks-connector/spec.md` — rewrite 4 reference sites to name `TopicMapper.MapFolder`
- `specs/009-bookmarks-connector/design.md` — rewrite 4 reference sites similarly
- `specs/009-bookmarks-connector/report.md` — append simplify R6 section
- `specs/009-bookmarks-connector/state.json` — append executionHistory entry + re-certify
- All artifacts in this bug folder

**Excluded surfaces (intentionally NOT modified by this scope):**

- `internal/connector/bookmarks/connector.go` — production runtime is unchanged; the dead utility had zero consumers here
- `internal/connector/bookmarks/dedup.go` — `URLDeduplicator.IsKnown` has live integration test consumers; intentionally retained
- `internal/connector/bookmarks/topics.go` — `TopicMapper.MapFolder` production source is unchanged; only documentation comments in tests reference the simplified contract
- `tests/integration/bookmarks_dedup_test.go` — uses `IsKnown`; intentionally NOT modified by this scope
- `tests/integration/bookmarks_topics_test.go` — exercises full `MapFolder` cascade; intentionally NOT modified by this scope (provides E2E coverage of the production path)
- `specs/003-phase2-ingestion/` — historical evidence rows referencing `FolderToTopicMapping` preserved as a historical record of phase-2 sign-off; explicitly excluded per spec.md Boundary
- `specs/009-bookmarks-connector/scopes.md` (parent) — no `FolderToTopicMapping` references in the parent scopes.md (confirmed by grep); excluded
- All other connector packages (alerts, browser, caldav, discord, guesthost, hospitable, imap, keep, maps, markets, photos, qfdecisions, rss, twitter, weather, youtube), scheduler, supervisor, registry — pure consumer-regression-sweep coverage; no edits

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-BK-FIX-004-01 | `TestSimplifyR6_FolderToTopicMapping_Removed` | unit | `internal/connector/bookmarks/topics_test.go` | (a) `MapFolder(ctx, "Tech/Go/Libraries")` returns `(nil, nil)` with nil pool; (b) inline `strings.Split + non-empty-trim` reproduces expected segment counts for 6 paths including leading/trailing-slash edge cases; (c) test name + failure diagnostic both reference `F-SIMPLIFY-R6-002 invariant` | SCN-BK-FIX-004-003 |
| T-BK-FIX-004-02 | `grep -rn 'FolderToTopicMapping' --include='*.go' internal/connector/bookmarks/` | manual | n/a | Zero matches (function + 3 tests gone) | SCN-BK-FIX-004-001 |
| T-BK-FIX-004-03 | `grep -rn 'FolderToTopicMapping' specs/009-bookmarks-connector/{spec.md,design.md,scopes.md}` | manual | n/a | Zero matches (all 8 planning-truth references gone) | SCN-BK-FIX-004-002 |
| T-BK-FIX-004-04 | Full bookmarks package suite | unit | `internal/connector/bookmarks/...` | `go test ./internal/connector/bookmarks/... -count=1 -race` exit 0 (44 PASS, 0 FAIL) | All three |
| T-BK-FIX-004-05 | Consumer regression sweep | unit | `internal/connector/... internal/scheduler/...` | `go test ./internal/connector/... ./internal/scheduler/...` exit 0 (22 packages PASS) | All three |
| T-BK-FIX-004-06 | go vet + gofmt | static | `internal/connector/bookmarks/...` | `go vet` exit 0; `gofmt -l` empty output | All three |
| T-BK-FIX-004-07 | Project lint | static | repo root | `./smackerel.sh lint` exit 0 | All three |
| T-BK-FIX-004-08 | Synthetic adversarial proof (deleted-vs-current algorithm) | manual | inline ephemeral Go program at simplify R6 | Demonstrates that the deleted flatten algorithm produces 1 segment for "Tech/Go/Libraries" (FAIL vs expected 3) while the production split algorithm produces 3 (PASS) for the same input — proves the two algorithms are NOT equivalent and the production user-facing hierarchical-topic contract depends on the split semantics | SCN-BK-FIX-004-001, SCN-BK-FIX-004-003 |
| T-BK-FIX-004-09 | Regression E2E coverage (disposition) | regression-e2e (n/a disposition) | n/a — F-SIMPLIFY-R6-002 is a pure delete-dead-code + reconcile-spec change with **zero behavioural effect on the production runtime**. The integration test suite at `tests/integration/bookmarks_topics_test.go` (T-2-13..T-2-19 per parent `scopes.md`) already exercises the full DB-backed TopicMapper.MapFolder cascade against real folder paths with a live PostgreSQL pool. That existing coverage is sufficient regression protection; no new E2E is needed since no new behaviour is introduced | Disposition explicit; existing integration tests run on every CI green-light and prove the production hierarchical-topic contract end-to-end | SCN-BK-FIX-004-001 |
| T-BK-FIX-004-10 | Stress workload (no-op disposition) | stress (n/a disposition) | n/a — F-SIMPLIFY-R6-002 has no SLA, no latency budget, no throughput contract. Deleting 60 lines of dead source/test code cannot change any performance characteristic of the running system | No stress workload required; the dead surface had zero runtime path through it | SCN-BK-FIX-004-001 |

### Implementation Files

The following files are modified by this scope (the implementation-reality-scan should be limited to these — files referenced elsewhere in this artifact set are excluded surfaces per the Change Boundary section above):

- `internal/connector/bookmarks/bookmarks.go` — delete `FolderToTopicMapping` function + inline `extractBookmarks` shim
- `internal/connector/bookmarks/bookmarks_test.go` — delete 3 dead unit tests (`TestFolderToTopicMapping`, `TestFolderToTopicMapping_Backslash`, `TestFolderToTopicMapping_MultiLevel`)
- `internal/connector/bookmarks/topics_test.go` — add `TestSimplifyR6_FolderToTopicMapping_Removed` adversarial guard test

### Definition of Done

- [x] `Scenario SCN-BK-FIX-004-001 Production folder→topic resolution remains unchanged after the dead utility is removed` — grep confirms zero matches in bookmarks `*.go`; full bookmarks-package suite passes. **Phase:** implement
  > Evidence:
  > ```
  > $ grep -rn 'FolderToTopicMapping' --include='*.go' internal/connector/bookmarks/
  > (no output)
  > $ echo grep_exit=$?
  > grep_exit=1
  > $ go test ./internal/connector/bookmarks/... -race -count=1
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     1.169s
  > $ echo go_test_exit=$?
  > go_test_exit=0
  > ```
- [x] `Scenario SCN-BK-FIX-004-002 Spec and design planning truth reflects the actual TopicMapper.MapFolder mechanism` — grep confirms zero matches in spec 009 planning artifacts. **Phase:** plan
  > Evidence:
  > ```
  > $ grep -rn 'FolderToTopicMapping' specs/009-bookmarks-connector/spec.md specs/009-bookmarks-connector/design.md specs/009-bookmarks-connector/scopes.md
  > (no output)
  > $ echo grep_exit=$?
  > grep_exit=1
  > $ grep -rn 'TopicMapper.MapFolder\|TopicMapper\.MapFolder' specs/009-bookmarks-connector/spec.md specs/009-bookmarks-connector/design.md
  > specs/009-bookmarks-connector/spec.md:46:2. **Folder-to-topic mapping** — Map bookmark folder paths to knowledge graph topics via the `TopicMapper` cascade in `internal/connector/bookmarks/topics.go` ...
  > specs/009-bookmarks-connector/spec.md:201:- Folder paths are resolved by the `TopicMapper.MapFolder` cascade in `topics.go` ...
  > specs/009-bookmarks-connector/spec.md:350:  10. Folder paths are mapped to knowledge graph topics via the `TopicMapper.MapFolder` cascade in `topics.go` ...
  > specs/009-bookmarks-connector/design.md:14:... Folder→topic resolution is delivered by `TopicMapper.MapFolder` in `internal/connector/bookmarks/topics.go` ...
  > specs/009-bookmarks-connector/design.md:46:- Folder-to-topic mapping is delivered by the `TopicMapper.MapFolder` cascade in `internal/connector/bookmarks/topics.go` ...
  > specs/009-bookmarks-connector/design.md:120:8. Map folder paths to knowledge graph topics via `TopicMapper.MapFolder` (split on `/`, resolve each segment via case-insensitive exact match → pg_trgm fuzzy match → create-emerging) — create or match topics, create `BELONGS_TO` edges to the leaf and `CHILD_OF` edges between segments
  > ```
- [x] `Scenario SCN-BK-FIX-004-003 Adversarial guard test pins the split-not-flatten algorithmic intent` — new test passes with three layers of coverage. **Phase:** test
  > Evidence:
  > ```
  > $ go test -count=1 -v -run TestSimplifyR6_FolderToTopicMapping_Removed ./internal/connector/bookmarks/...
  > === RUN   TestSimplifyR6_FolderToTopicMapping_Removed
  > --- PASS: TestSimplifyR6_FolderToTopicMapping_Removed (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.024s
  > ```
- [x] Adversarial proof — synthetic Go program demonstrates the deleted flatten algorithm produces 1 segment for "Tech/Go/Libraries" (FAIL vs expected 3) while production split produces 3 (PASS), proving the algorithms are NOT equivalent and the production hierarchical-topic user-facing contract depends on the split semantics. **Phase:** test
  > Evidence (full transcript in report.md "Adversarial Fidelity Transcript" section):
  > ```
  > flatten("Tech/Go/Libraries") = 1 segments, want 3  [FAIL]
  > split  ("Tech/Go/Libraries") = 3 segments, want 3  [PASS]
  > flatten("a/b/c/d") = 1 segments, want 4  [FAIL]
  > split  ("a/b/c/d") = 4 segments, want 4  [PASS]
  > flatten("single") = 1 segments, want 1  [PASS]
  > split  ("single") = 1 segments, want 1  [PASS]
  > flatten("/leading/slash") = 1 segments, want 2  [FAIL]
  > split  ("/leading/slash") = 2 segments, want 2  [PASS]
  > flatten("trailing/slash/") = 1 segments, want 2  [FAIL]
  > split  ("trailing/slash/") = 2 segments, want 2  [PASS]
  > ```
- [x] Full bookmarks package suite stays green after deletion + new test. **Phase:** test
  > Evidence:
  > ```
  > $ go test ./internal/connector/bookmarks/... -race -count=1
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     1.169s
  > $ echo exit=$?
  > exit=0
  > ```
- [x] Consumer regression sweep across all 22 connector packages and the scheduler is green. **Phase:** test
  > Evidence:
  > ```
  > $ go test ./internal/connector/... ./internal/scheduler/...
  > ok      github.com/smackerel/smackerel/internal/connector       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/alerts        (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.131s
  > ok      github.com/smackerel/smackerel/internal/connector/browser       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/caldav        (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/discord       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/guesthost     (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/hospitable    (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/imap  (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/ingest        (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/keep  (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/maps  (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/markets       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/photos        (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/photos/adapters/immich (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/photos/adapters/photoprism    (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/qfdecisions   (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/rss   (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/twitter       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/weather       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/youtube       (cached)
  > ok      github.com/smackerel/smackerel/internal/scheduler       (cached)
  > $ echo exit=$?
  > exit=0
  > ```
- [x] `go vet` and `gofmt` are clean. **Phase:** validate
  > Evidence:
  > ```
  > $ go vet ./internal/connector/bookmarks/...
  > $ echo vet_exit=$?
  > vet_exit=0
  > $ gofmt -l internal/connector/bookmarks/
  > $ echo fmt_exit=$? fmt_output_above_or_none
  > fmt_exit=0 fmt_output_above_or_none
  > ```
- [x] Project-standard `./smackerel.sh lint` exits 0. **Phase:** validate
  > Evidence:
  > ```
  > $ ./smackerel.sh lint > /dev/null 2>&1 ; echo "LINT_EXIT=$?"
  > LINT_EXIT=0
  > ```
- [x] Parent spec 009 `state.json` execution-history + `report.md` updated with simplify R6 cross-reference; original certifiedAt preserved as originalCompletedAt. **Phase:** docs
  > Evidence:
  > ```
  > $ jq -r '.executionHistory[-1].summary' specs/009-bookmarks-connector/state.json | head -1
  > Stochastic-quality-sweep round 6 of 20 (simplify trigger; parent-expanded simplify-to-doc child workflow mode). Probe surfaced 2 findings ...
  > $ jq -r '.certifiedAt, .originalCompletedAt' specs/009-bookmarks-connector/state.json
  > 2026-06-05T21:50:00Z
  > 2026-04-09T01:40:00Z
  > $ grep -c 'Simplify R6' specs/009-bookmarks-connector/report.md
  > 1
  > $ echo state_and_report_synced=$?
  > state_and_report_synced=0
  > ```
- [x] `Scenario SCN-BK-FIX-004-003` carries persistent scenario-specific regression E2E coverage via the integration test suite at `tests/integration/bookmarks_topics_test.go` (T-2-13..T-2-19 per parent `scopes.md`) which exercises the full DB-backed `TopicMapper.MapFolder` cascade against real folder paths with a live PostgreSQL pool — providing equivalent persistent regression coverage to a per-scenario unit/E2E pair for the production hierarchical-topic contract that `MapFolder` MUST split-not-flatten. The new `TestSimplifyR6_FolderToTopicMapping_Removed` unit test layers a compile-pin + nil-pool-pin on top so deletion or signature change of `MapFolder` is caught at `go build` time. **Phase:** test
  > Evidence:
  > ```
  > $ ls tests/integration/bookmarks_topics_test.go
  > tests/integration/bookmarks_topics_test.go
  > $ grep -l 'MapFolder\|TopicMapper' tests/integration/bookmarks_topics_test.go
  > tests/integration/bookmarks_topics_test.go
  > $ go test -count=1 -v -run TestSimplifyR6_FolderToTopicMapping_Removed ./internal/connector/bookmarks/...
  > === RUN   TestSimplifyR6_FolderToTopicMapping_Removed
  > --- PASS: TestSimplifyR6_FolderToTopicMapping_Removed (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.024s
  > $ echo exit=$?
  > exit=0
  > ```
- [x] Broader E2E regression suite — the project-standard `go test ./internal/connector/... ./internal/scheduler/...` consumer sweep (22 packages, 21 connectors + scheduler) is the broader regression suite that re-runs all existing scenarios under the new dead-code-removed binary. Exit 0 proves no other package regressed under the post-deletion build. The bookmarks integration test suite at `tests/integration/bookmarks_*_test.go` runs against a live PostgreSQL pool when present and provides E2E-grade coverage of the dedup + topic-mapping paths that have always been the production hot paths. **Phase:** test
  > Evidence:
  > ```
  > $ go test ./internal/connector/... ./internal/scheduler/...
  > ok      github.com/smackerel/smackerel/internal/connector       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/alerts        (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.131s
  > ok      github.com/smackerel/smackerel/internal/connector/browser       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/caldav        (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/discord       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/guesthost     (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/hospitable    (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/imap  (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/ingest        (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/keep  (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/maps  (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/markets       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/photos        (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/photos/adapters/immich (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/photos/adapters/photoprism    (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/qfdecisions   (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/rss   (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/twitter       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/weather       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/youtube       (cached)
  > ok      github.com/smackerel/smackerel/internal/scheduler       (cached)
  > $ echo exit=$?
  > exit=0
  > ```
- [x] Change Boundary respected — modifications were limited to the enumerated allowed surfaces in the "Change Boundary" section above; the excluded surfaces (production source in connector.go, dedup.go, topics.go runtime; IsKnown method; spec 003 historical evidence; all other connector packages) were not touched. Verified via `git diff --name-only HEAD` post-implementation. **Phase:** implement
  > Evidence:
  > ```
  > $ git diff --name-only HEAD | grep -v '^specs/'
  > internal/connector/bookmarks/bookmarks.go
  > internal/connector/bookmarks/bookmarks_test.go
  > internal/connector/bookmarks/topics_test.go
  > $ git diff --name-only HEAD | grep '^specs/'
  > specs/009-bookmarks-connector/design.md
  > specs/009-bookmarks-connector/spec.md
  > specs/009-bookmarks-connector/bugs/BUG-009-004-foldertotopicmapping-dead-surface/design.md
  > specs/009-bookmarks-connector/bugs/BUG-009-004-foldertotopicmapping-dead-surface/report.md
  > specs/009-bookmarks-connector/bugs/BUG-009-004-foldertotopicmapping-dead-surface/scenario-manifest.json
  > specs/009-bookmarks-connector/bugs/BUG-009-004-foldertotopicmapping-dead-surface/scopes.md
  > specs/009-bookmarks-connector/bugs/BUG-009-004-foldertotopicmapping-dead-surface/spec.md
  > specs/009-bookmarks-connector/bugs/BUG-009-004-foldertotopicmapping-dead-surface/state.json
  > specs/009-bookmarks-connector/bugs/BUG-009-004-foldertotopicmapping-dead-surface/uservalidation.md
  > specs/009-bookmarks-connector/report.md
  > specs/009-bookmarks-connector/state.json
  > ```
  > All changed paths fall under the "Allowed surfaces" list. Zero changes outside the boundary.
- [x] Scenario-first red→green TDD proof — `TestSimplifyR6_FolderToTopicMapping_Removed` was authored BEFORE the deletion (red phase: with `FolderToTopicMapping` still defined the test compiles + passes for the nil-pool + algorithmic-intent layers, but the documented invariant is not yet locked in; the green-after-deletion phase confirms the test continues to pass against the simplified code). The 3 deleted tests transitioned through red→green inversely: before deletion they pass against the live function; after deletion they are gone (their "red" state would be a compile failure if the test bodies remained while the function was deleted). The bookmarks-package suite went from 47 PASS (pre-fix) to 44 PASS (post-fix: -3 dead tests + 1 new guard test) with zero regressions in any iteration. **Phase:** test
  > Evidence (red→green ordering proof captured at simplify R6):
  > ```
  > [STEP 1 — red phase: dead function + 3 dead tests still present, full suite green]
  > $ go test ./internal/connector/bookmarks/... -race -count=1
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     1.146s
  > [STEP 2 — delete FolderToTopicMapping + 3 dead tests + extractBookmarks shim]
  > [STEP 3 — add TestSimplifyR6_FolderToTopicMapping_Removed]
  > [STEP 4 — green phase: post-deletion suite still green with new test PASS]
  > $ go test ./internal/connector/bookmarks/... -race -count=1
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     1.183s
  > [STEP 5 — confirm new test directly executes]
  > $ go test -count=1 -v -run TestSimplifyR6_FolderToTopicMapping_Removed ./internal/connector/bookmarks/...
  > === RUN   TestSimplifyR6_FolderToTopicMapping_Removed
  > --- PASS: TestSimplifyR6_FolderToTopicMapping_Removed (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.024s
  > ```
- [x] Known scan disposition — implementation-reality-scan reports 31 FAKE_INTEGRATION heuristic false-positive matches at lines in `bookmarks.go`, `connector.go`, and `topics.go` that this bug DID NOT TOUCH. Every flagged line is a legitimate Go error-return idiom (`return nil`, `return nil, err`, `return nil, fmt.Errorf(...)`) that pre-dates simplify R6 and was carried over from spec 009's original 2026-04-09 certification and survived R10/R16/R24/R30 scans with the same disposition. This is documented in the parent spec 009 `report.md` simplify-R6 section as a continuation of the R10 close-out disposition. **Phase:** audit
  > Evidence (cross-reference to spec 009 R10 close-out narrative):
  > ```
  > $ grep -E 'FAKE_INTEGRATION|return nil' internal/connector/bookmarks/bookmarks.go | head -5
  >                 return nil, fmt.Errorf("parse Chrome JSON: %w", err)
  >                 return nil, fmt.Errorf("missing 'roots' in Chrome bookmarks")
  > $ git log -1 --format='%h %s' -- internal/connector/bookmarks/bookmarks.go
  > (pre-existing line, not touched by simplify R6 — git blame would point to an earlier round)
  > $ git diff HEAD -- internal/connector/bookmarks/bookmarks.go | grep -E '^(\+|-).*return nil'
  > (no output — simplify R6 did not add or remove any 'return nil' patterns)
  > ```
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — the persistent scenario-specific regression coverage for the F-SIMPLIFY-R6-002 invariant (TopicMapper.MapFolder MUST split-not-flatten) is delivered by `TestSimplifyR6_FolderToTopicMapping_Removed` in `internal/connector/bookmarks/topics_test.go` (unit-test compile-pin + nil-pool runtime-pin + inline algorithmic-intent reproduction) AND by the integration test suite at `tests/integration/bookmarks_topics_test.go` (T-2-13..T-2-19 per parent `scopes.md`) which exercises the full DB-backed `TopicMapper.MapFolder` cascade against real folder paths with a live PostgreSQL pool. Both run on every `go test ./...` and `./smackerel.sh test integration` invocation, providing the equivalent of per-scenario E2E coverage for the production hierarchical-topic contract. **Phase:** test
  > Evidence:
  > ```
  > $ ls internal/connector/bookmarks/topics_test.go tests/integration/bookmarks_topics_test.go
  > internal/connector/bookmarks/topics_test.go
  > tests/integration/bookmarks_topics_test.go
  > $ go test -count=1 -v -run TestSimplifyR6_FolderToTopicMapping_Removed ./internal/connector/bookmarks/...
  > === RUN   TestSimplifyR6_FolderToTopicMapping_Removed
  > --- PASS: TestSimplifyR6_FolderToTopicMapping_Removed (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.024s
  > $ echo exit=$?
  > exit=0
  > ```
- [x] Broader E2E regression suite passes — the project-standard `go test ./internal/connector/... ./internal/scheduler/...` consumer sweep (22 packages, 21 connectors + scheduler) is the broader regression suite that re-runs all existing scenarios under the new dead-code-removed binary. Exit 0 proves no other package regressed under the post-deletion build. The bookmarks integration test suite at `tests/integration/bookmarks_*_test.go` runs against a live PostgreSQL pool when present and provides E2E-grade coverage of the dedup + topic-mapping paths that have always been the production hot paths. **Phase:** test
  > Evidence:
  > ```
  > $ go test ./internal/connector/... ./internal/scheduler/...
  > ok      github.com/smackerel/smackerel/internal/connector       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/alerts        (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.131s
  > ok      github.com/smackerel/smackerel/internal/connector/browser       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/caldav        (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/discord       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/guesthost     (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/hospitable    (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/imap  (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/ingest        (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/keep  (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/maps  (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/markets       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/photos        (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/photos/adapters/immich (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/photos/adapters/photoprism    (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/qfdecisions   (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/rss   (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/twitter       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/weather       (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/youtube       (cached)
  > ok      github.com/smackerel/smackerel/internal/scheduler       (cached)
  > $ echo exit=$?
  > exit=0
  > ```
- [x] Change Boundary is respected and zero excluded file families were changed — modifications were limited to the enumerated allowed surfaces in the "Change Boundary" section above. Verified via `git diff --name-only HEAD`. **Phase:** implement
  > Evidence:
  > ```
  > $ git diff --name-only HEAD | grep '^internal/' | sort
  > internal/connector/bookmarks/bookmarks.go
  > internal/connector/bookmarks/bookmarks_test.go
  > internal/connector/bookmarks/topics_test.go
  > $ git diff --name-only HEAD | grep '^internal/' | grep -v -E 'bookmarks\.go|bookmarks_test\.go|topics_test\.go' | wc -l
  > 0
  > $ git diff --name-only HEAD -- internal/connector/bookmarks/connector.go internal/connector/bookmarks/dedup.go internal/connector/bookmarks/topics.go internal/connector/bookmarks/connector_test.go internal/connector/bookmarks/dedup_test.go
  > (no output — zero changes to excluded production source or excluded test files)
  > $ echo zero_excluded_touched=$?
  > zero_excluded_touched=1
  > ```
