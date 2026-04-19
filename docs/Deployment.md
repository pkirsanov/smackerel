# Smackerel Production Deployment Guide

This guide covers production deployment concerns: TLS termination, auth token management, Docker Compose overrides, and HTTPS requirements for webhooks and OAuth.

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
