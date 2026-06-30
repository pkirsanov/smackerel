//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	smacknats "github.com/smackerel/smackerel/internal/nats"
	"github.com/smackerel/smackerel/internal/pipeline"
)

// TestCaptureDisconnectDurability_ProcessorSurvivesClientCancel is the SHARED
// live-stack durability regression for the inviolable capture-as-fallback
// contract (spec 069 Hard Constraint 5 / BS-001 /
// policySnapshot.captureAsFallback="inviolable"). It is the fixSequence order-2
// deliverable jointly owned by BUG-069-002 and BUG-069-003.
//
// Both fixed HTTP capture endpoints dispatch the SAME durable path and both now
// wrap the request context in context.WithoutCancel:
//
//   - BUG-069-002  internal/assistant/httpadapter/adapter.go::ServeHTTP
//     a.capture(context.WithoutCancel(r.Context()), …)        // POST /api/assistant/turn
//   - BUG-069-003  internal/api/capture.go::CaptureHandler
//     d.Pipeline.Process(context.WithoutCancel(r.Context()), …) // POST /api/capture
//
// The shared durable path is:
//
//	pipeline.Processor.Process
//	  → submitForProcessing
//	    → storeInitialArtifact   (Postgres INSERT, context-honoring)
//	    → NATS.Publish(SubjectArtifactsProcess)  (JetStream publish, context-honoring)
//
// net/http cancels r.Context() the instant the client connection drops. Before
// the fix, that cancellation aborted the INSERT / publish with
// context.Canceled and the capture was silently lost. The fix wraps the request
// context in context.WithoutCancel so a client disconnect can no longer abort
// the durable write, while request-scoped Values (request id / TraceID consumed
// by submitForProcessing via middleware.GetReqID) still survive.
//
// KEY INSIGHT exploited by the persistence assertion: submitForProcessing
// DELETES the artifact row when the NATS publish fails (orphan cleanup). So an
// artifact STILL PRESENT in Postgres after Process returns proves BOTH the
// INSERT and the NATS publish succeeded — one Postgres read certifies the whole
// durable write. The ARTIFACTS JetStream LastSeq advance is an independent
// corroboration that the publish physically landed in the stream.
//
// Per-handler HTTP wiring (that each endpoint actually passes
// context.WithoutCancel) is already proven by the unit regressions
// TestHTTPAdapter_CaptureSurvivesClientDisconnect (BUG-069-002) and
// TestCaptureHandler_CaptureSurvivesClientDisconnect (BUG-069-003). This test
// proves the processor-level durability invariant those wirings depend on,
// against a REAL pipeline.Processor + Postgres + NATS.
func TestCaptureDisconnectDurability_ProcessorSurvivesClientCancel(t *testing.T) {
	pool := testPool(t)
	natsClient := captureDurabilityNATSClient(t)
	proc := pipeline.NewProcessor(pool, natsClient)

	t.Run("WithoutCancel_after_client_disconnect_persists_the_capture", func(t *testing.T) {
		// Model production AFTER the fix: a request context that net/http has
		// ALREADY cancelled (client disconnected), wrapped in
		// context.WithoutCancel — the exact context both fixed endpoints pass
		// into the durable write.
		reqID := "disc-dur-" + testID(t)
		parent := context.WithValue(context.Background(), middleware.RequestIDKey, reqID)
		reqCtx, cancel := context.WithCancel(parent)
		cancel() // client disconnect: the request context is dead before the durable write

		durableCtx := context.WithoutCancel(reqCtx)

		// The fix decouples cancellation but MUST preserve request-scoped Values:
		// submitForProcessing reads middleware.GetReqID(ctx) for the artifact
		// TraceID. Prove that half of the contract directly (no stack needed for
		// this assertion, but it pins why WithoutCancel was chosen over a fresh
		// context.Background()).
		if durableCtx.Err() != nil {
			t.Fatalf("context.WithoutCancel must NOT be cancelled by the dead parent: err=%v", durableCtx.Err())
		}
		if got := middleware.GetReqID(durableCtx); got != reqID {
			t.Fatalf("context.WithoutCancel dropped the request id: GetReqID=%q want %q", got, reqID)
		}

		req := &pipeline.ProcessRequest{
			// Unique text -> unique content_hash -> no dedup collision across runs.
			Text:     "capture-as-fallback durability probe " + reqID,
			SourceID: pipeline.SourceCapture,
		}

		seqBefore := artifactsStreamLastSeq(t, natsClient)

		res, err := proc.Process(durableCtx, req)
		if err != nil {
			t.Fatalf("Process on context.WithoutCancel(cancelled-request-ctx) MUST succeed — capture-as-fallback is inviolable (Hard Constraint 5 / BS-001): %v", err)
		}
		if res == nil || res.ArtifactID == "" {
			t.Fatalf("Process returned no artifact id: res=%+v", res)
		}
		cleanupArtifact(t, pool, res.ArtifactID)

		// KEY INSIGHT: the row surviving in Postgres proves INSERT + NATS publish
		// BOTH succeeded — submitForProcessing deletes the row on publish failure.
		readCtx, cancelRead := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelRead()
		var contentRaw, status string
		if err := pool.QueryRow(readCtx,
			"SELECT content_raw, processing_status FROM artifacts WHERE id = $1",
			res.ArtifactID,
		).Scan(&contentRaw, &status); err != nil {
			t.Fatalf("durable capture artifact %s NOT found in Postgres — the disconnect aborted the write and the capture was lost: %v", res.ArtifactID, err)
		}
		if contentRaw != req.Text {
			t.Fatalf("persisted content_raw=%q want %q", contentRaw, req.Text)
		}
		if status != string(pipeline.StatusPending) {
			t.Fatalf("persisted processing_status=%q want %q", status, pipeline.StatusPending)
		}

		// Independent corroboration: the ARTIFACTS publish physically landed.
		// LastSeq is monotonic and never decreases, so this holds even if an ML
		// consumer drains the message (WorkQueue retention).
		seqAfter := artifactsStreamLastSeq(t, natsClient)
		if seqAfter <= seqBefore {
			t.Fatalf("ARTIFACTS JetStream LastSeq did not advance (before=%d after=%d) — the NATS publish did not land", seqBefore, seqAfter)
		}
		t.Logf("durable capture persisted despite client disconnect: artifact=%s status=%s ARTIFACTS LastSeq %d->%d", res.ArtifactID, status, seqBefore, seqAfter)
	})

	t.Run("raw_cancelled_request_ctx_loses_the_capture_pre_fix_loss_path", func(t *testing.T) {
		// Adversarial guard (anti-tautology): model the PRE-FIX call shape — the
		// durable write bound DIRECTLY to the cancelled request context. If a
		// future change reverts either endpoint to Process(r.Context(), …), THIS
		// is the exact loss it reintroduces: the durable write aborts and nothing
		// persists. If this case ever fails-open (artifact persisted on a dead
		// context), the WithoutCancel fix would not be load-bearing and the
		// sibling assertion above would be a tautology — so this case MUST show
		// the loss.
		reqID := "disc-dur-raw-" + testID(t)
		parent := context.WithValue(context.Background(), middleware.RequestIDKey, reqID)
		reqCtx, cancel := context.WithCancel(parent)
		cancel() // dead request context, passed straight through (the bug)

		req := &pipeline.ProcessRequest{
			Text:     "capture-as-fallback raw-cancel probe " + reqID,
			SourceID: pipeline.SourceCapture,
		}

		res, err := proc.Process(reqCtx, req)
		if err == nil {
			if res != nil && res.ArtifactID != "" {
				cleanupArtifact(t, pool, res.ArtifactID)
			}
			t.Fatalf("Process on a raw cancelled request context unexpectedly SUCCEEDED — a dead request context MUST abort the durable write (pre-fix loss path); without this loss the durability regression could not catch a revert to r.Context()")
		}
		// The loss manifests as a cancelled durable write; errors.Is tolerates
		// pgx's wrapping of context.Canceled. A non-Canceled failure is still a
		// loss, so it is logged rather than hard-failed.
		if !errors.Is(err, context.Canceled) {
			t.Logf("durable write failed with a non-Canceled error (still a capture loss): %v", err)
		}

		// Prove nothing leaked into Postgres under the cancelled write.
		readCtx, cancelRead := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelRead()
		var count int
		if err := pool.QueryRow(readCtx,
			"SELECT count(*) FROM artifacts WHERE content_raw = $1",
			req.Text,
		).Scan(&count); err != nil {
			t.Fatalf("count artifacts by content_raw: %v", err)
		}
		if count != 0 {
			t.Fatalf("a cancelled durable write left %d artifact row(s) in Postgres for %q — expected 0 (the pre-fix path loses the capture; it must not partially persist)", count, req.Text)
		}
		t.Logf("raw cancelled request context aborted the durable write as expected (err=%v); 0 rows persisted — confirms the context.WithoutCancel fix is load-bearing", err)
	})
}

