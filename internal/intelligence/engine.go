package intelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	smacknats "github.com/smackerel/smackerel/internal/nats"
	"github.com/smackerel/smackerel/internal/stringutil"
)

// maxSynthesisTopicGroups limits the number of cross-domain topic clusters
// evaluated per synthesis run. Capped to keep synthesis latency bounded and
// surface only the strongest signals.
const maxSynthesisTopicGroups = 10

// InsightType represents the type of synthesis insight.
type InsightType string

const (
	InsightThroughLine   InsightType = "through_line"
	InsightContradiction InsightType = "contradiction"
	InsightPattern       InsightType = "pattern"
	InsightSerendipity   InsightType = "serendipity"
)

// SynthesisInsight represents a detected cross-domain connection.
type SynthesisInsight struct {
	ID                string      `json:"id"`
	InsightType       InsightType `json:"insight_type"`
	ThroughLine       string      `json:"through_line"`
	KeyTension        string      `json:"key_tension,omitempty"`
	SuggestedAction   string      `json:"suggested_action,omitempty"`
	SourceArtifactIDs []string    `json:"source_artifact_ids"`
	Confidence        float64     `json:"confidence"`
	CreatedAt         time.Time   `json:"created_at"`
}

// AlertType represents the type of contextual alert.
type AlertType string

const (
	AlertBill              AlertType = "bill"
	AlertReturnWindow      AlertType = "return_window"
	AlertTripPrep          AlertType = "trip_prep"
	AlertRelationship      AlertType = "relationship_cooling"
	AlertCommitmentOverdue AlertType = "commitment_overdue"
	AlertMeetingBrief      AlertType = "meeting_brief"
)

// AlertStatus represents the lifecycle state of an alert.
type AlertStatus string

const (
	AlertPending   AlertStatus = "pending"
	AlertDelivered AlertStatus = "delivered"
	AlertDismissed AlertStatus = "dismissed"
	AlertSnoozed   AlertStatus = "snoozed"
)

// Alert represents a contextual alert.
type Alert struct {
	ID          string      `json:"id"`
	AlertType   AlertType   `json:"alert_type"`
	Title       string      `json:"title"`
	Body        string      `json:"body"`
	Priority    int         `json:"priority"` // 1=high, 2=medium, 3=low
	Status      AlertStatus `json:"status"`
	SnoozeUntil *time.Time  `json:"snooze_until,omitempty"`
	ArtifactID  string      `json:"artifact_id,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	DeliveredAt *time.Time  `json:"delivered_at,omitempty"`
}

// Engine orchestrates the intelligence pipeline.
type Engine struct {
	Pool *pgxpool.Pool
	NATS *smacknats.Client
}

// NewEngine creates a new intelligence engine.
func NewEngine(pool *pgxpool.Pool, nats *smacknats.Client) *Engine {
	return &Engine{Pool: pool, NATS: nats}
}

// RunSynthesis detects cross-domain clusters and generates insights.
func (e *Engine) RunSynthesis(ctx context.Context) ([]SynthesisInsight, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("synthesis requires a database connection")
	}

	// Find clusters: artifacts sharing topics from different sources (cross-domain).
	// R-301 requires clusters span multiple source_ids (email + article + video = different domains).
	rows, err := e.Pool.Query(ctx, `
		WITH topic_groups AS (
			SELECT t.id as topic_id, t.name,
			       array_agg(e.src_id) as artifact_ids,
			       COUNT(DISTINCT a.source_id) as source_count
			FROM edges e
			JOIN topics t ON t.id = e.dst_id AND e.dst_type = 'topic'
			JOIN artifacts a ON a.id = e.src_id
			WHERE e.edge_type = 'BELONGS_TO' AND e.src_type = 'artifact'
			GROUP BY t.id, t.name
			HAVING COUNT(*) >= 3 AND COUNT(DISTINCT a.source_id) >= 2
		)
		SELECT topic_id, name, artifact_ids, source_count FROM topic_groups
		ORDER BY array_length(artifact_ids, 1) DESC
		LIMIT $1
	`, maxSynthesisTopicGroups)
	if err != nil {
		return nil, fmt.Errorf("query clusters: %w", err)
	}
	defer rows.Close()

	var insights []SynthesisInsight
	for rows.Next() {
		// Check context between cluster evaluations
		if ctx.Err() != nil {
			return insights, ctx.Err()
		}

		var topicID, topicName string
		var artifactIDs []string
		var sourceCount int
		if err := rows.Scan(&topicID, &topicName, &artifactIDs, &sourceCount); err != nil {
			slog.Warn("synthesis scan failed", "error", err)
			continue
		}

		count := len(artifactIDs)
		if count < 3 {
			continue
		}

		confidence := synthesisConfidence(count, sourceCount)

		insights = append(insights, SynthesisInsight{
			ID:                ulid.Make().String(),
			InsightType:       InsightThroughLine,
			ThroughLine:       topicName,
			SourceArtifactIDs: artifactIDs,
			Confidence:        confidence,
			CreatedAt:         time.Now(),
		})
	}

	if err := rows.Err(); err != nil {
		return insights, fmt.Errorf("synthesis row iteration: %w", err)
	}

	return insights, nil
}

// synthesisConfidence computes insight confidence from artifact count and source diversity.
// More distinct sources (email + article + video) = higher confidence that
// the connection is genuinely cross-domain, not just volume.
// Returns a value in [0, 1].
func synthesisConfidence(artifactCount, sourceCount int) float64 {
	if artifactCount <= 0 || sourceCount <= 0 {
		return 0
	}
	volumeSignal := math.Log2(float64(artifactCount)) / 5.0
	diversitySignal := math.Log2(float64(sourceCount)) / 3.0
	return math.Min(1.0, 0.6*volumeSignal+0.4*diversitySignal)
}

// validAlertTypes is the set of known alert types for input validation.
var validAlertTypes = map[AlertType]bool{
	AlertBill:              true,
	AlertReturnWindow:      true,
	AlertTripPrep:          true,
	AlertRelationship:      true,
	AlertCommitmentOverdue: true,
	AlertMeetingBrief:      true,
}

// CreateAlert creates a new contextual alert.
func (e *Engine) CreateAlert(ctx context.Context, alert *Alert) error {
	if alert == nil {
		return fmt.Errorf("alert must not be nil")
	}
	if alert.Title == "" {
		return fmt.Errorf("alert title is required")
	}
	const maxTitleLen = 200
	const maxBodyLen = 2000
	if len(alert.Title) > maxTitleLen {
		slog.Warn("alert title truncated", "original_len", len(alert.Title), "max_len", maxTitleLen)
		alert.Title = stringutil.TruncateUTF8(alert.Title, maxTitleLen)
	}
	if len(alert.Body) > maxBodyLen {
		slog.Warn("alert body truncated", "original_len", len(alert.Body), "max_len", maxBodyLen)
		alert.Body = stringutil.TruncateUTF8(alert.Body, maxBodyLen)
	}
	// Sanitize control characters from connector-imported data (SEC-021-002, CWE-116).
	// Titles are single-line: collapse all control chars, newlines, and tabs to spaces.
	// Bodies may contain intentional newlines (e.g., meeting briefs) so only strip
	// true control chars (null bytes, carriage returns, escape sequences).
	alert.Title = strings.ReplaceAll(strings.ReplaceAll(
		stringutil.SanitizeControlChars(alert.Title), "\n", " "), "\t", " ")
	alert.Body = stringutil.SanitizeControlChars(alert.Body)
	if !validAlertTypes[alert.AlertType] {
		return fmt.Errorf("unknown alert type: %s", alert.AlertType)
	}
	if alert.Priority < 1 || alert.Priority > 3 {
		return fmt.Errorf("alert priority must be 1 (high), 2 (medium), or 3 (low), got %d", alert.Priority)
	}
	if e.Pool == nil {
		return fmt.Errorf("alert creation requires a database connection")
	}

	alert.ID = ulid.Make().String()
	alert.Status = AlertPending
	alert.CreatedAt = time.Now()

	_, err := e.Pool.Exec(ctx, `
		INSERT INTO alerts (id, alert_type, title, body, priority, status, artifact_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, alert.ID, string(alert.AlertType), alert.Title, alert.Body,
		alert.Priority, string(alert.Status), alert.ArtifactID, alert.CreatedAt)
	return err
}

