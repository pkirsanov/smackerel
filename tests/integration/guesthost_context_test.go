//go:build integration

package integration

import (
	"os"
	"testing"
)

// SCN-GH-037: Context-for endpoint returns assembled guest context.
// STUB: requires live stack with seeded guest data — no assertions yet.
func TestGuestHost_Integration_ContextForAPI(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("STUB: requires DATABASE_URL — integration test body not yet implemented")
	}
	t.Skip("STUB: test body not yet implemented — needs seeded guest data + context API assertions")
}

// SCN-GH-042: Communication hints for returning guests.
// STUB: requires live stack with multi-stay guest data — no assertions yet.
func TestGuestHost_Integration_CommunicationHints(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("STUB: requires DATABASE_URL — integration test body not yet implemented")
	}
	t.Skip("STUB: test body not yet implemented — needs guest with multiple stays + hint rule assertions")
}
