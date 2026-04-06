# Smackerel Docker Best Practices

Smackerel is designed as a self-hosted Docker stack. This document defines the Docker lifecycle rules the runtime must follow once it is committed.

## Core Rules

### Use Project-Scoped Docker Operations

Docker lifecycle must flow through the repo CLI:

```bash
./smackerel.sh build
./smackerel.sh up
./smackerel.sh down
./smackerel.sh status
./smackerel.sh logs
./smackerel.sh clean smart
```

Do not document `docker compose up`, `docker compose down`, or manual prune commands as the normal project workflow.

### Classify Runtime Resources Explicitly

Every runtime resource should be treated as one of:

| Class | Examples | Default handling |
|-------|----------|------------------|
| Persistent | Dev PostgreSQL, long-lived Ollama model store | Preserve by default |
| Ephemeral | Test databases, validation volumes, temp uploads | Safe to recreate |
| Cache | BuildKit cache, dependency cache volumes | Preserve unless cleanup policy says otherwise |
| Tooling | Builder images, config-generation helpers | Rebuildable |
| Monitoring | Prometheus, Grafana, tracing state | Preserve if declared persistent |

### Prefer Compose Projects, Profiles, And Labels

The runtime should use:

- Compose project names to isolate stacks by purpose
- Profiles to separate dev, test, and validation surfaces
- Labels to identify lifecycle owner, component, source hash, and cleanup class

Do not rely on `container_name` alone for grouping or cleanup.

## Build Freshness Rules

Freshness must be proven through image identity, not guessed from timestamps or `latest` tags.

Required image metadata:

- lifecycle owner label
- component or service name
- source hash
- dependency hash
- git revision or equivalent provenance
- build time

Allowed proof of freshness:

- matching source/dependency hash labels
- a generated build report tied to current inputs

Not acceptable as proof:

- `latest` tags
- image creation times alone
- assuming the running container is current because it exists

## Cleanup Model

### Smart Cleanup First

The default cleanup mode must be conservative:

```bash
./smackerel.sh clean smart
```

Smart cleanup should:

- remove stale repo-local artifacts
- remove disposable validation or test leftovers
- preserve protected persistent volumes
- preserve useful build caches unless policy thresholds require pruning
- report what it removed and why

### Full Cleanup Is Exceptional

Full cleanup is for broken Docker state or disk pressure emergencies:

```bash
./smackerel.sh clean full
```

Rules:

- Never make full cleanup the default behavior.
- Never prune persistent volumes without an explicit destructive path.
- Never run system-wide Docker cleanup before attempting project-scoped cleanup.

### Observe Before Cleaning

The CLI should expose status and measurement commands before cleanup escalates.

```bash
./smackerel.sh clean status
./smackerel.sh clean measure
```

## Test And Validation Isolation

Docker lifecycle rules must preserve isolation between environments.

- Development databases use persistent volumes.
- Test databases use disposable `tmpfs` or disposable volumes.
- Validation and chaos runs use isolated Compose projects and disposable state.
- Cleanup for test or validation must never delete the primary dev store.

## Stack Reuse And Session Safety

When runtime coordination is implemented, reuse should be compatibility-aware.

- Reuse only when the stack fingerprint matches current inputs.
- Use explicit runtime ownership or lease metadata when parallel sessions share infrastructure.
- Never tear down resources that belong to another active session without an explicit reclaim path.

## Recommended Compose Topology

Once committed, Smackerel should separate stacks by purpose:

| Stack | Purpose | Storage |
|-------|---------|---------|
| `smackerel` | Daily development | Persistent volumes |
| `smackerel-test` | Automated tests | Ephemeral storage |
| `smackerel-validate-*` | Validation, certification, or chaos | Isolated disposable storage |

## Documentation Contract

When Docker runtime files land, keep these surfaces aligned in the same change set:

- `docs/Development.md`
- `docs/Testing.md`
- `docs/Docker_Best_Practices.md`
- `.github/copilot-instructions.md`
- `.specify/memory/agents.md`

The runtime is not ready until the CLI, Compose topology, cleanup behavior, and docs all describe the same truth.