package hospitable

import (
	"testing"
	"time"
)

func TestNormalizeProperty(t *testing.T) {
	cfg := HospitableConfig{TierProperties: "light"}
	p := Property{
		ID:         "prop-001",
		Name:       "Beach House",
		Address:    Address{Street: "123 Ocean Dr", City: "Malibu", State: "CA", Country: "US", Zip: "90265"},
		Bedrooms:   3,
		Bathrooms:  2,
		MaxGuests:  6,
		Amenities:  []string{"Pool", "Hot Tub", "Ocean View"},
		ChannelIDs: []string{"Airbnb", "VRBO"},
		UpdatedAt:  time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
	}

	a := NormalizeProperty(p, cfg)

	if a.SourceID != "hospitable" {
		t.Errorf("SourceID = %q, want hospitable", a.SourceID)
	}
	if a.SourceRef != "property:prop-001" {
		t.Errorf("SourceRef = %q, want property:prop-001", a.SourceRef)
	}
	if a.ContentType != "property/str-listing" {
		t.Errorf("ContentType = %q, want property/str-listing", a.ContentType)
	}
	if a.Title != "Beach House" {
		t.Errorf("Title = %q, want Beach House", a.Title)
	}
	if a.Metadata["processing_tier"] != "light" {
		t.Errorf("tier = %v, want light", a.Metadata["processing_tier"])
	}
	if a.Metadata["bedrooms"] != 3 {
		t.Errorf("bedrooms = %v, want 3", a.Metadata["bedrooms"])
	}
	if !a.CapturedAt.Equal(p.UpdatedAt) {
		t.Errorf("CapturedAt = %v, want %v", a.CapturedAt, p.UpdatedAt)
	}

	// Content should contain property details
	if !containsStr(a.RawContent, "Beach House") {
		t.Error("content should contain property name")
	}
	if !containsStr(a.RawContent, "Pool") {
		t.Error("content should contain amenities")
	}
}

func TestNormalizeReservation(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard"}
	r := Reservation{
		ID:          "res-001",
		PropertyID:  "prop-001",
		Channel:     "Airbnb",
		Status:      "confirmed",
		CheckIn:     "2026-04-15",
		CheckOut:    "2026-04-18",
		GuestName:   "John Smith",
		GuestCount:  4,
		NightlyRate: 250,
		TotalPayout: 750,
		Nights:      3,
		BookedAt:    time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
	}

	a := NormalizeReservation(r, "Beach House", cfg)

	if a.SourceRef != "reservation:res-001" {
		t.Errorf("SourceRef = %q, want reservation:res-001", a.SourceRef)
	}
	if a.ContentType != "reservation/str-booking" {
		t.Errorf("ContentType = %q, want reservation/str-booking", a.ContentType)
	}
	if a.Title != "John Smith at Beach House (Apr 15–Apr 18)" {
		t.Errorf("Title = %q", a.Title)
	}
	if a.Metadata["processing_tier"] != "standard" {
		t.Errorf("tier = %v, want standard", a.Metadata["processing_tier"])
	}

	// Edge hints
	if a.Metadata["edge_belongs_to"] != "property:prop-001" {
		t.Errorf("edge_belongs_to = %v", a.Metadata["edge_belongs_to"])
	}
	if a.Metadata["stay_window_start"] != "2026-04-15" {
		t.Errorf("stay_window_start = %v", a.Metadata["stay_window_start"])
	}
	if a.Metadata["stay_window_end"] != "2026-04-18" {
		t.Errorf("stay_window_end = %v", a.Metadata["stay_window_end"])
	}
	if a.Metadata["stay_property_id"] != "prop-001" {
		t.Errorf("stay_property_id = %v", a.Metadata["stay_property_id"])
	}

	// Content
	if !containsStr(a.RawContent, "Airbnb") {
		t.Error("content should contain channel")
	}
	if !containsStr(a.RawContent, "$750") {
		t.Error("content should contain total payout")
	}
}

func TestNormalizeReservationFallbackPropertyID(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard"}
	r := Reservation{
		ID:         "res-002",
		PropertyID: "prop-xyz",
		CheckIn:    "2026-05-01",
		CheckOut:   "2026-05-03",
		GuestName:  "Jane Doe",
		BookedAt:   time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
	}

	// Pass empty property name — should fall back to property ID
	a := NormalizeReservation(r, "", cfg)
	if !containsStr(a.Title, "prop-xyz") {
		t.Errorf("Title should fall back to property ID when name is empty: %s", a.Title)
	}
}

func TestNormalizeReservationLeadTime(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard"}
	r := Reservation{
		ID:         "res-lead",
		PropertyID: "prop-001",
		CheckIn:    "2026-04-15",
		CheckOut:   "2026-04-18",
		GuestName:  "John Smith",
		BookedAt:   time.Date(2026, 3, 30, 0, 0, 0, 0, time.UTC),
	}
	a := NormalizeReservation(r, "Beach House", cfg)
	if !containsStr(a.RawContent, "16 days lead time") {
		t.Errorf("content should contain lead time, got: %s", a.RawContent)
	}
}

