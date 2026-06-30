# Report — BUG-069-003 Direct `/api/capture` capture lost on client disconnect

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

### Summary

A **bubbles.code-review MVP/evo-x2 readiness sweep** (finding **F-01**) surfaced
**F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL**: the direct capture endpoint
`POST /api/capture` is served by `internal/api/capture.go::CaptureHandler`, which
dispatched the durable pipeline write with the HTTP request context
(`d.Pipeline.Process(r.Context(), …)`). `net/http` cancels `r.Context()` on
client disconnect, and the production durable path
(`pipeline.Processor.Process` → `submitForProcessing` → `storeInitialArtifact`
Postgres `INSERT` + `NATS.Publish`) is context-honoring. A capture client
(extension / PWA / API caller) that disconnects after the body is received but
before the durable write completes therefore aborts `Process` with
`context.Canceled` and the capture is silently lost — violating the **inviolable**
capture-as-fallback contract (Hard Constraint 5, BS-001,
`policySnapshot.captureAsFallback: "inviolable"`).

This is the **same root cause** as the already-fixed
[BUG-069-002](../BUG-069-002-capture-context-cancellation/report.md)
(`/api/assistant/turn`), at the **direct** `/api/capture` endpoint that
BUG-069-002 never covered.

Fix: `d.Pipeline.Process(context.WithoutCancel(r.Context()), …)` plus the
previously-absent `"context"` import — the durable capture write is decoupled
from request cancellation while preserving request-scoped Values (request id /
trace correlation). One import + one call line + comment; proven by an adversarial
RED→GREEN regression in this session and a green `internal/api` package run (the
GREEN focused run compiled the whole module via `go test ./...` with no build
break).

### Code-Review Probe Evidence (read-only source inspection)

```text
# the direct capture endpoint dispatched the durable write on the request context (pre-fix):
internal/api/capture.go::CaptureHandler
  result, err := d.Pipeline.Process(r.Context(), &pipeline.ProcessRequest{ ... SourceID: pipeline.SourceCapture ... })

# the pipeline write is context-honoring (a cancelled ctx aborts it):
internal/pipeline/processor.go::submitForProcessing
  if err := p.storeInitialArtifact(ctx, artifactID, extracted, req, string(tier)); err != nil { ... }   # Postgres INSERT
  if err := p.NATS.Publish(ctx, smacknats.SubjectArtifactsProcess, data); err != nil { ... }             # NATS publish
```

`net/http` cancels `r.Context()` the instant the client connection drops, so the
durable `INSERT` / publish abort with `context.Canceled` and the capture is gone.
The Telegram path is not exposed (long-poll background context, not a per-request
HTTP context that dies on disconnect).

### Test Evidence

Focused unit RED→GREEN re-captured **this session** via the repo CLI
(`./smackerel.sh test unit --go --go-run 'TestCaptureHandler_CaptureSurvivesClientDisconnect' --verbose`),
which runs `go test -v -run … -count=1 ./...` inside the Dockerized Go runtime
(so the focused test executes amid a whole-module compile; non-matching packages
report `no tests to run`). The shared live-stack E2E disconnect-race regression
covering BOTH `/api/assistant/turn` and `/api/capture` against a real
`pipeline.Processor` (Postgres + NATS) is routed to `bubbles.test` (no live stack
this session — see Disposition).

**RED→GREEN technique (anti-fabrication disclosure):** to produce a genuine
*behavioral* RED rather than a mere unused-import compile error, the one durable
call site was temporarily reverted to `d.Pipeline.Process(r.Context(), …)` **and**
the now-unused `"context"` import was temporarily removed (its only use is
`context.WithoutCancel`). Immediately after capturing RED, both were restored
(`"context"` import re-added + `context.WithoutCancel(r.Context())` call) and
re-verified by re-reading the file (see [R2]); the GREEN run was then captured
against the restored fix.

### [R1] RED — pre-fix regression failure

**Command:** `./smackerel.sh test unit --go --go-run 'TestCaptureHandler_CaptureSurvivesClientDisconnect' --verbose`
(durable call temporarily reverted to `r.Context()`, `"context"` import temporarily removed) · **Exit Code:** 1 · **Claim Source:** executed

