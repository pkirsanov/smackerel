<!-- governance-version: 3.0.0 -->
# Project Configuration Contract (Cross-Project)

> **This document defines the interface between the Bubbles agent framework (project-agnostic) and any specific project that uses Bubbles.** Every project MUST supply the values defined here. Bubbles agents MUST read these values via indirection — never hardcode project-specific details.
>
> **Companion docs:** Use repository-specific setup guidance if present; see [agent-common.md](agent-common.md) for universal governance and [scope-workflow.md](scope-workflow.md) for workflow templates.

---

## Purpose

Bubbles agents are **project-agnostic**. They enforce universal governance: sequential spec completion, anti-fabrication, evidence standards, quality gates, and workflow orchestration. But they need project-specific information to execute commands, validate builds, and verify tests.

This document defines:
1. **What projects MUST provide** — the required project configuration
2. **Where projects provide it** — the canonical file locations
3. **How Bubbles agents consume it** — the indirection rules
4. **What is portable** — skills, instructions, agents, and governance that copy unchanged across projects
5. **What is project-specific** — the configuration that each project must customize

---

## Required Project Configuration Files

Every project using Bubbles MUST have these files:

| File | Purpose | Required |
|------|---------|----------|
| `.specify/memory/agents.md` | **Command registry** — CLI entrypoint, build/test/lint/format commands, file organization, naming conventions, tech stack declaration | **YES** |
| `.specify/memory/constitution.md` | **Governance principles** — project-specific principles layered on top of universal governance | **YES** |
| `.github/copilot-instructions.md` | **Project policies** — project-specific rules, testing requirements, Docker config, port allocation, command tables | **YES** |
| `.github/bubbles-project.yaml` | **Project-owned framework extensions** — scan-pattern overrides, managed-doc registry overrides, and custom gates (G100+) | **OPTIONAL** |

---

## Configuration Single Source of Truth (SST) — Required Pattern

Every project using Bubbles MUST implement a Configuration SST pipeline. This ensures all runtime configuration values flow from a single canonical file, preventing config drift, hardcoded values, and silent defaults.

### SST Pipeline Requirements

| Requirement | Description |
|-------------|-------------|
| **SST File** | A single YAML file (`config/<project>.yaml`) where ALL config values are defined |
| **Config Generator** | A script that parses the SST file and produces all derived config files |
| **CLI Command** | A `config generate` (or `config compile`) subcommand on the project CLI |
| **Generated File Manifest** | A documented list of generated files in `copilot-instructions.md` marked "DO NOT EDIT" |
| **Fail-Loud Validation** | Every required value validated at generation time and startup time — no silent defaults |
| **Per-Language Enforcement** | Rules in `copilot-instructions.md` forbidding fallback patterns per language |

### SST File Structure (Canonical Layout)

```yaml
# config/<project>.yaml — SINGLE SOURCE OF TRUTH
project:
  name: <project-name>
  namespace: <namespace>

ports:
  <service>:
    host: <host-port>        # Exposed to host
    internal: <internal-port> # Inside container

services:
  <service-name>:
    container: <container-name>
    image: <image-name>
    env: { ... }

infrastructure:
  database: { ... }
  cache: { ... }
  messaging: { ... }

environments:
  dev: { ... }
  test: { ... }
```

### What MUST Be in the SST File

All values that appear in generated `.env` files, Docker Compose port mappings, frontend build-time config, or test environment config MUST originate from the SST file. This includes ports, URLs, credentials (or secret refs), timeouts, resource limits, and service identity (container/image/volume names).

### What copilot-instructions.md MUST Document

Projects MUST list all generated files and their purpose, the config generation command, per-language zero-defaults enforcement rules, and port allocation from the SST. See the `bubbles-config-sst` instruction and skill for the full enforcement model.

### References

- Instruction: `instructions/bubbles-config-sst.instructions.md`
- Skill: `skills/bubbles-config-sst/SKILL.md`
- Docker port standards: `skills/bubbles-docker-port-standards/SKILL.md`

---

## `.specify/memory/agents.md` — Command Registry (REQUIRED)

This is the **single source of truth** for all project-specific commands and file organization.

### Required Sections

#### Section I: Context Loading Priority
Define which files agents should load and in what order. Projects provide the canonical document locations here; Bubbles agents choose a role-focused subset using the loading profiles in [agent-common.md](agent-common.md) rather than loading every governance file for every role.

