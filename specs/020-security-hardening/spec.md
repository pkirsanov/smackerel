# Spec: Security Hardening — Docker Binding, Auth Enforcement, Crypto Hygiene

**Feature:** 020-security-hardening
**Status:** Done
**Created:** 2026-04-10

---

## Problem Statement

A security review of Smackerel's deployed Docker Compose stack and application code identified 9 findings across network exposure, authentication gaps, and cryptographic hygiene. The most critical issues are that PostgreSQL, NATS, ML sidecar, and Ollama bind their host-forwarded ports to `0.0.0.0`, making them accessible to any device on the LAN — while only `smackerel-core` correctly binds to `127.0.0.1`.

Additionally, the ML sidecar FastAPI has zero authentication on all endpoints, the Web UI routes have an existing `webAuthMiddleware` that is never applied, OAuth start endpoints have no rate limiting, and the decryption layer silently falls back to plaintext on failure rather than failing closed.

These are not theoretical risks. Any device on the same network as the Docker host can connect directly to PostgreSQL (full DB access), NATS (inject/read messages), the ML sidecar (invoke LLM operations), and Ollama (arbitrary model inference). The Web UI is unauthenticated even when the operator has explicitly configured `auth_token`.

### Findings Addressed

| ID | Severity | Description |
|----|----------|-------------|
| SEC-005 | High | PostgreSQL and NATS ports bind to `0.0.0.0` — accessible on LAN |
| SEC-007 | High | ML sidecar FastAPI has zero authentication; port exposed to all interfaces |
| SEC-016 | Medium | ML sidecar host port binds to all interfaces |
| SEC-001 | Medium | Web UI routes have no auth even when AuthToken is configured; `webAuthMiddleware` exists but is unused |
| SEC-002 | Medium | OAuth start/callback endpoints have no rate limiting or auth |
| SEC-021 | Medium | Default empty `auth_token` in config — system runs with no auth if validation bypassed |
| SEC-003 | Low | AES encryption key derived from `auth_token` via SHA-256 — dual-purpose secret |
| SEC-004 | Low | Decryption silently falls back to plaintext on failure |
| SEC-006 | Low | NATS auth token visible in `docker ps` (command-line argument) |

---

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| Self-Hoster | Individual running Smackerel on a home/office network via Docker Compose | System is not exploitable by other LAN devices; auth works when configured | Full system access |
| LAN Attacker | Any device on the same network as the Docker host (another computer, IoT device, compromised phone) | Probe open ports, access databases, inject messages, invoke ML endpoints | No legitimate access — must be denied |
| Operator | Person configuring and monitoring Smackerel | Clear warnings when auth is not configured; confidence that auth_token actually protects all surfaces | Config + monitoring access |
| ML Sidecar | Internal Python FastAPI service handling embeddings, LLM gateway, vision | Accept requests only from authenticated internal callers | Internal service-to-service |

---

## Threat Model

### Attack Surface: Network Exposure (SEC-005, SEC-007, SEC-016)

**Current state:** `docker-compose.yml` port mappings for `postgres`, `nats`, `smackerel-ml`, and `ollama` use bare `${HOST_PORT}:${CONTAINER_PORT}` syntax, which Docker resolves to `0.0.0.0:PORT`. Only `smackerel-core` uses `127.0.0.1:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}`.

**Threat:** Any LAN device can connect to PostgreSQL (read/write all data, drop tables), NATS (inject messages, read streams), ML sidecar (invoke LLM, embeddings), and Ollama (run arbitrary model inference).

**Mitigation:** Bind all host-forwarded ports to `127.0.0.1` via `config/smackerel.yaml` host-bind address and the config generation pipeline.

### Attack Surface: Unauthenticated ML Sidecar (SEC-007)

**Current state:** `ml/app/main.py` defines a FastAPI app with `/health` and NATS-subscribed processing. All HTTP endpoints have zero authentication middleware.

**Threat:** Direct HTTP access to ML sidecar allows invoking embedding generation, LLM gateway calls, and vision processing without any credential check.

