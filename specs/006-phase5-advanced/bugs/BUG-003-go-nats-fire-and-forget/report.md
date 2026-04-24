# Report: BUG-003 — Go Runtime Fire-and-Forget NATS Publishes

## Discovery
- **Found by:** `bubbles.gaps` during stochastic quality sweep
- **Date:** April 22, 2026
- **Method:** Traced NATS publish calls in Go intelligence code; found publish-without-subscribe pattern

## Evidence
- `internal/intelligence/monthly.go:250`: publishes to `smk.monthly.generate` then immediately calls `assembleMonthlyReportText` — never awaits response
- `internal/intelligence/monthly.go:389`: publishes to `smk.content.analyze` then immediately generates template angles — never awaits response
- `internal/intelligence/learning.go`: no NATS publish at all — uses `classifyDifficultyHeuristic` only
- `internal/intelligence/lookups.go`: no NATS publish — stores pre-computed content
- `internal/intelligence/monthly.go::DetectSeasonalPatterns`: no NATS publish — SQL-only

### Summary

BUG-003 is **discovered and documented** but **the fix is not yet implemented**. The bug remains open; no code change has been applied to convert fire-and-forget `Publish()` calls into request/reply patterns, and no NATS publishes have been added to `learning.go`, `lookups.go`, or `DetectSeasonalPatterns`. The artifact-only repairs in this session are limited to (a) bringing the bug packet into compliance with the v3 state schema, (b) aligning top-level status with certification.status, and (c) adding the report's required sections so the lint gate can pass for the in-progress state. Implementation remains scope-01 work.

### Completion Statement

This bug is **NOT complete** and is **NOT promoted to `done`**.

- `state.json.status`: `in_progress`
- `state.json.certification.status`: `in_progress` (aligned with top-level)
- `state.json.policySnapshot`: present (added this session, repo defaults)
- `scopes.md` Scope 01 status: `Not Started`
- All 8 DoD items in Scope 01 remain unchecked

Verification this session confirms the original bug pattern is still present in production code:

```bash
$ grep -n "Request\|Publish" internal/intelligence/monthly.go | head -40
250:                    if pubErr := e.NATS.Publish(ctx, smacknats.SubjectMonthlyGenerate, data); pubErr != nil {
389:                            if pubErr := e.NATS.Publish(ctx, smacknats.SubjectContentAnalyze, data); pubErr != nil {

$ grep -n "Request\|Publish\|smk.learning\|smk.quickref\|smk.seasonal" internal/intelligence/learning.go internal/intelligence/lookups.go
(no output — no NATS interaction in either file)

$ grep -rn "Request(" internal/nats/
(no output — no Request method exists on the NATS client)
```

Promotion to `done` is forbidden until: (1) `internal/nats/` exposes a request/reply primitive, (2) the five intelligence call sites switch from `Publish` to that primitive (or add the missing publishes), (3) graceful local-fallback paths are exercised by unit tests, and (4) `./smackerel.sh test unit` passes with the new behavior. None of those steps have been performed.

### Test Evidence

Real test runs executed this session for the affected packages:

```bash
$ go test -count=1 ./internal/nats/ ./internal/intelligence/
ok      github.com/smackerel/smackerel/internal/nats    4.029s
ok      github.com/smackerel/smackerel/internal/intelligence    0.041s
```

These tests pass because they exercise the **current** (broken) fire-and-forget behavior; they do not yet cover the request/reply contract described by Scope 01's DoD. No new tests were added in this session — that is implementation-phase work and is intentionally deferred.

---

## Implementation Phase Evidence — April 24, 2026

**Phase:** implement
**Agent:** bubbles.implement (operator-driven session)
**Claim Source:** executed
**Status update:** code changes landed; bug is **not** closed (state.json remains `in_progress` per workflow gate ownership rules).

### Files modified

