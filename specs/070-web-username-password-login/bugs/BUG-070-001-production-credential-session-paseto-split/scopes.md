# Scopes: BUG-070-001 Unified Production Browser Session

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scenario-manifest.json](scenario-manifest.json) | [test-plan.json](test-plan.json) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Execution Outline

### Phase Order

1. **Scope 01 - Bound browser-account policy and role/grant model:** persist the canonical principal, the `daily-user`/`operator` role definitions, and explicit browser grants required before password verification may issue a session.
2. **Scope 02 - Purpose-bound session lifecycle:** issue, verify, persist, and revoke browser and API PASETOs with carrier-specific audiences.
3. **Scope 03 - Unified request authentication:** make legacy web, `/api`, and `/v1` middleware consume one authenticated `auth.Session` from one authoritative session contract (AUTH-015) without shared-token fallback.
4. **Scope 04 - Login, recovery, and logout UX:** wire the real form and session lifecycle to non-enumerating, safe-return, accessible browser states.
5. **Scope 05 - Product-wide CSRF and Origin mutation protection:** require a trusted same-origin context and a session-bound anti-CSRF proof for every cookie-authenticated mutation family, returning 403 before any state change.
6. **Scope 06 - Role/grant acceptance and global-corpus gating (disposable production):** prove one real login cookie reaches all representative product surfaces, the daily-user/operator 2xx/403 matrix holds, and the single operator-owned global corpus is grant-gated, before Assistant API acceptance or downstream journey certification.

Each scope is gated by the preceding scope. No Assistant API/PWA acceptance work starts before Scopes 01-05 establish the unified production session, CSRF/Origin protection, and the role/grant model.

### New Types And Signatures

- `BrowserAccountStore.VerifyAndResolve(ctx, username, password) (BrowserAccount, error)` where `BrowserAccount` contains a bound `auth_user_id` and validated browser grants.
- `IssueOptions.Audience` and `ParsedToken.Audience` with closed purposes `browser_session`, `api_bearer`, and bounded header-only `legacy_api_bearer`.
- `SessionIssuer.IssueBrowserSession(ctx, principal, scopes, source) (IssuedToken, error)`.
- `RequestAuthenticator.Authenticate(*http.Request) (auth.Session, AuthFailureKind)` with carrier-selected audience enforcement.
- `SessionRevoker.RevokeCurrent(ctx, session, actor, reason) error`.
- Additive persistence for `web_user_credentials.auth_user_id`, `auth_user_scope_grants`, `auth_tokens.token_purpose`, and explicit issued sources.
- Role model in `BrowserSessionPolicy`: `daily-user` and `operator` role constants with explicit persisted grant sets and no wildcard or implicit default grant (AUTH-012/AUTH-014).
- `MutationGuard.RequireTrustedContext(*http.Request, auth.Session) (MutationTrustResult, error)` (or equivalent) enforcing a trusted same-origin Origin plus a session-bound anti-CSRF proof for every cookie-authenticated mutation family; SameSite alone is insufficient (AUTH-011). The concrete session-bound-proof mechanism is a design-owned residual (see [report.md](report.md) finding RF-070-001-01).

### Validation Checkpoints

- **After Scope 01:** migration/repository canaries prove no identity or grant inference, the `daily-user`/`operator` role definitions and grants persist, and atomic provisioning rollback.
- **After Scope 02:** issuer/verifier/revoker canaries prove audience, carrier, expiry, persistence, and replay rejection before middleware changes.
- **After Scope 03:** one legacy page, one `/api` route, and one `/v1` route receive the same session from one authoritative contract; an operator-denied route remains 403.
- **After Scope 04:** real login/logout/recovery browser tests pass without interception, bearer injection, auth-state injection, or bailout returns.
- **After Scope 05:** every cookie-authenticated mutation family returns 403 before mutation on missing/stale/mismatched/cross-origin proof and proceeds only with valid session-bound proof.
- **After Scope 06:** the disposable production-mode journey proves all named surfaces, the daily-user/operator 2xx/403 matrix, leak-free global-corpus grant gating, and privacy states before implementation is routed onward.

## Scope Inventory

| # | Scope | Depends On | Surfaces | Test Rows | Status |
|---|---|---|---|---:|---|
| 01 | Bound browser-account policy and role/grant model | None | PostgreSQL, auth repository, provisioning, role/grant persistence, login rejection | 5 | Not Started |
| 02 | Purpose-bound session lifecycle | 01 | PASETO claims, token store, revocation, cookie policy | 5 | Not Started |
| 03 | Unified request authentication | 02 | Legacy web, `/api`, `/v1`, scope gates, telemetry | 5 | Not Started |
| 04 | Login, recovery, and logout UX | 03 | Login page, safe return, logout, accessibility | 5 | Not Started |
| 05 | Product-wide CSRF and Origin mutation protection | 04 | Server forms, HTMX, PWA fetch, JSON, Cards, admin mutations | 5 | Not Started |
| 06 | Role/grant acceptance and global-corpus gating (disposable production) | 05 | Full browser shell, daily-user/operator route matrix, global corpus | 5 | Not Started |

## Shared Authentication Infrastructure Impact Sweep

The affected auth foundation is high fan-out. Implementation must enumerate and preserve these consumers before broad changes: `internal/api/web_login.go`, `internal/api/router.go`, `internal/api/sanitize_next.go`, `internal/auth/issue.go`, `internal/auth/verify.go`, `internal/auth/bearer_store.go`, `internal/auth/revocation/**`, `internal/auth/webcreds/**`, `cmd/core/wiring.go`, `cmd/core/cmd_users.go`, `internal/api/web_register.go`, invitation provisioning, legacy HTML navigation, HTMX requests, PWA same-origin fetches, `/api`, `/v1`, `auth.RequireScope`, operator gates, Cards, Assistant, Graph/Wiki, Connectors, Photos, and model-picker/admin routes.

Independent canaries run in that order: account binding/grants, token purpose, password-to-cookie persistence, middleware parity, logout/replay, focused real-browser auth, then broader authenticated journeys. A failing canary stops the sequence; broad suites cannot launder a failed auth foundation.

### Shared-Auth Rollback And Restore

- Keep additive schema and authorization history; rollback never drops bindings, grants, token purpose, revocation records, or audit data.
- Disable production password login with an explicit unavailable response if unified issuance/authentication must be withdrawn.
- Reject and clear browser-purpose cookies during rollback; bulk-revoke suspect browser JTIs through the canonical revocation path.
- Keep machine Authorization-header authentication available under its existing API audience.
- Never restore shared-token cookies, production fallback, wrong-purpose acceptance, empty-token web bypass, or split middleware verification.
- Prove rollback with an independent canary: password login is unavailable, browser replay is rejected, and a valid API-header client remains accepted.

