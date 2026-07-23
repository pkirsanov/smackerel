// Spec 064 SCOPE-12 — facade source-assembler for the open_knowledge
// scenario.
//
// Flow: the substrate executor runs the `open_knowledge` scenario,
// which is wired with `direct_output_from_tool: open_knowledge_invoke`.
// The substrate Handler (internal/assistant/openknowledge/agenttool/
// substrate_tool.go::Handler) returns a JSON envelope of the form:
//
//	{
//	  "status":         "success" | "refused",
//	  "body":           "<final answer or canonical refusal body>",
//	  "refusal_cause":  "<contracts.RefusalCause>" (empty on success),
//	  "termination":    "<okagent.TerminationReason>",
//	  "sources":        [ { "kind": "...", ... }, ... ]
//	}
//
// That envelope becomes the executor's InvocationResult.Final. The
// capability-layer Facade calls this assembler BETWEEN executor.Run
// and provenance.Enforce; the assembler:
//
//  1. Parses the envelope.
//  2. Translates each `sources[]` entry into a contracts.Source per
//     the kind discriminator. Mapping:
//       "artifact"         → contracts.SourceArtifact +
//                            contracts.ArtifactRef{ArtifactID, CapturedAt}
//       "web"              → contracts.SourceWeb +
//                            contracts.WebSourceRef{URL, Provider,
//                                                  FetchedAt, ContentHash,
//                                                  Snippet}
//       "tool_computation" → contracts.SourceToolComputation +
//                            contracts.ComputationSourceRef{Tool,
//                                                            InputHash,
//                                                            OutputHash}
//  3. Replaces resp.Body with envelope.Body (the executor would
//     otherwise emit the raw JSON envelope as the user-visible body).
//  4. Replaces resp.Sources with the assembled slice (capped at
//     sourcesMax; OverflowCount records the truncation count).
//
// Without this assembler the provenance gate (extended via PKT-061-A
// to accept SourceWeb + SourceToolComputation) sees empty Sources on
// every open-knowledge response and rewrites every body to the
// canonical refusal — the BS-007 fabricated-source path. This file is
// the critical bridge that turns the gate from "refuse everything" to
// "trust what the cite-back verifier already verified inside the
// agent loop".
//
// Anti-fabrication invariant: this assembler trusts the substrate
// Handler's source list because the open-knowledge agent's
// internal/assistant/openknowledge/citeback verifier already rejected
// any fabricated citations BEFORE the Handler marshalled the envelope.
// Status=="refused" entries from the Handler carry an empty sources
// list per substrate_tool.go::MapTurnResult.
//
// NO-DEFAULTS (G028): every field is either present in the JSON or
// the assembler returns zero-value SourceAssembly (the Facade then
// falls through to the default-rendered body path and the gate
// rewrites to refusal — the safe fail-closed behaviour). The
// assembler never invents a Title/URL/Provider/Hash.

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// openKnowledgeEnvelope mirrors the JSON the substrate Handler emits.
// Field names match agenttool/substrate_tool.go::outputEnvelope JSON
// tags verbatim.
type openKnowledgeEnvelope struct {
	Status       string                   `json:"status"`
	Body         string                   `json:"body"`
	RefusalCause string                   `json:"refusal_cause"`
	Termination  string                   `json:"termination,omitempty"`
	Sources      []map[string]interface{} `json:"sources"`
}

