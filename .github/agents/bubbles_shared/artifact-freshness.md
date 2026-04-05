# Artifact Freshness Governance

Use this file when existing `spec.md`, `design.md`, or planning artifacts must be updated because requirements, behavior, UX, or architecture changed.

## Core Rule

Every artifact must present exactly one active truth.

If old requirements, wireframes, design decisions, or scopes are no longer valid, they MUST NOT remain mixed into the active sections as if they still apply.

## Default Reconciliation Behavior

For existing artifacts, authoring agents default to reconciliation, not blind append.

- `bubbles.analyst` reconciles business requirements, actors, use cases, scenarios, and product assumptions.
- `bubbles.ux` reconciles screen inventory, wireframes, flows, and responsive/accessibility expectations.
- `bubbles.design` reconciles architecture, contracts, data models, rollout decisions, and failure handling.
- `bubbles.plan` reconciles active scopes, test plans, DoD items, and execution ordering.

## Freshness Modes

Use these modes when touching an existing artifact:

| Mode | Meaning | Default Use |
|------|---------|-------------|
| `reconcile` | Bring the artifact back to one active truth while preserving history of invalidated sections | Default when an artifact already exists |
| `redesign` | Rework major flows, UX, contracts, or architecture while preserving feature identity | Significant behavioral or structural change |
| `replace` | Most prior content is no longer valid and must be superseded by a new artifact shape | Near-total rewrite |

`greenfield` / `from-scratch` remain valid for new artifacts.

## Deprecate Or Suppress First

When old content becomes invalid:

1. Remove it from active sections immediately.
2. Preserve it temporarily only in an explicitly labeled superseded or suppressed section if historical context still matters.
3. Cleanup can delete the superseded material later, but stale content must not remain active meanwhile.

Accepted labels include:

- `## Superseded Requirements`
- `## Superseded UX`
- `## Superseded Design Decisions`
- `## Superseded Scopes (Do Not Execute)`

The exact heading may vary, but the content must be clearly marked non-active and non-authoritative.

## Scope Invalidation Rules

Planning artifacts must treat stale scopes as invalid execution instructions.

- A scope that no longer matches current `spec.md` or `design.md` MUST be removed from the active scope inventory.
- Invalid scopes may be preserved only in a clearly marked superseded appendix.
- Superseded scopes MUST NOT remain eligible for execution, completion, or status promotion.
- If requirements/design changes invalidate tests or DoD, `bubbles.plan` must rewrite those sections instead of leaving drift in place.

## Validation Expectations

Freshness is satisfied only when:

- active sections agree with each other
- contradictory legacy content is removed from active sections
- any preserved legacy material is explicitly marked superseded
- active scopes reflect the current product/design truth

## Mechanical Guard Rules

The freshness guard enforces the minimum structure needed to keep preserved history from acting like active truth.

- In `spec.md` and `design.md`, once a superseded or suppressed section begins, only non-active appendix-style material may follow.
- In planning artifacts, a superseded scope appendix must not contain active execution markers such as `**Status:**`, `##/### Test Plan`, `##/### Definition of Done`, Test Plan tables, or DoD checkboxes.
- In per-scope directory mode, every real `scopes/NN-*` directory must still be referenced by `scopes/_index.md`, and every indexed scope directory must still be mirrored in `state.json.scopeProgress.scopeDir`. Drift in either direction is a stale planning artifact.
- If historical scope detail must be preserved, rewrite it as archival prose rather than leaving it in executable scope format.

If those conditions fail, the artifact is stale and must be lowered, reconciled, or regenerated before downstream work continues.