# Report — BUG-069-002 Assistant HTTP capture-as-fallback lost on client disconnect

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

### Summary

Stochastic-quality-sweep **Round 39** (`chaos-hardening` on
`specs/069-assistant-http-transport`) probed the assistant HTTP transport for
resilience / race / edge-case failures and surfaced **F-069-CHAOS39-CAPTURE-CTX-CANCEL**:
`HTTPAdapter.ServeHTTP` renders capture-as-fallback by calling
`a.capture(r.Context(), …)`. `net/http` cancels `r.Context()` on client
disconnect, and the production capture path
(`newAssistantHTTPCaptureFn` → `pipeline.Processor.Process`) runs a
context-honoring Postgres `INSERT` + NATS publish. A client that disconnects
between the `Facade.Handle` decision (`CaptureRoute=true`) and the capture write
therefore causes the write to abort with `context.Canceled`, and the user's
prompt is silently dropped — violating the **inviolable** capture-as-fallback
contract (Hard Constraint 5, BS-001, `policySnapshot.captureAsFallback: "inviolable"`).

Fix: `a.capture(context.WithoutCancel(r.Context()), …)` — the durable capture
write is decoupled from request cancellation while preserving request-scoped
Values (request id / trace correlation). One line + comment; proven by an
adversarial RED→GREEN regression and validated against the full HTTP-adapter
package with no regression.

### Chaos Probe Evidence (read-only source inspection)

```text
# capture-as-fallback is rendered on the request context (pre-fix):
internal/assistant/httpadapter/adapter.go
  if resp.CaptureRoute {
      a.capture(r.Context(), userID, req.TransportMessageID, req.Text)
  }

# production CaptureFn dispatches into the pipeline with that ctx and only logs on failure:
cmd/core/wiring_assistant_facade.go::newAssistantHTTPCaptureFn
  _, err := svc.proc.Process(ctx, &pipeline.ProcessRequest{Text: text, SourceID: pipeline.SourceCapture})
  if err != nil { slog.Error("assistant HTTP capture: pipeline.Process failed", ...) }   # <- prompt lost, no retry

# the pipeline write is context-honoring (a cancelled ctx aborts it):
internal/pipeline/processor.go::submitForProcessing
  if err := p.storeInitialArtifact(ctx, artifactID, extracted, req, string(tier)); err != nil { ... }   # Postgres INSERT
  if err := p.NATS.Publish(ctx, smacknats.SubjectArtifactsProcess, data); err != nil { ... }             # NATS publish
```

The Telegram adapter is not exposed: it processes updates from its long-poll
loop under a background context, not a per-request HTTP context that dies on
client disconnect.

### Test Evidence

Unit-level RED→GREEN + full-package regression + adversarial-signal guard for
F-069-CHAOS39-CAPTURE-CTX-CANCEL. Live-stack E2E disconnect-race regression is
routed to `bubbles.test` (no live stack this session — see Disposition).

### [R1] RED — pre-fix regression failure

**Command:** `./smackerel.sh test unit --go --go-run 'TestHTTPAdapter_CaptureSurvivesClientDisconnect' --verbose`
(against the pre-fix `a.capture(r.Context(), …)`) · **Exit Code:** 1

```text
[go-unit] applying -run selector: TestHTTPAdapter_CaptureSurvivesClientDisconnect
+ go test -v -run TestHTTPAdapter_CaptureSurvivesClientDisconnect -count=1 ./...
=== RUN   TestHTTPAdapter_CaptureSurvivesClientDisconnect
    capture_disconnect_test.go:97: capture ran with a cancelled context (err=context canceled); a client disconnect MUST NOT abort the durable capture write (F-069-CHAOS39-CAPTURE-CTX-CANCEL)
--- FAIL: TestHTTPAdapter_CaptureSurvivesClientDisconnect (0.00s)
FAIL    github.com/smackerel/smackerel/internal/assistant/httpadapter   0.028s
```

### [R2] Fix diff

**Command:** `git --no-pager diff -- internal/assistant/httpadapter/adapter.go`