// DismissAlert marks an alert as dismissed.
func (e *Engine) DismissAlert(ctx context.Context, alertID string) error {
	if alertID == "" {
		return fmt.Errorf("alert ID is required")
	}
	if e.Pool == nil {
		return fmt.Errorf("alert dismissal requires a database connection")
	}
	result, err := e.Pool.Exec(ctx, `
		UPDATE alerts SET status = 'dismissed' WHERE id = $1 AND status != 'dismissed'
	`, alertID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("alert not found: %s", alertID)
	}
	return nil
}

// SnoozeAlert snoozes an alert until a given time.
func (e *Engine) SnoozeAlert(ctx context.Context, alertID string, until time.Time) error {
	if alertID == "" {
		return fmt.Errorf("alert ID is required")
	}
	if !until.After(time.Now()) {
		return fmt.Errorf("snooze time must be in the future")
	}
	if e.Pool == nil {
		return fmt.Errorf("alert snooze requires a database connection")
	}
	result, err := e.Pool.Exec(ctx, `
		UPDATE alerts SET status = 'snoozed', snooze_until = $2 WHERE id = $1 AND status IN ('pending', 'delivered')
	`, alertID, until)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("alert not found: %s", alertID)
	}
	return nil
}

// maxPendingAlertAgeDays bounds how long a pending alert remains eligible for
// delivery. Alerts older than this are effectively dead-lettered to prevent
// infinite retry loops from poison alerts that Telegram consistently rejects
// (CWE-770: Allocation of Resources Without Limits).
const maxPendingAlertAgeDays = 7

// GetPendingAlerts returns alerts ready for delivery (max 2/day).
func (e *Engine) GetPendingAlerts(ctx context.Context) ([]Alert, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("alert delivery requires a database connection")
	}
	// Single query: compute remaining delivery slots and fetch pending alerts in one round-trip.
	// The age filter (SEC-021-001) prevents poison alerts from retrying indefinitely.
	rows, err := e.Pool.Query(ctx, fmt.Sprintf(`
		SELECT id, alert_type, title, body, priority, status, artifact_id, created_at
		FROM alerts
		WHERE (status = 'pending'
		   OR (status = 'snoozed' AND snooze_until <= NOW()))
		  AND created_at > NOW() - INTERVAL '%d days'
		ORDER BY priority, created_at
		LIMIT GREATEST(0, 2 - (
			SELECT COUNT(*) FROM alerts
			WHERE status = 'delivered' AND delivered_at >= CURRENT_DATE
		))
	`, maxPendingAlertAgeDays))
	if err != nil {
		return nil, fmt.Errorf("query pending alerts: %w", err)
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.AlertType, &a.Title, &a.Body,
			&a.Priority, &a.Status, &a.ArtifactID, &a.CreatedAt); err != nil {
			slog.Warn("alert scan failed", "error", err)
			continue
		}
		alerts = append(alerts, a)
	}
	if err := rows.Err(); err != nil {
		return alerts, err
	}
	return alerts, nil
}

