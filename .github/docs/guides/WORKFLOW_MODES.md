# <img src="../../icons/bubbles-glasses.svg" width="28"> Bubbles Workflow Modes

> *"Julian's got a plan. A good plan this time."*

Workflow modes define **which phases run** and **in what order** for a given piece of work.

## Goal Is The Universal Endpoint

Use `/bubbles.goal` when you have one outcome and do not want to choose the process. Goal may execute zero, one, or several workflows plus direct specialist phases:

```
/bubbles.goal  improve the booking feature to be competitive
/bubbles.goal  continue
/bubbles.goal  fix the calendar bug
```

Use `/bubbles.workflow` when you want **exactly one root mode**:

```
/bubbles.workflow  specs/042 mode: full-delivery
/bubbles.workflow  resolve the best single mode for specs/042 and run it
```

**Runner boundaries:**
- **Goal** → one outcome, any required workflows/agents
- **Workflow** → one explicit or `super`-resolved mode
- **Sprint** → several goals under one time budget
- **Super** → resolution and framework operations, no product-workflow execution
- **Domain runner** → only the mode family granted in `workflowModeGrants`

Workflow execution is default-deny. Gate G064 requires an authorized top-level runner to interpret the mode and invoke specialist phase owners directly. Workflow-running orchestrators never invoke one another as subagents.

Continuation-shaped input includes plain `continue`/`next`, but also phrases like `fix all found`, `fix the rest`, and `address the rest` after a workflow run. Those should preserve the active workflow mode whenever workflow packets, run-state, or active spec state make that mode recoverable.

## Review Is Not A Workflow Mode

`bubbles.code-review` and `bubbles.system-review` are intentionally agents, not workflow modes.

Use them when you want diagnosis, prioritization, or assessment without entering the gated delivery lifecycle.

Use workflow modes after review when you already know you want follow-through work such as planning, implementation, testing, validation, audit, or docs synchronization.

Current assessment: review still does not need its own separate workflow family, but existing-feature work now splits more clearly: use `improve-existing` for evolutionary improvements, `reconcile-to-doc` for stale state cleanup, and `product-to-delivery` (with existing impl) when requirements, UX, design, and scopes all need reconciliation before a major rewrite.

Optional execution tags apply across modes when you need more control without changing the default autonomous behavior:
- `grillMode: on-demand|required-on-ambiguity|required-for-lockdown` inserts or requires `bubbles.grill` before analysis, selection, bootstrap, or locked-behavior invalidation so weak assumptions get challenged early.
- `tdd: true` forces a red-green-first implementation loop for changed behavior after the normal planning and scenario gates are already satisfied.

## Goal Scenarios Are Not Workflow Modes

A **goal scenario** is a runtime execution plan, not a workflow mode. When an outcome is bigger than one spec and one mode — it spans repos, chains review → plan → deliver → deploy → operate, or ends in a host-mutating deploy — `bubbles.goal` (single outcome) or `bubbles.sprint` (multi outcome) compile a typed, dependency-ordered DAG whose nodes each resolve to an EXISTING mode or specialist. There is deliberately NO new `mode:` per mission; the scenario is data, not a mode.

- `bubbles.super` resolves natural language to intent only (scenario-aware RESOLUTION-ENVELOPE fields); it never compiles or runs the DAG.
- Node types (`diagnostic`, `planning`, `delivery`, `verification`, `action`, `ongoing-ops`) are routing metadata, not a new completion model — `action`/`ongoing-ops` nodes are OPS packets certified per repo by `bubbles.validate`.
- Host-mutating `action` nodes are gated by a pre-mutation approval token (the propagate pattern).
- No node may resolve to a `requiresTopLevelRuntime` fan-out mode (`iterate` / `autonomous-*` / `*-quality-sweep` / `idea-to-release-completion`) because the scenario's active top-level runner must retain phase-owner dispatch authority. `bubbles/scripts/scenario-compile-lint.sh` enforces this.

