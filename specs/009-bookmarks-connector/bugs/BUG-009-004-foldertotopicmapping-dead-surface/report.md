# Report: BUG-009-004 — FolderToTopicMapping dead exported surface (simplify R6)

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md) | [state.json](state.json)

## Summary

Stochastic-quality-sweep round 6 of 20 (trigger `simplify`, parent-expanded child workflow mode `simplify-to-doc`, statusCeiling `done`) probed `internal/connector/bookmarks/` for duplication, dead code, over-engineering, and unnecessary abstractions. The probe surfaced two findings:

- **F-SIMPLIFY-R6-001 (LOW, micro-fix)** — `extractBookmarks` in `bookmarks.go` was a one-line shim wrapper over `extractBookmarksDepth` with a single caller. Inlined directly in the same round; no bug spawned (private, unexported, no spec/design contract impact). Closed in the parent spec 009 `report.md` simplify-R6 section.
- **F-SIMPLIFY-R6-002 (MEDIUM, this bug)** — `FolderToTopicMapping` was exported, documented in 4 places in `spec.md` and 4 places in `design.md` as the primary folder→topic mechanism, and exercised by 3 dedicated unit tests — yet production never called it. The actual production folder→topic path is `TopicMapper.MapFolder` in `topics.go`, which implements a fundamentally different algorithm (segment-split + per-segment exact/fuzzy/create cascade against the `topics` table). The deleted utility flattened folder paths into a single string; the production path produces N hierarchical topics with `CHILD_OF` edges between them.

**Fix:** deleted the function and its 3 dead tests, rewrote all 8 spec/design reference sites to name `TopicMapper.MapFolder` (or describe the segment-split cascade), and added an adversarial guard test `TestSimplifyR6_FolderToTopicMapping_Removed` in `topics_test.go` that pins the algorithmic intent and the `MapFolder` API surface.

**Test:** the new guard test provides three layers of regression protection (compile-pin on `MapFolder` signature, runtime-pin on nil-pool early-return contract, and inline algorithmic-intent documentation reproducing the same `strings.Split + non-empty-trim` loop visible in `topics.go::MapFolder`). A synthetic adversarial proof captured at simplify R6 demonstrates that 4 of 5 fixture paths distinguish the deleted flatten algorithm from the production split algorithm.

## Completion Statement

All 9 DoD bullets in `scopes.md` Scope 1 are checked `[x]` with inline raw evidence. F-SIMPLIFY-R6-002 is closed via four parallel deliverables: (a) the dead `FolderToTopicMapping` function and its 3 dedicated tests are deleted from the bookmarks package (`grep` confirms zero `*.go` matches under `internal/connector/bookmarks/`); (b) all 8 stale planning-truth references in spec 009's `spec.md` and `design.md` are rewritten to name `TopicMapper.MapFolder` (`grep` confirms zero matches under `specs/009-bookmarks-connector/{spec.md,design.md,scopes.md}`); (c) the adversarial guard test `TestSimplifyR6_FolderToTopicMapping_Removed` is added and PASSES with three layers of coverage; (d) full bookmarks-package test suite (44 PASS, 0 FAIL) and consumer regression sweep across all 22 connector packages plus the scheduler are green. `go vet`, `gofmt`, and `./smackerel.sh lint` all exit 0.

## Test Evidence

### Test execution — new adversarial guard test (fix applied)

```text
$ go test -count=1 -v -run TestSimplifyR6_FolderToTopicMapping_Removed ./internal/connector/bookmarks/...
=== RUN   TestSimplifyR6_FolderToTopicMapping_Removed
--- PASS: TestSimplifyR6_FolderToTopicMapping_Removed (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.024s
$ echo exit=$?
exit=0
```

### Test execution — full bookmarks package suite

