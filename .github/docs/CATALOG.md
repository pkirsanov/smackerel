# <img src="../icons/bubbles-glasses.svg" width="28"> Bubbles Recipe Catalog

> *"Alright boys, here's the full menu."*

Every recipe solves a specific problem. Find yours, follow the steps.

> **New here?** Before picking a recipe, the [Effective Prompting](guides/EFFECTIVE_PROMPTING.md)
> guide shows how to phrase a request Bubbles can act on, and the
> [Agent Manual](guides/AGENT_MANUAL.md) maps situations to agents and modes.

---

## At A Glance

| # | Recipe | Mode/Agent | One-Liner |
|---|--------|-----------|-----------|
| 1 | [Just Tell Bubbles](recipes/just-tell-bubbles.md) | `bubbles.goal` | Describe one outcome in plain English |
| 2 | [Ask the Super First](recipes/ask-the-super-first.md) | `bubbles.super` | Get command recommendations before acting |
| 3 | [New Feature](recipes/new-feature.md) | `product-to-delivery` | Idea → shipped code |
| 4 | [Fix a Bug](recipes/fix-a-bug.md) | `bugfix-fastlane` | Reproduce → fix → verify → ship |
| 5 | [Resume Work](recipes/resume-work.md) | `resume-only` | Pick up where you left off |
| 6 | [Set Up a New Project](recipes/setup-project.md) | `bubbles.setup` | Bootstrap Bubbles in a new repo |
| 7 | [Plan Only](recipes/plan-only.md) | `spec-scope-hardening` | Plan and scope without implementing |
| 8 | [Brainstorm an Idea](recipes/brainstorm-idea.md) | `brainstorm` | Explore before building — like YC office hours |
| 9 | [Explore an Idea](recipes/explore-idea.md) | `product-discovery` | Flesh out a vague product idea |
| 10 | [Grill an Idea](recipes/grill-an-idea.md) | `bubbles.grill` | Hard questions before commitment |
| 11 | [TDD First Execution](recipes/tdd-first-execution.md) | `tdd: true` tag | Red-green-first inner loop |
| 12 | [Reconcile/Redesign](recipes/reconcile-redesign-existing-feature.md) | `redesign-existing` | Stale specs → fresh design → delivery |
| 13 | [Choose The Right Review](recipes/choose-review-path.md) | Decision tree | code-review vs system-review vs workflow |
| 14 | [Code Review Directly](recipes/review-code-directly.md) | `bubbles.code-review` | Engineering-only review, no gates |
| 15 | [Review A Feature/System](recipes/system-review.md) | `bubbles.system-review` | Holistic product/UX/runtime review |
| 16 | [Review Then Improve](recipes/review-then-improve.md) | Review → workflow | Assess first, choose follow-through |
| 17 | [Quality Sweep](recipes/quality-sweep.md) | `full-delivery` | Keep looping until green |
| 18 | [Post-Impl Hardening](recipes/post-impl-hardening.md) | Hardening sequence | Clean up before shipping |
| 19 | [Safe Shared-Infrastructure Refactor](recipes/safe-shared-infrastructure-refactor.md) | `simplify-to-doc` | Refactor shared fixtures/bootstrap safely |
| 20 | [Simplify Existing Code](recipes/simplify-existing-code.md) | `simplify-to-doc` | Reduce complexity safely |
| 21 | [Code Health Analysis](recipes/code-health-analysis.md) | `bubbles.retro hotspots` | Bug magnets, coupling, bus factor |
| 22 | [Data-Driven Simplify](recipes/retro-driven-simplify.md) | `retro-to-simplify` | Retro finds targets → simplify fixes them |
| 23 | [Data-Driven Harden](recipes/retro-driven-harden.md) | `retro-to-harden` | Retro finds targets → harden fixes them |
| 24 | [Retro Quality Sweep](recipes/retro-quality-sweep.md) | `retro-quality-sweep` | Retro finds the mess → simplify/harden sweep cleans it |
| 25 | [Data-Driven Review](recipes/retro-driven-review.md) | `retro-to-review` | Retro finds targets → review diagnoses them |
| 26 | [Retrospective](recipes/retro.md) | `bubbles.retro` | Velocity, gate health, shipping patterns |
| 27 | [Regression Check](recipes/regression-check.md) | `bubbles.regression` | Verify changes didn't break things |
| 28 | [Chaos Testing](recipes/chaos-testing.md) | `chaos-hardening` | Break things to find weaknesses |
| 29 | [Security Review](recipes/security-review.md) | `bubbles.security` | Vulnerability scanning |
| 30 | [Spec Freshness Review](recipes/spec-freshness-review.md) | `spec-review-to-doc` | Check if specs are still valid |
| 31 | [DevOps Work](recipes/devops-work.md) | `devops-to-doc` | CI/CD, deployment, monitoring |
| 32 | [Ops Packet Work](recipes/ops-packet-work.md) | OPS packets | Cross-cutting infra work |
| 33 | [Parallel Scopes](recipes/parallel-scopes.md) | `parallelScopes: dag` | Run independent scopes concurrently |
| 34 | [Cross-Model Review: Unavailable](recipes/cross-model-review.md) | Migration note | No verified external provider/model adapter is enabled |
| 35 | [Structured Commits](recipes/structured-commits.md) | `autoCommit: scope` | Clean git history |
| 36 | [Custom Gates](recipes/custom-gates.md) | CLI | Project-specific quality checks |
| 37 | [Framework Ops](recipes/framework-ops.md) | CLI / `bubbles.super` | Health, framework validation, release hygiene, hooks, gates, upgrades, metrics |
| 38 | [Coordinate Runtime Leases](recipes/runtime-coordination.md) | `bubbles runtime ...` | Shared Docker/Compose ownership and conflict recovery |
| 39 | [Check Status](recipes/check-status.md) | `bubbles.status` | Current work state |
| 40 | [End of Day](recipes/end-of-day.md) | `bubbles.handoff` | Session handoff |
| 41 | [Update Docs](recipes/update-docs.md) | `docs-only` | Publish managed docs |
| 42 | [Outcome-First Specs](recipes/outcome-first-specs.md) | `bubbles.analyst` | Define what 'done' means before how to get there |
| 43 | [Autonomous Goal](recipes/autonomous-goal.md) | `autonomous-goal` | Give a goal, agent does everything until done |
| 44 | [Autonomous Sprint](recipes/autonomous-sprint.md) | `autonomous-sprint` | Multiple goals + time budget, fully autonomous |
| 45 | [Release Planning](recipes/release-planning.md) | `bubbles.releases` | Phase release packets + cross-product coordination (Sonny "Iron Lung" Smith) |
| 46 | [Idea → Release Completion](recipes/idea-to-release.md) | `idea-to-release-completion` | Full lifecycle: idea → packet bootstrap → specs → ship → packet refresh (Sonny "Iron Lung" Smith) |
| 47 | [Framework Dogfood](recipes/framework-dogfood.md) | `framework-validate` / G085 | Source repo no-specs dogfood evidence model |
| 48 | [Validation Latency Budgets](recipes/validation-latency-budgets.md) | `validation-latency-report.sh` | Phase latency report and budget interpretation |
| 49 | [Propagate Changes](recipes/propagate-changes.md) | `bubbles.propagate` / `propagate-*` | Forward-merge, backport, or audit drift across release trains |
| 50 | [Multi-Train Status](recipes/multi-train-status.md) | `release-train-status-all` | One table for all trains: phase, slot, retention, PII, open flags |
| 51 | [Incident Response](recipes/incident-response.md) | `incident-fastlane` | Diagnose production incidents, route rollback authority to train, execute and validate |
| 52 | [Observe Production](recipes/observe-production.md) | `traceContracts.observability.endpoints` | Wire live telemetry through swappable observability adapters |
| 53 | [Framework Health](recipes/framework-health.md) | `framework-health` | Analyze Bubbles itself and emit proposal-first improvement packets |
| 54 | [Cross-Repo Goal Scenario](recipes/cross-repo-scenario.md) | `bubbles.goal` / `bubbles.sprint` | Compile an outcome (review → plan → deliver → deploy → operate, possibly cross-repo) into one approval-gated scenario DAG |
| 55 | [Adversarial Verification](recipes/adversarial-verification.md) | `bubbles.redteam` / `redteam-to-doc` / `production-adversarial-probe` | Attack a finished result to falsify "done" with counterexamples, risk-gated correlated samples, or a bounded prod chaos-monkey probe. Off by default |
| 56 | [Guided Journey](recipes/guided-journey.md) | `journey-refinement` / `bubbles.journey` | Walk the live product with the user toward a goal — verify UI/API/telemetry/data each step, capture friction, route refinements (Cathy Curtis) |
| 57 | [Live Deployment Convergence](recipes/live-deployment-convergence.md) | `bubbles.goal` | Converge a real target through connector activation, approved seed data, Playwright journeys, and live four-layer proof |

