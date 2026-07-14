// Spec 096 SCOPE-03 — the provider-aware dispatch resolver (credential seam).
//
// DispatchResolver maps a selected provider-qualified model id
// ("<kind>/<backend-id>", e.g. "anthropic/claude-3-5-sonnet") to a populated
// ChatRequest: it locates the model's operator-global connection in the
// SCOPE-01 registry, confirms the connection is effective-enabled, decrypts the
// connection's credential through the SCOPE-02 SecretVault, and stamps the
// per-request Provider / APIBase / APIKey / ProviderParams onto a ChatRequest
// whose Model is the BARE backend id (the sidecar recomposes the litellm
// provider model).
//
// NEVER-FALLBACK-TO-OLLAMA (G028, FR-X1 / SCN-096-G01). A target whose
// connection is missing, disabled, lacks a stored credential, or fails
// authenticated decryption is rejected with a typed *ResolveError — the
// resolver NEVER silently substitutes the local Ollama connection for a
// hosted target. The local (no-secret) connection is resolved ONLY when the
// selected id names it.
//
// SECRET-SAFETY (design §11.5 / SCN-096-G05). The resolver holds no logger and
// emits no logs or spans. The cleartext credential lives ONLY in the returned
// ChatRequest.APIKey (the seam the sidecar consumes). No *ResolveError, and no
// field other than ChatRequest.APIKey, ever carries the secret — the error
// vocabulary is built from the typed reason + the connection identity (id /
// kind / provider-qualified model), never the credential.
package llm

import (
	"fmt"
	"sort"
	"strings"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/connvault"
	"github.com/smackerel/smackerel/internal/config"
)

// RejectReason is the typed, secret-free cause of a dispatch-resolution
// refusal. It NEVER carries credential material.
type RejectReason string

const (
	// RejectMalformedModelID — the selected id is not a "<kind>/<backend>"
	// provider-qualified pair.
	RejectMalformedModelID RejectReason = "malformed_model_id"
	// RejectConnectionNotFound — no registry connection serves the id's kind.
	RejectConnectionNotFound RejectReason = "connection_not_found"
	// RejectConnectionDisabled — the connection exists but is not enabled.
	RejectConnectionDisabled RejectReason = "connection_disabled"
	// RejectCredentialMissing — a db-mode connection has no stored credential
	// (or the decrypted bundle lacks the required secret field).
	RejectCredentialMissing RejectReason = "credential_missing"
	// RejectVaultNotConfigured — a db-mode connection was selected but no
	// SecretVault is loaded (a wiring bug — the vault is constructed whenever a
	// db-mode connection is declared).
	RejectVaultNotConfigured RejectReason = "vault_not_configured"
	// RejectDecryptFailed — authenticated decryption failed (tamper, wrong
	// master key, or AAD mismatch). The wrapped vault error never carries
	// plaintext.
	RejectDecryptFailed RejectReason = "decrypt_failed"
	// RejectInvalidConnectionParams — a connection carries a routing param
	// outside its provider-owned closed contract, or a declared param has an
	// invalid type. The supplied value is never retained in the error.
	RejectInvalidConnectionParams RejectReason = "invalid_connection_params"
)

// ResolveError is the typed, fail-loud refusal returned by Resolve. It is
// secret-free by construction: Error() renders only the reason and the
// connection identity, NEVER the credential. A refusal is NEVER a silent
// Ollama fallback — the caller MUST surface it, not retry against a local
// model.
type ResolveError struct {
	Reason RejectReason
	// Model is the provider-qualified id that was rejected.
	Model string
	// Kind is the id's provider kind (the part before the first "/").
	Kind string
	// ConnID is the resolved connection id, when one was located.
	ConnID string
	// Param is the rejected non-secret connection-param key, when applicable.
	// Its supplied value is intentionally never stored.
	Param string
	// err is the wrapped cause (e.g. the vault decryption error). The vault
	// itself never includes plaintext in its errors.
	err error
}

func (e *ResolveError) Error() string {
	reason := string(e.Reason)
	if e.Param != "" {
		reason += fmt.Sprintf(" (param=%q)", e.Param)
	}
	return fmt.Sprintf(
		"llm: dispatch resolution rejected provider-qualified model %q (kind=%q connection=%q): %s",
		e.Model, e.Kind, e.ConnID, reason)
}

// Unwrap exposes the wrapped cause for errors.Is / errors.As. The wrapped
// error (a vault decryption failure) is itself secret-free.
func (e *ResolveError) Unwrap() error { return e.err }

