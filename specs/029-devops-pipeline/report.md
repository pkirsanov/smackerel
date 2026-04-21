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

---

## Security-to-Doc Sweep (2026-04-21)

### Probe Scope

Security probe of spec 029 devops-pipeline implementation: CI workflow, Dockerfiles, docker-compose.yml, config generation, GHCR push, integration test configuration. Checked for supply-chain vulnerabilities, secrets exposure, auth bypass, injection, SSRF, privilege escalation.

### Findings

| # | Severity | Category | Location | Description | Disposition |
|---|----------|----------|----------|-------------|-------------|
| SEC-029-001 | MEDIUM | Supply chain | `.github/workflows/ci.yml` | All GitHub Actions referenced by mutable version tags (`@v4`, `@v5`, `@v3`). A compromised upstream action tag reassignment could inject code into CI. | **FIXED** — Pinned all 8 action references to immutable SHA digests with version comments. |
| SEC-029-002 | LOW | Supply chain | `ml/requirements.txt` | Python packages pinned by version (`==`) but lack `--require-hashes` integrity verification. Design doc specified hash pinning but it was not implemented. | **DOCUMENTED** — Low risk for self-hosted project. Noted as future hardening item. |
| SEC-029-003 | INFO | Docker images | `Dockerfile`, `ml/Dockerfile`, `docker-compose.yml` | Base images pinned by version tag (`golang:1.24.3-alpine`, `python:3.12-slim`, `alpine:3.20`) but not by `@sha256:` digest. Third-party service images (`pgvector/pgvector:pg16`, `nats:2.10-alpine`, `ollama/ollama:0.6`) use minor-version mutable tags. | **ACCEPTED** — Adequate for self-hosted local deployment. Version tags provide reasonable pinning. |
| SEC-029-004 | INFO | CI credentials | `.github/workflows/ci.yml:149` | Integration test uses hardcoded `SMACKEREL_AUTH_TOKEN: ci-test-token-integration` and `POSTGRES_PASSWORD: smackerel`. | **ACCEPTED** — Ephemeral CI service containers with no persistence. Test-only credentials appropriate for this context. |
| SEC-029-005 | INFO | Network | `docker-compose.yml`, `config/generated/dev.env` | PostgreSQL connections use `sslmode=disable`. | **ACCEPTED** — Docker internal network for local dev. Production deployment guide (`docs/Deployment.md`) should recommend TLS for non-local deployments. |

### Positive Security Controls Verified

| Control | Status | Evidence |
|---------|--------|----------|
| Non-root container execution | PASS | Both Dockerfiles create `smackerel` user and set `USER smackerel` |
| `no-new-privileges` security opt | PASS | All services in `docker-compose.yml` have `security_opt: - no-new-privileges:true` |
| Capability dropping | PASS | `nats` and `smackerel-core` services drop ALL capabilities |
| Localhost-only port binding | PASS | All host port bindings use `127.0.0.1:` prefix |
| Minimal CI permissions | PASS | Workflow-level `permissions: contents: read`; `packages: write` only on push-images job |
| GHCR auth via GITHUB_TOKEN | PASS | No additional secrets required; automatic token scoped to repo |
| Go module verification | PASS | CI runs `go mod verify` before build |
| CSRF protection on OAuth | PASS | `auth/handler.go` uses cryptographic state tokens with TTL eviction and rate limiting |
| Resource limits on containers | PASS | All services have `deploy.resources.limits.memory` set |
| Read-only volume mounts | PASS | Data import volumes mounted `:ro` |

### Fix Applied

**SEC-029-001:** Pinned all GitHub Actions in `.github/workflows/ci.yml` to immutable commit SHAs:
- `actions/checkout` → `34e114876b0b11c390a56381ad16ebd13914f8d5` (v4.3.1)
- `actions/setup-go` → `40f1582b2485089dde7abd97c1529aa768e1baff` (v5.6.0)
- `actions/setup-python` → `a26af69be951a213d495a4c3e4e4022e16d87065` (v5.6.0)
- `docker/login-action` → `c94ce9fb468520275223c153574b00df6fe4bcc9` (v3.7.0)

