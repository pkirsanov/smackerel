//go:build integration

// TP-01-03 — spec 108 SCOPE-01 integration test (SCN-108-P02).
//
// Proves a `corpus:read` token round-trips through the REAL path against the
// ephemeral test stack's PostgreSQL: issuance → PASETO verify (the step
// bearerAuthMiddleware performs) → Session → grant decision, and that the
// grant the server RECORDED for the principal agrees with the one the token
// carries.
//
// Why the recorded-grant half matters: spec 108 makes granting a token
// ROTATION rather than a flag flip (design.md §5, F-108-GRANT-MECHANISM-01).
// A token that carries `corpus:read` while `auth_tokens.granted_scopes` says
// otherwise would make the operator's roster disagree with what the system
// actually honours, which is the failure the column exists to prevent.
//
// Non-vacuity: the ungranted principal in each test is the control. If
// GateGlobalCorpusRead ever returned Allowed unconditionally — the exact
// regression that would silently open the corpus — the granted assertions
// would still pass and only the ungranted ones would fail. Both directions
// are asserted for that reason.

package integration

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

// corpusGrantFixture enrolls one principal and persists one standing token
// carrying exactly the supplied scope set. Passing a non-nil empty slice
// records "granted nothing", which is a DIFFERENT state from NULL/unknown and
// is asserted as such below.
func corpusGrantFixture(t *testing.T, store *auth.BearerStore, userID string, scopes []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := store.Enroll(ctx, auth.EnrollUserParams{
		UserID:     userID,
		EnrolledBy: "tp-01-03",
		Notes:      "spec 108 TP-01-03 fixture",
	}); err != nil {
		t.Fatalf("enroll %s: %v", userID, err)
	}

	tokenID, err := auth.GenerateTokenID()
	if err != nil {
		t.Fatalf("GenerateTokenID: %v", err)
	}
	now := time.Now()
	if err := store.PersistToken(ctx, auth.PersistTokenParams{
		TokenID:       tokenID,
		UserID:        userID,
		KeyID:         "tp-01-03-kid",
		IssuedAt:      now,
		ExpiresAt:     now.Add(24 * time.Hour),
		HashedToken:   "tp0103-hash-" + tokenID,
		IssuedBy:      "tp-01-03",
		IssuedSource:  "admin_api",
		GrantedScopes: scopes,
	}); err != nil {
		t.Fatalf("persist token for %s: %v", userID, err)
	}
}

// TestCorpusGrant_TokenRoundTripsToAGrantedSession_TP_01_03 is the positive
// half of SCN-108-P02.
func TestCorpusGrant_TokenRoundTripsToAGrantedSession_TP_01_03(t *testing.T) {
	pool := testPool(t)
	store, err := auth.NewBearerStore(pool)
	if err != nil {
		t.Fatalf("NewBearerStore: %v", err)
	}

	priv, pub := auth.GenerateSigningKeypair()
	const kid = "tp-01-03-kid"
	userID := "tp0103-granted-" + time.Now().Format("150405.000000")

	corpusGrantFixture(t, store, userID, []string{auth.GrantGlobalCorpusRead})

	tokenID, err := auth.GenerateTokenID()
	if err != nil {
		t.Fatalf("GenerateTokenID: %v", err)
	}
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     userID,
		TokenID:    tokenID,
		SigningKey: priv,
		KeyID:      kid,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
		Scopes:     []string{auth.GrantGlobalCorpusRead},
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	// The verify step bearerAuthMiddleware performs.
	parsed, err := auth.VerifyAndParse(issued.WireToken, auth.VerifyOptions{
		ActivePublicKey: pub,
		ActiveKeyID:     kid,
		Issuer:          "smackerel",
		Now:             time.Now,
	})
	if err != nil {
		t.Fatalf("VerifyAndParse: %v", err)
	}

	sess := auth.Session{
		UserID: parsed.UserID,
		Scopes: parsed.Scopes,
		Source: auth.SessionSourcePerUserToken,
	}

	if !slices.Contains(sess.Scopes, auth.GrantGlobalCorpusRead) {
		t.Fatalf("session scopes = %v; want to contain %q. The scope claim did not survive issuance → verify, so a granted principal would be denied at the gate", sess.Scopes, auth.GrantGlobalCorpusRead)
	}
	if got := auth.GateGlobalCorpusRead(sess); !got.Allowed {
		t.Errorf("GateGlobalCorpusRead(granted session).Allowed = false; a principal holding %q must be authorized", auth.GrantGlobalCorpusRead)
	}

	// The server-side recorded answer must agree with the token.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rec, err := store.GrantsForPrincipal(ctx, userID)
	if err != nil {
		t.Fatalf("GrantsForPrincipal(%s): %v", userID, err)
	}
	if !rec.Recorded {
		t.Fatalf("recorded grants for %s report UNKNOWN (granted_scopes IS NULL) for a token just persisted WITH a scope set; the roster would show unknown for a principal the system honours", userID)
	}
	if !slices.Contains(rec.Scopes, auth.GrantGlobalCorpusRead) {
		t.Errorf("recorded grants = %v; want to contain %q. The token carries the grant but the server's record disagrees, so the operator roster and the enforced answer would diverge", rec.Scopes, auth.GrantGlobalCorpusRead)
	}
}

