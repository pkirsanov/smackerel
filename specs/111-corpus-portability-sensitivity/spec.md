# Feature: 111 Corpus Portability & Artifact Sensitivity

**Status:** `not_started` · **Workflow mode:** `product-to-planning` · **Release train:** `next`
**Authored by:** `bubbles.analyst` (requirements only — no source file changed, no test run)
**Governing product rule:** [`docs/Product-Principles.md`](../../docs/Product-Principles.md) **Principle 11 — Local-First Data Ownership** (ratified 2026-07-29)
**Diagnostic findings:** D12, D23, D11
**Inbound routing:** [`specs/110-retrieval-quality-foundation/spec.md`](../110-retrieval-quality-foundation/spec.md) `F-110-EGRESS-01` routes the passage record class here
**Evidence verified:** 2026-08-04 against the working tree (§3)

---

## 1. Problem Statement

Principle 11 makes two promises in the product's own words. The product currently keeps
neither.

> *"Ownership also means **exit is unconditional**: the user can export, relocate, or
> delete the entire corpus without asking permission, and accumulated value must never
> become a switching barrier."*

> *"Where a capability lets an **authorized external client** read the corpus, **local
> inference is the default** and remote inference is an **explicit, per-client, audited
> operator grant** — never a default, never a build-time switch, never silent."*

**Promise one fails three ways at once.**

1. **Export is not an export.** The one export surface emits rows from a single table,
   filtered to a single processing state. Every capture still pending, every capture that
   **failed**, and every other corpus record class — graph edges, digests, synthesis
   output, annotations, lists, topics, people, action items — is silently absent. The
   omission is silent: the response carries no statement of what it excluded, so a user
   who exports and then deletes believes they took everything with them.

2. **The export that does run is not reliably complete within its own table.** Pagination
   advances on a timestamp alone, compares it strictly, and serialises it at coarser
   precision than the store keeps. Those are two independent defects pulling in opposite
   directions: one **skips** records, the other **duplicates** them. A user cannot tell
   which happened, because a correct export and a lossy one are the same shape.

3. **There is no delete.** Not per-artifact, not per-source, not per-topic, not
   whole-corpus. The word *delete* appears in the principle as an unconditional
   guarantee, and there is no surface that performs it at any granularity.

**Promise two has no enforcement point.** The design treats artifact sensitivity as the
thing that governs model routing and external egress, but sensitivity is not a persisted
property of an artifact. It is a soft convention inside a free-form metadata document,
with no column, no constraint, no vocabulary enforcement, and no record of who decided it
or how. A reader that cannot find it treats the artifact as unclassified and moves on.
A policy that cannot be read cannot be enforced, so today there is no point in the system
where a request to an external model is refused because of what the artifact contains.

**Why these belong in one spec rather than three.** Export, import and delete are not
three features that happen to be related. They are **three readings of one question:
what, exactly, is the corpus?** Today only export answers it, and it answers wrongly.
If delete is built against a different answer than export, then the failed captures that
export already drops become records that delete also misses — invisible to the user, present
on disk, and still readable by an egress path. Sensitivity belongs with them because it is
the per-record property that decides whether a record may leave at all; a portability
contract that moves records without carrying their sensitivity has moved the data and
dropped the policy.

**What this spec refuses to do.** It does not add a second export path beside the existing
one. A "full export" alongside a "normal export" would leave two answers to the same
question and guarantee they diverge again.

---

## 2. Outcome Contract

**Intent.** A user can take their entire corpus out, put it back somewhere else, and erase
it from the original — without asking permission, without losing records to a silent
filter, and without discovering afterwards that something they could not see was left
behind. Separately, no artifact reaches an external model unless an authenticated,
granted, audience-correct request has been checked against that artifact's recorded
sensitivity first.

**Success Signal.**
1. A corpus containing records of every manifest-owned class — including at least one
   pending capture and one **failed** capture — survives an export, an import into an
   empty destination, and a comparison: per-class record counts are equal and the
   canonical content hash of each class is equal.
2. After a full delete against that same manifest, every manifest-owned record class is
   empty.
3. An export spanning a page boundary where several records share one identical creation
   instant returns each of those records exactly once — not zero times, not twice.
4. A request that would send artifact content to an external model is refused before any
   external call when the principal is absent, the grant is absent or does not cover the
   request, the credential's intended audience does not match, or the artifact's
   sensitivity is unset or disallowed by policy.

**Hard Constraints.**
- Export, import and delete are defined by **one** versioned manifest. A record class is
  in all three or in none. There is no operation-specific class list.
- A record class present in the store and absent from the manifest is a **defect in the
  manifest**, and the system must be able to say so rather than fail silently.
- Export completeness is not conditional on processing state. A failed capture is part of
  the user's corpus.
