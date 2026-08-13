//go:build integration

// Spec 108 SCOPE-04 — TP-04-02.
//
// The Telegram bridge end of SCN-108-E01, driven through the REAL minter
// against a REAL router backed by a REAL database. No mock minter, no stubbed
// grant reader, no hand-written bearer.
//
// WHY THIS ROW NEEDS THE REAL MINTER
//
// The interesting behaviour is not "does a corpus route accept a token that
// carries corpus:read" — TP-03-* already proves that. It is whether the token
// the BRIDGE ACTUALLY MINTS carries the right authority for the mapped
// principal, and what the operator sees when it does not.
//
// A test that minted its own bearer with corpus:read and called the API would
// pass whether or not deriveGrants worked at all, because it would have
// skipped the one step under test. So these drive
// PerUserTokenMinter.MintForUser and use whatever it returns.
//
// THE FAILURE MODE IS THE POINT
//
// For an unentitled principal, the mint ABORTS with a typed error. The user
// never receives an unexplained 403 from a corpus route, because no request is
// ever made — the bridge fails closed at issuance with a named, permanent,
// operator-actionable condition. That distinction is what the row asks for and
// is asserted below in both directions.
package graphapi_integration

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/telegram"
)

// TestIntegration_CorpusGrant_TelegramBridgeReachesCorpusUnderEnforce_TP_04_02
// covers SCN-108-E01 and its adversarial twin SCN-108-E04 in one place,
// because the two only mean something relative to each other: "the bridge
// works" is satisfied by a bridge that hands everyone full authority, and
// "the bridge refuses" is satisfied by one that is simply broken.
func TestIntegration_CorpusGrant_TelegramBridgeReachesCorpusUnderEnforce_TP_04_02(t *testing.T) {
	stack, privateHex := newCorpusEnforceStack(t, "tp0402")
	base := stack.serve(t, true /* enforce */)

	store, err := auth.NewBearerStore(stack.pool)
	if err != nil {
		t.Fatalf("bearer store: %v", err)
	}

	// The at-rest hashing key MUST differ from the signing key
	// (internal/auth/startup.go). Deriving it from the public half keeps
	// them distinct without inventing a literal.
	atRest := stack.publicHex
	if atRest == "" || atRest == privateHex {
		t.Fatalf("test setup: at-rest key must be non-empty and distinct from the signing key")
	}

	// Two principals whose ONLY difference is the recorded grant set.
	entitled := "tp0402-entitled-principal"
	unentitled := "tp0402-unentitled-principal"

	persist := func(userID string, scopes []string) {
		t.Helper()
		// auth_tokens.user_id is FK-bound to auth_users, so the principal
		// must be enrolled before a token can be persisted against it.
		if err := store.Enroll(context.Background(), auth.EnrollUserParams{
			UserID:     userID,
			EnrolledBy: "spec108-tp0402",
			Notes:      "spec 108 TP-04-02 fixture",
		}); err != nil {
			t.Fatalf("enroll %s: %v", userID, err)
		}
		if _, err := auth.IssueAndPersistToken(context.Background(), store, auth.IssueAndPersistOptions{
			UserID:            userID,
			SigningPrivateKey: privateHex,
			SigningKeyID:      corpusKeyID,
			AtRestHashingKey:  atRest,
			TTL:               time.Hour,
			Issuer:            corpusIssuer,
			Now:               time.Now,
			IssuedBy:          "spec108-tp0402",
			IssuedSource:      "admin_api",
			Scopes:            scopes,
		}); err != nil {
			t.Fatalf("persist grants for %s: %v", userID, err)
		}
	}
	persist(entitled, []string{auth.GrantGlobalCorpusRead, corpusOtherScope})
	persist(unentitled, []string{corpusOtherScope})

	// The real minter. MintForUser does not touch the bot (the chat→user
	// resolve step it skips lives in MintForChat), but the constructor
	// requires a non-nil one, so the binding stays explicit.
	minter, err := telegram.NewPerUserTokenMinter(telegram.PerUserTokenMinterOptions{
		Bot:             &telegram.Bot{},
		PrincipalGrants: store,
		SigningKey:      privateHex,
		KeyID:           corpusKeyID,
		Issuer:          corpusIssuer,
		TTL:             time.Hour,
	})
	if err != nil {
		t.Fatalf("construct per-user token minter: %v", err)
	}

	// SCN-108-E01 — the entitled principal's Telegram corpus commands
	// complete. Several routes, because the Telegram command surface spans
	// search/digest/recent/knowledge and a regression in one of them would
	// hide behind a single-route check.
	t.Run("entitled_principal_completes_corpus_commands", func(t *testing.T) {
		minted, err := minter.MintForUser(context.Background(), 4402, entitled)
		if err != nil {
			t.Fatalf("SCN-108-E01: minting for an ENTITLED principal failed (%v); the bridge must issue a usable token when the principal holds the grant", err)
		}
		if minted.WireToken == "" {
			t.Fatal("SCN-108-E01: mint returned an empty wire token")
		}

		for _, rt := range []corpusEnforceRoute{
			{method: http.MethodGet, path: "/api/recent?limit=1"},
			{method: http.MethodGet, path: "/api/digest"},
			{method: http.MethodPost, path: "/api/search", body: `{"query":"tp0402","limit":1}`},
			{method: http.MethodGet, path: "/api/knowledge/stats"},
		} {
			resp, body := corpusDo(t, base, minted.WireToken, rt)
			// This harness wires the AUTH GATE, not the full service graph,
			// so a downstream 503 (DB / ML / digest / knowledge unavailable)
			// is expected here and is not a failure of this row. What must
			// hold is that the request got PAST authentication AND past the
			// corpus gate. The 2xx proof lives in the e2e lane
			// (TP-04-05 / TP-04-07), where the real stack is up.
			//
			// Both conditions are required. Checking only "not a gate
			// denial" is too weak, because a 401 is not a gate denial
			// either — which is exactly how this test reported green while
			// every route returned 401 and the bridge was entirely broken.
			if resp.StatusCode == http.StatusUnauthorized {
				t.Errorf("SCN-108-E01: %s %s returned 401 for an entitled principal; the bridge-minted token did not even authenticate. body=%s", rt.method, rt.path, body)
				continue
			}
			if isCorpusGateDenial(resp, body) {
				t.Errorf("SCN-108-E01: %s %s was refused by the corpus gate (403) for a principal whose persisted grants include corpus:read; a working Telegram corpus command would now fail. body=%s", rt.method, rt.path, body)
			}
		}
	})

	// SCN-108-E04 — the minter must not confer authority the principal
	// lacks. This is the case a naive "Telegram works" test passes while the
	// real defect ships.
	//
	// The mint SUCCEEDS here, and that is CORRECT: the principal holds a
	// grant the bridge may delegate (annotation:edit), so there is a valid
	// token to issue. E04 does not ask for a mint failure — it asks that the
	// minted token not carry corpus:read and that the corpus command be
	// refused with 403. The assertion is therefore on the route's refusal.
	t.Run("unentitled_principal_gets_a_valid_token_that_cannot_reach_the_corpus", func(t *testing.T) {
		minted, err := minter.MintForUser(context.Background(), 4403, unentitled)
		if err != nil {
			t.Fatalf("mint for a principal holding a delegable non-corpus grant failed (%v); the bridge must still work for its other commands", err)
		}

		// The token is VALID — proven by an ungated route accepting it.
		// Without this, the 403 below could equally mean "broken
		// credential", and the test would credit the gate for what is
		// really a dead bridge.
		ungated := corpusEnforceRoute{method: corpusEnforceUngated[0].method, path: corpusEnforceUngated[0].path}
		if resp, body := corpusDo(t, base, minted.WireToken, ungated); resp.StatusCode == http.StatusUnauthorized {
			t.Fatalf("the minted token failed authentication on ungated %s %s (401); the corpus refusal below would then prove nothing about the gate. body=%s", ungated.method, ungated.path, body)
		}

		// ...and it cannot reach the corpus.
		resp, body := corpusDo(t, base, minted.WireToken, corpusEnforceRoute{method: http.MethodGet, path: "/api/recent?limit=1"})
		if !isCorpusGateDenial(resp, body) {
			t.Fatalf("SCN-108-E04: a bridge token for a principal WITHOUT corpus:read reached /api/recent (status=%d); authority must come from the principal, never from the minter. body=%s", resp.StatusCode, body)
		}

		// The refusal must be PERMANENT. 403 says "you may not"; a
		// transient code would send the bridge into a retry loop for a
		// condition only an operator token-rotation can fix.
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusRequestTimeout {
			t.Errorf("SCN-108-E04: the refusal used a TRANSIENT status (%d); a missing grant is permanent until rotation", resp.StatusCode)
		}
	})

	// The OTHER operator-actionable failure mode: a principal with no
	// delegable grant at all. Here the mint itself aborts, so the user never
	// reaches a corpus route. The row requires this be a NAMED condition
	// rather than an unexplained refusal.
	t.Run("undelegable_principal_aborts_at_mint_with_a_named_condition", func(t *testing.T) {
		unprovisioned := "tp0402-unprovisioned-principal"
		minted, err := minter.MintForUser(context.Background(), 4404, unprovisioned)
		if err == nil {
			t.Fatalf("minting for a principal with no recorded grants SUCCEEDED; absent grant data must deny, never default")
		}
		if minted.WireToken != "" {
			t.Error("mint returned an error AND a non-empty wire token; a scopeless-but-valid credential authenticates and then silently fails every gated call")
		}
		// auth.ErrPrincipalNotProvisioned is the third named condition:
		// deriveGrants wraps it for a principal with no auth_users row at
		// all, which is distinct from "recorded but nothing delegable".
		if !errors.Is(err, telegram.ErrNoDelegableGrant) &&
			!errors.Is(err, telegram.ErrPrincipalGrantsUnrecorded) &&
			!errors.Is(err, auth.ErrPrincipalNotProvisioned) {
			t.Errorf("mint failed with an UNTYPED error (%v); the reply site separates permanent from transient with errors.Is, so an untyped failure gets rendered as 'try again' for a condition retrying can never fix", err)
		}
		if !strings.Contains(err.Error(), unprovisioned) {
			t.Errorf("the mint error does not name the principal (%q); the operator action is 'rotate that principal's token', which is not actionable without knowing which principal", err.Error())
		}
	})

	// The gate itself must still be live on this stack — otherwise the
	// entitled subtest above would pass on a router where nothing is
	// enforced, and this whole file would be vacuous.
	t.Run("canary_gate_is_actually_enforcing", func(t *testing.T) {
		bare := mintCorpusToken(t, privateHex, "tp0402-canary-no-grant", []string{corpusOtherScope})
		resp, body := corpusDo(t, base, bare, corpusEnforceRoute{method: http.MethodGet, path: "/api/recent?limit=1"})
		if !isCorpusGateDenial(resp, body) {
			t.Fatalf("canary: a bearer WITHOUT corpus:read reached /api/recent (status=%d) on a stack booted with enforce=true; the gate is not active, so every other assertion in this file is vacuous. body=%s", resp.StatusCode, body)
		}
	})
}