// captureDurabilityNATSClient connects a real smacknats.Client (the same type
// pipeline.NewProcessor consumes in production) to the disposable test NATS.
// Skips when NATS_URL is unset, matching the testPool / testNATSConn gate in
// helpers_test.go. The client is closed automatically at test end.
func captureDurabilityNATSClient(t *testing.T) *smacknats.Client {
	t.Helper()

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("integration: NATS_URL not set — live stack not available")
	}
	authToken := os.Getenv("SMACKEREL_AUTH_TOKEN")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := smacknats.Connect(ctx, natsURL, authToken)
	if err != nil {
		t.Fatalf("connect test NATS (smacknats.Client): %v", err)
	}
	t.Cleanup(client.Close)
	return client
}

// artifactsStreamLastSeq reads the ARTIFACTS JetStream stream's last assigned
// sequence number. The stream is created by core's EnsureStreams at startup, so
// in a live integration stack it MUST exist. LastSeq is monotonic and never
// decreases, so a post-publish read strictly greater than the pre-publish read
// proves a message physically landed in the stream.
func artifactsStreamLastSeq(t *testing.T, client *smacknats.Client) uint64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := client.JetStream.Stream(ctx, "ARTIFACTS")
	if err != nil {
		t.Fatalf("look up ARTIFACTS stream (core EnsureStreams must have created it): %v", err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatalf("read ARTIFACTS stream info: %v", err)
	}
	return info.State.LastSeq
}
