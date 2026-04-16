package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestNew(t *testing.T) {
	c := New("gov-alerts")
	if c.ID() != "gov-alerts" {
		t.Errorf("expected gov-alerts, got %s", c.ID())
	}
}

func TestConnect_NoLocations(t *testing.T) {
	c := New("gov-alerts")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{},
	})
	if err == nil {
		t.Error("expected error for no locations")
	}
}

func TestConnect_Valid(t *testing.T) {
	c := New("gov-alerts")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
			},
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHaversineKm(t *testing.T) {
	// SF to LA is approximately 559 km
	d := haversineKm(37.7749, -122.4194, 34.0522, -118.2437)
	if d < 500 || d > 600 {
		t.Errorf("SF to LA distance should be ~559 km, got %.0f", d)
	}

	// Same point = 0
	if d := haversineKm(37.0, -122.0, 37.0, -122.0); d != 0 {
		t.Errorf("same point should be 0, got %v", d)
	}
}

func TestFindNearestLocation(t *testing.T) {
	c := New("gov-alerts")
	c.config.Locations = []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	}

	// Nearby earthquake (within 200km)
	match := findNearestLocation(37.50, -122.10, c.config.Locations)
	if match == nil {
		t.Fatal("expected match for nearby earthquake")
	}
	if match.LocationName != "Home" {
		t.Errorf("expected Home, got %s", match.LocationName)
	}

	// Distant earthquake (Hawaii - way beyond 200km)
	match = findNearestLocation(20.0, -155.0, c.config.Locations)
	if match != nil {
		t.Error("expected no match for distant earthquake")
	}
}

func TestClassifyEarthquakeSeverity(t *testing.T) {
	tests := []struct {
		mag      float64
		dist     float64
		expected string
	}{
		{7.5, 150, "extreme"},
		{5.5, 50, "severe"},
		{3.5, 30, "moderate"},
		{2.5, 180, "minor"},
	}
	for _, tt := range tests {
		got := classifyEarthquakeSeverity(tt.mag, tt.dist)
		if got != tt.expected {
			t.Errorf("classifyEarthquakeSeverity(%.1f, %.0f) = %s, want %s", tt.mag, tt.dist, got, tt.expected)
		}
	}
}

func TestClose(t *testing.T) {
	c := New("gov-alerts")
	c.health = connector.HealthHealthy
	c.Close()
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Error("should be disconnected")
	}
}

func TestNormalizeEarthquake(t *testing.T) {
	eq := Earthquake{
		ID: "us7000test", Magnitude: 4.2, Latitude: 37.5, Longitude: -122.1,
		DepthKm: 8.5, Place: "10km NW of San Jose",
	}
	match := &ProximityMatch{LocationName: "Home", DistanceKm: 40}

	artifact := normalizeEarthquake(eq, match)
	if artifact.SourceID != "gov-alerts" {
		t.Errorf("expected gov-alerts, got %s", artifact.SourceID)
	}
	if artifact.ContentType != "alert/earthquake" {
		t.Errorf("expected alert/earthquake, got %s", artifact.ContentType)
	}
	if artifact.Metadata["magnitude"] != 4.2 {
		t.Errorf("expected magnitude 4.2, got %v", artifact.Metadata["magnitude"])
	}
}

// --- Chaos-hardening tests ---

// TestConcurrentSyncHealth verifies no data race between concurrent Sync and Health calls.
func TestConcurrentSyncHealth(t *testing.T) {
	c := New("gov-alerts")
	c.config = AlertsConfig{
		Locations:        []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceEarthquake: false, // disable API calls for race test
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = c.Sync(ctx, "")
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Health(ctx)
		}()
	}
	wg.Wait()
}

// TestConcurrentCloseHealth verifies no data race between Close and Health.
func TestConcurrentCloseHealth(t *testing.T) {
	c := New("gov-alerts")
	c.mu.Lock()
	c.health = connector.HealthHealthy
	c.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = c.Close()
		}()
		go func() {
			defer wg.Done()
			_ = c.Health(context.Background())
		}()
	}
	wg.Wait()
}

// TestSyncContextCancellation verifies Sync respects context cancellation.
func TestSyncContextCancellation(t *testing.T) {
	c := New("gov-alerts")
	c.config = AlertsConfig{
		Locations:        []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceEarthquake: false,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, _, err := c.Sync(ctx, "")
	// With no sources enabled, the Sync completes normally even with cancelled ctx;
	// the key assertion is that it doesn't hang or panic.
	_ = err
}

// TestKnownMapEviction verifies old dedup entries are evicted.
func TestKnownMapEviction(t *testing.T) {
	c := New("gov-alerts")
	c.config = AlertsConfig{
		Locations:        []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceEarthquake: false,
	}

	// Insert an old entry directly.
	oldTime := time.Now().Add(-8 * 24 * time.Hour)
	c.known["old-alert-123"] = oldTime
	c.known["recent-alert-456"] = time.Now()

	// Sync triggers eviction.
	_, _, _ = c.Sync(context.Background(), "")

	c.mu.RLock()
	defer c.mu.RUnlock()
	if _, exists := c.known["old-alert-123"]; exists {
		t.Error("old alert should have been evicted from known map")
	}
	if _, exists := c.known["recent-alert-456"]; !exists {
		t.Error("recent alert should still exist in known map")
	}
}

// TestIsFiniteCoord verifies coordinate validation rejects NaN, Inf, and out-of-range.
func TestIsFiniteCoord(t *testing.T) {
	tests := []struct {
		name string
		lat  float64
		lon  float64
		want bool
	}{
		{"valid SF", 37.77, -122.42, true},
		{"valid equator", 0.0, 0.0, true},
		{"valid poles", 90.0, 180.0, true},
		{"valid negative", -90.0, -180.0, true},
		{"NaN lat", math.NaN(), -122.0, false},
		{"NaN lon", 37.0, math.NaN(), false},
		{"Inf lat", math.Inf(1), -122.0, false},
		{"NegInf lon", 37.0, math.Inf(-1), false},
		{"lat too high", 91.0, -122.0, false},
		{"lat too low", -91.0, -122.0, false},
		{"lon too high", 37.0, 181.0, false},
		{"lon too low", 37.0, -181.0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFiniteCoord(tt.lat, tt.lon); got != tt.want {
				t.Errorf("isFiniteCoord(%v, %v) = %v, want %v", tt.lat, tt.lon, got, tt.want)
			}
		})
	}
}

// TestParseAlertsConfig_InvalidCoordinates verifies locations with bad coords are discarded.
func TestParseAlertsConfig_InvalidCoordinates(t *testing.T) {
	cfg, err := parseAlertsConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Valid", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
				map[string]interface{}{"name": "NaN", "latitude": math.NaN(), "longitude": -122.42, "radius_km": 200.0},
				map[string]interface{}{"name": "OutOfRange", "latitude": 95.0, "longitude": -122.42, "radius_km": 200.0},
				map[string]interface{}{"name": "ZeroRadius", "latitude": 37.0, "longitude": -122.0, "radius_km": 0.0},
				map[string]interface{}{"name": "NegativeRadius", "latitude": 37.0, "longitude": -122.0, "radius_km": -50.0},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Locations) != 1 {
		t.Errorf("expected 1 valid location, got %d", len(cfg.Locations))
	}
	if len(cfg.Locations) > 0 && cfg.Locations[0].Name != "Valid" {
		t.Errorf("expected Valid location, got %s", cfg.Locations[0].Name)
	}
}

// TestParseAlertsConfig_MissingName verifies locations without a name are discarded.
func TestParseAlertsConfig_MissingName(t *testing.T) {
	cfg, _ := parseAlertsConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
			},
		},
	})
	if len(cfg.Locations) != 0 {
		t.Errorf("expected 0 locations for missing name, got %d", len(cfg.Locations))
	}
}

// TestConcurrentConnectSync verifies no data race between Connect and Sync.
func TestConcurrentConnectSync(t *testing.T) {
	c := New("gov-alerts")
	validConfig := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
			},
		},
	}

	// Connect first to have valid config.
	if err := c.Connect(context.Background(), validConfig); err != nil {
		t.Fatal(err)
	}
	// Disable API calls for the race test.
	c.mu.Lock()
	c.config.SourceEarthquake = false
	c.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = c.Connect(context.Background(), validConfig)
		}()
		go func() {
			defer wg.Done()
			_, _, _ = c.Sync(context.Background(), "")
		}()
	}
	wg.Wait()
}

// --- Edge case tests ---

// usgsResponse builds a JSON-serializable USGS GeoJSON response.
func usgsResponse(features []map[string]interface{}) []byte {
	resp := map[string]interface{}{"features": features}
	b, _ := json.Marshal(resp)
	return b
}

// makeFeature builds one GeoJSON feature for test USGS responses.
func makeFeature(id string, mag float64, lon, lat, depth float64, place string) map[string]interface{} {
	return map[string]interface{}{
		"id": id,
		"properties": map[string]interface{}{
			"mag":   mag,
			"place": place,
			"time":  time.Now().UnixMilli(),
		},
		"geometry": map[string]interface{}{
			"coordinates": []float64{lon, lat, depth},
		},
	}
}

// newTestConnector creates a Connector pointed at the given test server URL.
func newTestConnector(serverURL string, locations []LocationConfig) *Connector {
	c := New("gov-alerts-test")
	c.baseURL = serverURL
	c.nwsBaseURL = serverURL
	c.config = AlertsConfig{
		Locations:        locations,
		SourceEarthquake: true,
		SourceWeather:    false,
		MinEarthquakeMag: 2.5,
	}
	return c
}

func TestClassifyEarthquakeSeverity_Boundaries(t *testing.T) {
	tests := []struct {
		name     string
		mag      float64
		dist     float64
		expected string
	}{
		{"exactly 7.0 at far range", 7.0, 500, "extreme"},
		{"7.0 at zero distance", 7.0, 0, "extreme"},
		{"6.99 at 0km distance", 6.99, 0, "severe"},
		{"exactly 5.0 at exactly 100km", 5.0, 100, "severe"},
		{"5.0 at 100.1km just outside severe", 5.0, 100.1, "minor"},
		{"exactly 3.0 at exactly 50km", 3.0, 50, "moderate"},
		{"3.0 at 50.1km just outside moderate", 3.0, 50.1, "minor"},
		{"4.99 at 50km (below severe threshold)", 4.99, 50, "moderate"},
		{"2.99 at 10km (below moderate threshold)", 2.99, 10, "minor"},
		{"negative magnitude", -1.0, 10, "minor"},
		{"zero magnitude zero distance", 0.0, 0, "minor"},
		{"huge magnitude", 9.5, 1000, "extreme"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyEarthquakeSeverity(tt.mag, tt.dist)
			if got != tt.expected {
				t.Errorf("classifyEarthquakeSeverity(%.2f, %.1f) = %s, want %s", tt.mag, tt.dist, got, tt.expected)
			}
		})
	}
}

func TestNormalizeEarthquake_TierAssignment(t *testing.T) {
	tests := []struct {
		name         string
		mag          float64
		distance     float64
		expectedTier string
	}{
		{"extreme severity gets full tier", 7.5, 200, "full"},
		{"severe severity gets full tier", 5.5, 50, "full"},
		{"moderate severity gets standard tier", 3.5, 30, "standard"},
		{"minor severity gets light tier", 2.0, 500, "light"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eq := Earthquake{ID: "test", Magnitude: tt.mag, Latitude: 37.5, Longitude: -122.1, DepthKm: 10, Place: "Test Place"}
			match := &ProximityMatch{LocationName: "Home", DistanceKm: tt.distance}
			artifact := normalizeEarthquake(eq, match)
			tier, ok := artifact.Metadata["processing_tier"].(string)
			if !ok || tier != tt.expectedTier {
				t.Errorf("expected tier %q, got %q", tt.expectedTier, tier)
			}
		})
	}
}

func TestFindNearestLocation_MultipleCandidates(t *testing.T) {
	locations := []LocationConfig{
		{Name: "FarCity", Latitude: 38.5, Longitude: -121.5, RadiusKm: 500},
		{Name: "NearCity", Latitude: 37.78, Longitude: -122.43, RadiusKm: 500},
	}
	// Point very close to NearCity
	match := findNearestLocation(37.77, -122.42, locations)
	if match == nil {
		t.Fatal("expected a match")
	}
	if match.LocationName != "NearCity" {
		t.Errorf("expected NearCity (closer), got %s", match.LocationName)
	}
}

func TestFindNearestLocation_EmptyLocations(t *testing.T) {
	match := findNearestLocation(37.77, -122.42, nil)
	if match != nil {
		t.Error("expected nil for empty locations")
	}
}

func TestFindNearestLocation_ExactBoundary(t *testing.T) {
	// Set up a location with a very small radius
	locations := []LocationConfig{
		{Name: "Tight", Latitude: 0, Longitude: 0, RadiusKm: 1},
	}
	// Point at the origin should match (distance 0)
	match := findNearestLocation(0, 0, locations)
	if match == nil {
		t.Fatal("expected match at exact same point")
	}
	if match.DistanceKm != 0 {
		t.Errorf("expected 0 distance, got %f", match.DistanceKm)
	}
}

func TestHaversineKm_ExtremeDistances(t *testing.T) {
	tests := []struct {
		name  string
		lat1  float64
		lon1  float64
		lat2  float64
		lon2  float64
		minKm float64
		maxKm float64
	}{
		{"north pole to south pole", 90, 0, -90, 0, 20000, 20100},
		{"antipodal points on equator", 0, 0, 0, 180, 20000, 20100},
		{"equator quarter", 0, 0, 0, 90, 10000, 10100},
		{"date line crossing", 0, 179, 0, -179, 200, 250},
		{"same point at pole", 90, 0, 90, 0, 0, 0.001},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := haversineKm(tt.lat1, tt.lon1, tt.lat2, tt.lon2)
			if d < tt.minKm || d > tt.maxKm {
				t.Errorf("haversineKm(%v,%v,%v,%v) = %.1f, want [%.0f, %.0f]",
					tt.lat1, tt.lon1, tt.lat2, tt.lon2, d, tt.minKm, tt.maxKm)
			}
		})
	}
}

func TestParseAlertsConfig_Defaults(t *testing.T) {
	cfg, err := parseAlertsConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MinEarthquakeMag != 2.5 {
		t.Errorf("expected default magnitude 2.5, got %f", cfg.MinEarthquakeMag)
	}
	if !cfg.SourceEarthquake {
		t.Error("expected SourceEarthquake true by default")
	}
	if !cfg.SourceWeather {
		t.Error("expected SourceWeather true by default")
	}
	// No radius_km specified; default is 200
	if len(cfg.Locations) != 1 {
		t.Fatalf("expected 1 location, got %d", len(cfg.Locations))
	}
	if cfg.Locations[0].RadiusKm != 200 {
		t.Errorf("expected default radius 200, got %f", cfg.Locations[0].RadiusKm)
	}
}

func TestParseAlertsConfig_CustomMagnitude(t *testing.T) {
	cfg, err := parseAlertsConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 100.0},
			},
			"min_earthquake_magnitude": 5.0,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MinEarthquakeMag != 5.0 {
		t.Errorf("expected magnitude 5.0, got %f", cfg.MinEarthquakeMag)
	}
}

func TestParseAlertsConfig_NilSourceConfig(t *testing.T) {
	cfg, err := parseAlertsConfig(connector.ConnectorConfig{
		SourceConfig: nil,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Locations) != 0 {
		t.Errorf("expected 0 locations for nil SourceConfig, got %d", len(cfg.Locations))
	}
}

func TestSync_Deduplication(t *testing.T) {
	features := []map[string]interface{}{
		makeFeature("eq-dup-1", 4.0, -122.42, 37.77, 10, "Near Home"),
		makeFeature("eq-dup-2", 5.0, -122.43, 37.76, 10, "Also Near Home"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(features))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	// First sync: both earthquakes are new.
	arts1, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("first sync error: %v", err)
	}
	if len(arts1) != 2 {
		t.Errorf("first sync: expected 2 artifacts, got %d", len(arts1))
	}

	// Second sync: same IDs, should be deduped.
	arts2, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("second sync error: %v", err)
	}
	if len(arts2) != 0 {
		t.Errorf("second sync: expected 0 artifacts (deduped), got %d", len(arts2))
	}
}

func TestSync_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error for HTTP 500")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("expected status 500 in error, got: %v", err)
	}
}

