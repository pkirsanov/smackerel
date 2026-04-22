package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestParseListCommand(t *testing.T) {
	tests := []struct {
		input      string
		wantType   string
		wantFilter string
	}{
		{"shopping from #weeknight", "shopping", "#weeknight"},
		{"reading from golang", "reading", "golang"},
		{"comparison from laptops", "comparison", "laptops"},
		{"shopping #dinner", "shopping", "#dinner"},
		{"", "", ""},
		{"unknown from #tag", "", ""},
	}

	for _, tt := range tests {
		gotType, gotFilter := parseListCommand(tt.input)
		if gotType != tt.wantType {
			t.Errorf("parseListCommand(%q) type = %q, want %q", tt.input, gotType, tt.wantType)
		}
		if gotFilter != tt.wantFilter {
			t.Errorf("parseListCommand(%q) filter = %q, want %q", tt.input, gotFilter, tt.wantFilter)
		}
	}
}

func TestFormatListSummary(t *testing.T) {
	lists := []listSummary{
		{ID: "l1", Title: "Groceries", ListType: "shopping", TotalItems: 10, CheckedItems: 3},
		{ID: "l2", Title: "Reading", ListType: "reading", TotalItems: 5, CheckedItems: 0},
	}

	result := formatListSummary(lists)
	if result == "" {
		t.Fatal("expected non-empty summary")
	}
	if !containsStr(result, "Groceries") {
		t.Error("expected list title in summary")
	}
	if !containsStr(result, "3/10") {
		t.Error("expected progress in summary")
	}
}

func TestFormatListMessage(t *testing.T) {
	lwi := listWithItemsResponse{
		List: listSummary{
			ID:           "lst-1",
			Title:        "Shopping",
			TotalItems:   3,
			CheckedItems: 1,
		},
		Items: []listItemResponse{
			{ID: "i1", ListID: "lst-1", Content: "Garlic", Status: "done"},
			{ID: "i2", ListID: "lst-1", Content: "Flour", Status: "pending"},
			{ID: "i3", ListID: "lst-1", Content: "Salt", Status: "pending"},
		},
	}

	text, keyboard := formatListMessage(lwi)

	if !containsStr(text, "Shopping") {
		t.Error("expected title in message")
	}
	if !containsStr(text, "✅") {
		t.Error("expected checkmark for done item")
	}
	if !containsStr(text, "⬜") {
		t.Error("expected empty box for pending item")
	}
	// Should have 2 keyboard buttons (for 2 pending items)
	if len(keyboard) != 2 {
		t.Errorf("expected 2 keyboard rows for pending items, got %d", len(keyboard))
	}
}

func TestFormatListMessage_AllDone(t *testing.T) {
	lwi := listWithItemsResponse{
		List: listSummary{
			ID:           "lst-1",
			Title:        "Done List",
			TotalItems:   2,
			CheckedItems: 2,
		},
		Items: []listItemResponse{
			{ID: "i1", ListID: "lst-1", Content: "Item A", Status: "done"},
			{ID: "i2", ListID: "lst-1", Content: "Item B", Status: "done"},
		},
	}

	_, keyboard := formatListMessage(lwi)
	if len(keyboard) != 0 {
		t.Errorf("expected 0 keyboard rows when all done, got %d", len(keyboard))
	}
}

func TestHandleList_ShowActiveLists(t *testing.T) {
	// Mock API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"lists": []listSummary{
				{ID: "l1", Title: "Groceries", ListType: "shopping", TotalItems: 5, CheckedItems: 2},
			},
			"count": 1,
		})
	}))
	defer server.Close()

	var replied string
	bot := &Bot{
		listsURL:   server.URL,
		httpClient: server.Client(),
		replyFunc:  func(_ int64, text string) { replied = text },
	}

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	bot.handleList(context.Background(), msg, "")

	if !containsStr(replied, "Groceries") {
		t.Errorf("expected Groceries in reply, got: %s", replied)
	}
}

func TestHandleList_ShowEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"lists": []any{}, "count": 0})
	}))
	defer server.Close()

	var replied string
	bot := &Bot{
		listsURL:   server.URL,
		httpClient: server.Client(),
		replyFunc:  func(_ int64, text string) { replied = text },
	}

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	bot.handleList(context.Background(), msg, "")

	if !containsStr(replied, "No active lists") {
		t.Errorf("expected 'No active lists', got: %s", replied)
	}
}

func TestHandleList_AddItem(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// First call: GET active lists
			json.NewEncoder(w).Encode(map[string]any{
				"lists": []listSummary{
					{ID: "l1", Title: "Groceries"},
				},
			})
		} else {
			// Second call: POST item
			json.NewEncoder(w).Encode(map[string]string{"id": "new-item"})
		}
	}))
	defer server.Close()

	var replied string
	bot := &Bot{
		listsURL:   server.URL,
		httpClient: server.Client(),
		replyFunc:  func(_ int64, text string) { replied = text },
	}

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	bot.handleList(context.Background(), msg, "add paper towels")

	if !containsStr(replied, "paper towels") {
		t.Errorf("expected confirmation with item name, got: %s", replied)
	}
}