**Mitigation:** Add auth middleware to ML sidecar that validates `SMACKEREL_AUTH_TOKEN` on all non-health endpoints. Health endpoint stays open for Docker healthcheck.

### Attack Surface: Unauthenticated Web UI (SEC-001)

**Current state:** `internal/api/router.go` registers Web UI routes without `webAuthMiddleware`, with a comment noting this is intentional for "local-first" design. However, when `auth_token` IS configured, the operator expects auth to protect all surfaces.

**Threat:** When auth_token is configured, API routes require auth but Web UI routes do not. An attacker (or unauthorized user) can search artifacts, view digests, and access settings through the Web UI without authentication.

**Mitigation:** Apply `webAuthMiddleware` to the Web UI route group. When `auth_token` is empty, all requests pass through (current dev behavior). When configured, Web UI requires the same auth.

### Attack Surface: OAuth Rate Limiting (SEC-002)

**Current state:** `/auth/{provider}/start` and `/auth/{provider}/callback` have no rate limiting. They are outside the authenticated route group.

**Threat:** Brute-force state exhaustion, CSRF probe flooding, or callback replay attacks.

**Mitigation:** Rate-limit the OAuth start endpoint (e.g., 10 req/min per IP).

### Attack Surface: Silent Plaintext Fallback (SEC-004)

**Current state:** `internal/auth/store.go` `decrypt()` returns the raw encoded string on any decryption failure (not base64, too short, GCM Open failure) with a `slog.Warn` message.

**Threat:** If an attacker can manipulate stored token data, they can bypass encryption by storing plaintext that the system silently accepts. Also masks data corruption.

**Mitigation:** When an encryption key IS configured, decryption failures must return an error rather than silently returning the ciphertext as plaintext.

### Attack Surface: NATS Auth in CLI Args (SEC-006)

**Current state:** `docker-compose.yml` passes `--auth ${SMACKEREL_AUTH_TOKEN}` in the NATS `command` array. This is visible in `docker ps` output.

**Threat:** Anyone with Docker CLI access on the host can see the auth token in the process command line.

**Mitigation:** Use a NATS config file mounted as a Docker secret/volume instead of command-line arguments.

---

## Requirements

### R-001: Bind All Docker Service Ports to Localhost (SEC-005, SEC-016)

```gherkin
Scenario: All Docker host-forwarded ports bind to 127.0.0.1
  Given the Smackerel stack is deployed via Docker Compose
  When a device on the same LAN attempts to connect to any service host port
  Then the connection is refused because all host-forwarded ports bind to 127.0.0.1

Scenario: Config generation produces localhost-bound port mappings
  Given config/smackerel.yaml defines service ports
  When ./smackerel.sh config generate runs
  Then all port mappings in docker-compose.yml use 127.0.0.1 prefix

Scenario: Postgres is not accessible from LAN
  Given the Smackerel stack is running
  When a LAN device connects to the Docker host on the Postgres host port
  Then the connection is refused

Scenario: NATS is not accessible from LAN
  Given the Smackerel stack is running
  When a LAN device connects to the Docker host on the NATS client or monitor host port
  Then the connection is refused

Scenario: ML sidecar is not accessible from LAN
  Given the Smackerel stack is running
  When a LAN device connects to the Docker host on the ML sidecar host port
  Then the connection is refused

Scenario: Ollama is not accessible from LAN
  Given the Smackerel stack is running with the ollama profile
  When a LAN device connects to the Docker host on the Ollama host port
  Then the connection is refused
```

### R-002: ML Sidecar Auth Middleware (SEC-007)

