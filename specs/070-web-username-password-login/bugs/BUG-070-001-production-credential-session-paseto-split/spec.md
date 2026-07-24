# Expected Behavior: [BUG-070-001] Unified Production Browser Session

## Problem Statement

Production username/password login currently communicates success before the browser holds a session accepted by all authenticated Smackerel surfaces. It also inherits certified spec 091 assumptions that every invite-created account is full admin, that the production cookie may carry shared-token authority, and that no identity/grant schema change is needed. Those assumptions conflict with the required product outcome: one production-grade username/password login that issues a modern claim-bound session for multiple authenticated identities and works across every route that identity is permitted to use. The repair must make login success product-wide and must not leave both contracts active.

## Outcome Contract

**Intent:** A valid invited identity signs in once and receives a modern claim-bound browser session whose role/grants are accepted uniformly by legacy server pages, HTMX, PWA pages, JSON APIs, Cards, and admin middleware wherever that identity is permitted.

**Success Signal:** Separate daily-user and operator real-browser journeys each perform one username/password login, reuse the same HttpOnly cookie across every permitted middleware family, receive 2xx for representative permitted routes and 403 for authenticated-but-ungranted routes, and never encounter token copy/paste, shared-token fallback, or a second login. A daily user is not required to access operator-only routes.

**Hard Constraints:** Production shared-token fallback remains false; session material is HttpOnly, same-origin, and inaccessible to client script/storage; session issuance and validation preserve claim-bound role/grants, expiry, revocation, and logout; every cookie-authenticated mutation is protected by product-wide CSRF/Origin validation; the repeatable static invite gate remains protected and repeatable; private artifacts/knowledge use the single operator-owned global corpus with explicit grants and no tenant/user row-isolation claim; errors are non-enumerating and value-safe.

**Failure Condition:** Any surface-specific trust split, silent coexistence of incompatible spec 091 and BUG-070 production contracts, shared-token compatibility path, mutation family without CSRF/Origin enforcement, daily user forced through operator routes, malformed-cookie rejection after accepted login, credential disclosure, or test that bypasses the real cookie flow is a failed outcome.

## Certified Contract Amendment

This packet is the explicit successor amendment for the production browser-auth portions of `specs/070-web-username-password-login` and certified `specs/091-web-self-registration-invite-gated`. The certified files remain sealed; this packet and its state metadata carry the amendment and recertification route.

**Preserved from spec 091:**

- The browser registration gate remains a repeatable static invite secret; it is not consumed after one registration and never degrades to open signup.
- Username/password credentials remain non-enumerating, Argon2id-protected intake material, and duplicate usernames never overwrite an existing credential.
- The existing `web_user_credentials` table remains the credential-verification store unless design proves an expand-contract change is required; credential verification itself does not become a second identity authority.

**Superseded where the contracts conflict:**

- Spec 091's assertion that every web user is automatically in the shared-token/full-admin band is replaced by explicit principal binding, role, and grants. Operator authority is granted, not inferred from possession of any valid username/password.
- Spec 091's production shared-token cookie/token-form compatibility is replaced by one claim-bound production browser-session issuer and validator. Shared-token and bootstrap bypasses remain non-production/testing concerns and cannot satisfy production acceptance.
- Spec 091's no-schema expectation is not binding on this repair. If claim-bound principal or grant persistence requires additive schema, BUG-070 SHALL use expand-contract compatibility and preserve the old release's reads until candidate acceptance. The repair may not retain a renderer-specific cookie merely to avoid an additive migration.

The amendment is complete only after BUG-070 is certified and validate-owned recertification records the successor relationship. Until then, readiness must describe the production browser-session contract as unverified rather than silently choosing either legacy interpretation.

## Requirements

