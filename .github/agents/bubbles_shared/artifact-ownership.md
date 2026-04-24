# Artifact Ownership

Artifact authorship is a hard boundary, not a suggestion. Violations are blocking — the ownership lint (`agent-ownership-lint.sh`) and state-transition guard enforce these rules mechanically.

## Canonical Ownership

| Artifact | Owner | Notes |
|----------|-------|-------|
| `spec.md` business requirements, actors, use cases, scenarios | `bubbles.analyst` | `bubbles.ux` may update only UX sections of `spec.md` |
| `spec.md` UX sections (`## UI Wireframes`, `## User Flows`, `### Screen: *`) | `bubbles.ux` | MUST be written inline in `spec.md` — sidecar UX files are forbidden (see Forbidden Artifacts below) |
| `design.md` | `bubbles.design` | Technical design only |
| `scopes.md` / `scopes/*/scope.md` planning content | `bubbles.plan` | Gherkin, Test Plan, DoD, execution structure |
| `report.md` template structure | `bubbles.plan` | Execution evidence is appended by execution agents |
| `report.md` evidence content | `bubbles.implement`, `bubbles.test`, `bubbles.devops`, `bubbles.validate`, `bubbles.audit`, `bubbles.chaos`, `bubbles.harden`, `bubbles.gaps`, `bubbles.stabilize`, `bubbles.security`, `bubbles.regression` | Append-only to their own sections |
| `uservalidation.md` | `bubbles.plan` | Acceptance checklist/template |
| `bug.md` | `bubbles.bug` | Bug description, reproduction, severity |
| `objective.md` in `specs/_ops/OPS-*` | `bubbles.devops` | Operational objective, scope, and success conditions for cross-cutting ops work |
| `runbook.md` in `specs/_ops/OPS-*` | `bubbles.devops` | Operational procedures, rollback steps, and verification guidance |
| `state.json` certification state | `bubbles.validate` | `certification.*`, promotion state, and reopen/invalidate certification only |
| `state.json` execution claims | All execution agents | `execution.*` fields only — never `certification.*` |
| `scenario-manifest.json` | `bubbles.plan` | `bubbles.test`, `bubbles.validate`, `bubbles.regression` may update evidence links only |
| `test-plan.json` | `bubbles.plan` | Machine-readable test handoff; `bubbles.test` reads it, never writes it |
| `.specify/memory/retros/*.md` | `bubbles.retro` | Read-only retrospective reports |
| Product code / tests | `bubbles.implement`, `bubbles.test` | Per their phase ownership |
| Operational code / CI/CD / deploy / monitoring surfaces | `bubbles.devops` | Pipelines, build/release automation, deployment config, dashboards, alerts, observability wiring |
| Managed docs (declared in the effective managed-doc registry) | `bubbles.docs` | Must reflect real implementation state |

## Read-Only Diagnostic And Certification Agents

`bubbles.validate`, `bubbles.audit`, `bubbles.harden`, `bubbles.gaps`, `bubbles.stabilize`, `bubbles.security`, `bubbles.code-review`, `bubbles.system-review`, `bubbles.regression`, and `bubbles.clarify` are diagnostic or certification agents. They MAY identify missing scenarios, tests, contracts, or DoD items, but they MUST NOT directly author foreign-owned planning, execution, or certification surfaces.

## Foreign-Artifact Rule (NON-NEGOTIABLE)

If an agent discovers that a foreign-owned artifact must change:

1. It MUST NOT edit that artifact itself — not even "small fixes" or "obvious corrections".
2. It MUST emit a concrete `route_required` result envelope or invoke the owning agent via `runSubagent`, never a narrative-only handoff or "suggested next step".
3. If invoked by `bubbles.workflow`, `bubbles.iterate`, or another orchestrator, it MUST return the route-required packet so the orchestrator can invoke the owner immediately.
4. If invoked standalone, it may explicitly delegate to the owner via `runSubagent`, or it must stop with a concrete owner-targeted route result; it still MUST NOT perform foreign-owned remediation inline.
5. The phase MUST NOT be reported complete until the owning specialist has run and the result has been verified.

Owning one planning artifact does NOT grant permission to mutate sibling planning artifacts. Example: `bubbles.analyst` owns business requirements in `spec.md`, but `design.md` still belongs exclusively to `bubbles.design` and `scopes.md` still belongs exclusively to `bubbles.plan`.

