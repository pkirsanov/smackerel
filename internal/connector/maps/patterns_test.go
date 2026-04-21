package maps

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestDetectCommuteAboveThreshold(t *testing.T) {
	// 4 weekday same-route trips + 1 weekend trip. With weekdays_only=true, expect 1 pattern (4 trips).
	clusters := []LocationCluster{
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-23", 1, 8, 15.0, 25.0),  // Monday
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-24", 2, 8, 14.0, 24.0),  // Tuesday
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-25", 3, 9, 16.0, 26.0),  // Wednesday
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-26", 4, 8, 15.0, 25.0),  // Thursday
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-28", 6, 10, 20.0, 28.0), // Saturday
	}

	config := MapsConfig{
		CommuteMinOccurrences: 3,
		CommuteWeekdaysOnly:   true,
		CommuteWindowDays:     14,
	}

	patterns := classifyCommutes(clusters, config)

	if len(patterns) != 1 {
		t.Fatalf("expected 1 commute pattern, got %d", len(patterns))
	}
	if patterns[0].Frequency != 4 {
		t.Errorf("expected frequency 4 (weekdays only), got %d", patterns[0].Frequency)
	}
	if patterns[0].StartLat != 47.37 {
		t.Errorf("start_lat = %v, want 47.37", patterns[0].StartLat)
	}
	if patterns[0].EndLat != 47.40 {
		t.Errorf("end_lat = %v, want 47.40", patterns[0].EndLat)
	}
}

func TestDetectCommuteBelowThreshold(t *testing.T) {
	// Only 2 trips between same clusters → below min_occurrences=3.
	clusters := []LocationCluster{
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-23", 1, 8, 15.0, 25.0),
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-24", 2, 8, 14.0, 24.0),
	}

	config := MapsConfig{
		CommuteMinOccurrences: 3,
		CommuteWeekdaysOnly:   true,
		CommuteWindowDays:     14,
	}

	patterns := classifyCommutes(clusters, config)

	if len(patterns) != 0 {
		t.Errorf("expected 0 commute patterns below threshold, got %d", len(patterns))
	}
}

func TestCommuteWeekdaysOnlyFilter(t *testing.T) {
	// 3 Saturday trips + 2 weekday trips. weekdays_only=true → only 2 weekday count → below threshold.
	clusters := []LocationCluster{
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-21", 6, 10, 15.0, 25.0), // Saturday
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-22", 0, 10, 15.0, 25.0), // Sunday
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-28", 6, 10, 15.0, 25.0), // Saturday
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-23", 1, 8, 15.0, 25.0),  // Monday
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-24", 2, 8, 14.0, 24.0),  // Tuesday
	}

	config := MapsConfig{
		CommuteMinOccurrences: 3,
		CommuteWeekdaysOnly:   true,
		CommuteWindowDays:     14,
	}

	patterns := classifyCommutes(clusters, config)

	if len(patterns) != 0 {
		t.Errorf("expected 0 patterns (only 2 weekday trips), got %d", len(patterns))
	}

	// With weekdays_only=false, all 5 should count.
	config.CommuteWeekdaysOnly = false
	patterns = classifyCommutes(clusters, config)

	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern with weekdays_only=false, got %d", len(patterns))
	}
	if patterns[0].Frequency != 5 {
		t.Errorf("expected frequency 5, got %d", patterns[0].Frequency)
	}
}

func TestNormalizeCommutePattern(t *testing.T) {
	p := CommutePattern{
		StartClusterID:       "47.370,8.540",
		EndClusterID:         "47.400,8.550",
		StartLat:             47.370,
		StartLng:             8.540,
		EndLat:               47.400,
		EndLng:               8.550,
		Frequency:            4,
		TypicalDepartureHour: 8,
		AvgDurationMin:       25.0,
		AvgDistanceKm:        15.0,
		LatestActivityDate:   time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC),
	}

	artifact := normalizeCommutePattern(p)

	if artifact.ContentType != "pattern/commute" {
		t.Errorf("ContentType = %q, want %q", artifact.ContentType, "pattern/commute")
	}
	if artifact.SourceID != "google-maps-timeline" {
		t.Errorf("SourceID = %q, want %q", artifact.SourceID, "google-maps-timeline")
	}
	if artifact.Metadata["frequency"] != 4 {
		t.Errorf("frequency = %v, want 4", artifact.Metadata["frequency"])
	}
	if artifact.Metadata["typical_departure_hour"] != 8 {
		t.Errorf("typical_departure_hour = %v, want 8", artifact.Metadata["typical_departure_hour"])
	}
	if artifact.Metadata["avg_duration_min"] != 25.0 {
		t.Errorf("avg_duration_min = %v, want 25.0", artifact.Metadata["avg_duration_min"])
	}
	if artifact.Metadata["avg_distance_km"] != 15.0 {
		t.Errorf("avg_distance_km = %v, want 15.0", artifact.Metadata["avg_distance_km"])
	}
	if artifact.Metadata["start_lat"] != 47.370 {
		t.Errorf("start_lat = %v, want 47.370", artifact.Metadata["start_lat"])
	}
	if artifact.Metadata["processing_tier"] != "light" {
		t.Errorf("processing_tier = %v, want %q", artifact.Metadata["processing_tier"], "light")
	}

	// SourceRef should be deterministic.
	artifact2 := normalizeCommutePattern(p)
	if artifact.SourceRef != artifact2.SourceRef {
		t.Errorf("SourceRef not deterministic: %q vs %q", artifact.SourceRef, artifact2.SourceRef)
	}

	// Title format check.
	expectedTitle := "Commute: 47.370,8.540→47.400,8.550 (4 trips)"
	if artifact.Title != expectedTitle {
		t.Errorf("Title = %q, want %q", artifact.Title, expectedTitle)
	}
}