func TestNormalizeMessage(t *testing.T) {
	cfg := HospitableConfig{TierMessages: "full"}
	m := Message{
		ID:            "msg-001",
		ReservationID: "res-001",
		Sender:        "John Smith",
		Body:          "What's the Wi-Fi password?",
		IsAutomated:   false,
		SentAt:        time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC),
	}

	a := NormalizeMessage(m, "res-001", cfg)

	if a.SourceRef != "message:msg-001" {
		t.Errorf("SourceRef = %q", a.SourceRef)
	}
	if a.ContentType != "message/str-conversation" {
		t.Errorf("ContentType = %q", a.ContentType)
	}
	if a.Title != "Message from John Smith (guest)" {
		t.Errorf("Title = %q", a.Title)
	}
	if a.Metadata["processing_tier"] != "full" {
		t.Errorf("tier = %v, want full", a.Metadata["processing_tier"])
	}
	if a.Metadata["edge_part_of"] != "reservation:res-001" {
		t.Errorf("edge_part_of = %v", a.Metadata["edge_part_of"])
	}
	if !containsStr(a.RawContent, "Wi-Fi password") {
		t.Error("content should contain message body")
	}
}

func TestNormalizeReview(t *testing.T) {
	cfg := HospitableConfig{TierReviews: "full"}
	r := Review{
		ID:            "rev-001",
		PropertyID:    "prop-001",
		ReservationID: "res-001",
		Rating:        5,
		ReviewText:    "Amazing stay!",
		HostResponse:  "Thank you!",
		Channel:       "Airbnb",
		SubmittedAt:   time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
	}

	a := NormalizeReview(r, "Beach House", cfg)

	if a.SourceRef != "review:rev-001" {
		t.Errorf("SourceRef = %q", a.SourceRef)
	}
	if a.ContentType != "review/str-guest" {
		t.Errorf("ContentType = %q", a.ContentType)
	}
	if a.Title != "Review: 5★ at Beach House" {
		t.Errorf("Title = %q", a.Title)
	}
	if a.Metadata["processing_tier"] != "full" {
		t.Errorf("tier = %v, want full", a.Metadata["processing_tier"])
	}
	if a.Metadata["edge_review_of"] != "property:prop-001" {
		t.Errorf("edge_review_of = %v", a.Metadata["edge_review_of"])
	}
	if !containsStr(a.RawContent, "Amazing stay!") {
		t.Error("content should contain review text")
	}
	if !containsStr(a.RawContent, "Thank you!") {
		t.Error("content should contain host response")
	}
}

func TestNormalizeReviewFallbackPropertyID(t *testing.T) {
	cfg := HospitableConfig{TierReviews: "full"}
	r := Review{
		ID:          "rev-002",
		PropertyID:  "prop-abc",
		Rating:      4,
		ReviewText:  "Nice place",
		SubmittedAt: time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC),
	}

	a := NormalizeReview(r, "", cfg)
	if !containsStr(a.Title, "prop-abc") {
		t.Errorf("Title should fall back to property ID: %s", a.Title)
	}
}

func TestNormalizeAllTiers(t *testing.T) {
	cfg := HospitableConfig{
		TierProperties:   "light",
		TierReservations: "standard",
		TierMessages:     "full",
		TierReviews:      "full",
	}

	propA := NormalizeProperty(Property{ID: "p1", Name: "H", UpdatedAt: time.Now()}, cfg)
	resA := NormalizeReservation(Reservation{ID: "r1", CheckIn: "2026-01-01", CheckOut: "2026-01-02", BookedAt: time.Now()}, "", cfg)
	msgA := NormalizeMessage(Message{ID: "m1", Sender: "X", Body: "hi", SentAt: time.Now()}, "r1", cfg)
	revA := NormalizeReview(Review{ID: "rv1", Rating: 3, ReviewText: "ok", SubmittedAt: time.Now()}, "", cfg)

	if propA.Metadata["processing_tier"] != "light" {
		t.Errorf("property tier = %v, want light", propA.Metadata["processing_tier"])
	}
	if resA.Metadata["processing_tier"] != "standard" {
		t.Errorf("reservation tier = %v, want standard", resA.Metadata["processing_tier"])
	}
	if msgA.Metadata["processing_tier"] != "full" {
		t.Errorf("message tier = %v, want full", msgA.Metadata["processing_tier"])
	}
	if revA.Metadata["processing_tier"] != "full" {
		t.Errorf("review tier = %v, want full", revA.Metadata["processing_tier"])
	}
}

// --- helper ---

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- R-019: Sender Classification ---

