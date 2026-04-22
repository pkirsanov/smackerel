package guesthost

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"time"
	"unicode/utf8"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/stringutil"
)

// NormalizeEvent converts a GuestHost ActivityEvent into a RawArtifact.
func NormalizeEvent(event ActivityEvent) (connector.RawArtifact, error) {
	if len(event.Data) == 0 || string(event.Data) == "null" {
		return connector.RawArtifact{}, fmt.Errorf("event data is empty for event %s (type %s)", event.ID, event.Type)
	}

	capturedAt, err := time.Parse(time.RFC3339, event.Timestamp)
	if err != nil {
		return connector.RawArtifact{}, fmt.Errorf("parse event timestamp %q: %w", event.Timestamp, err)
	}

	sourceRef := event.ID
	if sourceRef == "" {
		contentHash := sha256.Sum256([]byte(event.Type + event.EntityID + event.Timestamp))
		sourceRef = fmt.Sprintf("%x", contentHash[:])
	}
	// H-013-R2-003: Cap sourceRef to prevent storing excessively long external IDs.
	if len(sourceRef) > maxSourceRefLen {
		sourceRef = sourceRef[:maxSourceRefLen]
	}

	var contentType, title string
	metadata := map[string]interface{}{}

	switch event.Type {
	case "booking.created", "booking.updated", "booking.cancelled":
		var d BookingData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal booking data: %w", err)
		}
		// IMP-013-SQS-002: Sanitize API-supplied text fields (CWE-116).
		d.PropertyName = stringutil.SanitizeControlChars(d.PropertyName)
		d.GuestName = stringutil.SanitizeControlChars(d.GuestName)
		d.GuestEmail = stringutil.SanitizeControlChars(d.GuestEmail)
		d.Source = stringutil.SanitizeControlChars(d.Source)
		contentType = "booking"
		switch event.Type {
		case "booking.created":
			title = fmt.Sprintf("%s — %s — %s-%s", d.PropertyName, d.GuestName, d.CheckIn, d.CheckOut)
		case "booking.updated":
			title = fmt.Sprintf("%s — Booking updated: %s", d.PropertyName, d.GuestName)
		case "booking.cancelled":
			title = fmt.Sprintf("%s — Booking cancelled: %s", d.PropertyName, d.GuestName)
		}
		var metaErr error
		metadata, metaErr = bookingMetadata(d)
		if metaErr != nil {
			return connector.RawArtifact{}, fmt.Errorf("booking metadata: %w", metaErr)
		}

	case "guest.created", "guest.updated":
		var d GuestData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal guest data: %w", err)
		}
		// IMP-013-SQS-002: Sanitize API-supplied text fields (CWE-116).
		d.Name = stringutil.SanitizeControlChars(d.Name)
		d.Email = stringutil.SanitizeControlChars(d.Email)
		contentType = "guest"
		if event.Type == "guest.created" {
			title = fmt.Sprintf("Guest: %s (%s)", d.Name, d.Email)
		} else {
			title = fmt.Sprintf("Guest updated: %s", d.Name)
		}
		metadata["guest_email"] = capMetaValue(d.Email)
		metadata["guest_name"] = capMetaValue(d.Name)

	case "review.received":
		var d ReviewData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal review data: %w", err)
		}
		// IMP-013-SQS-002: Sanitize API-supplied text fields (CWE-116).
		d.PropertyName = stringutil.SanitizeControlChars(d.PropertyName)
		d.GuestName = stringutil.SanitizeControlChars(d.GuestName)
		d.GuestEmail = stringutil.SanitizeControlChars(d.GuestEmail)
		d.Rating = stringutil.SanitizeControlChars(d.Rating)
		contentType = "review"
		title = fmt.Sprintf("%s — %s★ review from %s", d.PropertyName, d.Rating, d.GuestName)
		metadata["property_id"] = capMetaValue(d.PropertyID)
		metadata["property_name"] = capMetaValue(d.PropertyName)
		metadata["guest_email"] = capMetaValue(d.GuestEmail)
		metadata["guest_name"] = capMetaValue(d.GuestName)
		metadata["rating"] = capMetaValue(d.Rating)

	case "message.received":
		var d MessageData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal message data: %w", err)
		}
		// IMP-013-SQS-002: Sanitize API-supplied text fields (CWE-116).
		d.PropertyName = stringutil.SanitizeControlChars(d.PropertyName)
		d.GuestName = stringutil.SanitizeControlChars(d.GuestName)
		d.GuestEmail = stringutil.SanitizeControlChars(d.GuestEmail)
		// IMP-013-IMP-001: Sanitize Body and SenderRole (CWE-116).
		d.Body = stringutil.SanitizeControlChars(d.Body)
		d.SenderRole = stringutil.SanitizeControlChars(d.SenderRole)
		contentType = "guest_message"
		title = fmt.Sprintf("%s — Message from %s", d.PropertyName, d.GuestName)
		metadata["property_id"] = capMetaValue(d.PropertyID)
		metadata["property_name"] = capMetaValue(d.PropertyName)
		metadata["guest_email"] = capMetaValue(d.GuestEmail)
		metadata["guest_name"] = capMetaValue(d.GuestName)
		metadata["booking_id"] = capMetaValue(d.BookingID)

	case "task.created", "task.completed":
		var d TaskData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal task data: %w", err)
		}
		// IMP-013-SQS-002: Sanitize API-supplied text fields (CWE-116).
		d.PropertyName = stringutil.SanitizeControlChars(d.PropertyName)
		d.Title = stringutil.SanitizeControlChars(d.Title)
		d.Category = stringutil.SanitizeControlChars(d.Category)
		contentType = "task"
		if event.Type == "task.created" {
			title = fmt.Sprintf("%s — Task: %s", d.PropertyName, d.Title)
		} else {
			title = fmt.Sprintf("%s — Task completed: %s", d.PropertyName, d.Title)
		}
		metadata["property_id"] = capMetaValue(d.PropertyID)
		metadata["property_name"] = capMetaValue(d.PropertyName)
		metadata["category"] = capMetaValue(d.Category)
		// IMP-013-IMP-002: Store task status so the hospitality linker can
		// differentiate task.created (increment issue count) from
		// task.completed (decrement issue count).
		metadata["task_status"] = capMetaValue(d.Status)

	case "expense.created":
		var d ExpenseData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal expense data: %w", err)
		}
		// IMP-013-SQS-002: Guard against IEEE 754 Inf/NaN from JSON 1e999.
		if math.IsNaN(d.Amount) || math.IsInf(d.Amount, 0) {
			return connector.RawArtifact{}, fmt.Errorf("expense amount is not a finite number")
		}
		// IMP-013-SQS-002: Sanitize API-supplied text fields (CWE-116).
		d.PropertyName = stringutil.SanitizeControlChars(d.PropertyName)
		d.Description = stringutil.SanitizeControlChars(d.Description)
		contentType = "financial"
		title = fmt.Sprintf("%s — Expense: %s $%.2f", d.PropertyName, d.Description, d.Amount)
		metadata["property_id"] = capMetaValue(d.PropertyID)
		metadata["property_name"] = capMetaValue(d.PropertyName)
		// Store as negative (expense). If the API already sent a negative value, preserve it.
		if d.Amount > 0 {
			metadata["amount"] = -d.Amount
		} else {
			metadata["amount"] = d.Amount
		}

	case "property.updated":
		var d PropertyData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal property data: %w", err)
		}
		// IMP-013-SQS-002: Sanitize API-supplied text fields (CWE-116).
		d.Name = stringutil.SanitizeControlChars(d.Name)
		contentType = "property"
		title = fmt.Sprintf("Property updated: %s", d.Name)
		metadata["property_id"] = capMetaValue(d.ID)

	default:
		return connector.RawArtifact{}, fmt.Errorf("unknown event type: %s", event.Type)
	}

	// H-013-R2-004: Cap raw content to prevent storing excessively large payloads.
	rawContent := string(event.Data)
	if len(rawContent) > maxRawContentLen {
		rawContent = rawContent[:maxRawContentLen]
	}

	return connector.RawArtifact{
		SourceID:    "guesthost",
		SourceRef:   sourceRef,
		ContentType: contentType,
		Title:       truncateStr(title, 500),
		RawContent:  rawContent,
		URL:         "",
		Metadata:    metadata,
		CapturedAt:  capturedAt,
	}, nil
}

