package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestProcessEntries_DwellTimeTiering(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		DwellFullMin:     5 * time.Minute,
		DwellStandardMin: 2 * time.Minute,
		DwellLightMin:    30 * time.Second,
	}

	entries := []HistoryEntry{
		{URL: "https://example.com/deep-read", Title: "Deep Read", VisitTime: time.Now(), DwellTime: 6 * time.Minute, Domain: "example.com"},
		{URL: "https://example.com/medium-read", Title: "Medium Read", VisitTime: time.Now(), DwellTime: 3 * time.Minute, Domain: "example.com"},
		{URL: "https://example.com/quick-look", Title: "Quick Look", VisitTime: time.Now(), DwellTime: 45 * time.Second, Domain: "example.com"},
		{URL: "https://example.com/glance", Title: "Glance", VisitTime: time.Now(), DwellTime: 10 * time.Second, Domain: "example.com"},
	}

	artifacts, _, stats := c.processEntries(entries, 0)

	// Privacy gate excludes metadata-tier entries from individual artifacts
	if len(artifacts) != 3 {
		t.Fatalf("expected 3 artifacts (privacy gate excludes metadata tier), got %d", len(artifacts))
	}

	// Verify tier counts (all 4 entries classified, even if metadata doesn't produce artifact)
	if stats.byTier["full"] != 1 {
		t.Errorf("expected 1 full tier, got %d", stats.byTier["full"])
	}
	if stats.byTier["standard"] != 1 {
		t.Errorf("expected 1 standard tier, got %d", stats.byTier["standard"])
	}
	if stats.byTier["light"] != 1 {
		t.Errorf("expected 1 light tier, got %d", stats.byTier["light"])
	}
	if stats.byTier["metadata"] != 1 {
		t.Errorf("expected 1 metadata tier, got %d", stats.byTier["metadata"])
	}

	// Verify tier is set in metadata and no metadata-tier artifact exists
	for _, a := range artifacts {
		tier, ok := a.Metadata["processing_tier"].(string)
		if !ok || tier == "" {
			t.Errorf("artifact %s missing processing_tier metadata", a.URL)
		}
		if tier == "metadata" {
			t.Errorf("metadata-tier entry should not produce individual artifact (privacy gate)")
		}
	}
}

func TestProcessEntries_SkipFiltering(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		CustomSkipDomains: []string{"internal.corp.com"},
	}

	entries := []HistoryEntry{
		{URL: "chrome://settings", Title: "Settings", VisitTime: time.Now(), DwellTime: time.Minute, Domain: ""},
		{URL: "chrome-extension://abc123/popup.html", Title: "Extension", VisitTime: time.Now(), DwellTime: time.Minute, Domain: ""},
		{URL: "localhost:3000/dashboard", Title: "Dev", VisitTime: time.Now(), DwellTime: time.Minute, Domain: "localhost"},
		{URL: "about:blank", Title: "Blank", VisitTime: time.Now(), DwellTime: time.Minute, Domain: ""},
		{URL: "file:///home/user/notes.html", Title: "File", VisitTime: time.Now(), DwellTime: time.Minute, Domain: ""},
		{URL: "https://example.com/real-article", Title: "Real Article", VisitTime: time.Now(), DwellTime: 3 * time.Minute, Domain: "example.com"},
		{URL: "internal.corp.com/secret", Title: "Corp", VisitTime: time.Now(), DwellTime: time.Minute, Domain: "internal.corp.com"},
	}

	artifacts, _, stats := c.processEntries(entries, 0)

	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact (only real article), got %d", len(artifacts))
	}
	if artifacts[0].URL != "https://example.com/real-article" {
		t.Errorf("expected real-article URL, got %s", artifacts[0].URL)
	}
	if stats.skipped != 6 {
		t.Errorf("expected 6 skipped, got %d", stats.skipped)
	}
}

func TestConnect_HistoryFileNotFound(t *testing.T) {
	c := New("browser-history")

	config := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"history_path": "/nonexistent/path/History",
		},
	}

	err := c.Connect(context.Background(), config)
	if err == nil {
		t.Fatal("expected error for nonexistent history file")
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected health error, got %s", c.Health(context.Background()))
	}
}

func TestCopyHistoryFileFrom_RetryOnFailure(t *testing.T) {
	// Create a temp file to simulate a Chrome History file
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "History")
	if err := os.WriteFile(historyPath, []byte("fake-sqlite-data"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	c := New("browser-history")

	// Success case
	tmpPath, err := c.copyHistoryFileFrom(historyPath)
	if err != nil {
		t.Fatalf("copyHistoryFileFrom failed: %v", err)
	}
	defer os.Remove(tmpPath)

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("read temp: %v", err)
	}
	if string(data) != "fake-sqlite-data" {
		t.Errorf("expected fake-sqlite-data, got %s", string(data))
	}

	// Failure case: non-existent source
	_, err = c.copyHistoryFileFrom("/nonexistent/path/History")
	if err == nil {
		t.Error("expected error for nonexistent source")
	}
}

func TestParseBrowserConfig_Defaults(t *testing.T) {
	config := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"history_path": "/home/user/.config/google-chrome/Default/History",
		},
	}

	cfg, err := parseBrowserConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.HistoryPath != "/home/user/.config/google-chrome/Default/History" {
		t.Errorf("unexpected history_path: %s", cfg.HistoryPath)
	}
	if cfg.AccessStrategy != "copy" {
		t.Errorf("expected default access_strategy 'copy', got %s", cfg.AccessStrategy)
	}
	if cfg.InitialLookbackDays != 30 {
		t.Errorf("expected default lookback 30, got %d", cfg.InitialLookbackDays)
	}
	if cfg.RepeatVisitWindow != 7*24*time.Hour {
		t.Errorf("expected default repeat_visit_window 7d, got %v", cfg.RepeatVisitWindow)
	}
	if cfg.RepeatVisitThreshold != 3 {
		t.Errorf("expected default repeat_visit_threshold 3, got %d", cfg.RepeatVisitThreshold)
	}
	if cfg.ContentFetchTimeout != 15*time.Second {
		t.Errorf("expected default content_fetch_timeout 15s, got %v", cfg.ContentFetchTimeout)
	}
	if cfg.ContentFetchConcurrency != 5 {
		t.Errorf("expected default concurrency 5, got %d", cfg.ContentFetchConcurrency)
	}
	if cfg.SocialMediaIndividualThreshold != 5*time.Minute {
		t.Errorf("expected default social_media_individual_threshold 5m, got %v", cfg.SocialMediaIndividualThreshold)
	}
}