- The egress decision fails **closed**. Missing input is refusal, never permission.
- Sensitivity is stored with the artifact, from a fixed vocabulary, and carries the
  provenance of the decision.
- Delete is irreversible by design and must never be reachable by accident.

**Failure Condition.** The feature has failed — even with every test green — if any of the
following is true: a user exports, imports, deletes, and finds a record class that survived
the delete or never arrived at the destination; the product reports a successful export that
silently omitted records; sensitivity exists as a column but no egress path consults it,
making it decorative; or the egress gate can be satisfied by an artifact whose sensitivity
was never set.

---

## 3. Evidence Base (verified 2026-08-04 against the working tree)

Every row below was re-read at authoring time. Two line references supplied in the
originating brief were checked and **corrected**; the corrections are recorded rather than
quietly adopted.

| # | Claim | Verified location | Note |
|---|---|---|---|
| **E1** | The export query filters to a single processing state, so pending and failed captures are excluded | `internal/db/postgres.go:122` — `WHERE processing_status = 'processed' AND created_at > $1` | Brief cited `:92`; **actual is `:122`**. Claim holds, reference corrected. |
| **E2** | Export reads one table; the store holds many other corpus record classes | `internal/db/migrations/*.sql` declare `edges`, `digests`, `synthesis_insights`, `annotations`, `lists`, `list_items`, `topics`, `people`, `action_items`, `knowledge_concepts`, `knowledge_entities` among others | None of these is reachable through the export surface |
| **E3** | Pagination orders and compares on creation time only, strictly greater | `internal/db/postgres.go:122-123` — `created_at > $1` … `ORDER BY created_at ASC` | Records sharing the boundary instant are skipped: no tiebreak key |
| **E4** | The cursor is serialised and re-parsed at second precision | `internal/api/capture.go:388` emits `Format(time.RFC3339)`; `:356` parses `time.Parse(time.RFC3339, …)` | Truncation moves the cursor *backwards*, re-returning records |
| **E5** | The store keeps creation time at sub-second precision | `internal/db/migrations/001_initial_schema.sql:39` — `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()` | E4 and E5 together are the duplication defect |
| **E6** | There is no corpus delete surface at any granularity | `internal/api/router.go` — export is `:105`; the only delete routes cover annotation tags, lists, list items, preference corrections, watches, evidence exports, drive rules, and agent model config | No artifact, source, topic, or whole-corpus delete exists |
| **E7** | The artifacts table has no sensitivity column | `internal/db/migrations/001_initial_schema.sql:16-63` (full `artifacts` declaration) | Brief cited `:24`; the correct claim is that **no line in the whole declaration** defines one |
| **E8** | Artifact sensitivity is a soft convention in a free-form metadata document, and unreadable values are silently skipped | `internal/knowledge/sensitivity_query.go:26-30` — reads `artifact.metadata->>'sensitivity_tier'`; "rows with missing or out-of-vocabulary metadata are conservatively skipped" | Skipping is safe for that reader and useless as an enforcement point |
| **E9** | Canonical sensitivity columns already exist for other record classes | `drive_files.sensitivity` (with a CHECK constraint) and the `photo_sensitivity` enum | The artifacts gap is an **inconsistency with the repo's own established pattern**, not an unexplored design question |
| **E10** | No remote-egress decision surface exists | Repository-wide search for a remote-inference / remote-egress decision or audit symbol returns nothing outside test files | There is no place today where such a decision could be recorded |
| **E11** | A sibling spec has already routed a record class to this spec | `specs/110-retrieval-quality-foundation/spec.md:411` — `F-110-EGRESS-01` … "Interacts directly with spec 111's bundle record classes" | Passages are a second copy of user content and must enter the manifest |
| **E12** | Principle 11 is ratified and binding | `docs/Product-Principles.md:185` — "Ratified 2026-07-29"; `.github/instructions/product-principles.instructions.md` marks it BLOCKING | Not aspirational |

**One premise could not be verified.** The authoring brief instructed that this spec follow
`.specify/templates/spec-template.md`. **That file does not exist in this repository**
(`.specify/templates/` is absent; the template exists only in sibling repositories). This
spec therefore follows the structure established by the adjacent spec in this repository
and the Bubbles BDD scenario contract, and records the missing template as finding
`F-111-TEMPLATE-01` rather than claiming conformance to a file it could not read.

---

## 4. Domain Capability Model

The capability being introduced is **corpus portability**, and it has more than one
implementation surface from the outset (export, import, delete — and later, additional
record classes). Modelling it provider-first would reproduce exactly the divergence this
spec exists to remove, so the domain model is defined before any surface.

### 4.1 Primitives

