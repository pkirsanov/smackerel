package ntfy

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNtfyAdapterHasNoOutputChannelImports(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate current test file")
	}
	packageDir := filepath.Dir(currentFile)
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		t.Fatalf("read ntfy package dir: %v", err)
	}
	forbiddenTokens := []string{"internal/telegram", "outputdispatcher", "deliveryattempt", "telegram."}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(packageDir, name))
		if err != nil {
			t.Fatalf("read ntfy production file %s: %v", name, err)
		}
		body := strings.ToLower(string(contents))
		for _, forbidden := range forbiddenTokens {
			if strings.Contains(body, forbidden) {
				t.Fatalf("ntfy production file %s contains output-channel coupling token %q", name, forbidden)
			}
		}
	}
}
