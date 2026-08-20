# Scopes: 112 Capability Registry

> **Packet status:** `not_started` · **Workflow mode:** `product-to-planning` · **Release train:** `next`
> **Owner of this artifact:** `bubbles.plan`
> **Created by:** `bubbles.analyst` as an honest initial artifact so the packet is structurally complete
>
> This is packet-level metadata, not a scope status. Per-scope statuses appear only under
> each `## SCOPE-NN` heading below and are drawn solely from `Not Started` / `In Progress` /
> `Done` / `Blocked`.

---

## What This File Is, And Is Not

**This is not a ratified plan.** It was created alongside [`spec.md`](spec.md) during
requirements authoring so the artifact set is complete and lintable. The scopes below are
derived from the requirement groups in `spec.md` §8 as a **starting shape only**.

`bubbles.plan` owns this file and may restructure, merge, split, reorder, or discard any of
it. No scope has been executed. **Every Definition of Done item below is unchecked, and
none carries evidence, because no work has been done.**

Four findings are BLOCKING and gate work before it begins. They are carried onto the
specific scopes they block rather than left for discovery during execution.

---

## Scope Dependency Graph

```
SCOPE-01 (descriptor record)
   ├── SCOPE-02 (declared exposure)
   │      └── SCOPE-04 (assistant intent projection)
   │      └── SCOPE-06 (alias projection)
   │      └── SCOPE-07 (external tool projection)
   ├── SCOPE-03 (per-capability authorization)  ← gated on spec 108
   │      └── SCOPE-04, SCOPE-06, SCOPE-07 (nothing surfaces before its guard)
   └── SCOPE-05 (navigation core projection, digest included)

SCOPE-08 (coverage check)      ← depends on SCOPE-01, SCOPE-02, SCOPE-03
SCOPE-09 (retirement of hand-maintained inventories) ← depends on 04, 05, 06, 07
```

### Foundation and concrete implementations

`design.md` splits this feature into one **Capability Foundation** — the capability
descriptor record plus the projection contract — and **four concrete implementations**, one
per reachability surface. The scope order below carries that split:

| Role | Scopes | Why |
|---|---|---|
| Foundation | SCOPE-01 (`foundation:true`) | Establishes the single record every other scope reads. Nothing can project from a record that does not exist. |
| Foundation facets | SCOPE-02, SCOPE-03 | Add the exposure class and the authorization requirement to that **same** record. They are not projections, which is why every projection depends on all three. |
| Concrete implementations | SCOPE-04, SCOPE-05, SCOPE-06, SCOPE-07 | The four projections. Each consumes the foundation and adds nothing to it. |
| Validation | SCOPE-08 | Proves the foundation carries every facet a projection needs, against an independently derived universe. |
| Retirement | SCOPE-09 | Removes the hand-maintained predecessors the projections replace. |

No projection scope may start before SCOPE-03, because P4 forbids surfacing a capability
whose guard is not enforceable — and SCOPE-03 is itself blocked on spec 108.

---

## SCOPE-01 — The capability descriptor record

**Status:** Not Started
**Tags:** `foundation:true`
**Depends On:** none
**Blocked by:** `F-112-UNIT-01` (BLOCKING) — what constitutes one capability, and how prompt contracts bind to capabilities, is undecided; the mapping is not 1:1.
**Requirements:** R-112-01 … R-112-07
**Scenarios:** SCN-112-A01, SCN-112-A05, SCN-112-A06

This is the foundation scope named in `design.md` — `## Capability Foundation`. It builds the
record and the projection contract that SCOPE-04 through SCOPE-07 each project from, and
that SCOPE-08 validates. It is deliberately not a projection itself.

### Definition of Done

- [ ] One capability descriptor exists per capability, carrying stable id, intent phrasing, owning domain service, required principal and grants, provenance requirement, side-effect class, navigation projection, alias projection and external tool projection
- [ ] The record extends the existing experience catalog rather than standing beside it, and capability identity is authored in exactly one place
- [ ] The existing catalog's per-surface capability reference resolves to a descriptor, and an unresolvable reference is reported
- [ ] A capability's stable id survives changes to its phrasing, navigation position, alias and exposure class
- [ ] The record carries no user content, no readiness fact and no session scope
- [ ] SCN-112-A01, SCN-112-A05 and SCN-112-A06 pass
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope exist and pass, one per `SCN-112-A01`, `SCN-112-A05` and `SCN-112-A06`, including a catalog surface whose `capability_id` resolves to nothing which must be reported — so the test fails against today's nil-`KnownCapabilities` seam that silently disables the check (E8)
- [ ] Broader E2E regression suite passes with no previously-green scenario broken by introducing the capability descriptor record
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-02 — Declared exposure for every built capability