// IMPROVE-011: Commute CapturedAt uses deterministic LatestActivityDate, not time.Now()
func TestNormalizeCommutePatternDeterministicCapturedAt(t *testing.T) {
	latestDate := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	p := CommutePattern{
		StartClusterID:       "47.370,8.540",
		EndClusterID:         "47.400,8.550",
		StartLat:             47.370,
		StartLng:             8.540,
		EndLat:               47.400,
		EndLng:               8.550,
		Frequency:            4,
		TypicalDepartureHour: 8,
		AvgDurationMin:       25.0,
		AvgDistanceKm:        15.0,
		LatestActivityDate:   latestDate,
	}

	artifact1 := normalizeCommutePattern(p)
	artifact2 := normalizeCommutePattern(p)

	if !artifact1.CapturedAt.Equal(latestDate) {
		t.Errorf("CapturedAt = %v, want %v (deterministic from LatestActivityDate)", artifact1.CapturedAt, latestDate)
	}
	if !artifact1.CapturedAt.Equal(artifact2.CapturedAt) {
		t.Errorf("CapturedAt not deterministic across calls: %v vs %v", artifact1.CapturedAt, artifact2.CapturedAt)
	}
}

// IMPROVE-011: classifyCommutes propagates LatestActivityDate correctly
func TestClassifyCommutesTracksLatestDate(t *testing.T) {
	clusters := []LocationCluster{
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-23", 1, 8, 15.0, 25.0),
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-24", 2, 8, 14.0, 24.0),
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-26", 4, 8, 16.0, 26.0),
	}

	config := MapsConfig{
		CommuteMinOccurrences: 3,
		CommuteWeekdaysOnly:   false,
		CommuteWindowDays:     14,
	}

	patterns := classifyCommutes(clusters, config)
	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}

	expectedDate := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	if !patterns[0].LatestActivityDate.Equal(expectedDate) {
		t.Errorf("LatestActivityDate = %v, want %v", patterns[0].LatestActivityDate, expectedDate)
	}
}

func TestDetectTripOvernight(t *testing.T) {
	// Home in Zurich (47.37, 8.54). 3-day cluster in Berlin (52.52, 13.40) ~660km away.
	home := LatLng{Lat: 47.37, Lng: 8.54}

	clusters := []LocationCluster{
		// Day 1: drive from Zurich, walk in Berlin, transit in Berlin
		makeCluster(47.37, 8.54, 52.52, 13.40, "drive", "2026-04-10", 4, 7, 660.0, 420.0),
		makeCluster(52.52, 13.40, 52.53, 13.41, "walk", "2026-04-10", 4, 14, 3.0, 45.0),
		makeCluster(52.52, 13.40, 52.54, 13.42, "transit", "2026-04-10", 4, 16, 5.0, 15.0),
		// Day 2: hike near Berlin, walk in Berlin
		makeCluster(52.52, 13.40, 52.60, 13.50, "hike", "2026-04-11", 5, 9, 12.0, 180.0),
		makeCluster(52.52, 13.40, 52.53, 13.41, "walk", "2026-04-11", 5, 16, 2.0, 30.0),
		// Day 3: drive back to Zurich
		makeCluster(52.52, 13.40, 47.37, 8.54, "drive", "2026-04-12", 6, 8, 660.0, 420.0),
	}

	config := MapsConfig{
		TripMinDistanceKm:     50,
		TripMinOvernightHours: 18,
	}

	trips := classifyTrips(clusters, home, config)

	if len(trips) != 1 {
		t.Fatalf("expected 1 trip, got %d", len(trips))
	}

	trip := trips[0]
	if trip.StartDate.Format("2006-01-02") != "2026-04-10" {
		t.Errorf("start_date = %v, want 2026-04-10", trip.StartDate.Format("2006-01-02"))
	}
	if trip.EndDate.Format("2006-01-02") != "2026-04-12" {
		t.Errorf("end_date = %v, want 2026-04-12", trip.EndDate.Format("2006-01-02"))
	}
	if trip.DistanceFromHome < 600 {
		t.Errorf("distance_from_home = %.0f, expected >600km", trip.DistanceFromHome)
	}
	if trip.TotalActivities != 6 {
		t.Errorf("total_activities = %d, want 6", trip.TotalActivities)
	}

	// Check activity breakdown.
	if trip.ActivityBreakdown["drive"] != 2 {
		t.Errorf("drive count = %d, want 2", trip.ActivityBreakdown["drive"])
	}
	if trip.ActivityBreakdown["walk"] != 2 {
		t.Errorf("walk count = %d, want 2", trip.ActivityBreakdown["walk"])
	}
	if trip.ActivityBreakdown["hike"] != 1 {
		t.Errorf("hike count = %d, want 1", trip.ActivityBreakdown["hike"])
	}
	if trip.ActivityBreakdown["transit"] != 1 {
		t.Errorf("transit count = %d, want 1", trip.ActivityBreakdown["transit"])
	}
}

