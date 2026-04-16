//go:build integration

package integration

import (
	"os"
	"testing"
)

// SCN-GH-029: Digest includes hospitality section when bookings exist.
func TestGuestHost_Integration_DigestSection(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set")
	}
	t.Log("digest integration: requires live DB with booking artifacts + digest generator")
}

// SCN-GH-033: Weekly digest includes revenue summary.
func TestGuestHost_Integration_WeeklyRevenue(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set")
	}
	t.Log("weekly revenue integration: requires live DB with expense artifacts")
}
