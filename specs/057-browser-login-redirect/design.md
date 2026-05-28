# Feature 057 — Design

**Status:** in_progress (planning-only)

## Current Truth (objective research)

- `internal/api/router.go` line 237-238: `POST /v1/web/login` and `POST /v1/web/logout` are registered **outside** `bearerAuthMiddleware` (public). This is where `GET /login` must also be registered.
- `internal/api/router.go` line ~619 `bearerAuthMiddleware`: every failure path calls `writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", ...)` — no content negotiation, no redirect.
- `internal/api/web_login.go`: existing `HandleWebLogin` handles JSON body `{"token": "..."}` and sets `auth_token` cookie with correct attributes (HttpOnly, Secure when prod, SameSite=Lax, Path=/). MUST be reused unchanged by accepting the same body shape from form POST OR adding a form-content path.
- Existing CSP (per spec 044 design §10.4 and current header): `script-src 'self' https://unpkg.com/htmx.org@1.9.12/ 'sha256-<hash>'`. New `/login` page MUST conform.
- No `admin_ui_static` directory exists yet under `internal/api/`. This feature will introduce it OR reuse an existing static-file embed pattern (TBD during plan phase by reading `router.go` static-asset handling).

## Architecture

```
                ┌──────────────────────────────────────────────┐
                │  Unauthenticated browser GET /  (text/html)  │
                └─────────────────┬────────────────────────────┘
                                  │
                                  ▼
            ┌────────────────────────────────────────────┐
            │ bearerAuthMiddleware                       │
            │   if no token AND no cookie AND            │
            │      method ∈ {GET, HEAD} AND              │
            │      Accept contains text/html             │
            │     → 303 Location: /login?next=<path>     │
            │   else                                     │
            │     → 401 UNAUTHORIZED (unchanged)         │
            └────────────────────┬───────────────────────┘
                                  │
                                  ▼ (303)
                ┌────────────────────────────────────┐
                │  GET /login   (PUBLIC route)       │
                │   renders login.html               │
                │   form posts to /v1/web/login      │
                └─────────────────┬──────────────────┘
                                  │ (form POST)
                                  ▼
                ┌────────────────────────────────────┐
                │  POST /v1/web/login (UNCHANGED)    │
                │   validates token, sets cookie     │
                │   responds 303 Location: <next>    │
                └────────────────────────────────────┘
```

## Content-Negotiation Logic (`bearerAuthMiddleware` change)

Insert into the failure paths that currently call `writeError(..., 401, ...)`:

```go
// Pseudocode — exact placement determined in implementation phase.
if isBrowserNavigation(r) {
    next := sanitizeNext(r.URL.RequestURI())
    http.Redirect(w, r, "/login?next="+url.QueryEscape(next), http.StatusSeeOther)
    return
}
// existing 401 path unchanged
```

