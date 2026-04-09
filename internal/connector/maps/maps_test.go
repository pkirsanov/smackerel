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

	// R003 regression: duration-based qualification per R-404 (>=30 min)
	longWalk := TakeoutActivity{Type: ActivityWalk, DistanceKm: 1.5, DurationMin: 45.0}
	if !IsTrailQualified(longWalk) {
		t.Error("1.5km / 45min walk should qualify as trail by duration (R-404: >=30 min)")
	}

	shortWalk := TakeoutActivity{Type: ActivityWalk, DistanceKm: 1.0, DurationMin: 20.0}
	if IsTrailQualified(shortWalk) {
		t.Error("1.0km / 20min walk should not qualify (below both thresholds)")
	}

	// Cycling requires >=5km per R-404
	shortCycle := TakeoutActivity{Type: ActivityCycle, DistanceKm: 3.0}
	if IsTrailQualified(shortCycle) {
		t.Error("3km cycle should not qualify (cycling threshold is 5km)")
	}

	longCycle := TakeoutActivity{Type: ActivityCycle, DistanceKm: 8.0}
	if !IsTrailQualified(longCycle) {
		t.Error("8km cycle should qualify as trail")
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

func TestParseTakeoutJSON_HappyPath(t *testing.T) {
	// R004 regression: verify ParseTakeoutJSON correctly parses valid Takeout JSON
	input := `{
		"timelineObjects": [
			{
				"activitySegment": {
					"startLocation": {"latitudeE7": 407128000, "longitudeE7": -740060000},
					"endLocation":   {"latitudeE7": 407580000, "longitudeE7": -739855000},
					"duration": {
						"startTimestamp": "2026-03-15T10:00:00Z",
						"endTimestamp":   "2026-03-15T11:30:00Z"
					},
					"distance": 8500,
					"activityType": "WALKING",
					"waypointPath": {
						"waypoints": [
							{"latE7": 407128000, "lngE7": -740060000},
							{"latE7": 407350000, "lngE7": -739950000},
							{"latE7": 407580000, "lngE7": -739855000}
						]
					}
				}
			},
			{
				"activitySegment": {
					"startLocation": {"latitudeE7": 407580000, "longitudeE7": -739855000},
					"endLocation":   {"latitudeE7": 405220000, "longitudeE7": -742437000},
					"duration": {
						"startTimestamp": "2026-03-15T12:00:00Z",
						"endTimestamp":   "2026-03-15T12:45:00Z"
					},
					"distance": 20000,
					"activityType": "IN_VEHICLE",
					"waypointPath": {"waypoints": []}
				}
			}
		]
	}`

	activities, err := ParseTakeoutJSON([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(activities) != 2 {
		t.Fatalf("expected 2 activities, got %d", len(activities))
	}

	// First activity: 8.5km walk → classified as hike (>5km)
	a0 := activities[0]
	if a0.Type != ActivityHike {
		t.Errorf("activity[0].Type = %q, want %q (8.5km walk → hike)", a0.Type, ActivityHike)
	}
	if a0.DistanceKm != 8.5 {
		t.Errorf("activity[0].DistanceKm = %f, want 8.5", a0.DistanceKm)
	}
	if len(a0.Route) != 3 {
		t.Errorf("activity[0].Route has %d waypoints, want 3", len(a0.Route))
	}
	if a0.DurationMin != 90.0 {
		t.Errorf("activity[0].DurationMin = %f, want 90.0", a0.DurationMin)
	}

	// Second activity: 20km drive
	a1 := activities[1]
	if a1.Type != ActivityDrive {
		t.Errorf("activity[1].Type = %q, want %q", a1.Type, ActivityDrive)
	}
}

func TestParseTakeoutJSON_BadTimestamp(t *testing.T) {
	// R004 regression: activities with unparseable timestamps are skipped
	input := `{
		"timelineObjects": [
			{
				"activitySegment": {
					"startLocation": {"latitudeE7": 407128000, "longitudeE7": -740060000},
					"endLocation":   {"latitudeE7": 407580000, "longitudeE7": -739855000},
					"duration": {
						"startTimestamp": "not-a-timestamp",
						"endTimestamp":   "2026-03-15T11:30:00Z"
					},
					"distance": 5000,
					"activityType": "WALKING",
					"waypointPath": {"waypoints": []}
				}
			},
			{
				"activitySegment": {
					"startLocation": {"latitudeE7": 407128000, "longitudeE7": -740060000},
					"endLocation":   {"latitudeE7": 407580000, "longitudeE7": -739855000},
					"duration": {
						"startTimestamp": "2026-03-15T10:00:00Z",
						"endTimestamp":   "2026-03-15T11:00:00Z"
					},
					"distance": 3000,
					"activityType": "CYCLING",
					"waypointPath": {"waypoints": []}
				}
			}
		]
	}`

	activities, err := ParseTakeoutJSON([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First activity has bad timestamp → skipped; only second should parse
	if len(activities) != 1 {
		t.Fatalf("expected 1 activity (bad timestamp skipped), got %d", len(activities))
	}
	if activities[0].Type != ActivityCycle {
		t.Errorf("expected cycling activity, got %q", activities[0].Type)
	}
}
