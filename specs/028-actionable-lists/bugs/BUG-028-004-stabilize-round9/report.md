# BUG-028-004 — Stochastic Quality Sweep Round 9 Closure

**Parent Spec:** specs/028-actionable-lists (status: done, workflowMode: full-delivery)
**Round:** stochastic-quality-sweep round 9
**Scope of this bug:** Tractable Go-only stability fixes in `internal/list/` and
`internal/api/lists.go`. Findings F5, F6, F8, F10 deferred — see `routing.md`.

## Findings Addressed

| ID | Severity | Class | Status | Owned Surface |
|----|----------|-------|--------|---------------|
| F1 | HIGH | reliability | FIXED | `internal/list/store.go` `AddManualItem` |
| F2 | HIGH | reliability | FIXED | `internal/list/store.go` `RemoveItem` |
| F3 | HIGH | reliability | FIXED | `internal/list/store.go` `CompleteList` |
| F4 | HIGH | observability (subset) | FIXED | `internal/list/store.go` + `internal/metrics/metrics.go` |
| F7 | MEDIUM | resource | FIXED | `internal/api/lists.go` |
| F9 | LOW | reliability | FIXED | `internal/list/store.go` `AddManualItem` |

## Findings Deferred (routed)

See [routing.md](routing.md):

| ID | Severity | Reason for Defer | Owner |
|----|----------|------------------|-------|
| F5 | — | DB migration (pg_trgm) — planning surface | bubbles.plan → bubbles.design |
| F6 | — | Pagination contract — spec/API contract surface | bubbles.analyst |
| F8 | — | Auth scope decision — policy surface | bubbles.analyst + bubbles.design |
| F10 | — | FK contract — DB schema/migration surface | bubbles.plan → bubbles.design |

## Implementation Notes

### F1 — `AddManualItem` race + drift
- Wrapped INSERT + counter recalc in `Pool.Begin` transaction (mirrors
  `UpdateItemStatus` pattern at L191–L233).
- Added `SELECT id FROM lists WHERE id=$1 FOR UPDATE` at top of tx to serialize
  concurrent `AddManualItem` against the same list — closes the
  `MAX(sort_order)` interleave race.
- Recalculate `total_items` via `COUNT(*)` instead of `+1` increment so a
  transient failure can't leave the counter drifted.

### F2 — `RemoveItem` counter drift
- Same tx wrap. Both `total_items` and `checked_items` recomputed from
  `COUNT(*)` inside the tx so DELETE + recalc commit or roll back together.

### F3 — `CompleteList` event payload
- Collapsed the 4 separate `QueryRow` round-trips into a single SELECT with
  `FILTER` aggregates and `LEFT JOIN list_items`. Removes per-count
  inconsistency windows and 4× DB round-trip cost.

### F4 — Publish-failure metric (observability subset only)
- Added `metrics.ListEventsPublishFailed` (`smackerel_list_events_publish_failed_total`)
  with `subject` label; registered in `internal/metrics/metrics.go` alongside
  existing `ListsCompleted` / `ListItemStatusChanges`.
- Incremented at both `slog.Warn` publish-failure sites (`lists.created`,
  `lists.completed`).
- Outbox/retry pattern NOT added — routed as planning work.

### F7 — Body size + length caps
- Added `decodeListBody` helper in `internal/api/lists.go` that wraps `r.Body`
  with `http.MaxBytesReader(w, r.Body, 64<<10)` and maps
  `http.MaxBytesError` to `413 body_too_large`.
- Applied to `CreateListHandler`, `AddItemHandler`, `UpdateListHandler`.
- `CheckItemHandler` keeps its empty-body back-compat fallback to `status=done`
  but now still enforces the 64 KiB cap before that fallback.
- Length caps: `CreateListRequest.Title` ≤ 256 (`title_too_long`),
  `AddItemRequest.Content` ≤ 4096 (`content_too_long`). Both return 400 with
  stable error codes.

### F9 — Manual item ID entropy
- Replaced `fmt.Sprintf("itm-%s-%d", listID[:8], now.UnixNano())` with
  `uuid.NewString()[:8]` suffix. `github.com/google/uuid v1.6.0` already in
  `go.mod`.