func TestDetectTripBelowDistance(t *testing.T) {
	// Cluster 30km from home → below 50km threshold → no trips.
	home := LatLng{Lat: 47.37, Lng: 8.54}

	// Winterthur is ~25km from Zurich.
	clusters := []LocationCluster{
		makeCluster(47.37, 8.54, 47.50, 8.72, "drive", "2026-04-10", 4, 8, 25.0, 30.0),
		makeCluster(47.50, 8.72, 47.51, 8.73, "walk", "2026-04-10", 4, 14, 3.0, 45.0),
		makeCluster(47.50, 8.72, 47.51, 8.73, "walk", "2026-04-11", 5, 10, 2.0, 30.0),
		makeCluster(47.50, 8.72, 47.37, 8.54, "drive", "2026-04-12", 6, 8, 25.0, 30.0),
	}

	config := MapsConfig{
		TripMinDistanceKm:     50,
		TripMinOvernightHours: 18,
	}

	trips := classifyTrips(clusters, home, config)

	if len(trips) != 0 {
		t.Errorf("expected 0 trips below distance threshold, got %d", len(trips))
	}
}

func TestNormalizeTripEvent(t *testing.T) {
	trip := TripEvent{
		DestinationLat:   52.52,
		DestinationLng:   13.40,
		StartDate:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		EndDate:          time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC),
		DistanceFromHome: 660.0,
		ActivityBreakdown: map[string]int{
			"drive":   2,
			"walk":    2,
			"hike":    1,
			"transit": 1,
		},
		TotalActivities: 6,
	}

	artifact := normalizeTripEvent(trip)

	if artifact.ContentType != "event/trip" {
		t.Errorf("ContentType = %q, want %q", artifact.ContentType, "event/trip")
	}
	if artifact.SourceID != "google-maps-timeline" {
		t.Errorf("SourceID = %q, want %q", artifact.SourceID, "google-maps-timeline")
	}

	expectedTitle := "Trip to (52.52,13.40) — 2026-04-10–2026-04-12"
	if artifact.Title != expectedTitle {
		t.Errorf("Title = %q, want %q", artifact.Title, expectedTitle)
	}

	if artifact.Metadata["destination_lat"] != 52.52 {
		t.Errorf("destination_lat = %v, want 52.52", artifact.Metadata["destination_lat"])
	}
	if artifact.Metadata["start_date"] != "2026-04-10" {
		t.Errorf("start_date = %v, want 2026-04-10", artifact.Metadata["start_date"])
	}
	if artifact.Metadata["end_date"] != "2026-04-12" {
		t.Errorf("end_date = %v, want 2026-04-12", artifact.Metadata["end_date"])
	}
	if artifact.Metadata["total_activities"] != 6 {
		t.Errorf("total_activities = %v, want 6", artifact.Metadata["total_activities"])
	}
	if artifact.Metadata["processing_tier"] != "full" {
		t.Errorf("processing_tier = %v, want %q", artifact.Metadata["processing_tier"], "full")
	}

	breakdown, ok := artifact.Metadata["activity_breakdown"].(map[string]int)
	if !ok {
		t.Fatal("activity_breakdown missing or wrong type")
	}
	if breakdown["drive"] != 2 {
		t.Errorf("breakdown drive = %d, want 2", breakdown["drive"])
	}

	// SourceRef should be deterministic.
	artifact2 := normalizeTripEvent(trip)
	if artifact.SourceRef != artifact2.SourceRef {
		t.Errorf("SourceRef not deterministic: %q vs %q", artifact.SourceRef, artifact2.SourceRef)
	}
}

