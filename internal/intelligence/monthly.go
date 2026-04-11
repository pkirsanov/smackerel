package intelligence

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// MonthlyReport is the monthly self-knowledge report per R-506.
type MonthlyReport struct {
	Month             string               `json:"month"`
	ExpertiseShifts   []ExpertiseShift     `json:"expertise_shifts"`
	InformationDiet   InformationDiet      `json:"information_diet"`
	InterestEvolution []InterestPeriod     `json:"interest_evolution"`
	ProductivityPats  []string             `json:"productivity_patterns"`
	SubscriptionSum   *SubscriptionSummary `json:"subscription_summary,omitempty"`
	LearningProgress  []LearningPath       `json:"learning_progress,omitempty"`
	TopInsights       []SynthesisInsight   `json:"top_insights"`
	SeasonalPatterns  []SeasonalPattern    `json:"seasonal_patterns,omitempty"`
	ReportText        string               `json:"report_text"`
	WordCount         int                  `json:"word_count"`
	GeneratedAt       time.Time            `json:"generated_at"`
}

// ExpertiseShift tracks a topic's depth change over the month.
type ExpertiseShift struct {
	TopicName    string  `json:"topic_name"`
	PrevDepth    float64 `json:"previous_depth"`
	CurrentDepth float64 `json:"current_depth"`
	Direction    string  `json:"direction"` // gained, lost, stable
}

// InformationDiet summarizes content type distribution.
type InformationDiet struct {
	Articles int `json:"articles"`
	Videos   int `json:"videos"`
	Emails   int `json:"emails"`
	Notes    int `json:"notes"`
	Other    int `json:"other"`
	Total    int `json:"total"`
}

// InterestPeriod shows topic distribution for a time period.
type InterestPeriod struct {
	Period    string   `json:"period"` // e.g., "Jan-Feb"
	TopTopics []string `json:"top_topics"`
}