| Primitive | Definition | Lifecycle |
|---|---|---|
| **Corpus** | The complete set of user-owned knowledge records the product holds. Defined by the manifest, not by any one operation's query. | Continuous; exists until deleted |
| **Record class** | One named, independently countable kind of corpus record (captures, relationships, derived summaries, user annotations, curated collections, and their kin). | Declared in a manifest version; may be added in later versions, never silently dropped |
| **Manifest** | The versioned declaration of which record classes constitute the corpus, and of each class's identity key, ordering key, and canonical serialised form. | Versioned; a bundle names the version it was written under |
| **Bundle** | A self-describing, transportable serialisation of a corpus at one point in time, naming its manifest version and carrying a per-class census. | Created by export; consumed by import; compared for parity |
| **Page cursor** | A resumable position within one record class's ordered traversal. | Opaque to the holder; valid across the traversal it belongs to |
| **Sensitivity** | A recorded classification of what a record contains, from a fixed vocabulary, together with the provenance of the decision. | Set at or after capture; may be re-decided, with provenance replaced, never blanked to unset |
| **Egress decision** | A single verdict, taken before any external transmission, on whether specified content may leave. | Per request; recorded whether it permits or refuses |
| **Grant** | An operator-authorised, per-client permission that names what a principal may do. | Explicitly granted; revocable; audited |

### 4.2 Relationships

- A **manifest** enumerates **record classes**. Export, import and delete each take the
  manifest as their *only* definition of scope. None of them carries a private list.
- A **bundle** is written against exactly one **manifest version** and must state it. An
  importer that does not understand that version refuses; it never partially applies.
- Every **record class** declares an identity key (what makes two records the same record)
  and an ordering key (how a traversal proceeds deterministically). A **page cursor** is
  meaningful only in terms of its class's ordering key.
- **Sensitivity** attaches to an artifact and travels with it in a bundle. A record whose
  sensitivity is lost in transit has become *less* classified by moving — which is a
  portability defect, not a sensitivity defect.
- An **egress decision** consumes a principal, a grant, a credential audience, and the
  **sensitivity** of every record in scope. Absence of any input is a refusal.

### 4.3 Policies every implementation must obey

- **P1 — One definition.** Export, import and delete resolve their scope from the same
  manifest version. Divergence between them is a defect in the implementation, not a
  configuration choice.
- **P2 — No state-based exclusion.** Membership of the corpus never depends on how far a
  record got through processing. A failed capture is the user's.
- **P3 — Silence is forbidden.** An operation that cannot cover a manifest-declared class
  reports that fact. It does not return a success that means less than the caller thinks.
- **P4 — Traversal is total and exact.** Every record in a class is visited exactly once
  across a complete paged traversal, regardless of how many records share a value of the
  ordering key.
- **P5 — Deletion is proven by absence.** A delete's success criterion is that the
  manifest-owned classes it named are empty, verified after the fact — not that a delete
  operation returned without error.
- **P6 — Sensitivity is a property, not an annotation.** It is stored with the record, drawn
  from a fixed vocabulary, and accompanied by the provenance of the decision.
- **P7 — Egress fails closed.** The decision is taken once, before transmission, and every
  missing or disallowed input produces refusal. There is no permissive fallback.
- **P8 — Growth is a manifest change.** Introducing a new content-bearing record class
  requires adding it to the manifest in the same change. A class that exists in the store
  but not the manifest is a detectable defect.

---

## 5. Actors & Personas

| Actor | Description | Goals | Boundary |
|---|---|---|---|
| **Corpus owner** | The person whose knowledge the corpus holds; self-hosting operator of their own instance | Leave with everything; arrive somewhere else with everything; erase the original and be certain | Needs no permission from anyone (Principle 11); needs certainty, not reassurance |
| **Relocating owner** | A corpus owner mid-migration between two instances | Reconstruct the corpus at the destination and confirm it matches before erasing the source | Cannot inspect internal storage; relies entirely on what the product reports |
| **Authorised external client** | A client the operator explicitly granted corpus read access | Read what it was granted | Must never receive content its grant, audience, or the record's sensitivity disallows |
| **Operator (policy role)** | The same human acting in an administrative capacity | Decide which sensitivities may leave, and to whom; be able to review what left | Grants are explicit and per-client; never global, never implicit |
| **Auditor** | The owner reviewing after the fact | Determine what left, when, for whom, and under which decision | Reads recorded decisions; does not re-derive them |

---

## 6. Use Cases

### UC-111-001 — Owner leaves with the whole corpus
**Actor:** Corpus owner
**Preconditions:** A corpus containing records of several classes, including at least one capture that failed processing.
**Main flow:** The owner requests an export → the system produces a bundle naming its manifest version → the bundle carries a per-class census → the owner compares the census to what the system reports the corpus contains → they match.
**Alternative flow:** A manifest-declared class cannot be read → the export reports the failure and does not present itself as complete.
**Postcondition:** The owner holds a bundle they can verify without trusting a claim.

