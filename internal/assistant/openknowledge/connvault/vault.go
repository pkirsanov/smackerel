// Package connvault implements the Spec 096 SCOPE-02 reversible,
// authenticated, encrypted-at-rest credential vault for the operator-global
// multi-provider model connections.
//
// # Primitive (design §11.1)
//
// AES-256-GCM (Go stdlib crypto/aes + crypto/cipher.NewGCM), a single
// operator master key, a per-record random 96-bit nonce, a 128-bit auth tag,
// and an AAD that binds each ciphertext to its context:
//
//	AAD = connection_id ":" provider_kind ":" secret_key_version
//
// so a ciphertext cannot be relocated to another record or replayed under a
// different key epoch. Tamper, a wrong master key, or an AAD mismatch fail
// the authenticated decryption loudly (fail-closed) — never silent garbage.
//
// # Reversible — NOT hashed (binding)
//
// The stored credential is REPLAYED to `Authorization: Bearer <key>` at
// dispatch time, so it MUST be recoverable. This is the reversible
// managed-secret class (like CARD_REWARDS_GCAL_CREDENTIALS / telegram bot
// token), explicitly NOT the verifier class (AUTH_AT_REST_HASHING_KEY).
// One-way hashing (argon2id) is structurally wrong for this data and is
// FORBIDDEN here: argon2id verifies a presented secret, it cannot recover
// one. A future agent MUST NOT "harden" this vault into a hash — that breaks
// dispatch.
//
// # Never returns plaintext
//
// A VaultRecord — the exact at-rest shape persisted to
// model_provider_connections — carries only ciphertext + nonce + key-version
// + a non-secret last-4 redaction hint. It NEVER carries plaintext. The only
// way to recover the credential is Decrypt, in-core. No method returns or
// logs the plaintext or the master key.
//
// # Master-key lifecycle (fail-loud, G028; design §11.2/§11.3)
//
// The master key is the env-held managed secret LLM_PROVIDER_SECRET_MASTER_KEY
// (base64 of exactly 32 bytes), confined to the Go core and never passed to
// the sidecar nor logged. LoadVault enforces the fail-loud predicate: the key
// is REQUIRED iff the registry declares at least one db-mode connection; an
// Ollama-only deployment needs no vault and no new secret. Rotation is the
// per-row Rotate primitive (bump key_version + re-encrypt under the new key)
// driving the documented re-encrypt-all procedure.
package connvault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/smackerel/smackerel/internal/config"
)

// masterKeyBytes is the required decoded master-key length: 32 bytes = 256-bit
// (AES-256). The fail-loud loader rejects any other length (design §11.2).
const masterKeyBytes = 32

// ErrVaultNotConfigured is returned by Encrypt/Decrypt/Rotate when invoked on
// a nil/unconfigured vault (no master key loaded). Callers that reach a vault
// operation on an Ollama-only deployment have a wiring bug — the vault is only
// constructed when a db-mode connection is declared.
var ErrVaultNotConfigured = errors.New("connvault: vault not configured (no master key loaded)")

// ErrMasterKeyRequired is the named fail-loud abort (G028) when a db-mode
// connection is declared but LLM_PROVIDER_SECRET_MASTER_KEY is absent/empty.
var ErrMasterKeyRequired = errors.New(
	"llm: LLM_PROVIDER_SECRET_MASTER_KEY is required and must be a base64-encoded 32-byte key " +
		"when one or more llm.connections declare secret_ref.mode=db")

// SecretVault holds the in-core AES-256-GCM AEAD bound to the loaded master
// key plus the current master-key epoch (secret_key_version). It is the only
// component that ever holds the master key; the key never leaves this struct.
type SecretVault struct {
	gcm        cipher.AEAD
	keyVersion int
}

// VaultRecord is the encrypted-at-rest representation of a connection's secret
// bundle — the EXACT set of at-rest columns persisted to / re-read from
// model_provider_connections. It NEVER carries plaintext: the credential is
// recoverable only via SecretVault.Decrypt, in-core.
type VaultRecord struct {
	ConnectionID string // 1:1 to the SST registry slug; part of the AEAD AAD
	Kind         string // provider kind; part of the AEAD AAD
	Ciphertext   []byte // AES-256-GCM ciphertext + 128-bit tag (secret_ciphertext)
	Nonce        []byte // per-record random 96-bit nonce (secret_nonce)
	KeyVersion   int    // master-key epoch (secret_key_version); part of the AEAD AAD
	Redaction    string // non-secret last-4 display hint (secret_redaction), e.g. "…wxyz"
}

