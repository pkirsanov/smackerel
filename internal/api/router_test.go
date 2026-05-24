package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/drive"
)

// --- Mocks for router-level tests ---

// mockWebUI implements WebUI with a simple 200 OK for every page.
type mockWebUI struct{}

func (m *mockWebUI) SearchPage(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
func (m *mockWebUI) SearchResults(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) ArtifactDetail(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) EvidenceBundleBuilderPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) DigestPage(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
func (m *mockWebUI) TopicsPage(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
func (m *mockWebUI) SettingsPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) StatusPage(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
func (m *mockWebUI) RecommendationsPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) RecommendationsResults(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) RecommendationPreferencesPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) RecommendationFeedback(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) RecommendationDetail(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) RecommendationWatchesPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) RecommendationWatchEditorPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) RecommendationWatchDetailPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) RecommendationWatchPauseAction(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) RecommendationWatchResumeAction(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) RecommendationWatchSilenceAction(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) RecommendationWatchDeleteAction(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) TripDossierPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) NotificationDashboard(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) NotificationSourcesPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) NotificationNtfySourcePage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) NotificationNtfyDeadLettersPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) NotificationNtfyDeadLetterDetailPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) NotificationEventsPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) NotificationIncidentsPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) NotificationIncidentDetailPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) NotificationApprovalsPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) NotificationApprovalDetailPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) NotificationSuppressionsPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) NotificationSummaryPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) NotificationOutputsPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) SyncConnectorHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) BookmarkUploadHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) KnowledgeDashboard(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) ConceptsList(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) ConceptDetail(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) EntitiesList(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) EntityDetail(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) LintReport(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) LintFindingDetail(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// mockOAuth implements OAuthFlow with 200 OK stubs.
type mockOAuth struct{}

func (m *mockOAuth) StartHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockOAuth) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockOAuth) StatusHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

type routerDriveProvider struct {
	id      string
	disp    string
	caps    drive.Capabilities
	authURL string
	state   string
}

func (provider *routerDriveProvider) ID() string          { return provider.id }
func (provider *routerDriveProvider) DisplayName() string { return provider.disp }
func (provider *routerDriveProvider) Capabilities() drive.Capabilities {
	return provider.caps
}
func (provider *routerDriveProvider) BeginConnect(_ context.Context, _ drive.AccessMode, _ drive.Scope) (string, string, error) {
	return provider.authURL, provider.state, nil
}
func (provider *routerDriveProvider) FinalizeConnect(_ context.Context, _ string, _ string) (string, error) {
	return "", drive.ErrNotImplemented
}
func (provider *routerDriveProvider) Disconnect(_ context.Context, _ string) error {
	return drive.ErrNotImplemented
}
func (provider *routerDriveProvider) Scope(_ context.Context, _ string) (drive.Scope, error) {
	return drive.Scope{}, nil
}
func (provider *routerDriveProvider) SetScope(_ context.Context, _ string, _ drive.Scope) error {
	return drive.ErrNotImplemented
}
func (provider *routerDriveProvider) ListFolder(_ context.Context, _ string, _ string, _ string) ([]drive.FolderItem, string, error) {
	return nil, "", drive.ErrNotImplemented
}
func (provider *routerDriveProvider) GetFile(_ context.Context, _ string, _ string) (drive.FileBytes, error) {
	return drive.FileBytes{}, drive.ErrNotImplemented
}
func (provider *routerDriveProvider) PutFile(_ context.Context, _ string, _ string, _ string, _ drive.FileBytes) (string, error) {
	return "", drive.ErrNotImplemented
}
func (provider *routerDriveProvider) Changes(_ context.Context, _ string, _ string) ([]drive.Change, string, error) {
	return nil, "", drive.ErrNotImplemented
}
func (provider *routerDriveProvider) Health(_ context.Context, _ string) (drive.Health, error) {
	return drive.Health{Status: drive.HealthHealthy}, nil
}

