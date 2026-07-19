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