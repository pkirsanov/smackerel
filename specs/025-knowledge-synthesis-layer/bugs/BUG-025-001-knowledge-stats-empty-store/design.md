# Bug Fix Design: BUG-025-001

## Root Cause Analysis

### Investigation Summary
The 2026-04-27 workflow context reports HTTP 500 from knowledge stats on an empty store. Source inspection found `internal/knowledge/store.go::GetStats` scanning a scalar subquery into `stats.PromptContractVersion`. The subquery applies `COALESCE` inside the row selection from `knowledge_concepts`, but an empty table produces no row for that inner selection.

### Root Cause
The empty-store prompt contract version path can yield NULL to the outer scan. Scanning NULL into a Go string returns an error, which the API surfaces as stats failure.

### Impact Analysis
- Affected components: knowledge stats store query, knowledge stats API, knowledge synthesis E2E.
- Affected data: empty or newly initialized stores.
- Affected users: operators and workflows checking knowledge health before synthesis has produced concepts.

## Fix Design

### Solution Approach
Move empty-result handling to the outer expression so the scalar subquery's no-row case produces an explicit empty value. Preserve error propagation for genuine database errors. Add tests that run against an empty knowledge store and assert both HTTP/API success and zero-valued stats.

### Alternative Approaches Considered
1. Seed a concept row before stats checks. Rejected because empty store is a valid runtime state.
2. Swallow all stats query errors in the handler. Rejected because it would hide real storage failures.

## Affected Files
- `internal/knowledge/store.go`
- `internal/knowledge/store_test.go` or live integration tests for empty-store stats
- `tests/e2e/knowledge_synthesis_test.go` if E2E stats assertions need stronger diagnostics

## Regression Test Design
- Unit/integration regression: `GetStats` against empty tables returns zero counts and empty prompt contract version.
- E2E regression: authenticated `GET /api/knowledge/stats` returns HTTP 200 on fresh disposable stack.
- Adversarial case: no rows in `knowledge_concepts`, no lint report, and no synthesized artifacts.

## Ownership
- Owning feature/spec: `specs/025-knowledge-synthesis-layer`
- Fix owner: `bubbles.implement`
- Test owner: `bubbles.test`
- Validation owner: `bubbles.validate`
