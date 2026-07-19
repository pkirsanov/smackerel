// Spec 073 web assistant robustness source-contract guard (BUG-073-002).
//
// The assistant page is auth-gated, so the disposable e2e-ui stack cannot
// drive the composer in a browser (the paired Playwright specs are served-
// route stubs and the real coverage is server-side Go e2e). This static
// guard locks the CLIENT-SIDE robustness wiring in web/pwa/assistant.js so a
// future edit cannot silently drop it:
//
//   - single-flight guard (inFlight) so a rapid double-submit cannot fire
//     two overlapping turns with distinct transport_message_ids;
//   - an AbortController request timeout so a hung endpoint cannot leave the
//     turn pending forever;
//   - the disabled-Send busy affordance during an in-flight turn.
//
// Mirrors the assistant_storage_guard_test.go approach (read the committed
// source, assert a contract) and ships with an adversarial twin proving the
// guard detects a regression.
package webcodegen_drift_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/testsupport/jssource"
)

// requiredRobustnessPatterns enumerates the client-side robustness wiring
// that MUST be present in web/pwa/assistant.js. Each entry carries a minimum
// match count so a single decorative mention cannot satisfy a multi-site
// contract (e.g. the inFlight early-return guards both dispatchTurn and the
// submit handler).
var requiredRobustnessPatterns = []struct {
	name    string
	re      *regexp.Regexp
	minHits int
}{
	{"AbortController construction", regexp.MustCompile(`new\s+AbortController\s*\(`), 1},
	{"abort on timeout", regexp.MustCompile(`setTimeout\([^;]*\.abort\s*\(\s*\)`), 1},
	{"fetch carries the abort signal", regexp.MustCompile(`signal:\s*controller\.signal`), 1},
	{"single-flight inFlight guard", regexp.MustCompile(`if\s*\(\s*inFlight\s*\)`), 2},
	{"disabled Send busy affordance", regexp.MustCompile(`\.disabled\s*=\s*busy`), 1},
	{"request timeout budget constant", regexp.MustCompile(`TURN_TIMEOUT_MS`), 2},
}

func readAssistantJS(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	p := filepath.Join(root, "web", "pwa", "assistant.js")
	body, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	// Ignore comments so wiring is matched in real code, not documentation.
	return jssource.WithoutComments(string(body))
}

// TestWebAssistantRobustnessGuard_BUG_073_002 fails if any client-side
// robustness mechanism is missing from the committed assistant.js source.
func TestWebAssistantRobustnessGuard_BUG_073_002(t *testing.T) {
	src := readAssistantJS(t)
	for _, want := range requiredRobustnessPatterns {
		got := len(want.re.FindAllStringIndex(src, -1))
		if got < want.minHits {
			t.Errorf("assistant.js is missing the %q robustness wiring: pattern %s matched %d time(s), want >= %d",
				want.name, want.re.String(), got, want.minHits)
		}
	}
}

// TestWebAssistantRobustnessGuard_Adversarial_BUG_073_002 proves the guard
// detects a regression: a synthetic source with each mechanism stripped out
// must fail every required pattern's minimum-hit contract.
func TestWebAssistantRobustnessGuard_Adversarial_BUG_073_002(t *testing.T) {
	// A "regressed" assistant client: no AbortController, no signal, no
	// inFlight guard, no busy toggle, no timeout constant — exactly the
	// pre-fix shape this guard exists to forbid.
	regressed := `
		async function postTurn(requestBody) {
			const resp = await fetch(ENDPOINT, {
				method: "POST",
				credentials: "same-origin",
				body: JSON.stringify(requestBody),
			});
			return JSON.parse(await resp.text());
		}
		async function dispatchTurn(requestBody) {
			pendingTurn = requestBody;
			const response = await postTurn(requestBody);
			renderResponse(response);
		}
		form.addEventListener("submit", (ev) => {
			ev.preventDefault();
			submitText(String(input.value).trim());
		});
	`
	var stillPass []string
	for _, want := range requiredRobustnessPatterns {
		if len(want.re.FindAllStringIndex(regressed, -1)) >= want.minHits {
			stillPass = append(stillPass, want.name)
		}
	}
	if len(stillPass) > 0 {
		t.Fatalf("robustness guard would NOT detect a regression — these patterns still satisfied on the regressed sample: %s",
			strings.Join(stillPass, ", "))
	}
}