**Status:** Not Started
**Depends On:** SCOPE-01 (foundation — exposure is a facet of the descriptor record)
**Blocked by:** `F-112-UNIT-01` (BLOCKING) — the exposure decision is per capability, so the capability unit must be settled first.
**Requirements:** R-112-13 … R-112-18
**Scenarios:** SCN-112-B01, SCN-112-B02, SCN-112-B03, SCN-112-B04, SCN-112-B05

### Definition of Done

- [ ] Every built capability carries an exposure class from a closed vocabulary, and no capability is in an undeclared state
- [ ] Every non-user-facing class carries a recorded reason; an empty or absent reason is reported as a defect
- [ ] A structurally non-dispatchable pipeline stage is identifiable as such and is not counted among capabilities lacking a front door
- [ ] A test-only capability appears in no user-facing projection
- [ ] A newly built dispatchable capability with no recorded decision fails validation rather than defaulting to exposed or hidden
- [ ] Each of the eleven currently-undeclared dispatchable capabilities named in `spec.md` §3 carries an explicit class and, where not user-facing, a reason
- [ ] SCN-112-B01 through SCN-112-B05 pass
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope exist and pass, one per `SCN-112-B01`…`SCN-112-B05`, including a newly built dispatchable capability carrying no exposure decision which must fail validation rather than default either way — so the test fails against any implementation that keeps treating absence from a list as "hidden"
- [ ] Broader E2E regression suite passes with no previously-green scenario broken by making exposure a declared fact
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-03 — Per-capability authorization

**Status:** Not Started
**Depends On:** SCOPE-01 (foundation — authorization is a facet of the descriptor record)
**Blocked by:** `F-112-108-01` (BLOCKING) — the grant model belongs to spec 108, which is `blocked`, not `specs_hardened`. That model is **built, mounted and green**, but it runs in the OBSERVE (non-denying) stage and its enforcement flip is held by three operator-owned, time-bound items plus a review carrying `blocks_on_failure: [release-train-promote]`. This scope cannot complete before that grant model actually denies.
**Requirements:** R-112-23 … R-112-28
**Scenarios:** SCN-112-D01, SCN-112-D02, SCN-112-D03, SCN-112-D04, SCN-112-D05

### Definition of Done

- [ ] Every reachable capability names the authenticated principal and the grants it requires
- [ ] Authorization is derived on the server from the descriptor and the authenticated principal, and caller-asserted authority is not trusted
- [ ] A capability whose declared grant the running system cannot enforce is not surfaced, and the reason is reported without leaking the capability's existence
- [ ] The grant vocabulary is consumed from spec 108 and is not redefined, widened or forked here
- [ ] Every capability declares its side-effect class
- [ ] Every capability declares its provenance requirement, and a capability requiring provenance never presents an uncited answer as grounded
- [ ] SCN-112-D01 through SCN-112-D05 pass
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope exist and pass, one per `SCN-112-D01`…`SCN-112-D05`, including a capability whose declared grant the running system cannot enforce, asserted absent from every projection with its absence leaking nothing — so the test fails against a projection that admits on exposure class alone
- [ ] Broader E2E regression suite passes with no previously-green scenario broken by deriving authorization per capability on the server
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-04 — Assistant intent projection

**Status:** Not Started
**Depends On:** SCOPE-01 (foundation), SCOPE-02, SCOPE-03
**Requirements:** R-112-08, R-112-10
**Scenarios:** SCN-112-A02, SCN-112-A03

Concrete implementation of the foundation — the intent projection.

### Definition of Done

- [ ] The natural-language intent set is generated from the descriptor set and is not authored by hand
- [ ] A capability declared user-facing becomes reachable by plain-language request without any reachability surface being edited
- [ ] No capability is surfaced whose authorization requirement is not enforceable
- [ ] SCN-112-A02 and SCN-112-A03 pass
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope exist and pass, one per `SCN-112-A02` and `SCN-112-A03`, including a capability made reachable by plain-language request after a single descriptor edit with the hand-maintained intent registry asserted unmodified — so the test fails against the four-edit reachability model this scope replaces
- [ ] Broader E2E regression suite passes with no previously-green scenario broken by generating the intent set
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-05 — Navigation core projection, digest included

