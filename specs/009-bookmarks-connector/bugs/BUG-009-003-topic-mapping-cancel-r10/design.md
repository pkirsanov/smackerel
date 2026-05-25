# Design: BUG-009-003 — Topic mapping ignores ctx cancellation

Links: [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [state.json](state.json)

## Current Truth

Solution-blind facts gathered from the codebase before the fix design:

1. `internal/connector/bookmarks/connector.go` defines `BookmarksConnector.Sync(ctx context.Context, cursor *connector.SyncCursor) (*connector.SyncResult, error)`.
2. `Sync()` already checks `ctx.Err()` once before entering the scan loop (covered by pre-existing test `TestSyncRespectsContextCancel`), but the post-dedup topic-mapping loop has no equivalent guard.
3. The topic-mapping loop body issues three sequential blocking DB calls per artifact via `c.topicMapper.MapFolder`, `CreateTopicEdge`, and `UpdateTopicMomentum`. Each accepts `ctx context.Context`.
4. `c.topicMapper` is initialised only by `NewConnectorWithPool` (the pool-bearing constructor). `NewConnector` leaves it `nil`. All three `topicMapper` methods are nil-safe by virtue of an `if c.topicMapper == nil { return ... }` early-return that already exists. They are also pool-nil-safe — the production helpers early-return on a nil pool.
5. Bookmarks connectors are driven sequentially by `internal/connector/supervisor.go`'s `runConnector` goroutine. No two `Sync()` calls overlap for a single connector instance, so this is not a Sync-vs-Sync race — it is a **cancellation-during-loop** stability defect.
6. The supervisor cancels `connCtx` when the parent context is cancelled. Without a mid-loop guard, the bookmarks connector keeps running long after the rest of the system has decided to shut down.
7. The pre-existing `TestSyncRespectsContextCancel` only proves the **pre-loop** guard works. It does NOT prove the topic-mapping loop respects cancellation. Coverage gap.

## Root Cause

The post-dedup topic-mapping loop in `Sync()` was implemented as a tight per-artifact iteration that delegated cancellation handling to the per-call `topicMapper` methods. Those methods do honour `ctx` at the driver level — but they emit per-artifact `slog.Warn` for every failure, and they have no awareness of the loop's intent (`"I am the last guard before issuing the next round of doomed work"`). Result: the loop generates O(N) stale warnings and O(3·N) doomed DB calls per cancelled sync.

The fix belongs at the loop's top, not inside the per-call methods, because:

- The loop is the only place that knows the size of the remaining work.
- The loop is the only place that can emit a single, structured "we stopped because the caller cancelled" log entry.
- Leaving the guard inside `topicMapper` methods would still allow the loop to walk every artifact, just with cheaper failures — the surface defect (no observable mid-loop cancel signal) would persist.

## Fix Approach

1. **Extract** the post-dedup topic-mapping loop from `Sync()` into a private method:

   ```go
   func (c *BookmarksConnector) mapFolderTopics(ctx context.Context, artifacts []connector.RawArtifact) int
   ```

   The method returns the count of artifacts whose topic mapping was *attempted* (i.e., the iteration was entered, even if individual `topicMapper` calls logged a warning and continued). The return value is used by tests; `Sync()` does not need it for its existing observability.

2. **Add a `ctx.Err()` check at the top of every iteration**:

   ```go
   for _, a := range artifacts {
       if err := ctx.Err(); err != nil {
           slog.Warn("bookmarks topic mapping cancelled mid-loop",
               "error", err,
               "processed", processed,
               "remaining", len(artifacts)-processed,
           )
           return processed
       }
       // ... existing per-artifact body unchanged ...
       processed++
   }
   ```

   Order matters: the guard MUST run **before** the iteration body, otherwise the first cancelled iteration would still issue all three doomed DB calls. The adversarial test `TestStabR10_TopicMappingCancelBeforeFirstIteration` proves this ordering invariant by passing a pre-cancelled context and asserting `processed == 0`.

3. **Single log line, not three per artifact**: replace the per-call `slog.Warn` cascade with a single structured warning naming the loop, the wrapped error, the count of artifacts that completed before cancel was observed, and the count remaining. This gives the operator one clear log line per cancelled sync, instead of O(N) stale warnings.

4. **`Sync()` invocation** becomes a single call: `c.mapFolderTopics(ctx, allArtifacts)`. The lockmgr block that follows (`c.mu.Lock(); c.lastSyncCount = len(allArtifacts); c.lastSyncTime = time.Now(); c.mu.Unlock()`) is unchanged.

5. **Behaviour preservation for healthy contexts**: when `ctx` is not cancelled, the new method is byte-identical in observable effect to the inline loop — same `topicMapper` calls in the same order, same per-call error handling, same `slog.Warn` cascade on individual mapper failures. The full bookmarks-package test suite (which never cancels mid-loop) passes unchanged.

## Rationale: why extract, not inline?

The extracted method is the only place where:

- The cancel guard is testable in isolation (the test can synthesise `[]RawArtifact` directly without setting up file scanning + parsing + dedup).
- The "processed vs remaining" accounting becomes trivially correct (the loop owns both counters).
- A future R-NN round can add a per-iteration timeout or rate-limiter without re-touching `Sync()`.

The cost is one helper method with package-private scope. The benefit is targeted testability of the cancellation contract and a single source of truth for the mid-loop log line.

## Adversarial Fidelity Protocol

The fix is proven faithful by the following toggle procedure (already executed in this round, transcript in `report.md`):

1. With fix applied: `go test -count=1 -v -run TestStabR10 ./internal/connector/bookmarks/...` — all 3 R10 tests PASS.
2. Replace the `if err := ctx.Err(); err != nil { slog.Warn(...); return processed }` guard with `_ = ctx` (toggle OFF).
3. Re-run the same test selector: `TestStabR10_TopicMappingRespectsContextCancel` FAILS with `mapFolderTopics processed=10 on pre-cancelled ctx, want 0`; `TestStabR10_TopicMappingCancelBeforeFirstIteration` FAILS with `ctx guard must run BEFORE the iteration body, not after: processed=1 on pre-cancelled ctx with 1 artifact, want 0`; `TestStabR10_TopicMappingNilMapper` still PASSES (orthogonal to cancellation).
4. Restore the guard (toggle ON). Re-run: all 3 R10 tests PASS again.

The two failing diagnostics name the exact invariant the test enforces, so a future developer who removes the guard sees the failure tied directly to F-STAB-R10-001.

## Out of Scope

- The pre-`Sync` `ctx.Err()` check (already covered by `TestSyncRespectsContextCancel`) is not modified.
- The file-scan loop inside `findNewFiles` (already covered by R30/R24/R16 chaos rounds) is not modified.
- `topicMapper.MapFolder/CreateTopicEdge/UpdateTopicMomentum` per-call behaviour is not modified — they continue to honour `ctx` at the driver level.
- No changes to dedup, normalization, or artifact emission paths.
- No changes to `cmd/core/main.go` or to supervisor/scheduler wiring.
