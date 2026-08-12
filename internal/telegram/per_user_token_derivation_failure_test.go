// Spec 108 Scope 04 — design.md §10.10 layer **T3**: the derivation-FAILURE
// path of the Telegram bridge mint.
//
// WHY THIS FILE EXISTS. Before it, `ErrPrincipalGrantsUnrecorded` and
// `ErrNoDelegableGrant` were DEFINED and RETURNED in production and asserted
// NOWHERE — `grep -rn 'ErrPrincipalGrantsUnrecorded|ErrNoDelegableGrant'
// --include='*_test.go' .` returned zero hits. An error path with no test can
// be deleted, or softened into a silent empty mint, and every other test in
// this package still passes: they all seed a fully-granted principal, so they
// only ever traverse the success arm. A scopeless-but-VALID token is the worst
// possible outcome here — it authenticates, so nothing looks broken, and then
// fails every gated call. That is precisely the "silent default" §18 decision
// 3 forbids.
//
// WHAT IS ASSERTED, and why each part is load-bearing:
//
//  1. THE SENTINEL, via errors.Is. Not a substring match on the message: the
//     reply site branches on the sentinel to decide whether to render a
//     permanent operator-actionable condition or a transient retry, so the
//     identity of the error is the contract, not its wording.
//
//  2. NULL ≠ '{}'. Migration 063 deliberately gives `granted_scopes` no
//     default so UNKNOWN (`NULL`, Recorded=false) stays separable from
//     RECORDED-AS-NONE (`'{}'`, Recorded=true, len 0). Both deny, but the
//     operator remedies differ (rotate the token to record grants, versus
//     re-issue with a grant set), so collapsing them makes the diagnostic
//     useless. Each case therefore asserts BOTH that it reaches its own
//     sentinel AND that it does NOT reach the other one — a conflating
//     implementation that mapped everything to one error would pass a
//     positive-only assertion.
//
//  3. AN UNDERLYING READER ERROR PROPAGATES UNCONVERTED. `auth.
//     ErrPrincipalNotProvisioned` is a THIRD distinct state (no active
//     unexpired token at all). Swallowing it into either bridge sentinel
//     would misreport a provisioning failure as a grant failure, so it is
//     asserted to survive `errors.Is` AND to match neither sentinel.
//
//  4. THE RETURNED TOKEN IS THE ZERO VALUE, checked field by field with
//     `WireToken == ""` called out explicitly. A partial mint that populated
//     UserID/TokenID but left the wire empty, or vice versa, is the shape a
//     careless refactor produces; asserting the whole struct is zero closes
//     both.
//
//  5. NO FALLBACK BEARER. Every failure case is driven through `MintForChat`
//     — the production entry point — as well as `MintForUser`, because
//     `MintForChat` is where a "fall back to the shared token" branch would
//     be added. In PRODUCTION a mapped chat whose derivation fails MUST
//     error; it must NOT take the (zero, nil) dev/test unmapped path, which
//     is the caller's signal to use the legacy shared bearer.
//
//  6. NON-VACUITY. `TestDeriveGrants_SuccessArmStillMints` runs the identical
//     harness with a delegable recorded set and requires a real token. Without
//     it, a minter that refused EVERY mint would satisfy every assertion above
//     — "always deny" and "denies correctly" would be indistinguishable.
//
// This file writes no scope literal; the adversarial recorded sets are built
// from `auth.*` constants, per the T1 guard in `scope_literal_guard_test.go`.
package telegram

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

// derivationFailureChatID is the mapped chat every case below drives. The
// mapping is deliberately POPULATED, so a refusal can only come from
// derivation — not from `ErrNoUserMappingForChat`, which is a different
// failure with its own tests.
const derivationFailureChatID int64 = 776644

// derivationFailureUser is the principal the chat maps to.
const derivationFailureUser = "tg-derivation-principal"

// errReaderUnavailable stands in for a transport-level read failure (pool
// down, context cancelled) that is neither of the bridge sentinels and
// neither of auth's own sentinels.
var errReaderUnavailable = errors.New("telegram-test: grant reader unavailable")

// newDerivationMinter builds a production-environment minter over the
// supplied reader, with the chat→user mapping already populated.
func newDerivationMinter(t *testing.T, reader PrincipalGrantReader) (*PerUserTokenMinter, string) {
	t.Helper()
	priv, pub := auth.GenerateSigningKeypair()
	m, err := NewPerUserTokenMinter(PerUserTokenMinterOptions{
		Bot:             &Bot{environment: "production", userMapping: map[int64]string{derivationFailureChatID: derivationFailureUser}},
		PrincipalGrants: reader,
		SigningKey:      priv,
		KeyID:           "scope04-t3-key",
		Issuer:          "smackerel",
		TTL:             2 * time.Minute,
		Now:             func() time.Time { return time.Unix(1_700_000_000, 0).UTC() },
	})
	if err != nil {
		t.Fatalf("NewPerUserTokenMinter: %v", err)
	}
	return m, pub
}