See [Cross-Repo Goal Scenario](../recipes/cross-repo-scenario.md) and the contract in [`agents/bubbles_shared/scenario-compile.md`](../../agents/bubbles_shared/scenario-compile.md).
- `backlogExport: tasks|issues` forwards backlog-ready task or issue output preferences to `bubbles.plan`.
- `socratic: true` with `socraticQuestions: <1-5>` enables a bounded clarification loop before discovery/bootstrap work.
- `gitIsolation: true` opts into isolated branch/worktree setup when repo policy allows it.
- `autoCommit: scope|dod` opts into atomic commits after fully validated milestones (`off` is default).
- `maxScopeMinutes` and `maxDodMinutes` tighten scope sizing (recommended: scope 60-120, DoD 15-45).
- `microFixes: false` is the opt-out switch if you explicitly do not want narrow repair loops.
- `specReview: once-before-implement` runs a one-shot `bubbles.spec-review` pass before legacy improvement or implementation-capable work begins. If the mode includes `analyze`, that review runs after analysis so it sees the refreshed intent. It does not repeat on retries or later rounds.
- `samples: N` applies when the selected mode has an active redteam phase. The normal default is `1`; higher bounded counts require risk or uncertainty justification. The top-level runner dispatches one actual `bubbles.redteam` invocation per sample and deterministically aggregates the schema-version-1 records. All samples are `same-runtime-correlated`; this is not cross-model execution, and Bubbles has no verified external provider/model adapter.
- `parallelScopes: dag|dag-dry` executes DAG-independent scopes in parallel via git worktrees. Off by default. `dag-dry` shows the plan without executing.
- `maxParallelScopes: 2-4` controls maximum concurrent scope executions when `parallelScopes: dag`.

### Smart Phase Routing

Workflow modes support **phase relevance evaluation** — before invoking each phase, the active authorized runner checks whether the phase is relevant to the current scope's changed surface. Irrelevant phases are skipped with a recorded justification.

**Safety guarantees:** Skipped phases are never silent. They're recorded in `executionHistory`. If artifacts change after a skip (e.g., a prior phase adds security-relevant code), the skip decision is re-evaluated and the phase is included if now relevant. Core phases (`implement`, `test`, `validate`, `docs`, `audit`, `finalize`) never skip.

### Decision Policy (Orchestrated Workflows)

When `bubbles.plan` or `bubbles.design` are invoked by the orchestrator (not directly by the user), decisions are classified as **mechanical** or **taste**:

- **Mechanical** (auto-resolved): match existing patterns, prefer completeness, prefer reversible, respect spec constraints
- **Taste** (surfaced to user): ambiguous choices below 60% confidence, security-affecting decisions, creative direction

Taste decisions are batched at phase boundaries (max 5 per phase). When `grillMode` is active, taste decisions are pressure-tested by `bubbles.grill` first. When `socratic: true`, taste decisions become the Socratic questions (within the `socraticQuestions` limit).

Baseline workflow law already requires spec/design/plan coherence, explicit Gherkin scenarios, scenario-specific test planning, and scenario-driven E2E/integration proof before implementation is allowed to proceed. The **Outcome Contract** (G070) additionally requires every spec to declare Intent, Success Signal, Hard Constraints, and Failure Condition before bootstrap completes — and validation verifies the outcome was actually achieved, not just that the process was followed. Capability-first design (G094) now applies to delivery-capable modes when proportionality triggers fire: reusable capability foundation first, concrete providers/adapters/variants second, with old specs grandfathered by `state.json.createdAt`. Those are not optional tags.

### Planning Alignment Checkpoints (v3.4)

Two short, human-reviewable alignment artifacts are now mandatory:

- **Design Brief** — A ~30-50 line section at the top of `design.md` showing current state, target state, patterns to follow, patterns to avoid, resolved decisions, and open questions. Review this for steering (5 min) instead of reading the full design doc.
- **Execution Outline** — A ~30-50 line preamble at the top of `scopes.md` showing phase order, new types/signatures being introduced, and validation checkpoints. Like C header files for the plan.

