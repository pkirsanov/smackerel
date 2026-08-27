# Features — Smackerel `next`

<!--
  MACHINE-BINDING ANNOTATIONS — Gate G101 (release-delivery-reconciliation-guard.sh).
  The HTML-comment annotation lines below reconcile this packet's promised features
  against validate-certified, terminal spec truth. They are additive and invisible in
  rendered Markdown: the human prose tables remain the authoritative narrative.

  AUTHORED 2026-08-04 against HEAD 8d971420. This packet did not exist before that
  date, which meant release train `next` — eleven member specs, seven of them
  delivered — carried ZERO G101 surface. Every class below was DERIVED by replaying
  the guard's own required-feature test against each bound spec's state.json at that
  commit (terminal-for-mode AND `validate` present in
  certifiedCompletedPhases/completedPhases AND status != blocked). No class was
  copied from another packet or from prose.

  This packet is the HOME PACKET (docs/releases/README.md, Census rule 2) for every
  spec carrying `releaseTrain: next`. Re-run the gate with:
    bash .github/bubbles/scripts/release-delivery-reconciliation-guard.sh \
      --repo-root "$(pwd)" --phase next --require-coverage

  RE-RECONCILED 2026-08-27 against HEAD 8a599234. Three changes, each derived by
  reading the bound spec's state.json at this commit, none copied from prose:
    1. The three `spec=none` reserved slots now bind real specs (110, 111, 112),
       which is the flip condition this packet wrote for itself on 2026-08-04.
    2. Spec 108 is `blocked` / `full-delivery`, not `specs_hardened` /
       `product-to-planning` as the 2026-08-04 prose claimed. Its class stays
       `optional` (correct for `blocked` under Census rule 3); only the false
       status claim and its table placement were corrected.
    3. Spec 110 binds here as a NON-HOME reference only — it declares
       `releaseTrain: mvp`, so its home binding is in ../mvp/features.md.
-->
<!-- bubbles:reconciled-packet schemaVersion=1 phase=next -->

<!-- ── Delivered on train `next` (done + validate-certified) ─────────────────── -->
<!-- bubbles:feature id=next-knowledge-graph-public-api spec=specs/080-knowledge-graph-public-api delivery=required -->
<!-- bubbles:feature id=next-nats-sidecar-hardening-parity spec=specs/081-nats-python-sidecar-hardening-parity delivery=required -->
<!-- bubbles:feature id=next-target-readiness-hardening spec=specs/082-mvp-target-readiness-hardening delivery=required -->
<!-- bubbles:feature id=next-open-knowledge-reasoning-loop spec=specs/084-open-knowledge-reasoning-loop delivery=required -->
<!-- bubbles:feature id=next-open-knowledge-genuine-synthesis spec=specs/087-open-knowledge-genuine-synthesis delivery=required -->
<!-- bubbles:feature id=next-runtime-switchable-models spec=specs/088-runtime-switchable-models delivery=required -->
<!-- bubbles:feature id=next-retrieval-strategy-routing spec=specs/095-retrieval-strategy-routing delivery=required -->

<!-- ── Planning artifacts on train `next` (specs_hardened; not validate-certified) ─ -->
<!-- bubbles:feature id=next-prod-autonomous-supervisor spec=specs/079-prod-autonomous-supervisor delivery=optional -->
<!-- bubbles:feature id=next-mcp-knowledge-server spec=specs/109-mcp-knowledge-server delivery=optional -->

<!-- ── Blocked on train `next` (blocker quoted in the prose table below) ─────── -->
<!-- bubbles:feature id=next-multi-provider-model-connections spec=specs/096-multi-provider-model-connections delivery=optional -->
<!-- bubbles:feature id=next-corpus-grant-enforcement spec=specs/108-corpus-grant-enforcement delivery=optional -->

