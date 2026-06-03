# Ops & Scalability — Smackerel v1

## Operational complexity assessment

v1 adds substantial operational complexity over MVP:

| Source of complexity | Delta vs MVP | Mitigation |
|----------------------|--------------|-----------|
| **Many new OAuth flows** (V1-A..H) | +8 connector families; each with per-user token refresh, per-provider quirks | Centralize OAuth-token storage + refresh via spec 044/060/052; per-spec quirk handling |
| **Outbound Action runtime surface (V2-A)** | First write-side capability; new Postgres tables; new audit-log integrity requirement | V2-A foundation spec MUST design tables + integrity model; spec 048 backup coverage required |
| **V2-A per-target rate limits + undo windows** | Per-target state machines; per-target external API rate-limit awareness | OQ-V5/V6 operator decisions inform defaults; spec 086 design |
| **Conditional native mobile (V3-B/C)** | If chosen: app-signing custody, per-store CI, beta-channel management, crash reporting | Out of scope until V3-A decides; if native, dedicated deployment narrative in specs 088/089 |
| **Capability map generator (V4-A)** | Continuous regeneration pipeline; drift detection vs manual capability claims | V4-A spec design |
| **SLO alerting (V5-A/B)** | Alert tuning; alert routing decision; on-call definition (operator-only at v1) | Per spec 049 + observability adapter |
| **Voice transcription (V1-I)** | Whisper model in ML sidecar OR cloud STT routing | spec 050 sidecar scaling; OQ-V4 backend decision |
| **Messages family (V1-H) device bridges** | Apple/Google/Signal/Slack bridges; each with auth + isolation concerns | Per-provider in V1-H spec design; defer Signal/iMessage if bridge model unclear (RV8 in [`business-plan.md`](business-plan.md)) |

## Per-user load profile (v1)

v1 target: 1–10 users per Smackerel instance (slight expansion from MVP's 1–5).

| Surface | v1 load expectation | Source |
|---------|---------------------|--------|
| Connectors running | MVP roster + 8–12 new connector families; each at provider-specific cron rate | V1-A..J |
| NATS message rate | +30–80% vs MVP (more sources × more items per source) | V1-A..J |
| Postgres query load | +graph traversal load from V4-A; +outbound-action-log writes from V2 | V2-A, V4-A |
| ML sidecar | +voice transcription (V1-I) — significantly heavier per-item than text embedding | V1-I, spec 050 |
| Outbound action rate | New: bursty (operator-triggered or rule-triggered) writes to external APIs | V2-A |
| Surfacing controller | Same MVP-level work; SLO alerts add a Prometheus rule eval | V5-A |
| Wiki/graph-browse | Same MVP-level work | unchanged |
| Audit log writes | New: every V2-B outbound action writes an audit row | V2-A |

## Scaling triggers (when does v1 break)

| Trigger | Threshold | Mitigation |
|---------|-----------|------------|
| Postgres outbound-action-log write rate saturates | sustained > 100 writes/s | Partition table by time; archive policy via spec 048 |
| V2-A undo-window state lookup latency degrades | p95 > 500 ms | Index on (user, action_target, status); cache hot window in-process |
| ML sidecar voice-transcription queue depth grows | queue > 100 items sustained | Scale to multiple sidecar replicas; downscale model; route to cloud STT |
| OAuth token refresh failures spike for one provider | > 5% failure rate sustained | Per-provider circuit breaker; alert operator |
| External API rate-limit-hit counter (V2-A per-target) > threshold | per-target threshold from V2-A spec | Per-target backoff; queue the action; surface to user |
| Native mobile (if V3-B/C) push-notification fan-out spikes | n/a until V3 lands | Per-store push infrastructure planning in V3-B/C |
| SLO alerting noise > operator tolerance | > N alerts/week sustained | Tune thresholds per [`business-plan.md`](business-plan.md) RV7 mitigation |
| Audit log storage growth > backup window capacity | > target/month sustained | Per spec 048 retention policy; archive cold audit rows |

## Support plan (v1)

Same posture as MVP: no commercial support obligation. Operator + community.

If a commercial conversation triggers post-v1 (OQ-V9), support model becomes a v2 question.

## Incident response readiness

| Capability | v1 state | Source |
|-----------|----------|--------|
| Backup tier model | Carry-forward MVP; new tables (`outbound_action_log` etc.) MUST be in backup scope | spec 048; V2-A |
| Restore drill | Quarterly per `bubbles-upkeep-cadence`; new tables MUST be in restore drill scope | spec 048 |
| BCDR plan | Carry-forward; V2-A audit-log integrity is a new BCDR concern (regulatory analog) | `bubbles-backup-bcdr-doctrine` |
| Rollback | Pointer-swap per Gate G081; V2-A external-side actions are NOT auto-undone on rollback | [`deployment.md`](deployment.md) |
| Observability | MVP + V5-A/B alerting | spec 049, V5-A/B |
| Secret rotation | Carry-forward; OAuth refresh tokens added to rotation inventory | `bubbles-upkeep-cadence` |
| Compliance sweep | Quarterly per `bubbles-upkeep-cadence`; V2-A audit-log makes G117 audit-trail trivially satisfied for outbound actions | gates G117–G120 |

## Post-launch monitoring + iteration cadence

| Activity | Cadence | Owner |
|---------|---------|-------|
| MVP M1a + V5-A/B SLO review | Weekly first month; monthly thereafter | Operator |
| V2-A outbound-action audit review | Monthly | Operator |
| OAuth refresh-failure review per provider | Continuous via spec 049 dashboards | Operator |
| Connector freshness audit | Monthly per spec 029 dashboards | Operator |
| Spec-portfolio drift sweep (V6-A) | At v1 close + per `bubbles.spec-review` cadence | `bubbles.spec-review` |
| Capability map regeneration | Continuous (V4-A) | V4-A generator |
| Principle-violation grep gate review | Continuous per PR | PR review |
| Capability ledger reconciliation | At next release-packet refresh | `bubbles.releases` |

## What v1 explicitly does NOT scale to

- **Multi-tenant SaaS hosting** — same as MVP non-goal. Architectural; would require auth model overhaul.
- **≥ 100 concurrent users per instance** — Postgres + ML sidecar would still saturate.
- **Unattended autonomous outbound action** — V2-A is consent-gated by design; v1 does NOT relax that.
- **Real-time outbound action against high-rate-limit external APIs** — V2-A per-target rate-limit handling is best-effort; not designed for sub-second action latency.
- **Cross-instance federation** — single-instance, single-host model retained.

## Environment-pollution discipline (v1 specific)

Per `.github/instructions/bubbles-env-pollution-isolation.instructions.md`:

- V2-A tests MUST NEVER send real outbound actions against real external accounts (no real gmail send, no real calendar event creation in a real calendar, no real Slack post). Test stacks use mocked external endpoints.
- V1-A..H connector tests MUST use disposable OAuth credentials in ephemeral test stacks. NEVER bind a test to a real production user account.
- V2-A `outbound_action_log` test writes MUST go to test Postgres only; NEVER to prod tables.
- env-pollution-scan.sh continues to enforce at pre-push.
