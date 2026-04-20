# BUG-001 Report: Docker Compose Environment Variable Gap

**Parent:** [029-devops-pipeline](../../spec.md)
**Bug:** [bug.md](bug.md)
**Severity:** CRITICAL (P0)
**Status:** Fixed

---

## Summary

Replaced 100+ individual `KEY: ${KEY}` environment declarations in `docker-compose.yml` with `env_file: config/generated/dev.env` for both `smackerel-core` and `smackerel-ml` services. Added env_file drift guard to `./smackerel.sh check`.

## Changes

| File | Change |
|------|--------|
| `docker-compose.yml` | Replaced individual env declarations with `env_file:` directive for core and ML services. Kept only container-internal path overrides in `environment:` block. |
| `smackerel.sh` | Added env_file drift guard to `check` command — verifies `env_file:` exists and no SST-managed vars are individually declared. |

## Verification

| Check | Result |
|-------|--------|
| `./smackerel.sh check` | PASS — "Config is in sync with SST" + "env_file drift guard: OK" |
| `./smackerel.sh test unit` (Go) | PASS — all packages |
| `./smackerel.sh test unit` (Python) | PASS — 214 passed, 0 failed |
| dev.env variable count | 140+ variables generated from SST |
| Drift guard adversarial | Guard correctly rejects individual SST-managed var declarations |

## Impact

All features added since the initial Compose wiring — expense tracking (034), recipe cook mode (035), meal planning (036), GuestHost connector (013), Telegram assembly (008), OTEL observability (030), and others — now receive their configuration in containerized deployment via `env_file`.
