# User Validation: 022 Operational Resilience

## Validation Checklist

- [x] `./smackerel.sh backup` produces a valid, restorable pg_dump file
- [x] Backup with stopped PostgreSQL exits non-zero with clear error message
- [x] POST /api/capture returns HTTP 503 when PostgreSQL is unreachable
- [x] POST /api/capture succeeds normally when PostgreSQL is healthy
- [x] Search returns text-fallback results within 2s when ML sidecar is down
- [x] Search resumes semantic path when ML sidecar recovers
- [x] Overlapping cron jobs of the same type are skipped with warning log
- [x] Different cron job types run concurrently without interference
- [x] Graceful shutdown completes within 30s on SIGTERM
- [x] Shutdown order: scheduler → HTTP → Telegram → subscribers → connectors → NATS → DB
- [x] Exhausted NATS messages route to DEADLETTER stream with metadata
- [x] DB pool MaxConns/MinConns configurable via `config/smackerel.yaml`
- [x] Missing DB pool config causes startup failure (no hardcoded fallback)
- [x] All new config values flow from `config/smackerel.yaml` (SST compliance)
- [x] Docker `stop_grace_period: 30s` set on smackerel-core service

## Sign-Off

**Validated by:** _pending_
**Date:** _pending_
