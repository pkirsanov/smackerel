# BUG-069-002 User Validation

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [scenario-manifest.json](scenario-manifest.json)

## Checklist

All items verified with real command output captured in [scopes.md](scopes.md)
DoD evidence blocks and [report.md](report.md).

### Finding / Fix Acceptance

- [x] AV-01: The defect is real — the pre-fix capture call `a.capture(r.Context(), …)` runs the capture on a context `net/http` cancels on client disconnect; the production capture path (`newAssistantHTTPCaptureFn` → `pipeline.Process`) aborts its ctx-honoring Postgres `INSERT` / NATS publish on a cancelled context and only `slog.Error`s the loss.
- [x] AV-02: Pre-fix RED proof captured — `TestHTTPAdapter_CaptureSurvivesClientDisconnect` FAILS with `capture ran with a cancelled context (err=context canceled)` against `r.Context()`.
- [x] AV-03: Fix applied — `internal/assistant/httpadapter/adapter.go` now calls `a.capture(context.WithoutCancel(r.Context()), …)` with an explanatory comment citing F-069-CHAOS39-CAPTURE-CTX-CANCEL.
- [x] AV-04: Post-fix GREEN proof captured — the same test PASSES; the capture context is live (`ctx.Err() == nil`) and the prompt text + user id are preserved.
- [x] AV-05: No regression — the full `internal/assistant/httpadapter` package (existing `TestChaos069`, golden-contract, adapter, transport-hint + new regression) is green; capture is still invoked exactly once.
- [x] AV-06: Adversarial-signal guard green — `regression-quality-guard.sh --bugfix` reports 1 file with an adversarial signal, 0 violations, 0 warnings.
- [x] AV-07: Request-scoped values preserved — `context.WithoutCancel` retains parent Values, so `middleware.GetReqID` (artifact `TraceID`) and downstream request-scoped values survive.
- [x] AV-08: Change Boundary honored — only `internal/assistant/httpadapter/adapter.go` (one line + comment) and the new test file changed; Telegram adapter, schema, SST, pipeline, and other `specs/` folders untouched.
- [ ] AV-09: Status promoted to terminal-for-mode by validate-owned certification (workflow finalize step).

## Acceptance Summary

The inviolable capture-as-fallback contract now holds under client disconnect:
when the facade routes a turn to capture, the user's prompt is durably persisted
even if the client has already vanished. The fix is a single-line cancellation
decoupling proven by an adversarial RED→GREEN regression and validated against
the full HTTP-adapter package with no regression.

## One-To-One Finding Closure Accounting

- **F-069-CHAOS39-CAPTURE-CTX-CANCEL (MEDIUM):** closed by
  `a.capture(context.WithoutCancel(r.Context()), …)` +
  `TestHTTPAdapter_CaptureSurvivesClientDisconnect`. One finding, one root cause,
  one Scope 1; no deferral, no cherry-picking.
