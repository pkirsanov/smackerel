package guesthost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// fastBackoff returns a backoff with tiny delays for fast tests.
func fastBackoff() *connector.Backoff {
	return &connector.Backoff{
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
		MaxRetries: 3,
	}
}

func TestClientAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token-abc")
	err := client.Validate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer test-token-abc" {
		t.Errorf("got auth header %q, want %q", gotAuth, "Bearer test-token-abc")
	}
}

func TestClientValidateSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "valid-key")
	err := client.Validate(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestClientValidateUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "bad-key")
	err := client.Validate(context.Background())
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if got := err.Error(); !strings.Contains(got, "unauthorized") {
		t.Errorf("error should contain 'unauthorized', got: %s", got)
	}
}

func TestClientValidateForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "limited-key")
	err := client.Validate(context.Background())
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if got := err.Error(); !strings.Contains(got, "forbidden") {
		t.Errorf("error should contain 'forbidden', got: %s", got)
	}
}

func TestFetchActivityURLConstruction(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ActivityFeedResponse{Events: []ActivityEvent{}, HasMore: false})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	_, err := client.FetchActivity(context.Background(), "2026-04-01T00:00:00Z", "booking.created,review.received", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(gotURL, "/api/v1/activity") {
		t.Errorf("URL should contain /api/v1/activity, got: %s", gotURL)
	}
	if !strings.Contains(gotURL, "since=2026-04-01T00%3A00%3A00Z") {
		t.Errorf("URL should contain since param, got: %s", gotURL)
	}
	if !strings.Contains(gotURL, "types=booking.created") {
		t.Errorf("URL should contain types param, got: %s", gotURL)
	}
	if !strings.Contains(gotURL, "limit=50") {
		t.Errorf("URL should contain limit=50, got: %s", gotURL)
	}
}

func TestFetchActivityHasMorePagination(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case 1:
			json.NewEncoder(w).Encode(ActivityFeedResponse{
				Events: []ActivityEvent{
					{ID: "e1", Type: "booking.created", Timestamp: "2026-04-01T10:00:00Z", EntityID: "b1", Data: json.RawMessage(`{"propertyId":"p1","propertyName":"Beach","guestName":"Alice","guestEmail":"a@b.com","checkIn":"2026-05-01","checkOut":"2026-05-03","source":"direct","totalPrice":500}`)},
				},
				HasMore: true,
				Cursor:  "cursor-page-2",
			})
		case 2:
			// Verify cursor is passed
			if r.URL.Query().Get("cursor") != "cursor-page-2" {
				t.Errorf("page 2 should have cursor=cursor-page-2, got: %s", r.URL.Query().Get("cursor"))
			}
			json.NewEncoder(w).Encode(ActivityFeedResponse{
				Events: []ActivityEvent{
					{ID: "e2", Type: "guest.created", Timestamp: "2026-04-01T11:00:00Z", EntityID: "g1", Data: json.RawMessage(`{"email":"g@h.com","name":"Bob"}`)},
				},
				HasMore: false,
			})
		default:
			t.Errorf("unexpected page %d request", page)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	resp, err := client.FetchActivity(context.Background(), "", "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Events) != 2 {
		t.Errorf("got %d events, want 2", len(resp.Events))
	}
	if resp.HasMore {
		t.Error("final response should have HasMore=false")
	}
	if page != 2 {
		t.Errorf("expected 2 page requests, got %d", page)
	}
}

func TestClientRetryOn429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	client.SetBackoff(fastBackoff())
	err := client.Validate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestClientMaxRetriesOn429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	client.SetBackoff(fastBackoff())
	err := client.Validate(context.Background())
	if err == nil {
		t.Fatal("expected error after max retries")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("error should mention rate limiting: %v", err)
	}
	// fastBackoff has MaxRetries=3, so 1 initial + 3 retries = 4 attempts
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
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	client.SetBackoff(fastBackoff())
	err := client.Validate(context.Background())
	if err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestFetchActivityEmptyCursorOmitsSince(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ActivityFeedResponse{Events: []ActivityEvent{}, HasMore: false})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "token")
	_, err := client.FetchActivity(context.Background(), "", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(gotURL, "since=") {
		t.Errorf("empty cursor should omit since param, got URL: %s", gotURL)
	}
	if strings.Contains(gotURL, "limit=") {
		t.Errorf("zero limit should omit limit param, got URL: %s", gotURL)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     map[string]interface{}
		wantErr string
	}{
		{
			name:    "missing api_key",
			cfg:     map[string]interface{}{"base_url": "http://localhost"},
			wantErr: "api_key",
		},
		{
			name:    "empty api_key",
			cfg:     map[string]interface{}{"base_url": "http://localhost", "api_key": ""},
			wantErr: "api_key",
		},
		{
			name:    "missing base_url",
			cfg:     map[string]interface{}{"api_key": "secret"},
			wantErr: "base_url",
		},
		{
			name:    "empty base_url",
			cfg:     map[string]interface{}{"api_key": "secret", "base_url": "  "},
			wantErr: "base_url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New()
			err := c.Connect(context.Background(), connector.ConnectorConfig{
				SourceConfig: tt.cfg,
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error should contain %q, got: %v", tt.wantErr, err)
			}
		})
	}
}
