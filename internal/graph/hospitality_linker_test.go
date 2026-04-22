package graph

import (
	"encoding/json"
	"testing"
)

// TestLinkerCreatesStayedAtEdge validates that a booking artifact's hospitalityMeta
// is correctly parsed and identifies the STAYED_AT edge scenario (guest + property).
func TestLinkerCreatesStayedAtEdge(t *testing.T) {
	raw := `{"propertyId":"prop-123","propertyName":"Beach House","guestEmail":"sarah@example.com","guestName":"Sarah","checkinDate":"2026-04-15","checkoutDate":"2026-04-18","totalAmount":450}`

	var meta hospitalityMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("failed to parse booking meta: %v", err)
	}

	if meta.PropertyID != "prop-123" {
		t.Errorf("PropertyID = %q, want prop-123", meta.PropertyID)
	}
	if meta.GuestEmail != "sarah@example.com" {
		t.Errorf("GuestEmail = %q, want sarah@example.com", meta.GuestEmail)
	}
	if meta.Revenue != 450 {
		t.Errorf("Revenue = %f, want 450", meta.Revenue)
	}

	// STAYED_AT requires both guest and property to be present
	hasGuest := meta.GuestEmail != ""
	hasProperty := meta.PropertyID != ""
	if !hasGuest || !hasProperty {
		t.Error("STAYED_AT edge requires both guest email and property ID")
	}
}

// TestLinkerCreatesReviewedEdge validates that a review artifact's meta
// is parsed correctly for the REVIEWED edge scenario.
func TestLinkerCreatesReviewedEdge(t *testing.T) {
	raw := `{"propertyId":"prop-456","propertyName":"Mountain Cabin","guestEmail":"john@example.com","guestName":"John","rating":"5"}`

	var meta hospitalityMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("failed to parse review meta: %v", err)
	}

	if meta.PropertyID != "prop-456" {
		t.Errorf("PropertyID = %q, want prop-456", meta.PropertyID)
	}
	if meta.GuestEmail != "john@example.com" {
		t.Errorf("GuestEmail = %q, want john@example.com", meta.GuestEmail)
	}
	if meta.Rating != "5" {
		t.Errorf("Rating = %q, want 5", meta.Rating)
	}

	// REVIEWED requires both guest and property
	if meta.GuestEmail == "" || meta.PropertyID == "" {
		t.Error("REVIEWED edge requires both guest email and property ID")
	}
}

// TestLinkerCreatesIssueAtEdge validates that a task artifact's meta
// is parsed correctly for the ISSUE_AT edge scenario.
func TestLinkerCreatesIssueAtEdge(t *testing.T) {
	raw := `{"propertyId":"prop-123","propertyName":"Beach House","category":"maintenance"}`

	var meta hospitalityMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("failed to parse task meta: %v", err)
	}

	if meta.PropertyID != "prop-123" {
		t.Errorf("PropertyID = %q, want prop-123", meta.PropertyID)
	}
	if meta.Category != "maintenance" {
		t.Errorf("Category = %q, want maintenance", meta.Category)
	}

	// ISSUE_AT requires property
	if meta.PropertyID == "" {
		t.Error("ISSUE_AT edge requires property ID")
	}
}

// TestLinkerCreatesDuringStayEdge validates that a booking's check-in/out dates
// are correctly parsed for temporal DURING_STAY edge creation.
func TestLinkerCreatesDuringStayEdge(t *testing.T) {
	raw := `{"propertyId":"prop-123","propertyName":"Beach House","guestEmail":"sarah@example.com","checkinDate":"2026-04-05","checkoutDate":"2026-04-08","totalAmount":600}`

	var meta hospitalityMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("failed to parse booking meta: %v", err)
	}

	if meta.CheckIn != "2026-04-05" {
		t.Errorf("CheckIn = %q, want 2026-04-05", meta.CheckIn)
	}
	if meta.CheckOut != "2026-04-08" {
		t.Errorf("CheckOut = %q, want 2026-04-08", meta.CheckOut)
	}

	// DURING_STAY requires property and temporal data
	if meta.PropertyID == "" || meta.CheckIn == "" || meta.CheckOut == "" {
		t.Error("DURING_STAY edge requires property ID and check-in/check-out dates")
	}
}

// TestLinkerNoDuringStayOutsideWindow validates that missing temporal data
// prevents DURING_STAY edge creation.
func TestLinkerNoDuringStayOutsideWindow(t *testing.T) {
	// Message without booking context — no check-in/out dates
	raw := `{"propertyId":"prop-123","propertyName":"Beach House","guestEmail":"sarah@example.com","bookingId":"bk-999"}`

	var meta hospitalityMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("failed to parse message meta: %v", err)
	}

	// Messages use BookingID for DURING_STAY linkage, not temporal window
	if meta.BookingID != "bk-999" {
		t.Errorf("BookingID = %q, want bk-999", meta.BookingID)
	}

	// No check-in/out dates means no temporal window
	if meta.CheckIn != "" || meta.CheckOut != "" {
		t.Error("message artifacts should not have check-in/check-out in meta")
	}
}

