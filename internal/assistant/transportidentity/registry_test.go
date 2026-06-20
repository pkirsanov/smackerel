// Spec 072 SCOPE-1 foundation coverage — direct unit tests for the
// reusable TransportIdentityRegistry's phone normalization + HMAC
// hashing.
//
// design.md "Risks & Open Questions" names the mitigation for
// "Identity mismatch from phone normalization" as "one normalization
// function, HMAC lookup tests", and §"Data Model" states that
// "case/whitespace drift produces the same hash" and that the raw
// phone is never echoed in errors (§8 "Security/Compliance"). Before
// this file the package shipped with no test file and those
// properties were exercised only indirectly through the WhatsApp
// adapter (which feeds a single valid +E.164 number). These tests
// pin the normalization, determinism, collision, and leak-avoidance
// properties directly and adversarially.

package transportidentity

import (
	"strings"
	"testing"
)

const testHashKey = "test-identity-hash-key-072"

// HashPhoneE164 MUST require a non-empty key — fail loud rather than
// silently hash under a zero key (NO-DEFAULTS).
func TestHashPhoneE164_EmptyKeyFailsLoud(t *testing.T) {
	if _, err := HashPhoneE164("", "+15555550123"); err == nil {
		t.Fatal("expected error for empty identityHashKey, got nil")
	}
}

// The same normalized phone MUST hash deterministically so the
// lookup key is stable across deliveries.
func TestHashPhoneE164_Deterministic(t *testing.T) {
	a, err := HashPhoneE164(testHashKey, "+15555550123")
	if err != nil {
		t.Fatalf("HashPhoneE164: %v", err)
	}
	b, err := HashPhoneE164(testHashKey, "+15555550123")
	if err != nil {
		t.Fatalf("HashPhoneE164: %v", err)
	}
	if a != b {
		t.Fatalf("non-deterministic hash: %q != %q", a, b)
	}
	if len(a) != 64 { // hex-encoded SHA-256 digest
		t.Fatalf("expected 64-char hex digest, got %d (%q)", len(a), a)
	}
}

// Case/whitespace/missing-"+" drift MUST canonicalize to the SAME
// hash (design §"Data Model"). Adversarial: a no-op normalizer would
// produce distinct hashes for these variants and fail every row.
func TestHashPhoneE164_CanonicalizationCollapsesDrift(t *testing.T) {
	canonical, err := HashPhoneE164(testHashKey, "+15555550123")
	if err != nil {
		t.Fatalf("HashPhoneE164(canonical): %v", err)
	}
	for _, variant := range []string{
		" +15555550123 ", // surrounding whitespace
		"+15555550123",   // identical
		"15555550123",    // missing leading '+'
	} {
		got, err := HashPhoneE164(testHashKey, variant)
		if err != nil {
			t.Fatalf("HashPhoneE164(%q): unexpected err %v", variant, err)
		}
		if got != canonical {
			t.Errorf("variant %q hashed to %q, want canonical %q", variant, got, canonical)
		}
	}
}

// Distinct numbers MUST NOT collide.
func TestHashPhoneE164_DistinctNumbersDiffer(t *testing.T) {
	a, err := HashPhoneE164(testHashKey, "+15555550123")
	if err != nil {
		t.Fatalf("HashPhoneE164(a): %v", err)
	}
	b, err := HashPhoneE164(testHashKey, "+15555550124")
	if err != nil {
		t.Fatalf("HashPhoneE164(b): %v", err)
	}
	if a == b {
		t.Fatal("distinct phone numbers produced identical hash")
	}
}

// Malformed phones MUST be rejected AND the error MUST NOT echo the
// raw phone (design §8 leak avoidance). Adversarial: a naive
// fmt.Errorf("...%s", phone) would leak and fail the substring
// assertion.
func TestHashPhoneE164_MalformedRejectedWithoutLeak(t *testing.T) {
	for _, bad := range []string{
		"",                    // empty
		"+0123456789",         // leading zero after '+'
		"+1",                  // too short
		"not-a-phone",         // non-numeric
		"+123456789012345678", // too long (>15 digits)
	} {
		_, err := HashPhoneE164(testHashKey, bad)
		if err == nil {
			t.Errorf("phone %q: expected rejection, got nil error", bad)
			continue
		}
		if bad != "" && strings.Contains(err.Error(), bad) {
			t.Errorf("phone %q leaked into error message %q", bad, err.Error())
		}
	}
}
