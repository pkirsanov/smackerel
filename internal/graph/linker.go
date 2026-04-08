package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

// Linker creates knowledge graph edges after artifact processing.
type Linker struct {
	Pool *pgxpool.Pool
}

// NewLinker creates a new knowledge graph linker.
func NewLinker(pool *pgxpool.Pool) *Linker {
	if pool == nil {
		return nil
	}
	return &Linker{Pool: pool}
}

// LinkArtifact runs all linking strategies for a processed artifact.
func (l *Linker) LinkArtifact(ctx context.Context, artifactID string) (int, error) {
	if l == nil || l.Pool == nil {
		return 0, nil
	}

	var totalEdges int

	// 1. Vector similarity linking
	simEdges, err := l.linkBySimilarity(ctx, artifactID)
	if err != nil {
		slog.Warn("similarity linking failed", "artifact_id", artifactID, "error", err)
	} else {
		totalEdges += simEdges
	}

	// 2. Entity-based linking (people)
	entEdges, err := l.linkByEntities(ctx, artifactID)
	if err != nil {
		slog.Warn("entity linking failed", "artifact_id", artifactID, "error", err)
	} else {
		totalEdges += entEdges
	}

	// 3. Topic clustering
	topicEdges, err := l.linkByTopics(ctx, artifactID)
	if err != nil {
		slog.Warn("topic linking failed", "artifact_id", artifactID, "error", err)
	} else {
		totalEdges += topicEdges
	}

	// 4. Temporal linking (same-day)
	tempEdges, err := l.linkByTemporal(ctx, artifactID)
	if err != nil {
		slog.Warn("temporal linking failed", "artifact_id", artifactID, "error", err)
	} else {
		totalEdges += tempEdges
	}

	slog.Info("artifact linking complete",
		"artifact_id", artifactID,
		"edges_created", totalEdges,
	)
	return totalEdges, nil
}

