package guesthost

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// NormalizeEvent converts a GuestHost ActivityEvent into a RawArtifact.
func NormalizeEvent(event ActivityEvent) (connector.RawArtifact, error) {
	capturedAt, err := time.Parse(time.RFC3339, event.Timestamp)
	if err != nil {
		return connector.RawArtifact{}, fmt.Errorf("parse event timestamp %q: %w", event.Timestamp, err)
	}

	rawContent, err := json.Marshal(event.Data)
	if err != nil {
		return connector.RawArtifact{}, fmt.Errorf("marshal event data: %w", err)
	}

	contentHash := sha256.Sum256([]byte(event.Type + event.EntityID + event.Timestamp))
	sourceRef := fmt.Sprintf("%x", contentHash[:])

	if event.ID != "" {
		sourceRef = event.ID
	}

	var contentType, title string
	metadata := map[string]interface{}{}

	switch event.Type {
	case "booking.created":
		var d BookingData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal booking data: %w", err)
		}
		contentType = "booking"
		title = fmt.Sprintf("%s — %s — %s-%s", d.PropertyName, d.GuestName, d.CheckIn, d.CheckOut)
		metadata = bookingMetadata(d)

	case "booking.updated":
		var d BookingData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal booking data: %w", err)
		}
		contentType = "booking"
		title = fmt.Sprintf("%s — Booking updated: %s", d.PropertyName, d.GuestName)
		metadata = bookingMetadata(d)

	case "booking.cancelled":
		var d BookingData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal booking data: %w", err)
		}
		contentType = "booking"
		title = fmt.Sprintf("%s — Booking cancelled: %s", d.PropertyName, d.GuestName)
		metadata = bookingMetadata(d)

	case "guest.created":
		var d GuestData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal guest data: %w", err)
		}
		contentType = "guest"
		title = fmt.Sprintf("Guest: %s (%s)", d.Name, d.Email)
		metadata["guest_email"] = d.Email
		metadata["guest_name"] = d.Name

	case "guest.updated":
		var d GuestData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal guest data: %w", err)
		}
		contentType = "guest"
		title = fmt.Sprintf("Guest updated: %s", d.Name)
		metadata["guest_email"] = d.Email
		metadata["guest_name"] = d.Name

	case "review.received":
		var d ReviewData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal review data: %w", err)
		}
		contentType = "review"
		title = fmt.Sprintf("%s — %s★ review from %s", d.PropertyName, d.Rating, d.GuestName)
		metadata["property_id"] = d.PropertyID
		metadata["property_name"] = d.PropertyName
		metadata["guest_email"] = d.GuestEmail
		metadata["guest_name"] = d.GuestName
		metadata["rating"] = d.Rating

	case "message.received":
		var d MessageData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal message data: %w", err)
		}
		contentType = "guest_message"
		title = fmt.Sprintf("%s — Message from %s", d.PropertyName, d.GuestName)
		metadata["property_id"] = d.PropertyID
		metadata["property_name"] = d.PropertyName
		metadata["guest_email"] = d.GuestEmail
		metadata["guest_name"] = d.GuestName
		metadata["booking_id"] = d.BookingID

	case "task.created":
		var d TaskData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal task data: %w", err)
		}
		contentType = "task"
		title = fmt.Sprintf("%s — Task: %s", d.PropertyName, d.Title)
		metadata["property_id"] = d.PropertyID
		metadata["property_name"] = d.PropertyName
		metadata["category"] = d.Category

	case "task.completed":
		var d TaskData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal task data: %w", err)
		}
		contentType = "task"
		title = fmt.Sprintf("%s — Task completed: %s", d.PropertyName, d.Title)
		metadata["property_id"] = d.PropertyID
		metadata["property_name"] = d.PropertyName
		metadata["category"] = d.Category

	case "expense.created":
		var d ExpenseData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal expense data: %w", err)
		}
		contentType = "financial"
		title = fmt.Sprintf("%s — Expense: %s $%.2f", d.PropertyName, d.Description, d.Amount)
		metadata["property_id"] = d.PropertyID
		metadata["property_name"] = d.PropertyName
		metadata["amount"] = -d.Amount // negative for expense

	case "property.updated":
		var d PropertyData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return connector.RawArtifact{}, fmt.Errorf("unmarshal property data: %w", err)
		}
		contentType = "property"
		title = fmt.Sprintf("Property updated: %s", d.Name)
		metadata["property_id"] = d.ID

	default:
		return connector.RawArtifact{}, fmt.Errorf("unknown event type: %s", event.Type)
	}

	return connector.RawArtifact{
		SourceID:    "guesthost",
		SourceRef:   sourceRef,
		ContentType: contentType,
		Title:       title,
		RawContent:  string(rawContent),
		URL:         "",
		Metadata:    metadata,
		CapturedAt:  capturedAt,
	}, nil
}

// bookingMetadata builds the common metadata map for all booking event types.
func bookingMetadata(d BookingData) map[string]interface{} {
	return map[string]interface{}{
		"property_id":    d.PropertyID,
		"property_name":  d.PropertyName,
		"guest_email":    d.GuestEmail,
		"guest_name":     d.GuestName,
		"checkin_date":   d.CheckIn,
		"checkout_date":  d.CheckOut,
		"booking_source": d.Source,
		"revenue":        d.TotalPrice,
	}
}