// GenerateMonthlyReport assembles the monthly self-knowledge report per R-506.
func (e *Engine) GenerateMonthlyReport(ctx context.Context) (*MonthlyReport, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("monthly report requires a database connection")
	}

	report := &MonthlyReport{
		Month:       time.Now().Format("2006-01"),
		GeneratedAt: time.Now(),
	}

	// 1. Expertise shifts — topics that gained/lost depth this month
	shiftRows, err := e.Pool.Query(ctx, `
		WITH this_month AS (
			SELECT t.name, COUNT(DISTINCT a.id) AS captures
			FROM topics t
			JOIN edges e ON e.dst_id = t.id AND e.dst_type = 'topic' AND e.edge_type = 'BELONGS_TO'
			JOIN artifacts a ON a.id = e.src_id AND a.created_at > DATE_TRUNC('month', NOW())
			GROUP BY t.name
		),
		last_month AS (
			SELECT t.name, COUNT(DISTINCT a.id) AS captures
			FROM topics t
			JOIN edges e ON e.dst_id = t.id AND e.dst_type = 'topic' AND e.edge_type = 'BELONGS_TO'
			JOIN artifacts a ON a.id = e.src_id
			  AND a.created_at BETWEEN DATE_TRUNC('month', NOW()) - INTERVAL '1 month' AND DATE_TRUNC('month', NOW())
			GROUP BY t.name
		)
		SELECT COALESCE(tm.name, lm.name) AS topic,
		       COALESCE(lm.captures, 0) AS prev,
		       COALESCE(tm.captures, 0) AS curr
		FROM this_month tm
		FULL OUTER JOIN last_month lm ON tm.name = lm.name
		WHERE COALESCE(tm.captures, 0) != COALESCE(lm.captures, 0)
		ORDER BY ABS(COALESCE(tm.captures, 0) - COALESCE(lm.captures, 0)) DESC
		LIMIT 10
	`)
	if err == nil {
		defer shiftRows.Close()
		for shiftRows.Next() {
			var es ExpertiseShift
			var prev, curr int
			if shiftRows.Scan(&es.TopicName, &prev, &curr) == nil {
				es.PrevDepth = float64(prev)
				es.CurrentDepth = float64(curr)
				if curr > prev {
					es.Direction = "gained"
				} else if curr < prev {
					es.Direction = "lost"
				} else {
					es.Direction = "stable"
				}
				report.ExpertiseShifts = append(report.ExpertiseShifts, es)
			}
		}
		if err := shiftRows.Err(); err != nil {
			slog.Warn("expertise shifts row iteration failed", "error", err)
		}
	}

	// Check context between heavy operations to abort early on cancellation
	if ctx.Err() != nil {
		return report, ctx.Err()
	}

	// 2. Information diet — content types consumed this month (single query)
	var allCount int
	if err := e.Pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE content_type LIKE '%article%'),
			COUNT(*) FILTER (WHERE content_type LIKE '%youtube%'),
			COUNT(*) FILTER (WHERE source_id IN ('gmail','imap','outlook')),
			COUNT(*) FILTER (WHERE content_type LIKE '%note%'),
			COUNT(*)
		FROM artifacts WHERE created_at > DATE_TRUNC('month', NOW())
	`).Scan(
		&report.InformationDiet.Articles,
		&report.InformationDiet.Videos,
		&report.InformationDiet.Emails,
		&report.InformationDiet.Notes,
		&allCount,
	); err != nil {
		slog.Warn("failed to query information diet", "error", err)
	}
	categorized := report.InformationDiet.Articles + report.InformationDiet.Videos +
		report.InformationDiet.Emails + report.InformationDiet.Notes
	report.InformationDiet.Other = allCount - categorized
	report.InformationDiet.Total = allCount

	// 3. Interest evolution (last 6 months, bi-monthly periods — single query)
	if ctx.Err() != nil {
		return report, ctx.Err()
	}
	periodStart := time.Now().AddDate(0, -6, 0)
	evoRows, err := e.Pool.Query(ctx, `
		WITH period_topics AS (
			SELECT
				CASE
					WHEN a.created_at >= NOW() - INTERVAL '2 months' THEN 0
					WHEN a.created_at >= NOW() - INTERVAL '4 months' THEN 1
					ELSE 2
				END AS period_idx,
				t.name,
				COUNT(*) AS cnt
			FROM topics t
			JOIN edges e ON e.dst_id = t.id AND e.dst_type = 'topic' AND e.edge_type = 'BELONGS_TO'
			JOIN artifacts a ON a.id = e.src_id AND a.created_at >= $1
			GROUP BY period_idx, t.name
		),
		ranked AS (
			SELECT period_idx, name, cnt,
			       ROW_NUMBER() OVER (PARTITION BY period_idx ORDER BY cnt DESC) AS rn
			FROM period_topics
		)
		SELECT period_idx, name FROM ranked WHERE rn <= 3
		ORDER BY period_idx, rn
	`, periodStart)
	if err == nil {
		defer evoRows.Close()
		periodTopics := make(map[int][]string)
		for evoRows.Next() {
			var idx int
			var name string
			if evoRows.Scan(&idx, &name) == nil {
				periodTopics[idx] = append(periodTopics[idx], name)
			}
		}
		if err := evoRows.Err(); err != nil {
			slog.Warn("interest evolution row iteration failed", "error", err)
		}
		for i := 0; i < 3; i++ {
			if topics, ok := periodTopics[i]; ok && len(topics) > 0 {
				start := time.Now().AddDate(0, -(i+1)*2, 0)
				end := time.Now().AddDate(0, -i*2, 0)
				report.InterestEvolution = append(report.InterestEvolution, InterestPeriod{
					Period:    start.Format("Jan") + "-" + end.Format("Jan"),
					TopTopics: topics,
				})
			}
		}
	}

	if ctx.Err() != nil {
		return report, ctx.Err()
	}

	// 4. Productivity patterns (from weekly synthesis)
	report.ProductivityPats = e.detectCapturePatterns(ctx)

	// 5. Subscription summary
	subSummary, err := e.GetSubscriptionSummary(ctx)
	if err == nil {
		report.SubscriptionSum = subSummary
	}

	// 6. Learning progress
	paths, err := e.GetLearningPaths(ctx)
	if err == nil {
		report.LearningProgress = paths
	}

	if ctx.Err() != nil {
		return report, ctx.Err()
	}

	// 7. Top synthesis insights
	insights, err := e.RunSynthesis(ctx)
	if err == nil && len(insights) > 3 {
		insights = insights[:3]
	}
	report.TopInsights = insights

	// 8. Seasonal patterns (R-508 — requires 6+ months)
	seasonalPatterns, err := e.DetectSeasonalPatterns(ctx)
	if err != nil {
		slog.Warn("seasonal pattern detection failed", "error", err)
	} else if len(seasonalPatterns) > 0 {
		// Cap at 2 seasonal observations per monthly report per design
		if len(seasonalPatterns) > 2 {
			seasonalPatterns = seasonalPatterns[:2]
		}
		report.SeasonalPatterns = seasonalPatterns
	}

	// Assemble report text — try NATS LLM generation first, fall back to local
	if e.NATS != nil {
		data, err := json.Marshal(report)
		if err == nil {
			if pubErr := e.NATS.Publish(ctx, smacknats.SubjectMonthlyGenerate, data); pubErr != nil {
				slog.Warn("NATS monthly report publish failed, using local assembly", "error", pubErr)
			}
		}
	}
	report.ReportText = assembleMonthlyReportText(report)
	report.WordCount = len(strings.Fields(report.ReportText))

	return report, nil
}

func assembleMonthlyReportText(r *MonthlyReport) string {
	var sections []string
	sections = append(sections, fmt.Sprintf("MONTHLY KNOWLEDGE REPORT — %s", r.Month))

	// Expertise shifts
	if len(r.ExpertiseShifts) > 0 {
		var lines []string
		for _, es := range r.ExpertiseShifts {
			arrow := "→"
			if es.Direction == "gained" {
				arrow = "↑"
			} else if es.Direction == "lost" {
				arrow = "↓"
			}
			lines = append(lines, fmt.Sprintf("  %s %s (%.0f → %.0f)", arrow, es.TopicName, es.PrevDepth, es.CurrentDepth))
		}
		sections = append(sections, "EXPERTISE SHIFTS:\n"+strings.Join(lines, "\n"))
	}

	// Information diet
	d := r.InformationDiet
	if d.Total > 0 {
		sections = append(sections, fmt.Sprintf("INFORMATION DIET: %d articles, %d videos, %d emails, %d notes",
			d.Articles, d.Videos, d.Emails, d.Notes))
	}

	// Subscription summary
	if r.SubscriptionSum != nil && r.SubscriptionSum.MonthlyTotal > 0 {
		sections = append(sections, fmt.Sprintf("SUBSCRIPTIONS: $%.2f/month across %d active services",
			r.SubscriptionSum.MonthlyTotal, len(r.SubscriptionSum.Active)))
	}

	// Patterns
	if len(r.ProductivityPats) > 0 {
		sections = append(sections, "PATTERNS:\n"+strings.Join(r.ProductivityPats, "\n"))
	}

	// Seasonal patterns (R-508)
	if len(r.SeasonalPatterns) > 0 {
		var lines []string
		for _, sp := range r.SeasonalPatterns {
			lines = append(lines, fmt.Sprintf("  %s: %s", sp.Month, sp.Observation))
		}
		sections = append(sections, "SEASONAL INSIGHTS:\n"+strings.Join(lines, "\n"))
	}

	if len(sections) <= 1 {
		return "Not enough data for a meaningful monthly report yet."
	}

	return strings.Join(sections, "\n\n")
}

// ContentAngle represents a writing angle suggestion per R-503.
type ContentAngle struct {
	Title            string   `json:"title"`
	UniqueRationale  string   `json:"uniqueness_rationale"`
	SupportingIDs    []string `json:"supporting_artifact_ids"`
	FormatSuggestion string   `json:"format_suggestion"`
}

// GenerateContentFuel identifies original writing angles from deep topics per R-503.
func (e *Engine) GenerateContentFuel(ctx context.Context) ([]ContentAngle, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("content fuel requires a database connection")
	}

	// Find topics with 30+ captures
	rows, err := e.Pool.Query(ctx, `
		SELECT t.id, t.name, COUNT(DISTINCT a.id) AS cnt,
		       COUNT(DISTINCT a.source_id) AS source_diversity
		FROM topics t
		JOIN edges e ON e.dst_id = t.id AND e.dst_type = 'topic' AND e.edge_type = 'BELONGS_TO'
		JOIN artifacts a ON a.id = e.src_id
		GROUP BY t.id, t.name
		HAVING COUNT(DISTINCT a.id) >= 30
		ORDER BY cnt DESC
		LIMIT 5
	`)
	if err != nil {
		return nil, fmt.Errorf("query deep topics: %w", err)
	}
	defer rows.Close()

	var angles []ContentAngle
	for rows.Next() {
		var topicID, topicName string
		var captureCount, sourceDiversity int
		if err := rows.Scan(&topicID, &topicName, &captureCount, &sourceDiversity); err != nil {
			slog.Warn("content fuel scan failed", "error", err)
			continue
		}

		// Get supporting artifacts (highest relevance)
		if ctx.Err() != nil {
			return angles, ctx.Err()
		}

		var supportingIDs []string
		artRows, err := e.Pool.Query(ctx, `
			SELECT a.id FROM artifacts a
			JOIN edges e ON e.src_id = a.id AND e.dst_id = $1 AND e.edge_type = 'BELONGS_TO'
			ORDER BY a.relevance_score DESC
			LIMIT 5
		`, topicID)
		if err == nil {
			for artRows.Next() {
				var id string
				if artRows.Scan(&id) == nil {
					supportingIDs = append(supportingIDs, id)
				}
			}
			if iterErr := artRows.Err(); iterErr != nil {
				slog.Warn("content fuel supporting artifacts iteration failed", "topic", topicName, "error", iterErr)
			}
			artRows.Close()
		}

		// Publish to NATS for LLM-enhanced angle generation (R-503)
		if e.NATS != nil {
			payload := map[string]interface{}{
				"topic_id":         topicID,
				"topic_name":       topicName,
				"capture_count":    captureCount,
				"source_diversity": sourceDiversity,
				"supporting_ids":   supportingIDs,
			}
			if data, err := json.Marshal(payload); err == nil {
				if pubErr := e.NATS.Publish(ctx, smacknats.SubjectContentAnalyze, data); pubErr != nil {
					slog.Warn("NATS content analyze publish failed, using local generation", "topic", topicName, "error", pubErr)
				}
			}
		}

		// Local fallback angle generation
		format := "blog post"
		if captureCount > 100 {
			format = "long-form essay"
		} else if captureCount > 50 {
			format = "detailed guide"
		}

		angles = append(angles, ContentAngle{
			Title:            fmt.Sprintf("Deep dive: %s — %d sources over %d captures", topicName, sourceDiversity, captureCount),
			UniqueRationale:  fmt.Sprintf("You have %d captures from %d different sources about %s, giving you a multi-perspective view most writers lack", captureCount, sourceDiversity, topicName),
			SupportingIDs:    supportingIDs,
			FormatSuggestion: format,
		})
	}

	return angles, rows.Err()
}

// SeasonalPattern represents a detected seasonal behavior pattern per R-508.
type SeasonalPattern struct {
	Pattern     string `json:"pattern"`
	Month       string `json:"month"`
	Observation string `json:"observation"`
	Actionable  bool   `json:"actionable"`
}

// DetectSeasonalPatterns analyzes year-over-year capture behavior per R-508.
func (e *Engine) DetectSeasonalPatterns(ctx context.Context) ([]SeasonalPattern, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("seasonal patterns require a database connection")
	}

	// Check data maturity (need 6+ months)
	var dataDays int
	if err := e.Pool.QueryRow(ctx, `
		SELECT COALESCE(EXTRACT(DAY FROM NOW() - MIN(created_at))::int, 0) FROM artifacts
	`).Scan(&dataDays); err != nil {
		return nil, fmt.Errorf("check data maturity: %w", err)
	}

	if dataDays < 180 {
		return nil, nil // Not enough data
	}

	var patterns []SeasonalPattern

	// Volume pattern: compare current month to same month last year — single query
	var thisMonthCount, lastYearSameMonthCount int
	currentMonth := time.Now().Month()
	if err := e.Pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE EXTRACT(YEAR FROM created_at) = EXTRACT(YEAR FROM NOW())),
			COUNT(*) FILTER (WHERE EXTRACT(YEAR FROM created_at) = EXTRACT(YEAR FROM NOW()) - 1)
		FROM artifacts
		WHERE EXTRACT(MONTH FROM created_at) = $1
	`, int(currentMonth)).Scan(&thisMonthCount, &lastYearSameMonthCount); err != nil {
		slog.Warn("failed to query seasonal volume", "error", err)
	}

	if lastYearSameMonthCount > 0 {
		ratio := float64(thisMonthCount) / float64(lastYearSameMonthCount)
		if ratio < 0.7 {
			patterns = append(patterns, SeasonalPattern{
				Pattern:     "volume_drop",
				Month:       time.Now().Format("January"),
				Observation: fmt.Sprintf("Your capture volume is down %.0f%% compared to %s last year (%d vs %d)", (1-ratio)*100, time.Now().Format("January"), thisMonthCount, lastYearSameMonthCount),
				Actionable:  true,
			})
		} else if ratio > 1.5 {
			patterns = append(patterns, SeasonalPattern{
				Pattern:     "volume_spike",
				Month:       time.Now().Format("January"),
				Observation: fmt.Sprintf("Your capture volume is up %.0f%% compared to %s last year (%d vs %d)", (ratio-1)*100, time.Now().Format("January"), thisMonthCount, lastYearSameMonthCount),
				Actionable:  false,
			})
		}
	}

	// Topic seasonal pattern: topics that spike in the same month
	topicRows, err := e.Pool.Query(ctx, `
		SELECT t.name, COUNT(*) AS cnt
		FROM topics t
		JOIN edges e ON e.dst_id = t.id AND e.dst_type = 'topic' AND e.edge_type = 'BELONGS_TO'
		JOIN artifacts a ON a.id = e.src_id
		WHERE EXTRACT(MONTH FROM a.created_at) = $1
		GROUP BY t.name
		HAVING COUNT(*) >= 5
		ORDER BY cnt DESC
		LIMIT 5
	`, int(currentMonth))
	if err == nil {
		defer topicRows.Close()
		for topicRows.Next() {
			var name string
			var cnt int
			if topicRows.Scan(&name, &cnt) == nil {
				patterns = append(patterns, SeasonalPattern{
					Pattern:     "topic_seasonal",
					Month:       time.Now().Format("January"),
					Observation: fmt.Sprintf("%s tends to spike in %s (%d captures this month)", name, time.Now().Format("January"), cnt),
					Actionable:  false,
				})
			}
		}
		if err := topicRows.Err(); err != nil {
			slog.Warn("seasonal topic row iteration failed", "error", err)
		}
	}

	return patterns, nil
}