func TestSync_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"features": [{"id": "bad", "properties": {`))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestSync_EmptyFeatures(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(nil))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(arts) != 0 {
		t.Errorf("expected 0 artifacts for empty features, got %d", len(arts))
	}
}

func TestSync_InsufficientCoordinates(t *testing.T) {
	// Feature with only 2 coordinates (missing depth) should be skipped.
	features := []map[string]interface{}{
		{
			"id":         "eq-short-coords",
			"properties": map[string]interface{}{"mag": 4.0, "place": "Incomplete", "time": time.Now().UnixMilli()},
			"geometry":   map[string]interface{}{"coordinates": []float64{-122.42, 37.77}},
		},
		makeFeature("eq-valid", 4.0, -122.42, 37.77, 10, "Valid"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(features))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the valid feature should appear.
	if len(arts) != 1 {
		t.Errorf("expected 1 artifact (short-coords skipped), got %d", len(arts))
	}
}

func TestSync_InvalidCoordSkipped(t *testing.T) {
	// Earthquake with out-of-range coords should be skipped by isFiniteCoord check.
	features := []map[string]interface{}{
		makeFeature("eq-bad", 4.0, -200.0, 95.0, 10, "Out of range coords"),
		makeFeature("eq-ok", 4.0, -122.42, 37.77, 10, "Valid"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(features))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(arts) != 1 {
		t.Errorf("expected 1 artifact (NaN skipped), got %d", len(arts))
	}
}

func TestSync_OutOfRangeFiltered(t *testing.T) {
	// Earthquake far away should not produce an artifact.
	features := []map[string]interface{}{
		makeFeature("eq-far", 6.0, 139.69, 35.68, 10, "Tokyo"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(features))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(arts) != 0 {
		t.Errorf("expected 0 artifacts for out-of-range, got %d", len(arts))
	}
}

func TestSync_PassesMinMagnitudeToURL(t *testing.T) {
	var requestedURL string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(nil))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})
	c.config.MinEarthquakeMag = 4.5

	_, _, _ = c.Sync(context.Background(), "")
	if !strings.Contains(requestedURL, "minmagnitude=4.5") {
		t.Errorf("expected minmagnitude=4.5 in URL, got: %s", requestedURL)
	}
}

func TestConnect_ThenClose_ThenReconnect(t *testing.T) {
	c := New("gov-alerts")
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
			},
		},
	}

	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("first connect: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Error("expected healthy after connect")
	}

	c.Close()
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Error("expected disconnected after close")
	}

	// Reconnect should work.
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Error("expected healthy after reconnect")
	}
}

func TestSync_HealthTransitions(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(nil))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	// Before sync: health should still be disconnected (New sets it).
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Errorf("expected disconnected before sync, got %s", c.Health(context.Background()))
	}

	// After sync: health should return to healthy (deferred restore).
	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("expected healthy after sync, got %s", c.Health(context.Background()))
	}
}

func TestSync_ContextCancelledMidEarthquakeLoop(t *testing.T) {
	// Return many earthquakes so the context check in the loop triggers.
	var features []map[string]interface{}
	for i := 0; i < 20; i++ {
		features = append(features, makeFeature(
			"eq-cancel-"+strings.Repeat("x", i+1), 4.0, -122.42, 37.77, 10, "Test",
		))
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(features))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after letting at least one iteration (cancel immediately exercises the check).
	cancel()

	_, _, err := c.Sync(ctx, "")
	// The fetch itself may fail or the loop may return ctx.Err().
	// Either way, no panic is the key assertion.
	_ = err
}

func TestNormalizeEarthquake_MetadataFields(t *testing.T) {
	eq := Earthquake{
		ID: "us7000meta", Magnitude: 6.0, Latitude: 37.5, Longitude: -122.1,
		DepthKm: 15.5, Place: "25km SE of City", Time: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	match := &ProximityMatch{LocationName: "TestLoc", DistanceKm: 80}

	artifact := normalizeEarthquake(eq, match)

	// Verify all expected metadata fields.
	checks := map[string]interface{}{
		"alert_id":         "us7000meta",
		"source":           "usgs",
		"event_type":       "earthquake",
		"magnitude":        6.0,
		"depth_km":         15.5,
		"latitude":         37.5,
		"longitude":        -122.1,
		"severity":         "severe",
		"distance_km":      80.0,
		"nearest_location": "TestLoc",
		"processing_tier":  "full",
	}
	for key, want := range checks {
		got, exists := artifact.Metadata[key]
		if !exists {
			t.Errorf("missing metadata key %q", key)
			continue
		}
		if got != want {
			t.Errorf("metadata[%q] = %v, want %v", key, got, want)
		}
	}

	// Verify artifact-level fields.
	if artifact.SourceID != "gov-alerts" {
		t.Errorf("SourceID = %q, want gov-alerts", artifact.SourceID)
	}
	if artifact.SourceRef != "us7000meta" {
		t.Errorf("SourceRef = %q, want us7000meta", artifact.SourceRef)
	}
	if artifact.URL != "https://earthquake.usgs.gov/earthquakes/eventpage/us7000meta" {
		t.Errorf("URL = %q", artifact.URL)
	}
	if !strings.Contains(artifact.Title, "M6.0") {
		t.Errorf("Title missing magnitude: %q", artifact.Title)
	}
	if !artifact.CapturedAt.Equal(eq.Time) {
		t.Errorf("CapturedAt = %v, want %v", artifact.CapturedAt, eq.Time)
	}
}

// --- Hardening tests (R20) ---

// TestSync_ErrorSetsHealthDegraded verifies that a failed sync sets health to degraded, not healthy.
func TestSync_ErrorSetsHealthDegraded(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})
	// Set to healthy before sync to verify the transition.
	c.mu.Lock()
	c.health = connector.HealthHealthy
	c.mu.Unlock()

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for HTTP 503")
	}
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Errorf("expected degraded after failed sync, got %s", c.Health(context.Background()))
	}
}

// TestSync_SuccessRestoresHealthAfterDegraded verifies a successful sync after a failure restores healthy.
func TestSync_SuccessRestoresHealthAfterDegraded(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(nil))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	// First sync fails → degraded.
	_, _, _ = c.Sync(context.Background(), "")
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Fatalf("expected degraded after first sync, got %s", c.Health(context.Background()))
	}

	// Second sync succeeds → healthy.
	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("second sync error: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("expected healthy after successful sync, got %s", c.Health(context.Background()))
	}
}

// TestSync_OversizedResponseBody verifies the LimitReader prevents OOM on huge responses.
func TestSync_OversizedResponseBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Write a valid JSON prefix then pad with spaces to exceed limit.
		// The LimitReader will truncate, causing a JSON decode error.
		w.Write([]byte(`{"features": [`))
		// Write 11MB of padding (exceeds 10MB limit).
		pad := make([]byte, 4096)
		for i := range pad {
			pad[i] = ' '
		}
		for i := 0; i < 2816; i++ { // 2816 * 4096 = ~11.5MB
			w.Write(pad)
		}
		w.Write([]byte(`]}`))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error when response exceeds maxResponseBytes")
	}
}

// TestSync_ConcurrentWithLiveKnownMapWrites verifies no race when multiple concurrent syncs
// write to the known map via real HTTP responses.
func TestSync_ConcurrentWithLiveKnownMapWrites(t *testing.T) {
	features := []map[string]interface{}{
		makeFeature("eq-race-1", 4.0, -122.42, 37.77, 10, "Near Home"),
		makeFeature("eq-race-2", 5.0, -122.43, 37.76, 10, "Also Near Home"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(features))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = c.Sync(context.Background(), "")
		}()
	}
	wg.Wait()

	// After concurrent syncs, the known map should contain both IDs.
	c.mu.RLock()
	defer c.mu.RUnlock()
	if _, ok := c.known["eq-race-1"]; !ok {
		t.Error("eq-race-1 should be in known map after concurrent syncs")
	}
	if _, ok := c.known["eq-race-2"]; !ok {
		t.Error("eq-race-2 should be in known map after concurrent syncs")
	}
}

// TestParseAlertsConfig_WrongFieldTypes verifies config parsing is robust when fields have unexpected types.
func TestParseAlertsConfig_WrongFieldTypes(t *testing.T) {
	tests := []struct {
		name string
		cfg  map[string]interface{}
		want int // expected location count
	}{
		{
			"latitude as string",
			map[string]interface{}{
				"locations": []interface{}{
					map[string]interface{}{"name": "Bad", "latitude": "37.77", "longitude": -122.42, "radius_km": 200.0},
				},
			},
			0, // latitude type assertion fails → rejected (requires explicit float64)
		},
		{
			"longitude as string",
			map[string]interface{}{
				"locations": []interface{}{
					map[string]interface{}{"name": "Bad", "latitude": 37.77, "longitude": "-122.42", "radius_km": 200.0},
				},
			},
			0, // longitude type assertion fails → rejected (requires explicit float64)
		},
		{
			"radius as string",
			map[string]interface{}{
				"locations": []interface{}{
					map[string]interface{}{"name": "Bad", "latitude": 37.77, "longitude": -122.42, "radius_km": "200"},
				},
			},
			1, // radius_km type assertion fails → default 200 used → valid
		},
		{
			"locations as string not array",
			map[string]interface{}{
				"locations": "not-an-array",
			},
			0,
		},
		{
			"location entry as string not map",
			map[string]interface{}{
				"locations": []interface{}{"not-a-map"},
			},
			0,
		},
		{
			"min_earthquake_magnitude as string",
			map[string]interface{}{
				"locations": []interface{}{
					map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
				},
				"min_earthquake_magnitude": "5.0",
			},
			1, // location valid, magnitude type assertion fails → default 2.5 kept
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseAlertsConfig(connector.ConnectorConfig{SourceConfig: tt.cfg})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(cfg.Locations) != tt.want {
				t.Errorf("expected %d locations, got %d", tt.want, len(cfg.Locations))
			}
		})
	}
}

// TestSync_EmptyEarthquakeID verifies that earthquakes with empty IDs are rejected (security: prevents dedup collision).
func TestSync_EmptyEarthquakeID(t *testing.T) {
	features := []map[string]interface{}{
		makeFeature("", 4.0, -122.42, 37.77, 10, "No ID 1"),
		makeFeature("", 5.0, -122.43, 37.76, 10, "No ID 2"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(features))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	// Both have empty ID → both should be rejected (not silently deduped).
	if len(arts) != 0 {
		t.Errorf("expected 0 artifacts (empty IDs rejected), got %d", len(arts))
	}
}

// TestSync_BeforeConnect verifies Sync on a never-connected connector doesn't crash.
func TestSync_BeforeConnect(t *testing.T) {
	c := New("gov-alerts")

	arts, cursor, err := c.Sync(context.Background(), "")
	// Zero-value config has SourceEarthquake=false, so no fetch is attempted.
	if err != nil {
		t.Errorf("expected no error for unconnected sync, got: %v", err)
	}
	if len(arts) != 0 {
		t.Errorf("expected 0 artifacts, got %d", len(arts))
	}
	if cursor == "" {
		t.Error("expected non-empty cursor (RFC3339 timestamp)")
	}
}

// TestSync_ContextCancelledDuringHTTPFetch verifies that a cancelled context during
// the actual HTTP fetch returns an error and sets health to degraded.
func TestSync_ContextCancelledDuringHTTPFetch(t *testing.T) {
	// Server hangs to force context cancellation during the HTTP request.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // block until client cancels
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, _, err := c.Sync(ctx, "")
	if err == nil {
		t.Error("expected error when context is cancelled during fetch")
	}
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Errorf("expected degraded after cancelled fetch, got %s", c.Health(context.Background()))
	}
}

// TestSync_NegativeDepthHandled verifies earthquakes with negative depth (surface deformation) parse.
func TestSync_NegativeDepthHandled(t *testing.T) {
	features := []map[string]interface{}{
		makeFeature("eq-neg-depth", 3.5, -122.42, 37.77, -2.5, "Surface event"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(features))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	depth, ok := arts[0].Metadata["depth_km"].(float64)
	if !ok || depth != -2.5 {
		t.Errorf("expected depth_km=-2.5, got %v", arts[0].Metadata["depth_km"])
	}
}

// TestSync_LargeEarthquakeBatch verifies Sync handles many earthquakes without issues.
func TestSync_LargeEarthquakeBatch(t *testing.T) {
	var features []map[string]interface{}
	for i := 0; i < 20; i++ {
		features = append(features, makeFeature(
			fmt.Sprintf("eq-batch-%d", i), 4.0, -122.42, 37.77, 10, "Batch test",
		))
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(features))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(arts) != 20 {
		t.Errorf("expected 20 artifacts, got %d", len(arts))
	}

	// All should be in known map now.
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.known) != 20 {
		t.Errorf("expected 20 known entries, got %d", len(c.known))
	}
}

// TestConnect_OverwritesPreviousConfig verifies repeated Connect updates config atomically.
func TestConnect_OverwritesPreviousConfig(t *testing.T) {
	c := New("gov-alerts")
	cfg1 := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "First", "latitude": 37.77, "longitude": -122.42, "radius_km": 100.0},
			},
		},
	}
	cfg2 := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Second", "latitude": 40.71, "longitude": -74.01, "radius_km": 300.0},
			},
		},
	}

	if err := c.Connect(context.Background(), cfg1); err != nil {
		t.Fatal(err)
	}
	c.mu.RLock()
	if c.config.Locations[0].Name != "First" {
		t.Error("expected First location after first connect")
	}
	c.mu.RUnlock()

	if err := c.Connect(context.Background(), cfg2); err != nil {
		t.Fatal(err)
	}
	c.mu.RLock()
	if c.config.Locations[0].Name != "Second" {
		t.Errorf("expected Second location after reconnect, got %s", c.config.Locations[0].Name)
	}
	c.mu.RUnlock()
}

// --- Security hardening tests ---

// TestSanitizeStringField verifies control character stripping and length truncation.
func TestSanitizeStringField(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"normal string", "10km NW of San Jose", "10km NW of San Jose"},
		{"control chars stripped", "Hello\x00World\x07Test", "HelloWorldTest"},
		{"newlines stripped", "Line1\nLine2\rLine3", "Line1Line2Line3"},
		{"tabs stripped", "Col1\tCol2", "Col1Col2"},
		{"spaces preserved", "Hello World", "Hello World"},
		{"empty string", "", ""},
		{"unicode preserved", "地震 café résumé", "地震 café résumé"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeStringField(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeStringField(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSanitizeStringField_Truncation verifies strings are capped at maxStringFieldLen.
func TestSanitizeStringField_Truncation(t *testing.T) {
	long := strings.Repeat("x", maxStringFieldLen+500)
	got := sanitizeStringField(long)
	if len(got) > maxStringFieldLen {
		t.Errorf("expected max length %d, got %d", maxStringFieldLen, len(got))
	}
}

// TestSanitizeContentField_HigherLimit verifies content fields use maxContentFieldLen (IMP-017-IMPROVE-008).
func TestSanitizeContentField_HigherLimit(t *testing.T) {
	// A 3000-char description should survive sanitizeContentField but be truncated by sanitizeStringField.
	long := strings.Repeat("A severe thunderstorm warning. ", 120) // ~3600 chars
	if len(long) <= maxStringFieldLen {
		t.Fatal("test setup: need input longer than maxStringFieldLen")
	}

	// sanitizeStringField truncates at 1024
	shortResult := sanitizeStringField(long)
	if len(shortResult) > maxStringFieldLen {
		t.Errorf("sanitizeStringField should truncate at %d, got %d", maxStringFieldLen, len(shortResult))
	}

	// sanitizeContentField preserves up to maxContentFieldLen
	longResult := sanitizeContentField(long)
	if len(longResult) < len(shortResult) {
		t.Error("sanitizeContentField should preserve more than sanitizeStringField")
	}
	if len(longResult) != len(long) {
		t.Errorf("sanitizeContentField should preserve full %d-char content, got %d", len(long), len(longResult))
	}

	// Very long content (>maxContentFieldLen) IS truncated
	veryLong := strings.Repeat("x", maxContentFieldLen+1000)
	veryLongResult := sanitizeContentField(veryLong)
	if len(veryLongResult) > maxContentFieldLen {
		t.Errorf("expected max content length %d, got %d", maxContentFieldLen, len(veryLongResult))
	}
}

// TestSanitizeContentField_ControlChars verifies content field sanitization strips control chars.
func TestSanitizeContentField_ControlChars(t *testing.T) {
	input := "Line 1\x00Line 2\x07Line 3"
	got := sanitizeContentField(input)
	if strings.ContainsAny(got, "\x00\x07") {
		t.Errorf("sanitizeContentField should strip control chars: %q", got)
	}
	if got != "Line 1Line 2Line 3" {
		t.Errorf("unexpected result: %q", got)
	}
}

// TestNWSDescription_LongContentPreserved verifies that long NWS descriptions are not truncated
// at 1024 chars, preserving critical safety information (IMP-017-IMPROVE-008).
func TestNWSDescription_LongContentPreserved(t *testing.T) {
	// Simulate a realistic NWS description that exceeds 1024 chars.
	longDesc := strings.Repeat("Tornado spotted moving northeast. Affected areas include multiple counties. ", 20)
	if len(longDesc) <= maxStringFieldLen {
		t.Fatal("test setup: description must exceed maxStringFieldLen")
	}

	features := []map[string]interface{}{
		makeNWSFeature(
			"urn:oid:long-desc-1", "Tornado Warning", "Extreme", "Observed", "Immediate",
			"Tornado Warning headline", longDesc, "TAKE COVER NOW",
			"Central Oklahoma",
			"2024-01-15T14:30:00-06:00", "2024-01-15T15:30:00-06:00",
		),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(nwsResponse(features))
	}))
	defer ts.Close()

	c := New("test")
	c.nwsBaseURL = ts.URL

	alerts, err := c.fetchNWSAlerts(context.Background(), 35.47, -97.52)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	// Description should be preserved beyond 1024 chars.
	if len(alerts[0].Description) <= maxStringFieldLen {
		t.Errorf("long NWS description was truncated at %d chars (IMP-017-IMPROVE-008); got %d chars",
			maxStringFieldLen, len(alerts[0].Description))
	}
	if len(alerts[0].Description) != len(longDesc) {
		t.Errorf("expected description length %d, got %d", len(longDesc), len(alerts[0].Description))
	}
}

// TestSanitizeAlertID verifies ID sanitization and empty rejection.
func TestSanitizeAlertID(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantID string
		wantOK bool
	}{
		{"normal ID", "us7000abc", "us7000abc", true},
		{"whitespace-only rejected", "   ", "", false},
		{"empty rejected", "", "", false},
		{"control chars stripped valid", "us\x00700", "us700", true},
		{"trimmed", "  us123  ", "us123", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotOK := sanitizeAlertID(tt.input)
			if gotOK != tt.wantOK || gotID != tt.wantID {
				t.Errorf("sanitizeAlertID(%q) = (%q, %v), want (%q, %v)", tt.input, gotID, gotOK, tt.wantID, tt.wantOK)
			}
		})
	}
}

// TestSafeEventPageURL verifies URL path escaping for untrusted IDs.
func TestSafeEventPageURL(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{"normal ID", "us7000abc", "https://earthquake.usgs.gov/earthquakes/eventpage/us7000abc"},
		{"ID with slash", "us/../../etc/passwd", "https://earthquake.usgs.gov/earthquakes/eventpage/us%2F..%2F..%2Fetc%2Fpasswd"},
		{"ID with spaces", "us 7000", "https://earthquake.usgs.gov/earthquakes/eventpage/us%207000"},
		{"ID with special chars", "us<script>alert(1)</script>", "https://earthquake.usgs.gov/earthquakes/eventpage/us%3Cscript%3Ealert%281%29%3C%2Fscript%3E"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safeEventPageURL(tt.id)
			if got != tt.want {
				t.Errorf("safeEventPageURL(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

// TestSync_UserAgentHeader verifies outbound requests include the User-Agent header.
func TestSync_UserAgentHeader(t *testing.T) {
	var gotUA string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(nil))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	_, _, _ = c.Sync(context.Background(), "")
	if gotUA != userAgent {
		t.Errorf("expected User-Agent %q, got %q", userAgent, gotUA)
	}
}

// TestSync_ControlCharsInPlaceSanitized verifies Place field is sanitized from API responses.
func TestSync_ControlCharsInPlaceSanitized(t *testing.T) {
	features := []map[string]interface{}{
		makeFeature("eq-inject-1", 4.0, -122.42, 37.77, 10, "10km NW\x00of\nSan\tJose"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(features))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	// The Title/RawContent should not contain control characters.
	if strings.ContainsAny(arts[0].Title, "\x00\n\t\r") {
		t.Errorf("Title contains control chars: %q", arts[0].Title)
	}
	if strings.ContainsAny(arts[0].RawContent, "\x00\n\t\r") {
		t.Errorf("RawContent contains control chars: %q", arts[0].RawContent)
	}
}

// TestSync_PathTraversalInID verifies path traversal in ID is safely escaped in URL.
func TestSync_PathTraversalInID(t *testing.T) {
	features := []map[string]interface{}{
		makeFeature("../../etc/passwd", 4.0, -122.42, 37.77, 10, "Test"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(features))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	// URL must be properly escaped, not containing raw path traversal.
	if strings.Contains(arts[0].URL, "../") {
		t.Errorf("URL contains unescaped path traversal: %q", arts[0].URL)
	}
	if !strings.Contains(arts[0].URL, "%2F") {
		t.Errorf("URL should contain escaped slashes: %q", arts[0].URL)
	}
}

// TestParseAlertsConfig_InvalidMagnitude verifies out-of-range magnitudes are rejected.
func TestParseAlertsConfig_InvalidMagnitude(t *testing.T) {
	tests := []struct {
		name string
		mag  interface{}
		err  bool
	}{
		{"negative magnitude", -1.0, true},
		{"too high magnitude", 11.0, true},
		{"NaN magnitude", math.NaN(), true},
		{"Inf magnitude", math.Inf(1), true},
		{"valid magnitude", 5.0, false},
		{"zero magnitude (valid floor)", 0.0, false},
		{"ten magnitude (valid ceiling)", 10.0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseAlertsConfig(connector.ConnectorConfig{
				SourceConfig: map[string]interface{}{
					"locations": []interface{}{
						map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
					},
					"min_earthquake_magnitude": tt.mag,
				},
			})
			if tt.err && err == nil {
				t.Error("expected error for invalid magnitude")
			}
			if !tt.err && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestSync_WhitespaceOnlyID verifies whitespace-only IDs are rejected.
func TestSync_WhitespaceOnlyID(t *testing.T) {
	features := []map[string]interface{}{
		makeFeature("   ", 4.0, -122.42, 37.77, 10, "Blank ID"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(features))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(arts) != 0 {
		t.Errorf("expected 0 artifacts (whitespace-only ID rejected), got %d", len(arts))
	}
}

// --- NWS Weather Alerts Tests ---

// nwsResponse builds a JSON NWS alert API response.
func nwsResponse(features []map[string]interface{}) []byte {
	resp := map[string]interface{}{"features": features}
	b, _ := json.Marshal(resp)
	return b
}

// makeNWSFeature builds one NWS alert feature for test responses.
func makeNWSFeature(id, event, severity, certainty, urgency, headline, description, instruction, areaDesc, effective, expires string) map[string]interface{} {
	return map[string]interface{}{
		"properties": map[string]interface{}{
			"id":          id,
			"event":       event,
			"severity":    severity,
			"certainty":   certainty,
			"urgency":     urgency,
			"headline":    headline,
			"description": description,
			"instruction": instruction,
			"areaDesc":    areaDesc,
			"effective":   effective,
			"expires":     expires,
		},
	}
}

// newNWSTestConnector creates a Connector with NWS source enabled, earthquakes disabled.
func newNWSTestConnector(nwsServerURL string, locations []LocationConfig) *Connector {
	c := New("gov-alerts-nws-test")
	c.nwsBaseURL = nwsServerURL
	c.config = AlertsConfig{
		Locations:        locations,
		SourceEarthquake: false,
		SourceWeather:    true,
		MinEarthquakeMag: 2.5,
	}
	return c
}

func TestFetchNWSAlerts_ValidResponse(t *testing.T) {
	features := []map[string]interface{}{
		makeNWSFeature(
			"urn:oid:2.49.0.1.840.0.001",
			"Tornado Warning",
			"Extreme",
			"Observed",
			"Immediate",
			"Tornado Warning issued for Central Oklahoma",
			"A large tornado was observed moving northeast.",
			"TAKE COVER NOW. Move to a basement or interior room.",
			"Central Oklahoma",
			"2024-01-15T14:30:00-06:00",
			"2024-01-15T15:30:00-06:00",
		),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(nwsResponse(features))
	}))
	defer ts.Close()

	c := New("test")
	c.nwsBaseURL = ts.URL

	alerts, err := c.fetchNWSAlerts(context.Background(), 35.47, -97.52)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	alert := alerts[0]
	if alert.ID != "urn:oid:2.49.0.1.840.0.001" {
		t.Errorf("ID = %q", alert.ID)
	}
	if alert.Event != "Tornado Warning" {
		t.Errorf("Event = %q", alert.Event)
	}
	if alert.Severity != "Extreme" {
		t.Errorf("Severity = %q", alert.Severity)
	}
	if alert.Certainty != "Observed" {
		t.Errorf("Certainty = %q", alert.Certainty)
	}
	if alert.Urgency != "Immediate" {
		t.Errorf("Urgency = %q", alert.Urgency)
	}
	if alert.Headline != "Tornado Warning issued for Central Oklahoma" {
		t.Errorf("Headline = %q", alert.Headline)
	}
	if alert.Description != "A large tornado was observed moving northeast." {
		t.Errorf("Description = %q", alert.Description)
	}
	if alert.Instruction != "TAKE COVER NOW. Move to a basement or interior room." {
		t.Errorf("Instruction = %q", alert.Instruction)
	}
	if alert.AreaDesc != "Central Oklahoma" {
		t.Errorf("AreaDesc = %q", alert.AreaDesc)
	}
	if alert.Effective.IsZero() {
		t.Error("Effective should be parsed")
	}
	if alert.Expires.IsZero() {
		t.Error("Expires should be parsed")
	}
}

func TestFetchNWSAlerts_EmptyResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(nwsResponse(nil))
	}))
	defer ts.Close()

	c := New("test")
	c.nwsBaseURL = ts.URL

	alerts, err := c.fetchNWSAlerts(context.Background(), 35.47, -97.52)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}

func TestFetchNWSAlerts_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write([]byte(`{"features": [{"properties": {`))
	}))
	defer ts.Close()

	c := New("test")
	c.nwsBaseURL = ts.URL

	_, err := c.fetchNWSAlerts(context.Background(), 35.47, -97.52)
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestFetchNWSAlerts_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	c := New("test")
	c.nwsBaseURL = ts.URL

	_, err := c.fetchNWSAlerts(context.Background(), 35.47, -97.52)
	if err == nil {
		t.Error("expected error for HTTP 503")
	}
	if !strings.Contains(err.Error(), "status 503") {
		t.Errorf("expected status 503 in error, got: %v", err)
	}
}

func TestFetchNWSAlerts_UserAgentHeader(t *testing.T) {
	var gotUA string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(nwsResponse(nil))
	}))
	defer ts.Close()

	c := New("test")
	c.nwsBaseURL = ts.URL

	_, _ = c.fetchNWSAlerts(context.Background(), 35.47, -97.52)
	if gotUA != userAgent {
		t.Errorf("expected User-Agent %q, got %q", userAgent, gotUA)
	}
}

func TestFetchNWSAlerts_EmptyIDRejected(t *testing.T) {
	features := []map[string]interface{}{
		makeNWSFeature("", "Flood Warning", "Severe", "Likely", "Expected", "Flood Warning", "desc", "", "Area", "2024-01-15T14:30:00-06:00", "2024-01-15T15:30:00-06:00"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(nwsResponse(features))
	}))
	defer ts.Close()

	c := New("test")
	c.nwsBaseURL = ts.URL

	alerts, err := c.fetchNWSAlerts(context.Background(), 35.47, -97.52)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts (empty ID rejected), got %d", len(alerts))
	}
}

func TestFetchNWSAlerts_InvalidTimeFallback(t *testing.T) {
	features := []map[string]interface{}{
		makeNWSFeature(
			"urn:oid:test-bad-time", "Heat Advisory", "Moderate", "Likely", "Expected",
			"Heat Advisory", "Stay hydrated", "", "Metro Area",
			"not-a-date", "also-not-a-date",
		),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(nwsResponse(features))
	}))
	defer ts.Close()

	c := New("test")
	c.nwsBaseURL = ts.URL

	alerts, err := c.fetchNWSAlerts(context.Background(), 35.47, -97.52)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if !alerts[0].Effective.IsZero() {
		t.Error("expected zero Effective for invalid time")
	}
	if !alerts[0].Expires.IsZero() {
		t.Error("expected zero Expires for invalid time")
	}
}

func TestMapNWSSeverity(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Extreme", "extreme"},
		{"Severe", "severe"},
		{"Moderate", "moderate"},
		{"Minor", "minor"},
		{"Unknown", "unknown"},
		{"", "unknown"},
		{"extreme", "extreme"},
		{"SEVERE", "severe"},
		{"Something Else", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapNWSSeverity(tt.input)
			if got != tt.want {
				t.Errorf("mapNWSSeverity(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestClassifyNWSEventType(t *testing.T) {
	tests := []struct {
		event string
		want  string
	}{
		{"Tornado Warning", "tornado"},
		{"Tornado Watch", "tornado"},
		{"Hurricane Warning", "hurricane"},
		{"Tropical Storm Warning", "hurricane"},
		{"Flash Flood Warning", "flood"},
		{"Flood Watch", "flood"},
		{"Winter Storm Warning", "winter_storm"},
		{"Blizzard Warning", "winter_storm"},
		{"Ice Storm Warning", "winter_storm"},
		{"Severe Thunderstorm Warning", "thunderstorm"},
		{"Excessive Heat Warning", "heat"},
		{"Heat Advisory", "heat"},
		{"High Wind Warning", "wind"},
		{"Red Flag Warning", "fire"},
		{"Dense Fog Advisory", "fog"},
		{"Air Quality Alert", "weather"},
		{"Special Weather Statement", "weather"},
		{"", "weather"},
	}
	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			got := classifyNWSEventType(tt.event)
			if got != tt.want {
				t.Errorf("classifyNWSEventType(%q) = %q, want %q", tt.event, got, tt.want)
			}
		})
	}
}

func TestNormalizeNWSAlert(t *testing.T) {
	alert := NWSAlert{
		ID:          "urn:oid:2.49.0.1.840.0.001",
		Event:       "Tornado Warning",
		Severity:    "Extreme",
		Certainty:   "Observed",
		Urgency:     "Immediate",
		Headline:    "Tornado Warning issued for Central Oklahoma",
		Description: "A large tornado was observed.",
		Instruction: "TAKE COVER NOW.",
		AreaDesc:    "Central Oklahoma",
		Effective:   time.Date(2024, 1, 15, 20, 30, 0, 0, time.UTC),
		Expires:     time.Date(2024, 1, 15, 21, 30, 0, 0, time.UTC),
	}
	match := &ProximityMatch{LocationName: "Home", DistanceKm: 25}

	artifact := normalizeNWSAlert(alert, match)

	if artifact.SourceID != "gov-alerts" {
		t.Errorf("SourceID = %q", artifact.SourceID)
	}
	if artifact.SourceRef != "urn:oid:2.49.0.1.840.0.001" {
		t.Errorf("SourceRef = %q", artifact.SourceRef)
	}
	if artifact.ContentType != "alert/weather" {
		t.Errorf("ContentType = %q", artifact.ContentType)
	}
	if !strings.Contains(artifact.Title, "Tornado Warning") {
		t.Errorf("Title missing event: %q", artifact.Title)
	}
	if !strings.Contains(artifact.Title, "Home") {
		t.Errorf("Title missing location: %q", artifact.Title)
	}
	if !strings.Contains(artifact.RawContent, "TAKE COVER NOW.") {
		t.Errorf("RawContent missing instruction: %q", artifact.RawContent)
	}

	checks := map[string]interface{}{
		"alert_id":         "urn:oid:2.49.0.1.840.0.001",
		"source":           "nws",
		"event_type":       "tornado",
		"event":            "Tornado Warning",
		"severity":         "extreme",
		"certainty":        "Observed",
		"urgency":          "Immediate",
		"area_desc":        "Central Oklahoma",
		"distance_km":      25.0,
		"nearest_location": "Home",
		"processing_tier":  "full",
	}
	for key, want := range checks {
		got, exists := artifact.Metadata[key]
		if !exists {
			t.Errorf("missing metadata key %q", key)
			continue
		}
		if got != want {
			t.Errorf("metadata[%q] = %v, want %v", key, got, want)
		}
	}

	// IMP-017-IMPROVE-007: Verify effective/expires timestamps in metadata.
	if eff, ok := artifact.Metadata["effective"].(string); !ok || eff == "" {
		t.Error("missing effective timestamp in metadata (IMP-017-IMPROVE-007)")
	}
	if exp, ok := artifact.Metadata["expires"].(string); !ok || exp == "" {
		t.Error("missing expires timestamp in metadata (IMP-017-IMPROVE-007)")
	}
}

// TestNormalizeNWSAlert_ExpiresInMetadata verifies effective/expires are stored in metadata (IMP-017-IMPROVE-007).
func TestNormalizeNWSAlert_ExpiresInMetadata(t *testing.T) {
	eff := time.Date(2024, 1, 15, 20, 30, 0, 0, time.UTC)
	exp := time.Date(2024, 1, 15, 21, 30, 0, 0, time.UTC)
	alert := NWSAlert{
		ID:        "test-expires",
		Event:     "Heat Advisory",
		Severity:  "Moderate",
		Effective: eff,
		Expires:   exp,
	}
	match := &ProximityMatch{LocationName: "Home", DistanceKm: 0}

	artifact := normalizeNWSAlert(alert, match)

	if got, ok := artifact.Metadata["effective"].(string); !ok {
		t.Error("effective metadata missing")
	} else if got != eff.Format(time.RFC3339) {
		t.Errorf("effective = %q, want %q", got, eff.Format(time.RFC3339))
	}

	if got, ok := artifact.Metadata["expires"].(string); !ok {
		t.Error("expires metadata missing")
	} else if got != exp.Format(time.RFC3339) {
		t.Errorf("expires = %q, want %q", got, exp.Format(time.RFC3339))
	}
}

// TestNormalizeNWSAlert_ZeroTimesOmitted verifies that zero-value times are not stored (IMP-017-IMPROVE-007).
func TestNormalizeNWSAlert_ZeroTimesOmitted(t *testing.T) {
	alert := NWSAlert{
		ID:       "test-zero-times",
		Event:    "Test",
		Severity: "Minor",
		// Effective and Expires left as zero values
	}
	match := &ProximityMatch{LocationName: "Home", DistanceKm: 0}

	artifact := normalizeNWSAlert(alert, match)

	if _, exists := artifact.Metadata["effective"]; exists {
		t.Error("zero Effective should not be stored in metadata")
	}
	if _, exists := artifact.Metadata["expires"]; exists {
		t.Error("zero Expires should not be stored in metadata")
	}
}

func TestNormalizeNWSAlert_TierAssignment(t *testing.T) {
	tests := []struct {
		name         string
		severity     string
		expectedTier string
	}{
		{"extreme gets full", "Extreme", "full"},
		{"severe gets full", "Severe", "full"},
		{"moderate gets standard", "Moderate", "standard"},
		{"minor gets light", "Minor", "light"},
		{"unknown gets light", "FooBar", "light"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alert := NWSAlert{ID: "test-tier", Event: "Test", Severity: tt.severity}
			match := &ProximityMatch{LocationName: "Home", DistanceKm: 10}
			artifact := normalizeNWSAlert(alert, match)
			tier, ok := artifact.Metadata["processing_tier"].(string)
			if !ok || tier != tt.expectedTier {
				t.Errorf("expected tier %q, got %q", tt.expectedTier, tier)
			}
		})
	}
}

func TestNormalizeNWSAlert_ZeroEffectiveFallback(t *testing.T) {
	alert := NWSAlert{ID: "test-zero-time", Event: "Test"}
	match := &ProximityMatch{LocationName: "Home", DistanceKm: 0}
	artifact := normalizeNWSAlert(alert, match)
	if artifact.CapturedAt.IsZero() {
		t.Error("CapturedAt should not be zero when Effective is zero (should fall back to now)")
	}
}

func TestParseAlertsConfig_SourceWeather(t *testing.T) {
	tests := []struct {
		name string
		cfg  map[string]interface{}
		want bool
	}{
		{
			"default (no key) is true",
			map[string]interface{}{
				"locations": []interface{}{
					map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
				},
			},
			true,
		},
		{
			"explicitly true",
			map[string]interface{}{
				"locations": []interface{}{
					map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
				},
				"source_weather": true,
			},
			true,
		},
		{
			"explicitly false",
			map[string]interface{}{
				"locations": []interface{}{
					map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
				},
				"source_weather": false,
			},
			false,
		},
		{
			"wrong type ignored, default true",
			map[string]interface{}{
				"locations": []interface{}{
					map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
				},
				"source_weather": "yes",
			},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseAlertsConfig(connector.ConnectorConfig{SourceConfig: tt.cfg})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.SourceWeather != tt.want {
				t.Errorf("SourceWeather = %v, want %v", cfg.SourceWeather, tt.want)
			}
		})
	}
}

func TestSync_WeatherAlertsOnly(t *testing.T) {
	features := []map[string]interface{}{
		makeNWSFeature(
			"urn:oid:nws-sync-1", "Flood Warning", "Severe", "Likely", "Expected",
			"Flood Warning for Bay Area", "Heavy rain expected.", "Move to higher ground.",
			"San Francisco Bay Area",
			"2024-01-15T14:30:00-06:00", "2024-01-15T18:30:00-06:00",
		),
		makeNWSFeature(
			"urn:oid:nws-sync-2", "Wind Advisory", "Minor", "Possible", "Expected",
			"Wind Advisory", "Gusty winds expected.", "",
			"San Francisco Bay Area",
			"2024-01-15T14:30:00-06:00", "2024-01-15T18:30:00-06:00",
		),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(nwsResponse(features))
	}))
	defer ts.Close()

	c := newNWSTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(arts) != 2 {
		t.Errorf("expected 2 weather artifacts, got %d", len(arts))
	}
	for _, art := range arts {
		if art.ContentType != "alert/weather" {
			t.Errorf("expected alert/weather, got %s", art.ContentType)
		}
		if art.Metadata["source"] != "nws" {
			t.Errorf("expected source nws, got %v", art.Metadata["source"])
		}
	}
}

func TestSync_WeatherAlertDeduplication(t *testing.T) {
	features := []map[string]interface{}{
		makeNWSFeature(
			"urn:oid:nws-dedup-1", "Heat Advisory", "Moderate", "Likely", "Expected",
			"Heat Advisory", "Stay hydrated.", "", "Metro Area",
			"2024-01-15T14:30:00-06:00", "2024-01-15T18:30:00-06:00",
		),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(nwsResponse(features))
	}))
	defer ts.Close()

	c := newNWSTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	// First sync: new alert.
	arts1, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("first sync error: %v", err)
	}
	if len(arts1) != 1 {
		t.Errorf("first sync: expected 1 artifact, got %d", len(arts1))
	}

	// Second sync: same ID, should be deduped.
	arts2, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("second sync error: %v", err)
	}
	if len(arts2) != 0 {
		t.Errorf("second sync: expected 0 artifacts (deduped), got %d", len(arts2))
	}
}

func TestSync_BothEarthquakeAndWeather(t *testing.T) {
	reqCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		w.Header().Set("Content-Type", "application/json")
		// USGS endpoint contains "fdsnws"
		if strings.Contains(r.URL.Path, "fdsnws") {
			features := []map[string]interface{}{
				makeFeature("eq-both-1", 4.0, -122.42, 37.77, 10, "Near Home"),
			}
			w.Write(usgsResponse(features))
		} else {
			// NWS endpoint
			features := []map[string]interface{}{
				makeNWSFeature(
					"urn:oid:nws-both-1", "Tornado Warning", "Extreme", "Observed", "Immediate",
					"Tornado Warning", "Take cover.", "Shelter now.",
					"Local Area",
					"2024-01-15T14:30:00-06:00", "2024-01-15T15:30:00-06:00",
				),
			}
			w.Write(nwsResponse(features))
		}
	}))
	defer ts.Close()

	c := New("gov-alerts-both-test")
	c.baseURL = ts.URL
	c.nwsBaseURL = ts.URL
	c.config = AlertsConfig{
		Locations:        []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceEarthquake: true,
		SourceWeather:    true,
		MinEarthquakeMag: 2.5,
	}

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(arts) != 2 {
		t.Errorf("expected 2 artifacts (1 earthquake + 1 weather), got %d", len(arts))
	}

	// Verify we got both types.
	types := map[string]bool{}
	for _, art := range arts {
		types[art.ContentType] = true
	}
	if !types["alert/earthquake"] {
		t.Error("missing alert/earthquake artifact")
	}
	if !types["alert/weather"] {
		t.Error("missing alert/weather artifact")
	}
}

func TestSync_WeatherDisabled(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "alerts") {
			called = true
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(usgsResponse(nil))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})
	c.config.SourceWeather = false

	_, _, _ = c.Sync(context.Background(), "")
	if called {
		t.Error("NWS endpoint should not have been called when source_weather is false")
	}
}

func TestSync_NWSHTTPError_SetsDegraded(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := newNWSTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})
	c.mu.Lock()
	c.health = connector.HealthHealthy
	c.mu.Unlock()

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error for NWS HTTP 500")
	}
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Errorf("expected degraded after NWS failure, got %s", c.Health(context.Background()))
	}
}

func TestFetchNWSAlerts_ControlCharsInFields(t *testing.T) {
	features := []map[string]interface{}{
		makeNWSFeature(
			"urn:oid:nws-inject", "Flood\x00Warning", "Severe\x07", "Likely", "Expected",
			"Headline\ninjected", "Desc\ttab", "Instruct\rreturn",
			"Area\x1BEscape",
			"2024-01-15T14:30:00-06:00", "2024-01-15T18:30:00-06:00",
		),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(nwsResponse(features))
	}))
	defer ts.Close()

	c := New("test")
	c.nwsBaseURL = ts.URL

	alerts, err := c.fetchNWSAlerts(context.Background(), 35.47, -97.52)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	a := alerts[0]
	if strings.ContainsAny(a.Event, "\x00\x07") {
		t.Errorf("Event contains control chars: %q", a.Event)
	}
	if strings.ContainsAny(a.Headline, "\n\r") {
		t.Errorf("Headline contains control chars: %q", a.Headline)
	}
	if strings.ContainsAny(a.Description, "\t") {
		t.Errorf("Description contains control chars: %q", a.Description)
	}
	if strings.ContainsAny(a.AreaDesc, "\x1B") {
		t.Errorf("AreaDesc contains escape: %q", a.AreaDesc)
	}
}

func TestFetchNWSAlerts_PointInURL(t *testing.T) {
	var requestedURL string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(nwsResponse(nil))
	}))
	defer ts.Close()

	c := New("test")
	c.nwsBaseURL = ts.URL

	_, _ = c.fetchNWSAlerts(context.Background(), 35.4700, -97.5200)
	if !strings.Contains(requestedURL, "point=35.4700,-97.5200") {
		t.Errorf("expected point=35.4700,-97.5200 in URL, got: %s", requestedURL)
	}
	if !strings.Contains(requestedURL, "status=actual") {
		t.Errorf("expected status=actual in URL, got: %s", requestedURL)
	}
	if !strings.Contains(requestedURL, "limit=50") {
		t.Errorf("expected limit=50 in URL (IMP-017-IMPROVE-006), got: %s", requestedURL)
	}
}

// =====================================================
// NOAA Tsunami Source Tests
// =====================================================

func tsunamiAtomXML(entries []string) string {
	xml := `<?xml version="1.0" encoding="UTF-8"?><feed xmlns="http://www.w3.org/2005/Atom">`
	for _, e := range entries {
		xml += e
	}
	xml += `</feed>`
	return xml
}

func makeTsunamiEntry(id, title, summary, link, published string) string {
	return fmt.Sprintf(`<entry><id>%s</id><title>%s</title><summary>%s</summary><link href="%s"/><published>%s</published></entry>`,
		id, title, summary, link, published)
}

func makeTsunamiEntryWithGeo(id, title, summary, link, published, geoPoint string) string {
	return fmt.Sprintf(`<entry><id>%s</id><title>%s</title><summary>%s</summary><link href="%s"/><published>%s</published><point xmlns="http://www.georss.org/georss">%s</point></entry>`,
		id, title, summary, link, published, geoPoint)
}

func TestFetchTsunamiAlerts_ValidResponse(t *testing.T) {
	entries := []string{
		makeTsunamiEntry(
			"tsunami-001",
			"Tsunami Warning — Pacific Coast",
			"A tsunami warning has been issued following M7.8 earthquake.",
			"https://www.tsunami.gov/events/tsunami-001",
			"2024-03-15T10:30:00Z",
		),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, tsunamiAtomXML(entries))
	}))
	defer ts.Close()

	c := New("test")
	c.tsunamiBaseURL = ts.URL

	alerts, err := c.fetchTsunamiAlerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].ID != "tsunami-001" {
		t.Errorf("ID = %q", alerts[0].ID)
	}
	if alerts[0].Title != "Tsunami Warning — Pacific Coast" {
		t.Errorf("Title = %q", alerts[0].Title)
	}
	if alerts[0].Severity != "severe" {
		t.Errorf("Severity = %q, want severe", alerts[0].Severity)
	}
	if alerts[0].Published.IsZero() {
		t.Error("Published should be parsed")
	}
}

func TestFetchTsunamiAlerts_EmptyResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, tsunamiAtomXML(nil))
	}))
	defer ts.Close()

	c := New("test")
	c.tsunamiBaseURL = ts.URL

	alerts, err := c.fetchTsunamiAlerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}

func TestFetchTsunamiAlerts_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	c := New("test")
	c.tsunamiBaseURL = ts.URL

	_, err := c.fetchTsunamiAlerts(context.Background())
	if err == nil {
		t.Error("expected error for HTTP 503")
	}
	if !strings.Contains(err.Error(), "status 503") {
		t.Errorf("expected status 503 in error, got: %v", err)
	}
}

func TestClassifyTsunamiSeverity(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Tsunami Warning — Pacific Coast", "severe"},
		{"Tsunami Watch issued for Hawaii", "moderate"},
		{"Tsunami Advisory for coastal areas", "minor"},
		{"Tsunami Information Statement", "info"},
		{"", "info"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := classifyTsunamiSeverity(tt.title)
			if got != tt.want {
				t.Errorf("classifyTsunamiSeverity(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestNormalizeTsunamiAlert(t *testing.T) {
	alert := TsunamiAlert{
		ID:        "tsunami-test-001",
		Title:     "Tsunami Warning — Alaska",
		Summary:   "Major tsunami warning following M8.0 earthquake.",
		Link:      "https://www.tsunami.gov/events/tsunami-test-001",
		Published: time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		Severity:  "severe",
		GeoPoint:  "61.2181 -149.9003",
	}

	match := &ProximityMatch{LocationName: "Home", DistanceKm: 50}
	artifact := normalizeTsunamiAlert(alert, match)
	if artifact.SourceID != "gov-alerts" {
		t.Errorf("SourceID = %q", artifact.SourceID)
	}
	if artifact.ContentType != "alert/tsunami" {
		t.Errorf("ContentType = %q", artifact.ContentType)
	}
	if artifact.Metadata["source"] != "noaa_tsunami" {
		t.Errorf("source = %v", artifact.Metadata["source"])
	}
	if artifact.Metadata["severity"] != "severe" {
		t.Errorf("severity = %v", artifact.Metadata["severity"])
	}
	if artifact.Metadata["processing_tier"] != "full" {
		t.Errorf("processing_tier = %v", artifact.Metadata["processing_tier"])
	}
}

func TestSync_TsunamiSource(t *testing.T) {
	entries := []string{
		makeTsunamiEntryWithGeo("tsunami-sync-1", "Tsunami Watch — West Coast", "Watch issued.", "https://example.com/1", "2024-03-15T10:30:00Z", "37.50 -122.10"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, tsunamiAtomXML(entries))
	}))
	defer ts.Close()

	c := New("test")
	c.tsunamiBaseURL = ts.URL
	c.config = AlertsConfig{
		Locations:     []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceTsunami: true,
	}

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(arts) != 1 {
		t.Errorf("expected 1 artifact, got %d", len(arts))
	}
}

// =====================================================
// USGS Volcano Source Tests
// =====================================================

func volcanoJSON(entries []map[string]interface{}) []byte {
	b, _ := json.Marshal(entries)
	return b
}

func TestFetchVolcanoAlerts_ValidResponse(t *testing.T) {
	entries := []map[string]interface{}{
		{"id": "vol-001", "volcanoName": "Mount Rainier", "alertLevel": "WATCH", "colorCode": "ORANGE", "issuedDate": "2024-04-01T12:00:00Z"},
		{"id": "vol-002", "volcanoName": "Kilauea", "alertLevel": "WARNING", "colorCode": "RED", "issuedDate": "2024-04-01T14:00:00Z"},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(volcanoJSON(entries))
	}))
	defer ts.Close()

	c := New("test")
	c.volcanoBaseURL = ts.URL

	alerts, err := c.fetchVolcanoAlerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(alerts))
	}
	if alerts[0].Volcano != "Mount Rainier" {
		t.Errorf("Volcano = %q", alerts[0].Volcano)
	}
	if alerts[0].Severity != "moderate" {
		t.Errorf("Severity = %q, want moderate", alerts[0].Severity)
	}
	if alerts[1].Severity != "severe" {
		t.Errorf("Severity = %q, want severe", alerts[1].Severity)
	}
}

func TestFetchVolcanoAlerts_EmptyResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer ts.Close()

	c := New("test")
	c.volcanoBaseURL = ts.URL

	alerts, err := c.fetchVolcanoAlerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}

func TestFetchVolcanoAlerts_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	c := New("test")
	c.volcanoBaseURL = ts.URL

	_, err := c.fetchVolcanoAlerts(context.Background())
	if err == nil {
		t.Error("expected error for HTTP 502")
	}
	if !strings.Contains(err.Error(), "status 502") {
		t.Errorf("expected status 502 in error, got: %v", err)
	}
}

func TestClassifyVolcanoSeverity(t *testing.T) {
	tests := []struct {
		level string
		want  string
	}{
		{"WARNING", "severe"},
		{"WATCH", "moderate"},
		{"ADVISORY", "minor"},
		{"NORMAL", "info"},
		{"warning", "severe"},
		{"Watch", "moderate"},
		{"", "info"},
		{"UNKNOWN", "info"},
	}
	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			got := classifyVolcanoSeverity(tt.level)
			if got != tt.want {
				t.Errorf("classifyVolcanoSeverity(%q) = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

func TestNormalizeVolcanoAlert(t *testing.T) {
	alert := VolcanoAlert{
		ID:         "vol-test-001",
		Volcano:    "Mount St. Helens",
		AlertLevel: "WARNING",
		ColorCode:  "RED",
		Issued:     time.Date(2024, 4, 1, 12, 0, 0, 0, time.UTC),
		Severity:   "severe",
	}

	artifact := normalizeVolcanoAlert(alert)
	if artifact.ContentType != "alert/volcano" {
		t.Errorf("ContentType = %q", artifact.ContentType)
	}
	if artifact.Metadata["source"] != "usgs_volcano" {
		t.Errorf("source = %v", artifact.Metadata["source"])
	}
	if artifact.Metadata["volcano_name"] != "Mount St. Helens" {
		t.Errorf("volcano_name = %v", artifact.Metadata["volcano_name"])
	}
	if artifact.Metadata["alert_level"] != "WARNING" {
		t.Errorf("alert_level = %v", artifact.Metadata["alert_level"])
	}
	if artifact.Metadata["color_code"] != "RED" {
		t.Errorf("color_code = %v", artifact.Metadata["color_code"])
	}
	if !strings.Contains(artifact.Title, "Mount St. Helens") {
		t.Errorf("Title missing volcano name: %q", artifact.Title)
	}
}

// =====================================================
// InciWeb Wildfire Source Tests
// =====================================================

func wildfireRSSXML(items []string) string {
	xml := `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel>`
	for _, item := range items {
		xml += item
	}
	xml += `</channel></rss>`
	return xml
}

func makeWildfireItem(guid, title, description, link, pubDate string) string {
	return fmt.Sprintf(`<item><guid>%s</guid><title>%s</title><description>%s</description><link>%s</link><pubDate>%s</pubDate></item>`,
		guid, title, description, link, pubDate)
}

func TestFetchWildfireAlerts_ValidResponse(t *testing.T) {
	items := []string{
		makeWildfireItem(
			"fire-001",
			"Caldor Fire",
			"Wildfire burning in El Dorado County. Evacuation orders in effect.",
			"https://inciweb.wildfire.gov/incident/fire-001",
			"Mon, 15 Jul 2024 10:30:00 +0000",
		),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, wildfireRSSXML(items))
	}))
	defer ts.Close()

	c := New("test")
	c.wildfireBaseURL = ts.URL

	alerts, err := c.fetchWildfireAlerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Title != "Caldor Fire" {
		t.Errorf("Title = %q", alerts[0].Title)
	}
	if alerts[0].Severity != "extreme" {
		t.Errorf("Severity = %q, want extreme (evacuation keyword)", alerts[0].Severity)
	}
}

func TestFetchWildfireAlerts_EmptyResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, wildfireRSSXML(nil))
	}))
	defer ts.Close()

	c := New("test")
	c.wildfireBaseURL = ts.URL

	alerts, err := c.fetchWildfireAlerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}

func TestFetchWildfireAlerts_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	c := New("test")
	c.wildfireBaseURL = ts.URL

	_, err := c.fetchWildfireAlerts(context.Background())
	if err == nil {
		t.Error("expected error for HTTP 404")
	}
	if !strings.Contains(err.Error(), "status 404") {
		t.Errorf("expected status 404 in error, got: %v", err)
	}
}

func TestClassifyWildfireSeverity(t *testing.T) {
	tests := []struct {
		title       string
		description string
		want        string
	}{
		{"Caldor Fire", "Evacuation orders in effect.", "extreme"},
		{"Oak Fire", "Mandatory evacuate all residents.", "extreme"},
		{"Creek Fire", "Fire warning in effect for area.", "severe"},
		{"Dixie Fire", "Large fire burning in remote area.", "moderate"},
		{"", "", "moderate"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := classifyWildfireSeverity(tt.title, tt.description)
			if got != tt.want {
				t.Errorf("classifyWildfireSeverity(%q, %q) = %q, want %q", tt.title, tt.description, got, tt.want)
			}
		})
	}
}

func TestNormalizeWildfireAlert(t *testing.T) {
	alert := WildfireAlert{
		ID:          "fire-test-001",
		Title:       "Park Fire",
		Description: "Wildfire burning 5000 acres. Warning issued.",
		Link:        "https://inciweb.wildfire.gov/incident/fire-test-001",
		PubDate:     time.Date(2024, 7, 15, 10, 30, 0, 0, time.UTC),
		Severity:    "severe",
	}

	artifact := normalizeWildfireAlert(alert)
	if artifact.ContentType != "alert/wildfire" {
		t.Errorf("ContentType = %q", artifact.ContentType)
	}
	if artifact.Metadata["source"] != "inciweb" {
		t.Errorf("source = %v", artifact.Metadata["source"])
	}
	if artifact.Metadata["event_type"] != "wildfire" {
		t.Errorf("event_type = %v", artifact.Metadata["event_type"])
	}
	if artifact.URL != "https://inciweb.wildfire.gov/incident/fire-test-001" {
		t.Errorf("URL = %q", artifact.URL)
	}
	if artifact.Metadata["processing_tier"] != "full" {
		t.Errorf("processing_tier = %v (severe should be full)", artifact.Metadata["processing_tier"])
	}
}

// =====================================================
// AirNow AQI Source Tests
// =====================================================

func airnowJSON(entries []map[string]interface{}) []byte {
	b, _ := json.Marshal(entries)
	return b
}

func TestFetchAirNowAQI_ValidResponse(t *testing.T) {
	entries := []map[string]interface{}{
		{
			"DateObserved":  "2024-07-15 ",
			"HourObserved":  14,
			"AQI":           165,
			"ParameterName": "PM2.5",
			"ReportingArea": "San Francisco",
			"Category":      map[string]interface{}{"Name": "Unhealthy"},
		},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(airnowJSON(entries))
	}))
	defer ts.Close()

	c := New("test")
	c.airnowBaseURL = ts.URL

	obs, err := c.fetchAirNowAQI(context.Background(), 37.77, -122.42, "test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(obs))
	}
	if obs[0].AQI != 165 {
		t.Errorf("AQI = %d", obs[0].AQI)
	}
	if obs[0].Severity != "moderate" {
		t.Errorf("Severity = %q, want moderate (AQI 165)", obs[0].Severity)
	}
	if obs[0].Category != "Unhealthy" {
		t.Errorf("Category = %q", obs[0].Category)
	}
}

func TestFetchAirNowAQI_EmptyResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer ts.Close()

	c := New("test")
	c.airnowBaseURL = ts.URL

	obs, err := c.fetchAirNowAQI(context.Background(), 37.77, -122.42, "test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(obs) != 0 {
		t.Errorf("expected 0 observations, got %d", len(obs))
	}
}

func TestFetchAirNowAQI_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	c := New("test")
	c.airnowBaseURL = ts.URL

	_, err := c.fetchAirNowAQI(context.Background(), 37.77, -122.42, "bad-key")
	if err == nil {
		t.Error("expected error for HTTP 403")
	}
	if !strings.Contains(err.Error(), "status 403") {
		t.Errorf("expected status 403 in error, got: %v", err)
	}
}

func TestClassifyAQISeverity(t *testing.T) {
	tests := []struct {
		aqi  int
		want string
	}{
		{350, "extreme"},
		{301, "extreme"},
		{250, "severe"},
		{201, "severe"},
		{175, "moderate"},
		{151, "moderate"},
		{120, "minor"},
		{101, "minor"},
		{50, "info"},
		{100, "info"},
		{0, "info"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("AQI_%d", tt.aqi), func(t *testing.T) {
			got := classifyAQISeverity(tt.aqi)
			if got != tt.want {
				t.Errorf("classifyAQISeverity(%d) = %q, want %q", tt.aqi, got, tt.want)
			}
		})
	}
}

func TestNormalizeAirNowAlert(t *testing.T) {
	obs := AirNowObservation{
		ID:              "airnow-SF-PM2.5-165",
		AQI:             165,
		Category:        "Unhealthy",
		Pollutant:       "PM2.5",
		ReportingArea:   "San Francisco",
		Severity:        "moderate",
		ObservationTime: time.Date(2024, 7, 15, 14, 0, 0, 0, time.UTC),
	}
	match := &ProximityMatch{LocationName: "Home", DistanceKm: 5}

	artifact := normalizeAirNowAlert(obs, match)
	if artifact.ContentType != "alert/air-quality" {
		t.Errorf("ContentType = %q", artifact.ContentType)
	}
	if artifact.Metadata["source"] != "airnow" {
		t.Errorf("source = %v", artifact.Metadata["source"])
	}
	if artifact.Metadata["aqi"] != 165 {
		t.Errorf("aqi = %v", artifact.Metadata["aqi"])
	}
	if artifact.Metadata["pollutant"] != "PM2.5" {
		t.Errorf("pollutant = %v", artifact.Metadata["pollutant"])
	}
	if !strings.Contains(artifact.Title, "AQI 165") {
		t.Errorf("Title missing AQI: %q", artifact.Title)
	}
	if !strings.Contains(artifact.Title, "Home") {
		t.Errorf("Title missing location: %q", artifact.Title)
	}
}

func TestSync_AirNowDisabledWithoutKey(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer ts.Close()

	c := New("test")
	c.airnowBaseURL = ts.URL
	c.config = AlertsConfig{
		Locations:    []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceAirNow: true,
		AirNowAPIKey: "", // empty key → source skipped
	}

	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if called {
		t.Error("AirNow API should not be called when API key is empty")
	}
}

// =====================================================
// GDACS Global Disasters Source Tests
// =====================================================

func gdacsRSSXML(items []string) string {
	xml := `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel>`
	for _, item := range items {
		xml += item
	}
	xml += `</channel></rss>`
	return xml
}

func makeGDACSItem(guid, title, description, link, pubDate, alertLevel, geoPoint string) string {
	return fmt.Sprintf(`<item><guid>%s</guid><title>%s</title><description>%s</description><link>%s</link><pubDate>%s</pubDate><alertlevel>%s</alertlevel><point>%s</point></item>`,
		guid, title, description, link, pubDate, alertLevel, geoPoint)
}

func TestFetchGDACSAlerts_ValidResponse(t *testing.T) {
	items := []string{
		makeGDACSItem(
			"gdacs-001",
			"Earthquake M7.2 — Indonesia",
			"Major earthquake in Java region.",
			"https://www.gdacs.org/report/gdacs-001",
			"Mon, 15 Jul 2024 10:30:00 +0000",
			"Red",
			"-6.5 110.4",
		),
		makeGDACSItem(
			"gdacs-002",
			"Tropical Cyclone TC-2024-001",
			"Category 3 cyclone approaching coast.",
			"https://www.gdacs.org/report/gdacs-002",
			"Mon, 15 Jul 2024 12:00:00 +0000",
			"Orange",
			"15.2 -88.3",
		),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, gdacsRSSXML(items))
	}))
	defer ts.Close()

	c := New("test")
	c.gdacsBaseURL = ts.URL

	alerts, err := c.fetchGDACSAlerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(alerts))
	}
	if alerts[0].Severity != "extreme" {
		t.Errorf("alert[0] Severity = %q, want extreme (Red)", alerts[0].Severity)
	}
	if alerts[1].Severity != "severe" {
		t.Errorf("alert[1] Severity = %q, want severe (Orange)", alerts[1].Severity)
	}
	if alerts[0].GeoPoint != "-6.5 110.4" {
		t.Errorf("GeoPoint = %q", alerts[0].GeoPoint)
	}
}

func TestFetchGDACSAlerts_EmptyResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, gdacsRSSXML(nil))
	}))
	defer ts.Close()

	c := New("test")
	c.gdacsBaseURL = ts.URL

	alerts, err := c.fetchGDACSAlerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}

func TestFetchGDACSAlerts_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGatewayTimeout)
	}))
	defer ts.Close()

	c := New("test")
	c.gdacsBaseURL = ts.URL

	_, err := c.fetchGDACSAlerts(context.Background())
	if err == nil {
		t.Error("expected error for HTTP 504")
	}
	if !strings.Contains(err.Error(), "status 504") {
		t.Errorf("expected status 504 in error, got: %v", err)
	}
}

func TestClassifyGDACSAlertLevel(t *testing.T) {
	tests := []struct {
		level string
		want  string
	}{
		{"Red", "extreme"},
		{"red", "extreme"},
		{"RED", "extreme"},
		{"Orange", "severe"},
		{"ORANGE", "severe"},
		{"Green", "moderate"},
		{"green", "moderate"},
		{"", "info"},
		{"Yellow", "info"},
	}
	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			got := classifyGDACSAlertLevel(tt.level)
			if got != tt.want {
				t.Errorf("classifyGDACSAlertLevel(%q) = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

func TestNormalizeGDACSAlert(t *testing.T) {
	alert := GDACSAlert{
		ID:          "gdacs-test-001",
		Title:       "Earthquake M7.2 — Indonesia",
		Description: "Strong earthquake detected.",
		Link:        "https://www.gdacs.org/report/gdacs-test-001",
		PubDate:     time.Date(2024, 7, 15, 10, 30, 0, 0, time.UTC),
		GeoPoint:    "-6.5 110.4",
		Severity:    "extreme",
	}

	artifact := normalizeGDACSAlert(alert, &ProximityMatch{LocationName: "Home", DistanceKm: 150})
	if artifact.ContentType != "alert/disaster" {
		t.Errorf("ContentType = %q", artifact.ContentType)
	}
	if artifact.Metadata["source"] != "gdacs" {
		t.Errorf("source = %v", artifact.Metadata["source"])
	}
	if artifact.Metadata["severity"] != "extreme" {
		t.Errorf("severity = %v", artifact.Metadata["severity"])
	}
	if artifact.Metadata["processing_tier"] != "full" {
		t.Errorf("processing_tier = %v", artifact.Metadata["processing_tier"])
	}
	if artifact.Metadata["geo_point"] != "-6.5 110.4" {
		t.Errorf("geo_point = %v", artifact.Metadata["geo_point"])
	}
	if artifact.Metadata["latitude"] != -6.5 {
		t.Errorf("latitude = %v", artifact.Metadata["latitude"])
	}
	if artifact.Metadata["longitude"] != 110.4 {
		t.Errorf("longitude = %v", artifact.Metadata["longitude"])
	}
	if artifact.Metadata["distance_km"] != 150.0 {
		t.Errorf("distance_km = %v, want 150", artifact.Metadata["distance_km"])
	}
	if artifact.Metadata["nearest_location"] != "Home" {
		t.Errorf("nearest_location = %v, want Home", artifact.Metadata["nearest_location"])
	}
}

func TestNormalizeGDACSAlert_NoGeoPoint(t *testing.T) {
	alert := GDACSAlert{
		ID:       "gdacs-no-geo",
		Title:    "Flood Alert",
		Severity: "moderate",
	}

	artifact := normalizeGDACSAlert(alert, nil)
	if _, exists := artifact.Metadata["geo_point"]; exists {
		t.Error("geo_point should not be present when GeoPoint is empty")
	}
	if _, exists := artifact.Metadata["latitude"]; exists {
		t.Error("latitude should not be present when GeoPoint is empty")
	}
	if _, exists := artifact.Metadata["distance_km"]; exists {
		t.Error("distance_km should not be present when match is nil")
	}
	if _, exists := artifact.Metadata["nearest_location"]; exists {
		t.Error("nearest_location should not be present when match is nil")
	}
}

// =====================================================
// Config Parsing Tests for New Sources
// =====================================================

func TestParseAlertsConfig_NewSourceFlags(t *testing.T) {
	cfg, err := parseAlertsConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"airnow_api_key": "test-key-123"},
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
			},
			"source_tsunami":  true,
			"source_volcano":  true,
			"source_wildfire": true,
			"source_airnow":   true,
			"source_gdacs":    true,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.SourceTsunami {
		t.Error("expected SourceTsunami true")
	}
	if !cfg.SourceVolcano {
		t.Error("expected SourceVolcano true")
	}
	if !cfg.SourceWildfire {
		t.Error("expected SourceWildfire true")
	}
	if !cfg.SourceAirNow {
		t.Error("expected SourceAirNow true")
	}
	if !cfg.SourceGDACS {
		t.Error("expected SourceGDACS true")
	}
	if cfg.AirNowAPIKey != "test-key-123" {
		t.Errorf("AirNowAPIKey = %q", cfg.AirNowAPIKey)
	}
}

func TestParseAlertsConfig_NewSourceDefaults(t *testing.T) {
	cfg, err := parseAlertsConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SourceTsunami {
		t.Error("expected SourceTsunami false by default")
	}
	if cfg.SourceVolcano {
		t.Error("expected SourceVolcano false by default")
	}
	if cfg.SourceWildfire {
		t.Error("expected SourceWildfire false by default")
	}
	if cfg.SourceAirNow {
		t.Error("expected SourceAirNow false by default")
	}
	if cfg.SourceGDACS {
		t.Error("expected SourceGDACS false by default")
	}
	if cfg.AirNowAPIKey != "" {
		t.Errorf("expected empty AirNowAPIKey, got %q", cfg.AirNowAPIKey)
	}
}

// --- Scope 6: Proactive Delivery & Travel Alerts Tests ---

// mockNotifier records notifications for test verification.
type mockNotifier struct {
	mu            sync.Mutex
	notifications []AlertNotification
}

func (m *mockNotifier) NotifyAlert(_ context.Context, payload AlertNotification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, payload)
	return nil
}

func (m *mockNotifier) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.notifications)
}

func (m *mockNotifier) last() AlertNotification {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.notifications[len(m.notifications)-1]
}

// Unit Test 1: Extreme severity triggers notification
func TestMaybeNotify_Extreme(t *testing.T) {
	mn := &mockNotifier{}
	c := New("test")
	c.Notifier = mn

	art := connector.RawArtifact{
		SourceRef:   "eq-extreme-1",
		ContentType: "alert/earthquake",
		Title:       "M7.5 Earthquake — Offshore (50 km from Home)",
		RawContent:  "Magnitude 7.5 earthquake at depth 10 km.",
		Metadata: map[string]interface{}{
			"severity":         "extreme",
			"source":           "usgs",
			"distance_km":      50.0,
			"nearest_location": "Home",
		},
	}

	c.maybeNotify(context.Background(), art)

	if mn.count() != 1 {
		t.Fatalf("expected 1 notification, got %d", mn.count())
	}
	n := mn.last()
	if n.Severity != "extreme" {
		t.Errorf("expected severity extreme, got %s", n.Severity)
	}
	if n.AlertID != "eq-extreme-1" {
		t.Errorf("expected alert_id eq-extreme-1, got %s", n.AlertID)
	}
}

// Unit Test 2: Severe severity triggers notification
func TestMaybeNotify_Severe(t *testing.T) {
	mn := &mockNotifier{}
	c := New("test")
	c.Notifier = mn

	art := connector.RawArtifact{
		SourceRef:   "nws-severe-1",
		ContentType: "alert/weather",
		Title:       "Tornado Warning — Oklahoma County",
		RawContent:  "Tornado Warning\n\nInstruction: Take shelter immediately",
		Metadata: map[string]interface{}{
			"severity":         "severe",
			"source":           "nws",
			"distance_km":      10.0,
			"nearest_location": "Home",
		},
	}

	c.maybeNotify(context.Background(), art)

	if mn.count() != 1 {
		t.Fatalf("expected 1 notification, got %d", mn.count())
	}
	n := mn.last()
	if n.Severity != "severe" {
		t.Errorf("expected severity severe, got %s", n.Severity)
	}
	if n.Instructions != "Take shelter immediately" {
		t.Errorf("expected instructions extraction, got %q", n.Instructions)
	}
}

// Unit Test 3: Moderate severity does NOT trigger notification
func TestMaybeNotify_Moderate_NoNotification(t *testing.T) {
	mn := &mockNotifier{}
	c := New("test")
	c.Notifier = mn

	art := connector.RawArtifact{
		SourceRef: "eq-moderate-1",
		Metadata: map[string]interface{}{
			"severity": "moderate",
			"source":   "usgs",
		},
	}

	c.maybeNotify(context.Background(), art)

	if mn.count() != 0 {
		t.Errorf("expected 0 notifications for moderate severity, got %d", mn.count())
	}
}

// Unit Test 4: Nil notifier does not panic
func TestMaybeNotify_NilNotifier(t *testing.T) {
	c := New("test")
	// c.Notifier is nil

	art := connector.RawArtifact{
		SourceRef: "eq-extreme-1",
		Metadata: map[string]interface{}{
			"severity": "extreme",
		},
	}

	// Should not panic
	c.maybeNotify(context.Background(), art)
}

// Unit Test 5: Travel locations use doubled radius in merged locations
func TestTravelLocations_DoubleRadius(t *testing.T) {
	c := New("test")
	cfg := AlertsConfig{
		Locations: []LocationConfig{
			{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
		},
		TravelLocations: []LocationConfig{
			{Name: "Trip-NYC", Latitude: 40.71, Longitude: -74.01, RadiusKm: 100},
		},
	}

	merged := c.mergedLocations(context.Background(), cfg)

	if len(merged) != 2 {
		t.Fatalf("expected 2 merged locations, got %d", len(merged))
	}

	// Home location keeps original radius
	if merged[0].RadiusKm != 200 {
		t.Errorf("home radius should be 200, got %.0f", merged[0].RadiusKm)
	}

	// Travel location should have doubled radius
	if merged[1].RadiusKm != 200 { // 100 * 2 = 200
		t.Errorf("travel radius should be doubled to 200, got %.0f", merged[1].RadiusKm)
	}
	if !merged[1].IsTravel {
		t.Error("travel location should have IsTravel=true")
	}
}

// Unit Test 6: Alert notification payload structure
func TestAlertNotificationPayload(t *testing.T) {
	mn := &mockNotifier{}
	c := New("test")
	c.Notifier = mn

	art := connector.RawArtifact{
		SourceRef:   "gdacs-red-1",
		ContentType: "alert/disaster",
		Title:       "GDACS Red: Cyclone in Indian Ocean",
		RawContent:  "Category 5 cyclone.\n\nInstruction: Evacuate coastal areas",
		Metadata: map[string]interface{}{
			"severity":         "extreme",
			"source":           "gdacs",
			"distance_km":      75.5,
			"nearest_location": "Beach House",
		},
	}

	c.maybeNotify(context.Background(), art)

	if mn.count() != 1 {
		t.Fatalf("expected 1 notification, got %d", mn.count())
	}
	n := mn.last()
	if n.AlertID != "gdacs-red-1" {
		t.Errorf("AlertID = %q, want gdacs-red-1", n.AlertID)
	}
	if n.Headline != "GDACS Red: Cyclone in Indian Ocean" {
		t.Errorf("Headline = %q", n.Headline)
	}
	if n.Severity != "extreme" {
		t.Errorf("Severity = %q, want extreme", n.Severity)
	}
	if n.Source != "gdacs" {
		t.Errorf("Source = %q, want gdacs", n.Source)
	}
	if n.DistanceKm != 75.5 {
		t.Errorf("DistanceKm = %f, want 75.5", n.DistanceKm)
	}
	if n.LocationName != "Beach House" {
		t.Errorf("LocationName = %q, want Beach House", n.LocationName)
	}
	if n.Instructions != "Evacuate coastal areas" {
		t.Errorf("Instructions = %q, want 'Evacuate coastal areas'", n.Instructions)
	}
	if n.ContentType != "alert/disaster" {
		t.Errorf("ContentType = %q, want alert/disaster", n.ContentType)
	}
}

// Integration Test 1: Full sync with extreme earthquake triggers notification
func TestSync_ExtremeEarthquake_NotifiesAlert(t *testing.T) {
	// Serve a M7.5 earthquake close to Home
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"type":"FeatureCollection","features":[{
			"id":"us7000extreme",
			"properties":{"mag":7.5,"place":"10km SE of Home","time":1700000000000},
			"geometry":{"type":"Point","coordinates":[-122.30,37.70,10.0]}
		}]}`)
	}))
	defer ts.Close()

	mn := &mockNotifier{}
	c := New("test")
	c.baseURL = ts.URL
	c.Notifier = mn

	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
			},
			"source_weather": false,
		},
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}

	if mn.count() != 1 {
		t.Fatalf("expected 1 notification for extreme earthquake, got %d", mn.count())
	}
	n := mn.last()
	if n.Severity != "extreme" {
		t.Errorf("notification severity = %q, want extreme", n.Severity)
	}
	if n.AlertID != "us7000extreme" {
		t.Errorf("notification alert_id = %q, want us7000extreme", n.AlertID)
	}
}

