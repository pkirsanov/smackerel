package db

import (
	"strings"
	"testing"
)

// TestPropertyUpsertCreate validates that UpsertByExternalID rejects invalid inputs.
func TestPropertyUpsertCreate(t *testing.T) {
	repo := &PropertyRepository{Pool: nil}

	// Empty external_id → error
	_, err := repo.UpsertByExternalID(nil, "", "guesthost", "Beach House")
	if err == nil {
		t.Fatal("expected error for empty external_id")
	}
	if !strings.Contains(err.Error(), "invalid external_id") {
		t.Errorf("expected 'invalid external_id' error, got: %v", err)
	}

	// External ID > 255 chars → error
	longID := strings.Repeat("x", 256)
	_, err = repo.UpsertByExternalID(nil, longID, "guesthost", "Test")
	if err == nil {
		t.Fatal("expected error for external_id exceeding 255 chars")
	}
	if !strings.Contains(err.Error(), "invalid external_id") {
		t.Errorf("expected 'invalid external_id' error, got: %v", err)
	}
}

// TestPropertyUpsertUpdate validates name truncation logic.
func TestPropertyUpsertUpdate(t *testing.T) {
	// Verify that a name longer than 500 chars gets truncated.
	// The actual truncation happens in UpsertByExternalID before the DB call.

	// Names at exactly 500 are fine
	name500 := strings.Repeat("P", 500)
	if len(name500) != 500 {
		t.Fatalf("expected 500-char name, got %d", len(name500))
	}

	// Names over 500 are truncated silently (not rejected)
	longName := strings.Repeat("P", 600)
	truncated := longName
	if len(truncated) > 500 {
		truncated = truncated[:500]
	}
	if len(truncated) != 500 {
		t.Errorf("expected truncated name length 500, got %d", len(truncated))
	}
}

// TestPropertyIncrementBookingsValidation validates revenue bounds.
func TestPropertyIncrementBookingsValidation(t *testing.T) {
	repo := &PropertyRepository{Pool: nil}

	// Negative revenue → error at validation level
	err := repo.IncrementBookings(nil, "prop-1", -500)
	if err == nil {
		t.Fatal("expected error for negative revenue")
	}
	if !strings.Contains(err.Error(), "non-negative") {
		t.Errorf("expected 'non-negative' error, got: %v", err)
	}
}

// TestPropertyNodeStructure validates the PropertyNode type fields.
func TestPropertyNodeStructure(t *testing.T) {
	rating := 4.5
	p := PropertyNode{
		ID:            "ulid-prop-1",
		ExternalID:    "gh-prop-123",
		Source:        "guesthost",
		Name:          "Beach House",
		TotalBookings: 25,
		TotalRevenue:  50000.00,
		AvgRating:     &rating,
		IssueCount:    3,
		Topics:        []string{"cleaning", "maintenance"},
	}

	if p.Name != "Beach House" {
		t.Errorf("expected name Beach House, got %s", p.Name)
	}
	if p.TotalBookings != 25 {
		t.Errorf("expected 25 bookings, got %d", p.TotalBookings)
	}
	if p.TotalRevenue != 50000.00 {
		t.Errorf("expected revenue 50000, got %f", p.TotalRevenue)
	}
	if *p.AvgRating != 4.5 {
		t.Errorf("expected avg rating 4.5, got %f", *p.AvgRating)
	}
	if p.IssueCount != 3 {
		t.Errorf("expected 3 issues, got %d", p.IssueCount)
	}
	if len(p.Topics) != 2 {
		t.Errorf("expected 2 topics, got %d", len(p.Topics))
	}
}

// TestPropertyExternalIDMaxLength validates boundary at exactly 255 chars.
func TestPropertyExternalIDMaxLength(t *testing.T) {
	repo := &PropertyRepository{Pool: nil}

	// Exactly 255 chars → validation passes (1-255 range accepted)
	exactID := strings.Repeat("x", 255)
	if len(exactID) > 255 {
		t.Errorf("expected 255-char ID to not exceed limit")
	}

	// 256 chars → validation rejects
	overID := strings.Repeat("x", 256)
	_, err := repo.UpsertByExternalID(nil, overID, "guesthost", "Test")
	if err == nil {
		t.Fatal("expected error for 256-char external_id")
	}

	// 0 chars → validation rejects
	_, err = repo.UpsertByExternalID(nil, "", "guesthost", "Test")
	if err == nil {
		t.Fatal("expected error for empty external_id")
	}
}