- **AUTH-001:** Valid username/password login SHALL issue a production-valid per-user session cookie through the canonical PASETO/session issuer.
- **AUTH-002:** The same cookie SHALL authenticate every browser-facing server, PWA, `/api`, and `/v1` route permitted to that user.
- **AUTH-003:** Login SHALL NOT place the shared runtime token in the cookie or enable shared-token fallback.
- **AUTH-004:** The cookie SHALL be HttpOnly, same-origin, Secure in production, and protected by the repository's explicit SameSite and expiry policy.
- **AUTH-005:** Invalid credentials SHALL return a non-enumerating error and SHALL NOT establish a partial session.
- **AUTH-006:** Malformed, expired, revoked, or wrong-purpose tokens SHALL fail closed and present re-authentication rather than a normal empty state.
- **AUTH-007:** Logout SHALL invalidate the browser session consistently across legacy and modern middleware.
- **AUTH-008:** Authentication telemetry SHALL record outcome class and surface without recording usernames, passwords, cookie values, or token bodies.
- **AUTH-009:** Modern API/PWA regression coverage SHALL use the real login and cookie path without injecting bearer tokens or intercepting internal requests.
- **AUTH-010:** Existing repeatable static-invite, rate-limit, revocation, safe-return, duplicate-credential, and non-enumeration behavior SHALL remain intact except for the explicitly superseded full-admin/shared-token/no-schema assumptions above.
- **AUTH-011:** Every cookie-authenticated state-changing request SHALL require a trusted same-origin request context and a server-validated anti-CSRF proof bound to the browser session. Coverage SHALL include login, registration, logout, server forms, HTMX mutations, PWA fetch, JSON mutations, Cards, and admin. Missing, cross-origin, stale, or mismatched proof SHALL return 403 before any mutation; SameSite alone is not sufficient acceptance evidence.
- **AUTH-012:** The product SHALL define at least `daily-user` and `operator` roles with explicit grants. Each role SHALL authenticate once through the same username/password flow and reuse the same cookie across every permitted server, HTMX, PWA, JSON, Cards, and admin middleware family. Permitted representative routes SHALL return 2xx; authenticated routes outside the identity's grants SHALL return 403 without a second login.
- **AUTH-013:** Daily-user acceptance SHALL cover daily routes only and SHALL explicitly expect 403 for representative operator-only routes. It SHALL NOT require a daily user to access or succeed on admin/provider/configuration routes. Operator acceptance SHALL cover the operator route set plus permitted daily routes.
- **AUTH-014:** Artifacts, knowledge, Graph, Digest, and Synthesis form one operator-owned global corpus. The operator role MAY read private content; another identity MAY read only capabilities granted explicitly. Authentication without the relevant grant SHALL disclose no private content or existence metadata. This contract SHALL NOT claim tenant-level or per-user row isolation.
- **AUTH-015:** There SHALL be exactly one active production username/password-to-browser-session contract. Any old renderer-specific/shared-token cookie path SHALL be removed from production acceptance or explicitly rejected; it may not remain as a hidden compatibility success path.

## Role And Route Acceptance Matrix

| Authenticated Role | Server Forms And HTMX | PWA And JSON Reads | Cards | Admin / Provider / Configuration | Mutation Protection |
|---|---|---|---|---|---|
| Daily user | 2xx for explicitly granted daily routes; 403 for ungranted protected actions | 2xx for explicitly granted daily routes; 403 for ungranted APIs | 2xx only when the Cards grant is present; otherwise 403 | 403; no second login and no operator-content disclosure | Valid session-bound CSRF proof plus trusted same-origin context is required; rejection is 403 before mutation |
| Operator | 2xx for permitted daily and operator routes | 2xx for permitted daily and operator APIs | 2xx when Cards is enabled/permitted | 2xx for permitted admin/provider/configuration routes | The same CSRF/Origin contract applies; operator role does not bypass request-forgery protection |

An unauthenticated, expired, malformed, or revoked session receives re-authentication/401 behavior rather than this authenticated 2xx/403 matrix. A 403 proves the same cookie was accepted and the role/grant check denied the route; it must not redirect into a second login loop.

## User Scenarios

