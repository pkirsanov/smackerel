# Cross-Surface Surfacing Prioritizer

**Status:** Adopt-existing (in-tree groundwork formalization)
**Release Train:** mvp
**Workflow Mode:** improve-existing

## Summary

Formally own the unified **surfacing controller** infrastructure currently
present on trunk at `internal/intelligence/surfacing/` as orphan groundwork.
The controller is the single decision point between intelligence
**producers** (alerts, digest, resurfacing, weekly synthesis, monthly
report, pre-meeting briefs, frequent lookups) and user-visible **channels**
(Telegram, web push, ntfy, email-out, digest) and enforces three product
invariants: a per-day nudge **budget**, cross-channel **content-key
dedupe**, and post-acknowledgement **suppression**.

This spec adopts code that already exists, is wired into `cmd/core/main.go`
and `internal/scheduler/`, and has 3 passing e2e tests
(`tests/e2e/surfacing_budget_test.go`). Commit `640b95d0` rescoped the
work out of spec 021 with an explicit hand-off note naming this spec.

## Release Train

- **Train:** `mvp` (default-on)
- **`next` train:** controller stays compiled in but the surfacing config
  block keeps `urgent_escalation_enabled` decided per-bundle via the
  existing SST loader; the controller introduces NO new feature flag, so
  `flagsIntroduced: []`.
- Default-off behavior on other trains: not applicable — there is no flag
  to default-off. The controller is structural infrastructure that every
  train that ships intelligence producers MUST route through.

## Why Now

1. Commit `640b95d0` left this code in-tree without a spec owner.
   Artifact-ownership policy requires a feature folder before any further
   changes can be reviewed against a Definition of Done.
2. Coordination with spec 025 (alert delivery sweep) and spec 054 (digest
   producer) is now blocked on a stable Propose() contract; this spec
   pins that contract.
3. Product principles 6 (invisible-by-default, <3 nudges/week), 7
   (small/frequent/actionable), and 9 (design-for-restart) all require an
   enforcement layer. This is that layer.

## Actors

| Actor | Description | Interaction |
|-------|-------------|-------------|
| Intelligence Producer (system) | One of 7 in-process producers proposing a candidate | Calls `controller.Propose(ctx, candidate)` and respects the returned `SurfacingDecision` |
| End User | Recipient of nudges/digests | Observes the budget/dedupe/suppression behavior; never interacts with the controller directly |
| Operator | Self-host owner | Tunes the four `surfacing:` SST keys; reads `surfacing_*` Prometheus metrics |

## Outcome Contract

**Intent:** Every user-visible surfacing decision in Smackerel flows
through one controller that honors a per-day budget, deduplicates on
content key across channels, suppresses follow-ups for items the user
already acted on, and lets urgent items escalate past an exhausted budget
when policy allows.

**Success Signal:** All 7 producers call `controller.Propose(...)` before
dispatch; the controller emits the 8 `surfacing_*` Prometheus metric
families; the 3 e2e tests in `tests/e2e/surfacing_budget_test.go` pass on
the disposable test stack; no producer can bypass the controller without
breaking compilation.

**Hard Constraints:**
- Nudge budget is per-rolling-24h and global across channels.
- Dedupe key is `(content_key, dedupe_window_hours)` — same key collapses
  across Telegram, web push, ntfy, and email-out.
- Suppression is per-`content_key` for `suppression_window_hours` after
  the user acknowledges an item.
- Urgent escalation requires BOTH `priority == 1` AND
  `time_critical == true` AND `urgent_escalation_enabled == true`.
- Controller is in-process and synchronous; no NATS hop introduced.

**Failure Condition:** A producer dispatches a nudge without consulting
the controller, or two channels deliver the same `content_key` within the
dedupe window, or an acknowledged item generates a follow-up inside the
suppression window.

## Business Scenarios

### SCN-078-001 — Budget exhaustion defers non-urgent candidate

```gherkin
Given the surfacing daily_nudge_budget is 5
And 5 non-urgent nudges have already been permitted in the rolling 24h
When a producer proposes a 6th candidate with priority=3, time_critical=false
Then the controller returns DecisionDeferredBudgetExhausted
And the smackerel_surfacing_deferred_budget_exhausted_total counter increments by 1
And no user-visible dispatch occurs
```

