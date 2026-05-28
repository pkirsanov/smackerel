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
)

// AllSourceKinds is the exhaustive closed-vocabulary list.
var AllSourceKinds = []SourceKind{
	SourceArtifact,
	SourceExternalProvider,
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
