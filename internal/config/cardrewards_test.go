// Spec 083 Card Rewards Companion (Scope 01) — tests for the card_rewards
// SST loader. SCN-083-A01 (parse enabled), A03 (fail-loud on missing required),
// A04 (empty sources rejected), A07 (disabled parses without requiring config).
package config

import (
	"strings"
	"testing"
)

func setValidCardRewardsEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CARD_REWARDS_ENABLED", "true")
	t.Setenv("CARD_REWARDS_SCRAPE_CRON", "0 6 * * *")
	t.Setenv("CARD_REWARDS_MONTHLY_RECOMMEND_CRON", "0 7 1 * *")
	t.Setenv("CARD_REWARDS_CALENDAR_SYNC", "false")
	t.Setenv("CARD_REWARDS_CALENDAR_UID_PREFIX", "smackerel-cardrec")
	t.Setenv("CARD_REWARDS_FETCH_TIMEOUT_SECONDS", "20")
	t.Setenv("CARD_REWARDS_EXTRACTION_MODEL", "gpt-oss:20b")
	t.Setenv("CARD_REWARDS_EXTRACTION_ENDPOINT", "http://ollama:11434")
	t.Setenv("CARD_REWARDS_EXTRACTION_CONFIDENCE_THRESHOLD", "0.7")
	t.Setenv("CARD_REWARDS_EXTRACTION_MAX_SOURCES_PER_CARD", "3")
	t.Setenv("CARD_REWARDS_SOURCES", `[{"name":"Doctor of Credit","url":"https://doctorofcredit.com","issuer_hint":"discover"},{"name":"Discover","url":"https://discover.com/cashback","issuer_hint":"discover"}]`)
	t.Setenv("CARD_REWARDS_TRACKED_CATEGORIES", `["Dining","Groceries","Gas"]`)
}

// SCN-083-A01 — card_rewards config populated from env (all fields).
func TestLoadCardRewardsConfig_PopulatesWhenEnabled(t *testing.T) {
	setValidCardRewardsEnv(t)
	cfg, err := LoadCardRewardsConfig()
	if err != nil {
		t.Fatalf("LoadCardRewardsConfig: %v", err)
	}
	if !cfg.Enabled {
		t.Fatalf("Enabled = false, want true")
	}
	if cfg.ScrapeCron != "0 6 * * *" {
		t.Errorf("ScrapeCron = %q, want %q", cfg.ScrapeCron, "0 6 * * *")
	}
	if cfg.MonthlyRecommendCron != "0 7 1 * *" {
		t.Errorf("MonthlyRecommendCron = %q, want %q", cfg.MonthlyRecommendCron, "0 7 1 * *")
	}
	if cfg.FetchTimeoutSeconds != 20 {
		t.Errorf("FetchTimeoutSeconds = %d, want 20", cfg.FetchTimeoutSeconds)
	}
	if cfg.Extraction.Model != "gpt-oss:20b" {
		t.Errorf("Extraction.Model = %q, want %q", cfg.Extraction.Model, "gpt-oss:20b")
	}
	if cfg.Extraction.Endpoint != "http://ollama:11434" {
		t.Errorf("Extraction.Endpoint = %q, want %q", cfg.Extraction.Endpoint, "http://ollama:11434")
	}
	if cfg.Extraction.ConfidenceThreshold != 0.7 {
		t.Errorf("Extraction.ConfidenceThreshold = %v, want 0.7", cfg.Extraction.ConfidenceThreshold)
	}
	if cfg.Extraction.MaxSourcesPerCard != 3 {
		t.Errorf("Extraction.MaxSourcesPerCard = %d, want 3", cfg.Extraction.MaxSourcesPerCard)
	}
	if len(cfg.Sources) != 2 {
		t.Fatalf("len(Sources) = %d, want 2", len(cfg.Sources))
	}
	if cfg.Sources[0].Name != "Doctor of Credit" || cfg.Sources[0].URL != "https://doctorofcredit.com" || cfg.Sources[0].IssuerHint != "discover" {
		t.Errorf("Sources[0] = %+v, want DoC source", cfg.Sources[0])
	}
	if len(cfg.TrackedCategories) != 3 || cfg.TrackedCategories[0] != "Dining" {
		t.Errorf("TrackedCategories = %v, want [Dining Groceries Gas]", cfg.TrackedCategories)
	}
}

// SCN-083-A07 — disabled feature parses without requiring extraction/source config.
func TestLoadCardRewardsConfig_DisabledParsesWithoutRequiringConfig(t *testing.T) {
	t.Setenv("CARD_REWARDS_ENABLED", "false")
	// Deliberately set nothing else.
	cfg, err := LoadCardRewardsConfig()
	if err != nil {
		t.Fatalf("disabled config must not error, got: %v", err)
	}
	if cfg.Enabled {
		t.Errorf("Enabled = true, want false")
	}
	if len(cfg.Sources) != 0 || len(cfg.TrackedCategories) != 0 {
		t.Errorf("disabled config must not populate sources/categories, got %+v", cfg)
	}
}

// Backward-compat: an unset enable flag behaves as disabled (no error) so the
// monolithic Load() does not break callers that predate this feature.
func TestLoadCardRewardsConfig_UnsetEnabledTreatedAsDisabled(t *testing.T) {
	t.Setenv("CARD_REWARDS_ENABLED", "")
	cfg, err := LoadCardRewardsConfig()
	if err != nil {
		t.Fatalf("unset enabled must not error, got: %v", err)
	}
	if cfg.Enabled {
		t.Errorf("Enabled = true, want false")
	}
}

