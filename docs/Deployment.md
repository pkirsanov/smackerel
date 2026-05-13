# Smackerel Production Deployment Guide

> **Architecture:** Build-Once Deploy-Many — bubbles framework gate **G074**.
> The same `git SHA` produces immutable artifacts that any environment can consume.
> **CI builds and signs. CI does NOT deploy.** Deploy runs on a different trust
> boundary, invoked by an operator (or a separate workflow with adapter-only credentials).

> **Boundary:** This guide describes the **generic** build-and-publish pipeline
> that the Smackerel repo owns. The artifacts produced here (signed images +
> per-env config bundles) are deployment-target-agnostic. Per-target **final**
> configuration — home-lab and any other concrete environment, including real
> hostnames, real IPs, mesh-VPN identity, reverse-proxy site files, `ufw` rules,
> systemd unit names, secret values, and per-target `manifest.yaml` / `params.yaml`
> — lives in the separate **`knb` deploy-adapter overlay repo**, NOT in this repo.
> The formal contract for that split is the adapter-locality rule in
> [`.github/instructions/bubbles-deployment-target.instructions.md`](../.github/instructions/bubbles-deployment-target.instructions.md)
> (see "Adapter owns target-specific knowledge" later in this document for the
> per-adapter responsibilities).

This document is operator-facing. For framework rationale see
[`.github/instructions/bubbles-deployment-target.instructions.md`](../.github/instructions/bubbles-deployment-target.instructions.md)
and [`.github/skills/bubbles-deployment-target-adapter/SKILL.md`](../.github/skills/bubbles-deployment-target-adapter/SKILL.md).

---

## Three artifacts produced per source SHA

| Artifact | Identifier | Mutable? | Producer |
|----------|-----------|----------|----------|
| Application image (`smackerel-core`) | `ghcr.io/pkirsanov/smackerel-core@sha256:<digest>` | No (immutable) | `.github/workflows/build.yml` |
| Application image (`smackerel-ml`)   | `ghcr.io/pkirsanov/smackerel-ml@sha256:<digest>`   | No (immutable) | `.github/workflows/build.yml` |
| Config bundle (per env)              | `ghcr.io/pkirsanov/smackerel-config-bundles:<env>-<sourceSha>` | No (immutable, deterministic) | `./smackerel.sh config generate --env <env> --bundle` |
| Build manifest                       | `build-manifest-<sourceSha>.yaml`                  | No (immutable) | CI workflow artifact |
| Deployment manifest (per target)     | `deploy/<target>/manifest.yaml`                    | **Yes** (pointer)              | `deploy/<target>/apply.sh` |

Image tags like `:latest`, `:main`, `:staging-latest` MUST NOT be used in any
deployment manifest. Adapters consume images by digest only.

---

## CI pipeline (`.github/workflows/build.yml`)

```text
git push (main / tag) → tests → buildx → cosign keyless sign (Sigstore + Rekor)
                              → syft SBOM attestation
                              → SLSA provenance attestation
                              → for env in (dev, test, home-lab):
                                    ./smackerel.sh config generate --env $env --bundle
                                    determinism check (regenerate, compare sha256)
                                    oras push bundle → registry
                              → publish build-manifest-<sourceSha>.yaml
                              → END (no deploy)
```

The CI workflow has **no SSH key**, **no host credentials**, **no `apply` invocation**.
It cannot mutate any deploy target.

---

## Operator workflow

```bash
# 1) Pick a release: locate the build-manifest.yaml from the CI run on the desired commit
gh run download <run-id> --name build-manifest-<sourceSha> --dir /tmp/sm-release

# 2) Promote to a target (resolves digests + bundle ref from the manifest, calls apply)
bash scripts/deploy/promote.sh --target home-lab --build-manifest /tmp/sm-release/build-manifest.yaml

# 2b) Or apply directly with explicit digests
./smackerel.sh deploy-target home-lab apply \
    --image-core=sha256:abc123... \
    --image-ml=sha256:def456... \
    --config-bundle=home-lab-9f8a7b6c

# 3) Verify
./smackerel.sh deploy-target home-lab verify

# 4) On regression, pure pointer-swap rollback (NEVER rebuilds)
./smackerel.sh deploy-target home-lab rollback
```

