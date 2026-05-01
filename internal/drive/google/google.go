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
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
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
func (p *Provider) Scope(ctx context.Context, connectionID string) (drive.Scope, error) {
	if p.pool == nil {
		return drive.Scope{}, fmt.Errorf("google: Scope called before ConfigureRuntime (pool is nil)")
	}
	var scopeJSON []byte
	if err := p.pool.QueryRow(ctx, `SELECT scope FROM drive_connections WHERE id=$1`, connectionID).Scan(&scopeJSON); err != nil {
		return drive.Scope{}, fmt.Errorf("google: load scope: %w", err)
	}
	var payload struct {
		FolderIDs     []string `json:"folder_ids"`
		IncludeShared bool     `json:"include_shared"`
	}
	if len(scopeJSON) > 0 {
		if err := json.Unmarshal(scopeJSON, &payload); err != nil {
			return drive.Scope{}, fmt.Errorf("google: decode scope: %w", err)
		}
	}
	return drive.Scope{FolderIDs: payload.FolderIDs, IncludeShared: payload.IncludeShared}, nil
}

// SetScope implements drive.Provider.
func (p *Provider) SetScope(ctx context.Context, connectionID string, scope drive.Scope) error {
	if p.pool == nil {
		return fmt.Errorf("google: SetScope called before ConfigureRuntime (pool is nil)")
	}
	scopeJSON, err := json.Marshal(map[string]any{"folder_ids": scope.FolderIDs, "include_shared": scope.IncludeShared})
	if err != nil {
		return fmt.Errorf("google: marshal scope: %w", err)
	}
	if _, err := p.pool.Exec(ctx, `UPDATE drive_connections SET scope=$2, updated_at=now() WHERE id=$1`, connectionID, scopeJSON); err != nil {
		return fmt.Errorf("google: update scope: %w", err)
	}
	return nil
}

