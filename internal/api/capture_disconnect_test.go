package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/pipeline"
)

// recordingPipeline is a Pipeliner stub that records the context handed to
// Process (and the prompt text) so the test can assert the durable capture
// write is decoupled from request cancellation. It returns a successful,
// minimal ProcessResult so CaptureHandler runs to the 200 response path.
//
// Mirrors the established internal/api harness conventions (mockDB / mockNATS
// in health_test.go, the Dependencies test wiring in capture_test.go) rather
// than inventing a parallel harness.
type recordingPipeline struct {
	called    bool
	gotCtxErr error
	gotText   string
}

func (p *recordingPipeline) Process(ctx context.Context, req *pipeline.ProcessRequest) (*pipeline.ProcessResult, error) {
	p.called = true
	p.gotCtxErr = ctx.Err()
	p.gotText = req.Text
	return &pipeline.ProcessResult{
		ArtifactID:       "art-durable-1",
		Title:            "durable idea",
		ArtifactType:     "note",
		ProcessingStatus: "pending",
	}, nil
}

// TestCaptureHandler_CaptureSurvivesClientDisconnect is the spec-069
// code-review readiness regression for F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL
// (finding F-01).
//
// Scenario: an API/extension/PWA client POSTs to the direct /api/capture
// endpoint, then disconnects (or its request deadline fires) after the body
// is received but before the durable write completes. net/http cancels
// r.Context() the instant the connection drops, so the production capture
// path — pipeline.Processor.Process, which runs a context-honoring Postgres
// INSERT (storeInitialArtifact) and a context-honoring NATS publish
// (submitForProcessing) — aborts with context.Canceled and the user's
// capture is silently lost.
//
// This is the SAME root cause already fixed for /api/assistant/turn in
// BUG-069-002 (a.capture(context.WithoutCancel(r.Context()), ...)), but at
// the DIRECT /api/capture endpoint, which was never in BUG-069-002's scope.
//
// Inviolable contract — Hard Constraint 5 / BS-001 /
// policySnapshot.captureAsFallback="inviolable": the user's prompt MUST NOT
// be lost. The durable Process call therefore MUST run with a context that is
// NOT cancelled by the client disconnect.
//
// Adversarial RED-before / GREEN-after: with the pre-fix code
// (d.Pipeline.Process(r.Context(), ...)) the recorded context carries
// context.Canceled and this test fails. It passes only when the durable write
// is decoupled from request cancellation
// (d.Pipeline.Process(context.WithoutCancel(r.Context()), ...)). If the fix is
// ever reverted to r.Context(), this test fails again.
func TestCaptureHandler_CaptureSurvivesClientDisconnect(t *testing.T) {
	pipe := &recordingPipeline{}
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		Pipeline:  pipe,
	}

	const body = `{"text":"durable idea"}`
	req := httptest.NewRequest(http.MethodPost, "/api/capture", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	// Model the client disconnecting after the body is received but before the
	// durable write completes: cancel the request context before CaptureHandler
	// runs the pipeline write.
	ctx, cancel := context.WithCancel(req.Context())
	cancel() // the client is gone before the durable Process call runs
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	deps.CaptureHandler(rec, req)

	if !pipe.called {
		t.Fatal("pipeline.Process was never invoked on /api/capture; the user's capture was lost (BS-001 / Hard Constraint 5 violation)")
	}
	if pipe.gotCtxErr != nil {
		t.Fatalf("durable capture write ran with a cancelled context (err=%v); a client disconnect MUST NOT abort the /api/capture durable write (F-069-CR-CAPTURE-ENDPOINT-CTX-CANCEL)", pipe.gotCtxErr)
	}
	if pipe.gotText != "durable idea" {
		t.Errorf("capture text = %q, want the original prompt preserved", pipe.gotText)
	}
}