// NewSecretVault builds a vault from a base64-encoded 32-byte master key at the
// given key-version epoch. It is fail-loud (design §11.2): an empty key, a
// non-base64 key, a key that does not decode to exactly 32 bytes, or a
// non-positive epoch each abort with a named error and NO substituted default.
func NewSecretVault(masterKeyB64 string, keyVersion int) (*SecretVault, error) {
	if masterKeyB64 == "" {
		return nil, ErrMasterKeyRequired
	}
	raw, err := base64.StdEncoding.DecodeString(masterKeyB64)
	if err != nil {
		return nil, fmt.Errorf("connvault: LLM_PROVIDER_SECRET_MASTER_KEY is not valid base64: %w", err)
	}
	if len(raw) != masterKeyBytes {
		return nil, fmt.Errorf(
			"connvault: LLM_PROVIDER_SECRET_MASTER_KEY must decode to exactly %d bytes (AES-256), got %d",
			masterKeyBytes, len(raw))
	}
	if keyVersion <= 0 {
		return nil, fmt.Errorf("connvault: master-key epoch (key_version) must be a positive integer, got %d", keyVersion)
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, fmt.Errorf("connvault: create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("connvault: create GCM: %w", err)
	}
	// aes.NewCipher copies the key into the block; scrub our local copy.
	zero(raw)
	return &SecretVault{gcm: gcm, keyVersion: keyVersion}, nil
}

// RequiresMasterKey reports whether the registry declares at least one db-mode
// connection (secret_ref.mode == "db") — the design §11.2 predicate that makes
// the master key mandatory. An Ollama-only / env-only deployment returns false.
func RequiresMasterKey(connections []config.ModelConnection) bool {
	for _, c := range connections {
		if c.SecretRef.Mode == config.ModelConnectionSecretModeDB {
			return true
		}
	}
	return false
}

// LoadVault constructs the vault per the design §11.2 fail-loud predicate:
//
//   - a db-mode connection is declared + the master key is absent/empty
//     → fail-loud (ErrMasterKeyRequired);
//   - no db-mode connection + no master key → (nil, nil): an Ollama-only
//     deployment needs no vault and adds no new required secret;
//   - a master key is present → it MUST be valid (NewSecretVault validates the
//     32-byte length), db-mode or not — a configured key is never silently
//     ignored.
func LoadVault(masterKeyB64 string, keyVersion int, connections []config.ModelConnection) (*SecretVault, error) {
	if masterKeyB64 == "" {
		if RequiresMasterKey(connections) {
			return nil, ErrMasterKeyRequired
		}
		return nil, nil
	}
	return NewSecretVault(masterKeyB64, keyVersion)
}

// KeyVersion returns the loaded master-key epoch this vault encrypts under.
func (v *SecretVault) KeyVersion() int { return v.keyVersion }

// Encrypt seals a secret bundle (the kind's secret fields, e.g.
// {"api_key": "..."} or Bedrock's {"aws_access_key_id":..., "aws_secret_access_key":...})
// as ONE AES-256-GCM blob under the loaded master key, with a fresh per-record
// 96-bit nonce and an AAD bound to (connection_id, kind, key_version). It
// returns the at-rest VaultRecord (ciphertext + nonce + key_version + a
// non-secret last-4 redaction) — never the plaintext.
func (v *SecretVault) Encrypt(connID, kind string, bundle map[string]string) (VaultRecord, error) {
	if v == nil || v.gcm == nil {
		return VaultRecord{}, ErrVaultNotConfigured
	}
	if connID == "" || kind == "" {
		return VaultRecord{}, fmt.Errorf("connvault: connection_id and provider_kind are required to encrypt")
	}
	if len(bundle) == 0 {
		return VaultRecord{}, fmt.Errorf("connvault: refusing to encrypt an empty secret bundle for connection %q", connID)
	}
	// json.Marshal of a map[string]string emits keys in sorted order →
	// deterministic plaintext, so the round-trip recovers the bundle exactly.
	plaintext, err := json.Marshal(bundle)
	if err != nil {
		return VaultRecord{}, fmt.Errorf("connvault: marshal secret bundle: %w", err)
	}
	defer zero(plaintext)

	nonce := make([]byte, v.gcm.NonceSize()) // GCM standard nonce = 96-bit
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return VaultRecord{}, fmt.Errorf("connvault: generate nonce: %w", err)
	}
	aad := canonicalAAD(connID, kind, v.keyVersion)
	// Seal(nil, ...) — the nonce is stored SEPARATELY (secret_nonce column),
	// not prepended to the ciphertext.
	ciphertext := v.gcm.Seal(nil, nonce, plaintext, aad)
	return VaultRecord{
		ConnectionID: connID,
		Kind:         kind,
		Ciphertext:   ciphertext,
		Nonce:        nonce,
		KeyVersion:   v.keyVersion,
		Redaction:    redactionFor(bundle),
	}, nil
}

