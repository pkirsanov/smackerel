package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
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
	// DestinationForecast carries fresh weather/forecast artifacts for the
	// destination when available. Restored per BUG-016-W2 — see
	// specs/016-weather-connector/bugs/BUG-016-W2-dossier-no-forecast/.
	DestinationForecast *DossierForecast `json:"destination_forecast,omitempty"`
}

// DetectTripsFromEmail scans email artifacts for flight/hotel booking patterns per R-405.
func (e *Engine) DetectTripsFromEmail(ctx context.Context) ([]TripDossier, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("trip detection requires a database connection")
	}

	// Query email artifacts containing booking/flight/hotel keywords
	rows, err := e.Pool.Query(ctx, `
		SELECT a.id, a.title, a.raw_content, a.created_at,
		       COALESCE(a.metadata->>'sender', '') AS sender
		FROM artifacts a
		WHERE a.source_id IN ('gmail', 'imap', 'outlook')
		AND (
			LOWER(a.title) SIMILAR TO '%(flight|booking|reservation|itinerary|confirmation|hotel|airbnb)%'
			OR LOWER(a.raw_content) SIMILAR TO '%(flight|booking|reservation|itinerary|confirmation|hotel|airbnb)%'
		)
		AND a.created_at > NOW() - INTERVAL '60 days'
		ORDER BY a.created_at ASC
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

	// Sort for deterministic output — map iteration order is non-deterministic.
	sort.Slice(dossiers, func(i, j int) bool {
		return dossiers[i].Destination < dossiers[j].Destination
	})

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
	// BUG-016-W2: include destination forecast line when available.
	if line := formatDossierForecastLine(d.DestinationForecast); line != "" {
		parts = append(parts, line)
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
		profiles = append(profiles, pp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("people profile row iteration: %w", err)
	}

	if len(profiles) == 0 {
		return profiles, nil
	}

	// Batch-fetch shared topics for all people in a single query
	personIDs := make([]string, len(profiles))
	personIdx := make(map[string]int, len(profiles))
	for i, pp := range profiles {
		personIDs[i] = pp.PersonID
		personIdx[pp.PersonID] = i
	}

	topicRows, err := e.Pool.Query(ctx, `
		SELECT e2.dst_id AS person_id, t.name
		FROM edges e1
		JOIN topics t ON t.id = e1.dst_id AND e1.dst_type = 'topic' AND e1.edge_type = 'BELONGS_TO'
		JOIN edges e2 ON e2.src_id = e1.src_id AND e2.dst_type = 'person'
		WHERE e2.dst_id = ANY($1)
		GROUP BY e2.dst_id, t.name
		ORDER BY e2.dst_id, COUNT(*) DESC
	`, personIDs)
	if err != nil {
		slog.Warn("batch shared topics query failed", "error", err)
	} else {
		defer topicRows.Close()
		topicCount := make(map[string]int)
		for topicRows.Next() {
			var personID, topicName string
			if topicRows.Scan(&personID, &topicName) == nil {
				key := personID
				if topicCount[key] < 5 {
					if idx, ok := personIdx[personID]; ok {
						profiles[idx].SharedTopics = append(profiles[idx].SharedTopics, topicName)
					}
					topicCount[key]++
				}
			}
		}
		if err := topicRows.Err(); err != nil {
			slog.Warn("batch shared topics iteration failed", "error", err)
		}
	}

	// Batch-fetch pending action items for all people in a single query
	aiRows, err := e.Pool.Query(ctx, `
		SELECT person_id, text
		FROM (
			SELECT person_id, text,
			       ROW_NUMBER() OVER (PARTITION BY person_id ORDER BY created_at DESC) AS rn
			FROM action_items
			WHERE person_id = ANY($1) AND status = 'open'
		) ranked
		WHERE rn <= 3
	`, personIDs)
	if err != nil {
		slog.Warn("batch action items query failed", "error", err)
	} else {
		defer aiRows.Close()
		for aiRows.Next() {
			var personID, text string
			if aiRows.Scan(&personID, &text) == nil {
				if idx, ok := personIdx[personID]; ok {
					profiles[idx].PendingItems = append(profiles[idx].PendingItems, text)
				}
			}
		}
		if err := aiRows.Err(); err != nil {
			slog.Warn("batch action items iteration failed", "error", err)
		}
	}

	return profiles, nil
}

// classifyInteractionTrend determines the relationship trend using a 4-tier model
// aligned with R-405 design: increasing, stable, decreasing, lapsed.
// Uses daysSince and totalInteractions as a proxy for the designed ratio-based
// calculation (current_month / avg_monthly) until per-month counters are available.
// GAP-005-F3: Aligned with design's 4-tier trend model.
func classifyInteractionTrend(daysSince, totalInteractions int) string {
	if daysSince > 42 && totalInteractions < 3 {
		return "lapsed"
	}
	if daysSince > 42 {
		return "decreasing"
	}
	if daysSince > 21 && totalInteractions < 5 {
		return "decreasing"
	}
	if daysSince < 7 && totalInteractions > 10 {
		return "increasing"
	}
	if daysSince < 7 {
		return "stable"
	}
	return "stable"
}