// assertZeroMintedToken checks the ENTIRE MintedTelegramToken is the zero
// value, naming WireToken first because an empty wire token is the specific
// thing that stops a scopeless credential from escaping.
func assertZeroMintedToken(t *testing.T, label string, tok MintedTelegramToken) {
	t.Helper()
	if tok.WireToken != "" {
		t.Errorf("%s: WireToken=%q — a failed derivation MUST NOT emit a credential; a scopeless-but-valid token authenticates and then fails every gated call, which is the silent default spec 108 §18 decision 3 forbids", label, tok.WireToken)
	}
	if tok != (MintedTelegramToken{}) {
		t.Errorf("%s: MintedTelegramToken=%+v want the zero value — a partially-populated mint is a half-committed credential", label, tok)
	}
}

// derivationFailureCase is one adversarial reader state plus the sentinel it
// MUST produce and the sentinels it MUST NOT be confused with.
type derivationFailureCase struct {
	name   string
	reader *fakePrincipalGrantReader
	// want is the sentinel errors.Is MUST match.
	want error
	// mustNotMatch are the sentinels this state MUST stay distinguishable
	// from. Populated for every case, because "denies" is not the property
	// under test — "denies with the right diagnosis" is.
	mustNotMatch []error
	why          string
}

func derivationFailureCases() []derivationFailureCase {
	return []derivationFailureCase{
		{
			name: "granted_scopes IS NULL (UNKNOWN)",
			reader: &fakePrincipalGrantReader{grants: auth.RecordedGrants{
				TokenID:  "tok-grants-null",
				Recorded: false,
			}},
			want:         ErrPrincipalGrantsUnrecorded,
			mustNotMatch: []error{ErrNoDelegableGrant, auth.ErrPrincipalNotProvisioned},
			why:          "the standing token predates grant recording; the remedy is ROTATION, which is a different operator action from re-issuing with a grant set",
		},
		{
			name: "granted_scopes = '{}' (RECORDED AS NONE)",
			reader: &fakePrincipalGrantReader{grants: auth.RecordedGrants{
				TokenID:  "tok-grants-empty",
				Scopes:   []string{},
				Recorded: true,
			}},
			want:         ErrNoDelegableGrant,
			mustNotMatch: []error{ErrPrincipalGrantsUnrecorded, auth.ErrPrincipalNotProvisioned},
			why:          "the operator deliberately recorded NO grants; that is a determinate answer, not an unknown one — migration 063 has no DEFAULT precisely so this stays separable from NULL",
		},
		{
			name: "recorded, but nothing inside the delegation ceiling",
			reader: &fakePrincipalGrantReader{grants: auth.RecordedGrants{
				TokenID: "tok-grants-outside-ceiling",
				// Real product grants, none of which the bridge may
				// delegate. recorded ∩ ceiling = ∅.
				Scopes:   []string{auth.GrantAssistantTurn, auth.GrantKnowledgeGraphRead, auth.GrantOperatorAdmin},
				Recorded: true,
			}},
			want:         ErrNoDelegableGrant,
			mustNotMatch: []error{ErrPrincipalGrantsUnrecorded, auth.ErrPrincipalNotProvisioned},
			why:          "the principal holds real authority, just none the bridge may carry; the ceiling withholds and the mint must refuse rather than emit an empty claim",
		},
		{
			name: "reader reports the principal has no active token",
			reader: &fakePrincipalGrantReader{
				err: auth.ErrPrincipalNotProvisioned,
			},
			want:         auth.ErrPrincipalNotProvisioned,
			mustNotMatch: []error{ErrPrincipalGrantsUnrecorded, ErrNoDelegableGrant},
			why:          "a THIRD distinct state; converting it into a grant sentinel would send the operator to rotate a token that does not exist",
		},
		{
			name: "reader fails at the transport level",
			reader: &fakePrincipalGrantReader{
				err: errReaderUnavailable,
			},
			want:         errReaderUnavailable,
			mustNotMatch: []error{ErrPrincipalGrantsUnrecorded, ErrNoDelegableGrant, auth.ErrPrincipalNotProvisioned},
			why:          "a transient infrastructure failure must not be laundered into a permanent operator-actionable grant condition, which the reply site would render as 'do not retry'",
		},
	}
}

// TestMintForUser_DerivationFailure_ReturnsSentinelAndZeroToken is T3 on the
// direct entry point.
func TestMintForUser_DerivationFailure_ReturnsSentinelAndZeroToken(t *testing.T) {
	for _, tc := range derivationFailureCases() {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newDerivationMinter(t, tc.reader)

			tok, err := m.MintForUser(context.Background(), derivationFailureChatID, derivationFailureUser)
			if err == nil {
				t.Fatalf("MintForUser returned nil error — %s", tc.why)
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("errors.Is(err, %v) = false; err=%v — %s", tc.want, err, tc.why)
			}
			for _, other := range tc.mustNotMatch {
				if errors.Is(err, other) {
					t.Errorf("err also matches %v — the states MUST stay distinguishable; %s", other, tc.why)
				}
			}
			assertZeroMintedToken(t, "MintForUser/"+tc.name, tok)
		})
	}
}

