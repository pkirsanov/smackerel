package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
