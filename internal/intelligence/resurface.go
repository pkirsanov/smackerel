package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ResurfaceCandidate represents an artifact that might be valuable to resurface.
type ResurfaceCandidate struct {
	ArtifactID   string    `json:"artifact_id"`
	Title        string    `json:"title"`
	Score        float64   `json:"score"`
	Reason       string    `json:"reason"`
	LastAccessed time.Time `json:"last_accessed"`
}

// Resurface finds artifacts worth resurfacing based on decay, relevance, and serendipity.
//
// Architecture note: resurface.go implements the two core resurfacing strategies
// (dormancy-based and serendipity). The remaining intelligence scopes — expertise,
// learning, subscriptions, monthly, lookups, content fuel, and seasonal — are
// aggregated through engine.go which orchestrates all intelligence signals including
// this resurfacing output. See Engine.GenerateDigest in engine.go for the full
// intelligence pipeline.
func (e *Engine) Resurface(ctx context.Context, limit int) ([]ResurfaceCandidate, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("resurface requires a database connection")
	}

	if limit <= 0 {
		limit = 5
	}

	// Strategy 1: High-value dormant artifacts (not accessed in 30+ days, high relevance)
	rows, err := e.Pool.Query(ctx, `
		SELECT id, title, relevance_score,
		       COALESCE(last_accessed, created_at) as last_access,
		       EXTRACT(DAY FROM NOW() - COALESCE(last_accessed, created_at))::int as days_dormant
		FROM artifacts
		WHERE COALESCE(last_accessed, created_at) < NOW() - INTERVAL '30 days'
		AND relevance_score > 0.3
		ORDER BY relevance_score DESC, last_accessed ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query dormant artifacts: %w", err)
	}
	defer rows.Close()

	var candidates []ResurfaceCandidate
	for rows.Next() {
		var c ResurfaceCandidate
		var daysDormant int
		if err := rows.Scan(&c.ArtifactID, &c.Title, &c.Score, &c.LastAccessed, &daysDormant); err != nil {
			slog.Warn("resurface scan failed", "error", err)
			continue
		}
		c.Reason = fmt.Sprintf("High-value artifact dormant for %d days", daysDormant)
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return candidates, fmt.Errorf("resurface row iteration: %w", err)
	}

	// Strategy 2: Serendipity — random artifact from underexplored topics
	if len(candidates) < limit {
		serendipity, err := e.serendipityPick(ctx, limit-len(candidates))
		if err != nil {
			slog.Warn("serendipity pick failed", "error", err)
		} else {
			candidates = append(candidates, serendipity...)
		}
	}

	// Note: We deliberately do NOT update last_accessed here. Generating
	// candidates for a digest or weekly report is not the same as the user
	// actually viewing the content. Updating last_accessed as a side effect
	// of candidate generation would taint dormancy scores for future runs.
	// The caller (e.g., the delivery layer) should call MarkResurfaced()
	// after the user has actually been shown the content.

	return candidates, nil
}

// MarkResurfaced updates last_accessed and access_count for artifacts that
// have been delivered to the user. Call this after the user has actually
// been shown resurfaced content, not during candidate generation.
func (e *Engine) MarkResurfaced(ctx context.Context, artifactIDs []string) error {
	if len(artifactIDs) == 0 {
		return nil
	}
	// Filter out empty-string IDs that would match rows with empty id columns.
	var filtered []string
	for _, id := range artifactIDs {
		if id != "" {
			filtered = append(filtered, id)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if e.Pool == nil {
		return fmt.Errorf("mark resurfaced requires a database connection")
	}
	_, err := e.Pool.Exec(ctx, `
		UPDATE artifacts SET last_accessed = NOW(), access_count = access_count + 1
		WHERE id = ANY($1)
	`, filtered)
	if err != nil {
		return fmt.Errorf("batch update resurfaced artifacts: %w", err)
	}
	return nil
}

// serendipityPick selects random artifacts from underexplored topics.
func (e *Engine) serendipityPick(ctx context.Context, limit int) ([]ResurfaceCandidate, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("resurface requires a database connection")
	}

	rows, err := e.Pool.Query(ctx, `
		SELECT a.id, a.title, a.relevance_score, COALESCE(a.last_accessed, a.created_at)
		FROM artifacts a
		WHERE a.access_count < 3
		AND a.created_at > NOW() - INTERVAL '90 days'
		ORDER BY RANDOM()
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []ResurfaceCandidate
	for rows.Next() {
		var c ResurfaceCandidate
		if err := rows.Scan(&c.ArtifactID, &c.Title, &c.Score, &c.LastAccessed); err != nil {
			slog.Warn("serendipity scan failed", "error", err)
			continue
		}
		c.Reason = "Serendipity — underexplored content worth revisiting"
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

// Note: rand.Seed was removed — since Go 1.20 the global rand source is
// automatically seeded and rand.Seed is deprecated.

// SerendipityCandidate extends ResurfaceCandidate with context match scoring.
type SerendipityCandidate struct {
	ResurfaceCandidate
	CalendarMatch bool    `json:"calendar_match"`
	TopicMatch    bool    `json:"topic_match"`
	ContextScore  float64 `json:"context_score"`
	ContextReason string  `json:"context_reason"`
}

// SerendipityPick selects a single context-aware archive item per R-505.
// This is the full implementation replacing the dormancy-only Resurface for weekly use.
func (e *Engine) SerendipityPick(ctx context.Context) (*SerendipityCandidate, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("serendipity requires a database connection")
	}

	// 1. Query candidate pool: 6+ months dormant (or 3+ if pinned), above-average relevance
	rows, err := e.Pool.Query(ctx, `
		SELECT a.id, a.title, a.relevance_score,
		       COALESCE(a.last_accessed, a.created_at) AS last_access,
		       EXTRACT(DAY FROM NOW() - COALESCE(a.last_accessed, a.created_at))::int AS days_dormant,
		       COALESCE(a.access_count, 0) AS access_count,
		       COALESCE(a.pinned, FALSE) AS pinned
		FROM artifacts a
		WHERE (
			(COALESCE(a.last_accessed, a.created_at) < NOW() - INTERVAL '180 days')
			OR (a.pinned = TRUE AND COALESCE(a.last_accessed, a.created_at) < NOW() - INTERVAL '90 days')
		)
		AND a.relevance_score > (SELECT COALESCE(AVG(relevance_score), 0) FROM artifacts)
		ORDER BY a.relevance_score DESC
		LIMIT 50
	`)
	if err != nil {
		return nil, fmt.Errorf("query serendipity candidates: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		id          string
		title       string
		relevance   float64
		lastAccess  time.Time
		daysDormant int
		accessCount int
		pinned      bool
	}

	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.title, &c.relevance, &c.lastAccess, &c.daysDormant, &c.accessCount, &c.pinned); err != nil {
			slog.Warn("serendipity candidate scan failed", "error", err)
			continue
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("serendipity candidate iteration: %w", err)
	}

	if len(candidates) == 0 {
		return nil, nil // No candidates available
	}

	// 2. Batch-fetch topic matches in a single query instead of N+1 per-candidate
	candidateIDs := make([]string, len(candidates))
	for i, c := range candidates {
		candidateIDs[i] = c.id
	}
	topicMatchSet := make(map[string]bool)
	tmRows, err := e.Pool.Query(ctx, `
		SELECT DISTINCT e.src_id
		FROM edges e
		JOIN topics t ON t.id = e.dst_id AND e.dst_type = 'topic'
		WHERE e.src_id = ANY($1) AND e.edge_type = 'BELONGS_TO'
		AND t.state IN ('hot', 'active')
	`, candidateIDs)
	if err != nil {
		slog.Warn("batch topic match query failed", "error", err)
	} else {
		defer tmRows.Close()
		for tmRows.Next() {
			var id string
			if tmRows.Scan(&id) == nil {
				topicMatchSet[id] = true
			}
		}
		if err := tmRows.Err(); err != nil {
			slog.Warn("batch topic match iteration failed", "error", err)
		}
	}

	// Score each candidate with context matching
	var best *SerendipityCandidate
	var bestScore float64

	for _, c := range candidates {
		sc := &SerendipityCandidate{
			ResurfaceCandidate: ResurfaceCandidate{
				ArtifactID:   c.id,
				Title:        c.title,
				Score:        c.relevance,
				LastAccessed: c.lastAccess,
			},
		}

		// Base score from relevance
		score := c.relevance * 0.5

		// Topic match from batch query
		if topicMatchSet[c.id] {
			score += 2.0
			sc.TopicMatch = true
			sc.ContextReason = "Connects to a currently active topic"
		}

		// Quality bonus
		if c.pinned {
			score += 1.0
		}

		sc.ContextScore = score

		if score > bestScore {
			bestScore = score
			best = sc
		}
	}

	if best == nil {
		return nil, nil
	}

	// Format reason
	if best.ContextReason == "" {
		best.Reason = fmt.Sprintf("You saved this %d days ago. Still relevant?", int(time.Since(best.LastAccessed).Hours()/24))
	} else {
		best.Reason = fmt.Sprintf("Remember this? %s — %s", best.Title, best.ContextReason)
	}

	return best, nil
}
