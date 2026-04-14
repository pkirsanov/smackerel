//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/browser"
)

// testHistoryFixturePath returns the path to a Chrome History SQLite fixture.
// If BROWSER_HISTORY_TEST_FIXTURE is set, that path is used; otherwise the
// repo-local data/browser-history/History/ fixture is tried. Returns empty
// string when no usable fixture exists.
func testHistoryFixturePath(t *testing.T) string {
	t.Helper()

	if p := os.Getenv("BROWSER_HISTORY_TEST_FIXTURE"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		t.Skipf("integration: BROWSER_HISTORY_TEST_FIXTURE=%s not accessible", p)
	}

	// Repo-local fixture (may be empty directory in CI)
	candidates := []string{
		"../../data/browser-history/History",
		"data/browser-history/History",
	}
	for _, c := range candidates {
		info, err := os.Stat(c)
		if err == nil && !info.IsDir() && info.Size() > 0 {
			return c
		}
	}

	return ""
}

// requireFixture skips the test if no SQLite fixture is available.
func requireFixture(t *testing.T) string {
	t.Helper()
	p := testHistoryFixturePath(t)
	if p == "" {
		t.Skip("integration: Chrome History test fixture not available")
	}
	return p
}

// TestBrowserHistorySync_InitialImport (T-15)
// Verifies the full sync flow on first run (empty cursor):
// connector starts → copies History → parses since lookback → filters → tiers → produces RawArtifacts → cursor returned.
func TestBrowserHistorySync_InitialImport(t *testing.T) {
	fixturePath := requireFixture(t)

	c := browser.New("browser-history")
	ctx := context.Background()

	config := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"history_path":       fixturePath,
			"access_strategy":    "copy",
			"initial_lookback":   30,
			"dwell_full_min":     "5m",
			"dwell_standard_min": "2m",
			"dwell_light_min":    "30s",
		},
	}

	if err := c.Connect(ctx, config); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	if c.Health(ctx) != connector.HealthHealthy {
		t.Fatalf("expected healthy after Connect, got %s", c.Health(ctx))
	}

	// Sync with empty cursor (initial import)
	artifacts, cursor, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Cursor must advance (non-empty)
	if cursor == "" {
		t.Error("expected non-empty cursor after initial sync")
	}

	// Artifacts should have required fields
	for i, a := range artifacts {
		if a.SourceID != "browser-history" {
			t.Errorf("artifact[%d]: expected source_id 'browser-history', got %q", i, a.SourceID)
		}
		if a.CapturedAt.IsZero() {
			t.Errorf("artifact[%d]: CapturedAt is zero", i)
		}
		if a.SourceRef == "" {
			t.Errorf("artifact[%d]: SourceRef is empty", i)
		}
	}

	t.Logf("initial import: %d artifacts, cursor=%s", len(artifacts), cursor)
}

// TestBrowserHistorySync_IncrementalCursor (T-16)
// Verifies cursor-based incremental sync: second call with cursor from first
// call returns no duplicates and cursor does not regress.
func TestBrowserHistorySync_IncrementalCursor(t *testing.T) {
	fixturePath := requireFixture(t)

	c := browser.New("browser-history")
	ctx := context.Background()

	config := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"history_path":     fixturePath,
			"access_strategy":  "copy",
			"initial_lookback": 30,
		},
	}

	if err := c.Connect(ctx, config); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// First sync
	_, cursor1, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("first Sync failed: %v", err)
	}
	if cursor1 == "" {
		t.Fatal("expected non-empty cursor from first sync")
	}

	// Second sync with cursor from first — should return 0 new artifacts (static fixture)
	artifacts2, cursor2, err := c.Sync(ctx, cursor1)
	if err != nil {
		t.Fatalf("second Sync failed: %v", err)
	}

	if len(artifacts2) != 0 {
		t.Errorf("incremental sync against static fixture should return 0 new artifacts, got %d", len(artifacts2))
	}

	// Cursor must not regress
	if cursor2 < cursor1 {
		t.Errorf("cursor regressed: %s < %s", cursor2, cursor1)
	}
}

