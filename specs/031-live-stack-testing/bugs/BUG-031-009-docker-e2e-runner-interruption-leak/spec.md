# Specification: BUG-031-009 Dockerized Go E2E interruption cleanup

## Release Train

Target train: `mvp`. No feature flag or release-train bundle changes are introduced.

## Expected Behavior

Every child launched by the E2E parent, including a Docker container whose process is daemon-owned, must stop before the parent tears down the shared disposable stack.

### Single-Capability Justification

This repairs the existing E2E child-lifecycle capability implemented by `e2e_run_child` and `e2e_stop_child`. Docker is a second ownership domain for the same child lifecycle, not a new runner provider, strategy, or public capability.

## Requirements

### BR-031-009-001 Stable runner identity

Each Dockerized E2E child must carry the current `e2e_child_run_id` as a Docker label. The identity must be collision-resistant per invocation and must not expose secrets.

### BR-031-009-002 Stop-before-down ordering

`e2e_stop_child` must force-remove all running containers carrying the active run label before `e2e_down_test_stack` executes.

### BR-031-009-003 Scoped cleanup

Cleanup may remove only containers carrying the exact current run ID. It must not remove another test lane, another worktree's unrelated container, dev resources, or persistent volumes.

### BR-031-009-004 Adversarial interruption proof

A live shell E2E regression must interrupt a nested focused Go E2E invocation after its runner container begins executing and prove that the exact runner is absent before teardown completes. The detector must prove it would fail while the container survives.

### BR-031-009-005 Cross-platform authoring

Shell changes must remain WSL/Linux and macOS compatible. Docker label filtering and existing portable process helpers are preferred over `/proc`-only logic.

## Acceptance Criteria

1. Current source fails the controlled Docker-runner interruption regression.
2. The runner receives the exact run-ID label and is removed before Compose teardown.
3. Existing stubborn shell-child cleanup remains green.
4. Focused Drive search/observability neighbors and the serialized Drive package pass without cascade symptoms.
5. Certification remains `in_progress`; no validate-owned completion is claimed.

## Deployment Boundary

This is local test-harness code only. No target deployment, `knb`, evo-x2, manifest, or secret changes are permitted.
