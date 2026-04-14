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
| `./smackerel.sh test unit` | PASS — 53 tests (Go 33 packages + Python 53 pytest) | 2026-04-12 (security sweep) |

## Security Sweep (2026-04-12) — Stochastic Quality Trigger

### Sweep Methodology

Full codebase security review covering:
- Dependency scanning (Go go.mod, Python requirements.txt)
- Code review for OWASP Top 10: injection, XSS, SSRF, auth bypass, CSRF, crypto weaknesses
- Docker security hardening (CIS Docker benchmark)
- Threat modeling of all HTTP attack surfaces

### Findings

| ID | Severity | Description | Status |
|----|----------|-------------|--------|
| SEC-SWEEP-001 | Medium | OAuth callback endpoint (`/auth/{provider}/callback`) lacked rate limiting — allows DoS via log flooding and state-map probing | **Fixed** |
| SEC-SWEEP-002 | Low | Application containers (smackerel-core, smackerel-ml, nats) did not drop Linux capabilities | **Fixed** |

### Verified Non-Findings (Negative Evidence)

| Category | Finding | Evidence |
|----------|---------|----------|
| SQL Injection | None — all queries use pgx parameterized `$N` placeholders | Grep for `Sprintf.*WHERE/SELECT/INSERT` returns 0 matches |
| XSS | None — Go `html/template` auto-escapes, `html.EscapeString` used in dynamic HTML | All templates use `html/template`; `safeURL` only allows http/https |
| SSRF | None — ML sidecar and Ollama URLs are from config, not user input | `MLSidecarURL` and `OllamaURL` from env vars only |
| Command Injection | None — no `os/exec` or `subprocess` usage | Zero matches for exec.Command or subprocess |
| Path Traversal | Already mitigated — bookmarks/twitter connectors validate paths with `filepath.Abs`, `EvalSymlinks`, symlink guards, and boundary checks | Tests cover symlink, traversal, and TOCTOU scenarios |
| Auth Bypass | None — Bearer + cookie auth with constant-time compare, dev passthrough only when token empty | `subtle.ConstantTimeCompare` and `hmac.compare_digest` |
| CSRF | Mitigated — crypto/rand state token, 100-entry cap, 10m TTL | `generateState()` uses `crypto/rand` |
| Security Headers | All OWASP-recommended headers present — CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy, Permissions-Policy | `securityHeadersMiddleware` sets all 5 |
| Docker Root | Core + ML run as non-root (`USER smackerel`), `no-new-privileges` on all 5 services | Dockerfiles + docker-compose.yml verified |
| Secrets in CLI | NATS auth token not in CLI args — uses config file mount | `--config /etc/nats/nats.conf` verified |
| Crypto | AES-256-GCM with random nonce, SHA-256 key derivation, fail-closed decrypt | `internal/auth/store.go` verified |

### Fixes Implemented

**SEC-SWEEP-001: OAuth callback rate limiting**
- File: [internal/api/router.go](internal/api/router.go) — moved callback into rate-limited group (10 req/min/IP)
- Test: [internal/api/router_test.go](internal/api/router_test.go) — `TestOAuthCallback_RateLimited` replaces `TestOAuthCallback_NotRateLimited`
- Rationale: Defense in depth against callback abuse; CSRF state validation provides primary protection, rate limiting prevents log flooding

**SEC-SWEEP-002: Docker container capability dropping**
- File: [docker-compose.yml](docker-compose.yml) — added `cap_drop: [ALL]` to smackerel-core, smackerel-ml, nats
- Test: [internal/config/docker_security_test.go](internal/config/docker_security_test.go) — `TestDockerCompose_CapDropAll`
- Rationale: CIS Docker benchmark recommendation; limits blast radius on container compromise. Postgres and Ollama excluded (postgres needs init capabilities, Ollama needs GPU access)

All 3 test coverage gaps identified in the 2026-04-10 reconciliation have been closed:

