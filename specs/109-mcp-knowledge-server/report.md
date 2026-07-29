# Report: 109 MCP Knowledge Server

**Mode:** `product-to-planning` · **Ceiling:** `specs_hardened` · **Planning only — no source edited.**
**Status:** `specs_hardened` · **Release train:** `next` · **Flag:** `mcpKnowledgeServer`

---

## Summary

This packet took spec 109 from a commissioning brief to a hardened, executable plan. No MCP server
exists in the repository (`spec.md` §1), so **no implementation was attempted, no test was run, and
no execution evidence is claimed**. Every statement below is planning-phase evidence: an artifact
that exists, a decision that was recorded, or a finding that was routed.

The delivered plan is **seven scopes** in dependency order, carrying **43 Test Plan rows** — each
with a matching DoD item — plus **14 standing regression DoD items** (two per scope), for **57 test
DoD items** in total. Sixteen of the eighteen tracked scenarios are planned; two
(`SCN-109-013`, `SCN-109-014`) are deliberately deferred with the `memory-write` toolset, which
decision D5 sets off. One scope (**Scope 06 — `hospitality-read`**) is recorded **Blocked** on
BUG-019-003 rather than planned as deliverable.

---

## Agents That Ran, And What Each Produced

| Order | Agent | Produced | Key output |
|---|---|---|---|
| 1 | `bubbles.analyst` | `spec.md` (1133 lines) | Problem statement, the §2 Outcome Contract, decisions of record D1–D5, actors, JTBD J1–J4, use cases UC-109-001…008, fifteen §7 Gherkin scenarios (SCN-109-001…015), the §8 capability/toolset model, the §9 Projection Contract, §10 authorization, §11 audit/observability, §12 failure semantics, §13 non-goals, blocking findings F-109-001…006, §15 documentation/config requirements, §16 principle alignment, §17 release train, §18 operator review gate. |
| 2 | `bubbles.ux` | `spec.md` §9A (Non-UI UX Contract) | The seven-token closed status vocabulary (§9A.1) and the banned-word list; the fixed four-field refusal envelope (§9A.2) and the R-109-UX2…UX7 shape rules; operator flows OF-1…OF-5 (§9A.3); the agent-facing discovery Cases A/B/C (§9A.4); the honest-degradation surface R-109-UX15…UX18 (§9A.5); the failure-honesty mapping R-109-UX19…UX23 (§9A.6); and five routed findings UX-F-001…UX-F-005 (§9A.7). Ships **no screens** — the surface is workflow behavior, status language, refusal shape, and exception handling. |
| 3 | `bubbles.design` | `design.md` (635 lines) | Architecture placement (`/mcp` on the existing `smackerel-core` listener, sibling integration capability); the six-package `internal/mcp/` layout with an explicit non-inheritance rule against `internal/agent` and `internal/assistant/openknowledge`; the `Descriptor` schema and why four orthogonal hints cannot come from a three-value `SideEffectClass`; **the §4 resolution of UX-F-001** (MCP owns its corpus grant as a third `Grant` kind) with four independent reasons and the recorded rejected alternative; the fixed five-gate order and why egress is gate 4; the spec-095 `Executor` seam and its three-part F-109-002 mitigation; the structural projection enforcement (a type that cannot express raw content); the deferred-but-designed write plane over `confirm.Machine`; and the twelve-test adversarial design T1…T12 plus the Complexity Tracking table. |
| 4 | `bubbles.plan` | `scopes.md`, `report.md`, `uservalidation.md`, `state.json`, `scenario-manifest.json` | Seven dependency-ordered scopes with Execution Outline, phase order, new types and signatures, validation checkpoints, per-scope Gherkin traced to §7 SCN IDs, implementation plans, Test Plan tables, and tiered DoD. Scope 06 recorded Blocked with an explicit `blockedReason`. Findings recorded as open with named owners and each planned into a named scope with a blocking DoD item — none silently resolved, none deferred to an unnamed session. |