// CheckOverdueCommitments finds action items past their expected date.
func (e *Engine) CheckOverdueCommitments(ctx context.Context) error {
	if e.Pool == nil {
		return fmt.Errorf("commitment check requires a database connection")
	}

	// Query overdue items, excluding those that already have a pending/delivered commitment_overdue alert.
	// This prevents duplicate alerts when CheckOverdueCommitments runs on consecutive days.
	rows, err := e.Pool.Query(ctx, `
		SELECT ai.id, ai.text, ai.expected_date, COALESCE(p.name, 'unknown')
		FROM action_items ai
		LEFT JOIN people p ON p.id = ai.person_id
		WHERE ai.status = 'open' AND ai.expected_date < CURRENT_DATE
		  AND NOT EXISTS (
		    SELECT 1 FROM alerts al
		    WHERE al.artifact_id = ai.id
		      AND al.alert_type = 'commitment_overdue'
		      AND al.status IN ('pending', 'delivered')
		  )
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, text, person string
		var expectedDate time.Time
		if err := rows.Scan(&id, &text, &expectedDate, &person); err != nil {
			slog.Warn("overdue commitment scan failed", "error", err)
			continue
		}

		localToday := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.Local)
		daysOverdue := calendarDaysBetween(expectedDate, localToday)
		if err := e.CreateAlert(ctx, &Alert{
			AlertType:  AlertCommitmentOverdue,
			Title:      fmt.Sprintf("Overdue: %s", text),
			Body:       fmt.Sprintf("%s — %d days overdue (from %s)", text, daysOverdue, person),
			Priority:   1,
			ArtifactID: id,
		}); err != nil {
			slog.Warn("failed to create overdue alert", "action_item_id", id, "error", err)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("overdue commitments row iteration: %w", err)
	}

	return nil
}

// MeetingBrief is a pre-meeting context brief per R-306.
type MeetingBrief struct {
	EventID    string          `json:"event_id"`
	EventTitle string          `json:"event_title"`
	StartsAt   time.Time       `json:"starts_at"`
	Attendees  []AttendeeBrief `json:"attendees"`
	BriefText  string          `json:"brief_text"`
}

// AttendeeBrief summarizes context for one meeting attendee.
type AttendeeBrief struct {
	Name          string   `json:"name"`
	Email         string   `json:"email"`
	RecentThreads []string `json:"recent_threads"`
	SharedTopics  []string `json:"shared_topics"`
	PendingItems  []string `json:"pending_action_items"`
	IsNewContact  bool     `json:"is_new_contact"`
}

// GeneratePreMeetingBriefs checks for upcoming meetings and generates context briefs per R-306.
// Queries calendar events 25-35 minutes from now, deduplicates by event ID, and assembles
// per-attendee context from email threads, shared topics, and pending commitments.
func (e *Engine) GeneratePreMeetingBriefs(ctx context.Context) ([]MeetingBrief, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("pre-meeting briefs require a database connection")
	}

	// 1. Find calendar events starting in 25-35 minutes
	rows, err := e.Pool.Query(ctx, `
		SELECT a.id, a.title, a.captured_at,
		       COALESCE(a.metadata->>'attendees', '[]') AS attendees
		FROM artifacts a
		WHERE a.source_id IN ('caldav', 'google-calendar', 'outlook-calendar')
		  AND a.captured_at BETWEEN NOW() + INTERVAL '25 minutes' AND NOW() + INTERVAL '35 minutes'
		  AND NOT EXISTS (
			SELECT 1 FROM alerts al
			WHERE al.alert_type = 'meeting_brief' AND al.artifact_id = a.id
		  )
		ORDER BY a.captured_at ASC
		LIMIT 5
	`)
	if err != nil {
		return nil, fmt.Errorf("query upcoming meetings: %w", err)
	}
	defer rows.Close()

	var briefs []MeetingBrief
	for rows.Next() {
		var eventID, title, attendeesJSON string
		var startsAt time.Time
		if err := rows.Scan(&eventID, &title, &startsAt, &attendeesJSON); err != nil {
			slog.Warn("meeting scan failed", "error", err)
			continue
		}

		// Parse attendees
		var attendeeEmails []string
		if err := json.Unmarshal([]byte(attendeesJSON), &attendeeEmails); err != nil {
			slog.Debug("failed to unmarshal meeting attendees", "event_id", eventID, "error", err)
		}

		// Cap attendees to prevent excessive per-attendee DB queries.
		const maxAttendeesPerMeeting = 10
		if len(attendeeEmails) > maxAttendeesPerMeeting {
			attendeeEmails = attendeeEmails[:maxAttendeesPerMeeting]
		}

		brief := MeetingBrief{
			EventID:    eventID,
			EventTitle: title,
			StartsAt:   startsAt,
		}

		// 2. Build per-attendee context
		for _, email := range attendeeEmails {
			ab := e.buildAttendeeBrief(ctx, email)
			brief.Attendees = append(brief.Attendees, ab)
		}

		// 3. Assemble brief text
		brief.BriefText = assembleBriefText(brief)

		// 4. Create dedup alert to prevent duplicate briefs.
		// Mark as delivered immediately since the brief is sent directly by the
		// scheduler — prevents the alert delivery sweep from re-sending it
		// (SEC-021-003: double-delivery of meeting brief alerts).
		briefAlert := &Alert{
			AlertType:  AlertMeetingBrief,
			Title:      fmt.Sprintf("Meeting brief: %s", title),
			Body:       brief.BriefText,
			Priority:   1,
			ArtifactID: eventID,
		}
		if err := e.CreateAlert(ctx, briefAlert); err != nil {
			slog.Warn("failed to create meeting brief alert", "event", eventID, "error", err)
		} else {
			// Use a detached context for marking delivered so that context cancellation
			// between CreateAlert and MarkAlertDelivered doesn't leave sent-but-unmarked
			// alerts (matching the C2 fix pattern from deliverPendingAlerts).
			markCtx, markCancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := e.MarkAlertDelivered(markCtx, briefAlert.ID); err != nil {
				slog.Warn("failed to mark brief alert delivered", "event", eventID, "error", err)
			}
			markCancel()
		}

		briefs = append(briefs, brief)
	}

	return briefs, rows.Err()
}

// buildAttendeeBrief assembles context for a single meeting attendee.
func (e *Engine) buildAttendeeBrief(ctx context.Context, email string) AttendeeBrief {
	ab := AttendeeBrief{Email: email}

	// Check if known contact
	var personName string
	err := e.Pool.QueryRow(ctx, `
		SELECT name FROM people WHERE email = $1 LIMIT 1
	`, email).Scan(&personName)
	if err != nil {
		ab.IsNewContact = true
		ab.Name = email
		return ab
	}
	ab.Name = personName

	// Escape LIKE wildcards in email to prevent unintended pattern matching
	escapedEmail := stringutil.EscapeLikePattern(email)

	// Recent email threads (last 3)
	threadRows, err := e.Pool.Query(ctx, `
		SELECT a.title FROM artifacts a
		WHERE a.source_id IN ('gmail', 'imap', 'outlook')
		  AND (a.metadata->>'sender' = $1 OR a.metadata->>'recipients' LIKE '%' || $2 || '%')
		ORDER BY a.created_at DESC LIMIT 3
	`, email, escapedEmail)
	if err == nil {
		defer threadRows.Close()
		for threadRows.Next() {
			var t string
			if threadRows.Scan(&t) == nil {
				ab.RecentThreads = append(ab.RecentThreads, t)
			}
		}
	}

	// Shared topics
	topicRows, err := e.Pool.Query(ctx, `
		SELECT DISTINCT t.name FROM topics t
		JOIN edges e ON e.dst_id = t.id AND e.dst_type = 'topic' AND e.edge_type = 'BELONGS_TO'
		JOIN artifacts a ON a.id = e.src_id
		WHERE a.metadata->>'sender' = $1 OR a.metadata->>'recipients' LIKE '%' || $2 || '%'
		LIMIT 5
	`, email, escapedEmail)
	if err == nil {
		defer topicRows.Close()
		for topicRows.Next() {
			var t string
			if topicRows.Scan(&t) == nil {
				ab.SharedTopics = append(ab.SharedTopics, t)
			}
		}
	}

	// Pending action items from/to this person
	aiRows, err := e.Pool.Query(ctx, `
		SELECT text FROM action_items
		WHERE person_id IN (SELECT id FROM people WHERE email = $1)
		  AND status = 'open'
		LIMIT 3
	`, email)
	if err == nil {
		defer aiRows.Close()
		for aiRows.Next() {
			var t string
			if aiRows.Scan(&t) == nil {
				ab.PendingItems = append(ab.PendingItems, t)
			}
		}
	}

	return ab
}

// assembleBriefText generates a 2-3 sentence brief for a meeting.
func assembleBriefText(brief MeetingBrief) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("Meeting: %s", brief.EventTitle))

	for _, a := range brief.Attendees {
		if a.IsNewContact {
			parts = append(parts, fmt.Sprintf("• %s — No prior context. New contact.", a.Email))
			continue
		}
		var context []string
		if len(a.RecentThreads) > 0 {
			context = append(context, fmt.Sprintf("%d recent threads", len(a.RecentThreads)))
		}
		if len(a.SharedTopics) > 0 {
			context = append(context, fmt.Sprintf("shared topics: %s", strings.Join(a.SharedTopics, ", ")))
		}
		if len(a.PendingItems) > 0 {
			context = append(context, fmt.Sprintf("%d pending items", len(a.PendingItems)))
		}
		if len(context) > 0 {
			parts = append(parts, fmt.Sprintf("• %s — %s", a.Name, strings.Join(context, "; ")))
		}
	}

	return strings.Join(parts, "\n")
}

// WeeklySynthesis is the weekly knowledge synthesis per R-307.
type WeeklySynthesis struct {
	WeekOf           string               `json:"week_of"`
	Stats            WeeklyStats          `json:"stats"`
	Insights         []SynthesisInsight   `json:"insights"`
	TopicMovement    []TopicMovement      `json:"topic_movement"`
	OpenLoops        []string             `json:"open_loops"`
	SerendipityPicks []ResurfaceCandidate `json:"serendipity_picks"`
	Patterns         []string             `json:"patterns"`
	WordCount        int                  `json:"word_count"`
	SynthesisText    string               `json:"synthesis_text"`
}

// WeeklyStats summarizes the week's activity.
type WeeklyStats struct {
	ArtifactsProcessed int `json:"artifacts_processed"`
	NewConnections     int `json:"new_connections"`
	TopicsActive       int `json:"topics_active"`
	SearchesPerformed  int `json:"searches_performed"`
}

// TopicMovement shows how a topic's momentum changed this week.
type TopicMovement struct {
	TopicName string `json:"topic_name"`
	Direction string `json:"direction"` // rising, falling, stable
	Captures  int    `json:"captures_this_week"`
}

// GenerateWeeklySynthesis assembles and generates the weekly knowledge synthesis per R-307.
func (e *Engine) GenerateWeeklySynthesis(ctx context.Context) (*WeeklySynthesis, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("weekly synthesis requires a database connection")
	}

	ws := &WeeklySynthesis{
		WeekOf: time.Now().Format("2006-01-02"),
	}

	// 1. Weekly stats — single query to reduce round-trips and honour context cancellation
	if err := e.Pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM artifacts WHERE created_at > NOW() - INTERVAL '7 days'),
			(SELECT COUNT(*) FROM edges WHERE created_at > NOW() - INTERVAL '7 days'),
			(SELECT COUNT(DISTINCT dst_id) FROM edges WHERE edge_type = 'BELONGS_TO' AND dst_type = 'topic' AND created_at > NOW() - INTERVAL '7 days'),
			(SELECT COUNT(*) FROM search_log WHERE created_at > NOW() - INTERVAL '7 days')
	`).Scan(&ws.Stats.ArtifactsProcessed, &ws.Stats.NewConnections,
		&ws.Stats.TopicsActive, &ws.Stats.SearchesPerformed); err != nil {
		slog.Warn("failed to query weekly stats", "error", err)
	}

	// Check context between heavy operations to abort early on cancellation
	if ctx.Err() != nil {
		return ws, ctx.Err()
	}

	// 2. Synthesis insights from this week
	insights, err := e.RunSynthesis(ctx)
	if err == nil {
		ws.Insights = insights
	}

	// 3. Topic movement
	topicRows, err := e.Pool.Query(ctx, `
		SELECT t.name,
		       COUNT(DISTINCT CASE WHEN a.created_at > NOW() - INTERVAL '7 days' THEN a.id END) AS this_week,
		       COUNT(DISTINCT CASE WHEN a.created_at BETWEEN NOW() - INTERVAL '14 days' AND NOW() - INTERVAL '7 days' THEN a.id END) AS last_week
		FROM topics t
		JOIN edges e ON e.dst_id = t.id AND e.dst_type = 'topic' AND e.edge_type = 'BELONGS_TO'
		JOIN artifacts a ON a.id = e.src_id
		WHERE a.created_at > NOW() - INTERVAL '14 days'
		GROUP BY t.name
		HAVING COUNT(DISTINCT CASE WHEN a.created_at > NOW() - INTERVAL '7 days' THEN a.id END) > 0
		ORDER BY this_week DESC
		LIMIT 10
	`)
	if err == nil {
		defer topicRows.Close()
		for topicRows.Next() {
			var tm TopicMovement
			var lastWeek int
			if topicRows.Scan(&tm.TopicName, &tm.Captures, &lastWeek) == nil {
				if tm.Captures > lastWeek+1 {
					tm.Direction = "rising"
				} else if tm.Captures < lastWeek-1 {
					tm.Direction = "falling"
				} else {
					tm.Direction = "stable"
				}
				ws.TopicMovement = append(ws.TopicMovement, tm)
			}
		}
		if err := topicRows.Err(); err != nil {
			slog.Warn("weekly synthesis topic movement iteration failed", "error", err)
		}
	}

	// 4. Open loops (overdue action items)
	loopRows, err := e.Pool.Query(ctx, `
		SELECT text FROM action_items WHERE status = 'open' AND expected_date < CURRENT_DATE
		ORDER BY expected_date ASC LIMIT 5
	`)
	if err == nil {
		defer loopRows.Close()
		for loopRows.Next() {
			var t string
			if loopRows.Scan(&t) == nil {
				ws.OpenLoops = append(ws.OpenLoops, t)
			}
		}
		if err := loopRows.Err(); err != nil {
			slog.Warn("weekly synthesis open loops iteration failed", "error", err)
		}
	}

	if ctx.Err() != nil {
		return ws, ctx.Err()
	}

	// 5. Serendipity pick
	candidates, err := e.Resurface(ctx, 1)
	if err == nil {
		ws.SerendipityPicks = candidates
	}

	// 6. Patterns (capture timing analysis)
	ws.Patterns = e.detectCapturePatterns(ctx)

	// Assemble synthesis text and enforce R-302 250-word cap
	ws.SynthesisText = assembleWeeklySynthesisText(ws)
	words := strings.Fields(ws.SynthesisText)
	if len(words) > 250 {
		ws.SynthesisText = strings.Join(words[:250], " ")
	}
	ws.WordCount = len(strings.Fields(ws.SynthesisText))

	return ws, nil
}

