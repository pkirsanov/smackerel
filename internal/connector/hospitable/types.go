package hospitable

import "time"

// Property represents a Hospitable property listing.
type Property struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Address     Address   `json:"address"`
	Bedrooms    int       `json:"bedrooms"`
	Bathrooms   int       `json:"bathrooms"`
	MaxGuests   int       `json:"max_guests"`
	Amenities   []string  `json:"amenities"`
	ListingURLs []string  `json:"listing_urls"`
	ChannelIDs  []string  `json:"channel_ids"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Address represents a property's physical address.
type Address struct {
	Street  string `json:"street"`
	City    string `json:"city"`
	State   string `json:"state"`
	Country string `json:"country"`
	Zip     string `json:"zip"`
}

// Reservation represents a Hospitable reservation/booking.
type Reservation struct {
	ID          string    `json:"id"`
	PropertyID  string    `json:"property_id"`
	Channel     string    `json:"channel"`
	Status      string    `json:"status"`
	CheckIn     string    `json:"check_in"`
	CheckOut    string    `json:"check_out"`
	GuestName   string    `json:"guest_name"`
	GuestCount  int       `json:"guest_count"`
	NightlyRate float64   `json:"nightly_rate"`
	TotalPayout float64   `json:"total_payout"`
	Nights      int       `json:"nights"`
	BookedAt    time.Time `json:"booked_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Message represents a guest/host message in a conversation.
type Message struct {
	ID            string    `json:"id"`
	ReservationID string    `json:"reservation_id"`
	Sender        string    `json:"sender"`
	Body          string    `json:"body"`
	IsAutomated   bool      `json:"is_automated"`
	SenderRole    string    `json:"sender_role"` // R-019: "guest", "host", or "automated"
	SentAt        time.Time `json:"sent_at"`
}

// Review represents a guest review.
type Review struct {
	ID            string    `json:"id"`
	ReservationID string    `json:"reservation_id"`
	PropertyID    string    `json:"property_id"`
	Rating        float64   `json:"rating"`
	ReviewText    string    `json:"review_text"`
	HostResponse  string    `json:"host_response"`
	Channel       string    `json:"channel"`
	SubmittedAt   time.Time `json:"submitted_at"`
}

// PaginatedResponse wraps paginated API responses.
type PaginatedResponse[T any] struct {
	Data    []T    `json:"data"`
	NextURL string `json:"next"`
	Total   int    `json:"total"`
}

// SyncCursor stores per-resource-type sync timestamps.
type SyncCursor struct {
	Properties           time.Time         `json:"properties"`
	Reservations         time.Time         `json:"reservations"`
	Messages             time.Time         `json:"messages"`
	Reviews              time.Time         `json:"reviews"`
	PropertyNames        map[string]string `json:"property_names,omitempty"`
	ActiveReservationIDs []string          `json:"active_reservation_ids,omitempty"`
}
