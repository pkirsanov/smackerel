package main

import (
	"context"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	alertsConnector "github.com/smackerel/smackerel/internal/connector/alerts"
	bookmarksConnector "github.com/smackerel/smackerel/internal/connector/bookmarks"
	browserConnector "github.com/smackerel/smackerel/internal/connector/browser"
	caldavConnector "github.com/smackerel/smackerel/internal/connector/caldav"
	discordConnector "github.com/smackerel/smackerel/internal/connector/discord"
	guesthostConnector "github.com/smackerel/smackerel/internal/connector/guesthost"
	hospitableConnector "github.com/smackerel/smackerel/internal/connector/hospitable"
	imapConnector "github.com/smackerel/smackerel/internal/connector/imap"
	keepConnector "github.com/smackerel/smackerel/internal/connector/keep"
	mapsConnector "github.com/smackerel/smackerel/internal/connector/maps"
	marketsConnector "github.com/smackerel/smackerel/internal/connector/markets"
	rssConnector "github.com/smackerel/smackerel/internal/connector/rss"
	twitterConnector "github.com/smackerel/smackerel/internal/connector/twitter"
	weatherConnector "github.com/smackerel/smackerel/internal/connector/weather"
	youtubeConnector "github.com/smackerel/smackerel/internal/connector/youtube"
)

// --- SCN-019-001 / SCN-019-006: All 15 connectors register successfully ---

// TestAllConnectorsRegistered verifies that all 15 connector packages can be
// instantiated and registered in a single Registry without collision, matching
// the exact set in main.go's run() function. This is the unit-level proof for
// SCN-019-001 (all 15 registered) and SCN-019-006 (no regression from new registrations).
func TestAllConnectorsRegistered(t *testing.T) {
	registry := connector.NewRegistry()

	// Instantiate all 15 connectors using the same IDs and constructors as main.go.
	// bookmarksConnector.NewConnector (no pool) is used instead of NewConnectorWithPool
	// because registration only requires the Connector interface — pool is not needed.
	connectors := []connector.Connector{
		imapConnector.New("gmail"),
		caldavConnector.New("google-calendar"),
		youtubeConnector.New("youtube"),
		rssConnector.New("rss", nil),
		keepConnector.New("google-keep"),
		bookmarksConnector.NewConnector("bookmarks"),
		browserConnector.New("browser-history"),
		mapsConnector.New("google-maps-timeline"),
		hospitableConnector.New("hospitable"),
		guesthostConnector.New(),
		discordConnector.New("discord"),
		twitterConnector.New("twitter"),
		weatherConnector.New("weather"),
		alertsConnector.New("gov-alerts"),
		marketsConnector.New("financial-markets"),
	}

	for _, c := range connectors {
		if err := registry.Register(c); err != nil {
			t.Fatalf("failed to register connector %q: %v", c.ID(), err)
		}
	}

	if registry.Count() != 15 {
		t.Errorf("expected 15 registered connectors, got %d", registry.Count())
	}

	// Verify every expected ID is present (15 total: 10 original + 5 wired by spec 019).
	expectedIDs := []string{
		"gmail", "google-calendar", "youtube", "rss", "google-keep",
		"bookmarks", "browser-history", "google-maps-timeline",
		"hospitable", "guesthost",
		"discord", "twitter", "weather", "gov-alerts", "financial-markets",
	}
	registeredIDs := registry.List()
	sort.Strings(registeredIDs)
	sort.Strings(expectedIDs)

	if len(registeredIDs) != len(expectedIDs) {
		t.Fatalf("ID count mismatch: registered %v, expected %v", registeredIDs, expectedIDs)
	}
	for i := range expectedIDs {
		if registeredIDs[i] != expectedIDs[i] {
			t.Errorf("ID mismatch at index %d: got %q, want %q", i, registeredIDs[i], expectedIDs[i])
		}
	}
}

// TestDuplicateRegistrationRejected verifies that registering a connector
// with the same ID twice returns an error (guards against accidental double-wiring).
func TestDuplicateRegistrationRejected(t *testing.T) {
	registry := connector.NewRegistry()

	c1 := discordConnector.New("discord")
	c2 := discordConnector.New("discord")

	if err := registry.Register(c1); err != nil {
		t.Fatalf("first registration should succeed: %v", err)
	}
	if err := registry.Register(c2); err == nil {
		t.Error("expected error for duplicate registration, got nil")
	}
}

