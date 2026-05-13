# Scopes: BUG-028-002 — CompareAggregator silent JSON swallow

## Scope 1 — Add visibility for malformed product/comparison `domain_data`

**Status:** Done

**Depends on:** none

### Definition of Done

- [x] **SCN-BUG-028-002-001** Insert a `slog.Warn("compare aggregator: skipping artifact with malformed domain_data", "artifact_id", src.ArtifactID, "error", err)` before the existing `continue` in `CompareAggregator.Aggregate` (`internal/list/reading_aggregator.go`). **Evidence:** [report.md](report.md) → "Code Diff Evidence" — verbatim diff of the addition with surrounding context.
- [x] **SCN-BUG-028-002-002** Skip-the-bad-source semantics preserved (a malformed source contributes zero seeds; good sources still produce one seed each). **Evidence:** [report.md](report.md) → "Validation Evidence" — `TestCompareAggregator_LogsAndSkipsBadJSON` PASS output showing `len(seeds) == 1` from a 2-bad + 1-good input.
- [x] **SCN-BUG-028-002-003** Adversarial regression test `TestCompareAggregator_LogsAndSkipsBadJSON` added to `internal/list/harden_test.go`. The test captures structured log output, asserts the warning is emitted with `artifact_id` and `error` fields, asserts each malformed source is individually identified, asserts the good source is NOT logged as malformed, and asserts seed-count behavior is preserved. **Evidence:** [report.md](report.md) → "Test Evidence" — full text of the added test function.
- [x] **SCN-BUG-028-002-004** Adversarial proof recorded: the new test FAILS when the bare `continue` is reintroduced and PASSES when the new warning call is restored. **Evidence:** [report.md](report.md) → "Adversarial proof (red-then-green)" — both terminal outputs.
- [x] **SCN-BUG-028-002-005** Full `internal/list/...` Go test suite remains green; no behavioral regression in any sibling test. **Evidence:** [report.md](report.md) → "Validation Evidence" — `go test -count=1 ./internal/list/...` PASS output.
- [x] **SCN-BUG-028-002-006** Change boundary verified — only `internal/list/reading_aggregator.go` (CompareAggregator block) and `internal/list/harden_test.go` are modified; no other source files, no schema delta, no behavior-surface delta. **Evidence:** [report.md](report.md) → "Audit Evidence" — `git diff --name-only` style listing.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — covered at the unit-test layer because the touched code path is an internal aggregator with no HTTP/UI/E2E surface. The unit-level adversarial regression test `TestCompareAggregator_LogsAndSkipsBadJSON` is the single authoritative scenario-specific regression — it exercises the exact behavior that was broken and would FAIL on reintroduction (red-then-green proven). **Evidence:** [report.md](report.md) → "Adversarial proof (red-then-green)" plus "Test Evidence".
- [x] Broader E2E regression suite passes — the broader Go regression surface is `go test -count=1 ./internal/list/...` which exercises every CompareAggregator/RecipeAggregator/ReadingAggregator/Generator/Store path. PASS confirmed in [report.md](report.md) → "Validation Evidence". The application-wide live-stack E2E suite is not applicable to a unit-level aggregator observability change with no API/UI surface.
- [x] **Stress / chaos coverage**: race-detector run (`go test -count=1 -race ./internal/list/...`) PASS in 1.058s plus multi-malformed-input chaos at the unit level (two divergent corruption shapes asserted independently). **Evidence:** [report.md](report.md) → "Chaos Evidence".

### Test Plan

| Test type | Coverage | Status |
|---|---|---|
| Unit — Regression E2E equivalent for the malformed-product-domain_data path | `TestCompareAggregator_LogsAndSkipsBadJSON` in `internal/list/harden_test.go` (Regression: red-then-green proven non-tautological) | PASS |
| Unit — sibling parity check | `TestRecipeAggregator_LogsAndSkipsBadJSON`, `TestReadingAggregator_FallsBackOnBadJSON` (existing) | PASS |
| Unit — full CompareAggregator suite | `TestCompareAggregator_BasicComparison`, `..._MissingFields`, `..._InvalidJSON`, `..._MultiProductAlignment` (existing) | PASS |
| Stress — race detector | `go test -count=1 -race ./internal/list/...` | PASS |
| Static analysis | `go vet ./internal/list/...` | clean |
