# Scopes: BUG-069-003 Decouple the direct `/api/capture` durable write from request cancellation

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Scope 1: Direct `/api/capture` survives client disconnect

**Status:** In Progress
**Depends On:** None
**Owner sequence:** `bubbles.code-review` (MVP/evo-x2 readiness sweep → finding F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL) → `bubbles.implement` (scenario-first regression test, RED; then the `context.WithoutCancel` fix + `context` import, GREEN) → `bubbles.test` (package regression + shared live-stack E2E) → validate-owned certification.
**Surfaces:** `internal/api/capture.go` (durable `Process` call site + `context` import), `internal/api/capture_disconnect_test.go` (new regression).

### Use Cases (Gherkin)

```gherkin
Scenario: BUG-069-003-SCN-001 The direct /api/capture durable write persists the capture when the client disconnects mid-request
  Given the /api/capture endpoint is served by CaptureHandler with a healthy DB and a recording pipeline
  And the HTTP client has already disconnected (the request context is cancelled)
  When the handler processes the POST /api/capture
  Then the durable pipeline.Process is invoked exactly once
  And the context handed to Process is NOT cancelled (ctx.Err() == nil)
  And the original prompt text is passed through intact

Scenario: BUG-069-003-SCN-002 Adversarial — the regression FAILS against r.Context() and PASSES after context.WithoutCancel
  Given the regression test cancels the request context before the durable write
  When the durable call uses d.Pipeline.Process(r.Context(), ...)
  Then the assertion that the durable-write context is live FAILS (ctx carries context.Canceled)
  And when the durable call uses d.Pipeline.Process(context.WithoutCancel(r.Context()), ...)
  Then the same assertion PASSES (ctx.Err() == nil) and the prompt text survives

Scenario: BUG-069-003-SCN-003 Shared live-stack E2E disconnect-race regression covers BOTH /api/assistant/turn and /api/capture
  Given a real pipeline.Processor backed by Postgres + NATS on the live stack
  When a real HTTP client connection is aborted mid-write against /api/assistant/turn AND against /api/capture
  Then the captured artifact is persisted despite the disconnect on BOTH endpoints
```

### Implementation Plan

- Author `internal/api/capture_disconnect_test.go`:
  `TestCaptureHandler_CaptureSurvivesClientDisconnect` — a `recordingPipeline`
  `Pipeliner` stub recording `ctx.Err()` / text, the established `mockDB` /
  `mockNATS` harness, and a `POST /api/capture` request whose context is cancelled
  before `CaptureHandler` runs the durable write. Asserts `Process` invoked + live
  context + prompt preserved. RED against the pre-fix code.
- Apply the fix in `internal/api/capture.go`: add the `"context"` import and swap
  `d.Pipeline.Process(r.Context(), …)` → `d.Pipeline.Process(context.WithoutCancel(r.Context()), …)`
  with the explanatory comment. GREEN.
- Run the `internal/api` package via the repo CLI to prove no regression and that
  the durable `Process` is still invoked exactly once.

### Test Plan

| Type | Test | Purpose | Scenarios |
|------|------|---------|-----------|
| Unit (Go) | `TestCaptureHandler_CaptureSurvivesClientDisconnect` | Durable write runs with a live context under client disconnect; prompt preserved | SCN-001 |
| Adversarial RED/GREEN (Go) | same test, run against `r.Context()` (RED) then `context.WithoutCancel(r.Context())` (GREEN) | Proves the regression is non-tautological and would catch a revert | SCN-002 |
| Regression (Go package) | `internal/api` package via `./smackerel.sh test unit --go` | `TestCaptureHandler_*` family stays green; durable Process invoked once; module compiles | SCN-001, SCN-002 |
| Regression E2E (live-stack, shared) | shared disconnect-race E2E covering `/api/assistant/turn` (BUG-069-002) + `/api/capture` (this bug) against a real `pipeline.Processor` (Postgres + NATS) | Capture persisted despite real client abort on BOTH endpoints | SCN-003 |

### Definition of Done — 3-Part Validation

