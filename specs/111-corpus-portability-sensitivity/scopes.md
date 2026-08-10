# Scopes: 111 Corpus Portability & Artifact Sensitivity

**Status:** `not_started` · **Workflow mode:** `product-to-planning` · **Release train:** `next`
**Owner of this artifact:** `bubbles.plan`
**Created by:** `bubbles.analyst` as an honest initial artifact so the packet is structurally complete

---

## What This File Is, And Is Not

**No scope has been planned, ratified, or started.** This file was created alongside
[`spec.md`](spec.md) during requirements authoring so the artifact set is complete and
lintable.

The decomposition below is a **starting shape derived directly from the requirement groups
in [`spec.md`](spec.md) §8** — it is not a plan of record. `bubbles.plan` owns this file and
may restructure, merge, split, or reorder anything here. Every Definition-of-Done item is
unchecked, every scope is `Not Started`, and no Test Plan row claims an existing test file,
because none exists.

Three findings in [`spec.md`](spec.md) §11 are **BLOCKING and gate planning itself**:
`F-111-FLAG-01` (no flag declared), `F-111-MANIFEST-01` (class list underivable as written),
and — for scope ordering — `F-111-108-01` and `F-111-110-01` (cross-spec dependencies).

---

## Scope Dependency Graph

```
SCOPE-01 (manifest)
   ├─> SCOPE-02 (traversal)  ─┐
   ├─> SCOPE-03 (export)     ─┼─> SCOPE-05 (round-trip parity)
   ├─> SCOPE-04 (import)     ─┤
   └─> SCOPE-06 (delete)     ─┘
SCOPE-07 (sensitivity) ─> SCOPE-08 (egress decision)
SCOPE-07 ─> SCOPE-03/04 (sensitivity must travel in a bundle)
```

---

## SCOPE-01 — Corpus manifest

**Status:** Not Started
**Depends On:** none
**Blocked by:** `F-111-MANIFEST-01` (BLOCKING) — the initial class list must be derived from the store, not hand-listed. `F-111-110-01` — spec 110's passage class must be included if 110 lands first.
**Requirements:** R-111-01 … R-111-07
**Scenarios:** SCN-111-A01, SCN-111-A02, SCN-111-A03

### Definition of Done

- [ ] A versioned manifest declares the corpus record classes, each with an identity key and a deterministic ordering key
- [ ] Export, import and delete each resolve scope from this manifest and carry no private class list
- [ ] A record class present in the store and absent from the manifest is detected and reported as a coverage defect
- [ ] SCN-111-A01 and SCN-111-A02 pass against the real store
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-02 — Stable, lossless traversal

**Status:** Not Started
**Depends On:** SCOPE-01
**Requirements:** R-111-10, R-111-11, R-111-12
**Scenarios:** SCN-111-B01, SCN-111-B02, SCN-111-B03, SCN-111-B04

### Definition of Done

- [ ] A complete paged traversal returns every record exactly once, including records sharing an identical ordering-key value
- [ ] The resume position preserves the full precision the store keeps
- [ ] A resume position presented to a traversal it does not belong to is refused rather than silently restarted
- [ ] Boundary scenario SCN-111-B01 (tied instants across a page boundary) passes and would fail against the current tiebreak-free traversal
- [ ] Boundary scenario SCN-111-B02 (sub-second precision across a resume) passes and would fail against a second-precision cursor
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-03 — Export against the manifest

**Status:** Not Started
**Depends On:** SCOPE-01, SCOPE-02, SCOPE-07
**Requirements:** R-111-06, R-111-08, R-111-09, R-111-23
**Scenarios:** SCN-111-A03, SCN-111-A04, SCN-111-C02, SCN-111-C06

### Definition of Done

- [ ] Export covers every manifest-declared class and no operation-specific subset
- [ ] Pending and failed captures are exported; membership does not depend on processing state
- [ ] The bundle names its manifest version and carries a per-class census including zero-count classes
- [ ] An unreadable class causes the export to report incompleteness and identify the class
- [ ] Artifact sensitivity and its provenance are carried in the bundle
- [ ] The existing export surface is corrected rather than supplemented; no second export path exists
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-04 — Import against the manifest

**Status:** Not Started
**Depends On:** SCOPE-01, SCOPE-07
**Requirements:** R-111-07, R-111-23
**Scenarios:** SCN-111-C01, SCN-111-C06

### Definition of Done

- [ ] Import covers exactly the manifest-declared classes
- [ ] A bundle whose manifest version is not understood is refused, applying nothing
- [ ] Sensitivity and provenance are restored as recorded at the source
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-05 — Proven round-trip parity

**Status:** Not Started
**Depends On:** SCOPE-03, SCOPE-04, SCOPE-06
**Blocked by:** `F-111-CENSUS-01` — canonical per-class hashing is undefined, so parity is not yet reproducible.
**Requirements:** R-111-13, R-111-14
**Scenarios:** SCN-111-C01, SCN-111-C02, SCN-111-C03

### Definition of Done

- [ ] Export then import into an empty destination yields equal per-class counts and equal per-class canonical hashes
- [ ] Boundary scenario SCN-111-C02 (a failed capture and a pending capture both survive the round trip with their processing state intact) passes
- [ ] A full delete afterwards leaves every manifest-owned class empty, verified after the fact
- [ ] Parity is verifiable by the owner from reported output alone, without inspecting internal storage
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-06 — Deletion at every scope

