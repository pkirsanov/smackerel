// Package google provides the Google Drive concrete implementation of the
// drive.Provider contract. Spec 038 Scope 1 ships a real Capabilities()
// surface backed by a config-injected struct plus scaffold stubs for the
// behavior-bearing methods. The behavior-bearing methods (BeginConnect,
// FinalizeConnect, ListFolder, GetFile, PutFile, Changes, full Health
// snapshot) return drive.ErrNotImplemented so later scopes (BeginConnect +
// FinalizeConnect later in Scope 1, scan/monitor in Scope 2, save/get in
// Scope 5+, retrieval in Scope 7) MUST land their behavior explicitly
// rather than inheriting silent stubs.
//
// Production code in later scopes will exercise the real Google Drive API
// through the recorded owned-fixture boundary described in design §8.3.
package google

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/drive"
)

// providerID is the stable identifier used in config (drive.providers.google)
// and persisted to drive_connections.provider_id.
const providerID = "google"

// googleAPIHardCeilingBytes is Google Drive's published per-file upload
// ceiling (5 TiB). It is the ABSOLUTE hard ceiling — runtime wiring SHOULD
// always pass a tighter value sourced from drive.limits.max_file_size_bytes
// via NewFromConfig. The init() registration uses this ceiling so the
// connectors-list endpoint can advertise the provider before configuration
// is loaded; runtime wiring MUST overwrite via Configure once config has
// been parsed.
const googleAPIHardCeilingBytes int64 = 5 * 1024 * 1024 * 1024 * 1024

// Provider is the Google Drive concrete implementation. Capabilities are
// injected through New / NewFromConfig / Configure so MaxFileSizeBytes
// reflects the SST-resolved drive.limits.max_file_size_bytes value rather
// than a hardcoded constant in this file.
//
// Runtime dependencies (database pool, HTTP client, OAuth + API base URLs,
// client secrets) are injected through the Google-specific ConfigureRuntime
// setter rather than added to the drive.Provider interface, so the
// provider-neutral contract stays free of provider-specific runtime knobs.
type Provider struct {
	caps   drive.Capabilities
	pool   *pgxpool.Pool
	client *http.Client
	cfg    config.DriveGoogleProviderConfig
}

// DefaultCapabilities returns the platform-default Google Drive capabilities
// used at init() time before runtime config has been loaded. MaxFileSizeBytes
// uses the published Google Drive ceiling of 5 TiB; runtime wiring MUST
// overwrite this with the configured limit.
func DefaultCapabilities() drive.Capabilities {
	return drive.Capabilities{
		SupportsVersions:      true,
		SupportsSharing:       true,
		SupportsChangeHistory: true,
		MaxFileSizeBytes:      googleAPIHardCeilingBytes,
		// Google Drive accepts arbitrary MIME types. The save service
		// applies tighter per-rule MIME filters from configuration; this
		// provider-level filter stays nil to advertise that breadth.
		SupportedMimeFilter: nil,
	}
}

// New returns a Google Provider with the supplied capabilities. Tests SHOULD
// use this constructor with explicit capabilities so they can assert the
// config-injection path. Production wiring SHOULD use NewFromConfig.
func New(caps drive.Capabilities) *Provider {
	return &Provider{caps: caps}
}

// NewFromConfig returns a Google Provider whose Capabilities reflect the
// runtime SST-resolved drive limits. Callers pass scalar values rather than
// importing internal/config to keep this package free of upstream-config
// imports (and to avoid an import cycle when other internal/drive consumers
// depend on this package). Runtime wiring constructs values from
// internal/config.DriveConfig.
//
//	maxFileSizeBytes — drive.limits.max_file_size_bytes from SST. Values
//	  <= 0 fall back to googleAPIHardCeilingBytes so the provider always
//	  advertises a non-zero ceiling.
//	supportedMimeFilter — provider-level MIME allowlist. nil means accept
//	  any MIME type; per-save-rule MIME guardrails are enforced separately.
func NewFromConfig(maxFileSizeBytes int64, supportedMimeFilter []string) *Provider {
	caps := DefaultCapabilities()
	if maxFileSizeBytes > 0 {
		caps.MaxFileSizeBytes = maxFileSizeBytes
	}
	if supportedMimeFilter != nil {
		caps.SupportedMimeFilter = supportedMimeFilter
	}
	return New(caps)
}