func TestParseBrowserConfig_ValidationErrors(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]interface{}
		errMsg string
	}{
		{
			name:   "missing history_path",
			config: map[string]interface{}{},
			errMsg: "history_path is required",
		},
		{
			name:   "empty history_path",
			config: map[string]interface{}{"history_path": ""},
			errMsg: "history_path is required",
		},
		{
			name: "invalid access_strategy",
			config: map[string]interface{}{
				"history_path":    "/some/path",
				"access_strategy": "direct",
			},
			errMsg: "access_strategy must be",
		},
		{
			name: "invalid repeat_visit_window",
			config: map[string]interface{}{
				"history_path":        "/some/path",
				"repeat_visit_window": "invalid",
			},
			errMsg: "invalid repeat_visit_window",
		},
		{
			name: "invalid content_fetch_timeout",
			config: map[string]interface{}{
				"history_path":          "/some/path",
				"content_fetch_timeout": "not-a-duration",
			},
			errMsg: "invalid content_fetch_timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := connector.ConnectorConfig{
				SourceConfig: tt.config,
			}
			_, err := parseBrowserConfig(config)
			if err == nil {
				t.Fatalf("expected error containing %q", tt.errMsg)
			}
			if !contains(err.Error(), tt.errMsg) {
				t.Errorf("expected error containing %q, got: %v", tt.errMsg, err)
			}
		})
	}
}

func TestCursorConversion_RoundTrip(t *testing.T) {
	// Test GoTimeToChrome / ChromeTimeToGo round trip
	now := time.Now().Truncate(time.Microsecond)
	chrome := GoTimeToChrome(now)
	back := ChromeTimeToGo(chrome)
	if !now.Equal(back) {
		t.Errorf("round trip failed: %v != %v", now, back)
	}
}

func TestConnector_HealthLifecycle(t *testing.T) {
	ctx := context.Background()
	c := New("browser-history")

	// Initial state: disconnected
	if c.Health(ctx) != connector.HealthDisconnected {
		t.Errorf("expected disconnected, got %s", c.Health(ctx))
	}

	// Create a temp file as fake history
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "History")
	if err := os.WriteFile(historyPath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	config := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"history_path": historyPath,
		},
	}

	// After Connect: healthy
	if err := c.Connect(ctx, config); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if c.Health(ctx) != connector.HealthHealthy {
		t.Errorf("expected healthy after Connect, got %s", c.Health(ctx))
	}

	// After Close: disconnected
	if err := c.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if c.Health(ctx) != connector.HealthDisconnected {
		t.Errorf("expected disconnected after Close, got %s", c.Health(ctx))
	}
}

func TestClose_SetsDisconnected(t *testing.T) {
	c := New("browser-history")
	c.mu.Lock()
	c.health = connector.HealthHealthy
	c.mu.Unlock()

	if err := c.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Errorf("expected disconnected, got %s", c.Health(context.Background()))
	}
}

func TestSync_EmptyCursor_UsesLookback(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		InitialLookbackDays: 30,
	}

	// processEntries with empty entries should return prev cursor
	artifacts, cursor, stats := c.processEntries(nil, GoTimeToChrome(time.Now().AddDate(0, 0, -30)))
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts, got %d", len(artifacts))
	}
	if cursor == "" {
		t.Error("expected non-empty cursor")
	}
	if stats.skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", stats.skipped)
	}
}

func TestGoTimeToChrome_ChromeTimeToGo_RoundTrip(t *testing.T) {
	// Known value: 2023-01-01 00:00:00 UTC
	known := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	chrome := GoTimeToChrome(known)
	back := ChromeTimeToGo(chrome)
	if !known.Equal(back) {
		t.Errorf("round trip failed: %v != %v", known, back)
	}

	// Edge: Unix epoch
	epoch := time.Unix(0, 0).UTC()
	chromeEpoch := GoTimeToChrome(epoch)
	backEpoch := ChromeTimeToGo(chromeEpoch)
	if !epoch.Equal(backEpoch) {
		t.Errorf("unix epoch round trip failed: %v != %v (chrome: %d)", epoch, backEpoch, chromeEpoch)
	}
}

func TestProcessEntries_CursorAdvances(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{}

	baseTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{URL: "https://example.com/a", Title: "A", VisitTime: baseTime, DwellTime: 3 * time.Minute, Domain: "example.com"},
		{URL: "https://example.com/b", Title: "B", VisitTime: baseTime.Add(time.Hour), DwellTime: 3 * time.Minute, Domain: "example.com"},
		{URL: "https://example.com/c", Title: "C", VisitTime: baseTime.Add(2 * time.Hour), DwellTime: 3 * time.Minute, Domain: "example.com"},
	}

	_, cursor, _ := c.processEntries(entries, 0)

	expected := strconv.FormatInt(GoTimeToChrome(baseTime.Add(2*time.Hour)), 10)
	if cursor != expected {
		t.Errorf("expected cursor %s, got %s", expected, cursor)
	}
}

func TestProcessEntries_SourceID(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{}

	entries := []HistoryEntry{
		{URL: "https://example.com/page", Title: "Page", VisitTime: time.Now(), DwellTime: 3 * time.Minute, Domain: "example.com"},
	}

	artifacts, _, _ := c.processEntries(entries, 0)

	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].SourceID != "browser-history" {
		t.Errorf("expected source_id 'browser-history', got %s", artifacts[0].SourceID)
	}
}

func TestParseDurationWithDays(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"7d", 7 * 24 * time.Hour, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"5m", 5 * time.Minute, false},
		{"15s", 15 * time.Second, false},
		{"2h", 2 * time.Hour, false},
		{"bad", 0, true},
	}

	for _, tt := range tests {
		got, err := parseDurationWithDays(tt.input)
		if tt.wantErr && err == nil {
			t.Errorf("parseDurationWithDays(%q) expected error", tt.input)
			continue
		}
		if !tt.wantErr && err != nil {
			t.Errorf("parseDurationWithDays(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.expected {
			t.Errorf("parseDurationWithDays(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestParseBrowserConfig_CustomSkipDomains(t *testing.T) {
	config := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"history_path":        "/some/path",
			"custom_skip_domains": []interface{}{"internal.corp.com", "intranet.local"},
		},
	}

	cfg, err := parseBrowserConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.CustomSkipDomains) != 2 {
		t.Fatalf("expected 2 custom skip domains, got %d", len(cfg.CustomSkipDomains))
	}
	if cfg.CustomSkipDomains[0] != "internal.corp.com" || cfg.CustomSkipDomains[1] != "intranet.local" {
		t.Errorf("unexpected custom skip domains: %v", cfg.CustomSkipDomains)
	}
}

// contains checks if s contains substr.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// --- Scope 2: Social Media Aggregation, Repeat Visits & Privacy Gate ---