**Status:** Not Started
**Depends On:** SCOPE-01 (foundation)
**Blocked by:** `F-112-CUTOVER-01` (HIGH) — three navigation authorities are live and already divergent; whether this feature cuts them over or the shell spec does is undecided. `F-112-ISLANDS-01` (MEDIUM) — 29 of 31 PWA pages render no navigation at all.
**Requirements:** R-112-19 … R-112-22, R-112-11, R-112-12
**Scenarios:** SCN-112-C01, SCN-112-C02, SCN-112-C03, SCN-112-C04

Concrete implementation of the foundation — the navigation core projection. It is the one
projection that must reconcile three live authorities rather than one (E11 – E14).

### Definition of Done

- [ ] The daily digest is a member of the guaranteed cross-surface navigation core
- [ ] Every member of the guaranteed core is present on every surface that renders navigation
- [ ] A capability present only in a surface-local navigation addition is reported as absent from the core and is not treated as cross-surface reachable
- [ ] A divergence between two surfaces that are both projections of the same descriptor set fails rather than passing silently
- [ ] A claim by one surface that it mirrors another is verified against the generated core rather than asserted in a comment
- [ ] The set of surfaces that render no navigation is identifiable, so the core's reach is a known quantity
- [ ] SCN-112-C01 through SCN-112-C04 pass
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope exist and pass, one per `SCN-112-C01`…`SCN-112-C04`, asserting the digest is present in the guaranteed core on every navigation-rendering surface — which fails against both the six-link shared partial (E11) and the eight-link PWA list (E12) — and that a surface-local append (E14) is still reported as absent from the core
- [ ] Broader E2E regression suite passes with no previously-green scenario broken by generating the navigation core
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-06 — Alias projection

**Status:** Not Started
**Depends On:** SCOPE-01 (foundation), SCOPE-02, SCOPE-03
**Blocked by:** `F-112-ALIAS-01` (HIGH) — the alias table and the assistant registry currently contradict each other on `/ask`, `/recipe` and `/cook`. Generating both from one descriptor forces a decision that changes shipped user-visible behaviour for at least one token.
**Requirements:** R-112-08, R-112-10
**Scenarios:** SCN-112-A02, SCN-112-A03

Concrete implementation of the foundation — the alias projection. Its predecessor actively
contradicts the intent registry today (E15, E16), so generation forces a recorded decision.

### Definition of Done

- [ ] The alias table is generated from the descriptor set and is not authored by hand
- [ ] The `/ask` contradiction is resolved by an explicit recorded decision, not silently by generation order
- [ ] The status of `/recipe` and `/cook` is decided explicitly and recorded
- [ ] An alias resolves to exactly one capability, and two aliases naming different capabilities for the same token is impossible by construction
- [ ] SCN-112-A02 and SCN-112-A03 pass for the alias surface
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope exist and pass, one per `SCN-112-A02` and `SCN-112-A03` on the alias surface, asserting `/ask` resolves to exactly one capability across every surface — which fails against today's two-target contradiction (E15) — and that `/recipe` and `/cook` agree with the registry rather than shipping against it (E16)
- [ ] Broader E2E regression suite passes with no previously-green scenario broken by resolving the alias contradictions
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-07 — External tool projection

**Status:** Not Started
**Depends On:** SCOPE-01 (foundation), SCOPE-02, SCOPE-03
**Blocked by:** `F-112-109-01` (HIGH) — spec 109 forbids deriving the tool list by passthrough, so the projection must be per-capability and declared. `F-112-108-01` (BLOCKING) — external exposure is a grant decision under Principle 11.
**Requirements:** R-112-08, R-112-25, R-112-26
**Scenarios:** SCN-112-A02, SCN-112-D03

Concrete implementation of the foundation — the external tool projection. It is the only one
with no hand-maintained predecessor, and the only one whose admission is itself a grant
decision (R-112-25, Principle 11).

### Definition of Done

- [ ] The external tool projection is generated per capability from the descriptor set, and is not a blanket export of the capability set
- [ ] Exposure of a capability to an external client is a declared grant decision, never a generation side effect
- [ ] A capability whose grant cannot be enforced does not appear in the projection, and its absence leaks nothing about its existence
- [ ] The projection satisfies spec 109's constraint that no tool is derived by passthrough
- [ ] SCN-112-A02 and SCN-112-D03 pass for the external surface
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope exist and pass, one per `SCN-112-A02` and `SCN-112-D03` on the external surface, including a capability present in the descriptor set but absent from the projection because its grant is unenforceable — so the test fails against a blanket export of the capability set, which spec 109 forbids (E19)
- [ ] Broader E2E regression suite passes with no previously-green scenario broken by projecting capabilities to an external client
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-08 — The coverage check

