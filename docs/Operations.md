# Smackerel Operations Runbook

This guide covers deployment, daily operations, connector management, troubleshooting, backup/restore, and monitoring for a self-hosted Smackerel instance.

## Deployment

### First-Time Setup

1. **Clone the repository:**
   ```bash
   git clone <repo-url> && cd smackerel
   ```

2. **Edit configuration:**
   ```bash
   nano config/smackerel.yaml
   ```
   At minimum, set:
   - `runtime.auth_token` — a secure Bearer token (min 16 chars: `openssl rand -hex 24`)
   - `llm.provider`, `llm.model`, `llm.api_key` — your LLM provider credentials
   - `telegram.bot_token`, `telegram.chat_ids` — if using the Telegram bot

3. **Generate runtime config:**
   ```bash
   ./smackerel.sh config generate
   ```
   This renders `config/generated/dev.env` and `config/generated/test.env` from `config/smackerel.yaml`.

4. **Build Docker images:**
   ```bash
   ./smackerel.sh build
   ```

5. **Start the stack:**
   ```bash
   ./smackerel.sh up
   ```

6. **Verify:**
   ```bash
   ./smackerel.sh status
   ```
   All 4 services (postgres, nats, smackerel-core, smackerel-ml) should show as healthy.

### Configuration Changes

After editing `config/smackerel.yaml`, always regenerate and restart:

```bash
./smackerel.sh config generate
./smackerel.sh down && ./smackerel.sh up
```

**Never edit files under `config/generated/` directly.** They are derived artifacts regenerated from `config/smackerel.yaml`.

### Upgrading

```bash
git pull
./smackerel.sh build
./smackerel.sh down && ./smackerel.sh up
```

Database migrations run automatically on startup. The migration runner uses PostgreSQL advisory locks to prevent concurrent migration attempts.

### Pre-built Image Deployment

Tagged releases publish images to GitHub Container Registry (GHCR). To deploy from pre-built images instead of building from source:

1. **Set image override variables** in your environment or `config/smackerel.yaml`:
   ```bash
   export SMACKEREL_CORE_IMAGE=ghcr.io/<owner>/smackerel-core:v1.0.0
   export SMACKEREL_ML_IMAGE=ghcr.io/<owner>/smackerel-ml:v1.0.0
   ```

2. **Pull and start:**
   ```bash
   ./smackerel.sh config generate
   ./smackerel.sh up
   ```
   Compose pulls the pre-built images instead of building from source.

3. **Rollback** to a previous version:
   ```bash
   export SMACKEREL_CORE_IMAGE=ghcr.io/<owner>/smackerel-core:v0.9.0
   export SMACKEREL_ML_IMAGE=ghcr.io/<owner>/smackerel-ml:v0.9.0
   ./smackerel.sh down && ./smackerel.sh up
   ```

When `SMACKEREL_CORE_IMAGE` and `SMACKEREL_ML_IMAGE` are unset (the default), Compose builds from local Dockerfiles as before.

## Stack Lifecycle

| Action | Command |
|--------|---------|
| Start all services | `./smackerel.sh up` |
| Stop all services | `./smackerel.sh down` |
| Check service health | `./smackerel.sh status` |
| View logs | `./smackerel.sh logs` |
| Rebuild images | `./smackerel.sh build` |
| Clean unused Docker resources | `./smackerel.sh clean smart` |
| Full Docker cleanup | `./smackerel.sh clean full` |
| Measure Docker disk usage | `./smackerel.sh clean measure` |

### Health Check Endpoint

```bash
curl http://127.0.0.1:40001/api/health
```

Returns JSON with status for: API, PostgreSQL, NATS, ML sidecar, Telegram bot, Ollama (if enabled), and knowledge layer stats.

### Prometheus Metrics

Available at `http://127.0.0.1:40001/metrics` (unauthenticated, standard Prometheus scrape endpoint).

## Connector Management

Connectors run on 5-minute sync cycles managed by the connector supervisor. Each connector maintains a cursor in the `sync_state` table for incremental syncing.

### Check Connector Status

```bash
# Via CLI
./smackerel.sh status

# Via API
curl -H "Authorization: Bearer <token>" http://127.0.0.1:40001/api/health
```

The health endpoint reports per-connector status: `healthy`, `syncing`, `error`, or `disconnected`.

### Trigger Immediate Sync

Via the web UI:
1. Open `http://127.0.0.1:40001/settings`
2. Click "Sync Now" next to the connector

Via API:
```bash
curl -X POST -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/settings/connectors/<connector-name>/sync
```

### Enable/Disable a Connector

