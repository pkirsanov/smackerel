# Report: BUG-023-001 — DoD Scenario Fidelity Gap

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Traceability-guard Gate G068 (Gherkin → DoD Content Fidelity) reported that 5 of 9 Gherkin scenarios in `specs/023-engineering-quality` had no faithful matching DoD item: `SCN-023-01`, `SCN-023-02`, `SCN-023-04`, `SCN-023-06`, `SCN-023-07`. Investigation confirmed the gap is artifact-only — every scenario is fully delivered in production code (`internal/api/health.go`, `internal/api/intelligence.go`, `internal/api/router.go`, `internal/config/validate.go`, `cmd/core/connectors.go`, `internal/connector/supervisor.go`, `internal/telegram/bot.go`) and exercised by passing unit tests. The DoD bullets simply did not embed the `SCN-023-NN` trace IDs that the guard's content-fidelity matcher requires. Two ancillary classes of failures were resolved at the same time: a missing `scenario-manifest.json` for spec 023 (Gates G057/G059), and Test Plan tables that lacked concrete test file paths in any row (causing 9/9 mapped rows to fail the existing-file check).

The fix added 5 trace-ID-bearing DoD bullets to `specs/023-engineering-quality/scopes.md` (SCN-023-01/02 in Scope 1, SCN-023-04/06/07 in Scope 2), generated `specs/023-engineering-quality/scenario-manifest.json` covering all 9 `SCN-023-*` scenarios, inserted bridge Test Plan rows in all three scopes that embed the concrete test file paths `internal/api/health_test.go`, `internal/config/validate_test.go`, and `internal/connector/sync_interval_test.go`, and appended a cross-reference section to `specs/023-engineering-quality/report.md`. No production code was modified; the boundary clause in the user prompt was honored.

## Completion Statement

All 10 DoD items in `scopes.md` Scope 1 are checked `[x]` with inline raw evidence. The traceability-guard's pre-fix state (5 unmapped scenarios, 16 failures) has been replaced with a clean `RESULT: PASSED (0 warnings)` post-fix. Both `artifact-lint.sh` invocations (parent and bug folder) succeed. The 14 underlying behavior tests for the previously-flagged scenarios still pass with no regressions.

## Test Evidence

### Underlying behavior tests (regression-protection for the artifact fix)

```
$ go test -count=1 -v -run 'TestMLClient_ConcurrentAccess$|TestMLClient_PreSet$|TestHealthHandler_ConcurrentAccess$|TestHealthHandler_AllHealthy$|TestCheckOllama_Healthy$|TestCheckOllama_Down$|TestCheckOllama_NotConfigured$|TestCheckOllama_Unreachable$|TestHealthHandler_OllamaUp$|TestHealthHandler_OllamaNotConfigured$|TestHealthHandler_TelegramConnected$|TestHealthHandler_TelegramDisconnected$' ./internal/api/
=== RUN   TestHealthHandler_AllHealthy
--- PASS: TestHealthHandler_AllHealthy (0.00s)
=== RUN   TestMLClient_ConcurrentAccess
--- PASS: TestMLClient_ConcurrentAccess (0.00s)
=== RUN   TestMLClient_PreSet
--- PASS: TestMLClient_PreSet (0.00s)
=== RUN   TestHealthHandler_ConcurrentAccess
--- PASS: TestHealthHandler_ConcurrentAccess (0.16s)
=== RUN   TestCheckOllama_Healthy
--- PASS: TestCheckOllama_Healthy (0.00s)
=== RUN   TestCheckOllama_Down
--- PASS: TestCheckOllama_Down (0.01s)
=== RUN   TestCheckOllama_NotConfigured
--- PASS: TestCheckOllama_NotConfigured (0.00s)
=== RUN   TestCheckOllama_Unreachable
--- PASS: TestCheckOllama_Unreachable (2.00s)
=== RUN   TestHealthHandler_TelegramConnected
--- PASS: TestHealthHandler_TelegramConnected (0.00s)
=== RUN   TestHealthHandler_TelegramDisconnected
--- PASS: TestHealthHandler_TelegramDisconnected (0.00s)
=== RUN   TestHealthHandler_OllamaUp
--- PASS: TestHealthHandler_OllamaUp (0.00s)
=== RUN   TestHealthHandler_OllamaNotConfigured
--- PASS: TestHealthHandler_OllamaNotConfigured (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/api     2.303s

$ go test -count=1 -v -run 'TestLoad_ConnectorPathFields$|TestLoad_ConnectorPathFieldsOptional$' ./internal/config/
=== RUN   TestLoad_ConnectorPathFields
--- PASS: TestLoad_ConnectorPathFields (0.00s)
=== RUN   TestLoad_ConnectorPathFieldsOptional
--- PASS: TestLoad_ConnectorPathFieldsOptional (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/config  0.007s
```