#### Section II: Design Document References
Table mapping document names to file paths.

#### Section III: Verification Commands
**This is the most critical section.** Bubbles agents resolve ALL commands from here.

```markdown
### CLI Entrypoint
CLI_ENTRYPOINT=<project-cli-wrapper>

### Build/Test/Lint Commands (ALL REQUIRED)
BUILD_COMMAND=<full build command>
CHECK_COMMAND=<fast compile check command>
LINT_COMMAND=<lint command>
FORMAT_COMMAND=<format command>
UNIT_TEST_RUST_COMMAND=<backend unit test command>    # or language-appropriate name
UNIT_TEST_WEB_COMMAND=<frontend unit test command>    # if project has frontend
INTEGRATION_AND_E2E_API_COMMAND=<integration + e2e api test command>
E2E_UI_COMMAND=<e2e ui test command using browser automation framework>
DEV_ALL_COMMAND=<start full dev stack command>
DEV_ALL_SYNTH_COMMAND=<start full dev stack with synthetic data>
DOWN_COMMAND=<stop all services command>
STATUS_COMMAND=<check service status command>
```

**Rules:**
- ALL commands MUST be provided — no optional commands
- Commands MUST be the repo-standard way to execute that operation
- Commands MUST NOT require local toolchain installation (use Docker/containers)
- If a category does not apply (e.g., no frontend), state `N/A - no frontend in this project`

#### Section IV: Code Patterns
File organization table, naming conventions, required practices.

#### Section V-X: Error Resolution, Quality Standards, Escalation, Quick Reference, Quality Gates, Sources of Truth

---

## `.github/copilot-instructions.md` — Project Policies (REQUIRED)

This file provides **project-specific** rules that extend (not duplicate) the universal governance in `agent-common.md`.

### What MUST Be In This File (Project-Specific Only)

| Section | Content |
|---------|---------|
| **Testing Requirements** | Project-specific test type table with commands, coverage targets, required/optional flags |
| **Commands** | Project-specific command table (mirrors agents.md but in context of copilot instructions) |
| **Docker Config** | Container names, image names, static roots, bundler, port allocation |
| **Pre-Completion Audit** | Project-specific verification commands |
| **Language Discipline** | Hot/warm/cold/async path language assignments |
| **Framework-Specific Rules** | Rhai Script exposure, UI support requirements, simulation data rules |
| **UI Stack Configuration** | (If using UI/Designer agents) Framework, routing, styling, and animation libraries |
| **Prohibited/Required Patterns** | Language-specific code patterns |

### What MUST NOT Be In This File (Governance — Already in agent-common.md)

Do NOT duplicate these — they are universal governance in `agent-common.md`:

| Already Covered By | Topics |
|--------------------|--------|
| `agent-common.md` → Sequential Spec Completion Policy | Sequential completion rules, DoD completion gates |
| `agent-common.md` → Anti-Fabrication Policy | Fabrication detection heuristics, evidence standards |
| `agent-common.md` → Execution Evidence Standard | Valid/invalid evidence, self-check, evidence format |
| `agent-common.md` → Specialist Completion Chain | Required specialist agents, promotion blocks |
| `agent-common.md` → Quality Work Standards | Real vs fake work definitions |
| `agent-common.md` → Test Type Integrity Gate | Test classification rules |
| `agent-common.md` → E2E Anti-False-Positive Guardrails | Forbidden test patterns |
| `scope-workflow.md` → Phase Exit Gates | Phase completion requirements |
| `scope-workflow.md` → DoD Templates | Mandatory DoD items |
| `scope-workflow.md` → Status Ceiling Enforcement | Status transition rules |

**Rationale:** Duplicating governance creates drift. When the rule exists in `agent-common.md`, agents already follow it. Repeating it in `copilot-instructions.md` risks version divergence.

---

## `.github/bubbles-project.yaml` — Project-Owned Framework Extensions (OPTIONAL)

This file allows projects to extend or override selected framework behavior without patching framework-managed files. The file is **project-owned** and never overwritten by Bubbles upgrades.

### Supported Configuration Sections

