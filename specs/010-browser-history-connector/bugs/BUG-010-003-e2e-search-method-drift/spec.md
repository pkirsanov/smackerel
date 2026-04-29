# Feature: BUG-010-003 Browser-history E2E search method drift

## Problem Statement
The browser-history connector is implemented and has E2E coverage, but its Go E2E search consumer calls a method that the current API router does not expose. This blocks live-stack certification because the E2E path fails on contract drift rather than proving browser-history artifacts are searchable.

## Outcome Contract
**Intent:** Browser-history E2E tests use the same authenticated search contract exposed by the running core API.
**Success Signal:** Browser-history E2E search checks send supported requests to `/api/search` and receive successful search responses from the live stack.
**Hard Constraints:** The fix must preserve the existing public API contract unless a consumer impact sweep deliberately changes the API; browser-history assertions must remain live-stack assertions with no request interception.
**Failure Condition:** The E2E suite still sends an unsupported method to `/api/search`, or the test passes without validating live search behavior.

## Goals
- Align browser-history E2E requests with the current search API contract.
- Keep browser-history E2E coverage live-stack and assertion-bearing.
- Add regression coverage that fails if the test consumer reverts to an unsupported search method.

## Non-Goals
- Rewriting browser-history connector ingestion logic.
- Changing search ranking behavior.
- Adding a new GET search API unless a consumer impact sweep and API contract decision explicitly select that path.

## Requirements
- The browser-history E2E suite must call a supported search endpoint method.
- The regression must assert HTTP success and response content shape for browser-history search queries.
- The regression must not skip or return early when the method mismatch appears.
- The fix owner must inventory first-party `/api/search` consumers before changing the API contract.

## User Scenarios (Gherkin)

```gherkin
Scenario: Browser-history E2E uses the supported search contract
  Given the live stack exposes authenticated search as POST /api/search
  When the browser-history E2E suite searches for browser-history artifacts
  Then the test sends a supported search request
  And the response status is successful
  And the response body is parsed and asserted without a method-mismatch bailout

Scenario: Browser-history E2E fails if the stale GET search consumer returns
  Given the router does not expose authenticated GET /api/search
  When a regression check scans or executes the browser-history E2E search path
  Then no browser-history E2E search request uses GET /api/search
```

## Acceptance Criteria
- Browser-history E2E search requests align with `internal/api/router.go`.
- At least one adversarial regression would fail if `apiGet(cfg, "/api/search?..." )` returned to the browser-history E2E search path.
- Full `./smackerel.sh test e2e` no longer reports this browser-history method mismatch after the fix owner completes the work.