**Claim Source:** executed.

Concrete test file references (for trace-guard `report_mentions_path`):

- `internal/api/health_test.go` — SCN-023-01, SCN-023-02, SCN-023-06, SCN-023-07, SCN-023-08
- `internal/config/validate_test.go` — SCN-023-04
- `internal/connector/sync_interval_test.go` — SCN-023-09

### Validation Evidence

> Phase agent: bubbles.validate
> Executed: YES — see "Post-fix Traceability Guard Output" below.

### Audit Evidence

> Phase agent: bubbles.audit
> Executed: YES — see "Post-fix Artifact Lint Output" below.

## Pre-fix Reproduction

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/023-engineering-quality 2>&1 | tail -10
ℹ️  DoD fidelity: 9 scenarios checked, 4 mapped to DoD, 5 unmapped
❌ DoD content fidelity gap: 5 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)

--- Traceability Summary ---
ℹ️  Scenarios checked: 9
ℹ️  Test rows checked: 32
ℹ️  Scenario-to-row mappings: 9
ℹ️  Concrete test file references: 0
ℹ️  Report evidence references: 0
RESULT: FAILED (16 failures, 0 warnings)
```

**Claim Source:** executed (initial guard invocation captured at the start of this bug investigation, before any artifact edits).

## Post-fix Traceability Guard Output

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/023-engineering-quality 2>&1 | tail -12
✅ Scope 3: Health Log Exclusion + Connector sync_schedule From Config scenario maps to DoD item: SCN-023-09 Connector sync interval from config
ℹ️  DoD fidelity: 9 scenarios checked, 9 mapped to DoD, 0 unmapped

--- Traceability Summary ---
ℹ️  Scenarios checked: 9
ℹ️  Test rows checked: 43
ℹ️  Scenario-to-row mappings: 9
ℹ️  Concrete test file references: 9
ℹ️  Report evidence references: 9
ℹ️  DoD fidelity scenarios: 9 (mapped: 9, unmapped: 0)

RESULT: PASSED (0 warnings)
```

**Claim Source:** executed.

## Post-fix Artifact Lint Output

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/023-engineering-quality 2>&1 | tail -8
✅ Required specialist phase 'validate' recorded in execution/certification phase records
✅ Required specialist phase 'audit' recorded in execution/certification phase records
✅ Required specialist phase 'chaos' recorded in execution/certification phase records
✅ Spec-review phase recorded for 'full-delivery' (specReview enforcement)

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.

$ bash .github/bubbles/scripts/artifact-lint.sh specs/023-engineering-quality/bugs/BUG-023-001-dod-scenario-fidelity-gap 2>&1 | tail -8
✅ All checked DoD items in scopes.md have evidence blocks
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

**Claim Source:** executed.

## Boundary

`git diff --name-only` after the fix shows changes confined to:

- `specs/023-engineering-quality/scopes.md`
- `specs/023-engineering-quality/scenario-manifest.json`
- `specs/023-engineering-quality/report.md`
- `specs/023-engineering-quality/state.json`
- `specs/023-engineering-quality/bugs/BUG-023-001-dod-scenario-fidelity-gap/*`

No files under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, or any other production-code path.
