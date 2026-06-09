# Spec 070 — Web Username/Password Login

## Status
done

## Problem
Today, logging in to the smackerel web admin UI requires pasting the shared
`SMACKEREL_AUTH_TOKEN` (a 64-char hex string) into the `/login` form's
"Token" field. Operators cannot remember that, paste it from a password
manager every time, and it leaks if accidentally shown. A real
username/password credential is needed for the human operator surface.

The Telegram bot and machine API clients keep using bearer tokens; this
spec adds a *human-facing credential layer* on top of the existing
shared-token mechanism. No change to bearer auth, no change to PASETO,
no change to `auth.RequireScope` semantics.

## Goals
1. Operator can log in to https://<deploy-host>.<tailnet>.ts.net/login with a
   username + password they remember.
2. Once logged in, all admin pages work in the same browser session
   (single sign-on across the smackerel UI surfaces).
3. Operator can create new users and rotate passwords via a CLI on the
   deployed container (no DB shell required).
4. No regression: existing token-form login still works for Telegram
   bot OAuth callback and any machine clients.

## Non-Goals
- Per-user permissions / roles (any web user = full admin, same as
  the shared token grants today).
- Password reset by email / SMS (operator-only rotation via CLI).
- MFA / WebAuthn (followup spec when needed).
- Browser SSO across multiple smackerel instances (out of scope).
- Replacing PASETO bearer tokens or the spec 044/060 auth model.

## Use Cases

### UC-1: Operator logs in
- **Given** a web user has been created with username `operator`
- **When** the operator opens `/login`, enters username + password,
  submits
- **Then** the form returns 303 to `/` (or `?next=`), sets the
  `auth_token` cookie, and the operator lands on the admin home page

### UC-2: Operator stays logged in
- **Given** the operator just logged in (cookie set)
- **When** they navigate to `/admin`, `/api/health`, `/admin/agent`,
  or any other authenticated page
- **Then** every page loads without prompting again until the cookie
  expires or `/v1/web/logout` is called

### UC-3: Operator bootstraps the first user
- **Given** a fresh deploy with no `web_user_credentials` rows
- **When** the operator runs
  `docker exec smackerel-<env>-smackerel-core-1 smackerel-core users add operator`
- **Then** the CLI prompts for a password on TTY (no echo),
  hashes it with argon2id, inserts the row, prints success

### UC-4: Operator rotates a password
- **Given** an existing user
- **When** the operator runs
  `docker exec ... smackerel-core users set-password operator`
- **Then** the CLI prompts for a new password, replaces the hash,
  prints success, old sessions stay valid until cookie expiry

### UC-5: Wrong password is rejected
- **Given** user `operator` exists with a known password
- **When** an attacker submits the wrong password
- **Then** the form renders an error ("Invalid username or password.")
  with a generic message (no user-existence leak), and no cookie is
  set

### UC-6: Unknown user is rejected
- **Given** a request for `username=ghost`
- **When** the form is submitted with any password
- **Then** the same generic error is rendered. Timing MUST NOT leak
  user existence (constant-time compare on a dummy hash when user not
  found).

### UC-7: Token-form login still works
- **Given** Telegram OAuth or machine client posts the shared token
  via the existing `/v1/web/login` form path
- **When** the request contains only `token=<hex>` (no `username` or
  `password`)
- **Then** the existing behaviour is unchanged: token verified, cookie
  set, 303 to `/`

## Acceptance
- AC-1: New table `web_user_credentials` exists with columns
  `username TEXT PRIMARY KEY`, `password_hash TEXT NOT NULL`,
  `created_at TIMESTAMPTZ`, `last_login_at TIMESTAMPTZ NULL`.
- AC-2: `POST /v1/web/login` with form fields
  `username=...&password=...` returns 303 + cookie when credentials
  match; 401-equivalent error render when they do not.
- AC-3: `smackerel-core users add <name>` and
  `smackerel-core users set-password <name>` work end-to-end and
  prompt for password on TTY without echoing.
- AC-4: Existing `token=<hex>` form path is unchanged (regression
  test).
- AC-5: Unknown-user timing matches known-user-wrong-password timing
  (no user-enumeration leak via timing).
- AC-6: Login form HTML (`internal/api/admin_ui_static/login.html`)
  shows username + password fields above the existing token field;
  token field stays as a fallback for machine clients.

## Security Model
- Passwords stored as `argon2id` hashes with parameters
  `time=1, memory=64MB, threads=4, keyLen=32, saltLen=16` (encoded
  in the standard `$argon2id$v=19$...` PHC string).
- Verification is constant-time per the argon2 PHC reader.
- No password complexity requirements enforced server-side (operator
  trust model; CLI may print a length-suggestion warning).
- No account lockout (Caddy / future spec can layer rate limiting).
- Form posts are rate-limited by the existing
  `r.Use(httprate.LimitByIP(20, 1*time.Minute))` group in
  `internal/api/router.go`.
- On successful user+pass verify, the cookie value is the existing
  shared `AuthToken`. This means a web user gets the same access as
  the shared token — full admin. Documented and explicit.
- Web users SHOULD NOT be granted to non-operators. Same trust band
  as the shared token.

## Out-of-Scope Anti-Patterns (DO NOT BUILD)
- Storing passwords in plaintext or with reversible encryption.
- "Remember me" cookies independent of `auth_token`.
- Per-page re-authentication.
- Sending password reset emails (no email infra in scope).
- Returning the hash or any other secret in the API response.

## Dependencies
- Spec 044 (per-user PASETO bearer) — unchanged.
- Spec 057 (form-encoded `/v1/web/login`) — extended.
- Spec 060 (scope claims) — unchanged.

## Open Questions Resolved
- **Q: Should successful login mint a per-user PASETO instead of
  reusing the shared token?**
  A: No. Out of scope. Trust band is identical to the shared token;
  the user table is a UX layer, not a privilege layer. Document
  clearly. If per-user privilege ever becomes a requirement, file a
  followup spec.
- **Q: Should the legacy token field be removed from the login form?**
  A: No. Telegram OAuth callback and machine clients still POST the
  token field. Keep it visible (collapsible "Machine client login"
  section) to avoid breaking those.
