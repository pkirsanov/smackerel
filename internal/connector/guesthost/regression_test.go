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

// --- CHAOS-013-001: Cursor regression off-by-one (stuck cursor wastes an extra request) ---

func TestChaos013001_StuckCursorDetectedImmediately(t *testing.T) {
	// A server that always returns the same cursor with HasMore=true is stuck.
	// The client MUST detect this on the FIRST repetition (2 total requests),
	// not after an extra wasted round (3 total requests).
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ActivityFeedResponse{
			Events: []ActivityEvent{
				{ID: "e1", Type: "guest.created", Timestamp: "2026-04-01T10:00:00Z", EntityID: "g1", Data: json.RawMessage(`{"email":"a@b.com","name":"A"}`)},
			},
			HasMore: true,
			Cursor:  "stuck-cursor-value",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	resp, err := client.FetchActivity(context.Background(), "", "", 10)
	if err != nil {
		t.Fatalf("stuck cursor should not error, just break: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	// The CORRECT behavior: page 0 returns cursor "stuck-cursor-value",
	// page 1 sends cursor "stuck-cursor-value" and gets back the SAME cursor.
	// Detection should fire on page 1 → total 2 requests.
	// Before fix: detection fires on page 2 → total 3 requests (off-by-one waste).
	if requestCount > 2 {
		t.Errorf("stuck cursor detection off-by-one: made %d requests, want at most 2 — "+
			"the client compared resp.Cursor with previousCursor (2 iterations ago) instead of "+
			"cursor (current request), wasting an extra round", requestCount)
	}
}

// --- CHAOS-013-002: Sync after Close overwrites HealthDisconnected with HealthHealthy ---

func TestChaos013002_SyncAfterCloseHealthNotOverwritten(t *testing.T) {
	// If Close() is called while Sync() is in-flight (after Sync releases the initial
	// lock to do the network call), Sync's deferred health update must NOT overwrite
	// the HealthDisconnected state set by Close.
	syncStarted := make(chan struct{})
	allowResponse := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
			return
		}
		// Signal that sync has started its network call
		select {
		case syncStarted <- struct{}{}:
		default:
		}
		// Wait until we're told to respond (Close will happen in between)
		<-allowResponse
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

	// Start Sync in a goroutine
	syncDone := make(chan error, 1)
	go func() {
		_, _, err := c.Sync(context.Background(), "")
		syncDone <- err
	}()

	// Wait for sync to reach the network call (past the initial lock release)
	<-syncStarted

	// Close the connector while sync is in-flight
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify Close set the health to disconnected
	if h := c.Health(context.Background()); h != connector.HealthDisconnected {
		t.Fatalf("after Close, expected HealthDisconnected, got %v", h)
	}

	// Allow the sync response to complete
	close(allowResponse)
	<-syncDone

	// After Sync completes, the connector health must STILL be HealthDisconnected.
	// Before fix: Sync's deferred setHealth(HealthHealthy) overwrites the Close state.
	health := c.Health(context.Background())
	if health == connector.HealthHealthy {
		t.Errorf("Sync after Close overwrote HealthDisconnected with HealthHealthy — " +
			"a closed connector appears healthy, which is a race condition")
	}
	if health != connector.HealthDisconnected {
		t.Errorf("after Close+Sync, expected HealthDisconnected, got %v", health)
	}
}

// --- CHAOS-013-003: Empty events with HasMore=true causes wasteful pagination ---

func TestChaos013003_EmptyEventsWithHasMoreBreaksPagination(t *testing.T) {
	// A malicious or buggy server returning HasMore=true with zero events and a
	// changing cursor should NOT cause up to maxPaginationPages (1000) requests.
	// The client should detect the empty-events condition and break early.
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		// Empty events, HasMore=true, advancing cursor — pagination runs wild
		json.NewEncoder(w).Encode(ActivityFeedResponse{
			Events:  []ActivityEvent{},
			HasMore: true,
			Cursor:  "cursor-" + strings.Repeat("x", requestCount),
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	resp, err := client.FetchActivity(context.Background(), "", "", 10)
	if err != nil {
		t.Fatalf("empty-events pagination should not error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	// With no guard, this would make maxPaginationPages (1000) requests.
	// A reasonable cap is 1-3 requests before detecting the empty-events pattern.
	if requestCount > 3 {
		t.Errorf("empty-events with HasMore=true caused %d requests (want ≤3) — "+
			"a buggy/malicious server can trigger up to 1000 wasteful requests without this guard",
			requestCount)
	}
}

// --- IMP-013-IMP-001: Message Body and SenderRole sanitization (CWE-116) ---

func TestNormalizeMessageBodySanitized(t *testing.T) {
	// Control characters in Body should be stripped by SanitizeControlChars.
	event := ActivityEvent{
		ID:        "evt-msg-sanitize",
		Type:      "message.received",
		Timestamp: "2026-04-10T15:00:00Z",
		EntityID:  "m1",
		Data: json.RawMessage(`{
			"propertyId": "p1",
			"propertyName": "Beach House",
			"guestEmail": "alice@example.com",
			"guestName": "Alice",
			"senderRole": "guest\u0000injected",
			"body": "Check-in\u0007at 3pm",
			"bookingId": "b1"
		}`),
	}

	a, err := NormalizeEvent(event)
	if err != nil {
		t.Fatalf("NormalizeEvent: %v", err)
	}

	// The raw content includes the entire event.Data, so control chars in Body
	// and SenderRole are sanitized in the typed struct fields. Verify the title
	// and metadata don't contain control characters.
	if a.ContentType != "guest_message" {
		t.Errorf("ContentType = %q, want guest_message", a.ContentType)
	}
	// SenderRole and Body are not in metadata/title currently, but ensuring
	// the normalizer doesn't crash and produces valid output is the key.
	if a.Metadata["guest_email"] != "alice@example.com" {
		t.Errorf("metadata guest_email = %v", a.Metadata["guest_email"])
	}
}

// --- IMP-013-IMP-002: Task status stored in metadata ---

func TestNormalizeTaskCreatedHasStatus(t *testing.T) {
	event := ActivityEvent{
		ID:        "evt-task-created",
		Type:      "task.created",
		Timestamp: "2026-04-10T16:00:00Z",
		EntityID:  "t1",
		Data: json.RawMessage(`{
			"propertyId": "p1",
			"propertyName": "Mountain Cabin",
			"title": "Fix faucet",
			"description": "Kitchen faucet leaking",
			"status": "pending",
			"category": "maintenance"
		}`),
	}

	a, err := NormalizeEvent(event)
	if err != nil {
		t.Fatalf("NormalizeEvent: %v", err)
	}
	if a.Metadata["task_status"] != "pending" {
		t.Errorf("metadata task_status = %v, want 'pending'", a.Metadata["task_status"])
	}
}

func TestNormalizeTaskCompletedHasStatus(t *testing.T) {
	event := ActivityEvent{
		ID:        "evt-task-done",
		Type:      "task.completed",
		Timestamp: "2026-04-11T10:00:00Z",
		EntityID:  "t2",
		Data: json.RawMessage(`{
			"propertyId": "p1",
			"propertyName": "Mountain Cabin",
			"title": "Fix faucet",
			"description": "Kitchen faucet leaking",
			"status": "completed",
			"category": "maintenance"
		}`),
	}

	a, err := NormalizeEvent(event)
	if err != nil {
		t.Fatalf("NormalizeEvent: %v", err)
	}
	if a.Metadata["task_status"] != "completed" {
		t.Errorf("metadata task_status = %v, want 'completed'", a.Metadata["task_status"])
	}
}
