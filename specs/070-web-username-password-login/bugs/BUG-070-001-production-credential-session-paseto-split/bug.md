# Bug: [BUG-070-001] Production Credential Session and PASETO Split

## Summary

Username/password login is accepted by the production legacy web surface, but the resulting cookie represents the shared runtime token rather than a per-user PASETO accepted by modern `/api` and `/v1` middleware.

## Severity

High (S2): login appears successful while multiple primary authenticated surfaces remain unusable.

## Status

Reported. The reproduction below is operator-supplied current-session historical input and was not executed by `bubbles.bug` in this planning-only invocation.

## Reproduction Provenance

- **Claim Source:** interpreted historical input supplied by the operator on 2026-07-23.
- **Execution in this invocation:** none.
- **Environment described by the input:** production on `<deploy-host>` (target name normalized per the product deployment boundary).
- **Certainty boundary:** the observed split is preserved as reported; technical root cause remains design-owned and unconfirmed by this packet.

## Reproduction Steps

1. On `<deploy-host>` production, open the username/password login page.
2. Submit valid credentials while production shared-token fallback is false.
3. Observe that login succeeds and legacy server-rendered pages load.
4. Navigate to Assistant, Connectors, Photos, model picker/admin, or another modern `/api` or `/v1` PASETO-protected surface.
5. Observe that the modern middleware rejects the cookie as malformed because it contains or represents the shared runtime token.

## Expected Behavior

One successful username/password login establishes a production-valid, per-user, HttpOnly cookie consumed consistently by every server-rendered, PWA, `/api`, and `/v1` surface. Shared-token fallback remains disabled and no token value is exposed to browser scripts, logs, URLs, or rendered content.

## Actual Behavior

Legacy pages accept the post-login cookie, while modern PASETO middleware rejects it. Assistant, Connectors, Photos, model picker/admin, and related API-backed experiences fail after the UI has already presented login as successful.

## Outcome Contract

**Intent:** Make username/password login establish one browser session with one production-valid per-user identity across all authenticated Smackerel surfaces.

**Success Signal:** A real browser logs in once with valid credentials, opens both legacy pages and representative modern PWA/API surfaces without a second credential, and receives authorized responses until explicit logout, expiry, or revocation.

**Hard Constraints:** The cookie is HttpOnly and same-origin; no shared-token fallback is enabled; no shared runtime token or per-user token is exposed to JavaScript, storage, URLs, logs, or response bodies; invalid, expired, malformed, and revoked sessions fail closed with non-enumerating user feedback.

**Failure Condition:** The bug remains open if login succeeds on only one renderer or middleware family, if any modern surface still rejects the issued session, if compatibility depends on shared-token fallback, or if a credential/token becomes visible outside the server-side session boundary.

## Impact

- Blocks authenticated modern API/PWA Playwright journeys required by `specs/106-coherent-product-experience`.
- Blocks product-level journey acceptance in `BUG-102-001-product-journey-acceptance-gap`.
- Makes a successful login response misleading because the accepted session is not product-wide.

## Root Cause Ownership

`bubbles.design` must confirm the exact issuer, cookie encoding, middleware validation, actor lookup, expiry, revocation, and logout path before implementation. This packet records the observed trust-model split without certifying an unexecuted root cause.

## Related

- Parent feature: `specs/070-web-username-password-login/`
- Product experience dependency: `specs/106-coherent-product-experience/`
- Deployment acceptance dependency: `specs/102-target-deploy-hardening/bugs/BUG-102-001-product-journey-acceptance-gap/`
- Existing regression surface: `tests/unit/web/bug_077_002_login_session_reuse_test.sh`
