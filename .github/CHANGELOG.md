# Changelog

## Unreleased

### Release Manifest And Install Provenance Trust

- **Release manifest generation** — `bubbles/release-manifest.json` is now generated from the source repo and records version, git SHA, supported profiles, supported interop sources, validated surfaces, trust-doc digest, and framework-managed checksum inventory.
- **Downstream install provenance** — installs now write `.github/bubbles/.install-source.json` with install mode, symbolic source ref, source SHA, dirty-tree state, and installed version. Local-source installs never persist an absolute checkout path.
- **Trust-aware framework ops** — downstream `framework-write-guard`, `doctor`, and `upgrade --dry-run` now surface installed provenance, managed-file integrity, and dirty local-source warnings explicitly instead of treating trust state as implicit.
- **Trust canaries** — added `release-manifest-selftest.sh`, `install-provenance-selftest.sh`, and `trust-doctor-selftest.sh`, and wired them into `framework-validate` plus `release-check`.

### Workflow Continuation Guardrails

- **Continuation language now preserves active workflow orchestration** — `bubbles.workflow` and `bubbles.super` treat follow-ups like `continue`, `fix all found`, `fix everything found`, `address rest`, `fix the rest`, and `resolve remaining findings` as workflow continuation, not as permission to drop into raw specialist execution.
- **Active workflow mode preservation** — continuation handling now prefers the active mode and target recovered from continuation envelopes, recent workflow outputs, workflow run-state, or spec state. Existing `stochastic-quality-sweep`, `iterate`, and `delivery-lockdown` runs stay in those modes unless the remaining work is explicitly narrowed.
- **Continuation envelope widened** — `preferredWorkflowMode` in recap/status/workflow continuation packets now accepts any valid workflow mode from `bubbles/workflows.yaml`, allowing stochastic sweeps and iterate runs to survive handoff and recovery without lossy down-conversion.
- **Stochastic sweep closeout safety** — when a stochastic sweep ends with non-terminal touched specs, workflow output must preserve a workflow-owned continuation packet instead of suggesting raw `/bubbles.implement`, `/bubbles.test`, or similar specialist follow-ups.

### Planning Alignment & Research Quality (v3.4)

- **Design Brief** — `bubbles.design` now produces a required ~30-50 line alignment checkpoint at the top of design.md: current state, target state, patterns to follow/avoid, resolved decisions, open questions. Gives reviewers 5-minute steering leverage before expensive scoping.
- **Execution Outline** — `bubbles.plan` now produces a required ~30-50 line preamble at the top of scopes.md: phase order, new types/signatures, validation checkpoints. Like C header files for the plan.
- **Phase 0.55: Objective Research Pass** — For brownfield modes (`improve-existing`, `redesign-existing`, `delivery-lockdown`, `bugfix-fastlane`, `reconcile-to-doc`), the workflow runs a two-pass research phase: (1) generate codebase questions while knowing the intent, (2) research in a fresh solution-blind context. Produces objective "current truth" instead of confirmation-biased research.
- **Horizontal plan detection** — `bubbles.plan` Phase 4 now mechanically detects horizontal scope sequences (3+ consecutive single-layer scopes) and restructures into vertical slices.
- **Slop Tax metrics** — `bubbles.retro` tracks rework: scope reopens, phase retries, post-validate reversions, design reversals, fix-on-fix chains, net forward progress score. Target: < 15%.
- **`instruction-budget-lint.sh`** — New script counting directive lines per agent prompt. Warning at 120, hard at 200. Registered as `bubbles lint-budget` CLI command.
- **Super agent v3.4 awareness** — Design Brief, Execution Outline, Phase 0.55, horizontal plan detection, Slop Tax, instruction budget lint, 18 previously undocumented CLI commands now surfaced.
- **CHEATSHEET fixes** — Added missing gates G067 (shared infrastructure blast radius) and G069 (collateral change containment).
- **WORKFLOW_MODES.md fixes** — Added missing `retro-to-simplify`, `retro-to-harden`, `retro-to-review` to Quick Reference table. Added new capability sections.

### Data-Driven Workflow Modes + Recipe Catalog (v3.3)