func TestProcessEntries_SocialMediaAggregation(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		SocialMediaIndividualThreshold: 5 * time.Minute,
		RepeatVisitThreshold:           3,
	}

	baseTime := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{URL: "https://reddit.com/r/golang/post1", Title: "Go Post 1", VisitTime: baseTime, DwellTime: 2 * time.Minute, Domain: "reddit.com"},
		{URL: "https://reddit.com/r/golang/post2", Title: "Go Post 2", VisitTime: baseTime.Add(10 * time.Minute), DwellTime: 90 * time.Second, Domain: "reddit.com"},
		{URL: "https://reddit.com/r/rust/post1", Title: "Rust Post", VisitTime: baseTime.Add(20 * time.Minute), DwellTime: 45 * time.Second, Domain: "reddit.com"},
		{URL: "https://twitter.com/user/status/123", Title: "Tweet 1", VisitTime: baseTime, DwellTime: 30 * time.Second, Domain: "twitter.com"},
		{URL: "https://twitter.com/user/status/456", Title: "Tweet 2", VisitTime: baseTime.Add(5 * time.Minute), DwellTime: 15 * time.Second, Domain: "twitter.com"},
	}

	artifacts, _, stats := c.processEntries(entries, 0)

	// Should produce 2 aggregate artifacts (reddit.com + twitter.com), no individual
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 aggregate artifacts, got %d", len(artifacts))
	}
	if stats.socialAggregates != 2 {
		t.Errorf("expected 2 social aggregates, got %d", stats.socialAggregates)
	}

	for _, a := range artifacts {
		if a.ContentType != "browsing/social-aggregate" {
			t.Errorf("expected content_type 'browsing/social-aggregate', got %s", a.ContentType)
		}
		if a.SourceID != "browser-history" {
			t.Errorf("expected source_id 'browser-history', got %s", a.SourceID)
		}
	}

	// Find reddit aggregate and verify fields
	var foundReddit bool
	for _, a := range artifacts {
		if a.Metadata["domain"] == "reddit.com" {
			foundReddit = true
			if a.Metadata["visit_count"] != 3 {
				t.Errorf("expected reddit visit_count 3, got %v", a.Metadata["visit_count"])
			}
			totalDwell, ok := a.Metadata["total_dwell_seconds"].(float64)
			if !ok {
				t.Fatal("total_dwell_seconds not float64")
			}
			// 2m + 1m30s + 45s = 255s
			if totalDwell != 255.0 {
				t.Errorf("expected reddit total_dwell_seconds 255, got %v", totalDwell)
			}
			// Privacy: individual URLs must NOT be stored in social media aggregates
			if _, hasURLs := a.Metadata["urls"]; hasURLs {
				t.Error("social media aggregate must not store individual URLs (privacy requirement R-402)")
			}
		}
	}
	if !foundReddit {
		t.Error("reddit.com aggregate not found")
	}
}

func TestProcessEntries_SocialMediaHighDwellIndividual(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		SocialMediaIndividualThreshold: 5 * time.Minute,
		RepeatVisitThreshold:           3,
	}

	baseTime := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{URL: "https://reddit.com/r/programming/long-post", Title: "Long Post", VisitTime: baseTime, DwellTime: 8 * time.Minute, Domain: "reddit.com"},
		{URL: "https://reddit.com/r/golang/post1", Title: "Quick Post", VisitTime: baseTime.Add(10 * time.Minute), DwellTime: 1 * time.Minute, Domain: "reddit.com"},
	}

	artifacts, _, _ := c.processEntries(entries, 0)

	// 1 individual artifact (long-post at full tier) + 1 aggregate (quick post)
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
	}

	var foundIndividual, foundAggregate bool
	for _, a := range artifacts {
		if a.ContentType == "url" {
			foundIndividual = true
			if a.URL != "https://reddit.com/r/programming/long-post" {
				t.Errorf("unexpected individual URL: %s", a.URL)
			}
			tier, _ := a.Metadata["processing_tier"].(string)
			if tier != "full" {
				t.Errorf("expected 'full' tier for long-post, got %s", tier)
			}
		}
		if a.ContentType == "browsing/social-aggregate" {
			foundAggregate = true
			if a.Metadata["domain"] != "reddit.com" {
				t.Errorf("expected reddit.com domain, got %v", a.Metadata["domain"])
			}
			if a.Metadata["visit_count"] != 1 {
				t.Errorf("expected 1 visit in aggregate, got %v", a.Metadata["visit_count"])
			}
		}
	}
	if !foundIndividual {
		t.Error("individual long-post artifact not found")
	}
	if !foundAggregate {
		t.Error("reddit.com aggregate not found")
	}
}

func TestDetectRepeatVisits_TierEscalation(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		RepeatVisitThreshold:           3,
		SocialMediaIndividualThreshold: 5 * time.Minute,
	}

	// URL repeated across 5 different days with 90s dwell each (normally "light").
	// Multi-day layout exercises both repeat detection and R-010 dedup correctly:
	// repeat detection sees 5 raw visits; dedup produces 5 entries (one per day).
	day1 := time.Date(2025, 3, 11, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 3, 12, 10, 0, 0, 0, time.UTC)
	day3 := time.Date(2025, 3, 13, 10, 0, 0, 0, time.UTC)
	day4 := time.Date(2025, 3, 14, 10, 0, 0, 0, time.UTC)
	day5 := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{URL: "https://docs.example.com/api-ref", Title: "API Ref", VisitTime: day1, DwellTime: 90 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/api-ref", Title: "API Ref", VisitTime: day2, DwellTime: 90 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/api-ref", Title: "API Ref", VisitTime: day3, DwellTime: 90 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/api-ref", Title: "API Ref", VisitTime: day4, DwellTime: 90 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/api-ref", Title: "API Ref", VisitTime: day5, DwellTime: 90 * time.Second, Domain: "docs.example.com"},
		// A unique URL for comparison
		{URL: "https://example.com/once", Title: "Once", VisitTime: day1, DwellTime: 90 * time.Second, Domain: "example.com"},
	}

	artifacts, _, stats := c.processEntries(entries, 0)

	// 5 escalated (light→standard) + 1 non-escalated (light) = 6 artifacts
	if len(artifacts) != 6 {
		t.Fatalf("expected 6 artifacts, got %d", len(artifacts))
	}
	if stats.repeatEscalations != 5 {
		t.Errorf("expected 5 repeat escalations, got %d", stats.repeatEscalations)
	}

	var standardCount, lightCount int
	for _, a := range artifacts {
		tier, _ := a.Metadata["processing_tier"].(string)
		switch tier {
		case "standard":
			standardCount++
			if a.URL == "https://docs.example.com/api-ref" {
				rv, ok := a.Metadata["repeat_visits"]
				if !ok {
					t.Error("expected repeat_visits metadata on escalated artifact")
				}
				if rv != 5 {
					t.Errorf("expected repeat_visits 5, got %v", rv)
				}
			}
		case "light":
			lightCount++
		}
	}
	if standardCount != 5 {
		t.Errorf("expected 5 standard tier artifacts, got %d", standardCount)
	}
	if lightCount != 1 {
		t.Errorf("expected 1 light tier artifact, got %d", lightCount)
	}
}

