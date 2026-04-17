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