---

## Adapter contract (per bubbles G074)

Each `deploy/<target>/apply.sh` MUST:

1. Reject any image reference not of form `<repo>@sha256:<digest>`
2. Pull both images by digest
3. Verify cosign signature + transparency-log entry against the configured
   identity/issuer (`signing.cosignIdentity`, `signing.cosignIssuer` in `params.yaml`)
4. Pull the config bundle by `<env>-<sourceSha>` tag and verify its sha256
5. Write the new pointer into `manifest.yaml` (preserving the prior pointer in
   `previousManifest`) BEFORE starting any container
6. Run the rollout strategy declared in `params.yaml` (`recreate` for home-lab today;
   `blue-green` available)
7. On verify failure, invoke `rollback.sh` (pointer-swap, no rebuild)

Each `deploy/<target>/rollback.sh` MUST:

- Restore the `previousManifest` pointer (atomic swap)
- NEVER invoke any build step
- Fail explicitly if `previousManifest` is null (no prior release to roll back to)

---

## Adding a new deploy target

1. `cp -R deploy/home-lab deploy/<new-target>`
2. Edit `deploy/<new-target>/params.yaml` with target-specific knobs (rollout
   strategy, hostnames, replica counts, host paths)
3. Edit each script for target-specific differences (e.g., k8s vs docker compose)
4. Add the env name to `deploy/contract.yaml` `configBundles.environments` and to
   the matrix in `.github/workflows/build.yml`
5. The CLI auto-discovers the new target on the next `./smackerel.sh deploy-target` run

---

## Forbidden patterns (G074)

| Pattern | Why it's blocked |
|---------|------------------|
| Mutable image tag in `manifest.yaml` (`:latest`, `:main`, branch names) | Defeats reproducibility + rollback |
| CI workflow performing `apply`/`deploy`/`ssh` | Wrong trust boundary |
| Adapter `apply.sh` invoking `docker build`, `cargo build`, `npm run build` | Defeats build-once |
| Adapter falling back to local build on registry pull failure | Defeats build-once |
| Missing cosign verification before container start | Allows unsigned/tampered images |
| Missing bundle hash verification | Allows tampered config |
| `rollback.sh` rebuilding instead of pointer-swap | Defeats fast rollback |
| Target-side bundle generation | Defeats reproducibility |
| Plaintext secrets in bundle | Use injected env vars / sealed secrets |
| Non-deterministic bundle | Two CI runs on same SHA produce different bundles |
| Two targets sharing one `manifest.yaml` | Each target owns its own pointer |

---

# Reverse-proxy and operational concerns

The remainder of this guide covers production deployment concerns: TLS termination, auth token management, Docker Compose overrides, and HTTPS requirements for webhooks and OAuth.

## Reverse Proxy Setup for TLS

Smackerel services bind to `127.0.0.1` by default. For production, terminate TLS at a reverse proxy and forward to the core service on port `40001`.

**Only expose port `40001` (smackerel-core).** All other services (ML sidecar, PostgreSQL, NATS, Ollama) must remain on localhost.

### Caddy (Recommended — Automatic HTTPS)

Caddy automatically obtains and renews TLS certificates from Let's Encrypt.