```gherkin
Scenario: SCN-070-001-01 One login authenticates every browser surface
  Given a valid invited identity with an explicit role/grant set and production shared-token fallback disabled
  When the user signs in with username and password
  Then one claim-bound HttpOnly session cookie authenticates every permitted legacy, HTMX, PWA, JSON, Cards, and admin middleware family
  And no second credential or bearer-token entry is required

Scenario: SCN-070-001-02 Invalid credentials create no partial session
  Given a username or password is invalid
  When login is submitted
  Then the response is non-enumerating
  And no authenticated cookie accepted by any middleware is created

Scenario: SCN-070-001-03 Malformed expired and revoked sessions fail closed
  Given a cookie is malformed, expired, revoked, or issued for the wrong purpose
  When an authenticated route validates it
  Then access is denied consistently
  And the user receives a re-authentication path rather than an empty success state

Scenario: SCN-070-001-04 Session material remains private
  Given login succeeds
  When the browser renders and navigates the product
  Then no password, shared token, or per-user token appears in DOM, URLs, client storage, console output, or response bodies

Scenario: SCN-070-001-05 Logout closes both trust paths
  Given one session currently authenticates legacy and modern surfaces
  When the user logs out
  Then both middleware families reject subsequent use of that session

Scenario: SCN-070-001-06 Authenticated no-data remains a normal empty state
  Given the user is authenticated and an authorized collection has no records
  When a modern surface reads it
  Then it shows its true no-data state
  And it does not confuse a valid empty result with authentication failure

Scenario: SCN-070-001-07 Downstream failure does not invalidate a valid session
  Given login succeeds and one downstream capability is degraded
  When the user opens another healthy authenticated surface
  Then the session remains valid
  And only the degraded capability reports its typed failure

Scenario: SCN-070-001-08 Login and re-authentication are accessible and responsive
  Given a keyboard or screen-reader user on a narrow viewport
  When login succeeds or a session expires
  Then focus, status, errors, and the safe return action are perceivable and operable without overlap

Scenario: SCN-070-001-09 Cookie mutations enforce CSRF and Origin
  Given a daily user or operator has a valid claim-bound browser session
  When a server form, HTMX request, PWA fetch, JSON mutation, Cards action, or admin action is submitted with missing, stale, mismatched, or cross-origin CSRF evidence
  Then the request returns 403 before any state changes
  When the same permitted action carries valid session-bound CSRF evidence and trusted same-origin context
  Then it may proceed under the identity's role/grants

Scenario: SCN-070-001-10 Roles share one login but not one authority
  Given one daily-user identity and one operator identity
  When each identity signs in once and traverses representative permitted route families
  Then the same cookie for that identity yields 2xx across all of its permitted middleware families without a second login
  And the daily user receives 403 on representative operator-only routes without a login loop or content disclosure
  And the operator receives 2xx only on operator routes permitted by its grants

Scenario: SCN-070-001-11 Global private corpus is grant-gated
  Given private artifacts, Graph, Digest, and Synthesis exist in the single operator-owned global corpus
  When an operator, a specifically granted daily user, and an ungranted daily user read those capabilities
  Then each identity receives only the content its explicit grants allow
  And the ungranted identity receives 403 without content, counts, labels, or existence hints
  And no response or readiness claim asserts tenant or per-user row isolation
```

## Acceptance Criteria

1. All eleven scenarios have persistent scenario-specific tests and at least one adversarial case using a shared-token-shaped or malformed cookie that the old split would mishandle.
2. Daily-user and operator sessions each use one real login and the same cookie across representative server, HTMX, Assistant, Connectors, Photos, Graph, Cards, JSON API, and role-appropriate admin routes; the asserted 2xx/403 matrix never requires daily-user admin success.
3. Shared-token fallback remains disabled and no secret material appears in client-observable surfaces.
4. Invalid, expired, malformed, revoked, empty-data, and degraded-dependency outcomes remain distinguishable.
5. Authenticated Playwright uses the real login form and browser cookie jar with no internal request interception.
6. Every cookie-authenticated mutation family enforces trusted Origin plus session-bound CSRF proof and returns 403 before mutation on adversarial requests.
7. The successor amendment preserves the repeatable static invite gate while replacing incompatible full-admin/shared-token/no-schema assumptions; certification cannot report two active production contracts.
8. Global private-corpus access is proved by role/grant behavior and leak-free 403 denial, not by a tenant/user row-isolation claim.

