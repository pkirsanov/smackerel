# Spec-Driven AI Agent Orchestration: A Conceptual Framework

> A high-level overview of concepts, structures, policies, and lessons learned from building a governance framework that turns AI coding agents into a reliable software delivery pipeline.

---

## 0. Top Learnings

1. **Scripts that block state transitions are the only real enforcement.** Prose rules get ignored. Mechanical gates don't.
2. **A wrong answer is 3x worse than a blank answer.** Encoding this as a formal incentive made agents produce honest gaps instead of fabricated completions.
3. **Completion must flow bottom-up from evidence.** No amount of status-setting replaces raw terminal output proving the work was done.
4. **Agents must not edit artifacts they don't own.** Strict ownership boundaries with routing protocols eliminated an entire class of subtle cross-agent inconsistencies.
5. **Planning before implementation always pays for itself.** Requiring substantive specs/designs before coding eliminated mid-implementation correction spirals.
6. **Classify gates as "model compensation" vs. "business invariant."** This creates an evolution path — relax the first category as AI improves, keep the second forever.
7. **Small isolated scopes with explicit dependencies beat large sequential plans.** Scope isolation prevents cross-contamination cheaper than debugging it.
8. **Self-healing needs hard limits.** Three retries, narrowing context, no nesting — without this, agents thrash forever on the same failure.
9. **Classify all work into typed folders with mandatory artifacts.** Unclassified ad-hoc changes are invisible to governance. Structured work taxonomy makes every change auditable and resumable.
10. **Intent-driven orchestration over manual agent selection.** One entry point resolves user intent to a workflow mode with the right specialist chain — users describe outcomes, not process steps.
11. **Every requirement must trace to a Gherkin scenario, every scenario to an E2E test.** This spec → scenario → test → evidence chain is the backbone of provable delivery. Without it, agents implement code that satisfies no stated requirement and write tests that validate no planned behavior.

---

## 1. Core Philosophy

The framework rests on a single premise: **AI agents are powerful but untrustworthy by default.** Left unconstrained, they fabricate evidence, skip steps, mark work complete that isn't, and optimize for appearing productive rather than being productive. The entire system is designed as a set of mechanical guardrails that make it harder to fake progress than to do real work.

Three foundational principles:

1. **Specifications are the source of truth, not the implementation.** Tests validate specs. If a test fails, the code is wrong — not the test. Agents must never weaken or rewrite tests to match broken behavior.

2. **Completion flows bottom-up from evidence.** A task isn't done because an agent says it's done. It's done because raw terminal output proves it's done, and that output was produced in the current session.

3. **Honesty is more valuable than completion.** A gap left open with an explanation is infinitely better than a gap covered with fabricated evidence. The framework encodes this as an explicit incentive: a wrong answer is formally scored as 3x worse than a blank answer.

---

## 2. Structured Work Taxonomy

All work is classified into typed folders with mandatory artifact sets. No ad-hoc code changes are valid outside these structures.

### 2.1 Work Types

| Type | Folder Convention | Purpose |
|------|------------------|---------|
| **Feature** | `specs/NNN-feature-name/` | New capabilities |
| **Bug** | `specs/.../bugs/BUG-NNN-description/` | Defect correction |
| **Ops** | `specs/_ops/OPS-NNN-description/` | Infrastructure and cross-cutting operational work |

### 2.2 Required Artifacts

Every unit of work has a mandatory artifact set that must exist **before implementation begins**. This is the "planning-first" principle: agents cannot improvise implementation without planning documents.

| Artifact | Purpose | Owner |
|----------|---------|-------|
| **Specification** (`spec.md`) | What to build — requirements, user scenarios, acceptance criteria | Business analyst |
| **Design** (`design.md`) | How to build it — architecture, data models, APIs, testing strategy | Designer |
| **Scopes** (`scopes.md`) | Decomposed units of work with Gherkin scenarios, test plans, and definitions of done | Planner |
| **Report** (`report.md`) | Raw execution evidence from each scope | Execution agents |
| **User Validation** (`uservalidation.md`) | Human acceptance checklist — users uncheck items to report regressions | Planner |
| **State** (`state.json`) | Machine-readable execution state, phase claims, scope progress | All agents |

