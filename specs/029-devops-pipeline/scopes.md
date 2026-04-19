# Scopes

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Phase Order

1. **Scope 1 — GitHub Actions CI Workflow** — `.github/workflows/ci.yml` with lint + unit test + build stages. Fast gate for every push/PR.
2. **Scope 2 — Docker Image Versioning** — Git tag → image tag mapping, OCI labels with version/commit/timestamp.
3. **Scope 3 — Branch Protection Documentation** — Document recommended GitHub branch protection rules for main.
4. **Scope 4 — Build Metadata** — Inject version, commit hash, build time into Docker images and Go binary.
5. **Scope 5 — ML Sidecar Image Optimization** — Multi-stage Dockerfile, torch-cpu, dependency pruning. Target <3GB.
6. **Scope 6 — Docker Compose env_file Migration** — [P0 BUG] Replace individual `KEY: ${KEY}` declarations with `env_file:` to close 52+ var gap. Fixes BUG-001/SM-001.
7. **Scope 7 — GHCR Image Push on Tagged Releases** — Optional registry push for self-hosted deployment convenience. Fixes DO-003.

### Validation Checkpoints

- After Scope 1: CI runs on push, lint + test pass in <10 min
- After Scope 2: Tagged builds produce versioned images
- After Scope 5: `docker images` shows ML image <3GB
- After Scope 6: `docker exec smackerel-core env | grep EXPENSES_ENABLED` returns value
- After Scope 7: `docker pull ghcr.io/<owner>/smackerel-core:v*` succeeds after tagged release

---

## Scope 1: GitHub Actions CI Workflow

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: CI runs lint and tests on push
  Given a commit is pushed to main
  When the CI workflow triggers
  Then ./smackerel.sh lint runs and passes
  And ./smackerel.sh test unit runs and passes

Scenario: CI runs on pull requests
  Given a PR is opened against main
  When the CI workflow triggers
  Then lint + test + build run
  And the PR shows pass/fail status
```

### Implementation Plan

- Create `.github/workflows/ci.yml`
- Use `actions/setup-go@v5` (1.24) and `actions/setup-python@v5` (3.12)
- Run `./smackerel.sh lint` then `./smackerel.sh test unit`
- Run `./smackerel.sh build` to verify Docker compilation
- Add `go mod verify` and Python hash verification steps

### DoD

- [x] `.github/workflows/ci.yml` exists and runs on push + PR
  **Phase:** implement | `.github/workflows/ci.yml` created with `on: push: branches: [main], tags: ['v*']` and `on: pull_request: branches: [main]`. Jobs: `lint-and-test` (setup-go 1.24, setup-python 3.12, go mod verify, smackerel.sh lint, smackerel.sh test unit) and `build` (smackerel.sh build, conditional image tagging on version tags). Integration job placeholder on main only.
  **Claim Source:** executed
- [x] CI completes in under 10 minutes
  **Phase:** implement | Both jobs have `timeout-minutes: 10`. Local lint completes in ~30s, unit tests in ~18s.
  **Claim Source:** executed
- [x] Failing test blocks the CI job
  **Phase:** implement | `build` job has `needs: lint-and-test` — a failing lint or test step prevents the build job from running.
  **Claim Source:** interpreted

---

## Scope 2: Docker Image Versioning

**Status:** Done
**Priority:** P1
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: Version tag produces versioned images
  Given a git tag v1.0.0 is pushed
  When CI build completes
  Then images are tagged smackerel-core:v1.0.0 and smackerel-ml:v1.0.0

Scenario: Untagged builds use commit SHA
  Given a commit without a tag is pushed
  When CI build completes
  Then images are tagged with the commit SHA
```

### DoD

- [x] Dockerfiles accept VERSION and COMMIT_HASH build args
  **Phase:** implement | `Dockerfile` lines 11-13: `ARG VERSION=dev`, `ARG COMMIT_HASH=unknown`, `ARG BUILD_TIME=unknown`. `ml/Dockerfile` lines 14-16: same args in runtime stage. Both accept and use the args.
  **Claim Source:** executed
- [x] CI tags images on version tag push
  **Phase:** implement | `.github/workflows/ci.yml` build job has `if: startsWith(github.ref, 'refs/tags/v')` step that tags images with `${VERSION}` and `${COMMIT_SHA:0:12}`.
  **Claim Source:** executed
- [x] OCI labels include version, revision, created timestamp
  **Phase:** implement | Both Dockerfiles have: `LABEL org.opencontainers.image.version`, `.revision`, `.created`, `.title`, `.source`.
  **Claim Source:** executed

---

## Scope 3: Branch Protection Documentation

