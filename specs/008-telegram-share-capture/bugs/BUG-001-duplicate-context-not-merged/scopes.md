# BUG-001 Scopes

## Scope 1 — Implement server-side context merge on duplicate capture

**Status:** Done

### Change Boundary

Allowed:
- `internal/pipeline/merge.go` (new)
- `internal/pipeline/merge_test.go` (new)
- `internal/pipeline/processor.go` (call merge in `Process` on duplicate path)
- `tests/integration/capture_duplicate_context_test.go` (new)

Forbidden:
- Any change to `internal/api/capture.go` (preserves 409 contract)
- Any change to `internal/telegram/*` (bot reply already correct after server fix)
- Schema migrations
- Other specs

### Definition of Done

- [x] `MergeUserContext` helper exists in `internal/pipeline/merge.go` with documented `Execer` interface and SQL using `jsonb_set` + `jsonb_build_array`.
  - Evidence: `internal/pipeline/merge.go`
- [x] `Processor.Process` invokes the merge when dedup fires AND `req.Context != ""`, before returning the `*DuplicateError`.
  - Evidence: `internal/pipeline/processor.go::Process`
- [x] Adversarial unit test fails on pre-fix code (helper does not exist) and passes on fixed code.
  - Evidence: `internal/pipeline/merge_test.go::TestMergeUserContext_AppendsContextToMetadata`
- [x] Empty-context and empty-id calls are no-ops (no SQL executed).
  - Evidence: `TestMergeUserContext_NoOpOnEmptyContext`, `TestMergeUserContext_NoOpOnEmptyArtifactID`
- [x] Underlying exec errors are propagated (wrapped) so ops can observe merge failures via slog.
  - Evidence: `TestMergeUserContext_PropagatesExecError`
- [x] Integration test (build tag `integration`) inserts a stub artifact, runs merge twice with two different contexts, and asserts `metadata->'user_contexts' = ["first context","second context"]`.
  - Evidence: `tests/integration/capture_duplicate_context_test.go::TestBUG001_MergeUserContext_AppendsTwoContexts`
- [x] `./smackerel.sh test unit` exits 0.
  - Evidence: see `report.md` § Test Evidence
- [x] No files outside the change boundary modified.
  - Evidence: see `report.md` § Files Modified