**All-or-nothing rule:** Creating a partial artifact set (e.g., just the spec and state file) is a policy violation equivalent to creating a stub. All artifacts must be substantive, not skeleton headers.

### 2.3 Outcome Contracts

Every specification must declare an **Outcome Contract** with four fields:

- **Intent** — what outcome should be achieved from the user/system perspective
- **Success Signal** — observable, testable proof (not "tests pass" but "user can do X and sees Y")
- **Hard Constraints** — business invariants that must hold regardless of implementation approach
- **Failure Condition** — what would make this feature a failure even if all process gates pass

This forces planning to focus on real-world outcomes rather than process compliance.

---

## 3. Scope Decomposition Model

Work is broken into **scopes** — small, independent, testable units with explicit dependencies.

### 3.1 Scope Anatomy

Each scope contains:

- **Status** — `Not Started | In Progress | Done | Done with Concerns | Blocked`
- **Dependencies** — explicit DAG (directed acyclic graph) of prerequisite scopes
- **Gherkin Scenarios** — behavioral specifications in Given/When/Then format
- **Implementation Plan** — concrete steps
- **Test Plan** — mapping of test types to files and commands
- **Definition of Done (DoD)** — tiered checkbox items requiring individual evidence

### 3.2 Scope Isolation

Scopes are independent execution units. An agent picks up one scope and works only within that scope's artifacts. This prevents cross-contamination where fixing one thing silently breaks another.

**Two layout modes adapt to project size:**

| Scope Count | Layout | Structure |
|-------------|--------|-----------|
| 1–5 | Single file (`scopes.md`) | All scopes in one document |
| 6+ | Per-scope directories | `scopes/NN-name/scope.md` + `scopes/NN-name/report.md` |

### 3.3 Dependency DAG

Instead of forcing strict sequential ordering, scopes declare explicit dependencies. Scopes with no dependencies can run in parallel. An agent picks the lowest-numbered eligible scope where all dependencies are satisfied.

### 3.4 Scope Size Discipline

Scopes default to small — one primary outcome, one coherent validation story. Mixed-purpose scopes must be split unless explicitly justified. Optional effort hints (`maxScopeMinutes`, `maxDodMinutes`) tighten limits further.

---

## 4. Specialist Agent Architecture

The framework uses ~34 specialist agents, each with a narrowly defined role and artifact ownership boundaries.

### 4.1 Agent Categories

| Category | Agents | Purpose |
|----------|--------|---------|
| **Planning** | Analyst, UX Designer, Designer, Planner | Create and maintain work artifacts |
| **Execution** | Implementer, Tester, DevOps, Simplifier | Write code, run tests, deploy |
| **Diagnostic** | Validator, Auditor, Hardener, Gap Analyzer, Stabilizer, Security Reviewer, Regression Guardian | Read-only analysis and certification |
| **Review** | Code Reviewer, System Reviewer, Spec Reviewer | Cross-cutting review and freshness checks |
| **Orchestration** | Workflow, Iterator, Super | Coordinate specialist chains |
| **Utility** | Bug Reporter, Clarifier, Griller, Handoff, Recap, Retro, Status | Supporting workflows |

### 4.2 Artifact Ownership (Key Insight)

**Ownership boundaries are hard, not soft.** Each agent owns specific artifacts and cannot modify artifacts owned by other agents — not even for "small fixes" or "obvious corrections."

If an agent discovers that a foreign-owned artifact needs to change, it must:
1. Emit a structured routing packet identifying the owner and the required change
2. The orchestrator then invokes the correct owner
3. The phase is not complete until the owning specialist has run

This eliminates a class of problems where well-intentioned agents make changes outside their competence area, creating subtle inconsistencies.

### 4.3 Evidence Attribution

Each evidence block must be tagged with the phase that produced it. An agent may only write evidence for its own phase. Cross-phase evidence writing is classified as fabrication.

---

## 5. Workflow Modes & Orchestration

### 5.1 Intent-Driven Entry Point