// detectCapturePatterns analyzes timestamp patterns in user captures.
func (e *Engine) detectCapturePatterns(ctx context.Context) []string {
	if e.Pool == nil {
		return nil
	}
	if ctx.Err() != nil {
		return nil
	}
	var patterns []string

	// Day-of-week pattern
	rows, err := e.Pool.Query(ctx, `
		SELECT EXTRACT(DOW FROM created_at)::int AS dow, COUNT(*) AS cnt
		FROM artifacts
		WHERE created_at > NOW() - INTERVAL '30 days'
		GROUP BY dow
		ORDER BY cnt DESC
		LIMIT 1
	`)
	if err == nil {
		defer rows.Close()
		dayNames := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
		for rows.Next() {
			var dow, cnt int
			if rows.Scan(&dow, &cnt) == nil && dow >= 0 && dow < 7 {
				patterns = append(patterns, fmt.Sprintf("You save the most content on %ss (%d captures in the last 30 days)", dayNames[dow], cnt))
			}
		}
		if err := rows.Err(); err != nil {
			slog.Warn("capture pattern day-of-week iteration failed", "error", err)
		}
	}

	if ctx.Err() != nil {
		return patterns
	}

	// Hour-of-day pattern
	rows2, err := e.Pool.Query(ctx, `
		SELECT EXTRACT(HOUR FROM created_at)::int AS hr, COUNT(*) AS cnt
		FROM artifacts
		WHERE created_at > NOW() - INTERVAL '30 days'
		GROUP BY hr
		ORDER BY cnt DESC
		LIMIT 1
	`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var hr, cnt int
			if rows2.Scan(&hr, &cnt) == nil {
				period := "morning"
				if hr >= 12 && hr < 17 {
					period = "afternoon"
				} else if hr >= 17 {
					period = "evening"
				}
				patterns = append(patterns, fmt.Sprintf("Your peak capture time is %s (%d:00, %d captures in 30 days)", period, hr, cnt))
			}
		}
		if err := rows2.Err(); err != nil {
			slog.Warn("capture pattern hour-of-day iteration failed", "error", err)
		}
	}

	return patterns
}

