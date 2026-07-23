# BUG-061-010 — `/ask` can't answer questions about smackerel itself (grounding gap)

- **Severity:** S3 (product capability gap, not a correctness defect)
- **Spec:** specs/061-conversational-assistant (mechanism lives in specs/064-open-ended-knowledge-agent + specs/084-open-knowledge-reasoning-loop)
- **Discovered:** 2026-07-23, during the BUG-061-009 close-out (operator `/ask how smackerel works as second brain or llm wiki?`)
- **Status:** blocked (fix DELIVERED + live via spec 104; only the shared operator Telegram smoke remains — see "Resolution")
- **Depends on:** BUG-061-009 (done) — that fix made this case *refuse honestly* ("I don't have a sourced answer for that.") instead of masking it as "saved as an idea". This bug is about making it *answer*.

## Symptom

`/ask <question about smackerel the product>` returns an honest refusal
("I don't have a sourced answer for that.") instead of an answer. The
open_knowledge agent runs to completion (`status=success termination=final`) but
grounds **zero** sources, so the cite-back verifier + provenance gate correctly
refuse the uncited synthesis.

## Root cause (live evidence, home-lab deploy host, 2026-07-23)

The open_knowledge agent has two grounding tools — `internal_retrieval` (the
user's knowledge graph) and `web_search` (searxng) — and **both come up empty for
a question about this private product**:

1. **web_search is enabled and working**, but the public web has no knowledge of
   this product. `ENABLE_SEARXNG=true` in the prod env; the agent is wired
   `provider=searxng model=gemma4:26b synthesis_model=qwen3:30b-a3b tool_count=4`.
   A direct searxng query for `smackerel second brain` returns:

   > "Smackerel — Super Mario Wiki … Smackerels are enemies that appear in Super
   > Mario Bros. Wonder. They resemble flatfish, with both eyes on the same side
   > of their body…"

   i.e. the token "smackerel" resolves on the public web to a **Super Mario
   enemy / a small snack**, never to this product. Web results are irrelevant, so
   nothing citeable is returned for a product meta-question.

2. **internal_retrieval is empty for meta-questions.** The user has never
   captured smackerel's own product docs into the knowledge graph, so there is no
   artifact describing "smackerel the second brain" to retrieve and cite.

3. **The local LLM has no training data on a private product.** gemma4:26b /
   qwen3:30b-a3b cannot know a private product's design from weights, so even
   relaxing provenance would produce a *hallucinated* answer (likely about the
   Mario enemy), not a correct one.

**Therefore the refusal is correct.** The gap is that the "second brain" has
never been told about itself — there is no grounded source, anywhere, that knows
what smackerel is. This is a **knowledge/data gap, not a code defect.**

## Fix options (the decision this bug is blocked on)

- **(A) Self-knowledge ingestion — RECOMMENDED.** Ingest smackerel's own docs
  (`docs/smackerel.md`, `README.md`, key spec summaries) into a citeable source
  the open_knowledge agent's `internal_retrieval` can find, so meta-questions are
  answered **with real citations to the product's own docs**. Sub-decision: ingest
  into the user's personal graph (simplest; risks mixing product docs with
  personal captures) **vs** a dedicated "system/help knowledge" collection the
  agent searches separately (keeps personal captures clean — preferred).
- **(B) A dedicated `about_smackerel` / help tool** in the agent tool_allowlist
  that serves the product's own docs as a first-class, always-available cite
  source, distinct from personal retrieval.
- **(C) Accept the limitation.** Leave meta-questions to the (now-honest) refusal
  from BUG-061-009. The bot simply doesn't answer questions about itself. No code
  change.

## Recommendation

**(A) with a dedicated system-knowledge collection** (the (B)-flavored variant):
seed smackerel's own documentation as a separate, citeable corpus so `/ask` about
the product answers with citations to the real docs, without polluting the user's
personal knowledge graph. This is the only option that makes the second brain
actually able to answer questions about itself with grounded, non-hallucinated
sources.

## Non-goals

- Weakening the cite-back verifier or `requires_provenance` (that would let
  hallucinated product answers through — the LLM has no real knowledge here).
- Changing the honest-refusal behavior from BUG-061-009 (that stays as the
  fallback when no grounded source exists).

## Resolution (2026-07-23)

**RESOLVED by supersession — fix option A delivered by [spec 104](../../../104-universal-ask-self-knowledge/).**

The operator/owner chose **option A with a dedicated system-knowledge collection**
(the recommended variant), and it was fully designed + implemented + deployed as
spec 104 (Universal /ask + Self-Knowledge Grounding). smackerel's own SSTs
(scenarios, shortcuts, skills, curated docs) are now derived fresh-by-construction
and ingested under a dedicated `smackerel_self` pgvector namespace — separate from
the user's personal graph, exactly as recommended — and surfaced via a first-class
`self_knowledge` tool in the agent `tool_allowlist`. `/ask` about the product now
answers with real citations to the product's own knowledge; the cite-back verifier
+ provenance gate + the BUG-061-009 honest refusal are unchanged as the fallback
for anything ungroundable.

This diagnosis bug required **no separate implementation** — it routed its fix to
spec 104. Live verification: core `sha256:3b6261a9…` + ml `sha256:25f36dc5…`
running/healthy on home-lab; `smackerel_self` corpus ingested (13 artifacts,
embeddings async). The single remaining item — the operator's live Telegram
behavioral smoke test — is tracked on spec 104 (operator-only; agent cannot send
Telegram / prod HTTP needs PASETO); the behavior is already e2e-proven.
