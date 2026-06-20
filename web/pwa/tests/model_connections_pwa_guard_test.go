// Spec 096 SCOPE-06 (frontend) — static UI guard for the operator-gated
// model-connections PWA triad (model-connections.{html,js},
// model-connection-add.{html,js}, model-connection-detail.{html,js}).
//
// This is the runnable `ui-unit` surface for the triad: it reads the committed
// PWA source and asserts the three BINDING UX contracts from spec.md without a
// live stack (the live `e2e-ui` Playwright legs are deferred to a clean-stack
// dispatch). It mirrors assistant_source_href_security_guard_test.go (read the
// committed source, assert a contract) — the accepted pattern for auth-gated
// PWA pages the disposable e2e-ui stack cannot drive here — and every required
// check ships with an adversarial twin proving the guard detects a regression.
//
// Contracts pinned:
//  1. WRITE-ONLY SECRET — the only credential input is type=password,
//     autocomplete=off, empty on load, never pre-filled, never echoed; NO
//     reveal/show/copy/unmask control, NO text-type secret input, and no
//     assignment of server data into an input value.
//  2. OPERATOR BOUNDARY — every page handles 401 (→ /login?next=) and 403
//     (→ operator-only notice with no setup affordances).
//  3. TRUTHFUL TEST — the detail page renders a failed probe as a role=alert
//     danger banner via an explicit outcome==="ok" branch, never a false
//     success and never an Ollama substitute.
package webcodegen_drift_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// readPWA reads a committed file under web/pwa/ relative to the repo root.
func readPWA(t *testing.T, name string) string {
	t.Helper()
	root := repoRoot(t)
	p := filepath.Join(root, "web", "pwa", name)
	body, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(body)
}

// The three credential-bearing JS surfaces (secret entry happens in add/detail;
// the list never accepts a secret but must still honor the operator boundary).
var modelConnSecretJS = []string{
	"model-connection-add.js",
	"model-connection-detail.js",
}

var modelConnAllJS = []string{
	"model-connections.js",
	"model-connection-add.js",
	"model-connection-detail.js",
}

var modelConnAllHTML = []string{
	"model-connections.html",
	"model-connection-add.html",
	"model-connection-detail.html",
}

// requiredWriteOnlyPatterns MUST be present in each secret-bearing JS file so a
// future edit cannot silently weaken the write-only credential input.
var requiredWriteOnlyPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"secret input is a password field", regexp.MustCompile(`\.type\s*=\s*"password"`)},
	{"secret input disables autocomplete", regexp.MustCompile(`\.autocomplete\s*=\s*"off"`)},
	{"secret input empty on load", regexp.MustCompile(`\.value\s*=\s*""`)},
	{"secret input marked never-displayed", regexp.MustCompile(`never displayed`)},
}

// forbiddenWriteOnlyPatterns MUST NOT appear in any secret-bearing JS file.
// A reveal/show/copy control, a text-type secret input, or an assignment of a
// variable / non-empty literal into an input value would echo or expose the
// secret — exactly what the write-only contract forbids.
var forbiddenWriteOnlyPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"reveal / show-password / copy-secret / unmask control", regexp.MustCompile(`(?i)reveal|show.?password|copy.?secret|unmask|toggle.?password`)},
	{"secret input switched to a visible text field", regexp.MustCompile(`\.type\s*=\s*"text"`)},
	{"server data assigned into an input value (echo)", regexp.MustCompile(`\.value\s*=\s*[A-Za-z_]`)},
	{"non-empty literal assigned into an input value (pre-fill)", regexp.MustCompile(`\.value\s*=\s*"[^"]`)},
}