// assembleWeeklySynthesisText generates the week-in-review text.
func assembleWeeklySynthesisText(ws *WeeklySynthesis) string {
	var sections []string

	// STATS
	if ws.Stats.ArtifactsProcessed > 0 {
		sections = append(sections, fmt.Sprintf("THIS WEEK: %d artifacts processed, %d new connections, %d active topics.",
			ws.Stats.ArtifactsProcessed, ws.Stats.NewConnections, ws.Stats.TopicsActive))
	}

	// INSIGHTS
	if len(ws.Insights) > 0 {
		var lines []string
		for _, i := range ws.Insights {
			lines = append(lines, fmt.Sprintf("• %s (confidence: %.0f%%)", i.ThroughLine, i.Confidence*100))
		}
		sections = append(sections, "INSIGHTS:\n"+strings.Join(lines, "\n"))
	}

	// TOPICS
	if len(ws.TopicMovement) > 0 {
		var lines []string
		for _, tm := range ws.TopicMovement {
			arrow := "→"
			if tm.Direction == "rising" {
				arrow = "↑"
			} else if tm.Direction == "falling" {
				arrow = "↓"
			}
			lines = append(lines, fmt.Sprintf("• %s %s (%d this week)", arrow, tm.TopicName, tm.Captures))
		}
		sections = append(sections, "TOPICS:\n"+strings.Join(lines, "\n"))
	}

	// OPEN LOOPS
	if len(ws.OpenLoops) > 0 {
		var lines []string
		for _, l := range ws.OpenLoops {
			lines = append(lines, "• "+l)
		}
		sections = append(sections, "OPEN LOOPS:\n"+strings.Join(lines, "\n"))
	}

	// SERENDIPITY
	if len(ws.SerendipityPicks) > 0 {
		pick := ws.SerendipityPicks[0]
		sections = append(sections, fmt.Sprintf("FROM THE ARCHIVE: %s — %s", pick.Title, pick.Reason))
	}

	// PATTERNS
	if len(ws.Patterns) > 0 {
		sections = append(sections, "PATTERNS NOTICED:\n"+strings.Join(ws.Patterns, "\n"))
	}

	if len(sections) == 0 {
		return "Quiet week — not much to report. Keep exploring!"
	}

	return strings.Join(sections, "\n\n")
}

