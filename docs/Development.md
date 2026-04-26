# Smackerel Development Guide

Smackerel is Bubbles-bootstrapped and exposes a repo-standard runtime CLI and config pipeline. This guide documents the current command surface and the constraints the runtime must follow.

## Current Repo State

Committed:

- `README.md`
- `docs/smackerel.md`
- `specs/` (001-036, all with spec, design, scopes, reports)
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
- 15 passive connectors (IMAP email, CalDAV calendar, YouTube API, RSS/Atom, Bookmarks, Browser, Google Keep/Takeout, Google Maps, Hospitable STR, GuestHost STR, Discord, Twitter/X archive, Weather via Open-Meteo, Government Alerts via USGS, Financial Markets via Finnhub/CoinGecko)
- Intelligence engine (synthesis at 2AM, momentum hourly, resurfacing at 8AM, overdue alerts)
- Knowledge synthesis layer (concept pages, entity profiles, cross-source connections, lint auditing, prompt contract validation)
- Domain extraction pipeline (recipe and product schemas) with NATS-backed async processing and Prometheus metrics
- User annotations (freeform ratings, tags, notes, interactions) with Telegram reply-based annotation and materialized summaries
- Actionable lists (shopping, reading, product comparison) with domain-aware aggregation and completion tracking
- Observability (Prometheus metrics for ingestion, search, connector sync, domain extraction, NATS dead-letter; W3C trace propagation via NATS headers)
- PWA share target for mobile capture and browser extension (Chrome MV3 / Firefox) for desktop capture
- OAuth2 flow with CSRF protection, token storage, auto-refresh
- Data export endpoint with cursor pagination (JSONL streaming)
- Database migrations (3 SQL files — migrations 002–017 consolidated into 001)
- NATS JetStream with token authentication (11 streams: ARTIFACTS, SEARCH, DIGEST, KEEP, INTELLIGENCE, ALERTS, SYNTHESIS, DOMAIN, ANNOTATIONS, LISTS, DEADLETTER)
- Security: CSP, rate limiting, dedup unique index, config validation, body size limits
- CI/CD pipeline (GitHub Actions workflows, Docker image versioning, branch protection)

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
| Stress smoke | `./smackerel.sh test stress` | Run live-stack health burst validation |
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

## Go Packages (`internal/`)

| Package | Purpose |
|---------|---------|
| `internal/annotation/` | User annotation model, freeform parser (ratings, tags, interactions, notes), PostgreSQL store, materialized summary view |
| `internal/api/` | Chi router, REST API handlers (capture, search, digest, export, knowledge, annotations, lists, expense API (7 endpoints: query, export CSV, correction, classification, suggestions), meal plan API (12 endpoints), recipe domain scaling endpoint), Bearer auth, security headers, rate limiting |
| `internal/auth/` | OAuth2 provider abstraction, token exchange/refresh, Google OAuth scopes, token storage |
| `internal/config/` | SST-compliant configuration loader — reads all env vars, validates required fields, parses numeric config, cross-validates constraints |
| `internal/connector/` | Connector interface, registry, supervisor (5-min sync cycles), health status model. Sub-packages per connector: `alerts/`, `bookmarks/`, `browser/`, `caldav/`, `discord/`, `guesthost/`, `hospitable/`, `imap/`, `keep/`, `maps/`, `markets/`, `rss/`, `twitter/`, `weather/`, `youtube/` |
| `internal/db/` | PostgreSQL connection pool wrapper, migration runner (embed.FS), artifact CRUD, export with cursor pagination, guest/property repos |
| `internal/digest/` | Daily digest assembly (action items, overnight artifacts, hot topics, hospitality context, knowledge health, expense digest section (summary, needs-review, suggestions, missing receipts, word limit enforcement)), LLM generation via NATS, Telegram delivery with retry |
| `internal/domain/` | Domain extraction schema registry — maps artifact content types to prompt contracts for structured extraction (recipes, products), expense metadata types, vendor alias types |
| `internal/extract/` | Content extraction from URLs — HTML readability, YouTube transcript fetching, media type detection, SSRF-safe HTTP client |
| `internal/graph/` | Knowledge graph linker — 4 strategies (similarity, entity, topic, temporal), bidirectional edge creation, connection counting |
| `internal/intelligence/` | Intelligence engine — cross-domain synthesis, expertise mapping (R-501), learning paths (R-502), subscription detection (R-504), serendipity resurfacing (R-505), content fuel (R-506), quick references (R-507), monthly reports (R-508), seasonal patterns, momentum tracking, alerts, expense classification (7-level rule chain), vendor normalization (LRU cache + pre-seeded aliases), expense suggestion generation |
| `internal/knowledge/` | Knowledge synthesis layer — concept pages, entity profiles, lint reports, cross-source connection assessment, store with trigram search, prompt contract integration |
| `internal/list/` | Actionable list model — lists and list items, domain-aware aggregation from extracted data, completion tracking, PostgreSQL store |
| `internal/metrics/` | Prometheus metrics (ingestion, capture, search latency, domain extraction, connector sync, NATS dead-letter counters, DB connection gauge) and W3C traceparent propagation via NATS headers |
| `internal/nats/` | NATS JetStream client — stream/consumer creation, publish/subscribe helpers, subject constants matching `config/nats_contract.json` |
| `internal/pipeline/` | Artifact processing pipeline — NATS subscribers for process/embed/rerank/digest/synthesis/domain-extract, result handlers, retry logic |
| `internal/scheduler/` | Cron-based task scheduler — digest generation (configurable cron), intelligence synthesis (2AM), momentum (hourly), resurfacing (8AM), knowledge lint (configurable), alert checks |
| `internal/stringutil/` | String utility functions — UTF-8 safe truncation, control character sanitization, text normalization |
| `internal/telegram/` | Telegram bot — message handling (URLs, text, voice, forwards, media groups, conversations), 9 commands (/find, /concept, /person, /lint, /digest, /done, /status, /recent, /rate), annotation via reply, disambiguation flow, recipe commands (serving scaler, cook mode with session store), expense interactions (receipt confirmation, query, correction, suggestions), meal plan commands (create, assign, query, cook-from-plan) |
| `internal/mealplan/` | Meal planning calendar — plan store, service (lifecycle, overlap, copy), shopping list bridge (reuses RecipeAggregator + ScaleIngredients), CalDAV calendar sync bridge |
| `internal/recipe/` | Shared recipe types, serving scaler, kitchen fraction formatter, quantity parsing (extracted from list aggregator for reuse by scaler and cook mode) |
| `internal/topics/` | Topic extraction and management — topic CRUD, promotion/archival lifecycle, hot topic detection |
| `internal/web/` | HTMX web UI — search, artifact detail, digest, topics, settings, status, knowledge dashboard (concepts, entities, lint), embedded HTML templates |

