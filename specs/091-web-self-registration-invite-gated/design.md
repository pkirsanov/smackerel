# Design 091 — Web Self-Registration (Invite-Token Gated)

**Phase:** design · **Owner:** bubbles.design · **Status:** in_progress
**Spec:** [spec.md](spec.md) · **Mode:** full-delivery · **Status ceiling:** done
**Analysis depth:** `from-analysis` (spec.md carries analyst BDD + ux `## UX Specification`,
`## UI Wireframes`, `## User Flows`) — this design is contract-grade.

---

## Design Brief

**Current State.** The only way to create a smackerel-core web login account today is
the TTY-only CLI from spec 070 (`smackerel-core users add <name>` via `docker exec`).
The browser surface has `GET /login` ([internal/api/web_login_page.go](../../internal/api/web_login_page.go))
+ `POST /v1/web/login` ([internal/api/web_login.go](../../internal/api/web_login.go)),
both backed by the argon2id credential store
[internal/auth/webcreds](../../internal/auth/webcreds/repo.go). There is **no** `/register`
surface (greenfield).

**Target State.** Add a browser self-registration surface — `GET /register`
(page) + `POST /v1/web/register` (intake) — gated by a **repeatable invite token**
the operator holds. A successful POST creates one `web_user_credentials` row
(argon2id) and **303-redirects to `/login?registered=1`** (no cookie); the operator
then logs in through the unchanged spec-070 flow and reaches `/cards`. The invite
token is a **dedicated managed secret** (`WEB_REGISTRATION_INVITE_TOKEN`); empty/unset
⇒ registration disabled (fail-loud at POST, never open signup).

**Patterns to Follow.**
- GET page: mirror [web_login_page.go](../../internal/api/web_login_page.go) `HandleLoginPage`
  — embedded `html/template`, `loginUIFS` embed.FS, header trio
  (`Content-Type: text/html; charset=utf-8`, `Cache-Control: no-store`,
  `X-Content-Type-Options: nosniff`), `sanitizeNext` on `?next=`, HEAD short-circuit.
- POST handler: mirror `HandleWebLogin`’s form branch — `isFormContentType`,
  `http.MaxBytesReader(w, r.Body, 64*1024)`, `r.ParseForm()`, `subtle.ConstantTimeCompare`,
  `sanitizeNext`, 303-on-success, re-render-on-failure (`renderLoginError` →
  `renderRegisterError`).
- Credential store: reuse `webcreds.Repo.UpsertPassword(ctx, username, password, create=true)`
  verbatim (returns `ErrUserExists`), `webcreds.ValidateUsername`, `webcreds.MinPasswordLength`.
- Routing: register alongside `/login` + `/v1/web/login` — GET public, POST inside the
  existing `httprate.LimitByIP(20, 1*time.Minute)` group, both OUTSIDE `bearerAuthMiddleware`
  ([router.go](../../internal/api/router.go) ~L325-336).
- Secret: mirror `AUTH_BOOTSTRAP_TOKEN`’s SST 3-mirror + placeholder-emission wiring
  ([secret_keys.go](../../internal/config/secret_keys.go), [config.sh](../../scripts/commands/config.sh),
  [smackerel.yaml](../../config/smackerel.yaml)) — **except** the production-required `authErrors` block.