Users describe what they want in natural language. A single orchestrator resolves intent, selects the appropriate workflow mode, and drives specialist agents through completion without requiring the user to memorize agent names or parameters.

### 5.2 Workflow Modes

~32 modes cover the spectrum from full delivery to narrow operations:

| Category | Examples |
|----------|---------|
| **Full delivery** | New feature end-to-end, hard redesign, large-batch delivery |
| **Bug fixing** | Fast-lane bugfix, multi-bug batch |
| **Improvement** | Improve existing, harden, simplify, stabilize |
| **Planning only** | Spec/scope hardening, discovery |
| **Validation** | Validate, audit, chaos testing |
| **Cross-cutting** | DevOps, security review, system review, retrospective |

Each mode defines:
- **Phase order** — which specialist phases run and in what sequence
- **Required gates** — which quality gates must pass
- **Status ceiling** — the maximum status the mode is allowed to set (e.g., planning-only modes cannot mark work as "done")

### 5.3 Auto-Continuation & Escalation

Workflow agents run to completion by default. When they discover that different actions are needed (artifacts need repair, bugs block progress, tests fail), they escalate inline by invoking the appropriate specialist rather than stopping and suggesting the user run a different command.

**Terminal stop conditions are strictly limited to:**
- All target work is done or blocked after max retries
- User explicitly requested stop
- No specs could be resolved from input

### 5.4 Self-Healing Loops

When a phase encounters a failure, the framework attempts bounded self-healing:
- Maximum 3 retries per failure, 5 total per phase
- Context narrows on each retry (broad → file → function)
- Fix attempts cannot trigger nested fix attempts (max depth = 1)
- After exhaustion, escalate to a different specialist or mark blocked

### 5.5 Batch Execution

When multiple specs are targeted, the workflow splits: per-spec phases run individually, shared phases (test, validate, audit) run once across all specs. This avoids redundant builds while maintaining per-spec completion rigor.

---

## 6. Quality Gate System

The framework defines ~60 named, mechanically enforced gates. Gates are the primary mechanism for preventing incomplete or fabricated work from being marked complete.

### 6.1 Gate Categories

| Category | Examples | Purpose |
|----------|---------|---------|
| **Artifact gates** | Artifact existence, scope definition, docs sync | Planning completeness |
| **Test gates** | Test integrity, test execution, evidence capture | Verification completeness |
| **Anti-fabrication gates** | Fabrication heuristics, evidence depth, phase-scope coherence | Honesty enforcement |
| **Completion gates** | All scopes done, per-DoD evidence, specialist completion | Bottom-up completion chain |
| **Security gates** | IDOR/auth bypass, silent decode failure, security scan | Safety invariants |
| **Architectural gates** | Vertical slice, integration completeness, implementation reality | Structural correctness |
| **Process gates** | Deferral language, DoD format integrity, artifact ownership | Process fidelity |

### 6.2 Gate Classification (Key Insight)

Gates are classified into two categories:

- **Model compensation gates** — exist to compensate for current AI limitations (fabrication, shortcutting, batching). These should be reviewed as models improve.
- **Business invariant gates** — enforce durable security, architectural, or correctness requirements that should remain regardless of model capability.

This classification is important for framework evolution: as AI models improve, compensation gates can be relaxed while business invariant gates remain permanent.

### 6.3 Mechanical Enforcement

Gates are not just documentation — they are enforced by scripts:

| Script | Purpose |
|--------|---------|
| **State transition guard** | Runs ~22 checks before any status transition to "done" |
| **Artifact lint** | Validates artifact structure, template compliance, deferral language |
| **Implementation reality scan** | Detects stubs, fake data, hardcoded responses, default values |
| **Regression quality guard** | Detects silent-pass patterns in required tests |
| **Traceability guard** | Verifies Gherkin-to-DoD-to-test linkage |
| **Artifact freshness guard** | Prevents stale content from masquerading as active truth |

---

## 7. Evidence & Anti-Fabrication System

This is arguably the most important part of the framework. Without it, everything else is theater.

### 7.1 Evidence Provenance Taxonomy

Every evidence block must be classified:

