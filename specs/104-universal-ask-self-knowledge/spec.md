# Spec 104 — Universal `/ask` + Self-Knowledge Grounding

> **Status:** draft (planning)
> **Owner:** bubbles.goal
> **Depends on:** 061 (conversational assistant), 064 (open-ended knowledge agent),
> 084 (open-knowledge reasoning loop), 095 (retrieval strategy routing).
> **Composes with — does NOT weaken:** the cite-back verifier (064 SCOPE-08) and
> the provenance gate (061 Principle 8). BUG-061-009's honest-refusal invariant
> remains the fallback when nothing can be grounded.

## Problem

`/ask` (the `open_knowledge` agent) can already ground answers in the user's
knowledge graph (`internal_retrieval`) and the live web (`web_search`), and it now
refuses **honestly** when it can't ground an answer (BUG-061-009). But it has a
blind spot that is inexcusable for a personal "second brain": **it cannot answer
questions about smackerel itself.**

Live evidence (BUG-061-010, home-lab, 2026-07-23): `/ask how does smackerel work
as a second brain?` grounds **zero** sources and refuses, because:

- `web_search` (searxng, enabled) resolves the token "smackerel" to a **Super
  Mario enemy / a small snack** — the public web has no knowledge of this private
  product.
- `internal_retrieval` is **empty** for meta-questions — the product's own docs
  were never ingested into the graph.
- the local LLM (gemma/qwen) has **no training data** on a private product, so an
  ungrounded answer would be a hallucination.

The bot has never been **told about itself**. Answering "what can you do", "what
recipes do you have", "how do I use X", "what commands exist" is an **essential,
first-class skill** for a personal knowledge assistant — not an afterthought.

## Vision

`/ask` recognises and answers **any** question by grounding on the right source:

1. the user's knowledge graph (`internal_retrieval`) — personal data,
2. the live web (`web_search`) — world knowledge,
3. computation (`calculator`, `unit_convert`),
4. **NEW: smackerel's own capabilities (`self_knowledge`)** — the product's
   skills, slash commands, recipes, features, use-cases, scenarios, and
   how-to/overview docs, as a **grounded, cited** source.

When no source can ground an answer, it refuses honestly (BUG-061-009). The agent
`tool_allowlist` is the **general seam**: any command or data surface can be wired
as an answer source under the same `Tool` contract, so "use any other command for
answering" generalises uniformly.

## Actors

| Actor | Goal |
|---|---|
| End user (Telegram/web/whatsapp) | Ask anything — including "what can smackerel do?", "how do I …?", "what recipes/skills exist?" — and get a real, cited answer. |
| End user | Get an honest refusal (not a hallucination, not "saved as an idea") when nothing can be grounded. |
| Operator | Have self-knowledge stay **automatically fresh** as scenarios/commands/recipes change — never hand-maintain a capability doc. |
| Agent planner (LLM) | See a `self_knowledge` tool and route product/meta-questions to it. |

## Functional requirements

- **FR-1 — Self-knowledge is a grounded source.** A new `self_knowledge` tool
  implements the existing `openknowledge.Tool` contract and returns cited
  `Source` entries that pass the cite-back verifier and the provenance gate. It is
  **always in the `tool_allowlist`** (self-knowledge is essential; it is not
  operator-disableable to `off` by default).
- **FR-2 — Fresh-by-construction corpus.** The self-knowledge corpus is
  **derived from the live product SSTs**, never hand-maintained:
  - skills/scenarios ← `config/assistant/scenarios.yaml` + `skills_manifest`
    (user-facing label, description, `slash_shortcut`, examples),
  - slash commands ← `shortcuts.go` + the `SetMyCommands` inventory + `/help`,
  - recipes ← the `recipe_search` catalog,
  - features/use-cases/overview ← curated sections of `docs/smackerel.md` +
    `README.md`.
- **FR-3 — Real ingestion + semantic search (no shortcuts).** Self-knowledge
  entries are ingested as first-class **artifacts** via the existing ingestion
  pipeline (`RawArtifactPublisher.PublishRawArtifact`) under a **dedicated source
  namespace** (`source_id = "smackerel_self"`), so they receive real embeddings
  (`vector(384)` via the ML sidecar) and are deduped by `content_hash`
  (idempotent re-ingest on deploy). Retrieval is **embedding-cosine semantic
  search scoped to that namespace** — NOT keyword-only, NOT an in-memory bolt-on.
- **FR-4 — General embedding-backed namespace searcher.** This spec lands the
  embedding-backed semantic searcher deferred by 064 SCOPE-06, as a **general,
  namespace-parameterised** capability (`SemanticSearcher(namespace, query, k)`
  over pgvector cosine). `self_knowledge` consumes it scoped to `smackerel_self`;
  it is designed so `internal_retrieval` can adopt it for the personal namespace
  in a follow-on (no forked search path).
- **FR-5 — `self_knowledge` isolation.** The `self_knowledge` searcher returns
  ONLY `smackerel_self` artifacts; personal captures never leak into a product
  meta-answer, and product docs never pollute personal `internal_retrieval`.
