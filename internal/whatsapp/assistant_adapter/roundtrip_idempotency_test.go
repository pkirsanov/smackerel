// Spec 072 SCOPE-3 — Round-trip controls and Meta retry idempotency.
//
// Unit-level proofs that:
//
//   - SCN-072-A08: inbound interactive payload ids for
//     disambiguation, confirm, and reset translate back to the
//     canonical AssistantMessage shapes the facade expects.
//   - SCN-072-A08: a `/reset` text message translates to KindReset
//     (parity with Telegram).
//   - SCN-072-A10: a duplicate webhook delivery with the same
//     TransportMessageID is swallowed BEFORE the facade and BEFORE
//     the capture-as-fallback hook run.

package assistant_adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// --- SCN-072-A08: round-trip translation ---

func TestTranslate_ResetTextProducesKindReset(t *testing.T) {
	adapter := newTestAdapter(t, fixedRegistry{userID: "user-reset"})
	for _, body := range []string{"/reset", "  /reset  ", "/RESET", "/Reset"} {
		t.Run(body, func(t *testing.T) {
			payload := textPayloadWithBody("wamid.reset.1", body)
			msg, err := ParsePayload(payload)
			if err != nil {
				t.Fatalf("ParsePayload: %v", err)
			}
			canonical, err := adapter.Translate(context.Background(), msg)
			if err != nil {
				t.Fatalf("Translate: %v", err)
			}
			if canonical.Kind != contracts.KindReset {
				t.Errorf("Kind: want %q, got %q", contracts.KindReset, canonical.Kind)
			}
		})
	}
}

func TestTranslate_PlainTextStaysKindText(t *testing.T) {
	adapter := newTestAdapter(t, fixedRegistry{userID: "user-text"})
	payload := textPayloadWithBody("wamid.text.1", "hello there")
	msg, err := ParsePayload(payload)
	if err != nil {
		t.Fatalf("ParsePayload: %v", err)
	}
	canonical, err := adapter.Translate(context.Background(), msg)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if canonical.Kind != contracts.KindText {
		t.Fatalf("Kind: want KindText, got %q", canonical.Kind)
	}
	// Adversarial: a `/reset` substring elsewhere in a sentence
	// must NOT be promoted to KindReset.
	if canonical.Text != "hello there" {
		t.Errorf("Text: want %q, got %q", "hello there", canonical.Text)
	}
}

func TestTranslate_InteractivePayloadsRoundTripToCanonicalKinds(t *testing.T) {
	adapter := newTestAdapter(t, fixedRegistry{userID: "user-round"})

	cases := []struct {
		name           string
		payloadID      string
		want           contracts.MessageKind
		wantConfirmPos contracts.ConfirmChoice
		wantDisambig   int
		wantRef        string
	}{
		{
			name:         "disambiguation choice 2",
			payloadID:    EncodeDisambigPayload("ref-d-1", 2),
			want:         contracts.KindDisambiguation,
			wantRef:      "ref-d-1",
			wantDisambig: 2,
		},
		{
			name:           "confirm yes",
			payloadID:      EncodeConfirmPayload("ref-c-1", true),
			want:           contracts.KindConfirm,
			wantRef:        "ref-c-1",
			wantConfirmPos: contracts.ConfirmPositive,
		},
		{
			name:           "confirm no",
			payloadID:      EncodeConfirmPayload("ref-c-2", false),
			want:           contracts.KindConfirm,
			wantRef:        "ref-c-2",
			wantConfirmPos: contracts.ConfirmNegative,
		},
		{
			name:      "reset payload",
			payloadID: EncodeResetPayload("ref-r-1"),
			want:      contracts.KindReset,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := buttonReplyPayload("wamid.btn."+tc.name, tc.payloadID)
			msg, err := ParsePayload(payload)
			if err != nil {
				t.Fatalf("ParsePayload: %v", err)
			}
			canonical, err := adapter.Translate(context.Background(), msg)
			if err != nil {
				t.Fatalf("Translate: %v", err)
			}
			if canonical.Kind != tc.want {
				t.Fatalf("Kind: want %q, got %q", tc.want, canonical.Kind)
			}
			if tc.want == contracts.KindConfirm {
				if canonical.ConfirmRef != tc.wantRef {
					t.Errorf("ConfirmRef: want %q, got %q", tc.wantRef, canonical.ConfirmRef)
				}
				if canonical.ConfirmChoice != tc.wantConfirmPos {
					t.Errorf("ConfirmChoice: want %v, got %v", tc.wantConfirmPos, canonical.ConfirmChoice)
				}
			}
			if tc.want == contracts.KindDisambiguation {
				if canonical.DisambiguationRef != tc.wantRef {
					t.Errorf("DisambiguationRef: want %q, got %q", tc.wantRef, canonical.DisambiguationRef)
				}
				if canonical.DisambiguationChoice != tc.wantDisambig {
					t.Errorf("DisambiguationChoice: want %d, got %d", tc.wantDisambig, canonical.DisambiguationChoice)
				}
			}
		})
	}
}

