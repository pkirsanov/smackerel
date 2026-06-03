# Smackerel Development Guidelines

Smackerel is already Bubbles-bootstrapped. The current repository contains Bubbles governance assets, phased specs, a Go core runtime, a Python ML sidecar, Docker Compose, and a repo-standard CLI. Project-owned bootstrap configuration must stay truthful to that state.

## Current Repo State

- Committed today: `README.md`, `docs/smackerel.md`, `specs/`, `.github/`, `.specify/memory/`, Go runtime sources under `cmd/` and `internal/`, the Python ML sidecar under `ml/`, `docker-compose.yml`, `config/smackerel.yaml`, and `./smackerel.sh`.
- The current runtime surface covers the foundation scaffold: config generation, image builds, container lifecycle, unit tests, live-stack health checks, E2E scaffold tests, and stress smoke checks.
- Do not invent commands or paths that are not present in the repository, and do not treat ad-hoc runtime commands as the sanctioned workflow when `./smackerel.sh` already owns the surface.

## Documentation References

Project-owned operational docs live in:

- `README.md` — project overview and runtime standards summary
- `docs/smackerel.md` — product and architecture design
- `docs/Development.md` — current repo state plus required runtime command/config contract
- `docs/Testing.md` — bootstrap validation and future runtime testing rules
- `docs/Docker_Best_Practices.md` — Docker lifecycle, cleanup, freshness, and isolation rules

## Planned Runtime Stack

| Area | Technology | Scope |
|------|------------|-------|
| Core runtime | Go (Chi or Gin) | API, connectors, scheduler, graph, lifecycle, digests, delivery |
| ML sidecar | Python (FastAPI) | Embeddings, LLM gateway, transcript and extraction fallback |
| Database | PostgreSQL + pgvector | Canonical artifact store and vector search |
| Message bus | NATS JetStream | Async boundary between Go and Python |
| Local inference | Ollama | Local model execution |
| Deployment | Docker Compose | Self-hosted local deployment |

This stack comes from `docs/smackerel.md` and should be treated as the project truth unless that document changes.

---

## Commands

Use `./smackerel.sh` for runtime work and the committed Bubbles commands for framework/spec governance:

