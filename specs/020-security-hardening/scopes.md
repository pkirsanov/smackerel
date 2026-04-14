# Scopes: 020 Security Hardening — Docker Binding, Auth Enforcement, Crypto Hygiene

**Feature:** 020-security-hardening
**Created:** 2026-04-10
**Status:** Done

---

## Execution Outline

### Phase Order

1. **Scope 1: Docker Port Binding + NATS Config File** — Bind all Docker host-forwarded ports to `127.0.0.1`, replace NATS `--auth` CLI arg with config file mount, add `host_bind_address` to SST pipeline.
2. **Scope 2: ML Sidecar Auth + Web UI Auth + OAuth Rate Limiting** — Add FastAPI auth dependency to ML sidecar, apply `webAuthMiddleware` to Web UI routes, add `httprate` rate limiter to OAuth start endpoint.
3. **Scope 3: Decrypt Fail-Closed + Startup Auth Warning** — Remove 3 silent plaintext fallback paths in `decrypt()`, add startup WARN log when `auth_token` is empty in both core and ML sidecar.

### New Types & Signatures

- `ml/app/auth.py`: `verify_auth(request: Request) -> None` — FastAPI dependency
- `internal/api/router.go`: `httprate.LimitByIP(10, 1*time.Minute)` middleware on OAuth start
- `internal/auth/store.go`: `decrypt()` signature unchanged but error paths changed from `(rawText, nil)` to `("", error)`
- `config/smackerel.yaml`: `runtime.host_bind_address: "127.0.0.1"` field added
- `config/generated/nats.conf`: New generated file with NATS auth config

### Validation Checkpoints

- After Scope 1: All port mappings in `docker-compose.yml` use `127.0.0.1:` prefix, NATS uses config file mount, `./smackerel.sh config generate` produces `nats.conf`.
- After Scope 2: ML sidecar rejects unauthenticated non-health requests when token configured, Web UI requires auth when token configured, OAuth start rate-limited. `./smackerel.sh test unit` passes.
- After Scope 3: `decrypt()` returns error on failure when key present, startup logs warn on empty auth_token. All unit tests pass.

---

## Scope Summary

| # | Name | Surfaces | Tests | DoD Summary | Status |
|---|------|----------|-------|-------------|--------|
| 1 | Docker Port Binding + NATS Config File | Docker Compose, Config SST, Config generator | Unit, Integration, E2E-API | All ports 127.0.0.1, NATS token hidden from `docker ps` | Done |
| 2 | ML Sidecar Auth + Web UI Auth + OAuth Rate Limiting | Python ML sidecar, Go core router | Unit, Integration, E2E-API | All non-health HTTP surfaces require auth when configured | Done |
| 3 | Decrypt Fail-Closed + Startup Auth Warning | Go auth store, Go core startup, Python ML startup | Unit | No silent plaintext fallback, clear startup warnings | Done |

---

## Scope 1: Docker Port Binding + NATS Config File

**Status:** [x] Done

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-020-001 All Docker host-forwarded ports bind to 127.0.0.1
  Given the Smackerel stack is deployed via Docker Compose
  When a device on the same LAN attempts to connect to any service host port
  Then the connection is refused because all host-forwarded ports bind to 127.0.0.1

Scenario: SCN-020-002 Config generation produces localhost-bound port mappings
  Given config/smackerel.yaml defines runtime.host_bind_address as "127.0.0.1"
  When ./smackerel.sh config generate runs
  Then HOST_BIND_ADDRESS is written to the generated env file

Scenario: SCN-020-003 NATS auth token is not visible in docker ps
  Given the Smackerel stack is running
  When an operator runs docker ps to inspect running containers
  Then the NATS auth token does not appear in the command column

Scenario: SCN-020-004 NATS uses config file for authentication
  Given config/smackerel.yaml defines auth_token
  When ./smackerel.sh config generate runs
  Then config/generated/nats.conf is created with the auth token
  And docker-compose.yml mounts the config file and references it via --config flag
