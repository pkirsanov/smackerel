# Smackerel Development Guide

Smackerel is Bubbles-bootstrapped and exposes a repo-standard runtime CLI and config pipeline. This guide documents the current command surface and the constraints the runtime must follow.

## Current Repo State

Committed:

- `README.md`
- `docs/smackerel.md`
- `specs/` (feature packets through 054 in this checkout, all with spec, design, scopes, reports)
- `.github/`
- `.specify/memory/`
- Go core runtime sources under `cmd/` and `internal/` (154 source files, 152 test files)
- Python ML sidecar sources under `ml/` (17 source files, 16 test files)
- `docker-compose.yml` with health checks, resource limits, restart policies, NATS auth
- `config/smackerel.yaml`
- Generated environment files under `config/generated/` via `./smackerel.sh config generate`
- `./smackerel.sh`
- E2E test scripts under `tests/e2e/` (59 scripts)
- Stress test scripts under `tests/stress/` (2 scripts)

Implemented runtime capabilities:

- Capture pipeline (URL, text, voice, conversation, media group) with SSRF protection
- 5-stage semantic search (temporal intent → embed → pgvector → graph expand → LLM rerank)
- Daily digest generation with Telegram delivery and retry
- Knowledge graph linking (4 strategies: similarity, entity, topic, temporal) — wired into pipeline
- Telegram bot (share-sheet, forwards, conversation assembly, media groups, 9 commands)
- Web UI (HTMX semantic search, artifact detail, digest, topics, settings, status, knowledge dashboard)
- 17 passive connectors (IMAP email, CalDAV calendar, YouTube API, RSS/Atom, Bookmarks, Browser, Google Keep/Takeout, Google Maps, Hospitable STR, GuestHost STR, Discord, Twitter/X archive, Weather via Open-Meteo, Government Alerts via USGS, Financial Markets via Finnhub/CoinGecko, QF Decisions companion via spec 041 read-only packet flow, Card Rewards rotating-category source via spec 083 read-only fetch)
- Cloud Drives integration (spec 038): Google Drive provider with OAuth `BeginConnect`/`FinalizeConnect`, scan + monitor loop on the `DRIVE` NATS stream, classification + sensitivity policy, Save Rules engine with audit/dry-run, Save Service with confirmations (`/v1/drive/confirmations/{id}`), Drive search/artifact-detail surface (`/v1/drive/artifacts/{id}`), agent retrieval tools, and 17 `drive_*` schema tables
- Cloud Photo Libraries integration (spec 040): provider-neutral `photolib.PhotoLibrary` contract with Immich and PhotoPrism adapters, capability taxonomy SST, lifecycle/duplicates/removal analyzers, scope-hash `PhotoActionToken` mint/confirm flow, sensitivity reveal tokens (`/v1/photos/{id}/reveal`), unified `/v1/photos/upload` capture pipeline shared by Telegram/PWA/web, cross-feature routing (8 RouteTargets), and 7 `photo_*` schema tables
- Notification Intelligence Handler (spec 054): source-neutral `internal/notification` service with source adapter contracts, source health, raw event persistence, normalized notifications, classification rationale, incident correlation, decisions, suppressions, redacted delivery attempts, and authenticated `/api/notifications/*` operator endpoints
- ntfy source adapter (spec 055): `internal/notification/source/ntfy` implements the concrete ntfy source adapter, runtime startup from `NTFY_SOURCES_JSON`, webhook ingest, stream transport hooks, topic health, reconnect state, dead-letter records, replay-through-source-sink controls, and HTMX operator pages while keeping ntfy transport out of the core handler
- 17 connector source families across `internal/connector/` and `internal/drive/`
- Intelligence engine (synthesis at 2AM, momentum hourly, resurfacing at 8AM, overdue alerts)
- Knowledge synthesis layer (concept pages, entity profiles, cross-source connections, lint auditing, prompt contract validation)
- Domain extraction pipeline (recipe and product schemas) with NATS-backed async processing and Prometheus metrics
- User annotations (freeform ratings, tags, notes, interactions) with Telegram reply-based annotation and materialized summaries
- Actionable lists (shopping, reading, product comparison) with domain-aware aggregation and completion tracking
- Observability (Prometheus metrics for ingestion, search, connector sync, domain extraction, NATS dead-letter; W3C trace propagation via NATS headers)
- PWA share target for mobile capture and browser extension (Chrome MV3 / Firefox) for desktop capture
- OAuth2 flow with CSRF protection, token storage, auto-refresh
- Data export endpoint with cursor pagination (JSONL streaming)
- Database migrations through `060_artifact_evergreen_signal.sql`; migrations 002-017 remain consolidated into `001_initial_schema.sql`
- NATS JetStream with token authentication (11 streams: ARTIFACTS, SEARCH, DIGEST, KEEP, INTELLIGENCE, ALERTS, SYNTHESIS, DOMAIN, ANNOTATIONS, LISTS, DEADLETTER)
- Security: CSP, rate limiting, dedup unique index, config validation, body size limits
- CI/CD pipeline (GitHub Actions workflows, Docker image versioning, branch protection)

QF companion connector status (`specs/041-qf-companion-connector/`):

- Current Scope 1 implementation: `qf-decisions` config/env wiring, connector registration, explicit QF bridge contract validation, read-only `GET` client calls to the QF private Smackerel surface, and health mapping where schema compatibility errors are degraded while other bridge validation errors are error health.
- Current Scope 1 boundary: `Sync()` validates the QF read contract and returns zero artifacts, so it does not publish QF packets, search cards, digest items, Telegram cards, or evidence bundles.
- Certification state: Scope 1 is implemented but not certified complete because fresh uncontended live integration and E2E evidence is still required.
- Later-scope contract: read-only ingestion of QF `DecisionPacket` envelopes, QF trust metadata preservation, web/Telegram/digest/search surfacing, and `PersonalEvidenceBundle` export back to QF remain tied to the later scopes in spec 041 and the QF 063 read/outbox readiness gate.

Do not bypass `./smackerel.sh` with ad-hoc `go`, `python`, `pytest`, or `docker compose` commands as the normal repo workflow.

## Commands Available Today

Use `./smackerel.sh` for runtime work and keep the committed Bubbles validation surface for framework/artifact governance.

**Global flag:** `--env dev|test` selects the target environment (default: `dev`). The test environment uses separate Compose project names and port ranges to avoid colliding with the dev stack.

| Action | Command | Purpose |
|--------|---------|---------|
| Generate config | `./smackerel.sh config generate` | Render environment files from `config/smackerel.yaml` |
| Build images | `./smackerel.sh build [--no-cache]` | Build the Go core and Python sidecar images |
| Check compose wiring | `./smackerel.sh check` | Validate generated config and docker-compose interpolation |
| Lint | `./smackerel.sh lint` | Run Go vet, Python ruff, and web asset validation |
| Format | `./smackerel.sh format` | Format Go and Python sources in containers |
| Package extension | `./smackerel.sh package extension` | Package browser extension for Chrome and Firefox distribution |
| Unit tests | `./smackerel.sh test unit [--go\|--python]` | Run Go and Python unit tests (or one language only) |
| Integration tests | `./smackerel.sh test integration` | Run live-stack foundation integration validation |
| E2E tests | `./smackerel.sh test e2e` | Run compose start, persistence, and config-failure E2E checks |
| Stress smoke | `./smackerel.sh test stress` | Run disposable test-stack shell and Go stress validation |
| Start stack | `./smackerel.sh up` | Start the foundation runtime |
| Stop stack | `./smackerel.sh down [--volumes]` | Stop the current runtime stack; optionally remove named volumes |
| Backup database | `./smackerel.sh backup` | Create a compressed pg_dump backup in `backups/` |
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
| Build | `./smackerel.sh build [--no-cache]` |
| Fast compile or static checks | `./smackerel.sh check` |
| Lint | `./smackerel.sh lint` |
| Format | `./smackerel.sh format` |
| Unit tests | `./smackerel.sh test unit [--go\|--python]` |
| Integration tests | `./smackerel.sh test integration` |
| End-to-end tests | `./smackerel.sh test e2e` |
| Stress tests | `./smackerel.sh test stress` |
| Full dev stack | `./smackerel.sh up` |
| Stack shutdown | `./smackerel.sh down [--volumes]` |
| Database backup | `./smackerel.sh backup` |
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
- Docker Compose and deploy specs use fail-loud required interpolation such as `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}`. If loopback is desired, it must be an explicit SST/generated env value, not a Compose fallback.
- Generated files are derived artifacts, never hand-edited sources of truth.
- Missing required config must fail loudly.

### Notification Intelligence SST (Spec 054)

