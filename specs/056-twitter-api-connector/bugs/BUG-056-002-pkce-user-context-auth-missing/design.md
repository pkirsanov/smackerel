# Bug Fix Design: [BUG-056-002] User-Context OAuth 2.0 PKCE auth missing for user-owned endpoints

> **STATUS: Path A LOCKED (2026-06-08).** The maintainer resolved Open Question Q1 — build the real User-Context OAuth 2.0 PKCE flow. Q2 (token storage) and Q3 (authorize UX) are now resolved below. This document is implementation-ready; Path B (de-scope) survives only as a rejected alternative. No code is written and nothing is marked Fixed in this packet — `bubbles.plan` re-scopes and `bubbles.implement` builds from here.

## Design Brief

**Current State.** The shipped Twitter connector authenticates EVERY v2 endpoint with one static App-Only bearer — `internal/connector/twitter/api.go:62` (`bearerToken` field), built in `newAPIClient` (`api.go:96`), attached uniformly by `buildRequest` (`api.go:141` `req.Header.Set("Authorization", "Bearer "+c.bearerToken)`). There is zero PKCE / OAuth2 user-context machinery anywhere in the connector (verified: `grep -rniE 'pkce|code_verifier|code_challenge|oauth2|refresh_token' internal/connector/twitter/` → exit 1). App-Only is correct ONLY for the public endpoints `/2/users/:id/tweets` and `/2/users/:id/mentions`.

**Target State.** Build a real User-Context OAuth 2.0 Authorization-Code-with-PKCE (S256) flow that yields a user-context access token + a rotating refresh token, persisted AES-256-GCM-encrypted at rest. Route `/2/users/me`, `/2/users/:id/bookmarks`, `/2/users/:id/liked_tweets` through the user-context token; keep App-Only for the two public endpoints. Refresh on pre-expiry / 401 (retry once); fail loud — never silently fall back to App-Only on a user-owned endpoint. Plus the independent G2 gauge (`x-rate-limit-remaining`).

**Patterns to Follow** (repo precedent this design mirrors):
- OAuth begin/finalize + short-TTL state row + delete-on-consume → `internal/drive/google/google.go` `BeginConnect`/`FinalizeConnect`; state table `drive_oauth_states` (migration `023_drive_connection_expires_at.sql`, 15-min TTL).
- AES-256-GCM at-rest crypto (key = `SHA-256(SMACKEREL_AUTH_TOKEN)`, nonce-prepended base64) → `internal/auth/store.go` `TokenStore.encrypt`/`decrypt`.
- `auth.Token` value type + bounded-body token-endpoint JSON parse → `internal/auth/oauth.go` (`Token`, `tokenRequest`).
- Fail-loud connector-credential sentinel error → `internal/connector/twitter/api.go` `ErrAPIBearerTokenRequired`.
- SST config chain → `config/smackerel.yaml` → `scripts/commands/config.sh` → `internal/config/config.go` → `cmd/core/connectors.go` (mirror the existing `bearer_token` path).
- Operator CLI surface → `main.go` `os.Args[1]` dispatch (`auth`/`users`/`assistant`) + `cmd/core/cmd_auth.go`; `smackerel.sh` `auth)` passthrough (lines 662-680).
- Connector runtime DI (inject DB pool) → `internal/drive/google/google.go` `ConfigureRuntime(pool, client, cfg)`.

**Patterns to Avoid** (present in the codebase but WRONG to copy here):
- `auth.TokenStore` plaintext-on-empty-key fallback (`internal/auth/store.go` `NewTokenStore`: "If empty, tokens are stored in plaintext (development only)"). The user-context refresh token is a long-lived credential and MUST be encrypted — the Twitter store **fails loud** when the at-rest key is empty.
- Drive's plaintext `credentials_ref = "bearer:" + tokenResp.AccessToken` (`internal/drive/google/google.go` `FinalizeConnect`). Do NOT persist any token in plaintext.
- Reusing the generic single-`provider`-keyed `oauth_tokens` table / `auth.TokenStore` (originally `archive/007_oauth_tokens.sql`, consolidated into `001_initial_schema.sql`) for the connector's tokens — wrong key shape, plaintext fallback, and no PKCE-state companion. Twitter owns its own tables (mirrors Drive).
- The hidden default in `scripts/commands/config.sh:1156` (`if [[ -z "$TWITTER_ARCHIVE_DIR" ]]; then TWITTER_ARCHIVE_DIR="./data/twitter-archive"; fi`). OAuth credential keys get NO such fallback — empty string in env, fail-loud in Go (smackerel-no-defaults).
- The defect itself: the uniform `Authorization: Bearer <App-Only>` in `buildRequest` (`api.go:141`).