// TestModelConnectionsWriteOnlySecretAffordance_Spec096 locks the write-only
// credential input in the add + detail PWA JS (DoD: write-only secret).
func TestModelConnectionsWriteOnlySecretAffordance_Spec096(t *testing.T) {
	for _, name := range modelConnSecretJS {
		src := stripLineComments(readPWA(t, name))

		for _, want := range requiredWriteOnlyPatterns {
			if !want.re.MatchString(src) {
				t.Errorf("%s is missing the write-only contract wiring %q: pattern %s did not match",
					name, want.name, want.re.String())
			}
		}
		for _, forbid := range forbiddenWriteOnlyPatterns {
			if forbid.re.MatchString(src) {
				t.Errorf("%s contains a forbidden write-only violation (%s): pattern %s must not appear",
					name, forbid.name, forbid.re.String())
			}
		}
	}

	// Every credential surface's HTML carries the operator-only notice block.
	for _, name := range modelConnAllHTML {
		html := readPWA(t, name)
		if !regexp.MustCompile(`id="operator-only"`).MatchString(html) {
			t.Errorf("%s is missing the operator-only notice block (FR-B4)", name)
		}
	}
}

// TestModelConnectionsWriteOnly_Adversarial_Spec096 proves the write-only guard
// is not tautological: the echo/reveal shapes the guard forbids MUST trip the
// forbidden-pattern checks, and a snippet with no password/autocomplete wiring
// MUST fail the required checks. Built from in-memory strings, never a file.
func TestModelConnectionsWriteOnly_Adversarial_Spec096(t *testing.T) {
	echoRegression := `
	  var input = document.createElement("input");
	  input.type = "text";
	  input.value = view.secret_redaction;
	  var btn = document.createElement("button");
	  btn.textContent = "Reveal secret";
	`
	trippedAny := false
	for _, forbid := range forbiddenWriteOnlyPatterns {
		if forbid.re.MatchString(echoRegression) {
			trippedAny = true
		}
	}
	if !trippedAny {
		t.Fatal("adversarial echo/reveal regression tripped NO forbidden pattern — the write-only guard would miss a real violation")
	}

	missingWiring := `
	  var input = document.createElement("input");
	  input.placeholder = "secret";
	`
	for _, want := range requiredWriteOnlyPatterns {
		if want.re.MatchString(missingWiring) {
			t.Errorf("adversarial no-wiring snippet unexpectedly satisfied required pattern %q (%s) — guard is tautological",
				want.name, want.re.String())
		}
	}
}

// requiredOperatorBoundaryPatterns MUST be present in EVERY triad JS file.
var requiredOperatorBoundaryPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"401 anonymous handled", regexp.MustCompile(`status\s*===\s*401`)},
	{"401 routes to web login", regexp.MustCompile(`/login\?next=`)},
	{"403 non-operator handled", regexp.MustCompile(`status\s*===\s*403`)},
	{"403 shows operator-only notice", regexp.MustCompile(`showOperatorOnly`)},
}

// TestModelConnectionsOperatorBoundary_Spec096 locks the 401→login and
// 403→operator-only handling across the whole triad (DoD: operator gate).
func TestModelConnectionsOperatorBoundary_Spec096(t *testing.T) {
	for _, name := range modelConnAllJS {
		src := stripLineComments(readPWA(t, name))
		for _, want := range requiredOperatorBoundaryPatterns {
			if !want.re.MatchString(src) {
				t.Errorf("%s is missing the operator-boundary wiring %q: pattern %s did not match",
					name, want.name, want.re.String())
			}
		}
	}
}

// TestModelConnectionsOperatorBoundary_Adversarial_Spec096 proves the boundary
// guard is meaningful: a fetch handler that omits the 403 branch MUST fail the
// required-pattern check.
func TestModelConnectionsOperatorBoundary_Adversarial_Spec096(t *testing.T) {
	noGate := `
	  var resp = await fetch(ENDPOINT, { credentials: "same-origin" });
	  if (!resp.ok) { showError("HTTP " + resp.status); return; }
	  var body = await resp.json();
	`
	gate403 := regexp.MustCompile(`status\s*===\s*403`)
	if gate403.MatchString(noGate) {
		t.Fatal("adversarial no-gate handler unexpectedly satisfied the 403 check — operator-boundary guard is tautological")
	}
}

