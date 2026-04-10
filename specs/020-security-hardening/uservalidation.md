# User Validation: 020 Security Hardening — Docker Binding, Auth Enforcement, Crypto Hygiene

**Feature:** 020-security-hardening
**Created:** 2026-04-10

---

## Acceptance Checklist

### Network Exposure (SEC-005, SEC-016)
- [x] PostgreSQL host port is bound to 127.0.0.1 — not accessible from LAN
- [x] NATS client and monitor host ports are bound to 127.0.0.1
- [x] ML sidecar host port is bound to 127.0.0.1
- [x] Ollama host port is bound to 127.0.0.1
- [x] smackerel-core port remains bound to 127.0.0.1 (already correct)

### NATS Token Visibility (SEC-006)
- [x] NATS auth token does NOT appear in `docker ps` command column
- [x] NATS uses config file mount (`nats.conf`) instead of `--auth` CLI argument
- [x] Generated `nats.conf` lives in gitignored `config/generated/`

### ML Sidecar Auth (SEC-007)
- [x] ML sidecar rejects unauthenticated requests on non-health endpoints when token configured
- [x] ML sidecar `/health` endpoint works without authentication (Docker healthcheck)
- [x] ML sidecar allows all requests when `SMACKEREL_AUTH_TOKEN` is empty (dev mode)

### Web UI Auth (SEC-001)
- [x] Web UI routes require authentication when `auth_token` is configured
- [x] Web UI routes allow unauthenticated access when `auth_token` is empty (dev mode)

### OAuth Rate Limiting (SEC-002)
- [x] OAuth start endpoint rejects >10 requests/minute from the same IP with 429
- [x] OAuth start endpoint allows ≤10 requests/minute normally

### Decrypt Fail-Closed (SEC-004)
- [x] Corrupted/invalid encrypted data returns error when encryption key is present
- [x] No silent plaintext fallback when encryption is configured
- [x] Plaintext passthrough still works when no encryption key (dev mode)

### Startup Warnings (SEC-021)
- [x] smackerel-core logs WARN when `auth_token` is empty at startup
- [x] ML sidecar logs WARNING when `SMACKEREL_AUTH_TOKEN` is empty at startup
- [x] No warning emitted when `auth_token` is properly configured

### General
- [x] All config values flow through `config/smackerel.yaml` → SST pipeline (no hardcoded defaults)
- [x] Systems with empty `auth_token` continue to work without auth (backwards compatible)
- [x] Docker healthchecks continue to work for all services

## Notes

Baseline checklist — items checked by default. Uncheck any item that fails validation.