```go
func isBrowserNavigation(r *http.Request) bool {
    // 1. Method gate: only idempotent safe methods.
    if r.Method != http.MethodGet && r.Method != http.MethodHead {
        return false
    }
    // 2. HTMX / in-page fetch suppression: these are NOT top-level navigations.
    //    Returning HTML for a JSON/HTMX fetch would silently corrupt the page.
    if r.Header.Get("HX-Request") == "true" {
        return false
    }
    // 3. Sec-Fetch-Mode (when present): only `navigate` is a real page load.
    //    Browsers always send this for top-level nav; CORS/fetch send `cors`/`no-cors`.
    if mode := r.Header.Get("Sec-Fetch-Mode"); mode != "" && mode != "navigate" {
        return false
    }
    // 4. Accept gate: explicit text/html required. `*/*` (curl default) is NOT enough.
    accept := r.Header.Get("Accept")
    if !strings.Contains(accept, "text/html") {
        return false
    }
    return true
}
```

**Critical:** `Accept: */*` (curl default) MUST NOT trigger the redirect. Only
explicit `text/html` does. This preserves spec 044's wire contract for all
non-browser clients. HTMX (`HX-Request: true`) and fetch (`Sec-Fetch-Mode:
cors|no-cors`) are also suppressed even when they ask for `text/html`,
because they consume the response as data, not as a top-level page swap.

## `next` Sanitization (open-redirect protection)

```go
func sanitizeNext(raw string) string {
    const safeDefault = "/"

    if raw == "" {
        return safeDefault
    }
    // 1. Header-injection guard (must run on raw, pre-decode).
    if strings.ContainsAny(raw, "\r\n") {
        return safeDefault
    }
    // 2. Decode percent-encoding so attackers can't smuggle `//` as `%2F%2F`
    //    or `\\` as `%5C%5C`. url.PathUnescape (not QueryUnescape) preserves `+`.
    decoded, err := url.PathUnescape(raw)
    if err != nil {
        return safeDefault
    }
    // 3. Must be a path — starts with exactly one `/` and not `//` or `/\`.
    if !strings.HasPrefix(decoded, "/") {
        return safeDefault
    }
    if strings.HasPrefix(decoded, "//") || strings.HasPrefix(decoded, "/\\") {
        return safeDefault  // protocol-relative or backslash-trick
    }
    // 4. Parse and require empty scheme + empty host.
    u, err := url.Parse(decoded)
    if err != nil || u.Scheme != "" || u.Host != "" {
        return safeDefault
    }
    // 5. Reject login-loop: post-login redirect to /login defeats the purpose.
    //    Match the path component only (ignore query/fragment).
    if u.Path == "/login" {
        return safeDefault
    }
    return decoded
}
```

Table of inputs the validator MUST reject (mirrors spec.md Scenario 6 matrix):
`https://evil/`, `//evil/`, `/\evil/`, `javascript:x`, `JavaScript:x`,
`data:text/html,x`, `%2F%2Fevil`, `%5C%5Cevil`, `/login`, `/login?next=/foo`,
`` (empty), `foo/bar` (no leading /), anything with `\r`/`\n`.

This function is the SINGLE source of truth and is called from BOTH
`HandleLoginPage` (for the `?next=` query) AND the form handler (for the
hidden field value on POST) — never trust the client.

## File Layout

| Path | Status | Purpose |
|------|--------|---------|
| `internal/api/admin_ui_static/login.html` | NEW | HTML form (CSP-compliant; external JS only) |
| `internal/api/admin_ui_static/login.js` | NEW | Vanilla JS: progressive enhancement (e.g., focus token field, hidden-field next round-trip) |
| `internal/api/web_login_page.go` | NEW | `HandleLoginPage` for `GET /login`; renders template, propagates `next` |
| `internal/api/web_login.go` | MODIFIED | Accept form-encoded POST in addition to JSON (or add sibling form handler that delegates) |
| `internal/api/auth_middleware.go` (or `router.go` where middleware lives) | MODIFIED | Content-negotiated 303 path |
| `internal/api/router.go` | MODIFIED | Register `GET /login` and static asset routes OUTSIDE bearer middleware |
| `internal/api/web_login_page_test.go` | NEW | Unit tests for GET /login render + next handling |
| `internal/api/auth_middleware_test.go` | MODIFIED | Tests for content-negotiation branch |
| `tests/e2e/auth/browser_login_test.go` | NEW | e2e-api: curl with/without text/html |
| `tests/e2e/auth/browser_login_playwright_test.go` (or existing Playwright harness) | NEW | e2e-ui: full browser flow |

## CSP Strategy

**Required:** ALL JS is externalized to `/admin_ui_static/login.js`. This
keeps the existing CSP `script-src 'self' ...` directive valid with no new
SHA-256 hashes and no `'unsafe-inline'`.

**Hard rules (DoD-enforced):**

- No `<script>...</script>` blocks anywhere in `login.html`.
- No inline event handler attributes: `onclick`, `onsubmit`, `onload`,
  `onerror`, `onkeydown`, etc. are all forbidden in the rendered HTML.
- The only `<script>` tag in `login.html` is `<script src="/admin_ui_static/login.js"></script>`.
- `login.js` MUST be served from `/admin_ui_static/login.js` (path is part of
  the contract; if the route changes, the CSP changes too).
- The login flow MUST work with JavaScript disabled (native `<form>` POST).
- E2E UI test asserts: zero console CSP violation messages during the full
  visit → redirect → form → success cycle.

## Cookie & Logout Flow

`POST /v1/web/login` is unchanged for JSON callers — spec 044 §10.4 cookie
attributes (`HttpOnly`, `Secure` in production, `SameSite=Lax`, `Path=/`)
are preserved byte-for-byte. The form-content branch (or a new
`HandleWebLoginForm`) responds with `303 See Other` instead of JSON; the
cookie issuance path is shared and untouched.