- **3 new workflow modes** — `retro-to-simplify`, `retro-to-harden`, `retro-to-review`: data-driven workflows that run retro hotspot analysis first to identify targets, then execute the appropriate action (simplify, harden, or code-review) on those targets
- **2 new phases** — `retro` (owner: bubbles.retro) and `code-review` (owner: bubbles.code-review) added to the phase registry in workflows.yaml
- **3 new recipes** — `retro-driven-simplify.md`, `retro-driven-harden.md`, `retro-driven-review.md` with workflow diagrams, decision tables, and related recipe links
- **Recipe Catalog** — new `docs/CATALOG.md` providing a numbered index of all 38 recipes with mode/agent mappings, category groupings, and a decision tree for choosing the right recipe
- **README updated** — added Recipe Catalog link to nav bar, updated mode count to 33
- **New Sunnyvale aliases** — `sunnyvale liquor-then-tape` (retro-to-simplify), `sunnyvale liquor-then-harden` (retro-to-harden), `sunnyvale liquor-then-look` (retro-to-review)
- **New Rickyisms** — "Liquor then tape", "Liquor then harden", "Liquor then look"
- **New vocabulary** — Data-driven workflow terms added to CHEATSHEET.md and HTML cheatsheet
- **Framework stats updated** — 30→33 workflow modes, 23→25 phases across all docs and badges
- **Super agent updated** — new intent resolution entries, workflow mode advisor row, and v3.3 awareness for data-driven modes
- **Recipes README** — new "Data-Driven Workflows (Retro → Action)" section, code-health-analysis moved there
- **iterate supportedModes** — 3 new modes added to iterate's mode pool

### Deep Code Hotspot Analysis — Retro Agent Enhancement (v3.3)

- **Bug-fix density mapping** — `bubbles.retro` now classifies commits as bug-fix vs feature and surfaces files with highest bug-fix ratio ("bug magnets" — files where >50% of commits are fixes)
- **Co-change coupling detection** — Computes a co-change matrix from git history to find files that always change together, especially cross-directory pairs revealing hidden architectural dependencies
- **Author concentration (bus factor)** — Reports single-author risk per high-churn file. Files with bus factor = 1 are knowledge silos
- **Churn trend analysis** — Compares current hotspots against prior retro data to show stabilizing, worsening, new, and resolved hotspots
- **Recommended actions** — Retro output now includes a "Recommended Actions" section with targeted follow-up commands (`/bubbles.simplify` for bug magnets, `/bubbles.code-review` for coupling, `/bubbles.harden` for worsening hotspots)
- **Focused retro modes** — New sub-commands: `hotspots` (deep hotspot-only analysis), `coupling` (co-change coupling only), `busfactor` (author concentration only). All support time-bounding: `hotspots week`, `hotspots month`
- **New Sunnyvale aliases** — `sunnyvale wheres-the-bodies` (retro hotspots), `sunnyvale whos-driving` (retro busfactor), `sunnyvale tangled-up` (retro coupling)
- **New Rickyisms** — "Where the bodies are buried" (deep hotspot analysis), "All tangled up like Christmas lights" (co-change coupling), "Somebody's gotta drive" (bus factor)
- **New Fun Mode messages** — Deep hotspot analysis, co-change coupling detected, bus factor risk, bug magnet file, hotspot stabilizing, hotspot worsening
- **Super agent v3.3 awareness** — Updated intent resolution, decision flow, and Tag Selection Matrix with hotspot-related entries
- **New recipe** — `code-health-analysis.md` — data-driven refactoring workflow using retro hotspot analysis
- **Updated recipe** — `retro.md` expanded with new commands, output descriptions, and "Acting On Findings" section
- **Docs updated** — CHEATSHEET.md, HTML cheatsheet, AGENT_MANUAL.md, recipes/README.md all updated with new capabilities

### Universal Entry Point — Workflow as Single Front Door (v3.3)

- **Phase -1: Intent Resolution** — `bubbles.workflow` now accepts ANY input (vague natural language, continuation requests, framework ops, or structured parameters). A new Phase -1 classifies input into 4 buckets (STRUCTURED, VAGUE, CONTINUE, FRAMEWORK) and delegates to the appropriate agent via `runSubagent`:
  - VAGUE → invokes `bubbles.super` for NLP intent resolution (returns RESOLUTION-ENVELOPE)
  - CONTINUE → invokes `bubbles.iterate` for work discovery (returns WORK-ENVELOPE)
  - FRAMEWORK → invokes `bubbles.super` for framework operation execution (returns FRAMEWORK-ENVELOPE)
  - STRUCTURED → skips Phase -1 entirely (existing behavior unchanged)
- **RESOLUTION-ENVELOPE** — New subagent response contract for `bubbles.super`. When invoked via `runSubagent`, super returns machine-readable `{ mode, specTargets, tags, confidence }` instead of user-facing slash commands. Direct user invocation behavior unchanged.
- **WORK-ENVELOPE** — New subagent picker contract for `bubbles.iterate`. When invoked via `runSubagent` in picker mode, iterate returns `{ spec, scope, mode, workType, priority }` without executing work. Direct user invocation behavior unchanged.
- **FRAMEWORK-ENVELOPE** — New subagent response contract for `bubbles.super` framework operations. Returns `{ operation, result, status }` for doctor, hooks, upgrade, metrics, etc.
- **Iterate NLP delegation** — `bubbles.iterate` now delegates to `bubbles.super` when free-text input cannot be resolved by iterate's own Natural Language Input Resolution table. Zero logic duplication — super is the single NLP resolver.
- **Handoff additions** — Added `bubbles.super` as handoff target for both `bubbles.workflow` (Intent Resolution, Framework Operations) and `bubbles.iterate` (Intent Resolution). Added `bubbles.iterate` as handoff target for `bubbles.workflow` (Work Discovery).
- All existing modes, phases, gates, specialist dispatch, and direct agent invocation remain unchanged.

