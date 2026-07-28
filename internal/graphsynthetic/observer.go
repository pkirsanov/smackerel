package graphsynthetic

// observer.go — the GENERIC observability adapter seam.
//
// The synthetic runner publishes results through this interface and
// knows nothing about Prometheus, OpenTelemetry, slog, or any concrete
// backend. A deployment substitutes its own adapter without touching
// the read logic, and a test substitutes a recording adapter without a
// live telemetry stack.
//
// The seam is VALUE-SAFE BY TYPE: every method receives an already
// closed, already validated struct (Activation, GraphFamilyResult,
// AggregateResult). There is no method that accepts a free-form string,
// a response body, a request URL, or a credential, so an adapter
// structurally cannot be handed content to emit.
//
// The runner calls Validate before every publication, so an adapter
// never observes a result carrying a value outside the closed
// vocabularies.

import "github.com/smackerel/smackerel/internal/api/graphapi"

// Observer receives value-safe synthetic observations.
type Observer interface {
	// ObserveActivation reports the resolved fail-soft activation that
	// governs a run. Activation carries only the state, the secret
	// PRESENCE CLASS, and a closed code — never the secret itself.
	ObserveActivation(activation graphapi.Activation)
	// ObserveFamilyRead reports one validated canonical family result.
	ObserveFamilyRead(result GraphFamilyResult)
	// ObserveAggregate reports the validated aggregate observation.
	ObserveAggregate(result AggregateResult)
}

// NopObserver discards every observation. It is the explicit choice for
// a deployment that runs the synthetic without telemetry; it is not a
// fallback the runner selects on its own.
type NopObserver struct{}

// ObserveActivation implements Observer.
func (NopObserver) ObserveActivation(graphapi.Activation) {}

// ObserveFamilyRead implements Observer.
func (NopObserver) ObserveFamilyRead(GraphFamilyResult) {}

// ObserveAggregate implements Observer.
func (NopObserver) ObserveAggregate(AggregateResult) {}

// MultiObserver fans one observation out to several adapters in order.
// It lets a deployment combine, for example, the metrics/trace/log
// adapter with a result-store adapter without either knowing about the
// other.
type MultiObserver []Observer

// ObserveActivation implements Observer.
func (m MultiObserver) ObserveActivation(activation graphapi.Activation) {
	for _, o := range m {
		if o != nil {
			o.ObserveActivation(activation)
		}
	}
}

// ObserveFamilyRead implements Observer.
func (m MultiObserver) ObserveFamilyRead(result GraphFamilyResult) {
	for _, o := range m {
		if o != nil {
			o.ObserveFamilyRead(result)
		}
	}
}

// ObserveAggregate implements Observer.
func (m MultiObserver) ObserveAggregate(result AggregateResult) {
	for _, o := range m {
		if o != nil {
			o.ObserveAggregate(result)
		}
	}
}
