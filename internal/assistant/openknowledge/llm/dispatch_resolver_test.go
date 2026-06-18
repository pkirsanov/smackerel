// Spec 096 SCOPE-03 — SCN-096-G01 / SCN-096-G05 adversarial tests for the
// provider-aware DispatchResolver: a misconfigured / not-effective-enabled
// target is rejected with a TYPED reason and NEVER silently re-routed to the
// local Ollama connection (FR-X1, G028 fail-loud); and the decrypted cleartext
// credential lives ONLY in the resolved ChatRequest.APIKey — never in a typed
// error, the attribution, or any other resolver-produced value (design §11.5).
//
// The vault is a REAL connvault.SecretVault keyed to a synthetic 32-byte key;
// the credential record is produced by the real Encrypt path; the credential
// source is an in-memory map standing in for the SCOPE-06 DB read — so SCOPE-03
// needs no live Postgres.
package llm

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/connvault"
	"github.com/smackerel/smackerel/internal/config"
)

// spec096Secret is the synthetic cleartext credential the scrub assertions hunt
// for. It is NOT a real key — purely a recognizable sentinel.
const spec096Secret = "sk-synthetic-anthropic-096-DO-NOT-LOG"

// memCreds is the in-memory CredentialSource: the SCOPE-06 DB read of
// model_provider_connections mocked by a map.
type memCreds map[string]connvault.VaultRecord

func (m memCreds) Credential(connID string) (connvault.VaultRecord, bool) {
	rec, ok := m[connID]
	return rec, ok
}

// spec096Vault builds a real AES-256-GCM vault from a 32-byte seed at epoch 1.
func spec096Vault(t *testing.T, seed string) *connvault.SecretVault {
	t.Helper()
	if len(seed) != 32 {
		t.Fatalf("test seed must be exactly 32 bytes, got %d", len(seed))
	}
	v, err := connvault.NewSecretVault(base64.StdEncoding.EncodeToString([]byte(seed)), 1)
	if err != nil {
		t.Fatalf("NewSecretVault: %v", err)
	}
	return v
}

// spec096Conns is the registry fixture: an enabled local Ollama (no secret), an
// enabled hosted anthropic (db-mode), and a DISABLED hosted openai (db-mode).
func spec096Conns() []config.ModelConnection {
	return []config.ModelConnection{
		{
			ID:        "local-ollama",
			Kind:      config.ModelConnectionKindOllama,
			Enabled:   true,
			Params:    map[string]any{"base_url": "http://ollama.test:11434"},
			SecretRef: config.ModelConnectionSecretRef{Mode: config.ModelConnectionSecretModeNone},
		},
		{
			ID:        "anthropic-primary",
			Kind:      config.ModelConnectionKindAnthropic,
			Enabled:   true,
			SecretRef: config.ModelConnectionSecretRef{Mode: config.ModelConnectionSecretModeDB},
		},
		{
			ID:        "openai-primary",
			Kind:      config.ModelConnectionKindOpenAI,
			Enabled:   false, // declared but disabled — exercising it must fail loud.
			SecretRef: config.ModelConnectionSecretRef{Mode: config.ModelConnectionSecretModeDB},
		},
	}
}

// TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096 —
// ADVERSARIAL. Every not-effective-enabled hosted target (missing credential,
// disabled connection, unknown kind) is rejected with a typed *ResolveError and
// the returned ResolvedDispatch is ZERO — NO Ollama substitution. The test
// fails if any reject path yields a local/Ollama ChatRequest or a non-empty
// attribution. A positive control proves a fully-configured hosted target DOES
// resolve (so the rejections are meaningful, not a resolver that rejects
// everything).
func TestDispatchResolver_MisconfiguredConnection_NeverFallsBackToOllama_Spec096(t *testing.T) {
	vault := spec096Vault(t, "0123456789abcdef0123456789abcdef")

	// Positive control: anthropic with a real stored credential resolves.
	t.Run("control_fully_configured_hosted_resolves", func(t *testing.T) {
		rec, err := vault.Encrypt("anthropic-primary", config.ModelConnectionKindAnthropic, map[string]string{"api_key": spec096Secret})
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		r, err := NewDispatchResolver(spec096Conns(), vault, memCreds{"anthropic-primary": rec})
		if err != nil {
			t.Fatalf("NewDispatchResolver: %v", err)
		}
		got, err := r.Resolve("anthropic/claude-3-5-sonnet")
		if err != nil {
			t.Fatalf("control resolve MUST succeed, got %v", err)
		}
		if got.Request.Provider != config.ModelConnectionKindAnthropic {
			t.Fatalf("control provider = %q, want anthropic (NOT ollama)", got.Request.Provider)
		}
		if got.Request.Model != "claude-3-5-sonnet" {
			t.Fatalf("control model MUST be the bare backend id, got %q", got.Request.Model)
		}
		if got.Request.APIKey == nil || *got.Request.APIKey != spec096Secret {
			t.Fatalf("control MUST carry the decrypted api_key in the request")
		}
	})

	// Each reject case: typed error + ZERO dispatch (no Ollama fallback).
	rejectCases := []struct {
		name       string
		model      string
		creds      memCreds
		wantReason RejectReason
	}{
		{
			name:       "hosted_target_with_no_stored_credential",
			model:      "anthropic/claude-3-5-sonnet",
			creds:      memCreds{}, // anthropic enabled but NO credential stored
			wantReason: RejectCredentialMissing,
		},
		{
			name:       "disabled_connection",
			model:      "openai/gpt-4o",
			creds:      memCreds{},
			wantReason: RejectConnectionDisabled,
		},
		{
			name:       "unknown_provider_kind",
			model:      "google/gemini-1.5-pro", // no google connection declared
			creds:      memCreds{},
			wantReason: RejectConnectionNotFound,
		},
		{
			name:       "malformed_model_id_no_qualifier",
			model:      "claude-3-5-sonnet", // no "<kind>/" prefix
			creds:      memCreds{},
			wantReason: RejectMalformedModelID,
		},
	}
	for _, tc := range rejectCases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := NewDispatchResolver(spec096Conns(), vault, tc.creds)
			if err != nil {
				t.Fatalf("NewDispatchResolver: %v", err)
			}
			got, err := r.Resolve(tc.model)
			if err == nil {
				t.Fatalf("MUST reject %q, got a successful dispatch %+v", tc.model, got)
			}
			var re *ResolveError
			if !errors.As(err, &re) {
				t.Fatalf("error MUST be a typed *ResolveError, got %T: %v", err, err)
			}
			if re.Reason != tc.wantReason {
				t.Fatalf("reject reason = %q, want %q", re.Reason, tc.wantReason)
			}
			// ADVERSARIAL — the never-fall-back-to-Ollama guarantee: the
			// returned dispatch MUST be the zero value. A regression that
			// substituted the local Ollama connection would populate
			// Provider="ollama" / a non-empty attribution and fail here.
			if got.Request.Provider != "" || got.Request.Model != "" || got.Attribution != "" {
				t.Fatalf("rejected target MUST yield a ZERO dispatch (no Ollama fallback), got %+v", got)
			}
			if got.Request.Provider == config.ModelConnectionKindOllama {
				t.Fatalf("rejected hosted target was silently re-routed to Ollama — FORBIDDEN (SCN-096-G01)")
			}
		})
	}
}