### Docs, Prefix Rule, G068 Issue (v3.3)

- **Command prefix rule strengthened** — The `/` slash prefix rule for agent commands is now NON-NEGOTIABLE in `agent-common.md` and explicitly added to `bubbles.recap`, `bubbles.handoff`, and `bubbles.status` agents. All agents generating next-step commands or continuation options MUST use `/bubbles.*`, never `@bubbles.*`.
- **Workflow emphasized as universal entry point** across all docs:
  - CHEATSHEET.md: workflow card updated, "Starting a Job" table reordered, natural language section rewritten, new vocabulary entries ("Just tell Bubbles", "Bubbles figures it out")
  - HTML cheatsheet: workflow card, iterate card, vocabulary, Rickyisms updated, version bumped to v3.2
  - WORKFLOW_MODES.md: new "Workflow Is The Universal Entry Point" section with examples
  - AGENT_MANUAL.md: workflow promoted to "Start Here First" with usage examples
  - Recipes: new `just-tell-bubbles.md` recipe, updated `ask-the-super-first.md` with v3.2 note, updated `resume-work.md` with "continue" shortcut
- **G068 issue filed** — `docs/issues/G068-word-overlap-threshold.md` documents the false-positive matching problem where `stg_significant_words` exclusion list and 3-word overlap threshold prevent legitimate DoD items from matching their source Gherkin scenarios. Proposes lowering min word length from 4→3, reducing exclusion list, and considering percentage-based thresholds.

### Learning & Personalization (v3.2)

- **Skill Evolution Loop** — Closed-loop learning from `lessons.md`. When the same problem pattern occurs 3+ times, the framework generates a skill proposal in `.specify/memory/skill-proposals.md`. User approves, `bubbles.create-skill` scaffolds the SKILL.md. Configured in `workflows.yaml` → `skillEvolution:`.
- **Developer Profile (Observation-Driven)** — Dynamically tracks developer preferences from measurable activity: git diffs, taste decisions, workflow mode choices, post-agent code edits, scope sizing patterns. Patterns promoted to profile after ≥3 observations. Feeds `decisionPolicy` for taste-decision auto-resolution. Fresh/aging/stale/contradicted confidence tiers prevent staleness. Never auto-applied — always user-visible.
- **Activity Tracking (Measurable Only)** — Extended `metrics` with `activityTracking:` for per-agent/per-spec/per-scope metrics. Tracks only what is measurable: invocation count, phase duration, retry count, gate pass/fail rate, scope completion time, lines changed. Explicitly does NOT track dollar costs or token counts (not exposed by platform).
- **Brainstorm Mode** — New workflow mode for exploratory thinking before implementation. Runs `analyze → bootstrap → harden → finalize` with `statusCeiling: specs_hardened`. Socratic mode on by default. Outputs spec/design/scopes artifacts with zero code written. Like YC office hours for features.
- **Parallel Scope Execution** — New opt-in execution tag `parallelScopes: dag|dag-dry` for concurrent scope execution via git worktrees. DAG-independent scopes (no mutual dependencies) run in parallel, dependent scopes wait. `maxParallelScopes: 2-4`. Off by default — sequential execution remains the safe default.
- **Agent Activity Dashboard** — `bubbles.status` now shows per-agent invocation table, active execution chain visualization, and measurable activity metrics when tracking is enabled.
- Updated `bubbles.super` with v3.2 capability awareness: brainstorm mode, skill evolution, developer profile, activity tracking, parallel scopes. New CLI commands: `skill-proposals`, `profile`, `profile --stale`, `profile --clear-stale`.
- New recipes: `brainstorm-idea.md`, `parallel-scopes.md`.
- Updated HTML cheatsheet, CHEATSHEET.md, WORKFLOW_MODES.md, and recipes README with new features, Rickyisms, and TPB vocabulary.
- New Rickyisms: "Let me think about it over a couple smokes" (brainstorm), "Get two birds stoned at once" (parallel scopes), "The park knows what you like" (developer profile), "Same greasy mistake three times" (skill evolution), "Count the empties, Randy" (activity tracking).

### DevOps Execution Lane

