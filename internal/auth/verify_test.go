package auth

import (
	"errors"
	"testing"
	"time"

	"aidanwoods.dev/go-paseto"
)

// derivePublicHexT is the shared helper for verify_test.go and
// issue_test.go that converts a hex-encoded V4 secret key into the hex
// of its corresponding public key. Tests use it to avoid persisting
// keypairs separately.
func derivePublicHexT(t *testing.T, privHex string) string {
	t.Helper()
	secret, err := paseto.NewV4AsymmetricSecretKeyFromHex(privHex)
	if err != nil {
		t.Fatalf("derive public hex: parse private: %v", err)
	}
	return secret.Public().ExportHex()
}

// T1-05 — VerifyAndParse rejects a token whose iat / nbf / exp falls
// outside the configured ClockSkewTolerance window. Specifically we
// prove three failure modes:
//
//	(a) expired token (exp + tol < now) returns ErrTokenExpired.
//	(b) future token (nbf - tol > now) returns ErrTokenNotYetValid.
//	(c) issuer mismatch returns ErrIssuerMismatch.
func TestVerifyAndParse_RejectsExpiredAndFutureAndForeignIssuer(t *testing.T) {
	priv, _ := GenerateSigningKeypair()
	pub := derivePublicHexT(t, priv)

	issuedAt := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return issuedAt }
	issued, err := IssueToken(IssueOptions{
		UserID:     "user-bob",
		TokenID:    "tok-bob-1",
		SigningKey: priv,
		KeyID:      "key-2026-05",
		TTL:        time.Hour,
		Issuer:     "smackerel-test",
		Now:        clock,
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	// (a) expired — clock advanced past exp + tolerance.
	expired := func() time.Time { return issuedAt.Add(2 * time.Hour) }
	_, err = VerifyAndParse(issued.WireToken, VerifyOptions{
		ActivePublicKey:    pub,
		ActiveKeyID:        "key-2026-05",
		Issuer:             "smackerel-test",
		ClockSkewTolerance: 30 * time.Second,
		Now:                expired,
	})
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got: %v", err)
	}

	// (b) future — clock rewound below nbf - tolerance.
	future := func() time.Time { return issuedAt.Add(-2 * time.Hour) }
	_, err = VerifyAndParse(issued.WireToken, VerifyOptions{
		ActivePublicKey:    pub,
		ActiveKeyID:        "key-2026-05",
		Issuer:             "smackerel-test",
		ClockSkewTolerance: 30 * time.Second,
		Now:                future,
	})
	if !errors.Is(err, ErrTokenNotYetValid) {
		t.Errorf("expected ErrTokenNotYetValid, got: %v", err)
	}

	// (c) issuer mismatch — verifier expects different issuer.
	_, err = VerifyAndParse(issued.WireToken, VerifyOptions{
		ActivePublicKey:    pub,
		ActiveKeyID:        "key-2026-05",
		Issuer:             "wrong-issuer",
		ClockSkewTolerance: 30 * time.Second,
		Now:                clock,
	})
	if !errors.Is(err, ErrIssuerMismatch) {
		t.Errorf("expected ErrIssuerMismatch, got: %v", err)
	}

	// Boundary check — token within tolerance window MUST pass even when
	// strictly past exp by < tolerance. Adversarial guarantee that the
	// tolerance branch is reachable.
	withinTol := func() time.Time { return issuedAt.Add(time.Hour + 10*time.Second) }
	_, err = VerifyAndParse(issued.WireToken, VerifyOptions{
		ActivePublicKey:    pub,
		ActiveKeyID:        "key-2026-05",
		Issuer:             "smackerel-test",
		ClockSkewTolerance: 30 * time.Second,
		Now:                withinTol,
	})
	if err != nil {
		t.Errorf("token within tolerance window should validate, got: %v", err)
	}
}

