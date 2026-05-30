// BUG-061-003 — facade-side assembler for the recipe_search skill.
//
// Behavior contract (design.md D5):
//
//   - Final shape: {"answer": string, "cited_artifact_ids": [string]}
//     produced by the recipe-search-v1 scenario.
//   - Zero-hit case (Outcome=OK, answer=="" AND cited_artifact_ids
//     empty) → emit a ResponseOverride with StatusUnavailable,
//     ErrCause=ErrNoMatch, CaptureRoute=false, and an actionable
//     body naming the next concrete action (capture or import).
//     The facade applies the override verbatim AND skips the
//     provenance gate.
//   - Non-empty case → delegate to retrieval.AssembleSources so the
//     existing artifact-lookup + drop/overflow accounting is reused.
//   - Any other Outcome (timeout, schema fail, etc.) returns zero-
//     value SourceAssembly so the facade's default body rendering
//     applies and the provenance gate refuses for requires_provenance.
package recipesearch

import (
	"context"
	"encoding/json"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/tools/retrieval"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// ScenarioID is the canonical scenario id for recipe_search.
const ScenarioID = "recipe_search"

// EmptyGraphBody is the actionable Principle-8 body emitted when
// the owned graph has zero recipe matches. Exposed as a package
// constant so tests can assert the contract without duplicating
// the literal string.
const EmptyGraphBody = "no recipes saved yet — capture one with /capture or import via a connector."

type recipeFinalPayload struct {
	Answer           string   `json:"answer"`
	CitedArtifactIDs []string `json:"cited_artifact_ids"`
}

// NewFacadeAssembler returns the recipe_search source assembler.
// Panics on misconfiguration at wiring time.
func NewFacadeAssembler(lookup retrieval.ArtifactLookupFn, sourcesMax int) contracts.SourceAssembler {
	if lookup == nil {
		panic("recipesearch: NewFacadeAssembler requires a non-nil ArtifactLookupFn")
	}
	if sourcesMax <= 0 {
		panic("recipesearch: NewFacadeAssembler requires sourcesMax > 0")
	}
	return func(ctx context.Context, result *agent.InvocationResult) contracts.SourceAssembly {
		if result == nil {
			return contracts.SourceAssembly{}
		}
		if result.Outcome != agent.OutcomeOK {
			return contracts.SourceAssembly{}
		}
		if len(result.Final) == 0 {
			return contracts.SourceAssembly{}
		}
		var payload recipeFinalPayload
		if err := json.Unmarshal(result.Final, &payload); err != nil {
			return contracts.SourceAssembly{}
		}

		// Zero-hit deterministic state: the LLM honored rule 3 of
		// the system prompt and produced an empty Final because the
		// recipe_search tool returned no artifacts. Translate to a
		// StatusUnavailable override; the facade skips the
		// provenance gate AND the BandLow capture branch.
		if payload.Answer == "" && len(payload.CitedArtifactIDs) == 0 {
			return contracts.SourceAssembly{
				Override: &contracts.ResponseOverride{
					Status:       contracts.StatusUnavailable,
					ErrorCause:   contracts.ErrNoMatch,
					CaptureRoute: false,
					Body:         EmptyGraphBody,
				},
			}
		}

		sources, overflow := retrieval.AssembleSources(ctx, ScenarioID, payload.CitedArtifactIDs, lookup, sourcesMax)
		var cause contracts.ProvenanceCause
		if payload.Answer != "" && len(sources) == 0 {
			switch {
			case sourcesMax <= 0:
				cause = contracts.ProvenanceCauseDroppedForQuota
			case len(payload.CitedArtifactIDs) == 0:
				cause = contracts.ProvenanceCauseFabricatedSource
			default:
				cause = contracts.ProvenanceCauseMissingArtifact
			}
		}
		return contracts.SourceAssembly{
			Body:          payload.Answer,
			Sources:       sources,
			OverflowCount: overflow,
			Cause:         cause,
		}
	}
}
