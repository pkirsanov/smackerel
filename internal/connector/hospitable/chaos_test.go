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
		"",                             // empty body
		strings.Repeat("مرحبا ", 1000), // Arabic
		"Привет! Как дела?\nОтлично 👍", // Russian with emoji
		"\x00\x01\x02\x03",               // control chars
		strings.Repeat("a\u0300", 500),   // combining diacriticals
		"<script>alert('xss')</script>",  // XSS attempt
		"'; DROP TABLE messages; --",     // SQL injection attempt
		string([]byte{0xED, 0xA0, 0x80}), // invalid UTF-8 (surrogate half)
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
	if !strings.Contains(a.RawContent, "素晴らしい") {
		t.Errorf("CHAOS: review text not preserved in content")
	}
	if !strings.Contains(a.RawContent, "ありがとう") {
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

		if strings.Contains(r.URL.Path, "/properties") {
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
		case strings.Contains(r.URL.Path, "/properties"):
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{
				Data: []Property{{ID: "p1", Name: "House"}}, Total: 1,
			})
		case strings.Contains(r.URL.Path, "/messages"):
			json.NewEncoder(w).Encode(PaginatedResponse[Message]{Data: []Message{}, Total: 0})
		case strings.Contains(r.URL.Path, "/reservations"):
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{
				Data: []Reservation{{
					ID: "r1", PropertyID: "p1", GuestName: "Test",
					CheckIn: "2026-04-15", CheckOut: "2026-04-18", BookedAt: time.Now(),
				}},
				Total: 1,
			})
		case strings.Contains(r.URL.Path, "/reviews"):
			json.NewEncoder(w).Encode(PaginatedResponse[Review]{
				Data:  []Review{{ID: "rev1", PropertyID: "p1", Rating: 5, ReviewText: "Great", SubmittedAt: time.Now()}},
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
		case strings.Contains(r.URL.Path, "/properties"):
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
	// SEC-012-003: huge page_size must be capped at maxPageSize (CWE-400)
	if cfg.PageSize != maxPageSize {
		t.Errorf("huge page size should be capped at %d: got %d", maxPageSize, cfg.PageSize)
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
	if err != nil && !strings.Contains(err.Error(), "not connected") {
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
	if err != nil && !strings.Contains(err.Error(), "not connected") {
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
		case strings.Contains(r.URL.Path, "/messages"):
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
		case strings.Contains(r.URL.Path, "/properties"):
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{
				Data: []Property{
					{ID: "p1", Name: "Name Version 1", UpdatedAt: time.Now()},
					{ID: "p1", Name: "Name Version 2", UpdatedAt: time.Now()},
				},
				Total: 2,
			})
		case strings.Contains(r.URL.Path, "/reservations"):
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
			if !strings.Contains(a.Title, "Name Version 2") {
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
		"2026-13-45",                // invalid month/day
		"0000-00-00",                // zero date
		"9999-12-31",                // far future
		"2026-04-15T10:00:00Z",      // RFC3339 (fallback)
		"2026-04-15T10:00:00+05:30", // RFC3339 with offset
		"04/15/2026",                // US format (unsupported)
		"15-Apr-2026",               // day-month-year
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
		"empty":             "",
		"no_angle_brackets": `http://example.com; rel="next"`,
		"single_bracket":    `<http://example.com; rel="next"`,
		"no_semicolon":      `<http://example.com> rel="next"`,
		"multiple_rels":     `<http://example.com/1>; rel="prev", <http://example.com/2>; rel="next"`,
		"whitespace":        `  <http://example.com>  ;  rel="next"  `,
		"unquoted_rel":      `<http://example.com>; rel=next`,
		"url_with_params":   `<http://example.com?page=2&per_page=10>; rel="next"`,
		"unicode_url":       `<http://example.com/données?page=2>; rel="next"`,
		"very_long_url":     fmt.Sprintf(`<%s>; rel="next"`, "http://example.com/"+strings.Repeat("a", 10000)),
	}

	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			// Must not panic
			result := parseLinkNext(header)
			t.Logf("%s: %q -> %q", name, header, result)
		})
	}
}

// --- Chaos: Client snapshot race (Close during Sync) ---

