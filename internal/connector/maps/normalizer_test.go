package maps

import (
	"testing"
	"time"
)

func TestNormalizeActivityMetadata(t *testing.T) {
	activity := TakeoutActivity{
		Type:        ActivityHike,
		StartTime:   time.Date(2026, 3, 15, 13, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 3, 15, 15, 22, 0, 0, time.UTC),
		DistanceKm:  8.3,
		DurationMin: 142,
		Route: []LatLng{
			{Lat: 47.500, Lng: 8.700},
			{Lat: 47.505, Lng: 8.710},
			{Lat: 47.510, Lng: 8.720},
			{Lat: 47.515, Lng: 8.730},
			{Lat: 47.520, Lng: 8.740},
			{Lat: 47.525, Lng: 8.745},
			{Lat: 47.528, Lng: 8.748},
			{Lat: 47.530, Lng: 8.749},
			{Lat: 47.532, Lng: 8.750},
			{Lat: 47.534, Lng: 8.751},
			{Lat: 47.536, Lng: 8.752},
			{Lat: 47.538, Lng: 8.754},
		},
	}

	cfg := MapsConfig{DefaultTier: "standard"}
	artifact := NormalizeActivity(activity, "march-2026.json", cfg)

	if artifact.SourceID != "google-maps-timeline" {
		t.Errorf("SourceID = %q, want %q", artifact.SourceID, "google-maps-timeline")
	}
	if artifact.ContentType != "activity/hike" {
		t.Errorf("ContentType = %q, want %q", artifact.ContentType, "activity/hike")
	}
	if artifact.CapturedAt != activity.StartTime {
		t.Errorf("CapturedAt = %v, want %v", artifact.CapturedAt, activity.StartTime)
	}

	// Check all 17 metadata fields per R-007
	requiredKeys := []string{
		"activity_type", "start_time", "end_time",
		"distance_km", "duration_min", "elevation_m",
		"start_lat", "start_lng", "end_lat", "end_lng",
		"route_geojson", "waypoint_count",
		"trail_qualified", "source_file",
		"dedup_hash", "processing_tier",
	}
	for _, key := range requiredKeys {
		if _, ok := artifact.Metadata[key]; !ok {
			t.Errorf("metadata missing key %q", key)
		}
	}

	if artifact.Metadata["trail_qualified"] != true {
		t.Errorf("trail_qualified = %v, want true", artifact.Metadata["trail_qualified"])
	}
	if artifact.Metadata["waypoint_count"] != 12 {
		t.Errorf("waypoint_count = %v, want 12", artifact.Metadata["waypoint_count"])
	}
	if artifact.Metadata["processing_tier"] != "full" {
		t.Errorf("processing_tier = %v, want %q", artifact.Metadata["processing_tier"], "full")
	}
	if artifact.Metadata["source_file"] != "march-2026.json" {
		t.Errorf("source_file = %v, want %q", artifact.Metadata["source_file"], "march-2026.json")
	}
}

func TestNormalizeActivityTitle(t *testing.T) {
	tests := []struct {
		actType  ActivityType
		distance float64
		duration float64
		want     string
	}{
		{ActivityHike, 8.3, 142, "Hike — 8.3km, 142min"},
		{ActivityWalk, 1.2, 15, "Walk — 1.2km, 15min"},
		{ActivityCycle, 15.0, 45, "Cycle — 15.0km, 45min"},
		{ActivityDrive, 30.5, 35, "Drive — 30.5km, 35min"},
		{ActivityTransit, 12.0, 25, "Transit — 12.0km, 25min"},
		{ActivityRun, 5.0, 28, "Run — 5.0km, 28min"},
	}

	for _, tt := range tests {
		activity := TakeoutActivity{
			Type:        tt.actType,
			DistanceKm:  tt.distance,
			DurationMin: tt.duration,
			StartTime:   time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
			EndTime:     time.Date(2026, 3, 15, 11, 0, 0, 0, time.UTC),
		}
		got := buildTitle(activity)
		if got != tt.want {
			t.Errorf("buildTitle(%s) = %q, want %q", tt.actType, got, tt.want)
		}
	}
}

func TestNormalizeAllActivityTypes(t *testing.T) {
	types := []struct {
		actType     ActivityType
		wantContent string
	}{
		{ActivityHike, "activity/hike"},
		{ActivityWalk, "activity/walk"},
		{ActivityCycle, "activity/cycle"},
		{ActivityDrive, "activity/drive"},
		{ActivityTransit, "activity/transit"},
		{ActivityRun, "activity/run"},
	}

	cfg := MapsConfig{DefaultTier: "standard"}
	for _, tt := range types {
		activity := TakeoutActivity{
			Type:        tt.actType,
			StartTime:   time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
			EndTime:     time.Date(2026, 3, 15, 11, 0, 0, 0, time.UTC),
			DistanceKm:  5.0,
			DurationMin: 30,
			Route:       []LatLng{{Lat: 47.5, Lng: 8.7}, {Lat: 47.52, Lng: 8.75}},
		}
		artifact := NormalizeActivity(activity, "test.json", cfg)
		if artifact.ContentType != tt.wantContent {
			t.Errorf("NormalizeActivity(%s).ContentType = %q, want %q", tt.actType, artifact.ContentType, tt.wantContent)
		}
	}
}

