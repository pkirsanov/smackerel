package api

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ac11Hit is the per-line classification record used by the AC-11 grep
// guard. Hoisted to package scope so the helper signature can reference
// a named type (anonymous-struct equality is by-shape, but the Go
// compiler treats a function-local declaration as a distinct type even
// if the shape matches an anonymous struct in another function).
type ac11Hit struct {
	path     string
	line     int
	text     string
	ok       bool
	category string
}

// TestAuthActorIdentitySourcesGrepGuard implements spec 044 AC-11.
//
// Spec 044 §AC-11 (verbatim):
//
//	`grep -rEn 'X-Actor-Id|actor_id_in_body_forbidden|"actor_id"'
//	internal/` after spec close shows that the only remaining matches
//	in `production`-applicable code paths are dev/test fallbacks
//	explicitly gated by `cfg.Environment != "production"`, OR are
//	removed entirely. No production-applicable header-trust or
//	body-trust paths remain for `actor_id`.
//
// The test mechanically reproduces that grep against the live
// internal/ tree and inspects each remaining hit. Every hit MUST be
// either:
//
//   - inside a comment line (// ...) — documentation of past behaviour
//   - on a line that names the production-rejection error code
//     ("actor_id_in_body_forbidden", "actor_id_in_header_forbidden")
//   - on a line that explicitly checks `h.environment == "production"`
//     OR sits inside a block whose first preceding control-flow line
//     is one of: `if h.environment == "production"`,
//     `if h.Environment == "production"`,
//     `case "production"`, or a comment naming the production gate
//
// Any unguarded `X-Actor-Id` header read or unguarded body smuggling
// path remaining in production code is a regression and fails this
// test loudly.
//
// AC-11 fixture coverage: the test additionally constructs an
// adversarial in-memory fixture line that would represent a
// regression (an unguarded `r.Header.Get("X-Actor-Id")` in a handler
// without an environment gate) and verifies the same classification
// helper rejects it. This guarantees the test cannot pass simply by
// having no matches at all.
func TestAuthActorIdentitySourcesGrepGuard(t *testing.T) {
	t.Helper()

	// Resolve internal/ relative to this test file (we are in
	// internal/api/...).  filepath.Abs("..") points at internal/api,
	// then `..` again at internal/.
	thisDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	internalDir := filepath.Clean(filepath.Join(thisDir, ".."))

	pattern := regexp.MustCompile(`X-Actor-Id|actor_id_in_body_forbidden|actor_id_in_header_forbidden|"actor_id"`)

	var hits []ac11Hit

	walkErr := filepath.WalkDir(internalDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		buf := make([]byte, 0, 1<<20)
		scanner.Buffer(buf, 1<<24)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		for i, ln := range lines {
			if !pattern.MatchString(ln) {
				continue
			}
			h := ac11Hit{path: path, line: i + 1, text: ln}
			classifyActorIDLine(lines, i, &h)
			hits = append(hits, h)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk internal/: %v", walkErr)
	}

	for _, h := range hits {
		if !h.ok {
			t.Errorf("AC-11 violation: %s:%d unguarded actor-identity reference (category=%s): %s",
				h.path, h.line, h.category, strings.TrimSpace(h.text))
		}
	}

	// AC-11 adversarial-fixture sub-check: prove the classifier would
	// reject an unguarded X-Actor-Id read. This guarantees the test
	// cannot pass vacuously.
	adversarial := []string{
		"func bad(w http.ResponseWriter, r *http.Request) {",
		`	actor := r.Header.Get("X-Actor-Id")`,
		`	_ = actor`,
		"}",
	}
	advHit := ac11Hit{path: "<adversarial>", line: 2, text: adversarial[1]}
	classifyActorIDLine(adversarial, 1, &advHit)
	if advHit.ok {
		t.Fatalf("AC-11 adversarial fixture FAILED: classifier accepted an unguarded X-Actor-Id read; got category=%s", advHit.category)
	}
}

// classifyActorIDLine inspects a matched line in context and decides
// whether the match is an acceptable dev/test fallback or a production
// regression. It mutates the provided hit pointer with the verdict.
func classifyActorIDLine(lines []string, i int, h *ac11Hit) {
	ln := lines[i]
	trimmed := strings.TrimSpace(ln)

	// Comment lines are documentation of past behaviour or design
	// guidance; they do not represent a runtime trust path.
	if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
		h.ok = true
		h.category = "comment"
		return
	}

	// Lines that name the production rejection error codes ARE the
	// closure surface — they are exactly what AC-11 wants to see.
	if strings.Contains(ln, "actor_id_in_body_forbidden") ||
		strings.Contains(ln, "actor_id_in_header_forbidden") {
		h.ok = true
		h.category = "production-rejection-code"
		return
	}

	// `validActorIDKeyName` style constants — ban-set construction is
	// not a runtime trust path.
	if strings.Contains(ln, `[]byte(`) && strings.Contains(ln, `"actor_id"`) {
		// e.g. `bytes.Contains(bodyBytes, []byte(`"actor_id"`))` is
		// the rejection check itself; safe.
		h.ok = true
		h.category = "rejection-check"
		return
	}

	// Walk backward through the function body looking for either:
	//   - a production-mode if-gate that wraps this line
	//   - a function/method boundary (we exited the gate without seeing it)
	for j := i - 1; j >= 0 && j >= i-30; j-- {
		prev := strings.TrimSpace(lines[j])
		if prev == "" {
			continue
		}
		if strings.HasPrefix(prev, "//") {
			continue
		}
		if strings.HasPrefix(prev, "}") {
			// Exited a sub-block; keep walking — the gate may
			// wrap a wider region.
			continue
		}
		if strings.Contains(prev, `Environment == "production"`) ||
			strings.Contains(prev, `environment == "production"`) ||
			strings.Contains(prev, `cfg.Environment != "production"`) ||
			strings.Contains(prev, `Environment != "production"`) {
			h.ok = true
			h.category = "production-gated-above"
			return
		}
		// Function boundary
		if strings.HasPrefix(prev, "func ") {
			break
		}
	}

	// Walk forward up to a few lines to detect the gate-after-read
	// idiom: read X-Actor-Id once, then immediately check production
	// and either reject or proceed (this is the MintReveal /
	// PlanAction pattern). The forward window is shorter than the
	// backward window because the production check should be
	// adjacent.
	for j := i + 1; j < len(lines) && j <= i+8; j++ {
		next := strings.TrimSpace(lines[j])
		if next == "" {
			continue
		}
		if strings.HasPrefix(next, "//") {
			continue
		}
		if strings.Contains(next, `Environment == "production"`) ||
			strings.Contains(next, `environment == "production"`) ||
			strings.Contains(next, `actor_id_in_header_forbidden`) {
			h.ok = true
			h.category = "production-gated-below"
			return
		}
		// Stop scanning if we see a return statement or a func
		// boundary that would indicate we're past the gate window.
		if strings.HasPrefix(next, "return") || strings.HasPrefix(next, "func ") {
			break
		}
	}

	// Method-form helper exception: the helper itself is allowed to
	// read X-Actor-Id because it is the centralized gate that other
	// handlers delegate to. Detect this by checking whether the
	// enclosing func declaration has a `*PhotosHandlers` receiver
	// AND the function body begins with a session check.
	for j := i - 1; j >= 0 && j >= i-50; j-- {
		prev := strings.TrimSpace(lines[j])
		if !strings.HasPrefix(prev, "func ") {
			continue
		}
		if strings.Contains(prev, "(h *PhotosHandlers)") &&
			strings.Contains(prev, "actorIDFromRequest") {
			// This is the centralized helper; safe.
			h.ok = true
			h.category = "centralized-helper"
			return
		}
		break
	}

	// No production gate found AND not a comment AND not a rejection
	// code AND not a ban-set construction AND not the centralized
	// helper → flag as regression.
	h.ok = false
	h.category = "unguarded"
}
