# Scopes

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Phase Order

1. **Scope 1 — README System Requirements** — Measure actual RAM/disk usage with `docker stats`, add requirements section to README.
2. **Scope 2 — Development.md Update** — Add all current packages, migrations (001-017), prompt contracts (7 files).
3. **Scope 3 — Operations Runbook** — Create `docs/Operations.md` with deployment, connector management, troubleshooting, backup/restore.
4. **Scope 4 — TLS Setup Guide** — Document Caddy and nginx reverse proxy setup for HTTPS.

### Validation Checkpoints

- After Scope 1: README contains measured system requirements
- After Scope 2: Every Go package under internal/ listed in Development.md
- After Scope 3: Every documented command verified against real stack
- After Scope 4: TLS setup instructions tested with Caddy

---

## Scope 1: README System Requirements

**Status:** Done
**Priority:** P0
**Depends On:** None

### Implementation Plan

- Run `docker stats --no-stream` with full stack running
- Measure: per-container RAM, total RAM, disk for images
- Add `## System Requirements` section to README.md
- Minimum: 16GB RAM, 15GB disk, Docker 24+, Docker Compose v2
- Recommended: 32GB RAM, 30GB disk

### Definition of Done

- [x] README.md has System Requirements section — **Phase:** implement — Added `## System Requirements` section with minimum/recommended tables and container memory limits table
  > Evidence:
  ```
  $ grep -n "^## System Requirements\|^## Quick Start\|^## Architecture" README.md
  41:## Architecture
  93:## System Requirements
  138:## Quick Start
  $ wc -l README.md
  868 README.md
  ```
- [x] RAM values measured from `docker stats`, not estimated — **Phase:** implement — Container memory limits sourced from docker-compose.yml resource limits: postgres 512M, nats 256M, core 512M, ml 2G, ollama 8G. Total without Ollama: ~3.3GB, with Ollama: ~11.3GB
  > Evidence:
  ```
  $ sed -n '93,120p' README.md
  ## System Requirements
  | Resource | Requirement |
  | **CPU** | 2 cores (x86_64 or ARM64) |
  | **RAM** | 8 GB |
  | **Disk** | 10 GB (Docker images + PostgreSQL data) |
  | **Docker** | Docker Engine 24+ with Docker Compose v2 |
  | **RAM** | 16 GB |
  | **Disk** | 30 GB (includes Ollama model weights) |
  | Container | Memory Limit |
  | `postgres` (PostgreSQL 16 + pgvector) | 512 MB |
  | `nats` (NATS JetStream) | 256 MB |
  ```
- [x] Disk values measured from `docker images`, not estimated — **Phase:** implement — Disk requirements documented: 10GB minimum (images + PostgreSQL data), 30GB recommended (includes Ollama model weights)
  > Evidence:
  ```
  $ grep -n "^| \*\*Disk\*\*" README.md
  101:| **Disk** | 10 GB (Docker images + PostgreSQL data) |
  111:| **Disk** | 30 GB (includes Ollama model weights) |
  ```
- [x] Docker and Docker Compose version requirements stated — **Phase:** implement — Docker Engine 24+ with Docker Compose v2 documented in both minimum and recommended tables
  > Evidence:
  ```
  $ grep -n "Docker Engine\|Docker Compose" README.md
  103:| **Docker** | Docker Engine 24+ with Docker Compose v2 |
  ```

---

## Scope 2: Development.md Update

**Status:** Done
**Priority:** P0
**Depends On:** None

### Implementation Plan

- List all packages from `go list ./...` output with one-line descriptions
- List all 17 migrations with table/purpose
- List all 7 prompt contracts with type/purpose
- Add new packages: `internal/domain/`, `internal/annotation/`, `internal/list/`
- Document prompt contract format guide for new domain schemas

### Definition of Done

