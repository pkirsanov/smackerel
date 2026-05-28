# Feature 057 — Browser-Friendly Login & 401→Login Redirect

**Status:** in_progress (planning-only; statusCeiling: specs_hardened)
**Workflow Mode:** product-to-planning
**Depends On:** [044-per-user-bearer-auth](../044-per-user-bearer-auth/) (cookie session foundation; PASETO + shared dev token credential forms)

## Problem Statement

Smackerel-core's HTTP surface is API-shaped: the only authentication paths are
`Authorization: Bearer <token>` and the `auth_token` cookie obtained via
`POST /v1/web/login` (spec 044 Scope 03). There is **no HTML entry point**.

A browser visit to `https://<host>/` arrives with no cookie and no
`Authorization` header. `bearerAuthMiddleware`
(`internal/api/router.go` ~line 619) returns plain `401 Unauthorized` text and
the user is stuck — they cannot acquire a cookie because the only way to do
so is to manually craft a `POST /v1/web/login` with a JSON body containing
their token.

Observed live on `evo-x2`: tile `Smackerel → https://evo-x2.<tailnet>.ts.net/`
renders "Unauthorized" with no actionable path forward for the operator.

The PWA-cookie design from spec 044 was correct at the API layer but never
grew an HTML entry point. This feature closes that gap with the minimum
surface required: a token-paste login page and content-negotiated redirect.

## Scope (NON-NEGOTIABLE)

1. **HTML login page at `GET /login`** — minimal form (single token field +
   submit) that POSTs to the existing `/v1/web/login`, then redirects to
   the original `next` path (default `/`). Plain HTML + small vanilla JS or
   HTMX (already loaded via existing CSP at `htmx.org@1.9.12`). Must
   conform to existing CSP. Must work in production (per-user PASETO)
   AND dev (shared `SMACKEREL_AUTH_TOKEN`).
2. **401 → /login redirect for browser GETs** — `bearerAuthMiddleware`,
   when the request has `Accept: text/html` AND method is `GET` or `HEAD`
   AND no valid credential is present, MUST return `303 See Other` with
   `Location: /login?next=<requested-path>` instead of `401`. API/JSON
   requests (no `text/html` in `Accept`) MUST keep returning `401`
   unchanged — zero regression for spec 044's wire contract.
3. **Logout link** — `/login` page and the root `/` UI MUST expose a
   `POST /v1/web/logout` action so a browser user can clear their cookie
   and return to the login form.

## Non-Goals (Explicit)

- No new auth modes. PASETO + shared dev token remain the only credentials.
- No password-based login. The token IS the credential; the form collects it.
- No "remember me" beyond the cookie's existing `Max-Age`.
- No PWA enhancement work beyond what the login flow itself requires.
- No Caddy-layer changes. The single-host topology with app-level auth is preserved.
- No new auth surfaces (OAuth, SSO, magic links).

## Gherkin Scenarios

### Scenario 1: Browser visits root without cookie → redirected to login

```gherkin
Given smackerel-core is running in production mode
And the user has no auth_token cookie
When the user's browser sends GET / with Accept: text/html
Then the response status is 303 See Other
And the Location header is /login?next=/
And no 401 body is returned
```

### Scenario 2: CLI/API client without cookie → still gets 401

```gherkin
Given smackerel-core is running in production mode
When a client sends GET /v1/health with Accept: */* and no Authorization header
Then the response status is 401 Unauthorized
And the body contains the existing UNAUTHORIZED error shape
And no 303 redirect is issued
```

### Scenario 3: Browser visits /login → sees form

```gherkin
Given smackerel-core is running
When the user's browser sends GET /login
Then the response status is 200 OK
And the response Content-Type is text/html
And the body contains a <form method="POST" action="/v1/web/login">
And the form contains an <input type="password" name="token" autocomplete="off">
And the form preserves the next query parameter as a hidden field
And the response conforms to the existing CSP (no inline-script violations)
```

### Scenario 4: User pastes token and submits → cookie set, redirected to next