---

## Planning Artifacts Produced (this agent)

| Artifact | Purpose | State |
|---|---|---|
| `scopes.md` | Seven-scope sequential plan; Execution Outline; per-scope Gherkin, implementation plan, Test Plan, and tiered DoD | Written |
| `report.md` | This file — planning-phase evidence only | Written |
| `uservalidation.md` | Operator acceptance checklist, checked-by-default baseline | Written |
| `state.json` | v3 execution/certification state at the `specs_hardened` ceiling | Written |
| `scenario-manifest.json` | Stable `SCN-*` contract entries with planned scope and expected live tests | Written |

**Not written by `bubbles.plan`, deliberately.** `spec.md` and `design.md` were **read
only** by the planning agent, because they are owned by `bubbles.analyst` / `bubbles.ux`
and `bubbles.design` respectively. Under mode `product-to-planning` the G073 source-edit
lockout is also active and the ceiling is `specs_hardened`; no file under `internal/`,
`cmd/`, `config/`, or `docs/` was created or modified, and no other spec folder was
touched.

**Precision on G073, added 2026-07-29.** G073 forbids changes *outside*
`spec.md, design.md, scopes.md, report.md, uservalidation.md, state.json, docs/**,
.github/**`. It therefore constrains **source code**, not `spec.md`. `spec.md` was left
unedited by `bubbles.plan` for **ownership** reasons, not because G073 forbade it. Later
`spec.md` edits in this packet — the §18 ratification record, the Principle 11
reconciliation, and the 2026-07-29 analyst corrections below — were made by the owning
agent and are all inside G073's permitted set.

---

## Plan Shape

### Scope Sequence And Rationale

| # | Scope | Depends On | Why here |
|---|---|---|---|
| 01 | MCP Foundation | — | Nothing can be authorized, projected, refused, or audited until the six `internal/mcp/` packages, the audience-bound credential (F-109-004), and the `/mcp` mount exist. |
| 02 | `context` toolset + discovery | 01 | The first real, service-backed tools. Proves determinism and the no-existence-oracle rule **before** any corpus tool exists, so a discovery regression is attributable. |
| 03 | `memory-read` + projection enforcement | 01, 02 | The `routing.Executor` seam, the `corpus` data scope resolving UX-F-001, the SQL source-level filter boundary, egress gated before the read, honest degradation. Heaviest adversarial test load. |
| 04 | `person-context` | 01, 02 | An opt-in toolset with **empty** `RequiredData`, isolating the §4 corpus term from the toolset term. |
| 05 | `graph-read` + step-up challenge | 01, 02, 04 | Depends on 04 so **two** opt-in toolsets coexist; with only one, "names exactly one scope" is trivially satisfied and cannot detect a catalog leak. |
| 06 | `hospitality-read` (**Blocked**) | 01, 02 | Registered as honestly unavailable. Serving it is blocked on BUG-019-003. |
| 07 | Docs, release packet, config, flags | 03, 04, 05 | Every `spec.md` §15 row, the §17 train targeting, both flag bundles, the release packet and its `features.md`, the capability-ledger surface, and the `docs/Product-Principles.md` alignment note. Not gated on 06, because 06's documentation obligation is the honest *unavailability* statement, which is writable today. |

### Test Plan ↔ DoD Parity

| Scope | Test Plan rows | Test DoD items | Categories |
|---|---|---|---|
| 01 | 7 | 7 | 5 unit, 1 integration, 1 e2e-api |
| 02 | 5 | 5 | 3 unit, 2 integration |
| 03 | 8 | 8 | 2 unit, 4 integration, 2 e2e-api |
| 04 | 4 | 4 | 2 unit, 1 integration, 1 e2e-api |
| 05 | 4 | 4 | 2 unit, 1 integration, 1 e2e-api |
| 06 | 3 | 3 | 2 unit, 1 integration |
| 07 | 5 | 5 | 3 unit, 1 integration, 1 e2e-api |
| **Total** | **36** | **36** | **19 unit, 11 integration, 6 e2e-api** |

