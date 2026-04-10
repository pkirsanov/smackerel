package hospitable

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// --- Client tests ---

func TestClientAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token-123", 10)
	_, err := client.ListProperties(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer test-token-123" {
		t.Errorf("got auth header %q, want %q", gotAuth, "Bearer test-token-123")
	}
}

func TestClientValidateSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "valid-token", 10)
	err := client.Validate(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClientValidateUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "unauthorized"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "bad-token", 10)
	err := client.Validate(context.Background())
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if got := err.Error(); !contains(got, "unauthorized") {
		t.Errorf("error should contain 'unauthorized', got: %s", got)
	}
}

func TestClientValidateForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": "forbidden"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "limited-token", 10)
	err := client.Validate(context.Background())
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if got := err.Error(); !contains(got, "forbidden") {
		t.Errorf("error should contain 'forbidden', got: %s", got)
	}
}

func TestClientPaginatesProperties(t *testing.T) {
	page := 0
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		var resp PaginatedResponse[Property]
		switch page {
		case 1:
			resp = PaginatedResponse[Property]{
				Data:  []Property{{ID: "p1", Name: "House 1"}, {ID: "p2", Name: "House 2"}},
				Total: 5,
			}
			w.Header().Set("Link", fmt.Sprintf(`<%s/properties?per_page=2&page=2>; rel="next"`, srvURL))
		case 2:
			resp = PaginatedResponse[Property]{
				Data:  []Property{{ID: "p3", Name: "House 3"}, {ID: "p4", Name: "House 4"}},
				Total: 5,
			}
			// Use NextURL in body for page 3
			resp.NextURL = fmt.Sprintf("%s/properties?per_page=2&page=3", srvURL)
		case 3:
			resp = PaginatedResponse[Property]{
				Data:  []Property{{ID: "p5", Name: "House 5"}},
				Total: 5,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	srvURL = srv.URL

	client := NewClient(srv.URL, "token", 2)
	props, err := client.ListProperties(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(props) != 5 {
		t.Errorf("got %d properties, want 5", len(props))
	}
}

func TestClientRetryOn429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Review]{Data: []Review{{ID: "r1", Rating: 5}}, Total: 1})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token", 10)
	client.SetBackoff(fastBackoff())
	reviews, err := client.ListReviews(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}
	if len(reviews) != 1 {
		t.Errorf("got %d reviews, want 1", len(reviews))
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestClientMaxRetriesOn429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token", 10)
	client.SetBackoff(fastBackoff())
	_, err := client.ListReviews(context.Background(), time.Time{})
	if err == nil {
		t.Fatal("expected error after max retries")
	}
	if !contains(err.Error(), "rate limited") {
		t.Errorf("error should mention rate limiting: %v", err)
	}
}

func TestDefaultClientMaxRetries3(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	// Use production client (no SetBackoff override) — should fail after 3 retries per R-009
	client := NewClient(srv.URL, "token", 10)
	// Override delays to be tiny for fast test, but keep MaxRetries from constructor
	client.backoff.BaseDelay = 1 * time.Millisecond
	client.backoff.MaxDelay = 10 * time.Millisecond
	_, err := client.ListProperties(context.Background(), time.Time{})
	if err == nil {
		t.Fatal("expected error after max retries")
	}
	// Should have attempted exactly 1 initial + 3 retries = 4 total calls,
	// but backoff.Next() is called first so attempts = MaxRetries + 1 won't happen;
	// it stops at exactly MaxRetries attempts through the retry loop
	if attempts != 4 {
		t.Errorf("expected 4 attempts (1 initial + 3 retries), got %d", attempts)
	}
}

func TestClientRetryOnServerError(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
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
		t.Fatalf("expected success after retry: %v", err)
	}
	if len(props) != 1 {
		t.Errorf("got %d properties, want 1", len(props))
	}
}

func TestClientURLConstruction(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{Data: []Reservation{}, Total: 0})
	}))
	defer srv.Close()

	since := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	client := NewClient(srv.URL, "token", 50)
	_, err := client.ListReservations(context.Background(), since)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contains(gotURL, "per_page=50") {
		t.Errorf("URL should contain per_page=50, got: %s", gotURL)
	}
	if !contains(gotURL, "updated_since=") {
		t.Errorf("URL should contain updated_since, got: %s", gotURL)
	}
}

// --- Connector tests ---

