package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestRecordMessageArtifact_CallsInternalEndpoint(t *testing.T) {
	var called bool
	var gotBody map[string]interface{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/telegram-message-artifact" && r.Method == http.MethodPost {
			called = true
			json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusCreated)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	bot := &Bot{
		baseURL:    ts.URL,
		captureURL: ts.URL + "/api/capture",
		authToken:  "test-token",
		httpClient: ts.Client(),
		done:       make(chan struct{}),
	}

	bot.recordMessageArtifact(context.Background(), 1001, 5555, "art-abc")

	if !called {
		t.Fatal("expected internal mapping endpoint to be called")
	}
	if int(gotBody["message_id"].(float64)) != 1001 {
		t.Errorf("message_id = %v, want 1001", gotBody["message_id"])
	}
	if int(gotBody["chat_id"].(float64)) != 5555 {
		t.Errorf("chat_id = %v, want 5555", gotBody["chat_id"])
	}
	if gotBody["artifact_id"].(string) != "art-abc" {
		t.Errorf("artifact_id = %v, want art-abc", gotBody["artifact_id"])
	}
}

func TestRecordMessageArtifact_EmptyArtifactIDSkips(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()

	bot := &Bot{
		baseURL:    ts.URL,
		captureURL: ts.URL + "/api/capture",
		httpClient: ts.Client(),
		done:       make(chan struct{}),
	}

	bot.recordMessageArtifact(context.Background(), 1001, 5555, "")

	if called {
		t.Fatal("should not call endpoint when artifact_id is empty")
	}
}

func TestResolveArtifactFromMessage_Found(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/telegram-message-artifact" && r.Method == http.MethodGet {
			msgID := r.URL.Query().Get("message_id")
			chatID := r.URL.Query().Get("chat_id")
			if msgID == "1001" && chatID == "5555" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"artifact_id": "art-abc"})
				return
			}
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	bot := &Bot{
		baseURL:    ts.URL,
		captureURL: ts.URL + "/api/capture",
		authToken:  "test-token",
		httpClient: ts.Client(),
		done:       make(chan struct{}),
	}

	got := bot.resolveArtifactFromMessage(context.Background(), 1001, 5555)
	if got != "art-abc" {
		t.Errorf("resolveArtifactFromMessage = %q, want %q", got, "art-abc")
	}
}

func TestResolveArtifactFromMessage_NotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	bot := &Bot{
		baseURL:    ts.URL,
		captureURL: ts.URL + "/api/capture",
		httpClient: ts.Client(),
		done:       make(chan struct{}),
	}

	got := bot.resolveArtifactFromMessage(context.Background(), 9999, 5555)
	if got != "" {
		t.Errorf("resolveArtifactFromMessage = %q, want empty string", got)
	}
}

func TestResolveArtifactFromMessage_MultipleMappings(t *testing.T) {
	mappings := map[string]string{
		"1001-5555": "art-abc",
		"1002-5555": "art-def",
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/telegram-message-artifact" && r.Method == http.MethodGet {
			key := r.URL.Query().Get("message_id") + "-" + r.URL.Query().Get("chat_id")
			if artID, ok := mappings[key]; ok {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"artifact_id": artID})
				return
			}
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	bot := &Bot{
		baseURL:    ts.URL,
		captureURL: ts.URL + "/api/capture",
		httpClient: ts.Client(),
		done:       make(chan struct{}),
	}

	got1 := bot.resolveArtifactFromMessage(context.Background(), 1001, 5555)
	if got1 != "art-abc" {
		t.Errorf("message 1001 → %q, want art-abc", got1)
	}

	got2 := bot.resolveArtifactFromMessage(context.Background(), 1002, 5555)
	if got2 != "art-def" {
		t.Errorf("message 1002 → %q, want art-def", got2)
	}
}

func TestReplyWithMapping_TestMode(t *testing.T) {
	var mu sync.Mutex
	var replies []string

	bot := &Bot{
		baseURL:    "http://localhost",
		captureURL: "http://localhost/api/capture",
		httpClient: http.DefaultClient,
		done:       make(chan struct{}),
		replyFunc: func(chatID int64, text string) {
			mu.Lock()
			replies = append(replies, text)
			mu.Unlock()
		},
	}

	bot.replyWithMapping(context.Background(), 5555, "test message", "art-123")

	mu.Lock()
	defer mu.Unlock()
	if len(replies) != 1 || replies[0] != "test message" {
		t.Errorf("replies = %v, want [test message]", replies)
	}
}