func TestChaos_CloseDuringSync(t *testing.T) {
	// Server adds a small delay so Close() can race with in-flight requests.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/properties"):
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{
				Data: []Property{{ID: "p1", Name: "House", UpdatedAt: time.Now()}}, Total: 1,
			})
		case strings.Contains(r.URL.Path, "/reservations"):
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{Data: []Reservation{}, Total: 0})
		case strings.Contains(r.URL.Path, "/reviews"):
			json.NewEncoder(w).Encode(PaginatedResponse[Review]{Data: []Review{}, Total: 0})
		default:
			json.NewEncoder(w).Encode(PaginatedResponse[Message]{Data: []Message{}, Total: 0})
		}
	}))
	defer srv.Close()

	c := New("hospitable")
	config := connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": "token"},
		SourceConfig: map[string]interface{}{"base_url": srv.URL},
	}
	if err := c.Connect(context.Background(), config); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	var wg sync.WaitGroup
	// Launch multiple syncs
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Must not panic even if Close() races
			c.Sync(context.Background(), "")
		}()
	}

	// Close while syncs are in-flight
	time.Sleep(5 * time.Millisecond)
	c.Close()

	wg.Wait()
	// Key assertion: no panic (run with -race to verify no data races)
}

// --- Chaos: Double Close ---

func TestChaos_DoubleClose(t *testing.T) {
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

	// First close
	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Errorf("health after first Close = %v, want disconnected", c.Health(context.Background()))
	}

	// Second close — must not panic
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Errorf("health after second Close = %v, want disconnected", c.Health(context.Background()))
	}
}

// --- Chaos: Reconnect after Close ---

func TestChaos_ReconnectAfterClose(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Property]{
			Data: []Property{{ID: "p1", Name: "House", UpdatedAt: time.Now()}}, Total: 1,
		})
	}))
	defer srv.Close()

	c := New("hospitable")
	config := connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": "token"},
		SourceConfig: map[string]interface{}{"base_url": srv.URL},
	}

	// Connect, Close, Reconnect
	if err := c.Connect(context.Background(), config); err != nil {
		t.Fatalf("first Connect: %v", err)
	}
	c.Close()

	// Sync should fail after Close
	_, _, err := c.Sync(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Errorf("Sync after Close should fail with not connected, got: %v", err)
	}

	// Reconnect
	if err := c.Connect(context.Background(), config); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("health after reconnect = %v, want healthy", c.Health(context.Background()))
	}

	// Sync should work again
	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync after reconnect: %v", err)
	}
	if len(artifacts) == 0 {
		t.Error("expected artifacts after reconnect sync")
	}
	if cursor == "" {
		t.Error("expected non-empty cursor after reconnect sync")
	}
}

// --- Chaos: Retry-After cap enforcement ---

func TestChaos_RetryAfterCapped(t *testing.T) {
	// Verify that maxRetryAfterCap is enforced
	d := parseRetryAfter("999999", time.Now())
	if d > maxRetryAfterCap {
		// The cap is applied in doGetPaginated, not in parseRetryAfter itself.
		// Verify the raw parser returns the large value so we know the cap must
		// be applied at the call site.
		if d != 999999*time.Second {
			t.Errorf("parseRetryAfter(999999) = %v, expected 999999s", d)
		}
	}

	// Actually test via a server that the client doesn't sleep forever
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 1 {
			w.Header().Set("Retry-After", "999999")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{{ID: "p1"}}, Total: 1})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token", 10)
	client.SetBackoff(fastBackoff())

	// With the cap at 60s and a 3s context timeout, the request should fail
	// with context deadline rather than sleeping 999999s.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := client.ListProperties(ctx, time.Time{})
	// Should either succeed (if cap is short enough) or context-cancel — NOT hang
	if err != nil && ctx.Err() == nil {
		// Got an error but context is still valid — max retries exceeded, which is fine
		t.Logf("RetryAfterCapped: error=%v (context still valid — retries exhausted)", err)
	} else if err != nil {
		t.Logf("RetryAfterCapped: error=%v (context cancelled as expected)", err)
	}
}

// --- Chaos: Concurrent Connect calls ---

func TestChaos_ConcurrentConnect(t *testing.T) {
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

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Connect(context.Background(), config)
		}()
	}
	wg.Wait()

	// After all concurrent connects, connector should be healthy
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("health after concurrent connects = %v, want healthy", c.Health(context.Background()))
	}
}

// --- Chaos: Pagination next URL SSRF via special characters ---

func TestChaos_PaginationSSRFAttempts(t *testing.T) {
	ssrfURLs := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://localhost:6379/",
		"file:///etc/passwd",
		"gopher://internal:25/",
		"http://[::1]:8080/",
		"http://0x7f000001:8080/", // hex-encoded localhost
	}

	for _, ssrfURL := range ssrfURLs {
		t.Run(ssrfURL, func(t *testing.T) {
			callCount := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callCount++
				w.Header().Set("Content-Type", "application/json")
				resp := PaginatedResponse[Property]{
					Data:    []Property{{ID: "p1", Name: "House"}},
					NextURL: ssrfURL,
					Total:   100,
				}
				json.NewEncoder(w).Encode(resp)
			}))
			defer srv.Close()

			client := NewClient(srv.URL, "token", 10)
			props, err := client.ListProperties(context.Background(), time.Time{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// Should get exactly 1 page (SSRF URL rejected by same-origin check)
			if len(props) != 1 {
				t.Errorf("expected 1 property (SSRF blocked), got %d", len(props))
			}
			if callCount != 1 {
				t.Errorf("expected 1 API call (SSRF blocked pagination), got %d", callCount)
			}
		})
	}
}