// ListFolder implements drive.Provider.
func (p *Provider) ListFolder(ctx context.Context, connectionID string, folderID string, pageToken string) ([]drive.FolderItem, string, error) {
	if folderID == "" {
		folderID = "root"
	}
	accessToken, err := p.accessToken(ctx, connectionID)
	if err != nil {
		return nil, "", err
	}
	baseURL, err := url.Parse(strings.TrimRight(p.cfg.APIBaseURL, "/") + "/drive/v3/files")
	if err != nil {
		return nil, "", fmt.Errorf("google: parse files URL: %w", err)
	}
	query := baseURL.Query()
	query.Set("q", "'"+folderID+"' in parents and trashed = false")
	query.Set("pageSize", "100")
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	query.Set("fields", "nextPageToken,files(id,name,mimeType,size,parents,webViewLink,modifiedTime,headRevisionId,owners,sharingUser,shared,trashed,appProperties)")
	baseURL.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("google: build list request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("google: list folder: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("google: list folder status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed googleFilesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, "", fmt.Errorf("google: decode files response: %w", err)
	}
	items := make([]drive.FolderItem, 0, len(parsed.Files))
	for _, file := range parsed.Files {
		items = append(items, file.toFolderItem())
	}
	return items, parsed.NextPageToken, nil
}

// GetFile implements drive.Provider.
func (p *Provider) GetFile(ctx context.Context, connectionID string, providerFileID string) (drive.FileBytes, error) {
	accessToken, err := p.accessToken(ctx, connectionID)
	if err != nil {
		return drive.FileBytes{}, err
	}
	fileURL := strings.TrimRight(p.cfg.APIBaseURL, "/") + "/drive/v3/files/" + path.Clean(providerFileID)
	reqURL, err := url.Parse(fileURL)
	if err != nil {
		return drive.FileBytes{}, fmt.Errorf("google: parse file URL: %w", err)
	}
	query := reqURL.Query()
	query.Set("alt", "media")
	reqURL.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return drive.FileBytes{}, fmt.Errorf("google: build get file request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := p.client.Do(req)
	if err != nil {
		return drive.FileBytes{}, fmt.Errorf("google: get file: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return drive.FileBytes{}, fmt.Errorf("google: get file status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return drive.FileBytes{MimeType: mediaType(resp.Header.Get("Content-Type")), Reader: resp.Body, Size: resp.ContentLength}, nil
}

// PutFile implements drive.Provider.
//
// Spec 038 Scope 5 ships PutFile against the owned fixture surface
// described in design §8.3 and tests/integration/drive/fixtures/server.go.
// The fixture exposes a JSON-bodied upload endpoint at
// {api_base_url}/upload/drive/v3/files; the runtime client wraps the
// FileBytes payload in a base64 envelope so the fixture can store and
// audit it without parsing real Google multipart-related bodies. Real
// Google Drive integration (POST /upload/drive/v3/files?uploadType=multipart
// with related-multipart bodies) is a Scope 8 follow-up.
func (p *Provider) PutFile(ctx context.Context, connectionID string, folderID string, title string, body drive.FileBytes) (string, error) {
	if connectionID == "" {
		return "", fmt.Errorf("google: PutFile: connection_id required")
	}
	if folderID == "" {
		return "", fmt.Errorf("google: PutFile: folder_id required")
	}
	if title == "" {
		return "", fmt.Errorf("google: PutFile: title required")
	}
	accessToken, err := p.accessToken(ctx, connectionID)
	if err != nil {
		return "", err
	}
	defer func() {
		if body.Reader != nil {
			_ = body.Reader.Close()
		}
	}()
	data, err := io.ReadAll(body.Reader)
	if err != nil {
		return "", fmt.Errorf("google: PutFile: read bytes: %w", err)
	}
	envelope := map[string]string{
		"folder_id": folderID,
		"title":     title,
		"mime_type": body.MimeType,
		"data_b64":  encodeBase64(data),
	}
	envelopeBytes, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("google: PutFile: marshal envelope: %w", err)
	}
	uploadURL := strings.TrimRight(p.cfg.APIBaseURL, "/") + "/upload/drive/v3/files"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, strings.NewReader(string(envelopeBytes)))
	if err != nil {
		return "", fmt.Errorf("google: PutFile: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("google: PutFile: do request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google: PutFile status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("google: PutFile: decode response: %w", err)
	}
	if parsed.ID == "" {
		return "", fmt.Errorf("google: PutFile: empty id in response")
	}
	return parsed.ID, nil
}

// EnsureFolder is the Spec 038 Scope 5 FolderEnsurer hook used by the Save
// Service. The runtime client delegates to the fixture-backed
// {api_base_url}/drive/v3/folders surface: GET first, fall back to POST on
// 404. Concurrent callers MAY race here; the Save Service guarantees
// exactly-one durable mapping via drive_folder_resolutions, and the
// fixture's create counter records every attempt so tests can prove BS-016.
func (p *Provider) EnsureFolder(ctx context.Context, connectionID string, folderPath string) (string, error) {
	if connectionID == "" {
		return "", fmt.Errorf("google: EnsureFolder: connection_id required")
	}
	cleaned := strings.Trim(folderPath, "/")
	if cleaned == "" {
		return "", fmt.Errorf("google: EnsureFolder: folder_path required")
	}
	accessToken, err := p.accessToken(ctx, connectionID)
	if err != nil {
		return "", err
	}
	base := strings.TrimRight(p.cfg.APIBaseURL, "/") + "/drive/v3/folders"

	// GET first.
	getURL := base + "?path=" + url.QueryEscape(cleaned)
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, getURL, nil)
	if err != nil {
		return "", fmt.Errorf("google: EnsureFolder: build GET: %w", err)
	}
	getReq.Header.Set("Authorization", "Bearer "+accessToken)
	getResp, err := p.client.Do(getReq)
	if err != nil {
		return "", fmt.Errorf("google: EnsureFolder: GET: %w", err)
	}
	if getResp.StatusCode == http.StatusOK {
		defer getResp.Body.Close()
		var parsed struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(getResp.Body).Decode(&parsed); err != nil {
			return "", fmt.Errorf("google: EnsureFolder: decode GET: %w", err)
		}
		if parsed.ID != "" {
			return parsed.ID, nil
		}
	} else if getResp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(getResp.Body)
		getResp.Body.Close()
		return "", fmt.Errorf("google: EnsureFolder: GET status %d: %s", getResp.StatusCode, strings.TrimSpace(string(body)))
	} else {
		getResp.Body.Close()
	}

	// POST to create.
	createBody, err := json.Marshal(map[string]string{"path": cleaned})
	if err != nil {
		return "", fmt.Errorf("google: EnsureFolder: marshal create body: %w", err)
	}
	postReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base, strings.NewReader(string(createBody)))
	if err != nil {
		return "", fmt.Errorf("google: EnsureFolder: build POST: %w", err)
	}
	postReq.Header.Set("Authorization", "Bearer "+accessToken)
	postReq.Header.Set("Content-Type", "application/json")
	postResp, err := p.client.Do(postReq)
	if err != nil {
		return "", fmt.Errorf("google: EnsureFolder: POST: %w", err)
	}
	defer postResp.Body.Close()
	postBody, _ := io.ReadAll(postResp.Body)
	if postResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google: EnsureFolder: POST status %d: %s", postResp.StatusCode, strings.TrimSpace(string(postBody)))
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(postBody, &parsed); err != nil {
		return "", fmt.Errorf("google: EnsureFolder: decode POST: %w", err)
	}
	if parsed.ID == "" {
		return "", fmt.Errorf("google: EnsureFolder: empty id in POST response")
	}
	return parsed.ID, nil
}

