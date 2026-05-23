package qfdecisions

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// SCN-SM-041-028 — Keystore selects the newest not_before-valid key.
func TestCallbackKeystoreSelectsNewestNotBeforeValidKeyAndIncludesKeyIdInEnvelope(t *testing.T) {
	raw := `[
		{"key_id":"key-may","secret":"secret-may","not_before":"2026-05-01T00:00:00Z"},
		{"key_id":"key-june","secret":"secret-june","not_before":"2026-06-01T00:00:00Z"},
		{"key_id":"key-april","secret":"secret-april","not_before":"2026-04-01T00:00:00Z"}
	]`
	keystore, err := LoadCallbackKeystoreFromJSON(raw)
	if err != nil {
		t.Fatalf("load keystore: %v", err)
	}
	if got := keystore.Len(); got != 3 {
		t.Fatalf("keystore Len: want 3, got %d", got)
	}
	// Probe at 2026-06-15: every key is past — newest valid is key-june.
	now := mustParse(t, "2026-06-15T00:00:00Z")
	key, err := keystore.SelectActiveKey(now)
	if err != nil {
		t.Fatalf("SelectActiveKey(2026-06-15): %v", err)
	}
	if key.KeyID != "key-june" {
		t.Fatalf("SelectActiveKey at 2026-06-15 picked %q, want key-june", key.KeyID)
	}
	// Probe at 2026-05-15: key-june is future — newest valid is key-may.
	now = mustParse(t, "2026-05-15T00:00:00Z")
	key, err = keystore.SelectActiveKey(now)
	if err != nil {
		t.Fatalf("SelectActiveKey(2026-05-15): %v", err)
	}
	if key.KeyID != "key-may" {
		t.Fatalf("SelectActiveKey at 2026-05-15 picked %q, want key-may", key.KeyID)
	}
	// Sign an envelope and confirm the signed envelope carries key_id
	// from the selected key.
	signer := NewCallbackSigner(keystore, func() time.Time {
		return mustParse(t, "2026-06-15T00:00:00Z")
	})
	env := validEnvelope("2026-06-15T00:01:00Z")
	signed, err := signer.Sign(env)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if signed.KeyID != "key-june" {
		t.Fatalf("signed envelope KeyID: want key-june, got %q", signed.KeyID)
	}
	if signed.Signature == "" {
		t.Fatal("signed envelope Signature is empty")
	}
}

// SCN-SM-041-028 — Empty keystore and all-future-keys fail loud.
func TestCallbackKeystoreFailsLoudOnEmptyKeySetAndOnAllKeysWithFutureNotBefore(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		if _, err := LoadCallbackKeystoreFromJSON(""); err == nil {
			t.Fatal("LoadCallbackKeystoreFromJSON(\"\") want error, got nil")
		}
	})
	t.Run("empty array", func(t *testing.T) {
		if _, err := LoadCallbackKeystoreFromJSON("[]"); err == nil {
			t.Fatal("LoadCallbackKeystoreFromJSON(\"[]\") want error, got nil")
		}
	})
	t.Run("missing key_id", func(t *testing.T) {
		raw := `[{"secret":"x","not_before":"2026-05-01T00:00:00Z"}]`
		if _, err := LoadCallbackKeystoreFromJSON(raw); err == nil {
			t.Fatal("LoadCallbackKeystoreFromJSON missing key_id want error, got nil")
		}
	})
	t.Run("missing secret", func(t *testing.T) {
		raw := `[{"key_id":"k","not_before":"2026-05-01T00:00:00Z"}]`
		if _, err := LoadCallbackKeystoreFromJSON(raw); err == nil {
			t.Fatal("LoadCallbackKeystoreFromJSON missing secret want error, got nil")
		}
	})
	t.Run("missing not_before", func(t *testing.T) {
		raw := `[{"key_id":"k","secret":"x"}]`
		if _, err := LoadCallbackKeystoreFromJSON(raw); err == nil {
			t.Fatal("LoadCallbackKeystoreFromJSON missing not_before want error, got nil")
		}
	})
	t.Run("duplicate key_id", func(t *testing.T) {
		raw := `[
			{"key_id":"k","secret":"a","not_before":"2026-05-01T00:00:00Z"},
			{"key_id":"k","secret":"b","not_before":"2026-06-01T00:00:00Z"}
		]`
		if _, err := LoadCallbackKeystoreFromJSON(raw); err == nil {
			t.Fatal("LoadCallbackKeystoreFromJSON duplicate key_id want error, got nil")
		}
	})
	t.Run("all keys future", func(t *testing.T) {
		raw := `[
			{"key_id":"k1","secret":"a","not_before":"2099-05-01T00:00:00Z"},
			{"key_id":"k2","secret":"b","not_before":"2099-06-01T00:00:00Z"}
		]`
		keystore, err := LoadCallbackKeystoreFromJSON(raw)
		if err != nil {
			t.Fatalf("LoadCallbackKeystoreFromJSON: %v", err)
		}
		_, selErr := keystore.SelectActiveKey(mustParse(t, "2026-05-15T00:00:00Z"))
		if !errors.Is(selErr, ErrNoActiveCallbackKey) {
			t.Fatalf("SelectActiveKey at 2026-05-15: want ErrNoActiveCallbackKey, got %v", selErr)
		}
	})
	t.Run("nil keystore returns sentinel", func(t *testing.T) {
		var keystore *CallbackKeystore
		_, selErr := keystore.SelectActiveKey(time.Now())
		if !errors.Is(selErr, ErrNoActiveCallbackKey) {
			t.Fatalf("nil keystore SelectActiveKey: want ErrNoActiveCallbackKey, got %v", selErr)
		}
	})
}

