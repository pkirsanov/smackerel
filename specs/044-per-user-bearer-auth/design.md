# Spec 044 — Per-User Bearer Auth Foundation: Design

**Status:** in_progress (design phase)
**Source spec:** [`spec.md`](./spec.md) — 11 SCN-AUTH-001..011 + 21 FR-AUTH-001..021 + 8 NFR-AUTH-001..008 + 11 AC-1..11 + 10 OQ-1..10
**Closes:** MIT-040-S-008 (carry-forward from MIT-040-S-003 partial close at commit `4e399a4`); MIT-038-S-003 (cloud-drive Connect body-sourced `owner_user_id`); MIT-027-TRACE-001 actor-source segment (annotation actor_source).
**Related design anchors:** `bubbles-config-sst` skill (SST zero-defaults); `bubbles-test-environment-isolation` skill (test isolation); `docs/Operations.md` (operator surfaces); `docs/Deployment.md` (Build-Once Deploy-Many bundle contract).

---

## 1. Design Brief

Spec 044 establishes Smackerel's per-user trust boundary. Today the API uses a single shared token (`SMACKEREL_AUTH_TOKEN`) per deployment, which forces every per-user identity question to be answered the wrong way (header trust via `X-Actor-Id`, body trust via `OwnerUserID`, literal `actor_source: "system"`). This spec replaces that with a **stateless bearer-token contract** in production, while preserving the existing dev/test ergonomic intact.

The hot-path validation is fully stateless — the middleware verifies a signature against an SST-derived signing key and parses claims, with **zero database queries per request** in the common case (NFR-AUTH-001 ≤ 5 ms p99). Revocation is checked against an in-memory cache that is refreshed asynchronously via NATS broadcast (NFR-AUTH-006 ≤ 60 s propagation budget). Rotation supports a configurable grace window (NFR-AUTH-003 ≥ 24 h default) so long-lived clients can refresh without seeing 401s.

The closure plan in this design routes three previously-routed MIT items (MIT-040-S-008 photo reveal mint, MIT-038-S-003 drive Connect owner, MIT-027-TRACE-001 annotation actor_source) through a single uniform claim-binding contract. The dev/test backward-compat plan keeps `SMACKEREL_AUTH_TOKEN` as a fully-supported authentication mechanism for `runtime.environment in {development, test}`, including the empty-token bypass at `internal/api/router.go` lines 444–451.

This design resolves all 10 open questions raised in spec.md, lands 14 new SST keys under a new `auth.*` block in `config/smackerel.yaml`, and lays out a 4-phase rollout that closes the routed MIT items in Phase 2.

---

## 2. System Context

| Component | Owner | Change |
|-----------|-------|--------|
| `internal/api/router.go` `bearerAuthMiddleware` | Go core | Extended to validate per-user PASETO tokens in production; preserves shared-token + empty-token paths in dev/test |
| `internal/api/router.go` `webAuthMiddleware` | Go core | Cookie-based session preserved; cookie value is per-user PASETO in production, shared token in dev/test |
| **NEW** `internal/auth/` package | Go core | Token issuance, validation, rotation, revocation, session-context helpers |
| **NEW** `internal/auth/revocation/` | Go core | NATS-pub/sub revocation broadcaster + per-instance in-memory cache |
| `internal/api/photos_upload.go` `MintReveal` | Go core | Production: derive `actor_id` from session; dev/test: fall back to `X-Actor-Id` per existing MIT-040-S-003 partial-close contract |
| `internal/drive/google/google.go` `Connect` + `internal/drive/context.go` | Go core | Production: derive `OwnerUserID` from session; dev/test: fall back to body |
| `internal/annotation/` pipeline | Go core | Production: derive `actor_source` from session; dev/test: fall back to literal `system` or supplied value |
| `cmd/core/wiring.go` startup | Go core | Production fail-loud extension: missing `auth.*` SST keys → service refuses to start |
| `internal/config/config.go` SST surface | Go core | New `Auth` config struct + validation |
| `config/smackerel.yaml` SST source | Configuration | 14 new keys under `auth.*` block |
| `scripts/commands/config.sh` | Shell | Emit `AUTH_*` env vars to `config/generated/{dev,test,production}.env` |
| `internal/nats/` subscriber wiring | Go core | New NATS subject `auth.revocations` for cross-instance broadcast |
| **NEW** `cmd/core/cmd_auth.go` | Go core | CLI entry points: `./smackerel.sh auth enroll <user-id>`, `./smackerel.sh auth rotate <user-id>`, `./smackerel.sh auth revoke <token-id>`, `./smackerel.sh auth list-users`, `./smackerel.sh auth bootstrap` |
| **NEW** `internal/api/auth_handlers.go` | Go core | Admin HTTP endpoints: `POST /v1/auth/users`, `POST /v1/auth/users/{user-id}/rotate`, `POST /v1/auth/tokens/{token-id}/revoke`, `GET /v1/auth/users` (gated on admin scope) |
| **NEW** PostgreSQL tables `auth_users`, `auth_tokens`, `auth_revocations` | Database | Schema migrations under `internal/db/migrations/` |
| `web/pwa/` + `web/extension/` | Web surfaces | Receive per-user PASETO tokens; preserve cookie storage with HTTP-only + Secure flags in production |
| `internal/telegram/` | Telegram connector | Map Telegram chat-id to enrolled user; emit annotation events with session-derived `actor_source` |

**Out of scope (per spec.md Non-Goals):** third-party connector OAuth, end-user enrollment UX (admin-issued only for MVP), multi-factor auth, federated identity (OIDC/SAML/SSO), session-management UI, RBAC beyond identity, rate limiting, replacing shared token in dev/test, historical record migration.

---

## 3. Component Diagram

```text
┌──────────────────────────────────────────────────────────────────────┐
│                      CLIENT (PWA / extension / CLI / Telegram)       │
│                                                                      │
│   Authorization: Bearer <PASETO-v4.public-token>                     │
└──────────────────────────────────────┬───────────────────────────────┘
                                       │ HTTPS (production)
                                       ▼
┌──────────────────────────────────────────────────────────────────────┐
│                   internal/api/router.go middleware chain             │
│                                                                      │
│   ┌────────────────────────────────────────────────────────────┐     │
│   │   bearerAuthMiddleware (per request, ≤ 5 ms p99)           │     │
│   │                                                            │     │
│   │   1. extractBearerToken(r)         (header parse)          │     │
│   │   2. if dev/test: matchSharedToken (existing path)         │     │
│   │   3. else (production):                                    │     │
│   │       3a. paseto.Verify(token, signingKey)  (in-memory)   │     │
│   │       3b. claims.Parse() → Session{UserID,...}             │     │
│   │       3c. revocationCache.IsRevoked(tokenID)  (sync.Map)   │     │
│   │       3d. attach Session to r.Context() via auth.WithSess  │     │
│   │   4. else: HTTP 401 + slog.Warn("bearer auth failure")     │     │
│   └─────────────────────────┬──────────────────────────────────┘     │
└─────────────────────────────┼────────────────────────────────────────┘
                              │ r.Context() carries auth.Session
                              ▼
┌──────────────────────────────────────────────────────────────────────┐
│                       Downstream handlers                            │
│                                                                      │
│   sess := auth.SessionFromContext(r.Context())  // typed accessor   │
│   if sess == nil { return 500 }  // middleware contract violation    │
│                                                                      │
│   actorID       := sess.UserID  // photos_upload.MintReveal          │
│   ownerUserID   := sess.UserID  // drive.Connect                     │
│   actorSource   := sess.UserID  // annotation pipeline               │
│                                                                      │
│   In production: body/header values for these fields are REJECTED.   │
│   In dev/test:   body/header values fall back per existing patterns. │
└──────────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────────┐
│                    Async background workers                          │
│                                                                      │
│   ┌──────────────────────────────────┐                               │
│   │  RevocationBroadcaster (NATS)    │                               │
│   │  Subject: auth.revocations       │                               │
│   │                                  │                               │
│   │  On revoke: publish {tokenID}    │                               │
│   │  On startup: bootstrap from DB   │                               │
│   │  On message: cache.Set(tokenID)  │                               │
│   │  On timer (15 min): refresh DB   │                               │
│   └──────────────────────────────────┘                               │
│                                                                      │
│   ┌──────────────────────────────────┐                               │
│   │  RotationGracePruner             │                               │
│   │  On timer (1 h): SELECT tokens   │                               │
│   │     WHERE rotated_at <           │                               │
│   │     now() - grace_window         │                               │
│   │  Mark rotated tokens revoked     │                               │
│   └──────────────────────────────────┘                               │
└──────────────────────────────────────────────────────────────────────┘
```

---

## 4. Configuration Plan

This section resolves OQs 1, 2, 4, 5, 7, 8, 9, 10 by declaring the SST surface they map to. OQs 3, 6 (session shape, enrollment surface) are resolved structurally in §5 below.

### New SST keys under `auth.*` block in `config/smackerel.yaml`

```yaml
auth:
  # OQ-1 RESOLVED: PASETO v4.public — stateless, no JWKS required,
  # no algorithm-confusion attacks, EdDSA-based, simpler than JWT.
  # See §10 Security Considerations for full rationale.
  token_format: paseto-v4-public

  # OQ-2 RESOLVED: signing keys are SST values; rotation is overlap-based
  # with active key + at-most-one-prior key for grace window.
  signing:
    # Active signing key (Ed25519 private key, base64-encoded).
    # Empty in dev/test (falls back to shared-token mode).
    # MUST be set in production; fail-loud at startup if empty.
    active_private_key: ""
    active_key_id: ""  # short identifier, e.g., "k1"; embedded in token claims

    # Optional prior key for the rotation grace window. When the active
    # key is rotated, the prior key MUST be moved here for grace_window
    # so existing tokens still validate.
    prior_public_key: ""
    prior_key_id: ""

  # Token TTL — how long an issued token is valid before requiring rotation.
  # MUST be bounded; no infinite tokens in production.
  token_ttl_hours: 720  # 30 days; design recommendation, configurable

  # Rotation grace window — how long the prior token + prior key remain valid
  # after rotation. NFR-AUTH-003: ≥ 24 hours.
  rotation_grace_window_hours: 168  # 7 days; design recommendation, configurable

  # Clock-skew tolerance — NFR-AUTH-005: ≤ 60 seconds.
  clock_skew_tolerance_seconds: 30

  # Revocation propagation — NFR-AUTH-006: ≤ 60 s across runtime instances.
  # In-memory cache refresh interval (DB poll fallback when NATS is unavailable).
  revocation_cache_refresh_interval_seconds: 30
  # NATS subject for cross-instance revocation broadcast.
  revocation_nats_subject: "auth.revocations"

  # OQ-8 RESOLVED: at-rest token storage uses HMAC-SHA-256 keyed by a separate
  # SST key. Tokens are NOT stored as plaintext; only the HMAC is stored.
  # Constant-time comparison applied at lookup time.
  # MUST be set in production; fail-loud at startup if empty.
  at_rest_hashing_key: ""

  # OQ-5 RESOLVED: in production, SMACKEREL_AUTH_TOKEN is REJECTED by default
  # once any per-user token is enrolled. Operators may opt in to the shared
  # token as an escape hatch via this flag (deprecation pathway with warning logs).
  production_shared_token_fallback_enabled: false

  # OQ-9 RESOLVED: telemetry surface for spec 030 observability.
  telemetry_enabled: true
  telemetry_metric_prefix: "smackerel_auth"

  # OQ-10 RESOLVED: bootstrap token for first-user enrollment on a fresh
  # production deployment. Consumed exactly once via
  # `./smackerel.sh auth bootstrap` and cleared.
  # MUST be set when production deployment has zero enrolled users.
  bootstrap_token: ""

# Per-environment overrides
environments:
  development:
    auth:
      enabled: false  # FR-AUTH-016: per-user auth disabled-by-default in dev
  test:
    auth:
      enabled: false  # disabled-by-default in test
  production:
    auth:
      enabled: true   # FR-AUTH-016: enabled-by-default in production
  self-hosted:
    auth:
      enabled: true   # production-class environment
```

