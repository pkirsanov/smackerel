# BUG-064-002 — open-knowledge `/ask` returns a triplicated raw-snippet dump under a "thinking…" header instead of a synthesized answer

- **Spec:** `specs/064-open-ended-knowledge-agent`
- **Severity:** S1 (the headline open-knowledge answer is unusable: no extracted data, duplicated 3×, mislabeled as in-flight)
- **Discovered by:** operator, live home-lab Telegram bot
- **Discovered at:** 2026-06-11
- **Follows:** BUG-064-001 (routing + per-user budget fix, deployed). This is a NEW bug; BUG-064-001 stays closed.

## Summary

After the BUG-064-001 routing + budget fix was deployed (sourceSha with the
`/ask → open_knowledge` map and the 100/25 USD ceilings), an NL question now
reaches the open-knowledge agent and the agent runs to `termination_reason=final`
/ `status=success`. But the **answer quality is poor**: instead of a synthesized
answer it returns a raw web-search snippet, repeated verbatim three times, under
a literal `thinking…` header, with no extracted data.

A user sent (NL, no slash — routed to `open_knowledge`, `band=high`):

```
ask tide schedule for 06/11 in wa-town-A, wa, provide actual highs and lows (time + ft)
```

The bot replied with the unsatisfactory answer:

- Header line: `thinking…`
- Then the SAME web-search snippet block repeated **three times** verbatim:
  > "Tide Times · Home · United States; wa-town-A tides. wa-town-A Tide
  > Times, Washington. Tide Times Today & Tomorrow. « Thu, June 11. wa-town-A
  > tide today …"
- Then a sources list: `[1] tidetime.org (web), [2] surf-forecast.com (web),
  [3] surfline.com (web), … +2 more sources`
- It **never** provided the actual high/low tide times or heights in feet that
  the user explicitly requested.

## Three distinct quality defects

- **DEFECT 1 — snippet-dump instead of synthesis.** The final body is raw
  web-search snippet text, not a synthesized answer that extracts and presents
  the requested data (high/low tide times + ft).
- **DEFECT 2 — triplicate duplication.** The identical snippet block appears
  exactly 3 times, matching the 3 `web_search` tool calls.
- **DEFECT 3 — "thinking…" status on a final answer + source over-attach.**
  `assistant_turn status="thinking"` despite `openknowledge.turn
  termination_reason=final / status=success`; and `num_sources=32/23` is absurd
  for a question that needs a handful of sources.

## Reproduction steps (observed, live)

1. Home-lab deployment with the Telegram assistant bound,
   `assistant.open_knowledge.enabled=true`, `provider=searxng`,
   `model=gemma4:26b`, budgets `100/25`.
2. Send the NL prompt above (no slash) to the bot.
3. Observe a single reply: `thinking…` header, the same snippet 3×, then a
   source list of `[1][2][3] … +2 more sources`. No extracted highs/lows.

## Observed vs expected

| | Observed | Expected |
|---|----------|----------|
| **1** | body = raw web-search snippet text (search-result preview), no extracted data | body = ONE concise synthesized answer that directly answers the question (the actual high/low tide times with heights in ft for the requested date/place), OR an honest no-ground refusal AFTER a real attempt |
| **2** | the identical snippet block appears 3× (one per `web_search` call) | the body is produced ONCE — no duplicated/repeated blocks |
| **3a** | `thinking…` header on a completed answer; `assistant_turn status="thinking"` | no in-flight status header on a delivered answer; the user-visible status is terminal/success |
| **3b** | `num_sources=32/23` internally; user-visible list is capped but the sources are arbitrary trace results, not the ones used | source list is deduplicated and capped to the few sources actually used by the answer |

## Captured structured turn-logs (live core container, this session)

`msg="openknowledge.turn"` (smackerel-home-lab-smackerel-core-1 on evo-x2):

```
turn ba00c52368fdc93f: iterations=4, tokens_used=6088, usd_spent=0, status=success, termination_reason=final, num_sources=32, tool_calls=[web_search success x3]
turn acb50def9319669d: iterations=4, tokens_used=6572, usd_spent=0, status=success, termination_reason=final, num_sources=23, tool_calls=[web_search success x3]
```

`msg="assistant_turn"`: `scenario_id=open_knowledge`, `status="thinking"`,
`body_redacted=true`, `latency_ms≈17857 / 27020`.

Startup log: open-knowledge subsystem wired `provider=searxng`, `model=gemma4:26b`,
`per_query_usd_budget=0.05`, budgets `100/25`. `usd_spent=0` because the local
model is a zero-cost `CostFn` stub.

> `body_redacted=true` hides the assembled body in prod logs. The un-redacted
> body is proven at the agent layer (unit/integration tests on the assembled
> `TurnResult.FinalText` / `AssistantResponse.Body`), per the investigation
> requirement.

## Root-cause leads (verified in design.md)

- ✅ **DEFECT 1+2 root cause:** the open-knowledge agent's snippet-salvage path
  (`internal/assistant/openknowledge/agent/agent.go::synthesizeFromSnippets`)
  emits raw tool-snippet text as the final body whenever the model fails to
  produce a cited synthesis (forced-final empty text, or an "ungrounded excuse"
  body). `synthesizeFromSnippets` takes the lead snippet of EACH `web_search`
  tool call and joins them with `\n\n` **without de-duplication**, so 3 searches
  returning the same top result produce the identical block 3×. The salvage masks
  a synthesis failure with a duplicated raw-snippet dump.
- ✅ **DEFECT 3a root cause:**
  `internal/assistant/facade.go::translateOutcomeToStatus` maps every
  `OutcomeOK` answer — including a completed `open_knowledge` answer — to
  `contracts.StatusThinking`, and the Telegram adapter
  (`internal/telegram/assistant_adapter/render_outbound.go::statusPrefix`)
  renders `StatusThinking` as the `thinking…` first line. The closed status
  vocabulary had no terminal "answer delivered" token.
- ✅ **DEFECT 3b root cause:** in the salvage path the agent attaches
  `collectTraceSources(trace)` (ALL deduped trace sources — 32) rather than the
  cited set. The facade assembler caps the user-visible list to
  `assistant.sources_max` (5), but the agent's own source set is unbounded, so
  the internal `num_sources` is absurd and the attached sources are arbitrary
  trace results rather than the ones used.

## Code vs deployment

Code + config + prompt-contract fix lands in-repo and is validated at the
unit / integration level. The **live home-lab symptom clears only after a
redeploy** (rebuild `smackerel-core` from the fixed SHA + home-lab
config-bundle regen + redeploy via the knb `smackerel/home-lab` adapter) —
the same build-once-deploy-many chain BUG-064-001 used. Owner of the redeploy:
`bubbles.devops`.
