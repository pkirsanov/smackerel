package assistant_adapter

import (
	"context"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// BUG-064-001 DEFECT B — a captured-as-fallback idea must NOT contain the
// leading slash-command token (/ask, /weather, /remind, /recipe, /cook).
//
// Live evidence (self-hosted, 2026-06-11): a "/ask tide schedule …" turn was
// captured as `. Saved: "/ask tide schedule …" (idea)` — the /ask prefix
// leaked into the idea title because HandleUpdate dispatched the verbatim
// msg.Text (which translate_inbound preserves so the facade's LookupShortcut
// can pin the scenario) straight into the CaptureFn.
//
// These tests are adversarial: the prefix assertions FAIL against the
// pre-fix dispatch (which passed the /ask-prefixed text) and PASS once the
// dispatch strips the v1 shortcut prefix via assistant.StripShortcutPrefix.

// TestHandleUpdate_BUG064001_CaptureStripsAskPrefix reproduces the exact
// reported failure and asserts the captured text is the natural-language
// tail with no /ask prefix.
func TestHandleUpdate_BUG064001_CaptureStripsAskPrefix(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{}
	capture := &recordingCapture{}
	stub := &stubAssistant{
		response: contracts.AssistantResponse{
			Status:       contracts.StatusSavedAsIdea,
			CaptureRoute: true,
			// No body — silent capture, matching the open-knowledge
			// no-ground / refusal capture path.
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
	_ = a.Start(context.Background(), stub)

	const tail = "tide schedule for 06/11 in wa-town-A, wa"
	handled, err := a.HandleUpdate(context.Background(), updateWithText(99007, 1, "/ask "+tail))
	if err != nil {
		t.Fatalf("HandleUpdate err = %v; want nil", err)
	}
	if !handled {
		t.Fatal("handled = false; want true")
	}
	if capture.count() != 1 {
		t.Fatalf("capture.count() = %d; want 1", capture.count())
	}
	capture.mu.Lock()
	got := capture.calls[0].text
	capture.mu.Unlock()

	if got == "/ask "+tail {
		t.Fatalf("DEFECT B regression: captured text still carries the /ask prefix: %q", got)
	}
	if got != tail {
		t.Fatalf("captured text = %q; want %q (slash-command prefix stripped)", got, tail)
	}
}

// TestHandleUpdate_BUG064001_AllV1ShortcutsStripped covers every v1 slash
// shortcut so a future shortcut addition cannot silently leak its prefix
// into a captured idea.
func TestHandleUpdate_BUG064001_AllV1ShortcutsStripped(t *testing.T) {
	t.Parallel()
	cases := []struct {
		shortcut string
		tail     string
	}{
		{"/ask", "tide schedule for wa-town-A"},
		{"/weather", "in wa-town-A wa tomorrow"},
		{"/remind", "me to call the marina at 9am"},
		{"/recipe", "for clam chowder"},
		{"/cook", "the clam chowder tonight"},
	}
	for i, tc := range cases {
		tc := tc
		t.Run(tc.shortcut, func(t *testing.T) {
			t.Parallel()
			sender := &recordingSender{}
			capture := &recordingCapture{}
			stub := &stubAssistant{
				response: contracts.AssistantResponse{
					Status:       contracts.StatusSavedAsIdea,
					CaptureRoute: true,
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
			_ = a.Start(context.Background(), stub)

			chatID := int64(99100 + i)
			handled, err := a.HandleUpdate(context.Background(), updateWithText(chatID, i+1, tc.shortcut+" "+tc.tail))
			if err != nil {
				t.Fatalf("HandleUpdate err = %v", err)
			}
			if !handled {
				t.Fatalf("handled = false for %s", tc.shortcut)
			}
			if capture.count() != 1 {
				t.Fatalf("capture.count() = %d; want 1", capture.count())
			}
			capture.mu.Lock()
			got := capture.calls[0].text
			capture.mu.Unlock()
			if got != tc.tail {
				t.Fatalf("%s: captured text = %q; want %q (prefix stripped)", tc.shortcut, got, tc.tail)
			}
		})
	}
}

// TestHandleUpdate_BUG064001_NonShortcutCapturedVerbatim is the FR-2a guard:
// plain text that is NOT a v1 shortcut must be captured verbatim (the strip
// must never mangle ordinary ideas).
func TestHandleUpdate_BUG064001_NonShortcutCapturedVerbatim(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{}
	capture := &recordingCapture{}
	stub := &stubAssistant{
		response: contracts.AssistantResponse{
			Status:       contracts.StatusSavedAsIdea,
			CaptureRoute: true,
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
	_ = a.Start(context.Background(), stub)

	const text = "remember to winterize the crab pots before november"
	if _, err := a.HandleUpdate(context.Background(), updateWithText(99200, 1, text)); err != nil {
		t.Fatalf("HandleUpdate err = %v", err)
	}
	if capture.count() != 1 {
		t.Fatalf("capture.count() = %d; want 1", capture.count())
	}
	capture.mu.Lock()
	got := capture.calls[0].text
	capture.mu.Unlock()
	if got != text {
		t.Fatalf("non-shortcut capture text = %q; want verbatim %q", got, text)
	}
}