Parity holds in every scope. Every live-category test (`integration`, `e2e-api`) is planned against
the ephemeral test stack with disposable storage and `env=test*` telemetry only — no writes to prod
monitoring, prod backup paths, or knb manifests (G115).

---

## Test Evidence

**None. No test was executed and none is claimed.**

This is a planning-only packet at ceiling `specs_hardened`. No MCP server exists (`spec.md` §1), so
there is nothing to execute against. Every Test Plan row in `scopes.md` is a **planned** test with a
named location, a named adversarial property, and a canonical command; none has been run.

Recording fabricated output here — a synthetic `PASS` line, an invented exit code, or a narrative
"all tests pass" — would be an anti-fabrication violation. The correct evidence at this ceiling is
the artifact set, not test output.

Execution evidence is produced later by `bubbles.implement` and certified by `bubbles.validate`,
using:

```
./smackerel.sh test unit
./smackerel.sh test integration
./smackerel.sh test e2e
./smackerel.sh check
./smackerel.sh lint
./smackerel.sh format --check
./smackerel.sh config generate
```

---

## Findings Carried Forward

### Resolved 2026-07-29 by `bubbles.analyst` (were open at planning close)

The five rows below were recorded **OPEN** when this planning packet closed. They were
resolved in a subsequent `bubbles.analyst` run by their recorded owner. **The rows are
retained rather than deleted** — the audit trail of what was open, and why, is the point.
Every resolution is an editorial correction to analyst-owned `spec.md` sections. **No
ratified decision (D1–D5, or any of the seven §18 items) was reopened, weakened, or
renegotiated, no implementation is claimed, and no test was executed.**

> **Correction of record — a false gate constraint this report previously carried.** The
> UX-F-002 row below used to justify its OPEN status with *"amending `spec.md` §15 is not
> permitted under G073 in this mode."* **That was factually wrong.** G073
> (`planning_only_source_edit_lockout_gate`) forbids changes *outside*
> `spec.md, design.md, scopes.md, report.md, uservalidation.md, state.json, docs/**,
> .github/**` — `spec.md` is explicitly on the **permitted** list. The real reason these
> findings stayed open was **artifact ownership**: `bubbles.plan` and `bubbles.ux`
> correctly declined to rewrite analyst-owned `spec.md` sections and routed them to the
> owning agent instead. **That routing was right; the gate citation was not.** The
> justification is corrected here so this packet no longer carries a fabricated gate
> constraint that a future reader could cite as precedent.

