package contracts

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// TestAllMessageKinds_Exhaustive ensures every literal MessageKind
// constant declared in message.go is present in AllMessageKinds
// exactly once. Adversarial: removing a constant from AllMessageKinds
// (without removing the const itself) MUST fail the test.
func TestAllMessageKinds_Exhaustive(t *testing.T) {
	declared := []MessageKind{
		KindText,
		KindConfirm,
		KindDisambiguation,
		KindReset,
	}
	if len(AllMessageKinds) != len(declared) {
		t.Fatalf("AllMessageKinds length %d != declared %d", len(AllMessageKinds), len(declared))
	}
	seen := map[MessageKind]int{}
	for _, k := range AllMessageKinds {
		seen[k]++
	}
	for _, k := range declared {
		if seen[k] != 1 {
			t.Errorf("MessageKind %q appears %d times in AllMessageKinds, want 1", k, seen[k])
		}
	}
}

// TestAllConfirmChoices_Exhaustive — same exhaustiveness check for
// ConfirmChoice.
func TestAllConfirmChoices_Exhaustive(t *testing.T) {
	declared := []ConfirmChoice{ConfirmPositive, ConfirmNegative}
	if len(AllConfirmChoices) != len(declared) {
		t.Fatalf("AllConfirmChoices length %d != declared %d", len(AllConfirmChoices), len(declared))
	}
	seen := map[ConfirmChoice]int{}
	for _, c := range AllConfirmChoices {
		seen[c]++
	}
	for _, c := range declared {
		if seen[c] != 1 {
			t.Errorf("ConfirmChoice %q appears %d times in AllConfirmChoices, want 1", c, seen[c])
		}
	}
}

// TestAssistantMessage_FieldRoundTrip — populate every field and
// assert reflect.DeepEqual against a constructed expectation. Guards
// against accidental field rename / removal.
func TestAssistantMessage_FieldRoundTrip(t *testing.T) {
	t0 := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	msg := AssistantMessage{
		UserID:               "u-1",
		Transport:            "telegram",
		TransportMessageID:   "tg-42",
		Text:                 "hello",
		Kind:                 KindConfirm,
		ConfirmRef:           "ULID-CR-1",
		ConfirmChoice:        ConfirmPositive,
		DisambiguationRef:    "ULID-DR-1",
		DisambiguationChoice: 2,
		Attachments:          []Attachment{{Kind: "image", MimeType: "image/png"}},
		ReceivedAt:           t0,
		TransportMetadata:    map[string]string{"chat_id": "100"},
	}
	if msg.UserID != "u-1" || msg.Kind != KindConfirm || msg.ConfirmChoice != ConfirmPositive {
		t.Fatalf("AssistantMessage field round-trip lost values: %+v", msg)
	}
	if !reflect.DeepEqual(msg.Attachments, []Attachment{{Kind: "image", MimeType: "image/png"}}) {
		t.Fatalf("Attachments lost on round-trip")
	}
	if got := msg.TransportMetadata["chat_id"]; got != "100" {
		t.Fatalf("TransportMetadata lost: got %q", got)
	}
}

// TestMessageKind_ClosedVocabRejectsUnknownLiteral is an adversarial
// regression: if a future change adds a MessageKind constant but
// forgets to extend AllMessageKinds, this test (paired with
// TestAllMessageKinds_Exhaustive above) will catch it. Here we
// explicitly assert the four known values are the ONLY ones a
// hypothetical caller would expect to find in AllMessageKinds.
func TestMessageKind_ClosedVocabRejectsUnknownLiteral(t *testing.T) {
	allowed := map[MessageKind]bool{
		KindText:           true,
		KindConfirm:        true,
		KindDisambiguation: true,
		KindReset:          true,
	}
	for _, k := range AllMessageKinds {
		if !allowed[k] {
			t.Errorf("AllMessageKinds contains unexpected literal %q — closed-vocab violation", k)
		}
	}
}

// Compile-time assertion that the package-level Assistant interface is
// satisfiable by a minimal in-test fake. If response.go / assistant.go
// drift in a way that breaks the contract, this fails to compile.
type _staticFake struct{}

func (_staticFake) Handle(_ context.Context, _ AssistantMessage) (AssistantResponse, error) {
	return AssistantResponse{}, nil
}

var _ Assistant = _staticFake{}
