# Execution Report: 029 — DevOps Pipeline

Links: [spec.md](spec.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Spec 029 establishes CI/CD pipeline infrastructure: GitHub Actions workflows, Docker image versioning, branch protection documentation, build metadata embedding, ML sidecar optimization, env_file migration, and GHCR image publishing. All 7 scopes completed.

---

## Scope Evidence

### Scope 1 — GitHub Actions CI Workflow
- CI workflow configured for lint, unit test, and build on push/PR. Integration tests on main with PostgreSQL + NATS services.

### Scope 2 — Docker Image Versioning
- Docker images tagged with git SHA and version tag. OCI labels for version, revision, created.

### Scope 3 — Branch Protection Documentation
- `docs/Branch_Protection.md` documents branch protection rules.

### Scope 4 — Build Metadata Embedding
- Build time, git revision, version embedded via ldflags and Docker labels. `/api/health` returns version info.

### Scope 5 — ML Sidecar Image Optimization
- CPU-only torch, multi-stage build, cache pruning. Target <3GB vs previous 8.63GB.

### Scope 6 — Docker Compose env_file Migration
- Replaced 100+ individual env declarations with `env_file: config/generated/dev.env` for both core and ML services. env_file drift guard added to `./smackerel.sh check`.

### Scope 7 — GHCR Image Push on Tagged Releases
- `push-images` CI job pushes to `ghcr.io` on tagged releases. `docker-compose.yml` supports image override via `SMACKEREL_CORE_IMAGE` / `SMACKEREL_ML_IMAGE`. `docs/Operations.md` documents pull-based deployment.

---

## Test-to-Doc Sweep (2026-04-21)

### Findings

| # | Type | Location | Description | Fix |
|---|------|----------|-------------|-----|
| TQ-029-001 | Test gap | `health_test.go` | `TestHealthHandler_VersionAndCommitHash` tested Version and CommitHash but never set or asserted BuildTime — leaving the entire BuildTime response field untested | Added `BuildTime: "2026-04-18T00:00:00Z"` to deps and assertion for `resp.BuildTime` |
| TQ-029-002 | Test gap | `health_test.go` | `TestHealthHandler_VersionHiddenWithoutAuth` and `TestHealthHandler_VersionVisibleWithAuth` verified auth-gated hiding/showing for Version and CommitHash but not BuildTime | Added BuildTime to deps and assertions in both tests |
| TQ-029-003 | Code bug | `smackerel.sh` | env_file drift guard regex `"^\s+DATABASE_URL:|NATS_URL:|..."` had broken alternation — `^\s+` only anchored the first alternative; other vars matched anywhere in any line (latent false-positive risk) | Wrapped alternatives in group: `"^\s+(DATABASE_URL|NATS_URL|...):"` |

### Evidence

| Check | Result |
|-------|--------|
| `go test -race ./internal/api/...` | PASS (all health tests pass with BuildTime assertions) |
| `./smackerel.sh check` | PASS — "Config is in sync with SST" + "env_file drift guard: OK" |
