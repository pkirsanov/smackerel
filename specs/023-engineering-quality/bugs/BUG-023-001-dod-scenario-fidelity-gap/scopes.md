# Scopes: BUG-023-001 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → DoD trace-ID fidelity for spec 023

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-023-FIX-001 Trace guard accepts SCN-023-01/02/04/06/07 as faithfully covered
  Given specs/023-engineering-quality/scopes.md DoD entries that name each Gherkin scenario by ID
  And specs/023-engineering-quality/scenario-manifest.json mapping all 9 SCN-023-* scenarios
  And specs/023-engineering-quality/report.md referencing internal/api/health_test.go, internal/config/validate_test.go, internal/connector/sync_interval_test.go
  And each Test Plan row maps to an existing concrete test file path
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/023-engineering-quality`
  Then Gate G068 reports "9 scenarios checked, 9 mapped to DoD, 0 unmapped"
  And the overall result is PASSED
```

### Implementation Plan

1. Append SCN-023-01 DoD bullet (raw `go test` output for `TestMLClient_ConcurrentAccess`, `TestMLClient_PreSet`, `TestHealthHandler_ConcurrentAccess` + source pointer to `health.go::Dependencies.mlClient`/`mlClientOnce`) to Scope 1 DoD in `specs/023-engineering-quality/scopes.md`
2. Append SCN-023-02 DoD bullet (`go build ./...` exit 0 + `TestHealthHandler_AllHealthy` PASS + grep listing typed interfaces) to Scope 1 DoD
3. Append SCN-023-04 DoD bullet (raw output for `TestLoad_ConnectorPathFields`/`TestLoad_ConnectorPathFieldsOptional` + source pointer to `internal/config/validate.go` and `cmd/core/connectors.go`) to Scope 2 DoD
4. Append SCN-023-06 DoD bullet (raw output for `TestCheckOllama_Healthy/_Down/_NotConfigured/_Unreachable` and `TestHealthHandler_OllamaUp/_OllamaNotConfigured` + source pointer to `health.go::checkOllama`) to Scope 2 DoD
5. Append SCN-023-07 DoD bullet (raw output for `TestHealthHandler_TelegramConnected/_TelegramDisconnected` + source pointer to `health.go` `TelegramHealthChecker` and `internal/telegram/bot.go::Healthy`) to Scope 2 DoD
6. Generate `specs/023-engineering-quality/scenario-manifest.json` covering all 9 `SCN-023-*` scenarios with `linkedTests`, `evidenceRefs`, and `linkedDoD`
7. Insert bridge Test Plan rows (1 per scenario) at the top of each Scope's Test Plan table whose "Test" cell embeds the concrete test file path so the trace guard finds an existing file
8. Append a "BUG-023-001 — DoD Scenario Fidelity Gap" section to `specs/023-engineering-quality/report.md` with per-scenario classification, raw `go test` evidence, and full-path test file references
9. Run `bash .github/bubbles/scripts/artifact-lint.sh` against both the parent and bug folder; run `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/023-engineering-quality` and confirm PASS

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 9 mapped, 0 unmapped` | SCN-023-FIX-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/023-engineering-quality` | SCN-023-FIX-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/023-engineering-quality/bugs/BUG-023-001-dod-scenario-fidelity-gap` | SCN-023-FIX-001 |
| T-FIX-1-04 | Underlying behavior tests still pass | unit | `internal/api/health_test.go`, `internal/config/validate_test.go` | `go test -count=1 -v ./internal/api/ ./internal/config/` exit 0; the 13 named tests for SCN-023-01/02/04/06/07 all PASS | SCN-023-FIX-001 |

### Definition of Done

- [x] Scope 1 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-023-01` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-023-01" specs/023-engineering-quality/scopes.md` returns the new DoD bullet at the bottom of Scope 1 DoD; full raw test output recorded inline.
- [x] Scope 1 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-023-02` with inline `go build` + `TestHealthHandler_AllHealthy` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-023-02" specs/023-engineering-quality/scopes.md` returns the new DoD bullet; full raw build/test output recorded inline.
- [x] Scope 2 DoD in parent `scopes.md` contains bullets citing `Scenario SCN-023-04`, `SCN-023-06`, `SCN-023-07` with inline raw `go test` evidence — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-023-04\|Scenario SCN-023-06\|Scenario SCN-023-07" specs/023-engineering-quality/scopes.md` returns three matches in the Scope 2 DoD section; full raw test output recorded inline.
- [x] `specs/023-engineering-quality/scenario-manifest.json` exists and lists all 9 `SCN-023-*` scenarios — **Phase:** implement
  > Evidence: `grep -c '"scenarioId"' specs/023-engineering-quality/scenario-manifest.json` returns `9`.
