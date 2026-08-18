# Ops & Scalability — Smackerel `next`

## Operational complexity assessment

`next` adds **no new infrastructure component, no new container, no new persistent store, and no new secret class.** Every capability in this phase runs on the operational surface `mvp` already established. This is a structural property, not a coincidence: [`vision.md`](vision.md) makes "no parallel store" a non-goal (Product Principle 5), and [`deployment.md`](deployment.md) records that the phase rides the existing artifact contract unchanged.

The complexity `next` *does* add is concentrated in **per-query behaviour** and in **what a deploy no longer pins**.

| Source of complexity | Delta vs `mvp` | Where it is governed |
|---|---|---|
| **Multi-step reasoning + composing synthesis** (`084`, `087`) | The heaviest change. `mvp` did one embedding per artifact and an LLM rerank per search; `next` runs a multi-step reasoning loop and composes an answer from multiple artifacts, so **LLM work per query rises and becomes variable per query** rather than roughly constant. | ML sidecar capacity, below |
| **Intent-routed retrieval** (`095`) | Three retrieval strategies (`whole_document`, `structured_aggregate`, `vague_recall`) instead of one path, each with a per-artifact-type contract. Different intents now have materially different query costs. | [`deployment.md`](deployment.md) rollout table |
| **Runtime-switchable models** (`088`) | **A deploy no longer pins model behaviour.** Selection changes at runtime without a redeploy. Operationally this is a new class of change that alters system behaviour without leaving a deployment record. | [`deployment.md`](deployment.md) |
| **Knowledge-graph public API** (`080`) | +1 externally-reachable read surface. Exposure is an **edge-layer decision made in the deploy adapter, not here**; default posture is *not published beyond the trusted network boundary*. | knb deploy-adapter overlay |
| **NATS sidecar hardening parity** (`081`) | No new component. Reinforces an existing boundary: the sidecar stays **compute-only** — no datastore credentials, data only through the owning service tier over the typed contract wire. | [`deployment.md`](deployment.md) |
| **Target readiness hardening** (`082`) | Preconditions that run **before** apply. A failure here is the intended stop, not an incident. | [`deployment.md`](deployment.md) |
| **Fail-loud configuration** (`095`) | A missing config key **aborts startup** rather than silently defaulting. An operator seeing that error is the system working as designed. | [`deployment.md`](deployment.md) |

Deliberately absent: multi-provider model credentials. `096` is `blocked` and therefore **not part of this phase's deployable set** ([`features.md`](features.md), [`deployment.md`](deployment.md)), so no new secret class arrives with this phase.

## Per-user load profile (`next`)

Target scale is **unchanged from `mvp`: 1–5 users per instance** (carried from [`../mvp/ops-scalability.md`](../mvp/ops-scalability.md); `v1` is where the figure widens to 1–10). `next` has no reason to move it — the connector-roster lock is still in force, so **the ingestion side of the system is unchanged**.

| Surface | `next` load expectation vs `mvp` | Basis |
|---|---|---|
| Connectors running | **Unchanged.** `next` adds no connector; the roster lock lifts at `v1`, not here. | [`vision.md`](vision.md) non-goals |
| NATS message rate | **Unchanged in volume.** `081` hardens the sidecar's bus path; it does not add traffic. | `081`, [`deployment.md`](deployment.md) |
| Postgres query load | **Higher and more variable per query.** Routing (`095`) means an aggregate-intent query and a vague-recall query no longer cost the same. Still the one existing store. | `095`, Principle 5 |
| ML sidecar inference | **The dominant increase in this phase.** Multi-step reasoning (`084`) plus composing synthesis (`087`) replace a single rerank call per search. | `084`, `087` |
| Graph public API (`080`) | New, client-driven, and **unbounded by any Smackerel-side limit** — an authorized client's read rate is that client's behaviour. | `080` |
| Model selection (`088`) | No steady-state load, but the **selected model is now a load variable**: switching models changes per-query cost and latency without any deploy. | `088` |

