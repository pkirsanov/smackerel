package metrics

import "github.com/prometheus/client_golang/prometheus"

// BUG-004-004 SCOPE-04 — synthesis health as a metric.
//
// Health was previously visible only as a field inside an /api/health JSON
// response, which nothing scrapes and no rule can fire on. An operator learned
// that synthesis had stopped producing only by looking. This gauge is what
// makes stale-and-failed states alertable.
//
// The value encoding is deliberate. A single gauge with a `state` LABEL would
// let two states be non-zero at once during a scrape race, and an alert on
// `state="failed"` could then fire while `state="up"` was also set. One numeric
// gauge cannot be in two states at the same time, so exclusivity is structural
// rather than something the rule has to assume.
const (
	// SynthesisStateNeverRun — nothing has ever run. Distinct from failed: a
	// fresh install is not broken, but it is also not healthy.
	SynthesisStateNeverRun = 0
	// SynthesisStateUp — a verified output exists and is inside its freshness
	// budget.
	SynthesisStateUp = 1
	// SynthesisStateStale — a verified output exists but has aged past the
	// budget.
	SynthesisStateStale = 2
	// SynthesisStatePartial — the latest output deliberately omitted an optional
	// source class. Durable and honest, but never full health.
	SynthesisStatePartial = 3
	// SynthesisStateFailed — the latest attempt failed. An older verified output
	// may still exist; it does not clear this.
	SynthesisStateFailed = 4
	// SynthesisStateProbeError — the durable state could not be read. Not
	// healthy, and deliberately distinct from failed: the run may have been fine
	// and the database unreachable.
	SynthesisStateProbeError = 5
)

// SynthesisState is the exclusive numeric state of the latest synthesis run.
// See the constants above for the encoding.
var SynthesisState = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "smackerel_synthesis_state",
		Help: "Exclusive synthesis state: 0 never-run, 1 up, 2 stale, 3 partial, 4 failed, 5 probe-error.",
	},
)

// SynthesisLastVerifiedOutputUnixtime is the timestamp of the newest read-back
// verified output, or 0 when none exists.
//
// Zero is the strongest stale signal rather than a gap: `time() - 0` is
// trivially larger than any window, so a host that has never produced anything
// alerts instead of looking quiet.
var SynthesisLastVerifiedOutputUnixtime = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "smackerel_synthesis_last_verified_output_unixtime",
		Help: "Unix timestamp (seconds) of the newest read-back-verified synthesis output; 0 when none exists.",
	},
)
