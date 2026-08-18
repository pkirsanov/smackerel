# Actions — Smackerel `next`

Action items required to close phase `next`, grouped by owner. Every row states who
owns it. `bubbles.releases` owns this packet and nothing else; a row targeting a
spec, a train config, or a deploy adapter is a **route**, not work done here.

## Engineering

| ID | Action | Target | Owner | Priority |
|----|--------|--------|-------|----------|
| ENG-N1 | Dispatch a `full-delivery` run for [`079-prod-autonomous-supervisor`](../../../specs/079-prod-autonomous-supervisor/) so its `optional` binding can flip to `required` | `specs/079-…` | `bubbles.workflow` | P2 |
| ENG-N2 | Dispatch a `full-delivery` run for [`108-corpus-grant-enforcement`](../../../specs/108-corpus-grant-enforcement/). It gates ENG-N3. | `specs/108-…` | `bubbles.workflow` | P1 |
| ENG-N3 | Dispatch a `full-delivery` run for [`109-mcp-knowledge-server`](../../../specs/109-mcp-knowledge-server/) **after** ENG-N2 — an MCP read surface without an audited grant behind it violates Product Principle 11 | `specs/109-…` | `bubbles.workflow` | P1 (blocked on ENG-N2) |
| ENG-N4 | When the retrieval-quality spec directory is created, replace the reserved `spec=none` binding `next-retrieval-quality-pipeline` in [`features.md`](features.md) with the real path and re-derive its class from the new `state.json` | `docs/releases/next/features.md` | `bubbles.releases` | P1 — at spec creation |
| ENG-N5 | Same for `next-corpus-portability` | `docs/releases/next/features.md` | `bubbles.releases` | P1 — at spec creation |
| ENG-N6 | Same for `next-capability-registry`. In the same change, decide whether it subsumes the `v1` item **V4-A** (capability-map generator) and re-point the V4-A row rather than dropping it. | `docs/releases/next/features.md` + [`../v1/features.md`](../v1/features.md) | `bubbles.releases` | P1 — at spec creation |

## Ops / DevOps

| ID | Action | Target | Owner | Priority |
|----|--------|--------|-------|----------|
| OPS-N1 | Run the owner-directed self-hosted handoff that certifies the `088` → `087` → `084` cohort, clearing Gate G089 for [`096-multi-provider-model-connections`](../../../specs/096-multi-provider-model-connections/), then execute spec 096's C7 live hosted-provider `e2e-api` legs on the target. This is the sole discharge path recorded in 096's `blockedReason`. | deploy target | `bubbles.devops` | P1 — sole blocker for 096 |
| OPS-N2 | Validate this packet's [`deployment.md`](deployment.md) technical claims (promotion path, digest pinning, rollback shape) before any external claim is made from it | this packet | `bubbles.devops` | P0 before external claim |
| OPS-N3 | Wire `bash .github/bubbles/scripts/release-delivery-reconciliation-guard.sh --repo-root "$(pwd)" --phase next --require-coverage` into the same pre-push/CI position that already runs the `mvp` phase, so a `next` regression is caught mechanically rather than at review | pre-push / CI wiring | `bubbles.devops` | P1 |

## Routed defects (found while reconciling this packet — NOT fixed here)

| ID | Finding | Owner | Why not fixed here |
|----|---------|-------|--------------------|
| RTE-N1 | `specs/039-recommendations-engine/state.json` records `certifiedCompletedPhases` as a **mixed array** of strings and objects. The G101 parse (`.[]? \| ascii_downcase`) aborts at the first object, and in 039's case every string element before that point excludes `validate` — so a genuinely validate-certified spec is invisible to the guard. Specs `038` and `040` share the mixed shape but happen to emit `validate` before the first object, so they pass by ordering luck, not by correctness. | `bubbles.plan` (owns `state.json`) | `bubbles.releases` does not edit any spec's `state.json`. Recorded here and in [`../mvp/actions.md`](../mvp/actions.md) so the `carried` class on 039 has a written reason. |
| RTE-N2 | The guard's `is_validate_certified` treats a parse abort as "not certified" rather than as a malformed-input error. That is fail-safe in direction but silent in kind: a data-shape defect is reported as a delivery defect. Consider surfacing a distinct diagnostic. | Bubbles framework (`bubbles.setup` to route upstream) | The guard is a framework-managed install artifact; it is not patched downstream. |

## Owner decisions still pending

| OQ-ID | Question | Affects | Suggested default |
|-------|----------|---------|-------------------|
| OQ-N1 | Does phase `next` close when its seven delivered members are promoted, or does it stay open until `108`/`109` deliver? | phase-close criteria | Close on promotion of the delivered seven. `108`/`109` are authored plans; holding the gate open for them turns a promotion train into an open-ended backlog. |
| OQ-N2 | Should the three reserved capabilities (retrieval quality, corpus portability, capability registry) home to `next` or to a phase after it? | reserved slots | `next`, as recorded. All three extend specs already on this train. Revisit only if `next` is promoted before they are specced. |
| OQ-N3 | `096` is `blocked` on an owner-directed handoff, not on engineering. If that handoff does not happen before promotion, does `096` move to a later phase or stay on `next` as an unpromoted member? | `096` | Stay on `next`. Moving it would require a receiving phase directory to exist, and inventing one to park a blocked spec is how dangling `deferred-to:` tokens get created. |

## Cross-product coordination

| ID | Action | Counterparty |
|----|--------|--------------|
| XP-N1 | The knowledge-graph public API (`080`) and the planned MCP server (`109`) are both read surfaces over the corpus. Neither may expose a write path into the QF Companion boundary, and neither may modify QF decision-packet metadata it carries (Product Principle 10). | QF Companion repo |
| XP-N2 | If corpus-portability export formats end up carrying `PersonalEvidenceBundle`-shaped data, the QF-side schema must be updated **first** per Principle 10's cross-product ordering rule. | QF Companion repo |

## Explicitly NOT taken on in this packet

- No spec was created. Every capability without a spec is bound `spec=none` with its flip condition written down.
- No spec `state.json` was edited, including the 039 shape defect (routed as RTE-N1).
- `config/release-trains.yaml` and `config/feature-flags.*.yaml` were not touched — `bubbles.train` owns them.
- No deployment-target adapter, manifest, or concrete host binding appears anywhere in this packet. Those live in the knb deploy-adapter overlay.