```yaml
# .github/bubbles-project.yaml — Project-specific Bubbles extensions

# Custom quality gates (G100+ range, auto-assigned IDs)
gates:
  license-compliance:
    script: scripts/license-check.sh
    blocking: true
    description: Verify all dependencies have approved licenses

# Scan pattern overrides for built-in gates
scans:
  # G047: IDOR / Auth Bypass Detection
  idor:
    # Regex patterns for identity fields extracted from request body
    # (overrides generic defaults when provided)
    bodyIdentityPatterns:
        - 'body\.user_id\|body\.owner_id\|body\.org_id'
        - 'req\.body\.userId\|req\.body\.ownerId'
    # Regex pattern for correct auth context usage
    authContextPatterns: 'claims\.\|auth_user\|CurrentUser\|FromRequest'
    # Regex pattern to identify handler/controller files
    handlerFilePatterns: 'handler|controller|route|api'

  # G048: Silent Decode Failure Detection
  silentDecode:
    # Regex patterns for silent decode anti-patterns
    patterns:
        - 'if let Ok.*decode\|filter_map.*\.ok()'
        - 'proto\.Unmarshal.*_\b'
    # Regex pattern for acceptable error handling nearby
    errorHandling: 'log::error\|tracing::error\|return Err\('

  # G051: Test Environment Dependency Detection
  testEnvDependency:
    # Additional regex patterns for env-dependent test failures
    # (appended to generic defaults, not replacing them)
    patterns: 'TRUSTED_PROXY_COUNT\|MY_CUSTOM_ENV_VAR'

  # Regression quality guard: bailout + adversarial heuristics
  regressionQuality:
    # Regex patterns that indicate a required test bails out instead of failing
    bailoutPatterns:
        - 'if.*includes\(.*login.*\).*return;'
        - 'if.*!has.*return;'
    # Regex patterns for optional assertions that do not prove required behavior
    optionalAssertionPatterns:
        - 'if \(.*layout.*\)'
        - 'toBeDefined\(\)'
    # Regex patterns that count as adversarial bug-fix regression signals
    adversarialSignals:
        - '\\.not\\.'
        - '\\bfalse\\b'
        - '\\bmissing\\b'

# Managed-doc registry overrides
docsRegistryOverrides:
  managedDocs:
    testing:
      path: docs/Test_Architecture.md
      requiredSections:
        - Executive Summary
        - Current Test Inventory
    operations:
      path: docs/Troubleshooting.md
  classification:
    featureRoot: specs
    opsRoot: specs/_ops
```

### Design Principles

| Principle | Description |
|-----------|-------------|
| **Override, not replace** | If a project provides patterns, they replace the generic defaults for that scan |
| **Append for env deps** | `testEnvDependency.patterns` is appended to generic defaults (both match) |
| **Never overwritten** | `install.sh` upgrades never touch `bubbles-project.yaml` |
| **Optional** | If the file does not exist, all scans use sensible generic defaults |
| **YAML structure** | Simple `key: value` or `key: [list]` format parseable by `sed`/`awk` in bash scripts |

`regressionQuality.*` follows the standard override model: if provided, those lists replace the generic fallback patterns used by `regression-quality-guard.sh`.

`docsRegistryOverrides.*` follows the same ownership model: framework defaults remain in `bubbles/docs-registry.yaml`, while projects can override managed doc entries or classification values from `.github/bubbles-project.yaml`.

---

## How Bubbles Agents Resolve Project-Specific Values (Indirection Rules)

### Rule 1: Command Resolution
```
Agent needs to run tests → reads `.specify/memory/agents.md` → finds UNIT_TEST_RUST_COMMAND → executes that command
```

Bubbles agents MUST NEVER hardcode ecosystem-native commands. They resolve commands from `agents.md`.

### Rule 2: Docker Configuration Resolution
```
Agent needs frontend container name → reads `.github/copilot-instructions.md` → finds Docker Bundle Freshness Configuration table
```

Bubbles agents use placeholders (`<frontend-container>`, `<static-root>`) in their definitions. When executing, they resolve these from the project's `copilot-instructions.md`.

### Rule 3: Policy Resolution (Priority Order)
```
1. `.specify/memory/constitution.md` — Highest authority (project governance)
2. `.github/copilot-instructions.md` — Project-specific rules
3. `agents/bubbles_shared/agent-common.md` — Universal agent governance
4. `agents/bubbles_shared/scope-workflow.md` — Universal workflow governance
5. `bubbles/workflows.yaml` — Workflow orchestration config
```

When policies conflict, higher-priority files win.

### Rule 4: Test Type Resolution
```
Agent needs to determine required test types for a scope change
→ reads agent-common.md "Test Type Mapping" table (universal rules)
→ reads copilot-instructions.md "Testing Requirements" table (project commands)
→ combines: knows WHICH tests to run (universal) + HOW to run them (project-specific)
```