func TestConnectorID(t *testing.T) {
	c := New("hospitable")
	if c.ID() != "hospitable" {
		t.Errorf("got ID %q, want %q", c.ID(), "hospitable")
	}
}

func TestConnectValidConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
	}))
	defer srv.Close()

	c := New("hospitable")
	config := connector.ConnectorConfig{
		AuthType:    "token",
		Credentials: map[string]string{"access_token": "valid-pat"},
		SourceConfig: map[string]interface{}{
			"base_url": srv.URL,
		},
	}

	err := c.Connect(context.Background(), config)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("expected healthy, got %v", c.Health(context.Background()))
	}
}

func TestConnectInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := New("hospitable")
	config := connector.ConnectorConfig{
		AuthType:    "token",
		Credentials: map[string]string{"access_token": "bad-pat"},
		SourceConfig: map[string]interface{}{
			"base_url": srv.URL,
		},
	}

	err := c.Connect(context.Background(), config)
	if err == nil {
		t.Fatal("expected Connect to fail with invalid token")
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected error health, got %v", c.Health(context.Background()))
	}
}

func TestConfigValidationMissingToken(t *testing.T) {
	c := New("hospitable")
	config := connector.ConnectorConfig{
		AuthType:     "token",
		Credentials:  map[string]string{},
		SourceConfig: map[string]interface{}{},
	}

	err := c.Connect(context.Background(), config)
	if err == nil {
		t.Fatal("expected error for missing access_token")
	}
	if !contains(err.Error(), "access_token") {
		t.Errorf("error should mention access_token: %v", err)
	}
}

func TestConfigValidationNegativeLookback(t *testing.T) {
	_, err := parseHospitableConfig(connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": "test"},
		SourceConfig: map[string]interface{}{"initial_lookback_days": float64(-10)},
	})
	if err == nil {
		t.Fatal("expected error for negative lookback days")
	}
	if !contains(err.Error(), "initial_lookback_days") {
		t.Errorf("error should mention initial_lookback_days: %v", err)
	}
}

func TestConfigValidationDefaults(t *testing.T) {
	cfg, err := parseHospitableConfig(connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": "test"},
		SourceConfig: map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BaseURL != "https://api.hospitable.com" {
		t.Errorf("expected default base URL, got %s", cfg.BaseURL)
	}
	if cfg.PageSize != 100 {
		t.Errorf("expected default page size 100, got %d", cfg.PageSize)
	}
	if cfg.InitialLookbackDays != 90 {
		t.Errorf("expected default lookback 90, got %d", cfg.InitialLookbackDays)
	}
	if !cfg.SyncProperties || !cfg.SyncReservations || !cfg.SyncMessages || !cfg.SyncReviews {
		t.Error("expected all sync flags to default to true")
	}
}

func TestSyncCursorMarshal(t *testing.T) {
	original := SyncCursor{
		Properties:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		Reservations: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
		Messages:     time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
		Reviews:      time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC),
	}

	encoded := encodeCursor(original)
	decoded := parseCursor(encoded, 90)

	if !decoded.Properties.Equal(original.Properties) {
		t.Errorf("properties cursor mismatch: %v vs %v", decoded.Properties, original.Properties)
	}
	if !decoded.Reservations.Equal(original.Reservations) {
		t.Errorf("reservations cursor mismatch: %v vs %v", decoded.Reservations, original.Reservations)
	}
	if !decoded.Messages.Equal(original.Messages) {
		t.Errorf("messages cursor mismatch: %v vs %v", decoded.Messages, original.Messages)
	}
	if !decoded.Reviews.Equal(original.Reviews) {
		t.Errorf("reviews cursor mismatch: %v vs %v", decoded.Reviews, original.Reviews)
	}
}

func TestCursorEmptyAppliesLookback(t *testing.T) {
	cursor := parseCursor("", 30)

	// Properties should be zero (fetch all)
	if !cursor.Properties.IsZero() {
		t.Errorf("expected zero properties cursor for initial sync, got %v", cursor.Properties)
	}

	// Reservations should be ~30 days ago
	expected := time.Now().UTC().AddDate(0, 0, -30)
	diff := cursor.Reservations.Sub(expected)
	if diff < -time.Minute || diff > time.Minute {
		t.Errorf("reservations cursor should be ~30 days ago, got %v (diff: %v)", cursor.Reservations, diff)
	}
}

