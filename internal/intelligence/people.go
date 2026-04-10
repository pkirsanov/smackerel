package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// TripDossier is an assembled trip context package per R-405.
type TripDossier struct {
	TripID          string     `json:"trip_id"`
	Destination     string     `json:"destination"`
	DepartureDate   time.Time  `json:"departure_date"`
	ReturnDate      *time.Time `json:"return_date,omitempty"`
	State           string     `json:"state"` // upcoming, active, completed
	FlightArtifacts []string   `json:"flight_artifacts"`
	HotelArtifacts  []string   `json:"hotel_artifacts"`
	PlaceArtifacts  []string   `json:"place_artifacts"`
	RelatedCaptures []string   `json:"related_captures"`
	DossierText     string     `json:"dossier_text"`
	GeneratedAt     time.Time  `json:"generated_at"`
}

// DetectTripsFromEmail scans email artifacts for flight/hotel booking patterns per R-405.
func (e *Engine) DetectTripsFromEmail(ctx context.Context) ([]TripDossier, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("trip detection requires a database connection")
	}

	// Query email artifacts containing booking/flight/hotel keywords
	rows, err := e.Pool.Query(ctx, `
		SELECT a.id, a.title, a.raw_content, a.captured_at,
		       COALESCE(a.metadata->>'sender', '') AS sender
		FROM artifacts a
		WHERE a.source_id IN ('gmail', 'imap', 'outlook')
		AND (
			LOWER(a.title) SIMILAR TO '%(flight|booking|reservation|itinerary|confirmation|hotel|airbnb)%'
			OR LOWER(a.raw_content) SIMILAR TO '%(flight|booking|reservation|itinerary|confirmation|hotel|airbnb)%'
		)
		AND a.created_at > NOW() - INTERVAL '60 days'
		ORDER BY a.captured_at ASC
		LIMIT 50
	`)
	if err != nil {
		return nil, fmt.Errorf("query trip emails: %w", err)
	}
	defer rows.Close()

	// Group by destination (simplified: use title keywords)
	tripMap := make(map[string]*TripDossier)
	for rows.Next() {
		var id, title, content, sender string
		var capturedAt time.Time
		if err := rows.Scan(&id, &title, &content, &capturedAt, &sender); err != nil {
			slog.Warn("trip email scan failed", "error", err)
			continue
		}

		dest := extractDestination(title, content)
		if dest == "" {
			continue
		}

		dossier, exists := tripMap[dest]
		if !exists {
			dossier = &TripDossier{
				Destination:   dest,
				DepartureDate: capturedAt,
				State:         classifyTripState(capturedAt),
				GeneratedAt:   time.Now(),
			}
			tripMap[dest] = dossier
		}

		lower := strings.ToLower(title + " " + content)
		if strings.Contains(lower, "flight") || strings.Contains(lower, "airline") {
			dossier.FlightArtifacts = append(dossier.FlightArtifacts, id)
		} else if strings.Contains(lower, "hotel") || strings.Contains(lower, "airbnb") || strings.Contains(lower, "lodging") {
			dossier.HotelArtifacts = append(dossier.HotelArtifacts, id)
		} else {
			dossier.RelatedCaptures = append(dossier.RelatedCaptures, id)
		}
	}

	var dossiers []TripDossier
	for _, d := range tripMap {
		d.DossierText = assembleDossierText(d)
		dossiers = append(dossiers, *d)
	}

	return dossiers, rows.Err()
}

func extractDestination(title, content string) string {
	// Simple extraction: look for "to <City>" patterns
	lower := strings.ToLower(title + " " + content)
	markers := []string{" to ", "destination: ", "arriving at ", "check-in at "}
	for _, m := range markers {
		idx := strings.Index(lower, m)
		if idx >= 0 {
			rest := strings.TrimSpace(lower[idx+len(m):])
			// Take first word(s) as destination
			words := strings.Fields(rest)
			if len(words) >= 1 {
				tc := cases.Title(language.English)
				dest := tc.String(words[0])
				if len(words) >= 2 && len(words[1]) > 2 {
					dest += " " + tc.String(words[1])
				}
				return dest
			}
		}
	}
	return ""
}

func classifyTripState(departureDate time.Time) string {
	now := time.Now()
	if departureDate.After(now) {
		return "upcoming"
	}
	if departureDate.After(now.AddDate(0, 0, -14)) {
		return "active"
	}
	return "completed"
}