// TestDispatch_SecretNeverInLogsOrErrors_Spec096 — ADVERSARIAL. The decrypted
// cleartext credential appears ONLY in the resolved ChatRequest.APIKey (the
// seam the sidecar consumes). It NEVER appears in a typed *ResolveError, the
// attribution, the wire model, or the provider — proven on both the success
// path and the failure paths. The resolver holds NO logger, so there is no log
// surface to leak into; the error-body checks are the load-bearing adversarial
// assertion (a build that folded the secret into the error string would fail).
func TestDispatch_SecretNeverInLogsOrErrors_Spec096(t *testing.T) {
	vaultA := spec096Vault(t, "0123456789abcdef0123456789abcdef")

	rec, err := vaultA.Encrypt("anthropic-primary", config.ModelConnectionKindAnthropic, map[string]string{"api_key": spec096Secret})
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	t.Run("success_secret_only_in_request_api_key", func(t *testing.T) {
		r, err := NewDispatchResolver(spec096Conns(), vaultA, memCreds{"anthropic-primary": rec})
		if err != nil {
			t.Fatalf("NewDispatchResolver: %v", err)
		}
		got, err := r.Resolve("anthropic/claude-3-5-sonnet")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if got.Request.APIKey == nil || *got.Request.APIKey != spec096Secret {
			t.Fatalf("the api_key seam MUST carry the decrypted secret in the request")
		}
		// The secret must NOT have leaked into any non-credential field.
		for label, field := range map[string]string{
			"attribution": got.Attribution,
			"provider":    got.Request.Provider,
			"model":       got.Request.Model,
		} {
			if strings.Contains(field, spec096Secret) {
				t.Fatalf("secret leaked into resolved %s: %q", label, field)
			}
		}
	})

	t.Run("disabled_target_with_secret_on_disk_rejects_without_leaking", func(t *testing.T) {
		// openai is disabled; even though a secret-bearing record exists, the
		// resolver MUST reject WITHOUT decrypting and MUST NOT echo the secret.
		openaiRec, err := vaultA.Encrypt("openai-primary", config.ModelConnectionKindOpenAI, map[string]string{"api_key": spec096Secret})
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		r, err := NewDispatchResolver(spec096Conns(), vaultA, memCreds{"openai-primary": openaiRec})
		if err != nil {
			t.Fatalf("NewDispatchResolver: %v", err)
		}
		_, err = r.Resolve("openai/gpt-4o")
		if err == nil {
			t.Fatalf("disabled target MUST reject")
		}
		if strings.Contains(err.Error(), spec096Secret) {
			t.Fatalf("typed error leaked the secret: %v", err)
		}
	})

	t.Run("decrypt_failure_under_wrong_master_key_never_leaks", func(t *testing.T) {
		// A DIFFERENT 32-byte master key at the same epoch → GCM auth-tag
		// failure. The error MUST be typed decrypt_failed and secret-free.
		vaultB := spec096Vault(t, "ZZZZ56789abcdefZZZZ56789abcdefZZ")
		r, err := NewDispatchResolver(spec096Conns(), vaultB, memCreds{"anthropic-primary": rec})
		if err != nil {
			t.Fatalf("NewDispatchResolver: %v", err)
		}
		_, err = r.Resolve("anthropic/claude-3-5-sonnet")
		if err == nil {
			t.Fatalf("decryption under the wrong master key MUST fail loud")
		}
		var re *ResolveError
		if !errors.As(err, &re) || re.Reason != RejectDecryptFailed {
			t.Fatalf("want typed decrypt_failed, got %v", err)
		}
		if strings.Contains(err.Error(), spec096Secret) {
			t.Fatalf("decrypt-failure error leaked the secret: %v", err)
		}
	})
}

// TestDispatchResolver_DuplicateKind_FailsLoud_Spec096 — the at-most-one-
// connection-per-kind invariant: a registry that declares two connections of
// the same kind aborts construction rather than silently picking one.
func TestDispatchResolver_DuplicateKind_FailsLoud_Spec096(t *testing.T) {
	conns := []config.ModelConnection{
		{ID: "anthropic-a", Kind: config.ModelConnectionKindAnthropic, Enabled: true,
			SecretRef: config.ModelConnectionSecretRef{Mode: config.ModelConnectionSecretModeDB}},
		{ID: "anthropic-b", Kind: config.ModelConnectionKindAnthropic, Enabled: true,
			SecretRef: config.ModelConnectionSecretRef{Mode: config.ModelConnectionSecretModeDB}},
	}
	if _, err := NewDispatchResolver(conns, nil, memCreds{}); err == nil {
		t.Fatalf("duplicate provider kind MUST fail loud at construction")
	}
}