**Resolved Decisions:**
- **Q1 = Path A** (build PKCE). Path B (de-scope bookmarks/likes/users-me) REJECTED.
- **OAuth helper:** extend `auth.GenericOAuth2` *additively* (new struct-level PKCE methods + a token-endpoint Basic-auth style flag); the shared `auth.OAuth2Provider` interface is NOT changed (zero ripple to `TokenStore.GetValid` / Drive / the per-user-bearer subsystem).
- **Q2 token storage:** new migration `056_twitter_oauth_pkce.sql` → `twitter_oauth_states` (carries the per-flow `code_verifier`, 15-min TTL) + `twitter_oauth_tokens` (AES-256-GCM access+refresh, composite PK). Twitter-owned store reusing the `auth.TokenStore` crypto technique; **fail-loud on empty at-rest key**.
- **Q3 authorize UX:** CLI `connector twitter authorize-begin | authorize-finalize --state --code | authorize-status` (manual code paste). HTTP callback REJECTED for the headless single-user case.
- **PKCE:** S256; authorize `https://twitter.com/i/oauth2/authorize`; token `https://api.twitter.com/2/oauth2/token`; scopes `offline.access tweet.read users.read bookmark.read like.read`; confidential-client HTTP Basic auth at the token endpoint; **rotating** refresh token re-persisted on every exchange.
- **Routing:** per-endpoint auth tier — user-context for `users_me`/`bookmarks`/`liked_tweets`, App-Only for `tweets`/`mentions`.
- **G2:** `ConnectorTwitterAPIRateLimitRemaining` gauge parsed on every response.

**Open Questions:** None blocking. (Implementation latitude only: exact registered `redirect_uri` string is operator config; the loopback `http://127.0.0.1/callback` form is recommended for the CLI-paste flow.)

## Decision Record (Q1–Q3 RESOLVED)

| Q | Decision | Owner | Date |
|---|----------|-------|------|
| **Q1 — product** | **Path A — build the real User-Context OAuth 2.0 PKCE flow.** Path B (de-scope) REJECTED. | maintainer | 2026-06-08 |
| **Q2 — token storage** | Migration `056_twitter_oauth_pkce.sql`: `twitter_oauth_states` (+`code_verifier`, 15-min TTL) + `twitter_oauth_tokens` (AES-256-GCM access+refresh, composite PK). Twitter-owned store reusing the `auth.TokenStore` crypto; fail-loud on empty at-rest key. | bubbles.design | 2026-06-08 |
| **Q3 — authorize UX** | CLI begin/finalize (`connector twitter authorize-begin`/`authorize-finalize`/`authorize-status`), manual code paste. HTTP callback rejected. | bubbles.design | 2026-06-08 |

## Root Cause Analysis

### Investigation Summary
The reconcile-to-doc gaps phase probed spec 056's shipped connector against its resolved requirements. Verified evidence (captured in report.md → Diagnostic Evidence, HEAD `9638b065`):
- `grep -rniE 'pkce|code_verifier|code_challenge|oauth2|refresh_token|/oauth2/token' internal/connector/twitter/` → exit 1 (no user-context/PKCE flow anywhere in the connector, including tests).
- The implementation applies a single static App-Only bearer to every request: `api.go:62` `bearerToken string`; `api.go:117` `buildRequest`; `api.go:141` `req.Header.Set("Authorization", "Bearer "+c.bearerToken)`. `fetchUsersMe` and `fetchEndpointPaginated` (bookmarks/liked_tweets/tweets/mentions) all route through `buildRequest`.
- Requirement: `spec.md:225` (NC-1) — "Use **User-Context OAuth 2.0 with PKCE** for `/2/users/me/bookmarks` and `/2/users/:id/liked_tweets`. App-Only bearer tokens are insufficient for these user-owned endpoints." `design.md:131-133` endpoint matrix marks `/2/users/me`, `bookmarks`, `liked_tweets` as User-Context PKCE; `tweets`/`mentions` as App-Only.
- False delivered-claim: `report.md:7` (re-quoted `:342`) — "App-Only bearer + User-Context PKCE … covering 4 endpoints".
- GAP-G2: `grep -rniE 'x-rate-limit-remaining|RateLimitRemaining' internal/connector/twitter/ internal/metrics/` → exit 1. Only `ConnectorTwitterAPIRateLimitReset` exists (`metrics.go:105`), written only inside the 429 branch (`api.go:530-534`, `observeRateLimitReset` at `:636-637`). `spec.md:111` (R-016) requires an `x-rate-limit-remaining` gauge "after each API call".

