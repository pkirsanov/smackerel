//go:build integration

package integration

import (
	"os"
	"testing"
)

// SCN-GH-017: Graph linker creates STAYED_AT edges for booking artifacts.
// STUB: requires live DB with seeded data — no assertions yet.
func TestGuestHost_Integration_GraphLinking(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("STUB: requires DATABASE_URL — integration test body not yet implemented")
	}
	t.Skip("STUB: test body not yet implemented — needs seeded guest/property/artifact data + edge assertions")
}

// SCN-GH-025: DURING_STAY temporal edge for concurrent booking + location data.
// STUB: requires live DB with overlapping temporal data — no assertions yet.
func TestGuestHost_Integration_TemporalEdge(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("STUB: requires DATABASE_URL — integration test body not yet implemented")
	}
	t.Skip("STUB: test body not yet implemented — needs overlapping booking + location artifacts")
}
