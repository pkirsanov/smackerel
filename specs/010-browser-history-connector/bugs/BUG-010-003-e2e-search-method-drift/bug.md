# Bug: BUG-010-003 Browser-history E2E search method drift

## Summary
Browser-history E2E tests call `GET /api/search` while the core router exposes authenticated search only as `POST /api/search`, causing the browser-history live-stack E2E suite to fail against the current API contract.

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - Major feature certification blocked, no valid E2E workaround in the current suite
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status
- [x] Reported
- [x] Confirmed (pre-fix focused E2E reproduced `search returned 405`)
- [x] In Progress
- [x] Fixed
- [x] Verified
- [x] Closed

## Reproduction Steps
1. Run the full repo E2E suite through the repo CLI.
2. Allow the Go E2E binary to execute `tests/e2e/browser_history_e2e_test.go`.
3. Observe the browser-history search checks calling `/api/search?source=browser-history&limit=...` through an authenticated GET helper.
4. Compare that consumer to `internal/api/router.go`, where `/api/search` is registered with `r.Post("/search", deps.SearchHandler)` inside the authenticated API group.

## Expected Behavior
Browser-history E2E tests should exercise the real supported search API method and assert the browser-history search behavior without receiving a method mismatch from the router.

## Actual Behavior
Before the fix, the E2E consumer used GET query-string requests for `/api/search` even though the router exposes POST `/api/search`; the selected browser-history E2E tests failed with `search returned 405`.

## Environment
- Service: Go core API and Go E2E test harness
- Version: Workspace state on 2026-04-27 during 039 full-delivery e2e stabilization
- Platform: Linux, Docker-backed disposable E2E stack

## Error Output
```text
--- FAIL: TestBrowserHistory_E2E_InitialSyncProducesArtifacts
search returned 405
--- FAIL: TestBrowserHistory_E2E_SocialMediaAggregateInStore
search returned 405
--- FAIL: TestBrowserHistory_E2E_HighDwellArticleSearchable
search returned 405
FAIL: go-e2e (exit=1)
```

## Root Cause
The first-party E2E consumer drifted from the API contract. The router's authenticated API surface treats search as a POST endpoint with a JSON body, while the browser-history E2E suite retained older GET/query-string calls.

## Verification
`tests/e2e/browser_history_e2e_test.go` now sends authenticated JSON `POST /api/search` requests for the browser-history search checks, parses the current `results`, `total_candidates`, and `search_mode` response fields, and includes `TestBrowserHistory_E2E_SearchRequestsUsePOSTContract` as an adversarial guard against stale GET search use. Focused browser-history E2E passed after the fix, the broad implementation-stage E2E run showed all browser-history search tests passing, and the later c6d2b26 baseline recorded full `./smackerel.sh test e2e` exit 0 with shell E2E 34/34 and Go E2E packages passed.

## Related
- Feature: `specs/010-browser-history-connector/`
- Search API surface: `internal/api/router.go`
- E2E consumer: `tests/e2e/browser_history_e2e_test.go`
- Existing related but non-covering bugs: `BUG-010-001-missing-sqlite-driver`, `BUG-010-002-dod-scenario-fidelity-gap`
