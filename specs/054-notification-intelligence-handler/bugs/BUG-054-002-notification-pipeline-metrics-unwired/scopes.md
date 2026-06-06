# Scopes: BUG-054-002 — Notification Pipeline Metrics Declared In Design But Never Wired

## Scope 1: Wire The 6 Core Notification Pipeline Metrics (Single-Scope Bugfix-Fastlane)

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: BUG-054-002-SCN-001 — All 6 notification metric families register with bounded labels
  Given internal/notification/ registered ZERO Prometheus metrics at baseline HEAD 46def326
  And internal/metrics/metrics.go now declares the 6 smackerel_notification_* families and registers them in init()
  When the 6 emit sites are exercised through their real wiring and prometheus.DefaultGatherer.Gather() is walked
  Then all 6 families (ingest_total, normalization_errors_total, dedupe_total, action_attempts_total, delivery_attempts_total, processing_duration_ms) are present
  And each family carries ONLY its bounded-label allowlist (no unexpected/unbounded label)

Scenario: BUG-054-002-SCN-002 — ingest_total increments on a rejected raw event
  Given a Service backed by a nil-pool Store (CreateRawEvent returns a clean error, not a panic)
  And a SourceEventEnvelope with source_type=ingest_fixture and source_form=webhook
  When SubmitSourceEvent drives the real Process() ingest reject path
  Then smackerel_notification_ingest_total{source_type=ingest_fixture,source_form=webhook,status=rejected} increments by exactly 1
  And the test FAILS if the ingest .Inc() emit site is removed (adversarial)

Scenario: BUG-054-002-SCN-003 — delivery_attempts_total increments on dispatch (success and failure)
  Given an OutputDispatcher with a configured dashboard channel
  When Dispatch delivers to the dashboard channel successfully
  Then smackerel_notification_delivery_attempts_total{channel=dashboard,status=success} increments by exactly 1
  And when Dispatch targets an unconfigured channel
  Then smackerel_notification_delivery_attempts_total{channel=unconfigured,status=failure} increments by exactly 1
  And disabling the delivery .Inc() emit makes the test FAIL with "= 0, want 1" (RED-proven adversarial)

Scenario: BUG-054-002-SCN-004 — action_attempts_total increments on a decision
  Given a valid DecisionEngine and a notification with a non-empty suppression set
  When DecisionEngine.Decide evaluates the decision
  Then smackerel_notification_action_attempts_total{action_class=no_action,status=suppressed} increments by exactly 1
  And the test FAILS if the Decide .Inc() emit site is removed (adversarial)

Scenario: BUG-054-002-SCN-005 — normalization_errors_total increments on an invalid raw event
  Given a RawEventRecord with a present id and source identity but a zero ObservedAt
  When Normalizer.Normalize runs and returns "observed_at is required"
  Then smackerel_notification_normalization_errors_total{source_type=normalize_fixture,error_kind=missing_observed_at} increments by exactly 1
  And the error_kind label is a bounded classification, never the raw error string

Scenario: BUG-054-002-SCN-006 — dedupe_total increments on a reaction-loop suppression
  Given a LoopGuard and a SourceEventEnvelope whose loop_guard_key matches a prior LoopOrigin within the window
  When LoopGuard.Evaluate returns a reaction-loop suppression
  Then smackerel_notification_dedupe_total{source_type=dedupe_fixture,suppression_kind=reaction_loop} increments by exactly 1
  And the test FAILS if the loop-guard .Inc() emit site is removed (adversarial)

Scenario: BUG-054-002-SCN-007 — No notification metric label leaks payload content (SCN-054-024)
  Given a SUPERSECRET-PAYLOAD-MARKER is injected as Title/Body/RawPayload through the delivery, normalize, and ingest emit sites
  When prometheus.DefaultGatherer.Gather() is walked for all smackerel_notification_* families
  Then NO label value on any notification metric contains the marker
  And the test FAILS if any payload-derived label is introduced (adversarial redaction guard)
