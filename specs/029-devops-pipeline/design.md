# Design: 029 — DevOps Pipeline & Image Governance

> **Spec:** [spec.md](spec.md) | **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Author:** bubbles.design
> **Date:** April 18, 2026
> **Status:** Draft

---

## Overview

This design adds CI/CD automation via GitHub Actions, Docker image version tagging, dependency verification, and ML sidecar image optimization. The CI pipeline uses the existing `./smackerel.sh` CLI surface — no ad-hoc build commands.

### Key Design Decisions

1. **GitHub Actions over alternatives** — repo already on GitHub; native integration, free tier sufficient for single-dev project
2. **Staged pipeline** — Fast gate (lint+unit ~5min) runs always; integration/E2E is a separate job triggered on main only
3. **Image versioning via git tags** — `v1.2.3` tag → `smackerel-core:v1.2.3` image. Commit SHA used for non-tagged builds
4. **ML image optimization** — Switch to `torch-cpu`, multi-stage with model caching layer, target <3GB from current 8.63GB

---

## Architecture

### CI Pipeline Stages

```
Push/PR → [Lint] → [Unit Tests] → [Build Images] → ✅
                                                      ↓ (main only)
                                     [Integration Tests] → [E2E Tests] → ✅
```

### GitHub Actions Workflow Structure

```yaml
# .github/workflows/ci.yml
name: CI
on:
  push:
    branches: [main]
    tags: ['v*']
  pull_request:
    branches: [main]

jobs:
  lint-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.24' }
      - uses: actions/setup-python@v5
        with: { python-version: '3.12' }
      - run: ./smackerel.sh lint
      - run: ./smackerel.sh test unit

  build:
    needs: lint-and-test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: ./smackerel.sh build
      # Tag images on version tag push
      - if: startsWith(github.ref, 'refs/tags/v')
        run: |
          VERSION=${GITHUB_REF#refs/tags/}
          docker tag smackerel-smackerel-core:latest smackerel-core:${VERSION}
          docker tag smackerel-smackerel-ml:latest smackerel-ml:${VERSION}
```

### ML Sidecar Image Optimization

Current image breakdown (8.63GB):
- Python 3.12 slim base: ~150MB
- PyTorch (full): ~2.2GB
- sentence-transformers + dependencies: ~1.5GB
- litellm + dependencies: ~400MB
- Other (numpy, scipy, etc.): ~4.3GB

**Optimization strategy:**
1. Use `torch` CPU-only variant: saves ~1.5GB
2. Use `--no-deps` for packages where deps are already satisfied
3. Separate model download layer (cacheable across builds)
4. Strip `__pycache__`, `.dist-info`, test files from site-packages
5. Target: <3GB

### Docker Image Versioning

```dockerfile
# Added to both Dockerfiles
ARG VERSION=dev
ARG COMMIT_HASH=unknown
ARG BUILD_TIME=unknown
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${COMMIT_HASH}"
LABEL org.opencontainers.image.created="${BUILD_TIME}"
```

### Dependency Verification

- **Go:** `go.sum` verified automatically by Go toolchain during `go mod download`
- **Python:** Add `requirements.txt` hash pinning (`==` versions with `--require-hashes`)
- **CI step:** `go mod verify` and `pip install --require-hashes`

---

## Testing Strategy

| Test Type | Coverage | Evidence |
|-----------|----------|----------|
| Unit | CI workflow YAML validated by GitHub Actions runtime | `.github/workflows/ci.yml` passes |
| Integration | Staged CI job runs `./smackerel.sh test integration` on main | CI logs |
| Manual | Image size measured after optimization | `docker images` output |

---

## Risks & Open Questions

| # | Risk | Mitigation |
|---|------|------------|
| 1 | CI runner doesn't have Docker for `./smackerel.sh build` | Use `ubuntu-latest` which includes Docker |
| 2 | ML image optimization breaks runtime dependencies | Run Python unit tests against optimized image in CI |
| 3 | GitHub Actions minutes quota | Free tier: 2000 min/month. Pipeline ~5min = ~400 runs/month |
