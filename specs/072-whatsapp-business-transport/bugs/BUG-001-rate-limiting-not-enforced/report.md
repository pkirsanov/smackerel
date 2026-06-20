# Report: BUG-001 Per-User Rate Limiting Not Enforced

## Discovery

**Found by:** bubbles.security (stochastic-quality-sweep Round 5)
**Date:** 2026-06-17
**Trigger:** security-to-doc workflow mode

## Security Assessment

| Aspect | Assessment |
|--------|------------|
| **Attack Vector** | Network (public webhook) |
| **Complexity** | Low (requires only a mapped WhatsApp user) |
| **Privileges Required** | Low (any mapped WhatsApp user) |
| **Impact** | Resource exhaustion, DoS |
| **Severity** | HIGH |

## Root Cause Analysis

The spec 072 design specified per-user rate limiting as a required SST value (`rate_limit_per_user_per_minute`). SCOPE-1 implemented storage of the value. The design noted "SCOPE-3 enforces it" but SCOPE-3 only implemented idempotency (Meta retry deduplication), leaving rate limiting unimplemented.

## Fix Evidence

### Implementation Summary

1. **Created `internal/whatsapp/assistant_adapter/ratelimit.go`** — Per-user rate limiter with:
   - Fixed-window algorithm (60 second windows)
   - LRU eviction when capacity exceeded (65536 users)
   - Prometheus counter `webhookRateLimitExceeded`
   - Concurrent-safe via sync.Mutex

2. **Modified `internal/whatsapp/assistant_adapter/adapter.go`**:
   - Added `limiter *perUserLimiter` field to Adapter struct
   - Initialized in `NewAdapter()` from `RateLimitPerUserPerMinute` SST value
   - Added `AllowUser(userID string) bool` method

3. **Modified `internal/whatsapp/assistant_adapter/webhook_handler.go`**:
   - Added rate limit check after `Translate()` succeeds
   - Returns HTTP 429 with `Retry-After: 60` header when limit exceeded
   - Logs rate limit events with `kind=whatsapp_webhook_rate_limit`

### Unit Test Evidence

```
=== RUN   TestPerUserLimiter_AllowsThenBlocks
--- PASS: TestPerUserLimiter_AllowsThenBlocks (0.00s)
=== RUN   TestPerUserLimiter_ResetsAfterWindow
--- PASS: TestPerUserLimiter_ResetsAfterWindow (0.00s)
=== RUN   TestPerUserLimiter_RejectsEmptyUserID
--- PASS: TestPerUserLimiter_RejectsEmptyUserID (0.00s)
=== RUN   TestPerUserLimiter_EvictsOldestOnCapacity
--- PASS: TestPerUserLimiter_EvictsOldestOnCapacity (0.00s)
=== RUN   TestPerUserLimiter_ConcurrentSafety
--- PASS: TestPerUserLimiter_ConcurrentSafety (0.00s)
=== RUN   TestPerUserLimiter_IndependentUsers
--- PASS: TestPerUserLimiter_IndependentUsers (0.00s)
=== RUN   TestWebhookHandler_RateLimitEnforced
--- PASS: TestWebhookHandler_RateLimitEnforced (0.00s)
=== RUN   TestWebhookHandler_RateLimitRespectsRetries
--- PASS: TestWebhookHandler_RateLimitRespectsRetries (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter
```

### Files Changed

| File | Change Type |
|------|-------------|
| `internal/whatsapp/assistant_adapter/ratelimit.go` | Created |
| `internal/whatsapp/assistant_adapter/ratelimit_test.go` | Created |
| `internal/whatsapp/assistant_adapter/adapter.go` | Modified |
| `internal/whatsapp/assistant_adapter/webhook_handler.go` | Modified |
| `internal/whatsapp/assistant_adapter/webhook_handler_ratelimit_test.go` | Created |

### Remaining Work

Integration test TP-BUG001-04 is pending — requires live WhatsApp Business API stack. Unit test coverage is complete and exercises all rate limiting scenarios.
