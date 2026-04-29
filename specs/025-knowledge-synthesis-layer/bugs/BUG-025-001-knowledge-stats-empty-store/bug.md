# Bug: BUG-025-001 Knowledge stats empty-store 500

## Summary
`/api/knowledge/stats` returns HTTP 500 on an empty knowledge store, blocking knowledge synthesis E2E and any live-stack flow that checks stats before concepts exist.

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - Knowledge synthesis E2E and empty-store operator visibility blocked
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status
- [x] Reported
- [ ] Confirmed (targeted red-stage output to be captured by owner)
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps
1. Start the disposable E2E stack with an empty knowledge store.
2. Call authenticated `GET /api/knowledge/stats` before any concept page has been synthesized.
3. Observe HTTP 500 instead of a zero-valued stats payload.
4. Inspect `internal/knowledge/store.go::GetStats`, where `PromptContractVersion` is scanned from a scalar subquery over `knowledge_concepts`.

## Expected Behavior
Knowledge stats should return HTTP 200 with zero counts and an empty prompt contract version when no concepts, entities, lint reports, or synthesis records exist.

## Actual Behavior
The stats endpoint returns HTTP 500 on empty store, so the E2E knowledge synthesis test cannot use stats as a reliable readiness/visibility signal.

## Environment
- Service: Go core knowledge API and PostgreSQL-backed knowledge store
- Version: Workspace state on 2026-04-27 during 039 full-delivery e2e stabilization
- Platform: Linux, Docker-backed disposable E2E stack

## Error Output
```text
Workflow context from bubbles.stabilize: Knowledge stats returns 500 on empty store.
Relevant source path: internal/knowledge/store.go.
Likely failing endpoint path: /api/knowledge/stats.
```

## Root Cause (initial analysis)
`GetStats` selects prompt contract version with `(SELECT COALESCE(prompt_contract_version, '') FROM knowledge_concepts ORDER BY updated_at DESC LIMIT 1)`. When the table has no rows, the scalar subquery yields NULL, and scanning that NULL into a Go string can fail. The outer query should produce an empty string for the empty-store case.

## Related
- Feature: `specs/025-knowledge-synthesis-layer/`
- Source: `internal/knowledge/store.go`
- E2E: `tests/e2e/knowledge_synthesis_test.go`
