package hospitable

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/smackerel/smackerel/internal/connector"
)

// --- Chaos: Malformed API responses ---

func TestChaos_MalformedJSON_Response(t *testing.T) {
	cases := map[string]string{
		"truncated":     `{"data": [{"id": "p1", "name": "Hou`,
		"empty":         ``,
		"binary":        string([]byte{0x00, 0x01, 0xFF, 0xFE}),
		"just_null":     `null`,
		"just_string":   `"hello"`,
		"just_number":   `42`,
		"html_error":    `<html><body>502 Bad Gateway</body></html>`,
		"nested_broken": `{"data": [{"id": "p1", "name": {"nested": true}}], "total": 1}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(body))
			}))
			defer srv.Close()

			client := NewClient(srv.URL, "token", 10)
			_, err := client.ListProperties(context.Background(), time.Time{})
			// Must not panic; error is expected for corrupt responses
			if err == nil && name != "just_null" {
				// just_null may decode as empty data, which is tolerable
				t.Logf("no error for %s — check if response was silently accepted", name)
			}
		})
	}
}

func TestChaos_EmptyDataArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token", 10)
	props, err := client.ListProperties(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("empty data should not error: %v", err)
	}
	if len(props) != 0 {
		t.Errorf("expected 0 properties, got %d", len(props))
	}
}

func TestChaos_NullDataField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data": null, "total": 0}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token", 10)
	props, err := client.ListProperties(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("null data field should not error: %v", err)
	}
	if len(props) != 0 {
		t.Errorf("expected 0 properties from null data, got %d", len(props))
	}
}

func TestChaos_MissingDataField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total": 0}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token", 10)
	props, err := client.ListProperties(context.Background(), time.Time{})
	// Should not panic, may return empty
	if err != nil {
		t.Fatalf("missing data field should not error: %v", err)
	}
	if len(props) != 0 {
		t.Errorf("expected 0 properties from missing data, got %d", len(props))
	}
}

// --- Chaos: Empty/null field handling ---

func TestChaos_PropertyAllFieldsEmpty(t *testing.T) {
	cfg := HospitableConfig{TierProperties: "light"}
	p := Property{} // All zero-values

	a := NormalizeProperty(p, cfg)

	if a.SourceID != "hospitable" {
		t.Errorf("SourceID = %q, want hospitable", a.SourceID)
	}
	if a.SourceRef != "property:" {
		t.Errorf("SourceRef = %q, want property:", a.SourceRef)
	}
	// Should not panic on empty address
	if !utf8.ValidString(a.RawContent) {
		t.Errorf("CHAOS: RawContent is invalid UTF-8")
	}
	// CapturedAt should fall back to now, not zero
	if a.CapturedAt.IsZero() {
		t.Errorf("CHAOS: CapturedAt is zero for empty property — should fall back to now")
	}
}

func TestChaos_ReservationAllFieldsEmpty(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard"}
	r := Reservation{} // All zero-values

	a := NormalizeReservation(r, "", cfg)

	if a.SourceID != "hospitable" {
		t.Errorf("SourceID = %q, want hospitable", a.SourceID)
	}
	// CapturedAt should not be zero
	if a.CapturedAt.IsZero() {
		t.Errorf("CHAOS: CapturedAt is zero for empty reservation")
	}
	if !utf8.ValidString(a.Title) {
		t.Errorf("CHAOS: Title is invalid UTF-8: %q", a.Title)
	}
	if !utf8.ValidString(a.RawContent) {
		t.Errorf("CHAOS: RawContent is invalid UTF-8")
	}
}

func TestChaos_MessageAllFieldsEmpty(t *testing.T) {
	cfg := HospitableConfig{TierMessages: "full"}
	m := Message{} // All zero-values

	a := NormalizeMessage(m, "", cfg)

	if a.SourceID != "hospitable" {
		t.Errorf("SourceID = %q, want hospitable", a.SourceID)
	}
	if a.CapturedAt.IsZero() {
		t.Errorf("CHAOS: CapturedAt is zero for empty message")
	}
	if !utf8.ValidString(a.Title) {
		t.Errorf("CHAOS: Title is invalid UTF-8: %q", a.Title)
	}
}

func TestChaos_ReviewAllFieldsEmpty(t *testing.T) {
	cfg := HospitableConfig{TierReviews: "full"}
	r := Review{} // All zero-values

	a := NormalizeReview(r, "", cfg)

	if a.SourceID != "hospitable" {
		t.Errorf("SourceID = %q, want hospitable", a.SourceID)
	}
	if a.CapturedAt.IsZero() {
		t.Errorf("CHAOS: CapturedAt is zero for empty review")
	}
	if !utf8.ValidString(a.Title) {
		t.Errorf("CHAOS: Title is invalid UTF-8: %q", a.Title)
	}
}

// --- Chaos: Extremely long guest names ---

func TestChaos_ExtremelyLongGuestName(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard"}
	longName := strings.Repeat("A", 10000)
	r := Reservation{
		ID:         "res-long",
		PropertyID: "p1",
		GuestName:  longName,
		CheckIn:    "2026-04-15",
		CheckOut:   "2026-04-18",
		BookedAt:   time.Now(),
	}

	a := NormalizeReservation(r, "Beach House", cfg)

	// Must not panic, title must be valid UTF-8
	if !utf8.ValidString(a.Title) {
		t.Errorf("CHAOS: title with long guest name is invalid UTF-8")
	}
	// Content must be valid
	if !utf8.ValidString(a.RawContent) {
		t.Errorf("CHAOS: content with long guest name is invalid UTF-8")
	}
	// Metadata should store the full name
	if a.Metadata["guest_name"] != longName {
		t.Errorf("CHAOS: metadata guest_name was truncated")
	}
}

func TestChaos_GuestNameWithNullBytes(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard"}
	r := Reservation{
		ID:         "res-null",
		PropertyID: "p1",
		GuestName:  "John\x00Smith",
		CheckIn:    "2026-04-15",
		CheckOut:   "2026-04-18",
		BookedAt:   time.Now(),
	}

	// Must not panic
	a := NormalizeReservation(r, "Beach House", cfg)
	if a.SourceRef == "" {
		t.Errorf("CHAOS: empty SourceRef after null-byte guest name")
	}
}

func TestChaos_GuestNameNewlinesAndTabs(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard"}
	r := Reservation{
		ID:         "res-newline",
		PropertyID: "p1",
		GuestName:  "John\nSmith\r\n\tJr.",
		CheckIn:    "2026-04-15",
		CheckOut:   "2026-04-18",
		BookedAt:   time.Now(),
	}

	a := NormalizeReservation(r, "Beach House", cfg)
	if !utf8.ValidString(a.Title) {
		t.Errorf("CHAOS: title with newline guest name invalid UTF-8")
	}
	if !utf8.ValidString(a.RawContent) {
		t.Errorf("CHAOS: content with newline guest name invalid UTF-8")
	}
}

// --- Chaos: Unicode in property names and messages ---

func TestChaos_UnicodePropertyNames(t *testing.T) {
	cfg := HospitableConfig{TierProperties: "light"}

	unicodeNames := []string{
		"Casa de la Playa 🏖️",
		"Maison d'été à côté de la mer",
		"東京のアパート",
		"Квартира в Москве",
		"شقة في دبي",
		"ที่พักกรุงเทพ",
		"Ñoño's Casita™ — «Premium»",
		strings.Repeat("🏠", 500), // 500 house emojis = 2000 bytes
	}

	for i, name := range unicodeNames {
		t.Run(fmt.Sprintf("unicode_%d", i), func(t *testing.T) {
			p := Property{
				ID:        fmt.Sprintf("p-%d", i),
				Name:      name,
				UpdatedAt: time.Now(),
			}

			a := NormalizeProperty(p, cfg)

			if !utf8.ValidString(a.Title) {
				t.Errorf("CHAOS: property title invalid UTF-8 for %q", name)
			}
			if !utf8.ValidString(a.RawContent) {
				t.Errorf("CHAOS: property content invalid UTF-8 for %q", name)
			}
			if a.Title != name {
				t.Errorf("CHAOS: property name not preserved: got %q, want %q", a.Title, name)
			}
		})
	}
}

func TestChaos_UnicodeMessageBodies(t *testing.T) {
	cfg := HospitableConfig{TierMessages: "full"}

	bodies := []string{
		"What's the Wi-Fi password? 🤔",
		"", // empty body
		strings.Repeat("مرحبا ", 1000),            // Arabic
		"Привет! Как дела?\nОтлично 👍",           // Russian with emoji
		"\x00\x01\x02\x03",                        // control chars
		strings.Repeat("a\u0300", 500),             // combining diacriticals
		"<script>alert('xss')</script>",            // XSS attempt
		"'; DROP TABLE messages; --",               // SQL injection attempt
		string([]byte{0xED, 0xA0, 0x80}),           // invalid UTF-8 (surrogate half)
	}

	for i, body := range bodies {
		t.Run(fmt.Sprintf("body_%d", i), func(t *testing.T) {
			m := Message{
				ID:     fmt.Sprintf("m-%d", i),
				Sender: "Test User",
				Body:   body,
				SentAt: time.Now(),
			}

			// Must not panic
			a := NormalizeMessage(m, "res-1", cfg)
			if a.SourceID != "hospitable" {
				t.Errorf("CHAOS: SourceID lost for message body %d", i)
			}
		})
	}
}

func TestChaos_UnicodeReviewText(t *testing.T) {
	cfg := HospitableConfig{TierReviews: "full"}

	r := Review{
		ID:           "rev-uni",
		PropertyID:   "p1",
		Rating:       5,
		ReviewText:   "素晴らしいステイでした！🎉 Fantastic séjour à la maison — très belle vue 🌅",
		HostResponse: "Merci beaucoup! ありがとうございます 🙏",
		Channel:      "Airbnb",
		SubmittedAt:  time.Now(),
	}

	a := NormalizeReview(r, "Maison côtière", cfg)
	if !utf8.ValidString(a.Title) {
		t.Errorf("CHAOS: review title invalid UTF-8")
	}
	if !utf8.ValidString(a.RawContent) {
		t.Errorf("CHAOS: review content invalid UTF-8")
	}
	if !containsStr(a.RawContent, "素晴らしい") {
		t.Errorf("CHAOS: review text not preserved in content")
	}
	if !containsStr(a.RawContent, "ありがとう") {
		t.Errorf("CHAOS: host response not preserved in content")
	}
}

// --- Chaos: Pagination edge cases ---

func TestChaos_PaginationInfiniteLoop(t *testing.T) {
	// Server always returns a next link pointing to itself — client must not loop forever
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount > 200 {
			// Safety valve — if client loops more than 200 times, break
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		resp := PaginatedResponse[Property]{
			Data:  []Property{{ID: fmt.Sprintf("p%d", callCount), Name: "Loop Property"}},
			Total: 1000,
		}
		// Always set next, creating an infinite loop scenario
		w.Header().Set("Link", fmt.Sprintf(`<%s/properties?page=%d>; rel="next"`, r.Host, callCount+1))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token", 10)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	props, err := client.ListProperties(ctx, time.Time{})
	// Should eventually stop — either via context timeout or natural termination
	// The key assertion is that it doesn't hang forever
	t.Logf("Pagination loop: got %d properties, err=%v, calls=%d", len(props), err, callCount)
}

func TestChaos_PaginationEmptyNextURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := PaginatedResponse[Property]{
			Data:    []Property{{ID: "p1", Name: "Solo"}},
			NextURL: "", // explicitly empty
			Total:   1,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token", 10)
	props, err := client.ListProperties(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(props) != 1 {
		t.Errorf("expected 1 property, got %d", len(props))
	}
}

func TestChaos_PaginationMalformedLinkHeader(t *testing.T) {
	malformedLinks := []string{
		`not a real link header`,
		`<>; rel="next"`,
		`; rel="next"`,
		`<http://example.com/next>`,       // missing rel
		`<http://example.com/next>; rel=`, // empty rel
		`<http://example.com/next>; rel="prev"`,
		`,,,,`,
	}

	for i, link := range malformedLinks {
		t.Run(fmt.Sprintf("link_%d", i), func(t *testing.T) {
			callCount := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callCount++
				if callCount > 1 {
					t.Errorf("CHAOS: followed malformed link header %q", link)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Header().Set("Link", link)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(PaginatedResponse[Property]{
					Data: []Property{{ID: "p1"}}, Total: 1,
				})
			}))
			defer srv.Close()

			client := NewClient(srv.URL, "token", 10)
			props, err := client.ListProperties(context.Background(), time.Time{})
			if err != nil {
				t.Fatalf("malformed link header should not cause error: %v", err)
			}
			if len(props) != 1 {
				t.Errorf("expected 1 property, got %d", len(props))
			}
		})
	}
}

