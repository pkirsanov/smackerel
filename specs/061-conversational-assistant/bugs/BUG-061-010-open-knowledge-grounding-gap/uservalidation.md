# BUG-061-010 — User Validation

> **Acceptance status: NOT GRANTED.** Every item below ships unchecked. Under
> `bubbles/registry/acceptance-authority.yaml` the `## Checklist` is
> human-written and automation MUST NOT check an item, because checking one
> would fabricate the single fact Gate G136 exists to require.
>
> This packet is a **diagnosis-and-route** packet. It observed the grounding
> gap, diagnosed it, chose a fix direction, and routed that direction to
> [spec 104](../../../104-universal-ask-self-knowledge/). It implemented
> nothing, so the behavioural acceptance below is acceptance of the *delivered*
> fix, which spec 104 owns and certifies.

## Checklist

- [ ] `/ask` a question about smackerel the product (for example "how does smackerel work as a second brain?") through the deployed Telegram bot returns an **answer**, not the honest refusal.
- [ ] That answer carries at least one **citation to a product-owned document**, rather than narrating from model weights.
- [ ] A question that genuinely has no grounded source still returns the honest refusal `I don't have a sourced answer for that.` — never "saved as an idea" (the BUG-061-009 behaviour is unchanged).
- [ ] Personal `/ask` retrieval is unchanged — product documentation has not leaked into, or crowded out, the user's own captured knowledge.

## Why these are unchecked

The single open item on this packet is an **operator-owned live behavioural
confirmation**, and it is shared with spec 104. It is not agent-attemptable:

- An agent cannot send a message through the deployed Telegram channel.
- The production assistant HTTP surface requires a per-user PASETO token that
  agents do not hold.

Nothing here is checked on the strength of spec 104's certification. Spec 104
proved the delivered behaviour with its own tests against the code that actually
changed, and commit `4a7c545d` automated the *render* half of this smoke via a
connector-only `/ask` test that injects a synthetic inbound update and asserts
the rendered outbound. That narrows the open item to a final confirmation against
the live deployment — it does not discharge it, and it is spec 104's artifact,
not this packet's.

Importing another packet's certification into this checklist would be
cross-packet fabrication, and checking a box on the operator's behalf would be
exactly the forgery the acceptance authority is built to prevent.

## What resolves this

The operator sends a product meta-question to the deployed bot, observes the
reply, checks the items above that hold, and authors the human acceptance record.
Until then this packet's `state.json` status stays `blocked`.