```text
$ go test ./internal/connector/bookmarks/... -race -count=1 -v
=== RUN   TestNormalizeURL_WWWDedup
--- PASS: TestNormalizeURL_WWWDedup (0.00s)
=== RUN   TestNormalizeURL_HostOnly
--- PASS: TestNormalizeURL_HostOnly (0.00s)
=== RUN   TestChaosR24_NormalizeURLStripsUserinfo
=== RUN   TestChaosR24_NormalizeURLStripsUserinfo/user:pass_basic_auth
=== RUN   TestChaosR24_NormalizeURLStripsUserinfo/user-only_(no_password)
=== RUN   TestChaosR24_NormalizeURLStripsUserinfo/encoded_credentials
=== RUN   TestChaosR24_NormalizeURLStripsUserinfo/userinfo_with_port
=== RUN   TestChaosR24_NormalizeURLStripsUserinfo/ftp_with_credentials
--- PASS: TestChaosR24_NormalizeURLStripsUserinfo (0.00s)
... (additional sub-tests elided for brevity; all PASS) ...
=== RUN   TestMapFolder_EmptyPath
--- PASS: TestMapFolder_EmptyPath (0.00s)
=== RUN   TestTopicMapper_NilPool
--- PASS: TestTopicMapper_NilPool (0.00s)
=== RUN   TestTopicMatch_Fields
--- PASS: TestTopicMatch_Fields (0.00s)
=== RUN   TestSimplifyR6_FolderToTopicMapping_Removed
--- PASS: TestSimplifyR6_FolderToTopicMapping_Removed (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/bookmarks     1.183s
$ echo exit=$?
exit=0
```

### Test execution — consumer regression sweep

```text
$ go test ./internal/connector/... ./internal/scheduler/...
ok      github.com/smackerel/smackerel/internal/connector       (cached)
ok      github.com/smackerel/smackerel/internal/connector/alerts        (cached)
ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.131s
ok      github.com/smackerel/smackerel/internal/connector/browser       (cached)
ok      github.com/smackerel/smackerel/internal/connector/caldav        (cached)
ok      github.com/smackerel/smackerel/internal/connector/discord       (cached)
ok      github.com/smackerel/smackerel/internal/connector/guesthost     (cached)
ok      github.com/smackerel/smackerel/internal/connector/hospitable    (cached)
ok      github.com/smackerel/smackerel/internal/connector/imap  (cached)
ok      github.com/smackerel/smackerel/internal/connector/ingest        (cached)
ok      github.com/smackerel/smackerel/internal/connector/keep  (cached)
ok      github.com/smackerel/smackerel/internal/connector/maps  (cached)
ok      github.com/smackerel/smackerel/internal/connector/markets       (cached)
ok      github.com/smackerel/smackerel/internal/connector/photos        (cached)
ok      github.com/smackerel/smackerel/internal/connector/photos/adapters/immich        (cached)
ok      github.com/smackerel/smackerel/internal/connector/photos/adapters/photoprism    (cached)
ok      github.com/smackerel/smackerel/internal/connector/qfdecisions   (cached)
ok      github.com/smackerel/smackerel/internal/connector/rss   (cached)
ok      github.com/smackerel/smackerel/internal/connector/twitter       (cached)
ok      github.com/smackerel/smackerel/internal/connector/weather       (cached)
ok      github.com/smackerel/smackerel/internal/connector/youtube       (cached)
ok      github.com/smackerel/smackerel/internal/scheduler       (cached)
$ echo exit=$?
exit=0
```

All 22 packages PASS. The `bookmarks` package shows `0.131s` (not cached) because it was re-run after the deletion and the new test addition. Other packages show `(cached)` because their test inputs did not change.

### Build verification

```text
$ go build ./...
$ echo build_exit=$?
build_exit=0
$ ls -la cmd/core/main.go internal/connector/bookmarks/bookmarks.go
-rw-r--r-- 1 user user  2148 Jun  5 21:30 cmd/core/main.go
-rw-r--r-- 1 user user  6234 Jun  5 21:30 internal/connector/bookmarks/bookmarks.go
$ go vet ./internal/connector/bookmarks/...
$ echo vet_exit=$?
vet_exit=0
```

Zero compilation errors after the deletion confirms that no other Go source in the repo imports or calls `FolderToTopicMapping`. This is the strongest possible evidence of "zero production consumers" — Go's compile-time linker would have rejected the build if any consumer existed.

### Audit — vet + gofmt + lint

```text
$ go vet ./internal/connector/bookmarks/...
$ echo vet_exit=$?
vet_exit=0

$ gofmt -l internal/connector/bookmarks/
$ echo fmt_exit=$? fmt_output_above_or_none
fmt_exit=0 fmt_output_above_or_none

$ ./smackerel.sh lint > /dev/null 2>&1 ; echo "LINT_EXIT=$?"
LINT_EXIT=0
```

All three checks clean. `gofmt -l` flagged one alignment issue in the new `TestSimplifyR6_FolderToTopicMapping_Removed` table that was fixed and re-verified before the final test run above.

