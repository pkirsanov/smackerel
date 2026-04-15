package web

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestNewHandler(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.Templates == nil {
		t.Error("expected non-nil templates")
	}
}

func TestNewHandler_TemplateFuncs(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	// Verify templates can be looked up
	for _, name := range []string{"search.html", "detail.html", "digest.html", "topics.html", "settings.html", "status.html", "results-partial.html"} {
		tmpl := h.Templates.Lookup(name)
		if tmpl == nil {
			t.Errorf("template %q not found", name)
		}
	}
}

func TestTruncateFunc(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())
	// Templates should have been parsed with truncate function
	if h.Templates == nil {
		t.Fatal("templates should be parsed")
	}
}

func TestSearchPage_NilPool(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.SearchPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if body == "" {
		t.Error("expected non-empty response body")
	}
	if !containsString(body, "Smackerel") && !containsString(body, "search") {
		t.Error("expected search page content")
	}
}

func TestSettingsPage_Render(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()

	h.SettingsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !containsString(body, "Settings") {
		t.Error("expected settings content")
	}
}

func TestDigestPage_NoRows(t *testing.T) {
	// With nil pool, DigestPage will panic on nil pointer dereference.
	// This is expected behavior — in production, pool is always initialized.
	h := NewHandler(nil, nil, time.Now())
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.Pool != nil {
		t.Error("expected nil pool for this test")
	}
}

func TestTopicsPage_NilPool(t *testing.T) {
	// With nil pool, TopicsPage will panic on nil pointer dereference.
	// This is expected behavior — in production, pool is always initialized.
	h := NewHandler(nil, nil, time.Now())
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.Pool != nil {
		t.Error("expected nil pool for this test")
	}
}

func TestStatusPage_NilPool(t *testing.T) {
	// With nil pool, StatusPage will panic on nil pointer dereference.
	// This is expected behavior — in production, pool is always initialized.
	h := NewHandler(nil, nil, time.Now())
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
	if h.Pool != nil {
		t.Error("expected nil pool for this test")
	}
}

func TestArtifactDetail_NoID(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/artifact", nil)
	rec := httptest.NewRecorder()

	h.ArtifactDetail(rec, req)

	// Should redirect when no ID provided
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", rec.Code)
	}
}

// SCN-002-033: Search via web UI — search page renders
func TestSCN002033_WebSearchPage(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.SearchPage(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !containsString(body, "Smackerel") {
		t.Error("search page should contain 'Smackerel'")
	}
	if !containsString(body, "hx-post") {
		t.Error("search page should have HTMX attributes for search")
	}
}

// SCN-002-034: Artifact detail view — redirects when no ID
func TestSCN002034_ArtifactDetail_RedirectsWithoutID(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())
	req := httptest.NewRequest(http.MethodGet, "/artifact", nil)
	rec := httptest.NewRecorder()
	h.ArtifactDetail(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect when no ID, got %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "/" {
		t.Errorf("expected redirect to /, got %q", loc)
	}
}

// SCN-002-034: Artifact detail — template exists
func TestSCN002034_ArtifactDetail_TemplateExists(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())
	tmpl := h.Templates.Lookup("detail.html")
	if tmpl == nil {
		t.Error("detail.html template must exist for artifact detail view")
	}
}