### Evidence

| Check | Result |
|-------|--------|
| `./smackerel.sh check` | PASS — "Config is in sync with SST" + "env_file drift guard: OK" |
| CI YAML action refs | All 8 refs pinned to full 40-char SHA with version comment |
| Positive controls audit | 10/10 security controls verified present |

---

## Reconcile-to-Doc Sweep (2026-04-21)

### Scope

Reconciliation of all 7 scopes' claimed-vs-implemented state. Verified every DoD claim against actual source artifacts, then ran full CLI validation suite.

### Method

1. Read and cross-referenced every DoD item in `scopes.md` against source files
2. Executed `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh test unit`
3. Verified CI workflow structure, Dockerfile build args/labels, docker-compose.yml env_file/image overrides, health endpoint wiring, and documentation artifacts

### Artifact Verification

| Scope | Artifact | Claim | Verified |
|-------|----------|-------|----------|
| 1 | `.github/workflows/ci.yml` | CI runs lint+test+build on push/PR | YES — workflow triggers, jobs, timeouts all match |
| 1 | CI job dependency | `build` needs `lint-and-test` | YES — `needs: lint-and-test` present |
| 2 | `Dockerfile` build args | VERSION, COMMIT_HASH, BUILD_TIME args | YES — lines 15-17 |
| 2 | `ml/Dockerfile` build args | Same args in runtime stage | YES — lines 29-31 |
| 2 | OCI labels | Both Dockerfiles have 5 OCI labels | YES — version, revision, created, title, source |
| 2 | CI tagging | Images tagged on `refs/tags/v*` | YES — conditional step in build job |
| 3 | `docs/Branch_Protection.md` | Documents branch protection rules | YES — required checks, PR reviews, restrictions |
| 4 | ldflags | Go binary embeds version/commit/buildTime | YES — Dockerfile line 18, `cmd/core/main.go` vars |
| 4 | Health endpoint | Version, CommitHash, BuildTime in response | YES — `Dependencies` struct wired in `main.go`, auth-gated in handler |
| 4 | Health tests | BuildTime covered in tests | YES — 9 BuildTime assertions in `health_test.go` |
| 5 | CPU-only torch | `--index-url https://download.pytorch.org/whl/cpu` | YES — `ml/Dockerfile` line 8 |
| 5 | Multi-stage build | Builder + runtime stages | YES — two `FROM` directives |
| 5 | Cache stripping | `__pycache__`, `.dist-info`, tests removed | YES — `find` commands in builder stage |
| 6 | env_file directive | Both core and ml use `env_file:` | YES — `${SMACKEREL_ENV_FILE:-config/generated/dev.env}` |
| 6 | SST var removal | No individual SST-managed vars in environment blocks | YES — only container-path overrides remain |
| 6 | Drift guard | `./smackerel.sh check` includes env_file guard | YES — "env_file drift guard: OK" |
| 7 | push-images job | GHCR push on `refs/tags/v*` | YES — job with `docker/login-action`, tag+push steps |
| 7 | Image override | `image: ${SMACKEREL_CORE_IMAGE:-}` | YES — both services, empty default falls back to build |
| 7 | Operations docs | Pull-based deployment documented | YES — `docs/Operations.md` has Pre-built Image Deployment section |

### CLI Validation

| Check | Result |
|-------|--------|
| `./smackerel.sh check` | PASS — "Config is in sync with SST" + "env_file drift guard: OK" |
| `./smackerel.sh lint` | PASS — "All checks passed!" |
| `./smackerel.sh test unit` | PASS — Go: 41 packages OK, Python: 236 passed, 3 warnings |

### Verdict

**NO DRIFT.** All 7 scopes' claimed implementations are present and verified in source. Prior test-to-doc and security-to-doc sweeps already remediated the only findings (BuildTime test gaps, drift guard regex, Action SHA pinning). State is clean.
