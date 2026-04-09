package bookmarks

import (
	"context"
	"testing"
)

// T-2-10: Empty folder path returns nil
func TestMapFolder_EmptyPath(t *testing.T) {
	tm := NewTopicMapper(nil)

	tests := []string{"", "  ", "   "}
	for _, path := range tests {
		matches, err := tm.MapFolder(context.Background(), path)
		if err != nil {
			t.Errorf("MapFolder(%q) error: %v", path, err)
		}
		if matches != nil {
			t.Errorf("MapFolder(%q) = %v, want nil", path, matches)
		}
	}
}

// Test nil pool graceful handling
func TestTopicMapper_NilPool(t *testing.T) {
	tm := NewTopicMapper(nil)

	// MapFolder with nil pool returns nil
	matches, err := tm.MapFolder(context.Background(), "Tech/Go")
	if err != nil {
		t.Errorf("MapFolder() error: %v", err)
	}
	if matches != nil {
		t.Errorf("MapFolder() = %v, want nil", matches)
	}

	// CreateTopicEdge with nil pool is a no-op
	if err := tm.CreateTopicEdge(context.Background(), "art-1", "topic-1"); err != nil {
		t.Errorf("CreateTopicEdge() error: %v", err)
	}

	// CreateParentEdge with nil pool is a no-op
	if err := tm.CreateParentEdge(context.Background(), "child", "parent"); err != nil {
		t.Errorf("CreateParentEdge() error: %v", err)
	}

	// UpdateTopicMomentum with nil pool is a no-op
	if err := tm.UpdateTopicMomentum(context.Background(), "topic-1"); err != nil {
		t.Errorf("UpdateTopicMomentum() error: %v", err)
	}
}

// Test TopicMatch struct construction
func TestTopicMatch_Fields(t *testing.T) {
	m := TopicMatch{
		FolderName: "Tech",
		TopicID:    "01ABCDEF",
		TopicName:  "Technology",
		MatchType:  "fuzzy",
	}

	if m.FolderName != "Tech" {
		t.Errorf("FolderName = %q, want %q", m.FolderName, "Tech")
	}
	if m.TopicID != "01ABCDEF" {
		t.Errorf("TopicID = %q, want %q", m.TopicID, "01ABCDEF")
	}
	if m.TopicName != "Technology" {
		t.Errorf("TopicName = %q, want %q", m.TopicName, "Technology")
	}
	if m.MatchType != "fuzzy" {
		t.Errorf("MatchType = %q, want %q", m.MatchType, "fuzzy")
	}
}
