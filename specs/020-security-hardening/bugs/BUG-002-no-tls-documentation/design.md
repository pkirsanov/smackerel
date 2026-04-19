# Bug Fix Design: [BUG-002] No TLS / Reverse Proxy Documentation

## Design Brief

- **Current State:** No reverse proxy or TLS documentation exists. `docs/smackerel.md` mentions Traefik in a comparison table but provides no setup guide. All services bind to plaintext HTTP on `127.0.0.1`.
- **Target State:** A `docs/Deployment.md` guide covers reverse proxy setup with Caddy and nginx, including Telegram webhook and OAuth redirect URI configuration over HTTPS.
- **Patterns to Follow:** Existing docs (`docs/Development.md`, `docs/Docker_Best_Practices.md`) use Markdown with clear section headers, code blocks for config examples, and tables for reference.
- **Patterns to Avoid:** Do not add TLS to the Go server. Do not add a reverse proxy to `docker-compose.yml` as a default service (it should be documented as an optional addition). Do not hardcode domain names.
- **Resolved Decisions:** Document Caddy (simplest, auto-HTTPS) and nginx (most common). Provide Docker Compose snippets for adding the reverse proxy as an additional service. Use placeholder domain `smackerel.example.com`.
- **Open Questions:** None.

## Root Cause Analysis

### Investigation Summary

System review (TR-002) identified that Telegram webhooks and OAuth callbacks require HTTPS, but no documentation or configuration exists for TLS termination. The 020-security-hardening spec focused on network binding and authentication but did not address the HTTPS requirement for external-facing flows.

### Root Cause

Documentation gap. The architecture correctly uses `127.0.0.1` binding (spec 020 scope 1) which prevents LAN access, but external integrations (Telegram, OAuth) require a public HTTPS endpoint. The standard self-hosted solution — a reverse proxy — was not documented.

### Impact Analysis
- Affected components: `docs/Deployment.md` (new), `docs/smackerel.md` (update), `README.md` (update)
- Affected data: None
- Affected users: Operators setting up Telegram webhooks or OAuth in production
- Blast radius: Zero — documentation only, no runtime changes

## Fix Design

### Deliverable 1: `docs/Deployment.md` (new file)

Structure:

```
# Deployment Guide

## Overview
- Self-hosted Docker Compose deployment
- All services bind to 127.0.0.1 (localhost only)
- Production deployments requiring external access need a reverse proxy

## Reverse Proxy Setup

### Option A: Caddy (Recommended)
- Automatic HTTPS via Let's Encrypt
- Minimal config (~10 lines)
- Docker Compose service snippet
- Caddyfile example proxying to smackerel-core:8080

### Option B: nginx
- Manual cert setup (certbot or similar)
- nginx.conf example with proxy_pass
- Docker Compose service snippet
- SSL certificate volume mounts

## Telegram Webhook Configuration
- Set BOT_WEBHOOK_URL to https://smackerel.example.com/api/telegram/webhook
- Telegram requires HTTPS — no self-signed certs
- Polling mode as alternative (no HTTPS required, higher latency)

## OAuth Redirect URIs
- Configure provider callback URLs with https://smackerel.example.com/auth/{provider}/callback
- Provider-specific notes (Google requires verified domain, GitHub allows localhost for dev)

## Docker Compose Integration
- Add reverse proxy as a service in docker-compose.override.yml
- Network configuration to reach smackerel-core container
- Example docker-compose.override.yml with Caddy
```

### Deliverable 2: `docs/smackerel.md` update

Add a "Deployment" subsection in the architecture section referencing `docs/Deployment.md` and noting the Telegram polling vs. webhook trade-off.

### Deliverable 3: `README.md` update

Add a "Production Deployment" bullet in the documentation links section pointing to `docs/Deployment.md`.

### Scenario-to-Deliverable Mapping

| Scenario | Deliverable | Type |
|----------|-------------|------|
| Caddy reverse proxy setup | `docs/Deployment.md` § Option A | Documentation |
| nginx reverse proxy setup | `docs/Deployment.md` § Option B | Documentation |
| Telegram webhook HTTPS | `docs/Deployment.md` § Telegram | Documentation |
| OAuth redirect URI HTTPS | `docs/Deployment.md` § OAuth | Documentation |
| Architecture reference | `docs/smackerel.md` update | Documentation |
| Quick reference link | `README.md` update | Documentation |
