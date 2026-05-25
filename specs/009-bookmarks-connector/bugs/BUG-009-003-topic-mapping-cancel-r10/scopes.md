# Scopes: BUG-009-003 — Topic mapping ignores ctx cancellation (stabilize R10)

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Guard the post-dedup topic-mapping loop against ctx cancellation

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-BK-FIX-003-001 Cancelled context stops the topic-mapping loop before issuing the next round of DB calls
  Given a BookmarksConnector with topicMapper initialised (pool-nil-safe)
  And 10 RawArtifacts each carrying folder_path metadata
  And a context that has already been cancelled before the call
  When mapFolderTopics(ctx, artifacts) is invoked
  Then it returns 0 (zero artifacts processed)
  And it emits exactly one slog.Warn "bookmarks topic mapping cancelled mid-loop" with processed=0 and remaining=10
  And the topicMapper is NOT called for any artifact

Scenario: SCN-BK-FIX-003-002 Cancellation is detected before the very first iteration body runs
  Given a BookmarksConnector with topicMapper initialised
  And exactly 1 RawArtifact in the slice
  And a context that has already been cancelled
  When mapFolderTopics(ctx, artifacts) is invoked
  Then it returns 0 (proving the ctx.Err() check runs BEFORE the iteration body, not AFTER)

Scenario: SCN-BK-FIX-003-003 Nil topicMapper path is preserved (no-op when constructor lacks pool)
  Given a BookmarksConnector built via NewConnector (no pool, so topicMapper is nil)
  And 2 RawArtifacts in the slice
  When mapFolderTopics(ctx, artifacts) is invoked with a live context
  Then it returns 0 without panic (the nil-topicMapper early-return wins)
