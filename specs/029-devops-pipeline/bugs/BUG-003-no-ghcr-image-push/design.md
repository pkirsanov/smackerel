# Design: BUG-003 — No Image Push to Container Registry

## Status

**Stale at analysis time (2026-04-26).** The fix premise was overtaken by parent-feature work. See "Resolution" below; no new design is required for this bug.

## Original Fix Sketch (from bug.md, retained for traceability)

### GitHub Container Registry (GHCR) Push

Add a CI job that pushes images to GHCR on tagged releases only.

```yaml
# .github/workflows/ci.yml — push-images job
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
        # Build with version metadata, tag for GHCR, docker push
```

### docker-compose.yml Image Override

Allow pulling pre-built images via `SMACKEREL_CORE_IMAGE` / `SMACKEREL_ML_IMAGE` env vars; empty value retains build-from-source behavior.

### Operator Workflow

```bash
# Build from source (existing)
./smackerel.sh build && ./smackerel.sh up

# Pull pre-built (new)
export SMACKEREL_CORE_IMAGE=ghcr.io/owner/smackerel-core:v1.2.3
export SMACKEREL_ML_IMAGE=ghcr.io/owner/smackerel-ml:v1.2.3
./smackerel.sh up
```

## Resolution

The fix is already implemented in the parent spec, not in this bug folder:

| Concern | Where it landed |
|---------|-----------------|
| Spec Non-Goals exclude GHCR | **Amended** in [specs/029-devops-pipeline/spec.md](../../spec.md) line 33: "Container registry publishing is optional — GHCR push on tagged releases is supported for self-hosted deployment convenience" |
| CI push job missing | **Implemented** in [.github/workflows/ci.yml](../../../../.github/workflows/ci.yml) lines 45–90 (login-action + tag + push, gated on `startsWith(github.ref, 'refs/tags/v')`) |
| Tracked work | **Parent Scope 7** "GHCR Image Push on Tagged Releases" — status `Done` in [specs/029-devops-pipeline/state.json](../../state.json) |
| Scenarios registered | [specs/029-devops-pipeline/scenario-manifest.json](../../scenario-manifest.json) lines 96, 108, 215 |

## Open Items (for the next agent to decide)

1. Confirm `docker-compose.yml` image-override behavior matches the original sketch (was the override env-var path also implemented, or only the CI push half?). If the override is missing, that is a smaller follow-up — track as a separate bug, not as a re-open of BUG-003.
2. Confirm `docs/Operations.md` documents the pull-based deployment path for operators. If absent, raise a docs-only follow-up.

## Files That Would Be Touched (none, given resolution path)

If the recommendation is accepted, **no source files change**. Only this bug's artifacts move to `done` with closure evidence.
