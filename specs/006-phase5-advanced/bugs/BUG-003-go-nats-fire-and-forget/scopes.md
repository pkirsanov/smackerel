# Scopes: BUG-003 — Go Runtime Fire-and-Forget NATS Publishes

## Scope 01: Convert Phase 5 NATS to Request/Reply

**Status:** Done
**Priority:** P1
**Depends On:** BUG-001 (ML sidecar handlers must exist first)

### Definition of Done
- [x] `GenerateMonthlyReport` uses request/reply for LLM-generated report text with 30s timeout

  **Evidence:**
  ```bash
  $ grep -n "Request" internal/intelligence/monthly.go | head -3
  304:    // BUG-003: convert fire-and-forget Publish to synchronous Request so the
  312:                   reply, reqErr := e.NATS.Request(ctx, smacknats.SubjectMonthlyGenerate, data, monthlyReportLLMTimeout)
  ```
  Timeout constant `monthlyReportLLMTimeout = 30s` is declared in `internal/intelligence/monthly.go` and passed at the call site above.

- [x] `GenerateContentFuel` uses request/reply for LLM writing angles with 15s timeout

  **Evidence:**
  ```bash
  $ grep -n "Request(ctx, smacknats.SubjectContentAnalyze" internal/intelligence/monthly.go
  466:                           reply, reqErr := e.NATS.Request(ctx, smacknats.SubjectContentAnalyze, data, contentAnalyzeLLMTimeout)
  ```
  `contentAnalyzeLLMTimeout = 15s` is declared in `internal/intelligence/monthly.go` and applied per topic.

- [x] `GetLearningPaths` publishes to `smk.learning.classify` for LLM difficulty with 10s timeout

  **Evidence:**
  ```bash
  $ grep -n "Request(ctx, smacknats.SubjectLearningClassify" internal/intelligence/learning.go
  137:                          reply, reqErr := e.NATS.Request(ctx, smacknats.SubjectLearningClassify, data, learningClassifyLLMTimeout)
  ```
  `learningClassifyLLMTimeout = 10s`. Persisted `learning_progress.difficulty` still wins when present (no needless re-classification).

- [x] `CreateQuickReference` publishes to `smk.quickref.generate` for LLM compilation with 15s timeout

  **Evidence:**
  ```bash
  $ grep -n "Request(ctx, smacknats.SubjectQuickrefGenerate" internal/intelligence/lookups.go
  133:                   reply, reqErr := e.NATS.Request(ctx, smacknats.SubjectQuickrefGenerate, data, quickrefGenerateLLMTimeout)
  ```
  `quickrefGenerateLLMTimeout = 15s`. LLM-compiled body overrides caller content when non-empty; truncated to `maxContentLen`.

- [x] `DetectSeasonalPatterns` publishes to `smk.seasonal.analyze` for LLM commentary with 15s timeout

  **Evidence:**
  ```bash
  $ grep -n "Request(ctx, smacknats.SubjectSeasonalAnalyze" internal/intelligence/monthly.go
  613:                   reply, reqErr := e.NATS.Request(ctx, smacknats.SubjectSeasonalAnalyze, data, seasonalAnalyzeLLMTimeout)
  ```
  `seasonalAnalyzeLLMTimeout = 15s`. LLM observations append to (never replace) local SQL-derived patterns.

- [x] All 5 features gracefully fall back to local generation on NATS timeout/failure

  **Evidence:** Each call site checks `reqErr != nil` (or empty/invalid reply body) and falls through to the deterministic local path:
  - `monthly.go` `GenerateMonthlyReport` → `assembleMonthlyReportText`
  - `monthly.go` `GenerateContentFuel` → deterministic local angle (`SupportingIDs` pinned to request artifacts)
  - `monthly.go` `DetectSeasonalPatterns` → local volume/topic patterns kept; LLM observations only appended on success
  - `learning.go` `GetLearningPaths` → `classifyDifficultyHeuristic`
  - `lookups.go` `CreateQuickReference` → caller-supplied content preserved verbatim

  Verified by `TestNilNATS_FallbackPaths` in `internal/intelligence/bug003_test.go` (PASS — see next item).

- [x] Unit tests verify fallback behavior when NATS is nil or unavailable

  **Evidence:**
  ```bash
  $ go test -count=1 -v -run 'TestNormalizeDifficulty|TestLLMReplyShapes|TestNilNATS_FallbackPaths' ./internal/intelligence/...
  === RUN   TestNormalizeDifficulty_KnownLabels
  --- PASS: TestNormalizeDifficulty_KnownLabels (0.00s)
  === RUN   TestLLMReplyShapes
  === RUN   TestLLMReplyShapes/monthly
  === RUN   TestLLMReplyShapes/content
  === RUN   TestLLMReplyShapes/learning
  === RUN   TestLLMReplyShapes/quickref
  === RUN   TestLLMReplyShapes/seasonal
  --- PASS: TestLLMReplyShapes (0.00s)
  === RUN   TestNilNATS_FallbackPaths
  --- PASS: TestNilNATS_FallbackPaths (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/internal/intelligence    0.022s
  ```
  Plus `internal/nats` Request guard tests:
  ```bash
  $ go test -count=1 -v -run 'TestRequest_' ./internal/nats/...
  === RUN   TestRequest_NilClient
  --- PASS: TestRequest_NilClient (0.00s)
  === RUN   TestRequest_NilConn
  --- PASS: TestRequest_NilConn (0.00s)
  === RUN   TestRequest_ZeroTimeoutRejected
  --- PASS: TestRequest_ZeroTimeoutRejected (0.00s)
  === RUN   TestRequest_NegativeTimeoutRejected
  --- PASS: TestRequest_NegativeTimeoutRejected (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/internal/nats    0.012s
  ```

- [x] `./smackerel.sh test unit` passes

  **Evidence:**
  ```bash
  $ ./smackerel.sh test unit 2>&1 | tail -3
  -- Docs: https://docs.pytest.org/en/stable/how-to/capture-warnings.html
  330 passed, 2 warnings in 13.66s
  ```
  Go side: all 40+ packages `ok` (cached, full listing in `report.md` → Test Evidence). Exit code 0.