### Root Cause
The spec/design correctly resolved NC-1 to mandate User-Context OAuth 2.0 PKCE for user-owned endpoints, but the implementation phase (scopes 01-03) shipped only the App-Only bearer path — `apiClient` was built around a single `bearerToken` field and a uniform `Authorization: Bearer` header. Certification did not catch the divergence: App-Only fixture tests pass against `httptest.Server` because the fake server never enforces user-context, and the live PKCE arms are env-gated SKIPs (`api_live_test.go`), so the missing flow is never exercised. The connector builds and tests green while being unable to authenticate to its headline user-owned endpoints. The false report.md claim then masked the gap from later review.

### Impact Analysis
- Affected components: `internal/connector/twitter` user-owned ingestion path (`fetchUsersMe`, bookmarks + liked_tweets pagination).
- Affected data: bookmarks and likes are not retrievable at all against the real API (403); no partial/corrupt writes (fail at the request boundary).
- Affected users: any operator who enables the Twitter connector intending to ingest bookmarks or likes. Operators syncing only public `tweets`/`mentions` are unaffected (App-Only is correct there).
- Blast radius (G1): the auth strategy for three of four endpoints + a new credential-acquisition + refresh subsystem. Blast radius (G2): one header parse + one gauge registration/update.
- Artifact-integrity: spec 056 over-claims delivered capability (report.md:7,342).

## Fix Architecture (Path A — LOCKED)

The fix has four moving parts plus the independent G2 gauge: (A.1) a PKCE-capable
OAuth2 helper, (A.2) the locked PKCE wire protocol, (A.3) encrypted token storage,
(A.4) the authorize CLI, (A.5) per-endpoint auth-tier routing, (A.6) refresh-on-401,
(A.7) fail-loud-when-absent, (A.8) the config SST chain, and (A.9) connector runtime
wiring. Repo anchors are cited inline.

### A.1 OAuth helper — extend `auth.GenericOAuth2` additively (no interface change)

`internal/auth/oauth.go` already owns the shared OAuth2 client: `OAuth2Config`
(`ClientID/ClientSecret/RedirectURL/AuthEndpoint/TokenEndpoint/HTTPTimeoutSeconds`),
`GenericOAuth2`, and `tokenRequest` (bounded-body JSON decode → `auth.Token`). It does
NOT implement PKCE and posts the `client_secret` in the form body. Twitter needs (a) a
per-flow `code_verifier`/`code_challenge` and (b) confidential-client HTTP **Basic** auth
at the token endpoint.

**Decision: extend `GenericOAuth2` with ADDITIVE struct-level methods + one config flag.
Do NOT change the `auth.OAuth2Provider` interface.** Concretely:
- Add `OAuth2Config.TokenEndpointAuthStyle string` (`"body"` = current behavior, the
  default for existing callers; `"basic"` = `Authorization: Basic base64(id:secret)` and
  the secret omitted from the body). `tokenRequest` branches on it.
- Add `func (g *GenericOAuth2) AuthURLWithPKCE(scopes []string, state, codeChallenge string) string`
  — same as `AuthURL` plus `code_challenge` + `code_challenge_method=S256`.
- Add `func (g *GenericOAuth2) ExchangeCodeWithVerifier(ctx, code, codeVerifier string) (*Token, error)`
  and `func (g *GenericOAuth2) RefreshTokenBasic(ctx, refreshToken string) (*Token, error)`
  (or have `RefreshToken` honor `TokenEndpointAuthStyle`) — both add `code_verifier` /
  honor Basic auth via `tokenRequest`.
- A small PKCE util (`func GeneratePKCEPair() (verifier, challenge string, err error)`) in
  `internal/auth/` produces the S256 pair (see A.2).

**Why additive struct methods, not an interface change and not a fully separate wrapper:**
the `auth.OAuth2Provider` interface (`AuthURL`/`ExchangeCode`/`RefreshToken`/`ProviderName`)
is consumed by `auth.TokenStore.GetValid` (`internal/auth/store.go`) and the Google
oauthHandler path (`cmd/core/services.go:264`). Adding methods to the concrete struct keeps
those consumers byte-for-byte unchanged (zero ripple) while still reusing one OAuth2 client.
`TokenEndpointAuthStyle` is an RFC 6749 §2.3.1-standard generalization (Basic auth is the
RFC-recommended client-auth style), not a Twitter hack. The Twitter-specific orchestration
(state/verifier persistence + endpoint routing + token store) lives in the `twitter` package,
mirroring how `internal/drive/google/google.go` owns its own OAuth exchange rather than the
shared helper.

### A.2 PKCE mechanics (S256) — LOCKED endpoints, scopes, request/response shapes

**Verifier / challenge (RFC 7636):**
- `code_verifier` = `base64url-nopad(32 random bytes)` via `crypto/rand` → 43 chars from
  the unreserved set `[A-Za-z0-9-._~]`.
- `code_challenge` = `base64url-nopad( SHA-256( ASCII(code_verifier) ) )`; method `S256`.