func TestRouterMountsDriveConnectorRoutes(t *testing.T) {
	reg := drive.NewRegistry()
	reg.Register(&routerDriveProvider{
		id:      "google",
		disp:    "Google Drive",
		caps:    drive.Capabilities{MaxFileSizeBytes: 104857600},
		authURL: "https://accounts.example/oauth2/auth?state=state-123",
		state:   "state-123",
	})
	router := NewRouter(&Dependencies{DriveHandlers: NewDriveHandlers(reg)})

	getReq := httptest.NewRequest(http.MethodGet, "/v1/connectors/drive", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /v1/connectors/drive status = %d, want 200; body=%s", getRec.Code, getRec.Body.String())
	}

	postBody := `{"provider_id":"google","owner_user_id":"00000000-0000-0000-0000-000000000001","access_mode":"read_save","scope":{"folder_ids":["folder-acme"],"include_shared":false}}`
	postReq := httptest.NewRequest(http.MethodPost, "/v1/connectors/drive/connect", strings.NewReader(postBody))
	postReq.Header.Set("Content-Type", "application/json")
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("POST /v1/connectors/drive/connect status = %d, want 200; body=%s", postRec.Code, postRec.Body.String())
	}
}

// --- SCN-020-009: Web UI requires auth when auth_token is configured ---

func TestWebUI_RequiresAuth_WhenTokenConfigured(t *testing.T) {
	deps := &Dependencies{
		DB:         &mockDB{healthy: true},
		NATS:       &mockNATS{healthy: true},
		StartTime:  time.Now(),
		AuthToken:  "test-web-secret",
		WebHandler: &mockWebUI{},
	}

	router := NewRouter(deps)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/"},
		{http.MethodPost, "/search"},
		{http.MethodGet, "/digest"},
		{http.MethodGet, "/topics"},
		{http.MethodGet, "/settings"},
		{http.MethodGet, "/status"},
	}

	for _, rt := range routes {
		t.Run(rt.method+"_"+rt.path+"_no_auth", func(t *testing.T) {
			req := httptest.NewRequest(rt.method, rt.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 for %s %s without auth, got %d", rt.method, rt.path, rec.Code)
			}
		})
	}
}

func TestWebUI_AcceptsBearerToken(t *testing.T) {
	deps := &Dependencies{
		DB:         &mockDB{healthy: true},
		NATS:       &mockNATS{healthy: true},
		StartTime:  time.Now(),
		AuthToken:  "test-web-secret",
		WebHandler: &mockWebUI{},
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer test-web-secret")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with valid Bearer token, got %d", rec.Code)
	}
}

func TestWebUI_AcceptsCookie(t *testing.T) {
	deps := &Dependencies{
		DB:         &mockDB{healthy: true},
		NATS:       &mockNATS{healthy: true},
		StartTime:  time.Now(),
		AuthToken:  "test-web-secret",
		WebHandler: &mockWebUI{},
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: "test-web-secret"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 with valid auth_token cookie, got %d", rec.Code)
	}
}

func TestWebUI_RejectsWrongToken(t *testing.T) {
	deps := &Dependencies{
		DB:         &mockDB{healthy: true},
		NATS:       &mockNATS{healthy: true},
		StartTime:  time.Now(),
		AuthToken:  "test-web-secret",
		WebHandler: &mockWebUI{},
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong token, got %d", rec.Code)
	}
}

// --- SCN-020-010: Web UI allows all requests when auth_token is empty ---

func TestWebUI_AllowsAll_WhenTokenEmpty(t *testing.T) {
	deps := &Dependencies{
		DB:         &mockDB{healthy: true},
		NATS:       &mockNATS{healthy: true},
		StartTime:  time.Now(),
		AuthToken:  "", // dev mode
		WebHandler: &mockWebUI{},
	}

	router := NewRouter(deps)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/"},
		{http.MethodPost, "/search"},
		{http.MethodGet, "/digest"},
		{http.MethodGet, "/topics"},
		{http.MethodGet, "/settings"},
		{http.MethodGet, "/status"},
	}

	for _, rt := range routes {
		t.Run(rt.method+"_"+rt.path+"_dev_mode", func(t *testing.T) {
			req := httptest.NewRequest(rt.method, rt.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("expected 200 in dev mode for %s %s, got %d", rt.method, rt.path, rec.Code)
			}
		})
	}
}