// Integration Test 2: Moderate weather does NOT trigger notification
func TestSync_ModerateWeather_NoNotification(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"type":"FeatureCollection","features":[{
			"properties":{
				"id":"NWS-moderate-1",
				"event":"Wind Advisory",
				"severity":"Moderate",
				"certainty":"Likely",
				"urgency":"Expected",
				"headline":"Wind Advisory for County",
				"description":"Winds 30-40 mph",
				"instruction":"Secure loose objects",
				"areaDesc":"Test County",
				"effective":"2025-01-01T00:00:00Z",
				"expires":"2025-01-02T00:00:00Z"
			}
		}]}`)
	}))
	defer ts.Close()

	mn := &mockNotifier{}
	c := New("test")
	c.nwsBaseURL = ts.URL
	c.Notifier = mn

	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
			},
			"source_weather": true,
		},
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	// Override source_earthquake to false
	c.config.SourceEarthquake = false

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}

	if mn.count() != 0 {
		t.Errorf("expected 0 notifications for moderate weather, got %d", mn.count())
	}
}

// Integration Test 3: Travel location with expanded radius picks up distant alerts
func TestSync_TravelLocation_ExpandedRadius(t *testing.T) {
	// Earthquake at ~400km from travel location (within 2x radius of 300km = 600km, outside normal 300km)
	// Travel destination: NYC (40.71, -74.01) with radius 300km
	// Earthquake at ~400km from NYC (approximately Washington DC area: 38.90, -77.04)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"type":"FeatureCollection","features":[{
			"id":"us7000travel",
			"properties":{"mag":6.0,"place":"Near Washington DC","time":1700000000000},
			"geometry":{"type":"Point","coordinates":[-77.04,38.90,10.0]}
		}]}`)
	}))
	defer ts.Close()

	mn := &mockNotifier{}
	c := New("test")
	c.baseURL = ts.URL
	c.Notifier = mn

	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home-SF", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
			},
			"travel_locations": []interface{}{
				map[string]interface{}{"name": "Trip-NYC", "latitude": 40.71, "longitude": -74.01, "radius_km": 300.0},
			},
			"source_weather": false,
		},
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// The earthquake at Washington DC (~330km from NYC) should be within 2x travel radius (600km)
	// but NOT within home radius (SF is ~3900km away from DC) or normal travel radius (300km)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact from expanded travel radius, got %d", len(artifacts))
	}

	// Verify it matched the travel location
	meta := artifacts[0].Metadata
	if loc, ok := meta["nearest_location"].(string); ok {
		if loc != "Trip-NYC" {
			t.Errorf("expected nearest_location Trip-NYC, got %s", loc)
		}
	}
}

