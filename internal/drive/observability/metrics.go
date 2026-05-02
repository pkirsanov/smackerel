// Package observability hosts the spec 038 Scope 8 provider-neutral
// drive metrics + structured-log helpers. The package is intentionally
// thin so the scan/extract/save/retrieve services can emit one well-
// labeled signal per outcome without each service having to import the
// global Prometheus registry directly.
//
// Cardinality contract: every label MUST be drawn from a small bounded
// enum. The provider label is a stable provider ID ("google",
// "memdrive"), the outcome label is a controlled vocabulary defined
// here (`OutcomeOK`, `OutcomeSkipped`, `OutcomeBlocked`,
// `OutcomeError`, ...). Free-form values such as connection IDs or
// file IDs MUST NOT appear as label values.
package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/metrics"
)

// Outcome describes the bounded outcome label used by the drive
// observability counters. Adding a new outcome requires updating the
// dashboards/alerts that depend on the metric, so the set is closed.
type Outcome string

const (
	// OutcomeOK records a successful operation (file indexed, content
	// extracted, save written, retrieve delivered).
	OutcomeOK Outcome = "ok"
	// OutcomeSkipped records a deliberate, recoverable skip (file too
	// large, MIME unsupported, classification below confidence floor).
	OutcomeSkipped Outcome = "skipped"
	// OutcomeBlocked records a policy or credential block (sensitivity
	// downgrade, missing scope, refused by save rule).
	OutcomeBlocked Outcome = "blocked"
	// OutcomeError records a provider-side or runtime error that left
	// the operation in a retryable state.
	OutcomeError Outcome = "error"
	// OutcomeRefused records a hard refusal from the policy engine that
	// MUST NOT be retried automatically.
	OutcomeRefused Outcome = "refused"
)

// DriveScanFiles counts files indexed by the scan/monitor service.
//
// Labels:
//
//	provider — drive provider ID (e.g. "google", "memdrive")
//	outcome  — Outcome label
var DriveScanFiles = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_drive_scan_files_total",
		Help: "Drive files observed by the scan/monitor pipeline by provider and outcome",
	},
	[]string{"provider", "outcome"},
)

// DriveExtractFiles counts files passed through the extraction +
// classification pipeline.
//
// Labels:
//
//	provider — drive provider ID
//	outcome  — Outcome label (ok/skipped/blocked/error)
var DriveExtractFiles = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_drive_extract_files_total",
		Help: "Drive files processed by extraction/classification by provider and outcome",
	},
	[]string{"provider", "outcome"},
)

// DriveSaveAttempts counts save-back outcomes from the save service.
//
// Labels:
//
//	provider — drive provider ID receiving the file
//	outcome  — Outcome label (ok/skipped/blocked/refused/error)
var DriveSaveAttempts = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_drive_save_attempts_total",
		Help: "Drive save-back attempts by provider and outcome",
	},
	[]string{"provider", "outcome"},
)

// DriveRetrieveDecisions counts retrieve decisions emitted by the
// retrieval service. The mode label mirrors the public retrieve.Mode
// enum (bytes/secure_link/provider_link/refused/disambiguate).
//
// Labels:
//
//	provider — drive provider ID
//	mode     — retrieve.Mode value
var DriveRetrieveDecisions = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_drive_retrieve_decisions_total",
		Help: "Drive retrieve decisions by provider and delivery mode",
	},
	[]string{"provider", "mode"},
)

// DriveProviderErrors counts provider-side error events surfaced by the
// health tracker. work_type matches the controlled vocabulary used by
// drive_provider_work_queue (scan/save/retrieve).
//
// Labels:
//
//	provider  — drive provider ID
//	work_type — controlled vocabulary (scan/save/retrieve)
var DriveProviderErrors = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_drive_provider_errors_total",
		Help: "Drive provider error events by provider and work type",
	},
	[]string{"provider", "work_type"},
)

// metricsRegistered is set to true once registerWithDefaultRegistry has
// run so repeated package-level inits (which happen when both the
// production binary AND tests link the package) do not double-register.
var metricsRegistered bool

func init() {
	registerWithDefaultRegistry()
}

func registerWithDefaultRegistry() {
	if metricsRegistered {
		return
	}
	metricsRegistered = true
	prometheus.MustRegister(
		DriveScanFiles,
		DriveExtractFiles,
		DriveSaveAttempts,
		DriveRetrieveDecisions,
		DriveProviderErrors,
	)
	preInitLabelFamilies()
}

// preInitLabelFamilies emits a zero-valued sample for every known
// (provider, outcome|mode|work_type) combination so the /metrics scrape
// surfaces the HELP/TYPE/sample lines for these counters BEFORE the
// first scan/extract/save/retrieve fires.
//
// Without this pre-init, prometheus client_golang would suppress the
// metric family entirely until the first WithLabelValues call —
// dashboards, alerts, and the SCN-038-024 observability e2e check all
// need to see the metric exists to confirm the binary registered it.
//
// Provider IDs are a small bounded enum (currently "google" and
// "memdrive"). Adding a provider requires adding it here so the metric
// family stays observable from container start.
func preInitLabelFamilies() {
	providers := []string{"google", "memdrive"}
	scanOutcomes := []string{string(OutcomeOK), string(OutcomeSkipped), string(OutcomeBlocked), string(OutcomeError)}
	extractOutcomes := scanOutcomes
	saveOutcomes := []string{string(OutcomeOK), string(OutcomeSkipped), string(OutcomeBlocked), string(OutcomeRefused), string(OutcomeError)}
	retrieveModes := []string{"bytes", "secure_link", "provider_link", "refused", "disambiguate"}
	workTypes := []string{"scan", "save", "retrieve"}
	for _, provider := range providers {
		for _, outcome := range scanOutcomes {
			DriveScanFiles.WithLabelValues(provider, outcome)
		}
		for _, outcome := range extractOutcomes {
			DriveExtractFiles.WithLabelValues(provider, outcome)
		}
		for _, outcome := range saveOutcomes {
			DriveSaveAttempts.WithLabelValues(provider, outcome)
		}
		for _, mode := range retrieveModes {
			DriveRetrieveDecisions.WithLabelValues(provider, mode)
		}
		for _, work := range workTypes {
			DriveProviderErrors.WithLabelValues(provider, work)
		}
	}
}

// CounterValue returns the current value of a single (provider, outcome)
// pair on the supplied CounterVec. Tests use this helper to assert that
// the expected metric was incremented without depending on a Prometheus
// HTTP scrape. Returns 0 when the labels have not been emitted yet.
func CounterValue(vec *prometheus.CounterVec, labels ...string) float64 {
	counter, err := vec.GetMetricWithLabelValues(labels...)
	if err != nil {
		return 0
	}
	return testutil.ToFloat64(counter)
}

// HandlerForTests returns the global Prometheus handler so tests can
// assert metric output through a real scrape. It is exported through
// `metrics.Handler` for production wiring; this re-export keeps the
// scope-8 test plan from importing internal/metrics directly when the
// only need is to scrape drive metrics.
var HandlerForTests = metrics.Handler
