//go:build integration

// Spec 072 SCOPE-1 — TP-072-03 / SCN-072-A01.
//
// Integration row that proves the WhatsApp adapter's identity
// resolution uses ONLY the HMAC-SHA256 hash of the normalized E.164
// phone number when calling the transport identity registry — the
// raw phone string MUST never leave the adapter as a registry
// lookup key.

package assistant_integration

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/assistant/transportidentity"
	wa "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

type recordingRegistry struct {
	userID string

	mu     sync.Mutex
	lookup []recordingLookup
}

type recordingLookup struct {
	transport string
	subject   string
}

func (r *recordingRegistry) Resolve(_ context.Context, transport, subject string) (string, error) {
	r.mu.Lock()
	r.lookup = append(r.lookup, recordingLookup{transport: transport, subject: subject})
	r.mu.Unlock()
	return r.userID, nil
}

// TestTransportIdentity_TP_072_03_PhoneHashResolvesWithoutRawPhone
// asserts SCN-072-A01 + design.md §3 "Capability Foundation" + §8
// "Security/Compliance": the adapter calls the identity registry
// with the HMAC-SHA256 hash of the normalized E.164 phone — NOT the
// raw phone — and uses transport="whatsapp".
func TestTransportIdentity_TP_072_03_PhoneHashResolvesWithoutRawPhone(t *testing.T) {
	const appSecret = "test-app-secret-tp-072-03"
	const hashKey = "test-hash-key-tp-072-03"
	const rawPhone = "+15555550123"
	const wamid = "wamid.HBgN.tp-072-03"

	reg := &recordingRegistry{userID: "user-tp-072-03"}
	adapter, err := wa.NewAdapter(wa.Options{
		Verify:                    wa.HMACVerifier{AppSecret: appSecret, VerifyToken: "tok"},
		IdentityRegistry:          reg,
		IdentityHashKey:           hashKey,
		MaxTextChars:              4096,
		RateLimitPerUserPerMinute: 30,
		Cloud:                     &recordingCloud{},
		Capture:                   (&captureCounter{}).fn(),
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	fac := &recordingFacade{}
	if err := adapter.Start(context.Background(), boundAssistant(fac)); err != nil {
		t.Fatalf("Start: %v", err)
	}

	r := chi.NewRouter()
	r.Handle("/v1/assistant/transports/whatsapp/webhook", wa.NewWebhookHandler(wa.WebhookHandlerOptions{Adapter: adapter}))
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	body := waTextPayload(wamid, "hello identity")
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

	reg.mu.Lock()
	defer reg.mu.Unlock()
	if len(reg.lookup) != 1 {
		t.Fatalf("registry lookups: want 1, got %d", len(reg.lookup))
	}
	got := reg.lookup[0]
	if got.transport != wa.TransportName {
		t.Errorf("transport: want %q, got %q", wa.TransportName, got.transport)
	}
	wantHash, err := transportidentity.HashPhoneE164(hashKey, rawPhone)
	if err != nil {
		t.Fatalf("HashPhoneE164: %v", err)
	}
	if got.subject != wantHash {
		t.Errorf("subject hash: want %q, got %q", wantHash, got.subject)
	}
	// Adversarial: the registry MUST NOT receive the raw phone or
	// any substring of it as the lookup key.
	if strings.Contains(got.subject, rawPhone) {
		t.Errorf("subject contains raw phone %q: %q", rawPhone, got.subject)
	}
	if strings.Contains(got.subject, "15555550123") {
		t.Errorf("subject contains raw digits: %q", got.subject)
	}
	if len(got.subject) != 64 {
		t.Errorf("subject hash length: want 64 hex chars, got %d", len(got.subject))
	}
}
