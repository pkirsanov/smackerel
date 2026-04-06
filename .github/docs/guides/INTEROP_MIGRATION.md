# Interop Migration Guide

Use this guide when a downstream repo already contains Claude Code, Roo Code, Cursor, or Cline assets and you need an evidence-backed migration path into project-owned Bubbles outputs.

## Truth Surfaces

Do not maintain competitor or migration claims by hand. The comparison surfaces come from:

- [Competitive Capabilities](../generated/competitive-capabilities.md)
- [Interop Migration Matrix](../generated/interop-migration-matrix.md)

Those generated docs are refreshed from `bubbles/capability-ledger.yaml` and `bubbles/interop-registry.yaml`.

## Migration Paths

### Review-Only Intake

Start here when a repo is first adopting Bubbles or when imported assets still need maintainer review.

Review-only intake:
- snapshots raw source-tool assets into `.github/bubbles-project/imports/**`
- writes normalized output and translation reports into the same project-owned import tree
- stages candidate outputs under `.github/bubbles-project/imports/**/proposed-overrides/`
- routes framework-level requests into `.github/bubbles-project/proposals/**`

### Supported Apply

Supported apply is intentionally narrow. It may promote imported content only into explicit project-owned targets recorded in the import manifest:

- `.github/instructions/imported-*.instructions.md`
- additive recommendation blocks in `.specify/memory/agents.md`
- project-owned helper paths under `scripts/`
- project-owned migration skills under `.github/skills/`

Supported apply never writes directly into `.github/bubbles/**`, `.github/agents/bubbles*`, `.github/prompts/bubbles*`, or other framework-managed downstream paths.

### Proposal-Only Outcomes

These stay review-only and must not be auto-applied:

- workflow-mode or framework-surface requests
- collisions with existing project-owned files that cannot be merged cleanly
- any mapping not declared as a supported apply target in the import manifest

When that happens, the apply flow keeps the candidate output under `.github/bubbles-project/imports/**/proposed-overrides/` and records a project-owned proposal under `.github/bubbles-project/proposals/**`.

## Recommended Sequence

1. Run `interop detect` and `interop import --review-only` first.
2. Inspect the packet under `.github/bubbles-project/imports/**`.
3. Apply only through `interop apply --safe`.
4. Re-check the import manifest and any proposal refs before wider framework validation.

## Governance Tradeoff

Bubbles prefers explicit ownership boundaries over high-automation migration. That means supported apply is narrower than what source ecosystems may allow, but the tradeoff is deliberate:

- project-owned outputs can be created safely
- framework-managed surfaces stay immutable in downstream repos
- unsupported requests remain reviewable and auditable