1. Edit `config/smackerel.yaml` → set `connectors.<name>.enabled: true|false`
2. Regenerate config: `./smackerel.sh config generate`
3. Restart: `./smackerel.sh down && ./smackerel.sh up`

### Reset a Connector's Sync Cursor

If a connector is stuck or you want to re-sync from scratch, clear its cursor in the database. This requires the stack to be running:

```sql
-- Connect to PostgreSQL
-- psql "postgres://smackerel:smackerel@127.0.0.1:42001/smackerel"

-- View current cursors
SELECT connector_id, cursor, last_sync_at FROM sync_state;

-- Reset a specific connector
UPDATE sync_state SET cursor = '' WHERE connector_id = '<connector-name>';
```

The connector will re-sync from the beginning on its next cycle. Existing artifacts are deduplicated by content hash, so duplicates are not created.

### Import Bookmarks

```bash
# Via API
curl -X POST -H "Authorization: Bearer <token>" \
  -F "file=@bookmarks.json" \
  http://127.0.0.1:40001/api/bookmarks/import
```

Or via the web UI at `http://127.0.0.1:40001/settings` → Import Bookmarks.

## Troubleshooting

### Error Lookup Table

| Error Message | Cause | Resolution |
|---------------|-------|------------|
| `NATS connection refused` | NATS container not running or not healthy | Run `./smackerel.sh up` and wait for health checks. Check `./smackerel.sh logs` for NATS errors |
| `ping database: connection refused` | PostgreSQL not running or not ready | Run `./smackerel.sh up`. PostgreSQL has a 5-second health check interval — wait for it to become healthy |
| `execute migration NNN: ...` | Migration SQL error — schema conflict or corrupt state | Check the specific migration file in `internal/db/migrations/`. Look for conflicting manual schema changes |
| `ML sidecar unhealthy` | Python ML sidecar not ready (120s startup period for model loading) | Wait 2 minutes after startup. Check `./smackerel.sh logs` for ML sidecar errors. Verify LLM API key is set |
| `LLM call failed: timeout` | LLM provider rate limit or network issue | Check `LLM_API_KEY` in config. Verify provider status. For Ollama, ensure the model is downloaded |
| `SMACKEREL_AUTH_TOKEN rejected: known placeholder` | Auth token is still set to the default placeholder value | Set a real token in `config/smackerel.yaml` → `runtime.auth_token` and regenerate config |
| `missing required configuration: ...` | One or more required environment variables not set | Run `./smackerel.sh config generate` to regenerate env files from `config/smackerel.yaml`. Check that all required fields are populated |
| `bookmarks connector not connected: import directory not configured` | Bookmarks connector enabled but import directory doesn't exist | Create `data/bookmarks-import/` or disable the connector |
| `acquire migration lock` | Another instance is running migrations concurrently | Wait for the other instance to finish. If stuck, check for leaked advisory locks in PostgreSQL |
| `401 Unauthorized` | Missing or invalid Bearer token in API request | Include `Authorization: Bearer <token>` header. Token must match `runtime.auth_token` in config |
| `SSRF: blocked request to private IP` | Capture endpoint received a URL pointing to a private/internal IP | This is a security protection. Only public URLs are allowed for capture |
| `token endpoint returned 4xx for google` | OAuth2 token exchange failed — expired or revoked credentials | Re-authorize at `http://127.0.0.1:40001/auth/google/start`. Check client_id and client_secret in config |
| `synthesis subscriber: create consumer` | NATS stream not created or misconfigured | Check NATS config in `config/generated/nats.conf`. Restart NATS: `./smackerel.sh down && ./smackerel.sh up` |

### Checking Logs

```bash
# All services
./smackerel.sh logs

# Specific service (read-only, allowed per terminal discipline)
docker logs smackerel-core 2>&1
docker logs smackerel-ml 2>&1
docker logs smackerel-postgres 2>&1
docker logs smackerel-nats 2>&1
```

### Service Won't Start

1. Check config: `./smackerel.sh check`
2. Regenerate config: `./smackerel.sh config generate`
3. Check logs: `./smackerel.sh logs`
4. Clean and rebuild: `./smackerel.sh clean smart && ./smackerel.sh build && ./smackerel.sh up`

### ML Sidecar Slow to Start

The ML sidecar loads the `all-MiniLM-L6-v2` embedding model on startup. This takes up to 120 seconds (the `start_period` in docker-compose.yml). The core service waits for ML readiness before processing artifacts (configurable via `ml_readiness_timeout_s` in `config/smackerel.yaml`).

### NATS JetStream Issues

Check NATS monitoring dashboard at `http://127.0.0.1:42003` for stream and consumer status. Verify streams exist:

```bash
curl http://127.0.0.1:42003/jsz
```

