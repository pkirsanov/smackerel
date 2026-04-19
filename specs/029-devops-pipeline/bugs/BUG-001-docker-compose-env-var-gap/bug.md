# BUG-001: Docker Compose Environment Variable Gap

**Finding ID:** RT-001
**Severity:** CRITICAL (P0)
**Classification:** Bug — deployment defect in existing 029 infrastructure
**Discovered:** 2026-04-19 (system review)
**Parent Spec:** [029-devops-pipeline](../../spec.md)

---

## Problem Statement

The `smackerel-core` service in `docker-compose.yml` declares individual `KEY: ${KEY}` environment variables. This list has drifted out of sync with `scripts/commands/config.sh`, which generates the canonical `config/generated/dev.env`. **52+ environment variables** emitted by the config generator are absent from the Compose file's environment block.

This means every feature added since the initial Compose wiring — expense tracking (spec 034), recipe cook mode (spec 035), meal planning (spec 036), GuestHost connector (spec 013), Telegram assembly (spec 008), OTEL observability (spec 030), and more — is **non-functional in containerized deployment** because their config never reaches the container.

## Impact

| Dimension | Impact |
|-----------|--------|
| **Users** | Self-hosted operator cannot use any post-foundation feature after `./smackerel.sh up` |
| **Features broken** | Expense tracking, meal planning, recipe scaler/cook mode, GuestHost connector, Telegram assembly/disambiguation/cook sessions, OTEL, weather extended config, financial markets FRED series |
| **Blast radius** | Every containerized deployment since specs 008+ were implemented |
| **Workaround** | None — individual env declarations are the only path into the container |

## Root Cause

The `docker-compose.yml` `smackerel-core.environment` block was manually maintained as a list of individual `KEY: ${KEY}` declarations. Each new feature added its env vars to `config/smackerel.yaml` → `scripts/commands/config.sh` → `config/generated/dev.env`, but nobody added matching entries to `docker-compose.yml`. There is no automated guard to detect this drift.

## Missing Variables (52+ for smackerel-core)

### Telegram Assembly & Cook Mode (6 vars)
- `TELEGRAM_ASSEMBLY_WINDOW_SECONDS`
- `TELEGRAM_ASSEMBLY_MAX_MESSAGES`
- `TELEGRAM_MEDIA_GROUP_WINDOW_SECONDS`
- `TELEGRAM_DISAMBIGUATION_TIMEOUT_SECONDS`
- `TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES`
- `TELEGRAM_COOK_SESSION_MAX_PER_CHAT`

### Expense Tracking (16 vars)
- `EXPENSES_ENABLED`, `EXPENSES_DEFAULT_CURRENCY`
- `EXPENSES_EXPORT_MAX_ROWS`, `EXPENSES_EXPORT_QB_DATE_FORMAT`, `EXPENSES_EXPORT_STD_DATE_FORMAT`
- `EXPENSES_SUGGESTIONS_MIN_CONFIDENCE`, `EXPENSES_SUGGESTIONS_MIN_PAST_BUSINESS`, `EXPENSES_SUGGESTIONS_MAX_PER_DIGEST`, `EXPENSES_SUGGESTIONS_RECLASSIFY_BATCH_LIMIT`
- `EXPENSES_VENDOR_CACHE_SIZE`
- `EXPENSES_DIGEST_MAX_WORDS`, `EXPENSES_DIGEST_NEEDS_REVIEW_LIMIT`, `EXPENSES_DIGEST_MISSING_RECEIPT_LOOKBACK_DAYS`
- `IMAP_EXPENSE_LABELS`, `EXPENSES_BUSINESS_VENDORS`, `EXPENSES_CATEGORIES`

### Meal Planning (10 vars)
- `MEAL_PLANNING_ENABLED`, `MEAL_PLANNING_DEFAULT_SERVINGS`, `MEAL_PLANNING_MEAL_TYPES`
- `MEAL_PLANNING_MEAL_TIME_BREAKFAST`, `MEAL_PLANNING_MEAL_TIME_LUNCH`, `MEAL_PLANNING_MEAL_TIME_DINNER`, `MEAL_PLANNING_MEAL_TIME_SNACK`
- `MEAL_PLANNING_CALENDAR_SYNC`, `MEAL_PLANNING_AUTO_COMPLETE`, `MEAL_PLANNING_AUTO_COMPLETE_CRON`