- Added `bubbles.devops` as a new execution owner for CI/CD, build, deployment, monitoring, observability, and release automation work.
- Kept `bubbles.stabilize` diagnostic and routed operational execution through `bubbles.devops` across workflow control-plane registries and iterate/review dispatch tables.
- Added `devops-to-doc` workflow mode and inserted the `devops` phase into delivery and hardening paths that already pass through operational stabilization.
- Updated README, cheat sheets, HTML roster, workflow docs, agent manual, and recipes to reflect the new DevOps lane.

### Control Plane v3.0 — Registry-Driven Delegation, Validate-Owned Certification, and Scenario Contracts

Major architectural evolution implementing the unified control-plane design across the entire framework:

**New registries and schemas:**
- `bubbles/agent-capabilities.yaml` — Machine-readable agent class, phase ownership, artifact ownership, user-interaction permissions, and execution/certification write authority for all 33 agents.
- `bubbles/agent-ownership.yaml` v2 — Extended with `state.json` ownership (validate-owned), `scenario-manifest.json`, `lockdown-approvals.json`, `invalidation-ledger.json`, `transition-requests.json`, `rework-queue.json` ownership blocks, certified field declarations, and expanded routing rules.
- `.specify/memory/bubbles.config.json` v2 — Central execution policy registry with defaults for grill, TDD, auto-commit, lockdown, regression immutability, and validation certification. Mode overrides for `bugfix-fastlane` and `chaos-hardening`. Managed by `bubbles policy` CLI.

**New gates (G054–G064):**
- `G054 capability_delegation_gate` — Foreign-owned work must route through registered specialist.
- `G055 policy_provenance_gate` — Active modes must record value plus source provenance.
- `G056 validate_certification_gate` — Only validate may certify promotion state.
- `G057 scenario_manifest_gate` — Changed behavior must map to stable scenario IDs and live tests.
- `G058 lockdown_gate` — Locked scenarios require grill approval and invalidation.
- `G059 regression_contract_gate` — Protected regression tests cannot drift without scenario invalidation.
- `G060 scenario_tdd_gate` — Targeted failing proof required before green certification when TDD active.
- `G061 rework_packet_gate` — Route-required findings must produce structured packets.
- `G062 owner_only_remediation_gate` — Only owning planning/execution specialists may remediate owned surfaces; diagnostics and certification must route.
- `G063 concrete_result_gate` — Every agent invocation must finish with a concrete result shape rather than narrative-only findings.
- `G064 child_workflow_depth_gate` — Only orchestrators may invoke child workflows, and workflow nesting depth is bounded.

**State model v3:**
- `state.json` version 3 with `execution.*` (runtime claims) and `certification.*` (validate-owned authority) split. Top-level `status` mirrors `certification.status`.
- `policySnapshot` records effective grill/TDD/auto-commit/lockdown/regression/validation settings with provenance per run.
- `transitionRequests` and `reworkQueue` for structured specialist-to-validate routing.
- `scenario-manifest.json` template with stable `SCN-*` IDs, Gherkin hashes, linked tests, evidence refs, lockdown/regression flags.

**Guard script updates:**
- `state-transition-guard.sh` — New checks: 3A (policy provenance G055), 3B (certification state G056), 3C (scenario manifest G057), 3D (lockdown/regression G058/G059), 3E (TDD evidence G060), 3F (transition/rework closure G061), 3G (framework ownership/result contract integrity G062/G063/G064). Revert logic clears `certifiedCompletedPhases`, `completedPhaseClaims`, and legacy `completedPhases`.
- `state-transition-guard-selftest.sh` — Creates temporary docs-only fixtures to exercise the real promotion guard, including a positive path, a negative packet-field path for G063, and an illegal child-workflow caller path for G064.
- `artifact-lint.sh` — v3 schema validation with `execution`/`certification`/`policySnapshot` required fields. Backward-compatible v2 fallback. Nested array extraction for certification-scoped `completedScopes` and `certifiedCompletedPhases`.
- `spec-dashboard.sh` — Prefers `certification.status` and `certification.completedScopes` when present.
- `traceability-guard.sh` — Scenario manifest cross-check (G057/G059): verifies scope-defined Gherkin scenarios map to manifest entries with linked tests and evidence refs.
- `agent-ownership-lint.sh` — Extended to validate `agent-capabilities.yaml`, `state.json` ownership, `scenario-manifest.json` ownership, `certificationWriter`, orchestrator-only child workflows, and RESULT-ENVELOPE coverage across primary prompt surfaces.

**CLI:**
- `bubbles policy status|get|set|reset` — Manage control-plane defaults and provenance from the CLI.