- [x] **Root cause confirmed and documented** — the durable capture write was coupled to `r.Context()`; `net/http` cancels it on client disconnect; production `pipeline.Process` aborts its ctx-honoring Postgres `INSERT` / NATS publish and the capture is silently dropped.
   - Raw output evidence:

      **Phase:** code-review · **Owner:** bubbles.code-review · **Claim Source:** executed (read-only source inspection)
      ```
      internal/api/capture.go::CaptureHandler (pre-fix)
        result, err := d.Pipeline.Process(r.Context(), &pipeline.ProcessRequest{ ... SourceID: pipeline.SourceCapture ... })
      internal/pipeline/processor.go::submitForProcessing
        if err := p.storeInitialArtifact(ctx, artifactID, extracted, req, string(tier)); err != nil { ... }   # Postgres INSERT (ctx-honoring)
        if err := p.NATS.Publish(ctx, smacknats.SubjectArtifactsProcess, data); err != nil { ... }             # NATS publish (ctx-honoring)
      # net/http cancels r.Context() on client disconnect -> the durable write aborts with context.Canceled -> capture lost
      ```
- [x] **Pre-fix regression test FAILS (RED)** — the adversarial test is RED against `d.Pipeline.Process(r.Context(), …)` (the `"context"` import temporarily removed too, so the revert is the faithful pre-fix state, not an unused-import compile error).
   - Raw output evidence:

      **Phase:** implement · **Owner:** bubbles.implement · **Command:** `./smackerel.sh test unit --go --go-run 'TestCaptureHandler_CaptureSurvivesClientDisconnect' --verbose` (pre-fix `r.Context()`) · **Exit Code:** 1 · **Claim Source:** executed
      ```
      testing: warning: no tests to run
      PASS
      ok      github.com/smackerel/smackerel/internal/agent/userreply 0.019s [no tests to run]
      testing: warning: no tests to run
      PASS
      ok      github.com/smackerel/smackerel/internal/annotation      0.017s [no tests to run]
      === RUN   TestCaptureHandler_CaptureSurvivesClientDisconnect
          capture_disconnect_test.go:95: durable capture write ran with a cancelled context (err=context canceled); a client disconnect MUST NOT abort the /api/capture durable write (F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL)
      --- FAIL: TestCaptureHandler_CaptureSurvivesClientDisconnect (0.00s)
      FAIL
      FAIL    github.com/smackerel/smackerel/internal/api     0.190s
      testing: warning: no tests to run
      PASS
      ok      github.com/smackerel/smackerel/internal/api/admin/extensiondevices     0.008s [no tests to run]
      ...
      FAIL
      ===== RED run exit: 1 =====
      ```
- [x] **Fix implemented** — `internal/api/capture.go` uses `context.WithoutCancel(r.Context())` for the durable write and adds the previously-absent `"context"` import.
   - Raw output evidence:

      **Phase:** implement · **Owner:** bubbles.implement · **Command:** `git --no-pager diff -- internal/api/capture.go` · **Claim Source:** executed
      ```
      diff --git a/internal/api/capture.go b/internal/api/capture.go
      index cad8fe85..42647c4c 100644
      --- a/internal/api/capture.go
      +++ b/internal/api/capture.go
      @@ -1,6 +1,7 @@
       package api

       import (
      +	"context"
       	"encoding/json"
      @@ -92,8 +93,21 @@ func (d *Dependencies) CaptureHandler(w http.ResponseWriter, r *http.Request) {
      -	// Process the capture
      -	result, err := d.Pipeline.Process(r.Context(), &pipeline.ProcessRequest{
      +	// ... F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL explanatory comment ...
      +	result, err := d.Pipeline.Process(context.WithoutCancel(r.Context()), &pipeline.ProcessRequest{
      # working-tree status:  M internal/api/capture.go   ?? internal/api/capture_disconnect_test.go
      ```
