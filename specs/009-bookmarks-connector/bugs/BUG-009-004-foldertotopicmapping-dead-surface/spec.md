# Bug: BUG-009-004 — FolderToTopicMapping dead exported API + spec/design drift (simplify R6)

## Classification

- **Type:** Code-health defect (dead exported API surface) + planning-truth drift (spec/design references the dead function as the production mechanism)
- **Severity:** MEDIUM — no runtime defect; the function is correct in isolation, but it is exported, tested, documented as the primary folder→topic mechanism in `spec.md` and `design.md`, and never called by production code. The actual production path (`internal/connector/bookmarks/topics.go::TopicMapper.MapFolder`) implements a fundamentally different algorithm (segment-split + per-segment exact / pg_trgm-fuzzy / create-emerging cascade). Net effect: misleading public API, stale planning truth, and 3 unit tests that prove a dead surface is correct.
- **Parent Spec:** 009 — Bookmarks Connector
- **Workflow Mode:** parent-expanded child workflow mode `simplify-to-doc` (driven by stochastic-quality-sweep round 6 of 20, trigger `simplify`)
- **Status:** Fixed

## Problem Statement

Stochastic-quality-sweep round 6 of 20 ran a simplify probe (parent-expanded child workflow mode `simplify-to-doc`) over the `internal/connector/bookmarks/` package. The probe surfaced two findings; this bug carries closure for the larger one. The smaller finding (`F-SIMPLIFY-R6-001`, `extractBookmarks` shim micro-fix) was inlined directly without spawning a bug.

### F-SIMPLIFY-R6-002 — `FolderToTopicMapping` is exported dead code with spec/design drift (MEDIUM)

`internal/connector/bookmarks/bookmarks.go` defines an exported function:

```go
// FolderToTopicMapping converts bookmark folder names to topic names.
func FolderToTopicMapping(folder string) string {
    if folder == "" {
        return ""
    }
    // Normalize: lowercase, trim, replace separators
    topic := strings.ToLower(strings.TrimSpace(folder))
    topic = strings.ReplaceAll(topic, "/", " ")
    topic = strings.ReplaceAll(topic, "\\", " ")
    return topic
}
```

This function:

