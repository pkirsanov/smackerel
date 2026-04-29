# Bug Fix Design: BUG-010-003

## Root Cause Analysis

### Investigation Summary
The 2026-04-27 e2e stabilization context reported a browser-history E2E failure where the test calls `GET /api/search`. Source inspection in this packetization pass found three browser-history E2E search call sites that use the `apiGet` helper with `/api/search?...` query strings. The router registers authenticated search with `r.Post("/search", deps.SearchHandler)` under `/api`.

### Root Cause
The browser-history E2E test consumer is stale relative to the current API method contract. The test still treats search as a GET endpoint, while the router exposes POST search.

### Impact Analysis
- Affected components: `tests/e2e/browser_history_e2e_test.go`, first-party `/api/search` E2E consumers.
- Affected data: none expected; the bug is a request contract mismatch.
- Affected users: delivery workflow and browser-history certification, because broad E2E remains red.

## Fix Design

### Solution Approach
Prefer updating the browser-history E2E search helper/call sites to use authenticated POST `/api/search` with a JSON request body matching `SearchHandler`. Keep response assertions behavior-focused so the test still proves browser-history artifacts are searchable when present and that empty browser-history state returns a valid response.

### Alternative Approaches Considered
1. Add GET `/api/search` as a compatibility endpoint. This increases API surface and requires a consumer impact sweep. It should only be selected if the search API spec explicitly supports GET.
2. Remove browser-history E2E search assertions. Rejected because it would weaken live-stack coverage and would not protect the connector certification scenario.

## Affected Files
- `tests/e2e/browser_history_e2e_test.go`
- Potentially `internal/api/search.go` tests if request body expectations need a shared helper
- No production API route change unless selected by an explicit consumer sweep

## Regression Test Design
- Targeted E2E regression: browser-history initial sync/search test sends POST `/api/search` and asserts a successful response shape.
- Adversarial regression: a guard or test assertion fails if any browser-history E2E search path uses `apiGet` or `http.MethodGet` for `/api/search`.
- Broader suite: `./smackerel.sh test e2e` after the targeted fix.

## Ownership
- Owning feature/spec: `specs/010-browser-history-connector`
- Fix owner: `bubbles.implement`
- Test owner: `bubbles.test`
- Validation owner: `bubbles.validate`