---

## By Workflow Category

### 🚀 Getting Started
1 → 2 → 6 → 3

### 🧠 Planning & Exploration
8 → 9 → 10 → 7 → 11

### 🔨 Building & Delivering
43 → 44 → 3 → 4 → 5 → 12

### 🔍 Review & Assessment
13 → 14 → 15 → 16 → 28 → 56

### 📊 Data-Driven Workflows
20 → 21 → 22 → 23 → 24 → 25

### 🛡️ Quality & Hardening
17 → 18 → 19 → 25 → 26 → 27

### ⚙️ Operations & Framework
29 → 30 → 31 → 32 → 33 → 34 → 35 → 51 → 52 → 53

### 📋 Day-to-Day
36 → 37 → 38

### 📦 Release Lifecycle
8 → 9 → 45 → 46 → 49 → 50 → 31

---

## Decision Tree: "Which Recipe Do I Need?"

```
START
  │
  ├─ Don't know where to start?
  │     → Recipe 1 (Just Tell Bubbles) or Recipe 2 (Ask the Super)
  │
  ├─ Want fully autonomous execution?
  │     ├─ Single goal?
  │     │     → Recipe 43 (Autonomous Goal)
  │     │
  │     └─ Multiple goals + time budget?
  │           → Recipe 44 (Autonomous Sprint)
  │
  ├─ New feature from scratch?
  │     → Recipe 3 (New Feature) or Recipe 43 (Autonomous Goal)
  │
  ├─ Bug to fix?
  │     → Recipe 4 (Fix a Bug) or Recipe 43 (Autonomous Goal)
  │
  ├─ Continue yesterday's work?
  │     → Recipe 5 (Resume Work)
  │
  ├─ Want to improve existing code?
  │     ├─ Know where the problems are?
  │     │     → Recipe 19 (Simplify) or Recipe 18 (Harden)
  │     │
  │     └─ Need data first?
  │           → Recipe 20 (Code Health) then Recipe 21/22/23/24
  │
  ├─ Want a code review?
  │     ├─ Know what to review?
  │     │     → Recipe 14 (Code Review Directly)
  │     │
  │     └─ Need data to target it?
  │           → Recipe 24 (Data-Driven Review)
  │
  ├─ Have a backlog to work through?
  │     → Recipe 44 (Autonomous Sprint)
  │
  ├─ Pre-release quality check?
  │     → Recipe 17 (Quality Sweep)
  │
  ├─ New phase capability — idea all the way through release packet refresh?
  │     → Recipe 46 (Idea → Release Completion)
  │
  ├─ Just author or refresh a release packet (no implementation)?
  │     → Recipe 45 (Release Planning)
  │
  └─ Something else?
        → Recipe 2 (Ask the Super)
```

---

<p align="center">
  <em>"It ain't rocket appliances. Just pick a recipe."</em>
</p>
