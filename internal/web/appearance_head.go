package web

// Spec 106 SCOPE-106-01 — server head-adapter: first-paint appearance stamping.
//
// AppearanceHeadStamp WIRES the SCOPE-106-01 AppearancePreferenceCodec
// (ParseAppearanceCookie) into the server response path. It resolves the
// smk_appearance cookie BEFORE the response is flushed and stamps the resolved
//   data-theme / data-density / data-appearance-source
// (and, ONLY for an invalid preference, data-appearance-diagnostic) onto the
// document <html> element of every full server-rendered HTML page. The resolved
// appearance is therefore present at first paint — closing the
// null-<html>-at-first-paint flash — and the server emits the SAME attributes
// the PWA pre-paint resolver (web/pwa/experience-appearance.js) sets, so the two
// renderers resolve the same cookie to the same attributes (SCN-106-009
// cross-renderer coherence).
//
// It is fail-loud / no-default by construction: it never invents a theme; a
// missing or malformed cookie resolves through ParseAppearanceCookie to the
// UX-required explicit initial state (system/comfortable) plus the honest
// diagnostic, exactly like the PWA resolver.
//
// This is a READ-only head-adapter: it consumes the cookie and stamps the
// resolved attributes. Writing/refreshing the preference cookie (the appearance
// toggle) is a separate write path that requires the deployment-supplied
// retention/secure config and is reconciled in the SCOPE-106-04/05 shell
// cutover; nothing here writes a cookie or depends on write config.
//
// Static PWA pages under /pwa/, the JSON/stream API surfaces (/api/, /v1/), the
// health/metrics probes, and static asset trees are never buffered or rewritten
// — they carry no server-rendered <html> document to stamp (the PWA pages
// resolve appearance via their own synchronous pre-paint asset and must keep
// their byte-exact identity + service-worker cache version).

import (
	"bytes"
	"net/http"
	"strconv"
	"strings"
)

// AppearanceHeadStamp is the server head-adapter middleware. It is injected into
// the router via api.Dependencies (set in cmd/core) rather than referenced
// directly, because internal/web imports internal/api and the reverse import
// would be a cycle.
func AppearanceHeadStamp(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if appearanceStampSkip(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		pref, diag := ParseAppearanceCookie(appearanceRawCookie(r))
		sw := &appearanceStampWriter{
			ResponseWriter: w,
			attrs:          appearanceHTMLAttrs(pref, diag),
		}
		next.ServeHTTP(sw, r)
		sw.finish()
	})
}