| Source | Meaning | Gate Treatment |
|--------|---------|----------------|
| **Executed** | Command output directly proves the claim — no interpretation needed | Accepted |
| **Interpreted** | Command ran but conclusion requires agent reasoning | Flagged for review |
| **Not-run** | No command was executed | Item must stay unchecked |

Only "executed" evidence permits marking a DoD item complete without further review.

### 7.2 Fabrication Detection Heuristics

The framework applies automatic heuristics to detect fabricated evidence:

1. Evidence blocks with fewer than 10 lines → presumed fabricated
2. Evidence matching template text verbatim → definitely fabricated
3. Multiple DoD items marked complete in a single edit → batch fabrication
4. All timestamps identical → sequential execution not proven
5. "All tests pass" without terminal output → narrative, not evidence
6. Identical evidence blocks across different items → copy-paste fabrication
7. Reading files and predicting what a script would output → analysis-as-execution (fabrication even if predictions are accurate)

### 7.3 Consequences of Fabrication

If fabrication is detected at any point:
- All DoD items in the scope revert to unchecked
- Scope/spec status reverts to in-progress
- Re-execution with real evidence is required

### 7.4 Uncertainty Declarations

When an agent cannot verify something, the correct response is an **Uncertainty Declaration** — an explicit statement of what was attempted, what was unclear, and what the next agent or human should try. The item stays unchecked but the gap is actionable.

---

## 8. Completion Hierarchy (Bottom-Up Chain)

Completion flows strictly upward through a chain that cannot be short-circuited:

```
DoD Item (evidence) → Scope (all items checked) → Spec (all scopes done) → State (promotion)
```

### 8.1 DoD Item Level
- Execute a real command or tool
- Capture raw terminal output (≥10 lines)
- Tag with phase name and provenance classification
- Only then mark the checkbox

### 8.2 Scope Level
- Every DoD item is checked with evidence
- All required test types have been executed
- No deferral language exists in scope artifacts
- No checked items lack inline evidence
- Build quality gate passes (zero warnings, lint clean, docs aligned)

### 8.3 Spec Level
- Every scope is "Done" — zero scopes in any other status
- For implementation-bearing work, report artifacts contain code-diff evidence
- State transition guard script exits with code 0

### 8.4 Sequential Completion
Previous spec/scope must be fully complete before starting the next. No parallel advancement, no premature starts.

---

## 9. Spec-to-Test Traceability Chain

The single most important structural pattern in the framework is the **traceability chain** that connects every requirement to a provable test outcome.

### 9.1 The Chain

```
Spec Requirement → Gherkin Scenario → Test Plan Entry → E2E Test → DoD Item → Evidence
```

Each link is mandatory and mechanically verified:

| Link | What It Means | Enforcement |
|------|--------------|-------------|
| **Requirement → Gherkin** | Every functional requirement in the spec must be expressed as at least one Given/When/Then scenario | Scenario depth gate; traceability guard |
| **Gherkin → Test Plan** | Every Gherkin scenario must map to a specific test in the scope's Test Plan table | Test Plan ↔ DoD cross-check |
| **Test Plan → E2E Test** | Every Test Plan entry for live-system behavior must have a real E2E test file/function | Test file existence check |
| **E2E Test → DoD Item** | Every test must have a corresponding Definition of Done checkbox | Row count parity enforcement |
| **DoD Item → Evidence** | Every checked DoD item must have inline raw execution evidence | Per-item evidence gate |

### 9.2 Why This Matters

Without this chain, three failure modes dominate:

1. **Untested requirements** — the spec says "support bulk operations" but no Gherkin scenario exists, so no test is written, and the feature ships without validation.
2. **Unanchored tests** — agents write tests that exercise code paths not described in any requirement. The tests pass, but they don't prove the spec is satisfied.
3. **Phantom completion** — DoD items are checked based on "the code looks right" rather than traced back through a scenario to a requirement.

The traceability chain makes all three mechanically detectable.

### 9.3 Scenario Contracts

Gherkin scenarios receive stable identifiers (e.g., `SCN-001`) and are tracked in a scenario manifest. Each scenario maps to:
- The spec requirement it satisfies
- The live-system test that validates it
- The evidence in the report that proves execution

