// Spec 061 SCOPE-07 — facade-side adapter for the weather skill's
// source-assembly invariant. The capability-layer Facade calls this
// adapter (registered as a contracts.SourceAssembler in cmd/core
// wiring) BETWEEN agent.Executor.Run and provenance.Enforce, so
// resp.Sources carries the ExternalProviderRef BEFORE the gate
// inspects it.
//
// Why this lives in internal/agent/tools/weather and not in
// internal/assistant: spec 037 freezes internal/agent (per design.md
// §10 + §11.3 architecture tests forbid `internal/assistant/skills/`).
// The skill owns its own output shape — the facade only knows the
// contracts.SourceAssembler seam.
//
// Output shape contract (config/prompt_contracts/weather-query-v1.yaml
// output_schema, verbatim): the executor's Final is a JSON object of
// the form:
//
//	{
//	  "forecast_line": string,
//	  "provider_name": string,
//	  "retrieved_at":  string (RFC3339 timestamp)
//	}
//
// The adapter:
//
//  1. Parses Final into the local payload struct.
//  2. Returns zero-value SourceAssembly when any of the following
//     hold:
//       - result == nil (defensive)
//       - result.Outcome != OutcomeOK
//       - len(result.Final) == 0
//       - json.Unmarshal fails
//       - any of forecast_line / provider_name / retrieved_at is
//         missing or unparseable
//     The Facade then keeps its default-rendered Body and the
//     provenance gate refuses the response (weather_query is
//     requires_provenance: true), routing through the BS-006 capture
//     path. Refusing to fabricate attribution is the
//     anti-fabrication invariant.
//  3. On success, returns:
//       Body          = forecast_line (replaces default-rendered body)
//       Sources       = [Source{Kind: SourceExternalProvider,
//                               Ref:  ExternalProviderRef{
//                                   ProviderName, RetrievedAt}}]
//       OverflowCount = 0 (weather emits at most one provider source)
//
// RetrievedAt invariant (design.md §5.2): when the tool returns a
// cache hit, the weather tool emits the ORIGINAL upstream
// retrieved_at, NOT the cache-hit wall clock. This assembler simply
// trusts whatever the tool emits in the Final — preserving that
// invariant is the tool's responsibility (see tool.go marshalForecast
// and cache_test.go preservation cases).

package weather

import (
	"context"
	"encoding/json"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// weatherFinalPayload mirrors the attribution-relevant subset of the
// weather-query-v1.yaml output schema. The provenance gate's decision
// (assemble vs. refuse) depends ONLY on forecast_line (the Body) plus
// the two attribution fields, so this struct intentionally parses only
// those three. The spec-094 additive fields (current/daily/units) are
// deliberately NOT declared here — encoding/json drops the unmapped
// keys, and the facade has no use for them: the rich structured blocks
// ride through verbatim in the executor's Final (direct_output_from_tool)
// for machine consumers, while the human Body stays forecast_line. Field
// tags match the schema property names exactly; any drift between these
// three and the YAML schema is caught by scenario-lint and by
// tool_test.go assertions on marshalForecast output.
type weatherFinalPayload struct {
	ForecastLine string `json:"forecast_line"`
	ProviderName string `json:"provider_name"`
	RetrievedAt  string `json:"retrieved_at"`
}

// NewFacadeAssembler returns a contracts.SourceAssembler the
// capability-layer Facade can register for the weather scenario
// (registry key matches the scenario id, "weather_query").
//
// The sourcesMax parameter is accepted to match the shape of every
// other facade-assembler constructor (e.g. retrieval). Weather emits
// at most one provider source per response, so sourcesMax is never
// reached in practice; we still validate sourcesMax > 0 at wiring
// time so configuration drift is caught before the first request.
//
// Returns: a SourceAssembler closure. Panics on non-positive
// sourcesMax (wiring-time configuration bug, not a runtime data
// condition).
func NewFacadeAssembler(sourcesMax int) contracts.SourceAssembler {
	if sourcesMax <= 0 {
		panic("weather: NewFacadeAssembler requires sourcesMax > 0")
	}
	return func(ctx context.Context, result *agent.InvocationResult) contracts.SourceAssembly {
		// Defensive: the Facade already nil-checks before invoking,
		// but a future caller (e.g. a unit test) may pass nil and
		// the assembler must return safely.
		if result == nil {
			return contracts.SourceAssembly{}
		}
		// Weather emits sources only on success. Timeouts / provider
		// errors / validation failures route through the Facade's
		// default-body path; the gate then refuses
		// requires_provenance scenarios with empty Sources, which is
		// the BS-006 provider-unavailable signal.
		if result.Outcome != agent.OutcomeOK {
			return contracts.SourceAssembly{}
		}
		if len(result.Final) == 0 {
			return contracts.SourceAssembly{}
		}
		var payload weatherFinalPayload
		if err := json.Unmarshal(result.Final, &payload); err != nil {
			// Malformed Final — the scenario was misconfigured or
			// the model produced something that bypassed schema
			// validation. We DO NOT fabricate attribution; gate
			// refuses (requires_provenance: true).
			return contracts.SourceAssembly{}
		}
		// All three fields are mandatory for attribution. Missing
		// any one of them → refuse to assemble (anti-fabrication
		// invariant). The provenance gate will then refuse the
		// response and the BS-006 capture path takes over.
		if payload.ForecastLine == "" || payload.ProviderName == "" || payload.RetrievedAt == "" {
			return contracts.SourceAssembly{}
		}
		// Parse retrieved_at as RFC3339 — the tool's marshalForecast
		// always emits RFC3339 UTC. We accept either pure RFC3339 or
		// RFC3339Nano for forward compatibility with providers that
		// emit sub-second precision.
		retrievedAt, err := parseProviderTimestamp(payload.RetrievedAt)
		if err != nil {
			return contracts.SourceAssembly{}
		}
		return contracts.SourceAssembly{
			Body: payload.ForecastLine,
			Sources: []contracts.Source{{
				ID:    payload.ProviderName,
				Title: payload.ProviderName,
				Kind:  contracts.SourceExternalProvider,
				Ref: contracts.ExternalProviderRef{
					ProviderName: payload.ProviderName,
					RetrievedAt:  retrievedAt,
				},
			}},
			OverflowCount: 0,
		}
	}
}

// parseProviderTimestamp accepts RFC3339 or RFC3339Nano. RFC3339Nano
// is a strict superset of RFC3339, so we try the more permissive
// parser first.
func parseProviderTimestamp(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}
