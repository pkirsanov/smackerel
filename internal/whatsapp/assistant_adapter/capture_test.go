// SCN-072-A05 — unit-level capture-as-fallback test for the WhatsApp
// adapter. The full live-stack integration row (TP-072-08) is
// deferred to the runtime stack; this test asserts the adapter
// contract: when the facade returns AssistantResponse{CaptureRoute:true},
// the configured CaptureFn is invoked exactly once with the canonical
// AssistantMessage BEFORE the CloudClient is asked to render.

package assistant_adapter

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

type captureRecorder struct {
	calls atomic.Int64
	last  contracts.AssistantMessage
}

func (c *captureRecorder) fn() CaptureFn {
	return func(ctx context.Context, msg contracts.AssistantMessage) {
		c.calls.Add(1)
		c.last = msg
	}
}

type recordingCloud struct {
	textCalls        atomic.Int64
	interactiveCalls atomic.Int64
	lastTextBody     string
	lastTo           string
}

func (c *recordingCloud) SendText(_ context.Context, to string, msg TextMessage) error {
	c.textCalls.Add(1)
	c.lastTextBody = msg.Body
	c.lastTo = to
	return nil
}

func (c *recordingCloud) SendInteractive(_ context.Context, _ string, _ InteractiveMessage) error {
	c.interactiveCalls.Add(1)
	return nil
}

// captureRouteFacade always returns CaptureRoute=true with the
// canonical saved-as-idea body so the renderer produces the same text
// Telegram and HTTP would emit.
type captureRouteFacade struct{}

func (captureRouteFacade) Handle(_ context.Context, _ contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	return contracts.AssistantResponse{
		Status:       contracts.StatusSavedAsIdea,
		CaptureRoute: true,
		Body:         "saved as an idea — i'll surface it later.",
	}, nil
}

func TestWebhook_CaptureRouteInvokesCaptureBeforeRender(t *testing.T) {
	cap := &captureRecorder{}
	cloud := &recordingCloud{}
	adapter, err := NewAdapter(Options{
		Verify:                    HMACVerifier{AppSecret: testAppSecret, VerifyToken: "tok"},
		IdentityRegistry:          fixedRegistry{userID: "user-cap-1"},
		IdentityHashKey:           "test-hash-key",
		MaxTextChars:              4096,
		RateLimitPerUserPerMinute: 30,
		Cloud:                     cloud,
		Capture:                   cap.fn(),
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	if err := adapter.Start(context.Background(), captureRouteFacade{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	h := NewWebhookHandler(WebhookHandlerOptions{Adapter: adapter})

	body := []byte(sampleTextPayload)
	req := httptest.NewRequest(http.MethodPost, "/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(body))
	req.Header.Set(SignatureHeader, sign(body, testAppSecret))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if got := cap.calls.Load(); got != 1 {
		t.Fatalf("CaptureFn invocations: want 1, got %d", got)
	}
	if cap.last.UserID != "user-cap-1" {
		t.Errorf("capture got UserID=%q, want %q", cap.last.UserID, "user-cap-1")
	}
	if cap.last.Transport != TransportName {
		t.Errorf("capture got Transport=%q, want %q", cap.last.Transport, TransportName)
	}
	if got := cloud.textCalls.Load(); got != 1 {
		t.Fatalf("Cloud.SendText calls: want 1, got %d", got)
	}
	if cloud.lastTextBody != "saved as an idea — i'll surface it later." {
		t.Errorf("rendered body drift: %q", cloud.lastTextBody)
	}
	if cloud.lastTo != "+15555550123" {
		t.Errorf("rendered destination: want %q, got %q", "+15555550123", cloud.lastTo)
	}
}

// Non-capture path: CaptureRoute=false MUST NOT invoke the capture
// hook even if it is configured (BS-001 regression — capture only
// fires when the facade asks for it).
func TestWebhook_NonCaptureRouteDoesNotInvokeCapture(t *testing.T) {
	cap := &captureRecorder{}
	cloud := &recordingCloud{}
	adapter, err := NewAdapter(Options{
		Verify:                    HMACVerifier{AppSecret: testAppSecret, VerifyToken: "tok"},
		IdentityRegistry:          fixedRegistry{userID: "user-nocap"},
		IdentityHashKey:           "test-hash-key",
		MaxTextChars:              4096,
		RateLimitPerUserPerMinute: 30,
		Cloud:                     cloud,
		Capture:                   cap.fn(),
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	if err := adapter.Start(context.Background(), plainBodyFacade{}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	h := NewWebhookHandler(WebhookHandlerOptions{Adapter: adapter})

	body := []byte(sampleTextPayload)
	req := httptest.NewRequest(http.MethodPost, "/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(body))
	req.Header.Set(SignatureHeader, sign(body, testAppSecret))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	if got := cap.calls.Load(); got != 0 {
		t.Fatalf("CaptureFn MUST NOT fire when CaptureRoute=false; got %d", got)
	}
	if got := cloud.textCalls.Load(); got != 1 {
		t.Fatalf("Cloud.SendText calls: want 1, got %d", got)
	}
}

type plainBodyFacade struct{}

func (plainBodyFacade) Handle(_ context.Context, _ contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	return contracts.AssistantResponse{Body: "ok"}, nil
}
