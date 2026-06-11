# Design — Spec 084 (Open-Knowledge Reasoning Loop)

**Mode:** full-delivery · **Status ceiling:** `done` · **Amends:** spec 064

This design records the objective current truth gathered before any change,
the three changes to deliver (review items 1, 2, 4 — NOT model), and the
decisions + findings the analyze phase surfaced.

---

## Current Truth (gathered 2026-06-11, solution-blind)

The `/ask` open-knowledge request path on home-lab:

```
Telegram / web /ask
  -> internal/assistant/facade.go  (router + capability dispatch)
     -> scenarioID == "open_knowledge" AND okagenttool.CurrentAgent() != nil ?
        YES (home-lab, agent wired):
          -> facade.go::runOpenKnowledgeDirect(ctx, ...)        [FAST-PATH]
             -> okagent.Agent.Run(ctx, prompt)                  [the loop]
        NO (agent not wired / other scenarios):
          -> f.executor.Run(ctx, sc, env)                       [substrate path]
             -> open_knowledge_invoke tool (PerCallTimeoutMs=0)
```

Key files:
- Loop: `internal/assistant/openknowledge/agent/agent.go::Run`
- Prompt: `agent_system_prompt` in `config/prompt_contracts/open_knowledge.yaml`
  (loaded by `cmd/core/wiring_assistant_openknowledge.go`)
- SST: `assistant.open_knowledge.*` in `config/smackerel.yaml`
- Fast-path: `internal/assistant/facade.go::runOpenKnowledgeDirect`
- HTTP server: `cmd/core/main.go` (`WriteTimeout`)
- Trust: `internal/assistant/openknowledge/citeback` (cite-back verifier),
  the provenance gate (zero-source → canonical refusal).

### Finding C1 — the `/ask` path bypasses the substrate timeouts (latency)

The owner directive asked to "confirm `llm_timeout_ms` (600000) and the
substrate `timeout_ms` (120000 in the scenario `limits`) still bound it
correctly." The objective truth is **neither substrate limit bounds the
`/ask` reasoning path**:

- `facade.go::runOpenKnowledgeDirect` was added in spec 064 SCOPE-17 as a
  deliberate fast-path. Its comment: *"bypass the substrate executor … the
  substrate enforces a per-tool timeout … too tight for slow on-prem LLMs …
  The fast-path invokes the agent directly with the request context (HTTP
  WriteTimeout) so the agent's internal budgets are authoritative."*
- It calls `okagent.Agent.Run(ctx, prompt)` directly with the **HTTP request
  context**. The substrate `limits.timeout_ms` (120000) and
  `limits.per_tool_timeout_ms` (30000) are NOT applied on this path.
- The substrate `open_knowledge_invoke` tool (with `PerCallTimeoutMs: 0`,
  i.e. bounded by `sc.Limits.PerToolTimeoutMs` = 30000 in
  `executor.go`) is only reached when the agent is **not wired** — and in
  that case the substrate handler returns a `not_wired` refusal immediately
  (no loop). So the substrate `per_tool_timeout_ms` / `timeout_ms` are
  effectively **dead** for the real wired reasoning path.
- The substrate loader hard-caps `limits.timeout_ms` at `[1000, 120000]`
  (`internal/agent/loader.go`), so even if it DID apply, it could not be
  raised past 120000 without a substrate-wide loader change (out of scope).

**Conclusion:** the real ceiling for a `/ask` reasoning turn is the HTTP
server **`WriteTimeout`** in `cmd/core/main.go`, currently `1800s`, with the
comment *"sized for … max_iterations × per_llm_timeout (e.g. 3 × 600s = 30
min)."* The agent's own budgets (`max_iterations × llm_timeout_ms` per call)
are authoritative inside that window.

### Finding C2 — token budget re-add growth

`agent.go::Run` appends each tool result to `messages` and re-sends the FULL
history every iteration. `web_search` snippets are large and re-added each
turn, so cumulative `TokensUsed` (the quantity `per_query_token_budget`
caps) grows roughly **quadratically** with iteration count:
`total ≈ Σ_{i=1..N} (prompt_i + completion_i)` where `prompt_i` grows
linearly in `i`. Going 4 → 6 iterations is ≈ `(6²)/(4²) ≈ 2.25×` the
cumulative token consumption. The live 4-iteration turn (`73900d2089b6a557`)
succeeded under `64000` (status=success, not `cap_tokens`), but a
6-iteration turn at ~2.25× could approach or exceed `64000` and trigger a
premature `cap_tokens` refusal **before** the 6th synthesis turn — defeating
the purpose.

### Finding C3 — the model is in a straitjacket, then the failure is masked

- Prompt anti-drill bias: *"After your FIRST successful tool call returns
  useful content, write the final answer in the NEXT turn. Do NOT keep
  calling the same tool with paraphrased queries."* A comparison needs BOTH
  sides; this pushes a one-search answer.