### Brownfield Research (Phase 0.55)

For brownfield modes (`improve-existing`, `redesign-existing`, `full-delivery`, `bugfix-fastlane`, `reconcile-to-doc`), the workflow now runs an **Objective Research Pass** before design:
1. Questions are generated while knowing the solution intent
2. Codebase research is done in a **fresh, solution-blind context** — it never sees the spec or ticket
3. Results are recorded as a `## Current Truth` section in design.md

This prevents confirmation bias where the model finds patterns that support its intended design instead of reporting what actually exists.

### Horizontal Plan Detection

`bubbles.plan` Phase 4 now mechanically detects horizontal scope sequences (3+ consecutive single-layer scopes like all-DB → all-service → all-API → all-UI) and restructures them into vertical slices. Horizontal plans are the #1 quality failure in AI-generated scope sequences.

### Slop Tax Tracking

`bubbles.retro` now tracks rework metrics: scope reopens, phase retries, post-validate reversions, design reversals, fix-on-fix chains, and a **net forward progress** score. Target: < 15% slop tax.

---

## Choosing a Mode

```
/bubbles.workflow  <mode-name> for <feature/bug>
```

If you don't specify a mode, `full-delivery` is the default.

**Natural language works too** — workflow routes plain-English requests through `super`, then executes the resolved mode:

```
/bubbles.workflow  improve the booking feature to be competitive
/bubbles.workflow  fix the calendar bug
/bubbles.workflow  harden specs 11 through 37
/bubbles.sprint  minutes: 120 goals: improve booking; fix calendar; validate release
```

The workflow runner does not keep its own second routing brain. `bubbles.super` owns natural-language translation and runner selection. If a request resolves to `iterate`, `autonomous-goal`, or `autonomous-sprint`, workflow returns the registered top-level owner instead of executing that excluded meta mode.

**Not sure which mode?** Ask the super: `/bubbles.super  which mode should I use for <your situation>`

## Adoption Profiles Are Not Workflow Modes

Workflow modes decide phase order. Adoption profiles decide how Bubbles explains bootstrap and readiness guidance.

- Use workflow modes to choose how work executes.
- Use adoption profiles to choose how onboarding and readiness surfaces speak to the repo.
- No profile weakens validate-owned certification, scenario contracts, or artifact ownership.

Useful profile commands:

```bash
bash bubbles/scripts/cli.sh profile show
bash bubbles/scripts/cli.sh profile set delivery
bash bubbles/scripts/cli.sh repo-readiness . --profile assured
```

---

## Full Delivery Modes

These run the complete pipeline. Use for new features.

### <img src="../../icons/julian-glass.svg" width="20"> full-delivery

**The default — maximum-assurance delivery.** The workflow loops through the full improvement and certification chain until `bubbles.validate` certifies `done` or records a documented blocker. Includes gaps, harden, chaos, and all specialist phases.

```
[repeat until certified done: optional analyze/ux/design/plan prelude → bootstrap → implement → test → regression → simplify → gaps → harden → stabilize → devops → security → validate → audit → chaos → docs] → finalize
```

**Use when:** All feature work, bug fixes (use `bugfix-fastlane` for focused bugs), release-candidate work, and legacy hardening. This is the default mode.

```
/bubbles.workflow implement action:full-delivery target:spec for 042-catalog-assistant
```

### <img src="../../icons/bubbles-glasses.svg" width="20"> value-first-e2e-batch

Prioritized delivery. Scores work items by business value and implements in priority order.

```
discover → select → bootstrap → implement → test → regression → simplify → stabilize → devops → security → docs → validate → audit → chaos → finalize
```

**Use when:** Multiple features compete for time and you want the highest-value work first.

### <img src="../../icons/bubbles-glasses.svg" width="20"> product-to-delivery

Full product discovery through delivery.