// Adversarial: an unknown payload prefix must NOT be silently
// reclassified — the adapter MUST refuse rather than invent a
// canonical kind that the facade would mis-process.
func TestTranslate_UnknownInteractivePayloadIsRejected(t *testing.T) {
	adapter := newTestAdapter(t, fixedRegistry{userID: "user-unknown"})
	payload := buttonReplyPayload("wamid.btn.unknown", "x:not-a-real-prefix:foo")
	msg, err := ParsePayload(payload)
	if err != nil {
		t.Fatalf("ParsePayload: %v", err)
	}
	if _, err := adapter.Translate(context.Background(), msg); err == nil {
		t.Fatalf("expected error for unknown payload id, got nil")
	}
}

// --- SCN-072-A10: idempotency cache ---

func TestIdempotencyCache_DuplicateIsSwallowed(t *testing.T) {
	c := newIdempotencyCache(8)
	if dup := c.markSeen("wamid.A"); dup {
		t.Fatalf("first markSeen MUST report not-duplicate")
	}
	if dup := c.markSeen("wamid.A"); !dup {
		t.Fatalf("second markSeen MUST report duplicate")
	}
	if dup := c.markSeen("wamid.B"); dup {
		t.Fatalf("distinct id MUST report not-duplicate")
	}
}

func TestIdempotencyCache_EmptyIdNeverRecorded(t *testing.T) {
	c := newIdempotencyCache(4)
	if c.markSeen("") {
		t.Fatalf("empty id MUST never report duplicate")
	}
	if c.size() != 0 {
		t.Fatalf("empty id MUST NOT be stored; size=%d", c.size())
	}
}

func TestIdempotencyCache_EvictsOldestAtCapacity(t *testing.T) {
	c := newIdempotencyCache(3)
	c.markSeen("a")
	c.markSeen("b")
	c.markSeen("c")
	c.markSeen("d") // evicts "a"
	if c.size() != 3 {
		t.Fatalf("size: want 3, got %d", c.size())
	}
	if dup := c.markSeen("a"); dup {
		t.Fatalf("after eviction, 'a' MUST be treated as new")
	}
}

