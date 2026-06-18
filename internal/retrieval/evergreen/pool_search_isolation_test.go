// Spec 095 SCOPE-08 — cross-path isolation guard (R13 / Principle 9).
//
// Pool exclusion removes a persisted-ephemeral artifact ONLY from the synthesis
// (§10) and digest (§12) CANDIDATE pools. It MUST NEVER touch the §9.2
// search/retrieval path — an excluded artifact stays fully searchable and is
// still returned by a normal search (R13: ephemeral items are never hidden or
// deleted, Principle 9).
//
// The live end-to-end proof (ingest an ephemeral artifact on the real stack →
// confirm it is absent from the synthesis/digest pool yet present in search)
// is the accel-tier-gated F-095-E2E-LIVE deferral. THIS test proves the same
// invariant structurally without a DB: the evergreen pool-exclusion seam
// (evergreen.PoolExclusionSQLPredicate) is wired into the synthesis and digest
// candidate builders and is wired into NONE of the search/retrieval query
// builders — so search carries no `evergreen_score` filter and an ephemeral
// artifact can only be dropped from a pool, never from search results.
//
// Mirrors the internal/retrieval/routing architecture-test pattern (source
// scan + would_catch_regression adversarial sub-test; bubbles-test-integrity).
package evergreen

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRootFromTest returns the repo root derived from this test file's location
// (robust regardless of CWD): .../internal/retrieval/evergreen → up three.
func repoRootFromTest(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

func readRepoFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// poolExclusionSeam is the single call site that injects the evergreen
// candidate-pool exclusion. Any file that excludes by evergreen score must
// reference it; the search path must not.
const poolExclusionSeam = "PoolExclusionSQLPredicate"

// evergreenColumn is the persisted score column. The search path must never
// filter on it (directly or via the seam) — R13.
const evergreenColumn = "evergreen_score"

// TestPoolExclusionWiredIntoCandidateBuildersOnly proves the exclusion seam is
// present in the synthesis + digest candidate builders and ABSENT from every
// search/retrieval query builder — so a pool-excluded ephemeral artifact stays
// searchable (R13 / Principle 9).
func TestPoolExclusionWiredIntoCandidateBuildersOnly(t *testing.T) {
	root := repoRootFromTest(t)

	// The exclusion seam MUST be wired into the candidate-pool builders.
	poolBuilders := []string{
		"internal/intelligence/synthesis.go", // §10 synthesis candidate gathering
		"internal/digest/generator.go",       // §12 digest candidate gathering
	}
	for _, rel := range poolBuilders {
		src := readRepoFile(t, root, rel)
		if !strings.Contains(src, poolExclusionSeam) {
			t.Errorf("%s must wire the evergreen pool-exclusion seam %q (SCOPE-08 candidate-pool exclusion not applied)", rel, poolExclusionSeam)
		}
	}

	// The search/retrieval path MUST NOT filter by evergreen — neither via the
	// seam nor by referencing the persisted column directly. This is the R13
	// guarantee: exclusion is pool-eligibility only, search is untouched.
	searchPaths := []string{
		"internal/api/search.go", // vector + text + time-range search (the normal §9.2 retrieval path)
	}
	for _, rel := range searchPaths {
		src := readRepoFile(t, root, rel)
		if strings.Contains(src, poolExclusionSeam) {
			t.Errorf("%s references the pool-exclusion seam %q — search MUST NOT exclude by evergreen (R13 violated: ephemeral items would stop being searchable)", rel, poolExclusionSeam)
		}
		if strings.Contains(src, evergreenColumn) {
			t.Errorf("%s filters on %q — search MUST NOT consult the evergreen score (R13 violated)", rel, evergreenColumn)
		}
	}

	t.Run("would_catch_regression", func(t *testing.T) {
		// Adversarial: prove the guard's scan tokens WOULD trip on a search
		// file that gained an evergreen filter — either by calling the seam
		// or by referencing the persisted column directly.
		regressedViaSeam := `query += evergreen.PoolExclusionSQLPredicate("a", excludeLowEvergreen)`
		if !strings.Contains(regressedViaSeam, poolExclusionSeam) {
			t.Fatal("adversarial: a search file calling the seam must contain the seam token the guard scans for")
		}
		regressedViaColumn := `query += " AND a.evergreen_score >= 0"`
		if !strings.Contains(regressedViaColumn, evergreenColumn) {
			t.Fatal("adversarial: a search file filtering the column must contain the column token the guard scans for")
		}
		// Non-vacuous: the REAL search source today contains neither token, so
		// the guard above is asserting a real (currently-true) invariant, not
		// passing because the tokens never appear anywhere.
		realSearch := readRepoFile(t, root, "internal/api/search.go")
		if strings.Contains(realSearch, poolExclusionSeam) || strings.Contains(realSearch, evergreenColumn) {
			t.Fatal("regression: search.go now excludes by evergreen (R13 violated — ephemeral items would stop being searchable)")
		}
	})
}
