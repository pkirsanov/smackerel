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
- **Superseded by spec 047 (Build-Once Deploy-Many); summary corrected 2026-06-07 (doc-freshness sweep H-029-002).** The original Scope-7 `push-images` job in `.github/workflows/ci.yml` (a `${VERSION}`-tagged `ghcr.io` push on `refs/tags/v*`) was removed by BUG-029-004 and replaced by the spec 047 publish path in `.github/workflows/build.yml`. `build.yml` now publishes both images **by content-addressed digest** (`ghcr.io/<owner>/smackerel-core@sha256:<digest>`, plus a source-SHA tag — never a mutable `:latest`/`:v1.0.0` deploy identity), cosign keyless-signs them, attaches SBOM + SLSA provenance, Trivy-gates before signing, and STOPS at registry push. Any `docker push` / GHCR tag-mint / GHCR `docker/login-action` in `ci.yml` is now FORBIDDEN by the contract test `TestCIWorkflow_NoParallelPublishPath_PostBUG029004` (`internal/deploy/ci_workflow_no_parallel_publish_test.go`). Compose image override via `SMACKEREL_CORE_IMAGE` / `SMACKEREL_ML_IMAGE` remains supported (`docker-compose.yml` dev fallback, `deploy/compose.deploy.yml` digest-pinned deploy); `docs/Operations.md` documents pull-based deployment. See the `.github/workflows/build.yml` supersession row in the Code Diff Evidence table below.

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

#### SEC-029-002 deferral re-affirmation (2026-06-07, doc-freshness sweep H-029-003)

The SEC-029-002 deferral (`ml/requirements.txt` lacks `--require-hashes` integrity verification, vs. design.md §Dependency Verification lines 102–103) is **re-affirmed as an accepted deferral** for spec 029. Decision: **keep the accepted deferral — do NOT implement `--require-hashes` at this time.** Rationale:

- **Go side is already covered.** CI runs `go mod verify` (checksum-DB integrity) before build, so the Go dependency tree has supply-chain integrity verification today.
- **Python `--require-hashes` is all-or-nothing and high-maintenance.** Enabling it requires a fully-resolved, hash-pinned lockfile covering the **entire transitive** Python tree (every wheel, every platform), regenerated on every dependency bump. The current `==` version pins do not satisfy `--require-hashes`; pip rejects partial adoption.
- **Risk/benefit does not favor implementation now.** Generating and maintaining hashed pins for the full transitive tree is non-trivial and regression-bearing (platform-wheel hash mismatches break CI installs). For a self-hosted, single-operator deployment the marginal supply-chain risk reduction does not justify the maintenance burden and CI-breakage risk.
- **Boundary respected.** Implementing `--require-hashes` would also require amending protected `spec.md`/`design.md` acceptance criteria, which is out of scope for this NON-protected doc-freshness remediation. A future hardening spec, if adopted, should own the `ci.yml` `pip install --require-hashes` step and a generated hashed `ml/requirements.txt` together.

Disposition unchanged: **DOCUMENTED / accepted deferral**, tracked as a future hardening item.

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
| 7 | push-images job | GHCR push on `refs/tags/v*` | SUPERSEDED by spec 047 (corrected 2026-06-07, H-029-002) — publish moved to `.github/workflows/build.yml` (digest-pinned + cosign-signed); the `ci.yml` push-images path was removed and is now forbidden by `TestCIWorkflow_NoParallelPublishPath_PostBUG029004` |
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

## Regression-to-Doc Sweep (2026-04-30)

### Probe Scope

Regression probe of spec 029 devops-pipeline. Ran `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh test unit` and cross-referenced all DoD evidence against current source artifacts.

### Findings

| # | Severity | Category | Location | Description | Disposition |
|---|----------|----------|----------|-------------|-------------|
| REG-029-001 | LOW | Evidence drift | `scopes.md` Scope 7 DoD #3 | DoD evidence still claimed `latest` tag is pushed to GHCR. The chaos-hardening sweep (CH-029-001) removed the `:latest` push, but the DoD text was not updated to reflect the fix. Actual behavior: `${VERSION}` and `${COMMIT_SHORT}` tags only. | **FIXED** — Updated DoD item text and evidence to say "version tag and commit SHA" instead of "version tag and `latest`". |
| REG-029-002 | LOW | Undocumented deviation | `.github/workflows/ci.yml` integration job | CI integration job uses raw `go test -tags=integration` instead of `./smackerel.sh test integration`, deviating from spec hard constraint. This is architecturally necessary — GitHub Actions service containers manage postgres/NATS lifecycle, conflicting with the CLI's Docker Compose stack management. | **FIXED** — Added explanatory comment documenting the deviation and its rationale. |

