# Spec: BUG-073-003 — Toolchain-gate the cross-language renderer canary

## Problem Statement

The spec-073 cross-language renderer canary fails the `CI` `lint-and-test` job because the
Go unit runner (`golang:1.25.10-bookworm` container) has no `node` or `dart`. The canary
treats toolchain absence as a hard failure (`t.Fatalf`), which is wrong: absence is an
environment gap, not a code defect.

## Expected Behavior (Requirements)

- **R1 — Skip on absence.** When `node` and/or `dart` are not on PATH, the toolchain-dependent
  canary tests MUST `t.Skip` with a clear reason, NOT fail. This greens the go-only CI unit
  lane and partial developer environments.
- **R2 — Fail-loud on present-but-broken.** When the toolchains ARE present, every existing
  fail-loud path (dart AOT compile failure, empty/`non-executable` exe, JS↔Dart disagreement,
  golden mismatch, schema violation) MUST remain `t.Fatalf`. Drift detection MUST NOT be
  weakened.
- **R3 — Canary still runs in CI.** The canary MUST execute in at least one CI job that
  provisions node + dart, so cross-language drift is still caught on every push. The wiring
  MUST be mechanically guarded so the canary can never silently drop out of CI.
- **R4 — Non-tautological coverage.** The skip-vs-fail decision MUST be covered by tests that
  would fail if the decision regressed to "always run" (re-introducing the CI failure) or
  "always skip" (silently disabling drift detection).

## Acceptance Criteria (BDD)

```gherkin
Feature: BUG-073-003 — Cross-language canary is environment-gated, not silently disabled

  Scenario: Toolchain absent → canary skips (not fails)
    Given node and dart are NOT on PATH (the go-only CI unit container)
    When the cross-language renderer canary tests run
    Then they report "--- SKIP" with a reason naming the missing toolchain
    And the Go unit lane exits 0

  Scenario: Toolchain present but output drifts → canary still fails loud
    Given node and dart ARE on PATH
    And a golden descriptor is deliberately corrupted
    When the cross-language renderer canary runs
    Then the affected fixture subtest FAILS with a deep-equal/drift diagnostic (t.Fatalf)
    And no skip masks the drift

  Scenario: Skip decision is non-tautological
    Given a stubbed PATH probe
    When node is reported absent
    Then the gating decision returns available=false with a reason naming node
    And when both are reported present the decision returns available=true with no reason

  Scenario: Canary still runs in CI with real toolchains
    Given the dedicated cross-language-canary CI job provisions node and Flutter/dart
    When the job runs the canary
    Then the 7 cross-language fixtures execute (not skip) and the drift comparison is live
    And a contract test fails the build if that job is removed or unwired
```

## Out Of Scope

- Refactoring the dart package's `flutter` SDK dependency or the renderer cores.
- Changing the render-descriptor schema or fixtures.
- spec-083 card-rewards WIP (explicitly untouched).