// E2E Test: NATSAlertNotifier publishes correctly structured JSON
func TestNATSAlertNotifier_PublishesJSON(t *testing.T) {
	var published struct {
		subject string
		data    []byte
	}

	notifier := &NATSAlertNotifier{
		PublishFn: func(_ context.Context, subject string, data []byte) error {
			published.subject = subject
			published.data = data
			return nil
		},
		Subject: "alerts.notify",
	}

	payload := AlertNotification{
		AlertID:      "eq-test-001",
		Headline:     "M8.0 Earthquake — Pacific Ocean",
		Severity:     "extreme",
		Source:       "usgs",
		DistanceKm:   150.0,
		LocationName: "Coastal Home",
		Instructions: "Move to higher ground",
		ContentType:  "alert/earthquake",
		Metadata: map[string]interface{}{
			"magnitude": 8.0,
		},
	}

	err := notifier.NotifyAlert(context.Background(), payload)
	if err != nil {
		t.Fatalf("NotifyAlert: %v", err)
	}

	if published.subject != "alerts.notify" {
		t.Errorf("published to %q, want alerts.notify", published.subject)
	}

	// Verify JSON structure
	var decoded AlertNotification
	if err := json.Unmarshal(published.data, &decoded); err != nil {
		t.Fatalf("unmarshal published data: %v", err)
	}
	if decoded.AlertID != "eq-test-001" {
		t.Errorf("decoded AlertID = %q, want eq-test-001", decoded.AlertID)
	}
	if decoded.Severity != "extreme" {
		t.Errorf("decoded Severity = %q, want extreme", decoded.Severity)
	}
	if decoded.DistanceKm != 150.0 {
		t.Errorf("decoded DistanceKm = %f, want 150.0", decoded.DistanceKm)
	}
	if decoded.Instructions != "Move to higher ground" {
		t.Errorf("decoded Instructions = %q", decoded.Instructions)
	}
}