**Prompt migrations (all 33 agents updated where applicable):**
- Orchestrators (workflow, iterate, bug) updated to use `execution.currentPhase`/`certification.status` split and route final closure through validate.
- Planning agents (analyst, ux, design, plan, security) updated to use v3 state template and execution-only metadata writes.
- Execution agents (implement, test, docs, chaos) record `execution.completedPhaseClaims` only; never write `certification.*`.
- Diagnostic agents (harden, gaps, regression) reference `certification.completedScopes` and `execution.completedPhaseClaims` coherence.
- `bubbles.validate` updated with validate-owned certification checks (items 2–9), policy provenance, scenario contract, and transition/rework closure.
- `bubbles.audit` references execution/certification phase records.
- `bubbles.grill` updated for `grillMode` (off/on-demand/required-on-ambiguity/required-for-lockdown).
- `bubbles.super` updated with `grillMode`/`tdd`/`backlogExport` control-plane tags.
- `bubbles.super` front-door policy is now explicit: use it for vague intent and prompt translation, but bypass it when the exact specialist or workflow mode is already known.

**Workflow mode updates:**
- Added `delivery-lockdown` — a new maximum-assurance workflow mode that repeats the full improvement and certification chain until validate can legitimately certify `done` or records an explicit blocker. Supports optional `improvementPrelude` and `improvementPreludeRounds` tags for bounded analyst/UX/design/plan refresh passes ahead of implementation rounds.
- Added `specReview: once-before-implement` — a one-shot execution tag that runs `bubbles.spec-review` before legacy implementation/improvement work so stale or redundant active specs are reconciled once instead of rediscovered every retry round. `improve-existing`, `reconcile-to-doc`, `redesign-existing`, and `delivery-lockdown` now default this hook on.
- `bugfix-fastlane` and `chaos-hardening` now force `scenario-first` TDD by default (`forceTddMode: scenario-first`).
- `chaos-hardening` now lockdown-aware with `requireProtectedRegressionContracts`.
- `grillFirst` tag deprecated in favor of `grillMode` with `inherit` default.
- New `lockdown` optional tag with values: `inherit|off|protect-existing-scenarios|require-approval`.
- `defaultPolicyBehavior` and `policyRegistry` sections added to `executionOptions`.
- G054–G064 wired into delivery enforcement, with G062/G063/G064 enforced by framework lint and promotion-time guard checks.

**Shared governance docs:**
- `feature-templates.md` — v3 state.json template, scenario-manifest.json template, policySnapshot structure.
- `scope-templates.md` — v3 state snippet, scenario contract evidence sections.
- `scope-workflow.md` — Execution/certification split, validate-owned finalize, phase recording responsibility, status ceiling examples updated.
- `completion-governance.md`, `state-gates.md`, `quality-gates.md` — Updated for v3 field names.
- `project-config-contract.md` — Anti-fabrication checklist updated for certification-owned fields.

**Skills and instructions:**
- `bubbles-agents.instructions.md` — Control Plane Requirements section added.
- `bubbles-skills.instructions.md` — Skill-level control-plane guidance.
- `bubbles-skill-authoring/SKILL.md` — References control-plane artifacts.
- `bubbles-spec-template-bdd/SKILL.md` — Stable `SCN-*` scenario contract readiness.
- `bubbles-test-integrity/SKILL.md` — Durable scenario contracts and live-test linkage.

**Docs and cheat sheets:**
- `docs/guides/CONTROL_PLANE_DESIGN.md` — Full architecture design document.
- `docs/guides/CONTROL_PLANE_ROLLOUT.md` — Phased rollout plan mapping all 11 requested changes.
- `docs/guides/CONTROL_PLANE_SCHEMAS.md` — Schema definitions for all 8 control-plane surfaces.
- `docs/CHEATSHEET.md` — Updated to 64 gates, no-hybrid control-plane law summary, and public-facing owner/executor vs diagnostic/certification taxonomy.
- `docs/its-not-rocket-appliances.html` — v3.0, now updated through the DevOps lane expansion to 33 agents, 64 gates, 29 modes, and 20 phases. Control Plane Quick Rules, public taxonomy, and Sunnyvale vocabulary updated.
- All recipes updated from `grillFirst` to `grillMode`.

**Install system:**
- `install.sh` now installs `agent-ownership.yaml` and `agent-capabilities.yaml` alongside `workflows.yaml`.
- Bootstrap scaffolds `.specify/memory/bubbles.config.json` from the Bubbles source.
- Framework manifest includes YAML registry files.