**No numeric load figure is stated for the new capabilities in this phase, because none has been measured.** `mvp`'s per-surface figures were derived from delivered specs; the equivalent measurements for the reasoning and synthesis path do not exist yet — that measurement is precisely what the reserved `next-retrieval-quality-pipeline` slot carries. Inventing a msg/s or queue-depth number here would fabricate a capacity target nobody established.

## Scaling triggers (when does `next` break)

Carried forward from [`../mvp/ops-scalability.md`](../mvp/ops-scalability.md) and **still governing**, because the underlying surfaces are unchanged:

| Trigger | Threshold (carried from `mvp`) | Mitigation |
|---|---|---|
| Postgres query latency exceeds search SLO | p95 > 5 s on semantic search | Tune pgvector index params; scale Postgres |
| ML sidecar inference saturates | Embedding queue depth > 1000 sustained | Scale sidecar replicas; faster embedding model |
| NATS queue depth grows unbounded | JetStream consumer lag > 5 min | Scale consumers; rate-limit producer connectors |
| Backup window exceeds nightly slot | T1 + T2 > 6 h | T1 nightly, T2 weekly |
| Disk usage growth | > 80% | Operator alert; archive policy |

New in this phase. **Each threshold below is explicitly unestablished** — the trigger is real and the direction of failure is known, but no number has been measured, so none is asserted:

| Trigger | Threshold | Why it is not stated |
|---|---|---|
| Reasoning-loop depth inflates per-query latency | **Not established** | `084` makes per-query work variable. Setting a latency SLO before the evaluation gate exists would be a number without a measurement behind it. |
| Synthesis cost per answer scales with the number of composed artifacts | **Not established** | Depends on corpus shape and the selected model — and `088` means the model is not fixed by the deploy. |
| Graph public API (`080`) read rate from an authorized client | **Not established** | No rate limit is specified in this packet, and client behaviour is outside Smackerel's control. Recorded as a genuine open exposure below. |
| A model switch (`088`) degrades latency or quality | **Not applicable — this is not threshold-shaped** | The change is instantaneous and leaves no deployment record. Mitigation is procedural: record the selected model alongside any latency or quality observation ([`deployment.md`](deployment.md)). |
| Answer-quality regression | **Split verdict — routing and capture have enforced floors; answer quality has no threshold at all** | **Enforced automatically.** The assistant acceptance gate runs in the integration lane (`./tests/eval/...` is in the lane's package list), and on a full-lane run the lane fails unless exactly one `ASSISTANT_ACCEPTANCE_GATE_V1` line reports `executed_assertions >= 1` — so a gate that is skipped or evaluates nothing cannot go green ([`scripts/runtime/go-integration.sh`](../../../scripts/runtime/go-integration.sh); the wiring is contract-tested with adversarial cases in [`internal/deploy/eval_lane_contract_test.go`](../../../internal/deploy/eval_lane_contract_test.go)). It scores **routing accuracy** and **capture-fallback coverage** against two SST-*required* floors, `ASSISTANT_EVAL_ROUTING_ACCURACY_MIN` and `ASSISTANT_EVAL_CAPTURE_FALLBACK_MIN`. Recorded evidence, not re-executed for this packet: `executed_assertions=210 rows=150`, lane `1974` pass / `0` fail, fix commit `c7667d99` (BUG-061-011). **Not measured anywhere.** Retrieval quality and answer/synthesis quality: no `recall`, `precision`, `nDCG`, or `groundedness` metric exists in [`tests/eval/assistant/`](../../../tests/eval/assistant/), and `LabelRetrieval` there is an *intent* label matched from trigger stems ("show me", "pull up", "did i save") — it records that a turn **asked for** retrieval, never whether retrieval returned the right documents. The harness also states outright that "Production wiring is OUT OF SCOPE", scoring a deterministic keyword classifier that is an explicit *proxy for the production agent router* ([`tests/eval/assistant/harness.go`](../../../tests/eval/assistant/harness.go)) — so even the routing figure is a proxy signal, not a production measurement. **Net.** An *answer-quality* regression still cannot be triggered, and that dimension remains **invisible** ([`business-plan.md`](business-plan.md) R2, which stands as written) — but "nothing is evaluated" would be false. The reserved `next-retrieval-quality-pipeline` slot is what closes the remaining half, exactly as R2's mitigation already frames it. |

The last two rows are the honest operational headline of this phase: **`next` ships the capability whose failure mode it cannot yet detect.**

## Support plan

Unchanged in posture from `mvp` and `v1`: **single-operator, self-hosted, no commercial support obligation.**

| Channel | Owner | Cadence |
|---|---|---|
| [`README.md`](../../../README.md) + [`docs/`](../../) | Operator + `bubbles.docs` | Updated at phase close |
| Issue tracker | Operator | Best-effort |
| Direct / commercial support | **None** | No obligation — consistent with [`monetization.md`](monetization.md) |

## Incident response readiness

| Capability | `next` state | Source |
|---|---|---|
| Backup tiers (T1 ZFS + T2 host-local restic) | Carried forward. `next` adds no new persistent store, so **backup scope is unchanged** — a genuine benefit of the no-parallel-store constraint. | [`../mvp/ops-scalability.md`](../mvp/ops-scalability.md), Principle 5 |
| Restore drill | Quarterly per `bubbles-upkeep-cadence`; scope unchanged | `.github/instructions/bubbles-upkeep-operations.instructions.md` |
| Offsite backend | **Not provisioned.** `config/release-trains.yaml` sets `offsite_required: false` with the note that G114/G116 **warn, not block**, pending T3/T4. Carried state, not a `next` regression. | [`config/release-trains.yaml`](../../../config/release-trains.yaml) |
| Rollback | Pointer swap only; no rebuild, no source pull | [`deployment.md`](deployment.md) |
| **Rollback completeness** | **Partial by design.** A rollback restores the **code** but not necessarily the **model selection** active before it. If a rollback is motivated by answer quality, the operator must separately confirm which model is selected afterwards. | [`deployment.md`](deployment.md) |
| Pre-apply verification | Signature + bundle-hash verification before any container starts; no bypass flag | [`deployment.md`](deployment.md) |
| Observability | Health + metrics on the existing contract | [`deployment.md`](deployment.md) |
| **Quality alerting** | **Does not exist.** No retrieval-quality or synthesis-quality SLO alert. [`deployment.md`](deployment.md) states plainly that claiming quality monitoring before the evaluation gate lands would be a fabricated capability. | [`deployment.md`](deployment.md) |
| Secret rotation | Carried forward. No new secret class this phase (`096` blocked). | `bubbles-upkeep-cadence` |

## Release-process debt carried into this phase

This is operational load on the **release and certification process**, not on the runtime. It is recorded here because it is measured, it is real, and it will be paid one packet at a time by whoever drives `next` to close. Every figure below is attributed to the row that measured it; none is re-derived or estimated here.

| Debt | Measured state | Recorded in |
|---|---|---|
| **G136 human-acceptance shape** | **353** `uservalidation.md` files under `specs/`; **352 carry the pre-PD-12 shape**, the single migrated packet being `BUG-080-001`. **33** packets are non-terminal-for-mode; **31** run a mode whose ceiling is `done` and will meet G136 at their terminal transition, while **2** run `product-to-planning` and do not meet it at their own ceiling. Specs `110`/`111`/`112` carry **24**/**34**/**44** already-checked items (**102**, zero unchecked) that are *planning-review agreements*, not accepted delivered behaviour. | RTE-N4, [`actions.md`](actions.md) |
| **G136 across blocked packets** | **17** packets are `blocked`; **16** emit a guard result block and **G136 fails in 16 of 16** of those. The seventeenth emits none and is counted as neither pass nor fail. | RTE-N5, [`actions.md`](actions.md) |
| **G088 vs the repo's own PII policy** | **PASS 59 / FAIL 40 of 99** `done` specs with a sibling `spec.md`. Of **90** post-certification file-touches, **68 (76%)** are *mandated redaction* under this repo's genericization policy, not planning drift. A repo obeying its own PII policy **necessarily accumulates** G088 violations, because G088 cannot distinguish a hostname redaction from a requirements change. | **RTE-M6, [`../mvp/actions.md`](../mvp/actions.md)** — a portfolio-wide condition recorded in the `mvp` packet, not a `next` finding |
| **Product-deployment-boundary lint** | **Resolved.** Re-measured 2026-08-18: `PASS`, **0 findings, exit 0** (previously 11 findings). Nine paths fixed at source in `3b263562`; two residual captured-transcript lines allowlisted per-file in knb `be18236a` rather than rewritten, because editing recorded evidence to satisfy a lint is tampering. | [`actions.md`](actions.md) boundary-lint note |

The G088 row is the structurally interesting one, and it is **not** a `next` defect: it is a framework-level conflict between two rules this repo is simultaneously obliged to obey. It is routed upstream via `bubbles.setup`, not fixed downstream, because `post-cert-spec-edit-guard.sh` is a framework-managed install artifact.

## Post-launch monitoring + iteration cadence

| Activity | Cadence | Owner |
|---|---|---|
| Gate G101 reconciliation for phase `next` | **Continuous — mechanically enforced.** `./smackerel.sh release reconcile` runs blocking in `scripts/git-hooks/pre-push` and in the standalone `release-schema` CI job. Aggregated across phases, so one failing phase cannot mask another. | `bubbles.devops` (delivered, OPS-N3) |
| Selected-model record alongside any latency or quality observation | Every observation | Operator |
| Graph public API (`080`) exposure review — is it still inside the trusted boundary? | At each promotion | Operator + deploy adapter |
| Connector freshness audit | Monthly (carried from `mvp`) | Operator |
| Reserved-slot binding refresh — replace `spec=none` with the real path as each spec directory is created | At spec creation (ENG-N4/N5/N6) | `bubbles.releases` |
| Principle-violation grep gates (Principle 11 local-default, export-entitlement, remote-egress audit) | Per PR, blocking | PR review |
| Capability ledger / release-packet reconciliation | At each packet refresh | `bubbles.releases` |
| Retrieval-quality review | **Not schedulable yet** — requires the reserved evaluation gate | — |

## Environment-pollution discipline (`next` specific)

Per `.github/instructions/bubbles-env-pollution-isolation.instructions.md` and [`deployment.md`](deployment.md):

- Telemetry emitted during **any** test category carries an `env=test*` label and targets the ephemeral test stack's own monitoring. The production/operate plane is **read-only** from a feature scope.
- **The ML sidecar must never receive a datastore or bus URL in any environment.** `081` hardens exactly this boundary; a deploy or test fixture that hands the sidecar a database URL violates the compute-only contract.
- Graph public API (`080`) tests must bind an ephemeral test stack, never a live corpus.
- `088` model-switching tests must not silently reconfigure a running instance's selected model — the runtime switch is the capability under test **and** a live-system mutation.
- No test writes to prod monitoring, prod backup paths, or a deployment manifest.

## What `next` explicitly does NOT scale to

Carried forward from `mvp` and `v1`, all still true:

- **Multi-tenant SaaS hosting** — architectural; would require an auth-model overhaul.
- **≥ 100 concurrent users per instance** — Postgres and the ML sidecar would saturate, and the reasoning layer makes this *worse* than at `mvp`, not better.
- **Geographic distribution / cross-instance federation** — single-instance, single-host by design.
- **Outbound action at any scale** — not in this phase at all.

New at `next`:

- **High-rate external consumption of the graph API.** `080` is a read surface for authorized clients on a trusted network, not a public API tier. No rate limit, quota, or per-client throttle is specified in this packet.
- **Unattended answer-quality assurance.** Until the evaluation gate lands, quality is verified by a human reading answers. That does not scale, and it is the honest state.
- **Auditable per-client corpus egress.** Grant enforcement is `108`, which is `specs_hardened` and undelivered. Until it lands, the graph API's default posture — inside the trusted boundary — is doing the work that an audited grant will later do. This is also why [`actions.md`](actions.md) gates ENG-N3 behind ENG-N2: shipping the MCP surface (`109`) before grants (`108`) would expose the corpus with no audited grant behind it.
