# Design — Cross-Surface Surfacing Prioritizer

**Spec:** [spec.md](spec.md)
**Mode:** improve-existing (adopt in-tree groundwork)
**Release Train:** mvp

## 1. Architecture

```text
┌──────────────────────────────────────────────────────────────────┐
│  Intelligence Producers (7)                                      │
│  alerts │ digest │ resurfacing │ weekly_synthesis │              │
│  monthly_report │ pre_meeting_briefs │ frequent_lookups          │
└────────────────────┬─────────────────────────────────────────────┘
                     │ controller.Propose(ctx, SurfacingCandidate)
                     ▼
┌──────────────────────────────────────────────────────────────────┐
│  internal/intelligence/surfacing/controller.go                   │
│                                                                  │
│  dedupe → suppress → budget → escalate                           │
│     │         │         │         │                              │
│     ▼         ▼         ▼         ▼                              │
│  dedupe.go suppress.go budget.go (escalate inlined)              │
└────────────────────┬─────────────────────────────────────────────┘
                     │ SurfacingDecision { Kind, Reason }
                     ▼
┌──────────────────────────────────────────────────────────────────┐
│  Dispatch channels                                               │
│  telegram │ web_push │ ntfy │ email_out │ digest                 │
└──────────────────────────────────────────────────────────────────┘

  metrics sink (internal/metrics/surfacing.go)
  └─ smackerel_surfacing_nudges_delivered_total          (producer,channel)
  └─ smackerel_surfacing_acted_on_total                  (producer,channel)
  └─ smackerel_surfacing_false_positive_total            (producer,channel)
  └─ smackerel_surfacing_dedupe_total                    (producer)
  └─ smackerel_surfacing_suppression_total               (reason)
  └─ smackerel_surfacing_budget_overrides_total          (reason)
  └─ smackerel_surfacing_deferred_budget_exhausted_total (producer)
  └─ smackerel_surfacing_budget_remaining                (gauge)
```

## 2. Producer Enum (bounded — adding a new producer is a code change)

```go
ProducerAlerts            // spec 025
ProducerDigest            // spec 054
ProducerResurfacing       // future
ProducerWeeklySynthesis   // future
ProducerMonthlyReport     // future
ProducerPreMeetingBriefs  // future
ProducerFrequentLookups   // future
```

## 3. Channel Enum

```go
ChannelTelegram  ChannelWebPush  ChannelNtfy  ChannelEmailOut  ChannelDigest
```

## 4. Decision Vocabulary

| Decision | When | Metric |
|----------|------|--------|
| `permit` | All gates clear | `nudges_delivered_total{producer,channel}` |
| `deduped` | Content key seen in dedupe window | `dedupe_total{producer}` |
| `suppressed` | User acked this content key recently | `suppression_total{reason}` |
| `deferred-budget-exhausted` | Budget hit, no escalation | `deferred_exhausted_total{producer}` |
| `escalated` | Budget hit but p1+time_critical+enabled | `budget_overrides_total{reason}` |

## 5. SST Configuration

`config/smackerel.yaml`:

```yaml
surfacing:
  daily_nudge_budget: 5
  suppression_window_hours: 4
  dedupe_window_hours: 6
  urgent_escalation_enabled: true
```

Loader: `internal/config/surfacing.go` → `SurfacingConfig`. Emit via
`scripts/commands/config.sh` → `SURFACING_DAILY_NUDGE_BUDGET`,
`SURFACING_SUPPRESSION_WINDOW_HOURS`,
`SURFACING_DEDUPE_WINDOW_HOURS`,
`SURFACING_URGENT_ESCALATION_ENABLED`. Fail-loud — missing env aborts
startup (NO-DEFAULTS SST policy).

## 6. Pipeline Order (mandated)

`dedupe → suppress → budget → escalate`

Rationale:
1. **dedupe** before suppress — if the candidate is a same-key duplicate
   we never want to "consume" a suppression slot or budget unit.
2. **suppress** before budget — acknowledged items must never count
   against the daily budget (otherwise quiet users get penalized).
3. **budget** before escalate — escalate is the explicit override path
   and only triggers when budget would otherwise reject.

See `internal/intelligence/surfacing/controller.go` `Propose()` for the
implemented step ordering.

## 7. Test Strategy

| Layer | File | Coverage |
|-------|------|----------|
| Unit | `internal/intelligence/surfacing/controller_test.go` | Per-gate truth tables |
| Integration | (uses scheduler's existing `jobs_test.go`) | Propose() invoked from each of 7 producers |
| E2E | `tests/e2e/surfacing_budget_test.go` | SCN-078-001 (budget defers), SCN-078-002 (ack suppresses), metrics scrape proves families exposed |

E2E tests are already PASS — see evidence anchors in
`report.md#adopted-evidence`.

## 8. Coordination with Consumer Specs

- **Spec 021** — provided the SCN-021-016..019 design contract. This
  spec re-numbers as SCN-078-001..004 in our scenario-manifest.
- **Spec 025 (knowledge-synthesis-layer)** and **Spec 054
  (notification-intelligence-handler)** — earlier scope-09 entries in
  both specs referenced an aspirational `SurfacingProposalSink`
  interface and `AcknowledgmentBus` async pattern. Those scopes were
  rescoped out before any consumer wiring was built; both specs are now
  `status=done` without that interface. Spec 078 ships the synchronous
  `Controller.Propose(ctx, SurfacingCandidate) (SurfacingDecision,
  error)` call as the live contract. Any future producer (including
  reactivated 025/054 work) integrates by calling `Propose` directly —
  no sink or bus indirection exists or is planned.

## 9. Test Environment Isolation

E2E tests run only on the disposable test stack via
`./smackerel.sh test e2e`. The controller's in-memory budget store is
reset per-process; the test stack's separate process guarantees no
spillover to dev.

## 10. Migration / Rollback

- **Migration:** none — code is already on trunk.
- **Rollback:** revert the producers' `Propose()` call sites to direct
  dispatch (would require code change to each producer; controller code
  can remain compiled in as dead code).
- **Pointer-swap rollback (release train model):** unaffected — no new
  flag, no schema change.

## 11. Open Design Questions

None blocking. Future work (out of scope for this spec):

- Persisting the suppression store to Postgres so process restarts don't
  reset acknowledgements.
- Adding a `surfacing_proposal_latency_seconds` histogram if p99 ever
  starts to drift.

## 12. Out of Scope

- **Explicit `normalize` pipeline stage** — earlier drafts of this design
  listed `normalize` as the first pipeline step. The implemented
  controller has no such stage: `SurfacingCandidate` structs arrive at
  `Controller.Propose` already normalized by the producer-side adapters
  (ContentKey is a string the producer chooses; ProposedAt is a
  `time.Time` the producer supplies). Centralizing normalization in the
  controller would duplicate work the producers already do and would
  not change any observable behavior, so no normalize stage exists or
  is planned.