func TestRoundToGridPatterns(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{47.500, 47.500},
		{47.503, 47.500},
		{47.506, 47.505},
		{8.704, 8.700},
		// Negative coordinates (southern/western hemisphere).
		{-33.867, -33.870},
		{151.209, 151.205},
	}

	for _, tt := range tests {
		got := roundToGrid(tt.input)
		if got != tt.want {
			t.Errorf("roundToGrid(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestPostSyncContinuesOnFailure(t *testing.T) {
	// PostSync with nil pool should return nil immediately.
	c := New("google-maps-timeline")
	c.config = MapsConfig{
		CommuteMinOccurrences: 3,
		CommuteWindowDays:     14,
		CommuteWeekdaysOnly:   true,
		TripMinDistanceKm:     50,
		TripMinOvernightHours: 18,
		LinkTimeExtendMin:     30,
		LinkProximityRadiusM:  1000,
	}

	activities := []TakeoutActivity{
		{
			Type:        ActivityDrive,
			StartTime:   time.Date(2026, 3, 23, 8, 0, 0, 0, time.UTC),
			EndTime:     time.Date(2026, 3, 23, 8, 30, 0, 0, time.UTC),
			DistanceKm:  15.0,
			DurationMin: 30,
			Route:       []LatLng{{Lat: 47.37, Lng: 8.54}, {Lat: 47.40, Lng: 8.55}},
		},
	}

	// With nil pool, PostSync should return nil artifacts and nil error (skip pattern detection gracefully).
	artifacts, err := c.PostSync(context.Background(), activities)
	if err != nil {
		t.Errorf("PostSync with nil pool should return nil error, got: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("PostSync with nil pool should return 0 artifacts, got %d", len(artifacts))
	}
}

// SCN-MT-021: Commute-classified activities produce pattern artifacts with "light" tier.
// The normalizeCommutePattern output artifact carries processing_tier="light"
// while the original activity artifacts remain at their assigned tier.
func TestTierDowngradeCommute(t *testing.T) {
	// Original activity: drive, standard tier.
	activity := TakeoutActivity{
		Type:        ActivityDrive,
		StartTime:   time.Date(2026, 3, 23, 8, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 3, 23, 8, 25, 0, 0, time.UTC),
		DistanceKm:  15.0,
		DurationMin: 25,
		Route:       []LatLng{{Lat: 47.37, Lng: 8.54}, {Lat: 47.40, Lng: 8.55}},
	}
	original := NormalizeActivity(activity, "commute-data.json")
	if original.Metadata["processing_tier"] != "standard" {
		t.Fatalf("precondition: drive activity tier = %v, want standard", original.Metadata["processing_tier"])
	}

	// Commute pattern artifact: processing_tier must be "light".
	pattern := CommutePattern{
		StartClusterID:       "47.370,8.540",
		EndClusterID:         "47.400,8.550",
		StartLat:             47.370,
		StartLng:             8.540,
		EndLat:               47.400,
		EndLng:               8.550,
		Frequency:            4,
		TypicalDepartureHour: 8,
		AvgDurationMin:       25.0,
		AvgDistanceKm:        15.0,
		LatestActivityDate:   time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC),
	}
	commuteArtifact := normalizeCommutePattern(pattern)
	if commuteArtifact.Metadata["processing_tier"] != "light" {
		t.Errorf("commute pattern artifact tier = %v, want light", commuteArtifact.Metadata["processing_tier"])
	}

	// Original activity artifact is NOT mutated by pattern detection.
	if original.Metadata["processing_tier"] != "standard" {
		t.Errorf("original activity tier changed to %v, want standard (unchanged)", original.Metadata["processing_tier"])
	}
}

// SCN-MT-021: Trip-associated activities produce trip artifacts with "full" tier.
func TestTierUpgradeTrip(t *testing.T) {
	// Original activity: transit, standard tier.
	activity := TakeoutActivity{
		Type:        ActivityTransit,
		StartTime:   time.Date(2026, 4, 10, 16, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 4, 10, 16, 15, 0, 0, time.UTC),
		DistanceKm:  5.0,
		DurationMin: 15,
		Route:       []LatLng{{Lat: 52.52, Lng: 13.40}, {Lat: 52.54, Lng: 13.42}},
	}
	original := NormalizeActivity(activity, "trip-data.json")
	if original.Metadata["processing_tier"] != "standard" {
		t.Fatalf("precondition: transit activity tier = %v, want standard", original.Metadata["processing_tier"])
	}

	// Trip event artifact: processing_tier must be "full".
	trip := TripEvent{
		DestinationLat:   52.52,
		DestinationLng:   13.40,
		StartDate:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		EndDate:          time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC),
		DistanceFromHome: 660.0,
		ActivityBreakdown: map[string]int{
			"drive":   2,
			"walk":    2,
			"hike":    1,
			"transit": 1,
		},
		TotalActivities: 6,
	}
	tripArtifact := normalizeTripEvent(trip)
	if tripArtifact.Metadata["processing_tier"] != "full" {
		t.Errorf("trip event artifact tier = %v, want full", tripArtifact.Metadata["processing_tier"])
	}

	// Original activity artifact is NOT mutated.
	if original.Metadata["processing_tier"] != "standard" {
		t.Errorf("original activity tier changed to %v, want standard (unchanged)", original.Metadata["processing_tier"])
	}
}

func TestDetermineLinkTypeSpatial(t *testing.T) {
	activity := TakeoutActivity{
		StartTime: time.Date(2026, 3, 15, 13, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 3, 15, 15, 22, 0, 0, time.UTC),
		Route: []LatLng{
			{Lat: 47.500, Lng: 8.700},
			{Lat: 47.510, Lng: 8.720},
			{Lat: 47.520, Lng: 8.740},
		},
	}

	// Artifact near the route: temporal-spatial.
	linkType := determineLinkType(activity, 47.501, 8.702, 1.0) // ~1km proximity
	if linkType != "temporal-spatial" {
		t.Errorf("expected temporal-spatial for nearby artifact, got %q", linkType)
	}

	// Artifact far from route: temporal-only.
	linkType = determineLinkType(activity, 48.000, 9.000, 1.0) // ~50km away
	if linkType != "temporal-only" {
		t.Errorf("expected temporal-only for distant artifact, got %q", linkType)
	}

	// Artifact with no location (0,0): temporal-only.
	linkType = determineLinkType(activity, 0, 0, 1.0)
	if linkType != "temporal-only" {
		t.Errorf("expected temporal-only for no-location artifact, got %q", linkType)
	}
}

func TestTypicalHour(t *testing.T) {
	tests := []struct {
		name  string
		hours []int
		want  int
	}{
		{"clear_winner", []int{8, 8, 9, 8, 7}, 8},
		{"tie_picks_lower", []int{7, 7, 8, 8}, 7}, // IMPROVE-011: deterministic tie → lower hour
		{"tie_picks_lower_reversed", []int{8, 8, 7, 7}, 7},
		{"empty", []int{}, 0},
		{"single", []int{14}, 14},
		{"three_way_tie", []int{6, 7, 8}, 6}, // IMPROVE-011: all equal freq → lowest hour
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := typicalHour(tt.hours)
			if got != tt.want {
				t.Errorf("typicalHour(%v) = %d, want %d", tt.hours, got, tt.want)
			}
		})
	}
}

