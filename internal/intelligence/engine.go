package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	smacknats "github.com/smackerel/smackerel/internal/nats"
)

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
			SELECT t.id as topic_id, t.name, array_agg(e.src_id) as artifact_ids
			FROM edges e
			JOIN topics t ON t.id = e.dst_id AND e.dst_type = 'topic'
			JOIN artifacts a ON a.id = e.src_id
			WHERE e.edge_type = 'BELONGS_TO' AND e.src_type = 'artifact'
			GROUP BY t.id, t.name
			HAVING COUNT(*) >= 3 AND COUNT(DISTINCT a.source_id) >= 2
		)
		SELECT topic_id, name, artifact_ids FROM topic_groups
		ORDER BY array_length(artifact_ids, 1) DESC
		LIMIT 10
	`)
	if err != nil {
		return nil, fmt.Errorf("query clusters: %w", err)
	}
	defer rows.Close()

	var insights []SynthesisInsight
	for rows.Next() {
		var topicID, topicName string
		var artifactIDs []string
		if err := rows.Scan(&topicID, &topicName, &artifactIDs); err != nil {
			slog.Warn("synthesis scan failed", "error", err)
			continue
		}

		count := len(artifactIDs)
		if count < 3 {
			continue
		}

		insights = append(insights, SynthesisInsight{
			ID:                ulid.Make().String(),
			InsightType:       InsightThroughLine,
			ThroughLine:       topicName,
			SourceArtifactIDs: artifactIDs,
			Confidence:        math.Min(1.0, math.Log2(float64(count))/5.0),
			CreatedAt:         time.Now(),
		})
	}

	if err := rows.Err(); err != nil {
		return insights, fmt.Errorf("synthesis row iteration: %w", err)
	}

	return insights, nil
}

// CreateAlert creates a new contextual alert.
func (e *Engine) CreateAlert(ctx context.Context, alert *Alert) error {
	if alert.Title == "" {
		return fmt.Errorf("alert title is required")
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
	_, err := e.Pool.Exec(ctx, `
		UPDATE alerts SET status = 'dismissed' WHERE id = $1
	`, alertID)
	return err
}

// SnoozeAlert snoozes an alert until a given time.
func (e *Engine) SnoozeAlert(ctx context.Context, alertID string, until time.Time) error {
	_, err := e.Pool.Exec(ctx, `
		UPDATE alerts SET status = 'snoozed', snooze_until = $2 WHERE id = $1
	`, alertID, until)
	return err
}

// GetPendingAlerts returns alerts ready for delivery (max 2/day).
func (e *Engine) GetPendingAlerts(ctx context.Context) ([]Alert, error) {
	// Single query: compute remaining delivery slots and fetch pending alerts in one round-trip
	rows, err := e.Pool.Query(ctx, `
		SELECT id, alert_type, title, body, priority, status, artifact_id, created_at
		FROM alerts
		WHERE status = 'pending'
		   OR (status = 'snoozed' AND snooze_until <= NOW())
		ORDER BY priority, created_at
		LIMIT GREATEST(0, 2 - (
			SELECT COUNT(*) FROM alerts
			WHERE status = 'delivered' AND delivered_at >= CURRENT_DATE
		))
	`)
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

	rows, err := e.Pool.Query(ctx, `
		SELECT ai.id, ai.text, ai.expected_date, COALESCE(p.name, 'unknown')
		FROM action_items ai
		LEFT JOIN people p ON p.id = ai.person_id
		WHERE ai.status = 'open' AND ai.expected_date < CURRENT_DATE
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

		daysOverdue := int(time.Since(expectedDate).Hours() / 24)
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
