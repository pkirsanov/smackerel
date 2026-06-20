# Scopes: BUG-069-002 Decouple assistant HTTP capture-as-fallback from request cancellation

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Scope 1: Capture-as-fallback survives client disconnect

**Status:** In Progress
**Depends On:** None
**Owner sequence:** `bubbles.chaos` (probe → finding F-069-CHAOS39-CAPTURE-CTX-CANCEL) → `bubbles.implement` (scenario-first regression test, RED; then the `context.WithoutCancel` fix, GREEN) → `bubbles.test` (full-package regression + adversarial-signal guard) → validate-owned certification.
**Surfaces:** `internal/assistant/httpadapter/adapter.go` (capture call site), `internal/assistant/httpadapter/capture_disconnect_test.go` (new regression).

### Use Cases (Gherkin)

```gherkin
Scenario: BUG-069-002-SCN-001 Capture-as-fallback persists the prompt when the client disconnects mid-turn
  Given the HTTP adapter is enabled and the facade returns AssistantResponse{CaptureRoute: true}
  And the HTTP client has already disconnected (the request context is cancelled)
  When the adapter handles the turn
  Then the capture path is invoked exactly once
  And the context handed to the capture path is NOT cancelled (ctx.Err() == nil)
  And the original prompt text and resolved user id are passed through intact

Scenario: BUG-069-002-SCN-002 Adversarial — the regression FAILS against r.Context() and PASSES after context.WithoutCancel
  Given the regression test cancels the request context before the capture write
  When the capture call uses a.capture(r.Context(), ...)
  Then the assertion that the capture context is live FAILS (ctx carries context.Canceled)
  And when the capture call uses a.capture(context.WithoutCancel(r.Context()), ...)
  Then the same assertion PASSES (ctx.Err() == nil) and the prompt text/user id survive
```

### Implementation Plan

- Author `internal/assistant/httpadapter/capture_disconnect_test.go`:
  `TestHTTPAdapter_CaptureSurvivesClientDisconnect` — a facade stub returning
  `CaptureRoute=true`, a capture stub recording `ctx.Err()` / userID / text, a
  request with a shared-token session whose context is cancelled before
  `ServeHTTP`. Asserts capture invoked + live context + prompt/user preserved.
  RED against the pre-fix code.
- Apply the one-line fix in `internal/assistant/httpadapter/adapter.go`:
  `a.capture(r.Context(), …)` → `a.capture(context.WithoutCancel(r.Context()), …)`
  with the explanatory comment. GREEN.
- Run the full `internal/assistant/httpadapter` package (existing `TestChaos069`,
  golden-contract, adapter, transport-hint + the new regression) to prove no
  regression and that capture is still invoked exactly once.

### Test Plan

| Type | Test | Purpose | Scenarios |
|------|------|---------|-----------|
| Unit (Go) | `TestHTTPAdapter_CaptureSurvivesClientDisconnect` | Capture runs with a live context under client disconnect; prompt/user preserved | SCN-001 |
| Adversarial RED/GREEN (Go) | same test, run against `r.Context()` (RED) then `context.WithoutCancel(r.Context())` (GREEN) | Proves the regression is non-tautological and would catch a revert | SCN-002 |
| Regression (Go package) | full `internal/assistant/httpadapter` package | Existing `TestChaos069` + golden + adapter + transport-hint stay green; capture-once unchanged | SCN-001, SCN-002 |
| Quality guard | `regression-quality-guard.sh --bugfix capture_disconnect_test.go` | Adversarial signal present; no bailout/optional-assertion patterns | SCN-002 |

### Definition of Done — 3-Part Validation

- [x] **Root cause confirmed and documented** — capture-as-fallback coupled to `r.Context()`; `net/http` cancels it on client disconnect; production `pipeline.Process` aborts its ctx-honoring Postgres `INSERT` / NATS publish and the prompt is silently dropped.
   - Raw output evidence:

      **Phase:** chaos · **Owner:** bubbles.chaos · **Claim Source:** executed (read-only source inspection)
      ```
      $ grep -n 'a.capture(' internal/assistant/httpadapter/adapter.go      # pre-fix
      a.capture(r.Context(), userID, req.TransportMessageID, req.Text)
      $ grep -n 'Process(ctx' cmd/core/wiring_assistant_facade.go
      _, err := svc.proc.Process(ctx, &pipeline.ProcessRequest{Text: text, SourceID: pipeline.SourceCapture})
      $ grep -nE 'storeInitialArtifact\(ctx|NATS.Publish\(ctx' internal/pipeline/processor.go
      if err := p.storeInitialArtifact(ctx, artifactID, extracted, req, string(tier)); err != nil {
      if err := p.NATS.Publish(ctx, smacknats.SubjectArtifactsProcess, data); err != nil {
      # both honor ctx -> a cancelled r.Context() aborts the durable capture write
      ```
