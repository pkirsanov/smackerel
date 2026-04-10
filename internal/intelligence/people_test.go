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
		Name:             "Sarah",
		Email:            "sarah@example.com",
		TotalInteractions: 25,
		DaysSinceContact: 3,
		InteractionTrend: "warming",
		SharedTopics:     []string{"product", "strategy"},
		PendingItems:     []string{"Review proposal"},
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

func TestGetPeopleIntelligence_NilPool(t *testing.T) {
	engine := &Engine{Pool: nil}
	_, err := engine.GetPeopleIntelligence(nil)
	if err == nil {
		t.Error("expected error for nil pool")
	}
}
