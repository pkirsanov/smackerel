---
name: bubbles-docker-lifecycle-governance
description: Use when designing or modifying Docker build, cleanup, compose, or validation workflows that need correct freshness, low disk usage, persistent-store protection, disposable test storage, and predictable stack grouping.
---

# Bubbles Docker Lifecycle Governance

## Goal
Keep Docker workflows fast, fresh, safe, and clean.

## Use This Skill When
- Changing Dockerfiles
- Changing Compose files
- Adding cleanup commands
- Adding rebuild or deploy verification logic
- Deciding whether storage should be persistent or disposable
- Designing stack grouping for local, validation, or CI workflows

## Workflow
1. Classify each affected resource.
2. Decide whether the protected object is the container, volume, image, or network.
3. Verify freshness behavior for built images.
4. Verify cleanup behavior for cache versus persistent state.
5. Verify validation and test storage isolation.
6. Verify stack grouping through project names, profiles, and labels.

## Resource Classification
- `persistent`: must survive normal cleanup and rebuilds
- `ephemeral`: disposable validation or test state
- `cache`: speed-up artifact, safe to prune under pressure
- `tooling`: debug or operator tool, safe to recreate
- `monitoring`: observability component, preserve unless explicitly marked disposable

## Mandatory Rules
- Protect persistent volumes by default.
- Use disposable storage for tests and validation.
- Use image identity labels for build freshness.
- Prefer label-aware cleanup over broad prune commands.
- Use project name and profiles as the main stack-grouping mechanism.

## Do Not Do
- Broad system prune as a default action
- Volume pruning without explicit opt-in
- Validation writes into persistent developer stores
- Skip-build decisions based only on timestamps or tags
- Group large stacks using only `container_name`

## References
- `.github/agents/bubbles_shared/docker-lifecycle-governance.md`
- `.github/agents/bubbles_shared/agent-common.md`
- `.github/instructions/bubbles-docker-lifecycle-governance.instructions.md`