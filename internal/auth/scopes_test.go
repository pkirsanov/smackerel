// Spec 060 scope 1 — registry, regex, surface extraction.
package auth

import (
	"slices"
	"testing"
)

func TestValidateScopeName(t *testing.T) {
	good := []string{
		"extension:bookmarks,history",
		"extension:bookmarks",
		"admin:users",
		"a:b",
	}
	for _, s := range good {
		if err := ValidateScopeName(s); err != nil {
			t.Errorf("ValidateScopeName(%q) unexpected err: %v", s, err)
		}
	}
	bad := []string{
		"",
		"BadlyFormatted",
		"Extension:bookmarks",
		":bookmarks",
		"extension:",
		"extension:BOOKMARKS",
		"1bad:scope",
	}
	for _, s := range bad {
		if err := ValidateScopeName(s); err == nil {
			t.Errorf("ValidateScopeName(%q) expected error", s)
		}
	}
}

func TestRegisteredScopeSurfaces_ContainsExtension(t *testing.T) {
	if !slices.Contains(RegisteredScopeSurfaces, "extension") {
		t.Fatalf("RegisteredScopeSurfaces missing 'extension': %v", RegisteredScopeSurfaces)
	}
	if !IsRegisteredScopeSurface("extension") {
		t.Errorf("IsRegisteredScopeSurface('extension') = false")
	}
	if IsRegisteredScopeSurface("future-surface") {
		t.Errorf("IsRegisteredScopeSurface('future-surface') = true; expected false")
	}
}

// Spec 027 scope 9 PLAN-9-03 — the `annotation` scope surface must be
// registered so spec 060's RequireScope middleware accepts the
// `annotation:edit` and `annotation:read` claims used by the spec 073
// graph-browse UI.
func TestRegisteredScopeSurfaces_ContainsAnnotation(t *testing.T) {
	if !slices.Contains(RegisteredScopeSurfaces, "annotation") {
		t.Fatalf("RegisteredScopeSurfaces missing 'annotation' (spec 027 scope 9): %v", RegisteredScopeSurfaces)
	}
	if !IsRegisteredScopeSurface("annotation") {
		t.Errorf("IsRegisteredScopeSurface('annotation') = false; expected true")
	}
	for _, scope := range []string{"annotation:edit", "annotation:read"} {
		if err := ValidateScopeName(scope); err != nil {
			t.Errorf("ValidateScopeName(%q) unexpected err: %v", scope, err)
		}
	}
}

// Spec 080 SCOPE-080-01 — the `knowledge-graph` scope surface must be
// registered so the spec 060 RequireScope middleware accepts the
// `knowledge-graph:read` claim that gates the 8 Knowledge Graph
// Public API endpoints (SCN-080-09 / SCN-080-10).
func TestRegisteredScopeSurfaces_ContainsKnowledgeGraph(t *testing.T) {
	if !slices.Contains(RegisteredScopeSurfaces, "knowledge-graph") {
		t.Fatalf("RegisteredScopeSurfaces missing 'knowledge-graph' (spec 080 SCOPE-080-01): %v", RegisteredScopeSurfaces)
	}
	if !IsRegisteredScopeSurface("knowledge-graph") {
		t.Errorf("IsRegisteredScopeSurface('knowledge-graph') = false; expected true")
	}
	if err := ValidateScopeName("knowledge-graph:read"); err != nil {
		t.Errorf("ValidateScopeName(%q) unexpected err: %v", "knowledge-graph:read", err)
	}
}

func TestExtractScopeSurface(t *testing.T) {
	cases := map[string]string{
		"extension:bookmarks,history": "extension",
		"admin:users":                 "admin",
		"a:b":                         "a",
		"no-colon":                    "",
	}
	for in, want := range cases {
		if got := ExtractScopeSurface(in); got != want {
			t.Errorf("ExtractScopeSurface(%q) = %q; want %q", in, got, want)
		}
	}
}
