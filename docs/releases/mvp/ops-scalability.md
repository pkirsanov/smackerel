# Ops & Scalability — Smackerel MVP

## Operational complexity assessment

MVP runs the same operational surface as the post-Phase-5 system, with three adjustment-shaped additions (M1a surfacing controller, M1b calendar-triggered briefs, M1c reminder/promise engine) and one UI surface (M2 wiki/graph-browse). Operational complexity grows by:

- **+1 new scheduler tenant** (M1b + M1c share the existing `internal/scheduler/`; no new scheduler component)
- **+1 cross-channel coordination point** (M1a's interruption budget — single in-process arbiter; no new infrastructure)
- **+1 web-UI view family** (M2 pivot views; same web service as 073)

No new infrastructure component is introduced. No new container is added.

## Per-user load profile (MVP)

MVP target: 1–5 users per Smackerel instance. Load profile per instance:

| Surface | MVP load expectation | Source |
|---------|---------------------|--------|
| Connectors running | 18+, cron-scheduled, staggered backoff (`internal/connector/backoff.go`) | spec 019 |
| NATS message rate | Bursty per connector poll; steady-state low (≤ 10 msg/s sustained) | spec 002, 046 |
| Postgres query load | Dominated by semantic search (pgvector cosine) + graph traversal | spec 002, 063, 068 |
| ML sidecar inference | Embedding generation on every artifact; LLM rerank on every search; LLM agent on every assistant turn | spec 002, 050, 061, 064 |
| Surfacing controller (M1a) | Single-process arbiter; trivial CPU/memory | M1a design |
| Brief producer (M1b) | Scheduled per upcoming calendar event with brief-qualifying filter | M1b design |
| Reminder engine (M1c) | Scheduled per active user-stated promise + condition-eval poll | M1c design |
| Wiki/graph-browse (M2) | User-initiated; one graph traversal per pivot click | M2 design |

No surface exceeds household-scale load at MVP.

## Scaling triggers (when does MVP break)

| Trigger | Threshold | Mitigation |
|---------|-----------|------------|
| Postgres query latency exceeds search SLO | p95 > 5 s on semantic search | Tune pgvector index params; scale Postgres instance; sharded household-per-DB (not in MVP) |
| ML sidecar inference saturates | Embedding queue depth > 1000 sustained | Scale to multiple sidecar replicas (spec 050 supports); switch to faster embedding model |
| NATS queue depth grows unbounded | JetStream consumer lag > 5 min | Scale consumers; rate-limit producer connectors |
| Surfacing controller exceeds daily nudge ceiling | Nudges/day > operator-set N (OQ-1) | M1a contract: drop nudges or batch — by design |
| Reminder engine condition-eval CPU exceeds budget | Eval CPU > 10% of single core | Apply per-promise resource budget (R8 in [`business-plan.md`](business-plan.md)) |
| Backup window exceeds nightly slot | T1 + T2 backup > 6 h | Tier T1 alone for nightly; T2 weekly; spec 048 |
| Multi-user contention on per-user bearer auth | Token churn > 10/min/user | Already addressed by spec 044, 060; no MVP scaling action |
| Disk usage growth from photos/cloud-drives ingestion | Disk usage > 80% | Operator alert; archive policy via spec 048 + ZFS T1 |

## Support plan

| Channel | Owner | Cadence |
|---------|-------|---------|
| `README.md` + `docs/` | Operator + `bubbles.docs` | Updated as M-items close |
| Issue tracker | Operator | Best-effort |
| Direct support | None | No commercial support obligation at MVP |
| Discord / chat | None at MVP | Optional RELEASE-V1+ |

## Incident response readiness

| Capability | MVP state | Source |
|-----------|-----------|--------|
| Backup tier (T1 ZFS + T2 host-local restic) | Delivered | spec 048 |
| Restore drill | Per `bubbles-upkeep-cadence` (quarterly default) | spec 048, `.github/instructions/bubbles-upkeep-operations.instructions.md` |
| BCDR plan | Per `bubbles-backup-bcdr-doctrine` | `.github/skills/bubbles-backup-bcdr-doctrine/SKILL.md` |
| Rollback | Pointer-swap per Gate G081 | [`docs/Deployment.md`](../../Deployment.md), [`deployment.md`](deployment.md) |
| Observability | Prometheus + adapter contract; metrics exposed (alert promotion = RELEASE-V1) | spec 030, 049 |
| Secret rotation | Per `bubbles-upkeep-cadence` | `.github/skills/bubbles-upkeep-cadence/SKILL.md` |
| Compliance sweep | Quarterly per `bubbles-upkeep-cadence`; `bubbles.audit` signs off | spec 048, gates G117–G120 |

## Post-launch monitoring + iteration cadence

| Activity | Cadence | Owner |
|---------|---------|-------|
| Surfacing-controller SLO review (acted-on-rate, false-positive rate) | Weekly during first month after M1a lands; monthly thereafter | Operator |
| Connector freshness audit (each connector still polling?) | Monthly | Operator + spec 029 dashboards |
| Spec-portfolio drift sweep | Per `bubbles.spec-review` cadence (operator-triggered) | `bubbles.spec-review` |
| Principle-violation grep gate review | Per PR (continuous after M3 ratification) | PR review |
| Capability ledger reconciliation | At each release-packet refresh | `bubbles.releases` |

## What MVP explicitly does NOT scale to

- Multi-tenant SaaS hosting (architectural; would require auth model overhaul beyond spec 044/060)
- ≥ 100 concurrent users per instance (Postgres + ML sidecar would saturate; not designed for)
- Real-time outbound action with external-system rate limits (Outbound Action is RELEASE-V1 Gap B)
- Geographic distribution (single-instance, single-host model — by design at MVP)