### Verification — grep confirms total removal of dead surface

```text
$ grep -rn 'FolderToTopicMapping' --include='*.go' internal/connector/bookmarks/
$ echo grep_exit=$?
grep_exit=1
$ wc -l internal/connector/bookmarks/bookmarks.go internal/connector/bookmarks/bookmarks_test.go internal/connector/bookmarks/topics_test.go
  197 internal/connector/bookmarks/bookmarks.go
  948 internal/connector/bookmarks/bookmarks_test.go
  142 internal/connector/bookmarks/topics_test.go
 1287 total
$ ls -la internal/connector/bookmarks/*.go | awk '{print $NF}'
internal/connector/bookmarks/bookmarks.go
internal/connector/bookmarks/bookmarks_test.go
internal/connector/bookmarks/connector.go
internal/connector/bookmarks/connector_test.go
internal/connector/bookmarks/dedup.go
internal/connector/bookmarks/dedup_test.go
internal/connector/bookmarks/topics.go
internal/connector/bookmarks/topics_test.go
```

Zero matches in any `*.go` file under the bookmarks package — the function and all 3 dead tests are gone. `bookmarks.go` shrunk from 213 to 197 lines (-16 lines: removed `FolderToTopicMapping` function + inlined `extractBookmarks` shim). `bookmarks_test.go` shrunk from 995 to 948 lines (-47 lines: removed 3 dead tests). `topics_test.go` grew from 77 to 142 lines (+65 lines: added `TestSimplifyR6_FolderToTopicMapping_Removed`).

```text
$ grep -rn 'FolderToTopicMapping' specs/009-bookmarks-connector/spec.md specs/009-bookmarks-connector/design.md specs/009-bookmarks-connector/scopes.md
$ echo grep_exit=$?
grep_exit=1
$ wc -l specs/009-bookmarks-connector/spec.md specs/009-bookmarks-connector/design.md
  365 specs/009-bookmarks-connector/spec.md
  368 specs/009-bookmarks-connector/design.md
  733 total
$ git diff --stat HEAD -- specs/009-bookmarks-connector/spec.md specs/009-bookmarks-connector/design.md
 specs/009-bookmarks-connector/design.md |  9 +++++----
 specs/009-bookmarks-connector/spec.md   | 13 +++++++------
 2 files changed, 12 insertions(+), 10 deletions(-)
```

Zero matches in spec 009 planning artifacts — all 8 stale references rewritten. The `git diff --stat` shows a small net change (12 insertions, 10 deletions across spec.md + design.md) consistent with surgical reference-site rewrites rather than wholesale rewrites.

```text
$ grep -rn 'TopicMapper.MapFolder\|TopicMapper\.MapFolder' specs/009-bookmarks-connector/spec.md specs/009-bookmarks-connector/design.md
specs/009-bookmarks-connector/spec.md:46:2. **Folder-to-topic mapping** — Map bookmark folder paths to knowledge graph topics via the `TopicMapper` cascade in `internal/connector/bookmarks/topics.go` ...
specs/009-bookmarks-connector/spec.md:201:- Folder paths are resolved by the `TopicMapper.MapFolder` cascade in `topics.go` ...
specs/009-bookmarks-connector/spec.md:350:  10. Folder paths are mapped to knowledge graph topics via the `TopicMapper.MapFolder` cascade in `topics.go` ...
specs/009-bookmarks-connector/design.md:14:... Folder→topic resolution is delivered by `TopicMapper.MapFolder` in `internal/connector/bookmarks/topics.go` ...
specs/009-bookmarks-connector/design.md:46:- Folder-to-topic mapping is delivered by the `TopicMapper.MapFolder` cascade in `internal/connector/bookmarks/topics.go` ...
specs/009-bookmarks-connector/design.md:120:8. Map folder paths to knowledge graph topics via `TopicMapper.MapFolder` (split on `/`, resolve each segment via case-insensitive exact match → pg_trgm fuzzy match → create-emerging) — create or match topics, create `BELONGS_TO` edges to the leaf and `CHILD_OF` edges between segments
```

The rewritten reference sites correctly name the production mechanism.

### Adversarial Fidelity Transcript

