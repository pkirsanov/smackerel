# BUG-061-010 — Specification: `/ask` must be able to answer questions about smackerel itself

- **Bug:** [bug.md](bug.md)
- **Parent spec:** [specs/061-conversational-assistant](../../)
- **Mechanism specs:** [064-open-ended-knowledge-agent](../../../064-open-ended-knowledge-agent/), [084-open-knowledge-reasoning-loop](../../../084-open-knowledge-reasoning-loop/)
- **Implementing spec (delivered the fix):** [104-universal-ask-self-knowledge](../../../104-universal-ask-self-knowledge/)

## What this packet is

This is a **diagnosis-and-route** packet. It specifies the behavior the product
should have, records why the deployed product did not have it, and names the
direction that closes the gap. It does not carry the implementation: the chosen
direction was routed to spec 104, which designed, implemented, and deployed it.

This spec therefore describes **expected behavior**, not delivered behavior. The
delivery contract lives in spec 104 and is owned there.

## Expected behavior

### EB-1 — A product meta-question must be answerable with grounded citations

When a user asks `/ask` a question about smackerel the product — what it is, how
it works as a second brain, what a given capability does — the assistant must
return an answer that cites real sources describing the product.

### EB-2 — Grounding must come from a citeable corpus, never from model weights

The answer must be grounded in retrievable artifacts. It must never be produced
by relaxing the provenance requirement and letting the language model narrate
from its weights, because a local model carries no training data about a private
product and would narrate something else entirely.

### EB-3 — The product corpus must not contaminate the user's personal graph

The corpus that makes EB-1 possible must be addressable independently of the
user's personal captures. A user's knowledge graph is theirs; product
documentation is not a personal capture and must not be mixed into it.

### EB-4 — The honest refusal remains the fallback

When no grounded source exists for a question, the assistant must still refuse
honestly (`I don't have a sourced answer for that.`) rather than answer. The
behavior established by [BUG-061-009](../BUG-061-009-high-band-refusal-masked-as-saved-as-idea/)
is unchanged by this specification. EB-1 narrows *when* the refusal fires; it
does not weaken the refusal itself.

## Acceptance criteria

| ID | Criterion | Owner |
|----|-----------|-------|
| AC-1 | A product meta-question through `/ask` returns an answer carrying at least one citation to a product-owned document | spec 104 |
| AC-2 | The citeable product corpus is addressable independently of the user's personal knowledge graph | spec 104 |
| AC-3 | The cite-back verifier and the provenance gate are unmodified by the fix | spec 104 |
| AC-4 | An ungroundable question still returns the honest refusal, never a capture acknowledgement | BUG-061-009 (delivered), re-asserted by spec 104 |
| AC-5 | A live behavioural confirmation through the deployed messaging channel | operator |

AC-1 through AC-4 are certified on spec 104. AC-5 is operator-owned: an agent
cannot send a message through the deployed Telegram channel, and the production
assistant HTTP surface requires a per-user PASETO token that agents do not hold.

## Not owned by this packet

- Any change to the cite-back verifier, the provenance gate, or `requires_provenance`.
  Weakening either would admit an ungrounded product answer, which is the exact
  failure mode this specification exists to prevent.
- Any change to the honest-refusal behavior delivered by BUG-061-009.
- The ingestion mechanism, the corpus namespace, the agent tool wiring, and every
  test that proves them. Those belong to spec 104.

## Product Principle Alignment

- **Principle 2 — Vague In, Precise Out.** A user asking "how does smackerel work
  as a second brain" is asking vaguely and expects a precise, sourced answer.
  Returning a refusal for every question about the product is a direct shortfall
  against this principle, which is why the gap is worth closing rather than
  accepting.
- **Principle 8 — Trust Through Transparency.** The deployed refusal was *correct*:
  no grounded source existed, so no citation could be attached. The specification
  above closes the gap by supplying real sources, never by relaxing attribution.
  EB-2 and AC-3 exist to keep that boundary explicit.
- **Principle 11 — Local-First Data Ownership.** EB-3 keeps the product corpus
  addressable apart from the user's personal captures, so ingesting product docs
  never mutates or dilutes the user's own graph.

No tension with a ratified principle was identified, and no deviation is claimed.