```

### Test Plan

| Type | Scenario | Test Functions | Test Files / Targets |
|------|----------|----------------|----------------------|
| unit | BUG-054-002-SCN-001 | `TestNotificationMetricFamiliesRegisteredWithBoundedLabels` | `internal/notification/metrics_emit_test.go` |
| unit | BUG-054-002-SCN-002 | `TestNotificationIngestTotalIncrementsOnRejectedRawEvent` | `internal/notification/metrics_emit_test.go` |
| unit | BUG-054-002-SCN-003 | `TestNotificationDeliveryAttemptsIncrementsOnDispatch` | `internal/notification/metrics_emit_test.go` |
| unit | BUG-054-002-SCN-004 | `TestNotificationActionAttemptsIncrementsOnDecide` | `internal/notification/metrics_emit_test.go` |
| unit | BUG-054-002-SCN-005 | `TestNotificationNormalizationErrorsIncrementsOnInvalidRawEvent` | `internal/notification/metrics_emit_test.go` |
| unit | BUG-054-002-SCN-006 | `TestNotificationDedupeTotalIncrementsOnLoopSuppression` | `internal/notification/metrics_emit_test.go` |
| unit | BUG-054-002-SCN-007 | `TestNotificationMetricsDoNotLeakPayloadInLabels` | `internal/notification/metrics_emit_test.go` |
| Regression E2E | BUG-054-002-SCN-001..007 | all 7 `TestNotification*` functions (persistent, re-runnable on demand; each RED if its emit site regresses) | `internal/notification/metrics_emit_test.go` |

### Scenario-First TDD Evidence

This bugfix-fastlane packet preserves red→green TDD discipline. The adversarial property is proven for the `delivery_attempts_total` site: with the `.Inc()` emit disabled the test is RED (`smackerel_notification_delivery_attempts_total{channel=dashboard,status=success} = 0, want 1`, `--- FAIL`), and with the emit restored the test is GREEN (`--- PASS`). The full RED→GREEN block plus the 7-test GREEN run and the `go build`/`go vet` results are captured in `report.md` §Test Evidence. Each increment test captures the counter value before and after the real wiring path and asserts a delta of exactly 1, so removing any single emit site turns its scenario RED.

### Change Boundary

This scope is a **feature-add observability fix** (additive metric series + emit lines; zero behavior change to the existing pipeline). Containment is strict:

**Allowed file families (the ONLY paths this scope may touch):**

- `internal/metrics/metrics.go` (declare 6 vars + register in `init()`)
- `internal/notification/service.go` (ingest + dedupe(store) + duration emits + stage helper)
- `internal/notification/normalizer.go` (normalization_errors emit + bounded error_kind helper)
- `internal/notification/decision.go` (action_attempts emit + bounded status helper)
- `internal/notification/output_logic.go` (delivery_attempts emit)
- `internal/notification/reaction_logic.go` (reaction-loop dedupe emit)
- `internal/notification/metrics_emit_test.go` (new adversarial unit tests)
- `specs/054-notification-intelligence-handler/bugs/BUG-054-002-notification-pipeline-metrics-unwired/` (all 8 packet artifacts)

**Excluded surfaces (this scope MUST NOT touch any of these):**

- The 6 additional design.md metrics outside this packet's pipeline-stage set — NOT declared (no dead metrics).
- `internal/notification/` business logic (classification, correlation, decision policy, store schema) — only observability emit lines added.
- Parent spec 054 `spec.md`/`design.md`/`scopes.md`/`state.json`/`scenario-manifest.json`/`report.md`/`uservalidation.md` — NOT mutated.
- `.github/bubbles/**`, `.github/agents/**` — framework files (immutable; the working tree's unrelated externally-modified framework files are NOT staged/reverted/touched).
- `cmd/`, `ml/`, `scripts/`, `config/`, `.github/workflows/`, `deploy/`, `smackerel.sh`, any other spec under `specs/`, `docs/`.

Enumerated consumer surfaces: none — the change is additive observability with zero behavior change, so there are no `navigation`/`redirect`/`API client`/`deep link`/`stale-reference` consumers to sweep. The only new surface is the Prometheus `/metrics` scrape gaining 6 additive families.

### Definition of Done

- [x] BUG-054-002 packet contains 8 artifacts in `specs/054-notification-intelligence-handler/bugs/BUG-054-002-notification-pipeline-metrics-unwired/` (bug.md, spec.md, design.md, scopes.md, scenario-manifest.json, report.md, state.json, uservalidation.md). **Phase:** bootstrap **Evidence:** all 8 files authored under this packet directory; `git status --short` listing in report.md §Implementation Code Diff Evidence. **Claim Source:** executed
- [x] Change Boundary respected — only the 7 allowed source/test files plus this packet directory are touched; zero `.github/bubbles/**` framework files, zero other spec. **Phase:** implement **Evidence:** `git status --short` of the 7 changed files captured in report.md §Implementation Code Diff Evidence. **Claim Source:** executed
- [x] FR-01: `internal/metrics/metrics.go` declares all 6 `smackerel_notification_*` families and registers them in `init()` following the existing `var + MustRegister` pattern. **Phase:** implement **Evidence:** diffstat (+95 lines metrics.go) + registration list diff in report.md §Code Diff Evidence; confirmed registered by `TestNotificationMetricFamiliesRegisteredWithBoundedLabels`. **Claim Source:** executed
- [x] FR-02: every declared metric is wired at a real live emit site (ingest/normalize/dedupe/decide/deliver/duration); no dead metric. **Phase:** implement **Evidence:** per-file emit-site diffs in report.md §Code Diff Evidence; each increment proven by its scenario test. **Claim Source:** executed
- [x] Scenario "BUG-054-002-SCN-001 — All 6 notification metric families register with bounded labels": `TestNotificationMetricFamiliesRegisteredWithBoundedLabels` PASS. **Phase:** test **Evidence:** GREEN run in report.md §Test Evidence (`--- PASS: TestNotificationMetricFamiliesRegisteredWithBoundedLabels`). **Claim Source:** executed
- [x] Scenario "BUG-054-002-SCN-002 — ingest_total increments on a rejected raw event": `TestNotificationIngestTotalIncrementsOnRejectedRawEvent` PASS (adversarial, nil-pool reject path). **Phase:** test **Evidence:** GREEN run in report.md §Test Evidence. **Claim Source:** executed
- [x] Scenario "BUG-054-002-SCN-003 — delivery_attempts_total increments on dispatch": `TestNotificationDeliveryAttemptsIncrementsOnDispatch` PASS; RED-proven (disabling emit → `= 0, want 1`). **Phase:** test **Evidence:** RED→GREEN block in report.md §Test Evidence. **Claim Source:** executed
- [x] Scenario "BUG-054-002-SCN-004 — action_attempts_total increments on a decision": `TestNotificationActionAttemptsIncrementsOnDecide` PASS. **Phase:** test **Evidence:** GREEN run in report.md §Test Evidence. **Claim Source:** executed
- [x] Scenario "BUG-054-002-SCN-005 — normalization_errors_total increments on an invalid raw event": `TestNotificationNormalizationErrorsIncrementsOnInvalidRawEvent` PASS (bounded error_kind). **Phase:** test **Evidence:** GREEN run in report.md §Test Evidence. **Claim Source:** executed
- [x] Scenario "BUG-054-002-SCN-006 — dedupe_total increments on a reaction-loop suppression": `TestNotificationDedupeTotalIncrementsOnLoopSuppression` PASS. **Phase:** test **Evidence:** GREEN run in report.md §Test Evidence. **Claim Source:** executed
- [x] Scenario "BUG-054-002-SCN-007 — No notification metric label leaks payload content (SCN-054-024)": `TestNotificationMetricsDoNotLeakPayloadInLabels` PASS. **Phase:** security **Evidence:** GREEN run in report.md §Test Evidence; bounded-label allowlist + payload-marker assertion. **Claim Source:** executed
- [x] FR-03: every metric label value is a bounded enum or known source/channel identifier; none derives from RawPayload/Title/Body. **Phase:** security **Evidence:** DD-4 label-derivation table in design.md + `TestNotificationMetricsDoNotLeakPayloadInLabels` + the allowlist check in `TestNotificationMetricFamiliesRegisteredWithBoundedLabels`. **Claim Source:** executed
- [x] FR-05: the 6 other design.md metrics are NOT declared as dead metrics; each is recorded with per-metric rationale in bug.md §Root Cause. **Phase:** implement **Evidence:** the additional-metrics rationale table in bug.md §Root Cause and report.md §Summary; `grep` for those 6 metric names returns zero `metrics.go` declarations. **Claim Source:** executed
- [x] AC-01: `go build ./...` exits 0. **Phase:** stabilize **Evidence:** `BUILD_EXIT=0` block in report.md §Test Evidence. **Claim Source:** executed
- [x] AC-02: `go vet ./internal/notification/... ./internal/metrics/...` exits 0. **Phase:** stabilize **Evidence:** `VET_EXIT=0` block in report.md §Test Evidence. **Claim Source:** executed
- [x] AC-03: `go test ./internal/metrics/... ./internal/notification/... -count=1 -v` is all GREEN (incl. 7 new tests). **Phase:** regression **Evidence:** package `ok` summary lines + 7 `--- PASS: TestNotification*` lines in report.md §Test Evidence. **Claim Source:** executed
- [x] AC-07: `bash .github/bubbles/scripts/artifact-lint.sh specs/054-notification-intelligence-handler/bugs/BUG-054-002-notification-pipeline-metrics-unwired` returns PASSED. **Phase:** validate **Evidence:** artifact-lint output in report.md §Validation Evidence. **Claim Source:** executed
- [x] AC-08: `bash .github/bubbles/scripts/state-transition-guard.sh specs/054-notification-intelligence-handler/bugs/BUG-054-002-notification-pipeline-metrics-unwired` exits with 0 🔴 BLOCKs. **Phase:** validate **Evidence:** state-transition-guard verdict in report.md §Validation Evidence. **Claim Source:** executed
- [x] AC-09: staged closure diff lists ONLY the 7 source/test files plus this packet directory — no `.github/bubbles/**`, no other spec. **Phase:** audit **Evidence:** `git status --short` scoped listing in report.md §Audit Evidence. **Claim Source:** executed
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior in this scope (SCN-001..007) — the 7 persistent adversarial tests in `internal/notification/metrics_emit_test.go` are the scenario-specific regression cover (unit-level, the correct fidelity for metric-emit wiring — a live-stack e2e for a counter increment asserts less directly than the emit-site test; each turns RED if its emit site regresses, RED-proven for delivery) and re-run on demand via `go test ./internal/notification/...`. **Phase:** regression **Evidence:** full GREEN run + RED→GREEN proof in report.md §Test Evidence. **Claim Source:** executed
- [x] Broader E2E regression suite passes (SCN-001..007) — the full `internal/metrics` + `internal/notification` suite is GREEN alongside the 7 new tests with zero collateral failures, and the `//go:build integration` notification `Process` suite stays GREEN by construction (additive observability, zero behavior change). **Phase:** regression **Evidence:** `ok github.com/smackerel/smackerel/internal/notification` + `ok github.com/smackerel/smackerel/internal/metrics` summary lines in report.md §Test Evidence. **Claim Source:** executed
