// BUG-061-006 — bot-level adversarial regression test for the single,
// silent assistant capture-as-fallback acknowledgement.
//
// The user observed that ONE assistant turn that fell to capture produced
// TWO Telegram messages: the legacy ". Saved …"/"? Failed to save" reply
// (from the bot-side capture hook) AND the assistant "saved as an idea …"
// reply (from the renderer). This test proves the bot-side capture hook now
// persists WITHOUT sending a reply of its own — the legacy reply sink stays
// empty — while the assistant renderer emits exactly one acknowledgement.
//
// Adversarial: before the fix NewBotCaptureFn delegated to the replying
// handleTextCapture path, so the legacy reply sink would NOT be empty (it
// recorded one ". Saved …" message). The empty-reply-sink assertion fails if
// the fix is reverted.

package telegram

import (
	"context"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/telegram/assistant_adapter"
)

// recordingBotSender records the text of every outbound renderer message.
type recordingBotSender struct {
	mu   sync.Mutex
	sent []string
}

func (r *recordingBotSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	text := ""
	if mc, ok := c.(tgbotapi.MessageConfig); ok {
		text = mc.Text
	}
	r.sent = append(r.sent, text)
	return tgbotapi.Message{MessageID: len(r.sent)}, nil
}

func (r *recordingBotSender) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.sent))
	copy(out, r.sent)
	return out
}

// TestHandleMessage_BUG061006_CaptureRoute_SingleSilentAck drives a
// plain-text turn whose facade response is CaptureRoute=true (the
// capture-as-fallback path) and asserts:
//   - the legacy reply sink (b.replyFunc) receives NOTHING (silent hook),
//   - the assistant renderer sends EXACTLY ONE message (the single ack),
//   - the idea was persisted to the capture API.
func TestHandleMessage_BUG061006_CaptureRoute_SingleSilentAck(t *testing.T) {
	const savedAck = "saved as an idea — i'll surface it later."

	capRec := newAsstCaptureRecorder(t)

	var replyMu sync.Mutex
	var replies []string

	bot := &Bot{
		captureURL: capRec.server.URL,
		httpClient: capRec.server.Client(),
		replyFunc: func(_ int64, text string) {
			replyMu.Lock()
			replies = append(replies, text)
			replyMu.Unlock()
		},
		userMapping: map[int64]string{99: "u-99"},
		environment: "test",
	}

	sender := &recordingBotSender{}
	asst := &asstStubAssistant{resp: contracts.AssistantResponse{
		Status:       contracts.StatusSavedAsIdea,
		CaptureRoute: true,
		Body:         savedAck,
	}}

	adapter, err := assistant_adapter.NewAdapter(assistant_adapter.Options{
		Sender:          sender,
		Capture:         NewBotCaptureFn(bot),
		ResolveUser:     NewBotChatResolver(bot),
		MarkdownMode:    assistant_adapter.PlainText,
		MaxMessageChars: 4096,
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	if err := adapter.Start(context.Background(), asst); err != nil {
		t.Fatalf("adapter Start: %v", err)
	}
	bot.SetAssistantAdapter(adapter)

	msg := &tgbotapi.Message{
		Chat:      &tgbotapi.Chat{ID: 99},
		Text:      "a stray thought worth keeping",
		MessageID: 1,
	}
	bot.handleMessage(context.Background(), msg, 7)

	// Facade was consulted exactly once.
	if len(asst.calls) != 1 {
		t.Fatalf("assistant.Handle calls = %d; want 1", len(asst.calls))
	}

	// The idea was persisted (BS-001 durability preserved).
	if got := capRec.snapshot(); len(got) != 1 || got[0] != "a stray thought worth keeping" {
		t.Fatalf("capture persisted = %v; want one entry with the verbatim text", got)
	}

	// ADVERSARIAL: the bot-side capture hook persists WITHOUT sending a reply
	// — the legacy reply sink MUST be empty. Pre-fix it recorded a
	// ". Saved …" reply, so a non-empty sink here means the fix regressed.
	replyMu.Lock()
	gotReplies := append([]string(nil), replies...)
	replyMu.Unlock()
	if len(gotReplies) != 0 {
		t.Fatalf("ADVERSARIAL: legacy reply sink is not empty: got %d message(s) %v; want 0 (silent capture hook — no duplicate ack)", len(gotReplies), gotReplies)
	}

	// The renderer sends EXACTLY ONE acknowledgement — the single ack.
	sent := sender.snapshot()
	if len(sent) != 1 {
		t.Fatalf("renderer sent %d message(s) %v; want exactly 1 (single acknowledgement)", len(sent), sent)
	}
	if sent[0] != savedAck {
		t.Errorf("ack text = %q; want %q", sent[0], savedAck)
	}
}