`POST /v1/web/logout` is unchanged — already clears the cookie. The logout
form on `/login` and on the root UI is a plain `<form method="POST"
action="/v1/web/logout">` with a submit button. After clearing, the
handler responds with `303 See Other` to `/login` (already covered by
spec 044's logout contract when invoked from a form context).

### Logout CSRF model

CSRF protection for `POST /v1/web/logout` relies entirely on the existing
cookie's `SameSite=Lax` attribute. A cross-origin forged form POST will
NOT include the `auth_token` cookie, so the logout handler sees no
session to clear and the attack is a no-op against authenticated users.
No per-request CSRF token is introduced. The logout form is hosted on
the same origin as the API, so legitimate same-origin POSTs succeed
normally. This matches the protection model already in place for
`POST /v1/web/login` itself.

### Dev-bypass mode (`AuthConfig.Enabled = false`)

When the runtime starts with auth disabled and no dev token configured,
`GET /login` renders the same template with two changes:

- A static banner at the top: "Authentication is disabled in this environment."
- The token input and submit button are rendered with the `disabled` attribute.

The form action remains `/v1/web/login` (unreachable from a disabled
control). If a client crafts a manual POST, the existing handler already
rejects it with 400 (current behavior), which the form handler also
preserves. The 401→303 middleware branch is also a no-op in this mode
because `bearerAuthMiddleware` itself short-circuits when auth is
disabled.

## Test Plan Summary

| Type | Category | Coverage |
|------|----------|----------|
| Unit | `unit` | `HandleLoginPage` renders; `sanitizeNext` rejects bad inputs; `isBrowserGet` heuristic correctness across Accept header variants |
| Integration | `integration` | Live core container: full cookie round-trip via form POST |
| E2E API | `e2e-api` | curl with `Accept: text/html` → 303; curl with `Accept: */*` → 401; curl with `Accept: application/json` → 401 |
| E2E UI | `e2e-ui` | Playwright headless: visit `/`, follow redirect to `/login`, paste token, land on `/`; logout returns to `/login` |
| Regression | `e2e-api` | Existing spec 044 wire contract: all CLI/API paths still return 401 with JSON shape |

## Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Accept-header heuristic too permissive (XHR with `text/html` in Accept gets redirected) | `HX-Request` and `Sec-Fetch-Mode != navigate` gates suppress redirect; e2e test covers HTMX request explicitly |
| Operator pastes token into URL bar habit | Strip `token` from query string aggressively; never log full URL with token; document in operator runbook |
| CSP regression breaks login JS | E2E UI test asserts no console CSP violations; CI-fail on violation |
| Same-origin `next` validation too strict (legitimate paths rejected) | Validation is path-shape only; any in-app path passes; only scheme/host/protocol-relative/backslash/login-loop is rejected |
| Encoded open-redirect (`%2F%2Fevil`) | Validator decodes BEFORE shape checks; explicit unit tests cover both decoded and encoded forms |
| Login-loop (`next=/login`) traps user in infinite redirect | Validator rejects `/login` as a `next` target; explicit unit + e2e test |
| Cross-origin forged logout | `SameSite=Lax` cookie blocks cookie attachment on cross-site POST; logout is a no-op against authenticated users; documented in "Logout CSRF model" |
| Form auto-fill caches token | `autocomplete="off"` + `type="password"` + page-level `autocomplete="off"` |

## Out-of-Scope

Reaffirmed from spec.md non-goals — no auth-mode changes, no password
flow, no SSO/OAuth, no Caddy edits, no PWA scope creep.

## Single-Implementation Justification

### Single-Implementation Justification

The design intentionally has ONE concrete implementation per surface and
no foundation/provider/strategy split:

- ONE login page template (`login.html`) — no theming layer, no multiple skins.
- ONE middleware branch (`isBrowserNavigation` → 303) — no strategy registry.
- ONE sanitizer (`sanitizeNext`) — not a pluggable URL-policy interface.
- ONE form handler path — added to the existing `HandleWebLogin` via
  content-type branch, not a new pluggable handler chain.

A capability foundation with variation axes is NOT warranted (mirrors
spec.md "Single-Capability Justification"). If a second auth mode is
ever introduced, it will be a new spec that may refactor this one's
handler into a strategy — but speculatively building that abstraction
now would be over-engineering forbidden by the constitution.
