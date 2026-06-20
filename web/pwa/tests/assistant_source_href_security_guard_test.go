// Spec 076 security sweep (R25) — web assistant source-link href scheme guard.
//
// Citation URLs rendered by web/pwa/assistant.js originate from web-search
// tool results. The server treats those results as UNTRUSTED: it sanitises
// snippet TEXT (internal/assistant/openknowledge/web/sanitize.go, SCOPE-15)
// but passes result URLs through verbatim (web_search.go states "this tool
// does not re-validate URLs"; the cite-back verifier only lower-cases the
// scheme). If such a URL carried a "javascript:" / "data:" / "vbscript:"
// scheme and the web client assigned it to <a href> unguarded, clicking the
// citation would execute script in the authenticated PWA origin — DOM-XSS
// (OWASP A03). The snippet text was hardened; the URL→href sink was not.
//
// This static guard locks the CLIENT-SIDE scheme allowlist (safeHref) in
// assistant.js so a future edit cannot silently reintroduce an unguarded
// href. It mirrors assistant_robustness_guard_test.go (read the committed
// source, assert a contract) — the accepted pattern for the auth-gated
// assistant page, which the disposable e2e-ui stack cannot drive in a
// browser — and ships with an adversarial twin proving the guard detects a
// regression to the pre-fix bare-assignment shape.
package webcodegen_drift_test

import (
	"regexp"
	"testing"
)

// requiredHrefSchemeGuardPatterns enumerates the client-side scheme-allowlist
// wiring that MUST be present in web/pwa/assistant.js. Each entry carries a
// minimum match count so a single decorative mention cannot satisfy the
// contract (safeHref must be DEFINED and CALLED at the source-link site).
var requiredHrefSchemeGuardPatterns = []struct {
	name    string
	re      *regexp.Regexp
	minHits int
}{
	{"safeHref scheme validator defined", regexp.MustCompile(`function\s+safeHref\s*\(`), 1},
	{"validator allows https scheme", regexp.MustCompile(`protocol\s*===\s*"https:"`), 1},
	{"validator allows http scheme", regexp.MustCompile(`protocol\s*===\s*"http:"`), 1},
	{"source href gated behind safeHref", regexp.MustCompile(`safeHref\s*\(`), 2},
}

// forbiddenHrefSinkPatterns enumerates UNGUARDED href-assignment shapes that
// MUST NOT appear in assistant.js. A bare `a.href = s.url` (the source URL
// straight from an untrusted web-search result) is the DOM-XSS sink this
// guard exists to forbid.
var forbiddenHrefSinkPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"bare href assignment from untrusted source URL", regexp.MustCompile(`\.href\s*=\s*s\.url`)},
}

// TestWebAssistantSourceHrefSchemeGuard_SEC076 fails if the committed
// assistant.js is missing the http(s) scheme allowlist OR still contains the
// bare-href DOM-XSS sink.
func TestWebAssistantSourceHrefSchemeGuard_SEC076(t *testing.T) {
	src := readAssistantJS(t)

	for _, want := range requiredHrefSchemeGuardPatterns {
		got := len(want.re.FindAllStringIndex(src, -1))
		if got < want.minHits {
			t.Errorf("assistant.js is missing the %q href-scheme guard: pattern %s matched %d time(s), want >= %d",
				want.name, want.re.String(), got, want.minHits)
		}
	}

	for _, forbid := range forbiddenHrefSinkPatterns {
		if forbid.re.MatchString(src) {
			t.Errorf("assistant.js contains a forbidden unguarded href sink (%s): pattern %s must not appear — route every source URL through safeHref()",
				forbid.name, forbid.re.String())
		}
	}
}

// TestWebAssistantSourceHrefSchemeGuard_Adversarial_SEC076 proves the guard
// is not tautological: the pre-fix shape (a bare `a.href = s.url` with no
// safeHref allowlist) must FAIL every required pattern AND trip the
// forbidden-sink check. Without this twin, a guard that always passed would
// give false assurance.
func TestWebAssistantSourceHrefSchemeGuard_Adversarial_SEC076(t *testing.T) {
	// A "regressed" assistant client: the exact pre-fix source-link
	// rendering — no safeHref, no scheme allowlist, bare href from the
	// untrusted source URL. This is the DOM-XSS shape the guard forbids.
	regressed := `
		for (const s of sources) {
			const li = document.createElement("li");
			const title = (s && typeof s.title === "string") ? s.title : "(untitled source)";
			if (s && typeof s.url === "string" && s.url.length > 0) {
				const a = document.createElement("a");
				a.href = s.url;
				a.textContent = title;
				a.rel = "noopener noreferrer";
				li.appendChild(a);
			} else {
				li.textContent = title;
			}
			ul.appendChild(li);
		}
	`

	// Every required scheme-guard pattern must be ABSENT from the regressed
	// source — otherwise the guard could not tell the safe form from the sink.
	for _, want := range requiredHrefSchemeGuardPatterns {
		if want.re.MatchString(regressed) {
			t.Errorf("adversarial guard weak: required pattern %q (%s) unexpectedly matched the regressed pre-fix source; the guard would not catch a reintroduced XSS sink",
				want.name, want.re.String())
		}
	}

	// The forbidden bare-href sink MUST be detected in the regressed source.
	detected := false
	for _, forbid := range forbiddenHrefSinkPatterns {
		if forbid.re.MatchString(regressed) {
			detected = true
		}
	}
	if !detected {
		t.Fatalf("adversarial guard weak: no forbidden pattern matched the regressed source; a bare `a.href = s.url` sink would slip through undetected")
	}
}