// substitutionOllamaRe matches an Ollama FALLBACK / SUBSTITUTION in executable
// code (the SCN-096-W04 anti-pattern) — NOT a benign kind→display-name mapping
// like `case "ollama": return "Ollama"`. It requires "substitut" or a
// "fall…back" within ~40 chars of "ollama" on the same line.
var substitutionOllamaRe = regexp.MustCompile(`(?i)(substitut|fall.{0,6}back).{0,40}ollama|ollama.{0,40}(substitut|fall.{0,6}back)`)

// requiredTruthfulTestPatterns MUST be present in model-connection-detail.js so
// a failed probe is rendered into the role=alert danger banner via an explicit
// outcome==="ok" branch — never a false success.
var requiredTruthfulTestPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"explicit truthful outcome===ok branch", regexp.MustCompile(`outcome\s*===\s*"ok"`)},
	{"ok branch shows the success banner", regexp.MustCompile(`show\(bannerOk\)`)},
	{"fail branch shows the role=alert danger banner", regexp.MustCompile(`show\(bannerFail\)`)},
}

// TestModelConnectionDetailTruthfulTest_NoFalseSuccess_Spec096 locks the
// truthful failed-test rendering in the detail page: an explicit outcome==="ok"
// branch that shows the success banner, a fail path that shows the role=alert
// danger banner, and NO Ollama substitution/fallback in executable code
// (SCN-096-W04 / DoD: truthful test).
func TestModelConnectionDetailTruthfulTest_NoFalseSuccess_Spec096(t *testing.T) {
	js := stripLineComments(readPWA(t, "model-connection-detail.js"))

	for _, want := range requiredTruthfulTestPatterns {
		if !want.re.MatchString(js) {
			t.Errorf("model-connection-detail.js is missing %q: pattern %s did not match — a failed probe could be rendered as success",
				want.name, want.re.String())
		}
	}
	if substitutionOllamaRe.MatchString(js) {
		t.Error("model-connection-detail.js substitutes / falls back to Ollama in executable code — a failed hosted-provider test must NEVER substitute Ollama (SCN-096-W04)")
	}

	html := readPWA(t, "model-connection-detail.html")
	if !regexp.MustCompile(`id="banner-fail"[^>]*role="alert"`).MatchString(html) {
		t.Error("model-connection-detail.html failed-test banner (#banner-fail) must carry role=\"alert\"")
	}
	if !regexp.MustCompile(`id="banner-ok"[^>]*role="status"`).MatchString(html) {
		t.Error("model-connection-detail.html passed-test banner (#banner-ok) must carry role=\"status\"")
	}
}

// TestModelConnectionDetailTruthfulTest_Adversarial_Spec096 proves the truthful
// -test guard is not tautological: a renderer that always shows success (no
// outcome==="ok" branch) MUST fail the required check, an Ollama-fallback
// renderer MUST trip the substitution check, and a benign kind→display-name
// mapping MUST NOT be flagged.
func TestModelConnectionDetailTruthfulTest_Adversarial_Spec096(t *testing.T) {
	falseSuccess := `
	  function renderTestOutcome(result) {
	    showOkBanner("tested ok");
	    var note = "falling back to ollama on failure";
	  }
	`
	if regexp.MustCompile(`outcome\s*===\s*"ok"`).MatchString(falseSuccess) {
		t.Fatal("adversarial always-success renderer unexpectedly satisfied the outcome branch check — truthful-test guard is tautological")
	}
	if !substitutionOllamaRe.MatchString(falseSuccess) {
		t.Fatal("adversarial ollama-fallback renderer did NOT trip the substitution check — truthful-test guard would miss a substitution")
	}

	// The benign kind→display-name mapping the detail page legitimately carries
	// MUST NOT be flagged as an Ollama substitution (guard not over-broad).
	benignLabel := `case "ollama": return "Ollama";`
	if substitutionOllamaRe.MatchString(benignLabel) {
		t.Error("benign kind→display-name mapping was flagged as an Ollama substitution — the truthful-test guard is over-broad")
	}
}