func TestHandleList_Done(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			json.NewEncoder(w).Encode(map[string]any{
				"lists": []listSummary{
					{ID: "l1", Title: "Groceries", TotalItems: 5, CheckedItems: 4},
				},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]string{"status": "completed"})
		}
	}))
	defer server.Close()

	var replied string
	bot := &Bot{
		listsURL:   server.URL,
		httpClient: server.Client(),
		replyFunc:  func(_ int64, text string) { replied = text },
	}

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	bot.handleList(context.Background(), msg, "done")

	if !containsStr(replied, "Completed") {
		t.Errorf("expected completion confirmation, got: %s", replied)
	}
}

func TestHandleListCallback(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// POST check item
			json.NewEncoder(w).Encode(map[string]string{"status": "done"})
		} else {
			// GET list for refresh
			json.NewEncoder(w).Encode(listWithItemsResponse{
				List: listSummary{
					ID:           "lst-1",
					Title:        "Groceries",
					TotalItems:   2,
					CheckedItems: 1,
				},
				Items: []listItemResponse{
					{ID: "i1", ListID: "lst-1", Content: "Garlic", Status: "done"},
					{ID: "i2", ListID: "lst-1", Content: "Flour", Status: "pending"},
				},
			})
		}
	}))
	defer server.Close()

	var answeredText string
	bot := &Bot{
		listsURL:     server.URL,
		httpClient:   server.Client(),
		callbackFunc: func(_, text string) { answeredText = text },
	}

	cb := &tgbotapi.CallbackQuery{
		ID:   "cb-1",
		Data: "list_check:lst-1:i1",
		Message: &tgbotapi.Message{
			Chat:      &tgbotapi.Chat{ID: 123},
			MessageID: 456,
		},
	}

	bot.handleListCallback(context.Background(), cb)

	if answeredText != "✓ Done" {
		t.Errorf("expected callback answer '✓ Done', got: %s", answeredText)
	}
}

func TestHandleListCallback_InvalidData(t *testing.T) {
	bot := &Bot{}

	// Should not panic on empty data
	cb := &tgbotapi.CallbackQuery{Data: ""}
	bot.handleListCallback(context.Background(), cb)

	// Should not panic on invalid format
	cb2 := &tgbotapi.CallbackQuery{Data: "invalid"}
	bot.handleListCallback(context.Background(), cb2)
}

func TestHandleList_GenerateShoppingList(t *testing.T) {
	// Gherkin: "Generate shopping list via Telegram"
	// Given the user sends "/list shopping from #weeknight"
	// When the bot processes the command
	// Then a shopping list is generated from #weeknight-tagged recipe artifacts
	// And the list is sent as a formatted message with inline keyboard buttons
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)

		if body["list_type"] != "shopping" {
			t.Errorf("expected list_type 'shopping', got %q", body["list_type"])
		}
		if body["tag_filter"] != "#weeknight" {
			t.Errorf("expected tag_filter '#weeknight', got %q", body["tag_filter"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listWithItemsResponse{
			List: listSummary{
				ID:           "lst-gen-1",
				Title:        "Shopping list from #weeknight",
				ListType:     "shopping",
				TotalItems:   3,
				CheckedItems: 0,
			},
			Items: []listItemResponse{
				{ID: "i1", ListID: "lst-gen-1", Content: "5 cloves garlic", Category: "produce", Status: "pending"},
				{ID: "i2", ListID: "lst-gen-1", Content: "2 lbs chicken", Category: "proteins", Status: "pending"},
				{ID: "i3", ListID: "lst-gen-1", Content: "1 cup rice", Category: "pantry", Status: "pending"},
			},
		})
	}))
	defer server.Close()

	var replied string
	bot := &Bot{
		listsURL:   server.URL,
		httpClient: server.Client(),
		replyFunc:  func(_ int64, text string) { replied = text },
	}

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	bot.handleList(context.Background(), msg, "shopping from #weeknight")

	if replied == "" {
		t.Fatal("expected a reply")
	}
	if !containsStr(replied, "Shopping list from #weeknight") {
		t.Errorf("expected list title in reply, got: %s", replied)
	}
	if !containsStr(replied, "garlic") {
		t.Errorf("expected 'garlic' in reply, got: %s", replied)
	}
	if !containsStr(replied, "chicken") {
		t.Errorf("expected 'chicken' in reply, got: %s", replied)
	}
}

func TestHandleList_GenerateInvalidType(t *testing.T) {
	var replied string
	bot := &Bot{
		replyFunc: func(_ int64, text string) { replied = text },
	}

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	bot.handleList(context.Background(), msg, "unknown from #tag")

	if !containsStr(replied, "Usage") {
		t.Errorf("expected usage message for invalid type, got: %s", replied)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
