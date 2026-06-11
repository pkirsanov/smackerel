# BUG-064-002 — Design (root-cause analysis + fix design)

- **Spec:** `specs/064-open-ended-knowledge-agent`
- **Bug:** BUG-064-002

## Current Truth (verified against code this session)

### Data flow for an NL open-knowledge question

1. Telegram inbound → `internal/assistant/facade.go::Handle` → router → `BandHigh`.
2. `scenarioID == "open_knowledge"` and the agent is wired →
   `facade.go::runOpenKnowledgeDirect` → `okagent.Agent.Run(ctx, prompt)`.
3. `agent.go::Run` loops up to `MaxIterations` (config = 4). Iterations 1–3 call
   `web_search`; iteration 4 is the **forced-final turn** — tools are stripped
   and the model is asked to synthesize from prior tool results.
4. The terminal `TurnResult{Status, FinalText, Sources, …}` is mapped by
   `agenttool.MapTurnResult` → `{status, body, sources}` envelope.
5. `cmd/core/wiring_assistant_openknowledge_assembler.go::newOpenKnowledgeAssembler`
   parses the envelope, caps the user-visible sources to `assistant.sources_max`
   (5), sets `resp.Body = env.Body`.
6. `facade.go` sets `resp.Status = translateOutcomeToStatus(result.Outcome, scenarioID)`.
7. `internal/telegram/assistant_adapter/render_outbound.go` renders
   `statusPrefix(resp)` + body + sources block.

### Root cause — DEFECT 1 (snippet dump) + DEFECT 2 (triplicate)

`agent.go::Run` has three "salvage" paths that fire when the model fails to
produce a properly-cited synthesis:

- **Forced-final-turn empty-text salvage** — model returns empty text on the
  last iteration → `body := synthesizeFromSnippets(trace)`.
- **Forced-final-turn missing-`<CITATIONS>` salvage** — model wrote text but no
  citations block → uses the model's trimmed text (a synthesis, OK).
- **Empty-citations salvage** — `<CITATIONS>[]</CITATIONS>` with non-empty text:
  if `isUngroundedExcuse(finalText)` is true → `salvageBody = synthesizeFromSnippets(trace)`.

`synthesizeFromSnippets` (verified):

```go
func synthesizeFromSnippets(trace []ToolTraceEntry) string {
    var parts []string
    totalLen := 0
    const maxBodyChars = 1500
    for _, e := range trace {
        if e.Result == nil { continue }
        for _, snip := range e.Result.Snippets {
            text := strings.TrimSpace(snip.Text)
            if text == "" { continue }
            ...
            parts = append(parts, text)
            totalLen += len(text) + 2
            break // one snippet per tool call is enough
        }
        ...
    }
    return strings.Join(parts, "\n\n")
}
```

It takes the **lead snippet of each tool call** and joins them with `\n\n`,
**with no de-duplication**. The live turn ran 3 `web_search` calls for the tide
question; the top result was the same "wa-town-A tide times" preview each
time, so the body became that preview **3×** (DEFECT 2). The body is raw
search-result preview text, never the extracted high/low table (DEFECT 1) —
because the model failed to synthesize and the salvage dumps snippets. The
salvage masks a synthesis failure with a duplicated raw-snippet dump. This
confirms the operator's hypothesis.

> Which salvage trigger fired live (empty forced-final text vs. ungrounded
> excuse) is indeterminable from prod logs because `body_redacted=true`. Both
> triggers call `synthesizeFromSnippets`; both are reproduced and fixed by the
> de-duplication. The un-redacted body is proven at the agent layer in tests.

### Root cause — DEFECT 3a (thinking…) 

`facade.go::translateOutcomeToStatus`:

```go
func translateOutcomeToStatus(outcome agent.Outcome, scenarioID string) contracts.StatusToken {
    switch outcome {
    case agent.OutcomeOK:
        _ = scenarioID
        return contracts.StatusThinking   // ← delivered answer mislabeled in-flight
    ...
```

Every `OutcomeOK` answer — including a completed `open_knowledge` answer — gets
`StatusThinking`. `render_outbound.go::statusPrefix` maps `StatusThinking` →
`"thinking…"`, which is prepended as the first line of the delivered answer. The
closed status vocabulary (`contracts/response.go::AllStatusTokens`) had no
terminal "answer delivered" token.

### Root cause — DEFECT 3b (32 sources)

The salvage paths attach `collectTraceSources(trace)` — ALL deduped trace
sources (32 for the live turn). `collectTraceSources` dedups by
`web|URL|ContentHash`, so 32 = 32 distinct `(URL, hash)` pairs across 3
searches. The facade assembler caps the user-visible list to
`assistant.sources_max=5` (so the user sees `[1][2][3] … +2 more`), but the
agent's own `Sources` (and therefore the logged `num_sources`) is unbounded, and
the attached sources are arbitrary trace results rather than the few actually
used. When the model DOES cite, `Sources = verdict.Verified` (the cited set) —
already correct.

## Desired target behavior (operator-specified)

For an open-ended factual question like the tide example, the agent should
return **ONE concise synthesized answer** that directly answers the question
(here: the actual high/low tide times with heights in ft for the requested
date/place), followed by a **short, deduplicated, capped citation list** (the
few sources actually used). No `thinking…` header on the final message, no
verbatim snippet dumping, no triplicate repetition, no 32-source list.

## Honest model-capability note

Whether `gemma4:26b` reliably extracts a full tide table from raw search
snippets is a model-capability question, not purely a code one. Per the
operator directive we DO NOT swap the model. The in-repo fixes maximize the
chance of a real synthesis and make the failure mode non-terrible:

