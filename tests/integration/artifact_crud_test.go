//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Scenario: Artifact insert and vector search
func TestArtifact_InsertAndVectorSearch(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	id := testID(t)
	cleanupArtifact(t, pool, id)

	// Generate a simple 384-dim embedding (all-MiniLM-L6-v2 output dimension)
	embedding := make([]float32, 384)
	for i := range embedding {
		embedding[i] = float32(i) / 384.0
	}
	embJSON, _ := json.Marshal(embedding)

	// Insert artifact with embedding
	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, source_url, embedding, created_at, updated_at)
		VALUES ($1, 'article', 'Test Article', 'A test article about cooking', 'Full content about pasta recipes and Italian cooking', $2, 'test-source', 'https://example.com/test', $3::vector, NOW(), NOW())
	`, id, fmt.Sprintf("hash-%s", id), string(embJSON))
	if err != nil {
		t.Fatalf("insert artifact: %v", err)
	}

	// Verify insert via query
	var title, artifactType string
	err = pool.QueryRow(ctx, "SELECT title, artifact_type FROM artifacts WHERE id = $1", id).Scan(&title, &artifactType)
	if err != nil {
		t.Fatalf("query artifact: %v", err)
	}
	if title != "Test Article" {
		t.Errorf("expected title 'Test Article', got %q", title)
	}
	if artifactType != "article" {
		t.Errorf("expected type 'article', got %q", artifactType)
	}

	// Vector similarity search — should find the artifact we just inserted
	rows, err := pool.Query(ctx, `
		SELECT id, title, 1 - (embedding <=> $1::vector) AS similarity
		FROM artifacts
		WHERE embedding IS NOT NULL
		ORDER BY embedding <=> $1::vector
		LIMIT 5
	`, string(embJSON))
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var resultID, resultTitle string
		var similarity float64
		if err := rows.Scan(&resultID, &resultTitle, &similarity); err != nil {
			t.Fatalf("scan result: %v", err)
		}
		if resultID == id {
			found = true
			if similarity < 0.99 {
				t.Errorf("expected near-perfect similarity for same embedding, got %.4f", similarity)
			}
			t.Logf("found artifact %s with similarity %.4f", resultID, similarity)
		}
	}
	if !found {
		t.Error("inserted artifact not found in vector search results")
	}
}

func TestArtifact_TextSearch(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	id := testID(t)
	cleanupArtifact(t, pool, id)

	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'article', 'Homemade Sourdough Bread Recipe', 'A guide to making sourdough bread at home', 'Detailed instructions for sourdough starter and baking', $2, 'test-source', NOW(), NOW())
	`, id, fmt.Sprintf("hash-%s", id))
	if err != nil {
		t.Fatalf("insert artifact: %v", err)
	}

	// Trigram text search on title
	var count int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM artifacts WHERE title ILIKE $1 AND id = $2
	`, "%sourdough%", id).Scan(&count)
	if err != nil {
		t.Fatalf("text search: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 result for sourdough search, got %d", count)
	}
}

func TestArtifact_DomainDataContainmentQuery(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	id := testID(t)
	cleanupArtifact(t, pool, id)

	// Insert artifact with domain_data (recipe with ingredients)
	domainData := map[string]interface{}{
		"domain": "recipe",
		"ingredients": []map[string]string{
			{"name": "chicken", "quantity": "500g"},
			{"name": "garlic", "quantity": "5 cloves"},
			{"name": "olive oil", "quantity": "2 tbsp"},
		},
		"steps": []string{"Marinate chicken", "Roast at 200C", "Rest for 10 minutes"},
	}
	domainJSON, _ := json.Marshal(domainData)

	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, domain_data, domain_extraction_status, created_at, updated_at)
		VALUES ($1, 'article', 'Roasted Chicken Recipe', 'A roasted chicken recipe', 'Full recipe content', $2, 'test-source', $3::jsonb, 'completed', NOW(), NOW())
	`, id, fmt.Sprintf("hash-%s", id), string(domainJSON))
	if err != nil {
		t.Fatalf("insert artifact with domain_data: %v", err)
	}

	// JSONB containment query for chicken ingredient
	var found bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM artifacts
			WHERE domain_data @> '{"ingredients": [{"name": "chicken"}]}'
			AND id = $1
		)
	`, id).Scan(&found)
	if err != nil {
		t.Fatalf("containment query: %v", err)
	}
	if !found {
		t.Error("expected containment query to find artifact with chicken ingredient")
	}

	// Negative case: should not find non-existent ingredient
	err = pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM artifacts
			WHERE domain_data @> '{"ingredients": [{"name": "tofu"}]}'
			AND id = $1
		)
	`, id).Scan(&found)
	if err != nil {
		t.Fatalf("negative containment query: %v", err)
	}
	if found {
		t.Error("containment query should not find tofu in chicken recipe")
	}
}