### UC-111-002 — Owner relocates to a new instance
**Actor:** Relocating owner
**Preconditions:** A verified bundle; an empty destination.
**Main flow:** The owner imports the bundle → the destination reports per-class counts and canonical hashes → they equal the bundle's census → the owner confirms parity before touching the source.
**Alternative flow:** The bundle's manifest version is not understood → the import refuses entirely and changes nothing.
**Postcondition:** Two corpora demonstrably carry the same records.

### UC-111-003 — Owner erases the original
**Actor:** Corpus owner
**Preconditions:** Parity confirmed at the destination.
**Main flow:** The owner requests deletion of the whole corpus → the system states exactly which classes will be erased → the owner confirms deliberately → deletion runs → the system verifies and reports that every named class is empty.
**Alternative flow:** The owner deletes a narrower scope — one source, one topic, one artifact — and the same statement, confirmation, and verification apply to that scope.
**Postcondition:** Nothing manifest-owned remains, and the owner has been shown that.

### UC-111-004 — Owner classifies what an artifact contains
**Actor:** Corpus owner / operator
**Preconditions:** An artifact exists.
**Main flow:** A sensitivity is recorded for the artifact from the fixed vocabulary, together with how it was decided → the classification is visible → it travels with the artifact in any bundle.
**Alternative flow:** No classification has been made → the artifact is *unset*, which is a distinct state and is treated as disallowed by the egress decision rather than as permissive.
**Postcondition:** Sensitivity is an inspectable property with a traceable origin.

### UC-111-005 — Authorised client requests content that must not leave
**Actor:** Authorised external client
**Preconditions:** A grant exists; the request would send artifact content to an external model.
**Main flow:** The egress decision evaluates principal, grant coverage, credential audience, and every in-scope artifact's sensitivity → any failing input produces refusal → no external call is made → the decision is recorded.
**Alternative flow:** All inputs pass → the request proceeds and the permitting decision is recorded with the same fidelity as a refusal.
**Postcondition:** No content left without a recorded decision that examined it.

---

## 7. Business Scenarios (Gherkin)

### One manifest, three operations

#### SCN-111-A01 — Export, import and delete cover identical record classes
```gherkin
Given a manifest version that declares the record classes making up the corpus
When the set of classes covered by export, by import, and by delete is compared
Then all three sets are identical to the manifest's set
  And no operation covers a class the others do not
```

#### SCN-111-A02 — A record class in the store but not the manifest is reported
```gherkin
Given a corpus record class that exists in the store
  And that class is not declared in the current manifest version
When the corpus is examined for portability coverage
Then the undeclared class is reported as a coverage defect
  And no operation reports itself as covering the whole corpus
```

#### SCN-111-A03 — An export states what it contains
```gherkin
Given a corpus containing records of several declared classes
When an export is produced
Then the resulting bundle names the manifest version it was written under
  And it carries a count for every declared class, including classes with no records
```

#### SCN-111-A04 — A class that cannot be read fails the export honestly
```gherkin
Given a manifest-declared record class that cannot be read during an export
When the export runs
Then the export does not present itself as complete
  And it identifies the class it could not cover
```

### Traversal correctness (boundary)

#### SCN-111-B01 — Records sharing one instant across a page boundary are all returned
```gherkin
Given several records whose creation instants are exactly identical
  And a page size that places that identical instant on a page boundary
When the full corpus is traversed page by page to the end
Then every one of those records is returned
  And none of them is returned more than once
```

#### SCN-111-B02 — Sub-second precision survives the page boundary
```gherkin
Given records whose creation instants differ only by a fraction of a second
  And a traversal that pauses at a page boundary between two of them
When the traversal resumes from the position it was given
Then no record is returned a second time
  And no record between the two instants is skipped
```

#### SCN-111-B03 — Every record is visited exactly once across a full traversal
```gherkin
Given a record class whose total record count is known
When the class is traversed completely, one page at a time
Then the number of distinct records returned equals the known total
  And the number of records returned in total also equals the known total
```

#### SCN-111-B04 — A resume position is only meaningful for its own traversal
```gherkin
Given a resume position obtained while traversing one record class
When that position is presented while traversing a different record class
Then the traversal refuses the position
  And it does not silently start from the beginning
```

### Round-trip parity

#### SCN-111-C01 — A corpus survives export, import and comparison unchanged
```gherkin
Given a source corpus containing records of every declared class
When the corpus is exported and then imported into an empty destination
Then the destination's record count for every declared class equals the source's
  And the canonical content hash for every declared class equals the source's
```

#### SCN-111-C02 — A capture that failed processing survives the round trip (boundary)
```gherkin
Given a corpus containing a capture that failed processing
  And a capture that is still pending processing
When the corpus is exported and imported into an empty destination
Then both the failed capture and the pending capture are present at the destination
  And each retains the processing state it had at the source
```

