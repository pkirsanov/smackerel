package guesthost

import "encoding/json"

// ActivityEvent represents a single event from the GuestHost activity feed.
type ActivityEvent struct {
	ID         string          `json:"id"`
	Type       string          `json:"eventType"`
	Timestamp  string          `json:"createdAt"`
	EntityID   string          `json:"entityId"`
	EntityType string          `json:"entityType"`
	Data       json.RawMessage `json:"data"`
}

// ActivityFeedResponse is the paginated response from the GuestHost activity feed endpoint.
type ActivityFeedResponse struct {
	Events  []ActivityEvent `json:"events"`
	Cursor  string          `json:"cursor"`
	HasMore bool            `json:"hasMore"`
}

// BookingData is the typed payload for booking-related activity events.
type BookingData struct {
	PropertyID   string  `json:"propertyId"`
	PropertyName string  `json:"propertyName"`
	GuestID      string  `json:"guestId"`
	GuestEmail   string  `json:"guestEmail"`
	GuestName    string  `json:"guestName"`
	CheckIn      string  `json:"checkIn"`
	CheckOut     string  `json:"checkOut"`
	Source       string  `json:"source"`
	Status       string  `json:"status"`
	TotalPrice   float64 `json:"totalPrice"`
}

// ReviewData is the typed payload for review-related activity events.
type ReviewData struct {
	PropertyID   string `json:"propertyId"`
	PropertyName string `json:"propertyName"`
	GuestEmail   string `json:"guestEmail"`
	GuestName    string `json:"guestName"`
	Rating       string `json:"rating"`
	Text         string `json:"text"`
	HostResponse string `json:"hostResponse"`
}

// MessageData is the typed payload for message-related activity events.
type MessageData struct {
	PropertyID   string `json:"propertyId"`
	PropertyName string `json:"propertyName"`
	GuestEmail   string `json:"guestEmail"`
	GuestName    string `json:"guestName"`
	SenderRole   string `json:"senderRole"`
	Body         string `json:"body"`
	BookingID    string `json:"bookingId"`
}

// TaskData is the typed payload for task-related activity events.
type TaskData struct {
	PropertyID   string `json:"propertyId"`
	PropertyName string `json:"propertyName"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	Category     string `json:"category"`
}

// ExpenseData is the typed payload for expense-related activity events.
type ExpenseData struct {
	PropertyID   string  `json:"propertyId"`
	PropertyName string  `json:"propertyName"`
	Category     string  `json:"category"`
	Description  string  `json:"description"`
	Amount       float64 `json:"amount"`
}

// GuestData is the typed payload for guest-related activity events.
type GuestData struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// PropertyData is the typed payload for property-related activity events.
type PropertyData struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
