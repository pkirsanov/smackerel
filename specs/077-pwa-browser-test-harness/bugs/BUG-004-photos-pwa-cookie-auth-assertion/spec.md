# Specification: BUG-077-004 Photos PWA Cookie-Auth Assertion

## Release Train

Target train: `mvp`. No feature flag is introduced.

## Requirements

- **FR-01:** The Photos connector wizard E2E test MUST assert the served script sends same-origin credentials.
- **FR-02:** The test MUST fail if the served script explicitly omits same-origin cookies.
- **FR-03:** The test MUST continue to assert both connector endpoints and included-album payload wiring.
- **FR-04:** The test MUST exercise the real served PWA and live connector API on the disposable stack.
- **FR-05:** Production PWA authentication behavior MUST remain unchanged.

### Single-Capability Justification

- **Classification:** This is a test-contract repair inside the existing Photos connector wizard and HttpOnly session capability. It does not add a photo provider, authentication mode, screen, or API variant.
- **Existing foundation and reuse path:** `web/pwa/photo-library-add.js` already uses `fetch` with `credentials: "same-origin"` so the browser sends the existing HttpOnly session cookie to the Photos connector endpoints. `tests/e2e/photos_pwa_test.go` continues to inspect the served script and exercise the live connector API, included-album payload, and provider response.
- **Consumer set:** The served Photos wizard, its connector create/list API calls, the included-album selection flow, and the Photos PWA E2E all consume the same cookie-authenticated contract.
- **Why no new abstraction or provider registry is needed:** Provider handling and session authentication already exist outside this assertion. The repair replaces one obsolete bearer-token expectation with the current same-origin cookie invariant; adding an auth strategy or provider registry would create a production variation that this bug explicitly forbids.
- **Residual risk:** The packet evidence records the opt-in Ollama agent E2E as not executed. That remains unproven broad coverage and is routed to `bubbles.test` by `TR-BUG-077-004-GOVERNANCE-001`; this analyst edit leaves the skip visible and makes no full-E2E or certification claim.

## Scenarios

```gherkin
Scenario: Served Photos wizard uses the HttpOnly session cookie
  Given the disposable Smackerel stack serves photo-library-add.js
  When the Photos wizard E2E contract inspects the served script
  Then the script contains credentials set to same-origin
  And both connector endpoints and included-album wiring remain present

Scenario: Omitted cookies fail the regression
  Given the Photos wizard depends on an HttpOnly same-origin session cookie
  When the served script contains credentials set to omit
  Then the E2E contract fails
```

## Acceptance Criteria

- The focused live E2E test passes after failing on the stale `Authorization` assertion.
- The complete root E2E package passes.
- No production source, auth middleware, config, deployment, or secret changes are made.

## Product Principle Alignment

This change supports Principle 8, Trust Through Transparency: the test describes the actual authentication boundary and fails on a real cookie-omission regression instead of enforcing obsolete bearer-token text.