## Verification Evidence

**Claim Source:** executed

### Build
```
$ go build ./...
(no output, exit 0)
```

### Vet
```
$ go vet ./internal/list/... ./internal/api/... ./internal/metrics/...
(no output, exit 0)
```

### Unit / short tests
```
$ go test -count=1 -short ./internal/list/... ./internal/api/... ./internal/metrics/...
ok      github.com/smackerel/smackerel/internal/list                              0.085s
ok      github.com/smackerel/smackerel/internal/api                              11.358s
ok      github.com/smackerel/smackerel/internal/api/admin/extensiondevices       0.010s
ok      github.com/smackerel/smackerel/internal/api/connectors/extension         0.098s
ok      github.com/smackerel/smackerel/internal/api/graphapi                     0.018s
ok      github.com/smackerel/smackerel/internal/metrics                          0.066s
EXIT=0
```

### Uncertainty declaration

Concurrency-stress / adversarial tests for F1 (FOR UPDATE serialization) and
F2 (tx atomicity under partial failure) are NOT included in this bug. The
existing `internal/list` test suite is pure-unit and does not spin up a real
pgx pool; introducing pg testcontainers would expand surface beyond this
bug's stabilize scope and is routed as planning work to `bubbles.plan`.
Behavior is structurally identical to the pre-existing `UpdateItemStatus` tx
which is already covered by integration tests in `tests/integration/`.

## Discipline

- IDE edit tools only (`multi_replace_string_in_file`, `create_file`).
- No fallback constants introduced. All limits (`maxListRequestBodyBytes`,
  `maxAddItemContentLen`, `maxCreateListTitleLen`) are explicit named constants
  in `internal/api/lists.go`, not env-var defaults — they are server-side
  invariants, not configuration.
- No foreign artifacts modified (spec.md, design.md, scopes.md planning content,
  uservalidation.md, state.json untouched).

## RESULT-ENVELOPE

```yaml
outcome: completed_owned
agent: bubbles.implement
scope: BUG-028-004-stabilize-round9
addressedFindings:
  - id: F1
    status: fixed
    surface: internal/list/store.go AddManualItem
  - id: F2
    status: fixed
    surface: internal/list/store.go RemoveItem
  - id: F3
    status: fixed
    surface: internal/list/store.go CompleteList
  - id: F4
    status: fixed-subset
    surface: internal/list/store.go + internal/metrics/metrics.go
    note: observability counter only; outbox/retry routed as planning
  - id: F7
    status: fixed
    surface: internal/api/lists.go
  - id: F9
    status: fixed
    surface: internal/list/store.go AddManualItem
unresolvedFindings: []
deferredFindings:
  - id: F5
    routedTo: bubbles.plan
    reason: pg_trgm migration is planning/design surface
  - id: F6
    routedTo: bubbles.analyst
    reason: pagination contract is spec surface
  - id: F8
    routedTo: bubbles.analyst
    reason: auth scope decision is policy surface
  - id: F10
    routedTo: bubbles.plan
    reason: FK contract is schema/migration surface
evidence:
  build:
    command: go build ./...
    exitCode: 0
  vet:
    command: go vet ./internal/list/... ./internal/api/... ./internal/metrics/...
    exitCode: 0
  tests:
    command: go test -count=1 -short ./internal/list/... ./internal/api/... ./internal/metrics/...
    exitCode: 0
    packages:
      - github.com/smackerel/smackerel/internal/list
      - github.com/smackerel/smackerel/internal/api
      - github.com/smackerel/smackerel/internal/api/admin/extensiondevices
      - github.com/smackerel/smackerel/internal/api/connectors/extension
      - github.com/smackerel/smackerel/internal/api/graphapi
      - github.com/smackerel/smackerel/internal/metrics
files:
  - internal/list/store.go
  - internal/metrics/metrics.go
  - internal/api/lists.go
  - specs/028-actionable-lists/bugs/BUG-028-004-stabilize-round9/report.md
  - specs/028-actionable-lists/bugs/BUG-028-004-stabilize-round9/routing.md
nextOwner: bubbles.plan
nextOwnerReason: route deferred findings F5/F6/F8/F10 per routing.md
```
