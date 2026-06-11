# BUG-064-001 — `/ask` refuses every open-ended question AND leaks the `/ask` prefix into the captured idea

- **Spec:** `specs/064-open-ended-knowledge-agent`
- **Severity:** S1 (the headline open-knowledge capability is dead-on-arrival for every user query on the default self-hosted config)
- **Discovered by:** operator, live home-lab Telegram bot
- **Discovered at:** 2026-06-11

## Summary

On the live home-lab Telegram bot a user sent:

```
/ask tide schedule for 06/11 in wa-town-A, wa
```

The bot replied with TWO messages:

1. `. Saved: "/ask tide schedule for 06/11 in wa-town-A, wa" (idea)`
2. `I don't have a sourced answer for that.`

This surfaces **two distinct defects**:

- **DEFECT A — answerability.** An open-ended, web-answerable public question
  (a tide schedule) returns the canonical refusal instead of a sourced
  web-search answer.
- **DEFECT B — capture pollution.** The capture-as-fallback saves the idea with
  the literal `/ask ` slash prefix included in the title. The captured text
  should have the command prefix stripped.

## Reproduction steps (observed, live)

1. On a home-lab deployment with the Telegram assistant bound and
   `assistant.open_knowledge.enabled=true`.
2. Send `/ask tide schedule for 06/11 in wa-town-A, wa` to the bot.
3. Observe two replies: a `. Saved: "/ask …" (idea)` capture ack containing the
   literal `/ask ` prefix, followed by `I don't have a sourced answer for that.`

## Observed vs expected

| | Observed | Expected |
|---|----------|----------|
| **A** | `open_knowledge` agent refuses **before any LLM/tool call** (`termination_reason=cap_usd`); user gets the canonical refusal | `/ask` reaches the open-knowledge agent, which web-searches (searxng) and returns a sourced answer, OR a genuine no-ground refusal only AFTER actually attempting to ground |
| **B** | captured idea title = `"/ask tide schedule for 06/11 in wa-town-A, wa"` (prefix included) | captured idea title = `"tide schedule for 06/11 in wa-town-A, wa"` (slash-command prefix stripped) |

## Root-cause leads provided (status after verification)

The bug was filed with several leads; ALL were verified against code + the live
deployment (see `design.md`):

- ❌ **DISPROVEN:** "`/ask` maps to `retrieval_qa` (notes-only search)." The
  *capability-metadata* YAML still lists `retrieval_qa.slash_shortcut: "/ask"`,
  but the **actual Go routing map** (`internal/assistant/shortcuts.go`,
  Spec 064 SCOPE-17, commit `ebdbf852`, 2026-06-01) maps `/ask → open_knowledge`.
  The deployed image (`sourceSha 0bc04cfb`) contains this fix.
- ❌ **DISPROVEN:** "deployment lag — home-lab runs a stale pre-064 image." The
  deployed `sourceSha 0bc04cfb` post-dates the routing fix and the live
  `assistant_turn` log shows `scenario_id="open_knowledge"`. Routing is correct.
- ✅ **TRUE ROOT CAUSE (A):** `open_knowledge` refuses every query at the
  pre-flight per-user-monthly USD budget gate
  (`internal/assistant/openknowledge/agent/agent.go:294`,
  `PerUserMonthlyUSDRemaining <= 0`) because the SST config sets
  `assistant.open_knowledge.per_user_monthly_budget_usd: 0`. The production
  `CostFn` is a zero-cost stub, so the budget is purely a gate — `0` means
  "refuse everything". Present on current `main`, so NOT deployment lag.
- ✅ **TRUE ROOT CAUSE (B):** the Telegram adapter's `CaptureRoute` hook passes
  the verbatim `msg.Text` (which preserves the `/ask` prefix for shortcut
  detection) into `handleTextCapture`; the prefix is never stripped before
  capture.

## Live evidence (captured this session, read-only)

`openknowledge.turn` structured log from the deployed core container
(`smackerel-home-lab-smackerel-core-1`):

```
{"msg":"openknowledge.turn","turn_id":"43cdf83f81d3fce6","iterations":1,
 "tokens_used":0,"usd_spent":0,"status":"refused","termination_reason":"cap_usd",
 "num_sources":0,"tool_calls":[],
 "refusal_reason":"openknowledge: per-user monthly USD budget exceeded"}
```

`iterations:1, tokens_used:0, tool_calls:[]` proves the agent refused **before**
any LLM call or web search — the pre-flight budget gate fired.

## Impact

The flagship "ask an open-ended question" capability (spec 064) is non-functional
for **every** query on any deployment that ships the default
`per_user_monthly_budget_usd: 0`. Every `/ask` becomes a guaranteed refusal plus
a polluted idea capture.
