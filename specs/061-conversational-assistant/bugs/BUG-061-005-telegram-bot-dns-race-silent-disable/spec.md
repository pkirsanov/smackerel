# BUG-061-005 — Spec

When `cmd/core/main.go` calls `startTelegramBotIfConfigured()` AND
the operator has configured a real `TELEGRAM_BOT_TOKEN`,
`telegram.NewBot()` MUST either:

1. Succeed (the operator's intent is honored), OR
2. Fail loud — exit the process non-zero so Docker's `restart:
   unless-stopped` policy can recycle the container with fresh DNS
   resolver state. The process MUST NOT silently continue with
   `tgBot == nil` for the rest of its lifetime.

Transient failures (DNS hiccup, brief network blip) SHOULD be
retried with bounded backoff (≤30s) before the fail-loud exit.

The retry attempts and the final exit MUST log enough information
for an operator to triage from the docker log alone (attempt count,
error from each failed attempt, total elapsed time).

## Out of scope

- Webhook-mode bots (separate code path, separate failure surface).
- Per-user-token-minter init failure (already correctly handled as
  WARN + continue per the existing comment — the bot stays
  operational, only PASETO per-user binding is disabled).
- Reconnect logic for an established bot that loses its connection
  later (that's `bot.Start()`-internal and not covered here).
