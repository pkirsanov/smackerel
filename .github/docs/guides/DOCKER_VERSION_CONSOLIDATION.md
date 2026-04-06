# Docker Version Consolidation

## Purpose

This document records the current Docker toolchain and base-image state across the workspace and defines a practical path to reduce image sprawl without breaking project-specific builds.

Workspace covered:
- `wanderaide`
- `guestHost`
- `quantitativeFinance`

## Cleanup Completed

On 2026-04-04, the following clearly stale local images were removed from the Docker host because they were outside current repo baselines and not referenced by the active container set:

- `rust:1.75-bookworm`
- `rust:1.79`
- `rust:1.80-bookworm`
- `rust:1.81-bookworm`
- `rust:1.83-bookworm`
- `rust:1.84-bookworm`
- `rust:1.85-bookworm`
- `rust:1.87-bookworm`
- `golang:1.24`
- `golang:1.26.0`
- `python:3.12-alpine`
- `node:22-alpine`
- `grafana/grafana:10.2.2`
- `prom/prometheus:v2.48.0`
- `jaegertracing/all-in-one:1.52`

Some images were only partially untagged because multiple tags pointed to the same image ID.

## Current Effective Baselines

### WanderAide

Build/runtime sources currently indicate:
- Rust build base: `rust:1.88-alpine`
- Python base: `python:3.11-slim`
- Node base: `node:18-alpine`
- Node slim base: `node:18-slim`
- Alpine runtime: `alpine:3.18`
- Nginx runtime: `nginx:alpine`

Important drift:
- Main Rust service builds use `1.88`
- Rust test helper still uses `rust:1.94`

Consequence:
- WanderAide currently requires both Rust `1.88` and `1.94` images locally
- Version-tagged cache volumes may also remain for both families

### GuestHost

Build/runtime sources currently indicate:
- Go build base: `golang:1.26-alpine`
- Go helper tooling: `golang:1.26-bookworm`
- Node build base: `node:20-alpine3.20`
- Alpine runtime: `alpine:3.20`
- Nginx runtime: `nginx:1.27-alpine`
- Postgres runtime: `postgres:15-alpine`
- Redis runtime: `redis:7-alpine`

Assessment:
- GuestHost is already internally consistent on Go `1.26`
- The bookworm vs alpine split is functional, not drift

### QuantitativeFinance

Build/runtime sources currently indicate:
- Rust build base: `rust:1.88-bookworm`
- Rust devtools image tag: `quantitativefinance/rust-devtools:1.88`
- Node config baseline: `22`
- Web dashboard Dockerfile base: `node:20-alpine`
- JVM services: `eclipse-temurin:21.0.2_13`
- Debian runtime: `debian:bookworm-slim`
- Nginx runtime: `nginx:1.27-alpine`

Important drift:
- Config declares Node `22`
- Dashboard Dockerfile still uses Node `20`

Consequence:
- Extra Node image churn without a clear source of truth
- Team can believe Node `22` is standard while Docker builds still use `20`

## Can All Projects Use The Same Version?

Yes, but only by language family, not by forcing every project into one universal version rule.

Recommended interpretation:
- One Rust baseline across Rust-based repos
- One Node baseline across Node-based repos
- One Python baseline across Python-based repos
- One Go baseline across Go-based repos

Do not try to make every project use the same exact set of images immediately. That would create unnecessary risk, especially where repos have different runtime and dependency constraints.

## Recommended Consolidation Targets

### Rust

Short-term target:
- Standardize workspace Rust on `1.88`

Why:
- WanderAide builds already use `1.88`
- QuantitativeFinance already uses `1.88`
- Only WanderAide test helper is keeping `1.94` alive

Immediate action:
- Update WanderAide Rust testing helper to derive its version from the configured Rust base instead of hardcoding `1.94`

Longer-term option:
- Upgrade both WanderAide and QuantitativeFinance together to a newer Rust baseline, but only as a deliberate cross-repo upgrade

### Node

Short-term target:
- Standardize workspace Node on `20`

Why:
- GuestHost already uses `20`
- QuantitativeFinance Dockerfile already uses `20`
- WanderAide is the only repo still on `18`
- This is the lowest-risk convergence point

Immediate action:
- Fix QuantitativeFinance config drift so declared Node version matches Docker reality
- Plan WanderAide `18` to `20` upgrade as a separate validation task

