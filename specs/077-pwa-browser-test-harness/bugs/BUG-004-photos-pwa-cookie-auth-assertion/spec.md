# Specification: BUG-077-004 Photos PWA Cookie-Auth Assertion

## Release Train

Target train: `mvp`. No feature flag is introduced.

## Requirements

- **FR-01:** The Photos connector wizard E2E test MUST assert the served script sends same-origin credentials.
- **FR-02:** The test MUST fail if the served script explicitly omits same-origin cookies.
- **FR-03:** The test MUST continue to assert both connector endpoints and included-album payload wiring.
- **FR-04:** The test MUST exercise the real served PWA and live connector API on the disposable stack.
- **FR-05:** Production PWA authentication behavior MUST remain unchanged.

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