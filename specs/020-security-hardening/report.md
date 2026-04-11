# Report: 020 Security Hardening — Docker Binding, Auth Enforcement, Crypto Hygiene

**Feature:** 020-security-hardening
**Created:** 2026-04-10
**Last Reconciled:** 2026-04-10

---

## Summary

| Scope | Name | Status | Evidence |
|-------|------|--------|----------|
| 1 | Docker Port Binding + NATS Config File | Done | All 6 services use `127.0.0.1:` binding. NATS uses config file. |
| 2 | ML Sidecar Auth + Web UI Auth + OAuth Rate Limiting | Done | `ml/app/auth.py` with `hmac.compare_digest`. `webAuthMiddleware` applied. `httprate` on OAuth start. |
| 3 | Decrypt Fail-Closed + Startup Auth Warning | Done | `decrypt()` returns error on 3 failure paths. Startup WARN in core + ML. |

## Reconciliation Evidence (validate trigger — 2026-04-10)

### Scope 1: Docker Port Binding + NATS Config File

**Code-verified claims:**

| Claim | File | Line Evidence | Verified |
|-------|------|---------------|----------|
| postgres ports `127.0.0.1` | `docker-compose.yml` | `"127.0.0.1:${POSTGRES_HOST_PORT}:${POSTGRES_CONTAINER_PORT}"` | ✅ |
| nats client port `127.0.0.1` | `docker-compose.yml` | `"127.0.0.1:${NATS_CLIENT_HOST_PORT}:${NATS_CLIENT_PORT}"` | ✅ |
| nats monitor port `127.0.0.1` | `docker-compose.yml` | `"127.0.0.1:${NATS_MONITOR_HOST_PORT}:${NATS_MONITOR_PORT}"` | ✅ |
| smackerel-core port `127.0.0.1` | `docker-compose.yml` | `"127.0.0.1:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"` | ✅ |
| smackerel-ml port `127.0.0.1` | `docker-compose.yml` | `"127.0.0.1:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"` | ✅ |
| ollama port `127.0.0.1` | `docker-compose.yml` | `"127.0.0.1:${OLLAMA_HOST_PORT}:${OLLAMA_CONTAINER_PORT}"` | ✅ |
| NATS uses `--config` not `--auth` | `docker-compose.yml` | `command: ["--config", "/etc/nats/nats.conf"]` | ✅ |
| NATS config file mounted read-only | `docker-compose.yml` | `./config/generated/nats.conf:/etc/nats/nats.conf:ro` | ✅ |
| `host_bind_address` in SST | `config/smackerel.yaml` | `host_bind_address: "127.0.0.1"` under `runtime:` | ✅ |
| Config generator reads `host_bind_address` | `scripts/commands/config.sh` | `HOST_BIND_ADDRESS="$(required_value runtime.host_bind_address)"` | ✅ |
| Config generator produces `nats.conf` | `scripts/commands/config.sh` | Writes `jetstream`, `http_port`, `authorization { token: }` | ✅ |
| `nats.conf` not committed (gitignored) | `config/generated/` | Directory is in `.gitignore` | ✅ |

### Scope 2: ML Sidecar Auth + Web UI Auth + OAuth Rate Limiting

**Code-verified claims:**

| Claim | File | Line Evidence | Verified |
|-------|------|---------------|----------|
| `verify_auth()` exists with `hmac.compare_digest` | `ml/app/auth.py` | `hmac.compare_digest(token, _AUTH_TOKEN)` | ✅ |
| Accepts `Authorization: Bearer` | `ml/app/auth.py` | `if auth_header.lower().startswith("bearer "):` | ✅ |
| Accepts `X-Auth-Token` header | `ml/app/auth.py` | `token = request.headers.get("x-auth-token")` | ✅ |
| Dev mode passthrough (empty token) | `ml/app/auth.py` | `if not _AUTH_TOKEN: return` | ✅ |
| `authed_router` with `Depends(verify_auth)` | `ml/app/main.py` | `authed_router = APIRouter(dependencies=[Depends(verify_auth)])` | ✅ |
| `/health` is NOT on authed router | `ml/app/main.py` | `@app.get("/health")` registered on `app`, not `authed_router` | ✅ |
| `webAuthMiddleware` applied to Web UI group | `internal/api/router.go` | `r.Use(deps.webAuthMiddleware)` in Web UI group | ✅ |
| `webAuthMiddleware` uses constant-time compare | `internal/api/router.go` | `subtle.ConstantTimeCompare([]byte(parts[1]), []byte(d.AuthToken))` | ✅ |
| `webAuthMiddleware` checks cookie | `internal/api/router.go` | `r.Cookie("auth_token")` fallback | ✅ |
| `webAuthMiddleware` dev mode passthrough | `internal/api/router.go` | `if d.AuthToken == "" { next.ServeHTTP(w, r); return }` | ✅ |
| `httprate.LimitByIP(10, 1*time.Minute)` on OAuth start | `internal/api/router.go` | Applied to OAuth start group | ✅ |
| OAuth callback NOT rate-limited | `internal/api/router.go` | `r.Get("/auth/{provider}/callback", ...)` registered outside rate-limited group | ✅ |
| `go-chi/httprate` in `go.mod` | `go.mod` | `github.com/go-chi/httprate v0.15.0 // indirect` | ✅ |
| ML sidecar auth tests (7 tests) | `ml/tests/test_auth.py` | `TestMLSidecarAuthWithToken` (5 tests), `TestMLSidecarAuthDevMode` (2 tests) | ✅ |

