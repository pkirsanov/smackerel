package bookmarks

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// TopicMatch holds the result of mapping a folder segment to a topic.
type TopicMatch struct {
	FolderName string
	TopicID    string
	TopicName  string
	MatchType  string // "exact", "fuzzy", "created"
}

// TopicMapper resolves bookmark folder hierarchies to knowledge graph topics.
type TopicMapper struct {
	pool *pgxpool.Pool
}

// NewTopicMapper creates a new topic mapper.
func NewTopicMapper(pool *pgxpool.Pool) *TopicMapper {
	return &TopicMapper{pool: pool}
}

// MapFolder splits a folder path by "/" and resolves each segment to a topic.
// Returns one TopicMatch per non-empty segment. Parent-child CHILD_OF edges are
// created between hierarchical segments.
func (tm *TopicMapper) MapFolder(ctx context.Context, folderPath string) ([]TopicMatch, error) {
	if tm.pool == nil || strings.TrimSpace(folderPath) == "" {
		return nil, nil
	}

	segments := strings.Split(folderPath, "/")

	var matches []TopicMatch
	var prevTopicID string

	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}

		match, err := tm.resolveSegment(ctx, seg)
		if err != nil {
			slog.Warn("failed to resolve topic segment", "segment", seg, "error", err)
			continue
		}

		matches = append(matches, *match)

		// Create CHILD_OF edge for hierarchy
		if prevTopicID != "" {
			if err := tm.CreateParentEdge(ctx, match.TopicID, prevTopicID); err != nil {
				slog.Warn("failed to create parent edge",
					"child", match.TopicID,
					"parent", prevTopicID,
					"error", err,
				)
			}
		}
		prevTopicID = match.TopicID
	}

	return matches, nil
}

// resolveSegment resolves a single folder segment to a topic via a 3-stage cascade:
// 1. Exact match (case-insensitive)
// 2. Fuzzy match via pg_trgm similarity (threshold 0.4)
// 3. Create new topic with state "emerging"
func (tm *TopicMapper) resolveSegment(ctx context.Context, segment string) (*TopicMatch, error) {
	// Stage 1: Exact match (case-insensitive)
	var id, name string
	err := tm.pool.QueryRow(ctx, `
		SELECT id, name FROM topics WHERE LOWER(name) = LOWER($1) LIMIT 1
	`, segment).Scan(&id, &name)
	if err == nil {
		return &TopicMatch{
			FolderName: segment,
			TopicID:    id,
			TopicName:  name,
			MatchType:  "exact",
		}, nil
	}

	// Stage 2: Fuzzy match via pg_trgm (gracefully skip if extension unavailable)
	fuzzyMatch, err := tm.fuzzyMatch(ctx, segment)
	if err == nil && fuzzyMatch != nil {
		return fuzzyMatch, nil
	}

	// Stage 3: Create new topic
	newID := ulid.Make().String()
	_, err = tm.pool.Exec(ctx, `
		INSERT INTO topics (id, name, state, capture_count_total, capture_count_30d, capture_count_90d, search_hit_count_30d)
		VALUES ($1, $2, 'emerging', 0, 0, 0, 0)
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, newID, segment)
	if err != nil {
		return nil, fmt.Errorf("create topic %q: %w", segment, err)
	}

	// Re-read the actual ID in case ON CONFLICT triggered
	err = tm.pool.QueryRow(ctx, `SELECT id FROM topics WHERE LOWER(name) = LOWER($1)`, segment).Scan(&newID)
	if err != nil {
		return nil, fmt.Errorf("re-read topic %q: %w", segment, err)
	}

	slog.Info("created new topic from bookmark folder", "segment", segment, "topic_id", newID)
	return &TopicMatch{
		FolderName: segment,
		TopicID:    newID,
		TopicName:  segment,
		MatchType:  "created",
	}, nil
}

// fuzzyMatch attempts a pg_trgm similarity match. Returns nil if no match above threshold
// or if the pg_trgm extension is not available.
func (tm *TopicMapper) fuzzyMatch(ctx context.Context, segment string) (*TopicMatch, error) {
	var id, name string
	var sim float64

	err := tm.pool.QueryRow(ctx, `
		SELECT id, name, similarity(name, $1) AS sim
		FROM topics
		WHERE similarity(name, $1) > 0.4
		ORDER BY sim DESC
		LIMIT 1
	`, segment).Scan(&id, &name, &sim)
	if err != nil {
		return nil, err // pg_trgm not available or no match
	}

	slog.Debug("fuzzy topic match", "segment", segment, "matched", name, "similarity", sim)
	return &TopicMatch{
		FolderName: segment,
		TopicID:    id,
		TopicName:  name,
		MatchType:  "fuzzy",
	}, nil
}

// CreateTopicEdge creates a BELONGS_TO edge from an artifact to a topic.
func (tm *TopicMapper) CreateTopicEdge(ctx context.Context, artifactID, topicID string) error {
	if tm.pool == nil {
		return nil
	}

	id := ulid.Make().String()
	_, err := tm.pool.Exec(ctx, `
		INSERT INTO edges (id, src_type, src_id, dst_type, dst_id, edge_type, weight, metadata)
		VALUES ($1, 'artifact', $2, 'topic', $3, 'BELONGS_TO', 1.0, '{}')
		ON CONFLICT (src_type, src_id, dst_type, dst_id, edge_type) DO NOTHING
	`, id, artifactID, topicID)
	if err != nil {
		return fmt.Errorf("create BELONGS_TO edge: %w", err)
	}
	return nil
}

// CreateParentEdge creates a CHILD_OF edge from a child topic to a parent topic.
func (tm *TopicMapper) CreateParentEdge(ctx context.Context, childTopicID, parentTopicID string) error {
	if tm.pool == nil {
		return nil
	}

	id := ulid.Make().String()
	_, err := tm.pool.Exec(ctx, `
		INSERT INTO edges (id, src_type, src_id, dst_type, dst_id, edge_type, weight, metadata)
		VALUES ($1, 'topic', $2, 'topic', $3, 'CHILD_OF', 1.0, '{}')
		ON CONFLICT (src_type, src_id, dst_type, dst_id, edge_type) DO NOTHING
	`, id, childTopicID, parentTopicID)
	if err != nil {
		return fmt.Errorf("create CHILD_OF edge: %w", err)
	}
	return nil
}

// UpdateTopicMomentum increments capture counts for the topic to feed momentum scoring.
func (tm *TopicMapper) UpdateTopicMomentum(ctx context.Context, topicID string) error {
	if tm.pool == nil {
		return nil
	}

	_, err := tm.pool.Exec(ctx, `
		UPDATE topics
		SET capture_count_total = capture_count_total + 1,
		    capture_count_30d = capture_count_30d + 1,
		    capture_count_90d = capture_count_90d + 1,
		    last_active = NOW(),
		    updated_at = NOW()
		WHERE id = $1
	`, topicID)
	if err != nil {
		return fmt.Errorf("update topic momentum: %w", err)
	}
	return nil
}
