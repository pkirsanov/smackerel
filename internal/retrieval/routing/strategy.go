// Spec 095 SCOPE-03 — RetrievalStrategy interface + StrategyKind closed
// vocabulary + the dependency interfaces the concrete strategy overlays
// (SCOPE-04/05/06) consume.
//
// CRITICAL (Principle 5): every dependency below is an INJECTED INTERFACE.
// internal/retrieval never constructs a backing store — the concrete adapters
// that wrap the EXISTING pgvector / knowledge-graph / structured tables are
// wired in cmd/core (outside this package). The architecture tests
// (architecture_test.go) mechanically prove no package under
// internal/retrieval opens a second store, index, or graph.
package routing

import "context"

// StrategyKind is the closed vocabulary of concrete retrieval read-paths
// (Idea 1). Each is one read path over the SAME store, not a new store.
type StrategyKind string

const (
	// StrategyWholeDocument fetches the full preserved artifact (Idea 1a).
	StrategyWholeDocument StrategyKind = "whole_document"
	// StrategyStructuredAggregate runs a structured aggregate over the
	// existing expenses/subscriptions tables (Idea 1b).
	StrategyStructuredAggregate StrategyKind = "structured_aggregate"
	// StrategyVagueRecall is today's §9.2 vector→graph→rerank path, the
	// default and safe fallback (Idea 1c).
	StrategyVagueRecall StrategyKind = "vague_recall"
)

// allStrategyKinds is the closed vocabulary used by validation/tests.
var allStrategyKinds = map[StrategyKind]struct{}{
	StrategyWholeDocument:       {},
	StrategyStructuredAggregate: {},
	StrategyVagueRecall:         {},
}

// IsValidStrategyKind reports whether k is in the closed vocabulary.
func IsValidStrategyKind(k StrategyKind) bool {
	_, ok := allStrategyKinds[k]
	return ok
}

// strategyForShape maps a desired query shape to the StrategyKind that serves
// it. A shape with no specialized strategy in v1 (e.g. dossier) maps to
// vague_recall (R6 — a type with no admissible specialized strategy resolves
// to vague_recall). ok is false when the shape itself is unknown.
func strategyForShape(s QueryShape) (kind StrategyKind, specialized bool) {
	switch s {
	case ShapeWholeDocumentSummary:
		return StrategyWholeDocument, true
	case ShapeAggregateSpend:
		return StrategyStructuredAggregate, true
	case ShapeDossier:
		// v1 ships no dossier overlay; dossier-shaped queries fall back to
		// vague_recall (design §2 — dossier is a future kind).
		return StrategyVagueRecall, false
	case ShapeVagueRecall:
		return StrategyVagueRecall, false
	default:
		return StrategyVagueRecall, false
	}
}

// RetrievalRequest is the input to a strategy execution. It carries the
// already-computed routing Selection (so the strategy never re-routes) plus
// the resolved target identifiers the strategy needs.
type RetrievalRequest struct {
	Query        string
	ArtifactType string
	// ArtifactID is the resolved target artifact for the whole_document
	// strategy (the full-context recall target).
	ArtifactID string
	// Aggregate carries the structured-aggregate query parameters derived
	// from the intent slots (period/category/extremum) for the
	// structured_aggregate strategy.
	Aggregate AggregateQuery
	// Selection is the router's traced decision for this request.
	Selection StrategySelection
}

// RetrievedSourceKind is the closed provenance vocabulary recorded on each
// retrieved source (Principle 8). It mirrors the existing
// knowledge.AgentAnswerSourceKind contract (artifact citation) extended with
// the structured-aggregate provenance.
type RetrievedSourceKind string

const (
	// SourceFullArtifact cites the COMPLETE preserved artifact (whole_document).
	SourceFullArtifact RetrievedSourceKind = "artifact"
	// SourceStructuredAggregate cites the existing structured table the
	// aggregate was computed from (structured_aggregate).
	SourceStructuredAggregate RetrievedSourceKind = "structured_aggregate"
	// SourceVagueRecallSet cites the vector+graph+rerank candidate set
	// (vague_recall).
	SourceVagueRecallSet RetrievedSourceKind = "vague_recall_set"
)

// RetrievedSource is one cite-back provenance row for a retrieval result.
type RetrievedSource struct {
	Kind       RetrievedSourceKind
	ArtifactID string
	// Detail is a human-readable provenance string (e.g. the structured
	// table name + computed figure, or the full-artifact title).
	Detail string
}

// RetrievalResult is the output of a strategy execution.
type RetrievalResult struct {
	Strategy StrategyKind
	Sources  []RetrievedSource
	// FullArtifact is true when the whole preserved artifact was fetched
	// (not a top-k chunk subset) — proves R2 / SCN-095-A02.
	FullArtifact bool
	// Answer is the strategy's synthesized/structured answer text.
	Answer string
}

// RetrievalStrategy is a concrete retrieval read-path over the EXISTING store.
// Implementations live in internal/retrieval/routing/strategies/* and consume
// only injected dependency interfaces — they MUST NOT open a store.
type RetrievalStrategy interface {
	Kind() StrategyKind
	Execute(ctx context.Context, req RetrievalRequest) (RetrievalResult, error)
}

// --- Injected dependency interfaces (Principle 5 — existing store only) ---

// FullArtifact is the COMPLETE preserved artifact (all chunks), NOT a top-k
// subset. The whole_document strategy synthesizes from this complete context.
type FullArtifact struct {
	ID        string
	Type      string
	Title     string
	Content   string
	NumChunks int
}

// ArtifactFetcher fetches the full preserved artifact by id from the EXISTING
// store. Wired in cmd/core over the existing artifact store; injected so
// internal/retrieval never opens a store (Principle 5).
type ArtifactFetcher interface {
	FetchFullArtifact(ctx context.Context, artifactID string) (FullArtifact, error)
}

// AggregateExtremum is the closed vocabulary of superlative directions.
type AggregateExtremum string

const (
	ExtremumMax AggregateExtremum = "max"
	ExtremumMin AggregateExtremum = "min"
)

// AggregateQuery parameterizes a structured aggregate over the existing
// expenses/subscriptions tables.
type AggregateQuery struct {
	Category string // e.g. "subscriptions", "expenses"
	Period   string // e.g. "month"
	Extremum AggregateExtremum
	// Financial is true when the aggregate is over financial-markets/QF
	// artifacts; the strategy returns descriptive recall only (Principle 10).
	Financial bool
}

// AggregateResult is the exact computed figure with structured-table
// provenance.
type AggregateResult struct {
	// Bucket is the winning period label (e.g. "2026-03").
	Bucket string
	// Amount is the aggregated figure for the winning bucket.
	Amount float64
	// Table is the existing structured table the figure was computed from.
	Table string
}

// SpendAggregator runs a structured aggregate over the EXISTING
// expenses/subscriptions tables (Idea 1b). Wired in cmd/core as a thin adapter
// over internal/intelligence — NOT a new analytics DB (Principle 5).
type SpendAggregator interface {
	SuperlativeSpend(ctx context.Context, q AggregateQuery) (AggregateResult, error)
}

// VagueRecallExecutor runs today's §9.2 vector → graph-expand → LLM-rerank
// pipeline, byte-for-byte unchanged (Idea 1c, NFR-3). Wired in cmd/core as a
// thin adapter over the existing retrieval pipeline.
type VagueRecallExecutor interface {
	VagueRecall(ctx context.Context, query string) (RetrievalResult, error)
}
