package auth

import (
	"context"
	"strings"
	"testing"
)

// Spec 108 design.md §10 (F-108-UX-ROSTER-01) — unit coverage for the
// server-side grant reader primitive.
//
// What these tests can and cannot prove is stated plainly, because
// overclaiming here would be worse than the gap. They exercise the
// three-state TYPE CONTRACT and the SQL STRUCTURE, both of which are
// where the UNKNOWN/'{}' conflation would actually be introduced. They
// do NOT execute the queries — that needs a live Postgres and belongs
// to `./smackerel.sh test integration`, which is out of scope for this
// pass. The structural guards exist precisely because the query text
// cannot otherwise be regression-checked without a database.

// TestDecodeRecordedGrants_ThreeStatesArePairwiseDistinguishable is the
// central test of the primitive. A two-valued return could not pass it:
// UNKNOWN and RECORDED-AS-NONE both carry zero scopes, so any encoding
// that reports "how many scopes" without reporting "was anything
// recorded" collapses rows 1 and 2 into one indistinguishable value.
func TestDecodeRecordedGrants_ThreeStatesArePairwiseDistinguishable(t *testing.T) {
	unknown := decodeRecordedGrants("tok-unknown", false, nil)
	recordedNone := decodeRecordedGrants("tok-none", true, []string{})
	recordedSet := decodeRecordedGrants("tok-set", true, []string{"corpus:read", "annotation:edit"})

	// UNKNOWN (granted_scopes IS NULL).
	if unknown.Recorded {
		t.Errorf("UNKNOWN state: Recorded = true, want false (granted_scopes IS NULL)")
	}
	if len(unknown.Scopes) != 0 {
		t.Errorf("UNKNOWN state: Scopes = %v, want empty (meaningless when not recorded)", unknown.Scopes)
	}

	// RECORDED AS NONE (granted_scopes = '{}').
	if !recordedNone.Recorded {
		t.Errorf("RECORDED-AS-NONE state: Recorded = false, want true ('{}' is a recording)")
	}
	if len(recordedNone.Scopes) != 0 {
		t.Errorf("RECORDED-AS-NONE state: Scopes = %v, want empty", recordedNone.Scopes)
	}
	if recordedNone.Scopes == nil {
		t.Errorf("RECORDED-AS-NONE state: Scopes is nil; Recorded==true MUST imply a non-nil slice so callers cannot reach the UNKNOWN reading from an empty recorded set")
	}

	// RECORDED SET.
	if !recordedSet.Recorded {
		t.Errorf("RECORDED-SET state: Recorded = false, want true")
	}
	if strings.Join(recordedSet.Scopes, ",") != "corpus:read,annotation:edit" {
		t.Errorf("RECORDED-SET state: Scopes = %v, want the exact recorded claim", recordedSet.Scopes)
	}

	// The load-bearing assertion: UNKNOWN and RECORDED-AS-NONE have the
	// same scope count and MUST still be distinguishable. This is the
	// row that fails against a []string-only return type.
	if len(unknown.Scopes) != len(recordedNone.Scopes) {
		t.Fatalf("test precondition broken: UNKNOWN and RECORDED-AS-NONE must both carry zero scopes for this assertion to be meaningful")
	}
	if unknown.Recorded == recordedNone.Recorded {
		t.Errorf("ADVERSARIAL FAILURE: UNKNOWN and RECORDED-AS-NONE are indistinguishable (both Recorded=%v) — spec 108 §10.4 forbids conflating NULL with '{}'", unknown.Recorded)
	}
}

