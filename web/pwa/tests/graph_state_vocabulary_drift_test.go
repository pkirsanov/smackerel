// graph_state_vocabulary_drift_test.go — BUG-080-001 SCOPE-04.
//
// web/pwa/wiki_state.js is the ONE closed activation/read model the
// Knowledge Graph surfaces consume. Because it is hand-written (the
// assistant-turn codegen pipeline does not cover the graph wire shapes),
// nothing structural stops its vocabulary from drifting away from the Go
// constants it mirrors — and a UI that silently disagrees with the
// server about what "disabled" or "store unavailable" means is exactly
// the class of defect this bug exists to remove.
//
// This test makes that drift impossible to merge. It takes the Go
// constants as COMPILE-TIME truth (importing the real packages rather
// than scraping their source, so renaming a constant breaks the build
// here) and asserts the JS mirrors them exactly in both directions:
// every Go code is declared in JS, and JS declares no code that Go does
// not define.
package webcodegen_drift_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/graphsynthetic"
	"github.com/smackerel/smackerel/internal/testsupport/jssource"
)

// wikiStateSource returns web/pwa/wiki_state.js with comments stripped,
// so a closed code merely NAMED in a policy comment cannot satisfy the
// "is declared" check.
func wikiStateSource(t *testing.T) string {
	t.Helper()
	path := filepath.Join(graphSectionRepoRoot(t), "web", "pwa", "wiki_state.js")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(raw) == 0 {
		t.Fatalf("%s is empty; the closed UI state model must exist", path)
	}
	return jssource.WithoutComments(string(raw))
}

// goClosedCodes is the authoritative closed diagnostic vocabulary, taken
// from the Go constants themselves. A constant renamed or removed in Go
// fails to compile here, which is the point.
func goClosedCodes() []string {
	return []string{
		graphsynthetic.CodeOK,
		graphsynthetic.CodeEmptyPermitted,
		graphsynthetic.CodeEmptyNotPermitted,
		graphsynthetic.CodeUnauthenticated,
		graphsynthetic.CodeForbidden,
		graphsynthetic.CodeRouteAbsent,
		graphsynthetic.CodeCapabilityDisabled,
		graphsynthetic.CodeStoreUnavailable,
		graphsynthetic.CodeServerError,
		graphsynthetic.CodeSchemaInvalid,
		graphsynthetic.CodeCursorInvalid,
		graphsynthetic.CodeRowMissing,
		graphsynthetic.CodeTransport,
		graphsynthetic.CodeUnexpectedStatus,
		graphsynthetic.CodePolicyDisabled,
		graphsynthetic.CodeFamilyMissing,
		graphsynthetic.CodeOptionalOmitted,
	}
}

func goAggregateStates() []string {
	return []string{
		string(graphsynthetic.AggregateAvailable),
		string(graphsynthetic.AggregateDegraded),
		string(graphsynthetic.AggregateUnavailable),
		string(graphsynthetic.AggregatePolicyDisabled),
	}
}

// goReadinessCodes are the projection-level codes published in the same
// `graph` health section, so the UI may legitimately recognise them.
func goReadinessCodes() []string {
	return []string{
		api.GraphReadinessCodeNotObserved,
		api.GraphReadinessCodeStale,
		api.GraphReadinessCodeActivationMismatch,
		api.GraphReadinessCodeConfigInvalid,
	}
}

