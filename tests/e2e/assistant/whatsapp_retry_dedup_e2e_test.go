//go:build e2e

// Spec 072 SCOPE-3 — TP-072-13 / SCN-072-A10.
//
// Live-stack regression that a duplicate Meta webhook delivery
// (same wamid) does NOT duplicate the scenario invocation OR the
// capture-as-fallback artifact. Drives the real chi router with a
// recording facade and a recording CaptureFn; the assertion is the
// per-wamid 1:1 invariant required by the spec.

package assistant_e2e

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

type wrdRegistry struct{ userID string }

func (r wrdRegistry) Resolve(_ context.Context, _, _ string) (string, error) { return r.userID, nil }

type wrdCloud struct{ text atomic.Int64 }

func (c *wrdCloud) SendText(_ context.Context, _ string, _ wa.TextMessage) error {
	c.text.Add(1)
	return nil
}
func (c *wrdCloud) SendInteractive(_ context.Context, _ string, _ wa.InteractiveMessage) error {
	return nil
}

type wrdFacade struct{ calls atomic.Int64 }

func (f *wrdFacade) Handle(_ context.Context, _ contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	f.calls.Add(1)
	return contracts.AssistantResponse{
		Status:       contracts.StatusSavedAsIdea,
		CaptureRoute: true,
		Body:         "saved as an idea — i'll surface it later.",
	}, nil
}

type wrdBound struct{ inner *wrdFacade }

func (b *wrdBound) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	return b.inner.Handle(ctx, msg)
}
func wrdBoundAssistant(f *wrdFacade) contracts.Assistant { return &wrdBound{inner: f} }

// TestWhatsAppRetryDedup_TP_072_13_DuplicateWebhookDoesNotDuplicate
// SCN-072-A10. Five Meta retries of the same wamid produce
// exactly one scenario invocation and exactly one capture
// artifact; the adversarial second-wamid delivery proves the dedup
// is per-message-id, not a global gate.
func TestWhatsAppRetryDedup_TP_072_13_DuplicateWebhookDoesNotDuplicate(t *testing.T) {
	const appSecret = "test-app-secret-072-tp-13"
	const wamid = "wamid.tp-072-13.dup"

	var captureCount atomic.Int64
	cloud := &wrdCloud{}
	adapter, err := wa.NewAdapter(wa.Options{
		Verify:                    wa.HMACVerifier{AppSecret: appSecret, VerifyToken: "tok"},
		IdentityRegistry:          wrdRegistry{userID: "user-tp-072-13"},
		IdentityHashKey:           "test-hash-key",
		MaxTextChars:              4096,
		RateLimitPerUserPerMinute: 30,
		Cloud:                     cloud,
		Capture: func(_ context.Context, _ contracts.AssistantMessage) {
			captureCount.Add(1)
		},
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	fac := &wrdFacade{}
	if err := adapter.Start(context.Background(), wrdBoundAssistant(fac)); err != nil {
		t.Fatalf("Start: %v", err)
	}

	r := chi.NewRouter()
	r.Handle("/v1/assistant/transports/whatsapp/webhook", wa.NewWebhookHandler(wa.WebhookHandlerOptions{Adapter: adapter}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	body := wrtTextPayload(wamid, "hello tp-072-13")
	sig := wrtSign(body, appSecret)
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(body))
		req.Header.Set(wa.SignatureHeader, sig)
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("retry %d: %v", i, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("retry %d: status want 200, got %d", i, resp.StatusCode)
		}
	}

	if got := fac.calls.Load(); got != 1 {
		t.Fatalf("scenario invocations: want 1 for retried wamid, got %d", got)
	}
	if got := captureCount.Load(); got != 1 {
		t.Fatalf("capture artifacts: want 1, got %d", got)
	}
	if got := cloud.text.Load(); got != 1 {
		t.Fatalf("Cloud.SendText calls: want 1, got %d", got)
	}

	// Adversarial: a second, distinct wamid for the same user MUST
	// reach the facade — the dedup MUST NOT degenerate into a
	// per-user gate that swallows legitimate follow-up turns.
	body2 := wrtTextPayload(wamid+"-followup", "second message")
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(body2))
	req.Header.Set(wa.SignatureHeader, wrtSign(body2, appSecret))
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("followup: %v", err)
	}
	_ = resp.Body.Close()
	if got := fac.calls.Load(); got != 2 {
		t.Fatalf("scenario invocations after distinct wamid: want 2, got %d", got)
	}
	if got := captureCount.Load(); got != 2 {
		t.Fatalf("capture artifacts after distinct wamid: want 2, got %d", got)
	}
}