// --- SCN-020-011: OAuth start endpoint is rate-limited ---

func TestOAuthStart_RateLimited(t *testing.T) {
	deps := &Dependencies{
		DB:           &mockDB{healthy: true},
		NATS:         &mockNATS{healthy: true},
		StartTime:    time.Now(),
		OAuthHandler: &mockOAuth{},
	}

	router := NewRouter(deps)

	// Send 11 requests — first 10 should succeed, 11th should be 429.
	var lastCode int
	got429 := false
	for i := 0; i < 15; i++ {
		req := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		lastCode = rec.Code

		if rec.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}

	if !got429 {
		t.Errorf("expected 429 after exceeding rate limit, last status was %d", lastCode)
	}
}

// --- SCN-020-012: OAuth start allows traffic within rate limit ---

func TestOAuthStart_AllowsWithinLimit(t *testing.T) {
	deps := &Dependencies{
		DB:           &mockDB{healthy: true},
		NATS:         &mockNATS{healthy: true},
		StartTime:    time.Now(),
		OAuthHandler: &mockOAuth{},
	}

	router := NewRouter(deps)

	// Send 5 requests — all should succeed.
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
		req.RemoteAddr = "10.0.0.1:54321" // different IP from rate-limit test
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, rec.Code)
		}
	}
}

// --- OAuth callback IS rate-limited (SEC-SWEEP-001) ---

func TestOAuthCallback_RateLimited(t *testing.T) {
	deps := &Dependencies{
		DB:           &mockDB{healthy: true},
		NATS:         &mockNATS{healthy: true},
		StartTime:    time.Now(),
		OAuthHandler: &mockOAuth{},
	}

	router := NewRouter(deps)

	// Send 15 requests to callback — should hit 429 after 10.
	got429 := false
	for i := 0; i < 15; i++ {
		req := httptest.NewRequest(http.MethodGet, "/auth/google/callback", nil)
		req.RemoteAddr = "192.168.1.200:12345"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}

	if !got429 {
		t.Error("expected 429 after exceeding rate limit on OAuth callback")
	}
}

// --- Security headers middleware tests ---

func TestSecurityHeaders_Present(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	expectedHeaders := map[string]string{
		"Content-Security-Policy": "default-src 'self'",
		"X-Frame-Options":         "DENY",
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Permissions-Policy":      "camera=(), microphone=(), geolocation=()",
		"Cache-Control":           "no-store",
	}

	for header, expectedSubstr := range expectedHeaders {
		got := rec.Header().Get(header)
		if got == "" {
			t.Errorf("missing security header %s", header)
			continue
		}
		if header == "Content-Security-Policy" {
			// CSP is long — just check it starts with default-src
			if !containsSubstring(got, expectedSubstr) {
				t.Errorf("header %s = %q, expected to contain %q", header, got, expectedSubstr)
			}
		} else {
			if got != expectedSubstr {
				t.Errorf("header %s = %q, want %q", header, got, expectedSubstr)
			}
		}
	}
}

func TestSecurityHeaders_OnAllRoutes(t *testing.T) {
	deps := &Dependencies{
		DB:         &mockDB{healthy: true},
		NATS:       &mockNATS{healthy: true},
		StartTime:  time.Now(),
		WebHandler: &mockWebUI{},
	}

	router := NewRouter(deps)

	// Security headers should appear on both API and Web UI routes.
	routes := []string{"/api/health", "/"}
	for _, path := range routes {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Header().Get("X-Frame-Options") != "DENY" {
				t.Errorf("missing X-Frame-Options on %s", path)
			}
			if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Errorf("missing X-Content-Type-Options on %s", path)
			}
		})
	}
}

// --- Bearer auth edge cases ---

func TestBearerAuth_MalformedHeader_Rejected(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		AuthToken: "test-secret-token-1234",
	}

	router := NewRouter(deps)

	cases := []struct {
		name string
		auth string
	}{
		{"token_scheme", "Token test-secret-token-1234"},
		{"basic_scheme", "Basic dGVzdDp0ZXN0"},
		{"no_scheme", "test-secret-token-1234"},
		{"empty_bearer", "Bearer "},
		{"bearer_no_space", "Bearertest-secret-token-1234"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/digest", nil)
			req.Header.Set("Authorization", tc.auth)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 for Authorization: %q, got %d", tc.auth, rec.Code)
			}
		})
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && len(sub) > 0 && stringContains(s, sub))
}