// Scenario: Annotation CRUD against real PostgreSQL
func TestAnnotation_CRUD(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First insert an artifact to annotate
	artifactID := testID(t)
	cleanupArtifact(t, pool, artifactID)

	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'article', 'Annotated Article', 'Test article for annotations', 'Content', $2, 'test-source', NOW(), NOW())
	`, artifactID, fmt.Sprintf("hash-%s", artifactID))
	if err != nil {
		t.Fatalf("insert artifact: %v", err)
	}

	// Create a rating annotation
	annID := fmt.Sprintf("ann-%s-rating", artifactID[:8])
	cleanupAnnotation(t, pool, annID)
	_, err = pool.Exec(ctx, `
		INSERT INTO annotations (id, artifact_id, annotation_type, rating, source_channel, created_at)
		VALUES ($1, $2, 'rating', 5, 'api', NOW())
	`, annID, artifactID)
	if err != nil {
		t.Fatalf("insert rating annotation: %v", err)
	}

	// Create an interaction annotation
	interactionID := fmt.Sprintf("ann-%s-interaction", artifactID[:8])
	cleanupAnnotation(t, pool, interactionID)
	_, err = pool.Exec(ctx, `
		INSERT INTO annotations (id, artifact_id, annotation_type, interaction_type, source_channel, created_at)
		VALUES ($1, $2, 'interaction', 'made_it', 'telegram', NOW())
	`, interactionID, artifactID)
	if err != nil {
		t.Fatalf("insert interaction annotation: %v", err)
	}

	// Create a tag annotation
	tagID := fmt.Sprintf("ann-%s-tag", artifactID[:8])
	cleanupAnnotation(t, pool, tagID)
	_, err = pool.Exec(ctx, `
		INSERT INTO annotations (id, artifact_id, annotation_type, tag, source_channel, created_at)
		VALUES ($1, $2, 'tag_add', 'favorite', 'web', NOW())
	`, tagID, artifactID)
	if err != nil {
		t.Fatalf("insert tag annotation: %v", err)
	}

	// Query annotation history
	rows, err := pool.Query(ctx, `
		SELECT id, annotation_type, rating, COALESCE(tag, ''), COALESCE(interaction_type, '')
		FROM annotations WHERE artifact_id = $1
		ORDER BY created_at DESC
	`, artifactID)
	if err != nil {
		t.Fatalf("query annotations: %v", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, annType, tag, interType string
		var rating *int
		if err := rows.Scan(&id, &annType, &rating, &tag, &interType); err != nil {
			t.Fatalf("scan annotation: %v", err)
		}
		count++
		t.Logf("annotation: id=%s type=%s rating=%v tag=%s interaction=%s", id, annType, rating, tag, interType)
	}
	if count != 3 {
		t.Errorf("expected 3 annotations, got %d", count)
	}

	// Refresh materialized view and query summary
	_, err = pool.Exec(ctx, "REFRESH MATERIALIZED VIEW CONCURRENTLY artifact_annotation_summary")
	if err != nil {
		t.Fatalf("refresh summary view: %v", err)
	}

	var currentRating *int
	var timesUsed int
	err = pool.QueryRow(ctx, `
		SELECT current_rating, times_used FROM artifact_annotation_summary WHERE artifact_id = $1
	`, artifactID).Scan(&currentRating, &timesUsed)
	if err != nil {
		t.Fatalf("query summary: %v", err)
	}
	if currentRating == nil || *currentRating != 5 {
		t.Errorf("expected current_rating=5, got %v", currentRating)
	}
	if timesUsed != 1 {
		t.Errorf("expected times_used=1, got %d", timesUsed)
	}

	// Verify rating constraint (should reject rating=0)
	_, err = pool.Exec(ctx, `
		INSERT INTO annotations (id, artifact_id, annotation_type, rating, source_channel, created_at)
		VALUES ($1, $2, 'rating', 0, 'api', NOW())
	`, fmt.Sprintf("ann-%s-badrating", artifactID[:8]), artifactID)
	if err == nil {
		t.Error("expected constraint violation for rating=0, but insert succeeded")
		pool.Exec(ctx, "DELETE FROM annotations WHERE id = $1", fmt.Sprintf("ann-%s-badrating", artifactID[:8]))
	}
}

// Scenario: List creation with items, item status update
func TestList_CreateAndUpdateStatus(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert a source artifact first
	artifactID := testID(t)
	cleanupArtifact(t, pool, artifactID)

	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'article', 'Recipe for Shopping List', 'A recipe', 'Content', $2, 'test-source', NOW(), NOW())
	`, artifactID, fmt.Sprintf("hash-%s", artifactID))
	if err != nil {
		t.Fatalf("insert source artifact: %v", err)
	}

	// Create a shopping list
	listID := testID(t)
	cleanupList(t, pool, listID)
	now := time.Now()

	_, err = pool.Exec(ctx, `
		INSERT INTO lists (id, list_type, title, status, source_artifact_ids, domain, total_items, checked_items, created_at, updated_at)
		VALUES ($1, 'shopping', 'Weekly Groceries', 'active', ARRAY[$2]::text[], 'recipe', 3, 0, $3, $3)
	`, listID, artifactID, now)
	if err != nil {
		t.Fatalf("insert list: %v", err)
	}

	// Insert list items
	items := []struct {
		id, content, category, normalizedName string
		quantity                              float64
		unit                                  string
		sortOrder                             int
	}{
		{testID(t) + "-1", "500g chicken breast", "meat", "chicken breast", 500, "g", 1},
		{testID(t) + "-2", "5 cloves garlic", "produce", "garlic", 5, "cloves", 2},
		{testID(t) + "-3", "2 tbsp olive oil", "pantry", "olive oil", 2, "tbsp", 3},
	}

	for _, item := range items {
		_, err = pool.Exec(ctx, `
			INSERT INTO list_items (id, list_id, content, category, status, source_artifact_ids, is_manual, quantity, unit, normalized_name, sort_order, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'pending', ARRAY[$5]::text[], false, $6, $7, $8, $9, $10, $10)
		`, item.id, listID, item.content, item.category, artifactID, item.quantity, item.unit, item.normalizedName, item.sortOrder, now)
		if err != nil {
			t.Fatalf("insert item %s: %v", item.content, err)
		}
	}

	// Query list with items
	var listTitle, listStatus string
	var totalItems, checkedItems int
	err = pool.QueryRow(ctx, "SELECT title, status, total_items, checked_items FROM lists WHERE id = $1", listID).
		Scan(&listTitle, &listStatus, &totalItems, &checkedItems)
	if err != nil {
		t.Fatalf("query list: %v", err)
	}
	if listTitle != "Weekly Groceries" {
		t.Errorf("expected title 'Weekly Groceries', got %q", listTitle)
	}
	if listStatus != "active" {
		t.Errorf("expected status 'active', got %q", listStatus)
	}

	// Update item status to 'done'
	_, err = pool.Exec(ctx, `
		UPDATE list_items SET status = 'done', checked_at = NOW(), updated_at = NOW() WHERE id = $1
	`, items[0].id)
	if err != nil {
		t.Fatalf("update item status: %v", err)
	}

	// Update list checked_items counter
	_, err = pool.Exec(ctx, `
		UPDATE lists SET checked_items = checked_items + 1, updated_at = NOW() WHERE id = $1
	`, listID)
	if err != nil {
		t.Fatalf("update list counter: %v", err)
	}

	// Verify counter
	err = pool.QueryRow(ctx, "SELECT checked_items FROM lists WHERE id = $1", listID).Scan(&checkedItems)
	if err != nil {
		t.Fatalf("query updated list: %v", err)
	}
	if checkedItems != 1 {
		t.Errorf("expected checked_items=1, got %d", checkedItems)
	}

	// Mark all items done and complete list
	_, err = pool.Exec(ctx, "UPDATE list_items SET status = 'done', checked_at = NOW() WHERE list_id = $1", listID)
	if err != nil {
		t.Fatalf("mark all done: %v", err)
	}
	_, err = pool.Exec(ctx, "UPDATE lists SET checked_items = total_items, status = 'completed', completed_at = NOW() WHERE id = $1", listID)
	if err != nil {
		t.Fatalf("complete list: %v", err)
	}

	err = pool.QueryRow(ctx, "SELECT status FROM lists WHERE id = $1", listID).Scan(&listStatus)
	if err != nil {
		t.Fatalf("query completed list: %v", err)
	}
	if listStatus != "completed" {
		t.Errorf("expected status 'completed', got %q", listStatus)
	}
	t.Log("list creation, item status update, and completion verified")
}

