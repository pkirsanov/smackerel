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
| OPS-N3 | **DELIVERED** in `088cdef8`. Added `./smackerel.sh release reconcile`: it auto-discovers every phase packet under `docs/releases/*/features.md` and reconciles all of them, aggregating results so one failing phase cannot mask another, and failing loud if discovery finds nothing rather than reporting a vacuous pass (`--phase <name>` narrows to one; no `--skip`/`--force`/`--ignore` bypass exists). G110 (`release-train-guard.sh`) and the aggregate G101 reconcile are now wired into `scripts/git-hooks/pre-push` (both blocking; the pre-existing knb blocks were left untouched) and into a standalone `release-schema` job in `.github/workflows/ci.yml` that is independent of build/test/stack and verifies `jq`/`yq` are present instead of letting a missing parser silently weaken the guard. **Premise corrected — see the note below this table.** | pre-push / CI wiring | `bubbles.devops` | Done — `088cdef8` |

**OPS-N3 premise correction (recorded, not dropped).** As written, OPS-N3 asked to wire
the `next` phase into "the same pre-push/CI position that already runs the `mvp` phase".
No such position existed. Re-verified against `HEAD~1` while recording this row:
`release-delivery-reconciliation-guard` matched **nowhere** in `scripts/`,
`.github/workflows/` or `smackerel.sh`, and `release-train-guard` matched at exactly one
site — `smackerel.sh:2860`, the body of the manual `./smackerel.sh release guard`
subcommand. So the real gap was not that `next` was missing from an existing lane: it was
that **neither release-axis gate had any mechanical enforcement, for any phase**. A row
that had been read as "extend the coverage" was in fact "there is no coverage".

The wiring was proven non-vacuous rather than merely observed green: flipping
`m5d-spec-banner-sweep` back to the unsatisfiable `delivery=required` made
`./smackerel.sh release reconcile` exit 1 and print `FAIL  mvp (exit 1)` while still
evaluating `next` and `v1`; reverting restored exit 0. That adversarial step is the
difference between a gate and a decoration, and it is why aggregation-not-short-circuit
is a stated property above.

## Routed defects (found while reconciling this packet — NOT fixed here)

| ID | Finding | Owner | Why not fixed here |
|----|---------|-------|--------------------|
| RTE-N1 | `specs/039-recommendations-engine/state.json` records `certifiedCompletedPhases` as a **mixed array** of strings and objects. The G101 parse (`.[]? \| ascii_downcase`) aborts at the first object, and in 039's case every string element before that point excludes `validate` — so a genuinely validate-certified spec is invisible to the guard. Specs `038` and `040` share the mixed shape but happen to emit `validate` before the first object, so they pass by ordering luck, not by correctness. | `bubbles.plan` (owns `state.json`) | `bubbles.releases` does not edit any spec's `state.json`. Recorded here and in [`../mvp/actions.md`](../mvp/actions.md) so the `carried` class on 039 has a written reason. |
| RTE-N2 | The guard's `is_validate_certified` treats a parse abort as "not certified" rather than as a malformed-input error. That is fail-safe in direction but silent in kind: a data-shape defect is reported as a delivery defect. Consider surfacing a distinct diagnostic. | Bubbles framework (`bubbles.setup` to route upstream) | The guard is a framework-managed install artifact; it is not patched downstream. |
| RTE-N3 | **Gate G110 and the knb product-deployment-boundary lint contradict each other, and `config/release-trains.yaml` cannot satisfy both.** `.github/bubbles/scripts/release-train-guard.sh:66` explicitly **accepts** the concrete deploy-target slot named at `config/release-trains.yaml:22` as a legal `target_slot`, and its rejection message on the next line enumerates the same allowed set including that value. knb's `config/product-deployment-boundary.tokens:11` rule `PRODUCT_CONCRETE_TARGET` **bans** that exact literal anywhere in a product repo. The train config therefore declares the slot to satisfy G110 and is, for precisely that reason, reported as an `E-PDB-001 PRODUCT_CONCRETE_TARGET` violation by a lint that is fail-closed and blocking in pre-push and CI. Satisfying either gate necessarily violates the other; no downstream edit to the train config can satisfy both. | Three-way: `bubbles.train` owns the slot value in `config/release-trains.yaml`; the Bubbles framework owns the G110 accepted-slot vocabulary (route via `bubbles.setup`); knb owns the token registry and its allowlist. | `bubbles.releases` owns none of the three artifacts. `config/release-trains.yaml` belongs to `bubbles.train`, `release-train-guard.sh` is a framework-managed install artifact that is never patched downstream, and the token registry is knb-owned and deliberately not duplicated into product repos. Resolving this requires one of the three owners to move — either G110 drops the concrete slot from its vocabulary, or knb allowlists the train-config declaration site. |

**Boundary-lint state at the time of this record.** Re-measured on 2026-08-18 with
`bash "$KNB_REPO_ROOT/scripts/lint/product-deployment-boundary.sh" --repo "$(pwd)"`:
**11 findings, exit 1, all `E-PDB-001 PRODUCT_CONCRETE_TARGET`, all pre-existing** —
`config/release-trains.yaml`, `docs/Delivery_Position_2026-08-10.md`,
`internal/config/release_trains_contract_test.go`,
`specs/080-knowledge-graph-public-api/bugs/BUG-080-001-graph-api-fail-soft-runtime-disable/report.md`,
`specs/108-corpus-grant-enforcement/scopes.md`,
`specs/110-retrieval-quality-foundation/{spec.md,state.json}`,
`specs/111-corpus-portability-sensitivity/state.json`,
`specs/112-capability-registry/{spec.md,state.json}`, and
`tests/integration/corpus_grant_env_emission_test.go`. No `docs/releases/**` path is
among them, and none is inside `bubbles.releases`' ownership. They are routed under
RTE-N3, not fixed here and not silently absorbed into this packet's baseline.

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
