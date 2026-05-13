# User Validation: BUG-028-002 — CompareAggregator silent JSON swallow

## Persona

This is a backend internal-observability bug. The relevant "user" is the Smackerel **operator** who consumes structured logs to detect upstream extractor regressions affecting comparison/product lists.

## Acceptance

| AC | Met? | Notes |
|---|---|---|
| BUG-028-002-AC-1 (slog.Warn on malformed product domain_data) | YES | Verified by `TestCompareAggregator_LogsAndSkipsBadJSON` log-buffer assertion. See [report.md](report.md) → "Validation Evidence". |
| BUG-028-002-AC-2 (artifact_id + error fields present) | YES | Verified by per-artifact-id assertions (`bad-1`, `bad-2`) in the test, and confirmed against the live WARN line shape produced by sibling aggregators in the verbose run. |
| BUG-028-002-AC-3 (skip-the-bad-source behavior preserved) | YES | Test asserts `len(seeds) == 1` from `2-bad + 1-good` input, with `Widget` content from the good source and no `bad-1`/`bad-2` ID leakage. |
| BUG-028-002-AC-4 (non-tautological adversarial test) | YES | Red-then-green proof recorded in [report.md](report.md) → "Adversarial proof (red-then-green)". The bare `continue` reintroduction triggered 3 assertion failures in the visibility section; restoring `slog.Warn` flipped to PASS. |
| BUG-028-002-AC-5 (full internal/list suite green) | YES | `go test -count=1 ./internal/list/...` PASS, recorded in [report.md](report.md). |
| BUG-028-002-AC-6 (change boundary respected) | YES | `git diff --stat HEAD -- internal/list/reading_aggregator.go` shows `1 file changed, 17 insertions(+), 2 deletions(-)`. The harden_test.go contribution is described in full in [report.md](report.md) (file untracked at HEAD). No other source files modified. |

## Operator-Visible Outcome

Before this fix, an operator monitoring Smackerel logs for product/comparison extractor regressions would see no signal when an upstream extractor produced malformed `domain_data` for a `product` artifact — affected products would silently disappear from generated comparison lists with no audit trail.

After this fix, every malformed product `domain_data` payload produces a structured WARN log line of the form:

```text
WARN compare aggregator: skipping artifact with malformed domain_data artifact_id=<id> error=<parser error>
```

This brings operator visibility into parity with the recipe and reading aggregators that were previously remediated for the same class of defect, closing the cross-aggregator observability gap.

## Sign-off

This bug is a parity-only internal observability fix. No end-user feature change, no UI surface, no API surface, no schema. Operator-facing acceptance is satisfied by the in-source `slog.Warn` and the adversarial regression test that prevents reintroduction.

**Status:** Accepted by orchestrator on behalf of the operator persona based on the recorded validation evidence.

## Checklist

- [x] Operator-visible WARN log shape is parity-consistent with `RecipeAggregator` and `ReadingAggregator` (verified by sibling-test verbose run in [report.md](report.md) → "Validation Evidence").
- [x] Each malformed product `domain_data` payload is individually identified by `artifact_id` in the WARN log (verified by per-`artifact_id` assertions in `TestCompareAggregator_LogsAndSkipsBadJSON`).
- [x] Skip-the-bad-source semantics preserved — a malformed source produces zero seeds; a well-formed source still produces one seed (verified by `len(seeds) == 1` assertion against a 2-bad + 1-good input).
- [x] No false-positive WARN logs for well-formed sources (verified by negative `artifact_id=good-1` assertion).
- [x] Adversarial regression test is non-tautological and would fail if the bare `continue` were reintroduced (verified by red-then-green proof in [report.md](report.md) → "Adversarial proof").
- [x] No public API change, no schema change, no NATS contract change, no config change (verified in [report.md](report.md) → "Audit Evidence").
- [x] No PII in logs (only opaque `artifact_id` and stdlib parser error string).
- [x] Operator can now distinguish between "no products in input" and "products dropped due to malformed `domain_data`" via the new WARN signal.