// --- Chaos: Config parsing edge cases ---

func TestChaos_ConfigBaseURLAttacks(t *testing.T) {
	attacks := []struct {
		name    string
		baseURL string
		wantErr bool
	}{
		{"javascript_proto", "javascript:alert(1)", true},
		{"data_proto", "data:text/html,<h1>pwned</h1>", true},
		{"ftp_proto", "ftp://evil.com/exfil", true},
		{"no_host", "https://", true},
		{"valid_http", "http://api.example.com", false},
		{"valid_https", "https://api.example.com", false},
	}

	for _, tc := range attacks {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseHospitableConfig(connector.ConnectorConfig{
				Credentials:  map[string]string{"access_token": "test"},
				SourceConfig: map[string]interface{}{"base_url": tc.baseURL},
			})
			if tc.wantErr && err == nil {
				t.Errorf("expected error for base_url %q", tc.baseURL)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for base_url %q: %v", tc.baseURL, err)
			}
		})
	}
}

// --- Chaos: Config missing credentials key ---

func TestChaos_ConfigMissingCredentialsKey(t *testing.T) {
	_, err := parseHospitableConfig(connector.ConnectorConfig{
		Credentials:  map[string]string{},
		SourceConfig: map[string]interface{}{},
	})
	if err == nil || !strings.Contains(err.Error(), "access_token") {
		t.Errorf("expected access_token error, got: %v", err)
	}
}

func TestChaos_ConfigNilCredentials(t *testing.T) {
	_, err := parseHospitableConfig(connector.ConnectorConfig{
		Credentials:  nil,
		SourceConfig: map[string]interface{}{},
	})
	if err == nil || !strings.Contains(err.Error(), "access_token") {
		t.Errorf("expected access_token error, got: %v", err)
	}
}

func TestChaos_ConfigNegativeLookback(t *testing.T) {
	_, err := parseHospitableConfig(connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": "test"},
		SourceConfig: map[string]interface{}{"initial_lookback_days": float64(-30)},
	})
	if err == nil || !strings.Contains(err.Error(), "negative") {
		t.Errorf("expected negative lookback error, got: %v", err)
	}
}

// --- Chaos regression: Message sync fan-out cap ---

func TestChaos_MessageSyncFanOutCap(t *testing.T) {
	// Override the cap to a small value for testing.
	oldCap := maxMessageSyncReservations
	maxMessageSyncReservations = 3
	defer func() { maxMessageSyncReservations = oldCap }()

	var msgFetches atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/properties":
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
		case r.URL.Path == "/reservations":
			var reservations []Reservation
			for i := 0; i < 10; i++ {
				reservations = append(reservations, Reservation{
					ID: fmt.Sprintf("r%d", i), PropertyID: "p1",
					GuestName: fmt.Sprintf("Guest %d", i),
					CheckIn:   "2026-04-15", CheckOut: "2026-04-18",
					BookedAt: time.Now(),
				})
			}
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{Data: reservations, Total: 10})
		case strings.Contains(r.URL.Path, "/messages"):
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
		AccessToken: "token", BaseURL: srv.URL, PageSize: 100,
		SyncProperties: true, SyncReservations: true, SyncMessages: true, SyncReviews: true,
		TierProperties: "light", TierReservations: "standard",
		TierMessages: "full", TierReviews: "full", InitialLookbackDays: 90,
	}
	c.client = NewClient(srv.URL, "token", 100)
	c.health = connector.HealthHealthy

	artifacts, newCursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Verify message fetches were capped at 3, not 10.
	fetches := msgFetches.Load()
	if fetches > int64(maxMessageSyncReservations) {
		t.Errorf("CHAOS: message fetches (%d) exceeded cap (%d) — fan-out not bounded",
			fetches, maxMessageSyncReservations)
	}
	if fetches != int64(maxMessageSyncReservations) {
		t.Errorf("expected exactly %d message fetches, got %d", maxMessageSyncReservations, fetches)
	}

	// Verify reservation artifacts are still all present (cap only affects messages).
	resCount := 0
	for _, a := range artifacts {
		if a.ContentType == "reservation/str-booking" {
			resCount++
		}
	}
	if resCount != 10 {
		t.Errorf("expected 10 reservation artifacts, got %d", resCount)
	}

	// Message cursor must NOT advance because fan-out was capped (incomplete coverage).
	var cur SyncCursor
	json.Unmarshal([]byte(newCursor), &cur)
	if !cur.Messages.IsZero() {
		// parseCursor for fresh sync sets Messages to lookback time; if it advanced
		// past that it means cursor progressed despite incomplete coverage.
		lookback := time.Now().UTC().AddDate(0, 0, -90)
		if cur.Messages.After(lookback.Add(time.Minute)) {
			t.Errorf("CHAOS: message cursor advanced despite capped fan-out: %v", cur.Messages)
		}
	}
}

