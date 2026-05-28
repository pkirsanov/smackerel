# Spec: BUG-022-003 — Uniform 429 / Retry-After handling for HTTP connectors

> **Parent spec:** [022 spec](../../spec.md)
> **Bug:** [bug.md](bug.md)

## Expected Behavior

Every HTTP-based Smackerel connector MUST treat `429 Too Many Requests` (and `503 Service Unavailable` when `Retry-After` is present) as a structured rate-limit signal, sleep for the duration indicated by `Retry-After` (bounded by `Backoff`), retry, and surface the event as a labelled metric and a structured log line — not as an opaque error string.

### Behavioral Contract

| Input | Required behavior |
|---|---|
| Response status `429` with `Retry-After: <delta-seconds>` | Sleep `min(deltaSeconds, backoff.MaxDelay)`; retry; bounded by `backoff.MaxAttempts`. |
| Response status `429` with `Retry-After: <HTTP-date>` | Compute `targetTime - now`; sleep `max(0, min(diff, backoff.MaxDelay))`; retry. |
| Response status `429` with no `Retry-After` header | Use `backoff.Next()` delay; retry; bounded by `backoff.MaxAttempts`. |
| Retries exhausted | Return `rate limited: max retries exceeded`; emit `connector_429_total{connector=...,outcome="exhausted"}`. |
| Retry succeeds (eventual 200) | Return body; emit `connector_429_total{connector=...,outcome="recovered"}`. |
| `ctx.Done()` during sleep | Return `ctx.Err()` immediately. |

### Invariants

- The helper MUST NOT spin: total wall-clock time bounded by `backoff.MaxAttempts × backoff.MaxDelay`.
- The helper MUST NOT silently lower the operator's intended retry budget; defaults come from `Backoff`, which is SST-driven.
- The helper MUST NOT swallow the response body on non-retryable statuses (4xx other than 429, 5xx without Retry-After). Existing per-connector error semantics are preserved.
- Helper extraction MUST NOT change behavior of discord/guesthost/hospitable/weather (they already handle 429; their tests must keep passing).
- No new external dependency; standard library + existing `internal/connector/backoff` package only.

## Acceptance Criteria

(Mirror of bug.md acceptance criteria; canonical list lives there.)

## Out of Scope

- Migrating discord/guesthost/hospitable/weather to the shared helper (their inline 429 logic is correct; refactor is a follow-up; this bug only fixes the missing-handling sites).
- Adding a global token-bucket / per-provider quota system (markets has one; generalising it is a separate design).
- Changing the `Backoff` package itself.
- Adding circuit-breaker behavior beyond the existing bounded retry.

### Single-Capability Justification

This bug introduces a single new capability — "honor HTTP 429 + Retry-After with a bounded retry budget" — exposed as the `connector.DoWithRetry` helper. No new domain capability model is required because:

1. The capability has exactly one implementation (`DoWithRetry`) shared by every HTTP-based connector that opts in. There is no second provider, strategy, or variant.
2. The capability is a pure infrastructure primitive (HTTP-retry contract) with no domain semantics — it does not introduce new entities, business rules, or user-facing surfaces.
3. The existing per-connector inline 429 cases in discord/guesthost/hospitable/weather are intentionally left in place as already-correct implementations of the same contract; they are not "alternative providers" of a multi-implementation capability.

If a future bug adds a second retry strategy (e.g., token-bucket, circuit-breaker, jittered exponential variants), the Capability Foundation pattern (`## Domain Capability Model` + `### Variation Axes`) will apply and this justification will be replaced by it. Until then, the single-helper shape is the minimum-viable form.
