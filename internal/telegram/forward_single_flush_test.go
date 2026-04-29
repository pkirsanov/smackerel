package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// captureRecorder is a test capture API endpoint that records every JSON body
// posted to it. It returns a synthetic 200 with a fixed artifact_id/title so
// the bot's reply pathway completes.
type captureRecorder struct {
	mu      sync.Mutex
	bodies  []map[string]interface{}
	server  *httptest.Server
	replyTo string
}

func newCaptureRecorder(t *testing.T) *captureRecorder {
	t.Helper()
	r := &captureRecorder{replyTo: "stub-artifact-id"}
	r.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var body map[string]interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		r.mu.Lock()
		r.bodies = append(r.bodies, body)
		r.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"artifact_id":   r.replyTo,
			"title":         "captured artifact",
			"artifact_type": "forwarded_message",
			"connections":   float64(0),
		})
	}))
	t.Cleanup(r.server.Close)
	return r
}

func (r *captureRecorder) snapshot() []map[string]interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]map[string]interface{}, len(r.bodies))
	copy(out, r.bodies)
	return out
}

// newWiredTestBot builds a Bot with a real ConversationAssembler wired to
// b.flushConversation, mimicking the production wiring at NewBot. The
// assembler's window is set very short so flushes happen quickly under test.
func newWiredTestBot(t *testing.T, recorder *captureRecorder, windowSecs int) *Bot {
	t.Helper()
	bot := &Bot{
		baseURL:    recorder.server.URL,
		captureURL: recorder.server.URL + "/api/capture",
		httpClient: recorder.server.Client(),
		done:       make(chan struct{}),
		replyFunc:  func(int64, string) {},
	}
	bot.assembler = NewConversationAssembler(
		context.Background(),
		windowSecs,
		100,
		bot.flushConversation,
		nil,
	)
	return bot
}

// TestBUG002_SC_TSC09_SingleForward_NotConversation is the adversarial
// regression for spec scenario SC-TSC09: a single forwarded message must
// flush to the capture API as a single forwarded-message artifact, NOT as
// a conversation payload. Pre-fix, flushConversation unconditionally emitted
// a "conversation" key for any buffer it received, including 1-message
// buffers — this test would have failed because the recorded body would
// contain a "conversation" key and lack the forward_meta key.
func TestBUG002_SC_TSC09_SingleForward_NotConversation(t *testing.T) {
	recorder := newCaptureRecorder(t)
	bot := newWiredTestBot(t, recorder, 1)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 100},
		ForwardFrom: &tgbotapi.User{
			ID:        42,
			FirstName: "Alice",
		},
		ForwardFromChat: &tgbotapi.Chat{
			ID:    -100777,
			Title: "Team Chat",
			Type:  "supergroup",
		},
		ForwardDate: int(time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC).Unix()),
		Text:        "single forwarded note without any URL",
	}

	bot.handleForwardedMessage(context.Background(), msg)

	// Wait for the inactivity timer to fire and the async flush to post.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(recorder.snapshot()) >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	bodies := recorder.snapshot()
	if len(bodies) != 1 {
		t.Fatalf("expected exactly 1 capture POST, got %d (bodies=%v)", len(bodies), bodies)
	}
	body := bodies[0]

	// Adversarial assertion 1: must NOT carry a conversation block.
	if _, hasConv := body["conversation"]; hasConv {
		t.Errorf("BUG-002 regression: single-message flush emitted a conversation payload (body=%v)", body)
	}

	// Adversarial assertion 2: must include forward_meta with the original sender.
	fwd, ok := body["forward_meta"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected forward_meta map in body, got %T (%v)", body["forward_meta"], body["forward_meta"])
	}
	if name, _ := fwd["sender_name"].(string); name != "Alice" {
		t.Errorf("expected forward_meta.sender_name=Alice, got %q", name)
	}
	if src, _ := fwd["source_chat"].(string); src != "Team Chat" {
		t.Errorf("expected forward_meta.source_chat=Team Chat, got %q", src)
	}

	// Adversarial assertion 3: text-only single forward uses text payload, not url.
	if _, hasText := body["text"]; !hasText {
		t.Errorf("expected text-only forward to post a 'text' field, got body=%v", body)
	}
	if _, hasURL := body["url"]; hasURL {
		t.Errorf("text-only forward must not post a 'url' field, got body=%v", body)
	}
}

