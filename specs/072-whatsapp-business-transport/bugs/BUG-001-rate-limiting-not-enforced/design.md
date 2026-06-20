# Design: BUG-001 Per-User Rate Limiting Not Enforced

Owner: `bubbles.design`

## Design Brief

**Current State.** The WhatsApp adapter accepts and processes all inbound webhooks after signature verification and identity resolution, with no per-user rate limiting despite the `RateLimitPerUserPerMinute` SST value being required and stored.

**Target State.** Enforce per-user rate limiting using a sliding-window or token-bucket limiter. After identity resolution, check whether the resolved `user_id` has exceeded their per-minute budget. If exceeded, reject with HTTP 429 before facade invocation.

## Implementation

### Rate Limiter

New file: `internal/whatsapp/assistant_adapter/ratelimit.go`

```go
// perUserLimiter tracks per-user request counts in a sliding window.
type perUserLimiter struct {
    mu       sync.Mutex
    capacity int
    window   time.Duration
    buckets  map[string]*userBucket
}

type userBucket struct {
    count      int
    windowStart time.Time
}

func newPerUserLimiter(perMinute int) *perUserLimiter

// Allow returns true if the user has budget remaining, false if rate limited.
func (l *perUserLimiter) Allow(userID string) bool
```

### Integration Point

In `webhook_handler.go`, after successful `Translate()`:

```go
// After identity resolution, check rate limit
if !h.adapter.AllowUser(canonical.UserID) {
    webhookRateLimitExceeded.Inc()
    h.logger.Warn("whatsapp webhook rate limited",
        "kind", "whatsapp_webhook_rate_limit",
        "user_id_hash", hashForLog(canonical.UserID),
    )
    w.Header().Set("Retry-After", "60")
    writeJSONError(w, http.StatusTooManyRequests, "rate_limit_exceeded")
    return
}
```

### Metrics

New counter:
```go
webhookRateLimitExceeded = prometheus.NewCounter(
    prometheus.CounterOpts{
        Name: "assistant_whatsapp_webhook_ratelimit_exceeded_total",
        Help: "WhatsApp webhook deliveries rejected for per-user rate limit exceeded.",
    },
)
```

### Testing

1. Unit test: verify limiter allows N requests then blocks N+1
2. Unit test: verify limiter resets after window expires
3. Integration test: verify 429 response shape and Retry-After header
4. Adversarial test: verify concurrent requests from same user are properly limited
