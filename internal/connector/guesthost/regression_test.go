package guesthost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/smackerel/smackerel/internal/connector"
)

// --- H-013-001: UTF-8 safe truncation (CWE-838) ---

func TestTruncateStrUTF8SafeMultibyte(t *testing.T) {
	// "日本語テスト" is 6 runes × 3 bytes = 18 bytes.
	// Truncating at byte 10 previously sliced through the 4th character,
	// producing invalid UTF-8.
	input := "日本語テスト"
	result := truncateStr(input, 10)

	if !utf8.ValidString(result) {
		t.Errorf("truncateStr produced invalid UTF-8: %q", result)
	}
	if len(result) > 10 {
		t.Errorf("result exceeds maxLen 10: len=%d, %q", len(result), result)
	}
}

func TestTruncateStrUTF8Safe4ByteChar(t *testing.T) {
	// Emoji 🏠 is 4 bytes. Build a title that forces a cut mid-emoji.
	input := "Beach House 🏠 Resort"
	result := truncateStr(input, 15)

	if !utf8.ValidString(result) {
		t.Errorf("truncateStr produced invalid UTF-8 with emoji: %q", result)
	}
	if len(result) > 15 {
		t.Errorf("result exceeds maxLen: len=%d", len(result))
	}
}

func TestTruncateStrShortStringUnchanged(t *testing.T) {
	s := "hello"
	if truncateStr(s, 100) != s {
		t.Errorf("short string should pass through unchanged")
	}
}

func TestTruncateStrExactBoundary(t *testing.T) {
	s := "abcde" // 5 bytes
	if truncateStr(s, 5) != s {
		t.Errorf("exact-length string should pass through unchanged")
	}
}

func TestTruncateStrTinyLimit(t *testing.T) {
	// maxLen <= 3: should still produce valid UTF-8
	input := "日本語"
	result := truncateStr(input, 2)
	if !utf8.ValidString(result) {
		t.Errorf("tiny-limit truncation produced invalid UTF-8: %q", result)
	}
	if len(result) > 2 {
		t.Errorf("result exceeds maxLen 2: len=%d", len(result))
	}
}

func TestNormalizeEventUTF8TitleSafe(t *testing.T) {
	// Long property name with multi-byte chars that must be safely truncated.
	longName := strings.Repeat("日本語テスト", 100) // 1800 bytes
	event := ActivityEvent{
		ID:        "evt-utf8",
		Type:      "booking.created",
		Timestamp: "2026-04-10T14:30:00Z",
		EntityID:  "b1",
		Data: json.RawMessage(`{
			"propertyId": "p1",
			"propertyName": "` + longName + `",
			"guestName": "Guest",
			"guestEmail": "g@e.com",
			"checkinDate": "2026-05-01",
			"checkoutDate": "2026-05-02",
			"source": "direct",
			"totalAmount": 100
		}`),
	}

	a, err := NormalizeEvent(event)
	if err != nil {
		t.Fatalf("NormalizeEvent failed: %v", err)
	}
	if !utf8.ValidString(a.Title) {
		t.Errorf("NormalizeEvent produced artifact with invalid UTF-8 title: %q", a.Title)
	}
	if len(a.Title) > 500 {
		t.Errorf("Title exceeds 500 byte cap: len=%d", len(a.Title))
	}
}

// --- H-013-002: Non-string event_types config must not be silently ignored ---

func TestSyncEventTypesAsSlice(t *testing.T) {
	var capturedTypes string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
			return
		}
		capturedTypes = r.URL.Query().Get("types")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ActivityFeedResponse{Events: []ActivityEvent{}, HasMore: false})
	}))
	defer srv.Close()

	c := New()
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"base_url": srv.URL,
			"api_key":  "key",
			// YAML list syntax parses to []interface{}
			"event_types": []interface{}{"booking.created", "review.received"},
		},
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	_, _, err := c.Sync(context.Background(), "2026-04-01T00:00:00Z")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Before fix: capturedTypes would be "" (all types fetched).
	// After fix: should be "booking.created,review.received".
	if capturedTypes != "booking.created,review.received" {
		t.Errorf("event_types as []interface{} not properly joined: got types param %q, want %q",
			capturedTypes, "booking.created,review.received")
	}
}