## Change Boundary

**Allowed file families:** the auth/session persistence and migration files named by `design.md`; focused account/token/middleware/login tests; real disposable-stack auth Playwright support; directly affected auth operator/API documentation.

**Excluded surfaces:** unrelated feature behavior; any packet outside this bug directory; `specs/079-prod-autonomous-supervisor/**`; operator-owned deploy manifests; release-train configuration; production data; unrelated shared fixtures; and business requirements or design text. Collateral cleanup is not implicit.

---

## Scope 01: Bound Browser-Account Policy And Role/Grant Model

**Status:** Not Started  
**Depends On:** None  
**Scope-Kind:** runtime-behavior  
**foundation:** true

### Gherkin Scenarios

```gherkin
Scenario: SCN-070-001-02 Invalid or unbound credentials create no partial session
	Given an unknown username, wrong password, disabled account, malformed password record, or credential row without an explicit active principal and valid browser grants
	When production username/password login is submitted
	Then every credential-class rejection presents the same non-enumerating response
	And no browser token is persisted or accepted cookie is created
	And a repository or issuer failure is presented as temporarily unavailable rather than invalid credentials
```

### Implementation Plan

1. Add the additive credential-principal, grant, token-purpose, and invite metadata migrations described in `design.md`; leave historical credential bindings nullable only as an explicit migration state and add no database defaults.
2. Replace `internal/auth/webcreds/repo.go::VerifyAndTouch` as the login boundary with `BrowserAccountStore.VerifyAndResolve`, preserving Argon2 timing parity while returning only an active canonical principal and validated grants.
3. Extend `internal/auth/bearer_store.go` with transactional principal/grant reads and writes; validate every grant with `auth.ValidateScopeName`, register the existing `assistant:turn` surface without wildcard inference, and define the `daily-user` and `operator` roles as explicit persisted grant sets (AUTH-012/AUTH-014) with no wildcard or implicit default grant.
4. Make `cmd/core/cmd_users.go`, `internal/api/web_register.go`, and `internal/auth/webinvite/**` provision principal, credential binding, and grants atomically; production static-registration secrets cannot create session-ready accounts.
5. Add a value-safe migration readiness command that reports only bound, unbound, and invalid-grant counts and refuses guessed usernames or grants.
6. Preserve account/invite state on rollback: a failed transaction leaves no principal-only, credential-only, or grant-only residue.

### Shared Infrastructure Impact Sweep

- Protected contracts: auth principal IDs, grant validation, invitation consumption, password rotation, login timing parity, account-disable revocation, migration audit output, and CLI provisioning.
- Canary boundary: direct repository tests and a real PostgreSQL transaction test must pass before token issuance or HTTP middleware is changed.
- Rollback: retain additive columns/tables, disable unbound production login, and restore the prior application binary only if it cannot re-enable shared-token issuance.

### Test Plan

| ID | Test Type | Category | File / Symbol | Scenario | Exact Test Title / Required Assertion | Command | Live |
|---|---|---|---|---|---|---|---|
| AUTH-S01-T01 | Unit | `unit` | `internal/auth/webcreds/repo_test.go::TestVerifyAndResolveBrowserAccount` and `internal/auth/scope_registry_test.go` | SCN-070-001-02 | `TestVerifyAndResolveBrowserAccountRejectsUnknownWrongDisabledUnboundAndInvalidGrantWithoutIdentityLeak` | `./smackerel.sh test unit --go` | No |
| AUTH-S01-T02 | Integration red-to-green | `integration` | `tests/integration/auth/browser_account_binding_test.go` | SCN-070-001-02 | `TestProductionCredentialBindingIsAtomicAndNeverInfersPrincipalOrScopes`; before fix, the real unbound row is wrongly session-eligible or lacks the required binding contract; after fix it is rejected with zero token rows | `./smackerel.sh test integration` | Yes |
| AUTH-S01-T03 | Regression API | `e2e-api` | `tests/e2e/auth/browser_account_provisioning_test.go` | SCN-070-001-02 | `TestInviteProvisioningRoundTripCreatesBoundPrincipalAndExplicitGrantsWhileUnboundLoginCreatesNoSession` | `./smackerel.sh test e2e` | Yes |
| AUTH-S01-T04 | Regression UI | `e2e-ui` | `web/pwa/tests/auth_account_binding.spec.ts` | SCN-070-001-02 | `invalid disabled and unbound accounts share one visible rejection and create no authenticated browser session` | `./smackerel.sh test e2e-ui` | Yes |
| AUTH-S01-T05 | Broader browser regression | `e2e-ui` | `web/pwa/tests/auth_registration_login.spec.ts` | SCN-070-001-02 | `a real invited account registers then signs in while a failed provisioning transaction leaves no usable partial account` | `./smackerel.sh test e2e-ui` | Yes |

All live rows use the disposable test stack and real PostgreSQL. Playwright uses the real registration/login UI and browser cookie jar with no `page.route`, `context.route`, auth injection, storage-state token injection, or conditional bailout.

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-070-001-02 Invalid or unbound credentials create no partial session`: all credential-class failures are non-enumerating, create no persisted browser token or accepted cookie, and distinguish infrastructure unavailability without leaking account state. Evidence: [report.md#scope-01-core](report.md#scope-01-core).
- [ ] Credential verification resolves only an explicitly bound active principal and validated explicit grants; no username, route, wildcard, or prior shared-token privilege is inferred. Evidence: [report.md#scope-01-core](report.md#scope-01-core).
- [ ] The `daily-user` and `operator` roles are defined as explicit persisted grant sets (AUTH-012/AUTH-014); operator authority is granted, never inferred from possession of a valid credential, and no wildcard or implicit default grant exists. Evidence: [report.md#scope-01-core](report.md#scope-01-core).
- [ ] Provisioning and invite consumption atomically create principal, credential binding, and grants, while password rotation preserves binding and grants. Evidence: [report.md#scope-01-core](report.md#scope-01-core).
- [ ] Migration readiness reports value-safe counts and production login refuses unbound rows without partial state. Evidence: [report.md#scope-01-core](report.md#scope-01-core).

#### Test Evidence - 5 Rows / 5 Items

- [ ] `AUTH-S01-T01` unit evidence passes. Evidence: [report.md#auth-s01-t01](report.md#auth-s01-t01).
- [ ] `AUTH-S01-T02` adversarial integration evidence records the pre-fix failure and post-fix pass. Evidence: [report.md#auth-s01-t02](report.md#auth-s01-t02).
- [ ] `AUTH-S01-T03` real e2e-api provisioning and login round trip passes. Evidence: [report.md#auth-s01-t03](report.md#auth-s01-t03).
- [ ] `AUTH-S01-T04` real e2e-ui rejection matrix passes without interception or injected auth. Evidence: [report.md#auth-s01-t04](report.md#auth-s01-t04).
- [ ] `AUTH-S01-T05` broader registration/login browser regression passes. Evidence: [report.md#auth-s01-t05](report.md#auth-s01-t05).

#### Build Quality Gate

- [ ] Shared-account canary, rollback check, repo-standard build/check/lint/format, migration validation, artifact lint, traceability guard, privacy scan, and directly affected docs are clean with zero warnings or deferrals. Evidence: [report.md#scope-01-quality](report.md#scope-01-quality).

---

## Scope 02: Purpose-Bound Session Lifecycle

**Status:** Not Started  
**Depends On:** Scope 01  
**Scope-Kind:** runtime-behavior  
**foundation:** true

### Gherkin Scenarios

```gherkin
Scenario: SCN-070-001-03 Malformed expired revoked and wrong-purpose sessions fail closed
	Given a browser cookie is malformed, expired, revoked, signed by an unknown key, issued for the API audience, contains no audience, or has the shared runtime token shape
	When the canonical session verifier evaluates it as a browser carrier
	Then validation denies it with a bounded failure class
	And no legacy or modern consumer can reinterpret it as authenticated

