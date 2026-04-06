# Smackerel Development Guide

Smackerel is Bubbles-bootstrapped, but the runtime is not committed yet. This guide separates what exists today from the runtime contract the implementation must satisfy when the Go core, Python sidecar, Docker stack, and project CLI land.

## Current Repo State

Committed today:

- `README.md`
- `docs/smackerel.md`
- `specs/`
- `.github/`
- `.specify/memory/`

Not committed yet:

- Go runtime source trees
- Python ML sidecar source trees
- Docker Compose runtime files
- `config/smackerel.yaml`
- A project CLI such as `./smackerel.sh`

Do not claim runtime validation or runtime commands until those assets are committed.

## Commands Available Today

Use the committed Bubbles validation surface only:

| Action | Command | Purpose |
|--------|---------|---------|
| Bootstrap doctor | `bash .github/bubbles/scripts/cli.sh doctor` | Framework and bootstrap health |
| Framework validate | `timeout 1200 bash .github/bubbles/scripts/cli.sh framework-validate` | Full framework self-check |
| Artifact lint | `bash .github/bubbles/scripts/artifact-lint.sh specs/<feature>` | Artifact template and structure validation |
| Traceability guard | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/<feature>` | Traceability and guard validation |
| Regression baseline guard | `timeout 600 bash .github/bubbles/scripts/regression-baseline-guard.sh specs/<feature> --verbose` | Managed-doc and baseline drift checks |

## Required Runtime Contract

When runtime code is committed, Smackerel must adopt a single repo CLI and Docker-only workflow comparable to the stronger downstream repos.

### One CLI For Everything

The runtime command surface must converge on one entrypoint:

```bash
./smackerel.sh
```

Required command families:

| Area | Required command shape |
|------|------------------------|
| Config generation | `./smackerel.sh config generate` |
| Build | `./smackerel.sh build` |
| Fast compile or static checks | `./smackerel.sh check` |
| Lint | `./smackerel.sh lint` |
| Format | `./smackerel.sh format` |
| Unit tests | `./smackerel.sh test unit` |
| Integration tests | `./smackerel.sh test integration` |
| End-to-end tests | `./smackerel.sh test e2e` |
| Stress tests | `./smackerel.sh test stress` |
| Full dev stack | `./smackerel.sh up` |
| Stack shutdown | `./smackerel.sh down` |
| Health and status | `./smackerel.sh status` |
| Logs | `./smackerel.sh logs` |
| Cleanup | `./smackerel.sh clean smart|full|status|measure` |

Direct `go`, `python`, `docker compose`, `pytest`, `playwright`, or `npm` commands should not become the documented runtime interface. The CLI owns orchestration, config generation, build freshness checks, cleanup safety, and test environment selection.

### Docker-Only Development

The committed runtime must be Docker-only.

- Development services run in Docker containers.
- Validation and test stacks run in Docker containers.
- Local setup should not require ad-hoc host installs beyond Docker and repo prerequisites.
- The repo CLI must generate or propagate env files and Compose inputs automatically.

### Configuration Single Source Of Truth

All runtime configuration must originate from one file:

```text
config/smackerel.yaml
```

Expected generation pattern once committed:

```text
config/smackerel.yaml
  -> scripts/commands/config.sh
  -> config/generated/*
  -> docker-compose*.yml
  -> runtime env files consumed by the CLI and services
```

Rules:

- No hardcoded ports, hostnames, URLs, or secrets in source files.
- No fallback defaults such as `${VAR:-default}` or `process.env.X || 'fallback'`.
- Generated files are derived artifacts, never hand-edited sources of truth.
- Missing required config must fail loudly.

### Environment Model

The runtime must separate persistent development state from disposable test state.

| Environment | Persistence | Purpose | Allowed writes |
|-------------|-------------|---------|----------------|
| `dev` | Persistent named volumes | Daily development and manual exploration | Yes |
| `test` | Ephemeral `tmpfs` or disposable volumes | Automated integration and E2E execution | Yes |
| `validate` | Isolated Compose project + disposable storage | Validation, chaos, and certification runs | Yes |

Rules:

- Automated tests must never write to the primary persistent dev store.
- Validation or chaos flows must never run against the dev database or long-lived JetStream state.
- Reuse of running stacks must be compatibility-aware and safe to prove.

### Port And URL Discipline

When ports are introduced, they must come from the config pipeline, not from literals embedded in code or Compose files.

- External URLs use host-mapped ports.
- Internal service-to-service traffic uses Compose service DNS names and container ports.
- The CLI and generated config must make both explicit.

## Source Of Truth Documents

Once runtime code lands, these docs become the operational source of truth:

- `docs/smackerel.md` for product and architecture
- `docs/Development.md` for command surface and configuration contract
- `docs/Testing.md` for test taxonomy and environment isolation
- `docs/Docker_Best_Practices.md` for Docker lifecycle, cleanup, and freshness rules

Any runtime change that affects command surfaces, topology, storage, or test behavior must update the relevant docs in the same change set.