## Dependencies

- Blocks modern authenticated API/PWA verification in `specs/106-coherent-product-experience`.
- Blocks the product journey synthetic in `specs/102-target-deploy-hardening/bugs/BUG-102-001-product-journey-acceptance-gap`.
- Preserves spec 091's repeatable static invite and credential-intake safety, while explicitly amending its production full-admin/shared-token/no-schema assumptions through this successor packet and validate-owned recertification.

## Release Train

- Target train: `mvp`.
- Flags introduced: none.
- The repair must not depend on a train-specific fallback; other trains may claim login readiness only when the same product-wide session contract is verified.

## UI Wireframes

### UX Requirements

| ID | Observable Contract |
|---|---|
| UX-070-001-01 | Initial sign-in and session recovery use one username/password form, one non-enumerating credential error, and one product-wide session outcome. No renderer presents a token-entry alternative to ordinary users. |
| UX-070-001-02 | The form does not announce success when credentials merely verify or a response merely contains Set-Cookie; successful UX is the authorized destination rendered with the canonical session. |
| UX-070-001-03 | Invalid username, invalid password, unknown user, malformed credential record, and disabled credential produce the same visible message and leave no partially authenticated destination. |
| UX-070-001-04 | Expired, revoked, malformed, and wrong-purpose sessions produce the same safe re-authentication presentation; the browser-visible copy does not reveal token-validation internals. |
| UX-070-001-05 | A 401/session rejection differs from 403/insufficient scope, true empty, filtered empty, degraded dependency, and ordinary server error on every authenticated surface. |
| UX-070-001-06 | Safe return displays a human destination label, carries only an allowlisted same-origin destination, and re-authorizes that destination after sign-in; raw queries, fragments, credentials, and cross-origin URLs are not echoed. |
| UX-070-001-07 | Logout exposes signing-out, signed-out, and failed-to-sign-out states and does not claim completion until the server has invalidated/cleared the session accepted by both middleware families. |
| UX-070-001-08 | Password/session material never appears in DOM text/attributes, URL, browser storage, script-readable cookies, response bodies, clipboard affordances, or console-visible UX diagnostics. |

### Screen Inventory

| Screen / Surface | Actor(s) | Status | Scenarios Served |
|---|---|---|---|
| Login and Session Recovery (`/login`) | Invited daily user, operator | Existing - Modify | SCN-070-001-01 through 05, 08 |
| Authenticated Product Shell (all browser surfaces) | Authenticated user | Existing - Modify contract | SCN-070-001-01, 03, 05 through 08 |

### UI Primitives

| Primitive | Used By Screens | Composition Rule | Accessibility / Responsive Constraint |
|---|---|---|---|
| Authentication form | Initial login; re-authentication | One semantic POST form; username persists after recoverable rejection, password clears; no machine-token field in the primary flow. | One-column source order; labels persist; password-manager semantics remain intact. |
| Authentication status region | Login; logout; session recovery | One mutually exclusive state replaces the prior state; generic security messages never expose account/token cause. | Progress is polite; rejection/failure is one alert linked to fields/action. |
| Session-ended presentation | Every authenticated surface | Protected content nodes are removed before redirect/band; contains one safe destination label and `Sign in again`. | Focus starts at `Your session ended`; no background content remains perceivable. |
| Access-denied presentation | Every authenticated surface | 403 remains on a safe shell with `You do not have access`; it does not redirect into a login loop. | Heading receives focus; safe navigation follows. |
| Safe return marker | Login; recovery | Human label only (`Assistant`, `Cards`, `Graph`, etc.); destination is re-authorized after login. | Label is visible text; no hidden query-dependent description. |
| Session menu and logout feedback | Authenticated shell | Shows signed-in identity label appropriate to policy and one Logout action; no token/session metadata. | Keyboard-operable menu; in-flow status remains after close/redirect. |