**Status:** Not Started
**Depends On:** SCOPE-01 (foundation), SCOPE-02, SCOPE-03
**Blocked by:** `F-112-UNIVERSE-01` (BLOCKING) — how the independent universe of built capabilities is derived, and how it avoids the circularity already present in the existing catalog's integration test, is undecided.
**Requirements:** R-112-29 … R-112-33
**Scenarios:** SCN-112-E01, SCN-112-E02, SCN-112-E03, SCN-112-E04, SCN-112-E05, SCN-112-E06

### Definition of Done

- [ ] The check fails when an enabled capability lacks an intent phrase, an authorization requirement, a navigation status, or an evaluation case
- [ ] The check names the specific capability and the specific missing facet, not an aggregate count
- [ ] The universe the check compares against is derived independently of the registry under test
- [ ] A capability present in the built system but absent from the registry fails the check
- [ ] A check that did not execute is reported as a failure, never as a pass
- [ ] An adversarial case proves the check fails when a facet is removed, so the check is not satisfiable by construction
- [ ] SCN-112-E01 through SCN-112-E06 pass
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope exist and pass, one per `SCN-112-E01`…`SCN-112-E06`, including a capability built but absent from the registry entirely which must fail the check — so the test fails against a universe derived from the artifact under test (E9) — and a run in which the check did not execute, which must be reported as a failure
- [ ] Broader E2E regression suite passes with no previously-green scenario broken by making the coverage check able to fail
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-09 — Retirement of the hand-maintained inventories

**Status:** Not Started
**Depends On:** SCOPE-04, SCOPE-05, SCOPE-06, SCOPE-07
**Blocked by:** `F-112-CUTOVER-01` (HIGH) — whether this feature performs the cutover or the shell spec does is undecided, and R-112-09 cannot be satisfied by whichever spec does not.
**Requirements:** R-112-04, R-112-09
**Scenarios:** SCN-112-A04

### Definition of Done

- [ ] Each hand-maintained inventory that now has a generated projection is retired, not left active beside it
- [ ] No reachability surface carries a capability fact absent from its descriptor
- [ ] A separately authored list for a surface that has a projection is reported as a defect and is not accepted as a transitional state
- [ ] The condition the existing catalog package documents — generated catalog running alongside handwritten authorities — no longer holds for any surface this feature projects
- [ ] SCN-112-A04 passes
- [ ] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope exist and pass, one per `SCN-112-A04`, asserting a separately authored list still consulted at runtime beside its projection is reported as a defect — so the test fails against the "generated catalog added ALONGSIDE the handwritten authorities" state the catalog package documents (E10)
- [ ] Broader E2E regression suite passes with no previously-green scenario broken by retiring the hand-maintained inventories
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## Test Plan

### Test Matrix

**This table is a planning input, not an executed plan.** No test below has been written or
run. Canonical commands are taken from the repository's declared command surface.

| Test type | Category | Scope | What it must prove | Command |
|---|---|---|---|---|
| Descriptor record | `unit` | SCOPE-01 | One record per capability; stable id survives churn; content-free | `./smackerel.sh test unit` |
| Catalog reference resolution | `unit` | SCOPE-01 | Every existing `capability_id` resolves; an unresolvable one is reported | `./smackerel.sh test unit` |
| Exposure vocabulary | `unit` | SCOPE-02 | Closed vocabulary; undeclared is impossible; reason required when not user-facing | `./smackerel.sh test unit` |
| Authorization derivation | `unit` | SCOPE-03 | Server-derived; caller-asserted authority rejected | `./smackerel.sh test unit` |
| Projection determinism | `unit` | SCOPE-04…07 | Identical descriptor set produces byte-identical projections; stable ordering | `./smackerel.sh test unit` |
| Coverage check | `unit` | SCOPE-08 | Fails per missing facet; names capability and facet; adversarial removal case fails | `./smackerel.sh test unit` |
| Independent universe | `integration` | SCOPE-08 | Expected set is not derived from the registry under test; a built-but-unregistered capability fails | `./smackerel.sh test integration` |
| Cross-surface core parity | `integration` | SCOPE-05 | Digest present in the core on every navigation-rendering surface; divergence fails | `./smackerel.sh test integration` |
| Alias uniqueness | `integration` | SCOPE-06 | One token resolves to one capability across every surface | `./smackerel.sh test integration` |
| Reachability end to end | `e2e-api` | SCOPE-04 | A capability added by one descriptor edit becomes reachable by plain-language request | `./smackerel.sh test e2e` |
| Withheld capability | `e2e-api` | SCOPE-03, SCOPE-07 | An unenforceable-grant capability is not surfaced and its absence leaks nothing | `./smackerel.sh test e2e` |
| Inventory retirement | `integration` | SCOPE-09 | No hand-maintained list survives beside its projection | `./smackerel.sh test integration` |