// TestDecodeRecordedGrants_RecordednessComesFromSQLNotSliceShape is the
// adversarial case. An implementation that inferred recordedness from
// "did the scopes slice scan as nil?" would pass every test above and
// fail this one, because here the flag and the slice shape disagree in
// both directions.
func TestDecodeRecordedGrants_RecordednessComesFromSQLNotSliceShape(t *testing.T) {
	// Flag says NOT recorded while a non-empty slice is present. The
	// flag MUST win: granted_scopes IS NULL is the authority.
	notRecorded := decodeRecordedGrants("tok-a", false, []string{"corpus:read"})
	if notRecorded.Recorded {
		t.Errorf("Recorded = true, want false — the SQL grants_recorded projection is authoritative, not the scanned slice")
	}

	// Flag says recorded while the slice is nil (how pgx presents an
	// empty text[] is a driver detail). This MUST be RECORDED-AS-NONE,
	// not UNKNOWN.
	recorded := decodeRecordedGrants("tok-b", true, nil)
	if !recorded.Recorded {
		t.Errorf("Recorded = false, want true — a nil scan target for '{}' MUST NOT downgrade to UNKNOWN")
	}
	if recorded.Scopes == nil {
		t.Errorf("Scopes = nil, want non-nil empty slice for the recorded-as-none state")
	}
}

// TestRecordedGrants_ZeroValueFailsClosed pins the fail-closed
// property of the zero value returned on every reader error path. A
// caller that ignores the error still gets UNKNOWN, which denies,
// rather than a recorded-empty or permissive set.
func TestRecordedGrants_ZeroValueFailsClosed(t *testing.T) {
	var zero RecordedGrants
	if zero.Recorded {
		t.Errorf("zero RecordedGrants: Recorded = true, want false — error paths MUST read as UNKNOWN")
	}
	if len(zero.Scopes) != 0 {
		t.Errorf("zero RecordedGrants: Scopes = %v, want empty", zero.Scopes)
	}
}

// TestGrantsForPrincipal_RejectsEmptyUserIDWithoutQuerying proves the
// guard runs before the pool is touched — the store here has a nil
// pool, so any query attempt would panic rather than return.
func TestGrantsForPrincipal_RejectsEmptyUserIDWithoutQuerying(t *testing.T) {
	s := &BearerStore{}
	got, err := s.GrantsForPrincipal(context.Background(), "")
	if err == nil {
		t.Fatalf("GrantsForPrincipal(\"\") returned nil error, want a refusal")
	}
	if got.Recorded {
		t.Errorf("error path returned Recorded = true, want the fail-closed zero value")
	}
	if len(got.Scopes) != 0 {
		t.Errorf("error path returned Scopes = %v, want empty", got.Scopes)
	}
}

// TestStandingTokenGrantsQuery_PinsTheDefinedPredicate is a structural
// guard in the style of sst_grep_guard_test.go. "Current standing
// token" is DEFINED in spec 108 design.md §10.4, not inferred, and
// dropping any clause silently widens what the reader treats as
// authoritative — an expired or revoked token would start answering.
// The query text is the only place that definition lives, and it
// cannot be regression-checked at unit level any other way.
func TestStandingTokenGrantsQuery_PinsTheDefinedPredicate(t *testing.T) {
	for _, q := range []struct {
		name  string
		query string
	}{
		{"GrantsForPrincipal", standingTokenGrantsQuery},
		{"ListUsersWithGrants", listUsersWithGrantsQuery},
	} {
		normalized := strings.Join(strings.Fields(q.query), " ")

		required := []string{
			"status = 'active'",          // revoked and rotated rows never answer
			"expires_at > now()",         // a lapsed token holds nothing
			"ORDER BY issued_at DESC",    // newest issuance wins
			"token_id DESC",              // deterministic tiebreak
			"LIMIT 1",                    // exactly one standing token
			"granted_scopes IS NOT NULL", // three-state discriminator
		}
		for _, want := range required {
			if !strings.Contains(normalized, want) {
				t.Errorf("%s query missing required clause %q — spec 108 §10.4 defines the standing token as status='active' AND unexpired, newest first", q.name, want)
			}
		}

		// COALESCE over granted_scopes would erase the UNKNOWN state at
		// the exact boundary that exists to preserve it.
		if strings.Contains(strings.ToUpper(normalized), "COALESCE") {
			t.Errorf("ADVERSARIAL FAILURE: %s query uses COALESCE — substituting a value for NULL granted_scopes collapses UNKNOWN into RECORDED-AS-NONE, which spec 108 §10.4 forbids", q.name)
		}
	}
}
