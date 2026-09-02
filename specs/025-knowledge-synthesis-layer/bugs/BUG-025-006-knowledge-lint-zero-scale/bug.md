# Bug: BUG-025-006 Knowledge lint can pass at zero scale

## Summary

`TestKnowledge_LintAt1000ArtifactScale` invokes production lint without creating or counting any artifacts. A healthy empty stack can therefore satisfy the current report and timing assertions.

The same dirty test file replaces endpoint and configuration `t.Skip` paths with fail-loud assertions. The fix must preserve those changes.

## Severity

- [ ] Critical - System unusable or data loss
- [x] High - A required scale gate can report success without exercising its declared workload
- [ ] Medium - Feature broken with a reliable workaround
- [ ] Low - Minor issue

## Status

- [x] Reported
- [ ] Confirmed by an executed reproduction
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

Source inspection confirms the missing setup and cardinality assertions. No Docker-backed reproduction ran during this packet-only invocation.

## Reproduction Steps

1. Use the dirty `tests/stress/knowledge_stress_test.go` at inspected revision `7ce32d703a17f3cb5a2481a1ebc0cc084a1fac61`.
2. Read `TestKnowledge_LintAt1000ArtifactScale` from PostgreSQL connection setup through `linter.RunLint`.
3. Observe that the function performs no artifact insert before production lint starts.
4. Observe that the function performs no owned-row count or distinct-row count before production lint starts.
5. Run the focused scenario later through `./smackerel.sh test stress` against the disposable stress stack.
6. Record whether the test passes while its unique ownership marker has zero rows.

Step 5 was not executed because the operator restricted this invocation to artifact creation and prohibited Docker.

## Expected Behavior

The stress test must create exactly 1000 uniquely identifiable synthetic artifacts in disposable PostgreSQL. It must prove that count before invoking the real production linter.

The test must assert a fresh persisted lint result and both measured durations. The wall duration and report duration must remain within five minutes.

Cleanup must delete only test-owned artifacts and the report created by the test. Cleanup must fail the test when any owned row remains.

Missing stress configuration, unavailable endpoints, transport errors, and unexpected endpoint statuses must fail. They must never become `t.Skip` outcomes.

## Actual Behavior

The dirty function constructs a real `KnowledgeStore` and `Linter`, then calls `RunLint`. It only validates the returned report identity, freshness, and timing.

No code establishes the advertised 1000-artifact precondition. The report assertions can therefore succeed when the artifact table contains zero test-owned rows.

## Environment

- Repository: Smackerel
- Parent feature: `specs/025-knowledge-synthesis-layer`
- Requirement: `R-2506`
- Performance contract: daily lint completes within five minutes for a 1000-artifact knowledge base
- Test: `tests/stress/knowledge_stress_test.go::TestKnowledge_LintAt1000ArtifactScale`
- Platform: Linux
- Inspected revision: `7ce32d703a17f3cb5a2481a1ebc0cc084a1fac61`
- Dirty file size: 122 inserted lines and 24 deleted lines

## Inspection Evidence

The current function follows this control path:

```text
connect disposable PostgreSQL
connect disposable NATS
construct KnowledgeStore
construct production Linter
call linter.RunLint
read GET /api/knowledge/lint
assert report identity, freshness, and duration
```

No artifact seed or artifact cardinality query appears before `linter.RunLint`.

## Root Cause

The test name and comments carry the scale claim, but executable setup does not establish it. The dirty repair made lint execution and endpoint failures fail loud without adding a measurable workload precondition.

## Change Boundary

This invocation may change only:

- `specs/025-knowledge-synthesis-layer/bugs/BUG-025-006-knowledge-lint-zero-scale/**`

A later implementation may change only:

- `tests/stress/knowledge_stress_test.go`
- this bug packet for execution evidence and owner-controlled status updates

The later implementation must not change production runtime code, migrations, configuration, Compose files, generated files, caches, or other tests.

## Related

- Parent feature: `specs/025-knowledge-synthesis-layer`
- Parent requirement: `R-2506: Knowledge Lint System`
- Parent performance contract: `< 5 minutes for 1000 artifacts`
- Neighbor packet: `BUG-025-003-health-endpoint-stress-budget`
- Dirty test file: `tests/stress/knowledge_stress_test.go`

## Deferred Reason

The operator requested a complete bug packet only. Runtime and test changes require a later explicit activation of `bugfix-fastlane` for this folder.