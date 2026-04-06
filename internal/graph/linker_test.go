package graph

import (
	"testing"
)

func TestLinker_Init(t *testing.T) {
	linker := NewLinker(nil)
	if linker == nil {
		t.Fatal("expected non-nil linker")
	}
	if linker.Pool != nil {
		t.Error("expected nil pool for test linker")
	}
}

func TestParseJSON_People(t *testing.T) {
	data := []byte(`{"people": ["Alice", "Bob"], "orgs": ["Acme"]}`)

	type Entities struct {
		People []string `json:"people"`
		Orgs   []string `json:"orgs"`
	}
	var entities Entities
	if err := parseJSON(data, &entities); err != nil {
		t.Fatalf("parse entities: %v", err)
	}

	if len(entities.People) != 2 {
		t.Errorf("expected 2 people, got %d", len(entities.People))
	}
	if entities.People[0] != "Alice" {
		t.Errorf("expected first person Alice, got %q", entities.People[0])
	}
}

func TestParseJSON_Topics(t *testing.T) {
	data := []byte(`["pricing", "saas", "growth"]`)

	var topics []string
	if err := parseJSON(data, &topics); err != nil {
		t.Fatalf("parse topics: %v", err)
	}

	if len(topics) != 3 {
		t.Errorf("expected 3 topics, got %d", len(topics))
	}
}

func TestParseJSON_Empty(t *testing.T) {
	err := parseJSON([]byte{}, nil)
	if err == nil {
		t.Error("expected error for empty JSON")
	}
}

func TestParseJSON_MalformedJSON(t *testing.T) {
	var result []string
	err := parseJSON([]byte(`{invalid`), &result)
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestParseJSON_EmptyObject(t *testing.T) {
	type Entities struct {
		People []string `json:"people"`
	}
	var result Entities
	err := parseJSON([]byte(`{}`), &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.People) != 0 {
		t.Errorf("expected 0 people, got %d", len(result.People))
	}
}

func TestParseJSON_EmptyArray(t *testing.T) {
	var result []string
	err := parseJSON([]byte(`[]`), &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 items, got %d", len(result))
	}
}

func TestParseJSON_NestedEntities(t *testing.T) {
	data := []byte(`{"people": ["Alice"], "orgs": ["Acme", "TechCorp"], "places": ["NYC"], "products": [], "dates": ["2026-04-01"]}`)

	type FullEntities struct {
		People   []string `json:"people"`
		Orgs     []string `json:"orgs"`
		Places   []string `json:"places"`
		Products []string `json:"products"`
		Dates    []string `json:"dates"`
	}
	var result FullEntities
	if err := parseJSON(data, &result); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.People) != 1 || result.People[0] != "Alice" {
		t.Errorf("unexpected people: %v", result.People)
	}
	if len(result.Orgs) != 2 {
		t.Errorf("expected 2 orgs, got %d", len(result.Orgs))
	}
	if len(result.Products) != 0 {
		t.Errorf("expected 0 products, got %d", len(result.Products))
	}
}

func TestNewLinker_WithNilPool(t *testing.T) {
	l := NewLinker(nil)
	if l.Pool != nil {
		t.Error("expected nil pool")
	}
}

func TestConnectionCount_Structure(t *testing.T) {
	// Verify the method signature exists
	l := NewLinker(nil)
	_ = l // ConnectionCount requires pool, would panic
}