Longer-term option:
- Once all repos validate on `20`, consider a coordinated jump to a newer shared LTS baseline

### Python

Short-term target:
- Standardize workspace Python on `3.11-slim`

Why:
- WanderAide already uses it
- No active repo usage was found for `3.12-alpine`

### Go

Short-term target:
- Keep GuestHost on `1.26`

Why:
- GuestHost is the only Go-heavy repo in this workspace
- It is already consolidated

### Monitoring Images

Short-term target:
- Keep only the versions pinned by active compose files
- Avoid accumulating newer tags locally unless a repo upgrade is in progress

## Drift Prevention Model

The main problem is not a lack of pinned versions. The main problem is multiple sources of truth.

### Rule 1: One version declaration per language per repo

Each repo should declare its Docker toolchain versions in one canonical place.

Examples:
- WanderAide: keep using `infrastructure/service-config/docker-versions.yaml`
- GuestHost: keep using `docker-bake.hcl` plus generated config inputs
- QuantitativeFinance: keep using `config/quantitativefinance.yaml` and generated build args

All Dockerfiles, helper scripts, and build commands must consume those values instead of hardcoding their own versions.

### Rule 2: No helper script may hardcode a version that differs from the repo baseline

Examples of what to forbid:
- Hardcoded `rust:1.94` in a test helper when the repo baseline is `1.88`
- Config declaring `node 22` while Dockerfiles still use `node 20`

### Rule 3: Add drift checks to each repo

Each repo should have a lightweight validation script that checks:
- Dockerfiles match canonical version inputs
- Helper scripts match canonical version inputs
- Compose/bake args match canonical version inputs
- Documentation does not claim a different active version

This check should run in CI and fail fast on mismatch.

### Rule 4: Add workspace-level review for planned convergence

Use a single workspace document like this one to track:
- Current repo baselines
- Target shared baselines
- Approved exceptions
- Pending migrations

This avoids each repo drifting independently.

## Practical Implementation Options

### Option A: Per-repo source of truth plus per-repo drift lint

This is the lowest-risk model.

How it works:
- Each repo keeps its own canonical version file
- Each repo adds a script that validates Dockerfiles and helper scripts against it
- CI blocks drift

Pros:
- Minimal restructuring
- Compatible with current repo layouts
- Easy to adopt incrementally

Cons:
- Cross-repo convergence still requires manual coordination

### Option B: Workspace-level shared toolchain catalog

This is the strongest consistency model.

How it works:
- A shared catalog defines approved workspace baselines by language family
- Repos import or mirror those values
- Repos may declare explicit exceptions with justification

Pros:
- Makes workspace-wide standardization straightforward
- Easier to review and upgrade centrally

Cons:
- Higher coupling between repos
- Requires governance around exceptions and rollout timing

### Recommended Hybrid

Adopt both layers:
- Per repo: canonical version file plus drift lint
- Workspace: shared target-baseline document plus coordinated upgrades

This gives local autonomy with central visibility.

## Immediate Follow-Up Actions

### High priority

1. WanderAide: remove the hardcoded `rust:1.94` test helper usage and derive the test image version from the configured Rust baseline.
2. QuantitativeFinance: align config and Dockerfile on one Node version. Prefer `20` now unless there is a deliberate decision to upgrade everything to `22`.
3. Add per-repo drift checks for Docker toolchain versions.

### Medium priority

1. Audit duplicate named volumes, especially old node-modules caches and duplicate WanderAide volume families.
2. Clean orphaned disposable volumes with label-aware filters instead of broad volume pruning.
3. Review whether GuestHost should keep both Alpine and Bookworm Go helper images or standardize helper tooling on one base family.

### Lower priority

1. Plan a coordinated Rust upgrade across WanderAide and QuantitativeFinance.
2. Plan a coordinated Node upgrade across WanderAide, GuestHost, and QuantitativeFinance.

## Decision Summary

- Do not force one universal Docker version policy across unrelated languages.
- Do converge each language family to one baseline where practical.
- Today, the best workspace convergence targets are:
  - Rust `1.88`
  - Node `20`
  - Python `3.11-slim`
  - Go `1.26`
- Prevent future drift by eliminating helper-script hardcoding and adding CI drift validation in every repo.