- Added `bubbles-test-integrity` portable skill — Trinity's field manual for making sure tests are real, not greasy shortcuts. Consolidates Gherkin scenario coverage, anti-mock scans, anti-false-positive scans, assertion audits, and Test Plan↔DoD parity checks into one actionable checklist. Activates on any test work.
- Added artifact-freshness reconciliation as a first-class planning rule: analyst, UX, design, and plan now reconcile stale active content and isolate superseded material instead of leaving conflicting truths active.
- Added `artifact-freshness-guard.sh` plus Gate `G052` so superseded sections are mechanically isolated from active truth, superseded scope appendices cannot keep executable status/Test Plan/DoD structure, and per-scope directory drift is blocked when `scopes/_index.md`, on-disk `scopes/NN-*`, and `state.json.scopeProgress.scopeDir` fall out of sync.
- Added explicit existing-feature redesign support: new `redesign-existing` workflow mode, new `same-lot-new-trailer` Sunnyvale alias, and updated docs/recipes for reconcile vs improve vs redesign decisions.
- Strengthened planning validation profiles to check active-requirement, active-UX, active-design, and active-scope reconciliation.
- Added machine-readable `## ROUTE-REQUIRED` orchestration blocks to `bubbles.validate` and `bubbles.audit`, then promoted `## RESULT-ENVELOPE` to the primary workflow contract and kept the legacy block as compatibility fallback during migration.
- Fixed `done-spec-audit.sh` so it resolves installed-project repo roots correctly, rejects running outside repos with `specs/`, and fails closed on suspicious zero-done-spec scans.
- Added `bubbles/scripts/agnosticity-lint.sh` and a shipped allowlist file so portable Bubbles surfaces are mechanically checked for project-specific and concrete-tool drift.
- Wired agnosticity checks into the Bubbles CLI, doctor output, generated git hooks, installer payload, and GitHub Actions.
- Generalized remaining shared prompt, workflow, and governance references that still named concrete projects or browser automation tools.

## v2.3.0 — 2026-03-23

### New Gates: G047, G048, G049, G050, G051 — Systemic Pattern Prevention

Five new gates addressing systemic vulnerability and quality patterns discovered across multiple specs:

- **G047** (`idor_auth_bypass_gate`) — BLOCKING. Detects handlers extracting user/org/tenant identity from request body instead of auth context (JWT/session/middleware). Prevents IDOR vulnerabilities (OWASP A01). Enforced by `implementation-reality-scan.sh` Scan 7 and `bubbles.security` Phase 3.2.
- **G048** (`silent_decode_failure_gate`) — BLOCKING. Detects code that silently discards deserialization/decode errors (`if let Ok()`, `filter_map(.ok())`, `unwrap_or_default()` on decode). Prevents data corruption from going undetected. Enforced by `implementation-reality-scan.sh` Scan 8.
- **G049** (`evidence_clone_detection_gate`) — BLOCKING. Detects copy-pasted evidence where ≥80% of lines are shared across different DoD items. Strengthens G021 anti-fabrication. Enforced by `state-transition-guard.sh` Check 20.
- **G050** (`gateway_route_forwarding_gate`) — BLOCKING. Ensures every backend endpoint has a corresponding gateway/proxy forwarding rule. Prevents unreachable endpoints. Strengthens G035 vertical slice gate.
- **G051** (`test_env_dependency_gate`) — BLOCKING. Detects test failures caused by missing environment variables. Prevents pre-existing env-dependent test failures from persisting. Enforced by `state-transition-guard.sh` Check 19.

### Pluggable Scan Pattern System (`.github/bubbles-project.yaml`)

All new scan patterns (G047, G048, G051) are **project-configurable** via `.github/bubbles-project.yaml`. Projects can override or extend detection patterns without modifying Bubbles core:

```yaml
scans:
  idor:
    bodyIdentityPatterns: [...]
    authContextPatterns: '...'
    handlerFilePatterns: '...'
  silentDecode:
    patterns: [...]
    errorHandling: '...'
  testEnvDependency:
    patterns: '...'
```

When no project config exists, sensible generic defaults apply across all languages.

### Updated Files

- **`workflows.yaml`** — 5 new gate definitions (project-agnostic). Added to 6 delivery modes + `inheritedRequiredGates` + phase-level gates.
- **`implementation-reality-scan.sh`** — Scan 7 (IDOR) and Scan 8 (silent decode) load patterns from `bubbles-project.yaml` with generic fallbacks. Language-specific patterns removed from core.
- **`state-transition-guard.sh`** — Check 19 (env dependency) loads extra patterns from `bubbles-project.yaml`. Check 20 (evidence similarity) added.
- **`bubbles.security.agent.md`** — Phase 3.2 and 3.6 reference mechanical scan enforcement rather than hardcoding language-specific grep patterns. OWASP mapping updated.
- **`bubbles.audit.agent.md`** — Checklist expanded with G047-G051 rows. Quick scans delegate to `implementation-reality-scan.sh`.
- **`project-config-contract.md`** — New section documenting `bubbles-project.yaml` scan extension interface.
- **`project-scan-setup.sh`** — NEW. Auto-detects project languages, auth patterns, serialization, handler dirs, and test env dependencies. Generates `bubbles-project.yaml` with project-appropriate scan patterns.
- **`cli.sh`** — New `bubbles project setup [--dry-run]` subcommand invoking `project-scan-setup.sh`.
- **`install.sh`** — Bootstrap scaffolds `bubbles-project.yaml` template. Post-bootstrap output recommends `bubbles project setup`.
- **`bubbles.setup.agent.md`** — Post-apply validation checks for `bubbles-project.yaml`. Recommends `bubbles project setup` when scan config is missing.