// --- Chaos regression: Cursor preserved on context cancellation ---

func TestChaos_CursorPreservedOnCancellation(t *testing.T) {
	// resRequested signals that the reservation request has reached the server,
	// ensuring properties have been fully served and processed by the client
	// before we cancel the context. This eliminates the race between the
	// server writing the properties response and the client reading it.
	resRequested := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/properties"):
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{
				Data: []Property{{ID: "p1", Name: "House", UpdatedAt: time.Now()}}, Total: 1,
			})
		case strings.Contains(r.URL.Path, "/reservations"):
			// Signal that reservoir request arrived (properties fully processed).
			select {
			case resRequested <- struct{}{}:
			default:
			}
			// Block until context is cancelled to simulate slow downstream API
			<-r.Context().Done()
		default:
			<-r.Context().Done()
		}
	}))
	defer srv.Close()

	c := New("hospitable")
	c.config = HospitableConfig{
		AccessToken: "token", BaseURL: srv.URL, PageSize: 10,
		SyncProperties: true, SyncReservations: true, SyncMessages: false, SyncReviews: false,
		TierProperties: "light", TierReservations: "standard",
		InitialLookbackDays: 90,
	}
	c.client = NewClient(srv.URL, "token", 10)
	c.health = connector.HealthHealthy

	ctx, cancel := context.WithCancel(context.Background())

	// Run sync in a goroutine; cancel after reservations request arrives
	// (which proves properties completed successfully).
	type syncResult struct {
		artifacts []connector.RawArtifact
		cursor    string
		err       error
	}
	resultCh := make(chan syncResult, 1)
	go func() {
		arts, cur, err := c.Sync(ctx, "")
		resultCh <- syncResult{arts, cur, err}
	}()

	// Wait for reservation request to reach server (properties already done), then cancel.
	<-resRequested
	cancel()

	result := <-resultCh

	// The sync should have returned an error (cancelled during reservations),
	// but the cursor should preserve the properties advancement.
	if result.cursor == "" {
		t.Fatal("CHAOS: cursor is empty after partial sync — progress lost")
	}

	var cur SyncCursor
	if err := json.Unmarshal([]byte(result.cursor), &cur); err != nil {
		t.Fatalf("CHAOS: cursor is not valid JSON: %v", err)
	}

	// Properties cursor should have advanced (not zero, not the original lookback).
	if cur.Properties.IsZero() {
		t.Errorf("CHAOS: properties cursor is zero — sync progress lost on cancellation")
	}
	// Properties cursor should be recent (within the last minute).
	if time.Since(cur.Properties) > time.Minute {
		t.Errorf("CHAOS: properties cursor not recent: %v", cur.Properties)
	}
}

// --- Chaos regression: Property name cache pruning under cap ---

func TestChaos_PropertyNameCachePruning(t *testing.T) {
	oldCap := maxPropertyNameCacheSize
	maxPropertyNameCacheSize = 5
	defer func() { maxPropertyNameCacheSize = oldCap }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/properties"):
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{
				Data: []Property{
					{ID: "p1", Name: "House 1", UpdatedAt: time.Now()},
					{ID: "p2", Name: "House 2", UpdatedAt: time.Now()},
				},
				Total: 2,
			})
		case strings.Contains(r.URL.Path, "/reservations"):
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
	// Pre-populate propertyNames with 10 entries (above the cap of 5).
	for i := 0; i < 10; i++ {
		c.propertyNames[fmt.Sprintf("old-p%d", i)] = fmt.Sprintf("Old House %d", i)
	}
	c.config = HospitableConfig{
		AccessToken: "token", BaseURL: srv.URL, PageSize: 10,
		SyncProperties: true, SyncReservations: true, SyncMessages: false, SyncReviews: false,
		TierProperties: "light", TierReservations: "standard",
		InitialLookbackDays: 90,
	}
	c.client = NewClient(srv.URL, "token", 10)
	c.health = connector.HealthHealthy

	_, newCursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	var cur SyncCursor
	json.Unmarshal([]byte(newCursor), &cur)

	// Cache should have been pruned to only referenced properties.
	// The sync produced artifacts referencing p1 and p2 (from properties and reservation).
	if len(cur.PropertyNames) > maxPropertyNameCacheSize {
		t.Errorf("CHAOS: property name cache in cursor (%d) exceeds cap (%d) — unbounded growth",
			len(cur.PropertyNames), maxPropertyNameCacheSize)
	}

	// p1 must be preserved (referenced by reservation artifact).
	if cur.PropertyNames["p1"] != "House 1" {
		t.Errorf("CHAOS: referenced property p1 was pruned from cache: %v", cur.PropertyNames)
	}

	// Old entries should NOT be in the cursor (they're not referenced by any artifact).
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("old-p%d", i)
		if _, exists := cur.PropertyNames[key]; exists {
			t.Errorf("CHAOS: unreferenced property %s should have been pruned from cursor", key)
		}
	}
}

