# SCOPE-07: What-Changed Feed

**Status:** Not Started  
**Scope-Kind:** runtime-behavior  
**Depends On:** SCOPE-06

## Outcome

Provide a two-column "what changed" activity feed as a spec-106 `Activity`
workspace view, populated by `WhatChangedRead`: a left column of what the system
did/decided (a bounded, cursor-paged, authorized read of `agent_traces` +
surfacing verdicts + topic-lifecycle transitions) and a right column of recently
touched items (a bounded recency read). It answers a returning user's
what-did-I-miss without an unread backlog counter or guilt, fabricates no entry,
is restart-safe (a stateless projection with no per-user seen/unread watermark),
and adds no second activity store.

## Requirements And Scenarios

- FR-107-020, FR-107-021, FR-107-022, FR-107-027, FR-107-028
- SCN-107-014, SCN-107-015

```gherkin
Scenario: SCN-107-014 What-changed feed shows real system decisions
  Given the user opens the what-changed feed
  When the feed renders
  Then the left column shows real ingested, connected, and decided events from the audit trail and topic lifecycle
  And the right column shows recently touched items
  And no entry is fabricated
```

```gherkin
Scenario: SCN-107-015 Returning user sees what changed without backlog guilt
  Given a user returns after a week away
  When the user opens the Today cockpit and the what-changed feed
  Then the surface answers what changed while away
  And it does not present an unread backlog counter or guilt-inducing state
```

## UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|---|---|---|---|---|
| Real system decisions | Disposable stack; seeded `agent_traces` + surfacing verdicts + topic-lifecycle moves + recent items; valid scoped session | Open the what-changed feed | Left column shows real ingested/connected/decided/lifecycle events; right column shows recently touched items; no fabricated entry | e2e-ui |
| Returning without guilt | A user returns after a week; real events in the away window | Open the cockpit + feed | A bounded "while you were away (N days)" aggregate of real events; no unread counter or guilt-inducing state | integration / e2e-ui |
| Restart-safe | Process restart between renders | Re-open the feed | Same stateless projection of real events; no persisted per-user seen/unread watermark; no second store | integration |
| Honest quiet/partial | No activity in range / one source fails | Open the feed | `quiet` (successful empty read) distinct from `partial` (one source failed) distinct from `unavailable` (all failed); empty is never failure and failure is never empty | integration / e2e-ui |

## Implementation Plan

1. Implement `WhatChangedRead(principal, range, cursor)` over existing owner stores only (no new store): left column = a bounded read of `agent_traces` (`internal/agent/store.go`, `020_agent_traces.sql`) for ingested/connected/decided events + the surfacing controller's terminal verdicts made legible (from the existing `smackerel_surfacing_*` verdict vocabulary) + topic-lifecycle transitions (`internal/topics/lifecycle.go`); right column = a bounded recency read of canonical item tables ordered by `updated_at`/last-interaction (the same tables the product already reads).
2. Bound + paginate: a range selector (`Last 24h`/`Last 7d`) and an opaque principal-bound cursor over `created_at DESC` capped at `what_changed_page_cap` per column (spec-105 HMAC-cursor pattern; no raw offset, no unbounded scan).
3. Honest outcomes through `HonestStatePresenter`: `populated`, `quiet` (successful empty read in range — not an error), `partial` (one source failed; the others render), `unavailable` (all sources failed), `unauthorized`. Empty is never rendered as failure; failure is never rendered as empty; no entry is fabricated (FR-107-021, FR-107-022).
4. Restart-safe (no second store): the feed is a stateless projection of real events with no persisted per-user "seen"/"unread" watermark and no counter. A returning user's "while you were away (N days)" is a bounded aggregate of the same real events (counts of digests/ingests/lifecycle moves in the away window), never a backlog (FR-107-021). Persisting an unread watermark is forbidden (would add a second store + a guilt counter).
5. Authorization + telemetry: re-authorize every row against the principal's grants using each underlying store's existing authorization (agent-trace gating, topic read, graph-reader allowlist for connected-events); telemetry carries only the bounded producer/channel/verdict/timing/count vocabulary; each entry links to the underlying item/decision without exposing unauthorized content (FR-107-027, FR-107-028).
6. Update architecture/testing documentation through the docs owner during implementation; introduce no new activity store or migration (the read is over existing owner stores).

## SST No-Default Decision (Reserved)

- `what_changed_page_cap = 25` — the per-column, per-page cap. Fail-loud SST key under `config/smackerel.yaml`; config-compile validates it is an integer in `1..N`. It caps the cursor-paged `created_at DESC` read per column; no unbounded scan. No `${VAR:-default}`/`os.getenv(k, default)` fallback. (design.md OQ4.)

## Shared Infrastructure Impact Sweep

- **Protected contracts:** `agent_traces` (`internal/agent/store.go`, `020_agent_traces.sql`), topic lifecycle (`internal/topics/lifecycle.go`), the canonical item recency tables, the existing `smackerel_surfacing_*` verdict vocabulary, and the spec-106 `Activity` workspace view; the spec-105 HMAC-cursor pattern.
- **Independent canaries:** the existing `agent_traces` admin list stays green; existing item/detail recency reads are unchanged; the existing topic-lifecycle transitions are unchanged; no service worker caches the feed read.
- **Rollback:** the feed is an additive stateless projection; disabling it leaves `agent_traces`, topics, and item stores untouched; there is no watermark/store to migrate or roll back.

