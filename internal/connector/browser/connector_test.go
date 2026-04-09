package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

func TestCopyHistoryFile_RetryOnFailure(t *testing.T) {
	// Create a temp file to simulate a Chrome History file
	tmpDir := t.TempDir()
	historyPath := filepath.Join(tmpDir, "History")
	if err := os.WriteFile(historyPath, []byte("fake-sqlite-data"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	c := New("browser-history")
	c.config = BrowserConfig{HistoryPath: historyPath}

	// Success case
	tmpPath, err := c.copyHistoryFile()
	if err != nil {
		t.Fatalf("copyHistoryFile failed: %v", err)
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
	c.config.HistoryPath = "/nonexistent/path/History"
	_, err = c.copyHistoryFile()
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
	// Test a known Chrome timestamp
	chromeTime := int64(13350000000000000)
	cursor := strconv.FormatInt(chromeTime, 10)

	parsed := parseCursorToChrome(cursor)
	if parsed != chromeTime {
		t.Errorf("expected %d, got %d", chromeTime, parsed)
	}

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
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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
			urls, ok := a.Metadata["urls"].([]string)
			if !ok {
				t.Fatal("urls not []string")
			}
			if len(urls) != 3 {
				t.Errorf("expected 3 reddit URLs, got %d", len(urls))
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

	baseTime := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	// URL repeated 5 times with 90s dwell (normally "light")
	entries := []HistoryEntry{
		{URL: "https://docs.example.com/api-ref", Title: "API Ref", VisitTime: baseTime, DwellTime: 90 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/api-ref", Title: "API Ref", VisitTime: baseTime.Add(time.Hour), DwellTime: 90 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/api-ref", Title: "API Ref", VisitTime: baseTime.Add(2 * time.Hour), DwellTime: 90 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/api-ref", Title: "API Ref", VisitTime: baseTime.Add(3 * time.Hour), DwellTime: 90 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/api-ref", Title: "API Ref", VisitTime: baseTime.Add(4 * time.Hour), DwellTime: 90 * time.Second, Domain: "docs.example.com"},
		// A unique URL for comparison
		{URL: "https://example.com/once", Title: "Once", VisitTime: baseTime, DwellTime: 90 * time.Second, Domain: "example.com"},
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
	urls, ok := agg.Metadata["urls"].([]string)
	if !ok {
		t.Fatal("urls not []string")
	}
	if len(urls) != 2 {
		t.Errorf("expected 2 URLs, got %d", len(urls))
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

	baseTime := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	// URL visited only 2 times (below threshold of 3)
	entries := []HistoryEntry{
		{URL: "https://example.com/page", Title: "Page", VisitTime: baseTime, DwellTime: 90 * time.Second, Domain: "example.com"},
		{URL: "https://example.com/page", Title: "Page", VisitTime: baseTime.Add(time.Hour), DwellTime: 90 * time.Second, Domain: "example.com"},
		{URL: "https://example.com/other", Title: "Other", VisitTime: baseTime, DwellTime: 45 * time.Second, Domain: "example.com"},
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

	baseTime := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	// URL visited 4 times with 10s dwell (normally "metadata" → privacy gate excludes)
	// But repeat escalation should bump metadata→light, which survives the gate
	entries := []HistoryEntry{
		{URL: "https://docs.example.com/faq", Title: "FAQ", VisitTime: baseTime, DwellTime: 10 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/faq", Title: "FAQ", VisitTime: baseTime.Add(time.Hour), DwellTime: 10 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/faq", Title: "FAQ", VisitTime: baseTime.Add(2 * time.Hour), DwellTime: 10 * time.Second, Domain: "docs.example.com"},
		{URL: "https://docs.example.com/faq", Title: "FAQ", VisitTime: baseTime.Add(3 * time.Hour), DwellTime: 10 * time.Second, Domain: "docs.example.com"},
		// Control: single-visit metadata-tier URL should be excluded by privacy gate
		{URL: "https://clickbait.com/bait", Title: "Bait", VisitTime: baseTime, DwellTime: 5 * time.Second, Domain: "clickbait.com"},
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

func TestParseCursorToChrome_BadInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{"garbage string returns 0", "not-a-number", 0},
		{"empty string returns 0", "", 0},
		{"float string returns 0", "123.456", 0},
		{"valid integer", "13350000000000000", 13350000000000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCursorToChrome(tt.input)
			if got != tt.expected {
				t.Errorf("parseCursorToChrome(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
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
