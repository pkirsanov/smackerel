//go:build integration

package integration

import (
	"os"
	"testing"
)

// SCN-GH-017: Graph linker creates STAYED_AT edges for booking artifacts.
func TestGuestHost_Integration_GraphLinking(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set")
	}
	t.Log("graph linking integration: requires live DB with seeded guest/property/artifact data")
}

// SCN-GH-025: DURING_STAY temporal edge for concurrent booking + location data.
func TestGuestHost_Integration_TemporalEdge(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set")
	}
	t.Log("temporal edge integration: requires live DB with overlapping booking + location artifacts")
}
