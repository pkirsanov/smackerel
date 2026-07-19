# Bug: BUG-038-003 Drive E2E core health collapse

## Summary

In the serialized broad E2E run, the Drive cross-feature test completed its API assertion, then the next Drive observability test could not reach healthy core for two minutes; subsequent Drive and later packages observed missing core/network services.

## Severity

- [ ] Critical - System unusable or data loss
- [x] High - The real disposable stack disappears and invalidates an entire regression lane
- [ ] Medium - Feature broken with a reliable workaround
- [ ] Low - Minor issue

## Status

- [x] Reported
- [x] Confirmed
- [x] In Progress
- [x] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. Confirm no concurrent Smackerel test process and no residual `smackerel-test` containers, networks, or volumes.
2. Start the real disposable E2E stack through the repository CLI.
3. Run `TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture` alone.
4. Capture core container state, exit code, health response, and logs immediately if readiness fails.
5. Repeat in package order with the Drive neighbor sequence that precedes observability.
6. Run the next Drive health probe to determine whether state contamination survives the observability test.

## Expected Behavior

Core remains healthy before, during, and after the observability fixture. The fixture reconciles live metric registration, in-process counter deltas, and database rows without stopping or exhausting shared disposable services. Later serialized Drive tests begin against the same healthy stack.

## Actual Behavior

The synthesis closeout broad run waited two minutes for core health in the observability test, then later Drive tests reported core unhealthy and later packages could no longer resolve stack services.

## Environment

- Service: disposable `smackerel-test` core and E2E lifecycle
- Source baseline: `a6d2fb3ffd03e7b09e294f2cdac14816fb2f5d4f`
- Test category: live `e2e-api`, serialized Go packages
- Platform: Linux Docker runtime

## Error Output

```text
=== RUN   TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture
drive_observability_e2e_test.go:48: e2e: services not healthy after 2m0s at http://smackerel-core:8080
--- FAIL: TestDriveObservabilityE2E_MetricsAndCountersReconcileAfterStressFixture (121.94s)
drive_policy_e2e_test.go:38: e2e: services not healthy after 30s at http://smackerel-core:8080
drive_scan_e2e_test.go:17: waitForHealth
FAIL github.com/smackerel/smackerel/tests/e2e/drive 300.054s
spec_076_migrations_e2e_test.go:59: lookup postgres on 127.0.0.11:53: no such host
FAIL github.com/smackerel/smackerel/tests/e2e/foundation 0.056s
```

This is inherited routing provenance, not current-session RED certification.

## Root Cause

Drive-local causes were falsified: observability alone, the cross-feature-to-observability neighbor sequence, the full serialized Drive package, the four known assistant failures followed by both Drive probes, and the complete assistant package followed by both Drive probes all left core healthy. The readiness helper calls non-strict `/api/health`, which returns HTTP 200 for every live core, so its two-minute failure proves core/network loss rather than dependency degradation. The first actual defect is owned by BUG-031-009: parent interruption kills the host Docker CLI but leaves the daemon-owned Go runner alive while the cleanup trap tears down the Compose stack.

## Related

- Owning feature: `specs/038-cloud-drives-integration/`
- Owning requirements: FR-017, FR-018, SCN-038-023
- Parent synthesis evidence: `specs/026-domain-extraction/bugs/BUG-026-008-synthesis-schema-repair/report.md#independent-broad-findings-routed-out-of-packet`
- Independent search packet: `../BUG-038-002-provider-neutral-search-omission/`