func TestAssignTierTrailFull(t *testing.T) {
	tests := []struct {
		name     string
		activity TakeoutActivity
		want     string
	}{
		{
			name: "hike_trail_qualified",
			activity: TakeoutActivity{
				Type: ActivityHike, DistanceKm: 8.3,
			},
			want: "full",
		},
		{
			name: "walk_trail_qualified",
			activity: TakeoutActivity{
				Type: ActivityWalk, DistanceKm: 3.0,
			},
			want: "full",
		},
		{
			name: "run_trail_qualified",
			activity: TakeoutActivity{
				Type: ActivityRun, DistanceKm: 5.0,
			},
			want: "full",
		},
		{
			name: "cycle_trail_qualified",
			activity: TakeoutActivity{
				Type: ActivityCycle, DistanceKm: 10.0,
			},
			want: "full",
		},
		{
			name: "short_walk_standard",
			activity: TakeoutActivity{
				Type: ActivityWalk, DistanceKm: 1.0,
			},
			want: "standard",
		},
		{
			name: "drive_standard",
			activity: TakeoutActivity{
				Type: ActivityDrive, DistanceKm: 50.0,
			},
			want: "standard",
		},
		{
			name: "transit_standard",
			activity: TakeoutActivity{
				Type: ActivityTransit, DistanceKm: 20.0,
			},
			want: "standard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assignTier(tt.activity)
			if got != tt.want {
				t.Errorf("assignTier() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestComputeDedupHash(t *testing.T) {
	a1 := TakeoutActivity{
		StartTime: time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
		Route:     []LatLng{{Lat: 47.500, Lng: 8.700}, {Lat: 47.520, Lng: 8.750}},
	}
	a2 := TakeoutActivity{
		StartTime: time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
		Route:     []LatLng{{Lat: 47.500, Lng: 8.700}, {Lat: 47.520, Lng: 8.750}},
	}
	if computeDedupHash(a1) != computeDedupHash(a2) {
		t.Error("identical activities should produce the same dedup hash")
	}

	// Different end location
	a3 := TakeoutActivity{
		StartTime: time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
		Route:     []LatLng{{Lat: 47.500, Lng: 8.700}, {Lat: 47.550, Lng: 8.800}},
	}
	if computeDedupHash(a1) == computeDedupHash(a3) {
		t.Error("activities with different end locations should have different hashes")
	}

	// Different date
	a4 := TakeoutActivity{
		StartTime: time.Date(2026, 3, 16, 10, 0, 0, 0, time.UTC),
		Route:     []LatLng{{Lat: 47.500, Lng: 8.700}, {Lat: 47.520, Lng: 8.750}},
	}
	if computeDedupHash(a1) == computeDedupHash(a4) {
		t.Error("activities on different dates should have different hashes")
	}
}

func TestBuildContent(t *testing.T) {
	activity := TakeoutActivity{
		Type:        ActivityHike,
		StartTime:   time.Date(2026, 3, 15, 13, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 3, 15, 15, 22, 0, 0, time.UTC),
		DistanceKm:  8.3,
		DurationMin: 142,
		Route: []LatLng{
			{Lat: 47.500, Lng: 8.700},
			{Lat: 47.520, Lng: 8.750},
		},
	}

	content := buildContent(activity)
	if content == "" {
		t.Fatal("buildContent returned empty string")
	}
	// Should contain key info
	if !containsAll(content, "Hike", "2026-03-15", "8.3km", "142 minutes", "2 waypoints") {
		t.Errorf("content missing expected fragments: %s", content)
	}
}

func TestNormalizeActivityNoRoute(t *testing.T) {
	activity := TakeoutActivity{
		Type:        ActivityDrive,
		StartTime:   time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC),
		DistanceKm:  20.0,
		DurationMin: 30,
		Route:       nil,
	}

	cfg := MapsConfig{DefaultTier: "standard"}
	artifact := NormalizeActivity(activity, "test.json", cfg)

	if artifact.Metadata["start_lat"] != 0.0 {
		t.Errorf("expected start_lat 0 for no-route, got %v", artifact.Metadata["start_lat"])
	}
	if artifact.Metadata["waypoint_count"] != 0 {
		t.Errorf("expected waypoint_count 0 for no-route, got %v", artifact.Metadata["waypoint_count"])
	}
}

func TestRoundToGrid(t *testing.T) {
	// 47.500 → floor(47.500 * 200) / 200 = floor(9500.0) / 200 = 47.500
	if got := roundToGrid(47.500); got != 47.500 {
		t.Errorf("roundToGrid(47.500) = %v, want 47.500", got)
	}
	// 47.503 → floor(47.503 * 200) / 200 = floor(9500.6) / 200 = 9500 / 200 = 47.500
	if got := roundToGrid(47.503); got != 47.500 {
		t.Errorf("roundToGrid(47.503) = %v, want 47.500", got)
	}
}

// containsAll checks if s contains all fragments.
func containsAll(s string, fragments ...string) bool {
	for _, f := range fragments {
		found := false
		for i := 0; i <= len(s)-len(f); i++ {
			if s[i:i+len(f)] == f {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