- [x] **Post-fix regression test PASSES (GREEN)** — the same test passes after the fix is restored (`"context"` import + `context.WithoutCancel(r.Context())`).
   - Raw output evidence:

      **Phase:** implement · **Owner:** bubbles.implement · **Command:** `./smackerel.sh test unit --go --go-run 'TestCaptureHandler_CaptureSurvivesClientDisconnect' --verbose` · **Exit Code:** 0 · **Claim Source:** executed
      ```
      testing: warning: no tests to run
      PASS
      ok      github.com/smackerel/smackerel/internal/agent/userreply 0.016s [no tests to run]
      testing: warning: no tests to run
      PASS
      ok      github.com/smackerel/smackerel/internal/annotation      0.014s [no tests to run]
      === RUN   TestCaptureHandler_CaptureSurvivesClientDisconnect
      --- PASS: TestCaptureHandler_CaptureSurvivesClientDisconnect (0.00s)
      PASS
      ok      github.com/smackerel/smackerel/internal/api     0.087s
      testing: warning: no tests to run
      PASS
      ok      github.com/smackerel/smackerel/internal/api/admin/extensiondevices     0.003s [no tests to run]
      ...
      ===== GREEN run exit: 0 =====
      ```
- [x] **Adversarial regression case exists and would fail if the bug returned** — the test cancels the request context and asserts the durable-write context is live; reverting to `r.Context()` makes it RED (proven above, exit 1) and restoring makes it GREEN (exit 0).
   - Raw output evidence:

      **Phase:** test · **Owner:** bubbles.test · **Claim Source:** executed (RED above, GREEN above)
      ```
      # RED (pre-fix r.Context()):
      capture_disconnect_test.go:95: durable capture write ran with a cancelled context (err=context canceled); ...
      --- FAIL: TestCaptureHandler_CaptureSurvivesClientDisconnect (0.00s)
      # GREEN (context.WithoutCancel):
      --- PASS: TestCaptureHandler_CaptureSurvivesClientDisconnect (0.00s)
      ok      github.com/smackerel/smackerel/internal/api     0.087s
      ```
- [x] **Regression test contains no silent-pass bailout patterns** (no `t.Skip`, no `if cond { return }` early-exit).
   - Raw output evidence:

      **Phase:** test · **Owner:** bubbles.test · **Command:** `grep -nE 't\.Skip|if .*\{ *return *\}|return *$' internal/api/capture_disconnect_test.go` · **Exit Code:** 1 (no matches) · **Claim Source:** executed
      ```
      ===== bailout-scan exit: 1 (1 = no matches = clean) =====
      # the test asserts with t.Fatal/t.Fatalf/t.Errorf only; no skip/bailout/early-return paths
      ```
- [x] **No build break — whole module compiles, `internal/api` package green** — the GREEN run is `go test -count=1 ./...` inside Docker, compiling the entire module before running the selected test; exit 0 with `ok internal/api` proves the package (incl. the new regression) compiles and passes.
   - Raw output evidence:

      **Phase:** test · **Owner:** bubbles.test · **Command:** `./smackerel.sh test unit --go --go-run 'TestCaptureHandler_CaptureSurvivesClientDisconnect' --verbose` · **Exit Code:** 0 · **Claim Source:** executed
      ```
      --- PASS: TestCaptureHandler_CaptureSurvivesClientDisconnect (0.00s)
      PASS
      ok      github.com/smackerel/smackerel/internal/api     0.087s
      ===== GREEN run exit: 0 =====
      ```
- [x] **Change Boundary honored** — only `internal/api/capture.go` (import + the durable `Process` call + comment) + the new test file changed; response-path `d.DB.Healthy(r.Context())` / `writeJSON` / `writeError` stay request-scoped; no pipeline / Telegram / schema / SST / other-`specs/` edits.
   - Raw output evidence:

      **Phase:** implement · **Owner:** bubbles.implement · **Command:** `git --no-pager status --porcelain -- internal/api/capture.go internal/api/capture_disconnect_test.go` · **Claim Source:** executed
      ```
       M internal/api/capture.go
      ?? internal/api/capture_disconnect_test.go
      # no changes to internal/pipeline/* / Telegram adapter / wire schema / SST / config/smackerel.yaml / other specs/ folders
      ```
