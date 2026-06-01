// Spec 066 SCOPE-1 — unit coverage that /status remains a
// deterministic operational handler and does NOT invoke the LLM or
// the assistant facade. SCN-066-A09.
package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestStatusCommandBypassesLLMAndFacade(t *testing.T) {
	// Stand up a fake health endpoint and assert that handleStatus
	// reaches it directly. The Bot is constructed without an
	// assistantAdapter so any LLM/facade detour would produce a
	// distinct observable (no health hit + a runtime panic on nil
	// adapter); the absence of either is the bypass proof.
	var healthHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&healthHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","services":{"db":{"status":"ok"}}}`))
	}))
	defer server.Close()

	var replies []string
	bot := &Bot{
		healthURL:  server.URL,
		httpClient: server.Client(),
		// assistantAdapter intentionally nil — proves /status path
		// does NOT consult the assistant facade.
		replyFunc: func(chatID int64, text string) {
			replies = append(replies, text)
		},
	}

	msg := &tgbotapi.Message{
		MessageID: 1,
		Chat:      &tgbotapi.Chat{ID: 12345},
		Text:      "/status",
	}
	bot.handleStatus(context.Background(), msg)

	if got := atomic.LoadInt32(&healthHits); got != 1 {
		t.Fatalf("handleStatus must call the health URL exactly once; got %d hits", got)
	}
	if len(replies) != 1 {
		t.Fatalf("handleStatus must reply exactly once; got %d replies: %v", len(replies), replies)
	}
	// Adversarial guard: classifier must agree that /status is
	// operational. A regression that reclassified it as a retained
	// shortcut would let it route through the facade in SCOPE-2.
	if got := ClassifyCommand("status"); got != LegacyCommandOperational {
		t.Fatalf("ClassifyCommand(\"status\") = %d, want LegacyCommandOperational", got)
	}
}