**Examples of violations:**
- `bubbles.harden` adding new Gherkin scenarios to `scopes.md` → must invoke `bubbles.plan`
- `bubbles.implement` creating `uservalidation.md` → must invoke `bubbles.plan`
- `bubbles.gaps` updating DoD items in `scopes.md` → must invoke `bubbles.plan`
- `bubbles.test` modifying `spec.md` requirements → must invoke `bubbles.analyst`
- `bubbles.docs` changing `design.md` architecture → must invoke `bubbles.design`
- `bubbles.analyst` updating `design.md` after a review or analysis run → must invoke `bubbles.design` or return a route-required packet
- Any agent writing `certification.*` fields in `state.json` → must route to `bubbles.validate`

## Execution-Only Exception

`bubbles.implement` may update `scopes.md` only for execution-progress concerns already defined by planning artifacts: inline evidence, DoD checkbox completion, and scope progress tied to completed work. It MUST NOT invent new Gherkin scenarios, Test Plan rows, or DoD structures that belong to `bubbles.plan`.

**DoD Text Immutability (NON-NEGOTIABLE):** The text description of existing DoD items is owned by `bubbles.plan` and MUST NOT be modified by execution agents. `bubbles.implement` may only transition checkboxes (`- [ ]` → `- [x]`) and append inline evidence blocks beneath items. Rewriting a DoD item's behavioral claim to match what was delivered instead of what the Gherkin scenario specified is **content fabrication** — semantically equivalent to deleting the original DoD item and inventing a new one. If the planned DoD text does not match what can be delivered, the agent MUST route to `bubbles.plan` for a plan correction, not silently rewrite the item.

## Forbidden Artifacts (NON-NEGOTIABLE)

These filenames MUST NOT exist anywhere under `specs/<feature>/` (or `specs/<feature>/bugs/<bug>/`). Their content belongs inside owned artifacts.

| Forbidden File | Reason | Correct Home |
|----------------|--------|--------------|
| `ux.md`, `wireframes.md`, `flows.md`, `user-flows.md`, `screens.md` | UX content sidecar — bypasses validation gates UX1–UX5 and breaks `bubbles.design`/`bubbles.implement` handoff | `spec.md` `## UI Wireframes` and `## User Flows` sections (owned by `bubbles.ux`) |
| `actors.md`, `scenarios.md`, `use-cases.md` | Business-content sidecar — fragments `bubbles.analyst` ownership | `spec.md` (owned by `bubbles.analyst`) |
| `architecture.md`, `tech-design.md` | Technical-design sidecar — fragments `bubbles.design` ownership | `design.md` (owned by `bubbles.design`) |
| `dod.md`, `gherkin.md` | Planning sidecar — fragments `bubbles.plan` ownership | `scopes.md` or `scopes/NN-name/scope.md` (owned by `bubbles.plan`) |
| `evidence.md`, `results.md` | Evidence sidecar — bypasses report.md ownership rules | `report.md` or `scopes/NN-name/report.md` |

**Why this is a hard rule:** Bubbles workflow gates and downstream agents read from a fixed set of artifacts. Sidecar files are invisible to validation, gates, and handoffs — content placed there is functionally lost even when it appears thorough. Length, organization, or "separation of concerns" are NOT valid reasons to create a sidecar; if an artifact is too long, the planning agent splits it via the per-scope-directory layout, not via off-spec files.

**Speckit interop note:** `tasks.md`, `data-model.md`, `requirements.md` (under `checklists/`), and `test-plan.md` are NOT forbidden — they are produced by the speckit workflow (which coexists with bubbles) and serve different purposes. The forbidden list above targets only filenames that duplicate bubbles-owned content.

**Enforcement:** `artifact-lint.sh` fails when any forbidden file exists. `bubbles.ux` Tier 2 self-check rejects sidecar UX files before reporting.

## Enforcement

The ownership contract is enforced at three levels:

1. **Prompt-level** — each agent declares an explicit `**Artifact Ownership**` block listing what it owns and what is foreign. This declaration is cross-checked against `agent-ownership.yaml`.
2. **Lint-level** — `agent-ownership-lint.sh` verifies that diagnostic agents do not contain language permitting foreign-artifact edits, and that every agent's declared ownership matches the YAML registry.
3. **Guard-level** — `state-transition-guard.sh` verifies artifact authorship coherence before allowing `done` transitions (Gate G042).

## Related Modules

- [evidence-rules.md](evidence-rules.md) — evidence attribution is an ownership rule (agents may only write evidence for their own phase)
- [completion-governance.md](completion-governance.md) — the completion chain that artifact ownership supports
- [state-gates.md](state-gates.md) — mechanical gate definitions including G042 (artifact ownership enforcement) and G066 (phase-claim provenance)