func TestSameDate(t *testing.T) {
	a := time.Date(2026, 3, 15, 8, 0, 0, 0, time.UTC)
	b := time.Date(2026, 3, 15, 20, 0, 0, 0, time.UTC)
	c := time.Date(2026, 3, 16, 8, 0, 0, 0, time.UTC)

	if !sameDate(a, b) {
		t.Error("same day should be true")
	}
	if sameDate(a, c) {
		t.Error("different day should be false")
	}
}

func TestDaysDiff(t *testing.T) {
	a := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	b := time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC)

	if got := daysDiff(a, b); got != 3 {
		t.Errorf("daysDiff = %d, want 3", got)
	}
	if got := daysDiff(b, a); got != 3 {
		t.Errorf("daysDiff reversed = %d, want 3", got)
	}
}

func TestClassifyTripsEmptyClusters(t *testing.T) {
	home := LatLng{Lat: 47.37, Lng: 8.54}
	config := MapsConfig{TripMinDistanceKm: 50, TripMinOvernightHours: 18}

	trips := classifyTrips(nil, home, config)
	if len(trips) != 0 {
		t.Errorf("expected 0 trips for empty clusters, got %d", len(trips))
	}
}

func TestClassifyTripsSingleDayBelowOvernight(t *testing.T) {
	// REG-011-R70-001: Single remote day → span = 0h < TripMinOvernightHours → no trip.
	// A same-day round-trip has zero overnight hours and must NOT be classified as a trip.
	home := LatLng{Lat: 47.37, Lng: 8.54}
	clusters := []LocationCluster{
		makeCluster(47.37, 8.54, 52.52, 13.40, "drive", "2026-04-10", 4, 8, 660.0, 420.0),
	}
	config := MapsConfig{TripMinDistanceKm: 50, TripMinOvernightHours: 18}

	trips := classifyTrips(clusters, home, config)
	if len(trips) != 0 {
		t.Errorf("expected 0 trips for single-day remote activity (no overnight), got %d", len(trips))
	}
}

func TestClassifyTripsGapBreaks(t *testing.T) {
	// REG-011-R70-001: Two remote days with a 3-day gap → two separate single-day entries.
	// Each single-day entry has span 0h < 18h, so neither qualifies as a trip.
	home := LatLng{Lat: 47.37, Lng: 8.54}
	clusters := []LocationCluster{
		makeCluster(47.37, 8.54, 52.52, 13.40, "drive", "2026-04-10", 4, 8, 660.0, 420.0),
		// Gap: Apr 11, 12, 13 back home
		makeCluster(47.37, 8.54, 52.52, 13.40, "drive", "2026-04-14", 1, 8, 660.0, 420.0),
	}
	config := MapsConfig{TripMinDistanceKm: 50, TripMinOvernightHours: 18}

	trips := classifyTrips(clusters, home, config)
	// Gap >1 day breaks the consecutive run → two separate single-day entries,
	// each with 0h span < 18h threshold → 0 trips.
	if len(trips) != 0 {
		t.Errorf("expected 0 trips (single-day entries have no overnight), got %d", len(trips))
	}
}

