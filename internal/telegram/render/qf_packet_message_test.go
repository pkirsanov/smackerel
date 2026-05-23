package render

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
)

// SCN-SM-041-029 — EmitSignedCallback is a pass-through to
// qfdecisions.PostCallback. This test exercises the wiring end-to-end
// against an httptest server returning CALLBACK_DEFERRED_TO_V1 and
// confirms the exported behaviour (status=rejected_v1_deferred, no
// retry, no local action acceptance) is preserved by the render-layer
// entry point.
func TestEmitSignedCallbackPreMVPRejectionWiringMatchesPostCallback(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"CALLBACK_DEFERRED_TO_V1","message":"pre-MVP"}`))
	}))
	defer server.Close()

	client := qfdecisions.NewClient(server.URL, "bridge-test-token", 1, 100)
	keystoreRaw := `[{"key_id":"k-render","secret":"s-render","not_before":"2026-01-01T00:00:00Z"}]`
	keystore, err := qfdecisions.LoadCallbackKeystoreFromJSON(keystoreRaw)
	if err != nil {
		t.Fatalf("LoadCallbackKeystoreFromJSON: %v", err)
	}
	now, _ := time.Parse(time.RFC3339, "2026-05-22T12:00:00Z")
	signer := qfdecisions.NewCallbackSigner(keystore, func() time.Time { return now.UTC() })

	env := qfdecisions.CallbackEnvelope{
		CallbackID: "cb-render-001",
		TraceID:    "tr-render-001",
		PacketID:   "pk-render-001",
		Action:     qfdecisions.CallbackActionOpen,
		Nonce:      "no-render-001",
		ExpiresAt:  "2026-05-22T12:00:30Z",
		Surface:    qfdecisions.SurfaceTelegram,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := EmitSignedCallback(ctx, client, signer, env)
	if err != nil {
		t.Fatalf("EmitSignedCallback: %v", err)
	}
	if result.Status != qfdecisions.CallbackStatusRejectedV1Deferred {
		t.Fatalf("result.Status: want %q, got %q", qfdecisions.CallbackStatusRejectedV1Deferred, result.Status)
	}
	if result.QFResponse.RejectionCode != qfdecisions.CallbackRejectionCodeDeferredV1 {
		t.Fatalf("RejectionCode: want %q, got %q", qfdecisions.CallbackRejectionCodeDeferredV1, result.QFResponse.RejectionCode)
	}
	if hits != 1 {
		t.Fatalf("server hits: want 1 (no retry), got %d", hits)
	}
	if result.Envelope.Signature == "" {
		t.Fatal("envelope Signature is empty after EmitSignedCallback")
	}
	if result.Envelope.KeyID != "k-render" {
		t.Fatalf("envelope KeyID: want k-render, got %q", result.Envelope.KeyID)
	}
}
