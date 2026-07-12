# BUG-016-001 — Design

## Root cause

See `bug.md` § Root cause. Summary: `PerCallTimeoutMs: 2000` in
`internal/agent/tools/weather/tool.go::init()` wraps the entire
`handleWeatherLookup` body in a 2s context; the body makes two
sequential ~1-2s HTTPS round trips to open-meteo, so half the
cold-cache calls deadline-exceeded.

## Solution design

### Single-line bump from 2000 to 8000 ms

- Observed worst case per HTTP round trip on self-hosted: 2s
- End-to-end worst case: 4s
- New cap: 8s (2x headroom)
- Doc comment in the source records the measurement so future
  tuning is anchored to data, not vibes

### Why not 4s (matches worst case exactly)

Worst case is observed, not guaranteed. open-meteo could degrade
to 3-4s per call without going fully down. Even at 4s per call
(8s total), the new cap holds. 4s would leave no headroom and
recreate the bug under modest provider degradation.

### Why not 60s (the scenario YAML's `per_tool_timeout_ms`)

The scenario YAML's value is the **scenario-wide** ceiling that
the tool registration's value falls back to when set to 0. We
keep a tool-specific cap because:

1. Other tools in the same scenario shouldn't inherit a 60s
   ceiling (small fast tools should fail fast).
2. 60s is too forgiving for a single external HTTP call —
   if open-meteo is taking that long, the right response is to
   fail the turn so the LLM can recover (BS-015), not to keep
   the user's UI spinning for a full minute.

### Why this is the smallest sufficient fix

- One-line value change.
- No new env vars.
- No scenario YAML changes.
- Test surface unchanged (weather unit tests don't pin the
  numeric value).
- Cache behavior unchanged.

## Out of scope

- Lifting the timeout into SST (would require scenario YAML
  precedence refactor; deferred).
- Parallel HTTPS round trips (geocode + forecast are inherently
  sequential).
- Switching providers.
