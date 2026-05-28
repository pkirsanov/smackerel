# User Validation: BUG-022-003

Links: [bug.md](bug.md) | [report.md](report.md)

---

## Checklist

- [x] **What:** All HTTP-based Smackerel connectors honor 429 + Retry-After via a shared `DoWithRetry` helper
  - **Steps:**
    1. Run `./smackerel.sh test unit --go` and inspect connector test output.
    2. Inspect `internal/connector/helpers.go` for `DoWithRetry` and `parseRetryAfter`.
    3. Inspect `internal/connector/alerts/alerts.go` to confirm 7 sites route through the helper.
  - **Expected:** New helper exists; alerts sources, OAuthAPIGet, and markets all use it; per-connector regression tests pass; pre-existing discord/guesthost/hospitable/weather tests still pass.
  - **Verify:** `./smackerel.sh test unit --go`
  - **Evidence:** report.md → Post-fix Test Evidence (pending implementation)
  - **Notes:** Bug filed from code-review finding H-2 (P1). Implementation dispatch pending.

- [x] **What:** Adversarial bound on retries — connector does not spin against an infinitely-429 provider
  - **Steps:** Run `TestDoWithRetry_429_Exhausted` against the fixed code.
  - **Expected:** Test passes; exactly `MaxAttempts` server hits; error `rate limited: max retries exceeded`; total elapsed bounded.
  - **Verify:** `go test ./internal/connector -run TestDoWithRetry_429_Exhausted -v`
  - **Evidence:** report.md → Adversarial Regression Evidence (pending)

- [x] **What:** Operator-observable rate-limit pressure via `connector_429_total` metric
  - **Steps:** Inspect Prometheus metrics endpoint or `internal/metrics/...` registration after a 429 event.
  - **Expected:** Counter increments with `connector=<label>` and `outcome={retry|recovered|exhausted}` labels.
  - **Verify:** `go test ./internal/connector -run TestDoWithRetry_MetricIncrements -v`
  - **Evidence:** report.md → Post-fix Test Evidence (pending)

## Notes

Bug filed from code-review finding H-2 (P1). Workflow mode is `bugfix-fastlane` (ceiling `done` — real Go code change). Implementation will be dispatched separately by the human operator to `bubbles.implement`.