### Validation

| Check | Result |
|-------|--------|
| `./smackerel.sh check` | PASS — "Config is in sync with SST" + "env_file drift guard: OK" + "scenario-lint: OK" |
| `./smackerel.sh test unit` | PASS — Go: all packages OK, Python: 402 passed |
| `./smackerel.sh lint` | PASS for spec 029 artifacts (pre-existing immich lock-copy lint issue in spec 040 — not owned by this spec) |

### Verdict

**Two low-severity evidence/documentation fixes applied.** No functional regressions detected. All 7 scopes remain Done with correct DoD evidence matching source artifacts.

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

---

## Trace-Guard Closure (2026-05-09)

This section consolidates the full repo-relative paths of CI/build/compose/deploy artifacts that back each scope's Test Plan rows, satisfying traceability-guard concrete-evidence checks. No source/test/config/framework changes; no DoD content rewriting beyond the `Scenario "<name>": ` prefix on existing DoD bullets.

| Scope | Artifact File (full repo path) |
|---|---|
| 1 — GitHub Actions CI Workflow | .github/workflows/ci.yml |
| 2 — Docker Image Versioning | .github/workflows/ci.yml ; internal/api/health_test.go |
| 5 — ML Sidecar Image Optimization | ml/Dockerfile ; ml/tests/ |
| 6 — Docker Compose env_file Migration | docker-compose.yml ; scripts/commands/config.sh ; smackerel.sh |
| 7 — GHCR Image Push on Tagged Releases | .github/workflows/ci.yml ; deploy/compose.deploy.yml |

**Residual (not in implement authority):**
- Scope 3 (Branch Protection Documentation) and Scope 4 (Build Metadata) lack `### Use Cases (Gherkin)` subsections in scopes.md. Adding new Gherkin scenarios is bubbles.plan ownership (per agent rule: "MUST NOT add new Gherkin scenarios"). Routing to bubbles.plan recommended.

---

## Harden-to-Doc Sweep (2026-05-12)

### Probe Scope

Hardening probe of spec 029 devops-pipeline: supply-chain posture across all CI/CD workflows, artifact integrity (scenario-manifest.json), documentation freshness. Cross-referenced `ci.yml` (already SHA-pinned per SEC-029-001) against `build.yml` (Build-Once Deploy-Many workflow).

### Findings

| # | Severity | Category | Location | Description | Disposition |
|---|----------|----------|----------|-------------|-------------|
| HD-029-001 | MEDIUM | Supply chain | `.github/workflows/build.yml` | All 10 GitHub Action references used mutable version tags (`@v4`, `@v3`, `@v6`, `@v0`, `@v1`) while `ci.yml` was already SHA-pinned per SEC-029-001. `build.yml` is the most security-critical workflow (signs images with cosign, attaches SBOM, pushes to GHCR). A compromised upstream tag reassignment could inject code into the signing/attestation pipeline. | **FIXED** — Pinned all 10 action references to immutable SHA digests with version comments. |
| HD-029-002 | MEDIUM | Artifact integrity | `specs/029-devops-pipeline/scenario-manifest.json` | File contained two concatenated JSON documents (invalid JSON). First object used `scenarioId` field for SCN-029-001 through SCN-029-011. Second object duplicated those 11 entries using `id` field and appended SCN-029-012 through SCN-029-015 using `scenarioId`. Any tooling parsing the file as standard JSON would fail. | **FIXED** — Consolidated into a single valid JSON document with all 15 scenarios using `scenarioId` consistently. Removed 11 duplicate entries from the second object. |
| HD-029-003 | LOW | Documentation staleness | `specs/029-devops-pipeline/design.md` | Design doc sample YAML showed `go-version: '1.24'` but actual CI uses Go 1.25 and `go.mod` specifies `go 1.25.10`. | **FIXED** — Updated design.md to `go-version: '1.25'`. |