The source-neutral notification intelligence handler is configured only through
`config/smackerel.yaml`. `scripts/commands/config.sh` reads the keys with
`required_value` / `required_json_value` and emits the corresponding
`NOTIFICATION_*` env vars into `config/generated/dev.env` and
`config/generated/test.env`. Runtime loading then fails loud in
`internal/config/notification.go` if any emitted key is missing, malformed, or
empty.

| YAML path | Generated env var | Current committed value | Runtime purpose |
|-----------|-------------------|-------------------------|-----------------|
| `notification_intelligence.enabled` | `NOTIFICATION_INTELLIGENCE_ENABLED` | `true` | Enables the handler and its authenticated operator endpoints. |
| `notification_intelligence.persistence_threshold` | `NOTIFICATION_PERSISTENCE_THRESHOLD` | `2` | Number of correlated events that can trigger escalation by persistence. |
| `notification_intelligence.escalation_severity` | `NOTIFICATION_ESCALATION_SEVERITY` | `high` | Minimum severity that can trigger user-facing escalation. Valid values are `medium`, `high`, and `critical`. |
| `notification_intelligence.low_confidence_threshold` | `NOTIFICATION_LOW_CONFIDENCE_THRESHOLD` | `0.55` | Classifier confidence floor below which the decision engine chooses diagnostics instead of pretending certainty. |
| `notification_intelligence.max_retries` | `NOTIFICATION_MAX_RETRIES` | `2` | Bounded retry budget for notification reaction/output handling. |
| `notification_outputs.channels` | `NOTIFICATION_OUTPUT_CHANNELS` | `["dashboard"]` | Output-channel list consumed by the decision policy and dispatcher. Channels stay separate from source adapters. |

Implementation references:

- `scripts/commands/config.sh` owns SST extraction and env-file emission for the six notification env vars.
- `internal/config/notification.go` parses and validates the env vars into `config.NotificationConfig`.
- `cmd/core/wiring.go` converts `config.NotificationConfig` into `notification.DecisionPolicy` and wires `api.NewNotificationHandlers`.
- `internal/notification/config_validation.go` contains package-level validation used by notification tests.

Do not add ntfy-specific settings under these core blocks. Spec 055 owns ntfy
adapter configuration, subscription transport, topic mapping, and ntfy payload
translation into the spec 054 source adapter contract.

### ntfy Source Adapter SST (Spec 055)

The concrete ntfy adapter is configured through
`notification_sources.ntfy.instances` in `config/smackerel.yaml`. The config
generator reads that array with `required_json_value` and emits it as
`NTFY_SOURCES_JSON` into each generated env file. `internal/config/config.go`
marks `NTFY_SOURCES_JSON` as required, and `cmd/core/wiring.go` calls
`ntfy.BootstrapConfiguredSources` and `ntfy.StartConfiguredAdapters` during
startup. A missing JSON array, malformed JSON, or invalid enabled source
instance stops startup loudly.

Each ntfy source instance currently supports these SST fields:

| YAML field | Runtime meaning |
|------------|-----------------|
| `source_instance_id` | Stable source identity used by health, events, dead letters, replay attempts, and UI/API routes. |
| `enabled` | Starts or skips the instance. |
| `source_form` and `transport_mode` | Must match exactly and be either `stream` or `webhook`. |
| `endpoint_url` | Transport endpoint. Webhook mode uses the Smackerel API route for the source. |
| `endpoint_ref_name` | Redacted endpoint identity surfaced in status and detail responses. |
| `topics` | Explicit topic allowlist; unconfigured topics are rejected. |
| `auth_mode` | `none`, `bearer_token`, or `basic`; `none` must be explicit to allow zero secret references. |
| `secret_ref_names` | Secret reference names only for credential-backed modes. Secret values stay outside status, logs, UI, and dead letters. |
| `default_domain`, `topic_subjects`, `tag_services`, `tag_intents` | Mapping hints that the core classifier may use; they are not policy decisions. |
| `retry_budget`, `initial_delay_seconds`, `max_delay_seconds`, `keepalive_timeout_seconds` | Positive reconnect policy values. |
| `lag_degraded_after_seconds`, `lag_disconnected_after_seconds` | Positive lag thresholds; degraded must be lower than disconnected. |
| `dead_letter_retry_budget`, `max_payload_bytes`, `pressure_threshold_count` | Positive dead-letter/replay controls and payload ceiling. |
| `display_name`, `endpoint_label`, `config_hash` | Redacted operator display and drift/audit metadata. |

Implementation references:

- `scripts/commands/config.sh` emits `NTFY_SOURCES_JSON` from `notification_sources.ntfy.instances`.
- `internal/notification/source/ntfy/config_json.go` parses the JSON and bootstraps enabled source instances.
- `internal/notification/source/ntfy/types.go` validates identity, source form, auth mode, secret references, config hash, redacted metadata, and positive policy values.
- `cmd/core/wiring.go` starts the ntfy runtime and wires the webhook receiver plus ntfy operational store into notification handlers.
- `internal/api/notifications_ntfy.go` owns ntfy detail, webhook, reconnect, dead-letter, and replay API handlers.

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