```

### Implementation Plan

| File | Change |
|------|--------|
| `docker-compose.yml` | Prefix all `ports:` entries with `127.0.0.1:` (postgres, nats ×2, smackerel-ml, ollama — core already correct). Replace NATS `command` with `--config /etc/nats/nats.conf`. Add volume mount `./config/generated/nats.conf:/etc/nats/nats.conf:ro`. |
| `config/smackerel.yaml` | Add `runtime.host_bind_address: "127.0.0.1"` |
| `scripts/commands/config.sh` | Read `host_bind_address` via `required_value`. Generate `config/generated/nats.conf` with resolved auth token and monitor port. Write `HOST_BIND_ADDRESS` to env file. |

**NATS config file format (generated):**
```
jetstream { store_dir: /data }
http_port: <resolved-monitor-port>
authorization { token: "<resolved-auth-token>" }
```

**Security:** `config/generated/` is `.gitignore`d — `nats.conf` with resolved token is never committed.

### Test Plan

| Type | File/Location | Purpose | Scenarios Covered |
|------|---------------|---------|-------------------|
| Unit | `scripts/commands/config_test.sh` | Config generate produces `nats.conf` and `HOST_BIND_ADDRESS` in env | SCN-020-002, SCN-020-004 |
| Integration | `tests/integration/docker_ports_test.go` | Verify `docker-compose.yml` port mappings all contain `127.0.0.1:` prefix | SCN-020-001 |
| E2E-API | `tests/e2e/port_binding_test.go` | On running stack, verify `ss -tlnp` shows all ports bound to 127.0.0.1 | SCN-020-001 |
| E2E-API | `tests/e2e/nats_token_hidden_test.go` | On running stack, verify `docker ps` command column for NATS does not contain auth token | SCN-020-003 |
| Regression | `./smackerel.sh test unit` | All existing tests pass | SCN-020-001 through SCN-020-004 |

### Definition of Done

- [x] All `ports:` entries in `docker-compose.yml` use `127.0.0.1:` prefix (5 services verified)
- [x] `smackerel-core` port entry remains unchanged (already correct)
- [x] `config/smackerel.yaml` contains `runtime.host_bind_address: "127.0.0.1"`
- [x] `scripts/commands/config.sh` reads `host_bind_address` and writes `HOST_BIND_ADDRESS` to env
- [x] `scripts/commands/config.sh` generates `config/generated/nats.conf` with resolved token
- [x] `docker-compose.yml` NATS service uses `--config /etc/nats/nats.conf` instead of `--auth`
- [x] `docker-compose.yml` NATS service mounts `./config/generated/nats.conf:/etc/nats/nats.conf:ro`
- [x] NATS auth token is NOT visible in `docker ps` output
- [x] `./smackerel.sh config generate` completes without error
- [x] `./smackerel.sh test unit` passes
- [x] Inter-container networking is unchanged (containers communicate on Docker bridge)

---

## Scope 2: ML Sidecar Auth + Web UI Auth + OAuth Rate Limiting

**Status:** [x] Done

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-020-005 ML sidecar rejects unauthenticated requests when token configured
  Given the ML sidecar is running with SMACKEREL_AUTH_TOKEN="test-secret"
  When a request arrives at a non-health endpoint without auth header
  Then the ML sidecar returns 401 Unauthorized with body {"detail": "Unauthorized"}

Scenario: SCN-020-006 ML sidecar accepts authenticated requests
  Given the ML sidecar is running with SMACKEREL_AUTH_TOKEN="test-secret"
  When a request arrives with Authorization: Bearer test-secret
  Then the ML sidecar processes the request normally

Scenario: SCN-020-007 ML sidecar health endpoint remains unauthenticated
  Given the ML sidecar is running with SMACKEREL_AUTH_TOKEN configured
  When a GET /health request arrives without any auth header
  Then the ML sidecar returns 200 with health status

Scenario: SCN-020-008 ML sidecar allows all requests when auth_token is empty
  Given the ML sidecar is running with SMACKEREL_AUTH_TOKEN=""
  When a request arrives at any endpoint without auth
  Then the ML sidecar processes the request normally (dev mode)

Scenario: SCN-020-009 Web UI requires auth when auth_token is configured
  Given smackerel-core is running with a non-empty auth_token
  When a browser request arrives at a Web UI route without auth credentials
  Then the server returns 401 Unauthorized

Scenario: SCN-020-010 Web UI allows all requests when auth_token is empty
  Given smackerel-core is running with an empty auth_token
  When a browser request arrives at any Web UI route without auth
  Then the server serves the page normally (dev mode)

Scenario: SCN-020-011 OAuth start endpoint is rate-limited
  Given smackerel-core is running with OAuth providers configured
  When more than 10 requests arrive at /auth/{provider}/start from the same IP within 1 minute
  Then subsequent requests receive 429 Too Many Requests

Scenario: SCN-020-012 OAuth start endpoint allows traffic within rate limit
  Given smackerel-core is running with OAuth providers configured
  When 5 requests arrive at /auth/{provider}/start from the same IP within 1 minute
  Then all 5 requests are processed normally
```

