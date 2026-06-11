package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// BUG-064-001 DEFECT A — shipped SST contract.
//
// The open-knowledge agent refuses every query at the per-user-monthly USD
// pre-flight gate when its configured monthly budget is 0 (the production
// CostFn is zero-cost, so a 0 budget means "refuse all"). The shipped
// config/smackerel.yaml therefore MUST set both open-knowledge monthly USD
// budgets to > 0 whenever open_knowledge is enabled.
//
// Adversarial: this test FAILS against the pre-fix config (both budgets 0)
// and PASSES once the budgets are positive. It guards the SST value directly
// so a future reset to 0 (which would re-break /ask) is caught at unit time.

// bug064001RepoRoot climbs from the test CWD to the repo root (the dir that
// contains config/smackerel.yaml).
func bug064001RepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "config", "smackerel.yaml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate repo root (config/smackerel.yaml) from %s", wd)
	return ""
}

func TestShippedConfig_BUG064001_OpenKnowledgeBudgetsAllowOperation(t *testing.T) {
	root := bug064001RepoRoot(t)
	path := filepath.Join(root, "config", "smackerel.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var doc struct {
		Assistant struct {
			OpenKnowledge struct {
				Enabled                 bool    `yaml:"enabled"`
				MonthlyBudgetUSD        float64 `yaml:"monthly_budget_usd"`
				PerUserMonthlyBudgetUSD float64 `yaml:"per_user_monthly_budget_usd"`
			} `yaml:"open_knowledge"`
		} `yaml:"assistant"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal %s: %v", path, err)
	}

	ok := doc.Assistant.OpenKnowledge
	if !ok.Enabled {
		// When the capability is disabled the pre-flight gate never runs,
		// so a 0 budget is harmless. The bug only manifests when enabled.
		t.Skip("assistant.open_knowledge.enabled = false; budget pre-flight gate inactive")
	}
	if ok.PerUserMonthlyBudgetUSD <= 0 {
		t.Fatalf("DEFECT A: assistant.open_knowledge.per_user_monthly_budget_usd = %v; must be > 0 when enabled (a 0 budget makes the agent refuse every /ask via the cap_usd pre-flight gate)", ok.PerUserMonthlyBudgetUSD)
	}
	if ok.MonthlyBudgetUSD <= 0 {
		t.Fatalf("DEFECT A: assistant.open_knowledge.monthly_budget_usd = %v; must be > 0 when enabled", ok.MonthlyBudgetUSD)
	}
}
