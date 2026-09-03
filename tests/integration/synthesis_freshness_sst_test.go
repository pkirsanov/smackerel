//go:build integration

package integration

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/intelligence"
)

func TestSynthesisFreshness_DistinctDailyWeeklyValuesReachAllRuntimeConsumers(t *testing.T) {
	root := synthesisFreshnessIntegrationRepoRoot(t)
	generated := readGeneratedSynthesisFreshness(t, filepath.Join(root, "config", "generated", "test.env"))

	dailySeconds := requiredGeneratedPositiveSeconds(t, generated, "SYNTHESIS_DAILY_FRESHNESS_SECONDS")
	weeklySeconds := requiredGeneratedPositiveSeconds(t, generated, "SYNTHESIS_WEEKLY_FRESHNESS_SECONDS")
	if dailySeconds == weeklySeconds {
		t.Fatalf("generated daily and weekly freshness values must be deliberately distinct, both=%d", dailySeconds)
	}
	if dailySeconds >= weeklySeconds {
		t.Fatalf("daily freshness=%d must be shorter than weekly freshness=%d", dailySeconds, weeklySeconds)
	}

	t.Setenv("SYNTHESIS_DAILY_FRESHNESS_SECONDS", strconv.FormatInt(dailySeconds, 10))
	t.Setenv("SYNTHESIS_WEEKLY_FRESHNESS_SECONDS", strconv.FormatInt(weeklySeconds, 10))
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load generated synthesis freshness configuration: %v", err)
	}

	policy, err := intelligence.NewSynthesisFreshnessPolicy(
		cfg.Synthesis.DailyFreshness,
		cfg.Synthesis.WeeklyFreshness,
	)
	if err != nil {
		t.Fatalf("construct production cadence freshness policy: %v", err)
	}
	assertSynthesisCadenceBudget(t, policy, intelligence.CadenceDaily, time.Duration(dailySeconds)*time.Second)
	assertSynthesisCadenceBudget(t, policy, intelligence.CadenceWeekly, time.Duration(weeklySeconds)*time.Second)
	for _, test := range []struct {
		name   string
		daily  time.Duration
		weekly time.Duration
	}{
		{name: "zero daily", daily: 0, weekly: time.Second},
		{name: "negative daily", daily: -time.Second, weekly: time.Second},
		{name: "zero weekly", daily: time.Second, weekly: 0},
		{name: "negative weekly", daily: time.Second, weekly: -time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := intelligence.NewSynthesisFreshnessPolicy(test.daily, test.weekly); err == nil {
				t.Fatal("non-positive synthesis freshness must be rejected")
			}
		})
	}
	if _, err := policy.BudgetFor(intelligence.SynthesisCadence("monthly")); err == nil {
		t.Fatal("unknown synthesis cadence must be rejected")
	}

	consumerContracts := map[string][]string{
		"cmd/core/wiring.go": {
			"NewSynthesisFreshnessPolicy",
			"SynthesisFreshnessPolicy",
		},
		"internal/api/health.go": {
			"SynthesisFreshnessPolicy",
			"BudgetFor",
		},
		"internal/api/synthesis.go": {
			"SynthesisFreshnessPolicy",
			"BudgetFor",
		},
		"internal/web/handler.go": {
			"SynthesisFreshnessPolicy",
		},
		"internal/web/synthesis_page.go": {
			"BudgetFor",
		},
		"config/prometheus/alerts.yml": {
			"smackerel_synthesis_state == 2",
			"SYNTHESIS_DAILY_FRESHNESS_SECONDS",
			"SYNTHESIS_WEEKLY_FRESHNESS_SECONDS",
			"owns no independent threshold",
		},
	}
	for relativePath, requiredTokens := range consumerContracts {
		contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatalf("read runtime consumer %s: %v", relativePath, err)
		}
		for _, requiredToken := range requiredTokens {
			if !strings.Contains(string(contents), requiredToken) {
				t.Errorf("runtime consumer %s does not use shared cadence freshness contract %q", relativePath, requiredToken)
			}
		}
	}
}

func assertSynthesisCadenceBudget(
	t *testing.T,
	policy intelligence.SynthesisFreshnessPolicy,
	cadence intelligence.SynthesisCadence,
	want time.Duration,
) {
	t.Helper()
	got, err := policy.BudgetFor(cadence)
	if err != nil {
		t.Fatalf("resolve %s synthesis freshness budget: %v", cadence, err)
	}
	if got != want {
		t.Fatalf("%s freshness budget=%s, want generated %s", cadence, got, want)
	}
}

func readGeneratedSynthesisFreshness(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open generated test environment: %v", err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if found {
			values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan generated test environment: %v", err)
	}
	return values
}

func requiredGeneratedPositiveSeconds(t *testing.T, values map[string]string, key string) int64 {
	t.Helper()
	raw, ok := values[key]
	if !ok || strings.TrimSpace(raw) == "" {
		t.Fatalf("generated test environment is missing required %s", key)
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seconds <= 0 {
		t.Fatalf("generated %s=%q must be a positive integer", key, raw)
	}
	return seconds
}

func synthesisFreshnessIntegrationRepoRoot(t *testing.T) string {
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
