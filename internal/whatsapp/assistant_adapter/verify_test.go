// TP-072-02 — unit: invalid Meta signatures reject before facade
// invocation. SCN-072-A02.

package assistant_adapter

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/transportidentity"
)

const testAppSecret = "test-app-secret-072-scope-1"

func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestHMACVerifier_Verify(t *testing.T) {
	v := HMACVerifier{AppSecret: testAppSecret, VerifyToken: "tok"}
	body := []byte(`{"hello":"world"}`)

	t.Run("valid signature accepted", func(t *testing.T) {
		if err := v.Verify(body, sign(body, testAppSecret)); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("missing signature rejected", func(t *testing.T) {
		if err := v.Verify(body, ""); !errors.Is(err, ErrInvalidSignature) {
			t.Fatalf("expected ErrInvalidSignature, got %v", err)
		}
	})

	t.Run("wrong prefix rejected", func(t *testing.T) {
		if err := v.Verify(body, "sha1=deadbeef"); !errors.Is(err, ErrInvalidSignature) {
			t.Fatalf("expected ErrInvalidSignature, got %v", err)
		}
	})

	t.Run("invalid hex rejected", func(t *testing.T) {
		if err := v.Verify(body, "sha256=not-hex"); !errors.Is(err, ErrInvalidSignature) {
			t.Fatalf("expected ErrInvalidSignature, got %v", err)
		}
	})

	t.Run("wrong secret rejected", func(t *testing.T) {
		if err := v.Verify(body, sign(body, "other-secret")); !errors.Is(err, ErrInvalidSignature) {
			t.Fatalf("expected ErrInvalidSignature, got %v", err)
		}
	})

	t.Run("tampered body rejected", func(t *testing.T) {
		sig := sign(body, testAppSecret)
		tampered := []byte(`{"hello":"WORLD"}`)
		if err := v.Verify(tampered, sig); !errors.Is(err, ErrInvalidSignature) {
			t.Fatalf("expected ErrInvalidSignature, got %v", err)
		}
	})

	t.Run("empty AppSecret returns config error", func(t *testing.T) {
		emptyV := HMACVerifier{AppSecret: "", VerifyToken: "tok"}
		err := emptyV.Verify(body, sign(body, testAppSecret))
		if err == nil || errors.Is(err, ErrInvalidSignature) {
			t.Fatalf("expected non-nil non-ErrInvalidSignature config error, got %v", err)
		}
	})
}

func TestHMACVerifier_VerifyChallenge(t *testing.T) {
	v := HMACVerifier{AppSecret: testAppSecret, VerifyToken: "expected-token"}

	if err := v.VerifyChallenge("expected-token"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if err := v.VerifyChallenge("wrong-token"); !errors.Is(err, ErrInvalidChallenge) {
		t.Fatalf("expected ErrInvalidChallenge, got %v", err)
	}
}

// recordingFacade records every Handle call. The webhook handler MUST
// NOT invoke Handle when the signature is invalid (SCN-072-A02).
type recordingFacade struct {
	calls int
}

func (r *recordingFacade) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	r.calls++
	return contracts.AssistantResponse{}, nil
}

// fixedRegistry resolves every hashed subject to the configured user.
type fixedRegistry struct {
	userID string
}

func (r fixedRegistry) Resolve(_ context.Context, _, _ string) (string, error) {
	if r.userID == "" {
		return "", transportidentity.ErrUnknownSubject
	}
	return r.userID, nil
}

const sampleTextPayload = `{
  "object": "whatsapp_business_account",
  "entry": [{
    "id": "biz-1",
    "changes": [{
      "field": "messages",
      "value": {
        "messaging_product": "whatsapp",
        "metadata": {"display_phone_number": "+15550001", "phone_number_id": "pid-1"},
        "messages": [{
          "id": "wamid.HBgN",
          "from": "+15555550123",
          "timestamp": "1700000000",
          "type": "text",
          "text": {"body": "hello"}
        }]
      }
    }]
  }]
}`

func newTestAdapter(t *testing.T, reg transportidentity.Registry) *Adapter {
	t.Helper()
	a, err := NewAdapter(Options{
		Verify:                    HMACVerifier{AppSecret: testAppSecret, VerifyToken: "tok"},
		IdentityRegistry:          reg,
		IdentityHashKey:           "test-hash-key",
		MaxTextChars:              4096,
		RateLimitPerUserPerMinute: 30,
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	return a
}

func TestWebhookHandler_RejectsUnsignedBeforeFacade(t *testing.T) {
	fac := &recordingFacade{}
	adapter := newTestAdapter(t, fixedRegistry{userID: "user-1"})
	if err := adapter.Start(context.Background(), assistantBound(fac)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	h := NewWebhookHandler(WebhookHandlerOptions{Adapter: adapter})

	body := []byte(sampleTextPayload)
	cases := []struct {
		name      string
		signature string
		wantCode  int
	}{
		{"missing", "", http.StatusUnauthorized},
		{"wrong prefix", "sha1=deadbeef", http.StatusUnauthorized},
		{"wrong secret", sign(body, "not-the-secret"), http.StatusUnauthorized},
		{"tampered body", sign([]byte(`{"different":"payload"}`), testAppSecret), http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(body))
			if tc.signature != "" {
				req.Header.Set(SignatureHeader, tc.signature)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("status: want %d, got %d (body=%s)", tc.wantCode, rec.Code, rec.Body.String())
			}
		})
	}
	if fac.calls != 0 {
		t.Fatalf("facade MUST NOT be invoked for invalid signatures; got %d call(s)", fac.calls)
	}
}

func TestWebhookHandler_AcceptsValidSignature(t *testing.T) {
	fac := &recordingFacade{}
	adapter := newTestAdapter(t, fixedRegistry{userID: "user-72"})
	if err := adapter.Start(context.Background(), assistantBound(fac)); err != nil {
		t.Fatalf("Start: %v", err)
	}
	h := NewWebhookHandler(WebhookHandlerOptions{Adapter: adapter})

	body := []byte(sampleTextPayload)
	req := httptest.NewRequest(http.MethodPost, "/v1/assistant/transports/whatsapp/webhook", bytes.NewReader(body))
	req.Header.Set(SignatureHeader, sign(body, testAppSecret))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if fac.calls != 1 {
		t.Fatalf("facade MUST be invoked exactly once for a verified delivery; got %d", fac.calls)
	}
}

// SCN-072-A01 — verified Translate produces canonical
// AssistantMessage{Transport:"whatsapp", TransportMessageID:<Meta id>}.
func TestTranslate_TextMessageProducesCanonicalAssistantMessage(t *testing.T) {
	adapter := newTestAdapter(t, fixedRegistry{userID: "user-canonical"})
	msg, err := ParsePayload([]byte(sampleTextPayload))
	if err != nil {
		t.Fatalf("ParsePayload: %v", err)
	}
	canonical, err := adapter.Translate(context.Background(), msg)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if canonical.Transport != TransportName {
		t.Errorf("Transport: want %q, got %q", TransportName, canonical.Transport)
	}
	if canonical.TransportMessageID != "wamid.HBgN" {
		t.Errorf("TransportMessageID: want %q, got %q", "wamid.HBgN", canonical.TransportMessageID)
	}
	if canonical.Kind != contracts.KindText {
		t.Errorf("Kind: want %q, got %q", contracts.KindText, canonical.Kind)
	}
	if canonical.Text != "hello" {
		t.Errorf("Text: want %q, got %q", "hello", canonical.Text)
	}
	if canonical.UserID != "user-canonical" {
		t.Errorf("UserID: want %q, got %q", "user-canonical", canonical.UserID)
	}
}

// SCN-072-A01 supporting check — unknown phone subject is refused
// before facade invocation.
func TestTranslate_UnknownSubjectRefused(t *testing.T) {
	adapter := newTestAdapter(t, fixedRegistry{userID: ""}) // ErrUnknownSubject
	msg, err := ParsePayload([]byte(sampleTextPayload))
	if err != nil {
		t.Fatalf("ParsePayload: %v", err)
	}
	if _, err := adapter.Translate(context.Background(), msg); !errors.Is(err, transportidentity.ErrUnknownSubject) {
		t.Fatalf("expected ErrUnknownSubject, got %v", err)
	}
}

// assistantBound adapts a recordingFacade to contracts.Assistant by
// composition; the production contracts.Assistant has additional
// methods this test does not exercise, so we wrap.
type assistantWrap struct{ inner *recordingFacade }

func assistantBound(r *recordingFacade) contracts.Assistant {
	return &assistantWrap{inner: r}
}

func (a *assistantWrap) Handle(ctx context.Context, msg contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	return a.inner.Handle(ctx, msg)
}
