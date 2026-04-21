package list

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// mockArtifactResolver is a test double for ArtifactResolver.
type mockArtifactResolver struct {
	byIDs   map[string][]AggregationSource
	byTag   map[string][]AggregationSource
	byQuery map[string][]AggregationSource
}

func (m *mockArtifactResolver) ResolveByIDs(ctx context.Context, ids []string) ([]AggregationSource, error) {
	var result []AggregationSource
	for _, id := range ids {
		if sources, ok := m.byIDs[id]; ok {
			result = append(result, sources...)
		}
	}
	return result, nil
}

func (m *mockArtifactResolver) ResolveByTag(ctx context.Context, tag string) ([]AggregationSource, error) {
	if sources, ok := m.byTag[tag]; ok {
		return sources, nil
	}
	return nil, nil
}

func (m *mockArtifactResolver) ResolveByQuery(ctx context.Context, query string) ([]AggregationSource, error) {
	if sources, ok := m.byQuery[query]; ok {
		return sources, nil
	}
	return nil, nil
}

// mockStore implements ListStore for testing without a real database.
type mockStore struct {
	lists []List
	items map[string][]ListItem
}

func newMockStore() *mockStore {
	return &mockStore{items: make(map[string][]ListItem)}
}

func (m *mockStore) CreateList(_ context.Context, list *List, items []ListItem) error {
	list.TotalItems = len(items)
	m.lists = append(m.lists, *list)
	m.items[list.ID] = items
	return nil
}

func (m *mockStore) GetList(_ context.Context, listID string) (*ListWithItems, error) {
	for _, l := range m.lists {
		if l.ID == listID {
			return &ListWithItems{List: l, Items: m.items[listID]}, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockStore) ListLists(_ context.Context, _, _ string, _, _ int) ([]List, error) {
	return m.lists, nil
}

func (m *mockStore) UpdateItemStatus(_ context.Context, _, _ string, _ ItemStatus, _ string) error {
	return nil
}

func (m *mockStore) AddManualItem(_ context.Context, listID, content, category string) (*ListItem, error) {
	item := &ListItem{ID: "mock-item", ListID: listID, Content: content, Category: category}
	return item, nil
}

func (m *mockStore) CompleteList(_ context.Context, _ string) error  { return nil }
func (m *mockStore) ArchiveList(_ context.Context, _ string) error   { return nil }
func (m *mockStore) RemoveItem(_ context.Context, _, _ string) error { return nil }

func TestGenerator_GenerateFromIDs(t *testing.T) {
	recipeSrc1 := AggregationSource{
		ArtifactID: "art-1",
		DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"garlic","quantity":"2","unit":"cloves"}]}`),
	}
	recipeSrc2 := AggregationSource{
		ArtifactID: "art-2",
		DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"garlic","quantity":"3","unit":"cloves"}]}`),
	}

	resolver := &mockArtifactResolver{
		byIDs: map[string][]AggregationSource{
			"art-1": {recipeSrc1},
			"art-2": {recipeSrc2},
		},
	}

	aggregators := map[string]Aggregator{
		"recipe": &RecipeAggregator{},
	}

	store := newMockStore()
	gen := NewGenerator(resolver, store, aggregators)

	result, err := gen.Generate(context.Background(), GenerateRequest{
		ListType:    TypeShopping,
		Title:       "Weeknight Groceries",
		ArtifactIDs: []string{"art-1", "art-2"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.List.Title != "Weeknight Groceries" {
		t.Errorf("expected title 'Weeknight Groceries', got %q", result.List.Title)
	}
	if result.List.ListType != TypeShopping {
		t.Errorf("expected type shopping, got %s", result.List.ListType)
	}
	if result.List.Status != StatusDraft {
		t.Errorf("expected status draft, got %s", result.List.Status)
	}
	if len(result.List.SourceArtifactIDs) != 2 {
		t.Errorf("expected 2 source artifact IDs, got %d", len(result.List.SourceArtifactIDs))
	}
	if result.List.Domain != "recipe" {
		t.Errorf("expected domain 'recipe', got %q", result.List.Domain)
	}
	// Recipe aggregator should merge the duplicate garlic
	if len(result.Items) != 1 {
		t.Errorf("expected 1 merged item (garlic), got %d", len(result.Items))
	}
	if len(store.lists) != 1 {
		t.Errorf("expected 1 list persisted, got %d", len(store.lists))
	}
}

func TestGenerator_GenerateFromTagFilter(t *testing.T) {
	recipeSrc := AggregationSource{
		ArtifactID: "art-tag-1",
		DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"flour","quantity":"2","unit":"cups"}]}`),
	}

	resolver := &mockArtifactResolver{
		byTag: map[string][]AggregationSource{
			"#weeknight": {recipeSrc},
		},
	}

	aggregators := map[string]Aggregator{
		"recipe": &RecipeAggregator{},
	}

	store := newMockStore()
	gen := NewGenerator(resolver, store, aggregators)

	result, err := gen.Generate(context.Background(), GenerateRequest{
		ListType:  TypeShopping,
		Title:     "Tag Filter List",
		TagFilter: "#weeknight",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) == 0 {
		t.Fatal("expected at least one item")
	}
	if len(store.lists) != 1 {
		t.Errorf("expected 1 list persisted, got %d", len(store.lists))
	}
}

func TestGenerator_RejectMixedDomains(t *testing.T) {
	resolver := &mockArtifactResolver{
		byIDs: map[string][]AggregationSource{
			"art-recipe": {{
				ArtifactID: "art-recipe",
				DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[]}`),
			}},
			"art-product": {{
				ArtifactID: "art-product",
				DomainData: json.RawMessage(`{"domain":"product","product_name":"Widget"}`),
			}},
		},
	}

	aggregators := map[string]Aggregator{
		"recipe":  &RecipeAggregator{},
		"product": &CompareAggregator{},
	}

	store := newMockStore()
	gen := NewGenerator(resolver, store, aggregators)

	_, err := gen.Generate(context.Background(), GenerateRequest{
		ListType:    TypeShopping,
		Title:       "Mixed List",
		ArtifactIDs: []string{"art-recipe", "art-product"},
	})

	if err == nil {
		t.Fatal("expected error for mixed domains")
	}

	if got := err.Error(); !contains(got, "incompatible domains") {
		t.Fatalf("expected incompatible domains error, got: %s", got)
	}
}