func TestSyncEventTypesAsString(t *testing.T) {
	var capturedTypes string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
			return
		}
		capturedTypes = r.URL.Query().Get("types")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ActivityFeedResponse{Events: []ActivityEvent{}, HasMore: false})
	}))
	defer srv.Close()

	c := New()
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"base_url":    srv.URL,
			"api_key":     "key",
			"event_types": "task.created,expense.created",
		},
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	_, _, err := c.Sync(context.Background(), "2026-04-01T00:00:00Z")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if capturedTypes != "task.created,expense.created" {
		t.Errorf("string event_types not passed: got %q, want %q",
			capturedTypes, "task.created,expense.created")
	}
}

// --- H-013-003: Stale HealthSyncing on cursor parse error ---

func TestSyncBadCursorResetsHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ActivityFeedResponse{Events: []ActivityEvent{}, HasMore: false})
	}))
	defer srv.Close()

	c := New()
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"base_url": srv.URL,
			"api_key":  "key",
		},
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Confirm healthy after connect.
	if h := c.Health(context.Background()); h != connector.HealthHealthy {
		t.Fatalf("expected healthy after connect, got %v", h)
	}

	// Sync with an invalid cursor (not RFC3339).
	_, _, err := c.Sync(context.Background(), "not-a-valid-timestamp")
	if err == nil {
		t.Fatal("expected error for invalid cursor")
	}

	// Before fix: health would be stuck at HealthSyncing.
	// After fix: health should be HealthError.
	health := c.Health(context.Background())
	if health == connector.HealthSyncing {
		t.Errorf("health is stuck at HealthSyncing after cursor error — should be HealthError")
	}
	if health != connector.HealthError {
		t.Errorf("expected HealthError after bad cursor, got %v", health)
	}
}

// --- H-013-004: Dead baseOrigin field removed ---

func TestNewClientNoBaseOriginField(t *testing.T) {
	// After fix, Client struct should not have a baseOrigin field.
	// This test confirms that NewClient still works without the dead field
	// and that the URL construction path remains functional.
	c := NewClient("https://example.com", "key")
	if c.baseURL != "https://example.com" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "https://example.com")
	}
	if c.apiKey != "key" {
		t.Errorf("apiKey mismatch")
	}
}

// --- IMP-013-001: Trailing slash in base_url produces double-slash in API paths ---

func TestConnectTrailingSlashStripped(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New()
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"base_url": srv.URL + "/", // trailing slash
			"api_key":  "key",
		},
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Before fix: gotPath would be "//health" (double slash).
	// After fix: gotPath should be "/health".
	if gotPath != "/health" {
		t.Errorf("trailing slash in base_url caused malformed path: got %q, want /health", gotPath)
	}
}

func TestConnectMultipleTrailingSlashesStripped(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := New()
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"base_url": srv.URL + "///", // multiple trailing slashes
			"api_key":  "key",
		},
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	if gotPath != "/health" {
		t.Errorf("multiple trailing slashes not stripped: got path %q, want /health", gotPath)
	}
}

// --- IMP-013-002: Expense Amount and Booking TotalPrice IEEE 754 Inf/NaN guards ---

func TestNormalizeExpenseInfAmount(t *testing.T) {
	// Go 1.22+ rejects 1e999 at json.Unmarshal level; code guard is defense-in-depth.
	event := ActivityEvent{
		ID:        "evt-inf-expense",
		Type:      "expense.created",
		Timestamp: "2026-04-10T18:00:00Z",
		EntityID:  "x1",
		Data:      json.RawMessage(`{"propertyId":"p1","propertyName":"Lodge","category":"utilities","description":"Power","amount":1e999}`),
	}

	_, err := NormalizeEvent(event)
	if err == nil {
		t.Fatal("expected error for Inf expense amount, got nil — overflow must not poison downstream financial aggregation")
	}
}

func TestNormalizeExpenseNegativeInfAmount(t *testing.T) {
	event := ActivityEvent{
		ID:        "evt-neginf-expense",
		Type:      "expense.created",
		Timestamp: "2026-04-10T18:00:00Z",
		EntityID:  "x2",
		Data:      json.RawMessage(`{"propertyId":"p1","propertyName":"Lodge","category":"repair","description":"Roof","amount":-1e999}`),
	}

	_, err := NormalizeEvent(event)
	if err == nil {
		t.Fatal("expected error for -Inf expense amount")
	}
}

