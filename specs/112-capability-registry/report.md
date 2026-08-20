# Report: 112 Capability Registry

**Status:** `not_started` · **Workflow mode:** `product-to-planning` · **Release train:** `next`
**Owner of this artifact:** the delivery agents
**Created by:** `bubbles.analyst` as an honest initial artifact so the packet is structurally complete

---

## Summary

This packet was **authored, not executed.** `bubbles.analyst` created the requirements in
[`spec.md`](spec.md) from diagnostic findings `D17` and `D18`, governed by Pillar C of
[`docs/Product_Delivery_Plan.md`](../../docs/Product_Delivery_Plan.md), and re-verified
every current-state claim against the working tree on 2026-08-04 before writing a single
requirement.

**No source file was changed. No feature work was performed. No test was run.**

What this run produced:

- `spec.md` — problem statement, outcome contract, a nineteen-row verified evidence base, a
  domain capability model with nine binding policies, five actors, five use cases, 26
  Gherkin scenarios with stable `SCN-112-*` ids, 33 requirements, six non-functional
  requirements, seven non-goals, ten routed open findings, product principle alignment, the
  release-train declaration, and an exposure contract.
- `design.md`, `scopes.md`, `uservalidation.md`, `state.json` and this file — honest initial
  artifacts. Each states in its own opening section that it carries no decision, no ratified
  plan, and no evidence.

**Artifact ownership deviation, recorded rather than hidden.** `bubbles.analyst` owns
`spec.md`. `design.md` is owned by `bubbles.design`, `scopes.md` by `bubbles.plan`, and
`report.md` and `uservalidation.md` by the delivery agents. They were created in this run
under an explicit operator instruction to produce the full initial artifact set so the
packet is structurally complete and lintable. Each carries a header naming its real owner
and stating that its contents are provisional. The same record appears in
`state.json.executionHistory`.

## Source Verification

Every factual claim supplied in the authoring brief was re-read against the working tree
before it was written into `spec.md`. **All claims hold**, and two were sharpened by
evidence the brief did not contain.

| Brief claim | Outcome |
|---|---|
| 27 scenario contracts built under `config/prompt_contracts/`, only 5 `user_facing: true` in `config/assistant/scenarios.yaml` — `retrieval_qa`, `weather_query`, `notification_schedule`, `recipe_search`, `open_knowledge` | **Holds exactly.** 27 files present; 5 occurrences of `user_facing: true` at `scenarios.yaml:26,34,42,56,72`; the five ids match the brief. Recorded as E1, E2. |
| Reachability is split across four hand-maintained inventories that can and do drift | **Holds, and the drift is now demonstrated rather than predicted** — see the sharpening below. Recorded as E11–E16. |
| 31 PWA pages exist, only 2 load `web/pwa/lib/appnav.js` | **Holds exactly.** 31 first-party pages once vendored `node_modules` and the generated `playwright-report` are excluded; `assistant.html` and `index.html` are the two adopters. Recorded as E17. |
| The daily digest is absent from the guaranteed cross-surface navigation core | **Holds as stated, with a precision note.** Absent from the shared partial (`appshell.go:31`) and from the PWA navigation (`appnav.js:22`); present in the server page shell, which appends it after including the shared partial (`templates.go:79-84`). `D18` is a core-membership finding, not an unreachability claim, and its own caveat says so. Recorded as E11, E12, E14. |
| The registry is HALF-BUILT, not missing: `internal/experience/` already contains `catalog.gen.json` (20 surfaces, schema `smackerel-product-experience/v1`) plus the navigation, renderer, consumer-inventory, state and mutation projections and a validator | **Holds exactly, and is stronger than stated.** All named files present; 20 surfaces; schema string matches. Recorded as E6, E8, E10. |

### Two facts found that the brief did not state, both strengthening it

1. **The capability reference already exists and points at nothing.** Every one of the 20
   catalog surfaces carries a `capability_id`, and the validator already emits an
   `unknown-capability` violation — but there is no capability record anywhere. In the
   single source of truth, `capability_id` appears only as an attribute of a surface
   (`config/smackerel.yaml:2706-2934`); there is no `capabilities:` block. Worse, the only
   production-shaped test supplies the capability universe with `iCapsOf(cat)`, which builds
   the expected set **from the catalog being validated**, so the membership check is
   circular and cannot fail (`tests/integration/experience/route_inventory_test.go:92-100`).
   This makes the half-built framing precise: the catalog holds a foreign key to a table
   that does not exist. Recorded as E7, E8, E9.