---

## Portability Checklist (When Adding Bubbles to a New Project)

When adopting Bubbles for a new project, populate these files:

- [ ] `.specify/memory/agents.md` — Fill in CLI entrypoint, all commands, file organization, naming
- [ ] `.specify/memory/constitution.md` — Adapt principles for your project's domain (keep universal ones, add domain-specific ones)
- [ ] `.github/copilot-instructions.md` — Fill in testing requirements, Docker config, language rules, framework-specific rules
- [ ] `config/<project>.yaml` — Create the SST config file with ports, services, infrastructure, environments
- [ ] `scripts/commands/config.sh` (or equivalent) — Create the config generator that parses SST and writes derived files
- [ ] `.github/copilot-instructions.md` → SST section — Document generated files, per-language enforcement, port allocation
- [ ] `bubbles/workflows.yaml` — Copy as-is (project-agnostic) or customize modes
- [ ] `agents/` — Copy all `bubbles.*.agent.md` files as-is (project-agnostic)
- [ ] `agents/bubbles_shared/` — Copy all shared files as-is (project-agnostic)
- [ ] `bubbles/scripts/` — Copy governance scripts as-is (project-agnostic)

**The Bubbles agent files (`agents/`) and shared governance (`agents/bubbles_shared/`) MUST NOT be modified per-project.** They are universal. Only the three configuration files listed above are project-specific.

---

## Portable vs Project-Specific — Complete Inventory

### Portable (Copy Unchanged Across Projects)