#### SCN-111-C03 — A full delete leaves every manifest-owned class empty
```gherkin
Given a corpus whose contents have been exported and verified at a destination
When the owner deletes the whole corpus
Then every record class the manifest declares contains no records
  And the system reports that emptiness as a verified result rather than as an assumption
```

#### SCN-111-C04 — A scoped delete removes its scope and nothing else
```gherkin
Given a corpus containing records that belong to a chosen source and records that do not
When the owner deletes only the chosen source's scope
Then no record belonging to that scope remains in any declared class
  And every record outside that scope is still present and unchanged
```

#### SCN-111-C05 — Deletion is never reached without deliberate confirmation
```gherkin
Given an owner initiating a corpus deletion
When the deletion is requested without the deliberate confirmation the system requires
Then no record is deleted
  And the system states the scope that would have been erased
```

#### SCN-111-C06 — Sensitivity survives the round trip
```gherkin
Given artifacts carrying different recorded sensitivities and their provenance
When the corpus is exported and imported into an empty destination
Then every artifact at the destination carries the same sensitivity as at the source
  And it carries the same provenance for that decision
```

### Sensitivity as a stored property

#### SCN-111-D01 — Sensitivity is recorded with the artifact and readable
```gherkin
Given an artifact that has been classified
When the artifact's sensitivity is read
Then a value from the fixed vocabulary is returned
  And the provenance of the classification decision is returned with it
```

#### SCN-111-D02 — An unclassified artifact is unset, not permissive
```gherkin
Given an artifact for which no classification has ever been made
When the artifact's sensitivity is read
Then the result is the distinct unset state
  And the unset state is not interpreted as the least sensitive value
```

#### SCN-111-D03 — A value outside the vocabulary is refused at write time
```gherkin
Given a classification value that is not part of the fixed vocabulary
When it is recorded against an artifact
Then the write is refused
  And the artifact's existing sensitivity is unchanged
```

#### SCN-111-D04 — Re-classification replaces provenance, never blanks it
```gherkin
Given an artifact with a recorded sensitivity and provenance
When the artifact is re-classified
Then the new sensitivity is recorded with the provenance of the new decision
  And the artifact is never left classified without provenance
```

### One fail-closed egress decision

#### SCN-111-E01 — Absent principal refuses before any external call
```gherkin
Given a request that would send artifact content to an external model
  And no authenticated principal on the request
When the egress decision is evaluated
Then the request is refused
  And no external request is made
```

#### SCN-111-E02 — A grant that does not cover the request refuses it
```gherkin
Given an authenticated principal holding a grant that does not cover the requested content
When the egress decision is evaluated
Then the request is refused
  And no external request is made
```

#### SCN-111-E03 — A credential for a different audience refuses the request
```gherkin
Given an authenticated principal whose credential names an audience other than the one being addressed
When the egress decision is evaluated
Then the request is refused
  And no external request is made
```

#### SCN-111-E04 — Disallowed sensitivity refuses the request
```gherkin
Given an authenticated and granted principal with a correct credential audience
  And artifact content whose recorded sensitivity the policy disallows for external transmission
When the egress decision is evaluated
Then the request is refused
  And no external request is made
```

#### SCN-111-E05 — Unset sensitivity refuses the request
```gherkin
Given an authenticated and granted principal with a correct credential audience
  And artifact content whose sensitivity has never been recorded
When the egress decision is evaluated
Then the request is refused
  And no external request is made
```

#### SCN-111-E06 — One decision point governs every external path
```gherkin
Given more than one path through which artifact content could reach an external model
When each path is exercised
Then each one is governed by the same egress decision
  And no path can transmit content without that decision having been taken
```

#### SCN-111-E07 — Every decision is recorded, whether it permits or refuses
```gherkin
Given an egress decision has been evaluated
When the record of decisions is examined
Then the decision appears with the principal, the outcome, and the reason
  And a permitting decision is recorded with the same fidelity as a refusal
```

---

## 8. Requirements

Requirements are behavioural and implementation-free. Each is testable against at least one
scenario above.

### Manifest and symmetry

| ID | Requirement | Scenarios |
|---|---|---|
| **R-111-01** | The corpus MUST be defined by a single versioned manifest that enumerates its record classes. | SCN-111-A01 |
| **R-111-02** | Export, import and delete MUST derive their scope from that one manifest. No operation may carry its own class list. | SCN-111-A01 |
| **R-111-03** | Each declared record class MUST specify an identity key and a deterministic ordering key. | SCN-111-B03, SCN-111-B04 |
| **R-111-04** | A record class present in the store and absent from the manifest MUST be detectable and reported as a coverage defect. | SCN-111-A02 |
| **R-111-05** | Introducing a new content-bearing record class MUST include adding it to the manifest in the same change. | SCN-111-A02 |
| **R-111-06** | A bundle MUST state the manifest version under which it was written, and MUST carry a per-class census including classes with zero records. | SCN-111-A03 |
| **R-111-07** | An import MUST refuse a bundle whose manifest version it does not understand, and MUST apply nothing in that case. | UC-111-002 alt |

