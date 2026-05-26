---
name: bubbles-artifact-ownership-routing
description: Respect Bubbles artifact ownership boundaries and route foreign-artifact changes through the owning agent. Use when about to modify any spec/scope/state/test/source/docs artifact, when discovering a problem in someone else's artifact, or when handling a stale spec/design/scope. Covers ownership tables, freshness/supersession, and route-required packets.
---

# Bubbles Artifact Ownership & Routing

## Goal
Edit only the artifacts your agent owns. When you find a defect in a foreign-owned artifact, route it instead of patching it inline. Keep ownership boundaries clean so the framework's routing guarantees hold.

## When to use
- About to edit any file under `specs/`, `tests/`, `src/`, `docs/`, or framework-managed paths
- About to modify `state.json`, `scopes.md`, `report.md`, `uservalidation.md`, `design.md`, `spec.md`
- Discovered a bug or staleness in someone else's owned artifact
- Inheriting work from another specialist via a handoff packet

## Ownership at a glance (consult agent-ownership.yaml for the authoritative table)
| Artifact | Owner |
|----------|-------|
| `specs/<NNN>/spec.md` | `bubbles.analyst` (creation), `bubbles.clarify` (revision after questions) |
| `specs/<NNN>/design.md` | `bubbles.design` |
| `specs/<NNN>/scopes.md`, `scopes/*/scope.md` | `bubbles.plan` |
| `specs/<NNN>/report.md` | the agent that produced the evidence (commonly `bubbles.implement`, `bubbles.test`) |
| `specs/<NNN>/uservalidation.md` | human owner (agents create the checklist; only humans uncheck items) |
| `specs/<NNN>/state.json` execution.* | `bubbles.implement` (claims), `bubbles.test`, `bubbles.audit` |
| `specs/<NNN>/state.json` certification.* | `bubbles.validate` only |
| Source code, tests, configs | `bubbles.implement`, `bubbles.test`, `bubbles.devops` per scope |
| Framework-managed `.github/bubbles/**` | upstream Bubbles only (downstream consumers MUST NOT edit) |

## Cross-artifact mutation rule (NON-NEGOTIABLE)
If your scope's owned artifact is, say, `report.md`, you may NOT edit `spec.md` to make a test pass. The flow is:
1. Detect the gap in the foreign artifact.
2. Emit a `route_required` finding with `nextRequiredOwner: bubbles.<owner>`.
3. Stop. Wait for the orchestrator to dispatch the owner.

## Freshness and supersession
When `spec.md` or `design.md` drift after implementation has already shipped:
- Do NOT silently rewrite them. That breaks traceability.
- Open the spec-review handoff: `bubbles.spec-review` decides `STILL_TRUE`, `MINOR_DRIFT`, `MAJOR_DRIFT`, or `OBSOLETE`.
- For `MAJOR_DRIFT` or `OBSOLETE` on a `done` spec, `bubbles.workflow` auto-dispatches `improve-existing` mode and the spec re-enters in-progress.

## Framework-managed boundary (downstream repos)
Consumer repos must not edit:
- `.github/bubbles/**` (scripts, workflows.yaml, capabilities, ledger)
- `.github/agents/bubbles*` (framework agents)
- `.github/prompts/bubbles*`
- `.github/skills/bubbles-*` (framework-shipped skills)
- `.github/instructions/bubbles-*.instructions.md`

If a downstream repo needs a framework change, file a project-owned proposal at `.github/bubbles-project/proposals/<slug>.md` or run `bubbles framework-proposal <slug>`. The actual change lands upstream in the Bubbles source repo.

## Mechanical enforcement
- `downstream-framework-write-guard.sh` blocks pushes that touch framework-managed paths from a downstream
- `agent-ownership-lint.sh` catches agents that claim ownership outside their declared surface
- `artifact-freshness-guard.sh` rejects stale artifacts that are being used as current authority

## Authoritative modules
- `agents/bubbles_shared/artifact-ownership.md` — ownership tables + delegation contract
- `agents/bubbles_shared/artifact-freshness.md` — supersession + spec-review routing
- `agents/bubbles_shared/artifact-lifecycle.md` — feature/bug artifact lifecycle
- `agents/bubbles_shared/consumer-trace.md` — renames + removals
- `bubbles/agent-ownership.yaml` — machine-readable ownership index
- `bubbles/scripts/downstream-framework-write-guard.sh` — mechanical guard
