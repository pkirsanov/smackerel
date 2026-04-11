package guesthost

import (
	"encoding/json"
	"testing"
)

func TestNormalizeBookingCreated(t *testing.T) {
	event := ActivityEvent{
		ID:        "evt-b1",
		Type:      "booking.created",
		Timestamp: "2026-04-10T14:30:00Z",
		EntityID:  "b100",
		Data: json.RawMessage(`{
			"propertyId": "p1",
			"propertyName": "Mountain Lodge",
			"guestId": "g1",
			"guestEmail": "alice@example.com",
			"guestName": "Alice Smith",
			"checkIn": "2026-05-01",
			"checkOut": "2026-05-05",
			"source": "direct",
			"status": "confirmed",
			"totalPrice": 1200.50
		}`),
	}

	a, err := NormalizeEvent(event)
	if err != nil {
		t.Fatalf("NormalizeEvent: %v", err)
	}
	if a.SourceID != "guesthost" {
		t.Errorf("SourceID = %q, want guesthost", a.SourceID)
	}
	if a.ContentType != "booking" {
		t.Errorf("ContentType = %q, want booking", a.ContentType)
	}
	if a.Title != "Mountain Lodge — Alice Smith — 2026-05-01-2026-05-05" {
		t.Errorf("Title = %q", a.Title)
	}
	if a.SourceRef != "evt-b1" {
		t.Errorf("SourceRef = %q, want evt-b1 (event ID)", a.SourceRef)
	}

	// Metadata checks
	if a.Metadata["property_id"] != "p1" {
		t.Errorf("metadata property_id = %v", a.Metadata["property_id"])
	}
	if a.Metadata["guest_email"] != "alice@example.com" {
		t.Errorf("metadata guest_email = %v", a.Metadata["guest_email"])
	}
	if a.Metadata["checkin_date"] != "2026-05-01" {
		t.Errorf("metadata checkin_date = %v", a.Metadata["checkin_date"])
	}
	if a.Metadata["checkout_date"] != "2026-05-05" {
		t.Errorf("metadata checkout_date = %v", a.Metadata["checkout_date"])
	}
	if a.Metadata["booking_source"] != "direct" {
		t.Errorf("metadata booking_source = %v", a.Metadata["booking_source"])
	}
	if a.Metadata["revenue"] != 1200.50 {
		t.Errorf("metadata revenue = %v, want 1200.50", a.Metadata["revenue"])
	}
	if a.CapturedAt.IsZero() {
		t.Error("CapturedAt should not be zero")
	}
}

func TestNormalizeReviewReceived(t *testing.T) {
	event := ActivityEvent{
		ID:        "evt-r1",
		Type:      "review.received",
		Timestamp: "2026-04-10T15:00:00Z",
		EntityID:  "r100",
		Data: json.RawMessage(`{
			"propertyId": "p2",
			"propertyName": "Lake House",
			"guestEmail": "bob@example.com",
			"guestName": "Bob Jones",
			"rating": "5",
			"text": "Amazing stay!",
			"hostResponse": "Thank you!"
		}`),
	}

	a, err := NormalizeEvent(event)
	if err != nil {
		t.Fatalf("NormalizeEvent: %v", err)
	}
	if a.ContentType != "review" {
		t.Errorf("ContentType = %q, want review", a.ContentType)
	}
	if a.Title != "Lake House — 5★ review from Bob Jones" {
		t.Errorf("Title = %q", a.Title)
	}
	if a.Metadata["rating"] != "5" {
		t.Errorf("metadata rating = %v, want 5", a.Metadata["rating"])
	}
	if a.Metadata["guest_name"] != "Bob Jones" {
		t.Errorf("metadata guest_name = %v", a.Metadata["guest_name"])
	}
	if a.Metadata["property_name"] != "Lake House" {
		t.Errorf("metadata property_name = %v", a.Metadata["property_name"])
	}
}

