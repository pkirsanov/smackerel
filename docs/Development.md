# Smackerel Development Guide

Smackerel is Bubbles-bootstrapped and exposes a repo-standard runtime CLI and config pipeline. This guide documents the current command surface and the constraints the runtime must follow.

## Current Repo State

Committed:

- `README.md`
- `docs/smackerel.md`
- `specs/` (002-018, all with spec, design, scopes, reports)
- `.github/`
- `.specify/memory/`
- Go core runtime sources under `cmd/` and `internal/` (76 source files, 70 test files)
- Python ML sidecar sources under `ml/` (11 files, 2 test files)
- `docker-compose.yml` with health checks, resource limits, restart policies, NATS auth
- `config/smackerel.yaml`
- Generated environment files under `config/generated/` via `./smackerel.sh config generate`
- `./smackerel.sh`
- E2E test scripts under `tests/e2e/` (57 scripts)
- Stress test scripts under `tests/stress/` (2 scripts)

Implemented runtime capabilities:

- Capture pipeline (URL, text, voice, conversation, media group) with SSRF protection
- 5-stage semantic search (temporal intent → embed → pgvector → graph expand → LLM rerank)
- Daily digest generation with Telegram delivery and retry
- Knowledge graph linking (4 strategies: similarity, entity, topic, temporal) — wired into pipeline
- Telegram bot (share-sheet, forwards, conversation assembly, media groups, 7 commands)
- Web UI (HTMX semantic search, artifact detail, digest, topics, settings, status)
- 14 passive connectors (Gmail API, Google Calendar API, YouTube API, RSS/Atom, Bookmarks, Browser, Google Keep/Takeout, Google Maps, Hospitable STR, Discord, Twitter/X archive, Weather via Open-Meteo, Government Alerts via USGS, Financial Markets via Finnhub/CoinGecko)
- Intelligence engine (synthesis at 2AM, momentum hourly, resurfacing at 8AM, overdue alerts)
- OAuth2 flow with CSRF protection, token storage, auto-refresh
- Data export endpoint with cursor pagination (JSONL streaming)
- Database migrations (8 SQL files)
- NATS JetStream with token authentication (5 streams: ARTIFACTS, SEARCH, DIGEST, KEEP, SYNTHESIS)
- Security: CSP, rate limiting, dedup unique index, config validation, body size limits

Do not bypass `./smackerel.sh` with ad-hoc `go`, `python`, `pytest`, or `docker compose` commands as the normal repo workflow.

## Commands Available Today

Use `./smackerel.sh` for runtime work and keep the committed Bubbles validation surface for framework/artifact governance:

| Action | Command | Purpose |
|--------|---------|---------|
| Generate config | `./smackerel.sh config generate` | Render environment files from `config/smackerel.yaml` |
| Build images | `./smackerel.sh build` | Build the Go core and Python sidecar images |
| Check compose wiring | `./smackerel.sh check` | Validate generated config and docker-compose interpolation |
| Lint | `./smackerel.sh lint` | Run Go and Python linting in containers |
| Format | `./smackerel.sh format` | Format Go and Python sources in containers |
| Unit tests | `./smackerel.sh test unit` | Run Go and Python unit tests |
| Integration tests | `./smackerel.sh test integration` | Run live-stack foundation integration validation |
| E2E tests | `./smackerel.sh test e2e` | Run compose start, persistence, and config-failure E2E checks |
| Stress smoke | `./smackerel.sh test stress` | Run live-stack health burst validation |
| Start stack | `./smackerel.sh up` | Start the foundation runtime |
| Stop stack | `./smackerel.sh down` | Stop the current runtime stack |
| Runtime status | `./smackerel.sh status` | Show docker status and API health |
| Runtime logs | `./smackerel.sh logs` | Show compose logs |
| Cleanup | `./smackerel.sh clean smart|full|status|measure` | Project-scoped docker cleanup |
| Bootstrap doctor | `bash .github/bubbles/scripts/cli.sh doctor` | Framework and bootstrap health |
| Framework validate | `timeout 1200 bash .github/bubbles/scripts/cli.sh framework-validate` | Full framework self-check |
| Artifact lint | `bash .github/bubbles/scripts/artifact-lint.sh specs/<feature>` | Artifact template and structure validation |
| Traceability guard | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/<feature>` | Traceability and guard validation |
| Regression baseline guard | `timeout 600 bash .github/bubbles/scripts/regression-baseline-guard.sh specs/<feature> --verbose` | Managed-doc and baseline drift checks |

## Runtime Contract

The current scaffold already uses a single repo CLI and a Docker-only workflow. New runtime work must preserve that contract instead of introducing parallel command surfaces.

### One CLI For Everything

The runtime command surface is:

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

Direct `go`, `python`, `docker compose`, `pytest`, `playwright`, or `npm` commands must not become the documented runtime interface. The CLI owns orchestration, config generation, build freshness checks, cleanup safety, and test environment selection.

### Docker-Only Development

The committed runtime is Docker-only.

- Development services run in Docker containers.
- Validation and test stacks run in Docker containers.
- Local setup should not require ad-hoc host installs beyond Docker and repo prerequisites.
- The repo CLI must generate or propagate env files and Compose inputs automatically.

### Configuration Single Source Of Truth

All runtime configuration must originate from one file:

```text
config/smackerel.yaml
```

Current generation pattern:

```text
config/smackerel.yaml
  -> scripts/commands/config.sh
  -> config/generated/dev.env and config/generated/test.env
  -> docker-compose.yml interpolation
  -> runtime env consumed by the CLI and services