// Scenario: Vector similarity with different embeddings
func TestArtifact_VectorSimilarityDifferentEmbeddings(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert two artifacts with different embeddings
	id1 := testID(t) + "-similar"
	id2 := testID(t) + "-different"
	cleanupArtifact(t, pool, id1)
	cleanupArtifact(t, pool, id2)

	// Similar embedding (close to query)
	emb1 := make([]float32, 384)
	for i := range emb1 {
		emb1[i] = float32(i) / 384.0
	}

	// Different embedding (far from query)
	emb2 := make([]float32, 384)
	for i := range emb2 {
		emb2[i] = float32(383-i) / 384.0
	}

	emb1JSON, _ := json.Marshal(emb1)
	emb2JSON, _ := json.Marshal(emb2)

	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, embedding, created_at, updated_at)
		VALUES ($1, 'article', 'Similar Article', 'Close embedding', 'Content A', $2, 'test', $3::vector, NOW(), NOW())
	`, id1, fmt.Sprintf("hash-%s", id1), string(emb1JSON))
	if err != nil {
		t.Fatalf("insert similar: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, embedding, created_at, updated_at)
		VALUES ($1, 'article', 'Different Article', 'Far embedding', 'Content B', $2, 'test', $3::vector, NOW(), NOW())
	`, id2, fmt.Sprintf("hash-%s", id2), string(emb2JSON))
	if err != nil {
		t.Fatalf("insert different: %v", err)
	}

	// Search using emb1 as query — id1 should rank higher
	queryEmb, _ := json.Marshal(emb1)
	rows, err := pool.Query(ctx, `
		SELECT id, 1 - (embedding <=> $1::vector) AS similarity
		FROM artifacts
		WHERE embedding IS NOT NULL AND id IN ($2, $3)
		ORDER BY embedding <=> $1::vector
		LIMIT 2
	`, string(queryEmb), id1, id2)
	if err != nil {
		t.Fatalf("similarity search: %v", err)
	}
	defer rows.Close()

	var results []struct {
		id         string
		similarity float64
	}
	for rows.Next() {
		var r struct {
			id         string
			similarity float64
		}
		if err := rows.Scan(&r.id, &r.similarity); err != nil {
			t.Fatalf("scan: %v", err)
		}
		results = append(results, r)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First result should be id1 (most similar)
	if results[0].id != id1 {
		t.Errorf("expected first result to be %s (similar), got %s", id1, results[0].id)
	}

	// Similarity scores should differ significantly
	if math.Abs(results[0].similarity-results[1].similarity) < 0.01 {
		t.Errorf("expected different similarity scores, got %.4f and %.4f", results[0].similarity, results[1].similarity)
	}
	t.Logf("similar=%.4f different=%.4f", results[0].similarity, results[1].similarity)
}

