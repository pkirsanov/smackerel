package assistant_adapter

import (
	"testing"

	"github.com/smackerel/smackerel/internal/proactive"
)

const nudgeTestRef proactive.NudgeRef = "01HZY2ABCDEFGHJKMNPQRSTVWX"

// TestDecodeCallbackData_NudgeFamily proves the additive a:n: nudge family
// decodes to callbackKindNudge with the opaque ref and action, routed through
// the shared foundation codec (spec 107 SCOPE-01, SCN-107-005/006).
func TestDecodeCallbackData_NudgeFamily(t *testing.T) {
	cases := []struct {
		suffix string
		action proactive.NudgeAction
	}{
		{"a", proactive.ActionAct},
		{"s", proactive.ActionSnooze},
		{"d", proactive.ActionDismiss},
	}
	for _, tc := range cases {
		data := "a:n:" + string(nudgeTestRef) + ":" + tc.suffix
		got, err := decodeCallbackData(data)
		if err != nil {
			t.Fatalf("decodeCallbackData(%q) err = %v", data, err)
		}
		if got.kind != callbackKindNudge {
			t.Errorf("kind = %v, want callbackKindNudge", got.kind)
		}
		if got.nudgeRef != nudgeTestRef {
			t.Errorf("nudgeRef = %q, want %q", got.nudgeRef, nudgeTestRef)
		}
		if got.nudgeAction != tc.action {
			t.Errorf("nudgeAction = %v, want %v", got.nudgeAction, tc.action)
		}
	}
}

// TestEncodeNudgeCallback_RoundTripsThroughDecode proves the Telegram encode
// wrapper produces a payload the adapter's own decoder accepts as a nudge.
func TestEncodeNudgeCallback_RoundTripsThroughDecode(t *testing.T) {
	wire, ok := encodeNudgeCallback(nudgeTestRef, proactive.ActionSnooze)
	if !ok {
		t.Fatalf("encodeNudgeCallback ok = false")
	}
	if !IsAssistantCallback(wire) {
		t.Fatalf("IsAssistantCallback(%q) = false, want true (a: namespace)", wire)
	}
	got, err := decodeCallbackData(wire)
	if err != nil {
		t.Fatalf("decodeCallbackData(%q) err = %v", wire, err)
	}
	if got.kind != callbackKindNudge || got.nudgeRef != nudgeTestRef || got.nudgeAction != proactive.ActionSnooze {
		t.Errorf("round-trip = %+v, want kind=nudge ref=%q action=snooze", got, nudgeTestRef)
	}
}

// TestDecodeCallbackData_NudgeDoesNotBreakConfirmOrDisambig is the non-collision
// guard: the existing a:c: / a:d: families still decode exactly as before after
// the a:n: family was added (FR-107-006). If this regresses, the two shipped
// assistant callback families break.
func TestDecodeCallbackData_NudgeDoesNotBreakConfirmOrDisambig(t *testing.T) {
	confirm, err := decodeCallbackData("a:c:" + string(nudgeTestRef) + ":pos")
	if err != nil {
		t.Fatalf("confirm decode err = %v", err)
	}
	if confirm.kind != callbackKindConfirm {
		t.Errorf("confirm kind = %v, want callbackKindConfirm", confirm.kind)
	}

	disambig, err := decodeCallbackData("a:d:" + string(nudgeTestRef) + ":2")
	if err != nil {
		t.Fatalf("disambig decode err = %v", err)
	}
	if disambig.kind != callbackKindDisambig {
		t.Errorf("disambig kind = %v, want callbackKindDisambig", disambig.kind)
	}
	if disambig.number != 2 {
		t.Errorf("disambig number = %d, want 2", disambig.number)
	}
}

// TestDecodeCallbackData_MalformedNudgeErrors proves a malformed a:n: payload
// surfaces an error rather than mis-routing.
func TestDecodeCallbackData_MalformedNudgeErrors(t *testing.T) {
	for _, bad := range []string{"a:n:", "a:n:ref", "a:n:ref:x", "a:n::a"} {
		if _, err := decodeCallbackData(bad); err == nil {
			t.Errorf("decodeCallbackData(%q) err = nil, want error", bad)
		}
	}
}