| Finding | Status | What changed, and where |
|---|---|---|
| **UX-F-002** — `docs/smackerel.md` §17.1 (confidence signals) was absent from the §15 documentation table, yet §9A.5 ties R-109-UX15–UX18 to it | **RESOLVED** 2026-07-29 | `spec.md` §15 gained a `docs/smackerel.md` §17.1 (Trust Architecture) row recording the specific tie: §17.1's **Confidence signals** trust mechanism is expressed on the MCP path as the spec-095 `StrategySelection` honest-degradation fields `retrieval_reason`, `retrieval_contract_known`, and `retrieval_fell_back` carried on every Projection (§9), with `degraded-fallback` rendered as a successful-but-degraded answer and never as confidence. §9A.5's "Principle tie" was updated from "see UX-F-002 for that reference's status" to the now-funded row. §9A.7's entry records the disposition. Scope 07 had already planned the §17.1 update and TP-07-03 already asserts a §17.1 anchor, so **no planned work changed** — §15 was brought into agreement with the plan. |
| **UX-F-004** — §2's Success Signal omitted `retrieval_reason` and `retrieval_contract_known`, both of which §9 returns and R-109-UX15/UX18 depend on | **RESOLVED** 2026-07-29 | `spec.md` §2's Success Signal now enumerates the complete six-field §9 provenance set — `source_kind`, `retrieval_strategy`, `retrieval_reason`, `retrieval_fell_back`, `retrieval_contract_known`, `trace_token` — and states the R-109-UX15/UX18 non-omittability rule, with an inline correction note. **§9 was not narrowed to match §2**: §9 is the authoritative field table, §2 was the incomplete restatement, and the restatement is what moved. `scopes.md` TP-03-01 already tested all six, so **no planned work changed**. |
| **UX-F-005** — D1's prose asserted `remote-inference` was already implemented while §1 says no MCP server exists; read literally these conflict | **RESOLVED** 2026-07-29 | `spec.md` D1 now reads "fully **specified** but **default-OFF**", with an inline correction note. The **posture** ratified by §18 item 1 is untouched — `local-inference` remains the default and only enabled egress class, remote remains an explicit per-client individually audited grant, and the BINDING CONSTRAINT on claim shape still binds every surface. Only the inaccurate implementation claim was removed. §18 item 1's "Not resolved by this item" paragraph and §9A.7's entry record the correction without rewriting what the gate did. The stale phrasing is now absent from `spec.md` entirely. |
| **F-109-OF2-AMEND** — OF-2 lacked a `corpus` data-scope grant/revoke step, and OF-2 step 4's effective-list invariant held only conditionally | **RESOLVED** 2026-07-29 | `spec.md` §9A.3's OF-2 was amended and retitled "Grant / revoke a toolset **and a data scope**": new **step 3** grants the `corpus` data scope and states the client experiences §9A.4 Case A verbatim until it exists (a distinct client-visible corpus token would be an existence oracle — R-109-UX12); new **step 4** requires the operator surface to show the *effective* result, not the grant just made; **step 5** covers data-scope revocation; **step 6** states the four-term intersection explicitly and requires the operator surface to call the **same authorizer the request path calls** rather than reimplement the set logic. **Verified before relying on it:** `scopes.md` Scope 03 already requires `data-scope` as a third `GrantKind` with the authorizer evaluating the full four-term intersection (plus TP-03-02), and Scope 07's `docs/Operations.md` plan and DoD already name the `corpus` data-scope grant/revoke step against F-109-OF2-AMEND. So this brought §9A.3 into line with the existing plan; **no planned work changed**. |
| **§15 lacked a `docs/Product-Principles.md` row** (found in re-review 2026-07-29) | **RESOLVED** 2026-07-29 | `scopes.md` TP-07-03 asserts *"every `spec.md` §15 target file exists"* and then names the `docs/Product-Principles.md` alignment note plus the delivered Principle 11 assertions — but §15 declared no such row, so the test asserted against a target the spec never declared. `spec.md` §15 now carries the row with both obligations: **(a)** the §16 Product Principle Alignment note citing **P8 — Trust Through Transparency** and **P11 — Local-First Data Ownership**, with the principle-gap note in its *delivered* form; **(b)** verification that the shipped `## Principle 11 — Local-First Data Ownership` is present **together with** its matching `### Principle 11` enforcement block in the BLOCKING companion `.github/instructions/product-principles.instructions.md` — a principle present in one but not the other is the gap, not the fix. The row is explicitly a **verification** obligation: P11 and its enforcement block were authored and signed off by the **owner** on 2026-07-29 outside this packet (§18 item 7), and nothing in §15 authorises an agent to amend an owner-ratified document. |

