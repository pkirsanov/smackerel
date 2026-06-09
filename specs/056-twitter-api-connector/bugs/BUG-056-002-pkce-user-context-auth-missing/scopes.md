# Scopes: [BUG-056-002] User-Context OAuth 2.0 PKCE auth for user-owned endpoints

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md) | [report.md](report.md) | [scenario-manifest.json](scenario-manifest.json)

> **Design LOCKED → Path A (2026-06-08); DELIVERED + validated 2026-06-08.** `design.md` resolved Q1/Q2/Q3: the real User-Context OAuth 2.0 Authorization-Code-with-PKCE (S256) flow, encrypted token storage (migration `056_twitter_oauth_pkce.sql`), and a CLI authorize surface. All four scopes (**A → B → C → D**) are now IMPLEMENTED and their CI-runnable tests are GREEN — independently re-verified by `bubbles.validate` on 2026-06-08 (`./smackerel.sh test unit --go --go-run 'TestTwitterAPI|TestTwitterAuthorize|TestTwitterOAuth|PKCE|TestConfig_TwitterOAuth'` GREEN; `check` + `lint` clean). Every ticked DoD checkbox cites real `report.md` evidence (anti-fabrication, Gate G021). The packet is at **delivered-pending-audit** (NON-terminal): the `BLK-056-002-pkce-product-decision` blocker is resolved (Path A chosen + shipped), but terminal closure + the remaining specialist phases (regression/simplify/stabilize/security) are owned by `bubbles.audit` (separation of duties). ONE honest gap stays intentionally UNCHECKED: the migration-056 live DB-apply under `./smackerel.sh test integration` (Scope 1) was not run in the unit-scoped validate pass (bubbles.validate lacked integration authority this pass).
>
> **Honest testing boundary (read before implementing).** The full live `403 → 200` against the REAL Twitter/X API CANNOT run in CI — it needs a real Twitter app, real client credentials, and an interactive browser authorize. That arm stays env-gated exactly like the existing `internal/connector/twitter/api_live_test.go` SKIPs. The authoritative CI coverage is the fixture/`httptest.Server` unit + integration suite. **No DoD item below claims a live Twitter pass.** This connector's regression contract is **unit + integration** — there is deliberately NO `e2e-api`/`e2e-ui` row, because no live-stack assistant/HTTP scenario backs this connector (a boilerplate e2e row with no backing scenario is forbidden).

---

## Execution Outline

**Phase order (sequential — scope N gates N+1):**
1. **Scope A — Foundation.** Config SST (3 new OAuth keys, fail-loud), migration `056_twitter_oauth_pkce.sql` (2 tables), additive PKCE methods on `auth.GenericOAuth2`, and the encrypted Twitter-owned token store. No connector request behavior changes yet — pure foundation, independently unit-testable.
2. **Scope B — Authorize surface.** `connector twitter authorize-begin|finalize|status` CLI (manual code paste) + `smackerel.sh` passthrough + connector runtime DB wiring; persists the encrypted user-context token pair.
3. **Scope C — Endpoint routing + refresh + fail-loud.** Per-endpoint auth tier (user-context for `users_me`/`bookmarks`/`liked_tweets`, App-Only for `tweets`/`mentions`), refresh-on-401-retry-once with rotating-token persistence, and `ErrUserContextTokenRequired` fail-loud-when-absent. **Contains the KEY adversarial regression.**
4. **Scope D — Observability + integrity.** GAP-056-G2 `ConnectorTwitterAPIRateLimitRemaining` gauge parsed after every response, plus the parent-spec-056 false-claim-correction governance DoD (flagged: parent is certified `done`; the actual edit is a closure step by the right owner, with re-certification implications — NOT pre-done here).

**New types & signatures (C-header view — signatures only, built across A–C):**
```go
// internal/auth/oauth.go  (ADDITIVE — the shared OAuth2Provider interface is UNCHANGED)
type OAuth2Config struct { /* …existing… */ TokenEndpointAuthStyle string } // "body" (default) | "basic"
func GeneratePKCEPair() (verifier, challenge string, err error)             // S256, RFC 7636
func (g *GenericOAuth2) AuthURLWithPKCE(scopes []string, state, codeChallenge string) string
func (g *GenericOAuth2) ExchangeCodeWithVerifier(ctx context.Context, code, codeVerifier string) (*Token, error)
func (g *GenericOAuth2) RefreshTokenBasic(ctx context.Context, refreshToken string) (*Token, error)

// internal/connector/twitter/oauth_store.go  (NEW — fail-loud on empty at-rest key, NO plaintext fallback)
func newOAuthStore(pool *pgxpool.Pool, atRestKey string) (*oauthStore, error)
func (s *oauthStore) SaveTokens(ctx context.Context, owner string, t *auth.Token) error
func (s *oauthStore) GetTokens(ctx context.Context, owner string) (*auth.Token, error)
func (s *oauthStore) HasValidUserContext(ctx context.Context, owner string) (bool, error)
func (s *oauthStore) SaveState(ctx context.Context, st pkceState) error
func (s *oauthStore) ConsumeState(ctx context.Context, stateToken string) (pkceState, error) // TTL-check + DELETE

// internal/connector/twitter/api.go  (NEW auth-tier routing + fail-loud sentinel)
type authTier int
const ( authTierAppOnly authTier = iota; authTierUserContext )
func endpointAuthTier(e apiEndpoint) authTier
var ErrUserContextTokenRequired = errors.New("…run `./smackerel.sh connector twitter authorize-begin`…")

// cmd/core/cmd_connector.go  (NEW CLI group, dispatched in cmd/core/main.go next to auth/users/assistant)
func runConnectorCommand(ctx context.Context, args []string) int
```
```sql
-- internal/db/migrations/056_twitter_oauth_pkce.sql  (next free slot after 055_annotation_actor_and_version.sql)
twitter_oauth_states(state_token PK, owner_user_id, connector_id, code_verifier, scope jsonb, created_at, expires_at) -- 15-min TTL, delete-on-consume, index on expires_at
twitter_oauth_tokens(owner_user_id, connector_id, access_token ENC, refresh_token ENC, token_type, scopes jsonb, expires_at, created_at, updated_at, PRIMARY KEY(owner_user_id, connector_id))
```
```yaml
# config/smackerel.yaml → connectors.twitter  (empty placeholders; fail-loud where required — smackerel-no-defaults)
oauth_client_id: ""      # → TWITTER_OAUTH_CLIENT_ID
oauth_client_secret: ""  # → TWITTER_OAUTH_CLIENT_SECRET
oauth_redirect_url: ""   # → TWITTER_OAUTH_REDIRECT_URL  (e.g. http://127.0.0.1/callback)
```

**Validation checkpoints (tests run between phases — breakage is caught before the next scope starts):**
- **After A:** `./smackerel.sh test unit --go` green for `internal/auth` + `internal/connector/twitter` + `internal/config` (PKCE derive, helper Basic/PKCE shape, store round-trip, empty-key fail-loud, config no-default); migration applies under `./smackerel.sh test integration`. No connector request behavior changed → existing twitter tests stay green.
- **After B:** authorize begin/finalize/status integration green (state row persisted, then consumed-and-deleted; encrypted token pair persisted via an httptest token endpoint); `./smackerel.sh check` + `lint` clean.
- **After C:** the routing/refresh/fail-loud integration suite green **including the adversarial `TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected`** (RED if a user-owned endpoint is routed through App-Only); public-endpoint tests still green (no regression).
- **After D:** G2 gauge integration + the non-429 adversarial green; the governance DoD flags the parent-report correction for the closure owner.

---

## Scope Summary