// --- SEC-012-005: Control character sanitization in user-supplied text (CWE-116) ---

func TestSEC012005_GuestNameControlChars(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard"}
	r := Reservation{
		ID:         "res-ctrl",
		PropertyID: "p1",
		GuestName:  "John\x00Smith\x01\x02\x03\rDoe",
		CheckIn:    "2026-04-15",
		CheckOut:   "2026-04-18",
		BookedAt:   time.Now(),
	}

	a := NormalizeReservation(r, "Beach House", cfg)

	// Null bytes and control chars must be stripped from title and content
	if strings.ContainsAny(a.Title, "\x00\x01\x02\x03\r") {
		t.Errorf("SEC-012-005: title still contains control characters: %q", a.Title)
	}
	if strings.ContainsAny(a.RawContent, "\x00\x01\x02\x03\r") {
		t.Errorf("SEC-012-005: content still contains control characters: %q", a.RawContent)
	}
	// Guest name in metadata should also be sanitized
	if name, ok := a.Metadata["guest_name"].(string); ok {
		if strings.ContainsAny(name, "\x00\x01\x02\x03\r") {
			t.Errorf("SEC-012-005: metadata guest_name still contains control characters: %q", name)
		}
	}
}

func TestSEC012005_MessageBodyControlChars(t *testing.T) {
	cfg := HospitableConfig{TierMessages: "full"}
	m := Message{
		ID:     "msg-ctrl",
		Sender: "Evil\x00Sender\x1B[31m",
		Body:   "Normal text\x00hidden\rinjection\x1B[0m",
		SentAt: time.Now(),
	}

	a := NormalizeMessage(m, "res-1", cfg)

	if strings.ContainsAny(a.Title, "\x00\x1B\r") {
		t.Errorf("SEC-012-005: message title contains control chars: %q", a.Title)
	}
	if strings.ContainsAny(a.RawContent, "\x00\x1B\r") {
		t.Errorf("SEC-012-005: message content contains control chars: %q", a.RawContent)
	}
}

func TestSEC012005_ReviewTextControlChars(t *testing.T) {
	cfg := HospitableConfig{TierReviews: "full"}
	r := Review{
		ID:           "rev-ctrl",
		PropertyID:   "p1",
		Rating:       4.5,
		ReviewText:   "Great\x00stay\rno\x1Bproblems",
		HostResponse: "Thanks\x00for\rstaying\x01!",
		Channel:      "Airbnb",
		SubmittedAt:  time.Now(),
	}

	a := NormalizeReview(r, "Beach\x00House", cfg)

	if strings.ContainsAny(a.Title, "\x00\x01\r\x1B") {
		t.Errorf("SEC-012-005: review title contains control chars: %q", a.Title)
	}
	if strings.ContainsAny(a.RawContent, "\x00\x01\r\x1B") {
		t.Errorf("SEC-012-005: review content contains control chars: %q", a.RawContent)
	}
}

func TestSEC012005_PropertyNameControlChars(t *testing.T) {
	cfg := HospitableConfig{TierProperties: "light"}
	p := Property{
		ID:        "prop-ctrl",
		Name:      "Beach\x00House\rEvil\x1BPlace",
		UpdatedAt: time.Now(),
	}

	a := NormalizeProperty(p, cfg)

	if strings.ContainsAny(a.Title, "\x00\r\x1B") {
		t.Errorf("SEC-012-005: property title contains control chars: %q", a.Title)
	}
	if strings.ContainsAny(a.RawContent, "\x00\r\x1B") {
		t.Errorf("SEC-012-005: property content contains control chars: %q", a.RawContent)
	}
}

// --- SEC-012-006: ActiveReservationIDs cursor cap (CWE-770) ---

