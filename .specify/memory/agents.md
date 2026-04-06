# Smackerel — Agent Command Registry

> This file is the single source of truth for project commands and runtime expectations.
> Current repo state: Bubbles bootstrap assets, product design, phased specs, a Go core, a Python ML sidecar, Docker Compose, YAML-backed config generation, and the `./smackerel.sh` runtime CLI are committed.

---

## I. Context Loading Priority

| Priority | File | Purpose |
|----------|------|---------|
| 1 | `.specify/memory/constitution.md` | Project governance |
| 2 | `.specify/memory/agents.md` | Command registry (this file) |
| 3 | `.github/copilot-instructions.md` | Project policies |
| 4 | `.github/agents/bubbles_shared/agent-common.md` | Universal governance |
| 5 | `.github/agents/bubbles_shared/scope-workflow.md` | Workflow templates |

---

## II. Design Document References

| Document | Path |
|----------|------|
| Product overview | `README.md` |
| Product design and architecture | `docs/smackerel.md` |
| Development guide | `docs/Development.md` |
| Testing guide | `docs/Testing.md` |
| Docker lifecycle guide | `docs/Docker_Best_Practices.md` |
| Phase specifications | `specs/` |

---

## III. Verification Commands

### CLI Entrypoint
```
CLI_ENTRYPOINT=./smackerel.sh
```

### Current Repo Validation Commands
```
FRAMEWORK_DOCTOR_COMMAND=bash .github/bubbles/scripts/cli.sh doctor
FRAMEWORK_VALIDATE_COMMAND=timeout 1200 bash .github/bubbles/scripts/cli.sh framework-validate
ARTIFACT_LINT_COMMAND=bash .github/bubbles/scripts/artifact-lint.sh specs/<feature>
TRACEABILITY_GUARD_COMMAND=timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/<feature>
REGRESSION_BASELINE_GUARD_COMMAND=timeout 600 bash .github/bubbles/scripts/regression-baseline-guard.sh specs/<feature> --verbose
```

### Runtime Build/Test/Lint Commands
```
CONFIG_COMMAND=./smackerel.sh config generate
BUILD_COMMAND=./smackerel.sh build
CHECK_COMMAND=./smackerel.sh check
LINT_COMMAND=./smackerel.sh lint
FORMAT_COMMAND=./smackerel.sh format --check
UNIT_TEST_GO_COMMAND=./smackerel.sh test unit --go
UNIT_TEST_PYTHON_COMMAND=./smackerel.sh test unit --python
INTEGRATION_COMMAND=./smackerel.sh test integration
E2E_API_COMMAND=./smackerel.sh test e2e
E2E_UI_COMMAND=N/A - no committed UI application yet
STRESS_COMMAND=./smackerel.sh test stress
UP_COMMAND=./smackerel.sh up
DOWN_COMMAND=./smackerel.sh down
STATUS_COMMAND=./smackerel.sh status
LOGS_COMMAND=./smackerel.sh logs
CLEAN_COMMAND=./smackerel.sh clean smart
```

---

## IV. Code Patterns

| Category | Convention |
|----------|-----------|
| Source code | Go core under `cmd/` and `internal/`, Python ML sidecar under `ml/` |
| Tests | Go unit tests, Python unit tests, live-stack integration, E2E, and stress smoke commands flow through `./smackerel.sh` |
| Specs | `specs/` |
| Config | Runtime config originates from `config/smackerel.yaml` and generates env files under `config/generated/` |
| Docs | `README.md` and `docs/` |

---

## V. Planned Runtime Stack Declaration

- **Core runtime:** Go for API, connectors, scheduler, knowledge graph, lifecycle engine, digest assembly, and channel delivery.
- **ML sidecar:** Python only for embeddings, LLM gateway work, and extraction fallbacks that do not have a strong Go alternative.
- **Primary storage:** PostgreSQL with pgvector.
- **Async boundary:** NATS JetStream between the Go core and Python sidecar.
- **Local inference:** Ollama.
- **Deployment model:** Docker Compose on user-controlled hardware.

### Runtime Operational Contract

- **Single CLI:** all runtime operations flow through `./smackerel.sh`.
- **SSOT config:** runtime config originates from `config/smackerel.yaml` and generated env artifacts.
- **Environment isolation:** dev state is persistent; test state is isolated and disposable through CLI cleanup.
- **Smart cleanup:** default cleanup preserves persistent dev stores.
- **Freshness proof:** build and compose wiring flow through generated config and the repo CLI.

---

## VI. Error Resolution

When encountering errors:
1. Read the full error output without truncation.
2. Check whether the failure is in the changed files or a pre-existing repo issue.
3. Fix misleading project-owned bootstrap config immediately when it drifts from repo truth.
4. Re-run the failing validation command to verify the fix.

---

## VII. Quality Standards

- Project-owned Bubbles configuration must stay free of placeholder commands and dead document links.
- Runtime stack declarations must match `docs/smackerel.md`.
- Runtime command, testing, and Docker lifecycle rules must match `docs/Development.md`, `docs/Testing.md`, and `docs/Docker_Best_Practices.md`.
- Do not advertise commands, CLIs, or source paths that are not committed in the repository.
- Keep this registry, `.github/copilot-instructions.md`, `docs/Development.md`, and terminal discipline synchronized with the CLI surface.

---

## VIII. Sources of Truth

| Item | Source |
|------|--------|
| Commands and validation entrypoints | This file (`agents.md`) |
| Project policies | `.github/copilot-instructions.md` |
| Governance | `.specify/memory/constitution.md` |
| Product architecture | `docs/smackerel.md` |
| Universal rules | `.github/agents/bubbles_shared/agent-common.md` |
| Workflow config | `.github/bubbles/workflows.yaml` |