```text
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

### [R2] Fix diff

**Command:** `git --no-pager diff -- internal/api/capture.go` · **Claim Source:** executed

```text
diff --git a/internal/api/capture.go b/internal/api/capture.go
index cad8fe85..42647c4c 100644
--- a/internal/api/capture.go
+++ b/internal/api/capture.go
@@ -1,6 +1,7 @@
 package api

 import (
+	"context"
 	"encoding/json"
 	"errors"
 	"log/slog"
@@ -92,8 +93,21 @@ func (d *Dependencies) CaptureHandler(w http.ResponseWriter, r *http.Request) {
 		return
 	}

-	// Process the capture
-	result, err := d.Pipeline.Process(r.Context(), &pipeline.ProcessRequest{
+	// Process the capture.
+	//
+	// Capture-as-fallback is inviolable (Hard Constraint 5 / BS-001 /
+	// policySnapshot.captureAsFallback="inviolable"): the user's capture MUST
+	// persist even if the client has already disconnected. net/http cancels
+	// r.Context() the instant the connection drops, which would abort the
+	// downstream pipeline.Process Postgres INSERT (storeInitialArtifact) and
+	// NATS publish (submitForProcessing) — both context-honoring — and
+	// silently lose the capture. Decouple the durable write from request
+	// cancellation while preserving request-scoped values (request id / trace
+	// correlation via middleware.GetReqID, consumed by submitForProcessing for
+	// the artifact TraceID). Same root cause as BUG-069-002 (assistant
+	// /api/assistant/turn), here at the direct /api/capture endpoint —
+	// F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL.
+	result, err := d.Pipeline.Process(context.WithoutCancel(r.Context()), &pipeline.ProcessRequest{
 		URL:          req.URL,
 		Text:         req.Text,
 		VoiceURL:     req.VoiceURL,
```

Working-tree status: `M internal/api/capture.go` (fix), `?? internal/api/capture_disconnect_test.go` (new regression). Fix is uncommitted on disk and **intact**.

### [R3] GREEN — post-fix regression pass

**Command:** `./smackerel.sh test unit --go --go-run 'TestCaptureHandler_CaptureSurvivesClientDisconnect' --verbose`
(fix restored: `"context"` import + `context.WithoutCancel(r.Context())`) · **Exit Code:** 0 · **Claim Source:** executed

```text
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

### [R4] No build break — whole module compiles, `internal/api` package green

The GREEN focused run (R3) is driven by `go test -count=1 ./...` inside the
Dockerized runtime, which compiles the **entire** module before running the
`-run`-selected test. Exit code 0 with `ok  github.com/smackerel/smackerel/internal/api  0.087s`
proves the `internal/api` package (including the new
`capture_disconnect_test.go` regression) compiles and passes with no build break
anywhere in the module. **Claim Source:** executed (same run as R3).

### Completion Statement

The **bubbles.code-review MVP/evo-x2 readiness sweep** (F-01, parent-expanded —
no nested `runSubagent` in this runtime) discovered and **fixed**
F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL: the direct `/api/capture` durable write was
bound to `r.Context()` and silently dropped the user's capture on client
disconnect. The fix (`d.Pipeline.Process(context.WithoutCancel(r.Context()), …)`
+ the previously-absent `"context"` import in `internal/api/capture.go`) is
**landed and unit-verified** by an adversarial RED→GREEN regression re-captured
this session (R1 exit 1 → R3 exit 0), the fix diff (R2), and a green `internal/api`
package run that compiled the whole module (R4). One finding discovered, one
finding closed at the handler level — no cherry-picking.

This packet is **`in_progress`, not `done`**: the done-ceiling certification
requires a shared live-stack E2E disconnect-race regression (real HTTP client
abort against the bound `/api/assistant/turn` + `/api/capture` handlers + a real
`pipeline.Processor` with Postgres + NATS), the broader E2E regression suite, and
stress coverage — none runnable in this discovery/fix session. Those are
**routed** to `bubbles.test` → `bubbles.validate` (shared with BUG-069-002
fixSequence order-2) rather than fabricated. `nextRequiredOwner: bubbles.test`.

### Finding Ledger (one-to-one closure)

| Finding | Severity | Status | Closure |
|---------|----------|--------|---------|
| F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL | Medium | Closed (unit) | `d.Pipeline.Process(context.WithoutCancel(r.Context()), …)` + `"context"` import in `capture.go` + `TestCaptureHandler_CaptureSurvivesClientDisconnect` adversarial regression (RED→GREEN); `internal/api` package green |

One finding discovered, one finding closed. No deferral, no cherry-picking.

### Disposition

Scope 1 In Progress. The code fix + adversarial unit regression are complete and
green (R1→R4); the finding is closed at the handler level with no build break in
the module. The remaining done-ceiling certification — a **shared** live-stack
E2E disconnect-race regression covering BOTH `/api/assistant/turn` (BUG-069-002)
and `/api/capture` (this bug) against a real `pipeline.Processor` (Postgres +
NATS), the broader E2E regression suite, and stress coverage — is **routed** to
`bubbles.test` → `bubbles.validate` (state.json fixSequence order-2, shared with
BUG-069-002) and is not runnable in this session (no live stack). Status is held
at `in_progress` rather than fabricating E2E/stress evidence. The already-`done`
parent spec 069 artifacts and the BUG-069-002 packet are left untouched; this
finding and fix are recorded entirely in this bug packet.