// MarkAlertDelivered marks an alert as delivered with a delivery timestamp.
func (e *Engine) MarkAlertDelivered(ctx context.Context, alertID string) error {
	if alertID == "" {
		return fmt.Errorf("alert ID is required")
	}
	if e.Pool == nil {
		return fmt.Errorf("alert delivery requires a database connection")
	}
	result, err := e.Pool.Exec(ctx, `
		UPDATE alerts SET status = 'delivered', delivered_at = NOW()
		WHERE id = $1 AND status IN ('pending', 'snoozed')
	`, alertID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("alert not found or already delivered: %s", alertID)
	}
	return nil
}

// ProduceBillAlerts creates alerts for subscriptions with upcoming billing dates.
func (e *Engine) ProduceBillAlerts(ctx context.Context) error {
	if e.Pool == nil {
		return fmt.Errorf("bill alert production requires a database connection")
	}

	rows, err := e.Pool.Query(ctx, `
		SELECT id, service_name, amount, currency, billing_freq, first_seen
		FROM subscriptions
		WHERE status = 'active'
		  AND billing_freq IS NOT NULL
		  AND NOT EXISTS (
		    SELECT 1 FROM alerts
		    WHERE alert_type = 'bill'
		      AND artifact_id = subscriptions.id
		      AND status IN ('pending', 'delivered')
		      AND created_at > NOW() - INTERVAL '30 days'
		  )
		LIMIT 20
	`)
	if err != nil {
		return fmt.Errorf("query subscriptions for billing: %w", err)
	}
	defer rows.Close()

	var created int
	for rows.Next() {
		if ctx.Err() != nil {
			slog.Warn("bill alert production context cancelled, remaining rows skipped", "created_so_far", created)
			break
		}

		var id, serviceName, currency, billingFreq string
		var amount float64
		var firstSeen time.Time
		if err := rows.Scan(&id, &serviceName, &amount, &currency, &billingFreq, &firstSeen); err != nil {
			slog.Warn("bill alert scan failed", "error", err)
			continue
		}

		// Estimate next billing date using proper date arithmetic.
		// For monthly: same day-of-month in the current month (clamped to month length).
		// For annual: same month and day as first_seen.
		now := time.Now()
		billingDay := firstSeen.Day()
		// Use local midnight (not now.Truncate which aligns to UTC boundaries)
		// to ensure consistent comparison with clampDay's time.Local dates.
		localToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

		var nextBilling time.Time
		if billingFreq == "annual" {
			// Try this year first, then next year
			nextBilling = clampDay(now.Year(), firstSeen.Month(), billingDay)
			if nextBilling.Before(localToday) {
				nextBilling = clampDay(now.Year()+1, firstSeen.Month(), billingDay)
			}
		} else {
			// Monthly: try current month, then next month
			nextBilling = clampDay(now.Year(), now.Month(), billingDay)
			if nextBilling.Before(localToday) {
				nextMonth := now.Month() + 1
				nextYear := now.Year()
				if nextMonth > 12 {
					nextMonth = 1
					nextYear++
				}
				nextBilling = clampDay(nextYear, nextMonth, billingDay)
			}
		}

		daysUntilBilling := calendarDaysBetween(localToday, nextBilling)
		if daysUntilBilling < 0 || daysUntilBilling > 3 {
			continue
		}

		title := fmt.Sprintf("Upcoming charge: %s", serviceName)
		if amount > 0 {
			title = fmt.Sprintf("Upcoming charge: %s (%.2f %s)", serviceName, amount, currency)
		}

		if err := e.CreateAlert(ctx, &Alert{
			AlertType:  AlertBill,
			Title:      title,
			Body:       fmt.Sprintf("%s billing expected in ~%d days", serviceName, daysUntilBilling),
			Priority:   2,
			ArtifactID: id,
		}); err != nil {
			slog.Warn("failed to create bill alert", "subscription", serviceName, "error", err)
		} else {
			created++
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("bill alert row iteration: %w", err)
	}

	slog.Info("bill alert production complete", "created", created)
	return nil
}

// ProduceTripPrepAlerts creates alerts for upcoming trips with departure within 5 days.
func (e *Engine) ProduceTripPrepAlerts(ctx context.Context) error {
	if e.Pool == nil {
		return fmt.Errorf("trip prep alert production requires a database connection")
	}

	rows, err := e.Pool.Query(ctx, `
		SELECT id, destination, start_date
		FROM trips
		WHERE status = 'upcoming'
		  AND start_date BETWEEN CURRENT_DATE AND CURRENT_DATE + INTERVAL '5 days'
		  AND NOT EXISTS (
		    SELECT 1 FROM alerts
		    WHERE alert_type = 'trip_prep'
		      AND artifact_id = trips.id
		      AND status IN ('pending', 'delivered')
		  )
		LIMIT 10
	`)
	if err != nil {
		return fmt.Errorf("query upcoming trips: %w", err)
	}
	defer rows.Close()

	var created int
	for rows.Next() {
		if ctx.Err() != nil {
			slog.Warn("trip prep alert production context cancelled, remaining rows skipped", "created_so_far", created)
			break
		}

		var id, destination string
		var startDate time.Time
		if err := rows.Scan(&id, &destination, &startDate); err != nil {
			slog.Warn("trip alert scan failed", "error", err)
			continue
		}

		// Use calendarDaysBetween for DST-safe, time-of-day-independent day counting
		// consistent with ProduceBillAlerts and CheckOverdueCommitments.
		now := time.Now()
		localToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		daysUntil := calendarDaysBetween(localToday, startDate)
		if daysUntil < 0 {
			daysUntil = 0
		}

		if err := e.CreateAlert(ctx, &Alert{
			AlertType:  AlertTripPrep,
			Title:      fmt.Sprintf("Trip prep: %s in %d days", destination, daysUntil),
			Body:       fmt.Sprintf("Your trip to %s departs in %d days. Check bookings and packing.", destination, daysUntil),
			Priority:   2,
			ArtifactID: id,
		}); err != nil {
			slog.Warn("failed to create trip prep alert", "trip", destination, "error", err)
		} else {
			created++
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("trip alert row iteration: %w", err)
	}

	slog.Info("trip prep alert production complete", "created", created)
	return nil
}

// ProduceReturnWindowAlerts creates alerts for artifacts with return deadlines expiring within 5 days.
func (e *Engine) ProduceReturnWindowAlerts(ctx context.Context) error {
	if e.Pool == nil {
		return fmt.Errorf("return window alert production requires a database connection")
	}

	// Use a safe date cast so a single artifact with a malformed return_deadline
	// doesn't cause the entire query to fail (killing all return window detection).
	rows, err := e.Pool.Query(ctx, `
		SELECT id, title, metadata->>'return_deadline' AS return_deadline
		FROM artifacts
		WHERE metadata->>'return_deadline' IS NOT NULL
		  AND metadata->>'return_deadline' ~ '^\d{4}-\d{2}-\d{2}$'
		  AND (metadata->>'return_deadline')::date BETWEEN CURRENT_DATE AND CURRENT_DATE + INTERVAL '5 days'
		  AND NOT EXISTS (
		    SELECT 1 FROM alerts
		    WHERE alert_type = 'return_window'
		      AND artifact_id = artifacts.id
		      AND status IN ('pending', 'delivered')
		  )
		LIMIT 10
	`)
	if err != nil {
		return fmt.Errorf("query return windows: %w", err)
	}
	defer rows.Close()

	var created int
	for rows.Next() {
		if ctx.Err() != nil {
			slog.Warn("return window alert production context cancelled, remaining rows skipped", "created_so_far", created)
			break
		}

		var id, title, deadlineStr string
		if err := rows.Scan(&id, &title, &deadlineStr); err != nil {
			slog.Warn("return window scan failed", "error", err)
			continue
		}

		if err := e.CreateAlert(ctx, &Alert{
			AlertType:  AlertReturnWindow,
			Title:      fmt.Sprintf("Return window closing: %s", title),
			Body:       fmt.Sprintf("Return deadline for \"%s\" is %s. Act soon.", title, deadlineStr),
			Priority:   1,
			ArtifactID: id,
		}); err != nil {
			slog.Warn("failed to create return window alert", "artifact", id, "error", err)
		} else {
			created++
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("return window row iteration: %w", err)
	}

	slog.Info("return window alert production complete", "created", created)
	return nil
}

// ProduceRelationshipCoolingAlerts creates alerts for contacts with fading communication.
func (e *Engine) ProduceRelationshipCoolingAlerts(ctx context.Context) error {
	if e.Pool == nil {
		return fmt.Errorf("relationship cooling alert production requires a database connection")
	}

	rows, err := e.Pool.Query(ctx, `
		SELECT p.id, p.name,
		       EXTRACT(DAY FROM NOW() - MAX(a.created_at))::int AS days_since
		FROM people p
		JOIN edges e ON e.dst_id = p.id AND e.dst_type = 'person'
		JOIN artifacts a ON a.id = e.src_id
		GROUP BY p.id, p.name
		HAVING EXTRACT(DAY FROM NOW() - MAX(a.created_at)) > 30
		   AND COUNT(DISTINCT a.id) FILTER (WHERE a.created_at BETWEEN NOW() - INTERVAL '180 days' AND NOW() - INTERVAL '90 days') >= 4
		   AND NOT EXISTS (
		     SELECT 1 FROM alerts
		     WHERE alert_type = 'relationship_cooling'
		       AND artifact_id = p.id
		       AND status IN ('pending', 'delivered')
		       AND created_at > NOW() - INTERVAL '30 days'
		   )
		LIMIT 10
	`)
	if err != nil {
		return fmt.Errorf("query cooling relationships: %w", err)
	}
	defer rows.Close()

	var created int
	for rows.Next() {
		if ctx.Err() != nil {
			slog.Warn("relationship cooling alert production context cancelled, remaining rows skipped", "created_so_far", created)
			break
		}

		var id, name string
		var daysSince int
		if err := rows.Scan(&id, &name, &daysSince); err != nil {
			slog.Warn("relationship cooling scan failed", "error", err)
			continue
		}

		if err := e.CreateAlert(ctx, &Alert{
			AlertType:  AlertRelationship,
			Title:      fmt.Sprintf("Reconnect with %s? Last contact %d days ago", name, daysSince),
			Body:       fmt.Sprintf("You used to communicate regularly with %s, but it's been %d days since your last interaction.", name, daysSince),
			Priority:   3,
			ArtifactID: id,
		}); err != nil {
			slog.Warn("failed to create relationship cooling alert", "person", name, "error", err)
		} else {
			created++
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("relationship cooling row iteration: %w", err)
	}

	slog.Info("relationship cooling alert production complete", "created", created)
	return nil
}

// GetLastSynthesisTime returns the timestamp of the most recent synthesis insight.
// Returns epoch time if no synthesis has ever run.
func (e *Engine) GetLastSynthesisTime(ctx context.Context) (time.Time, error) {
	if e.Pool == nil {
		return time.Time{}, fmt.Errorf("synthesis freshness check requires a database connection")
	}

	var lastSynthesis time.Time
	err := e.Pool.QueryRow(ctx, `
		SELECT COALESCE(MAX(created_at), '1970-01-01'::timestamptz) FROM synthesis_insights
	`).Scan(&lastSynthesis)
	if err != nil {
		return time.Time{}, fmt.Errorf("query last synthesis time: %w", err)
	}
	return lastSynthesis, nil
}

// clampDay returns the date for (year, month, day) clamped to the last day
// of the given month. E.g. clampDay(2026, time.February, 31) → Feb 28.
func clampDay(year int, month time.Month, day int) time.Time {
	// time.Date normalises overflow: Feb 31 → Mar 3, so instead compute
	// the last day of the month and clamp.
	if day < 1 {
		day = 1
	}
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, time.Local).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.Local)
}

// calendarDaysBetween counts whole calendar days from 'from' to 'to',
// ignoring time-of-day. Uses UTC normalisation so DST transitions
// (23-hour or 25-hour days) don't cause off-by-one errors.
func calendarDaysBetween(from, to time.Time) int {
	fromUTC := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	toUTC := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(toUTC.Sub(fromUTC).Hours() / 24)
}