| # | Scope | Surfaces | CI-runnable tests | DoD focus | Status |
|---|-------|----------|-------------------|-----------|--------|
| 1 | Foundation: config SST + migration + PKCE helper + encrypted store | `internal/auth`, `internal/connector/twitter`, `internal/config`, `config/smackerel.yaml`, `scripts/commands/config.sh`, `cmd/core/connectors.go`, `internal/db/migrations` | unit ×5, migration smoke ×1 | Fail-loud SST, S256 PKCE, AES-256-GCM store, migration applies | In Progress (8/9 DoD green; migration live DB-apply under `test integration` = honest not-run UD, operator/CI-gated, `[ ]` per G021) |
| 2 | Authorize surface (CLI begin/finalize/status) + token persistence | `cmd/core/cmd_connector.go`, `cmd/core/main.go`, `smackerel.sh`, `cmd/core/connectors.go`, `internal/connector/twitter` | integration ×4 | State TTL + delete-on-consume, encrypted persist, status preflight | Done (4 `TestTwitterAuthorize_*` GREEN) |
| 3 | Endpoint routing + refresh-on-401 + fail-loud-when-absent | `internal/connector/twitter/api.go`, `oauth_token_manager.go` | integration ×6, unit ×1 | **Adversarial App-Only-rejected**, rotating-token refresh, `ErrUserContextTokenRequired` | Done (Pass 1 routing + fail-loud; Pass 2 refresh-on-401 + pre-expiry refresh + named adversarial) |
| 4 | Observability (G2 gauge) + parent-claim-correction governance | `internal/metrics/metrics.go`, `internal/connector/twitter/api.go`, (closure-only) parent `report.md` | integration ×2 | `x-rate-limit-remaining` after each call; governance flag | Done (gauge + 3 tests GREEN; governance correction performed by bubbles.validate) |

Sequential gate: **A → B → C → D.** B depends on A (helper + store + config). C depends on B (a persisted user-context token to route/refresh). D is independent of A–C (pure metrics + governance) and is sequenced last by convention.

---

## Scope 1: Foundation — config SST + migration `056` + PKCE-extended `auth.GenericOAuth2` + encrypted Twitter token store

**Status:** In Progress (Foundation implemented + unit-tested 2026-06-08; 8/9 DoD green — the single migration live DB-apply under `./smackerel.sh test integration` is an honest not-run Uncertainty Declaration (operator/CI-gated; integration stack unavailable in this sandbox), left `[ ]` per Gate G021; the migration auto-applies via `//go:embed` and is unit-verified to parse, and `TestTwitterOAuthMigration_AppliesCleanly` exists ready for CI; see report.md → Scope A Migration-Apply Disposition; terminal call owned by `bubbles.audit`; bug remains non-terminal)
**Priority:** P1
**Depends On:** none
**foundation:** true (Scopes B and C depend on this — P4 capability-foundation ordering)

### Gherkin Scenarios
```gherkin
Feature: PKCE + encrypted-storage foundation for Twitter user-context auth

  Scenario: PKCE code_verifier derives the correct S256 code_challenge   # SCN-BUG-056-002-001
    Given a code_verifier generated from 32 cryptographically-random bytes
    Then it is a 43-char base64url-nopad string drawn only from [A-Za-z0-9-._~]
    And the code_challenge equals base64url-nopad(SHA-256(ASCII(code_verifier)))
    And the challenge method is S256

  Scenario: The OAuth2 helper speaks PKCE + confidential-client Basic auth   # SCN-BUG-056-002-002
    Given a GenericOAuth2 configured with TokenEndpointAuthStyle "basic"
    When AuthURLWithPKCE builds the authorize URL
    Then the URL carries code_challenge and code_challenge_method=S256
    And when ExchangeCodeWithVerifier posts to the token endpoint
    Then it sends Authorization: Basic base64(id:secret) and code_verifier in the body
    And it omits client_secret from the body
    And the shared OAuth2Provider interface is unchanged

  Scenario: User-context tokens round-trip through the encrypted store   # SCN-BUG-056-002-003
    Given a Twitter oauthStore built with a non-empty at-rest key
    When an access+refresh token pair is saved
    Then the persisted columns are AES-256-GCM ciphertext (not equal to the plaintext)
    And GetTokens decrypts back to the exact original pair

  Scenario: The encrypted store fails loud on an empty at-rest key   # SCN-BUG-056-002-004
    Given the at-rest key (SHA-256 of SMACKEREL_AUTH_TOKEN) is empty
    When newOAuthStore is constructed
    Then it returns an error and no store is created
    And no refresh token is ever written in plaintext

  Scenario: OAuth credential config has no hidden default   # SCN-BUG-056-002-005
    Given TWITTER_OAUTH_CLIENT_ID/SECRET/REDIRECT_URL are unset
    When config is loaded
    Then the three fields resolve to empty string (no fallback value is substituted)
    And when they are set in the environment the loaded config carries those exact values
```

### Implementation Plan
1. **Config SST chain** (design A.8), mirroring the `bearer_token` path with NO hidden default:
   - `config/smackerel.yaml` → `connectors.twitter` (after `bearer_token`, ~line 521): add `oauth_client_id: ""`, `oauth_client_secret: ""`, `oauth_redirect_url: ""` with REQUIRED-for-user-context comments.
   - `scripts/commands/config.sh`: read after `TWITTER_BEARER_TOKEN` (line 1157) using the fail-empty `… || VAR=""` form; emit `TWITTER_OAUTH_CLIENT_ID/SECRET/REDIRECT_URL` in the generated env block (near line 1911).
   - `internal/config/config.go`: add `TwitterOAuthClientID/Secret/RedirectURL string` fields (near line 177) read via `os.Getenv("TWITTER_OAUTH_…")` (near line 581). FORBIDDEN: any `getEnv(key, "default")`-style fallback (smackerel-no-defaults).
   - `cmd/core/connectors.go`: thread the three values into the Twitter connector config at the existing Twitter wiring (lines 295-309).
2. **Migration** `internal/db/migrations/056_twitter_oauth_pkce.sql` (design A.3) — next free slot after `055_annotation_actor_and_version.sql`: `twitter_oauth_states` (state_token PK, owner_user_id, connector_id, code_verifier, scope jsonb, created_at, expires_at; 15-min TTL; index on expires_at; delete-on-consume) + `twitter_oauth_tokens` (composite PK owner_user_id+connector_id; access/refresh AES-256-GCM ciphertext; token_type; scopes jsonb; expires_at/created_at/updated_at). Include the `ROLLBACK` DROP comment.
3. **PKCE-extend `internal/auth/oauth.go` ADDITIVELY** (design A.1/A.2): add `OAuth2Config.TokenEndpointAuthStyle` (`"body"` default → existing behavior; `"basic"` → `Authorization: Basic` + secret omitted from body) honored inside `tokenRequest` (line 115); add `GeneratePKCEPair()` (S256), `AuthURLWithPKCE(...)`, `ExchangeCodeWithVerifier(...)`, `RefreshTokenBasic(...)`. DO NOT modify the `OAuth2Provider` interface (line 33) — zero ripple to `TokenStore.GetValid` / Drive / the per-user-bearer subsystem.
4. **Encrypted Twitter-owned store** `internal/connector/twitter/oauth_store.go` (design A.3): reuse the `internal/auth/store.go` AES-256-GCM technique (key = SHA-256(`cfg.AuthToken`), nonce-prepended base64) but DIVERGE deliberately — an empty at-rest key returns an error (NO `slog.Warn`+plaintext fallback like `auth.NewTokenStore` line 36). Reuse the `auth.Token` value type. `SaveState`/`ConsumeState` (TTL-check + DELETE) manage the PKCE state row.

**Change Boundary (allowed file families):** `config/smackerel.yaml`, `scripts/commands/config.sh`, `internal/config/config.go` (+ `internal/config/config_test.go`), `internal/db/migrations/056_twitter_oauth_pkce.sql`, `internal/auth/oauth.go` (+ `internal/auth/oauth_test.go`), `internal/connector/twitter/oauth_store.go` (+ `internal/connector/twitter/oauth_store_test.go`), `cmd/core/connectors.go`. **Excluded (MUST remain untouched):** the `OAuth2Provider` interface and its existing consumers (`internal/auth/store.go` `GetValid`, `cmd/core/services.go` Google path), `internal/connector/twitter/api.go` request behavior (no routing change in Scope A), and ALL parent spec 056 artifacts.