### SCN-078-002 — Acknowledged content_key suppresses follow-ups across channels

```gherkin
Given a user acknowledged content_key "artifact-42" via Telegram
And the suppression_window_hours is 4
When any producer proposes a candidate with content_key "artifact-42" within 4h on any channel
Then the controller returns DecisionSuppressed with reason "acknowledged-by-user"
And the smackerel_surfacing_suppression_total counter increments with reason="acknowledged-by-user"
```

### SCN-078-003 — Urgent escalation permitted past exhausted budget

```gherkin
Given the daily budget is exhausted
And urgent_escalation_enabled is true
When a producer proposes a candidate with priority=1 and time_critical=true
Then the controller returns DecisionEscalated with reason "urgent_escalation"
And the smackerel_surfacing_budget_overrides_total counter increments with reason="urgent_escalation"
And user-visible dispatch is permitted
```

### SCN-078-004 — Content-key dedupe collapses cross-channel duplicates

```gherkin
Given a candidate for content_key "insight-7" on channel telegram was permitted at T0
And the dedupe_window_hours is 6
When a second producer proposes content_key "insight-7" on channel web_push at T0+1h
Then the controller returns DecisionDeduped
And the smackerel_surfacing_dedupe_total counter increments
And no duplicate dispatch occurs on web_push
```

## Non-Functional Requirements

| Area | Requirement |
|------|-------------|
| Latency | `controller.Propose` must return in <5ms p99 (in-process, no I/O on the hot path) |
| Cardinality | All `surfacing_*` metric labels bounded by the `Producer` and `Channel` enums plus a fixed reason vocabulary |
| Observability | Live `/metrics` scrape MUST expose `smackerel_surfacing_budget_remaining` as a gauge |
| Test isolation | E2E tests run only on the disposable test stack (`tests/e2e/surfacing_budget_test.go` is already compliant) |
| Configuration | All tunables flow from `config/smackerel.yaml` → `SURFACING_*` env → loader; no in-source fallback defaults |

## Adoption Inventory (existing in-tree artifacts this spec formalizes)

| Path | Kind |
|------|------|
| `internal/intelligence/surfacing/{types,controller,budget,dedupe,suppression}.go` | New code |
| `internal/intelligence/surfacing/controller_test.go` | Unit tests |
| `internal/metrics/surfacing.go` | 8 `surfacing_*` Prometheus metric families |
| `internal/config/surfacing.go` | SST loader for the `surfacing:` block |
| `tests/e2e/surfacing_budget_test.go` | 3 e2e scenarios (PASS evidence in report.md) |
| `cmd/core/main.go` | Controller wiring |
| `internal/scheduler/{scheduler,jobs,jobs_test}.go` | `Propose()` integration across 7 producers |
| `internal/config/{config,validate_test}.go` | `SurfacingConfig` struct |
| `internal/metrics/metrics_test.go` | 8 new metric families covered |
| `scripts/commands/config.sh` | `SURFACING_*` env emit |
| `config/smackerel.yaml` | `surfacing:` SST block |

## Product Principle Alignment

| Principle | How this spec implements it |
|-----------|----------------------------|
| 6 — Invisible by default, felt not heard | The budget + suppression layers are the mechanical enforcement of the `<3 nudges/week` and "no status-update prompts" rules. Without the controller, individual producers cannot honor a global cap. |
| 7 — Small, frequent, actionable output | Dedupe collapses redundant cross-channel deliveries that would otherwise dilute actionability. Budget keeps daily volume bounded. |
| 9 — Design for restart, not perfection | Suppression of acknowledged items means the user is never re-pestered after engaging once — returning sessions don't restart the nudge backlog. |

## Cross-Spec Coordination

- **Spec 021 (intelligence-delivery):** Provided the original Scope 4
  design notes (SCN-021-016..019). This spec is the explicit follow-up
  per commit `640b95d0`.
- **Spec 025 (alert-delivery-sweep):** First production consumer of
  `Propose()`. No code change needed here — adoption only.
- **Spec 054 (digest-producer):** Second consumer. Same.

## Out of Scope

- Adding new producers or channels (covered by their owning specs).
- A persistent suppression store across process restarts (current
  in-memory store is acceptable for mvp; future spec may add Postgres
  backing).
- A user-facing controller-tuning UI (operator-only via SST today).