// appearanceStampSkip reports whether a request path serves no full server
// HTML document the head-adapter should stamp.
func appearanceStampSkip(path string) bool {
	switch path {
	case "/pwa", "/metrics", "/readyz", "/ping":
		return true
	}
	for _, prefix := range []string{"/api/", "/v1/", "/pwa/", "/admin_ui_static/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// appearanceRawCookie returns the raw smk_appearance cookie value, or "" when it
// is absent. ParseAppearanceCookie fail-loud-resolves "" to the explicit initial
// state, so a missing cookie yields the UX-required system/comfortable stamp.
func appearanceRawCookie(r *http.Request) string {
	c, err := r.Cookie(AppearanceCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

// appearanceHTMLAttrs renders the closed-enum <html> attribute string the server
// stamps. It mirrors web/pwa/experience-appearance.js EXACTLY: data-theme,
// data-density, data-appearance-source ("cookie" | "missing" | "invalid"), and
// data-appearance-diagnostic ONLY when the preference is invalid. Every value is
// a fixed enum token derived from the closed codec (never request-controlled
// content), so the attributes are injection-safe.
func appearanceHTMLAttrs(p AppearancePreference, diag AppearanceDiagnostic) string {
	theme, density := p.HTMLDataAttributes()
	source := "cookie"
	switch diag {
	case AppearanceMissing:
		source = "missing"
	case AppearanceInvalid:
		source = "invalid"
	}
	var b strings.Builder
	b.WriteString(`data-theme="`)
	b.WriteString(theme)
	b.WriteString(`" data-density="`)
	b.WriteString(density)
	b.WriteString(`" data-appearance-source="`)
	b.WriteString(source)
	b.WriteByte('"')
	if diag == AppearanceInvalid {
		b.WriteString(` data-appearance-diagnostic="`)
		b.WriteString(string(AppearanceInvalid))
		b.WriteByte('"')
	}
	return b.String()
}

// appearanceStampWriter buffers a candidate full HTML document so its <html>
// open tag can be stamped before the bytes are flushed. The buffer-vs-passthrough
// decision is made on the first write from the response Content-Type plus a
// leading "<!doctype html" sniff: a non-HTML response (JSON, an HTMX fragment, a
// redirect body) streams straight through, byte-for-byte, with its status
// unchanged.
type appearanceStampWriter struct {
	http.ResponseWriter
	attrs       string
	status      int
	wroteHeader bool // underlying WriteHeader already sent (passthrough path)
	decided     bool // buffer-vs-passthrough decision has been made
	buffer      bool // true => accumulate into buf and stamp on finish
	buf         bytes.Buffer
}

// WriteHeader defers the real status write. The underlying WriteHeader is sent
// either when the first write selects passthrough, or in finish() for the
// buffered path (where the stamp changes Content-Length), or in finish() for a
// header-only response (a redirect/401 with no body).
func (a *appearanceStampWriter) WriteHeader(status int) {
	a.status = status
}

func (a *appearanceStampWriter) Write(p []byte) (int, error) {
	if !a.decided {
		a.decide(p)
	}
	if a.buffer {
		return a.buf.Write(p)
	}
	return a.ResponseWriter.Write(p)
}

// decide classifies the response on the first write. It buffers ONLY a full HTML
// document (Content-Type text/html AND a leading "<!doctype html"); every other
// response is passed through untouched.
func (a *appearanceStampWriter) decide(first []byte) {
	a.decided = true
	ct := strings.ToLower(a.Header().Get("Content-Type"))
	if strings.Contains(ct, "text/html") && looksLikeHTMLDocument(first) {
		a.buffer = true
		return
	}
	a.buffer = false
	a.flushHeader()
}

func (a *appearanceStampWriter) flushHeader() {
	if a.wroteHeader {
		return
	}
	if a.status == 0 {
		a.status = http.StatusOK
	}
	a.ResponseWriter.WriteHeader(a.status)
	a.wroteHeader = true
}

// finish stamps + flushes a buffered HTML document, or finalizes a passthrough /
// header-only response.
func (a *appearanceStampWriter) finish() {
	if !a.buffer {
		// Passthrough already streamed its body, or the handler wrote only a
		// header (redirect / 401 / 304) — ensure the deferred status is sent.
		a.flushHeader()
		return
	}
	body := stampHTMLOpen(a.buf.Bytes(), a.attrs)
	// The stamp changes the document length; recompute Content-Length so the
	// buffered body is delivered intact.
	a.Header().Set("Content-Length", strconv.Itoa(len(body)))
	a.flushHeader()
	_, _ = a.ResponseWriter.Write(body)
}

// looksLikeHTMLDocument reports whether b begins (after optional leading
// whitespace) with a "<!doctype html" marker. HTMX fragments and JSON never
// start with a doctype, so only full server documents are stamped.
func looksLikeHTMLDocument(b []byte) bool {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
		i++
	}
	const marker = "<!doctype html"
	rest := b[i:]
	if len(rest) < len(marker) {
		return false
	}
	return strings.EqualFold(string(rest[:len(marker)]), marker)
}

// stampHTMLOpen injects attrs into the document's <html> open tag (case
// insensitive), immediately after "<html", so a bare "<html lang=\"en\">"
// becomes "<html data-theme=... lang=\"en\">". It stamps only the first <html>
// element and never double-stamps a tag that already carries data-theme.
func stampHTMLOpen(body []byte, attrs string) []byte {
	lower := bytes.ToLower(body)
	idx := bytes.Index(lower, []byte("<html"))
	if idx < 0 {
		return body
	}
	gt := bytes.IndexByte(body[idx:], '>')
	if gt < 0 {
		return body
	}
	if bytes.Contains(lower[idx:idx+gt], []byte("data-theme")) {
		return body // already stamped — never duplicate.
	}
	insertAt := idx + len("<html")
	out := make([]byte, 0, len(body)+len(attrs)+1)
	out = append(out, body[:insertAt]...)
	out = append(out, ' ')
	out = append(out, attrs...)
	out = append(out, body[insertAt:]...)
	return out
}
