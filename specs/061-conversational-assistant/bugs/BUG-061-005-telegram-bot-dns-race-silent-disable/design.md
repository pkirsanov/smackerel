# BUG-061-005 — Design

## Root cause

See `bug.md` § Root cause. The pre-fix code:

```go
tgBot, err := telegram.NewBot(...)
if err != nil {
    slog.Warn("telegram bot initialization failed", "error", err)
    return nil
}
```

Two failures:

1. **No retry.** A single DNS hiccup is terminal. Docker's embedded
   DNS resolver (`127.0.0.11`) takes a few seconds to come up after
   `containerd` starts the container; if the smackerel-core init
   races that, the lookup fails.
2. **Silent disable.** Returning `nil` lets the calling code path
   continue with `tgBot == nil`. The downstream `wireAssistantTelegramAdapter`
   sees nil and logs `telegram bot not configured; assistant facade
   ready but no telegram transport bound` — phrased as if the
   operator intentionally disabled telegram, not as a startup failure.

## Solution design

### Retry with bounded exponential backoff

```go
backoffs := []time.Duration{0, 1*s, 2*s, 4*s, 8*s, 16*s}  // 6 attempts, ~30s total
for attempt, backoff := range backoffs {
    if backoff > 0 {
        select {
        case <-ctx.Done(): return nil
        case <-time.After(backoff):
        }
    }
    tgBot, err = telegram.NewBot(tgBotCfg)
    if err == nil { break }
    slog.Warn("telegram bot initialization failed; will retry", ...)
}
```

Rationale for the specific schedule:

- Total ~30s covers observed Docker embedded-DNS startup latency
  (typically 1-5s in practice) AND brief network blips with margin.
- Exponential backoff avoids hammering the LLM provider's auth
  endpoint with rapid retries.
- 6 attempts is bounded; we'll never spin forever.
- `ctx.Done()` check allows graceful shutdown during retry.

### Fail loud on exhaustion

```go
if err != nil {
    slog.Error("telegram bot initialization failed after retries; exiting so the container can be restarted with fresh DNS", ...)
    os.Exit(1)
}
```

`os.Exit(1)` is the correct response because:

1. The operator configured a token → they intend telegram to work.
2. Docker `restart: unless-stopped` (smackerel-core's default policy)
   recycles the container with fresh DNS resolver state.
3. The alternative (return nil and continue with telegram silently
   off) violates the smackerel-no-defaults / fail-loud SST policy
   that the repo enforces for runtime config.

### Why this is in `wiring.go` not `telegram.NewBot`

`telegram.NewBot` is a pure constructor — it shouldn't decide retry
policy. Retry is a wiring concern (how aggressively to fight init
failures) that varies by deployment surface.

## Out of scope

- Pre-flight DNS resolution check before calling `telegram.NewBot`
  (would add complexity without removing the race; the retry covers it).
- Configurable retry parameters via SST (the hard-coded values are
  appropriate for the failure surface; SST would invite tuning
  bikeshedding without clear use case).
