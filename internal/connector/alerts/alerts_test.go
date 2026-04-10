package alerts

import (
	"context"
	"testing"

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
	d := HaversineKm(37.7749, -122.4194, 34.0522, -118.2437)
	if d < 500 || d > 600 {
		t.Errorf("SF to LA distance should be ~559 km, got %.0f", d)
	}

	// Same point = 0
	if d := HaversineKm(37.0, -122.0, 37.0, -122.0); d != 0 {
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