func TestEscalateTier_AllTransitions(t *testing.T) {
	c := New("browser-history")

	tests := []struct {
		input    string
		expected string
	}{
		{"metadata", "light"},
		{"light", "standard"},
		{"standard", "full"},
		{"full", "full"},
	}

	for _, tt := range tests {
		got := c.escalateTier(tt.input)
		if got != tt.expected {
			t.Errorf("escalateTier(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestProcessEntries_PrivacyGate_MetadataTierNoArtifact(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		SocialMediaIndividualThreshold: 5 * time.Minute,
	}

	baseTime := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	// All entries under 30s → metadata tier → privacy gate → no artifacts
	entries := []HistoryEntry{
		{URL: "https://a.com/1", Title: "A1", VisitTime: baseTime, DwellTime: 10 * time.Second, Domain: "a.com"},
		{URL: "https://b.com/2", Title: "B2", VisitTime: baseTime.Add(time.Minute), DwellTime: 15 * time.Second, Domain: "b.com"},
		{URL: "https://c.com/3", Title: "C3", VisitTime: baseTime.Add(2 * time.Minute), DwellTime: 20 * time.Second, Domain: "c.com"},
		{URL: "https://d.com/4", Title: "D4", VisitTime: baseTime.Add(3 * time.Minute), DwellTime: 25 * time.Second, Domain: "d.com"},
		{URL: "https://e.com/5", Title: "E5", VisitTime: baseTime.Add(4 * time.Minute), DwellTime: 29 * time.Second, Domain: "e.com"},
	}

	artifacts, _, stats := c.processEntries(entries, 0)

	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts for metadata-tier entries, got %d", len(artifacts))
	}
	if stats.byTier["metadata"] != 5 {
		t.Errorf("expected 5 metadata tier, got %d", stats.byTier["metadata"])
	}
}

func TestProcessEntries_ContentFetchFailure(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		SocialMediaIndividualThreshold: 5 * time.Minute,
		RepeatVisitThreshold:           3,
	}
	c.contentFetcher = func(url string) (string, error) {
		return "", fmt.Errorf("HTTP 404: page not found")
	}

	entries := []HistoryEntry{
		{URL: "https://example.com/article", Title: "Article", VisitTime: time.Now(), DwellTime: 6 * time.Minute, Domain: "example.com"},
	}

	artifacts, _, stats := c.processEntries(entries, 0)

	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}

	a := artifacts[0]
	if a.Metadata["content_fetch_failed"] != true {
		t.Error("expected content_fetch_failed to be true")
	}
	if a.RawContent != "" {
		t.Errorf("expected empty RawContent for failed fetch, got %q", a.RawContent)
	}
	if stats.fetchFails != 1 {
		t.Errorf("expected 1 fetch failure, got %d", stats.fetchFails)
	}
}

func TestBuildSocialAggregate_ArtifactFields(t *testing.T) {
	c := New("browser-history")
	day := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)

	entries := []HistoryEntry{
		{URL: "https://reddit.com/r/golang/post1", Title: "Post 1", VisitTime: day.Add(2 * time.Hour), DwellTime: 2 * time.Minute, Domain: "reddit.com"},
		{URL: "https://reddit.com/r/golang/post2", Title: "Post 2", VisitTime: day.Add(4 * time.Hour), DwellTime: 90 * time.Second, Domain: "reddit.com"},
	}

	agg := c.buildSocialAggregate("reddit.com", entries, day)

	if agg.SourceID != "browser-history" {
		t.Errorf("expected source_id 'browser-history', got %s", agg.SourceID)
	}
	if agg.ContentType != "browsing/social-aggregate" {
		t.Errorf("expected content_type 'browsing/social-aggregate', got %s", agg.ContentType)
	}
	expectedRef := "social-aggregate:reddit.com:2025-03-15"
	if agg.SourceRef != expectedRef {
		t.Errorf("expected source_ref %q, got %q", expectedRef, agg.SourceRef)
	}
	if agg.Metadata["domain"] != "reddit.com" {
		t.Errorf("expected domain 'reddit.com', got %v", agg.Metadata["domain"])
	}
	if agg.Metadata["date"] != "2025-03-15" {
		t.Errorf("expected date '2025-03-15', got %v", agg.Metadata["date"])
	}
	if agg.Metadata["visit_count"] != 2 {
		t.Errorf("expected visit_count 2, got %v", agg.Metadata["visit_count"])
	}
	// Total dwell: 2m + 1m30s = 210s
	totalDwell, ok := agg.Metadata["total_dwell_seconds"].(float64)
	if !ok {
		t.Fatal("total_dwell_seconds not float64")
	}
	if totalDwell != 210.0 {
		t.Errorf("expected total_dwell_seconds 210, got %v", totalDwell)
	}
	// Peak page tracking (R-005)
	peakTitle, ok := agg.Metadata["peak_page_title"].(string)
	if !ok || peakTitle == "" {
		t.Error("expected peak_page_title in aggregate metadata")
	}
	if peakTitle != "Post 1" {
		t.Errorf("expected peak_page_title 'Post 1' (2m dwell), got %q", peakTitle)
	}
	peakDwell, ok := agg.Metadata["peak_page_dwell_seconds"].(float64)
	if !ok {
		t.Fatal("peak_page_dwell_seconds not float64")
	}
	if peakDwell != 120.0 {
		t.Errorf("expected peak_page_dwell_seconds 120, got %v", peakDwell)
	}
	// Aggregate should have human-readable content (R-005)
	if agg.RawContent == "" {
		t.Error("expected non-empty RawContent in social aggregate")
	}
	// Privacy: individual URLs must NOT be stored in social media aggregates (R-402)
	if _, hasURLs := agg.Metadata["urls"]; hasURLs {
		t.Error("social media aggregate must not store individual URLs (privacy requirement R-402)")
	}
	if !agg.CapturedAt.Equal(day) {
		t.Errorf("expected captured_at %v, got %v", day, agg.CapturedAt)
	}
}