// --- IMP-020-001: Bearer auth direct constant-time compare (no double extraction) ---

func TestBearerAuth_SubtleDifferences_Rejected(t *testing.T) {
	// Ensures the direct subtle.ConstantTimeCompare path correctly rejects
	// tokens that differ by trailing whitespace, case, or one character.
	// If bearerAuthMiddleware used a double-extraction path, subtle differences
	// in extraction could lead to inconsistent behavior between checks.
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		AuthToken: "correct-token-value",
	}

	router := NewRouter(deps)

	cases := []struct {
		name  string
		token string
	}{
		{"trailing_space", "correct-token-value "},
		{"leading_space", " correct-token-value"},
		{"case_flip", "Correct-token-value"},
		{"one_char_off", "correct-token-valuf"},
		{"prefix_only", "correct-token"},
		{"extended", "correct-token-value-extra"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/digest", nil)
			req.Header.Set("Authorization", "Bearer "+tc.token)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 for token %q, got %d", tc.token, rec.Code)
			}
		})
	}

	// Correct token must still be accepted (auth passes — handler may return non-200 without full mock)
	t.Run("correct_token_accepted", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/digest", nil)
		req.Header.Set("Authorization", "Bearer correct-token-value")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code == http.StatusUnauthorized {
			t.Errorf("expected auth to pass for correct token, got 401")
		}
	})
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- Security headers on 404 (unmatched routes) ---

func TestSecurityHeaders_On404(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent/path", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Even 404 responses must carry security headers (middleware runs before routing)
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("missing X-Frame-Options on 404 response")
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options on 404 response")
	}
	if rec.Header().Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
		t.Error("missing Referrer-Policy on 404 response")
	}
	if rec.Header().Get("Permissions-Policy") != "camera=(), microphone=(), geolocation=()" {
		t.Error("missing Permissions-Policy on 404 response")
	}
}

// --- CSP directive completeness ---

func TestSecurityHeaders_CSP_Directives(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")

	// Verify each required CSP directive is present
	requiredDirectives := []string{
		"default-src 'self'",
		"style-src",
		"script-src",
		"img-src",
		"connect-src 'self'",
	}
	for _, directive := range requiredDirectives {
		if !stringContains(csp, directive) {
			t.Errorf("CSP missing required directive %q, full CSP: %s", directive, csp)
		}
	}

	// Verify no unsafe-eval in script-src
	if stringContains(csp, "'unsafe-eval'") {
		t.Error("CSP should not include 'unsafe-eval' in script-src")
	}
}

// --- API bearer auth dev mode (empty AuthToken allows all) ---

func TestAPIAuth_DevMode_AllowsAll(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		AuthToken: "", // dev mode
	}

	router := NewRouter(deps)

	// Authenticated API routes should be accessible without auth in dev mode
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/health"},
		{http.MethodGet, "/api/digest"},
		{http.MethodGet, "/api/recent"},
	}

	for _, rt := range routes {
		t.Run(rt.method+"_"+rt.path, func(t *testing.T) {
			req := httptest.NewRequest(rt.method, rt.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code == http.StatusUnauthorized {
				t.Errorf("dev mode should allow %s %s without auth, got 401", rt.method, rt.path)
			}
		})
	}
}

// --- Web auth: wrong cookie value ---

func TestWebUI_RejectsWrongCookie(t *testing.T) {
	deps := &Dependencies{
		DB:         &mockDB{healthy: true},
		NATS:       &mockNATS{healthy: true},
		StartTime:  time.Now(),
		AuthToken:  "correct-secret-token-12345",
		WebHandler: &mockWebUI{},
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: "wrong-cookie-value"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong cookie, got %d", rec.Code)
	}
}

// --- Web auth: both wrong bearer AND wrong cookie ---

