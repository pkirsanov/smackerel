# Bug: BUG-002-001 — Traceability gaps in spec 002 (manifest coverage + report evidence + Test Plan row)

## Classification

- **Type:** Artifact-only documentation/traceability bug
- **Severity:** MEDIUM (governance gate failure on a feature already marked `done`; no runtime impact)
- **Parent Spec:** 002 — Phase 1: Foundation
- **Workflow Mode:** bugfix-fastlane
- **Status:** Fixed (artifact-only)

## Problem Statement

Bubbles `traceability-guard` reports 12 failures on `specs/002-phase1-foundation`, all of them documentation/linkage gaps on a feature whose runtime is already delivered, certified, and `status: done`:

1. **Scenario manifest under-coverage (1 failure, gates G057/G059):** `scenario-manifest.json` lists only `SCN-002-001`–`SCN-002-044` (44 entries) but `scopes.md` defines 82 Gherkin scenarios (`SCN-002-001`–`SCN-002-082`). Scopes 9–25 (improvement, system-review, and gap-closure scopes added after the original manifest was generated) added 38 scenarios that were never appended.
2. **Missing report evidence references (10 failures):** `report.md` does not contain the literal path strings `internal/scheduler/scheduler_test.go` (scope 14 ×2, scope 15 ×4), `internal/auth/oauth_test.go` (scope 18 ×3), or `internal/connector/supervisor_test.go` (scope 19 ×1). The guard's `report_mentions_path` is a literal-substring match, and all four test files exist on disk and pass.
3. **Missing Test Plan row for SCN-002-080 (1 failure):** Scope 24 (Image OCR Pipeline) has Gherkin scenarios `SCN-002-079` and `SCN-002-080` but the Test Plan table only has one row referencing `SCN-002-079`. `SCN-002-080` (graceful OCR fallback) has no traceable Test Plan row even though `ml/tests/test_ocr.py::TestExtractTextTesseract::test_returns_empty_on_exception` already exercises the fallback path.

## Reproduction (Pre-fix)

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/002-phase1-foundation 2>&1 | grep -E "^❌|RESULT:"
❌ scenario-manifest.json covers only 44 scenarios but scopes define 82
❌ Scope 14: Scheduler Data Race Fix report is missing evidence reference for concrete test file: internal/scheduler/scheduler_test.go
❌ Scope 14: Scheduler Data Race Fix report is missing evidence reference for concrete test file: internal/scheduler/scheduler_test.go
❌ Scope 15: Scheduler Test Coverage report is missing evidence reference for concrete test file: internal/scheduler/scheduler_test.go
❌ Scope 15: Scheduler Test Coverage report is missing evidence reference for concrete test file: internal/scheduler/scheduler_test.go
❌ Scope 15: Scheduler Test Coverage report is missing evidence reference for concrete test file: internal/scheduler/scheduler_test.go
❌ Scope 15: Scheduler Test Coverage report is missing evidence reference for concrete test file: internal/scheduler/scheduler_test.go
❌ Scope 18: Auth Decryption Fallback Logging report is missing evidence reference for concrete test file: internal/auth/oauth_test.go
❌ Scope 18: Auth Decryption Fallback Logging report is missing evidence reference for concrete test file: internal/auth/oauth_test.go
❌ Scope 18: Auth Decryption Fallback Logging report is missing evidence reference for concrete test file: internal/auth/oauth_test.go
❌ Scope 19: Supervisor Sleep Context Cancellation report is missing evidence reference for concrete test file: internal/connector/supervisor_test.go
❌ Scope 24: Image OCR Pipeline scenario has no traceable Test Plan row: SCN-002-080 Image OCR graceful fallback on failure
RESULT: FAILED (12 failures, 0 warnings)
```

## Gap Analysis

All 12 failures are **delivered-but-undocumented** at the trace-ID/path level. None of them indicate a missing test or missing implementation:

| Scope | Behavior delivered? | Tests pass? | Concrete test file (already on disk) |
|---|---|---|---|
| 14 | Yes — `mu sync.Mutex` field in `Scheduler` struct; protected reads/writes around `digestPendingRetry`/`digestPendingDate` | Yes — `TestSCN002058_MutexProtectsRetryFields`, `TestSCN002059_RetryClearsOnSuccess` PASS | `internal/scheduler/scheduler_test.go` |
| 15 | Yes — cron-entry registration, nil digestGen guard, concurrent retry access (race-detector clean), retry-field lifecycle | Yes — `TestSCN002060_CronEntries`, `TestSCN002061_NilDigestGenGuard`, `TestSCN002062_ConcurrentRetryAccess`, `TestSCN002063_RetryFieldLifecycle` PASS | `internal/scheduler/scheduler_test.go` |
| 18 | Yes — `slog.Warn` on each of three decrypt fallback paths (base64 decode, short data, GCM open failure) | Yes — `TestTokenStore_Decrypt_FailClosed_NotBase64`, `TestTokenStore_Decrypt_FailClosed_TooShort`, `TestTokenStore_Decrypt_FailClosed_GCMFailure` PASS | `internal/auth/oauth_test.go` |
| 19 | Yes — `time.Sleep(5s)` in `runWithRecovery` replaced with `select { case <-parentCtx.Done(): return; case <-time.After(5*time.Second): }` | Yes — `TestPanicRecovery_RestartAfterPanic`, `TestPanicRecovery_SkipsRestartWhenStopped` PASS (cover the supervisor recovery path) | `internal/connector/supervisor_test.go` |
| 24 | Yes — `_handle_artifact_process` in `ml/app/nats_client.py` wraps OCR download/extract in try/except; logs warning and falls back to URL-only text on failure | Yes — `ml/tests/test_ocr.py::TestExtractTextTesseract::test_returns_empty_on_exception` exercises the fallback (returns empty string on bad bytes), and the existing scope-24 row already references `ml/app/nats_client.py` | `ml/tests/test_ocr.py` (new Test Plan row added by this fix) |

For the manifest gap: the missing 38 scenarios (`SCN-002-045`–`SCN-002-082`) all map to scopes 9–25 of `scopes.md` and to test functions / files that already exist on disk and pass. No scenario has a missing test.

**Disposition:** All 12 failures are **artifact-only**. The fix is a documentation-only edit; production code is not touched.

## Acceptance Criteria

- [x] `specs/002-phase1-foundation/scenario-manifest.json` covers all 82 Gherkin scenarios with `scenarioId`, `scope`, `requiredTestType`, `linkedTests`, `evidenceRefs`, and `linkedDoD`
- [x] `specs/002-phase1-foundation/report.md` literally contains the strings `internal/scheduler/scheduler_test.go`, `internal/auth/oauth_test.go`, and `internal/connector/supervisor_test.go`
- [x] `specs/002-phase1-foundation/scopes.md` Scope 24 Test Plan table has a row mapping `SCN-002-080` to `ml/tests/test_ocr.py`
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/002-phase1-foundation` PASS
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/002-phase1-foundation/bugs/BUG-002-001-traceability-gaps` PASS
- [x] `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/002-phase1-foundation` PASS
- [x] No production code changed (boundary: zero changes under `internal/`, `cmd/`, `ml/app/`, `config/`, `tests/`)
