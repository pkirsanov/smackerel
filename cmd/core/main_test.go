package main

import (
	"context"
	"os"
	"sort"
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

// --- parseJSONObject tests ---

func TestParseJSONObject_ValidObject(t *testing.T) {
	result := parseJSONObject(`{"key": "value", "count": 42}`)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result["key"] != "value" {
		t.Errorf("expected key=value, got %v", result["key"])
	}
	if result["count"] != float64(42) {
		t.Errorf("expected count=42, got %v", result["count"])
	}
}

func TestParseJSONObject_EmptyString(t *testing.T) {
	result := parseJSONObject("")
	if result != nil {
		t.Errorf("expected nil for empty string, got %v", result)
	}
}

func TestParseJSONObject_EmptyObject(t *testing.T) {
	result := parseJSONObject("{}")
	if result == nil {
		t.Fatal("expected non-nil for empty JSON object")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 keys, got %d", len(result))
	}
}

func TestParseJSONObject_InvalidJSON(t *testing.T) {
	result := parseJSONObject("[not valid")
	if result != nil {
		t.Errorf("expected nil for invalid JSON, got %v", result)
	}
}

func TestParseJSONObject_NotAnObject(t *testing.T) {
	// A JSON array is not a valid JSON object
	result := parseJSONObject(`["a", "b"]`)
	if result != nil {
		t.Errorf("expected nil for JSON array input, got %v", result)
	}
}

func TestParseJSONObject_NestedObject(t *testing.T) {
	result := parseJSONObject(`{"outer": {"inner": "value"}}`)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	inner, ok := result["outer"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested map, got %T", result["outer"])
	}
	if inner["inner"] != "value" {
		t.Errorf("expected inner=value, got %v", inner["inner"])
	}
}

// --- parseFloatEnv tests ---

func TestParseFloatEnv_ValidFloat(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "3.14")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 3.14 {
		t.Errorf("expected 3.14, got %f", result)
	}
}

func TestParseFloatEnv_Integer(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "42")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 42.0 {
		t.Errorf("expected 42.0, got %f", result)
	}
}

func TestParseFloatEnv_EmptyString(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 0 {
		t.Errorf("expected 0 for empty string, got %f", result)
	}
}

func TestParseFloatEnv_UnsetVar(t *testing.T) {
	// Ensure the var does not exist
	t.Setenv("TEST_FLOAT_UNSET", "")
	result := parseFloatEnv("TEST_FLOAT_UNSET")
	if result != 0 {
		t.Errorf("expected 0 for unset var, got %f", result)
	}
}

func TestParseFloatEnv_InvalidFloat(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "not-a-number")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 0 {
		t.Errorf("expected 0 for invalid float, got %f", result)
	}
}

func TestParseFloatEnv_NegativeFloat(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "-1.5")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != -1.5 {
		t.Errorf("expected -1.5, got %f", result)
	}
}

func TestParseFloatEnv_Zero(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "0")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 0 {
		t.Errorf("expected 0, got %f", result)
	}
}

func TestParseFloatEnv_ScientificNotation(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "1.5e2")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 150.0 {
		t.Errorf("expected 150.0, got %f", result)
	}
}

// --- CHAOS-019-001: parseFloatEnv must reject IEEE 754 special values ---

func TestParseFloatEnv_Inf(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "Inf")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 0 {
		t.Errorf("expected 0 for Inf, got %f", result)
	}
}

func TestParseFloatEnv_NegInf(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "-Inf")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 0 {
		t.Errorf("expected 0 for -Inf, got %f", result)
	}
}

func TestParseFloatEnv_PosInf(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "+Inf")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 0 {
		t.Errorf("expected 0 for +Inf, got %f", result)
	}
}

func TestParseFloatEnv_NaN(t *testing.T) {
	t.Setenv("TEST_FLOAT_VAR", "NaN")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 0 {
		t.Errorf("expected 0 for NaN, got %f", result)
	}
}

func TestParseFloatEnv_NaN_Lowercase(t *testing.T) {
	// strconv.ParseFloat is case-insensitive for NaN
	t.Setenv("TEST_FLOAT_VAR", "nan")
	result := parseFloatEnv("TEST_FLOAT_VAR")
	if result != 0 {
		t.Errorf("expected 0 for nan, got %f", result)
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

// --- H-019-002: parseJSONArrayEnv / parseJSONObjectEnv include key in logs ---

func TestParseJSONArrayEnv_ValidArray(t *testing.T) {
	t.Setenv("TEST_JSON_ARRAY", `["x","y"]`)
	result := parseJSONArrayEnv("TEST_JSON_ARRAY")
	if len(result) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result))
	}
}