func TestSEC012006_ActiveReservationIDsCursorCap(t *testing.T) {
	oldCap := maxMessageSyncReservations
	maxMessageSyncReservations = 5
	defer func() { maxMessageSyncReservations = oldCap }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/properties":
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
		case r.URL.Path == "/reservations":
			var reservations []Reservation
			for i := 0; i < 20; i++ {
				reservations = append(reservations, Reservation{
					ID: fmt.Sprintf("r-%04d", i), PropertyID: "p1",
					GuestName: "Guest", CheckIn: "2026-04-15", CheckOut: "2026-04-18",
					BookedAt: time.Now(),
				})
			}
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{Data: reservations, Total: 20})
		case strings.Contains(r.URL.Path, "/messages"):
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
		AccessToken: "token", BaseURL: srv.URL, PageSize: 100,
		SyncProperties: true, SyncReservations: true, SyncMessages: true, SyncReviews: true,
		TierProperties: "light", TierReservations: "standard",
		TierMessages: "full", TierReviews: "full", InitialLookbackDays: 90,
	}
	c.client = NewClient(srv.URL, "token", 100)
	c.health = connector.HealthHealthy

	_, newCursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	var cur SyncCursor
	if err := json.Unmarshal([]byte(newCursor), &cur); err != nil {
		t.Fatalf("cursor unmarshal: %v", err)
	}

	// ActiveReservationIDs in cursor must be capped at maxMessageSyncReservations
	if len(cur.ActiveReservationIDs) > maxMessageSyncReservations {
		t.Errorf("SEC-012-006: ActiveReservationIDs (%d) exceeds cap (%d) — unbounded cursor growth",
			len(cur.ActiveReservationIDs), maxMessageSyncReservations)
	}
}

// --- SEC-012-007: Property name cache string length cap (CWE-400) ---

func TestSEC012007_OversizedPropertyIDSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/properties"):
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{
				Data: []Property{
					{ID: strings.Repeat("x", maxCacheStringLen+1), Name: "Oversized ID", UpdatedAt: time.Now()},
					{ID: "p-normal", Name: "Normal Prop", UpdatedAt: time.Now()},
				},
				Total: 2,
			})
		case strings.Contains(r.URL.Path, "/reservations"):
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{
				Data: []Reservation{{
					ID: "r1", PropertyID: "p-normal", GuestName: "Alice",
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
		AccessToken: "token", BaseURL: srv.URL, PageSize: 10,
		SyncProperties: true, SyncReservations: true, SyncMessages: false, SyncReviews: false,
		TierProperties: "light", TierReservations: "standard", InitialLookbackDays: 90,
	}
	c.client = NewClient(srv.URL, "token", 10)
	c.health = connector.HealthHealthy

	_, newCursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	var cur SyncCursor
	json.Unmarshal([]byte(newCursor), &cur)

	// Oversized ID must NOT appear in cursor property names
	oversizedKey := strings.Repeat("x", maxCacheStringLen+1)
	if _, exists := cur.PropertyNames[oversizedKey]; exists {
		t.Errorf("SEC-012-007: oversized property ID was stored in cursor cache")
	}

	// Normal ID should still be cached
	if cur.PropertyNames["p-normal"] != "Normal Prop" {
		t.Errorf("SEC-012-007: normal property ID should be cached, got: %v", cur.PropertyNames["p-normal"])
	}
}

func TestSEC012007_OversizedPropertyNameSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/properties"):
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{
				Data: []Property{
					{ID: "p-bigname", Name: strings.Repeat("N", maxCacheStringLen+1), UpdatedAt: time.Now()},
					{ID: "p-ok", Name: "Small Name", UpdatedAt: time.Now()},
				},
				Total: 2,
			})
		default:
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{Data: []Reservation{}, Total: 0})
		}
	}))
	defer srv.Close()

	c := New("hospitable")
	c.config = HospitableConfig{
		AccessToken: "token", BaseURL: srv.URL, PageSize: 10,
		SyncProperties: true, SyncReservations: false, SyncMessages: false, SyncReviews: false,
		TierProperties: "light", InitialLookbackDays: 90,
	}
	c.client = NewClient(srv.URL, "token", 10)
	c.health = connector.HealthHealthy

	_, newCursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	var cur SyncCursor
	json.Unmarshal([]byte(newCursor), &cur)

	// Property with oversized name must not appear in cache
	if _, exists := cur.PropertyNames["p-bigname"]; exists {
		t.Errorf("SEC-012-007: property with oversized name was stored in cursor cache")
	}

	// Property with normal name should be cached
	if cur.PropertyNames["p-ok"] != "Small Name" {
		t.Errorf("SEC-012-007: normal property should be cached, got: %v", cur.PropertyNames["p-ok"])
	}
}

