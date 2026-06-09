# BUG-061-005 — Execution evidence

## Pre-fix observation (2026-06-09)

```bash
$ ssh <deploy-host> 'docker logs smackerel-home-lab-smackerel-core-1 --since 7h --until 6h 2>&1 \
              | grep -iE "telegram|bot" | tail -10'

{"time":"2026-06-09T07:56:21.134574598Z","level":"WARN","msg":"telegram bot initialization failed","error":"create bot API: Post \"https://api.telegram.org/bot.../getMe\": dial tcp: lookup api.telegram.org on 127.0.0.11:53: server misbehaving"}
{"time":"2026-06-09T07:57:35.615253302Z","level":"INFO","msg":"telegram bot not configured; assistant facade ready but no telegram transport bound"}
```

Total bot downtime: ~6h35m (07:56 PDT → 14:33 PDT manual restart).

DNS was healthy inside the container moments later:

```bash
$ ssh <deploy-host> 'docker exec smackerel-home-lab-smackerel-core-1 \
                wget -qO- --timeout=3 https://api.telegram.org/ 2>&1 | head -5'
<!DOCTYPE html>
<html class="">
  <head>
    <meta charset="utf-8">
    <title>Bots: An introduction for developers</title>
```

But the smackerel-core process had already given up on telegram for
the rest of its lifetime.

## Fix application

Commit:

```
fix(weather,telegram): /weather provider_unavailable + bot DNS-race silent disable
SHA: 96acf29459e9c972005e0c9d95d365941e1bda28
Date: 2026-06-09T05:53 UTC (during this same triage session)
```

Build verification:

```bash
$ cd ~/smackerel && go build ./cmd/core/ 2>&1
[exit 0]
$ go vet ./cmd/core/ 2>&1
[exit 0]
```

CI verification:

```bash
$ gh run view 27214998293 --json status,conclusion
status=completed conclusion=success
```

Live deploy (via ci-keyless promote on knb):

```bash
$ ssh <deploy-host> 'docker logs smackerel-home-lab-smackerel-core-1 --since 35m 2>&1 \
                | grep "telegram bot started"'
{"time":"2026-06-09T15:04:57.062286122Z","level":"INFO","msg":"telegram bot started","bot_name":"smackerel_bot"}
```

No retries were needed (DNS was healthy at startup), so the retry
loop did not exercise. The fail-loud exit path also did not exercise.
Live exercise of both paths is deferred to a chaos drill (e.g.
deliberately freeze `127.0.0.11` during container start).

## Files changed (in commit `96acf294`)

- `cmd/core/wiring.go` — startTelegramBotIfConfigured() now retries
  6× with exponential backoff (1→16s) and `os.Exit(1)` on exhaustion

## Doc note

The fix's commit message explicitly cites today's incident: "this is
how the production bot disappeared for ~6h on 2026-06-09 after a
transient DNS blip during container restart." Future debuggers will
find the chain via `git log -G`.
