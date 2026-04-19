# BUG-003: No Image Push to Container Registry

**Finding ID:** DO-003
**Severity:** HIGH
**Classification:** Scope extension — contradicts spec Non-Goals, requires spec amendment
**Discovered:** 2026-04-19 (system review)
**Parent Spec:** [029-devops-pipeline](../../spec.md)

---

## Problem Statement

Docker images are built locally by `./smackerel.sh build` and tagged by CI on version pushes, but they are **never pushed to a container registry**. Self-hosted deployment currently requires:

1. Clone the repo
2. Run `./smackerel.sh config generate`
3. Run `./smackerel.sh build` (compiles from source — 5-10 min)
4. Run `./smackerel.sh up`

There is no way to `docker pull` a pre-built image. Rollback to a previous version requires checking out that git tag and rebuilding from source.

## Spec Conflict

The current `spec.md` explicitly lists this as a **Non-Goal**:

> **Non-Goals:**
> - Container registry publishing (DockerHub, GHCR) — images are built locally

This finding requests adding GHCR push as a **new scope**, which requires updating the spec's non-goals list.

## Business Impact

| Dimension | Impact |
|-----------|--------|
| **Operators** | Cannot deploy without build toolchain (Go 1.24, Python 3.12, Docker) |
| **Rollback** | Rollback requires `git checkout <tag>` + full rebuild — 5-10 min downtime |
| **Disk usage** | Every deployment builds all layers from scratch (no layer cache sharing) |
| **Multi-host** | Cannot deploy to a second machine without cloning the repo and building |

## Actors

- **Self-hosted operator**: Deploys Smackerel on a home server or VPS
- **CI pipeline**: Builds images on tagged releases, could push them
- **Future contributor**: Wants to test a release without building from source

## Design

### GitHub Container Registry (GHCR) Push

Add a new CI job that pushes images to GHCR on tagged releases only.

```yaml
# .github/workflows/ci.yml — new job
  push-images:
    if: startsWith(github.ref, 'refs/tags/v')
    needs: build
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
    - uses: actions/checkout@v4

    - name: Log in to GHCR
      uses: docker/login-action@v3
      with:
        registry: ghcr.io
        username: ${{ github.actor }}
        password: ${{ secrets.GITHUB_TOKEN }}

    - name: Build and push
      run: |
        VERSION="${GITHUB_REF#refs/tags/}"
        COMMIT_SHA="${GITHUB_SHA:0:12}"

        # Build with version metadata
        export SMACKEREL_VERSION="${VERSION}"
        export SMACKEREL_COMMIT="${COMMIT_SHA}"
        export SMACKEREL_BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
        ./smackerel.sh build

        # Tag for GHCR
        docker tag smackerel-smackerel-core:latest "ghcr.io/${{ github.repository_owner }}/smackerel-core:${VERSION}"
        docker tag smackerel-smackerel-core:latest "ghcr.io/${{ github.repository_owner }}/smackerel-core:latest"
        docker tag smackerel-smackerel-ml:latest "ghcr.io/${{ github.repository_owner }}/smackerel-ml:${VERSION}"
        docker tag smackerel-smackerel-ml:latest "ghcr.io/${{ github.repository_owner }}/smackerel-ml:latest"

        # Push
        docker push "ghcr.io/${{ github.repository_owner }}/smackerel-core:${VERSION}"
        docker push "ghcr.io/${{ github.repository_owner }}/smackerel-core:latest"
        docker push "ghcr.io/${{ github.repository_owner }}/smackerel-ml:${VERSION}"
        docker push "ghcr.io/${{ github.repository_owner }}/smackerel-ml:latest"
```

### docker-compose.yml Image Override

Support pulling pre-built images instead of building:

```yaml
services:
  smackerel-core:
    image: ${SMACKEREL_CORE_IMAGE:-}   # Empty = build from source
    build:
      context: .
      # ...
```

When `SMACKEREL_CORE_IMAGE` is set (e.g., `ghcr.io/owner/smackerel-core:v1.2.3`), Compose uses the pre-built image. When empty, it builds from the Dockerfile.

### Operator Workflow

```bash
# Option A: Build from source (current behavior — unchanged)
./smackerel.sh build
./smackerel.sh up

# Option B: Pull pre-built images (new)
export SMACKEREL_CORE_IMAGE=ghcr.io/owner/smackerel-core:v1.2.3
export SMACKEREL_ML_IMAGE=ghcr.io/owner/smackerel-ml:v1.2.3
./smackerel.sh up
```

## Gherkin Scenarios

```gherkin
Scenario: Tagged release pushes images to GHCR
  Given a git tag v1.0.0 is pushed
  When the CI pipeline completes successfully
  Then smackerel-core:v1.0.0 is pushed to ghcr.io
  And smackerel-ml:v1.0.0 is pushed to ghcr.io
  And both images have OCI version labels

Scenario: Operator deploys from pre-built images
  Given SMACKEREL_CORE_IMAGE is set to ghcr.io/owner/smackerel-core:v1.0.0
  When the operator runs ./smackerel.sh up
  Then Compose pulls the pre-built image instead of building
  And all services start successfully

Scenario: Operator deploys from source (backward compatible)
  Given SMACKEREL_CORE_IMAGE is not set
  When the operator runs ./smackerel.sh build && ./smackerel.sh up
  Then images are built from local Dockerfiles as before
```

## DoD (Proposed)

- [ ] CI pushes images to GHCR on tagged releases
- [ ] `docker-compose.yml` supports image override via env var
- [ ] OCI labels present on pushed images
- [ ] `spec.md` non-goals updated to remove GHCR exclusion
- [ ] `docs/Operations.md` updated with pull-based deployment instructions

## Security Considerations

- Uses `GITHUB_TOKEN` (automatic) — no additional secrets needed
- GHCR packages inherit repository visibility (private repo = private packages)
- No credentials stored in workflow files
- Image digests logged for supply-chain auditability

## Prerequisites

- Spec 029 non-goals must be amended before implementation
- BUG-001 (env_file migration) should be resolved first so pushed images have correct env handling