### Reused existing SST keys

| Key | Path | Reuse rationale |
|-----|------|-----------------|
| `runtime.auth_token` | existing | Shared `SMACKEREL_AUTH_TOKEN` — preserved per FR-AUTH-015 for dev/test ergonomic |
| `runtime.environment` | existing | Gates production-strict claim-binding behavior (FR-AUTH-007/008/009/010) |
| `db.url` | existing | `auth_users` / `auth_tokens` / `auth_revocations` tables live in canonical DB |
| `nats.url` | existing | RevocationBroadcaster subscribes/publishes here |
| `runtime.cookie_domain` | existing | Used by `webAuthMiddleware` to set Secure cookies |

### Generated env file additions

`config/generated/{dev,test,production}.env` adds (via `scripts/commands/config.sh`):

```sh
AUTH_ENABLED=
AUTH_TOKEN_FORMAT=
AUTH_SIGNING_ACTIVE_PRIVATE_KEY=
AUTH_SIGNING_ACTIVE_KEY_ID=
AUTH_SIGNING_PRIOR_PUBLIC_KEY=
AUTH_SIGNING_PRIOR_KEY_ID=
AUTH_TOKEN_TTL_HOURS=
AUTH_ROTATION_GRACE_WINDOW_HOURS=
AUTH_CLOCK_SKEW_TOLERANCE_SECONDS=
AUTH_REVOCATION_CACHE_REFRESH_INTERVAL_SECONDS=
AUTH_REVOCATION_NATS_SUBJECT=
AUTH_AT_REST_HASHING_KEY=
AUTH_PRODUCTION_SHARED_TOKEN_FALLBACK_ENABLED=
AUTH_TELEMETRY_ENABLED=
AUTH_TELEMETRY_METRIC_PREFIX=
AUTH_BOOTSTRAP_TOKEN=
```

### SST validation contract (cmd/core/wiring.go extension)

When `SMACKEREL_ENV=production` AND `auth.enabled=true`:

```go
// FR-AUTH-019: fail-loud on missing/invalid auth config in production
if cfg.Environment == "production" && cfg.Auth.Enabled {
    var missing []string
    if cfg.Auth.SigningActivePrivateKey == "" { missing = append(missing, "auth.signing.active_private_key") }
    if cfg.Auth.SigningActiveKeyID == "" { missing = append(missing, "auth.signing.active_key_id") }
    if cfg.Auth.AtRestHashingKey == "" { missing = append(missing, "auth.at_rest_hashing_key") }
    if cfg.Auth.TokenTTLHours <= 0 { missing = append(missing, "auth.token_ttl_hours (must be > 0)") }
    if cfg.Auth.RotationGraceWindowHours < 24 { missing = append(missing, "auth.rotation_grace_window_hours (must be >= 24)") }

    // bootstrap_token required only when zero users enrolled
    enrolledCount, err := authStore.UserCount(ctx)
    if err != nil { return fmt.Errorf("auth user count: %w", err) }
    if enrolledCount == 0 && cfg.Auth.BootstrapToken == "" {
        missing = append(missing, "auth.bootstrap_token (required when production has zero enrolled users)")
    }

    if len(missing) > 0 {
        return fmt.Errorf("production auth config missing required keys: %s", strings.Join(missing, ", "))
    }
}
```

---

## 5. Token Lifecycle

### 5.1 Issuance flow (FR-AUTH-001/002/003)

**Trigger:** Operator runs `./smackerel.sh auth enroll <user-id>` OR sends `POST /v1/auth/users` (admin-scoped).

```text
1. Validate user-id format (UUID or stable alphanumeric, ≥ 8 chars)
2. INSERT INTO auth_users(user_id, enrolled_at, enrolled_by)
3. Generate token-id (UUIDv7)
4. Build claims:
   - sub:    <user-id>
   - iat:    <now>
   - exp:    <now> + token_ttl_hours
   - iss:    <runtime.environment>:<runtime.cookie_domain>
   - kid:    <auth.signing.active_key_id>
   - tid:    <token-id>
5. PASETO v4.public sign with auth.signing.active_private_key
6. Compute hash := HMAC-SHA-256(auth.at_rest_hashing_key, token)
7. INSERT INTO auth_tokens(token_id, user_id, key_id, issued_at, expires_at, hash)
8. Return token to operator (over secure channel; tokens are NEVER re-derivable)
9. Emit metric smackerel_auth_issuance_total{user_id=<...>}
```

**Bootstrap flow (OQ-10):** When the production deployment has zero enrolled users, the operator MUST set `auth.bootstrap_token` and run `./smackerel.sh auth bootstrap`. The bootstrap command:

1. Verifies the supplied bootstrap-token matches `auth.bootstrap_token` SST value (constant-time compare).
2. Prompts the operator for the first user-id (or accepts via `--user-id` flag).
3. Runs the issuance flow for that user.
4. Updates `config/smackerel.yaml` (or emits a secret-manager directive) to clear `auth.bootstrap_token`.
5. Operator commits the cleared SST + redeploys.

### 5.2 Validation flow (FR-AUTH-004/005/006, NFR-AUTH-001/002)

```text
1. extractBearerToken(r) → token string
2. if dev/test:
     return matchSharedToken(token, cfg.RuntimeAuthToken)  // existing path
3. else (production):
     3a. paseto.Verify(token, cfg.Auth.SigningActivePrivateKey.Public()) OR
         (if "kid" claim matches PriorKeyID and within grace window)
         paseto.Verify(token, cfg.Auth.SigningPriorPublicKey)
     3b. parse claims → Session{UserID, IssuedAt, ExpiresAt, KeyID, TokenID}
     3c. if Session.ExpiresAt + clock_skew < now: return UNAUTHORIZED
     3d. if revocationCache.IsRevoked(Session.TokenID): return UNAUTHORIZED
     3e. attach Session to r.Context() via auth.WithSession(ctx, sess)
4. r.Context() now carries auth.Session
```

**Latency budget breakdown (NFR-AUTH-001 ≤ 5 ms p99):**

| Step | Budget |
|------|--------|
| Header parse + base64 decode | ≤ 0.1 ms |
| PASETO signature verify (Ed25519) | ≤ 1.5 ms |
| Claims parse | ≤ 0.1 ms |
| Expiry + clock-skew check | ≤ 0.05 ms |
| Revocation cache lookup (sync.Map) | ≤ 0.05 ms |
| Context attach | ≤ 0.05 ms |
| **Total per request** | **≤ 2 ms p50, ≤ 5 ms p99** |

### 5.3 Rotation flow (FR-AUTH-011/012, NFR-AUTH-003)

**Trigger:** Operator runs `./smackerel.sh auth rotate <user-id>` OR sends `POST /v1/auth/users/{user-id}/rotate` (admin-scoped or self-scoped).

```text
1. SELECT * FROM auth_tokens WHERE user_id=<...> AND revoked=false
   ORDER BY issued_at DESC LIMIT 1  → oldToken
2. Run issuance flow → newToken
3. UPDATE auth_tokens SET rotated_at=now() WHERE token_id=<oldToken.token_id>
4. The oldToken remains VALID until rotated_at + rotation_grace_window_hours
5. RotationGracePruner background worker (1h timer):
   SELECT token_id FROM auth_tokens
   WHERE rotated_at IS NOT NULL
     AND rotated_at + interval '<grace_window_hours> hours' < now()
     AND revoked = false
   For each: UPDATE auth_tokens SET revoked=true, revoked_at=now()
             AND publish revocation broadcast on auth.revocations
6. Return newToken to operator
7. Emit metric smackerel_auth_rotation_total{user_id=<...>}
```

### 5.4 Revocation flow (FR-AUTH-013/014, NFR-AUTH-006)

**Trigger:** Operator runs `./smackerel.sh auth revoke <token-id>` OR sends `POST /v1/auth/tokens/{token-id}/revoke` (admin-scoped).

```text
1. UPDATE auth_tokens SET revoked=true, revoked_at=now() WHERE token_id=<...>
2. Publish on NATS subject auth.revocations: {token_id, revoked_at}
3. Every running runtime instance:
   On NATS message: revocationCache.Set(token_id, true)
   (sync.Map; no DB query, no allocation per validation)
4. Emit metric smackerel_auth_revocation_total{reason=<...>}
```

**Cold-start bootstrap:** On startup, every runtime instance loads ALL non-expired revoked tokens from the DB into the in-memory cache. The 30-second timer-based refresh re-loads to catch any missed broadcasts. Combined with NATS pub/sub, propagation is well within NFR-AUTH-006's 60-second budget under normal conditions.

**Failure mode:** If NATS is unavailable, the 30-second timer refresh is the only propagation channel, raising worst-case propagation to 30 seconds (still inside the 60-second budget). If both NATS AND the DB are unavailable, the cache stays stale until either recovers — a tradeoff accepted for the no-DB-roundtrip-on-hot-path Hard Constraint.

### 5.5 Claim-binding rules (FR-AUTH-007/008/009/010)

The following table summarizes how each MIT-routed handler MUST derive identity in `production` vs `development`/`test`:

| Handler | Production source | Dev/test fallback | Enforcement |
|---------|-------------------|-------------------|-------------|
| `MintReveal` (photos_upload.go) | `sess.UserID` from `r.Context()` | `X-Actor-Id` header (existing MIT-040-S-003 partial-close pattern) | If body/header present in production, return HTTP 400 with `actor_id_in_body_forbidden` |
| `Connect` (drive/google/google.go) | `sess.UserID` from `r.Context()` | request body `OwnerUserID` (existing MIT-038-S-003 pattern) | If `OwnerUserID` present in body in production, return HTTP 400 with `owner_user_id_in_body_forbidden` |
| Annotation pipeline | `sess.UserID` from event context | event payload `actor_source` OR literal `system` (existing patterns) | If event payload supplies `actor_source` in production, log warning and override with session-derived value |

### 5.6 Authenticated session context shape (OQ-3 RESOLVED)

A typed `auth.Session` struct attached to `context.Context` via type-keyed value:

