# BUG-054-002 — Notification Pipeline Metrics Declared In Design But Never Wired

**Spec:** 054-notification-intelligence-handler
**Severity:** Observability gap (real implementation fix; Scope 8 DoD claim was not actually true)
**Status:** open → resolved
**Discovered:** 2026-06-06
**Discovered by:** stochastic-quality-sweep observation `OBS-054-15-001` (Scope 8 DoD ⇄ source-code reality mismatch)
**Closure Mode:** bugfix-fastlane (real code: `internal/metrics/metrics.go` + 5 notification pipeline files + 1 adversarial test file)

---

## Summary

Spec 054 Scope 8's Definition of Done claims:

> "Metrics and traces expose source-qualified pipeline stages without leaking secrets."

`design.md` §"Observability And Failure Handling → Metrics" (lines ~861-876) fully specifies a `smackerel_notification_*` metric family. But the implementation never wired ANY of it: a workspace scan at baseline HEAD `46def326` shows `internal/notification/` registered **ZERO** Prometheus metrics — there was not a single `prometheus.New*` declaration or `metrics.Notification*` reference anywhere in the notification package or its pipeline. The Scope 8 DoD claim was therefore **not true** as written: there was no source-qualified pipeline-stage metric to expose.

This BUG packet makes the claim true by declaring and wiring the **6 core pipeline-stage metrics** that the DoD claim depends on — one per source-qualified pipeline stage (ingest → normalize → dedupe → decide/action → deliver) plus per-stage latency — each at a **real emit site** in the live pipeline, with **bounded-enum labels only** (SCN-054-024: observability must not leak secrets), and each covered by an **adversarial test** that fails if the `.Inc()`/`.Observe()` site is removed.

| Metric (`smackerel_notification_…`) | Type | Labels | Real emit site (file:func) |
|---|---|---|---|
| `ingest_total` | counter | `source_type, source_form, status` | `service.go` `Service.Process` (after `CreateRawEvent`) |
| `normalization_errors_total` | counter | `source_type, error_kind` | `normalizer.go` `Normalizer.Normalize` (error defer) |
| `dedupe_total` | counter | `source_type, suppression_kind` | `reaction_logic.go` `LoopGuard.Evaluate` + `service.go` `Process` (FindSuppressions) |
| `action_attempts_total` | counter | `action_class, status` | `decision.go` `DecisionEngine.Decide` (named-return defer) |
| `delivery_attempts_total` | counter | `channel, status` | `output_logic.go` `OutputDispatcher.Dispatch` (named-return defer) |
| `processing_duration_ms` | histogram | `stage` | `service.go` `Service.Process` (per-stage `{ingest,normalize,decide,total}`) |

The remaining 6 metrics enumerated in `design.md` (`source_health_state`, `source_lag_seconds`, `classification_confidence_bucket`, `incidents_open`, `incident_transitions_total`, `action_failures_total`) are **explicitly deferred** (see Root Cause → Deferred Metrics). No dead, registered-but-never-incremented metric is declared.

This is a **real implementation** fix — `internal/metrics/metrics.go` (+95 lines), the 5 notification pipeline files (+74/−5 lines), and a new adversarial test file `internal/notification/metrics_emit_test.go`. G093 delivery-delta is satisfied by the actual code diff.

---

## Root Cause

The notification handler (spec 054) was implemented and certified with the pipeline behavior (ingest/normalize/classify/correlate/dedupe/decide/deliver) but the observability layer specified in `design.md` was never translated into code. The `metrics` package (`internal/metrics/metrics.go`) registers dozens of `smackerel_*` families for other surfaces (artifacts, search, connectors, drive, QF, backup, …) but had **no** `smackerel_notification_*` family. The Scope 8 DoD item ("Metrics and traces expose source-qualified pipeline stages without leaking secrets") was checked `[x]` against the design intent rather than against a wired emit site — a DoD ⇄ reality drift that the stochastic sweep observation `OBS-054-15-001` surfaced.

### Why the 6 core metrics (and not all 12)

The DoD claim is specifically about **source-qualified pipeline stages**. The pipeline stages are: ingest → normalize → dedupe → decide/action → deliver, plus per-stage latency. Those map exactly to the 6 core metrics, each wired at the live stage boundary with a bounded `source_type`/`source_form`/`stage`/`status` label. Wiring these 6 makes the DoD claim true with real evidence.

### Deferred Metrics (NO dead metric declared — honest deferral with rationale)