func TestWikiStateJSMirrorsGoClosedVocabulary(t *testing.T) {
	source := wikiStateSource(t)

	t.Run("every Go diagnostic code is declared in the UI model", func(t *testing.T) {
		for _, code := range goClosedCodes() {
			if !strings.Contains(source, `"`+code+`"`) {
				t.Errorf("closed code %q is defined in Go but absent from web/pwa/wiki_state.js; "+
					"the UI cannot resolve a state the server can publish", code)
			}
		}
	})

	t.Run("every Go aggregate state is declared in the UI model", func(t *testing.T) {
		for _, state := range goAggregateStates() {
			if !strings.Contains(source, `"`+state+`"`) {
				t.Errorf("aggregate state %q is defined in Go but absent from web/pwa/wiki_state.js", state)
			}
		}
	})

	// Reverse direction: the UI must not invent a diagnostic code. A
	// fabricated F080 code would render copy for a state the server can
	// never actually publish, which is a fabricated claim in the UI.
	t.Run("the UI model invents no diagnostic code Go does not define", func(t *testing.T) {
		known := map[string]bool{}
		for _, code := range goClosedCodes() {
			known[code] = true
		}
		// Readiness codes are published in the same `graph` health section
		// and are legitimate for the UI to recognise.
		for _, code := range goReadinessCodes() {
			known[code] = true
		}
		found := regexp.MustCompile(`"(F080-[A-Z0-9-]+)"`).FindAllStringSubmatch(source, -1)
		var unknown []string
		for _, m := range found {
			if !known[m[1]] {
				unknown = append(unknown, m[1])
			}
		}
		sort.Strings(unknown)
		if len(unknown) > 0 {
			t.Errorf("web/pwa/wiki_state.js declares %d code(s) with no Go definition: %v; "+
				"the UI must not name a state the server cannot publish", len(unknown), unknown)
		}
	})

	// Anti-vacuity: if the extraction found nothing at all, the two
	// directional checks above are trivially satisfiable and this test
	// would pass against an empty vocabulary.
	t.Run("the scan actually observed the vocabulary", func(t *testing.T) {
		found := regexp.MustCompile(`"(F080-[A-Z0-9-]+)"`).FindAllString(source, -1)
		if len(found) < len(goClosedCodes())-1 {
			t.Fatalf("only %d F080 literals were extracted from wiki_state.js but Go defines %d closed codes; "+
				"the drift scan is not reading the model it claims to check",
				len(found), len(goClosedCodes()))
		}
	})
}

// TestWikiStateJSClassifierMatchesSyntheticStatusMapping pins the JS
// classifier to the server's own status mapping. The UI performs its own
// reads, so it necessarily classifies HTTP outcomes itself; what must
// never happen is the two classifying the SAME response differently.
//
// The mapping is asserted structurally: each status the Go classifier
// switches on must appear in the JS classifier alongside the code Go
// returns for it. A 503 that Go disambiguates by typed envelope code but
// JS collapses to a single state is precisely the regression that made
// "deliberately off" indistinguishable from "store is down".
func TestWikiStateJSClassifierMatchesSyntheticStatusMapping(t *testing.T) {
	source := wikiStateSource(t)

	cases := []struct {
		status string
		code   string
		why    string
	}{
		{"401", graphsynthetic.CodeUnauthenticated, "a rejected session must be its own state"},
		{"403", graphsynthetic.CodeForbidden, "a denied scope must not read as a rejected session"},
		{"404", graphsynthetic.CodeRouteAbsent, "an absent route must not read as an empty graph"},
		{"400", graphsynthetic.CodeSchemaInvalid, "a malformed request must not read as a server fault"},
		{"503", graphsynthetic.CodeCapabilityDisabled, "a deliberately disabled capability must be distinguishable"},
		{"503", graphsynthetic.CodeStoreUnavailable, "a downed store must be distinguishable from a disabled capability"},
	}

	classifier := classifierBody(t, source)
	for _, tc := range cases {
		if !strings.Contains(classifier, tc.status) {
			t.Errorf("JS classifier does not handle HTTP %s (%s)", tc.status, tc.why)
		}
		if !strings.Contains(classifier, `"`+tc.code+`"`) && !strings.Contains(classifier, jsConstName(tc.code)) {
			t.Errorf("JS classifier never yields %q for HTTP %s (%s)", tc.code, tc.status, tc.why)
		}
	}
}

// classifierBody isolates the classifyStatus function so the assertions
// above cannot be satisfied by an unrelated mention elsewhere in the file.
func classifierBody(t *testing.T, source string) string {
	t.Helper()
	const marker = "export function classifyStatus("
	start := strings.Index(source, marker)
	if start < 0 {
		t.Fatalf("wiki_state.js does not export classifyStatus; the UI has no single classifier")
	}
	rest := source[start:]
	end := strings.Index(rest, "\n}")
	if end < 0 {
		t.Fatalf("could not delimit classifyStatus body")
	}
	return rest[:end]
}

// jsConstName maps a wire code to the JS constant identifier the model
// uses, so the classifier may reference either form.
func jsConstName(code string) string {
	switch code {
	case graphsynthetic.CodeUnauthenticated:
		return "CODE_UNAUTHENTICATED"
	case graphsynthetic.CodeForbidden:
		return "CODE_FORBIDDEN"
	case graphsynthetic.CodeRouteAbsent:
		return "CODE_ROUTE_ABSENT"
	case graphsynthetic.CodeSchemaInvalid:
		return "CODE_SCHEMA_INVALID"
	case graphsynthetic.CodeCapabilityDisabled:
		return "CODE_CAPABILITY_DISABLED"
	case graphsynthetic.CodeStoreUnavailable:
		return "CODE_STORE_UNAVAILABLE"
	}
	return "\x00never"
}