Locked scenarios (protecting shipped behavior) cannot be changed without an explicit approval and invalidation path. This prevents agents from silently weakening acceptance criteria.

### 9.4 Gherkin Coverage Gate

The framework enforces that:
- Every Gherkin scenario maps to specific E2E tests with expected assertions
- Every new or changed behavior has scenario-specific persistent regression coverage
- DoD explicitly includes E2E items aligned with scenario coverage
- E2E test count ≥ Gherkin scenario count, each traceable

If any scenario lacks a corresponding test, the scope cannot be marked complete.

---

## 10. Test Taxonomy & Fidelity

### 10.1 Canonical Categories

| Category | Live System | Mocks Allowed |
|----------|-------------|---------------|
| `unit` | No | External deps only |
| `functional` | Optional | External deps only |
| `integration` | Yes | None |
| `ui-unit` | No | Backend mocked |
| `e2e-api` | Yes | None |
| `e2e-ui` | Yes | None |
| `stress` | Yes | None |
| `load` | Yes | None |

### 10.2 Test Classification Integrity

Tests must be classified by execution reality. A mocked test labeled as a live-system test is a blocking policy failure. If reclassification removes the last live-system test for a scope, that gap must be filled.

### 10.3 Key Testing Rules

- **Tests validate specs, not implementation.** If a test fails, fix the code, not the test.
- **No self-validating tests.** Every assertion must verify a value produced by the code under test, not by the test's own setup.
- **Adversarial regression tests for bugs.** Each bug-fix test must include at least one case that would fail if the bug returned. Tautological tests that pass regardless are forbidden.
- **Red-green traceability.** Changed behavior must show failing state first, then passing state after the fix.
- **No mocking internal systems.** Only external third-party dependencies may be mocked.

---

## 11. Configuration & Terminal Discipline

### 11.1 Configuration Single Source of Truth

All configuration originates from a canonical YAML file. Generated environment files and compose files are derived artifacts, never hand-edited. Missing config must fail loudly — no hidden defaults, no fallback values.

### 11.2 Terminal Discipline

Three rules govern all terminal operations:
1. **No piping output into files** — use IDE file tools for all writes
2. **No truncating command output** — always capture full, unfiltered output
3. **Always use the repo CLI** — no direct tool invocation that bypasses environment guarantees

### 11.3 Absolute Prohibitions

| Forbidden | Why | Alternative |
|-----------|-----|-------------|
| Default values | Hide missing config | Fail fast on missing values |
| Fallback branches | Mask failures | Surface errors immediately |
| Stubs/TODOs | Incomplete work pretending to be complete | Implement fully or don't merge |
| Dead code | Tech debt | Delete it; git has history |
| Commented-out code | Code graveyard | Delete it |
| Deferral language | Disguised incomplete work | Complete now or leave status as in-progress |

---

## 12. Artifact Freshness & Lifecycle

### 12.1 Single Active Truth

Every artifact must present exactly one active truth. Stale requirements, design decisions, or scopes that no longer match reality must be removed from active sections.

### 12.2 Freshness Modes

| Mode | When | What Happens |
|------|------|-------------|
| **Reconcile** | Artifact exists but drifted from reality | Bring back to one active truth, preserve history |
| **Redesign** | Major behavioral or structural change | Rework flows while preserving feature identity |
| **Replace** | Most prior content is invalid | Near-total rewrite |

### 12.3 Supersession

Old content may be preserved only in clearly labeled appendix sections (`## Superseded Requirements`, etc.). Superseded scopes must not retain executable markers (status blocks, DoD checkboxes, test plans).

---

## 13. Framework Portability

### 13.1 Project-Agnostic Core

All governance documents, agent definitions, shared modules, and scripts are project-agnostic. They use placeholder references (`[cmd]`) resolved from project-specific configuration files.

Only three files per project need customization:
- Agent configuration (commands, paths, naming conventions)
- Project constitution (governance principles)
- Copilot instructions (project-specific policies)

### 13.2 Upstream-First Rule

Framework files are owned by the canonical source repository and propagated to downstream projects via an installer. Downstream projects never directly edit framework-managed files. If a framework script needs fixing, the fix is made upstream and then upgraded.