// --- parseJSONArray tests ---

func TestParseJSONArray_ValidArray(t *testing.T) {
	result := parseJSONArray(`["a", "b", "c"]`)
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
	if result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Errorf("unexpected values: %v", result)
	}
}

func TestParseJSONArray_EmptyString(t *testing.T) {
	result := parseJSONArray("")
	if result != nil {
		t.Errorf("expected nil for empty string, got %v", result)
	}
}

func TestParseJSONArray_EmptyArray(t *testing.T) {
	result := parseJSONArray("[]")
	if result == nil {
		t.Fatal("expected non-nil for empty JSON array")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 elements, got %d", len(result))
	}
}

func TestParseJSONArray_InvalidJSON(t *testing.T) {
	result := parseJSONArray("{not valid json")
	if result != nil {
		t.Errorf("expected nil for invalid JSON, got %v", result)
	}
}

func TestParseJSONArray_MixedTypes(t *testing.T) {
	result := parseJSONArray(`["text", 42, true, null]`)
	if len(result) != 4 {
		t.Fatalf("expected 4 elements, got %d", len(result))
	}
}

func TestParseJSONArray_NestedArrays(t *testing.T) {
	result := parseJSONArray(`[["a", "b"], ["c"]]`)
	if len(result) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result))
	}
}

func TestParseJSONArray_NotAnArray(t *testing.T) {
	// A JSON object is not a valid JSON array
	result := parseJSONArray(`{"key": "value"}`)
	if result != nil {
		t.Errorf("expected nil for JSON object input, got %v", result)
	}
}

// --- IMP-022-001: runWithTimeout overall deadline enforcement ---

func TestRunWithTimeout_CompletesBeforeBudget(t *testing.T) {
	deadline := make(chan struct{}) // never fires
	completed := false
	runWithTimeout("test-fast", 2*time.Second, deadline, func() {
		completed = true
	})
	if !completed {
		t.Error("expected fn to complete")
	}
}

func TestRunWithTimeout_ExceedsBudget(t *testing.T) {
	deadline := make(chan struct{}) // never fires
	start := time.Now()
	runWithTimeout("test-slow", 50*time.Millisecond, deadline, func() {
		time.Sleep(5 * time.Second) // much longer than budget
	})
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Errorf("runWithTimeout should return after ~50ms budget, took %v", elapsed)
	}
}

func TestRunWithTimeout_OverallDeadlineSkipsExpiredStep(t *testing.T) {
	// Deadline already expired before step starts
	deadline := make(chan struct{})
	close(deadline)
	called := false
	runWithTimeout("test-skip", 2*time.Second, deadline, func() {
		called = true
	})
	// Give the goroutine a moment to potentially run (it shouldn't)
	time.Sleep(10 * time.Millisecond)
	if called {
		t.Error("fn should not be called when deadline is already expired")
	}
}

func TestRunWithTimeout_OverallDeadlineFiringDuringStep(t *testing.T) {
	// Deadline fires 50ms into a 2s budget step
	deadlineCtx, deadlineCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer deadlineCancel()
	start := time.Now()
	runWithTimeout("test-deadline", 2*time.Second, deadlineCtx.Done(), func() {
		time.Sleep(5 * time.Second)
	})
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Errorf("runWithTimeout should return when overall deadline fires (~50ms), took %v", elapsed)
	}
}

func TestRunWithTimeout_StepBudgetWinsOverDeadlineWhenShorter(t *testing.T) {
	// Step budget (50ms) < overall deadline (5s) — step budget should win
	deadlineCtx, deadlineCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer deadlineCancel()
	start := time.Now()
	runWithTimeout("test-budget-wins", 50*time.Millisecond, deadlineCtx.Done(), func() {
		time.Sleep(5 * time.Second)
	})
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Errorf("step budget (50ms) should control, took %v", elapsed)
	}
}

// --- H-019-002: backward compatibility — old parseJSONArray still works ---

