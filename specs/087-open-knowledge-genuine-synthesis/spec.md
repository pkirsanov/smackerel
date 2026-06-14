# Spec 087 — Open-Knowledge Genuine Synthesis

**Status:** in_progress (planning bootstrap; ceiling = `done`)
**Workflow Mode:** `full-delivery` (prelude: `analyze-design-plan`)
**Execution Model:** `parent-expanded` — `bubbles.workflow` ran the
analyze → ux → design → plan → implement → test → … → chaos phaseOrder
directly because no specialist sub-agent dispatch (`runSubagent`) is
available in this runtime. `full-delivery` is not a
`requiresTopLevelRuntime` mode, so parent-expansion is permitted
(same precedent as spec 084 `state.json` `executionModel`).

**Owner Directive (2026-06-13):** The open-knowledge `/ask` agent
answers open-ended questions with a search-results wall, not a reasoned
answer. Make it run a GENUINE reasoning loop that DECOMPOSES,
GATHERS evidence for each part, RECONCILES contradictions, and
SYNTHESIZES the actual verdict — falling back to honest salvage only
when synthesis genuinely is not possible.

**Depends On:** spec 064 (open-ended knowledge agent — the loop, the
cite-back verifier, the provenance gate, the SST config block) and
spec 084 (the reasoning-prompt rewrite, `max_iterations` 6, token
budget 128000, reflect-before-final nudge, honest salvage).
**Amends:** spec 064 and spec 084. This spec does NOT reopen the closed
`BUG-064-001` / `BUG-064-002`; it builds on top of the spec-084
baseline and changes the **synthesis-turn model, the forced-final
synthesis prompt, and the retry-before-salvage behavior** on top of it.
**Unblocked by:** spec 084 (which stated its purpose was to "empirically
test whether gemma4:26b reasons well once it is no longer in a
straitjacket" and explicitly unblocked "a future model-comparison
spec"). This IS that spec — the empirical result is in: with the
straitjacket removed, gemma4:26b STILL fails to synthesize.

**Out of scope (explicit):** the spec-083 card-rewards WIP
(`internal/cardrewards/`, `ml/app/card_categories.py`,
`ml/app/main.py`, `ml/tests/test_card_categories.py`,
`specs/083-card-rewards-companion/`,
`tests/integration/cardrewards_extract_test.go`); the home-lab deploy
itself (a separate `bubbles.devops` dispatch); a full deterministic
decompose→gather→reconcile→synthesize re-architecture (Axis B —
considered, designed at a high level, and DEFERRED; see design.md).

---

## 1. Problem Statement

Spec 084 removed the prompt/loop straitjacket: it stripped the
anti-drill bias and the `BUG-064-002` question-type enumeration, added a
DECOMPOSE/GATHER/RECONCILE/ANSWER contract to
`config/prompt_contracts/open_knowledge.yaml`, raised `max_iterations`
4→6 and `per_query_token_budget` 64000→128000, added a
reflect-before-final nudge, and made the snippet salvage honest. Spec
084 shipped and is live (commit `ae0d540c`).

Spec 084's stated hypothesis was that gemma4:26b would reason well once
unshackled. **That hypothesis is now empirically DISPROVEN.** Live on
home-lab (gemma4:26b, 2026-06-13):

> User: `/ask what is better place to grow pomegranate wa-town-A or
> wa-town-B, wa?`
> Bot: "I searched but couldn't directly answer your question. Here is
> the most relevant information I found:" followed by one paragraph per
> source, with contradictory snippets ("thrives down to 0 degrees" vs
> "cannot stand freezing") presented side-by-side unreconciled.

The honest-salvage frame the user sees is exactly the spec-084
`honestSalvagePrefix` (`internal/assistant/openknowledge/agent/agent.go`).
So spec 084 IS deployed; the screenshot is post-084 behavior. The
straitjacket is gone, and the model still produces a snippet wall.

**Exact failure mechanism** (verified in
`internal/assistant/openknowledge/agent/agent.go::Run`, the
`llm.StopEndTurn` branch on the forced-final turn `iter ==
MaxIterations-1`, tools stripped): the model either (a) returns empty
text → `honestSalvageBody(trace)` snippet salvage fires; or (b) emits
`<CITATIONS>[]</CITATIONS>` with an ungrounded-excuse body
(`isUngroundedExcuse`) → snippet salvage fires. Either way the user
gets a deduped/capped snippet wall, now honestly framed. **Genuine
cited synthesis never happens for this question class.**

The root cause spec 084 could not fix with prompt changes alone is
**raw synthesis capability on the forced-final turn**: gemma4:26b is a
strong tool-caller but a weak synthesizer/reasoner for a
reconcile-and-decide task. Removing the straitjacket let it gather
better; it did not make it reason better.

---

## 2. Actors & Personas

| Actor | Description | Goals | Permissions |
|-------|-------------|-------|-------------|
| **Human user (chat owner)** | Single operator on any spec 061 transport (Telegram / web `/ask`). | Ask an open-ended comparison/why/recommendation question and get the ACTUAL synthesized verdict — or an honest "couldn't directly answer; here's what I found". | All spec 061 transport permissions; per-user monthly budget cap. |
| **Open-Knowledge Agent Loop** (amended) | The spec 064/084 bounded planner ↔ tool ↔ observation loop in `agent.go::Run`. | Gather evidence with the tool-calling model, then SYNTHESIZE the actual answer on the forced-final turn with a reasoning model, retrying once with a stronger prompt before falling to honest salvage. | Calls allowlisted tools via the Registry; bounded by SST iteration / token / USD budgets and the HTTP request deadline. |
| **Synthesis Model** (new role) | The reasoning model used ONLY on the tools-stripped forced-final synthesis turn (and its bounded retry). deepseek-r1:7b on home-lab; equals the tool-calling model on dev. | Reason over the gathered, structured evidence and write the comparison verdict / causal explanation / recommendation. Emits `<think>` chain-of-thought that MUST be stripped before citation parsing. | Read-only over the per-turn evidence; no tool access (tools are already stripped on this turn). |
| **Cite-Back Verifier / Provenance Gate** (unchanged) | The spec 064 mechanical, non-LLM trust mechanism. | Reject any citation that does not hash-match a recorded tool result; refuse zero-source responses. | Pure function over the per-turn tool trace. **Preserved verbatim. Runs on the post-`<think>`-strip text.** |
| **Operator** | Owns SST config + budgets + the hardware-tier model matrix. | Choose the synthesis model per tier; understand the latency envelope. | Edits `config/smackerel.yaml` `assistant.open_knowledge.*` + the per-environment override layer. |

---

## 3. Outcome Contract

**Intent:** For an answerable open-ended question, the open-knowledge
assistant gathers evidence on every part/side and returns the actual
synthesized verdict — reconciled from cited evidence — drawn on the
forced-final turn by a reasoning model, with one bounded
stronger-prompt retry before the honest snippet salvage. Honest salvage
remains the genuine-failure fallback, not the default outcome.

**Success Signal:**
- **Split synthesis model.** A new SST key
  `assistant.open_knowledge.synthesis_model_id` selects the model used
  on the tools-stripped forced-final synthesis turn (and its retry).
  The tool-calling turns keep using `llm_model_id`. Dev default =
  `gemma3:4b` (== `llm_model_id`, no effective split, envelope-safe);
  home-lab override = `deepseek-r1:7b` (a reasoning model already in
  the ollama envelope via `OLLAMA_REASONING_MODEL`).
- **Reasoning-model `<think>` handling.** The synthesis turn output is
  stripped of any leading `<think>...</think>` chain-of-thought BEFORE
  citation parsing and cite-back verification. The `<think>` content
  NEVER appears in the user-visible body and NEVER bypasses cite-back.
- **Structured forced-final synthesis.** The forced-final prompt lays
  out the gathered evidence and instructs the model to write the actual
  verdict now (not a per-source recap).
- **Retry-before-salvage.** A new SST key
  `assistant.open_knowledge.synthesis_retry_budget` (shipped value `1`)
  governs how many times an empty/ungrounded forced-final triggers a
  re-issued synthesis turn with an even stronger "you already analyzed
  this; WRITE the verdict now" prompt BEFORE the honest snippet salvage
  fires. Budget `0` = no retry (preserves the exact spec-084 salvage
  timing).
- **Genuine synthesis is returned verbatim.** When the synthesis turn
  produces a real cited answer, it is returned with no honest-salvage
  frame (the happy path).
- **Honest salvage preserved.** When synthesis genuinely fails (retry
  budget exhausted), the user-visible body is still framed as raw
  findings via the spec-084 `honestSalvagePrefix`; the deduped/capped
  sources still attach; the provenance/cite-back contracts still hold.
- **Latency envelope is honest.** The `/ask` fast-path
  (`facade.go::runOpenKnowledgeDirect`) is bounded by the HTTP
  `WriteTimeout`, updated to `(max_iterations + synthesis_retry_budget)
  × llm_timeout_ms` = `(6 + 1) × 600s` = `4200s`. The synthesis
  model's per-turn time is bounded by the same `llm_timeout_ms`.
- **Model matrix consistency preserved.** `synthesis_model_id` is SST
  and fail-loud; the selected models already have
  `model_memory_profiles` entries; the home-lab `deepseek-r1:7b`
  co-resides with `gemma4:26b` (23296 MiB) within the 28672 MiB
  envelope with margin and is an on-demand specialist (not in the
  concurrent interactive working-set guard).

**Failure / Refusal contract (unchanged from spec 064 / 084):**
- Zero-source responses → canonical refusal-with-capture (provenance gate).
- Fabricated citations → canonical refusal (cite-back verifier), now
  applied to the post-`<think>`-strip synthesis text.
- Budget / iteration / tool caps → typed refusal-with-capture.
- Capture-as-fallback is performed by the Facade unconditionally
  regardless of the turn result (inviolable).

---

## 4. Behavioral Scenarios (Gherkin)

### SCN-087-A01 — Reasoning-model `<think>` stripped, genuine verdict returned
```gherkin
Feature: The synthesis turn handles a reasoning model's <think> chain-of-thought
  Scenario: deepseek-r1-style forced-final emits <think> then a cited verdict
    Given the forced-final synthesis turn uses the synthesis model
    And the model returns "<think>weighing both climates</think>" followed by a
        synthesized comparison verdict and a valid <CITATIONS> block
    When the agent finalizes the turn
    Then the <think>...</think> block is stripped before citation parsing
    And the user-visible body is the synthesized verdict with NO <think> text
    And the answer is returned verbatim with NO honest-salvage frame
    And the cite-back verifier ran on the post-strip text and accepted the citation
```

### SCN-087-A02 — Synthesis turn uses the synthesis model, tool turns use the tool-calling model
```gherkin
Feature: The forced-final turn swaps to the synthesis model
  Scenario: A multi-iteration turn routes models per role
    Given synthesis_model_id differs from llm_model_id
    When the agent loop runs to the forced-final turn
    Then every tool-calling request used the tool-calling model (llm_model_id)
    And the forced-final synthesis request used the synthesis model
    And the forced-final request had its tools stripped
```

### SCN-087-A03 — Two-distinct-tool-call comparison yields a real cited verdict (not salvage)
```gherkin
Feature: A comparison question synthesizes the actual verdict
  Scenario: Side X and side Y gathered, synthesis succeeds
    Given the model gathered evidence on side X and side Y via DISTINCT tool calls
    And the forced-final synthesis turn produced a real cited verdict
    When the agent finalizes
    Then the body is the synthesized comparison verdict returned verbatim
    And the body is NOT the honest-salvage snippet wall
    And the verified citations are attached (not the raw trace-source cap)
```

### SCN-087-A04 — Retry-before-salvage rescues an empty/ungrounded forced-final
```gherkin
Feature: An empty/ungrounded forced-final is retried with a stronger prompt before salvage
  Scenario: First synthesis attempt blank, retry produces the verdict
    Given synthesis_retry_budget is 1
    And the first forced-final synthesis attempt returned empty/ungrounded text
    When the agent issues the synthesis retry
    Then the retry request carried a stronger "write the verdict now" instruction
    And the retry produced a real cited verdict
    And that verdict is returned verbatim with NO honest-salvage frame
    And the honest snippet salvage did NOT fire
```

### SCN-087-A05 — Trust contracts preserved (negative / guard)
```gherkin
Feature: The cite-back trust contract and honest fallback are not regressed
  Scenario: Fabricated citation in the synthesis output is still rejected
    Given the synthesis turn emitted a citation that does not hash-match any tool result
    When the cite-back verifier runs in enforce mode on the post-<think>-strip text
    Then the answer is replaced with the canonical refusal
  Scenario: Retry budget exhausted falls back to honest salvage
    Given synthesis_retry_budget retries were all empty/ungrounded
    When the agent salvages
    Then the body is framed with the honest "couldn't directly answer" prefix
    And the capped, deduped sources still attach (not a zero-source refusal)
  Scenario: <think> content never leaks and never bypasses cite-back
    Given the synthesis turn emitted a <think> block containing a fabricated URL
    When the agent finalizes
    Then the fabricated URL from the <think> block is NOT in the user body
    And it is NOT treated as a citation
```

---

## 5. Functional Requirements

- **FR-1.** A new SST key `assistant.open_knowledge.synthesis_model_id`
  (string) selects the model for the forced-final synthesis turn and
  its retries. REQUIRED non-empty when `enabled=true` (G028 fail-loud).
- **FR-2.** A new SST key
  `assistant.open_knowledge.synthesis_retry_budget` (int, shipped `1`)
  governs the number of stronger-prompt synthesis retries before
  honest salvage. REQUIRED `>= 0` when `enabled=true`.
- **FR-3.** The forced-final synthesis turn (`iter ==
  MaxIterations-1`, tools stripped) issues its LLM request with
  `Model = synthesis_model_id` and a structured "write the verdict
  now" prompt.
- **FR-4.** The synthesis-turn output has any leading
  `<think>...</think>` block stripped BEFORE `parseCitations` and
  before cite-back; the stripped text never reaches the user body and
  never participates in citation parsing.
- **FR-5.** When the (post-strip) forced-final text is empty OR an
  ungrounded excuse with zero verified citations, AND
  `synthesis_retry_budget > 0`, the agent re-issues the synthesis turn
  (same synthesis model) with a stronger prompt, up to
  `synthesis_retry_budget` times, before honest salvage.
- **FR-6.** A genuine cited synthesis on the forced-final turn (or a
  retry) is returned verbatim with no honest-salvage frame; the
  verified citations are attached.
- **FR-7.** When all synthesis attempts fail, the spec-084 honest
  salvage fires unchanged (honest prefix + capped deduped sources).
- **FR-8.** The cite-back verifier and provenance gate are preserved
  verbatim and run on the post-`<think>`-strip text; fabricated
  citations and zero-source responses are still refused.
- **FR-9.** Capture-as-fallback (the Facade) is untouched and remains
  inviolable.
- **FR-10.** The HTTP `WriteTimeout` is updated to `(max_iterations +
  synthesis_retry_budget) × llm_timeout_ms` and documented; the
  worst-case latency envelope stays honest.
- **FR-11.** All new/changed config values originate from
  `config/smackerel.yaml`, are REQUIRED and fail-loud (no
  `${VAR:-default}`), flow through `scripts/commands/config.sh` into
  the generated env, and are loaded + validated in
  `internal/config/openknowledge.go`.
- **FR-12.** The home-lab synthesis model is set via the per-environment
  override layer (`environments.<env>.assistant_open_knowledge_synthesis_model_id`),
  mirroring the existing `assistant_open_knowledge_llm_model_id`
  override; the dev default is the tool-calling model (no split).

---

## 6. Product Principle Alignment

This feature is governed by `docs/Product-Principles.md` (binding since
2026-06-03) and `.github/instructions/product-principles.instructions.md`.

- **Principle 2 — Vague In, Precise Out.** A vague comparison question
  ("which is better for pomegranates, wa-town-A or wa-town-B?")
  MUST yield a precise verdict, not a search-results wall. This spec's
  whole purpose is to make the precise synthesized verdict the default
  outcome for answerable questions. **Implements.**
- **Principle 4 — Source-Qualified Processing.** Cited evidence is
  RECONCILED (contradictions resolved or caveated) and synthesized, not
  pasted side-by-side as raw snippets. The reasoning model on the
  forced-final turn performs the reconcile-and-decide step. **Implements.**
- **Principle 8 — Trust Through Transparency.** A snippet wall is never
  dressed as a verdict: genuine synthesis is returned verbatim; honest
  salvage keeps its transparent "couldn't directly answer" frame; the
  reasoning model's `<think>` scratchpad is stripped and can never leak
  ungrounded content or a fabricated citation into the body. **Implements.**

No principle deviation. No financial-action surface is touched
(Principle 10 N/A). No new artifact type or lifecycle change
(Principles 3/5 N/A). No capture-time tagging is introduced
(Principle 1 N/A). No new notification (Principle 6 N/A).

---

## 7. Release Train

This spec amends an existing capability (the spec 064/084 open-knowledge
agent) and introduces **no new feature flag** — the synthesis
improvement is unconditional, exactly like spec 084's loop/prompt
changes (which also introduced no flag). The per-tier behavior
difference (dev vs home-lab synthesis model) is governed by the
existing SST per-environment override layer, not by a release-train
flag. Targets the current trunk; no train-scoped flag bundle change.

---

## 8. Validation Reality

The decisive proof — does the model now synthesize a real verdict for
the pomegranate question? — is model+GPU-dependent and runs on the live
home-lab stack (dev's `gemma3:4b`/`qwen` cannot synthesize either, and
the dev sandbox has no Ollama daemon). In-repo work is:
prompt/scaffold/model-wiring/config edits + adversarial tests using
scripted fake-LLM traces that prove the mechanism fires correctly:
- a fake deepseek-r1 trace with a `<think>` block + a genuine cited
  synthesis proving the verdict path fires and `<think>` is stripped;
- a two-distinct-tool-call comparison that now produces a real cited
  verdict instead of salvage;
- a retry-before-salvage trace (empty forced-final → stronger-prompt
  retry → cited verdict);
- trust-contract guards (fabricated citation still refused, zero-source
  still refused via the unchanged provenance gate, capture-as-fallback
  still inviolable, `<think>` content never leaks / never cited).

Adversarial tests are RED-before / GREEN-after and include cases that
fail if the bug regressed (no tautological tests, no bailout
early-returns).

**Terminal posture:** in-repo work is implemented + validated;
the home-lab live re-verify of the pomegranate turn is a separate
`bubbles.devops` dispatch (build + signed images + apply + operator
live re-verify). This spec terminates at validated-in-repo with
`nextRequiredOwner: bubbles.devops` if the `done` ceiling's live-stack
gates are not satisfiable in-session — following the spec-084
precedent. No live-stack result is fabricated.
