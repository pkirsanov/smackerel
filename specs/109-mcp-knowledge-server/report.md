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

**Not written, deliberately.** `spec.md` and `design.md` were **read only**. Under mode
`product-to-planning` the G073 source-edit lockout is active and the ceiling is `specs_hardened`; no
file under `internal/`, `cmd/`, `config/`, or `docs/` was created or modified, and no other spec
folder was touched.

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

### Resolved during this packet

| Finding | Resolution |
|---|---|
| **UX-F-001** — `memory-read`'s "corpus grant" is undefined in the capability model | **RESOLVED by `design.md` §4.** MCP owns its own corpus-read grant, carried in its own credential, as a third `Grant` **kind** (`data-scope`) — not a second grant system and not spec 108's `corpus:read`. Four independent reasons are recorded (D3 survivability, per-client vs per-principal expressiveness, spec 108's OBSERVE stage importing a rollout hole into a security boundary, and revocation blast radius). A corpus denial emits `unauthorized-toolset` to the client — Case A, byte-identical to nonexistent — because a distinct corpus token would itself be an existence oracle. Operator granularity lives in the ledger's `denial_reason` column only. Planned in **Scope 03**. |

### Open — routed, not silently resolved

| Finding | Status | Where planned | Routed to |
|---|---|---|---|
| **UX-F-002** — `docs/smackerel.md` §17.1 (confidence signals) is absent from the §15 documentation table, yet §9A.5 ties R-109-UX15–UX18 to it | **OPEN** | Scope 07 plans the §17.1 update as a recorded planning decision | `bubbles.analyst` (RQ-109-02) — amending `spec.md` §15 is not permitted under G073 in this mode |
| **UX-F-003** — Principle numbering: Trust Through Transparency is **Principle 8**; there is **no** pre-existing **Principle 11** | **PARTIALLY RESOLVED 2026-07-29** — the numbering facts stand; the *principle-gap* half is settled by ratified `spec.md` §18 item 7 | Scope 07 writes the alignment note against Principle 8, **and** carries two owner-sign-off DoD items: add `Principle 11 — Your Data Stays Yours` to `docs/Product-Principles.md` and a matching block to its BLOCKING companion enforcement file. This packet still applies neither | Owner sign-off at delivery (was RQ-109-05) |
| **UX-F-004** — §2's Success Signal omits `retrieval_reason` and `retrieval_contract_known`, both of which §9 returns and R-109-UX15/UX18 depend on | **OPEN** | Scope 03 plans and tests **all six** provenance fields (TP-03-01) | `bubbles.analyst` — re-routed: the §18 gate closed 2026-07-29 on the seven product decisions and did **not** decide this editorial correction (RQ-109-03) |
| **UX-F-005** — D1 says `remote-inference` is "fully coded but default-OFF" while §1 says no MCP server exists; read literally these conflict. Intended meaning: "fully **specified**" | **OPEN** | Scope 03 plans it as fully specified and default-OFF per client | `bubbles.analyst` — re-routed: ratified §18 item 1 accepted D1's *posture* but did **not** correct D1's *prose* (RQ-109-04) |
| **F-109-OF2-AMEND** — new, surfaced by `design.md` §4. OF-2 needs a `corpus` data-scope grant/revoke step, and OF-2 step 4's effective-list invariant holds only if the operator-side computation runs the same four-term intersection | **OPEN** | Scopes 03 and 07 are already planned to the amended behavior; Scope 07's `docs/Operations.md` DoD item names the data-scope step explicitly | `bubbles.analyst` (RQ-109-01) |

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
`specs_hardened`. `spec.md` and `design.md` were read only; no source file was created or modified
under the G073 lockout; no other spec folder was touched.

Open findings UX-F-002, UX-F-004, UX-F-005, and F-109-OF2-AMEND remain recorded as
**open** with named owners in `state.json` `certification.outstandingFindings`; UX-F-003 is
**partially resolved** (see the table above). None was silently resolved, and none is deferred: each
is resolved inline in the plan by a named scope and a blocking DoD item (see `state.json`
`reworkQueueNote`, which is why `reworkQueue` is empty rather than queued). Scope 06 is recorded
**Blocked** on BUG-019-003 rather than planned as deliverable.

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
  not product decisions, so the ratification did **not** settle them. They are re-routed to
  `bubbles.analyst` rather than being quietly marked resolved.

**Next owner:** two, in parallel.

1. **A delivery-capable run** (this packet is `planningOnly` under `product-to-planning`), starting
   at **Scope 01**. Scope 07 additionally requires **owner sign-off** before amending the
   owner-ratified `docs/Product-Principles.md` and its BLOCKING companion enforcement file.
2. **`bubbles.analyst`**, for the three editorial `spec.md` corrections the §18 gate did not decide
   (UX-F-002, UX-F-004, UX-F-005) plus F-109-OF2-AMEND.