// Configure overwrites the provider's capabilities. This is the runtime
// wiring path: init() registers a provider with DefaultCapabilities, and
// main wiring calls Configure once config is loaded so the connectors-list
// endpoint advertises the SST-resolved MaxFileSizeBytes. Tests can also
// use Configure to swap capabilities mid-test.
func (p *Provider) Configure(caps drive.Capabilities) {
	p.caps = caps
}

// ConfigureRuntime injects the runtime dependencies the Google provider
// needs to perform real OAuth + Drive API calls (database pool for
// drive_oauth_states + drive_connections persistence, HTTP client for
// provider calls, and the SST-resolved Google provider config block).
//
// This is intentionally a Google-provider-specific setter rather than a
// drive.Provider interface method: provider-neutral wiring belongs in
// the registry; provider-specific runtime deps belong on the concrete
// type. Callers obtain the Google provider via
// drive.DefaultRegistry.Get("google") and type-assert (or, in tests,
// construct via google.New(...).ConfigureRuntime(...)).
//
// ConfigureRuntime returns the receiver so it composes with New.
// Repeated calls overwrite the previously-configured deps.
func (p *Provider) ConfigureRuntime(pool *pgxpool.Pool, client *http.Client, cfg config.DriveGoogleProviderConfig) *Provider {
	p.pool = pool
	p.client = client
	p.cfg = cfg
	return p
}

// ID implements drive.Provider.
func (p *Provider) ID() string { return providerID }

// DisplayName implements drive.Provider.
func (p *Provider) DisplayName() string { return "Google Drive" }

// Capabilities implements drive.Provider. The advertised values come from
// the config-injected struct supplied at construction (or via Configure)
// so MaxFileSizeBytes reflects drive.limits.max_file_size_bytes from SST.
func (p *Provider) Capabilities() drive.Capabilities {
	return p.caps
}