func TestHealthTransitions(t *testing.T) {
	c := New("hospitable")
	ctx := context.Background()

	if c.Health(ctx) != connector.HealthDisconnected {
		t.Errorf("initial health should be disconnected, got %v", c.Health(ctx))
	}

	// Connect
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
	}))
	defer srv.Close()

	config := connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": "token"},
		SourceConfig: map[string]interface{}{"base_url": srv.URL},
	}
	c.Connect(ctx, config)
	if c.Health(ctx) != connector.HealthHealthy {
		t.Errorf("after connect, expected healthy, got %v", c.Health(ctx))
	}

	// Close
	c.Close()
	if c.Health(ctx) != connector.HealthDisconnected {
		t.Errorf("after close, expected disconnected, got %v", c.Health(ctx))
	}
}

func TestDisabledResourceSkipped(t *testing.T) {
	requestPaths := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPaths = append(requestPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		// Return empty for everything
		w.Write([]byte(`{"data":[],"total":0}`))
	}))
	defer srv.Close()

	c := New("hospitable")
	c.config = HospitableConfig{
		AccessToken:         "token",
		BaseURL:             srv.URL,
		PageSize:            10,
		SyncProperties:      true,
		SyncReservations:    false,
		SyncMessages:        false,
		SyncReviews:         false,
		TierProperties:      "light",
		InitialLookbackDays: 90,
	}
	c.client = NewClient(srv.URL, "token", 10)
	c.health = connector.HealthHealthy

	_, newCursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only properties should have been fetched
	for _, path := range requestPaths {
		if contains(path, "reservations") || contains(path, "reviews") || contains(path, "messages") {
			t.Errorf("disabled resource was fetched: %s", path)
		}
	}

	// Cursor should have been set for properties only
	var cursor SyncCursor
	json.Unmarshal([]byte(newCursor), &cursor)
	if cursor.Properties.IsZero() {
		t.Error("properties cursor should have been updated")
	}
}

func TestSyncFullLifecycle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case contains(r.URL.Path, "/properties"):
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{
				Data:  []Property{{ID: "p1", Name: "Beach House", Bedrooms: 3, Bathrooms: 2, MaxGuests: 6}},
				Total: 1,
			})
		case contains(r.URL.Path, "/messages"):
			json.NewEncoder(w).Encode(PaginatedResponse[Message]{
				Data: []Message{{
					ID: "m1", ReservationID: "r1", Sender: "John",
					Body: "What's the Wi-Fi password?", SentAt: time.Now(),
				}},
				Total: 1,
			})
		case contains(r.URL.Path, "/reservations"):
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{
				Data: []Reservation{{
					ID: "r1", PropertyID: "p1", Channel: "Airbnb", Status: "confirmed",
					CheckIn: "2026-04-15", CheckOut: "2026-04-18", GuestName: "John Smith",
					GuestCount: 4, NightlyRate: 250, TotalPayout: 750, Nights: 3,
					BookedAt: time.Now().Add(-24 * time.Hour),
				}},
				Total: 1,
			})
		case contains(r.URL.Path, "/reviews"):
			json.NewEncoder(w).Encode(PaginatedResponse[Review]{
				Data: []Review{{
					ID: "rev1", PropertyID: "p1", ReservationID: "r1", Rating: 5,
					ReviewText: "Amazing place!", Channel: "Airbnb", SubmittedAt: time.Now(),
				}},
				Total: 1,
			})
		default:
			w.Write([]byte(`{"data":[],"total":0}`))
		}
	}))
	defer srv.Close()

	c := New("hospitable")
	config := connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": "token"},
		SourceConfig: map[string]interface{}{"base_url": srv.URL},
	}

	err := c.Connect(context.Background(), config)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Should have: 1 property + 1 reservation + 1 message + 1 review = 4
	if len(artifacts) != 4 {
		t.Errorf("expected 4 artifacts, got %d", len(artifacts))
		for _, a := range artifacts {
			t.Logf("  %s: %s (%s)", a.ContentType, a.Title, a.SourceRef)
		}
	}

	// Cursor should be non-empty
	if cursor == "" {
		t.Error("cursor should not be empty after sync")
	}

	// Verify content types
	types := map[string]bool{}
	for _, a := range artifacts {
		types[a.ContentType] = true
	}
	for _, expected := range []string{"property/str-listing", "reservation/str-booking", "message/str-conversation", "review/str-guest"} {
		if !types[expected] {
			t.Errorf("missing content type: %s", expected)
		}
	}

	// Verify property name cache enriched reservation title
	for _, a := range artifacts {
		if a.ContentType == "reservation/str-booking" {
			if !contains(a.Title, "Beach House") {
				t.Errorf("reservation title should contain property name: %s", a.Title)
			}
		}
		if a.ContentType == "review/str-guest" {
			if !contains(a.Title, "Beach House") {
				t.Errorf("review title should contain property name: %s", a.Title)
			}
		}
	}
}

