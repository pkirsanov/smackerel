---
name: bubbles-workflow-mode-resolution
description: Resolve a Bubbles workflow mode against `bubbles/workflows.yaml`, including template inheritance, status ceilings, gate wiring, and decision policy. Use when picking a mode for a request, when a guard reports a mode-related failure, or when extending workflows.yaml with a new mode or template. Covers natural-language intent routing and the workflow registry consistency rules.
---

# Bubbles Workflow Mode Resolution

## Goal
Pick the right workflow mode for a request, understand the mode's inherited and explicit policy, and respect the registry's consistency rules.

## When to use
- Translating a natural-language request to a workflow mode
- Adding or editing a mode/template in `bubbles/workflows.yaml`
- Investigating a `workflow-registry-consistency` or `mode-resolver` failure
- Confirming the `statusCeiling`, `executionOptions`, or `gates` for a mode

## How resolution works
1. Read the requested mode by name from `bubbles/workflows.yaml`.
2. Resolve template inheritance via `bubbles/scripts/mode-resolver.sh`:
   - scalar fields flow through from a single template
   - arrays concatenate, dedupe, and sort across templates
   - explicit mode fields override inherited values
   - cycles are rejected (cycle detection)
   - unknown template references are rejected
3. Inspect the resolved mode's:
   - `statusCeiling` — highest status the spec may reach in this mode
   - `executionOptions` — declarative knobs (e.g., spec review default, parent-expanded fallback)
   - `gates` — the gate IDs activated for this mode
   - `decisionPolicy` — mechanical vs taste decision routing
   - `phaseRelevance` — which phases are required, optional, or skipped
   - `crossModelReview` — whether cross-model review applies

## Common natural-language intent mappings
| Plain-English intent | Mode |
|----------------------|------|
| "brainstorm" / "explore an idea" | `brainstorm` |
| "plan but don't code" | `plan-only` |
| "make sure specs are tight" | `spec-scope-hardening` |
| "improve an existing feature" | `improve-existing` |
| "fix the bug" | `bugfix-fastlane` |
| "keep going until done" | `full-delivery` |
| "review documentation only" | `docs-only` |
| "validate the certification" | `validate-only` |
| "audit findings only" | `audit-only` |

For ambiguous requests, `bubbles.super` owns natural-language translation and emits a `route_required` packet with the preferred mode.

## Authoritative modules
- `agents/bubbles_shared/workflow-mode-resolution.md` — template inheritance semantics
- `agents/bubbles_shared/workflow-delegation-core.md` — intent routing ownership
- `agents/bubbles_shared/workflow-input-bootstrap.md` — improve-existing / stale route routing
- `bubbles/workflows.yaml` — single source of truth for modes
- `bubbles/scripts/mode-resolver.sh` — resolver + `--validate` surface
- `bubbles/scripts/workflow-registry-consistency.sh` — registry lint