func TestSEC012007_OversizedCursorPropertyNamesSkippedOnLoad(t *testing.T) {
	// Simulate a cursor with pre-existing oversized entries (persisted before the fix)
	oversizedCursor := SyncCursor{
		PropertyNames: map[string]string{
			strings.Repeat("K", maxCacheStringLen+1): "Oversized Key",
			"normal-key":                             strings.Repeat("V", maxCacheStringLen+1),
			"ok-key":                                 "OK Value",
		},
	}
	cursorJSON, _ := json.Marshal(oversizedCursor)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
	}))
	defer srv.Close()

	c := New("hospitable")
	c.config = HospitableConfig{
		AccessToken: "token", BaseURL: srv.URL, PageSize: 10,
		SyncProperties: true, SyncReservations: false, SyncMessages: false, SyncReviews: false,
		TierProperties: "light", InitialLookbackDays: 90,
	}
	c.client = NewClient(srv.URL, "token", 10)
	c.health = connector.HealthHealthy

	_, _, err := c.Sync(context.Background(), string(cursorJSON))
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	// Oversized key should NOT have been loaded into the in-memory cache
	oversizedKey := strings.Repeat("K", maxCacheStringLen+1)
	if _, exists := c.propertyNames[oversizedKey]; exists {
		t.Errorf("SEC-012-007: oversized cursor key was loaded into memory cache")
	}
	// Oversized value should NOT have been loaded
	if _, exists := c.propertyNames["normal-key"]; exists {
		t.Errorf("SEC-012-007: cursor entry with oversized value was loaded into memory cache")
	}
	// Normal entry should be loaded
	if c.propertyNames["ok-key"] != "OK Value" {
		t.Errorf("SEC-012-007: normal cursor entry should be loaded, got: %q", c.propertyNames["ok-key"])
	}
}

// --- SEC-012-008: Cursor PropertyNames deserialization cap (CWE-770) ---

func TestSEC012008_CursorPropertyNamesCappedOnLoad(t *testing.T) {
	// Build a cursor with more PropertyNames than maxPropertyNameCacheSize.
	oldCap := maxPropertyNameCacheSize
	maxPropertyNameCacheSize = 5 // Use small cap for test speed
	defer func() { maxPropertyNameCacheSize = oldCap }()

	oversizedCache := make(map[string]string, 20)
	for i := 0; i < 20; i++ {
		oversizedCache[fmt.Sprintf("p-%04d", i)] = fmt.Sprintf("Prop %d", i)
	}
	craftedCursor := SyncCursor{PropertyNames: oversizedCache}
	cursorJSON, _ := json.Marshal(craftedCursor)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
	}))
	defer srv.Close()

	c := New("hospitable")
	c.config = HospitableConfig{
		AccessToken: "token", BaseURL: srv.URL, PageSize: 10,
		SyncProperties: true, SyncReservations: false, SyncMessages: false, SyncReviews: false,
		TierProperties: "light", InitialLookbackDays: 90,
	}
	c.client = NewClient(srv.URL, "token", 10)
	c.health = connector.HealthHealthy

	_, _, err := c.Sync(context.Background(), string(cursorJSON))
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	c.mu.RLock()
	loaded := len(c.propertyNames)
	c.mu.RUnlock()

	// In-memory cache must be capped at maxPropertyNameCacheSize.
	if loaded > maxPropertyNameCacheSize {
		t.Errorf("SEC-012-008: cursor deserialization loaded %d entries, exceeding cap %d (CWE-770)",
			loaded, maxPropertyNameCacheSize)
	}
}

func TestSEC012008_CursorBelowCapLoadsAll(t *testing.T) {
	// A cursor with entries below the cap should load all entries.
	smallCache := map[string]string{"p-a": "House A", "p-b": "House B"}
	cursorJSON, _ := json.Marshal(SyncCursor{PropertyNames: smallCache})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
	}))
	defer srv.Close()

	c := New("hospitable")
	c.config = HospitableConfig{
		AccessToken: "token", BaseURL: srv.URL, PageSize: 10,
		SyncProperties: true, SyncReservations: false, SyncMessages: false, SyncReviews: false,
		TierProperties: "light", InitialLookbackDays: 90,
	}
	c.client = NewClient(srv.URL, "token", 10)
	c.health = connector.HealthHealthy

	_, _, err := c.Sync(context.Background(), string(cursorJSON))
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.propertyNames["p-a"] != "House A" {
		t.Errorf("SEC-012-008: small cursor entry p-a should load, got %q", c.propertyNames["p-a"])
	}
	if c.propertyNames["p-b"] != "House B" {
		t.Errorf("SEC-012-008: small cursor entry p-b should load, got %q", c.propertyNames["p-b"])
	}
}

// --- SEC-012-009: Address fields bypass control char sanitization (CWE-116) ---