// TestBUG002_SingleForward_WithURL_PreservesURLArtifact asserts that the
// URL-detection branch in single-forward capture is reachable through the
// wired assembler path. Pre-fix, this branch was dead code: every forward
// went through the conversation flush, which emits "text" + "conversation"
// regardless of whether the message contained a URL — losing URL semantics.
func TestBUG002_SingleForward_WithURL_PreservesURLArtifact(t *testing.T) {
	recorder := newCaptureRecorder(t)
	bot := newWiredTestBot(t, recorder, 1)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 200},
		ForwardFrom: &tgbotapi.User{
			ID:        7,
			FirstName: "Bob",
		},
		ForwardDate: int(time.Date(2026, 4, 2, 12, 30, 0, 0, time.UTC).Unix()),
		Text:        "Read this https://example.com/article",
	}

	bot.handleForwardedMessage(context.Background(), msg)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(recorder.snapshot()) >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	bodies := recorder.snapshot()
	if len(bodies) != 1 {
		t.Fatalf("expected exactly 1 capture POST, got %d", len(bodies))
	}
	body := bodies[0]

	// Adversarial assertion 1: URL-bearing single forward must emit a url payload.
	url, ok := body["url"].(string)
	if !ok || url != "https://example.com/article" {
		t.Errorf("expected url=https://example.com/article, got %v", body["url"])
	}

	// Adversarial assertion 2: must NOT emit a conversation block.
	if _, hasConv := body["conversation"]; hasConv {
		t.Errorf("BUG-002 regression: URL forward emitted conversation payload (body=%v)", body)
	}

	// Adversarial assertion 3: forward_meta must be populated with the sender.
	fwd, ok := body["forward_meta"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected forward_meta map in body, got %T", body["forward_meta"])
	}
	if name, _ := fwd["sender_name"].(string); name != "Bob" {
		t.Errorf("expected forward_meta.sender_name=Bob, got %q", name)
	}
}

// TestBUG002_TwoForwards_StillProduceConversation guards against an
// over-correction of the fix: when 2+ forwarded messages cluster into one
// buffer, the flush MUST still emit a conversation payload. This ensures
// the SC-TSC08 multi-message path is not regressed.
func TestBUG002_TwoForwards_StillProduceConversation(t *testing.T) {
	recorder := newCaptureRecorder(t)
	bot := newWiredTestBot(t, recorder, 1)

	srcChat := &tgbotapi.Chat{ID: -100888, Title: "Group", Type: "supergroup"}
	for i := 0; i < 3; i++ {
		msg := &tgbotapi.Message{
			Chat:            &tgbotapi.Chat{ID: 300},
			ForwardFrom:     &tgbotapi.User{ID: int64(i + 1), FirstName: "User"},
			ForwardFromChat: srcChat,
			ForwardDate:     int(time.Date(2026, 4, 3, 10, i, 0, 0, time.UTC).Unix()),
			Text:            "msg",
		}
		bot.handleForwardedMessage(context.Background(), msg)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(recorder.snapshot()) >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	bodies := recorder.snapshot()
	if len(bodies) != 1 {
		t.Fatalf("expected 1 capture POST for clustered forwards, got %d", len(bodies))
	}
	body := bodies[0]

	conv, ok := body["conversation"].(map[string]interface{})
	if !ok {
		t.Fatalf("multi-message buffer must produce conversation payload, got body=%v", body)
	}
	if mc, _ := conv["message_count"].(float64); int(mc) != 3 {
		t.Errorf("expected conversation.message_count=3, got %v", conv["message_count"])
	}
}