// CHAOS-031-004: Concurrent duplicate content_hash insertion.
// The idx_artifacts_content_hash_unique partial unique index must reject
// concurrent duplicate inserts. This test verifies that exactly one writer
// wins and all others get a unique constraint violation, preventing silent
// duplicate artifacts under concurrent capture.
func TestArtifact_Chaos_ConcurrentDuplicateContentHash(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sharedHash := fmt.Sprintf("chaos-dedup-%d", time.Now().UnixNano())
	const concurrency = 10

	var successCount atomic.Int32
	var conflictCount atomic.Int32
	var otherErrors atomic.Int32

	var wg sync.WaitGroup
	for i := range concurrency {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("chaos-%s-%d", sharedHash, idx)
			_, err := pool.Exec(ctx, `
				INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, created_at, updated_at)
				VALUES ($1, 'article', $2, 'chaos test', 'content', $3, 'test-source', NOW(), NOW())
			`, id, fmt.Sprintf("Chaos Article %d", idx), sharedHash)
			if err == nil {
				successCount.Add(1)
			} else if isUniqueViolation(err) {
				conflictCount.Add(1)
			} else {
				otherErrors.Add(1)
				t.Logf("unexpected error from goroutine %d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	// Cleanup: delete all artifacts with this hash (only 1 should exist)
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		pool.Exec(cctx, "DELETE FROM artifacts WHERE content_hash = $1", sharedHash)
	})

	successes := successCount.Load()
	conflicts := conflictCount.Load()
	others := otherErrors.Load()

	t.Logf("concurrent inserts: successes=%d conflicts=%d errors=%d", successes, conflicts, others)

	if successes != 1 {
		t.Errorf("expected exactly 1 successful insert, got %d", successes)
	}
	if conflicts != int32(concurrency)-1 {
		t.Errorf("expected %d unique violations, got %d", concurrency-1, conflicts)
	}
	if others != 0 {
		t.Errorf("expected 0 other errors, got %d", others)
	}

	// Verify only 1 row exists
	var count int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM artifacts WHERE content_hash = $1", sharedHash).Scan(&count)
	if err != nil {
		t.Fatalf("count check: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 artifact with hash %s, got %d", sharedHash, count)
	}
}

// isUniqueViolation checks if a pgx error is a unique constraint violation (SQLSTATE 23505).
// STAB-031-002: Uses typed pgconn.PgError extraction instead of fragile string matching.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

// CHAOS-031-007: Vector search with degenerate (all-zero) embedding.
// Cosine distance with a zero vector is mathematically undefined (division by zero
// in the normalization step). This test verifies that pgvector handles all-zero
// embeddings gracefully — either by returning NaN/Inf similarity (which we must
// tolerate) or by excluding them from results. A crash or uncaught error here
// would mean any artifact with a failed embedding generation (all zeros) could
// poison the entire search pipeline.
func TestArtifact_Chaos_ZeroEmbeddingSearch(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Insert an artifact with an all-zero 384-dim embedding
	zeroID := testID(t) + "-zero"
	cleanupArtifact(t, pool, zeroID)

	zeroEmb := make([]float32, 384)
	zeroJSON, _ := json.Marshal(zeroEmb)

	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, embedding, created_at, updated_at)
		VALUES ($1, 'article', 'Zero Embedding Article', 'All zeros', 'Content', $2, 'test', $3::vector, NOW(), NOW())
	`, zeroID, fmt.Sprintf("hash-%s", zeroID), string(zeroJSON))
	if err != nil {
		t.Fatalf("insert zero-embedding artifact: %v", err)
	}

	// Insert a normal artifact to search for
	normalID := testID(t) + "-normal"
	cleanupArtifact(t, pool, normalID)

	normalEmb := make([]float32, 384)
	for i := range normalEmb {
		normalEmb[i] = float32(i) / 384.0
	}
	normalJSON, _ := json.Marshal(normalEmb)

	_, err = pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, embedding, created_at, updated_at)
		VALUES ($1, 'article', 'Normal Embedding Article', 'Good data', 'Content', $2, 'test', $3::vector, NOW(), NOW())
	`, normalID, fmt.Sprintf("hash-%s", normalID), string(normalJSON))
	if err != nil {
		t.Fatalf("insert normal artifact: %v", err)
	}

	// Search using the normal embedding as query — must not crash even with
	// the zero-embedding artifact in the table.
	rows, err := pool.Query(ctx, `
		SELECT id, 1 - (embedding <=> $1::vector) AS similarity
		FROM artifacts
		WHERE embedding IS NOT NULL AND id IN ($2, $3)
		ORDER BY embedding <=> $1::vector
		LIMIT 5
	`, string(normalJSON), zeroID, normalID)
	if err != nil {
		t.Fatalf("vector search with zero embedding present: %v", err)
	}
	defer rows.Close()

	foundNormal := false
	for rows.Next() {
		var id string
		var similarity float64
		if err := rows.Scan(&id, &similarity); err != nil {
			t.Fatalf("scan: %v", err)
		}
		t.Logf("result: id=%s similarity=%.4f isNaN=%v isInf=%v", id, similarity, math.IsNaN(similarity), math.IsInf(similarity, 0))
		if id == normalID {
			foundNormal = true
		}
		// The key assertion: pgvector must not return an error, even if similarity
		// is NaN or Inf for the zero-embedding row. The query must complete.
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows iteration error: %v", err)
	}
	if !foundNormal {
		t.Error("normal artifact not found in search results — zero embedding may have poisoned the query")
	}
	t.Log("vector search completed safely with zero-embedding artifact present")
}