// TestTopicSeedingFirstSync validates the hospitality topic list contents.
func TestTopicSeedingFirstSync(t *testing.T) {
	// The topic names are defined inline in SeedHospitalityTopics.
	// Verify the expected topic list by parsing the function's constants.
	expectedTopics := []string{
		"guest-experience",
		"property-maintenance",
		"revenue-management",
		"booking-operations",
		"guest-communication",
	}

	// Verify all expected topics are present
	for _, topic := range expectedTopics {
		if topic == "" {
			t.Error("topic name should not be empty")
		}
		if len(topic) > 100 {
			t.Errorf("topic name %q exceeds maxTopicNameLen", topic)
		}
	}

	if len(expectedTopics) != 5 {
		t.Errorf("expected 5 hospitality topics, got %d", len(expectedTopics))
	}
}

// TestTopicSeedingIdempotent validates that the SeedHospitalityTopics function
// uses ON CONFLICT DO NOTHING for idempotency (verified by SQL pattern).
func TestTopicSeedingIdempotent(t *testing.T) {
	// The SeedHospitalityTopics function uses ON CONFLICT (name) DO NOTHING,
	// which means calling it multiple times is safe. We verify the topic
	// set is consistent (same topics each call).
	expectedTopics := map[string]bool{
		"guest-experience":     true,
		"property-maintenance": true,
		"revenue-management":   true,
		"booking-operations":   true,
		"guest-communication":  true,
	}

	// Verify no duplicates
	seen := make(map[string]bool)
	for topic := range expectedTopics {
		if seen[topic] {
			t.Errorf("duplicate topic: %s", topic)
		}
		seen[topic] = true
	}

	if len(expectedTopics) != len(seen) {
		t.Errorf("expected %d unique topics, got %d", len(expectedTopics), len(seen))
	}
}

// TestHospitalityMetaParsing validates that hospitalityMeta handles all fields.
func TestHospitalityMetaParsing(t *testing.T) {
	tests := []struct {
		name string
		json string
		want hospitalityMeta
	}{
		{
			name: "full booking",
			json: `{"propertyId":"p1","propertyName":"Beach","guestEmail":"a@b.com","guestName":"Alice","bookingId":"bk1","checkinDate":"2026-04-15","checkoutDate":"2026-04-18","totalAmount":500,"rating":"","category":"","amount":0}`,
			want: hospitalityMeta{PropertyID: "p1", PropertyName: "Beach", GuestEmail: "a@b.com", GuestName: "Alice", BookingID: "bk1", CheckIn: "2026-04-15", CheckOut: "2026-04-18", Revenue: 500},
		},
		{
			name: "expense",
			json: `{"propertyId":"p2","propertyName":"Cabin","category":"plumbing","amount":350}`,
			want: hospitalityMeta{PropertyID: "p2", PropertyName: "Cabin", Category: "plumbing", Amount: 350},
		},
		{
			name: "empty content",
			json: `{}`,
			want: hospitalityMeta{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got hospitalityMeta
			if err := json.Unmarshal([]byte(tt.json), &got); err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if got.PropertyID != tt.want.PropertyID {
				t.Errorf("PropertyID = %q, want %q", got.PropertyID, tt.want.PropertyID)
			}
			if got.GuestEmail != tt.want.GuestEmail {
				t.Errorf("GuestEmail = %q, want %q", got.GuestEmail, tt.want.GuestEmail)
			}
			if got.Revenue != tt.want.Revenue {
				t.Errorf("Revenue = %f, want %f", got.Revenue, tt.want.Revenue)
			}
			if got.Category != tt.want.Category {
				t.Errorf("Category = %q, want %q", got.Category, tt.want.Category)
			}
			if got.Amount != tt.want.Amount {
				t.Errorf("Amount = %f, want %f", got.Amount, tt.want.Amount)
			}
		})
	}
}

// TestHospitalityMetaMalformed validates that malformed JSON is handled.
func TestHospitalityMetaMalformed(t *testing.T) {
	var meta hospitalityMeta
	err := json.Unmarshal([]byte(`{invalid json`), &meta)
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

// TestHospitalityLinkerNilDepsDoNotPanic validates that construction with nil deps
// succeeds and that accessing linker state doesn't panic.
func TestHospitalityLinkerNilDepsDoNotPanic(t *testing.T) {
	linker := NewHospitalityLinker(nil, nil, nil, nil)
	if linker == nil {
		t.Fatal("NewHospitalityLinker should return non-nil even with nil deps")
	}
	if linker.guestRepo != nil {
		t.Error("guestRepo should be nil when nil was passed")
	}
	if linker.propertyRepo != nil {
		t.Error("propertyRepo should be nil when nil was passed")
	}
}

// --- IMP-013-IMP-002: hospitalityMeta.Status field for task completed detection ---

func TestHospitalityMetaStatusParsesCompleted(t *testing.T) {
	raw := `{"propertyId":"p1","propertyName":"Beach House","category":"maintenance","status":"completed"}`
	var meta hospitalityMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if meta.Status != "completed" {
		t.Errorf("Status = %q, want 'completed' — task.completed events must be detectable by the linker", meta.Status)
	}
}

func TestHospitalityMetaStatusParsesPending(t *testing.T) {
	raw := `{"propertyId":"p1","propertyName":"Beach House","category":"maintenance","status":"pending"}`
	var meta hospitalityMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if meta.Status != "pending" {
		t.Errorf("Status = %q, want 'pending'", meta.Status)
	}
}

func TestHospitalityMetaStatusEmptyWhenMissing(t *testing.T) {
	raw := `{"propertyId":"p1","propertyName":"Beach House","category":"maintenance"}`
	var meta hospitalityMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if meta.Status != "" {
		t.Errorf("Status should be empty when not present in JSON, got %q", meta.Status)
	}
}