// LoadCallbackKeystoreFromEnv reads QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON
// and returns nil-nil when unset, parsed keystore when set, error when
// set but malformed.
func TestLoadCallbackKeystoreFromEnvReturnsNilWhenUnsetAndKeystoreWhenSet(t *testing.T) {
	t.Setenv(CallbackSigningKeysEnvVar, "")
	got, err := LoadCallbackKeystoreFromEnv()
	if err != nil {
		t.Fatalf("LoadCallbackKeystoreFromEnv unset: %v", err)
	}
	if got != nil {
		t.Fatalf("LoadCallbackKeystoreFromEnv unset: want nil, got %v", got)
	}
	t.Setenv(CallbackSigningKeysEnvVar, `[{"key_id":"k","secret":"x","not_before":"2026-01-01T00:00:00Z"}]`)
	got, err = LoadCallbackKeystoreFromEnv()
	if err != nil {
		t.Fatalf("LoadCallbackKeystoreFromEnv set: %v", err)
	}
	if got == nil || got.Len() != 1 {
		t.Fatalf("LoadCallbackKeystoreFromEnv set: want 1-key keystore, got %v", got)
	}
	t.Setenv(CallbackSigningKeysEnvVar, `{"this is not": "an array"}`)
	if _, err := LoadCallbackKeystoreFromEnv(); err == nil {
		t.Fatal("LoadCallbackKeystoreFromEnv malformed: want error, got nil")
	}
}

// Adversarial: prove KeyIDs returns descending-by-not_before order so
// diagnostic logs surface the active key first.
func TestCallbackKeystoreKeyIDsReturnsDescendingByNotBefore(t *testing.T) {
	raw := `[
		{"key_id":"k-old","secret":"a","not_before":"2026-01-01T00:00:00Z"},
		{"key_id":"k-new","secret":"b","not_before":"2026-06-01T00:00:00Z"},
		{"key_id":"k-mid","secret":"c","not_before":"2026-03-01T00:00:00Z"}
	]`
	keystore, err := LoadCallbackKeystoreFromJSON(raw)
	if err != nil {
		t.Fatalf("LoadCallbackKeystoreFromJSON: %v", err)
	}
	want := []string{"k-new", "k-mid", "k-old"}
	got := keystore.KeyIDs()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("KeyIDs: want %v, got %v", want, got)
	}
}

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ts.UTC()
}

func validEnvelope(expiresAt string) CallbackEnvelope {
	return CallbackEnvelope{
		CallbackID: "01939a4f-7ad2-7c5f-aaaa-000000000001",
		TraceID:    "trace-test-001",
		PacketID:   "packet-test-001",
		Action:     CallbackActionNoop,
		Nonce:      "nonce-test-001",
		ExpiresAt:  expiresAt,
		Surface:    SurfaceTelegram,
	}
}