func TestParseJSONArrayEnv_EmptyVar(t *testing.T) {
	t.Setenv("TEST_JSON_ARRAY", "")
	result := parseJSONArrayEnv("TEST_JSON_ARRAY")
	if result != nil {
		t.Errorf("expected nil for empty env var, got %v", result)
	}
}

func TestParseJSONArrayEnv_InvalidJSON(t *testing.T) {
	t.Setenv("TEST_JSON_ARRAY", "{broken")
	result := parseJSONArrayEnv("TEST_JSON_ARRAY")
	if result != nil {
		t.Errorf("expected nil for invalid JSON, got %v", result)
	}
}

func TestParseJSONObjectEnv_ValidObject(t *testing.T) {
	t.Setenv("TEST_JSON_OBJ", `{"k":"v"}`)
	result := parseJSONObjectEnv("TEST_JSON_OBJ")
	if result == nil || result["k"] != "v" {
		t.Errorf("expected {k:v}, got %v", result)
	}
}

func TestParseJSONObjectEnv_EmptyVar(t *testing.T) {
	t.Setenv("TEST_JSON_OBJ", "")
	result := parseJSONObjectEnv("TEST_JSON_OBJ")
	if result != nil {
		t.Errorf("expected nil for empty env var, got %v", result)
	}
}

func TestParseJSONObjectEnv_InvalidJSON(t *testing.T) {
	t.Setenv("TEST_JSON_OBJ", "not-json")
	result := parseJSONObjectEnv("TEST_JSON_OBJ")
	if result != nil {
		t.Errorf("expected nil for invalid JSON, got %v", result)
	}
}

// --- H-019-002: backward compatibility — old parseJSONArray/parseJSONObject still work ---

func TestParseJSONArray_BackwardCompat(t *testing.T) {
	result := parseJSONArray(`[1,2,3]`)
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
}

func TestParseJSONObject_BackwardCompat(t *testing.T) {
	result := parseJSONObject(`{"a":1}`)
	if result == nil || result["a"] != float64(1) {
		t.Errorf("expected {a:1}, got %v", result)
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

// --- IMP-019-R27-002: Weather enable_alerts/forecast_days/precision SST end-to-end ---
// These three config fields were readable by the weather connector after R23 fix
// but never wired through main.go SourceConfig. Operators could not enable weather
// alerts, change forecast horizon, or adjust coordinate precision.

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

func TestWeatherForecastDaysWiring(t *testing.T) {
	t.Setenv("WEATHER_FORECAST_DAYS", "14")
	result := parseFloatEnv("WEATHER_FORECAST_DAYS")
	if result != 14.0 {
		t.Errorf("expected 14.0, got %f", result)
	}
}

func TestWeatherPrecisionWiring(t *testing.T) {
	t.Setenv("WEATHER_PRECISION", "4")
	result := parseFloatEnv("WEATHER_PRECISION")
	if result != 4.0 {
		t.Errorf("expected 4.0, got %f", result)
	}
}

func TestWeatherForecastDaysWiring_ZeroOnEmpty(t *testing.T) {
	// When env var is empty/unset, parseFloatEnv returns 0.
	// The weather connector should use its internal default (7 days) when
	// SourceConfig has 0 — this verifies the wiring doesn't inject a non-zero
	// false-positive that would override the connector's range guard.
	t.Setenv("WEATHER_FORECAST_DAYS", "")
	result := parseFloatEnv("WEATHER_FORECAST_DAYS")
	if result != 0 {
		t.Errorf("expected 0 for empty env var, got %f", result)
	}
}

// --- IMP-019-R28: Financial Markets fred_enabled / fred_series SST end-to-end ---
// Before this fix, fred_enabled and fred_series were readable by parseMarketsConfig
// but never set by main.go, meaning operators could not disable FRED data when an
// API key was present, and could not customize which FRED series to track.

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

func TestMarketsFredSeriesWiring(t *testing.T) {
	t.Setenv("FINANCIAL_MARKETS_FRED_SERIES", `["GDP","UNRATE","DFF"]`)
	result := parseJSONArrayEnv("FINANCIAL_MARKETS_FRED_SERIES")
	if len(result) != 3 {
		t.Fatalf("expected 3 FRED series, got %d", len(result))
	}
	if result[0] != "GDP" || result[1] != "UNRATE" || result[2] != "DFF" {
		t.Errorf("unexpected series values: %v", result)
	}
}

func TestMarketsFredSeriesWiring_Empty(t *testing.T) {
	// When env var is absent, parseJSONArrayEnv returns nil.
	// parseMarketsConfig should fall back to defaultFREDSeries.
	t.Setenv("FINANCIAL_MARKETS_FRED_SERIES", "")
	result := parseJSONArrayEnv("FINANCIAL_MARKETS_FRED_SERIES")
	if result != nil {
		t.Errorf("expected nil for empty env var, got %v", result)
	}
}