```
analyze → select → bootstrap → implement → test → regression → simplify → stabilize → devops → security → docs → validate → audit → chaos → finalize
```

**Use when:** Starting from a product idea rather than an already-shaped technical spec.

---

## Brainstorm & Exploration Modes

Explore and refine ideas without writing code.

### <img src="../../icons/ray-lawnchair.svg" width="20"> brainstorm

Like YC office hours for your feature. Explore the idea, analyze competitors, harden scenarios — zero code.

```
analyze → bootstrap → harden → finalize
```

**Use when:** "I have an idea but want to think it through before building." Outputs spec.md + design.md + scopes.md with `statusCeiling: specs_hardened`. Socratic mode is on by default (5 questions).

```
/bubbles.workflow  brainstorm for "property search engine with competitive edge"
```

---

## Fast-Track Modes

Skip phases you do not need without dropping the governance chain.

### <img src="../../icons/ricky-dynamite.svg" width="20"> bugfix-fastlane

Fast bug resolution with reproduce-before and verify-after evidence.

```
select → implement → test → regression → simplify → stabilize → devops → security → validate → audit → finalize
```

**Use when:** Bug fixes.

### <img src="../../icons/cory-trevor-smokes.svg" width="20"> feature-bootstrap

Bootstrap missing planning artifacts, then continue through implementation and the full verification chain for that feature.

```
select → bootstrap → implement → test → regression → simplify → stabilize → devops → security → docs → validate → audit → finalize
```

**Use when:** A feature is missing spec/design/scope readiness and you want the workflow to repair that planning debt before continuing delivery.

### <img src="../../icons/julian-glass.svg" width="20"> iterate

Pick the highest-priority next slice inside an existing spec and run one iteration through the right specialists.

```
[priority pick] → [auto-selected mode] → specialist phases → finalize
```

**Use when:** Picking up where you left off.

If the next executable action is ambiguous, `bubbles.iterate` may run `bubbles.code-review` or `bubbles.system-review` as a narrow diagnostic precursor, then continue into planning or execution.

---

## Quality & Hardening Modes

Focus on quality, hardening, and operational readiness.

### <img src="../../icons/conky-puppet.svg" width="20"> harden-gaps-to-doc

```
select → bootstrap → validate → harden → gaps → implement → test → regression → simplify → stabilize → devops → security → chaos → validate → audit → docs → finalize
```

**Use when:** You want a thorough post-implementation quality sweep.

### <img src="../../icons/ricky-dynamite.svg" width="20"> chaos-hardening

```
select → bootstrap → chaos → implement → test → regression → simplify → stabilize → devops → security → validate → audit → finalize
```

**Use when:** Resilience testing and fixing what breaks.

### <img src="../../icons/conky-puppet.svg" width="20"> spec-scope-hardening

```
select → bootstrap → harden → docs → validate → audit → finalize
```

**Use when:** Specs are vague and scope quality needs tightening.

### <img src="../../icons/ricky-dynamite.svg" width="20"> stochastic-quality-sweep

```
[N randomized rounds: random spec + random trigger + trigger-specific fix cycle] → docs → finalize
```

**Use when:** Periodic adversarial maintenance across the codebase.

If a sweep ends with findings and routed follow-up work, resume it through `/bubbles.workflow` using continuation-shaped language such as `fix all found` or `address the rest`. The workflow should preserve the active `stochastic-quality-sweep` continuation context instead of flattening the result into raw specialist next steps.

### <img src="../../icons/lahey-bottle.svg" width="20"> retro-quality-sweep

```
select → retro → simplify → harden → gaps → implement → test → regression → stabilize → devops → security → validate → audit → docs → finalize
```

**Use when:** You want retro to pick the hotspots, then run a deterministic quality sweep on those areas instead of random trigger rounds.

---

## Focused Modes

Do one thing well.

### Finding-Owned Closure Rule

Focused modes are still closure modes. If their starting specialist finds a legitimate bug, regression, gap, or improvement, the mode must launch the full finding-owned closure workflow before it returns terminal success upward:

- Planning workflow: `bubbles.analyst` → `bubbles.ux` when the finding touches UI or a user-visible journey → `bubbles.design` → `bubbles.plan`
- Delivery workflow: `bubbles.implement` → `bubbles.test` → `bubbles.validate` → `bubbles.audit` → `bubbles.docs` → finalize/certification

This applies to `chaos`, `test`, `simplify`, `stabilize`, `devops`, `security`, `validate`, `regression`, `harden`, `gaps`, and future trigger-style workflows. Parent workflows must wait for the mapped workflow mode's terminal result envelope instead of accepting a diagnosis-only return. If nested subagent delegation is unavailable, the active parent expands the mapped mode locally and records that execution model.

### <img src="../../icons/trinity-notebook.svg" width="20"> test-to-doc

```
test → validate → docs
```

**Use when:** Tests need running and docs need updating. If tests expose real work, this mode must execute the full finding-owned planning/remediation chain before it returns.

### <img src="../../icons/jroc-mic.svg" width="20"> docs-only

```
docs
```

**Use when:** Pure documentation work.

### <img src="../../icons/sonny-ledger.svg" width="20"> release-planning-to-doc

```
read-direction → reconcile-capabilities → produce-release-packet → update-investor-overview → docs
```

**Use when:** Producing or refreshing a phase release packet (vision, features, actions, business-plan, deployment, marketing, monetization, ops-scalability). Owned by `bubbles.releases` ("Sonny Iron Lung Smith"). Refuses to run on a repo missing the Product Direction Surfaces trio (`docs/INVESTOR_OVERVIEW.md`, `docs/Product-Principles.md`, `.github/instructions/product-principles.instructions.md`) — routes to `bubbles.setup` first.

**Subtypes:**
- `mode: bootstrap` — Fresh phase, no prior packet
- `mode: refresh` — Reconcile existing packet against current capability state
- `mode: extend` — Add new plan to existing phase's `docs/plans/<phase>/`
- `mode: cross-product` — Coordinated plan across two repos with `paired_repo: <path>`

### <img src="../../icons/sonny-ledger.svg" width="20"> idea-to-release-completion

```
analyze → releases (bootstrap-or-refresh) → select → bootstrap → implement → test → regression → simplify → stabilize → devops → security → docs → validate → audit → chaos → releases (refresh) → finalize
```

**Use when:** A user has an idea and wants the FULL lifecycle: a release packet entry created up front, the spec / design / scopes built and shipped, audit clean, AND the release packet refreshed at the end so `features.md` and the `docs/INVESTOR_OVERVIEW.md` Phase Overview correctly reflect the now-shipped capability. This is the only mode that closes the loop — the standard `product-to-delivery` mode stops at "audit clean" and leaves the release packet stale.

**Why two `releases` phases:** The first run (position 1) is in `bootstrap-or-refresh` mode — if the phase release packet does not exist yet, Sonny bootstraps it; if it does, Sonny refreshes it to add the new idea as a planned capability. The second run (position -2, just before `finalize`) is in `refresh` mode — Sonny reconciles the now-shipped capability into the packet, updates `features.md`, and updates the Phase Overview. Both runs are owned by `bubbles.releases` ("Sonny Iron Lung Smith").

**Required prerequisites:**
- Product Direction Surfaces trio MUST exist (`docs/INVESTOR_OVERVIEW.md`, `docs/Product-Principles.md`, `.github/instructions/product-principles.instructions.md`). If missing, the mode refuses to start — user must run `/bubbles.setup` first.
- `phase: <phase-id>` parameter is required so Sonny knows which release packet to bootstrap or refresh.
- `idea: <plain English>` parameter is recommended so the analyze phase has explicit input.

**Anti-fabrication guarantees:** `forbidFabricatedDeliveredClaim: true` prevents the second `releases` run from flipping the capability to `delivered` if `bubbles.audit` did not certify the work as done. The carry-forward table, inline vision restatement, and no-fabricated-principles/capabilities/competitors constraints all apply.