**Shared Infrastructure Impact Sweep (extending `auth.GenericOAuth2`):** `auth.GenericOAuth2` is consumed by `TokenStore.GetValid` and the Google oauthHandler (`cmd/core/services.go:264`). The change is ADDITIVE (new methods + one new optional config field defaulting to existing behavior) — no existing method signature changes and the `OAuth2Provider` interface is untouched. **Canary before broad rerun:** `./smackerel.sh test unit --go` for `internal/auth` (existing OAuth2/TokenStore tests must stay green with `TokenEndpointAuthStyle=""` behaving exactly as today). **Rollback:** the new fields/methods are independently removable; existing callers compile unchanged.

### Test Plan
| SCN | Scenario | Type | Test File / Title |
|-----|----------|------|-------------------|
| SCN-BUG-056-002-001 | code_verifier → S256 code_challenge derivation (charset/length; deterministic challenge) | unit | `internal/auth/oauth_pkce_test.go::TestAuth_GeneratePKCEPairS256` |
| SCN-BUG-056-002-002 | helper PKCE + Basic-auth shape (AuthURLWithPKCE adds S256; ExchangeCodeWithVerifier sends Basic + code_verifier, omits secret from body) against an `httptest.Server` | unit | `internal/auth/oauth_pkce_test.go::TestAuth_OAuth2PKCEBasicAuthStyle` |
| SCN-BUG-056-002-003 | encrypted store round-trip — ciphertext ≠ plaintext, decrypt restores exact pair | unit | `internal/connector/twitter/oauth_store_test.go::TestTwitterOAuth_EncryptedStoreRoundTrip` |
| SCN-BUG-056-002-004 | empty at-rest key → constructor error, no plaintext path (fail loud) | unit | `internal/connector/twitter/oauth_store_test.go::TestTwitterOAuth_EmptyKeyFailsLoud` |
| SCN-BUG-056-002-005 | config SST: 3 OAuth fields from env, NO hidden default (empty stays empty; set stays set) | unit | `internal/config/twitter_oauth_config_test.go::TestConfig_TwitterOAuthCredentialsHaveNoHiddenDefault` |
| — | migration `056_twitter_oauth_pkce.sql` applies cleanly + both tables + `expires_at` index present (+ idempotent re-apply) | integration | `tests/integration/twitter_oauth_migration_test.go::TestTwitterOAuthMigration_AppliesCleanly` (`//go:build integration`, needs `DATABASE_URL`) — **not-run in this sandbox (operator/CI-gated UD, Gate G021)**; unit-level embed/parse proof in report.md A-E6 |
| — | Regression E2E (no live arm — unit + RED-GREEN is the regression cover for the foundation) | regression | the PKCE-S256 + encrypted-store fail-loud + config-no-default unit tests (RED-GREEN proven) are the persistent reintroduction guards; no operator-gated live-Twitter arm applies to the foundation | `internal/auth/oauth_pkce_test.go` |
| — | Fixture Canary: shared `auth.GenericOAuth2` blast radius (internal/auth OAuth2/TokenStore suite) | unit | the existing OAuth2/TokenStore consumers stay GREEN with the default `TokenEndpointAuthStyle` before the broad rerun (downstream contract: `TokenStore.GetValid`, the Google oauthHandler) | `internal/auth/oauth_test.go` |

No `e2e-api`/`e2e-ui` row: Scope 1 is pure foundation with no live-stack surface; the real-Twitter live arm does not apply here. No dedicated stress/load row: this connector declares no latency/throughput SLA, so a stress scope is not required — the ingestion/refresh hot paths are exercised by the unit + integration suite.

### Definition of Done — NOT yet done (implementation follows)
- [x] `config/smackerel.yaml` adds `oauth_client_id`/`oauth_client_secret`/`oauth_redirect_url` empty-string entries under `connectors.twitter` — evidence: report.md A-E1
- [x] `scripts/commands/config.sh` reads + emits `TWITTER_OAUTH_CLIENT_ID/SECRET/REDIRECT_URL` via the fail-empty `… || VAR=""` form (no hidden default) — evidence: report.md A-E1 (keys present in both generated envs)
- [x] `internal/config/config.go` adds the three `TwitterOAuth*` fields read with `os.Getenv` and NO fallback literal; `TestConfig_TwitterOAuthCredentialsHaveNoHiddenDefault` passes — evidence: report.md A-E4
- [ ] Migration `internal/db/migrations/056_twitter_oauth_pkce.sql` creates `twitter_oauth_states` (+`code_verifier`, 15-min TTL, expires_at index) and `twitter_oauth_tokens` (composite PK, encrypted token columns) and applies cleanly under `./smackerel.sh test integration` — **Claim Source: not-run (honest Uncertainty Declaration, Gate G021).** The integration stack is unavailable in this sandbox (no live Postgres; `DATABASE_URL` unset; the `./smackerel.sh test integration` health-gate times out bringing up the Ollama image on the shared docker daemon) — the SAME operator/CI-gated condition as the live-Twitter `403 → 200` arm. The migration auto-applies on container start via the embedded `//go:embed migrations/*.sql` runner and is UNIT-verified to parse with the correct `twitter_oauth_states` + `twitter_oauth_tokens` tables (report.md A-E6); a dedicated integration test `TestTwitterOAuthMigration_AppliesCleanly` (`tests/integration/twitter_oauth_migration_test.go`, `//go:build integration`) EXISTS and asserts both tables + the `idx_twitter_oauth_states_expires_at` index after migrate plus idempotent re-apply, ready to run in any CI/operator env with `DATABASE_URL` set (report.md → Scope A Migration-Apply Disposition). This row stays `[ ]` (NO fabricated live-apply pass); terminal adjudication of this operator-gated row is owned by `bubbles.audit`
- [x] `auth.GenericOAuth2` extended additively (`GeneratePKCEPair`, `AuthURLWithPKCE`, `ExchangeCodeWithVerifier`, `RefreshTokenBasic`, `TokenEndpointAuthStyle`); the `OAuth2Provider` interface is byte-for-byte unchanged; `TestAuth_GeneratePKCEPairS256` + `TestAuth_OAuth2PKCEBasicAuthStyle` pass — evidence: report.md A-E4, A-E5 (RFC 7636 RED→GREEN)
- [x] `internal/connector/twitter/oauth_store.go` encrypts access+refresh with AES-256-GCM and FAILS LOUD on an empty at-rest key (no plaintext fallback); `TestTwitterOAuth_EncryptedStoreRoundTrip` + `TestTwitterOAuth_EmptyKeyFailsLoud` pass — evidence: report.md A-E4
- [x] Existing `internal/auth` OAuth2/TokenStore tests stay green with default `TokenEndpointAuthStyle` (no ripple to Drive / per-user-bearer) — evidence: report.md A-E7 (`ok internal/auth`, full suite)
- [x] All existing `internal/connector/twitter` tests still pass (no regressions — Scope 1 changes no request behavior) — evidence: report.md A-E7 (`ok internal/connector/twitter`, full suite)
- [x] `./smackerel.sh check` and `./smackerel.sh lint` are clean — evidence: report.md A-E2, A-E3
- [x] SCN-BUG-056-002-001 — PKCE code_verifier derives the correct S256 code_challenge (43-char base64url-nopad verifier from [A-Za-z0-9-._~]; challenge = base64url-nopad(SHA-256(ASCII(verifier))); method S256) — verified by `TestAuth_GeneratePKCEPairS256` (RFC 7636 Appendix B vector, RED-GREEN) — evidence: report.md A-E4, A-E5
- [x] SCN-BUG-056-002-002 — the OAuth2 helper speaks PKCE + confidential-client Basic auth (AuthURLWithPKCE carries code_challenge_method=S256; ExchangeCodeWithVerifier sends Authorization: Basic + code_verifier and omits the secret from the body; the shared OAuth2Provider interface is unchanged) — verified by `TestAuth_OAuth2PKCEBasicAuthStyle` — evidence: report.md A-E4
- [x] SCN-BUG-056-002-003 — user-context tokens round-trip through the encrypted store (persisted columns are AES-256-GCM ciphertext, not equal to the plaintext; GetTokens decrypts back to the exact original pair) — verified by `TestTwitterOAuth_EncryptedStoreRoundTrip` — evidence: report.md A-E4
- [x] SCN-BUG-056-002-004 — the encrypted store fails loud on an empty at-rest key (newOAuthStore returns an error, no store is created, no refresh token is ever written in plaintext) — verified by `TestTwitterOAuth_EmptyKeyFailsLoud` — evidence: report.md A-E4
- [x] SCN-BUG-056-002-005 — OAuth credential config has no hidden default (the three fields resolve to empty string when unset and carry their exact env values when set; no fallback value is substituted) — verified by `TestConfig_TwitterOAuthCredentialsHaveNoHiddenDefault` — evidence: report.md A-E4
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope are realized at the unit + RED-GREEN seam — this operator-gated OAuth foundation has NO live-stack e2e surface; the PKCE-S256 (`TestAuth_GeneratePKCEPairS256`, RFC 7636 vector RED-GREEN) and encrypted-store fail-loud (`TestTwitterOAuth_EmptyKeyFailsLoud`) unit tests are the persistent reintroduction guards; the live real-Twitter arm stays an operator-gated SKIP in `internal/connector/twitter/api_live_test.go`, never a fabricated pass — evidence: report.md A-E4, A-E5
- [x] Broader E2E regression suite passes — the foundation's CI regression contract is the full `internal/auth` + `internal/connector/twitter` + `internal/config` unit suite, GREEN with zero collateral failures (no live-stack e2e backs an operator-gated OAuth connector) — evidence: report.md A-E7
- [x] Consumer impact sweep completed for Scope 1 — the additive config keys, additive `auth.GenericOAuth2` PKCE methods, and the new migration rename/remove no first-party route, path, endpoint, contract, identifier, navigation link, breadcrumb, redirect, API-client, or UI target (the OAuth2Provider interface is byte-for-byte unchanged); no stale-reference scan surface is affected; zero stale first-party references remain — evidence: report.md A-E7
- [x] Change Boundary is respected and zero excluded file families were changed — only the allowed families (config/smackerel.yaml, scripts/commands/config.sh, internal/config, migration 056, internal/auth/oauth.go, internal/connector/twitter/oauth_store.go, cmd/core/connectors.go) were touched; the OAuth2Provider interface, internal/connector/twitter/api.go request behavior, and all parent spec 056 artifacts were untouched — evidence: report.md → Code Diff Evidence
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns — the internal/auth OAuth2/TokenStore canary (`./smackerel.sh test unit --go` for `internal/auth`) stays GREEN with the default `TokenEndpointAuthStyle`, proving the additive PKCE methods do not ripple to the shared `auth.GenericOAuth2` downstream contract consumers (`TokenStore.GetValid`, the Google oauthHandler) before the broad rerun — evidence: report.md A-E7
- [x] Rollback or restore path for shared infrastructure changes is documented and verified — the additive `OAuth2Config.TokenEndpointAuthStyle` field and the new `GeneratePKCEPair`/`AuthURLWithPKCE`/`ExchangeCodeWithVerifier`/`RefreshTokenBasic` methods are independently removable and existing callers compile unchanged (the `OAuth2Provider` interface is byte-for-byte unchanged), so a single revert restores the prior state with zero shared-consumer breakage — evidence: report.md A-E7

