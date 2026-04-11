package guesthost

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestConnectorID(t *testing.T) {
	c := New()
	if c.ID() != "guesthost" {
		t.Errorf("got ID %q, want %q", c.ID(), "guesthost")
	}
}

func TestConnectValidConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	c := New()
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"base_url": srv.URL,
			"api_key":  "valid-key",
		},
	}
	err := c.Connect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("expected healthy, got %v", c.Health(context.Background()))
	}
}

func TestConnectInvalidKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	c := New()
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"base_url": srv.URL,
			"api_key":  "bad-key",
		},
	}
	err := c.Connect(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected Connect to fail with invalid key")
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected error health, got %v", c.Health(context.Background()))
	}
}

func TestSyncNoNewEvents(t *testing.T) {
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

	cursor := "2026-04-01T00:00:00Z"
	artifacts, newCursor, err := c.Sync(context.Background(), cursor)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts, got %d", len(artifacts))
	}
	if newCursor != cursor {
		t.Errorf("cursor should remain %q, got %q", cursor, newCursor)
	}
}

func TestHealthTransitions(t *testing.T) {
	c := New()
	ctx := context.Background()

	// Initial: disconnected
	if c.Health(ctx) != connector.HealthDisconnected {
		t.Errorf("initial health should be disconnected, got %v", c.Health(ctx))
	}

	// After connect: healthy
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"base_url": srv.URL,
			"api_key":  "key",
		},
	}
	if err := c.Connect(ctx, cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if c.Health(ctx) != connector.HealthHealthy {
		t.Errorf("after connect, expected healthy, got %v", c.Health(ctx))
	}

	// After close: disconnected
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if c.Health(ctx) != connector.HealthDisconnected {
		t.Errorf("after close, expected disconnected, got %v", c.Health(ctx))
	}
}

func TestCursorAdvancement(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ActivityFeedResponse{
			Events: []ActivityEvent{
				{
					ID:        "evt-1",
					Type:      "booking.created",
					Timestamp: "2026-04-05T10:00:00Z",
					EntityID:  "b1",
					Data:      json.RawMessage(`{"propertyId":"p1","propertyName":"Lodge","guestName":"Eve","guestEmail":"eve@test.com","checkIn":"2026-05-01","checkOut":"2026-05-03","source":"direct","totalPrice":300}`),
				},
				{
					ID:        "evt-2",
					Type:      "guest.created",
					Timestamp: "2026-04-05T12:00:00Z",
					EntityID:  "g1",
					Data:      json.RawMessage(`{"email":"eve@test.com","name":"Eve"}`),
				},
			},
			HasMore: false,
		})
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

	artifacts, newCursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 2 {
		t.Errorf("expected 2 artifacts, got %d", len(artifacts))
	}
	// Cursor should advance to the latest timestamp
	if newCursor != "2026-04-05T12:00:00Z" {
		t.Errorf("expected cursor %q, got %q", "2026-04-05T12:00:00Z", newCursor)
	}
}
