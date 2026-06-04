# Sweep Round 5 — Spec 054 Findings Closure

Parent: stochastic-quality-sweep round 5
Target: `specs/054-notification-intelligence-handler`
Closed by: bubbles.plan
Date: 2026-06-04

## Findings

### FND-054-T5-01 (medium, traceability-drift) — CLOSED

Refreshed 14 stale `linkedTests[].testTitle` entries in
`specs/054-notification-intelligence-handler/scenario-manifest.json` to match
current Go test function names. Mapping applied:

| Scenario(s) | Old → New |
|---|---|
| SCN-054-016 | TestDiagnosticsRunnerExecutesOnlyReadOnlyChecksAndAuditsResults → TestDiagnosticsRunnerExecutesOnlyReadOnlyAllowlistedChecks |
| SCN-054-017 | TestActionExecutorRunsOnlyAllowlistedLowRiskActions → TestActionExecutorRunsOnlyLowRiskAllowlistedAutonomousActions |
| SCN-054-018 | TestApprovalPolicyRequiresUserApprovalAndRefusesDestructiveAutomation → TestHighBlastRadiusRequiresApprovalAndDestructiveActionsAreRefused |
| SCN-054-019 | TestLoopGuardSuppressesReentrantOutputAndActionEvents → TestLoopGuardSuppressesReentrantOutputEvents |
| SCN-054-020 | TestOutputDispatcherDeliversRedactedSourceQualifiedRequests → TestOutputDispatcherBuildsConciseRedactedSourceQualifiedMessage |
| SCN-054-021 | TestOutputChannelResultsCannotMutateCorePolicy → TestOutputChannelResultCannotMutateCorePolicy |
| SCN-054-020, SCN-054-021 | TestOutputDeliveryAttemptsPersistWithoutMutatingPolicy → TestOutputDeliveryAttemptsPersistWithoutIncidentPolicyMutation |
| SCN-054-016, SCN-054-017, SCN-054-018, SCN-054-019 | TestReactionApprovalActionAndLoopRecordsPersist → TestDiagnosticsActionsApprovalsAndLoopGuardsPersist |
| SCN-054-024 | TestNotificationRedactionCoversLogsAPIRawPreviewsDiagnosticsActionsAndDeliveries → TestNotificationRedactorRemovesSecretsFromLogsAPIAndDeliveryPayloads |
| SCN-054-026 | TestCoreNotificationPackageHasNoNtfyProductionDependency → TestCoreNotificationPackageHasNoNtfySpecificProductionDependency |

All 10 new names verified to exist via grep against `internal/notification/**/*_test.go`:

```
internal/notification/reaction_store_integration_test.go:11: func TestDiagnosticsActionsApprovalsAndLoopGuardsPersist
internal/notification/approval_policy_test.go:5: func TestHighBlastRadiusRequiresApprovalAndDestructiveActionsAreRefused
internal/notification/action_executor_test.go:8: func TestActionExecutorRunsOnlyLowRiskAllowlistedAutonomousActions
internal/notification/diagnostics_runner_test.go:9: func TestDiagnosticsRunnerExecutesOnlyReadOnlyAllowlistedChecks
internal/notification/output_store_integration_test.go:11: func TestOutputDeliveryAttemptsPersistWithoutIncidentPolicyMutation
internal/notification/redaction_test.go:8: func TestNotificationRedactorRemovesSecretsFromLogsAPIAndDeliveryPayloads
internal/notification/no_ntfy_core_dependency_test.go:5: func TestCoreNotificationPackageHasNoNtfySpecificProductionDependency
internal/notification/output_dispatcher_test.go:9: func TestOutputDispatcherBuildsConciseRedactedSourceQualifiedMessage
internal/notification/output_dispatcher_test.go:26: func TestOutputChannelResultCannotMutateCorePolicy
internal/notification/loop_guard_test.go:8: func TestLoopGuardSuppressesReentrantOutputEvents
```

### FND-054-T5-02 (low, manifest-schema-violation) — CLOSED

Two `linkedTests[]` entries pointed e2e-ui category at production source files
(`internal/api/notifications_pipeline.go`, `internal/api/notifications.go`)
which is illegal per scenario-manifest schema.

Investigation: searched `tests/e2e/` for real web-surface coverage of
`/notifications` operator pages and redacted responses. Found two existing
live-stack Go e2e tests that hit the rendered web pages:

- `tests/e2e/notification_operator_web_test.go::TestNotificationOperatorPagesShowRedactedStatusAndIncidentTimeline`
- `tests/e2e/notification_security_web_test.go::TestNotificationWebSurfacesAreRedactedAndAuthProtected`

Relinked (no schema downgrade required — real e2e-ui coverage exists):

- SCN-054-022: `internal/api/notifications_pipeline.go` (bogus) → `tests/e2e/notification_operator_web_test.go::TestNotificationOperatorPagesShowRedactedStatusAndIncidentTimeline`
- SCN-054-024: `internal/api/notifications.go` (bogus) → `tests/e2e/notification_security_web_test.go::TestNotificationWebSurfacesAreRedactedAndAuthProtected`

`requiredTestType` unchanged — e2e-ui obligation now backed by real live-stack
test files.

## Validation Evidence

### artifact-lint.sh

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/054-notification-intelligence-handler
...
Artifact lint PASSED.
EXIT=0
```

### traceability-guard.sh

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/054-notification-intelligence-handler
...
--- Traceability Summary ---
ℹ️  Scenarios checked: 30
ℹ️  Test rows checked: 63
ℹ️  Scenario-to-row mappings: 30
ℹ️  Concrete test file references: 30
ℹ️  Report evidence references: 30
ℹ️  DoD fidelity scenarios: 30 (mapped: 30, unmapped: 0)

RESULT: PASSED (0 warnings)
EXIT=0
```

## RESULT-ENVELOPE

```yaml
agent: bubbles.plan
outcome: completed_owned
addressedFindings:
  - id: FND-054-T5-01
    severity: medium
    class: traceability-drift
    status: closed
    summary: 14 stale linkedTests testTitle entries refreshed to current Go test names; all 10 new names verified to exist.
  - id: FND-054-T5-02
    severity: low
    class: manifest-schema-violation
    status: closed
    summary: SCN-054-022 and SCN-054-024 e2e-ui linkedTests relinked from production source files to real live-stack Go web e2e tests (notification_operator_web_test.go, notification_security_web_test.go). requiredTestType preserved.
unresolvedFindings: []
evidence:
  - command: bash .github/bubbles/scripts/artifact-lint.sh specs/054-notification-intelligence-handler
    exitCode: 0
    result: PASSED
  - command: timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/054-notification-intelligence-handler
    exitCode: 0
    result: PASSED
nextOwner: bubbles.workflow
```