### Completeness

| ID | Requirement | Scenarios |
|---|---|---|
| **R-111-08** | Corpus membership MUST NOT depend on processing state. Pending and failed captures are part of the corpus. | SCN-111-C02 |
| **R-111-09** | An operation that cannot cover a manifest-declared class MUST NOT report itself as complete, and MUST identify the uncovered class. | SCN-111-A04 |

### Traversal

| ID | Requirement | Scenarios |
|---|---|---|
| **R-111-10** | A complete paged traversal of a record class MUST return every record exactly once, including records that share an identical value of the ordering key. | SCN-111-B01, SCN-111-B03 |
| **R-111-11** | A resume position MUST preserve the full precision the store keeps, so that resuming neither repeats nor skips records. | SCN-111-B02 |
| **R-111-12** | A resume position MUST be self-describing enough that presenting it to a traversal it does not belong to is refused rather than silently restarted. | SCN-111-B04 |

### Round trip and deletion

| ID | Requirement | Scenarios |
|---|---|---|
| **R-111-13** | Export followed by import into an empty destination MUST produce equal per-class record counts and equal per-class canonical content hashes. | SCN-111-C01 |
| **R-111-14** | A full corpus delete MUST leave every manifest-declared class empty, and that emptiness MUST be verified after the operation and reported as a result. | SCN-111-C03 |
| **R-111-15** | Deletion MUST be available at artifact, source, topic, and whole-corpus scope, and a scoped delete MUST remove its scope and nothing outside it. | SCN-111-C04 |
| **R-111-16** | Deletion MUST require deliberate confirmation, and MUST state the scope it would erase before erasing it. | SCN-111-C05 |
| **R-111-17** | Deletion MUST NOT require permission from any party other than the corpus owner. | Principle 11 |

### Sensitivity

| ID | Requirement | Scenarios |
|---|---|---|
| **R-111-18** | Artifact sensitivity MUST be persisted as a property of the artifact, drawn from a fixed vocabulary. | SCN-111-D01, SCN-111-D03 |
| **R-111-19** | A recorded sensitivity MUST carry the provenance of the decision that produced it. | SCN-111-D01, SCN-111-D04 |
| **R-111-20** | Absence of classification MUST be a distinct *unset* state and MUST NOT be treated as the least sensitive value. | SCN-111-D02 |
| **R-111-21** | A value outside the vocabulary MUST be refused at write time, leaving the prior value unchanged. | SCN-111-D03 |
| **R-111-22** | Re-classification MUST replace provenance, never leave a classified artifact without it. | SCN-111-D04 |
| **R-111-23** | Sensitivity and its provenance MUST travel with the artifact through export and import. | SCN-111-C06 |

### Egress

| ID | Requirement | Scenarios |
|---|---|---|
| **R-111-24** | A single egress decision MUST be evaluated before any external transmission of artifact content, and MUST consume the authenticated principal, the grant, the credential audience, and the sensitivity of every in-scope artifact. | SCN-111-E01 … E05 |
| **R-111-25** | The egress decision MUST fail closed: any missing or policy-disallowed input produces refusal. | SCN-111-E01 … E05 |
| **R-111-26** | Refusal MUST occur before the external request is made, not after. | SCN-111-E01 … E05 |
| **R-111-27** | Every path capable of transmitting artifact content externally MUST be governed by that same decision. | SCN-111-E06 |
| **R-111-28** | Every egress decision MUST be recorded with principal, outcome and reason, whether it permits or refuses. | SCN-111-E07 |
| **R-111-29** | Remote egress MUST remain an explicit per-client operator grant. It MUST NOT be enabled globally, by default, or by a build-time switch. | Principle 11 |

---

## 9. Non-Functional Requirements

| ID | Requirement |
|---|---|
| **NFR-111-01** | Export and delete MUST be resumable after interruption without losing or repeating records, so that a corpus larger than a single session can still be moved. |
| **NFR-111-02** | Round-trip parity MUST be verifiable by the owner from reported output alone, without inspecting internal storage. |
| **NFR-111-03** | The egress decision MUST NOT be bypassable by configuration, environment, or build flag. |
| **NFR-111-04** | Bundles MUST be self-describing: a reader with only the bundle can determine its manifest version and per-class census. |
| **NFR-111-05** | Sensitivity classification MUST be readable for an artifact without scanning unrelated records. |
| **NFR-111-06** | Copy in any surface describing this capability MUST obey Principle 11's honesty constraint: the only permitted claim shape is *"Smackerel never sends your knowledge anywhere; a client you explicitly authorize may."* No surface may claim the product enforces, verifies, guarantees or attests client-side inference locality. |

