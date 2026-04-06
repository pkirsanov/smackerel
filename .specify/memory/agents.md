# Smackerel — Agent Command Registry

> This file is the single source of truth for project commands and runtime expectations.
> Current repo state: Bubbles bootstrap assets, product design, and phased specs are committed. The runtime implementation is not committed yet, so this registry distinguishes available framework validation commands from planned runtime command surfaces.

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
| Phase specifications | `specs/` |

---

## III. Verification Commands

### CLI Entrypoint
```
CLI_ENTRYPOINT=N/A - no project runtime CLI is committed yet
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
BUILD_COMMAND=N/A - Go core and Python sidecar source trees are not committed yet
CHECK_COMMAND=N/A - no runtime source tree is committed yet
LINT_COMMAND=N/A - no runtime source tree is committed yet
FORMAT_COMMAND=N/A - no runtime source tree is committed yet
UNIT_TEST_GO_COMMAND=N/A - no Go sources are committed yet
UNIT_TEST_PYTHON_COMMAND=N/A - no Python sources are committed yet
INTEGRATION_AND_E2E_API_COMMAND=N/A - no runnable services or Docker Compose stack are committed yet
E2E_UI_COMMAND=N/A - no UI application is committed yet
DEV_ALL_COMMAND=N/A - no runtime CLI or Compose stack is committed yet
DEV_ALL_SYNTH_COMMAND=N/A - no synthetic-data dev stack is committed yet
DOWN_COMMAND=N/A - no stack lifecycle command surface is committed yet
STATUS_COMMAND=bash .github/bubbles/scripts/cli.sh doctor
```

---

## IV. Code Patterns

| Category | Convention |
|----------|-----------|
| Source code | Not committed yet. Planned runtime is a Go core plus a Python ML sidecar, as defined in `docs/smackerel.md` Section 23. |
| Tests | Not committed yet. Planned coverage is Go unit tests, Python unit tests, integration tests, and end-to-end workflow tests. |
| Specs | `specs/` |
| Config | Project bootstrap config lives in `.github/` and `.specify/memory/`; runtime config will be added with the implementation. |
| Docs | `README.md` and `docs/` |

---

## V. Planned Runtime Stack Declaration

- **Core runtime:** Go for API, connectors, scheduler, knowledge graph, lifecycle engine, digest assembly, and channel delivery.
- **ML sidecar:** Python only for embeddings, LLM gateway work, and extraction fallbacks that do not have a strong Go alternative.
- **Primary storage:** PostgreSQL with pgvector.
- **Async boundary:** NATS JetStream between the Go core and Python sidecar.
- **Local inference:** Ollama.
- **Deployment model:** Docker Compose on user-controlled hardware.

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
- Do not advertise commands, CLIs, or source paths that are not committed in the repository.
- When runtime code lands, update this registry, `.github/copilot-instructions.md`, and terminal discipline in the same change set.

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
