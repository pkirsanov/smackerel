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
- [x] Confirmed (targeted red-stage output captured: empty-store stats returned HTTP 500 before the fix)
- [x] In Progress
- [x] Fixed (current HEAD contains the `GetStats` outer `COALESCE` fix; later `c6d2b26` broad E2E baseline was GREEN)
- [x] Verified
- [x] Closed

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

## Root Cause
`GetStats` selects prompt contract version with `(SELECT COALESCE(prompt_contract_version, '') FROM knowledge_concepts ORDER BY updated_at DESC LIMIT 1)`. When the table has no rows, the scalar subquery yields NULL, and scanning that NULL into a Go string can fail. The outer query should produce an empty string for the empty-store case.

## Resolution
`internal/knowledge/store.go::GetStats` now applies `COALESCE` around the scalar subquery result, so no-row `knowledge_concepts` produces an explicit empty `prompt_contract_version` string. The lint report branch only treats `pgx.ErrNoRows` as an empty lint-report state, preserving real database errors.

## Verification
- Targeted red proof reproduced HTTP 500 from `/api/knowledge/stats` before the fix.
- Focused post-fix E2E `TestKnowledgeStore_TablesExist` passed with zero/default stats on a fresh disposable stack.
- Live PostgreSQL integration regression `TestKnowledgeStats_EmptyStoreReturnsZeroValues` asserts zero counts, nil last synthesis time, and empty prompt contract version with no `knowledge_concepts` rows.
- The later `c6d2b26` broad baseline recorded `./smackerel.sh test e2e` exit 0, shell E2E 34/34 passed, and Go E2E packages passed, so the broad suite no longer reports the empty-store stats 500.

## Related
- Feature: `specs/025-knowledge-synthesis-layer/`
- Source: `internal/knowledge/store.go`
- E2E: `tests/e2e/knowledge_synthesis_test.go`