---

## 10. Non-Goals

1. **A second export path.** The existing surface is corrected, not supplemented. Two paths would diverge again.
2. **Encrypting bundles at rest.** Bundle confidentiality is a separate concern with its own key-management question. This spec defines completeness and symmetry, not cryptography.
3. **Cross-version bundle migration.** An importer refuses a manifest version it does not understand. Translating between versions is future work.
4. **Deciding the classification of any specific artifact.** This spec requires that sensitivity be stored, vocabulary-constrained, provenance-bearing and consulted. Which artifacts are sensitive is a policy question for the operator.
5. **Replacing the existing grant model.** The egress decision *consumes* grants; it does not redefine them. Spec 108 owns grant enforcement.
6. **Selective import (merge into a non-empty destination).** Parity is defined against an empty destination. Merge semantics — conflict resolution, identity collision — are a distinct problem and are deliberately excluded.

---

## 11. Open Findings (routed, not resolved here)

| ID | Severity | Finding | Owner |
|---|---|---|---|
| **F-111-TEMPLATE-01** | MEDIUM | The brief required conformance to `.specify/templates/spec-template.md`; that file does not exist in this repository. This spec follows the adjacent spec's structure instead. Either the template should be installed or the instruction corrected — silently ignoring it would let the next author believe a template was consulted. | operator via `bubbles.plan` |
| **F-111-FLAG-01** | BLOCKING | This capability plainly warrants a feature flag, but declaring one requires editing the non-owning train's flag bundle so the flag is default-OFF there (G111). Those bundles are owned by `bubbles.train` and were not editable in this run. `flagsIntroduced` is therefore **empty**, which is honest rather than complete: the flag must be declared before delivery begins. | `bubbles.train` |
| **F-111-MANIFEST-01** | BLOCKING | The manifest's initial class list is not fixed by this spec. It must be derived from the store rather than hand-listed, or it will be incomplete on the day it is written — which is the original defect in a new location. | `bubbles.design` |
| **F-111-110-01** | HIGH | Spec 110 introduces passages, a second copy of user content, and routes their portability here via `F-110-EGRESS-01`. If 110 lands first, the manifest must include passages from its first version. The two specs' ordering is a real dependency, not a coincidence. | `bubbles.design` (with spec 110) |
| **F-111-108-01** | HIGH | The egress decision consumes grants that spec 108 defines and enforces. Spec 108 targets `next` and is `specs_hardened`, not delivered. This spec's egress requirements cannot be satisfied before 108's grant model exists. | `bubbles.plan` |
| **F-111-BACKFILL-01** | HIGH | Persisting sensitivity leaves every existing artifact *unset*, and R-111-25 makes unset a refusal. Turning the gate on without a backfill or a declared migration posture would refuse legitimate existing traffic. The migration posture must be an explicit decision. | `bubbles.design` |
| **F-111-DELETE-01** | MEDIUM | Deletion interacts with derived records that reference artifacts (relationships, summaries, collections). Whether deleting an artifact deletes, orphans, or rewrites its derivatives is undecided and affects both delete and parity. | `bubbles.design` |
| **F-111-CENSUS-01** | MEDIUM | "Canonical content hash per class" requires a canonical serialisation order and field set per class. Undefined, parity comparison is not reproducible across instances. | `bubbles.design` |

---

## Product Principle Alignment

Bound by [`.github/instructions/product-principles.instructions.md`](../../.github/instructions/product-principles.instructions.md)
(Principles 1–10 ratified 2026-06-03; **Principle 11 ratified 2026-07-29**; BLOCKING).

