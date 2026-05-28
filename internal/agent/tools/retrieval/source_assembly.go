// Spec 061 SCOPE-06 — source-assembly invariant for the retrieval
// skill. The Spec 037 substrate's tool-loop terminates with a JSON
// body of the shape {answer: string, cited_artifact_ids: []string}
// (per config/prompt_contracts/retrieval-qa-v1.yaml output schema).
// The capability layer then MUST translate those cited IDs into the
// contracts.Source[] the provenance gate inspects.
//
// This file owns the translation. It is a pure function (no global
// state apart from the metrics counter increment) so:
//
//   - it can be unit-tested without spinning up PG;
//   - it is safe to invoke from the facade once SCOPE-04 wires the
//     per-scenario source-assembly hook (see scopes.md SCOPE-06 DoD
//     evidence pointer for the routed finding);
//   - it can be invoked directly from any future capability-layer
//     post-processor without re-implementing the drift contract.
//
// design.md §5.1 contract (verbatim):
//
//	"Capability facade assembles []contracts.Source from
//	 cited_artifact_ids (NOT from the raw hits — only what the LLM
//	 actually cited). Drops cited_artifact_ids for missing artifacts
//	 (graph drift) and increments
//	 smackerel_assistant_source_assembly_drops_total{cause=
//	 \"missing_artifact\"}. If ALL citations are missing, Sources is
//	 empty and the provenance gate fires (refusal + capture)."
package retrieval

import (
	"context"
	"errors"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	assistantmetrics "github.com/smackerel/smackerel/internal/assistant/metrics"
)

// ArtifactLookupFn is the lookup callback the source-assembly
// invariant consults for each cited artifact ID. Production wiring
// supplies a callback that reads (title, created_at) from the
// artifacts table; tests supply a synchronous in-memory map.
//
// Contract:
//   - Return found=true with a populated title + capturedAt when the
//     artifact exists in the graph.
//   - Return found=false with a nil error when the artifact has been
//     deleted, pruned, or never existed (graph drift). The assembler
//     drops the ID and increments the missing_artifact counter.
//   - Return found=false with a non-nil error for transient lookup
//     failures (PG outage, timeout). The assembler drops the ID and
//     increments the lookup_error counter rather than crashing the
//     whole response.
type ArtifactLookupFn func(ctx context.Context, artifactID string) (title string, capturedAt time.Time, found bool, err error)

// AssembleSources is the pure source-assembly invariant. Given the
// LLM's cited_artifact_ids and a lookup callback, returns the
// gate-ready []contracts.Source plus the overflow count that the
// caller MUST forward into contracts.AssistantResponse.SourcesOverflowCount.
//
// Behavior (matches design §5.1 verbatim):
//
//  1. citedIDs are processed in input order. Duplicates ARE collapsed
//     so the LLM citing the same artifact twice does not double-spend
//     the sources_max budget.
//  2. Each ID is resolved via lookup. found=false (any reason) →
//     drop + counter increment; found=true → append a contracts.Source
//     with Kind=SourceArtifact and a populated ArtifactRef.
//  3. Survivors are capped at sourcesMax. Any survivor beyond the cap
//     becomes overflow.
//  4. When sourcesMax <= 0 the function returns ([], 0) with no
//     lookups attempted — the caller is misconfigured and the
//     provenance gate will refuse downstream. This is a guardrail
//     against a typo in SST silently exposing zero sources for every
//     retrieval call.
//
// The function NEVER returns a non-nil error: every recoverable
// condition is encoded as a drop + counter so the response can still
// be assembled (possibly empty, possibly partial) and the gate can
// make the canonical refusal vs. accept decision downstream.
func AssembleSources(
	ctx context.Context,
	citedIDs []string,
	lookup ArtifactLookupFn,
	sourcesMax int,
) (sources []contracts.Source, sourcesOverflowCount int) {
	if sourcesMax <= 0 || lookup == nil || len(citedIDs) == 0 {
		return nil, 0
	}

	seen := make(map[string]struct{}, len(citedIDs))
	survivors := make([]contracts.Source, 0, len(citedIDs))

	for _, rawID := range citedIDs {
		id := rawID
		if id == "" {
			// An empty string is not a valid artifact ID; treat as
			// missing_artifact so the counter still records the
			// drift. This matches "ID that does not exist" in the
			// design.md §5.1 contract.
			assistantmetrics.SourceAssemblyDropsCounter.
				WithLabelValues(string(assistantmetrics.DropCauseMissingArtifact)).
				Inc()
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}

		title, capturedAt, found, err := lookup(ctx, id)
		switch {
		case err != nil && !errors.Is(err, ErrArtifactNotFound):
			assistantmetrics.SourceAssemblyDropsCounter.
				WithLabelValues(string(assistantmetrics.DropCauseLookupError)).
				Inc()
			continue
		case !found:
			assistantmetrics.SourceAssemblyDropsCounter.
				WithLabelValues(string(assistantmetrics.DropCauseMissingArtifact)).
				Inc()
			continue
		}

		survivors = append(survivors, contracts.Source{
			ID:    id,
			Title: title,
			Kind:  contracts.SourceArtifact,
			Ref: contracts.ArtifactRef{
				ArtifactID: id,
				CapturedAt: capturedAt,
			},
		})
	}

	if len(survivors) <= sourcesMax {
		return survivors, 0
	}
	return survivors[:sourcesMax], len(survivors) - sourcesMax
}

// ErrArtifactNotFound is a sentinel callbacks may return alongside
// found=false to signal "definitely not present" (vs. a transient
// lookup failure). The assembler treats both nil-error-not-found and
// ErrArtifactNotFound-not-found as the missing_artifact drop cause,
// so the sentinel is provided as a convenience for callers that want
// to distinguish in their own diagnostics without changing the
// counter cardinality.
var ErrArtifactNotFound = errors.New("artifact not found")