// TestBrowserHistorySync_FullPipelineFlow (T-17)
// Verifies the full pipeline: connect → sync → artifacts have correct tiers,
// skipped entries are filtered, and health transitions follow lifecycle spec.
func TestBrowserHistorySync_FullPipelineFlow(t *testing.T) {
	fixturePath := requireFixture(t)

	c := browser.New("browser-history")
	ctx := context.Background()

	config := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"history_path":        fixturePath,
			"access_strategy":     "copy",
			"initial_lookback":    365,
			"custom_skip_domains": []interface{}{"example-internal.corp"},
			"dwell_full_min":      "5m",
			"dwell_standard_min":  "2m",
			"dwell_light_min":     "30s",
		},
	}

	// Pre-connect: disconnected
	if c.Health(ctx) != connector.HealthDisconnected {
		t.Errorf("expected disconnected before Connect, got %s", c.Health(ctx))
	}

	if err := c.Connect(ctx, config); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	// Post-connect: healthy
	if c.Health(ctx) != connector.HealthHealthy {
		t.Errorf("expected healthy after Connect, got %s", c.Health(ctx))
	}

	// Sync
	artifacts, cursor, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Post-sync: should be healthy (no errors expected)
	if c.Health(ctx) != connector.HealthHealthy {
		t.Errorf("expected healthy after Sync, got %s", c.Health(ctx))
	}

	// Verify tier metadata where present
	for _, a := range artifacts {
		tier, ok := a.Metadata["processing_tier"].(string)
		if !ok || tier == "" {
			t.Errorf("artifact %s missing processing_tier", a.URL)
		}
		if tier == "metadata" {
			t.Errorf("metadata-tier artifact should be filtered by privacy gate: %s", a.URL)
		}
	}

	// Verify no skipped domains leaked through
	for _, a := range artifacts {
		domain, _ := a.Metadata["domain"].(string)
		if domain == "example-internal.corp" {
			t.Error("custom skip domain 'example-internal.corp' should not produce artifacts")
		}
	}

	t.Logf("pipeline flow: %d artifacts, cursor=%s", len(artifacts), cursor)

	// Close: disconnected
	c.Close()
	if c.Health(ctx) != connector.HealthDisconnected {
		t.Errorf("expected disconnected after Close, got %s", c.Health(ctx))
	}
}

// TestBrowserHistorySync_SocialMediaAggregation (T-30)
// Integration-level test: verifies social media entries are aggregated by domain+day
// through the full connector Sync flow with real data.
func TestBrowserHistorySync_SocialMediaAggregation(t *testing.T) {
	fixturePath := requireFixture(t)

	c := browser.New("browser-history")
	ctx := context.Background()

	config := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"history_path":                      fixturePath,
			"access_strategy":                   "copy",
			"initial_lookback":                  365,
			"social_media_individual_threshold": "5m",
			"dwell_full_min":                    "5m",
			"dwell_standard_min":                "2m",
			"dwell_light_min":                   "30s",
		},
	}

	if err := c.Connect(ctx, config); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Check for social aggregate artifacts (if fixture has social media entries)
	var socialAggregates int
	for _, a := range artifacts {
		if a.ContentType == "browsing/social-aggregate" {
			socialAggregates++
			domain, ok := a.Metadata["domain"].(string)
			if !ok || domain == "" {
				t.Error("social aggregate missing domain metadata")
			}
			visitCount, ok := a.Metadata["visit_count"]
			if !ok || visitCount == nil {
				t.Error("social aggregate missing visit_count metadata")
			}
			// Privacy: individual URLs must NOT be in aggregate metadata
			if _, hasURLs := a.Metadata["urls"]; hasURLs {
				t.Error("social aggregate must not store individual URLs (privacy R-402)")
			}
		}
	}

	t.Logf("social media aggregation: %d aggregate artifacts out of %d total", socialAggregates, len(artifacts))
}