| Principle | Alignment | Evidence |
|---|---|---|
| **11 — Local-First Data Ownership** | The governing principle. Its unconditional-exit clause — *"the user can export, relocate, or delete the entire corpus without asking permission"* — is currently unmet on all three verbs (E1, E2, E3, E4, E6). Its per-client audited-egress clause has no enforcement point at all (E8, E10). This spec makes exit complete and symmetric, and creates the single fail-closed egress decision the principle presumes. NFR-111-06 binds all copy to the principle's honesty constraint. | R-111-01…29; NFR-111-06; SCN-111-C01, C03, E01…E07 |
| **4 — Source-Qualified Processing** | Source metadata is a manifest-owned property that must survive export and import. A bundle that moved records but dropped their source qualifiers would strip exactly what this principle forbids stripping "for simplicity". | R-111-13, R-111-23; SCN-111-C01 |
| **5 — One Graph, Many Views** | Portability is defined over the existing graph's record classes. No parallel store is created, and the manifest is a declaration about the one graph rather than a second model of it. | Non-Goal 1; §4.2 |
| **3 — Knowledge Breathes (Lifecycle, Not Static)** | Every primitive in §4.1 declares its lifecycle, including the manifest and the sensitivity classification. Deletion is the terminal lifecycle state the corpus previously lacked entirely. | §4.1; R-111-14, R-111-15 |
| **8 — Trust Through Transparency** | An export states what it contains and refuses to claim completeness it cannot deliver; a delete proves emptiness rather than asserting success; every egress decision is recorded including the permitting ones. | R-111-06, R-111-09, R-111-14, R-111-28; SCN-111-A03, A04, E07 |
| **9 — Design For Restart, Not Perfection** | Export and delete are resumable after interruption, so an owner who is interrupted mid-migration resumes rather than restarts. | NFR-111-01; SCN-111-B02 |
| **6 — Invisible By Default, Felt Not Heard** | Deliberate confirmation before deletion is the one place this capability is *deliberately* loud. Irreversible corpus erasure is exactly the case where an interruption is warranted. | R-111-16; SCN-111-C05 |

**Deviations:** none.

**Tension recorded, not resolved.** Choosing the `next` train (below) means the `mvp` train
continues to under-deliver Principle 11's unconditional-exit guarantee until promotion. That
is a real, ongoing gap on the live train, and it is stated here rather than obscured by the
train choice.

---

## Release Train

Targets the **`next`** train (`config/release-trains.yaml` — `id: next`, `phase: active`,
`target_slot: staging`, `flags_bundle: config/feature-flags.next.yaml`).

**Why `next` and not `mvp`.** Four reasons, in order of weight.

1. **It introduces a fail-closed enforcement point.** R-111-24 through R-111-27 insert a
   refusal into a path that today has none. This is the same change class that
   [`specs/108-corpus-grant-enforcement`](../108-corpus-grant-enforcement/spec.md) declared
   `next` for, in its own words: *"security-posture change; MUST NOT ship on `mvp`."*
   Applying a different rule to the same class of change here would be inconsistent.
2. **It composes with a grant model that is itself on `next`.** The egress decision consumes
   spec 108's grants (`F-111-108-01`). Shipping the gate on `mvp` while the grant model it
   validates against lives on `next` would split one security posture across two trains, and
   the gate would check a grant vocabulary the `mvp` train does not enforce.
3. **It introduces an irreversible destructive capability.** Whole-corpus deletion has no
   undo. Proving export → import → delete parity on staging before exposing corpus erasure
   on the live self-hosted host is the correct order for a capability whose failure mode is
   permanent data loss.
4. **Enforcement lands before the data it enforces on exists.** Persisting sensitivity leaves
   every existing artifact unset, and unset is a refusal (R-111-20, R-111-25). Until the
   backfill posture in `F-111-BACKFILL-01` is decided, turning this on against a live corpus
   would refuse legitimate traffic.

**Behaviour on `mvp`.** Unchanged. The `mvp` train continues to serve the existing export
surface with its current partial output, exposes no delete surface, persists no artifact
sensitivity, and inserts no egress decision. Nothing in this spec alters `mvp` behaviour
until `bubbles.train` promotes. The honest consequence is stated in the tension note above:
`mvp` remains non-conformant with Principle 11's exit guarantee for the duration.

**Flags.** `flagsIntroduced: []`.

This is a **deliberate empty**, not an oversight. A capability of this size warrants a flag,
and G111 requires any introduced flag to be default-OFF in every non-owning train's bundle —
which means editing `config/feature-flags.mvp.yaml`, a `bubbles.train`-owned artifact this
authoring run is not permitted to modify. Naming a flag here without that bundle entry would
be a declaration with no backing configuration: it would read as done and be unenforced.
The flag declaration is therefore routed to `bubbles.train` as `F-111-FLAG-01`, marked
BLOCKING, and must be resolved before delivery scopes begin.

---

## Exposure Contract

| Capability | Surface class | Surface id | Status | Plan |
|---|---|---|---|---|
| Corpus export (bundle) | httpRoute | corpus export endpoint | planned | This spec, delivery packet under a delivery-capable mode |
| Corpus import (bundle) | httpRoute | corpus import endpoint | planned | This spec, delivery packet |
| Corpus delete (artifact / source / topic / whole) | httpRoute | corpus delete endpoints | planned | This spec, delivery packet |
| Artifact sensitivity read/write | httpRoute | artifact sensitivity surface | planned | This spec, delivery packet |
| Egress decision | internal | evaluated in-process by every path that can transmit artifact content externally | planned | This spec, delivery packet; deliberately internal — it is a chokepoint, not a callable service |
| Egress decision record | httpRoute | egress decision audit read surface | planned | This spec, delivery packet |

No row is `delivered`. Nothing in this spec has been built.
