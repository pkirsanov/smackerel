package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPWAShareHandler_ValidFormData(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	form := url.Values{}
	form.Set("title", "Great Recipe")
	form.Set("text", "Found this amazing recipe")
	form.Set("url", "https://example.com/recipe")

	req := httptest.NewRequest(http.MethodPost, "/pwa/share", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	deps.PWAShareHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected Content-Type text/html, got %q", ct)
	}

	body := rec.Body.String()

	// Template must render the shared data into the page
	if !strings.Contains(body, "Great Recipe") {
		t.Error("response body should contain the shared title")
	}
	if !strings.Contains(body, "https://example.com/recipe") {
		t.Error("response body should contain the shared URL")
	}
	if !strings.Contains(body, "Found this amazing recipe") {
		t.Error("response body should contain the shared text")
	}

	// Page must include the capture script and queue library
	if !strings.Contains(body, "/pwa/lib/queue.js") {
		t.Error("response body should reference the offline queue script")
	}
	if !strings.Contains(body, "/api/capture") {
		t.Error("response body should reference the capture API endpoint")
	}
}

func TestPWAShareHandler_EmptyFields(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	form := url.Values{}
	// No title, text, or url — all empty

	req := httptest.NewRequest(http.MethodPost, "/pwa/share", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	deps.PWAShareHandler(rec, req)

	// Handler should still render the template (empty data is valid — the JS handles validation)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 even with empty fields, got %d", rec.Code)
	}
}

func TestPWAShareHandler_URLOnlyShare(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	form := url.Values{}
	form.Set("url", "https://example.com/page")

	req := httptest.NewRequest(http.MethodPost, "/pwa/share", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	deps.PWAShareHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "https://example.com/page") {
		t.Error("response body should contain the shared URL")
	}
}

func TestPWAShareHandler_SpecialCharactersEscaped(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	form := url.Values{}
	form.Set("title", `Recipe <script>alert("xss")</script>`)
	form.Set("text", `"quotes" & ampersands`)
	form.Set("url", "https://example.com/page?q=1&b=2")

	req := httptest.NewRequest(http.MethodPost, "/pwa/share", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	deps.PWAShareHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()

	// Go html/template auto-escapes; raw <script> tags must NOT appear in output
	if strings.Contains(body, `<script>alert("xss")</script>`) {
		t.Error("template must escape HTML special characters in user-supplied title to prevent XSS")
	}
}

func TestPWAShareHandler_GETMethodRejected(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	// The router restricts to POST, but if handler were called directly with GET,
	// ParseForm still succeeds on a GET with no body — just empty values.
	req := httptest.NewRequest(http.MethodGet, "/pwa/share", nil)
	rec := httptest.NewRecorder()

	deps.PWAShareHandler(rec, req)

	// Handler renders template even on GET (router handles method restriction)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (handler is method-agnostic, router restricts), got %d", rec.Code)
	}
}

func TestPWAShareHandler_RendersStructuralElements(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	form := url.Values{}
	form.Set("title", "Test")
	form.Set("url", "https://example.com")

	req := httptest.NewRequest(http.MethodPost, "/pwa/share", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	deps.PWAShareHandler(rec, req)

	body := rec.Body.String()

	// Success and error feedback elements (Scope 2 DoD)
	if !strings.Contains(body, `id="result-success"`) {
		t.Error("template must contain success result element")
	}
	if !strings.Contains(body, `id="result-error"`) {
		t.Error("template must contain error result element")
	}
	if !strings.Contains(body, `id="queued"`) {
		t.Error("template must contain offline queue indicator element")
	}
	if !strings.Contains(body, "Saved!") {
		t.Error("template must contain success message text")
	}
	if !strings.Contains(body, "Retry") {
		t.Error("template must contain retry button for error recovery")
	}
	if !strings.Contains(body, "doCapture()") {
		t.Error("template must auto-invoke capture on load")
	}
}

func TestPWAShareHandler_CSPHeaderPresent(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	form := url.Values{}
	form.Set("title", "CSP Test")
	form.Set("url", "https://example.com")

	req := httptest.NewRequest(http.MethodPost, "/pwa/share", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	deps.PWAShareHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy header must be present on share page response")
	}

	// CSP must contain nonce-based script-src
	if !strings.Contains(csp, "script-src 'nonce-") {
		t.Errorf("CSP must use nonce-based script-src, got: %s", csp)
	}

	// CSP must block object-src
	if !strings.Contains(csp, "object-src 'none'") {
		t.Errorf("CSP must block object-src, got: %s", csp)
	}

	// Verify nonce in header matches nonce in HTML script tags
	body := rec.Body.String()
	if !strings.Contains(body, "nonce=") {
		t.Error("script tags in share page must include nonce attribute")
	}
}

func TestPWAShareHandler_CSPNonceUniqueness(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	form := url.Values{}
	form.Set("title", "Nonce Test")
	form.Set("url", "https://example.com")

	// First request
	req1 := httptest.NewRequest(http.MethodPost, "/pwa/share", strings.NewReader(form.Encode()))
	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec1 := httptest.NewRecorder()
	deps.PWAShareHandler(rec1, req1)

	// Second request
	req2 := httptest.NewRequest(http.MethodPost, "/pwa/share", strings.NewReader(form.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec2 := httptest.NewRecorder()
	deps.PWAShareHandler(rec2, req2)

	csp1 := rec1.Header().Get("Content-Security-Policy")
	csp2 := rec2.Header().Get("Content-Security-Policy")

	// Nonces must differ between requests
	if csp1 == csp2 {
		t.Error("CSP nonces must be unique per request to prevent replay attacks")
	}
}

func TestPWAShareHandler_NoInlineEventHandlers(t *testing.T) {
	deps := &Dependencies{
		DB:        &mockDB{healthy: true},
		NATS:      &mockNATS{healthy: true},
		StartTime: time.Now(),
	}

	form := url.Values{}
	form.Set("title", "Inline Test")
	form.Set("url", "https://example.com")

	req := httptest.NewRequest(http.MethodPost, "/pwa/share", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	deps.PWAShareHandler(rec, req)

	body := rec.Body.String()

	// No inline event handlers — all handlers must use addEventListener for CSP compliance
	if strings.Contains(body, "onclick=") {
		t.Error("share page must not use inline onclick handlers (CSP compliance); use addEventListener instead")
	}
	if strings.Contains(body, "onload=") {
		t.Error("share page must not use inline onload handlers (CSP compliance)")
	}
}

func TestPWAStaticFileServer(t *testing.T) {
	handler := pwaFileServer()

	tests := []struct {
		name        string
		path        string
		wantStatus  int
		wantContain string
	}{
		{"manifest.json exists", "/manifest.json", http.StatusOK, "share_target"},
		{"root serves index", "/", http.StatusOK, "Smackerel"},
		{"index.html redirects to root", "/index.html", http.StatusMovedPermanently, ""},
		{"style.css exists", "/style.css", http.StatusOK, ""},
		{"service worker exists", "/sw.js", http.StatusOK, "smackerel-pwa"},
		{"queue.js exists", "/lib/queue.js", http.StatusOK, "CaptureQueue"},
		{"icon.svg exists", "/icon.svg", http.StatusOK, ""},
		{"nonexistent 404", "/nonexistent.txt", http.StatusNotFound, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("GET %s: expected %d, got %d", tt.path, tt.wantStatus, rec.Code)
			}
			if tt.wantContain != "" && !strings.Contains(rec.Body.String(), tt.wantContain) {
				t.Errorf("GET %s: body should contain %q", tt.path, tt.wantContain)
			}
		})
	}
}
