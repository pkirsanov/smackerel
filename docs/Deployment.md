# Smackerel Production Deployment Guide

> **Architecture:** Build-Once Deploy-Many — bubbles framework gate **G074**.
> The same `git SHA` produces immutable artifacts that any environment can consume.
> **CI builds and signs. CI does NOT deploy.** Deploy runs on a different trust
> boundary, invoked by an operator (or a separate workflow with adapter-only credentials).

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
- Back up PostgreSQL daily (see [Operations Runbook](Operations.md#backup--restore))

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
