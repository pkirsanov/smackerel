---
name: bubbles-skills-first-discovery
description: Discover which Bubbles policy skill applies to the current task. Use as the first step when an agent is uncertain which governance module to load, when a user asks about Bubbles policy without naming a specific module, or when onboarding a new agent that should lean on skills instead of bulk-loading agent-common.md. Maps common situations to the right skill.
---

# Bubbles Skills-First Discovery

## Goal
Replace eager-loading of `agents/bubbles_shared/*.md` with on-demand skill loading driven by what the agent is actually about to do. This is the entry point that maps a situation to the right skill.

## When to use
- An agent is starting a task and needs to know which Bubbles policy applies
- A user asked a Bubbles policy question with no module name
- Reviewing whether agent prompts can shed bulk by relying on skill auto-loading

## Situation → Skill map
| If you are about to... | Load this skill |
|------------------------|-----------------|
| Mark a DoD item `[x]` or claim a test/build passed | `bubbles-anti-fabrication` |
| Write a `report.md` evidence section or capture terminal output | `bubbles-evidence-capture` |
| Transition a scope to `Done` or a spec to `done` | `bubbles-dod-validation` |
| Change `state.json` status or run pre-push status checks | `bubbles-status-transition` |
| Return control to the orchestrator at end of an agent run | `bubbles-result-envelope` |
| Edit any artifact — confirm you own it, or route if not | `bubbles-artifact-ownership-routing` |
| A guard rejected work with a `G0XX` label | `bubbles-quality-gates-catalog` |
| Author or revise `scopes.md` / `scopes/*/scope.md` | `bubbles-scope-workflow-runtime` || Creating or refreshing a feature folder | `bubbles-feature-template` |
| Filing or working a bug under `specs/<feature>/bugs/BUG-*` | `bubbles-bug-template` |
| Orchestrating a workflow round (dispatch-and-wait) | `bubbles-workflow-execution-loops` |
| Translating natural-language intent to a workflow mode | `bubbles-workflow-mode-resolution` |
| Running a fix-cycle round; finding-set closure | `bubbles-fix-cycle-protocol` || Write or extend a Bubbles skill | `bubbles-skill-authoring` |
| Design a reusable capability (adapter/provider/strategy) | `bubbles-capability-foundation-design` |
| Write tests of any kind | `bubbles-test-integrity` |
| Touch test compose files, test DB setup, or test data | `bubbles-test-environment-isolation` |
| Add or change config values, ports, or services | `bubbles-config-sst`, `bubbles-docker-port-standards`, `bubbles-docker-lifecycle-governance` |
| Author a deploy target adapter | `bubbles-deployment-target-adapter` |
| Write a spec.md from scratch with BDD | `bubbles-spec-template-bdd` |
| Surface product principles from existing repo evidence | `bubbles-product-principle-discovery` |
| Audit downstream repo readiness | `bubbles-repo-readiness` |
| Apply a premium / cinematic design language to a UI feature (opt-in per repo) | `bubbles-cinematic-design` |

## Why skills-first
Skills are auto-loaded by description match (semantic search) by Copilot, Claude, Cursor, and similar tools. They are lazy: a skill only enters context when the description matches the work. The Bubbles framework keeps the authoritative governance in `agents/bubbles_shared/*.md` modules — skills are discovery shims that route to those modules.

This avoids:
- Eager-loading 14k+ lines of policy at the top of every agent prompt
- Duplicating policy text across 38 agents
- Drift between agent prompts and the shared module set

## Authoritative governance still lives in modules
Skills are entry points. The non-negotiable rules and the mechanical guards still live in:
- `agents/bubbles_shared/*.md` — full policy text
- `bubbles/scripts/*.sh` — enforcement scripts
- `bubbles/workflows.yaml` — workflow modes + gate wiring
- `bubbles/capability-ledger.yaml` — shipped capabilities

When a skill points to a module, open the module for the full enforceable text. Do not rewrite policy inside agent prompts.

## Grandfather clause reminder (PRESERVED across the skills-first refactor)
Historical `done` specs do not get re-evaluated under new skill-derived policy unless their `state.json` is touched in the same commit. `done-spec-audit.sh --profile advisory` (the default) keeps the grandfather intact. Skills do not change this.
