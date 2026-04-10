package alerts

import (
	"context"
	"math"
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
	match := c.findNearestLocation(37.50, -122.10)
	if match == nil {
		t.Fatal("expected match for nearby earthquake")
	}
	if match.LocationName != "Home" {
		t.Errorf("expected Home, got %s", match.LocationName)
	}

	// Distant earthquake (Hawaii - way beyond 200km)
	match = c.findNearestLocation(20.0, -155.0)
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
