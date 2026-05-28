// Package assistantmetrics owns the spec 061 capability-layer
// Prometheus counters that do NOT belong to the provenance gate
// (which lives in internal/assistant/provenance). Per-skill source
// assembly is the canonical example: it has to count graph-drift
// drops independently from the gate's refusal counter so dashboards
// can tell "the LLM cited an artifact that no longer exists" apart
// from "the response had zero sources at all".
//
// This package is import-pure: it registers its CounterVec(s) at
// package init time, and exposes them as exported variables so test
// code can sample values deterministically with the same
// dto.Metric.Counter.GetValue() pattern used by
// internal/assistant/provenance/gate_test.go.
//
// Naming convention: every counter name is prefixed
// `smackerel_assistant_` so /metrics scraping can pick the whole
// capability layer with one prefix.
package assistantmetrics

import "github.com/prometheus/client_golang/prometheus"

// SourceAssemblyDropsCounter records every cited artifact ID that
// the source-assembly invariant dropped instead of materializing
// into a contracts.Source.
//
// Label `cause` is a closed vocabulary describing why the assembler
// declined to include the ID:
//
//   - "missing_artifact" — the artifact row was not present in the
//     graph at the moment of assembly (graph drift; user deleted /
//     pruned / re-ingested between the search hit and the synthesis
//     turn). This is the spec 061 design §5.1 example case that
//     proves BS-007 fires on real graph drift.
//   - "lookup_error"     — the artifact lookup callback returned a
//     non-nil error other than "not found" (transient PG outage,
//     timeout, etc). The assembler MUST treat this like a drop
//     rather than crash the response so the gate downstream can
//     decide whether the remaining sources are enough.
//
// Cardinality is bounded (two label values forever) so high QPS
// retrieval traffic cannot explode the time-series count.
var SourceAssemblyDropsCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_assistant_source_assembly_drops_total",
		Help: "Number of cited artifact IDs the capability-layer source-assembly invariant dropped because the artifact was missing from the knowledge graph (cause=\"missing_artifact\") or the lookup callback errored (cause=\"lookup_error\"). When ALL cited IDs drop, the resulting empty Sources[] makes the provenance gate fire a canonical refusal (BS-007).",
	},
	[]string{"cause"},
)

// SourceAssemblyDropCause is the closed-vocabulary set of label
// values for SourceAssemblyDropsCounter. Callers MUST use these
// constants rather than literal strings so any future relabel stays
// caller-driven.
type SourceAssemblyDropCause string

const (
	DropCauseMissingArtifact SourceAssemblyDropCause = "missing_artifact"
	DropCauseLookupError     SourceAssemblyDropCause = "lookup_error"
)

func init() {
	prometheus.MustRegister(SourceAssemblyDropsCounter)
}
