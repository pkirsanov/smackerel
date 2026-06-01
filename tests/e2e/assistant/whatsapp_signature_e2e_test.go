//go:build e2e

// Spec 072 SCOPE-1 — TP-072-05 / SCN-072-A02.
//
// Live-stack regression that unsigned or wrongly-signed Meta
// webhook deliveries are rejected BEFORE any facade or capture
// invocation. Drives the real chi-mounted webhook handler against
// an httptest server and asserts every rejection variant returns a
// non-2xx status and that the recording facade is never invoked.

package assistant_e2e

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	wa "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

type sigE2ERegistry struct{ userID string }

func (r sigE2ERegistry) Resolve(_ context.Context, _, _ string) (string, error) {
	return r.userID, nil
}

type sigE2ECloud struct{ sends atomic.Int64 }

func (c *sigE2ECloud) SendText(_ context.Context, _ string, _ wa.TextMessage) error {
	c.sends.Add(1)
	return nil
}
func (c *sigE2ECloud) SendInteractive(_ context.Context, _ string, _ wa.InteractiveMessage) error {
	c.sends.Add(1)
	return nil
}

type sigE2EFacade struct{ calls atomic.Int64 }

func (f *sigE2EFacade) Handle(_ context.Context, _ contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	f.calls.Add(1)
	return contracts.AssistantResponse{Body: "ok"}, nil
}

type sigE2EBound struct{ inner *sigE2EFacade }

func (b *sigE2EBound) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	return b.inner.Handle(ctx, msg)
}

func sigE2ETextPayload(wamid string) []byte {
	out, _ := json.Marshal(map[string]any{
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
						"text":      map[string]any{"body": "hello"},
					}},
				},
			}},
		}},
	})
	return out
}

func sigE2ESign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// TestWhatsAppSignatureE2E_TP_072_05_UnsignedNeverReachesFacade
// asserts SCN-072-A02: every signature-rejection variant
// (missing, wrong-prefix, wrong-secret, tampered-body) returns a
// non-2xx status and the facade is NEVER invoked.
func TestWhatsAppSignatureE2E_TP_072_05_UnsignedNeverReachesFacade(t *testing.T) {
	const appSecret = "test-app-secret-tp-072-05"

	cloud := &sigE2ECloud{}
	adapter, err := wa.NewAdapter(wa.Options{
		Verify:                    wa.HMACVerifier{AppSecret: appSecret, VerifyToken: "tok"},
		IdentityRegistry:          sigE2ERegistry{userID: "user-tp-072-05"},
		IdentityHashKey:           "test-hash-key",
		MaxTextChars:              4096,
		RateLimitPerUserPerMinute: 30,
		Cloud:                     cloud,
		Capture:                   func(context.Context, contracts.AssistantMessage) {},
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	fac := &sigE2EFacade{}
	if err := adapter.Start(context.Background(), &sigE2EBound{inner: fac}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	r := chi.NewRouter()
	r.Handle("/v1/assistant/transports/whatsapp/webhook", wa.NewWebhookHandler(wa.WebhookHandlerOptions{Adapter: adapter}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	body := sigE2ETextPayload("wamid.tp-072-05.1")
	other := sigE2ETextPayload("wamid.tp-072-05.2")

	cases := []struct {
		name      string
		signature string
		body      []byte
	}{
		{"missing", "", body},
		{"wrong_prefix", "md5=" + hex.EncodeToString([]byte("nope")), body},
		{"wrong_secret", sigE2ESign(body, "different-secret"), body},
		{"tampered_body", sigE2ESign(other, appSecret), body},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(tc.body))
			if tc.signature != "" {
				req.Header.Set(wa.SignatureHeader, tc.signature)
			}
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				t.Fatalf("status: want non-2xx for rejected signature, got %d", resp.StatusCode)
			}
		})
	}

	if got := fac.calls.Load(); got != 0 {
		t.Fatalf("facade invocations: want 0 across all rejection variants, got %d", got)
	}
	if got := cloud.sends.Load(); got != 0 {
		t.Fatalf("Cloud sends: want 0 across all rejection variants, got %d", got)
	}

	// Adversarial: a validly-signed delivery MUST reach the facade
	// — proves the rejection is signature-driven, not a global
	// gate that swallows every POST.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(body))
	req.Header.Set(wa.SignatureHeader, sigE2ESign(body, appSecret))
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("valid Do: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid delivery status: want 200, got %d", resp.StatusCode)
	}
	if got := fac.calls.Load(); got != 1 {
		t.Fatalf("facade invocations after valid delivery: want 1, got %d", got)
	}
}