**Begin — authorize URL** (printed by the CLI; operator opens in a browser):
```
GET https://twitter.com/i/oauth2/authorize
    ?response_type=code
    &client_id=<TWITTER_OAUTH_CLIENT_ID>
    &redirect_uri=<TWITTER_OAUTH_REDIRECT_URL>
    &scope=offline.access%20tweet.read%20users.read%20bookmark.read%20like.read
    &state=<random-state-token>
    &code_challenge=<S256-challenge>
    &code_challenge_method=S256
```
Scopes are MANDATORY and exact: `offline.access` (REQUIRED to receive a refresh token),
`users.read` (`/2/users/me`), `bookmark.read` (`/2/users/:id/bookmarks`),
`like.read` (`/2/users/:id/liked_tweets`), `tweet.read` (tweet payloads on every endpoint).

**Finalize — code → token exchange:**
```
POST https://api.twitter.com/2/oauth2/token
Authorization: Basic base64(<client_id>:<client_secret>)
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code&code=<code>&redirect_uri=<TWITTER_OAUTH_REDIRECT_URL>
&code_verifier=<verifier>&client_id=<client_id>
```
Response `200`:
```json
{ "token_type": "bearer", "expires_in": 7200,
  "access_token": "<at>", "scope": "offline.access tweet.read users.read bookmark.read like.read",
  "refresh_token": "<rt>" }
```

**Refresh — refresh_token exchange:**
```
POST https://api.twitter.com/2/oauth2/token
Authorization: Basic base64(<client_id>:<client_secret>)
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token&refresh_token=<refresh_token>&client_id=<client_id>
```
Response `200` is the same shape and **contains a NEW `refresh_token`** — Twitter rotates
refresh tokens, so the rotated pair MUST be re-encrypted and persisted on every refresh, or
the next refresh fails. `expires_in` (typ. 7200s) → `expires_at = now + expires_in`.

### A.3 Token storage (Q2) — migration 056 + encrypted Twitter-owned store

Next sequential migration after the highest live file
`internal/db/migrations/055_annotation_actor_and_version.sql` is **`056_twitter_oauth_pkce.sql`**
(the integer is independent of the spec number; 056 is simply the next free slot — no
`056_*` file exists). Two tables, mirroring the Drive precedent
(`drive_oauth_states` from migration 023) but adding the PKCE verifier and encryption:

