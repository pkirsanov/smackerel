package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/smackerel/smackerel/internal/stringutil"
)

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
	// Uses MAKE_INTERVAL instead of fmt.Sprintf to keep all SQL parameterized.
	rows, err := e.Pool.Query(ctx, `
		SELECT id, alert_type, title, body, priority, status, COALESCE(artifact_id, ''), created_at
		FROM alerts
		WHERE (status = 'pending'
		   OR (status = 'snoozed' AND snooze_until <= NOW()))
		  AND created_at > NOW() - MAKE_INTERVAL(days => $1)
		ORDER BY priority, created_at
		LIMIT GREATEST(0, 2 - (
			SELECT COUNT(*) FROM alerts
			WHERE status = 'delivered' AND delivered_at >= CURRENT_DATE
		))
	`, maxPendingAlertAgeDays)
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

// HasStalePendingAlerts checks whether any alerts have been pending for longer
// than the given threshold without delivery. This detects a broken delivery
// pipeline: if alerts are being produced but not swept, something is wrong.
func (e *Engine) HasStalePendingAlerts(ctx context.Context, threshold time.Duration) (bool, error) {
	if e.Pool == nil {
		return false, fmt.Errorf("alert staleness check requires a database connection")
	}
	var count int
	err := e.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM alerts
		WHERE status = 'pending'
		  AND created_at < NOW() - MAKE_INTERVAL(secs => $1)
	`, int(threshold.Seconds())).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("query stale pending alerts: %w", err)
	}
	return count > 0, nil
}
