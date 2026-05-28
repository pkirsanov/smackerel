// Package contracts — SourceAssembler is the per-scenario
// post-processor surface invoked by the capability-layer Facade
// BandHigh dispatch between executor.Run and provenance.Enforce.
//
// Spec 061 SCOPE-04 (facade) + SCOPE-06 (retrieval rendering) own
// the rationale together:
//
//   - The Facade does not know how to translate a scenario's
//     Final answer JSON into user-visible Sources. The shape of
//     Final is scenario-specific (retrieval_qa returns
//     {answer, cited_artifact_ids}; other scenarios return a
//     plain string or a different envelope).
//
//   - The retrieval scenario's source-assembly invariant
//     (cited_artifact_ids → AssembleSources lookup → []Source) is
//     owned by internal/agent/tools/retrieval — it MUST NOT leak
//     into the capability-layer Facade, because spec 037 freezes
//     internal/agent and forbids skill-specific code in the
//     assistant substrate.
//
//   - The provenance gate (provenance.Enforce) requires
//     resp.Sources to be populated BEFORE it runs; otherwise it
//     correctly refuses every retrieval_qa response as a Principle
//     8 violation (the SCOPE-04 facade-source-assembly-hook bug).
//
// SourceAssembler is the single seam that connects these three
// requirements without re-introducing the parallel agent substrate
// the architecture forbids. The Facade looks up a SourceAssembler by
// scenario id; if registered, it invokes the assembler between the
// executor and the gate; the assembler returns the assembled Sources
// (and optionally a re-rendered Body extracted from Final) which the
// Facade splices into the AssistantResponse.
//
// Wiring rules:
//
//  1. SourceAssembler MUST NOT mutate the executor's result.
//  2. SourceAssembler MUST return a zero-value SourceAssembly when
//     the scenario's Final does not match the expected shape; the
//     Facade then leaves resp.Sources and resp.Body untouched (the
//     translateFinalToBody default-rendering path stands).
//  3. SourceAssembler MUST NOT panic on malformed input — emit a
//     metric counter increment and return zero-value SourceAssembly.
//  4. Registration is keyed by scenario id (matching the manifest
//     metadata key the Facade already consults).
package contracts

import (
	"context"

	"github.com/smackerel/smackerel/internal/agent"
)

// SourceAssembler is the function signature for per-scenario
// post-processors invoked by the Facade BandHigh dispatch path.
//
// Parameters:
//   - ctx:     the request-scoped context (carries trace IDs, deadlines).
//   - result:  the executor's InvocationResult; never nil when invoked.
//     The assembler reads result.Final and result.Outcome.
//
// Returns: a SourceAssembly describing how the Facade should splice
// the assembler's output into the AssistantResponse. See the
// SourceAssembly doc comment for field semantics.
//
// Contract — non-negotiable invariants the Facade relies on:
//   - MUST be deterministic for identical inputs.
//   - MUST NOT block on unbounded I/O; if a downstream artifact
//     lookup is required, it MUST be wrapped with a deadline derived
//     from ctx.
//   - MUST NOT return more than the configured SourcesMax entries
//     in Sources (the assembler is responsible for honoring the
//     cap and reporting the truncation count in OverflowCount).
//   - MUST return a zero-value SourceAssembly (Body=="" && Sources==nil
//     && OverflowCount==0) when result.Final is empty, malformed,
//     or does not match the scenario's expected output shape. The
//     Facade interprets a zero-value return as "no override" and
//     keeps its default-rendered Body + empty Sources.
type SourceAssembler func(ctx context.Context, result *agent.InvocationResult) SourceAssembly

// SourceAssembly is the per-scenario post-processor output the
// Facade splices into the AssistantResponse before invoking
// provenance.Enforce.
//
// Field semantics:
//
//   - Body: when non-empty, REPLACES resp.Body. When empty, the
//     Facade keeps the body computed by translateFinalToBody.
//     Used by retrieval_qa to render the `answer` field of the
//     scenario's Final JSON as the user-facing reply, instead of
//     leaking the raw JSON envelope.
//
//   - Sources: REPLACES resp.Sources unconditionally. nil is
//     valid and signals "no sources were assembled" — the
//     provenance gate will then refuse the response if the
//     scenario sets requires_provenance=true and Body is non-empty.
//     This is the BS-007 graph-drift refusal path.
//
//   - OverflowCount: count of sources that were truncated by the
//     SourcesMax cap inside the assembler. Surfaces in
//     resp.SourcesOverflowCount for telemetry and UI tail
//     ("…and N more sources").
//
// Cardinality and zero-value: a zero-value SourceAssembly
// (Body == "" && Sources == nil && OverflowCount == 0 && Cause == "")
// is the canonical "no override" signal the Facade interprets as
// "this assembler does not apply to this scenario's Final shape".
type SourceAssembly struct {
	Body          string
	Sources       []Source
	OverflowCount int
	// Cause is the spec 061 SCOPE-09 attribution hint the
	// provenance gate uses when it has to rewrite to the canonical
	// refusal. Set by the assembler when Sources is empty but the
	// scenario emitted a non-empty body (i.e. the gate is about to
	// fire). The Facade forwards Cause to provenance.Enforce so the
	// ViolationsCounter can attribute the rewrite to the originating
	// upstream condition (missing artifact, lookup error, fabricated
	// source, or drop-for-quota). Empty Cause is valid and means
	// "the assembler did not classify it"; the gate defaults to
	// ProvenanceCauseFabricatedSource because a non-empty body with
	// no Sources is — by definition — a fabricated answer.
	Cause ProvenanceCause
}

// ProvenanceCause is the closed-vocabulary set of attribution causes
// the provenance gate uses to label the
// smackerel_assistant_provenance_violations_total counter (spec 061
// SCOPE-09). Cardinality is bounded so dashboards can distinguish
// graph-drift from LLM fabrication without unbounded label growth.
type ProvenanceCause string

const (
	// ProvenanceCauseMissingArtifact — the LLM cited artifact IDs
	// but the source-assembly lookup returned found=false for every
	// citation (graph drift). Pair with the assembly-drops counter
	// cause=missing_artifact to confirm the upstream signal.
	ProvenanceCauseMissingArtifact ProvenanceCause = "missing_artifact"
	// ProvenanceCauseLookupError — the LLM cited artifact IDs but
	// every lookup errored (typically a transient PG outage). Pair
	// with the assembly-drops counter cause=lookup_error.
	ProvenanceCauseLookupError ProvenanceCause = "lookup_error"
	// ProvenanceCauseFabricatedSource — the LLM emitted a non-empty
	// body without any cited artifact IDs at all. This is the
	// "model hallucinated context" case and is the default cause
	// the gate falls back to when the assembler did not classify.
	ProvenanceCauseFabricatedSource ProvenanceCause = "fabricated_source"
	// ProvenanceCauseDroppedForQuota — the sources_max cap was
	// configured to 0 (a misconfiguration) so even valid lookups
	// were truncated to an empty Sources slice.
	ProvenanceCauseDroppedForQuota ProvenanceCause = "dropped_for_quota"
)

// AllProvenanceCauses is the canonical iteration order for
// closed-vocabulary tests. Adding a new cause MUST update this slice
// + the labels_test vocabulary fixture in the metrics package.
var AllProvenanceCauses = []ProvenanceCause{
	ProvenanceCauseMissingArtifact,
	ProvenanceCauseLookupError,
	ProvenanceCauseFabricatedSource,
	ProvenanceCauseDroppedForQuota,
}