```sql
-- 056_twitter_oauth_pkce.sql  (spec 056 / BUG-056-002)
-- twitter_oauth_states: short-lived PKCE flow binding. Holds the per-flow
-- code_verifier so authorize-finalize can present it at the token exchange.
-- 15-min TTL, deleted on consume (mirrors drive_oauth_states).
CREATE TABLE IF NOT EXISTS twitter_oauth_states (
    state_token    TEXT PRIMARY KEY,
    owner_user_id  TEXT NOT NULL,
    connector_id   TEXT NOT NULL,                       -- 'twitter'
    code_verifier  TEXT NOT NULL,                       -- PKCE verifier; single-use; server-side only
    scope          JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at     TIMESTAMPTZ NOT NULL                 -- now() + 15 min
);
CREATE INDEX IF NOT EXISTS idx_twitter_oauth_states_expires_at ON twitter_oauth_states (expires_at);

-- twitter_oauth_tokens: persistent user-context credentials, encrypted at rest.
CREATE TABLE IF NOT EXISTS twitter_oauth_tokens (
    owner_user_id  TEXT NOT NULL,
    connector_id   TEXT NOT NULL,                       -- 'twitter'
    access_token   TEXT NOT NULL,                       -- AES-256-GCM ciphertext, base64
    refresh_token  TEXT NOT NULL,                       -- AES-256-GCM ciphertext, base64
    token_type     TEXT NOT NULL DEFAULT 'bearer',
    scopes         JSONB NOT NULL DEFAULT '[]'::jsonb,
    expires_at     TIMESTAMPTZ NOT NULL,                -- access-token expiry
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (owner_user_id, connector_id)
);
-- ROLLBACK: DROP TABLE IF EXISTS twitter_oauth_tokens; DROP TABLE IF EXISTS twitter_oauth_states;
```
The composite `(owner_user_id, connector_id)` PK matches the single-operator deployment today
while leaving room for multi-account later (mirrors `drive_connections`'s uniqueness shape)
without DDL churn.

**`code_verifier` storage rationale:** the verifier is an ephemeral, single-use,
15-min-TTL value deleted on consume, and is useless without the concurrently-issued
authorization code. It is stored plaintext in the TTL'd state row (matching
`drive_oauth_states`'s plaintext state binding). The long-lived `access_token`/`refresh_token`
are the credentials and ARE encrypted.

**Encryption — reuse the `auth.TokenStore` technique, but fail loud (no plaintext fallback):**
a Twitter-owned store (`internal/connector/twitter/oauth_store.go`) reuses the exact AES-256-GCM
pattern from `internal/auth/store.go` (key = `SHA-256(SMACKEREL_AUTH_TOKEN)` via `cfg.AuthToken`,
nonce-prepended, base64). The ONE deliberate divergence from `auth.TokenStore`: when the at-rest
key is empty the constructor returns an error instead of `slog.Warn`-ing and storing plaintext —
the refresh token is a long-lived credential and smackerel-no-defaults forbids the silent
plaintext path. Go shape (reuses the `auth.Token` value type):
```go
type oauthStore struct { pool *pgxpool.Pool; gcm cipher.AEAD }
func newOAuthStore(pool *pgxpool.Pool, atRestKey string) (*oauthStore, error) // err if atRestKey == ""
func (s *oauthStore) SaveTokens(ctx, owner string, t *auth.Token) error        // encrypt → upsert
func (s *oauthStore) GetTokens(ctx, owner string) (*auth.Token, error)         // select → decrypt
func (s *oauthStore) HasValidUserContext(ctx, owner string) (bool, error)      // exists row
func (s *oauthStore) SaveState(ctx, st pkceState) error                        // begin
func (s *oauthStore) ConsumeState(ctx, stateToken string) (pkceState, error)   // lookup+TTL-check+DELETE
```
We do NOT reuse the generic `oauth_tokens` table / `auth.TokenStore`: it is keyed by a single
`provider` string (no owner/connector dimension), has the plaintext fallback, and has no PKCE
state companion. Twitter owns its tables, mirroring Drive.

### A.4 Authorize UX (Q3) — CLI begin/finalize (manual code paste)

**Decision: a CLI begin/finalize pair, NOT an HTTP callback.** Drive uses an HTTP redirect
callback (`GET /v1/connectors/drive/oauth/callback`) because it is a multi-account,
browser-from-the-web-UI connect flow. The Twitter connector is a single-operator, headless,
one-time authorize; requiring a publicly-reachable OAuth redirect endpoint on a Tailscale/Caddy
home-lab box is friction the CLI-paste flow avoids. Twitter deprecated true OOB
(`urn:ietf:wg:oauth:2.0:oob`), so we use a registered loopback `redirect_uri`
(recommended `http://127.0.0.1/callback`) and have the operator copy `state` + `code` from the
browser address bar after authorizing — the redirect target need not run a server.

New top-level command group `connector`, dispatched in `cmd/core/main.go` next to the existing
`auth`/`users`/`assistant` `os.Args[1]` checks, implemented in `cmd/core/cmd_connector.go`
(`runConnectorCommand`, mirroring `runAuthCommand` + `loadAuthCLIConfig` for config+pool load):

| Command | Action |
|---------|--------|
| `connector twitter authorize-begin` | Generate verifier+S256 challenge+state; insert a `twitter_oauth_states` row (15-min TTL); print the authorize URL + the `state` token. |
| `connector twitter authorize-finalize --state <s> --code <c>` | `ConsumeState` (verify TTL + delete); `ExchangeCodeWithVerifier`; persist encrypted access+refresh into `twitter_oauth_tokens`. Prints success only — NEVER a token value. |
| `connector twitter authorize-status` | Print whether a valid/refreshable user-context token is persisted (drives the A.7 preflight). |

`smackerel.sh` gains a `connector)` passthrough case mirroring the `auth)` case (lines 662-680):
`smackerel_compose "$TARGET_ENV" exec smackerel-core smackerel-core connector "$@"`.

**Operator steps:** (1) register a Twitter OAuth 2.0 confidential client; redirect URI =
configured `oauth_redirect_url`; scopes per A.2. (2) Set `oauth_client_id`/`oauth_client_secret`/
`oauth_redirect_url` in `config/smackerel.yaml`; `./smackerel.sh config generate`. (3)
`./smackerel.sh connector twitter authorize-begin` → open the printed URL, authorize. (4) Copy
`state`+`code` from the redirected address bar. (5) `./smackerel.sh connector twitter authorize-finalize
--state <state> --code <code>`. (6) Set `sync_mode: api` (or `hybrid`); the connector now uses the
user-context token for bookmarks/likes/users-me.

### A.5 Endpoint routing — per-endpoint auth tier

Introduce an auth tier in `internal/connector/twitter/api.go` and have `buildRequest` select the
token by tier instead of always attaching `c.bearerToken`:
```go
type authTier int
const ( authTierAppOnly authTier = iota; authTierUserContext )
func endpointAuthTier(e apiEndpoint) authTier {
    switch e {
    case endpointBookmarks, endpointLikes: return authTierUserContext
    default:                                return authTierAppOnly   // tweets, mentions
    }
}
```
`/2/users/me` (fetched by `fetchUsersMe`, not in the `apiEndpoint` enum) is also user-context.
`buildRequest(ctx, method, path, query, tier)` attaches `c.bearerToken` (App-Only) for
`authTierAppOnly`, or the resolved user-context access token for `authTierUserContext`. The
public-endpoint path (`tweets`/`mentions`) is unchanged → no regression.

### A.6 Refresh-on-401 / pre-expiry refresh + rotating-token persistence

The apiClient gains a user-context token source (injected — see A.9) that wraps `oauthStore` +
the PKCE refresh exchange:
- **Pre-flight:** before any `authTierUserContext` request, if `token.IsExpired()` (or within a
  60s skew), call `RefreshTokenBasic`, persist the **rotated** pair (`SaveTokens` re-encrypts),
  and use the new access token.
- **On 401:** `doWithRetry` currently fast-fails 401/403 as `errAuthRejected` (no retry,
  `api.go:~470`). Add a user-context wrapper (`doUserContextRequest`) that, on a 401, refreshes
  once, persists the rotated pair, and retries the request exactly once. A second 401 → surface
  `errAuthRejected` (fail loud).
- **403 stays terminal** (insufficient-auth / wrong token type — not refreshable). This is what
  surfaces the bug if App-Only is ever used on a user-owned endpoint (A.8 test).

### A.7 Fail-loud when user-context token absent (no App-Only fallback)

Add `ErrUserContextTokenRequired` mirroring `ErrAPIBearerTokenRequired` (`api.go:38`). When
`sync_mode=api|hybrid` is configured to fetch bookmarks/likes/users-me but `oauthStore` has no
persisted user-context token (operator never ran authorize), the connector fails loud with a
message naming the remedy: *"run `./smackerel.sh connector twitter authorize-begin` to authorize
user-context access"*. It NEVER silently attaches the App-Only bearer to a user-owned endpoint —
that is precisely this bug and a smackerel-no-defaults violation.

### A.8 Config SST additions (client_id / client_secret / redirect_url)

Mirror the existing `bearer_token` SST path end-to-end (NO hidden defaults):
- `config/smackerel.yaml` `connectors.twitter` (after `bearer_token`, line ~521):
  ```yaml
      oauth_client_id: ""      # REQUIRED for user-context (bookmarks/likes/users-me): Twitter OAuth 2.0 client id
      oauth_client_secret: ""  # REQUIRED for user-context: confidential-client secret (never logged)
      oauth_redirect_url: ""   # REQUIRED for user-context: registered redirect URI (e.g. http://127.0.0.1/callback)
  ```
- `scripts/commands/config.sh` (after `TWITTER_BEARER_TOKEN`, line ~1157), mirroring the
  fail-empty form (`... || VAR=""`), and emit the vars in the generated env block (~1908):
  `TWITTER_OAUTH_CLIENT_ID`, `TWITTER_OAUTH_CLIENT_SECRET`, `TWITTER_OAUTH_REDIRECT_URL`.
- `internal/config/config.go`: add `TwitterOAuthClientID/Secret/RedirectURL string` fields
  (lines ~177) and `os.Getenv("TWITTER_OAUTH_...")` reads (lines ~581). **FORBIDDEN:**
  `os.Getenv("TWITTER_OAUTH_CLIENT_ID", "default")`-style fallbacks. Empty → the connector /
  authorize CLI fails loud where the value is required.
- `cmd/core/connectors.go` (Twitter wiring, lines 295-309): thread the three values into the
  connector config/runtime so both `Connect()` and the authorize CLI can read them.

### A.9 Connector runtime wiring (DB pool + token store injection)

The Twitter connector currently receives only `connector.ConnectorConfig` (Credentials +
SourceConfig maps) and has no DB pool, so it cannot build `oauthStore`. Add a
`ConfigureRuntime(pool *pgxpool.Pool, atRestKey string, oauthCfg TwitterOAuthConfig)` method on
the Twitter `Connector`, mirroring `internal/drive/google/google.go` `ConfigureRuntime`, called
from `cmd/core/connectors.go` during wiring. `Connect()` then builds `oauthStore` and injects the
user-context token source into the apiClient. The authorize CLI builds the same `oauthStore`
directly from `config.Load()` + a pgx pool (mirroring `loadAuthCLIConfig`).

## GAP-056-G2 — `x-rate-limit-remaining` gauge (R-016, independent of A)

1. Add `ConnectorTwitterAPIRateLimitRemaining = prometheus.NewGaugeVec(GaugeOpts{Name:
   "smackerel_connector_twitter_api_rate_limit_remaining", Help: "..."}, []string{"connector","endpoint"})`
   in `internal/metrics/metrics.go` (next to `ConnectorTwitterAPIRateLimitReset`, line ~105) and
   register it in the collector list (~593).
2. In `doWithRetry`, after `c.observeRequest(endpoint, statusLabel)` (which runs for EVERY
   response), parse `resp.Header.Get("x-rate-limit-remaining")` and, when present+numeric, set the
   gauge via a new `observeRateLimitRemaining(endpoint, header)` helper (mirrors
   `observeRateLimitReset`, `api.go:~655`). This satisfies "after each API call" — the reset gauge
   is set only on 429; the remaining gauge is set on 2xx and every other response carrying the header.

## Testing Strategy

### Honest testing boundary (anti-fabrication — read before writing any test)

The COMPLETE PKCE machinery (verifier/challenge, code→token exchange, refresh+rotation,
encrypted persistence, per-endpoint routing, fail-loud) IS delivered and is fixture/unit-tested
via `httptest.Server` instances that emulate (a) Twitter's `/2/oauth2/token` endpoint and (b)
user-context enforcement on user-owned paths. **The full live `403→200` against the REAL
Twitter/X API CANNOT be verified in CI or this environment** — it requires a real Twitter app,
real client credentials, and a real interactive browser authorize against a real account. That
arm stays env-gated, exactly like the existing SKIP arms in
`internal/connector/twitter/api_live_test.go` (gate on, e.g., a `TWITTER_LIVE=1` +
`TWITTER_OAUTH_CLIENT_ID` env). No test may claim a live Twitter pass it cannot make; the
fixture server is authoritative for CI. Do NOT design any test that asserts a real Twitter call.

### Why the adversarial App-Only-rejected test is the key regression

A happy-path user-context test alone would still pass if the connector silently fell back to
App-Only against a permissive fake server — the exact blind spot that hid this bug. The
fixture server therefore ENFORCES user-context: for a user-owned path presented with the
App-Only sentinel bearer it returns `403 {"title":"Unsupported Authentication", ...}`. The
test asserts the connector SURFACES that 403 and never silently succeeds. It is RED precisely
when the bug is reintroduced (routing a user-owned endpoint through App-Only), and it is
environment-independent (no real API).

### Test matrix

| # | Scenario | Type | Test (file `internal/connector/twitter/`) |
|---|----------|------|-------------------------------------------|
| 1 | verifier→challenge S256 derivation + verifier charset/length | unit | `api_test.go::TestTwitterAPI_PKCEChallengeS256` |
| 2 | code→token exchange: Basic auth header + `code_verifier` in body; persists encrypted (ciphertext ≠ plaintext via store roundtrip) | integration (httptest) | `oauth_store_test.go::TestTwitterOAuth_CodeExchangePersistsEncrypted` |
| 3 | encrypted store: empty at-rest key → constructor error (fail loud, no plaintext) | unit | `oauth_store_test.go::TestTwitterOAuth_EmptyKeyFailsLoud` |
| 4 | bookmarks request carries the user-context token, not App-Only | integration (httptest) | `api_test.go::TestTwitterAPI_BookmarksUsesUserContextToken` |
| 5 | **adversarial:** App-Only on a user-owned endpoint → 403 surfaced, never silently used | regression (httptest) | `api_test.go::TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected` |
| 6 | refresh-on-401: 401 → refresh exchange → retry → ROTATED refresh token persisted | integration (httptest) | `api_test.go::TestTwitterAPI_RefreshOn401PersistsRotatedToken` |
| 7 | fail-loud when absent: api/hybrid + bookmarks + no persisted token → `ErrUserContextTokenRequired` | unit | `api_test.go::TestTwitterAPI_UserContextTokenRequiredFailsLoud` |
| 8 | public endpoints keep App-Only (no regression) | integration (httptest) | `api_test.go::TestTwitterAPI_PublicEndpointsAppOnly` |
| 9 | secrets never logged: user-context access/refresh tokens absent from logs | regression | extend `api_test.go::TestTwitterAPI_BearerTokenNeverAppearsInLogs` |
| 10 | G2 gauge set after each call | integration (httptest) | `api_test.go::TestTwitterAPI_RateLimitRemainingGauge` |
| 11 | G2 adversarial: gauge updates on a non-429 200 (not only on 429) | regression (httptest) | `api_test.go::TestTwitterAPI_RateLimitRemainingUpdatesOnNon429` |
| 12 | live 403→200 (REAL API) | e2e-live (SKIP unless gated) | `api_live_test.go` (env-gated; never runs in CI) |

All rows are PLANNED — execution belongs to the delivery pass (`bubbles.implement`); no results
are claimed in this packet (anti-fabrication, Gate G021).

## Proposed Scope Breakdown (sequential, each independently testable)

`bubbles.plan` owns the final `scopes.md`; this is the recommended decomposition (expands the
current Scope 1 into A/B/C; G2 becomes its own scope D):

- **Scope A — Foundation: config SST + migration + PKCE-extended helper + encrypted store.**
  A.1, A.2 util, A.3, A.8. Independently testable via matrix rows 1–3 (PKCE derivation, encrypted
  exchange+persist, empty-key fail-loud) + a config-SST fail-loud unit. No connector behavior
  changes yet.
- **Scope B — Authorize surface + token persistence.** A.4 + A.9 CLI half. Testable: begin
  persists a state row and prints an S256 challenge URL; finalize consumes state, exchanges,
  persists encrypted tokens; status reflects presence.
- **Scope C — Endpoint routing + refresh-on-401 + fail-loud-when-absent.** A.5, A.6, A.7, A.9
  connector half. Testable via matrix rows 4–9 — including the adversarial App-Only-rejected
  regression (row 5).
- **Scope D — G2 gauge + false-claim-correction governance.** GAP-056-G2 + matrix rows 10–11,
  plus the closure governance flag below. Independent of A–C.

## Parent Claim Correction & Governance Flag

Delivering Path A makes spec 056 `report.md:7` (re-quoted `:342`) — "App-Only bearer +
User-Context PKCE … covering 4 endpoints" — genuinely TRUE. At **closure (delivery pass, NOT
this design packet and NOT this agent)** the parent `specs/056-twitter-api-connector/report.md`
MUST be updated to describe the now-shipped PKCE delivery accurately.