| Path | Content | Why Portable |
|------|---------|--------------|
| `agents/bubbles.*.agent.md` | All `bubbles.*` agent definitions | Contain zero project-specific commands/paths/tools |
| `agents/speckit.*.agent.md` | All `speckit.*` agent definitions | Specification-focused, project-agnostic |
| `agents/bubbles_shared/agent-common.md` | Top-level governance index and compatibility reference | Routes agents to smaller authoritative modules |
| `agents/bubbles_shared/artifact-lifecycle.md` | Artifact structure and lifecycle rules | Single source of required artifact and scope lifecycle rules |
| `agents/bubbles_shared/artifact-ownership.md` | Canonical artifact ownership and routing contract | Single source of ownership boundaries |
| `agents/bubbles_shared/completion-governance.md` | Completion hierarchy and completion-state rules | Single source of DoD → scope → spec completion rules |
| `agents/bubbles_shared/execution-ops.md` | Retry, timeout, and auxiliary execution ops | Single source of bounded execution-ops rules |
| `agents/bubbles_shared/operating-baseline.md` | Shared loading/loop/indirection baseline | Single source of operating behavior |
| `agents/bubbles_shared/planning-core.md` | Planning-specific shared rules | Small planning-time core |
| `agents/bubbles_shared/execution-core.md` | Implementation/orchestration shared rules | Small execution-time core |
| `agents/bubbles_shared/test-core.md` | Testing shared rules | Small test-time core |
| `agents/bubbles_shared/audit-core.md` | Audit/validation shared rules | Small audit-time core |
| `agents/bubbles_shared/validation-core.md` | Shared completion-validation baseline | Single source of validation model |
| `agents/bubbles_shared/validation-profiles.md` | Agent-specific Tier 2 validation checks | Single source of role-specific validation tables |
| `agents/bubbles_shared/plan-bootstrap.md` | Minimal planning bootstrap | Small mandatory planning load |
| `agents/bubbles_shared/implement-bootstrap.md` | Minimal implementation bootstrap | Small mandatory execution load |
| `agents/bubbles_shared/test-bootstrap.md` | Minimal testing bootstrap | Small mandatory test load |
| `agents/bubbles_shared/audit-bootstrap.md` | Minimal audit/validation bootstrap | Small mandatory audit load |
| `agents/bubbles_shared/analysis-bootstrap.md` | Minimal analyst bootstrap | Small mandatory analysis load |
| `agents/bubbles_shared/design-bootstrap.md` | Minimal design bootstrap | Small mandatory design load |
| `agents/bubbles_shared/docs-bootstrap.md` | Minimal documentation bootstrap | Small mandatory docs load |
| `agents/bubbles_shared/clarify-bootstrap.md` | Minimal clarification bootstrap | Small mandatory clarify load |
| `agents/bubbles_shared/ux-bootstrap.md` | Minimal UX bootstrap | Small mandatory UX load |
| `agents/bubbles_shared/state-gates.md` | Completion, loop, and state-integrity rules | Compact gate reference |
| `agents/bubbles_shared/test-fidelity.md` | Planned-behavior and use-case testing rules | Reusable policy module |
| `agents/bubbles_shared/consumer-trace.md` | Rename/removal dependency-chain rules | Reusable policy module |
| `agents/bubbles_shared/e2e-regression.md` | Persistent E2E regression rules | Reusable policy module |
| `agents/bubbles_shared/evidence-rules.md` | Execution evidence and anti-fabrication rules | Reusable policy module |
| `agents/bubbles_shared/critical-requirements.md` | Top-priority non-negotiable policy set (no fabrication, no stubs/TODOs/fallbacks/defaults, full implementation and validation) | Project-agnostic governance, no project-specific values |
| `agents/bubbles_shared/quality-gates.md` | Test taxonomy, evidence, and anti-fabrication gates | Single source of quality/completion gate rules |
| `agents/bubbles_shared/scope-workflow.md` | Universal workflow: DoD templates, phase exit gates, artifact templates, status ceiling, state.json canonical schema | Uses `[cmd]` placeholders |
| `agents/bubbles_shared/scope-templates.md` | Artifact templates and examples | On-demand template reference |
| `agents/bubbles_shared/project-config-contract.md` | This file — the cross-project interface contract | Describes the interface, not the implementation |
| `agents/bubbles_shared/feature-templates.md` | Feature artifact templates | Structure-only, no project references |
| `agents/bubbles_shared/bug-templates.md` | Bug artifact templates | Structure-only, no project references |
| `agents/bubbles_shared/docker-lifecycle-governance.md` | Docker lifecycle governance (freshness, cleanup, labeling) | Universal Docker patterns, no project references |
| `bubbles/agent-ownership.yaml` | Canonical artifact ownership map for specialist delegation | Portable governance data |
| `bubbles/workflows.yaml` | Workflow modes, gates, phases, retry policy, priority scoring | Orchestration config, no project references |
| `bubbles/scripts/*.sh` | Governance scripts (artifact lint, done-spec audit, state transition guard, implementation reality scan, agent ownership lint, etc.) | Validate artifact structure, not project-specific content |
| `instructions/bubbles-*.instructions.md` | Namespaced portable instruction files | Clearly Bubbles-owned while remaining project-agnostic |
| `skills/bubbles-*/SKILL.md` | Namespaced portable governance skills (includes `bubbles-config-sst/`) | Clearly Bubbles-owned while remaining project-agnostic |
| `docs/*.md` | Bubbles documentation (examples, cheatsheet, sessions, prompts, etc.) | Project-agnostic reference docs |
| `prompts/bubbles.*.prompt.md` | Prompt shims routing to agents | Minimal routing files, no project content |

### Project-Specific (Customize Per Project)

| Path | Content | What to Customize |
|------|---------|-------------------|
| `.specify/memory/agents.md` | Command registry, file organization, naming, tech stack | ALL sections — this is the project's operational manual |
| `.specify/memory/constitution.md` | Governance principles | Add domain-specific principles (e.g., Rhai, simulation data) on top of universal ones |
| `.github/copilot-instructions.md` | Project policies, Docker config, language rules, testing commands | ALL project-specific sections; reference `agent-common.md` for governance |
| `.github/skills/<project-skill>/SKILL.md` | Domain-specific skills (e.g., chaos-execution, protobuf-only) | Fully project-specific — create per project needs |
| `.github/instructions/<project>.instructions.md` | Project-specific instruction files (e.g., ui-design, docker-ports) | Fully project-specific — create per project needs |
| `.github/agents/push.agent.md` (if exists) | Project-specific push workflow | Project-specific — NOT portable across repos |

### Agent Classification

**All `bubbles.*.agent.md` files are PORTABLE** — they contain zero project-specific content. Copy unchanged to any project adopting Bubbles.

**All `speckit.*.agent.md` files are PORTABLE** — specification-focused agents with no project dependencies.

**`push.agent.md` is PROJECT-SPECIFIC** — it references project-specific pre-push validation and CLI commands. Each project should create its own push agent or document its push workflow in `copilot-instructions.md`.

### Skills Classification

Skills in `.github/skills/` may be either portable or project-specific. Use this classification:

| Classification | Rule | Examples |
|---------------|------|---------|
| **Portable** | Uses `agents.md` indirection for commands; no project-specific paths/tools hardcoded | `bubbles-skill-authoring/`, `bubbles-docker-lifecycle-governance/`, `bubbles-docker-port-standards/`, `bubbles-spec-template-bdd/`, `bug-fix-testing/` |
| **Project-specific** | References project CLI, project-specific services, or domain-specific patterns | `project-operations/`, `serialization-policy/`, `live-system-chaos/`, `frontend-ui/`, `prepush-validation/` |

Project-specific skills should remain in `.github/skills/` (co-located with governance) but MUST NOT be assumed to exist in other repos adopting Bubbles.

---

## Adopting Bubbles in a New Project — `copilot-instructions.md` Update Guide

When adding Bubbles to a new project, the `.github/copilot-instructions.md` file needs project-specific sections that integrate with Bubbles' mechanical enforcement. This section tells you exactly what to add.

### Required Sections to Add/Update

#### 1. State Transition Guard Reference

Add the guard script to the project's pre-completion self-audit and status transition rules:

```markdown
### Pre-Completion Self-Audit (Project-Specific)

Before marking any task "done", execute these checks:

# 0. RUN STATE TRANSITION GUARD (FIRST — MANDATORY Gate G023)
bash bubbles/scripts/state-transition-guard.sh specs/<NNN-feature-name>
# If exit code 1 → STOP. You are NOT done. Fix ALL failures.

# ... project-specific build/test/lint commands ...

# N. Run artifact lint
bash bubbles/scripts/artifact-lint.sh specs/<NNN-feature-name>
```

#### 2. Status Transition Rules Table

The status transition table MUST include Gate G023 as the FIRST requirement:

```markdown
| Gate | Requirement |
|------|-------------|
| ✅ **State Transition Guard (Gate G023)** | `bash bubbles/scripts/state-transition-guard.sh specs/<NNN>` exits 0. |
| ✅ **Status Ceiling** | Workflow mode's statusCeiling allows "done". |
| ✅ **All Scopes Done** | ALL scope statuses are "Done" (zero "Not Started"). |
| ... | (remaining gates from agent-common.md) |
```

#### 3. Anti-Fabrication Self-Audit Checklist

Add these items to the project's anti-fabrication checklist:

```markdown
[ ] State transition guard script passes → exits 0
[ ] ALL scope statuses in scopes.md or scopes/*/scope.md are "Done"
[ ] ALL DoD items in scopes.md or scopes/*/scope.md are checked [x]
[ ] Artifact lint passes → exits 0
[ ] execution.completedPhaseClaims / certification.certifiedCompletedPhases in state.json include ALL mode-required phases
[ ] certification.completedScopes in state.json matches scopes marked Done
```

#### 4. Docker Bundle Freshness Configuration (if project has frontend)

Provide project-specific container names, image names, and static roots:

```markdown
### Docker Bundle Freshness Configuration (UI Scopes)

| Key | Value |
|-----|-------|
| Frontend container | `<your-frontend-container-name>` |
| Frontend image | `<your-frontend-image>` |
| Static root | `<path-inside-container>` |
| Stop command | `<your-stop-command>` |
| Build (no-cache) | `<your-build-command> --no-cache` |
| Start command | `<your-start-command>` |
| Bundler | `Vite` / `webpack` / etc |
```

#### 5. Testing Requirements Table

Provide project-specific test commands mapped to the Canonical Test Taxonomy:

```markdown
| Test Type | Category | Command | Required? | Live System |
|-----------|----------|---------|-----------|-------------|
| Unit | `unit` | `<your-unit-test-cmd>` | ✅ Always | No |
| Integration | `integration` | `<your-integration-cmd>` | ✅ Always | Yes |
| E2E API | `e2e-api` | `<your-e2e-api-cmd>` | ✅ Always | Yes |
| E2E UI | `e2e-ui` | `<your-e2e-ui-cmd>` | ✅ If UI | Yes |
```

### What NOT to Duplicate

Do NOT copy governance rules from `agent-common.md` into your `copilot-instructions.md`. Instead, reference them:

```markdown
> **Authoritative source:** [`agent-common.md` → Anti-Fabrication Policy](agents/bubbles_shared/agent-common.md)
```

