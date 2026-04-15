package intelligence

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	smacknats "github.com/smackerel/smackerel/internal/nats"
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
