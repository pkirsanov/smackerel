//go:build e2e

// Spec 072 SCOPE-2 — TP-072-10 / SCN-072-A03.
//
// Live-stack regression that a facade-produced AssistantResponse
// containing a three-choice DisambiguationPrompt is rendered as a
// WhatsApp interactive-button message and dispatched through the
// CloudClient with three buttons whose payload ids round-trip the
// disambiguation_ref and choice number per design.md "Outbound
// Render Mapping".

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
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	wa "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

type renderE2ERegistry struct{ userID string }

func (r renderE2ERegistry) Resolve(_ context.Context, _, _ string) (string, error) {
	return r.userID, nil
}

type renderE2ECloud struct {
	mu              sync.Mutex
	textCalls       atomic.Int64
	interactive     []wa.InteractiveMessage
	lastInteractive wa.InteractiveMessage
}

func (c *renderE2ECloud) SendText(_ context.Context, _ string, _ wa.TextMessage) error {
	c.textCalls.Add(1)
	return nil
}
func (c *renderE2ECloud) SendInteractive(_ context.Context, _ string, msg wa.InteractiveMessage) error {
	c.mu.Lock()
	c.interactive = append(c.interactive, msg)
	c.lastInteractive = msg
	c.mu.Unlock()
	return nil
}

type disambigFacade struct {
	ref string
}

func (f *disambigFacade) Handle(_ context.Context, _ contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	return contracts.AssistantResponse{
		Body: "Which one did you mean?",
		DisambiguationPrompt: &contracts.DisambiguationPrompt{
			DisambiguationRef: f.ref,
			Timeout:           5 * time.Minute,
			Choices: []contracts.DisambiguationChoice{
				{Number: 1, ID: "weather.lookup", Label: "Check the weather"},
				{Number: 2, ID: "recipe.search", Label: "Search a recipe"},
				{Number: 3, ID: contracts.SaveAsNoteChoiceID, Label: "Save as a note"},
			},
		},
	}, nil
}

type disambigBound struct{ inner *disambigFacade }

func (b *disambigBound) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	return b.inner.Handle(ctx, msg)
}

func renderE2EPayload(wamid, body string) []byte {
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
						"text":      map[string]any{"body": body},
					}},
				},
			}},
		}},
	})
	return out
}

func renderE2ESign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// TestWhatsAppRenderE2E_TP_072_10_DisambiguationRendersAsButtons
// asserts SCN-072-A03: a three-choice DisambiguationPrompt is sent
// to the user as a WhatsApp interactive-button message; the button
// payload ids carry (disambiguation_ref, choice number) and
// round-trip through DecodeDisambigPayload so the next inbound
// turn can resolve the selection.
func TestWhatsAppRenderE2E_TP_072_10_DisambiguationRendersAsButtons(t *testing.T) {
	const appSecret = "test-app-secret-tp-072-10"
	const wamid = "wamid.HBgN.tp-072-10"
	const ref = "DR01HW72A03"

	cloud := &renderE2ECloud{}
	adapter, err := wa.NewAdapter(wa.Options{
		Verify:                    wa.HMACVerifier{AppSecret: appSecret, VerifyToken: "tok"},
		IdentityRegistry:          renderE2ERegistry{userID: "user-tp-072-10"},
		IdentityHashKey:           "test-hash-key",
		MaxTextChars:              4096,
		RateLimitPerUserPerMinute: 30,
		Cloud:                     cloud,
		Capture:                   func(context.Context, contracts.AssistantMessage) {},
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	if err := adapter.Start(context.Background(), &disambigBound{inner: &disambigFacade{ref: ref}}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	r := chi.NewRouter()
	r.Handle("/v1/assistant/transports/whatsapp/webhook", wa.NewWebhookHandler(wa.WebhookHandlerOptions{Adapter: adapter}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	body := renderE2EPayload(wamid, "weather or recipe?")
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(body))
	req.Header.Set(wa.SignatureHeader, renderE2ESign(body, appSecret))
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}

	cloud.mu.Lock()
	defer cloud.mu.Unlock()
	if got := cloud.textCalls.Load(); got != 0 {
		t.Errorf("Cloud.SendText calls: want 0 for disambiguation, got %d", got)
	}
	if len(cloud.interactive) != 1 {
		t.Fatalf("Cloud.SendInteractive calls: want 1, got %d", len(cloud.interactive))
	}
	msg := cloud.lastInteractive
	if msg.Kind != wa.OutboundInteractiveButtons {
		t.Fatalf("interactive Kind: want %q, got %q", wa.OutboundInteractiveButtons, msg.Kind)
	}
	if len(msg.Buttons) != 3 {
		t.Fatalf("buttons: want 3, got %d", len(msg.Buttons))
	}
	for i, btn := range msg.Buttons {
		gotRef, gotChoice, ok := wa.DecodeDisambigPayload(btn.ID)
		if !ok {
			t.Errorf("button %d: payload id %q is not a disambig round-trip token", i, btn.ID)
			continue
		}
		if gotRef != ref {
			t.Errorf("button %d ref: want %q, got %q", i, ref, gotRef)
		}
		if gotChoice != i+1 {
			t.Errorf("button %d choice: want %d, got %d", i, i+1, gotChoice)
		}
		if btn.Title == "" {
			t.Errorf("button %d title: empty (user-visible label MUST be human-readable)", i)
		}
	}

	// Adversarial: the user-visible button labels MUST NOT contain
	// the opaque payload id — design.md "opaque payload ids,
	// never visible labels".
	for i, btn := range msg.Buttons {
		if bytes.Contains([]byte(btn.Title), []byte(btn.ID)) {
			t.Errorf("button %d title leaks payload id: title=%q id=%q", i, btn.Title, btn.ID)
		}
	}
}