func TestPartialFailureReturnsSuccessful(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case contains(path, "/messages"):
			// Messages fail (this catches /reservations/{id}/messages)
			w.WriteHeader(http.StatusInternalServerError)
		case path == "/properties":
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{
				Data: []Property{{ID: "p1", Name: "House 1"}}, Total: 1,
			})
		case path == "/reservations":
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{
				Data:  []Reservation{{ID: "r1", PropertyID: "p1", CheckIn: "2026-04-10", CheckOut: "2026-04-12", GuestName: "Test", BookedAt: time.Now()}},
				Total: 1,
			})
		case path == "/reviews":
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
	config := connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": "token"},
		SourceConfig: map[string]interface{}{"base_url": srv.URL},
	}
	c.Connect(context.Background(), config)
	c.client.SetBackoff(fastBackoff())

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync should not fail on partial error: %v", err)
	}

	// Should have property + reservation + review = 3 (messages failed)
	if len(artifacts) != 3 {
		t.Errorf("expected 3 artifacts (messages failed), got %d", len(artifacts))
	}

	// Health should still be healthy since we got some artifacts
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("expected healthy (partial success), got %v", c.Health(context.Background()))
	}
}

func TestAllFailuresSetHealthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
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
		SyncReviews:         true,
		TierProperties:      "light",
		TierReservations:    "standard",
		TierReviews:         "full",
		InitialLookbackDays: 90,
	}
	c.client = NewClient(srv.URL, "token", 10)
	c.client.SetBackoff(fastBackoff())
	c.health = connector.HealthHealthy

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync should not return error: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts, got %d", len(artifacts))
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected error health when all fail, got %v", c.Health(context.Background()))
	}
}

func TestPropertyNameCacheEnrichesTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case contains(r.URL.Path, "/properties"):
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{
				Data: []Property{{ID: "p1", Name: "Mountain Cabin"}}, Total: 1,
			})
		case contains(r.URL.Path, "/reservations"):
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{
				Data: []Reservation{{
					ID: "r1", PropertyID: "p1", GuestName: "Alice",
					CheckIn: "2026-05-01", CheckOut: "2026-05-03",
					BookedAt: time.Now(),
				}},
				Total: 1,
			})
		default:
			json.NewEncoder(w).Encode(PaginatedResponse[Message]{Data: []Message{}, Total: 0})
		}
	}))
	defer srv.Close()

	c := New("hospitable")
	config := connector.ConnectorConfig{
		Credentials: map[string]string{"access_token": "token"},
		SourceConfig: map[string]interface{}{
			"base_url":      srv.URL,
			"sync_messages": false,
			"sync_reviews":  false,
		},
	}
	c.Connect(context.Background(), config)

	artifacts, _, _ := c.Sync(context.Background(), "")

	for _, a := range artifacts {
		if a.ContentType == "reservation/str-booking" {
			if !contains(a.Title, "Mountain Cabin") {
				t.Errorf("reservation title should contain cached property name 'Mountain Cabin', got: %s", a.Title)
			}
			if contains(a.Title, "p1") {
				t.Errorf("reservation title should NOT contain raw prop ID: %s", a.Title)
			}
		}
	}
}

func TestConnectEmptyToken(t *testing.T) {
	c := New("hospitable")
	config := connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": ""},
		SourceConfig: map[string]interface{}{},
	}

	err := c.Connect(context.Background(), config)
	if err == nil {
		t.Fatal("expected error for empty token")
	}
	if !contains(err.Error(), "access_token") {
		t.Errorf("error should mention access_token: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected error health, got %v", c.Health(context.Background()))
	}
}

// --- helper ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// fastBackoff returns a backoff with tiny delays for fast tests.
func fastBackoff() *connector.Backoff {
	return &connector.Backoff{
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
		MaxRetries: 3,
	}
}

