//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"
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