// REG-011-R70-001: Adversarial regression for the trip overnight span bug.
// The old code added +24h to the span, making same-day remote activity
// clusters (span 0h + 24h = 24h) satisfy the 18h overnight threshold.
// Fixed: span is endDay - startDay with no +24h offset. Same-day = 0h < 18h.
// If the bug were reintroduced, this test would fail because the single-day
// remote cluster would produce a trip artifact.
func TestRegressionSingleDayRemoteIsNotOvernightTrip(t *testing.T) {
	home := LatLng{Lat: 47.37, Lng: 8.54}

	// Single day: 3 activities in Berlin (660km from Zurich), all on April 10.
	clusters := []LocationCluster{
		makeCluster(47.37, 8.54, 52.52, 13.40, "drive", "2026-04-10", 4, 7, 660.0, 420.0),
		makeCluster(52.52, 13.40, 52.53, 13.41, "walk", "2026-04-10", 4, 14, 3.0, 45.0),
		makeCluster(52.52, 13.40, 47.37, 8.54, "drive", "2026-04-10", 4, 18, 660.0, 420.0),
	}

	config := MapsConfig{TripMinDistanceKm: 50, TripMinOvernightHours: 18}

	trips := classifyTrips(clusters, home, config)
	if len(trips) != 0 {
		t.Errorf("single-day remote activity cluster (no overnight) must NOT be classified as a trip, got %d trips", len(trips))
	}

	// Verify that a genuine 2-day trip IS still detected (no false negatives).
	twoDayClusters := []LocationCluster{
		makeCluster(47.37, 8.54, 52.52, 13.40, "drive", "2026-04-10", 4, 7, 660.0, 420.0),
		makeCluster(52.52, 13.40, 52.53, 13.41, "walk", "2026-04-10", 4, 14, 3.0, 45.0),
		makeCluster(52.52, 13.40, 52.53, 13.41, "walk", "2026-04-11", 5, 10, 2.0, 30.0),
		makeCluster(52.52, 13.40, 47.37, 8.54, "drive", "2026-04-11", 5, 16, 660.0, 420.0),
	}

	trips = classifyTrips(twoDayClusters, home, config)
	if len(trips) != 1 {
		t.Fatalf("2-day overnight trip must still be detected, got %d trips", len(trips))
	}
	if trips[0].StartDate.Format("2006-01-02") != "2026-04-10" {
		t.Errorf("trip start = %s, want 2026-04-10", trips[0].StartDate.Format("2006-01-02"))
	}
	if trips[0].EndDate.Format("2006-01-02") != "2026-04-11" {
		t.Errorf("trip end = %s, want 2026-04-11", trips[0].EndDate.Format("2006-01-02"))
	}
}

func TestClassifyCommutesMultipleRoutes(t *testing.T) {
	// Two distinct routes, both above threshold → 2 commute patterns
	clusters := []LocationCluster{
		// Route A: home→office (4 trips)
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-23", 1, 8, 15.0, 25.0),
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-24", 2, 8, 15.0, 25.0),
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-25", 3, 8, 15.0, 25.0),
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-26", 4, 8, 15.0, 25.0),
		// Route B: home→gym (3 trips)
		makeCluster(47.37, 8.54, 47.45, 8.60, "drive", "2026-03-23", 1, 18, 8.0, 15.0),
		makeCluster(47.37, 8.54, 47.45, 8.60, "drive", "2026-03-24", 2, 18, 8.0, 15.0),
		makeCluster(47.37, 8.54, 47.45, 8.60, "drive", "2026-03-25", 3, 18, 8.0, 15.0),
	}

	config := MapsConfig{
		CommuteMinOccurrences: 3,
		CommuteWeekdaysOnly:   false,
		CommuteWindowDays:     14,
	}

	patterns := classifyCommutes(clusters, config)
	if len(patterns) != 2 {
		t.Fatalf("expected 2 commute patterns, got %d", len(patterns))
	}
	// Sorted by frequency desc → Route A (4) first, Route B (3) second
	if patterns[0].Frequency != 4 {
		t.Errorf("first pattern frequency = %d, want 4", patterns[0].Frequency)
	}
	if patterns[1].Frequency != 3 {
		t.Errorf("second pattern frequency = %d, want 3", patterns[1].Frequency)
	}
}

