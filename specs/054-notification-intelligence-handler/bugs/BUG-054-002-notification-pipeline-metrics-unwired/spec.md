# Spec: BUG-054-002 — Notification Pipeline Metrics Declared In Design But Never Wired

**Parent Spec:** 054-notification-intelligence-handler
**Discovered:** 2026-06-06 (stochastic-quality-sweep observation `OBS-054-15-001`, trigger=observation, mapped child workflow=bugfix-fastlane)
**Mode:** bugfix-fastlane (real implementation: metrics declaration + pipeline emit wiring + adversarial tests)

## Use Cases

- **UC-01 — Operators can observe source-qualified notification pipeline health.** When the notification handler ingests, normalizes, deduplicates, decides on, and delivers notifications, each pipeline stage emits a `smackerel_notification_*` Prometheus series labeled by `source_type`/`source_form`/`stage`/`status`, so an operator can alert on a stuck stage (e.g. a spike in `ingest_total{status="rejected"}` or `delivery_attempts_total{status="failure"}`) and attribute it to a source — making the Scope 8 DoD claim ("Metrics and traces expose source-qualified pipeline stages") actually true.
- **UC-02 — Security reviewers can verify observability does not leak secrets.** When a reviewer scrapes the notification metrics, every label value is a bounded enum or a registered source identifier — never raw payload/title/body — satisfying SCN-054-024 ("observability must not leak secrets"). A reviewer can assert this mechanically by gathering the families and checking the label allowlist.
- **UC-03 — Future regressions in the emit wiring are caught.** When a future change removes or breaks an emit site (`.Inc()`/`.Observe()`), the adversarial unit tests in `internal/notification/metrics_emit_test.go` fail, preventing the observability gap from silently re-opening.

## Functional Requirements

- **FR-01 — Declare 6 core notification metrics under the `smackerel_notification_` prefix.** `internal/metrics/metrics.go` MUST declare, following the existing package-level `var X = prometheus.New*Vec(...)` + `init()` `MustRegister` pattern, exactly: `ingest_total` (counter; `source_type,source_form,status`), `normalization_errors_total` (counter; `source_type,error_kind`), `dedupe_total` (counter; `source_type,suppression_kind`), `action_attempts_total` (counter; `action_class,status`), `delivery_attempts_total` (counter; `channel,status`), `processing_duration_ms` (histogram; `stage`).
- **FR-02 — Wire each metric at a real live emit site.** Every declared metric MUST be incremented/observed from the live notification pipeline: ingest at `Service.Process` (post-`CreateRawEvent`), normalization error at `Normalizer.Normalize`, dedupe at `LoopGuard.Evaluate` (reaction-loop) + `Service.Process` (store-found suppressions), action at `DecisionEngine.Decide`, delivery at `OutputDispatcher.Dispatch`, and per-stage duration at `Service.Process`. No declared metric may be dead (registered-but-never-incremented).
- **FR-03 — Bounded-enum labels only (no payload leakage).** Every label value MUST be a bounded enum (`source_form`, `status`, `error_kind`, `suppression_kind`, `action_class`, `stage`) or a known source/channel identifier (`source_type`, `channel`). No label value may derive from `RawPayload`, `Title`, `Body`, or any free-text notification content (SCN-054-024).
- **FR-04 — Adversarial test coverage.** `internal/notification/metrics_emit_test.go` MUST assert (a) all 6 families are registered with `prometheus.DefaultGatherer` carrying only their bounded-label allowlist, (b) `ingest_total` and `delivery_attempts_total` increment with the expected labels via the real wiring (adversarial: failing if the emit is removed), (c) `action_attempts_total`, `normalization_errors_total`, and `dedupe_total` increment via their real emit sites, and (d) no gathered notification-metric label value contains injected payload content.
- **FR-05 — Honest deferral of the remaining 6 design metrics.** The 6 other design.md metrics (`source_health_state`, `source_lag_seconds`, `classification_confidence_bucket`, `incidents_open`, `incident_transitions_total`, `action_failures_total`) MUST NOT be declared as dead metrics; each MUST be recorded as explicitly deferred with rationale in `bug.md`/`report.md`.

## Acceptance Criteria

- **AC-01** — `go build ./...` exits 0.
- **AC-02** — `go vet ./internal/notification/... ./internal/metrics/...` exits 0.
- **AC-03** — `go test ./internal/metrics/... ./internal/notification/... -count=1 -v` is all GREEN, including the 7 new `TestNotification*` tests, with package summary lines `ok github.com/smackerel/smackerel/internal/metrics` and `ok github.com/smackerel/smackerel/internal/notification`.
- **AC-04** — All 6 `smackerel_notification_*` families appear in `prometheus.DefaultGatherer.Gather()` after the emit sites are exercised, and each carries only its bounded-label allowlist (`TestNotificationMetricFamiliesRegisteredWithBoundedLabels`).
- **AC-05** — Disabling the `delivery_attempts_total` `.Inc()` site makes `TestNotificationDeliveryAttemptsIncrementsOnDispatch` FAIL with `= 0, want 1` (RED proof captured in `report.md`); restoring it makes it PASS (GREEN).
- **AC-06** — `TestNotificationMetricsDoNotLeakPayloadInLabels` passes: no notification-metric label value contains the injected `SUPERSECRET-PAYLOAD-MARKER`.
- **AC-07** — `bash .github/bubbles/scripts/artifact-lint.sh specs/054-notification-intelligence-handler/bugs/BUG-054-002-notification-pipeline-metrics-unwired` returns `Artifact lint PASSED.`.
- **AC-08** — `bash .github/bubbles/scripts/state-transition-guard.sh specs/054-notification-intelligence-handler/bugs/BUG-054-002-notification-pipeline-metrics-unwired` exits with 0 🔴 BLOCKs.
- **AC-09** — `git diff --cached --name-status` for the closure (captured pre-commit) lists ONLY `internal/metrics/metrics.go`, the 5 `internal/notification/*.go` pipeline files, `internal/notification/metrics_emit_test.go`, and paths under `specs/054-notification-intelligence-handler/bugs/BUG-054-002-notification-pipeline-metrics-unwired/` — no `.github/bubbles/**` framework files, no other spec.
- **AC-10** — The 6 deferred design.md metrics are NOT declared in `metrics.go` (no dead metrics) and are documented as deferred-with-rationale in `bug.md` and `report.md`.

## Product Principle Alignment

- **Principle 8 — Trust Through Transparency.** Source-qualified pipeline-stage metrics make the notification handler's behavior observable and attributable to a source without exposing notification content. The bounded-label contract (no payload in labels) is the transparency-without-leakage guarantee, verified by `TestNotificationMetricsDoNotLeakPayloadInLabels`.
- **Principle 4 — Source-Qualified Processing.** Every wired metric preserves `source_type`/`source_form` (or a stage/channel identifier) so operators can attribute pipeline behavior to its source. Evidence: `design.md` §Metrics and `docs/smackerel.md`.