- `internal/nats/client.go` — added `Client.Request(ctx, subject, data, timeout) ([]byte, error)` using core `Conn.RequestWithContext`. SST-compliant: rejects `timeout <= 0` (no hidden default).
- `internal/nats/request_test.go` — new file. Unit tests for the four error guards (nil receiver, nil conn, zero timeout, negative timeout) plus two skip-by-default live-broker round-trip tests gated on `SMACKEREL_NATS_TEST_URL`.
- `internal/intelligence/monthly.go`
  - Added LLM timeout constants (`monthlyReportLLMTimeout=30s`, `contentAnalyzeLLMTimeout=15s`, `learningClassifyLLMTimeout=10s`, `quickrefGenerateLLMTimeout=15s`, `seasonalAnalyzeLLMTimeout=15s`) matching Scope 01 DoD.
  - Added reply structs (`monthlyGenerateReply`, `contentAnalyzeReply`, `learningClassifyReply`, `quickrefGenerateReply`, `seasonalAnalyzeReply`, `seasonalObservation`) for typed unmarshal.
  - `GenerateMonthlyReport` (line ~250 before): fire-and-forget `Publish(SubjectMonthlyGenerate)` → synchronous `Request(SubjectMonthlyGenerate, …, 30s)`; uses `report_text` reply when present, falls back to `assembleMonthlyReportText` on any error/empty body.
  - `GenerateContentFuel` (line ~389 before): fire-and-forget `Publish(SubjectContentAnalyze)` → synchronous `Request(…, 15s)` per topic; uses LLM-supplied title/rationale/format when both title and rationale are non-empty, otherwise falls back to the deterministic local angle. `SupportingIDs` are pinned to the request's artifact list (LLM cannot fabricate new IDs).
  - `DetectSeasonalPatterns`: added new `Request(SubjectSeasonalAnalyze, …, 15s)` after the local volume/topic patterns are computed. Appends LLM observations on top of local patterns; never replaces them.
- `internal/intelligence/learning.go`
  - Added `smacknats` import.
  - Added `normalizeDifficulty(s string) LearningDifficulty` helper that only accepts `beginner`/`intermediate`/`advanced` (case-insensitive, trimmed); anything else returns `""` so the local heuristic wins.
  - `GetLearningPaths`: per-resource `Request(SubjectLearningClassify, …, 10s)` when persisted `learning_progress.difficulty` is empty. Persisted difficulty still wins when present (prevents re-classifying completed work).
- `internal/intelligence/lookups.go`
  - Added `smacknats` import.
  - `CreateQuickReference`: `Request(SubjectQuickrefGenerate, …, 15s)` after argument validation. LLM-compiled `content` overrides the caller-supplied content when non-empty; truncated to `maxContentLen` to honor the existing storage cap. On any failure the caller-supplied content is preserved exactly as-is.
- `internal/intelligence/bug003_test.go` — new file. Tests for `normalizeDifficulty`, JSON tag bindings on all five reply structs, and a `NewEngine(nil, nil)` smoke test confirming the local fallbacks (`assembleMonthlyReportText`, `classifyDifficultyHeuristic`) still operate without NATS.

### Call-site conversion summary

| File | Function | Before | After | Timeout |
|------|----------|--------|-------|---------|
| `internal/intelligence/monthly.go` | `GenerateMonthlyReport` | `Publish(SubjectMonthlyGenerate)` (fire-and-forget) | `Request(SubjectMonthlyGenerate, …, 30s)` consuming `report_text` | 30s |
| `internal/intelligence/monthly.go` | `GenerateContentFuel` | `Publish(SubjectContentAnalyze)` (fire-and-forget, per topic) | `Request(SubjectContentAnalyze, …, 15s)` per topic, consuming `title`/`uniqueness_rationale`/`format_suggestion` | 15s/topic |
| `internal/intelligence/monthly.go` | `DetectSeasonalPatterns` | (no NATS interaction) | new `Request(SubjectSeasonalAnalyze, …, 15s)` appending LLM observations to local patterns | 15s |
| `internal/intelligence/learning.go` | `GetLearningPaths` | (no NATS interaction; heuristic only) | per-resource `Request(SubjectLearningClassify, …, 10s)` when persisted difficulty is empty | 10s/resource |
| `internal/intelligence/lookups.go` | `CreateQuickReference` | (no NATS interaction) | `Request(SubjectQuickrefGenerate, …, 15s)` overriding caller content with LLM-compiled body when present | 15s |

