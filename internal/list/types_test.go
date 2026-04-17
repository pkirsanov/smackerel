package list

import (
	"encoding/json"
	"testing"
	"time"
)

func TestListType_Constants(t *testing.T) {
	types := []ListType{TypeShopping, TypeReading, TypeComparison, TypePacking, TypeChecklist, TypeCustom}
	seen := make(map[ListType]bool)
	for _, lt := range types {
		if lt == "" {
			t.Error("list type constant is empty")
		}
		if seen[lt] {
			t.Errorf("duplicate list type: %s", lt)
		}
		seen[lt] = true
	}
}

func TestListStatus_Constants(t *testing.T) {
	statuses := []ListStatus{StatusDraft, StatusActive, StatusCompleted, StatusArchived}
	seen := make(map[ListStatus]bool)
	for _, s := range statuses {
		if s == "" {
			t.Error("list status constant is empty")
		}
		if seen[s] {
			t.Errorf("duplicate list status: %s", s)
		}
		seen[s] = true
	}
}

func TestItemStatus_Constants(t *testing.T) {
	statuses := []ItemStatus{ItemPending, ItemDone, ItemSkipped, ItemSubstituted}
	seen := make(map[ItemStatus]bool)
	for _, s := range statuses {
		if s == "" {
			t.Error("item status constant is empty")
		}
		if seen[s] {
			t.Errorf("duplicate item status: %s", s)
		}
		seen[s] = true
	}
}

func TestList_JSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	list := List{
		ID:                "lst-001",
		ListType:          TypeShopping,
		Title:             "Weekend cooking",
		Status:            StatusActive,
		SourceArtifactIDs: []string{"art-001", "art-002"},
		Domain:            "recipe",
		TotalItems:        15,
		CheckedItems:      3,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	data, err := json.Marshal(list)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded List
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != list.ID {
		t.Errorf("ID mismatch: %s vs %s", decoded.ID, list.ID)
	}
	if decoded.ListType != TypeShopping {
		t.Errorf("ListType mismatch: %s", decoded.ListType)
	}
	if len(decoded.SourceArtifactIDs) != 2 {
		t.Errorf("SourceArtifactIDs length: %d", len(decoded.SourceArtifactIDs))
	}
	if decoded.TotalItems != 15 {
		t.Errorf("TotalItems: %d", decoded.TotalItems)
	}
}

func TestListItem_JSONRoundTrip(t *testing.T) {
	qty := 5.0
	item := ListItem{
		ID:                "itm-001",
		ListID:            "lst-001",
		Content:           "5 cloves garlic",
		Category:          "produce",
		Status:            ItemPending,
		SourceArtifactIDs: []string{"art-001"},
		Quantity:          &qty,
		Unit:              "cloves",
		NormalizedName:    "garlic",
		SortOrder:         1,
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ListItem
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Content != "5 cloves garlic" {
		t.Errorf("Content: %s", decoded.Content)
	}
	if decoded.Quantity == nil || *decoded.Quantity != 5.0 {
		t.Errorf("Quantity: %v", decoded.Quantity)
	}
	if decoded.Status != ItemPending {
		t.Errorf("Status: %s", decoded.Status)
	}
}

func TestListWithItems_JSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	lwi := ListWithItems{
		List: List{
			ID:       "lst-001",
			ListType: TypeReading,
			Title:    "My reading list",
			Status:   StatusDraft,
		},
		Items: []ListItem{
			{ID: "itm-001", Content: "Article about Go", SortOrder: 0, CreatedAt: now, UpdatedAt: now},
			{ID: "itm-002", Content: "Article about Rust", SortOrder: 1, CreatedAt: now, UpdatedAt: now},
		},
	}

	data, err := json.Marshal(lwi)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ListWithItems
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(decoded.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(decoded.Items))
	}
}

func TestAggregationSource_RawJSON(t *testing.T) {
	src := AggregationSource{
		ArtifactID: "art-001",
		DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"flour"}]}`),
	}

	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded AggregationSource
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ArtifactID != "art-001" {
		t.Errorf("ArtifactID: %s", decoded.ArtifactID)
	}
	if string(decoded.DomainData) == "" {
		t.Error("DomainData is empty after roundtrip")
	}
}

func TestListItemSeed_NilQuantity(t *testing.T) {
	seed := ListItemSeed{
		Content:        "a pinch of salt",
		Category:       "spices",
		NormalizedName: "salt",
	}

	data, err := json.Marshal(seed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ListItemSeed
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Quantity != nil {
		t.Error("expected nil Quantity for unmeasured ingredient")
	}
}
