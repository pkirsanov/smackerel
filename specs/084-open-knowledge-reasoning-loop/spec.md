# Spec 084 — Open-Knowledge Reasoning Loop

**Status:** in_progress (planning bootstrap; ceiling = `done`)
**Workflow Mode:** `full-delivery` (prelude: `analyze-design-plan`)
**Owner Directive (2026-06-11):** Make the Smackerel open-knowledge agent
(the `/ask` + NL open-ended path) a question-**agnostic** reasoning agent
that plans, drills in, reconciles, and answers ANY question type —
comparison, why, recommendation, trade-off, multi-hop factual — instead of
stitching snippets from a single search. **Keep the current model**
(gemma4:26b on self-hosted, gemma3:4b on dev). No model swap, no deepseek-r1
wiring. This delivery is the **loop + prompt + honesty** changes only, so we
can empirically test whether gemma4:26b reasons well once it is no longer in
a straitjacket.

**Depends On:** spec 064 (open-ended knowledge agent — the loop, the
cite-back verifier, the provenance gate, the SST config block).
**Amends:** spec 064. This spec does NOT reopen the closed
`BUG-064-001` / `BUG-064-002`; it builds on top of their shipped edits
(the per-user budget pre-flight and the snippet-dedup / source-cap
salvage) and changes the **reasoning guidance, the iteration budget, and
the salvage honesty** on top of that baseline.
**Unblocks:** a future model-comparison spec (empirically testing
gemma4:26b reasoning quality once the prompt/loop straitjacket is removed,
before any deepseek-r1 evaluation).

**Out of scope (explicit):** model selection, the hardware-tier model
matrix, `llm_model_id`, deepseek-r1 wiring, the spec-083 card-rewards WIP,
and the self-hosted deploy itself (a separate `bubbles.devops` dispatch).

---

## 1. Problem Statement

Spec 064 shipped an open-knowledge agent loop behind `/ask`. In live use on
self-hosted (gemma4:26b, 2026-06-11) the agent answers single-fact and unit
questions well but **fails open-ended reasoning questions**.

Motivating case — turn `73900d2089b6a557`:

> User: `/ask what is better place to grow pomegranate wa-town-A or wa-town-B, wa?`
> Result: iterations=4, web_search ×3, num_sources=5, status=success,
> termination=final.

The answer:
- **Never compared the two towns.** It read as "here is what each link
  said" — one paragraph per source.
- **Presented contradictory snippets unreconciled** ("thrives down to 0
  degrees" vs "cannot stand freezing") side by side as if both were the
  answer.
- Was a **confident topic summary**, not the comparison verdict the user
  asked for.

The root cause is **not** the model's raw capability. It is three platform
choices that put the model in a straitjacket and then mask the failure:

1. **Anti-multi-hop prompt bias.** The `agent_system_prompt` tells the model
   "After your FIRST successful tool call returns useful content, write the
   final answer in the NEXT turn. Do NOT keep calling the same tool" — the
   *opposite* of drilling in. A comparison needs evidence on BOTH sides; the
   prompt pushes the model to answer after one search.
2. **The `BUG-064-002` value-extraction shaping.** The prompt's
   "EXTRACT-THEN-SYNTHESIZE … (times, prices, temperatures, highs/lows, a
   schedule, a table)" enumeration biases the model toward a fixed set of
   data-point question shapes and away from open reasoning.
3. **A snippet-salvage cascade that lies.** When the model fails to truly
   synthesize, the platform stitches deduped snippets and presents them as a
   confident answer. The contradictory-snippet pomegranate body is exactly
   this — a snippet wall wearing the costume of a reasoned answer.

The owner wants the agent to reason over ANY question type while preserving
every spec-064 trust invariant: capture-as-fallback is never lost, the
cite-back contract still hash-matches every citation to a tool result, the
provenance gate still refuses zero-source responses, and the model is never
trusted to attest its own grounding.

---

## 2. Actors & Personas

| Actor | Description | Goals | Permissions |
|-------|-------------|-------|-------------|
| **Human user (chat owner)** | Single operator on any spec 061 transport (Telegram / web `/ask`). | Ask open-domain questions of ANY shape (comparison, why, recommend, trade-off, multi-hop) and get the ACTUAL answer or an honest "couldn't answer, here's what I found". | All spec 061 transport permissions; per-user monthly budget cap. |
| **Open-Knowledge Agent Loop** (amended) | The spec 064 bounded planner ↔ tool ↔ observation loop in `internal/assistant/openknowledge/agent/agent.go::Run`. | Decompose the question, gather evidence for every sub-question / side with distinct tool calls, reconcile contradictions, synthesize the actual answer. Salvage honestly when synthesis fails. | Calls allowlisted tools via the Registry; bounded by SST iteration / token / USD budgets and the HTTP request deadline. |
| **Reasoning Prompt** (amended) | The `agent_system_prompt` block in `config/prompt_contracts/open_knowledge.yaml`. | Drive question-agnostic decompose → gather-all-sides → reconcile → answer behavior. Preserve the `<CITATIONS>` contract, R1-R4, refusal shape verbatim. | Loaded into `okagent.Config.SystemPrompt` by `cmd/core` wiring. |
| **Cite-Back Verifier / Provenance Gate** (unchanged) | The spec 064 mechanical, non-LLM trust mechanism. | Reject any citation that does not hash-match a recorded tool result; refuse zero-source responses. | Pure function over the per-turn tool trace. **Preserved verbatim.** |
| **Operator** | Owns SST config + budgets. | Tune `max_iterations`, `per_query_token_budget`; understand the latency envelope. | Edits `config/smackerel.yaml` `assistant.open_knowledge.*`. |