// linkBySimilarity finds the top 10 most similar artifacts by embedding and creates RELATED_TO edges.
func (l *Linker) linkBySimilarity(ctx context.Context, artifactID string) (int, error) {
	// Fetch the target artifact's embedding directly
	var embeddingBytes []byte
	err := l.Pool.QueryRow(ctx,
		"SELECT embedding FROM artifacts WHERE id = $1 AND embedding IS NOT NULL", artifactID,
	).Scan(&embeddingBytes)
	if err != nil {
		return 0, nil // No embedding yet, skip
	}

	// Single parameterized nearest-neighbor query using a CTE to avoid subquery duplication
	rows, err := l.Pool.Query(ctx, `
		WITH target AS (SELECT embedding FROM artifacts WHERE id = $1)
		SELECT id, 1 - (embedding <=> (SELECT embedding FROM target)) AS similarity
		FROM artifacts
		WHERE id != $1 AND embedding IS NOT NULL
		ORDER BY embedding <=> (SELECT embedding FROM target)
		LIMIT 10
	`, artifactID)
	if err != nil {
		return 0, fmt.Errorf("similarity query: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var relatedID string
		var similarity float64
		if err := rows.Scan(&relatedID, &similarity); err != nil {
			continue
		}

		// Only create edge if similarity is above threshold
		if similarity < 0.3 {
			continue
		}

		// Normalize direction to prevent bidirectional duplicates
		srcID, dstID := artifactID, relatedID
		if srcID > dstID {
			srcID, dstID = dstID, srcID
		}

		if err := l.createEdge(ctx, "artifact", srcID, "artifact", dstID, "RELATED_TO", float32(similarity)); err != nil {
			slog.Debug("edge creation failed", "src", srcID, "dst", dstID, "error", err)
			continue
		}
		count++
	}

	if err := rows.Err(); err != nil {
		return count, fmt.Errorf("similarity row iteration: %w", err)
	}

	return count, nil
}

// linkByEntities matches extracted entities against the People table and creates MENTIONS edges.
func (l *Linker) linkByEntities(ctx context.Context, artifactID string) (int, error) {
	// Get entity list from artifact
	var entitiesJSON []byte
	err := l.Pool.QueryRow(ctx,
		"SELECT COALESCE(entities, '{}'::jsonb) FROM artifacts WHERE id = $1", artifactID,
	).Scan(&entitiesJSON)
	if err != nil {
		return 0, fmt.Errorf("get entities: %w", err)
	}

	// Parse people from entities
	type Entities struct {
		People []string `json:"people"`
	}

	var entities Entities
	if err := parseJSON(entitiesJSON, &entities); err != nil {
		return 0, nil // No parseable entities
	}

	// Collect unique non-empty names
	var names []string
	for _, name := range entities.People {
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return 0, nil
	}

	// Batch upsert all people
	personMap, err := l.findOrCreatePeople(ctx, names)
	if err != nil {
		return 0, fmt.Errorf("batch upsert people: %w", err)
	}

	count := 0
	for _, personName := range names {
		personID, ok := personMap[personName]
		if !ok {
			continue
		}

		// Create MENTIONS edge
		if err := l.createEdge(ctx, "artifact", artifactID, "person", personID, "MENTIONS", 1.0); err != nil {
			slog.Debug("mentions edge failed", "artifact", artifactID, "person", personID, "error", err)
			continue
		}

		// Increment interaction count
		if _, err := l.Pool.Exec(ctx, `
			UPDATE people SET interaction_count = interaction_count + 1, last_interaction = NOW(), updated_at = NOW()
			WHERE id = $1
		`, personID); err != nil {
			slog.Warn("failed to update interaction count", "person_id", personID, "error", err)
		}

		count++
	}

	return count, nil
}

// linkByTopics assigns artifacts to topics and creates BELONGS_TO edges.
func (l *Linker) linkByTopics(ctx context.Context, artifactID string) (int, error) {
	// Get topics from artifact
	var topicsJSON []byte
	err := l.Pool.QueryRow(ctx,
		"SELECT COALESCE(topics, '[]'::jsonb) FROM artifacts WHERE id = $1", artifactID,
	).Scan(&topicsJSON)
	if err != nil {
		return 0, fmt.Errorf("get topics: %w", err)
	}

	var topicNames []string
	if err := parseJSON(topicsJSON, &topicNames); err != nil {
		return 0, nil
	}

	// Collect unique non-empty names
	var cleaned []string
	for _, name := range topicNames {
		name = strings.TrimSpace(strings.ToLower(name))
		if name != "" {
			cleaned = append(cleaned, name)
		}
	}
	if len(cleaned) == 0 {
		return 0, nil
	}

	// Batch upsert all topics
	topicMap, err := l.findOrCreateTopics(ctx, cleaned)
	if err != nil {
		return 0, fmt.Errorf("batch upsert topics: %w", err)
	}

	count := 0
	for _, topicName := range cleaned {
		topicID, ok := topicMap[topicName]
		if !ok {
			continue
		}

		if err := l.createEdge(ctx, "artifact", artifactID, "topic", topicID, "BELONGS_TO", 1.0); err != nil {
			slog.Debug("belongs_to edge failed", "artifact", artifactID, "topic", topicID, "error", err)
			continue
		}

		// Update topic stats
		if _, err := l.Pool.Exec(ctx, `
			UPDATE topics SET
				capture_count_total = capture_count_total + 1,
				capture_count_30d = capture_count_30d + 1,
				last_active = NOW(),
				state = CASE
					WHEN capture_count_total >= 10 THEN 'hot'
					WHEN capture_count_total >= 5 THEN 'active'
					WHEN capture_count_total >= 3 THEN 'emerging'
					ELSE state
				END,
				updated_at = NOW()
			WHERE id = $1
		`, topicID); err != nil {
			slog.Warn("failed to update topic stats", "topic_id", topicID, "error", err)
		}

		count++
	}

	return count, nil
}

// linkByTemporal creates edges between artifacts captured on the same day.
func (l *Linker) linkByTemporal(ctx context.Context, artifactID string) (int, error) {
	rows, err := l.Pool.Query(ctx, `
		SELECT a2.id, DATE(a1.created_at) FROM artifacts a1, artifacts a2
		WHERE a1.id = $1
		AND a2.id != $1
		AND DATE(a2.created_at) = DATE(a1.created_at)
		AND a1.embedding IS NOT NULL AND a2.embedding IS NOT NULL
		AND a1.embedding <=> a2.embedding < 0.8
		LIMIT 20
	`, artifactID)
	if err != nil {
		return 0, fmt.Errorf("temporal query: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var relatedID string
		var createdDate time.Time
		if err := rows.Scan(&relatedID, &createdDate); err != nil {
			continue
		}

		// Normalize direction to prevent bidirectional duplicates
		srcID, dstID := artifactID, relatedID
		if srcID > dstID {
			srcID, dstID = dstID, srcID
		}

		metadata := fmt.Sprintf(`{"proximity": "same_day", "date": "%s"}`, createdDate.Format("2006-01-02"))
		if err := l.createEdgeWithMetadata(ctx, "artifact", srcID, "artifact", dstID, "RELATED_TO", 0.5, metadata); err != nil {
			continue
		}
		count++
	}

	if err := rows.Err(); err != nil {
		return count, fmt.Errorf("temporal row iteration: %w", err)
	}

	return count, nil
}

// createEdge creates a graph edge, ignoring duplicates.
func (l *Linker) createEdge(ctx context.Context, srcType, srcID, dstType, dstID, edgeType string, weight float32) error {
	return l.createEdgeWithMetadata(ctx, srcType, srcID, dstType, dstID, edgeType, weight, "{}")
}

// createEdgeWithMetadata creates a graph edge with JSON metadata, ignoring duplicates.
func (l *Linker) createEdgeWithMetadata(ctx context.Context, srcType, srcID, dstType, dstID, edgeType string, weight float32, metadata string) error {
	id := ulid.Make().String()
	_, err := l.Pool.Exec(ctx, `
		INSERT INTO edges (id, src_type, src_id, dst_type, dst_id, edge_type, weight, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (src_type, src_id, dst_type, dst_id, edge_type) DO UPDATE SET weight = $7, metadata = $8
	`, id, srcType, srcID, dstType, dstID, edgeType, weight, metadata)
	return err
}

// findOrCreatePeople batch-upserts people by name and returns a map of name→id.
func (l *Linker) findOrCreatePeople(ctx context.Context, names []string) (map[string]string, error) {
	ids := make([]string, len(names))
	for i := range names {
		ids[i] = ulid.Make().String()
	}
	rows, err := l.Pool.Query(ctx, `
		INSERT INTO people (id, name)
		SELECT unnest($1::text[]), unnest($2::text[])
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING name, id
	`, ids, names)
	if err != nil {
		return nil, fmt.Errorf("batch upsert people: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string, len(names))
	for rows.Next() {
		var name, id string
		if err := rows.Scan(&name, &id); err != nil {
			continue
		}
		result[name] = id
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("batch upsert people rows: %w", err)
	}
	return result, nil
}

// findOrCreateTopics batch-upserts topics by name and returns a map of name→id.
func (l *Linker) findOrCreateTopics(ctx context.Context, names []string) (map[string]string, error) {
	ids := make([]string, len(names))
	for i := range names {
		ids[i] = ulid.Make().String()
	}
	rows, err := l.Pool.Query(ctx, `
		INSERT INTO topics (id, name, state)
		SELECT unnest($1::text[]), unnest($2::text[]), 'emerging'
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		RETURNING name, id
	`, ids, names)
	if err != nil {
		return nil, fmt.Errorf("batch upsert topics: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string, len(names))
	for rows.Next() {
		var name, id string
		if err := rows.Scan(&name, &id); err != nil {
			continue
		}
		result[name] = id
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("batch upsert topics rows: %w", err)
	}
	return result, nil
}

// ConnectionCount returns the number of edges connected to an artifact.
func (l *Linker) ConnectionCount(ctx context.Context, artifactID string) (int, error) {
	var count int
	err := l.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM edges
		WHERE (src_type = 'artifact' AND src_id = $1)
		   OR (dst_type = 'artifact' AND dst_id = $1)
	`, artifactID).Scan(&count)
	return count, err
}

// ConnectionCount returns the number of graph edges connected to an artifact.
// Package-level helper for use by other packages without needing a Linker instance.
func ConnectionCount(ctx context.Context, pool *pgxpool.Pool, artifactID string) int {
	var count int
	_ = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM edges
		WHERE (src_type = 'artifact' AND src_id = $1)
		   OR (dst_type = 'artifact' AND dst_id = $1)
	`, artifactID).Scan(&count)
	return count
}

func parseJSON(data []byte, v interface{}) error {
	if len(data) == 0 {
		return fmt.Errorf("empty JSON")
	}
	return json.Unmarshal(data, v)
}
