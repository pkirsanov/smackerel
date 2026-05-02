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
| Stop and remove volumes | `./smackerel.sh down --volumes` |
| Check service health | `./smackerel.sh status` |
| View logs | `./smackerel.sh logs` |
| Rebuild images | `./smackerel.sh build` |
| Backup database | `./smackerel.sh backup` |
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

### QF Decisions Connector Operations

The `qf-decisions` connector is governed as a companion read surface for QuantitativeFinance.

| Operation | Requirement |
|-----------|-------------|
| Credential rotation | Rotate the QF service credential from `config/smackerel.yaml`, regenerate config, and restart the stack |
| Connector health | Treat missing QF packet IDs, trace IDs, calibration badges, or provenance badges as degraded health until corrected upstream |
| Cursor reset | Reset only the `qf-decisions` cursor when replaying QF packets; deduplication must keep packet IDs stable |
| Evidence export | Export `PersonalEvidenceBundle`s only with explicit user consent, source artifact references, sensitivity labels, and provenance metadata |
| Incident response | If Smackerel displays QF packets without required trust metadata or shows action controls, disable the connector before resuming sync |

Smackerel operators must not use this connector to approve trades, alter QF mandates, submit execution requests, or rewrite QF-provided decision content.

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

The CLI provides a one-command backup:

```bash
./smackerel.sh backup
```

This creates a compressed `pg_dump` file in the `backups/` directory.

For manual backups (e.g., custom format or piping to remote storage):

```bash
# Plain SQL (while stack is running)
docker exec smackerel-postgres pg_dump -U smackerel smackerel > backup.sql

# Or compressed custom format
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

#### Go Core (`http://127.0.0.1:40001/metrics`)

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `smackerel_artifacts_ingested_total` | Counter | `source`, `type` | Artifact ingestion by source and type |
| `smackerel_capture_total` | Counter | `source` | Capture request count (telegram, api, extension, pwa) |
| `smackerel_search_latency_seconds` | Histogram | `mode` | Search latency distribution |
| `smackerel_domain_extraction_total` | Counter | `schema`, `status` | Domain extraction attempts |
| `smackerel_connector_sync_total` | Counter | `connector`, `status` | Connector sync success/failure |
| `smackerel_nats_deadletter_total` | Counter | `stream` | Messages routed to dead letter |
| `smackerel_db_connections_active` | Gauge | — | Active database connections |
| `smackerel_digest_generation_total` | Counter | `status` | Digest generation (published, fallback, quiet) |

#### Recommendations (Spec 039 Scope 6)

The recommendation runtime exposes eight Prometheus metrics with bounded labels (no `watch_id`, `recommendation_id`, `request_id`, `trace_id`, or `actor_user_id` labels). Per-watch operator visibility is provided by joining the bounded `*_watch_runs_total` metric with the persisted `recommendation_watch_runs` table on `watch_id` — never by embedding the watch id as a Prometheus label.

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `smackerel_recommendation_provider_requests_total` | Counter | `provider`, `category`, `outcome` | Provider request count by outcome (success, error, timeout, degraded) |
| `smackerel_recommendation_provider_latency_seconds` | Histogram | `provider`, `category` | Provider call latency distribution (buckets 0.05..30s) |
| `smackerel_recommendation_candidates_total` | Counter | `category`, `stage`, `outcome` | Candidate counts at each ranking/dedupe/policy stage |
| `smackerel_recommendation_watch_runs_total` | Counter | `kind`, `outcome` | Watch evaluation runs by kind and outcome |
| `smackerel_recommendation_delivery_total` | Counter | `channel`, `outcome` | Delivery outcomes per channel (telegram, digest, drop) |
| `smackerel_recommendation_suppression_total` | Counter | `reason` | Recommendations suppressed by policy/quiet hours/rate limit |
| `smackerel_recommendation_ranking_confidence_total` | Counter | `confidence` | Distribution of ranking confidence bands |
| `smackerel_recommendation_location_precision_total` | Counter | `requested`, `sent` | Requested vs. sent location precision (privacy reduction) |

**Operator audit view:** `GET /recommendations/watches/{id}` renders a `data-testid="watch-audit-counts"` block sourced from `recommendation_watch_runs` (data-source marker on the section). Use this surface — not Prometheus — for per-watch run counts.