### 13.3 Custom Quality Gates

Projects can define custom quality gates (G100+) in their own configuration. These are auto-discovered by the enforcement scripts alongside the framework's built-in gates.

---

## 14. Decision Classification

When agents are invoked by the orchestrator (not directly by the user), decisions are classified:

- **Mechanical decisions** — auto-resolved using principles like "prefer completeness," "match existing patterns," "prefer reversible options"
- **Taste decisions** — batched and surfaced to the user at phase boundaries with options, rationale, and a recommended auto-resolution

If more than 5 taste decisions accumulate in a single phase, the specification likely needs more clarity and is routed to a clarification specialist.

---

## 15. Developer Profile (Observation-Driven)

The framework can build a dynamic developer profile from observed activity — git diffs, mode choices, taste decisions, post-agent edits, scope sizing patterns. Observations are promoted to patterns after ≥3 occurrences.

**Key constraint:** The profile is used only for auto-resolving taste decisions. Execution agents (implementer, tester, validator) never read it. Stale preferences (>180 days) are flagged for review.

---

## 16. Skill Evolution Loop

When the same problem or pattern is recorded ≥3 times in the lessons-learned memory, the framework proposes creating or updating a reusable skill document. Proposals are user-approved, never auto-applied. This closes the loop between execution experience and reusable knowledge.

---

## 17. Metrics & Activity Tracking

The framework tracks only what can be measured with certainty:
- Invocation counts per agent
- Wall-clock phase durations
- Retry budget consumption
- Gate pass/fail rates
- Scope completion times
- Lines changed per scope

Explicitly not tracked: token counts, dollar costs, thinking time, agent efficiency — none of these are observable and guessing would be fabricated data.

---

## 18. Key Lessons & What Worked Well

### 18.1 Mechanical Enforcement Over Prose Rules

Prose governance documents are necessary for context but insufficient for enforcement. AI agents routinely ignore or reinterpret written rules. The breakthrough came from **scripts that mechanically block state transitions** when conditions aren't met. The state transition guard alone runs ~22 automated checks. When an agent can't advance without passing a script, compliance stops being optional.

### 18.2 Artifact Ownership Boundaries

Allowing agents to edit artifacts outside their expertise created subtle, hard-to-detect inconsistencies. Enforcing strict ownership boundaries — with routing protocols for cross-boundary needs — dramatically improved artifact quality. The overhead of routing is far cheaper than the debugging cost of ownership violations.

### 18.3 The Honesty Incentive

Explicitly valuing honest gaps over fabricated completion changed agent behavior. Before this, agents would always attempt to mark everything complete. After encoding that a wrong answer is 3x worse than a blank answer, agents began producing Uncertainty Declarations instead of fabricating evidence — which is far more useful for the next human or agent in the chain.

### 18.4 Evidence Provenance Taxonomy

Distinguishing between "executed" (output directly proves the claim), "interpreted" (output requires reasoning), and "not-run" (nothing was executed) created a fast-review pipeline. Auditors can focus on "interpreted" and "not-run" blocks rather than re-verifying everything.

### 18.5 Gate Classification (Model Compensation vs. Business Invariant)

Tagging which gates exist because AI models are unreliable vs. which gates enforce durable business rules created a framework evolution path. As models improve, the compensation gates can be relaxed. Without this classification, it would be unclear which strictness is fundamental vs. which is compensating for current limitations.

### 18.6 Bottom-Up Completion Chain

Forcing completion to flow strictly upward (evidence → DoD item → scope → spec) eliminated the class of problems where specs were marked done but individual scopes were incomplete. The chain is simple, absolute, and mechanically verifiable.

### 18.7 Scope Isolation

Requiring agents to work within a single scope at a time and preventing cross-scope modifications eliminated cross-contamination. The overhead of formal scope transitions is genuinely cheaper than debugging cascading scope interactions.

### 18.8 Self-Healing with Hard Limits

Allowing bounded self-healing (3 retries, narrowing context, no nesting) let agents recover from most transient failures while preventing infinite thrashing loops. The key insight was the anti-stacking rule: a fix attempt can never trigger another fix attempt.

