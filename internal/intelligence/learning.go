package intelligence

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

// LearningDifficulty represents the difficulty level of a resource.
type LearningDifficulty string

const (
	DifficultyBeginner     LearningDifficulty = "beginner"
	DifficultyIntermediate LearningDifficulty = "intermediate"
	DifficultyAdvanced     LearningDifficulty = "advanced"
)

// LearningResource represents a single resource in a learning path.
type LearningResource struct {
	ArtifactID  string             `json:"artifact_id"`
	Title       string             `json:"title"`
	ContentType string             `json:"content_type"`
	Position    int                `json:"position"`
	Difficulty  LearningDifficulty `json:"difficulty"`
	Completed   bool               `json:"completed"`
	CompletedAt *time.Time         `json:"completed_at,omitempty"`
}

// LearningPath represents an auto-assembled learning path for a topic.
type LearningPath struct {
	TopicID        string             `json:"topic_id"`
	TopicName      string             `json:"topic_name"`
	Resources      []LearningResource `json:"resources"`
	TotalCount     int                `json:"total_count"`
	CompletedCount int                `json:"completed_count"`
	Gaps           []string           `json:"gaps"`
	GeneratedAt    time.Time          `json:"generated_at"`
}

// GetLearningPaths returns all topics with 5+ learning-type artifacts per R-502.
func (e *Engine) GetLearningPaths(ctx context.Context) ([]LearningPath, error) {
	if e.Pool == nil {
		return nil, fmt.Errorf("learning paths require a database connection")
	}

	// Find topics with 5+ educational artifacts
	rows, err := e.Pool.Query(ctx, `
		WITH topic_resources AS (
			SELECT
				t.id AS topic_id,
				t.name AS topic_name,
				a.id AS artifact_id,
				a.title,
				a.content_type,
				a.processing_tier,
				COALESCE(lp.position, 0) AS position,
				COALESCE(lp.difficulty, '') AS difficulty,
				COALESCE(lp.completed, FALSE) AS completed,
				lp.completed_at
			FROM topics t
			JOIN edges e ON e.dst_id = t.id AND e.dst_type = 'topic' AND e.edge_type = 'BELONGS_TO'
			JOIN artifacts a ON a.id = e.src_id AND e.src_type = 'artifact'
			LEFT JOIN learning_progress lp ON lp.topic_id = t.id AND lp.artifact_id = a.id
			WHERE a.content_type IN ('article', 'youtube', 'url', 'note/text', 'pdf')
		)
		SELECT topic_id, topic_name, artifact_id, title, content_type,
		       position, difficulty, completed, completed_at
		FROM topic_resources
		WHERE topic_id IN (
			SELECT topic_id FROM topic_resources GROUP BY topic_id HAVING COUNT(*) >= 5
		)
		ORDER BY topic_id, position, title
	`)
	if err != nil {
		return nil, fmt.Errorf("query learning resources: %w", err)
	}
	defer rows.Close()

	pathMap := make(map[string]*LearningPath)
	for rows.Next() {
		var topicID, topicName, artifactID, title, contentType string
		var position int
		var difficulty string
		var completed bool
		var completedAt *time.Time

		if err := rows.Scan(&topicID, &topicName, &artifactID, &title, &contentType,
			&position, &difficulty, &completed, &completedAt); err != nil {
			slog.Warn("learning path scan failed", "error", err)
			continue
		}

		path, exists := pathMap[topicID]
		if !exists {
			path = &LearningPath{
				TopicID:     topicID,
				TopicName:   topicName,
				GeneratedAt: time.Now(),
			}
			pathMap[topicID] = path
		}

		diff := classifyDifficultyHeuristic(title, contentType, position)
		if difficulty != "" {
			diff = LearningDifficulty(difficulty)
		}

		resource := LearningResource{
			ArtifactID:  artifactID,
			Title:       title,
			ContentType: contentType,
			Position:    position,
			Difficulty:  diff,
			Completed:   completed,
			CompletedAt: completedAt,
		}

		path.Resources = append(path.Resources, resource)
		path.TotalCount++
		if completed {
			path.CompletedCount++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("learning path row iteration: %w", err)
	}

	var paths []LearningPath
	for _, path := range pathMap {
		// Sort resources by difficulty per R-502: foundational concepts first,
		// then progressive complexity. Uses stable sort to preserve title order
		// within the same difficulty level.
		sort.SliceStable(path.Resources, func(i, j int) bool {
			return difficultyOrder(path.Resources[i].Difficulty) < difficultyOrder(path.Resources[j].Difficulty)
		})
		path.Gaps = detectGaps(path.Resources)
		paths = append(paths, *path)
	}

	// Sort for deterministic output — map iteration order is non-deterministic.
	sort.Slice(paths, func(i, j int) bool {
		return paths[i].TopicID < paths[j].TopicID
	})

	return paths, nil
}

// MarkLearningResourceCompleted marks a resource as completed in a learning path.
func (e *Engine) MarkLearningResourceCompleted(ctx context.Context, topicID, artifactID string) error {
	if e.Pool == nil {
		return fmt.Errorf("learning progress requires a database connection")
	}

	now := time.Now()
	_, err := e.Pool.Exec(ctx, `
		INSERT INTO learning_progress (id, topic_id, artifact_id, completed, completed_at, created_at)
		VALUES ($1, $2, $3, TRUE, $4, $4)
		ON CONFLICT (topic_id, artifact_id) DO UPDATE SET completed = TRUE, completed_at = $4
	`, ulid.Make().String(), topicID, artifactID, now)
	return err
}

// classifyDifficultyHeuristic assigns difficulty based on heuristics when LLM classification is unavailable.
func classifyDifficultyHeuristic(title, contentType string, position int) LearningDifficulty {
	lower := strings.ToLower(fmt.Sprintf("%s %s", title, contentType))
	advancedTerms := []string{"advanced", "deep dive", "internals", "architecture", "performance", "optimization", "expert"}
	beginnerTerms := []string{"introduction", "intro ", "beginner", "getting started", "101", "basics", "fundamentals", "tutorial"}

	for _, t := range advancedTerms {
		if strings.Contains(lower, t) {
			return DifficultyAdvanced
		}
	}
	for _, t := range beginnerTerms {
		if strings.Contains(lower, t) {
			return DifficultyBeginner
		}
	}
	return DifficultyIntermediate
}

// detectGaps identifies missing difficulty levels in a learning path.
func detectGaps(resources []LearningResource) []string {
	hasBeginner, hasIntermediate, hasAdvanced := false, false, false
	for _, r := range resources {
		switch r.Difficulty {
		case DifficultyBeginner:
			hasBeginner = true
		case DifficultyIntermediate:
			hasIntermediate = true
		case DifficultyAdvanced:
			hasAdvanced = true
		}
	}

	var gaps []string
	if hasBeginner && hasAdvanced && !hasIntermediate {
		gaps = append(gaps, "No intermediate-level resources — consider finding a tutorial between beginner and advanced")
	}
	if !hasBeginner && (hasIntermediate || hasAdvanced) {
		gaps = append(gaps, "No beginner-level resources — the path may be hard to start")
	}
	return gaps
}

// difficultyOrder returns an integer for sorting resources by difficulty level.
func difficultyOrder(d LearningDifficulty) int {
	switch d {
	case DifficultyBeginner:
		return 0
	case DifficultyIntermediate:
		return 1
	case DifficultyAdvanced:
		return 2
	default:
		return 1
	}
}
