# BUG-061-009 — User Validation

## Checklist

- [x] `/ask <a real question>` that the bot can't ground replies with an honest "I don't have a sourced answer for that." — NEVER "saved as an idea".
- [x] A genuine dropped thought (unrouted input) still replies "saved as an idea — i'll surface it later." (band-low capture unchanged).
- [x] A typed open_knowledge refusal (budget/tool/etc.) leads with the honest reason and does NOT show "(saved as idea)".
- [x] The deeper "why can't it answer about my own product" gap is diagnosed and routed as a follow-up (this bug does not claim to fix grounding).

## Note

Behavioral confirmation on the live bot is operator-only: sending a Telegram
message and the prod assistant HTTP API (per-user PASETO) are not agent-feasible.
The items above are checked by default (validated by the mechanical invariant
tests in `facade_execution_error_honesty_test.go` plus the adapter/contracts
tests); uncheck an item to report a live regression.
