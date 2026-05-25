# Bug: BUG-009-003 — Topic mapping ignores ctx cancellation (stabilize R10)

## Classification

- **Type:** Runtime defect (cancellation hygiene / shutdown stability)
- **Severity:** MEDIUM — does not corrupt data, but on supervisor shutdown or mid-sync cancel the connector burns one DB roundtrip per remaining artifact against a doomed context, every one of which fails with `context.Canceled` and emits a `slog.Warn`. Net effect: log spam, wasted DB load, delayed graceful shutdown, no observable mid-loop cancellation signal.
- **Parent Spec:** 009 — Bookmarks Connector
- **Workflow Mode:** parent-expanded child workflow mode `stabilize-to-doc` (driven by stochastic-quality-sweep `sweep-2026-05-25-r10`, round 10, trigger `stabilize`)
- **Status:** Fixed

## Problem Statement

Stochastic-quality-sweep round 10 of `sweep-2026-05-25-r10` ran a stabilize probe (parent-expanded child workflow mode `stabilize-to-doc`) over `internal/connector/bookmarks/connector.go`'s `Sync()` path. One concrete defect surfaced. The pre-existing test `TestSyncRespectsContextCancel` only exercised cancellation *before* `Sync` entered the file scan; the post-dedup topic-mapping loop had no equivalent guard or test.

### F-STAB-R10-001 — Topic mapping loop ignores `ctx` cancellation (MEDIUM)

After dedup completes inside `Sync()`, the connector iterates `allArtifacts` and for each one calls:

1. `c.topicMapper.MapFolder(ctx, folder)`
2. `c.topicMapper.CreateTopicEdge(ctx, artifact, topicID)`
3. `c.topicMapper.UpdateTopicMomentum(ctx, topicID)`

All three are blocking DB roundtrips. The loop body had **no `ctx.Err()` check at the top of the iteration**. When the supervisor cancels (graceful shutdown, deadline exceeded, parent goroutine death), the loop keeps spinning until it walks the entire artifact slice. For each surviving iteration:

- The three DB calls all return `context.Canceled` immediately at the driver level (pgx honours ctx).
- Each call's error is converted to a `slog.Warn(...)` — three warnings per artifact.
- The DB pool still incurs the round-trip setup cost, the cancellation propagation cost, and the error allocation cost for each call.
- The connector silently proceeds as if topic mapping had merely failed for individual artifacts, masking the fact that the *entire sync was cancelled*.

For a single bookmark batch of 1 000 artifacts cancelled at artifact 10, the loop emits ~2 990 stale `slog.Warn` entries and issues ~2 970 doomed DB calls before returning. There is no single, clear "cancellation interrupted topic mapping at artifact N of M" log line for the operator.

Reproduction (in-package probe, captured by R10 test `TestStabR10_TopicMappingRespectsContextCancel`):

```text
ctx := context.Background()
ctx, cancel := context.WithCancel(ctx)
cancel()                                    // cancel BEFORE the loop starts
artifacts := makeBookmarkArtifacts(10)      // 10 RawArtifacts with folder_path metadata
processed := c.mapFolderTopics(ctx, artifacts)
// BEFORE FIX: processed == 10 (loop ignored cancel, all 10 burned DB roundtrips)
// AFTER  FIX: processed == 0  (loop exits before iteration 0 emits the single cancel warn)
```

## Acceptance Criteria

- [x] The post-dedup topic-mapping loop in `Sync()` checks `ctx.Err()` at the top of every iteration and returns immediately (without issuing the three `topicMapper` DB calls) if the context is cancelled. (F-STAB-R10-001)
- [x] Mid-loop cancellation emits **one** structured log entry — `slog.Warn("bookmarks topic mapping cancelled mid-loop", "error", ..., "processed", N, "remaining", M)` — instead of N stale per-artifact warnings. (F-STAB-R10-001)
- [x] Adversarial regression tests are added that FAIL with concrete diagnostics if the `ctx.Err()` guard is removed from the loop top (proven by toggling the guard in `connector.go` and re-running the new R10 tests).
- [x] All pre-existing bookmarks-package tests continue to pass (`TestSyncRespectsContextCancel`, `TestSyncCreatesArtifacts`, `TestSyncDedupsCorrectly`, `TestHealthDisconnectedBeforeConnect`, dedup/normalize tests, etc.).
- [x] `go test ./internal/connector/... ./internal/scheduler/...` is green (consumer regression sweep).
- [x] `go vet ./internal/connector/bookmarks/...` and `gofmt -l` are clean.
- [x] Parent `specs/009-bookmarks-connector/state.json` and `report.md` reference this bug under stabilize R10 history.

## Boundary

- No DB schema change.
- No connector-config-shape change.
- No change to the public `connector.Connector` (`Connect/Sync/Health/Close`) contract — `Sync()`'s signature and return-shape are unchanged.
- The refactor extracts the loop into a private `mapFolderTopics(ctx, artifacts) int` method on `*BookmarksConnector`. The method is package-private and only callable from `Sync()`.
- Only the bookmarks connector is modified; consumer packages (`internal/scheduler`, `internal/connector` registry, `cmd/core`) are not touched.
- Working-tree files modified by unrelated parallel work (deploy/CI, framework-stats) are NOT staged in this round's commits.
