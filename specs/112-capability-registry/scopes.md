# Scopes: 112 Capability Registry

**Status:** `not_started` · **Workflow mode:** `product-to-planning` · **Release train:** `next`
**Owner of this artifact:** `bubbles.plan`
**Created by:** `bubbles.analyst` as an honest initial artifact so the packet is structurally complete

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

---

## SCOPE-01 — The capability descriptor record

**Status:** Not Started
**Depends On:** none
**Blocked by:** `F-112-UNIT-01` (BLOCKING) — what constitutes one capability, and how prompt contracts bind to capabilities, is undecided; the mapping is not 1:1.
**Requirements:** R-112-01 … R-112-07
**Scenarios:** SCN-112-A01, SCN-112-A05, SCN-112-A06

### Definition of Done

- [ ] One capability descriptor exists per capability, carrying stable id, intent phrasing, owning domain service, required principal and grants, provenance requirement, side-effect class, navigation projection, alias projection and external tool projection
- [ ] The record extends the existing experience catalog rather than standing beside it, and capability identity is authored in exactly one place
- [ ] The existing catalog's per-surface capability reference resolves to a descriptor, and an unresolvable reference is reported
- [ ] A capability's stable id survives changes to its phrasing, navigation position, alias and exposure class
- [ ] The record carries no user content, no readiness fact and no session scope
- [ ] SCN-112-A01, SCN-112-A05 and SCN-112-A06 pass
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-02 — Declared exposure for every built capability

**Status:** Not Started
**Depends On:** SCOPE-01
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
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-03 — Per-capability authorization

**Status:** Not Started
**Depends On:** SCOPE-01
**Blocked by:** `F-112-108-01` (BLOCKING) — the grant model belongs to spec 108, which is `specs_hardened`, not delivered. This scope cannot complete before that grant model exists.
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
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-04 — Assistant intent projection

**Status:** Not Started
**Depends On:** SCOPE-01, SCOPE-02, SCOPE-03
**Requirements:** R-112-08, R-112-10
**Scenarios:** SCN-112-A02, SCN-112-A03

### Definition of Done

- [ ] The natural-language intent set is generated from the descriptor set and is not authored by hand
- [ ] A capability declared user-facing becomes reachable by plain-language request without any reachability surface being edited
- [ ] No capability is surfaced whose authorization requirement is not enforceable
- [ ] SCN-112-A02 and SCN-112-A03 pass
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-05 — Navigation core projection, digest included

**Status:** Not Started
**Depends On:** SCOPE-01
**Blocked by:** `F-112-CUTOVER-01` (HIGH) — three navigation authorities are live and already divergent; whether this feature cuts them over or the shell spec does is undecided. `F-112-ISLANDS-01` (MEDIUM) — 29 of 31 PWA pages render no navigation at all.
**Requirements:** R-112-19 … R-112-22, R-112-11, R-112-12
**Scenarios:** SCN-112-C01, SCN-112-C02, SCN-112-C03, SCN-112-C04

### Definition of Done

- [ ] The daily digest is a member of the guaranteed cross-surface navigation core
- [ ] Every member of the guaranteed core is present on every surface that renders navigation
- [ ] A capability present only in a surface-local navigation addition is reported as absent from the core and is not treated as cross-surface reachable
- [ ] A divergence between two surfaces that are both projections of the same descriptor set fails rather than passing silently
- [ ] A claim by one surface that it mirrors another is verified against the generated core rather than asserted in a comment
- [ ] The set of surfaces that render no navigation is identifiable, so the core's reach is a known quantity
- [ ] SCN-112-C01 through SCN-112-C04 pass
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-06 — Alias projection

**Status:** Not Started
**Depends On:** SCOPE-01, SCOPE-02, SCOPE-03
**Blocked by:** `F-112-ALIAS-01` (HIGH) — the alias table and the assistant registry currently contradict each other on `/ask`, `/recipe` and `/cook`. Generating both from one descriptor forces a decision that changes shipped user-visible behaviour for at least one token.
**Requirements:** R-112-08, R-112-10
**Scenarios:** SCN-112-A02, SCN-112-A03

### Definition of Done

- [ ] The alias table is generated from the descriptor set and is not authored by hand
- [ ] The `/ask` contradiction is resolved by an explicit recorded decision, not silently by generation order
- [ ] The status of `/recipe` and `/cook` is decided explicitly and recorded
- [ ] An alias resolves to exactly one capability, and two aliases naming different capabilities for the same token is impossible by construction
- [ ] SCN-112-A02 and SCN-112-A03 pass for the alias surface
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-07 — External tool projection

**Status:** Not Started
**Depends On:** SCOPE-01, SCOPE-02, SCOPE-03
**Blocked by:** `F-112-109-01` (HIGH) — spec 109 forbids deriving the tool list by passthrough, so the projection must be per-capability and declared. `F-112-108-01` (BLOCKING) — external exposure is a grant decision under Principle 11.
**Requirements:** R-112-08, R-112-25, R-112-26
**Scenarios:** SCN-112-A02, SCN-112-D03

### Definition of Done

- [ ] The external tool projection is generated per capability from the descriptor set, and is not a blanket export of the capability set
- [ ] Exposure of a capability to an external client is a declared grant decision, never a generation side effect
- [ ] A capability whose grant cannot be enforced does not appear in the projection, and its absence leaks nothing about its existence
- [ ] The projection satisfies spec 109's constraint that no tool is derived by passthrough
- [ ] SCN-112-A02 and SCN-112-D03 pass for the external surface
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-08 — The coverage check

**Status:** Not Started
**Depends On:** SCOPE-01, SCOPE-02, SCOPE-03
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
