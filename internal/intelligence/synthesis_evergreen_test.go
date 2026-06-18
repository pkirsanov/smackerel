// Spec 095 SCOPE-08 — synthesis (§10) candidate-pool exclusion wiring tests.
//
// These prove the pool-exclusion properties at the query-construction level
// WITHOUT a DB (the live ingest→synthesis end-to-end is the accel-tier-gated
// F-095-E2E-LIVE deferral): (1) policy OFF leaves the candidate query
// byte-for-byte unchanged (safe additive activation); (2) policy ON drops
// persisted-ephemeral (evergreen_score < 0) candidates (R12); (3) a NULL score
// is kept (Principle 9). The fourth property — an excluded artifact stays
// searchable (R13) — is proven by the evergreen package's cross-path isolation
// guard (the exclusion seam is wired here, never on the §9.2 search path).
package intelligence

import (
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/retrieval/evergreen"
)

// TestBuildSynthesisClusterQuery_DefaultUnchanged — Property 1: with the SST
// switch off (the shipped default) the §10 synthesis candidate query carries no
// evergreen reference at all and preserves every structural landmark of the
// pre-spec-095 query, so synthesis behavior is unchanged until the operator
// opts in.
func TestBuildSynthesisClusterQuery_DefaultUnchanged(t *testing.T) {
	off := buildSynthesisClusterQuery(false)
	if strings.Contains(off, "evergreen") {
		t.Errorf("synthesis candidate query (exclusion off) must carry NO evergreen reference (default unchanged), got:\n%s", off)
	}
	for _, want := range []string{
		"WITH topic_groups AS (",
		"FROM edges e",
		"JOIN artifacts a ON a.id = e.src_id",
		"WHERE e.edge_type = 'BELONGS_TO' AND e.src_type = 'artifact'",
		"GROUP BY t.id, t.name",
		"HAVING COUNT(*) >= 3 AND COUNT(DISTINCT a.source_id) >= 2",
		"LIMIT $1",
	} {
		if !strings.Contains(off, want) {
			t.Errorf("synthesis candidate query missing pre-spec-095 landmark %q", want)
		}
	}
}

// TestBuildSynthesisClusterQuery_ExcludesEphemeralAdditively — Properties 2 & 3
// plus the byte-for-byte additivity invariant: ON drops ephemeral candidates,
// keeps NULL + evergreen, and is EXACTLY OFF with the predicate spliced in.
func TestBuildSynthesisClusterQuery_ExcludesEphemeralAdditively(t *testing.T) {
	off := buildSynthesisClusterQuery(false)
	on := buildSynthesisClusterQuery(true)
	predicate := evergreen.PoolExclusionSQLPredicate("a", true)

	if !strings.Contains(on, "a.evergreen_score") {
		t.Fatal("synthesis candidate query (exclusion on) must filter on a.evergreen_score (R12)")
	}
	if !strings.Contains(on, "a.evergreen_score IS NULL") {
		t.Error("synthesis exclusion must keep NULL/not-yet-scored candidates (Principle 9)")
	}
	if !strings.Contains(on, "a.evergreen_score >= 0") {
		t.Error("synthesis exclusion must keep present-evergreen (score >= 0) candidates")
	}

	// Byte-for-byte additivity: removing the spliced predicate from ON must
	// reproduce OFF verbatim — proving the default candidate set is preserved
	// and ON changes nothing else.
	if got := strings.Replace(on, predicate, "", 1); got != off {
		t.Errorf("exclusion is not purely additive — default (OFF) candidate behavior changed.\nON-minus-predicate:\n%q\nOFF:\n%q", got, off)
	}

	t.Run("would_catch_regression", func(t *testing.T) {
		// If the predicate were widened to also drop NULLs, the IS NULL
		// whitelist would vanish and not-yet-scored artifacts would be wrongly
		// excluded (Principle 9 violation).
		if !strings.Contains(buildSynthesisClusterQuery(true), "IS NULL") {
			t.Fatal("regression: synthesis exclusion no longer whitelists NULL (Principle 9 violated)")
		}
	})
}