// newOpenKnowledgeAssembler returns a contracts.SourceAssembler for
// the "open_knowledge" scenario. Panics on non-positive sourcesMax
// (wiring-time configuration bug, not a runtime condition) to match
// the existing weather / retrieval assemblers' construction contract.
func newOpenKnowledgeAssembler(sourcesMax int) contracts.SourceAssembler {
	if sourcesMax <= 0 {
		panic("open_knowledge: newOpenKnowledgeAssembler requires sourcesMax > 0")
	}
	return func(_ context.Context, result *agent.InvocationResult) contracts.SourceAssembly {
		if result == nil {
			return contracts.SourceAssembly{}
		}
		if result.Outcome != agent.OutcomeOK {
			return contracts.SourceAssembly{}
		}
		if len(result.Final) == 0 {
			return contracts.SourceAssembly{}
		}
		var env openKnowledgeEnvelope
		if err := json.Unmarshal(result.Final, &env); err != nil {
			return contracts.SourceAssembly{}
		}
		// "refused" envelopes carry the honest, cause-specific canonical
		// refusal body already (the Handler emits CanonicalRefusalBodyFor
		// (cause)). BUG-061-009 — a refused open_knowledge turn is a
		// high-band REFUSAL, surfaced HONESTLY as StatusUnavailable with the
		// cause-specific body, and NEVER the band-low "saved as an idea"
		// capture. Emit a deterministic Override (which bypasses the
		// provenance gate) so the cause-specific body is not overwritten by
		// the gate's default refusal text. A typed (non-default) spec-064
		// cause is carried in ErrorCause so the transport can render the
		// cause-specific refusal; the default/unknown case uses the umbrella
		// ErrNoGroundedAnswer so the honest body still renders verbatim.
		if env.Status == "refused" {
			cause := contracts.RefusalCause(env.RefusalCause)
			errCause := contracts.ErrNoGroundedAnswer
			if cause != contracts.RefusalDefault && cause != "" {
				errCause = contracts.ErrorCause(string(cause))
			}
			return contracts.SourceAssembly{
				Override: &contracts.ResponseOverride{
					Status:       contracts.StatusUnavailable,
					ErrorCause:   errCause,
					Body:         env.Body,
					CaptureRoute: false,
				},
			}
		}
		// "success" envelopes — translate sources.
		assembled := make([]contracts.Source, 0, len(env.Sources))
		overflow := 0
		for _, raw := range env.Sources {
			src, ok := convertOpenKnowledgeSource(raw)
			if !ok {
				// Malformed entry — skip silently rather than
				// fabricate a Source. The gate will refuse if the
				// final Sources slice is empty.
				continue
			}
			if len(assembled) >= sourcesMax {
				overflow++
				continue
			}
			assembled = append(assembled, src)
		}
		// Empty Sources after a "success" envelope means the agent
		// returned an answer but every source entry was malformed.
		// Return zero-value SourceAssembly so the Facade falls
		// through to the default body path and the gate refuses
		// (anti-fabrication: never emit a body without verified
		// attribution).
		if len(assembled) == 0 {
			return contracts.SourceAssembly{}
		}
		return contracts.SourceAssembly{
			Body:          env.Body,
			Sources:       assembled,
			OverflowCount: overflow,
		}
	}
}

// convertOpenKnowledgeSource maps one raw entry to a contracts.Source.
// Returns ok=false on missing/invalid required fields per Kind; the
// caller drops the entry.
func convertOpenKnowledgeSource(raw map[string]interface{}) (contracts.Source, bool) {
	kindStr, _ := raw["kind"].(string)
	switch kindStr {
	case "artifact":
		id, _ := raw["artifact_id"].(string)
		if strings.TrimSpace(id) == "" {
			return contracts.Source{}, false
		}
		title, _ := raw["title"].(string)
		return contracts.Source{
			ID:    id,
			Title: title,
			Kind:  contracts.SourceArtifact,
			Ref:   contracts.ArtifactRef{ArtifactID: id, CapturedAt: time.Time{}},
		}, true
	case "web":
		url, _ := raw["url"].(string)
		provider, _ := raw["provider"].(string)
		hash, _ := raw["content_hash"].(string)
		if strings.TrimSpace(url) == "" || strings.TrimSpace(provider) == "" || strings.TrimSpace(hash) == "" {
			return contracts.Source{}, false
		}
		title, _ := raw["title"].(string)
		snippet, _ := raw["snippet"].(string)
		return contracts.Source{
			ID:    url,
			Title: title,
			Kind:  contracts.SourceWeb,
			Ref: contracts.WebSourceRef{
				URL:         url,
				Provider:    provider,
				FetchedAt:   time.Time{},
				ContentHash: hash,
				Snippet:     snippet,
			},
		}, true
	case "tool_computation":
		tool, _ := raw["tool"].(string)
		if strings.TrimSpace(tool) == "" {
			return contracts.Source{}, false
		}
		inputHash := hashAny(raw["input"])
		outputHash := hashAny(raw["output"])
		if inputHash == "" || outputHash == "" {
			return contracts.Source{}, false
		}
		return contracts.Source{
			ID:    tool,
			Title: tool,
			Kind:  contracts.SourceToolComputation,
			Ref: contracts.ComputationSourceRef{
				Tool:       tool,
				InputHash:  inputHash,
				OutputHash: outputHash,
			},
		}, true
	default:
		return contracts.Source{}, false
	}
}

// hashAny canonicalises a JSON-decoded value (string, number, bool,
// nested map/slice) and returns the sha256 hex of its JSON
// re-encoding, prefixed with "sha256:" to match the spec 061
// provenance gate's expected hash format.
func hashAny(v interface{}) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 {
		return ""
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}