| Deferred metric | Why deferred (no real emit site wireable within this fix's DoD surface) |
|---|---|
| `source_health_state` (gauge) | Closest candidate is `Service.ReportSourceHealth`, but the design label set adds `source_instance_id` (per-instance cardinality outside the bounded core-6 contract) and a correct gauge needs source-registry connect/disconnect lifecycle plumbing (set on connect, clear on stop) that is not part of the ingest→deliver pipeline this fix covers. |
| `source_lag_seconds` (gauge) | Requires a periodic `now − last_event_at` computation emitted on a scheduler tick; the notification path has no such tick today. |
| `classification_confidence_bucket` (histogram) | Wireable at `Classifier.Classify`, but it is a classification-quality distribution, not a pipeline-stage-coverage signal; deferred to keep this fix scoped to the Scope 8 DoD claim (pipeline stages) rather than expanding into classifier analytics. |
| `incidents_open` (gauge) | Requires a periodic store-scan count of open incidents by `status,severity` emitted on a tick; no tick exists in the notification path. |
| `incident_transitions_total` (counter) | Emit site is `IncidentStateMachine.Transition`, which is NOT invoked in the live `Process()` path (incidents are upserted via `Store.UpsertIncident`); wiring needs the resolve/escalate transition call sites that are outside this fix's surface. |
| `action_failures_total` (counter) | Belongs to the action-execution failure path (`ActionExecutor`), a separate surface from the decide/deliver pipeline-stage coverage this fix delivers. |

Each deferred metric is **NOT declared** in `metrics.go` — a registered-but-never-incremented metric would itself be a fabricated observability claim. They are recorded here for a future, separately-scoped fix.

### Redaction safety (SCN-054-024)

Every label on the 6 wired metrics is a **bounded enum or known source identifier**: `source_type` (registered adapter type), `source_form` (the `SourceForm` enum), `status` (`accepted|rejected` / `success|failure` / decision-derived), `error_kind` (bounded classifier), `suppression_kind` (`dedupe|reaction_loop`), `action_class` (the `DecisionType` enum), `channel` (configured output-channel id), `stage` (`ingest|normalize|decide|total`). **None** derive from `RawPayload`, `Title`, `Body`, or any free-text notification content. `TestNotificationMetricsDoNotLeakPayloadInLabels` is an adversarial proof: it drives a `SUPERSECRET-PAYLOAD-MARKER` through the delivery/normalize/ingest emit sites and asserts no gathered label value contains the marker.

---

## Scope

**In-scope (real code + tests):**

- `internal/metrics/metrics.go` — declare 6 `smackerel_notification_*` metric vars + register them in `init()`.
- `internal/notification/service.go` — wire `ingest_total`, `dedupe_total` (FindSuppressions branch), `processing_duration_ms` per-stage; add a stage-duration helper.
- `internal/notification/normalizer.go` — wire `normalization_errors_total` via named-return defer + bounded `error_kind` classifier.
- `internal/notification/decision.go` — wire `action_attempts_total` via named-return defer + bounded `status` classifier.
- `internal/notification/output_logic.go` — wire `delivery_attempts_total` via named-return defer.
- `internal/notification/reaction_logic.go` — wire reaction-loop `dedupe_total` at `LoopGuard.Evaluate`.
- `internal/notification/metrics_emit_test.go` — new; 7 adversarial unit tests (registration + 5 increment tests + redaction-safety).
- `specs/054-notification-intelligence-handler/bugs/BUG-054-002-notification-pipeline-metrics-unwired/**` — this 8-artifact packet.

**Out-of-scope (NOT touched):**

- The 6 deferred design.md metrics (no dead declarations) — see Root Cause.
- `internal/notification/` business logic (classification, correlation, decision policy, store schema) — unchanged; only observability emit lines added.
- `spec.md`, `design.md`, `scopes.md`, `state.json`, `scenario-manifest.json`, `report.md`, `uservalidation.md` of the **parent** spec 054 — NOT mutated by this packet.
- `.github/bubbles/**` and `.github/agents/**` — framework files, immutable per repo policy (the working tree carries unrelated externally-modified framework files which this packet does NOT stage, revert, or touch).
- Any other spec folder, `cmd/`, `ml/`, `scripts/`, `config/`, `.github/workflows/`, `deploy/`, `smackerel.sh`.

---

## Acceptance

- `go build ./...` exits 0.
- `go vet ./internal/notification/... ./internal/metrics/...` exits 0.
- `go test ./internal/metrics/... ./internal/notification/... -count=1 -v` is all GREEN, including the 7 new `TestNotification*` tests.
- All 6 `smackerel_notification_*` families are registered with `prometheus.DefaultGatherer` and carry only their bounded-label allowlist.
- `ingest_total` and `delivery_attempts_total` each have an adversarial increment test (RED-proven: disabling the emit makes the test fail).
- No metric label value contains payload/title/body content.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/054-notification-intelligence-handler/bugs/BUG-054-002-notification-pipeline-metrics-unwired` returns PASSED.
- `bash .github/bubbles/scripts/state-transition-guard.sh specs/054-notification-intelligence-handler/bugs/BUG-054-002-notification-pipeline-metrics-unwired` returns 0 BLOCKs.
