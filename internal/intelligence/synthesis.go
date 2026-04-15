package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

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

	// CONNECTION DISCOVERED (R-302 §2: highest-value cross-domain insight)
	if len(ws.Insights) > 0 {
		var lines []string
		for _, i := range ws.Insights {
			lines = append(lines, fmt.Sprintf("• %s (confidence: %.0f%%)", i.ThroughLine, i.Confidence*100))
		}
		sections = append(sections, "CONNECTION DISCOVERED:\n"+strings.Join(lines, "\n"))
	}

	// TOPIC MOMENTUM (R-302 §3: rising, steady, declining with capture counts)
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
		sections = append(sections, "TOPIC MOMENTUM:\n"+strings.Join(lines, "\n"))
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