- Prompt BUG-064-002 shaping: *"EXTRACT-THEN-SYNTHESIZE … (times, prices,
  temperatures, highs/lows, a schedule, a table)"* biases toward fixed
  data-point shapes, away from open reasoning.
- Salvage cascade in `agent.go`: `synthesizeFromSnippets` (forced-final empty
  text) and the `isUngroundedExcuse → synthesizeFromSnippets` body-quality
  salvage stitch deduped snippets and present them **as a confident answer**.
  The contradictory-snippet pomegranate body is exactly this.

---

## Decisions

### D1 — Reasoning prompt (CHANGE 1) — `open_knowledge.yaml::agent_system_prompt`

Replace the "Recommended planning bias", the "CRITICAL — When to stop calling
tools" anti-drill section, and the question-type enumeration in the
"Final-answer shape" with a **question-agnostic reasoning contract**:

1. **DECOMPOSE** the question into the sub-questions needed to answer it.
   A comparison ("X vs Y for Z") requires evidence about X, about Y, and the
   criteria Z. A "why"/"how" requires the mechanism. A "recommend"/"which is
   better" requires the candidate options AND the deciding factors. The
   guidance describes these as *examples of decomposition*, NOT an enumerated
   set of supported question types (FR-2: stays general).
2. **GATHER** evidence for EACH sub-question / side with distinct, targeted
   tool calls (not paraphrases). For a comparison, gather ALL sides before
   answering. Re-issue a call only to fill a specific gap.
3. **RECONCILE** contradictions explicitly — resolve (more
   authoritative/specific/recent) or caveat; never paste two contradictory
   snippets side by side as if both are the answer.
4. **ANSWER THE ACTUAL QUESTION** — the comparison verdict / causal
   explanation / recommendation / specific value — synthesized in the
   agent's own words. A per-source recap is NOT an answer.