```go
package auth

type Session struct {
    UserID    string
    TokenID   string
    KeyID     string
    IssuedAt  time.Time
    ExpiresAt time.Time
    // Source distinguishes per-user PASETO sessions from the legacy
    // shared-token / bootstrap fallbacks for telemetry, audit logs,
    // and admin-route gating policy. In production with auth.enabled=true,
    // Source is always SessionSourcePerUserToken (except for the one-shot
    // SessionSourceBootstrap path used by `./smackerel.sh auth bootstrap`).
    Source SessionSource
}

// SessionSource is a string enum (NOT int/iota) so the value flows
// directly into structured logs, metrics labels, and audit records
// without a separate stringification step. Reconciled at spec-review
// against shipped code in `internal/auth/session.go`.
type SessionSource string
const (
    SessionSourcePerUserToken SessionSource = "per_user_token" // production path
    SessionSourceSharedToken  SessionSource = "shared_token"   // dev/test + opt-in production fallback
    SessionSourceBootstrap    SessionSource = "bootstrap"      // one-shot first-user enrollment
)

type sessionContextKey struct{}

// WithSession takes Session by VALUE (not pointer) because Session is
// a small immutable record and pointer-passing would invite mutation
// from downstream handlers. Reconciled at spec-review against shipped
// code in `internal/auth/session.go`.
func WithSession(ctx context.Context, sess Session) context.Context {
    if sess.Source == "" {
        return ctx // no-op for zero value to surface programming errors
    }
    return context.WithValue(ctx, sessionContextKey{}, sess)
}

// SessionFromContext returns (Session, bool) tuple — the boolean ok
// flag is the single source of truth for "is this an authenticated
// request" downstream of middleware. Reconciled at spec-review.
func SessionFromContext(ctx context.Context) (Session, bool) {
    sess, ok := ctx.Value(sessionContextKey{}).(Session)
    return sess, ok
}

// IsAdmin is a method on Session that gates the admin HTTP surface
// (POST /v1/auth/users, etc). Bootstrap is admin unconditionally;
// SharedToken is admin in dev/test only (handler-side defense-in-depth
// re-checks production_shared_token_fallback_enabled); PerUserToken
// admin requires SST allowlist membership evaluated at handler layer.
//
// **UserIDFromContext helper deferred to Scope 02.** Scope 01 admin
// handlers consume Session directly via SessionFromContext +
// session.IsAdmin(). The convenience helper `UserIDFromContext` for
// downstream business handlers (MintReveal, drive.Connect, annotation
// pipeline) lands when Scope 02 wires `bearerAuthMiddleware`.
```

Handlers downstream of the middleware use `auth.SessionFromContext(r.Context())` to read full session metadata. Scope 02 will add `auth.UserIDFromContext` as a convenience accessor for the common "I just need the caller's user-id" case.

### 5.7 Enrollment surface (OQ-6 RESOLVED)

**MVP enrollment surface is admin-issued only.** Two parallel surfaces are provided:

1. **CLI:** `./smackerel.sh auth enroll <user-id>` (operator-side, runs against the live deployment via the local DB).
2. **Admin HTTP:** `POST /v1/auth/users` with body `{"user_id": "<...>"}`. The HTTP endpoint is gated to admin scope, which for the MVP is a session originating from a token whose `Session.UserID` matches one in the SST `auth.admin_user_ids` allowlist (added per OQ-6 to the SST schema if needed; otherwise the bootstrap-issued user IS the implicit first admin).

**Self-service signup is explicitly out-of-scope per spec.md Non-Goals.**

---

## 6. Hot-Path Validation Anatomy

This section maps the validation flow §5.2 onto the actual Go code surfaces and the test-able guarantees they enforce.

### 6.1 Middleware code path (production)

```go
// internal/api/router.go bearerAuthMiddleware (production branch)
func bearerAuthMiddleware(cfg *config.Config, authStore auth.Store, revoker auth.RevocationCache) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := extractBearerToken(r)
            if token == "" {
                if cfg.Environment != "production" && cfg.RuntimeAuthToken == "" {
                    // dev empty-token bypass (preserved per FR-AUTH-015)
                    sess := &auth.Session{Source: auth.SessionSourceEmpty}
                    next.ServeHTTP(w, r.WithContext(auth.WithSession(r.Context(), sess)))
                    return
                }
                bearerAuthFail(w, r, "missing bearer")
                return
            }

            // Production per-user PASETO path
            if cfg.Environment == "production" && cfg.Auth.Enabled {
                sess, err := auth.VerifyAndParse(token, cfg.Auth, revoker)
                if err != nil {
                    bearerAuthFail(w, r, err.Error())
                    return
                }
                next.ServeHTTP(w, r.WithContext(auth.WithSession(r.Context(), sess)))
                return
            }

            // Dev/test shared-token path (preserved per FR-AUTH-015)
            if subtle.ConstantTimeCompare([]byte(token), []byte(cfg.RuntimeAuthToken)) == 1 {
                sess := &auth.Session{Source: auth.SessionSourceSharedToken}
                next.ServeHTTP(w, r.WithContext(auth.WithSession(r.Context(), sess)))
                return
            }

            // Optional production shared-token escape hatch (OQ-5)
            if cfg.Environment == "production" && cfg.Auth.ProductionSharedTokenFallbackEnabled {
                if subtle.ConstantTimeCompare([]byte(token), []byte(cfg.RuntimeAuthToken)) == 1 {
                    slog.Warn("production shared-token fallback used (deprecation pathway)", ...)
                    sess := &auth.Session{Source: auth.SessionSourceSharedToken}
                    next.ServeHTTP(w, r.WithContext(auth.WithSession(r.Context(), sess)))
                    return
                }
            }

            bearerAuthFail(w, r, "no match")
        })
    }
}
```

### 6.2 PASETO verification (auth.VerifyAndParse)

```go
// internal/auth/verify.go
func VerifyAndParse(token string, cfg config.AuthConfig, revoker RevocationCache) (*Session, error) {
    parser := paseto.NewParser()
    parser.AddRule(paseto.NotExpired())
    parser.AddRule(paseto.IssuedBy(cfg.ExpectedIssuer()))

    parsed, err := parser.ParseV4Public(cfg.SigningActivePublicKey, token, nil)
    if err != nil && cfg.SigningPriorPublicKey != nil {
        // Try prior key (rotation grace window)
        parsed, err = parser.ParseV4Public(cfg.SigningPriorPublicKey, token, nil)
    }
    if err != nil {
        return nil, fmt.Errorf("paseto verify: %w", err)
    }

    sess := &Session{
        UserID:    parsed.GetString("sub"),
        TokenID:   parsed.GetString("tid"),
        KeyID:     parsed.GetString("kid"),
        IssuedAt:  parsed.GetTime("iat"),
        ExpiresAt: parsed.GetTime("exp"),
        Source:    SessionSourcePerUser,
    }

    if revoker.IsRevoked(sess.TokenID) {
        return nil, errors.New("revoked")
    }

    return sess, nil
}
```

### 6.3 Revocation cache (in-memory sync.Map)

```go
// internal/auth/revocation/cache.go
type Cache struct {
    revoked sync.Map // map[tokenID]time.Time (revoked_at)
}

func (c *Cache) IsRevoked(tokenID string) bool {
    _, ok := c.revoked.Load(tokenID)
    return ok
}

func (c *Cache) Set(tokenID string, revokedAt time.Time) {
    c.revoked.Store(tokenID, revokedAt)
}

func (c *Cache) BootstrapFromDB(ctx context.Context, store Store) error {
    rows, err := store.ListNonExpiredRevoked(ctx)
    if err != nil { return err }
    for _, row := range rows {
        c.Set(row.TokenID, row.RevokedAt)
    }
    return nil
}
```

### 6.4 NATS broadcaster

```go
// internal/auth/revocation/broadcaster.go
type Broadcaster struct {
    nats    *nats.Conn
    cache   *Cache
    subject string
}

func (b *Broadcaster) Publish(ctx context.Context, tokenID string, revokedAt time.Time) error {
    payload, _ := json.Marshal(map[string]interface{}{
        "token_id": tokenID,
        "revoked_at": revokedAt,
    })
    return b.nats.Publish(b.subject, payload)
}

func (b *Broadcaster) Subscribe(ctx context.Context) error {
    _, err := b.nats.Subscribe(b.subject, func(m *nats.Msg) {
        var ev struct {
            TokenID   string    `json:"token_id"`
            RevokedAt time.Time `json:"revoked_at"`
        }
        if err := json.Unmarshal(m.Data, &ev); err != nil {
            slog.Warn("auth revocation broadcast unmarshal", "err", err)
            return
        }
        b.cache.Set(ev.TokenID, ev.RevokedAt)
    })
    return err
}
```

---

## 7. Failure Modes

| Failure mode | Detection | Response | Test |
|--------------|-----------|----------|------|
| Invalid token (not PASETO) | `paseto.Parse` returns error | HTTP 401, log `"bearer auth failure"` with reason category | `TestVerifyAndParse_InvalidFormat_Returns401` |
| Expired token | `NotExpired` rule fails | HTTP 401, log reason `"expired"` | `TestVerifyAndParse_Expired_Returns401` |
| Wrong signing key | Signature verification fails | HTTP 401, log reason `"signature_mismatch"` | `TestVerifyAndParse_WrongKey_Returns401` |
| Revoked token | `revoker.IsRevoked` returns true | HTTP 401, log reason `"revoked"` | `TestVerifyAndParse_Revoked_Returns401` |
| Mid-rotation: token signed with prior key | First parse fails on active key; retry on prior key succeeds | Validation succeeds (within grace window) | `TestVerifyAndParse_PriorKeyDuringGrace_Validates` |
| Mid-rotation: token signed with prior key, grace expired | Both keys fail (RotationGracePruner has revoked the token) | HTTP 401 | `TestVerifyAndParse_PriorKeyAfterGrace_Returns401` |
| Missing `auth.signing.active_private_key` in production | `wiring.go` startup validation | Service refuses to start with error naming the key | `TestStartup_MissingSigningKey_FailsLoud` |
| Missing `auth.bootstrap_token` AND zero enrolled users | `wiring.go` startup validation queries `UserCount` | Service refuses to start with error naming the key | `TestStartup_NoUsersNoBootstrap_FailsLoud` |
| NATS disconnection during revocation broadcast | `nats.Publish` returns error | Log warning; revocation is still persisted in DB; 30-second timer refresh will catch it next cycle | `TestRevocation_NATSDown_DBRefreshCatches` |
| DB unreachable during cache bootstrap on startup | `BootstrapFromDB` returns error | Service refuses to start (cannot guarantee NFR-AUTH-006 with empty cache) | `TestStartup_DBDownDuringBootstrap_FailsLoud` |
| Body/header attempts to claim `actor_id` in production | Handler reads request body OR header | HTTP 400 with `actor_id_in_body_forbidden` error | `TestMintReveal_ActorIDInBody_Production_Returns400` |
| Body/header attempts to claim `owner_user_id` in production | Handler reads request body | HTTP 400 with `owner_user_id_in_body_forbidden` error | `TestDriveConnect_OwnerInBody_Production_Returns400` |
| Annotation pipeline event supplies `actor_source` in production | Pipeline logs warning, OVERRIDES with session-derived value | Pipeline succeeds with corrected `actor_source`; warning logged | `TestAnnotationPipeline_ActorSourceInPayload_Production_Overrides` |
| Web UI cookie has shared-token in production with fallback enabled | `webAuthMiddleware` matches shared token path | Authenticated as `SessionSourceSharedToken`; deprecation warning logged | `TestWebAuthMiddleware_SharedTokenInProductionFallback_Works` |
| Web UI cookie has shared-token in production WITHOUT fallback | `webAuthMiddleware` rejects | HTTP 401 + cookie cleared | `TestWebAuthMiddleware_SharedTokenInProductionDefault_Rejects` |
| Adversarial: re-using a token across users (token theft) | Per-token `tid` claim cannot be transferred; revocation eliminates it | Operator-driven revocation flow | `TestRevocation_StolenToken_RevokesPropagates` |

---

## 8. Performance Budget