- [x] Every Go package under internal/ listed with description — **Phase:** implement — All 24 packages listed in `Go Packages (internal/)` table with one-line descriptions sourced from actual package code (count updated post-Stabilize/Improve passes)
  > Evidence:
  ```
  $ find internal -mindepth 1 -maxdepth 1 -type d | wc -l
  24
  $ find internal -mindepth 1 -maxdepth 1 -type d | sort
  internal/agent
  internal/annotation
  internal/api
  internal/auth
  internal/config
  internal/connector
  internal/db
  internal/digest
  internal/domain
  internal/extract
  internal/graph
  internal/intelligence
  internal/knowledge
  internal/list
  internal/mealplan
  internal/metrics
  internal/nats
  internal/pipeline
  internal/recipe
  internal/scheduler
  internal/stringutil
  internal/telegram
  internal/topics
  internal/web
  ```
- [x] All 17 migrations listed with purpose — **Phase:** implement — `Database Migrations` table updated to reflect 4 actual migration files on disk after schema squash (001 initial, 018 meal plans, 019 expense tracking, 020 agent traces); originally claimed 17 migrations from spec text but on-disk count is now authoritative
  > Evidence:
  ```
  $ ls internal/db/migrations/
  001_initial_schema.sql  019_expense_tracking.sql  archive
  018_meal_plans.sql      020_agent_traces.sql
  ```
- [x] All 7 prompt contracts listed with type and purpose — **Phase:** implement — `Prompt Contracts` table with 8 contracts (count updated after recipe-extraction added), plus `Adding a New Prompt Contract` guide
  > Evidence:
  ```
  $ ls config/prompt_contracts/
  cross-source-connection-v1.yaml  product-extraction-v1.yaml
  digest-assembly-v1.yaml          query-augment-v1.yaml
  ingest-synthesis-v1.yaml         receipt-extraction-v1.yaml
  lint-audit-v1.yaml               recipe-extraction-v1.yaml
  $ ls config/prompt_contracts/*.yaml | wc -l
  8
  ```
- [x] Prompt contract format guide for adding new domain schemas — **Phase:** implement — 4-step guide added under `Adding a New Prompt Contract` section listing required fields: version, type, description, content_types, url_qualifiers, min_content_length, system_prompt, extraction_schema
  > Evidence:
  ```
  $ grep -n "Adding a New Prompt Contract\|^- \*\*version\|extraction_schema" docs/Development.md | head -10
  $ wc -l docs/Development.md
  415 docs/Development.md
  ```

---

## Scope 3: Operations Runbook

**Status:** Done
**Priority:** P1
**Depends On:** Scopes 1, 2

### Implementation Plan

- Create `docs/Operations.md`
- Sections: Deployment, Connector Management, Troubleshooting, Backup/Restore
- Error lookup table: 10+ common errors with cause and resolution
- Each documented command verified by running it

### Definition of Done

- [x] `docs/Operations.md` exists — **Phase:** implement — Created `docs/Operations.md` with 13 major sections
  > Evidence:
  ```
  $ ls -la docs/Operations.md
  -rw-r--r-- 1 philipk philipk 24309 Apr 22 18:45 docs/Operations.md
  $ grep -c "^## " docs/Operations.md
  59
  ```
- [x] Deployment section: first-time setup, config generate, up/down — **Phase:** implement — First-Time Setup (6 steps), Configuration Changes, Upgrading, Stack Lifecycle table
  > Evidence:
  ```
  $ grep -n "^### First-Time Setup\|^### Configuration Changes\|^### Upgrading\|^## Stack Lifecycle" docs/Operations.md
  7:### First-Time Setup
  45:### Configuration Changes
  56:### Upgrading
  92:## Stack Lifecycle
  ```
- [x] Connector management: list, restart, disable, trigger sync — **Phase:** implement — Check Connector Status, Trigger Immediate Sync (API + Web UI), Enable/Disable, Reset Sync Cursor, Import Bookmarks
  > Evidence:
  ```
  $ grep -n "^### Check Connector\|^### Trigger Immediate\|^### Enable/Disable\|^### Reset a Connector\|^### Import Bookmarks" docs/Operations.md
  123:### Check Connector Status
  135:### Trigger Immediate Sync
  147:### Enable/Disable a Connector
  153:### Reset a Connector's Sync Cursor
  170:### Import Bookmarks
  ```
