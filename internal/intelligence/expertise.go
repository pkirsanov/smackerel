package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"
)

// ExpertiseTier represents the user's expertise level in a topic.
type ExpertiseTier string

const (
	TierNovice       ExpertiseTier = "novice"
	TierFoundation   ExpertiseTier = "foundation"
	TierIntermediate ExpertiseTier = "intermediate"
	TierDeep         ExpertiseTier = "deep"
	TierExpert       ExpertiseTier = "expert"
)

// GrowthTrajectory represents how a topic's capture velocity is changing.
type GrowthTrajectory string

const (
	TrajectoryAccelerating GrowthTrajectory = "accelerating"
	TrajectorySteady       GrowthTrajectory = "steady"
	TrajectoryDecelerating GrowthTrajectory = "decelerating"
	TrajectoryStopped      GrowthTrajectory = "stopped"
)

// TopicExpertise represents the expertise assessment for a single topic.
type TopicExpertise struct {
	TopicID           string           `json:"topic_id"`
	TopicName         string           `json:"topic_name"`
	CaptureCount      int              `json:"capture_count"`
	SourceDiversity   int              `json:"source_diversity"`
	DepthRatio        float64          `json:"depth_ratio"`
	Engagement        int              `json:"engagement"`
	ConnectionDensity float64          `json:"connection_density"`
	Tier              ExpertiseTier    `json:"tier"`
	Growth            GrowthTrajectory `json:"growth"`
	RecentCaptures    int              `json:"recent_captures_30d"`
	AvgMonthly        float64          `json:"avg_monthly_captures"`
}

// BlindSpot represents a topic that is referenced but under-captured.
type BlindSpot struct {
	TopicName    string `json:"topic_name"`
	MentionCount int    `json:"mention_count"`
	CaptureCount int    `json:"capture_count"`
	Gap          int    `json:"gap"`
}

// ExpertiseMap is the full expertise assessment for the user.
type ExpertiseMap struct {
	Topics      []TopicExpertise `json:"topics"`
	BlindSpots  []BlindSpot      `json:"blind_spots"`
	TotalTopics int              `json:"total_topics"`
	DataDays    int              `json:"data_days"`
	Mature      bool             `json:"mature"`
	GeneratedAt time.Time        `json:"generated_at"`
}