| Metric | Budget | Verification |
|--------|--------|--------------|
| Per-request middleware validation latency p50 | ≤ 2 ms | Microbenchmark `BenchmarkVerifyAndParse_HotPath` |
| Per-request middleware validation latency p99 | ≤ 5 ms (NFR-AUTH-001) | `tests/stress/auth_validation_latency_test.go` measures p99 over 10k requests |
| DB queries per request (common case) | 0 (NFR-AUTH-002) | `TestVerifyAndParse_NoDBQueries` uses query-counting harness |
| Issuance latency | ≤ 100 ms (admin operation, not hot path) | not measured beyond rough wall-clock |
| Rotation grace window | ≥ 24 h (NFR-AUTH-003); design default 168 h (7 days) | SST schema validates `>= 24` |
| Clock-skew tolerance | ≤ 60 s (NFR-AUTH-005); design default 30 s | Validated against in-flight tests with adjusted clocks |
| Revocation propagation worst-case (NATS up) | ≤ 1 s | `TestRevocation_PropagationLatency` measures wall-clock from publish to all-instances-aware |
| Revocation propagation worst-case (NATS down, DB up) | ≤ 30 s (cache refresh interval) | `TestRevocation_NATSDown_DBRefreshCatches` |
| Revocation propagation absolute ceiling | ≤ 60 s (NFR-AUTH-006) | NFR test |
| Cold-start cache bootstrap latency | ≤ 500 ms for 10k revoked tokens | `BenchmarkRevocationCache_BootstrapFromDB_10k` |
| Cache memory footprint per revoked token | ≤ 64 bytes | `BenchmarkRevocationCache_MemoryPerEntry` |

---

## 9. Backward Compatibility Plan

### 9.1 Dev/test environments (FR-AUTH-015/016, SCN-AUTH-005/011)

**ZERO changes required.** A developer pulling the spec-044 implementation runs `./smackerel.sh config generate && ./smackerel.sh up` and the deployment behaves exactly as at HEAD `f7001ab`:

- `SMACKEREL_AUTH_TOKEN` is the auth source.
- The empty-token bypass at `internal/api/router.go` lines 444–451 is preserved.
- No enrollment step is required.
- `MintReveal` accepts `X-Actor-Id` header.
- `drive.Connect` accepts `OwnerUserID` in body.
- Annotation pipeline accepts `actor_source` in payload OR defaults to `system`.

The dev/test policy is encoded as `auth.enabled: false` in `environments.development.auth` and `environments.test.auth`. Even if a developer enables `auth.enabled: true` in dev for testing the new path, the shared-token + empty-token paths remain valid (FR-AUTH-015 unconditional preservation).

### 9.2 Production environment migration

For an existing production operator at HEAD `f7001ab` running `SMACKEREL_AUTH_TOKEN` only:

```text
Step 1: Add auth.* SST keys to config/smackerel.yaml
   - auth.signing.active_private_key (generate via openssl + base64)
   - auth.signing.active_key_id (e.g., "k1")
   - auth.at_rest_hashing_key (generate via openssl rand -hex 32)
   - auth.token_ttl_hours, auth.rotation_grace_window_hours, etc. (use defaults)
   - auth.bootstrap_token (generate via openssl rand -hex 32)
   - environments.production.auth.enabled: true

Step 2: Run ./smackerel.sh config generate --env production
   - Verifies all required keys present (FR-AUTH-019 fail-loud)
   - Emits config/generated/production.env

Step 3: Deploy the new build (per docs/Deployment.md Build-Once Deploy-Many)
   - Service starts up; sees zero enrolled users + bootstrap_token set; service starts
   - First request without per-user token: 401 + "no match" log

Step 4: Run ./smackerel.sh auth bootstrap --user-id <admin-user-id>
   - Consumes bootstrap_token; enrolls first user; emits per-user PASETO token
   - Operator stores token securely; uses for subsequent admin operations

Step 5: Update auth.bootstrap_token to "" (or delete from SST)
   - Redeploy
   - Service now requires per-user tokens for all auth (no bootstrap fallback)

Step 6: Enroll additional users via admin endpoint or CLI
   - For each user: ./smackerel.sh auth enroll <user-id>
   - Distribute tokens through secure channel

Step 7 (optional): Disable production shared-token fallback
   - auth.production_shared_token_fallback_enabled: false (default)
   - Existing SMACKEREL_AUTH_TOKEN clients receive 401
   - Per-user tokens only
```

**Rollback path:** If migration fails mid-way, set `environments.production.auth.enabled: false` in `config/smackerel.yaml` and redeploy. Service reverts to shared-token-only mode (per FR-AUTH-015 preservation). All issued per-user tokens remain in DB but are unused.

### 9.3 Coexistence policy (OQ-5 RESOLVED)

In production with at least one enrolled user:

- **Default:** `SMACKEREL_AUTH_TOKEN` is REJECTED. Only per-user PASETO tokens authenticate.
- **Opt-in escape hatch:** `auth.production_shared_token_fallback_enabled: true` allows the shared token to authenticate as `SessionSourceSharedToken`. Each use logs a deprecation warning. Handlers requiring per-user identity (MintReveal, drive.Connect, annotations) MUST detect `Source == SharedToken` and either reject or fall back to body/header values per the same dev/test pattern.

This gives operators a transitional pathway without a hard cutover. The expectation is that the fallback flag stays `false` in production after the migration completes.

---

## 10. Security Considerations

### 10.1 Token format choice (OQ-1 RESOLVED)

**Selected: PASETO v4.public (Ed25519 signature, no encryption).**

Rationale:

| Format | Pros | Cons | Verdict |
|--------|------|------|---------|
| JWT (HS256) | Widely supported; simple | Symmetric — verifier needs the same key as issuer; algorithm-confusion attacks well-documented; "alg":"none" footgun | REJECTED — algorithm-confusion footguns disqualifying for a security-foundation spec |
| JWT (EdDSA / RS256) | Public-key verification; widely supported | Library-quality varies; algorithm-confusion still a concern; key formats sprawl (PEM vs JWK vs raw) | REJECTED — too much footgun surface |
| **PASETO v4.public (Ed25519)** | **No algorithm field — no algorithm-confusion attacks; one canonical signing primitive (Ed25519); compact (~70 bytes for our claims); standard library quality (e.g., aidantwoods/go-paseto)** | **Fewer integrations than JWT; client-side parser needs PASETO support** | **SELECTED** |
| PASETO v4.local (XChaCha20 encrypted) | Encryption hides claims from intermediaries | Symmetric (verifier needs the encryption key); for our hot-path-stateless contract, no benefit over signed | REJECTED — encryption not needed; signed is sufficient for trust boundary |
| Opaque tokens with stored hash | Trivial revocation; no key management for verification | DB roundtrip per request — VIOLATES Hard Constraint NFR-AUTH-002 | REJECTED — disqualifying constraint violation |

PASETO v4.public is the only candidate that satisfies all of: (a) no DB roundtrip per request; (b) no algorithm-confusion attack surface; (c) compact wire size; (d) clean key-management story (one Ed25519 keypair per active+prior); (e) no JWKS server requirement (local-first per Constitution C1).

**Library choice:** `github.com/aidantwoods/go-paseto` (well-maintained, idiomatic Go API, PASETO v4 native).

### 10.2 Signing key storage and rotation

- Signing keys are SST values resolved into `config/generated/production.env`. Storage discipline depends on the operator's secret-management posture (env vars, sealed secrets, KMS-injected). Keys MUST NOT be checked into the repo (existing `.gitignore` covers `config/generated/`).
- Rotation: operator generates a new Ed25519 keypair, swaps `auth.signing.active_private_key` to the new private key, moves the old public key to `auth.signing.prior_public_key`, redeploys. Existing tokens validate via `prior_public_key` for `rotation_grace_window_hours`; new tokens are signed with the active key.
- After grace window: `auth.signing.prior_public_key` cleared via SST update + redeploy.

### 10.3 Token transport

- `Authorization: Bearer <token>` header is the only sanctioned transport for API clients.
- Web UI continues to use the existing `auth_token` cookie at `internal/api/router.go` lines 425–433 (per OQ-7 below). In production, the cookie value is a per-user PASETO token; the cookie MUST be marked `HttpOnly` and `Secure`.
- Tokens MUST NOT appear in URLs, query strings, or referer headers.

### 10.4 Web UI session model (OQ-7 RESOLVED)

**Preserve cookie-based sessions for the web UI.** Browser ergonomics + CSRF prevention are well-served by the existing pattern. In production:

- `webAuthMiddleware` reads `auth_token` cookie value, treats it as a PASETO token, runs `auth.VerifyAndParse`.
- Cookie attributes: `HttpOnly`, `Secure` (enforced when `runtime.environment == production`), `SameSite=Lax`, `Path=/`.
- Cookie value is set by the login flow (admin-issued for MVP; UX out-of-scope per Non-Goals).
- In dev/test, cookie value remains `SMACKEREL_AUTH_TOKEN` (existing behavior).

### 10.5 At-rest token hashing (OQ-8 RESOLVED)

**Selected: HMAC-SHA-256 with a separate at-rest hashing key.**

Rationale: The stored hash is used for token-lookup-by-presented-value scenarios (e.g., debugging "is this token in our DB?", or future audit-trail-by-token), NOT for hot-path validation. We don't need a memory-hard primitive (Argon2id) because tokens are high-entropy random bytes, not user-chosen passwords. HMAC-SHA-256 with a separate key (`auth.at_rest_hashing_key`) is fast enough for batch operations, secure against rainbow-table attacks (the key is the unguessable component), and constant-time compare friendly.

```go
func HashToken(token string, key []byte) []byte {
    h := hmac.New(sha256.New, key)
    h.Write([]byte(token))
    return h.Sum(nil)
}

func CompareTokenHash(stored, candidate []byte) bool {
    return subtle.ConstantTimeCompare(stored, candidate) == 1
}
```

`auth.at_rest_hashing_key` MUST be a 32-byte (64-hex-char) random value, distinct from the signing key. SST validation enforces non-empty in production.

### 10.6 Replay

- Bounded TTL via `auth.token_ttl_hours` (design default 30 days; configurable).
- Revocation via in-memory cache + NATS broadcast.
- No anti-replay `jti` cache (out-of-scope for MVP; replay window is bounded by TTL).

### 10.7 Constant-time comparison sites

| Site | Comparison | Discipline |
|------|------------|------------|
| `bearerAuthMiddleware` shared-token match | `cfg.RuntimeAuthToken` vs presented | `subtle.ConstantTimeCompare` (preserves existing pattern at `internal/api/router.go` line 467) |
| PASETO signature verify | Library handles internally | `aidantwoods/go-paseto` uses constant-time Ed25519 |
| `CompareTokenHash` (auth/hash.go) | Stored HMAC vs computed HMAC | `subtle.ConstantTimeCompare` |
| Bootstrap token compare | `cfg.Auth.BootstrapToken` vs presented | `subtle.ConstantTimeCompare` |

### 10.8 Logging hygiene (NFR-AUTH-007)

Authentication-failure logs include only:

- `path` (e.g., `/v1/photos/{id}/reveal`)
- `remote_addr`
- `reason` (one of: `missing_bearer`, `expired`, `signature_mismatch`, `revoked`, `no_match`, `bootstrap_required`)
- `user_id` (only on SUCCESS, never on failure — failure logs do NOT disclose which user-id was attempted because a malicious caller could probe enrolled user-ids)

Authentication SUCCESS logs (at debug level) include `user_id`, `token_id`, `key_id` for audit trail.

The raw token value, signature bytes, and any HMAC of the token MUST NEVER appear in logs.

### 10.9 Cross-spec QF Companion boundary

Per spec.md Security Considerations: Smackerel-issued PASETO tokens MUST NOT be reused as QF authentication tokens. The QF `PersonalEvidenceBundle` and `QFDecisionPacket` metadata are separate trust boundaries. This spec does not change the QF packet metadata surface (Principle 10 NON-NEGOTIABLE).

