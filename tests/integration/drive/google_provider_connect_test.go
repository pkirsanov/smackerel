//go:build integration

// Spec 038 Scope 1 — SCN-038-002 integration row.
//
// TestGoogleDriveFixtureConnectStoresHealthyScopedConnection drives the
// real GoogleDriveProvider through the OAuth begin/finalize redirect
// against the owned fixture server (tests/integration/drive/fixtures).
// It proves:
//
//  1. BeginConnect persists a drive_oauth_states row keyed by a
//     cryptographically-random state token bound to the owning user
//     and returns a provider authorization URL pointing at the
//     fixture's oauth_base_url.
//  2. FinalizeConnect exchanges the fixture-issued code for tokens
//     against /oauth2/token, fetches the bound account email from
//     /drive/v3/about, inserts a healthy drive_connections row with
//     access_mode=read_save, scope persisted, and expires_at populated,
//     and deletes the consumed drive_oauth_states row.
//  3. Health returns HealthHealthy for the new connection (live call
//     against /drive/v3/about with the stored bearer token).
//  4. The empty-drive listing seeds zero drive_files rows for the
//     connection (no auto-scan at connect time).
package drive

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/smackerel/smackerel/internal/config"
	smdrive "github.com/smackerel/smackerel/internal/drive"
	"github.com/smackerel/smackerel/internal/drive/google"
	"github.com/smackerel/smackerel/tests/integration/drive/fixtures"
)

func TestGoogleDriveFixtureConnectStoresHealthyScopedConnection(t *testing.T) {
	pool := driveTestPool(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fx := fixtures.NewServer()
	defer fx.Close()

	p := google.New(google.DefaultCapabilities()).ConfigureRuntime(
		pool,
		http.DefaultClient,
		config.DriveGoogleProviderConfig{
			OAuthClientID:     "fixture-client",
			OAuthClientSecret: "fixture-secret",
			OAuthRedirectURL:  "http://127.0.0.1:0/api/v1/connectors/drive/google/oauth/callback",
			OAuthBaseURL:      fx.URL,
			APIBaseURL:        fx.URL,
			ScopeDefaults:     []string{"https://www.googleapis.com/auth/drive.file"},
		},
	)

	// Owner is a synthetic UUID — drive_connections.owner_user_id is
	// UUID NOT NULL but has no FK, so any well-formed UUID works.
	owner := uuid.NewString()
	ownedCtx := smdrive.WithOwnerUserID(ctx, owner)

	scope := smdrive.Scope{FolderIDs: []string{"folder-acme"}, IncludeShared: false}
	authURL, state, err := p.BeginConnect(ownedCtx, smdrive.AccessReadSave, scope)
	if err != nil {
		t.Fatalf("BeginConnect: %v", err)
	}
	if !strings.Contains(authURL, fx.URL) {
		t.Fatalf("authURL %q does not contain fixture URL %q", authURL, fx.URL)
	}
	if state == "" {
		t.Fatalf("empty state token returned")
	}
	if !strings.Contains(authURL, "state="+state) {
		t.Fatalf("authURL %q missing state token %q", authURL, state)
	}

	// drive_oauth_states row must exist with the right owner + access_mode.
	var (
		gotOwner   string
		gotMode    string
		gotProv    string
		gotExpires time.Time
	)
	if err := pool.QueryRow(ctx,
		`SELECT owner_user_id::text, access_mode, provider_id, expires_at
		   FROM drive_oauth_states WHERE state_token=$1`, state,
	).Scan(&gotOwner, &gotMode, &gotProv, &gotExpires); err != nil {
		t.Fatalf("oauth state row not persisted: %v", err)
	}
	if gotOwner != owner {
		t.Errorf("oauth state owner = %s, want %s", gotOwner, owner)
	}
	if gotMode != "read_save" {
		t.Errorf("oauth state access_mode = %s, want read_save", gotMode)
	}
	if gotProv != "google" {
		t.Errorf("oauth state provider_id = %s, want google", gotProv)
	}
	if !gotExpires.After(time.Now()) {
		t.Errorf("oauth state expires_at %s is not in the future", gotExpires)
	}

	// IssueAuthCode mints a code bound to our state without simulating
	// a browser hop through /oauth2/auth — the redirect leg is exercised
	// separately at the e2e-ui layer.
	code := fx.IssueAuthCode(state)
	if code == "" {
		t.Fatalf("IssueAuthCode returned empty code")
	}

	connID, err := p.FinalizeConnect(ctx, state, code)
	if err != nil {
		t.Fatalf("FinalizeConnect: %v", err)
	}
	if _, err := uuid.Parse(connID); err != nil {
		t.Fatalf("connection id %q is not a UUID: %v", connID, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM drive_connections WHERE id=$1`, connID)
	})

	// drive_oauth_states row must be consumed.
	var lingering int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM drive_oauth_states WHERE state_token=$1`, state,
	).Scan(&lingering); err != nil {
		t.Fatalf("count lingering oauth state: %v", err)
	}
	if lingering != 0 {
		t.Errorf("oauth state row not consumed (count=%d)", lingering)
	}

	// drive_connections row must reflect the requested access_mode +
	// scope, be healthy, and carry an expires_at populated from the
	// fixture token's expires_in.
	var (
		status        string
		accessMode    string
		accountLabel  string
		ownerStored   string
		credsRef      string
		scopeJSON     []byte
		expiresAtRow  *time.Time
		providerStore string
	)
	if err := pool.QueryRow(ctx,
		`SELECT status, access_mode, account_label, owner_user_id::text,
		        credentials_ref, scope, expires_at, provider_id
		   FROM drive_connections WHERE id=$1`, connID,
	).Scan(&status, &accessMode, &accountLabel, &ownerStored,
		&credsRef, &scopeJSON, &expiresAtRow, &providerStore); err != nil {
		t.Fatalf("drive_connections row not persisted: %v", err)
	}
	if status != "healthy" {
		t.Errorf("status = %s, want healthy", status)
	}
	if accessMode != "read_save" {
		t.Errorf("access_mode = %s, want read_save", accessMode)
	}
	if providerStore != "google" {
		t.Errorf("provider_id = %s, want google", providerStore)
	}
	if ownerStored != owner {
		t.Errorf("owner_user_id = %s, want %s", ownerStored, owner)
	}
	if accountLabel == "" {
		t.Errorf("account_label is empty")
	}
	if !strings.HasPrefix(credsRef, "bearer:") {
		t.Errorf("credentials_ref = %q, want bearer:* prefix", credsRef)
	}
	if !strings.Contains(string(scopeJSON), "folder-acme") {
		t.Errorf("scope JSON %s missing folder-acme", scopeJSON)
	}
	if expiresAtRow == nil {
		t.Errorf("expires_at not populated")
	} else if !expiresAtRow.After(time.Now()) {
		t.Errorf("expires_at %s is not in the future", *expiresAtRow)
	}

	// Health: live call to /drive/v3/about must return Healthy.
	h, err := p.Health(ctx, connID)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if h.Status != smdrive.HealthHealthy {
		t.Errorf("Health.Status = %s, want healthy (reason=%s)", h.Status, h.Reason)
	}

	// Empty drive: no drive_files rows are auto-created at connect time.
	var fileCount int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM drive_files WHERE connection_id=$1`, connID,
	).Scan(&fileCount); err != nil {
		t.Fatalf("count drive_files: %v", err)
	}
	if fileCount != 0 {
		t.Errorf("drive_files count for new connection = %d, want 0 (connect must not auto-scan)", fileCount)
	}
}
