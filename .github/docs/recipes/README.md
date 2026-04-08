# <img src="../../icons/bubbles-glasses.svg" width="28"> Recipes Index

> *"Alright boys, here's what we're gonna do."*

Each recipe solves a specific problem — the situation you're in, and exactly what to type.

Optional execution tags you can append to many workflow commands:
- `grillMode: required-on-ambiguity` to pressure-test the direction before planning or implementation starts
- `tdd: true` to force a red-green-first execution loop inside the already-approved implement/test path
- `backlogExport: tasks|issues` to make `bubbles.plan` emit copy-ready backlog outputs per scope
- `socratic: true` for bounded clarification before discovery/bootstrap
- `bubbles.clarify` is different: use it when you explicitly want ambiguity classified and routed to the owning planning agent
- `gitIsolation: true` for isolated branch/worktree setup when allowed
- `autoCommit: scope` or `autoCommit: dod` for validated milestone commits
- `maxScopeMinutes` and `maxDodMinutes` to keep scopes aggressively small
- `microFixes: true` to keep failures in narrow repair loops
- `improvementPrelude: analyze-design-plan|analyze-ux-design-plan` to make `delivery-lockdown` refresh planning before early rounds
- `improvementPreludeRounds: N` to cap how many delivery-lockdown rounds include that prelude
- `specReview: once-before-implement` to run a one-shot stale/redundant spec audit before legacy improvement or implementation work starts
- `crossModelReview: codex|terminal` to get an independent second-opinion review from a different AI model during code-review or audit phases
- `parallelScopes: dag|dag-dry` to execute DAG-independent scopes in parallel via git worktrees (off by default)
- `maxParallelScopes: 2-4` to control maximum concurrent scope executions

Baseline workflow law already requires spec/design/plan coherence, explicit Gherkin scenarios, and scenario-specific test planning before implementation starts.

Control-plane law also requires owner-only remediation and concrete result envelopes: orchestrators dispatch, owners execute, diagnostics and certification route via packets, and every invocation ends with `completed_owned`, `completed_diagnostic`, `route_required`, or `blocked`.

---

## Start Here

| Recipe | Problem → Solution |
|--------|-------------------|
| [Ask the Super First](ask-the-super-first.md) | "I don't know the right command, agent, workflow mode, or recovery step" |

> **💡 Tip:** The super is the help desk for Bubbles itself: prompts, workflow choices, troubleshooting, and framework guidance in plain English.

> **💡 Tip:** `bubbles.super` should be your natural-language front door, not a mandatory middleman. Use it when you need translation or guidance. If you already know the exact agent or mode, call it directly.

> **💡 Tip:** Install and bootstrap recipes target downstream project repos. If you are inside the Bubbles source repository, maintain the framework directly and validate with `bash bubbles/scripts/cli.sh ...` instead of rerunning `install.sh` in that checkout.

> **💡 Tip:** Repo-readiness is advisory framework guidance, not delivery certification. Use framework ops when you want to know if a repo is well-prepared for agentic work; use `bubbles.validate` when you need completion authority.

> **💡 Tip:** For release-candidate or "no loose ends" work, ask for `delivery-lockdown` rather than a one-pass sweep. It reuses the test, quality, validation, and bug workflows until certification is actually clean.

> **💡 Tip:** The newer planning improvements mostly show up as workflow behavior, not extra commands. Brownfield modes run objective research automatically, planning produces a short Design Brief and Execution Outline for steering, and `bubbles.retro` now exposes slop tax so you can see whether you are shipping craft or just rework.

## Getting Started

| Recipe | Problem → Solution |
|--------|-------------------|
| [Set Up a New Project](setup-project.md) | "I just installed Bubbles and need to get my project ready" |
| [New Feature](new-feature.md) | "I have a feature idea and need to take it from concept to shipped code" |
| [Fix a Bug](fix-a-bug.md) | "Something's broken and I need to fix it properly" |
| [Resume Work](resume-work.md) | "I was working on something yesterday, need to pick up where I left off" |

### Common How-To Patterns

| Goal | Best Entry Point |
|------|------------------|
| Explore an idea before any code | `/bubbles.workflow  mode: brainstorm for <idea>` |
| Improve a legacy feature with objective research first | `/bubbles.workflow  improve <feature>` |
| Fix a bug in brownfield code | `/bubbles.workflow  fix the <bug>` |
| Keep shipping the next most important slice | `/bubbles.workflow  continue` |
| Keep going until the whole thing is truly green | `/bubbles.workflow  <feature> mode: delivery-lockdown` |
| Review rework and churn after a run | `/bubbles.retro  week` |
| Audit framework prompt size | `bash bubbles/scripts/cli.sh lint-budget` |

> **💡 Tip:** Not sure which recipe? Ask the super first: `/bubbles.super  help me <describe what you want to do>` — the super agent will recommend the right agent, mode, and steps.

