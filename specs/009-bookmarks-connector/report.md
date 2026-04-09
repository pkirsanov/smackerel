# Report: 009 — Bookmarks Connector

> **Status:** Done

---

## Execution Evidence

### Delivery Lockdown Certification

- **Scopes completed:** 2/2 (Scope 01: Connector Implementation, Config & Registration; Scope 02: URL Dedup, Folder-to-Topic Mapping & Integration)
- **Unit tests:** 26 tests across 3 test files — all pass
- **Lint:** Pass
- **Format:** Pass
- **Check:** Pass

### DevOps Quality Sweep (Round 8 — Stochastic Quality Sweep)

**Date:** 2026-04-09
**Trigger:** devops
**Mode:** devops-to-doc

#### Findings

| ID | Category | Severity | Description | Status |
|----|----------|----------|-------------|--------|
| D001 | Config SST | High | `BOOKMARKS_IMPORT_DIR` not extracted by config generation pipeline — `connectors.bookmarks.import_dir` exists in `smackerel.yaml` but `scripts/commands/config.sh` did not emit it to `config/generated/*.env` | Fixed |
| D002 | Docker Compose | High | `BOOKMARKS_IMPORT_DIR` not passed to `smackerel-core` container — `docker-compose.yml` environment block lacked the variable | Fixed |
| D003 | Docker Volumes | Medium | No volume mount for bookmarks import directory — connector reads host filesystem files but had no bind mount into the Docker container | Fixed |

#### Fixes Applied

**D001 — Config SST Fix** (`scripts/commands/config.sh`):
- Added extraction of `connectors.bookmarks.import_dir` from smackerel.yaml
- Added `BOOKMARKS_IMPORT_DIR` to generated env file template
- Verified: `./smackerel.sh config generate` now emits `BOOKMARKS_IMPORT_DIR=` in `config/generated/dev.env`

**D002 — Docker Compose Environment** (`docker-compose.yml`):
- Added `BOOKMARKS_IMPORT_DIR: ${BOOKMARKS_IMPORT_DIR:+/data/bookmarks-import}` to smackerel-core environment
- Uses `${VAR:+value}` substitution: sets container-side path `/data/bookmarks-import` only when the host path is configured; empty otherwise (connector silently skips startup per existing logic in `main.go`)

**D003 — Docker Volume Mount** (`docker-compose.yml`, `.gitignore`):
- Added read-only bind mount: `${BOOKMARKS_IMPORT_DIR:-./data/bookmarks-import}:/data/bookmarks-import:ro`
- When user configures `import_dir` in `smackerel.yaml`, their host path is mounted into the container
- When unconfigured (default empty), falls back to `./data/bookmarks-import` (empty local dir — gracefully a no-op)
- Added `data/` to `.gitignore` to prevent user import data from being committed

#### Verification Evidence

- `./smackerel.sh config generate` → success, `BOOKMARKS_IMPORT_DIR` present in `config/generated/dev.env` line 44
- `./smackerel.sh lint` → exit code 0
- `./smackerel.sh test unit` → all 24 Go packages pass (including `internal/connector/bookmarks` at 0.108s), 0 failures