// TestCorpusGrant_UngrantedPrincipalIsDeniedAndRecordedAsNone_TP_01_03 is the
// adversarial half. It also pins the distinction the migration comment and
// spec.md §7 forbid conflating: recorded-as-NONE is not UNKNOWN.
func TestCorpusGrant_UngrantedPrincipalIsDeniedAndRecordedAsNone_TP_01_03(t *testing.T) {
	pool := testPool(t)
	store, err := auth.NewBearerStore(pool)
	if err != nil {
		t.Fatalf("NewBearerStore: %v", err)
	}

	priv, pub := auth.GenerateSigningKeypair()
	const kid = "tp-01-03-kid-neg"
	userID := "tp0103-ungranted-" + time.Now().Format("150405.000000")

	// Non-nil empty slice: "we issued this principal nothing", recorded.
	corpusGrantFixture(t, store, userID, []string{})

	tokenID, err := auth.GenerateTokenID()
	if err != nil {
		t.Fatalf("GenerateTokenID: %v", err)
	}
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     userID,
		TokenID:    tokenID,
		SigningKey: priv,
		KeyID:      kid,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	parsed, err := auth.VerifyAndParse(issued.WireToken, auth.VerifyOptions{
		ActivePublicKey: pub,
		ActiveKeyID:     kid,
		Issuer:          "smackerel",
		Now:             time.Now,
	})
	if err != nil {
		t.Fatalf("VerifyAndParse: %v", err)
	}

	sess := auth.Session{
		UserID: parsed.UserID,
		Scopes: parsed.Scopes,
		Source: auth.SessionSourcePerUserToken,
	}

	if got := auth.GateGlobalCorpusRead(sess); got.Allowed {
		t.Errorf("GateGlobalCorpusRead(ungranted session).Allowed = true; an ungranted principal must NOT read the global corpus. If this passes while the positive case also passes, the gate is authorizing unconditionally")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rec, err := store.GrantsForPrincipal(ctx, userID)
	if err != nil {
		t.Fatalf("GrantsForPrincipal(%s): %v", userID, err)
	}
	if !rec.Recorded {
		t.Errorf("recorded grants report UNKNOWN for a principal deliberately issued an EMPTY scope set; NULL (unknown) and '{}' (recorded as none) are different states and conflating them is exactly what spec.md §7 forbids")
	}
	if slices.Contains(rec.Scopes, auth.GrantGlobalCorpusRead) {
		t.Errorf("recorded grants = %v; an empty issuance must not record %q", rec.Scopes, auth.GrantGlobalCorpusRead)
	}
}