// Decrypt recovers the secret bundle from an at-rest VaultRecord, in-core. It
// fails closed: a key-version mismatch, a bad nonce length, or an authenticated
// decryption failure (tamper / wrong master key / AAD mismatch) returns an
// error and NO plaintext — never silent garbage.
func (v *SecretVault) Decrypt(rec VaultRecord) (map[string]string, error) {
	if v == nil || v.gcm == nil {
		return nil, ErrVaultNotConfigured
	}
	if rec.KeyVersion != v.keyVersion {
		return nil, fmt.Errorf(
			"connvault: key_version mismatch for connection %q (record epoch %d, loaded master-key epoch %d); rotate or load the matching key",
			rec.ConnectionID, rec.KeyVersion, v.keyVersion)
	}
	if len(rec.Nonce) != v.gcm.NonceSize() {
		return nil, fmt.Errorf("connvault: invalid nonce length for connection %q (got %d, want %d)",
			rec.ConnectionID, len(rec.Nonce), v.gcm.NonceSize())
	}
	aad := canonicalAAD(rec.ConnectionID, rec.Kind, rec.KeyVersion)
	plaintext, err := v.gcm.Open(nil, rec.Nonce, rec.Ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf(
			"connvault: authenticated decryption failed for connection %q (tamper, wrong master key, or AAD mismatch): %w",
			rec.ConnectionID, err)
	}
	defer zero(plaintext)
	var bundle map[string]string
	if err := json.Unmarshal(plaintext, &bundle); err != nil {
		return nil, fmt.Errorf("connvault: unmarshal decrypted bundle for connection %q: %w", rec.ConnectionID, err)
	}
	return bundle, nil
}

// Rotate re-encrypts rec — currently sealed under old's master key at
// rec.KeyVersion — to THIS vault's (new) master-key epoch, with a fresh nonce
// and an AAD bound to the new key_version. It is the per-row primitive of the
// documented re-encrypt-all rotation (design §11.3): the operator provisions
// the new key (this vault) while the prior key stays available (old), then a
// one-shot operator-invoked driver walks every row calling Rotate and persists
// the returned record transactionally per row, bumping secret_key_version.
// That driver lands in a later scope; this primitive + the §11.3 procedure are
// SCOPE-02's rotation deliverable. Rotate NEVER logs key bytes or plaintext.
func (v *SecretVault) Rotate(old *SecretVault, rec VaultRecord) (VaultRecord, error) {
	if v == nil || v.gcm == nil || old == nil || old.gcm == nil {
		return VaultRecord{}, ErrVaultNotConfigured
	}
	bundle, err := old.Decrypt(rec)
	if err != nil {
		return VaultRecord{}, fmt.Errorf("connvault: rotate: decrypt under old epoch %d: %w", rec.KeyVersion, err)
	}
	defer clear(bundle)
	return v.Encrypt(rec.ConnectionID, rec.Kind, bundle)
}

// canonicalAAD binds a ciphertext to its connection_id, provider_kind, and
// master-key epoch (design §11.1). connection_id and provider_kind are
// colon-free slugs from the closed registry vocabulary, so the ":" join is
// unambiguous. (The "|" separator shown illustratively in scopes.md is
// superseded by the design §11.1 ":" canonical form.)
func canonicalAAD(connID, kind string, keyVersion int) []byte {
	return []byte(fmt.Sprintf("%s:%s:%d", connID, kind, keyVersion))
}

// redactionPriority is the documented per-field precedence for choosing the
// single secret value the last-4 display hint is derived from when a bundle
// carries multiple secret fields (e.g. Bedrock's two keys). It is a display
// concern only — the full secret is never exposed.
var redactionPriority = []string{"api_key", "aws_secret_access_key", "service_account", "aws_access_key_id"}

// redactionFor returns the non-secret last-4 display hint ("…wxyz") for a
// bundle, exposing ONLY the final ≤4 characters of one designated field. It
// returns "" when the chosen value is empty.
func redactionFor(bundle map[string]string) string {
	primary := primarySecret(bundle)
	r := []rune(primary)
	if len(r) == 0 {
		return ""
	}
	if len(r) > 4 {
		r = r[len(r)-4:]
	}
	return "…" + string(r)
}

// primarySecret deterministically picks the bundle value the redaction is
// derived from: the highest-priority known field, else the value of the
// lexicographically-smallest non-empty key.
func primarySecret(bundle map[string]string) string {
	for _, k := range redactionPriority {
		if val, ok := bundle[k]; ok && val != "" {
			return val
		}
	}
	keys := make([]string, 0, len(bundle))
	for k := range bundle {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if bundle[k] != "" {
			return bundle[k]
		}
	}
	return ""
}

// zero overwrites a byte slice in place — used to scrub marshaled plaintext
// and decoded key material from memory once it is no longer needed.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