---

## v2.2.0 — 2026-03-23

### New Agent: `bubbles.regression` (Steve French)

Cross-feature regression guardian. Detects test baseline regressions, cross-spec interference, design contradictions, and coverage decreases after implementation or bug fixes.

- **Character:** Steve French (the mountain lion from Trailer Park Boys)
- **Signature:** *"Something's prowlin' around in the code, boys."*
- **Icon:** `steve-french-paw.svg` — a paw print with claw marks (regression scratches)
- **Phase:** `regression` — runs after `test`, before `simplify`
- **Role:** Diagnostic agent (read-only for artifacts, routes fixes to specialists)

### New Gates: G044, G045, G046

- **G044** (`regression_baseline_gate`) — Test baseline snapshot before/after implementation. Previously-passing tests must still pass. Enforced by `regression-baseline-guard.sh`.
- **G045** (`cross_spec_regression_gate`) — Tests from already-done specs re-executed after changes. Catches cross-feature interference (e.g., spec N breaking spec M).
- **G046** (`spec_conflict_detection_gate`) — Route/endpoint collisions, shared table mutations, contradictory business rules scanned across all specs before implementation.

### Post-Implementation Hardening (Mandatory)

All delivery modes now include a mandatory hardening sequence after `test`:

```
implement → test → regression → simplify → stabilize → security → docs → ...
```

**Updated modes:** `full-delivery`, `full-delivery-strict`, `bugfix-fastlane`, `feature-bootstrap`, `value-first-e2e-batch`, `chaos-hardening`, `product-to-delivery`, `harden-to-doc`, `gaps-to-doc`, `harden-gaps-to-doc`, `reconcile-to-doc`, `stabilize-to-doc`, `improve-existing`, `stochastic-quality-sweep`, `iterate`

Previously, `simplify`, `stabilize`, and `security` were only in specialized modes. Now they run on every delivery.

### New Governance Script

- `regression-baseline-guard.sh` — Mechanical enforcement for G044/G045/G046. Checks test baseline existence, cross-spec inventory, and route collision detection.

### New Recipes

- **[Regression Check](docs/recipes/regression-check.md)** — How to verify new changes didn't break existing features
- **[Post-Implementation Hardening](docs/recipes/post-impl-hardening.md)** — The mandatory hardening sequence explained

### Infrastructure

- `bubbles.regression` added to `agent-ownership.yaml` as diagnostic agent
- `regression` trigger added to `stochastic-quality-sweep` trigger pool
- `regression` fix cycle: `bootstrap → implement → test → validate → audit`
- `e2e-regression.md` expanded with cross-spec regression rules (G044-G046)
- Fun mode messages for regression events (Steve French purring, prowling)
- 29 agents, 45 gates, 18 phases total

### Character & Quote Improvements

- **system-review:** Private Dancer → **Orangie** (the goldfish who sees everything from the fishbowl)
  - New icon: `orangie-fishbowl.svg`
  - Quote: *"Orangie sees everything. He's not dead, he's just... reviewing."*
- **iterate/Jacob:** *"I'll do whatever you need, Julian."* (was: "I can help with that.")
- **ux/Lucy:** *"You can't just slap things together and call it a home, Ricky."* (was: generic)
- **bug/Cory:** *"I didn't wanna find it, but... there it is."* (was: "I found the thing that's busted.")
- **simplify/Donny:** *"Just tape it up and move on."* (was: "Have another drink, Ray!")
- **handoff/Trevor:** *"Here, take this. I gotta go."* (was: "Cory, take this to Julian.")
- **create-skill/Sam:** *"I used to be a vet, you know. I got specialties."* (was: generic)

### Complete Alias Coverage

- 36 agent aliases (every agent has at least one `sunnyvale` alias)
- 24 workflow mode aliases (every mode has a `sunnyvale` alias)
- New agent aliases: the-super, i-got-work-to-do, not-how-that-works, lets-get-organized, whats-going-on-here, parts-unknown, whole-show, nice-kitty, just-fixes, pave-your-cave, jim-needs-a-plan, used-to-be-a-vet, true, ill-do-whatever, cant-just-slap
- New mode aliases: shit-winds-coming, gut-feeling, survival-of-the-fitness, i-toad-a-so, bill-fixes-it, open-and-shut, just-watching, smokes-and-setup, keep-going, resume-the-tape, whats-the-big-idea, harden-up, we-broke-it

