# Bug: No TLS / Reverse Proxy Documentation for Production Deployment

**Bug ID:** BUG-020-002
**Severity:** High
**Found by:** System review (TR-002)
**Feature:** 020-security-hardening

---

## Problem

Smackerel's architecture describes Telegram webhook and OAuth callback flows that require HTTPS endpoints reachable from the public internet. However:

1. **No TLS termination exists anywhere in the stack.** The Go core binds to `127.0.0.1:8080` (plaintext HTTP). The ML sidecar binds to `127.0.0.1:8081` (plaintext HTTP). No service supports TLS natively.

2. **No reverse proxy documentation exists.** The standard approach for self-hosted Docker applications is a reverse proxy (nginx, Caddy, or Traefik) that handles TLS termination and forwards to the application. `docs/smackerel.md` mentions Traefik in a technology evaluation table but provides no setup guide.

3. **Affected flows that require HTTPS:**
   - **Telegram webhooks:** Telegram Bot API requires webhook URLs to use HTTPS. Without TLS, the bot can only operate in polling mode — which the design does not document as a limitation.
   - **OAuth callbacks:** Google, GitHub, and other OAuth providers require HTTPS redirect URIs in production. The OAuth flow in `internal/auth/` generates callback URLs but has no mechanism to produce `https://` URLs.

### What This Is NOT

This is **not** a request to add TLS to the Go server itself. TLS termination at the reverse proxy is the correct, standard approach for self-hosted Docker applications. The gap is entirely in documentation and deployment guidance.

### Concrete Impact

| Flow | Impact Without TLS |
|------|-------------------|
| Telegram webhooks | Cannot receive webhook pushes — must use polling (undocumented limitation) |
| OAuth callbacks | Cannot complete OAuth flows with providers that require HTTPS redirect URIs |
| API access from other hosts | Credentials sent in plaintext if accessed beyond localhost |
| Browser extension / PWA | Cannot connect securely if deployed on a different host from the browser |

---

## Fix

Create a deployment guide section documenting reverse proxy setup for TLS termination. This is documentation-only — no code changes.

### Deliverables

1. **`docs/Deployment.md`** (new file) — Reverse proxy configuration guide covering:
   - Caddy (simplest, automatic HTTPS via Let's Encrypt)
   - nginx (most common, manual cert configuration)
   - Docker Compose integration (reverse proxy as a service alongside the stack)
   - Telegram webhook URL configuration
   - OAuth redirect URI configuration with HTTPS

2. **`docs/smackerel.md` update** — Add a "Deployment" section referencing the new guide. Document the Telegram polling vs. webhook trade-off.

3. **`README.md` update** — Add a "Production Deployment" quick reference linking to `docs/Deployment.md`.

## Regression Test Cases

This is a documentation-only bug. No runtime regression tests apply. Verification:

1. `docs/Deployment.md` exists and contains Caddy and nginx reverse proxy examples
2. `docs/Deployment.md` covers Telegram webhook HTTPS setup
3. `docs/Deployment.md` covers OAuth redirect URI HTTPS configuration
4. `docs/smackerel.md` references the deployment guide
5. `README.md` links to the deployment guide