```gherkin
Scenario: ML sidecar rejects unauthenticated requests
  Given the ML sidecar is running with SMACKEREL_AUTH_TOKEN configured
  When a request arrives at any non-health endpoint without a valid auth token
  Then the ML sidecar returns 401 Unauthorized

Scenario: ML sidecar accepts authenticated requests
  Given the ML sidecar is running with SMACKEREL_AUTH_TOKEN configured
  When a request arrives with a valid Bearer token matching SMACKEREL_AUTH_TOKEN
  Then the ML sidecar processes the request normally

Scenario: ML sidecar health endpoint remains unauthenticated
  Given the ML sidecar is running
  When a request arrives at /health without any auth header
  Then the ML sidecar returns 200 with health status
  Because Docker healthcheck must work without credentials

Scenario: ML sidecar allows all requests when auth_token is empty
  Given the ML sidecar is running with SMACKEREL_AUTH_TOKEN empty
  When a request arrives at any endpoint without auth
  Then the ML sidecar processes the request normally
  Because dev mode should not require auth configuration
```

### R-003: Apply Web UI Auth Middleware (SEC-001)

```gherkin
Scenario: Web UI requires auth when auth_token is configured
  Given smackerel-core is running with a non-empty auth_token
  When a browser request arrives at any Web UI route without auth credentials
  Then the server returns 401 Unauthorized

Scenario: Web UI accepts valid cookie auth
  Given smackerel-core is running with a non-empty auth_token
  When a browser request arrives at a Web UI route with a valid auth_token cookie
  Then the server serves the page normally

Scenario: Web UI accepts valid Bearer auth
  Given smackerel-core is running with a non-empty auth_token
  When a request arrives at a Web UI route with a valid Authorization: Bearer header
  Then the server serves the page normally

Scenario: Web UI allows all requests when auth_token is empty
  Given smackerel-core is running with an empty auth_token
  When a browser request arrives at any Web UI route without auth
  Then the server serves the page normally
  Because local dev mode preserves frictionless access
```

### R-004: Rate-Limit OAuth Start Endpoint (SEC-002)

```gherkin
Scenario: OAuth start endpoint is rate-limited
  Given smackerel-core is running with OAuth providers configured
  When more than 10 requests arrive at /auth/{provider}/start from the same IP within 1 minute
  Then subsequent requests receive 429 Too Many Requests

Scenario: OAuth start endpoint allows traffic within rate limit
  Given smackerel-core is running with OAuth providers configured
  When 5 requests arrive at /auth/{provider}/start from the same IP within 1 minute
  Then all 5 requests are processed normally
```

### R-005: Startup Warning for Empty Auth Token (SEC-021)

```gherkin
Scenario: Startup emits warning when auth_token is empty
  Given auth_token is empty in config/smackerel.yaml
  When smackerel-core starts
  Then the startup log emits a WARN-level message indicating the system is running without authentication

Scenario: ML sidecar emits warning when auth_token is empty
  Given SMACKEREL_AUTH_TOKEN is empty
  When the ML sidecar starts
  Then the startup log emits a WARNING-level message indicating the sidecar is running without authentication

Scenario: No warning when auth_token is configured
  Given auth_token is set to a non-empty value
  When smackerel-core starts
  Then no auth warning is emitted at startup
```

### R-006: NATS Config File Mount (SEC-006)

```gherkin
Scenario: NATS auth token is not visible in docker ps
  Given the Smackerel stack is running
  When an operator runs docker ps to inspect running containers
  Then the NATS auth token does not appear in the command column

Scenario: NATS uses config file for authentication
  Given config/smackerel.yaml defines auth_token
  When ./smackerel.sh config generate runs
  Then a NATS config file is generated with the auth token
  And docker-compose.yml mounts the config file and references it via --config flag
  And the NATS command no longer includes --auth
```

### R-007: Fail Closed on Decryption Failure (SEC-004)

```gherkin
Scenario: Decryption fails closed when encryption key is configured
  Given an encryption key is derived from a non-empty auth_token
  When decrypt() is called on data that cannot be decrypted
  Then an error is returned instead of the raw ciphertext
  And the token is NOT silently treated as plaintext

Scenario: No encryption key means plaintext passthrough
  Given auth_token is empty (no encryption key derived)
  When decrypt() is called on any stored value
  Then the value is returned as-is
  Because encryption is opt-in via auth_token presence

Scenario: Valid encrypted data decrypts successfully
  Given an encryption key is derived from a non-empty auth_token
  When decrypt() is called on properly encrypted data
  Then the original plaintext is returned
```