// --- Regression test: source config flags wiring ---

// TestParseAlertsConfig_AllSourceFlags verifies ALL source flags are parsed from config
// and not silently ignored. This is a regression test for the bug where main.go did not
// wire GOV_ALERTS_SOURCE_* env vars into SourceConfig, causing user config to be ignored.
func TestParseAlertsConfig_AllSourceFlags(t *testing.T) {
	cfg, err := parseAlertsConfig(connector.ConnectorConfig{
		Credentials: map[string]string{"airnow_api_key": "test-key-123"},
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
			},
			"source_weather":  false,
			"source_tsunami":  true,
			"source_volcano":  true,
			"source_wildfire": true,
			"source_airnow":   true,
			"source_gdacs":    true,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Earthquake defaults to true (no explicit source_earthquake key parsed).
	if !cfg.SourceEarthquake {
		t.Error("SourceEarthquake should default to true")
	}

	// Weather explicitly set to false.
	if cfg.SourceWeather {
		t.Error("SourceWeather should be false when explicitly set")
	}

	// All optional sources explicitly enabled.
	if !cfg.SourceTsunami {
		t.Error("SourceTsunami should be true when source_tsunami=true")
	}
	if !cfg.SourceVolcano {
		t.Error("SourceVolcano should be true when source_volcano=true")
	}
	if !cfg.SourceWildfire {
		t.Error("SourceWildfire should be true when source_wildfire=true")
	}
	if !cfg.SourceAirNow {
		t.Error("SourceAirNow should be true when source_airnow=true")
	}
	if !cfg.SourceGDACS {
		t.Error("SourceGDACS should be true when source_gdacs=true")
	}
	if cfg.AirNowAPIKey != "test-key-123" {
		t.Errorf("AirNowAPIKey = %q, want test-key-123", cfg.AirNowAPIKey)
	}
}

// =====================================================
// Partial Failure Resilience Tests (Stabilize R1)
// =====================================================

// TestSync_PartialFailure_USGSDown_NWSUp verifies that when USGS is down,
// NWS weather alerts are still returned (not aborted).
func TestSync_PartialFailure_USGSDown_NWSUp(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "fdsnws") {
			// USGS is down
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// NWS returns valid alerts
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(nwsResponse([]map[string]interface{}{
			makeNWSFeature(
				"urn:oid:partial-nws-1", "Heat Advisory", "Moderate", "Likely", "Expected",
				"Heat Advisory for SF", "Stay hydrated.", "", "San Francisco",
				"2024-01-15T14:30:00-06:00", "2024-01-15T18:30:00-06:00",
			),
		}))
	}))
	defer ts.Close()

	c := New("gov-alerts-partial-test")
	c.baseURL = ts.URL
	c.nwsBaseURL = ts.URL
	c.config = AlertsConfig{
		Locations:        []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceEarthquake: true,
		SourceWeather:    true,
		MinEarthquakeMag: 2.5,
	}

	arts, cursor, err := c.Sync(context.Background(), "")

	// Error should be non-nil (USGS failed)
	if err == nil {
		t.Fatal("expected error when USGS fails")
	}
	if !strings.Contains(err.Error(), "usgs earthquake fetch") {
		t.Errorf("error should mention USGS failure, got: %v", err)
	}

	// Weather alerts should still be present despite USGS failure
	if len(arts) != 1 {
		t.Fatalf("expected 1 NWS artifact despite USGS failure, got %d", len(arts))
	}
	if arts[0].ContentType != "alert/weather" {
		t.Errorf("expected alert/weather, got %s", arts[0].ContentType)
	}

	// Cursor should be valid
	if cursor == "" {
		t.Error("expected non-empty cursor")
	}

	// Health should be degraded (partial failure)
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Errorf("expected degraded health on partial failure, got %s", c.Health(context.Background()))
	}
}