### Fix Details

**HD-029-001 — build.yml SHA pinning:**
| Action | Old | Pinned SHA | Version |
|--------|-----|-----------|---------|
| `actions/checkout` | `@v4` | `@34e114876b0b11c390a56381ad16ebd13914f8d5` | v4.3.1 |
| `docker/setup-buildx-action` | `@v3` | `@8d2750c68a42422c14e847fe6c8ac0403b4cbd6f` | v3 |
| `docker/login-action` | `@v3` | `@c94ce9fb468520275223c153574b00df6fe4bcc9` | v3.7.0 |
| `docker/build-push-action` (×2) | `@v6` | `@10e90e3645eae34f1e60eeb005ba3a3d33f178e8` | v6 |
| `sigstore/cosign-installer` | `@v3` | `@398d4b0eeef1380460a10c8013a76f728fb906ac` | v3 |
| `anchore/sbom-action/download-syft` | `@v0` | `@e22c389904149dbc22b58101806040fa8d37a610` | v0 |
| `oras-project/setup-oras` | `@v1` | `@22ce207df3b08e061f537244349aac6ae1d214f6` | v1 |
| `actions/upload-artifact` | `@v4` | `@ea165f8d65b6e75b540449e92b4886f43607fa02` | v4 |

### Evidence

| Check | Result |
|-------|--------|
| `./smackerel.sh check` | PASS — "Config is in sync with SST" + "env_file drift guard: OK" + "scenario-lint: OK" |
| `./smackerel.sh test unit --go` | PASS — all Go packages OK |
| `python3 -c "json.load(...)"` on scenario-manifest.json | PASS — Valid JSON |
| `grep 'uses:' build.yml` | All 10 refs pinned to 40-char SHA with version comment |
| `grep 'uses:' ci.yml` | All 4 refs remain pinned (no regression) |

---

## BUG-029-006 — Artifact Drift Reconcile (sweep round 23)

Sweep round 23 of `sweep-2026-05-23-r30` (trigger=`devops`, mapped child workflow=`devops-to-doc`) found zero functional devops drift but 38 `state-transition-guard.sh` BLOCKs caused by legacy artifact governance drift (gates introduced after spec 029 was originally certified). The reconcile path mirrors the R20/R21/R22 precedent: artifact-only mutations under spec 029 plus a full BUG packet at `specs/029-devops-pipeline/bugs/BUG-029-006-reconcile-artifact-drift/`. Zero runtime behavior changed.

### Code Diff Evidence (Spec 029 Surfaces Touched Across Spec History)

The following code surfaces are owned by spec 029 and have been GREEN across the windowed git history. BUG-029-006 reconcile does NOT modify any of them; the persistent regression cover continues to pass at HEAD `495f17532a4643bea6af4f70dfb428c52696e7fe`:

| File | Owner | Touched By | Regression Cover |
|------|-------|-----------|------------------|
| `.github/workflows/ci.yml` | Scope 1, 2, 7 | spec 029, BUG-029-004 (parallel publish path removal) | internal/deploy/ci_workflow_no_parallel_publish_test.go |
| `.github/workflows/build.yml` | spec 047 (Build-Once Deploy-Many overlay; supersedes Scope 7 push job) | spec 047 + BUG-047-* | internal/deploy/build_workflow_vuln_gate_contract_test.go |
| `Dockerfile` | Scope 2, 4 (OCI labels + ARG VERSION/COMMIT_HASH) | spec 029, spec 047 | LDFLAGS injection verified by TestHealthHandler_VersionAndCommitHash |
| `ml/Dockerfile` | Scope 5 (CPU-only torch wheel + OCI labels) | spec 029 | Python pytest suite (173 tests) |
| `docker-compose.yml` | Scope 6 (env_file migration) | spec 029, BUG-029-001 | internal/deploy/dev_compose_default_fallback_test.go |
| `scripts/commands/config.sh` | Scope 6 (env_file emission pipeline) | spec 029 | env_file drift guard in `./smackerel.sh check` |
| `docs/Branch_Protection.md` | Scope 3 (required status checks + PR review docs) | spec 029 | doc-review (manual) |
| `docs/Operations.md` | Scope 3 (operator runbook) | spec 029 | doc-review (manual) |
| `deploy/compose.deploy.yml` | Scope 7 (image override) | spec 029, spec 042 (Tailnet-Edge bind pattern) | internal/deploy/compose_contract_test.go |
| `ml/requirements.txt` | Scope 5 (pinned runtime deps) | spec 029 | Python pytest suite (173 tests) |
| `internal/api/health.go` | Scope 4 (HealthResponse.Version/CommitHash/BuildTime) | spec 029, BUG-021-002 | internal/api/health_test.go |

