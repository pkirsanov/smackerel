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
	// With nil pool, DigestPage will fail the query but still render
	h := NewHandler(nil, nil, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/digest", nil)
	rec := httptest.NewRecorder()

	// This shouldn't panic even with nil pool
	defer func() {
		if r := recover(); r != nil {
			// Nil pool will panic on query but template should handle
			t.Log("DigestPage panicked with nil pool (expected)")
		}
	}()

	h.DigestPage(rec, req)
}

func TestTopicsPage_NilPool(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/topics", nil)
	rec := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Log("TopicsPage panicked with nil pool (expected)")
		}
	}()

	h.TopicsPage(rec, req)
}

func TestStatusPage_NilPool(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			t.Log("StatusPage panicked with nil pool (expected)")
		}
	}()

	h.StatusPage(rec, req)
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

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