Zero call sites were left as `Publish`. Every Phase 5 LLM-delegation path now uses `Request` with an explicit timeout and a graceful local fallback.

### Real test output

```bash
$ go build ./...
(no output — clean build)

$ go vet ./...
(no output — clean vet)

$ go test -count=1 ./internal/nats/... ./internal/intelligence/...
ok      github.com/smackerel/smackerel/internal/nats    4.020s
ok      github.com/smackerel/smackerel/internal/intelligence    0.027s

$ go test -count=1 -v -run 'TestRequest_' ./internal/nats/...
=== RUN   TestRequest_NilClient
--- PASS: TestRequest_NilClient (0.00s)
=== RUN   TestRequest_NilConn
--- PASS: TestRequest_NilConn (0.00s)
=== RUN   TestRequest_ZeroTimeoutRejected
--- PASS: TestRequest_ZeroTimeoutRejected (0.00s)
=== RUN   TestRequest_NegativeTimeoutRejected
--- PASS: TestRequest_NegativeTimeoutRejected (0.00s)
=== RUN   TestRequest_HappyPath
    request_test.go:81: SMACKEREL_NATS_TEST_URL not set; skipping live NATS request/reply test
--- SKIP: TestRequest_HappyPath (0.00s)
=== RUN   TestRequest_TimeoutNoResponder
    request_test.go:112: SMACKEREL_NATS_TEST_URL not set; skipping live NATS request/reply test
--- SKIP: TestRequest_TimeoutNoResponder (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/nats    0.012s

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

The two live-broker round-trip cases (`TestRequest_HappyPath`, `TestRequest_TimeoutNoResponder`) skip when `SMACKEREL_NATS_TEST_URL` is unset, mirroring the existing pattern in `internal/nats/client_test.go` (which also avoids requiring a broker for unit-tier tests). They will execute against the live NATS instance during the `./smackerel.sh test integration` harness.

### Notes & limitations

- **Live-broker round-trip not exercised in this session.** No NATS container was running locally, so `TestRequest_HappyPath` and `TestRequest_TimeoutNoResponder` only have static-test coverage. Integration validation is deferred to the integration tier per `docs/Testing.md`.
- **ML sidecar reply contract is asymmetric today.** `ml/app/nats_client.py` currently publishes results via `SUBJECT_RESPONSE_MAP` (fan-out on `*.generated`/`*.analyzed`/`*.classified`), not via `msg.Respond()`. Until the sidecar mirrors the responder pattern (separate ML-side change), every `Request` call here will time out and the local fallback will run. That matches the bug spec's "fall back to local generation only if NATS publish fails or times out" requirement; the Go-side conversion stands on its own and is correct against the request/reply contract.
- **No `Publish` callsites kept as fire-and-forget.** All five Phase 5 LLM-delegation paths were converted. No call sites needed to remain `Publish`.
- **Pre-existing failing test (unrelated).** `internal/telegram` `TestSplitRateArgs` fails on `main` before any of these edits (verified by `git stash`/`git stash pop`). It is not introduced by this work.
- **DoD checkboxes intentionally left unchecked.** Per the operator instruction and the workflow ownership gate, this agent did not modify Scope 01 DoD checkboxes or `state.json` — those promotions belong to the certification chain.

```bash
$ go vet ./internal/nats/... ./internal/intelligence/...
(no output)
```
