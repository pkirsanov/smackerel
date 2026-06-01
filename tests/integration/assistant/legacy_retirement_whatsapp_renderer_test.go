//go:build integration

// Spec 075 SCOPE-6.4 — TP-075-21.
//
// Integration row: the WhatsApp transport renderer (Render in
// internal/whatsapp/assistant_adapter) appends the spec 075
// LegacyRetirementNotice as a short message addendum to the
// rendered body without replacing or blocking the primary
// response. Covers SCN-075-A01 at the WhatsApp transport boundary.
//
// Live-system rationale: the renderer is pure-Go but ships in the
// outbound path that the live WhatsApp Cloud integration drives.
// This row pins the wire-visible shape (text body + addendum) the
// production cloud client will deliver.

package assistant_integration

import (
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	wa "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

func TestWhatsAppRenderer_TP_075_21_NoticeAppendedAsAddendum(t *testing.T) {
	resp := contracts.AssistantResponse{
		Body: "Sunny, 22°C tomorrow.",
		LegacyRetirementNotice: &contracts.NoticePayload{
			Command:            "/weather",
			ReplacementExample: "weather in Barcelona tomorrow",
			CopyKey:            "spec066-weather",
			WindowID:           "tp-075-21-window",
		},
	}
	out, err := wa.Render(resp, 4096)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out.Kind != wa.OutboundText || out.Text == nil {
		t.Fatalf("Render kind = %q text=%v; want text outbound", out.Kind, out.Text)
	}
	wantPrimary := "Sunny, 22°C tomorrow."
	if !strings.Contains(out.Text.Body, wantPrimary) {
		t.Errorf("rendered body missing primary content %q; got %q", wantPrimary, out.Text.Body)
	}
	wantAddendum := wa.LegacyRetirementNoticeAddendum(resp.LegacyRetirementNotice)
	if wantAddendum == "" {
		t.Fatal("LegacyRetirementNoticeAddendum returned empty; setup bug")
	}
	if !strings.Contains(out.Text.Body, wantAddendum) {
		t.Errorf("rendered body missing notice addendum %q; got %q", wantAddendum, out.Text.Body)
	}
	// Primary must precede addendum (addendum is appended, not
	// prepended — proves it is a tail "heads up" not a blocking
	// replacement of the answer).
	if i, j := strings.Index(out.Text.Body, wantPrimary), strings.Index(out.Text.Body, wantAddendum); i < 0 || j < 0 || i >= j {
		t.Errorf("addendum ordering wrong: primary at %d, addendum at %d, body=%q", i, j, out.Text.Body)
	}
}

func TestWhatsAppRenderer_TP_075_21_NoNotice_NoAddendum(t *testing.T) {
	resp := contracts.AssistantResponse{Body: "ok"}
	out, err := wa.Render(resp, 4096)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out.Text == nil || out.Text.Body != "ok" {
		t.Fatalf("body mutated when no notice attached: %+v", out.Text)
	}
}
