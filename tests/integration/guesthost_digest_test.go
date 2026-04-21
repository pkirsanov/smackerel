//go:build integration

package integration

import (
	"os"
	"testing"
)

// SCN-GH-029: Digest includes hospitality section when bookings exist.
// STUB: requires live DB with booking artifacts — no assertions yet.
func TestGuestHost_Integration_DigestSection(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("STUB: requires DATABASE_URL — integration test body not yet implemented")
	}
	t.Skip("STUB: test body not yet implemented — needs seeded booking artifacts + digest generator")
}

// SCN-GH-033: Weekly digest includes revenue summary.
// STUB: requires live DB with expense artifacts — no assertions yet.
func TestGuestHost_Integration_WeeklyRevenue(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("STUB: requires DATABASE_URL — integration test body not yet implemented")
	}
	t.Skip("STUB: test body not yet implemented — needs expense artifacts + revenue aggregation")
}
