package httpadapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/auth"
)

// captureRouteFacade always asks the adapter to take the
// capture-as-fallback branch (CaptureRoute=true). It ignores the
// context so the test can drive the client-disconnect race
// deterministically — the contract under test is that the *capture
// write* is not abandoned when the request context is cancelled.
type captureRouteFacade struct{}

func (captureRouteFacade) Handle(_ context.Context, _ contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	return contracts.AssistantResponse{
		Status:       contracts.StatusSavedAsIdea,
		CaptureRoute: true,
		EmittedAt:    time.Unix(1735689600, 0).UTC(),
	}, nil
}

// TestHTTPAdapter_CaptureSurvivesClientDisconnect is the spec-069
// chaos Round 39 regression for F-069-CHAOS39-CAPTURE-CTX-CANCEL.
//
// Scenario: a web/mobile client POSTs a turn, the facade decides the
// turn must be captured-as-fallback (CaptureRoute=true), then the
// client disconnects (or its request deadline fires) before the
// capture pipeline write completes. The net/http server cancels
// r.Context() the instant the connection drops, so any context-aware
// downstream work (the production capture path runs a Postgres INSERT
// and a NATS publish, both ctx-honoring) aborts with context.Canceled
// and the user's prompt is silently lost.
//
// Inviolable contract — Hard Constraint 5 / BS-001 /
// policySnapshot.captureAsFallback="inviolable": the user's prompt
// MUST NOT be lost. The capture path therefore MUST run with a
// context that is NOT cancelled by the client disconnect.
//
// Adversarial RED-before / GREEN-after: with the pre-fix code
// (a.capture(r.Context(), ...)) the captured context is an invalid
// (cancelled) context and this test fails. It passes only when the
// durable capture write completes without the client connection —
// i.e. the capture is decoupled from request cancellation. If the
// fix is ever reverted to r.Context(), this test fails again.
func TestHTTPAdapter_CaptureSurvivesClientDisconnect(t *testing.T) {
	var (
		captureCalled bool
		captureCtxErr error
		captureUser   string
		captureText   string
	)

	cfg := defaultConfig()
	cfg.SharedUserID = "disconnect-user"

	adapter, err := NewHTTPAdapter(Options{
		Facade: captureRouteFacade{},
		Capture: func(ctx context.Context, userID, _, text string) {
			captureCalled = true
			captureCtxErr = ctx.Err()
			captureUser = userID
			captureText = text
		},
		Clock:  func() time.Time { return time.Unix(1735689600, 0).UTC() },
		Config: cfg,
	})
	if err != nil {
		t.Fatalf("NewHTTPAdapter: %v", err)
	}

	const body = `{"schema_version":"v1","transport_message_id":"disc-1","kind":"text","text":"remember to file taxes"}`
	r := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", strings.NewReader(body))

	// Inject a shared-token session so the adapter resolves the
	// SharedUserID branch instead of 401-ing, then cancel the request
	// context to model the client disconnecting mid-turn.
	ctx, cancel := context.WithCancel(
		auth.WithSession(r.Context(), auth.Session{Source: auth.SessionSourceSharedToken}),
	)
	cancel() // the client is gone before the capture write runs
	r = r.WithContext(ctx)

	w := httptest.NewRecorder()
	adapter.ServeHTTP(w, r)

	if !captureCalled {
		t.Fatal("capture-as-fallback was never invoked on a CaptureRoute=true turn; the user's prompt was lost (BS-001 / Hard Constraint 5 violation)")
	}
	if captureCtxErr != nil {
		t.Fatalf("capture ran with a cancelled context (err=%v); a client disconnect MUST NOT abort the durable capture write (F-069-CHAOS39-CAPTURE-CTX-CANCEL)", captureCtxErr)
	}
	if captureText != "remember to file taxes" {
		t.Errorf("capture text = %q, want the original prompt preserved", captureText)
	}
	if captureUser != "disconnect-user" {
		t.Errorf("capture userID = %q, want disconnect-user", captureUser)
	}
}