---

## Non-Functional Requirements

- **Performance:** Auth middleware on ML sidecar and Web UI adds < 1ms latency per request (constant-time token comparison).
- **Backwards Compatibility:** Systems running with empty `auth_token` must continue to work without auth. No behavior change for unconfigured systems.
- **Observability:** Auth failures logged at WARN level with request path and IP (no token values in logs).
- **Configuration:** All new config values (host bind address, NATS config path) flow through `config/smackerel.yaml` → `config generate` pipeline (SST policy).
- **Migration:** Existing NATS data volumes remain compatible. NATS config file mount is additive.

---

## Success Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Host port binding | 100% of services bind to 127.0.0.1 | Verify `docker-compose.yml` port mappings and `ss -tlnp` on running stack |
| ML sidecar auth | 0 unauthenticated non-health requests succeed when token configured | Integration test with/without Bearer header |
| Web UI auth | 0 unauthenticated Web UI requests succeed when token configured | Integration test with/without cookie/Bearer |
| OAuth rate limiting | >10 req/min/IP blocked on start endpoint | Unit test with rapid sequential requests |
| Empty auth warning | WARN log emitted on startup when auth_token empty | Unit test on startup log output |
| NATS token visibility | 0 secrets in `docker ps` command column | Manual verification on running stack |
| Decrypt fail-closed | 0 silent plaintext fallbacks when encryption key present | Unit test with corrupted ciphertext |

---

## Outcome Contract

**Intent:** Harden Smackerel's Docker Compose deployment so that all services are protected from LAN-level network access, all HTTP surfaces enforce authentication when configured, and cryptographic operations fail closed rather than silently degrading.

**Success Signal:** With `auth_token` configured, no service port is reachable from another LAN device, no HTTP endpoint (except health checks) responds without valid authentication, and corrupted encrypted data returns an error rather than plaintext.

**Hard Constraints:**
- All config values originate from `config/smackerel.yaml` — zero hardcoded defaults (SST policy)
- Empty `auth_token` continues to work as frictionless dev mode — no forced auth for unconfigured systems
- Docker healthchecks must continue to work (health endpoints remain unauthenticated)
- No changes to inter-container networking — containers communicate on the Docker network, host port binding is the only change

**Failure Condition:** Any service port remains LAN-accessible after deployment, any non-health HTTP endpoint accepts unauthenticated requests when `auth_token` is configured, or decryption silently returns ciphertext as plaintext when an encryption key is present.

---

## Competitive Analysis

Not applicable — this is an infrastructure security hardening feature, not a user-facing capability. There is no competitive differentiation surface; these are baseline security hygiene requirements for any self-hosted system.

---

## Improvement Proposals

### IP-001: Mutual TLS for Inter-Service Communication

- **Impact:** High
- **Effort:** L
- **Rationale:** Currently inter-container traffic (core→ML, core→NATS, core→postgres) uses plaintext within the Docker network. mTLS would protect against container escape attacks.
- **Actors Affected:** All internal services
- **Deferred:** Out of scope for this hardening pass. Revisit when multi-host deployment is considered.

### IP-002: Separate Encryption Key from Auth Token (SEC-003)

- **Impact:** Low
- **Effort:** S
- **Rationale:** Currently AES encryption key is derived from `auth_token` via SHA-256. A dedicated `encryption_key` config field would eliminate the dual-purpose secret concern.
- **Actors Affected:** Self-Hoster (config change), Operator (key rotation)
- **Deferred:** Low severity. Can be addressed in a future crypto-hygiene pass. Noted in SEC-003.

### IP-003: Login Page for Web UI

- **Impact:** Medium
- **Effort:** M
- **Rationale:** R-003 adds middleware but returns 401 to browsers. A login page with cookie-setting flow would provide a proper UX for authenticated Web UI access.
- **Actors Affected:** Self-Hoster (browser access)
- **Deferred:** Can follow as a UX improvement after R-003 is implemented.
