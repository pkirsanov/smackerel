package hospitable

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// NormalizeProperty converts a Hospitable Property to a RawArtifact.
func NormalizeProperty(p Property, config HospitableConfig) connector.RawArtifact {
	content := buildPropertyContent(p)
	metadata := map[string]interface{}{
		"property_id":     p.ID,
		"name":            p.Name,
		"address":         formatAddress(p.Address),
		"bedrooms":        p.Bedrooms,
		"bathrooms":       p.Bathrooms,
		"max_guests":      p.MaxGuests,
		"amenities":       p.Amenities,
		"listing_urls":    p.ListingURLs,
		"channel_ids":     p.ChannelIDs,
		"processing_tier": config.TierProperties,
	}

	capturedAt := p.UpdatedAt
	if capturedAt.IsZero() {
		capturedAt = p.CreatedAt
	}
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC()
	}

	return connector.RawArtifact{
		SourceID:    "hospitable",
		SourceRef:   "property:" + p.ID,
		ContentType: "property/str-listing",
		Title:       p.Name,
		URL:         firstNonEmpty(p.ListingURLs),
		RawContent:  content,
		Metadata:    metadata,
		CapturedAt:  capturedAt,
	}
}

// NormalizeReservation converts a Hospitable Reservation to a RawArtifact.
func NormalizeReservation(r Reservation, propertyName string, config HospitableConfig) connector.RawArtifact {
	if propertyName == "" {
		propertyName = r.PropertyID
	}

	title := fmt.Sprintf("%s at %s (%s–%s)", r.GuestName, propertyName, formatDate(r.CheckIn), formatDate(r.CheckOut))
	content := buildReservationContent(r, propertyName)

	metadata := map[string]interface{}{
		"reservation_id":    r.ID,
		"property_id":       r.PropertyID,
		"property_name":     propertyName,
		"channel":           r.Channel,
		"status":            r.Status,
		"check_in":          r.CheckIn,
		"check_out":         r.CheckOut,
		"guest_name":        r.GuestName,
		"guest_count":       r.GuestCount,
		"nightly_rate":      r.NightlyRate,
		"total_payout":      r.TotalPayout,
		"nights":            r.Nights,
		"booked_at":         r.BookedAt.Format(time.RFC3339),
		"processing_tier":   config.TierReservations,
		"edge_belongs_to":   "property:" + r.PropertyID,
		"stay_window_start": r.CheckIn,
		"stay_window_end":   r.CheckOut,
		"stay_property_id":  r.PropertyID,
	}

	capturedAt := r.BookedAt
	if capturedAt.IsZero() {
		capturedAt = r.CreatedAt
	}
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC()
	}

	var reservationURL string
	if strings.Contains(config.BaseURL, "api.hospitable.com") {
		reservationURL = "https://app.hospitable.com/reservations/" + r.ID
	}

	return connector.RawArtifact{
		SourceID:    "hospitable",
		SourceRef:   "reservation:" + r.ID,
		ContentType: "reservation/str-booking",
		Title:       title,
		URL:         reservationURL,
		RawContent:  content,
		Metadata:    metadata,
		CapturedAt:  capturedAt,
	}
}

// NormalizeMessage converts a Hospitable Message to a RawArtifact.
func NormalizeMessage(m Message, reservationID string, config HospitableConfig) connector.RawArtifact {
	role := classifySender(m)
	title := fmt.Sprintf("Message from %s (%s)", m.Sender, role)
	content := buildMessageContent(m, reservationID)

	metadata := map[string]interface{}{
		"message_id":      m.ID,
		"reservation_id":  m.ReservationID,
		"sender":          m.Sender,
		"sender_role":     role,
		"is_automated":    m.IsAutomated,
		"sent_at":         m.SentAt.Format(time.RFC3339),
		"processing_tier": config.TierMessages,
		"edge_part_of":    "reservation:" + reservationID,
	}

	capturedAt := m.SentAt
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC()
	}

	return connector.RawArtifact{
		SourceID:    "hospitable",
		SourceRef:   "message:" + m.ID,
		ContentType: "message/str-conversation",
		Title:       title,
		RawContent:  content,
		Metadata:    metadata,
		CapturedAt:  capturedAt,
	}
}

// NormalizeReview converts a Hospitable Review to a RawArtifact.
func NormalizeReview(r Review, propertyName string, config HospitableConfig) connector.RawArtifact {
	if propertyName == "" {
		propertyName = r.PropertyID
	}

	title := fmt.Sprintf("Review: %s at %s", formatRating(r.Rating), propertyName)
	content := buildReviewContent(r, propertyName)

	metadata := map[string]interface{}{
		"review_id":       r.ID,
		"reservation_id":  r.ReservationID,
		"property_id":     r.PropertyID,
		"property_name":   propertyName,
		"rating":          r.Rating,
		"channel":         r.Channel,
		"submitted_at":    r.SubmittedAt.Format(time.RFC3339),
		"processing_tier": config.TierReviews,
		"edge_review_of":  "property:" + r.PropertyID,
	}

	capturedAt := r.SubmittedAt
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC()
	}

	return connector.RawArtifact{
		SourceID:    "hospitable",
		SourceRef:   "review:" + r.ID,
		ContentType: "review/str-guest",
		Title:       title,
		RawContent:  content,
		Metadata:    metadata,
		CapturedAt:  capturedAt,
	}
}