// GenerateExpertiseMap computes the user's expertise map per R-501.
func (e *Engine) GenerateExpertiseMap(ctx context.Context) (*ExpertiseMap, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("expertise mapping requires a database connection")
	}
	if e.expertise == nil || e.expertise.Evaluator == nil {
		// BUG-021-008: tier/growth are LLM-judged; there is NO hardcoded
		// fallback. With no evaluator wired the map cannot be classified, so
		// fail loud rather than emit bogus tiers.
		return nil, fmt.Errorf("expertise mapping requires the LLM evaluator (agent bridge not wired)")
	}
	cfg := e.expertise

	// Check data maturity against the operational data-sufficiency floor.
	var dataDays int
	err := e.Pool.QueryRow(ctx, `
		SELECT COALESCE(EXTRACT(DAY FROM NOW() - MIN(created_at))::int, 0) FROM artifacts
	`).Scan(&dataDays)
	if err != nil {
		return nil, fmt.Errorf("check data maturity: %w", err)
	}

	result := &ExpertiseMap{
		DataDays:    dataDays,
		Mature:      dataDays >= cfg.MaturityDays,
		GeneratedAt: time.Now(),
	}

	// Query topic expertise dimensions
	rows, err := e.Pool.Query(ctx, `
		WITH topic_artifacts AS (
			SELECT
				t.id AS topic_id,
				t.name AS topic_name,
				COUNT(DISTINCT a.id) AS capture_count,
				COUNT(DISTINCT a.source_id) AS source_diversity,
				COUNT(DISTINCT CASE WHEN a.processing_tier = 'full' THEN a.id END)::float
					/ NULLIF(COUNT(DISTINCT a.id), 0) AS depth_ratio,
				COALESCE(SUM(a.access_count), 0) AS engagement,
				COUNT(DISTINCT CASE WHEN a.created_at > NOW() - INTERVAL '30 days' THEN a.id END) AS recent_30d,
				EXTRACT(MONTH FROM AGE(NOW(), MIN(a.created_at))) + 1 AS months_active
			FROM topics t
			JOIN edges e ON e.dst_id = t.id AND e.dst_type = 'topic' AND e.edge_type = 'BELONGS_TO'
			JOIN artifacts a ON a.id = e.src_id AND e.src_type = 'artifact'
			GROUP BY t.id, t.name
			HAVING COUNT(DISTINCT a.id) >= 1
		),
		topic_connections AS (
			SELECT
				ta.topic_id,
				COUNT(DISTINCT e2.id)::float / NULLIF(ta.capture_count, 0) AS connection_density
			FROM topic_artifacts ta
			JOIN edges e1 ON e1.dst_id = ta.topic_id AND e1.dst_type = 'topic' AND e1.edge_type = 'BELONGS_TO'
			LEFT JOIN edges e2 ON e2.src_id = e1.src_id AND e2.edge_type != 'BELONGS_TO'
			GROUP BY ta.topic_id, ta.capture_count
		)
		SELECT
			ta.topic_id, ta.topic_name, ta.capture_count, ta.source_diversity,
			COALESCE(ta.depth_ratio, 0), ta.engagement,
			COALESCE(tc.connection_density, 0),
			ta.recent_30d, ta.months_active
		FROM topic_artifacts ta
		LEFT JOIN topic_connections tc ON tc.topic_id = ta.topic_id
		ORDER BY ta.capture_count DESC
		LIMIT $1
	`, cfg.MaxTopics)
	if err != nil {
		return nil, fmt.Errorf("query expertise dimensions: %w", err)
	}
	defer rows.Close()

	// Gather each topic's deterministic signals; the tier + growth JUDGMENT is
	// made by the LLM, not by any Go threshold. Ref is the positional key the
	// model echoes back so classifications map to the right topic.
	var signals []ExpertiseSignals
	for rows.Next() {
		var te TopicExpertise
		var monthsActive float64
		if err := rows.Scan(
			&te.TopicID, &te.TopicName, &te.CaptureCount, &te.SourceDiversity,
			&te.DepthRatio, &te.Engagement, &te.ConnectionDensity,
			&te.RecentCaptures, &monthsActive,
		); err != nil {
			slog.Warn("expertise scan failed", "error", err)
			continue
		}

		te.AvgMonthly = float64(te.CaptureCount) / math.Max(monthsActive, 1)

		ref := len(result.Topics)
		signals = append(signals, ExpertiseSignals{
			TopicID:           te.TopicID,
			Ref:               ref,
			TopicName:         te.TopicName,
			CaptureCount:      te.CaptureCount,
			SourceDiversity:   te.SourceDiversity,
			DepthRatio:        te.DepthRatio,
			Engagement:        te.Engagement,
			ConnectionDensity: te.ConnectionDensity,
			RecentCaptures:    te.RecentCaptures,
			AvgMonthly:        te.AvgMonthly,
		})
		result.Topics = append(result.Topics, te)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("expertise row iteration: %w", err)
	}
	result.TotalTopics = len(result.Topics)

	// LLM-driven classification: ONE batched call assigns a tier + growth per
	// topic (docs/smackerel.md §3.6) — no hardcoded weighted score or numeric
	// tier/velocity threshold. The model reasons comparatively across the whole
	// graph in a single request (this is an on-demand endpoint).
	if len(signals) > 0 {
		classifications, err := cfg.Evaluator.ClassifyExpertise(ctx, dataDays, signals)
		if err != nil {
			return nil, fmt.Errorf("expertise classification: %w", err)
		}
		for _, c := range classifications {
			if c.Ref < 0 || c.Ref >= len(result.Topics) {
				slog.Warn("expertise classification ref out of range", "ref", c.Ref, "topics", len(result.Topics))
				continue
			}
			result.Topics[c.Ref].Tier = ExpertiseTier(c.Tier)
			result.Topics[c.Ref].Growth = GrowthTrajectory(c.Growth)
		}
	}

	// Blind spot detection (operational gap-detection bounds, SST).
	blindSpots, err := e.detectBlindSpots(ctx, cfg)
	if err != nil {
		slog.Warn("blind spot detection failed", "error", err)
	} else {
		result.BlindSpots = blindSpots
	}

	return result, nil
}

// detectBlindSpots finds topics that are mentioned but under-captured. The
// gap-detection bounds (min mentions, max captures, result limit) are
// OPERATIONAL SST values, not business thresholds.
func (e *Engine) detectBlindSpots(ctx context.Context, cfg *ExpertiseConfig) ([]BlindSpot, error) {
	rows, err := e.Pool.Query(ctx, `
		WITH mentioned_topics AS (
			SELECT t.name, COUNT(DISTINCT e.src_id) AS mention_count
			FROM topics t
			JOIN edges e ON e.dst_id = t.id AND e.dst_type = 'topic'
			GROUP BY t.name
		),
		captured_topics AS (
			SELECT t.name, COUNT(DISTINCT e.src_id) AS capture_count
			FROM topics t
			JOIN edges e ON e.dst_id = t.id AND e.dst_type = 'topic' AND e.edge_type = 'BELONGS_TO'
			GROUP BY t.name
		)
		SELECT mt.name, mt.mention_count, COALESCE(ct.capture_count, 0) AS capture_count
		FROM mentioned_topics mt
		LEFT JOIN captured_topics ct ON ct.name = mt.name
		WHERE COALESCE(ct.capture_count, 0) < $1
		  AND mt.mention_count > $2
		ORDER BY (mt.mention_count - COALESCE(ct.capture_count, 0)) DESC
		LIMIT $3
	`, cfg.BlindSpotMaxCaptures, cfg.BlindSpotMinMentions, cfg.BlindSpotLimit)
	if err != nil {
		return nil, fmt.Errorf("query blind spots: %w", err)
	}
	defer rows.Close()

	var spots []BlindSpot
	for rows.Next() {
		var bs BlindSpot
		if err := rows.Scan(&bs.TopicName, &bs.MentionCount, &bs.CaptureCount); err != nil {
			slog.Warn("blind spot scan failed", "error", err)
			continue
		}
		bs.Gap = bs.MentionCount - bs.CaptureCount
		spots = append(spots, bs)
	}
	return spots, rows.Err()
}
