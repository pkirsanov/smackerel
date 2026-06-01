// Package microtools is the spec 065 capability foundation for the
// generic micro-tools (location_normalize, unit_convert, calculator,
// entity_resolve). It defines the shared output envelope every
// micro-tool emits and the validation contract every handler MUST
// satisfy before the executor persists a tool-call row.
//
// SCOPE-1 ships the envelope types, status vocabulary, and
// ValidateEnvelopeBytes / ValidateEnvelope helpers. SCOPE-2..4 layer
// the concrete tools on top and register them through the existing
// spec 037 agent.RegisterTool path; no second registry is introduced.
//
// Design source: specs/065-generic-micro-tools/design.md §"Reusable
// Foundation" + §"Concrete Implementations".
package microtools

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// CurrentSchemaVersion is the envelope schema version every micro-tool
// emits in v1. Bumping this is a coordinated change across every
// concrete tool and every consumer (executor, trace renderer,
// scenario YAMLs that assert envelope fields).
const CurrentSchemaVersion = "v1"

// Status is the envelope status discriminator.
//
//	resolved  — value is populated and source-attributed
//	ambiguous — candidates is populated and the facade must clarify
//	failed    — error is populated and no other field is authoritative
type Status string

const (
	StatusResolved  Status = "resolved"
	StatusAmbiguous Status = "ambiguous"
	StatusFailed    Status = "failed"
)

// Valid reports whether s is one of the three known statuses.
func (s Status) Valid() bool {
	switch s {
	case StatusResolved, StatusAmbiguous, StatusFailed:
		return true
	default:
		return false
	}
}

// SourceKind classifies where the data ultimately came from. Concrete
// tools choose the value that matches their provider semantics.
type SourceKind string

const (
	SourceKindHTTPProvider SourceKind = "http_provider"
	SourceKindLocalCompute SourceKind = "local_compute"
	SourceKindGraphRead    SourceKind = "graph_read"
)

// Valid reports whether k is one of the recognized source kinds.
func (k SourceKind) Valid() bool {
	switch k {
	case SourceKindHTTPProvider, SourceKindLocalCompute, SourceKindGraphRead:
		return true
	default:
		return false
	}
}

// Source identifies the provider/origin and attribution metadata for a
// micro-tool result. Every envelope MUST carry a non-zero Source — the
// trace renderer surfaces these fields verbatim so users see who
// produced the answer.
type Source struct {
	// Provider is a short identifier (e.g. "open-meteo",
	// "calculator", "graph"). Required.
	Provider string `json:"provider"`
	// Kind classifies the origin (http_provider | local_compute |
	// graph_read). Required and must satisfy SourceKind.Valid.
	Kind SourceKind `json:"kind"`
	// RetrievedAt is the wall-clock moment the source produced the
	// result. Required and must be non-zero.
	RetrievedAt time.Time `json:"retrieved_at"`
	// Attribution is a human-readable attribution string the trace
	// renderer can display ("Data: Open-Meteo"). Required non-empty.
	Attribution string `json:"attribution"`
}

// Candidate is a single ranked alternative returned in the ambiguous
// envelope. Concrete tools populate Value with their tool-specific
// canonical structure (e.g. a canonical location).
type Candidate struct {
	// Rank is 1-based; lower is better. Required >= 1.
	Rank int `json:"rank"`
	// Label is a short human-readable label for the candidate.
	// Required non-empty.
	Label string `json:"label"`
	// Value is the tool-specific canonical value for the candidate.
	// Required non-nil.
	Value map[string]any `json:"value"`
	// Confidence is the tool's self-reported confidence in [0, 1].
	Confidence float64 `json:"confidence"`
	// Distinguishing is a short string the facade can use to help
	// the user pick between candidates ("Springfield, IL vs MO").
	Distinguishing string `json:"distinguishing,omitempty"`
}

// Error is the envelope error payload for status=failed.
type Error struct {
	// Code is a stable machine-readable error code (e.g.
	// "provider_unreachable", "ambiguous_substance"). Required.
	Code string `json:"code"`
	// Message is a short human-readable explanation. Required.
	Message string `json:"message"`
}

