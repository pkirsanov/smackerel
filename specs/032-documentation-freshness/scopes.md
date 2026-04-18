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

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Implementation Plan

- Run `docker stats --no-stream` with full stack running
- Measure: per-container RAM, total RAM, disk for images
- Add `## System Requirements` section to README.md
- Minimum: 16GB RAM, 15GB disk, Docker 24+, Docker Compose v2
- Recommended: 32GB RAM, 30GB disk

### DoD

- [ ] README.md has System Requirements section
- [ ] RAM values measured from `docker stats`, not estimated
- [ ] Disk values measured from `docker images`, not estimated
- [ ] Docker and Docker Compose version requirements stated

---

## Scope 2: Development.md Update

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Implementation Plan

- List all packages from `go list ./...` output with one-line descriptions
- List all 17 migrations with table/purpose
- List all 7 prompt contracts with type/purpose
- Add new packages: `internal/domain/`, `internal/annotation/`, `internal/list/`
- Document prompt contract format guide for new domain schemas

### DoD

- [ ] Every Go package under internal/ listed with description
- [ ] All 17 migrations listed with purpose
- [ ] All 7 prompt contracts listed with type and purpose
- [ ] Prompt contract format guide for adding new domain schemas

---

## Scope 3: Operations Runbook

**Status:** Not Started
**Priority:** P1
**Depends On:** Scopes 1, 2

### Implementation Plan

- Create `docs/Operations.md`
- Sections: Deployment, Connector Management, Troubleshooting, Backup/Restore
- Error lookup table: 10+ common errors with cause and resolution
- Each documented command verified by running it

### DoD

- [ ] `docs/Operations.md` exists
- [ ] Deployment section: first-time setup, config generate, up/down
- [ ] Connector management: list, restart, disable, trigger sync
- [ ] Error lookup table with 10+ entries
- [ ] All commands verified against real stack

---

## Scope 4: TLS Setup Guide

**Status:** Not Started
**Priority:** P2
**Depends On:** Scope 3

### Implementation Plan

- Add TLS section to Operations.md
- Caddy reverse proxy setup (auto-HTTPS, recommended)
- nginx reverse proxy setup (alternative)
- Document which ports to expose and which to keep localhost-only

### DoD

- [ ] Caddy reverse proxy configuration documented
- [ ] nginx reverse proxy configuration documented
- [ ] Security guidance: which services stay on localhost
- [ ] Browser extension/PWA HTTPS requirement noted (spec 033 coordination)
