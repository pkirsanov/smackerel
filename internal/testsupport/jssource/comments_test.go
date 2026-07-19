package jssource

import (
	"strings"
	"testing"
)

func TestWithoutComments_IgnoresPolicyCommentsAndRetainsExecutableAccess(t *testing.T) {
	source := `
// localStorage is forbidden in this client.
/* sessionStorage and indexedDB are forbidden too. */
const token = localStorage.getItem("bearer");
`
	got := WithoutComments(source)
	if strings.Contains(got, "sessionStorage") || strings.Contains(got, "indexedDB") {
		t.Fatalf("comment token survived lexical removal:\n%s", got)
	}
	if !strings.Contains(got, `localStorage.getItem("bearer")`) {
		t.Fatalf("executable storage access was removed:\n%s", got)
	}
}

func TestWithoutComments_PreservesStringsTemplatesAndFollowingCode(t *testing.T) {
	source := `
const url = "https://example.test/path"; // trailing comment
const quoted = '// not a comment';
const template = ` + "`/* also text */ ${localStorage.getItem(\"key\")}`" + `;
/* block
   comment */
const after = sessionStorage.getItem("key");
`
	got := WithoutComments(source)
	for _, required := range []string{
		`"https://example.test/path"`,
		`'// not a comment'`,
		"`/* also text */ ${localStorage.getItem(\"key\")}`",
		`sessionStorage.getItem("key")`,
	} {
		if !strings.Contains(got, required) {
			t.Errorf("preserved source missing %q:\n%s", required, got)
		}
	}
	if len(got) != len(source) {
		t.Fatalf("source length changed: got=%d want=%d", len(got), len(source))
	}
}