> **GOVERNANCE FLAG (recert risk):** parent spec 056 is certified `done`. Editing its `report.md`
> after delivery may trigger a re-certification pass (as occurred with spec 070 this session).
> This is a delivery/closure step owned by the delivery agent + a governance decision for the
> orchestrator — it is recorded here as a DoD item and is **not performed by `bubbles.design`**.
> This design packet does NOT touch any parent spec 056 artifact.

## Alternatives Considered

1. **Path B — de-scope bookmarks/likes/users-me to a future spec + restate spec 056 as
   App-Only-public-only.** REJECTED by the maintainer (Q1): bookmarks/likes are a headline
   capability; the platform constraint is real and worth building for.
2. **Change the `auth.OAuth2Provider` interface to thread PKCE.** REJECTED: ripples through
   `TokenStore.GetValid` + the Google oauthHandler path for one connector's benefit. Additive
   struct methods (A.1) achieve reuse with zero interface ripple.
3. **Reuse the generic `oauth_tokens` table / `auth.TokenStore` (provider="twitter").** REJECTED:
   single-`provider` key shape (no owner/connector dimension), plaintext-on-empty-key fallback we
   must not inherit for a long-lived refresh token, and no PKCE-state companion. Dedicated Twitter
   tables mirror the Drive precedent.