func TestClassifyCommutesEmptyClusters(t *testing.T) {
	config := MapsConfig{CommuteMinOccurrences: 3, CommuteWeekdaysOnly: true}
	patterns := classifyCommutes(nil, config)
	if len(patterns) != 0 {
		t.Errorf("expected 0 patterns for nil clusters, got %d", len(patterns))
	}
}

func TestDetermineLinkTypeEmptyRoute(t *testing.T) {
	// Activity with no route and no start/end location → temporal-only
	activity := TakeoutActivity{
		StartTime: time.Date(2026, 3, 15, 13, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 3, 15, 15, 0, 0, 0, time.UTC),
		Route:     nil,
	}

	linkType := determineLinkType(activity, 47.500, 8.700, 1.0)
	if linkType != "temporal-only" {
		t.Errorf("expected temporal-only for empty route with no locations, got %q", linkType)
	}
}

// --- IMP-011-R14-002: determineLinkType uses StartLocation/EndLocation fallback ---

func TestImprove_DetermineLinkTypeStartLocationFallback(t *testing.T) {
	// Activity with no route but StartLocation near the artifact → temporal-spatial.
	activity := TakeoutActivity{
		StartTime:     time.Date(2026, 3, 15, 13, 0, 0, 0, time.UTC),
		EndTime:       time.Date(2026, 3, 15, 15, 0, 0, 0, time.UTC),
		Route:         nil,
		StartLocation: LatLng{Lat: 47.500, Lng: 8.700},
		EndLocation:   LatLng{Lat: 47.600, Lng: 8.800},
	}

	linkType := determineLinkType(activity, 47.501, 8.701, 1.0) // ~0.1km from start
	if linkType != "temporal-spatial" {
		t.Errorf("expected temporal-spatial for artifact near StartLocation, got %q", linkType)
	}
}

func TestImprove_DetermineLinkTypeEndLocationFallback(t *testing.T) {
	// Activity with no route but EndLocation near the artifact → temporal-spatial.
	activity := TakeoutActivity{
		StartTime:     time.Date(2026, 3, 15, 13, 0, 0, 0, time.UTC),
		EndTime:       time.Date(2026, 3, 15, 15, 0, 0, 0, time.UTC),
		Route:         nil,
		StartLocation: LatLng{Lat: 40.000, Lng: 5.000}, // far from artifact
		EndLocation:   LatLng{Lat: 47.500, Lng: 8.700}, // near artifact
	}

	linkType := determineLinkType(activity, 47.501, 8.701, 1.0)
	if linkType != "temporal-spatial" {
		t.Errorf("expected temporal-spatial for artifact near EndLocation, got %q", linkType)
	}
}

func TestImprove_DetermineLinkTypeLocationsFarAway(t *testing.T) {
	// Activity with no route, StartLocation and EndLocation both far → temporal-only.
	activity := TakeoutActivity{
		StartTime:     time.Date(2026, 3, 15, 13, 0, 0, 0, time.UTC),
		EndTime:       time.Date(2026, 3, 15, 15, 0, 0, 0, time.UTC),
		Route:         nil,
		StartLocation: LatLng{Lat: 40.000, Lng: 5.000},
		EndLocation:   LatLng{Lat: 41.000, Lng: 6.000},
	}

	linkType := determineLinkType(activity, 47.500, 8.700, 1.0) // far from both
	if linkType != "temporal-only" {
		t.Errorf("expected temporal-only for artifact far from both locations, got %q", linkType)
	}
}

func TestRouteKey(t *testing.T) {
	key := routeKey(47.370, 8.540, 47.400, 8.550)
	expected := "47.370,8.540→47.400,8.550"
	if key != expected {
		t.Errorf("routeKey = %q, want %q", key, expected)
	}
	// Different coordinates → different key
	key2 := routeKey(47.370, 8.540, 47.500, 8.600)
	if key == key2 {
		t.Error("different routes should produce different keys")
	}
}

func TestCommuteSourceRefDeterministic(t *testing.T) {
	p := CommutePattern{StartLat: 47.37, StartLng: 8.54, EndLat: 47.40, EndLng: 8.55}
	ref1 := commuteSourceRef(p)
	ref2 := commuteSourceRef(p)
	if ref1 != ref2 {
		t.Errorf("commuteSourceRef not deterministic: %q vs %q", ref1, ref2)
	}
	// Different pattern → different ref
	p2 := CommutePattern{StartLat: 47.37, StartLng: 8.54, EndLat: 47.50, EndLng: 8.60}
	if commuteSourceRef(p) == commuteSourceRef(p2) {
		t.Error("different patterns should have different sourceRefs")
	}
}

func TestTripSourceRefDeterministic(t *testing.T) {
	trip := TripEvent{
		DestinationLat: 52.52, DestinationLng: 13.40,
		StartDate: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC),
	}
	ref1 := tripSourceRef(trip)
	ref2 := tripSourceRef(trip)
	if ref1 != ref2 {
		t.Errorf("tripSourceRef not deterministic: %q vs %q", ref1, ref2)
	}
}