func TestGenerator_HandlesMissingDomainData(t *testing.T) {
	// Only one of 3 artifacts has domain_data
	resolver := &mockArtifactResolver{
		byIDs: map[string][]AggregationSource{
			"art-1": {{
				ArtifactID: "art-1",
				DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"butter","quantity":"1","unit":"tbsp"}]}`),
			}},
			// art-2 and art-3 would not be returned by resolver since they have no domain_data
		},
	}

	aggregators := map[string]Aggregator{
		"recipe": &RecipeAggregator{},
	}

	store := newMockStore()
	gen := NewGenerator(resolver, store, aggregators)

	result, err := gen.Generate(context.Background(), GenerateRequest{
		ListType:    TypeShopping,
		Title:       "Partial Data List",
		ArtifactIDs: []string{"art-1", "art-2", "art-3"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should proceed with the 1 resolved artifact
	if len(result.List.SourceArtifactIDs) != 1 {
		t.Errorf("expected 1 source artifact, got %d", len(result.List.SourceArtifactIDs))
	}
}

func TestGenerator_NoArtifactsFound(t *testing.T) {
	resolver := &mockArtifactResolver{
		byIDs: map[string][]AggregationSource{},
	}

	aggregators := map[string]Aggregator{
		"recipe": &RecipeAggregator{},
	}

	store := newMockStore()
	gen := NewGenerator(resolver, store, aggregators)

	_, err := gen.Generate(context.Background(), GenerateRequest{
		ListType:    TypeShopping,
		Title:       "Empty List",
		ArtifactIDs: []string{"nonexistent"},
	})

	if err == nil {
		t.Fatal("expected error for no artifacts")
	}
	if got := err.Error(); !contains(got, "no artifacts with domain_data found") {
		t.Fatalf("expected no-artifacts error, got: %s", got)
	}
}

func TestGenerator_MissingTitle(t *testing.T) {
	resolver := &mockArtifactResolver{}
	aggregators := map[string]Aggregator{}
	store := newMockStore()
	gen := NewGenerator(resolver, store, aggregators)

	_, err := gen.Generate(context.Background(), GenerateRequest{
		ListType:    TypeShopping,
		ArtifactIDs: []string{"art-1"},
	})

	if err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestGenerator_NoAggregatorForDomain(t *testing.T) {
	resolver := &mockArtifactResolver{
		byIDs: map[string][]AggregationSource{
			"art-1": {{
				ArtifactID: "art-1",
				DomainData: json.RawMessage(`{"domain":"unknown_domain","data":"stuff"}`),
			}},
		},
	}

	// No aggregator registered for "unknown_domain"
	aggregators := map[string]Aggregator{
		"recipe": &RecipeAggregator{},
	}

	store := newMockStore()
	gen := NewGenerator(resolver, store, aggregators)

	_, err := gen.Generate(context.Background(), GenerateRequest{
		ListType:    TypeShopping,
		Title:       "Unknown Domain",
		ArtifactIDs: []string{"art-1"},
	})

	if err == nil {
		t.Fatal("expected error for unknown aggregator domain")
	}
	if got := err.Error(); !contains(got, "no aggregator registered") {
		t.Fatalf("expected aggregator error, got: %s", got)
	}
}

func TestGenerator_DefaultListType(t *testing.T) {
	// Verify that when ListType is empty, it uses the aggregator's default
	resolver := &mockArtifactResolver{
		byIDs: map[string][]AggregationSource{
			"art-1": {{
				ArtifactID: "art-1",
				DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"salt","quantity":"1","unit":"tsp"}]}`),
			}},
		},
	}

	aggregators := map[string]Aggregator{
		"recipe": &RecipeAggregator{},
	}

	store := newMockStore()
	gen := NewGenerator(resolver, store, aggregators)

	result, err := gen.Generate(context.Background(), GenerateRequest{
		Title:       "Default Type Test",
		ArtifactIDs: []string{"art-1"},
		// ListType intentionally empty
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// RecipeAggregator defaults to TypeShopping
	if result.List.ListType != TypeShopping {
		t.Errorf("expected default list type %s, got %s", TypeShopping, result.List.ListType)
	}
}

func TestValidateDomains_SingleDomain(t *testing.T) {
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"recipe"}`)},
		{ArtifactID: "a2", DomainData: json.RawMessage(`{"domain":"recipe"}`)},
	}

	domain, err := validateDomains(sources)
	if err != nil {
		t.Fatal(err)
	}
	if domain != "recipe" {
		t.Fatalf("expected domain 'recipe', got '%s'", domain)
	}
}

func TestValidateDomains_MixedDomains(t *testing.T) {
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"recipe"}`)},
		{ArtifactID: "a2", DomainData: json.RawMessage(`{"domain":"product"}`)},
	}

	_, err := validateDomains(sources)
	if err == nil {
		t.Fatal("expected error for mixed domains")
	}
}