## Database Migrations

All migrations live in `internal/db/migrations/` and run automatically on startup via `internal/db/migrate.go` with advisory locking.

Migrations 002–017 were consolidated into `001_initial_schema.sql` during a schema squash. The consolidated migration creates all tables in a single file (31 tables, 86 DDL statements).

| Migration | File | Purpose |
|-----------|------|---------|
| 001 | `001_initial_schema.sql` | Consolidated schema (original migrations 001–017): `artifacts`, `people`, `topics`, `edges`, `sync_state`, `action_items`, `digests`, `synthesis_insights`, `alerts`, `meeting_briefs`, `weekly_synthesis`, `trips`, `trails`, `privacy_consent`, `keep_exports`, `ocr_cache`, `oauth_tokens`, `location_clusters`, `subscriptions`, `learning_progress`, `quick_references`, `search_log`, `guests`, `properties`, `knowledge_concepts`, `knowledge_entities`, `knowledge_lint_reports`, `annotations`, `telegram_message_artifacts`, `lists`, `list_items`. Extensions: `vector`, `pg_trgm`. Includes all indexes, unique constraints, materialized views, and column additions from the original 17 migrations |
| 018 | `018_meal_plans.sql` | Meal planning (spec 036): `meal_plans` + `meal_plan_slots` tables with date range, lifecycle status, slot constraints |
| 019 | `019_expense_tracking.sql` | Expense tracking (spec 034): `vendor_aliases`, `expense_suggestions`, `expense_suggestion_suppressions` tables, GIN index on artifacts expense metadata |

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
removal from existing code:

- Regex-based intent routers (e.g., long `switch`/`if` chains over message
  text in the Telegram bot or any other channel).
- Multi-level hardcoded classification chains in Go (e.g., the current 7-level
  expense classifier) for decisions that involve language understanding,
  vendor judgement, or fuzzy matching.
- Keyword-map categorization (e.g., "if ingredient name contains
  'milk'/'cheese' → dairy") for decisions an LLM with the right tool can make.
- Hardcoded vendor / alias / synonym seed lists in Go source — such data
  belongs behind a tool that consults the database (or asks the LLM with the
  database as context), not as a literal in code.
- Adding a new Go branch to extend a scenario when the same outcome is
  achievable by editing a prompt contract.

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

## NATS JetStream Streams

All NATS subjects and streams are defined in `config/nats_contract.json`. Both Go core and Python ML sidecar have tests that verify their local constants match this contract.

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
| `ANNOTATIONS` | `annotations.>` | Annotation event notifications (core internal) |
| `LISTS` | `lists.>` | List lifecycle events (core internal) |
| `DEADLETTER` | `deadletter.>` | Failed message storage for debugging |