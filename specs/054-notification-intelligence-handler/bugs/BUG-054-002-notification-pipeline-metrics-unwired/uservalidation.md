# User Validation: BUG-054-002 — Notification Pipeline Metrics Declared In Design But Never Wired

**Closure status:** Resolved (real implementation: additive observability; zero behavior change to the notification pipeline)

## User-facing impact

- **Operators / DevOps:** New capability. The notification handler now emits 6 source-qualified `smackerel_notification_*` Prometheus series — `ingest_total`, `normalization_errors_total`, `dedupe_total`, `action_attempts_total`, `delivery_attempts_total`, and `processing_duration_ms` — so a `/metrics` scrape can alert on a stuck pipeline stage (e.g. a spike in `ingest_total{status="rejected"}` or `delivery_attempts_total{status="failure"}`) and attribute it to a `source_type`/`source_form`. No existing notification behavior changes.
- **Security reviewers:** Every metric label is a bounded enum or registered source/channel identifier — never raw notification payload, title, or body. SCN-054-024 ("observability must not leak secrets") is enforced by construction and proven by `TestNotificationMetricsDoNotLeakPayloadInLabels`.
- **End users:** Not applicable — the notification handler is internal infrastructure; this change adds operator-facing metrics only.

## Acceptance

- AC-01..AC-10 from `spec.md` all pass; full evidence captured in `report.md`.
- The Scope 8 DoD claim ("Metrics and traces expose source-qualified pipeline stages without leaking secrets") is now true with real wired emit sites.
- Per the task contract, this packet does NOT commit or push; the parent orchestrator validates and commits the 7 source/test files plus this packet directory.

## Sign-off

Stochastic-quality-sweep observation `OBS-054-15-001` (sweep `sweep-2026-06-06-obs`, trigger=`observation`, mapped child workflow=`bugfix-fastlane`, executionModel=`parent-expanded-child-mode`) terminates `completed_owned` for the BUG-054-002 packet. The 6 deferred design.md metrics are recorded for a future, separately-scoped observability fix.

## Checklist

- [x] AC-01: `go build ./...` exits 0 (report.md Test Evidence — `BUILD_EXIT=0`).
- [x] AC-02: `go vet ./internal/notification/... ./internal/metrics/...` exits 0 (report.md Test Evidence — `VET_EXIT=0`).
- [x] AC-03: `go test ./internal/metrics/... ./internal/notification/... -count=1 -v` all GREEN incl. 7 new tests (report.md Test Evidence — package `ok` lines).
- [x] AC-04: all 6 `smackerel_notification_*` families registered with bounded labels (`TestNotificationMetricFamiliesRegisteredWithBoundedLabels`).
- [x] AC-05: delivery emit RED-proven (`= 0, want 1`) then GREEN on restore (report.md Test Evidence).
- [x] AC-06: no metric label leaks payload content (`TestNotificationMetricsDoNotLeakPayloadInLabels`).
- [x] AC-07: BUG packet `artifact-lint.sh` returns PASSED (report.md Validation Evidence).
- [x] AC-08: BUG packet `state-transition-guard.sh` exits with 0 BLOCKs (report.md Validation Evidence).
- [x] AC-09: change set confined to the 7 source/test files + this packet directory; zero `.github/bubbles/**` framework files, no other spec (report.md Audit Evidence).
- [x] AC-10: the 6 deferred design.md metrics are NOT declared as dead metrics and are documented as deferred-with-rationale in bug.md + report.md.
- [x] Zero parent-spec planning artifact (`spec.md`/`design.md`/`scopes.md`/`state.json`/`scenario-manifest.json`/`report.md`/`uservalidation.md` of spec 054) is mutated by this packet.