// --- Content builders ---

func buildPropertyContent(p Property) string {
	var b strings.Builder
	b.WriteString(p.Name)
	b.WriteString("\n")

	addr := formatAddress(p.Address)
	if addr != "" {
		b.WriteString(addr)
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("Bedrooms: %d | Bathrooms: %d | Max Guests: %d\n", p.Bedrooms, p.Bathrooms, p.MaxGuests))

	if len(p.Amenities) > 0 {
		b.WriteString("Amenities: ")
		b.WriteString(strings.Join(p.Amenities, ", "))
		b.WriteString("\n")
	}

	if len(p.ChannelIDs) > 0 {
		b.WriteString("Channels: ")
		b.WriteString(strings.Join(p.ChannelIDs, ", "))
		b.WriteString("\n")
	}

	return b.String()
}

func buildReservationContent(r Reservation, propertyName string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Reservation: %s at %s\n", r.GuestName, propertyName))
	b.WriteString(fmt.Sprintf("Channel: %s | Status: %s\n", r.Channel, r.Status))
	b.WriteString(fmt.Sprintf("Check-in: %s | Check-out: %s | Nights: %d\n", formatDate(r.CheckIn), formatDate(r.CheckOut), r.Nights))
	b.WriteString(fmt.Sprintf("Guests: %d | Nightly Rate: $%.0f | Total: $%.0f\n", r.GuestCount, r.NightlyRate, r.TotalPayout))

	if !r.BookedAt.IsZero() {
		checkIn, err := time.Parse("2006-01-02", r.CheckIn)
		if err == nil && checkIn.After(r.BookedAt) {
			leadDays := int(checkIn.Sub(r.BookedAt).Hours() / 24)
			b.WriteString(fmt.Sprintf("Booked: %s (%d days lead time)\n", r.BookedAt.Format("Jan 2, 2006"), leadDays))
		} else {
			b.WriteString(fmt.Sprintf("Booked: %s\n", r.BookedAt.Format("Jan 2, 2006")))
		}
	}

	return b.String()
}

func buildMessageContent(m Message, reservationID string) string {
	var b strings.Builder
	role := classifySender(m)
	b.WriteString(fmt.Sprintf("From: %s (%s)\n", m.Sender, role))
	b.WriteString(fmt.Sprintf("Re: Reservation %s\n\n", reservationID))
	b.WriteString(m.Body)
	return b.String()
}

func buildReviewContent(r Review, propertyName string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Review: %s at %s\n", formatRating(r.Rating), propertyName))
	b.WriteString(fmt.Sprintf("Channel: %s\n\n", r.Channel))
	b.WriteString(r.ReviewText)

	if r.HostResponse != "" {
		b.WriteString("\n\nHost Response:\n")
		b.WriteString(r.HostResponse)
	}

	return b.String()
}

// --- Helpers ---

func formatAddress(a Address) string {
	parts := []string{}
	if a.Street != "" {
		parts = append(parts, a.Street)
	}
	cityState := ""
	if a.City != "" {
		cityState = a.City
	}
	if a.State != "" {
		if cityState != "" {
			cityState += ", " + a.State
		} else {
			cityState = a.State
		}
	}
	if cityState != "" {
		parts = append(parts, cityState)
	}
	if a.Country != "" {
		parts = append(parts, a.Country)
	}
	if a.Zip != "" {
		parts = append(parts, a.Zip)
	}
	return strings.Join(parts, ", ")
}

func formatDate(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		// Try RFC3339 fallback
		t, err = time.Parse(time.RFC3339, dateStr)
		if err != nil {
			return dateStr
		}
	}
	return t.Format("Jan 2")
}

// classifySender returns "guest", "host", or "automated" based on message fields (R-019).
func classifySender(m Message) string {
	if m.IsAutomated {
		return "automated"
	}
	if m.SenderRole == "host" {
		return "host"
	}
	return "guest"
}

// formatRating returns "5★" for whole numbers and "4.5★" for fractional ratings (R-022).
func formatRating(rating float64) string {
	if rating == math.Floor(rating) {
		return fmt.Sprintf("%.0f★", rating)
	}
	return fmt.Sprintf("%.1f★", rating)
}

// firstNonEmpty returns the first non-empty string from a slice, or "" (R-020).
func firstNonEmpty(ss []string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