**Recipe:** [`docs/recipes/idea-to-release.md`](../recipes/idea-to-release.md)

### <img src="../../icons/randy-cheeseburger.svg" width="20"> validate-only

```
validate
```

**Use when:** Quick gate checks without a full workflow rerun.

### <img src="../../icons/lahey-badge.svg" width="20"> audit-only

```
audit
```

**Use when:** Compliance-only review on existing work.

### <img src="../../icons/bill-wrench.svg" width="20"> stabilize-to-doc

Diagnose operational reliability issues, route delivery remediation through DevOps when needed, then run the rest of the quality chain.

```
select → bootstrap → validate → stabilize → devops → implement → test → regression → simplify → security → chaos → validate → audit → docs → finalize
```

**Use when:** Infrastructure and operational hardening starts with diagnosis.

### <img src="../../icons/tommy-rack.svg" width="20"> devops-to-doc

Focused operational delivery for an existing feature or bug.

```
select → bootstrap → devops → test → stabilize → security → validate → audit → docs → finalize
```

**Use when:** CI/CD, build, deployment, monitoring, observability, or release automation needs direct execution work. If DevOps work exposes real product or planning changes, this mode must execute the full finding-owned planning/remediation chain before it returns.

### <img src="../../icons/jroc-cap.svg" width="20"> propagate-forward

```
select → devops → validate → audit → docs → finalize
```

**Use when:** A change on one declared release train should move forward to the next train(s). Owned by `bubbles.propagate` (J-Roc), with git execution routed to `bubbles.devops` and receiving-train validation routed per `propagation-policy.yaml`.

### <img src="../../icons/jroc-cap.svg" width="20"> propagate-backport

```
select → devops → validate → audit → docs → finalize
```

**Use when:** A prod/hotfix change must move backward to an earlier train. Requires a `backportable: true` edge and approval token when policy requires approval.

### <img src="../../icons/jroc-cap.svg" width="20"> propagate-audit

```
select → audit → docs → finalize
```

**Use when:** You need a read-only drift report showing commits present on a source train but missing from downstream trains.

### <img src="../../icons/dvs-mic.svg" width="20"> release-train-status-all

```
select → audit → finalize
```

**Use when:** You want one read-only table for every train: phase, slot, flag bundle, retention, PII, and open flag count. Also available as `/bubbles.train status --all-trains`.

### <img src="../../icons/bill-wrench.svg" width="20"> incident-fastlane

```
select → stabilize → devops → validate → audit → docs → finalize
```

**Use when:** Production is actively broken. `bubbles.stabilize` classifies severity; `incident` findings route rollback authority to `bubbles.train`; `bubbles.devops` executes; `bubbles.validate` confirms.

### <img src="../../icons/lahey-bottle.svg" width="20"> framework-health

```
select → audit → docs → finalize
```

**Use when:** You want `bubbles.retro target: framework` to analyze Bubbles' own framework-events, workflow-runs, gate trends, and capability freshness, then write a proposal under `improvements/` without auto-mutating framework files.

### <img src="../../icons/cyrus-sunglasses.svg" width="20"> improve-existing

```
analyze → select → validate → harden → gaps → implement → test → regression → simplify → stabilize → devops → security → validate → audit → chaos → docs → finalize
```

**Use when:** An existing feature needs a full improvement pass.

### <img src="../../icons/lucy-mirror.svg" width="20"> redesign-existing

```
analyze → select → bootstrap → implement → test → regression → simplify → stabilize → devops → security → docs → validate → audit → chaos → finalize
```

**Use when:** An existing feature needs major artifact reconciliation and redesign before implementation can safely proceed.

### <img src="../../icons/donny-ducttape.svg" width="20"> simplify-to-doc

```
select → simplify → test → validate → audit → docs → finalize
```

**Use when:** The main goal is to reduce complexity without inventing a new product direction. If simplification exposes legitimate new work, this mode must execute the full finding-owned planning/remediation chain before it returns.