- **FR-6 — `/help` / `/capabilities` human twin.** The human-facing capability
  surface reads the **same** SST-derived corpus as `self_knowledge`, so the menu
  a user sees and the answers `/ask` gives can never diverge.
- **FR-7 — General command-as-source seam.** The design documents (and the code
  demonstrates via `self_knowledge`) the uniform pattern for wiring any future
  command/surface as an `openknowledge.Tool` answer source.
- **FR-8 — Honest fallback preserved.** When a meta-question cannot be grounded
  even in self-knowledge, the response is the BUG-061-009 honest refusal, never a
  hallucinated answer and never "saved as an idea".
- **FR-9 — Deploy-time (re)ingestion.** Self-knowledge (re)ingestion runs
  idempotently at deploy/boot so the corpus always reflects the deployed SSTs; a
  changed scenario/command/recipe/doc is reflected without manual steps.

## Non-functional requirements

- **NFR-1 — No provenance weakening.** The cite-back verifier and
  `requires_provenance` are unchanged. Self-knowledge answers are cited to real
  ingested artifacts; ungrounded synthesis is still rejected.
- **NFR-2 — Determinism / freshness.** Re-ingesting an unchanged SST set is a
  no-op (content-hash dedup). Changing an SST and redeploying updates the corpus.
- **NFR-3 — Bounded cost.** The self-knowledge corpus is small and bounded (the
  registered scenario/command/recipe set + a curated doc slice); ingestion and
  search stay within the agent's existing per-step budgets.
- **NFR-4 — Consistency.** `self_knowledge` search fidelity is the general
  embedding-cosine path; it is not a lesser keyword matcher, and it is the same
  substrate offered to `internal_retrieval`.

## Acceptance scenarios (BDD)

```gherkin
Feature: /ask answers questions about smackerel itself

  Scenario: A product meta-question is answered with citations
    Given smackerel's scenarios, commands, recipes, and overview docs are
      ingested under the "smackerel_self" namespace with embeddings
    When the user asks "/ask what can smackerel do?"
    Then the open_knowledge agent invokes the self_knowledge tool
    And the answer cites one or more smackerel_self artifacts
    And the response passes the cite-back verifier and the provenance gate
    And the response is StatusAnswered (never "saved as an idea")

  Scenario: A "how does smackerel work" narrative question is grounded in the docs
    Given the curated docs/smackerel.md overview is ingested under smackerel_self
    When the user asks "/ask how does smackerel work as a second brain?"
    Then the answer is grounded in and cites the ingested overview artifact(s)

  Scenario: A recipes question is grounded in the recipe catalog
    When the user asks "/ask what recipes do you have?"
    Then the answer cites recipe entries derived from the recipe catalog

  Scenario: Self-knowledge does not leak personal captures
    Given the user's personal graph contains a private note
    When the user asks a product meta-question routed to self_knowledge
    Then only smackerel_self artifacts are returned as sources
    And the private personal note is never cited

  Scenario: An unanswerable meta-question refuses honestly (BUG-061-009 preserved)
    When the user asks a product meta-question with no grounded self-knowledge match
    Then the response is an honest refusal ("I don't have a sourced answer for that.")
    And it is NEVER a hallucinated answer and NEVER "saved as an idea"

  Scenario: /help renders the same corpus the agent answers from
    When the user sends "/help"
    Then the rendered capability list is derived from the same SST-derived corpus
      that self_knowledge searches

  Scenario: A changed scenario is reflected after redeploy (fresh-by-construction)
    Given a new scenario is added to config/assistant/scenarios.yaml
    When smackerel is redeployed (self-knowledge re-ingestion runs)
    Then "/ask what can you do?" reflects the new scenario, with no hand edits
```

## Non-goals

- Weakening the cite-back verifier or `requires_provenance` (NFR-1).
- Letting the LLM answer product meta-questions from its own weights (it has no
  real knowledge of a private product — that is hallucination).
- Ingesting product docs into the user's **personal** graph (self-knowledge is a
  **separate** `smackerel_self` namespace).
- A full rewrite of `internal_retrieval` to embedding search (this spec makes the
  general searcher available; `internal_retrieval`'s adoption is a follow-on).
- Answering about **other** products/tenants (self = this product only).

## Traceability

| Requirement | Acceptance scenario(s) | Design section |
|---|---|---|
| FR-1 self_knowledge grounded source | 1, 5 | §Tool |
| FR-2 fresh-by-construction corpus | 6, 7 (redeploy) | §Corpus derivation |
| FR-3 real ingestion + semantic search | 1, 2, 3 | §Ingestion + §Search |
| FR-4 general embedding searcher | 1, 2 | §Search |
| FR-5 namespace isolation | 4 | §Namespace |
| FR-6 /help twin | 6 | §Human twin |
| FR-7 general seam | (design-documented) | §General seam |
| FR-8 honest fallback | 5 | §Trust integration |
| FR-9 deploy-time reingestion | 7 | §Ingestion lifecycle |