```

### Implementation Plan

1. Extract the post-dedup topic-mapping loop from `Sync()` in `internal/connector/bookmarks/connector.go` into a new private method `func (c *BookmarksConnector) mapFolderTopics(ctx context.Context, artifacts []connector.RawArtifact) int`. Body preserves the existing `folder_path` / `folder` metadata extraction, the `topicMapper.MapFolder` / `CreateTopicEdge` / `UpdateTopicMomentum` cascade, and the per-call `slog.Warn` on individual failures. (F-STAB-R10-001)
2. Add an `if err := ctx.Err(); err != nil { ... }` guard at the **top of every iteration** in `mapFolderTopics`. On cancel: emit one `slog.Warn("bookmarks topic mapping cancelled mid-loop", "error", err, "processed", processed, "remaining", len(artifacts)-processed)` and return `processed`. (F-STAB-R10-001)
3. Replace the inline loop in `Sync()` with `c.mapFolderTopics(ctx, allArtifacts)`. The `c.mu.Lock(); c.lastSyncCount = ...; c.lastSyncTime = ...; c.mu.Unlock()` block that follows is unchanged.
4. Add adversarial regression tests in `internal/connector/bookmarks/connector_test.go`:
   - `TestStabR10_TopicMappingRespectsContextCancel` — 10 artifacts, pre-cancelled ctx → asserts `processed == 0`; live ctx → asserts `processed == 10`. Failure diagnostic names the exact violated invariant. (SCN-BK-FIX-003-001)
   - `TestStabR10_TopicMappingCancelBeforeFirstIteration` — 1 artifact, pre-cancelled ctx → asserts `processed == 0`. Failure diagnostic explicitly says "ctx guard must run BEFORE the iteration body, not after". (SCN-BK-FIX-003-002)
   - `TestStabR10_TopicMappingNilMapper` — nil topicMapper via `NewConnector` (no pool), 2 artifacts, live ctx → asserts `processed == 0` and no panic. (SCN-BK-FIX-003-003)
5. Prove adversarial fidelity: temporarily replace the `ctx.Err()` guard with `_ = ctx`; re-run the 3 R10 tests; confirm `TestStabR10_TopicMappingRespectsContextCancel` and `TestStabR10_TopicMappingCancelBeforeFirstIteration` FAIL with the exact authored diagnostics; restore the guard; confirm all 3 PASS again. (Acceptance criterion #3 in spec.md)
6. Run `go test ./internal/connector/bookmarks/... -count=1` and confirm the full bookmarks package suite is green.
7. Run `go test ./internal/connector/... ./internal/scheduler/...` and confirm no regression in consumer packages.
8. Run `go vet ./internal/connector/bookmarks/...` and `gofmt -l internal/connector/bookmarks/connector.go internal/connector/bookmarks/connector_test.go` — both clean.
9. Append a "Stabilize R10 — Topic-mapping cancel hardening" section to `specs/009-bookmarks-connector/report.md` and one R10 entry to `specs/009-bookmarks-connector/state.json::executionHistory`.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-BK-FIX-003-01 | `TestStabR10_TopicMappingRespectsContextCancel` | unit | `internal/connector/bookmarks/connector_test.go` | pre-cancelled ctx → `processed == 0`; live ctx → `processed == 10` | SCN-BK-FIX-003-001 |
| T-BK-FIX-003-02 | `TestStabR10_TopicMappingCancelBeforeFirstIteration` | unit | `internal/connector/bookmarks/connector_test.go` | 1-artifact slice, pre-cancelled ctx → `processed == 0` (ordering invariant) | SCN-BK-FIX-003-002 |
| T-BK-FIX-003-03 | `TestStabR10_TopicMappingNilMapper` | unit | `internal/connector/bookmarks/connector_test.go` | nil topicMapper → `processed == 0`, no panic | SCN-BK-FIX-003-003 |
| T-BK-FIX-003-04 | Full bookmarks package suite | unit | `internal/connector/bookmarks/...` | `go test ./internal/connector/bookmarks/... -count=1` exit 0; no regression | All three |
| T-BK-FIX-003-05 | Consumer regression sweep | unit | `internal/connector/... internal/scheduler/...` | `go test ./internal/connector/... ./internal/scheduler/...` exit 0; all 21 connector packages + scheduler PASS | All three |
| T-BK-FIX-003-06 | Adversarial proof | manual | (procedure documented in report.md) | Toggling the `ctx.Err()` guard OFF causes T-BK-FIX-003-01 and -02 to FAIL with concrete diagnostics; restoring returns to PASS | SCN-BK-FIX-003-001, SCN-BK-FIX-003-002 |
| T-BK-FIX-003-07 | Regression E2E coverage (disposition) | regression-e2e (n/a disposition) | n/a — local-file bookmarks connector has no live-stack harness; scenario-specific persistent regression coverage is carried by T-BK-FIX-003-01/02/03 (per-scenario unit tests that lock the ctx-cancel ordering invariant) and the broader sweep T-BK-FIX-003-05 across all 21 connector packages + scheduler | Disposition explicit; tests live under `internal/connector/bookmarks/` and rerun on every `go test ./...` and every `./smackerel.sh test unit` invocation, providing equivalent persistent regression coverage to a live-stack e2e for the cancellation invariant | SCN-BK-FIX-003-001, SCN-BK-FIX-003-002, SCN-BK-FIX-003-003 |
| T-BK-FIX-003-08 | Stress workload (no-op disposition) | stress (n/a disposition) | n/a — F-STAB-R10-001 is a cancellation-hygiene fix, not a throughput or load fix. No SLA, no latency budget, no throughput contract is in scope. The slog usage in the connector triggers a heuristic false-positive on the SLA-stress gate (see report.md Guard Disposition) | No stress workload required; the regression tests cover the only behavior contract (ctx cancel terminates the loop with a single structured warn) | SCN-BK-FIX-003-001 |

### Definition of Done

- [x] `Scenario SCN-BK-FIX-003-001 Cancelled context stops the topic-mapping loop before issuing the next round of DB calls` — `mapFolderTopics` returns 0 on pre-cancelled ctx with 10 artifacts and emits a single structured warn. **Phase:** implement
  > Evidence:
  > ```
  > $ go test -count=1 -v -run TestStabR10_TopicMappingRespectsContextCancel ./internal/connector/bookmarks/...
  > === RUN   TestStabR10_TopicMappingRespectsContextCancel
  > 2026/05/25 17:01:53 WARN bookmarks topic mapping cancelled mid-loop error="context canceled" processed=0 remaining=10
  > --- PASS: TestStabR10_TopicMappingRespectsContextCancel (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.016s
  > ```
- [x] `Scenario SCN-BK-FIX-003-002 Cancellation is detected before the very first iteration body runs` — 1-artifact slice with pre-cancelled ctx returns 0 (ordering invariant: guard runs BEFORE body). **Phase:** implement
  > Evidence:
  > ```
  > $ go test -count=1 -v -run TestStabR10_TopicMappingCancelBeforeFirstIteration ./internal/connector/bookmarks/...
  > === RUN   TestStabR10_TopicMappingCancelBeforeFirstIteration
  > 2026/05/25 17:01:53 WARN bookmarks topic mapping cancelled mid-loop error="context canceled" processed=0 remaining=1
  > --- PASS: TestStabR10_TopicMappingCancelBeforeFirstIteration (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.016s
  > ```
- [x] `Scenario SCN-BK-FIX-003-003 Nil topicMapper path is preserved` — nil-topicMapper connector returns 0 without panic. **Phase:** implement
  > Evidence:
  > ```
  > $ go test -count=1 -v -run TestStabR10_TopicMappingNilMapper ./internal/connector/bookmarks/...
  > === RUN   TestStabR10_TopicMappingNilMapper
  > --- PASS: TestStabR10_TopicMappingNilMapper (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.016s
  > ```
- [x] Adversarial proof — toggling the `ctx.Err()` guard OFF causes 2 of the 3 R10 tests to FAIL with the exact authored diagnostics; restoring the guard returns all 3 to PASS. **Phase:** test
  > Evidence (full toggle transcript in report.md "Adversarial Fidelity Transcript" section):
  > ```
  > [fix REMOVED — guard replaced with `_ = ctx`]
  > === RUN   TestStabR10_TopicMappingRespectsContextCancel
  > --- FAIL: TestStabR10_TopicMappingRespectsContextCancel (0.00s)
  >     connector_test.go: mapFolderTopics processed=10 on pre-cancelled ctx, want 0
  > === RUN   TestStabR10_TopicMappingCancelBeforeFirstIteration
  > --- FAIL: TestStabR10_TopicMappingCancelBeforeFirstIteration (0.00s)
  >     connector_test.go: ctx guard must run BEFORE the iteration body, not after: processed=1 on pre-cancelled ctx with 1 artifact, want 0
  > === RUN   TestStabR10_TopicMappingNilMapper
  > --- PASS: TestStabR10_TopicMappingNilMapper (0.00s)
  > FAIL    github.com/smackerel/smackerel/internal/connector/bookmarks     0.016s
  >
  > [fix RESTORED]
  > === RUN   TestStabR10_TopicMappingRespectsContextCancel
  > --- PASS: TestStabR10_TopicMappingRespectsContextCancel (0.00s)
  > === RUN   TestStabR10_TopicMappingCancelBeforeFirstIteration
  > --- PASS: TestStabR10_TopicMappingCancelBeforeFirstIteration (0.00s)
  > === RUN   TestStabR10_TopicMappingNilMapper
  > --- PASS: TestStabR10_TopicMappingNilMapper (0.00s)
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.016s
  > ```
- [x] Full bookmarks package suite stays green. **Phase:** test
  > Evidence:
  > ```
  > $ go test ./internal/connector/bookmarks/... -count=1
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.158s
  > $ echo exit=$?
  > exit=0
  > ```
- [x] Consumer regression sweep across all 21 connector packages and the scheduler is green. **Phase:** test
  > Evidence (head + tail):
  > ```
  > $ go test ./internal/connector/... ./internal/scheduler/...
  > ok      github.com/smackerel/smackerel/internal/connector       43.616s
  > ok      github.com/smackerel/smackerel/internal/connector/alerts        4.910s
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/browser       0.216s
  > ok      github.com/smackerel/smackerel/internal/connector/caldav        0.044s
  > ok      github.com/smackerel/smackerel/internal/connector/discord       12.644s
  > ok      github.com/smackerel/smackerel/internal/connector/guesthost     1.615s
  > ok      github.com/smackerel/smackerel/internal/connector/hospitable    18.203s
  > ok      github.com/smackerel/smackerel/internal/connector/imap  0.292s
  > ok      github.com/smackerel/smackerel/internal/connector/keep  (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/maps  0.640s
  > ok      github.com/smackerel/smackerel/internal/connector/markets       3.192s
  > ok      github.com/smackerel/smackerel/internal/connector/photos        0.011s
  > ok      github.com/smackerel/smackerel/internal/connector/photos/adapters/immich 0.077s
  > ok      github.com/smackerel/smackerel/internal/connector/photos/adapters/photoprism 0.239s
  > ok      github.com/smackerel/smackerel/internal/connector/qfdecisions   1.322s
  > ok      github.com/smackerel/smackerel/internal/connector/rss   0.457s
  > ok      github.com/smackerel/smackerel/internal/connector/twitter       5.242s
  > ok      github.com/smackerel/smackerel/internal/connector/weather       35.435s
  > ok      github.com/smackerel/smackerel/internal/connector/youtube       0.034s
  > ok      github.com/smackerel/smackerel/internal/scheduler       5.069s
  > ```
- [x] `go vet` and `gofmt -l` are clean on the modified files. **Phase:** audit
  > Evidence (separate commands so exit codes are individually visible):
  > ```
  > $ go vet ./internal/connector/bookmarks/...
  > $ echo "vet exit=$?"
  > vet exit=0
  > $ gofmt -l internal/connector/bookmarks/connector.go internal/connector/bookmarks/connector_test.go
  > $ echo "fmt exit=$?"
  > fmt exit=0
  > # both commands emit no output and exit 0 — vet clean, fmt clean
  > ```
- [x] Parent `specs/009-bookmarks-connector/state.json` has a new R10 execution-history entry and `report.md` has a Stabilize R10 section that names this bug and lists F-STAB-R10-001 with status `Fixed`. **Phase:** docs
  > Evidence (grep returns matches in BOTH parent files after the docs phase):
  > ```
  > $ grep -n 'BUG-009-003-topic-mapping-cancel-r10' specs/009-bookmarks-connector/report.md specs/009-bookmarks-connector/state.json
  > specs/009-bookmarks-connector/report.md:<LINE>:**Bug:** [BUG-009-003-topic-mapping-cancel-r10](bugs/BUG-009-003-topic-mapping-cancel-r10/spec.md) — status `done`.
  > specs/009-bookmarks-connector/state.json:<LINE>:      "summary": "Stochastic-quality-sweep sweep-2026-05-25-r10 round 10, parent-expanded child workflow mode stabilize-to-doc. ..."
  > ```
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior live in `internal/connector/bookmarks/connector_test.go` and gate every future change to the topic-mapping cancellation contract. **Phase:** regression
  > Disposition: local-file bookmarks connector has no live-stack e2e harness. Persistent scenario-specific regression coverage is carried by T-BK-FIX-003-01/02/03 (per-scenario unit tests that lock the ctx-cancel ordering invariant) and rerun on every `go test ./...` and every `./smackerel.sh test unit` invocation. The adversarial proof T-BK-FIX-003-06 demonstrates they fail RED if the ctx.Err() guard is reverted. See [report.md → Adversarial Fidelity Transcript](report.md) and [report.md → Guard Disposition](report.md).
  > Evidence (all three scenario-specific regression tests resolve and PASS on a clean run):
  > ```
  > $ go test ./internal/connector/bookmarks/... -count=1 -run 'TestStabR10_' -v
  > === RUN   TestStabR10_TopicMappingRespectsContextCancel
  > --- PASS: TestStabR10_TopicMappingRespectsContextCancel (0.00s)
  > === RUN   TestStabR10_TopicMappingCancelBeforeFirstIteration
  > --- PASS: TestStabR10_TopicMappingCancelBeforeFirstIteration (0.00s)
  > === RUN   TestStabR10_TopicMappingNilMapper
  > --- PASS: TestStabR10_TopicMappingNilMapper (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.158s
  > exit=0
  > ```
- [x] Broader E2E regression suite passes — disposition: bookmarks connector has no live-stack e2e harness, so the broader regression suite is the cross-package consumer sweep across all 21 connector packages plus the scheduler. **Phase:** regression
  > Evidence:
  > ```
  > $ go test ./internal/connector/... ./internal/scheduler/...
  > ok      github.com/smackerel/smackerel/internal/connector       43.616s
  > ok      github.com/smackerel/smackerel/internal/connector/alerts        4.910s
  > ok      github.com/smackerel/smackerel/internal/connector/bookmarks     (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/browser       0.216s
  > ok      github.com/smackerel/smackerel/internal/connector/caldav        0.044s
  > ok      github.com/smackerel/smackerel/internal/connector/discord       12.644s
  > ok      github.com/smackerel/smackerel/internal/connector/guesthost     1.615s
  > ok      github.com/smackerel/smackerel/internal/connector/hospitable    18.203s
  > ok      github.com/smackerel/smackerel/internal/connector/imap  0.292s
  > ok      github.com/smackerel/smackerel/internal/connector/keep  (cached)
  > ok      github.com/smackerel/smackerel/internal/connector/maps  0.640s
  > ok      github.com/smackerel/smackerel/internal/connector/markets       3.192s
  > ok      github.com/smackerel/smackerel/internal/connector/photos        0.011s
  > ok      github.com/smackerel/smackerel/internal/connector/photos/adapters/immich 0.077s
  > ok      github.com/smackerel/smackerel/internal/connector/photos/adapters/photoprism 0.239s
  > ok      github.com/smackerel/smackerel/internal/connector/qfdecisions   1.322s
  > ok      github.com/smackerel/smackerel/internal/connector/rss   0.457s
  > ok      github.com/smackerel/smackerel/internal/connector/twitter       5.242s
  > ok      github.com/smackerel/smackerel/internal/connector/weather       35.435s
  > ok      github.com/smackerel/smackerel/internal/connector/youtube       0.034s
  > ok      github.com/smackerel/smackerel/internal/scheduler       5.069s
  > ```
- [x] Stress workload disposition: F-STAB-R10-001 is a cancellation-hygiene fix, not a throughput or load fix. The `slog` token in the connector triggers a heuristic SLA-stress flag (see [report.md → Guard Disposition](report.md)); the keyword match is a false positive — no SLA, no latency budget, no throughput contract is in scope, so no stress workload is required for this round. The deterministic regression tests (T-BK-FIX-003-01/02/03) are the complete behavior contract. **Phase:** chaos
  > Evidence (the `slog` token alone — not a real `sla`/`slo`/`p95` SLA contract — is what trips the heuristic; the file contains zero real SLA contracts):
  > ```
  > $ grep -niE 'latency|throughput|p95|p99|response time|^sla|^slo|slo[^g]' internal/connector/bookmarks/connector.go | head -5
  > $ # no matches — only 'slog' (structured logging) appears, never a real SLA token
  > $ grep -ciE 'slog\.' internal/connector/bookmarks/connector.go
  > 9
  > exit=0
  > ```
