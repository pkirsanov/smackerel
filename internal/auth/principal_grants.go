package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrPrincipalNotProvisioned is returned by GrantsForPrincipal when the
// principal has no current standing token — no auth_tokens row that is
// both status='active' and unexpired. It is deliberately distinct from
// "the standing token records no grants" ('{}') and from "the standing
// token predates grant recording" (NULL): all three deny, but they
// demand different operator remedies (provision, re-issue with scopes,
// rotate to record), so collapsing them would make the diagnostic
// useless. See spec 108 design.md §10.8.
var ErrPrincipalNotProvisioned = errors.New("auth: principal has no active unexpired token")

// RecordedGrants is the server-side answer to "what does this principal
// hold?", readable WITHOUT possessing the wire token. It is the
// primitive spec 108 design.md §10.1 identifies as absent and §10.4
// specifies.
//
// The type is deliberately THREE-valued, not two-valued, because
// auth_tokens.granted_scopes is three-valued (migration 063):
//
//	Recorded == false                  → granted_scopes IS NULL.
//	                                     UNKNOWN. The token was issued
//	                                     before grant recording existed.
//	                                     Scopes is meaningless and nil.
//	Recorded == true, len(Scopes) == 0 → granted_scopes = '{}'.
//	                                     RECORDED AS NONE. The token was
//	                                     issued with no scope claim.
//	Recorded == true, len(Scopes) > 0  → the exact claim in the token.
//
// A plain []string return could not express this: nil would have to
// mean both "nobody recorded" and "recorded as no grants", which is
// exactly the conflation the migration comment and spec.md S7 forbid.
// The Recorded flag is what keeps UNKNOWN separable from empty.
//
// The zero value reads as UNKNOWN with no scopes, so a caller that
// ignores an error and uses the returned value still denies rather
// than permitting.
type RecordedGrants struct {
	// TokenID is the standing token the grants were read from. Set
	// even when Recorded is false, so an operator diagnostic can name
	// the specific token that needs rotating.
	TokenID string

	// Scopes is meaningful ONLY when Recorded is true. It is non-nil
	// whenever Recorded is true, including the recorded-as-none case.
	Scopes []string

	// Recorded is false when granted_scopes IS NULL.
	Recorded bool
}

// EnrolledUserGrants is an enrolled principal joined to the recorded
// grant set of its current standing token. Used by `auth list-users`
// (spec 108 design.md §10.9).
//
// HasStandingToken is a FOURTH display state, distinct from the three
// RecordedGrants states: a principal with no active unexpired token
// holds nothing, and that is a known fact rather than an unknown one.
// Rendering it as "unknown" would misreport a determinate answer.
type EnrolledUserGrants struct {
	EnrolledUser

	// Grants is meaningful only when HasStandingToken is true.
	Grants RecordedGrants

	// HasStandingToken is false when the principal has no auth_tokens
	// row that is both status='active' and unexpired.
	HasStandingToken bool
}

// standingTokenGrantsQuery reads the recorded grant set of ONE
// principal's current standing token.
//
// "Current standing token" is defined, not inferred (spec 108 design.md
// §10.4): the row for user_id with status='active' AND expires_at >
// now(), ordered issued_at DESC, token_id DESC, limit 1. Rotation moves
// the prior token to 'rotated', so the newest active row is the
// operator's latest issuance. Expired and revoked rows are excluded, so
// a principal whose token lapsed holds nothing and every consumer fails
// closed.
//
// granted_scopes IS NOT NULL is projected as its own column rather than
// inferred from a nil scan target: the NULL/'{}' distinction is the
// entire point of the column, and leaving it to driver-level nil-vs-
// empty-slice semantics would make the three states depend on an
// encoding detail. IS NOT NULL never itself yields NULL, so the column
// is always a definite boolean.
//
// No COALESCE. Substituting a value for NULL here would erase the
// UNKNOWN state at the exact boundary that exists to preserve it.
//
// No new index: ix_auth_tokens_user_id already covers the predicate.
const standingTokenGrantsQuery = `
    SELECT token_id,
           (granted_scopes IS NOT NULL) AS grants_recorded,
           granted_scopes
    FROM auth_tokens
    WHERE user_id = $1
      AND status = 'active'
      AND expires_at > now()
    ORDER BY issued_at DESC, token_id DESC
    LIMIT 1
`