| Gap | Scenario(s) | Resolution | Evidence |
|-----|-------------|------------|----------|
| ~~No Web UI auth unit test~~ | SCN-020-009, SCN-020-010 | Created `internal/api/router_test.go` with 6 subtests: auth required on all 6 Web UI routes, Bearer token accepted, cookie accepted, wrong token rejected, dev mode passthrough on all 6 routes | `TestWebUI_RequiresAuth_WhenTokenConfigured`, `TestWebUI_AcceptsBearerToken`, `TestWebUI_AcceptsCookie`, `TestWebUI_RejectsWrongToken`, `TestWebUI_AllowsAll_WhenTokenEmpty` |
| ~~No OAuth rate limit unit test~~ | SCN-020-011, SCN-020-012 | Created rate limit tests in `internal/api/router_test.go`: fires 15 requests and asserts 429, fires 5 within limit and asserts all 200, verifies callback is NOT rate-limited | `TestOAuthStart_RateLimited`, `TestOAuthStart_AllowsWithinLimit`, `TestOAuthCallback_NotRateLimited` |
| ~~No ML startup warning test~~ | SCN-020-017, SCN-020-018 | Created `ml/tests/test_startup_warning.py` with 2 tests: lifespan emits WARNING when token empty, no warning when token set | `TestMLStartupWarningEmptyToken`, `TestMLStartupNoWarningWithToken` |

## Completion Statement

All 3 scopes implemented and code-verified. All security controls are correctly in place. All 3 test coverage gaps from prior reconciliation have been closed with targeted unit tests. 53 unit tests pass (Go 31 packages all green + Python 53 pytest all green).

## Gaps Probe (2026-04-14) — Stochastic Quality Sweep R30

### Sweep Methodology

Full gap analysis of spec 020-security-hardening implementation covering:
- NATS config generation pipeline for special character handling
- ML sidecar auth middleware edge cases
- Artifact integrity verification (DoD claims vs actual code)

### Findings

| ID | Severity | CWE | Description | Status |
|----|----------|-----|-------------|--------|
| GAP-020-R30-001 | Medium | CWE-74 | NATS config token not escaped for `"` and `\` — embedding special chars in auth token corrupts nats.conf or silently disables NATS authentication | **Fixed** |
| GAP-020-R30-002 | Low | CWE-755 | ML sidecar `hmac.compare_digest` raises `TypeError` on non-ASCII token strings — attacker sends non-ASCII `Authorization: Bearer` header, gets 500 instead of 401 | **Fixed** |
| GAP-020-R30-003 | Low | N/A | Scope 2 DoD "OAuth callback is NOT rate-limited" is wrong — code rate-limits both start + callback since SEC-SWEEP-001; artifact integrity mismatch | **Fixed** |

### Fixes Implemented

**GAP-020-R30-001: NATS config token escaping (CWE-74)**
- File: [scripts/commands/config.sh](scripts/commands/config.sh) — escape `\` → `\\` and `"` → `\"` in token before interpolation into nats.conf
- Tests: [internal/config/docker_security_test.go](internal/config/docker_security_test.go) — `TestNATSConfGenerator_EscapesSpecialCharsInToken` (verifies escape substitutions exist and are ordered correctly), `TestNATSConf_GeneratedFile_TokenProperlyQuoted` (validates generated nats.conf has no unescaped double-quotes inside the token value)
- Rationale: R07-actual (IMP-020-002) added double-quoting but omitted intra-value escaping. A token containing `"` terminates the NATS string early; `\` is interpreted as escape prefix. Both can silently disable NATS auth or corrupt the effective token value.

**GAP-020-R30-002: ML sidecar non-ASCII auth token handling (CWE-755)**
- File: [ml/app/auth.py](ml/app/auth.py) — wrap `hmac.compare_digest` in try/except TypeError, treat as auth failure
- Tests: [ml/tests/test_auth.py](ml/tests/test_auth.py) — `TestMLSidecarAuthAdversarial.test_non_ascii_bearer_returns_401`, `test_non_ascii_x_auth_token_returns_401`, `test_empty_bearer_prefix_returns_401`
- Rationale: Python `hmac.compare_digest` raises TypeError on non-ASCII str args. Uvicorn delivers headers as Latin-1 decoded str, so non-ASCII bytes reach the auth code. Without the guard, an attacker gets 500 (information disclosure + DoS vector) instead of 401.

**GAP-020-R30-003: OAuth callback rate-limit DoD correction**
- File: [specs/020-security-hardening/scopes.md](specs/020-security-hardening/scopes.md) — corrected DoD item from "OAuth callback is NOT rate-limited" to "OAuth callback is rate-limited alongside start (SEC-SWEEP-001)"
- Rationale: SEC-SWEEP-001 intentionally moved callback into the rate-limited group but the DoD was not updated to reflect the change.

### Test Evidence

| Command | Result | Timestamp |
|---------|--------|-----------|
| `./smackerel.sh test unit` | PASS — Go 33 packages all green + Python 75 tests (75 passed, 1 skipped) | 2026-04-14 (gaps probe) |