// T1-06 — VerifyAndParse routes validation to the prior signing key
// when the footer kid matches the prior key id, and rejects tokens
// whose kid matches neither active nor prior with ErrUnknownKeyID.
func TestVerifyAndParse_RotationGraceWindow_HonorsPriorKey(t *testing.T) {
	priorPriv, _ := GenerateSigningKeypair()
	priorPub := derivePublicHexT(t, priorPriv)
	activePriv, _ := GenerateSigningKeypair()
	activePub := derivePublicHexT(t, activePriv)

	issuedAt := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return issuedAt.Add(15 * time.Minute) }

	// Token signed by the PRIOR key (in flight at the moment of rotation).
	issuedByPrior, err := IssueToken(IssueOptions{
		UserID:     "user-charlie",
		TokenID:    "tok-charlie-prior",
		SigningKey: priorPriv,
		KeyID:      "key-2026-04",
		TTL:        24 * time.Hour,
		Issuer:     "smackerel-test",
		Now:        func() time.Time { return issuedAt },
	})
	if err != nil {
		t.Fatalf("IssueToken (prior): %v", err)
	}

	parsed, err := VerifyAndParse(issuedByPrior.WireToken, VerifyOptions{
		ActivePublicKey:    activePub,
		ActiveKeyID:        "key-2026-05",
		PriorPublicKey:     priorPub,
		PriorKeyID:         "key-2026-04",
		Issuer:             "smackerel-test",
		ClockSkewTolerance: 30 * time.Second,
		Now:                clock,
	})
	if err != nil {
		t.Fatalf("prior-key validation should succeed within grace window: %v", err)
	}
	if parsed.KeyID != "key-2026-04" {
		t.Errorf("KeyID mismatch: want key-2026-04, got %q", parsed.KeyID)
	}

	// Token signed by an UNKNOWN key id — must be rejected with
	// ErrUnknownKeyID. We mint a fresh keypair under a kid that is
	// neither active nor prior.
	unknownPriv, _ := GenerateSigningKeypair()
	issuedByUnknown, err := IssueToken(IssueOptions{
		UserID:     "user-eve",
		TokenID:    "tok-eve-unknown",
		SigningKey: unknownPriv,
		KeyID:      "key-2026-99-revoked",
		TTL:        24 * time.Hour,
		Issuer:     "smackerel-test",
		Now:        func() time.Time { return issuedAt },
	})
	if err != nil {
		t.Fatalf("IssueToken (unknown): %v", err)
	}

	_, err = VerifyAndParse(issuedByUnknown.WireToken, VerifyOptions{
		ActivePublicKey:    activePub,
		ActiveKeyID:        "key-2026-05",
		PriorPublicKey:     priorPub,
		PriorKeyID:         "key-2026-04",
		Issuer:             "smackerel-test",
		ClockSkewTolerance: 30 * time.Second,
		Now:                clock,
	})
	if !errors.Is(err, ErrUnknownKeyID) {
		t.Errorf("expected ErrUnknownKeyID for foreign kid, got: %v", err)
	}

	// Adversarial — a token signed by the PRIOR key whose kid is forged
	// to match the active kid MUST fail signature verification (NOT pass
	// because the verifier looked up the active public key by kid).
	forgedKid, err := IssueToken(IssueOptions{
		UserID:     "user-mallory",
		TokenID:    "tok-mallory-forged",
		SigningKey: priorPriv,
		KeyID:      "key-2026-05", // claims to be the active kid
		TTL:        24 * time.Hour,
		Issuer:     "smackerel-test",
		Now:        func() time.Time { return issuedAt },
	})
	if err != nil {
		t.Fatalf("IssueToken (forged kid): %v", err)
	}
	_, err = VerifyAndParse(forgedKid.WireToken, VerifyOptions{
		ActivePublicKey:    activePub,
		ActiveKeyID:        "key-2026-05",
		PriorPublicKey:     priorPub,
		PriorKeyID:         "key-2026-04",
		Issuer:             "smackerel-test",
		ClockSkewTolerance: 30 * time.Second,
		Now:                clock,
	})
	if err == nil {
		t.Fatal("forged-kid token MUST fail verification, but it passed")
	}
}

// VerifyAndParse rejects half-rotation state (only one of prior pair set).
func TestVerifyAndParse_RejectsHalfRotationConfig(t *testing.T) {
	priv, _ := GenerateSigningKeypair()
	pub := derivePublicHexT(t, priv)

	// Some valid token to hand in — content is irrelevant because we
	// expect the config check to fire before signature validation.
	issued, err := IssueToken(IssueOptions{
		UserID: "user-dave", TokenID: "t-dave", SigningKey: priv,
		KeyID: "k", TTL: time.Hour, Issuer: "iss",
		Now: func() time.Time { return time.Now() },
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	cases := []struct {
		name        string
		priorPubKey string
		priorKeyID  string
	}{
		{"only-prior-public-set", pub, ""},
		{"only-prior-key-id-set", "", "key-x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := VerifyAndParse(issued.WireToken, VerifyOptions{
				ActivePublicKey:    pub,
				ActiveKeyID:        "k",
				PriorPublicKey:     tc.priorPubKey,
				PriorKeyID:         tc.priorKeyID,
				Issuer:             "iss",
				ClockSkewTolerance: 30 * time.Second,
				Now:                func() time.Time { return time.Now() },
			})
			if err == nil {
				t.Fatal("expected error for half-rotation config, got nil")
			}
		})
	}
}