func TestClassifySenderGuest(t *testing.T) {
	m := Message{Sender: "John", SenderRole: "guest", IsAutomated: false}
	if got := classifySender(m); got != "guest" {
		t.Errorf("classifySender = %q, want guest", got)
	}
}

func TestClassifySenderHost(t *testing.T) {
	m := Message{Sender: "Philip", SenderRole: "host", IsAutomated: false}
	if got := classifySender(m); got != "host" {
		t.Errorf("classifySender = %q, want host", got)
	}
}

func TestClassifySenderAutomated(t *testing.T) {
	m := Message{Sender: "System", SenderRole: "host", IsAutomated: true}
	if got := classifySender(m); got != "automated" {
		t.Errorf("classifySender = %q, want automated (IsAutomated takes precedence)", got)
	}
}

func TestClassifySenderDefaultGuest(t *testing.T) {
	m := Message{Sender: "Someone", SenderRole: "", IsAutomated: false}
	if got := classifySender(m); got != "guest" {
		t.Errorf("classifySender = %q, want guest (default)", got)
	}
}

func TestNormalizeMessageHostSender(t *testing.T) {
	cfg := HospitableConfig{TierMessages: "full"}
	m := Message{
		ID:         "msg-002",
		Sender:     "Philip",
		Body:       "Welcome!",
		SenderRole: "host",
		SentAt:     time.Now(),
	}
	a := NormalizeMessage(m, "res-001", cfg)
	if a.Title != "Message from Philip (host)" {
		t.Errorf("Title = %q, want host sender in title", a.Title)
	}
	if a.Metadata["sender_role"] != "host" {
		t.Errorf("sender_role = %v, want host", a.Metadata["sender_role"])
	}
	if !containsStr(a.RawContent, "(host)") {
		t.Error("content should contain (host) sender type")
	}
}

// --- R-020: URL Population ---

func TestNormalizePropertyURL(t *testing.T) {
	cfg := HospitableConfig{TierProperties: "light"}
	p := Property{
		ID:          "p1",
		Name:        "Beach House",
		ListingURLs: []string{"https://airbnb.com/rooms/12345", "https://vrbo.com/67890"},
		UpdatedAt:   time.Now(),
	}
	a := NormalizeProperty(p, cfg)
	if a.URL != "https://airbnb.com/rooms/12345" {
		t.Errorf("URL = %q, want first listing URL", a.URL)
	}
}

func TestNormalizePropertyNoURL(t *testing.T) {
	cfg := HospitableConfig{TierProperties: "light"}
	p := Property{ID: "p1", Name: "Beach House", UpdatedAt: time.Now()}
	a := NormalizeProperty(p, cfg)
	if a.URL != "" {
		t.Errorf("URL = %q, want empty when no listing URLs", a.URL)
	}
}

func TestNormalizeReservationURLProduction(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard", BaseURL: "https://api.hospitable.com"}
	r := Reservation{ID: "res-001", CheckIn: "2026-04-15", CheckOut: "2026-04-18", BookedAt: time.Now()}
	a := NormalizeReservation(r, "", cfg)
	if a.URL != "https://app.hospitable.com/reservations/res-001" {
		t.Errorf("URL = %q, want dashboard URL for production", a.URL)
	}
}

func TestNormalizeReservationURLTest(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard", BaseURL: "http://localhost:8080"}
	r := Reservation{ID: "res-001", CheckIn: "2026-04-15", CheckOut: "2026-04-18", BookedAt: time.Now()}
	a := NormalizeReservation(r, "", cfg)
	if a.URL != "" {
		t.Errorf("URL = %q, want empty for non-production base URL", a.URL)
	}
}

// --- R-022: Rating Precision ---

func TestFormatRatingWhole(t *testing.T) {
	if got := formatRating(5.0); got != "5★" {
		t.Errorf("formatRating(5.0) = %q, want 5★", got)
	}
}

func TestFormatRatingFractional(t *testing.T) {
	if got := formatRating(4.5); got != "4.5★" {
		t.Errorf("formatRating(4.5) = %q, want 4.5★", got)
	}
}

func TestFormatRatingZero(t *testing.T) {
	if got := formatRating(0); got != "0★" {
		t.Errorf("formatRating(0) = %q, want 0★", got)
	}
}

func TestNormalizeReviewFractionalRating(t *testing.T) {
	cfg := HospitableConfig{TierReviews: "full"}
	r := Review{
		ID:          "rev-003",
		PropertyID:  "prop-001",
		Rating:      4.5,
		ReviewText:  "Good stay",
		SubmittedAt: time.Now(),
	}
	a := NormalizeReview(r, "Beach House", cfg)
	if a.Title != "Review: 4.5★ at Beach House" {
		t.Errorf("Title = %q, want fractional rating", a.Title)
	}
	if !containsStr(a.RawContent, "4.5★") {
		t.Errorf("Content should contain 4.5★, got: %s", a.RawContent)
	}
}
