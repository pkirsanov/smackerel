# Design: BUG-054-002 — Notification Pipeline Metrics Declared In Design But Never Wired

## Current Truth (Phase 0.55 — solution-blind reality probe)

**HEAD SHA:** `46def326` (current at probe time 2026-06-06)
**Probe timestamp:** 2026-06-06
**Probed surface:** `internal/metrics/metrics.go` registration list, `internal/notification/` package for any Prometheus metric declaration/reference, the live `Service.Process` pipeline, and spec 054 `design.md` §Metrics.

### Findings — Observability gap (the bug)

- **`internal/notification/` registers ZERO Prometheus metrics at HEAD `46def326`.** No `prometheus.New*` declaration and no `metrics.Notification*` reference existed anywhere in the notification package or its pipeline (`service.go`, `normalizer.go`, `correlation.go`, `decision.go`, `output_logic.go`, `reaction_logic.go`).
- **`design.md` §"Observability And Failure Handling → Metrics" (lines ~861-876) fully specifies a 12-row `smackerel_notification_*` table.** None of it was implemented.
- **Scope 8 DoD claim "Metrics and traces expose source-qualified pipeline stages without leaking secrets" was therefore not true as written.** There was no source-qualified pipeline-stage metric to expose.

### Findings — Live pipeline emit-site reality (confirmed by reading the code)

| Stage | Live emit site | Pure/DB-bound | Unit-testable without DB? |
|---|---|---|---|
| ingest | `service.go` `Service.Process` after `Store.CreateRawEvent` (which returns a clean error — not a panic — on a nil pool, `store_pipeline.go:13`) | DB-bound accept / unit-reachable reject | yes (nil-pool reject path) |
| normalize | `normalizer.go` `Normalizer.Normalize` (pure; called in `Process`) | pure | yes |
| dedupe | `reaction_logic.go` `LoopGuard.Evaluate` (pure; called in `Process`) + `service.go` `Process` over `Store.FindSuppressions` results | pure (loop) / DB-bound (store) | yes (loop path) |
| decide/action | `decision.go` `DecisionEngine.Decide` (pure; called in `Process`) | pure | yes |
| deliver | `output_logic.go` `OutputDispatcher.Dispatch` (pure; no DB) | pure | yes |
| duration | `service.go` `Service.Process` per-stage timers | DB-bound (full) / unit-reachable (ingest+total on reject) | yes (reject path observes ingest+total) |

- **Dead-code note:** `internal/notification/incident_logic.go` is `//go:build ignore` (excluded from compilation). The live `Deduper`/`Correlator` are in `correlation.go`; `Deduper.Evaluate` is NOT invoked in `Process()` (suppression in the live path comes from `Store.FindSuppressions` + `LoopGuard.Evaluate`). Wiring dedupe at `LoopGuard.Evaluate` + the `Process` FindSuppressions loop is therefore the correct LIVE site; wiring it at `Deduper.Evaluate` would have produced a dead emit.
- **`DecisionType` enum (`model.go:75`):** `no_action, record_only, diagnostics, autonomous_handling, user_escalation, approval_request` — bounded; safe `action_class` label.
- **`SourceForm` enum (`types.go`):** `stream, webhook, polling, queue, file_drop, api_pull, manual` — bounded; safe `source_form` label.

### Findings — No import cycle

`internal/metrics` imports only `net/http` + prometheus (plus its own `recommendations.go`/`backup.go`); it does NOT import `internal/notification`. Therefore `internal/notification` importing `internal/metrics` introduces no cycle (`go build ./...` confirmed at probe time after wiring).

## Design Decisions

### DD-1 — Wire the 6 core pipeline-stage metrics; defer the other 6 with rationale

The Scope 8 DoD claim is specifically about **source-qualified pipeline stages**. The 6 core metrics map one-to-one onto the stages (ingest → normalize → dedupe → decide/action → deliver) plus per-stage latency. The other 6 design.md metrics (`source_health_state`, `source_lag_seconds`, `classification_confidence_bucket`, `incidents_open`, `incident_transitions_total`, `action_failures_total`) depend on richer state/lifecycle plumbing (health lifecycle, scheduler ticks, store-scan gauges, the state-machine transition call sites, the action-executor failure path) that is outside the pipeline-stage DoD surface. They are **deferred, NOT declared** — a registered-but-never-incremented metric would be a fabricated observability claim. Rationale per metric is in `bug.md`/`report.md`.