func TestNormalizeMessageReceived(t *testing.T) {
	event := ActivityEvent{
		ID:        "evt-m1",
		Type:      "message.received",
		Timestamp: "2026-04-10T16:00:00Z",
		EntityID:  "m100",
		Data: json.RawMessage(`{
			"propertyId": "p3",
			"propertyName": "City Flat",
			"guestEmail": "carol@example.com",
			"guestName": "Carol White",
			"senderRole": "guest",
			"body": "What time is check-in?",
			"bookingId": "b200"
		}`),
	}

	a, err := NormalizeEvent(event)
	if err != nil {
		t.Fatalf("NormalizeEvent: %v", err)
	}
	if a.ContentType != "guest_message" {
		t.Errorf("ContentType = %q, want guest_message", a.ContentType)
	}
	if a.Title != "City Flat — Message from Carol White" {
		t.Errorf("Title = %q", a.Title)
	}
	if a.Metadata["booking_id"] != "b200" {
		t.Errorf("metadata booking_id = %v", a.Metadata["booking_id"])
	}
	if a.Metadata["guest_email"] != "carol@example.com" {
		t.Errorf("metadata guest_email = %v", a.Metadata["guest_email"])
	}
}

func TestNormalizeTaskCreated(t *testing.T) {
	event := ActivityEvent{
		ID:        "evt-t1",
		Type:      "task.created",
		Timestamp: "2026-04-10T17:00:00Z",
		EntityID:  "t100",
		Data: json.RawMessage(`{
			"propertyId": "p4",
			"propertyName": "Beach Villa",
			"title": "Replace HVAC filter",
			"description": "Annual maintenance",
			"status": "pending",
			"category": "maintenance"
		}`),
	}

	a, err := NormalizeEvent(event)
	if err != nil {
		t.Fatalf("NormalizeEvent: %v", err)
	}
	if a.ContentType != "task" {
		t.Errorf("ContentType = %q, want task", a.ContentType)
	}
	if a.Title != "Beach Villa — Task: Replace HVAC filter" {
		t.Errorf("Title = %q", a.Title)
	}
	if a.Metadata["category"] != "maintenance" {
		t.Errorf("metadata category = %v", a.Metadata["category"])
	}
	if a.Metadata["property_name"] != "Beach Villa" {
		t.Errorf("metadata property_name = %v", a.Metadata["property_name"])
	}
}

func TestNormalizeExpenseCreated(t *testing.T) {
	event := ActivityEvent{
		ID:        "evt-x1",
		Type:      "expense.created",
		Timestamp: "2026-04-10T18:00:00Z",
		EntityID:  "x100",
		Data: json.RawMessage(`{
			"propertyId": "p5",
			"propertyName": "Ski Chalet",
			"category": "utilities",
			"description": "Electric bill",
			"amount": 250.75
		}`),
	}

	a, err := NormalizeEvent(event)
	if err != nil {
		t.Fatalf("NormalizeEvent: %v", err)
	}
	if a.ContentType != "financial" {
		t.Errorf("ContentType = %q, want financial", a.ContentType)
	}
	if a.Title != "Ski Chalet — Expense: Electric bill $250.75" {
		t.Errorf("Title = %q", a.Title)
	}
	// Amount should be negated for expenses
	amount, ok := a.Metadata["amount"].(float64)
	if !ok {
		t.Fatalf("metadata amount not float64: %T", a.Metadata["amount"])
	}
	if amount != -250.75 {
		t.Errorf("metadata amount = %v, want -250.75 (negative for expense)", amount)
	}
}