4. **HTTP redirect callback (mirror Drive).** REJECTED for the headless single-user case:
   requires a publicly-reachable redirect endpoint on the home-lab box; CLI-paste is simpler and
   robust (A.4).
5. **Silent fallback to App-Only when the user-context token is missing.** REJECTED: re-creates
   this exact bug and violates smackerel-no-defaults. Fail loud (A.7).
6. **Parse `x-rate-limit-remaining` only on 429 (mirror the reset gauge).** REJECTED: R-016
   requires "after each API call"; per-call headroom is the operational signal operators need.

## Resolved Questions

- **Q1 (product):** RESOLVED → Path A (build PKCE). See Decision Record.
- **Q2 (token storage):** RESOLVED → migration 056 `twitter_oauth_states`(+`code_verifier`) +
  `twitter_oauth_tokens` (AES-256-GCM access+refresh); Twitter-owned store reusing the
  `auth.TokenStore` crypto with a fail-loud-on-empty-key divergence (A.3).
- **Q3 (authorize UX):** RESOLVED → CLI `connector twitter authorize-begin|finalize|status`,
  manual code paste, registered loopback redirect URI (A.4).

## Open Questions

None blocking. The only implementation latitude is the exact registered `redirect_uri` string,
which is operator config (`oauth_redirect_url`); `http://127.0.0.1/callback` is the recommended
loopback form for the CLI-paste flow.

## Downstream Handoff

- `bubbles.plan`: re-scope `scopes.md` from this locked design (Scopes A–D above) and clear the
  `BLK-056-002-pkce-product-decision` blocker in `state.json` (the product decision is now made);
  transition status off `blocked`. (Design records the decision but does not own status
  transitions or `certification.*`.)
- `bubbles.implement`: build Scopes A→B→C→D in order; honor the honest testing boundary (no
  fabricated live-API pass); keep all changes within the connector / metrics / config / migration
  / CLI surfaces named above.
- At closure: perform the parent-report claim correction under the governance flag above.