// truncateStr truncates s to maxLen bytes at a valid UTF-8 rune boundary,
// appending "..." if truncated. Never produces invalid UTF-8 (CWE-838).
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		// Even for tiny limits, respect rune boundaries.
		cut := maxLen
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		return s[:cut]
	}
	// Walk back from the byte budget to find a valid rune boundary.
	cut := maxLen - 3
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

// bookingMetadata builds the common metadata map for all booking event types.
// R27-H23-03: Rejects Inf/NaN with an error (consistent with expense.created).
func bookingMetadata(d BookingData) (map[string]interface{}, error) {
	if math.IsNaN(d.TotalPrice) || math.IsInf(d.TotalPrice, 0) {
		return nil, fmt.Errorf("booking total_price is not a finite number")
	}
	return map[string]interface{}{
		"property_id":    capMetaValue(d.PropertyID),
		"property_name":  capMetaValue(d.PropertyName),
		"guest_email":    capMetaValue(d.GuestEmail),
		"guest_name":     capMetaValue(d.GuestName),
		"checkin_date":   capMetaValue(d.CheckIn),
		"checkout_date":  capMetaValue(d.CheckOut),
		"booking_source": capMetaValue(d.Source),
		"total_price":    d.TotalPrice,
	}, nil
}

// capMetaValue truncates a metadata string value to maxMetadataValueLen bytes
// (H-013-R2-002: prevent storing excessively long API-supplied values).
func capMetaValue(s string) string {
	if len(s) <= maxMetadataValueLen {
		return s
	}
	return stringutil.TruncateUTF8(s, maxMetadataValueLen)
}