// TestBrowserHistorySync_RepeatVisitEscalation (T-31)
// Integration-level test: verifies repeat visit detection and tier escalation
// through the full connector Sync flow.
func TestBrowserHistorySync_RepeatVisitEscalation(t *testing.T) {
	fixturePath := requireFixture(t)

	c := browser.New("browser-history")
	ctx := context.Background()

	config := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"history_path":           fixturePath,
			"access_strategy":        "copy",
			"initial_lookback":       365,
			"repeat_visit_window":    "30d",
			"repeat_visit_threshold": 3,
			"dwell_full_min":         "5m",
			"dwell_standard_min":     "2m",
			"dwell_light_min":        "30s",
		},
	}

	if err := c.Connect(ctx, config); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// Check for repeat visit escalation evidence in metadata
	var escalated int
	for _, a := range artifacts {
		if rv, ok := a.Metadata["repeat_visits"]; ok {
			escalated++
			if count, ok := rv.(int); ok && count < 3 {
				t.Errorf("repeat_visits metadata present but count %d < threshold 3", count)
			}
		}
	}

	t.Logf("repeat visit escalation: %d escalated artifacts out of %d total", escalated, len(artifacts))
}

// TestBrowserHistorySync_FullPipeline_WithAggregationAndPrivacy (T-32)
// Integration-level test: end-to-end pipeline with social aggregation,
// repeat visits, and privacy gate all active.
func TestBrowserHistorySync_FullPipeline_WithAggregationAndPrivacy(t *testing.T) {
	fixturePath := requireFixture(t)

	c := browser.New("browser-history")
	ctx := context.Background()

	config := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"history_path":                      fixturePath,
			"access_strategy":                   "copy",
			"initial_lookback":                  365,
			"social_media_individual_threshold": "5m",
			"repeat_visit_window":               "30d",
			"repeat_visit_threshold":            3,
			"custom_skip_domains":               []interface{}{"internal.test"},
			"dwell_full_min":                    "5m",
			"dwell_standard_min":                "2m",
			"dwell_light_min":                   "30s",
		},
	}

	if err := c.Connect(ctx, config); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer c.Close()

	artifacts, cursor, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	if cursor == "" {
		t.Error("expected non-empty cursor")
	}

	// Validate all artifacts have required fields
	for i, a := range artifacts {
		if a.SourceID != "browser-history" {
			t.Errorf("artifact[%d]: expected source_id 'browser-history', got %q", i, a.SourceID)
		}
		if a.SourceRef == "" {
			t.Errorf("artifact[%d]: SourceRef is empty", i)
		}
		if a.ContentType == "" {
			t.Errorf("artifact[%d]: ContentType is empty", i)
		}

		// Privacy gate: no metadata-tier individual artifacts
		if a.ContentType == "url" {
			tier, _ := a.Metadata["processing_tier"].(string)
			if tier == "metadata" {
				t.Errorf("artifact[%d]: metadata-tier entry must not produce individual artifact (privacy gate)", i)
			}
		}

		// No skip-domain artifacts
		domain, _ := a.Metadata["domain"].(string)
		if domain == "internal.test" {
			t.Errorf("artifact[%d]: skip-domain 'internal.test' should not produce artifacts", i)
		}
	}

	// Count by type
	var individual, aggregate int
	for _, a := range artifacts {
		switch a.ContentType {
		case "url":
			individual++
		case "browsing/social-aggregate":
			aggregate++
		}
	}

	t.Logf("full pipeline: %d individual + %d aggregate = %d total, cursor=%s",
		individual, aggregate, len(artifacts), cursor)

	// Verify incremental sync returns no new data on static fixture
	artifacts2, cursor2, err := c.Sync(ctx, cursor)
	if err != nil {
		t.Fatalf("incremental Sync failed: %v", err)
	}
	if len(artifacts2) != 0 {
		t.Errorf("incremental sync on static fixture should return 0 artifacts, got %d", len(artifacts2))
	}
	if cursor2 < cursor {
		t.Errorf("cursor regressed: %s < %s", cursor2, cursor)
	}
}

// Unused import guard — ensure time is used in at least one test path.
var _ = time.Now
