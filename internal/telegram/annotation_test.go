package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/annotation"
)

func TestFormatAnnotationConfirmation_RatingOnly(t *testing.T) {
	rating := 4
	parsed := annotation.ParsedAnnotation{Rating: &rating}
	got := formatAnnotationConfirmation(nil, parsed)
	if got != "Rated ★★★★☆" {
		t.Errorf("got %q, want %q", got, "Rated ★★★★☆")
	}
}

func TestFormatAnnotationConfirmation_Full(t *testing.T) {
	rating := 4
	parsed := annotation.ParsedAnnotation{
		Rating:          &rating,
		InteractionType: annotation.InteractionMadeIt,
		Tags:            []string{"weeknight"},
		Note:            "great",
	}
	got := formatAnnotationConfirmation(nil, parsed)
	expected := "Rated ★★★★☆\nLogged: Made it\nTagged: #weeknight\nNote: great"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestFormatAnnotationConfirmation_TagsOnly(t *testing.T) {
	parsed := annotation.ParsedAnnotation{
		Tags: []string{"weeknight", "quick"},
	}
	got := formatAnnotationConfirmation(nil, parsed)
	expected := "Tagged: #weeknight\nTagged: #quick"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestFormatAnnotationConfirmation_NoteOnly(t *testing.T) {
	parsed := annotation.ParsedAnnotation{
		Note: "needs more garlic",
	}
	got := formatAnnotationConfirmation(nil, parsed)
	if got != "Note: needs more garlic" {
		t.Errorf("got %q", got)
	}
}

func TestFormatAnnotationConfirmation_Empty(t *testing.T) {
	parsed := annotation.ParsedAnnotation{}
	got := formatAnnotationConfirmation(nil, parsed)
	if got != "Annotation recorded" {
		t.Errorf("got %q, want %q", got, "Annotation recorded")
	}
}

func TestRenderStars(t *testing.T) {
	tests := []struct {
		rating int
		want   string
	}{
		{1, "★☆☆☆☆"},
		{2, "★★☆☆☆"},
		{3, "★★★☆☆"},
		{4, "★★★★☆"},
		{5, "★★★★★"},
	}
	for _, tt := range tests {
		got := renderStars(tt.rating)
		if got != tt.want {
			t.Errorf("renderStars(%d) = %q, want %q", tt.rating, got, tt.want)
		}
	}
}

func TestHumanizeInteraction(t *testing.T) {
	tests := []struct {
		input annotation.InteractionType
		want  string
	}{
		{annotation.InteractionMadeIt, "Made it"},
		{annotation.InteractionBoughtIt, "Bought it"},
		{annotation.InteractionReadIt, "Read it"},
		{annotation.InteractionVisited, "Visited"},
		{annotation.InteractionTriedIt, "Tried it"},
		{annotation.InteractionUsedIt, "Used it"},
	}
	for _, tt := range tests {
		got := humanizeInteraction(tt.input)
		if got != tt.want {
			t.Errorf("humanizeInteraction(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSplitRateArgs(t *testing.T) {
	tests := []struct {
		args       string
		wantSearch string
		wantAnnot  string
	}{
		{"pasta carbonara 4/5 great dish", "pasta carbonara", "4/5 great dish"},
		{"chicken recipe #weeknight", "chicken recipe", "#weeknight"},
		{"that cake 5/5 made it", "that cake", "5/5 made it"},
		{"just search terms", "just search terms", ""},
		{"pasta made it", "pasta", "made it"},
	}
	for _, tt := range tests {
		search, annot := splitRateArgs(tt.args)
		if search != tt.wantSearch || annot != tt.wantAnnot {
			t.Errorf("splitRateArgs(%q) = (%q, %q), want (%q, %q)",
				tt.args, search, annot, tt.wantSearch, tt.wantAnnot)
		}
	}
}

func TestHandleReplyAnnotation_UnknownMessage(t *testing.T) {
	// Mock server: resolve returns 404 (unknown message)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/telegram-message-artifact" && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	var mu sync.Mutex
	var replies []string
	bot := &Bot{
		baseURL:         ts.URL,
		captureURL:      ts.URL + "/api/capture",
		httpClient:      ts.Client(),
		done:            make(chan struct{}),
		disambiguations: newDisambiguationStore(120),
		replyFunc: func(chatID int64, text string) {
			mu.Lock()
			replies = append(replies, text)
			mu.Unlock()
		},
	}

	// Simulate a reply to an unknown message
	msg := &tgbotapi.Message{
		Text: "4/5",
		Chat: &tgbotapi.Chat{ID: 5555},
		ReplyToMessage: &tgbotapi.Message{
			MessageID: 9999,
		},
	}

	handled := bot.handleReplyAnnotation(context.Background(), msg)
	if handled {
		t.Fatal("expected false for unknown message")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(replies) != 0 {
		t.Errorf("expected no replies, got %v", replies)
	}
}

func TestHandleReplyAnnotation_KnownMessage(t *testing.T) {
	// Mock server: resolve returns artifact ID, annotation endpoint accepts
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/telegram-message-artifact" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"artifact_id": "art-abc"})
			return
		}
		if r.URL.Path == "/api/artifacts/art-abc/annotations" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"created": []map[string]interface{}{
					{"annotation_type": "rating", "rating": 4},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	var mu sync.Mutex
	var replies []string
	bot := &Bot{
		baseURL:         ts.URL,
		captureURL:      ts.URL + "/api/capture",
		httpClient:      ts.Client(),
		done:            make(chan struct{}),
		disambiguations: newDisambiguationStore(120),
		replyFunc: func(chatID int64, text string) {
			mu.Lock()
			replies = append(replies, text)
			mu.Unlock()
		},
	}

	msg := &tgbotapi.Message{
		Text: "4/5",
		Chat: &tgbotapi.Chat{ID: 5555},
		ReplyToMessage: &tgbotapi.Message{
			MessageID: 1001,
		},
	}

	handled := bot.handleReplyAnnotation(context.Background(), msg)
	if !handled {
		t.Fatal("expected true for known message")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(replies))
	}
	if replies[0] != "Rated ★★★★☆" {
		t.Errorf("reply = %q, want %q", replies[0], "Rated ★★★★☆")
	}
}

func TestHandleRate_NoArgs(t *testing.T) {
	var mu sync.Mutex
	var replies []string
	bot := &Bot{
		baseURL:         "http://localhost",
		captureURL:      "http://localhost/api/capture",
		httpClient:      http.DefaultClient,
		done:            make(chan struct{}),
		disambiguations: newDisambiguationStore(120),
		replyFunc: func(chatID int64, text string) {
			mu.Lock()
			replies = append(replies, text)
			mu.Unlock()
		},
	}

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 5555},
	}

	bot.handleRate(context.Background(), msg, "")

	mu.Lock()
	defer mu.Unlock()
	if len(replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(replies))
	}
	if !strings.Contains(replies[0], "Usage:") {
		t.Errorf("expected usage message, got %q", replies[0])
	}
}

func TestHandleRate_NoResults(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/search" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"results": []interface{}{},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	var mu sync.Mutex
	var replies []string
	bot := &Bot{
		baseURL:         ts.URL,
		captureURL:      ts.URL + "/api/capture",
		searchURL:       ts.URL + "/api/search",
		httpClient:      ts.Client(),
		done:            make(chan struct{}),
		disambiguations: newDisambiguationStore(120),
		replyFunc: func(chatID int64, text string) {
			mu.Lock()
			replies = append(replies, text)
			mu.Unlock()
		},
	}

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 5555},
	}

	bot.handleRate(context.Background(), msg, "unicorn stew 5/5")

	mu.Lock()
	defer mu.Unlock()
	if len(replies) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(replies))
	}
	if replies[0] != "No matching artifacts found" {
		t.Errorf("reply = %q, want %q", replies[0], "No matching artifacts found")
	}
}

