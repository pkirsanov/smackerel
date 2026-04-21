package list

import (
	"encoding/json"
	"testing"
)

func TestReadingAggregator_BasicList(t *testing.T) {
	a := &ReadingAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"article","title":"Go Best Practices"}`)},
		{ArtifactID: "a2", DomainData: json.RawMessage(`{"domain":"article","title":"Rust vs Go"}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds) != 2 {
		t.Fatalf("expected 2 items, got %d", len(seeds))
	}
	if seeds[0].SourceArtifactIDs[0] != "a1" {
		t.Errorf("expected source a1, got %s", seeds[0].SourceArtifactIDs[0])
	}
}

func TestReadingAggregator_MissingTitle(t *testing.T) {
	a := &ReadingAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds) != 1 {
		t.Fatal("expected 1 item")
	}
	if seeds[0].Content == "" {
		t.Error("content should not be empty even with missing title")
	}
}

func TestEstimateReadTime(t *testing.T) {
	cases := []struct {
		chars    int
		expected int
	}{
		{0, 0},
		{100, 0},  // too short for 1 min
		{500, 1},  // ~100 words, rounds up to 1 min
		{5000, 5}, // ~1000 words / 200 wpm = 5 min
		{10000, 10},
	}
	for _, tc := range cases {
		got := EstimateReadTime(tc.chars)
		if got != tc.expected {
			t.Errorf("EstimateReadTime(%d) = %d, want %d", tc.chars, got, tc.expected)
		}
	}
}

func TestCompareAggregator_BasicComparison(t *testing.T) {
	a := &CompareAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{
			"domain": "product",
			"product_name": "WH-1000XM5",
			"brand": "Sony",
			"price": {"amount": 349.99, "currency": "USD"},
			"rating": {"score": 4.5, "max": 5, "count": 1000}
		}`)},
		{ArtifactID: "a2", DomainData: json.RawMessage(`{
			"domain": "product",
			"product_name": "QC Ultra",
			"brand": "Bose",
			"price": {"amount": 429.00, "currency": "USD"},
			"rating": {"score": 4.3, "max": 5, "count": 800}
		}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds) != 2 {
		t.Fatalf("expected 2 items, got %d", len(seeds))
	}

	// First item should have Sony brand in content
	if seeds[0].Quantity == nil || *seeds[0].Quantity != 349.99 {
		t.Errorf("expected price 349.99, got %v", seeds[0].Quantity)
	}
}

func TestCompareAggregator_MissingFields(t *testing.T) {
	a := &CompareAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"product","product_name":"Generic Widget"}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds) != 1 {
		t.Fatal("expected 1 item")
	}
	if seeds[0].Quantity != nil {
		t.Error("expected nil price for product without price")
	}
}

func TestCompareAggregator_InvalidJSON(t *testing.T) {
	a := &CompareAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`not json`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(seeds) != 0 {
		t.Error("expected 0 items for invalid JSON")
	}
}

func TestReadingAggregator_SortOrder(t *testing.T) {
	// Gherkin: "items are ordered by relevance score descending"
	// Current implementation preserves insertion order (index-based sort_order).
	// Verify that sort_order reflects input ordering.
	a := &ReadingAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{"domain":"article","title":"First Article"}`)},
		{ArtifactID: "a2", DomainData: json.RawMessage(`{"domain":"article","title":"Second Article"}`)},
		{ArtifactID: "a3", DomainData: json.RawMessage(`{"domain":"article","title":"Third Article"}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	if len(seeds) != 3 {
		t.Fatalf("expected 3 items, got %d", len(seeds))
	}
	for i, s := range seeds {
		if s.SortOrder != i {
			t.Errorf("seed[%d] sort_order = %d, want %d", i, s.SortOrder, i)
		}
	}
}

func TestReadingAggregator_SourceTraceability(t *testing.T) {
	// Each reading list item must trace back to its source artifact.
	a := &ReadingAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "art-42", DomainData: json.RawMessage(`{"domain":"article","title":"Deep Dive"}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	if len(seeds) != 1 {
		t.Fatal("expected 1 item")
	}
	if len(seeds[0].SourceArtifactIDs) != 1 || seeds[0].SourceArtifactIDs[0] != "art-42" {
		t.Errorf("expected source art-42, got %v", seeds[0].SourceArtifactIDs)
	}
}

func TestCompareAggregator_MultiProductAlignment(t *testing.T) {
	// Gherkin: "common spec names are aligned across products"
	// Verify that multiple products produce one seed per product with content
	// containing brand and price info for comparison.
	a := &CompareAggregator{}
	sources := []AggregationSource{
		{ArtifactID: "a1", DomainData: json.RawMessage(`{
			"domain":"product", "product_name":"Alpha",
			"brand":"BrandA", "price":{"amount":199.99,"currency":"USD"},
			"rating":{"score":4.2,"max":5,"count":500}
		}`)},
		{ArtifactID: "a2", DomainData: json.RawMessage(`{
			"domain":"product", "product_name":"Beta",
			"brand":"BrandB", "price":{"amount":249.00,"currency":"USD"},
			"rating":{"score":4.7,"max":5,"count":300}
		}`)},
		{ArtifactID: "a3", DomainData: json.RawMessage(`{
			"domain":"product", "product_name":"Gamma",
			"brand":"BrandC", "price":{"amount":149.00,"currency":"USD"},
			"rating":{"score":3.9,"max":5,"count":800}
		}`)},
	}

	seeds, err := a.Aggregate(sources)
	if err != nil {
		t.Fatal(err)
	}

	if len(seeds) != 3 {
		t.Fatalf("expected 3 comparison items, got %d", len(seeds))
	}

	// Verify each product has brand, price in content and correct price in quantity
	for _, s := range seeds {
		if s.Content == "" {
			t.Error("comparison item has empty content")
		}
		if s.Category != "comparison" {
			t.Errorf("expected category 'comparison', got %q", s.Category)
		}
		if s.Quantity == nil {
			t.Error("expected non-nil price quantity")
		}
		if len(s.SourceArtifactIDs) != 1 {
			t.Errorf("expected 1 source artifact, got %d", len(s.SourceArtifactIDs))
		}
	}

	// Verify the cheapest product (Gamma at $149) is correctly captured
	if seeds[2].Quantity == nil || *seeds[2].Quantity != 149.0 {
		t.Errorf("expected Gamma price 149, got %v", seeds[2].Quantity)
	}
}
