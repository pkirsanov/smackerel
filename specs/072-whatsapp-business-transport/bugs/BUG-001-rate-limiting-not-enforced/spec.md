# Bug: BUG-001 Per-User Rate Limiting Not Enforced

**Status:** in_progress
**Severity:** HIGH
**Found in spec:** 072-whatsapp-business-transport
**Root cause:** Implementation gap — SST value stored but enforcement never wired

---

## Bug Summary

The WhatsApp adapter stores `RateLimitPerUserPerMinute` from SST configuration but never actually enforces per-user rate limiting. This allows a malicious or compromised WhatsApp user to flood the system with unlimited requests.

## Reproduction

1. Configure `assistant.transports.whatsapp.rate_limit_per_user_per_minute = 10`
2. Map a WhatsApp phone number to a user
3. Send more than 10 messages per minute from that phone number
4. **Expected:** Messages beyond the limit should be rate-limited (429)
5. **Actual:** All messages are processed without limit

## Evidence

The code shows the value is stored but never used:

```go
// adapter.go line 230-232
// RateLimitPerUserPerMinute is the SST-supplied per-user
// rate-limit. REQUIRED, MUST be > 0. SCOPE-1 stores the value;
// SCOPE-3 enforces it.
RateLimitPerUserPerMinute int
```

However, `webhook_handler.go` processes all requests after identity resolution without any rate checking, and SCOPE-3 only implemented idempotency (duplicate message suppression), not rate limiting.

## Security Impact

- **DoS vector:** A single mapped user can exhaust system resources by flooding the webhook
- **Resource exhaustion:** Each message invokes facade processing which is CPU/memory intensive
- **Amplification:** Meta retries failed deliveries, potentially amplifying an attack
- **No circuit breaker:** The idempotency cache only dedupes same-message retries, not distinct messages

## Fix Approach

Add per-user rate limiting enforcement AFTER identity resolution but BEFORE facade invocation in `webhook_handler.go`:

1. Create `internal/whatsapp/assistant_adapter/ratelimit.go` with a per-user token bucket
2. Check rate limit after `Translate()` succeeds (user identity is resolved)
3. Return 429 with appropriate headers if rate exceeded
4. Increment Prometheus counter `assistant_whatsapp_webhook_ratelimit_exceeded_total`
