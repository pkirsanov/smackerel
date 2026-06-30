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
# Direct capture endpoint: CaptureHandler dispatches the durable pipeline write.
# Pre-fix it passed r.Context() (cancelled on client disconnect); the committed
# fix now dispatches on context.WithoutCancel(r.Context()) at line 110:
$ grep -n 'CaptureHandler\|d.Pipeline.Process' internal/api/capture.go
57:// CaptureHandler handles POST /api/capture.
58:func (d *Dependencies) CaptureHandler(w http.ResponseWriter, r *http.Request) {
110:	result, err := d.Pipeline.Process(context.WithoutCancel(r.Context()), &pipeline.ProcessRequest{

# The downstream pipeline write is context-honoring — a cancelled ctx aborts the
# Postgres INSERT (storeInitialArtifact, line 431/499) and the NATS publish (line 471):
$ grep -n 'storeInitialArtifact\|p.NATS.Publish' internal/pipeline/processor.go
431:	if err := p.storeInitialArtifact(ctx, artifactID, extracted, req, string(tier)); err != nil {
471:	if err := p.NATS.Publish(ctx, smacknats.SubjectArtifactsProcess, data); err != nil {
499:func (p *Processor) storeInitialArtifact(ctx context.Context, id string, result *extract.Result, req *ProcessRequest, tier string) error {
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
`pipeline.Processor` (Postgres + NATS) PASSED on a real pg+nats stack (captured
2026-06-30; ARTIFACTS LastSeq 0→1; adversarial raw-cancel persists 0 rows) — see
the Live-Stack Durable Regression section below.

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

**Command:** `git --no-pager show --stat d395d00c -- internal/api/capture.go` · **Exit Code:** 0 · **Claim Source:** executed

```text
$ git --no-pager show --stat d395d00c -- internal/api/capture.go
commit d395d00c1b8e021291b57c91655f5fdee03583cd
Author: Philippe Kirsanov <pkirsanov@gmail.com>
Date:   Tue Jun 30 02:14:05 2026 -0700

    fix(api): decouple /api/capture durable write from request cancellation (BUG-069-003)

 internal/api/capture.go | 18 ++++++++++++++++--
 1 file changed, 16 insertions(+), 2 deletions(-)

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

Working-tree status when captured during the fix session: `M internal/api/capture.go` (fix), `?? internal/api/capture_disconnect_test.go` (new regression). The fix is now **committed at `d395d00c`** (working tree clean) — the committed `git show` is in the Code Diff Evidence section below.

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

### Validation Evidence

Done-ceiling validation: the shared live-stack durable regression
`TestCaptureDisconnectDurability_ProcessorSurvivesClientCancel` PASSED on a real
`pipeline.Processor` + Postgres + NATS stack. The committing run is recorded in
git — the stores-only integration helper landed and the durability lane went
green in commit `f00a2132` (shared order-2 deliverable with BUG-069-002):

```text
$ git --no-pager show --stat f00a2132 -- tests/integration/capture_disconnect_durability_test.go
commit f00a2132caca179b397185a139d3fb6370c21c70
Author: Philippe Kirsanov <pkirsanov@gmail.com>
Date:   Tue Jun 30 11:36:51 2026 -0700

    test(integration): stores-only schema+stream provisioning helper; durability green (BUG-099-002 done)

    Proven (independent re-run): the capture-durability integration test PASSES on the live pg+nats
    stack — both sub-tests green (durable persist despite client disconnect, ARTIFACTS LastSeq 0->1;
    adversarial raw-cancelled-ctx persists 0 rows, fix is load-bearing).

 tests/integration/capture_disconnect_durability_test.go | 8 ++++++++
 1 file changed, 8 insertions(+)
```

Captured live-stack PASS (real Postgres+NATS; same shared order-2 run as BUG-069-002):

```text
$ ./smackerel.sh --env test test integration-light --go-run TestCaptureDisconnectDurability_ProcessorSurvivesClientCancel
Smackerel pre-flight resource check: OK
  RAM  available: 13711 MB (required >= 2000 MB)
integration-light health OK: postgres + nats up (stores-only; no core/ml, no ml_sidecar gate)
=== RUN   TestCaptureDisconnectDurability_ProcessorSurvivesClientCancel
2026/06/30 18:35:07 INFO ensured NATS stream name=ARTIFACTS subjects=[artifacts.>]
    capture_disconnect_durability_test.go:138: durable capture persisted despite client disconnect: artifact=01KWCX0HNVZK3E8MM4ZB0ZFYXS status=pending ARTIFACTS LastSeq 0->1
    capture_disconnect_durability_test.go:187: raw cancelled request context aborted the durable write as expected; 0 rows persisted — fix is load-bearing
--- PASS: TestCaptureDisconnectDurability_ProcessorSurvivesClientCancel (0.67s)
ok      github.com/smackerel/smackerel/tests/integration        0.090s
```

### Audit Evidence

Audit of the changed surface in committed code: the durable `/api/capture` write
is dispatched on `context.WithoutCancel(r.Context())` at exactly ONE call site
(capture-once invariant BS-001 preserved), proven by the committed diff at
`d395d00c` and the on-disk grep:

```text
$ git --no-pager show d395d00c -- internal/api/capture.go
commit d395d00c1b8e021291b57c91655f5fdee03583cd
Author: Philippe Kirsanov <pkirsanov@gmail.com>
Date:   Tue Jun 30 02:14:05 2026 -0700

    fix(api): decouple /api/capture durable write from request cancellation (BUG-069-003)

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
+	// […14-line "capture-as-fallback is inviolable" comment; full verbatim text in the Code Diff Evidence section below…]
+	result, err := d.Pipeline.Process(context.WithoutCancel(r.Context()), &pipeline.ProcessRequest{
$ grep -n 'WithoutCancel' internal/api/capture.go
110:	result, err := d.Pipeline.Process(context.WithoutCancel(r.Context()), &pipeline.ProcessRequest{
```

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

The done-ceiling evidence is now complete: the shared live-stack disconnect-race
durability regression `TestCaptureDisconnectDurability_ProcessorSurvivesClientCancel`
PASSED on a real `pipeline.Processor` + Postgres + NATS (ARTIFACTS LastSeq 0→1;
adversarial raw-cancel persists 0 rows), and the bugfix-fastlane review phases
(audit, simplify, stabilize, security, validate) were genuinely reviewed and
recorded in `state.json`. A durability *correctness* fix is not an SLA/stress scope
(Gate G026). Scope 1 is Done; status is held at `in_progress` for the orchestrator's
commit-then-certify step. `nextRequiredOwner: bubbles.validate`.

### Finding Ledger (one-to-one closure)

| Finding | Severity | Status | Closure |
|---------|----------|--------|---------|
| F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL | Medium | Closed | `d.Pipeline.Process(context.WithoutCancel(r.Context()), …)` + `"context"` import in `capture.go` + `TestCaptureHandler_CaptureSurvivesClientDisconnect` adversarial regression (RED→GREEN); `internal/api` package green; shared live-stack durability regression PASSED (real pg+nats) |

One finding discovered, one finding closed. No deferral, no cherry-picking.

### Disposition

Scope 1 Done. The code fix + adversarial unit regression are complete and green
(R1→R4), AND the shared live-stack durability regression
`TestCaptureDisconnectDurability_ProcessorSurvivesClientCancel` PASSED on a real
`pipeline.Processor` + Postgres + NATS stack (ARTIFACTS LastSeq 0→1; adversarial
raw-cancel persists 0 rows; committed at `f00a2132`). The five remaining
bugfix-fastlane phases (audit, simplify, stabilize, security, validate) were
genuinely reviewed and recorded in `state.json`. A durability *correctness* fix is
not an SLA/stress scope (Gate G026: no SLA-sensitive scope). Status is held at
`in_progress`; the authoritative `certification.status` flip is the orchestrator's
commit-then-certify step. The already-`done` parent spec 069 artifacts and the
BUG-069-002 packet are left untouched; this finding and fix are recorded entirely in
this bug packet.

### Live-Stack Durable Regression — PASSED (shared with BUG-069-002)

**Owner:** bubbles.test · **fixSequence order-2** (shared with BUG-069-002 order-2) · **Claim Source:** executed — live `integration-light` run on a real Postgres+NATS stack (stores-only lane), captured 2026-06-30. The two bugs share one root cause and one durable processor path, so order-2 is ONE shared regression (no duplicate live-stack harness):

- **Test file:** `tests/integration/capture_disconnect_durability_test.go` (`//go:build integration`, package `integration`; committed at `f00a2132`)
- **Test:** `TestCaptureDisconnectDurability_ProcessorSurvivesClientCancel`

It exercises the durable path THIS bug's fix relies on — `internal/api/capture.go::CaptureHandler` dispatching `pipeline.Processor.Process(context.WithoutCancel(r.Context()), …) → submitForProcessing → storeInitialArtifact INSERT + NATS.Publish` — the identical path `/api/assistant/turn` (BUG-069-002) uses, against a REAL `pipeline.Processor` + Postgres + NATS.

```text
=== RUN   TestCaptureDisconnectDurability_ProcessorSurvivesClientCancel
2026/06/30 18:35:07 INFO ensured NATS stream name=ARTIFACTS subjects=[artifacts.>]
    capture_disconnect_durability_test.go:138: durable capture persisted despite client disconnect: artifact=01KWCX0HNVZK3E8MM4ZB0ZFYXS status=pending ARTIFACTS LastSeq 0->1
    capture_disconnect_durability_test.go:187: raw cancelled request context aborted the durable write as expected (err=...context canceled); 0 rows persisted — fix is load-bearing
--- PASS: TestCaptureDisconnectDurability_ProcessorSurvivesClientCancel (0.67s)
ok      github.com/smackerel/smackerel/tests/integration        0.090s
```

Sub-test 1 drives `Process` on `context.WithoutCancel` of an already-cancelled request context and asserts the artifact survives in Postgres (since `submitForProcessing` deletes the row on NATS-publish failure, survival proves INSERT + publish BOTH landed; the ARTIFACTS JetStream LastSeq 0→1 advance corroborates). Sub-test 2 is the adversarial pre-fix loss path: the RAW cancelled context aborts the write and persists 0 rows, proving the `context.WithoutCancel` fix is load-bearing. The per-handler wiring for THIS bug (that `CaptureHandler` passes `context.WithoutCancel`) is additionally pinned by the unit regression `TestCaptureHandler_CaptureSurvivesClientDisconnect` (R1→R3).

**Status:** Scope 1 Done; the shared regression PASSED on a real pg+nats stack. A durability *correctness* fix is not an SLA/stress scope (Gate G026 detects no SLA-sensitive scope). Certification is held at `in_progress`; the authoritative `certification.status` flip is the orchestrator's commit-then-certify step.

### Code Diff Evidence

The fix is a single durable-write call-site change in `internal/api/capture.go` (the `/api/capture` durable write is decoupled from request cancellation via `context.WithoutCancel`) plus the previously-absent `"context"` import. It is committed at `d395d00c` (working tree clean); the greps below confirm the single decoupled call site on disk:

```text
$ grep -n 'd.Pipeline.Process(' internal/api/capture.go
110:	result, err := d.Pipeline.Process(context.WithoutCancel(r.Context()), &pipeline.ProcessRequest{
$ grep -nc 'context.WithoutCancel' internal/api/capture.go
1
$ git log --oneline -n 1 -- internal/api/capture.go
d395d00c fix(api): decouple /api/capture durable write from request cancellation (BUG-069-003)
```

Git-backed proof — the committed diff in `internal/api/capture.go` (executed this session):

```text
$ git show d395d00c -- internal/api/capture.go
commit d395d00c1b8e021291b57c91655f5fdee03583cd
Author: Philippe Kirsanov <pkirsanov@gmail.com>
Date:   Tue Jun 30 02:14:05 2026 -0700

    fix(api): decouple /api/capture durable write from request cancellation (BUG-069-003)

    The direct POST /api/capture handler passed r.Context() into pipeline.Process, so a client disconnect mid-request cancelled the durable Postgres INSERT + NATS publish and silently lost the capture — the same inviolable capture-as-fallback violation fixed for /api/assistant/turn in BUG-069-002, at the endpoint never in 069-002's scope (code-review readiness finding F-01 / F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL).

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
+	// […14-line "capture-as-fallback is inviolable" comment; full verbatim text rendered in [R2] above…]
+	result, err := d.Pipeline.Process(context.WithoutCancel(r.Context()), &pipeline.ProcessRequest{
 		URL:          req.URL,
 		Text:         req.Text,
 		VoiceURL:     req.VoiceURL,
```

`internal/api/capture.go` is a runtime source file (not an artifact), satisfying the Gate G053 non-artifact delta requirement.

### Phase Provenance — INFO[G022-PARENT-EXPANDED]

INFO[G022-PARENT-EXPANDED]: the bugfix-fastlane phases (code-review discovery, implement, test, regression, audit, simplify, stabilize, security, validate) were parent-expanded by the registered orchestrator `bubbles.goal` because this runtime had no nested `runSubagent` specialist dispatch (capability missing). Each phase was genuinely executed/reviewed against the changed files; provenance is recorded in `state.json` `execution.executionHistory` (`expandedBy: bubbles.goal`, `expansionEvidenceRef: report.md`), mirroring the sibling BUG-069-001 and the just-certified BUG-069-002 packets.
