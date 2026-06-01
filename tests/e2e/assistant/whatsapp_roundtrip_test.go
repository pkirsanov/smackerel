//go:build e2e

// Spec 072 SCOPE-3 — TP-072-11 / SCN-072-A08.
//
// Round-trip controls. Proves disambiguation, confirm, and reset
// interactive replies sent by a WhatsApp client translate, through
// the real chi-mounted webhook handler, to the same canonical
// AssistantMessage shapes the facade is invoked with for the
// equivalent Telegram and HTTP transports. The facade is recorded
// (not the full live facade) so the test asserts the *transport
// contract*: the WhatsApp adapter MUST present these three control
// shapes to the shared facade identically — same Kind, same
// ConfirmRef / DisambiguationRef / choice number / ConfirmChoice —
// so the shared scenario handlers cannot tell the transports apart.

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
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	wa "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

type wrtFixedRegistry struct{ userID string }

func (r wrtFixedRegistry) Resolve(_ context.Context, _, _ string) (string, error) {
	return r.userID, nil
}

type wrtCloud struct{}

func (wrtCloud) SendText(_ context.Context, _ string, _ wa.TextMessage) error { return nil }
func (wrtCloud) SendInteractive(_ context.Context, _ string, _ wa.InteractiveMessage) error {
	return nil
}

type wrtFacade struct {
	mu       sync.Mutex
	observed []contracts.AssistantMessage
}

func (f *wrtFacade) Handle(_ context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	f.mu.Lock()
	f.observed = append(f.observed, msg)
	f.mu.Unlock()
	return contracts.AssistantResponse{Body: "ok"}, nil
}

type wrtBound struct{ inner *wrtFacade }

func (b *wrtBound) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	return b.inner.Handle(ctx, msg)
}

func wrtBoundAssistant(f *wrtFacade) contracts.Assistant { return &wrtBound{inner: f} }

func wrtSign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// TestWhatsAppRoundTrip_TP_072_11_ControlsRoundTripIdentically
// SCN-072-A08. Drives a disambiguation pick, a confirm-accept, and
// a /reset text turn through the real webhook ingress and asserts
// every facade-visible field matches what Telegram/HTTP would emit
// for the same canonical control payload.
func TestWhatsAppRoundTrip_TP_072_11_ControlsRoundTripIdentically(t *testing.T) {
	const appSecret = "test-app-secret-072-tp-11"
	fac := &wrtFacade{}
	adapter, err := wa.NewAdapter(wa.Options{
		Verify:                    wa.HMACVerifier{AppSecret: appSecret, VerifyToken: "tok"},
		IdentityRegistry:          wrtFixedRegistry{userID: "user-tp-072-11"},
		IdentityHashKey:           "test-hash-key",
		MaxTextChars:              4096,
		RateLimitPerUserPerMinute: 30,
		Cloud:                     wrtCloud{},
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	if err := adapter.Start(context.Background(), wrtBoundAssistant(fac)); err != nil {
		t.Fatalf("Start: %v", err)
	}

	r := chi.NewRouter()
	r.Handle("/v1/assistant/transports/whatsapp/webhook", wa.NewWebhookHandler(wa.WebhookHandlerOptions{Adapter: adapter}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	deliver := func(wamid string, body []byte) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		req.Header.Set(wa.SignatureHeader, wrtSign(body, appSecret))
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("deliver %s: %v", wamid, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("deliver %s: status want 200, got %d", wamid, resp.StatusCode)
		}
	}

	// (a) Disambiguation: user picks choice 2 of disambiguation ref
	// "ref-disambig-A".
	deliver("wamid.tp11.dis", wrtInteractivePayload("wamid.tp11.dis", wa.EncodeDisambigPayload("ref-disambig-A", 2)))

	// (b) Confirm-accept: user accepts confirm ref "ref-confirm-B".
	deliver("wamid.tp11.con", wrtInteractivePayload("wamid.tp11.con", wa.EncodeConfirmPayload("ref-confirm-B", true)))

	// (c) Reset via text command (parity with Telegram /reset).
	deliver("wamid.tp11.rst", wrtTextPayload("wamid.tp11.rst", "/reset"))

	fac.mu.Lock()
	defer fac.mu.Unlock()
	if len(fac.observed) != 3 {
		t.Fatalf("facade observations: want 3, got %d", len(fac.observed))
	}
	dis, con, rst := fac.observed[0], fac.observed[1], fac.observed[2]

	if dis.Kind != contracts.KindDisambiguation ||
		dis.DisambiguationRef != "ref-disambig-A" ||
		dis.DisambiguationChoice != 2 ||
		dis.Transport != "whatsapp" ||
		dis.UserID != "user-tp-072-11" ||
		dis.TransportMessageID != "wamid.tp11.dis" {
		t.Errorf("disambig drift: %+v", dis)
	}
	if con.Kind != contracts.KindConfirm ||
		con.ConfirmRef != "ref-confirm-B" ||
		con.ConfirmChoice != contracts.ConfirmPositive ||
		con.Transport != "whatsapp" ||
		con.TransportMessageID != "wamid.tp11.con" {
		t.Errorf("confirm drift: %+v", con)
	}
	if rst.Kind != contracts.KindReset ||
		rst.Transport != "whatsapp" ||
		rst.TransportMessageID != "wamid.tp11.rst" {
		t.Errorf("reset drift: %+v", rst)
	}
}

func wrtTextPayload(wamid, body string) []byte {
	p := map[string]any{
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
	out, _ := json.Marshal(p)
	return out
}

func wrtInteractivePayload(wamid, payloadID string) []byte {
	p := map[string]any{
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
						"type":      "interactive",
						"interactive": map[string]any{
							"type": "button_reply",
							"button_reply": map[string]any{
								"id":    payloadID,
								"title": "Choice",
							},
						},
					}},
				},
			}},
		}},
	}
	out, _ := json.Marshal(p)
	return out
}
