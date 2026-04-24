# BUG-001 Scopes: Docker Compose Environment Variable Gap

**Parent:** [029-devops-pipeline](../../spec.md)
**Bug:** [bug.md](bug.md)

---

## Scope 1: env_file Migration

**Status:** Done

Replace individual `KEY: ${KEY}` environment declarations with `env_file: config/generated/dev.env` for both `smackerel-core` and `smackerel-ml` services.

### Definition of Done
- [x] `smackerel-core` uses `env_file: config/generated/dev.env`
  **Evidence:** docker-compose.yml line 67: `env_file: - config/generated/dev.env`
- [x] `smackerel-ml` uses `env_file: config/generated/dev.env`
  **Evidence:** docker-compose.yml line 101: `env_file: - config/generated/dev.env`
- [x] Individual SST-managed env declarations removed from both services
  **Evidence:** Only container-path overrides remain in `environment:` blocks (PORT, mount paths, PROMPT_CONTRACTS_DIR)
- [x] All 140+ config-generated variables reach the container via env_file
  **Evidence:** `./smackerel.sh config generate` produces dev.env with 140+ entries; env_file loads all of them

## Scope 2: Drift Guard

**Status:** Done

Add env_file drift detection to `./smackerel.sh check` to prevent future regressions.

### Definition of Done
- [x] `./smackerel.sh check` verifies `env_file:` directive exists in docker-compose.yml
  **Evidence:** smackerel.sh line 139: `grep -q 'env_file:' docker-compose.yml`
- [x] `./smackerel.sh check` detects individual SST-managed var declarations
  **Evidence:** smackerel.sh line 144-148: grep for DATABASE_URL, NATS_URL, LLM_API_KEY, etc. as individual declarations
- [x] `./smackerel.sh check` passes clean
  **Evidence:** Command output: "Config is in sync with SST" + "env_file drift guard: OK"

## Scope 3: Regression Validation

**Status:** Done

Verify no regressions from the migration.

### Definition of Done
- [x] `./smackerel.sh test unit` passes (Go + Python)
  **Evidence:** All Go packages pass; Python: 214 passed, 0 failed
- [x] `./smackerel.sh check` passes with no drift detected
  **Evidence:** "Config is in sync with SST" + "env_file drift guard: OK"
- [x] Container-internal path overrides preserved for volume mounts
  **Evidence:** BOOKMARKS_IMPORT_DIR, MAPS_IMPORT_DIR, BROWSER_HISTORY_PATH, TWITTER_ARCHIVE_DIR use `${VAR:+/data/...}` conditional syntax; PROMPT_CONTRACTS_DIR set to `/app/prompt_contracts`
