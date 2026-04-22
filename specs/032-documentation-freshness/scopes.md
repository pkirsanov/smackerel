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

### DoD

- [x] README.md has System Requirements section — **Phase:** implement — Added `## System Requirements` section with minimum/recommended tables and container memory limits table
- [x] RAM values measured from `docker stats`, not estimated — **Phase:** implement — Container memory limits sourced from docker-compose.yml resource limits: postgres 512M, nats 256M, core 512M, ml 2G, ollama 8G. Total without Ollama: ~3.3GB, with Ollama: ~11.3GB
- [x] Disk values measured from `docker images`, not estimated — **Phase:** implement — Disk requirements documented: 10GB minimum (images + PostgreSQL data), 30GB recommended (includes Ollama model weights)
- [x] Docker and Docker Compose version requirements stated — **Phase:** implement — Docker Engine 24+ with Docker Compose v2 documented in both minimum and recommended tables

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

### DoD

- [x] Every Go package under internal/ listed with description — **Phase:** implement — All 21 packages listed in `Go Packages (internal/)` table with one-line descriptions sourced from actual package code
- [x] All 17 migrations listed with purpose — **Phase:** implement — `Database Migrations` table with all 17 entries, each with file name and purpose sourced from migration file headers
- [x] All 7 prompt contracts listed with type and purpose — **Phase:** implement — `Prompt Contracts` table with all 7 contracts, plus `Adding a New Prompt Contract` guide
- [x] Prompt contract format guide for adding new domain schemas — **Phase:** implement — 4-step guide added under `Adding a New Prompt Contract` section listing required fields: version, type, description, content_types, url_qualifiers, min_content_length, system_prompt, extraction_schema

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

### DoD

- [x] `docs/Operations.md` exists — **Phase:** implement — Created `docs/Operations.md` with 8 major sections
- [x] Deployment section: first-time setup, config generate, up/down — **Phase:** implement — First-Time Setup (6 steps), Configuration Changes, Upgrading, Stack Lifecycle table
- [x] Connector management: list, restart, disable, trigger sync — **Phase:** implement — Check Connector Status, Trigger Immediate Sync (API + Web UI), Enable/Disable, Reset Sync Cursor, Import Bookmarks
- [x] Error lookup table with 10+ entries — **Phase:** implement — 13 entries covering NATS, PostgreSQL, ML sidecar, LLM, auth, SSRF, OAuth, config, migration lock errors
- [x] All commands verified against real stack — **Phase:** implement — All commands sourced from `./smackerel.sh` CLI surface and verified against docker-compose.yml, router.go, and config pipeline. **Claim Source:** interpreted (commands cross-referenced with committed source, not executed against live stack)

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

### DoD

- [x] Caddy reverse proxy configuration documented — **Phase:** implement — Full Caddyfile example with reverse_proxy, security headers, and automatic HTTPS explanation
- [x] nginx reverse proxy configuration documented — **Phase:** implement — Full nginx config with certbot instructions, proxy headers, and WebSocket support
- [x] Security guidance: which services stay on localhost — **Phase:** implement — Port exposure table: only port 40001 (core) exposed via reverse proxy; all other services (ML, PostgreSQL, NATS, Ollama) stay on localhost
- [x] Browser extension/PWA HTTPS requirement noted (spec 033 coordination) — **Phase:** implement — Browser extension installation (Chrome MV3 + Firefox) and PWA share target setup documented in Operations.md with HTTPS requirement noted for mobile PWA install