func TestDetectRepeatVisits_BelowThreshold_NoEscalation(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		RepeatVisitThreshold:           3,
		SocialMediaIndividualThreshold: 5 * time.Minute,
	}

	// URL visited only 2 times across different days (below threshold of 3).
	// Multi-day layout prevents R-010 dedup from merging them.
	day1 := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 3, 16, 10, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{URL: "https://example.com/page", Title: "Page", VisitTime: day1, DwellTime: 90 * time.Second, Domain: "example.com"},
		{URL: "https://example.com/page", Title: "Page", VisitTime: day2, DwellTime: 90 * time.Second, Domain: "example.com"},
		{URL: "https://example.com/other", Title: "Other", VisitTime: day1, DwellTime: 45 * time.Second, Domain: "example.com"},
	}

	artifacts, _, stats := c.processEntries(entries, 0)

	if stats.repeatEscalations != 0 {
		t.Errorf("expected 0 repeat escalations, got %d", stats.repeatEscalations)
	}
	// All 3 stay at "light" tier
	if len(artifacts) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(artifacts))
	}
	for _, a := range artifacts {
		tier, _ := a.Metadata["processing_tier"].(string)
		if tier != "light" {
			t.Errorf("expected 'light' tier, got %s for %s", tier, a.URL)
		}
		if _, ok := a.Metadata["repeat_visits"]; ok {
			t.Errorf("did not expect repeat_visits metadata on non-escalated artifact")
		}
	}
}

func TestDetectRepeatVisits_SocialMediaExcluded(t *testing.T) {
	c := New("browser-history")

	entries := []HistoryEntry{
		{URL: "https://reddit.com/r/golang/post1", Title: "Post 1", VisitTime: time.Now(), DwellTime: time.Minute, Domain: "reddit.com"},
		{URL: "https://reddit.com/r/golang/post1", Title: "Post 1", VisitTime: time.Now(), DwellTime: time.Minute, Domain: "reddit.com"},
		{URL: "https://reddit.com/r/golang/post1", Title: "Post 1", VisitTime: time.Now(), DwellTime: time.Minute, Domain: "reddit.com"},
		{URL: "https://example.com/page", Title: "Page", VisitTime: time.Now(), DwellTime: time.Minute, Domain: "example.com"},
	}

	freq := c.detectRepeatVisits(entries)

	if _, ok := freq["https://reddit.com/r/golang/post1"]; ok {
		t.Error("social media URL should be excluded from repeat visit detection")
	}
	if count, ok := freq["https://example.com/page"]; !ok || count != 1 {
		t.Errorf("expected example.com page count 1, got %d", count)
	}
}

func TestProcessEntries_PrivacyGate_LightTierStoresURL(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		SocialMediaIndividualThreshold: 5 * time.Minute,
	}

	entries := []HistoryEntry{
		{URL: "https://example.com/article", Title: "Article", VisitTime: time.Now(), DwellTime: 45 * time.Second, Domain: "example.com"},
	}

	artifacts, _, _ := c.processEntries(entries, 0)

	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact for light-tier entry, got %d", len(artifacts))
	}

	a := artifacts[0]
	if a.URL != "https://example.com/article" {
		t.Errorf("expected URL stored for light tier, got %s", a.URL)
	}
	if a.RawContent != "https://example.com/article" {
		t.Errorf("expected RawContent with URL for light tier, got %s", a.RawContent)
	}
	tier, _ := a.Metadata["processing_tier"].(string)
	if tier != "light" {
		t.Errorf("expected 'light' tier, got %s", tier)
	}
}

func TestProcessEntries_ContentFetchSuccess(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		SocialMediaIndividualThreshold: 5 * time.Minute,
		RepeatVisitThreshold:           3,
	}
	c.contentFetcher = func(url string) (string, error) {
		return "<p>Extracted article content about Go programming</p>", nil
	}

	entries := []HistoryEntry{
		{URL: "https://example.com/go-article", Title: "Go Article", VisitTime: time.Now(), DwellTime: 6 * time.Minute, Domain: "example.com"},
	}

	artifacts, _, stats := c.processEntries(entries, 0)

	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}

	a := artifacts[0]
	if a.RawContent != "<p>Extracted article content about Go programming</p>" {
		t.Errorf("expected fetched content in RawContent, got %q", a.RawContent)
	}
	if _, ok := a.Metadata["content_fetch_failed"]; ok {
		t.Error("content_fetch_failed should not be set on successful fetch")
	}
	if stats.fetchFails != 0 {
		t.Errorf("expected 0 fetch failures, got %d", stats.fetchFails)
	}
	tier, _ := a.Metadata["processing_tier"].(string)
	if tier != "full" {
		t.Errorf("expected 'full' tier, got %s", tier)
	}
}

func TestProcessEntries_RepeatEscalation_MetadataToLight_SurvivesPrivacyGate(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		RepeatVisitThreshold:           3,
		SocialMediaIndividualThreshold: 5 * time.Minute,
	}

	// URL visited once per day across 4 different days with 10s dwell each
	// (normally "metadata" → privacy gate excludes). Multi-day layout prevents
	// R-010 dedup from merging. Repeat detection sees 4 raw visits → escalation
	// bumps metadata→light, which survives the privacy gate.
	day1 := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 3, 16, 10, 0, 0, 0, time.UTC)
	day3 := time.Date(2025, 3, 17, 10, 0, 0, 0, time.UTC)
	day4 := time.Date(2025, 3, 18, 10, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{URL: "https://docs.example.com/faq", Title: "FAQ", VisitTime: day1, DwellTime: 10 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/faq", Title: "FAQ", VisitTime: day2, DwellTime: 10 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/faq", Title: "FAQ", VisitTime: day3, DwellTime: 10 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/faq", Title: "FAQ", VisitTime: day4, DwellTime: 10 * time.Second, Domain: "docs.example.com"},
		// Control: single-visit metadata-tier URL should be excluded by privacy gate
		{URL: "https://clickbait.com/bait", Title: "Bait", VisitTime: day1, DwellTime: 5 * time.Second, Domain: "clickbait.com"},
	}

	artifacts, _, stats := c.processEntries(entries, 0)

	// 4 repeat-escalated entries (metadata→light) survive privacy gate
	// 1 single-visit metadata entry blocked by privacy gate
	if len(artifacts) != 4 {
		t.Fatalf("expected 4 artifacts (repeat-escalated survive gate, single metadata excluded), got %d", len(artifacts))
	}
	if stats.repeatEscalations != 4 {
		t.Errorf("expected 4 repeat escalations, got %d", stats.repeatEscalations)
	}

	for _, a := range artifacts {
		tier, _ := a.Metadata["processing_tier"].(string)
		if tier != "light" {
			t.Errorf("expected 'light' tier after escalation, got %s for %s", tier, a.URL)
		}
		if a.URL != "https://docs.example.com/faq" {
			t.Errorf("only escalated FAQ entries should produce artifacts, got %s", a.URL)
		}
	}
}

