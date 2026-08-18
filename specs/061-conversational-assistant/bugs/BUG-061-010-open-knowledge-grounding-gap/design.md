# BUG-061-010 — Design: root cause and fix direction

- **Bug:** [bug.md](bug.md) · **Spec:** [spec.md](spec.md) · **Scopes:** [scopes.md](scopes.md)
- **Implementing spec:** [104-universal-ask-self-knowledge](../../../104-universal-ask-self-knowledge/)

## Scope of this document

This packet designed a **diagnosis and a fix direction**. It did not design the
implementation. The implementation design — corpus derivation, namespace layout,
tool wiring, ingestion lifecycle, and the tests that prove them — is spec 104's
`design.md` and is owned there. Restating it here would create a second authority
for a contract this packet does not own.

## Root cause

The symptom was that `/ask <question about smackerel>` returned the honest
refusal `I don't have a sourced answer for that.` while the open_knowledge agent
itself completed normally (`status=success termination=final`) and grounded zero
sources. The refusal was produced by the cite-back verifier plus the provenance
gate, acting correctly on an uncited synthesis.

The defect is therefore **not** in the refusal path. It is upstream of it: every
grounding channel available to the agent was empty for this class of question.

| Grounding channel | State on the deployed system | Why it produced nothing |
|---|---|---|
| `web_search` (searxng) | Enabled and returning results | The public web has no knowledge of a private product. The token "smackerel" resolves publicly to a Super Mario Bros. Wonder enemy and to a snack, never to this product. Results were returned, and none were relevant. |
| `internal_retrieval` (user knowledge graph) | Reachable and functioning | The user had never captured smackerel's own product documentation, so no artifact describing the product existed to retrieve. |
| Local language model weights | Loaded and serving | A local model cannot carry training data about a private product. Relaxing provenance would have produced a confident narration about the Mario enemy, not a correct answer. |

The three rows share one shape: **the second brain had never been told about
itself.** No grounded source existed anywhere in the system that knew what
smackerel is. That makes this a knowledge/data gap, not a code defect — and it is
why "fix the verifier" is the wrong repair.

A working search integration is what makes this diagnosis non-obvious. The
natural first hypothesis for a zero-source answer is a broken or disabled search
tool. The recorded evidence rules that hypothesis out: search was wired, enabled,
and returning results. That is the observation that redirects the investigation
from the mechanism to the corpus.

## Fix options considered

### Option A — Self-knowledge ingestion (chosen)

Ingest smackerel's own documentation into a citeable source the open_knowledge
agent can retrieve, so meta-questions answer with real citations to the product's
own docs.

Two sub-variants were weighed:

- **A1 — ingest into the user's personal graph.** Simplest. Rejected: it mixes
  product documentation into the user's personal captures, which contaminates
  every subsequent personal retrieval and violates the ownership boundary the
  personal graph exists to hold.
- **A2 — ingest into a dedicated system-knowledge collection the agent searches
  separately.** Chosen. Keeps personal captures clean while making the product
  citeable.

### Option B — A dedicated help tool in the agent allowlist

Serve the product's own docs through a first-class, always-available tool
distinct from personal retrieval. Not rejected on merit — it converges with A2,
because A2 needs a retrieval surface anyway. The delivered design took the
A2-plus-B shape: a dedicated corpus namespace surfaced through a dedicated tool.

### Option C — Accept the limitation

Leave meta-questions to the honest refusal from BUG-061-009 and make no change.
Rejected: an assistant that cannot answer a single question about itself fails
the product's own promise, and the refusal — while correct — is a poor answer to
a question the system could answer from documents it already owns.

## Decision

**Option A2, in the B-flavored shape.** Derive smackerel's own knowledge into a
dedicated, separately-addressable corpus, surface it through a first-class tool
in the agent allowlist, and leave the cite-back verifier, the provenance gate,
and the BUG-061-009 refusal untouched as the fallback for anything ungroundable.

## Routing

The decision above was routed to **spec 104 (Universal `/ask` + Self-Knowledge
Grounding)**, which owns and delivered it. Per this packet's `state.json`, spec
104 derives smackerel's own SSTs fresh-by-construction, ingests them under a
dedicated `smackerel_self` pgvector namespace kept apart from the personal graph,
and surfaces them via a `self_knowledge` tool in the agent `tool_allowlist`.

This packet performed no implementation, ran no tests, and certifies no delivery.
That is why its terminal status is `blocked` rather than `done`: the
`bugfix-fastlane` completion gate asks for implement/test/validate/audit phases on
this artifact, and this artifact has none to show. Claiming them would be
fabrication; mirroring spec 104's status is the honest alternative.

## Verification design

Because this packet routed its fix, it designs no regression test and owns no
test file. The adversarial regression obligation is discharged on spec 104
against the implementation that actually changed.

The one verification this packet's own resolution depends on is a live
behavioural confirmation through the deployed messaging channel. That is
operator-owned and is shared with spec 104: agents cannot send Telegram messages,
and the production assistant HTTP surface requires a per-user PASETO token.