- [x] **Pre-fix regression test FAILS (RED)** — the adversarial test is RED against `a.capture(r.Context(), …)`.
   - Raw output evidence:

      **Phase:** implement · **Owner:** bubbles.implement · **Command:** `./smackerel.sh test unit --go --go-run 'TestHTTPAdapter_CaptureSurvivesClientDisconnect' --verbose` (against the pre-fix `r.Context()` call) · **Exit Code:** 1 · **Claim Source:** executed
      ```
      [go-unit] applying -run selector: TestHTTPAdapter_CaptureSurvivesClientDisconnect
      + go test -v -run TestHTTPAdapter_CaptureSurvivesClientDisconnect -count=1 ./...
      === RUN   TestHTTPAdapter_CaptureSurvivesClientDisconnect
          capture_disconnect_test.go:97: capture ran with a cancelled context (err=context canceled); a client disconnect MUST NOT abort the durable capture write (F-069-CHAOS39-CAPTURE-CTX-CANCEL)
      --- FAIL: TestHTTPAdapter_CaptureSurvivesClientDisconnect (0.00s)
      FAIL    github.com/smackerel/smackerel/internal/assistant/httpadapter   0.028s
      ```
- [x] **Fix implemented** — `internal/assistant/httpadapter/adapter.go` uses `context.WithoutCancel(r.Context())` for the capture call.
   - Raw output evidence:

      **Phase:** implement · **Owner:** bubbles.implement · **Command:** `git --no-pager diff -- internal/assistant/httpadapter/adapter.go` · **Claim Source:** executed
      ```
      -		a.capture(r.Context(), userID, req.TransportMessageID, req.Text)
      +		// Capture-as-fallback is inviolable (Hard Constraint 5 / BS-001):
      +		// the user's prompt MUST persist even if the client has already
      +		// disconnected. net/http cancels r.Context() the instant the
      +		// connection drops, which would abort the downstream
      +		// pipeline.Process Postgres INSERT / NATS publish and silently
      +		// lose the prompt. Decouple the durable capture write from
      +		// request cancellation while preserving request-scoped values
      +		// (request id, trace correlation via middleware.GetReqID).
      +		// Spec 069 chaos Round 39 — F-069-CHAOS39-CAPTURE-CTX-CANCEL.
      +		a.capture(context.WithoutCancel(r.Context()), userID, req.TransportMessageID, req.Text)
      ```
- [x] **Post-fix regression test PASSES (GREEN)** — the same test passes after the swap.
   - Raw output evidence:

      **Phase:** implement · **Owner:** bubbles.implement · **Command:** `./smackerel.sh test unit --go --go-run 'TestHTTPAdapter_CaptureSurvivesClientDisconnect' --verbose` · **Exit Code:** 0 · **Claim Source:** executed
      ```
      [go-unit] applying -run selector: TestHTTPAdapter_CaptureSurvivesClientDisconnect
      + go test -v -run TestHTTPAdapter_CaptureSurvivesClientDisconnect -count=1 ./...
      --- PASS: TestHTTPAdapter_CaptureSurvivesClientDisconnect (0.00s)
      ok      github.com/smackerel/smackerel/internal/assistant/httpadapter   0.021s
      ```
- [x] **Adversarial regression case exists and would fail if the bug returned** — the test cancels the request context and asserts the capture context is live; reverting to `r.Context()` makes it RED (proven above). `regression-quality-guard --bugfix` confirms an adversarial signal with no bailout patterns.
   - Raw output evidence:

      **Phase:** test · **Owner:** bubbles.test · **Command:** `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/assistant/httpadapter/capture_disconnect_test.go` · **Exit Code:** 0 · **Claim Source:** executed
      ```
        BUBBLES REGRESSION QUALITY GUARD
        Bugfix mode: true
      ℹ️  Scanning internal/assistant/httpadapter/capture_disconnect_test.go
      ✅ Adversarial signal detected in internal/assistant/httpadapter/capture_disconnect_test.go
        REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
        Files scanned: 1
        Files with adversarial signals: 1
      ```
- [x] **No regression — full `internal/assistant/httpadapter` package passes (capture still invoked exactly once)** — existing `TestChaos069`, golden-contract, adapter, and transport-hint tests stay green alongside the new regression.
   - Raw output evidence:

      **Phase:** test · **Owner:** bubbles.test · **Command:** `./smackerel.sh test unit --go --go-run 'TestHTTPA|TestChaos069|TestTransportHint' --verbose` · **Exit Code:** 0 · **Claim Source:** executed
      ```
      ok      github.com/smackerel/smackerel/internal/assistant/httpadapter   0.157s
      ok      github.com/smackerel/smackerel/internal/assistant/intent/policyguard   0.027s [no tests to run]
      EXIT_PIPELINE=0
      # package covers: TestHTTPAdapterTranslatesTextTurnToAssistantMessage, TestHTTPAdapter_Validate*,
      # TestHTTPAdapter_CaptureSurvivesClientDisconnect (new), TestChaos069, TestHTTPAssistantTurnGoldenContractV1,
      # TestTransportHintIsClosedVocabularyAndTelemetryOnly — no FAIL line
      ```
