# BUG-001 Design: Docker Compose Environment Variable Gap

**Parent:** [029-devops-pipeline](../../spec.md)
**Bug:** [bug.md](bug.md)

---

## Root Cause

The `smackerel-core` and `smackerel-ml` services in `docker-compose.yml` used individual `KEY: ${KEY}` environment declarations. This list drifted out of sync with the config generator (`scripts/commands/config.sh`), leaving 52+ environment variables absent from the Compose environment block.

## Fix Design

### Part 1: Replace individual declarations with `env_file:`

Both `smackerel-core` and `smackerel-ml` services use:

```yaml
env_file:
  - config/generated/dev.env
```

This loads ALL variables from the generated env file. The only remaining `environment:` entries are container-internal overrides that cannot come from the env file:

**smackerel-core:**
- `PORT: ${CORE_CONTAINER_PORT}` — container listening port
- `BOOKMARKS_IMPORT_DIR`, `MAPS_IMPORT_DIR`, `BROWSER_HISTORY_PATH`, `TWITTER_ARCHIVE_DIR` — volume mount paths using `${VAR:+/container/path}` conditional syntax

**smackerel-ml:**
- `PROMPT_CONTRACTS_DIR: /app/prompt_contracts` — container volume mount path

### Part 2: Drift guard in `./smackerel.sh check`

The check command verifies:
1. `env_file:` directive exists in `docker-compose.yml`
2. No SST-managed variables (DATABASE_URL, NATS_URL, LLM_API_KEY, etc.) appear as individual declarations

### SST Compliance

```
config/smackerel.yaml → config generate → config/generated/dev.env → env_file → container
```

No manual `KEY: ${KEY}` declarations needed. Any new variable added to `config/smackerel.yaml` automatically flows through the pipeline.
