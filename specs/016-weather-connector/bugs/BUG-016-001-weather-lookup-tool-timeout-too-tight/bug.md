# BUG-016-001 — `weather_lookup` tool `PerCallTimeoutMs: 2000` deadline-exceeded on most cold-cache invocations

> **Bug ID:** BUG-016-001
> **Spec:** 016-weather-connector
> **Severity:** S1 (user-visible: `/weather <city>` returned "I don't have a sourced answer for that" most cold-cache calls)
> **Discovered:** 2026-06-09 during `bubbles.devops` triage
> **Status:** **DONE — fix already shipped in commit `96acf294` (deployed 2026-06-09T15:33:39Z)**

Retroactive bug artifact for a fix already authored, committed, reviewed, and
deployed earlier in this session.

---

## Symptoms

User sends `/weather <city>` via Telegram. Bot replies with the canonical
provenance-gate refusal:

```
I don't have a sourced answer for that.
```

`assistant_turn` log shows:

```json
{
  "scenario_id": "weather_query",
  "status": "saved_as_idea",
  "error_cause": "provider_unavailable",
  "latency_ms": 4448
}
```

Latency 4.4s is interesting — it exceeded the tool's 2s budget by 2x. Multiple
test calls reproduced; warm-cache calls (same city, < 10min apart) worked.

---

## Root cause

`internal/agent/tools/weather/tool.go` registered the tool with
`PerCallTimeoutMs: 2000`. The agent executor (line 597 of `executor.go`)
wraps the **entire `handleWeatherLookup` call** in a 2s `context.WithTimeout`:

```go
toolCtx, toolCancel := context.WithTimeout(ctx, time.Duration(perToolMs)*time.Millisecond)
toolResult, toolErr := toolMeta.Handler(toolCtx, call.Arguments)
```

But a single weather lookup is **two sequential HTTPS round trips** to
open-meteo:

1. Geocoding (`https://geocoding-api.open-meteo.com/v1/search?name=...`) — ~1-2s
2. Forecast (`https://api.open-meteo.com/v1/forecast?latitude=...&longitude=...`) — ~1-2s

Measured worst case from inside the self-hosted container:

```bash
$ ssh <deploy-host> 'docker exec smackerel-self-hosted-smackerel-core-1 sh -c \
   "for i in 1 2 3; do time wget -qO- ... open-meteo geocode ...; done"'
geocode 1: 1s
geocode 2: 1s
geocode 3: 2s
forecast 1: 1s
forecast 2: 2s
forecast 3: 1s
```

End-to-end cold call: 2-4s. The 2s cap deadline-exceeded ~50% of cold calls
and surfaced as `weather_lookup_provider_error: context deadline exceeded`
→ `OutcomeProviderError` → `provider_unavailable` ErrorCause →
provenance-gate refusal.

Cache hits (10-minute TTL via `assistant.skills.weather.cache_ttl`) worked
because the provider call was skipped entirely.

---

## Fix (already shipped)

Commit `96acf294`:

```go
// internal/agent/tools/weather/tool.go
func init() {
    agent.RegisterTool(agent.Tool{
        ...
        // A single lookup is geocode + forecast, two sequential HTTPS
        // round trips to open-meteo. Measured worst case from the
        // self-hosted container is ~2s per call (so ~4s end-to-end on a
        // cold cache). The previous 2000 ms cap was tighter than a
        // single HTTP call and made /weather fail with
        // `provider_unavailable` on most cold-cache invocations. 8s
        // gives ~2x headroom over the observed worst case while still
        // failing fast if open-meteo or DNS is degraded.
        PerCallTimeoutMs: 8000,
        ...
    })
}
```

`2000` → `8000` ms. Cache hits and warm-path latency are unchanged
(the cap is a ceiling, not a sleep). 8s gives ~2x headroom over the
observed worst case while still failing fast if open-meteo or DNS is
truly degraded.

---

## Why not deeper changes

Considered and deferred:

- **Move timeout to SST (scenario YAML).** `config/prompt_contracts/weather-query-v1.yaml`
  already declares `per_tool_timeout_ms: 60000`, but the executor's
  `perToolMs := toolMeta.PerCallTimeoutMs` precedence (line 599) means
  the tool's own value wins when > 0. Fixing the precedence would be
  a wider refactor; bumping the tool's own ceiling is the smaller
  change with identical user-visible effect.
- **Parallelize geocode + forecast.** The forecast needs the geocode
  result (lat/lon), so they're inherently sequential. A two-stage
  parallel pipeline would require speculative geocoding which is
  out of scope.
- **Local geocoding cache.** Could pre-warm popular locations to skip
  the geocode round-trip entirely. Separate optimization scope.

---

## Definition of Done

- [x] `PerCallTimeoutMs: 2000 → 8000` in `internal/agent/tools/weather/tool.go`
- [x] Doc comment explaining the measurement + rationale
- [x] Weather unit tests still pass (`go test ./internal/agent/tools/weather/...`)
- [x] Build clean
- [x] Committed to `main` (`96acf294`)
- [x] CI green
- [x] Deployed to <deploy-host> via ci-keyless promote
- [ ] Live verification: user sends `/weather <city>` after ollama recovery
      (BUG-015-001) and gets a real forecast. **Awaits user test now that
      ollama is recovered + new tool timeout is deployed.**

## Files changed

- `internal/agent/tools/weather/tool.go` — 1-line value bump + 8-line
  doc comment explaining the change

## Related work

- BUG-015-001 (ollama volume) — the ollama-down failure mode masked
  this fix because every call short-circuited at `OutcomeProviderError`
  upstream before the weather tool was invoked
- BUG-061-004 (provider_unavailable observability) — the same
  `error_cause:"provider_unavailable"` label masked both bugs (this
  one + ollama-down) for hours
- BUG-061-005 (telegram bot DNS-race) — shipped together in `96acf294`
