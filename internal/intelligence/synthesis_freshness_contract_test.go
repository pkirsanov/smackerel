package intelligence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSynthesisFreshness_HasOneSSTAndNo48hDigestAlias(t *testing.T) {
	root := synthesisFreshnessRepoRoot(t)

	sst := readSynthesisFreshnessContractFile(t, root, "config/smackerel.yaml")
	for _, key := range []string{"daily_freshness_seconds:", "weekly_freshness_seconds:"} {
		if count := strings.Count(sst, key); count != 1 {
			t.Errorf("config/smackerel.yaml contains %q %d times, want exactly one SST declaration", key, count)
		}
	}

	generator := readSynthesisFreshnessContractFile(t, root, "scripts/commands/config.sh")
	for _, required := range []string{
		"intelligence.synthesis.daily_freshness_seconds",
		"intelligence.synthesis.weekly_freshness_seconds",
		"SYNTHESIS_DAILY_FRESHNESS_SECONDS",
		"SYNTHESIS_WEEKLY_FRESHNESS_SECONDS",
	} {
		if !strings.Contains(generator, required) {
			t.Errorf("config generator does not carry required synthesis freshness token %q", required)
		}
	}

	health := readSynthesisFreshnessContractFile(t, root, "internal/api/health.go")
	for _, forbidden := range []string{"intelligenceSynthesisFreshnessBudget", "48 * time.Hour"} {
		if strings.Contains(health, forbidden) {
			t.Errorf("internal/api/health.go retains forbidden synthesis freshness fallback %q", forbidden)
		}
	}

	wiring := readSynthesisFreshnessContractFile(t, root, "cmd/core/wiring.go")
	if count := strings.Count(wiring, "DigestStaleAfterHours"); count != 1 {
		t.Errorf("cmd/core/wiring.go has %d DigestStaleAfterHours consumers, want only the digest consumer", count)
	}
	if strings.Contains(wiring, "SynthesisFreshnessBudget") {
		t.Error("cmd/core/wiring.go retains a cadence-blind synthesis freshness budget")
	}

	for _, relativePath := range []string{
		"internal/api/synthesis.go",
		"internal/intelligence/synthesis_readmodel.go",
		"internal/web/synthesis_page.go",
	} {
		production := readSynthesisFreshnessContractFile(t, root, relativePath)
		for _, forbidden := range []string{"LatestOutcome(", ".Latest("} {
			if strings.Contains(production, forbidden) {
				t.Errorf("%s retains unscoped production read %q", relativePath, forbidden)
			}
		}
	}
}

func synthesisFreshnessRepoRoot(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for current := workingDirectory; ; current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, "config", "smackerel.yaml")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatalf("locate repository root from %s", workingDirectory)
		}
	}
}

func readSynthesisFreshnessContractFile(t *testing.T, root string, relativePath string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relativePath)))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return string(contents)
}