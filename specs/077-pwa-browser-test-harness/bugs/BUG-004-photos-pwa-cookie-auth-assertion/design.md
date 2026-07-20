# Design: BUG-077-004 Photos PWA Cookie-Auth Assertion

## Root Cause Analysis

`web/pwa/photo-library-add.js` intentionally authenticates through an HttpOnly `auth_token` cookie attached by `fetch(..., { credentials: "same-origin" })`. JavaScript cannot and should not read that cookie to construct an `Authorization` header. The E2E test retained an older literal-source assertion after spec 100 migrated the PWA auth model.

## Change

Update only `tests/e2e/photos_pwa_test.go`:

1. Require `credentials: "same-origin"` in the served wizard script.
2. Reject `credentials: "omit"` adversarially.
3. Preserve endpoint, payload, live API response, and Immich-provider assertions.

## Safety

- No production code changes.
- No auth, cookie, endpoint, schema, configuration, deployment, or secret changes.
- The negative assertion catches the concrete regression that would prevent the browser from sending the HttpOnly session cookie.

## Alternatives Rejected

- Adding an `Authorization` header to production JavaScript would expose or duplicate credential handling and contradict the HttpOnly-cookie design.
- Removing authentication assertions entirely would leave the original regression class uncovered.

### Single-Implementation Justification

- **Existing owning abstraction:** The served PWA relies on the existing same-origin HttpOnly `auth_token` session boundary. `web/pwa/photo-library-add.js` centralizes Photos wizard POSTs in `post()`, where `credentials: "same-origin"` attaches that cookie without exposing it to JavaScript.
- **Concrete implementations:** The current wizard posts the Immich connector payload to `/v1/photos/connectors/test` and `/v1/photos/connectors`; `tests/e2e/photos_pwa_test.go` inspects the served script and exercises the live authenticated connector list. This repair changes only the assertion contract.
- **Current consumers:** The Photos add wizard, test-connection action, connector-create action, included-album payload flow, live connector API, and Photos PWA E2E all consume the same cookie-authenticated path.
- **Bounded variation axes:** The POST target varies between test and create, and album scope varies between included and excluded selections. Provider variation remains owned by the existing Photos connector contract; cookie transport does not vary in this bug.
- **Extension path:** Another Photos provider continues through the existing connector request shape and authenticated endpoints. Another same-origin PWA action uses the existing cookie session invariant; it does not add an auth strategy or browser credential provider.
- **Foundation decision:** This is a stale test assertion inside one established Photos/session path, with no new provider, authentication mode, screen, or API contract. A provider registry or auth-strategy abstraction would create production variation that the fix explicitly does not require.
- **Residual coverage risk:** The packet's opt-in Ollama agent E2E skip remains an unexecuted coverage risk. This design remediation does not convert that skip into a pass or claim coverage for Ollama-dependent behavior.

## Complexity Tracking

None - the smallest viable repair aligns one E2E assertion with the existing HttpOnly same-origin cookie contract.