//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector/bookmarks"
)

// T-2-09: TopicMapper resolves an exact match for a known topic.
func TestTopicMapper_ExactMatch(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tid := testID(t)
	topicName := "TestTopic-" + tid

	// Insert a known topic
	_, err := pool.Exec(ctx, `
		INSERT INTO topics (id, name, state, capture_count_total, capture_count_30d, capture_count_90d, search_hit_count_30d)
		VALUES ($1, $2, 'active', 0, 0, 0, 0)
		ON CONFLICT DO NOTHING
	`, "topic-"+tid, topicName)
	if err != nil {
		t.Fatalf("insert topic: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM topics WHERE id = $1`, "topic-"+tid)

	tm := bookmarks.NewTopicMapper(pool)
	matches, err := tm.MapFolder(ctx, topicName)
	if err != nil {
		t.Fatalf("MapFolder() error: %v", err)
	}

	if len(matches) != 1 {
		t.Fatalf("matches len = %d, want 1", len(matches))
	}
	if matches[0].MatchType != "exact" {
		t.Errorf("MatchType = %q, want %q", matches[0].MatchType, "exact")
	}
	if matches[0].TopicName != topicName {
		t.Errorf("TopicName = %q, want %q", matches[0].TopicName, topicName)
	}
}

// T-2-11: TopicMapper creates a new topic when no match exists.
func TestTopicMapper_CreatesNewTopic(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tid := testID(t)
	folderName := "NewFolder-" + tid

	tm := bookmarks.NewTopicMapper(pool)
	matches, err := tm.MapFolder(ctx, folderName)
	if err != nil {
		t.Fatalf("MapFolder() error: %v", err)
	}

	if len(matches) != 1 {
		t.Fatalf("matches len = %d, want 1", len(matches))
	}
	if matches[0].MatchType != "created" {
		t.Errorf("MatchType = %q, want %q", matches[0].MatchType, "created")
	}
	if matches[0].TopicID == "" {
		t.Error("TopicID is empty, expected a new ID")
	}

	// Cleanup the created topic
	defer pool.Exec(ctx, `DELETE FROM topics WHERE id = $1`, matches[0].TopicID)

	// Verify it was persisted
	var name string
	err = pool.QueryRow(ctx, `SELECT name FROM topics WHERE id = $1`, matches[0].TopicID).Scan(&name)
	if err != nil {
		t.Fatalf("query created topic: %v", err)
	}
	if name != folderName {
		t.Errorf("persisted name = %q, want %q", name, folderName)
	}
}

// T-2-12: TopicMapper handles hierarchical paths with parent-child edges.
func TestTopicMapper_HierarchicalPath(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tid := testID(t)
	folderPath := "ParentFolder-" + tid + "/ChildFolder-" + tid

	tm := bookmarks.NewTopicMapper(pool)
	matches, err := tm.MapFolder(ctx, folderPath)
	if err != nil {
		t.Fatalf("MapFolder() error: %v", err)
	}

	if len(matches) != 2 {
		t.Fatalf("matches len = %d, want 2 (parent + child)", len(matches))
	}

	// Cleanup
	for _, m := range matches {
		defer pool.Exec(ctx, `DELETE FROM topics WHERE id = $1`, m.TopicID)
	}

	if matches[0].FolderName != "ParentFolder-"+tid {
		t.Errorf("matches[0].FolderName = %q", matches[0].FolderName)
	}
	if matches[1].FolderName != "ChildFolder-"+tid {
		t.Errorf("matches[1].FolderName = %q", matches[1].FolderName)
	}

	// Both should be created (no pre-existing topics with these names)
	if matches[0].MatchType != "created" {
		t.Errorf("matches[0].MatchType = %q, want created", matches[0].MatchType)
	}
	if matches[1].MatchType != "created" {
		t.Errorf("matches[1].MatchType = %q, want created", matches[1].MatchType)
	}
}
