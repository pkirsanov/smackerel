//go:build integration

// Spec 072 SCOPE-2 — TP-072-08 / SCN-072-A05.
//
// Integration row that drives a signed Meta WhatsApp text webhook
// through the real chi-mounted route with a facade that returns
// CaptureRoute=true and asserts the capture hook fires exactly
// once and the rendered WhatsApp body matches the canonical
// "saved-as-idea" acknowledgement Telegram and HTTP emit.

package assistant_integration

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	wa "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

const canonicalSavedAsIdeaBody = "saved as an idea — i'll surface it later."

type captureRouteAlwaysFacade struct {
	calls atomic.Int64
}

func (f *captureRouteAlwaysFacade) Handle(_ context.Context, _ contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	f.calls.Add(1)
	return contracts.AssistantResponse{
		Status:       contracts.StatusSavedAsIdea,
		CaptureRoute: true,
		Body:         canonicalSavedAsIdeaBody,
	}, nil
}

type captureRouteBound struct{ inner *captureRouteAlwaysFacade }

func (b *captureRouteBound) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	return b.inner.Handle(ctx, msg)
}

type capturingCloud struct {
	text         atomic.Int64
	interactive  atomic.Int64
	lastTextBody string
	lastTo       string
}

func (c *capturingCloud) SendText(_ context.Context, to string, msg wa.TextMessage) error {
	c.text.Add(1)
	c.lastTextBody = msg.Body
	c.lastTo = to
	return nil
}
func (c *capturingCloud) SendInteractive(_ context.Context, _ string, _ wa.InteractiveMessage) error {
	c.interactive.Add(1)
	return nil
}

type recordingCapture struct {
	calls atomic.Int64
	last  contracts.AssistantMessage
}

func (c *recordingCapture) fn() wa.CaptureFn {
	return func(_ context.Context, msg contracts.AssistantMessage) {
		c.calls.Add(1)
		c.last = msg
	}
}

// TestWhatsAppCapture_TP_072_08_CaptureRouteInvokesCaptureOnce
// asserts SCN-072-A05: when the facade returns
// AssistantResponse{CaptureRoute:true}, the capture hook fires
// exactly once with the canonical AssistantMessage and the
// rendered WhatsApp body matches the canonical acknowledgement
// shape Telegram and HTTP emit.
func TestWhatsAppCapture_TP_072_08_CaptureRouteInvokesCaptureOnce(t *testing.T) {
	const appSecret = "test-app-secret-tp-072-08"
	const wamid = "wamid.HBgN.tp-072-08"

	cap := &recordingCapture{}
	cloud := &capturingCloud{}
	adapter, err := wa.NewAdapter(wa.Options{
		Verify:                    wa.HMACVerifier{AppSecret: appSecret, VerifyToken: "tok"},
		IdentityRegistry:          fixedRegistry{userID: "user-tp-072-08"},
		IdentityHashKey:           "test-hash-key",
		MaxTextChars:              4096,
		RateLimitPerUserPerMinute: 30,
		Cloud:                     cloud,
		Capture:                   cap.fn(),
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	fac := &captureRouteAlwaysFacade{}
	if err := adapter.Start(context.Background(), &captureRouteBound{inner: fac}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	r := chi.NewRouter()
	r.Handle("/v1/assistant/transports/whatsapp/webhook", wa.NewWebhookHandler(wa.WebhookHandlerOptions{Adapter: adapter}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	body := waTextPayload(wamid, "remember to buy milk")
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(body))
	req.Header.Set(wa.SignatureHeader, signMeta(body, appSecret))
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}

	if got := cap.calls.Load(); got != 1 {
		t.Fatalf("capture invocations: want 1, got %d", got)
	}
	if cap.last.Transport != wa.TransportName {
		t.Errorf("capture Transport: want %q, got %q", wa.TransportName, cap.last.Transport)
	}
	if cap.last.TransportMessageID != wamid {
		t.Errorf("capture TransportMessageID: want %q, got %q", wamid, cap.last.TransportMessageID)
	}
	if cap.last.UserID != "user-tp-072-08" {
		t.Errorf("capture UserID: want %q, got %q", "user-tp-072-08", cap.last.UserID)
	}
	if got := cloud.text.Load(); got != 1 {
		t.Fatalf("Cloud.SendText calls: want 1, got %d", got)
	}
	if got := cloud.interactive.Load(); got != 0 {
		t.Fatalf("Cloud.SendInteractive MUST be 0 for capture acknowledgement; got %d", got)
	}
	if cloud.lastTextBody != canonicalSavedAsIdeaBody {
		t.Errorf("rendered body: want %q, got %q", canonicalSavedAsIdeaBody, cloud.lastTextBody)
	}
	if cloud.lastTo != "+15555550123" {
		t.Errorf("rendered destination: want %q, got %q", "+15555550123", cloud.lastTo)
	}
	if got := fac.calls.Load(); got != 1 {
		t.Fatalf("facade calls: want 1, got %d", got)
	}
}