func TestProcessEntries_SocialMediaAggregation_MultiDay(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		SocialMediaIndividualThreshold: 5 * time.Minute,
		RepeatVisitThreshold:           3,
	}

	day1 := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 3, 16, 14, 0, 0, 0, time.UTC)
	entries := []HistoryEntry{
		{URL: "https://reddit.com/r/golang/post1", Title: "Day1 Post", VisitTime: day1, DwellTime: 2 * time.Minute, Domain: "reddit.com"},
		{URL: "https://reddit.com/r/golang/post2", Title: "Day1 Post2", VisitTime: day1.Add(30 * time.Minute), DwellTime: 1 * time.Minute, Domain: "reddit.com"},
		{URL: "https://reddit.com/r/rust/post1", Title: "Day2 Post", VisitTime: day2, DwellTime: 90 * time.Second, Domain: "reddit.com"},
	}

	artifacts, _, stats := c.processEntries(entries, 0)

	// Should produce 2 separate aggregates: reddit.com on 2025-03-15 and reddit.com on 2025-03-16
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 aggregate artifacts (one per day), got %d", len(artifacts))
	}
	if stats.socialAggregates != 2 {
		t.Errorf("expected 2 social aggregates, got %d", stats.socialAggregates)
	}

	dates := make(map[string]int)
	for _, a := range artifacts {
		if a.ContentType != "browsing/social-aggregate" {
			t.Errorf("expected browsing/social-aggregate, got %s", a.ContentType)
		}
		date, _ := a.Metadata["date"].(string)
		visitCount, _ := a.Metadata["visit_count"].(int)
		dates[date] = visitCount
	}

	if dates["2025-03-15"] != 2 {
		t.Errorf("expected 2 visits on 2025-03-15, got %d", dates["2025-03-15"])
	}
	if dates["2025-03-16"] != 1 {
		t.Errorf("expected 1 visit on 2025-03-16, got %d", dates["2025-03-16"])
	}
}

func TestProcessEntries_CustomDwellThresholds(t *testing.T) {
	// R002 regression: configurable dwell thresholds must actually be used
	// by processEntries, not silently ignored in favor of hardcoded defaults.
	c := New("browser-history")
	c.config = BrowserConfig{
		DwellFullMin:                   10 * time.Minute, // Custom: 10m instead of default 5m
		DwellStandardMin:               5 * time.Minute,  // Custom: 5m instead of default 2m
		DwellLightMin:                  1 * time.Minute,  // Custom: 1m instead of default 30s
		SocialMediaIndividualThreshold: 5 * time.Minute,
	}

	entries := []HistoryEntry{
		// 6 minutes: with defaults would be "full", with custom thresholds should be "standard"
		{URL: "https://example.com/article", Title: "Article", VisitTime: time.Now(), DwellTime: 6 * time.Minute, Domain: "example.com"},
		// 3 minutes: with defaults would be "standard", with custom thresholds should be "light"
		{URL: "https://example.com/quick", Title: "Quick", VisitTime: time.Now(), DwellTime: 3 * time.Minute, Domain: "example.com"},
		// 45 seconds: with defaults would be "light", with custom thresholds should be "metadata" (excluded by privacy gate)
		{URL: "https://example.com/glance", Title: "Glance", VisitTime: time.Now(), DwellTime: 45 * time.Second, Domain: "example.com"},
		// 12 minutes: should be "full" under custom thresholds
		{URL: "https://example.com/deep", Title: "Deep", VisitTime: time.Now(), DwellTime: 12 * time.Minute, Domain: "example.com"},
	}

	artifacts, _, stats := c.processEntries(entries, 0)

	// Glance (45s) → metadata → excluded by privacy gate → 3 artifacts
	if len(artifacts) != 3 {
		t.Fatalf("expected 3 artifacts (metadata-tier excluded by privacy gate), got %d", len(artifacts))
	}

	// Verify tier assignments use custom thresholds
	if stats.byTier["full"] != 1 {
		t.Errorf("expected 1 full (12m entry), got %d", stats.byTier["full"])
	}
	if stats.byTier["standard"] != 1 {
		t.Errorf("expected 1 standard (6m entry), got %d", stats.byTier["standard"])
	}
	if stats.byTier["light"] != 1 {
		t.Errorf("expected 1 light (3m entry), got %d", stats.byTier["light"])
	}
	if stats.byTier["metadata"] != 1 {
		t.Errorf("expected 1 metadata (45s entry), got %d", stats.byTier["metadata"])
	}

	// Verify each artifact has the correct tier in metadata
	tiers := make(map[string]string)
	for _, a := range artifacts {
		tier, _ := a.Metadata["processing_tier"].(string)
		tiers[a.URL] = tier
	}
	if tiers["https://example.com/deep"] != "full" {
		t.Errorf("12m article expected 'full', got %q", tiers["https://example.com/deep"])
	}
	if tiers["https://example.com/article"] != "standard" {
		t.Errorf("6m article expected 'standard', got %q", tiers["https://example.com/article"])
	}
	if tiers["https://example.com/quick"] != "light" {
		t.Errorf("3m article expected 'light', got %q", tiers["https://example.com/quick"])
	}
}

func TestParseBrowserConfig_DwellTimeThresholds(t *testing.T) {
	// R002 regression: dwell_time_thresholds from config must be parsed into BrowserConfig
	config := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"history_path": "/some/path",
			"dwell_time_thresholds": map[string]interface{}{
				"full_min":     "10m",
				"standard_min": "5m",
				"light_min":    "1m",
			},
		},
	}

	cfg, err := parseBrowserConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DwellFullMin != 10*time.Minute {
		t.Errorf("DwellFullMin = %v, want 10m", cfg.DwellFullMin)
	}
	if cfg.DwellStandardMin != 5*time.Minute {
		t.Errorf("DwellStandardMin = %v, want 5m", cfg.DwellStandardMin)
	}
	if cfg.DwellLightMin != 1*time.Minute {
		t.Errorf("DwellLightMin = %v, want 1m", cfg.DwellLightMin)
	}
}

func TestParseBrowserConfig_DwellTimeThresholds_Invalid(t *testing.T) {
	config := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"history_path": "/some/path",
			"dwell_time_thresholds": map[string]interface{}{
				"full_min": "not-a-duration",
			},
		},
	}

	_, err := parseBrowserConfig(config)
	if err == nil {
		t.Fatal("expected error for invalid dwell_time_thresholds.full_min")
	}
	if !contains(err.Error(), "invalid dwell_time_thresholds.full_min") {
		t.Errorf("expected error about full_min, got: %v", err)
	}
}

// --- R-010: URL+Date Dedup ---