// listUsersWithGrantsQuery is standingTokenGrantsQuery applied to every
// enrolled principal in one round trip via LEFT JOIN LATERAL, so
// `auth list-users` does not issue N+1 reads. The lateral subquery
// carries the identical standing-token predicate; a principal with no
// standing token yields NULL token_id, which is the HasStandingToken
// discriminator.
const listUsersWithGrantsQuery = `
    SELECT u.user_id, u.enrolled_at, u.enrolled_by, u.status, u.notes,
           t.token_id,
           (t.granted_scopes IS NOT NULL) AS grants_recorded,
           t.granted_scopes
    FROM auth_users u
    LEFT JOIN LATERAL (
        SELECT token_id, granted_scopes
        FROM auth_tokens
        WHERE user_id = u.user_id
          AND status = 'active'
          AND expires_at > now()
        ORDER BY issued_at DESC, token_id DESC
        LIMIT 1
    ) t ON TRUE
    ORDER BY u.enrolled_at ASC, u.user_id ASC
`

// decodeRecordedGrants builds a RecordedGrants from a scanned row.
//
// grantsRecorded comes from the SQL projection, never from inspecting
// whether scopes scanned as nil, so the UNKNOWN/'{}' distinction does
// not ride on driver nil-vs-empty-slice behavior. When the grants ARE
// recorded, Scopes is normalized to non-nil so callers can rely on
// "Recorded implies Scopes is non-nil" and cannot accidentally reach
// the UNKNOWN reading from an empty recorded set.
func decodeRecordedGrants(tokenID string, grantsRecorded bool, scopes []string) RecordedGrants {
	if !grantsRecorded {
		return RecordedGrants{TokenID: tokenID}
	}
	if scopes == nil {
		scopes = []string{}
	}
	return RecordedGrants{TokenID: tokenID, Scopes: scopes, Recorded: true}
}

// GrantsForPrincipal returns the recorded grant set of the principal's
// current standing token, server-side and without the wire token. This
// is the primitive spec 108 design.md §10.1 requires.
//
// Returns ErrPrincipalNotProvisioned (wrapped) when the principal has
// no active unexpired token.
//
// Fails closed. Every error path returns the zero RecordedGrants, which
// reads as UNKNOWN — never an empty-but-recorded set and never a
// permissive one. A caller that mistakes the error for success still
// gets a value that denies.
func (s *BearerStore) GrantsForPrincipal(ctx context.Context, userID string) (RecordedGrants, error) {
	if userID == "" {
		return RecordedGrants{}, errors.New("auth: GrantsForPrincipal requires userID")
	}

	var (
		tokenID        string
		grantsRecorded bool
		scopes         []string
	)
	err := s.pool.QueryRow(ctx, standingTokenGrantsQuery, userID).
		Scan(&tokenID, &grantsRecorded, &scopes)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RecordedGrants{}, fmt.Errorf("auth: grants for principal %q: %w", userID, ErrPrincipalNotProvisioned)
		}
		return RecordedGrants{}, fmt.Errorf("auth: grants for principal %q: %w", userID, err)
	}

	return decodeRecordedGrants(tokenID, grantsRecorded, scopes), nil
}

// ListUsersWithGrants returns every enrolled user, ordered identically
// to ListUsers, joined to the recorded grant set of its current
// standing token.
//
// Deliberately a separate method rather than a widening of ListUsers:
// the admin REST surface (internal/api/auth_handlers.go) reads
// ListUsers and is explicitly out of scope here — grant rendering there
// is owned by F-108-UX-ADMINUI-01 (spec 108 design.md §10.9).
//
// Fails closed: any read or scan error returns a nil slice and the
// error, never a partial roster that would under-report grants.
func (s *BearerStore) ListUsersWithGrants(ctx context.Context) ([]EnrolledUserGrants, error) {
	rows, err := s.pool.Query(ctx, listUsersWithGrantsQuery)
	if err != nil {
		return nil, fmt.Errorf("auth: list users with grants: %w", err)
	}
	defer rows.Close()

	var out []EnrolledUserGrants
	for rows.Next() {
		var (
			u              EnrolledUserGrants
			tokenID        *string
			grantsRecorded bool
			scopes         []string
		)
		if scanErr := rows.Scan(
			&u.UserID, &u.EnrolledAt, &u.EnrolledBy, &u.Status, &u.Notes,
			&tokenID, &grantsRecorded, &scopes,
		); scanErr != nil {
			return nil, fmt.Errorf("auth: scan user grants row: %w", scanErr)
		}
		if tokenID != nil {
			u.HasStandingToken = true
			u.Grants = decodeRecordedGrants(*tokenID, grantsRecorded, scopes)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auth: iterate user grants rows: %w", err)
	}
	return out, nil
}