// BeginConnect implements drive.Provider. It generates a cryptographically
// random state token, persists the (owner, provider, accessMode, scope)
// tuple to drive_oauth_states keyed by that token, and returns the
// provider authorization URL plus the state token. Runtime deps must
// have been wired through ConfigureRuntime; otherwise the call fails
// loudly.
func (p *Provider) BeginConnect(ctx context.Context, mode drive.AccessMode, scope drive.Scope) (string, string, error) {
	if err := mode.Validate(); err != nil {
		return "", "", err
	}
	if p.pool == nil {
		return "", "", fmt.Errorf("google: BeginConnect called before ConfigureRuntime (pool is nil)")
	}
	if p.cfg.OAuthBaseURL == "" || p.cfg.OAuthRedirectURL == "" {
		return "", "", fmt.Errorf("google: BeginConnect called with empty oauth_base_url or oauth_redirect_url")
	}
	owner, err := drive.OwnerUserIDFromContext(ctx)
	if err != nil {
		return "", "", err
	}

	state, err := randomStateToken()
	if err != nil {
		return "", "", fmt.Errorf("google: generate state: %w", err)
	}
	scopeJSON, err := json.Marshal(map[string]any{
		"folder_ids":     scope.FolderIDs,
		"include_shared": scope.IncludeShared,
	})
	if err != nil {
		return "", "", fmt.Errorf("google: marshal scope: %w", err)
	}
	expires := time.Now().Add(15 * time.Minute)
	if _, err := p.pool.Exec(ctx,
		`INSERT INTO drive_oauth_states (state_token, owner_user_id, provider_id, access_mode, scope, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		state, owner, providerID, string(mode), scopeJSON, expires,
	); err != nil {
		return "", "", fmt.Errorf("google: persist oauth state: %w", err)
	}

	q := url.Values{}
	q.Set("client_id", p.cfg.OAuthClientID)
	q.Set("redirect_uri", p.cfg.OAuthRedirectURL)
	q.Set("response_type", "code")
	q.Set("access_type", "offline")
	q.Set("state", state)
	if len(p.cfg.ScopeDefaults) > 0 {
		q.Set("scope", strings.Join(p.cfg.ScopeDefaults, " "))
	}
	authURL := strings.TrimRight(p.cfg.OAuthBaseURL, "/") + "/oauth2/auth?" + q.Encode()
	return authURL, state, nil
}

// FinalizeConnect implements drive.Provider. It consumes the state row
// persisted by BeginConnect, exchanges the authorization code for
// provider tokens against drive.providers.google.oauth_base_url, fetches
// the bound user identity from drive.providers.google.api_base_url, and
// inserts a healthy drive_connections row referencing the access token
// (stored in credentials_ref). The drive_oauth_states row is deleted
// after a successful insert.
func (p *Provider) FinalizeConnect(ctx context.Context, state string, code string) (string, error) {
	if p.pool == nil {
		return "", fmt.Errorf("google: FinalizeConnect called before ConfigureRuntime (pool is nil)")
	}
	if p.client == nil {
		return "", fmt.Errorf("google: FinalizeConnect called before ConfigureRuntime (client is nil)")
	}
	if state == "" || code == "" {
		return "", fmt.Errorf("google: FinalizeConnect requires non-empty state and code")
	}

	var (
		owner      string
		accessMode string
		scopeJSON  []byte
		expiresAt  time.Time
	)
	if err := p.pool.QueryRow(ctx,
		`SELECT owner_user_id::text, access_mode, scope, expires_at
		   FROM drive_oauth_states WHERE state_token=$1`,
		state,
	).Scan(&owner, &accessMode, &scopeJSON, &expiresAt); err != nil {
		return "", fmt.Errorf("google: lookup oauth state: %w", err)
	}
	if time.Now().After(expiresAt) {
		_, _ = p.pool.Exec(ctx, `DELETE FROM drive_oauth_states WHERE state_token=$1`, state)
		return "", fmt.Errorf("google: oauth state %s expired at %s", state, expiresAt.Format(time.RFC3339))
	}

	tokenResp, err := p.exchangeCodeForToken(ctx, code)
	if err != nil {
		return "", err
	}

	accountLabel, err := p.fetchAccountEmail(ctx, tokenResp.AccessToken)
	if err != nil {
		return "", err
	}

	connID := uuid.New()
	var tokenExpiresAt *time.Time
	if tokenResp.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		tokenExpiresAt = &t
	}

	// credentials_ref carries the bearer token plaintext for Scope 1; a
	// proper credentials vault lands in Scope 6 (design §10 / decision
	// B2). Persisting the bearer token here is what enables Health() to
	// re-issue Drive API calls without a separate token-store lookup.
	credsRef := "bearer:" + tokenResp.AccessToken

	if _, err := p.pool.Exec(ctx,
		`INSERT INTO drive_connections
		 (id, provider_id, owner_user_id, account_label, access_mode, status,
		  scope, credentials_ref, expires_at)
		 VALUES ($1, $2, $3, $4, $5, 'healthy', $6, $7, $8)`,
		connID, providerID, owner, accountLabel, accessMode,
		scopeJSON, credsRef, tokenExpiresAt,
	); err != nil {
		return "", fmt.Errorf("google: insert drive_connections: %w", err)
	}
	if _, err := p.pool.Exec(ctx,
		`DELETE FROM drive_oauth_states WHERE state_token=$1`, state,
	); err != nil {
		return "", fmt.Errorf("google: delete consumed oauth state: %w", err)
	}
	return connID.String(), nil
}

// googleTokenResponse mirrors the subset of Google's OAuth token-exchange
// response that the provider consumes. Unknown fields are ignored by
// encoding/json so future provider response additions stay compatible.
type googleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

func (p *Provider) exchangeCodeForToken(ctx context.Context, code string) (googleTokenResponse, error) {
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", p.cfg.OAuthClientID)
	form.Set("client_secret", p.cfg.OAuthClientSecret)
	form.Set("redirect_uri", p.cfg.OAuthRedirectURL)
	form.Set("grant_type", "authorization_code")

	tokenURL := strings.TrimRight(p.cfg.OAuthBaseURL, "/") + "/oauth2/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return googleTokenResponse{}, fmt.Errorf("google: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return googleTokenResponse{}, fmt.Errorf("google: token exchange: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return googleTokenResponse{}, fmt.Errorf("google: token exchange status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed googleTokenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return googleTokenResponse{}, fmt.Errorf("google: decode token response: %w", err)
	}
	if parsed.AccessToken == "" {
		return googleTokenResponse{}, fmt.Errorf("google: token response missing access_token")
	}
	return parsed, nil
}

func (p *Provider) fetchAccountEmail(ctx context.Context, accessToken string) (string, error) {
	if p.cfg.APIBaseURL == "" {
		return "", fmt.Errorf("google: api_base_url not configured")
	}
	aboutURL := strings.TrimRight(p.cfg.APIBaseURL, "/") + "/drive/v3/about"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, aboutURL, nil)
	if err != nil {
		return "", fmt.Errorf("google: build about request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("google: drive about call: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google: drive about status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed struct {
		User struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("google: decode about response: %w", err)
	}
	if parsed.User.EmailAddress == "" {
		return "", fmt.Errorf("google: about response missing user.emailAddress")
	}
	return parsed.User.EmailAddress, nil
}

func randomStateToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Disconnect implements drive.Provider.
func (p *Provider) Disconnect(_ context.Context, _ string) error {
	return drive.ErrNotImplemented
}

// Scope implements drive.Provider.
func (p *Provider) Scope(_ context.Context, _ string) (drive.Scope, error) {
	return drive.Scope{}, drive.ErrNotImplemented
}

// SetScope implements drive.Provider.
func (p *Provider) SetScope(_ context.Context, _ string, _ drive.Scope) error {
	return drive.ErrNotImplemented
}

// ListFolder implements drive.Provider.
func (p *Provider) ListFolder(_ context.Context, _ string, _ string, _ string) ([]drive.FolderItem, string, error) {
	return nil, "", drive.ErrNotImplemented
}

// GetFile implements drive.Provider.
func (p *Provider) GetFile(_ context.Context, _ string, _ string) (drive.FileBytes, error) {
	return drive.FileBytes{}, drive.ErrNotImplemented
}

// PutFile implements drive.Provider.
func (p *Provider) PutFile(_ context.Context, _ string, _ string, _ string, _ drive.FileBytes) (string, error) {
	return "", drive.ErrNotImplemented
}

// Changes implements drive.Provider.
func (p *Provider) Changes(_ context.Context, _ string, _ string) ([]drive.Change, string, error) {
	return nil, "", drive.ErrNotImplemented
}

// Health implements drive.Provider. When ConfigureRuntime has wired the
// pool + http client, Health performs a live GET against
// {api_base_url}/drive/v3/about with the stored access token and returns
// HealthHealthy on a 2xx response. Without runtime deps wired, Health
// falls back to reporting disconnected so the connectors UI can render
// the "not connected" empty state cleanly during early-bootstrap calls.
func (p *Provider) Health(ctx context.Context, connectionID string) (drive.Health, error) {
	if p.pool == nil || p.client == nil {
		return drive.Health{Status: drive.HealthDisconnected, Reason: "scaffold: runtime deps not wired"}, nil
	}
	var credsRef string
	if err := p.pool.QueryRow(ctx,
		`SELECT credentials_ref FROM drive_connections WHERE id=$1`,
		connectionID,
	).Scan(&credsRef); err != nil {
		return drive.Health{Status: drive.HealthDisconnected, Reason: "connection not found", ObservedAt: time.Now()}, fmt.Errorf("google: lookup connection: %w", err)
	}
	if !strings.HasPrefix(credsRef, "bearer:") {
		return drive.Health{Status: drive.HealthDegraded, Reason: "credentials_ref unsupported format", ObservedAt: time.Now()}, nil
	}
	if _, err := p.fetchAccountEmail(ctx, strings.TrimPrefix(credsRef, "bearer:")); err != nil {
		return drive.Health{Status: drive.HealthFailing, Reason: err.Error(), ObservedAt: time.Now()}, nil
	}
	return drive.Health{Status: drive.HealthHealthy, Reason: "about call succeeded", ObservedAt: time.Now()}, nil
}

// init registers a Google provider with DefaultCapabilities into the
// package-default registry so the connectors-list endpoint can advertise it
// before runtime config is loaded. Runtime wiring SHOULD locate this
// provider via drive.DefaultRegistry.Get("google") and call Configure with
// the SST-resolved capabilities. This mirrors the established init()
// pattern in internal/agent/registry.go. Tests that need an isolated
// registry should construct one with drive.NewRegistry() and avoid the
// package default.
func init() {
	drive.DefaultRegistry.Register(New(DefaultCapabilities()))
}