// Envelope is the shared output envelope every micro-tool emits. The
// executor persists the marshalled envelope in agent_tool_calls; the
// trace renderer reads the typed shape directly.
type Envelope struct {
	SchemaVersion string         `json:"schema_version"`
	Status        Status         `json:"status"`
	Value         map[string]any `json:"value,omitempty"`
	Candidates    []Candidate    `json:"candidates,omitempty"`
	Confidence    float64        `json:"confidence,omitempty"`
	Source        Source         `json:"source"`
	Error         *Error         `json:"error,omitempty"`
}

// ValidateEnvelope enforces the foundation invariants documented in
// design.md §"Foundation policies":
//
//   - schema_version equals CurrentSchemaVersion
//   - status is one of the three known values
//   - source is non-zero and well-formed (every required sub-field set)
//   - resolved → value is non-empty
//   - ambiguous → candidates is non-empty, each entry well-formed
//   - failed → error is non-nil with non-empty code+message
//   - confidence (when set) is in [0, 1]
//
// Tool handlers MUST call ValidateEnvelope (or
// ValidateEnvelopeBytes) on every output before returning to the
// executor; the executor and trace persistence path also run
// ValidateEnvelopeBytes as a defense-in-depth check.
func ValidateEnvelope(env Envelope) error {
	if env.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("microtools: schema_version %q != %q", env.SchemaVersion, CurrentSchemaVersion)
	}
	if !env.Status.Valid() {
		return fmt.Errorf("microtools: invalid status %q (must be resolved|ambiguous|failed)", env.Status)
	}
	if err := validateSource(env.Source); err != nil {
		return err
	}
	if env.Confidence < 0 || env.Confidence > 1 {
		return fmt.Errorf("microtools: confidence %g out of range [0,1]", env.Confidence)
	}
	switch env.Status {
	case StatusResolved:
		if len(env.Value) == 0 {
			return errors.New("microtools: status=resolved requires non-empty value")
		}
		if env.Error != nil {
			return errors.New("microtools: status=resolved must not carry error")
		}
	case StatusAmbiguous:
		if len(env.Candidates) == 0 {
			return errors.New("microtools: status=ambiguous requires at least one candidate")
		}
		if env.Error != nil {
			return errors.New("microtools: status=ambiguous must not carry error")
		}
		for i, c := range env.Candidates {
			if err := validateCandidate(i, c); err != nil {
				return err
			}
		}
	case StatusFailed:
		if env.Error == nil {
			return errors.New("microtools: status=failed requires error payload")
		}
		if env.Error.Code == "" || env.Error.Message == "" {
			return errors.New("microtools: status=failed requires non-empty error code and message")
		}
		if len(env.Value) != 0 {
			return errors.New("microtools: status=failed must not carry value")
		}
		if len(env.Candidates) != 0 {
			return errors.New("microtools: status=failed must not carry candidates")
		}
	}
	return nil
}

// ValidateEnvelopeBytes decodes raw JSON and runs ValidateEnvelope.
// The executor uses this on the handler's raw return bytes to enforce
// the foundation contract independent of how the handler constructed
// the value.
func ValidateEnvelopeBytes(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("microtools: empty envelope bytes")
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("microtools: envelope decode: %w", err)
	}
	return ValidateEnvelope(env)
}

func validateSource(s Source) error {
	if s.Provider == "" {
		return errors.New("microtools: source.provider is required")
	}
	if !s.Kind.Valid() {
		return fmt.Errorf("microtools: source.kind %q invalid (must be http_provider|local_compute|graph_read)", s.Kind)
	}
	if s.RetrievedAt.IsZero() {
		return errors.New("microtools: source.retrieved_at is required")
	}
	if s.Attribution == "" {
		return errors.New("microtools: source.attribution is required")
	}
	return nil
}

func validateCandidate(i int, c Candidate) error {
	if c.Rank < 1 {
		return fmt.Errorf("microtools: candidates[%d].rank must be >= 1, got %d", i, c.Rank)
	}
	if c.Label == "" {
		return fmt.Errorf("microtools: candidates[%d].label is required", i)
	}
	if len(c.Value) == 0 {
		return fmt.Errorf("microtools: candidates[%d].value is required", i)
	}
	if c.Confidence < 0 || c.Confidence > 1 {
		return fmt.Errorf("microtools: candidates[%d].confidence %g out of range [0,1]", i, c.Confidence)
	}
	return nil
}
