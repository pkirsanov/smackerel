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

### Validation Checkpoints

- After Scope 1: CI runs on push, lint + test pass in <10 min
- After Scope 2: Tagged builds produce versioned images
- After Scope 5: `docker images` shows ML image <3GB

---

## Scope 1: GitHub Actions CI Workflow

**Status:** Not Started
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

- [ ] `.github/workflows/ci.yml` exists and runs on push + PR
- [ ] CI completes in under 10 minutes
- [ ] Failing test blocks the CI job

---

## Scope 2: Docker Image Versioning

**Status:** Not Started
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

- [ ] Dockerfiles accept VERSION and COMMIT_HASH build args
- [ ] CI tags images on version tag push
- [ ] OCI labels include version, revision, created timestamp

---

## Scope 3: Branch Protection Documentation

**Status:** Not Started
**Priority:** P2
**Depends On:** Scope 1

### DoD

- [ ] Documented recommended branch protection settings for main
- [ ] Require CI pass before merge
- [ ] Require PR review (optional for solo developer)

---

## Scope 4: Build Metadata

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 1

### DoD

- [ ] Go binary includes version and commit via ldflags (already exists — verify)
- [ ] Docker images have OCI labels (org.opencontainers.image.version, revision, created)
- [ ] `/api/health` response includes version and commit hash

---

## Scope 5: ML Sidecar Image Optimization

**Status:** Not Started
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

- [ ] ML sidecar image < 3GB (measured via `docker images`)
- [ ] All Python unit tests pass against optimized image
- [ ] No runtime dependency missing