- [x] **The direct /api/capture durable write persists the capture when the client disconnects mid-request (BUG-069-003-SCN-001)** — at the handler contract level: `Process` is invoked exactly once with a non-cancelled context and the original prompt text is preserved when the request context is cancelled (proven by the unit regression GREEN above).
   - Raw output evidence:

      **Phase:** test · **Owner:** bubbles.test · **Command:** `./smackerel.sh test unit --go --go-run 'TestCaptureHandler_CaptureSurvivesClientDisconnect' --verbose` · **Exit Code:** 0 · **Claim Source:** executed
      ```
      --- PASS: TestCaptureHandler_CaptureSurvivesClientDisconnect (0.00s)
      ok      github.com/smackerel/smackerel/internal/api     0.087s
      # asserts: pipe.called==true, pipe.gotCtxErr==nil (live ctx), pipe.gotText=="durable idea"
      ```
- [x] **Adversarial RED→GREEN proof for the disconnect race (BUG-069-003-SCN-002)** — the regression FAILS against `d.Pipeline.Process(r.Context(), …)` (captured `context canceled`, exit 1) and PASSES after `context.WithoutCancel` (exit 0); reverting re-opens the finding.
   - Raw output evidence:

      **Phase:** test · **Owner:** bubbles.test · **Claim Source:** executed (RED exit 1 in [R1], GREEN exit 0 in [R3])
      ```
      # RED:  --- FAIL: TestCaptureHandler_CaptureSurvivesClientDisconnect (0.00s)  | FAIL internal/api 0.190s | exit 1
      # GREEN: --- PASS: TestCaptureHandler_CaptureSurvivesClientDisconnect (0.00s) | ok   internal/api 0.087s | exit 0
      ```
- [ ] **Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior** — the shared live-stack E2E disconnect-race regression (BUG-069-003-SCN-003) covering `/api/assistant/turn` + `/api/capture` against a real `pipeline.Processor` (Postgres + NATS). **ROUTED to `bubbles.test`** (state.json fixSequence order-2, shared with BUG-069-002); requires `./smackerel.sh up` + `./smackerel.sh test integration|e2e`; NOT runnable in the discovery/fix session.
   - Raw output evidence:
      ```
      [pending live-stack run — routed to bubbles.test; see state.json certification.blockers and fixSequence order-2]
      ```
- [ ] **Broader E2E regression suite passes** — **ROUTED to `bubbles.validate`** after the shared live-stack E2E lands. Status held at `in_progress` rather than fabricating E2E/stress evidence.
   - Raw output evidence:
      ```
      [pending — routed to bubbles.validate]
      ```
- [ ] **Done-ceiling certification (scenario-specific + broader E2E + stress)** — **ROUTED to `bubbles.validate`** after the live-stack E2E lands; validate writes the authoritative `certification.status`. NOT self-certified.
   - Raw output evidence:
      ```
      [pending — routed to bubbles.validate]
      ```

### Change Boundary

The fix stays strictly inside `internal/api/capture.go` (the `"context"` import +
the durable `Process` call line + comment) and the new
`internal/api/capture_disconnect_test.go`. Anything touching `internal/pipeline/*`,
the Telegram adapter, the wire schema, the SST contract, `config/smackerel.yaml`,
or any other `specs/` folder is out of boundary. The response-path
`d.DB.Healthy(r.Context())` re-check and all `writeJSON` / `writeError` calls stay
request-scoped.

### Rollback Contract

One import + one-line argument change plus a test; `git revert <SHA>` cleanly
restores prior behavior. No migration / topology / restart semantics. Reverting
re-opens F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL, which the regression test catches
(RED).

## One-To-One Finding Closure Accounting

- **F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL (MEDIUM, primary):** code-review readiness
  sweep finding (F-01) — the direct `/api/capture` durable write bound to
  `r.Context()`, lost on client disconnect. Closed at the handler level by
  `d.Pipeline.Process(context.WithoutCancel(r.Context()), …)` + the `"context"`
  import + the `TestCaptureHandler_CaptureSurvivesClientDisconnect` adversarial
  regression (RED→GREEN). One finding, one root cause, one Scope 1; no
  cherry-picking. The shared live-stack E2E (SCN-003) is consolidated with
  BUG-069-002 fixSequence order-2 — one regression covering BOTH endpoints.