<!-- ── Formerly-reserved slots — the specs now exist and are bound (2026-08-27) ─ -->
<!-- bubbles:feature id=next-corpus-portability spec=specs/111-corpus-portability-sensitivity delivery=optional -->
<!-- bubbles:feature id=next-capability-registry spec=specs/112-capability-registry delivery=optional -->
<!-- bubbles:feature id=next-retrieval-quality-pipeline spec=specs/110-retrieval-quality-foundation delivery=optional -->
<!-- machine-binding note (Gate G101): next-retrieval-quality-pipeline is a NON-HOME
     binding. Spec 110's state.json declares `releaseTrain: mvp`, so Census rule 2
     (home phase is DERIVED from releaseTrain, never chosen) puts its HOME binding in
     ../mvp/features.md, not here. This packet retains the narrative row because the
     capability extends the delivered 095 router, exactly the double-binding Census
     rule 4 permits. The train assignment contradicts this packet's own 2026-08-04
     "why it homes to next" rationale; that contradiction is routed to bubbles.plan
     (owner of state.json) in actions.md, NOT silently corrected here. -->

Phase `next` is the promotion-candidate gate. It is backed by release train `next`
(`config/release-trains.yaml`, slot `staging`), and this packet is that train's
G101 surface.

Scope statement: this packet is the **home packet** for every spec whose
`state.json` carries `releaseTrain: next`, and for nothing else. Specs with
`releaseTrain: mvp` and the pre-train legacy estate home to
[`../mvp/features.md`](../mvp/features.md); forward-looking planned capability with
no train lives in [`../v1/features.md`](../v1/features.md).

## Carried Forward From Prior Phases

`next` promotes on top of the `mvp` gate. It deprecates nothing and re-specifies
nothing that `mvp` already owns.

| Prior-phase capability | `next` status | Where it is enforced |
|---|---|---|
| The whole `mvp` delivery set (M1a–M5d plus the pre-train legacy estate) | carry forward unchanged | [`../mvp/features.md`](../mvp/features.md) — that packet stays the enforcing surface; `next` does not re-bind it |
| MVP deferrals `m1b` / `m1c-promise-engine-full` / `m5b` | **not received by `next`** | `mvp` defers those to phase `v1`, not to `next`. See [`../v1/features.md`](../v1/features.md) § *Deferrals inherited from the MVP gate*. |
| Connector-roster lock (MVP) | still in force | `next` adds no connector. The roster lock lifts at `v1`, not here. |

The one carry-forward correction this packet makes: spec `095` was planned in the
`v1` packet as item **V7** while it was still an unauthored idea. It was
subsequently delivered on train `next`, and its `state.json` records
`releaseTrain: next`. Its **home** binding is therefore here, `required`. The `v1`
packet re-binds it under a distinct id because the v1 narrative still depends on
it — permitted by Census rule 4.

## New In This Phase

### Delivered — enforced by G101 (`delivery=required`)

Each row below binds a spec that is `done`, terminal-for-mode, non-blocked, and
carries `validate` in its certified completed phases. A regression in any of them
turns this gate red.

| Capability | Owning spec | `status` | Mode | `validate` certified |
|---|---|---|---|---|
| **Knowledge Graph Public API** — external read surface over the single knowledge graph | [`080-knowledge-graph-public-api`](../../../specs/080-knowledge-graph-public-api/) | `done` | `full-delivery` | yes |
| **NATS Python Sidecar Hardening Parity** — the ML sidecar's bus path brought to the same hardening bar as the Go core | [`081-nats-python-sidecar-hardening-parity`](../../../specs/081-nats-python-sidecar-hardening-parity/) | `done` | `full-delivery` | yes |
| **Target Readiness Hardening** — deploy-target readiness sweep for the promotion path | [`082-mvp-target-readiness-hardening`](../../../specs/082-mvp-target-readiness-hardening/) | `done` | `full-delivery` | yes |
| **Open-Knowledge Reasoning Loop** — multi-step reasoning over the corpus | [`084-open-knowledge-reasoning-loop`](../../../specs/084-open-knowledge-reasoning-loop/) | `done` | `full-delivery` | yes |
| **Open-Knowledge Genuine Synthesis** — synthesis that composes rather than concatenates, with source attribution | [`087-open-knowledge-genuine-synthesis`](../../../specs/087-open-knowledge-genuine-synthesis/) | `done` | `full-delivery` | yes |
| **Runtime-Switchable Models** — model selection changeable at runtime without a rebuild | [`088-runtime-switchable-models`](../../../specs/088-runtime-switchable-models/) | `done` | `full-delivery` | yes |
| **Retrieval-Strategy Routing + Freshness-Aware Retrieval** — intent-routed retrieval (`whole_document` / `structured_aggregate` / `vague_recall`), per-artifact-type retrieval contracts, and the evergreen-vs-ephemeral signal at the ingestion front door | [`095-retrieval-strategy-routing`](../../../specs/095-retrieval-strategy-routing/) | `done` | `full-delivery` | yes |

