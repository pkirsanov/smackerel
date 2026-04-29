//go:build integration

package integration

import (
	"database/sql"
	"slices"
	"testing"

	// Blank-import the browser connector package so this test transitively depends
	// on the SQLite driver registration in internal/connector/browser/sqlite_driver.go.
	// Removing that blank import (or the registration init) makes this sentinel fail,
	// which is the adversarial property required by BUG-010-001 contract A1.
	_ "github.com/smackerel/smackerel/internal/connector/browser"
)

// TestSQLiteDriverRegistered is the adversarial driver-presence sentinel for
// BUG-010-001. It asserts that some package in the build graph has registered
// a database/sql driver under the name "sqlite3" so that
// internal/connector/browser/browser.go:111 (sql.Open("sqlite3", ...)) works
// at runtime.
//
// Per spec.md A5, this test contains NO t.Skip and NO bailout-on-error
// pattern. If the driver is not registered, the test MUST hard-fail.
func TestSQLiteDriverRegistered(t *testing.T) {
	drivers := sql.Drivers()
	t.Logf("sql.Drivers() = %v", drivers)

	if !slices.Contains(drivers, "sqlite3") {
		t.Fatalf("expected sql.Drivers() to contain %q; got %v", "sqlite3", drivers)
	}

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf(`sql.Open("sqlite3", ":memory:") returned error: %v`, err)
	}
	if db == nil {
		t.Fatal(`sql.Open("sqlite3", ":memory:") returned nil *sql.DB`)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("db.Ping on in-memory SQLite failed: %v", err)
	}
}