func TestDisambiguationStore_SetGetClear(t *testing.T) {
	ds := newDisambiguationStore(120)

	ds.set(5555, &pendingDisambiguation{
		Artifacts: []disambiguationOption{
			{ArtifactID: "art-1", Title: "Pasta"},
			{ArtifactID: "art-2", Title: "Pizza"},
		},
		Annotation: "4/5",
	})

	got := ds.get(5555)
	if got == nil {
		t.Fatal("expected pending disambiguation")
	}
	if len(got.Artifacts) != 2 {
		t.Errorf("got %d artifacts, want 2", len(got.Artifacts))
	}
	if got.Annotation != "4/5" {
		t.Errorf("annotation = %q, want 4/5", got.Annotation)
	}

	ds.clear(5555)
	if ds.get(5555) != nil {
		t.Error("expected nil after clear")
	}
}

func TestDisambiguationStore_Expiry(t *testing.T) {
	ds := newDisambiguationStore(0) // 0 second timeout = immediate expiry

	ds.set(5555, &pendingDisambiguation{
		Artifacts:  []disambiguationOption{{ArtifactID: "art-1", Title: "Test"}},
		Annotation: "5/5",
	})

	got := ds.get(5555)
	if got != nil {
		t.Error("expected nil due to expired timeout")
	}
}
