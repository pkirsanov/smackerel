// TP-072-06 / TP-072-07 — unit golden tests for the WhatsApp
// outbound renderer. SCN-072-A03 (disambiguation -> buttons) and
// SCN-072-A04 (unknown response shape -> text fallback / observable
// error, never silent drop).

package assistant_adapter

import (
	"errors"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

const renderTestMaxChars = 4096

// TP-072-06 — SCN-072-A03: three-choice disambiguation renders as
// WhatsApp interactive buttons with opaque round-trip payload ids.
func TestRender_DisambiguationThreeChoicesProducesButtons(t *testing.T) {
	resp := contracts.AssistantResponse{
		Status: contracts.StatusThinking,
		Body:   "Which one did you mean?",
		DisambiguationPrompt: &contracts.DisambiguationPrompt{
			DisambiguationRef: "01HXYZ-DISAMBIG",
			Choices: []contracts.DisambiguationChoice{
				{Number: 1, ID: "weather", Label: "Weather"},
				{Number: 2, ID: "recipe", Label: "Recipe search"},
				{Number: 3, ID: contracts.SaveAsNoteChoiceID, Label: "Save as a note"},
			},
		},
	}
	out, err := Render(resp, renderTestMaxChars)
	if err != nil {
		t.Fatalf("Render: unexpected err: %v", err)
	}
	if out.Kind != OutboundInteractiveButtons {
		t.Fatalf("kind: want %q, got %q", OutboundInteractiveButtons, out.Kind)
	}
	if out.Interactive == nil {
		t.Fatalf("Interactive payload missing")
	}
	if out.Text != nil {
		t.Fatalf("Text MUST be nil for interactive kind")
	}
	if got := out.Interactive.Body; got != "Which one did you mean?" {
		t.Errorf("body: want %q, got %q", "Which one did you mean?", got)
	}
	if len(out.Interactive.Buttons) != 3 {
		t.Fatalf("buttons: want 3, got %d", len(out.Interactive.Buttons))
	}
	wantLabels := []string{"Weather", "Recipe search", "Save as a note"}
	for i, b := range out.Interactive.Buttons {
		if b.Title != wantLabels[i] {
			t.Errorf("button[%d].Title: want %q, got %q", i, wantLabels[i], b.Title)
		}
		ref, n, ok := DecodeDisambigPayload(b.ID)
		if !ok {
			t.Errorf("button[%d].ID = %q is not a decodable disambig payload", i, b.ID)
			continue
		}
		if ref != "01HXYZ-DISAMBIG" {
			t.Errorf("button[%d] ref: want %q, got %q", i, "01HXYZ-DISAMBIG", ref)
		}
		if n != i+1 {
			t.Errorf("button[%d] choice: want %d, got %d", i, i+1, n)
		}
	}
	// Round-trip: payloads decode back through Translate without
	// exposing user-visible labels.
	for i, b := range out.Interactive.Buttons {
		var canonical contracts.AssistantMessage
		if err := decodeInteractivePayload(b.ID, &canonical); err != nil {
			t.Fatalf("button[%d] round-trip decode failed: %v", i, err)
		}
		if canonical.Kind != contracts.KindDisambiguation {
			t.Errorf("button[%d] decoded kind: want %q, got %q", i, contracts.KindDisambiguation, canonical.Kind)
		}
		if canonical.DisambiguationChoice != i+1 {
			t.Errorf("button[%d] decoded choice: want %d, got %d", i, i+1, canonical.DisambiguationChoice)
		}
	}
}

// TP-072-06 — 4..10 choices render as an interactive list, NOT
// buttons (WhatsApp button cap is 3).
func TestRender_DisambiguationFiveChoicesProducesList(t *testing.T) {
	choices := []contracts.DisambiguationChoice{
		{Number: 1, ID: "a", Label: "Option A"},
		{Number: 2, ID: "b", Label: "Option B"},
		{Number: 3, ID: "c", Label: "Option C"},
		{Number: 4, ID: "d", Label: "Option D"},
		{Number: 5, ID: contracts.SaveAsNoteChoiceID, Label: "Save as a note"},
	}
	resp := contracts.AssistantResponse{
		Body: "Pick one",
		DisambiguationPrompt: &contracts.DisambiguationPrompt{
			DisambiguationRef: "REF-5",
			Choices:           choices,
		},
	}
	out, err := Render(resp, renderTestMaxChars)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out.Kind != OutboundInteractiveList {
		t.Fatalf("kind: want %q, got %q", OutboundInteractiveList, out.Kind)
	}
	if got := len(out.Interactive.ListSections); got != 1 || len(out.Interactive.ListSections[0].Rows) != 5 {
		t.Fatalf("list shape: want 1 section/5 rows, got %d sections, rows=%v", got, out.Interactive.ListSections)
	}
}

// TP-072-07 — SCN-072-A04: an AssistantResponse with no recognized
// shape and a non-empty Body MUST render as a plain text message
// (never silently dropped).
func TestRender_UnknownShapeFallsBackToText(t *testing.T) {
	resp := contracts.AssistantResponse{
		Status: contracts.StatusThinking,
		Body:   "Here is some text without confirm, disambig, sources, or error.",
	}
	out, err := Render(resp, renderTestMaxChars)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out.Kind != OutboundText {
		t.Fatalf("kind: want %q, got %q", OutboundText, out.Kind)
	}
	if out.Text == nil || !strings.Contains(out.Text.Body, "Here is some text") {
		t.Fatalf("text body missing or wrong: %#v", out.Text)
	}
}

// TP-072-07 — SCN-072-A04 negative half: empty AssistantResponse
// (no body, no confirm, no disambig, no error) MUST return an
// observable error rather than silently dropping.
func TestRender_EmptyResponseFailsObservably(t *testing.T) {
	resp := contracts.AssistantResponse{}
	_, err := Render(resp, renderTestMaxChars)
	if !errors.Is(err, ErrNothingToRender) {
		t.Fatalf("expected ErrNothingToRender, got %v", err)
	}
}

// SCN-072-A05 — capture acknowledgement is a plain text rendering of
// the facade-supplied "saved as an idea" body. The body string is
// owned by the facade so the renderer is byte-for-byte identical to
// what Telegram and HTTP emit when they pass the same Body through.
func TestRender_CaptureAcknowledgementRendersBodyText(t *testing.T) {
	resp := contracts.AssistantResponse{
		Status:       contracts.StatusSavedAsIdea,
		CaptureRoute: true,
		Body:         "saved as an idea — i'll surface it later.",
	}
	out, err := Render(resp, renderTestMaxChars)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out.Kind != OutboundText {
		t.Fatalf("capture ack must render as text, got %q", out.Kind)
	}
	if out.Text.Body != "saved as an idea — i'll surface it later." {
		t.Errorf("capture ack body drift: got %q", out.Text.Body)
	}
}

// SCN-072-A03 supporting — ConfirmCard renders two interactive
// buttons carrying opaque positive/negative payloads.
func TestRender_ConfirmCardProducesTwoButtons(t *testing.T) {
	resp := contracts.AssistantResponse{
		Body: "Set a reminder?",
		ConfirmCard: &contracts.ConfirmCard{
			ConfirmRef:     "01HC-CONF",
			ProposedAction: "Remind you tomorrow at 9am",
			PositiveLabel:  "Yes",
			NegativeLabel:  "No",
		},
	}
	out, err := Render(resp, renderTestMaxChars)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out.Kind != OutboundInteractiveButtons {
		t.Fatalf("kind: want %q, got %q", OutboundInteractiveButtons, out.Kind)
	}
	if len(out.Interactive.Buttons) != 2 {
		t.Fatalf("want 2 buttons, got %d", len(out.Interactive.Buttons))
	}
	pos := out.Interactive.Buttons[0]
	neg := out.Interactive.Buttons[1]
	if ref, positive, ok := DecodeConfirmPayload(pos.ID); !ok || ref != "01HC-CONF" || !positive {
		t.Errorf("positive button payload not round-tripping: id=%q ref=%q pos=%v ok=%v", pos.ID, ref, positive, ok)
	}
	if ref, positive, ok := DecodeConfirmPayload(neg.ID); !ok || ref != "01HC-CONF" || positive {
		t.Errorf("negative button payload not round-tripping: id=%q ref=%q pos=%v ok=%v", neg.ID, ref, positive, ok)
	}
}

// SCN-072-A04 — StatusUnavailable renders as single-line text, not
// dropped.
func TestRender_StatusUnavailableRendersText(t *testing.T) {
	resp := contracts.AssistantResponse{
		Status:     contracts.StatusUnavailable,
		ErrorCause: contracts.ErrProviderUnavailable,
		Body:       "Weather provider unavailable.",
	}
	out, err := Render(resp, renderTestMaxChars)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out.Kind != OutboundText {
		t.Fatalf("status_unavailable must render text, got %q", out.Kind)
	}
	if !strings.Contains(out.Text.Body, "Weather provider unavailable") {
		t.Errorf("error body missing: %q", out.Text.Body)
	}
}

// Truncation — body longer than maxTextChars is shortened, not
// dropped.
func TestRender_TruncatesOversizeBody(t *testing.T) {
	long := strings.Repeat("a", 50)
	resp := contracts.AssistantResponse{Body: long}
	out, err := Render(resp, 10)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := []rune(out.Text.Body); len(got) != 10 {
		t.Fatalf("expected truncation to 10 runes, got %d (%q)", len(got), out.Text.Body)
	}
}