func TestWebUI_RejectsBothWrongBearerAndWrongCookie(t *testing.T) {
	deps := &Dependencies{
		DB:         &mockDB{healthy: true},
		NATS:       &mockNATS{healthy: true},
		StartTime:  time.Now(),
		AuthToken:  "correct-secret-token-12345",
		WebHandler: &mockWebUI{},
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: "also-wrong"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with both wrong bearer and wrong cookie, got %d", rec.Code)
	}
}

// --- Bearer auth: case-insensitive "Bearer" scheme ---

func TestBearerAuth_CaseInsensitiveScheme(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		AuthToken: "test-secret-token-1234",
	}

	router := NewRouter(deps)

	// "bearer" (lowercase) should also be accepted
	cases := []string{
		"Bearer test-secret-token-1234",
		"bearer test-secret-token-1234",
		"BEARER test-secret-token-1234",
		"BeArEr test-secret-token-1234",
	}

	for _, auth := range cases {
		t.Run(auth[:6], func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
			req.Header.Set("Authorization", auth)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code == http.StatusUnauthorized {
				t.Errorf("expected auth to accept %q, got 401", auth[:6])
			}
		})
	}
}

// --- Rate limit IP isolation ---

func TestOAuthStart_RateLimitPerIP(t *testing.T) {
	deps := &Dependencies{
		DB:           &mockDB{healthy: true},
		NATS:         &mockNATS{healthy: true},
		StartTime:    time.Now(),
		OAuthHandler: &mockOAuth{},
	}

	router := NewRouter(deps)

	// Exhaust rate limit for IP A
	for i := 0; i < 11; i++ {
		req := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
		req.RemoteAddr = "10.10.10.1:12345"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
	}

	// IP B should still have its own limit — first request should succeed
	req := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
	req.RemoteAddr = "10.10.10.2:12345"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusTooManyRequests {
		t.Error("rate limit for IP A should not affect IP B")
	}
}

// --- Security headers on authenticated error responses ---

func TestSecurityHeaders_OnUnauthorized(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		AuthToken: "test-secret-token-1234",
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/digest", nil)
	// No auth header — should get 401
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	// Security headers must still be present on error responses
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("missing X-Frame-Options on 401 response")
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options on 401 response")
	}
}

// --- API bearer auth: empty Authorization header value ---

func TestBearerAuth_EmptyAuthorizationHeader(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		AuthToken: "test-secret-token-1234",
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/digest", nil)
	req.Header.Set("Authorization", "")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty Authorization header, got %d", rec.Code)
	}
}

// --- OAuth routes not behind bearer auth ---

func TestOAuthRoutes_NotBehindBearerAuth(t *testing.T) {
	deps := &Dependencies{
		DB:           &mockDB{healthy: true},
		NATS:         &mockNATS{healthy: true},
		StartTime:    time.Now(),
		AuthToken:    "test-secret-token-1234",
		OAuthHandler: &mockOAuth{},
	}

	router := NewRouter(deps)

	// OAuth start and callback should be accessible without Bearer token
	routes := []string{
		"/auth/google/start",
		"/auth/google/callback",
	}

	for _, path := range routes {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.RemoteAddr = "172.16.0.1:12345" // unique IP to avoid rate limit
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			// Should NOT be 401 — OAuth routes are outside bearer auth group
			if rec.Code == http.StatusUnauthorized {
				t.Errorf("OAuth route %s should not require bearer auth, got 401", path)
			}
		})
	}
}

// --- Auth status endpoint requires bearer auth ---

func TestOAuthStatus_RequiresBearerAuth(t *testing.T) {
	deps := &Dependencies{
		DB:           &mockDB{healthy: true},
		NATS:         &mockNATS{healthy: true},
		StartTime:    time.Now(),
		AuthToken:    "test-secret-token-1234",
		OAuthHandler: &mockOAuth{},
	}

	router := NewRouter(deps)

	// /api/auth/status should require bearer auth
	req := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for /api/auth/status without auth, got %d", rec.Code)
	}

	// With valid auth, should succeed
	req2 := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	req2.Header.Set("Authorization", "Bearer test-secret-token-1234")
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)

	if rec2.Code == http.StatusUnauthorized {
		t.Error("/api/auth/status should be accessible with valid bearer token")
	}
}