**Per-user bearer auth (spec 044)** is disabled by default in `dev` and `test`
(`environments.dev.auth_enabled=false`, `environments.test.auth_enabled=false`).
The legacy shared `SMACKEREL_AUTH_TOKEN` flow remains the local-development
contract; no per-user enrollment is required for `./smackerel.sh up`,
`./smackerel.sh test unit`, or `./smackerel.sh test integration`. The per-user
PASETO subsystem and operator runbook live under `internal/auth/` and are
documented for production-class deployments in
[Operations.md](../docs/Operations.md#per-user-bearer-authentication-spec-044).

In `dev` and `test`, the Scope 02 MIT-closure handlers (photos `MintReveal`,
cloud-drive `Connect`, user-annotation create) continue to honor body-supplied
`actor_id` / `owner_user_id` / `actor_source` and the `X-Actor-Id` header, so
existing local-dev scripts and integration fixtures work unchanged. The
production-mode rejection (HTTP 400) only fires when `auth.enabled=true` AND
`runtime.environment=production`. The hot-path middleware lives at
`internal/api/router.go` (`(*Dependencies).bearerAuthMiddleware`).

#### Spec 044 Scope 03 Dev Notes (Web Surfaces + Telegram + Admin UI)

Scope 03 adds four new caller-side surfaces, each with its own extension point.
The same shared-token dev contract applies to all of them — none require
per-user enrollment for local development.

**Adding a web surface that uses cookie-derived sessions.** The browser-side
contract is `POST /v1/web/login` → `Set-Cookie: auth_token=...; HttpOnly` →
subsequent requests carry the cookie. The login handler lives at
[`internal/api/web_login.go`](../internal/api/web_login.go) and the cookie
fallback in `extractBearerToken` lives in
[`internal/api/router.go`](../internal/api/router.go) (the same
`bearerAuthMiddleware` accepts the bearer from either the `Authorization`
header OR the `auth_token` cookie). New web routes that require auth should be
mounted under a `chi.Group` with `r.Use(deps.bearerAuthMiddleware)` — the
session is attached before the handler runs and is reachable via
`auth.SessionFromContext(r.Context())` /
`auth.UserIDFromContext(r.Context())`. The login route itself MUST stay
outside `bearerAuthMiddleware` (it is the entry point) and SHOULD be
rate-limited (the existing 20-req/IP/min `httprate.LimitByIP` Group is the
canonical pattern).

**Extending the Telegram bridge for new user mappings.** Chat → user
resolution lives at
[`internal/telegram/user_mapping.go`](../internal/telegram/user_mapping.go).
`ParseUserMapping(raw string)` is the SST parser; `Bot.resolveActorUserID(chatID)`
is the runtime lookup. Production with an unmapped chat returns
`ErrNoUserMappingForChat` and the calling handler MUST drop the message; dev
and test return `("", nil)` so existing single-user dev flows keep working.
The mapping is parsed once at startup from the `TELEGRAM_USER_MAPPING` env var
(format: `<chat_id>:<user_id>` comma-separated) — there is no hot-reload. To
add a new mapping at runtime, edit `telegram.user_mapping` in
`config/smackerel.yaml`, run `./smackerel.sh config generate`, and restart
the stack.

The companion library
[`internal/telegram/per_user_token.go`](../internal/telegram/per_user_token.go)
authors `PerUserTokenMinter`, which mints short-lived per-user PASETO bearers
keyed by the resolved `user_id`. Spec 044 Scope 04 closes the F02
deferred-finalize-blocker by wiring this minter into the bot's outbound
HTTP calls via `Bot.bearerForChat(chatID)` and the
`Bot.setBearerHeader(req, chatID)` helper
([`internal/telegram/bot.go`](../internal/telegram/bot.go) lines 200–254).
Production wiring is performed by `startTelegramBotIfConfigured`
([`cmd/core/wiring.go`](../cmd/core/wiring.go)) when `auth.enabled=true`
AND `auth.signing.active_private_key` is configured. See the F02 closure
section in
[Operations.md](../docs/Operations.md#f02-closure-scope-04-shipped) for
the live decision matrix and the auth metrics surface
([`internal/metrics/auth.go`](../internal/metrics/auth.go)) used to
monitor the deprecation pathway.

**Extending the admin token-management UI.** The single embedded HTML page
lives at
[`internal/api/admin_ui_static/tokens.html`](../internal/api/admin_ui_static/tokens.html)
and is served by `HandleAdminTokensUI` in
[`internal/api/admin_ui.go`](../internal/api/admin_ui.go) via `//go:embed
admin_ui_static/tokens.html`. The page calls the existing Scope 02
`/v1/auth/*` admin REST endpoints via `fetch()` with `credentials:
'same-origin'` (the cookie set by `/v1/web/login` carries the admin
session). Three constraints when extending:

- All response data MUST be rendered with `textContent` + `appendChild` —
  never `innerHTML` for response data (XSS-safe rendering policy).
- The strict CSP header set by `HandleAdminTokensUI` (`default-src 'none';
  style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self';
  base-uri 'none'; form-action 'none'`) forbids external script/style
  loads, image loads, font loads, and form submissions — keep all UI logic
  inline.
- The page handler enforces ONLY `bearerAuthMiddleware` admit; admin-scope
  enforcement happens at the underlying `/v1/auth/*` XHR layer via
  `AuthAdminHandlers.callerIsAdmin`. Do NOT duplicate the admin gate at the
  page layer (defense-in-depth at the XHR layer is stronger — see
  design.md §16.1 row 2 for the rationale).

**Build-tag conventions for Scope 03 tests.** Per the live-stack
classification in `docs/Testing.md`:

| Surface | Test files | Build tag |
|---|---|---|
| PWA cookie-derived session E2E | `tests/e2e/auth/pwa_per_user_test.go` | `e2e` |
| Browser extension Authorization-header forward | `tests/integration/auth_extension_test.go` | `integration` |
| Telegram bridge per-user mint + bridge | `tests/integration/auth_telegram_e2e_test.go` | `integration` |
| Admin UI page + headers + method allowlist | `tests/integration/auth_admin_ui_test.go` | `integration` |
| Scope 03 chaos behaviors | `tests/integration/auth_chaos_scope03_test.go` | `integration` |
| Web login handler unit tests | `internal/api/web_login_test.go` | (none — default lane) |
| Per-user token minter unit tests | `internal/telegram/per_user_token_test.go` | (none) |
| User mapping parser + resolver unit tests | `internal/telegram/user_mapping_test.go` | (none) |

The `tests/integration/auth_*_e2e_test.go` files use the `integration` tag
(NOT `e2e`) by deliberate choice during the Scope 03 follow-up implement pass
— assembling the live PostgreSQL + revocation cache + Telegram-bot wiring
in-process via `httptest.NewServer(api.NewRouter(deps))` is substantially
simpler under the `integration` tag than under the multi-process `e2e`
runner. The functional contract is identical: real PostgreSQL on
`127.0.0.1:47001`, real PASETO mint via `auth.IssueToken`, real
`RevocationCache`, real `bearerAuthMiddleware`. Only the PWA cookie-derived
session test file remains under the `e2e` tag because it is the discharge
test for `FINALIZE-PREREQ-044-V7-001`.

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

### Build Tag Discipline

Most live-stack tests under `tests/e2e/` and `tests/integration/` use Go build tags to keep the default `./smackerel.sh test unit` run free of live-stack dependencies. Two tags carry contractual meaning today:

| Tag | Owner | What it gates | Default test lane behavior |
|-----|-------|---------------|-----------------------------|
| `e2e` | spec 037 + general E2E | Live-stack agent + API E2E tests that depend on the test compose stack | Excluded by `./smackerel.sh test unit`; included by `./smackerel.sh test e2e` |
| `e2e_ollama` | spec 043 | `tests/e2e/agent/happy_path_test.go` (the only file in the repo using this tag) — drives the production NATS + Python sidecar + LiteLLM + Ollama path against a real local model | Excluded from every default lane; included only when `SMACKEREL_TEST_OLLAMA=1 ./smackerel.sh test e2e` is invoked |

Constraints:

- A combined `go test -tags=e2e,e2e_ollama ./tests/e2e/agent/...` invocation is structurally invalid today: helper symbols (`postInvoke`, `liveDB`) are redeclared between `happy_path_test.go` and the spec-037 `e2e`-tagged helpers because the two helper signatures differ. `./smackerel.sh test e2e` (with `SMACKEREL_TEST_OLLAMA=1`) builds with `-tags=e2e_ollama` only, never the combined form.
- The non-tagged `tests/e2e/agent/no_skip_guard_test.go` file MUST remain compilable under every tag config (default, `e2e`, `e2e_ollama`) so its `t.Skip`-bailout regression continues to fire on every test invocation.
- See `docs/Testing.md` "Ollama-Backed Agent E2E Test Lane (Spec 043)" for the canonical operator workflow, env-var contract, exit codes from `scripts/commands/ollama-test-pull.sh`, and the per-environment `agent_provider_fast_model` override (`qwen2.5:0.5b-instruct` for `test`, `gpt-oss:20b` for `dev` and `home-lab`).

## Source Of Truth Documents

These docs are already the operational source of truth for architecture and governance. When the standardized runtime workflow lands, they must also become the source of truth for the command surface:

- `docs/smackerel.md` for product and architecture
- `docs/Development.md` for command surface and configuration contract
- `docs/Testing.md` for test taxonomy and environment isolation
- `docs/Docker_Best_Practices.md` for Docker lifecycle, cleanup, and freshness rules

Any runtime change that affects command surfaces, topology, storage, or test behavior must update the relevant docs in the same change set.

## Go Packages (`internal/`)

| Package | Purpose |
|---------|---------|
| `internal/annotation/` | User annotation model, freeform parser (ratings, tags, interactions, notes), PostgreSQL store, materialized summary view |
| `internal/api/` | Chi router, REST API handlers (capture, search, digest, export, knowledge, annotations, lists, expense API (7 endpoints: query, export CSV, correction, classification, suggestions), meal plan API (12 endpoints), recipe domain scaling endpoint, notification intelligence operator API), Bearer auth, security headers, rate limiting |
| `internal/auth/` | Two coexisting subsystems: (1) OAuth2 provider abstraction, token exchange/refresh, Google OAuth scopes, encrypted token storage (`oauth.go`, `handler.go`, `store.go`); (2) spec 044 per-user PASETO v4.public bearer auth (`issue.go`, `verify.go`, `hash.go`, `session.go`, `startup.go`, `bearer_store.go`) plus `revocation/` cache + NATS broadcaster. Per-user surface is gated by `auth.enabled` per environment; dev/test default to off (legacy shared `SMACKEREL_AUTH_TOKEN` ergonomic preserved) and home-lab defaults to on. |
| `internal/cardrewards/` | Spec 083 Card Rewards Companion domain — PostgreSQL-backed card catalog/wallet/offers/selections/bonuses/category-alias store, deterministic card-name resolver, one-time CCManager JSON→PG importer, and the strict-schema LLM rotating-category extraction orchestrator (`extract.go`; the model-gateway call lives in the Python sidecar per Constitution C2) |
| `internal/config/` | SST-compliant configuration loader — reads all env vars, validates required fields, parses numeric config, cross-validates constraints |
| `internal/connector/` | Connector interface, registry, supervisor (5-min sync cycles), health status model. Sub-packages per connector: `alerts/`, `bookmarks/`, `browser/`, `caldav/`, `cardrewards/`, `discord/`, `guesthost/`, `hospitable/`, `imap/`, `keep/`, `maps/`, `markets/`, `qfdecisions/` (Scope 1 QF bridge validation only; no artifact publication yet), `rss/`, `twitter/`, `weather/`, `youtube/`, plus the `photos/` provider-neutral library and adapters under `photos/adapters/{immich,photoprism}/` |
| `internal/drive/` | Spec 038 cloud-drives surface — `DriveProvider` interface, `google/` provider implementation (OAuth `BeginConnect`/`FinalizeConnect`, plaintext bearer in `drive_connections.credentials_ref` per design.md §2.3 + decision-log A1), `scan/` + `monitor/` loops on the `DRIVE` NATS stream, `extract/` content extraction, `rules/` Save Rules engine, `save/` Save Service, `confirm/` low-confidence confirmation handler, `policy/` sensitivity policy enforcement, `retrieve/` retrieval service, `tools/` agent tool registrations, `health/` cursor durability and bounded rescan, `consumers/` NATS consumer wiring, `memprovider/` in-memory test provider |
| `internal/db/` | PostgreSQL connection pool wrapper, migration runner (embed.FS), artifact CRUD, export with cursor pagination, guest/property repos |
| `internal/digest/` | Daily digest assembly (action items, overnight artifacts, hot topics, hospitality context, knowledge health, expense digest section (summary, needs-review, suggestions, missing receipts, word limit enforcement)), LLM generation via NATS, Telegram delivery with retry |
| `internal/domain/` | Domain extraction schema registry — maps artifact content types to prompt contracts for structured extraction (recipes, products), expense metadata types, vendor alias types |
| `internal/extract/` | Content extraction from URLs — HTML readability, YouTube transcript fetching, media type detection, SSRF-safe HTTP client |
| `internal/graph/` | Knowledge graph linker — 4 strategies (similarity, entity, topic, temporal), bidirectional edge creation, connection counting |
| `internal/intelligence/` | Intelligence engine — cross-domain synthesis, expertise mapping (R-501), learning paths (R-502), subscription detection (R-504), serendipity resurfacing (R-505), content fuel (R-506), quick references (R-507), monthly reports (R-508), seasonal patterns, momentum tracking, alerts, expense classification (7-level rule chain), vendor normalization (LRU cache + pre-seeded aliases), expense suggestion generation |
| `internal/knowledge/` | Knowledge synthesis layer — concept pages, entity profiles, lint reports, cross-source connection assessment, store with trigram search, prompt contract integration |
| `internal/list/` | Actionable list model — lists and list items, domain-aware aggregation from extracted data, completion tracking, PostgreSQL store |
| `internal/metrics/` | Prometheus metrics (ingestion, capture, search latency, domain extraction, connector sync, NATS dead-letter counters, DB connection gauge, **eight bounded recommendation metrics — see `docs/Operations.md` Recommendations table**) and W3C traceparent propagation via NATS headers |
| `internal/nats/` | NATS JetStream client — stream/consumer creation, publish/subscribe helpers, subject constants matching `config/nats_contract.json` |
| `internal/notification/` | Spec 054 source-neutral notification intelligence handler — source adapter contract and registry, redacted source health, raw event and normalized notification stores, classifier, dedupe/correlation, incident model, decision engine, diagnostics/action/approval policy helpers, output dispatcher, and redaction guard. Core code must not import or branch on ntfy-specific behavior. |
| `internal/notification/source/ntfy/` | Spec 055 concrete ntfy source adapter — explicit config validation, stream/webhook transport, ntfy JSON parsing, `SourceEventEnvelope` mapping, topic health/lag/reconnect state, adapter-owned dead letters, replay through `SourceEventSink`, and boundary guards preventing output-channel coupling. |
| `internal/pipeline/` | Artifact processing pipeline — NATS subscribers for process/embed/rerank/digest/synthesis/domain-extract, result handlers, retry logic |
| `internal/scheduler/` | Cron-based task scheduler — digest generation (configurable cron), intelligence synthesis (2AM), momentum (hourly), resurfacing (8AM), knowledge lint (configurable), alert checks |
| `internal/stringutil/` | String utility functions — UTF-8 safe truncation, control character sanitization, text normalization |
| `internal/telegram/` | Telegram bot — message handling (URLs, text, voice, forwards, media groups, conversations), 9 commands (/find, /concept, /person, /lint, /digest, /done, /status, /recent, /rate), annotation via reply, disambiguation flow, recipe commands (serving scaler, cook mode with session store), expense interactions (receipt confirmation, query, correction, suggestions), meal plan commands (create, assign, query, cook-from-plan) |
| `internal/mealplan/` | Meal planning calendar — plan store, service (lifecycle, overlap, copy), shopping list bridge (reuses RecipeAggregator + ScaleIngredients), CalDAV calendar sync bridge |
| `internal/recipe/` | Shared recipe types, serving scaler, kitchen fraction formatter, quantity parsing (extracted from list aggregator for reuse by scaler and cook mode) |
| `internal/topics/` | Topic extraction and management — topic CRUD, promotion/archival lifecycle, hot topic detection |
| `internal/web/` | HTMX web UI — search, artifact detail, digest, topics, settings, status, knowledge dashboard (concepts, entities, lint), embedded HTML templates |
| `internal/recommendation/` | Spec 039 recommendation runtime — reactive engine (`reactive/`), watch evaluator (`watch/`), provider registry (`provider/`), candidate ranking (`rank/`), quality dedupe (`quality/`), policy enforcement (`policy/`), trip dossier composer, persistence store (`store/`) including the `AssertRedactSafe` log/trace redaction guard and `GetWatchAuditCounts` audit-table accessor used by the per-watch operator visibility view |
| `internal/agent/` | Spec 037 agent bridge — wires the prompt-contract loader, intent router, and tool executor into a single `Bridge` that API, Telegram, scheduler, and pipeline surfaces call via a uniform `Invoke` / `KnownIntents` contract |
| `internal/assistant/` | Spec 061 assistant capability layer — facade audit boundary (every turn writes one `assistant_turn` audit record), borderline-intent handling, proposal-payload persistence, and confirm-pending lifecycle |
| `internal/backup/` | Spec 048 backup & restore — pure retention policy (7 daily + 4 weekly), on-disk status contract written by `scripts/commands/backup.sh`, and metrics watcher that exposes the most recent backup outcome for spec 049 alerting |
| `internal/deploy/` | Build-Once Deploy-Many deployment contracts — static-file tests for `.github/workflows/build.yml` (bundle-hash contract, vuln gate, bundle-secret contract) and the deploy-target compose contract enforcing the tailnet-edge bind invariants |
| `internal/manifest/` | Spec 076 scenario-manifest schema and loader — typed inherited-scenario manifest (own SCN-076-Fxx entries + `inheritsFrom` references to specs 064/065/066/073/074/075) consumed by traceability tooling |
| `internal/whatsapp/` | Spec 072 WhatsApp Business Cloud API transport — `assistant_adapter/` implements the canonical `TransportAdapter` contract: webhook ingress, Meta signature verification, generic transport-identity lookup, inbound payload → `AssistantMessage` translation, and outbound text/list/buttons rendering |
| `internal/scopesdriftguard/` | Test-only contract guard — ratchets the count of broken `path` references in `specs/*/scopes.md` against the filesystem so new traceability drift fails CI. Contains only `scopes_drift_guard_test.go` (no runtime code). |
| `internal/docfreshness/` | Test-only contract guard (spec 032 / BUG-003) — asserts `docs/Development.md` documents every `internal/` Go package, every `internal/db/migrations/*.sql`, and every `config/prompt_contracts/*.yaml`, so documentation-inventory drift fails the Go unit suite and CI. Contains only `doc_freshness_test.go` (no runtime code). |
| `internal/retrieval/` | Spec 095 retrieval-strategy routing + freshness-aware retrieval — `routing/` holds the pure `RetrievalStrategyRouter` (selects whole_document / structured_aggregate / vague_recall from the already-computed `CompiledIntent` + the per-artifact-type `RetrievalContract` registry, with a traced `StrategySelection`), the strategy overlays under `routing/strategies/*`, and the architecture tests proving the single-store invariant (`TestNoParallelStore`); `evergreen/` holds the ingestion-front-door evergreen-vs-ephemeral signal (scenario-driven judgment + deterministic `TierSignals` fallback) and the synthesis/digest pool-eligibility predicate. All strategies are read paths over the SINGLE existing pgvector + knowledge-graph + structured store via injected interfaces (Principle 5 — no parallel index). |

## Database Migrations

All migrations live in `internal/db/migrations/` and run automatically on startup via `internal/db/migrate.go` with advisory locking.

Migrations 002–017 were consolidated into `001_initial_schema.sql` during a schema squash. The consolidated migration creates all tables in a single file (31 tables, 86 DDL statements).

| Migration | File | Purpose |
|-----------|------|---------|
| 001 | `001_initial_schema.sql` | Consolidated schema (original migrations 001–017): `artifacts`, `people`, `topics`, `edges`, `sync_state`, `action_items`, `digests`, `synthesis_insights`, `alerts`, `meeting_briefs`, `weekly_synthesis`, `trips`, `trails`, `privacy_consent`, `keep_exports`, `ocr_cache`, `oauth_tokens`, `location_clusters`, `subscriptions`, `learning_progress`, `quick_references`, `search_log`, `guests`, `properties`, `knowledge_concepts`, `knowledge_entities`, `knowledge_lint_reports`, `annotations`, `telegram_message_artifacts`, `lists`, `list_items`. Extensions: `vector`, `pg_trgm`. Includes all indexes, unique constraints, materialized views, and column additions from the original 17 migrations |
| 018 | `018_meal_plans.sql` | Meal planning (spec 036): `meal_plans` + `meal_plan_slots` tables with date range, lifecycle status, slot constraints |
| 019 | `019_expense_tracking.sql` | Expense tracking (spec 034): `vendor_aliases`, `expense_suggestions`, `expense_suggestion_suppressions` tables, GIN index on artifacts expense metadata |
| 020 | `020_agent_traces.sql` | Agent trace persistence for prompt/tool execution audit |
| 021 | `021_drive_schema.sql` | Cloud-drive connection, scan, retrieval, and policy schema (spec 038) |
| 022 | `022_recommendations.sql` | Recommendation requests, preferences, provider payloads, suppressions, delivery attempts, and audit records (spec 039) |
| 023 | `023_drive_connection_expires_at.sql` | Adds `expires_at` to `drive_connections` for token lifecycle tracking |
| 024 | `024_drive_scan_monitor_read_models.sql` | Drive scan/monitor read-model tables for cursor durability |
| 025 | `025_photo_libraries.sql` | Cloud photo libraries base schema (spec 040): providers, connections, scans |
| 026 | `026_photo_scope2_progress.sql` | Photo scope 2 — scan progress and resumable cursors |
| 027 | `027_recommendation_watch_runtime.sql` | Recommendation watch runtime tables (due-watch scheduling, audit) |
| 028 | `028_drive_save_back.sql` | Drive Save Service tables: confirmations, save attempts, rules |
| 029 | `029_photo_scope3_lifecycle_dedupe_removal.sql` | Photo lifecycle, duplicate analysis, and removal-action records |
| 030 | `030_drive_confirmations_and_share_changes.sql` | Drive low-confidence confirmations and share-change tracking |
| 031 | `031_photo_scope4_capture_routing_sensitivity.sql` | Photo capture routing and sensitivity policy fields |
| 032 | `032_photo_reveal_tokens_secret_hash_and_toctou.sql` | Photo reveal-token secret hashing and TOCTOU-safe redemption |
| 033 | `033_auth_per_user_bearer.sql` | Spec 044 per-user PASETO v4.public bearer auth, sessions, and revocation cache |
| 034 | `034_qf_decisions_capability.sql` | QF companion (spec 041) capability registration and bridge state |
| 035 | `035_qf_personal_evidence_exports.sql` | QF `PersonalEvidenceBundle` export records and dispatch state |
| 036 | `036_notification_intelligence.sql` | Spec 054 notification intelligence schema: `notification_source_instances`, `notification_source_health_events`, `notification_raw_events`, `normalized_notifications`, `notification_classifications`, `notification_incidents`, `notification_incident_events`, `notification_processing_decisions`, `notification_suppressions`, and `notification_delivery_attempts` |
| 037 | `037_qf_personal_context_consent_tokens.sql` | Spec 041 QF personal-context consent tokens and audit columns |
| 038 | `038_notification_ntfy_source_adapter.sql` | Spec 055 ntfy source adapter schema: relaxes source-instance secret references only when `auth_mode=none` is explicit, then adds `notification_ntfy_subscription_states`, `notification_ntfy_dead_letters`, and `notification_ntfy_replay_attempts` |
| 039 | `039_hospitality_counter_applications.sql` | Hospitality counter-applications schema for STR ops |
| 040 | `040_raw_ingest_dedup.sql` | Raw-ingest dedupe table to guard duplicate capture across sources |
| 041 | `041_assistant_conversations.sql` | Assistant (spec 061) conversation persistence schema |
| 042 | `042_assistant_proposal_payload.sql` | Assistant proposal-payload JSONB column for audit replay |
| 043 | `043_assistant_confirm_pending.sql` | Assistant confirm-pending lifecycle table |
| 044 | `044_web_user_credentials.sql` | Web user-credential rows for per-user auth surface |
| 045 | `045_open_knowledge.sql` | Open-knowledge scenario tables and references |
| 046 | `046_legacy_retirement.sql` | Retirement of legacy artifact columns and unused indexes |
| 047 | `047_assistant_intent_traces.sql` | Assistant intent-trace persistence for capability audit |
| 048 | `048_legacy_retirement_residual.sql` | Residual cleanup after migration 046 |
| 050 | `050_assistant_transport_identities.sql` | Generic transport-identity table for assistant transports (WhatsApp, Telegram, etc.) |
| 051 | `051_artifact_capture_policy.sql` | Artifact-capture policy fields for capture-as-fallback flow |
| 052 | `052_capture_as_fallback_pending_clarify.sql` | Pending-clarify state for capture-as-fallback assistant turns |
| 053 | `053_assistant_tool_traces.sql` | Assistant tool-call trace persistence |
| 054 | `054_assistant_tool_traces_call_outcome.sql` | Adds `call_outcome` column to tool-trace rows |
| 055 | `055_annotation_actor_and_version.sql` | Annotation actor and version columns for audit and conflict resolution |
| 056 | `056_twitter_oauth_pkce.sql` | Twitter/X user-context OAuth 2.0 PKCE storage (BUG-056-002): `twitter_oauth_states` (PKCE `code_verifier` binding, 15-min TTL, delete-on-consume) and `twitter_oauth_tokens` (AES-256-GCM-encrypted access/refresh, composite PK) |
| 057 | `057_card_rewards.sql` | Spec 083 Card Rewards Companion schema — 10 tables (`card_catalog`, `card_runs`, `user_cards`, `card_offers`, `card_selections`, `signup_bonuses`, `rotating_category_observations`, `rotating_categories`, `category_aliases`, `card_recommendations`) with CHECK/FK/UNIQUE constraints and indexes; `CREATE … IF NOT EXISTS` self-idempotent |
| 058 | `058_web_registration_invites.sql` | Spec 093 admin-generated single-use registration invites — `web_registration_invites` table (`token_hash` lowercase-hex SHA-256, `label`, `created_by`, `created_at`, `expires_at`, `used_at`, `used_by`, `revoked_at`); hashed-at-rest with no plaintext column, `UNIQUE(token_hash)` lookup, atomically marked used on a successful `/register` (single-use, TOCTOU-guarded UPDATE), augmenting spec 091's static `WEB_REGISTRATION_INVITE_TOKEN` bootstrap gate; `CREATE TABLE IF NOT EXISTS` self-idempotent |
| 059 | `059_user_model_preferences.sql` | Spec 089 per-user sticky open-knowledge `/ask` synthesis-model preference — `user_model_preferences` table (`actor_user_id TEXT PRIMARY KEY` claim-bound principal, `synthesis_model TEXT NOT NULL`, `gather_model TEXT` reserved/nullable for F-STICKY-GATHER, `updated_at TIMESTAMPTZ NOT NULL` app-written, no DB-side default); one row per user (cheap PK read on the `/ask` hot path), upsert on set (`ON CONFLICT (actor_user_id) DO UPDATE`), reset == `DELETE`; mirrors the actor-keyed pattern of `022_recommendations.sql` / `055_*`; NEVER settable by a request-body user id (OWASP A01); `CREATE TABLE IF NOT EXISTS` self-idempotent |
| 060 | `060_artifact_evergreen_signal.sql` | Spec 095 SCOPE-07 / PKT-095-B evergreen-signal persistence at the ingestion front door — ADDITIVE nullable `artifacts.evergreen_score REAL` (signed: `>= 0` evergreen, `< 0` ephemeral, magnitude = calibrated confidence, NULL = not yet scored ⇒ treated as evergreen/not-excluded downstream per Principle 9) + `artifacts.evergreen_source TEXT` (judgment provenance `scenario`/`tier_signals_fallback`/`tier_signals` for Principle 8); written by the app-side `evergreen.Scorer` (no DB-side default — NO-DEFAULTS / G028), lives on the EXISTING artifacts table (Principle 5 — never a parallel store); `ADD COLUMN IF NOT EXISTS` self-idempotent |

Migration `036_notification_intelligence.sql` is additive and follows the existing application-written ID/timestamp pattern: it uses `CHECK` constraints for enum-like values, JSONB for redaction/rationale envelopes, and no database-side runtime fallback values. Raw source input is stored in `notification_raw_events` before a normalized notification can be processed downstream.

Migration `038_notification_ntfy_source_adapter.sql` is adapter-owned. It stores one subscription-state row per `(source_instance_id, topic)`, redacted dead-letter records for failed ntfy intake, and replay attempts keyed by idempotency hash. The replay table records whether the source sink accepted, rejected, or failed a replay; it does not represent output delivery. Runtime replay locks the dead-letter row and short-circuits already-replayed records before another `SourceEventSink` call, so the idempotency contract bounds both audit rows and source-sink side effects.

## Prompt Contracts

Prompt contracts live in `config/prompt_contracts/` and are mounted into the ML sidecar container at `/app/prompt_contracts`. Each contract defines the system prompt, extraction schema, and validation rules for a specific LLM task.

| Contract | File | Type | Purpose |
|----------|------|------|---------|
| Ingest Synthesis | `ingest-synthesis-v1.yaml` | `ingest-synthesis` | Extract concepts, entities, claims, and relationships from an artifact for the knowledge layer |
| Cross-Source Connection | `cross-source-connection-v1.yaml` | `cross-source-connection` | Assess whether artifacts from different sources sharing a concept have a genuine cross-domain connection |
| Lint Audit | `lint-audit-v1.yaml` | `lint-audit` | Audit knowledge quality — detect contradictions, stale concepts, orphan entities |
| Query Augment | `query-augment-v1.yaml` | `query-augment` | Augment a search query with context from concept pages and entity profiles |
| Digest Assembly | `digest-assembly-v1.yaml` | `digest-assembly` | Assemble daily digest section from pre-synthesized knowledge layer content |
| Recipe Extraction | `recipe-extraction-v1.yaml` | `domain-extraction` | Extract structured recipe data (ingredients, steps, nutrition) from recipe content |
| Product Extraction | `product-extraction-v1.yaml` | `domain-extraction` | Extract structured product data (price, specs, ratings) from product pages and reviews |
| Receipt Extraction | `receipt-extraction-v1.yaml` | `domain-extraction` | Extract structured receipt/invoice data (vendor, date, amount, currency, tax, line items, payment method) |
| Annotation Classify | `annotation-classify-v1.yaml` | `scenario` | Classify freeform user annotations (ratings, tags, interactions, notes) into the canonical annotation schema |
| Drive Classification | `drive-classification-v1.yaml` | `drive-classification` | Classify extracted drive files into provider-neutral Smackerel domains (spec 038) |
| Drive Folder Context | `drive-folder-context-v1.yaml` | `drive-folder-context` | Summarize drive folder context for classification refreshes |
| E2E Ollama Smoke | `e2e-ollama-smoke-v1.yaml` | `scenario` | Minimal smoke contract used by the E2E live-stack Ollama health check |
| Notification Schedule | `notification-schedule-v1.yaml` | `scenario` | Propose a scheduled reminder; on user confirm, register it with the spec 054 scheduler |
| Open Knowledge | `open_knowledge.yaml` | `scenario` | Open-knowledge scenario for general retrieval over the user's knowledge graph |
| Recipe Search | `recipe-search-v1.yaml` | `scenario` | Find recipes in the user's owned knowledge graph and return them with artifact-ID citations |
| Recommendation Feedback | `recommendation-feedback-v1.yaml` | `scenario` | Record recommendation feedback, suppression state, and preference corrections (spec 039) |
| Recommendation Reactive | `recommendation-reactive-v1.yaml` | `scenario` | Reactive place recommendation with provider facts, graph signals, policy, quality, and persistence |
| Recommendation Watch Evaluate | `recommendation-watch-evaluate-v1.yaml` | `scenario` | Evaluate a due recommendation watch and persist a delivery, queue, summarize, or drop decision |
| Recommendation Why | `recommendation-why-v1.yaml` | `scenario` | Explain an existing recommendation from persisted trace and refs only |
| Retrieval QA | `retrieval-qa-v1.yaml` | `scenario` | Answer a user question over the user's knowledge graph with artifact-ID citations |
| Weather Query | `weather-query-v1.yaml` | `scenario` | Answer a current/forecast weather question from an external provider with provider+timestamp attribution |
| Relationship Cooling Evaluate | `relationship-cooling-evaluate-v1.yaml` | `scenario` | LLM-driven relationship-cooling judgment (spec 021 R-021-005 / BUG-021-005) — replaces the hardcoded SQL interaction/silence heuristic with a per-situation decision |
| Alert Timing Evaluate | `alert-timing-evaluate-v1.yaml` | `scenario` | LLM-driven alert-timing judgment for bill/trip-prep/return-window alerts (spec 021 R-021-002/003/004 / BUG-021-006) — replaces hardcoded timing windows |
| Resurface Evaluate | `resurface-evaluate-v1.yaml` | `scenario` | LLM-driven resurfacing-worthiness judgment (spec 021 R-505 / BUG-021-007) — replaces the hardcoded dormancy/relevance window in `internal/intelligence/resurface.go` |
| Expertise Classify | `expertise-classify-v1.yaml` | `scenario` | LLM-driven expertise-tier and growth-trajectory classification (spec 021 R-501 / BUG-021-008) — replaces the hardcoded heuristics in `internal/intelligence/expertise.go` |
| Hospitality Concern Evaluate | `hospitality-concern-evaluate-v1.yaml` | `scenario` | LLM-driven guest/property hospitality concern judgment (spec 013 / 021 BUG-021-010) — replaces hardcoded sentiment/rating/issue thresholds in `internal/digest/hospitality.go` |
| Retrieval Evergreen | `retrieval-evergreen-v1.yaml` | `scenario` | LLM-driven evergreen-vs-ephemeral judgment at the ingestion front door (spec 095 SCOPE-07 / Idea 2) — decides whether an arriving artifact is durable reference knowledge or transient noise; Go holds only operational bounds (confidence floor, per-tick budget, dedup window) as SST, never a cutoff (docs §3.6); deterministic TierSignals fallback when the scenario is unavailable (NFR-2, Principle 9) |

### Adding a New Prompt Contract

To add a new domain extraction contract:

1. Create `config/prompt_contracts/<domain>-extraction-v1.yaml`
2. Required fields:
   - `version`: Contract version identifier (e.g., `"recipe-extraction-v1"`)
   - `type`: Must be `"domain-extraction"` for extraction contracts
   - `description`: One-line description
   - `content_types`: List of content types this contract handles
   - `url_qualifiers`: URL patterns that trigger this contract
   - `min_content_length`: Minimum content length to attempt extraction
   - `system_prompt`: The LLM system prompt
   - `extraction_schema`: JSON schema for the expected output
3. Register the contract in `internal/domain/` schema registry
4. The ML sidecar auto-discovers contracts from the mounted directory

### Agent + Tool Development Discipline

Domain reasoning in Smackerel follows the LLM agent + tools pattern described in
`docs/smackerel.md` §3.6. When extending the system, choose the *lowest-power*
mechanism that does the job, in this order:

1. **New or revised prompt contract** that composes existing tools — default.
2. **New deterministic tool** in the Go tool registry — only when the agent
   needs a capability the current registry cannot express.
3. **New hardcoded Go logic** — only for non-reasoning concerns: math, format
   helpers, schema-bound CRUD, transport, authn/authz, scheduling, validation.

#### When To Add A Tool vs. A Prompt

| Symptom | Correct response |
|---------|------------------|
| New scenario, same data and operations | New prompt contract |
| New user intent on an existing surface (Telegram, API, digest) | New prompt contract; do **not** add another regex/intent branch |
| New domain entity needs structured extraction | New `domain-extraction` prompt contract + schema entry in `internal/domain/` |
| New deterministic transform on existing data (e.g., scale, format, aggregate) | New tool in the Go registry, exposed to relevant prompts |
| New data source / external call the agent must reach | New tool wrapping the call, with typed args and a JSON schema |
| New classification, scoring, or routing decision | Prompt contract that calls existing lookup tools — **not** a new rule chain |

#### Tool Conventions

- Tools live in the Go core, in the package that owns the data they touch
  (e.g., recipe operations in `internal/recipe/`, expense operations in
  `internal/intelligence/`, knowledge operations in `internal/knowledge/`),
  and are registered with the agent runtime through a single registry surface.
- Each tool MUST declare:
  - a stable, snake_case name (e.g., `parse_receipt`, `scale_recipe`,
    `search_artifacts`);
  - a JSON schema for its arguments and for its return value;
  - a one-line human description used by the LLM for tool selection;
  - a deterministic Go implementation with unit tests.
- Tools MUST NOT embed business policy that should live in a prompt
  (e.g., "if vendor looks like a grocery store and amount > X, mark as
  household"). Such policy belongs in the scenario prompt; the tool only
  exposes the lookup or transform the prompt needs.
- Tool side effects (writes, external calls) MUST be explicit in the tool name
  and signature; read-only and write tools are not interchangeable.

#### Prompt Contract Conventions

- All scenario and extraction prompts live in `config/prompt_contracts/` and
  follow the existing YAML shape (`version`, `type`, `description`,
  `system_prompt`, plus type-specific fields such as `extraction_schema` or the
  set of allowed tool names).
- A prompt contract MUST declare the subset of tools the agent is permitted to
  call; tool access is not implicit.
- A prompt contract MUST declare the expected output schema; the ML sidecar
  validates against it before returning to Go.
- Versioning: bump the `version` suffix (`-v2`, `-v3`) when behavior changes in
  ways that downstream consumers can observe; do not silently mutate `-v1`.

#### Agent Runtime Configuration

The agent runtime is configured under the top-level `agent:` block in
`config/smackerel.yaml`. All values are SST zero-defaults — every key is
required, every value flows through `./smackerel.sh config generate`, and
both the Go core (`internal/agent/Config.LoadConfig`) and the Python ML
sidecar (`ml/app/agent_config.load_agent_config`) refuse to start when any
required `AGENT_*` environment variable is missing or malformed. Empty-string
values are accepted only for `agent.routing.fallback_scenario_id` and
`agent.routing.embedding_model`, both documented as opt-outs in spec
`specs/037-llm-agent-tools/design.md` §11. The NATS contract for the agent
loop lives in `config/nats_contract.json` under the `AGENT` stream
(`agent.invoke.request`, `agent.invoke.response`, `agent.tool_call.executed`,
`agent.complete`); the Go and Python contract tests assert the constants on
both sides match the contract file.

#### Scenario Loader & Linter

Scenario YAML files live under `config/prompt_contracts/` (the loader scans
that directory and any other `agent.scenario_dir` configured) and are loaded
by `internal/agent/loader.go`. The loader applies every load-time validation
rule from `specs/037-llm-agent-tools/design.md` §2.2:

- All required top-level fields present (`id`, `version`, `type`,
  `system_prompt`, `allowed_tools`, `input_schema`, `output_schema`,
  `limits`, `side_effect_class`).
- `id` matches `^[a-z][a-z0-9_]*$`; `version` ends in `-v<N>` and its slug
  (with dashes mapped to underscores) equals `id`.
- Every `allowed_tools[].name` is in the registry, and every declared
  `side_effect_class` matches the registered tool's class.
- Scenario-level `side_effect_class` ≥ max of allowed-tool classes
  (`read` < `write` < `external`).
- `input_schema` and `output_schema` compile as JSON Schema Draft 2020-12;
  no required field in `output_schema` may carry `x-redact: true`.
- `limits.max_loop_iterations` ∈ `[1, 32]`, `timeout_ms` ∈ `[1000, 120000]`,
  `schema_retry_budget` ∈ `[0, 5]`, `per_tool_timeout_ms` ∈ `[1, timeout_ms]`.
- Two scenarios sharing an `id` is fatal — the process refuses to start
  (`BS-011`).

Each loaded scenario carries a `content_hash` (sha256 over a canonical JSON
projection of the YAML) that is recorded on every trace so replay can
detect post-hoc edits to the source file.

CI must run the scenario linter binary (`cmd/scenario-lint`) against the
configured scenario directory:

```bash
go run ./cmd/scenario-lint config/prompt_contracts
```

Exit codes: `0` clean, `1` rejection or duplicate id, `2` usage error.
The linter reports each rejected file (`REJECT <path>: <message>`) and any
fatal duplicate-id condition (`FATAL <message>`) on stderr, then prints a
one-line registered/rejected summary on stdout.

#### Forbidden Patterns

The following are explicitly out of scope for new code and are targets for
removal from existing code (see
[spec 066 — Legacy Keyword Surface Retirement](../specs/066-legacy-keyword-surface-retirement/)
for the active retirement plan, and
[spec 067 — Intent-Driven Policy Enforcement](../specs/067-intent-driven-policy-enforcement/)
for the CI guards that mechanically enforce these rules — scenario-prompt
length cap, mandatory `principleAlignment` block per scenario YAML,
broadened NO-DEFAULTS check, forbidden-keyword guard, and compiler-bypass
detection):

- Regex-based intent routers (e.g., long `switch`/`if` chains over message
  text in the Telegram bot or any other channel).
- Multi-level hardcoded classification chains in Go (e.g., the current 7-level
  expense classifier) for decisions that involve language understanding,
  vendor judgement, or fuzzy matching.
- Keyword-map categorization (e.g., "if ingredient name contains
  'milk'/'cheese' → dairy") for decisions an LLM with the right tool can make.
  Cross-scenario primitives such as `location_normalize`, `unit_convert`,
  `entity_resolve`, and `calculator` are provided by
  [spec 065 — Generic Micro-Tools](../specs/065-generic-micro-tools/) and
  MUST be used instead of forking per-scenario normalization into prompts
  or scenario-local Go.
- Hardcoded vendor / alias / synonym seed lists in Go source — such data
  belongs behind a tool that consults the database (or asks the LLM with the
  database as context), not as a literal in code. The
  `internal/api/domain_intent.go` regex parser and the annotation keyword
  map are explicitly slated for retirement under spec 066; new code MUST
  NOT reintroduce equivalents.
- Adding a new Go branch to extend a scenario when the same outcome is
  achievable by editing a prompt contract.
- Calling `Router.Route` (or any scenario routing entrypoint) from a
  user-facing NL path without a validated `CompiledIntent` trace record
  produced by the spec 068 structured intent compiler. Compiler-bypass is
  caught by the spec 067 CI guard.
- Branching on `AssistantMessage.Transport` outside transport-adapter or
  audit code. Facade, scenarios, executor, and tool code MUST treat every
  transport identically (Telegram, HTTP per
  [spec 069](../specs/069-assistant-http-transport/), and any future
  adapter). This is enforced by a spec 067 CI guard.

#### Adding A New Scenario (BS-001 — zero Go changes)

Spec 037 Scope 10 wires the agent runtime so adding a new scenario is a
**configuration change, not a code change**. The end-to-end procedure:

1. Drop a new YAML file under `config/prompt_contracts/` (or any directory
   pointed at by `agent.scenario_dir`) following the schema in
   `specs/037-llm-agent-tools/design.md` §2.2.
2. Make sure every name in `allowed_tools` is already registered by some
   Go package's `init()` (see "Adding A New Tool" below). If a tool you
   need does not exist, that is the rare case where Go *is* required —
   add the tool first, then the scenario.
3. Run `./smackerel.sh check` — `cmd/scenario-lint` is wired into this
   command (Scope 10) and rejects any scenario that fails the load-time
   rules (BS-009 / BS-010 / BS-011).
4. Send the running `smackerel-core` process a `SIGHUP`. The agent
   bridge atomically reloads the scenario directory and rebuilds the
   router. In-flight invocations pin the version of the scenario they
   started with (BS-019).
5. Invoke the new scenario by id via any surface (`POST /v1/agent/invoke`,
   `POST /api/assistant/turn` (the spec 069 HTTP transport — the canonical
   programmatic entrypoint for end-to-end tests of any assistant journey),
   the Telegram bridge, `scheduler.FireScenario`, or
   `pipeline.FireScenario`). No restart, no rebuild, no Go diff.

The end-user-observable contract is exercised by
`tests/e2e/agent/bs001_zero_go_change_test.go::TestBS001_DropYAMLAndReload_NewScenarioInvokable`,
which writes a fresh YAML, calls `Bridge.Reload`, asserts the new id
appears in `KnownIntents`, invokes it, and proves the pre-existing
scenario still works.

#### Adding A New Tool

Tools always require a Go change — but the change is bounded to one
package and one `init()` registration. The procedure:

1. Decide which package owns the data the tool touches (e.g., recipe
   reads/writes go in `internal/recipe/`, expense reads/writes in
   `internal/intelligence/`). Do **not** create a new package just for
   the tool.
2. Implement the `agent.ToolHandler` (deterministic Go function with
   typed JSON Schema args/return).
3. Call `agent.RegisterTool(...)` from a package-level `init()` with
   the tool name, description, input/output schema, side-effect class
   (`read` / `write` / `external`), and owning-package label.
4. Add a unit test in the owning package that exercises the handler
   directly.
5. Add or extend a scenario YAML that lists the new tool in
   `allowed_tools`. The Go core enforces the allowlist independently
   of the LLM (BS-003); the ML sidecar only renders the tool list.
6. Run `./smackerel.sh check lint test unit` to confirm the registry
   is healthy and the scenario lints clean. The forbidden-pattern
   guard at `tests/integration/agent/forbidden_pattern_test.go`
   continues to enforce that no regex/switch routers slip into
   `internal/agent/`, `internal/telegram/`, `internal/api/`, or
   `internal/scheduler/`.

Tool registration is decentralized (each package's `init()` registers
its own tools); there is no central registration table to update. This
is the design choice that makes "add a tool" a one-package change.

### Spec 064 Open-Knowledge Agent

The open-knowledge agent ([`specs/064-open-ended-knowledge-agent/design.md`](../specs/064-open-ended-knowledge-agent/design.md))
is a bounded planner ↔ tool ↔ observation loop that lives under
[`internal/assistant/openknowledge/`](../internal/assistant/openknowledge).
It registers itself as the second-to-last entry in the spec 048
scenario manifest (immediately before `capture_as_fallback`) and is
gated by `assistant.open_knowledge.enabled` (see
[`docs/Operations.md`](Operations.md#open-knowledge-assistant-agent-spec-064)
for the operator surface).

**Local dev config.** The committed `config/smackerel.yaml` block
ships disabled. To exercise the subsystem locally:

1. Set `assistant.open_knowledge.enabled: true` in
   `config/smackerel.yaml`.
2. Pick a provider. For local-first dev, leave `provider: "searxng"`
   and flip `environments.dev.searxng_enabled: true` to bring the
   self-hosted SearxNG container up under the `searxng` Compose
   profile.
3. Populate `assistant.open_knowledge.llm_model_id` with a model
   served by the ML sidecar (`/llm/chat` route), and confirm the
   budget keys are non-zero per spec.
4. `./smackerel.sh config generate` to re-emit env files.
5. `./smackerel.sh up` then `docker compose --profile searxng up -d searxng`
   if the profile is not already active.

**Integration tests that need SearxNG.** The integration leg
auto-enables the profile via `environments.test.searxng_enabled: true`
and runs `TestSearxNGIntegration_Smoke` against the live container:

```bash
ENABLE_SEARXNG=true ./smackerel.sh test integration
```

**E2E tests.** Spec 064 SCOPE-17 staged seven e2e tests under
`tests/e2e/agent/openknowledge_e2e_test.go` (`//go:build e2e`). They
currently `t.Skip(...)` with explicit messages naming the routed
infrastructure findings under PKT-WORKFLOW-A (no `AGENT_INVOKE_URL`
in the e2e runner, no real `/llm/chat` dispatch path, no
`fixture-fabricated-cite` test mode, no per-test budget /
allowlist override knobs). When those findings land, the tests
activate without modification. Until then,
`go test -tags e2e ./tests/e2e/agent/...` is expected to PASS with
seven SKIP entries.

## NATS JetStream Streams

All NATS subjects and streams are defined in `config/nats_contract.json`. Both Go core and Python ML sidecar have tests that verify their local constants match this contract. The runtime currently provisions 15 streams.

| Stream | Subjects Pattern | Purpose |
|--------|-----------------|---------|
| `ARTIFACTS` | `artifacts.>` | Artifact processing pipeline (core → ML → core) |
| `SEARCH` | `search.>` | Embedding and re-ranking (core → ML → core) |
| `DIGEST` | `digest.>` | Daily digest generation (core → ML → core) |
| `KEEP` | `keep.>` | Google Keep sync and OCR (core → ML → core) |
| `INTELLIGENCE` | `learning.>`, `content.>`, `monthly.>`, `quickref.>`, `seasonal.>` | Phase 5 intelligence features (learning classification, content analysis, monthly reports, quick references, seasonal patterns) |
| `ALERTS` | `alerts.>` | Contextual alert notifications (core → external) |
| `SYNTHESIS` | `synthesis.>` | Knowledge synthesis and cross-source connection assessment (core → ML → core) |
| `DOMAIN` | `domain.>` | Domain-aware structured extraction (core → ML → core) |
| `DRIVE` | `drive.>` | Spec 038 cloud-drives surface — `drive.scan.request`, `drive.scan.result`, `drive.change.notify`, `drive.health.report`, `drive.extract.request`/`.result`, `drive.classify.request`/`.result` (core → ML → core) |
| `PHOTOS` | `photos.>` | Spec 040 cloud-photos surface — `photos.classify`/`.classified`, `photos.ocr`/`.ocred`, `photos.embed`/`.embedded`, `photos.lifecycle`/`.result`, `photos.dedupe`/`.result` (core → ML → core) |
| `ANNOTATIONS` | `annotations.>` | Annotation event notifications (core internal) |
| `LISTS` | `lists.>` | List lifecycle events (core internal) |
| `AGENT` | `agent.>` | Spec 037 agent loop — `agent.invoke.request`, `agent.invoke.response`, `agent.tool_call.executed`, `agent.complete` |
| `WEATHER` | `weather.>` | Weather connector reactive subjects |
| `DEADLETTER` | `deadletter.>` | Dead-letter queue for messages that exhaust retry budgets |

## QF Companion Connector Internals (Spec 041 Scope 5)

The QF companion connector lives at `internal/connector/qfdecisions/` and is
the only Smackerel package that talks to the QF (quantitativeFinance)
companion surface. Scope 5 adds four hardening modules on top of the Scope
1-4 sync / render / export base.

### Module Map

| File | Responsibility |
|------|----------------|
| `connector.go`, `render.go`, `evidence_bundle.go` | Scopes 1-4 sync / render / evidence export call sites that invoke the Scope 5 hardening helpers below |
| `credentials.go` | Pure planner `PlanCredentialRotation(creds, state, now)` and the connector-level entry point `(*Connector).RotateCredentials` that wires it into capability re-read + atomic state swap |
| `boundary.go` | Pure helper `EnforceQFActionBoundary(attempt ActionBoundaryAttempt) (ActionBoundaryDiagnostic, bool, error)` — returns `(diagnostic, fired, err)`; callers short-circuit on `fired == true` |
| `audit.go` | `BuildCrossProductAuditEnvelopeV1(input AuditEnvelopeInput) EvidenceAuditEnvelope` plus the slog sink `EmitConnectorAuditEnvelope` that writes the canonical `qf-decisions: cross_product_audit` JSON record |
| `metrics.go` | Lightweight metric-emission helpers that wrap the 12 `smackerel_qf_*` Prometheus collectors declared in `internal/metrics/metrics.go` |
| `types.go` | Action-constant and forbidden-action-type tables consumed by every helper above |

### Calling Pattern: Action Boundary

Every sync, render, and evidence-export code path that could ever attempt a
QF-side mutating action MUST funnel the attempt through
`EnforceQFActionBoundary` before any other work. The contract is:

```go
diag, fired, err := qfdecisions.EnforceQFActionBoundary(qfdecisions.ActionBoundaryAttempt{
    AttemptedActionType: attemptedAction, // typed enum, never free-form string
    TraceID:             traceID,
    PacketID:            packetID,
    // ... per-call envelope context ...
})
if err != nil {
    return err
}
if fired {
    // Boundary rejected the attempt. The helper has already emitted
    // smackerel_qf_action_boundary_attempts_total{attempted_action_type=...}
    // and an audit envelope with action=action_boundary_kick, outcome=rejected.
    // Callers MUST short-circuit; no further QF-side work is permitted.
    return nil
}
// Boundary allowed the attempt (or it was a benign / empty attempt).
// Proceed with the read-only flow.
```

The helper itself never enables an action; the `fired == true` branch is the
only legal outcome for any of the 8 pre-MVP forbidden action types listed in
[`docs/Operations.md`](Operations.md#pre-mvp-safety-boundary).

### Calling Pattern: Audit Envelope

Every Smackerel-side QF emission point builds a Cross-Product Audit Envelope
v1 via the shared helper before emitting the metric counter:

```go
envelope := qfdecisions.BuildCrossProductAuditEnvelopeV1(qfdecisions.AuditEnvelopeInput{
    Action:            qfdecisions.AuditActionPacketIngest, // typed constant
    Outcome:           "ok",
    Reason:            "scope2_ingest_packet",
    TraceID:           traceID,
    PacketID:          packetID,
    AuditEnvelopeVersion: capability.AuditEnvelopeVersion, // sourced from persisted capability
    ObservedAt:        observedAt,
})
qfdecisions.EmitConnectorAuditEnvelope(ctx, envelope)
metrics.QFPacketIngestTotal.WithLabelValues(eventType, decisionType, approvalState, sourceSurface).Inc()
```

The builder fills the always-required field set (`actor_ref`,
`surface`, `recorded_at`) when the caller supplies empty values, normalizes
`ts` and `recorded_at` to RFC3339 UTC, and copies through the per-event
optional fields (`export_id`, `signal_id`, `bundle_id`,
`target_context_type`, `sensitivity_tier`) only when the caller sets them.

### Calling Pattern: Credential Rotation

`(*Connector).RotateCredentials(ctx, newCreds)` is the single supported
entry point for credential rotation. It:

1. Calls `PlanCredentialRotation(newCreds, currentState, now)` — pure, no I/O.
2. On `plan.SelectedCredentialRef != nil`, builds a new `Client` against the
   selected credential and calls `FetchCapability` + `CompatibilityCheck`
   BEFORE swapping any in-memory state. The capability re-read emits a
   `capability_handshake` envelope with the appropriate outcome.
3. On successful capability re-read, atomically swaps `c.client`,
   `c.cfg.CredentialRef`, `c.capability`, `c.capabilityStatus`,
   `c.capabilityFetchedAt`, and `c.health` to the new credential's values.
4. Emits a `credential_rotation` audit envelope with outcome `ok` /
   `rejected` / `error` and the plan diagnostics attached.

Failure at any step preserves the previous credential, cursor, and
evidence-export idempotency state verbatim.

| `DEADLETTER` | `deadletter.>` | Failed message storage for debugging |