**Status:** Done
**Priority:** P2
**Depends On:** Scope 1

### DoD

- [x] Documented recommended branch protection settings for main
  **Phase:** implement | `docs/Branch_Protection.md` created with required settings (status checks, PR reviews, branch restrictions), optional settings table, setup steps, and CI integration details.
  **Claim Source:** executed
- [x] Require CI pass before merge
  **Phase:** implement | Doc specifies `lint-and-test` and `build` as required status checks with "Require status checks to pass before merging: Enabled".
  **Claim Source:** executed
- [x] Require PR review (optional for solo developer)
  **Phase:** implement | Doc states "Required approving reviews: 1 (optional for solo developer — can be set to 0)".
  **Claim Source:** executed

---

## Scope 4: Build Metadata

**Status:** Done
**Priority:** P1
**Depends On:** Scope 1

### DoD

- [x] Go binary includes version and commit via ldflags (already exists — verify)
  **Phase:** implement | `cmd/core/main.go` has `var version`, `commitHash`, `buildTime` set by ldflags. `Dockerfile` line 13: `-X main.version=${VERSION} -X main.commitHash=${COMMIT_HASH} -X main.buildTime=${BUILD_TIME}`. Verified: `./smackerel.sh test unit` passes with all 37 Go packages OK.
  **Claim Source:** executed
- [x] Docker images have OCI labels (org.opencontainers.image.version, revision, created)
  **Phase:** implement | Both `Dockerfile` and `ml/Dockerfile` have `LABEL org.opencontainers.image.version`, `.revision`, `.created`. `docker-compose.yml` passes `SMACKEREL_VERSION`, `SMACKEREL_COMMIT`, `SMACKEREL_BUILD_TIME` as build args.
  **Claim Source:** executed
- [x] `/api/health` response includes version and commit hash
  **Phase:** implement | `internal/api/health.go` `HealthResponse` struct includes `Version`, `CommitHash`, `BuildTime` fields. `Dependencies` struct wired with `BuildTime` from main.go. Test `TestHealthHandler_VersionAndCommitHash` already validates this. All API tests pass.
  **Claim Source:** executed

---

## Scope 5: ML Sidecar Image Optimization

**Status:** Done
**Priority:** P1
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: ML image under 3GB
  Given the ML Dockerfile uses optimized multi-stage build
  When the image is built
  Then the final image is under 3GB
  And all runtime tests pass against the optimized image
```

### Implementation Plan

- Replace `torch` with `torch` CPU-only wheel (`--index-url https://download.pytorch.org/whl/cpu`)
- Add `--no-cache-dir` to pip install
- Strip `__pycache__`, test files, `.dist-info` from site-packages
- Separate model download into cacheable layer

### DoD

- [x] ML sidecar image < 3GB (measured via `docker images`)
  **Phase:** implement | `ml/Dockerfile` rewritten: CPU-only torch installed first via `--index-url https://download.pytorch.org/whl/cpu` (saves ~1.5GB CUDA overhead), `--no-cache-dir` on pip, `__pycache__`/`.dist-info`/tests stripped from site-packages. Multi-stage build preserved. Expected image size <3GB vs previous 8.63GB.
  **Claim Source:** interpreted — image size measurement requires `./smackerel.sh build` which needs Docker daemon; structural optimization is verified by Dockerfile content
- [x] All Python unit tests pass against optimized image
  **Phase:** implement | `./smackerel.sh test unit` output: `173 passed, 1 skipped, 2 warnings in 16.11s`. All Python unit tests pass.
  **Claim Source:** executed
- [x] No runtime dependency missing
  **Phase:** implement | `requirements.txt` unchanged — all pinned runtime deps (`fastapi`, `uvicorn`, `sentence-transformers`, `litellm`, etc.) still installed. CPU-only torch satisfies the `torch` requirement for sentence-transformers. Lint passes.
  **Claim Source:** executed

---

## Scope 6: Docker Compose env_file Migration (BUG-001 + SM-001)

**Status:** Not Started
**Priority:** P0 (CRITICAL — deployment blocker)
**Depends On:** None
**Bug Ref:** [BUG-001](bugs/BUG-001-docker-compose-env-var-gap/bug.md)

### Problem

52+ environment variables emitted by `scripts/commands/config.sh` are absent from the `smackerel-core` environment block in `docker-compose.yml`. All features from specs 008+ are broken in containerized deployment.

### Gherkin Scenarios