// CHAOS-031-008: Concurrent annotation creation on same artifact.
// Tests that the annotations table and the artifact_annotation_summary
// materialized view handle concurrent writes gracefully. In production,
// multiple sources (Telegram, web, API) may annotate the same artifact
// simultaneously.
func TestAnnotation_Chaos_ConcurrentCreation(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create the target artifact
	artifactID := testID(t)
	cleanupArtifact(t, pool, artifactID)
	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'article', 'Concurrent Annotation Target', 'Test', 'Content', $2, 'test', NOW(), NOW())
	`, artifactID, fmt.Sprintf("hash-%s", artifactID))
	if err != nil {
		t.Fatalf("insert target artifact: %v", err)
	}

	// Concurrently insert annotations from multiple "sources"
	const concurrency = 10
	var wg sync.WaitGroup
	var insertErrors atomic.Int32

	for i := range concurrency {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			annID := fmt.Sprintf("chaos-ann-%s-%d", artifactID[:16], idx)
			cleanupAnnotation(t, pool, annID)

			annType := "rating"
			rating := (idx % 5) + 1 // ratings 1-5
			if idx%3 == 0 {
				annType = "interaction"
			}

			var insertErr error
			if annType == "rating" {
				_, insertErr = pool.Exec(ctx, `
					INSERT INTO annotations (id, artifact_id, annotation_type, rating, source_channel, created_at)
					VALUES ($1, $2, $3, $4, 'chaos-test', NOW())
				`, annID, artifactID, annType, rating)
			} else {
				_, insertErr = pool.Exec(ctx, `
					INSERT INTO annotations (id, artifact_id, annotation_type, interaction_type, source_channel, created_at)
					VALUES ($1, $2, 'interaction', 'made_it', 'chaos-test', NOW())
				`, annID, artifactID)
			}
			if insertErr != nil {
				insertErrors.Add(1)
				t.Logf("concurrent annotation insert %d failed: %v", idx, insertErr)
			}
		}(i)
	}
	wg.Wait()

	if insertErrors.Load() > 0 {
		t.Errorf("expected 0 insert errors, got %d — concurrent annotation creation is unsafe", insertErrors.Load())
	}

	// Verify all annotations exist
	var count int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM annotations WHERE artifact_id = $1", artifactID).Scan(&count)
	if err != nil {
		t.Fatalf("count annotations: %v", err)
	}
	if count != concurrency {
		t.Errorf("expected %d annotations, got %d", concurrency, count)
	}

	// Verify REFRESH MATERIALIZED VIEW CONCURRENTLY doesn't error after burst writes
	_, err = pool.Exec(ctx, "REFRESH MATERIALIZED VIEW CONCURRENTLY artifact_annotation_summary")
	if err != nil {
		t.Fatalf("materialized view refresh after concurrent writes failed: %v", err)
	}

	// Verify the summary is coherent
	var currentRating *int
	var timesUsed int
	err = pool.QueryRow(ctx, `
		SELECT current_rating, times_used FROM artifact_annotation_summary WHERE artifact_id = $1
	`, artifactID).Scan(&currentRating, &timesUsed)
	if err != nil {
		t.Fatalf("query summary after concurrent writes: %v", err)
	}
	t.Logf("concurrent annotation summary: rating=%v times_used=%d (from %d concurrent inserts)", currentRating, timesUsed, concurrency)
}

// CHAOS-031-009: List cascade delete under concurrent item updates.
// Tests that deleting a list while items are being updated concurrently
// does not produce constraint violations or data corruption. The ON DELETE
// CASCADE FK on list_items should handle this atomically, but concurrent
// updates to items (status changes, checked_at) racing against a parent
// list deletion can expose FK enforcement gaps.
func TestList_Chaos_CascadeDeleteDuringConcurrentUpdates(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create source artifact
	artifactID := testID(t)
	cleanupArtifact(t, pool, artifactID)
	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'article', 'Cascade Test Recipe', 'Test', 'Content', $2, 'test', NOW(), NOW())
	`, artifactID, fmt.Sprintf("hash-%s", artifactID))
	if err != nil {
		t.Fatalf("insert artifact: %v", err)
	}

	// Create list with items
	listID := testID(t)
	cleanupList(t, pool, listID)
	_, err = pool.Exec(ctx, `
		INSERT INTO lists (id, list_type, title, status, source_artifact_ids, total_items, checked_items, created_at, updated_at)
		VALUES ($1, 'shopping', 'Cascade Test', 'active', ARRAY[$2]::text[], 5, 0, NOW(), NOW())
	`, listID, artifactID)
	if err != nil {
		t.Fatalf("insert list: %v", err)
	}

	itemIDs := make([]string, 5)
	for i := range 5 {
		itemIDs[i] = fmt.Sprintf("%s-item-%d", listID[:20], i)
		_, err = pool.Exec(ctx, `
			INSERT INTO list_items (id, list_id, content, category, status, source_artifact_ids, sort_order, created_at, updated_at)
			VALUES ($1, $2, $3, 'produce', 'pending', ARRAY[$4]::text[], $5, NOW(), NOW())
		`, itemIDs[i], listID, fmt.Sprintf("Item %d", i), artifactID, i)
		if err != nil {
			t.Fatalf("insert item %d: %v", i, err)
		}
	}

	// Concurrently: update item statuses while deleting the parent list
	var wg sync.WaitGroup
	var updateErrors atomic.Int32
	var deleteError atomic.Value

	// Launch updaters for each item
	for _, itemID := range itemIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, err := pool.Exec(ctx, `
				UPDATE list_items SET status = 'done', checked_at = NOW(), updated_at = NOW() WHERE id = $1
			`, id)
			if err != nil {
				updateErrors.Add(1)
				// Errors are expected: cascade may delete items before update runs
			}
		}(itemID)
	}

	// Launch the parent delete concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := pool.Exec(ctx, "DELETE FROM lists WHERE id = $1", listID)
		if err != nil {
			deleteError.Store(err)
		}
	}()

	wg.Wait()

	// The delete must succeed
	if de := deleteError.Load(); de != nil {
		t.Fatalf("cascade delete failed: %v", de)
	}

	// After cascade, no list or items should remain
	var listCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM lists WHERE id = $1", listID).Scan(&listCount)
	if err != nil {
		t.Fatalf("count lists: %v", err)
	}
	if listCount != 0 {
		t.Errorf("expected 0 lists after delete, got %d", listCount)
	}

	var itemCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM list_items WHERE list_id = $1", listID).Scan(&itemCount)
	if err != nil {
		t.Fatalf("count items: %v", err)
	}
	if itemCount != 0 {
		t.Errorf("expected 0 items after cascade delete, got %d — FK cascade broken", itemCount)
	}

	t.Logf("cascade delete verified: list deleted, %d update errors (expected for race), 0 orphaned items", updateErrors.Load())
}