- [x] **Regression test contains no silent-pass bailout patterns** (no `if cond { return }` early-exit / no `t.Skip`).
   - Raw output evidence:

      **Phase:** test · **Owner:** bubbles.test · **Command:** `grep -nE 't\.Skip|if .*\{ *return *\}|url\(\)\.includes' internal/assistant/httpadapter/capture_disconnect_test.go` · **Exit Code:** 1 (no matches) · **Claim Source:** executed
      ```
      (no matches — the test has no skip/bailout/early-return patterns; it asserts with t.Fatal/t.Fatalf/t.Errorf only)
      ```
- [x] **Change Boundary honored** — only `internal/assistant/httpadapter/adapter.go` + the new test file changed; no Telegram adapter, schema, SST, pipeline, or other `specs/` edits.
   - Raw output evidence:

      **Phase:** implement · **Owner:** bubbles.implement · **Command:** `git --no-pager diff --stat -- internal/assistant/httpadapter/adapter.go internal/assistant/httpadapter/capture_disconnect_test.go` · **Claim Source:** executed
      ```
      internal/assistant/httpadapter/adapter.go            | (1 line swapped + comment)
      internal/assistant/httpadapter/capture_disconnect_test.go | (new file, regression)
      # no changes to middleware.go / late_binding.go / schema.go / Telegram adapter / internal/pipeline / config/smackerel.yaml
      ```
- [x] **Capture-as-fallback persists the prompt when the client disconnects mid-turn (BUG-069-002-SCN-001)** — at the adapter contract level: capture is invoked exactly once with a non-cancelled context and the original prompt text + resolved user id are preserved when the request context is cancelled (proven by the unit regression above, R1→R3).
   - Raw output evidence:

      **Phase:** test · **Owner:** bubbles.test · **Command:** `./smackerel.sh test unit --go --go-run 'TestHTTPAdapter_CaptureSurvivesClientDisconnect' --verbose` · **Exit Code:** 0 · **Claim Source:** executed
      ```
      --- PASS: TestHTTPAdapter_CaptureSurvivesClientDisconnect (0.00s)
      ok      github.com/smackerel/smackerel/internal/assistant/httpadapter   0.021s
      # asserts: captureCalled==true, captureCtxErr==nil (live ctx), captureText=="remember to file taxes", captureUser=="disconnect-user"
      ```
- [x] **Adversarial RED→GREEN proof for the disconnect race (BUG-069-002-SCN-002)** — the regression FAILS against `a.capture(r.Context(), …)` (captured `context canceled`) and PASSES after `context.WithoutCancel`; reverting re-opens the finding.
   - Raw output evidence:

      **Phase:** test · **Owner:** bubbles.test · **Claim Source:** executed (RED in R1, GREEN in R3)
      ```
      # RED (pre-fix r.Context()):
      capture_disconnect_test.go:97: capture ran with a cancelled context (err=context canceled); ...
      --- FAIL: TestHTTPAdapter_CaptureSurvivesClientDisconnect (0.00s)
      # GREEN (context.WithoutCancel):
      --- PASS: TestHTTPAdapter_CaptureSurvivesClientDisconnect (0.00s)
      ```
- [ ] **Live-stack E2E disconnect-race regression (scenario-specific) PASSES** — drive a real HTTP request whose client connection is aborted mid-turn against the bound adapter + a real `pipeline.Processor` (Postgres + NATS) and assert the captured artifact is persisted despite the disconnect. **ROUTED to `bubbles.test`** — requires `./smackerel.sh up` + `./smackerel.sh test integration|e2e`; NOT runnable in the discovery/fix session (no live stack).
   - Raw output evidence:
      ```
      [pending live-stack run — routed to bubbles.test; see state.json certification.blockers]
      ```
- [ ] **Broader E2E regression suite green + done-ceiling certification (scenario-specific + broader E2E + stress)** — **ROUTED to `bubbles.validate`** after the live-stack E2E lands. Status held at `in_progress` rather than fabricating E2E/stress evidence.
   - Raw output evidence:
      ```
      [pending — routed to bubbles.validate]
      ```

### Change Boundary

The fix stays strictly inside `internal/assistant/httpadapter/adapter.go` (the
capture call line + comment) and the new
`internal/assistant/httpadapter/capture_disconnect_test.go`. Anything touching
the Telegram adapter, `middleware.go`, `late_binding.go`, `schema.go`,
`internal/pipeline/*`, `config/smackerel.yaml`, or any other `specs/` folder is
out of boundary.

### Rollback Contract

Single-line change plus a test; `git revert <SHA>` cleanly restores prior
behavior. No migration / topology / restart semantics. Reverting re-opens
F-069-CHAOS39-CAPTURE-CTX-CANCEL, which the regression test catches (RED).

## One-To-One Finding Closure Accounting

- **F-069-CHAOS39-CAPTURE-CTX-CANCEL (MEDIUM, primary):** chaos Round 39 finding —
  capture-as-fallback bound to `r.Context()`, lost on client disconnect. Closed
  by `a.capture(context.WithoutCancel(r.Context()), …)` + the
  `TestHTTPAdapter_CaptureSurvivesClientDisconnect` adversarial regression
  (RED→GREEN). One finding, one root cause, one Scope 1; no cherry-picking.
