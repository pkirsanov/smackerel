// BUG-022-003 — Uniform 429/Retry-After handling across HTTP connectors.
//
// connector_429_total tracks every 429 response observed by the shared
// connector.DoWithRetry helper. The outcome label is bounded to
// {retry, recovered, exhausted} so operators can alert on per-connector
// rate-limit pressure rather than chasing opaque "HTTP 429" log lines.
package metrics

import "github.com/prometheus/client_golang/prometheus"

// ConnectorRateLimit429Total counts 429 responses by connector and outcome.
// Outcome values: "retry" (per retry attempt scheduled), "recovered"
// (eventual success after one or more 429s), "exhausted" (max attempts hit).
var ConnectorRateLimit429Total = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_connector_429_total",
		Help: "HTTP 429 events observed by the shared connector retry helper, by connector and outcome.",
	},
	[]string{"connector", "outcome"},
)

func init() {
	prometheus.MustRegister(ConnectorRateLimit429Total)
}
