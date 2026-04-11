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
	c.config = AlertsConfig{
		Locations:        locations,
		SourceEarthquake: true,
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
		{"minor severity gets standard tier", 2.0, 500, "standard"},
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

// TestSync_EmptyEarthquakeID verifies that earthquakes with empty IDs still get deduped correctly.
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
	// Both have empty ID, so only the first should be created (second is deduped).
	if len(arts) != 1 {
		t.Errorf("expected 1 artifact (empty ID collision dedup), got %d", len(arts))
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