### Scope 3: Decrypt Fail-Closed + Startup Auth Warning

**Code-verified claims:**

| Claim | File | Line Evidence | Verified |
|-------|------|---------------|----------|
| Path 1 (not base64): returns error | `internal/auth/store.go` | `return "", fmt.Errorf("token is not valid base64: %w", err)` | ✅ |
| Path 2 (too short): returns error | `internal/auth/store.go` | `return "", fmt.Errorf("encrypted token data too short ...")` | ✅ |
| Path 3 (GCM failure): returns error | `internal/auth/store.go` | `return "", fmt.Errorf("token decryption failed: %w", err)` | ✅ |
| No-key passthrough preserved | `internal/auth/store.go` | `if len(s.encKey) == 0 { return encoded, nil }` | ✅ |
| `Get()` propagates decrypt errors | `internal/auth/store.go` | `return nil, fmt.Errorf("decrypt access token for %s: %w", ...)` | ✅ |
| Core startup WARN on empty token | `cmd/core/main.go` | `slog.Warn("SMACKEREL_AUTH_TOKEN is empty — system running without authentication")` | ✅ |
| ML startup WARNING on empty token | `ml/app/main.py` | `logger.warning("SMACKEREL_AUTH_TOKEN is empty — ML sidecar running without authentication")` | ✅ |
| Fail-closed unit tests (3 tests) | `internal/auth/oauth_test.go` | `TestTokenStore_Decrypt_FailClosed_NotBase64`, `_TooShort`, `_GCMFailure` | ✅ |
| Plaintext passthrough test | `internal/auth/oauth_test.go` | `TestTokenStore_Decrypt_NoKey_PlaintextPassthrough` | ✅ |

## Test Evidence

| Command | Result | Timestamp |
|---------|--------|-----------|
| `./smackerel.sh check` | PASS — config in sync with SST | 2026-04-10 |
| `./smackerel.sh test unit` | PASS — 51 tests (Go 31 packages + Python 51 pytest) | 2026-04-10 |
| `./smackerel.sh test unit` | PASS — 53 tests (Go 31 packages + Python 53 pytest) | 2026-04-11 |

## Test Coverage Gaps (Resolved)

All 3 test coverage gaps identified in the 2026-04-10 reconciliation have been closed:

| Gap | Scenario(s) | Resolution | Evidence |
|-----|-------------|------------|----------|
| ~~No Web UI auth unit test~~ | SCN-020-009, SCN-020-010 | Created `internal/api/router_test.go` with 6 subtests: auth required on all 6 Web UI routes, Bearer token accepted, cookie accepted, wrong token rejected, dev mode passthrough on all 6 routes | `TestWebUI_RequiresAuth_WhenTokenConfigured`, `TestWebUI_AcceptsBearerToken`, `TestWebUI_AcceptsCookie`, `TestWebUI_RejectsWrongToken`, `TestWebUI_AllowsAll_WhenTokenEmpty` |
| ~~No OAuth rate limit unit test~~ | SCN-020-011, SCN-020-012 | Created rate limit tests in `internal/api/router_test.go`: fires 15 requests and asserts 429, fires 5 within limit and asserts all 200, verifies callback is NOT rate-limited | `TestOAuthStart_RateLimited`, `TestOAuthStart_AllowsWithinLimit`, `TestOAuthCallback_NotRateLimited` |
| ~~No ML startup warning test~~ | SCN-020-017, SCN-020-018 | Created `ml/tests/test_startup_warning.py` with 2 tests: lifespan emits WARNING when token empty, no warning when token set | `TestMLStartupWarningEmptyToken`, `TestMLStartupNoWarningWithToken` |

## Completion Statement

All 3 scopes implemented and code-verified. All security controls are correctly in place. All 3 test coverage gaps from prior reconciliation have been closed with targeted unit tests. 53 unit tests pass (Go 31 packages all green + Python 53 pytest all green).