// ResolvedDispatch is the output of a successful Resolve: a populated
// ChatRequest plus the provider-qualified attribution string.
type ResolvedDispatch struct {
	// Request carries the per-request routing fields. Model is the BARE
	// backend id; Provider is the connection kind; APIKey (hosted only) is the
	// decrypted cleartext credential.
	Request ChatRequest
	// Attribution is the provider-qualified model id ("<kind>/<backend>") —
	// the spec 089 ModelAttribution value, carried through unchanged so an
	// answer is attributed to the provider+model that produced it and NEVER
	// coerced to a bare or Ollama name (SCN-096-G04).
	Attribution string
}

// CredentialSource yields the at-rest encrypted credential record for a
// db-mode connection. Production wiring (SCOPE-06) backs this with the DB read
// of model_provider_connections; tests inject an in-memory implementation.
// Keeping it an interface means SCOPE-03 carries no DB import and the resolver
// is unit-testable with a synthetic vault + synthetic record.
type CredentialSource interface {
	// Credential returns the encrypted vault record for connID. found=false
	// means no credential is stored — the connection is NOT effective-enabled
	// and the resolver rejects (it never falls back to Ollama).
	Credential(connID string) (rec connvault.VaultRecord, found bool)
}

// DispatchResolver resolves provider-qualified model ids against the operator-
// global registry + vault. It is built once from the SST-declared connections
// and is safe for concurrent reads (its maps are never mutated after New).
type DispatchResolver struct {
	// byKind is the at-most-one connection per provider kind (design §1
	// grammar: at-most-one-enabled-connection-per-kind). Keyed by kind.
	byKind map[string]config.ModelConnection
	vault  *connvault.SecretVault
	creds  CredentialSource
}

// NewDispatchResolver builds a resolver from the registry connections, the
// loaded vault (may be nil for an Ollama-only deployment), and the credential
// source (may be nil when no db-mode connection is declared). It is fail-loud:
// a duplicate connection kind (which would make dispatch ambiguous) aborts with
// a named error rather than silently picking one.
func NewDispatchResolver(conns []config.ModelConnection, vault *connvault.SecretVault, creds CredentialSource) (*DispatchResolver, error) {
	byKind := make(map[string]config.ModelConnection, len(conns))
	for _, c := range conns {
		if existing, dup := byKind[c.Kind]; dup {
			return nil, fmt.Errorf(
				"llm: dispatch resolver: provider kind %q is served by two connections (%q and %q); "+
					"at most one connection per kind may be declared",
				c.Kind, existing.ID, c.ID)
		}
		byKind[c.Kind] = c
	}
	return &DispatchResolver{byKind: byKind, vault: vault, creds: creds}, nil
}

// Resolve maps a provider-qualified model id to a populated ChatRequest +
// provider-qualified attribution. A not-effective-enabled or credential-less
// target yields a typed *ResolveError — NEVER a silent Ollama fallback.
func (r *DispatchResolver) Resolve(providerQualifiedModel string) (ResolvedDispatch, error) {
	kind, backend, ok := splitProviderQualified(providerQualifiedModel)
	if !ok {
		return ResolvedDispatch{}, &ResolveError{Reason: RejectMalformedModelID, Model: providerQualifiedModel}
	}
	conn, found := r.byKind[kind]
	if !found {
		return ResolvedDispatch{}, &ResolveError{Reason: RejectConnectionNotFound, Model: providerQualifiedModel, Kind: kind}
	}
	if !conn.Enabled {
		return ResolvedDispatch{}, &ResolveError{Reason: RejectConnectionDisabled, Model: providerQualifiedModel, Kind: kind, ConnID: conn.ID}
	}
	apiBase, providerParams, invalidParam := providerDispatchParams(kind, conn.Params)
	if invalidParam != "" {
		return ResolvedDispatch{}, &ResolveError{
			Reason: RejectInvalidConnectionParams,
			Model:  providerQualifiedModel,
			Kind:   kind,
			ConnID: conn.ID,
			Param:  invalidParam,
		}
	}

	// Local / no-secret connection (ollama): the byte-for-byte Ollama dispatch.
	// Reached ONLY because the selected id named this kind — never as a
	// fallback for a rejected hosted target.
	if conn.SecretRef.Mode == config.ModelConnectionSecretModeNone {
		req := ChatRequest{Model: backend, Provider: kind}
		if apiBase != "" {
			b := apiBase
			req.APIBase = &b
		}
		return ResolvedDispatch{Request: req, Attribution: providerQualifiedModel}, nil
	}

	// Hosted db-mode connection: decrypt the credential. A missing credential
	// or a failed decryption is a typed rejection, NEVER an Ollama fallback.
	rec, hasCred := r.lookupCredential(conn.ID)
	if !hasCred {
		return ResolvedDispatch{}, &ResolveError{Reason: RejectCredentialMissing, Model: providerQualifiedModel, Kind: kind, ConnID: conn.ID}
	}
	if r.vault == nil {
		return ResolvedDispatch{}, &ResolveError{Reason: RejectVaultNotConfigured, Model: providerQualifiedModel, Kind: kind, ConnID: conn.ID}
	}
	bundle, err := r.vault.Decrypt(rec)
	if err != nil {
		// The vault error never includes plaintext; wrap it for diagnosis.
		return ResolvedDispatch{}, &ResolveError{Reason: RejectDecryptFailed, Model: providerQualifiedModel, Kind: kind, ConnID: conn.ID, err: err}
	}
	apiKey := strings.TrimSpace(bundle["api_key"])
	if apiKey == "" {
		return ResolvedDispatch{}, &ResolveError{Reason: RejectCredentialMissing, Model: providerQualifiedModel, Kind: kind, ConnID: conn.ID}
	}

	req := ChatRequest{
		Model:    backend, // BARE backend id; sidecar recomposes "<kind>/<backend>".
		Provider: kind,
	}
	key := apiKey
	req.APIKey = &key
	if apiBase != "" {
		b := apiBase
		req.APIBase = &b
	}
	if len(providerParams) > 0 {
		req.ProviderParams = providerParams
	}
	return ResolvedDispatch{Request: req, Attribution: providerQualifiedModel}, nil
}