### <img src="../../icons/gary-laser-eyes.svg" width="20"> spec-review-to-doc

```
select → spec-review → docs → finalize
```

**Use when:** You need a trust and freshness audit before maintenance work relies on existing artifacts.

### <img src="../../icons/conky-puppet.svg" width="20"> harden-to-doc

```
bootstrap → validate → harden → implement → test → regression → simplify → stabilize → devops → security → chaos → validate → audit → docs
```

**Use when:** Code has weak spots that need tightening before shipping.

### <img src="../../icons/phil-collins-baam.svg" width="20"> gaps-to-doc

```
bootstrap → validate → gaps → implement → test → regression → simplify → stabilize → devops → security → chaos → validate → audit → docs
```

**Use when:** Implementation may be incomplete or missing key edge cases.

### <img src="../../icons/ricky-dynamite.svg" width="20"> chaos-to-doc

```
chaos → validate → audit → docs
```

**Use when:** You want a chaos-started workflow. If chaos finds real bugs or regressions, this mode must execute the full finding-owned planning/remediation chain before it returns.

### <img src="../../icons/randy-cheeseburger.svg" width="20"> reconcile-to-doc

```
bootstrap → validate → implement → test → regression → simplify → stabilize → devops → security → validate → audit → chaos → docs
```

**Use when:** Implementation reality and artifact state need to be reconciled.

### <img src="../../icons/randy-cheeseburger.svg" width="20"> validate-to-doc

```
validate → audit → docs
```

**Use when:** Quick validation plus documentation sync.

---

## Resume & Discovery

### <img src="../../icons/camera-crew.svg" width="20"> resume-only

Resume from where the last session stopped.

```
(reads state.json and continues from last known position)
```

**Use when:** Picking up interrupted work.

```
/bubbles.workflow  resume
```

### <img src="../../icons/ray-lawnchair.svg" width="20"> product-discovery

Business analysis and requirements only. No design or implementation.

```
analyze → ux
```

**Use when:** Early product exploration.

---

## Mode Quick Reference