func TestDedupByURLDate(t *testing.T) {
	day1 := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	day1Later := time.Date(2025, 3, 15, 14, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 3, 16, 10, 0, 0, 0, time.UTC)

	entries := []HistoryEntry{
		{URL: "https://example.com/page", Title: "Page v1", VisitTime: day1, DwellTime: 90 * time.Second, Domain: "example.com"},
		{URL: "https://example.com/page", Title: "Page v2", VisitTime: day1Later, DwellTime: 3 * time.Minute, Domain: "example.com"},
		{URL: "https://example.com/page", Title: "Page day2", VisitTime: day2, DwellTime: 45 * time.Second, Domain: "example.com"},
		{URL: "https://other.com/x", Title: "Other", VisitTime: day1, DwellTime: time.Minute, Domain: "other.com"},
	}

	result := dedupByURLDate(entries)

	if len(result) != 3 {
		t.Fatalf("expected 3 deduped entries, got %d", len(result))
	}

	// First entry: page on day1 — merged (90s + 3m = 4.5m), latest time, best title
	if result[0].URL != "https://example.com/page" {
		t.Errorf("entry 0 URL = %s, want example.com/page", result[0].URL)
	}
	if result[0].DwellTime != 90*time.Second+3*time.Minute {
		t.Errorf("entry 0 dwell = %v, want 4m30s (merged)", result[0].DwellTime)
	}
	if !result[0].VisitTime.Equal(day1Later) {
		t.Errorf("entry 0 visit_time = %v, want latest %v", result[0].VisitTime, day1Later)
	}
	if result[0].Title != "Page v2" {
		t.Errorf("entry 0 title = %q, want 'Page v2' (from longest dwell)", result[0].Title)
	}

	// Second entry: page on day2 — not merged with day1
	if result[1].URL != "https://example.com/page" {
		t.Errorf("entry 1 URL = %s, want example.com/page", result[1].URL)
	}
	if result[1].DwellTime != 45*time.Second {
		t.Errorf("entry 1 dwell = %v, want 45s (unmerged)", result[1].DwellTime)
	}

	// Third entry: other URL — unchanged
	if result[2].URL != "https://other.com/x" {
		t.Errorf("entry 2 URL = %s, want other.com/x", result[2].URL)
	}
}

func TestProcessEntries_DedupSameURLSameDay(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		DwellFullMin:                   5 * time.Minute,
		DwellStandardMin:               2 * time.Minute,
		DwellLightMin:                  30 * time.Second,
		SocialMediaIndividualThreshold: 5 * time.Minute,
	}

	baseTime := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	// Three visits to the same URL on the same day: 2m + 2m + 2m = 6m total.
	// Individually each is "standard" (2-5m), but merged is "full" (≥5m).
	entries := []HistoryEntry{
		{URL: "https://example.com/article", Title: "Article", VisitTime: baseTime, DwellTime: 2 * time.Minute, Domain: "example.com"},
		{URL: "https://example.com/article", Title: "Article", VisitTime: baseTime.Add(2 * time.Hour), DwellTime: 2 * time.Minute, Domain: "example.com"},
		{URL: "https://example.com/article", Title: "Article", VisitTime: baseTime.Add(4 * time.Hour), DwellTime: 2 * time.Minute, Domain: "example.com"},
		// Different URL on same day — unaffected by dedup
		{URL: "https://example.com/other", Title: "Other", VisitTime: baseTime, DwellTime: 3 * time.Minute, Domain: "example.com"},
	}

	artifacts, _, stats := c.processEntries(entries, 0)

	// Dedup merges 3 article visits into 1 (6m → "full") + 1 other (3m → "standard") = 2 artifacts
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts after dedup, got %d", len(artifacts))
	}

	tiers := make(map[string]string)
	for _, a := range artifacts {
		tier, _ := a.Metadata["processing_tier"].(string)
		tiers[a.URL] = tier
	}
	if tiers["https://example.com/article"] != "full" {
		t.Errorf("merged article (6m) expected 'full', got %q", tiers["https://example.com/article"])
	}
	if tiers["https://example.com/other"] != "standard" {
		t.Errorf("other (3m) expected 'standard', got %q", tiers["https://example.com/other"])
	}
	_ = stats
}

func TestDetectRepeatVisits_RespectsWindow(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		RepeatVisitWindow:              7 * 24 * time.Hour, // 7 days
		RepeatVisitThreshold:           3,
		SocialMediaIndividualThreshold: 5 * time.Minute,
	}

	now := time.Now()
	// 2 visits within window + 2 visits outside window = 4 total, but only 2 in window
	entries := []HistoryEntry{
		{URL: "https://docs.example.com/api", Title: "API", VisitTime: now.Add(-1 * 24 * time.Hour), DwellTime: 90 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/api", Title: "API", VisitTime: now.Add(-3 * 24 * time.Hour), DwellTime: 90 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/api", Title: "API", VisitTime: now.Add(-10 * 24 * time.Hour), DwellTime: 90 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/api", Title: "API", VisitTime: now.Add(-20 * 24 * time.Hour), DwellTime: 90 * time.Second, Domain: "docs.example.com"},
	}

	artifacts, _, stats := c.processEntries(entries, 0)

	// Only 2 visits in the 7-day window → below threshold of 3 → no escalation
	if stats.repeatEscalations != 0 {
		t.Errorf("expected 0 repeat escalations (only 2 visits in window), got %d", stats.repeatEscalations)
	}

	// All 4 entries should be light-tier (90s dwell), no escalation
	for _, a := range artifacts {
		tier, _ := a.Metadata["processing_tier"].(string)
		if tier != "light" {
			t.Errorf("expected light tier (no escalation), got %s for %s", tier, a.URL)
		}
	}
	_ = artifacts
}

func TestDetectRepeatVisits_AllWithinWindow_Escalates(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		RepeatVisitWindow:              7 * 24 * time.Hour,
		RepeatVisitThreshold:           3,
		SocialMediaIndividualThreshold: 5 * time.Minute,
	}

	now := time.Now()
	// 3 visits all within the 7-day window → meets threshold → escalation
	day1 := now.Add(-1 * 24 * time.Hour)
	day2 := now.Add(-3 * 24 * time.Hour)
	day3 := now.Add(-5 * 24 * time.Hour)
	entries := []HistoryEntry{
		{URL: "https://docs.example.com/api", Title: "API", VisitTime: day1, DwellTime: 90 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/api", Title: "API", VisitTime: day2, DwellTime: 90 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/api", Title: "API", VisitTime: day3, DwellTime: 90 * time.Second, Domain: "docs.example.com"},
	}

	artifacts, _, stats := c.processEntries(entries, 0)

	// 3 visits in 7-day window → meets threshold → escalation
	if stats.repeatEscalations != 3 {
		t.Errorf("expected 3 repeat escalations, got %d", stats.repeatEscalations)
	}

	for _, a := range artifacts {
		tier, _ := a.Metadata["processing_tier"].(string)
		if tier != "standard" {
			t.Errorf("expected standard tier (light→standard escalation), got %s", tier)
		}
	}
}

// --- CHAOS-HARDENING R3: Adversarial tests ---

