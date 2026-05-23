package qfdecisions

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// CallbackSigningKeysEnvVar is the SST-managed environment variable that
// carries the JSON array of HMAC bridge signing keys for the QF
// signed-callback transport. The connector reads this variable at
// Connect time and constructs a CallbackKeystore. The deploy adapter
// MUST populate this value from the operator's secret store; the
// reference SST YAML key in config/smackerel.yaml
// (connectors.qf-decisions.callback_signing_keys_json) is a
// documentation placeholder for operator awareness — the runtime value
// comes from the environment variable so per-environment secret
// rotation does not require a YAML edit.
//
// Format (JSON array, UTF-8):
//
//	[
//	  {"key_id":"qf-callback-2026-05-A","secret":"<hex|base64|raw>","not_before":"2026-05-01T00:00:00Z"},
//	  {"key_id":"qf-callback-2026-06-A","secret":"<hex|base64|raw>","not_before":"2026-06-01T00:00:00Z"}
//	]
//
// SCN-SM-041-028. See docs/Operations.md "QF Callback Signing Key
// Rotation Playbook" for the operator rotation workflow.
const CallbackSigningKeysEnvVar = "QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON"

// CallbackSigningKey is a single HMAC bridge secret entry in the
// Scope 8 callback signing key store. SCN-SM-041-028.
type CallbackSigningKey struct {
	KeyID     string    `json:"key_id"`
	Secret    string    `json:"secret"`
	NotBefore time.Time `json:"not_before"`
}

// CallbackKeystore is the in-process HMAC key store used by the
// Scope 8 signed-callback signer. SCN-SM-041-028.
//
// Construction-time invariants (enforced by LoadCallbackKeystoreFromJSON):
//   - Input MUST be a non-empty JSON array.
//   - Every entry MUST carry non-empty key_id, non-empty secret, and a
//     parseable RFC3339 not_before timestamp.
//   - Duplicate key_ids are rejected.
//   - Entries are stored sorted descending by NotBefore.
//
// Selection-time invariant (enforced by SelectActiveKey):
//   - SelectActiveKey returns the newest key whose NotBefore <= now.
//   - If every key's NotBefore is in the future, ErrNoActiveCallbackKey
//     is returned and the signing call site MUST abort locally and
//     record the signature-failure metric + audit envelope
//     (reason=NO_ACTIVE_KEY).
//
// The keystore is immutable after construction; key rotation is
// performed by restarting the connector with an updated SST config
// bundle that includes the new key entry. See docs/Operations.md
// "QF Callback Signing Key Rotation Playbook" for the operator
// workflow.
type CallbackKeystore struct {
	keys []CallbackSigningKey
}

// ErrNoActiveCallbackKey is returned by SelectActiveKey when no key
// has not_before <= now. The signing call site MUST emit
// smackerel_qf_callback_signature_failures_total{reason="NO_ACTIVE_KEY"}
// and the Cross-Product Audit Envelope v1 record describing the
// callback_attempt outcome=rejected reason=NO_ACTIVE_KEY.
var ErrNoActiveCallbackKey = errors.New("qf-decisions: no active callback signing key (every key not_before is in the future or keystore is empty)")

// LoadCallbackKeystoreFromJSON parses the JSON array form of the
// keystore and constructs a CallbackKeystore. Returns an error if the
// JSON is empty, malformed, the array is empty, or any entry is
// missing required fields. SCN-SM-041-028.
func LoadCallbackKeystoreFromJSON(raw string) (*CallbackKeystore, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("qf-decisions: callback signing keystore JSON is empty")
	}
	var entries []CallbackSigningKey
	if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
		return nil, fmt.Errorf("qf-decisions: callback signing keystore JSON unmarshal: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("qf-decisions: callback signing keystore JSON array is empty")
	}
	seen := make(map[string]struct{}, len(entries))
	cleaned := make([]CallbackSigningKey, 0, len(entries))
	for i, entry := range entries {
		entry.KeyID = strings.TrimSpace(entry.KeyID)
		entry.Secret = strings.TrimSpace(entry.Secret)
		if entry.KeyID == "" {
			return nil, fmt.Errorf("qf-decisions: callback signing keystore entry %d missing key_id", i)
		}
		if entry.Secret == "" {
			return nil, fmt.Errorf("qf-decisions: callback signing keystore entry %d (key_id=%s) missing secret", i, entry.KeyID)
		}
		if entry.NotBefore.IsZero() {
			return nil, fmt.Errorf("qf-decisions: callback signing keystore entry %d (key_id=%s) missing not_before", i, entry.KeyID)
		}
		entry.NotBefore = entry.NotBefore.UTC()
		if _, dup := seen[entry.KeyID]; dup {
			return nil, fmt.Errorf("qf-decisions: callback signing keystore has duplicate key_id %q", entry.KeyID)
		}
		seen[entry.KeyID] = struct{}{}
		cleaned = append(cleaned, entry)
	}
	sort.SliceStable(cleaned, func(i, j int) bool {
		return cleaned[i].NotBefore.After(cleaned[j].NotBefore)
	})
	return &CallbackKeystore{keys: cleaned}, nil
}

// LoadCallbackKeystoreFromEnv reads the SST-managed environment
// variable QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON and constructs a
// CallbackKeystore. Returns (nil, nil) when the environment variable
// is empty or unset — the connector treats this as "callback signing
// not configured" rather than a fatal error so the QF connector can
// run with engagement, evidence, and read-only flows wired even when
// the pre-MVP callback transport is intentionally not exercised in a
// particular environment. Returns (nil, error) when the variable is
// set but malformed or invalid — startup MUST fail loud in that case
// because the operator declared an intent to enable signing but the
// configuration is broken.
//
// The fail-loud-when-all-keys-future invariant is enforced lazily by
// SelectActiveKey at the first signing call site (see callback.go
// CallbackSigner.Sign). Connect-time validation can optionally probe
// SelectActiveKey(now) once to surface the failure during startup;
// see internal/connector/qfdecisions/connector.go Connect. SCN-SM-041-028.
func LoadCallbackKeystoreFromEnv() (*CallbackKeystore, error) {
	raw := strings.TrimSpace(os.Getenv(CallbackSigningKeysEnvVar))
	if raw == "" {
		return nil, nil
	}
	return LoadCallbackKeystoreFromJSON(raw)
}

// SelectActiveKey returns the newest key whose NotBefore <= now (in
// UTC). Returns ErrNoActiveCallbackKey when every key's NotBefore is
// strictly after now, or when the keystore is empty/nil.
// SCN-SM-041-028.
func (s *CallbackKeystore) SelectActiveKey(now time.Time) (CallbackSigningKey, error) {
	if s == nil || len(s.keys) == 0 {
		return CallbackSigningKey{}, ErrNoActiveCallbackKey
	}
	cmp := now.UTC()
	for _, k := range s.keys {
		if !k.NotBefore.After(cmp) {
			return k, nil
		}
	}
	return CallbackSigningKey{}, ErrNoActiveCallbackKey
}

// KeyIDs returns the list of key_ids in the keystore (descending by
// NotBefore). Exposed for diagnostic logging and tests; the signing
// call site MUST go through SelectActiveKey.
func (s *CallbackKeystore) KeyIDs() []string {
	if s == nil {
		return nil
	}
	ids := make([]string, len(s.keys))
	for i, k := range s.keys {
		ids[i] = k.KeyID
	}
	return ids
}

// Len returns the count of keys in the keystore. A nil keystore reports 0.
func (s *CallbackKeystore) Len() int {
	if s == nil {
		return 0
	}
	return len(s.keys)
}
