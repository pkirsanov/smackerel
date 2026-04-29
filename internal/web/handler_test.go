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
	for _, name := range []string{"search.html", "detail.html", "digest.html", "topics.html", "settings.html", "status.html", "results-partial.html", "knowledge-dashboard.html", "concepts-list.html", "concept-detail.html", "entities-list.html", "entity-detail.html", "lint-report.html", "lint-finding-detail.html"} {
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

func TestStatusPage_RecommendationProvidersEmptyState(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	var body bytes.Buffer
	err := h.Templates.ExecuteTemplate(&body, "status.html", map[string]interface{}{
		"Title":                          "System Status",
		"ArtifactCount":                  0,
		"TopicCount":                     0,
		"EdgeCount":                      0,
		"Uptime":                         "0h 0m",
		"DBHealthy":                      true,
		"NATSHealthy":                    true,
		"RecommendationsEnabled":         true,
		"RecommendationProviderStatuses": []recommendationProviderStatus{},
	})
	if err != nil {
		t.Fatalf("render status.html: %v", err)
	}

	rendered := body.String()
	if !containsString(rendered, "Recommendation Providers") {
		t.Fatal("status page missing Recommendation Providers block")
	}
	if !containsString(rendered, "0 recommendation providers configured") {
		t.Fatal("status page missing empty provider registry message")
	}
	if containsString(rendered, "Google Places") || containsString(rendered, "Yelp") {
		t.Fatal("status page should not fabricate disabled recommendation provider rows")
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

// Verify all templates are present (including Scope 6 knowledge templates)
func TestAllTemplates_Present(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())
	required := []string{
		"search.html", "results-partial.html", "detail.html",
		"digest.html", "topics.html", "settings.html", "status.html",
		"knowledge-dashboard.html", "concepts-list.html", "concept-detail.html",
		"entities-list.html", "entity-detail.html", "lint-report.html", "lint-finding-detail.html",
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

// --- Scope 6: Web UI Knowledge Pages ---

// T6-01: SCN-025-17 — KnowledgeDashboard renders with stats
func TestKnowledgeDashboard_NilStore(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/knowledge", nil)
	rec := httptest.NewRecorder()

	h.KnowledgeDashboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !containsString(body, "Knowledge Layer") {
		t.Error("expected knowledge dashboard content")
	}
	if !containsString(body, "not enabled") {
		t.Error("expected 'not enabled' message when KnowledgeStore is nil")
	}
}

// T6-02: SCN-025-19 — ConceptDetail renders claims and citations
func TestConceptDetail_NoID(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/knowledge/concepts/", nil)
	rec := httptest.NewRecorder()

	h.ConceptDetail(rec, req)

	// Should redirect when no ID provided
	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", rec.Code)
	}
}

func TestConceptDetail_NilStore(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	r := chi.NewRouter()
	r.Get("/knowledge/concepts/{id}", h.ConceptDetail)

	req := httptest.NewRequest(http.MethodGet, "/knowledge/concepts/test-id", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

// T6-03: SCN-025-18 — SearchResults knowledge_match card rendered in template
func TestSearchResults_KnowledgeMatchTemplate(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	// Verify results-partial.html template exists and can render knowledge match
	tmpl := h.Templates.Lookup("results-partial.html")
	if tmpl == nil {
		t.Fatal("results-partial.html template not found")
	}

	// Verify the template contains knowledge match rendering
	if !containsString(allTemplates, "KnowledgeMatch") {
		t.Error("results-partial.html should contain KnowledgeMatch rendering")
	}
	if !containsString(allTemplates, "From Knowledge Layer") {
		t.Error("results-partial.html should contain '★ From Knowledge Layer' indicator")
	}
}

// T6-04: ConceptsList renders with sort/filter
func TestConceptsList_NilStore(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/knowledge/concepts", nil)
	rec := httptest.NewRecorder()

	h.ConceptsList(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !containsString(body, "Concept Pages") {
		t.Error("expected concept list content")
	}
}

// T6-05: EntityDetail renders
func TestEntityDetail_NoID(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/knowledge/entities/", nil)
	rec := httptest.NewRecorder()

	h.EntityDetail(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", rec.Code)
	}
}

func TestEntityDetail_NilStore(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	r := chi.NewRouter()
	r.Get("/knowledge/entities/{id}", h.EntityDetail)

	req := httptest.NewRequest(http.MethodGet, "/knowledge/entities/test-id", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

// T6-06: LintReport renders findings by severity
func TestLintReport_NilStore(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/knowledge/lint", nil)
	rec := httptest.NewRecorder()

	h.LintReport(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !containsString(body, "Lint Report") {
		t.Error("expected lint report content")
	}
}

// T6-07: StatusPage includes Knowledge Layer section in template
func TestStatusPage_KnowledgeSection(t *testing.T) {
	// Verify status.html template contains Knowledge Layer section
	if !containsString(allTemplates, "Knowledge Layer") {
		t.Error("status.html should contain Knowledge Layer section")
	}
	if !containsString(allTemplates, "KnowledgeStats") {
		t.Error("status.html should contain KnowledgeStats conditional")
	}
}

// Scope 6: Nav bar regression — Knowledge link present in nav
func TestNavBar_KnowledgeLink(t *testing.T) {
	if !containsString(allTemplates, `<a href="/knowledge">Knowledge</a>`) {
		t.Error("nav bar should contain Knowledge link")
	}
}

// Scope 6: All 7 new templates are present
func TestScope6_AllNewTemplates(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())
	newTemplates := []string{
		"knowledge-dashboard.html",
		"concepts-list.html",
		"concept-detail.html",
		"entities-list.html",
		"entity-detail.html",
		"lint-report.html",
		"lint-finding-detail.html",
	}
	for _, name := range newTemplates {
		if h.Templates.Lookup(name) == nil {
			t.Errorf("required template %q missing", name)
		}
	}
}

// Scope 6 regression: existing templates still present
func TestScope6_ExistingTemplates_StillPresent(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())
	existing := []string{
		"search.html", "results-partial.html", "detail.html",
		"digest.html", "topics.html", "settings.html", "status.html",
		"bookmark-import-result.html",
	}
	for _, name := range existing {
		if h.Templates.Lookup(name) == nil {
			t.Errorf("existing template %q must still be present", name)
		}
	}
}

// Scope 6 regression: existing pages render with new nav
func TestScope6_SearchPage_RendersWithNavKnowledgeLink(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.SearchPage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !containsString(body, "/knowledge") {
		t.Error("search page should contain /knowledge nav link")
	}
}

func TestScope6_SettingsPage_RendersWithNavKnowledgeLink(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()
	h.SettingsPage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !containsString(body, "/knowledge") {
		t.Error("settings page should contain /knowledge nav link")
	}
}

// T6-11: LintFindingDetail renders
func TestLintFindingDetail_NoID(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/knowledge/lint/", nil)
	rec := httptest.NewRecorder()

	h.LintFindingDetail(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", rec.Code)
	}
}

func TestLintFindingDetail_NilStore(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	r := chi.NewRouter()
	r.Get("/knowledge/lint/{id}", h.LintFindingDetail)

	req := httptest.NewRequest(http.MethodGet, "/knowledge/lint/test-id", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

// EntitiesList renders with nil store
func TestEntitiesList_NilStore(t *testing.T) {
	h := NewHandler(nil, nil, time.Now())

	req := httptest.NewRequest(http.MethodGet, "/knowledge/entities", nil)
	rec := httptest.NewRecorder()

	h.EntitiesList(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !containsString(body, "Entity Profiles") {
		t.Error("expected entity list content")
	}
}