**Residual, honestly recorded — two foreign-artifact instances of the same false G073
citation are NOT corrected here.** The identical mistaken justification also appears in
`scopes.md` (line ~107, *"`spec.md` §15 is not edited by this packet (G073)"*, and line
~772, *"this packet may not edit `spec.md` (G073)"*) and in `design.md` (§4 closing,
*"spec.md is not edited by this design (mode `product-to-planning`, G073 source-edit
lockout)"*). Those artifacts are owned by `bubbles.plan` and `bubbles.design`
respectively, not by `bubbles.analyst`, and `scopes.md` additionally carries lint-enforced
Test Plan ↔ DoD parity. Rewriting them from here would be exactly the ownership violation
that produced the original routing. **Routed to their owners**, with the correction
already stated above: substitute the ownership rationale for the G073 citation. Note that
`scopes.md` line ~139 already states it correctly for F-109-OF2-AMEND — *"amending
`spec.md` §9A.3 is owned by `bubbles.analyst` under a mode that permits spec edits"* — so
the correct framing already exists in that file and can simply be applied to the other two
sites.

**One further residual — `state.json`.** The same false citation is embedded in
`certification.outstandingFindings[UX-F-002].summary`. `certification.*` is validate-owned
and is explicitly out of bounds for this agent; the resolution of all five findings is
recorded additively in `executionHistory` instead. **Routed to `bubbles.validate`** to
reconcile `certification.outstandingFindings` and strike the G073 citation.

### Resolved during the original planning packet

