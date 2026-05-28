// Spec 061 SCOPE-06 + SCOPE-04 \u2014 facade-side adapter for the
// retrieval skill's source-assembly invariant. The capability-layer
// Facade calls this adapter (registered as a
// contracts.SourceAssembler in cmd/core wiring) BETWEEN
// agent.Executor.Run and provenance.Enforce, so resp.Sources is
// populated before the provenance gate inspects it.
//
// Why this lives in internal/agent/tools/retrieval and not in
// internal/assistant: spec 037 freezes internal/agent and forbids
// skill-specific code in the assistant substrate (design.md
// \u00a710 + \u00a711.3 architecture tests). The skill owns its own output
// shape \u2014 the facade only knows about the contracts.SourceAssembler
// seam.
//
// Output shape contract (config/prompt_contracts/retrieval-qa-v1.yaml
// output_schema, verbatim): the executor's Final is a JSON object
// of the form:
//
//	{"answer": string, "cited_artifact_ids": [string, ...]}
//
// The adapter:
//
//  1. Parses Final into the local payload struct.
//  2. Returns zero-value SourceAssembly when Final is empty, has the
//     wrong shape (json.Unmarshal error), or the outcome is anything
//     other than OK \u2014 the Facade then keeps its default-rendered
//     Body and the provenance gate refuses requires_provenance
//     scenarios with empty Sources.
//  3. On success, calls AssembleSources to translate the cited IDs
//     into the gate-ready []contracts.Source (drops missing IDs +
//     counts overflow per design \u00a75.1) and returns the answer
//     field as Body override so the user sees the synthesized text
//     instead of the raw JSON envelope.

package retrieval

import (
	"context"
	"encoding/json"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// retrievalFinalPayload mirrors the retrieval-qa-v1.yaml output
// schema. Field tags match the schema property names exactly; any
// drift between this struct and the YAML schema is caught by
// retrieval-qa-v1's prompt-contract tests in
// internal/agent/tools/retrieval/tool_test.go.
type retrievalFinalPayload struct {
	Answer           string   `json:"answer"`
	CitedArtifactIDs []string `json:"cited_artifact_ids"`
}

// NewFacadeAssembler returns a contracts.SourceAssembler the
// capability-layer Facade can register for the retrieval scenario
// (registry key matches the scenario id, e.g. "retrieval_qa").
//
// Parameters:
//   - lookup:     ArtifactLookupFn the assembler consults for each
//     cited artifact id. Production wiring uses *db.Postgres-backed
//     lookup; tests inject in-memory map closures.
//   - sourcesMax: the capability-layer SST cap forwarded into
//     AssembleSources (typically assistant.sources_max from SCOPE-01).
//
// Both parameters are required \u2014 the constructor panics on nil
// lookup or non-positive sourcesMax at wiring time so configuration
// drift is caught before the first request. (The Facade does not
// re-validate, because by the time the assembler runs we already
// trust the wiring contract; misconfiguration here is a wiring bug,
// not a runtime data condition.)
func NewFacadeAssembler(scenarioID string, lookup ArtifactLookupFn, sourcesMax int) contracts.SourceAssembler {
	if scenarioID == "" {
		panic("retrieval: NewFacadeAssembler requires a non-empty scenarioID")
	}
	if lookup == nil {
		panic("retrieval: NewFacadeAssembler requires a non-nil ArtifactLookupFn")
	}
	if sourcesMax <= 0 {
		panic("retrieval: NewFacadeAssembler requires sourcesMax > 0")
	}
	return func(ctx context.Context, result *agent.InvocationResult) contracts.SourceAssembly {
		// Defensive: the Facade already nil-checks before invoking,
		// but a future caller (e.g. a unit test) may pass nil and
		// the assembler must return safely.
		if result == nil {
			return contracts.SourceAssembly{}
		}
		// The retrieval scenario only emits sources on success.
		// Timeouts / provider errors / validation failures all
		// produce a default body via the Facade's
		// translateFinalToBody path; the assembler stays out of the
		// way so the gate can either refuse (requires_provenance)
		// or pass through (advisory scenarios).
		if result.Outcome != agent.OutcomeOK {
			return contracts.SourceAssembly{}
		}
		if len(result.Final) == 0 {
			return contracts.SourceAssembly{}
		}
		var payload retrievalFinalPayload
		if err := json.Unmarshal(result.Final, &payload); err != nil {
			// Malformed Final (e.g. plain string literal because the
			// scenario was misconfigured to bypass the JSON schema).
			// The Facade keeps the default-rendered Body; the gate
			// refuses if requires_provenance. We intentionally do
			// NOT increment a metric here \u2014 the upstream agent
			// substrate already records schema validation failures
			// (spec 037 OutcomeSchemaFailure counter).
			return contracts.SourceAssembly{}
		}
		sources, overflow := AssembleSources(ctx, scenarioID, payload.CitedArtifactIDs, lookup, sourcesMax)
		// Spec 061 SCOPE-09 — when Sources is empty but the LLM
		// emitted a body, classify the provenance-gate cause so
		// dashboards can distinguish graph-drift from fabrication.
		// We attribute by the most specific observable signal:
		//   - LLM cited IDs but cap was 0     → dropped_for_quota
		//   - LLM cited IDs but ALL dropped   → missing_artifact
		//     (the dominant cause in this scenario; lookup_error is
		//     observable on the assembly-drops counter for the
		//     dashboard-side breakdown)
		//   - LLM emitted body with 0 citations → fabricated_source
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