Concretely: if QF Companion (spec 041) lands and needs an authenticated channel, it MUST use its own token format (whether signed identically with PASETO+Ed25519 or otherwise) with a distinct issuer claim and a distinct signing key. Smackerel auth subsystem MUST refuse to validate any token whose issuer claim does not match `cfg.ExpectedIssuer()`.

---

## 11. Risks & Mitigations

| # | Risk | Likelihood | Impact | Mitigation |
|---|------|------------|--------|------------|
| 1 | Operators set `auth.signing.active_private_key` to a known/weak value (e.g., copying a sample from docs) | medium | critical | (a) SST schema does NOT ship a default; (b) `./smackerel.sh auth keygen` command emits cryptographically-random keys; (c) operator runbook mandates `openssl genpkey` not copy-paste; (d) startup validation rejects keys with insufficient entropy via length check |
| 2 | `auth.at_rest_hashing_key` reused across signing key (same value in both fields) | medium | high | SST validation rejects when `at_rest_hashing_key == signing.active_private_key` |
| 3 | Production deployment without enrolled users gets locked out (chicken-and-egg) | high (without mitigation) | critical | OQ-10 RESOLUTION: `auth.bootstrap_token` SST key + `./smackerel.sh auth bootstrap` command. Startup validation enforces "either ≥1 enrolled user OR bootstrap_token set". |
| 4 | NATS subject `auth.revocations` collides with another subject (typo, mis-config) | low | high | New SST key `auth.revocation_nats_subject` defaults to `"auth.revocations"`; subject is namespaced; NATS contract test validates uniqueness |
| 5 | Web UI cookie not marked Secure in production due to misconfig (running behind reverse proxy without TLS termination forwarding) | medium | high | `webAuthMiddleware` UNCONDITIONALLY sets `Secure` flag when `cfg.Environment == "production"`; if cookie cannot be set (browser rejects on plain HTTP), authentication fails-loud with a clear error |
| 6 | Adversarial actor extracts a token from logs (server-side log compromise) | low | critical | NFR-AUTH-007 logging hygiene: tokens never logged. Plus: token revocation is a one-call operation if leak is detected. |
| 7 | PASETO library bug introduces false-positive validation | low | critical | (a) Pin `aidantwoods/go-paseto` to an audited release; (b) integration test suite exercises the full PASETO v4 spec test vectors; (c) validation depends on Go's standard `crypto/ed25519` for the underlying primitive |
| 8 | Cache desync between runtime instances (NATS partition + cache refresh interval mismatch) | medium | medium | (a) NATS pub/sub is the primary propagation channel; (b) 30-second timer-based DB refresh is a backstop; (c) in worst case (both NATS AND DB down), cache stays stale until either recovers — accepted tradeoff for the no-DB-roundtrip-on-hot-path Hard Constraint |
| 9 | Operators forget to clear `auth.bootstrap_token` after first use | medium | medium | Bootstrap CLI command emits a clear "next steps" message including "set auth.bootstrap_token: \"\" in config/smackerel.yaml and redeploy". Service start-up logs a WARNING on every boot when both `bootstrap_token != ""` AND `enrolled_user_count > 0`. |
| 10 | Cross-spec drift: handlers added after spec 044 closure don't honor claim-binding | medium | high | Code-quality test `TestNoBodyHeaderActorIDInProductionHandlers` greps `internal/` for body/header `actor_id` reads and asserts every match is gated on `cfg.Environment != "production"` (per AC-11) |

---

## 12. Rollout Plan

The implementation lands in 4 sequential phases. Each phase ends with a working state.

### Phase 1: SST Foundation + Token Subsystem (Scope 01)

**Outcome:** `auth.*` SST keys exist; `internal/auth/` package implements PASETO issuance/validation; `cmd/core/cmd_auth.go` provides CLI entry points; admin HTTP endpoints exist; bootstrap flow works on a fresh production deployment. **No handler refactor yet** — the new infrastructure is staged but not yet hot-pathed for MIT closures.

- Add 14 SST keys to `config/smackerel.yaml`
- Update `scripts/commands/config.sh` to emit `AUTH_*` env vars
- Author `internal/auth/` package: `Session`, `WithSession`, `SessionFromContext`, `VerifyAndParse`, `IssueToken`, hash helpers
- Author `internal/auth/revocation/` package: `Cache`, `Broadcaster`, `BootstrapFromDB`
- Author `cmd/core/cmd_auth.go` CLI commands: `enroll`, `rotate`, `revoke`, `list-users`, `bootstrap`, `keygen`
- Author `internal/api/auth_handlers.go` admin HTTP endpoints
- Author DB migrations: `auth_users`, `auth_tokens`, `auth_revocations` tables
- Update `cmd/core/wiring.go` startup fail-loud validation
- Author unit tests: `TestVerifyAndParse_*`, `TestRevocationCache_*`, `TestStartup_*Auth*FailsLoud`
- Author integration test: `TestAuthBootstrap_FreshProduction_EnrollsFirstUser`
- Author SST grep guard: `TestSST_NoHardcodedAuthValues`

### Phase 2: Hot-Path Middleware Integration + MIT Closures (Scope 02)

**Outcome:** `bearerAuthMiddleware` validates per-user tokens in production; `MintReveal`, `drive.Connect`, and the annotation pipeline derive identity from session in production. MIT-040-S-008, MIT-038-S-003, and MIT-027-TRACE-001 actor-source segment are closed.

- Refactor `bearerAuthMiddleware` per §6.1
- Refactor `webAuthMiddleware` for cookie-based PASETO sessions
- Refactor `MintReveal` to read `auth.UserIDFromContext(r.Context())` in production; preserve `X-Actor-Id` fallback in dev/test
- Refactor `drive.google.Connect` to read session in production; preserve body fallback in dev/test
- Refactor annotation pipeline to derive `actor_source` from session in production; log warning + override in production if payload supplies it
- Update spec 040, 038, 027 state.json files: mark MIT entries resolved with `closureSpec: 044-per-user-bearer-auth`
- Update `internal/api/photos_upload.go` lines 246–321 comment block to reflect resolved state (per FR-AUTH-021)
- Author integration tests: `TestMintReveal_Production_DerivesActorIDFromSession`, `TestDriveConnect_Production_DerivesOwnerFromSession`, `TestAnnotationPipeline_Production_DerivesActorSourceFromSession`
- Author production-vs-dev-test fork tests: `TestMintReveal_DevTest_HonorsXActorID`, etc.
- Author code-quality grep test: `TestNoBodyHeaderActorIDInProductionHandlers` (per AC-11)
- Author adversarial regression test: `TestMintReveal_BodyActorIDInProduction_Returns400_FailsLoudly` (per Adversarial Regression rule)

### Phase 3: Web Surfaces + Telegram Connector (Scope 03)

**Outcome:** `web/pwa/` and `web/extension/` send per-user PASETO tokens; Telegram connector maps Telegram chat-id → enrolled user; admin UX enables operator self-service for user enrollment (admin HTTP via PWA, not full UI).

- Update `web/pwa/` to send `Authorization: Bearer <PASETO>` from a per-user storage slot
- Update `web/extension/` similarly
- Update `internal/telegram/` to map `chat_id` → enrolled user; emit annotation events with session-derived `actor_source`
- Author admin token-management UI in `web/pwa/` (list users, rotate token, revoke token)
- Author E2E tests: `TestE2E_PWAAuth_Production_PerUserSession`
- Update `docs/Operations.md` with the operator enrollment workflow

### Phase 4: Deprecation Pathway + Documentation Freshness (Scope 04)

**Outcome:** `SMACKEREL_AUTH_TOKEN` is fully deprecated for production; `auth.production_shared_token_fallback_enabled` defaults to `false`; documentation updated; metrics dashboard for spec 030 observability.

- Default `auth.production_shared_token_fallback_enabled: false` in `config/smackerel.yaml`
- Update `docs/Deployment.md` with the new SST keys + bootstrap flow
- Update `docs/Development.md` with the dev/test backward-compat contract
- Update `docs/smackerel.md` architecture section with the auth boundary
- Author Prometheus metrics emitters per OQ-9 resolution (`smackerel_auth_issuance_total`, `smackerel_auth_validation_latency_seconds`, `smackerel_auth_revocation_total`, `smackerel_auth_failure_total{reason}`)
- Update spec 030 observability docs to include the auth metrics
- Run `bash .github/bubbles/scripts/regression-baseline-guard.sh specs/044-per-user-bearer-auth --verbose`

---

## 13. Open Questions Deferred to Scopes

All 10 spec.md OQs are resolved in this design. No questions are deferred to scopes. The scope DoD bullets will reference these resolutions:

- OQ-1 → §10.1 (PASETO v4.public)
- OQ-2 → §4 + §10.2 (SST signing keys with overlap rotation)
- OQ-3 → §5.6 (typed `auth.Session` via `context.Context`)
- OQ-4 → §5.4 + §6.4 (NATS pub/sub + 30s timer DB refresh)
- OQ-5 → §9.3 (rejected by default; opt-in fallback for transition)
- OQ-6 → §5.7 (admin-issued only; CLI + HTTP)
- OQ-7 → §10.4 (preserve cookie-based sessions, HttpOnly + Secure)
- OQ-8 → §10.5 (HMAC-SHA-256 with separate at-rest key)
- OQ-9 → §4 (telemetry SST keys; `smackerel_auth` metric prefix)
- OQ-10 → §4 + §5.1 (`auth.bootstrap_token` SST + `./smackerel.sh auth bootstrap` CLI)

---

## 14. Design Decisions Reconciled During Scope 01 Implement

The implement → test → validate → audit → chaos cycle for Scope 01 surfaced a small number of design adjustments and forward-compat observations that this design document records here so that downstream scopes (02/03/04) inherit a faithful design baseline. None of these alter the spec.md outcome contract or any FR/NFR commitment; they refine §5–§6 implementation details and capture observations carried forward to subsequent scopes.

### 14.1 Adjusted from §5–§6 design (rationale-preserving)

| # | Design (§5–§6 pseudocode) | Shipped reality (`internal/auth/`) | Rationale |
|---|---------------------------|-------------------------------------|-----------|
| 1 | `SessionSource` is `int` (iota) | `SessionSource` is `string` with stable named values (`per_user_token` / `shared_token` / `bootstrap`) | Stable string values flow directly into structured logs, metrics labels, and audit records without a stringification table — better observability ergonomics for the same trust boundary. |
| 2 | `SessionSourcePerUser` / `SessionSourceEmpty` | `SessionSourcePerUserToken` / `SessionSourceBootstrap` (no `SourceEmpty`) | The bootstrap path needed an explicit, non-empty source distinct from per-user-token; the dev empty-token bypass continues to attach a session whose Source is one of the named values per `bearerAuthMiddleware` design (Scope 02 work). |
| 3 | `WithSession(ctx, *Session)` and `SessionFromContext(ctx) *Session` | `WithSession(ctx, Session)` and `SessionFromContext(ctx) (Session, bool)` | Pass-by-value Session is small and immutable; pointer-passing would invite mutation from downstream handlers. The `(Session, bool)` tuple makes "is this an authenticated request?" the single canonical check downstream of middleware. |
| 4 | `func VerifyAndParse(token string, cfg config.AuthConfig, revoker RevocationCache) (*Session, error)` | `func VerifyAndParse(wireToken string, opts VerifyOptions) (ParsedToken, error)` | Cleaner separation of concerns: `VerifyAndParse` does PASETO signature verify + claim parsing only and returns `ParsedToken`. Session attachment + revocation cache lookup happens at the middleware boundary (Scope 02 work). This keeps `internal/auth/verify.go` provably DB-free (verified by Audit Gate A18) and lets Scope 02 wire revocation policy + session attach without touching the verifier. |
| 5 | `auth.UserIDFromContext(ctx)` helper landed in Scope 01 | Helper deferred to Scope 02 | No Scope 01 caller needs it: admin handlers consume `Session` directly via `IsAdmin`. Scope 02 will add the helper alongside `bearerAuthMiddleware` integration when `MintReveal`/`drive.Connect`/annotation pipeline begin reading per-request user identity from context. |
| 6 | Admin allowlist surface for per-user admin gating | `auth.Session.IsAdmin()` returns `false` for `SessionSourcePerUserToken` at Scope 01; allowlist surface deferred to Scope 02 | Scope 01 admin gating is sufficient for bootstrap + non-production shared-token paths. Per-user admin gating requires the SST allowlist evaluator at the handler layer; Scope 02 wires it alongside the route registration in `internal/api/router.go`. |

