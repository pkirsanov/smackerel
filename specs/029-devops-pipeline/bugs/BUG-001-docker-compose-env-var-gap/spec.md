# Spec: BUG-001 — Docker Compose Environment Variable Gap

**Parent:** [029-devops-pipeline](../../spec.md)
**Bug:** [bug.md](bug.md)
**Severity:** CRITICAL (P0)
**Status:** in_progress

---

## Problem

`docker-compose.yml` previously listed individual `KEY: ${KEY}` env declarations under `smackerel-core` and `smackerel-ml`. That hand-maintained list drifted out of sync with `config/generated/dev.env` produced by `./smackerel.sh config generate`, so 50+ SST-managed variables never reached the containers and post-foundation features (expense tracking, meal planning, recipe cook mode, GuestHost, Telegram assembly, OTEL, etc.) were silently broken in containerized deployment.

## Goal

The `smackerel-core` and `smackerel-ml` services consume the SST-generated env file as a single source of truth, individual SST-managed env declarations are removed, and `./smackerel.sh check` rejects any future regression.

## Acceptance Criteria

- `smackerel-core` declares `env_file: config/generated/dev.env` and only retains container-internal path overrides in `environment:`.
- `smackerel-ml` declares `env_file: config/generated/dev.env` and only retains container-internal path overrides in `environment:`.
- `./smackerel.sh check` fails when `env_file:` is missing from `docker-compose.yml` or when SST-managed variables are individually declared on the affected services.
- `./smackerel.sh check` passes against the current tree without warnings about env drift.
- No regressions in `./smackerel.sh test unit` (Go and Python).

## Non-Goals

- Re-architecting the SST pipeline.
- Changing variable names or adding new config fields.
- Production-only secrets management (covered by other 020-/029- work items).

## Evidence Anchors

- `docker-compose.yml` line 77 / line 130: `env_file: config/generated/dev.env` for the two services.
- `smackerel.sh` lines 139–176: env_file drift guard inside `./smackerel.sh check`.
- `bug.md` (companion in this folder): full root-cause analysis and missing-variable inventory.

## Status Note

This spec is recorded at `in_progress`. Although the implementation appears present in `docker-compose.yml` and `smackerel.sh`, the bug folder has not been independently re-validated and certified in this artifact pass; promotion to `done` is deferred until each DoD item in `scopes.md` is re-checked with captured evidence.
