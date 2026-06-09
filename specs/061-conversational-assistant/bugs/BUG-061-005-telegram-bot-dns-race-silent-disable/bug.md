# BUG-061-005 — Telegram bot init silently disabled on DNS hiccup at container start

> **Bug ID:** BUG-061-005
> **Spec:** 061-conversational-assistant
> **Severity:** S1 (production bot disappears for entire process lifetime after a transient DNS event)
> **Discovered:** 2026-06-09 during `bubbles.devops` triage of broken `/weather`
> **Status:** **DONE — fix already shipped in commit `96acf294` (deployed 2026-06-09T15:33:39Z)**

This is a retroactive bug artifact for a fix that was authored,
committed, reviewed, and deployed earlier in the same triage session.
Filing now per "for found issues file bugs and resolve" instruction.

---

## Symptoms

After `smackerel-home-lab-smackerel-core-1` restarted at
2026-06-09T07:56:12Z, the Telegram bot was unreachable for ~6.5
hours. Logs showed:

```json
{"time":"2026-06-09T07:56:21Z","level":"WARN","msg":"telegram bot initialization failed","error":"create bot API: Post \"https://api.telegram.org/bot.../getMe\": dial tcp: lookup api.telegram.org on 127.0.0.11:53: server misbehaving"}
{"time":"2026-06-09T07:57:35Z","level":"INFO","msg":"telegram bot not configured; assistant facade ready but no telegram transport bound"}
```

No further bot activity until manual `docker restart` at 14:33 PDT.

---

## Root cause

`cmd/core/wiring.go::startTelegramBotIfConfigured()` made a single
`telegram.NewBot()` call. On failure it logged WARN and `return nil`,
leaving the smackerel facade running with `tgBot == nil` for the
entire process lifetime. The next code path
(`wireAssistantTelegramAdapter`) saw the nil and logged
`telegram bot not configured; assistant facade ready but no telegram transport bound`
— a misleading INFO line that suggests intent rather than failure.

DNS for `api.telegram.org` recovered within seconds, but nothing in
the process re-tried.

---

## Fix (already shipped)

Commit `257fced1` → `96acf294` (squashed; the relevant change is in
`96acf294`'s diff for `cmd/core/wiring.go`):

```go
// Retry with exponential backoff: 0,1,2,4,8,16s (6 attempts, ~30s total)
backoffs := []time.Duration{0, 1*time.Second, 2*time.Second, 4*time.Second, 8*time.Second, 16*time.Second}
for attempt, backoff := range backoffs {
    if backoff > 0 {
        select {
        case <-ctx.Done():
            return nil
        case <-time.After(backoff):
        }
    }
    tgBot, err = telegram.NewBot(tgBotCfg)
    if err == nil { break }
}
if err != nil {
    // All retries exhausted. A token IS configured, so the operator
    // intends telegram to be on. Fail loud so Docker's auto-restart
    // can try again with fresh DNS.
    slog.Error("telegram bot initialization failed after retries; exiting", ...)
    os.Exit(1)
}
```

### Why retry-then-exit-loud, not retry-forever

- A 6-attempt 30s window covers transient DNS hiccups and Docker
  embedded-DNS startup race.
- If 30s isn't enough, exiting non-zero lets Docker's restart
  policy kick in with fresh container init (including fresh DNS
  resolver state), instead of silently running for the rest of
  the process lifetime with no Telegram transport. Matches the
  smackerel-no-defaults / fail-loud SST policy.

---

## Definition of Done

- [x] Retry loop with bounded backoff added
- [x] Fail-loud exit on retry exhaustion
- [x] Committed to `main` (`96acf294`)
- [x] CI green
- [x] Deployed to <deploy-host> via ci-keyless promote
- [x] Post-deploy verification: telegram bot started at 15:04:57Z, no
      retries needed (DNS was healthy)
- [ ] Live exercise of the retry path requires forcing a DNS failure
      at container start (deferred to a chaos drill)

## Files changed

- `cmd/core/wiring.go` — added retry loop + os.Exit(1) on exhaustion;
  added explanatory comments referencing today's incident as the
  forcing function

## Related work

- BUG-016-001 (weather tool timeout too tight) — both shipped together
  in `96acf294`
- BUG-015-001 (ollama volume) — same root incident (2026-06-09 boot)
- BUG-061-004 (provider_unavailable observability) — sibling
  observability theme: smackerel-core silently disabled a transport
  without making the operator-visible signal loud enough
