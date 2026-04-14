package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/db"
)

// ContextRequest is the JSON body for POST /api/context-for.
type ContextRequest struct {
	EntityType string   `json:"entityType"` // "guest", "property", "booking"
	EntityID   string   `json:"entityId"`
	Source     string   `json:"source,omitempty"` // optional: scope lookups to a specific connector source (e.g. "guesthost")
	Include    []string `json:"include"`          // ["artifacts", "hints", "alerts", "sentiment"]
}

// ContextResponse is the JSON response for POST /api/context-for.
type ContextResponse struct {
	EntityType string              `json:"entityType"`
	EntityID   string              `json:"entityId"`
	Guest      *GuestContext       `json:"guest,omitempty"`
	Property   *PropertyContext    `json:"property,omitempty"`
	Booking    *BookingContext     `json:"booking,omitempty"`
	Hints      []CommunicationHint `json:"hints,omitempty"`
	Alerts     []Alert             `json:"alerts,omitempty"`
}

// GuestContext is the guest entity context.
type GuestContext struct {
	Name            string            `json:"name"`
	Email           string            `json:"email"`
	TotalStays      int               `json:"totalStays"`
	TotalSpend      float64           `json:"totalSpend"`
	AvgRating       *float64          `json:"avgRating,omitempty"`
	SentimentScore  *float64          `json:"sentimentScore,omitempty"`
	FirstStay       *string           `json:"firstStay,omitempty"`
	LastStay        *string           `json:"lastStay,omitempty"`
	RecentArtifacts []ArtifactSummary `json:"recentArtifacts,omitempty"`
}

// PropertyContext is the property entity context.
type PropertyContext struct {
	Name            string            `json:"name"`
	ExternalID      string            `json:"externalId"`
	TotalBookings   int               `json:"totalBookings"`
	TotalRevenue    float64           `json:"totalRevenue"`
	AvgRating       *float64          `json:"avgRating,omitempty"`
	IssueCount      int               `json:"issueCount"`
	Topics          []string          `json:"topics,omitempty"`
	RecentArtifacts []ArtifactSummary `json:"recentArtifacts,omitempty"`
}

// BookingContext is the booking entity context.
type BookingContext struct {
	GuestName    string  `json:"guestName"`
	PropertyName string  `json:"propertyName"`
	CheckIn      string  `json:"checkIn"`
	CheckOut     string  `json:"checkOut"`
	Source       string  `json:"source"`
	Status       string  `json:"status"`
	TotalPrice   float64 `json:"totalPrice"`
}

// ArtifactSummary is a brief summary of a captured artifact.
type ArtifactSummary struct {
	ID          string `json:"id"`
	ContentType string `json:"contentType"`
	Title       string `json:"title"`
	CapturedAt  string `json:"capturedAt"`
}