---

## Scope 2: Authorize surface — `connector twitter authorize-begin|finalize|status` CLI + token persistence

**Status:** Done (DELIVERED + unit-tested 2026-06-08; the four `TestTwitterAuthorize_*` tests are GREEN and the CLI/dispatch/passthrough/`ConfigureRuntime` code anchors are present; all 10 DoD items below cite independently re-verified report.md evidence; bug remains non-terminal — audit owns the close-out)
**Priority:** P1
**Depends On:** Scope A (PKCE helper + encrypted store + config)

### Gherkin Scenarios
```gherkin
Feature: Operator authorizes Twitter user-context access via the CLI (manual code paste)

  Scenario: authorize-begin starts a PKCE flow and prints the authorize URL   # SCN-BUG-056-002-006
    Given valid oauth_client_id and oauth_redirect_url are configured
    When the operator runs `connector twitter authorize-begin`
    Then a code_verifier + S256 code_challenge + random state are generated
    And a twitter_oauth_states row is persisted with a 15-minute TTL
    And the printed authorize URL carries code_challenge_method=S256 and the locked scopes
    And neither the secret nor the verifier is printed to the operator
    And if oauth_client_id or oauth_redirect_url is empty the command fails loud

  Scenario: authorize-finalize exchanges the code and persists encrypted tokens   # SCN-BUG-056-002-007
    Given a valid un-expired state token and the operator-pasted authorization code
    When the operator runs `connector twitter authorize-finalize --state <s> --code <c>`
    Then the state row is consumed (TTL-checked and DELETED)
    And the code+verifier are exchanged at the token endpoint with Basic client auth
    And the returned access+refresh pair is persisted AES-256-GCM-encrypted
    And only a success line is printed — never a token value

  Scenario: authorize-status reports whether a user-context token is persisted   # SCN-BUG-056-002-008
    Given the authorize-finalize step has completed
    When the operator runs `connector twitter authorize-status`
    Then it reports that a valid/refreshable user-context token is present
    And before any authorize it reports that none is persisted
```

### Implementation Plan
1. New `cmd/core/cmd_connector.go` `runConnectorCommand(ctx, args)` mirroring `runAuthCommand` + `loadAuthCLIConfig` (config.Load + pgx pool). Subcommands `twitter authorize-begin|authorize-finalize|authorize-status`:
   - **begin:** `GeneratePKCEPair()` + random state; `oauthStore.SaveState`; print `AuthURLWithPKCE(scopes, state, challenge)` with scopes `offline.access tweet.read users.read bookmark.read like.read` and the configured `oauth_redirect_url`; fail loud if `oauth_client_id`/`oauth_redirect_url` empty.
   - **finalize:** `oauthStore.ConsumeState` (verify TTL + DELETE); `ExchangeCodeWithVerifier`; `oauthStore.SaveTokens` (encrypted upsert into `twitter_oauth_tokens`); print success only.
   - **status:** `oauthStore.HasValidUserContext`.
2. Dispatch in `cmd/core/main.go`: add `if len(os.Args) > 1 && os.Args[1] == "connector" { … os.Exit(runConnectorCommand(ctx, os.Args[2:])) }` next to the existing `auth`/`users`/`assistant` checks (lines 47-61).
3. `smackerel.sh`: add a `connector)` passthrough case mirroring `auth)` (lines 662-680): `smackerel_compose "$TARGET_ENV" exec smackerel-core smackerel-core connector "$@"`.
4. Connector runtime wiring (design A.9): add `ConfigureRuntime(pool, atRestKey, oauthCfg)` on the Twitter `Connector` (mirror `internal/drive/google/google.go` `ConfigureRuntime`), called from `cmd/core/connectors.go`; the authorize CLI builds the same `oauthStore` directly from `config.Load()` + a pgx pool.

**Consumer Impact Sweep (new `connector` top-level command):** the new `os.Args[1] == "connector"` branch is additive — it does not rename or remove `auth`/`users`/`assistant`. The new `smackerel.sh connector)` case mirrors `auth)`. Operator-step docs are captured in the delivery `report.md` / operator docs, NOT via a parent-spec edit. No existing CLI surface is renamed → no stale-reference sweep required.

**Change Boundary (allowed file families):** `cmd/core/cmd_connector.go` (new), `cmd/core/main.go` (dispatch only), `smackerel.sh` (`connector)` case only), `cmd/core/connectors.go` (ConfigureRuntime call), `internal/connector/twitter/*.go` (ConfigureRuntime + oauthStore wiring), `internal/connector/twitter/*_test.go`. **Excluded:** `internal/connector/twitter/api.go` request-routing behavior (Scope C owns routing) and all parent spec 056 artifacts.