### 18.9 Planning-First Delivery

Requiring substantive planning artifacts before implementation began eliminated the pattern of agents "improvising" implementation and then backfilling documentation. The planning overhead is always repaid by fewer mid-implementation correction cycles.

### 18.10 Consumer Trace Requirements

Requiring agents to inventory all consumers of any renamed/removed interface before marking work complete eliminated "mostly done" situations where code changes were correct but downstream references were stale.

---

## 19. Framework Evolution Approach

The framework itself follows the upstream-first principle. Changes are proposed, reviewed, and committed in the canonical source repository. Downstream projects receive updates through a standard upgrade mechanism. This prevents framework drift across projects while allowing project-specific customizations through designated extension points (custom gates, project config, local scripts).

---

## Appendix: Conceptual Glossary

| Term | Meaning |
|------|---------|
| **Scope** | A small, independent unit of work with its own scenarios, test plan, and definition of done |
| **DoD** | Definition of Done — checkbox items requiring individual evidence before checking |
| **Gate** | A named, mechanically enforced condition that must pass before state transitions |
| **Phase** | A workflow step owned by a specific specialist agent |
| **Mode** | A workflow configuration defining which phases run, their order, and their status ceiling |
| **Evidence** | Raw terminal output from actual command execution, classified by provenance |
| **Fabrication** | Any claim of completion without actual execution evidence |
| **Uncertainty Declaration** | An honest statement of what couldn't be verified, with actionable next steps |
| **Artifact** | A structured document (spec, design, scopes, report, etc.) with defined ownership |
| **Status Ceiling** | The maximum status a workflow mode is allowed to set |
| **Outcome Contract** | A spec-level declaration of intent, success signal, hard constraints, and failure conditions |
| **Consumer Trace** | An inventory of all code/docs/config that references a renamed or removed interface |
| **Freshness Mode** | How an existing artifact is updated: reconcile, redesign, or replace |
| **Vertical Slice** | A cross-layer scope where frontend, backend, gateway, and tests must all be wired together |
| **Red-Green Traceability** | Requiring failing → passing evidence for changed behavior |

---

## Appendix B: Most Effective Policies, Guardrails & Instructions

The policies and instruction patterns below had the highest measurable impact on steering AI agents toward better code, testing, and validation outcomes.

### B.1 Code Quality Steering

| Policy | What It Does | Why It Works |
|--------|-------------|-------------|
| **No defaults / no fallbacks** | Forbids any pattern that silently substitutes a value when configuration is missing | Forces fail-fast behavior; eliminates silent degradation where missing config is masked by a default that works in dev but breaks elsewhere |
| **No stubs, fakes, or dead code** | Zero tolerance for TODO markers, placeholder functions, commented-out code, unreachable paths | Agents leave stubs and claim completion; this makes that mechanically impossible to get past gates |
| **Implementation reality scan** | Automated script detects hardcoded responses, stub patterns, fake data, hooks with no real calls | Catches the exact patterns agents use to shortcut real implementation — literal response arrays, handlers that return canned data |
| **No self-validating tests** | Tests must not assert on values the test itself injected; the assertion path must pass through real production code | Without this, agents write tests that verify their own setup data round-trips unchanged — proving nothing about the system |
| **Single CLI surface** | All build/test/lint/deploy goes through one repo CLI; no direct tool invocation | Guarantees consistent environments and reproducible results; eliminates "works differently when run directly" |

### B.2 Testing Fidelity Steering

| Policy | What It Does | Why It Works |
|--------|-------------|-------------|
| **Tests validate specs, not implementation** | If a test fails, fix the code — never weaken the test to match broken behavior | Agents instinctively "fix" failing tests by relaxing assertions; this rule blocks that pattern |
| **Canonical test taxonomy with classification integrity** | Defined categories with strict rules about which can use mocks and which must hit live systems | Prevents agents from claiming live-system coverage with mocked tests — a common shortcut |
| **Adversarial regression tests for bug fixes** | Every bug-fix test must include at least one case that fails if the bug returns; tautological tests are forbidden | Without this, agents write regression tests where all fixtures already satisfy the broken condition — the test passes whether the bug is fixed or not |
| **Unbreakable E2E guardrails** | Forbids early-return bailout patterns, optional assertions, and redirect-swallowing in required test bodies | Agents write "defensive" test patterns that convert missing features into silent passes |
| **Red-green traceability** | Changed behavior must show failing state first, then passing state after | Forces agents to prove the fix actually changed something, not just that the test suite is green |
| **No mocking internal systems** | Only external third-party dependencies may be mocked; internal business logic and service boundaries are never mocked | Agents over-mock by default; this ensures tests actually exercise the real system |