// TestMintForChat_DerivationFailure_RefusesWithoutFallbackBearer is the same
// matrix driven through the PRODUCTION entry point.
//
// `MintForChat` has a legitimate (zero, nil) return — the dev/test UNMAPPED
// chat, whose caller then falls back to the legacy shared bearer. This test
// pins that a PRODUCTION mapped chat with a failed derivation does NOT reach
// that shape: a nil error here would silently route the bridge onto the shared
// operator bearer, which carries far more authority than the principal holds
// and bypasses the grant model entirely.
func TestMintForChat_DerivationFailure_RefusesWithoutFallbackBearer(t *testing.T) {
	for _, tc := range derivationFailureCases() {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newDerivationMinter(t, tc.reader)

			tok, err := m.MintForChat(context.Background(), derivationFailureChatID)
			if err == nil {
				t.Fatalf("MintForChat returned nil error for a MAPPED production chat — that is the dev/test 'fall back to the shared bearer' signal, so a failed derivation would silently borrow operator authority. %s", tc.why)
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("errors.Is(err, %v) = false; err=%v — %s", tc.want, err, tc.why)
			}
			for _, other := range tc.mustNotMatch {
				if errors.Is(err, other) {
					t.Errorf("err also matches %v — the states MUST stay distinguishable; %s", other, tc.why)
				}
			}
			assertZeroMintedToken(t, "MintForChat/"+tc.name, tok)
		})
	}
}

// TestDeriveGrants_UnknownAndRecordedAsNoneAreNotConflated states the
// migration-063 distinction as its own named assertion rather than leaving it
// implicit in the table above.
//
// The two inputs differ ONLY in the `Recorded` flag — same token id, same
// (empty) scope slice semantics — so any divergence in the resulting sentinel
// is attributable to that flag alone. An implementation that dropped the flag
// and inferred UNKNOWN from `len(Scopes) == 0` would map both to one sentinel
// and fail here, while still passing every success-path test in this package.
func TestDeriveGrants_UnknownAndRecordedAsNoneAreNotConflated(t *testing.T) {
	unknown, _ := newDerivationMinter(t, &fakePrincipalGrantReader{grants: auth.RecordedGrants{
		TokenID:  "tok-shared-id",
		Recorded: false,
	}})
	recordedNone, _ := newDerivationMinter(t, &fakePrincipalGrantReader{grants: auth.RecordedGrants{
		TokenID:  "tok-shared-id",
		Scopes:   []string{},
		Recorded: true,
	}})

	_, unknownErr := unknown.MintForUser(context.Background(), derivationFailureChatID, derivationFailureUser)
	_, noneErr := recordedNone.MintForUser(context.Background(), derivationFailureChatID, derivationFailureUser)

	if unknownErr == nil || noneErr == nil {
		t.Fatalf("both states MUST deny; unknownErr=%v recordedNoneErr=%v", unknownErr, noneErr)
	}
	if errors.Is(unknownErr, ErrNoDelegableGrant) {
		t.Errorf("UNKNOWN (granted_scopes IS NULL) reached ErrNoDelegableGrant — that reports a determinate 'holds nothing' for a state where nothing is known, and sends the operator to re-issue instead of rotate")
	}
	if errors.Is(noneErr, ErrPrincipalGrantsUnrecorded) {
		t.Errorf("RECORDED-AS-NONE (granted_scopes = '{}') reached ErrPrincipalGrantsUnrecorded — that reports an unknown for a state the operator determined, and sends them to rotate a token that is already recorded")
	}
	if errors.Is(unknownErr, ErrNoDelegableGrant) == errors.Is(noneErr, ErrNoDelegableGrant) {
		t.Errorf("the two states produced indistinguishable diagnoses (unknown=%v, recorded-none=%v); migration 063 omits a column DEFAULT specifically so they stay separable", unknownErr, noneErr)
	}
}

// TestDeriveGrants_SuccessArmStillMints is the NON-VACUITY control for this
// whole file.
//
// Every other test here asserts a refusal. A minter that refused
// unconditionally — the exact regression a careless "fail closed harder"
// change produces — would satisfy all of them. This case runs the identical
// harness with a delegable recorded set and demands a real, non-empty wire
// token, so "denies correctly" is separated from "denies always".
func TestDeriveGrants_SuccessArmStillMints(t *testing.T) {
	m, _ := newDerivationMinter(t, &fakePrincipalGrantReader{grants: auth.RecordedGrants{
		TokenID:  "tok-delegable",
		Scopes:   append([]string(nil), auth.TelegramBridgeDelegableGrants...),
		Recorded: true,
	}})

	tok, err := m.MintForChat(context.Background(), derivationFailureChatID)
	if err != nil {
		t.Fatalf("a principal holding the full delegation ceiling MUST mint; err=%v — if this fails, every refusal assertion in this file is vacuous", err)
	}
	if tok.WireToken == "" {
		t.Fatal("WireToken empty on the success arm — the refusal assertions above prove nothing if no input can ever mint")
	}
	if tok.UserID != derivationFailureUser {
		t.Errorf("UserID=%q want %q", tok.UserID, derivationFailureUser)
	}
}