// --- Health endpoint does not require auth ---

func TestHealthEndpoint_NoAuthRequired(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
		AuthToken: "test-secret-token-1234",
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Error("/api/health should not require authentication")
	}
}

// --- IMP-020-CSP-001: CSP script-src includes hash for inline theme toggle script ---

func TestSecurityHeaders_CSP_ScriptHashPresent(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")

	// CSP must include a sha256 hash for the inline theme toggle script
	// (no 'unsafe-inline' in script-src, so hash is required for inline scripts to execute).
	if !stringContains(csp, "'sha256-") {
		t.Errorf("CSP script-src should contain a sha256 hash for inline scripts, got: %s", csp)
	}

	// Must NOT contain 'unsafe-inline' in script-src — hash-based approach is preferred
	// Extract the script-src directive
	scriptSrcIdx := -1
	for i, c := range csp {
		if c == 's' && stringContains(csp[i:], "script-src") {
			scriptSrcIdx = i
			break
		}
	}
	if scriptSrcIdx >= 0 {
		// Find the next directive boundary (;) or end of string
		scriptSrc := csp[scriptSrcIdx:]
		semiIdx := len(scriptSrc)
		for i, c := range scriptSrc {
			if c == ';' {
				semiIdx = i
				break
			}
		}
		scriptSrcDirective := scriptSrc[:semiIdx]
		if stringContains(scriptSrcDirective, "'unsafe-inline'") {
			t.Errorf("CSP script-src should NOT contain 'unsafe-inline' (use hash instead), got: %s", scriptSrcDirective)
		}
	}
}

// --- IMP-020-CSP-002: CSP hash must NOT contain 'unsafe-eval' ---

func TestSecurityHeaders_CSP_NoUnsafeEvalOrInline(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")

	if stringContains(csp, "'unsafe-eval'") {
		t.Errorf("CSP must not contain 'unsafe-eval', got: %s", csp)
	}
}

// --- SEC-R68-001: CSP script-src must pin CDN to specific package version path ---

func TestSecurityHeaders_CSP_PinnedCDNPath(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")

	// CSP must NOT allow the entire unpkg.com domain — only the pinned HTMX version path
	if stringContains(csp, "https://unpkg.com ") || stringContains(csp, "https://unpkg.com;") || strings.HasSuffix(csp, "https://unpkg.com") {
		t.Errorf("CSP script-src must pin CDN to specific package path, not entire domain; got: %s", csp)
	}

	// Must contain the pinned version path
	if !stringContains(csp, "https://unpkg.com/htmx.org@") {
		t.Errorf("CSP script-src must contain pinned HTMX version path (https://unpkg.com/htmx.org@...); got: %s", csp)
	}
}

// --- BUG-020-005 / F-SEC-R30-001 — OAuth rate-limit header-spoof bypass regression ---
//
// Background: prior to the fix, chi.middleware.RealIP was applied
// unconditionally at router root. httprate.LimitByIP keys on
// r.RemoteAddr, which middleware.RealIP rewrites from any client-supplied
// X-Forwarded-For / X-Real-IP / True-Client-IP header. Rotating one of
// those headers per request bypassed the per-IP rate limit completely.
//
// Fix: r.Use(deps.trustedProxyRealIPMiddleware()) — only honours
// forwarded headers when the TCP peer is in Dependencies.TrustedProxies
// CIDR allowlist. Empty allowlist (SST default) → headers ignored.
//
// Adversarial fidelity: temporarily replacing the gated middleware with
// the old r.Use(middleware.RealIP) makes TestSecR30_OAuthRateLimit_*
// (cases 1 and 3) FAIL because the spoofed XFF re-keys the bucket per
// request and 429 never fires.

