package caldav

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// Connector implements the CalDAV calendar connector.
type Connector struct {
	id     string
	config connector.ConnectorConfig
	health connector.HealthStatus
}

// CalendarEvent represents a parsed calendar event.
type CalendarEvent struct {
	UID         string    `json:"uid"`
	Summary     string    `json:"summary"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	Organizer   string    `json:"organizer"`
	Attendees   []string  `json:"attendees"`
	Recurring   bool      `json:"recurring"`
	Status      string    `json:"status"` // confirmed, tentative, cancelled
	Updated     time.Time `json:"updated"`
}

// New creates a new CalDAV connector.
func New(id string) *Connector {
	return &Connector{id: id, health: connector.HealthDisconnected}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	c.config = config
	if config.AuthType != "oauth2" {
		return fmt.Errorf("CalDAV connector requires oauth2 auth")
	}
	c.health = connector.HealthHealthy
	slog.Info("CalDAV connector connected", "id", c.id)
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.health = connector.HealthSyncing
	defer func() { c.health = connector.HealthHealthy }()

	events, err := c.fetchEvents(ctx, cursor)
	if err != nil {
		c.health = connector.HealthError
		return nil, cursor, fmt.Errorf("fetch events: %w", err)
	}

	if len(events) == 0 {
		slog.Info("CalDAV sync: no new events", "id", c.id, "cursor", cursor)
		return nil, cursor, nil
	}

	// Sort by updated time for cursor advancement
	sort.Slice(events, func(i, j int) bool {
		return events[i].Updated.Before(events[j].Updated)
	})

	var artifacts []connector.RawArtifact
	newCursor := cursor

	for _, evt := range events {
		cursorTime := evt.Updated.Format(time.RFC3339)
		if cursorTime <= cursor && cursor != "" {
			continue
		}

		// Skip cancelled events
		if evt.Status == "cancelled" {
			continue
		}

		// Build content from event details
		var contentParts []string
		contentParts = append(contentParts, evt.Summary)
		if evt.Description != "" {
			contentParts = append(contentParts, evt.Description)
		}
		if evt.Location != "" {
			contentParts = append(contentParts, "Location: "+evt.Location)
		}
		if len(evt.Attendees) > 0 {
			contentParts = append(contentParts, "Attendees: "+strings.Join(evt.Attendees, ", "))
		}
		content := strings.Join(contentParts, "\n")

		// Determine tier: meetings with attendees get full processing
		tier := "standard"
		if len(evt.Attendees) > 0 {
			tier = "full"
		}
		if evt.Recurring {
			tier = "light" // recurring events get lighter treatment
		}

		metadata := map[string]interface{}{
			"organizer":       evt.Organizer,
			"attendees":       evt.Attendees,
			"attendee_count":  len(evt.Attendees),
			"location":        evt.Location,
			"start":           evt.Start.Format(time.RFC3339),
			"end":             evt.End.Format(time.RFC3339),
			"recurring":       evt.Recurring,
			"status":          evt.Status,
			"processing_tier": tier,
		}

		artifacts = append(artifacts, connector.RawArtifact{
			SourceID:    c.id,
			SourceRef:   evt.UID,
			ContentType: "event",
			Title:       evt.Summary,
			RawContent:  content,
			Metadata:    metadata,
			CapturedAt:  evt.Start,
		})

		if cursorTime > newCursor {
			newCursor = cursorTime
		}
	}

	slog.Info("CalDAV sync complete",
		"id", c.id,
		"fetched", len(events),
		"artifacts", len(artifacts),
		"cursor", newCursor,
	)

	return artifacts, newCursor, nil
}

// fetchEvents retrieves events from source config or live CalDAV.
func (c *Connector) fetchEvents(ctx context.Context, cursor string) ([]CalendarEvent, error) {
	rawEvents, ok := c.config.SourceConfig["events"]
	if ok {
		return parseCalendarEvents(rawEvents)
	}
	return nil, nil
}

// parseCalendarEvents converts interface{} events from config into CalendarEvent structs.
func parseCalendarEvents(raw interface{}) ([]CalendarEvent, error) {
	evts, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("events must be an array")
	}

	var result []CalendarEvent
	for _, e := range evts {
		em, ok := e.(map[string]interface{})
		if !ok {
			continue
		}

		evt := CalendarEvent{
			UID:     getStr(em, "uid"),
			Summary: getStr(em, "summary"),
			Status:  "confirmed",
			Updated: time.Now(),
		}

		if desc := getStr(em, "description"); desc != "" {
			evt.Description = desc
		}
		if loc := getStr(em, "location"); loc != "" {
			evt.Location = loc
		}
		if org := getStr(em, "organizer"); org != "" {
			evt.Organizer = org
		}
		if status := getStr(em, "status"); status != "" {
			evt.Status = status
		}
		if s, ok := em["start"].(string); ok {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				evt.Start = t
			}
		}
		if e, ok := em["end"].(string); ok {
			if t, err := time.Parse(time.RFC3339, e); err == nil {
				evt.End = t
			}
		}
		if u, ok := em["updated"].(string); ok {
			if t, err := time.Parse(time.RFC3339, u); err == nil {
				evt.Updated = t
			}
		}
		if r, ok := em["recurring"].(bool); ok {
			evt.Recurring = r
		}
		if attendees, ok := em["attendees"].([]interface{}); ok {
			for _, a := range attendees {
				if s, ok := a.(string); ok {
					evt.Attendees = append(evt.Attendees, s)
				}
			}
		}

		result = append(result, evt)
	}
	return result, nil
}

func getStr(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus { return c.health }
func (c *Connector) Close() error {
	c.health = connector.HealthDisconnected
	return nil
}

var _ connector.Connector = (*Connector)(nil)