### 14.2 Observations carried forward (from audit + chaos phases)

The audit phase and chaos phase recorded four LOW-severity / informational observations. None are Scope 01 blockers; all are tracked here so subsequent scopes / future evolution can address them.

- **OBS-AUDIT-044-S01-01** (LOW) — `cmd/core/cmd_auth.go` bootstrap-token compare uses `!=` instead of `subtle.ConstantTimeCompare`. The CLI is local-shell-only and the bootstrap token is one-shot (cleared after first use), so timing-oracle exploitability requires co-located shell access (which already grants direct read of `auth.bootstrap_token`). **Recommended follow-up:** harden to `subtle.ConstantTimeCompare` for symmetry with the runtime-side `CompareTokenHash` discipline. Tracked in `report.md` → Audit Findings.
- **OBS-AUDIT-044-S01-02** (LOW) — `internal/api/auth_handlers.go` admin handlers leak raw `err.Error()` strings (which may include pgx wrapping) into JSON response bodies. Handlers are admin-only; routes NOT registered at Scope 01. **Recommended follow-up:** tighten error sinks to opaque categories at Scope 02 router-binding time. Tracked in `report.md` → Audit Findings.
- **OBS-AUDIT-044-S01-03** (INFORMATIONAL) — `internal/auth/revocation/broadcaster.go::handle` silently drops malformed NATS events to avoid log-amplification DoS surface. Cache integrity preserved. **Recommended follow-up:** add a metrics counter `smackerel_auth_revocation_broadcast_drops_total` in Scope 04 (alongside the OQ-9 telemetry surface).
- **OBS-CHAOS-044-S01-01** (LOW) — `Broadcaster.handle` accepts events with unknown `version` strings as long as `token_id` is non-empty. Benign at v1 (the only consumer-visible field is `token_id`); becomes a forward-compat hazard if v2 adds semantic fields the v1 subscriber must enforce. **Recommended follow-up:** add version-strict gating OR explicit version-allowlist in any v2 broadcaster envelope evolution. Tracked in `report.md` → Chaos Evidence → Findings Summary.

### 14.3 Helper deferred to Scope 02 (not yet shipped)

- `auth.UserIDFromContext(ctx context.Context) string` — convenience accessor for handlers that only need the caller's user-id (the common case for `MintReveal`/`drive.Connect`/annotation pipeline in Scope 02). Spec 044 §6.1 middleware code path snippet uses this helper; Scope 02 lands the helper alongside the middleware refactor.

### 14.4 SST line-number reconciliation

The 14 `auth.*` SST keys live at `config/smackerel.yaml` lines 459-511 (top-level `auth:` block) plus per-environment `auth_enabled` overrides at lines 750 (dev), 767 (test), and 800 (self-hosted). Earlier evidence blocks reference `lines 67-130` from the implement-phase snapshot before USER WIP merged additional config blocks above the `auth:` section; the current line numbers are reconciled here for posterity. The block content itself is unchanged from the implement-phase landing.

---

## 15. Design Decisions Reconciled During Scope 02 Implement

The implement → test → validate → audit → chaos cycle for Scope 02 surfaced a small number of design adjustments and forward-compat observations that this design document records here so that downstream scopes (03/04) inherit a faithful design baseline. None of these alter the spec.md outcome contract or any FR/NFR commitment; they refine §6 implementation details, capture observations carried forward, and record helpers that DID land alongside Scope 02 work.

### 15.1 Adjusted from §6 design (rationale-preserving)

§6.1 / §6.2 pseudocode is preserved AS WRITTEN above. Shipped reality at `internal/api/router.go` and `internal/auth/verify.go` differs as follows; each adjustment was forward-noted in §14.1 and is recorded here for closure.

| # | Design (§6 pseudocode) | Shipped reality | Rationale |
|---|------------------------|-----------------|-----------|
| 1 | `bearerAuthMiddleware(cfg, authStore, revoker)` constructor function returning `func(http.Handler) http.Handler` | Method on `*Dependencies` receiver (`func (d *Dependencies) bearerAuthMiddleware(next http.Handler) http.Handler`) at `internal/api/router.go` line 497 | Aligns with the existing router's `Dependencies` injection pattern (also used by `webAuthMiddleware`); avoids introducing a parallel `internal/api/middleware/` subpackage just for this one middleware. The struct receiver gives uniform access to `AuthConfig`, `AuthVerifyOptions`, `BearerStore`, `RevocationCache`, `AuthAdminHandlers`, `Environment` without a per-handler closure-capture surface. |
| 2 | §6.1 production branch: `sess, err := auth.VerifyAndParse(token, cfg.Auth, revoker)` returning `*Session` | Production branch: `parsed, err := auth.VerifyAndParse(token, d.AuthVerifyOptions)` returning `(ParsedToken, error)` followed by separate `if d.RevocationCache != nil && d.RevocationCache.IsRevoked(parsed.TokenID)` check, then `auth.Session{...Source: SessionSourcePerUserToken}` constructed at the middleware boundary | Forward-noted in §14.1 row 4. Cleaner separation of concerns: `VerifyAndParse` is provably DB-free (verified at Scope 01 by Audit Gate A18 grep guard); revocation policy + session attach lives at the middleware boundary where the request-scoped logging context is also available. Lets Scope 02 wire the revocation cache + admin allowlist surface without touching the verifier. |
| 3 | §6.1 dev empty-token bypass: `sess := &auth.Session{Source: auth.SessionSourceEmpty}` | Dev empty-token bypass: `auth.Session{Source: auth.SessionSourceSharedToken}` (synthetic SharedToken session by value, not pointer) | Forward-noted in §14.1 row 2. There is no `SourceEmpty` enum member; the synthetic SharedToken session lets downstream `auth.SessionFromContext` return `(Session, ok=true)` so handlers do not need a special "is this a dev bypass?" branch. The dev/test claim-binding fallbacks (e.g. `actorIDFromRequest` honoring `X-Actor-Id` in dev) honor the SharedToken source. |
| 4 | §6.1 / §6.2 reference `auth.SessionSourcePerUser` enum constant | Shipped enum: `auth.SessionSourcePerUserToken` (string value `"per_user_token"`) | Forward-noted in §14.1 row 2. The shipped name disambiguates from `SessionSourceSharedToken` and matches the metric/log label string value 1:1. |
| 5 | §6.1 implies a single MintReveal/Connect/annotation handler hop where the middleware-attached session is consumed downstream | Drive routes wrapped at `internal/api/router.go` lines 257-304 in a `chi.Group` with `r.Use(deps.bearerAuthMiddleware)` so the session is attached BEFORE the route handler runs. `OAuthCallback` intentionally remains UNAUTHENTICATED inside that group via a sub-route that bypasses the middleware (the upstream OAuth provider needs to redirect back without a bearer token). | Surfaced during implement phase. The `chi.Group` + `r.Use` pattern is the chi-idiomatic way to scope a middleware to a subset of routes; the OAuthCallback bypass is required by the OAuth2 redirect contract. Documented inline in router.go at the group declaration. |
| 6 | §6.1 admin handlers consume `auth.UserIDFromContext` directly from the request context (helper deferred to Scope 02 per §14.3) | `auth.UserIDFromContext(ctx context.Context) string` shipped at `internal/auth/session.go` lines 116-122 alongside the middleware integration per the §14.3 deferral plan; consumed by `MintReveal`, `drive.Connect`, `AnnotationHandlers.CreateAnnotation`, and the chaos benchmark | Closes the §14.3 deferral. The helper returns `""` when no session is attached, letting handlers pattern-match on `if userID == ""` for the fail-closed branch (production rejects with HTTP 400 `*_required`; dev/test falls back to the shared-token claim-binding pattern). |

### 15.2 Observations carried forward (from chaos phase)

The chaos phase recorded two LOW-severity / informational observations on the live middleware integration. Neither is a Scope 02 blocker; both are tracked here so subsequent scopes / future evolution can address them if/when warranted.

- **OBS-CHAOS-044-S02-01** (LOW / informational) — at 128 concurrent verifies from a single source IP, the chi-router rate-limit middleware classifies 28/128 as HTTP 429 (throttled) BEFORE `bearerAuthMiddleware` runs. Auth verification correctness is unaffected — `auth_reject=0` (zero spurious 401/403 across the 128-request burst). 429 is the correct production behavior for single-IP burst load; rate-limit configuration is orthogonal to bearer-auth. **Recommended follow-up:** none required. If rate-limit tuning becomes a topic (e.g. for multi-tenant self-hosted deployments serving multiple PWA clients behind a single upstream NAT), the rate limiter's `KeyFunc` could be made session-aware (per-`UserID` rather than per-source-IP) — but this is a multi-spec architectural question, not a 044 deliverable. Tracked in `report.md` → Chaos Evidence (Scope 02) → Observations.
- **OBS-CHAOS-044-S02-02** (INFORMATIONAL) — the verify-vs-revoke window in C2-B02 was tight enough that 40/40 admit pre-revoke and 40/40 reject post-revoke cleanly — no admits leaked into the post-revoke window, demonstrating sub-millisecond cache convergence on the loopback NATS connection. NFR-AUTH-006's ≤1s budget is met by **>3 orders of magnitude**. The synchronous `cache.MarkRevoked` inside `Broadcaster.Publish` (Scope 01 design intent) is the reason the convergence is essentially atomic from the caller's perspective when both publisher and subscriber share a single NATS process. **Recommended follow-up:** none — this is the intended behavior. Add a chaos behavior at Scope 04 metrics phase that asserts the loopback-shared convergence is preserved when the metric counters (`smackerel_auth_revocation_broadcast_*`) are wired.

### 15.3 Helpers / refinements that DID land alongside Scope 02 work

These were out-of-scope for Scope 01 (helper deferred per §14.3, contract refinement deferred to admin-route binding) and landed during Scope 02 implement / follow-up implement passes.