## Backup & Restore

### PostgreSQL Backup

```bash
# Backup (while stack is running)
docker exec smackerel-postgres pg_dump -U smackerel smackerel > backup.sql

# Or compressed
docker exec smackerel-postgres pg_dump -U smackerel -Fc smackerel > backup.dump
```

### PostgreSQL Restore

```bash
# Stop the stack
./smackerel.sh down

# Start only PostgreSQL
docker compose -p smackerel up -d postgres

# Wait for PostgreSQL to be ready, then restore
docker exec -i smackerel-postgres psql -U smackerel smackerel < backup.sql

# Or from compressed backup
docker exec -i smackerel-postgres pg_restore -U smackerel -d smackerel < backup.dump

# Restart full stack
./smackerel.sh up
```

### Volume Backup

Docker named volumes store persistent data. To back up:

```bash
# List volumes
docker volume ls | grep smackerel

# Backup a volume
docker run --rm -v smackerel-dev-postgres-data:/data -v $(pwd):/backup alpine \
  tar czf /backup/postgres-data.tar.gz -C /data .

# Restore a volume
docker run --rm -v smackerel-dev-postgres-data:/data -v $(pwd):/backup alpine \
  sh -c "rm -rf /data/* && tar xzf /backup/postgres-data.tar.gz -C /data"
```

### What to Back Up

| Data | Location | Frequency |
|------|----------|-----------|
| PostgreSQL database | `smackerel-dev-postgres-data` volume | Daily |
| Configuration | `config/smackerel.yaml` | On change |
| Import data | `data/bookmarks-import/`, `data/maps-import/`, `data/twitter-archive/` | On change |
| NATS JetStream | `smackerel-dev-nats-data` volume | Weekly (messages are replayable) |

## Monitoring

### Health Checks

All containers have health checks configured in docker-compose.yml:

| Service | Health Check | Interval | Start Period |
|---------|-------------|----------|-------------|
| PostgreSQL | `pg_isready` | 5s | — |
| NATS | HTTP `/healthz` | 5s | 5s |
| smackerel-core | HTTP `/api/health` | 10s | 15s |
| smackerel-ml | HTTP `/health` | 10s | 120s |
| ollama (optional) | HTTP `/api/tags` | 10s | 30s |

### Key Metrics

The `/metrics` endpoint exposes Prometheus-format metrics:

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `smackerel_artifacts_ingested_total` | Counter | `source`, `type` | Artifact ingestion by source and type |
| `smackerel_capture_total` | Counter | `source` | Capture request count |
| `smackerel_search_latency_seconds` | Histogram | `mode` | Search latency distribution |
| `smackerel_domain_extraction_total` | Counter | `schema`, `status` | Domain extraction attempts |
| `smackerel_connector_sync_total` | Counter | `connector`, `status` | Connector sync success/failure |
| `smackerel_nats_deadletter_total` | Counter | `stream` | Messages routed to dead letter |
| `smackerel_db_connections_active` | Gauge | — | Active database connections |

### NATS Monitoring

The NATS monitoring endpoint at `http://127.0.0.1:42003` provides:
- `/varz` — general server info
- `/connz` — connection details
- `/jsz` — JetStream stream and consumer status
- `/healthz` — health status

## TLS Setup

Smackerel services bind to `127.0.0.1` by default (localhost only). To expose the stack over a network with HTTPS, use a reverse proxy.

### Caddy (Recommended — Automatic HTTPS)

Caddy automatically obtains and renews TLS certificates from Let's Encrypt.