**Preserved verbatim (FR-4):** the intro, R1-R4 hard rules, the tool
descriptions, the refusal shape, the entire `<CITATIONS>` contract block and
the three citation shapes, and the Style rules ("SYNTHESIZE not dump", "never
repeat the same block", "do not invent URLs / artifact IDs / numeric
values", "CITATIONS only at the very end", "no multiple CITATIONS blocks").

### D2 — Iteration budget (CHANGE 2, config) — `max_iterations` 4 → **6**

`assistant.open_knowledge.max_iterations: 6`. Rationale: 6 = **5
tool-calling turns + 1 forced-synthesis turn**. Five tool-calling turns is
enough for a multi-hop or multi-side question (e.g. side X, side Y, a
criteria search, and a gap-fill, then synthesize). 6 is chosen over 8 because
the worst-case-cold latency at 8 would press the WriteTimeout / loader
ceilings (see D5) with little reasoning benefit. Kept SST + fail-loud
(`> 0` when enabled; validated by `internal/config/openknowledge.go`).

### D3 — Token budget (CHANGE 2, config) — `per_query_token_budget` 64000 → **128000**

`assistant.open_knowledge.per_query_token_budget: 128000`. Rationale (Finding
C2): 4 → 6 iterations is ≈ 2.25× cumulative tokens; doubling the budget keeps
the 6th synthesis turn off the `cap_tokens` refusal. 128000 is 50% of
gemma4:26b's `262144` context, and — because the production `CostFn` is
zero-cost (local Ollama + local SearxNG) — the token budget is a pure safety
guardrail, so erring generous costs nothing. Kept SST + fail-loud (`> 0`).

### D4 — Reflect-before-final nudge (CHANGE 2, `agent.go`)

Preserve the forced-final-turn tool-stripping mechanism (it prevents
`cap_iterations` refusals). Add a lightweight, **ephemeral** nudge on the
second-to-last iteration (`iter == MaxIterations-2`, only when
`len(trace) > 0`): re-read the question, check whether the evidence answers
what was asked and whether all parts/sides are covered, and issue ONE more
targeted tool call to fill a gap if needed — otherwise proceed to the final
answer. It is appended to an ephemeral copy of the message list (mirrors the
forced-final pattern), within the existing iteration budget, with no new
model and no new dependency. The nudge text is question-agnostic (it gives
"comparison → every option", "why → mechanism", "recommend → criteria" as
*examples* of what "all parts" means, consistent with the prompt contract).

### D5 — Latency ceiling (CHANGE 2) — `WriteTimeout` 1800s → **3600s**

Per Finding C1, the real `/ask` ceiling is the HTTP `WriteTimeout`, which the
spec-064 comment sizes as `max_iterations × per_llm_timeout`. At
`max_iterations: 6` and `llm_timeout_ms: 600000`, the documented worst-case
invariant is `6 × 600s = 3600s`. We raise `WriteTimeout` from `1800s` to
`3600s` in `cmd/core/main.go` and update the comment so the invariant stays
honest. This only matters in the pathological all-calls-hit-the-10-minute-cap
case (impossible on home-lab GPU, where calls are seconds); realistic
6-iteration home-lab turns are ~40-60s (observed 4-iteration turns are
25-36s end-to-end), leaving enormous headroom. The change is a defensive
backstop, not a hot-path latency cost.

Expected latency at 6 iterations (home-lab, gemma4:26b resident,
keep_alive=24h): ~40-60s per reasoning turn (vs ~25-36s at 4 iterations).
Worst-case-cold (rare, once per 24h eviction): ~70-114s. All well inside the
3600s WriteTimeout.

**Finding F-LAT (surfaced to operator):** the substrate `limits.timeout_ms`
(120000) and `limits.per_tool_timeout_ms` (30000) in
`config/prompt_contracts/open_knowledge.yaml` do NOT bound the `/ask` reasoning
path (the fast-path bypasses them) and the loader hard-caps `timeout_ms` at
120000. They are retained unchanged (they still govern the not-wired fallback
and document the substrate contract); a clarifying comment is added to the
`limits` block noting the fast-path bypass. No substrate loader change is made
(out of scope). If a future operator raises `max_iterations` to 8+, only the
`WriteTimeout` invariant (D5) needs revisiting — not the substrate limits.

### D6 — Honest salvage (CHANGE 4, `agent.go`)

When genuine synthesis did NOT happen and the platform falls back to
`synthesizeFromSnippets`, frame the user-visible body as raw findings via a
single helper:

```
honestSalvagePrefix = "I searched but couldn't directly answer your question. Here is the most relevant information I found:"
honestSalvageBody(trace) = honestSalvagePrefix + "\n\n" + synthesizeFromSnippets(trace)   // "" when no snippets
```

Both snippet-salvage entry points are reframed:
1. Forced-final empty-text salvage (`isForcedFinalTurn && FinalText==""`).
2. Empty-citations + `isUngroundedExcuse` body-quality salvage.

The deduped/capped sources still attach (not a zero-source refusal), and the
cite-back / provenance contracts are unchanged. The frame is
question-agnostic (no question-type list). This is the Principle-8 trust fix:
a snippet wall is never again presented as a reasoned verdict.

**Unchanged (FR-10):** the genuine-synthesis path (`verdict.Verified` set, or
forced-final missing-CITATIONS where the model DID write a real text answer)
returns the model's own text verbatim — the honest-salvage frame is applied
ONLY to `synthesizeFromSnippets` output.

---

## Affected Surfaces (exact manifest for the devops dispatch)

| File | Change | Owner-listed? |
|------|--------|---------------|
| `config/prompt_contracts/open_knowledge.yaml` | CHANGE 1 prompt rewrite + `limits` clarifying comment (F-LAT) | yes |
| `config/smackerel.yaml` | `max_iterations` 4→6; `per_query_token_budget` 64000→128000 (+comments) | yes |
| `cmd/core/main.go` | `WriteTimeout` 1800s→3600s (+comment) (D5) | analyze-surfaced |
| `internal/assistant/openknowledge/agent/agent.go` | reflect-before-final nudge (D4) + honest-salvage frame (D6) | yes |
| `internal/assistant/openknowledge/agent/reasoning_loop_spec084_test.go` | new adversarial tests | yes (new tests) |
| `cmd/core/openknowledge_prompt_contract_test.go` | new prompt-content guard test | yes (new tests) |
| `docs/Operations.md` | amend the spec-064 open-knowledge section (max_iterations, budget, latency, honest-salvage, F-LAT) | yes (docs) |
| `specs/084-open-knowledge-reasoning-loop/**` | the spec artifact set | yes |
| `config/generated/{dev,test}.env` | regenerated by `./smackerel.sh config generate` | gitignored (NOT in git manifest) |

**NOT touched** (spec-083 + model): `internal/cardrewards/`,
`ml/app/card_categories.py`, `ml/app/main.py`,
`ml/tests/test_card_categories.py`, `specs/083-card-rewards-companion/*`,
`tests/integration/cardrewards_extract_test.go`,
`internal/deploy/docs_connector_count_contract_test.go`,
`docs/Development.md`, `docs/smackerel.md`, `llm_model_id` / the
hardware-tier model matrix.

---

## Operator Safety & Rollback

- **Worst-case failure mode:** if 6 iterations + the larger context regress
  latency or quality on home-lab, the operator reverts the four edits
  (prompt, two SST values, WriteTimeout, agent.go) — they are independent and
  contain no schema/migration changes. The cite-back / provenance trust
  contracts are untouched, so there is no trust regression surface.
- **No data migration, no schema change, no new dependency.**
- **Rollback path:** `git revert` of the devops commit restores the
  spec-064 baseline (max_iterations=4, 64000, WriteTimeout=1800, old prompt,
  old salvage). The deploy adapter is a pure pointer-swap (knb overlay).

## Pre-Apply Verification Impact

This spec changes no deploy adapter, no manifest, and no secret. The spec-064
SCOPE-17/18 deploy contract is unchanged. The home-lab apply is a separate
`bubbles.devops` dispatch; this spec terminates at validated-in-repo.