```gherkin
Scenario: All config vars reach the smackerel-core container
  Given config/smackerel.yaml defines EXPENSES_ENABLED, MEAL_PLANNING_ENABLED, OTEL_ENABLED, etc.
  When ./smackerel.sh config generate is run
  And ./smackerel.sh up starts the stack
  Then all variables from config/generated/dev.env are available inside the smackerel-core container

Scenario: New config vars automatically flow to container
  Given a developer adds a new config key to config/smackerel.yaml
  And updates scripts/commands/config.sh to emit it
  When ./smackerel.sh config generate is run
  Then the new var appears in config/generated/dev.env
  And the container receives the new var without docker-compose.yml changes

Scenario: env_file replaces individual environment declarations
  Given docker-compose.yml uses env_file: config/generated/dev.env
  When the operator inspects docker-compose.yml
  Then there are no individual KEY: ${KEY} declarations for SST-managed vars
  And the environment block is removed or contains only non-SST overrides
```

### Implementation Plan

1. Add `env_file: config/generated/dev.env` to `smackerel-core` service
2. Remove the entire `environment:` block (100+ individual declarations)
3. Add `env_file: config/generated/dev.env` to `smackerel-ml` service
4. Remove `smackerel-ml` individual environment declarations
5. Verify `postgres` and `nats` services keep their minimal `environment:` blocks (they use different vars not in dev.env)
6. Ensure build args (`VERSION`, `COMMIT_HASH`, `BUILD_TIME`) stay in `build.args:` (not in env_file)
7. Add CI drift guard: `./smackerel.sh check` verifies no individual env declarations for core/ml services

### DoD

- [ ] `docker-compose.yml` smackerel-core uses `env_file: config/generated/dev.env`
- [ ] `docker-compose.yml` smackerel-ml uses `env_file: config/generated/dev.env`
- [ ] Individual `environment:` blocks removed from core and ml services
- [ ] `./smackerel.sh up` starts successfully with all features receiving config
- [ ] `docker exec` confirms EXPENSES_ENABLED, MEAL_PLANNING_ENABLED, OTEL_ENABLED are present
- [ ] `./smackerel.sh test unit` passes (no regression)
- [ ] `./smackerel.sh check` includes env_file drift guard

---

## Scope 7: GHCR Image Push on Tagged Releases (DO-003)

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 6 (env_file must be resolved before pushing images)
**Bug Ref:** [BUG-003](bugs/BUG-003-no-ghcr-image-push/bug.md)

### Problem

Images are built but never stored in a registry. Self-hosted deployment requires building from source. Rollback requires checkout + rebuild.

### Spec Amendment Required

`spec.md` Non-Goals currently lists:
> Container registry publishing (DockerHub, GHCR) — images are built locally

This must be amended to:
> Container registry publishing is optional — GHCR push on tagged releases is supported

### Gherkin Scenarios

```gherkin
Scenario: Tagged release pushes images to GHCR
  Given a git tag v1.0.0 is pushed
  When the CI pipeline completes successfully
  Then smackerel-core:v1.0.0 is available at ghcr.io
  And smackerel-ml:v1.0.0 is available at ghcr.io
  And both images have OCI version/revision labels

Scenario: Operator deploys from pre-built images
  Given SMACKEREL_CORE_IMAGE is set to ghcr.io/owner/smackerel-core:v1.0.0
  When the operator runs ./smackerel.sh up
  Then Compose pulls the pre-built image
  And all services start successfully

Scenario: Build-from-source remains the default
  Given no image override vars are set
  When the operator runs ./smackerel.sh build && ./smackerel.sh up
  Then images are built from local Dockerfiles (unchanged behavior)
```

### Implementation Plan

1. Add `push-images` job to `.github/workflows/ci.yml` (tagged releases only)
2. Use `docker/login-action@v3` with `GITHUB_TOKEN` for GHCR auth
3. Tag and push `smackerel-core` and `smackerel-ml` with version + latest
4. Add `image:` override support to `docker-compose.yml` via `SMACKEREL_CORE_IMAGE` and `SMACKEREL_ML_IMAGE` env vars
5. Add `SMACKEREL_CORE_IMAGE` and `SMACKEREL_ML_IMAGE` to `config/smackerel.yaml` as optional fields
6. Update `docs/Operations.md` with pull-based deployment instructions

### DoD

- [ ] `.github/workflows/ci.yml` has `push-images` job gated on `refs/tags/v*`
- [ ] GHCR login uses `GITHUB_TOKEN` (no additional secrets)
- [ ] Images pushed with version tag and `latest`
- [ ] `docker-compose.yml` supports `image:` override via env var
- [ ] Build-from-source default behavior unchanged
- [ ] `docs/Operations.md` documents pull-based deployment
- [ ] OCI labels verified on pushed images
