package intelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/stringutil"
)

// overdueItem holds the fields needed to create an overdue-commitment alert.
type overdueItem struct {
	id           string
	text         string
	expectedDate time.Time
	person       string
}

// CheckOverdueCommitments finds action items past their expected date.
func (e *Engine) CheckOverdueCommitments(ctx context.Context) error {
	if e.Pool == nil {
		return fmt.Errorf("commitment check requires a database connection")
	}

	// Collect overdue items first, then close the cursor before creating
	// alerts. This avoids holding a SELECT cursor open while performing
	// INSERT operations, which would pin two pool connections simultaneously
	// per iteration under high overdue counts (IMP-004-SQS-001).
	items, err := e.collectOverdueItems(ctx)
	if err != nil {
		return err
	}

	localToday := time.Date(time.Now().Year(), time.Now().Month(), time.Now().Day(), 0, 0, 0, 0, time.Local)
	for _, item := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		daysOverdue := calendarDaysBetween(item.expectedDate, localToday)
		if err := e.CreateAlert(ctx, &Alert{
			AlertType:  AlertCommitmentOverdue,
			Title:      fmt.Sprintf("Overdue: %s", item.text),
			Body:       fmt.Sprintf("%s — %d days overdue (from %s)", item.text, daysOverdue, item.person),
			Priority:   1,
			ArtifactID: item.id,
		}); err != nil {
			slog.Warn("failed to create overdue alert", "action_item_id", item.id, "error", err)
		}
	}

	return nil
}

// collectOverdueItems queries overdue action items and returns them as a slice,
// closing the cursor before the caller performs any write operations.
func (e *Engine) collectOverdueItems(ctx context.Context) ([]overdueItem, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("commitment check requires a database connection")
	}
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
		return nil, err
	}
	defer rows.Close()

	var items []overdueItem
	for rows.Next() {
		var item overdueItem
		if err := rows.Scan(&item.id, &item.text, &item.expectedDate, &item.person); err != nil {
			slog.Warn("overdue commitment scan failed", "error", err)
			continue
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return items, fmt.Errorf("overdue commitments row iteration: %w", err)
	}
	return items, nil
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
// Checks ctx.Err() between sequential DB queries to abort early on
// cancellation instead of running all 3 queries unconditionally
// (IMP-004-SQS-002).
func (e *Engine) buildAttendeeBrief(ctx context.Context, email string) AttendeeBrief {
	ab := AttendeeBrief{Email: email}

	if e.Pool == nil {
		ab.IsNewContact = true
		ab.Name = email
		return ab
	}

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

	if ctx.Err() != nil {
		return ab
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

	if ctx.Err() != nil {
		return ab
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
