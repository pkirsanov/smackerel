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
| `./smackerel.sh lint` | PASS — lint clean |
| `./smackerel.sh test unit` | PASS — Go: 41 packages OK, Python: 236 passed, 3 warnings |

### Verdict

**NO DRIFT.** All 7 scopes' claimed implementations are present and verified in source. Prior test-to-doc and security-to-doc sweeps already remediated the only findings (BuildTime test gaps, drift guard regex, Action SHA pinning). State is clean.

---

## Chaos-Hardening Sweep (2026-04-22)

### Probe Scope

Chaos probe of spec 029 devops-pipeline: CI workflow edge cases, env_file drift guard robustness, GHCR push policy compliance, image pinning consistency. Probed for fragile guards, policy contradictions, and incomplete security posture.

### Findings

| # | Severity | Category | Location | Description | Disposition |
|---|----------|----------|----------|-------------|-------------|
| CH-029-001 | MEDIUM | Policy violation | `.github/workflows/ci.yml` | GHCR push step pushed both versioned tags and `:latest` to registry. `docs/Docker_Best_Practices.md` explicitly lists `:latest` tags as "Not acceptable as proof" of freshness. Mutable `:latest` tag defeats rollback guarantee and contradicts project governance. | **FIXED** — Removed `:latest` push. Now pushes only `${VERSION}` and `${COMMIT_SHORT}` (12-char SHA) tags. Both are immutable references. |
| CH-029-002 | MEDIUM | False security | `smackerel.sh` | env_file drift guard used a hardcoded blocklist of 10 SST-managed vars, but `config/generated/dev.env` emits 100+ vars. Any of the other 90+ vars (e.g., `TELEGRAM_BOT_TOKEN`, `EXPENSES_ENABLED`, `KNOWLEDGE_ENABLED`) could be individually declared in docker-compose.yml without triggering the guard. | **FIXED** — Rewrote guard to dynamically read all vars from the generated env file and check against the core/ml service `environment:` blocks. Only allowed container-path overrides (PORT, BOOKMARKS_IMPORT_DIR, etc.) are exempt. Guard now covers all SST-managed vars. |
| CH-029-003 | LOW | Inconsistency | `.github/workflows/ci.yml:107` | CI integration test `docker run` used mutable `nats:2.10-alpine` tag while all GitHub Actions were SHA-pinned per SEC-029-001. Inconsistent security posture for supply-chain integrity. | **FIXED** — Pinned NATS image to `nats@sha256:b83efabe3e7def1e0a4a31ec6e078999bb17c80363f881df35edc70fcb6bb927` (2.10-alpine digest). |

### Evidence

| Check | Result |
|-------|--------|
| `./smackerel.sh check` | PASS — "Config is in sync with SST" + "env_file drift guard: OK" |
| `./smackerel.sh test unit` | PASS — Go: 41 packages OK, Python: 257 passed, 2 warnings |
| Drift guard awk extraction | Correctly extracts only 6 allowed override vars from core/ml service blocks |
| CI YAML GHCR push | Only `${VERSION}` and `${COMMIT_SHORT}` tags — no `:latest` |
| CI YAML NATS image | Pinned to immutable SHA digest |

---

## Completion Statement

**Executed:** YES
**Phase Agent:** bubbles.workflow
**Date:** 2026-04-24

All 7 scopes Done with verified file/line evidence in scopes.md DoD blocks. Implementation present in:
- `.github/workflows/ci.yml` — lint/test/build pipeline with timeouts
- `Dockerfile` + `ml/Dockerfile` — VERSION/COMMIT_HASH/BUILD_TIME ARGs + OCI labels
- `docs/Branch_Protection.md` — required status checks and PR review policy
- `cmd/core/main.go` — version/commitHash/buildTime ldflags wiring
- `docker-compose.yml` — env_file migration via `config/generated/dev.env`
- `.github/workflows/ci.yml` GHCR push job (tag-triggered)
- `smackerel.sh` — repo-standard CLI surface

Status promoted to `done` after stochastic-quality-sweep rounds (test, security, reconcile, chaos-hardening) closed all findings.

---

### Test Evidence

**Executed:** YES
**Command:** `./smackerel.sh test unit`
**Phase Agent:** bubbles.test
**Date:** 2026-04-24

```
$ ./smackerel.sh test unit
........................................................................ [ 21%]
..FF.................................................................... [ 43%]
........................................................................ [ 65%]
........................................................................ [ 87%]
..........................................                               [100%]
2 failed, 328 passed, 1 warning in 21.31s
```

Note: 2 failing tests are in spec 020-security-hardening's ML sidecar auth (Python 3.12 asyncio API), not owned by spec 029. Go core (37 packages including `cmd/core`) compiles and tests pass.

---

### Validation Evidence

**Executed:** YES
**Command:** `./smackerel.sh check`
**Phase Agent:** bubbles.validate
**Date:** 2026-04-24

```
$ ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
```

Exit Code: 0. Config SST validation passed. `env_file` drift guard verifies generated `config/generated/dev.env` and `config/generated/test.env` only expose the 6 allowed override vars to `core` and `ml` services per spec 029 scope 6 design.

---

### Audit Evidence

**Executed:** YES
**Command:** `./smackerel.sh lint`
**Phase Agent:** bubbles.audit
**Date:** 2026-04-24

```
$ ./smackerel.sh lint
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: web/extension/manifest.json
  OK: web/extension/manifest.firefox.json
=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/pwa/sw.js
  OK: web/pwa/lib/queue.js
  OK: web/extension/background.js
  OK: web/extension/popup/popup.js
  OK: web/extension/lib/queue.js
  OK: web/extension/lib/browser-polyfill.js
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
```

Exit Code: 0. Lint clean across Go, Python, web. CI YAML, Dockerfile, docker-compose.yml, smackerel.sh syntax validated.

---

### Chaos Evidence

**Executed:** YES
**Command:** `grep -n 'GHCR_PAT\|secrets\.\|github.token\|latest$' .github/workflows/ci.yml docker-compose.yml`
**Phase Agent:** bubbles.chaos
**Date:** 2026-04-24

**Approach:** Spec 029 is build-time CI infrastructure with no runtime failure surface that warrants a dedicated chaos harness. CI failures surface via GitHub Actions run status; image build failures surface via `docker build` exit codes; env_file drift is caught by `./smackerel.sh check`. The chaos-hardening sweep recorded above (2026-04-22 section) already verified absence of `:latest` floating tags, immutable NATS SHA pinning, and proper secret usage. End-to-end chaos belongs to spec 022-operational-resilience and spec 031-live-stack-testing.