Scenario: SCN-070-001-04 Session material remains private
	Given a bound account successfully receives a browser-purpose session
	When the token is persisted and the cookie is written
	Then the cookie is HttpOnly, same-origin, Secure in production, SameSite Lax, path-wide, and expiry-aligned
	And no password, token, cookie, shared secret, username, claims body, or raw safe-return value is emitted to client-visible output or telemetry
```

### Implementation Plan

1. Extend `internal/auth/issue.go::IssueOptions` and `IssueAndPersistToken` to require an audience/purpose and explicit issued source; persist browser sessions with JTI, expiry, scope snapshot, purpose, and HMAC-at-rest metadata.
2. Extend `internal/auth/verify.go::ParsedToken` and `VerifyAndParse` to parse signed audience while preserving key rotation, issuer, `nbf`, `iat`, `exp`, footer `kid`, and scope validation.
3. Add `BrowserSessionPolicy` for the closed audiences, carrier rules, `auth_token` attributes, configured TTL, and bounded auth outcome vocabulary; do not add a hidden TTL or SameSite fallback.
4. Implement header-only compatibility for unexpired no-audience legacy API tokens; browser cookies require `smackerel-browser-session` without exception, and API tokens require explicit exchange before browser use.
5. Implement `SessionIssuer` and `SessionRevoker` over `internal/auth/bearer_store.go` and `internal/auth/revocation/**`; canonical DB revocation precedes successful logout, local cache updates immediately, and NATS remains convergence rather than truth.
6. Remove sensitive identity/grant labels from `auth.RequireScope` logs/metrics and prove bounded route/carrier/outcome telemetry.

### Shared Infrastructure Impact Sweep

- Protected contracts: PASETO wire compatibility, key rotation, token persistence, revocation cache/NATS, token exchange, cookie attributes, machine headers, telemetry cardinality, and auth failure privacy.
- Canary boundary: issuer/verifier/revoker tests run before any middleware consumer switches to the new authenticator.
- Rollback: reject/clear browser sessions and keep API-header tokens; never downgrade browser audience checks or delete purpose history.

### Test Plan

| ID | Test Type | Category | File / Symbol | Scenario | Exact Test Title / Required Assertion | Command | Live |
|---|---|---|---|---|---|---|---|
| AUTH-S02-T01 | Unit | `unit` | `internal/auth/issue_test.go`, `internal/auth/verify_test.go`, `internal/api/browser_session_policy_test.go` | SCN-070-001-03, SCN-070-001-04 | `TestBrowserSessionAudienceCarrierCookieExpiryAndPrivacyMatrix` | `./smackerel.sh test unit --go` | No |
| AUTH-S02-T02 | Integration red-to-green | `integration` | `tests/integration/auth/browser_session_lifecycle_test.go` | SCN-070-001-03 | `TestSharedAndAPITokenCookiesFailWhilePersistedBrowserTokenRevocationRejectsReplay`; before fix the password flow persists no browser-purpose token and writes the shared token, after fix only the persisted browser audience survives | `./smackerel.sh test integration` | Yes |
| AUTH-S02-T03 | Regression API | `e2e-api` | `tests/e2e/auth/browser_token_exchange_test.go` | SCN-070-001-03 | `TestAPIHeaderTokenExchangesForNewBrowserSessionButFailsWhenCopiedDirectlyIntoCookie` | `./smackerel.sh test e2e` | Yes |
| AUTH-S02-T04 | Regression UI | `e2e-ui` | `web/pwa/tests/auth_session_privacy.spec.ts` | SCN-070-001-04 | `real login keeps password and session material out of DOM URLs storage responses and console` | `./smackerel.sh test e2e-ui` | Yes |
| AUTH-S02-T05 | Broader browser regression | `e2e-ui` | `web/pwa/tests/auth_session_expiry_revocation.spec.ts` | SCN-070-001-03 | `malformed expired revoked wrong-purpose unknown-key and shared-token-shaped cookies all require re-authentication` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-070-001-03 Malformed expired revoked and wrong-purpose sessions fail closed`: every malformed, expired, revoked, unknown-key, wrong-audience, no-audience, API-token, and shared-token-shaped browser cookie is denied consistently. Evidence: [report.md#scope-02-core](report.md#scope-02-core).
- [ ] `SCN-070-001-04 Session material remains private`: explicit cookie attributes hold and no password, token, cookie, shared secret, identity, claims body, or raw safe-return value enters client-visible output or telemetry. Evidence: [report.md#scope-02-core](report.md#scope-02-core).
- [ ] Newly issued browser and API tokens carry explicit signed audiences and persisted purposes/sources; no-audience compatibility is header-only and expiry-bounded. Evidence: [report.md#scope-02-core](report.md#scope-02-core).
- [ ] Browser cookie attributes and lifetime derive from the explicit policy and signed expiry, with no token copied into a response body or script-readable state. Evidence: [report.md#scope-02-core](report.md#scope-02-core).
- [ ] Revocation is DB-canonical, cache-immediate, broadcast-convergent, and privacy-safe. Evidence: [report.md#scope-02-core](report.md#scope-02-core).

#### Test Evidence - 5 Rows / 5 Items

- [ ] `AUTH-S02-T01` unit lifecycle matrix passes. Evidence: [report.md#auth-s02-t01](report.md#auth-s02-t01).
- [ ] `AUTH-S02-T02` adversarial integration evidence records red then green. Evidence: [report.md#auth-s02-t02](report.md#auth-s02-t02).
- [ ] `AUTH-S02-T03` real e2e-api exchange and wrong-carrier regression passes. Evidence: [report.md#auth-s02-t03](report.md#auth-s02-t03).
- [ ] `AUTH-S02-T04` real e2e-ui privacy regression passes without interception or injected auth. Evidence: [report.md#auth-s02-t04](report.md#auth-s02-t04).
- [ ] `AUTH-S02-T05` broader adversarial browser lifecycle regression passes. Evidence: [report.md#auth-s02-t05](report.md#auth-s02-t05).

#### Build Quality Gate

- [ ] Token-lifecycle canary, auth rollback check, repo-standard build/check/lint/format, migration validation, artifact lint, traceability guard, secret/privacy scans, and affected auth docs are clean with zero warnings or deferrals. Evidence: [report.md#scope-02-quality](report.md#scope-02-quality).

---

## Scope 03: Unified Request Authentication

**Status:** Not Started  
**Depends On:** Scope 02  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-070-001-06 Authenticated no-data remains a normal empty state
	Given the unified authenticator accepts the user and an authorized collection has no records
	When a modern surface reads that collection
	Then the route returns its true authorized empty state
	And it does not project authentication failure access denial or dependency degradation

Scenario: SCN-070-001-07 Downstream failure does not invalidate a valid session
	Given the unified authenticator accepts the user and one downstream capability is degraded
	When the user opens that capability and then another healthy authenticated surface
	Then only the degraded capability reports its typed failure
	And the healthy surface remains authorized under the unchanged session
```

### Implementation Plan

1. Add `RequestAuthenticator.Authenticate` in `internal/auth/**` to enforce header precedence, carrier-selected audience, signature/time/key checks, revocation, and one `auth.Session` result without per-request PostgreSQL lookup.
2. Refactor `internal/api/router.go::bearerAuthMiddleware` and `webAuthMiddleware` to call the same authenticator and attach the same session; retain only presentation differences for top-level HTML, HTMX/fetch, and API failures.
3. Remove the legacy constant-time shared-token comparison, production empty-token bypass, and direct cookie/header fallback from web middleware; keep public PWA assets public while every protected data read authenticates.
4. Preserve `auth.RequireScope` and operator gates as authorization after authentication so 401, 403, true empty, filtered empty, degraded dependency, and ordinary errors remain structurally distinct.
5. Wire the account store, issuer, authenticator, and revoker fail-loud in `cmd/core/wiring.go` from existing auth SST.
6. Add bounded request/carrier/surface/outcome metrics and traces without username, user ID, token ID for unverified input, token scopes, cookie, or claims body.

### Shared Infrastructure Impact Sweep

- Protected consumers: every legacy route group, PWA data client, Assistant turn route, Graph/Wiki, Cards, Connectors, Photos, model/admin, annotation, operator gate, and API-header client.
- Canary boundary: compare `Session.UserID`, JTI, scopes, and failure class across one legacy, one `/api`, and one `/v1` route before enabling broader consumers.
- Rollback: switch production password login to unavailable and reject browser cookies; never restore split middleware or shared fallback.

### Test Plan

| ID | Test Type | Category | File / Symbol | Scenario | Exact Test Title / Required Assertion | Command | Live |
|---|---|---|---|---|---|---|---|
| AUTH-S03-T01 | Unit | `unit` | `internal/api/router_auth_middleware_test.go`, `internal/auth/request_authenticator_test.go` | SCN-070-001-06, SCN-070-001-07 | `TestUnifiedAuthenticatorPreservesCarrierPrecedenceSessionParityAnd401403EmptyDegradedSeparation` | `./smackerel.sh test unit --go` | No |
| AUTH-S03-T02 | Integration red-to-green | `integration` | `tests/integration/auth/middleware_parity_test.go` | SCN-070-001-06 | `TestOnePasswordLoginCookieAuthenticatesLegacyAPIAndV1WithIdenticalSession`; before fix the shared cookie passes legacy and fails modern, after fix all three accept the browser-purpose session | `./smackerel.sh test integration` | Yes |
| AUTH-S03-T03 | Regression API | `e2e-api` | `tests/e2e/auth/browser_middleware_parity_test.go` | SCN-070-001-06 | `TestBrowserCookieAuthorizesRepresentativeAPIAndV1ReadsAndPreserves401403AndEmptySemantics` | `./smackerel.sh test e2e` | Yes |
| AUTH-S03-T04 | Regression UI | `e2e-ui` | `web/pwa/tests/auth_middleware_parity.spec.ts` | SCN-070-001-06 | `one real login navigates a legacy page and modern PWA data route without a second credential` | `./smackerel.sh test e2e-ui` | Yes |
| AUTH-S03-T05 | Broader browser regression | `e2e-ui` | `web/pwa/tests/auth_empty_degraded_access.spec.ts` | SCN-070-001-06, SCN-070-001-07 | `authenticated true empty filtered empty degraded dependency access denied and healthy content remain distinguishable` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] Middleware parity and one authoritative session contract (AUTH-015): one browser cookie yields the same principal, JTI, and scope set across a legacy page, `/api`, and `/v1` from one `RequestAuthenticator`, while operator denial remains 403, and exactly one production username/password-to-browser-session contract is active. Evidence: [report.md#scope-03-core](report.md#scope-03-core).
- [ ] `SCN-070-001-06 Authenticated no-data remains a normal empty state`: a valid authorized empty read remains distinct from authentication failure, access denial, and degradation. Evidence: [report.md#scope-03-core](report.md#scope-03-core).
- [ ] `SCN-070-001-07 Downstream failure does not invalidate a valid session`: only the degraded capability fails and a healthy authenticated surface remains authorized under the unchanged session. Evidence: [report.md#scope-03-core](report.md#scope-03-core).
- [ ] Legacy web, `/api`, and `/v1` authentication consume one `RequestAuthenticator` and receive one session shape; all shared-token and empty-token web bypasses are absent in production. Evidence: [report.md#scope-03-core](report.md#scope-03-core).
- [ ] Header and cookie purposes remain distinct, malformed header precedence fails closed, and authorization remains downstream of identity verification. Evidence: [report.md#scope-03-core](report.md#scope-03-core).
- [ ] Authentication failure, access denial, true empty, filtered empty, degraded dependency, and ordinary errors remain distinct without sensitive telemetry. Evidence: [report.md#scope-03-core](report.md#scope-03-core).

#### Test Evidence - 5 Rows / 5 Items

- [ ] `AUTH-S03-T01` unit middleware matrix passes. Evidence: [report.md#auth-s03-t01](report.md#auth-s03-t01).
- [ ] `AUTH-S03-T02` adversarial trust-split integration evidence records red then green. Evidence: [report.md#auth-s03-t02](report.md#auth-s03-t02).
- [ ] `AUTH-S03-T03` real e2e-api parity regression passes. Evidence: [report.md#auth-s03-t03](report.md#auth-s03-t03).
- [ ] `AUTH-S03-T04` real e2e-ui legacy/modern parity regression passes without interception or auth injection. Evidence: [report.md#auth-s03-t04](report.md#auth-s03-t04).
- [ ] `AUTH-S03-T05` broader state-separation browser regression passes. Evidence: [report.md#auth-s03-t05](report.md#auth-s03-t05).

#### Build Quality Gate

- [ ] Middleware-parity canary, shared-auth rollback check, repo-standard build/check/lint/format, artifact lint, traceability guard, telemetry privacy checks, and affected route/auth docs are clean with zero warnings or deferrals. Evidence: [report.md#scope-03-quality](report.md#scope-03-quality).

---

## Scope 04: Login Recovery And Logout UX

**Status:** Not Started  
**Depends On:** Scope 03  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-070-001-05 Logout closes both trust paths
	Given one browser-purpose session authenticates legacy and modern surfaces
	When the user submits the real logout action
	Then canonical revocation commits before the product claims signed out and clears the matching cookie
	And browser Back direct legacy navigation and direct modern navigation all require re-authentication
	And a revocation-store failure presents retry without a false signed-out state

Scenario: SCN-070-001-08 Login and re-authentication are accessible and responsive
	Given a keyboard or screen-reader user at a 320 CSS-pixel viewport and 200 percent zoom
	When the user submits valid or invalid credentials or recovers from a rejected session
	Then labels focus status errors and the allowlisted safe-return action are perceivable and operable in logical order
	And protected content is removed from both visual and accessibility trees before recovery is shown
```

### UI Scenario Matrix

| Scenario | Preconditions | Real Browser Steps | Required Visible / Browser State | Regression Test |
|---|---|---|---|---|
| Valid login and safe return | Bound invited account; fallback false | Open `/login?next=<allowlisted>`; submit form once | `Signing in` then authorized destination; no success card or token field | `auth_login_recovery.spec.ts` |
| Credential rejection | Unknown and wrong-password runs | Submit each through the same form | Identical message; username retained; password cleared; no protected content | `auth_login_recovery.spec.ts` |
| Session recovery | Expired/revoked/malformed cookie | Open protected route | `Your session ended`; safe label; focus on recovery heading | `auth_login_recovery.spec.ts` |
| 403 authorization | Valid non-operator session | Open operator route | `You do not have access`; no login loop | `auth_login_recovery.spec.ts` |
| Logout success/failure | Valid session; then owned DB revocation failure case | Submit Logout | Truthful `Signing out`, signed-out or retry state; replay rejected only after success | `auth_logout.spec.ts` |
| Narrow accessible flow | 320px, 200% zoom, keyboard and accessibility snapshot | Login, reject, recover, logout | No overlap/horizontal scroll; persistent labels; correct focus/live regions | `auth_accessibility.spec.ts` |
| Hostile return | Cross-origin, query-bearing, fragment, unauthorized target | Submit login from each target | Authorized default without echoing raw rejected input | `auth_login_recovery.spec.ts` |

### Implementation Plan

1. Refactor `internal/api/web_login.go::HandleWebLogin` to resolve `BrowserAccount`, call `SessionIssuer`, write only the persisted browser token cookie after complete success, and redirect only to `internal/api/sanitize_next.go` output.
2. Change API-token login to explicit browser-session exchange: verify header-purpose input, resolve current grants, mint a new browser token, and never copy the supplied token into the cookie.
3. Implement logout through `SessionRevoker`: idempotently clear invalid/already-revoked cookies, but show a typed failure and retain truthful UI when canonical revocation of a valid session fails.
4. Update the server-rendered login/recovery/session menu presentation for initial, session-ended, submitting, invalid, rate-limited, service-unavailable, network-error, authorizing-destination, access-denied, signing-out, signed-out, and logout-failed states defined by `spec.md`.
5. Ensure safe return is sanitized before and after login, displayed only as a human product label, and re-authorized under the new principal.
6. Preserve password-manager semantics, one semantic POST, duplicate-submit prevention, persistent labels, linked errors, live status, focus restoration, 44px targets, and 320px/200% zoom behavior.

### Shared Infrastructure Impact Sweep

- Protected UI/auth contracts: login form field names, password managers, safe return, redirect status, HTMX/fetch 401, page 303, cookie clearing, session menu, browser Back, CSP, and existing invitation/rate-limit flows.
- Canary boundary: focused real-browser login/logout/recovery tests run before product-wide surface navigation.
- Rollback: present production login unavailable and clear/reject browser cookies; never expose token-entry fallback to ordinary users.

### Test Plan

| ID | Test Type | Category | File / Symbol | Scenario | Exact Test Title / Required Assertion | Command | Live |
|---|---|---|---|---|---|---|---|
| AUTH-S04-T01 | Unit | `unit` | `internal/api/web_login_credential_test.go`, `internal/api/web_login_page_test.go`, `internal/api/web_logout_test.go`, `internal/api/sanitize_next_test.go` | SCN-070-001-05, SCN-070-001-08 | `TestLoginRecoverySafeReturnCookieAndLogoutPresentationMatrix` | `./smackerel.sh test unit --go` | No |
| AUTH-S04-T02 | Integration red-to-green | `integration` | `tests/integration/auth/password_login_logout_test.go` | SCN-070-001-05 | `TestRealPasswordLoginPersistsBrowserSessionAndLogoutRevokesBeforeSuccess`; before fix login writes the shared token and logout cannot revoke its JTI, after fix replay fails across both middleware families | `./smackerel.sh test integration` | Yes |
| AUTH-S04-T03 | Regression API | `e2e-api` | `tests/e2e/auth/browser_login_logout_test.go` | SCN-070-001-05 | `TestLoginLogoutAndReplayAcrossLegacyAPIAndV1PreservesTypedRevocationFailure` | `./smackerel.sh test e2e` | Yes |
| AUTH-S04-T04 | Regression UI | `e2e-ui` | `web/pwa/tests/auth_login_recovery.spec.ts`, `web/pwa/tests/auth_logout.spec.ts` | SCN-070-001-05, SCN-070-001-08 | `real form login safe recovery and logout remain truthful with no auth injection interception or bailout` | `./smackerel.sh test e2e-ui` | Yes |
| AUTH-S04-T05 | Broader browser regression | `e2e-ui` | `web/pwa/tests/auth_accessibility.spec.ts` | SCN-070-001-08 | `login rejection session recovery and logout are keyboard screen-reader and narrow-viewport operable` | `./smackerel.sh test e2e-ui` | Yes |

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-070-001-05 Logout closes both trust paths`: canonical revocation commits before signed-out success, matching cookie attributes clear, and Back/direct replay fails across legacy and modern middleware while store failure stays retryable. Evidence: [report.md#scope-04-core](report.md#scope-04-core).
- [ ] `SCN-070-001-08 Login and re-authentication are accessible and responsive`: labels, focus, status, errors, safe return, and protected-content removal remain operable at 320px and 200% zoom for keyboard and screen-reader users. Evidence: [report.md#scope-04-core](report.md#scope-04-core).
- [ ] Password login and explicit API-token exchange mint a persisted browser-purpose token and never copy a shared/API token into the cookie. Evidence: [report.md#scope-04-core](report.md#scope-04-core).
- [ ] Logout revokes before success, clears matching attributes, rejects replay in both middleware families, and remains truthful on store failure. Evidence: [report.md#scope-04-core](report.md#scope-04-core).
- [ ] Login/recovery/access-denied/logout states satisfy safe-return, privacy, keyboard, screen-reader, and responsive contracts. Evidence: [report.md#scope-04-core](report.md#scope-04-core).

#### Test Evidence - 5 Rows / 5 Items

- [ ] `AUTH-S04-T01` unit presentation/lifecycle matrix passes. Evidence: [report.md#auth-s04-t01](report.md#auth-s04-t01).
- [ ] `AUTH-S04-T02` adversarial login/logout integration evidence records red then green. Evidence: [report.md#auth-s04-t02](report.md#auth-s04-t02).
- [ ] `AUTH-S04-T03` real e2e-api login/logout/replay regression passes. Evidence: [report.md#auth-s04-t03](report.md#auth-s04-t03).
- [ ] `AUTH-S04-T04` real e2e-ui login/recovery/logout regression passes without interception, auth injection, or bailout. Evidence: [report.md#auth-s04-t04](report.md#auth-s04-t04).
- [ ] `AUTH-S04-T05` broader accessibility browser regression passes. Evidence: [report.md#auth-s04-t05](report.md#auth-s04-t05).

#### Build Quality Gate

- [ ] Focused browser-auth canary, rollback check, repo-standard build/check/lint/format, artifact lint, traceability guard, regression-quality guards, accessibility checks, and affected user/auth docs are clean with zero warnings or deferrals. Evidence: [report.md#scope-04-quality](report.md#scope-04-quality).

---

## Scope 05: Product-Wide CSRF And Origin Mutation Protection

**Status:** Not Started  
**Depends On:** Scope 04  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-070-001-09 Cookie mutations enforce CSRF and Origin
	Given a daily user or operator holds a valid claim-bound browser session
	When a server form HTMX request PWA fetch JSON mutation Cards action or admin action is submitted with missing stale mismatched or cross-origin CSRF evidence
	Then the request returns 403 before any state change
	And SameSite alone is never accepted as sufficient evidence
	When the same permitted action carries valid session-bound CSRF evidence and trusted same-origin context
	Then it may proceed under the identity role and grants
```

### Implementation Plan

1. Add a `MutationGuard` (or equivalent middleware) applied to every cookie-authenticated mutation family — login, registration, logout, server forms, HTMX, PWA fetch, JSON, Cards, and admin — that requires a trusted same-origin Origin plus a server-validated session-bound anti-CSRF proof before any state change.
2. Keep the existing `SameSite=Lax`, same-origin fetch, CORS allowlist, and POST-only posture, but treat them as necessary-not-sufficient; the session-bound proof is the additional acceptance evidence AUTH-011 requires.
3. Bind the anti-CSRF proof to the claim-bound browser session issued by Scopes 02-03 so a stale, mismatched, or cross-session proof is rejected; never expose the shared runtime token or a per-user token to a client CSRF scheme.
4. Return a typed 403 before mutation that is distinct from 401 session rejection and from ordinary errors; missing, stale, mismatched, and cross-origin proof share one non-enumerating rejection.
5. Preserve the operator role's obligation to satisfy the same CSRF/Origin contract; an operator does not bypass request-forgery protection.
6. Record bounded telemetry (`outcome=origin_rejected|csrf_missing|csrf_stale|csrf_mismatch|accepted`, `family=form|htmx|pwa|json|cards|admin`) without token, cookie, or identity values.

> **Design residual (RF-070-001-01):** `design.md` §Security relies on `SameSite Lax` + same-origin/CORS and states "No token is exposed for a custom CSRF scheme," while `spec.md` AUTH-011 requires a server-validated **session-bound anti-CSRF proof** and declares SameSite alone insufficient. The concrete session-bound-proof mechanism is design-owned and MUST be specified by `bubbles.design` before implementation. This scope is planned to the spec requirement; the mechanism gap is routed, not resolved here.

### Shared Infrastructure Impact Sweep

- Protected contracts: every cookie-authenticated mutation route, the middleware chain order, existing CORS allowlist, CSP, SameSite posture, and login/logout/registration POST handlers.
- Canary boundary: a focused per-family CSRF/Origin unit and integration canary runs before broader journey regression.
- Rollback: if the mutation guard regresses a healthy path, disable the affected family's mutation and keep read paths; never fall back to SameSite-only acceptance for a mutation.

### Test Plan

| ID | Test Type | Category | File / Symbol | Scenario | Exact Test Title / Required Assertion | Command | Live |
|---|---|---|---|---|---|---|---|
| AUTH-S05-T01 | Unit | `unit` | `internal/api/web_login_test.go`, planned `internal/api/csrf_origin_middleware_test.go` | SCN-070-001-09 | `TestCookieMutationsRequireTrustedOriginAndSessionBoundCsrfProofAcrossEveryMutationFamily` | `./smackerel.sh test unit --go` | No |
| AUTH-S05-T02 | Integration red-to-green | `integration` | `tests/integration/auth/csrf_origin_enforcement_test.go` | SCN-070-001-09 | `TestEveryCookieMutationFamilyRejectsMissingStaleMismatchedAndCrossOriginProofBeforeStateChange`; before fix a mutation family accepts a forged or cross-origin request, after fix every family returns 403 before mutation | `./smackerel.sh test integration` | Yes |
| AUTH-S05-T03 | Regression API | `e2e-api` | `tests/e2e/auth/csrf_mutation_matrix_test.go` | SCN-070-001-09 | `TestLoginRegisterLogoutJsonAndAdminMutationsReturn403BeforeMutationOnForgedRequests` | `./smackerel.sh test e2e` | Yes |
| AUTH-S05-T04 | Regression UI | `e2e-ui` | `web/pwa/tests/auth_csrf_origin.spec.ts`, `web/pwa/tests/auth_login.spec.ts` | SCN-070-001-09 | `cross-origin stale and missing csrf submissions are blocked before mutation while valid same-origin proof proceeds` | `./smackerel.sh test e2e-ui` | Yes |
| AUTH-S05-T05 | Broader browser regression | `e2e-ui` | `web/pwa/tests/production_authenticated_product_journey.spec.ts`, `web/pwa/tests/auth_login.spec.ts` | SCN-070-001-09 | `csrf and origin protection holds across every mutation family in the broader authenticated journey` | `./smackerel.sh test e2e-ui` | Yes |

All live rows use the disposable test stack and real PostgreSQL; Playwright uses the real form and browser cookie jar with no `page.route`, `context.route`, auth injection, storage-state token injection, or conditional bailout.

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-070-001-09 Cookie mutations enforce CSRF and Origin`: every cookie-authenticated mutation family (server form, HTMX, PWA fetch, JSON, Cards, admin) returns 403 before any state change on missing, stale, mismatched, or cross-origin evidence, and proceeds only with a valid session-bound proof and trusted same-origin context. Evidence: [report.md#scope-05-core](report.md#scope-05-core).
- [ ] SameSite alone is never accepted as sufficient evidence; the session-bound anti-CSRF proof is required in addition to Origin and same-origin posture (AUTH-011). Evidence: [report.md#scope-05-core](report.md#scope-05-core).
- [ ] The operator role satisfies the same CSRF/Origin contract and does not bypass request-forgery protection. Evidence: [report.md#scope-05-core](report.md#scope-05-core).
- [ ] The 403 forgery rejection is distinct from 401 session rejection and from ordinary errors, is non-enumerating, and emits only bounded telemetry with no token, cookie, or identity values. Evidence: [report.md#scope-05-core](report.md#scope-05-core).

#### Test Evidence - 5 Rows / 5 Items

- [ ] `AUTH-S05-T01` unit CSRF/Origin matrix passes. Evidence: [report.md#auth-s05-t01](report.md#auth-s05-t01).
- [ ] `AUTH-S05-T02` adversarial per-family integration evidence records red then green. Evidence: [report.md#auth-s05-t02](report.md#auth-s05-t02).
- [ ] `AUTH-S05-T03` real e2e-api forged-mutation regression passes. Evidence: [report.md#auth-s05-t03](report.md#auth-s05-t03).
- [ ] `AUTH-S05-T04` real e2e-ui CSRF/Origin regression passes without interception or injected auth. Evidence: [report.md#auth-s05-t04](report.md#auth-s05-t04).
- [ ] `AUTH-S05-T05` broader CSRF/Origin browser regression passes. Evidence: [report.md#auth-s05-t05](report.md#auth-s05-t05).

#### Build Quality Gate

- [ ] CSRF/Origin canary, rollback check, repo-standard build/check/lint/format, artifact lint, traceability guard, regression-quality guards, telemetry privacy checks, and affected auth/API docs are clean with zero warnings or deferrals. Evidence: [report.md#scope-05-quality](report.md#scope-05-quality).

---

## Scope 06: Role And Grant Acceptance And Global Corpus Gating (Disposable Production)

**Status:** Not Started  
**Depends On:** Scope 05  
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: SCN-070-001-01 One login authenticates every browser surface
	Given a disposable production-mode stack a bound invited user explicit permitted grants and production shared-token fallback disabled
	When the user signs in once through the real username and password form
	Then one per-user HttpOnly browser-session cookie authenticates permitted legacy pages Assistant Connectors Photos Wiki or Graph model picker or admin Cards and representative /api and /v1 reads
	And no second credential bearer-token entry request interception auth injection or compatibility fallback is used

Scenario: SCN-070-001-10 Roles share one login but not one authority
	Given one daily-user identity and one operator identity each bound to explicit grants
	When each identity signs in once and traverses representative permitted route families
	Then the same cookie yields 2xx across every permitted middleware family without a second login
	And the daily user receives 403 on representative operator-only routes without a login loop or content disclosure
	And the operator receives 2xx only on operator routes permitted by its grants

Scenario: SCN-070-001-11 Global private corpus is grant-gated
	Given private artifacts Graph Digest and Synthesis exist in the single operator-owned global corpus
	When an operator a specifically granted daily user and an ungranted daily user read those capabilities
	Then each identity receives only the content its explicit grants allow
	And the ungranted identity receives 403 with no content counts labels or existence hints
	And no response or readiness claim asserts tenant or per-user row isolation
```

### UI Scenario Matrix

| Journey | Preconditions | Steps | Required Observation | Forbidden Observation |
|---|---|---|---|---|
| Full permitted journey | Bound user with explicit surface grants | Login; navigate Search/Digest, Assistant, Connectors, Photos, Graph/Wiki, model picker, Cards | Each surface renders authorized content or its true empty state with one cookie | Token prompt, second login, 401-as-empty, intercepted request |
| Daily-user vs operator matrix | One daily-user and one operator identity | Each signs in once; traverse permitted and ungranted routes | Daily user 2xx on daily routes and 403 on operator routes; operator 2xx on permitted operator routes; no second login | Daily-user admin success, login loop, operator-content disclosure |
| Global corpus grant gate | Operator, granted daily user, ungranted daily user | Each reads private artifacts/Graph/Digest/Synthesis | Operator and granted daily user see only granted content; ungranted daily user gets 403 with no counts/labels/existence hints | Any content, count, label, existence hint, or tenant/row-isolation claim for the ungranted identity |
| Privacy sweep | Same browser context | Inspect cookie metadata, DOM, URL, storage, responses, and console during journey | HttpOnly cookie; no credential material elsewhere | Password/token/shared secret/claims body exposed |
| Logout replay | Completed journey | Logout; Back; direct legacy/API/V1 navigation | Signed-out state and re-authentication everywhere | Cached protected DOM or one trust path accepted |

### Implementation Plan

1. Create a disposable production-mode auth lane using the repository's test-stack lifecycle, real PostgreSQL, real signing keys generated for the test namespace, fallback false, and no connection to persistent dev/operate data or telemetry.
2. Provision one daily-user and one operator account only through the real invite/account/password/grant path with their explicit distinct grant sets; Playwright must not mint a token directly, inject a cookie, reuse storage state, or call an internal auth shortcut.
3. Build one minimal-helper real-browser journey per role whose helpers only perform user-visible actions and assertions; prohibit request interception, auth injection, conditional returns, and login-redirect bailouts.
4. Validate the daily-user/operator 2xx/403 matrix across representative legacy, Assistant, Connectors, Photos, Graph/Wiki, model-picker/admin, Cards, `/api`, and `/v1` consumers; the daily user is never required to succeed on operator/admin routes.
5. Validate global-corpus grant gating: operator reads private content, a specifically granted daily user reads only its granted capabilities, and an ungranted daily user receives 403 with no content, counts, labels, or existence hints, asserting no tenant/row-isolation claim.
6. Capture browser privacy assertions and validate-plane auth traces with bounded attributes; inspect no secret values. Execute the auth rollback canary and then restore the unified path in the disposable namespace before broader regression.

### Shared Infrastructure Impact Sweep

- The disposable lane owns its Compose project, database volume/tmpfs, signing keys, invitation/account rows, browser profiles, and validate-plane telemetry namespace.
- It must not mutate persistent dev/prod data, operate-plane telemetry, backups, release-train config, deploy manifests, or another packet's fixtures.
- Teardown runs on success and failure; residue or failed restore blocks acceptance.

### Test Plan

| ID | Test Type | Category | File / Symbol | Scenario | Exact Test Title / Required Assertion | Command | Live |
|---|---|---|---|---|---|---|---|
| AUTH-S06-T01 | Unit | `unit` | `internal/api/model_connections_operator_gate_test.go`, `internal/auth/scope_middleware_test.go`; extend with planned `internal/api/auth_surface_contract_test.go`; anchor `web/pwa/tests/auth_login.spec.ts` | SCN-070-001-01, SCN-070-001-10, SCN-070-001-11 | `TestSurfaceInventoryRoleGrantMatrixAndGlobalCorpusGateUseUnifiedAuthenticatorAndRejectBypassHelpers` | `./smackerel.sh test unit --go` | No |
| AUTH-S06-T02 | Integration red-to-green | `integration` | `tests/integration/auth/production_browser_session_acceptance_test.go` | SCN-070-001-01 | `TestProductionPasswordCookieReachesLegacyAssistantAPIAndV1WithFallbackFalse`; capture the pre-fix modern-route 401 after real login, then the post-fix accepted session with fallback still false | `./smackerel.sh test integration` | Yes |
| AUTH-S06-T03 | Regression API | `e2e-api` | `tests/e2e/auth/role_route_matrix_test.go`, `tests/e2e/auth/global_corpus_grant_gate_test.go`, `tests/integration/graphapi/auth_test.go` | SCN-070-001-10, SCN-070-001-11 | `TestDailyUserAndOperatorRouteMatrixAndGlobalCorpusGrantGateReturn2xxAnd403WithoutLeak` | `./smackerel.sh test e2e` | Yes |
| AUTH-S06-T04 | Regression UI | `e2e-ui` | `web/pwa/tests/production_authenticated_product_journey.spec.ts` | SCN-070-001-01, SCN-070-001-10 | `one real production login reaches every permitted surface and the daily and operator matrix returns 2xx and 403 correctly` | `./smackerel.sh test e2e-ui` | Yes |
| AUTH-S06-T05 | Broader browser regression | `e2e-ui` | `web/pwa/tests/production_authenticated_product_journey.spec.ts` plus existing authenticated PWA/server-rendered suite | SCN-070-001-01 through SCN-070-001-11 | `logout privacy empty degraded denied recovery csrf role-matrix and corpus-gate states remain correct across the broader authenticated journey` | `./smackerel.sh test e2e-ui` | Yes |

### Adversarial Red-To-Green Proof

The same disposable production-mode reproduction must run before and after implementation. Before the fix, the real form writes the shared-runtime-token-shaped cookie and a representative modern route rejects it while legacy web accepts it. After the fix, the form writes a persisted browser-purpose PASETO accepted by both, while a directly supplied shared-token-shaped cookie is rejected by both, the daily-user/operator matrix returns the expected 2xx/403, and an ungranted global-corpus read returns 403 without leak. Source inspection, direct token minting, dev shared-token mode, request interception, injected cookie/storage state, and login bailout cannot satisfy this proof.

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] `SCN-070-001-01 One login authenticates every browser surface`: one real production-mode form login reaches every permitted representative surface with one private browser-purpose cookie and no second credential, interception, auth injection, or fallback. Evidence: [report.md#scope-06-core](report.md#scope-06-core).
- [ ] `SCN-070-001-10 Roles share one login but not one authority`: the daily-user and operator identities each sign in once and reuse one cookie; the daily user gets 2xx on daily routes and 403 on representative operator routes without a login loop or content disclosure, and the operator gets 2xx only on permitted operator routes. Evidence: [report.md#scope-06-core](report.md#scope-06-core).
- [ ] `SCN-070-001-11 Global private corpus is grant-gated`: operator and specifically-granted daily-user reads return only granted content, an ungranted daily user receives 403 with no counts, labels, or existence hints, and no response or readiness claim asserts tenant or per-user row isolation. Evidence: [report.md#scope-06-core](report.md#scope-06-core).
- [ ] Disposable stack provisioning, validate-plane telemetry, teardown, rollback, and restore are isolated and leave zero residue. Evidence: [report.md#scope-06-core](report.md#scope-06-core).
- [ ] Implementation routing remains blocked until this packet's planning artifact lint and traceability guard are clean. Evidence: [report.md#planning-validation](report.md#planning-validation).

#### Test Evidence - 5 Rows / 5 Items

- [ ] `AUTH-S06-T01` unit inventory/role-grant/corpus-gate/helper-policy test passes. Evidence: [report.md#auth-s06-t01](report.md#auth-s06-t01).
- [ ] `AUTH-S06-T02` exact disposable production red-to-green evidence is recorded. Evidence: [report.md#auth-s06-t02](report.md#auth-s06-t02).
- [ ] `AUTH-S06-T03` real e2e-api role-matrix and corpus-gate acceptance passes. Evidence: [report.md#auth-s06-t03](report.md#auth-s06-t03).
- [ ] `AUTH-S06-T04` real e2e-ui product journey and role matrix passes without interception, auth injection, or bailout. Evidence: [report.md#auth-s06-t04](report.md#auth-s06-t04).
- [ ] `AUTH-S06-T05` broader authenticated browser regression passes. Evidence: [report.md#auth-s06-t05](report.md#auth-s06-t05).

#### Build Quality Gate

- [ ] Disposable acceptance and rollback canaries, regression-quality guards, repo-standard build/check/lint/format, artifact lint, traceability guard, privacy/secret scan, accessibility checks, and directly affected auth/API/operator docs are clean with zero warnings or deferrals. Evidence: [report.md#scope-06-quality](report.md#scope-06-quality).

## Planning Handoff Rule

This is a planning-only packet. No implementation owner is routed until `artifact-lint.sh` and `traceability-guard.sh` execute cleanly against this bug directory and the plan-owned `scenario-manifest.json`, `test-plan.json`, `report.md`, and `uservalidation.md` are synchronized with these six scopes.
