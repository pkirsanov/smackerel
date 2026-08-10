# User Validation: 110 Retrieval Quality Foundation

**Status:** `not_started` · **Workflow mode:** `full-delivery` · **Release train:** `mvp`
**Status of this file at authoring time:** requirements baseline only. Nothing designed, planned, implemented or executed.

## How To Use This File

Every entry below is **checked `[x]` by default**. Each records a statement that was
established during requirements authoring and is currently believed true.

**Uncheck an item `[ ]` to report that it is wrong.** An unchecked item is a user-reported
regression and is BLOCKING: no further scope work proceeds until it is investigated and
resolved.

At this point in the feature's life the checklist validates the **requirements**, not a
running system. Entries covering retrieval behaviour, measured accuracy, query plans and
latency are deliberately **absent** rather than pre-checked without execution. They are
added when the scopes are executed and real evidence exists.

## Checklist

### Problem framing

- [x] The stated problem is real: a user can save a document and then fail to find a fact that is inside it.
- [x] Treating this as four compounding defects in one substrate — granularity, indexed text, model identity, index behaviour — is a more accurate framing than four independent bugs.
- [x] "We do not know whether the vector index is used" is itself a legitimate finding, because both possible answers are unacceptable.
- [x] Retrieval quality is worth fixing before further capability expansion, because Pillars A and C both consume it.

### Evidence

- [x] It is correct that no chunk table exists anywhere in the migration set.
- [x] It is correct that the embedded text is title, summary and up to five key ideas, so a fact the extraction step omitted is unreachable by vector search rather than merely ranked low.
- [x] It is correct that the declared embedding model and the model the runtime loads disagree, and that neither side fails today.
- [x] It is correct that the stored vector column width matches the hardcoded model rather than the declared one, so a model change is also a column-width migration.
- [x] It is correct that the probe count is never set and that no approximate-nearest-neighbour graph index exists.
- [x] Stating openly that no query plan was executed and no latency was measured is preferable to reporting an assumed plan.

### Requirements shape

- [x] Requiring a retrieval result to cite a **verbatim** span of stored content, rather than a generated summary of the match, is the right bar.
- [x] Requiring startup to refuse when declared identity, running model and stored vector width disagree — rather than warn — is proportionate.
- [x] Requiring re-embedding to be resumable and exactly-once per item, rather than a one-shot backfill, is worth the extra complexity.
- [x] Requiring an evaluation set of at least 100 genuinely vague queries with known answer ids, rather than a smaller set of precise ones, is the right measurement contract.
- [x] Requiring the gate to report a non-zero executed-assertion count, and to fail when that count is zero, is a necessary response to the existing gate that runs in no lane.
- [x] Requiring quality floors to originate from an executed measurement, and requiring a lowered floor to be an explicit decision, is preferable to letting the gate adopt whatever it last measured.

### Boundaries

- [x] Excluding graph edge semantics from this spec, and touching the temporal linker only for the bounded-lookup defect, is the right boundary.
- [x] Excluding digest and synthesis ranking from this spec is the right boundary.
- [x] Refusing to create a second retrieval store, and extending the existing graph instead, correctly follows Product Principle 5.
- [x] Leaving the choice of embedding model to the operator, while making identity declared, verified and migratable, is the right split of responsibility.

### Release train and honesty

- [x] Targeting the `mvp` train is right, because this is a correctness fix to a promise the currently-deployed train already makes.
- [x] Introducing exactly one flag, gating the read-path cutover during the re-embed window, is proportionate rather than ceremonial.
- [x] Recording that passages create a second copy of user content — and that this weakens Principle 11's unconditional-exit guarantee until spec 111 covers it — is better than leaving the interaction unstated.
- [x] Recording `F-110-DIM-01` as a finding the plan of record does not name, rather than silently absorbing it, is the correct handling.