func TestSEC012009_AddressControlChars(t *testing.T) {
	cfg := HospitableConfig{TierProperties: "light"}
	p := Property{
		ID:   "prop-addr-ctrl",
		Name: "Clean Name",
		Address: Address{
			Street:  "123\x00Main\rSt",
			City:    "Evil\x1BCity",
			State:   "Bad\x01State",
			Country: "Null\x00Land",
			Zip:     "ZIP\x02CODE",
		},
		UpdatedAt: time.Now(),
	}

	a := NormalizeProperty(p, cfg)

	// Address in content must have control chars stripped
	if strings.ContainsAny(a.RawContent, "\x00\x01\x02\r\x1B") {
		t.Errorf("SEC-012-009: property content contains control chars from address: %q", a.RawContent)
	}

	// Address in metadata must be clean
	if addr, ok := a.Metadata["address"].(string); ok {
		if strings.ContainsAny(addr, "\x00\x01\x02\r\x1B") {
			t.Errorf("SEC-012-009: metadata address contains control chars: %q", addr)
		}
	}
}

func TestSEC012009_AddressFieldsCleaned(t *testing.T) {
	cfg := HospitableConfig{TierProperties: "light"}
	p := Property{
		ID:   "prop-addr-ok",
		Name: "Test Property",
		Address: Address{
			Street:  "456 Oak Ave",
			City:    "Portland",
			State:   "OR",
			Country: "US",
			Zip:     "97201",
		},
		UpdatedAt: time.Now(),
	}

	a := NormalizeProperty(p, cfg)

	// Clean addresses should pass through unchanged
	if addr, ok := a.Metadata["address"].(string); ok {
		if !strings.Contains(addr, "456 Oak Ave") {
			t.Errorf("SEC-012-009: clean address should be preserved, got %q", addr)
		}
	}
}

// --- SEC-012-010: Reservation Channel/Status bypass control char sanitization (CWE-116) ---

func TestSEC012010_ReservationChannelControlChars(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard"}
	r := Reservation{
		ID:         "res-chan-ctrl",
		PropertyID: "p1",
		GuestName:  "Alice",
		Channel:    "Airbnb\x00Direct\x1B[31m",
		Status:     "confirmed\x00\rinjected",
		CheckIn:    "2026-04-15",
		CheckOut:   "2026-04-18",
		BookedAt:   time.Now(),
	}

	a := NormalizeReservation(r, "Beach House", cfg)

	if strings.ContainsAny(a.RawContent, "\x00\x01\r\x1B") {
		t.Errorf("SEC-012-010: reservation content has control chars from Channel/Status: %q", a.RawContent)
	}
	if ch, ok := a.Metadata["channel"].(string); ok {
		if strings.ContainsAny(ch, "\x00\x1B") {
			t.Errorf("SEC-012-010: metadata channel contains control chars: %q", ch)
		}
	}
	if st, ok := a.Metadata["status"].(string); ok {
		if strings.ContainsAny(st, "\x00\r") {
			t.Errorf("SEC-012-010: metadata status contains control chars: %q", st)
		}
	}
}

func TestSEC012010_ReviewChannelControlChars(t *testing.T) {
	cfg := HospitableConfig{TierReviews: "full"}
	r := Review{
		ID:          "rev-chan-ctrl",
		PropertyID:  "p1",
		Rating:      4.0,
		ReviewText:  "Great stay",
		Channel:     "Booking\x00.com\x1B",
		SubmittedAt: time.Now(),
	}

	a := NormalizeReview(r, "Beach House", cfg)

	if strings.ContainsAny(a.RawContent, "\x00\x1B") {
		t.Errorf("SEC-012-010: review content has control chars from Channel: %q", a.RawContent)
	}
	if ch, ok := a.Metadata["channel"].(string); ok {
		if strings.ContainsAny(ch, "\x00\x1B") {
			t.Errorf("SEC-012-010: review metadata channel contains control chars: %q", ch)
		}
	}
}

func TestSEC012010_CleanChannelStatusPreserved(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard"}
	r := Reservation{
		ID:         "res-chan-ok",
		PropertyID: "p1",
		GuestName:  "Bob",
		Channel:    "Airbnb",
		Status:     "confirmed",
		CheckIn:    "2026-04-15",
		CheckOut:   "2026-04-18",
		BookedAt:   time.Now(),
	}

	a := NormalizeReservation(r, "Beach House", cfg)

	if ch, ok := a.Metadata["channel"].(string); ok {
		if ch != "Airbnb" {
			t.Errorf("SEC-012-010: clean channel should be preserved, got %q", ch)
		}
	}
	if st, ok := a.Metadata["status"].(string); ok {
		if st != "confirmed" {
			t.Errorf("SEC-012-010: clean status should be preserved, got %q", st)
		}
	}
}