1. [Install Caddy](https://caddyserver.com/docs/install)

2. Create a `Caddyfile`:

```
smackerel.example.com {
    reverse_proxy 127.0.0.1:40001

    header {
        X-Frame-Options "DENY"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "strict-origin-when-cross-origin"
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
    }
}
```

3. Start Caddy:
```bash
sudo caddy start
```

Caddy automatically:
- Obtains a Let's Encrypt certificate for your domain
- Redirects HTTP → HTTPS
- Renews certificates before expiry

### nginx + certbot

1. Install nginx and certbot:
```bash
sudo apt install nginx certbot python3-certbot-nginx
```

2. Create `/etc/nginx/sites-available/smackerel`:

```nginx
server {
    listen 80;
    server_name smackerel.example.com;

    location / {
        proxy_pass http://127.0.0.1:40001;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket support (future)
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

3. Enable and obtain a certificate:
```bash
sudo ln -s /etc/nginx/sites-available/smackerel /etc/nginx/sites-enabled/
sudo certbot --nginx -d smackerel.example.com
sudo systemctl reload nginx
```

Certbot configures HTTPS automatically and installs a renewal timer.

### Certificate Renewal

- **Caddy**: Automatic — no action needed.
- **certbot/nginx**: Verify the renewal timer:
  ```bash
  sudo certbot renew --dry-run
  ```

## Auth Token Generation

Generate a cryptographically secure auth token (minimum 16 characters):

```bash
openssl rand -hex 24
```

Set it in `config/smackerel.yaml`:

```yaml
runtime:
  auth_token: "your-generated-token-here"
```

**Rules:**
- Known placeholder values like `development-change-me` are rejected at startup
- Tokens shorter than 16 characters are rejected
- Token comparison uses constant-time comparison (`subtle.ConstantTimeCompare`)
- Rotate tokens by updating config, regenerating, and restarting

After changing the token:
```bash
./smackerel.sh config generate
./smackerel.sh down && ./smackerel.sh up
```

## Per-User Bearer Auth (Spec 044) — Production Posture

Spec 044 introduces a per-user PASETO v4.public bearer-auth subsystem alongside
the legacy `runtime.auth_token`. The per-environment default and the operator
runbook (key generation, bootstrap, enrollment, rotation, revocation) live in
[Operations.md](Operations.md#per-user-bearer-authentication-spec-044).
This section is the deploy-time checklist.

When deploying to a target where `auth.enabled=true` (the home-lab default;
optional per-target override for production rollouts), the deploy adapter MUST
inject the spec 044 secrets via the standard secret-injection mechanism. They
are NEVER committed in the build's per-env config bundle — the bundle treats
them as empty-string placeholders and the deploy adapter overlays the real
values at apply time.

Required `AUTH_*` env vars (target-specific):

| Env var | Source | Required when |
|---|---|---|
| `AUTH_SIGNING_ACTIVE_PRIVATE_KEY` | `smackerel-core auth keygen` (one per target) | `auth.enabled=true` AND `runtime.environment=production` |
| `AUTH_SIGNING_ACTIVE_KEY_ID` | Operator-chosen short identifier (e.g. `key-2026-05`) | `auth.enabled=true` AND `runtime.environment=production` |
| `AUTH_AT_REST_HASHING_KEY` | `openssl rand -hex 32` (must differ from signing key) | `auth.enabled=true` AND `runtime.environment=production` |
| `AUTH_SIGNING_PRIOR_PUBLIC_KEY` | Previous active public key (hex) | Only during a key rotation overlap window |
| `AUTH_SIGNING_PRIOR_KEY_ID` | Previous active key id | Only during a key rotation overlap window |
| `AUTH_BOOTSTRAP_TOKEN` | One-shot secret (`openssl rand -hex 24`); cleared after first user enrolls | Fresh production deployment with zero enrolled users |

Pre-`apply` checklist for any target with `auth.enabled=true`:

1. Confirm the target's bundle reports the three required keys as empty
   placeholders (per `bubbles G074` — secrets MUST NOT live in the bundle).
2. Confirm the deploy adapter overlay populates `AUTH_SIGNING_ACTIVE_PRIVATE_KEY`,
   `AUTH_SIGNING_ACTIVE_KEY_ID`, and `AUTH_AT_REST_HASHING_KEY` from the
   target's secret store before invoking the runtime.
3. For a fresh target, set `AUTH_BOOTSTRAP_TOKEN` in the overlay, run the
   bootstrap flow per Operations.md, then remove the bootstrap secret from the
   overlay and re-`apply`.
4. The runtime fails loud at startup if any required value is missing or if the
   hashing key equals the signing key (spec 044 OQ-8). Operators see explicit
   error messages naming each missing field; recovery is to populate the secret
   and re-`apply`.

Forbidden:

- Committing real `AUTH_SIGNING_*` or `AUTH_AT_REST_HASHING_KEY` values into
  `config/smackerel.yaml` or any file under `config/generated/`.
- Reusing the signing private key as the at-rest hashing key (rejected at
  startup per OQ-8).
- Leaving `AUTH_BOOTSTRAP_TOKEN` populated in the deploy overlay after the
  first user has been enrolled (the runbook clears it).

### Spec 051 Defense-In-Depth Contract

Spec 051 (`specs/051-deployment-secret-auth-contract/`) ratifies the deployment
secret contract that this section describes and adds two cross-cutting hard
gates that fire even if a future operator skips a step above:

1. **Layered secret rejection.** The dev-default Postgres password
   (`infrastructure.postgres.password: smackerel`) is refused at BOTH the SST
   loader (`scripts/commands/config.sh` for `TARGET_ENV=home-lab` and any
   future production-class target) AND the runtime (`internal/config/Validate`
   when `SMACKEREL_ENV=production`). Either layer alone protects the
   deployment; both layers together give defense-in-depth (FR-051-005,
   SCN-051-S02).
2. **Log-redaction guarantee.** Every error path that could plausibly receive
   a secret value is covered by the security-static adversarial test
   `internal/config/log_redaction_test.go`. The test seeds canary substrings
   and asserts no canary appears in any returned error. Adding a new secret
   env var to `loadAuthConfig` or `Validate` MUST extend this test to cover
   the new error path (FR-051-007, SCN-051-S03).

The canonical key names in the table above are also pinned by
`internal/config/docs_required_keys_test.go`: every name in this section MUST
appear at least once in this doc and in [Operations.md](Operations.md), and
the pre-spec-044 HMAC-based auth aliases listed in the test's
`forbiddenAuthAliases` slice MUST NOT appear anywhere. The test runs on every
`./smackerel.sh test unit` invocation and prevents silent contract drift
(FR-051-006).

### API-Consumer Migration (Scope 02)

A target that flips `auth_enabled=true` for the first time gains the per-user
`bearerAuthMiddleware` on the API hot path. Two consumer-visible changes
follow:

1. **Bearer-token transition.** API callers MUST present a per-user PASETO
   token issued via the bootstrap / enroll flow (or, when
   `auth.production_shared_token_fallback_enabled=true`, the legacy shared
   `SMACKEREL_AUTH_TOKEN`). The middleware verifies the token statelessly with
   no DB roundtrip per request, attaches the resolved `Session` to the request
   context, and returns `HTTP 401` on failure.
2. **Body-supplied actor identifiers are rejected.** In production mode, the
   photos `MintReveal`, cloud-drive `Connect`, and user-annotation create
   handlers reject any client-supplied actor identifier in the request body or
   headers (closing MIT-040-S-008, MIT-038-S-003, MIT-027-TRACE-001
   actor-source segment). See the operator-side error-code table in
   [Operations.md](Operations.md#production-body--header-actor-identity-rejection-scope-02-mit-closures).
   API consumers that previously sent `actor_id`, `owner_user_id`, or
   `actor_source` MUST be updated to omit those fields before the target flip
   — the actor identity is derived from the bearer-token claims and no
   client-supplied value can override it.

In `dev` and `test` (or in production while `auth.enabled=false`), all three
handlers continue to honor body-supplied actor identifiers and the
`X-Actor-Id` header, so existing local-dev consumers and integration fixtures
do not need to be changed before the flip.

### API-Consumer Migration (Scope 03)

Scope 03 extends per-user PASETO authentication onto the PWA, browser
extension, and Telegram bridge, plus an admin token-management UI. Each
surface has a distinct migration step for production targets where
`auth.enabled=true`.

1. **PWA users — clear browser state and re-authenticate.** Existing PWA
   sessions backed by a stored `SMACKEREL_AUTH_TOKEN` (in `localStorage` or
   the legacy cookie) MUST be cleared and the user must re-authenticate via
   the new `POST /v1/web/login` endpoint. The login handler converts a
   per-user PASETO into an `auth_token` cookie marked `HttpOnly +
   SameSite=Lax + Path=/` and (in production) `Secure`. End users who keep
   the existing token in localStorage will see authenticated requests
   continue to work unchanged in `dev` / `test`; in production once the
   shared-token fallback is disabled the cookie path is the only working
   browser auth surface. See
   [Operations.md](Operations.md#pwa-cookie-derived-sessions-v1weblogin) for
   the full request shape and cookie attribute table.

2. **Browser extension users — install per-user tokens.** The extension
   storage slot `chrome.storage.local.authToken`
   (`web/extension/background.js`) accepts EITHER a per-user PASETO
   (production) OR the legacy shared `SMACKEREL_AUTH_TOKEN` (dev/test). To
   migrate an installation:

    - On the server: mint a token for the user with `smackerel-core auth
      enroll <user-id>` (see Operations.md "CLI Surface" for the docker exec
      form).
    - On the client: open the extension popup, paste the wire token into
      the auth-token input, and click save. The popup writes the value to
      `chrome.storage.local.authToken` atomically; subsequent capture
      requests carry it as `Authorization: Bearer <token>` with no further
      code change. Operators MAY also write `chrome.storage.local.authToken`
      directly via Chrome DevTools for bulk rollouts.

3. **Operators — populate Telegram chat → user mapping before flip.** Any
   production target that intends to use the Telegram bridge with per-user
   attribution MUST populate `telegram.user_mapping` in
   `config/smackerel.yaml` (or the deploy adapter overlay's
   `TELEGRAM_USER_MAPPING` env var) before flipping `auth_enabled=true`.
   Format: `<chat_id>:<user_id>` pairs, comma-separated. Production with an
   unmapped chat drops the message at the bot's entry point (`slog.Warn` +
   no internal API call); production with empty mapping rejects all chats.
   Dev / test tolerate empty mapping. Steps:

    - Edit `telegram.user_mapping` in `config/smackerel.yaml` (or the deploy
      overlay).
    - `./smackerel.sh config generate` to refresh `<env>.env` with the new
      `TELEGRAM_USER_MAPPING` value.
    - Restart the stack so the bot reloads its mapping (the parser is
      startup-only).

4. **Admin operators — exercise the token-management UI behind admin
   bearer.** The admin token-management UI is reachable at `GET
   /admin/auth/tokens` (`internal/api/admin_ui.go`) behind
   `bearerAuthMiddleware`. The page exposes three panels — Mint a New
   User, Enrolled Users (with per-row Rotate), and Revoke a Specific
   Token — that drive the existing Scope 02 `/v1/auth/*` admin REST
   endpoints. Per the admin-scope rule (see
   [Operations.md](Operations.md#admin-token-management-ui-adminauthtokens)),
   per-user PASETO sessions do NOT yet pass `callerIsAdmin`, so admin
   mutations require either the bootstrap session or — when
   `production_shared_token_fallback_enabled=true` — the legacy shared
   token. The page itself loads under any authenticated session.

#### Telegram Per-User Attribution Wiring (F02 Scope 04 — shipped)

The library `internal/telegram/per_user_token.go` (`PerUserTokenMinter`) is
shipped, unit-tested, and integration-tested in isolation. Spec 044
Scope 04 closes the F02 deferred-finalize-blocker by wiring `MintForChat`
into the bot's outbound HTTP calls via `Bot.bearerForChat(chatID)` and
the `Bot.setBearerHeader(req, chatID)` helper
(`internal/telegram/bot.go` lines 200–254). Production wiring is
performed by `startTelegramBotIfConfigured` (`cmd/core/wiring.go`) when
`auth.enabled=true` AND `auth.signing.active_private_key` is configured;
the minter is constructed once with TTL = 5 minutes and attached via
`tgBot.SetPerUserTokenMinter(minter)` before `Start`.

Operator implication for any production Telegram deployment:

| Setting | Behavior |
|---|---|
| `auth_enabled=true` AND `production_shared_token_fallback_enabled=true` | **Working** — bot mints per-user PASETO for mapped chats; production unmapped chats are refused (no fallback to shared bearer); legacy callers presenting `runtime.auth_token` still authenticate during the transition window |
| `auth_enabled=true` AND `production_shared_token_fallback_enabled=false` (recommended default) | **Working** — bot mints per-user PASETO for mapped chats; production unmapped chats are refused; legacy callers presenting `runtime.auth_token` are rejected with 401 |

Closure evidence:
[`internal/telegram/bot_wiring_test.go`](../internal/telegram/bot_wiring_test.go)
(8 unit cases),
[`tests/integration/auth_telegram_f02_wiring_test.go`](../tests/integration/auth_telegram_f02_wiring_test.go)
(`TestF02Wiring_SetPerUserTokenMinter_HappyPath`,
`TestF02Wiring_SetPerUserTokenMinter_ProductionUnmappedRefuses`),
[`tests/integration/auth_telegram_e2e_test.go`](../tests/integration/auth_telegram_e2e_test.go)
(Scope 03 e2e). Operator deprecation sequence for the shared-token
fallback flag is documented in
[Operations.md → "Deprecation Pathway"](Operations.md#deprecation-pathway--production_shared_token_fallback_enabled).

## Docker Compose Production Overrides

Create a `docker-compose.prod.yml` for production-specific settings:

```yaml
services:
  smackerel-core:
    restart: always
    environment:
      - SMACKEREL_LOG_LEVEL=warn
    deploy:
      resources:
        limits:
          memory: 512M

  smackerel-ml:
    restart: always
    deploy:
      resources:
        limits:
          memory: 3G

  postgres:
    restart: always
    deploy:
      resources:
        limits:
          memory: 1G

  nats:
    restart: always
    deploy:
      resources:
        limits:
          memory: 512M
```

Use with the base Compose file:
```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

**Production considerations:**
- Increase PostgreSQL memory limit for larger datasets
- Increase ML sidecar memory if using larger embedding models
- Set `restart: always` so services recover from crashes
- Use Docker volumes on fast storage (SSD) for PostgreSQL data
- Back up PostgreSQL daily via the spec 048 contract (see [Operations Runbook → Backup & Restore](Operations.md#backup--restore))

### Spec 048 — Deploy Adapter Backup Contract

The Smackerel runtime owns the dump, retention, status file, and restore drill. The deploy adapter overlay (e.g., `knb` for the home-lab target) owns scheduling and off-host shipping. Adapter responsibilities:

| Responsibility | Adapter contract |
|----------------|------------------|
| Daily timer / cron | Install a systemd timer (or equivalent) that invokes `./smackerel.sh backup` on the host. Cadence MUST be at least daily so the 25h `SmackerelBackupStale` alert window stays satisfied. |
| Off-host destination | Set `BACKUP_DESTINATION_URL` in `app.env` (the adapter-written env file). The destination string is opaque to Smackerel — the adapter chooses S3, BackBlaze, NFS, rclone, etc. Never commit the real URL in this repo. |
| Off-host shipping job | After `./smackerel.sh backup` succeeds, the adapter ships `${BACKUP_LOCAL_DIR}/smackerel-*.sql.gz` to `${BACKUP_DESTINATION_URL}`. Smackerel does not invoke the shipping job; the adapter chains it after the dump. |
| Restore drill cadence | The adapter SHOULD invoke `./smackerel.sh backup-restore-test` at least weekly so a silent failure to produce a restorable artifact surfaces inside the alert window. |
| Bind mounts | If the adapter pins `BACKUP_LOCAL_DIR` to an absolute path on the host (recommended in production), bind-mount that path into the `smackerel-core` container so the watcher can read the status file. |

The adapter MUST NOT override the retention slot counts (`BACKUP_RETENTION_DAILY=7`, `BACKUP_RETENTION_WEEKLY=4`) per Product Principle 9 — those are part of the product contract and changing them is a spec-level decision, not an adapter knob.

## Telegram Webhook HTTPS Requirement

Telegram Bot API requires HTTPS for webhooks. When deploying with a public domain:

1. Set up TLS via the reverse proxy (Caddy or nginx — see above)
2. Telegram will use long polling by default. Webhook mode requires an HTTPS URL:
   - The bot connects outbound to Telegram's API servers, so long polling works without HTTPS
   - If you switch to webhook mode, the callback URL **must** be HTTPS
3. Ensure your domain's TLS certificate is valid and trusted (Let's Encrypt certificates work)

The default Smackerel configuration uses long polling, which works behind a firewall without exposing any ports to the internet. Webhook mode is only needed if you require lower latency for bot responses.

## OAuth Callback URL HTTPS Requirement

OAuth2 providers (Google) require HTTPS callback URLs in production. When switching from localhost to a public domain:

1. Update `config/smackerel.yaml`:
   ```yaml
   oauth:
     google:
       redirect_url: "https://smackerel.example.com/auth/google/callback"
   ```

2. Update the authorized redirect URI in Google Cloud Console to match

3. Regenerate config and restart:
   ```bash
   ./smackerel.sh config generate
   ./smackerel.sh down && ./smackerel.sh up
   ```

**Rules:**
- Google requires HTTPS for production redirect URIs (localhost exemption only applies to `http://127.0.0.1`)
- The redirect URL in config must exactly match the URL registered in Google Cloud Console
- After updating, existing OAuth tokens remain valid — only new authorization flows use the updated URL

## Port Exposure Summary

| Port | Service | Expose via reverse proxy? |
|------|---------|--------------------------|
| 40001 | smackerel-core (API + Web UI) | **Yes** |
| 40002 | smackerel-ml (ML sidecar) | **No** — internal only |
| 42001 | PostgreSQL | **No** — internal only |
| 42002 | NATS client | **No** — internal only |
| 42003 | NATS monitoring | **No** — internal only |
| 42004 | Ollama | **No** — internal only |
| 42005 | Prometheus (monitoring profile, dev) | **No** — operator-only on dev; deploy adapter chooses overlay exposure |

## Monitoring Profile (Spec 049 — Optional)

Smackerel ships a self-contained Prometheus monitoring stack as an
OPT-IN Docker Compose profile. The default deploy compose file
exists but `prometheus` does NOT start unless the operator passes
`--profile monitoring` to `docker compose up`.

```bash
# On the deploy host (after the deploy adapter has extracted the
# bundle and written app.env)
docker compose --env-file app.env -f compose.deploy.yml \
  --profile monitoring up -d
```

### What This Repo Ships

| Artifact | Source | Purpose |
|----------|--------|---------|
| Prometheus scrape config template | `config/prometheus/prometheus.yml.tmpl` | envsubst-rendered to `config/generated/prometheus.yml` and bundled into the deploy artifact |
| Alert rules | `config/prometheus/alerts.yml` | Committed as-is, mounted read-only at `/etc/prometheus/alerts.yml` |
| Prometheus service definition | `deploy/compose.deploy.yml::services.prometheus` | Inherits spec 042 fail-loud bind + spec 045 read-only + resource envelope |
| Dashboard inventory | `docs/Operations.md::Monitoring Stack` | Names the 10 dashboards the runtime metrics support |
| Alert runbook | `docs/Operations.md::Alert Runbook` | One row per alert: name, severity, firing action |
| SST keys | `config/smackerel.yaml::monitoring.prometheus.* + environments.<env>.prometheus_*` | Single source of truth for image, port, retention, intervals |

### What The Deploy Adapter Owns

Anything operator-specific is out of scope for this repo. The deploy
adapter overlay (e.g. the `knb` repo) MUST provide:

- `HOST_BIND_ADDRESS` substitution value (tailnet IP or loopback)
- Reverse-proxy / TLS fronting for the Prometheus UI (if needed)
- Alertmanager configuration (receivers: Telegram, PagerDuty, email)
- Grafana provisioning (datasource config + dashboard JSON)
- Retention beyond the SST default (remote-write, long-term storage)
- Secret rotation for any Alertmanager receivers it adds

### Verifying The Profile

```bash
# After enabling the profile, confirm Prometheus is up and scraping
curl -s http://${HOST_BIND_ADDRESS}:${PROMETHEUS_HOST_PORT}/-/healthy
curl -s http://${HOST_BIND_ADDRESS}:${PROMETHEUS_HOST_PORT}/api/v1/targets \
  | jq '.data.activeTargets[] | {job: .labels.job, health: .health}'

# Expected: both `smackerel-core` and `smackerel-ml` targets show
# health: "up". If a target is `down`, the failure surfaces in
# Prometheus's logs (`docker logs <prometheus-container>`).
```

### Disabling The Profile

```bash
# Stop only the prometheus service, leaving the runtime stack
docker compose --env-file app.env -f compose.deploy.yml \
  --profile monitoring stop prometheus

# Or omit the --profile flag entirely on next `up` — Compose will
# only start services in the default profile set.
```

The named volume `${PROMETHEUS_VOLUME_NAME}` (e.g.
`smackerel-home-lab-prometheus-data`) survives `down`/`up` cycles
so historical metrics persist for the configured
`PROMETHEUS_RETENTION_DAYS` (default 15 days).