```gherkin
Given the user has loaded GET /login?next=/dashboard
And the user has a valid token (PASETO in prod, shared in dev)
When the user submits the form with the token
Then the server validates the token via existing /v1/web/login logic
And sets the auth_token cookie (HttpOnly, Secure in prod, SameSite=Lax, Path=/)
And responds with 303 See Other and Location: /dashboard
```

### Scenario 5: Invalid token submitted → form re-renders with error

```gherkin
Given the user has loaded GET /login
When the user submits the form with an invalid token
Then the response status is 200 OK (or 401 with HTML body)
And the form re-renders with a non-revealing error message
And no cookie is set
```

### Scenario 6: Open-redirect protection on `next` parameter

```gherkin
Given a malicious link sends the user to /login?next=<payload>
When the user successfully logs in
Then the server ignores the unsafe value
And redirects to / (the safe default)
And logs the rejected next value at INFO level
```

Validation rule for `next` (applied BOTH on `GET /login` query AND on form
POST hidden field — defense in depth). The validator MUST reject and fall
back to `/` for ALL of these adversarial inputs:

| Input | Reason for rejection |
|-------|----------------------|
| `https://evil.example.com/` | absolute URL (scheme + host) |
| `//evil.example.com/path` | protocol-relative URL |
| `/\evil.example.com/path` | backslash-prefixed (some browsers normalize `\` → `/`) |
| `javascript:alert(1)` | non-http scheme |
| `JavaScript:alert(1)` | mixed-case scheme variant |
| `data:text/html,...` | data URL |
| `%2F%2Fevil.example.com` | URL-encoded `//` (validator must check post-decode) |
| `%5C%5Cevil.example.com` | URL-encoded `\\` |
| `/login` or `/login?next=/login` | login-loop (would re-render the login page after success) |
| `` (empty) | no destination |
| any value containing `\r` or `\n` | header-injection guard |
| any value NOT starting with `/` | not a same-origin path |

Valid: any same-origin in-app path that starts with a single `/` and is
not `/login`. Examples: `/`, `/dashboard`, `/notes/abc?q=1#frag`.

### Scenario 11: HTMX / fetch-style request without cookie → 401 (no redirect)

```gherkin
Given an authenticated single-page interaction
When the browser sends GET /v1/topics with HX-Request: true and Accept: text/html
Then the response status is 401 Unauthorized
And no 303 redirect is issued
And the client-side JS receives the error and can re-trigger the login flow itself
```

Rationale: HTMX and other in-page fetch helpers send `Accept: text/html`
but the response is being consumed by JavaScript, not by a top-level
navigation. Returning 303 would silently swap page fragments with the
login HTML, which is worse than a clean 401. Detection: presence of
`HX-Request: true` header (HTMX) OR `Sec-Fetch-Mode: cors` (fetch())
suppresses the redirect. Top-level navigations always send
`Sec-Fetch-Mode: navigate`.

### Scenario 12: Dev mode with auth disabled — /login is informational only

```gherkin
Given smackerel-core is running with AuthConfig.Enabled = false and SMACKEREL_AUTH_TOKEN is empty
When the user's browser sends GET /login
Then the response status is 200 OK
And the page renders a non-interactive notice ("Authentication is disabled in this environment")
And the form's submit control is disabled OR absent
And submitting the form (if reachable) does not set a cookie
```

### Scenario 7: Authenticated user logs out

```gherkin
Given the user has a valid auth_token cookie
When the user clicks the logout link (POST /v1/web/logout from a form)
Then the auth_token cookie is cleared (Max-Age=0)
And the response redirects (303) to /login
```

### Scenario 8: Dev-mode shared token works through the form

```gherkin
Given smackerel-core is running in dev mode with SMACKEREL_AUTH_TOKEN set
When the user submits the form with the shared dev token
Then the existing /v1/web/login dev path accepts it
And the cookie is set
And the user lands on the next path
```

### Scenario 9: GET /login does not accept token via query string

```gherkin
When a request arrives as GET /login?token=<value>
Then the server ignores the token query parameter
And renders the empty form
And no cookie is set
```

### Scenario 10: HEAD with text/html still redirects (no body)

```gherkin
Given an unauthenticated browser-style request
When it sends HEAD / with Accept: text/html
Then the response status is 303 See Other
And the Location header is /login?next=/
And the response body is empty
```

## Functional Requirements

- **FR-001:** `GET /login` is publicly accessible (NOT behind `bearerAuthMiddleware`).
- **FR-002:** `GET /login` returns CSP-compliant HTML with a token-paste form. All JS is externalized to `/admin_ui_static/login.js`; no inline `<script>` blocks and no inline event handler attributes (`onclick`, `onsubmit`, etc.).
- **FR-003:** `bearerAuthMiddleware` content-negotiates: returns `303 → /login` ONLY when ALL of the following hold: method ∈ {GET, HEAD} AND `Accept` header explicitly contains `text/html` AND the request does NOT carry `HX-Request: true` AND `Sec-Fetch-Mode` (when present) is `navigate` or absent. Any other combination keeps the existing 401 behavior.
- **FR-004:** `next` parameter is validated as a same-origin path (see Scenario 6 rejection matrix); invalid values fall back to `/`. Validation runs in two places: on `GET /login` query parsing AND on form POST hidden-field handling.
- **FR-005:** Logout flow is reachable from `/login` and from the authenticated `/` UI. The logout form posts to the existing `POST /v1/web/logout` with no JSON body. CSRF defense relies on the existing `SameSite=Lax` cookie attribute (cross-origin form POSTs do not send the cookie, so a forged logout is a no-op); no separate CSRF token is introduced.
- **FR-006:** `POST /v1/web/login` behavior is UNCHANGED for JSON callers (spec 044's contract is preserved byte-for-byte). Form-encoded callers receive a `303 See Other` to the sanitized `next` path instead of the JSON body — this is an additive content type, not a change to the existing JSON wire contract.
- **FR-007:** Cookie attributes (`HttpOnly`, `Secure` in production, `SameSite=Lax`, `Path=/`, existing `Max-Age`) are unchanged from spec 044 §10.4. This feature does NOT touch cookie issuance code.
- **FR-008:** When `AuthConfig.Enabled = false` and no dev token is configured, `GET /login` renders an informational page (Scenario 12) instead of a working form. The route remains reachable; the form is non-interactive.

## Out-of-Scope Confirmation

Spec 044's wire contract for API consumers (CLI, scripts, external integrations) is preserved: any request that does not explicitly accept `text/html` continues to receive `401 Unauthorized` with the existing JSON error shape.

## Single-Capability Justification

### Single-Capability Justification

This feature delivers ONE capability: a browser entry point for the
existing cookie-session authentication. There is no foundation/variant
structure to model because:

- There is exactly ONE login surface (`GET /login`) — not multiple screens.
- There is exactly ONE credential type (the bearer token from spec 044) — not multiple auth modes.
- The middleware change is ONE branch (content-negotiated 303) — not a strategy/adapter family.
- No second provider, no plugin surface, no variant axis exists or is anticipated within this spec's scope.

A Domain Capability Model with multiple concrete implementations is
explicitly NOT warranted. Any future second auth mode (OAuth, SSO,
magic-link) would be a new spec that supersedes this one's single-screen
shape — not an extension via plugin/variant within this spec.

## Cross-References

- [`specs/044-per-user-bearer-auth/spec.md`](../044-per-user-bearer-auth/spec.md) — cookie session foundation
- [`specs/044-per-user-bearer-auth/design.md`](../044-per-user-bearer-auth/design.md) §10.4 — cookie attributes
- [`internal/api/web_login.go`](../../internal/api/web_login.go) — existing POST handler (unchanged)
- [`internal/api/router.go`](../../internal/api/router.go) — middleware order and route registration
- [`.specify/memory/constitution.md`](../../.specify/memory/constitution.md) — binding principles
- [`docs/smackerel.md`](../../docs/smackerel.md) — product/architecture truth