1. **Prompt-contract redesign (extract-then-synthesize)** biases the model to
   extract the specific requested values and synthesize, and forbids pasting raw
   snippets verbatim — pushing the turn onto the cited-synthesis path (where the
   body is the model's synthesis and sources are the cited set) and away from
   the snippet-salvage path.
2. **Salvage de-duplication** guarantees that when salvage DOES fire, the body
   is not a 3× duplicated dump.
3. **Source cap + dedup** bounds the attached source set.
4. **Terminal status** removes the misleading `thinking…` header.

If, on the live GPU stack, `gemma4:26b` still cannot extract the exact tide
table, the user will receive a single (deduplicated) grounded evidence digest +
a capped source list instead of a triplicated dump under a `thinking…` header —
a strict improvement — and the honest path to perfect extraction is a
model/prompt-tuning follow-up, not in scope for this S1 quality bugfix.

## Fix design

### Fix 1 — de-duplicate `synthesizeFromSnippets` (FR-1 partial, FR-2)
File: `internal/assistant/openknowledge/agent/agent.go`.
Add a normalized dedup key (`lowercase(collapse-whitespace(text))`); skip any
snippet whose key was already emitted; advance to the next snippet within the
same tool call if its lead snippet is a duplicate. Result: each distinct snippet
appears at most once; identical lead snippets across the 3 searches collapse to
one block.

### Fix 2 — prompt-contract redesign (FR-1)
File: `config/prompt_contracts/open_knowledge.yaml` (`agent_system_prompt`).
Rewrite the "Final-answer shape" + "Style" guidance to: (a) EXTRACT the specific
values the user asked for (e.g., for a schedule/table question, list each entry
with its time and unit), (b) present ONE synthesized answer, (c) NEVER paste raw
search-result snippets verbatim, (d) keep the mandatory trailing
`<CITATIONS>[…]</CITATIONS>` contract unchanged (the cite-back verifier depends
on it). The `<CITATIONS>` JSON shapes and hard rules R1–R4 are preserved
verbatim.

### Fix 3 — terminal `StatusAnswered` token (FR-3)
Files: `internal/assistant/contracts/response.go`,
`internal/assistant/facade.go`,
`internal/telegram/assistant_adapter/render_outbound.go`.
Add a terminal `StatusAnswered StatusToken = "answered"` to the closed
vocabulary (and `AllStatusTokens`). `translateOutcomeToStatus` returns
`StatusAnswered` for `OutcomeOK` on the `open_knowledge` scenario (other
scenarios are unchanged to bound blast radius). `statusPrefix` renders no prefix
for `StatusAnswered` (an explicit `case … return ""` for self-documentation;
the existing `default` already returns "").

### Fix 4 — source cap + dedup in the agent (FR-4, FR-5)
Files: `internal/assistant/openknowledge/agent/agent.go`,
`cmd/core/wiring_assistant_openknowledge.go`,
`internal/assistant/openknowledge/agent/agent_test.go` (helper).
Add a REQUIRED `Config.SourcesMax int` (validated `> 0` in `New()`; no silent
default). Wire it from the existing `assistant.sources_max` SST key. In the
salvage paths, replace `collectTraceSources(trace)` with a helper that caps the
deduped trace sources to `cfg.SourcesMax`.

## Files changed (planned)

| File | Defect | Change |
|------|--------|--------|
| `internal/assistant/openknowledge/agent/agent.go` | 1,2,3b,FR-5 | dedup `synthesizeFromSnippets`; `SourcesMax` config + validation; `cappedTraceSources` helper; salvage sites use it |
| `config/prompt_contracts/open_knowledge.yaml` | 1 | extract-then-synthesize prompt; forbid raw-snippet passthrough |
| `internal/assistant/contracts/response.go` | 3a | add terminal `StatusAnswered` token + `AllStatusTokens` |
| `internal/assistant/facade.go` | 3a | `translateOutcomeToStatus` → `StatusAnswered` for open_knowledge OutcomeOK |
| `internal/telegram/assistant_adapter/render_outbound.go` | 3a | `statusPrefix` no-prefix case for `StatusAnswered` |
| `cmd/core/wiring_assistant_openknowledge.go` | 3b | pass `cfg.Assistant.SourcesMax` into `okagent.Config.SourcesMax` |
| `internal/assistant/openknowledge/agent/agent_test.go` | (helper) | `baseCfg` sets `SourcesMax` |

## Test design (adversarial, non-tautological)

- **T1 (DEFECT 2):** `synthesizeFromSnippets` over a trace with 3 identical lead
  snippets ⇒ snippet appears exactly once. RED today (3×).
- **T2 (DEFECT 1):** end-to-end `Run` — 3 `web_search` calls return the same
  snippet, model returns empty forced-final text ⇒ salvage body contains the
  snippet once and is NOT the `S\n\nS\n\nS` raw passthrough. Plus: when the model
  returns a real cited synthesis, the body equals the synthesis (not snippets).
  RED today (3×).
- **T3 (DEFECT 3a):** `translateOutcomeToStatus(OutcomeOK,"open_knowledge")` is
  the terminal answered status, not `StatusThinking`; and `statusPrefix` renders
  no `thinking…` for it. RED today (StatusThinking → "thinking…").
- **T4 (DEFECT 3b):** salvage over a trace with N>cap distinct sources ⇒
  `len(Sources) <= cap` and no duplicates. RED today (returns all N).
- **T5 (FR-5):** `New()` rejects `SourcesMax <= 0`. RED today (field absent).