```

Rules:

- No hardcoded ports, hostnames, URLs, or secrets in source files.
- No fallback defaults such as `${VAR:-default}` or `process.env.X || 'fallback'`.
- Generated files are derived artifacts, never hand-edited sources of truth.
- Missing required config must fail loudly.

### Environment Model

The runtime separates persistent development state from disposable test state.

| Environment | Persistence | Purpose | Allowed writes |
|-------------|-------------|---------|----------------|
| `dev` | Persistent named volumes | Daily development and manual exploration | Yes |
| `test` | Separate project-scoped named volumes removed by test cleanup | Automated integration and E2E execution | Yes |
| `validate` | Reserved for a future isolated Compose project | Validation, chaos, and certification runs | Yes |

Rules:

- Automated tests must never write to the primary persistent dev store.
- Validation or chaos flows must never run against the dev database or long-lived JetStream state.
- Reuse of running stacks must be compatibility-aware and safe to prove.

### Port And URL Discipline

When ports are introduced, they must come from the config pipeline, not from literals embedded in code or Compose files.

- Smackerel owns the workspace host-forwarding block `40000-49999`.
- The allocation avoids the repo blocks already used in this workspace: WanderAide `20000-26999`, QuantitativeFinance `30000-39999`, and GuestHost `50000-59999`.
- External URLs use host-mapped ports.
- Internal service-to-service traffic uses Compose service DNS names and container ports.
- The CLI and generated config must make both explicit.

Current host-forwarding allocation from `config/smackerel.yaml`:

| Environment | Area | Component | Host port | Internal port | External URL |
|-------------|------|-----------|-----------|---------------|--------------|
| `dev` | app | core | `40001` | `8080` | `http://127.0.0.1:40001` |
| `dev` | app | ml sidecar | `40002` | `8081` | `http://127.0.0.1:40002` |
| `dev` | infra | postgres | `42001` | `5432` | `postgres://127.0.0.1:42001` |
| `dev` | infra | nats client | `42002` | `4222` | `nats://127.0.0.1:42002` |
| `dev` | infra | nats monitor | `42003` | `8222` | `http://127.0.0.1:42003` |
| `dev` | infra | ollama | `42004` | `11434` | `http://127.0.0.1:42004` |
| `test` | app | core | `45001` | `8080` | `http://127.0.0.1:45001` |
| `test` | app | ml sidecar | `45002` | `8081` | `http://127.0.0.1:45002` |
| `test` | infra | postgres | `47001` | `5432` | `postgres://127.0.0.1:47001` |
| `test` | infra | nats client | `47002` | `4222` | `nats://127.0.0.1:47002` |
| `test` | infra | nats monitor | `47003` | `8222` | `http://127.0.0.1:47003` |
| `test` | infra | ollama | `47004` | `11434` | `http://127.0.0.1:47004` |

## Source Of Truth Documents

These docs are already the operational source of truth for architecture and governance. When the standardized runtime workflow lands, they must also become the source of truth for the command surface:

- `docs/smackerel.md` for product and architecture
- `docs/Development.md` for command surface and configuration contract
- `docs/Testing.md` for test taxonomy and environment isolation
- `docs/Docker_Best_Practices.md` for Docker lifecycle, cleanup, and freshness rules

Any runtime change that affects command surfaces, topology, storage, or test behavior must update the relevant docs in the same change set.