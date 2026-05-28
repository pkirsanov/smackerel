package assistant

import (
	"os"
	"path/filepath"
	"testing"
)

// repoRoot walks up from the test working directory until it finds a
// go.mod file (module root).
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 12; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate go.mod walking up from %q", wd)
	return ""
}

// repoFile resolves a repo-relative file path against the discovered
// repo root.
func repoFile(t *testing.T, parts ...string) string {
	t.Helper()
	all := append([]string{repoRoot(t)}, parts...)
	return filepath.Join(all...)
}

// writeTestFile writes `content` to `path` for use in t.TempDir-backed
// fixtures. Fails the test on any I/O error.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}