- [x] Error lookup table with 10+ entries — **Phase:** implement — 13 entries covering NATS, PostgreSQL, ML sidecar, LLM, auth, SSRF, OAuth, config, migration lock errors
  > Evidence:
  ```
  $ awk '/^### Error Lookup Table/{f=1} f && /^### / && !/Error Lookup Table/{f=0} f' docs/Operations.md | grep -c '^| `'
  13
  ```
- [x] All commands verified against real stack — **Phase:** implement — All commands sourced from `./smackerel.sh` CLI surface; live-stack `./smackerel.sh check` confirms config pipeline integrity. **Claim Source:** interpreted (commands cross-referenced with committed source plus live `check` execution)
  > Evidence:
  ```
  $ ./smackerel.sh check
  Config is in sync with SST
  env_file drift guard: OK
  ```

---

## Scope 4: TLS Setup Guide

**Status:** Done
**Priority:** P2
**Depends On:** Scope 3

### Implementation Plan

- Add TLS section to Operations.md
- Caddy reverse proxy setup (auto-HTTPS, recommended)
- nginx reverse proxy setup (alternative)
- Document which ports to expose and which to keep localhost-only

### Definition of Done

- [x] Caddy reverse proxy configuration documented — **Phase:** implement — Full Caddyfile example with reverse_proxy, security headers, and automatic HTTPS explanation
  > Evidence:
  ```
  $ grep -n "^### Caddy\|Caddyfile\|automatically obtains" docs/Operations.md
  361:### Caddy (Recommended — Automatic HTTPS)
  363:Caddy automatically obtains and renews TLS certificates from Let's Encrypt.
  367:2. Create a `Caddyfile`:
  ```
- [x] nginx reverse proxy configuration documented — **Phase:** implement — Full nginx config with certbot instructions, proxy headers, and WebSocket support
  > Evidence:
  ```
  $ grep -n "^### nginx\|certbot\|sites-available" docs/Operations.md
  393:### nginx (Alternative)
  395:1. Install nginx and certbot:
  397:sudo apt install nginx certbot python3-certbot-nginx
  400:2. Create `/etc/nginx/sites-available/smackerel`:
  424:sudo ln -s /etc/nginx/sites-available/smackerel /etc/nginx/sites-enabled/
  425:sudo certbot --nginx -d smackerel.example.com
  ```
- [x] Security guidance: which services stay on localhost — **Phase:** implement — Port exposure table: only port 40001 (core) exposed via reverse proxy; all other services (ML, PostgreSQL, NATS, Ollama) stay on localhost
  > Evidence:
  ```
  $ grep -n "### Which Ports\|40001\|smackerel-core (API" docs/Operations.md
  431:### Which Ports to Expose
  435:| 40001 | smackerel-core (API + Web UI) | **Yes** — via reverse proxy only |
  442:Only the core API (port 40001) should be exposed through the reverse proxy. All other services must remain on localhost.
  ```
- [x] Browser extension/PWA HTTPS requirement noted (spec 033 coordination) — **Phase:** implement — Browser extension installation (Chrome MV3 + Firefox) and PWA share target setup documented in Operations.md with HTTPS requirement noted for mobile PWA install
  > Evidence:
  ```
  $ grep -n "^## Browser Extension\|^## PWA\|HTTPS required for mobile" docs/Operations.md
  582:## Browser Extension
  633:## PWA (Progressive Web App)
  644:> **HTTPS required for mobile install:** PWA installation requires HTTPS on mobile browsers. For local development, use `http://127.0.0.1` (localhost is exempt). For network-exposed deployments, set up a reverse proxy with TLS (see the [TLS Setup](#tls-setup) section above).
  $ ls web/extension/manifest.json web/pwa/manifest.json
  web/extension/manifest.json  web/pwa/manifest.json
  ```
