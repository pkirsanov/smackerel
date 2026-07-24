package web

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/api"
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
	// BUG-002-007: a misconfigured/absent reader must render an honest read_error
	// (HTTP 500), never the old false-empty "No digest generated yet." with
	// today's date. (The prior version only asserted a nil pool and executed no
	// read behavior, so it could not guard this bug.)
	h := NewHandler(nil, nil, time.Now())
	rec := httptest.NewRecorder()
	h.DigestPage(rec, httptest.NewRequest(http.MethodGet, "/digest", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("nil reader: expected 500 read_error, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-digest-state="read_error"`) {
		t.Errorf("nil reader: expected read_error state marker; body=%q", body)
	}
	if strings.Contains(body, "No digest generated yet") {
		t.Error("nil reader: must not render the false-empty first-use copy for a read failure")
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

// errSearchBoom is a synthetic executor failure for the server_error path.
var errSearchBoom = errors.New("search engine boom")

// countingSearchExecutor is the injected zero-domain-work proof seam for
// BUG-002-006 SEARCH-004: it records how many times the Search handler
// dispatched domain work. A blank/control/whitespace-only submission MUST leave
// calls == 0 on both the native-form and HTMX paths.
type countingSearchExecutor struct {
	calls   int
	lastReq api.SearchRequest
	results []api.SearchResult
	err     error
}

func (c *countingSearchExecutor) Search(_ context.Context, req api.SearchRequest) ([]api.SearchResult, int, string, error) {
	c.calls++
	c.lastReq = req
	if c.err != nil {
		return nil, 0, "", c.err
	}
	return c.results, len(c.results), "text_fallback", nil
}

func searchForm(query string) (*http.Request, *httptest.ResponseRecorder) {
	body := url.Values{"query": {query}}.Encode()
	req := httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req, httptest.NewRecorder()
}

// TestSearchSemanticFormAndTypedFullPageFragmentStateMatrix is BUG-002-006
// SEARCH-S02-T01. It proves (SCN-002-006-02/04/05) the progressive-enhancement
// semantic form, the typed full-page/fragment state matrix, and that
// blank/control/whitespace-only input executes ZERO SearchEngine dispatch on
// both the native-form and HTMX paths (HTTP 422 validation permitted).
func TestSearchSemanticFormAndTypedFullPageFragmentStateMatrix(t *testing.T) {
	// --- SCN-002-006-02: the semantic baseline form is present on GET / and
	// submits without any client enhancement (native <form> POST /search). ---
	h := NewHandler(nil, nil, time.Now())
	getRec := httptest.NewRecorder()
	h.SearchPage(getRec, httptest.NewRequest(http.MethodGet, "/", nil))
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /: expected 200, got %d", getRec.Code)
	}
	page := getRec.Body.String()
	for _, want := range []string{
		`method="post"`, `action="/search"`, `role="search"`,
		`name="query"`, `required`, `type="submit"`, `hx-post="/search"`,
		`id="search-outcome"`, `data-search-state="ready"`, "<!DOCTYPE html>",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("GET / semantic form missing %q", want)
		}
	}

	// --- SCN-002-006-04: blank/control/whitespace-only input returns 422 and
	// dispatches ZERO search-domain work, on BOTH native and HTMX paths. ---
	blankInputs := []struct{ name, query string }{
		{"empty", ""},
		{"spaces", "     "},
		{"tabs-newlines", "\t\n\r "},
		{"control-only", "\x01\x02\x03"},
		{"nul-and-mixed", " \t\x00\x1f \n"},
		{"unicode-space", "\u00a0\u2003"},
	}
	paths := []struct {
		name   string
		htmx   bool
		marker string
		wantDoc bool
	}{
		{"native", false, `data-search-form`, true},
		{"htmx", true, `search-outcome-body`, false},
	}
	for _, p := range paths {
		for _, in := range blankInputs {
			exec := &countingSearchExecutor{}
			hb := NewHandler(nil, nil, time.Now())
			hb.SearchExecutorOverride = exec
			req, rec := searchForm(in.query)
			if p.htmx {
				req.Header.Set("HX-Request", "true")
			}
			hb.SearchResults(rec, req)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("%s/%s: expected 422 validation, got %d", p.name, in.name, rec.Code)
			}
			if exec.calls != 0 {
				t.Errorf("%s/%s: expected ZERO SearchEngine dispatch, got %d", p.name, in.name, exec.calls)
			}
			body := rec.Body.String()
			if !strings.Contains(body, `data-search-state="validation"`) {
				t.Errorf("%s/%s: expected validation state marker; body=%q", p.name, in.name, body)
			}
			if !strings.Contains(body, p.marker) {
				t.Errorf("%s/%s: expected %q marker for the %s path", p.name, in.name, p.marker, p.name)
			}
			if hasDoc := strings.Contains(body, "<!DOCTYPE html>"); hasDoc != p.wantDoc {
				t.Errorf("%s/%s: full-document=%v, wanted %v", p.name, in.name, hasDoc, p.wantDoc)
			}
		}
	}

	// --- SCN-002-006-02/05: a searchable query dispatches EXACTLY ONCE, the
	// baseline returns a COMPLETE page with the retained edge-trimmed query, and
	// the HTMX path returns only the outcome fragment. ---
	for _, p := range []struct {
		name    string
		htmx    bool
		wantDoc bool
	}{
		{"native-full-page", false, true},
		{"htmx-fragment", true, false},
	} {
		exec := &countingSearchExecutor{results: []api.SearchResult{
			{ArtifactID: "a-1", Title: "Ramen Notes", ArtifactType: "note", Summary: "broth"},
		}}
		hr := NewHandler(nil, nil, time.Now())
		hr.SearchExecutorOverride = exec
		req, rec := searchForm("  ramen  ")
		if p.htmx {
			req.Header.Set("HX-Request", "true")
		}
		hr.SearchResults(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", p.name, rec.Code)
		}
		if exec.calls != 1 {
			t.Errorf("%s: expected exactly one SearchEngine dispatch, got %d", p.name, exec.calls)
		}
		if exec.lastReq.Query != "ramen" {
			t.Errorf("%s: expected edge-trimmed query %q, got %q", p.name, "ramen", exec.lastReq.Query)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `data-search-state="results"`) {
			t.Errorf("%s: expected results state marker", p.name)
		}
		if !strings.Contains(body, "Ramen Notes") {
			t.Errorf("%s: expected the live result title in the body", p.name)
		}
		if hasDoc := strings.Contains(body, "<!DOCTYPE html>"); hasDoc != p.wantDoc {
			t.Errorf("%s: full-document=%v, wanted %v", p.name, hasDoc, p.wantDoc)
		}
		if p.wantDoc && !strings.Contains(body, `value="ramen"`) {
			t.Errorf("%s: baseline page must retain the query in the field", p.name)
		}
	}

	// --- SCN-002-006-05: an engine failure is a typed server_error (HTTP 500),
	// distinct from empty, with the query retained; exactly one dispatch. ---
	execErr := &countingSearchExecutor{err: errSearchBoom}
	he := NewHandler(nil, nil, time.Now())
	he.SearchExecutorOverride = execErr
	req, rec := searchForm("ramen")
	he.SearchResults(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("server failure: expected 500, got %d", rec.Code)
	}
	if execErr.calls != 1 {
		t.Errorf("server failure: expected one dispatch, got %d", execErr.calls)
	}
	if !strings.Contains(rec.Body.String(), `data-search-state="server_error"`) {
		t.Error("server failure: expected server_error state marker")
	}

	// --- SCN-002-006-05: a valid query with zero results is a typed empty
	// state (HTTP 200) with no error/retry language. ---
	execEmpty := &countingSearchExecutor{}
	hz := NewHandler(nil, nil, time.Now())
	hz.SearchExecutorOverride = execEmpty
	req, rec = searchForm("no-such-thing")
	hz.SearchResults(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("empty: expected 200, got %d", rec.Code)
	}
	emptyBody := rec.Body.String()
	if !strings.Contains(emptyBody, `data-search-state="empty"`) {
		t.Error("empty: expected empty state marker")
	}
	if strings.Contains(emptyBody, `data-search-state="server_error"`) {
		t.Error("empty: must not render server_error language for a zero-result query")
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
	ctxErr    error
}

func (m *mockSyncTrigger) TriggerSync(ctx context.Context, id string) {
	m.triggered = append(m.triggered, id)
	m.ctxErr = ctx.Err()
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

func TestSyncConnectorHandler_DetachesTriggerFromRequestCancellation(t *testing.T) {
	mock := &mockSyncTrigger{}
	h := NewHandler(nil, nil, time.Now())
	h.Supervisor = mock

	r := chi.NewRouter()
	r.Post("/settings/connectors/{id}/sync", h.SyncConnectorHandler)

	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/settings/connectors/qf-decisions/sync", nil).WithContext(requestCtx)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected 303 redirect, got %d", rec.Code)
	}
	if len(mock.triggered) != 1 || mock.triggered[0] != "qf-decisions" {
		t.Errorf("expected TriggerSync('qf-decisions'), got %v", mock.triggered)
	}
	if mock.ctxErr != nil {
		t.Fatalf("manual sync trigger inherited canceled request context: %v", mock.ctxErr)
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

// Scope 6: Nav bar regression — Knowledge link present in nav.
// Spec 100 SCOPE-01 moved the cross-surface links (Knowledge included) from
// inline allTemplates anchors into the single-source app-shell nav partial,
// which the head renders on every page. Assert the link at its new home AND in
// the rendered knowledge-base head so the regression intent is preserved.
func TestNavBar_KnowledgeLink(t *testing.T) {
	if !containsString(appShellNav, `href="/knowledge"`) {
		t.Error("shared app-shell nav should contain the Knowledge link")
	}
	h := NewHandler(nil, nil, time.Now())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.SearchPage(rec, req)
	if !containsString(rec.Body.String(), `href="/knowledge"`) {
		t.Error("rendered nav should contain the Knowledge link")
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
