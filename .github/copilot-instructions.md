# Smackerel Development Guidelines

Smackerel is already Bubbles-bootstrapped. The current repository contains Bubbles governance assets, a product design document, and phased specs, but it does not yet contain the runtime implementation. Project-owned bootstrap configuration must stay truthful to that state.

## Current Repo State

- Committed today: `README.md`, `docs/smackerel.md`, `specs/`, `.github/`, and `.specify/memory/`.
- Not committed yet: Go runtime sources, Python ML sidecar sources, Docker Compose stack, or a project CLI such as `./smackerel.sh`.
- Do not invent commands or paths for code that is not present in the repository.

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

Use committed framework validation commands for current work:

| Action | Command | Timeout |
|--------|---------|---------|
| Bootstrap doctor | `bash .github/bubbles/scripts/cli.sh doctor` | 2 min |
| Framework validate | `timeout 1200 bash .github/bubbles/scripts/cli.sh framework-validate` | 20 min |
| Artifact lint | `bash .github/bubbles/scripts/artifact-lint.sh specs/<feature>` | 5 min |
| Traceability guard | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/<feature>` | 10 min |
| Regression baseline guard | `timeout 600 bash .github/bubbles/scripts/regression-baseline-guard.sh specs/<feature> --verbose` | 10 min |
| Runtime build/test/lint | `N/A until runtime sources and repo CLI are committed` | N/A |

## Required Runtime Standards Once Implementation Exists

When runtime code lands, Smackerel must adopt the following standards in the same change set that introduces the implementation.

### One CLI Surface

The runtime must expose one documented entrypoint:

```bash
./smackerel.sh
```

It must own config generation, build, lint, format, test, stack lifecycle, logs, status, and cleanup. Do not document direct `go`, `python`, `docker compose`, or `pytest` commands as the normal project workflow.

### Configuration Single Source Of Truth

- All runtime config values must originate from `config/smackerel.yaml`.
- Generated env files and Compose files are derived artifacts, not hand-edited sources of truth.
- Missing required config must fail loudly. No hidden defaults or fallback hostnames/ports.

### Test Environment Isolation

- Persistent dev state is for manual development only.
- Automated tests must use disposable storage.
- E2E, validation, and chaos runs must never write to the main dev store.

### Smart Docker Lifecycle

- Prefer project-scoped cleanup before broader Docker cleanup.
- Preserve persistent volumes by default.
- Prove build freshness through image identity metadata, not timestamps or `latest` tags.
- Use Compose project names, profiles, and labels for grouping and lifecycle control.

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

### Runtime Test Expectations Once Implementation Exists

| Test Type | Category | Expected Stack | Required? |
|-----------|----------|----------------|-----------|
| Go unit | `unit` | Go core runtime | Always |
| Python unit | `unit` | Python ML sidecar | Always when Python sidecar code changes |
| Integration | `integration` | Go + NATS + Python + PostgreSQL + Ollama | Always |
| E2E API | `e2e-api` | Capture/query/digest flows across the live stack | Always |
| E2E UI | `e2e-ui` | Only if a committed web or mobile UI exists | When a UI is added |
| Stress | `stress` | Ingestion, retrieval, and synthesis hot paths | When perf-sensitive paths change |

Do not fill in runtime commands until the corresponding code and repo-standard command surface are actually committed.

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
- Use committed Bubbles validation commands for bootstrap and artifact checks.
- Do not run build, test, lint, deploy, or service-management commands for a runtime that is not in the repository yet.

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
Committed source:     no runtime source tree committed yet
```

---

## Docker Bundle Freshness Configuration

Not applicable at the current repo state because no frontend container or bundled UI has been committed yet.

| Key | Value |
|-----|-------|
| Frontend container | `N/A - no frontend container committed` |
| Frontend image | `N/A - no frontend image committed` |
| Static root | `N/A - no bundled frontend committed` |
| Stop command | `N/A - no runtime CLI committed` |
| Build command | `N/A - no runtime CLI committed` |
| Start command | `N/A - no runtime CLI committed` |
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

When runtime code lands, update this audit to use the committed repo-standard runtime commands in the same change set.