// --- Chaos: Auth token expiry during sync ---

func TestChaos_TokenExpiryMidSync(t *testing.T) {
	// Properties succeed, then reservations get 401
	reqCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		w.Header().Set("Content-Type", "application/json")

		if containsStr(r.URL.Path, "/properties") {
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{
				Data: []Property{{ID: "p1", Name: "House"}}, Total: 1,
			})
			return
		}
		// Everything else after properties gets 401 (token expired)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"token expired"}`))
	}))
	defer srv.Close()

	c := New("hospitable")
	c.config = HospitableConfig{
		AccessToken:         "expiring-token",
		BaseURL:             srv.URL,
		PageSize:            10,
		SyncProperties:      true,
		SyncReservations:    true,
		SyncMessages:        false,
		SyncReviews:         true,
		TierProperties:      "light",
		TierReservations:    "standard",
		TierReviews:         "full",
		InitialLookbackDays: 90,
	}
	c.client = NewClient(srv.URL, "expiring-token", 10)
	c.client.SetBackoff(fastBackoff())
	c.health = connector.HealthHealthy

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync should not return error on partial auth failure: %v", err)
	}

	// Should have at least the property artifact
	if len(artifacts) < 1 {
		t.Errorf("CHAOS: expected at least 1 artifact (properties), got %d", len(artifacts))
	}

	// Properties should have succeeded
	hasProperty := false
	for _, a := range artifacts {
		if a.ContentType == "property/str-listing" {
			hasProperty = true
		}
	}
	if !hasProperty {
		t.Error("CHAOS: property artifact missing despite properties API succeeding")
	}
}

