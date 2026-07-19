# Specification: BUG-038-003 Drive E2E core health collapse

## Release Train

Target train: `mvp`. This bug introduces no feature flag and does not modify release-train bundles.

## Expected Behavior

The serialized Drive E2E package shares one parent-owned disposable stack. Every test must preserve stack ownership, use isolated rows, and leave core/network health available to its successor. Readiness checks must be bounded by actual service state and produce actionable terminal evidence.

## Requirements

### BR-038-003-001 Isolated observability fixture

The observability E2E must reconcile the registered Drive metric families, per-provider counter deltas, and persisted row counts without stopping, restarting, exhausting, or reconfiguring core.

### BR-038-003-002 Serialized neighbor safety

The cross-feature-to-observability sequence and the next Drive scenario must all execute against the same healthy parent-owned stack. Test-local cleanup may delete only rows created by that test.

### BR-038-003-003 Diagnostic readiness

Readiness polling must use bounded requests and report the last observed HTTP/error state. If the core process or container is terminal, the harness must surface that state rather than hiding it behind an arbitrary wait.

### BR-038-003-004 No cascade over-filing

Drive policy/retrieve/save/scan, foundation, retirement, transport, and wiki failures after core/network disappearance are one cascade class until the first actual core/lifecycle defect is fixed and rerun.

### BR-038-003-005 Real disposable stack

All regression execution must use the repository-owned disposable test stack and real internal services. Mocks, interception, production monitoring, persistent dev state, and cleanup-based reuse of foreign data are forbidden.

## Acceptance Criteria

1. Isolation and package-order RED runs classify the first actual defect with container/process evidence.
2. An adversarial neighbor-order regression fails if a test stops or poisons core/network health.
3. Observability reconciliation and the immediate successor health probe both pass on the same stack.
4. The full serialized Drive E2E package passes; no full all-package E2E is run in this invocation.
5. Certification remains `in_progress`; no validate-owned completion is claimed.

## Deployment Boundary

This branch changes source/tests/packet only. It does not operate evo-x2, modify `knb`, or deploy a runtime.
