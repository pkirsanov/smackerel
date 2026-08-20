# Design: 111 Corpus Portability & Artifact Sensitivity

**Status:** `in_progress` · **Workflow mode:** `product-to-planning` · **Release train:** `next`
**Owner of this artifact:** `bubbles.design`
**Design pass run:** 2026-08-20 by `bubbles.design`, against the working tree

**Supersession.** Every earlier revision of this file said, in its own words, that *"no
design decision has been made for this feature"* and carried D1–D8 as open questions with
no preferred answer. That statement was true when `bubbles.analyst` wrote it and is false
now. It is removed rather than archived, because leaving it would leave two contradictory
claims about whether this feature has an architecture. D1–D8 are answered below; the three
that were behind BLOCKING findings are answered first and their findings' design halves are
closed in [Findings Status](#findings-status-after-this-pass). The decisions that genuinely
cannot be made without operator input are named as such, with the exact question, in
[Open Decisions Requiring Operator Input](#open-decisions-requiring-operator-input) — an
honest open decision is recorded there rather than a manufactured one recorded above.

**No source file was changed and no test was run by this pass.** Every code reference below
is a read of the working tree at design time, cited by path and line. Nothing here is
execution evidence, and no scope, DoD item or status was advanced.

---

## Design Brief

A reviewer who reads only this section should be able to stop the design if it is pointed
the wrong way.

### Current State

The corpus has one export surface — `internal/db/postgres.go:110` `ExportArtifacts` — that
reads a single table (`artifacts`), filters it to `processing_status = 'processed'`,
paginates on `created_at` alone with a strict `>` and no tiebreak, and hands the caller a
cursor serialised at second precision (`internal/api/capture.go:388`,
`time.Format(time.RFC3339)`) even though the column is `TIMESTAMPTZ`, which Postgres keeps
to the microsecond. There is no import, and no delete at any granularity. Artifact
sensitivity is not a column; it is a key inside the free-form `artifacts.metadata` JSONB
document, read by `internal/knowledge/sensitivity_query.go:26-30`, which skips rows whose
value is missing or out of vocabulary. Nothing in the repository refuses an outbound model
call because of what an artifact contains.

### Target State

One versioned **corpus manifest** partitions every table in the store into exactly one of
`corpus`, `derived`, or `operational`, and export, import and delete all resolve their
scope from it. Traversal becomes a total order over `(orderingKey, identityKey)` with an
opaque, class-tagged, microsecond-precision cursor. Sensitivity becomes a CHECK-constrained
column on `artifacts` with all-or-nothing provenance columns, where *unset* is SQL `NULL`.
A single egress decision is required at the credential seam —
`internal/assistant/openknowledge/llm/dispatch_resolver.go:218`, the only non-test writer of
`ChatRequest.APIKey` — so no caller can obtain a credentialed remote request without one.

### Patterns To Follow

| Pattern | Where it already lives | What this design takes from it |
|---|---|---|
| Staged rollout of a new refusal point: OBSERVE (measure, never deny) then ENFORCE | `internal/api/corpus_grant_gate.go` (spec 108 SCOPE-02) | Both new refusal points here — sensitivity-unset and egress — ship observable before they deny (D6, D8) |
| Sensitivity persisted as a typed column with a CHECK constraint | `internal/db/migrations/021_drive_schema.sql:66`; `025_photo_libraries.sql:13` | `artifacts.sensitivity_tier` is a column with a CHECK, not a JSONB key (D5) |
| One exported ordering function for a tier vocabulary, invalid input treated as most restrictive | `internal/connector/qfdecisions/personal_context_consent.go:348` `PersonalContextTierLessOrEqual`, `:356` `PersonalContextTierMinimum` | The egress ceiling comparison reuses this function rather than defining a second ordering (D5, D7) |
| Typed, secret-free reject vocabulary; never silently substitute a different target | `internal/assistant/openknowledge/llm/dispatch_resolver.go:38-60` | Egress refusals carry a typed reason and never a credential, and never downgrade to a permitted path (D7) |

### Patterns To Avoid

| Pattern | Where it lives | Why not to follow it |
|---|---|---|
| Sensitivity as a JSONB convention whose unreadable values are silently skipped | `internal/knowledge/sensitivity_query.go:26-30` | Skipping is safe for a *reader* and useless for an *enforcement point*: a policy that cannot be read cannot refuse (E8) |
| "The export" as one table filtered by processing state | `internal/db/postgres.go:122` | It is the defect this feature exists to remove, not a template to extend (E1, E2) |
| A domain timestamp serialised as RFC3339 text used as a pagination cursor | `internal/api/capture.go:388` and `:356` | The format has no sub-second field, so the round trip moves the cursor backwards and re-returns records (E4, E5) |
| Putting the corpus manifest in `internal/manifest/` | `internal/manifest/scenario_manifest.go` | That package is spec 076's *scenario* manifest. Two unrelated things called "the manifest" in one repository is how a future reader ends up editing the wrong one |

### Resolved Decisions

- **D1** The manifest is an exhaustive, machine-checked **partition** of the store's tables, not a hand-written include-list and not a blanket auto-include. The store enumerates; the manifest classifies; an unclassified table is a hard failure.
- **D2** Canonical per-class hash = SHA-256 over identity-key-sorted `(identityKey ‖ recordHash)` pairs plus the record count, where a record hash is SHA-256 over a fixed, manifest-declared portable field set rendered as sorted-key JSON with timestamps as integer microseconds.
- **D3** Total order `(created_at, id)`; SQL row-value predicate `(created_at, id) > ($1, $2)`; cursor is base64url JSON carrying manifest version, class id, and the ordering tuple with the timestamp as integer microseconds. A cursor from another class or another manifest version is refused.
- **D4** Per-class `deleteRule` of `cascade` / `unlink` / `independent`, declared in the manifest and consumed by one delete engine.
- **D5** Reuse the artifact tier ladder already in service — `low < medium < high` — as a CHECK-constrained nullable column; `unset` is SQL `NULL`, never a vocabulary member.
- **D6** Backfill only where a real prior decision exists (migrate valid `metadata->>'sensitivity_tier'` values); never invent a classification; stage the refusal OBSERVE then ENFORCE.
- **D7** The chokepoint is the credential seam in the dispatch resolver, not HTTP middleware, and the remote branch takes an egress decision as a required argument so the gate cannot be routed around.
- **D8** SCOPE-01…07 do not depend on specs 108 or 110 at all; only SCOPE-08's ENFORCE flip does, and it must not precede spec 108's ENFORCE.

### Open Questions

- **When** to flip sensitivity enforcement from OBSERVE to ENFORCE, and what to do with artifacts still unset at that moment. Operator-owned; the mechanism, the measurement and the default are decided below, the timing is not.
- **Which tiers may leave, per grant.** The design ships the deny-all default and the ceiling mechanism; the ceiling *values* are the operator's policy (spec.md Non-Goal 4).
- Whether the litellm sidecar is reachable by anything other than the Go core. If it is, the credential seam is not the last gate. Stated as an assumption for SCOPE-08 to verify, not as a fact.

---

## Inputs Consumed By This Design Pass

| Input | Location | Why it is load-bearing |
|---|---|---|
| Requirements R-111-01 … R-111-29 | [`spec.md`](spec.md) §8 | The behavioural contract; design chooses how, never whether |
| Domain capability model | [`spec.md`](spec.md) §4 | Primitives, relationships and the eight policies every implementation must obey |
| Verified evidence base E1 … E12 | [`spec.md`](spec.md) §3 | Current-state facts, each re-verified 2026-08-04 against the working tree |
| Open findings F-111-* | [`spec.md`](spec.md) §11 | Three were BLOCKING; their design halves are resolved in D1, D6 and D8 |
| Principle 11 | [`docs/Product-Principles.md`](../../docs/Product-Principles.md) §Principle 11 | Ratified 2026-07-29; BLOCKING; governs exit and egress |
| Spec 110 passage record class | [`specs/110-retrieval-quality-foundation/spec.md`](../110-retrieval-quality-foundation/spec.md) `F-110-EGRESS-01` | Read at design time; spec 110 is `not_started`, and D1 removes the ordering dependency |
| Spec 108 grant model | [`specs/108-corpus-grant-enforcement/spec.md`](../108-corpus-grant-enforcement/spec.md); `internal/api/corpus_grant_gate.go` | The egress decision consumes its grants; the gate exists in OBSERVE and does not yet deny |
| The store itself | `internal/db/migrations/*.sql`, `internal/db/postgres.go`, `internal/api/capture.go`, `internal/knowledge/sensitivity_query.go`, `internal/assistant/openknowledge/llm/dispatch_resolver.go` | Read directly at design time; D1, D3, D5 and D7 are grounded in specific lines rather than in the spec's summary of them |

---

## Decision Record

Eight decisions were open. Eight are answered. Each entry states the decision, what else was
considered, why the alternatives lose, and what the choice costs. The three behind BLOCKING
findings — D1, D6, D8 — are answered first because the rest depend on them.

### D1 — How the manifest's record-class list is derived · `F-111-MANIFEST-01` (BLOCKING)

**Decision.** The manifest is an **exhaustive partition of the store's tables**, checked
against the live store at startup and in test. Every table is declared as exactly one of
three dispositions, and a table that appears in the store under no disposition is a hard
failure that names the table.

| Disposition | Meaning | Export | Import | Whole-corpus delete |
|---|---|---|---|---|
| `corpus` | User-owned knowledge. The corpus proper. | yes | yes | yes |
| `derived` | Reconstructible from `corpus` rows by re-running a declared process (embeddings, passages, search indexes). | no | no | yes |
| `operational` | Instance-local state that is not the user's knowledge and must not travel (credentials, tokens, traces, progress markers, dedupe ledgers). | no | no | no |

**Alternatives considered.**

1. *Hand-write the `corpus` list.* Rejected: this is exactly what `F-111-MANIFEST-01`
   forbids. The list is incomplete on the day it is written, and its incompleteness is
   silent — which is the original defect relocated, not removed.
2. *Auto-include every table.* Rejected, and it is worse than the hand-list. The store holds
   136 distinct tables (counted across `internal/db/migrations/*.sql`), among them
   `model_provider_connections` (encrypted provider credentials),
   `web_user_credentials`, `auth_token_granted_scopes`, and the reveal-token secret hashes
   from `032_photo_reveal_tokens_secret_hash_and_toctou.sql`. An export that swept those into
   a portable bundle would be a credential-exfiltration surface wearing a portability label.
3. *Derive by naming convention or by a marker column.* Rejected: a convention is a hand-list
   with extra steps, and it fails open — a table that forgets the marker is silently
   excluded, which is the failure mode P3 forbids.

**Reasoning.** The finding asks for derivation because a human list goes stale. The thing
that actually goes stale is the *completeness* of the list, not its content. So derive the
**enumeration** from the store, where it is always current, and keep the **classification**
declarative, where a human can be held to it — then make the join between them mandatory. A
new table cannot be silently omitted, because it has no disposition and the check fails
naming it. This is also the mechanism R-111-04 and SCN-111-A02 already ask for: "a record
class present in the store and absent from the manifest MUST be detectable and reported as
a coverage defect" is precisely this check, so one construct satisfies the derivation
finding and the coverage requirement together.

**Consequence.** Adding any table becomes a two-file change — the migration and the manifest
— and the build says so. That is the cost, and it is the point (P8, R-111-05). The three-way
split also resolves a question the spec left implicit: `derived` rows are erased by a delete
but never carried in a bundle, because carrying a recomputable copy would make the bundle's
parity depend on the version of the process that produced it.

### D6 — Migration posture for artifacts left unset · `F-111-BACKFILL-01` (BLOCKING)

**Decision.** Three parts, all required together.

1. **Backfill only real decisions.** Migrate `artifacts.metadata->>'sensitivity_tier'` into
   the new column where the value is in vocabulary. Rows with no value, or an
   out-of-vocabulary value, are left `NULL` — *unset*.
2. **Report, never drop.** Out-of-vocabulary values are counted and their artifact ids
   recorded in the migration's report. Today `sensitivity_query.go:26-30` skips them
   silently; a migration that inherited that silence would erase evidence that a
   classification was attempted.
3. **Stage the refusal.** The unset-refuses rule ships OBSERVE first — it evaluates, counts,
   and logs what it *would* refuse without refusing — and flips to ENFORCE as a separate,
   per-instance act.

**Alternatives considered.**

1. *Backfill every unset artifact to a default tier.* Rejected on two grounds. It fabricates
   a classification no one made, and it cannot supply honest provenance for it — the
   provenance columns would have to say a decision happened that did not. R-111-19 and
   R-111-22 exist to prevent exactly this shape.
2. *Ship ENFORCE immediately and accept the refusals.* Rejected: it converts an unknown into
   an outage. Nobody knows today how many artifacts carry no classification, because the
   current reader skips them without counting.
3. *Treat unset as permissive until backfilled.* Rejected outright — R-111-20 and R-111-25,
   and it is the failure the spec names as disqualifying.

**Reasoning.** The finding's harm is "turning the gate on would refuse legitimate traffic",
and the missing input is a *number*. OBSERVE produces that number using the real gate on the
real path, so the operator decides with a measurement instead of an estimate. This is not an
invention: `internal/api/corpus_grant_gate.go` already stages exactly this way for spec 108's
grants, and reusing the repository's own staging shape means one mental model for both
refusal points.

**Consequence.** There is a window where sensitivity is persisted and consulted but does not
deny. That window is honest and must be described as such — while it is open, the product
does not yet enforce the Principle 11 egress clause, and no copy may say it does
(NFR-111-06). The **timing** of the flip is the operator's and is recorded as an open
decision below; the mechanism, the measurement and the default (OFF) are decided here.

### D8 — Ordering against specs 110 and 108 · `F-111-110-01`, `F-111-108-01`

**Decision.** Split the two, because they are not the same kind of dependency.

- **Spec 110 (passages) — the ordering dependency dissolves.** Under D1, a passage table
  arriving later is a table with no disposition, so the coverage check fails and names it.
  110's delivery must therefore include its own manifest entry, which R-111-05 and P8 already
  require of any new content-bearing class. 111 does not wait for 110, and 110 cannot land
  silently. Verified: spec 110 is `not_started` (`specs/110-.../state.json`), so it will not
  land first regardless.
- **Spec 108 (grants) — a real, narrow dependency on ENFORCE only.** SCOPE-08 is built
  against a **grant-decision port** (an interface it defines and 108 satisfies), so it can be
  written and unit-tested while 108's gate is non-denying. But SCOPE-08's own ENFORCE flip
  **must not precede** spec 108's. Verified: `internal/api/corpus_grant_gate.go:34`
  `corpusGrantModeObserve` with, in its own comment, "no denial branch"; spec 108's state is
  `blocked`.

**Alternatives considered.** *Have 111 define its own grant model so it does not wait.*
Rejected — spec.md Non-Goal 5 assigns grants to 108, and a second grant vocabulary is the
divergence this whole feature exists to remove.

**Reasoning.** A gate that consumes a grant which cannot deny, and refuses only on
sensitivity, is *partly* decorative: `SCN-111-E02` ("a grant that does not cover the request
refuses it") would pass its unit test against the port and fail in production, because the
production grant never says no. The spec's Failure Condition names decorative enforcement as
disqualifying, so the flip order is a technical consequence, not a scheduling preference.

**Consequence.** SCOPE-01 through SCOPE-07 are unblocked by this design and depend on neither
sibling spec. SCOPE-08 is buildable now and completable only after spec 108's SCOPE-04. The
`F-111-108-01` finding stands, with its scope narrowed from "blocks the feature" to "blocks
SCOPE-08's ENFORCE".

### D2 — What "canonical content hash per class" means · `F-111-CENSUS-01`

**Decision.** Defined in full under [Canonical Census And Parity](#canonical-census-and-parity).
In summary: a record's canonical form is the manifest-declared `portableFields` for its class,
rendered as JSON with keys sorted by UTF-8 byte order, no insignificant whitespace, `null`
emitted explicitly (never omitted), and timestamps as integer microseconds since the Unix
epoch in UTC. The class hash is order-independent: sort by identity key, hash the
concatenation of `identityKey ‖ recordHash` pairs, and include the record count in the
preimage.

**Alternatives considered.** *Hash the wire bytes of the bundle file.* Rejected: it makes
parity depend on serialisation incidentals and on traversal order, so two correct instances
disagree. *Hash the whole row.* Rejected: rows carry instance-local bookkeeping
(`last_accessed`, `access_count`, and the `vector(384)` embedding) that differs legitimately
between instances and would report a false mismatch.

**Reasoning.** NFR-111-02 requires the owner to verify parity "from reported output alone".
That is only true if the same corpus produces the same hash on two machines that never
communicated, which forces every degree of freedom out of the encoding: field set, key
order, number and timestamp form, null handling, and record order.

**Consequence.** `portableFields` becomes a reviewed, per-class part of the manifest, and
changing it is a manifest version change because it changes every hash.

### D3 — The resume-position encoding

**Decision.** Defined in full under [Traversal Contract](#traversal-contract). In summary:
order by `(orderingKey, identityKey)`; page with the SQL row-value predicate
`(orderingKey, identityKey) > ($1, $2)`; encode the cursor as base64url of a compact JSON
object carrying the manifest version, the class id, and the ordering tuple with the timestamp
as an **integer microsecond count**. A cursor whose class or manifest version does not match
the traversal is refused.

**Alternatives considered.**

1. *Keep a timestamp cursor and widen the format to `RFC3339Nano`.* Rejected: it fixes the
   precision half (E4/E5) and leaves the tiebreak half (E3) — records sharing an instant are
   still skipped by `created_at > $1`.
2. *Offset pagination (`LIMIT … OFFSET …`).* Rejected: it is not stable under concurrent
   insert or delete, and NFR-111-01 requires resuming across an interruption during which
   the corpus may have changed.
3. *An opaque server-side cursor stored in a table.* Rejected as disproportionate: it adds a
   record class, a lifetime, and a cleanup job to solve a problem that a self-describing
   value solves, and the cursor is not an authorization token (see below).

**Reasoning.** The two live defects pull in opposite directions — the missing tiebreak skips,
the truncated precision duplicates — so any fix that addresses one alone leaves a corpus
that is still wrong and now wrong in one direction only, which is harder to notice. A total
order over `(orderingKey, identityKey)` plus an exact-precision cursor closes both, and
P4's "exactly once" becomes a property of the order rather than a hope about the data.

**Consequence.** The existing `X-Next-Cursor` RFC3339 header form is **removed, not
extended**, and an old-format cursor is refused rather than coerced — coercing it would
silently skip, which is the defect. A composite index `(created_at, id)` is required on
`artifacts`; today only `idx_artifacts_created ON artifacts(created_at)` exists
(`001_initial_schema.sql`).

### D4 — Delete, orphan, or rewrite for derived records · `F-111-DELETE-01`

**Decision.** The manifest declares a `deleteRule` per class, from a closed set of three,
and one delete engine reads it.

| `deleteRule` | Behaviour when a referenced record is deleted | Applies to |
|---|---|---|
| `cascade` | The dependent record is deleted with its referent. | `derived` classes; annotations; graph edges (an edge needs both endpoints) |
| `unlink` | The dependent **row** is deleted; the container it belonged to survives, emptier. | membership rows such as list items |
| `independent` | Unaffected. | user-authored containers such as lists and topics |

**Alternatives considered.**

1. *Orphan everything (null the reference).* Rejected: it is the failure mode the spec names
   in §1 — a record the user believes is gone, still present on disk, still readable by an
   egress path. An orphaned passage is a verbatim copy of deleted content.
2. *Cascade everything.* Rejected: deleting one artifact would delete a list the user built,
   because the list contained it. That destroys a user-authored object to remove a member.
3. *Ask the user per delete.* Rejected: it moves a design decision into a dialog and makes
   the delete's blast radius vary per invocation, so parity after delete stops being
   predictable.

**Reasoning.** The distinction that does the work is **authorship**. A record that is a
function of an artifact (a passage, an embedding, a synthesis, an annotation *about* it) has
no meaning once the artifact is gone. A record the user authored independently (a list, a
topic) has meaning without any particular member. `unlink` is the honest middle for the join
row that is neither.

**Consequence.** Parity in SCOPE-05 must be compared against the **post-delete** census, not
a pre-delete one, because a scoped delete legitimately changes the counts of `cascade` and
`unlink` classes. `SCN-111-C04`'s "every record outside that scope is still present and
unchanged" is read as: outside the cascade closure. That reading must be stated in the test,
or the test will assert something the design deliberately does not do.

### D5 — The sensitivity vocabulary and its relationship to the existing ones

**Decision.** Adopt the tier ladder **already in service for this record class** —
`low < medium < high` — persisted as a nullable, CHECK-constrained column
`artifacts.sensitivity_tier`, where **`unset` is SQL `NULL` and is not a vocabulary member**.

**Alternatives considered.**

1. *Reuse `drive_files.sensitivity`'s vocabulary* — `('none','financial','medical','identity')`
   (`021_drive_schema.sql:66`). Rejected: it is **categorical, not ordered**, so a policy
   ceiling ("nothing above X may leave") cannot be expressed in it at all. It is also a
   different record class's vocabulary; adopting it would break the existing artifact reader.
2. *Reuse the `photo_sensitivity` enum* — `('none','sensitive','hidden')`
   (`025_photo_libraries.sql:13`). Rejected for the same reason, plus it is scoped to photos.
3. *Define a fourth, artifact-specific vocabulary.* Rejected: E9 already identifies a third
   independent scheme as the inconsistency to avoid.
4. *Represent unset as a fourth vocabulary value such as `'unset'`.* Rejected — see below.

**Reasoning.** Two separate things are being reused, and it matters that they are separated.
The **pattern** taken from E9 is *typed column plus CHECK constraint*, which
`drive_files.sensitivity` and `photo_sensitivity` both exemplify. The **vocabulary** is taken
from what already classifies *artifacts*: `internal/knowledge/sensitivity_query.go:26-30`
reads `low`/`medium`/`high`, the ordering is already single-sourced in
`internal/connector/qfdecisions/personal_context_consent.go:348`
(`PersonalContextTierLessOrEqual`), and a CHECK-constrained persisted counterpart already
exists at `037_qf_personal_context_consent_tokens.sql:20`
(`max_sensitivity_tier IN ('low','medium','high')`). So this is not a new vocabulary; it is
the existing artifact vocabulary being given a column, a constraint, and provenance.

`NULL` rather than an `'unset'` value is load-bearing, not stylistic. Under SQL's
three-valued logic, `sensitivity_tier <= ceiling` evaluates to `NULL` — not true — for an
unclassified row, so an unset artifact fails a ceiling test *at the database* even if
application code forgets to special-case it. A vocabulary member named `'unset'` would sort
somewhere in the ladder and could be compared successfully, which is precisely the
"treated as least sensitive" outcome R-111-20 forbids. Choosing the representation whose
default behaviour under a coding mistake is refusal is choosing fail-closed structurally.

**Consequence.** `PersonalContextTierMinimum`
(`internal/connector/qfdecisions/personal_context_consent.go:356`) already treats invalid
input as most-restrictive, so the ceiling arithmetic composes without a second ordering. The
JSONB convention is retired by the D6 migration, and the reader in
`internal/knowledge/sensitivity_query.go` reads the column afterwards, leaving one source.

### D7 — Where the single egress chokepoint sits

**Decision.** At the **credential seam**: `DispatchResolver`, in
`internal/assistant/openknowledge/llm/dispatch_resolver.go:218`. Its remote branch takes an
`EgressDecision` as a **required argument** and refuses to populate a credential without one.
Full contract under [The Egress Decision](#the-egress-decision).

**Alternatives considered.**

1. *HTTP middleware, mirroring `internal/api/corpus_grant_gate.go`.* Rejected, and this is the
   most important rejection in the file. Middleware governs **inbound routes**. Several
   paths reach a model without traversing a route at all — the scheduler, digest generation,
   synthesis, the Telegram adapter. A route-level gate would be invisible to all of them, and
   `SCN-111-E06` ("one decision point governs every external path") would be false the moment
   a background job ran.
2. *A gate inside each caller.* Rejected: N gates is N chances to omit one, and R-111-27
   would be an intention rather than a property.
3. *A gate in the litellm sidecar.* Rejected: it is after the credential has already been
   handed over, and the refusal must happen **before** the external call (R-111-26).

**Reasoning.** The chokepoint should be the narrowest thing every remote path must pass
through, and that is not a route — it is the **credential**. Verified by search of the
working tree: `dispatch_resolver.go:218` (`req.APIKey = &key`) is the only non-test writer of
`ChatRequest.APIKey` in the model path, and `agent/dispatch.go:151` copies from
`resolved.Request.APIKey` rather than producing one. `ChatRequest.APIKey` is documented at
`llm/client.go:118-120` as "nil for ollama", so the **local** path resolves without a
credential and therefore without an egress decision — which is Principle 11's "local
inference is the default" expressed as a code path rather than a claim.

**Consequence.** Two things must hold for this to stay true, and both are checkable rather
than hoped for: a lint asserting the set of `\.APIKey\s*=` writers outside tests is exactly
the two known sites, and the resolver's remote branch signature requiring the decision value.
One thing is **not** established by this design and is recorded as an open question: whether
the litellm sidecar can be reached by anything other than the Go core. If it can, the seam is
not the last gate, and SCOPE-08 must prove the network boundary rather than assume it.

---

## Architecture

Three layers, one declaration.

```
                    ┌──────────────────────────────────────────┐
                    │  corpus manifest  (versioned, embedded)  │
                    │  disposition · identityKey · orderingKey │
                    │  portableFields · deleteRule             │
                    └────────────────────┬─────────────────────┘
        ┌────────────────────┬───────────┴──────────┬────────────────────┐
        │                    │                      │                    │
   ┌────▼────┐        ┌──────▼──────┐        ┌──────▼─────┐      ┌───────▼──────┐
   │ coverage│        │   export    │        │   import   │      │    delete    │
   │  check  │        │ (traversal) │        │ (refusal)  │      │ (deleteRule) │
   └─────────┘        └─────────────┘        └────────────┘      └──────────────┘
        │                    │                      │                    │
        │              ┌─────▼──────────────────────▼─────┐              │
        │              │  canonical census + class hash   │◄─────────────┘
        │              └──────────────────────────────────┘
        │
   fails naming any table the manifest does not classify

   ── separate axis, same manifest ────────────────────────────────────────────
   artifacts.sensitivity_tier ─► egress decision ─► credential seam (resolver)
                                                    remote branch requires it
```

New package `internal/corpus` owns the manifest, the traversal, the bundle codec, the census
and the delete engine. It is deliberately **not** `internal/manifest`, which is spec 076's
scenario manifest. The egress decision lives in `internal/corpus/egress` and is consumed by
`internal/assistant/openknowledge/llm`, so the dependency points from the model path into the
corpus policy and never the reverse.

---

## The Corpus Manifest

**Location.** `internal/corpus/manifest.json`, embedded with `go:embed` and parsed into a
typed value at startup. Data in a reviewable file, behaviour in Go.

**Version.** A single integer, `manifestVersion`, starting at `1`. It changes when the class
set changes, when any `portableFields` list changes, or when an `identityKey` or
`orderingKey` changes — because each of those changes what a bundle means.

**Schema.**

```jsonc
{
  "manifestVersion": 1,
  "classes": [
    {
      "id": "artifacts",                    // stable, transport-visible class id
      "table": "artifacts",                 // the store table it partitions
      "disposition": "corpus",              // corpus | derived | operational
      "identityKey": ["id"],                // what makes two records the same record
      "orderingKey": ["created_at"],        // deterministic traversal order; identityKey is
                                            // always appended to make the order total
      "portableFields": [                   // exact, ordered-irrelevant set; hashed and carried
        "id", "artifact_type", "title", "summary", "content_raw", "content_hash",
        "key_ideas", "entities", "action_items", "topics", "sentiment",
        "source_id", "source_ref", "source_url", "source_quality", "source_qualifiers",
        "capture_method", "location", "location_geo", "temporal_relevance",
        "participants", "message_count", "source_chat", "timeline",
        "processing_status", "synthesis_status", "domain_data", "domain_schema_version",
        "metadata", "sensitivity_tier", "sensitivity_source", "sensitivity_decided_at",
        "sensitivity_decided_by", "created_at", "updated_at"
      ],
      "excludedFields": {                   // every non-portable column, with a reason
        "embedding": "derived — recomputable from content_raw",
        "last_accessed": "instance-local bookkeeping",
        "access_count": "instance-local bookkeeping",
        "relevance_score": "derived — recomputed at destination",
        "processing_tier": "instance-local scheduling state"
      },
      "deleteRule": "independent",
      "references": []                      // classes this one depends on, for cascade closure
    }
  ]
}
```

The `artifacts` entry above is written out in full because it is the one class this design
verified column-by-column against `internal/db/migrations/001_initial_schema.sql:16-63`. The
**remaining entries are produced by running the coverage check against the store**, which is
SCOPE-01's work and not this file's — hand-writing 135 more entries here, unverified, would
be the hand-list D1 just rejected, dressed as a design. What design owes SCOPE-01 is the
rule, the schema, and the failure mode, and all three are above.

**Two invariants, both enforced in test.**

- **Exhaustive.** Every table returned by
  `SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = 'public'` has exactly one
  manifest entry. A table with none fails the check, naming it. A manifest entry naming a
  table that does not exist also fails, so a dropped table cannot leave a phantom class.
- **Total.** Every `portableFields` plus `excludedFields` union equals the table's actual
  column set from `information_schema.columns`. A column added by a migration and mentioned
  in neither list fails, naming it. This is the part that stops the manifest going stale
  *within* a class — the same silent-omission defect one level down.

**Membership rules for export.** A record is exported when its class's disposition is
`corpus`. That is the whole rule. There is no processing-state predicate, no `deleted_at`
filter, no quality threshold, and no per-operation subset — P2 and R-111-08 make membership
independent of how far a record got, so the failed capture and the pending capture are in
because their class is in.

---

## Traversal Contract

This replaces the tiebreak-free, second-precision traversal at
`internal/db/postgres.go:122-123` and `internal/api/capture.go:356,388`.

**Order.** Total, per class: `ORDER BY <orderingKey…>, <identityKey…> ASC`. For `artifacts`
that is `ORDER BY created_at, id`. The identity key is appended automatically by the
traversal, not written per class in the manifest, so a class cannot accidentally declare a
non-total order.

**Page predicate.** SQL row-value comparison, not a scalar comparison:

```sql
SELECT <portableFields…>
FROM   artifacts
WHERE  (created_at, id) > ($1, $2)      -- omitted entirely on the first page
ORDER  BY created_at, id
LIMIT  $3
```

Row-value comparison is what makes "exactly once" a property of the order rather than of the
data: it advances past the exact `(created_at, id)` pair last seen, so records sharing an
instant differ in `id` and are each visited once. Requires a composite index
`CREATE INDEX idx_artifacts_created_id ON artifacts (created_at, id)`; the existing
`idx_artifacts_created` on `created_at` alone cannot serve the predicate efficiently.

**Timestamp precision.** The ordering timestamp is **never** rendered as text. It is carried
as an integer count of microseconds since the Unix epoch, UTC, which is exactly the
resolution PostgreSQL `TIMESTAMPTZ` stores. `time.RFC3339` has no sub-second field at all,
which is why the current cursor truncates downward and re-returns records (E4, E5);
`RFC3339Nano` would round-trip but strips trailing zeros, making the same value have several
textual spellings — an integer has one.

**Cursor.** `base64url(compact JSON)`, no padding:

```jsonc
{ "v": 1,                  // manifestVersion the traversal was started under
  "c": "artifacts",        // class id
  "k": { "created_at": 1754308800123456, "id": "art_01H…" } }   // the ordering tuple
```

**Cursor refusals**, each a typed error, none of them a silent restart:

| Condition | Outcome | Requirement |
|---|---|---|
| `c` is not the class being traversed | refused, naming both classes | R-111-12, SCN-111-B04 |
| `v` is not the current manifest version | refused; the order or field set may have changed under the cursor | R-111-03 |
| Not decodable, or missing a key component | refused | R-111-12 |
| RFC3339 text, the pre-existing format | **refused**, not coerced | E3, E4 |

Refusing the old format is deliberate. Coercing an RFC3339 cursor into the new order would
have to guess an `id` for the tie, and any guess either skips or repeats — silently, which
is the defect being removed. The correct answer to an old cursor is to restart the traversal
knowingly.

**The cursor is not an authorization token, and is not signed.** It names a position, not a
permission; authority comes from the principal and the grant, which are checked
independently on every page. A forged cursor can only make the holder's own traversal skip
their own records. It carries a checksum for corruption detection, not a signature, and this
paragraph exists so a later reader does not mistake the absence of a signature for an
oversight.

---

## Bundle Format And The Export / Import Interfaces

**Container.** An uncompressed `tar` stream, written in one pass.

```
bundle-part-000.tar
├── part.json                     # header, first member, always
├── classes/artifacts.ndjson      # one canonical JSON record per line
├── classes/edges.ndjson
└── … one file per corpus class covered by this part
```

`part.json` is written **first** so a reader can determine the manifest version and the
census without consuming the archive (NFR-111-04). `tar` rather than `zip` because it is
writable as a stream without seeking back to patch a central directory, so an export can
stream into an HTTP response and still be interrupted safely.

**Header.**

```jsonc
{
  "bundleFormat": 1,               // container version — independent of manifestVersion
  "manifestVersion": 1,
  "partIndex": 0,
  "complete": false,               // true only in the final part, and only if every class ended
  "createdAt": 1754308800123456,
  "coverage": {                    // per class, what this part actually covered
    "artifacts": { "count": 4096, "fromCursor": null, "toCursor": "eyJ2Ijox…" }
  },
  "census":     { "artifacts": 4096, "edges": 0 },   // final part only; includes zero-count classes
  "classHashes":{ "artifacts": "sha256:…" },          // final part only
  "uncovered":  []                 // classes the export could not read, with the reason
}
```

**Incompleteness is a first-class state, not an error path.** `complete` is `true` only when
every `corpus` class reached the end of its traversal and `uncovered` is empty. An export
that cannot read a class writes that class into `uncovered` with a reason and leaves
`complete` false — which is R-111-09 and SCN-111-A04 satisfied by the artefact's own shape
rather than by a log line the user never sees. Zero-count classes appear in `census` with
`0`, so "absent" and "empty" are distinguishable (R-111-06, SCN-111-A03).

**Resumability** (NFR-111-01). Interruption discards the part being written and resumes from
each class's last committed `toCursor` into `part-001.tar`. Parity is computed over the union
of parts; `census` and `classHashes` are written only in the final part, once every class has
ended. A part set with no `complete: true` member is an incomplete export and an importer
refuses it.

**Export interface.**

```go
// ExportRequest names the scope. Nothing else narrows it: membership is the
// manifest's disposition, per D1.
type ExportRequest struct {
    Resume  map[ClassID]Cursor // empty for a fresh export
    MaxRows int                // per class, per part
}

type ExportResult struct {
    Header    PartHeader
    Uncovered []Uncovered      // class + reason; non-empty implies Header.Complete == false
    Next      map[ClassID]Cursor
}
```

**Import interface and its refusals.** Every refusal below happens **before any transaction
is opened**, so "applies nothing" is structural rather than a rollback promise:

| Condition | Outcome | Requirement |
|---|---|---|
| `bundleFormat` not in the reader's supported set | refuse; report both the bundle's value and the supported set | NFR-111-04 |
| `manifestVersion` not in `supportedManifestVersions` | refuse; **no translation is attempted** | R-111-07, Non-Goal 3 |
| No part carries `complete: true` | refuse — a partial bundle would produce a destination that silently differs | R-111-13 |
| A class hash does not match the records read for it | refuse, naming the class | R-111-13 |
| The destination is non-empty for any `corpus` class | refuse — merge is out of scope and guessing at conflicts is worse than refusing | Non-Goal 6 |

`supportedManifestVersions` is an explicit set, initially `{1}`, and never a `>=` comparison.
A range would silently accept a future bundle whose field set the reader cannot know, which
is the unknown-version failure R-111-07 exists to prevent.

**Import preserves what parity depends on.** Identity keys and portable timestamps —
including `created_at` and `updated_at` — are written verbatim, not regenerated. An import
that stamped `updated_at = NOW()` would break `SCN-111-C01` by construction. Instance-local
bookkeeping (`last_accessed`, `access_count`) starts fresh at the destination and is excluded
from the hash, so the two facts do not conflict. `processing_status` is restored as recorded,
which is what makes `SCN-111-C02`'s failed capture arrive still failed rather than arrive
pending.

---

## Canonical Census And Parity

**Record canonical form.** For each class, the manifest's `portableFields`, rendered as JSON
with:

- keys sorted by UTF-8 byte order;
- no insignificant whitespace;
- `null` emitted explicitly for a NULL column — never omitted, so "absent" and "null" cannot
  collide;
- timestamps as integer microseconds since the Unix epoch, UTC;
- integers as bare decimal; floating-point values are not permitted in `portableFields`
  (`relevance_score` is excluded as derived), so no float formatting question arises;
- JSONB columns re-serialised through the same canonical renderer, recursively — Postgres
  `jsonb` does not preserve input key order, so the stored bytes are not stable and only a
  re-render is.

`recordHash = SHA-256(canonicalBytes)`.

**Class hash.** Order-independent by construction:

```
classHash = SHA-256(
    "smackerel/corpus-class/v1\n" ‖ classID ‖ "\n" ‖ decimal(recordCount) ‖ "\n" ‖
    concat over records sorted by canonical identityKey bytes of ( identityKeyBytes ‖ recordHash )
)
```

Sorting by identity key rather than by record hash is a diagnostic choice: when two instances
disagree, the first differing position names the record, so a mismatch is locatable without
re-exporting. Including `recordCount` in the preimage prevents a truncated class from
colliding with a shorter genuine one. The domain-separating prefix prevents a class hash from
being confused with any other SHA-256 in the system.

**What parity proves and what it does not.** Equal counts and equal class hashes prove the
portable field set arrived intact. They say nothing about excluded fields, which is correct —
those are instance-local or recomputable by definition. Any surface reporting parity must say
which of the two it is asserting; "the corpora are identical" would be a stronger claim than
the mechanism supports and NFR-111-06's honesty constraint applies to it.

---

## Deletion

**Scopes** (R-111-15): `artifact`, `source`, `topic`, `whole-corpus`. Each resolves to a set
of root records, and the delete engine expands that set through the manifest's `references`
graph applying each class's `deleteRule` (D4). The expansion is computed and **reported
before** anything is deleted, which is what makes the confirmation in R-111-16 meaningful:
the user confirms a stated blast radius, not the word "delete".

**Confirmation** (R-111-16, SCN-111-C05). The request must carry an explicit confirmation
token that names the resolved scope. A request without it performs no deletion and returns
the scope statement it would have erased. The token names the scope so that a confirmation
cannot be replayed against a differently-resolved scope.

**Verification** (R-111-14, P5, SCN-111-C03). After the delete transaction commits, the
engine re-runs the census over the affected classes and reports the observed counts. Success
is the observed emptiness, not the absence of an error — a delete that returned cleanly and
left rows is a failure this check catches and a status code does not.

**Whole-corpus delete erases `corpus` and `derived`, and leaves `operational` intact.** The
user's knowledge is the corpus plus everything computed from it; the instance's own
credentials, tokens and traces are not the user's knowledge and erasing them would break the
instance without advancing the guarantee. R-111-14's "every manifest-declared class is empty"
is read against the classes the manifest declares as the corpus, and the manifest states the
disposition of every other table explicitly — so the exclusion is auditable rather than
implicit.

**No permission gate** (R-111-17). The delete requires the corpus owner's confirmation and no
other party's approval. It is not behind a grant, an operator review, or a support path.

---

## Sensitivity Storage

**Schema**, extending `artifacts` and following the E9 pattern of typed column plus CHECK:

```sql
ALTER TABLE artifacts
    ADD COLUMN sensitivity_tier       TEXT,          -- NULL == unset
    ADD COLUMN sensitivity_source     TEXT,          -- provenance: who/what decided
    ADD COLUMN sensitivity_decided_at TIMESTAMPTZ,
    ADD COLUMN sensitivity_decided_by TEXT;          -- principal id when source = 'operator'

ALTER TABLE artifacts
    ADD CONSTRAINT artifacts_sensitivity_tier_vocab
        CHECK (sensitivity_tier IS NULL
               OR sensitivity_tier IN ('low', 'medium', 'high')),
    ADD CONSTRAINT artifacts_sensitivity_source_vocab
        CHECK (sensitivity_source IS NULL
               OR sensitivity_source IN ('operator', 'connector', 'classifier',
                                         'import', 'migration')),
    -- classified implies provenance, and unset implies no provenance: all-or-nothing
    ADD CONSTRAINT artifacts_sensitivity_provenance_complete
        CHECK (num_nonnulls(sensitivity_tier, sensitivity_source, sensitivity_decided_at)
               IN (0, 3));

CREATE INDEX idx_artifacts_sensitivity_tier ON artifacts (sensitivity_tier);
```

The third constraint is how R-111-22 stops being a rule someone must remember. "Re-classification
replaces provenance, never blanks it" becomes a state the database will not hold: a row with a
tier and no source, or a source and no tier, cannot be written at all. `SCN-111-D04` then tests
a property the schema guarantees rather than a discipline the code hopes for.

**Vocabulary refusal at write time** (R-111-21, SCN-111-D03). An out-of-vocabulary value is
rejected by the CHECK, the statement fails, and the prior value is unchanged because the
update did not apply. The application layer validates first to return a typed error, but the
constraint is what makes the guarantee hold against a path that forgot to validate.

**Unset is not least-sensitive** (R-111-20, SCN-111-D02). `NULL` is not a member of the
ladder. Reads return a distinct unset state, and every comparison against a ceiling yields
`NULL` — not true — so the SQL-level default is refusal. See D5 for why this representation
was chosen over an `'unset'` vocabulary value.

**Sensitivity travels** (R-111-23, SCN-111-C06). All four columns are in the `artifacts`
class's `portableFields`, so they are hashed, carried, and restored. A bundle that moved a
record and dropped its classification would make the record *less* classified by moving,
which spec.md §4.2 names as a portability defect rather than a sensitivity one.

---

## The Egress Decision

**Placement.** `internal/assistant/openknowledge/llm.DispatchResolver`, remote branch only.
The local (`ollama`) branch leaves `ChatRequest.APIKey` nil (`llm/client.go:118-120`) and
takes no decision — local inference is the default, structurally.

**Signature.** The decision is an argument, not an ambient lookup, so omitting it is a compile
error rather than a runtime oversight:

```go
// ResolveRemote is the only path that populates ChatRequest.APIKey for a hosted
// target. It cannot be called without a decision, and it refuses a decision that
// does not permit, that names different artifacts, or that has expired.
func (r *DispatchResolver) ResolveRemote(
    modelID string,
    scope   ArtifactScope,     // the artifact ids whose content is in the request
    decision egress.Decision,  // required; there is no variant without it
) (Resolved, error)
```

**Inputs and their refusal order** (R-111-24, R-111-25, SCN-111-E01…E05). Every input is
evaluated; the first failure refuses, and the refusal happens before any request is
constructed, let alone sent (R-111-26):

| # | Input | Missing or disallowed ⇒ |
|---|---|---|
| 1 | Authenticated principal | refuse — `principal_absent` |
| 2 | Grant covering this principal and this scope | refuse — `grant_absent` / `grant_does_not_cover` |
| 3 | Credential audience equals the audience being addressed | refuse — `audience_mismatch` |
| 4 | Every in-scope artifact has a `sensitivity_tier` | refuse — `sensitivity_unset` |
| 5 | Every in-scope tier is at or below the grant's ceiling | refuse — `sensitivity_above_ceiling` |

Steps 4 and 5 evaluate over the **whole scope**, not a sample: one unset or one
above-ceiling artifact refuses the request. A per-artifact filter that dropped the offending
records and proceeded would be a permissive fallback wearing a filter's clothes, and P7
forbids it.

**The ceiling comparison reuses the existing ordering.**
`qfdecisions.PersonalContextTierLessOrEqual(artifactTier, grantCeiling)`
(`personal_context_consent.go:348`) — one ordering function, already single-sourced, already
treating invalid input as most restrictive. The shipped default ceiling is **none**: a grant
with no ceiling set permits no tier to leave, so a newly created grant is fail-closed until
the operator sets a ceiling deliberately (R-111-29 — never global, never default-on).

**Recording** (R-111-28, SCN-111-E07). Every decision is persisted with principal, model
target, scope size, outcome and typed reason — permits with the same fidelity as refusals.
The record carries no artifact content and no credential; the reason vocabulary is closed and
built from the typed reason plus identities, following the secret-safety discipline already
documented at `dispatch_resolver.go:20-25`. A `corpus` disposition applies to this record
class: an auditor's history of what left is the user's, so it exports and it deletes.

**Non-bypassability** (NFR-111-03). There is no configuration, environment variable or build
tag that disables the decision. The staging in D6 controls *whether the sensitivity inputs
deny*, and it is a per-instance state, not a compile-time switch — R-111-29 rules out a
build-time switch explicitly.

---

## Testing And Validation Strategy

Each design decision is paired with the check that would fail if the decision were reverted.
The categories are the ones already in `scopes.md`'s Test Plan; nothing new is introduced
here, and no file path is named because none exists.

| Decision | What must be proved | Category | Fails if reverted to |
|---|---|---|---|
| D1 | A table added to the store with no manifest disposition fails the coverage check, naming it | integration | a hand-written include-list |
| D1 | A `portableFields`/`excludedFields` union that omits a real column fails, naming it | unit | a class-level-only check |
| D2 | The same corpus on two instances yields identical class hashes | e2e-api | hashing wire bytes or whole rows |
| D3 | Records sharing one instant across a page boundary are each returned exactly once | integration | `created_at > $1` with no tiebreak |
| D3 | Sub-second-apart records survive a resume without repeat or skip | unit | an RFC3339 cursor |
| D3 | A cursor from another class is refused, not restarted | unit | an opaque scalar cursor |
| D4 | A scoped delete removes the cascade closure and leaves independent containers present | integration | cascade-everything or orphan-everything |
| D5 | An unset artifact is distinct from `low` and fails a ceiling test | unit | an `'unset'` vocabulary member |
| D5 | A tier without provenance cannot be written | unit | application-only validation |
| D6 | An out-of-vocabulary metadata value is reported by the migration, not dropped | integration | inheriting the silent skip |
| D7 | A remote resolution without a decision does not compile, and with a refusing decision makes no external call | unit | an ambient or middleware gate |
| D7 | Every path that reaches a model is covered, including a background job that traverses no route | integration | route-level middleware |

The two boundary scenarios the spec singles out — `SCN-111-B01` and `SCN-111-B02` — are
adversarial by construction under this design: B01's fixture must contain records with
byte-identical `created_at`, which the current query provably skips, and B02's must differ
below one second, which the current cursor provably duplicates. A fixture that satisfied the
current implementation would not be testing either defect.

---

## Complexity Tracking

| Deviation from the simplest viable approach | Simpler alternative | Why the simpler one was rejected |
|---|---|---|
| Three dispositions rather than an include-list | `corpus` / not-corpus | `derived` and `operational` differ in *delete* behaviour, not just export: derived rows must die with their source, operational rows must survive. Two categories cannot express that, and collapsing them either orphans copies of deleted content or breaks the instance. |
| Exhaustive column-level check, not just table-level | Table-level only | A column added by a later migration to an already-classified table would be silently non-portable. That is the same silent-omission defect one level down, and it would not be caught by any table-level check. |
| Multi-part bundles with a completion header | One bundle, one file | NFR-111-01 requires resumption across interruption. Without parts, an interrupted export must restart from zero, which for a corpus larger than a session means it never completes. |
| Order-independent class hash sorted by identity key | Hash records in traversal order | Traversal order and insertion order differ between source and destination, so an order-dependent hash reports a false mismatch on a correct import. |
| Egress decision passed as a required argument | A package-level gate function callers invoke | A function callers *should* call is a convention; an argument they *must* supply is a compile error when omitted. R-111-27 asks for structural coverage, and only one of these delivers it. |
| Retaining `metadata` in `portableFields` alongside the new sensitivity columns | Drop `metadata` once sensitivity moves out | `metadata` carries connector-specific content beyond sensitivity. Dropping it would lose user content to fix a schema smell. |

---

## Constraints The Design Could Not Trade Away

Restated from `spec.md` because they bound the design space rather than sit inside it, each
paired with the mechanism in this design that honours it — so a reviewer can check the
binding rather than take it on trust.

| Constraint | Honoured by |
|---|---|
| **One manifest governs three operations** (P1) | All three read `disposition` from one embedded manifest; no operation holds a class list, and the coverage check fails on any table none of them classifies (D1) |
| **No second export path** (Non-Goal 1) | `ExportArtifacts` at `internal/db/postgres.go:110` is corrected in place — its state filter is dropped, its query becomes the manifest-driven traversal, and its RFC3339 cursor is replaced, not supplemented (D3) |
| **The egress decision fails closed** (P7, R-111-25) | Five inputs, any of which refuses; a grant with no ceiling permits no tier; `NULL` sensitivity fails a ceiling comparison at the database, not only in code (D5, D7) |
| **Unset sensitivity is not permissive** (R-111-20) | `unset` is SQL `NULL`, not a ladder member, so it cannot compare at-or-below anything (D5) |
| **Membership never depends on processing state** (P2, R-111-08) | Export membership is `disposition == "corpus"` and nothing else; `processing_status` is a portable *field*, never a filter (D1) |
| **Copy obeys Principle 11's honesty constraint** (NFR-111-06) | While D6's OBSERVE window is open the product does not enforce the egress clause, and this design says so in D6 rather than letting a later surface imply otherwise |

---

## Capability Shape

### Single-Implementation Justification

**One capability is introduced — corpus portability — and the completed design produced no
second implementation to split from.** This is now a finding of the design pass rather than a
restatement of spec constraints: every decision D1 through D8 is made, and the question was
re-asked against the answers.

The gate applies because trigger words occur. They occur in three places, and none is an
implementation set. `provider` appears in `spec.md` §4 in a sentence arguing *against*
provider-first modelling. It appears here naming
`internal/assistant/openknowledge/llm.DispatchResolver` — spec 096's **existing** provider
registry, which this design *consumes* as the chokepoint and does not extend, replace, or add
a member to. And `connector` appears naming existing packages under `internal/connector/`
whose ordering function is reused.

Four candidates were examined for a genuine second implementation, and each fails for a
stated reason rather than by assertion.

| Candidate | Why it is not a second implementation |
|---|---|
| Export, import, delete | Three *operations* over one declaration (P1), not three interchangeable realisations of one contract. They do not share a call signature, cannot substitute for one another, and no caller selects among them at runtime. |
| The `corpus` / `derived` / `operational` dispositions | Categories of **data**, consumed by one engine each. A disposition is a value in a table, not a type with variants. |
| The `cascade` / `unlink` / `independent` delete rules | The closest call, and still declarative: one delete engine reads a per-class rule field. There is no dispatch to three delete implementations, and adding a fourth rule would be a `switch` arm, not a new provider. |
| Local versus remote inference | Already spec 096's provider abstraction, already built. This design adds a required argument to one branch of it. Consuming an existing abstraction is not introducing one. |

Declaring `## Capability Foundation` / `## Concrete Implementations` / `### Variation Axes`
would require naming two or more concrete implementations and at least two variation axes.
After the full design pass there are none to name, so those sections could only be filled by
inventing a second implementation in order to have something to abstract over — which is the
inversion the proportionality rule exists to prevent.

**What would reopen this, stated as a testable condition rather than a caveat.** If a later
feature introduces a second **bundle transport** (a bundle written to object storage or
streamed to a peer rather than produced as a `tar` byte stream), the bundle codec acquires a
real second implementation and the split becomes required. `bundleFormat` is versioned
independently of `manifestVersion` in the header precisely so that change is expressible;
that is a seam, not an abstraction, and the difference is that a seam costs one integer while
an abstraction costs a foundation nobody needs yet.

---

## Open Decisions Requiring Operator Input

These are recorded as open because they cannot be settled by reasoning about the codebase.
Each names the exact question and what would unblock it. Manufacturing an answer to any of
them would make a heading look complete and the artifact less true.

### OD-111-01 — When to flip sensitivity enforcement from OBSERVE to ENFORCE

**Question.** After the D6 migration, some number of artifacts remain unset, and under
R-111-25 each of them refuses any egress request that includes it. At what point does the
operator flip ENFORCE: when the unset count reaches zero, when it falls below some
threshold, or on a date regardless of the count?

**Why it is not decidable here.** The answer depends on the operator's own corpus — how many
artifacts are unset, whether they are reachable by any egress path at all, and whether the
operator intends to classify them or to accept that they never leave. None of those facts
exists in the repository, and no amount of design reasoning produces them.

**What unblocks it.** The OBSERVE stage's own output: the unset count, and the count of
requests it would have refused, over a period the operator chooses. D6 exists to produce
exactly this number. Owner: operator, via `bubbles.plan` when SCOPE-07 completes.

**Default until then.** ENFORCE is off. The mechanism, the measurement, and the OFF default
are decided; only the flip is open.

### OD-111-02 — The per-grant sensitivity ceiling values

**Question.** Which tiers may leave, for which client? The design ships the ceiling
*mechanism* and a deny-all default; the values are policy.

**Why it is not decidable here.** `spec.md` Non-Goal 4 assigns it: *"Deciding the
classification of any specific artifact… is a policy question for the operator."* The same
reasoning covers the ceiling, which is the policy's other half.

**What unblocks it.** An operator decision per grant. Owner: operator. The fail-closed
default means an undecided ceiling denies rather than permits, so this open decision cannot
turn into a silent permission.

### OD-111-03 — Whether the litellm sidecar is reachable other than by the Go core

**Question.** D7 places the chokepoint at the credential seam in the Go core. That is the
last gate **only if** nothing else can reach the sidecar that performs the outbound call.

**Why it is not decidable here.** It is a deployment-topology fact about network reachability,
not a code fact, and this pass ran no container and inspected no running network. Asserting
it from the compose file would be inferring runtime reachability from a static declaration.

**What unblocks it.** SCOPE-08 verifying the boundary as part of its `SCN-111-E06`
coverage — the same scenario already requires proving every path is governed, and the
sidecar's reachability is one of those paths. Owner: `bubbles.plan` → SCOPE-08. Recorded here
so the assumption is visible rather than buried.

### Still owned elsewhere

`F-111-FLAG-01` (BLOCKING) is unchanged and is not a design decision. Declaring the feature
flag requires editing `config/feature-flags.mvp.yaml` so the flag is default-OFF in the
non-owning train (G111), and that bundle is `bubbles.train`'s. `flagsIntroduced` stays empty.

---

## Findings Status After This Pass

Only the **design-owned half** of each finding is closed here. A finding owned by another
agent is untouched, and no finding's severity was reclassified by this pass.

| Finding | Owner | Status after this pass |
|---|---|---|
| `F-111-MANIFEST-01` (BLOCKING) | `bubbles.design` | **Design half closed** by D1. The derivation rule, the schema, and the failure mode are fixed. Producing the v1 class rows by running the coverage check against the store is SCOPE-01's execution, not design. |
| `F-111-CENSUS-01` | `bubbles.design` | **Closed** by D2 — field set, key order, number and timestamp form, null handling, record order and domain separation are all fixed. |
| `F-111-DELETE-01` | `bubbles.design` | **Closed** by D4 — `cascade` / `unlink` / `independent`, declared per class, with the parity consequence stated. |
| `F-111-BACKFILL-01` (BLOCKING) | `bubbles.design` | **Design half closed** by D6 — backfill rule, reporting rule, and staged rollout decided. The flip timing is `OD-111-01` and is operator-owned by nature, not by deferral. |
| `F-111-110-01` | `bubbles.design` with spec 110 | **Dissolved** by D1. A later-arriving passage class is an unclassified table and fails the coverage check by name, so 111 no longer waits on 110 and 110 cannot land silently. Verified: spec 110 is `not_started`. |
| `F-111-108-01` | `bubbles.plan` | **Narrowed, not closed.** D8 confines the dependency to SCOPE-08's ENFORCE flip; SCOPE-08 is buildable now against a grant-decision port. Reclassifying the severity belongs to the owning agent. |
| `F-111-FLAG-01` (BLOCKING) | `bubbles.train` | **Unchanged.** Not a design decision. |
| `F-111-TEMPLATE-01` | operator via `bubbles.plan` | **Unchanged.** Not a design decision. |

**Routed to `bubbles.plan`, discovered by this pass and not acted on here.**

- `scopes.md` carries no Gherkin blocks; all 25 `SCN-111-*` scenarios live in `spec.md`. The
  traceability guard reads `scopes.md`, so it reports zero. The sibling packet that reached
  this ceiling, spec 109, carries its scenario blocks in `scopes.md`. This design changed no
  scenario and renumbered none, so the relocation is planning-owned and is recorded rather
  than performed.
- `SCN-111-C04`'s "every record outside that scope is still present and unchanged" needs the
  D4 reading — *outside the cascade closure* — written into the scope, or the test will
  assert behaviour this design deliberately does not implement.
- SCOPE-01's blocked-by line still cites `F-111-MANIFEST-01` as underivable. D1 answers it;
  the scope text is `bubbles.plan`'s to update.
