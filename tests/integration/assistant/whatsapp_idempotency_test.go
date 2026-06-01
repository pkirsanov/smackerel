//go:build integration

// Spec 072 SCOPE-3 — TP-072-12 / SCN-072-A10.
//
// Integration row that drives the WhatsApp webhook handler through
// the real chi router with a recording AssistantHandler and proves
// the adapter swallows Meta retries before they reach the facade.
//
// This row is integration (not unit) because it composes the full
// HTTP ingress path the live stack runs: chi route → signature
// verification → translate → identity resolution → idempotency
// gate → facade. The facade is replaced by a recording handler so
// the test does not depend on the wider knowledge graph; the row
// asserts the adapter contract that no second facade invocation
// reaches the handler regardless of how many times Meta retries.

package assistant_integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/transportidentity"
	wa "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

type fixedRegistry struct{ userID string }

func (r fixedRegistry) Resolve(_ context.Context, _, _ string) (string, error) {
	return r.userID, nil
}

type recordingCloud struct {
	text        atomic.Int64
	interactive atomic.Int64
}

func (c *recordingCloud) SendText(_ context.Context, _ string, _ wa.TextMessage) error {
	c.text.Add(1)
	return nil
}
func (c *recordingCloud) SendInteractive(_ context.Context, _ string, _ wa.InteractiveMessage) error {
	c.interactive.Add(1)
	return nil
}

type recordingFacade struct {
	calls atomic.Int64
}

func (f *recordingFacade) Handle(_ context.Context, _ contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	f.calls.Add(1)
	return contracts.AssistantResponse{
		Status:       contracts.StatusSavedAsIdea,
		CaptureRoute: true,
		Body:         "saved as an idea — i'll surface it later.",
	}, nil
}

type facadeBound struct{ inner *recordingFacade }

func (b *facadeBound) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	return b.inner.Handle(ctx, msg)
}

func boundAssistant(r *recordingFacade) contracts.Assistant {
	return &facadeBound{inner: r}
}

// TestWhatsAppIdempotency_TP_072_12_DuplicateMetaDeliveryInvokesFacadeOnce
// SCN-072-A10. Meta-style retries of the SAME wamid land on the
// same chi-mounted webhook route; the facade observes exactly one
// turn for that TransportMessageID even though the HTTP handler is
// invoked three times.
func TestWhatsAppIdempotency_TP_072_12_DuplicateMetaDeliveryInvokesFacadeOnce(t *testing.T) {
	const appSecret = "test-app-secret-072-scope-3"
	const wamid = "wamid.tp-072-12.dup"

	cap := &captureCounter{}
	cloud := &recordingCloud{}
	adapter, err := wa.NewAdapter(wa.Options{
		Verify:                    wa.HMACVerifier{AppSecret: appSecret, VerifyToken: "tok"},
		IdentityRegistry:          fixedRegistry{userID: "user-tp-072-12"},
		IdentityHashKey:           "test-hash-key",
		MaxTextChars:              4096,
		RateLimitPerUserPerMinute: 30,
		Cloud:                     cloud,
		Capture:                   cap.fn(),
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	rec := &recordingFacade{}
	if err := adapter.Start(context.Background(), boundAssistant(rec)); err != nil {
		t.Fatalf("Start: %v", err)
	}

	r := chi.NewRouter()
	r.Handle("/v1/assistant/transports/whatsapp/webhook", wa.NewWebhookHandler(wa.WebhookHandlerOptions{Adapter: adapter}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	body := waTextPayload(wamid, "hello tp-072-12")
	sig := signMeta(body, appSecret)
	for i := 0; i < 3; i++ {
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.Header.Set(wa.SignatureHeader, sig)
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("delivery %d: status want 200, got %d", i, resp.StatusCode)
		}
	}

	if got := rec.calls.Load(); got != 1 {
		t.Fatalf("facade Handle calls: want 1 for retried wamid, got %d", got)
	}
	if got := cap.count.Load(); got != 1 {
		t.Fatalf("capture invocations: want 1, got %d", got)
	}
	if got := cloud.text.Load(); got != 1 {
		t.Fatalf("Cloud.SendText invocations: want 1, got %d", got)
	}

	// Adversarial: a DIFFERENT wamid for the same user MUST reach
	// the facade — dedup is per-message-id, not per-user.
	other := waTextPayload(wamid+"-other", "second message")
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(other))
	req.Header.Set(wa.SignatureHeader, signMeta(other, appSecret))
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("distinct delivery: %v", err)
	}
	_ = resp.Body.Close()
	if got := rec.calls.Load(); got != 2 {
		t.Fatalf("facade Handle calls after distinct wamid: want 2, got %d", got)
	}
}

type captureCounter struct {
	count atomic.Int64
}

func (c *captureCounter) fn() wa.CaptureFn {
	return func(_ context.Context, _ contracts.AssistantMessage) {
		c.count.Add(1)
	}
}

func waTextPayload(wamid, body string) []byte {
	payload := map[string]any{
		"object": "whatsapp_business_account",
		"entry": []any{map[string]any{
			"id": "biz-1",
			"changes": []any{map[string]any{
				"field": "messages",
				"value": map[string]any{
					"messaging_product": "whatsapp",
					"metadata":          map[string]any{"display_phone_number": "+15550001", "phone_number_id": "pid-1"},
					"messages": []any{map[string]any{
						"id":        wamid,
						"from":      "+15555550123",
						"timestamp": "1700000000",
						"type":      "text",
						"text":      map[string]any{"body": body},
					}},
				},
			}},
		}},
	}
	out, _ := json.Marshal(payload)
	return out
}

func signMeta(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// silence unused-import lint when transportidentity isn't referenced
// outside the registry interface above.
var _ transportidentity.Registry = fixedRegistry{}
var _ = fmt.Sprintf