## Quality & Maintenance

| Recipe | Problem → Solution |
|--------|-------------------|
| [Choose The Right Review](choose-review-path.md) | "I know I need review, but I don't know whether it should be code-review, system-review, or a workflow" |
| [Code Review Directly](review-code-directly.md) | "I want an engineering-only review before deciding what to fix" |
| [Review A Feature Or System](system-review.md) | "I want a holistic review before deciding what to fix, streamline, or spec" |
| [Review First, Then Improve](review-then-improve.md) | "I want to assess an existing area before choosing the right improvement workflow" |
| [Quality Sweep](quality-sweep.md) | "I want to improve code quality across a feature" |
| [DevOps Work](devops-work.md) | "I need CI/CD, deployment, build, or monitoring work executed cleanly" |
| [Ops Packet Work](ops-packet-work.md) | "I need cross-cutting infra or platform work tracked outside a feature spec" |
| [Regression Check](regression-check.md) | "I need to make sure new changes didn't break existing features" |
| [Post-Implementation Hardening](post-impl-hardening.md) | "I want code cleaned up, stable, secure, and regression-free before shipping" |
| [Chaos Testing](chaos-testing.md) | "I need to break things to find weaknesses" |
| [Security Review](security-review.md) | "I need to check for security vulnerabilities" |
| [Spec Freshness Review](spec-freshness-review.md) | "I need to check if my specs are still valid before running maintenance" |
## Planning & Design

| Recipe | Problem → Solution |
|--------|-------------------|
| [Brainstorm an Idea](brainstorm-idea.md) | "I have an idea and want to explore it thoroughly before writing any code — like YC office hours" |
| [Plan Only](plan-only.md) | "I want to plan and scope a feature without implementing" |
| [Explore an Idea](explore-idea.md) | "I have a vague product idea and need to flesh it out" |
| [Reconcile Or Redesign An Existing Feature](reconcile-redesign-existing-feature.md) | "The feature exists, but the current spec/design/scopes are stale or need a major rewrite" |
| [Grill an Idea](grill-an-idea.md) | "I want hard questions before we commit to this direction" |
| [TDD First Execution](tdd-first-execution.md) | "I want the workflow to stay red-green-first instead of drifting into implementation-first" |
| [Outcome-First Specs](outcome-first-specs.md) | "I want to define what 'done' means (Intent, Success Signal, Hard Constraints) before getting into implementation details" |

## Performance & Parallelism

| Recipe | Problem → Solution |
|--------|-------------------|
| [Parallel Scope Execution](parallel-scopes.md) | "My spec has independent scopes and I want to run them concurrently via worktrees" |

## Data-Driven Workflows (Retro → Action)

| Recipe | Problem → Solution |
|--------|-------------------|
| [Code Health Analysis](code-health-analysis.md) | "Which files keep breaking? Are there hidden dependencies? What's our bus factor?" |
| [Data-Driven Simplification](retro-driven-simplify.md) | "I want to simplify code, but I don't know where to start — let the data decide" |
| [Data-Driven Hardening](retro-driven-harden.md) | "I want to harden code, but I want to focus on the areas that actually cause problems" |
| [Retro Quality Sweep](retro-quality-sweep.md) | "I want retro to pick the hotspots, then run a full cleanup-and-hardening sweep on those areas" |
| [Data-Driven Code Review](retro-driven-review.md) | "I want a code review, but I have a large codebase — let the data target the riskiest files" |

## Refactoring & Simplification

| Recipe | Problem → Solution |
|--------|-------------------|
| [Safe Shared-Infrastructure Refactor](safe-shared-infrastructure-refactor.md) | "I need to refactor a shared fixture/bootstrap surface without collateral damage" |
| [Simplify Existing Code](simplify-existing-code.md) | "This works, but it's too complicated and I want to reduce the noise safely" |

## Day-to-Day

| Recipe | Problem → Solution |
|--------|-------------------|
| [Coordinate Runtime Leases](runtime-coordination.md) | "Parallel sessions might share or collide on Docker/Compose stacks" |
| [Check Status](check-status.md) | "What's the state of my current work?" |
| [End of Day](end-of-day.md) | "I'm done for today, need to hand off context" |
| [Retrospective](retro.md) | "What's my velocity, which gates fail most, where are the hotspots?" |
| [Update Docs](update-docs.md) | "Code changed, managed docs need publishing or cleanup" |
| [Framework Ops](framework-ops.md) | "I need to manage Bubbles itself — health, framework validation, release hygiene, hooks, gates, upgrades, metrics" |
| [Structured Commits](structured-commits.md) | "I want clean, scope-by-scope git history" |
| [Custom Gates](custom-gates.md) | "I need project-specific quality checks beyond the built-in framework gates" |
| [Cross-Model Review](cross-model-review.md) | "I want a second AI opinion from a different model on my code review" |