**Log/trace redaction:** All serialized recommendation logs and traces are scanned for forbidden substrings (provider API keys, raw provider payloads, sensitive graph prompt text, raw GPS coordinates) at the persistence boundary via `internal/recommendation/store.AssertRedactSafe`. The unit test `internal/recommendation/store/redact_test.go::TestRecommendationRedaction_NoSecretsOrRawLocationInLogsOrTraces` is the regression guard.

#### ML Sidecar (`http://127.0.0.1:40002/metrics`)

| Metric | Type | Labels | Purpose |
|--------|------|--------|---------|
| `smackerel_llm_tokens_used_total` | Counter | `provider`, `model` | LLM token usage per provider/model |
| `smackerel_ml_processing_latency_seconds` | Histogram | `operation` | ML processing latency per operation |

Model label cardinality is bounded: known models pass through, unknown models map to `other`.

### OpenTelemetry Tracing (Opt-in)

Distributed trace context propagation through NATS messages is available but disabled by default. To enable:

1. Set `observability.otel_enabled: true` in `config/smackerel.yaml`
2. Optionally set `observability.otel_exporter_endpoint` to an OTLP gRPC collector (e.g., `http://localhost:4317`)
3. Regenerate config: `./smackerel.sh config generate`
4. Restart: `./smackerel.sh down && ./smackerel.sh up`

When enabled, trace context is propagated via W3C `traceparent` headers in NATS messages between Go core and ML sidecar. When disabled, there is zero overhead.

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

## Browser Extension

The Smackerel browser extension enables one-click capture of any web page. It supports Chrome (Manifest V3) and Firefox (Manifest V2).

### Chrome Installation

1. Open `chrome://extensions/` in Chrome
2. Enable **Developer mode** (toggle in the top-right corner)
3. Click **Load unpacked**
4. Select the `web/extension/` directory from the Smackerel repository
5. The Smackerel icon appears in the toolbar

### Firefox Installation

1. Open `about:debugging#/runtime/this-firefox` in Firefox
2. Click **Load Temporary Add-on...**
3. Select `web/extension/manifest.firefox.json` from the Smackerel repository
4. The Smackerel icon appears in the toolbar

> **Note:** Firefox temporary add-ons are removed when the browser closes. For persistent installation, use `./smackerel.sh package extension` to create distributable `.zip` files, then install from the packaged file.

### Packaging Extensions for Distribution

To create distributable packages for Chrome and Firefox:

```bash
./smackerel.sh package extension
```

This produces:
- `dist/extension/smackerel-chrome-{version}.zip` — Chrome extension package
- `dist/extension/smackerel-firefox-{version}.zip` — Firefox extension package (with Firefox-specific manifest)

Users can install the Chrome `.zip` by extracting it and loading via **Load unpacked** in `chrome://extensions/`, or by dragging the `.zip` into Chrome. For Firefox, rename the `.zip` to `.xpi` and install from `about:addons`.

### Extension Configuration

After installation, click the Smackerel toolbar icon to open the setup popup:

1. **Server URL** — enter your Smackerel instance URL (e.g., `https://smackerel.example.com` or `http://127.0.0.1:40001` for local dev)
2. **Auth Token** — paste your Bearer auth token (from `runtime.auth_token` in `config/smackerel.yaml`)
3. Click **Test Connection** to verify
4. Click **Save Settings** when the test passes

### Usage

- **Toolbar button:** Click the Smackerel icon → **Save to Smackerel** to capture the current page
- **Context menu:** Right-click any page, link, or image → **Save to Smackerel**
- **Text selection:** Select text on a page → right-click → **Save with selection**
- **Offline queue:** If the server is unreachable, captures are queued locally and synced when connectivity returns

## PWA (Progressive Web App)

The PWA provides a mobile share target so you can send URLs and text to Smackerel from any app's Share menu.

### Installation

1. Open `http://127.0.0.1:40001/pwa/` in a mobile browser (Chrome on Android, Safari on iOS)
   - For HTTPS deployments: `https://smackerel.example.com/pwa/`
