# User Validation: 111 Corpus Portability & Artifact Sensitivity

**Status:** `not_started` · **Workflow mode:** `product-to-planning` · **Release train:** `next`
**Status of this file at authoring time:** requirements baseline only. Nothing designed, planned, implemented or executed.

## How To Use This File

Every entry below is **checked `[x]` by default**. Each records a statement that was
established during requirements authoring and is currently believed true.

**Uncheck an item `[ ]` to report that it is wrong.** An unchecked item is a user-reported
regression and is BLOCKING: no further scope work proceeds until it is investigated and
resolved.

At this point in the feature's life the checklist validates the **requirements and the
verified source observations**, not a running system. Entries covering export completeness,
import parity, deletion, sensitivity enforcement, or egress refusal behaviour are
deliberately **absent** rather than pre-checked without execution. They are added when the
scopes execute and real evidence exists.

## Checklist

### Problem framing

- [x] The stated problem is real: the product promises unconditional exit and cannot currently deliver it on any of export, import, or delete.
- [x] Treating export, import and delete as three readings of one question — what is the corpus? — is a more accurate framing than three separate features.
- [x] It is correct that a portability contract which moves records without carrying their sensitivity has moved the data and dropped the policy.
- [x] Refusing to add a second "full export" path beside the existing one, and correcting the existing one instead, is the right call.

### Verified source observations

- [x] It is correct that the export surface filters to a single processing state, so pending and failed captures are silently omitted.
- [x] It is correct that many corpus record classes — relationships, digests, synthesis output, annotations, collections, topics, people, action items — are unreachable through the export surface entirely.
- [x] It is correct that pagination compares creation time strictly and carries no tiebreak, so records sharing an exact instant across a page boundary are skipped.
- [x] It is correct that the cursor is serialised at second precision while the store keeps sub-second precision, and that this re-returns records.
- [x] It is correct that these two cursor defects pull in opposite directions, so a user cannot tell from the output whether records were skipped or duplicated.
- [x] It is correct that no corpus delete surface exists at artifact, source, topic, or whole-corpus granularity.
- [x] It is correct that artifact sensitivity is a soft convention in a free-form metadata document with no column, no constraint, and no provenance.
- [x] It is correct that canonical sensitivity columns already exist for other record classes, which makes the artifacts gap an inconsistency with the repository's own pattern rather than an unexplored question.
- [x] It is correct that no remote-egress decision surface exists anywhere today.
- [x] Correcting the two wrong line references supplied in the authoring brief, and recording the corrections rather than copying them, was the right handling.
- [x] Recording that `.specify/templates/spec-template.md` does not exist in this repository — instead of claiming conformance to a file that could not be read — was the right handling.

### Requirements shape

- [x] Requiring one versioned manifest to own export, import and delete identically is the right mechanism; a per-operation class list is what lets failed captures disappear.
- [x] Requiring that a record class present in the store but absent from the manifest be *detectable* is necessary, because otherwise the manifest silently rots as the product grows.
- [x] Requiring that corpus membership never depend on processing state — so a failed capture is still the user's — is correct.
- [x] Requiring an export that cannot cover a class to report that fact, rather than return a success meaning less than the caller thinks, is the right bar.
- [x] Requiring deletion to prove emptiness after the fact, rather than report that a delete operation returned without error, is the right success criterion.
- [x] Requiring deliberate confirmation before deletion, in a product whose principle is otherwise to stay invisible, is a proportionate exception for an irreversible operation.
- [x] Requiring sensitivity to carry the provenance of the decision, not just the value, is worth the extra field.
- [x] Requiring unset sensitivity to be a distinct state that the egress decision treats as a refusal — rather than as the least sensitive value — is the right default.
- [x] Requiring one fail-closed egress decision that validates principal, grant, credential audience and sensitivity together, before any external call, is the right shape.
- [x] Requiring that permitting decisions be recorded with the same fidelity as refusals is right; an audit log of only refusals cannot answer what left.

### Boundaries

- [x] Excluding bundle encryption, cross-version bundle migration, and merge-into-non-empty import from this spec is the right boundary.
- [x] Declining to decide which artifacts are sensitive — while requiring that sensitivity be stored, constrained, provenanced and consulted — is the right split between product contract and operator policy.
- [x] Consuming spec 108's grant model rather than redefining it is the right relationship between the two specs.

### Release train and honesty

- [x] Targeting the `next` train is right, because this introduces a fail-closed enforcement point — the same change class spec 108 explicitly kept off `mvp`.
- [x] It is right that shipping this egress gate on `mvp` while the grant model it validates against lives on `next` would split one security posture across two trains.
- [x] It is right that an irreversible whole-corpus delete should be proven on staging before it is exposed on the live self-hosted host.
- [x] Declaring `flagsIntroduced` as an empty array — and routing the flag declaration to `bubbles.train` as a BLOCKING finding — is more honest than naming a flag with no backing bundle entry.
- [x] Stating plainly that the `mvp` train remains non-conformant with Principle 11's exit guarantee until promotion, rather than letting the train choice obscure it, is the right handling.
- [x] Binding all copy for this capability to Principle 11's honesty constraint — never claiming the product enforces, verifies, guarantees or attests client-side inference locality — is correct and non-negotiable.
