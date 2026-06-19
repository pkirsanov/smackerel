package connvault

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/config"
)

// Spec 096 SCOPE-02 (SCN-096-W05) unit suite for the reversible,
// authenticated, encrypted-at-rest credential vault.
//
// All secrets here are SYNTHETIC — never a real provider key. The adversarial
// tests (AAD tamper, wrong key, never-returns-plaintext) each carry a passing
// CONTROL so the rejection is provably caused by the asserted property and not
// by a vacuously-failing path: each would FAIL if the corresponding protection
// (the AEAD AAD binding, the authenticated tag, the write-only redaction) were
// removed.

// newTestVault builds a vault under a fresh random 32-byte synthetic master
// key at the given epoch.
func newTestVault(t *testing.T, keyVersion int) *SecretVault {
	t.Helper()
	v, err := NewSecretVault(syntheticMasterKeyB64(t), keyVersion)
	if err != nil {
		t.Fatalf("NewSecretVault: %v", err)
	}
	return v
}

// syntheticMasterKeyB64 returns a base64-encoded random 32-byte key.
func syntheticMasterKeyB64(t *testing.T) string {
	t.Helper()
	raw := make([]byte, masterKeyBytes)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("generate synthetic master key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// TestSecretVault_EncryptDecrypt_RoundTrip_Spec096 — Encrypt → Decrypt under
// the same master key + AAD returns the original bundle byte-for-byte; the
// credential is reversible (recoverable, not hashed), for both a single-field
// (anthropic) and a multi-field (bedrock) bundle.
func TestSecretVault_EncryptDecrypt_RoundTrip_Spec096(t *testing.T) {
	v := newTestVault(t, 1)

	cases := []struct {
		name   string
		connID string
		kind   string
		bundle map[string]string
	}{
		{
			name:   "single-field anthropic api_key",
			connID: "anthropic-primary",
			kind:   "anthropic",
			bundle: map[string]string{"api_key": "sk-ant-synthetic-roundtrip-0123456789"},
		},
		{
			name:   "multi-field bedrock credentials",
			connID: "bedrock-primary",
			kind:   "bedrock",
			bundle: map[string]string{
				"aws_access_key_id":     "AKIA-SYNTHETIC-ACCESS-0001",
				"aws_secret_access_key": "synthetic/secret/access/key/abcdef0123",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, err := v.Encrypt(tc.connID, tc.kind, tc.bundle)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			if rec.KeyVersion != 1 {
				t.Fatalf("rec.KeyVersion = %d, want 1", rec.KeyVersion)
			}
			if len(rec.Nonce) == 0 || len(rec.Ciphertext) == 0 {
				t.Fatalf("expected non-empty nonce+ciphertext, got nonce=%d ciphertext=%d", len(rec.Nonce), len(rec.Ciphertext))
			}

			got, err := v.Decrypt(rec)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if len(got) != len(tc.bundle) {
				t.Fatalf("decrypted bundle has %d fields, want %d", len(got), len(tc.bundle))
			}
			for k, want := range tc.bundle {
				if got[k] != want {
					t.Fatalf("decrypted[%q] = %q, want %q (credential must be reversible)", k, got[k], want)
				}
			}
		})
	}
}

// TestSecretVault_NeverReturnsPlaintext_RedactionLast4_Spec096 (ADVERSARIAL) —
// the at-rest VaultRecord exposes only ciphertext + nonce + key_version + a
// last-4 redaction; the cleartext secret never appears in the record (nor its
// %+v rendering, nor the stored ciphertext bytes). The redaction is exactly
// the last 4 characters. CONTROL: Decrypt still recovers the full secret, so
// the never-plaintext assertion is not vacuously true. This FAILS if any read
// path leaks the secret or if the redaction widened beyond last-4.
func TestSecretVault_NeverReturnsPlaintext_RedactionLast4_Spec096(t *testing.T) {
	v := newTestVault(t, 1)

	const secret = "sk-ant-synthetic-api03-DEADbeef0000WXYZ"
	const wantLast4 = "…WXYZ"
	bundle := map[string]string{"api_key": secret}

	rec, err := v.Encrypt("anthropic-primary", "anthropic", bundle)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Redaction is the last-4 hint only — never the full secret.
	if rec.Redaction != wantLast4 {
		t.Fatalf("rec.Redaction = %q, want %q (last-4 only)", rec.Redaction, wantLast4)
	}
	if strings.Contains(rec.Redaction, secret) {
		t.Fatal("rec.Redaction leaks the full secret")
	}

	// The full record rendering must not contain the plaintext anywhere
	// (no hidden plaintext field).
	if rendered := fmt.Sprintf("%+v", rec); strings.Contains(rendered, secret) {
		t.Fatalf("VaultRecord rendering leaks the plaintext secret: %s", rendered)
	}
	// The stored ciphertext must not contain the plaintext (proves it is
	// actually encrypted, not stored in the clear).
	if bytes.Contains(rec.Ciphertext, []byte(secret)) {
		t.Fatal("ciphertext contains the plaintext secret — not encrypted")
	}

	// CONTROL — the secret really is present and recoverable in-core (so the
	// negative checks above are meaningful, not vacuous).
	got, err := v.Decrypt(rec)
	if err != nil {
		t.Fatalf("control Decrypt: %v", err)
	}
	if got["api_key"] != secret {
		t.Fatalf("control: decrypted api_key = %q, want %q", got["api_key"], secret)
	}
}

// TestSecretVault_AADTamperRejected_Spec096 (ADVERSARIAL) — a decrypt with a
// tampered AAD context (connection_id or kind changed) is rejected by the
// authenticated AEAD; a ciphertext-byte flip is likewise rejected. CONTROL:
// the untampered record decrypts cleanly, so the rejection is specifically due
// to the AAD/tag binding (this FAILS if the AAD binding or the tag were
// removed — the relocated ciphertext would then decrypt).
func TestSecretVault_AADTamperRejected_Spec096(t *testing.T) {
	v := newTestVault(t, 1)
	bundle := map[string]string{"api_key": "sk-ant-synthetic-aad-0123456789"}

	rec, err := v.Encrypt("anthropic-primary", "anthropic", bundle)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// CONTROL: untampered decrypt succeeds.
	if _, err := v.Decrypt(rec); err != nil {
		t.Fatalf("control: untampered Decrypt must succeed, got %v", err)
	}

	t.Run("tampered connection_id rejected", func(t *testing.T) {
		bad := rec
		bad.ConnectionID = "openai-primary" // relocate to a different record's context
		if _, err := v.Decrypt(bad); err == nil {
			t.Fatal("expected rejection: a ciphertext relocated to a different connection_id must not decrypt")
		}
	})

	t.Run("tampered kind rejected", func(t *testing.T) {
		bad := rec
		bad.Kind = "openai"
		if _, err := v.Decrypt(bad); err == nil {
			t.Fatal("expected rejection: a ciphertext re-labelled to a different kind must not decrypt")
		}
	})

	t.Run("flipped ciphertext byte rejected", func(t *testing.T) {
		bad := rec
		bad.Ciphertext = append([]byte(nil), rec.Ciphertext...)
		bad.Ciphertext[0] ^= 0xFF // tamper one byte
		if _, err := v.Decrypt(bad); err == nil {
			t.Fatal("expected rejection: a tampered ciphertext must fail the auth tag")
		}
	})
}

// TestSecretVault_WrongKeyRejected_Spec096 (ADVERSARIAL) — a decrypt under a
// different master key (same key_version, so the epoch guard does not
// short-circuit) is rejected; no plaintext is returned. CONTROL: the original
// key decrypts cleanly. This FAILS if the ciphertext were recoverable without
// the exact master key.
func TestSecretVault_WrongKeyRejected_Spec096(t *testing.T) {
	vaultA := newTestVault(t, 1)
	vaultB := newTestVault(t, 1) // different random key, SAME epoch

	bundle := map[string]string{"api_key": "sk-ant-synthetic-wrongkey-0123456789"}
	rec, err := vaultA.Encrypt("anthropic-primary", "anthropic", bundle)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// CONTROL: the correct key recovers the secret.
	if _, err := vaultA.Decrypt(rec); err != nil {
		t.Fatalf("control: correct-key Decrypt must succeed, got %v", err)
	}

	// Wrong key → rejected, no plaintext.
	got, err := vaultB.Decrypt(rec)
	if err == nil {
		t.Fatal("expected rejection: a ciphertext must not decrypt under the wrong master key")
	}
	if got != nil {
		t.Fatalf("expected nil bundle on wrong-key decrypt, got %v", got)
	}
}

// TestSecretVault_MasterKeyFailLoud_Spec096 (ADVERSARIAL, G028) — LoadVault
// enforces the design §11.2 fail-loud predicate: the master key is REQUIRED
// iff an ENABLED db-mode connection is declared. CONTROL cases (valid key
// builds a vault; Ollama-only needs no key) prove the loader can succeed, so
// the failure cases are not vacuous. The disabled-slot case is the REGRESSION
// GUARD for the boot bug where the default-shipped DISABLED hosted db-mode
// slots demanded the master key and made a fresh/dev/test stack unbootable.
// This FAILS if the loader ever substituted a default key, skipped encryption
// when the key was required-but-absent, or demanded the key for a
// declared-but-disabled slot.
func TestSecretVault_MasterKeyFailLoud_Spec096(t *testing.T) {
	dbModeConns := []config.ModelConnection{
		{ID: "local-ollama", Kind: "ollama", SecretRef: config.ModelConnectionSecretRef{Mode: config.ModelConnectionSecretModeNone}},
		{ID: "anthropic-primary", Kind: "anthropic", Enabled: true, SecretRef: config.ModelConnectionSecretRef{Mode: config.ModelConnectionSecretModeDB}},
	}
	disabledDBModeConns := []config.ModelConnection{
		{ID: "local-ollama", Kind: "ollama", Enabled: true, SecretRef: config.ModelConnectionSecretRef{Mode: config.ModelConnectionSecretModeNone}},
		{ID: "anthropic-disabled", Kind: "anthropic", Enabled: false, SecretRef: config.ModelConnectionSecretRef{Mode: config.ModelConnectionSecretModeDB}},
	}
	ollamaOnlyConns := []config.ModelConnection{
		{ID: "local-ollama", Kind: "ollama", SecretRef: config.ModelConnectionSecretRef{Mode: config.ModelConnectionSecretModeNone}},
	}
	validKey := syntheticMasterKeyB64(t)

	t.Run("db-mode declared + empty master key → fail-loud", func(t *testing.T) {
		v, err := LoadVault("", 1, dbModeConns)
		if !errors.Is(err, ErrMasterKeyRequired) {
			t.Fatalf("expected ErrMasterKeyRequired, got vault=%v err=%v", v, err)
		}
		if v != nil {
			t.Fatal("expected nil vault on fail-loud")
		}
	})

	t.Run("db-mode declared + valid key → vault built (CONTROL)", func(t *testing.T) {
		v, err := LoadVault(validKey, 1, dbModeConns)
		if err != nil {
			t.Fatalf("expected a vault, got err=%v", err)
		}
		if v == nil {
			t.Fatal("expected a non-nil vault")
		}
	})

	t.Run("ollama-only + empty key → no vault, no error (CONTROL)", func(t *testing.T) {
		v, err := LoadVault("", 1, ollamaOnlyConns)
		if err != nil {
			t.Fatalf("ollama-only must not require a master key, got err=%v", err)
		}
		if v != nil {
			t.Fatal("ollama-only must not build a vault")
		}
	})

	t.Run("DISABLED db-mode slot + empty key → no vault, no error (REGRESSION GUARD: default-shipped disabled hosted slots must NOT block boot)", func(t *testing.T) {
		v, err := LoadVault("", 1, disabledDBModeConns)
		if err != nil {
			t.Fatalf("a declared-but-DISABLED db-mode slot must NOT require a master key (boot must succeed), got err=%v", err)
		}
		if v != nil {
			t.Fatal("expected nil vault when only disabled db-mode slots are declared and no key is set")
		}
	})

	t.Run("present-but-not-base64 key → fail-loud", func(t *testing.T) {
		if _, err := LoadVault("not-base64-!!!", 1, dbModeConns); err == nil {
			t.Fatal("expected a fail-loud error for a non-base64 master key")
		}
	})

	t.Run("present-but-wrong-length key → fail-loud", func(t *testing.T) {
		short := base64.StdEncoding.EncodeToString(make([]byte, 16)) // 128-bit, not 256-bit
		if _, err := LoadVault(short, 1, dbModeConns); err == nil {
			t.Fatal("expected a fail-loud error for a master key that is not 32 bytes")
		}
	})
}

// TestSecretVault_Rotation_ReEncryptsToNewEpoch_Spec096 — the documented
// rotation primitive (design §11.3) re-encrypts a record from the old epoch to
// the new epoch with a fresh nonce and key_version, recoverable under the new
// key only. Proves rotation bumps key_version + re-encrypts reversibly.
func TestSecretVault_Rotation_ReEncryptsToNewEpoch_Spec096(t *testing.T) {
	oldVault := newTestVault(t, 1)
	newVault := newTestVault(t, 2) // new key, new epoch

	bundle := map[string]string{"api_key": "sk-ant-synthetic-rotate-0123456789"}
	rec1, err := oldVault.Encrypt("anthropic-primary", "anthropic", bundle)
	if err != nil {
		t.Fatalf("Encrypt (epoch 1): %v", err)
	}

	rec2, err := newVault.Rotate(oldVault, rec1)
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if rec2.KeyVersion != 2 {
		t.Fatalf("rotated key_version = %d, want 2", rec2.KeyVersion)
	}
	if bytes.Equal(rec2.Nonce, rec1.Nonce) {
		t.Fatal("rotation must use a fresh nonce")
	}
	if bytes.Equal(rec2.Ciphertext, rec1.Ciphertext) {
		t.Fatal("rotation must produce a fresh ciphertext")
	}

	// Recoverable under the new key.
	got, err := newVault.Decrypt(rec2)
	if err != nil {
		t.Fatalf("Decrypt rotated record under new key: %v", err)
	}
	if got["api_key"] != bundle["api_key"] {
		t.Fatalf("rotated decrypt = %q, want %q", got["api_key"], bundle["api_key"])
	}

	// The old vault can no longer read the rotated (epoch-2) record.
	if _, err := oldVault.Decrypt(rec2); err == nil {
		t.Fatal("expected the old epoch-1 vault to reject the rotated epoch-2 record")
	}
}