// --- Hardening tests ---

// HARDEN-011: Same-day round trip far from home. Documents the +24h span behavior:
// REG-011-R70-001: Same-day round trip must NOT be detected as an overnight trip.
// The old code added +24h to the span, making same-day remote activity
// clusters (span 0h + 24h = 24h) satisfy the 18h overnight threshold.
// Fixed: span = endDay - startDay with no +24h offset. Same-day = 0h < 18h.
func TestClassifyTripsSameDayRoundTrip(t *testing.T) {
	home := LatLng{Lat: 47.37, Lng: 8.54}
	clusters := []LocationCluster{
		// Morning: drive Zurich→Berlin
		makeCluster(47.37, 8.54, 52.52, 13.40, "drive", "2026-04-10", 4, 6, 660.0, 420.0),
		// Afternoon: drive Berlin→Zurich (same day)
		makeCluster(52.52, 13.40, 47.37, 8.54, "drive", "2026-04-10", 4, 16, 660.0, 420.0),
	}
	config := MapsConfig{TripMinDistanceKm: 50, TripMinOvernightHours: 18}

	trips := classifyTrips(clusters, home, config)
	// Same-day round trip: span = 0h < 18h overnight threshold → no trip.
	if len(trips) != 0 {
		t.Errorf("same-day round trip (no overnight) must NOT be a trip, got %d trips", len(trips))
	}
}

// HARDEN-011: Commute detection at exact threshold boundary.
func TestDetectCommuteExactThreshold(t *testing.T) {
	clusters := []LocationCluster{
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-23", 1, 8, 15.0, 25.0),
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-24", 2, 8, 14.0, 24.0),
		makeCluster(47.37, 8.54, 47.40, 8.55, "drive", "2026-03-25", 3, 8, 16.0, 26.0),
	}
	config := MapsConfig{
		CommuteMinOccurrences: 3, // exactly at threshold
		CommuteWeekdaysOnly:   false,
		CommuteWindowDays:     14,
	}

	patterns := classifyCommutes(clusters, config)
	if len(patterns) != 1 {
		t.Fatalf("exactly 3 trips with threshold 3 should detect 1 pattern, got %d", len(patterns))
	}
	if patterns[0].Frequency != 3 {
		t.Errorf("frequency = %d, want 3", patterns[0].Frequency)
	}
}

// HARDEN-011: Trip with TripMinOvernightHours = 0 should accept any remote cluster.
func TestClassifyTripsZeroOvernightThreshold(t *testing.T) {
	home := LatLng{Lat: 47.37, Lng: 8.54}
	clusters := []LocationCluster{
		makeCluster(47.37, 8.54, 52.52, 13.40, "drive", "2026-04-10", 4, 8, 660.0, 420.0),
	}
	config := MapsConfig{TripMinDistanceKm: 50, TripMinOvernightHours: 0}

	trips := classifyTrips(clusters, home, config)
	// Single day: span = 0h + 24h = 24h >= 0h → trip detected
	if len(trips) != 1 {
		t.Errorf("zero overnight threshold should detect trip, got %d", len(trips))
	}
}

// HARDEN-011: determineLinkType with route containing only one point.
func TestDetermineLinkTypeSinglePointRoute(t *testing.T) {
	activity := TakeoutActivity{
		StartTime: time.Date(2026, 3, 15, 13, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 3, 15, 14, 0, 0, 0, time.UTC),
		Route:     []LatLng{{Lat: 47.500, Lng: 8.700}},
	}
	// Artifact very close to the single route point
	linkType := determineLinkType(activity, 47.501, 8.701, 1.0)
	if linkType != "temporal-spatial" {
		t.Errorf("artifact near single-point route should be temporal-spatial, got %q", linkType)
	}

	// Artifact far away
	linkType = determineLinkType(activity, 48.000, 9.000, 1.0)
	if linkType != "temporal-only" {
		t.Errorf("distant artifact should be temporal-only, got %q", linkType)
	}
}

// --- test helpers ---

func makeCluster(
	startLat, startLng, endLat, endLng float64,
	actType string, date string, dayOfWeek, departureHour int,
	distKm, durMin float64,
) LocationCluster {
	d, _ := time.Parse("2006-01-02", date)
	return LocationCluster{
		ID:              fmt.Sprintf("cluster-%s-%.3f-%.3f", date, startLat, endLat),
		SourceRef:       "test-ref",
		StartClusterLat: startLat,
		StartClusterLng: startLng,
		EndClusterLat:   endLat,
		EndClusterLng:   endLng,
		ActivityType:    actType,
		ActivityDate:    d,
		DayOfWeek:       dayOfWeek,
		DepartureHour:   departureHour,
		DistanceKm:      distKm,
		DurationMin:     durMin,
	}
}