// Changes implements drive.Provider.
func (p *Provider) Changes(ctx context.Context, connectionID string, cursor string) ([]drive.Change, string, error) {
	accessToken, err := p.accessToken(ctx, connectionID)
	if err != nil {
		return nil, "", err
	}
	changesURL, err := url.Parse(strings.TrimRight(p.cfg.APIBaseURL, "/") + "/drive/v3/changes")
	if err != nil {
		return nil, "", fmt.Errorf("google: parse changes URL: %w", err)
	}
	query := changesURL.Query()
	query.Set("pageToken", cursor)
	query.Set("fields", "newStartPageToken,nextPageToken,changes(fileId,removed,kind,file(id,name,mimeType,size,parents,webViewLink,modifiedTime,headRevisionId,owners,sharingUser,shared,trashed,appProperties))")
	changesURL.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, changesURL.String(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("google: build changes request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("google: changes: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusGone {
		return []drive.Change{{Kind: drive.ChangeCursorInv}}, "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("google: changes status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed googleChangesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, "", fmt.Errorf("google: decode changes response: %w", err)
	}
	changes := make([]drive.Change, 0, len(parsed.Changes))
	for _, item := range parsed.Changes {
		changeKind := mapGoogleChangeKind(item.Kind, item.Removed)
		change := drive.Change{ProviderFileID: item.FileID, Kind: changeKind}
		if item.File.ID != "" {
			change.Item = item.File.toFolderItem()
			if change.ProviderFileID == "" {
				change.ProviderFileID = change.Item.ProviderFileID
			}
		}
		changes = append(changes, change)
	}
	nextCursor := parsed.NextPageToken
	if nextCursor == "" {
		nextCursor = parsed.NewStartPageToken
	}
	return changes, nextCursor, nil
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

type googleFilesResponse struct {
	NextPageToken string       `json:"nextPageToken"`
	Files         []googleFile `json:"files"`
}

type googleChangesResponse struct {
	NewStartPageToken string         `json:"newStartPageToken"`
	NextPageToken     string         `json:"nextPageToken"`
	Changes           []googleChange `json:"changes"`
}

type googleChange struct {
	FileID  string     `json:"fileId"`
	Removed bool       `json:"removed"`
	Kind    string     `json:"kind"`
	File    googleFile `json:"file"`
}

type googleFile struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	MimeType       string            `json:"mimeType"`
	Size           json.RawMessage   `json:"size"`
	Parents        []string          `json:"parents"`
	WebViewLink    string            `json:"webViewLink"`
	ModifiedTime   string            `json:"modifiedTime"`
	HeadRevisionID string            `json:"headRevisionId"`
	Owners         []googleUser      `json:"owners"`
	SharingUser    *googleUser       `json:"sharingUser"`
	Shared         bool              `json:"shared"`
	Trashed        bool              `json:"trashed"`
	AppProperties  map[string]string `json:"appProperties"`
}

type googleUser struct {
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
}

func (file googleFile) toFolderItem() drive.FolderItem {
	ownerLabel := ""
	if len(file.Owners) > 0 {
		ownerLabel = file.Owners[0].EmailAddress
		if ownerLabel == "" {
			ownerLabel = file.Owners[0].DisplayName
		}
	}
	lastModifiedBy := ""
	if file.SharingUser != nil {
		lastModifiedBy = file.SharingUser.EmailAddress
		if lastModifiedBy == "" {
			lastModifiedBy = file.SharingUser.DisplayName
		}
	}
	modifiedAt, _ := time.Parse(time.RFC3339, file.ModifiedTime)
	return drive.FolderItem{
		ProviderFileID:     file.ID,
		ProviderRevisionID: file.HeadRevisionID,
		Title:              file.Name,
		MimeType:           file.MimeType,
		SizeBytes:          parseGoogleSize(file.Size),
		FolderPath:         folderPathFromProperties(file.AppProperties),
		IsFolder:           file.MimeType == "application/vnd.google-apps.folder",
		OwnerLabel:         ownerLabel,
		LastModifiedBy:     lastModifiedBy,
		ProviderURL:        file.WebViewLink,
		ModifiedAt:         modifiedAt,
		SharingState: map[string]any{
			"shared":  file.Shared,
			"trashed": file.Trashed,
			"parents": file.Parents,
		},
	}
}

func (p *Provider) accessToken(ctx context.Context, connectionID string) (string, error) {
	if p.pool == nil {
		return "", fmt.Errorf("google: provider called before ConfigureRuntime (pool is nil)")
	}
	if p.client == nil {
		return "", fmt.Errorf("google: provider called before ConfigureRuntime (client is nil)")
	}
	if p.cfg.APIBaseURL == "" {
		return "", fmt.Errorf("google: api_base_url not configured")
	}
	var credsRef string
	if err := p.pool.QueryRow(ctx, `SELECT credentials_ref FROM drive_connections WHERE id=$1`, connectionID).Scan(&credsRef); err != nil {
		return "", fmt.Errorf("google: lookup connection credentials: %w", err)
	}
	if !strings.HasPrefix(credsRef, "bearer:") {
		return "", fmt.Errorf("google: credentials_ref unsupported format")
	}
	return strings.TrimPrefix(credsRef, "bearer:"), nil
}

func parseGoogleSize(raw json.RawMessage) int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var asNumber int64
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		return asNumber
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		parsed, _ := strconv.ParseInt(asString, 10, 64)
		return parsed
	}
	return 0
}

func folderPathFromProperties(props map[string]string) []string {
	if props == nil {
		return nil
	}
	rawPath := strings.Trim(props["smackerel_folder_path"], "/")
	if rawPath == "" {
		return nil
	}
	parts := strings.Split(rawPath, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func mapGoogleChangeKind(kind string, removed bool) drive.ChangeKind {
	if removed {
		return drive.ChangeDelete
	}
	switch kind {
	case "move":
		return drive.ChangeMove
	case "trash":
		return drive.ChangeTrash
	case "permission_lost":
		return drive.ChangePermLost
	case "cursor_invalid":
		return drive.ChangeCursorInv
	default:
		return drive.ChangeUpsert
	}
}

func mediaType(contentType string) string {
	if contentType == "" {
		return "application/octet-stream"
	}
	parsed, _, err := mime.ParseMediaType(contentType)
	if err != nil || parsed == "" {
		return contentType
	}
	return parsed
}

// encodeBase64 returns the standard-encoding base64 of data. Used by
// PutFile to wrap the upload payload in a JSON-friendly envelope so the
// fixture surface stays inspectable from integration tests without parsing
// real Google Drive multipart-related bodies.
func encodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
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
