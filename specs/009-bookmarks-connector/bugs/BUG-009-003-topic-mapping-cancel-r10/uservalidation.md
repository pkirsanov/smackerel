# User Validation: BUG-009-003 — Topic mapping ignores ctx cancellation (stabilize R10)

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

---

## Acceptance Checklist

- [x] F-STAB-R10-001 closed — the post-dedup topic-mapping loop in `internal/connector/bookmarks/connector.go::Sync()` checks `ctx.Err()` at the top of every iteration. On supervisor shutdown the loop now exits immediately with a single structured `slog.Warn("bookmarks topic mapping cancelled mid-loop", "error", ..., "processed", N, "remaining", M)` instead of burning three doomed DB calls + three stale per-call warnings per remaining artifact.
- [x] Behaviour preservation — healthy contexts see byte-identical behaviour to the prior inline loop. The full bookmarks-package test suite passes unchanged (`go test ./internal/connector/bookmarks/... -count=1` exit 0).
- [x] Consumer regression sweep clean — `go test ./internal/connector/... ./internal/scheduler/...` runs all 21 connector packages plus the scheduler with exit 0.
- [x] Adversarial proof recorded — toggling the `ctx.Err()` guard OFF (replacing it with `_ = ctx`) causes `TestStabR10_TopicMappingRespectsContextCancel` and `TestStabR10_TopicMappingCancelBeforeFirstIteration` to FAIL with the exact authored diagnostics (`mapFolderTopics processed=10 on pre-cancelled ctx, want 0` and `ctx guard must run BEFORE the iteration body, not after: processed=1 on pre-cancelled ctx with 1 artifact, want 0`); restoring the guard returns all 3 R10 tests to PASS. Full transcript in [report.md](report.md) → "Adversarial Fidelity Transcript".
- [x] No production-contract change — `Connect/Sync/Health/Close` signatures and config shape are unchanged; only `internal/connector/bookmarks/connector.go` was modified plus the new tests in `internal/connector/bookmarks/connector_test.go`.
- [x] No DB-schema change.
- [x] `go vet ./internal/connector/bookmarks/...` and `gofmt -l internal/connector/bookmarks/connector.go internal/connector/bookmarks/connector_test.go` both exit 0 with empty output.
- [x] Parent spec 009 artifacts updated with stabilize R10 cross-reference (state.json execution history + report.md section).

## Sign-off

This bug closure is the parent-expanded child workflow execution of `stabilize-to-doc` mode for spec 009 within sweep `sweep-2026-05-25-r10` round 10. The stabilize probe ran inside the same workflow that planned, implemented, tested, validated, and audited the fix in one round. The user acceptance is implicit in the workflow contract: round 10 reaches `completed_owned` only after F-STAB-R10-001 closes with a passing regression test that would fail if the fix were reverted, which the adversarial proof in `report.md` demonstrates.