2. **The drift is not hypothetical — three live contradictions were found.** (a) The PWA
   navigation comment asserts it "mirrors the server-side `app-shell-nav` partial"; the two
   have already diverged, with `capture`, `connectors` and `photos` only in the PWA list and
   `knowledge` only in the server partial. (b) The slash table maps `/ask` to
   `open_knowledge` while the assistant registry assigns `/ask` to `retrieval_qa` — the same
   token names two different targets in two files. (c) The slash table ships `/recipe` and
   `/cook`, while the registry records `recipe_search` with an empty shortcut and a comment
   that the set "stays frozen at `/ask`, `/weather`, `/remind`, `/reset`". Recorded as E13,
   E15, E16.

### Sharpening of the unsurfaced count: 11, not 22

The brief stated that 22 working capabilities have no front door, following `D17` and the
plan of record, both of which reach 22 as `27 − 5`. **The arithmetic is correct but the
units are mixed**, and the evidence shows why.

Ten of the 27 contracts declare a pipeline type (`domain-extraction` ×3, `query-augment`,
`lint-audit`, `ingest-synthesis`, `drive-folder-context`, `drive-classification`,
`digest-assembly`, `cross-source-connection`) and **carry no routable `id:` at all**. They
are invoked directly by the service that owns them and were never dispatchable. One
further contract, `e2e_ollama_smoke`, is a test harness by its own description. Only the 17
contracts declaring `type: "scenario"` carry a routable id.

The genuinely unsurfaced dispatchable set is therefore **11**, not 22. This narrows the
count and strengthens the finding: the important defect was never the size of the number,
it is that **the repository cannot tell you which number is right**, because nothing records
whether a contract is unsurfaced deliberately or by omission. `spec.md` §3 carries the
full derivation and the eleven ids; R-112-13 through R-112-18 make the classification a
declared fact rather than an inference.

## Unverifiable Premise

The authoring brief noted that `.specify/templates/spec-template.md` is expected to govern
spec structure. **That file does not exist in this repository.** `.specify/` contains only
`memory/`, `metrics/` and `runtime/`; there is no `templates/` directory. This spec
therefore follows the structure established by the adjacent sibling spec in this repository
together with the Bubbles BDD scenario contract, and the discrepancy is recorded as finding
`F-112-TEMPLATE-01` rather than being silently ignored or falsely claimed as satisfied.

## Findings

Ten findings are recorded in [`spec.md`](spec.md) §11 with severity and owner. Four are
BLOCKING. Each is carried onto the specific scope it blocks in [`scopes.md`](scopes.md)
rather than left for discovery during execution.

| ID | Severity | Owner |
|---|---|---|
| `F-112-FLAG-01` | BLOCKING | `bubbles.train` |
| `F-112-108-01` | BLOCKING | `bubbles.plan` |
| `F-112-UNIT-01` | BLOCKING | `bubbles.design` |
| `F-112-UNIVERSE-01` | BLOCKING | `bubbles.design` |
| `F-112-109-01` | HIGH | `bubbles.design` (with spec 109) |
| `F-112-CUTOVER-01` | HIGH | `bubbles.plan` |
| `F-112-ALIAS-01` | HIGH | `bubbles.design` |
| `F-112-ISLANDS-01` | MEDIUM | `bubbles.design` (with the shell spec) |
| `F-112-TEMPLATE-01` | LOW | operator via `bubbles.plan` |
| `F-112-110-01` | LOW — **RESOLVED BY FACT** (spec 110 now has a `state.json` declaring `releaseTrain: mvp`; the reported gap has closed) | `bubbles.plan` |

## Test Evidence

**None. No test was executed in this run, and no test evidence is claimed.**

This section is deliberately empty of results. The packet is at `not_started`; no
implementation exists, so there is nothing to test. The E1–E19 rows in `spec.md` §3 are
**current-state source observations** — statements about what the repository contains
today, each carrying a file and, where applicable, a line reference — and they are
explicitly **not** test results. They establish that the described defects exist. They
prove nothing about a fix.

Test evidence is added when the scopes in [`scopes.md`](scopes.md) execute under a
delivery-capable workflow mode and real commands produce real output. Writing anything
here now would be fabrication.

## Completion Statement

**This feature is NOT complete. It has not started.**

Requirements authoring finished; nothing else ran. `state.json` status is `not_started` and
`certification.status` matches. No scope is complete, no Definition of Done item is checked,
and no evidence exists anywhere in this packet.

The declared `statusCeiling` for workflow mode `product-to-planning` is `specs_hardened`.
**This packet has not reached that ceiling**, because the `bubbles.design` and `bubbles.plan`
passes have not run under their own owners, and four BLOCKING findings gate that work.
Promotion to `done` is forbidden from this mode and requires a delivery-capable mode plus a
full evidence chain across the nine scopes.
