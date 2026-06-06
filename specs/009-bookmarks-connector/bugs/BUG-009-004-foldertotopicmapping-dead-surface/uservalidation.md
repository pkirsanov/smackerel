# User Validation: BUG-009-004 — FolderToTopicMapping dead exported surface (simplify R6)

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

---

## Acceptance Checklist

- [x] F-SIMPLIFY-R6-002 closed — the dead exported `FolderToTopicMapping` function is deleted from `internal/connector/bookmarks/bookmarks.go`; the three dedicated dead unit tests (`TestFolderToTopicMapping`, `TestFolderToTopicMapping_Backslash`, `TestFolderToTopicMapping_MultiLevel`) are deleted from `internal/connector/bookmarks/bookmarks_test.go`. Grep confirms zero `*.go` matches under `internal/connector/bookmarks/`.
- [x] Planning-truth drift resolved — all 4 `spec.md` reference sites and all 4 `design.md` reference sites are rewritten to name `TopicMapper.MapFolder` (or describe the segment-split + per-segment exact/fuzzy/create resolution cascade). Grep confirms zero `FolderToTopicMapping` matches under `specs/009-bookmarks-connector/{spec.md,design.md,scopes.md}`. Historical spec 003 evidence rows (under `specs/003-phase2-ingestion/`) are intentionally preserved as a historical record of phase-2 sign-off.
- [x] Behaviour preservation — the deletion is a pure dead-code removal. Production `TopicMapper.MapFolder` is unchanged and remains the single folder→topic entry point. The full bookmarks-package test suite passes unchanged (44 PASS after deletion + 1 new test added).
- [x] Consumer regression sweep clean — `go test ./internal/connector/... ./internal/scheduler/...` runs all 22 packages with exit 0.
- [x] Adversarial guard test recorded — `TestSimplifyR6_FolderToTopicMapping_Removed` in `topics_test.go` provides three layers of regression protection: compile-pin on `MapFolder` signature via direct call, runtime-pin on nil-pool early-return contract, and inline algorithmic-intent documentation reproducing the same `strings.Split + non-empty-trim` loop visible in `topics.go::MapFolder`. The test's failure diagnostic names `F-SIMPLIFY-R6-002 invariant` as the grep target so a future reviewer comparing test claims against production drift has immediate context.
- [x] Synthetic adversarial proof transcript captured in `report.md` "Adversarial Fidelity Transcript" — demonstrates that 4 of 5 R6 fixture paths distinguish between the deleted flatten algorithm (1 segment per path) and the production split algorithm (N segments per path).
- [x] No production-contract change — `Connect/Sync/Health/Close` signatures, config shape, NATS contract, DB schema, and migrations are unchanged; only `bookmarks.go`, `bookmarks_test.go`, `topics_test.go`, parent `spec.md`, parent `design.md` are modified.
- [x] `IsKnown` exported method on `URLDeduplicator` is intentionally NOT in scope (live integration test consumers in `tests/integration/bookmarks_dedup_test.go`; documented in `scopes.md`); simplify R6 verified this distinction and recorded it in `spec.md` Boundary.
- [x] `./smackerel.sh lint` exit 0; `go vet ./internal/connector/bookmarks/...` exit 0; `gofmt -l internal/connector/bookmarks/` empty output.
- [x] Parent spec 009 artifacts updated with simplify R6 cross-reference (state.json execution history + report.md section); `originalCompletedAt` preserved from prior 2026-06-04 / 2026-06-05 re-certifications.

## Sign-off

This bug closure is the parent-expanded child workflow execution of `simplify-to-doc` mode for spec 009 within the stochastic-quality-sweep round 6 of 20 (trigger `simplify`). The simplify probe ran inside the same workflow that planned, implemented, tested, validated, and audited the fix in one round. The user acceptance is implicit in the workflow contract: round 6 reaches `completed_owned` only after F-SIMPLIFY-R6-002 closes with a passing adversarial guard test and the spec/design planning truth is reconciled against the production code, both of which the evidence in `report.md` and `scopes.md` demonstrates.

This round also delivers the secondary micro-fix F-SIMPLIFY-R6-001 (inlining the `extractBookmarks` shim wrapper in `bookmarks.go`) directly without spawning a separate bug — that finding is documented in the parent spec 009 `report.md` simplify-R6 section as a same-round closure.
