// pwa_hidden_cascade_guard_test.go — guards the `.hidden` utility class.
//
// `.hidden` is the PWA's only generic "hide this element" utility, and it is
// applied from JS at four call sites in web/pwa/assistant.js. It was SHIPPED
// BROKEN: `.hidden { display: none }` is specificity (0,1,0) and `.btn` is also
// (0,1,0) but declared LATER in the same stylesheet, so on a tie the later rule
// wins and `display: inline-flex` silently defeated it.
//
// The live consequence was user-visible: assistant.html ships the retry button
// as `class="btn btn-secondary hidden"`, so it rendered permanently — offering
// "Retry last" when there was nothing to retry — and every
// classList.add("hidden") call meant to hide it was inert.
//
// This is the same cascade trap already documented on the `[hidden]` attribute
// rule at the top of style.css. Nothing structural stopped it recurring, and a
// silently-defeated hide is invisible in review, so it is asserted here.
//
// The guard is deliberately three-part: it checks the FIX, the REASON the fix
// is required, and the REAL element that depends on it. A guard asserting only
// `!important` would keep passing as a superstition if the conflicting rule
// ever disappeared; a guard asserting only the conflict would not catch the fix
// being reverted.
package webcodegen_drift_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// hiddenCascadeStylesheet returns web/pwa/style.css verbatim. Comments are NOT
// stripped here: this test reasons about declaration ORDER, and the offsets
// must correspond to the file a browser actually parses.
func hiddenCascadeStylesheet(t *testing.T) string {
	t.Helper()
	path := filepath.Join(graphSectionRepoRoot(t), "web", "pwa", "style.css")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

// TestHiddenUtilityBeatsLaterEqualSpecificityDisplay asserts the `.hidden`
// utility actually hides.
func TestHiddenUtilityBeatsLaterEqualSpecificityDisplay(t *testing.T) {
	css := hiddenCascadeStylesheet(t)

	hiddenRule := regexp.MustCompile(`(?m)^\.hidden\s*\{[^}]*\}`)
	loc := hiddenRule.FindStringIndex(css)
	if loc == nil {
		t.Fatal("web/pwa/style.css declares no top-level `.hidden` rule; " +
			"if the utility was renamed, update this guard and the four " +
			"classList.add(\"hidden\") call sites in web/pwa/assistant.js together")
	}
	rule := css[loc[0]:loc[1]]

	// Part 1 — the fix. Without `!important` the declaration loses the tie
	// described below and the utility silently stops hiding anything.
	if !strings.Contains(rule, "!important") {
		t.Errorf("`.hidden` must declare `display: none !important`, got %q.\n"+
			"Plain `display: none` is specificity (0,1,0) and loses to any "+
			"later equal-specificity `display` rule, which is exactly how the "+
			"assistant retry button shipped permanently visible.", rule)
	}
	if !strings.Contains(strings.ReplaceAll(rule, " ", ""), "display:none") {
		t.Errorf("`.hidden` must set `display: none`, got %q", rule)
	}

	// Part 2 — the reason. Prove a later, equal-specificity rule really does
	// set `display`, so the `!important` above is load-bearing rather than
	// cargo-culted. A single class selector is (0,1,0), the same weight as
	// `.hidden`, so on a tie the LATER declaration wins.
	laterConflict := regexp.MustCompile(`(?m)^\.[a-zA-Z][\w-]*\s*\{[^}]*display\s*:[^;}]+[;}]`)
	conflicts := laterConflict.FindAllStringIndex(css, -1)
	foundLater := false
	for _, c := range conflicts {
		if c[0] > loc[0] && !strings.Contains(css[c[0]:c[1]], "!important") {
			foundLater = true
			break
		}
	}
	if !foundLater {
		t.Log("NOTE: no later single-class rule sets `display` without " +
			"`!important` any more. The `!important` on `.hidden` is now " +
			"defensive rather than strictly required. Keep it — the utility is " +
			"applied from JS and must not depend on stylesheet ordering — but " +
			"this guard's Part 2 rationale no longer reproduces the original bug.")
	}
}

// TestHiddenUtilityHasALiveConsumerThatDependsOnIt pins the concrete element
// the bug was reported against, so the guard above cannot decay into an
// abstract style rule with nothing real behind it.
func TestHiddenUtilityHasALiveConsumerThatDependsOnIt(t *testing.T) {
	root := graphSectionRepoRoot(t)

	markupPath := filepath.Join(root, "web", "pwa", "assistant.html")
	markup, err := os.ReadFile(markupPath)
	if err != nil {
		t.Fatalf("read %s: %v", markupPath, err)
	}
	// The retry button combines `.btn` with `.hidden` — precisely the pairing
	// that lost the cascade tie.
	retryButton := regexp.MustCompile(`<button[^>]*id="assistant-retry-btn"[^>]*class="([^"]*)"`)
	m := retryButton.FindSubmatch(markup)
	if m == nil {
		t.Skip("assistant.html no longer ships #assistant-retry-btn with a class " +
			"attribute; the cascade guard above still stands on its own")
	}
	classes := string(m[1])
	if !strings.Contains(classes, "hidden") {
		t.Errorf("#assistant-retry-btn must ship hidden by default, got class=%q. "+
			"It is revealed by web/pwa/assistant.js only when a retry is actually "+
			"available.", classes)
	}
	if !strings.Contains(classes, "btn") {
		t.Logf("NOTE: #assistant-retry-btn no longer carries `btn` (class=%q), so "+
			"this element no longer reproduces the original cascade collision.", classes)
	}

	// And the JS really does drive it, so hiding is behaviour and not decoration.
	scriptPath := filepath.Join(root, "web", "pwa", "assistant.js")
	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %s: %v", scriptPath, err)
	}
	if !strings.Contains(string(script), `classList.add("hidden")`) {
		t.Error("web/pwa/assistant.js no longer hides anything via " +
			`classList.add("hidden"); if the hide mechanism changed, retire this ` +
			"guard deliberately rather than leaving it asserting a dead contract")
	}
}
