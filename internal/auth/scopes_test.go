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
