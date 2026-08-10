# User Validation: 112 Capability Registry

**Status:** `not_started` · **Workflow mode:** `product-to-planning` · **Release train:** `next`
**Status of this file at authoring time:** requirements baseline only. Nothing designed, planned, implemented or executed.

## How To Use This File

Every entry below is **checked `[x]` by default**. Each records a statement that was
established during requirements authoring and is currently believed true.

**Uncheck an item `[ ]` to report that it is wrong.** An unchecked item is a user-reported
regression and is BLOCKING: no further scope work proceeds until it is investigated and
resolved.

At this point in the feature's life the checklist validates the **requirements and the
verified source observations**, not a running system. Entries covering generated
projections, reachability of newly surfaced capabilities, navigation parity, or coverage
enforcement are deliberately **absent** rather than pre-checked without execution. They are
added when the scopes execute and real evidence exists.

## Checklist

### Problem framing

- [x] The stated problem is real: the system can do more than a user can ask it to do, and the gap is reachability rather than capability.
- [x] Framing the root cause as "reachability is described in four places that cannot see each other" is more accurate than framing it as "capabilities are missing intents".
- [x] It is correct that forgetting one of the four edits is silent — no build fails, no test fails, and the capability is simply unreachable from that surface.
- [x] Treating the registry as **half-built rather than missing** is the right framing, and materially changes the size and shape of the work.
- [x] Explicitly rejecting a second registry beside the existing catalog — because it would recreate the exact duplication this feature removes — is the right call.

### Verified source observations

- [x] It is correct that 27 scenario contracts are built and that exactly 5 are declared user-facing.
- [x] It is correct that every surface in the existing catalog already carries a capability reference, and that **no capability record exists for those references to resolve to**.
- [x] It is correct that the existing validator already has a capability-membership seam and already emits an unknown-capability violation, so the extension point exists.
- [x] It is correct that the only production-shaped test derives the expected capability universe from the very catalog it is validating, making that check circular and unable to fail.
- [x] It is correct that the catalog package's own documentation records that the handwritten navigation authorities remain active alongside the generated catalog, with cutover deferred.
- [x] It is correct that the PWA navigation and the shared server partial have **already drifted**, and that the PWA comment claiming it mirrors the server partial is false as written.
- [x] It is correct that the slash table and the assistant registry disagree about what `/ask` does — the same token naming two different targets in two files.
- [x] It is correct that the slash table ships `/recipe` and `/cook` while the assistant registry records that the shortcut set contains neither.
- [x] It is correct that the digest is absent from the shared navigation core and from the PWA navigation, while being present in the server page shell's own additions.
- [x] It is correct that 29 of 31 first-party PWA pages render no shared navigation at all.
- [x] It is correct that spec 108, which owns the grant model this feature's authorization depends on, is planned rather than delivered.

### The count correction

- [x] Sharpening the unsurfaced count from 22 to **11** — because ten of the contracts are pipeline stages carrying no routable id and one is a test harness — is the right handling, and recording the derivation rather than restating the brief's figure was correct.
- [x] It is correct that a prompt contract is not the same unit as a capability, and that assuming a one-to-one mapping would embed the wrong unit in the registry.
- [x] It is correct that the important defect is not the size of the number but that **the repository cannot tell you which number is right**, because nothing records whether a contract is unsurfaced deliberately or by omission.
- [x] Preserving `D18` as a core-membership finding rather than restating it as "the digest is unreachable" is the right level of precision.

### Requirements shape

- [x] Requiring exactly one descriptor per capability, with every reachability surface generated from it, is the right mechanism.
- [x] Requiring that a hand-maintained inventory surviving beside its generated projection be treated as a **defect rather than a transitional state** is the right bar — anything softer produces a fifth inventory.
- [x] Requiring exposure to be declared with a closed vocabulary, so that "not listed" stops being a way to be invisible, is correct.
- [x] Requiring a recorded reason for every non-user-facing class — rather than allowing silent omission — is worth the extra field.
- [x] Requiring that a structurally non-dispatchable pipeline stage be distinguishable from an accidental omission is necessary, because today they are indistinguishable without reading each file.
- [x] Requiring that a newly built dispatchable capability with no recorded decision **fail validation** rather than default to exposed or to hidden is the right default.
- [x] Requiring the digest in the guaranteed core, and requiring every core member to appear on every navigation-rendering surface, is the right pair — either alone is insufficient.
- [x] Requiring that a surface's claim to mirror another be verified against the generated core rather than asserted in a comment is directly justified by the drift already found.
- [x] Requiring authorization to be per capability and server-derived, with caller-asserted authority never trusted, is the right shape.
- [x] Requiring that a capability whose guard cannot be enforced **not be surfaced at all** is correct, and is the constraint that makes the release-train choice follow.
- [x] Requiring the coverage check to compare against an **independently derived** universe is the single most important requirement, because a check derived from the artifact under test cannot fail.
- [x] Requiring the check to name the specific capability and the specific missing facet, rather than report an aggregate, is the right failure ergonomics.
- [x] Requiring that a check which did not run be reported as a failure rather than a pass is correct.

### Boundaries

- [x] Excluding the grant vocabulary's definition, the external tool server itself, navigation information-architecture redesign, and the retrofit of shared navigation onto 29 island pages is the right boundary.
- [x] Declining to decide which exposure class each of the eleven capabilities receives — while requiring that every one carry a declared class and a reason — is the right split between product contract and operator decision.
- [x] Consuming spec 108's grant model and spec 109's tool-surface constraint rather than redefining either is the right relationship between the three specs.

### Release train and honesty

- [x] Targeting the `next` train is right, because surfacing capabilities raises reach while the authorization boundary that bounds it is planned rather than delivered.
- [x] It is right that shipping this on `mvp` would widen natural-language reach on the live host ahead of the guard that is supposed to bound it.
- [x] It is right that replacing three live and already-divergent navigation authorities should be proven on staging before the live host.
- [x] Stating plainly that the `mvp` train keeps the reachability gap for longer — rather than letting the train choice obscure that cost — is the right handling.
- [x] Declaring `flagsIntroduced` as an empty array, and routing the flag declaration to `bubbles.train` as a BLOCKING finding, is more honest than naming a flag with no backing bundle entry.
- [x] Recording that `.specify/templates/spec-template.md` does not exist in this repository — instead of claiming conformance to a file that could not be read — was the right handling.
- [x] Recording that sibling spec 110 has no `state.json`, while **not modifying it**, was the right handling of an observation made outside this run's permitted surface.
- [x] Recording `Principle 6 — Invisible By Default, Felt Not Heard` as a **tension rather than an alignment**, because this feature raises how much the product surfaces, is more honest than listing it as supportive.