// CHAOS-031-010: Vector embedding dimension mismatch.
// The embedding column is vector(384). Inserting a 768-dim embedding MUST be
// rejected by pgvector — if it silently truncates or accepts wrong-dimension
// data, every subsequent cosine distance calculation is corrupted.
// This simulates a model upgrade (e.g., MiniLM-L6 384-dim → a 768-dim model)
// where the application code forgets to update the schema.
func TestArtifact_Chaos_EmbeddingDimensionMismatch(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	id := testID(t) + "-dimfail"
	cleanupArtifact(t, pool, id)

	// 768-dim embedding — wrong for the vector(384) column
	wrongDimEmb := make([]float32, 768)
	for i := range wrongDimEmb {
		wrongDimEmb[i] = float32(i) / 768.0
	}
	wrongJSON, _ := json.Marshal(wrongDimEmb)

	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, embedding, created_at, updated_at)
		VALUES ($1, 'article', 'Wrong Dim Article', 'Bad embedding', 'Content', $2, 'test', $3::vector, NOW(), NOW())
	`, id, fmt.Sprintf("hash-%s", id), string(wrongJSON))

	if err == nil {
		t.Fatal("expected error when inserting 768-dim embedding into vector(384) column, but INSERT succeeded — dimension mismatch is silently accepted")
	}

	// Verify it's a dimension mismatch error from pgvector
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		t.Logf("correctly rejected: SQLSTATE=%s message=%s", pgErr.Code, pgErr.Message)
	} else {
		t.Logf("rejected with non-pg error: %v", err)
	}

	// Also test undersized embedding (128-dim)
	shortEmb := make([]float32, 128)
	for i := range shortEmb {
		shortEmb[i] = float32(i) / 128.0
	}
	shortJSON, _ := json.Marshal(shortEmb)

	shortID := testID(t) + "-shortdim"
	cleanupArtifact(t, pool, shortID)

	_, err = pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, embedding, created_at, updated_at)
		VALUES ($1, 'article', 'Short Dim Article', 'Too few dims', 'Content', $2, 'test', $3::vector, NOW(), NOW())
	`, shortID, fmt.Sprintf("hash-%s", shortID), string(shortJSON))

	if err == nil {
		t.Fatal("expected error when inserting 128-dim embedding into vector(384) column, but INSERT succeeded")
	}
	t.Logf("undersized embedding correctly rejected: %v", err)
}

