# BUG-069-003 User Validation

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [scenario-manifest.json](scenario-manifest.json)

## Checklist

All items verified with real command output captured in [scopes.md](scopes.md)
DoD evidence blocks and [report.md](report.md).

### Finding / Fix Acceptance

- [x] AV-01: The defect is real — the pre-fix call `d.Pipeline.Process(r.Context(), …)` runs the durable write on a context `net/http` cancels on client disconnect; the production durable path (`pipeline.Processor.Process` → `submitForProcessing` → `storeInitialArtifact` Postgres `INSERT` + `NATS.Publish`) aborts on a cancelled context and the capture is silently lost.
- [x] AV-02: Pre-fix RED proof captured — `TestCaptureHandler_CaptureSurvivesClientDisconnect` FAILS with `durable capture write ran with a cancelled context (err=context canceled)` against `r.Context()`.
- [x] AV-03: Fix applied — `internal/api/capture.go` now calls `d.Pipeline.Process(context.WithoutCancel(r.Context()), …)` and adds the previously-absent `"context"` import, with an explanatory comment citing F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL.
- [x] AV-04: Post-fix GREEN proof captured — the same test PASSES; the durable-write context is live (`ctx.Err() == nil`) and the prompt text is preserved.
- [x] AV-05: No regression — the `internal/api` package (`TestCaptureHandler_*` family + new regression) is green; the durable `Process` is still invoked exactly once.
- [x] AV-06: Request-scoped values preserved — `context.WithoutCancel` retains parent Values, so `middleware.GetReqID` (artifact `TraceID`) and downstream request-scoped values survive.
- [x] AV-07: Change Boundary honored — only `internal/api/capture.go` (import + the durable `Process` call line + comment) and the new test file changed; response-path `d.DB.Healthy(r.Context())` / `writeJSON` / `writeError` stay request-scoped; Telegram, pipeline, schema, SST, and other `specs/` folders untouched.
- [x] AV-08: Capture.go fix line confirmed intact after RED→GREEN capture — `d.Pipeline.Process(context.WithoutCancel(r.Context()), …)` with the `"context"` import restored (re-read verified).
- [ ] AV-09: Status promoted to terminal-for-mode by validate-owned certification after the shared live-stack E2E disconnect-race regression (covering `/api/assistant/turn` + `/api/capture`) lands. Routed to `bubbles.test` → `bubbles.validate`.

## Acceptance Summary

The inviolable capture-as-fallback contract now holds under client disconnect at
the **direct** `/api/capture` endpoint: a `POST /api/capture` durably persists the
user's content even if the client has already vanished. The fix is the same
single-call cancellation decoupling already ratified for the assistant endpoint
(BUG-069-002) — here `context.WithoutCancel` plus the previously-absent `context`
import — proven by an adversarial RED→GREEN regression and validated against the
`internal/api` package with no regression. Live-stack E2E + done-ceiling
certification remain pending and are routed honestly.

## One-To-One Finding Closure Accounting

- **F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL (MEDIUM):** closed at the handler level
  by `d.Pipeline.Process(context.WithoutCancel(r.Context()), …)` + the `"context"`
  import + `TestCaptureHandler_CaptureSurvivesClientDisconnect`. One finding, one
  root cause, one Scope 1; no deferral, no cherry-picking.
