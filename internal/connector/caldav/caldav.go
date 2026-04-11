package caldav

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
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

		// Determine tier: meetings with attendees get full processing,
		// recurring events without attendees get lighter treatment.
		tier := "standard"
		if len(evt.Attendees) > 0 {
			tier = "full"
		} else if evt.Recurring {
			tier = "light"
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

// fetchEvents retrieves events from source config (for testing/local)
// or from the Google Calendar REST API when OAuth credentials are present.
func (c *Connector) fetchEvents(ctx context.Context, cursor string) ([]CalendarEvent, error) {
	rawEvents, ok := c.config.SourceConfig["events"]
	if ok {
		return parseCalendarEvents(rawEvents)
	}

	// Check for OAuth access token (live API path)
	accessToken := getCredential(c.config.Credentials, "access_token")
	if accessToken == "" {
		slog.Debug("CalDAV: no source_config events and no access_token", "id", c.id)
		return nil, nil
	}

	return c.fetchGoogleCalendarEvents(ctx, accessToken, cursor)
}

// fetchGoogleCalendarEvents fetches events from the Google Calendar REST API v3.
func (c *Connector) fetchGoogleCalendarEvents(ctx context.Context, token string, cursor string) ([]CalendarEvent, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	// Build request for primary calendar events
	params := url.Values{
		"maxResults":   {"100"},
		"singleEvents": {"true"},
		"orderBy":      {"startTime"},
	}

	// Use cursor as sync token or time-based filter
	if cursor != "" {
		if t, err := time.Parse(time.RFC3339, cursor); err == nil {
			params.Set("timeMin", t.Format(time.RFC3339))
		} else {
			params.Set("syncToken", cursor)
		}
	} else {
		// Default: events from the past 30 days + future 14 days (R-204)
		params.Set("timeMin", time.Now().AddDate(0, 0, -30).Format(time.RFC3339))
		params.Set("timeMax", time.Now().AddDate(0, 0, 14).Format(time.RFC3339))
	}

	apiURL := "https://www.googleapis.com/calendar/v3/calendars/primary/events?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create calendar request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calendar API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("calendar API: token expired or invalid (401)")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("calendar API: HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Limit response body to 10MB to prevent resource exhaustion
	limitedBody := io.LimitReader(resp.Body, 10*1024*1024)

	var calResp struct {
		Items []struct {
			ID          string `json:"id"`
			Summary     string `json:"summary"`
			Description string `json:"description"`
			Location    string `json:"location"`
			Status      string `json:"status"`
			Start       struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"start"`
			End struct {
				DateTime string `json:"dateTime"`
				Date     string `json:"date"`
			} `json:"end"`
			Organizer struct {
				Email       string `json:"email"`
				DisplayName string `json:"displayName"`
			} `json:"organizer"`
			Attendees []struct {
				Email       string `json:"email"`
				DisplayName string `json:"displayName"`
			} `json:"attendees"`
			Recurrence []string `json:"recurrence"`
			Updated    string   `json:"updated"`
		} `json:"items"`
		NextSyncToken string `json:"nextSyncToken"`
	}

	if err := json.NewDecoder(limitedBody).Decode(&calResp); err != nil {
		return nil, fmt.Errorf("decode calendar response: %w", err)
	}

	var events []CalendarEvent
	for _, item := range calResp.Items {
		evt := CalendarEvent{
			UID:         item.ID,
			Summary:     item.Summary,
			Description: item.Description,
			Location:    item.Location,
			Status:      item.Status,
			Recurring:   len(item.Recurrence) > 0,
		}

		// Parse organizer
		if item.Organizer.DisplayName != "" {
			evt.Organizer = item.Organizer.DisplayName
		} else {
			evt.Organizer = item.Organizer.Email
		}

		// Parse attendees
		for _, a := range item.Attendees {
			name := a.DisplayName
			if name == "" {
				name = a.Email
			}
			evt.Attendees = append(evt.Attendees, name)
		}

		// Parse start/end times
		startStr := item.Start.DateTime
		if startStr == "" {
			startStr = item.Start.Date
		}
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			evt.Start = t
		} else if t, err := time.Parse("2006-01-02", startStr); err == nil {
			evt.Start = t
		}

		endStr := item.End.DateTime
		if endStr == "" {
			endStr = item.End.Date
		}
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			evt.End = t
		} else if t, err := time.Parse("2006-01-02", endStr); err == nil {
			evt.End = t
		}

		// Parse updated timestamp
		if item.Updated != "" {
			if t, err := time.Parse(time.RFC3339, item.Updated); err == nil {
				evt.Updated = t
			}
		}
		if evt.Updated.IsZero() {
			evt.Updated = evt.Start
		}

		if evt.Status == "" {
			evt.Status = "confirmed"
		}

		events = append(events, evt)
	}

	slog.Info("google calendar API fetch complete", "events", len(events))
	return events, nil
}

func getCredential(creds map[string]string, key string) string {
	if creds == nil {
		return ""
	}
	return creds[key]
}

// parseCalendarEvents converts interface{} events from config into CalendarEvent structs.
func parseCalendarEvents(raw interface{}) ([]CalendarEvent, error) {
	if raw == nil {
		return nil, nil
	}
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
