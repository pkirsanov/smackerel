package hospitable

import (
	"math"
	"strings"
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
	if !strings.Contains(a.RawContent, "Beach House") {
		t.Error("content should contain property name")
	}
	if !strings.Contains(a.RawContent, "Pool") {
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
	if !strings.Contains(a.RawContent, "Airbnb") {
		t.Error("content should contain channel")
	}
	if !strings.Contains(a.RawContent, "$750") {
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
	if !strings.Contains(a.Title, "prop-xyz") {
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
	if !strings.Contains(a.RawContent, "16 days lead time") {
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
	if !strings.Contains(a.RawContent, "Wi-Fi password") {
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
	if !strings.Contains(a.RawContent, "Amazing stay!") {
		t.Error("content should contain review text")
	}
	if !strings.Contains(a.RawContent, "Thank you!") {
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
	if !strings.Contains(a.Title, "prop-abc") {
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
	if !strings.Contains(a.RawContent, "(host)") {
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
	if !strings.Contains(a.RawContent, "4.5★") {
		t.Errorf("Content should contain 4.5★, got: %s", a.RawContent)
	}
}

// --- formatAddress: Partial Field Combos ---

func TestFormatAddressFull(t *testing.T) {
	a := Address{Street: "123 Ocean Dr", City: "Malibu", State: "CA", Country: "US", Zip: "90265"}
	got := formatAddress(a)
	if got != "123 Ocean Dr, Malibu, CA, US, 90265" {
		t.Errorf("formatAddress full = %q", got)
	}
}

func TestFormatAddressCityOnly(t *testing.T) {
	a := Address{City: "Paris"}
	got := formatAddress(a)
	if got != "Paris" {
		t.Errorf("formatAddress city-only = %q", got)
	}
}

func TestFormatAddressStateOnly(t *testing.T) {
	a := Address{State: "CA"}
	got := formatAddress(a)
	if got != "CA" {
		t.Errorf("formatAddress state-only = %q", got)
	}
}

func TestFormatAddressCityState(t *testing.T) {
	a := Address{City: "Malibu", State: "CA"}
	got := formatAddress(a)
	if got != "Malibu, CA" {
		t.Errorf("formatAddress city+state = %q", got)
	}
}

func TestFormatAddressEmpty(t *testing.T) {
	a := Address{}
	got := formatAddress(a)
	if got != "" {
		t.Errorf("formatAddress empty = %q, want empty", got)
	}
}

func TestFormatAddressStreetCountryOnly(t *testing.T) {
	a := Address{Street: "123 Main St", Country: "UK"}
	got := formatAddress(a)
	if got != "123 Main St, UK" {
		t.Errorf("formatAddress street+country = %q", got)
	}
}

// --- formatDate: RFC3339 Fallback ---

func TestFormatDateStandard(t *testing.T) {
	got := formatDate("2026-04-15")
	if got != "Apr 15" {
		t.Errorf("formatDate standard = %q, want Apr 15", got)
	}
}

func TestFormatDateRFC3339Fallback(t *testing.T) {
	got := formatDate("2026-04-15T10:30:00Z")
	if got != "Apr 15" {
		t.Errorf("formatDate RFC3339 = %q, want Apr 15", got)
	}
}

func TestFormatDateInvalidReturnsOriginal(t *testing.T) {
	got := formatDate("not-a-date")
	if got != "not-a-date" {
		t.Errorf("formatDate invalid = %q, want original string", got)
	}
}

func TestFormatDateEmptyString(t *testing.T) {
	got := formatDate("")
	if got != "" {
		t.Errorf("formatDate empty = %q, want empty", got)
	}
}

// --- Reservation: Zero BookedAt (no lead time) ---

func TestNormalizeReservationZeroBookedAt(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard"}
	r := Reservation{
		ID:         "res-nobooked",
		PropertyID: "prop-001",
		CheckIn:    "2026-04-15",
		CheckOut:   "2026-04-18",
		GuestName:  "John Smith",
		CreatedAt:  time.Date(2026, 3, 25, 0, 0, 0, 0, time.UTC),
		// BookedAt zero — should fallback to CreatedAt for CapturedAt and skip lead time line
	}
	a := NormalizeReservation(r, "Beach House", cfg)

	if !a.CapturedAt.Equal(r.CreatedAt) {
		t.Errorf("CapturedAt = %v, want CreatedAt %v (BookedAt is zero)", a.CapturedAt, r.CreatedAt)
	}
	if strings.Contains(a.RawContent, "lead time") {
		t.Errorf("content should NOT contain lead time when BookedAt is zero: %s", a.RawContent)
	}
	if strings.Contains(a.RawContent, "Booked:") {
		t.Errorf("content should NOT contain Booked line when BookedAt is zero: %s", a.RawContent)
	}
}

// --- Review: No Host Response ---

func TestNormalizeReviewNoHostResponse(t *testing.T) {
	cfg := HospitableConfig{TierReviews: "full"}
	r := Review{
		ID:           "rev-noresp",
		PropertyID:   "prop-001",
		Rating:       4,
		ReviewText:   "Nice place, well maintained.",
		HostResponse: "",
		Channel:      "VRBO",
		SubmittedAt:  time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
	}
	a := NormalizeReview(r, "Beach House", cfg)

	if strings.Contains(a.RawContent, "Host Response") {
		t.Errorf("content should NOT contain 'Host Response' when empty: %s", a.RawContent)
	}
	if !strings.Contains(a.RawContent, "Nice place") {
		t.Error("content should contain review text")
	}
	if !strings.Contains(a.RawContent, "VRBO") {
		t.Error("content should contain channel")
	}
}

// --- Property: CapturedAt Fallback Chain ---

func TestNormalizePropertyCapturedAtCreatedAt(t *testing.T) {
	cfg := HospitableConfig{TierProperties: "light"}
	p := Property{
		ID:        "p-fallback",
		Name:      "Fallback House",
		CreatedAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		// UpdatedAt zero — should fallback to CreatedAt
	}
	a := NormalizeProperty(p, cfg)

	if !a.CapturedAt.Equal(p.CreatedAt) {
		t.Errorf("CapturedAt = %v, want CreatedAt %v (UpdatedAt is zero)", a.CapturedAt, p.CreatedAt)
	}
}

func TestNormalizePropertyCapturedAtFallbackNow(t *testing.T) {
	cfg := HospitableConfig{TierProperties: "light"}
	before := time.Now().Add(-time.Second)
	p := Property{
		ID:   "p-now",
		Name: "Now House",
		// Both UpdatedAt and CreatedAt zero — should fallback to now
	}
	a := NormalizeProperty(p, cfg)

	if a.CapturedAt.Before(before) {
		t.Errorf("CapturedAt = %v, should be ~now", a.CapturedAt)
	}
}

// --- Message: CapturedAt Fallback ---

func TestNormalizeMessageCapturedAtFallbackNow(t *testing.T) {
	cfg := HospitableConfig{TierMessages: "full"}
	before := time.Now().Add(-time.Second)
	m := Message{
		ID:     "msg-notime",
		Sender: "Test",
		Body:   "Hello",
		// SentAt zero — should fallback to now
	}
	a := NormalizeMessage(m, "res-1", cfg)

	if a.CapturedAt.Before(before) {
		t.Errorf("CapturedAt = %v, should be ~now when SentAt is zero", a.CapturedAt)
	}
}

// --- firstNonEmpty ---

func TestFirstNonEmptyMultiple(t *testing.T) {
	got := firstNonEmpty([]string{"", "", "third", "fourth"})
	if got != "third" {
		t.Errorf("firstNonEmpty = %q, want third", got)
	}
}

func TestFirstNonEmptyAllEmpty(t *testing.T) {
	got := firstNonEmpty([]string{"", ""})
	if got != "" {
		t.Errorf("firstNonEmpty all-empty = %q, want empty", got)
	}
}

func TestFirstNonEmptyNil(t *testing.T) {
	got := firstNonEmpty(nil)
	if got != "" {
		t.Errorf("firstNonEmpty nil = %q, want empty", got)
	}
}

// --- SEC-012-001: ListingURL scheme validation (CWE-79/601) ---

func TestIsSafeURL(t *testing.T) {
	cases := map[string]bool{
		"https://airbnb.com/rooms/123": true,
		"http://localhost:8080":        true,
		"javascript:alert(1)":          false,
		"data:text/html,<h1>xss</h1>":  false,
		"vbscript:MsgBox('xss')":       false,
		"JAVASCRIPT:alert(1)":          false, // case-insensitive
		"ftp://evil.com/payload":       false,
		"file:///etc/passwd":           false,
		"":                             false,
		"not-a-url":                    false,
		"//evil.com/xss":               false, // protocol-relative
	}
	for u, want := range cases {
		t.Run(u, func(t *testing.T) {
			if got := isSafeURL(u); got != want {
				t.Errorf("isSafeURL(%q) = %v, want %v", u, got, want)
			}
		})
	}
}

func TestFirstSafeURL(t *testing.T) {
	// Only http/https should be returned
	got := firstSafeURL([]string{"javascript:alert(1)", "data:text/html,xss", "https://airbnb.com/rooms/123"})
	if got != "https://airbnb.com/rooms/123" {
		t.Errorf("firstSafeURL = %q, want https://airbnb.com/rooms/123", got)
	}
}

func TestFirstSafeURL_AllUnsafe(t *testing.T) {
	got := firstSafeURL([]string{"javascript:alert(1)", "data:text/html,xss", "vbscript:run"})
	if got != "" {
		t.Errorf("firstSafeURL all-unsafe = %q, want empty", got)
	}
}

func TestFirstSafeURL_Nil(t *testing.T) {
	got := firstSafeURL(nil)
	if got != "" {
		t.Errorf("firstSafeURL nil = %q, want empty", got)
	}
}

func TestNormalizePropertyRejectsJavascriptURL(t *testing.T) {
	cfg := HospitableConfig{TierProperties: "light"}
	p := Property{
		ID:          "p-xss",
		Name:        "XSS Property",
		ListingURLs: []string{"javascript:alert(document.cookie)"},
		UpdatedAt:   time.Now(),
	}
	a := NormalizeProperty(p, cfg)
	if a.URL != "" {
		t.Errorf("SEC-012-001: javascript: URL must be rejected (CWE-79), got %q", a.URL)
	}
}

func TestNormalizePropertyRejectsDataURL(t *testing.T) {
	cfg := HospitableConfig{TierProperties: "light"}
	p := Property{
		ID:          "p-data",
		Name:        "Data URL Property",
		ListingURLs: []string{"data:text/html,<script>alert(1)</script>", "https://airbnb.com/rooms/safe"},
		UpdatedAt:   time.Now(),
	}
	a := NormalizeProperty(p, cfg)
	if a.URL != "https://airbnb.com/rooms/safe" {
		t.Errorf("SEC-012-001: should skip data: URL and use safe URL, got %q", a.URL)
	}
}

// --- SEC-012-002: Reservation URL escape (CWE-79) ---

func TestNormalizeReservationURLPathEscape(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard", BaseURL: "https://api.hospitable.com"}
	r := Reservation{
		ID:       "res/../admin",
		CheckIn:  "2026-04-15",
		CheckOut: "2026-04-18",
		BookedAt: time.Now(),
	}
	a := NormalizeReservation(r, "", cfg)
	if strings.Contains(a.URL, "/../") {
		t.Errorf("SEC-012-002: reservation URL must escape path traversal, got %q", a.URL)
	}
	if !strings.Contains(a.URL, "res%2F..%2Fadmin") {
		t.Errorf("SEC-012-002: reservation ID must be path-escaped, got %q", a.URL)
	}
}

func TestNormalizeReservationURLQueryInjection(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard", BaseURL: "https://api.hospitable.com"}
	r := Reservation{
		ID:       "res-001?admin=true#fragment",
		CheckIn:  "2026-04-15",
		CheckOut: "2026-04-18",
		BookedAt: time.Now(),
	}
	a := NormalizeReservation(r, "", cfg)
	if strings.Contains(a.URL, "?admin=true") {
		t.Errorf("SEC-012-002: reservation URL must escape query injection, got %q", a.URL)
	}
}

// --- SEC-012-004: Rating clamping (CWE-20) ---

func TestClampRating(t *testing.T) {
	cases := []struct {
		input float64
		want  float64
	}{
		{5.0, 5.0},
		{4.5, 4.5},
		{0.0, 0.0},
		{-1.0, 0.0},
		{-999.0, 0.0},
		{6.0, 5.0},
		{100.0, 5.0},
		{math.NaN(), 0.0},
		{math.Inf(1), 0.0},
		{math.Inf(-1), 0.0},
	}
	for _, tc := range cases {
		got := clampRating(tc.input)
		if got != tc.want {
			t.Errorf("clampRating(%v) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeReviewNegativeRatingClamped(t *testing.T) {
	cfg := HospitableConfig{TierReviews: "full"}
	r := Review{
		ID:          "rev-neg",
		PropertyID:  "p1",
		Rating:      -5.0,
		ReviewText:  "Injected negative",
		SubmittedAt: time.Now(),
	}
	a := NormalizeReview(r, "Beach House", cfg)
	if strings.Contains(a.Title, "-") {
		t.Errorf("SEC-012-004: negative rating must be clamped to 0, got title %q", a.Title)
	}
	if a.Metadata["rating"] != 0.0 {
		t.Errorf("SEC-012-004: metadata rating should be 0, got %v", a.Metadata["rating"])
	}
}

func TestNormalizeReviewNaNRatingClamped(t *testing.T) {
	cfg := HospitableConfig{TierReviews: "full"}
	r := Review{
		ID:          "rev-nan",
		PropertyID:  "p1",
		Rating:      math.NaN(),
		ReviewText:  "Injected NaN",
		SubmittedAt: time.Now(),
	}
	a := NormalizeReview(r, "Beach House", cfg)
	if strings.Contains(a.Title, "NaN") {
		t.Errorf("SEC-012-004: NaN rating must be clamped to 0, got title %q", a.Title)
	}
}

func TestNormalizeReviewOverflowRatingClamped(t *testing.T) {
	cfg := HospitableConfig{TierReviews: "full"}
	r := Review{
		ID:          "rev-overflow",
		PropertyID:  "p1",
		Rating:      999.9,
		ReviewText:  "Injected overflow",
		SubmittedAt: time.Now(),
	}
	a := NormalizeReview(r, "Beach House", cfg)
	if a.Metadata["rating"] != 5.0 {
		t.Errorf("SEC-012-004: overflow rating must be clamped to 5, got %v", a.Metadata["rating"])
	}
}

// --- Coverage gap: NormalizeMessage with production BaseURL generates app URL ---

func TestNormalizeMessageURLProduction(t *testing.T) {
	cfg := HospitableConfig{TierMessages: "full", BaseURL: "https://api.hospitable.com"}
	m := Message{
		ID:            "msg-prod",
		ReservationID: "res-100",
		Sender:        "Guest",
		Body:          "Hello",
		SentAt:        time.Now(),
	}
	a := NormalizeMessage(m, "res-100", cfg)
	if a.URL != "https://app.hospitable.com/reservations/res-100" {
		t.Errorf("Message URL = %q, want production dashboard URL", a.URL)
	}
}

func TestNormalizeMessageURLNonProduction(t *testing.T) {
	cfg := HospitableConfig{TierMessages: "full", BaseURL: "http://localhost:8080"}
	m := Message{
		ID:     "msg-local",
		Sender: "Guest",
		Body:   "Hello",
		SentAt: time.Now(),
	}
	a := NormalizeMessage(m, "res-100", cfg)
	if a.URL != "" {
		t.Errorf("Message URL = %q, want empty for non-production", a.URL)
	}
}

// --- Coverage gap: NormalizeReview with production BaseURL + ReservationID generates app URL ---

func TestNormalizeReviewURLProduction(t *testing.T) {
	cfg := HospitableConfig{TierReviews: "full", BaseURL: "https://api.hospitable.com"}
	r := Review{
		ID:            "rev-prod",
		PropertyID:    "p1",
		ReservationID: "res-200",
		Rating:        4,
		ReviewText:    "Nice stay",
		SubmittedAt:   time.Now(),
	}
	a := NormalizeReview(r, "Beach House", cfg)
	if a.URL != "https://app.hospitable.com/reservations/res-200" {
		t.Errorf("Review URL = %q, want production dashboard URL", a.URL)
	}
}

func TestNormalizeReviewURLNoReservationID(t *testing.T) {
	cfg := HospitableConfig{TierReviews: "full", BaseURL: "https://api.hospitable.com"}
	r := Review{
		ID:          "rev-nores",
		PropertyID:  "p1",
		Rating:      4,
		ReviewText:  "Nice stay",
		SubmittedAt: time.Now(),
	}
	a := NormalizeReview(r, "Beach House", cfg)
	if a.URL != "" {
		t.Errorf("Review URL = %q, want empty when no ReservationID", a.URL)
	}
}

// --- Coverage gap: isSafeURL with unparseable URL ---

func TestIsSafeURLUnparseable(t *testing.T) {
	// url.Parse is very lenient, but we can test the general logic
	if isSafeURL("") {
		t.Error("empty string should not be safe")
	}
	if isSafeURL("://missing-scheme") {
		t.Error("URL without valid scheme should not be safe")
	}
}

// --- SEC-R82-001: Amenities/ChannelIDs control character sanitization ---

func TestNormalizePropertySanitizesAmenities(t *testing.T) {
	cfg := HospitableConfig{TierProperties: "light"}
	p := Property{
		ID:        "p-ctrl",
		Name:      "Clean House",
		Amenities: []string{"Pool\x00", "Hot\x07Tub", "\x1bOcean View"},
		UpdatedAt: time.Now(),
	}
	a := NormalizeProperty(p, cfg)
	if strings.ContainsAny(a.RawContent, "\x00\x07\x1b") {
		t.Error("SEC-R82-001: amenities with control chars must be sanitized in content")
	}
	amenities, ok := a.Metadata["amenities"].([]string)
	if !ok {
		t.Fatal("metadata amenities should be []string")
	}
	for _, am := range amenities {
		if strings.ContainsAny(am, "\x00\x07\x1b") {
			t.Errorf("SEC-R82-001: amenity metadata not sanitized: %q", am)
		}
	}
}

func TestNormalizePropertySanitizesChannelIDs(t *testing.T) {
	cfg := HospitableConfig{TierProperties: "light"}
	p := Property{
		ID:         "p-ctrl2",
		Name:       "Clean House",
		ChannelIDs: []string{"Airbnb\x00", "\x0DVRBO"},
		UpdatedAt:  time.Now(),
	}
	a := NormalizeProperty(p, cfg)
	if strings.ContainsAny(a.RawContent, "\x00\x0D") {
		t.Error("SEC-R82-001: channel IDs with control chars must be sanitized in content")
	}
	channels, ok := a.Metadata["channel_ids"].([]string)
	if !ok {
		t.Fatal("metadata channel_ids should be []string")
	}
	for _, ch := range channels {
		if strings.ContainsAny(ch, "\x00\x0D") {
			t.Errorf("SEC-R82-001: channel_id metadata not sanitized: %q", ch)
		}
	}
}

// --- SEC-R82-002: ListingURLs filtered in metadata ---

func TestNormalizePropertyFiltersUnsafeListingURLsFromMetadata(t *testing.T) {
	cfg := HospitableConfig{TierProperties: "light"}
	p := Property{
		ID:          "p-urls",
		Name:        "URL House",
		ListingURLs: []string{"javascript:alert(1)", "https://airbnb.com/rooms/123", "data:text/html,xss"},
		UpdatedAt:   time.Now(),
	}
	a := NormalizeProperty(p, cfg)
	urls, ok := a.Metadata["listing_urls"].([]string)
	if !ok {
		t.Fatal("metadata listing_urls should be []string")
	}
	for _, u := range urls {
		if !isSafeURL(u) {
			t.Errorf("SEC-R82-002: unsafe URL in metadata listing_urls: %q", u)
		}
	}
	if len(urls) != 1 {
		t.Errorf("SEC-R82-002: expected 1 safe URL, got %d", len(urls))
	}
	if a.URL != "https://airbnb.com/rooms/123" {
		t.Errorf("artifact URL should be the safe one, got %q", a.URL)
	}
}

// --- SEC-R82-003: Reservation date string sanitization ---

func TestNormalizeReservationSanitizesDateStrings(t *testing.T) {
	cfg := HospitableConfig{TierReservations: "standard"}
	r := Reservation{
		ID:         "res-ctrl",
		PropertyID: "p1",
		CheckIn:    "2026-04-15\x00",
		CheckOut:   "\x1b2026-04-18",
		GuestName:  "John",
		BookedAt:   time.Now(),
	}
	a := NormalizeReservation(r, "Beach House", cfg)
	if strings.ContainsAny(a.RawContent, "\x00\x1b") {
		t.Error("SEC-R82-003: date strings with control chars must be sanitized in content")
	}
	if ci, ok := a.Metadata["check_in"].(string); ok && strings.ContainsAny(ci, "\x00\x1b") {
		t.Errorf("SEC-R82-003: check_in metadata not sanitized: %q", ci)
	}
	if co, ok := a.Metadata["check_out"].(string); ok && strings.ContainsAny(co, "\x00\x1b") {
		t.Errorf("SEC-R82-003: check_out metadata not sanitized: %q", co)
	}
}
