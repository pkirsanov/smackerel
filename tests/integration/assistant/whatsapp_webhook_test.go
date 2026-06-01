//go:build integration

// Spec 072 SCOPE-1 — TP-072-01 / SCN-072-A01.
//
// Integration row that drives a signed Meta WhatsApp text webhook
// through the real chi-mounted route and asserts the facade
// observes a canonical AssistantMessage with Transport="whatsapp"
// and TransportMessageID equal to the inbound wamid.

package assistant_integration

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	wa "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

type capturingFacade struct {
	mu       sync.Mutex
	observed []contracts.AssistantMessage
}

func (f *capturingFacade) Handle(_ context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	f.mu.Lock()
	f.observed = append(f.observed, msg)
	f.mu.Unlock()
	return contracts.AssistantResponse{Body: "ok"}, nil
}

type capturingBound struct{ inner *capturingFacade }

func (b *capturingBound) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	return b.inner.Handle(ctx, msg)
}

// TestWhatsAppWebhook_TP_072_01_SignedTextBecomesCanonicalMessage
// asserts SCN-072-A01: a signed inbound WhatsApp text webhook is
// translated into an AssistantMessage{Transport:"whatsapp"} whose
// TransportMessageID equals the WhatsApp message id.
func TestWhatsAppWebhook_TP_072_01_SignedTextBecomesCanonicalMessage(t *testing.T) {
	const appSecret = "test-app-secret-tp-072-01"
	const wamid = "wamid.HBgN.tp-072-01"

	cap := &captureCounter{}
	cloud := &recordingCloud{}
	adapter, err := wa.NewAdapter(wa.Options{
		Verify:                    wa.HMACVerifier{AppSecret: appSecret, VerifyToken: "tok"},
		IdentityRegistry:          fixedRegistry{userID: "user-tp-072-01"},
		IdentityHashKey:           "test-hash-key",
		MaxTextChars:              4096,
		RateLimitPerUserPerMinute: 30,
		Cloud:                     cloud,
		Capture:                   cap.fn(),
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	fac := &capturingFacade{}
	if err := adapter.Start(context.Background(), &capturingBound{inner: fac}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	r := chi.NewRouter()
	r.Handle("/v1/assistant/transports/whatsapp/webhook", wa.NewWebhookHandler(wa.WebhookHandlerOptions{Adapter: adapter}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	body := waTextPayload(wamid, "hello tp-072-01")
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set(wa.SignatureHeader, signMeta(body, appSecret))
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}

	fac.mu.Lock()
	defer fac.mu.Unlock()
	if len(fac.observed) != 1 {
		t.Fatalf("facade observations: want 1, got %d", len(fac.observed))
	}
	got := fac.observed[0]
	if got.Transport != wa.TransportName {
		t.Errorf("Transport: want %q, got %q", wa.TransportName, got.Transport)
	}
	if got.TransportMessageID != wamid {
		t.Errorf("TransportMessageID: want %q, got %q", wamid, got.TransportMessageID)
	}
	if got.UserID != "user-tp-072-01" {
		t.Errorf("UserID: want %q, got %q", "user-tp-072-01", got.UserID)
	}
	if got.Kind != contracts.KindText {
		t.Errorf("Kind: want %q, got %q", contracts.KindText, got.Kind)
	}
	if got.Text != "hello tp-072-01" {
		t.Errorf("Text: want %q, got %q", "hello tp-072-01", got.Text)
	}

	// Adversarial: a tampered body with a stale signature MUST NOT
	// produce a second observation — proves the facade-observation
	// path is gated by signature verification, not by the existence
	// of the route.
	tampered := waTextPayload(wamid+"-2", "tampered")
	req2, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(tampered))
	req2.Header.Set(wa.SignatureHeader, signMeta(body, appSecret)) // signature for ORIGINAL body
	resp2, err := srv.Client().Do(req2)
	if err != nil {
		t.Fatalf("tampered Do: %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode == http.StatusOK {
		t.Fatalf("tampered delivery: want non-200, got 200")
	}
	if len(fac.observed) != 1 {
		t.Fatalf("facade observations after tampered: want 1, got %d", len(fac.observed))
	}
}
