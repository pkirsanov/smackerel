package browser

import (
	"database/sql"
	"slices"

	sqlite "modernc.org/sqlite"
)

// init registers the modernc.org/sqlite driver under the conventional name
// "sqlite3" so that the production call site in browser.go
// (sql.Open("sqlite3", ...)) resolves to a working driver.
//
// modernc.org/sqlite's own init() registers the driver under the name
// "sqlite". This file adds an alias under "sqlite3" without modifying the
// production call site. The blank-import-shaped registration here is what
// the BUG-010-001 driver-presence sentinel test depends on transitively;
// removing this file (or this init) is intended to re-break the test.
//
// We guard with slices.Contains so that re-registration (e.g. if another
// package adds the same alias in the future) does not panic at startup.
func init() {
	if slices.Contains(sql.Drivers(), "sqlite3") {
		return
	}
	sql.Register("sqlite3", &sqlite.Driver{})
}