| Mode | Phases | Best For |
|------|--------|----------|
| `full-delivery` | Convergence loop: all phases repeated until certified done | All features (default) |
| `value-first-e2e-batch` | Prioritized + batched | Large backlogs |
| `product-to-delivery` | Discovery → delivery | Product ideas |
| `spec-scope-hardening` (with analyze) | Analysis only | Early exploration |
| `bugfix-fastlane` | Reproduce → fix → test → regression → gaps → harden → validate → audit (loops until certified) | Bug fixes |
| `iterate` | Implement → test loop | Continuing work |
| `harden-to-doc` | Harden → fix → test → docs | Code quality |
| `gaps-to-doc` | Gaps → fix → test → docs | Gap closure |
| `harden-gaps-to-doc` | Quality sweep | Post-implementation |
| `stabilize-to-doc` | Stability/devops → test → docs | Infrastructure |
| `simplify-to-doc` | Simplify → test → docs | Code simplification |
| `spec-review-to-doc` | Spec audit → docs | Spec freshness check |
| `chaos-hardening` | Chaos → fix | Resilience |
| `chaos-to-doc` | Chaos → validate → docs | Chaos auditing |
| `reconcile-to-doc` | Reconcile → test → docs | Stale state cleanup |
| `product-to-delivery` (with existing impl) | Reconcile → redesign → deliver | Major existing-feature rewrite |
| `improve-existing` | Analyze → harden → gaps → fix | Code improvement |
| `retro-quality-sweep` | Retro-targeted quality sweep | Hotspot-guided maintenance |
| `retro-to-simplify` | Retro hotspots → simplify worst | Data-driven simplification |
| `retro-to-harden` | Retro bug magnets → harden targets | Data-driven hardening |
| `retro-to-review` | Retro risks → code review | Data-driven review |
| `release-planning-to-doc` | Read direction → reconcile capabilities → produce 8-doc release packet → update Phase Overview → docs sync | Phase release planning, carry-forward, cross-product coordination |
| `idea-to-release-completion` | analyze → release packet bootstrap-or-refresh → bootstrap → implement → test → regression → simplify → stabilize → devops → security → docs → validate → audit → chaos → release packet refresh → finalize | End-to-end: idea to shipped capability AND an updated release packet that reflects what shipped |
| `stochastic-quality-sweep` | Random quality | Maintenance |
| `test-to-doc` | Test → docs | Test/doc focus |
| `validate-to-doc` | Validate → audit → docs | Validation + docs |
| `spec-scope-hardening` | Harden specs only | Spec quality |
| `docs-only` | Docs only | Pure docs |
| `validate-only` | Validate only | Quick gate check |
| `audit-only` | Audit only | Compliance |
| `spec-scope-hardening` (with analyze + socratic) | Explore → bootstrap → harden → finalize | Idea exploration, no code |
| `devops-to-doc` | DevOps → test → stabilize → security → validate → audit → docs → finalize | Operational delivery |
| `propagate-forward` | Select → devops → validate → audit → docs → finalize | Forward change propagation across trains |
| `propagate-backport` | Select → devops → validate → audit → docs → finalize | Approval-guarded backport across trains |
| `propagate-audit` | Select → audit → docs → finalize | Read-only propagation drift report |
| `release-train-status-all` | Select → audit → finalize | Multi-train status rollup |
| `incident-fastlane` | Stabilize → train rollback → devops → validate → docs | Production incident response |
| `framework-health` | Retro target: framework → proposal | Framework self-observation |
| `resume-only` | Resume state | Picking up work |
| `autonomous-goal` | Convergence loop: plan → implement → verify → remediate → repeat | Fully autonomous single-goal execution |
| `autonomous-sprint` | Time-bounded multi-goal: plan → execute goals → wrap-up | Multiple goals with a deadline |

---

## Autonomous Modes

Full-cycle autonomous execution without human intervention until convergence or time budget is exhausted.

### <img src="../../icons/tyrone-chain.svg" width="20"> autonomous-goal

```
[CONVERGENCE LOOP: understand → plan → execute → verify (E2E + chaos + audit) → remediate → check] → optimize → docs → finalize
```

**Use when:** You want to give a single goal and have the agent handle everything autonomously — spec creation, design, implementation, testing, E2E, chaos, validation, audit, and remediation — looping until all gates pass and zero findings remain. Max 10 convergence iterations.

**Key behaviors:**
- Never stops on fixable obstacles (searches web/docs for solutions)
- E2E + chaos MANDATORY every verify cycle
- Creates spec/design/scopes automatically if they don't exist
- Remediates ALL findings (no reporting-only)
- Solution search when stuck: project docs → web → alternative approaches

```
/bubbles.goal  Implement the security deposit hold/release feature
/bubbles.goal  Fix the broken E2E tests and make all chaos scenarios pass
```

### <img src="../../icons/erica-doublestack.svg" width="20"> autonomous-sprint

```
[SPRINT: plan goals → FOR EACH goal: time-check → autonomous-goal loop → next] → sprint report
```

**Use when:** You have multiple goals and a fixed time budget. The agent prioritizes, executes each via the convergence loop, manages the clock, dynamically reorders if time is tight, and delivers a sprint report.

**Key behaviors:**
- Checks wall clock before each goal AND each scope
- Finishes current scope completely before stopping (never leaves broken state)
- Dynamically reorders: if next goal won't fit, tries a smaller one
- Reserves 15 minutes for wrap-up/docs at end
- Produces structured sprint report (completed / in-progress / not-started)
- Resume support for interrupted sprints

```
/bubbles.sprint  minutes: 240
1. Fix the calendar sync bug
2. Add deposit hold/release feature
3. Improve page builder E2E coverage
```

---

<p align="center">
  <em>"Way she goes, boys. Way she goes."</em>
</p>
