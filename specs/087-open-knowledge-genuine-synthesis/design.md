# Design 087 — Open-Knowledge Genuine Synthesis

> Design authored by `bubbles.workflow (parent-expanded)` during the
> `analyze-design-plan` prelude. Solution-blind current-truth gathered
> first (below), then the decision record, then the implementation
> design. Amends spec 064 + spec 084.

---

## Current Truth (gathered 2026-06-13, solution-blind)

Verified directly against the committed code (not re-derived from the
owner's diagnosis — confirmed it).

### CT-1 — The `/ask` fast-path and the agent loop
- Router selects `open_knowledge` as the fallback scenario →
  `internal/assistant/facade.go::runOpenKnowledgeDirect` (line ~1190)
  invokes `okagenttool.CurrentAgent().Run(ctx, prompt)` directly with
  the HTTP request context, bypassing the substrate executor and its
  `timeout_ms`/`per_tool_timeout_ms`. The real ceiling is the HTTP
  server `WriteTimeout`.
- The agent loop is `internal/assistant/openknowledge/agent/agent.go::Run`.
  It iterates `for iter := 0; iter < cfg.MaxIterations; iter++`. Each
  request carries `Model: a.cfg.Model` for EVERY turn.

### CT-2 — The forced-final synthesis turn and the salvage cascade
- On `iter == MaxIterations-1` with a non-empty trace, the loop strips
  tools (`requestTools = nil`) and appends a forced-final user message
  ("You have used all your tool calls. … write your final answer NOW.
  Include the `<CITATIONS>` block …").
- On `iter == MaxIterations-2` it appends the spec-084
  reflect-before-final nudge (kept; tool-calling turn).
- In the `llm.StopEndTurn` branch:
  - If `isForcedFinalTurn && TrimSpace(FinalText) == ""` →
    `honestSalvageBody(trace)` (the snippet wall with the honest prefix).
  - `parseCitations(FinalText)` — requires a trailing
    `<CITATIONS>[...]</CITATIONS>`; a missing block on the forced-final
    with trace sources is salvaged as the trimmed text + capped sources.
  - `a.verify(citations, …)` → `citeback.Decide`; `decision.Refuse` →
    canonical refusal (fabricated source).
  - If zero verified citations but non-empty text + trace sources →
    empty-citations salvage; if `isUngroundedExcuse(finalText)` the body
    is replaced by `honestSalvageBody(trace)`.
  - Else the genuine cited synthesis is returned verbatim
    (`verdict.Verified` sources).
- `honestSalvagePrefix` = exactly the user's screenshot text:
  "I searched but couldn't directly answer your question. Here is the
  most relevant information I found:".

### CT-3 — The trust perimeter (spec 064, preserved)
- `parseCitations` (regex `<CITATIONS>...</CITATIONS>` + JSON decode
  with `DisallowUnknownFields`) strips the block from the body.
- `citeback.Verify` hash-matches every citation to a recorded tool
  result; `enforcement=enforce` flips a mismatch to refusal.
- The provenance gate (refuse zero-source) lives downstream of the
  agent (facade/assembler) — the agent never returns a sourced answer
  the gate would have to reject.
- Capture-as-fallback is the Facade's job (SCOPE-13), unconditional,
  inviolable — the agent does NOT perform it.

### CT-4 — The config / wiring / model surface
- `config/smackerel.yaml` `assistant.open_knowledge.*`: `llm_model_id`
  (`gemma3:4b` dev), `max_iterations: 6`, `per_query_token_budget:
  128000`, `llm_timeout_ms: 600000`, … each REQUIRED + fail-loud.
- Home-lab override: `environments.<env>.assistant_open_knowledge_llm_model_id:
  "gemma4:26b"` (config.sh resolves the env override, else the base key).
- `internal/config/openknowledge.go`: `OpenKnowledgeConfig` struct +
  `LoadOpenKnowledge` (every `ASSISTANT_OPEN_KNOWLEDGE_*` env REQUIRED
  present; deep `Validate()` gated on `Enabled`).
- `cmd/core/wiring_assistant_openknowledge.go`: builds `okagent.Config`
  with `Model: okCfg.LLMModelID`.
- `cmd/core/main.go`: `WriteTimeout: 3600 * time.Second` (= 6 × 600s,
  spec-084 value), commented as the real `/ask` ceiling.

### CT-5 — The model envelope guard (de-risks the model choice)
- `internal/config/config.go::validateModelEnvelopes`:
  - **Per-model check** — each referenced model's
    `model_memory_profiles` MiB must fit `OLLAMA_MEMORY_LIMIT`. The ref
    list does NOT include the open-knowledge model; deepseek-r1:7b IS
    referenced via `OLLAMA_REASONING_MODEL` (4864 MiB, already
    validated against the 28672 MiB home-lab envelope).
  - **Concurrent interactive working-set check** — sums ONLY
    `LLM_MODEL/OLLAMA_MODEL/OLLAMA_VISION_MODEL/AGENT_PROVIDER_DEFAULT/FAST/VISION`
    when keep-alive is resident. The open-knowledge model and the
    synthesis model are on-demand specialists, explicitly NOT summed.
- **Consequence:** adding `synthesis_model_id = deepseek-r1:7b` on
  home-lab does NOT trip either check. No `ollama_memory_limit` change
  is required. `model_memory_profiles` already has `deepseek-r1:7b`
  (4864), `deepseek-r1:32b` (22528), `gemma3:4b` (4096), `gemma4:26b`
  (18432). Co-residence of gemma4:26b + deepseek-r1:7b = 23296 MiB ≤
  28672 MiB envelope, with margin.

---

## Decision Record — the three solution axes

The owner asked the design to investigate three axes, pick the smallest
combination that empirically produces a real synthesized verdict for
the pomegranate-class question, recommend a default, and record the
others as considered-and-why-not. Spec 084 already empirically proved
that removing the prompt straitjacket is **not** enough — gemma4:26b
still fails to synthesize. So the lever must be capability and/or
scaffolding, not prompt liberalization.

### D1 (RECOMMENDED DEFAULT) — Axis A (split reasoning model on synthesis) + Axis C (structured forced-final + bounded retry)

**What:** Keep the strong tool-caller (gemma) for the GATHER turns.
Swap to a reasoning model (deepseek-r1:7b home-lab) ONLY on the
tools-stripped forced-final synthesis turn (and its retry). Give that
turn a structured "here is the gathered evidence — write the verdict
now" prompt, and retry once with an even stronger prompt if the first
attempt is empty/ungrounded, before falling to honest salvage.

**Why this is the smallest effective combination:**
- The forced-final turn ALREADY strips tools — it is a natural seam to
  swap models with zero change to the gather loop. deepseek-r1's weaker
  tool-calling does not matter on a turn that has no tools.
- Spec 084 proved gemma can't synthesize even unshackled; a *reasoning*
  model is a different capability axis (RL-trained chain-of-thought),
  which is exactly the reconcile-and-decide skill the task needs. This
  is the central bet of Axis A.
- The retry directly addresses deepseek-r1's known "emits a `<think>`
  block but no concluded answer" failure mode — re-prompt "you already
  analyzed this; WRITE the verdict now, no preamble".
- Axis C's "structured evidence + write-the-verdict-now" is a thin
  slice of Axis B's benefit applied to ONLY the synthesis turn, without
  re-architecting the loop.
- Envelope-safe (CT-5): deepseek-r1:7b is already profiled + validated
  and is on-demand; no `ollama_memory_limit` change.
- Trust-safe: the synthesis output is `<think>`-stripped then run
  through the unchanged cite-back + provenance gates.

**Cost / new surface:** two new SST keys (`synthesis_model_id`,
`synthesis_retry_budget`); a `<think>` strip; a per-environment
override; a `WriteTimeout` bump to `(6+1)×600s`. Bounded and testable
with fake-LLM traces.

### D2 (CONSIDERED, DEFERRED) — Axis B (deterministic decompose → per-sub-question gather → reconcile → synthesize orchestration)

**What:** Replace the model's self-orchestration with code-driven
stages: an explicit DECOMPOSE LLM call producing sub-questions, a
per-sub-question gather loop, an explicit RECONCILE step, and a final
SYNTHESIZE call — each its own prompt + budget accounting.

**Why deferred (not rejected):** it is the most robust to weak
self-orchestration and matches the owner's "find X, find Y, then
synthesize" mental model most literally — but it is the LARGEST change
(a re-architecture of `Run` into discrete stages, each with new failure
modes, new budget accounting, and new tests). The owner asked for the
SMALLEST combination first. D1 delivers the structured-evidence benefit
on the synthesis turn and the capability benefit via the reasoning
model, which is the cheaper bet. If D1 empirically still fails on
home-lab (the `bubbles.devops` live re-verify is the judge), Axis B is
the documented next lever — recorded as finding **F-AXISB** so it is
not lost.

### D3 (CONSIDERED, REJECTED AS THE DEFAULT) — Axis C alone (stronger forced-final + retry, same gemma model)

**Why not the default:** spec 084 already proved gemma4:26b fails to
synthesize this question class even with a question-agnostic prompt. A
stronger forced-final prompt + retry on the SAME model is a marginal
improvement on a model that has demonstrably hit its synthesis ceiling
for this task. Axis C is therefore RETAINED but as a *complement* to
the reasoning-model swap (the structured prompt + retry wrap the
synthesis turn), not as the standalone lever. On dev (where
`synthesis_model_id == llm_model_id == gemma3:4b`) the deployment
degenerates to Axis C alone — acceptable because dev is not the proof
environment and has no Ollama daemon.

### D4 — Axis A variant rejected: deepseek-r1:32b as the home-lab default

**Why not the default:** deepseek-r1:32b (22528 MiB) co-resident with
gemma4:26b (18432) = 40960 MiB ≫ 28672 envelope; it would require
raising `ollama_memory_limit` (an opt-up touching deploy resources).
deepseek-r1:7b fits with no envelope change and is still a reasoning
model. **deepseek-r1:32b is documented as the operator opt-up** (raise
`ollama_memory_limit` first) for maximum synthesis quality, recorded as
finding **F-OPTUP**.

---

## Implementation Design (file-by-file)

### CHANGE 1 — SST keys (`config/smackerel.yaml`)
Add under `assistant.open_knowledge`:
- `synthesis_model_id: "gemma3:4b"` — REQUIRED non-empty when enabled.
  Dev default equals `llm_model_id` (no split; envelope-safe; dev has
  no Ollama daemon). Documented as the forced-final synthesis-turn
  model.
- `synthesis_retry_budget: 1` — REQUIRED `>= 0` when enabled. Number of
  stronger-prompt synthesis retries before honest salvage. `0` =
  spec-084 timing.
Add to the home-lab `environments.<env>` override block (right after
`assistant_open_knowledge_llm_model_id: "gemma4:26b"`):
- `assistant_open_knowledge_synthesis_model_id: "deepseek-r1:7b"`.

### CHANGE 2 — config load + validate (`internal/config/openknowledge.go`)
- Add fields `SynthesisModelID string`, `SynthesisRetryBudget int`.
- Load `ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_MODEL_ID` (string),
  `ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_RETRY_BUDGET` (int).
- `Validate()` (enabled-gated): `synthesis_model_id` non-empty;
  `synthesis_retry_budget >= 0`.

### CHANGE 3 — config generation (`scripts/commands/config.sh`)
- Resolve `ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_MODEL_ID` via the
  per-environment override pattern (mirror `..._LLM_MODEL_ID`): try
  `environments.$TARGET_ENV.assistant_open_knowledge_synthesis_model_id`,
  else `assistant.open_knowledge.synthesis_model_id`.
- Resolve `ASSISTANT_OPEN_KNOWLEDGE_SYNTHESIS_RETRY_BUDGET` via
  `required_value assistant.open_knowledge.synthesis_retry_budget`.
- Emit both in the generated-env heredoc.

### CHANGE 4 — agent loop (`internal/assistant/openknowledge/agent/agent.go`)
- `Config`: add `SynthesisModel string`, `SynthesisRetryBudget int`.
- `New()`: validate `SynthesisModel != ""` (fail-loud, consistent with
  `Model`); `SynthesisRetryBudget >= 0`.
- In `Run`, refactor the forced-final synthesis into a helper that:
  1. issues the forced-final request with `Model = a.cfg.SynthesisModel`
     and the structured "write the verdict now" prompt;
  2. strips `<think>...</think>` from `result.FinalText` via a new
     `stripThinkBlocks` helper BEFORE any parsing;
  3. classifies the result: genuine cited synthesis → return verbatim;
     empty OR ungrounded-excuse-with-zero-citations → eligible for retry;
  4. retries up to `SynthesisRetryBudget` times with an escalated prompt
     ("you already analyzed this; output ONLY the verdict and the
     `<CITATIONS>` block now; no `<think>`, no preamble"), each retry
     also `<think>`-stripped;
  5. only after the budget is exhausted, fall to the existing
     `honestSalvageBody` salvage paths (unchanged).
- `stripThinkBlocks`: remove every `<think>...</think>` (dotall,
  non-greedy); for an UNCLOSED `<think>` with no `</think>`, drop from
  `<think>` to end (a model that only thought and never answered →
  empty → triggers retry/salvage). Mirrors `ml/app/processor.py`
  semantics. Trim surrounding whitespace.
- The cite-back verifier, provenance behavior, and the existing
  empty-citations / missing-CITATIONS salvage helpers are reused
  unchanged — they now operate on `<think>`-stripped text.

### CHANGE 5 — wiring (`cmd/core/wiring_assistant_openknowledge.go`)
- Thread `SynthesisModel: okCfg.SynthesisModelID`,
  `SynthesisRetryBudget: okCfg.SynthesisRetryBudget` into
  `okagent.Config{...}`; add both to the startup log line.

### CHANGE 6 — latency invariant (`cmd/core/main.go`)
- `WriteTimeout = (max_iterations + synthesis_retry_budget) ×
  llm_timeout_ms = (6 + 1) × 600s = 4200s`. Update value `3600 →
  4200` and the comment to name `synthesis_retry_budget`.

### CHANGE 7 — deploy contract (`deploy/contract.yaml`)
- Add `assistant.open_knowledge.synthesis_model_id` (string) and
  `assistant.open_knowledge.synthesis_retry_budget` (int, ">=0 when
  enabled") to the contract path list, beside `llm_model_id`.

### CHANGE 8 — docs (`docs/Operations.md`)
- Amend the open-knowledge section: split synthesis model, `<think>`
  stripping, retry-before-salvage, and the updated `WriteTimeout`
  latency invariant.

### CHANGE 9 — tests
- NEW `agent/synthesis_spec087_test.go`: SCN-087-A01..A05 (adversarial
  RED→GREEN + guards), driven by fake-LLM traces.
- UPDATE `agent/agent_test.go::baseCfg`: set `SynthesisModel:
  "test-model"`, `SynthesisRetryBudget: 0` so EVERY existing agent test
  (incl. spec-084 salvage tests) constructs a valid Config and
  preserves the exact spec-084 salvage timing (budget 0 = no retry).
- UPDATE the config full-env test maps that enumerate every
  `ASSISTANT_OPEN_KNOWLEDGE_*` key
  (`internal/config/openknowledge_test.go`,
  `internal/config/validate_test.go`,
  `internal/config/spec_076_foundation_test.go`) to include the two new
  keys; add new-key fail-loud coverage in `openknowledge_test.go`.

---

## Latency Invariant (F-LAT, carried from spec 084)

The `/ask` fast-path is bounded by `WriteTimeout`. Worst case adds the
synthesis retries:

```
WriteTimeout = (max_iterations + synthesis_retry_budget) × llm_timeout_ms
             = (6 + 1) × 600s = 4200s
```

The synthesis model runs on ONLY the final turn (+ retries) and is
bounded by the same `llm_timeout_ms` (600s), so the envelope stays
honest. Realistic home-lab (gemma4:26b gather + deepseek-r1:7b
synthesis, GPU-resident) turns complete in ~40–90s; the 4200s ceiling
is the pathological-slow-CPU backstop. If an operator later raises
`max_iterations` or `synthesis_retry_budget`, `WriteTimeout` MUST be
recomputed (documented in `main.go` + `docs/Operations.md`).

---

## Trust-Invariant Preservation Matrix

| Invariant | How preserved |
|-----------|---------------|
| Cite-back verifier (hash-match) | Unchanged; runs on post-`<think>`-strip text. New guard test: fabricated citation in synthesis output still refused. |
| Provenance gate (refuse zero-source) | Untouched (downstream of agent). Salvage still attaches capped sources. |
| Capture-as-fallback (Facade) | Untouched, inviolable. |
| No fabricated citations | cite-back enforces; `<think>` block stripped so a fabricated URL inside `<think>` can never become a citation or body text. Guard test SCN-087-A05. |
| `<think>` never leaks | `stripThinkBlocks` runs before parse + body assembly. Guard test asserts no `<think>` text in body. |
| Honest salvage (Principle 8) | Preserved; fires only after retry budget exhausted. spec-084 salvage tests pass unchanged (budget 0). |

---

## Findings

- **F-LAT** — `/ask` fast-path latency ceiling is `WriteTimeout`,
  updated to `(max_iterations + synthesis_retry_budget) ×
  llm_timeout_ms`. Documented in `main.go` + `docs/Operations.md`.
- **F-AXISB** — Axis B (deterministic decompose/gather/reconcile/
  synthesize orchestration) is the documented next lever if D1
  empirically fails on the home-lab live re-verify. Not implemented in
  this spec (smallest-combination directive).
- **F-OPTUP** — deepseek-r1:32b is the operator opt-up for maximum
  synthesis quality on home-lab; requires raising `ollama_memory_limit`
  first (gemma4:26b 18432 + deepseek-r1:32b 22528 = 40960 > 28672).
  deepseek-r1:7b is the envelope-safe default.
- **F-PROOF** — The decisive "does it synthesize the pomegranate
  verdict?" proof is model+GPU-dependent and runs on home-lab via a
  separate `bubbles.devops` dispatch. In-repo tests prove only that the
  synthesis/`<think>`/retry/trust mechanisms fire correctly.
- **F-ENV-083** — Whole-working-tree guards
  (`internal/scopesdriftguard` 285>270; `ml/app/main.py` default
  fallbacks; `tests/unit/clients` node/dart canary) fail SOLELY on the
  operator's uncommitted spec-083 card-rewards WIP + the spec-073
  container env. This spec touches none of those files; attribution is
  by file path, not "fixed" here (do-not-touch directive).