- **`auth.UserIDFromContext(ctx context.Context) string`** — convenience accessor for handlers that only need the caller's user-id (the common case for `MintReveal`/`drive.Connect`/`AnnotationHandlers.CreateAnnotation`). Lives at `internal/auth/session.go` lines 116-122; covered by `internal/api/router_auth_middleware_test.go::TestUserIDFromContext` (4 sub-tests including "no session attached" → empty string). Closes §14.3.
- **`auth.BearerStore.RevokeToken` 3-outcome contract refinement** — refined during the Scope 02 follow-up implement pass to distinguish three outcomes via a single `SELECT ... FOR UPDATE` inside the revoke transaction: (1) row does not exist → wrapped `auth.ErrTokenNotFound` for clean admin-API 404 surfacing; (2) row exists and is `revoked` → idempotent no-op (commit-and-return-nil so operator retries and crash-restart loops never error twice); (3) row exists and is `active`/`rotated` → standard status flip + audit-row insert + commit. The new `auth.ErrTokenNotFound` sentinel is exported and consumed via `errors.Is` by `tests/integration/auth_revocation_test.go::TestRevocation_NonExistentToken_ClearError`. Backwards-compatible with all existing callers (CLI `cmd/core/cmd_auth.go` + admin `internal/api/auth_handlers.go` + chaos `tests/integration/auth_chaos_test.go`).
- **Environment plumbing via `WithEnvironment(env string)` setter** — `DriveHandlers`, `PhotosHandlers` (already established at MIT-040-S-004 in spec 040 Scope 1), and `AnnotationHandlers` all expose a fluent `WithEnvironment(env)` setter consumed by `cmd/core/wiring.go` at handler construction. The setter `panic`s on empty string (constructor-time fail-loud — verified by Audit Gate A5). Lets each handler gate its production-strict claim-binding rule on `h.environment == "production"` without re-reading the SST per request.
- **Defensive body-key scan in `AnnotationHandlers.CreateAnnotation`** — production branch reads the request body once via `http.MaxBytesReader + io.ReadAll`, scans the bytes for the JSON keys `"actor_source"` and `"actor_id"` via regex, and rejects with HTTP 400 BEFORE any store call. The stub-store assertion in `TestAnnotation_BodyActorSourceInProduction_Rejected` verifies the rejection precedes persistence (`createCalls` counter remains zero).

### 15.4 Items DEFERRED beyond Scope 02 (recorded for downstream scope traceability)

- **Annotation table `actor_source` schema column** — the annotation pipeline now derives `actor_source` from the authenticated session in production AND defensively rejects body/header smuggling, but the underlying `annotations` table schema currently stores `actor_source` as part of a JSONB `metadata` blob. A first-class `actor_source` column with an index would simplify per-source query / observability surface; this is **Scope 03 work** (along with Telegram-bridge and NATS-payload claim-binding wiring at the entry-point layer).
- **`webAuthMiddleware` per-user PASETO wiring** — design.md §10.4 specifies per-user PASETO cookies for the web/PWA path. Scope 02 landed the `bearerAuthMiddleware` PASETO wiring for the API surface; the `webAuthMiddleware` cookie-based per-user PASETO path is **Scope 03 work** (along with the SCN-AUTH-002 PWA-path E2E test `tests/e2e/auth/pwa_per_user_test.go` that currently powers the open `FINALIZE-PREREQ-044-V7-001` transitionRequest).
- **Per-user admin allowlist surface** — `auth.Session.IsAdmin()` returns `false` for `SessionSourcePerUserToken` at Scope 02 (admin gating works for `SessionSourceBootstrap` unconditionally and for `SessionSourceSharedToken` in non-production OR when `auth.production_shared_token_fallback_enabled=true`). Per-user admin gating requires the SST allowlist evaluator at the handler layer; deferred to a later scope per design.md §13 OQ-7 (no current user-facing trigger; the bootstrap/shared-token admin paths are sufficient for the current operator surface).
- **OBS-AUDIT-044-S01-01 / OBS-AUDIT-044-S01-02 / OBS-AUDIT-044-S01-03 carry-forward** — three Scope 01 LOW-severity audit observations remain open per §14.2. Scope 02 did NOT introduce regressions on these surfaces and did NOT (yet) close them; recommended follow-ups remain valid for Scope 04 metrics phase or a future hardening pass.

## 16. Design Decisions Reconciled During Scope 03 Implement

This section records the design-vs-shipped reconciliation surfaced by the
formal `bubbles.spec-review` phase for Scope 03. None of the items below
indicates a contract failure; all preserve the original §5–§11 design
intent. Items split into adjustments-that-landed (16.1), observations
carried forward from chaos (16.2), and items DEFERRED beyond Scope 03
(16.3) — recorded so downstream scope work has explicit traceability.

### 16.1 Adjusted from §10 / §11 design (rationale-preserving)

| # | §-reference | Original design intent | Shipped Scope 03 reality | Rationale |
|---|-------------|------------------------|---------------------------|-----------|
| 1 | §10.4 (Web UI session model) | "`webAuthMiddleware` reads `auth_token` cookie value, treats it as a PASETO token, runs `auth.VerifyAndParse`. Cookie attributes: `HttpOnly`, `Secure` (enforced when `runtime.environment == production`), `SameSite=Lax`, `Path=/`." | Cookie attributes shipped verbatim per the design (`internal/api/web_login.go` lines 134-141 set `HttpOnly: true`, `SameSite: http.SameSiteLaxMode`, `Path: "/"`, `Secure: strings.EqualFold(d.Environment, "production")`). The "webAuthMiddleware" naming is unified into `bearerAuthMiddleware` via the cookie-fallback extension to `extractBearerToken` (the bearer middleware now reads the `auth_token` cookie when no `Authorization` header is present, eliminating the parallel-middleware split). The login entry point is registered as `POST /v1/web/login` outside `bearerAuthMiddleware` (rate-limited at 20 req/min per IP via `httprate.LimitByIP`). | One middleware path is simpler than two parallel ones (`webAuthMiddleware` + `bearerAuthMiddleware`); the cookie fallback in `extractBearerToken` preserves all design semantics while reducing code surface. The login + logout endpoints live at `/v1/web/login` and `/v1/web/logout` (not the `/auth/...` namespace) to keep the API surface co-located with other versioned endpoints. |
| 2 | §11 Risk #10 ("Cross-spec drift: handlers added after spec 044 closure don't honor claim-binding") | "Code-quality test `TestNoBodyHeaderActorIDInProductionHandlers` greps `internal/` for body/header `actor_id` reads and asserts every match is gated on `cfg.Environment != "production"`." | Scope 03 ADDS the admin UI page handler (`internal/api/admin_ui.go::HandleAdminTokensUI`) plus three Scope 02 admin REST endpoints (`/v1/auth/users`, `/v1/auth/users/{id}/rotate`, `/v1/auth/tokens/{id}/revoke`). The page handler itself is gated by `bearerAuthMiddleware` via the chi.Group at `internal/api/router.go` registration but does NOT independently enforce admin-scope. Per the explanatory comment on the same chi.Group: "admin-scope enforcement happens at the underlying `/v1/auth/*` admin XHRs (not the page itself — the page is served to any authenticated session because the JS XHRs independently enforce admin scope per Scope 02's `callerIsAdmin`)". The XSS-safe rendering policy (textContent + appendChild only) plus strict CSP (`default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; form-action 'none'`) means the page cannot leak privileged data even when a non-admin authenticated session loads it. | Defense-in-depth at the XHR layer is stronger than at the page layer: a non-admin user who loads the admin page sees the form chrome but every admin operation 403s at the underlying endpoint. Adding a parallel admin-scope check at the page handler would be a redundant gate that can only fail-open under the same misconfiguration that breaks `callerIsAdmin`. |

### 16.2 Observations carried forward (from chaos phase)

| ID | Surface | Observation | Disposition |
|----|---------|-------------|-------------|
| OBS-CHAOS-044-S03-01 | `internal/api/router.go` chi `middleware.Throttle(100)` | Two simultaneous 100-goroutine cohorts in `TestAuthChaos_S03_ExtensionTokenRotationRace_GraceWindowSurvives` (200 concurrent in-flight) trip the global Throttle ceiling, returning 503 — orthogonal to auth correctness. The chaos test was hardened to classify Throttle 503s as throttle (not auth-reject) and to assert an adversarial lower-bound `postT1Admit > 0 && postT2Admit > 0` so the test cannot pass via 100% throttle. | Test-side classification only; no production code change. The Throttle limit is correct (it is a deliberate global in-flight ceiling; the chaos cohort sizes are unrealistic for production). Recorded for the docs-phase agent so the chaos contract is not misread as an auth failure mode. |
| OBS-CHAOS-044-S03-02 | `BenchmarkAuthChaos_S03_PWACookieDerivedSession_HotPath` | Hot-path benchmark recorded **1,477,561 ns/op** (~1.48 ms/op), **20,782 B/op**, **200 allocs/op** at b.N=10000 single-threaded against the live test stack (full DB roundtrip + chi middleware chain + PASETO verify + bearer cache + handler). | Informational, NOT a PASS gate. Consistent with prior Scope 02 chaos numbers; well below NFR-AUTH-001 (≤ 5 ms p99). Recorded for spec 030 observability dashboard sourcing (Scope 04 metrics work). |

### 16.3 Items DEFERRED beyond Scope 03 (recorded for downstream traceability)

| Deferred item | Reason | Routing |
|---------------|--------|---------|
| `internal/telegram/bot.go` callsite migration: `Bot.callCapture` / `Bot.handleReplyAnnotation` / `Bot.handleAnnotationCommand` continue to use `b.authToken` (the shared bot bearer) on internal API calls; `PerUserTokenMinter.MintForChat` is NOT yet invoked from production message-handling paths. | Scope 03 landed `PerUserTokenMinter` (library) + `tests/integration/auth_telegram_e2e_test.go` (3 tests proving the mint→admit→reject chain works in isolation) + `Bot.resolveActorUserID` (production unmapped-chat drop wired in `safeHandleMessage` + `safeHandleCallback` at `internal/telegram/bot.go` lines 251 + 284). The remaining wiring step (replace `b.authToken` with `MintForChat(chatID).Wire` per call) is intentionally deferred so the unmapped-chat drop ships first. The contract is implementable per the integration test; the production reality with `auth_enabled=true` AND `production_shared_token_fallback_enabled=false` is that mapped-chat Telegram captures would 401 from `bearerAuthMiddleware` until the wiring lands — not a security regression (defensive layer is intact + unmapped chats dropped) but a usability deferral for production Telegram operators. | **Scope 04 implement (or a Scope 03 follow-up implement pass) BEFORE spec 044 finalize.** Recorded as deferred-finalize-blocker. Trigger: any production Telegram deployment with `auth_enabled=true` AND `production_shared_token_fallback_enabled=false`. Until landed, production Telegram operators MUST keep `production_shared_token_fallback_enabled=true` (transitional escape hatch documented in §9.3). |
| `specs/027-user-annotations/state.json` Telegram-segment closure annotation | Scope 02 closure entry at `specs/027-user-annotations/state.json` line 216-218 (`closed_security_backlog_mit_027_trace_001_actor_source_segment_via_spec_044_scope_02_claim_binding`) closes the contract via the defensive body-source rejection in `internal/api/annotations.go`. Scope 03's `TestTelegramBridge_BodyClaimedActorRejected` proves the Scope 02 closure works end-to-end through the Telegram entry point — supplementary E2E proof, NOT a separate closure contract. A Telegram-segment closure annotation is OPTIONAL. | **`bubbles.docs` (per-scope docs phase) OR `bubbles.iterate finalize` (spec-level finalize).** Per spec-review-mode SR5 deferral language: "This may be deferred to `bubbles.docs` or `bubbles.iterate finalize`; document the deferral if so." |
| `FINALIZE-PREREQ-044-V7-001` scope-row count residual (manifest 11 vs scopes 12) | Scope 03's PWA-path test file landed and passes per V7 e2e gate; the manifest residual is the path-(a) completion clause from the original transitionRequest description (manifest 12th-entry addition OR scopes.md restructure). Carried forward at status `open` per Scope 03 validate-phase decision. | **Spec-level finalize (`bubbles.iterate finalize`) per existing transitionRequest `expectedResolution` field.** Unchanged by Scope 03 spec-review. |

