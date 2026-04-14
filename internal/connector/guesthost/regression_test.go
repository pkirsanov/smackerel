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
