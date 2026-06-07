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

	// cooling carries the LLM evaluator + operational bounds for the
	// relationship-cooling producer (BUG-021-005). Nil until wired by
	// cmd/core after the agent bridge is constructed; a nil cooling config
	// disables cooling-alert production (there is NO hardcoded fallback
	// heuristic — the judgment is the LLM's, not magic numbers in Go).
	cooling *CoolingConfig

	// alertTiming carries the LLM evaluator + operational bounds for the
	// timing-judged producers (bill / trip-prep / return-window, BUG-021-006).
	// Nil until wired; a nil config disables those producers (no hardcoded
	// alert-timing window fallback — the "alert now?" judgment is the LLM's).
	alertTiming *AlertTimingConfig

	// resurface carries the LLM evaluator + operational bounds for the
	// dormancy-based resurfacing strategy (BUG-021-007). Nil until wired; a
	// nil config disables the dormancy strategy (no hardcoded dormancy /
	// relevance threshold fallback — the "worth resurfacing?" judgment is the
	// LLM's). Serendipity (random discovery) is unaffected.
	resurface *ResurfaceConfig

	// expertise carries the LLM evaluator + operational bounds for the
	// expertise map (BUG-021-008). Nil until wired; a nil config makes the
	// expertise endpoint fail loud (no hardcoded weighted score or numeric
	// tier/velocity threshold fallback — the tier/growth judgment is the
	// LLM's).
	expertise *ExpertiseConfig
}

// NewEngine creates a new intelligence engine.
func NewEngine(pool *pgxpool.Pool, nats *smacknats.Client) *Engine {
	return &Engine{Pool: pool, NATS: nats}
}

// SetCoolingConfig injects the LLM-driven relationship-cooling evaluator and
// its operational bounds. Called by cmd/core wiring after the agent bridge is
// available. Passing nil (or a config with a nil Evaluator) leaves cooling
// alert production disabled.
func (e *Engine) SetCoolingConfig(c *CoolingConfig) {
	e.cooling = c
}

// SetResurfaceConfig injects the LLM-driven resurfacing evaluator and its
// operational bounds for the dormancy strategy. Passing nil (or a config with
// a nil Evaluator) leaves the dormancy strategy disabled (no hardcoded
// dormancy/relevance threshold fallback); serendipity is unaffected.
func (e *Engine) SetResurfaceConfig(c *ResurfaceConfig) {
	e.resurface = c
}

// SetExpertiseConfig injects the LLM-driven expertise evaluator and its
// operational bounds for the expertise map. Passing nil (or a config with a
// nil Evaluator) leaves the expertise endpoint failing loud (no hardcoded tier
// fallback — the tier/growth judgment is the LLM's).
func (e *Engine) SetExpertiseConfig(c *ExpertiseConfig) {
	e.expertise = c
}

// SetAlertTimingConfig injects the LLM-driven alert-timing evaluator and its
// operational bounds for the bill / trip-prep / return-window producers.
// Passing nil (or a config with a nil Evaluator) leaves those producers
// disabled (no hardcoded alert-timing window fallback).
func (e *Engine) SetAlertTimingConfig(c *AlertTimingConfig) {
	e.alertTiming = c
}
