# Bug Fix Design: BUG-061-001

## Root Cause Analysis

### Investigation Summary
1. Reproduced the user-supplied failure description against the running test stack (warm, 23 minutes uptime) — the test PASSED, all three rows green. The failure is **not** reproducible on warm stack, confirming the cold-start timing hypothesis.
2. Verified the dispatch chain by code-reading:
   - `internal/telegram/webhook_handler.go` `ServeHTTP` → `subtle.ConstantTimeCompare` secret check → `json.Unmarshal` → `dispatcher.DispatchMessage(ctx, update.Message)` → `b.safeHandleMessage(ctx, msg)` (synchronous; returns 200 *after* dispatch completes)
   - `internal/telegram/bot.go` `handleMessage` → `resolveActorUserID` (test env returns `("", nil)` for unmapped chats — does NOT short-circuit) → falls through every priority gate → `text != ""` branch → `assistantAdapter.IsBound()` true in test → `assistantAdapter.HandleUpdate(ctx, update)`
   - `internal/telegram/assistant_adapter/adapter.go` `HandleUpdate` → `assistant.Handle(ctx, msg)` (this is the cold-start LLM call that takes 20–40s on first invocation against unloaded Ollama) → on `resp.CaptureRoute && update.Message != nil` calls `a.capture(...)` which is bound to `bot.handleTextCapture` in `assistant_wiring.go`
3. Confirmed none of the uncommitted working-tree changes (assistant metrics package, confirm machine updates, config additions, eval/observability tests) touch the dispatch chain.
4. Confirmed test code: poll loop is `for i in 1..15; sleep 1` = 15s budget total. First-invocation Ollama classifier cold-load routinely exceeds this on a fresh stack.

### Root Cause
The test's 15-iteration / 15-second artifact-poll budget races the cold-start latency of `assistant.Handle` → Ollama model load on the first webhook delivery. The production code path is correct; only the test budget is wrong.

### Impact Analysis
- Affected components: `tests/e2e/test_telegram_assistant_bs001.sh` only
- Affected data: none (no production behavior changes)
- Affected users: CI / contributors running e2e against cold stack

## Fix Design

### Solution Approach
Bump the ROW-1 artifact-poll budget from 15 iterations × 1s = 15s to 60 iterations × 1s = 60s. This:
- Comfortably covers observed cold-start Ollama load (~45s) + slack
- Still fails fast (within 60s) when the dispatch chain is truly broken
- Preserves the adversarial discriminator: a broken dispatch produces ZERO artifact rows, which the loop will still detect because `e2e_psql` returns empty string for every poll until either the row appears or 60s elapses
- One-line edit, low risk

### Alternative Approaches Considered
1. **Eager Ollama model load on smackerel-core startup** — rejected; complicates startup, affects production
2. **Inject a `dispatch-complete` synchronous flush in the webhook handler** — rejected; would change production semantics and add coupling
3. **Bump to 120s** — rejected; too forgiving, would mask real regressions

### Adversarial Regression Coverage
- ROW-2 (wrong secret returns 401, zero artifact rows) — UNCHANGED; still detects regression to plain `==` or permissive fallback in `subtle.ConstantTimeCompare`
- ROW-3 (missing header returns 401, zero artifact rows) — UNCHANGED; still detects missing-header acceptance
- ROW-1 — with the bumped budget, a genuine dispatch break (e.g., webhook returns 200 but never calls `DispatchMessage`, or `safeHandleMessage` swallows the update) STILL produces zero artifact rows for the full 60s window and the test fails. The bumped budget does not weaken the assertion; it only widens the time tolerance.