### Screen: Login And Session Recovery

**Actor:** Daily User, Operator | **Route:** `/login` | **Status:** Modify

**Desktop:**

```text
┌──────────────────────────────────────────────────────────────────┐
│ Smackerel                                                        │
├──────────────────────────────────────────────────────────────────┤
│ [Your session ended / Signed out / Return to: Graph]            │
│                                                                  │
│ Sign in                                                          │
│ Use the account created from your invitation.                    │
│                                                                  │
│ Username                                                         │
│ ┌──────────────────────────────────────────────────────────────┐ │
│ │ [retained after recoverable rejection]                       │ │
│ └──────────────────────────────────────────────────────────────┘ │
│ Password                                                         │
│ ┌──────────────────────────────────────────────────────────────┐ │
│ │ [write-only password input; cleared after submission]        │ │
│ └──────────────────────────────────────────────────────────────┘ │
│                                                                  │
│ [Sign in]                                                        │
│ [Submitting / non-enumerating error / rate limit / unavailable] │
└──────────────────────────────────────────────────────────────────┘
```

**Mobile / narrow viewport:**

```text
┌──────────────────────────────┐
│ Smackerel                    │
├──────────────────────────────┤
│ [session/recovery message]   │
│ Return to: [safe label]      │
│                              │
│ Sign in                      │
│ Username                     │
│ [..........................] │
│ Password                     │
│ [..........................] │
│ [full-width Sign in]         │
│ [status/error wraps in flow] │
└──────────────────────────────┘
```

**Login and recovery states:**

| State Key | Visible Heading / Message | Field and Action Behavior | Privacy / Navigation Rule |
|---|---|---|---|
| `initial` | `Sign in` | Empty password; username may be browser-filled. | Default authorized destination is Assistant. |
| `session-ended` | `Your session ended` and safe return label. | Empty password; Sign in available. | No expired/revoked/malformed/wrong-purpose distinction or protected content. |
| `signed-out` | `You are signed out` | Empty password; Sign in available. | Back/direct navigation to protected routes re-enters auth. |
| `submitting` | `Signing in` | Submit disabled; fields remain present; duplicate Enter/click ignored. | No success copy or cookie/token value. |
| `invalid` | `Sign-in details were not accepted` | Username retained; password cleared; focus moves to error then username. | Same message/timing class for account/password/record rejection. |
| `rate-limited` | `Too many sign-in attempts` | Password cleared; submit disabled until explicit server-provided safe retry time or manual retry becomes available. | Does not reveal whether any attempt named a real user. |
| `service-unavailable` | `Sign in is temporarily unavailable` | Username retained; password cleared; Retry action. | Does not suggest creating a new account or using a token fallback. |
| `network-error` | `Smackerel could not be reached` | Username retained; password cleared; Retry action. | Distinct from invalid credentials and server rejection. |
| `authorizing-destination` | `Opening [safe destination label]` | Form disabled after canonical session issue while destination is re-authorized. | If destination denies scope, render Access denied rather than loop to login. |
| `success` | Authorized destination content, not a success card on `/login`. | Login form no longer visible. | Same HttpOnly session carries into legacy, PWA, `/api`, and `/v1` reads. |

**Interactions:**

- Enter in either credential flow submits exactly one semantic form request. Pointer and keyboard use the same pending/error contract.
- Invalid/recoverable failure retains username to reduce re-entry, clears password, and never offers show/copy/reveal session material.
- A safe `next` destination is displayed as a product label. Missing, hostile, cross-origin, unauthorized, or sensitive-query return targets resolve to the authorized default without echoing the rejected value.
- Session expiry/revocation on any product surface routes to this same recovery form. After sign-in, the destination is checked again against the new user's permissions.
- Browser password managers may fill/save username/password according to standard semantics; Smackerel itself never persists either value in client storage.

**Responsive:** One stable content column at all widths; no split hero, decorative card, fixed footer, or overlapping action. At 320px and 200% zoom, labels, fields, errors, safe-return text, and action wrap without horizontal scroll. Touch targets are at least 44 by 44 CSS pixels.