The following are defined in `agent-common.md` and should NOT be restated:
- Sequential spec completion rules (Gate G019)
- Fabrication detection heuristics (Gate G021)
- Specialist completion chain (Gate G022)
- State transition guard requirement (Gate G023)
- Evidence standards, quality work standards
- Test type integrity rules

### Verification After Setup

After updating `copilot-instructions.md`, verify the integration:

```bash
# 1. Guard script exists and is executable
ls -la bubbles/scripts/state-transition-guard.sh

# 2. Artifact lint exists and is executable
ls -la bubbles/scripts/artifact-lint.sh

# 3. Done-spec audit works
bash bubbles/scripts/done-spec-audit.sh

# 4. Guard script runs against a spec (should show checks)
bash bubbles/scripts/state-transition-guard.sh specs/<any-spec>
```

---

## Skills & Instructions — Portability Rules

### Instruction Files (`.github/instructions/`)

| Type | Portable? | Rule |
|------|-----------|------|
| `bubbles-agents.instructions.md` | **YES** — copy unchanged | Contains universal agent authoring guidelines. Uses `CLI_ENTRYPOINT from agents.md` indirection. |
| `bubbles-skills.instructions.md` | **YES** — copy unchanged | Contains universal skill authoring guidelines. References `agent-common.md` for policies. |
| Project-specific instructions (e.g., `ui-design.instructions.md`, `docker-ports.instructions.md`) | **NO** — project-specific | Create per project. These contain project-specific patterns, tools, and conventions. |

**When creating project-specific instruction files:**
- Reference universal governance from `agent-common.md` — do NOT duplicate
- Reference project commands from `agents.md` — do NOT hardcode
- Scope the file to specific file patterns using glob frontmatter where applicable

### Skill Files (`.github/skills/`)

Skills can be **either** portable or project-specific:

| Skill Type | Portable? | Examples |
|------------|-----------|---------|
| **Governance skills** (enforce universal rules) | **YES** — copy unchanged | `bubbles-skill-authoring/` (meta-skill for creating skills) |
| **Domain skills** (project-specific workflows) | **NO** — project-specific | `protobuf-only/`, `chaos-execution/`, `build-deploy-validation/` |

**Portable skill rules:**
- MUST NOT reference project-specific commands, paths, or tools
- MUST use `agents.md` indirection for commands
- MUST reference `agent-common.md` for governance policies
- MUST use `copilot-instructions.md` placeholders for Docker config

**Project-specific skill rules:**
- MAY reference project-specific commands, paths, and tools
- MUST still enforce evidence standards from `agent-common.md`
- MUST still require execution evidence (≥10 lines raw output)
- MUST still enforce operation timeouts
- MUST NOT break universal governance (anti-fabrication, sequential completion, etc.)

### Governance Enforcement in Skills (MANDATORY)

All skills — portable or project-specific — MUST enforce these universal policies from `agent-common.md`:

| Policy | Gate | Enforcement |
|--------|------|-------------|
| Anti-Fabrication | G021 | Skills MUST require actual execution evidence. No summaries, no expected output. |
| Evidence Standard | G005 | Verification steps MUST capture ≥10 lines raw terminal output |
| Operation Timeouts | — | All commands MUST have explicit timeout protection |
| Sequential Completion | G019 | Skills invoked during scope work inherit the sequential completion requirement |
| Quality Work Standards | — | No stubs, no placeholders, no fake data in skill outputs |

### Governance Enforcement in Instructions (MANDATORY)

All instruction files — portable or project-specific — MUST:

| Rule | Detail |
|------|--------|
| **Reference, don't duplicate** | Point to `agent-common.md` for governance rules |
| **Use indirection for commands** | Reference `agents.md` `CLI_ENTRYPOINT`, not hardcoded tools |
| **Enforce evidence standards** | Any verification guidance must require real execution evidence |
| **No default values** | Must reference project config, not hardcode defaults |
| **No localhost/ports** | Must reference Docker config from `copilot-instructions.md` |

---

## Validation (Agents Self-Check)

When an agent cannot resolve a required project-specific value, it MUST:

1. **STOP** — do not guess or use defaults
2. **REPORT** — state exactly which value is missing and from which file
3. **INSTRUCT** — tell the user to populate the missing value in the correct file

**PROHIBITED:**
- ❌ Guessing project commands from toolchain artifacts instead of using the repo's documented command registry
- ❌ Using fallback/default commands when `agents.md` is missing
- ❌ Hardcoding project-specific values in agent definitions
- ❌ Skipping test types because no command is defined (report the gap instead)