### Implementation Plan

| File | Change |
|------|--------|
| `ml/app/auth.py` | New file: `verify_auth()` FastAPI dependency using `hmac.compare_digest` for constant-time token comparison |
| `ml/app/main.py` | Create `authed_router = APIRouter(dependencies=[Depends(verify_auth)])`, move non-health endpoints to it, `app.include_router(authed_router)` |
| `internal/api/router.go` | Insert `r.Use(deps.webAuthMiddleware)` in Web UI route group. Add `import "github.com/go-chi/httprate"`. Wrap OAuth start endpoint with `httprate.LimitByIP(10, 1*time.Minute)`. |
| `go.mod` | Add `github.com/go-chi/httprate` dependency |

**ML sidecar auth contract:**
- `Authorization: Bearer <token>` or `X-Auth-Token: <token>` header
- `GET /health` always unauthenticated (Docker healthcheck)
- Empty `SMACKEREL_AUTH_TOKEN` → all requests pass (dev mode)
- Token comparison: `hmac.compare_digest()` (constant-time)

**Web UI auth:** Existing `webAuthMiddleware` in `router.go` already implements correct behavior — just needs to be applied to the Web UI group with `r.Use(deps.webAuthMiddleware)`.

**OAuth rate limiting:** `httprate.LimitByIP(10, 1*time.Minute)` on `/auth/{provider}/start` group only. Callback stays unprotected (browser redirect, low abuse vector).

### Test Plan

| Type | File/Location | Purpose | Scenarios Covered |
|------|---------------|---------|-------------------|
| Unit (Python) | `ml/tests/test_auth.py` | ML sidecar auth: reject without token, accept with valid token, health unauthenticated, dev mode passthrough | SCN-020-005, SCN-020-006, SCN-020-007, SCN-020-008 |
| Unit (Go) | `internal/api/router_test.go` | Web UI auth enforcement when token configured, passthrough when empty | SCN-020-009, SCN-020-010 |
| Unit (Go) | `internal/auth/oauth_test.go` | OAuth rate limiting: >10 req/min blocked, ≤10 allowed | SCN-020-011, SCN-020-012 |
| Integration | `tests/integration/ml_auth_test.go` | Live ML sidecar: authenticated access, rejected unauthenticated | SCN-020-005, SCN-020-006 |
| E2E-API | `tests/e2e/auth_enforcement_test.go` | Full stack: Web UI returns 401 without auth, ML sidecar returns 401 without auth, health endpoints remain open | SCN-020-005, SCN-020-007, SCN-020-009 |
| Regression | `./smackerel.sh test unit` | All existing tests pass | All |

### Definition of Done

- [x] `ml/app/auth.py` created with `verify_auth()` dependency using `hmac.compare_digest`
- [x] ML sidecar non-health endpoints require auth when `SMACKEREL_AUTH_TOKEN` is set
- [x] ML sidecar `/health` remains unauthenticated (Docker healthcheck works)
- [x] ML sidecar dev mode (empty token) allows all requests
- [x] `webAuthMiddleware` applied to Web UI route group in `internal/api/router.go`
- [x] Web UI requires auth when `auth_token` configured, passthrough when empty
- [x] `httprate.LimitByIP(10, 1*time.Minute)` applied to OAuth start endpoint
- [x] OAuth callback is rate-limited alongside start (SEC-SWEEP-001)
- [x] `go.mod` includes `github.com/go-chi/httprate`
- [x] Auth failures logged at WARN level with request path and IP (no token values in logs)
- [x] `./smackerel.sh test unit` passes (Go + Python)
- [x] `./smackerel.sh test integration` passes

---

## Scope 3: Decrypt Fail-Closed + Startup Auth Warning

**Status:** [x] Done

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-020-013 Decryption fails closed when encryption key is configured
  Given an encryption key is derived from a non-empty auth_token
  When decrypt() is called on data that cannot be decrypted
  Then an error is returned instead of the raw ciphertext
  And the token is NOT silently treated as plaintext