// SCN-083-A03 — fail-loud on each missing required value when enabled.
func TestLoadCardRewardsConfig_FailLoudOnMissingRequired(t *testing.T) {
	keys := []string{
		"CARD_REWARDS_SCRAPE_CRON",
		"CARD_REWARDS_MONTHLY_RECOMMEND_CRON",
		"CARD_REWARDS_FETCH_TIMEOUT_SECONDS",
		"CARD_REWARDS_EXTRACTION_MODEL",
		"CARD_REWARDS_EXTRACTION_ENDPOINT",
		"CARD_REWARDS_EXTRACTION_CONFIDENCE_THRESHOLD",
		"CARD_REWARDS_EXTRACTION_MAX_SOURCES_PER_CARD",
	}
	for _, missing := range keys {
		t.Run(missing, func(t *testing.T) {
			setValidCardRewardsEnv(t)
			t.Setenv(missing, "")
			_, err := LoadCardRewardsConfig()
			if err == nil {
				t.Fatalf("expected error when %s is empty, got nil", missing)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("error must name %s, got: %v", missing, err)
			}
		})
	}
}

// SCN-083-A04 — empty / malformed sources list rejected when enabled.
func TestLoadCardRewardsConfig_EmptySourcesRejected(t *testing.T) {
	cases := []struct {
		name, value string
	}{
		{"empty_array", "[]"},
		{"empty_string", ""},
		{"not_json", "doctorofcredit.com"},
		{"missing_url", `[{"name":"DoC"}]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setValidCardRewardsEnv(t)
			t.Setenv("CARD_REWARDS_SOURCES", tc.value)
			_, err := LoadCardRewardsConfig()
			if err == nil {
				t.Fatalf("expected error for sources=%q, got nil", tc.value)
			}
			if !strings.Contains(err.Error(), "CARD_REWARDS_SOURCES") {
				t.Errorf("error must name CARD_REWARDS_SOURCES, got: %v", err)
			}
		})
	}
}

// SCN-083-A04 (sibling) — empty tracked_categories rejected when enabled.
func TestLoadCardRewardsConfig_EmptyTrackedCategoriesRejected(t *testing.T) {
	for _, value := range []string{"[]", "", "[ ]"} {
		setValidCardRewardsEnv(t)
		t.Setenv("CARD_REWARDS_TRACKED_CATEGORIES", value)
		_, err := LoadCardRewardsConfig()
		if err == nil {
			t.Fatalf("expected error for tracked_categories=%q, got nil", value)
		}
		if !strings.Contains(err.Error(), "CARD_REWARDS_TRACKED_CATEGORIES") {
			t.Errorf("error must name CARD_REWARDS_TRACKED_CATEGORIES, got: %v", err)
		}
	}
}

// Parameter permutation — confidence threshold out of [0,1] and non-numeric.
func TestLoadCardRewardsConfig_RejectsBadConfidence(t *testing.T) {
	for _, bad := range []string{"-0.1", "1.5", "high"} {
		setValidCardRewardsEnv(t)
		t.Setenv("CARD_REWARDS_EXTRACTION_CONFIDENCE_THRESHOLD", bad)
		_, err := LoadCardRewardsConfig()
		if err == nil {
			t.Fatalf("expected error for confidence=%q, got nil", bad)
		}
		if !strings.Contains(err.Error(), "CARD_REWARDS_EXTRACTION_CONFIDENCE_THRESHOLD") {
			t.Errorf("error must name confidence threshold, got: %v", err)
		}
	}
}

// Parameter permutation — invalid cron expressions rejected.
func TestLoadCardRewardsConfig_RejectsBadCron(t *testing.T) {
	setValidCardRewardsEnv(t)
	t.Setenv("CARD_REWARDS_SCRAPE_CRON", "not a cron")
	_, err := LoadCardRewardsConfig()
	if err == nil {
		t.Fatalf("expected error for invalid scrape_cron, got nil")
	}
	if !strings.Contains(err.Error(), "CARD_REWARDS_SCRAPE_CRON") {
		t.Errorf("error must name scrape cron, got: %v", err)
	}
}

// Parameter permutation — non-positive integers rejected for bounded ints.
func TestLoadCardRewardsConfig_RejectsNonPositiveInts(t *testing.T) {
	cases := []struct{ key, bad string }{
		{"CARD_REWARDS_FETCH_TIMEOUT_SECONDS", "0"},
		{"CARD_REWARDS_FETCH_TIMEOUT_SECONDS", "-5"},
		{"CARD_REWARDS_EXTRACTION_MAX_SOURCES_PER_CARD", "0"},
	}
	for _, tc := range cases {
		t.Run(tc.key+"_"+tc.bad, func(t *testing.T) {
			setValidCardRewardsEnv(t)
			t.Setenv(tc.key, tc.bad)
			_, err := LoadCardRewardsConfig()
			if err == nil {
				t.Fatalf("expected error for %s=%s, got nil", tc.key, tc.bad)
			}
		})
	}
}

// calendar_sync enabled requires the UID prefix (fail-loud).
func TestLoadCardRewardsConfig_CalendarSyncRequiresUIDPrefix(t *testing.T) {
	setValidCardRewardsEnv(t)
	t.Setenv("CARD_REWARDS_CALENDAR_SYNC", "true")
	t.Setenv("CARD_REWARDS_CALENDAR_UID_PREFIX", "")
	_, err := LoadCardRewardsConfig()
	if err == nil {
		t.Fatalf("expected error when calendar_sync=true and uid_prefix empty, got nil")
	}
	if !strings.Contains(err.Error(), "CARD_REWARDS_CALENDAR_UID_PREFIX") {
		t.Errorf("error must name UID prefix, got: %v", err)
	}
}
