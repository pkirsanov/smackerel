// Spec 065 SCOPE-4 — entity_resolve micro-tool.
//
// entity_resolve maps a free-text colloquial reference ("the lease",
// "mom's birthday card") to a ranked list of user-scoped artifact
// references using the knowledge graph + vector search substrate.
//
// Semantics:
//
//   - resolved  → top candidate's confidence >= ConfidenceFloor, the
//                 envelope value carries the top artifact_id and
//                 candidates carries the bounded ranked list.
//   - ambiguous → at least one candidate exists but the top score is
//                 below the floor; the agent loop must surface a
//                 spec 061 clarification turn rather than guess.
//   - failed    → resolver returned zero candidates or an error;
//                 the trace records the error code.
//
// User-scope isolation: the input REQUIRES a non-empty `user_id` and
// the Resolver implementation is responsible for restricting reads to
// that user's artifacts. The handler refuses requests without
// user_id; cross-user reads are a Resolver-side concern enforced by
// integration tests against the live store.
//
// Wiring contract: production code in cmd/core constructs a
// *EntityResolveServices and calls SetEntityResolveServices once at
// startup. Until then the handler returns a structured
// "entity_resolve_not_configured" error so the trace shows the
// missing wiring instead of crashing the binary.

package microtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
)

// EntityResolveToolName is the canonical tool name registered through
// the spec 037 agent registry.
const EntityResolveToolName = "entity_resolve"

// EntityCandidate is a single ranked candidate returned by a
// Resolver. Score MUST be in [0, 1]; the handler enforces that.
type EntityCandidate struct {
	// ArtifactID is the canonical, user-scoped artifact identifier.
	// Required non-empty.
	ArtifactID string
	// Label is a short human-readable label the agent or the
	// clarification UI surfaces ("Apartment lease — signed 2024-06").
	// Required non-empty.
	Label string
	// Score is the resolver's confidence in [0, 1]. The handler
	// uses Score for the candidate ranking and for the top-vs-floor
	// resolved/ambiguous decision.
	Score float64
	// Snippet is an optional short excerpt the trace and the
	// clarification UI can render to help the user pick.
	Snippet string
	// CapturedAt is an optional ISO-8601 capture timestamp.
	CapturedAt string
	// ArtifactType is an optional type tag ("document",
	// "recipe", ...). Surfaced in the envelope value/candidate
	// payload for trace/disambiguation rendering.
	ArtifactType string
}

// EntityResolver is the substrate the entity_resolve micro-tool
// reads from. It is satisfied by a graph/search adapter wired in
// cmd/core (production) and by a fake in unit tests.
//
// The Resolver is responsible for:
//
//   - scoping results to the supplied userID (cross-user artifacts
//     MUST NOT leak),
//   - applying any per-scope filters ("documents", "recipes", ...),
//   - returning at most maxCandidates results sorted by score (best
//     first) — the handler trusts the order it receives.
type EntityResolver interface {
	Resolve(ctx context.Context, userID, input, scope string, maxCandidates int) ([]EntityCandidate, error)
}

// EntityResolveServices is the runtime wiring for the entity_resolve
// handler.
type EntityResolveServices struct {
	// Resolver is the substrate that returns ranked candidates.
	Resolver EntityResolver
	// ConfidenceFloor in [0, 1] is the resolved/ambiguous threshold.
	// Mirrors the SST key
	// ASSISTANT_TOOLS_ENTITY_RESOLVE_CONFIDENCE_FLOOR.
	ConfidenceFloor float64
	// MaxCandidates caps the candidate list size. Required >= 1.
	MaxCandidates int
	// Timeout is the per-call deadline applied to the Resolver call.
	// Required > 0.
	Timeout time.Duration
}

var (
	entityResolveMu           sync.RWMutex
	entityResolveSvc          *EntityResolveServices
	entityResolveRegisterOnce sync.Once
)

// SetEntityResolveServices wires the production entity_resolve
// runtime and registers the tool with the spec 037 agent registry on
// first call. Pass nil to clear (test-only).
func SetEntityResolveServices(s *EntityResolveServices) {
	entityResolveRegisterOnce.Do(registerEntityResolve)
	entityResolveMu.Lock()
	defer entityResolveMu.Unlock()
	entityResolveSvc = s
}

// ResetEntityResolveServicesForTest clears the wired services.
// Test-only.
func ResetEntityResolveServicesForTest() {
	entityResolveMu.Lock()
	defer entityResolveMu.Unlock()
	entityResolveSvc = nil
}

func loadEntityResolveServices() (*EntityResolveServices, error) {
	entityResolveMu.RLock()
	defer entityResolveMu.RUnlock()
	if entityResolveSvc == nil {
		return nil, errors.New("entity_resolve_not_configured")
	}
	if entityResolveSvc.Resolver == nil {
		return nil, errors.New("entity_resolve_resolver_not_configured")
	}
	if entityResolveSvc.ConfidenceFloor < 0 || entityResolveSvc.ConfidenceFloor > 1 {
		return nil, fmt.Errorf("entity_resolve_confidence_floor_invalid: %g", entityResolveSvc.ConfidenceFloor)
	}
	if entityResolveSvc.MaxCandidates < 1 {
		return nil, fmt.Errorf("entity_resolve_max_candidates_invalid: %d", entityResolveSvc.MaxCandidates)
	}
	if entityResolveSvc.Timeout <= 0 {
		return nil, fmt.Errorf("entity_resolve_timeout_invalid: %s", entityResolveSvc.Timeout)
	}
	return entityResolveSvc, nil
}

// -------------------- schemas --------------------

var entityResolveInputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["input", "user_id"],
  "properties": {
    "input":   {"type": "string", "minLength": 1},
    "user_id": {"type": "string", "minLength": 1},
    "scope":   {"type": "string"},
    "top_k":   {"type": "integer", "minimum": 1, "maximum": 50}
  }
}`)

var entityResolveOutputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": true,
  "required": ["schema_version", "status", "source"],
  "properties": {
    "schema_version": {"type": "string"},
    "status":         {"type": "string", "enum": ["resolved", "ambiguous", "failed"]},
    "source":         {"type": "object"}
  }
}`)

// -------------------- registration --------------------
//
// SCOPE-1 foundation rule: registration is gated behind
// SetEntityResolveServices so the package init alone does not
// pollute the spec 037 registry.

// init registers the tool at package import time so the spec 037
// loader (scenario-lint, cmd/core) recognizes the tool name; the
// handler returns entity_resolve_not_configured until
// SetEntityResolveServices wires runtime dependencies.
func init() { entityResolveRegisterOnce.Do(registerEntityResolve) }

func registerEntityResolve() {
	agent.RegisterTool(agent.Tool{
		Name:             EntityResolveToolName,
		Description:      "Resolve a colloquial reference (e.g. \"the lease\") to a ranked list of user-scoped artifact references. Returns resolved when the top candidate's confidence clears the configured floor, ambiguous when it does not, and failed when no candidates exist.",
		InputSchema:      entityResolveInputSchema,
		OutputSchema:     entityResolveOutputSchema,
		SideEffectClass:  agent.SideEffectRead,
		OwningPackage:    "internal/agent/tools/microtools",
		PerCallTimeoutMs: 2500,
		Handler:          handleEntityResolve,
	})
}

// -------------------- handler --------------------

type entityResolveInput struct {
	Input  string `json:"input"`
	UserID string `json:"user_id"`
	Scope  string `json:"scope,omitempty"`
	TopK   int    `json:"top_k,omitempty"`
}

func handleEntityResolve(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	svc, err := loadEntityResolveServices()
	if err != nil {
		return nil, err
	}
	var in entityResolveInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("entity_resolve_bad_input: %w", err)
	}
	if in.UserID == "" {
		return nil, errors.New("entity_resolve_missing_user_id")
	}
	if in.Input == "" {
		return nil, errors.New("entity_resolve_empty_input")
	}

	limit := in.TopK
	if limit < 1 || limit > svc.MaxCandidates {
		limit = svc.MaxCandidates
	}

	callCtx, cancel := context.WithTimeout(ctx, svc.Timeout)
	defer cancel()

	cands, rerr := svc.Resolver.Resolve(callCtx, in.UserID, in.Input, in.Scope, limit)
	if rerr != nil {
		return marshalEntityEnvelope(entityFailed("resolver_error", rerr.Error()))
	}
	if len(cands) == 0 {
		return marshalEntityEnvelope(entityFailed("no_candidates", "resolver returned zero candidates"))
	}

	// Defensive: clamp scores into [0, 1] before envelope validation.
	for i := range cands {
		if cands[i].Score < 0 {
			cands[i].Score = 0
		}
		if cands[i].Score > 1 {
			cands[i].Score = 1
		}
	}

	top := cands[0]
	if top.Score >= svc.ConfidenceFloor {
		return marshalEntityEnvelope(entityResolved(top, cands))
	}
	return marshalEntityEnvelope(entityAmbiguous(cands))
}

func entityResolved(top EntityCandidate, all []EntityCandidate) Envelope {
	return Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Status:        StatusResolved,
		Value: map[string]any{
			"artifact_id":   top.ArtifactID,
			"label":         top.Label,
			"artifact_type": top.ArtifactType,
			"snippet":       top.Snippet,
			"captured_at":   top.CapturedAt,
			"candidates":    candidateValues(all),
		},
		Confidence: top.Score,
		Source: Source{
			Provider:    "graph",
			Kind:        SourceKindGraphRead,
			RetrievedAt: time.Now().UTC(),
			Attribution: "Data: user knowledge graph",
		},
	}
}

func entityAmbiguous(cands []EntityCandidate) Envelope {
	out := make([]Candidate, 0, len(cands))
	for i, c := range cands {
		out = append(out, Candidate{
			Rank:           i + 1,
			Label:          c.Label,
			Value:          candidateValue(c),
			Confidence:     c.Score,
			Distinguishing: c.Snippet,
		})
	}
	return Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Status:        StatusAmbiguous,
		Candidates:    out,
		Source: Source{
			Provider:    "graph",
			Kind:        SourceKindGraphRead,
			RetrievedAt: time.Now().UTC(),
			Attribution: "Data: user knowledge graph",
		},
	}
}

func entityFailed(code, msg string) Envelope {
	return Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Status:        StatusFailed,
		Source: Source{
			Provider:    "graph",
			Kind:        SourceKindGraphRead,
			RetrievedAt: time.Now().UTC(),
			Attribution: "Data: user knowledge graph",
		},
		Error: &Error{Code: code, Message: msg},
	}
}

func candidateValue(c EntityCandidate) map[string]any {
	return map[string]any{
		"artifact_id":   c.ArtifactID,
		"label":         c.Label,
		"artifact_type": c.ArtifactType,
		"snippet":       c.Snippet,
		"captured_at":   c.CapturedAt,
		"score":         c.Score,
	}
}

func candidateValues(cs []EntityCandidate) []map[string]any {
	out := make([]map[string]any, 0, len(cs))
	for _, c := range cs {
		out = append(out, candidateValue(c))
	}
	return out
}

func marshalEntityEnvelope(env Envelope) (json.RawMessage, error) {
	if err := ValidateEnvelope(env); err != nil {
		return nil, fmt.Errorf("entity_resolve_envelope_invalid: %w", err)
	}
	return json.Marshal(env)
}