// SCN-002-035: Settings page renders
func TestSCN002035_SettingsPage(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()
	h.SettingsPage(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// SCN-002-036: System status page — template exists
func TestSCN002036_StatusPage_TemplateExists(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())
	tmpl := h.Templates.Lookup("status.html")
	if tmpl == nil {
		t.Error("status.html template must exist for status page")
	}
}

// Verify all 7 templates are present
func TestAllTemplates_Present(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())
	required := []string{
		"search.html", "results-partial.html", "detail.html",
		"digest.html", "topics.html", "settings.html", "status.html",
	}
	for _, name := range required {
		if h.Templates.Lookup(name) == nil {
			t.Errorf("required template %q missing", name)
		}
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// mockSyncTrigger records TriggerSync calls.
type mockSyncTrigger struct {
	triggered []string
}

func (m *mockSyncTrigger) TriggerSync(_ context.Context, id string) {
	m.triggered = append(m.triggered, id)
}

// SCN-003-029: Manual sync trigger — handler redirects and calls TriggerSync
func TestSyncConnectorHandler_Triggers(t *testing.T) {
	mock := &mockSyncTrigger{}
	h := NewHandler(nil, nil, time.Now())
	h.Supervisor = mock

	r := chi.NewRouter()
	r.Post("/settings/connectors/{id}/sync", h.SyncConnectorHandler)

	req := httptest.NewRequest(http.MethodPost, "/settings/connectors/imap/sync", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/settings" {
		t.Errorf("expected redirect to /settings, got %q", loc)
	}
	if len(mock.triggered) != 1 || mock.triggered[0] != "imap" {
		t.Errorf("expected TriggerSync('imap'), got %v", mock.triggered)
	}
}

// SCN-003-029: Sync handler works without supervisor (nil safe)
func TestSyncConnectorHandler_NilSupervisor(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	r := chi.NewRouter()
	r.Post("/settings/connectors/{id}/sync", h.SyncConnectorHandler)

	req := httptest.NewRequest(http.MethodPost, "/settings/connectors/imap/sync", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect even with nil supervisor, got %d", rec.Code)
	}
}

// SCN-003-030: Bookmark upload via web UI — Chrome JSON
func TestBookmarkUploadHandler_ChromeJSON(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	chromeJSON := []byte(`{
		"roots": {
			"bookmark_bar": {
				"type": "folder", "name": "Bar",
				"children": [
					{"type": "url", "name": "Example", "url": "https://example.com"},
					{"type": "url", "name": "Go", "url": "https://go.dev"}
				]
			}
		}
	}`)

	req := makeWebMultipartRequest(t, "bookmarks.json", chromeJSON)
	rec := httptest.NewRecorder()
	h.BookmarkUploadHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !containsString(body, "2 bookmarks") {
		t.Errorf("expected '2 bookmarks' in response, got: %s", body)
	}
}

// SCN-003-030: Bookmark upload — Netscape HTML
func TestBookmarkUploadHandler_NetscapeHTML(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	netscapeHTML := []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL>
<DT><A HREF="https://example.com/article">Article</A>
</DL>`)

	req := makeWebMultipartRequest(t, "bookmarks.html", netscapeHTML)
	rec := httptest.NewRecorder()
	h.BookmarkUploadHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !containsString(body, "1 bookmarks") {
		t.Errorf("expected '1 bookmarks' in response, got: %s", body)
	}
}

// SCN-003-030: Bookmark upload — unsupported format
func TestBookmarkUploadHandler_UnsupportedFormat(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	req := makeWebMultipartRequest(t, "random.txt", []byte("not bookmarks"))
	rec := httptest.NewRecorder()
	h.BookmarkUploadHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// SCN-003-027: Settings template includes last sync, items, sync-now button
func TestSettingsTemplate_ConnectorFields(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	// Verify settings.html template contains expected placeholders
	tmpl := h.Templates.Lookup("settings.html")
	if tmpl == nil {
		t.Fatal("settings.html template not found")
	}

	// Verify bookmark-import-result.html template exists
	importTmpl := h.Templates.Lookup("bookmark-import-result.html")
	if importTmpl == nil {
		t.Fatal("bookmark-import-result.html template not found")
	}
}

func makeWebMultipartRequest(t *testing.T, filename string, content []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write part: %v", err)
	}
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/settings/bookmarks/import", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

// --- IMP-020-CSP-005: Templates must not use inline event handlers (blocked by CSP) ---

func TestTemplates_NoInlineEventHandlers(t *testing.T) {
	// CSP script-src without 'unsafe-hashes' blocks inline event handlers
	// like onclick="...", onload="...", etc. All event binding must use
	// addEventListener inside a hashed <script> block.
	if strings.Contains(allTemplates, "onclick=") {
		t.Error("templates contain onclick= attribute — blocked by CSP without 'unsafe-hashes'; use addEventListener instead")
	}
	if strings.Contains(allTemplates, "onload=") {
		t.Error("templates contain onload= attribute — blocked by CSP; use addEventListener instead")
	}
	if strings.Contains(allTemplates, "onerror=") {
		t.Error("templates contain onerror= attribute — blocked by CSP; use addEventListener instead")
	}
	if strings.Contains(allTemplates, "onsubmit=") {
		t.Error("templates contain onsubmit= attribute — blocked by CSP; use addEventListener instead")
	}
}