| Finding | Resolution |
|---|---|
| **UX-F-001** — `memory-read`'s "corpus grant" is undefined in the capability model | **RESOLVED by `design.md` §4.** MCP owns its own corpus-read grant, carried in its own credential, as a third `Grant` **kind** (`data-scope`) — not a second grant system and not spec 108's `corpus:read`. Four independent reasons are recorded (D3 survivability, per-client vs per-principal expressiveness, spec 108's OBSERVE stage importing a rollout hole into a security boundary, and revocation blast radius). A corpus denial emits `unauthorized-toolset` to the client — Case A, byte-identical to nonexistent — because a distinct corpus token would itself be an existence oracle. Operator granularity lives in the ledger's `denial_reason` column only. Planned in **Scope 03**. |
| **UX-F-003** — Principle numbering: Trust Through Transparency is **Principle 8**; there was **no** pre-existing **Principle 11** | **RESOLVED 2026-07-29.** Both halves are closed. The *numbering* half was always a statement of fact and stands. The *principle-gap* half was settled by ratified `spec.md` §18 item 7 and **delivered by the owner**, not by this packet: under the sign-off that item required, `docs/Product-Principles.md` gained `## Principle 11 — Local-First Data Ownership` (`**Status**: Ratified 2026-07-29`) plus a matching enforcement block in its BLOCKING companion `.github/instructions/product-principles.instructions.md`. This packet then reconciled its own artifacts to the shipped title: `spec.md` §16 cites **Principle 11 — Local-First Data Ownership** as the product-track carrier with **Constitution C1** retained as the engineering-track cross-reference, and §16's alignment table carries a P11 row. Scope 07 now carries the shipped surfaces as **verification** DoD items rather than as edits to make. No implementation is claimed and no test was executed. |

### Open — routed, not silently resolved

**All four rows in the table below were CLOSED on 2026-07-29** by `bubbles.analyst`, the
recorded owner. They are retained verbatim as the historical record of what was open at
planning close and how it was routed; each row's disposition is in the *"Resolved
2026-07-29"* table above. The `bubbles.analyst` routing shown in the right-hand column is
what actually happened — the owner picked them up and closed them.

| Finding | Status | Where planned | Routed to |
|---|---|---|---|
| **UX-F-002** — `docs/smackerel.md` §17.1 (confidence signals) is absent from the §15 documentation table, yet §9A.5 ties R-109-UX15–UX18 to it | ~~OPEN~~ → **CLOSED 2026-07-29** | Scope 07 plans the §17.1 update as a recorded planning decision | `bubbles.analyst` (RQ-109-02) — held for the **owning agent** because `spec.md` §15 is analyst-owned and `bubbles.plan` does not rewrite it. *(This cell previously cited G073; that citation was wrong and is corrected above.)* |
| **UX-F-004** — §2's Success Signal omits `retrieval_reason` and `retrieval_contract_known`, both of which §9 returns and R-109-UX15/UX18 depend on | ~~OPEN~~ → **CLOSED 2026-07-29** | Scope 03 plans and tests **all six** provenance fields (TP-03-01) | `bubbles.analyst` — re-routed: the §18 gate closed 2026-07-29 on the seven product decisions and did **not** decide this editorial correction (RQ-109-03) |
| **UX-F-005** — D1's prose asserted `remote-inference` was already implemented while §1 says no MCP server exists; read literally these conflict. Intended meaning: "fully **specified**" | ~~OPEN~~ → **CLOSED 2026-07-29** | Scope 03 plans it as fully specified and default-OFF per client | `bubbles.analyst` — re-routed: ratified §18 item 1 accepted D1's *posture* but did **not** correct D1's *prose* (RQ-109-04) |
| **F-109-OF2-AMEND** — new, surfaced by `design.md` §4. OF-2 needs a `corpus` data-scope grant/revoke step, and OF-2 step 4's effective-list invariant holds only if the operator-side computation runs the same four-term intersection | ~~OPEN~~ → **CLOSED 2026-07-29** | Scopes 03 and 07 are already planned to the amended behavior; Scope 07's `docs/Operations.md` DoD item names the data-scope step explicitly | `bubbles.analyst` (RQ-109-01) |

### Spec findings inherited

| Finding | Status | Where planned |
|---|---|---|
| **F-109-001** — `hospitality-read` blocked on BUG-019-003 (`artifacts.source_ref` never persisted) | **OPEN, blocking J4** | Scope 06 (**Blocked**) — planned as honestly unavailable; serving it waits on the fix |
| **F-109-002** — spec 109 is spec 095's first live consumer of `internal/retrieval/routing` | **OPEN, risk** | Scope 03 — owns its own `cmd/core` wiring (PKT-095-A/B/C are explicitly not prerequisites) and funds live `e2e-api` coverage (TP-03-07), discharging F-095-E2E-LIVE **for the MCP path only** |
| **F-109-003** — sensitivity ceilings unimplementable (no sensitivity column; four incompatible satellite vocabularies) | **Accepted constraint** | Not planned; D1 controls on client inference locality instead and no scope claims sensitivity filtering |
| **F-109-004** — missing `aud` claim is a conformance blocker | **OPEN, blocking, in scope** | Scope 01 — wires the unwired `Audience` type into `IssueToken` and verifies with **positive equality** (TP-01-01, TP-01-02) |
| **F-109-005** — the general agent registry is not an MCP source of truth (19 non-capabilities) | **OPEN, design constraint** | Scope 02 — explicit manifest; TP-02-03 proves all 19 never appear |
| **F-109-006** — source-level egress needs SQL, not a post-filter | **OPEN, design constraint** | Scope 03 — the domain service applies the predicate in `WHERE`, before ranking and before `LIMIT` |

---

## Deferred, Deliberately

`SCN-109-013` and `SCN-109-014` (write-plane propose / confirm / expire) belong to the
`memory-write` toolset, which decision **D5** sets **off (later)**. `design.md` §8 settles the
`confirm.Machine` mapping now — `transport="mcp"` as a single-flight keying discriminator, never an
assistant `TransportAdapter` registration — so the deferral does not become a redesign. **No scope in
this packet delivers it**; planning it would ship an off-by-decision toolset. Recorded in
`scenario-manifest.json` with `plannedIn: null` and status `deferred`.

---

## Completion Statement

**Planning complete at the `product-to-planning` ceiling `specs_hardened`.**

Delivered: `scopes.md` (7 scopes, dependency-ordered, 43 Test Plan rows each with a matching DoD
item, plus 14 standing regression DoD items — 57 test DoD items in total),
`report.md`, `uservalidation.md`, `state.json` (v3), and `scenario-manifest.json` (18 scenario
contract entries — 16 planned, 2 deferred).

Not claimed: any implementation, any test result, any execution evidence, and any status above
`specs_hardened`. No source file was created or modified under the G073 lockout; no other spec
folder was touched. `design.md` was read only. `spec.md` was read only **by `bubbles.plan`**;
it was subsequently edited by its owning agent under explicit operator instruction — to record
the §18 ratification, to reconcile to the shipped Principle 11, and on 2026-07-29 to make the
five editorial corrections recorded above — each recorded in `state.json` `executionHistory`.

Findings UX-F-002, UX-F-004, UX-F-005, and F-109-OF2-AMEND were recorded as **open** with
named owners at planning close; **all four — plus a fifth found in re-review (§15 lacked a
`docs/Product-Principles.md` row) — were RESOLVED on 2026-07-29** by their recorded owner
`bubbles.analyst`. See *"Resolved 2026-07-29"* above for each disposition; the original
OPEN rows are retained rather than deleted. UX-F-001 was resolved by `design.md` §4 and
UX-F-003 is resolved. None was ever silently resolved, and none was deferred: each was
also resolved inline in the plan by a named scope and a blocking DoD item (see
`state.json` `reworkQueueNote`, which is why `reworkQueue` is empty rather than queued).
The six inherited spec findings **F-109-001…006 remain as recorded** — they are delivery
constraints, not editorial defects, and nothing in the 2026-07-29 pass touched them. Scope
06 is still recorded **Blocked** on BUG-019-003 rather than planned as deliverable.

**Operator review gate \u2014 CLOSED 2026-07-29.** All seven `spec.md` §18 decisions were **ratified by
operator delegation** under the instruction *"pick the best option for long term, no shortcuts."*
§18 is now a decision record, not a gate. Two consequences are recorded honestly:

- **D4 was amended.** Item 3 was accepted only *with a carve-out*: MCP prompts, MCP Apps, and
  subscriptions/`notifications/tools/list_changed` are permanently deleted, but OAuth 2.1 + DCR +
  RFC 9728 are **out of scope with a named re-open trigger** ("re-opens if and only if a client
  outside the operator's control must connect to `/mcp`"), because calling them permanently deleted
  would have been overclaiming. `spec.md` D4 and §13 were updated to match.
- **The gate did not decide everything it was carrying.** UX-F-002, UX-F-004, and UX-F-005 had been
  bundled into "the operator's §18 pass" for convenience. They are wording corrections to `spec.md`,
  not product decisions, so the ratification did **not** settle them. They were re-routed to
  `bubbles.analyst` rather than being quietly marked resolved. **That owner closed all three on
  2026-07-29** (together with F-109-OF2-AMEND and the §15 `docs/Product-Principles.md` gap). This
  does not revise the gate record: the gate genuinely did not decide them, and §18's ratification
  boundary still says so.

**Next owner.**

1. **A delivery-capable run** (this packet is `planningOnly` under `product-to-planning`), starting
   at **Scope 01**. Scope 07's `docs/Product-Principles.md` obligations are now **verification**
   items — the principle and its BLOCKING companion enforcement block shipped by owner sign-off on
   2026-07-29 — so no agent amends that owner-ratified document.
2. **`bubbles.validate`**, to reconcile `state.json` `certification.outstandingFindings`: mark
   UX-F-002, UX-F-004, UX-F-005, and F-109-OF2-AMEND resolved, and strike the false
   *"not permitted under G073"* justification embedded in UX-F-002's summary.
   `certification.*` is validate-owned and was correctly not written by this analyst run.
3. **`bubbles.plan` and `bubbles.design`**, to correct the same false G073 citation where it
   survives in their own artifacts (`scopes.md` ~L107 and ~L772; `design.md` §4 closing) —
   substituting the ownership rationale, which `scopes.md` ~L139 already states correctly.

~~2. `bubbles.analyst`, for the three editorial `spec.md` corrections the §18 gate did not decide
(UX-F-002, UX-F-004, UX-F-005) plus F-109-OF2-AMEND.~~ **Done 2026-07-29.**