// CommunicationHint is a rule-based hint for host communication.
type CommunicationHint struct {
	HintType    string `json:"hintType"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
}

// Alert is a contextual alert about an entity.
type Alert struct {
	AlertType   string `json:"alertType"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

// ContextHandler handles POST /api/context-for — enriched context lookups.
type ContextHandler struct {
	guestRepo    *db.GuestRepository
	propertyRepo *db.PropertyRepository
	pool         *pgxpool.Pool
}

// NewContextHandler creates a new ContextHandler.
func NewContextHandler(guestRepo *db.GuestRepository, propertyRepo *db.PropertyRepository, pool *pgxpool.Pool) *ContextHandler {
	return &ContextHandler{
		guestRepo:    guestRepo,
		propertyRepo: propertyRepo,
		pool:         pool,
	}
}

// maxEntityIDLen limits entity ID length to prevent abuse.
const maxEntityIDLen = 512

// HandleContextFor handles POST /api/context-for.
func (h *ContextHandler) HandleContextFor(w http.ResponseWriter, r *http.Request) {
	var req ContextRequest
	if !decodeJSONBody(w, r, &req, "INVALID_REQUEST", "Invalid JSON request body") {
		return
	}

	if req.EntityID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "entityId is required")
		return
	}

	if len(req.EntityID) > maxEntityIDLen {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "entityId exceeds maximum length")
		return
	}

	includeSet := make(map[string]bool, len(req.Include))
	for _, inc := range req.Include {
		includeSet[inc] = true
	}
	// If include is empty, include everything
	includeAll := len(req.Include) == 0

	ctx := r.Context()
	resp := ContextResponse{
		EntityType: req.EntityType,
		EntityID:   req.EntityID,
	}

	switch req.EntityType {
	case "guest":
		if err := h.buildGuestContext(ctx, &resp, req.EntityID, req.Source, includeSet, includeAll); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Guest not found")
				return
			}
			slog.Error("context-for guest lookup failed", "error", err, "entityId", req.EntityID)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to build guest context")
			return
		}

	case "property":
		if err := h.buildPropertyContext(ctx, &resp, req.EntityID, req.Source, includeSet, includeAll); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Property not found")
				return
			}
			slog.Error("context-for property lookup failed", "error", err, "entityId", req.EntityID)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to build property context")
			return
		}

	case "booking":
		if err := h.buildBookingContext(ctx, &resp, req.EntityID, includeSet, includeAll); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "Booking not found")
				return
			}
			slog.Error("context-for booking lookup failed", "error", err, "entityId", req.EntityID)
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to build booking context")
			return
		}

	default:
		writeError(w, http.StatusBadRequest, "INVALID_ENTITY_TYPE",
			"entityType must be one of: guest, property, booking")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *ContextHandler) buildGuestContext(ctx context.Context, resp *ContextResponse, email string, source string, includeSet map[string]bool, includeAll bool) error {
	guest, err := h.guestRepo.FindByEmail(ctx, email, source)
	if err != nil {
		return err
	}

	gc := &GuestContext{
		Name:       guest.Name,
		Email:      guest.Email,
		TotalStays: guest.TotalStays,
		TotalSpend: guest.TotalSpend,
		AvgRating:  guest.AvgRating,
	}

	if includeAll || includeSet["sentiment"] {
		gc.SentimentScore = guest.SentimentScore
	}

	if guest.FirstStayAt != nil {
		s := guest.FirstStayAt.Format(time.RFC3339)
		gc.FirstStay = &s
	}
	if guest.LastStayAt != nil {
		s := guest.LastStayAt.Format(time.RFC3339)
		gc.LastStay = &s
	}

	if includeAll || includeSet["artifacts"] {
		artifacts, err := h.recentArtifactsForEntity(ctx, "guest", guest.ID)
		if err != nil {
			slog.Warn("context-for: failed to fetch guest artifacts", "error", err, "guestId", guest.ID)
		} else {
			gc.RecentArtifacts = artifacts
		}
	}

	resp.Guest = gc

	if includeAll || includeSet["hints"] {
		resp.Hints = h.generateGuestHints(guest)
	}

	if includeAll || includeSet["alerts"] {
		resp.Alerts = h.generateGuestAlerts(guest)
	}

	return nil
}

func (h *ContextHandler) buildPropertyContext(ctx context.Context, resp *ContextResponse, externalID string, source string, includeSet map[string]bool, includeAll bool) error {
	property, err := h.propertyRepo.FindByExternalID(ctx, externalID, source)
	if err != nil {
		return err
	}

	pc := &PropertyContext{
		Name:          property.Name,
		ExternalID:    property.ExternalID,
		TotalBookings: property.TotalBookings,
		TotalRevenue:  property.TotalRevenue,
		AvgRating:     property.AvgRating,
		IssueCount:    property.IssueCount,
		Topics:        property.Topics,
	}

	if includeAll || includeSet["artifacts"] {
		artifacts, err := h.recentArtifactsForEntity(ctx, "property", property.ID)
		if err != nil {
			slog.Warn("context-for: failed to fetch property artifacts", "error", err, "propertyId", property.ID)
		} else {
			pc.RecentArtifacts = artifacts
		}
	}

	resp.Property = pc

	if includeAll || includeSet["hints"] {
		resp.Hints = h.generatePropertyHints(property)
	}

	if includeAll || includeSet["alerts"] {
		resp.Alerts = h.generatePropertyAlerts(property)
	}

	return nil
}

