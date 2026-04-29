# Scopes: BUG-010-003 Browser-history E2E search method drift

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Align browser-history E2E search consumer

**Status:** In Progress
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-010-003 prevent browser-history E2E search method drift
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

### Implementation Plan
1. Add or reuse an authenticated POST helper for E2E search requests.
2. Replace browser-history E2E `/api/search?...` GET call sites with supported POST requests.
3. Preserve assertions for status, response parsing, and browser-history source/content fields.
4. Add an adversarial regression guard that catches a return to GET `/api/search` in this E2E surface.
5. Run targeted Go E2E for browser-history and the broader E2E suite through the repo CLI.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-BUG-010-003-01 | Browser-history E2E search uses POST contract | e2e-api | `tests/e2e/browser_history_e2e_test.go` | Browser-history search checks call supported POST `/api/search` and receive successful live-stack responses | BUG-010-003-SCN-001 |
| T-BUG-010-003-02 | Regression E2E: stale GET search consumer is rejected | e2e-api | `tests/e2e/browser_history_e2e_test.go` or a repo guard owned by the test phase | Fails if browser-history E2E search calls use GET `/api/search` again | BUG-010-003-SCN-002 |
| T-BUG-010-003-03 | Broader E2E suite | e2e-api | `./smackerel.sh test e2e` | Browser-history method drift no longer appears in the broad live-stack suite | BUG-010-003-SCN-001 |

### Definition of Done
- [x] Root cause confirmed and documented with pre-fix evidence
  - **Phase:** implement
  - **Claim Source:** executed
  - **Evidence:** `timeout 900 ./smackerel.sh test e2e --go-run 'TestBrowserHistory_E2E_(InitialSyncProducesArtifacts|SocialMediaAggregateInStore|HighDwellArticleSearchable)$'` exited 1 before the fix. All three selected browser-history tests failed with `search returned 405`, matching the router contract where authenticated search is registered as POST-only.
- [x] Browser-history E2E search requests use the supported API method
  - **Phase:** implement
  - **Claim Source:** executed
  - **Evidence:** `tests/e2e/browser_history_e2e_test.go` now routes browser-history search checks through `apiPostJSON(cfg, "/api/search", req)` and parses the current `results`, `total_candidates`, and `search_mode` response fields. The focused selector below exited 0 after the fix.
- [x] Pre-fix regression test fails for the stale GET consumer
  - **Phase:** implement
  - **Claim Source:** executed
  - **Evidence:** Pre-fix focused selector exited 1 and reported `search returned 405` for `TestBrowserHistory_E2E_InitialSyncProducesArtifacts`, `TestBrowserHistory_E2E_SocialMediaAggregateInStore`, and `TestBrowserHistory_E2E_HighDwellArticleSearchable`.
- [x] Adversarial regression case exists and would fail if GET `/api/search` returned to browser-history E2E search
  - **Phase:** implement
  - **Claim Source:** executed
  - **Evidence:** `TestBrowserHistory_E2E_SearchRequestsUsePOSTContract` scans the browser-history E2E source for stale `apiGet(.../api/search?)` or `http.MethodGet` search use. It passed in `timeout 3600 ./smackerel.sh test e2e`, and `timeout 300 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/browser_history_e2e_test.go` exited 0 with an adversarial signal detected.
- [x] Post-fix targeted browser-history E2E regression passes
  - **Phase:** implement
  - **Claim Source:** executed
  - **Evidence:** `timeout 900 ./smackerel.sh test e2e --go-run 'TestBrowserHistory_E2E_(InitialSyncProducesArtifacts|SocialMediaAggregateInStore|HighDwellArticleSearchable)$'` exited 0 after the fix; all three selected browser-history tests passed.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  - **Phase:** implement
  - **Claim Source:** executed
  - **Evidence:** `TestBrowserHistory_E2E_InitialSyncProducesArtifacts`, `TestBrowserHistory_E2E_SocialMediaAggregateInStore`, and `TestBrowserHistory_E2E_HighDwellArticleSearchable` exercise the supported POST search contract for the changed browser-history searches; `TestBrowserHistory_E2E_SearchRequestsUsePOSTContract` covers the stale GET regression scenario.
- [ ] Broader E2E regression suite passes
  - **Phase:** implement
  - **Claim Source:** executed
  - **Evidence:** `timeout 3600 ./smackerel.sh test e2e` completed with exit 1. Browser-history search-method drift did not reappear, but broad Go E2E failed in `TestE2E_DomainExtraction` and `TestOperatorStatus_RecommendationProvidersEmptyByDefault`.
- [x] Regression tests contain no silent-pass bailout patterns
  - **Phase:** implement
  - **Claim Source:** executed
  - **Evidence:** The browser-history search tests now fail on non-200 responses, missing response fields, empty `search_mode`, over-limit results, and malformed result/detail fields. The old response-shape bailout skips are absent; the only remaining skips in the file are live-stack environment gates.
- [x] Consumer impact sweep confirms first-party `/api/search` consumers are aligned with the selected API contract
  - **Phase:** implement
  - **Claim Source:** executed
  - **Evidence:** Workspace search found no `api/search?` query-string consumers under `tests/**`, `internal/**`, or `web/**`. Current first-party search callers use POST JSON, including shell E2E helpers, Go E2E callers, stress search, and Telegram API clients; no production API compatibility route was added.
- [ ] Bug marked as Fixed in bug.md by the validation owner
  - **Phase:** implement
  - **Claim Source:** not-run
  - **Evidence:** Implement did not edit `bug.md` status or validation certification fields.
