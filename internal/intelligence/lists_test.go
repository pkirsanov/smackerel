package intelligence

import (
	"encoding/json"
	"testing"
)

func TestHandleListCompleted_UnmarshalEvent(t *testing.T) {
	event := ListCompletedEvent{
		ListID:            "lst-123",
		ListType:          "shopping",
		Title:             "Weeknight Groceries",
		SourceArtifactIDs: []string{"art-1", "art-2"},
		TotalItems:        10,
		CheckedItems:      8,
		CompletedAt:       "2026-04-17T20:00:00Z",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}

	var decoded ListCompletedEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.ListID != "lst-123" {
		t.Errorf("expected list_id 'lst-123', got %q", decoded.ListID)
	}
	if decoded.ListType != "shopping" {
		t.Errorf("expected list_type 'shopping', got %q", decoded.ListType)
	}
	if len(decoded.SourceArtifactIDs) != 2 {
		t.Errorf("expected 2 source artifacts, got %d", len(decoded.SourceArtifactIDs))
	}
	if decoded.TotalItems != 10 {
		t.Errorf("expected total_items 10, got %d", decoded.TotalItems)
	}
	if decoded.CheckedItems != 8 {
		t.Errorf("expected checked_items 8, got %d", decoded.CheckedItems)
	}
}

func TestHandleListCompleted_InvalidJSON(t *testing.T) {
	engine := &Engine{}
	err := engine.HandleListCompleted(nil, []byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestHandleListCompleted_NilPool(t *testing.T) {
	// Engine with nil Pool should not panic — just log warnings
	engine := &Engine{}
	event := ListCompletedEvent{
		ListID:            "lst-1",
		ListType:          "shopping",
		SourceArtifactIDs: []string{"art-1"},
	}
	data, _ := json.Marshal(event)

	// Should not panic even with nil pool
	err := engine.HandleListCompleted(nil, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBoostArtifactRelevance_NilPool(t *testing.T) {
	engine := &Engine{}
	err := engine.boostArtifactRelevance(nil, []string{"art-1"})
	if err != nil {
		t.Fatalf("expected nil error for nil pool, got: %v", err)
	}
}

func TestBoostArtifactRelevance_EmptyIDs(t *testing.T) {
	engine := &Engine{}
	err := engine.boostArtifactRelevance(nil, []string{})
	if err != nil {
		t.Fatalf("expected nil error for empty IDs, got: %v", err)
	}
}

func TestTrackPurchaseFrequency_NilPool(t *testing.T) {
	engine := &Engine{}
	err := engine.trackPurchaseFrequency(nil, "lst-1")
	if err != nil {
		t.Fatalf("expected nil error for nil pool, got: %v", err)
	}
}

func TestPurchaseFrequency_Struct(t *testing.T) {
	pf := PurchaseFrequency{
		NormalizedName: "garlic",
		Count:          5,
	}
	data, err := json.Marshal(pf)
	if err != nil {
		t.Fatal(err)
	}

	var decoded PurchaseFrequency
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.NormalizedName != "garlic" {
		t.Errorf("expected 'garlic', got %q", decoded.NormalizedName)
	}
	if decoded.Count != 5 {
		t.Errorf("expected count 5, got %d", decoded.Count)
	}
}

func TestSubscribeListsCompleted_NilNATS(t *testing.T) {
	engine := &Engine{}
	err := engine.SubscribeListsCompleted(nil)
	if err == nil {
		t.Fatal("expected error for nil NATS client")
	}
}
