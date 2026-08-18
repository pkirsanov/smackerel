# Vision — Smackerel `next`

## What Smackerel is (restated in full — this doc stands alone)

Smackerel is a self-hosted personal knowledge system. It observes what a person
already reads, saves, writes, and receives, and it turns that stream into a graph
it can reason over. The user does not tag, file, or classify at capture time — the
system infers structure from observation. When the user later asks a vague
question, the system answers precisely, with the sources attached.

The corpus is the user's. It lives on hardware the user controls. Smackerel never
sends the corpus anywhere; only a client the user explicitly authorizes may, and
that authorization is per-client and audited.

## What phase `next` is for

`next` is the **promotion-candidate gate**. It is backed by release train `next`
(`config/release-trains.yaml`, slot `staging`) — the staging train that proves a
capability set before it is promoted onto the `mvp` train.

The `mvp` gate proved that Smackerel can *capture* and *deliver*: ingest from many
sources, build the graph, produce a digest, surface it without nagging. `next`
proves the layer above that: that Smackerel can **reason over the corpus and answer
honestly**, and that the model behind that reasoning is not welded into the build.

Concretely, `next` is the phase where these four things become simultaneously true:

1. **Retrieval is intent-aware.** A question that wants a whole document, a question
   that wants an aggregate, and a vague "what was that thing about…" no longer
   share one retrieval path. The router picks a strategy per query intent, over the
   single existing store.
2. **Synthesis composes rather than concatenates.** An answer is genuinely
   synthesised from multiple artifacts and carries its sources, or it refuses
   honestly. It never dresses a failure as a success.
3. **The model is a runtime choice.** Switching the synthesis model does not require
   a rebuild, and the local-inference default is never displaced by a cloud default.
4. **The graph has a public read surface.** Other software can read the graph
   through a stable API instead of reaching into the store.

## What shipping `next` proves

That Smackerel's intelligence layer is real rather than a wrapper around a single
prompt. Anyone can hand a corpus to a language model. `next` is the claim that
Smackerel routes retrieval by intent, synthesises with attribution, refuses when it
cannot ground an answer, and does all of that against a corpus the operator owns and
a model the operator selected at runtime.

## Audience this phase serves

The operator running Smackerel on their own hardware, who has passed the `mvp` gate
and now wants the system to be *useful for thinking*, not just for recall. Secondary
audience: authorized external clients — an MCP-speaking assistant, a companion
product — that want to read the corpus through a supported surface rather than a
private one.

## Success signal

Observable, not asserted:

- Gate G101 for phase `next` reports every delivered train member as `DELIVERED` with `validate` certification, and turns red the moment one regresses.
- A vague natural-language question returns a precise answer with its sources attached, and a question with no grounded answer returns an honest refusal rather than a fabricated one or a capture acknowledgement.
- The synthesis model can be switched at runtime and the change is observable in the next answer's trace, without a rebuild.
- The knowledge-graph public API answers a read from outside the process boundary.

## Non-goals for `next`

- **No new connectors.** The MVP connector-roster lock is still in force. Roster expansion is `v1` scope.
- **No outbound action.** Smackerel still only reads. Writing to a user's mail, calendar, or messages is `v1` scope (V2-A foundation first).
- **No native mobile.** The PWA remains the client surface; the native decision is a `v1` item.
- **No cloud-default inference.** Runtime-switchable models make cloud an *option*. Local inference stays the shipped default (Product Principle 11, constitution C1).
- **No parallel store.** Every capability in this phase operates over the one existing pgvector + knowledge-graph + structured store (Product Principle 5).

## Relationship to the other phases

| Phase | Relationship |
|---|---|
| [`mvp`](../mvp/) | `next` promotes on top of it. It deprecates nothing and re-binds nothing that `mvp` owns. |
| [`v1`](../v1/) | `v1` is a forward-looking planning phase with no train. Where `v1` planned a capability (item **V7**) that was then delivered on train `next`, this packet holds the enforcing binding and `v1` re-binds it for narrative continuity. |

## Cross-product context

The QF Companion boundary is unchanged by this phase. Smackerel remains read-only
from the QF side: it may carry QF decision-packet metadata without modifying it, and
it initiates no financial action (Product Principle 10). Nothing in `next` — not the
public graph API, not the MCP server plan — creates a write path into QF.