**Status:** Not Started
**Depends On:** SCOPE-01
**Blocked by:** `F-111-DELETE-01` — whether deleting an artifact deletes, orphans, or rewrites its derived records is undecided.
**Requirements:** R-111-14, R-111-15, R-111-16, R-111-17
**Scenarios:** SCN-111-C03, SCN-111-C04, SCN-111-C05

### Definition of Done

- [ ] Deletion is available at artifact, source, topic and whole-corpus scope
- [ ] A scoped delete removes its scope and leaves everything outside it present and unchanged
- [ ] Deletion requires deliberate confirmation and states the scope it would erase beforehand
- [ ] Post-delete emptiness is verified and reported as a result, not assumed
- [ ] Deletion requires no permission from any party other than the corpus owner
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-07 — Persisted sensitivity with provenance

**Status:** Not Started
**Depends On:** none
**Blocked by:** `F-111-BACKFILL-01` (BLOCKING for enforcement) — every existing artifact becomes unset, and unset is a refusal.
**Requirements:** R-111-18 … R-111-23
**Scenarios:** SCN-111-D01, SCN-111-D02, SCN-111-D03, SCN-111-D04

### Definition of Done

- [ ] Sensitivity is persisted as a property of the artifact, from a fixed vocabulary
- [ ] Provenance of the classification decision is stored with it
- [ ] Absence of classification is a distinct unset state and is not treated as least-sensitive
- [ ] A value outside the vocabulary is refused at write time, leaving the prior value unchanged
- [ ] Re-classification replaces provenance and never leaves a classified artifact without it
- [ ] The vocabulary's relationship to the sensitivity vocabularies already established for other record classes is explicit rather than a third independent scheme
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## SCOPE-08 — One fail-closed egress decision

**Status:** Not Started
**Depends On:** SCOPE-07
**Blocked by:** `F-111-108-01` (BLOCKING) — the decision consumes spec 108's grants, and spec 108 is `specs_hardened`, not delivered.
**Requirements:** R-111-24 … R-111-29
**Scenarios:** SCN-111-E01 … SCN-111-E07

### Definition of Done

- [ ] A single decision is evaluated before any external transmission of artifact content
- [ ] It consumes authenticated principal, grant coverage, credential audience, and per-artifact sensitivity
- [ ] Absent principal, uncovered grant, wrong audience, disallowed sensitivity, and unset sensitivity each produce refusal before any external call is made
- [ ] Every path capable of transmitting artifact content externally is governed by that same decision
- [ ] Every decision is recorded with principal, outcome and reason, permits and refusals alike
- [ ] Remote egress remains an explicit per-client operator grant, never global, default, or build-time
- [ ] The decision is not bypassable by configuration, environment, or build flag
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint and format clean, artifact lint clean, docs aligned

---

## Test Plan

**No test file exists for this feature.** The table records the coverage each scope must
carry when it executes. Every row names a category and the scenarios it must prove; none
names a path, because naming a path that does not exist would be a claim.

| Scope | Category | Must prove | Live system |
|---|---|---|---|
| SCOPE-01 | unit | Manifest declares classes with identity and ordering keys; export/import/delete resolve from it | No |
| SCOPE-01 | integration | An undeclared store class is detected and reported (SCN-111-A02) | Yes |
| SCOPE-02 | unit | Tied ordering keys and sub-second precision are handled by the resume position (SCN-111-B01, B02) | No |
| SCOPE-02 | integration | A full paged traversal returns each record exactly once at corpus scale (SCN-111-B03) | Yes |
| SCOPE-03 | integration | Export covers all classes including failed and pending captures (SCN-111-A03, A04, C02) | Yes |
| SCOPE-04 | integration | Import refuses an unknown manifest version and applies nothing | Yes |
| SCOPE-05 | e2e-api | Export → import → compare → delete → verify empty (SCN-111-C01, C02, C03) | Yes |
| SCOPE-06 | integration | Scoped delete removes its scope only (SCN-111-C04); confirmation is required (SCN-111-C05) | Yes |
| SCOPE-07 | unit | Vocabulary enforcement, unset distinctness, provenance replacement (SCN-111-D01…D04) | No |
| SCOPE-08 | unit | Each missing or disallowed input independently produces refusal (SCN-111-E01…E05) | No |
| SCOPE-08 | integration | No external call is made on refusal; every path is governed; decisions are recorded (SCN-111-E06, E07) | Yes |
| SCOPE-05 | stress | Resumability of export and delete across interruption at corpus scale (NFR-111-01) | Yes |

---

## Planning Constraints

| Constraint | Finding | Owner |
|---|---|---|
| No flag is declared. G111 requires the flag be default-OFF in every non-owning train's bundle before delivery begins. | `F-111-FLAG-01` (BLOCKING) | `bubbles.train` |
| SCOPE-01's class list must be derived from the store, not hand-written, or it is incomplete the day it ships. | `F-111-MANIFEST-01` (BLOCKING) | `bubbles.design` |
| SCOPE-08 cannot execute before spec 108's grant model exists. | `F-111-108-01` | `bubbles.plan` |
| SCOPE-01's manifest must include spec 110's passage class if 110 lands first. | `F-111-110-01` | `bubbles.design` |
| SCOPE-05 cannot be defined until canonical per-class hashing is settled. | `F-111-CENSUS-01` | `bubbles.design` |
| SCOPE-07's enforcement cannot be enabled before the backfill posture for existing unset artifacts is decided. | `F-111-BACKFILL-01` | `bubbles.design` |
