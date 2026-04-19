# System Review Planning — 029 DevOps Pipeline Findings

**Date:** 2026-04-19
**Spec:** [029-devops-pipeline](spec.md)
**Chain:** analyst → ux → design → plan

---

## Phase 1: Analyst — Finding Classification

### RT-001: Docker Compose env var gap

| Field | Value |
|-------|-------|
| **Classification** | Bug — deployment defect in existing infrastructure |
| **Severity** | CRITICAL (P0) |
| **Business Impact** | All post-foundation features (expenses, meal planning, cook mode, GuestHost, Telegram assembly, OTEL) are non-functional in containerized deployment |
| **Actors** | Self-hosted operator, CI pipeline |
| **Root Cause** | docker-compose.yml maintained individual `KEY: ${KEY}` declarations that drifted from config.sh output |
| **Gap Size** | 52+ env vars missing for smackerel-core, 3+ for smackerel-ml |
| **Affected Specs** | 008 (Telegram share), 013 (GuestHost), 030 (Observability), 034 (Expenses), 035 (Recipe enhancements), 036 (Meal planning) |

### SM-001: Switch to env_file pattern

| Field | Value |
|-------|-------|
| **Classification** | Fix approach for RT-001 (same work item) |
| **Severity** | LOW (standalone) — but becomes P0 as the RT-001 fix vehicle |
| **Business Impact** | Eliminates ~100 lines of maintenance burden, prevents all future env var drift |
| **Actors** | Developer maintaining docker-compose.yml |

**Decision:** RT-001 and SM-001 are merged into a single scope (Scope 6). The bug is the drift; the fix is env_file.

### DO-002: CI Docker build on PRs

| Field | Value |
|-------|-------|
| **Classification** | Audit finding — **already addressed** |
| **Severity** | HIGH (original) → RESOLVED (verified) |
| **Evidence** | `.github/workflows/ci.yml` `build` job has no `if:` condition, runs on all events including PRs |
| **Action** | No code change. Documented as BUG-002 with RESOLVED status. |

### DO-003: No image push to registry

| Field | Value |
|-------|-------|
| **Classification** | Scope extension — contradicts spec Non-Goals |
| **Severity** | HIGH |
| **Business Impact** | Self-hosted operators must build from source; no pull-based deployment or fast rollback |
| **Actors** | Self-hosted operator, CI pipeline |
| **Spec Conflict** | spec.md Non-Goals explicitly excludes GHCR. Requires amendment. |

---

## Phase 2: UX — Operator Experience

These are infrastructure findings with no end-user UI impact. The UX surface is the **developer/operator workflow**.

### Expected Operator Workflow (Post-Fix)

```
1. Edit config/smackerel.yaml          ← SST source of truth
2. ./smackerel.sh config generate      ← Produces config/generated/dev.env
3. ./smackerel.sh build                ← Builds Docker images (optional if pulling)
4. ./smackerel.sh up                   ← Starts stack; env_file delivers ALL vars
5. ./smackerel.sh status               ← Verify all services healthy
```

### Operator Pain Points Addressed

| Pain Point | Before | After |
|------------|--------|-------|
| Missing env vars | Silent failure — features don't work | env_file delivers all vars automatically |
| Adding new config | Must update 3 places (yaml, config.sh, docker-compose.yml) | Update 2 places (yaml, config.sh) — Compose auto-inherits |
| Deploying to new host | Must clone + build from source | Can `docker pull` pre-built images |
| Rollback | Checkout old tag + rebuild (5-10 min) | `docker pull` old version (seconds) |

### UX Invariants

- `./smackerel.sh up` must work without manual env file editing
- `./smackerel.sh config generate` must be the only prerequisite for `up`
- Pre-built image deployment must be opt-in (build-from-source remains default)

---

## Phase 3: Design — Technical Solutions

### Scope 6 Design: env_file Migration

**Approach:** Replace individual `environment:` declarations with `env_file:` directive.

```yaml
# docker-compose.yml — smackerel-core
services:
  smackerel-core:
    build:
      context: .
      dockerfile: Dockerfile
      args:
        VERSION: ${SMACKEREL_VERSION:-dev}
        COMMIT_HASH: ${SMACKEREL_COMMIT:-unknown}
        BUILD_TIME: ${SMACKEREL_BUILD_TIME:-unknown}
    env_file: config/generated/dev.env
    # ... (volumes, healthcheck, depends_on unchanged)
```

**Key decisions:**
1. Build args stay in `build.args:` — they are compile-time, not runtime
2. `postgres` and `nats` keep their own `environment:` blocks — they don't use `dev.env`
3. `env_file` path is relative to the Compose file (repo root)
4. `config/generated/dev.env` is gitignored — operators must run `config generate` first

**SST compliance:** This completes the SST pipeline. The chain becomes:
```
smackerel.yaml → config.sh → dev.env → env_file → container
```
No manual `KEY: ${KEY}` maintenance anywhere.

**Drift guard:** Add to `./smackerel.sh check`:
```bash
# Verify no individual env declarations for SST-managed services
if grep -q 'environment:' docker-compose.yml | grep -A1 'smackerel-core'; then
  echo "FAIL: smackerel-core should use env_file, not individual environment declarations"
fi
```

### Scope 7 Design: GHCR Push

**Approach:** New CI job gated on `refs/tags/v*`, uses `GITHUB_TOKEN` for GHCR auth.

**Image naming convention:**
- `ghcr.io/<owner>/smackerel-core:<version>`
- `ghcr.io/<owner>/smackerel-ml:<version>`
- Plus `:latest` tag for convenience

**Compose override mechanism:**
```yaml
services:
  smackerel-core:
    image: ${SMACKEREL_CORE_IMAGE:-}
    build:
      # ... (build context unchanged)
```

When `SMACKEREL_CORE_IMAGE` is set, Compose uses it. When empty/unset, Compose builds from Dockerfile.

---

## Phase 4: Plan — Artifact Summary

### Bug Artifacts Created

| Bug | Finding | Severity | Status | Location |
|-----|---------|----------|--------|----------|
| BUG-001 | RT-001 + SM-001 | CRITICAL (P0) | Not Started | [bugs/BUG-001-docker-compose-env-var-gap/](bugs/BUG-001-docker-compose-env-var-gap/bug.md) |
| BUG-002 | DO-002 | HIGH → RESOLVED | Already Addressed | [bugs/BUG-002-ci-docker-build-on-prs/](bugs/BUG-002-ci-docker-build-on-prs/bug.md) |
| BUG-003 | DO-003 | HIGH | Not Started (scope extension) | [bugs/BUG-003-no-ghcr-image-push/](bugs/BUG-003-no-ghcr-image-push/bug.md) |

### Scope Extensions Added to scopes.md

| Scope | Name | Priority | Finding | Status |
|-------|------|----------|---------|--------|
| 6 | Docker Compose env_file Migration | P0 | RT-001 + SM-001 | Not Started |
| 7 | GHCR Image Push on Tagged Releases | P1 | DO-003 | Not Started |

### Execution Order

1. **Scope 6 first** (P0) — unblocks all containerized feature deployment
2. **Scope 7 after** (P1) — depends on Scope 6 being resolved; requires spec non-goals amendment

### Files NOT Modified (per instructions)

- `spec.md` — not modified (scope extension noted, amendment required before Scope 7 implementation)
- `design.md` — not modified (Scope 6/7 designs are self-contained in scopes.md and bug.md)
- `state.json` — not modified (no phase transitions in planning-only chain)
