// Package routing implements spec 095's intent-aware retrieval-strategy
// routing over the SINGLE existing pgvector + knowledge-graph + structured
// store (Principle 5 — One Graph, Many Views). It introduces NO new index,
// database, or graph: the router is a pure decision function and the
// strategies are thin read-path overlays that consume the existing store
// through injected interfaces. The architecture tests (architecture_test.go)
// mechanically enforce the no-parallel-store invariant.
//
// SCOPE-02 (this file) — the RetrievalContract registry (Idea 3): each
// artifact type declares the admissible query shapes it must satisfy. The
// registry is an in-code typed read model seeded from the validated SST
// config (internal/config/retrieval.go). Unknown types resolve safely to
// [vague_recall] (R9 fail-safe), and that resolution is observable (R9 /
// Principle 8) via RetrievalContract.Known.
//
// References:
//   - specs/095-retrieval-strategy-routing/spec.md §5.1, R6–R9
//   - specs/095-retrieval-strategy-routing/design.md §4
//   - specs/095-retrieval-strategy-routing/scopes.md SCOPE-02
package routing

import (
	"fmt"
	"sort"
	"strings"

	"github.com/smackerel/smackerel/internal/config"
)

// QueryShape is the closed vocabulary of query shapes an artifact type can
// declare it must satisfy (Idea 3, R7/R8). The canonical strings are owned by
// internal/config (where SST validation enforces the closed set); routing
// re-exports them as a typed value so the SST and the in-code registry can
// never drift.
type QueryShape string

const (
	ShapeWholeDocumentSummary QueryShape = config.QueryShapeWholeDocumentSummary
	ShapeAggregateSpend       QueryShape = config.QueryShapeAggregateSpend
	ShapeDossier              QueryShape = config.QueryShapeDossier
	ShapeVagueRecall          QueryShape = config.QueryShapeVagueRecall
)

// allQueryShapes is the closed vocabulary used by the registry's defensive
// validation (config already enforced this at startup; the registry re-checks
// so a programmatic caller cannot smuggle an unknown shape in).
var allQueryShapes = map[QueryShape]struct{}{
	ShapeWholeDocumentSummary: {},
	ShapeAggregateSpend:       {},
	ShapeDossier:              {},
	ShapeVagueRecall:          {},
}

// IsValidQueryShape reports whether s is in the closed vocabulary.
func IsValidQueryShape(s QueryShape) bool {
	_, ok := allQueryShapes[s]
	return ok
}

// RetrievalContract is a per-artifact-type declaration of the admissible query
// shapes that type must satisfy (R7/R8). It is read-only at query time. Every
// contract is guaranteed to admit ShapeVagueRecall — the router's safe
// fallback must always be admissible (the registry appends it if SST omitted
// it).
type RetrievalContract struct {
	// ArtifactType is the lowercase artifact type this contract governs.
	ArtifactType string
	// Shapes is the ordered list of admissible query shapes. Always contains
	// ShapeVagueRecall.
	Shapes []QueryShape
	// Known is false when this contract was synthesized by the fail-safe
	// fallback (R9) — i.e. the queried type had no registered contract. A
	// caller can observe Known=false to record the missing-contract condition
	// (Principle 8). Known=true contracts come from declared SST.
	Known bool
}

// Admits reports whether the contract admits the given query shape.
func (c RetrievalContract) Admits(s QueryShape) bool {
	for _, have := range c.Shapes {
		if have == s {
			return true
		}
	}
	return false
}

// ContractRegistry is the typed in-code read model seeded from validated SST.
// Lookups are case-insensitive on artifact type; an unknown type resolves
// safely to a [vague_recall] contract with Known=false (R9).
type ContractRegistry struct {
	byType map[string]RetrievalContract
}

// NewContractRegistry builds the registry from the validated SST routing
// config. It returns an error only on a defensive closed-vocabulary violation
// (config.LoadRetrieval already rejects unknown shapes at startup, so this is
// a belt-and-braces guard for programmatic callers). Every declared contract
// is normalized to admit ShapeVagueRecall last if SST omitted it, so the
// router's safe fallback is always admissible.
func NewContractRegistry(cfg config.RetrievalRoutingConfig) (*ContractRegistry, error) {
	byType := make(map[string]RetrievalContract, len(cfg.Contracts))
	// Deterministic iteration for stable error messages.
	types := make([]string, 0, len(cfg.Contracts))
	for t := range cfg.Contracts {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, rawType := range types {
		t := strings.ToLower(strings.TrimSpace(rawType))
		if t == "" {
			return nil, fmt.Errorf("routing: empty artifact-type key in contracts")
		}
		rawShapes := cfg.Contracts[rawType]
		shapes := make([]QueryShape, 0, len(rawShapes)+1)
		seen := make(map[QueryShape]struct{}, len(rawShapes)+1)
		for _, rs := range rawShapes {
			s := QueryShape(rs)
			if !IsValidQueryShape(s) {
				return nil, fmt.Errorf("routing: artifact type %q declares unknown query shape %q", t, rs)
			}
			if _, dup := seen[s]; dup {
				continue
			}
			seen[s] = struct{}{}
			shapes = append(shapes, s)
		}
		// Invariant: the router's safe fallback must always be admissible.
		if _, ok := seen[ShapeVagueRecall]; !ok {
			shapes = append(shapes, ShapeVagueRecall)
		}
		byType[t] = RetrievalContract{ArtifactType: t, Shapes: shapes, Known: true}
	}
	return &ContractRegistry{byType: byType}, nil
}

// ContractFor returns the contract for the given artifact type. The lookup is
// case-insensitive. An unknown or empty type resolves to a fail-safe
// [vague_recall] contract with Known=false (R9) — it never errors the query
// and the missing-contract condition is observable (Principle 8).
func (r *ContractRegistry) ContractFor(artifactType string) RetrievalContract {
	t := strings.ToLower(strings.TrimSpace(artifactType))
	if c, ok := r.byType[t]; ok {
		return c
	}
	return RetrievalContract{
		ArtifactType: t,
		Shapes:       []QueryShape{ShapeVagueRecall},
		Known:        false,
	}
}

// DeclaredTypes returns the sorted set of artifact types with a declared
// (Known=true) contract. Used by observability and tests.
func (r *ContractRegistry) DeclaredTypes() []string {
	out := make([]string, 0, len(r.byType))
	for t := range r.byType {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