func TestValidateDomains_NoDomainField(t *testing.T) {
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"ingredients":[]}`)},
	}

	_, err := validateDomains(sources)
	if err == nil {
		t.Fatal("expected error when no domain field present")
	}
}

func TestDomainFromData(t *testing.T) {
	cases := []struct {
		data   string
		domain string
	}{
		{`{"domain":"recipe"}`, "recipe"},
		{`{"domain":"product"}`, "product"},
		{`{"other":"field"}`, ""},
		{`invalid`, ""},
	}

	for _, tc := range cases {
		got := domainFromData(json.RawMessage(tc.data))
		if got != tc.domain {
			t.Errorf("domainFromData(%s) = %q, want %q", tc.data, got, tc.domain)
		}
	}
}

func TestGenerator_DeduplicatesArtifacts(t *testing.T) {
	// Same artifact returned by both IDs and tag
	src := AggregationSource{
		ArtifactID: "art-dup",
		DomainData: json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"onion","quantity":"1","unit":""}]}`),
	}

	resolver := &mockArtifactResolver{
		byIDs: map[string][]AggregationSource{
			"art-dup": {src},
		},
		byTag: map[string][]AggregationSource{
			"#dinner": {src},
		},
	}

	aggregators := map[string]Aggregator{
		"recipe": &RecipeAggregator{},
	}

	store := newMockStore()
	gen := NewGenerator(resolver, store, aggregators)

	result, err := gen.Generate(context.Background(), GenerateRequest{
		Title:       "Dedup Test",
		ArtifactIDs: []string{"art-dup"},
		TagFilter:   "#dinner",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Artifact should be deduplicated: only 1 source even though it appears in both IDs and tag
	if len(result.List.SourceArtifactIDs) != 1 {
		t.Errorf("expected 1 source artifact (deduplicated), got %d", len(result.List.SourceArtifactIDs))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