// CHAOS-031-011: Annotation rating upper boundary (rating=6).
// The chk_rating_range constraint is: rating IS NULL OR (rating >= 1 AND rating <= 5).
// The existing test only checks rating=0 (lower boundary). This test verifies the
// upper boundary (rating=6) is also rejected. Without this, a constraint widening
// during a schema migration could go undetected.
func TestAnnotation_Chaos_RatingBoundary(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Need an artifact to attach annotations to
	artifactID := testID(t)
	cleanupArtifact(t, pool, artifactID)
	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, summary, content_raw, content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'article', 'Rating Boundary Test', 'Test', 'Content', $2, 'test', NOW(), NOW())
	`, artifactID, fmt.Sprintf("hash-%s", artifactID))
	if err != nil {
		t.Fatalf("insert artifact: %v", err)
	}

	// Test cases: all should be rejected by chk_rating_range
	invalidRatings := []struct {
		name   string
		rating int
	}{
		{"upper_boundary_6", 6},
		{"large_value_100", 100},
		{"negative_minus1", -1},
		{"lower_boundary_0", 0},
	}

	for _, tc := range invalidRatings {
		t.Run(tc.name, func(t *testing.T) {
			annID := fmt.Sprintf("chaos-rating-%s-%s", tc.name, artifactID[:12])
			cleanupAnnotation(t, pool, annID)

			_, err := pool.Exec(ctx, `
				INSERT INTO annotations (id, artifact_id, annotation_type, rating, source_channel, created_at)
				VALUES ($1, $2, 'rating', $3, 'chaos-test', NOW())
			`, annID, artifactID, tc.rating)
			if err == nil {
				t.Errorf("expected constraint violation for rating=%d, but INSERT succeeded", tc.rating)
				// Clean up the incorrectly accepted row
				pool.Exec(ctx, "DELETE FROM annotations WHERE id = $1", annID)
			} else {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.ConstraintName == "chk_rating_range" {
					t.Logf("rating=%d correctly rejected by chk_rating_range", tc.rating)
				} else {
					t.Logf("rating=%d rejected with: %v", tc.rating, err)
				}
			}
		})
	}

	// Valid boundary values must succeed
	validRatings := []struct {
		name   string
		rating int
	}{
		{"lower_valid_1", 1},
		{"upper_valid_5", 5},
		{"mid_value_3", 3},
	}

	for _, tc := range validRatings {
		t.Run(tc.name, func(t *testing.T) {
			annID := fmt.Sprintf("chaos-valid-%s-%s", tc.name, artifactID[:12])
			cleanupAnnotation(t, pool, annID)

			_, err := pool.Exec(ctx, `
				INSERT INTO annotations (id, artifact_id, annotation_type, rating, source_channel, created_at)
				VALUES ($1, $2, 'rating', $3, 'chaos-test', NOW())
			`, annID, artifactID, tc.rating)
			if err != nil {
				t.Errorf("expected rating=%d to be accepted, got error: %v", tc.rating, err)
			}
		})
	}
}

// CHAOS-031-012: Concurrent REFRESH MATERIALIZED VIEW CONCURRENTLY.
// In production, multiple scheduler ticks, API handlers, or background jobs
// could trigger annotation summary refresh simultaneously. PostgreSQL uses a
// ShareUpdateExclusiveLock for CONCURRENTLY refreshes — concurrent calls block
// rather than fail, but under heavy load the blocking could cascade into timeouts.
// This test verifies that concurrent refreshes complete without errors or deadlocks.
func TestAnnotation_Chaos_ConcurrentMaterializedViewRefresh(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const concurrency = 5
	var wg sync.WaitGroup
	var refreshErrors atomic.Int32
	var successCount atomic.Int32

	for i := range concurrency {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := pool.Exec(ctx, "REFRESH MATERIALIZED VIEW CONCURRENTLY artifact_annotation_summary")
			if err != nil {
				refreshErrors.Add(1)
				t.Logf("concurrent refresh %d failed: %v", idx, err)
			} else {
				successCount.Add(1)
			}
		}(i)
	}
	wg.Wait()

	successes := successCount.Load()
	failures := refreshErrors.Load()
	t.Logf("concurrent matview refreshes: successes=%d failures=%d", successes, failures)

	if failures > 0 {
		t.Errorf("expected all concurrent REFRESH MATERIALIZED VIEW CONCURRENTLY to succeed (blocking is OK, errors are not), got %d failures", failures)
	}
	if successes != int32(concurrency) {
		t.Errorf("expected %d successful refreshes, got %d", concurrency, successes)
	}
}
