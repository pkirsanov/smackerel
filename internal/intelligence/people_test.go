package intelligence

import (
	"testing"
	"time"
)

func TestTripDossier_Struct(t *testing.T) {
	d := TripDossier{
		Destination:     "Berlin",
		DepartureDate:   time.Now().AddDate(0, 0, 5),
		State:           "upcoming",
		FlightArtifacts: []string{"art-1"},
		HotelArtifacts:  []string{"art-2"},
	}

	if d.Destination != "Berlin" {
		t.Errorf("expected Berlin, got %s", d.Destination)
	}
	if d.State != "upcoming" {
		t.Errorf("expected upcoming, got %s", d.State)
	}
}

func TestExtractDestination(t *testing.T) {
	tests := []struct {
		title    string
		content  string
		expected string
	}{
		{"Flight to Berlin", "", "Berlin"},
		{"", "Destination: Tokyo Japan", "Tokyo Japan"},
		{"Check-in at Marriott", "", "Marriott"},
		{"Random email", "Nothing useful here", ""},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := extractDestination(tt.title, tt.content)
			if got != tt.expected {
				t.Errorf("extractDestination(%q, %q) = %q, want %q", tt.title, tt.content, got, tt.expected)
			}
		})
	}
}

func TestClassifyTripState(t *testing.T) {
	now := time.Now()
	if classifyTripState(now.AddDate(0, 0, 5)) != "upcoming" {
		t.Error("5 days from now should be upcoming")
	}
	if classifyTripState(now.AddDate(0, 0, -3)) != "active" {
		t.Error("3 days ago should be active")
	}
	if classifyTripState(now.AddDate(0, 0, -30)) != "completed" {
		t.Error("30 days ago should be completed")
	}
}

func TestAssembleDossierText(t *testing.T) {
	d := &TripDossier{
		Destination:     "Tokyo",
		State:           "upcoming",
		FlightArtifacts: []string{"f1", "f2"},
		HotelArtifacts:  []string{"h1"},
	}

	text := assembleDossierText(d)
	if text == "" {
		t.Error("expected non-empty dossier text")
	}
	if !contains(text, "Tokyo") {
		t.Error("dossier should contain destination")
	}
	if !contains(text, "2 flight") {
		t.Error("dossier should mention flight count")
	}
}

func TestClassifyInteractionTrend(t *testing.T) {
	tests := []struct {
		days     int
		total    int
		expected string
	}{
		{3, 10, "warming"},
		{14, 10, "stable"},
		{50, 10, "cooling"},
		{25, 3, "cooling"},
		{25, 10, "stable"},
	}

	for _, tt := range tests {
		got := classifyInteractionTrend(tt.days, tt.total)
		if got != tt.expected {
			t.Errorf("classifyInteractionTrend(%d, %d) = %s, want %s", tt.days, tt.total, got, tt.expected)
		}
	}
}

func TestPersonProfile_Struct(t *testing.T) {
	pp := PersonProfile{
		Name:              "Sarah",
		Email:             "sarah@example.com",
		TotalInteractions: 25,
		DaysSinceContact:  3,
		InteractionTrend:  "warming",
		SharedTopics:      []string{"product", "strategy"},
		PendingItems:      []string{"Review proposal"},
	}

	if pp.InteractionTrend != "warming" {
		t.Errorf("expected warming, got %s", pp.InteractionTrend)
	}
	if len(pp.SharedTopics) != 2 {
		t.Errorf("expected 2 shared topics, got %d", len(pp.SharedTopics))
	}
}

func TestDetectTripsFromEmail_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.DetectTripsFromEmail(nil)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestClassifyTripState_Boundary14Days(t *testing.T) {
	now := time.Now()
	// Exactly 14 days ago: After() is strict, so exactly 14 days ago is NOT After(now-14d)
	// meaning it falls to "completed". 13 days ago is still "active".
	justInside := now.AddDate(0, 0, -13)
	if classifyTripState(justInside) != "active" {
		t.Error("13 days ago should be active")
	}
	justOutside := now.AddDate(0, 0, -15)
	if classifyTripState(justOutside) != "completed" {
		t.Error("15 days ago should be completed")
	}
	// Exactly at boundary: departure exactly 14 days ago → completed (After is strict)
	exactBoundary := now.AddDate(0, 0, -14)
	if classifyTripState(exactBoundary) != "completed" {
		t.Error("exactly 14 days ago should be completed (After is strict)")
	}
}

