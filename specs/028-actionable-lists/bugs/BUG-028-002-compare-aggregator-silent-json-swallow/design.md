# Design: BUG-028-002 — CompareAggregator silent JSON swallow

## Current Truth

**Source under fix:** [internal/list/reading_aggregator.go](../../../../internal/list/reading_aggregator.go) — `CompareAggregator.Aggregate` method.

**Pre-fix code (verbatim, the defect):**

```go
for i, src := range sources {
    var cd compareData
    if err := json.Unmarshal(src.DomainData, &cd); err != nil {
        continue
    }
    ...
}
```

**Sibling aggregators (parity reference, already-remediated by prior harden round):**

`RecipeAggregator.Aggregate` in [internal/list/recipe_aggregator.go](../../../../internal/list/recipe_aggregator.go):

```go
if err := json.Unmarshal(src.DomainData, &rd); err != nil {
    // Surface malformed domain_data instead of silently dropping the source.
    // A persistent log lets operators detect upstream extraction regressions.
    slog.Warn("recipe aggregator: skipping artifact with malformed domain_data",
        "artifact_id", src.ArtifactID, "error", err)
    continue
}
```

`ReadingAggregator.Aggregate` in [internal/list/reading_aggregator.go](../../../../internal/list/reading_aggregator.go):

```go
if err := json.Unmarshal(src.DomainData, &rd); err != nil {
    slog.Warn("reading aggregator: malformed domain_data, falling back to placeholder title",
        "artifact_id", src.ArtifactID, "error", err)
}
```

**Imports already in place:** `log/slog` is already imported in `reading_aggregator.go` (used by `ReadingAggregator`), so no import change is required for the fix.

## Design Decisions

### D1: Parity, not novelty

The fix MUST mirror the `RecipeAggregator` shape exactly: `slog.Warn(<aggregator-prefixed message>, "artifact_id", src.ArtifactID, "error", err)` followed by `continue`. This keeps cross-aggregator log shape consistent so a single operator dashboard / log filter can surface upstream extractor regressions across all three aggregators.

**Rejected alternatives:**

- **Returning the error from `Aggregate`.** Would break the partial-aggregation contract that the existing `Aggregate` interface depends on. Recipe and Reading aggregators both keep going on a single bad source; CompareAggregator must preserve the same skip-on-error semantics for parity.
- **Counting the failures and emitting a single summary warning at the end.** Loses per-artifact identification; operator can't trace back to the specific upstream extractor regression. The other aggregators log per-occurrence; CompareAggregator must do the same.
- **Promoting the error to a Prometheus counter.** Not part of this bug. If telemetry is later expanded, all three aggregators should be updated together as a dedicated planning round; this bug is intentionally narrow and parity-only.

### D2: Adversarial regression test design

The test MUST be non-tautological — it must FAIL if the `slog.Warn(...)` is removed. Achieved by:

1. Capturing `slog` output to an in-process `bytes.Buffer` via `slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})` and `slog.SetDefault(...)`.
2. Restoring the previous `slog.Default()` via `t.Cleanup(...)` so other tests are unaffected.
3. Asserting:
   - The captured buffer contains the literal log message prefix.
   - The captured buffer identifies each malformed `artifact_id` individually.
   - The good-source artifact is NOT logged as malformed (cross-check against false positives).
   - Behavior preservation: only the good source produces a seed; bad sources don't leak into `SourceArtifactIDs`.

**Adversarial proof recorded:** [report.md](report.md) > "Validation Evidence" — the test was confirmed to fail when the bare `continue` was reintroduced (red), then pass when the fix was restored (green). The red-then-green sequence is the canonical TDD-shape adversarial proof.

### D3: Change boundary

| Surface | Touched? |
|---|---|
| `internal/list/reading_aggregator.go` (CompareAggregator block only) | YES — 2 lines of comment, 2 lines of `slog.Warn` |
| `internal/list/harden_test.go` (new test + 2 imports) | YES — 1 new test function (`TestCompareAggregator_LogsAndSkipsBadJSON`), `bytes` and `log/slog` added to test imports |
| `internal/list/recipe_aggregator.go`, `internal/list/reading_aggregator.go` other aggregators | NO |
| `internal/list/store.go`, `internal/list/generator.go`, `internal/list/types.go` | NO |
| `internal/api/lists.go`, `internal/telegram/...` | NO |
| Schema migrations | NO |
| Public API surface | NO |
| `specs/028-actionable-lists/spec.md`, `design.md`, `scopes.md` | NO (parent spec is `done`; bug delivered as standalone artifact-coupled fix) |

### D4: No Shared Infrastructure Impact Sweep required

The change is local to one method body in `CompareAggregator` and adds one test case to the existing `harden_test.go`. It does not touch fixtures, harnesses, bootstrap, auth, sessions, storage, transport, or any cross-spec shared infrastructure. No canary, no rollback plan, no broader change boundary required.

## Implementation Sketch

1. Replace the bare `continue` block in `CompareAggregator.Aggregate` with the `slog.Warn(...) + continue` shape used by `RecipeAggregator`.
2. Add `TestCompareAggregator_LogsAndSkipsBadJSON` to `internal/list/harden_test.go`.
3. Add `bytes` and `log/slog` to the test file's imports.
4. Run `go test -count=1 ./internal/list/...`.
5. Run the adversarial proof: temporarily revert step 1, confirm test fails, then restore.
6. Run `go vet ./internal/list/...`.
