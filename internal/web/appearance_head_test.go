package web

// Spec 106 SCOPE-106-01 — XP106-01-U (server head-adapter unit proof).
//
// The AppearanceHeadStamp middleware is the render-path consumer that closes the
// null-<html>-at-first-paint bug: it parses the smk_appearance cookie with the
// codec and stamps the resolved data-theme/data-density/data-appearance-source
// (and, for an invalid preference, data-appearance-diagnostic) onto the document
// <html> element of full server HTML pages, mirroring the PWA pre-paint resolver
// EXACTLY so server and PWA renderers resolve the same cookie identically.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// htmlDocHandler renders a minimal full HTML document like every shared server
// head does: "<!DOCTYPE html>\n<html lang=\"en\">...".
func htmlDocHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<!DOCTYPE html>\n<html lang=\"en\">\n<head><title>T</title></head>\n<body>hi</body>\n</html>")
}

func doStampRequest(t *testing.T, path, cookie string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: AppearanceCookieName, Value: cookie})
	}
	rec := httptest.NewRecorder()
	AppearanceHeadStamp(handler).ServeHTTP(rec, req)
	return rec
}

func TestAppearanceHeadStamp_StampsResolvedThemeAndDensityBeforeFirstPaint(t *testing.T) {
	// A valid preference stamps the exact resolved attributes on <html>, and the
	// server emits the SAME attributes the PWA pre-paint resolver would set
	// (data-appearance-source="cookie"), so the two renderers are coherent.
	rec := doStampRequest(t, "/", "v1:dark:compact", htmlDocHandler)
	body := rec.Body.String()

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	want := `<html data-theme="dark" data-density="compact" data-appearance-source="cookie" lang="en">`
	if !strings.Contains(body, want) {
		t.Errorf("stamped <html> not found.\n got: %q\nwant substring: %q", body, want)
	}
	// The stamp must not have been double-applied, and the doctype/body survive.
	if strings.Count(body, "data-theme=") != 1 {
		t.Errorf("data-theme stamped %d times, want exactly 1", strings.Count(body, "data-theme="))
	}
	if !strings.HasPrefix(body, "<!DOCTYPE html>") || !strings.Contains(body, "<body>hi</body>") {
		t.Errorf("document was corrupted by stamping: %q", body)
	}
	// Content-Length must match the rewritten body so the response is intact.
	if cl := rec.Result().ContentLength; cl != int64(len(body)) {
		t.Errorf("Content-Length = %d, want %d (rewritten body length)", cl, len(body))
	}
}

func TestAppearanceHeadStamp_MissingCookieStampsExplicitInitialState(t *testing.T) {
	// No cookie -> the codec's explicit initial state (system/comfortable) with
	// source="missing" and NO diagnostic, matching the PWA resolver's "missing".
	rec := doStampRequest(t, "/", "", htmlDocHandler)
	body := rec.Body.String()
	want := `<html data-theme="system" data-density="comfortable" data-appearance-source="missing" lang="en">`
	if !strings.Contains(body, want) {
		t.Errorf("missing-cookie stamp wrong.\n got: %q\nwant substring: %q", body, want)
	}
	if strings.Contains(body, "data-appearance-diagnostic") {
		t.Errorf("missing cookie must NOT carry a diagnostic attribute: %q", body)
	}
}

func TestAppearanceHeadStamp_InvalidCookieStampsInitialStatePlusDiagnostic(t *testing.T) {
	// A malformed cookie fail-loud-resolves to the initial state, source="invalid"
	// and a preference_invalid diagnostic — never coerced to a "nearest" theme.
	rec := doStampRequest(t, "/", "v1:sepia:compact", htmlDocHandler)
	body := rec.Body.String()
	want := `<html data-theme="system" data-density="comfortable" data-appearance-source="invalid" data-appearance-diagnostic="preference_invalid" lang="en">`
	if !strings.Contains(body, want) {
		t.Errorf("invalid-cookie stamp wrong.\n got: %q\nwant substring: %q", body, want)
	}
}

func TestAppearanceHeadStamp_LeavesHTMXFragmentUntouched(t *testing.T) {
	// An HTMX fragment (text/html but NO doctype) must pass through byte-for-byte
	// — the head-adapter only stamps full documents.
	fragment := `<div class="results"><span>no doctype here</span></div>`
	rec := doStampRequest(t, "/search", "v1:dark:compact", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, fragment)
	})
	if got := rec.Body.String(); got != fragment {
		t.Errorf("fragment was modified.\n got: %q\nwant: %q", got, fragment)
	}
	if strings.Contains(rec.Body.String(), "data-theme") {
		t.Error("fragment must not be stamped with appearance attributes")
	}
}

func TestAppearanceHeadStamp_LeavesJSONUntouched(t *testing.T) {
	// A JSON API response is not text/html and must pass through untouched.
	payload := `{"ok":true,"html":"<html>not a document</html>"}`
	rec := doStampRequest(t, "/", "v1:dark:compact", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, payload)
	})
	if got := rec.Body.String(); got != payload {
		t.Errorf("JSON body was modified.\n got: %q\nwant: %q", got, payload)
	}
}

func TestAppearanceHeadStamp_SkipsPWAAndAPIPaths(t *testing.T) {
	// Static PWA pages resolve appearance via their own pre-paint asset and must
	// keep byte-exact identity; API paths carry no document to stamp.
	for _, path := range []string{"/pwa/", "/pwa/index.html", "/api/health", "/v1/web/login", "/metrics"} {
		rec := doStampRequest(t, path, "v1:dark:compact", htmlDocHandler)
		if strings.Contains(rec.Body.String(), "data-theme") {
			t.Errorf("path %s must be skipped by the head-adapter, but was stamped", path)
		}
	}
}

func TestAppearanceHeadStamp_PreservesRedirectStatusAndBody(t *testing.T) {
	// A redirect (303 + short non-document body) must keep its status and body —
	// the canaries require /, /cards, /pwa/ to answer 200/303/401, never a 5xx.
	rec := doStampRequest(t, "/", "v1:dark:compact", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("redirect status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login", loc)
	}
	if strings.Contains(rec.Body.String(), "data-theme") {
		t.Errorf("redirect body must not be stamped: %q", rec.Body.String())
	}
}

func TestAppearanceHeadStamp_PreservesUpstreamHeaders(t *testing.T) {
	// Headers set by upstream middleware (e.g. the CSP from securityHeaders) must
	// survive the buffered rewrite.
	rec := doStampRequest(t, "/", "v1:light:comfortable", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, "<!DOCTYPE html>\n<html lang=\"en\"><body>x</body></html>")
	})
	if csp := rec.Header().Get("Content-Security-Policy"); csp != "default-src 'self'" {
		t.Errorf("CSP header lost: %q", csp)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control header lost: %q", cc)
	}
	if !strings.Contains(rec.Body.String(), `data-theme="light"`) {
		t.Errorf("light theme not stamped: %q", rec.Body.String())
	}
}