func TestAssembleDossierText_OnlyCapturesNoFlightsNoHotels(t *testing.T) {
	// SCN-005-008d: trip with incomplete signals should render gracefully
	d := &TripDossier{
		Destination:     "Lisbon",
		State:           "upcoming",
		FlightArtifacts: nil,
		HotelArtifacts:  nil,
		RelatedCaptures: []string{"cap-1", "cap-2"},
	}

	text := assembleDossierText(d)
	if text == "" {
		t.Error("expected non-empty dossier text")
	}
	if !contains(text, "Lisbon") {
		t.Error("dossier should contain destination")
	}
	if !contains(text, "2 related") {
		t.Error("dossier should mention related captures")
	}
	// Should NOT mention flights or hotels when none exist
	if contains(text, "flight") {
		t.Error("dossier should not mention flights when none exist")
	}
	if contains(text, "lodging") {
		t.Error("dossier should not mention lodging when none exist")
	}
}

func TestAssembleDossierText_CompletlyEmpty(t *testing.T) {
	// A trip detected with incomplete signals may have only a destination
	d := &TripDossier{
		Destination: "Unknown",
		State:       "upcoming",
	}

	text := assembleDossierText(d)
	if text == "" {
		t.Error("expected non-empty dossier text even with no artifacts")
	}
	if !contains(text, "Unknown") {
		t.Error("dossier should still contain destination")
	}
}

func TestExtractDestination_ArrivingAtPattern(t *testing.T) {
	got := extractDestination("", "We will be arriving at the Marriott in Berlin")
	if got == "" {
		t.Error("expected extractDestination to match 'arriving at' pattern")
	}
}

func TestTripDossier_NilReturnDate(t *testing.T) {
	// SCN-005-008d: trip with no return date should still be valid
	d := TripDossier{
		Destination:   "Paris",
		DepartureDate: time.Now().AddDate(0, 0, 5),
		ReturnDate:    nil,
		State:         "upcoming",
	}
	if d.ReturnDate != nil {
		t.Error("expected nil return date for incomplete trip signal")
	}
	if d.State != "upcoming" {
		t.Errorf("expected upcoming, got %s", d.State)
	}
}

func TestGetPeopleIntelligence_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.GetPeopleIntelligence(nil)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

func TestClassifyInteractionTrend_BoundaryValues(t *testing.T) {
	tests := []struct {
		name     string
		days     int
		total    int
		expected string
	}{
		// Boundary at 7 days (warming threshold)
		{"exactly 7 days is stable", 7, 10, "stable"},
		{"6 days is warming", 6, 10, "warming"},
		// Boundary at 42 days (cooling threshold)
		{"exactly 42 days is stable", 42, 10, "stable"},
		{"43 days is cooling", 43, 10, "cooling"},
		// Boundary at 21 days with low interactions
		{"21 days, 4 interactions is stable", 21, 4, "stable"},
		{"22 days, 4 interactions is cooling", 22, 4, "cooling"},
		{"22 days, 5 interactions is stable", 22, 5, "stable"},
		// Zero interactions
		{"0 days, 0 interactions is warming", 0, 0, "warming"},
		{"30 days, 0 interactions is cooling", 30, 0, "cooling"},
		{"50 days, 0 interactions is cooling", 50, 0, "cooling"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyInteractionTrend(tt.days, tt.total)
			if got != tt.expected {
				t.Errorf("classifyInteractionTrend(%d, %d) = %q, want %q", tt.days, tt.total, got, tt.expected)
			}
		})
	}
}

func TestPersonProfile_EmptyCollections(t *testing.T) {
	pp := PersonProfile{
		Name:              "New Contact",
		TotalInteractions: 1,
		DaysSinceContact:  2,
		InteractionTrend:  "warming",
		SharedTopics:      nil,
		PendingItems:      nil,
	}

	if len(pp.SharedTopics) != 0 {
		t.Errorf("expected 0 shared topics, got %d", len(pp.SharedTopics))
	}
	if len(pp.PendingItems) != 0 {
		t.Errorf("expected 0 pending items, got %d", len(pp.PendingItems))
	}
}