**Patterns to Avoid.**
- **Do NOT reuse `AUTH_BOOTSTRAP_TOKEN`** (one-shot, production-required, cleared after
  first use — see Settled Decision #1). The invite token must be repeatable and optional.
- **Do NOT add a release-train feature flag** (see Settled Decision #3); secret-presence
  is the rollout control.
- **Do NOT set a session cookie in the register handler** (the cookie stays minted only by
  `POST /v1/web/login`; see Architecture §“No cookie on register”).
- **Do NOT render a distinct “registration unavailable” GET page** (it would leak the
  gate state and defeat AC-10 non-enumeration; see Reconciled Requirements → AC-5).
- **Do NOT evaluate username/password before the invite-token gate** (enumeration leak).
- **Do NOT add a `${VAR:-default}` fallback** for the invite token (NO-DEFAULTS SST;
  empty = disabled is the explicit, documented behavior).

**Resolved Decisions.**
- **#1 Secret identity →** dedicated `WEB_REGISTRATION_INVITE_TOKEN` (optional managed secret).
- **#2 Post-registration landing →** redirect-to-`/login?registered=1`, no cookie (settled by ux).
- **#3 Release-train flag →** none; `flagsIntroduced: []` stays empty.
- **AC-5 reconciliation →** GET always renders the identical form; the disabled gate is
  enforced only at POST returning the shared non-enumerating banner.

**Open Questions.** None blocking. One non-blocking hardening (placeholder-leak guard at the
wiring boundary) is recommended and specified in §Security; the plan phase may fold it into
SCOPE wiring or accept it as documented defense-in-depth.

---

## Purpose & Scope

**In scope (this repo, smackerel):**
1. `GET /register` page (embedded CSP-safe template + reused `login.css` + new `register.js`).
2. `POST /v1/web/register` handler (`HandleWebRegister`) — token-gated account intake.
3. Dedicated managed secret `WEB_REGISTRATION_INVITE_TOKEN` (SST 3-mirror + placeholder emission +
   config load + Dependencies wiring) — **optional** (empty ⇒ disabled), NOT production-required.
4. Additive `/login` success-flash on `?registered=1`.
5. Test strategy handed to the plan phase (unit table, regression, live e2e).

**Out of scope:**
- The knb-side deploy wiring (sops-encrypt + apply.sh substitution) — **named here, executed in the
  separate `<knb-repo>` repo as its own commit** (§knb Deploy Wiring).
- Any change to `web_user_credentials` schema (migration 044 is reused unchanged).
- Per-user roles, email/MFA/reset, an in-app invite-token admin surface (spec Non-Goals).

---

## Settled Open Decisions

### Decision #1 — Invite-token secret identity → **dedicated `WEB_REGISTRATION_INVITE_TOKEN`** (BINDING)

**Chosen:** introduce a **new dedicated managed secret `WEB_REGISTRATION_INVITE_TOKEN`**.
**Rejected:** reusing the existing `AUTH_BOOTSTRAP_TOKEN`.

**Rationale.** `AUTH_BOOTSTRAP_TOKEN` is a **one-shot bootstrap credential**: it authorizes
first-user enrollment on a fresh production deployment, is consumed exactly once via
`./smackerel.sh auth bootstrap`, and is then **cleared by the operator** (spec 044 OQ-10;
[config.go](../../internal/config/config.go) L519-522). It is also **production-REQUIRED** —
`loadAuthConfig` appends it to `authErrors` and refuses to produce a `Config` when
`SMACKEREL_ENV=production && auth.enabled=true && BootstrapToken==""`
([config.go](../../internal/config/config.go) L1607-1609). Spec 091 requires the **opposite**
properties: the invite gate must be **repeatable** (Goal 2 — "the same valid invite token can
create multiple accounts over time, not consumed on first use") and **optional** (Hard Constraint —
"an empty/unset invite token disables registration entirely … never falls back to open signup").
Conflating the two would either (a) break bootstrap’s one-shot/clear-after-use contract, or
(b) make registration single-use, or (c) force registration to be production-required when it must
be opt-in. The two secrets have orthogonal lifecycles; they must be distinct.

**Key distinction from `AUTH_BOOTSTRAP_TOKEN`: the invite token is OPTIONAL, so it is NOT added to
the production-required `authErrors` block.** It is a managed secret in the SST manifest (so it can
be shipped securely via the knb sops file and placeholder-emitted for self-hosted), but its empty value
is **valid in every environment** and means "registration off." Enforcement is **fail-loud-at-POST**
(the handler refuses every submission when the configured token is empty) — **not**
fail-loud-at-boot. This honors the repo NO-DEFAULTS SST policy ("empty = disabled is the documented,
intended behavior," not a hidden fallback) while keeping the boot path unchanged for deployments that
do not enable registration.

### Decision #3 — Release-train feature flag → **none** (BINDING)

**Chosen:** **no** separate release-train feature flag. **`flagsIntroduced: []` stays empty.**

**Rationale.** The **secret-presence gate is itself the rollout control**: registration is live
only where the operator has shipped a non-empty `WEB_REGISTRATION_INVITE_TOKEN` (i.e., the `mvp`
self-hosted target via the knb sops file); every other environment leaves it empty and registration is
off. Per the bubbles-release-train model, a feature flag must be declared default-OFF in **every**
train’s `feature-flags.<train>.yaml` bundle and read from an env var with no fallback — that is real
ceremony (a new flag name, a per-train bundle entry in each train, a fail-fast env read, and the
flag-lifecycle "dies with its train + 1 cycle" obligation) for **zero** incremental control here: the
secret already gates the feature per-environment with finer granularity (you can rotate or remove it
without a train cut). Spec 070’s login layer — which this extends and which carries the **same**
full-admin trust band — introduced **no** train flag for the identical reason. Adding one for the
register sibling would be inconsistent and add governance surface with no benefit. The secret gate is
strictly more expressive than a boolean train flag for this feature.

> If a future spec needs registration to ship dark on the `mvp` train while the secret is already
> present (decoupling "secret configured" from "feature visible"), that spec can introduce
> `webRegistrationEnabled` then. Today the two are intentionally the same switch.

---

## Reconciled Requirements

> This design holds **design authority** over the analyst-owned acceptance criteria where ux
> surfaced a contradiction. One minimal edit is made to `spec.md`.

### AC-5 (reconciled) — GET always renders the form; the gate is POST-only

**Finding (routed from ux → design).** The original AC-5 said the empty-token `GET /register`
"renders a 'registration unavailable' state." That contradicts AC-10 (non-enumeration: failures must
not reveal whether the gate is configured) and the ux Disabled-State decision. A GET page that
rendered a distinct "unavailable" state when the token is unset would let an unauthenticated prober
learn the gate is off with a single `GET /register`, making the carefully-shared POST banner
pointless.

**Resolution (applied to `spec.md` AC-5 text).** `GET /register` renders the **identical** form
regardless of token configuration; the disabled gate is enforced **only** at
`POST /v1/web/register`, which returns the **shared, non-enumerating banner**
`Registration is not available or the invite is invalid.` (byte-identical to a wrong-token
response). This makes enabled-vs-disabled indistinguishable end-to-end and preserves AC-10.

**Edit made:** `spec.md` → Acceptance Criteria → AC-5 reworded (see spec.md). No other AC changed.
AC-10’s non-enumeration property is now internally consistent with AC-5.

> All other acceptance criteria (AC-1..AC-4, AC-6..AC-10) are accepted as written; this design
> elaborates them into concrete contracts below without altering their intent.

---

## Architecture Overview

### New / modified files (this repo)

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/api/web_register_page.go` | **new** | `HandleRegisterPage` (GET `/register`); `registerPageData`; `registerTemplate` parsed from the shared `loginUIFS`. Mirrors `web_login_page.go`. |
| `internal/api/web_register.go` | **new** | `HandleWebRegister` (POST `/v1/web/register`); `renderRegisterError`; the security-critical control flow. Mirrors the form branch of `web_login.go`. |
| `internal/api/admin_ui_static/register.html` | **new** | CSP-safe embedded template — `<main>/<h1>`, label-wrapped `username`/`password`/`confirm-password`/`invite-token`, hidden `next`, one submit, `role="alert"` error banner, `/login` cross-link. Reuses `login.css`. |
| `internal/api/admin_ui_static/register.js` | **new** | Progressive-enhancement only — focuses the username field on load (CSP `script-src 'self'`). No validation logic. |
| `internal/api/web_login_page.go` | **modify** | Extend the `//go:embed` directive to include `register.html` + `register.js` so the existing `/admin_ui_static/*` file server serves them and `register.html` is embeddable. |
| `internal/api/admin_ui_static/login.html` | **modify (additive)** | Render `Account created — sign in.` (`banner-success`, `role="status"`) when `?registered=1`. |
| `internal/api/admin_ui_static/login.css` | **modify (additive)** | Add a `.banner-success` palette (WCAG-AA contrast). |
| `internal/api/web_login_page.go` (`loginPageData`) | **modify (additive)** | Add `Registered bool`; set from `r.URL.Query().Get("registered") == "1"` in `HandleLoginPage`. |
| `internal/api/router.go` | **modify** | Register `GET /register` (public) + `POST /v1/web/register` (inside the `LimitByIP(20,1min)` group). |
| `internal/api/health.go` (`Dependencies`) | **modify (additive)** | Add `WebRegistrationInviteToken string`. |
| `cmd/core/wiring.go` | **modify** | Set `deps.WebRegistrationInviteToken` from `cfg.Auth.WebRegistrationInviteToken` (with the placeholder-leak guard, §Security). |
| `internal/config/config.go` | **modify** | Add `AuthConfig.WebRegistrationInviteToken string`; load it in `loadAuthConfig`. |
| **SST 3-mirror + test** (4 files) | **modify** | Register the new managed secret (§Configuration & SST Secret Wiring). |

**No new package, no new store, no new migration.** The register handler is a second consumer of
the existing `webcreds.Repo`; the register page is a second consumer of the existing `loginUIFS`
embed + `/admin_ui_static/*` serving contract + header trio.

### Asset embedding & serving (no new router asset route)

`register.html` + `register.js` are added to the **existing** `//go:embed` directive in
`web_login_page.go` (the `loginUIFS embed.FS`). The router already serves
`r.Handle("/admin_ui_static/*", http.StripPrefix("/", http.FileServer(http.FS(loginUIFS))))`
([router.go](../../internal/api/router.go) L336) — so `register.js` and the reused `login.css` are
served with **no router change** for assets. `register.html` is parsed via
`template.Must(template.ParseFS(loginUIFS, "admin_ui_static/register.html"))` in
`web_register_page.go` (same package, shares the embed FS).

### No cookie on register (trust-boundary invariant)

Per ux Decision #2, **`HandleWebRegister` sets no `Set-Cookie`**. The `auth_token` session cookie is
minted **only** by `POST /v1/web/login` (`d.authCookie(...)` in `web_login.go`). Registration is pure
intake: it creates a row and grants no session. This keeps the "create account" and "establish
session" boundaries clean — a register-handler bug cannot mint a full-admin session — and avoids
duplicating any cookie / `Secure` / `SameSite` logic. A successful register returns
`303 → /login?registered=1` (+ sanitized `next`).

---

## Capability Foundation (DE4)

**Trigger:** ux defined **shared UI primitives across two screens** (`/register` and `/login`):
Banner, Form field, Submit button, Auth cross-link, the shared non-enumerating string, and the
asset/serving contract. That is a "shared UI surface across ≥2 screens" proportionality trigger.

**Foundation already exists — `/register` is a second consumer, not a new abstraction.** The shared
foundation is the **spec-057 login asset + serving contract**:
- the `loginUIFS` embed.FS,
- the `/admin_ui_static/*` `http.FileServer` route,
- the header trio (`Cache-Control: no-store`, `X-Content-Type-Options: nosniff`,
  `Content-Type: text/html; charset=utf-8`) + CSP `script-src 'self'` discipline,
- the `login.css` stylesheet (banner palette, single-column `max-width` layout),
- the `sanitizeNext` next-sanitization primitive,
- the embedded-`html/template` GET-page pattern (`HandleLoginPage`) and the form-POST
  re-render-on-error pattern (`renderLoginError`).

This design **reuses** that foundation verbatim (extend the embed, reuse `login.css`, mirror the
handler shapes) and adds `register.html`/`register.js` + `HandleRegisterPage`/`HandleWebRegister` as a
**second concrete consumer**. The one **new** shared content primitive — the success banner
(`banner-success`) — is added additively to the foundation’s stylesheet and login template so both
screens can use it.

### Variation Axes
- **Intake vs. session axis:** `/login` establishes a session (mints `auth_token`); `/register`
  performs intake only (no cookie). Same asset/serving foundation, opposite session behavior.
- **Gate axis:** `/login` gate = credential verify (`VerifyAndTouch`); `/register` gate =
  constant-time invite-token compare **then** credential create (`UpsertPassword(create=true)`).

### Single-Implementation Justification
_Handler layer._ The **registration intake handler itself** is intentionally a single concrete implementation: there
is exactly one registration path (invite-token-gated, argon2id, full-admin band). There is no second
provider/strategy/variant of "register a web user," so no provider/adapter abstraction is introduced
for the handler — that would be speculative generality (YAGNI). The shared-surface obligation is fully
discharged by reusing the existing login asset foundation above, not by inventing a new one.

---

## Data Model

**No schema change.** Registration reuses migration
[044_web_user_credentials.sql](../../internal/db/migrations) — table `web_user_credentials`
(`username` PK, `password_hash`, `created_at`, `last_login_at`). Account creation is a single
`webcreds.Repo.UpsertPassword(ctx, username, password, create=true)` call, which:
- runs `ValidateUsername` (≤ 64 runes, trimmed, no control chars),
- computes the argon2id PHC hash via `Hash` (enforces `MinPasswordLength = 12`),
- `INSERT`s a new row, returning `ErrUserExists` when the username already has a row
  (`create=true` ⇒ **no overwrite**, AC-3 / UC-4).

`created_at` is set by the table default; `last_login_at` stays NULL until the new user’s first
`POST /v1/web/login` (`VerifyAndTouch` updates it). The plaintext password is never persisted and
never logged (argon2id at rest, AC-10).

**Storage policy:** PostgreSQL only (via the existing `*pgxpool.Pool` in
`webcreds.NewPostgresRepo`). No new store, no embedded DB, no cache as a data source.

---

## Configuration & SST Secret Wiring (`WEB_REGISTRATION_INVITE_TOKEN`)

The invite token is a **dedicated managed secret**. The bundle 3-mirror contract is enforced by
[internal/deploy/bundle_secret_contract_test.go](../../internal/deploy/bundle_secret_contract_test.go)
(byte-for-byte parity, including order, across yaml + Go + shell) and pinned by
[internal/config/secret_keys_test.go](../../internal/config/secret_keys_test.go). **All four
mirror-bearing files plus the loader and emission blocks must be edited in one change set**, or the
contract test fails loud. Each exact edit site is enumerated below (append at the **end** of each
ordered list, after `CARD_REWARDS_GCAL_CREDENTIALS`, to keep the byte-for-byte order parity simple).

### Exact edit sites

| # | File | Site (current anchor) | Edit |
|---|------|-----------------------|------|
| **1** | `config/smackerel.yaml` | `infrastructure.secret_keys:` list (~L1702-1718), last entry `- CARD_REWARDS_GCAL_CREDENTIALS` | Append `- WEB_REGISTRATION_INVITE_TOKEN` with a doc comment (spec 091; OPTIONAL — empty ⇒ registration disabled; read by core `HandleWebRegister`; never logged). |
| **2** | `config/smackerel.yaml` | `auth:` block, after `bootstrap_token: ""` (~L829) | Add `web_registration_invite_token: ""` (dev/test default empty = registration off locally). |
| **3** | `internal/config/secret_keys.go` | `var secretKeys = []string{ … }` (~L28-46), last entry `"CARD_REWARDS_GCAL_CREDENTIALS",` | Append `"WEB_REGISTRATION_INVITE_TOKEN",` + comment. |
| **4** | `scripts/commands/config.sh` | `SHELL_SECRET_KEYS=( … )` (~L382-390), last entry `CARD_REWARDS_GCAL_CREDENTIALS` | Append `WEB_REGISTRATION_INVITE_TOKEN`. |
| **5** | `scripts/commands/config.sh` | placeholder-emission block (mirror the `AUTH_BOOTSTRAP_TOKEN` block at ~L1319-1322) | Add: `if is_production_class_target "$TARGET_ENV" && in_secret_keys "WEB_REGISTRATION_INVITE_TOKEN"; then WEB_REGISTRATION_INVITE_TOKEN="__SECRET_PLACEHOLDER__WEB_REGISTRATION_INVITE_TOKEN__"; else WEB_REGISTRATION_INVITE_TOKEN="$(yaml_get auth.web_registration_invite_token 2>/dev/null)" || WEB_REGISTRATION_INVITE_TOKEN=""; fi`. |
| **6** | `scripts/commands/config.sh` | `app.env` heredoc emission (mirror `AUTH_BOOTSTRAP_TOKEN=${AUTH_BOOTSTRAP_TOKEN}` at ~L2096) | Add line `WEB_REGISTRATION_INVITE_TOKEN=${WEB_REGISTRATION_INVITE_TOKEN}`. |
| **7** | `internal/config/secret_keys_test.go` | `TestSecretKeysMirror` hardcoded `want := []string{ … }` (~L103-111), last entry `"CARD_REWARDS_GCAL_CREDENTIALS",` | Append `"WEB_REGISTRATION_INVITE_TOKEN",` so the in-memory pin matches the new manifest. |
| **8** | `internal/config/config.go` | `AuthConfig` struct (~L461-523), after `BootstrapToken string` (L522) | Add field `WebRegistrationInviteToken string` with doc comment (OPTIONAL; empty ⇒ registration disabled). |
| **9** | `internal/config/config.go` | `loadAuthConfig` (~L1520, after `cfg.Auth.BootstrapToken = os.Getenv("AUTH_BOOTSTRAP_TOKEN")`) | Add `cfg.Auth.WebRegistrationInviteToken = os.Getenv("WEB_REGISTRATION_INVITE_TOKEN")`. **Do NOT add it to the production `authErrors` block (~L1590-1620)** — it is optional. |
| **10** | `internal/api/health.go` | `Dependencies` struct, near `WebCredentials webcreds.Repo` (L173) | Add `WebRegistrationInviteToken string` with doc comment (empty ⇒ POST registration disabled). |
| **11** | `cmd/core/wiring.go` | `&api.Dependencies{ … }` (~L196-230) or just after `deps.WebCredentials = webCredsRepo` (L422) | Set `deps.WebRegistrationInviteToken = …` from `cfg.Auth.WebRegistrationInviteToken`, applying the placeholder-leak guard (§Security). |

> `TestSecretKeys_MirrorsYAMLManifest` ([secret_keys_test.go](../../internal/config/secret_keys_test.go))
> compares the yaml list to `config.SecretKeys()` dynamically — no edit needed there; it will simply
> pass once edits #1 and #3 land. `bundle_secret_contract_test.go` reads `config.SecretKeys()`
> dynamically — no edit needed; it enforces that #1/#3/#4 agree and that self-hosted emits the
> placeholder. Edits #5 + #6 are what make the shell loader actually emit the placeholder + the
> `app.env` line; without them the bundle contract test’s self-hosted placeholder assertion fails.

### Why placeholder emission (self-hosted) is required

`self-hosted` is a `production_class_target`, so for **every** secret in the manifest the SST loader
emits `__SECRET_PLACEHOLDER__<KEY>__` in the bundle’s `app.env`, and the knb deploy adapter
substitutes the real value at apply time. Adding `WEB_REGISTRATION_INVITE_TOKEN` to the manifest
without the emission block (#5) + `app.env` line (#6) would make the bundle contract test fail (the
key would be declared but not emitted). This mirrors `CARD_REWARDS_GCAL_CREDENTIALS` and
`AUTH_BOOTSTRAP_TOKEN` exactly.

---

## knb Deploy Wiring (separate repo — named, NOT executed here)

> The knb deploy-adapter overlay lives in the separate `<knb-repo>` repository and is **its own
> commit, outside this spec’s execution scope**. This section names the required follow-up so the
> deploy phase can resume without re-discovery. Model: the `CARD_REWARDS_GCAL_CREDENTIALS` wiring in
> `<knb-repo>` (commit `1856ca3`).

1. **sops-encrypt** `WEB_REGISTRATION_INVITE_TOKEN` into `<knb-repo>/smackerel/secrets/self-hosted.enc.env`
   with a strong random value (to enable registration) **or an empty value** (to keep it disabled).
2. **apply.sh drift-check block:** add `WEB_REGISTRATION_INVITE_TOKEN` to the adapter’s expected-secret
   drift check (mirror the `CARD_REWARDS_GCAL_CREDENTIALS` / `AUTH_BOOTSTRAP_TOKEN` entries) so a
   missing/extra secret is caught.
3. **apply.sh placeholder-emission/substitution block:** add the substitution mapping
   `__SECRET_PLACEHOLDER__WEB_REGISTRATION_INVITE_TOKEN__` → the decrypted sops value (mirror the
   `CARD_REWARDS_GCAL_CREDENTIALS` substitution). The adapter MUST substitute **every** manifest
   placeholder; an un-substituted placeholder is a deploy-time error (see the Go-side guard in
   §Security as belt-and-suspenders).
4. **Redeploy** with `--trust-model=ci-keyless` (the standard smackerel self-hosted trust model).

Value-safe handling throughout (no echoing the secret value; sops at rest; presence-only
diagnostics) per the repo terminal-discipline + secret-hygiene policies.

---

## API Contracts

### `GET /register` (new, public)

| Property | Value |
|----------|-------|
| Method | `GET` (and `HEAD`) — other methods → `405` |
| Auth | **none** (OUTSIDE `bearerAuthMiddleware`) |
| Query | `?next=<path>` — read once, `sanitizeNext`-sanitized, embedded as hidden `next` |
| Response | `200` `text/html; charset=utf-8`; headers `Cache-Control: no-store`, `X-Content-Type-Options: nosniff` |
| Body | `register.html` rendered with `registerPageData{Next, Username:"", Error:""}` — the **identical** form regardless of token config (Reconciled AC-5) |
| Handler | `(*Dependencies).HandleRegisterPage` |

`registerPageData` (template input):

```go
type registerPageData struct {
    Next     string // sanitized ?next, echoed into hidden field
    Username string // preserved on POST re-render; blank on first GET
    Error    string // banner text on POST re-render; blank on GET
}
```

### `POST /v1/web/register` (new, rate-limited)

| Property | Value |
|----------|-------|
| Method | `POST` only — else `405` |
| Auth | **none** (OUTSIDE `bearerAuthMiddleware`); INSIDE `httprate.LimitByIP(20, 1*time.Minute)` |
| Content-Type | `application/x-www-form-urlencoded` (`isFormContentType`); body capped `http.MaxBytesReader(w, r.Body, 64*1024)` |
| Form fields | `username`, `password`, `confirm-password`, `invite-token`, `next` (hidden) |
| Success | `303 See Other` → `Location: /login?registered=1` (+ `&next=<sanitized>` when a non-default next was supplied); **no `Set-Cookie`** |
| Failure | re-render `register.html` with the matching banner + preserved `username` + status code (table below) |
| Handler | `(*Dependencies).HandleWebRegister` |

**Status / error-string contract** (exact strings, from the ux catalog; the **tokenless** path always
yields the single shared `401`):

| Condition | Status | Banner string | Enumerating? |
|-----------|--------|---------------|--------------|
| Wrong / missing / **empty-configured** invite token, **or** `WebCredentials == nil` | `401` | `Registration is not available or the invite is invalid.` | No (shared, byte-identical) |
| Token OK, a required field empty | `400` | `All fields are required.` | No |
| Token OK, `password != confirm-password` | `400` | `Passwords do not match.` | No |
| Token OK, `len(password) < 12` | `400` | `Password must be at least 12 characters.` | No |
| Token OK, `ValidateUsername` rejects | `400` | `Username must be 64 characters or fewer and contain no control characters.` | No |
| Token OK, fields OK, `UpsertPassword(create=true)` → `ErrUserExists` | `409` | `That username is taken.` | Acceptable (valid-token holder only) |
| Token OK, unexpected error (e.g. DB failure) | `500` | `Something went wrong. Please try again.` | No |

### `GET /login` (modified — additive success flash)

| Property | Value |
|----------|-------|
| New query | `?registered=1` → renders `Account created — sign in.` (`banner banner-success`, `role="status"`) above the form |
| Regression | absent `?registered=1` ⇒ **byte-identical** to spec-057/070 behavior (AC-9) |
| Handler | `(*Dependencies).HandleLoginPage` — set `loginPageData.Registered = r.URL.Query().Get("registered") == "1"` |

`loginPageData` gains one additive field:

```go
type loginPageData struct {
    AuthEnabled bool
    Next        string
    Error       string
    Registered  bool // NEW: render the post-registration success flash on ?registered=1
}
```

---

## POST Handler Control Flow (security-critical ORDER)

`HandleWebRegister` MUST evaluate steps in this exact order. **The invite-token gate is step 3 —
before any username/password evaluation** — so a request without a valid token can never produce a
username-existence or field-specific signal (AC-10 / UC-2 non-enumeration).

1. **Method guard.** Non-`POST` → `405` (`method_not_allowed`).
2. **Content-type + parse.** `isFormContentType` (else generic reject); `r.Body =
   http.MaxBytesReader(w, r.Body, 64*1024)`; `defer r.Body.Close()`; `r.ParseForm()` (on error →
   `generic-server-error`). Read `next := r.PostForm.Get("next")`, `invite :=
   r.PostForm.Get("invite-token")`.
3. **Invite-token gate (FIRST). Constant-time, value-safe.**
   - `configured := d.WebRegistrationInviteToken`.
   - **Disabled / unavailable:** if `d.WebCredentials == nil` **or** `configured == ""` →
     `renderRegisterError(w, r, sanitizeNext(next), preservedUsername="", shared-banner, 401)`,
     **STOP**. (The `configured == ""` check is a plain comparison of a server-config constant — it
     involves **no secret material**, so it does not need constant-time treatment; AC-4’s constant-time
     requirement is specifically about comparing the **configured secret value**.)
   - **Mismatch:** else if `subtle.ConstantTimeCompare([]byte(invite), []byte(configured)) != 1` →
     `renderRegisterError(... shared-banner, 401)`, **STOP**. The wrong-token and disabled responses
     are **byte-identical** (same string, same `401`, same blank-secrets re-render). The invite value
     is **never** logged, echoed, or placed in the redirect/template.
   > Why the empty-configured early-return is safe vs. enumeration: an attacker can only observe the
   > shared `401` banner; whether they hit the `configured == ""` branch or the `ConstantTimeCompare`
   > mismatch branch is indistinguishable in the response. The only difference is an O(1) branch on a
   > **server-side constant** (not attacker input, not the secret), so there is no password/secret
   > timing side-channel. A naive single `ConstantTimeCompare` would be **wrong** here: when
   > `configured == ""` and `invite == ""`, the compare returns 1 (match) → that would be **open
   > signup**. The explicit empty-configured guard is what prevents that.
4. **Field presence** (reached only after a valid token). Trim `username`; read `password`,
   `confirm`. If any of `username`/`password`/`confirm` empty → `missing-field` (`400`).
5. **Password rules.** If `password != confirm` → `passwords-mismatch` (`400`). Else if
   `len(password) < webcreds.MinPasswordLength` (12) → `password-too-short` (`400`).
6. **Username validity.** `webcreds.ValidateUsername(username)` non-nil → `invalid-username` (`400`).
7. **Create.** `err := d.WebCredentials.UpsertPassword(r.Context(), username, password, true)`:
   - `errors.Is(err, webcreds.ErrUserExists)` → `duplicate-username` (`409`), **existing hash
     unchanged**.
   - other non-nil `err` → `generic-server-error` (`500`).
   - `nil` → **success**: `http.Redirect(w, r, "/login?registered=1"+optionalNext, http.StatusSeeOther)`,
     **no cookie**.

Because steps 4-7 are reachable **only after** a valid token (step 3), their distinct, helpful
messages are safe — they are seen exclusively by a holder of the operator’s trusted secret, already in
the full-admin band.

`renderRegisterError(w, r, next, username, msg, status)` re-renders `register.html` with
`registerPageData{Next: next, Username: username, Error: msg}` and the given status. **The `username`
is echoed (the user’s own input, auto-escaped by `html/template`); `password`, `confirm-password`, and
`invite-token` are ALWAYS rendered empty** (secret-preservation invariant). On the shared-token reject
the preserved username is blank (the request never advanced past the gate).

> **No-enumeration & redundancy note:** `UpsertPassword` itself re-runs `ValidateUsername` + `Hash`
> (min-length). The handler pre-checks (steps 5-6) exist to return the **distinct** field-level
> messages the ux catalog requires (the store returns generic errors); they are not duplicated
> validation for its own sake but the source of the user-facing message granularity. The store’s
> `create=true`/`ErrUserExists` remains the authoritative no-overwrite guarantee.

---

## Dependency Wiring

`Dependencies` already carries `WebCredentials webcreds.Repo` (health.go L173, wired at
wiring.go L422) and `AuthToken` / `Environment`. Add **one** field:

```go
// Dependencies (internal/api/health.go), near WebCredentials:
//
// WebRegistrationInviteToken is the spec-091 self-registration gate
// secret (WEB_REGISTRATION_INVITE_TOKEN). OPTIONAL: empty ⇒ POST
// /v1/web/register is disabled (fail-loud at POST, never open signup).
// Compared constant-time; never logged.
WebRegistrationInviteToken string
```

Wired in `cmd/core/wiring.go` from `cfg.Auth.WebRegistrationInviteToken`. An empty value disables POST
registration; a non-empty value enables it. The register handler reuses the **already-wired**
`deps.WebCredentials` repo — no new store construction.

---

## Security & Compliance

- **Constant-time secret compare (AC-4).** Non-empty invite path uses
  `subtle.ConstantTimeCompare([]byte(submitted), []byte(configured))`. The empty-configured guard is a
  plain comparison of a server constant (no secret), preceding the compare, and prevents the
  empty-matches-empty open-signup trap.
- **Non-enumerating, value-safe (AC-10).** Wrong-token, missing-token, and disabled-gate all return the
  **same** `401` + same banner + same blank-secrets re-render. The invite value and any password value
  never appear in a log line, metric label, error body, redirect, or template field. All echoes (the
  preserved username) go through `html/template` auto-escaping — never string concatenation.
- **No overwrite (AC-3 / UC-4).** `UpsertPassword(create=true)` → `ErrUserExists` guarantees a
  duplicate username cannot replace an existing hash.
- **Same full-admin trust band (spec 070).** A registered account’s later `/login` mints the same
  `auth_token` cookie value (`d.AuthToken`) as the shared-token path — full admin, no escalation, no
  reduction. Documented and explicit. The invite token is shared only with people the operator already
  trusts at that level.
- **Argon2id at rest.** Passwords stored only as argon2id PHC strings via `webcreds.Hash`; plaintext
  never persisted, never logged.
- **Rate limited (AC-7 / UC-8).** `POST /v1/web/register` joins the existing
  `httprate.LimitByIP(20, 1*time.Minute)` group; excess → `429` (middleware default, no custom page —
  matches existing login behavior).
- **CSP (AC-1).** `register.html` carries no inline scripts / no inline event handlers; `register.js`
  is `script-src 'self'`, focus-only progressive enhancement; the page is fully operable with JS off.
- **NO-DEFAULTS SST.** No `${VAR:-default}` for the invite token anywhere. Empty ⇒ disabled is the
  explicit, documented behavior (not a hidden fallback). The secret is fail-loud-at-POST (refuses every
  submission), not fail-loud-at-boot (it is optional, unlike `AUTH_BOOTSTRAP_TOKEN`).

### Recommended hardening — un-substituted-placeholder guard (defense-in-depth, non-blocking)

`self-hosted` placeholder-emits the secret; the knb adapter substitutes the real value. If the adapter
**failed** to substitute (a deploy bug), the Go process would receive the literal
`__SECRET_PLACEHOLDER__WEB_REGISTRATION_INVITE_TOKEN__` as the configured value — a **publicly-known
constant** that would otherwise enable open admin signup (exactly the spec "Failure Condition"). To
close this at the system boundary, the wiring layer SHOULD map a placeholder-valued invite token to
empty (= disabled):

```go
// cmd/core/wiring.go — at the boundary where cfg → deps:
inviteTok := cfg.Auth.WebRegistrationInviteToken
if config.IsPlaceholder(inviteTok) { // un-substituted bundle placeholder ⇒ treat as disabled
    inviteTok = ""
}
deps.WebRegistrationInviteToken = inviteTok
```

`config.IsPlaceholder` already exists ([secret_keys.go](../../internal/config/secret_keys.go)) and is
strict (only declared keys). This is one line at the trust boundary guarding a concrete security
failure mode, not speculative generality. The plan phase may fold it into the wiring scope or accept
it as documented defense-in-depth on top of the primary "knb always substitutes" contract.

---

## Observability & Failure Modes

- **Structured log on reject (value-safe).** On a failed registration, emit one `slog.Info` line
  mirroring the spec-070 `web_login_credential_fail` pattern: `kind=web_register_fail`,
  `remote_addr`, `username_len` (length only), and a coarse `reason` enum
  (`gate` | `field` | `duplicate` | `server`) — **never** the invite token, the username value, or any
  password. The shared `gate` reason covers wrong/missing/disabled so the log itself is
  non-enumerating about gate state.
- **No new metrics required.** Rate-limit `429`s are already counted by the existing httprate group;
  the feature adds no SLA-bound hot path (registration is rare, single-operator), so no stress/load
  scope is required by Gate G026.
- **Failure modes:** DB down → `500` `generic-server-error` (no row, no partial state);
  `WebCredentials == nil` (config-validate / pool-less test) → treated as disabled (`401` shared
  banner); malformed/oversized body → `MaxBytesReader` truncation → `generic-server-error`.

---

## Testing & Validation Strategy (handed to bubbles.plan)

Tests validate the **spec**, not the implementation. All unit tests use the existing in-memory
`fakeRepo` pattern ([web_login_credential_test.go](../../internal/api/web_login_credential_test.go)) —
no internal mocks of HTTP/router; live e2e proves the real stack at deploy.

### Unit — `HandleWebRegister` table (drive the handler directly, like `web_login_credential_test.go`)

| Case | Setup | Assert | Scenario |
|------|-------|--------|----------|
| Valid token + new user + matching pw | invite set; fakeRepo empty | `303` → `/login?registered=1`; **no `Set-Cookie`**; row created in fakeRepo | UC-1 / AC-2 |
| Valid token, repeatable | same invite, second distinct user | second `303` success; invite still valid (not consumed) | UC-1 (Goal 2) |
| Wrong token | invite set; submit wrong | `401`; shared banner; **no row** | UC-2 / AC-4 |
| Missing token | invite set; omit invite field | `401`; shared banner (byte-identical to wrong-token); no row | UC-2 |
| **Empty-configured token** | `WebRegistrationInviteToken == ""`; submit any token incl. empty | `401`; shared banner; no row (proves empty≠match — the open-signup trap) | UC-3 / AC-5 |
| `WebCredentials == nil` | nil repo; valid-looking submit | `401`; shared banner; no panic | UC-3 (disabled) |
| Duplicate username | invite set; fakeRepo has `operator`; submit `operator` | `409`; `That username is taken.`; existing hash unchanged (UpsertPassword create=true path) | UC-4 / AC-3 |
| Password mismatch | invite set; `password != confirm` | `400`; `Passwords do not match.`; no row | UC-5 |
| Password too short | invite set; `len(pw) < 12` | `400`; `Password must be at least 12 characters.`; no row | UC-5 / AC-6 |
| Missing field | invite set; blank username | `400`; `All fields are required.`; no row | UC-5 |
| Invalid username | invite set; 65-rune or control-char username | `400`; invalid-username string; no row | AC-3 |
| **Non-enumeration / constant-time** | wrong-token vs disabled responses | response bodies + status are **byte-identical**; invite token absent from body/headers/`Location` | AC-10 |
| **Value-safe logging** | capture `slog` output on a reject | log contains no invite value, no username value, no password | AC-10 |
| Method guard | `GET` to POST route | `405` | — |

### Unit — `HandleRegisterPage` (mirror `web_login_page.go` tests)

| Case | Assert |
|------|--------|
| GET renders form (token set) | `200`; form present; header trio set |
| GET renders **identical** form (token empty) | `200`; **same** form bytes as token-set case (Reconciled AC-5 — no "unavailable" variant) |
| `?next=//evil/` | hidden `next` is `sanitizeNext`-sanitized (no origin escape) |
| HEAD | `200`, empty body |

### Unit — router rate-limit membership (mirror [web_login_ratelimit_test.go](../../internal/api/web_login_ratelimit_test.go))

| Case | Assert | Scenario |
|------|--------|----------|
| `POST /v1/web/register` > 20/min from one IP through **real `NewRouter`** | a `429` is observed (limiter fires) | UC-8 / AC-7 |
| Per-IP isolation companion | distinct IPs are not collectively throttled (rules out tautological "everything 429s") | AC-7 |

> Adversarial fidelity (RED→GREEN, verified out-of-band by the implementer): temporarily registering
> `/v1/web/register` OUTSIDE the `LimitByIP` group makes the rate-limit test FAIL; restoring it
> turns GREEN — proving the test has bite (matches the spec-070 ratelimit test’s documented method).

### Unit — `/login` success-flash (additive, regression-guarding)

| Case | Assert | Scenario |
|------|--------|----------|
| `GET /login?registered=1` | `Account created — sign in.` banner present (`role="status"`) | UC-1 landing / AC-8 |
| `GET /login` (no query) | **byte-identical** to pre-091 render (no success banner) | AC-9 regression |

### Regression — spec 070 `/login` unchanged (UC-7 / AC-9)

Re-run / assert the existing spec-070 credential + token-form tests are green and unchanged:
token-only POST → cookie set; username+password → cookie set; invalid → same generic error. No change
to status, redirect, or cookie semantics. (These tests already exist; the design adds **no** edit to
the `/login` POST path — only the GET page gains an additive `?registered=1` branch.)

### Live e2e (proof at deploy — handed to plan/test)

Full-stack browser flow against the real running stack (real PostgreSQL, no request interception):
`GET /register` → submit valid invite + new user + matching pw → land on `/login?registered=1` with
the success flash → log in → reach `/cards` (UC-1 + UC-6 + AC-8). A second run proves repeatability
(UC-1/Goal 2). A wrong-token run proves the shared `401` banner + no row (UC-2). This is the
live-stack authenticity requirement; it MUST NOT use `page.route`/`intercept`/mocks.

### Test type coverage (minimum baseline)

`unit` (handlers + page + ratelimit + flash) + `integration`/`e2e` (live registration→login→/cards) +
regression (`/login` unchanged). No `stress`/`load` scope is required — registration defines no latency
SLA (Gate G026 N/A); rate-limit behavior is covered by the unit ratelimit test.

---

## BDD Scenario → Test Mapping

| Scenario | Primary test(s) | Location (plan to finalize) |
|----------|-----------------|------------------------------|
| UC-1 valid → account + landing | unit "valid token + new user"; live e2e happy path | `internal/api/web_register_test.go`; `tests/e2e/...` |
| UC-2 invalid/missing token, no row | unit "wrong token" + "missing token" + non-enumeration | `internal/api/web_register_test.go` |
| UC-3 empty-configured disables | unit "empty-configured" + "WebCredentials nil" | `internal/api/web_register_test.go` |
| UC-4 duplicate, no overwrite | unit "duplicate username" | `internal/api/web_register_test.go` |
| UC-5 mismatch / too-short / missing | unit "password mismatch"/"too short"/"missing field" | `internal/api/web_register_test.go` |
| UC-6 new user logs in → /cards | live e2e happy path (login leg) | `tests/e2e/...` |
| UC-7 regression /login unchanged | existing spec-070 tests + `/login` no-query flash test | `internal/api/web_login_*_test.go` |
| UC-8 register POST rate-limited | unit router ratelimit membership | `internal/api/web_register_ratelimit_test.go` |

---

## Authorization Matrix

| Endpoint | Anonymous (no invite) | Invite-token holder (operator) | Authenticated session (cookie) |
|----------|-----------------------|--------------------------------|--------------------------------|
| `GET /register` | ✅ renders identical form | ✅ renders identical form | ✅ (no special handling) |
| `POST /v1/web/register` | ❌ shared `401` (no valid token) | ✅ creates account (full-admin band) | n/a — gate is the invite token, not a session |
| `GET /login?registered=1` | ✅ renders + success flash | ✅ | ✅ |

There is no per-user role: possession of the invite token is the **entire** authorization model for
creating an account (spec Actors). Both new routes are OUTSIDE `bearerAuthMiddleware` (entry points by
definition).

---

## Alternatives & Tradeoffs

| Decision | Chosen | Alternative | Why rejected |
|----------|--------|-------------|--------------|
| Secret identity | dedicated `WEB_REGISTRATION_INVITE_TOKEN` | reuse `AUTH_BOOTSTRAP_TOKEN` | bootstrap is one-shot + production-required; registration must be repeatable + optional (Decision #1) |
| Landing | 303 → `/login?registered=1` (no cookie) | auto-login (set `auth_token`, 303 → `/cards`) | ux-settled #2: one canonical session path; register stays pure intake; credential round-trip proof; no duplicated cookie logic |
| Rollout control | secret-presence gate | release-train feature flag | flag adds per-train ceremony with no incremental control; secret is finer-grained (Decision #3) |
| Disabled-state UX | GET always renders form; POST shared banner | GET renders "unavailable" page | a distinct GET state leaks gate config and defeats AC-10 (Reconciled AC-5) |
| Token-gate position | gate FIRST, before field/username eval | validate fields then gate | gating last leaks username-existence to tokenless probers (AC-10 / UC-2) |
| Empty-configured handling | explicit `configured==""` guard before constant-time compare | single `ConstantTimeCompare` | empty-vs-empty compare returns match → open signup; the guard prevents it |
| Asset embedding | extend existing `loginUIFS` embed + reuse `login.css` | new `registerUIFS` embed + `register.css` | reuse the spec-057 foundation; one stylesheet; no second file-server route |
| Placeholder-leak guard | map `IsPlaceholder` → empty at wiring | rely solely on knb substitution | one-line boundary guard closes a concrete open-signup failure mode (defense-in-depth) |

---

## Complexity Tracking

| Deviation from simplest approach | Simpler alternative considered | Why the deviation is justified |
|----------------------------------|-------------------------------|--------------------------------|
| New dedicated managed secret (+8 mirror/loader/wiring edit sites) | Reuse `AUTH_BOOTSTRAP_TOKEN` (zero new secret) | Reuse breaks bootstrap’s one-shot/clear-after-use + production-required semantics and makes registration single-use or boot-required — semantically wrong and operationally fragile (Decision #1). The new secret is the *simplest correct* option. |
| Un-substituted-placeholder guard (`IsPlaceholder` → empty) at wiring | Rely only on knb always substituting | A missed substitution would surface a publicly-known constant as the invite token = open admin signup (the spec "Failure Condition"). One line at the trust boundary guards a real, security-critical failure mode; not speculative. Marked non-blocking so plan may right-size it. |
| Handler-level field pre-checks (steps 4-6) that overlap `UpsertPassword`’s internal validation | Let `UpsertPassword` validate and surface its generic error | The store returns generic errors; the ux catalog requires **distinct** field-level messages (mismatch / too-short / invalid-username / duplicate). The pre-checks are the source of message granularity, not redundant validation for its own sake. |

> Everything else uses the simplest viable approach: reuse the existing credential store, the existing
> embed + serving contract, the existing rate-limit group, the existing `sanitizeNext` + header trio,
> and an additive-only `/login` change. No new package, store, migration, route group, or abstraction.

---

## Open Questions

**None blocking.** The two analyst-deferred decisions (#1 secret, #3 flag) are settled above; ux
settled #2 (landing); AC-5 is reconciled. The single non-blocking item — whether the
un-substituted-placeholder guard is implemented in this spec’s wiring scope or accepted as documented
defense-in-depth — is left to bubbles.plan to right-size when cutting scopes; both options satisfy the
spec.

---

## Next Owner

**bubbles.plan** — author `scopes.md` from this design + spec.md (the design intentionally does NOT
create scopes.md). Suggested scope shape (plan owns the final cut): (1) SST secret 3-mirror + loader +
Dependencies wiring + placeholder guard; (2) `GET /register` page + embed + `register.js`; (3)
`POST /v1/web/register` handler + control flow; (4) `/login` success-flash (additive); (5) live e2e
registration→login→/cards. The knb deploy wiring is a separate `<knb-repo>` commit (named in §knb
Deploy Wiring), not a scope in this repo.