```text
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

### [R3] GREEN — post-fix regression pass

**Command:** `./smackerel.sh test unit --go --go-run 'TestHTTPAdapter_CaptureSurvivesClientDisconnect' --verbose` · **Exit Code:** 0

```text
[go-unit] applying -run selector: TestHTTPAdapter_CaptureSurvivesClientDisconnect
+ go test -v -run TestHTTPAdapter_CaptureSurvivesClientDisconnect -count=1 ./...
--- PASS: TestHTTPAdapter_CaptureSurvivesClientDisconnect (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant/httpadapter   0.021s
```

### [R4] No regression — full httpadapter package

**Command:** `./smackerel.sh test unit --go --go-run 'TestHTTPA|TestChaos069|TestTransportHint' --verbose` · **Exit Code:** 0

```text
ok      github.com/smackerel/smackerel/internal/assistant/httpadapter   0.157s
ok      github.com/smackerel/smackerel/internal/assistant/intent/policyguard   0.027s [no tests to run]
EXIT_PIPELINE=0
# package covers TestHTTPAdapterTranslatesTextTurnToAssistantMessage, TestHTTPAdapter_Validate*,
# TestHTTPAdapter_CaptureSurvivesClientDisconnect (new), TestChaos069,
# TestHTTPAssistantTurnGoldenContractV1, TestTransportHintIsClosedVocabularyAndTelemetryOnly — no FAIL line
```

### [R5] Adversarial-signal / regression-quality guard

**Command:** `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/assistant/httpadapter/capture_disconnect_test.go` · **Exit Code:** 0

```text
  BUBBLES REGRESSION QUALITY GUARD
  Bugfix mode: true
ℹ️  Scanning internal/assistant/httpadapter/capture_disconnect_test.go
✅ Adversarial signal detected in internal/assistant/httpadapter/capture_disconnect_test.go
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 1
  Files with adversarial signals: 1
```

### Completion Statement

chaos-hardening Round 39 (parent-expanded, no nested `runSubagent`) discovered
and **fixed** F-069-CHAOS39-CAPTURE-CTX-CANCEL: the assistant HTTP
capture-as-fallback path was bound to `r.Context()` and silently dropped the
user's prompt on client disconnect. The single-line fix
(`a.capture(context.WithoutCancel(r.Context()), …)` in
`internal/assistant/httpadapter/adapter.go`) is **landed and unit-verified** by an
adversarial RED→GREEN regression (R1, R3), the fix diff (R2), a green full
`internal/assistant/httpadapter` package run (R4), and a green adversarial-signal
regression-quality guard (R5). One finding discovered, one finding closed at the
adapter contract level — no cherry-picking.

This packet is **`in_progress`, not `done`**: the done-ceiling certification
requires a live-stack E2E disconnect-race regression (real HTTP client abort
against the bound adapter + a real `pipeline.Processor` with Postgres + NATS),
the broader E2E regression suite, and stress coverage — none runnable in this
discovery/fix session. Those are **routed** to `bubbles.test` → `bubbles.validate`
rather than fabricated. `nextRequiredOwner: bubbles.test`.

### Finding Ledger (one-to-one closure)

| Finding | Severity | Status | Closure |
|---------|----------|--------|---------|
| F-069-CHAOS39-CAPTURE-CTX-CANCEL | Medium | Closed | `a.capture(context.WithoutCancel(r.Context()), …)` in `adapter.go` + `TestHTTPAdapter_CaptureSurvivesClientDisconnect` adversarial regression (RED→GREEN); full httpadapter package green |

One finding discovered, one finding closed. No deferral, no cherry-picking.

### Disposition

Scope 1 In Progress. The code fix + adversarial unit regression are complete and
green (R1–R5); the finding is closed at the adapter contract level with no
regression in the HTTP-adapter package. The remaining done-ceiling certification
— a live-stack E2E disconnect-race regression (real HTTP client abort against the
bound adapter + a real `pipeline.Processor` with Postgres + NATS), the broader
E2E regression suite, and stress coverage — is **routed** to `bubbles.test` →
`bubbles.validate` and is not runnable in this discovery/fix session (no live
stack). Status is held at `in_progress` rather than fabricating E2E/stress
evidence. The already-`done` parent spec 069 artifacts are left untouched; the
finding and fix are recorded entirely in this bug packet.
