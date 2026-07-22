# BUG-061-007 — `/weather <location>` answers "saved as an idea" instead of the forecast

- **Spec:** specs/061-conversational-assistant
- **Severity:** S2 (a core, advertised slash command is unusable on the live self-hosted bot)
- **Discovered by:** operator (live self-hosted Telegram bot, screenshot)
- **Discovered at:** 2026-07-22
- **Related:** BUG-061-006 (duplicate/contradictory capture ack — fixed) flagged this exact
  `/weather` masking as an out-of-scope follow-up. This bug is that follow-up.

## Symptom

On the live self-hosted Telegram bot:

```
user:  /weather <us-zip>
bot:   saved as an idea — i'll surface it later.
```

The user asked for the weather and was told their message was saved as an idea. No
forecast, no honest "unavailable" — a confusing, wrong answer for an explicit command.

## Reproduction (live)

1. Open the self-hosted Telegram bot.
2. Send `/weather <us-zip>` (any city or ZIP).
3. Observe the reply is `saved as an idea — i'll surface it later.` instead of a forecast.

## Root cause (traced from the deployed `assistant_turn` audit)

The deployed audit for the turn shows:

```
scenario_id = weather_query   band = high   status = saved_as_idea
outcome = provider-error
outcome_detail = error=llm_returned_no_tool_calls_and_no_final
provider = ollama   model = ollama_chat/qwen3:30b-a3b
```

`/weather` routes to the `weather_query` scenario, which relies on the LLM emitting a
`weather_lookup` tool call (the scenario uses `direct_output_from_tool: weather_lookup`,
which only short-circuits *after* the model emits the call). The self-hosted local model
(`qwen3:30b`) sometimes returns **no tool call and no final answer**, so the executor
records `OutcomeProviderError`. Because `weather_query` is `requires_provenance`, the
weather source-assembler returns empty Sources and the **provenance gate rewrites the
provider-error into the capture-as-fallback body** `saved as an idea — i'll surface it
later.` (provenance/gate.go sets `CaptureRoute=true`). The geocoder was never the
problem — Open-Meteo resolves the ZIP to a location directly.

## Impact

Every `/weather` invocation on the self-hosted deployment can silently degrade to a
"saved as an idea" non-answer whenever the local model fails to emit the tool call —
which is frequent for `qwen3:30b`. An explicit, advertised command is effectively broken.

## Expected

An explicit `/weather <location>` command must return the forecast (with provider +
timestamp attribution), or an HONEST "weather unavailable" line when the provider genuinely
fails — it must NEVER claim the weather question was "saved as an idea".