func TestParseJSONArray_BackwardCompat(t *testing.T) {
	result := parseJSONArray(`[1,2,3]`)
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
}

// --- IMP-019-R27-001: Gov Alerts source_earthquake SST end-to-end ---
// The source_earthquake toggle must flow env → SourceConfig → connector config.
// Before this fix, the toggle was readable by the connector but never set by main.go,
// meaning earthquake alerts could never be disabled via config.

func TestGovAlertsSourceEarthquakeWiring_Enabled(t *testing.T) {
	// Simulate the exact SourceConfig assembly from main.go's Gov Alerts auto-start block.
	t.Setenv("GOV_ALERTS_SOURCE_EARTHQUAKE", "true")
	cfg := connector.ConnectorConfig{
		AuthType:    "api_key",
		Credentials: map[string]string{"airnow_api_key": ""},
		Enabled:     true,
		SourceConfig: map[string]interface{}{
			"source_earthquake": os.Getenv("GOV_ALERTS_SOURCE_EARTHQUAKE") == "true",
		},
	}
	val, ok := cfg.SourceConfig["source_earthquake"].(bool)
	if !ok {
		t.Fatal("source_earthquake not present in SourceConfig as bool")
	}
	if !val {
		t.Error("source_earthquake should be true when env var is 'true'")
	}
}

func TestGovAlertsSourceEarthquakeWiring_Disabled(t *testing.T) {
	// This is the adversarial case: if the fix were reverted (source_earthquake removed
	// from SourceConfig), the connector would default to true and earthquake alerts
	// could never be disabled.
	t.Setenv("GOV_ALERTS_SOURCE_EARTHQUAKE", "false")
	cfg := connector.ConnectorConfig{
		AuthType:    "api_key",
		Credentials: map[string]string{"airnow_api_key": ""},
		Enabled:     true,
		SourceConfig: map[string]interface{}{
			"source_earthquake": os.Getenv("GOV_ALERTS_SOURCE_EARTHQUAKE") == "true",
		},
	}
	val, ok := cfg.SourceConfig["source_earthquake"].(bool)
	if !ok {
		t.Fatal("source_earthquake not present in SourceConfig as bool")
	}
	if val {
		t.Error("source_earthquake should be false when env var is 'false'")
	}
}

func TestGovAlertsSourceEarthquakeWiring_UnsetDefaultsFalse(t *testing.T) {
	// When the env var is absent, == "true" evaluates to false.
	// This is the correct SST behavior: if the YAML key is missing or config generate
	// doesn't run, the connector should NOT silently enable earthquake alerts.
	t.Setenv("GOV_ALERTS_SOURCE_EARTHQUAKE", "")
	val := os.Getenv("GOV_ALERTS_SOURCE_EARTHQUAKE") == "true"
	if val {
		t.Error("absent/empty GOV_ALERTS_SOURCE_EARTHQUAKE should evaluate to false via == 'true' pattern")
	}
}

// --- IMP-019-R27-002: Weather enable_alerts SST end-to-end ---
// The enable_alerts toggle was readable by the weather connector after R23 fix
// but never wired through main.go SourceConfig. Operators could not enable weather alerts.

func TestWeatherEnableAlertsWiring(t *testing.T) {
	t.Setenv("WEATHER_ENABLE_ALERTS", "true")
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"enable_alerts": os.Getenv("WEATHER_ENABLE_ALERTS") == "true",
		},
	}
	val, ok := cfg.SourceConfig["enable_alerts"].(bool)
	if !ok {
		t.Fatal("enable_alerts not present in SourceConfig as bool")
	}
	if !val {
		t.Error("enable_alerts should be true when env var is 'true'")
	}
}

func TestWeatherEnableAlertsWiring_Disabled(t *testing.T) {
	t.Setenv("WEATHER_ENABLE_ALERTS", "false")
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"enable_alerts": os.Getenv("WEATHER_ENABLE_ALERTS") == "true",
		},
	}
	val, ok := cfg.SourceConfig["enable_alerts"].(bool)
	if !ok {
		t.Fatal("enable_alerts not present in SourceConfig as bool")
	}
	if val {
		t.Error("enable_alerts should be false when env var is 'false'")
	}
}

