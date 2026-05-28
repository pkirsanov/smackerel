package assistant_adapter

import (
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// TestCallbackRoundTrip exercises encodeConfirmCallback /
// encodeDisambigCallback against decodeCallbackData for the full
// closed vocabulary of choices, asserting that the (ref, choice)
// and (ref, number) pairs survive the encode → decode round trip
// without truncation. The 64-byte Telegram callback_data ceiling
// is not enforced here (capability layer emits 26-char ULIDs that
// fit comfortably); the round-trip property is what matters for
// callbacks.go.
func TestCallbackRoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("confirm pos", func(t *testing.T) {
		t.Parallel()
		data := encodeConfirmCallback("01HXY9ZZZZAAAA1111BBBB2222", contracts.ConfirmPositive)
		if !IsAssistantCallback(data) {
			t.Fatalf("IsAssistantCallback(%q) = false; want true", data)
		}
		dec, err := decodeCallbackData(data)
		if err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if dec.kind != callbackKindConfirm {
			t.Errorf("kind = %v; want %v", dec.kind, callbackKindConfirm)
		}
		if dec.ref != "01HXY9ZZZZAAAA1111BBBB2222" {
			t.Errorf("ref = %q; want round-trip", dec.ref)
		}
		if dec.choice != contracts.ConfirmPositive {
			t.Errorf("choice = %v; want positive", dec.choice)
		}
	})

	t.Run("confirm neg", func(t *testing.T) {
		t.Parallel()
		data := encodeConfirmCallback("01HRESETXYZ", contracts.ConfirmNegative)
		dec, err := decodeCallbackData(data)
		if err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if dec.choice != contracts.ConfirmNegative {
			t.Errorf("choice = %v; want negative", dec.choice)
		}
	})

	for _, n := range []int{1, 2, 3} {
		t.Run("disambig", func(t *testing.T) {
			ref := "01HDISAMBIGREF12345"
			data := encodeDisambigCallback(ref, n)
			if !IsAssistantCallback(data) {
				t.Fatalf("IsAssistantCallback(%q) = false; want true", data)
			}
			dec, err := decodeCallbackData(data)
			if err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if dec.kind != callbackKindDisambig {
				t.Errorf("kind = %v; want %v", dec.kind, callbackKindDisambig)
			}
			if dec.ref != ref {
				t.Errorf("ref = %q; want %q", dec.ref, ref)
			}
			if dec.number != n {
				t.Errorf("number = %d; want %d", dec.number, n)
			}
		})
	}
}

// TestCallbackUnknownPrefixReturnsSentinel asserts that callback_data
// not bearing the "a:" assistant prefix is reported via the
// ErrNotAssistantMessage sentinel so the bot's safeHandleCallback
// can fall through to its existing list/cook/expense handler.
func TestCallbackUnknownPrefixReturnsSentinel(t *testing.T) {
	t.Parallel()
	tests := []string{
		"list:add:foo",
		"cook:42",
		"",
	}
	for _, in := range tests {
		_, err := decodeCallbackData(in)
		if err == nil {
			t.Errorf("decodeCallbackData(%q) error = nil; want non-nil", in)
			continue
		}
		if err != ErrNotAssistantMessage {
			t.Errorf("decodeCallbackData(%q) error = %v; want ErrNotAssistantMessage", in, err)
		}
		if IsAssistantCallback(in) {
			t.Errorf("IsAssistantCallback(%q) = true; want false", in)
		}
	}
}

// TestCallbackMalformedAssistant asserts the adapter refuses
// malformed assistant callbacks with a descriptive error (NOT the
// fallthrough sentinel) so the bot can log + ack the callback
// without misattributing the payload.
func TestCallbackMalformedAssistant(t *testing.T) {
	t.Parallel()
	tests := []string{
		"a:c:",
		"a:c:ref:",
		"a:c:ref:maybe",
		"a:d:",
		"a:d:ref:",
		"a:d:ref:notanum",
		"a:d:ref:0",
		"a:d:ref:-1",
		"a:x:weird",
	}
	for _, in := range tests {
		_, err := decodeCallbackData(in)
		if err == nil {
			t.Errorf("decodeCallbackData(%q) error = nil; want error", in)
			continue
		}
		if err == ErrNotAssistantMessage {
			t.Errorf("decodeCallbackData(%q) = ErrNotAssistantMessage; want descriptive error", in)
		}
	}
}