### Icon Improvements

- `steve-french-paw.svg` — proper 4-toe cat paw anatomy
- `lucy-mirror.svg` — simplified to single hand mirror with sparkle
- `donny-ducttape.svg` — added torn edge zigzag detail
- `orangie-fishbowl.svg` — new icon for system-review (fishbowl with fish and bubbles)

---

## v2.1.0 — 2026-03-19

### New Gate: G040 — Zero Deferral Language

Agents were writing deferral language ("deferred to future scope", "out of scope", "will address later") into DoD items and then marking specs as "done". This is now mechanically blocked.

- **Gate G040** (`zero_deferral_language_gate`) — state-transition-guard.sh Check 18 scans scope and report artifacts for deferral language patterns and BLOCKS promotion to "done" if found
- Added to `inheritedRequiredGates` (applies to ALL delivery modes)
- Deferral scan added to Tier 2 validation tables in: `bubbles.implement` (I5), `bubbles.iterate` (IT6), `bubbles.workflow` (W5), `bubbles.audit` (A7), `bubbles.harden` (H10)
- Rule 2 (Scope Cannot Be Done) and Rule 3 (Spec Cannot Be Done) in agent-common.md updated to explicitly block on deferral language
- Zero Deferral Policy expanded with FABRICATED COMPLETION declaration
- critical-requirements.md "No TODO Debt" expanded with deferral pattern list
- scope-workflow.md Status Transition Gate and Spec Completion sections updated
- 40 gates total (up from 39)

---

## v2.0.0 — 2026-03-17

Major reorganization and new features. Prefix-based file ownership, per-scope git commits, self-healing loops, framework operations agent, and more.

### Breaking Changes

- `agents/_shared/` → `agents/bubbles_shared/` (all internal references updated)
- `scripts/bubbles*.sh` → `bubbles/scripts/*.sh` (scripts consolidated under `bubbles/` folder, `bubbles-` prefix dropped)
- `scripts/bubbles.sh` → `bubbles/scripts/cli.sh` (main CLI moved)
- Generated docs moved from `.github/docs/BUBBLES_*.md` to `.github/bubbles/docs/`
- `autoCommit` changed from boolean to enum: `off|scope|dod`

### New Features

- **`bubbles.super` agent** (26th agent) — first-touch assistant for framework operations, commands, prompts, setup, upgrades, metrics, custom gates, lessons, and diagnostics
- **Self-healing loop protocol (G039)** — bounded, non-stacking fix loops with maxDepth=1, context narrowing, retry budgets
- **Atomic git commits** — `autoCommit: scope|dod` creates structured commits after validated milestones
- **Lessons-learned memory** — `.specify/memory/lessons.md` with auto-compaction at workflow start when >150 lines
- **Git hooks system** — built-in hook catalog + custom hooks, `hooks.json` registry, `bubbles hooks install/add/remove/run/status`
- **Custom gates** — project-defined quality gates via `.github/bubbles-project.yaml`, auto-discovered by state transition guard
- **Doctor command** — `bubbles doctor [--heal]` validates installation health with 11 checks and auto-fix
- **Scope DAG visualization** — `bubbles dag <spec>` outputs Mermaid dependency diagram
- **Metrics dashboard** — optional, off by default. JSONL event logging for gate failures, phase durations, agent invocations
- **Upgrade command** — `bubbles upgrade [version]` with migration, generated doc regeneration, and staleness recommendations
- **Status --explain** — `bubbles.status --explain` for narrative progress summaries
- **Spec examples gallery** — `docs/examples/` with annotated reference specs for REST API endpoints and bug fixes

### Infrastructure

- Prefix-based file ownership model: `bubbles.` prefix = Bubbles-owned (overwritten on upgrade), everything else = project-owned (never touched)
- Install.sh migration logic for pre-v2 → v2 path transitions
- `bubbles-project.yaml` for project-local extensions (custom gates) without modifying workflows.yaml
- `hooks.json` for hook registry management
- Fun mode aliases for new super agent

## v1.0.0 — 2026-03-17

Initial release. Rebranded from the Ralph agent system.

### What's Included

- **25 agents** — bubbles.workflow through bubbles.bug
- **25 prompt shims** — routing files for VS Code Copilot Chat
- **7 shared governance docs** — agent-common, scope-workflow, templates, etc.
- **9 governance scripts** — artifact lint, state transition guard, etc.
- **1 workflow config** — 23 modes, 33 gates, complete phase definitions
- **23 SVG icons** — one per agent character
- **install.sh** — one-line installer for any repo
- **Documentation** — agent manual, workflow guide, 10 recipes, visual cheatsheet
