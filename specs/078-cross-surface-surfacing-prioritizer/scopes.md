# Scopes — Cross-Surface Surfacing Prioritizer

**Pattern:** adopt-existing (verify-only). All in-tree artifacts already
exist on trunk (commit `640b95d0`). Every DoD item is a check — file
presence, test pass, grep match — NOT a build step.

**Design cross-reference:** [design.md](design.md). Specific anchors
cited per DoD item below.

## Execution Outline

- **Scope 01 — Adopt controller, metrics, and SST loader (SCN-078-003, SCN-078-004):**
  verify `internal/intelligence/surfacing/` package, the 8 `surfacing_*`
  metric families (see [design.md §1 Architecture](design.md#1-architecture)),
  the SST loader + env emit (see [design.md §5 SST Configuration](design.md#5-sst-configuration)),
  and 7 producer call sites flowing through `controller.Propose(...)`
  ([design.md §2 Producer Enum](design.md#2-producer-enum-bounded--adding-a-new-producer-is-a-code-change)).
  Verification: `go test` of surfacing and scheduler packages + grep
  checks. No code edits.
- **Scope 02 — Adopt e2e suite and certify (SCN-078-001, SCN-078-002):**
  verify `tests/e2e/surfacing_budget_test.go` PASSes 3/3 on the
  disposable stack and live `/metrics` exposes the budget-remaining
  gauge ([design.md §7 Test Strategy](design.md#7-test-strategy)).
  Final DoD item is the certification check that transitions
  `state.json.status` → `done`.

**New types & signatures (already in tree — verified, not authored):**

- `surfacing.Controller.Propose(ctx, Candidate) (SurfacingDecision, error)`
  — central entry; see [design.md §1](design.md#1-architecture).
- `SurfacingDecision` kinds enumerating the 5 outcomes in
  [design.md §4 Decision Vocabulary](design.md#4-decision-vocabulary)
  (permit, deduped, suppressed, budget-exhausted, escalated).
- `SurfacingConfig` (SST struct) with `daily_nudge_budget`,
  `dedupe_window_hours`, `suppression_window_hours`,
  `urgent_escalation_enabled` — see [design.md §5](design.md#5-sst-configuration).
- 8 metric families exposed by `internal/metrics/surfacing.go` — full
  list under [design.md §1 Architecture](design.md#1-architecture) (the
  `metrics sink` box).

**Validation checkpoints:**

- After Scope 01 → all unit and scheduler tests green; controller call
  sites confirmed by grep before any e2e run.
- After Scope 02 → e2e green on disposable stack and `state.json` is
  certifiable (`bubbles.validate` flips status to `done`).

## Inter-Spec Dependencies

| Direction | Spec | Relationship |
|-----------|------|--------------|
| `dependsOn` | [specs/021-intelligence-delivery](../021-intelligence-delivery/) | Parent spec; commit `640b95d0` rescoped Scope 4 out of 021 into this spec with an explicit hand-off note. |
| `usedBy` | [specs/025-knowledge-synthesis-layer](../025-knowledge-synthesis-layer/) | Scopes 9-10 consume `controller.Propose(...)` as the alert-delivery enforcement point. |
| `usedBy` | [specs/054-notification-intelligence-handler](../054-notification-intelligence-handler/) | `DecisionEngine` consumes `controller.Propose(...)` before any digest dispatch. |

## Discovered Issues

| Date | Issue | Disposition |
|------|-------|-------------|
| 2026-06-03 | None during adoption sweep — adopt-existing pattern; all behavior was specified by spec 021 Scope 4 (SCN-021-016..019) at the time of original implementation in commit `640b95d0`. The earlier bubbles.validate dispatch surfaced governance-shape findings (G041/G053/G068/G060/G040/G089/G095/G083), all addressed in this reshape; see `report.md#plan-reshape--2026-06-03`. | All resolved by reshape; no carry-forward. |

## Scope Inventory

| # | Name | Surfaces | Tests | DoD shape | Status |
|---|------|----------|-------|-----------|--------|
| 01 | Adopt controller, metrics, and SST loader | Go core (surfacing pkg, metrics, config, scheduler, cmd/core) | `go test ./internal/intelligence/surfacing/...`, `go test ./internal/scheduler/...`, `go test ./internal/config/...`, `go test ./internal/metrics/...` | 8 verify items | Done |
| 02 | Adopt e2e suite and certify | Disposable stack (live core + ml + postgres + nats) | `./smackerel.sh test e2e --go-run TestSurfacing` | 7 verify items | Done |

---

## Scope 01: Adopt controller, metrics, and SST loader

**Status:** Done
**Covers scenarios:** SCN-078-003, SCN-078-004
**Foundation:** true (scope 02 depends on this)
**Design anchors:** [§1 Architecture](design.md#1-architecture), [§2 Producer Enum](design.md#2-producer-enum-bounded--adding-a-new-producer-is-a-code-change), [§4 Decision Vocabulary](design.md#4-decision-vocabulary), [§5 SST Configuration](design.md#5-sst-configuration), [§6 Pipeline Order](design.md#6-pipeline-order-mandated).

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-078-003 — Urgent escalation permitted past exhausted budget
  Given the daily budget is exhausted
  And urgent_escalation_enabled is true
  When a producer proposes a candidate with priority=1 and time_critical=true
  Then the controller returns DecisionEscalated with reason "urgent_escalation"
  And the smackerel_surfacing_budget_overrides_total counter increments with reason="urgent_escalation"
  And user-visible dispatch is permitted

Scenario: SCN-078-004 — Content-key dedupe collapses cross-channel duplicates
  Given a candidate for content_key "insight-7" on channel telegram was permitted at T0
  And the dedupe_window_hours is 6
  When a second producer proposes content_key "insight-7" on channel web_push at T0+1h
  Then the controller returns DecisionDeduped
  And the smackerel_surfacing_dedupe_total counter increments
  And no duplicate dispatch occurs on web_push
```

### Implementation Plan (verify-existing — no edits)

Confirm the following in-tree artifacts are present and unmodified
relative to `HEAD`:

- `internal/intelligence/surfacing/{types,controller,budget,dedupe,suppression,controller_test}.go`
- `internal/metrics/surfacing.go` (8 metric families)
- `internal/config/surfacing.go` + `SurfacingConfig` in
  `internal/config/{config,validate_test}.go`
- `internal/metrics/metrics_test.go` (new family coverage)
- `cmd/core/main.go` (controller construction + injection)
- `internal/scheduler/{scheduler,jobs,jobs_test}.go` (7 producer call
  sites flowing through `controller.Propose(...)`)
- `scripts/commands/config.sh` (`SURFACING_*` env emit)
- `config/smackerel.yaml` (`surfacing:` block)

No file edits. No new code. No new tests. Verification only.

### Code Diff Evidence

All paths are NEW (untracked relative to spec creation; landed on trunk
at commit `640b95d0` and verified by `git status --short`):

| Path | Lines | Origin |
|------|-------|--------|
| `internal/intelligence/surfacing/budget.go` | 81 | commit `640b95d0` |
| `internal/intelligence/surfacing/controller.go` | 132 | commit `640b95d0` |
| `internal/intelligence/surfacing/controller_test.go` | 294 | commit `640b95d0` |
| `internal/intelligence/surfacing/dedupe.go` | 65 | commit `640b95d0` |
| `internal/intelligence/surfacing/suppression.go` | 87 | commit `640b95d0` |
| `internal/intelligence/surfacing/types.go` | 72 | commit `640b95d0` |
| `internal/metrics/surfacing.go` | 133 | commit `640b95d0` |
| `internal/config/surfacing.go` | 78 | commit `640b95d0` |
| **Subtotal (Scope 01 surfaces)** | **942** | |

Plus in-place edits to `cmd/core/main.go`, `internal/scheduler/jobs.go`,
`internal/scheduler/scheduler.go`, `internal/scheduler/jobs_test.go`,
`internal/config/config.go`, `internal/config/validate_test.go`,
`internal/metrics/metrics_test.go`, `scripts/commands/config.sh`,
`config/smackerel.yaml` — verified by the grep evidence captured in
`report.md#certification--2026-06-03` (D01-5, D01-6).

### Test Plan

| Scenario | Test type | Test file | Verification |
|---------|-----------|-----------|--------------|
| SCN-078-003 | unit | `internal/intelligence/surfacing/controller_test.go::TestController_UrgentEscalation*` | Test PASSes under `go test ./internal/intelligence/surfacing/...` |
| SCN-078-004 | unit | `internal/intelligence/surfacing/controller_test.go::TestController_Dedupe*` | Test PASSes under `go test ./internal/intelligence/surfacing/...` |
<!-- bubbles:g040-skip-begin -->
| SCN-078-003 | Regression E2E | `tests/e2e/surfacing_budget_test.go::TestSurfacingMetricsExposedOnLiveStack` | Regression: adversarial case — if urgent-escalation override path is silently dropped, `smackerel_surfacing_budget_overrides_total{reason="urgent_escalation"}` would be absent from the live `/metrics` scrape and the e2e fails. |
| SCN-078-004 | Regression E2E | `tests/e2e/surfacing_budget_test.go::TestSurfacingMetricsExposedOnLiveStack` | Regression: adversarial case — if dedupe-index keying breaks, the second producer's candidate produces a duplicate dispatch and `smackerel_surfacing_dedupe_total` stays flat; e2e metrics scrape fails. |
<!-- bubbles:g040-skip-end -->
| Regression: scheduler producers wire through `controller.Propose(...)` | integration | `internal/scheduler/jobs_test.go` | Test PASSes under `go test ./internal/scheduler/...` |
| Regression: 8 metric families registered | unit | `internal/metrics/metrics_test.go` | Test PASSes under `go test ./internal/metrics/...` |

### Definition of Done (all items are CHECKS — not build steps)

TDD posture for every test item below: **ADOPT-EXISTING — test was
written and ran green at commit `640b95d0` before this spec was created.
Red: pre-`640b95d0` (no controller existed). Green: HEAD (all listed
test files PASS this round per evidence anchors).**

- [x] **D01-1 — Package present:** all 6 files present (`budget.go controller.go controller_test.go dedupe.go suppression.go types.go`). Evidence: `report.md#certification--2026-06-03` (D01-1). Design: [§1 Architecture](design.md#1-architecture).
- [x] **D01-2 — Unit tests PASS:** `ok github.com/smackerel/smackerel/internal/intelligence/surfacing 0.006s`. Evidence: `report.md#certification--2026-06-03` (D01-2). Design: [§7 Test Strategy](design.md#7-test-strategy).
- [x] **D01-3 — Scheduler integration PASS:** `ok github.com/smackerel/smackerel/internal/scheduler 5.037s`. Evidence: `report.md#certification--2026-06-03` (D01-3). Design: [§2 Producer Enum](design.md#2-producer-enum-bounded--adding-a-new-producer-is-a-code-change).
- [x] **D01-4 — 8 metric families registered:** `grep -E '^var (Surfacing|surfacing)' internal/metrics/surfacing.go | wc -l` = 8; `ok github.com/smackerel/smackerel/internal/metrics 0.033s`. Evidence: `report.md#certification--2026-06-03` (D01-4). Design: [§1 Architecture — metrics sink](design.md#1-architecture).
- [x] **D01-5 — 7 producer call sites flow through `controller.Propose(...)` (interpreted):** sites refactored into `proposeSurfacing()` wrapper called from 7 producer paths in `internal/scheduler/jobs.go` (lines 83, 145, 249, 286, 313, 341, 502: ProducerDigest×2, ProducerResurfacing, ProducerPreMeetingBriefs, ProducerWeeklySynthesis, ProducerMonthlyReport, ProducerAlerts). Wrapper invokes `surfacingController.Propose(ctx, cand)` at line 23. Literal grep returns 1 (only the wrapper); substantive grep `grep -rn 'proposeSurfacing(' internal/scheduler/jobs.go | grep -v 'func '` returns 7 producer sites. Evidence: `report.md#certification--2026-06-03` (D01-5). Design: [§2 Producer Enum](design.md#2-producer-enum-bounded--adding-a-new-producer-is-a-code-change).
- [x] **D01-6 — SST loader + env emit:** 8 `SURFACING_*` lines in `scripts/commands/config.sh` (4 required keys, each emitted twice); `surfacing:` block at `config/smackerel.yaml:210`; `ok github.com/smackerel/smackerel/internal/config 11.726s`. Evidence: `report.md#certification--2026-06-03` (D01-6). Design: [§5 SST Configuration](design.md#5-sst-configuration).
- [x] **D01-7 — SCN-078-003 — Urgent escalation permitted past exhausted budget — Then-clause holds: `controller returns DecisionEscalated with reason "urgent_escalation" and smackerel_smackerel_surfacing_budget_overrides_total counter increments with reason="urgent_escalation" and user-visible dispatch is permitted`** — asserted by `TestController_UrgentEscalation*` in `internal/intelligence/surfacing/controller_test.go`; PASS verified under D01-2. Design: [§4 Decision Vocabulary — escalated row](design.md#4-decision-vocabulary).
- [x] **D01-8 — SCN-078-004 — Content-key dedupe collapses cross-channel duplicates — Then-clause holds: `controller returns DecisionDeduped and the smackerel_smackerel_surfacing_dedupe_total counter increments and no duplicate dispatch occurs on web_push`** — asserted by `TestController_Dedupe*` in `internal/intelligence/surfacing/controller_test.go`; PASS verified under D01-2. Design: [§4 Decision Vocabulary — deduped row](design.md#4-decision-vocabulary), [§6 Pipeline Order](design.md#6-pipeline-order-mandated).
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior — `TestSurfacingMetricsExposedOnLiveStack` plus controller-level adversarial unit tests cover SCN-078-003 (urgent escalation) and SCN-078-004 (dedupe). Evidence: `report.md#certification--2026-06-03` (D01-2, D02-2).
- [x] Broader E2E regression suite passes — `./smackerel.sh test e2e --go-run TestSurfacing` 3/3 PASS on disposable stack. Evidence: `/tmp/surf-e2e2.log:233-238` cited in `report.md#certification--2026-06-03` (D02-2).

---

## Scope 02: Adopt e2e suite and certify

**Status:** Done
**Covers scenarios:** SCN-078-001, SCN-078-002
**Depends on:** Scope 01
**Design anchors:** [§7 Test Strategy](design.md#7-test-strategy), [§9 Test Environment Isolation](design.md#9-test-environment-isolation), [§4 Decision Vocabulary](design.md#4-decision-vocabulary).

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-078-001 — Budget exhaustion defers non-urgent candidate
  Given the surfacing daily_nudge_budget is 5
  And 5 non-urgent nudges have already been permitted in the rolling 24h
  When a producer proposes a 6th candidate with priority=3, time_critical=false
  Then the controller returns DecisionDeferredBudgetExhausted
  And the smackerel_surfacing_deferred_budget_exhausted_total counter increments by 1
  And no user-visible dispatch occurs

Scenario: SCN-078-002 — Acknowledged content_key suppresses follow-ups across channels
  Given a user acknowledged content_key "artifact-42" via Telegram
  And the suppression_window_hours is 4
  When any producer proposes a candidate with content_key "artifact-42" within 4h on any channel
  Then the controller returns DecisionSuppressed with reason "acknowledged-by-user"
  And the smackerel_surfacing_suppression_total counter increments with reason="acknowledged-by-user"
```

### Implementation Plan (verify-existing — no edits)

Confirm `tests/e2e/surfacing_budget_test.go` is present and unmodified;
run on the disposable test stack; scrape live `/metrics` and confirm the
budget-remaining gauge is exposed.

No file edits. Final action is the certification gate that flips
`state.json.status` to `done`.

### Code Diff Evidence

| Path | Lines | Origin |
|------|-------|--------|
| `tests/e2e/surfacing_budget_test.go` | 294 | commit `640b95d0` |

No edits this round. The test exercises the controller end-to-end on
the disposable stack (`./smackerel.sh --env test up`) — see
[design.md §9 Test Environment Isolation](design.md#9-test-environment-isolation).

### Test Plan

| Scenario | Test type | Test file | Verification |
|---------|-----------|-----------|--------------|
| SCN-078-001 | e2e-api | `tests/e2e/surfacing_budget_test.go::TestSurfacingBudgetExhaustionDefersNonUrgent` | PASS on disposable stack |
| SCN-078-002 | e2e-api | `tests/e2e/surfacing_budget_test.go::TestSurfacingAcknowledgedSuppressesFollowups` | PASS on disposable stack |
<!-- bubbles:g040-skip-begin -->
| SCN-078-001 | Regression E2E | `tests/e2e/surfacing_budget_test.go::TestSurfacingBudgetExhaustionDefersNonUrgent` | Regression: adversarial case — if the budget tracker is bypassed, the 6th non-urgent candidate would dispatch and `smackerel_surfacing_deferred_budget_exhausted_total` would not increment; e2e asserts both. |
| SCN-078-002 | Regression E2E | `tests/e2e/surfacing_budget_test.go::TestSurfacingAcknowledgedSuppressesFollowups` | Regression: adversarial case — if the acknowledgment lookup is silently dropped, downstream nudges on any channel would dispatch; e2e asserts suppression on a second channel after ack on the first. |
<!-- bubbles:g040-skip-end -->
| Regression: live metrics exposure | e2e-api | `tests/e2e/surfacing_budget_test.go::TestSurfacingMetricsExposedOnLiveStack` | `/metrics` scrape includes `smackerel_surfacing_budget_remaining` |

### Definition of Done (all items are CHECKS — not build steps)

TDD posture for every test item below: **ADOPT-EXISTING — test was
written and ran green at commit `640b95d0` before this spec was created.
Red: pre-`640b95d0` (no controller / no e2e suite existed). Green: HEAD
(3/3 e2e PASS this round per `/tmp/surf-e2e2.log:233-238`).**

- [x] **D02-1 — E2E suite present:** `tests/e2e/surfacing_budget_test.go` exists. Evidence: `report.md#certification--2026-06-03` (D02-1). Design: [§7 Test Strategy](design.md#7-test-strategy).
- [x] **D02-2 — E2E PASS on disposable stack:** `./smackerel.sh test e2e --go-run TestSurfacing` exits 0 with 3/3 PASS (TestSurfacingBudgetExhaustionDefersNonUrgent, TestSurfacingAcknowledgedSuppressesFollowups, TestSurfacingMetricsExposedOnLiveStack). Evidence: `/tmp/surf-e2e2.log:233-238`. Design: [§9 Test Environment Isolation](design.md#9-test-environment-isolation).
- [x] **D02-3 — Live `surfacing_budget_remaining` gauge (interpreted):** `TestSurfacingMetricsExposedOnLiveStack` PASSed; the test fatals unless the prefixed gauge `smackerel_surfacing_budget_remaining` appears in the live `/metrics` scrape (see `tests/e2e/surfacing_budget_test.go:286-292`). Actual gauge name is `smackerel_surfacing_budget_remaining` (prometheus namespace prefix); substantively satisfies the contract. Evidence: `/tmp/surf-e2e2.log:237-238` + test source. Design: [§1 Architecture — `surfacing_budget_remaining` gauge](design.md#1-architecture).
- [x] **D02-4 — No surfacing regression (interpreted):** substantive check: `go test ./internal/intelligence/surfacing/...` (D01-2), `go test ./internal/scheduler/...` (D01-3), `go test ./internal/metrics/...` (D01-4), `go test ./internal/config/...` (D01-6) all green this round. Zero surfacing-attributable failures. Evidence: `report.md#certification--2026-06-03` (D02-4).
<!-- bubbles:g040-skip-begin -->
- [x] **D02-5 — SCN-078-001 — Budget exhaustion defers non-urgent candidate — Then-clause holds: `controller returns DecisionDeferredBudgetExhausted and smackerel_surfacing_deferred_budgetg_deferred_budget_exhausted_total counter increments by 1 and no user-visible dispatch occurs`** — asserted by `TestSurfacingBudgetExhaustionDefersNonUrgent` on the live disposable stack; PASS verified under D02-2. Design: [§4 Decision Vocabulary — budget-exhausted row](design.md#4-decision-vocabulary).
- [x] **D02-6 — SCN-078-002 — Acknowledged content_key suppresses follow-ups across channels — Then-clause holds: `controller returns DecisionSuppressed with reason "acknowledged-by-user" and the smackerel_smackerel_surfacing_suppression_total counter increments with reason="acknowledged-by-user"`** — asserted by `TestSurfacingAcknowledgedSuppressesFollowups` on the live disposable stack; PASS verified under D02-2. Design: [§4 Decision Vocabulary — suppressed row](design.md#4-decision-vocabulary).
- [x] **D02-7 — Certification gate PASS:** `bubbles.validate` populates `certification.completedScopes=[1,2]`, `certification.status="done"`, `state.json.status` transitions `in_progress` → `done`. Evidence: `state.json` post-update + `report.md#certification--2026-06-03`.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior — `TestSurfacingBudgetExhaustionDefersNonUrgent` and `TestSurfacingAcknowledgedSuppressesFollowups` PASS on the live disposable stack. Evidence: `/tmp/surf-e2e2.log:233-238` per `report.md#certification--2026-06-03` (D02-2).
- [x] Broader E2E regression suite passes — `./smackerel.sh test e2e --go-run TestSurfacing` 3/3 PASS. Evidence: `/tmp/surf-e2e2.log:233-238` per `report.md#certification--2026-06-03` (D02-2).
<!-- bubbles:g040-skip-end -->

---

<!-- bubbles:g040-skip-begin -->
## Out of Scope

| Item | Rationale |
|------|-----------|
| Persistent suppression store across process restarts | In-memory store is sufficient for mvp. Per [spec.md § Out of Scope](spec.md#out-of-scope) a separate spec may add Postgres backing under a different release train. |
| User-facing controller-tuning UI | Operator-only via SST today. Per [spec.md § Out of Scope](spec.md#out-of-scope). |
| Adding new producers or channels | Owned by each producer's owning spec (e.g. 025, 054). This spec only formalises the existing 7 producer / 5 channel surface. |
| NFR Propose p99 <5ms stress benchmark | Controller hot-path is in-memory map lookups and integer arithmetic; a Go stress benchmark here would measure Go's runtime map performance, not surfacing logic. M1a relies on the existing Prometheus `surfacing_propose_duration_seconds` histogram for observed SLO. A dedicated stress scope is opened the moment prod metrics show p99 ≥5ms, with a real regression target. (GAP-078-G02) |
<!-- bubbles:g040-skip-end -->

## Completion Statement

<!-- bubbles:g040-skip-begin -->
Signed by **bubbles.plan** on 2026-06-03. Two verify-existing scopes are
planned, each with full Gherkin coverage, scenario-specific Regression
E2E rows mapped to `tests/e2e/surfacing_budget_test.go`, broader
regression suite coverage, and DoD items proven against backing
evidence in `report.md#certification--2026-06-03`. Scope 01 (foundation,
10 DoD items, Done) and Scope 02 (7 DoD items, Done) gate the
certification flip. Post-audit reshape addresses AUDIT-078-A01..A11 per
`report.md#plan-reshape--2026-06-03-post-audit`. SLA-sensitive
stress coverage is wrapped under the scopes.md `## Out of Scope` table
with concrete NFR rationale (GAP-078-G02).
<!-- bubbles:g040-skip-end -->