// TestSync_PartialFailure_NWSDown_USGSUp verifies that when NWS is down,
// USGS earthquake results are still returned.
func TestSync_PartialFailure_NWSDown_USGSUp(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "fdsnws") {
			// USGS returns valid earthquakes
			w.Header().Set("Content-Type", "application/json")
			w.Write(usgsResponse([]map[string]interface{}{
				makeFeature("eq-partial-1", 4.5, -122.42, 37.77, 10, "Near Home"),
			}))
			return
		}
		// NWS is down
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	c := New("gov-alerts-partial-test-2")
	c.baseURL = ts.URL
	c.nwsBaseURL = ts.URL
	c.config = AlertsConfig{
		Locations:        []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceEarthquake: true,
		SourceWeather:    true,
		MinEarthquakeMag: 2.5,
	}

	arts, _, err := c.Sync(context.Background(), "")

	// Error should be non-nil (NWS failed)
	if err == nil {
		t.Fatal("expected error when NWS fails")
	}
	if !strings.Contains(err.Error(), "nws weather fetch") {
		t.Errorf("error should mention NWS failure, got: %v", err)
	}

	// Earthquake results should still be present despite NWS failure
	if len(arts) != 1 {
		t.Fatalf("expected 1 USGS artifact despite NWS failure, got %d", len(arts))
	}
	if arts[0].ContentType != "alert/earthquake" {
		t.Errorf("expected alert/earthquake, got %s", arts[0].ContentType)
	}
}

// TestSync_AllSourcesFail verifies error accumulation when all enabled sources fail.
func TestSync_AllSourcesFail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	c := New("gov-alerts-all-fail")
	c.baseURL = ts.URL
	c.nwsBaseURL = ts.URL
	c.tsunamiBaseURL = ts.URL
	c.config = AlertsConfig{
		Locations:        []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceEarthquake: true,
		SourceWeather:    true,
		SourceTsunami:    true,
		MinEarthquakeMag: 2.5,
	}

	arts, _, err := c.Sync(context.Background(), "")

	// All sources failed → error should contain all failures
	if err == nil {
		t.Fatal("expected error when all sources fail")
	}
	if !strings.Contains(err.Error(), "usgs earthquake fetch") {
		t.Errorf("error should mention USGS failure, got: %v", err)
	}
	if !strings.Contains(err.Error(), "nws weather fetch") {
		t.Errorf("error should mention NWS failure, got: %v", err)
	}
	if !strings.Contains(err.Error(), "noaa tsunami fetch") {
		t.Errorf("error should mention tsunami failure, got: %v", err)
	}

	// No artifacts
	if len(arts) != 0 {
		t.Errorf("expected 0 artifacts when all sources fail, got %d", len(arts))
	}

	// Health should be degraded
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Errorf("expected degraded when all sources fail, got %s", c.Health(context.Background()))
	}
}

// TestSync_PartialFailure_OneNWSLocationFails verifies that when one NWS location
// fails, other locations are still fetched.
func TestSync_PartialFailure_OneNWSLocationFails(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First location request fails
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Second location request succeeds
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(nwsResponse([]map[string]interface{}{
			makeNWSFeature(
				"urn:oid:partial-loc-2", "Wind Advisory", "Minor", "Possible", "Expected",
				"Wind Advisory", "Gusty winds.", "", "Area 2",
				"2024-01-15T14:30:00-06:00", "2024-01-15T18:30:00-06:00",
			),
		}))
	}))
	defer ts.Close()

	c := New("gov-alerts-partial-loc")
	c.nwsBaseURL = ts.URL
	c.config = AlertsConfig{
		Locations: []LocationConfig{
			{Name: "Location1", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
			{Name: "Location2", Latitude: 40.71, Longitude: -74.01, RadiusKm: 200},
		},
		SourceEarthquake: false,
		SourceWeather:    true,
	}

	arts, _, err := c.Sync(context.Background(), "")

	// Error should mention Location1 failure
	if err == nil {
		t.Fatal("expected error when one NWS location fails")
	}
	if !strings.Contains(err.Error(), "nws weather fetch for Location1") {
		t.Errorf("error should mention Location1 failure, got: %v", err)
	}

	// Location2 alert should still be present
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact from Location2, got %d", len(arts))
	}
}

// TestSync_PartialFailure_SuccessfulSyncAfterPartial verifies that a subsequent
// fully-successful sync restores health to healthy after a partial failure.
func TestSync_PartialFailure_SuccessfulSyncAfterPartial(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount <= 2 {
			// First sync: USGS fails, NWS succeeds
			if strings.Contains(r.URL.Path, "fdsnws") {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Write(nwsResponse([]map[string]interface{}{
				makeNWSFeature("urn:oid:recovery-1", "Heat Advisory", "Moderate", "Likely", "Expected",
					"Heat", "desc", "", "Area", "2024-01-15T14:30:00-06:00", "2024-01-15T18:30:00-06:00"),
			}))
		} else {
			// Second sync: both succeed
			if strings.Contains(r.URL.Path, "fdsnws") {
				w.Write(usgsResponse(nil))
			} else {
				w.Write(nwsResponse(nil))
			}
		}
	}))
	defer ts.Close()

	c := New("gov-alerts-recovery")
	c.baseURL = ts.URL
	c.nwsBaseURL = ts.URL
	c.config = AlertsConfig{
		Locations:        []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceEarthquake: true,
		SourceWeather:    true,
		MinEarthquakeMag: 2.5,
	}

	// First sync: partial failure
	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Fatal("expected error on partial failure")
	}
	if c.Health(context.Background()) != connector.HealthDegraded {
		t.Fatalf("expected degraded after partial failure, got %s", c.Health(context.Background()))
	}

	// Second sync: full success → health restored
	_, _, err = c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("expected no error on full success, got: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("expected healthy after full success, got %s", c.Health(context.Background()))
	}
}

// =====================================================
// Security: URL Scheme Validation Tests
// =====================================================