func TestNormalizeBookingInfTotalPrice(t *testing.T) {
	// Go 1.22+ rejects 1e999 at json.Unmarshal level; bookingMetadata guard is defense-in-depth.
	// Verify the error IS caught at some level.
	event := ActivityEvent{
		ID:        "evt-inf-booking",
		Type:      "booking.created",
		Timestamp: "2026-04-10T14:00:00Z",
		EntityID:  "b1",
		Data:      json.RawMessage(`{"propertyId":"p1","propertyName":"Lodge","guestName":"Eve","guestEmail":"e@t.com","checkinDate":"2026-05-01","checkoutDate":"2026-05-03","source":"direct","totalAmount":1e999}`),
	}

	_, err := NormalizeEvent(event)
	if err == nil {
		t.Fatal("expected error for Inf booking totalPrice — overflow must not reach metadata")
	}
}

func TestNormalizeExpenseFiniteAmountPasses(t *testing.T) {
	// Verify that normal finite expense amounts still work after the guard.
	event := ActivityEvent{
		ID:        "evt-normal-expense",
		Type:      "expense.created",
		Timestamp: "2026-04-10T18:00:00Z",
		EntityID:  "x3",
		Data:      json.RawMessage(`{"propertyId":"p1","propertyName":"Lodge","category":"supplies","description":"Soap","amount":42.50}`),
	}

	a, err := NormalizeEvent(event)
	if err != nil {
		t.Fatalf("NormalizeEvent: %v", err)
	}
	amount, ok := a.Metadata["amount"].(float64)
	if !ok {
		t.Fatalf("metadata amount not float64")
	}
	if amount != -42.50 {
		t.Errorf("amount = %v, want -42.50", amount)
	}
}

// --- IMP-013-003: Pagination cursor length cap (OOM defence) ---

func TestFetchActivityOversizedCursorRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return a response with HasMore=true and an absurdly large cursor.
		bigCursor := strings.Repeat("A", 5000) // exceeds maxCursorLen (4096)
		json.NewEncoder(w).Encode(ActivityFeedResponse{
			Events: []ActivityEvent{
				{ID: "e1", Type: "guest.created", Timestamp: "2026-04-01T10:00:00Z", EntityID: "g1", Data: json.RawMessage(`{"email":"a@b.com","name":"A"}`)},
			},
			HasMore: true,
			Cursor:  bigCursor,
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	_, err := client.FetchActivity(context.Background(), "", "", 10)
	if err == nil {
		t.Fatal("expected error for oversized cursor — without this cap, a malicious server can OOM the client via cursor inflation")
	}
	if !strings.Contains(err.Error(), "oversized cursor") {
		t.Errorf("error should mention oversized cursor, got: %v", err)
	}
}

func TestFetchActivityNormalCursorAccepted(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		w.Header().Set("Content-Type", "application/json")
		if page == 1 {
			json.NewEncoder(w).Encode(ActivityFeedResponse{
				Events:  []ActivityEvent{{ID: "e1", Type: "guest.created", Timestamp: "2026-04-01T10:00:00Z", EntityID: "g1", Data: json.RawMessage(`{"email":"a@b.com","name":"A"}`)}},
				HasMore: true,
				Cursor:  "normal-cursor-value",
			})
		} else {
			json.NewEncoder(w).Encode(ActivityFeedResponse{
				Events:  []ActivityEvent{{ID: "e2", Type: "guest.created", Timestamp: "2026-04-01T11:00:00Z", EntityID: "g2", Data: json.RawMessage(`{"email":"b@c.com","name":"B"}`)}},
				HasMore: false,
			})
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	resp, err := client.FetchActivity(context.Background(), "", "", 10)
	if err != nil {
		t.Fatalf("normal cursor should be accepted: %v", err)
	}
	if len(resp.Events) != 2 {
		t.Errorf("expected 2 events across 2 pages, got %d", len(resp.Events))
	}
}