func TestSecR30_OAuthRateLimit_NotBypassableViaXForwardedFor(t *testing.T) {
	// SCN-SEC-FIX-005-001 — TrustedProxies empty (the SST default).
	// Rotating X-Forwarded-For per request MUST NOT extend the per-IP
	// budget. The middleware ignores all forwarded headers because the
	// connecting peer is not in an empty allowlist.
	deps := &Dependencies{
		DB:             &mockDB{healthy: true},
		NATS:           &mockNATS{healthy: true},
		StartTime:      time.Now(),
		OAuthHandler:   &mockOAuth{},
		TrustedProxies: nil, // empty → forwarded headers ignored
	}

	router := NewRouter(deps)

	got429 := false
	var statuses []int
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
		req.RemoteAddr = "192.168.1.99:44444" // single TCP peer
		// Rotate XFF per request; without the fix this would bypass
		// the rate limit because chi.middleware.RealIP would rewrite
		// r.RemoteAddr to a new value each iteration.
		req.Header.Set("X-Forwarded-For", "10.0.0."+itoa(i+1))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		statuses = append(statuses, rec.Code)
		if rec.Code == http.StatusTooManyRequests {
			got429 = true
			if i >= 11 {
				// Sanity: we should hit 429 inside the first ~11
				// because the budget is 10/min/IP.
				t.Errorf("rate limit triggered at i=%d, expected within first ~11", i)
			}
			break
		}
	}

	if !got429 {
		// Print first 12 statuses for diagnostic clarity if the fix
		// regresses (every one of the 50 will be 200 in the bug state).
		head := statuses
		if len(head) > 12 {
			head = head[:12]
		}
		t.Fatalf("BUG-020-005 regression: rotating X-Forwarded-For bypassed the OAuth rate limit; first 12 statuses = %v", head)
	}
}

func TestSecR30_OAuthRateLimit_HonorsXForwardedForFromTrustedPeer(t *testing.T) {
	// SCN-SEC-FIX-005-002 — peer in 127.0.0.0/8 → forwarded header
	// honoured. Two distinct real-client XFF values get independent
	// per-IP rate-limit buckets (proves the legitimate proxy-trust
	// path still works after the fix).
	deps := &Dependencies{
		DB:             &mockDB{healthy: true},
		NATS:           &mockNATS{healthy: true},
		StartTime:      time.Now(),
		OAuthHandler:   &mockOAuth{},
		TrustedProxies: []string{"127.0.0.0/8"},
	}

	router := NewRouter(deps)

	// Client A — 203.0.113.42 — should hit 429 within its 11-request burst.
	gotA429 := false
	for i := 0; i < 15; i++ {
		req := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
		req.RemoteAddr = "127.0.0.1:55555" // upstream proxy peer (trusted)
		req.Header.Set("X-Forwarded-For", "203.0.113.42")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			gotA429 = true
			break
		}
	}
	if !gotA429 {
		t.Fatalf("client A (203.0.113.42) was not rate-limited despite trusted-proxy forwarding the same XFF for 15 requests")
	}

	// Client B — 203.0.113.43 — first request must still succeed; the
	// rate limit is per-real-client, not per-proxy-peer.
	reqB := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
	reqB.RemoteAddr = "127.0.0.1:55555"
	reqB.Header.Set("X-Forwarded-For", "203.0.113.43")
	recB := httptest.NewRecorder()
	router.ServeHTTP(recB, reqB)
	if recB.Code == http.StatusTooManyRequests {
		t.Fatalf("client B (203.0.113.43) was rate-limited on its first request — per-IP bucket leaked across XFF values")
	}
}

func TestSecR30_OAuthRateLimit_RejectsForwardedFromUntrustedPeer(t *testing.T) {
	// SCN-SEC-FIX-005-003 — trusted_proxies is non-empty but the
	// connecting peer is NOT in any trusted CIDR. Forwarded headers
	// MUST still be ignored.
	deps := &Dependencies{
		DB:             &mockDB{healthy: true},
		NATS:           &mockNATS{healthy: true},
		StartTime:      time.Now(),
		OAuthHandler:   &mockOAuth{},
		TrustedProxies: []string{"10.42.0.0/16"},
	}

	router := NewRouter(deps)

	got429 := false
	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/auth/google/start", nil)
		req.RemoteAddr = "192.168.1.99:44444" // NOT in 10.42.0.0/16
		req.Header.Set("X-Forwarded-For", "10.0.0."+itoa(i+1))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}

	if !got429 {
		t.Fatalf("BUG-020-005 regression: untrusted peer was allowed to spoof X-Forwarded-For")
	}
}