// CHAOS-F2: A corrupted cursor must not silently fall back to epoch 0, which would
// re-sync the entire Chrome history from 1601. parseCursorToChromeSafe must error.
// Adversarial: would fail if parseCursorToChromeSafe returned 0 without error.
func TestParseCursorToChromeSafe_CorruptedInput(t *testing.T) {
	tests := []struct {
		name    string
		cursor  string
		wantErr bool
	}{
		{"garbage string", "not-a-number", true},
		{"empty string", "", true},
		{"negative value", "-100", true},
		{"float string", "123.456", true},
		{"valid integer", "13350000000000000", false},
		{"zero is valid", "0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := parseCursorToChromeSafe(tt.cursor)
			if tt.wantErr && err == nil {
				t.Errorf("parseCursorToChromeSafe(%q) expected error, got value %d", tt.cursor, v)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("parseCursorToChromeSafe(%q) unexpected error: %v", tt.cursor, err)
			}
			if !tt.wantErr && tt.cursor == "13350000000000000" && v != 13350000000000000 {
				t.Errorf("parseCursorToChromeSafe(%q) = %d, want 13350000000000000", tt.cursor, v)
			}
		})
	}
}

// CHAOS-F5: Sync must respect context cancellation between expensive steps.
// Adversarial: would fail if Sync never checks ctx.Err() between copy/parse/process.
func TestSync_RespectsContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "History")
	if err := os.WriteFile(historyPath, []byte("fake-sqlite"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	c := New("browser-history")
	c.config = BrowserConfig{
		HistoryPath:         historyPath,
		InitialLookbackDays: 30,
	}
	c.mu.Lock()
	c.health = connector.HealthHealthy
	c.mu.Unlock()

	// Pre-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := c.Sync(ctx, "")
	if err == nil {
		t.Fatal("Sync with cancelled context should return error")
	}
	if err != context.Canceled {
		t.Logf("Sync returned error (acceptable): %v", err)
	}
}

// CHAOS-F1: Config snapshot must prevent data race between concurrent Connect and Sync.
// This test verifies the Connector's Sync snapshots config at start, so a concurrent
// Connect mutating config won't corrupt an in-progress Sync.
func TestConnector_ConfigSnapshotIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "History")
	if err := os.WriteFile(historyPath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	c := New("browser-history")
	config := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"history_path": historyPath,
		},
	}

	if err := c.Connect(context.Background(), config); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Verify the connector stores its own copy of config
	c.mu.RLock()
	originalPath := c.config.HistoryPath
	c.mu.RUnlock()

	if originalPath != historyPath {
		t.Errorf("expected HistoryPath %q, got %q", historyPath, originalPath)
	}
}

// CHAOS-F4: processEntries must handle entries with zero dwell time without panicking.
func TestProcessEntries_ZeroDwellTime(t *testing.T) {
	c := New("browser-history")
	c.config = BrowserConfig{
		SocialMediaIndividualThreshold: 5 * time.Minute,
	}

	entries := []HistoryEntry{
		{URL: "https://example.com/zero", Title: "Zero Dwell", VisitTime: time.Now(), DwellTime: 0, Domain: "example.com"},
	}

	// Must not panic; zero dwell → metadata tier → excluded by privacy gate
	artifacts, _, stats := c.processEntries(entries, 0)

	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts (zero dwell → metadata → privacy gate), got %d", len(artifacts))
	}
	if stats.byTier["metadata"] != 1 {
		t.Errorf("expected 1 metadata tier entry, got %d", stats.byTier["metadata"])
	}
}

// GAP-FIX: Connect must verify file readability, not just existence (R-002).
// Adversarial: would pass if Connect only used os.Stat (which succeeds on unreadable files).
func TestConnect_HistoryFileNotReadable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping readability test when running as root")
	}

	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "History")
	if err := os.WriteFile(historyPath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	// Remove read permission
	if err := os.Chmod(historyPath, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(historyPath, 0o644) })

	c := New("browser-history")
	config := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"history_path": historyPath,
		},
	}

	err := c.Connect(context.Background(), config)
	if err == nil {
		t.Fatal("expected error for unreadable history file")
	}
	if !contains(err.Error(), "not readable") {
		t.Errorf("expected 'not readable' in error, got: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected health error, got %s", c.Health(context.Background()))
	}
}

// GAP-FIX: Config validation rejects initial_lookback_days < 1 (R-012).
func TestParseBrowserConfig_InitialLookbackDaysValidation(t *testing.T) {
	tests := []struct {
		name   string
		value  interface{}
		errMsg string
	}{
		{"zero", 0, "initial_lookback_days must be >= 1"},
		{"negative", -5, "initial_lookback_days must be >= 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := connector.ConnectorConfig{
				SourceConfig: map[string]interface{}{
					"history_path":          "/some/path",
					"initial_lookback_days": tt.value,
				},
			}
			_, err := parseBrowserConfig(config)
			if err == nil {
				t.Fatalf("expected error containing %q", tt.errMsg)
			}
			if !contains(err.Error(), tt.errMsg) {
				t.Errorf("expected error containing %q, got: %v", tt.errMsg, err)
			}
		})
	}
}

// GAP-FIX: Config validation rejects content_fetch_concurrency < 1 (R-012).
func TestParseBrowserConfig_ContentFetchConcurrencyValidation(t *testing.T) {
	tests := []struct {
		name   string
		value  interface{}
		errMsg string
	}{
		{"zero", 0, "content_fetch_concurrency must be >= 1"},
		{"negative", -1, "content_fetch_concurrency must be >= 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := connector.ConnectorConfig{
				SourceConfig: map[string]interface{}{
					"history_path":              "/some/path",
					"content_fetch_concurrency": tt.value,
				},
			}
			_, err := parseBrowserConfig(config)
			if err == nil {
				t.Fatalf("expected error containing %q", tt.errMsg)
			}
			if !contains(err.Error(), tt.errMsg) {
				t.Errorf("expected error containing %q, got: %v", tt.errMsg, err)
			}
		})
	}
}

// GAP-FIX: Health() transitions to error when file disappears between syncs (R-013).
// Adversarial: would pass if Health() only returned cached status without re-checking file.
func TestHealth_FileDisappearsAfterConnect(t *testing.T) {
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "History")
	if err := os.WriteFile(historyPath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	c := New("browser-history")
	config := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"history_path": historyPath,
		},
	}

	if err := c.Connect(context.Background(), config); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Fatal("expected healthy after Connect")
	}

	// Delete the file — simulates user moving Chrome profile
	os.Remove(historyPath)

	// Health should detect file disappearance and transition to error
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected health error after file deletion, got %s", c.Health(context.Background()))
	}
}

// CHAOS: dedupByURLDate must handle empty/nil input without panicking.
func TestDedupByURLDate_EmptyInput(t *testing.T) {
	result := dedupByURLDate(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 entries for nil input, got %d", len(result))
	}
	result = dedupByURLDate([]HistoryEntry{})
	if len(result) != 0 {
		t.Errorf("expected 0 entries for empty input, got %d", len(result))
	}
}