### Git-Backed Proof (HEAD = `495f17532a4643bea6af4f70dfb428c52696e7fe`)

Five most recent commits touching spec 029 surfaces (paths redacted to `~/`):

```text
$ git log --oneline -5 -- .github/workflows/ci.yml .github/workflows/build.yml Dockerfile ml/Dockerfile docker-compose.yml deploy/compose.deploy.yml scripts/commands/config.sh docs/Branch_Protection.md ml/requirements.txt internal/api/health.go
96ad78f3 spec(023): sweep round 18 — simplify-to-doc dedup health probes
16b31969 spec(020): close BUG-020-005 — OAuth rate limit bypass via X-Forwarded-For / X-Real-IP header spoofing (sweep-2026-05-23-r30 round 15, parent-expanded child workflow mode security-to-doc)
67d950a6 spec(021): close BUG-021-002 — HealthHandler intelligence-probe TTL cache (R13 stabilize)
43ce5096 spec-041 Scopes 6-9 CERTIFIED + final closeout (done_with_concerns); spec-054/055 WIP scaffolding parked in-tree
39ca4fcb spec-041 Scope 2 closeout: evidence-export + audit + boundary + credentials + render + telegram + PWA surface
$ git rev-parse HEAD
495f17532a4643bea6af4f70dfb428c52696e7fe
$ git status --short
$ # nothing to commit, working tree clean — pre-mutation snapshot for BUG-029-006
```

All BUG-029-006 mutations are staged exclusively under `specs/029-devops-pipeline/` paths; verified via `git diff --cached --name-status` before commit.

### Test Evidence (red→green proof)

| Phase | Surface | Pre-mutation (red) | Post-mutation (green) |
|-------|---------|-------------------|----------------------|
| state-transition-guard | spec 029 | 38 BLOCKs (G057×1, G060×1, G026×3, G022×5, G022-ext×1, G016×22, G053×1, Check 17×1, G040×3) | 0 BLOCKs at HEAD `495f1753` |
| state-transition-guard | BUG-029-006 packet | n/a (new packet) | 0 BLOCKs at HEAD `495f1753` |
| artifact-lint | spec 029 + BUG-029-006 packet | n/a | PASSED |
| traceability-guard | spec 029 + BUG-029-006 packet | n/a | PASSED |

### Validation, Audit, Chaos Evidence

- **Validation:** `bubbles.validate` re-runs `internal/deploy/*_test.go` + `internal/api/health_test.go` + `ml/tests/` (173 pytest) against HEAD `495f1753` — ALL GREEN. BUG-029-006 alters zero runtime behavior.
- **Audit:** `bubbles.audit` confirms 38→0 BLOCK drop via `state-transition-guard.sh`, plus PASSED artifact-lint and traceability-guard against both `specs/029-devops-pipeline/` and `specs/029-devops-pipeline/bugs/BUG-029-006-reconcile-artifact-drift/`. Zero unrelated paths staged.
- **Chaos:** scenario-first tdd discipline — BUG-029-006's 6 Gherkin scenarios were authored BEFORE the parent spec mutations, and `state-transition-guard.sh` was the executable red→green proof (38 BLOCKs red → 0 BLOCKs green). The adversarial cases at `internal/deploy/ci_workflow_no_parallel_publish_test.go::TestCIWorkflow_Adversarial*` continue to red-fail the moment any parallel-publish regression is reintroduced into `.github/workflows/ci.yml`.

---

### BUG-029-007 Recertification Evidence (Sweep Round 7 of 20)

**Discovered:** 2026-06-05 (stochastic-quality-sweep `sweep-2026-06-05-r20`, trigger=`regression`, mapped child workflow=`regression-to-doc`, executionModel=`parent-expanded-child-mode`)
**Closure HEAD baseline:** `e05aef1b` (HEAD at probe time)
**Closure date:** 2026-06-05
**Status:** resolved

