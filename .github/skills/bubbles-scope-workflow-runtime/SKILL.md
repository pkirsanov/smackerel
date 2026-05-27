---
name: bubbles-scope-workflow-runtime
description: Operate Bubbles scope artifacts correctly during planning, implementation, testing, and validation. Use when creating a new scope, deciding between single-file and per-scope-directory layout, writing a Test Plan, choosing DoD shape, picking the next scope to work, or honoring scope isolation during multi-scope work.
---

# Bubbles Scope Workflow Runtime

## Goal
Use the scope layout, DoD shape, dependency declarations, and isolation rules the framework expects, so every scope's evidence and status changes survive `state-transition-guard.sh` and `artifact-lint.sh`.

## When to use
- Creating or revising `specs/<NNN>/scopes.md`
- Splitting a feature into per-scope directories
- Picking the next scope to execute (DAG-based work selection)
- Writing the Test Plan table for a scope
- Choosing between single-file and tiered DoD format

## v4.1.0 scope-kind taxonomy

Scopes may declare an optional `Scope-Kind:` header to opt out of E2E enforcement when the scope legitimately does not produce live-runtime evidence:

```markdown
## Scope 3 — Cosign signature verification
Status: Done (completed_owned)
Scope-Kind: deploy-pointer
Lockdown-FRs: [FR-020]
```

Valid kinds (default `runtime-behavior`): `runtime-behavior`, `contract-only`, `deploy-pointer`, `ci-config`, `docs-only`, `bootstrap`. Only `runtime-behavior` enforces the 3-row E2E DoD/Test-Plan requirement (G008A). See [`docs/v4.1.0-delivered-pending-activation.md`](../../docs/v4.1.0-delivered-pending-activation.md) for the full taxonomy + lockdown tag patterns + evidence-by-reference anchor convention.

## Layout decision
| Scope count | Layout |
|-------------|--------|
| ≤ 5 scopes | Single `specs/<NNN>/scopes.md` with all scopes inline |
| 6+ scopes | `specs/<NNN>/scopes/_index.md` + per-scope directory `specs/<NNN>/scopes/NN-name/scope.md` (one `report.md` per scope optional) |

## Required scope sections (every scope)
1. **Status** — `Not Started` / `In Progress` / `Done` / `Blocked`
2. **Depends On** — list of scope IDs this scope blocks on (DAG, no cycles)
3. **Gherkin scenarios** — Given/When/Then per acceptance criterion
4. **Implementation plan** — short ordered list of steps; vertical slice preferred over horizontal layer batches
5. **Test Plan table** — one row per test that will execute, with file path, category (from Canonical Test Taxonomy), description, command, live-system Yes/No
6. **Definition of Done — Tiered Format** — Core Items (one bullet per scope-specific outcome) + Build Quality Gate (one grouped bullet covering lint/format/warnings/artifact-lint/docs)

## Test Plan ↔ DoD parity (NON-NEGOTIABLE)
Test Plan row count MUST equal DoD test-related item count. The framework's lint rejects scopes where they diverge.

## Stress tests when latency SLAs present (G026)
If the scope's Gherkin or design defines a latency target (`< X ms`, `p95 < Y`, throughput target), the Test Plan MUST include a `stress` row and the DoD MUST include a corresponding stress evidence item.

## Scope isolation (per-scope-directory mode)
- Each scope only edits its own `scopes/NN-name/` files plus its owned source/test files.
- A scope MUST NOT silently amend another scope's DoD, scenarios, or status. Surface the gap via the orchestrator.

## DAG-based pickup
When picking the next scope:
1. List scopes whose `Status: Not Started` AND every `Depends On` is `Done`.
2. From that ready set, prefer the smallest, most foundational scope.
3. Never start work on a scope while a higher-priority pre-existing bug remains in the same feature folder (warn-then-proceed only with explicit acknowledgment).

## Pre-existing bug awareness
Before starting new scope work in `specs/<feature>/`, scan `specs/<feature>/bugs/*/state.json` for `status: in_progress|not_started|blocked`. Surface findings to the orchestrator.

## DoD anti-patterns
- ❌ "All tests passing" as a single catch-all DoD item — split per category
- ❌ "Audit clean" as a catch-all on top of Build Quality Gate — duplicate
- ❌ Pre-checking DoD items `[x]` at scope creation — every `[x]` must come from real execution
- ❌ DoD items without inline evidence reference

## Authoritative modules
- `agents/bubbles_shared/scope-workflow.md` — workflow phases + status ceiling
- `agents/bubbles_shared/scope-templates.md` — scope.md canonical template
- `agents/bubbles_shared/artifact-lifecycle.md` — feature/bug artifact lifecycle
- `agents/bubbles_shared/planning-core.md` — Test Plan shape + test-plan.json handoff
- `agents/bubbles_shared/state-gates.md` — status integrity
- `bubbles/scripts/artifact-lint.sh` — template/section presence enforcement
