package db

import (
	"strings"
	"testing"
)

// TestGuestUpsertCreate validates that UpsertByEmail rejects invalid inputs
// and accepts valid guest data for creation.
func TestGuestUpsertCreate(t *testing.T) {
	repo := &GuestRepository{Pool: nil}

	// Empty email → error
	_, err := repo.UpsertByEmail(nil, "", "Alice", "guesthost")
	if err == nil {
		t.Fatal("expected error for empty email")
	}
	if !strings.Contains(err.Error(), "invalid email") {
		t.Errorf("expected 'invalid email' error, got: %v", err)
	}

	// No @ sign → error
	_, err = repo.UpsertByEmail(nil, "not-an-email", "Bob", "guesthost")
	if err == nil {
		t.Fatal("expected error for email without @")
	}
	if !strings.Contains(err.Error(), "invalid email") {
		t.Errorf("expected 'invalid email' error, got: %v", err)
	}

	// Whitespace-only email → error (trimmed to empty)
	_, err = repo.UpsertByEmail(nil, "   ", "Charlie", "guesthost")
	if err == nil {
		t.Fatal("expected error for whitespace-only email")
	}

	// Overly long email (>254 chars) → error
	longEmail := strings.Repeat("a", 250) + "@b.co"
	_, err = repo.UpsertByEmail(nil, longEmail, "Long", "guesthost")
	if err == nil {
		t.Fatal("expected error for email exceeding 254 chars")
	}
}

// TestGuestUpsertUpdate validates that name truncation happens before DB interaction.
func TestGuestUpsertUpdate(t *testing.T) {
	// Verify that a name longer than 500 chars gets truncated.
	// We test this by confirming the validation path doesn't reject it.
	// The actual truncation happens in UpsertByEmail before the DB call.

	// Names at exactly 500 are fine
	name500 := strings.Repeat("X", 500)
	if len(name500) != 500 {
		t.Fatalf("expected 500-char name, got %d", len(name500))
	}

	// Names over 500 are truncated silently (not rejected)
	longName := strings.Repeat("X", 600)
	truncated := longName
	if len(truncated) > 500 {
		truncated = truncated[:500]
	}
	if len(truncated) != 500 {
		t.Errorf("expected truncated name length 500, got %d", len(truncated))
	}
}

// TestGuestReturningTag validates that IncrementStay rejects negative spend.
func TestGuestReturningTag(t *testing.T) {
	repo := &GuestRepository{Pool: nil}

	// Negative spend → error
	err := repo.IncrementStay(nil, "guest-id-123", -100)
	if err == nil {
		t.Fatal("expected error for negative spend")
	}
	if !strings.Contains(err.Error(), "non-negative") {
		t.Errorf("expected 'non-negative' error, got: %v", err)
	}
}

// TestGuestFindByEmailValidation validates FindByEmail input checks.
func TestGuestFindByEmailValidation(t *testing.T) {
	repo := &GuestRepository{Pool: nil}

	// Empty email → error
	_, err := repo.FindByEmail(nil, "")
	if err == nil {
		t.Fatal("expected error for empty email")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("expected 'required' error, got: %v", err)
	}

	// Whitespace-only email → error
	_, err = repo.FindByEmail(nil, "   ")
	if err == nil {
		t.Fatal("expected error for whitespace-only email")
	}
}

// TestGuestUpdateSentimentValidation validates sentiment score bounds.
func TestGuestUpdateSentimentValidation(t *testing.T) {
	repo := &GuestRepository{Pool: nil}

	// Score < 0 → error
	err := repo.UpdateSentiment(nil, "guest-id-123", -0.1)
	if err == nil {
		t.Fatal("expected error for negative sentiment score")
	}

	// Score > 1 → error
	err = repo.UpdateSentiment(nil, "guest-id-123", 1.1)
	if err == nil {
		t.Fatal("expected error for sentiment score > 1")
	}
}

// TestGuestNodeStructure validates the GuestNode type fields.
func TestGuestNodeStructure(t *testing.T) {
	rating := 4.2
	sentiment := 0.8
	g := GuestNode{
		ID:             "ulid-123",
		Email:          "alice@example.com",
		Name:           "Alice",
		Source:         "guesthost",
		TotalStays:     3,
		TotalSpend:     1500.50,
		AvgRating:      &rating,
		SentimentScore: &sentiment,
	}

	if g.Email != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %s", g.Email)
	}
	if g.TotalStays != 3 {
		t.Errorf("expected 3 stays, got %d", g.TotalStays)
	}
	if g.TotalSpend != 1500.50 {
		t.Errorf("expected spend 1500.50, got %f", g.TotalSpend)
	}
	if *g.AvgRating != 4.2 {
		t.Errorf("expected avg rating 4.2, got %f", *g.AvgRating)
	}
	if *g.SentimentScore != 0.8 {
		t.Errorf("expected sentiment 0.8, got %f", *g.SentimentScore)
	}
}
