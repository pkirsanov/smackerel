package intelligence

import (
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/stringutil"
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
		{3, 15, "increasing"},
		{3, 5, "stable"},
		{14, 10, "stable"},
		{50, 10, "decreasing"},
		{50, 2, "lapsed"},
		{25, 3, "decreasing"},
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
		InteractionTrend:  "increasing",
		SharedTopics:      []string{"product", "strategy"},
		PendingItems:      []string{"Review proposal"},
	}

	if pp.InteractionTrend != "increasing" {
		t.Errorf("expected increasing, got %s", pp.InteractionTrend)
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
		// Boundary at 7 days (increasing/stable threshold)
		{"exactly 7 days is stable", 7, 10, "stable"},
		{"6 days, 10 interactions is stable", 6, 10, "stable"},
		{"6 days, 15 interactions is increasing", 6, 15, "increasing"},
		// Boundary at 42 days (decreasing/lapsed threshold)
		{"exactly 42 days is stable", 42, 10, "stable"},
		{"43 days, 10 interactions is decreasing", 43, 10, "decreasing"},
		// Boundary at 21 days with low interactions
		{"21 days, 4 interactions is stable", 21, 4, "stable"},
		{"22 days, 4 interactions is decreasing", 22, 4, "decreasing"},
		{"22 days, 5 interactions is stable", 22, 5, "stable"},
		// Zero interactions
		{"0 days, 0 interactions is stable", 0, 0, "stable"},
		{"30 days, 0 interactions is decreasing", 30, 0, "decreasing"},
		{"50 days, 0 interactions is lapsed", 50, 0, "lapsed"},
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
		InteractionTrend:  "stable",
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

// === Edge cases: extractDestination ===

func TestExtractDestination_MultipleMarkers(t *testing.T) {
	// Both "to" in title and "destination:" in content are present.
	// extractDestination concatenates title+content and finds the first marker.
	// "flight to london destination: tokyo" → "to " matches first → takes "london destination:"
	// which grabs "London" + second word "Destination:" (>2 chars, so it's included).
	got := extractDestination("Flight to London", "Destination: Tokyo")
	if got == "" {
		t.Error("expected non-empty destination")
	}
	// The "to " marker fires first on the combined string, yielding "London Destination:"
	if !strings.Contains(got, "London") {
		t.Errorf("expected destination containing London, got %q", got)
	}
}

func TestExtractDestination_MarkerAtEnd(t *testing.T) {
	// "to " marker but nothing after it
	got := extractDestination("Going to ", "")
	if got != "" {
		t.Errorf("expected empty for trailing marker, got %q", got)
	}
}

func TestExtractDestination_CheckInWithMultiWord(t *testing.T) {
	got := extractDestination("", "Check-in at Hilton Garden Inn")
	if got == "" {
		t.Error("expected non-empty for check-in pattern")
	}
	if !strings.Contains(got, "Hilton") {
		t.Errorf("expected destination containing Hilton, got %q", got)
	}
}

// === Edge cases: assembleDossierText all artifact types ===

func TestAssembleDossierText_AllTypes(t *testing.T) {
	d := &TripDossier{
		Destination:     "Rome",
		State:           "active",
		FlightArtifacts: []string{"f1"},
		HotelArtifacts:  []string{"h1", "h2"},
		PlaceArtifacts:  []string{"p1"},
		RelatedCaptures: []string{"c1", "c2", "c3"},
	}

	text := assembleDossierText(d)
	if !strings.Contains(text, "1 flight") {
		t.Error("should mention 1 flight")
	}
	if !strings.Contains(text, "2 lodging") {
		t.Error("should mention 2 lodging")
	}
	if !strings.Contains(text, "3 related") {
		t.Error("should mention 3 related captures")
	}
	if !strings.Contains(text, "active") {
		t.Error("should show active state")
	}
}

// === Chaos: escapeLikePattern edge cases ===

func TestEscapeLikePattern_ChaosEdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user@example.com", "user@example.com"},
		{"user%40@example.com", "user\\%40@example.com"},
		{"under_score@mail.com", "under\\_score@mail.com"},
		{"back\\slash@mail.com", "back\\\\slash@mail.com"},
		{"all%_\\chars", "all\\%\\_\\\\chars"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stringutil.EscapeLikePattern(tt.input)
			if got != tt.expected {
				t.Errorf("EscapeLikePattern(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// === Chaos: classifyInteractionTrend with extreme values ===

func TestClassifyInteractionTrend_ExtremeValues(t *testing.T) {
	// Very large interactions with recent contact should be increasing
	got := classifyInteractionTrend(0, 999999)
	if got != "increasing" {
		t.Errorf("0 days, huge interactions should be increasing, got %s", got)
	}

	// Large days_since with high interactions should be decreasing (not lapsed)
	got = classifyInteractionTrend(9999, 999999)
	if got != "decreasing" {
		t.Errorf("huge days, high interactions should be decreasing, got %s", got)
	}

	// Large days_since with low interactions should be lapsed
	got = classifyInteractionTrend(9999, 1)
	if got != "lapsed" {
		t.Errorf("huge days, low interactions should be lapsed, got %s", got)
	}
}

// === Chaos: assembleBriefText with empty attendees ===

func TestAssembleBriefText_EmptyAttendees(t *testing.T) {
	brief := MeetingBrief{
		EventTitle: "Strategy session",
		Attendees:  nil,
	}
	text := assembleBriefText(brief)
	if text == "" {
		t.Error("expected non-empty brief even with no attendees")
	}
	if !strings.Contains(text, "Strategy session") {
		t.Error("brief should contain the event title")
	}
}

func TestAssembleBriefText_MixedNewAndKnownContacts(t *testing.T) {
	brief := MeetingBrief{
		EventTitle: "Sync",
		Attendees: []AttendeeBrief{
			{Name: "alice@co.com", Email: "alice@co.com", IsNewContact: true},
			{Name: "Bob", Email: "bob@co.com", IsNewContact: false, RecentThreads: []string{"thread1"}, SharedTopics: []string{"go"}},
		},
	}
	text := assembleBriefText(brief)
	if !strings.Contains(text, "No prior context") {
		t.Error("new contact should show 'No prior context'")
	}
	if !strings.Contains(text, "Bob") {
		t.Error("known contact should show name")
	}
}
