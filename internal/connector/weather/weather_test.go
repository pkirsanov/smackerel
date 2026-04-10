package weather

import (
	"context"
	"testing"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestNew(t *testing.T) {
	c := New("weather")
	if c.ID() != "weather" {
		t.Errorf("expected weather, got %s", c.ID())
	}
}

func TestConnect_NoLocations(t *testing.T) {
	c := New("weather")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{},
	})
	if err == nil {
		t.Error("expected error for no locations")
	}
}

func TestConnect_Valid(t *testing.T) {
	c := New("weather")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"locations": []interface{}{
				map[string]interface{}{"name": "Home", "latitude": 37.77, "longitude": -122.42},
			},
		},
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Error("should be healthy after connect")
	}
}

func TestRoundCoords(t *testing.T) {
	lat, lon := roundCoords(37.7749, -122.4194, 2)
	if lat != 37.77 {
		t.Errorf("expected 37.77, got %v", lat)
	}
	if lon != -122.42 {
		t.Errorf("expected -122.42, got %v", lon)
	}
}

func TestWmoCodeToDescription(t *testing.T) {
	tests := []struct {
		code     int
		expected string
	}{
		{0, "Clear sky"},
		{2, "Partly cloudy"},
		{45, "Fog"},
		{55, "Drizzle"},
		{65, "Rain"},
		{75, "Snow"},
		{95, "Thunderstorm"},
		{999, "Unknown"},
	}
	for _, tt := range tests {
		got := wmoCodeToDescription(tt.code)
		if got != tt.expected {
			t.Errorf("wmoCodeToDescription(%d) = %s, want %s", tt.code, got, tt.expected)
		}
	}
}

func TestClose(t *testing.T) {
	c := New("weather")
	c.health = connector.HealthHealthy
	c.Close()
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Error("should be disconnected")
	}
}
