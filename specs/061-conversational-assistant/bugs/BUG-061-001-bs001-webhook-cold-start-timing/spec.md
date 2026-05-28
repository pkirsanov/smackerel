# Spec: BUG-061-001 Expected Behavior

## Expected Behavior
`tests/e2e/test_telegram_assistant_bs001.sh` ROW-1 MUST succeed on both warm and cold test stacks. The poll budget MUST be large enough to cover cold-start latency of:

1. Telegram webhook handler dispatch (synchronous, sub-millisecond)
2. `assistant.Handle` first invocation against cold Ollama (model load: up to ~45s observed)
3. `handleTextCapture` HTTP round-trip to the capture API (sub-second)
4. PostgreSQL insert of the `artifacts` row (sub-second)

A reasonable cold-start budget is **60 seconds** (45s Ollama load + 15s slack). Test still fails fast (within 60s) when the dispatch chain is truly broken.

## Acceptance Criteria
- ROW-1 PASSes on both cold and warm stacks
- ROW-2 (wrong secret) PASSes — adversarial check that secret verification still short-circuits before dispatch
- ROW-3 (missing header) PASSes — adversarial check for missing-header path
- Adversarial regression: ROW-2 / ROW-3 would still detect a regression that bypasses `subtle.ConstantTimeCompare` (untouched by this fix)

## Non-Goals
- Changing production webhook/dispatch latency
- Eager-loading Ollama models at smackerel-core startup
- Modifying the assistant facade Handle path