func (h *ContextHandler) buildBookingContext(ctx context.Context, resp *ContextResponse, bookingID string, includeSet map[string]bool, includeAll bool) error {
	// Look up booking artifact by matching artifact_type='booking' and content_raw containing the booking ID
	var contentRaw string
	err := h.pool.QueryRow(ctx, `
		SELECT COALESCE(content_raw, '')
		FROM artifacts
		WHERE artifact_type = 'booking'
		  AND source_id = 'guesthost'
		  AND content_raw::jsonb @> jsonb_build_object('bookingId', $1)
		ORDER BY created_at DESC
		LIMIT 1
	`, bookingID).Scan(&contentRaw)
	if err != nil {
		return err
	}

	var meta struct {
		GuestName    string  `json:"guestName"`
		GuestEmail   string  `json:"guestEmail"`
		PropertyName string  `json:"propertyName"`
		PropertyID   string  `json:"propertyId"`
		CheckIn      string  `json:"checkIn"`
		CheckOut     string  `json:"checkOut"`
		Source       string  `json:"source"`
		Status       string  `json:"status"`
		TotalPrice   float64 `json:"totalPrice"`
		BookingID    string  `json:"bookingId"`
	}
	if err := json.Unmarshal([]byte(contentRaw), &meta); err != nil {
		return err
	}

	resp.Booking = &BookingContext{
		GuestName:    meta.GuestName,
		PropertyName: meta.PropertyName,
		CheckIn:      meta.CheckIn,
		CheckOut:     meta.CheckOut,
		Source:       meta.Source,
		Status:       meta.Status,
		TotalPrice:   meta.TotalPrice,
	}

	// Also build guest and property context if available
	if meta.GuestEmail != "" {
		guest, err := h.guestRepo.FindByEmail(ctx, meta.GuestEmail)
		if err == nil {
			resp.Guest = &GuestContext{
				Name:       guest.Name,
				Email:      guest.Email,
				TotalStays: guest.TotalStays,
				TotalSpend: guest.TotalSpend,
				AvgRating:  guest.AvgRating,
			}
			if includeAll || includeSet["hints"] {
				resp.Hints = append(resp.Hints, h.generateGuestHints(guest)...)
			}
			if includeAll || includeSet["alerts"] {
				resp.Alerts = append(resp.Alerts, h.generateGuestAlerts(guest)...)
			}
		}
	}

	if meta.PropertyID != "" {
		property, err := h.propertyRepo.FindByExternalID(ctx, meta.PropertyID)
		if err == nil {
			resp.Property = &PropertyContext{
				Name:          property.Name,
				ExternalID:    property.ExternalID,
				TotalBookings: property.TotalBookings,
				TotalRevenue:  property.TotalRevenue,
				AvgRating:     property.AvgRating,
				IssueCount:    property.IssueCount,
				Topics:        property.Topics,
			}
			if includeAll || includeSet["hints"] {
				resp.Hints = append(resp.Hints, h.generatePropertyHints(property)...)
			}
			if includeAll || includeSet["alerts"] {
				resp.Alerts = append(resp.Alerts, h.generatePropertyAlerts(property)...)
			}
		}
	}

	return nil
}

// recentArtifactsForEntity fetches recent artifacts linked to an entity via the edges table.
func (h *ContextHandler) recentArtifactsForEntity(ctx context.Context, entityType, entityID string) ([]ArtifactSummary, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT a.id, a.artifact_type, a.title, a.created_at
		FROM artifacts a
		JOIN edges e ON e.src_type = 'artifact' AND e.src_id = a.id
		WHERE e.dst_type = $1 AND e.dst_id = $2
		ORDER BY a.created_at DESC
		LIMIT 10
	`, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []ArtifactSummary
	for rows.Next() {
		var id, artType, title string
		var createdAt time.Time
		if err := rows.Scan(&id, &artType, &title, &createdAt); err != nil {
			continue
		}
		summaries = append(summaries, ArtifactSummary{
			ID:          id,
			ContentType: artType,
			Title:       title,
			CapturedAt:  createdAt.Format(time.RFC3339),
		})
	}
	return summaries, rows.Err()
}

// generateGuestHints produces rule-based communication hints for a guest.
func (h *ContextHandler) generateGuestHints(guest *db.GuestNode) []CommunicationHint {
	var hints []CommunicationHint

	if guest.TotalStays > 1 {
		hints = append(hints, CommunicationHint{
			HintType:    "repeat_guest",
			Description: "Returning guest with " + strconv.Itoa(guest.TotalStays) + " previous stays",
			Priority:    "medium",
		})
	}

	if guest.TotalSpend > 5000 {
		hints = append(hints, CommunicationHint{
			HintType:    "vip",
			Description: "High-value guest",
			Priority:    "high",
		})
	}

	if guest.AvgRating != nil && *guest.AvgRating >= 4 {
		hints = append(hints, CommunicationHint{
			HintType:    "positive_reviewer",
			Description: "Guest tends to leave positive reviews",
			Priority:    "low",
		})
	}

	return hints
}

// generatePropertyHints produces rule-based communication hints for a property.
func (h *ContextHandler) generatePropertyHints(property *db.PropertyNode) []CommunicationHint {
	var hints []CommunicationHint

	if property.IssueCount > 3 {
		hints = append(hints, CommunicationHint{
			HintType:    "issue_history",
			Description: "Property has pending maintenance issues",
			Priority:    "high",
		})
	}

	return hints
}

// generateGuestAlerts produces alerts for a guest.
func (h *ContextHandler) generateGuestAlerts(guest *db.GuestNode) []Alert {
	var alerts []Alert

	if guest.SentimentScore != nil && *guest.SentimentScore < 0.3 {
		alerts = append(alerts, Alert{
			AlertType:   "low_sentiment",
			Description: "Guest sentiment trending negative",
			Severity:    "warning",
		})
	}

	return alerts
}

// generatePropertyAlerts produces alerts for a property.
func (h *ContextHandler) generatePropertyAlerts(property *db.PropertyNode) []Alert {
	var alerts []Alert

	if property.IssueCount > 3 {
		alerts = append(alerts, Alert{
			AlertType:   "high_issue_count",
			Description: "Property maintenance attention needed",
			Severity:    "warning",
		})
	}

	return alerts
}
