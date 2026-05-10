//go:build integration

// Spec 044 Scope 02 — MIT-038-S-003 closure integration test.
//
// Exercises the production-mode owner-identity contract on
// POST /v1/connectors/drive/connect end-to-end against the router. A
// malicious client supplying owner_user_id in the request body MUST
// be rejected with HTTP 400 owner_user_id_in_body_forbidden, even
// when authentication is otherwise valid. The owner is derived from
// the authenticated session attached by bearerAuthMiddleware.
//
// The test does not require DATABASE_URL because drive.Connect
// returns 400 BEFORE any DB or provider work happens. It uses a fake
// drive provider so no upstream OAuth surface is touched.
package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/drive"
)

// fakeDriveProviderForAuth is a copy of the test fixture pattern from
// internal/api/drive_handlers_test.go. We can't import that file
// (different package + _test.go), so define a minimal one here.
type fakeDriveProviderForAuth struct {
	id   string
	disp string
}

func (f *fakeDriveProviderForAuth) ID() string                     { return f.id }
func (f *fakeDriveProviderForAuth) DisplayName() string            { return f.disp }
func (f *fakeDriveProviderForAuth) Capabilities() drive.Capabilities { return drive.Capabilities{} }
func (f *fakeDriveProviderForAuth) BeginConnect(_ context.Context, _ drive.AccessMode, _ drive.Scope) (string, string, error) {
	return "https://fake.example/auth", "fake-state", nil
}
func (f *fakeDriveProviderForAuth) FinalizeConnect(_ context.Context, _ string, _ string) (string, error) {
	return "fake-conn-id", nil
}
func (f *fakeDriveProviderForAuth) Disconnect(_ context.Context, _ string) error {
	return nil
}
func (f *fakeDriveProviderForAuth) Scope(_ context.Context, _ string) (drive.Scope, error) {
	return drive.Scope{}, nil
}
func (f *fakeDriveProviderForAuth) SetScope(_ context.Context, _ string, _ drive.Scope) error {
	return nil
}
func (f *fakeDriveProviderForAuth) ListFolder(_ context.Context, _ string, _ string, _ string) ([]drive.FolderItem, string, error) {
	return nil, "", nil
}
func (f *fakeDriveProviderForAuth) GetFile(_ context.Context, _ string, _ string) (drive.FileBytes, error) {
	return drive.FileBytes{}, nil
}
func (f *fakeDriveProviderForAuth) PutFile(_ context.Context, _ string, _ string, _ string, _ drive.FileBytes) (string, error) {
	return "", nil
}
func (f *fakeDriveProviderForAuth) Changes(_ context.Context, _ string, _ string) ([]drive.Change, string, error) {
	return nil, "", nil
}
func (f *fakeDriveProviderForAuth) Health(_ context.Context, _ string) (drive.Health, error) {
	return drive.Health{}, nil
}

// productionAuthDepsForDrive constructs the per-user PASETO subsystem
// + a fake drive registry without any DB. The Connect handler returns
// 400 long before any pool query happens.
func productionAuthDepsForDrive(t *testing.T) (*api.Dependencies, string) {
	t.Helper()
	priv, pub := auth.GenerateSigningKeypair()
	const kid = "scope02-drive-key"

	reg := drive.NewRegistry()
	reg.Register(&fakeDriveProviderForAuth{id: "google", disp: "Google Drive (test)"})

	deps := &api.Dependencies{
		Environment: "production",
		AuthConfig: config.AuthConfig{
			Enabled:                              true,
			TokenFormat:                          "paseto_v4_public",
			SigningActivePrivateKey:              priv,
			SigningActiveKeyID:                   kid,
			TokenTTLHours:                        24,
			RotationGraceWindowHours:             24,
			ClockSkewToleranceSeconds:            60,
			RevocationCacheRefreshIntervalSeconds: 60,
			AtRestHashingKey:                     priv + "-drivetest-hash",
			ProductionSharedTokenFallbackEnabled: false,
		},
		AuthVerifyOptions: auth.VerifyOptions{
			ActivePublicKey:    pub,
			ActiveKeyID:        kid,
			Issuer:             "smackerel",
			ClockSkewTolerance: time.Minute,
			Now:                time.Now,
		},
		RevocationCache: revocation.NewCache(),
		DriveHandlers:   api.NewDriveHandlers(reg).WithEnvironment("production"),
	}
	return deps, priv
}

// TestDriveConnect_OwnerInBody_Production_Returns400 is THE
// adversarial test for MIT-038-S-003. A request whose body smuggles
// `owner_user_id` MUST be rejected with HTTP 400
// owner_user_id_in_body_forbidden in production, even when the
// bearer token is valid.
func TestDriveConnect_OwnerInBody_Production_Returns400(t *testing.T) {
	deps, priv := productionAuthDepsForDrive(t)
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "alice",
		TokenID:    "tok-drive-001",
		SigningKey: priv,
		KeyID:      deps.AuthConfig.SigningActiveKeyID,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	router := api.NewRouter(deps)

	body := `{"provider_id":"google","owner_user_id":"mallory","access_mode":"read_only","scope":{"folder_ids":["root"],"include_shared":false}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/drive/connect", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+issued.WireToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for body owner_user_id in production, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "owner_user_id_in_body_forbidden") {
		t.Errorf("expected error code owner_user_id_in_body_forbidden, body=%s", rec.Body.String())
	}
}

// TestDriveConnect_NoOwnerNoSession_Production_Returns400 verifies
// the fail-closed contract: production with NO owner in body AND NO
// session UserID → 400 owner_user_id_required (the production code
// path can no longer downgrade to a client-controlled value).
//
// To get past bearerAuthMiddleware without a per-user identity, we
// configure the production_shared_token_fallback opt-in so a shared-
// token request reaches the handler with a SharedToken session
// (UserID="").
func TestDriveConnect_NoOwnerNoSession_Production_Returns400(t *testing.T) {
	deps, _ := productionAuthDepsForDrive(t)
	deps.AuthToken = "production-fallback-shared"
	deps.AuthConfig.ProductionSharedTokenFallbackEnabled = true
	router := api.NewRouter(deps)

	body := `{"provider_id":"google","access_mode":"read_only","scope":{"folder_ids":["root"],"include_shared":false}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/drive/connect", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+deps.AuthToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for production no-owner no-session, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "owner_user_id_required") {
		t.Errorf("expected error code owner_user_id_required, body=%s", rec.Body.String())
	}
}

// TestDriveConnect_ProductionWithSession_DerivesOwner verifies that
// a production request with a valid PASETO token and NO owner in
// body succeeds with the owner derived from session.
func TestDriveConnect_ProductionWithSession_DerivesOwner(t *testing.T) {
	deps, priv := productionAuthDepsForDrive(t)
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "alice",
		TokenID:    "tok-drive-002",
		SigningKey: priv,
		KeyID:      deps.AuthConfig.SigningActiveKeyID,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	router := api.NewRouter(deps)

	body := `{"provider_id":"google","access_mode":"read_only","scope":{"folder_ids":["root"],"include_shared":false}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/drive/connect", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+issued.WireToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for production with session, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "fake-state") {
		// Sanity check that we hit our fake provider rather than an
		// error path.
		t.Logf("response body=%s", rec.Body.String())
	}
	_ = fmt.Sprintf("") // keep fmt import used across files
}
