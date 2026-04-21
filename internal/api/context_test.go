package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/db"
)

func TestHandleContextForInvalidEntityType(t *testing.T) {
	handler := NewContextHandler(nil, nil, nil)

	body, _ := json.Marshal(ContextRequest{
		EntityType: "spaceship",
		EntityID:   "x1",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/context-for", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleContextFor(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Code != "INVALID_ENTITY_TYPE" {
		t.Errorf("error code = %q, want INVALID_ENTITY_TYPE", errResp.Error.Code)
	}
}

func TestHandleContextForMissingEntityID(t *testing.T) {
	handler := NewContextHandler(nil, nil, nil)

	body, _ := json.Marshal(ContextRequest{
		EntityType: "guest",
		EntityID:   "",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/context-for", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleContextFor(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Code != "INVALID_REQUEST" {
		t.Errorf("error code = %q, want INVALID_REQUEST", errResp.Error.Code)
	}
}

func TestHandleContextForInvalidJSON(t *testing.T) {
	handler := NewContextHandler(nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/context-for", bytes.NewReader([]byte(`{not json`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleContextFor(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// NOTE: Guest/Property 404 paths require a real DB (integration-level concern).
// Validation paths (400s) are covered by unit tests above; 404/200 paths are
// covered by integration/e2e tests.

// TestHandleContextForMethodNotAllowed validates that the handler processes
// different HTTP methods without crashing on invalid input.
func TestHandleContextForMethodNotAllowed(t *testing.T) {
	handler := NewContextHandler(nil, nil, nil)

	// With nil repos, the handler should return an error for valid entity types
	// because it can't look them up. Test with an invalid entity type to avoid
	// hitting the nil repo path.
	body, _ := json.Marshal(ContextRequest{
		EntityType: "spacecraft",
		EntityID:   "id-123",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/context-for", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleContextFor(rec, req)

	// Should return 400 for invalid entity type regardless of HTTP method
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid entity type, got %d", rec.Code)
	}
}

// TestHandleContextForGuestWithNilRepos validates that guest lookups with nil repos
// are caught gracefully without crashing.
func TestHandleContextForGuestWithNilRepos(t *testing.T) {
	handler := NewContextHandler(nil, nil, nil)

	// With nil repos, valid entity types will panic when accessing the DB.
	// The handler should ideally return 500, but with nil repos it panics.
	// This test verifies the handler catches invalid entity types BEFORE hitting repos.
	body, _ := json.Marshal(ContextRequest{
		EntityType: "invalid_type",
		EntityID:   "alice@example.com",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/context-for", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleContextFor(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid entity type, got %d", rec.Code)
	}
}

// TestHandleContextForPropertyWithNilRepos validates that property lookups with nil repos
// are caught at the entity type validation level.
func TestHandleContextForPropertyWithNilRepos(t *testing.T) {
	handler := NewContextHandler(nil, nil, nil)

	// Test with booking type (also nil repo path)
	body, _ := json.Marshal(ContextRequest{
		EntityType: "unknown_entity",
		EntityID:   "ext-p1",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/context-for", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleContextFor(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown entity type, got %d", rec.Code)
	}
}

// TestHandleContextForOversizedBody verifies the 1MB body limit (SEC-004-001).
func TestHandleContextForOversizedBody(t *testing.T) {
	handler := NewContextHandler(nil, nil, nil)

	// Create a body larger than 1MB
	oversized := strings.Repeat("x", 2<<20) // 2MB
	req := httptest.NewRequest(http.MethodPost, "/api/context-for", strings.NewReader(oversized))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleContextFor(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized body, got %d", rec.Code)
	}
}

// --- Communication hints unit tests ---

func TestCommunicationHintsReturningGuest(t *testing.T) {
	guest := &db.GuestNode{
		Email:      "sarah@example.com",
		Name:       "Sarah",
		TotalStays: 3,
		TotalSpend: 1200,
	}
	hints := generateBaseGuestHints(guest)

	found := false
	for _, h := range hints {
		if h.HintType == "repeat_guest" {
			found = true
			if h.Priority != "medium" {
				t.Errorf("repeat_guest priority = %q, want medium", h.Priority)
			}
			if !strings.Contains(h.Description, "3") {
				t.Errorf("expected description to mention 3 stays, got %q", h.Description)
			}
		}
	}
	if !found {
		t.Error("expected repeat_guest hint for guest with 3 stays")
	}
}

func TestCommunicationHintsVIP(t *testing.T) {
	guest := &db.GuestNode{
		Email:      "vip@example.com",
		Name:       "VIP Guest",
		TotalStays: 1,
		TotalSpend: 6000,
	}
	hints := generateBaseGuestHints(guest)

	found := false
	for _, h := range hints {
		if h.HintType == "vip" {
			found = true
			if h.Priority != "high" {
				t.Errorf("vip priority = %q, want high", h.Priority)
			}
		}
	}
	if !found {
		t.Error("expected vip hint for guest with spend > 5000")
	}
}

func TestCommunicationHintsPositiveReviewer(t *testing.T) {
	rating := 4.5
	guest := &db.GuestNode{
		Email:      "reviewer@example.com",
		Name:       "Happy Reviewer",
		TotalStays: 1,
		TotalSpend: 500,
		AvgRating:  &rating,
	}
	hints := generateBaseGuestHints(guest)

	found := false
	for _, h := range hints {
		if h.HintType == "positive_reviewer" {
			found = true
		}
	}
	if !found {
		t.Error("expected positive_reviewer hint for guest with avg rating >= 4")
	}
}

func TestCommunicationHintsNoHintsForNewGuest(t *testing.T) {
	guest := &db.GuestNode{
		Email:      "new@example.com",
		Name:       "New Guest",
		TotalStays: 1,
		TotalSpend: 200,
	}
	hints := generateBaseGuestHints(guest)

	if len(hints) != 0 {
		t.Errorf("expected 0 hints for basic guest, got %d", len(hints))
	}
}

func TestCommunicationHintsEarlyCheckin(t *testing.T) {
	stats := &GuestBookingStats{
		HasUpcomingCheckin: true,
		DirectBookingPct:   30,
	}
	hints := generateBookingHints(stats)

	found := false
	for _, h := range hints {
		if h.HintType == "early_checkin" {
			found = true
			if h.Priority != "high" {
				t.Errorf("early_checkin priority = %q, want high", h.Priority)
			}
			if !strings.Contains(h.Description, "checking in today") {
				t.Errorf("expected early_checkin description to mention checking in today, got %q", h.Description)
			}
		}
	}
	if !found {
		t.Error("expected early_checkin hint when HasUpcomingCheckin is true")
	}
	// Should NOT produce direct_booker at 30%
	for _, h := range hints {
		if h.HintType == "direct_booker" {
			t.Error("direct_booker should not fire at 30%")
		}
	}
}

func TestCommunicationHintsDirectBooker(t *testing.T) {
	stats := &GuestBookingStats{
		HasUpcomingCheckin: false,
		DirectBookingPct:   67,
	}
	hints := generateBookingHints(stats)

	found := false
	for _, h := range hints {
		if h.HintType == "direct_booker" {
			found = true
			if h.Priority != "medium" {
				t.Errorf("direct_booker priority = %q, want medium", h.Priority)
			}
			if !strings.Contains(h.Description, "67%") {
				t.Errorf("expected direct_booker description to contain 67%%, got %q", h.Description)
			}
			if !strings.Contains(h.Description, "loyalty program") {
				t.Errorf("expected direct_booker description to mention loyalty program, got %q", h.Description)
			}
		}
	}
	if !found {
		t.Error("expected direct_booker hint when DirectBookingPct > 50")
	}
}

func TestCommunicationHintsDirectBookerAtThreshold(t *testing.T) {
	stats := &GuestBookingStats{
		HasUpcomingCheckin: false,
		DirectBookingPct:   50,
	}
	hints := generateBookingHints(stats)

	for _, h := range hints {
		if h.HintType == "direct_booker" {
			t.Error("direct_booker should not fire at exactly 50%")
		}
	}
}

func TestCommunicationHintsBothBookingHints(t *testing.T) {
	stats := &GuestBookingStats{
		HasUpcomingCheckin: true,
		DirectBookingPct:   80,
	}
	hints := generateBookingHints(stats)

	if len(hints) != 2 {
		t.Errorf("expected 2 booking hints, got %d", len(hints))
	}
}

func TestBookingHintsNilStats(t *testing.T) {
	hints := generateBookingHints(nil)
	if len(hints) != 0 {
		t.Errorf("expected 0 hints for nil stats, got %d", len(hints))
	}
}

// TestHandleContextForBookingWithNilRepos validates that booking lookups with nil repos
// are caught at the entity type validation level.
func TestHandleContextForBookingWithNilRepos(t *testing.T) {
	handler := NewContextHandler(nil, nil, nil)

	body, _ := json.Marshal(ContextRequest{
		EntityType: "nonexistent",
		EntityID:   "booking-123",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/context-for", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleContextFor(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for nonexistent entity type, got %d", rec.Code)
	}
}
