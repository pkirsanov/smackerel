// Spec 080 + BUG-080-001 SCOPE-03 — Knowledge Graph activation, family
// read, and product read-synthetic observability metrics.
//
// Every metric here shares the `smackerel_graph_` prefix and a CLOSED,
// LOW-CARDINALITY label set drawn from design.md §"Observability And
// Failure Handling" → Metrics:
//
//	smackerel_graph_activation_total{mode,outcome,code}
//	smackerel_graph_read_requests_total{family,outcome}
//	smackerel_graph_read_duration_seconds{family,outcome}
//	smackerel_graph_synthetic_result{state}
//
// VALUE SAFETY (SCN-080-001-07): no label may EVER carry a node id, a
// topic/person/place label, a query value, a cursor body, a bearer
// token, secret material, a target host, or any other content. The
// permitted label values are exactly:
//
//	mode    — graphapi.ActivationState  ("enabled" | "disabled")
//	outcome — a closed graphsynthetic read/aggregate state
//	code    — a closed value-safe F080-* / "OK" diagnostic code
//	family  — a graphapi.GraphRouteFamily name from the canonical
//	          eight-family route manifest
//	state   — a closed graphsynthetic aggregate state
//
// Callers MUST source label values from those closed vocabularies;
// internal/graphsynthetic validates every result before emission so a
// content-bearing value structurally cannot reach a label.
package metrics

import "github.com/prometheus/client_golang/prometheus"

// GraphActivationTotal counts fail-soft graph activation resolutions.
// Incremented once per boot (and once per explicit re-resolution) with
// the resolved mode, the outcome class, and the closed value-safe
// activation diagnostic code. It never carries the cursor secret, its
// length, its hash, or any derivative.
var GraphActivationTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_graph_activation_total",
		Help: "Knowledge Graph fail-soft activation resolutions by mode, outcome, and closed value-safe code",
	},
	[]string{"mode", "outcome", "code"},
)

// GraphReadRequestsTotal counts product read-synthetic family reads by
// canonical family name and closed read outcome. It excludes ids,
// labels, query values, and cursor bodies.
var GraphReadRequestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_graph_read_requests_total",
		Help: "Knowledge Graph family reads by canonical family and closed read outcome",
	},
	[]string{"family", "outcome"},
)

// GraphReadDurationSeconds records bounded family read latency by
// canonical family and closed read outcome. Buckets are sized for a
// local authenticated HTTP read (5 ms .. 10 s).
var GraphReadDurationSeconds = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "smackerel_graph_read_duration_seconds",
		Help:    "Knowledge Graph family read latency by canonical family and closed read outcome",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	},
	[]string{"family", "outcome"},
)

// GraphSyntheticResult is the ONE-HOT gauge for the latest product
// read-synthetic aggregate observation: exactly one `state` series is
// 1 and every other declared state is 0. Readers therefore cannot
// mistake a stale series for the current truth.
var GraphSyntheticResult = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "smackerel_graph_synthetic_result",
		Help: "One-hot gauge for the latest Knowledge Graph read-synthetic aggregate state (1 = current state)",
	},
	[]string{"state"},
)

func init() {
	prometheus.MustRegister(
		GraphActivationTotal,
		GraphReadRequestsTotal,
		GraphReadDurationSeconds,
		GraphSyntheticResult,
	)
}