// TestSanitizeExternalURL verifies URL scheme allowlisting (http/https only).
func TestSanitizeExternalURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"https URL preserved", "https://www.tsunami.gov/events/001", "https://www.tsunami.gov/events/001"},
		{"http URL preserved", "http://example.com/alert", "http://example.com/alert"},
		{"javascript scheme rejected", "javascript:alert(1)", ""},
		{"data scheme rejected", "data:text/html,<script>alert(1)</script>", ""},
		{"vbscript scheme rejected", "vbscript:MsgBox", ""},
		{"ftp scheme rejected", "ftp://example.com/file", ""},
		{"empty string returns empty", "", ""},
		{"whitespace only returns empty", "   ", ""},
		{"no scheme returns empty", "no-scheme-url", ""},
		{"HTTPS upper case preserved", "HTTPS://WWW.TSUNAMI.GOV/events", "HTTPS://WWW.TSUNAMI.GOV/events"},
		{"mixed case javascript rejected", "JavaScript:alert(1)", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeExternalURL(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeExternalURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestTsunamiAlerts_JavascriptURLRejected verifies malicious URLs in tsunami feeds are sanitized.
func TestTsunamiAlerts_JavascriptURLRejected(t *testing.T) {
	entries := []string{
		makeTsunamiEntryWithGeo("tsunami-xss-1", "Tsunami Warning", "XSS test", "javascript:alert(document.cookie)", "2024-03-15T10:30:00Z", "37.50 -122.10"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, tsunamiAtomXML(entries))
	}))
	defer ts.Close()

	c := New("test-xss")
	c.tsunamiBaseURL = ts.URL
	c.config = AlertsConfig{
		Locations:     []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceTsunami: true,
	}

	arts, _, _ := c.Sync(context.Background(), "")
	if len(arts) == 0 {
		t.Fatal("expected artifact from tsunami feed")
	}
	if arts[0].URL != "" {
		t.Errorf("expected empty URL for javascript: scheme, got %q", arts[0].URL)
	}
}

// TestWildfireAlerts_DataURLRejected verifies malicious URLs in wildfire feeds are sanitized.
func TestWildfireAlerts_DataURLRejected(t *testing.T) {
	items := []string{
		makeWildfireItem("wf-xss-1", "Camp Fire", "Large wildfire", "data:text/html,<script>alert(1)</script>", "Mon, 15 Jul 2024 10:30:00 +0000"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, wildfireRSSXML(items))
	}))
	defer ts.Close()

	c := New("test-xss")
	c.wildfireBaseURL = ts.URL
	c.config = AlertsConfig{
		Locations:      []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceWildfire: true,
	}

	arts, _, _ := c.Sync(context.Background(), "")
	if len(arts) == 0 {
		t.Fatal("expected artifact from wildfire feed")
	}
	if arts[0].URL != "" {
		t.Errorf("expected empty URL for data: scheme, got %q", arts[0].URL)
	}
}

// TestGDACSAlerts_VbscriptURLRejected verifies malicious URLs in GDACS feeds are sanitized.
func TestGDACSAlerts_VbscriptURLRejected(t *testing.T) {
	items := []string{
		makeGDACSItem("gdacs-xss-1", "Flood", "Flood alert", "vbscript:MsgBox", "Mon, 15 Jul 2024 10:30:00 +0000", "Red", "37.0 -122.0"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, gdacsRSSXML(items))
	}))
	defer ts.Close()

	c := New("test-xss")
	c.gdacsBaseURL = ts.URL
	c.config = AlertsConfig{
		Locations:   []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceGDACS: true,
	}

	arts, _, _ := c.Sync(context.Background(), "")
	if len(arts) == 0 {
		t.Fatal("expected artifact from GDACS feed")
	}
	if arts[0].URL != "" {
		t.Errorf("expected empty URL for vbscript: scheme, got %q", arts[0].URL)
	}
}

// =====================================================
// Security: GDACS Coordinate Validation Tests
// =====================================================

// TestNormalizeGDACSAlert_InvalidCoordinatesRejected verifies NaN/Inf/out-of-range lat/lon from geo_point are not stored.
func TestNormalizeGDACSAlert_InvalidCoordinatesRejected(t *testing.T) {
	tests := []struct {
		name     string
		geoPoint string
		wantLat  bool
	}{
		{"valid coordinates", "37.5 -122.4", true},
		{"latitude out of range", "95.0 -122.4", false},
		{"longitude out of range", "37.5 -200.0", false},
		{"NaN latitude", "NaN -122.4", false},
		{"Inf longitude", "37.5 Inf", false},
		{"both invalid", "999.0 999.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alert := GDACSAlert{
				ID:       "gdacs-coord-test",
				Title:    "Test",
				GeoPoint: tt.geoPoint,
				Severity: "moderate",
			}
			artifact := normalizeGDACSAlert(alert, nil)
			_, hasLat := artifact.Metadata["latitude"]
			_, hasLon := artifact.Metadata["longitude"]
			if hasLat != tt.wantLat || hasLon != tt.wantLat {
				t.Errorf("geoPoint=%q: hasLat=%v hasLon=%v, wantBoth=%v", tt.geoPoint, hasLat, hasLon, tt.wantLat)
			}
			// geo_point string should still be present regardless
			if _, hasGP := artifact.Metadata["geo_point"]; !hasGP {
				t.Error("geo_point should always be present when GeoPoint is non-empty")
			}
		})
	}
}

// =====================================================
// Hardening Tests (H-017 — Round R02)
// =====================================================

// TestParseGeoPoint verifies georss:point parsing.
func TestParseGeoPoint(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		lat    float64
		lon    float64
		wantOK bool
	}{
		{"valid point", "-6.5 110.4", -6.5, 110.4, true},
		{"positive coords", "35.6 139.7", 35.6, 139.7, true},
		{"empty string", "", 0, 0, false},
		{"single value", "37.7", 0, 0, false},
		{"three values", "37.7 -122.4 10.0", 0, 0, false},
		{"non-numeric lat", "abc 110.4", 0, 0, false},
		{"non-numeric lon", "-6.5 xyz", 0, 0, false},
		{"whitespace only", "   ", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lat, lon, ok := parseGeoPoint(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("parseGeoPoint(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if ok {
				if lat != tt.lat || lon != tt.lon {
					t.Errorf("parseGeoPoint(%q) = (%v, %v), want (%v, %v)", tt.input, lat, lon, tt.lat, tt.lon)
				}
			}
		})
	}
}

// TestSync_GDACS_ProximityFiltered verifies GDACS alerts outside user radius are skipped (H-017-001).
func TestSync_GDACS_ProximityFiltered(t *testing.T) {
	items := []string{
		// Nearby alert: Jakarta (-6.2, 106.8) — ~0km from configured location
		makeGDACSItem("gdacs-nearby", "EQ Jakarta", "Earthquake in Jakarta.", "https://gdacs.org/1", "Mon, 15 Jul 2024 10:30:00 +0000", "Red", "-6.2 106.8"),
		// Distant alert: Tokyo (35.7, 139.7) — ~5000km from Jakarta
		makeGDACSItem("gdacs-distant", "EQ Tokyo", "Earthquake in Tokyo.", "https://gdacs.org/2", "Mon, 15 Jul 2024 11:00:00 +0000", "Orange", "35.7 139.7"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, gdacsRSSXML(items))
	}))
	defer ts.Close()

	c := New("test")
	c.gdacsBaseURL = ts.URL
	c.config = AlertsConfig{
		Locations:   []LocationConfig{{Name: "Jakarta-Office", Latitude: -6.2, Longitude: 106.8, RadiusKm: 500}},
		SourceGDACS: true,
	}

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	// Only the nearby alert should pass proximity filtering
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact (distant filtered out), got %d", len(arts))
	}
	if arts[0].SourceRef != "gdacs-nearby" {
		t.Errorf("expected gdacs-nearby, got %s", arts[0].SourceRef)
	}
	// Verify proximity metadata
	if dist, ok := arts[0].Metadata["distance_km"].(float64); !ok || dist > 500 {
		t.Errorf("expected distance_km <= 500, got %v", arts[0].Metadata["distance_km"])
	}
	if loc, ok := arts[0].Metadata["nearest_location"].(string); !ok || loc != "Jakarta-Office" {
		t.Errorf("expected nearest_location Jakarta-Office, got %v", arts[0].Metadata["nearest_location"])
	}
}

// TestSync_GDACS_NoGeoPoint_Skipped verifies GDACS alerts without coordinates are skipped (H-017-001).
func TestSync_GDACS_NoGeoPoint_Skipped(t *testing.T) {
	items := []string{
		makeGDACSItem("gdacs-no-geo", "Flood Alert", "Flooding in unknown area.", "https://gdacs.org/3", "Mon, 15 Jul 2024 10:30:00 +0000", "Orange", ""),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, gdacsRSSXML(items))
	}))
	defer ts.Close()

	c := New("test")
	c.gdacsBaseURL = ts.URL
	c.config = AlertsConfig{
		Locations:   []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 500}},
		SourceGDACS: true,
	}

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(arts) != 0 {
		t.Errorf("expected 0 artifacts (no geo_point → skipped), got %d", len(arts))
	}
}

// TestSync_GDACS_InvalidGeoPoint_Skipped verifies GDACS alerts with unparseable coordinates are skipped.
func TestSync_GDACS_InvalidGeoPoint_Skipped(t *testing.T) {
	items := []string{
		makeGDACSItem("gdacs-bad-coords", "Quake", "Bad coords.", "https://gdacs.org/4", "Mon, 15 Jul 2024 10:30:00 +0000", "Red", "not-a-number also-bad"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, gdacsRSSXML(items))
	}))
	defer ts.Close()

	c := New("test")
	c.gdacsBaseURL = ts.URL
	c.config = AlertsConfig{
		Locations:   []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 500}},
		SourceGDACS: true,
	}

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(arts) != 0 {
		t.Errorf("expected 0 artifacts (invalid geo_point → skipped), got %d", len(arts))
	}
}

// TestSync_NWS_LocationNameInMatch verifies NWS proximity match uses the querying location name (H-017-002).
func TestSync_NWS_LocationNameInMatch(t *testing.T) {
	features := []map[string]interface{}{
		makeNWSFeature(
			"urn:oid:nws-loc-name", "Heat Advisory", "Moderate", "Likely", "Expected",
			"Heat Advisory", "Stay cool.", "", "Metro Area",
			"2024-01-15T14:30:00-06:00", "2024-01-15T18:30:00-06:00",
		),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		w.Write(nwsResponse(features))
	}))
	defer ts.Close()

	c := newNWSTestConnector(ts.URL, []LocationConfig{
		{Name: "MyOffice", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	// Verify the proximity match uses the querying location name, not a random match
	if loc, ok := arts[0].Metadata["nearest_location"].(string); !ok || loc != "MyOffice" {
		t.Errorf("nearest_location = %v, want MyOffice", arts[0].Metadata["nearest_location"])
	}
	if dist, ok := arts[0].Metadata["distance_km"].(float64); !ok || dist != 0 {
		t.Errorf("distance_km = %v, want 0 (NWS point-filtered)", arts[0].Metadata["distance_km"])
	}
}

// TestFetchAirNowAQI_APIKeyRedacted verifies the API key is not exposed in error messages (H-017-003, CWE-532).
func TestFetchAirNowAQI_APIKeyRedacted(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer ts.Close()

	c := New("test")
	c.airnowBaseURL = ts.URL

	secretKey := "super-secret-api-key-12345"
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.fetchAirNowAQI(ctx, 37.77, -122.42, secretKey)
	if err == nil {
		t.Fatal("expected error for timeout")
	}
	if strings.Contains(err.Error(), secretKey) {
		t.Errorf("error message contains API key (CWE-532): %s", err.Error())
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Errorf("error message should contain [REDACTED]: %s", err.Error())
	}
}

// TestSync_Volcano_NoNotification verifies volcano alerts don't trigger proactive notifications
// because they lack coordinates for proximity verification (H-017-004).
func TestSync_Volcano_NoNotification(t *testing.T) {
	entries := []map[string]interface{}{
		{"id": "vol-severe-1", "volcanoName": "Mount Danger", "alertLevel": "WARNING", "colorCode": "RED", "issuedDate": "2024-04-01T12:00:00Z"},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(volcanoJSON(entries))
	}))
	defer ts.Close()

	mn := &mockNotifier{}
	c := New("test")
	c.volcanoBaseURL = ts.URL
	c.Notifier = mn
	c.config = AlertsConfig{
		Locations:     []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceVolcano: true,
	}

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 volcano artifact, got %d", len(arts))
	}
	// Volcano alerts should NOT trigger notifications (no proximity check possible)
	if mn.count() != 0 {
		t.Errorf("expected 0 notifications for volcano (no proximity data), got %d", mn.count())
	}
}

// TestSync_Wildfire_NoNotification verifies wildfire alerts don't trigger proactive notifications
// because they lack coordinates for proximity verification (H-017-004).
func TestSync_Wildfire_NoNotification(t *testing.T) {
	items := []string{
		makeWildfireItem("fire-evac-1", "Major Fire", "Evacuation ordered for area.", "https://inciweb.wildfire.gov/fire-evac-1", "Mon, 15 Jul 2024 10:30:00 +0000"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, wildfireRSSXML(items))
	}))
	defer ts.Close()

	mn := &mockNotifier{}
	c := New("test")
	c.wildfireBaseURL = ts.URL
	c.Notifier = mn
	c.config = AlertsConfig{
		Locations:      []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceWildfire: true,
	}

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 wildfire artifact, got %d", len(arts))
	}
	// Wildfire alerts should NOT trigger notifications (no proximity check possible)
	if mn.count() != 0 {
		t.Errorf("expected 0 notifications for wildfire (no proximity data), got %d", mn.count())
	}
}

// TestSync_GDACS_NearbyNotifies verifies that proximity-checked GDACS alerts DO trigger notifications.
func TestSync_GDACS_NearbyNotifies(t *testing.T) {
	items := []string{
		makeGDACSItem("gdacs-red-nearby", "Major Earthquake", "Devastating quake.", "https://gdacs.org/5", "Mon, 15 Jul 2024 10:30:00 +0000", "Red", "37.77 -122.42"),
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, gdacsRSSXML(items))
	}))
	defer ts.Close()

	mn := &mockNotifier{}
	c := New("test")
	c.gdacsBaseURL = ts.URL
	c.Notifier = mn
	c.config = AlertsConfig{
		Locations:   []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 500}},
		SourceGDACS: true,
	}

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 GDACS artifact, got %d", len(arts))
	}
	// Red-level GDACS alert near user SHOULD trigger notification after proximity check
	if mn.count() != 1 {
		t.Errorf("expected 1 notification for nearby Red GDACS alert, got %d", mn.count())
	}
}

// --- IMP-017-R24-001: source_earthquake config toggle must be readable ---

func TestParseAlertsConfig_SourceEarthquakeToggle(t *testing.T) {
	// Default: SourceEarthquake is true.
	cfg, err := parseAlertsConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.SourceEarthquake {
		t.Error("SourceEarthquake should default to true")
	}

	// Explicit disable.
	cfg, err = parseAlertsConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
			},
			"source_earthquake": false,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SourceEarthquake {
		t.Error("SourceEarthquake should be false when source_earthquake=false in config")
	}

	// Explicit enable.
	cfg, err = parseAlertsConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
			},
			"source_earthquake": true,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.SourceEarthquake {
		t.Error("SourceEarthquake should be true when source_earthquake=true in config")
	}
}

// TestSync_EarthquakeDisabledViaConfig verifies that setting source_earthquake=false stops USGS polling.
func TestSync_EarthquakeDisabledViaConfig(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"type":"FeatureCollection","features":[]}`)
	}))
	defer ts.Close()

	c := New("test")
	c.baseURL = ts.URL
	c.config = AlertsConfig{
		Locations:        []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceEarthquake: false,
	}

	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if called {
		t.Error("USGS endpoint was called despite source_earthquake=false; earthquake polling was not disabled")
	}
}

// --- IMP-017-R24-002: radius_km Inf/NaN must be rejected ---

func TestParseAlertsConfig_RadiusInfNaN(t *testing.T) {
	tests := []struct {
		name     string
		radius   float64
		wantLocs int
	}{
		{"valid radius", 200.0, 1},
		{"+Inf radius", math.Inf(1), 0},
		{"-Inf radius", math.Inf(-1), 0},
		{"NaN radius", math.NaN(), 0},
		{"zero radius", 0.0, 0},
		{"negative radius", -50.0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseAlertsConfig(connector.ConnectorConfig{
				SourceConfig: map[string]interface{}{
					"locations": []interface{}{
						map[string]interface{}{
							"name":      "Home",
							"latitude":  37.77,
							"longitude": -122.42,
							"radius_km": tt.radius,
						},
					},
				},
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(cfg.Locations) != tt.wantLocs {
				t.Errorf("radius_km=%v: expected %d locations, got %d", tt.radius, tt.wantLocs, len(cfg.Locations))
			}
		})
	}
}

func TestParseAlertsConfig_TravelRadiusInfNaN(t *testing.T) {
	tests := []struct {
		name     string
		radius   float64
		wantLocs int
	}{
		{"valid travel radius", 300.0, 1},
		{"+Inf travel radius", math.Inf(1), 0},
		{"NaN travel radius", math.NaN(), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseAlertsConfig(connector.ConnectorConfig{
				SourceConfig: map[string]interface{}{
					"locations": []interface{}{
						map[string]interface{}{
							"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0,
						},
					},
					"travel_locations": []interface{}{
						map[string]interface{}{
							"name":      "Tokyo",
							"latitude":  35.68,
							"longitude": 139.69,
							"radius_km": tt.radius,
						},
					},
				},
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(cfg.TravelLocations) != tt.wantLocs {
				t.Errorf("travel radius_km=%v: expected %d travel locations, got %d", tt.radius, tt.wantLocs, len(cfg.TravelLocations))
			}
		})
	}
}

// TestIsFinitePositiveRadius directly tests the guard function.
func TestIsFinitePositiveRadius(t *testing.T) {
	tests := []struct {
		r    float64
		want bool
	}{
		{200.0, true},
		{0.001, true},
		{0, false},
		{-1, false},
		{math.Inf(1), false},
		{math.Inf(-1), false},
		{math.NaN(), false},
	}
	for _, tt := range tests {
		if got := isFinitePositiveRadius(tt.r); got != tt.want {
			t.Errorf("isFinitePositiveRadius(%v) = %v, want %v", tt.r, got, tt.want)
		}
	}
}

// --- IMP-017-R24-003: AirNow dedup ID must include observation date ---

func TestFetchAirNowAQI_IDIncludesDate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"DateObserved":"2024-07-15 ","HourObserved":12,"AQI":42,"ParameterName":"PM2.5","ReportingArea":"San Francisco","Category":{"Name":"Good"}}]`)
	}))
	defer ts.Close()

	c := New("test")
	c.airnowBaseURL = ts.URL
	obs, err := c.fetchAirNowAQI(context.Background(), 37.77, -122.42, "test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(obs) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(obs))
	}
	// The ID must contain the date so that same-AQI observations on different days
	// are not deduped by the 7-day eviction window.
	if !strings.Contains(obs[0].ID, "2024-07-15") {
		t.Errorf("AirNow observation ID %q must include date '2024-07-15' for temporal dedup isolation", obs[0].ID)
	}
}