2. The browser displays an **Install** prompt (or tap the browser menu → **Add to Home Screen**)
3. Tap **Install** to add Smackerel to your home screen

> **HTTPS required for mobile install:** PWA installation requires HTTPS on mobile browsers. For local development, use `http://127.0.0.1` (localhost is exempt). For network-exposed deployments, set up a reverse proxy with TLS (see the [TLS Setup](#tls-setup) section above).

### Usage

Once installed, Smackerel appears as a share target on your device:

1. In any app (browser, notes, messaging), tap **Share**
2. Select **Smackerel** from the share sheet
3. The URL or text is captured to your Smackerel instance

The PWA uses the Web Share Target API to receive shared content. It posts the shared URL/text to the Smackerel capture endpoint using your configured auth token.

### PWA Troubleshooting

| Check | Resolution |
|-------|------------|
| Install prompt not showing? | Ensure you're on HTTPS (or localhost). Clear browser cache and revisit `/pwa/` |
| Share target not appearing? | The PWA must be installed to the home screen. Reinstall if missing |
| Captures failing? | Verify the Smackerel stack is running and the server URL is reachable from your device |
| Service worker not registering? | Check browser console for errors. The SW scope must match `/pwa/` |

## Cloud Drives Operations (Spec 038)

The cloud-drives surface (Google Drive provider in scope today) is operated through the `/v1/connectors/drive` and `/v1/drive/*` endpoints, the `DRIVE` NATS stream, and the `drive_*` PostgreSQL tables.

### Enabling A Drive Provider

1. Add OAuth credentials to `config/smackerel.yaml` under `drive.providers.<provider_id>` — required keys include `oauth_client_id`, `oauth_client_secret`, `oauth_redirect_url`, `oauth_base_url`, `api_base_url`, scan/monitor intervals, MIME allow-lists, and sensitivity thresholds. Empty secret values fail-loud at startup; do not rely on env fallbacks.
2. Regenerate config: `./smackerel.sh config generate`.
3. Restart: `./smackerel.sh down && ./smackerel.sh up`.
4. Connect a user account by issuing the OAuth web flow:
   ```bash
   curl -X POST -H "Authorization: Bearer <token>" \
     -H "Content-Type: application/json" \
     http://127.0.0.1:40001/v1/connectors/drive/connect \
     -d '{"provider":"google","owner_user_id":"<uuid>","access_mode":"read_only","scope":{"folder_ids":["<id>"]}}'
   ```
   The handler returns an authorization URL; the user authorizes, and the provider redirects back to `/v1/connectors/drive/oauth/callback?state=…&code=…` which calls `FinalizeConnect`. A row lands in `drive_connections` with `status='healthy'`, the provider-supplied `expires_at`, and the bearer token in `credentials_ref`.

### Drive Connection Health

| Endpoint | Purpose |
|----------|---------|
| `GET /v1/connectors/drive` | List provider catalog and capabilities (provider-neutral). |
| `GET /v1/connectors/drive/connection/{id}` | Inspect a single connection (status, scope, last health reason, expires_at). Returns `404 CONNECTION_NOT_FOUND` for unknown ids. |
| `GET /v1/connectors/drive/connection/{id}/skipped` | Group skipped/blocked files by reason for Screen 4. |

**Token expiry behaviour.** Per spec 038 design.md §2.3 + decision-log A1, only the bearer (access) token is persisted; refresh tokens are intentionally not stored in this scope. When `expires_at` passes, the connection moves out of `healthy`; the user must re-authorize through the OAuth flow above. A dedicated credentials vault is a follow-up scope and MUST move both access and refresh tokens out of `credentials_ref` when it lands.

### Save Rules And Save Service

The Save Rules engine (`/v1/drive/rules`) and Save Service (`/v1/drive/save`) gate every provider write. All endpoints require Bearer auth.

| Endpoint | Purpose |
|----------|---------|
| `GET /v1/drive/rules` / `POST /v1/drive/rules` | List or create Save Rules. |
| `GET/PUT/DELETE /v1/drive/rules/{id}` | Inspect, update, or delete a single rule. |
| `POST /v1/drive/rules/{id}/test` | Screen 8 dry-run — evaluate a rule against a candidate artifact without committing. |
| `GET /v1/drive/rules/audit` | Screen 7 audit feed — first-stable-match outcomes plus all conflicting matches per evaluation. |
| `POST /v1/drive/save` | Submit a save request; the rule engine evaluates, the Save Service routes through the provider writer, and a row lands in `drive_save_requests`. |
| `GET /v1/drive/save/requests` | Recent save requests for Screen 7. |

### Low-Confidence Confirmation

Low-confidence routing decisions and sensitive-content saves are paused at the Save Service and surfaced through Screen 11 and the Telegram numbered-reply path. Both channels share one handler so the exactly-once contract holds.

| Endpoint | Purpose |
|----------|---------|
| `GET /v1/drive/confirmations/{id}` | Fetch the pending confirmation payload. |
| `POST /v1/drive/confirmations/{id}` | Resolve a confirmation (`confirm` / `reject`). The first call wins; subsequent calls return the resolved state. |

### Drive Search And Artifact Detail

Drive content participates in the standard `/api/search` semantic surface. The Drive-specific artifact detail endpoint exposes folder context, version history, save provenance, and skipped/blocked grouping:

```bash
curl -H "Authorization: Bearer <token>" \
  http://127.0.0.1:40001/v1/drive/artifacts/<artifact-id>
```

### Resetting A Drive Cursor

Drive providers use `drive_cursors(provider_id, connection_id, cursor, valid_until)`. To force a bulk re-scan after a provider cursor invalidation:

```sql
DELETE FROM drive_cursors WHERE connection_id = '<uuid>';
```

The next monitor cycle falls back to a bounded rescan of in-scope folders, computes a delta against `drive_files`, and re-issues only those deltas as change events.

### Drive Database Tables

`drive_connections`, `drive_oauth_states`, `drive_files`, `drive_folders`, `drive_cursors`, `drive_rules`, `drive_save_requests`, `drive_folder_resolutions`, `drive_rule_audit`, `drive_scan_jobs`, `drive_provider_work_queue`, `drive_confirmations`, `drive_share_changes`, plus the consolidated migrations 021/024/030. Backups produced by `./smackerel.sh backup` cover all of them.

## Cloud Photo Libraries Operations (Spec 040)

The cloud-photos surface (Immich and PhotoPrism providers in scope today) is operated through the `/v1/photos/*` endpoints, the `PHOTOS` NATS stream, and the `photo_*` PostgreSQL tables.

### Enabling A Photo Provider

1. Add provider credentials under `photos.providers.<provider>` in `config/smackerel.yaml` (`base_url` + `access_token` for both Immich and PhotoPrism). Empty `access_token` values fail-loud at startup.
2. Regenerate config: `./smackerel.sh config generate`.
3. Restart: `./smackerel.sh down && ./smackerel.sh up`.
4. Test the connection without persisting it:
   ```bash
   curl -X POST -H "Authorization: Bearer <token>" \
     -H "Content-Type: application/json" \
     http://127.0.0.1:40001/v1/photos/connectors/test \
     -d '{"provider":"immich","base_url":"https://immich.example.com","access_token":"<key>"}'
   ```
5. Connect:
   ```bash
   curl -X POST -H "Authorization: Bearer <token>" \
     -H "Content-Type: application/json" \
     http://127.0.0.1:40001/v1/photos/connectors \
     -d '{"provider":"immich","base_url":"https://immich.example.com","access_token":"<key>"}'
   ```

### Photo Provider Endpoints

All `/v1/photos/*` endpoints require Bearer auth.

| Endpoint | Purpose |
|----------|---------|
| `GET /v1/photos/connectors` | List configured photo providers (Immich, PhotoPrism). |
| `POST /v1/photos/connectors` | Create a new provider connection. |
| `POST /v1/photos/connectors/test` | Test credentials without persisting. |
| `GET /v1/photos/connectors/{id}` | Inspect a single connection. |
| `POST /v1/photos/connectors/capabilities/{capability}/exercise` | Exercise a provider capability; unsupported operations return `409 PROVIDER_LIMITATION` with a stable `LimitationCode`. |
| `GET /v1/photos/health` | Aggregate photo health (sync progress, capability limitations). |
| `GET /v1/photos/search?q=<text>` | Semantic photo search; sensitive results omit preview URLs and set `requires_reveal=true`. |
| `GET /v1/photos/{id}` | Fetch a photo record. |
| `GET /v1/photos/{id}/preview?size=thumb\|full` | Sensitive previews return `403 sensitivity_requires_reveal` without a reveal token. |
| `POST /v1/photos/{id}/reveal` | Mint a single-use, actor-bound, TTL + hash-protected reveal token. |
| `POST /v1/photos/upload` | Unified multipart upload pipeline shared by Telegram, the mobile PWA, and web; preserves `source_channel` + `source_ref`. |

### Lifecycle, Duplicates, And Removal Review

| Endpoint | Purpose |
|----------|---------|
| `GET /v1/photos/health/lifecycle` | RAW-to-processed lifecycle dashboard (editor signature, confidence, rationale, review_state). |
| `GET /v1/photos/health/duplicates` | Duplicate clusters (exact, burst, HDR, panorama, near-duplicate, cross-provider). |
| `GET /v1/photos/health/duplicates/{id}` | Inspect a single cluster. |
| `POST /v1/photos/health/duplicates/{id}/best-pick` | Set the best-pick photo for a cluster. |
| `POST /v1/photos/health/duplicates/{id}/resolve` | Resolve a cluster (keep / archive / delete). |
| `GET /v1/photos/health/removal` | Removal-candidate review with reason + confidence + rationale + method, in reversible decision states. |
| `GET /v1/photos/health/quality` | Quality dashboard (Scope 3 placeholder rows). |

### Action Tokens And Destructive Confirmation

Every destructive write (archive, delete, album removal) MUST flow through plan → confirm:

```bash
# Plan a destructive action — returns a scope-hashed PhotoActionToken.
curl -X POST -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  http://127.0.0.1:40001/v1/photos/actions/plan \
  -d '{"action":"delete","scope":{"photo_ids":["<uuid>"]}}'

# Confirm with the token (and a text-confirmation phrase for delete).
curl -X POST -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  http://127.0.0.1:40001/v1/photos/actions/confirm \
  -d '{"token":"<photo_action_token>","confirmation":"DELETE"}'
```

`ConfirmedWriter` wraps every `ProviderWriter` so a write cannot fire before confirmation. If the planning scope changes between plan and confirm (scope-hash drift), the confirm call is rejected.

### Capability Taxonomy

Provider capability gaps surface through one taxonomy (`internal/connector/photos/capability_taxonomy.go`) used by:

- API responses (`409 PROVIDER_LIMITATION` envelopes carry a `LimitationCode`).
- Prometheus metrics (`smackerel_photos_capabilities_limited_total{code=…}`).
- The PWA Photo Health dashboard banner strings.

The taxonomy canary integration test asserts those three surfaces stay in sync. When adding a new provider, register limitations through the taxonomy registry — never inline ad-hoc strings.

### Sensitivity Reveal Tokens

Sensitive photo previews are gated server-side. The reveal token lifecycle:

1. Search and detail endpoints return `requires_reveal=true` for sensitive rows and omit preview URLs.
2. Calling `GET /v1/photos/{id}/preview` without a reveal token returns `403 sensitivity_requires_reveal`.
3. The user (or surface) requests a token via `POST /v1/photos/{id}/reveal`.
4. The token is single-use, actor-bound, TTL-bounded, and hash-protected; reuse fails closed.
5. Telegram's `handleFind` substitutes a reveal-required notice for sensitive results so the bot does not auto-deliver sensitive content.

### Photo Database Tables

Migration `025_photo_libraries.sql` plus `029_photo_scope3_lifecycle_dedupe_removal.sql` and `031_photo_scope4_capture_routing_sensitivity.sql` provide: `photo_lifecycle_links`, `photo_clusters`, `photo_cluster_members`, `photo_removal_candidates`, `photo_capabilities`, `photo_sync_state`, `photo_face_links`, `photo_embeddings`, `photo_action_tokens`, `photo_audit_events`, `photo_raw_export_links`, `photo_routing_decisions`, `photo_document_groups`. All are covered by the standard `./smackerel.sh backup` and the disposable test stack.
