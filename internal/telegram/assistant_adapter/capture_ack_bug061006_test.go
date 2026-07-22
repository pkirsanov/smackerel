// BUG-061-006 — adversarial regression tests for the Telegram assistant
// capture-as-fallback acknowledgement contract.
//
// The user observed, on the live self-hosted bot, that a single turn that
// fell to capture-as-fallback produced TWO Telegram messages (the legacy
// ". Saved …"/". Already saved"/"? Failed to save" reply AND the assistant
// "saved as an idea — i'll surface it later." reply) and, for a bare
// "/ask", a CONTRADICTORY pair ("? Failed to save" + "saved as an idea").
//
// After the fix the bot-side capture hook persists SILENTLY and reports
// whether an idea was actually saved; the adapter is the single source of
// the user-facing acknowledgement and keeps it HONEST:
//   - capture persisted        → render the facade "saved as an idea" body.
//   - nothing to save (bare)    → render an honest prompt, NOT "saved …".
//   - capture failed            → render an honest failure line, NOT "saved …".
//
// Each honest-ack case is adversarial: before the fix the renderer emitted
// the "saved as an idea" body regardless of whether the capture succeeded,
// so these assertions fail if the fix is reverted.

package assistant_adapter

import (
	"context"
	"errors"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

const savedAsIdeaBody = "saved as an idea — i'll surface it later."

// lastSentText returns the text of the most recent outbound message, or
// "" when none was sent. Fails the test if the message is not a plain
// tgbotapi.MessageConfig.
func lastSentText(t *testing.T, s *recordingSender) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.messages) == 0 {
		return ""
	}
	mc, ok := s.messages[len(s.messages)-1].(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("last outbound message is %T; want tgbotapi.MessageConfig", s.messages[len(s.messages)-1])
	}
	return mc.Text
}

// TestHandleUpdate_BUG061006_CaptureSuccess_SingleAck asserts that when the
// bot-side capture hook persists the idea, the adapter renders EXACTLY ONE
// message — the facade "saved as an idea" acknowledgement. The capture hook
// is silent (it does not send its own reply), so there is no duplicate.
func TestHandleUpdate_BUG061006_CaptureSuccess_SingleAck(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{}
	capture := &recordingCapture{} // err == nil → persisted
	stub := &stubAssistant{
		response: contracts.AssistantResponse{
			Status:       contracts.StatusSavedAsIdea,
			CaptureRoute: true,
			Body:         savedAsIdeaBody,
		},
	}
	a, _ := NewAdapter(Options{
		Sender:          sender,
		Capture:         capture.fn(),
		ResolveUser:     fixedResolver("u1"),
		MarkdownMode:    PlainText,
		MaxMessageChars: 4096,
	})
	_ = a.Start(context.Background(), stub)

	handled, err := a.HandleUpdate(context.Background(), updateWithText(123, 1, "a passing thought"))
	if err != nil {
		t.Fatalf("HandleUpdate err = %v; want nil", err)
	}
	if !handled {
		t.Fatal("handled = false; want true")
	}
	if capture.count() != 1 {
		t.Fatalf("capture.count() = %d; want 1 (idea persisted)", capture.count())
	}
	if sender.count() != 1 {
		t.Fatalf("sender.count() = %d; want exactly 1 (single acknowledgement)", sender.count())
	}
	if got := lastSentText(t, sender); got != savedAsIdeaBody {
		t.Errorf("ack text = %q; want %q", got, savedAsIdeaBody)
	}
}

// TestHandleUpdate_BUG061006_NothingToCapture_HonestAck is the ADVERSARIAL
// bare-"/ask" case: the stripped text is empty, so the capture hook returns
// ErrNothingToCapture. The adapter MUST render an honest prompt and MUST NOT
// claim "saved as an idea" (the contradictory pair the user saw).
func TestHandleUpdate_BUG061006_NothingToCapture_HonestAck(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{}
	capture := &recordingCapture{err: ErrNothingToCapture}
	stub := &stubAssistant{
		response: contracts.AssistantResponse{
			Status:       contracts.StatusSavedAsIdea,
			CaptureRoute: true,
			Body:         savedAsIdeaBody, // what a reverted fix would (wrongly) render
		},
	}
	a, _ := NewAdapter(Options{
		Sender:          sender,
		Capture:         capture.fn(),
		ResolveUser:     fixedResolver("u1"),
		MarkdownMode:    PlainText,
		MaxMessageChars: 4096,
	})
	_ = a.Start(context.Background(), stub)

	handled, err := a.HandleUpdate(context.Background(), updateWithText(123, 1, "/ask"))
	if err != nil {
		t.Fatalf("HandleUpdate err = %v; want nil", err)
	}
	if !handled {
		t.Fatal("handled = false; want true")
	}
	if sender.count() != 1 {
		t.Fatalf("sender.count() = %d; want exactly 1 (single honest acknowledgement)", sender.count())
	}
	got := lastSentText(t, sender)
	if strings.Contains(strings.ToLower(got), "saved as an idea") {
		t.Fatalf("ADVERSARIAL: bare /ask ack = %q; MUST NOT claim it was saved as an idea", got)
	}
	if !strings.Contains(strings.ToLower(got), "nothing to save") {
		t.Errorf("ack = %q; want an honest nothing-to-save prompt", got)
	}
}

// TestHandleUpdate_BUG061006_CaptureFailure_HonestAck is the ADVERSARIAL
// real-failure case: the capture hook returns a genuine error. The adapter
// MUST render an honest failure line and MUST NOT claim "saved as an idea".
func TestHandleUpdate_BUG061006_CaptureFailure_HonestAck(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{}
	capture := &recordingCapture{err: errors.New("capture API 500")}
	stub := &stubAssistant{
		response: contracts.AssistantResponse{
			Status:       contracts.StatusSavedAsIdea,
			CaptureRoute: true,
			Body:         savedAsIdeaBody,
		},
	}
	a, _ := NewAdapter(Options{
		Sender:          sender,
		Capture:         capture.fn(),
		ResolveUser:     fixedResolver("u1"),
		MarkdownMode:    PlainText,
		MaxMessageChars: 4096,
	})
	_ = a.Start(context.Background(), stub)

	handled, err := a.HandleUpdate(context.Background(), updateWithText(123, 1, "a thought that fails to persist"))
	if err != nil {
		t.Fatalf("HandleUpdate err = %v; want nil", err)
	}
	if !handled {
		t.Fatal("handled = false; want true")
	}
	if sender.count() != 1 {
		t.Fatalf("sender.count() = %d; want exactly 1 (single honest acknowledgement)", sender.count())
	}
	got := lastSentText(t, sender)
	if strings.Contains(strings.ToLower(got), "saved as an idea") {
		t.Fatalf("ADVERSARIAL: failed-capture ack = %q; MUST NOT claim it was saved as an idea", got)
	}
	if !strings.Contains(strings.ToLower(got), "couldn't save") {
		t.Errorf("ack = %q; want an honest capture-failure line", got)
	}
}
