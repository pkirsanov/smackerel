// Spec 095 SCOPE-07 / PKT-095-B — durable persistence shape for the evergreen
// signal.
//
// design.md §11 / OQ-PLAN-2 allocates an ADDITIVE artifacts.evergreen_score
// column on the EXISTING artifacts table (never a sibling store — Principle 5).
// The signal is (Evergreen bool, Confidence [0,1], Source); we encode it as a
// single SIGNED score so one nullable REAL column carries BOTH the direction
// and the confidence:
//
//	score = +Confidence when Evergreen   (>= 0 ⇒ judged evergreen)
//	score = -Confidence when ephemeral   (<  0 ⇒ judged ephemeral)
//	NULL  (column absent)                (not yet scored ⇒ evergreen, Principle 9)
//
// A NULL/absent score MUST be treated as "not excluded" downstream (Principle 9
// — no wrongful exclusion); PoolExcludedByPersistedScore encodes that contract.
// artifacts.evergreen_source is persisted alongside for Principle 8 provenance.
//
// This file is the SINGLE owner of the score⇄signal encoding so the ingestion
// writer (internal/pipeline) and the SCOPE-08 pool-exclusion reader (PKT-095-C)
// never drift.
package evergreen

// PersistedScore encodes the signal for the additive artifacts.evergreen_score
// column: +Confidence when evergreen, -Confidence when ephemeral. The sign is
// the judgment; the magnitude is the calibrated confidence.
func (s EvergreenSignal) PersistedScore() float64 {
	if s.Evergreen {
		return s.Confidence
	}
	return -s.Confidence
}

// EvergreenFromPersistedScore reconstructs the evergreen judgment from a stored
// score: >= 0 is evergreen, < 0 is ephemeral. Callers handle a NULL
// (not-yet-scored) column as evergreen/not-excluded BEFORE calling this
// (Principle 9) — see PoolExcludedByPersistedScore.
func EvergreenFromPersistedScore(score float64) bool { return score >= 0 }

// PoolExcludedByPersistedScore reports whether a persisted (possibly NULL)
// evergreen score excludes an artifact from a synthesis/digest pool when the
// SST exclusion switch is on. The Principle-9 invariant is explicit:
//
//   - exclusion switch off            ⇒ never excluded
//   - NULL score (scorePresent=false) ⇒ never excluded (not yet scored ⇒ evergreen)
//   - present score >= 0 (evergreen)  ⇒ never excluded
//   - present score <  0 (ephemeral)  ⇒ excluded
//
// Exclusion is pool-eligibility ONLY; the artifact stays fully searchable
// (R13). This is the downstream-facing reader the SCOPE-08 builder adapters
// (PKT-095-C) consult once the persisted column is read back.
func PoolExcludedByPersistedScore(scorePresent bool, score float64, excludeLowEvergreen bool) bool {
	if !excludeLowEvergreen || !scorePresent {
		return false
	}
	return !EvergreenFromPersistedScore(score)
}

// PoolExclusionSQLPredicate returns the ADDITIVE SQL WHERE-clause fragment that
// excludes persisted-ephemeral artifacts (evergreen_score present AND < 0) from
// a synthesis (§10) / digest (§12) CANDIDATE pool, or "" when exclusion is
// disabled. It is the SQL twin of PoolExcludedByPersistedScore — both live in
// this file so the persisted-score writer (internal/pipeline) and the SCOPE-08
// pool-exclusion readers (internal/intelligence synthesis + internal/digest)
// never drift on the score⇄signal encoding.
//
//   - excludeLowEvergreen == false ⇒ ""  — the host query is byte-for-byte
//     unchanged (the shipped default; safe additive activation).
//   - excludeLowEvergreen == true  ⇒ " AND (<col> IS NULL OR <col> >= 0)".
//
// The predicate keeps NULL (not-yet-scored ⇒ evergreen, Principle 9) and
// present-evergreen (score >= 0) rows; it drops only present-ephemeral
// (score < 0) rows. columnQualifier is the artifacts-table alias in the host
// query — "a" for `FROM artifacts a`, "" for an unaliased `FROM artifacts`.
//
// EXCLUSION IS POOL-ELIGIBILITY ONLY (R13): this fragment is NEVER applied on
// the §9.2 search/retrieval path, so an artifact dropped from a pool stays
// fully searchable. The fragment carries no SQL placeholders, so it composes
// safely into a parameterized query without shifting positional args.
func PoolExclusionSQLPredicate(columnQualifier string, excludeLowEvergreen bool) string {
	if !excludeLowEvergreen {
		return ""
	}
	col := "evergreen_score"
	if columnQualifier != "" {
		col = columnQualifier + ".evergreen_score"
	}
	return " AND (" + col + " IS NULL OR " + col + " >= 0)"
}