func TestChaos_TokenExpiryOnValidate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"token expired"}`))
	}))
	defer srv.Close()

	c := New("hospitable")
	config := connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": "expired-token"},
		SourceConfig: map[string]interface{}{"base_url": srv.URL},
	}

	err := c.Connect(context.Background(), config)
	if err == nil {
		t.Fatal("CHAOS: Connect should fail with expired token")
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("CHAOS: health should be error after auth failure, got %v", c.Health(context.Background()))
	}
}

// --- Chaos: Concurrent operations ---

func TestChaos_ConcurrentSync(t *testing.T) {
	var reqCount atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case containsStr(r.URL.Path, "/properties"):
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{
				Data: []Property{{ID: "p1", Name: "House"}}, Total: 1,
			})
		case containsStr(r.URL.Path, "/messages"):
			json.NewEncoder(w).Encode(PaginatedResponse[Message]{Data: []Message{}, Total: 0})
		case containsStr(r.URL.Path, "/reservations"):
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{
				Data: []Reservation{{
					ID: "r1", PropertyID: "p1", GuestName: "Test",
					CheckIn: "2026-04-15", CheckOut: "2026-04-18", BookedAt: time.Now(),
				}},
				Total: 1,
			})
		case containsStr(r.URL.Path, "/reviews"):
			json.NewEncoder(w).Encode(PaginatedResponse[Review]{
				Data: []Review{{ID: "rev1", PropertyID: "p1", Rating: 5, ReviewText: "Great", SubmittedAt: time.Now()}},
				Total: 1,
			})
		default:
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
		}
	}))
	defer srv.Close()

	c := New("hospitable")
	c.config = HospitableConfig{
		AccessToken:         "token",
		BaseURL:             srv.URL,
		PageSize:            10,
		SyncProperties:      true,
		SyncReservations:    true,
		SyncMessages:        true,
		SyncReviews:         true,
		TierProperties:      "light",
		TierReservations:    "standard",
		TierMessages:        "full",
		TierReviews:         "full",
		InitialLookbackDays: 90,
	}
	c.client = NewClient(srv.URL, "token", 10)
	c.health = connector.HealthHealthy

	var wg sync.WaitGroup
	errCh := make(chan error, 10)

	// Launch 10 concurrent syncs
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			artifacts, cursor, err := c.Sync(context.Background(), "")
			if err != nil {
				errCh <- fmt.Errorf("Sync error: %w", err)
				return
			}
			if len(artifacts) == 0 {
				errCh <- fmt.Errorf("expected artifacts, got 0")
				return
			}
			if cursor == "" {
				errCh <- fmt.Errorf("expected non-empty cursor")
				return
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("CHAOS concurrent sync: %v", err)
	}
}

func TestChaos_ConcurrentConnectAndSync(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{{ID: "p1", Name: "House"}}, Total: 1})
	}))
	defer srv.Close()

	c := New("hospitable")
	config := connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": "token"},
		SourceConfig: map[string]interface{}{"base_url": srv.URL},
	}

	var wg sync.WaitGroup

	// Connect in one goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = c.Connect(context.Background(), config)
	}()

	// Try syncing concurrently (may fail if not connected yet — that's fine)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Must not panic regardless of connect state
			c.Sync(context.Background(), "")
		}()
	}

	wg.Wait()
	// The key assertion: no panics, no data races (run with -race)
}

func TestChaos_ConcurrentHealthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
	}))
	defer srv.Close()

	c := New("hospitable")
	config := connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": "token"},
		SourceConfig: map[string]interface{}{"base_url": srv.URL},
	}
	c.Connect(context.Background(), config)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Health(context.Background())
		}()
	}
	wg.Wait()
}

// --- Chaos: Missing required API fields ---

func TestChaos_PropertyMissingID(t *testing.T) {
	cfg := HospitableConfig{TierProperties: "light"}
	p := Property{
		Name:      "No ID Property",
		UpdatedAt: time.Now(),
	}

	a := NormalizeProperty(p, cfg)
	// Should produce "property:" with empty ID — not panic
	if a.SourceRef != "property:" {
		t.Errorf("SourceRef = %q", a.SourceRef)
	}
}

func TestChaos_ReservationMissingDates(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard"}
	r := Reservation{
		ID:         "res-nodate",
		PropertyID: "p1",
		GuestName:  "John",
		// CheckIn and CheckOut are empty strings
	}

	a := NormalizeReservation(r, "Beach House", cfg)
	// Must not panic on empty date strings
	if !utf8.ValidString(a.Title) {
		t.Errorf("CHAOS: title invalid UTF-8 with missing dates")
	}
	if !utf8.ValidString(a.RawContent) {
		t.Errorf("CHAOS: content invalid UTF-8 with missing dates")
	}
}

func TestChaos_ReservationInvalidDateFormat(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard"}
	r := Reservation{
		ID:         "res-baddate",
		PropertyID: "p1",
		GuestName:  "John",
		CheckIn:    "not-a-date",
		CheckOut:   "2026/04/18",
		BookedAt:   time.Now(),
	}

	// Must not panic on unparseable dates
	a := NormalizeReservation(r, "Beach House", cfg)
	if !utf8.ValidString(a.Title) {
		t.Errorf("CHAOS: title invalid UTF-8 with invalid dates")
	}
}

func TestChaos_MessageMissingSender(t *testing.T) {
	cfg := HospitableConfig{TierMessages: "full"}
	m := Message{
		ID:     "msg-nosender",
		Body:   "Message with no sender",
		SentAt: time.Now(),
	}

	a := NormalizeMessage(m, "res-1", cfg)
	// Should produce "Message from  (guest)" or similar, not panic
	if a.SourceID != "hospitable" {
		t.Errorf("CHAOS: SourceID lost for no-sender message")
	}
}

// --- Chaos: Rate limit responses ---

func TestChaos_RateLimitWithZeroRetryAfter(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{{ID: "p1"}}, Total: 1})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token", 10)
	client.SetBackoff(fastBackoff())
	props, err := client.ListProperties(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("should eventually succeed: %v", err)
	}
	if len(props) != 1 {
		t.Errorf("expected 1 property, got %d", len(props))
	}
}

func TestChaos_RateLimitWithNegativeRetryAfter(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "-5")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{{ID: "p1"}}, Total: 1})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token", 10)
	client.SetBackoff(fastBackoff())
	props, err := client.ListProperties(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("negative retry-after should not block: %v", err)
	}
	if len(props) != 1 {
		t.Errorf("expected 1 property, got %d", len(props))
	}
}

func TestChaos_RateLimitWithHugeRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "999999")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token", 10)
	client.SetBackoff(fastBackoff())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.ListProperties(ctx, time.Time{})
	// Should fail (context timeout or max retries) not hang
	if err == nil {
		t.Error("CHAOS: expected error with huge retry-after, not indefinite wait")
	}
}

func TestChaos_RateLimitWithMalformedRetryAfter(t *testing.T) {
	malformed := []string{"not-a-number", "abc", "1.5", "", " ", "null"}

	for _, val := range malformed {
		t.Run(val, func(t *testing.T) {
			attempts := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts++
				if attempts == 1 {
					w.Header().Set("Retry-After", val)
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{{ID: "p1"}}, Total: 1})
			}))
			defer srv.Close()

			client := NewClient(srv.URL, "token", 10)
			client.SetBackoff(fastBackoff())
			props, err := client.ListProperties(context.Background(), time.Time{})
			if err != nil {
				t.Fatalf("malformed retry-after %q should not block: %v", val, err)
			}
			if len(props) != 1 {
				t.Errorf("expected 1 property, got %d", len(props))
			}
		})
	}
}

// --- Chaos: Server error variations ---

func TestChaos_ServerErrorWithBody(t *testing.T) {
	errorBodies := map[string]struct {
		status int
		body   string
	}{
		"500_html":        {500, "<html><body>Internal Server Error</body></html>"},
		"502_json":        {502, `{"error":"bad gateway","request_id":"abc123"}`},
		"503_empty":       {503, ""},
		"504_binary":      {504, string([]byte{0x00, 0xFF})},
		"500_huge":        {500, strings.Repeat("error ", 10000)},
		"599_nonstandard": {599, `{"message":"custom error"}`},
	}

	for name, tc := range errorBodies {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			client := NewClient(srv.URL, "token", 10)
			client.SetBackoff(fastBackoff())
			_, err := client.ListProperties(context.Background(), time.Time{})
			if err == nil {
				t.Errorf("CHAOS: expected error for %d response", tc.status)
			}
		})
	}
}

// --- Chaos: Context cancellation ---

func TestChaos_ContextCancelledDuringSync(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case containsStr(r.URL.Path, "/properties"):
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{
				Data: []Property{{ID: "p1", Name: "House"}}, Total: 1,
			})
		default:
			// Delay to allow cancellation to take effect
			time.Sleep(100 * time.Millisecond)
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{Data: []Reservation{}, Total: 0})
		}
	}))
	defer srv.Close()

	c := New("hospitable")
	c.config = HospitableConfig{
		AccessToken:         "token",
		BaseURL:             srv.URL,
		PageSize:            10,
		SyncProperties:      true,
		SyncReservations:    true,
		SyncMessages:        true,
		SyncReviews:         true,
		TierProperties:      "light",
		TierReservations:    "standard",
		TierMessages:        "full",
		TierReviews:         "full",
		InitialLookbackDays: 90,
	}
	c.client = NewClient(srv.URL, "token", 10)
	c.health = connector.HealthHealthy

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Should not panic on context cancellation
	_, _, _ = c.Sync(ctx, "")
}

// --- Chaos: Extreme numeric values ---

func TestChaos_ExtremeNumericValues(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard"}

	cases := []struct {
		name        string
		nightlyRate float64
		totalPayout float64
		guestCount  int
		nights      int
	}{
		{"zero_values", 0, 0, 0, 0},
		{"negative_rate", -100, -500, -2, -3},
		{"huge_rate", 999999999.99, 999999999999.99, 99999, 99999},
		{"nan", math.NaN(), math.NaN(), 0, 0},
		{"inf", math.Inf(1), math.Inf(-1), 0, 0},
		{"max_float", math.MaxFloat64, math.MaxFloat64, math.MaxInt32, math.MaxInt32},
		{"min_float", math.SmallestNonzeroFloat64, math.SmallestNonzeroFloat64, 0, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Reservation{
				ID:          "res-extreme",
				PropertyID:  "p1",
				GuestName:   "Test",
				CheckIn:     "2026-04-15",
				CheckOut:    "2026-04-18",
				NightlyRate: tc.nightlyRate,
				TotalPayout: tc.totalPayout,
				GuestCount:  tc.guestCount,
				Nights:      tc.nights,
				BookedAt:    time.Now(),
			}

			// Must not panic
			a := NormalizeReservation(r, "Beach House", cfg)
			if !utf8.ValidString(a.RawContent) {
				t.Errorf("CHAOS: content invalid UTF-8 for %s", tc.name)
			}
		})
	}
}

func TestChaos_ExtremeRatingValues(t *testing.T) {
	cfg := HospitableConfig{TierReviews: "full"}

	ratings := []float64{0, -1, 100, math.NaN(), math.Inf(1), math.Inf(-1), 4.999999999, math.MaxFloat64}

	for _, rating := range ratings {
		t.Run(fmt.Sprintf("rating_%v", rating), func(t *testing.T) {
			r := Review{
				ID:          "rev-extreme",
				PropertyID:  "p1",
				Rating:      rating,
				ReviewText:  "Test",
				SubmittedAt: time.Now(),
			}

			// Must not panic
			a := NormalizeReview(r, "Beach House", cfg)
			if !utf8.ValidString(a.Title) {
				t.Errorf("CHAOS: title invalid UTF-8 for rating %v", rating)
			}
		})
	}
}

// --- Chaos: Cursor corruption ---

func TestChaos_CorruptedCursor(t *testing.T) {
	corruptCursors := []string{
		`not-json`,
		`{"properties":"not-a-time"}`,
		`{"properties":"2026-04-01T00:00:00Z","reservations":null}`,
		`{`,
		`[]`,
		`""`,
		`0`,
		strings.Repeat("x", 100000), // 100KB of garbage
	}

	for i, cursor := range corruptCursors {
		t.Run(fmt.Sprintf("corrupt_%d", i), func(t *testing.T) {
			// Should not panic, should fall back to lookback
			sc := parseCursor(cursor, 90)
			// Reservations should have a sane value (lookback or parsed)
			if sc.Reservations.After(time.Now().Add(time.Hour)) {
				t.Errorf("CHAOS: cursor reservations in the future: %v", sc.Reservations)
			}
		})
	}
}

// --- Chaos: Config edge cases ---

func TestChaos_ConfigZeroPageSize(t *testing.T) {
	client := NewClient("http://example.com", "token", 0)
	if client.pageSize != 100 {
		t.Errorf("CHAOS: zero page size should default to 100, got %d", client.pageSize)
	}
}

func TestChaos_ConfigNegativePageSize(t *testing.T) {
	client := NewClient("http://example.com", "token", -1)
	if client.pageSize != 100 {
		t.Errorf("CHAOS: negative page size should default to 100, got %d", client.pageSize)
	}
}

func TestChaos_ConfigHugePageSize(t *testing.T) {
	cfg, err := parseHospitableConfig(connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": "test"},
		SourceConfig: map[string]interface{}{"page_size": float64(999999)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PageSize != 999999 {
		t.Errorf("huge page size should be accepted: got %d", cfg.PageSize)
	}
}

func TestChaos_ConfigZeroLookback(t *testing.T) {
	cfg, err := parseHospitableConfig(connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": "test"},
		SourceConfig: map[string]interface{}{"initial_lookback_days": float64(0)},
	})
	if err != nil {
		t.Fatalf("zero lookback should be allowed: %v", err)
	}
	if cfg.InitialLookbackDays != 0 {
		t.Errorf("expected 0, got %d", cfg.InitialLookbackDays)
	}
}

// --- Chaos: Sync with nil client (before Connect) ---

func TestChaos_SyncBeforeConnect(t *testing.T) {
	c := New("hospitable")
	// Sync without calling Connect first — should return error, not panic
	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("CHAOS: Sync before Connect should return error")
	}
	if err != nil && !containsStr(err.Error(), "not connected") {
		t.Errorf("CHAOS: error should mention not connected: %v", err)
	}
}

// --- Chaos: Close then Sync ---

func TestChaos_SyncAfterClose(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
	}))
	defer srv.Close()

	c := New("hospitable")
	config := connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": "token"},
		SourceConfig: map[string]interface{}{"base_url": srv.URL},
	}
	c.Connect(context.Background(), config)
	c.Close()

	// Should return error, not panic
	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("CHAOS: Sync after Close should return error")
	}
	if err != nil && !containsStr(err.Error(), "not connected") {
		t.Errorf("CHAOS: error should mention not connected: %v", err)
	}
}

// --- Chaos: API returns data with wildly wrong types via JSON ---

func TestChaos_APIReturnsWrongTypes(t *testing.T) {
	// Server returns properties where fields have wrong types
	wrongTypeResponses := map[string]string{
		"string_as_number": `{"data":[{"id":123,"name":456}],"total":1}`,
		"array_as_string":  `{"data":"not-an-array","total":1}`,
		"nested_null":      `{"data":[{"id":"p1","name":null,"address":null,"amenities":null}],"total":1}`,
		"total_as_string":  `{"data":[],"total":"zero"}`,
		"extra_nesting":    `{"data":[{"id":"p1","name":"House","address":{"street":{"nested":"too deep"}}}],"total":1}`,
	}

	for name, body := range wrongTypeResponses {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(body))
			}))
			defer srv.Close()

			client := NewClient(srv.URL, "token", 10)
			// Must not panic
			props, err := client.ListProperties(context.Background(), time.Time{})
			t.Logf("%s: props=%d, err=%v", name, len(props), err)
		})
	}
}

// --- Chaos: Large number of reservations for message sync ---

func TestChaos_ManyReservationsForMessageSync(t *testing.T) {
	// 500 reservations, each needing a message fetch
	var msgFetches atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/properties":
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
		case r.URL.Path == "/reservations":
			var reservations []Reservation
			for i := 0; i < 500; i++ {
				reservations = append(reservations, Reservation{
					ID:         fmt.Sprintf("r%d", i),
					PropertyID: "p1",
					GuestName:  fmt.Sprintf("Guest %d", i),
					CheckIn:    "2026-04-15",
					CheckOut:   "2026-04-18",
					BookedAt:   time.Now(),
				})
			}
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{Data: reservations, Total: 500})
		case containsStr(r.URL.Path, "/messages"):
			msgFetches.Add(1)
			json.NewEncoder(w).Encode(PaginatedResponse[Message]{Data: []Message{}, Total: 0})
		case r.URL.Path == "/reviews":
			json.NewEncoder(w).Encode(PaginatedResponse[Review]{Data: []Review{}, Total: 0})
		default:
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
		}
	}))
	defer srv.Close()

	c := New("hospitable")
	c.config = HospitableConfig{
		AccessToken:         "token",
		BaseURL:             srv.URL,
		PageSize:            1000,
		SyncProperties:      true,
		SyncReservations:    true,
		SyncMessages:        true,
		SyncReviews:         true,
		TierProperties:      "light",
		TierReservations:    "standard",
		TierMessages:        "full",
		TierReviews:         "full",
		InitialLookbackDays: 90,
	}
	c.client = NewClient(srv.URL, "token", 1000)
	c.health = connector.HealthHealthy

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Should have 500 reservations
	resCount := 0
	for _, a := range artifacts {
		if a.ContentType == "reservation/str-booking" {
			resCount++
		}
	}
	if resCount != 500 {
		t.Errorf("CHAOS: expected 500 reservation artifacts, got %d", resCount)
	}

	// Messages should have been fetched for each reservation
	fetches := msgFetches.Load()
	if fetches < 500 {
		t.Errorf("CHAOS: expected at least 500 message fetches, got %d", fetches)
	}
}

// --- Chaos: Property name cache consistency ---

func TestChaos_PropertyNameCacheWithDuplicateIDs(t *testing.T) {
	// Two properties returned with the same ID but different names
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case containsStr(r.URL.Path, "/properties"):
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{
				Data: []Property{
					{ID: "p1", Name: "Name Version 1", UpdatedAt: time.Now()},
					{ID: "p1", Name: "Name Version 2", UpdatedAt: time.Now()},
				},
				Total: 2,
			})
		case containsStr(r.URL.Path, "/reservations"):
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{
				Data: []Reservation{{
					ID: "r1", PropertyID: "p1", GuestName: "John",
					CheckIn: "2026-04-15", CheckOut: "2026-04-18", BookedAt: time.Now(),
				}},
				Total: 1,
			})
		default:
			json.NewEncoder(w).Encode(PaginatedResponse[Message]{Data: []Message{}, Total: 0})
		}
	}))
	defer srv.Close()

	c := New("hospitable")
	c.config = HospitableConfig{
		AccessToken:         "token",
		BaseURL:             srv.URL,
		PageSize:            10,
		SyncProperties:      true,
		SyncReservations:    true,
		SyncMessages:        false,
		SyncReviews:         false,
		TierProperties:      "light",
		TierReservations:    "standard",
		InitialLookbackDays: 90,
	}
	c.client = NewClient(srv.URL, "token", 10)
	c.health = connector.HealthHealthy

	artifacts, _, _ := c.Sync(context.Background(), "")

	// Reservation should use the last-seen property name (Version 2)
	for _, a := range artifacts {
		if a.ContentType == "reservation/str-booking" {
			if !containsStr(a.Title, "Name Version 2") {
				t.Errorf("CHAOS: reservation should use latest property name, got title: %s", a.Title)
			}
		}
	}
}

// --- Chaos: formatDate edge cases ---

func TestChaos_FormatDateEdgeCases(t *testing.T) {
	cases := []string{
		"",
		"not-a-date",
		"2026-13-45",    // invalid month/day
		"0000-00-00",    // zero date
		"9999-12-31",    // far future
		"2026-04-15T10:00:00Z", // RFC3339 (fallback)
		"2026-04-15T10:00:00+05:30", // RFC3339 with offset
		"04/15/2026",    // US format (unsupported)
		"15-Apr-2026",   // day-month-year
	}

	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			// Must not panic
			result := formatDate(c)
			if !utf8.ValidString(result) {
				t.Errorf("CHAOS: formatDate(%q) produced invalid UTF-8: %q", c, result)
			}
		})
	}
}

// --- Chaos: Link header parser edge cases ---

func TestChaos_ParseLinkNextEdgeCases(t *testing.T) {
	cases := map[string]string{
		"empty":               "",
		"no_angle_brackets":   `http://example.com; rel="next"`,
		"single_bracket":      `<http://example.com; rel="next"`,
		"no_semicolon":        `<http://example.com> rel="next"`,
		"multiple_rels":       `<http://example.com/1>; rel="prev", <http://example.com/2>; rel="next"`,
		"whitespace":          `  <http://example.com>  ;  rel="next"  `,
		"unquoted_rel":        `<http://example.com>; rel=next`,
		"url_with_params":     `<http://example.com?page=2&per_page=10>; rel="next"`,
		"unicode_url":         `<http://example.com/données?page=2>; rel="next"`,
		"very_long_url":       fmt.Sprintf(`<%s>; rel="next"`, "http://example.com/"+strings.Repeat("a", 10000)),
	}

	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			// Must not panic
			result := parseLinkNext(header)
			t.Logf("%s: %q -> %q", name, header, result)
		})
	}
}