### Planned — authored, not delivered (`delivery=optional`, flip condition stated)

These specs are real and hardened to their mode ceiling, but no source has been
certified through `validate`. `optional` is the class the census rule assigns to a
planning artifact; it is not a softening of a delivery claim, because no delivery is
claimed.

| Capability | Owning spec | `status` | Mode | Flip condition → `required` |
|---|---|---|---|---|
| **Production Autonomous Supervisor** | [`079-prod-autonomous-supervisor`](../../../specs/079-prod-autonomous-supervisor/) | `specs_hardened` | `spec-scope-hardening` | a `full-delivery` run reaches `done` with `validate` in certified phases |
| **MCP Knowledge Server** — MCP tool surface projecting the corpus to authorized external clients | [`109-mcp-knowledge-server`](../../../specs/109-mcp-knowledge-server/) | `specs_hardened` | `product-to-planning` | same |
| **Corpus Portability & Artifact Sensitivity** — export / import / delete with artifact sensitivity classification (Product Principle 11's unconditional-exit requirement) | [`111-corpus-portability-sensitivity`](../../../specs/111-corpus-portability-sensitivity/) | `in_progress` | `product-to-planning` | same |
| **Capability Registry** — one runtime registry projected to the assistant, navigation, slash commands, and the MCP tool surface | [`112-capability-registry`](../../../specs/112-capability-registry/) | `not_started` | `product-to-planning` | same |

> `not_started` is not a row in the Census rule 3 table in [`../README.md`](../README.md).
> It is classed `optional` here for the same reason `in_progress` is: `optional`
> asserts no delivery, so it cannot overstate a spec that has not started. The
> missing vocabulary row is recorded in that README rather than invented here.

### Blocked — in-repo work complete, external blocker recorded (`delivery=optional`)

| Capability | Owning spec | `status` | Recorded blocker (quoted from `state.json.blockedReason`) |
|---|---|---|---|
| **Multi-Provider AI Model Connections** | [`096-multi-provider-model-connections`](../../../specs/096-multi-provider-model-connections/) | `blocked` | "VALIDATED-IN-REPO; blocked on the owner-directed `bubbles.devops` self-hosted handoff (same cohort as dependencies 084/087/088)." Two external blockers: Gate G089 requires dependency `088` to be `done` from *its* owner-directed terminal status, and spec 096's own live hosted-provider `e2e-api` legs are self-hosted-gated. |
| **Corpus Grant Enforcement** — per-client, audited remote-egress grants over the corpus (Product Principle 11) | [`108-corpus-grant-enforcement`](../../../specs/108-corpus-grant-enforcement/) | `blocked` | "SCOPE-04 holds three operator-owned, time-bound DoD items that no code can satisfy: (1) >= 14 consecutive OBSERVE days with the stage resolved from SST at process start; (2) proactive rotation of every principal whose gr…" — certified `blocked` by `bubbles.validate`. |

> **Corrected 2026-08-27.** The 2026-08-04 packet listed spec 108 in the *Planned*
> table above as `specs_hardened` / `product-to-planning`. Reading
> `specs/108-corpus-grant-enforcement/state.json` at `HEAD` `8a599234` shows
> `status: blocked`, `workflowMode: full-delivery`, `certification.certifiedBy:
> bubbles.validate`. The engineering is built and green; what is outstanding is an
> operator-owned observation window, so the grant model **exists and does not yet
> deny**. Both halves of that are true and neither alone is. The `delivery=optional`
> class did not change — Census rule 3 assigns `optional` to `blocked` when no later
> phase receives the capability, which is the case here.

`optional` — not `deferred-to:` — is correct for both rows. There is no later phase
that receives these capabilities: `next` still owns them, and the discharge path is
the recorded operator/`bubbles.devops` handoff, not a hand-off to another gate.
Census rule 3 assigns `optional` + quoted blocker exactly for that shape.

### Formerly-reserved slots — now bound to real specs (reconciled 2026-08-27)

On 2026-08-04 three capabilities were being specced concurrently with this packet.
They had no spec directory and therefore no number, so they were bound `spec=none`
rather than to a guessed number — a guessed number would have created the same
dangling reference that the retired `deferred-to:release-v1` token created. The
flip condition this packet wrote for itself was: *replace `spec=none` with the real
`specs/NNN-…` path once the spec directory exists, then apply census rule 3 to its
`state.json`.* All three directories now exist, so that condition is discharged here.

| Reserved id | Now binds | Observed `status` | Home phase (Census rule 2) | Class |
|---|---|---|---|---|
| `next-corpus-portability` | [`111-corpus-portability-sensitivity`](../../../specs/111-corpus-portability-sensitivity/) | `in_progress` | `next` (`releaseTrain: next`) — **home is here** | `optional` |
| `next-capability-registry` | [`112-capability-registry`](../../../specs/112-capability-registry/) | `not_started` | `next` (`releaseTrain: next`) — **home is here** | `optional` |
| `next-retrieval-quality-pipeline` | [`110-retrieval-quality-foundation`](../../../specs/110-retrieval-quality-foundation/) | `not_started` | **`mvp`** (`releaseTrain: mvp`) — home is [`../mvp/features.md`](../mvp/features.md) | `optional` (non-home reference) |

**The 110 placement prediction did not survive authoring, and that is recorded
rather than papered over.** This packet argued on 2026-08-04 that retrieval quality
"homes to `next`" because it directly extends the delivered `095` router. The spec
was subsequently authored as `110-retrieval-quality-foundation` with
`releaseTrain: mvp`. Census rule 2 is explicit that the home phase is *derived, never
chosen*, so the derivation wins over this packet's rationale: 110's **home binding
lives in `mvp`**. The row is retained here as a non-home reference because the `next`
narrative genuinely depends on it — the double binding is what Census rule 4 permits.

Which of the two is wrong — the train field or the rationale — is **not**
`bubbles.releases`' call: `state.json` belongs to `bubbles.plan`. The contradiction is
routed, not corrected. See [`actions.md`](actions.md).

`next-capability-registry` is a **runtime** registry projecting live capability to
four surfaces. It is related to, but distinct from, the `v1` item **V4-A**, which is
a documentation generator that emits `docs/Capability_Map.md`. Do not merge the two
rows: one is a runtime projection, the other a managed-doc artifact. If spec 112
ends up subsuming V4-A, the `v1` packet's V4-A row must be re-pointed at it in the
same change, not silently dropped.

## Plan-to-Release Traceability

| `next` item | Owning artifact | Dispatch state |
|---|---|---|
| `080`, `081`, `082`, `084`, `087`, `088`, `095` | their spec directories | delivered, validate-certified, G101-enforced |
| `079`, `108`, `109` | their spec directories | planning packets authored; awaiting a `full-delivery` dispatch |
| `096` | its spec directory | in-repo complete; awaiting the `bubbles.devops` self-hosted handoff |
| retrieval quality / corpus portability / capability registry | **no spec directory yet** | authoring in flight; slot reserved above, number deliberately unassigned |

## Capability evidence trace

Every `required` row above was verified at HEAD `8d971420` by reading the bound
spec's `state.json` directly — `status`, `workflowMode`, and the union of
`certification.certifiedCompletedPhases` and `completedPhases`. No row restates a
claim from another document.

No capability in the *planned*, *blocked*, or *reserved* groups is described as
delivered anywhere in this packet. Where a capability's owning spec is `blocked`,
the recorded blocker is quoted rather than summarised, so the reason survives
review.

## Sequencing / dependencies

- `096` sits downstream of the `088` → `087` → `084` cohort through Gate G089. It cannot reach `done` before that cohort's owner-directed handoff completes, regardless of its own in-repo state.
- `109` (MCP knowledge server) depends on `108` (corpus grant enforcement) for its egress-authorisation model: an MCP client reading the corpus is exactly the grant `108` governs. Delivering `109` before `108` would ship a read surface with no audited grant behind it.
- `next-retrieval-quality-pipeline` depends on delivered `095`: the evaluation gate measures the router `095` shipped.
- `next-capability-registry` should land before, or with, `109`, so the MCP tool list is a projection of the registry rather than a second hand-maintained list.
- `next-corpus-portability` is independent of the rest of the train and may dispatch in parallel.

## Deprecations in this phase

None. `next` promotes on top of `mvp` and removes no capability.
