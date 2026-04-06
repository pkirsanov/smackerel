package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"
)

// ResurfaceCandidate represents an artifact that might be valuable to resurface.
type ResurfaceCandidate struct {
	ArtifactID  string  `json:"artifact_id"`
	Title       string  `json:"title"`
	Score       float64 `json:"score"`
	Reason      string  `json:"reason"`
	LastAccessed time.Time `json:"last_accessed"`
}

// Resurface finds artifacts worth resurfacing based on decay, relevance, and serendipity.
func (e *Engine) Resurface(ctx context.Context, limit int) ([]ResurfaceCandidate, error) {
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
			continue
		}
		c.Reason = fmt.Sprintf("High-value artifact dormant for %d days", daysDormant)
		candidates = append(candidates, c)
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

	// Update last_accessed for resurfaced artifacts
	for _, c := range candidates {
		_, _ = e.Pool.Exec(ctx, `
			UPDATE artifacts SET last_accessed = NOW(), access_count = access_count + 1 WHERE id = $1
		`, c.ArtifactID)
	}

	return candidates, nil
}

// serendipityPick selects random artifacts from underexplored topics.
func (e *Engine) serendipityPick(ctx context.Context, limit int) ([]ResurfaceCandidate, error) {
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
			continue
		}
		c.Reason = "Serendipity — underexplored content worth revisiting"
		candidates = append(candidates, c)
	}
	return candidates, nil
}

// ResurfaceScore combines signals to compute a resurfacing priority.
func ResurfaceScore(relevanceScore float64, daysDormant int, accessCount int) float64 {
	// Higher relevance = more worth resurfacing
	// More dormant = more worth resurfacing (up to a point)
	// Low access count = more worth resurfacing
	dormancyBonus := 0.0
	if daysDormant > 30 {
		dormancyBonus = float64(daysDormant-30) * 0.01
		if dormancyBonus > 1.0 {
			dormancyBonus = 1.0
		}
	}

	accessPenalty := float64(accessCount) * 0.1
	if accessPenalty > 1.0 {
		accessPenalty = 1.0
	}

	return (relevanceScore + dormancyBonus) * (1.0 - accessPenalty)
}

// init seeds random for serendipity picks
func init() {
	rand.Seed(time.Now().UnixNano())
}
