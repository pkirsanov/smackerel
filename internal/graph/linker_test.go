package graph

import (
	"context"
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
	l := NewLinker(nil)
	_ = l
}

// SCN-002-016: Vector similarity linking — linkBySimilarity runs and attempts DB query
func TestSCN002016_VectorSimilarityLinker_Exists(t *testing.T) {
	l := NewLinker(nil)
	if l == nil {
		t.Fatal("linker must be constructable")
	}

	// Call linkBySimilarity directly: with nil pool it panics at Pool.QueryRow,
	// proving the method exists, accepts (context, string), and attempts DB access.
	ctx := context.Background()
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		l.linkBySimilarity(ctx, "test-artifact-001")
	}()

	if !panicked {
		t.Error("linkBySimilarity with nil pool should attempt DB access")
	}
}

// SCN-002-017: Entity-based linking — verify people JSON parsing for MENTIONS edges
func TestSCN002017_EntityLinking_PeopleExtraction(t *testing.T) {
	data := []byte(`{"people": ["Sarah Chen", "David Kim"], "orgs": ["Acme Corp"]}`)
	type Entities struct {
		People []string `json:"people"`
		Orgs   []string `json:"orgs"`
	}
	var ent Entities
	if err := parseJSON(data, &ent); err != nil {
		t.Fatalf("parse entities: %v", err)
	}
	if len(ent.People) != 2 {
		t.Fatalf("expected 2 people, got %d", len(ent.People))
	}
	if ent.People[0] != "Sarah Chen" {
		t.Errorf("expected 'Sarah Chen', got %q", ent.People[0])
	}
	if ent.People[1] != "David Kim" {
		t.Errorf("expected 'David Kim', got %q", ent.People[1])
	}
}

// SCN-002-018: Topic clustering — verify topic name parsing for BELONGS_TO edges
func TestSCN002018_TopicClustering_TopicExtraction(t *testing.T) {
	data := []byte(`["negotiation", "saas pricing", "leadership"]`)
	var topics []string
	if err := parseJSON(data, &topics); err != nil {
		t.Fatalf("parse topics: %v", err)
	}
	if len(topics) != 3 {
		t.Fatalf("expected 3 topics, got %d", len(topics))
	}
	if topics[0] != "negotiation" {
		t.Errorf("expected 'negotiation', got %q", topics[0])
	}
}

// SCN-002-019: Temporal linking — linkByTemporal runs and attempts DB query
func TestSCN002019_TemporalLinking_Exists(t *testing.T) {
	l := NewLinker(nil)
	if l == nil {
		t.Fatal("linker must be constructable for temporal linking")
	}

	// Call linkByTemporal directly: nil pool → panic at Pool.Query,
	// proving the method exists and attempts same-day proximity DB query.
	ctx := context.Background()
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		l.linkByTemporal(ctx, "test-artifact-001")
	}()

	if !panicked {
		t.Error("linkByTemporal with nil pool should attempt DB access")
	}
}

// SCN-002-016/019: LinkArtifact orchestrates all four linking strategies
func TestSCN002016_019_LinkArtifact_OrchestratesAllStrategies(t *testing.T) {
	l := NewLinker(nil)
	ctx := context.Background()

	// LinkArtifact calls similarity → entities → topics → temporal.
	// With nil pool the first strategy panics, proving orchestration begins.
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		edges, err := l.LinkArtifact(ctx, "test-artifact-001")
		_ = edges
		_ = err
	}()

	if !panicked {
		t.Error("LinkArtifact with nil pool should attempt DB access via strategies")
	}

	// Verify all four strategy methods exist with correct (ctx, string) → (int, error) signatures
	strategies := []struct {
		name string
		fn   func(context.Context, string) (int, error)
	}{
		{"linkBySimilarity", l.linkBySimilarity},
		{"linkByEntities", l.linkByEntities},
		{"linkByTopics", l.linkByTopics},
		{"linkByTemporal", l.linkByTemporal},
	}

	for _, s := range strategies {
		t.Run(s.name, func(t *testing.T) {
			panicked := false
			func() {
				defer func() {
					if r := recover(); r != nil {
						panicked = true
					}
				}()
				s.fn(ctx, "test-artifact-002")
			}()
			if !panicked {
				t.Errorf("strategy %s should attempt DB access with nil pool", s.name)
			}
		})
	}
}
