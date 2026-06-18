// Spec 095 SCOPE-02 — RetrievalContract registry tests.
package routing

import (
	"testing"

	"github.com/smackerel/smackerel/internal/config"
)

// testRoutingConfig returns a representative validated routing config.
func testRoutingConfig() config.RetrievalRoutingConfig {
	return config.RetrievalRoutingConfig{
		Enabled:                    true,
		IntentConfidenceThreshold:  0.65,
		WholeDocumentEnabled:       true,
		StructuredAggregateEnabled: true,
		VagueRecallEnabled:         true,
		Contracts: map[string][]string{
			"transcript":   {"whole_document_summary", "vague_recall"},
			"subscription": {"aggregate_spend", "vague_recall"},
			"place":        {"dossier", "vague_recall"},
		},
	}
}

// TestContractForDeclaredTypes — SCN-095-C01: each declared type resolves to
// its admissible shapes, is Known, and admits vague_recall.
func TestContractForDeclaredTypes(t *testing.T) {
	reg, err := NewContractRegistry(testRoutingConfig())
	if err != nil {
		t.Fatalf("NewContractRegistry: %v", err)
	}
	cases := map[string]QueryShape{
		"transcript":   ShapeWholeDocumentSummary,
		"subscription": ShapeAggregateSpend,
		"place":        ShapeDossier,
	}
	for typ, wantShape := range cases {
		c := reg.ContractFor(typ)
		if !c.Known {
			t.Errorf("%s: contract should be Known", typ)
		}
		if !c.Admits(wantShape) {
			t.Errorf("%s: contract should admit %s, got %v", typ, wantShape, c.Shapes)
		}
		if !c.Admits(ShapeVagueRecall) {
			t.Errorf("%s: every contract must admit vague_recall (safe fallback), got %v", typ, c.Shapes)
		}
	}
}

// TestContractForDeclaredTypes_CaseInsensitive — the lookup is case-insensitive.
func TestContractForDeclaredTypes_CaseInsensitive(t *testing.T) {
	reg, err := NewContractRegistry(testRoutingConfig())
	if err != nil {
		t.Fatalf("NewContractRegistry: %v", err)
	}
	if c := reg.ContractFor("TRANSCRIPT"); !c.Known || !c.Admits(ShapeWholeDocumentSummary) {
		t.Errorf("case-insensitive lookup failed for TRANSCRIPT: %+v", c)
	}
}

// TestUnknownTypeFailsSafe — SCN-095-C03: an unknown type resolves to
// [vague_recall] with Known=false (observable missing-contract condition),
// never an error.
func TestUnknownTypeFailsSafe(t *testing.T) {
	reg, err := NewContractRegistry(testRoutingConfig())
	if err != nil {
		t.Fatalf("NewContractRegistry: %v", err)
	}
	for _, typ := range []string{"unknown_type", "", "   "} {
		c := reg.ContractFor(typ)
		if c.Known {
			t.Errorf("%q: unknown type should resolve with Known=false (observable)", typ)
		}
		if len(c.Shapes) != 1 || c.Shapes[0] != ShapeVagueRecall {
			t.Errorf("%q: unknown type should resolve to [vague_recall], got %v", typ, c.Shapes)
		}
		if !c.Admits(ShapeVagueRecall) {
			t.Errorf("%q: fail-safe contract must admit vague_recall", typ)
		}
	}
}

// TestNewContractRegistry_AppendsVagueRecall — when SST omits vague_recall the
// registry appends it so the safe fallback is always admissible.
func TestNewContractRegistry_AppendsVagueRecall(t *testing.T) {
	cfg := testRoutingConfig()
	cfg.Contracts = map[string][]string{"transcript": {"whole_document_summary"}}
	reg, err := NewContractRegistry(cfg)
	if err != nil {
		t.Fatalf("NewContractRegistry: %v", err)
	}
	c := reg.ContractFor("transcript")
	if !c.Admits(ShapeVagueRecall) {
		t.Errorf("registry should append vague_recall when SST omits it, got %v", c.Shapes)
	}
}

// TestNewContractRegistry_RejectsUnknownShape — the closed-vocabulary guard
// (defensive belt-and-braces tie to SCOPE-01): a programmatic config with an
// unknown shape is rejected.
func TestNewContractRegistry_RejectsUnknownShape(t *testing.T) {
	cfg := testRoutingConfig()
	cfg.Contracts = map[string][]string{"transcript": {"bogus_shape"}}
	if _, err := NewContractRegistry(cfg); err == nil {
		t.Fatal("NewContractRegistry should reject an unknown query shape")
	}
}

// TestNewContractRegistry_DeduplicatesShapes — duplicate shapes collapse.
func TestNewContractRegistry_DeduplicatesShapes(t *testing.T) {
	cfg := testRoutingConfig()
	cfg.Contracts = map[string][]string{"transcript": {"vague_recall", "vague_recall", "whole_document_summary"}}
	reg, err := NewContractRegistry(cfg)
	if err != nil {
		t.Fatalf("NewContractRegistry: %v", err)
	}
	c := reg.ContractFor("transcript")
	if len(c.Shapes) != 2 {
		t.Errorf("duplicate shapes should collapse, got %v", c.Shapes)
	}
}