1. [Install Caddy](https://caddyserver.com/docs/install)

2. Create a `Caddyfile`:

```
smackerel.example.com {
    # API and Web UI
    reverse_proxy 127.0.0.1:40001

    # Security headers (Caddy adds most by default)
    header {
        X-Frame-Options "DENY"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "strict-origin-when-cross-origin"
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
- Renews certificates before expiry (30 days before)

### nginx (Alternative)

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

        # WebSocket support (if needed in future)
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

3. Enable the site and obtain a certificate:
```bash
sudo ln -s /etc/nginx/sites-available/smackerel /etc/nginx/sites-enabled/
sudo certbot --nginx -d smackerel.example.com
sudo systemctl reload nginx
```

Certbot automatically configures HTTPS and sets up a renewal cron job.

### Which Ports to Expose

| Port | Service | Expose Externally? |
|------|---------|-------------------|
| 40001 | smackerel-core (API + Web UI) | **Yes** — via reverse proxy only |
| 40002 | smackerel-ml (ML sidecar) | **No** — internal only |
| 42001 | PostgreSQL | **No** — internal only |
| 42002 | NATS client | **No** — internal only |
| 42003 | NATS monitoring | **No** — internal only |
| 42004 | Ollama | **No** — internal only |

Only the core API (port 40001) should be exposed through the reverse proxy. All other services must remain on localhost.

### Certificate Renewal

- **Caddy**: Automatic. No action needed.
- **certbot/nginx**: Certbot installs a systemd timer or cron job that runs `certbot renew` twice daily. Verify with:
  ```bash
  sudo certbot renew --dry-run
  ```

### OAuth Callback URL Update

If switching from `http://127.0.0.1:40001` to `https://smackerel.example.com`, update:

1. `config/smackerel.yaml` → `oauth.google.redirect_url`:
   ```yaml
   oauth:
     google:
       redirect_url: "https://smackerel.example.com/auth/google/callback"
   ```
2. Google Cloud Console → Authorized redirect URIs
3. Regenerate config: `./smackerel.sh config generate`
4. Restart: `./smackerel.sh down && ./smackerel.sh up`

## Expense Tracking Configuration

Expense tracking captures receipts from email, photos, and PDFs, classifies them using a 7-level rule chain, and supports CSV export.

### Enabling Expense Tracking

Set `features.expense_tracking.enabled: true` in `config/smackerel.yaml` and configure:

```yaml
features:
  expense_tracking:
    enabled: true
    categories:
      - groceries
      - dining
      - transport
      - utilities
      - entertainment
      - health
      - travel
      - home
      - other
    business_vendors:
      - "Amazon Web Services"
      - "Google Cloud"
    labels:
      needs_review: "needs-review"
      personal: "personal"
      business: "business"
```

Regenerate config and restart after changes:
```bash
./smackerel.sh config generate
./smackerel.sh down && ./smackerel.sh up
```

The expense API provides 7 endpoints: query, export CSV, correction, classification, suggestions, receipt confirmation, and vendor normalization.

## Meal Planning Configuration

Meal planning provides weekly plans with slot assignment, shopping list generation, and optional CalDAV calendar sync.

### Enabling Meal Planning

Set `features.meal_planning.enabled: true` in `config/smackerel.yaml` and configure:

```yaml
features:
  meal_planning:
    enabled: true
    meal_types:
      - breakfast
      - lunch
      - dinner
      - snack
    meal_times:
      breakfast: "08:00"
      lunch: "12:00"
      dinner: "18:30"
      snack: "15:00"
    default_servings: 2
    caldav:
      enabled: false
      url: ""
      username: ""
      password: ""
      calendar_name: "Meal Plans"
```

The meal plan API provides 12 endpoints for creating, querying, assigning recipes to slots, copying plans, generating shopping lists, and syncing to CalDAV.

## Recipe Features

### Cook Session Timeout

Cook mode provides a step-by-step Telegram walkthrough for any recipe. Sessions time out after the configured duration (default 2 hours):

```yaml
features:
  recipes:
    cook_session_timeout: "2h"
```

### Serving Scaler

The serving scaler adjusts ingredient quantities for any serving count. Fractions are formatted as kitchen-friendly values (e.g., ½, ¾, ⅓). Scaling is available via the recipe API endpoint and Telegram commands.

## Troubleshooting — New Features

### Expenses Not Showing Up

| Check | Resolution |
|-------|------------|
| Feature enabled? | Verify `features.expense_tracking.enabled: true` in `config/smackerel.yaml` |
| Config regenerated? | Run `./smackerel.sh config generate` after config changes |
| Receipt extraction working? | Check ML sidecar logs: `docker logs smackerel-ml 2>&1`. The `receipt-extraction-v1` prompt contract must be present in `/app/prompt_contracts/` |
| Vendor not recognized? | Vendor normalization uses an LRU cache with pre-seeded aliases. Unknown vendors appear as "needs-review" until corrected |

### Meal Plan Slots Fail

| Check | Resolution |
|-------|------------|
| Feature enabled? | Verify `features.meal_planning.enabled: true` in `config/smackerel.yaml` |
| Overlapping plans? | Plans with overlapping date ranges are rejected. Check existing plans via `GET /api/meal-plans` |
| Recipe not found? | Slots require a valid recipe artifact ID. Verify the recipe exists in the system |
| CalDAV sync failing? | Check CalDAV URL, credentials, and network connectivity. CalDAV sync is optional and does not block plan creation |

### Cook Mode Timeout

| Check | Resolution |
|-------|------------|
| Session expired? | Default timeout is 2 hours. Adjust `features.recipes.cook_session_timeout` in config |
| No active session? | Start a cook session via Telegram command. Sessions are per-user and per-recipe |
| Bot not responding? | Check Telegram bot token and that the bot is receiving webhook updates |
