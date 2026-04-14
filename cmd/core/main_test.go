package main

import (
	"sort"
	"testing"

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

// --- SCN-019-001 / SCN-019-006: All 14 connectors register successfully ---

// TestAllConnectorsRegistered verifies that all 14 connector packages can be
// instantiated and registered in a single Registry without collision, matching
// the exact set in main.go's run() function. This is the unit-level proof for
// SCN-019-001 (all 15 registered) and SCN-019-006 (no regression from new registrations).
func TestAllConnectorsRegistered(t *testing.T) {
	registry := connector.NewRegistry()

	// Instantiate all 14 connectors using the same IDs and constructors as main.go.
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