### Regression E2E rows

The rows below are **separate from, and additional to,** the first-pass rows above. A
first-pass row proves the behaviour once. A regression row is the coverage that stays in
the suite afterwards, so the defect this spec removes cannot return unnoticed.

Every regression row is scenario-specific and adversarial: it names the `SCN-112-*` ids it
protects and states the current-state condition it must fail against, because a regression
test whose fixtures all satisfy the broken behaviour cannot detect that behaviour's return.
The current-state conditions are the `E*` observations in `spec.md` §3.

| Test type | Category | Scope | What it must prove | Command |
|---|---|---|---|---|
| Regression E2E | `e2e-api` | SCOPE-01 | A catalog surface whose `capability_id` resolves to nothing is still reported — fails against the nil-seam state that disables the check (SCN-112-A05, E8) | `./smackerel.sh test e2e` |
| Regression E2E | `e2e-api` | SCOPE-02 | A dispatchable capability with no recorded exposure decision still fails rather than defaulting either way (SCN-112-B05) | `./smackerel.sh test e2e` |
| Regression E2E | `e2e-api` | SCOPE-03 | A capability whose grant cannot be enforced is still withheld, and its absence still leaks nothing (SCN-112-D03, D05) | `./smackerel.sh test e2e` |
| Regression E2E | `e2e-api` | SCOPE-04 | A capability is still reachable in plain language after one descriptor edit, with no reachability surface modified (SCN-112-A02, A03) | `./smackerel.sh test e2e` |
| Regression E2E | `e2e-ui` | SCOPE-05 | The digest is still in the guaranteed core on every navigation-rendering surface, and a surface-local append still does not count as core membership — fails against the six-link partial and eight-link PWA list (SCN-112-C01, C04, E11, E12, E14) | `./smackerel.sh test e2e-ui` |
| Regression E2E | `e2e-api` | SCOPE-06 | `/ask` still resolves to exactly one capability across every surface, and `/recipe` and `/cook` still agree with the registry (SCN-112-A02, E15, E16) | `./smackerel.sh test e2e` |
| Regression E2E | `e2e-api` | SCOPE-07 | The external projection still admits per declared grant rather than by blanket export (SCN-112-A02, D03, E19) | `./smackerel.sh test e2e` |
| Regression E2E | `e2e-api` | SCOPE-08 | A built-but-unregistered capability still fails the check, and a check that did not run is still reported as a failure — fails against a universe derived from the artifact under test (SCN-112-E05, E06, E9) | `./smackerel.sh test e2e` |
| Regression E2E | `e2e-api` | SCOPE-09 | A hand-maintained list still consulted beside its projection is still reported as a defect — fails against the documented "ALONGSIDE" state (SCN-112-A04, E10) | `./smackerel.sh test e2e` |

---

## Planning Constraints

- **Four BLOCKING findings gate work before it starts:** `F-112-UNIT-01` and
  `F-112-UNIVERSE-01` gate the design pass; `F-112-108-01` gates SCOPE-03 and everything
  downstream of it; `F-112-FLAG-01` gates delivery entirely and belongs to `bubbles.train`.
- **Nothing surfaces before its guard.** SCOPE-04, SCOPE-06 and SCOPE-07 all depend on
  SCOPE-03, which depends on a spec that is not delivered. Any plan that surfaces
  capabilities before that dependency resolves violates `spec.md` policy P4.
- **SCOPE-09 is not optional.** A plan that delivers projections and leaves the
  hand-maintained inventories in place has produced a fifth inventory, which `spec.md` §2
  names as the feature's failure condition.
- **Ordering against spec 108 and spec 109** is a real dependency on both, and both target
  the same `next` train.
