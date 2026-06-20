# Scopes: BUG-001 Per-User Rate Limiting Not Enforced

## Scope 1: Implement Per-User Rate Limiter

**Status:** Done
**Depends On:** —
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-BUG001-01 — Per-user rate limit enforcement
  Given the WhatsApp adapter is configured with rate_limit_per_user_per_minute = 10
  And a user has sent 10 messages in the current minute
  When an 11th signed WhatsApp message webhook arrives from that user
  Then the request is rejected with HTTP 429
  And the response includes Retry-After header
  And the facade is never invoked
  And the rate limit exceeded metric is incremented

Scenario: SCN-BUG001-02 — Rate limit resets after window
  Given a user has been rate limited
  When 60 seconds pass
  Then the user can send messages again
  And the first message in the new window succeeds
```

### Implementation Plan

1. Create `internal/whatsapp/assistant_adapter/ratelimit.go` with sliding-window per-user limiter
2. Wire limiter into `Adapter` struct with limiter field
3. Add `AllowUser(userID string) bool` method to Adapter
4. Add rate limit check in `webhook_handler.go` after `Translate()` succeeds
5. Add `webhookRateLimitExceeded` Prometheus counter
6. Return 429 with `Retry-After: 60` header when limit exceeded

### Test Plan

| Row | Scenario | Category | File/Location | Test Title | Command |
|---|---|---|---|---|---|
| TP-BUG001-01 | SCN-BUG001-01 | unit | `internal/whatsapp/assistant_adapter/ratelimit_test.go` | Per-user limiter allows N then blocks N+1 | `./smackerel.sh test unit --go` |
| TP-BUG001-02 | SCN-BUG001-02 | unit | `internal/whatsapp/assistant_adapter/ratelimit_test.go` | Limiter resets after window expiry | `./smackerel.sh test unit --go` |
| TP-BUG001-03 | SCN-BUG001-01 | unit | `internal/whatsapp/assistant_adapter/webhook_handler_ratelimit_test.go` | Handler returns 429 with Retry-After when rate limited | `./smackerel.sh test unit --go` |
| TP-BUG001-04 | SCN-BUG001-01 | integration | `tests/integration/assistant/whatsapp_ratelimit_test.go` | Live rate limit enforcement | `./smackerel.sh test integration` |

### Definition of Done

- [x] Per-user rate limiter implemented in `ratelimit.go`
- [x] Limiter wired into Adapter at construction
- [x] Webhook handler checks rate limit after identity resolution
- [x] 429 response includes `Retry-After: 60` header
- [x] Prometheus counter `assistant_whatsapp_webhook_ratelimit_exceeded_total` incremented
- [x] Unit tests pass: TP-BUG001-01, TP-BUG001-02, TP-BUG001-03
- [ ] Integration test passes: TP-BUG001-04
- [x] Build gate passes: `./smackerel.sh check && ./smackerel.sh lint`
