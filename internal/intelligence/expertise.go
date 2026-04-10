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
	DepthScore        float64          `json:"depth_score"`
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

	// Check data maturity (need 90+ days)
	var dataDays int
	err := e.Pool.QueryRow(ctx, `
		SELECT COALESCE(EXTRACT(DAY FROM NOW() - MIN(created_at))::int, 0) FROM artifacts
	`).Scan(&dataDays)
	if err != nil {
		return nil, fmt.Errorf("check data maturity: %w", err)
	}

	result := &ExpertiseMap{
		DataDays:    dataDays,
		Mature:      dataDays >= 90,
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
	`)
	if err != nil {
		return nil, fmt.Errorf("query expertise dimensions: %w", err)
	}
	defer rows.Close()

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
		te.DepthScore = computeDepthScore(te)
		te.Tier = assignTier(te.CaptureCount, te.DepthScore)
		te.Growth = computeTrajectory(te.RecentCaptures, te.AvgMonthly)

		result.Topics = append(result.Topics, te)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("expertise row iteration: %w", err)
	}
	result.TotalTopics = len(result.Topics)

	// Blind spot detection
	blindSpots, err := e.detectBlindSpots(ctx)
	if err != nil {
		slog.Warn("blind spot detection failed", "error", err)
	} else {
		result.BlindSpots = blindSpots
	}

	return result, nil
}

// computeDepthScore calculates the composite depth score per design spec.
func computeDepthScore(te TopicExpertise) float64 {
	return float64(te.CaptureCount)*0.3 +
		float64(te.SourceDiversity)*15.0 +
		te.DepthRatio*20.0 +
		float64(te.Engagement)*0.1 +
		te.ConnectionDensity*10.0
}

// assignTier maps capture count and depth score to an expertise tier.
func assignTier(captureCount int, depthScore float64) ExpertiseTier {
	switch {
	case captureCount > 100 && depthScore > 90:
		return TierExpert
	case captureCount > 50 && depthScore > 60:
		return TierDeep
	case captureCount > 20 && depthScore > 30:
		return TierIntermediate
	case captureCount > 5 && depthScore > 10:
		return TierFoundation
	default:
		return TierNovice
	}
}

// computeTrajectory determines the growth direction of a topic.
func computeTrajectory(recent30d int, avgMonthly float64) GrowthTrajectory {
	if avgMonthly <= 0 {
		if recent30d > 0 {
			return TrajectoryAccelerating
		}
		return TrajectoryStopped
	}
	velocity := float64(recent30d) / avgMonthly
	switch {
	case velocity > 1.5:
		return TrajectoryAccelerating
	case velocity >= 0.7:
		return TrajectorySteady
	case velocity >= 0.3:
		return TrajectoryDecelerating
	default:
		return TrajectoryStopped
	}
}

// detectBlindSpots finds topics that are mentioned but under-captured.
func (e *Engine) detectBlindSpots(ctx context.Context) ([]BlindSpot, error) {
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
		WHERE COALESCE(ct.capture_count, 0) < 5
		  AND mt.mention_count > 5
		ORDER BY (mt.mention_count - COALESCE(ct.capture_count, 0)) DESC
		LIMIT 10
	`)
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