// lookupCredential is the nil-safe credential fetch: a resolver with no
// CredentialSource (Ollama-only wiring) reports "no credential" rather than
// panicking.
func (r *DispatchResolver) lookupCredential(connID string) (connvault.VaultRecord, bool) {
	if r.creds == nil {
		return connvault.VaultRecord{}, false
	}
	return r.creds.Credential(connID)
}

// splitProviderQualified splits "<kind>/<backend-id>" on the FIRST "/". Both
// halves MUST be non-empty. (The full catalog canonicalization — bare-Ollama
// normalization, off-catalog rejection — is the SCOPE-04 resolver-boundary
// concern; here we only need the kind→connection + backend mapping for an
// already-provider-qualified id.)
func splitProviderQualified(id string) (kind, backend string, ok bool) {
	id = strings.TrimSpace(id)
	k, b, found := strings.Cut(id, "/")
	k = strings.TrimSpace(k)
	b = strings.TrimSpace(b)
	if !found || k == "" || b == "" {
		return "", "", false
	}
	return k, b, true
}

type providerDispatchContract struct {
	apiBaseKey string
	params     map[string]string
}

// providerDispatchContracts is the only connection-param surface allowed onto
// the sidecar wire. Source keys are SST names; values are the LiteLLM names.
// Ollama-only controls such as options/keep_alive and generic controls such as
// extra_headers/timeout are absent by design and therefore rejected.
var providerDispatchContracts = map[string]providerDispatchContract{
	config.ModelConnectionKindOllama:       {apiBaseKey: "base_url"},
	config.ModelConnectionKindAnthropic:    {},
	config.ModelConnectionKindOpenAI:       {apiBaseKey: "base_url", params: map[string]string{"org": "organization"}},
	config.ModelConnectionKindAzureFoundry: {apiBaseKey: "endpoint", params: map[string]string{"api_version": "api_version", "deployment": "deployment"}},
	config.ModelConnectionKindGoogle:       {params: map[string]string{"project": "project", "location": "location"}},
	config.ModelConnectionKindBedrock:      {params: map[string]string{"region": "region"}},
}

// providerDispatchParams validates and translates one connection's generic SST
// params into its closed per-provider wire contract. invalidParam is a key only;
// supplied values never enter errors or logs.
func providerDispatchParams(kind string, params map[string]any) (apiBase string, providerParams map[string]any, invalidParam string) {
	contract, known := providerDispatchContracts[kind]
	if !known {
		return "", nil, "kind"
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		wireKey := ""
		isAPIBase := key == contract.apiBaseKey && contract.apiBaseKey != ""
		if !isAPIBase {
			wireKey = contract.params[key]
			if wireKey == "" {
				return "", nil, key
			}
		}
		value, ok := params[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			return "", nil, key
		}
		value = strings.TrimSpace(value)
		if isAPIBase {
			apiBase = value
			continue
		}
		if providerParams == nil {
			providerParams = make(map[string]any, len(contract.params))
		}
		providerParams[wireKey] = value
	}
	return apiBase, providerParams, ""
}