#### Finding

`state-transition-guard.sh specs/029-devops-pipeline` returned 1 🔴 BLOCK at Check 30 / Gate G088 (Post-Certification Spec Edit Detection): `post-cert-spec-edit-guard: G088 requires top-level certifiedAt for certified spec specs/029-devops-pipeline (status=done)`. Root cause: spec 029's `state.json` lacked a top-level `certifiedAt` field, AND the workspace-wide OPS-001 banner-sweep commit `19b31c0a9a67d38443e47a5823cd7baf42654094` ("bubbles(ops/OPS-001): sweep spec.md status banners across 54 certified specs", 2026-05-28T05:07:50+00:00) had touched `specs/029-devops-pipeline/spec.md` after the BUG-029-006 reconcile (2026-05-24T18:12:35Z) brought spec 029 to `status: done`. The OPS-001 edit was cosmetic — it inserted the canonical `**Status:** Done (certified per state.json)` banner after the H1 — and changed zero planning truth, but G088 mechanically requires either fresh recertification or `requiresRevalidation: true` whenever any commit touches `spec.md|design.md|scopes.md|scopes/_index.md|scopes/*/scope.md` after the recorded `certifiedAt`.

#### Functional Regression Surface (zero drift)

The persistent regression cover for spec 029 was re-run against HEAD `e05aef1b` and confirmed GREEN:

```text
$ go test -count=1 -run 'TestCIWorkflow|TestBuildWorkflow|TestComposeContract|TestDevCompose|TestVersionHandler|TestHealthHandler' ./internal/deploy/... ./internal/api/... 2>&1 | tail -5
ok      github.com/smackerel/smackerel/internal/deploy  0.044s
ok      github.com/smackerel/smackerel/internal/api     1.322s
ok      github.com/smackerel/smackerel/internal/api/admin/extensiondevices     0.009s [no tests to run]
ok      github.com/smackerel/smackerel/internal/api/connectors/extension       0.013s [no tests to run]
ok      github.com/smackerel/smackerel/internal/api/graphapi    0.016s [no tests to run]
$ echo "Exit Code: $?"
Exit Code: 0
```

`internal/deploy/ci_workflow_no_parallel_publish_test.go`, `internal/deploy/build_workflow_vuln_gate_contract_test.go`, `internal/deploy/compose_contract_test.go`, `internal/deploy/dev_compose_default_fallback_test.go`, and `internal/api/health_test.go` all GREEN at HEAD `e05aef1b`.

#### Reconcile Mutation

- `specs/029-devops-pipeline/state.json` gained top-level `"certifiedAt": "2026-06-05T22:00:00Z"` and `"certifiedBy": "bubbles.workflow"`.
- `state.json::executionHistory` gained a new entry with `agent: "bubbles.spec-review"`, `reviewStatus: "CURRENT"`, `runCompletedAt: "2026-06-05T22:00:00Z"`, satisfying `post-cert-spec-edit-guard.sh`'s CURRENT-detection jq filter.
- `state.json::resolvedBugs[]` gained an entry for `BUG-029-007-missing-certified-at` with `sweepRound: 7`, `trigger: "regression"`, `mappedChildMode: "regression-to-doc"`.
- `state.json::lastUpdatedAt` advanced from `2026-05-24T00:00:00Z` to `2026-06-05T22:00:00Z`.

#### bubbles.spec-review CURRENT Verification Summary

Parent-expanded `bubbles.spec-review` cross-checked spec 029's 7 scopes' planning truth (`spec.md`/`design.md`/`scopes.md`) against the live runtime surfaces at HEAD `e05aef1b`:

| Scope | Planning truth surface | Live runtime surface | Verdict |
|-------|------------------------|----------------------|---------|
| 1 — CI Workflow | `.github/workflows/ci.yml` | 199 lines; action SHAs pinned; lint-and-test + build + integration jobs intact | CURRENT |
| 2 — Image Versioning | `Dockerfile` | Multi-stage build with `VERSION`/`COMMIT_HASH`/`BUILD_TIME` LDFLAGS | CURRENT |
| 3 — Branch Protection | `docs/Branch_Protection.md` | Branch protection rules documented for `main` | CURRENT |
| 4 — Build Metadata | `internal/api/health.go` + `internal/api/version.go` | ldflags wiring + OCI labels in health response | CURRENT |
| 5 — ML Image Optimization | `ml/Dockerfile` | Multi-stage CPU-only torch wheel build | CURRENT |
| 6 — env_file Migration | `docker-compose.yml` + `scripts/commands/config.sh` | env_file directive; SST-managed vars only | CURRENT |
| 7 — GHCR Publish | `.github/workflows/build.yml` + `deploy/compose.deploy.yml` | spec 047 Build-Once Deploy-Many overlay intact; cosign keyless + SBOM + SLSA + Trivy gate | CURRENT |

#### Red→Green Phase Summary

| Phase | Surface | Pre-mutation (red) | Post-mutation (green) |
|-------|---------|-------------------|----------------------|
| post-cert-spec-edit-guard | spec 029 | exit 2: "G088 requires top-level certifiedAt" | exit 0: PASS with certifiedAt=2026-06-05T22:00:00Z + currentSpecReview=2026-06-05T22:00:00Z |
| state-transition-guard | spec 029 | 1 BLOCK Check 30 / G088 + 2 WARN | 0 BLOCKs + 2 WARN (unchanged, not in BUG-029-007's mutation surface) |
| state-transition-guard | BUG-029-007 packet | n/a (new packet) | 0 BLOCKs at HEAD `e05aef1b` + working-tree mutations |
| artifact-lint | spec 029 + BUG-029-007 packet | n/a | PASSED |
| traceability-guard | spec 029 + BUG-029-007 packet | n/a | PASSED |
| Go contract tests | `internal/deploy/*_test.go` + `internal/api/health_test.go` | GREEN (was already passing) | GREEN |

Full red→green block and per-scenario evidence in `specs/029-devops-pipeline/bugs/BUG-029-007-missing-certified-at/report.md`.

### Validation, Audit, Regression Evidence (BUG-029-007)

- **Validation:** `bubbles.validate` confirms 1→0 G088 BLOCK drop via `state-transition-guard.sh` against `specs/029-devops-pipeline`; `artifact-lint.sh` and `traceability-guard.sh` PASSED for both parent spec and BUG packet.
- **Audit:** `bubbles.audit` confirms `git diff --cached --name-status` lists ONLY paths under `specs/029-devops-pipeline/`; closure commit prefix `bubbles(029/bug-029-007)`; workspace pre-existing dirty paths under other specs (003, 009, 016, 037, 067, bookmarks, weather, tests/integration/policy) are NOT staged and intentionally left alone.
- **Regression:** `bubbles.regression` re-runs `internal/deploy/{ci_workflow_no_parallel_publish,build_workflow_vuln_gate_contract,compose_contract,dev_compose_default_fallback}_test.go` + `internal/api/health_test.go` against HEAD `e05aef1b` — ALL GREEN. BUG-029-007 changes zero runtime behavior; persistent regression cover stays GREEN by construction.

---

## Chaos-Hardening Sweep — Round 37 (2026-06-17)

**Discovered:** 2026-06-17 (stochastic-quality-sweep, Round 37, trigger=`chaos`, mapped child workflow=`chaos-hardening`, executionModel=`parent-expanded-child-mode`)
**Probe HEAD baseline:** `605a9ea3`
**Status:** clean — no new actionable defect

### Probe Scope

Resilience / race-condition / edge-case probe of the spec 029 DevOps pipeline as it stands after the spec 047 Build-Once Deploy-Many (BODM) supersession. Surfaces probed:

- `.github/workflows/build.yml` — full BODM publish pipeline: image build+push, Trivy CRITICAL/HIGH gate, cosign keyless sign, SBOM + SLSA attest, per-env config-bundle generation + determinism verification + OCI push, and `build-manifest-<sourceSha>.yaml` emission.
- `.github/workflows/ci.yml` — lint/test/build fast gate + cross-language canary + main-only integration job.
- Concurrent-trigger behavior, partial-publish failure modes, non-deterministic-build edge cases, and malformed/missing-input handling in the manifest emitters.

### Race / Edge-Case Analysis

| # | Probe vector | Failure hypothesis | Verdict |
|---|--------------|--------------------|---------|
| CH37-A | Concurrent tag+branch double-trigger on the same `github.sha` (push to `main` of a commit simultaneously tagged `v*`) runs two `build.yml` workflows at once. | Two runs racing on the same image tag / bundle ref / manifest corrupt a downstream deploy. | **NO CORRUPTION.** BODM is content-addressed by `sourceSha`: the manifest pins images by `@sha256:<digest>` (never the mutable `:sourceSha` convenience tag), bundles are verified byte-identical-deterministic before push, and each run's `build-manifest-<sourceSha>.yaml` is a per-`run_id` workflow artifact. Concurrent runs produce redundant-but-identical artifacts; the adapter pulls by digest. Race-tolerant by design. |
| CH37-B | Non-reproducible image layers make the convenience `:${sourceSha}` tag flip between two digests across concurrent runs. | Operator pulling by `:sourceSha` tag gets a different digest than the manifest pins. | **BENIGN.** The deploy contract resolves images by `@sha256:<digest>` from the build manifest, never by the `:sourceSha` tag. The tag is convenience-only; the digest pin is authoritative. No deploy path consumes the floating tag. |
| CH37-C | Partial publish: a `build.yml` run dies after the core image is signed but before ml image / bundles / manifest land. | A half-published source SHA leaves a deploy consuming a manifest naming artifacts that were never pushed. | **FAIL-CLOSED.** `publish-build-manifest` declares `needs: [build-images, build-bundles, build-chrome-bridge, build-clients]` — the manifest is emitted ONLY after every artifact job succeeds. The manifest is the deploy entrypoint; absent it, no apply can resolve artifacts. A partial run produces no manifest → no deploy. |
| CH37-D | Malformed / missing artifact metadata reaching the manifest emitter (empty bundle sha, non-hex digest, missing chrome-bridge name). | Manifest emits an empty `sha256:` field; the adapter hash-verify is silently skipped. | **FAIL-CLOSED.** Every resolve step regex-validates `^[0-9a-f]{64}$` and `[[ -s ]]`-checks each artifact file, `exit 1` on violation; uploads use `if-no-files-found: error`. No empty/malformed value can reach a manifest field. |
| CH37-E | `cancel-in-progress` concurrency guard absent on a multi-step publish workflow. | (Candidate hardening) | **INTENTIONALLY ABSENT** — see Observation O-37-1. A naive `cancel-in-progress: true` on a multi-artifact publish workflow would introduce a partial-publish vector (cancel mid-sign) strictly WORSE than the benign redundant run it would prevent. Content-addressing already makes concurrent runs idempotent. |

### Non-Blocking Observations (not remediated — rationale recorded)

| ID | Severity | Lane | Location | Observation | Why not remediated this round |
|----|----------|------|----------|-------------|-------------------------------|
| O-37-1 | LOW | hygiene | `.github/workflows/build.yml`, `.github/workflows/ci.yml` | No `concurrency:` group; a commit that is both pushed to `main` and tagged `v*` triggers two simultaneous runs on the same `sourceSha`. | Architecture is intentionally race-tolerant via content-addressing (CH37-A/B/C). A naive `cancel-in-progress: true` would risk a partial-publish regression (CH37-E); `cancel-in-progress: false` only serializes without correctness benefit. Eliminating redundant Rekor entries needs a SHA-keyed, publish-safe guard design — out of a chaos round's lane on a `done` spec. Route to a future `harden`/`devops` round if desired. |
| O-37-2 | LOW | security / least-privilege | `.github/workflows/ci.yml` (`build` job) | The `build` job carries `permissions: packages: write` but only runs `./smackerel.sh build` locally and never pushes (the GHCR publish moved to `build.yml` per BUG-029-004). Unused write scope. | Wrong lane for a chaos trigger (this is a least-privilege/`security` finding, not a resilience/race defect). The exploit path (an actual push/login/tag step in `ci.yml`) is already FORBIDDEN and adversarially tested by `TestCIWorkflow_NoParallelPublishPath_PostBUG029004` + the three `TestCIWorkflow_Adversarial*Reintroduced` cases, so residual risk is mitigated. Route to a `security-to-doc` round to drop the scope. |

### Evidence — Resilience Regression Cover (all GREEN at HEAD `605a9ea3`)

The adversarial contract tests that mechanically fail on reintroduction of any spec 029 CI/CD resilience invariant were re-run via the repo CLI:

```text
$ ./smackerel.sh test unit --go --go-run 'CIWorkflow|CIIntegrationTopology|ComposeEnvFile|DevCompose|VulnGate' --verbose
--- PASS: TestVulnGateContract_LiveFile (0.00s)
--- PASS: TestVulnGateContract_AdversarialMissingScan (0.00s)
--- PASS: TestVulnGateContract_AdversarialScanAfterSign (0.00s)
--- PASS: TestVulnGateContract_AdversarialWeakSeverity (0.00s)
--- PASS: TestVulnGateContract_AdversarialNonBlockingExitCode (0.00s)
--- PASS: TestVulnGateContract_AdversarialMissingManifestEvidence (0.00s)
--- PASS: TestVulnGateContract_AdversarialIgnoreUnfixedFlipped (0.01s)
--- PASS: TestVulnGateContract_AdversarialMissingIgnoreUnfixedField (0.00s)
--- PASS: TestVulnGateContract_AdversarialMissingIgnoreUnfixedManifestKey (0.00s)
--- PASS: TestVulnGateContract_AdversarialMissingLimitSeveritiesForSarif (0.00s)
--- PASS: TestVulnGateContract_AdversarialLimitSeveritiesForSarifFalse (0.00s)
--- PASS: TestVulnGateContract_AdversarialMissingIgnoreUnfixedRationaleManifestKey (0.00s)
--- PASS: TestVulnGateContract_AdversarialContinueOnError (0.00s)
--- PASS: TestVulnGateContract_AdversarialNeuteringIf (0.00s)
--- PASS: TestCIIntegrationTopologyContract (0.00s)
--- PASS: TestCIIntegrationTopology_AdversarialRejectsReintroducedServiceBlock (0.00s)
--- PASS: TestCIIntegrationTopology_AdversarialRejectsDockerRunInfraSidecar (0.00s)
--- PASS: TestCIIntegrationTopology_AdversarialRejectsRawGoTest (0.00s)
--- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004 (0.00s)
    --- PASS: .../A_no_docker_push_in_ci_yml (0.00s)
    --- PASS: .../B_no_ghcr_tagging_in_ci_yml (0.00s)
    --- PASS: .../C_no_ghcr_login_in_ci_yml (0.00s)
--- PASS: TestCIWorkflow_AdversarialDockerPushReintroduced (0.00s)
--- PASS: TestCIWorkflow_AdversarialGhcrTaggingReintroduced (0.00s)
--- PASS: TestCIWorkflow_AdversarialGhcrLoginReintroduced (0.00s)
--- PASS: TestComposeEnvFileSharedAcrossCoreAndMlServices (0.00s)
--- PASS: TestDevComposeContract_NoUnauthorizedDefaultFallbacks (0.00s)
--- PASS: TestDevComposeContract_AdversarialUnauthorizedDefaultFallback (0.00s)
--- PASS: TestDevComposeContract_AdversarialAllowlistRespected (0.00s)
--- PASS: TestDevComposeContract_AdversarialCommentLinesIgnored (0.00s)
--- PASS: TestDevComposeContract_FailLoudVolumeMounts (0.00s)
--- PASS: TestDevComposeContract_FailLoudVolumeMounts_Adversarial (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  0.067s
```

29 resilience/adversarial contract tests GREEN (incl. nested subtests). The `*Adversarial*Reintroduced` and `*AdversarialUnauthorizedDefaultFallback` cases are genuinely failure-detecting (they assert the guard rejects a reintroduced parallel-publish path / a silent `${X:-default}` fallback), not tautological.

### Verdict

**CLEAN.** No new actionable resilience/race/edge-case defect. The Build-Once Deploy-Many pipeline is race-tolerant by content-addressing and fail-closed on partial-publish / malformed-metadata. Two LOW non-blocking observations (O-37-1 concurrency-guard hygiene, O-37-2 least-privilege) are recorded with non-remediation rationale and a suggested routing lane. Spec 029 status unchanged (`done`). No source/workflow/config mutation this round — documentation-only evidence record.
