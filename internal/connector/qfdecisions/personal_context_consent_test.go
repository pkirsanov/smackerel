package qfdecisions

import (
	"errors"
	"testing"
	"time"
)

// Spec 041 Scope 7 — vocabulary and tier-ordering unit tests
// (SCN-SM-041-025/026/027). The DB-backed Issue/AtomicConsumeRead
// behavior is exercised by the integration test in
// tests/integration/qf_personal_context_read_test.go (live pgxpool
// required); the unit tests below cover the pure-Go validation
// vocabulary that the handler relies on for its failure matrix.

func TestPersonalContextTierVocabulary(t *testing.T) {
	cases := []struct {
		tier string
		want bool
	}{
		{PersonalContextTierLow, true},
		{PersonalContextTierMedium, true},
		{PersonalContextTierHigh, true},
		{"", false},
		{"ULTRA", false},
		{"LOW", false}, // case sensitive — the vocabulary is lower-case only
	}
	for _, tc := range cases {
		if got := isValidPersonalContextTier(tc.tier); got != tc.want {
			t.Fatalf("isValidPersonalContextTier(%q)=%v want %v", tc.tier, got, tc.want)
		}
	}
}

func TestPersonalContextTierOrdering(t *testing.T) {
	// low < medium < high
	if !PersonalContextTierLessOrEqual(PersonalContextTierLow, PersonalContextTierMedium) {
		t.Fatal("low <= medium should hold")
	}
	if !PersonalContextTierLessOrEqual(PersonalContextTierMedium, PersonalContextTierHigh) {
		t.Fatal("medium <= high should hold")
	}
	if !PersonalContextTierLessOrEqual(PersonalContextTierLow, PersonalContextTierHigh) {
		t.Fatal("low <= high should hold")
	}
	if PersonalContextTierLessOrEqual(PersonalContextTierHigh, PersonalContextTierLow) {
		t.Fatal("high <= low MUST NOT hold (adversarial — reversal would silently widen consent)")
	}
	if PersonalContextTierLessOrEqual(PersonalContextTierMedium, PersonalContextTierLow) {
		t.Fatal("medium <= low MUST NOT hold")
	}
	if PersonalContextTierLessOrEqual("invalid", PersonalContextTierHigh) {
		t.Fatal("invalid tier MUST NOT satisfy any ordering check")
	}
	if PersonalContextTierLessOrEqual(PersonalContextTierLow, "invalid") {
		t.Fatal("invalid ceiling MUST NOT satisfy any ordering check")
	}
}

func TestPersonalContextTierMinimum_CollapsesInvalidToLow(t *testing.T) {
	cases := []struct {
		a, b, want string
	}{
		{PersonalContextTierLow, PersonalContextTierHigh, PersonalContextTierLow},
		{PersonalContextTierHigh, PersonalContextTierMedium, PersonalContextTierMedium},
		{PersonalContextTierMedium, PersonalContextTierMedium, PersonalContextTierMedium},
		// Adversarial — a misconfigured ceiling MUST collapse to the
		// most-restrictive tier so the handler cannot accidentally
		// grant access above what the system actually permits.
		{"garbage", PersonalContextTierHigh, PersonalContextTierLow},
		{PersonalContextTierHigh, "", PersonalContextTierLow},
		{"", "", PersonalContextTierLow},
	}
	for _, tc := range cases {
		if got := PersonalContextTierMinimum(tc.a, tc.b); got != tc.want {
			t.Fatalf("PersonalContextTierMinimum(%q,%q)=%q want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestPersonalContextValidationError_ImplementsError(t *testing.T) {
	err := &PersonalContextValidationError{Code: PersonalContextErrConsentExpired, Message: "expired"}
	if err.Error() == "" {
		t.Fatal("error message is empty")
	}
	var v *PersonalContextValidationError
	if !errors.As(err, &v) {
		t.Fatal("errors.As should resolve *PersonalContextValidationError")
	}
	if v.Code != PersonalContextErrConsentExpired {
		t.Fatalf("Code=%q want %q", v.Code, PersonalContextErrConsentExpired)
	}
}

func TestPersonalContextConsentMaxTTLIs15Minutes(t *testing.T) {
	// Adversarial check — if a future edit raises the TTL ceiling
	// without spec approval, this test fails first.
	if PersonalContextConsentMaxTTL != 15*time.Minute {
		t.Fatalf("PersonalContextConsentMaxTTL=%s, want 15m (SCN-SM-041-025)", PersonalContextConsentMaxTTL)
	}
}

func TestPersonalContextConsentMaxReadsIs5(t *testing.T) {
	// Adversarial check — if a future edit relaxes the rate cap
	// without spec approval, this test fails first.
	if PersonalContextConsentMaxReads != 5 {
		t.Fatalf("PersonalContextConsentMaxReads=%d, want 5 (SCN-SM-041-027)", PersonalContextConsentMaxReads)
	}
}

func TestPersonalContextAuditActionAndOutcomeConstants(t *testing.T) {
	// Adversarial check — the audit action MUST be exactly
	// "personal_context_read" so downstream consumers can filter for
	// Scope 7 reads; the outcome vocabulary MUST be the documented set.
	if AuditActionPersonalContextRead != "personal_context_read" {
		t.Fatalf("AuditActionPersonalContextRead=%q want personal_context_read", AuditActionPersonalContextRead)
	}
	want := map[string]string{
		"degraded":            AuditOutcomeDegraded,
		"rate_limited":        AuditOutcomeRateLimited,
		"capability_disabled": AuditOutcomeCapabilityDisabled,
	}
	for w, got := range want {
		if got != w {
			t.Fatalf("audit outcome constant for %q drifted: got %q", w, got)
		}
	}
}

func TestNewPersonalContextConsentTokenIDHasPctPrefix(t *testing.T) {
	id, err := newPersonalContextConsentTokenID()
	if err != nil {
		t.Fatalf("newPersonalContextConsentTokenID: %v", err)
	}
	if len(id) < 5 || id[:4] != "pct_" {
		t.Fatalf("token id %q does not start with the documented pct_ prefix", id)
	}
	// 32 bytes hex = 64 chars after the pct_ prefix.
	if len(id) != 4+64 {
		t.Fatalf("token id length %d, want %d", len(id), 4+64)
	}
}
