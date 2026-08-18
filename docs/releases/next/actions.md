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
| RTE-N3 — **RESOLVED in `3b263562`** (with knb `be18236a`) | **Original diagnosis, kept because half of it is still live.** Gate G110 and the knb product-deployment-boundary lint contradicted each other, and `config/release-trains.yaml` could not satisfy both. `.github/bubbles/scripts/release-train-guard.sh:66` explicitly **accepts** a concrete operator-target slot name as a legal `target_slot`, and its rejection message on the next line enumerates the same allowed set including that value. knb's `config/product-deployment-boundary.tokens:11` rule `PRODUCT_CONCRETE_TARGET` **bans** that exact literal anywhere in a product repo. `config/release-trains.yaml:22` declared that slot to satisfy G110 and was, for precisely that reason, reported as an `E-PDB-001 PRODUCT_CONCRETE_TARGET` violation by a lint that is fail-closed and blocking in pre-push and CI. **HOW IT WAS RESOLVED — the product moved, not the framework.** `3b263562` changed the `mvp` train to the abstract slot `target_slot: prod` (now `config/release-trains.yaml:26`), which is in G110's vocabulary and is not a concrete operator-target name; the concrete host binding stays in the knb overlay. `internal/config/release_trains_contract_test.go` was then made **deliberately stricter than G110**: the concrete slot was dropped from `validTargetSlots` (now `prod`/`staging`/`none`), and a new `TestReleaseTrainsContract_RejectsConcreteOperatorTargetSlot` asserts both that the contract rejects the token and that the live config does not carry it — assembling the token at runtime, because writing it as a literal would trip the very lint the test defends. So the product cannot re-adopt the slot without failing its own test first. **No deployment-behaviour change:** nothing under the knb smackerel adapter consumes `target_slot`, and its only readers are `smackerel.sh:2945` (the `./smackerel.sh release status` display) and that contract test. **THE FRAMEWORK WART IS NOT FIXED.** `release-train-guard.sh:66` still admits the concrete slot in its G110 vocabulary, so any other product repo can still walk into the same contradiction. What changed is narrower and should not be overstated: smackerel no longer depends on that vocabulary entry. | Framework half **still open** — the Bubbles framework owns the G110 accepted-slot vocabulary (route via `bubbles.setup`). Product half closed by `bubbles.train` (`3b263562`); registry half closed by knb (`be18236a`). | Still not fixed here — `bubbles.releases` owns none of the three artifacts, and it did not author `3b263562`. This row is updated, not deleted, because the finding is the reason the boundary-lint note beneath it changed and because the framework half remains a live route for the next repo to hit. |
| RTE-N4 | **Gate G136 arrived with a 352-file acceptance-shape debt across the spec portfolio.** The v7.28.0 framework install carries Gate **G136** (Human Acceptance Terminal Gate) as redefined by **IMP-047 PD-12** and governed by `.github/bubbles/registry/acceptance-authority.yaml`. (The gate id itself is older — the registry records `since: 7.26.0`, BUG-029 / IMP-040 SCOPE-10 — but PD-12 changed *what it requires*, and that redefinition is what this install carries.) PD-12 splits `uservalidation.md` into **AUTOMATION READINESS** (`## Automation Readiness`; automation writes it and it grants no acceptance) and **HUMAN ACCEPTANCE** (`## Checklist`, now shipping *unchecked*, plus `## Human Acceptance Record`). At a transition whose resolved target status is `done`, `guards/tail-delegated-gates.sh` Check 43 requires an authored `## Human Acceptance Record` carrying `acceptedBy` (never an id matching `^bubbles\.`), `acceptedAt` and `method`. **Measured 2026-08-18 in this repo:** **353** `uservalidation.md` files exist under `specs/`; **352 carry the pre-PD-12 shape** — neither new heading present — the single migrated packet being `specs/080-knowledge-graph-public-api/bugs/BUG-080-001-graph-api-fail-soft-runtime-disable/`. **33** packets carrying a `uservalidation.md` are non-terminal-for-mode. Of those, **31** run a mode whose ceiling is `done` (22 `bugfix-fastlane`, 8 `full-delivery`, 1 `stabilize-to-doc`) and will meet G136 at their terminal transition; the other **2** run `product-to-planning`, whose ceiling is `specs_hardened`, and Check 43 passes explicitly when the target status is not `done`, so they do not meet it at their own ceiling. Three `not_started` specs already carry checked acceptance items: `110-retrieval-quality-foundation` (24), `111-corpus-portability-sensitivity` (34), `112-capability-registry` (44) — **102** items, zero unchecked. **The nuance changes the remedy.** Inspection of 110 shows those items are planning-review agreements — "The stated problem is real", "It is correct that no chunk table exists anywhere in the migration set" — and the file's own header states that at this point the checklist "validates the **requirements**, not a running system". Under PD-12 a checked `## Checklist` item means a human accepted *delivered behaviour*, so two different facts now occupy one section. They were deliberately **not** unchecked: a blanket uncheck would destroy a genuine human's recorded planning agreement, and G136 fires only at a `done` transition, so no `not_started` packet is blocked today. The debt materialises one packet at a time, as each attempts to go terminal. | `bubbles.plan` owns `uservalidation.md` shape across the portfolio. The Bubbles framework owns the migration story for the 352 artifacts that predate the contract (route via `bubbles.setup`). | `bubbles.releases` owns no `specs/**` artifact. This is a portfolio-wide migration, not a release-packet defect, and migrating one packet here would understate the scale rather than reduce it. Recorded so the debt is routed rather than discovered at the next terminal transition. |

**Boundary-lint state — re-measured 2026-08-18, after `3b263562` and knb `be18236a`.**
This note previously recorded **11 findings, exit 1**, all `E-PDB-001 PRODUCT_CONCRETE_TARGET`,
all pre-existing. Re-run the same way with
`bash "${KNB_REPO_ROOT:-$HOME/knb}/scripts/lint/product-deployment-boundary.sh" --repo "$(pwd)"`:

```
product-deployment-boundary: PASS (product repo contains only generic deployment surfaces)
```

**0 findings, exit 0.** Where the eleven went: **nine paths were fixed at source** in
`3b263562` and now carry zero occurrences of the banned token —
`config/release-trains.yaml`, `docs/Delivery_Position_2026-08-10.md`,
`internal/config/release_trains_contract_test.go`,
`specs/110-retrieval-quality-foundation/{spec.md,state.json}`,
`specs/111-corpus-portability-sensitivity/state.json`,
`specs/112-capability-registry/{spec.md,state.json}`, and
`tests/integration/corpus_grant_env_emission_test.go`. **Two paths retain exactly one
occurrence each and were deliberately not edited:**
`specs/080-knowledge-graph-public-api/bugs/BUG-080-001-graph-api-fail-soft-runtime-disable/report.md`
and `specs/108-corpus-grant-enforcement/scopes.md`, each carrying a single captured
`grep: config/generated/<target>.env: Permission denied` transcript line emitted by a real
command and recorded as spec evidence. Rewriting recorded evidence to satisfy a lint is
tampering, so the exception went to the owner registry instead: knb `be18236a` allowlists
those **two exact file paths** against the **single** `PRODUCT_CONCRETE_TARGET` rule,
anchored per-file rather than as a `specs/**` prefix, so a new spec inherits no exemption
and newly authored prose must still use a placeholder. (`specs/108-…/scopes.md` appears in
both sets: `3b263562` fixed its authored prose; the allowlist covers only its residual
transcript line.) No `docs/releases/**` path was among the original eleven and none is a
finding now.

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
