# Bug: BUG-023-001 — DoD scenario fidelity gap (SCN-023-01/02/04/06/07)

## Classification

- **Type:** Artifact-only documentation/traceability bug
- **Severity:** MEDIUM (governance gate failure on a feature already marked `done`; no runtime impact)
- **Parent Spec:** 023 — Engineering Quality
- **Workflow Mode:** bugfix-fastlane
- **Status:** Fixed (artifact-only)

## Problem Statement

Bubbles traceability-guard Gate G068 (Gherkin → DoD Content Fidelity) reported that 5 of the 9 Gherkin scenarios in the parent feature's `scopes.md` had no faithful matching DoD item:

- `SCN-023-01` Concurrent health checks are race-free
- `SCN-023-02` Typed Dependencies catch method signature changes at compile time
- `SCN-023-04` Connector paths flow through config.Config (SST)
- `SCN-023-06` Ollama health reflects actual reachability
- `SCN-023-07` Telegram bot health reflects actual connection state

The gate's content-fidelity matcher requires a DoD bullet to either (a) carry the same `SCN-023-NN` trace ID as the Gherkin scenario, or (b) share enough significant words. The pre-existing DoD entries described the implemented behavior but did not embed the trace ID, and the fuzzy matcher's significant-word threshold was not satisfied for these five scenarios. Two ancillary classes of failures piggybacked on the same gap:

1. No `scenario-manifest.json` had been generated for spec 023 (Gates G057/G059 reported "Resolved scopes define 9 Gherkin scenarios but scenario-manifest.json is missing").
2. None of the 32 Test Plan rows across the three scopes contained a concrete existing test file path (the existing tables used a `Type | Test | Purpose | Scenarios Covered` schema with no `Location` column), so all 9 scenario-to-row mappings failed the "concrete test file path" check.

## Reproduction (Pre-fix)

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

## Gap Analysis (per scenario)

For each unmapped scenario the bug investigator searched the production code (`internal/api/health.go`, `internal/api/intelligence.go`, `internal/api/router.go`, `internal/config/validate.go`, `cmd/core/connectors.go`, `internal/connector/supervisor.go`, `internal/telegram/bot.go`) and the test files (`internal/api/health_test.go`, `internal/config/validate_test.go`, `internal/connector/sync_interval_test.go`). All five behaviors are genuinely **delivered-but-undocumented at the trace-ID level** — there is no missing implementation and no missing test fixture; the only gap is that DoD bullets did not embed the `SCN-023-NN` ID that the guard uses for fidelity matching, and Test Plan rows did not embed concrete test file paths the row-existence check uses.

| Scenario | Behavior delivered? | Tests pass? | Concrete test file | Concrete source |
|---|---|---|---|---|
| SCN-023-01 | Yes — `Dependencies.mlClientOnce sync.Once` guards lazy creation of the shared `*http.Client` so 50 concurrent `mlClient()` callers receive the same pointer with no race; the health handler is invoked through a `sync.Once`-guarded init path. | Yes — `TestMLClient_ConcurrentAccess`, `TestMLClient_PreSet`, `TestHealthHandler_ConcurrentAccess` PASS | `internal/api/health_test.go` | `internal/api/health.go::Dependencies.mlClient`, `mlClientOnce` |
| SCN-023-02 | Yes — `Dependencies` struct fields for `Pipeline`, `SearchEngine`, `DigestGen`, `WebHandler`, `OAuthHandler` are now named interfaces (`Pipeliner`, `Searcher`, `DigestGenerator`, `WebUI`, `OAuthFlow`); a method-signature change in any of them now fails compilation rather than panicking at runtime. | Yes — `go build ./...` exit 0 against the typed `Dependencies` definitions; `TestHealthHandler_AllHealthy` constructs `Dependencies` against the typed interfaces and PASSes | `internal/api/health_test.go` | `internal/api/health.go` (interface declarations + `Dependencies` struct) |
| SCN-023-04 | Yes — `config.Config` exposes `BookmarksImportDir`, `BrowserHistoryPath`, `MapsImportDir`; `cmd/core/connectors.go` reads them from `cfg.*` (zero raw `os.Getenv` for these keys in `cmd/`); `config.Load` populates them from the env-var pipeline emitted by `scripts/commands/config.sh`. | Yes — `TestLoad_ConnectorPathFields`, `TestLoad_ConnectorPathFieldsOptional` PASS | `internal/config/validate_test.go` | `internal/config/validate.go`, `cmd/core/connectors.go` |
| SCN-023-06 | Yes — `checkOllama(client, url)` performs a live `GET {url}/api/tags` with a 2-second timeout and returns `up`/`down`/`not_configured`/`unreachable` based on the real response; the health handler reports the live status instead of the previous hardcoded `"unavailable"`. | Yes — `TestCheckOllama_Healthy`, `TestCheckOllama_Down`, `TestCheckOllama_NotConfigured`, `TestCheckOllama_Unreachable`, `TestHealthHandler_OllamaUp`, `TestHealthHandler_OllamaNotConfigured` PASS | `internal/api/health_test.go` | `internal/api/health.go::checkOllama` |
| SCN-023-07 | Yes — `TelegramHealthChecker` interface exposes `Healthy() bool`; `internal/telegram/bot.go::Bot.Healthy()` returns the actual connection state; the health handler returns `connected` / `disconnected` based on `d.TelegramBot.Healthy()` instead of the previous hardcoded `"disconnected"`. | Yes — `TestHealthHandler_TelegramConnected`, `TestHealthHandler_TelegramDisconnected` PASS | `internal/api/health_test.go` | `internal/api/health.go` (`TelegramHealthChecker`), `internal/telegram/bot.go::Healthy` |

**Disposition:** All five scenarios are **delivered-but-undocumented** — artifact-only fix.

## Acceptance Criteria

- [x] Parent `specs/023-engineering-quality/scopes.md` Scope 1 DoD has bullets that explicitly contain `SCN-023-01` and `SCN-023-02` with raw `go test` (and `go build`) evidence and source-file pointers
- [x] Parent `specs/023-engineering-quality/scopes.md` Scope 2 DoD has bullets that explicitly contain `SCN-023-04`, `SCN-023-06`, `SCN-023-07` with raw `go test` evidence and source-file pointers
- [x] Parent `specs/023-engineering-quality/scenario-manifest.json` exists and covers all 9 scenarios with `scenarioId`, `linkedTests`, `evidenceRefs`, and `linkedDoD`
- [x] Each scope's Test Plan has at least one row per Gherkin scenario whose cells include a concrete existing test file path (so the trace guard's "concrete test file path" check passes)
- [x] Parent `specs/023-engineering-quality/report.md` references the concrete test files `internal/api/health_test.go`, `internal/config/validate_test.go`, `internal/connector/sync_interval_test.go` by full relative path
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/023-engineering-quality` PASS
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/023-engineering-quality/bugs/BUG-023-001-dod-scenario-fidelity-gap` PASS
- [x] `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/023-engineering-quality` PASS
- [x] No production code changed (boundary)