// --- R-016: Active Reservation Message Sync ---

func TestActiveReservationMessageSync(t *testing.T) {
	// Mock: ListReservations (updated_since) returns only r2 (newly updated)
	// ListActiveReservations (checkout_after) returns r1 AND r2 (both active)
	// Messages should be fetched for BOTH r1 and r2
	var messagePaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case path == "/properties":
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
		case path == "/reservations" && r.URL.Query().Get("checkout_after") != "":
			// Active reservations query — returns both
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{
				Data: []Reservation{
					{ID: "r1", PropertyID: "p1", CheckIn: "2026-04-01", CheckOut: "2026-04-20", GuestName: "Old Guest", BookedAt: time.Now().Add(-30 * 24 * time.Hour)},
					{ID: "r2", PropertyID: "p1", CheckIn: "2026-04-10", CheckOut: "2026-04-15", GuestName: "New Guest", BookedAt: time.Now()},
				},
				Total: 2,
			})
		case path == "/reservations":
			// Incremental query — returns only r2
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{
				Data: []Reservation{
					{ID: "r2", PropertyID: "p1", CheckIn: "2026-04-10", CheckOut: "2026-04-15", GuestName: "New Guest", BookedAt: time.Now()},
				},
				Total: 1,
			})
		case contains(path, "/messages"):
			messagePaths = append(messagePaths, path)
			json.NewEncoder(w).Encode(PaginatedResponse[Message]{Data: []Message{}, Total: 0})
		case path == "/reviews":
			json.NewEncoder(w).Encode(PaginatedResponse[Review]{Data: []Review{}, Total: 0})
		default:
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
		}
	}))
	defer srv.Close()

	c := New("hospitable")
	config := connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": "token"},
		SourceConfig: map[string]interface{}{"base_url": srv.URL},
	}
	c.Connect(context.Background(), config)

	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Messages should have been fetched for both r1 and r2
	if len(messagePaths) < 2 {
		t.Errorf("expected messages fetched for at least 2 reservations, got %d paths: %v", len(messagePaths), messagePaths)
	}

	// Verify both r1 and r2 were queried
	hasR1, hasR2 := false, false
	for _, p := range messagePaths {
		if contains(p, "r1") {
			hasR1 = true
		}
		if contains(p, "r2") {
			hasR2 = true
		}
	}
	if !hasR1 {
		t.Error("messages not fetched for active reservation r1 (outside incremental window)")
	}
	if !hasR2 {
		t.Error("messages not fetched for incremental reservation r2")
	}
}

// --- R-017: Retry-After Header Parsing ---

func TestParseRetryAfterSeconds(t *testing.T) {
	now := time.Now()
	d := parseRetryAfter("5", now)
	if d != 5*time.Second {
		t.Errorf("parseRetryAfter(\"5\") = %v, want 5s", d)
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	now := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	future := now.Add(30 * time.Second)
	d := parseRetryAfter(future.Format(http.TimeFormat), now)
	if d < 29*time.Second || d > 31*time.Second {
		t.Errorf("parseRetryAfter(HTTP-date) = %v, want ~30s", d)
	}
}

func TestParseRetryAfterEmpty(t *testing.T) {
	d := parseRetryAfter("", time.Now())
	if d != 0 {
		t.Errorf("parseRetryAfter(\"\") = %v, want 0", d)
	}
}

func TestParseRetryAfterInvalid(t *testing.T) {
	d := parseRetryAfter("not-a-number", time.Now())
	if d != 0 {
		t.Errorf("parseRetryAfter(\"not-a-number\") = %v, want 0", d)
	}
}

func TestRetryAfterUsedOn429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{{ID: "p1"}}, Total: 1})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token", 10)
	props, err := client.ListProperties(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("expected success after retry: %v", err)
	}
	if len(props) != 1 {
		t.Errorf("got %d properties, want 1", len(props))
	}
}

// --- R-018: Persistent Property Name Cache ---

func TestPropertyNameCachePersistsInCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case contains(r.URL.Path, "/properties"):
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{
				Data: []Property{{ID: "p1", Name: "Beach House"}}, Total: 1,
			})
		default:
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{Data: []Reservation{}, Total: 0})
		}
	}))
	defer srv.Close()

	c := New("hospitable")
	config := connector.ConnectorConfig{
		Credentials: map[string]string{"access_token": "token"},
		SourceConfig: map[string]interface{}{
			"base_url":      srv.URL,
			"sync_messages": false,
			"sync_reviews":  false,
		},
	}
	c.Connect(context.Background(), config)

	// First sync — populates property name cache
	_, cursor1, _ := c.Sync(context.Background(), "")

	// Parse cursor and verify property names are persisted
	var sc SyncCursor
	json.Unmarshal([]byte(cursor1), &sc)
	if sc.PropertyNames["p1"] != "Beach House" {
		t.Errorf("PropertyNames[p1] = %q, want Beach House", sc.PropertyNames["p1"])
	}
}

func TestPropertyNameCacheLoadedFromCursor(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case contains(r.URL.Path, "/properties"):
			callCount++
			// Second sync returns NO updated properties
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
		case contains(r.URL.Path, "/reservations"):
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{
				Data: []Reservation{{
					ID: "r1", PropertyID: "p1", GuestName: "Alice",
					CheckIn: "2026-05-01", CheckOut: "2026-05-03",
					BookedAt: time.Now(),
				}},
				Total: 1,
			})
		default:
			json.NewEncoder(w).Encode(PaginatedResponse[Message]{Data: []Message{}, Total: 0})
		}
	}))
	defer srv.Close()

	c := New("hospitable")
	config := connector.ConnectorConfig{
		Credentials: map[string]string{"access_token": "token"},
		SourceConfig: map[string]interface{}{
			"base_url":      srv.URL,
			"sync_messages": false,
			"sync_reviews":  false,
		},
	}
	c.Connect(context.Background(), config)

	// Create a cursor with pre-loaded property names (simulating prior sync)
	cursorWithNames := encodeCursor(SyncCursor{
		Properties:    time.Now().Add(-time.Hour),
		Reservations:  time.Now().Add(-time.Hour),
		PropertyNames: map[string]string{"p1": "Beach House"},
	})

	artifacts, _, err := c.Sync(context.Background(), cursorWithNames)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Reservation title should use property name from cursor, not raw ID
	for _, a := range artifacts {
		if a.ContentType == "reservation/str-booking" {
			if !contains(a.Title, "Beach House") {
				t.Errorf("reservation title should use cached name from cursor: %s", a.Title)
			}
		}
	}
}

// --- R-021: Message Cursor Isolation ---

func TestMessageCursorNotAdvancedOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case path == "/properties":
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
		case path == "/reservations" && r.URL.Query().Get("checkout_after") != "":
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{Data: []Reservation{}, Total: 0})
		case path == "/reservations":
			json.NewEncoder(w).Encode(PaginatedResponse[Reservation]{
				Data: []Reservation{
					{ID: "r1", PropertyID: "p1", CheckIn: "2026-04-10", CheckOut: "2026-04-15", BookedAt: time.Now()},
				},
				Total: 1,
			})
		case contains(path, "/messages"):
			// Messages fail for all reservations
			w.WriteHeader(http.StatusInternalServerError)
		case path == "/reviews":
			json.NewEncoder(w).Encode(PaginatedResponse[Review]{Data: []Review{}, Total: 0})
		default:
			json.NewEncoder(w).Encode(PaginatedResponse[Property]{Data: []Property{}, Total: 0})
		}
	}))
	defer srv.Close()

	originalMsgTime := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	initialCursor := encodeCursor(SyncCursor{
		Messages: originalMsgTime,
	})

	c := New("hospitable")
	config := connector.ConnectorConfig{
		Credentials:  map[string]string{"access_token": "token"},
		SourceConfig: map[string]interface{}{"base_url": srv.URL},
	}
	c.Connect(context.Background(), config)
	c.client.SetBackoff(fastBackoff())

	_, newCursorStr, err := c.Sync(context.Background(), initialCursor)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Message cursor should NOT have advanced since messages failed
	var newCursor SyncCursor
	json.Unmarshal([]byte(newCursorStr), &newCursor)
	if !newCursor.Messages.Equal(originalMsgTime) {
		t.Errorf("message cursor should not advance on failure: got %v, want %v", newCursor.Messages, originalMsgTime)
	}

	// But reservation cursor SHOULD have advanced (reservations succeeded)
	if newCursor.Reservations.Equal(time.Time{}) || newCursor.Reservations.Before(originalMsgTime) {
		t.Errorf("reservation cursor should have advanced: %v", newCursor.Reservations)
	}
}