func TestAirNowDedup_SameAQIDifferentDays(t *testing.T) {
	// Simulate two observations with identical AQI but different days.
	// Both should produce artifacts if date is in the ID.
	day1Items := `[{"DateObserved":"2024-07-15 ","HourObserved":12,"AQI":50,"ParameterName":"PM2.5","ReportingArea":"Boston","Category":{"Name":"Good"}}]`
	day2Items := `[{"DateObserved":"2024-07-16 ","HourObserved":12,"AQI":50,"ParameterName":"PM2.5","ReportingArea":"Boston","Category":{"Name":"Good"}}]`

	callCount := 0
	responses := []string{day1Items, day2Items}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if callCount < len(responses) {
			fmt.Fprint(w, responses[callCount])
		} else {
			fmt.Fprint(w, `[]`)
		}
		callCount++
	}))
	defer ts.Close()

	c := New("test")
	c.airnowBaseURL = ts.URL
	c.config = AlertsConfig{
		Locations:    []LocationConfig{{Name: "Home", Latitude: 42.36, Longitude: -71.06, RadiusKm: 100}},
		SourceAirNow: true,
		AirNowAPIKey: "test-key",
	}

	// First sync — day 1
	arts1, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync 1 error: %v", err)
	}
	if len(arts1) != 1 {
		t.Fatalf("sync 1: expected 1 artifact, got %d", len(arts1))
	}

	// Second sync — day 2 same AQI. Must produce a NEW artifact, not be deduped.
	arts2, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync 2 error: %v", err)
	}
	if len(arts2) != 1 {
		t.Errorf("sync 2: expected 1 artifact (different date should not dedup), got %d — IMP-017-R24-003 regression", len(arts2))
	}
}

// --- Chaos Tests (C-017-001, C-017-002, C-017-003) ---

// panicNotifier is a test notifier that panics on NotifyAlert.
type panicNotifier struct{}

func (p *panicNotifier) NotifyAlert(ctx context.Context, payload AlertNotification) error {
	panic("notifier exploded")
}

// badTravelProvider returns invalid locations to test validation bypass.
type badTravelProvider struct {
	locations []LocationConfig
}

func (b *badTravelProvider) GetTravelLocations(ctx context.Context) ([]LocationConfig, error) {
	return b.locations, nil
}

// TestSync_NotifierPanic_DoesNotCrashSync verifies that a panicking Notifier is recovered
// and does not crash the Sync goroutine (C-017-002).
func TestSync_NotifierPanic_DoesNotCrashSync(t *testing.T) {
	// Build a USGS response with an extreme earthquake (triggers notification).
	feature := makeFeature("panic-eq", 8.0, -122.10, 37.50, 10.0, "Near test site")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(usgsResponse([]map[string]interface{}{feature}))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})
	c.Notifier = &panicNotifier{}

	// Sync must NOT panic — it should recover and continue.
	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error after notifier panic: %v", err)
	}
	if len(arts) != 1 {
		t.Errorf("expected 1 artifact despite notifier panic, got %d", len(arts))
	}
	// Health should be healthy (successful sync), not stuck on syncing.
	if h := c.Health(context.Background()); h != connector.HealthHealthy {
		t.Errorf("expected HealthHealthy after recovered panic, got %v", h)
	}
}

// TestSync_NotifierPanic_ArtifactStillReturned verifies that the artifact that triggered
// the panic is still included in the Sync results (C-017-002).
func TestSync_NotifierPanic_ArtifactStillReturned(t *testing.T) {
	feature := makeFeature("panic-eq-2", 7.5, -122.10, 37.50, 10.0, "Near test site")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(usgsResponse([]map[string]interface{}{feature}))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})
	c.Notifier = &panicNotifier{}

	arts, _, _ := c.Sync(context.Background(), "")
	if len(arts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(arts))
	}
	if arts[0].SourceRef != "panic-eq-2" {
		t.Errorf("expected SourceRef panic-eq-2, got %s", arts[0].SourceRef)
	}
}

// TestClose_DuringSync_HealthRemainsDisconnected verifies that Close() during Sync()
// does not have its HealthDisconnected overwritten by Sync's deferred health restoration (C-017-003).
func TestClose_DuringSync_HealthRemainsDisconnected(t *testing.T) {
	// Use a slow server so Sync is still running when we Close.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Write(usgsResponse(nil))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, _ = c.Sync(context.Background(), "")
	}()

	// Give Sync time to start, then Close.
	time.Sleep(20 * time.Millisecond)
	_ = c.Close()

	wg.Wait()

	// After Sync completes post-Close, health MUST remain Disconnected.
	h := c.Health(context.Background())
	if h != connector.HealthDisconnected {
		t.Errorf("expected HealthDisconnected after Close during Sync, got %v", h)
	}
}

// TestSync_AfterClose_ReturnsError verifies that Sync on a closed connector returns an error (C-017-003).
func TestSync_AfterClose_ReturnsError(t *testing.T) {
	c := New("gov-alerts")
	c.config = AlertsConfig{
		Locations:        []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceEarthquake: false,
	}
	_ = c.Close()

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error when syncing a closed connector")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("expected 'closed' in error message, got: %v", err)
	}
}

// TestConnect_AfterClose_ResetsClosedFlag verifies that Connect after Close re-enables Sync (C-017-003).
func TestConnect_AfterClose_ResetsClosedFlag(t *testing.T) {
	c := New("gov-alerts")
	validConfig := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42, "radius_km": 200.0},
			},
			"source_earthquake": false,
			"source_weather":    false,
		},
	}

	// Connect → Close → Connect should work.
	if err := c.Connect(context.Background(), validConfig); err != nil {
		t.Fatal(err)
	}
	_ = c.Close()

	// Sync should fail on closed connector.
	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Fatal("expected error on closed connector")
	}

	// Reconnect should clear closed flag.
	if err := c.Connect(context.Background(), validConfig); err != nil {
		t.Fatal(err)
	}

	// Sync should succeed now.
	_, _, err = c.Sync(context.Background(), "")
	if err != nil {
		t.Errorf("expected successful Sync after reconnect, got: %v", err)
	}
}

// TestMergedLocations_TravelProviderNaNCoords_Skipped verifies that TravelProvider locations
// with NaN coordinates are rejected before being used in proximity filtering (C-017-001).
func TestMergedLocations_TravelProviderNaNCoords_Skipped(t *testing.T) {
	c := New("gov-alerts")
	c.config = AlertsConfig{
		Locations: []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
	}
	c.TravelProvider = &badTravelProvider{
		locations: []LocationConfig{
			{Name: "NaN-Land", Latitude: math.NaN(), Longitude: -122.0, RadiusKm: 200},
			{Name: "Valid-Travel", Latitude: 40.0, Longitude: -74.0, RadiusKm: 300},
		},
	}

	merged := c.mergedLocations(context.Background(), c.config)
	// Home + Valid-Travel only; NaN-Land must be rejected.
	if len(merged) != 2 {
		t.Errorf("expected 2 merged locations (Home + Valid-Travel), got %d", len(merged))
	}
	for _, loc := range merged {
		if loc.Name == "NaN-Land" {
			t.Error("NaN-Land should have been filtered out")
		}
	}
}

// TestMergedLocations_TravelProviderInfRadius_Skipped verifies that a TravelProvider location
// with very large radius that overflows to +Inf when doubled is rejected (C-017-001).
func TestMergedLocations_TravelProviderInfRadius_Skipped(t *testing.T) {
	c := New("gov-alerts")
	c.config = AlertsConfig{
		Locations: []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
	}
	c.TravelProvider = &badTravelProvider{
		locations: []LocationConfig{
			{Name: "Overflow-City", Latitude: 35.0, Longitude: 139.0, RadiusKm: math.MaxFloat64},
		},
	}

	merged := c.mergedLocations(context.Background(), c.config)
	// Only Home; Overflow-City must be rejected because radius*2 = +Inf.
	if len(merged) != 1 {
		t.Errorf("expected 1 merged location (Home only), got %d", len(merged))
	}
	for _, loc := range merged {
		if loc.Name == "Overflow-City" {
			t.Error("Overflow-City with +Inf radius should have been filtered out")
		}
	}
}

// TestMergedLocations_TravelProviderZeroRadius_Skipped verifies that a TravelProvider
// location with zero radius is rejected (C-017-001).
func TestMergedLocations_TravelProviderZeroRadius_Skipped(t *testing.T) {
	c := New("gov-alerts")
	c.config = AlertsConfig{
		Locations: []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
	}
	c.TravelProvider = &badTravelProvider{
		locations: []LocationConfig{
			{Name: "Zero-Radius", Latitude: 35.0, Longitude: 139.0, RadiusKm: 0},
		},
	}

	merged := c.mergedLocations(context.Background(), c.config)
	if len(merged) != 1 {
		t.Errorf("expected 1 merged location (Home only), got %d", len(merged))
	}
}

// TestProcessingTier_Direct verifies processingTier mapping for all severity levels.
func TestProcessingTier_Direct(t *testing.T) {
	tests := []struct {
		severity string
		want     string
	}{
		{"extreme", "full"},
		{"severe", "full"},
		{"moderate", "standard"},
		{"minor", "light"},
		{"unknown", "light"},
		{"info", "light"},
		{"", "light"},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			got := processingTier(tt.severity)
			if got != tt.want {
				t.Errorf("processingTier(%q) = %q, want %q", tt.severity, got, tt.want)
			}
		})
	}
}

// TestExtractInstructions_Direct verifies instruction extraction from raw content.
func TestExtractInstructions_Direct(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"with instruction prefix", "Headline\n\nInstruction: Take shelter now", "Take shelter now"},
		{"instruction with newline after", "Headline\n\nInstruction: Take cover\nMore info", "Take cover"},
		{"no instruction prefix", "Just a description with no instructions", ""},
		{"empty string", "", ""},
		{"instruction at start", "Instruction: Move to higher ground", "Move to higher ground"},
		{"multiple instruction prefixes", "Instruction: First\nInstruction: Second", "First"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractInstructions(tt.content)
			if got != tt.want {
				t.Errorf("extractInstructions(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}

// errTravelProvider returns an error from GetTravelLocations.
type errTravelProvider struct{}

func (e *errTravelProvider) GetTravelLocations(ctx context.Context) ([]LocationConfig, error) {
	return nil, fmt.Errorf("calendar integration unavailable")
}

// TestMergedLocations_TravelProviderError_FallsBackToConfig verifies that when
// TravelProvider returns an error, mergedLocations falls back to cfg.TravelLocations.
func TestMergedLocations_TravelProviderError_FallsBackToConfig(t *testing.T) {
	c := New("gov-alerts")
	c.config = AlertsConfig{
		Locations: []LocationConfig{
			{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
		},
		TravelLocations: []LocationConfig{
			{Name: "Config-Travel", Latitude: 40.71, Longitude: -74.01, RadiusKm: 150},
		},
	}
	c.TravelProvider = &errTravelProvider{}

	merged := c.mergedLocations(context.Background(), c.config)
	// Should have Home + Config-Travel (from fallback).
	if len(merged) != 2 {
		t.Fatalf("expected 2 merged locations (Home + fallback travel), got %d", len(merged))
	}
	if merged[1].Name != "Config-Travel" {
		t.Errorf("expected Config-Travel from fallback, got %s", merged[1].Name)
	}
	if merged[1].RadiusKm != 300 { // 150 * 2 = 300
		t.Errorf("travel radius should be doubled to 300, got %.0f", merged[1].RadiusKm)
	}
}

// TestFetchVolcanoAlerts_FallbackToVolcanoNameAsID verifies that when a volcano alert
// has an empty ID, it falls back to using the volcano name as the alert ID.
func TestFetchVolcanoAlerts_FallbackToVolcanoNameAsID(t *testing.T) {
	entries := []map[string]interface{}{
		{"id": "", "volcanoName": "Mount Erebus", "alertLevel": "ADVISORY", "colorCode": "YELLOW", "issuedDate": "2024-04-01T12:00:00Z"},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(volcanoJSON(entries))
	}))
	defer ts.Close()

	c := New("test")
	c.volcanoBaseURL = ts.URL

	alerts, err := c.fetchVolcanoAlerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert (ID fallback to volcano name), got %d", len(alerts))
	}
	if alerts[0].ID != "Mount Erebus" {
		t.Errorf("expected ID 'Mount Erebus' (fallback), got %q", alerts[0].ID)
	}
}

// TestFetchVolcanoAlerts_BothIDAndNameEmpty_Skipped verifies that alerts with both
// empty ID and empty volcano name are skipped.
func TestFetchVolcanoAlerts_BothIDAndNameEmpty_Skipped(t *testing.T) {
	entries := []map[string]interface{}{
		{"id": "", "volcanoName": "", "alertLevel": "WATCH", "colorCode": "ORANGE", "issuedDate": "2024-04-01T12:00:00Z"},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(volcanoJSON(entries))
	}))
	defer ts.Close()

	c := New("test")
	c.volcanoBaseURL = ts.URL

	alerts, err := c.fetchVolcanoAlerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts (both ID and name empty), got %d", len(alerts))
	}
}

// TestFetchWildfireAlerts_GUIDFallbackToLink verifies that when GUID is empty,
// the wildfire alert uses the link URL as the ID.
func TestFetchWildfireAlerts_GUIDFallbackToLink(t *testing.T) {
	// Item with empty GUID but valid link — should use link as ID.
	xml := `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel>` +
		`<item><guid></guid><title>Ridge Fire</title><description>Fire near ridge.</description>` +
		`<link>https://inciweb.wildfire.gov/incident/12345</link>` +
		`<pubDate>Mon, 15 Jul 2024 10:30:00 +0000</pubDate></item>` +
		`</channel></rss>`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, xml)
	}))
	defer ts.Close()

	c := New("test")
	c.wildfireBaseURL = ts.URL

	alerts, err := c.fetchWildfireAlerts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert (GUID fallback to link), got %d", len(alerts))
	}
	if alerts[0].ID != "https://inciweb.wildfire.gov/incident/12345" {
		t.Errorf("expected ID from link fallback, got %q", alerts[0].ID)
	}
}

// TestSync_AirNow_ProducesArtifacts verifies full Sync flow with AirNow source producing artifacts.
func TestSync_AirNow_ProducesArtifacts(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(airnowJSON([]map[string]interface{}{
			{
				"DateObserved":  "2024-07-15 ",
				"HourObserved":  14,
				"AQI":           210,
				"ParameterName": "PM2.5",
				"ReportingArea": "San Francisco",
				"Category":      map[string]interface{}{"Name": "Very Unhealthy"},
			},
		}))
	}))
	defer ts.Close()

	mn := &mockNotifier{}
	c := New("test")
	c.airnowBaseURL = ts.URL
	c.Notifier = mn
	c.config = AlertsConfig{
		Locations:    []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
		SourceAirNow: true,
		AirNowAPIKey: "test-key",
	}

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("expected 1 AirNow artifact, got %d", len(arts))
	}
	if arts[0].ContentType != "alert/air-quality" {
		t.Errorf("ContentType = %q, want alert/air-quality", arts[0].ContentType)
	}
	// AQI 210 = severe → should trigger notification
	if mn.count() != 1 {
		t.Errorf("expected 1 notification for severe AQI, got %d", mn.count())
	}
}

// TestMergedLocations_TravelProviderNegativeRadius_Skipped verifies that a TravelProvider
// location with negative radius is rejected (C-017-001).
func TestMergedLocations_TravelProviderNegativeRadius_Skipped(t *testing.T) {
	c := New("gov-alerts")
	c.config = AlertsConfig{
		Locations: []LocationConfig{{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200}},
	}
	c.TravelProvider = &badTravelProvider{
		locations: []LocationConfig{
			{Name: "Negative-Radius", Latitude: 35.0, Longitude: 139.0, RadiusKm: -100},
		},
	}

	merged := c.mergedLocations(context.Background(), c.config)
	if len(merged) != 1 {
		t.Errorf("expected 1 merged location (Home only), got %d", len(merged))
	}
}

// TestSync_TravelProviderInfRadius_NoGlobalMatches verifies end-to-end that a TravelProvider
// returning an overflow radius does NOT cause worldwide earthquake matching (C-017-001).
func TestSync_TravelProviderInfRadius_NoGlobalMatches(t *testing.T) {
	// Return a distant earthquake (Tokyo) that should NOT match Home (SF) or the bad travel loc.
	distantFeature := makeFeature("distant-eq", 5.0, 139.69, 35.68, 10.0, "Tokyo")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(usgsResponse([]map[string]interface{}{distantFeature}))
	}))
	defer ts.Close()

	c := newTestConnector(ts.URL, []LocationConfig{
		{Name: "Home", Latitude: 37.77, Longitude: -122.42, RadiusKm: 200},
	})
	c.TravelProvider = &badTravelProvider{
		locations: []LocationConfig{
			{Name: "Overflow", Latitude: 35.0, Longitude: 139.0, RadiusKm: math.MaxFloat64},
		},
	}

	arts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Distant earthquake must NOT match — the overflow radius should be filtered.
	if len(arts) != 0 {
		t.Errorf("expected 0 artifacts (distant eq + bad travel loc filtered), got %d — C-017-001 regression", len(arts))
	}
}