Scenario: SCN-020-014 No encryption key means plaintext passthrough
  Given auth_token is empty (no encryption key derived)
  When decrypt() is called on any stored value
  Then the value is returned as-is (plaintext passthrough for dev mode)

Scenario: SCN-020-015 Valid encrypted data decrypts successfully
  Given an encryption key is derived from a non-empty auth_token
  When decrypt() is called on properly encrypted data
  Then the original plaintext is returned

Scenario: SCN-020-016 Startup emits warning when auth_token is empty (core)
  Given auth_token is empty in config/smackerel.yaml
  When smackerel-core starts
  Then the startup log emits a WARN-level message: "SMACKEREL_AUTH_TOKEN is empty — system running without authentication"

Scenario: SCN-020-017 ML sidecar emits warning when auth_token is empty
  Given SMACKEREL_AUTH_TOKEN is empty
  When the ML sidecar starts
  Then the startup log emits a WARNING-level message about running without authentication

Scenario: SCN-020-018 No warning when auth_token is configured
  Given auth_token is set to a non-empty value
  When smackerel-core starts
  Then no auth warning is emitted at startup
```

### Implementation Plan

| File | Change |
|------|--------|
| `internal/auth/store.go` | Replace 3 silent plaintext fallback paths in `decrypt()` with error returns when `s.encKey` is non-nil. Path 1 (not base64): return `("", fmt.Errorf("token is not valid base64: %w", err))`. Path 2 (too short): return `("", fmt.Errorf("encrypted token data too short ..."))`. Path 3 (GCM Open failure): return `("", fmt.Errorf("token decryption failed: %w", err))`. When `s.encKey` is nil: no change (plaintext passthrough). |
| `cmd/core/main.go` | After config load, before server start: `if cfg.AuthToken == "" { slog.Warn("SMACKEREL_AUTH_TOKEN is empty — system running without authentication...") }` |
| `ml/app/main.py` | In lifespan function: `if not auth_token: logger.warning("SMACKEREL_AUTH_TOKEN is empty — ML sidecar running without authentication...")` |

**Migration impact:** Tokens stored as plaintext before encryption was enabled will fail to decrypt once `auth_token` is set. This is intentional — operator must re-authorize OAuth providers after enabling `auth_token`.

### Test Plan

| Type | File/Location | Purpose | Scenarios Covered |
|------|---------------|---------|-------------------|
| Unit (Go) | `internal/auth/store_test.go` | `decrypt()` returns error on invalid base64 when key present | SCN-020-013 |
| Unit (Go) | `internal/auth/store_test.go` | `decrypt()` returns error on too-short data when key present | SCN-020-013 |
| Unit (Go) | `internal/auth/store_test.go` | `decrypt()` returns error on GCM failure when key present | SCN-020-013 |
| Unit (Go) | `internal/auth/store_test.go` | `decrypt()` returns plaintext when no encryption key | SCN-020-014 |
| Unit (Go) | `internal/auth/store_test.go` | `decrypt()` succeeds on valid encrypted data | SCN-020-015 |
| Unit (Go) | `cmd/core/main_test.go` | Startup logs WARN when auth_token empty | SCN-020-016, SCN-020-018 |
| Unit (Python) | `ml/tests/test_startup_warning.py` | ML sidecar logs WARNING when token empty, no warning when set | SCN-020-017, SCN-020-018 |
| E2E-API | `tests/e2e/decrypt_failclosed_test.go` | With auth_token set, corrupted stored token returns error, not plaintext | SCN-020-013, SCN-020-015 |
| Regression | `./smackerel.sh test unit` | All existing tests pass including auth store tests | All |

### Definition of Done

- [x] `decrypt()` returns `("", error)` on all 3 failure paths when `encKey` is non-nil
- [x] `decrypt()` still returns `(encoded, nil)` when `encKey` is nil (dev mode passthrough)
- [x] Valid encrypted data still decrypts correctly
- [x] Callers of `decrypt()` (e.g., `Get()`) already propagate errors — verify no caller swallows the new error
- [x] `cmd/core/main.go` logs WARN on empty `auth_token` at startup
- [x] `ml/app/main.py` logs WARNING on empty `SMACKEREL_AUTH_TOKEN` at startup
- [x] No warning emitted when `auth_token` is configured (non-empty)
- [x] `./smackerel.sh test unit` passes (Go + Python)
- [x] No secrets (token values) appear in any log messages
