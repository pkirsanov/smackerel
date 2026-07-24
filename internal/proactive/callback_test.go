package proactive

import (
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
)

// a 26-char ULID-shaped opaque ref literal for wire tests.
const sampleRef NudgeRef = "01HZY2ABCDEFGHJKMNPQRSTVWX"

func TestEncodeNudgeCallback_RoundTrip(t *testing.T) {
	cases := []struct {
		action NudgeAction
		suffix string
	}{
		{ActionAct, "a"},
		{ActionSnooze, "s"},
		{ActionDismiss, "d"},
	}
	for _, tc := range cases {
		wire, ok := EncodeNudgeCallback(sampleRef, tc.action)
		if !ok {
			t.Fatalf("EncodeNudgeCallback(%v) ok = false", tc.action)
		}
		want := "a:n:" + string(sampleRef) + ":" + tc.suffix
		if wire != want {
			t.Errorf("wire = %q, want %q", wire, want)
		}
		ref, action, dok := DecodeNudgeCallback(wire)
		if !dok {
			t.Fatalf("DecodeNudgeCallback(%q) ok = false", wire)
		}
		if ref != sampleRef || action != tc.action {
			t.Errorf("decoded (%q,%v), want (%q,%v)", ref, action, sampleRef, tc.action)
		}
	}
}

// TestEncodeNudgeCallback_WithinByteBudget proves a full a:n: callback stays
// within Telegram's 64-byte callback_data bound.
func TestEncodeNudgeCallback_WithinByteBudget(t *testing.T) {
	wire, ok := EncodeNudgeCallback(sampleRef, ActionDismiss)
	if !ok {
		t.Fatalf("encode ok = false")
	}
	if len(wire) != 32 {
		t.Errorf("len(%q) = %d, want 32 (a:n: + 26 + : + 1)", wire, len(wire))
	}
	if len(wire) > 64 {
		t.Errorf("callback %q exceeds Telegram's 64-byte bound", wire)
	}
}

func TestEncodeNudgeCallback_RejectsBadInput(t *testing.T) {
	if _, ok := EncodeNudgeCallback("", ActionAct); ok {
		t.Errorf("encode(empty ref) ok = true, want false")
	}
	if _, ok := EncodeNudgeCallback(sampleRef, ActionUnknown); ok {
		t.Errorf("encode(unknown action) ok = true, want false")
	}
	// A ref containing the delimiter would produce an ambiguous callback.
	if _, ok := EncodeNudgeCallback(NudgeRef("bad:ref"), ActionAct); ok {
		t.Errorf("encode(ref with ':') ok = true, want false")
	}
}

// TestDecodeNudgeCallback_NonCollision proves the a:n: decoder rejects the
// existing a:c:/a:d: families and malformed payloads, so it never mis-claims a
// non-nudge callback (FR-107-006).
func TestDecodeNudgeCallback_NonCollision(t *testing.T) {
	reject := []string{
		"",
		"a:",
		"a:n:",
		"a:n:" + string(sampleRef),          // missing action
		"a:n:" + string(sampleRef) + ":x",   // bad action byte
		"a:n::a",                            // empty ref
		"a:c:" + string(sampleRef) + ":pos", // confirm family
		"a:d:" + string(sampleRef) + ":2",   // disambig family
		"list:" + string(sampleRef) + ":a",  // spec-028 list namespace
		"a:n:" + string(sampleRef) + ":a:b", // trailing garbage
	}
	for _, payload := range reject {
		if _, _, ok := DecodeNudgeCallback(payload); ok {
			t.Errorf("DecodeNudgeCallback(%q) ok = true, want false", payload)
		}
	}
}

// TestNudgeCallback_CarriesOnlyRef is the wire half of the anti-leak boundary
// (FR-107-028): the callback string carries the opaque ref and NEVER a
// content_key. It complements the card-marshal leak test.
func TestNudgeCallback_CarriesOnlyRef(t *testing.T) {
	reg := NewNudgeRegistry(6 * time.Hour)
	secret := "artifact-super-secret-content-key"
	ref := reg.Mint(secret, surfacing.ProducerAlerts, surfacing.ChannelTelegram, "user-1")

	wire, ok := EncodeNudgeCallback(ref, ActionAct)
	if !ok {
		t.Fatalf("encode ok = false")
	}
	if strings.Contains(wire, secret) {
		t.Fatalf("callback %q leaks content_key %q", wire, secret)
	}
	if !strings.Contains(wire, string(ref)) {
		t.Errorf("callback %q missing opaque ref %q", wire, ref)
	}
}
