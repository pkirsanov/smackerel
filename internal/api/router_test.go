package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
func (m *mockWebUI) DigestPage(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
func (m *mockWebUI) TopicsPage(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
func (m *mockWebUI) SettingsPage(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
func (m *mockWebUI) StatusPage(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

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

// --- OAuth callback is NOT rate-limited ---

func TestOAuthCallback_NotRateLimited(t *testing.T) {
	deps := &Dependencies{
		DB:           &mockDB{healthy: true},
		NATS:         &mockNATS{healthy: true},
		StartTime:    time.Now(),
		OAuthHandler: &mockOAuth{},
	}

	router := NewRouter(deps)

	// Send 15 requests to callback — all should succeed (no rate limit).
	for i := 0; i < 15; i++ {
		req := httptest.NewRequest(http.MethodGet, "/auth/google/callback", nil)
		req.RemoteAddr = "192.168.1.200:12345"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("callback request %d: expected 200, got %d", i+1, rec.Code)
		}
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

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