func TestSecR30_TrustedProxyMiddleware_PreservesRawRemoteAddrWhenAllowlistEmpty(t *testing.T) {
	// SCN-SEC-FIX-005-001 — direct middleware unit. With an empty
	// allowlist, the middleware MUST be an identity pass-through; all
	// three forwarded headers MUST be ignored.
	deps := &Dependencies{TrustedProxies: nil}
	mw := deps.trustedProxyRealIPMiddleware()

	var observed string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed = r.RemoteAddr
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.RemoteAddr = "192.168.1.99:44444"
	req.Header.Set("True-Client-IP", "203.0.113.10")
	req.Header.Set("X-Real-IP", "203.0.113.11")
	req.Header.Set("X-Forwarded-For", "203.0.113.12, 198.51.100.7")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if observed != "192.168.1.99:44444" {
		t.Errorf("empty-allowlist middleware MUST be a pass-through; r.RemoteAddr observed downstream = %q, want %q", observed, "192.168.1.99:44444")
	}
}

func TestSecR30_TrustedProxyMiddleware_TrustedPeerHonorsXForwardedFor(t *testing.T) {
	// Direct middleware unit covering the trusted-peer honor path with
	// all three header variants. Asserts the header-precedence order
	// matches chi.middleware.RealIP (True-Client-IP > X-Real-IP > XFF).
	deps := &Dependencies{TrustedProxies: []string{"127.0.0.0/8"}}
	mw := deps.trustedProxyRealIPMiddleware()

	cases := []struct {
		name   string
		setHdr func(*http.Request)
		wantIP string
	}{
		{
			name: "True-Client-IP wins over X-Real-IP and XFF",
			setHdr: func(r *http.Request) {
				r.Header.Set("True-Client-IP", "203.0.113.1")
				r.Header.Set("X-Real-IP", "203.0.113.2")
				r.Header.Set("X-Forwarded-For", "203.0.113.3")
			},
			wantIP: "203.0.113.1",
		},
		{
			name: "X-Real-IP wins over XFF when True-Client-IP absent",
			setHdr: func(r *http.Request) {
				r.Header.Set("X-Real-IP", "203.0.113.2")
				r.Header.Set("X-Forwarded-For", "203.0.113.3, 198.51.100.7")
			},
			wantIP: "203.0.113.2",
		},
		{
			name: "XFF leftmost wins when only XFF present",
			setHdr: func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "203.0.113.3, 198.51.100.7")
			},
			wantIP: "203.0.113.3",
		},
		{
			name: "unparseable forwarded header → raw peer preserved",
			setHdr: func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "not-an-ip")
			},
			wantIP: "127.0.0.1:55555",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var observed string
			handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				observed = r.RemoteAddr
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodGet, "/anything", nil)
			req.RemoteAddr = "127.0.0.1:55555"
			tc.setHdr(req)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if observed != tc.wantIP {
				t.Errorf("downstream RemoteAddr = %q, want %q", observed, tc.wantIP)
			}
		})
	}
}

func TestSecR30_TrustedProxyMiddleware_DropsMalformedCIDRButTrustsRest(t *testing.T) {
	// One malformed entry MUST be logged + dropped, but a sibling
	// well-formed CIDR MUST still grant trust. Proves a single
	// operator typo does not silently disable the gate AND does not
	// silently grant trust everywhere.
	deps := &Dependencies{TrustedProxies: []string{"not-a-cidr", "127.0.0.0/8"}}
	mw := deps.trustedProxyRealIPMiddleware()

	var observed string
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed = r.RemoteAddr
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.RemoteAddr = "127.0.0.1:55555"
	req.Header.Set("X-Forwarded-For", "203.0.113.99")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if observed != "203.0.113.99" {
		t.Errorf("with one malformed + one valid CIDR, the trusted peer 127.0.0.1 MUST still have its XFF honoured; observed RemoteAddr = %q", observed)
	}
}

// itoa is a local small-int formatter to avoid pulling strconv into this
// test file's import list (and to keep the SecR30 tests self-contained).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