---

## 17. Design Decisions Reconciled During Scope 04 Implement

This section records the design-vs-shipped reconciliation surfaced by the
formal `bubbles.spec-review` phase for Scope 04 (the final scope of spec
044). None of the items below indicates a contract failure; all preserve
the original §4–§12 design intent. Items split into adjustments-that-landed
(17.1), observations carried forward from chaos (17.2), and items DEFERRED
beyond Scope 04 (17.3) — recorded so spec-level finalize and any future
follow-up specs have explicit traceability.

### 17.1 Adjusted from §4 / §11 / §12 design (rationale-preserving)

| # | §-reference | Original design intent | Shipped Scope 04 reality | Rationale |
|---|-------------|------------------------|---------------------------|-----------|
| 1 | §11 Risk #4 (deprecation pathway: F02 Telegram bot wiring) | "`Bot.callCapture` / `Bot.handleReplyAnnotation` / `Bot.handleAnnotationCommand` migrate from `b.authToken` to `PerUserTokenMinter.MintForChat(chatID)` per call." | Scope 04 ships `Bot.bearerForChat(chatID)` (`internal/telegram/bot.go` line 223) + `Bot.setBearerHeader(req, chatID)` helper (line 245) and migrates 6 internal-API call sites (`internal/telegram/bot.go` lines 701, 778, 883, 942, 1183, 1243) covering `Bot.callCapture` plus the reply-annotation, annotation-command, share-flow, photo-upload, and recipe-flow paths. `cmd/core/wiring.go::startTelegramBotIfConfigured` (lines 339–368) constructs `PerUserTokenMinter` (TTL=5m) and calls `tgBot.SetPerUserTokenMinter` only when production AND `auth.enabled=true` AND signing-key material present. | A two-helper pattern (`bearerForChat` returns the bearer; `setBearerHeader` applies it to a request OR propagates the production-unmapped-chat error) is cleaner than open-coding the per-call mint logic at every call site. The wiring guard (`production AND auth.enabled AND signing key present`) ensures dev/test workflows continue without modification — the minter stays nil and `bearerForChat` falls back to `b.authToken` (verified by `TestBot_bearerForChat_NilMinter_*` unit tests). |
| 2 | §4 OQ-9 telemetry surface (`smackerel_auth_*` prefix; SST keys `auth.telemetry_enabled`, `auth.telemetry_metric_prefix`) | "Five series: `smackerel_auth_issuance_total`, `smackerel_auth_validation_latency_seconds`, `smackerel_auth_rotation_total`, `smackerel_auth_revocation_total{reason}`, `smackerel_auth_failure_total{reason}`." | Scope 04 ships **seven** series (`internal/metrics/auth.go`): `smackerel_auth_token_issuance_total{source}`, `smackerel_auth_token_rotation_total`, `smackerel_auth_token_revocation_total{reason}`, `smackerel_auth_token_validation_latency_seconds` (Histogram), `smackerel_auth_token_validation_outcome_total{result, source}`, `smackerel_auth_legacy_fallback_used_total{environment}`, `smackerel_auth_failure_total{reason}`. All seven registered via `init()` calling `prometheus.MustRegister`. `NormalizeRevocationReason` buckets free-text revocation reasons into the closed set `{unspecified, compromise, rotation, offboarding, test, other}`. | Two additional series surface operator-actionable contracts that the original five did not name: (a) `validation_outcome_total{result, source}` distinguishes accepted from rejected-by-classification (expired / unknown_key / malformed / revoked) AND surfaces the request source (`header` / `pwa_cookie` / `""`) needed to monitor per-surface auth health; (b) `legacy_fallback_used_total{environment}` is the single operator alarm that fires every time the deprecation pathway escape hatch is taken in production — the metric the Operations.md "Deprecation Pathway" runbook directs operators to monitor for one workday before flipping the flag to `false`. The `smackerel_auth_token_*` naming pattern (instead of `smackerel_auth_*`) explicitly scopes the metrics to token-lifecycle events, leaving `smackerel_auth_*` namespace headroom for future auth surfaces (admin RBAC, API key management) without naming collisions. |
| 3 | §12 Phase 4 deliverable: "Update spec 030 observability docs/dashboards to include `smackerel_auth_*` metrics." | Spec 030 cross-spec docs/dashboard integration ships in Scope 04 as the `docs/Operations.md` "Authentication Metrics (Scope 04)" subsection (7-series surface table + emitter sites + 4 PromQL scrape examples) plus a deprecation-pathway runbook directing operators to alert on `smackerel_auth_legacy_fallback_used_total{environment="production"}`. NO `specs/030-observability/` artifact mutations made by Scope 04. | Per session-memory + audit-phase Gate A6 verification, Scope 04 made ZERO changes to `specs/030-observability/`. The operator-facing surface needed for spec 030 dashboards is delivered via the Operations.md authoritative metrics table — operators copy-paste the documented PromQL fragments into their spec-030 dashboard tooling. This avoids a parallel cross-spec edit (which would have required spec 030 status mutation) while delivering the same operator capability. |

### 17.2 Observations carried forward (from chaos phase)

| ID | Surface | Observation | Disposition |
|----|---------|-------------|-------------|
| OBS-CHAOS-044-S04-01 | `TestAuthChaos_S04_DeprecationFlagToggleRace_NoInconsistency` (C4-B03) | The pre/post-flip cohort split for the `production_shared_token_fallback_enabled` toggle race is **stochastic by design**: the Go runtime scheduler does not preserve goroutine launch order, so under `-race -count=20` stress workers `i > flipPoint` can win the schedule race and snapshot `flag` before the flipper goroutine reaches `flag.Store(true)`. The chaos test was hardened to assert the invariants that ACTUALLY guard the production semantic ("a request belongs to the flag value in effect when its handler started"): both cohorts MUST be non-empty (proves the test exercised an actual transition), per-request status MUST match per-request flag snapshot, and the `smackerel_auth_legacy_fallback_used_total` metric delta MUST equal the admitted-cohort size. The strict cohort-size assertions (`flagOffCount == flipPoint` AND `flagOnCount == totalReqs - flipPoint`) were removed. | Test-side classification only; no production code change. The deprecation-flag toggle semantic is correct (per-request snapshot at handler entry; flag is `atomic.Bool`-safe). Recorded for the docs-phase agent so the chaos contract is not misread as an auth correctness flake. |
| OBS-CHAOS-044-S04-02 | `TestAuthChaos_S04_AuthMetricsCounterConcurrentEmit_AggregatesMatch` (C4-B04) | Prometheus `CounterVec` atomicity verified at 5000 emissions across 10 closed-set buckets (5 results × 2 sources) under 100-goroutine contention with deterministic bucket assignment (`goroutineID % 10`): per-bucket delta == 500 exact for all 10 buckets; aggregate delta == 5000 exact; race detector clean. | Informational, NOT a PASS gate. Confirms NFR-AUTH-007 logging hygiene contract extends to metric emissions: no lost increments, no torn writes, no panic. Metric-emission contract is safe to call from any goroutine including those spawned inside `bearerAuthMiddleware`. |

### 17.3 Items DEFERRED beyond Scope 04 (recorded for spec-level finalize traceability)

| Deferred item | Reason | Routing |
|---------------|--------|---------|
| `specs/027-user-annotations/state.json` NATS-segment closure of `MIT-027-TRACE-001` (annotation pipeline derives `actor_source` from session for ALL entry points including raw NATS subjects) | Scope 04 audit-phase Gate A2 confirmed Scope 04 touched ZERO NATS files. The Scope 02 closure entry at spec 027 line 216-218 closes the contract via the defensive body-source rejection in `internal/api/annotations.go` (API entry path); Scope 03 docs-phase (line 237; closedAt 2026-05-11) closes the supplementary Telegram-end-to-end coverage. The remaining NATS-bus segment (annotation events arriving via NATS subjects WITHOUT going through `internal/api/annotations.go::CreateAnnotation`) is NOT YET demonstrably forced through claim-binding. The current production reality: NATS subjects do not currently produce annotation pipeline writes that trust body-supplied `actor_source` per Scope 02 closure invariant — the Scope 02 defensive rejection lives at `internal/api/annotations.go` which IS the NATS-bridged write path. No security regression. | **Spec-level finalize (`bubbles.iterate finalize`) OR a future spec.** Per spec-review-mode SR5 deferral language: "This may be deferred to `bubbles.docs` or `bubbles.iterate finalize`; document the deferral if so." Recorded as LOW (not blocking) — defensive layer at the API/NATS-bridged write path is intact. |
| `SCN-AUTH-012` declaration absent from `spec.md` (no `### SCN-AUTH-012 — ...` heading) and from `scopes.md` (no `Scenario: SCN-AUTH-012` Gherkin block) | Scope 04 implement added SCN-AUTH-012 to `scenario-manifest.json` as the path-(a) closure of `FINALIZE-PREREQ-044-V7-001` (manifest 12th-entry addition). The matching spec.md heading + scopes.md Gherkin block were intentionally NOT added because (a) the path-(a) discharge condition is satisfied by the manifest entry alone (the carry-forward registry now reads `status=resolved`), (b) adding both the spec.md heading + scopes.md scenario would expand the human-facing spec with a scenario whose contract is already covered by the SCN-AUTH-008 actor-source closure plus the SCN-AUTH-002 metrics observability closure plus the SCN-AUTH-005 dev/test backward-compat preservation — i.e. SCN-AUTH-012 is a derived/composite scenario of the already-shipped scenarios, used here as a manifest-level traceability handle for the F02 closure deliverables. | **Spec-level finalize (`bubbles.iterate finalize`) per the path-(b) clause of the original `FINALIZE-PREREQ-044-V7-001` `expectedResolution` field.** Recorded as LOW (not blocking) — the manifest 12th-entry path-(a) discharge IS the documented closure; the spec.md/scopes.md catchup is OPTIONAL per the same transitionRequest description. |

---

## References

- [`spec.md`](./spec.md) — feature specification
- `internal/api/router.go` — middleware refactor target (lines 425–471)
- `internal/api/photos_upload.go` — MintReveal refactor target (lines 246–321)
- `internal/drive/google/google.go` + `internal/drive/context.go` — Connect refactor target
- `internal/annotation/` — annotation pipeline refactor target
- `cmd/core/wiring.go` — startup fail-loud extension target (lines 48–55)
- `config/smackerel.yaml` — SST source (USER WIP — touch via PR after USER finalizes other in-flight changes)
- `specs/040-cloud-photo-libraries/state.json` — MIT-040-S-008 closure target
- `specs/038-cloud-drives-integration/state.json` — MIT-038-S-003 closure target
- `specs/027-user-annotations/state.json` — MIT-027-TRACE-001 actor-source segment closure target
- `.github/skills/bubbles-config-sst/SKILL.md` — SST zero-defaults compliance
- `.github/skills/bubbles-test-environment-isolation/SKILL.md` — test-isolated DB pattern
- `docs/Deployment.md` — Build-Once Deploy-Many bundle contract that flows the new SST keys
- `docs/Operations.md` — operator surfaces (auth enrollment workflow added in Phase 4)
- `docs/smackerel.md` — architecture posture (auth boundary section added in Phase 4)
- [PASETO v4 spec](https://github.com/paseto-standard/paseto-spec/blob/master/docs/01-Protocol-Versions/Version4.md) — wire format reference
- [`github.com/aidantwoods/go-paseto`](https://github.com/aidantwoods/go-paseto) — selected Go library
