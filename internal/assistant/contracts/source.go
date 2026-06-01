package contracts

import "time"

// Source is one provenance entry attached to an AssistantResponse.
// The provenance gate (internal/assistant/provenance) refuses any
// non-empty Body that lacks at least one Source when the scenario
// has requires_provenance: true (BS-007).
//
// Source of truth: design.md §2.2.
type Source struct {
	// ID is the stable canonical identifier (artifact_id for
	// SourceArtifact; provider-defined opaque id for
	// SourceExternalProvider).
	ID string

	// Title is the rendered human-readable label.
	Title string

	// Kind discriminates the SourceRef shape.
	Kind SourceKind

	// Ref is the kind-discriminated payload. Implementations:
	// ArtifactRef when Kind == SourceArtifact; ExternalProviderRef
	// when Kind == SourceExternalProvider.
	Ref SourceRef
}

// SourceKind is the closed-vocabulary Source.Kind discriminator.
type SourceKind string

const (
	// SourceArtifact is a Source backed by an artifact row in the
	// knowledge graph.
	SourceArtifact SourceKind = "artifact"
	// SourceExternalProvider is a Source backed by an external
	// provider response (weather forecast, gov-alerts feed, etc.)
	// that is NOT promoted into the knowledge graph.
	SourceExternalProvider SourceKind = "external_provider"
	// SourceWeb is a Source backed by a web search snippet returned
	// by the open-knowledge agent's web_search tool. Added via
	// PKT-061-A from spec 064 to extend the provenance gate's
	// accepted taxonomy to cover web-grounded citations.
	SourceWeb SourceKind = "web"
	// SourceToolComputation is a Source backed by a deterministic
	// tool computation (calculator, unit_convert). Added via
	// PKT-061-A from spec 064.
	SourceToolComputation SourceKind = "tool_computation"
)

// AllSourceKinds is the exhaustive closed-vocabulary list.
var AllSourceKinds = []SourceKind{
	SourceArtifact,
	SourceExternalProvider,
	SourceWeb,
	SourceToolComputation,
}

// SourceRef is the discriminated-union sealed interface for the
// kind-specific payload carried by Source.Ref. Sealed via the
// unexported isSourceRef() method so only the two structs defined
// in this file can satisfy it.
type SourceRef interface {
	isSourceRef()
}

// ArtifactRef is the SourceRef shape for Kind == SourceArtifact.
type ArtifactRef struct {
	ArtifactID string
	CapturedAt time.Time
}

func (ArtifactRef) isSourceRef() {}

// ExternalProviderRef is the SourceRef shape for
// Kind == SourceExternalProvider. RetrievedAt MUST be the ORIGINAL
// provider-side retrieval timestamp; on cache hits the weather skill
// (design.md §5.2) emits the ORIGINAL value, NOT the cache-hit time.
type ExternalProviderRef struct {
	ProviderName string
	RetrievedAt  time.Time
}

func (ExternalProviderRef) isSourceRef() {}

// WebSourceRef is the SourceRef shape for Kind == SourceWeb. All
// fields are mandatory; the source-assembler MUST populate every
// field. ContentHash is the sha256 of the canonicalised Snippet text
// and is the key the cite-back verifier (spec 064) uses to confirm
// the planner did not fabricate the citation.
type WebSourceRef struct {
	URL         string
	Provider    string
	FetchedAt   time.Time
	ContentHash string
	Snippet     string
}

func (WebSourceRef) isSourceRef() {}

// ComputationSourceRef is the SourceRef shape for
// Kind == SourceToolComputation. Tool is the registry name of the
// deterministic tool; InputHash and OutputHash are the sha256
// digests of the canonicalised tool input/output payloads, suitable
// for hash-based verification by the spec 064 cite-back verifier.
type ComputationSourceRef struct {
	Tool       string
	InputHash  string
	OutputHash string
}

func (ComputationSourceRef) isSourceRef() {}