**Keyboard:** Source order is recovery message, heading, username, password, Sign in, Retry/help where present. Error focus moves once to the summary, then next Tab reaches username. Enter submits once; focus is not lost during `Signing in`. Browser Back after successful login cannot reveal credential values.

**Screen reader and visual accessibility:** Fields have persistent labels and explicit autocomplete purposes; errors are linked by description and invalid state. `Signing in` is a polite live status; invalid/rate-limit/network/service errors are one-time alerts. Focus indication and error text meet contrast in both themes. No CAPTCHA, timeout, or rate-limit message is conveyed by animation/color alone.

### Surface Contract: Authenticated Product Shell

**Actor:** Authenticated user | **Routes:** Legacy pages, Assistant, Connectors, Photos, Wiki/Graph, model picker/admin, Cards, representative `/api` and `/v1` reads | **Status:** Modify contract

| Response / Data Condition | Required Visible State | Must Not Render |
|---|---|---|
| Session accepted, content loading | Surface-specific `Loading`/skeleton with shell identity. | Login prompt, empty state, or prior user's content. |
| Session accepted, populated read | Normal ready/content state. | Authentication success banner as a substitute for content. |
| Session accepted, true empty | Surface-specific `No [items] yet` after authorized successful read. | `Your session ended`, access denied, or generic error. |
| Session accepted, filters exclude rows | `No [items] match these filters` plus clear filters. | First-use empty or auth failure. |
| Session accepted, dependency degraded | Retained verified content where safe plus typed limitation/retry. | Session-ended message or global logout. |
| Session rejected (401) | Protected content removed; `Your session ended`; Sign in again with safe destination label. | Empty/filtered-empty, degraded content, token cause, or stale protected DOM. |
| Session accepted, scope denied (403) | `You do not have access` plus safe navigation. | Repeated login redirect or account-existence detail. |
| Ordinary server/network error | Surface-specific error and retry while session remains valid where known. | Forced logout or invalid-credential message. |

**Logout mutation feedback:**

| State | Visible Outcome | Contract |
|---|---|---|
| Ready | `Sign out` in session menu. | No token ID/expiry/cookie details. |
| Submitting | `Signing out`; duplicate action disabled. | Protected navigation is suspended for the attempt. |
| Complete | Redirect to `/login` with `You are signed out`. | Direct legacy and modern routes both require re-authentication; Back reveals no protected response. |
| Failed | `Could not sign out` plus Retry. | Product does not claim logout; session menu remains truthful until server outcome. |

**Accessibility / responsive:** The session menu is keyboard-operable, closes with Escape, and restores focus to its trigger. Session-ended and access-denied headings receive focus once. Protected content is removed from both visual and accessibility trees before recovery UI appears. Mobile shell keeps Sign out reachable without overlapping page actions.

### Playwright-Visible Behavior Contract

These are planned real-browser observations. They do not claim auth, browser, cookie, or test execution.

