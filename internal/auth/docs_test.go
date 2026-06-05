// Spec 060 SCN-060-019 + SCN-060-020 — doc-presence guards.
//
// These two tests grep the operator-facing docs to ensure the spec-060
// content (the Scoped Token Enrollment subsection in Operations.md and
// the 403 scope_required subsection in API.md) does not regress to
// missing-section status. They are intentionally cheap grep-style tests
// because the doc content itself is validated by human review; the
// only invariant these tests enforce is that the SECTIONS still exist
// at the expected anchors with the expected key strings.
//
// Spec 060 originally planned these tests but they never shipped during
// implementation (verified via specs/060-bearer-auth-scope-claim/state.json
// 2026-06-03 audit). This commit closes that gap so SCN-060-019 and
// SCN-060-020 have real Go-side regression coverage.
package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// docsRepoRoot returns the repo root from this test's location.
// internal/auth/ -> repo root is 2 parents up.
func docsRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed — cannot locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func readDoc(t *testing.T, relPath string) string {
	t.Helper()
	full := filepath.Join(docsRepoRoot(t), relPath)
	b, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read %s: %v", full, err)
	}
	return string(b)
}

// TestOperationsDoc_HasScopedTokenEnrollmentSubsection — SCN-060-019.
// Verifies docs/Operations.md contains the Scoped Token Enrollment
// subsection plus the key command examples and rotation modes that
// spec 060 promised. A regression that silently drops the section
// would be a documentation gap that this test catches.
func TestOperationsDoc_HasScopedTokenEnrollmentSubsection(t *testing.T) {
	doc := readDoc(t, "docs/Operations.md")

	// Required section header.
	if !strings.Contains(doc, "### Scoped Token Enrollment (Spec 060)") {
		t.Fatalf("ADVERSARIAL FAILURE: docs/Operations.md missing required subsection header '### Scoped Token Enrollment (Spec 060)' — SCN-060-019 regression")
	}

	// Required content markers documented by spec 060: --scope flag,
	// --allow-unknown-surface escape-hatch, three rotation modes, the
	// scope-rejected metric, and the initial RequireScope endpoint
	// wiring matrix reference.
	required := []string{
		"--scope",
		"--allow-unknown-surface",
		"auth_scope_rejected_total",
	}
	for _, want := range required {
		if !strings.Contains(doc, want) {
			t.Errorf("docs/Operations.md missing required substring %q in Scoped Token Enrollment section — SCN-060-019 regression", want)
		}
	}
}

// TestApiDoc_HasScopeRequiredResponseShape — SCN-060-020.
// Verifies docs/API.md contains the 403 scope_required subsection plus
// the response body shape and the wiring matrix headers that spec 060
// promised. A regression here would mean clients lose the contract
// reference for the 403 response.
func TestApiDoc_HasScopeRequiredResponseShape(t *testing.T) {
	doc := readDoc(t, "docs/API.md")

	if !strings.Contains(doc, "### 403 scope_required (Spec 060)") {
		t.Fatalf("ADVERSARIAL FAILURE: docs/API.md missing required subsection header '### 403 scope_required (Spec 060)' — SCN-060-020 regression")
	}

	// Required content: the literal scope_required error code and the
	// canonical body shape with the required[] array.
	required := []string{
		"scope_required",
		`"required"`,
	}
	for _, want := range required {
		if !strings.Contains(doc, want) {
			t.Errorf("docs/API.md missing required substring %q in 403 scope_required section — SCN-060-020 regression", want)
		}
	}
}