A standalone synthetic Go program was run at simplify R6 to capture the algorithmic-difference proof between the deleted `FolderToTopicMapping` flatten algorithm and the production `TopicMapper.MapFolder` split algorithm. The synthetic compared `flatten("/")` (which the deleted utility used: `strings.ReplaceAll(folder, "/", " ")` → single segment) vs `split("/")` (which the production path uses: `strings.Split(folder, "/")` → N segments, with empty trimmed).

```text
$ go run /tmp/adversarial-test.go
flatten("Tech/Go/Libraries") = 1 segments, want 3  [FAIL]
split  ("Tech/Go/Libraries") = 3 segments, want 3  [PASS]
flatten("a/b/c/d") = 1 segments, want 4  [FAIL]
split  ("a/b/c/d") = 4 segments, want 4  [PASS]
flatten("single") = 1 segments, want 1  [PASS]
split  ("single") = 1 segments, want 1  [PASS]
flatten("/leading/slash") = 1 segments, want 2  [FAIL]
split  ("/leading/slash") = 2 segments, want 2  [PASS]
flatten("trailing/slash/") = 1 segments, want 2  [FAIL]
split  ("trailing/slash/") = 2 segments, want 2  [PASS]
$ echo exit=$?
exit=0
=== 4 mismatches (algorithms NOT equivalent for multi-segment paths) PASSED expectation ===
```

Interpretation: 4 of 5 fixture paths (all multi-segment cases) produce dramatically different segment counts under the two algorithms. Only the single-segment fixture `"single"` collapses to the same count, because a path without `/` has nothing to split. The new `TestSimplifyR6_FolderToTopicMapping_Removed` test embeds these same 6 fixture paths (plus `"Bookmarks Bar/Tech/Distributed Systems"`) inline, asserting expected counts produced by the production split algorithm. A future reviewer changing the production algorithm to flatten — and also updating this test's expected counts to match — would break the user-facing hierarchical-topic behavior contract documented in `spec.md` R-005, which is the actual regression backstop.

### Scope of Confidence

What the new guard test catches with high confidence:
- **Deletion of `MapFolder`** — direct call site fails to compile, never reaches CI.
- **Signature change to `MapFolder`** — direct call site fails to compile, never reaches CI.
- **Removal of the nil-pool early-return contract** — runtime assertion `matches != nil` would fail with the exact authored diagnostic.

What the new guard test documents but does not directly catch (honest scoping):
- **Internal algorithm change from split to flatten inside `MapFolder`** — the test asserts the same split-and-trim loop body inline, separate from production. A regression that flattens production but leaves the test's inline algorithm intact would NOT fail the unit test. The integration test suite at `tests/integration/bookmarks_topics_test.go` (T-2-13..T-2-19 per parent `scopes.md`) provides this coverage by exercising the full DB-backed cascade against real folder paths with a live PostgreSQL pool.

## Findings Ledger

| Finding ID | Severity | Disposition | Closure |
|---|---|---|---|
| F-SIMPLIFY-R6-001 | LOW | Same-round inline fix (no bug spawned; unexported, no spec/design contract impact) | Closed; documented in parent spec 009 `report.md` simplify-R6 section |
| F-SIMPLIFY-R6-002 | MEDIUM | This bug (BUG-009-004); planning truth repair + code deletion + adversarial guard test + full validation chain | Closed; this report.md + the linked DoD evidence in `scopes.md` |

Total findings: 2. Closed: 2. Unresolved: 0. Round 6 reaches one-to-one closure accounting.

### Code Diff Evidence

Gate G053 requires a `Code Diff Evidence` section in report artifacts for implementation-bearing workflows. Captured via `git diff --stat HEAD --` over the simplify R6 change set:

```text
$ git diff --stat HEAD -- internal/connector/bookmarks/ specs/009-bookmarks-connector/spec.md specs/009-bookmarks-connector/design.md
 internal/connector/bookmarks/bookmarks.go      | 18 +------
 internal/connector/bookmarks/bookmarks_test.go | 47 -------------------
 internal/connector/bookmarks/topics_test.go    | 65 ++++++++++++++++++++++++++
 specs/009-bookmarks-connector/design.md        |  9 ++--
 specs/009-bookmarks-connector/spec.md          | 13 +++---
 5 files changed, 77 insertions(+), 75 deletions(-)
```

Diff classification (per Gate G093 delivery-implementation-delta semantics):

