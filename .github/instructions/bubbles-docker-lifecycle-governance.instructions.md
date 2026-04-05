# Bubbles Docker Lifecycle Governance Instructions

Use this instruction when creating or modifying Dockerfiles, Compose files, cleanup scripts, build scripts, or validation flows.

## Required checks

1. Classify affected resources as `persistent`, `ephemeral`, `cache`, `tooling`, or `monitoring`.
2. Preserve protected persistent state by default.
3. Ensure tests and validation target disposable storage.
4. Verify build freshness using image identity, not timestamps.
5. Prefer Compose project names, profiles, and labels for grouping.

## Required behavior

- Do not recommend destructive cleanup by default.
- Do not prune volumes unless the workflow explicitly requests it.
- Do not allow validation or test flows to write into the main persistent development store.
- Do not treat `latest` tags or image creation times as proof of freshness.
- Do not rely on `container_name` alone as the grouping mechanism for growing stacks.

## Preferred patterns

- Named volumes for persistent stores
- External volumes for lifecycle-managed permanent stores
- `tmpfs` or isolated validation stacks for disposable test state
- Build identity labels: revision, source hash, dependency hash, build time
- Label-aware cleanup filters
- Project-scoped cleanup before any system-wide cleanup

## References

- `.github/agents/bubbles_shared/docker-lifecycle-governance.md`
- `.github/agents/bubbles_shared/agent-common.md`
- `.github/bubbles/workflows.yaml`