### GuestHost Connector (5 vars)
- `GUESTHOST_ENABLED`, `GUESTHOST_BASE_URL`, `GUESTHOST_API_KEY`, `GUESTHOST_SYNC_SCHEDULE`, `GUESTHOST_EVENT_TYPES`

### Observability (2 vars)
- `OTEL_ENABLED`, `OTEL_EXPORTER_ENDPOINT`

### Weather Extended (3 vars)
- `WEATHER_ENABLE_ALERTS`, `WEATHER_FORECAST_DAYS`, `WEATHER_PRECISION`

### Gov Alerts (1 var)
- `GOV_ALERTS_SOURCE_EARTHQUAKE`

### Financial Markets (2 vars)
- `FINANCIAL_MARKETS_FRED_ENABLED`, `FINANCIAL_MARKETS_FRED_SERIES`

### Dual URLs (2 vars)
- `CORE_EXTERNAL_URL`, `ML_EXTERNAL_URL`

### Infrastructure (5 vars)
- `PROJECT_NAME`, `COMPOSE_PROJECT`, `ENABLE_OLLAMA`, `HOST_BIND_ADDRESS`, `OLLAMA_MODEL`

### smackerel-ml Missing Vars
- `OLLAMA_MODEL`, `EMBEDDING_MODEL`, `LOG_LEVEL` — and potentially others as the ML sidecar env block is minimal (8 entries vs the full config surface)

## Reproduction Steps

```bash
# 1. Generate config
./smackerel.sh config generate

# 2. Count vars emitted by config generator
grep -c '=' config/generated/dev.env   # Expect ~140+

# 3. Count individual env declarations in docker-compose.yml for smackerel-core
grep -c ': \${' docker-compose.yml | head -n1  # Expect ~90 for core

# 4. Diff the two lists
diff <(grep -oP '^\w+(?==)' config/generated/dev.env | sort) \
     <(awk '/smackerel-core:/,/smackerel-ml:/' docker-compose.yml | grep -oP '^\s+(\w+):' | sed 's/[: ]//g' | sort)
# Many vars in dev.env not in docker-compose.yml
```

## Fix Design

**Replace individual env declarations with `env_file:` directive** (see companion SM-001 below).

This is a two-part fix:
1. **Immediate**: Add `env_file: config/generated/dev.env` to `smackerel-core` and `smackerel-ml` services
2. **Cleanup**: Remove the entire `environment:` block of individual declarations (100+ lines) from both services, keeping only build args

This eliminates the maintenance gap permanently — any new var added to `config/smackerel.yaml` automatically flows through `config generate` → `dev.env` → `env_file` → container.

### SST Compliance

This fix aligns with the SST pipeline architecture:
```
config/smackerel.yaml → config generate → config/generated/dev.env → env_file in Compose → container
```

No manual `KEY: ${KEY}` declarations needed. The generated env file IS the contract.

### docker-compose.yml Changes (Pseudocode)

```yaml
# BEFORE (smackerel-core):
services:
  smackerel-core:
    environment:
      DATABASE_URL: ${DATABASE_URL}
      NATS_URL: ${NATS_URL}
      # ... 100+ individual declarations
    # ...

# AFTER:
services:
  smackerel-core:
    env_file: config/generated/dev.env
    # ...
```

Same pattern for `smackerel-ml`.

### Guard Against Future Drift

Add a CI check (or `./smackerel.sh check` enhancement) that verifies:
1. `config/generated/dev.env` exists and is not stale
2. No individual `environment:` declarations exist in `docker-compose.yml` for SST-managed services (core, ml)

## Testing

| Test | Method |
|------|--------|
| Env vars reach container | `./smackerel.sh up` then `docker exec smackerel-core env | grep EXPENSES_ENABLED` |
| All features functional | E2E smoke test for expense, meal planning, cook mode endpoints |
| No regression | `./smackerel.sh test unit` and `./smackerel.sh test integration` still pass |
| Config freshness | `./smackerel.sh config generate` followed by `./smackerel.sh up` succeeds |

## Related Findings

- **SM-001** (LOW): This bug's fix IS the env_file migration — they are the same work item
- **DO-002**: CI build on PRs (separate finding)
- **DO-003**: GHCR push (separate finding)