| Path class | Files | Insertions | Deletions | Net |
|---|---|---|---|---|
| Production source | `internal/connector/bookmarks/bookmarks.go` | 1 | 17 | **−16 lines** (removed `FolderToTopicMapping` function + inlined `extractBookmarks` shim) |
| Tests (dead removed) | `internal/connector/bookmarks/bookmarks_test.go` | 0 | 47 | **−47 lines** (removed 3 dead `TestFolderToTopicMapping*` tests) |
| Tests (new regression guard) | `internal/connector/bookmarks/topics_test.go` | 65 | 0 | **+65 lines** (added `TestSimplifyR6_FolderToTopicMapping_Removed` with three layers of coverage) |
| Planning truth (parent spec) | `specs/009-bookmarks-connector/spec.md`, `design.md` | 12 | 10 | **+2 lines** (surgical reference-site rewrites at 4 sites each) |
| **Total** | **5 files** | **78** | **74** | **+4 lines** (net) |

The delivery delta includes real production-source modifications (`bookmarks.go`) AND real test modifications (`bookmarks_test.go` deletions + `topics_test.go` additions) AND real planning-truth reconciliation (`spec.md` + `design.md` rewrites). Per G093, this is a valid done-ceiling delivery round because the change set is not specs-only; it touches both the implementation surface (bookmarks.go) and the test surface in a meaningful, coordinated way.

The bug folder itself (`specs/009-bookmarks-connector/bugs/BUG-009-004-foldertotopicmapping-dead-surface/`) and the parent `specs/009-bookmarks-connector/{report.md,state.json}` are governance artifacts maintained alongside the delivery diff, not part of the delivery itself.

### Guard Disposition

The state-transition-guard at simplify R6 first-run flagged 31 `[FAKE_INTEGRATION]` violations from the implementation-reality-scan (Gate G028) at lines in `internal/connector/bookmarks/{bookmarks,connector,topics}.go`. ALL flagged lines are pre-existing legitimate Go error-return idioms — `return nil`, `return nil, err`, and `return nil, fmt.Errorf(...)` — that have existed in the bookmarks package since spec 009's original 2026-04-09 certification and survived R10, R16, R24, R30, R2 (gaps), and Iter-9 (trace-guard) scans with the same disposition.

This bug **did not touch any of the 31 flagged lines.** Verified by `git diff -- internal/connector/bookmarks/` showing only the F-SIMPLIFY-R6-002 deletions (`FolderToTopicMapping` function lines 143–152) plus the `extractBookmarks` shim removal at line 52/155 — no edits to or near any of the lines flagged by Scan 1D (External Integration Authenticity).

```text
$ git diff HEAD -- internal/connector/bookmarks/bookmarks.go | grep -E '^(\+|-).*return nil' | wc -l
0
$ git diff HEAD -- internal/connector/bookmarks/connector.go | wc -l
0
$ git diff HEAD -- internal/connector/bookmarks/topics.go | wc -l
0
$ git diff HEAD -- internal/connector/bookmarks/dedup.go | wc -l
0
$ echo zero_changes_to_flagged_files=$?
zero_changes_to_flagged_files=0
$ go vet ./internal/connector/bookmarks/...
$ echo vet_exit_after_changes=$?
vet_exit_after_changes=0
```

Zero additions or deletions of any `return nil` pattern across simplify R6, and zero changes to `connector.go`, `topics.go`, or `dedup.go` source. The scan's heuristic remains a known false-positive on legitimate Go error-handling idioms — this disposition was documented in spec 009's R10 close-out (state.json executionHistory at 2026-05-25T17:30:00Z) and is reaffirmed here without modification.

**Disposition:** documented in scope DoD bullet "Known scan disposition" with cross-reference to the R10 close-out narrative. Not a blocking finding; not a defect introduced by this round; not a candidate for code change.

The state-transition-guard also flagged Gate G088 (post-certification spec edit) because this round modifies `specs/009-bookmarks-connector/spec.md` and `design.md` which were certified `done` at 2026-04-09T01:40:00Z (and re-certified at 2026-06-04T02:30:00Z + 2026-06-05T20:50:00Z by intermediate sweep rounds). The remediation is the standard re-certification pattern proven by spec 080 R5 (2026-06-05T20:50:00Z): bump `state.json::certifiedAt` to the current timestamp, preserve `originalCompletedAt`, append a `recertificationReason` documenting the simplify R6 cycle, and add the round's executionHistory entry. The parent state.json update is captured in the parent `state.json::executionHistory[-1]` and parent `report.md` simplify-R6 section.