### DD-2 — Follow the existing metrics.go pattern exactly

Each metric is a package-level `var Notification… = prometheus.New{Counter,Histogram}Vec(...)` added to the single `init()` `MustRegister(...)` list — identical to the 40+ existing `smackerel_*` families. Histogram buckets for `processing_duration_ms` span sub-millisecond in-memory stages through multi-second store-bound stages (`0.1 … 2500` ms). This keeps the change idiomatic and reviewable.

### DD-3 — Named-return + `defer` for single-exit emit on multi-return functions

`Normalizer.Normalize`, `DecisionEngine.Decide`, and `OutputDispatcher.Dispatch` each have multiple `return` points. Rather than scatter `.Inc()` across every branch (error-prone, easy to miss a branch), each uses a named return value (`(result …, err error)` / `(decision …)`) and a single `defer` that reads the final value and emits exactly once. This guarantees the emit fires on every exit path and derives the bounded `status`/`error_kind` from the final decision/result/error — robust to future branch additions.

### DD-4 — Bounded label derivation (redaction safety, SCN-054-024)

| Label | Source | Bounded by |
|---|---|---|
| `source_type` | `envelope.SourceType` / `raw.SourceType` / `normalized.SourceType` | registered adapter types (finite) |
| `source_form` | `string(envelope.SourceForm)` | `SourceForm` enum |
| `status` (ingest) | literal `accepted`/`rejected` | finite |
| `error_kind` | `normalizationErrorKind(err)` | `{none, missing_raw_event_id, missing_source_identity, missing_observed_at, source_event_id_derivation, other}` |
| `suppression_kind` | `Suppression.Kind` / `SuppressionReactionLoop` | `{dedupe, reaction_loop}` |
| `action_class` | `string(decision.Type)` | `DecisionType` enum |
| `status` (action) | `notificationActionStatus(decision)` | `{approval_required, diagnostics, output_required, suppressed, recorded}` |
| `channel` | `request.Channel` | configured output-channel ids (finite) |
| `status` (delivery) | derived from `err`/`result.Status` | `{success, failure}` |
| `stage` | literal `ingest`/`normalize`/`decide`/`total` | finite |

No label value derives from `RawPayload`, `Title`, or `Body`. The `normalizationErrorKind` classifier maps the free-text error string to a bounded kind so the raw error never becomes label cardinality. `TestNotificationMetricsDoNotLeakPayloadInLabels` is the adversarial proof.

### DD-5 — Adversarial, DB-free unit tests

All 6 emit sites are reachable without a live Postgres:
- delivery/action/normalize/dedupe(loop) emit sites are pure functions.
- ingest is exercised via the **nil-pool reject path** (`NewStore(nil)` → `CreateRawEvent` returns a clean error → `ingest_total{status="rejected"}` + `processing_duration_ms{ingest,total}` fire).

This lets the full adversarial suite run in the standard `go test ./internal/notification/...` command (no `integration` build tag, no DATABASE_URL). The store-found-suppression `dedupe` branch and the `ingest{status="accepted"}` branch additionally fire in the existing `//go:build integration` `Service.Process` tests when a DB is present; they are not required for the DoD claim's unit proof.

### DD-6 — RED→GREEN adversarial proof

To prove the increment tests are not tautological, the `delivery_attempts_total` `.Inc()` was temporarily disabled and `TestNotificationDeliveryAttemptsIncrementsOnDispatch` was re-run: it FAILED with `= 0, want 1`. Restoring the emit returns it to GREEN. The full RED→GREEN block is in `report.md` §Test Evidence.

## Consumer Impact

No route/path/contract/identifier is renamed or removed. The change is **purely additive observability** (new metric series + emit lines). Existing notification behavior, API contracts, and store schema are unchanged. Consumer surfaces: none — there are no downstream consumers of a behavior change because there is no behavior change; the only new surface is the Prometheus `/metrics` scrape, which gains 6 additive families.
