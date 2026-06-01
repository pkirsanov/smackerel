package openknowledge

import (
	"context"
	"encoding/json"
	"time"
)

// SourceKind enumerates the provenance classes recognised by the
// extended spec 061 provenance gate. The numeric values are stable
// because they are persisted as artifact metadata.
type SourceKind int

const (
	// SourceArtifact identifies a citation backed by an artifact already
	// present in the knowledge graph (the original spec 061 contract).
	SourceArtifact SourceKind = iota + 1
	// SourceWeb identifies a citation backed by a web snippet returned
	// by the web_search tool's chosen provider.
	SourceWeb
	// SourceToolComputation identifies a citation backed by a
	// deterministic tool computation (calculator, unit_convert).
	SourceToolComputation
)

// String returns the canonical lowercase label used in logs and metrics.
func (k SourceKind) String() string {
	switch k {
	case SourceArtifact:
		return "artifact"
	case SourceWeb:
		return "web"
	case SourceToolComputation:
		return "tool_computation"
	default:
		return "unknown"
	}
}

// ArtifactRef points at an artifact in the knowledge graph.
type ArtifactRef struct {
	ID    string
	Kind  string
	Title string
}

// WebSource carries the metadata required by the spec 061 gate for any
// citation whose Kind is SourceWeb. All fields are mandatory; the gate
// rejects entries with empty URL, ContentHash, or Provider.
type WebSource struct {
	URL         string
	Title       string
	Provider    string
	FetchedAt   time.Time
	ContentHash string
	Snippet     string
}

// ComputationSource carries the metadata required by the gate for any
// citation whose Kind is SourceToolComputation. Input and Output are
// the canonicalized JSON payloads of the tool's invocation, suitable
// for hash-based verification.
type ComputationSource struct {
	Tool   string
	Input  json.RawMessage
	Output json.RawMessage
}

// Source is the unified citation envelope. Exactly one of Artifact,
// Web, or Computation MUST be populated according to Kind.
type Source struct {
	Kind        SourceKind
	Artifact    *ArtifactRef
	Web         *WebSource
	Computation *ComputationSource
}

// Snippet is the grounding fragment a tool returned to the planner.
// ContentHash is the sha256 of the canonicalised Text and is the key
// the cite-back verifier uses to confirm planner citations.
type Snippet struct {
	Text        string
	ContentHash string
	SourceRef   string
}

// Computation is the structured output of a deterministic tool. It is
// surfaced both to the planner (as Snippets) and to the source
// assembler (as a ComputationSource).
type Computation struct {
	Tool   string
	Input  json.RawMessage
	Output json.RawMessage
}

// ToolError is a structured soft error returned by a Tool. The agent
// loop surfaces ToolError to the planner so it may try another tool;
// a hard panic from Execute is treated as a refusal trigger by the
// loop instead.
type ToolError struct {
	Code    string
	Message string
}

// Error satisfies the error interface for logging and metrics.
func (e *ToolError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code + ": " + e.Message
}

// ToolResult is the canonical envelope every Tool returns from Execute.
type ToolResult struct {
	Snippets    []Snippet
	Sources     []Source
	Computation *Computation
	Error       *ToolError
}

// Tool is the registry-level capability contract. Implementations live
// under internal/assistant/openknowledge/tools/. Name() MUST be stable
// and unique within a Registry instance.
type Tool interface {
	Name() string
	Description() string
	ParamsSchema() json.RawMessage
	Execute(ctx context.Context, params json.RawMessage) (*ToolResult, error)
}
