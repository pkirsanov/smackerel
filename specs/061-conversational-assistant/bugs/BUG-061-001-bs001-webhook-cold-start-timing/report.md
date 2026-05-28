# Report: BUG-061-001 BS-001 webhook cold-start poll budget

## Summary
BS-001 e2e ROW-1 intermittently failed on cold test stacks because the 15-second artifact-poll budget raced the cold-start latency of the assistant facade's first LLM classifier call (Ollama model load, observed 20–45s). The webhook handler, dispatch chain, and capture path are all correct. Fix: bump the test poll budget to 60s. Verified PASS on cold stack.

## Completion Statement
Single-line shell edit applied to `tests/e2e/test_telegram_assistant_bs001.sh`. Re-ran BS-001 against a freshly-restarted test stack (cold Ollama, "Services healthy after 18s") — all three rows PASS.

## Test Evidence

### Bug Reproduction — Before Fix (operator-supplied, cold stack)
**Claim Source:** interpreted (operator-supplied log; reproduced indirectly by code-reading the cold-start path and the 15s budget arithmetic)

```
--- ROW-1: webhook POST with valid secret -> 200 + artifact ---
  http_status=200 body=
FAIL: ROW-1: artifact with content_raw='bs001-webhook-probe-1780004294-16876 happy-path-marker' not present in PG after 15s
```

Honesty declaration: this turn could NOT directly reproduce the 15s-budget failure on the running stack because the stack had been up ~23 minutes (warm Ollama) at investigation start. The fix's correctness rests on (a) the operator-supplied cold-stack failure above, (b) code-reading the synchronous dispatch chain in `webhook_handler.go` → `bot.go handleMessage` → `assistant_adapter.HandleUpdate` → `assistant.Handle` (cold Ollama call) → `CaptureRoute`/legacy `handleTextCapture`, and (c) the post-fix cold-stack verification below.

### Post-Fix Verification — Cold Stack
**Claim Source:** executed (this session, cold stack — test stack was just restarted, "Services healthy after 18s" confirms cold)

```
Waiting for services to be healthy (max 120s)...
Services healthy after 18s

--- ROW-1: webhook POST with valid secret -> 200 + artifact ---
  http_status=200 body=
PASS: ROW-1: happy path - webhook POST -> idea artifact with verbatim text

--- ROW-2: webhook POST with WRONG secret -> 401 + zero artifact rows ---
  http_status=401 body={"error":"invalid_secret_token"}
PASS: ROW-2: adversarial wrong-secret refused with 401 and zero artifact rows

--- ROW-3: webhook POST with NO secret header -> 401 + zero artifact rows ---
  http_status=401 body={"error":"missing_secret_token"}
PASS: ROW-3: missing-header POST refused with 401 and zero artifact rows

PASS: Spec 061 SCOPE-05 §17.5 BS-001: webhook injection mechanism live-stack-green; SCOPE-05-E2E-INJECTION-MECHANISM resolved
```

### Changes
| File | Lines | Purpose |
|------|-------|---------|
| `tests/e2e/test_telegram_assistant_bs001.sh` | 1 loop + 1 message + 5 comment lines | bump ROW-1 artifact-poll budget 15s → 60s with BUG-061-001 rationale |

### Adversarial Regression Coverage (preserved)
- ROW-2 (wrong-secret returns 401, zero artifact rows for 2s settle window) — would still fail if `subtle.ConstantTimeCompare` were replaced by `==` or a permissive fallback
- ROW-3 (missing header returns 401, zero artifact rows for 2s settle window) — would still fail if the missing-header gate were removed
- ROW-1 with 60s budget — would still fail if `webhook_handler.ServeHTTP` returned 200 without dispatching, or if `safeHandleMessage` silently dropped the update, because either bug produces zero artifact rows for the full 60s window