---

## 3. Outcome Contract

**Intent:** A user can ask the Smackerel open-knowledge assistant a question
of ANY shape and receive the actual answer (the comparison verdict, the
causal explanation, the recommendation, the specific value) synthesized from
distinct, reconciled tool evidence — OR an honest "I searched and could not
directly answer; here is what I found" — never a snippet wall dressed up as a
confident verdict, and never a fabricated citation.

**Success Signal:**
- The reasoning prompt is **question-agnostic**: it contains NO enumerated
  question-type list and NO anti-drill "answer after the first tool call"
  bias; it DOES contain a decompose → gather-all-sides → reconcile →
  answer-the-actual-question contract. The `<CITATIONS>` contract, the three
  citation shapes, R1-R4, the honest-refusal shape, and the
  "never repeat / never invent" rules are preserved verbatim.
- The loop is **allowed to drill in**: `max_iterations` is raised to 6, the
  forced-final-synthesis turn is preserved, and a lightweight
  reflect-before-final nudge is injected on the second-to-last iteration so
  the model checks coverage and fills gaps before synthesizing.
- Salvage is **honest**: when genuine synthesis did not happen and the
  platform falls back to stitched snippets, the user-visible body is framed
  as raw findings ("I searched but couldn't directly answer your question;
  here is the most relevant information I found:") — never presented as a
  reasoned verdict. The deduped/capped sources still attach (not a
  zero-source refusal); the provenance/cite-back contracts still hold.
- The **happy path is unchanged**: a genuine cited synthesis is returned
  verbatim with no honest-salvage frame.
- **No model change**: `llm_model_id` and the hardware-tier matrix are
  untouched (gemma4:26b self-hosted, gemma3:4b dev).
- The latency envelope is **understood and bounded**: the `/ask` fast-path is
  bounded by the HTTP `WriteTimeout`, which is kept consistent with
  `max_iterations × llm_timeout_ms`.

**Failure / Refusal contract (unchanged from spec 064):**
- Zero-source responses → canonical refusal-with-capture (provenance gate).
- Fabricated citations → canonical refusal (cite-back verifier).
- Budget / iteration / tool caps → typed refusal-with-capture.
- Capture-as-fallback is performed by the Facade unconditionally regardless of
  the turn result (inviolable).

---

## 4. Behavioral Scenarios (Gherkin)

### SCN-084-A01 — Question-agnostic reasoning prompt
```gherkin
Feature: Question-agnostic reasoning guidance
  Scenario: The agent prompt no longer biases against multi-hop or to a fixed question shape
    Given the open-knowledge agent system prompt in config/prompt_contracts/open_knowledge.yaml
    When the prompt is loaded
    Then it MUST NOT contain the anti-drill instruction to write the final answer
         in the next turn after the first successful tool call
    And it MUST NOT contain the BUG-064-002 question-type enumeration
         (times, prices, temperatures, highs/lows, a schedule, a table)
    And it MUST contain a decompose / gather-all-sides / reconcile /
         answer-the-actual-question reasoning contract
    And it MUST preserve verbatim the <CITATIONS> contract, the three citation
         shapes, the R1-R4 hard rules, and the honest-refusal shape
```

### SCN-084-A02 — Loop drills in (multi-hop + reflect-before-final)
```gherkin
Feature: The loop is allowed to gather all sides before answering
  Scenario: Multi-hop turn issues distinct tool calls before the forced final
    Given max_iterations is 6 and the model issues distinct tool calls each turn
    When the agent loop runs
    Then the loop allows >= 2 distinct tool calls before the forced-final turn
    And a reflect-before-final nudge is injected on the second-to-last iteration
         instructing the model to check coverage and fill any gap with one more
         targeted call before synthesizing
    And the forced-final-turn tool-stripping mechanism is preserved
```

### SCN-084-A03 — Comparison salvage is honest
```gherkin
Feature: A failed comparison synthesis is framed honestly
  Scenario: Two distinct tool calls, synthesis fails, salvage fires
    Given the model gathered evidence on side X and side Y via DISTINCT tool calls
    And the model produced no usable synthesis on the forced-final turn
    When the agent salvages from snippets
    Then the body is framed as raw findings ("I searched but couldn't directly answer")
    And the body carries BOTH sides' evidence
    And the body is NOT presented as a confident comparison verdict
    And the capped, deduped sources still attach
```

### SCN-084-A04 — Honest salvage on empty / ungrounded forced-final
```gherkin
Feature: Salvage never presents a snippet wall as a confident answer
  Scenario: Forced-final empty text -> snippet salvage
    Given tools returned content but the model returned empty/ungrounded text on the forced final
    When the agent salvages
    Then the salvaged body is framed as raw findings, not a confident answer
    And the salvaged body still carries capped, deduped sources (not a zero-source refusal)
    And the existing provenance / cite-back contracts still hold
```

### SCN-084-A05 — Trust contracts preserved (negative / guard)
```gherkin
Feature: Genuine synthesis and the cite-back trust contract are not regressed
  Scenario: Genuine cited synthesis is returned verbatim
    Given the model produced a real synthesized answer with a valid <CITATIONS> block
    When the agent finalizes
    Then the body is returned verbatim with no honest-salvage frame
  Scenario: Fabricated citation is still rejected
    Given the model emitted a citation that does not hash-match any tool result
    When the cite-back verifier runs in enforce mode
    Then the answer is replaced with the canonical refusal
```

---

## 5. Requirements

| # | Requirement | Type |
|---|-------------|------|
| FR-1 | The `agent_system_prompt` MUST remove the anti-drill "answer after the first tool call" bias. | Functional |
| FR-2 | The `agent_system_prompt` MUST remove the BUG-064-002 question-type enumeration and NOT replace it with a different question-type list (it MUST be general). | Functional |
| FR-3 | The `agent_system_prompt` MUST instruct: decompose into sub-questions; gather evidence for each sub-question / each side with distinct targeted tool calls (BOTH/ALL sides before answering a comparison); reconcile contradictions explicitly; answer the ACTUAL question synthesized in the agent's own words. | Functional |
| FR-4 | The `agent_system_prompt` MUST preserve verbatim: the `<CITATIONS>` contract block, the three citation shapes, the R1-R4 hard rules, the honest-refusal shape, and the "never repeat the same block / never invent values or URLs" rules. | Functional / Trust |
| FR-5 | `assistant.open_knowledge.max_iterations` MUST be raised from 4 to 6 (SST, fail-loud, `> 0` when enabled, no default). The forced-final-synthesis turn MUST be preserved. | Functional / Config |
| FR-6 | A lightweight reflect-before-final nudge MUST be injected on the second-to-last iteration (within the existing iteration budget; no new model, no new external dependency). | Functional |
| FR-7 | `assistant.open_knowledge.per_query_token_budget` MUST be sized so it does not bind before a legitimate 6-iteration turn completes (re-added snippets each turn → ~quadratic growth). Raised to 128000 (SST, fail-loud, `> 0`). | Config / Non-functional |
| FR-8 | The latency ceiling MUST be analyzed and kept correct for 6 iterations. The `/ask` fast-path (`runOpenKnowledgeDirect`) is bounded by the HTTP `WriteTimeout`, sized as `max_iterations × llm_timeout_ms`; `WriteTimeout` MUST be kept consistent (raised to `6 × 600s = 3600s`). | Non-functional |
| FR-9 | When genuine synthesis did not happen and the platform salvages from snippets, the user-visible body MUST be framed as raw findings ("couldn't directly answer; here is what I found"), NOT a confident verdict. Capped/deduped sources still attach; provenance/cite-back contracts still hold. | Functional / Trust (Principle 8) |
| FR-10 | The genuine-synthesis happy path MUST be unchanged: a real cited synthesis is returned verbatim with no honest-salvage frame. | Functional |
| C-1 | `llm_model_id` and the hardware-tier model matrix MUST NOT change. No deepseek-r1. | Constraint |
| C-2 | Every changed/new config value MUST be SST + fail-loud (no `${VAR:-default}`). | Constraint (smackerel-no-defaults) |
| C-3 | NO files belonging to the spec-083 card-rewards WIP may be touched (see `state.json.ownerDirective.doNotTouch`). | Constraint |
| C-4 | NO commit / push in this delivery. Leave changes in the working tree; report the exact file manifest for the downstream devops dispatch. | Constraint |

---

## 6. Product Principle Alignment

- **Principle 2 (Vague In, Precise Out).** A vague open-ended question
  ("which town is better for pomegranates?") must yield a precise, reasoned
  answer — the comparison verdict — not a per-link recap. The reasoning
  contract directly serves this.
- **Principle 4 (Source-Qualified Processing).** Reconciling contradictory
  sources and answering with cited evidence (not pasting raw snippets) keeps
  the answer source-qualified. The cite-back / provenance contracts are
  preserved.
- **Principle 8 (Trust Through Transparency).** The honest-salvage change is
  a direct Principle-8 fix: the contradictory-snippet pomegranate body
  actively misled the user by wearing the costume of a reasoned answer.
  Framing salvage as raw findings restores honesty.

---

## 7. Non-Goals

- Changing the model, the model matrix, or wiring deepseek-r1.
- Adding a new tool, a new external dependency, or a new model call.
- Reopening `BUG-064-001` / `BUG-064-002`.
- Touching spec-083 card-rewards files.
- Performing the self-hosted deploy or the live re-verification (separate
  downstream `bubbles.devops` dispatch).