1. Is referenced as the production folder→topic mechanism by `specs/009-bookmarks-connector/spec.md` in **four places** (Goal #2, the ASCII architecture diagram under "Existing Parsers", R-005 ("Use the existing `bookmarks.FolderToTopicMapping()` for folder name normalization"), and UC-001 step 10 ("Folder paths are mapped to knowledge graph topics via `FolderToTopicMapping()`")).
2. Is referenced as the production mechanism by `specs/009-bookmarks-connector/design.md` in **four places** (Design Brief "Current State", Component Design ("Folder-to-topic mapping reuses existing `FolderToTopicMapping()` with hierarchical topic creation"), Data Flow step 8 ("Map folder paths to knowledge graph topics via `FolderToTopicMapping()`"), and Component Design §2 ("`FolderToTopicMapping(folder string) string` — normalizes folder names to topic names (lowercase, trim, replace separators)")).
3. Is exercised by **three dedicated unit tests** in `internal/connector/bookmarks/bookmarks_test.go`:
   - `TestFolderToTopicMapping` (line 71) — 4 basic inputs
   - `TestFolderToTopicMapping_Backslash` (line 255) — backslash separator
   - `TestFolderToTopicMapping_MultiLevel` (line 583) — multi-level slash paths (T-PARSE-005)
4. Has **zero production callers**. `grep -rn 'bookmarks\\.FolderToTopicMapping' --include='*.go'` returns no hits outside the test file. The actual production folder→topic flow is `TopicMapper.MapFolder(ctx, folderPath)` in `topics.go`, which:
   - Splits `folderPath` on `/` into segments (`strings.Split(folderPath, "/")`) — the OPPOSITE of `FolderToTopicMapping`, which REPLACES `/` with space and returns a single normalised string.
   - Resolves each segment via a 3-stage cascade: case-insensitive exact match in `topics.name`, pg_trgm `similarity > 0.4` fuzzy match, or insert a new `state='emerging'` topic.
   - Creates `CHILD_OF` edges between parent and child segments for hierarchical topic relationships.
   - Returns `[]TopicMatch` per segment, which `BookmarksConnector.mapFolderTopics` then uses to create `BELONGS_TO` edges via `CreateTopicEdge` and to call `UpdateTopicMomentum` for momentum scoring.

The two algorithms are not equivalent. `FolderToTopicMapping("Tech/Go")` returns the string `"tech go"` (a single flattened topic name); `TopicMapper.MapFolder(ctx, "Tech/Go")` returns two `TopicMatch` records (`Tech` and `Go`) with a `CHILD_OF` edge between them. The production hierarchical-topic behaviour mandated by `spec.md` R-005 ("Nested folders create hierarchical topic relationships") is delivered exclusively by `TopicMapper.MapFolder`, never by `FolderToTopicMapping`.

`FolderToTopicMapping` is dead code that was preserved during the 009 implementation because the design originally called for it. The implementation pivoted to the `TopicMapper` cascade (which was added as a new file, `topics.go`), but the dead utility was never deleted and the spec/design.md never caught up.

Reproduction (in-package probe, captured at simplify R6):

```text
$ grep -rn 'bookmarks\.FolderToTopicMapping\|FolderToTopicMapping(' \
    --include='*.go' --exclude='*_test.go' ~/smackerel
internal/connector/bookmarks/bookmarks.go:144:func FolderToTopicMapping(folder string) string {
# zero production callers; only the definition itself
```

```text
$ grep -rn 'TopicMapper.MapFolder\|c.topicMapper.MapFolder\|tm.MapFolder' \
    --include='*.go' ~/smackerel
internal/connector/bookmarks/connector.go:303:		matches, err := c.topicMapper.MapFolder(ctx, folder)
internal/connector/bookmarks/topics.go:35:func (tm *TopicMapper) MapFolder(ctx context.Context, folderPath string) ([]TopicMatch, error) {
internal/connector/bookmarks/topics_test.go:11:		matches, err := tm.MapFolder(context.Background(), path)
internal/connector/bookmarks/topics_test.go:31:	matches, err := tm.MapFolder(context.Background(), "Tech/Go")
# production calls TopicMapper.MapFolder exclusively
```

## Acceptance Criteria

- [x] `FolderToTopicMapping` is deleted from `internal/connector/bookmarks/bookmarks.go`. (F-SIMPLIFY-R6-002)
- [x] The 3 dead tests (`TestFolderToTopicMapping`, `TestFolderToTopicMapping_Backslash`, `TestFolderToTopicMapping_MultiLevel`) are deleted from `internal/connector/bookmarks/bookmarks_test.go`. (F-SIMPLIFY-R6-002)
- [x] `specs/009-bookmarks-connector/spec.md` is updated in all 4 reference sites to describe the production folder→topic mechanism (`TopicMapper.MapFolder`-style segment-split + per-segment cascade) instead of naming the deleted utility. The user-visible behaviour described by Goal #2, R-005, and UC-001 step 10 is unchanged — only the named implementation mechanism is corrected. (F-SIMPLIFY-R6-002)
- [x] `specs/009-bookmarks-connector/design.md` is updated in all 4 reference sites to describe the actual `TopicMapper.MapFolder` cascade instead of the dead utility. (F-SIMPLIFY-R6-002)
- [x] An adversarial guard test (`TestSimplifyR6_FolderToTopicMapping_Removed`) is added to `internal/connector/bookmarks/topics_test.go` that (a) compile-pins the `TopicMapper.MapFolder` signature via a direct call, (b) runtime-pins the nil-pool early-return contract, and (c) documents the split-not-flatten algorithmic intent inline using the same `strings.Split + non-empty-trim` loop body that lives in `topics.go::MapFolder` — so a reviewer comparing the test's named invariant against a production change that re-introduces the deleted flatten algorithm has a single grep target (`F-SIMPLIFY-R6-002 invariant`). Full coverage scoping (what the test catches vs what integration tests cover) is documented in `design.md` → "Adversarial Fidelity Protocol". (F-SIMPLIFY-R6-002)
- [x] All pre-existing bookmarks-package tests continue to pass (`go test ./internal/connector/bookmarks/... -count=1` exit 0).
- [x] `./smackerel.sh build` exit 0 (the deletion does not break any consumer, since there were no consumers).
- [x] `go vet ./internal/connector/bookmarks/...` and `gofmt -l` are clean.
- [x] Parent `specs/009-bookmarks-connector/state.json` and `report.md` reference this bug under simplify R6 history.
- [x] Spec 003 historical evidence (`specs/003-phase2-ingestion/report.md` and `scopes.md`) is left untouched — those are closed historical artifacts documenting the function's existence at the time of phase-2 sign-off; they are not production-truth statements about spec 009.

## Boundary

- No DB schema change.
- No connector-config-shape change.
- No change to the public `connector.Connector` (`Connect/Sync/Health/Close`) contract.
- No change to `TopicMapper.MapFolder` behaviour (the simplification is the removal of a dead parallel surface, not a rewrite of the production path).
- No change to `internal/connector/bookmarks/connector.go`, `dedup.go`, or `topics.go` source files apart from documentation comments referencing the deleted utility (there are no such references, confirmed by grep).
- The 4 `FolderToTopicMapping` mentions in `specs/003-phase2-ingestion/` are historical evidence rows for a closed phase and are NOT modified by this bug.
- The `IsKnown` exported method on `URLDeduplicator` is **NOT** in scope for this bug — it has live consumers in `tests/integration/bookmarks_dedup_test.go` and is a documented part of the deduplicator contract in `scopes.md`. Simplify R6 verified this and intentionally did not flag it.