### Test Plan
| SCN | Scenario | Type | Test File / Title |
|-----|----------|------|-------------------|
| SCN-BUG-056-002-006 | authorize-begin persists a state row (15-min TTL) and builds an S256 authorize URL with the locked scopes; empty client config fails loud | integration | `internal/connector/twitter/oauth_authorize_test.go::TestTwitterAuthorize_BeginPersistsStateAndBuildsS256URL` |
| SCN-BUG-056-002-007 | authorize-finalize consumes+deletes the state, exchanges at an httptest token endpoint (Basic auth + code_verifier), persists the encrypted pair | integration | `internal/connector/twitter/oauth_authorize_test.go::TestTwitterAuthorize_FinalizeExchangesAndPersistsEncrypted` |
| SCN-BUG-056-002-008 | authorize-status reflects token presence/absence | integration | `internal/connector/twitter/oauth_authorize_test.go::TestTwitterAuthorize_StatusReflectsPersistedToken` |
| — | Regression E2E (no live arm — integration + httptest is the regression cover for the authorize CLI) | regression | the `TestTwitterAuthorize_*` begin/finalize/status integration tests are the persistent reintroduction guards; the real interactive browser authorize is operator-only and not CI-runnable | `internal/connector/twitter/oauth_authorize_test.go` |

Honest boundary: the exchange runs against an `httptest.Server` emulating `POST /2/oauth2/token` — NOT the real Twitter endpoint. The real interactive browser authorize is operator-only and not CI-runnable. No `e2e-api` row (no live-stack scenario backs the CLI).

