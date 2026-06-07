package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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

	// Strategy 1: LLM-judged dormant artifacts (BUG-021-007). The Go core
	// retrieves dormant CANDIDATES and their signals — within an OPERATIONAL
	// dormancy-retrieval floor (exclude freshly-accessed items) and a cap — and
	// the `resurface_evaluate` scenario decides, per candidate, whether the
	// artifact is genuinely worth resurfacing. There is NO hardcoded dormancy /
	// relevance threshold; when the evaluator is not wired, the dormancy
	// strategy is skipped (serendipity still fills the digest).
	var candidates []ResurfaceCandidate
	if e.resurface != nil && e.resurface.Evaluator != nil {
		dormant, err := e.gatherResurfaceCandidates(ctx)
		if err != nil {
			slog.Warn("resurface candidate retrieval failed", "error", err)
		} else {
			for _, sig := range dormant {
				if len(candidates) >= limit {
					break
				}
				if ctx.Err() != nil {
					break
				}
				decision, derr := e.resurface.Evaluator.EvaluateResurface(ctx, sig)
				if derr != nil {
					slog.Warn("resurface evaluation failed", "artifact", sig.ArtifactID, "error", derr)
					continue
				}
				if !resurfaceShouldSurface(decision, e.resurface.ConfidenceFloor) {
					continue
				}
				reason := decision.Reason
				if reason == "" {
					reason = fmt.Sprintf("A valuable item you haven't seen in %d days.", sig.DaysDormant)
				}
				candidates = append(candidates, ResurfaceCandidate{
					ArtifactID: sig.ArtifactID,
					Title:      sig.Title,
					Score:      sig.Relevance,
					Reason:     reason,
				})
			}
		}
	} else {
		slog.Warn("resurface dormancy strategy skipped: LLM evaluator not wired (no hardcoded fallback); serendipity only")
	}

	// Strategy 2: Serendipity — random artifact from underexplored topics.
	// This is intentionally non-deterministic rediscovery (not a worthiness
	// threshold), so it is NOT LLM-judged and is unaffected by BUG-021-007.
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

// gatherResurfaceCandidates retrieves dormant artifacts and their signals for
// LLM resurfacing-worthiness judgment (BUG-021-007). The only numbers it
// applies are OPERATIONAL: $1 = the dormancy-retrieval floor in days (exclude
// freshly-accessed items so the LLM judges genuinely dormant ones) and $2 = the
// per-run candidate cap (throughput). It applies NO business threshold for
// "worth resurfacing" — that judgment is the LLM's. Items are ordered
// most-dormant-first so the cap keeps the highest-signal candidates; unscored
// relevance is treated as 0.
func (e *Engine) gatherResurfaceCandidates(ctx context.Context) ([]ResurfaceSignals, error) {
	rows, err := e.Pool.Query(ctx, `
		SELECT id, title,
		       COALESCE(relevance_score, 0) AS relevance_score,
		       EXTRACT(DAY FROM NOW() - COALESCE(last_accessed, created_at))::int AS days_dormant,
		       COALESCE(access_count, 0) AS access_count
		FROM artifacts
		WHERE COALESCE(last_accessed, created_at) < NOW() - make_interval(days => $1)
		ORDER BY COALESCE(last_accessed, created_at) ASC, relevance_score DESC
		LIMIT $2
	`, e.resurface.MinDormancyDays, e.resurface.MaxCandidates)
	if err != nil {
		return nil, fmt.Errorf("query dormant artifacts: %w", err)
	}
	defer rows.Close()

	var out []ResurfaceSignals
	for rows.Next() {
		var s ResurfaceSignals
		if err := rows.Scan(&s.ArtifactID, &s.Title, &s.Relevance, &s.DaysDormant, &s.AccessCount); err != nil {
			slog.Warn("resurface candidate scan failed", "error", err)
			continue
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("resurface candidate iteration: %w", err)
	}
	return out, nil
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

	// 3. Batch-fetch calendar matches: upcoming events in next 7 days from CalDAV
	calendarKeywords := make(map[string]bool) // lowercase keywords from upcoming events
	calRows, err := e.Pool.Query(ctx, `
		SELECT LOWER(title)
		FROM artifacts
		WHERE source_id = 'caldav'
		AND created_at > NOW() - INTERVAL '30 days'
		AND (metadata->>'dtstart')::timestamptz BETWEEN NOW() AND NOW() + INTERVAL '7 days'
		LIMIT 50
	`)
	if err != nil {
		slog.Warn("calendar event query failed", "error", err)
	} else {
		defer calRows.Close()
		for calRows.Next() {
			var title string
			if calRows.Scan(&title) == nil {
				// Extract meaningful words from event titles
				for _, word := range strings.Fields(title) {
					if len(word) > 3 { // skip short words
						calendarKeywords[word] = true
					}
				}
			}
		}
		if err := calRows.Err(); err != nil {
			slog.Warn("calendar event iteration failed", "error", err)
		}
	}
	calendarMatchSet := make(map[string]bool)
	if len(calendarKeywords) > 0 {
		for _, c := range candidates {
			lower := strings.ToLower(c.title)
			for kw := range calendarKeywords {
				if strings.Contains(lower, kw) {
					calendarMatchSet[c.id] = true
					break
				}
			}
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

		// Calendar match from upcoming events (+3 per R-505)
		if calendarMatchSet[c.id] {
			score += 3.0
			sc.CalendarMatch = true
			sc.ContextReason = "Matches an upcoming calendar event"
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
