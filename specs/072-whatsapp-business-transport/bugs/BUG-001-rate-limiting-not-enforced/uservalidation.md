# User Validation: BUG-001 Per-User Rate Limiting Not Enforced

## Validation Status

**Status:** Pending implementation

## Acceptance Criteria

1. A WhatsApp user cannot send more than `rate_limit_per_user_per_minute` messages
2. Excess messages receive HTTP 429 with Retry-After header
3. Rate limit resets after the 60-second window
4. Rate-limited requests never reach the facade
5. Prometheus metrics track rate limit hits

## Test Scenarios

- [ ] Send 10 messages rapidly → all succeed
- [ ] Send 11th message within same minute → 429 response
- [ ] Wait 60 seconds → next message succeeds
