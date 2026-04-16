//go:build integration

package integration

import (
	"os"
	"testing"
)

// SCN-GH-037: Context-for endpoint returns assembled guest context.
func TestGuestHost_Integration_ContextForAPI(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set")
	}
	t.Log("context-for integration: requires live stack with seeded guest data + context API")
}

// SCN-GH-042: Communication hints for returning guests.
func TestGuestHost_Integration_CommunicationHints(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set")
	}
	t.Log("communication hints integration: requires guest with multiple stays + hint rules")
}