func TestNormalizeAllEventTypes(t *testing.T) {
	cases := []struct {
		eventType   string
		data        string
		wantContent string
	}{
		{"booking.created", `{"propertyId":"p","propertyName":"P","guestName":"G","guestEmail":"g@e","checkIn":"2026-01-01","checkOut":"2026-01-03","source":"direct","totalPrice":100}`, "booking"},
		{"booking.updated", `{"propertyId":"p","propertyName":"P","guestName":"G","guestEmail":"g@e","checkIn":"2026-01-01","checkOut":"2026-01-03","source":"direct","totalPrice":100}`, "booking"},
		{"booking.cancelled", `{"propertyId":"p","propertyName":"P","guestName":"G","guestEmail":"g@e","checkIn":"2026-01-01","checkOut":"2026-01-03","source":"direct","totalPrice":100}`, "booking"},
		{"guest.created", `{"email":"g@e","name":"G"}`, "guest"},
		{"guest.updated", `{"email":"g@e","name":"G"}`, "guest"},
		{"review.received", `{"propertyId":"p","propertyName":"P","guestEmail":"g@e","guestName":"G","rating":"4","text":"ok"}`, "review"},
		{"message.received", `{"propertyId":"p","propertyName":"P","guestEmail":"g@e","guestName":"G","senderRole":"guest","body":"hi","bookingId":"b1"}`, "guest_message"},
		{"task.created", `{"propertyId":"p","propertyName":"P","title":"T","description":"D","status":"open","category":"clean"}`, "task"},
		{"task.completed", `{"propertyId":"p","propertyName":"P","title":"T","description":"D","status":"done","category":"clean"}`, "task"},
		{"expense.created", `{"propertyId":"p","propertyName":"P","category":"supplies","description":"Soap","amount":15.00}`, "financial"},
		{"property.updated", `{"id":"p","name":"P"}`, "property"},
	}

	for _, tc := range cases {
		t.Run(tc.eventType, func(t *testing.T) {
			event := ActivityEvent{
				ID:        "e-" + tc.eventType,
				Type:      tc.eventType,
				Timestamp: "2026-04-01T00:00:00Z",
				EntityID:  "ent-1",
				Data:      json.RawMessage(tc.data),
			}
			a, err := NormalizeEvent(event)
			if err != nil {
				t.Fatalf("NormalizeEvent(%s): %v", tc.eventType, err)
			}
			if a.ContentType != tc.wantContent {
				t.Errorf("ContentType = %q, want %q", a.ContentType, tc.wantContent)
			}
			if a.SourceID != "guesthost" {
				t.Errorf("SourceID = %q, want guesthost", a.SourceID)
			}
			if a.Title == "" {
				t.Error("Title should not be empty")
			}
			if a.RawContent == "" {
				t.Error("RawContent should not be empty")
			}
			if a.CapturedAt.IsZero() {
				t.Error("CapturedAt should not be zero")
			}
		})
	}
}

func TestContentHashConsistency(t *testing.T) {
	event := ActivityEvent{
		ID:        "",
		Type:      "guest.created",
		Timestamp: "2026-04-10T10:00:00Z",
		EntityID:  "g42",
		Data:      json.RawMessage(`{"email":"test@test.com","name":"Test"}`),
	}

	a1, err := NormalizeEvent(event)
	if err != nil {
		t.Fatalf("first normalize: %v", err)
	}
	a2, err := NormalizeEvent(event)
	if err != nil {
		t.Fatalf("second normalize: %v", err)
	}

	if a1.SourceRef != a2.SourceRef {
		t.Errorf("same event should produce same SourceRef: %q vs %q", a1.SourceRef, a2.SourceRef)
	}
	if a1.SourceRef == "" {
		t.Error("SourceRef should not be empty")
	}

	// When ID is set, SourceRef uses the ID directly
	event.ID = "explicit-id"
	a3, err := NormalizeEvent(event)
	if err != nil {
		t.Fatalf("normalize with ID: %v", err)
	}
	if a3.SourceRef != "explicit-id" {
		t.Errorf("SourceRef should be event ID %q, got %q", "explicit-id", a3.SourceRef)
	}
}

func TestNormalizeUnknownEventType(t *testing.T) {
	event := ActivityEvent{
		ID:        "evt-unk",
		Type:      "unknown.type",
		Timestamp: "2026-04-10T10:00:00Z",
		EntityID:  "u1",
		Data:      json.RawMessage(`{}`),
	}

	_, err := NormalizeEvent(event)
	if err == nil {
		t.Fatal("expected error for unknown event type")
	}
}

func TestNormalizeBadTimestamp(t *testing.T) {
	event := ActivityEvent{
		ID:        "evt-bad",
		Type:      "guest.created",
		Timestamp: "not-a-timestamp",
		EntityID:  "g1",
		Data:      json.RawMessage(`{"email":"a@b.com","name":"A"}`),
	}

	_, err := NormalizeEvent(event)
	if err == nil {
		t.Fatal("expected error for bad timestamp")
	}
}
