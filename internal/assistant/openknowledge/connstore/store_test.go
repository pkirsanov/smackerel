// Spec 096 SCOPE-06 — pure unit tests for the effective-enabled predicate, the
// SINGLE runtime-plane gate SCOPE-03 dispatch and SCOPE-04 discovery both
// consult. No DB: the predicate is a pure function over a Record + the
// registry-declared flag, so the truth table (and its adversarial controls) is
// exercised without an ephemeral Postgres. The DB-backed read/write paths are
// proven by the deferred integration leg (clean stack, C7).
package connstore

import (
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/connvault"
)

// withCredential returns a Record carrying a minimal at-rest credential (the
// cipher columns present). The bytes are synthetic — never a real secret.
func withCredential(rec Record) Record {
	rec.Secret = &connvault.VaultRecord{
		ConnectionID: rec.ConnectionID,
		Kind:         rec.ProviderKind,
		Ciphertext:   []byte{0x01, 0x02, 0x03, 0x04},
		Nonce:        []byte{0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		KeyVersion:   1,
		Redaction:    "…wxyz",
	}
	return rec
}

// TestEffectiveEnabled_SingleGate_Spec096 pins the four-way conjunction that is
// the SINGLE effective-enabled gate (design §5.1): registry-declared AND DB
// enabled AND last_test_outcome=ok AND a credential present. The passing
// control proves the predicate admits a fully-configured connection; each
// adversarial row flips exactly ONE condition and MUST flip the verdict to
// false — non-tautological (a predicate that ignored any condition would fail
// the matching row).
func TestEffectiveEnabled_SingleGate_Spec096(t *testing.T) {
	base := Record{ConnectionID: "anthropic-primary", ProviderKind: "anthropic", Enabled: true, LastTestOutcome: TestOutcomeOK}

	// CONTROL — every condition satisfied ⇒ effective-enabled.
	if !EffectiveEnabled(withCredential(base), true) {
		t.Fatal("CONTROL: a registry-declared, DB-enabled, tested-ok, credentialed connection MUST be effective-enabled")
	}

	cases := []struct {
		name             string
		rec              Record
		registryDeclared bool
	}{
		{
			name:             "not registry-declared (UI-invented slot)",
			rec:              withCredential(base),
			registryDeclared: false,
		},
		{
			name:             "DB disabled (operator toggled off)",
			rec:              withCredential(Record{ConnectionID: "anthropic-primary", ProviderKind: "anthropic", Enabled: false, LastTestOutcome: TestOutcomeOK}),
			registryDeclared: true,
		},
		{
			name:             "last test failed (never enable an unverified connection)",
			rec:              withCredential(Record{ConnectionID: "anthropic-primary", ProviderKind: "anthropic", Enabled: true, LastTestOutcome: TestOutcomeFailed}),
			registryDeclared: true,
		},
		{
			name:             "untested (last_test_outcome empty)",
			rec:              withCredential(Record{ConnectionID: "anthropic-primary", ProviderKind: "anthropic", Enabled: true, LastTestOutcome: ""}),
			registryDeclared: true,
		},
		{
			name:             "credential missing (no cipher material)",
			rec:              Record{ConnectionID: "anthropic-primary", ProviderKind: "anthropic", Enabled: true, LastTestOutcome: TestOutcomeOK},
			registryDeclared: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if EffectiveEnabled(tc.rec, tc.registryDeclared) {
				t.Fatalf("EffectiveEnabled MUST be false when %s — the single gate would admit a non-dispatchable connection", tc.name)
			}
		})
	}
}

// TestHasCredential_RequiresCipherMaterial_Spec096 — a credential is "present"
// only when the recoverable cipher columns exist; a redaction hint or a
// last-test row alone is NOT a credential.
func TestHasCredential_RequiresCipherMaterial_Spec096(t *testing.T) {
	if (Record{}).HasCredential() {
		t.Fatal("an empty record MUST NOT report a credential present")
	}
	// A redaction without cipher material is not a credential.
	redactionOnly := Record{Secret: &connvault.VaultRecord{Redaction: "…wxyz"}}
	if redactionOnly.HasCredential() {
		t.Fatal("a redaction-only record (no ciphertext/nonce) MUST NOT report a credential present")
	}
	if !withCredential(Record{ConnectionID: "c", ProviderKind: "anthropic"}).HasCredential() {
		t.Fatal("a record with ciphertext+nonce MUST report a credential present")
	}
}