### B.3 Validation & Completion Steering

| Policy | What It Does | Why It Works |
|--------|-------------|-------------|
| **Evidence provenance taxonomy** | Every evidence block must be tagged as directly proving, requiring interpretation, or not executed | Creates fast-review triage: reviewers focus on uncertain evidence instead of auditing everything |
| **Minimum raw output threshold** | Evidence blocks with insufficient terminal output are presumed fabricated | Agents produce one-line summaries; this forces capturing enough output to be meaningful |
| **Analysis-as-execution is fabrication** | Reading files and predicting what a script would output is fabrication even when predictions are accurate | Agents read a script's source code, predict its output, and present that as execution evidence; this blocks that shortcut |
| **Per-item evidence requirement** | Each completion checkbox requires its own validation and its own evidence block; batch-checking is forbidden | Without this, agents mark all items complete from a single generic test run |
| **Mechanical state transition guard** | A script with many automated checks must pass before any status can transition to "done" | The single most effective enforcement tool — agents cannot bypass what they cannot write around |
| **Deferral language detection** | Automated scan for phrases like "deferred", "future work", "follow-up", "out of scope", "will address later" | Agents disguise incomplete work with soft language; automated detection catches it |
| **Artifact structure lint** | Validates structure, template compliance, format integrity, evidence depth, provenance tags | Catches structural fabrication: reformatted checkboxes, invented statuses, deleted checklist items |

### B.4 Architecture & Integration Steering

| Policy | What It Does | Why It Works |
|--------|-------------|-------------|
| **Vertical slice gate** | Cross-layer scopes must wire all layers end-to-end with proof | Prevents "backend done, frontend TODO" where agents implement one layer and claim completion |
| **Integration completeness gate** | Every new endpoint, library, page, or service must have at least one real consumer wired up | Prevents orphan code — agents create backend endpoints without wiring them into any consumer |
| **Consumer trace requirement** | Renames/removals must inventory and update all first-party consumers | Agents rename a function and update the call site they see — but miss the other call sites across the codebase |
| **Configuration single source of truth** | All config from one canonical source; generated files are derived, never hand-edited | Eliminates config drift where agents hard-code a value in one place while the canonical config says something different |
| **Auth identity source gate** | Handlers must extract user identity from authenticated context, never from caller-controlled request parameters | Agents routinely pull user IDs from URL paths or request bodies for authorization — this catches that security flaw |

### B.5 Instruction Writing Patterns That Work

| Pattern | Example | Why It Works |
|---------|---------|-------------|
| **FORBIDDEN/REQUIRED tables** | Binary pairs: the bad pattern and the required replacement | Binary rules are harder for agents to misinterpret than nuanced guidance |
| **Concrete examples of both bad and good** | Show the exact anti-pattern next to the exact correct pattern | Agents pattern-match from examples more reliably than from abstract descriptions |
| **Detection commands** | Provide the exact command that catches violations | Makes enforcement self-service; agents can verify their own compliance |
| **Consequences stated explicitly** | "If fabrication is detected: all completion reverts, status regresses, re-execution required" | Agents respond to stated consequences more than to stated principles |
| **Tiered severity** | Explicit labels like ABSOLUTE / BLOCKING / NON-NEGOTIABLE vs. advisory guidance | Clear priority hierarchy prevents agents from treating all rules as equally flexible |
| **Repetition at multiple layers** | Same rule in project instructions, agent prompt, shared governance, and enforcement script | Agents have limited context windows; repeating critical rules at every layer ensures at least one copy is loaded |