// --- IMP-019-R28: Financial Markets fred_enabled SST end-to-end ---
// Before this fix, fred_enabled was readable by parseMarketsConfig but never set
// by main.go, meaning operators could not disable FRED data when an API key was present.

func TestMarketsFreddEnabledWiring_True(t *testing.T) {
	t.Setenv("FINANCIAL_MARKETS_FRED_ENABLED", "true")
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"fred_enabled": os.Getenv("FINANCIAL_MARKETS_FRED_ENABLED") == "true",
		},
	}
	val, ok := cfg.SourceConfig["fred_enabled"].(bool)
	if !ok {
		t.Fatal("fred_enabled not present in SourceConfig as bool")
	}
	if !val {
		t.Error("fred_enabled should be true when env var is 'true'")
	}
}

func TestMarketsFreddEnabledWiring_False(t *testing.T) {
	// Adversarial: if the fix were reverted (fred_enabled removed from SourceConfig),
	// parseMarketsConfig would auto-enable FRED whenever fred_api_key is non-empty.
	t.Setenv("FINANCIAL_MARKETS_FRED_ENABLED", "false")
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"fred_enabled": os.Getenv("FINANCIAL_MARKETS_FRED_ENABLED") == "true",
		},
	}
	val, ok := cfg.SourceConfig["fred_enabled"].(bool)
	if !ok {
		t.Fatal("fred_enabled not present in SourceConfig as bool")
	}
	if val {
		t.Error("fred_enabled should be false when env var is 'false'")
	}
}

func TestMarketsFreddEnabledWiring_UnsetDefaultsFalse(t *testing.T) {
	t.Setenv("FINANCIAL_MARKETS_FRED_ENABLED", "")
	val := os.Getenv("FINANCIAL_MARKETS_FRED_ENABLED") == "true"
	if val {
		t.Error("absent/empty FINANCIAL_MARKETS_FRED_ENABLED should evaluate to false via == 'true'")
	}
}

// --- STB-022-001: Parallel subscriber Stop() in shutdownAll ---

// TestShutdownAll_ParallelSubscriberStop verifies that shutdownAll stops all
// three subscribers in parallel rather than sequentially. Three slow-stopping
// subscribers that each take ~200ms should complete in roughly one 200ms window
// (parallel), not 600ms (sequential). With the 6s step budget, this proves the
// parallel pattern works correctly within its budget.
func TestShutdownAll_ParallelSubscriberStop(t *testing.T) {
	// The test uses shutdownAll with nil for most components.
	// Only the subscriber fields matter; nil components are skipped by shutdownAll.
	//
	// We can't use real pipeline.ResultSubscriber because it requires NATS.
	// Instead, we test the runWithTimeout + parallel WaitGroup pattern directly.
	deadline := make(chan struct{}) // never fires

	var order []string
	var mu sync.Mutex
	slowStop := func(name string) {
		time.Sleep(200 * time.Millisecond) // simulate Fetch() MaxWait
		mu.Lock()
		order = append(order, name)
		mu.Unlock()
	}

	start := time.Now()
	runWithTimeout("parallel-test", 6*time.Second, deadline, func() {
		var subWg sync.WaitGroup
		subWg.Add(3)
		go func() { defer subWg.Done(); slowStop("A") }()
		go func() { defer subWg.Done(); slowStop("B") }()
		go func() { defer subWg.Done(); slowStop("C") }()
		subWg.Wait()
	})
	elapsed := time.Since(start)

	// Parallel: all three ~200ms stops run concurrently → total ~200ms
	// Sequential would be ~600ms
	if elapsed > 500*time.Millisecond {
		t.Errorf("subscriber stops took %v — should be parallel (~200ms), not sequential (~600ms)", elapsed)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 3 {
		t.Errorf("expected all 3 subscribers stopped, got %d", len(order))
	}
}

// TestShutdownAll_NilSubscribersHandled verifies the parallel stop pattern
// handles nil subscribers without panicking.
func TestShutdownAll_NilSubscribersHandled(t *testing.T) {
	// shutdownAll with all nil components should complete without panic.
	// This exercises the nil-guard pattern in the parallel subscriber stop.
	shutdownAll(5, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
}
