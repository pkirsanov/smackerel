package maps

import (
	"math"
	"testing"
)

func TestClassifyActivity(t *testing.T) {
	tests := []struct {
		googleType string
		distance   float64
		expected   ActivityType
	}{
		{"WALKING", 1.0, ActivityWalk},
		{"WALKING", 8.0, ActivityHike},
		{"CYCLING", 5.0, ActivityCycle},
		{"IN_VEHICLE", 20.0, ActivityDrive},
		{"IN_BUS", 10.0, ActivityTransit},
		{"RUNNING", 5.0, ActivityRun},
		{"UNKNOWN", 1.0, ActivityWalk},
	}

	for _, tt := range tests {
		got := ClassifyActivity(tt.googleType, tt.distance)
		if got != tt.expected {
			t.Errorf("ClassifyActivity(%q, %.1f) = %q, want %q", tt.googleType, tt.distance, got, tt.expected)
		}
	}
}

func TestIsTrailQualified(t *testing.T) {
	qualified := TakeoutActivity{Type: ActivityHike, DistanceKm: 3.5}
	if !IsTrailQualified(qualified) {
		t.Error("3.5km hike should qualify as trail")
	}

	tooShort := TakeoutActivity{Type: ActivityWalk, DistanceKm: 0.5}
	if IsTrailQualified(tooShort) {
		t.Error("0.5km walk should not qualify as trail")
	}

	driving := TakeoutActivity{Type: ActivityDrive, DistanceKm: 50.0}
	if IsTrailQualified(driving) {
		t.Error("driving should not qualify as trail")
	}
}

func TestToGeoJSON(t *testing.T) {
	route := []LatLng{
		{Lat: 40.7128, Lng: -74.0060},
		{Lat: 40.7580, Lng: -73.9855},
	}

	geojson := ToGeoJSON(route)
	if geojson["type"] != "LineString" {
		t.Errorf("expected LineString, got %v", geojson["type"])
	}

	coords, ok := geojson["coordinates"].([][]float64)
	if !ok || len(coords) != 2 {
		t.Fatalf("expected 2 coordinates")
	}

	// GeoJSON uses [lng, lat]
	if coords[0][0] != -74.0060 {
		t.Errorf("expected lng -74.0060, got %f", coords[0][0])
	}
}

func TestHaversine(t *testing.T) {
	nyc := LatLng{Lat: 40.7128, Lng: -74.0060}
	la := LatLng{Lat: 34.0522, Lng: -118.2437}

	distance := Haversine(nyc, la)
	// NYC to LA is approximately 3940 km
	if math.Abs(distance-3940) > 100 {
		t.Errorf("NYC to LA distance should be ~3940 km, got %.0f km", distance)
	}
}

func TestParseJSON_MalformedInput(t *testing.T) {
	// Completely invalid JSON
	_, err := ParseTakeoutJSON([]byte(`{not valid json`))
	if err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}

	// Valid JSON but wrong structure (missing timelineObjects)
	activities, err := ParseTakeoutJSON([]byte(`{"other": "data"}`))
	if err != nil {
		t.Errorf("unexpected error for empty structure: %v", err)
	}
	if len(activities) != 0 {
		t.Errorf("expected 0 activities for wrong structure, got %d", len(activities))
	}

	// Empty array of timeline objects
	activities, err = ParseTakeoutJSON([]byte(`{"timelineObjects": []}`))
	if err != nil {
		t.Errorf("unexpected error for empty timeline: %v", err)
	}
	if len(activities) != 0 {
		t.Errorf("expected 0 activities for empty timeline, got %d", len(activities))
	}
}

func TestOptInRequired(t *testing.T) {
	// Maps connector processes location data which requires explicit consent.
	// ParseTakeoutJSON must not silently fabricate data from nil/empty input.
	activities, err := ParseTakeoutJSON(nil)
	if err == nil {
		t.Error("expected error when parsing nil input (no consent/data provided)")
	}
	if len(activities) != 0 {
		t.Errorf("expected 0 activities from nil input, got %d", len(activities))
	}

	// Empty byte slice should also produce no activities
	activities, err = ParseTakeoutJSON([]byte{})
	if err == nil {
		t.Error("expected error when parsing empty input")
	}
	if len(activities) != 0 {
		t.Errorf("expected 0 activities from empty input, got %d", len(activities))
	}
}