- [x] `specs/023-engineering-quality/report.md` references `internal/api/health_test.go`, `internal/config/validate_test.go`, and `internal/connector/sync_interval_test.go` by full relative path — **Phase:** implement
  > Evidence: `grep -n "internal/api/health_test.go\|internal/config/validate_test.go\|internal/connector/sync_interval_test.go" specs/023-engineering-quality/report.md` returns matches in the new BUG-023-001 section.
- [x] Each Scope Test Plan has a bridge row per scenario whose "Test" cell embeds an existing concrete test file path — **Phase:** implement
  > Evidence: bridge rows like `TestMLClient_ConcurrentAccess in internal/api/health_test.go` placed at the top of each Scope's Test Plan table.
- [x] Underlying behavior tests still pass — **Phase:** test
  > Evidence:
  > ```
  > $ go test -count=1 -v -run 'TestMLClient_ConcurrentAccess$|TestMLClient_PreSet$|TestHealthHandler_ConcurrentAccess$|TestHealthHandler_AllHealthy$|TestCheckOllama_Healthy$|TestCheckOllama_Down$|TestCheckOllama_NotConfigured$|TestCheckOllama_Unreachable$|TestHealthHandler_OllamaUp$|TestHealthHandler_OllamaNotConfigured$|TestHealthHandler_TelegramConnected$|TestHealthHandler_TelegramDisconnected$' ./internal/api/
  > === RUN   TestHealthHandler_AllHealthy
  > --- PASS: TestHealthHandler_AllHealthy (0.00s)
  > === RUN   TestMLClient_ConcurrentAccess
  > --- PASS: TestMLClient_ConcurrentAccess (0.00s)
  > === RUN   TestMLClient_PreSet
  > --- PASS: TestMLClient_PreSet (0.00s)
  > === RUN   TestHealthHandler_ConcurrentAccess
  > --- PASS: TestHealthHandler_ConcurrentAccess (0.16s)
  > === RUN   TestCheckOllama_Healthy
  > --- PASS: TestCheckOllama_Healthy (0.00s)
  > === RUN   TestCheckOllama_Down
  > --- PASS: TestCheckOllama_Down (0.01s)
  > === RUN   TestCheckOllama_NotConfigured
  > --- PASS: TestCheckOllama_NotConfigured (0.00s)
  > === RUN   TestCheckOllama_Unreachable
  > --- PASS: TestCheckOllama_Unreachable (2.00s)
  > === RUN   TestHealthHandler_TelegramConnected
  > --- PASS: TestHealthHandler_TelegramConnected (0.00s)
  > === RUN   TestHealthHandler_TelegramDisconnected
  > --- PASS: TestHealthHandler_TelegramDisconnected (0.00s)
  > === RUN   TestHealthHandler_OllamaUp
  > --- PASS: TestHealthHandler_OllamaUp (0.00s)
  > === RUN   TestHealthHandler_OllamaNotConfigured
  > --- PASS: TestHealthHandler_OllamaNotConfigured (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/api     2.303s
  > $ go test -count=1 -v -run 'TestLoad_ConnectorPathFields$|TestLoad_ConnectorPathFieldsOptional$' ./internal/config/
  > === RUN   TestLoad_ConnectorPathFields
  > --- PASS: TestLoad_ConnectorPathFields (0.00s)
  > === RUN   TestLoad_ConnectorPathFieldsOptional
  > --- PASS: TestLoad_ConnectorPathFieldsOptional (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/config  0.007s
  > ```
- [x] Traceability-guard PASSES against `specs/023-engineering-quality` — **Phase:** validate
  > Evidence: see report.md `### Validation Evidence` for the full guard output.
- [x] Artifact-lint PASSES against parent and bug folder — **Phase:** validate
  > Evidence: see report.md `### Audit Evidence` for both runs.
- [x] No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/023-engineering-quality/scopes.md`, `specs/023-engineering-quality/report.md`, `specs/023-engineering-quality/scenario-manifest.json`, `specs/023-engineering-quality/state.json`, and `specs/023-engineering-quality/bugs/BUG-023-001-dod-scenario-fidelity-gap/*`. No files under `internal/`, `cmd/`, `ml/`, `config/` are touched.
