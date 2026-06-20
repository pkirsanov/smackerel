// Spec 072 SCOPE-2 seam coverage — direct unit tests for the
// CloudClient injection seam's fail-loud behavior.
//
// design.md (reconciled 2026-06-16) documents that v1 ships only the
// CloudClient interface plus its cmd/core injection point, and that
// the phone-targeted render path "fails loud (whatsapp_adapter:
// RenderToPhone called without configured CloudClient)" until a
// concrete client is wired. Before this file that documented v1 seam
// behavior, the empty-destination guard, and the identity-only
// Render() contract error were not pinned by any direct test — they
// were merely guarded behind HasCloud()/identity resolution in the
// webhook handler and therefore unreachable from the handler path.
// These tests pin the exported seam directly and adversarially so a
// future concrete cloudapi wiring cannot regress the guards.

package assistant_adapter

import (
	"context"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// A constructed adapter with NO CloudClient MUST fail loud on the
// phone-targeted render path rather than nil-deref. Adversarial:
// removing the nil-cloud guard would panic on a.cloud.SendText and
// fail this test instead of returning the documented error.
func TestRenderToPhone_NilCloudClientFailsLoud(t *testing.T) {
	adapter := newTestAdapter(t, fixedRegistry{userID: "user-seam-1"})
	if adapter.HasCloud() {
		t.Fatal("precondition: newTestAdapter must construct without a CloudClient")
	}
	err := adapter.RenderToPhone(context.Background(), "+15555550123",
		contracts.AssistantResponse{Body: "hello"})
	if err == nil {
		t.Fatal("expected fail-loud error without configured CloudClient, got nil")
	}
	if !strings.Contains(err.Error(), "without configured CloudClient") {
		t.Fatalf("error message drift: %q", err.Error())
	}
}

// With a CloudClient wired but an empty destination phone,
// RenderToPhone MUST refuse BEFORE asking the Cloud API to send to an
// empty recipient. Adversarial: removing the empty-phone guard would
// dispatch SendText to "" and the cloud-not-called assertion would
// fail.
func TestRenderToPhone_EmptyDestinationFailsLoudAndDoesNotSend(t *testing.T) {
	cloud := &recordingCloud{}
	adapter, err := NewAdapter(Options{
		Verify:                    HMACVerifier{AppSecret: testAppSecret, VerifyToken: "tok"},
		IdentityRegistry:          fixedRegistry{userID: "user-seam-2"},
		IdentityHashKey:           "test-hash-key",
		MaxTextChars:              4096,
		RateLimitPerUserPerMinute: 30,
		Cloud:                     cloud,
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	for _, empty := range []string{"", "   "} {
		rerr := adapter.RenderToPhone(context.Background(), empty,
			contracts.AssistantResponse{Body: "hello"})
		if rerr == nil {
			t.Fatalf("phone %q: expected empty-destination error, got nil", empty)
		}
		if !strings.Contains(rerr.Error(), "empty destination phone") {
			t.Fatalf("phone %q: error drift: %q", empty, rerr.Error())
		}
	}
	if got := cloud.textCalls.Load(); got != 0 {
		t.Fatalf("Cloud.SendText MUST NOT be called for an empty destination; got %d", got)
	}
}

// The contracts.TransportAdapter.Render(identity, resp) entry point
// lacks a destination phone for WhatsApp and MUST return an
// actionable wiring error directing the caller to RenderToPhone —
// never silently succeed and drop the response.
func TestRender_IdentityOnlyEntryPointIsAWiringError(t *testing.T) {
	adapter := newTestAdapter(t, fixedRegistry{userID: "user-seam-3"})
	err := adapter.Render(context.Background(),
		contracts.TransportIdentity{UserID: "user-seam-3", Transport: TransportName},
		contracts.AssistantResponse{Body: "hello"})
	if err == nil {
		t.Fatal("Render(identity, resp) MUST return a wiring error, got nil")
	}
	if !strings.Contains(err.Error(), "RenderToPhone") {
		t.Fatalf("expected error to direct caller to RenderToPhone, got %q", err.Error())
	}
}
