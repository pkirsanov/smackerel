// BUG-061-003 S04 — Telegram-adapter integration regression.
//
// Reproduces the user-observed bug path through the assistant adapter:
// the facade routes "find best recipe" to the recipe_search skill and
// returns a Body without CaptureRoute. The adapter then sends a
// regular outbound message and MUST NOT match the byte-for-byte
// `^\. Saved: ".*" \(idea\)$` regex the BandLow capture branch
// produced before the fix.
package assistant_adapter

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// savedAsIdeaRegex pins the exact reply shape produced by the
// pre-fix BandLow → handleTextCapture path. Any future regression
// that returns the user to that branch on a recipe query will
// match this regex and fail the test.
var savedAsIdeaRegex = regexp.MustCompile(`^\. Saved: ".*" \(idea\)$`)

func TestHandleUpdate_RecipeSearch_NotSavedAsIdea_BUG061003_S04(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{}
	capture := &recordingCapture{}
	// Facade returns the recipe_search happy path: a body sourced
	// from owned recipe artifacts, CaptureRoute=false.
	stub := &stubAssistant{
		response: contracts.AssistantResponse{
			Status:       contracts.StatusThinking,
			Body:         "You have Pasta Carbonara saved.",
			CaptureRoute: false,
		},
	}
	a, err := NewAdapter(Options{
		Sender:          sender,
		Capture:         capture.fn(),
		ResolveUser:     fixedResolver("u1"),
		MarkdownMode:    PlainText,
		MaxMessageChars: 4096,
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	if err := a.Start(context.Background(), stub); err != nil {
		t.Fatalf("Start: %v", err)
	}

	handled, err := a.HandleUpdate(context.Background(), updateWithText(123, 1, "find best recipe"))
	if err != nil {
		t.Fatalf("HandleUpdate err = %v", err)
	}
	if !handled {
		t.Fatal("handled = false; want true")
	}
	if capture.count() != 0 {
		t.Errorf("capture.count() = %d; MUST be 0 (recipe path must NOT fall through to capture)", capture.count())
	}
	if sender.count() != 1 {
		t.Fatalf("sender.count() = %d; want 1", sender.count())
	}

	body := extractText(sender.messages[0])
	if savedAsIdeaRegex.MatchString(body) {
		t.Fatalf("bot reply matched the pre-fix \"Saved as idea\" regex: %q", body)
	}
}

// Adversarial S04 sibling: if the facade DID route to the capture
// branch (the pre-fix behavior), the adapter would forward to the
// capture hook AND render the canonical Saved-as-idea reply. This
// test confirms the savedAsIdeaRegex would actually catch a
// regression — without it, S04 would silently pass on any non-empty
// body that happened to differ from the canonical string.
func TestSavedAsIdeaRegex_AdversarialMatchesPreFixReply_BUG061003(t *testing.T) {
	t.Parallel()
	for _, title := range []string{"find best recipe", "find best recepie", "anything"} {
		got := fmt.Sprintf(`. Saved: %q (idea)`, title)
		if !savedAsIdeaRegex.MatchString(got) {
			t.Fatalf("regex MUST match the pre-fix reply %q", got)
		}
	}
	for _, ok := range []string{"You have Pasta Carbonara saved.", "no recipes saved yet — capture one with /capture or import via a connector.", ""} {
		if savedAsIdeaRegex.MatchString(ok) {
			t.Fatalf("regex MUST NOT match post-fix reply %q", ok)
		}
	}
}

// extractText pulls the Text out of a tgbotapi.MessageConfig (the
// only Chattable shape the adapter produces for plain-text replies).
func extractText(c tgbotapi.Chattable) string {
	if mc, ok := c.(tgbotapi.MessageConfig); ok {
		return mc.Text
	}
	return ""
}