| Action | Command | Timeout |
|--------|---------|---------|
| Config generate | `./smackerel.sh config generate` | 1 min |
| Build | `./smackerel.sh build` | 20 min |
| Check | `./smackerel.sh check` | 2 min |
| Lint | `./smackerel.sh lint` | 10 min |
| Format | `./smackerel.sh format --check` | 10 min |
| Test unit | `./smackerel.sh test unit` | 10 min |
| Test integration | `./smackerel.sh test integration` | 10 min |
| Test e2e | `./smackerel.sh test e2e` | 15 min |
| Test e2e-ui | `./smackerel.sh test e2e-ui` | 15 min |
| Test stress | `./smackerel.sh test stress` | 10 min |
| Up | `./smackerel.sh up` | 5 min |
| Down | `./smackerel.sh down` | 2 min |
| Status | `./smackerel.sh status` | 2 min |
| Logs | `./smackerel.sh logs` | 5 min |
| Clean smart | `./smackerel.sh clean smart` | 3 min |
| Config bundle | `./smackerel.sh config generate --env <env> --bundle --source-sha <sha>` | 1 min |
| Deploy target apply | `./smackerel.sh deploy-target <target> apply --image-core=sha256:<d> --image-ml=sha256:<d> --config-bundle=<env>-<sha> --config-bundle-sha=<sha256-hex>` | 5 min |
| Deploy target rollback | `./smackerel.sh deploy-target <target> rollback` | 1 min |
| Deploy target verify | `./smackerel.sh deploy-target <target> verify` | 1 min |
| Promote build manifest | `bash scripts/deploy/promote.sh --target <target> --build-manifest <path>` | 5 min |
| Bootstrap doctor | `bash .github/bubbles/scripts/cli.sh doctor` | 2 min |
| Framework validate | `timeout 1200 bash .github/bubbles/scripts/cli.sh framework-validate` | 20 min |
| Artifact lint | `bash .github/bubbles/scripts/artifact-lint.sh specs/<feature>` | 5 min |
| Traceability guard | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/<feature>` | 10 min |
| Regression baseline guard | `timeout 600 bash .github/bubbles/scripts/regression-baseline-guard.sh specs/<feature> --verbose` | 10 min |

## Required Runtime Standards

The committed runtime already has a repo-standard operational surface. New work must preserve it.

### One CLI Surface

The runtime entrypoint is:

```bash
./smackerel.sh
```

It must own config generation, build, lint, format, test, stack lifecycle, logs, status, and cleanup. Do not document direct `go`, `python`, `docker compose`, or `pytest` commands as the normal project workflow.

### Configuration Single Source Of Truth

- All runtime config values must originate from `config/smackerel.yaml`.
- Generated env files and Compose files are derived artifacts, not hand-edited sources of truth.
- Missing required config must fail loudly. No hidden defaults or fallback hostnames/ports.

### Generated Files (DO NOT EDIT DIRECTLY)

| File | Purpose |
|------|---------|
| `config/generated/dev.env` | Development environment variables |
| `config/generated/test.env` | Test environment variables |

Regenerate all config: `./smackerel.sh config generate`

### SST Zero-Defaults Enforcement (NON-NEGOTIABLE)

**ALL configuration values MUST originate from `config/smackerel.yaml`. Zero hardcoded ports, URLs, hostnames, or fallback defaults anywhere in the codebase.**

### Secrets Management

| Aspect | Details |
|--------|---------|
| **Dev secrets** | Inline in `config/smackerel.yaml` as empty-string placeholders (except dev DB password) |
| **Secret fields** | `runtime.auth_token`, `llm.api_key`, `telegram.bot_token`, connector `access_token` fields |
| **Gitignored** | `config/generated/` directory — resolved env files with secrets are not committed |
| **Production** | MUST set all secret values via environment variables or populate `smackerel.yaml` before `config generate` |

**Rules:**
- Empty-string placeholders in `smackerel.yaml` are the intended dev pattern — services must validate at startup
- Dev DB password (`smackerel`) is acceptable for local dev only — override for any non-local deployment
- `auth_token`, `api_key`, `bot_token` MUST be set before runtime — services should fail-loud if empty
- Generated env files (`config/generated/*.env`) contain resolved secrets — NEVER commit them

| Language | FORBIDDEN | REQUIRED |
|----------|-----------|----------|
| **Shell** | `${VAR:-default}` with fallback | `${VAR:?error message}` fail-loud |
| **Go** | `getEnv("KEY", "fallback")` | `os.Getenv("KEY")` + empty check → fatal |
| **Python** | `os.getenv("KEY", "default")` | `os.environ["KEY"]` (raises KeyError) |

### Test Environment Isolation

- Persistent dev state is for manual development only.
- Automated tests must use disposable storage.
- E2E, validation, and chaos runs must never write to the main dev store.

### Smart Docker Lifecycle

- Prefer project-scoped cleanup before broader Docker cleanup.
- Preserve persistent volumes by default.
- Prove build freshness through image identity metadata, not timestamps or `latest` tags.
- Use Compose project names, profiles, and labels for grouping and lifecycle control.

### Deployment Ownership Boundary (NON-NEGOTIABLE)

This repo's deployment surface is **generic and target-agnostic**. It produces
immutable build artifacts (signed images + per-env config bundles via
`./smackerel.sh config generate --env <env> --bundle`) and exposes adapter
contracts. It does NOT hold environment-specific final configuration.

Home-lab and all other environment-specific final configuration — real
hostnames, real IPs, real Tailscale tailnet identity (tailnet IDs, FQDNs,
CGNAT IPs), real Caddy/nginx site files, real `ufw` rules, real systemd unit
names tied to an operator's host, real secret values, and real per-target
`manifest.yaml` / `params.yaml` — lives in the operator-private deploy-adapter
overlay repo. Adding any operator-coupled topology to THIS repo is a
**blocking policy violation**; that content belongs in the deploy adapter.

| FORBIDDEN in this repo | ALLOWED in this repo |
|------------------------|----------------------|
| Real hostnames | Generic placeholders (`<home-lab-host>`, `<deploy-host>`) |
| Real IP addresses (CGNAT, public, RFC 6598) | `127.0.0.1`, `localhost`, RFC1918 references |
| Real Tailscale tailnet IDs / FQDNs (`*.ts.net`) | `<tailnet-id>`, `<tailnet-fqdn>` placeholders |
| Real Caddy/nginx site filenames | Generic per-product fragment naming (`<product>.caddy`) |
| Real systemd unit names that name the operator's host | Generic `<product>-*.service` references |
| Real secret values in any committed file | `${VAR}` substitution placeholders + sops/age in the deploy adapter |
| Hand-edited per-target `manifest.yaml` / `params.yaml` | Adapter contract docs + generic env vars |

The line is simple: if a value would change when a different operator deploys
Smackerel to a different machine, it does NOT belong in this repo. It belongs
in the deploy adapter. The Smackerel repo MUST stay deployable to ANY target by
ANY operator; the per-target adapter is what binds it to a real machine.

### Build-Once Deploy-Many (BLOCKING — bubbles G074)

Smackerel deployments follow the Build-Once Deploy-Many architecture. The same git
SHA produces immutable artifacts that any environment consumes:

| Artifact | Identifier | Mutable? |
|----------|-----------|----------|
| `smackerel-core` image | `ghcr.io/pkirsanov/smackerel-core@sha256:<digest>` | No |
| `smackerel-ml` image   | `ghcr.io/pkirsanov/smackerel-ml@sha256:<digest>`   | No |
| Config bundle (per env) | `ghcr.io/pkirsanov/smackerel-config-bundles:<env>-<sourceSha>` | No, deterministic |
| Build manifest         | `build-manifest-<sourceSha>.yaml` (CI artifact)    | No |
| Deployment manifest    | `deploy/<target>/manifest.yaml` (pointer)          | Yes (operator-controlled) |

**Producers vs deployers:**

- `.github/workflows/build.yml` — builds, signs (cosign keyless + Rekor), attests
  (SBOM + SLSA provenance), generates per-env config bundles, publishes to ghcr,
  writes `build-manifest-<sourceSha>.yaml`. **STOPS at registry push. NO SSH. NO apply.**
- `deploy/<target>/` — adapter scripts that pull by digest, verify signatures,
  swap manifest pointer, restart. Owns ALL target-specific knowledge.
- `scripts/deploy/promote.sh` — operator entrypoint: reads `build-manifest.yaml`,
  resolves digests + bundle ref for the target's environment, calls
  `./smackerel.sh deploy-target <target> apply ...`.
- `scripts/deploy/rollback.sh` — operator entrypoint: pure pointer-swap rollback.

**Forbidden in any deployment surface:**

- Mutable image tags in manifest (`:latest`, `:main`, branch names) — digests only
- CI workflow performing `apply`/`deploy`/`ssh` — wrong trust boundary
- Adapter `apply.sh` invoking `docker build` / `cargo build` / `npm run build`
- Adapter falling back to local build on registry pull failure
- Missing cosign verification before container start
- Missing bundle hash verification
- `rollback.sh` rebuilding instead of pointer-swap
- Target-side bundle generation (bundle is a build artifact, not a deploy artifact)
- Plaintext secrets in bundle (use injected env vars / sealed secrets)
- Non-deterministic bundle (two CI runs on same SHA producing different bundles)
- Two targets sharing one `manifest.yaml` (each target owns its own pointer)

**Operator commands:**

```bash
./smackerel.sh deploy-target home-lab apply \
    --image-core=sha256:<digest> --image-ml=sha256:<digest> \
    --config-bundle=home-lab-<sourceSha> \
    --config-bundle-sha=<sha256-hex>
./smackerel.sh deploy-target home-lab verify
./smackerel.sh deploy-target home-lab rollback
bash scripts/deploy/promote.sh --target home-lab --build-manifest <path>
```

See [`docs/Deployment.md`](../docs/Deployment.md) for full operator workflow,
[`.github/instructions/bubbles-deployment-target.instructions.md`](instructions/bubbles-deployment-target.instructions.md)
and [`.github/skills/bubbles-deployment-target-adapter/SKILL.md`](skills/bubbles-deployment-target-adapter/SKILL.md)
for framework rationale.

### Tailnet-Edge Bind Pattern (home-lab/production targets)

Smackerel's home-lab and production deployments use the canonical
tailnet-edge bind pattern (see
`bubbles/skills/bubbles-tailnet-edge-pattern/SKILL.md` in the framework
repo). The deploy compose file `deploy/compose.deploy.yml` ships with
adapter-ready L3 invariants. Future agents and operators MUST preserve
these invariants:

| Service              | Host port mapping                                                            | DevOps access path |
|----------------------|------------------------------------------------------------------------------|--------------------|
| `smackerel-core`     | `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}`   | HTTP UI fronted by host Caddy on the tailnet IP (Pattern P5) |
| `smackerel-ml`       | `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${ML_HOST_PORT}:${ML_CONTAINER_PORT}`       | HTTP UI fronted by host Caddy on the tailnet IP (Pattern P5) |
| `postgres`           | **None** (no `ports:` block)                                                  | `tailscale ssh <host> -- docker exec -it smackerel-<env>-postgres psql ...` (Pattern P1) |
| `nats`               | **None** (no `ports:` block)                                                  | `tailscale ssh <host> -- docker exec -it smackerel-<env>-nats nats ...` (Pattern P1) |

Forbidden — `literal 127.0.0.1: in deploy/compose.deploy.yml is forbidden`
for the `smackerel-core` and `smackerel-ml` `ports:` entries (this is the
spec 020 form and is reversed by spec 042). Also forbidden — the
`${HOST_BIND_ADDRESS:-127.0.0.1}:` default-fallback form. Gate G028's
NO-DEFAULTS / fail-loud SST policy requires the fail-loud
`${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:`
form. The deploy adapter MUST write `HOST_BIND_ADDRESS` explicitly into the
bundled `app.env`; if it is missing or empty, Docker Compose MUST abort at
substitution time with the named error.

When touching config, Compose, deployment docs, instructions, or skills that
mention runtime defaults, load and follow
`.github/instructions/smackerel-no-defaults.instructions.md` and
`.github/skills/smackerel-no-defaults/SKILL.md`.

Forbidden — re-publishing host ports for `postgres` or `nats` in
`deploy/compose.deploy.yml`. Infra services have no business reason to be
reachable from outside the compose network on home-lab; Pattern P1
(`docker exec` over Tailscale SSH) is the recommended and only DevOps
access path.

Enforced — `internal/deploy/compose_contract_test.go` parses the live
compose file on every `./smackerel.sh test unit --go` run and fails the
build if either invariant regresses. The test includes adversarial
sub-tests that prove it would catch a regression to the spec 020 literal
form or to a re-published infra port.

References:
- `specs/042-tailnet-edge-bind-pattern/` — spec, design, scope DoD
- `bubbles/skills/bubbles-tailnet-edge-pattern/SKILL.md` — canonical pattern
- `docs/Operations.md` → "DevOps Access on Home-Lab (Tailnet-Edge Pattern)"

---

## Testing Requirements

### Current Bootstrap Validation

| Test Type | Category | Command | Required? |
|-----------|----------|---------|-----------|
| Bootstrap health | `framework` | `bash .github/bubbles/scripts/cli.sh doctor` | Always when project-owned bootstrap files change |
| Framework validation | `framework` | `timeout 1200 bash .github/bubbles/scripts/cli.sh framework-validate` | Always before claiming bootstrap is healthy |
| Artifact lint | `artifact` | `bash .github/bubbles/scripts/artifact-lint.sh specs/<feature>` | When feature or bug artifacts change |
| Traceability guard | `artifact` | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/<feature>` | When traceability-sensitive spec content changes |
| Regression baseline guard | `artifact` | `timeout 600 bash .github/bubbles/scripts/regression-baseline-guard.sh specs/<feature> --verbose` | When changing managed docs or competitive baselines |

### Runtime Test Expectations

| Test Type | Category | Expected Stack | Required? |
|-----------|----------|----------------|-----------|
| Go unit | `unit` | Go core runtime | Always |
| Python unit | `unit` | Python ML sidecar | Always when Python sidecar code changes |
| Integration | `integration` | Go + NATS + Python + PostgreSQL + Ollama | Always |
| E2E API | `e2e-api` | Capture/query/digest flows across the live stack | Always |
| E2E UI | `e2e-ui` | Only if a committed web or mobile UI exists | When a UI is added |
| Stress | `stress` | Ingestion, retrieval, and synthesis hot paths | When perf-sensitive paths change |

Runtime commands are committed and must stay aligned with this file and `.specify/memory/agents.md`.

### Planned Runtime CLI Contract

The future runtime command surface must converge on:

- `./smackerel.sh config generate`
- `./smackerel.sh build`
- `./smackerel.sh check`
- `./smackerel.sh lint`
- `./smackerel.sh format`
- `./smackerel.sh test unit`
- `./smackerel.sh test integration`
- `./smackerel.sh test e2e`
- `./smackerel.sh test e2e-ui`
- `./smackerel.sh test stress`
- `./smackerel.sh up`
- `./smackerel.sh down`
- `./smackerel.sh status`
- `./smackerel.sh logs`
- `./smackerel.sh clean smart|full|status|measure`

### Live-Stack Test Authenticity

- Tests labeled `integration`, `e2e-api`, `e2e-ui`, or otherwise described as live-stack MUST hit the real running system.
- If a test uses request interception such as `route()`, `intercept()`, `msw`, `nock`, or equivalent, it is mocked and MUST be classified as `unit`, `functional`, or `ui-unit` instead.

### E2E And Validation Isolation

- `./smackerel.sh test e2e` must run against the disposable test stack, never the persistent dev stack.
- Validation and chaos workflows must use isolated Compose projects and disposable state.
- Synthetic test fixtures must be uniquely identifiable and safe to clean up.

### Adversarial Regression Tests For Bug Fixes

- Every bug-fix regression test MUST include at least one adversarial case that would fail if the bug were reintroduced.
- Tautological regressions are forbidden: if all fixtures already satisfy the broken filter, gate, or path, the regression cannot detect the bug.
- Required tests MUST NOT use bailout returns such as `if (page.url().includes('/login')) { return; }` or equivalent failure-condition early exits.

---

## Terminal Discipline

See `instructions/terminal-discipline.instructions.md` for current terminal rules.

At the present repo state:
- Use `./smackerel.sh` for runtime build, test, lint, deploy-adjacent lifecycle, and service-management operations.
- Use committed Bubbles validation commands for bootstrap and artifact checks.

---

## Bubbles Artifacts & Workflow

This applies to all work, whether initiated via a `bubbles.*` prompt or a regular agent request.

Full workflow rules, artifact templates, and verification gates are in:
- `agents/bubbles_shared/agent-common.md`
- `agents/bubbles_shared/scope-workflow.md`

### Required Artifacts

Before feature work begins, all artifacts must exist in `specs/[feature]/`:

| Artifact | Purpose |
|----------|---------|
| `spec.md` | Feature specification |
| `design.md` | Design document |
| `scopes.md` | Scope definitions + DoD |
| `report.md` | Execution evidence |
| `uservalidation.md` | User acceptance |
| `state.json` | Execution state |

### Work Classification

All work must be organized under feature or bug folders:
- Features: `specs/NNN-feature-name/`
- Bugs: `specs/[feature]/bugs/BUG-NNN-description/`

### Terminal Status Is Per-Mode (Not Always `done`)

A spec or bug is **complete for its workflow mode** when its `state.json` status is terminal-for-mode, NOT only when it equals the literal string `"done"`. Terminal-for-mode means: `status == "done"` OR `status == mode.statusCeiling` OR `status ∈ mode.terminalAliases` (see [`docs/Operations.md` → Terminal Status by Workflow Mode](../docs/Operations.md#terminal-status-by-workflow-mode)).

| Mode family | Terminal status |
|-------------|-----------------|
| `validate-only`, `audit-only`, `validate-to-doc` | `validated` |
| `docs-only`, `spec-review-to-doc`, `retro-to-review`, `release-planning-to-doc` | `docs_updated` |
| `spec-scope-hardening`, `product-to-planning` | `specs_hardened` |
| `adapter-readiness-to-packet`, `dark-launch-shipped`, `migration-shipped-pending-cutover` | `delivered_pending_activation` |
| everything else | `done` |

Rules for agents:

- DO NOT attempt to "promote" a ceiling-bound packet to `done`. The state-transition guard will (correctly) refuse, and re-orchestrating through `bugfix-fastlane` to force `done` is fake make-work — the actual fix already shipped.
- When sweeping the portfolio for "open work," use `bash .github/bubbles/scripts/is-terminal-for-mode.sh "$status" "$mode"` (exit 0 = terminal-for-mode) instead of comparing to `"done"`. The `spec-dashboard.sh`, `bubbles.status`, `bubbles.recap`, and retro tooling already do this.
- Ceiling enforcement in `state-transition-guard.sh` is unchanged — promotion past the ceiling remains forbidden.

---

## Language Discipline

- Go owns the primary runtime and connector layer.
- Python is limited to ML-sidecar responsibilities where the design explicitly calls for it.
- PostgreSQL + pgvector is the canonical data store.
- NATS JetStream is the async coordination boundary.
- Docker Compose is the expected local deployment mechanism once runtime code exists.

---

## Key Locations

```text
Product overview:     README.md
Architecture/design:  docs/smackerel.md
Development guide:    docs/Development.md
Testing guide:        docs/Testing.md
Docker operations:    docs/Docker_Best_Practices.md
Specifications:       specs/
Bootstrap config:     .github/ and .specify/memory/
Committed source:     cmd/, internal/, ml/, docker-compose.yml, config/, smackerel.sh
```

---

## Docker Bundle Freshness Configuration

Not applicable at the current repo state because no frontend container or bundled UI has been committed yet.

| Key | Value |
|-----|-------|
| Frontend container | `N/A - no frontend container committed` |
| Frontend image | `N/A - no frontend image committed` |
| Static root | `N/A - no bundled frontend committed` |
| Stop command | `./smackerel.sh down` |
| Build command | `./smackerel.sh build` |
| Start command | `./smackerel.sh up` |
| Bundler | `N/A - no bundled frontend committed` |

---

## Pre-Completion Self-Audit

Before marking bootstrap or artifact work done:

```bash
# 1. Verify project-owned bootstrap files contain no placeholder markers
grep -rn 'TODO|\[TODO' .github/copilot-instructions.md .specify/memory/agents.md .specify/memory/constitution.md .github/instructions/terminal-discipline.instructions.md

# 2. Run Bubbles bootstrap doctor
bash .github/bubbles/scripts/cli.sh doctor

# 3. Run full framework validation
timeout 1200 bash .github/bubbles/scripts/cli.sh framework-validate

# 4. If specs changed, run artifact lint for the affected feature
bash .github/bubbles/scripts/artifact-lint.sh specs/<feature>

# 5. If traceability-sensitive artifacts changed, run traceability guard
timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/<feature>
```

Keep this audit aligned with the committed repo-standard runtime commands in the same change set that changes the CLI surface.

---

## No Env-Specific Content In This Repo (NON-NEGOTIABLE)

**The Smackerel repo MUST be 100% generic. Zero environment-specific content. Zero personal data. Zero deployment-target identifiers.**

Environment-specific content (real hostnames, real IPs, real Linux usernames, real geographic locations, real Tailscale tailnet identifiers, real overlay-repo names) MUST live in the deploy-overlay repo, NEVER in this repo. The deploy adapter overlay owns all per-target knowledge; this repo provides only the abstract substitution points.

### What Is Forbidden In This Repo

| Class | Where it belongs |
|-------|------------------|
| Real Linux usernames and home-directory paths | local machine only — never committed |
| Real deploy-host short names | deploy-overlay repo |
| Real Tailscale tailnet identifiers and FQDNs | deploy-overlay repo |
| Real Tailscale CGNAT IPv4 addresses (RFC 6598 shared address space) | deploy-overlay repo |
| Real geographic locations (city, region, country) | nowhere — irrelevant to a generic runtime |
| The literal short name of the deploy-overlay repo | refer generically as "deploy adapter" |
| Real customer or tenant names | nowhere |
| Private key blocks (any `-----BEGIN ... PRIVATE KEY-----`) | secret manager only |

### What Is Allowed (Stays In This Repo)

| Identifier | Reason |
|------------|--------|
| The owner's GitHub username | Public, intentional. Used as the registry namespace owner (`ghcr.io/<owner>/...`) and copyright holder. |
| `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}` substitution form | Abstract substitution point with fail-loud SST enforcement — the deploy adapter must set the value explicitly at apply time. |
| Generic placeholders (`<deploy-host>`, `<deploy-host-fqdn>`, `<tailnet-id>`, `<tailnet-fqdn>`, `<host-tailnet-ip>`, `<sample-location>`) | Used in docs to describe DevOps shapes without leaking real values. |
| `127.0.0.1`, `localhost`, RFC1918 private ranges | Generic loopback / private-net references. |
| Test fixtures with placeholder secrets | Covered by `.gitleaks.toml` allowlist for test paths. |

### Enforcement (Layered Defense)

1. **Local pre-commit hook** runs `bash .github/bubbles/scripts/pii-scan.sh` against the staged diff. Blocks the commit if any pattern in `.gitleaks.toml` matches OR if any token in `~/.config/bubbles/pii-tokens.txt` (machine-local, gitignored) appears.
2. **CI workflow** `.github/workflows/gitleaks.yml` re-runs gitleaks against the diff on every push and PR. Blocks the merge on any finding.
3. **Code review** rejects any change that introduces real env-specific values, even if the scanners did not flag them.

### When You Need Environment-Specific Behavior

Implement abstract substitution points in this repo (env vars, config keys, generic placeholders in docs) and let the deploy adapter overlay populate the real values at apply time. NEVER embed the real values here.

### Bypass Policy

`SKIP_PII_SCAN=1 git commit ...` is reserved for genuine emergencies and MUST be justified in the commit message body. Bypass usage is reviewed by CI and by audit. Repeat bypass without legitimate cause is a rule violation.

