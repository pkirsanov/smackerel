// Spec 095 SCOPE-08 — digest (§12) candidate-pool exclusion wiring tests.
//
// These prove the pool-exclusion properties at the query-construction level
// WITHOUT a DB (the live ingest→digest end-to-end is the accel-tier-gated
// F-095-E2E-LIVE deferral): (1) policy OFF leaves the candidate query
// byte-for-byte unchanged (safe additive activation); (2) policy ON drops
// persisted-ephemeral (evergreen_score < 0) candidates (R12); (3) a NULL score
// is kept (Principle 9). The fourth property — an excluded artifact stays
// searchable (R13) — is proven by the evergreen package's cross-path isolation
// guard (the exclusion seam is wired here, never on the §9.2 search path).
package digest

import (
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/retrieval/evergreen"
)

// TestBuildOvernightArtifactsQuery_DefaultUnchanged — Property 1: with the SST
// switch off (the shipped default) the §12 digest candidate query carries no
// evergreen reference and preserves every landmark of the pre-spec-095 query.
func TestBuildOvernightArtifactsQuery_DefaultUnchanged(t *testing.T) {
	off := buildOvernightArtifactsQuery(false)
	if strings.Contains(off, "evergreen") {
		t.Errorf("digest candidate query (exclusion off) must carry NO evergreen reference (default unchanged), got:\n%s", off)
	}
	for _, want := range []string{
		"SELECT title, artifact_type FROM artifacts",
		"WHERE created_at > NOW() - INTERVAL '24 hours'",
		"ORDER BY created_at DESC",
		"LIMIT 20",
	} {
		if !strings.Contains(off, want) {
			t.Errorf("digest candidate query missing pre-spec-095 landmark %q", want)
		}
	}
}

// TestBuildOvernightArtifactsQuery_ExcludesEphemeralAdditively — Properties 2 &
// 3 plus the byte-for-byte additivity invariant. The artifacts table is
// unaliased here, so the predicate uses the bare column name.
func TestBuildOvernightArtifactsQuery_ExcludesEphemeralAdditively(t *testing.T) {
	off := buildOvernightArtifactsQuery(false)
	on := buildOvernightArtifactsQuery(true)
	predicate := evergreen.PoolExclusionSQLPredicate("", true)

	if !strings.Contains(on, "evergreen_score") {
		t.Fatal("digest candidate query (exclusion on) must filter on evergreen_score (R12)")
	}
	if !strings.Contains(on, "evergreen_score IS NULL") {
		t.Error("digest exclusion must keep NULL/not-yet-scored candidates (Principle 9)")
	}
	if !strings.Contains(on, "evergreen_score >= 0") {
		t.Error("digest exclusion must keep present-evergreen (score >= 0) candidates")
	}

	// Byte-for-byte additivity: removing the spliced predicate from ON must
	// reproduce OFF verbatim.
	if got := strings.Replace(on, predicate, "", 1); got != off {
		t.Errorf("exclusion is not purely additive — default (OFF) digest behavior changed.\nON-minus-predicate:\n%q\nOFF:\n%q", got, off)
	}

	t.Run("would_catch_regression", func(t *testing.T) {
		if !strings.Contains(buildOvernightArtifactsQuery(true), "IS NULL") {
			t.Fatal("regression: digest exclusion no longer whitelists NULL (Principle 9 violated)")
		}
	})
}
