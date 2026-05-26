---
name: bubbles-feature-template
description: Use the canonical Bubbles feature artifact template when creating or revising a feature's spec.md, design.md, scopes.md, report.md, uservalidation.md, or state.json. Use when starting a new feature, refreshing a stale feature, or auditing whether a feature folder has the required artifact shape. Covers the v3 control-plane fields (policySnapshot, scenario-manifest.json, certification.*).
---

# Bubbles Feature Template

## Goal
Produce feature artifacts that match the canonical shape so the state-transition guard, artifact lint, and capability ledger work without exemptions.

## When to use
- Creating a new feature folder under `specs/NNN-feature-name/`
- Refreshing a stale feature after `bubbles.spec-review` returns `MAJOR_DRIFT` or `OBSOLETE`
- Adding control-plane fields (`policySnapshot`, scenario manifest, certification) to an existing feature

## Required artifacts (every feature)
| File | Owner | Notes |
|------|-------|-------|
| `spec.md` | analyst (creation), clarify (revision) | acceptance criteria + Gherkin scenarios |
| `design.md` | design | objective research pass for brownfield; Design Brief at the top |
| `scopes.md` or `scopes/_index.md` + `scopes/NN-name/scope.md` | plan | DAG `Depends On`, Test Plan ↔ DoD parity |
| `report.md` (or per-scope `report.md`) | implement/test (or scope owners) | inline ≥10-line evidence per DoD item |
| `uservalidation.md` | human user | checkbox items default to `[x]` after audit; user unchecks `[ ]` to report regression |
| `state.json` | various; certification.* validate-only | execution.* (implement claims), certification.* (validate authority) |

## Control-plane fields (v3+)
- `state.json.policySnapshot` — effective defaults snapshot with provenance, written when the spec enters in_progress
- `scenario-manifest.json` — stable `SCN-*` user-journey contracts; tests preserve scenario IDs across revisions
- `state.json.certification.*` — validate-owned certification claims; observations[] for low/medium findings; high-severity must block
- `state.json.execution.executionHistory[]` — per-round provenance, no implausible jumps
- `state.json.followUps[]` (under `done_with_concerns` only) — explicit owner + action + target

## Anti-patterns
- ❌ Placeholder spec/design content committed as `done` — the artifact-lint rejects
- ❌ Per-scope reports without per-DoD-item inline evidence
- ❌ `policySnapshot` missing when status > `specs_scoped`
- ❌ Tests that reference dropped `SCN-*` IDs without explicit scenario invalidation

## Authoritative modules
- `agents/bubbles_shared/feature-templates.md` — full template text
- `agents/bubbles_shared/artifact-lifecycle.md` — lifecycle rules
- `agents/bubbles_shared/scope-templates.md` — scope.md / scope-dir templates
- `bubbles/scripts/artifact-lint.sh` — template/section presence enforcement
- `docs/guides/CONTROL_PLANE_DESIGN.md` — v3 control-plane rationale
- `docs/guides/CONTROL_PLANE_SCHEMAS.md` — schemas