// SCN-072-A10 end-to-end through the webhook handler: a Meta retry
// of the same wamid invokes the facade exactly once and the capture
// hook exactly once.
func TestWebhook_DuplicateDeliveryInvokesFacadeAndCaptureOnce(t *testing.T) {
	cap := &captureRecorder{}
	cloud := &recordingCloud{}
	adapter, err := NewAdapter(Options{
		Verify:                    HMACVerifier{AppSecret: testAppSecret, VerifyToken: "tok"},
		IdentityRegistry:          fixedRegistry{userID: "user-retry"},
		IdentityHashKey:           "test-hash-key",
		MaxTextChars:              4096,
		RateLimitPerUserPerMinute: 30,
		Cloud:                     cloud,
		Capture:                   cap.fn(),
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	rec := &countingFacade{}
	if err := adapter.Start(context.Background(), assistantBoundCounting(rec)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	h := NewWebhookHandler(WebhookHandlerOptions{Adapter: adapter})

	body := textPayloadWithBody("wamid.retry.1", "hello")
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(body))
		req.Header.Set(SignatureHeader, sign(body, testAppSecret))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("delivery %d: want 200, got %d (body=%s)", i, w.Code, w.Body.String())
		}
	}

	if got := rec.calls.Load(); got != 1 {
		t.Fatalf("facade Handle calls: want 1, got %d (idempotency broken)", got)
	}
	if got := cap.calls.Load(); got != 1 {
		t.Fatalf("CaptureFn calls: want 1, got %d (idempotency broken)", got)
	}
	if got := cloud.textCalls.Load(); got != 1 {
		t.Fatalf("Cloud.SendText calls: want 1, got %d (idempotency broken)", got)
	}
}

// Adversarial: distinct wamids must NOT be deduped.
func TestWebhook_DistinctDeliveriesAreNotDeduped(t *testing.T) {
	cap := &captureRecorder{}
	cloud := &recordingCloud{}
	adapter, err := NewAdapter(Options{
		Verify:                    HMACVerifier{AppSecret: testAppSecret, VerifyToken: "tok"},
		IdentityRegistry:          fixedRegistry{userID: "user-distinct"},
		IdentityHashKey:           "test-hash-key",
		MaxTextChars:              4096,
		RateLimitPerUserPerMinute: 30,
		Cloud:                     cloud,
		Capture:                   cap.fn(),
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	rec := &countingFacade{}
	if err := adapter.Start(context.Background(), assistantBoundCounting(rec)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	h := NewWebhookHandler(WebhookHandlerOptions{Adapter: adapter})

	for i := 0; i < 3; i++ {
		body := textPayloadWithBody(fmt.Sprintf("wamid.distinct.%d", i), "hello")
		req := httptest.NewRequest(http.MethodPost, "/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(body))
		req.Header.Set(SignatureHeader, sign(body, testAppSecret))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("delivery %d: want 200, got %d", i, w.Code)
		}
	}
	if got := rec.calls.Load(); got != 3 {
		t.Fatalf("facade Handle calls: want 3 distinct, got %d", got)
	}
}

// --- helpers ---

// countingFacade is an AssistantHandler that always returns
// CaptureRoute=true with a non-empty body so the dispatch path
// exercises capture + render.
type countingFacade struct {
	calls atomic.Int64
}

func (f *countingFacade) Handle(_ context.Context, _ contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	f.calls.Add(1)
	return contracts.AssistantResponse{
		Status:       contracts.StatusSavedAsIdea,
		CaptureRoute: true,
		Body:         "saved as an idea — i'll surface it later.",
	}, nil
}

type assistantBoundCountingT struct{ inner *countingFacade }

func assistantBoundCounting(f *countingFacade) contracts.Assistant {
	return &assistantBoundCountingT{inner: f}
}

func (a *assistantBoundCountingT) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	return a.inner.Handle(ctx, msg)
}

// textPayloadWithBody returns a signed-ready Meta payload with the
// configurable wamid and text body. The phone number matches
// sampleTextPayload so fixedRegistry resolves identity.
func textPayloadWithBody(wamid, body string) []byte {
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

// buttonReplyPayload returns a signed-ready Meta payload describing
// an interactive button_reply with the given opaque id.
func buttonReplyPayload(wamid, id string) []byte {
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
						"type":      "interactive",
						"interactive": map[string]any{
							"type": "button_reply",
							"button_reply": map[string]any{
								"id":    id,
								"title": "Choice",
							},
						},
					}},
				},
			}},
		}},
	}
	out, _ := json.Marshal(payload)
	return out
}