| ID | Real-Stack Setup and Gesture | Required Visible / Browser Observation | Forbidden Outcome |
|---|---|---|---|
| UX-E2E-070-001-01 | Valid invited user signs in through the real username/password form with production shared-token fallback disabled. | `Signing in` transitions to authorized destination; one HttpOnly same-origin cookie exists in browser context and representative legacy/PWA/API/V1 reads succeed through normal navigation/fetch. | Token field/copy-paste, second login, shared token cookie, intercepted internal request. |
| UX-E2E-070-001-02 | Unknown username and wrong password are submitted in separate runs. | Same `Sign-in details were not accepted`; username retained, password cleared; no protected destination/cookie accepted by any middleware. | Account enumeration, cause-specific copy, partial session. |
| UX-E2E-070-001-03 | Malformed, expired, revoked, wrong-purpose, and shared-token-shaped cookies are used in separate adversarial runs. | Every representative surface converges on `Your session ended` and one Sign in again path. | Normal empty state, renderer-specific token form, cause disclosure, or one middleware accepting what another rejects. |
| UX-E2E-070-001-04 | Successful login is followed by DOM/storage/URL/console inspection during cross-surface navigation. | No password/token/shared secret appears; session cookie is inaccessible to page script and absent from localStorage/sessionStorage/IndexedDB. | Any credential-bearing client storage, URL, DOM attribute/text, response body, or console UX output. |
| UX-E2E-070-001-05 | Authenticated user logs out, then navigates Back and directly opens representative legacy and modern routes. | `Signing out` transitions to `You are signed out`; all protected routes require sign-in. | Cached protected content or one trust path remaining authenticated. |
| UX-E2E-070-001-06 | Authenticated user opens an authorized empty collection and then applies filters that exclude existing rows. | True-empty and filtered-empty messages appear respectively with session intact. | Session-ended/access-denied/error copy. |
| UX-E2E-070-001-07 | Authenticated user opens one degraded capability, then another healthy surface. | Degraded surface shows its limitation; healthy surface renders normally under the same session. | Global logout or session-invalid message caused by dependency failure. |
| UX-E2E-070-001-08 | Authenticated user without required scope opens an operator route. | `You do not have access` and safe navigation appear. | Login loop, username disclosure, or operator content. |
| UX-E2E-070-001-09 | Safe return tests default, valid same-origin, hostile cross-origin, sensitive-query, and unauthorized destinations. | Valid destination label/return works; others resolve to authorized default without echoing rejected value. | Open redirect, raw query/fragment display, repeated auth loop. |
| UX-E2E-070-001-10 | Keyboard/screen-reader user signs in and recovers at 320px and 200% zoom. | Focus/error/status order is correct; fields/actions do not overlap; recovery removes protected content from accessibility tree. | Pointer-only action, horizontal scroll, color-only status. |

### Routed Design Questions

| Owner | Question | UX Constraint That Must Survive Resolution |
|---|---|---|
| `bubbles.design` | Where is the single browser-session issue/verify/revoke boundary shared by legacy and modern middleware? | One login creates one product-wide session; no renderer-specific compatibility path is user-visible. |
| `bubbles.design` | Which SameSite, expiry, safe-return allowlist, and logout invalidation contracts apply without exposing token purpose/cause? | Re-authentication is safe and non-enumerating; 401 and 403 remain distinct. |
| `bubbles.design` | How does the login response prove destination authorization before UX leaves `Opening [destination]`? | Success is destination access, not credential verification or Set-Cookie alone. |
| `bubbles.plan` | Which representative routes cover each middleware family and each malformed/expired/revoked/wrong-purpose/shared-token adversarial case? | Playwright must use the real form and browser cookie jar with no bearer injection or internal interception. |

## User Flows

### User Flow: Product-Wide Login And Recovery

```mermaid
stateDiagram-v2
  [*] --> Login
  Login --> SigningIn: Submit once
  SigningIn --> AuthorizingDestination: Credentials accepted; canonical session issued
  AuthorizingDestination --> AssistantToday: Destination authorized
  AuthorizingDestination --> AccessDenied: Session valid; destination scope denied
  SigningIn --> LoginError: Invalid / rate limited / network / service
  LoginError --> Login: Correct or retry
  AssistantToday --> ModernSurface: Same cookie
  ModernSurface --> TrueEmpty: Authorized successful empty read
  ModernSurface --> FilteredEmpty: Authorized filters exclude rows
  ModernSurface --> Degraded: Dependency degraded; session valid
  ModernSurface --> AccessDenied: Valid session; insufficient scope
  ModernSurface --> SessionEnded: Expired / revoked / malformed / wrong purpose
  SessionEnded --> Login: Sign in again with safe return
  Login --> SigningIn: Re-authenticate
  AuthorizingDestination --> ModernSurface: Re-authorized return
  ModernSurface --> SigningOut: Logout
  SigningOut --> Login: Server invalidates session; signed-out message
  SigningOut --> ModernSurface: Logout failed; session remains truthful
```