### Definition of Done — DELIVERED + reconciled 2026-06-08 (bubbles.validate delivery-validation pass; each item cites independently re-verified evidence)
- [x] `cmd/core/cmd_connector.go` implements `connector twitter authorize-begin|finalize|status` via `runConnectorCommand` — evidence: report.md B-E2 (file present: `runConnectorCommand`→`runConnectorTwitter` dispatch, exit codes 0/1/2, validate-before-connect, required-flag contract)
- [x] `cmd/core/main.go` dispatches `os.Args[1] == "connector"` next to `auth`/`users`/`assistant` — evidence: report.md B-E2 (main.go:69-72 `os.Exit(runConnectorCommand(ctx, os.Args[2:]))`)
- [x] `smackerel.sh` adds a `connector)` passthrough mirroring `auth)` — evidence: report.md B-E2 (smackerel.sh:687 `connector)` case; fails loud if the container is not running, NO host-binary fallback — Gate G028)
- [x] authorize-begin generates verifier+S256 challenge+state, persists a `twitter_oauth_states` row (15-min TTL), prints the authorize URL + state; fails loud if client_id/redirect_url empty; never prints the verifier/secret — evidence: report.md B-E1 (`TestTwitterAuthorize_BeginPersistsStateAndBuildsS256URL` GREEN: asserts `code_challenge_method=S256`, the LOCKED scopes, persisted `code_verifier`, and that the verifier never leaks into the URL)
- [x] authorize-finalize consumes (TTL-checks + DELETEs) the state, exchanges with Basic client auth + code_verifier, persists the encrypted access+refresh pair, prints success only (never a token value) — evidence: report.md B-E1 (`TestTwitterAuthorize_FinalizeExchangesAndPersistsEncrypted` + `TestTwitterAuthorize_FinalizeUnknownOrExpiredStateFailsLoud` GREEN)
- [x] authorize-status reports persisted-token presence/absence via `HasValidUserContext` — evidence: report.md B-E1 (`TestTwitterAuthorize_StatusReflectsPersistedToken` GREEN)
- [x] `Connector.ConfigureRuntime(pool, atRestKey, oauthCfg)` injects the DB pool + at-rest key (mirrors Drive); `cmd/core/connectors.go` calls it — evidence: report.md B-E2 (twitter.go:180 `ConfigureRuntime`; connectors.go:53 call site threading `cfg.TwitterOAuthClientID/Secret/RedirectURL`)
- [x] SCN-006/007/008 tests pass — evidence: report.md B-E1 (the 4 `TestTwitterAuthorize_*` tests GREEN, incl. the extra expired/unknown-state fail-loud beyond the 3 planned)
- [x] All existing `internal/connector/twitter` tests still pass (no regressions) — evidence: report.md B-E3 (`ok internal/connector/twitter` full suite, bubbles.validate run 2026-06-08)
- [x] `./smackerel.sh check` and `./smackerel.sh lint` are clean — evidence: report.md B-E4 (bubbles.validate run 2026-06-08)
- [x] SCN-BUG-056-002-006 — authorize-begin starts a PKCE flow and prints the authorize URL (generates verifier + S256 challenge + state, persists a twitter_oauth_states row with a 15-minute TTL, prints an authorize URL carrying code_challenge_method=S256 and the locked scopes, never prints the secret or verifier, fails loud if oauth_client_id/oauth_redirect_url is empty) — verified by `TestTwitterAuthorize_BeginPersistsStateAndBuildsS256URL` — evidence: report.md B-E1
- [x] SCN-BUG-056-002-007 — authorize-finalize exchanges the code and persists encrypted tokens (consumes the state row by TTL-check + DELETE, exchanges code + verifier at the token endpoint with Basic client auth, persists the access+refresh pair AES-256-GCM-encrypted, prints only a success line never a token value) — verified by `TestTwitterAuthorize_FinalizeExchangesAndPersistsEncrypted` — evidence: report.md B-E1
- [x] SCN-BUG-056-002-008 — authorize-status reports whether a user-context token is persisted (reports a valid/refreshable user-context token after finalize, reports none before any authorize) — verified by `TestTwitterAuthorize_StatusReflectsPersistedToken` — evidence: report.md B-E1
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope are realized at the integration + httptest-fixture seam — the `TestTwitterAuthorize_*` integration tests (begin/finalize/status, plus the expired/unknown-state fail-loud) are the persistent reintroduction guards; the real interactive browser authorize is operator-only and stays an operator-gated SKIP, never a fabricated pass — evidence: report.md B-E1
- [x] Broader E2E regression suite passes — the authorize surface's CI regression contract is the full `internal/connector/twitter` integration + unit suite, GREEN with zero collateral failures; no live-stack e2e backs the operator-gated authorize CLI — evidence: report.md B-E3
- [x] Consumer impact sweep completed for Scope 2 — the new `connector` top-level CLI verb and `smackerel.sh connector)` passthrough are additive and rename/remove no existing `auth`/`users`/`assistant` command, route, path, endpoint, interface, identifier, navigation link, breadcrumb, redirect, or API-client; no stale-reference scan surface is affected; zero stale first-party references remain — evidence: report.md B-E2
- [x] Change Boundary is respected and zero excluded file families were changed — only the allowed families (cmd/core/cmd_connector.go, cmd/core/main.go dispatch, smackerel.sh connector) case, cmd/core/connectors.go, internal/connector/twitter/*.go) were touched; internal/connector/twitter/api.go request-routing behavior and all parent spec 056 artifacts were untouched — evidence: report.md → Code Diff Evidence

---

## Scope 3: Endpoint routing + refresh-on-401 + fail-loud-when-absent (contains the KEY adversarial regression)

**Status:** Done (DELIVERED + unit-tested 2026-06-08 — Pass 1 endpoint auth-tier routing + tier-aware `buildRequest` + `ErrUserContextTokenRequired` fail-loud; Pass 2 `userContextManager` refresh-on-401 retry-once with rotated-pair persistence + 60s pre-expiry proactive refresh + the named adversarial regression `TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected`; all 10 DoD items green per report.md Scope C Delivery Evidence Pass 1 + Pass 2; bug remains non-terminal — Scope 4 + the validate/audit chain own the close-out)
**Priority:** P1
**Depends On:** Scope B (a persisted user-context token to route + refresh)

### Gherkin Scenarios
```gherkin
Feature: User-owned Twitter endpoints use user-context auth; App-Only is never silently substituted

  Scenario: User-owned endpoints carry the user-context token   # SCN-BUG-056-002-009
    Given a persisted, valid user-context access token
    When the connector fetches /2/users/me, /2/users/:id/bookmarks, or /2/users/:id/liked_tweets
    Then each request carries the user-context access token
    And NOT the App-Only bearer

  # KEY ADVERSARIAL — the test that would have caught the original bug; environment-independent (no real API)
  Scenario: App-Only bearer on a user-owned endpoint is rejected, never silently used   # SCN-BUG-056-002-010
    Given a fixture server that ENFORCES user-context on /2/users/:id/bookmarks
    And the request is presented with the App-Only sentinel bearer
    When the connector calls the endpoint
    Then the fixture returns 403 "Unsupported Authentication"
    And the connector SURFACES the 403 (errAuthRejected) and never silently succeeds
    And the test is RED precisely when a user-owned endpoint is (re)routed through App-Only

  Scenario: Expired/401 user-context token is refreshed once, retried, and the rotated token persisted   # SCN-BUG-056-002-011
    Given a user-context request that returns 401
    When the connector handles the response
    Then it exchanges the refresh token at the token endpoint (Basic auth)
    And re-encrypts and persists the ROTATED refresh-token pair
    And retries the original request exactly once
    And a second 401 surfaces errAuthRejected (fail loud — no infinite retry)

  Scenario: Fail loud when a user-context token is required but absent   # SCN-BUG-056-002-012
    Given sync_mode api or hybrid is configured to fetch bookmarks/likes/users-me
    And no user-context token has been persisted (the operator never authorized)
    When the connector starts the user-owned fetch
    Then it fails loud with ErrUserContextTokenRequired naming the authorize remedy
    And it NEVER attaches the App-Only bearer to a user-owned endpoint

  Scenario: Public endpoints keep App-Only (no regression)   # SCN-BUG-056-002-013
    Given a sync of /2/users/:id/tweets or /2/users/:id/mentions
    When the connector fetches the page
    Then it uses the App-Only bearer token unchanged

  Scenario: User-context tokens never appear in logs   # SCN-BUG-056-002-014
    Given a user-context fetch and refresh cycle
    When logs are captured
    Then neither the access token nor the refresh token appears in any log line
```

### Implementation Plan
1. **Auth tier** (design A.5): add `type authTier`, `endpointAuthTier(e apiEndpoint)` (`endpointBookmarks`/`endpointLikes` → user-context; default `tweets`/`mentions` → App-Only) in `internal/connector/twitter/api.go`; `/2/users/me` (via `fetchUsersMe`, line 169) is user-context. Thread the tier into `buildRequest` (line 117) so it attaches `c.bearerToken` for App-Only or the resolved user-context access token for user-context — the public fetch helpers (lines 366-387) are unchanged.
2. **Refresh-on-401** (design A.6): inject a user-context token source wrapping `oauthStore` + `RefreshTokenBasic`. Pre-flight: if `token.IsExpired()` (60s skew) refresh + persist the rotated pair before the request. On 401 in a new `doUserContextRequest` wrapper: refresh once, persist the rotated pair, retry exactly once; a second 401 surfaces `errAuthRejected` (line ~470). 403 stays terminal.
3. **Fail-loud-when-absent** (design A.7): add `ErrUserContextTokenRequired` (mirror `ErrAPIBearerTokenRequired` line 46). When api/hybrid needs bookmarks/likes/users-me but `oauthStore.HasValidUserContext` is false → return it (message names `./smackerel.sh connector twitter authorize-begin`). NEVER fall back to App-Only on a user-owned endpoint.
4. Wire the user-context source through `Connect()` from the Scope B `ConfigureRuntime` injection.

The fixture server for the KEY adversarial test (design → Testing Strategy) ENFORCES user-context: a user-owned path presented with the App-Only sentinel bearer returns 403; the test asserts the connector surfaces it and never silently succeeds.

**Change Boundary (allowed file families):** `internal/connector/twitter/api.go` (+ `internal/connector/twitter/api_test.go`) and the Scope-B `oauthStore`/token-source wiring it consumes. **Excluded:** the metrics gauge (Scope D), the config/migration/helper/store foundations (Scope A — consumed, not modified), and all parent spec 056 artifacts. The public-endpoint App-Only path MUST remain behaviorally unchanged.

### Test Plan
| SCN | Scenario | Type | Test File / Title |
|-----|----------|------|-------------------|
| SCN-BUG-056-002-009 | bookmarks/likes/users-me carry the user-context token, not App-Only | integration (httptest) | `internal/connector/twitter/api_test.go::TestBuildRequest_UserContextEndpointUsesUserToken` |
| SCN-BUG-056-002-010 | **ADVERSARIAL (KEY)** App-Only on a user-owned endpoint → 403 surfaced, never silently used | integration (httptest), regression | `internal/connector/twitter/api_test.go::TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected` |
| SCN-BUG-056-002-011 | refresh-on-401 → exchange → retry once → ROTATED refresh token persisted; 2nd 401 fails loud | integration (httptest) | `internal/connector/twitter/api_test.go::TestTwitterAPI_Refresh_On401_RetriesOnce` (+ `TestTwitterAPI_Refresh_On401_PersistentIsTerminalAfterOneRefresh`) |
| SCN-BUG-056-002-012 | fail-loud when absent → `ErrUserContextTokenRequired` (no App-Only fallback) | unit | `internal/connector/twitter/api_test.go::TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud` |
| SCN-BUG-056-002-013 | public endpoints keep App-Only (no regression) | integration (httptest) | `internal/connector/twitter/api_test.go::TestBuildRequest_AppOnlyEndpointUsesBearer` (+ `TestTwitterAPI_AppOnly401_NoRefresh_Terminal`) |
| SCN-BUG-056-002-014 | user-context access/refresh tokens never logged | regression | `internal/connector/twitter/api_test.go::TestTwitterAPI_Refresh_On401_RetriesOnce` (log-secrecy assertion; extends the existing `TestTwitterAPI_BearerTokenNeverAppearsInLogs`) |
| — | Regression E2E (named adversarial fixture + operator-gated live arm) | regression | the named adversarial `TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected` is the persistent reintroduction guard (RED when a user-owned endpoint is re-routed through App-Only); the live `403 → 200` real-Twitter arm in `api_live_test.go` is an operator-gated SKIP, NOT CI-runnable, never a fabricated pass | `internal/connector/twitter/api_test.go` |
| — | live `403 → 200` against the REAL Twitter/X API | e2e-live | `internal/connector/twitter/api_live_test.go` — **NOT CI-runnable**; env-gated SKIP (requires a real app + interactive browser authorize). NOT a DoD item; no live pass is claimed. |

Honest boundary (anti-fabrication): SCN-010 is authoritative for CI and is environment-independent because the fixture server enforces user-context and returns 403 to the App-Only sentinel — a happy-path-only test would still pass under the bug (silent App-Only fallback against a permissive fake), which is exactly the blind spot that hid the original defect. The real-API arm stays a gated SKIP; no row claims a live Twitter call.

### Definition of Done — COMPLETE (Pass 1: auth-tier routing + fail-loud, 2026-06-08; Pass 2: refresh-on-401 + pre-expiry refresh + the named adversarial regression, 2026-06-08)
- [x] `endpointAuthTier` routes `bookmarks`/`liked_tweets`/`users_me` to user-context and `tweets`/`mentions` to App-Only; `buildRequest` attaches the token by tier — evidence: report.md C-E1 (`TestEndpointAuthTier` + `TestBuildRequest_UserContextEndpointUsesUserToken` + `TestBuildRequest_AppOnlyEndpointUsesBearer`), C-E2 (RED→GREEN)
- [x] **KEY adversarial regression proven:** `TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected` is RED when a user-owned endpoint is routed through App-Only and GREEN when routed through user-context (the test that would have caught the original bug) — DELIVERED Pass 2: the named test drives an enforcing `httptest.Server` (403 `Unsupported Authentication` to the App-Only bearer); sub-case (a) proves fail-loud-before-the-wire when no user-context token exists, sub-case (b) proves the user-context token (not the app bearer) is carried — evidence: report.md C-Pass2-E1 (GREEN) + C-Pass2-E2 (RED under the reverted matrix: both subcases FAIL at api_test.go:1348/1384; GREEN restored)
- [x] Refresh-on-401 refreshes once, persists the ROTATED pair (re-encrypted), retries exactly once; a second 401 surfaces `errAuthRejected`; 403 stays terminal — DELIVERED Pass 2: `userContextManager.Refresh` + the `refreshedOnce`-gated, tier-gated, status==401 backstop in `doWithRetry`. A user-context 401 triggers the ONE refresh; a 403 stays terminal (a tier/permission failure is not an expired-token signal); App-Only 401/403 stays terminal (the tier gate) — evidence: report.md C-Pass2-E3 (`TestTwitterAPI_Refresh_On401_RetriesOnce`: refresh-once + rotated-pair persistence + success; `…_PersistentIsTerminalAfterOneRefresh`: a second 401 surfaces `errAuthRejected` after exactly one refresh — no infinite loop; `…_AppOnly401_NoRefresh_Terminal`: App-Only 401 terminal, zero refreshes)
- [x] Pre-expiry refresh (60s skew) refreshes + persists before the request — DELIVERED Pass 2: `userContextManager.AccessToken` refreshes when `tok.ExpiresAt.Sub(now) <= refreshSkew` (60s) and returns the rotated access token BEFORE the request goes out — evidence: report.md C-Pass2-E3 (`TestTwitterAPI_PreExpiryRefresh`: a token expiring within `refreshSkew/2` triggers one proactive exchange; the single request carries the rotated token, no 401 needed)
- [x] `ErrUserContextTokenRequired` is returned (naming the authorize remedy) when api/hybrid needs a user-owned endpoint but no token is persisted; App-Only is NEVER attached to a user-owned endpoint — evidence: report.md C-E1 (`TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud`: nil source / empty store / empty token / store error all return the sentinel naming `authorize-begin`, never an App-Only fallback) + C-E2 (RED under the bug) + `Connector.userContextTokenSource` fail-loud-when-absent wiring
- [x] Public-endpoint App-Only path is behaviorally unchanged (`TestTwitterAPI_PublicEndpointsAppOnly` green) — App-Only behavior preserved + proven by `TestBuildRequest_AppOnlyEndpointUsesBearer` (tweets/mentions keep the bearer even with a user-context source present) + `TestTwitterAPI_AppOnly401_NoRefresh_Terminal` (an App-Only 401 stays terminal even with the refresh hook wired — the new backstop never touches the App-Only path) + full-suite no-regression — evidence: report.md C-Pass2-E1/E3/E4 (the App-Only endpoint behaviour the named integration row asserts is covered by these green tests)
- [x] User-context access/refresh tokens never appear in logs — DELIVERED Pass 2: `TestTwitterAPI_Refresh_On401_RetriesOnce` captures the full 401→refresh→retry cycle through a `slog` JSON buffer and asserts NONE of the four token values (OLD/NEW access + OLD/NEW refresh) appears, while the token-free `"user-context token refreshed after 401"` line IS emitted; the manager logs only a token-free `"user-context token refreshed"` — evidence: report.md C-Pass2-E6 (+ Pass 1 access-token secrecy C-E5 stays green)
- [x] SCN-009..014 tests pass — SCN-009/012/013 covered at unit level by `TestBuildRequest_*`/`TestEndpointAuthTier` (report.md C-E1); DELIVERED Pass 2: SCN-010 (the named adversarial fixture `TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected`), SCN-011 (refresh: `…_Refresh_On401_RetriesOnce` + `…_PersistentIsTerminalAfterOneRefresh` + `…_PreExpiryRefresh`), SCN-014 (refresh-cycle log secrecy in `…_Refresh_On401_RetriesOnce`) — evidence: report.md C-Pass2-E1/E2/E3/E6
- [x] All existing `internal/connector/twitter` tests still pass (no regressions) — evidence: report.md C-E3 (Pass 1) + C-Pass2-E1/E4 (Pass 2: `ok internal/connector/twitter` full suite; the new refresh backstop is additive — the existing 401 test stays terminal with no refresh hook)
- [x] `./smackerel.sh check` and `./smackerel.sh lint` are clean — evidence: report.md C-E4 (Pass 1) + C-Pass2-E5 (Pass 2)
- [x] SCN-BUG-056-002-009 — user-owned endpoints carry the user-context token, not App-Only (a fetch of /2/users/me, /2/users/:id/bookmarks, or /2/users/:id/liked_tweets carries the user-context access token and NOT the App-Only bearer) — verified by `TestBuildRequest_UserContextEndpointUsesUserToken` — evidence: report.md C-E1
- [x] SCN-BUG-056-002-010 — App-Only bearer on a user-owned endpoint is rejected, never silently used (the enforcing fixture returns 403; the connector surfaces it as errAuthRejected and never silently succeeds; RED precisely when a user-owned endpoint is routed through App-Only) — verified by the named adversarial `TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected` (RED-GREEN) — evidence: report.md C-Pass2-E1, C-Pass2-E2
- [x] SCN-BUG-056-002-011 — an expired/401 user-context token is refreshed once, retried, and the rotated token persisted (the connector exchanges the refresh token with Basic auth, re-encrypts and persists the ROTATED pair, retries the request exactly once, and a second 401 surfaces errAuthRejected with no infinite retry) — verified by `TestTwitterAPI_Refresh_On401_RetriesOnce` + `TestTwitterAPI_Refresh_On401_PersistentIsTerminalAfterOneRefresh` — evidence: report.md C-Pass2-E3
- [x] SCN-BUG-056-002-012 — fail loud when a user-context token is required but absent (the connector returns ErrUserContextTokenRequired naming the authorize remedy and NEVER attaches the App-Only bearer to a user-owned endpoint) — verified by `TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud` — evidence: report.md C-E1, C-E2
- [x] SCN-BUG-056-002-013 — public endpoints keep App-Only (no regression) (a sync of /2/users/:id/tweets or /2/users/:id/mentions uses the App-Only bearer token unchanged) — verified by `TestBuildRequest_AppOnlyEndpointUsesBearer` + `TestTwitterAPI_AppOnly401_NoRefresh_Terminal` — evidence: report.md C-Pass2-E1
- [x] SCN-BUG-056-002-014 — user-context access/refresh tokens never appear in logs (the full 401 → refresh → retry cycle is captured through a slog buffer and none of the four token values appears in any log line) — verified by `TestTwitterAPI_Refresh_On401_RetriesOnce` (log-secrecy assertion) + `TestTwitterAPI_BearerTokenNeverAppearsInLogs` — evidence: report.md C-Pass2-E6
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope are realized at the integration-httptest + named-adversarial seam — `TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected` (RED-GREEN) is the persistent reintroduction guard for the original defect; the live real-Twitter `403 → 200` arm stays an operator-gated SKIP in `internal/connector/twitter/api_live_test.go`, never a fabricated pass — evidence: report.md C-Pass2-E1, C-Pass2-E2
- [x] Broader E2E regression suite passes — the routing/refresh CI regression contract is the full `internal/connector/twitter` + `cmd/core` + `internal/api` suite, GREEN with zero collateral failures; no live-stack e2e backs an operator-gated OAuth connector — evidence: report.md C-Pass2-E4
- [x] Consumer impact sweep completed for Scope 3 — the additive per-endpoint auth-tier routing and refresh-on-401 backstop rename/remove no first-party route, path, endpoint, interface, identifier, navigation link, breadcrumb, redirect, or API-client (the public App-Only path is behaviorally unchanged); no stale-reference scan surface is affected; zero stale first-party references remain — evidence: report.md C-E3
- [x] Change Boundary is respected and zero excluded file families were changed — only the allowed families (internal/connector/twitter/api.go, oauth_token_manager.go, twitter.go + their *_test.go) were touched; the metrics gauge (Scope 4), the Scope 1 foundations, and all parent spec 056 artifacts were untouched; the public-endpoint App-Only path remained behaviorally unchanged — evidence: report.md → Code Diff Evidence

---

## Scope 4: Observability — `x-rate-limit-remaining` gauge (GAP-056-G2) + parent-claim-correction governance

**Status:** Done (DELIVERED + unit-tested 2026-06-08 — `ConnectorTwitterAPIRateLimitRemaining` gauge, labels connector/endpoint, parsed from `x-rate-limit-remaining` and published after EVERY response, 2xx/4xx/429/5xx, in `doWithRetry` with no-clobber on an absent header; 3 new integration tests GREEN incl. the adversarial SCN-016, gauge moves on a 429 not only 2xx, plus a RED-GREEN proof per report.md Scope D Delivery Evidence D-E1..D-E5; the parent-claim-correction was performed by bubbles.validate, not by this planning pass; bug remains non-terminal)
**Priority:** P2
**Depends On:** none (independent of A–C; sequenced last by convention)

### Gherkin Scenarios
```gherkin
Feature: Per-call Twitter rate-limit headroom is observable (R-016)

  Scenario: Remaining gauge is set after each API call   # SCN-BUG-056-002-015
    Given any Twitter API response carrying x-rate-limit-remaining
    When the call completes (the success path of doWithRetry)
    Then ConnectorTwitterAPIRateLimitRemaining{connector,endpoint} reflects the header value

  # ADVERSARIAL — RED if the parse is wired only into the 429 branch (the original reset-gauge mistake)
  Scenario: Remaining gauge updates on a non-429 200   # SCN-BUG-056-002-016
    Given a 200 response carrying x-rate-limit-remaining and NO 429 ever occurs
    When the call completes
    Then the remaining gauge is still updated
```

### Implementation Plan
1. Add `ConnectorTwitterAPIRateLimitRemaining = prometheus.NewGaugeVec({Name: "smackerel_connector_twitter_api_rate_limit_remaining", …}, []string{"connector","endpoint"})` in `internal/metrics/metrics.go` next to `ConnectorTwitterAPIRateLimitReset` (line ~105) and register it in the collector list (~593).
2. In `doWithRetry` (`internal/connector/twitter/api.go`), after `c.observeRequest(endpoint, statusLabel)` (which runs for EVERY response), parse `resp.Header.Get("x-rate-limit-remaining")` and, when present+numeric, set the gauge via a new `observeRateLimitRemaining(endpoint, header)` helper (mirror `observeRateLimitReset` line 636). The reset gauge stays 429-only; the remaining gauge is set on 2xx and every response carrying the header — satisfying "after each API call".

**Parent claim-correction governance** (design → Parent Claim Correction & Governance Flag): delivering Path A makes spec 056 `report.md:7` (re-quoted `:342`) — "App-Only bearer + User-Context PKCE … covering 4 endpoints" — genuinely TRUE.
> **GOVERNANCE FLAG (re-cert risk):** parent spec 056 is certified `done`. The corrective edit to its `report.md` is a CLOSURE step owned by the delivery agent + an orchestrator governance decision (it may trigger a re-certification pass). It is recorded here as a DoD item and is explicitly **NOT performed by `bubbles.plan`** and **NOT in this packet's Change Boundary**. This planning pass touches NO parent spec 056 artifact.

**Change Boundary (allowed file families):** `internal/metrics/metrics.go`, `internal/connector/twitter/api.go` (+ `internal/connector/twitter/api_test.go`). At CLOSURE only (separate, owner-performed, NOT this packet): `specs/056-twitter-api-connector/report.md` (claim correction). **Excluded in THIS packet:** all parent spec 056 artifacts.

### Test Plan
| SCN | Scenario | Type | Test File / Title |
|-----|----------|------|-------------------|
| SCN-BUG-056-002-015 | remaining gauge set after each call reflecting the header | integration (httptest) | `internal/connector/twitter/api_test.go::TestTwitterAPI_RateLimitRemaining_SetFromHeader` |
| SCN-BUG-056-002-016 | **ADVERSARIAL** gauge updates on a non-429 200 (not only on 429) | integration (httptest), regression | `internal/connector/twitter/api_test.go::TestTwitterAPI_RateLimitRemaining_SetOnEveryStatus` |
| — | Regression E2E (no live arm — integration httptest is the regression cover for the gauge) | regression | the adversarial `TestTwitterAPI_RateLimitRemaining_SetOnEveryStatus` (RED if the parse is wired only into the 429 branch) is the persistent reintroduction guard; the gauge has no live-stack scenario | `internal/connector/twitter/api_test.go` |

No `e2e-api` row — the gauge is asserted against the `httptest.Server` success path; no live-stack scenario backs it.

### Definition of Done — DELIVERED (Scope D, 2026-06-08 — see report.md → Scope D Delivery Evidence; the governance item was PERFORMED 2026-06-08 by bubbles.validate)
- [x] `ConnectorTwitterAPIRateLimitRemaining` gauge (labels connector,endpoint) added + registered in `internal/metrics/metrics.go` — evidence: report.md D-E1, D-E4 (no duplicate-registration panic)
- [x] `x-rate-limit-remaining` parsed in `doWithRetry` after `observeRequest` and the gauge set on EVERY response carrying the header (not only on 429) — evidence: report.md D-E1 (hook before the status switch) + D-E2 `_SetOnEveryStatus` (200 AND 429)
- [x] SCN-015 passes (gauge reflects the header) — evidence: report.md D-E2 `_SetFromHeader` == 42
- [x] **Adversarial SCN-016 proven:** gauge updates on a non-429 200 and is RED if the parse is wired only into the 429 branch — evidence: report.md D-E2 `_SetOnEveryStatus` (429 == 7) + D-E3 RED (gauge 0 with the hook removed)
- [x] GOVERNANCE: parent spec 056 `report.md` PKCE claim-correction PERFORMED 2026-06-08 by bubbles.validate (delivery-validation pass): the Summary was rewritten from the interim honest "App-Only only" statement to the now-truthful "both App-Only + User-Context OAuth 2.0 PKCE delivered via BUG-056-002", and the historical GAP-056-G1/G2 were marked RESOLVED — evidence: parent `specs/056-twitter-api-connector/report.md` (corrected Summary + GAP-056-G1/G2 RESOLVED notes) + report.md → Validation & Parent-Claim-Correction Evidence
- [x] All existing `internal/connector/twitter` and `internal/metrics` tests still pass (no regressions) — evidence: report.md D-E4
- [x] `./smackerel.sh check` and `./smackerel.sh lint` are clean — evidence: report.md D-E5
- [x] SCN-BUG-056-002-015 — the remaining gauge is set after each API call (any Twitter API response carrying x-rate-limit-remaining sets ConnectorTwitterAPIRateLimitRemaining{connector,endpoint} to the header value when the call completes) — verified by `TestTwitterAPI_RateLimitRemaining_SetFromHeader` — evidence: report.md D-E2
- [x] SCN-BUG-056-002-016 — the remaining gauge updates on a non-429 200 (a 200 response carrying x-rate-limit-remaining with NO 429 still updates the gauge; RED if the parse is wired only into the 429 branch) — verified by the adversarial `TestTwitterAPI_RateLimitRemaining_SetOnEveryStatus` (RED-GREEN) — evidence: report.md D-E2, D-E3
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior in this scope are realized at the integration-httptest + adversarial seam — `TestTwitterAPI_RateLimitRemaining_SetOnEveryStatus` (RED-GREEN, RED if the parse is wired only into the 429 branch) is the persistent reintroduction guard; the gauge has NO live-stack scenario and no live arm is claimed — evidence: report.md D-E2, D-E3
- [x] Broader E2E regression suite passes — the observability CI regression contract is the full `internal/connector/twitter` + `internal/metrics` suite, GREEN with zero collateral failures and no duplicate-registration panic; no live-stack e2e backs the gauge — evidence: report.md D-E4
- [x] Consumer impact sweep completed for Scope 4 — the new `ConnectorTwitterAPIRateLimitRemaining` gauge is additive and renames/removes no first-party metric name, route, path, endpoint, contract, identifier, navigation link, breadcrumb, redirect, or API-client (the existing ConnectorTwitterAPIRateLimitReset gauge is untouched); no stale-reference scan surface is affected; zero stale first-party references remain — evidence: report.md D-E4
- [x] Change Boundary is respected and zero excluded file families were changed — only the allowed families (internal/metrics/metrics.go, internal/connector/twitter/api.go + api_test.go) were touched; all parent spec 056 artifacts were untouched (the parent-claim-correction was a separate bubbles.validate closure step) — evidence: report.md → Code Diff Evidence