func assembleDossierText(d *TripDossier) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("Trip to %s (%s)", d.Destination, d.State))
	if len(d.FlightArtifacts) > 0 {
		parts = append(parts, fmt.Sprintf("✈ %d flight booking(s)", len(d.FlightArtifacts)))
	}
	if len(d.HotelArtifacts) > 0 {
		parts = append(parts, fmt.Sprintf("🏨 %d lodging booking(s)", len(d.HotelArtifacts)))
	}
	if len(d.RelatedCaptures) > 0 {
		parts = append(parts, fmt.Sprintf("📎 %d related capture(s)", len(d.RelatedCaptures)))
	}
	return strings.Join(parts, "\n")
}

// PersonProfile is an assembled person intelligence profile per R-406.
type PersonProfile struct {
	PersonID          string    `json:"person_id"`
	Name              string    `json:"name"`
	Email             string    `json:"email"`
	TotalInteractions int       `json:"total_interactions"`
	LastInteraction   time.Time `json:"last_interaction"`
	SharedTopics      []string  `json:"shared_topics"`
	PendingItems      []string  `json:"pending_action_items"`
	InteractionTrend  string    `json:"interaction_trend"` // warming, stable, cooling
	DaysSinceContact  int       `json:"days_since_contact"`
}

// GetPeopleIntelligence returns profiles with interaction analysis per R-406.
func (e *Engine) GetPeopleIntelligence(ctx context.Context) ([]PersonProfile, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("people intelligence requires a database connection")
	}

	rows, err := e.Pool.Query(ctx, `
		SELECT p.id, p.name, COALESCE(p.email, '') AS email,
		       COUNT(DISTINCT e.src_id) AS interaction_count,
		       MAX(a.created_at) AS last_interaction,
		       EXTRACT(DAY FROM NOW() - MAX(a.created_at))::int AS days_since
		FROM people p
		JOIN edges e ON e.dst_id = p.id AND e.dst_type = 'person'
		JOIN artifacts a ON a.id = e.src_id
		GROUP BY p.id, p.name, p.email
		ORDER BY interaction_count DESC
		LIMIT 50
	`)
	if err != nil {
		return nil, fmt.Errorf("query people profiles: %w", err)
	}
	defer rows.Close()

	var profiles []PersonProfile
	for rows.Next() {
		var pp PersonProfile
		if err := rows.Scan(&pp.PersonID, &pp.Name, &pp.Email,
			&pp.TotalInteractions, &pp.LastInteraction, &pp.DaysSinceContact); err != nil {
			slog.Warn("people profile scan failed", "error", err)
			continue
		}

		// Interaction trend: cooling if >42 days since last contact
		pp.InteractionTrend = classifyInteractionTrend(pp.DaysSinceContact, pp.TotalInteractions)

		// Shared topics
		topicRows, err := e.Pool.Query(ctx, `
			SELECT DISTINCT t.name FROM topics t
			JOIN edges e1 ON e1.dst_id = t.id AND e1.dst_type = 'topic' AND e1.edge_type = 'BELONGS_TO'
			JOIN edges e2 ON e2.src_id = e1.src_id AND e2.dst_id = $1 AND e2.dst_type = 'person'
			LIMIT 5
		`, pp.PersonID)
		if err == nil {
			for topicRows.Next() {
				var t string
				if topicRows.Scan(&t) == nil {
					pp.SharedTopics = append(pp.SharedTopics, t)
				}
			}
			topicRows.Close()
		}

		// Pending action items
		aiRows, err := e.Pool.Query(ctx, `
			SELECT text FROM action_items WHERE person_id = $1 AND status = 'open' LIMIT 3
		`, pp.PersonID)
		if err == nil {
			for aiRows.Next() {
				var t string
				if aiRows.Scan(&t) == nil {
					pp.PendingItems = append(pp.PendingItems, t)
				}
			}
			aiRows.Close()
		}

		profiles = append(profiles, pp)
	}

	return profiles, rows.Err()
}

// classifyInteractionTrend determines if a relationship is warming, stable, or cooling.
func classifyInteractionTrend(daysSince, totalInteractions int) string {
	if daysSince > 42 {
		return "cooling"
	}
	if daysSince > 21 && totalInteractions < 5 {
		return "cooling"
	}
	if daysSince < 7 {
		return "warming"
	}
	return "stable"
}
