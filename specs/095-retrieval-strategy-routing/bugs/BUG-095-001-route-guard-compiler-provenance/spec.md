# BUG-095-001 Spec — Spec-068 route-bypass guard false-positive

## Bug Statement

The spec-068 raw-route bypass guard test
(`TestIntentBypassGuardReportsRouterRouteWithoutCompiledIntent`) fails because
`internal/assistant/retrieval_strategy_routing.go` references the compiler's
OUTPUT type `intent.CompiledIntent` but never the literal `intent.Compiler`, so
the guard's compiler-reference heuristic does not recognise the file as
downstream of the intent compiler — even though it genuinely is (it routes the
already-compiled intent, opens no store, makes no second LLM call).

## Functional Requirements

- **FR-BUG-095-001-1**: `internal/assistant/retrieval_strategy_routing.go` MUST
  carry a TRUTHFUL machine-visible reference to the spec-068 `intent.Compiler`
  expressing the real provenance invariant (the router consumes the
  already-compiled intent; it never invokes the compiler).
- **FR-BUG-095-001-2**: `go test -tags integration -run
  TestIntentBypassGuardReportsRouterRouteWithoutCompiledIntent
  ./tests/integration/policy/` MUST pass (zero findings under
  `internal/assistant`), making the `CI` workflow's `integration` job green for
  this finding.
- **FR-BUG-095-001-3**: The fix MUST NOT change any runtime behavior, control
  flow, signatures, or routing logic, and MUST NOT edit the guard, its
  `AllowedRouteCallers` allowlist, `ScanSubdirs`, or any policy file.
- **FR-BUG-095-001-4**: The added reference MUST be factually TRUE — no
  misleading comment implying this file invokes/calls the compiler, and no dead
  `var _ intent.Compiler` / fake token used solely to satisfy the regex.

## Acceptance Scenarios

```gherkin
Scenario: SCN-BUG-095-001-1 Guard recognises the downstream router as compiler-provenanced
  Given retrieval_strategy_routing.go routes an already-compiled intent.CompiledIntent
  And its doc comment truthfully names the spec-068 intent.Compiler as the upstream producer
  When policyguard.ReportRawRouteBypasses scans internal/assistant
  Then retrieval_strategy_routing.go is NOT reported as a raw-route bypass
  And the guard returns zero findings under internal/assistant

Scenario: SCN-BUG-095-001-2 CI integration job is green for this finding
  Given the provenance comment is added
  When go test -tags integration -run TestIntentBypassGuard... ./tests/integration/policy/ runs
  Then the test passes
  And the CI workflow integration job no longer reports the spec-095 finding

Scenario: SCN-BUG-095-001-3 Guard still catches a real raw-text bypass
  Given the guard's planted fixture calls router.Route() with no intent.Compiler reference
  When policyguard.ReportRawRouteBypasses scans that fixture
  Then the fixture IS reported with "missing intent.Compiler step before Router.Route"
  And the adversarial baseline (same fixture WITH an intent.Compiler reference) is NOT reported
```

## Out of Scope

- The guard `policyguard.ReportRawRouteBypasses`, its `AllowedRouteCallers`
  allowlist, `ScanSubdirs`, and all policy files are correct and are NOT
  changed. Adding the file to the allowlist is FORBIDDEN — it would blind the
  guard to that file forever and is a cross-spec (spec 068) policy edit.
- The spec-095 router runtime behavior, control flow, and routing logic are
  unchanged.
- Spec 095 top-level `state.json` status / certification is unchanged.
- `policy-exception-baseline.json` is unchanged.