## Change Boundary

**Allowed during execution:** the `WhatChangedRead` composition endpoint (a bounded
read over existing owner stores), the two-column `Activity` view render, the
`proactive:` SST `what_changed_page_cap` key, and tests/docs named by this scope.  
**Excluded:** creating a new activity store, an unread/seen watermark, or a
migration; editing `agent_traces`, the topic lifecycle, or the item stores;
the cockpit/rail/palette/card surfaces.

## Test Plan

| ID | Test Type | Category | Scenario | File / Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| T107-014-U | Unit | `unit` | SCN-107-014 | `web/pwa/tests/what_changed_feed_model_test.ts` - `SCN-107-014 two-column projection renders real events, no fabricated entry` | `./smackerel.sh test unit` | No |
| T107-014-I | Integration | `integration` | SCN-107-014 | `tests/integration/proactive/what_changed_read_test.go` - `SCN-107-014 bounded read over agent_traces + verdicts + lifecycle + recency` | `./smackerel.sh test integration` | Yes |
| T107-014-A | E2E API regression | `e2e-api` | SCN-107-014 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-014 what-changed read API` | `./smackerel.sh test e2e` | Yes |
| T107-014-W | E2E UI regression | `e2e-ui` | SCN-107-014 | `web/pwa/tests/what-changed-feed.spec.ts` - `SCN-107-014 real system decisions in the left column, recent items in the right` | `./smackerel.sh test e2e-ui` | Yes |
| T107-015-U | Unit | `unit` | SCN-107-015 | `web/pwa/tests/what_changed_feed_model_test.ts` - `SCN-107-015 while-you-were-away aggregate has no unread counter` | `./smackerel.sh test unit` | No |
| T107-015-I | Integration | `integration` | SCN-107-015 | `tests/integration/proactive/what_changed_read_test.go` - `SCN-107-015 returning-user summary is a bounded aggregate, no watermark` | `./smackerel.sh test integration` | Yes |
| T107-015-A | E2E API regression | `e2e-api` | SCN-107-015 | `tests/e2e/proactive_experience_e2e_test.go` - `SCN-107-015 returning-user summary API` | `./smackerel.sh test e2e` | Yes |
| T107-015-W | E2E UI regression | `e2e-ui` | SCN-107-015 | `web/pwa/tests/what-changed-feed.spec.ts` - `SCN-107-015 what changed while away, no backlog counter or guilt state` | `./smackerel.sh test e2e-ui` | Yes |
| T107-07-RESTART | Integration | `integration` | SCN-107-015 | `tests/integration/proactive/what_changed_read_test.go` - `feed is restart-safe: no persisted seen/unread watermark, no second store` | `./smackerel.sh test integration` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] SCN-107-014 What-changed feed shows real system decisions: the left column shows real ingested/connected/decided events from `agent_traces` + surfacing verdicts + topic lifecycle, the right column shows recently touched items, and no entry is fabricated.
- [ ] SCN-107-015 Returning user sees what changed without backlog guilt: the feed answers what changed while away as a bounded aggregate of real events, with no unread backlog counter or guilt-inducing state.
- [ ] `WhatChangedRead` is a bounded, cursor-paged (`what_changed_page_cap`), authorized, restart-safe projection over existing owner stores; it adds no second activity store, no unread/seen watermark, and no migration, and re-authorizes every row (FR-107-027).

#### Test Evidence - One Item Per Test Plan Row

- [ ] T107-014-U passes with current-session evidence in `report.md#t107-014-u`.
- [ ] T107-014-I passes against the disposable stack with current-session evidence in `report.md#t107-014-i`.
- [ ] T107-014-A passes through production HTTP routes with current-session evidence in `report.md#t107-014-a`.
- [ ] T107-014-W passes without interception and proves the two-column real-event projection in `report.md#t107-014-w`.
- [ ] T107-015-U passes with current-session evidence in `report.md#t107-015-u`.
- [ ] T107-015-I passes against the disposable stack with current-session evidence in `report.md#t107-015-i`.
- [ ] T107-015-A passes through production HTTP routes with current-session evidence in `report.md#t107-015-a`.
- [ ] T107-015-W passes without interception and proves no backlog counter/guilt state in `report.md#t107-015-w`.
- [ ] T107-07-RESTART proves the feed is restart-safe with no watermark/second store in `report.md#t107-07-restart`.

#### Build Quality Gate

- [ ] Scope tests, check, lint, format, source/config validation (including the `what_changed_page_cap` no-default SST key), architecture documentation, consumer review, artifact lint, traceability, zero warnings, and change-boundary review pass with executed evidence.

## Uncertainty Declaration

All items remain unchecked because implementation, tests, and runtime validation
have not been executed by the planning owner. No new activity store, watermark, or
migration is introduced; the read is over existing owner stores only.
