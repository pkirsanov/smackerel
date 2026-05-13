# Bug: BUG-028-002 — CompareAggregator silent JSON swallow

## Classification

- **Type:** Code defect — silent error swallowing (observability gap)
- **Severity:** MEDIUM (no incorrect output produced; correct sources still aggregate; but malformed product/comparison `domain_data` is silently dropped, hiding upstream extractor regressions from operators)
- **Parent Spec:** 028 — Actionable Lists & Resource Tracking
- **Workflow Mode:** harden-to-doc (parent: stochastic-quality-sweep round 8 of 20)
- **Status:** Fixed
- **Discovered By:** stochastic-quality-sweep (seed 20520512), trigger=`harden`, mapped child mode=`harden-to-doc`

## Problem Statement

`CompareAggregator.Aggregate` in [internal/list/reading_aggregator.go](../../../../internal/list/reading_aggregator.go) silently swallowed `json.Unmarshal` errors for product/comparison `domain_data`:

```go
for i, src := range sources {
    var cd compareData
    if err := json.Unmarshal(src.DomainData, &cd); err != nil {
        continue   // <-- silent: no slog.Warn, no error visibility
    }
    ...
}
```

The prior harden round (recorded inline in [internal/list/harden_test.go](../../../../internal/list/harden_test.go) and the comments in [internal/list/recipe_aggregator.go](../../../../internal/list/recipe_aggregator.go) and [internal/list/reading_aggregator.go](../../../../internal/list/reading_aggregator.go)) explicitly remediated the same anti-pattern in:

- `RecipeAggregator.Aggregate` — replaced silent `continue` with `slog.Warn(...)` + continue.
- `ReadingAggregator.Aggregate` — replaced `_ = json.Unmarshal(...)` with a logged warning fallback.

`CompareAggregator.Aggregate` was missed by that round. The defect violates the same governance rationale (Gate G028 / `requireNoDefaultsNoFallbacks` parity, plus the project's general silent-error-swallowing prohibition) and creates a cross-aggregator observability inconsistency: malformed recipe/reading sources are visible to operators; malformed product/comparison sources are not.

## Detection

| Aspect | Detail |
|---|---|
| Trigger | stochastic-quality-sweep `harden` probe on `internal/list/...` |
| Sweep round | 8 of 20 (selection seed `20520512`) |
| Mapped child mode | `harden-to-doc` (parent-expanded; nested `runSubagent` unavailable) |
| File | [internal/list/reading_aggregator.go](../../../../internal/list/reading_aggregator.go) (CompareAggregator definition lives in the same file as ReadingAggregator) |
| Location | `CompareAggregator.Aggregate` — first statement inside `for i, src := range sources` loop |
| Pre-existing test coverage | `TestCompareAggregator_InvalidJSON` (`internal/list/reading_aggregator_test.go`) — only checked behavior (`len(seeds) == 0`), not visibility, so the defect could not be caught by the test suite. |

## Behavior Contract

**Pre-fix (defect):**
- Silent `continue` on `json.Unmarshal` failure.
- Operators have no telemetry signal that an upstream extractor has produced malformed `domain_data` for `product` artifacts.
- Repeated extractor regressions could silently suppress an entire comparison list with no audit trail.

**Post-fix (required behavior):**
- `slog.Warn(...)` is emitted before `continue`, with `artifact_id` and the unmarshal `error` as structured fields.
- The bad source is still skipped (skip-the-bad-source semantics preserved — parity with `RecipeAggregator`).
- Good sources still produce seeds normally.
- The fix is parity-only; no behavior change for any existing caller, no API change, no schema change.

## Acceptance Criteria

| ID | Criterion |
|---|---|
| BUG-028-002-AC-1 | Malformed `domain_data` for a `product` artifact produces a `WARN`-level log via `log/slog` with key `compare aggregator: skipping artifact with malformed domain_data`. |
| BUG-028-002-AC-2 | The structured log fields include `artifact_id` (the offending source ID) and `error` (the wrapped `json.Unmarshal` error). |
| BUG-028-002-AC-3 | Behavior is preserved: a malformed source contributes zero seeds; a well-formed source contributes one seed; mixed inputs yield only the good source's seed. |
| BUG-028-002-AC-4 | A non-tautological adversarial regression test exists in [internal/list/harden_test.go](../../../../internal/list/harden_test.go) that fails if the `slog.Warn(...)` is removed (i.e., if the bare `continue` is reintroduced). |
| BUG-028-002-AC-5 | The full `internal/list/...` Go test suite remains green; no behavioral regression. |
| BUG-028-002-AC-6 | No API surface change, no schema change, no migration. The fix is confined to `internal/list/reading_aggregator.go` (CompareAggregator block) and `internal/list/harden_test.go` (new regression test). |
