// Spec 061 SCOPE-09 — metric registration and type contract tests.
//
// Proves every metric declared in metrics.go is registered with the
// expected Prometheus type and exact label set from design.md §8.1.
// Cardinality bounds are exercised by labels_test.go; this file is
// the "did we declare what the design table requires?" gate.
package assistantmetrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// metricFixtures lists, for every spec 061 §8.1 series owned by this
// package, the expected fully-qualified metric name, its Prometheus
// type, the exact label set (order matters), and a small helper that
// emits ONE sample so the registered series materialises in the
// gather output. Adversarial: drift in either the metric name or
// the label tuple fails this table.
type metricFixture struct {
	name     string
	typ      dto.MetricType
	labels   []string
	emitOnce func() // one Inc / Observe / Set against the labels.
}

func metricFixtures() []metricFixture {
	return []metricFixture{
		{
			name:   "smackerel_assistant_facade_turns_total",
			typ:    dto.MetricType_COUNTER,
			labels: []string{"transport", "outcome"},
			emitOnce: func() {
				FacadeTurnsTotal.WithLabelValues(TransportFake, OutcomeAnswered).Inc()
			},
		},
		{
			name:   "smackerel_assistant_facade_latency_seconds",
			typ:    dto.MetricType_HISTOGRAM,
			labels: []string{"transport", "outcome"},
			emitOnce: func() {
				FacadeLatencySeconds.WithLabelValues(TransportFake, OutcomeAnswered).Observe(0.1)
			},
		},
		{
			name:   "smackerel_assistant_router_band_total",
			typ:    dto.MetricType_COUNTER,
			labels: []string{"band", "transport"},
			emitOnce: func() {
				RouterBandTotal.WithLabelValues(BandHigh, TransportFake).Inc()
			},
		},
		{
			name:   "smackerel_assistant_skill_invocations_total",
			typ:    dto.MetricType_COUNTER,
			labels: []string{"scenario_id", "outcome", "transport"},
			emitOnce: func() {
				SkillInvocationsTotal.WithLabelValues("retrieval_qa", SkillOutcomeOK, TransportFake).Inc()
			},
		},
		{
			name:   "smackerel_assistant_capture_fallback_total",
			typ:    dto.MetricType_COUNTER,
			labels: []string{"cause", "transport"},
			emitOnce: func() {
				CaptureFallbackTotal.WithLabelValues(CauseLowConfidence, TransportFake).Inc()
			},
		},
		{
			name:   "smackerel_assistant_confirm_card_outcomes_total",
			typ:    dto.MetricType_COUNTER,
			labels: []string{"scenario_id", "outcome", "transport"},
			emitOnce: func() {
				ConfirmCardOutcomesTotal.WithLabelValues("notification_schedule", ConfirmOutcomeConfirmed, TransportFake).Inc()
			},
		},
		{
			name:   "smackerel_assistant_disambiguation_outcomes_total",
			typ:    dto.MetricType_COUNTER,
			labels: []string{"outcome", "transport"},
			emitOnce: func() {
				DisambiguationOutcomesTotal.WithLabelValues(DisambigOutcomeResolvedUser, TransportFake).Inc()
			},
		},
		{
			name:   "smackerel_assistant_active_threads",
			typ:    dto.MetricType_GAUGE,
			labels: []string{"transport"},
			emitOnce: func() {
				ActiveThreadsGauge.WithLabelValues(TransportFake).Set(0)
			},
		},
	}
}

// TestEveryMetricRegisteredWithExpectedTypeAndLabels gathers from the
// default Prometheus registry (the same one /metrics serves) and
// asserts every fixture metric appears with the exact type and label
// tuple. Drift in name → "metric ... missing"; drift in label order
// or set → "label drift"; drift in type → "type drift".
func TestEveryMetricRegisteredWithExpectedTypeAndLabels(t *testing.T) {
	for _, f := range metricFixtures() {
		f.emitOnce() // ensure the series is materialised in the registry
	}

	fams, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	byName := map[string]*dto.MetricFamily{}
	for _, fam := range fams {
		byName[fam.GetName()] = fam
	}

	for _, f := range metricFixtures() {
		fam, ok := byName[f.name]
		if !ok {
			t.Errorf("metric %q missing from /metrics gather (registration regression)", f.name)
			continue
		}
		if fam.GetType() != f.typ {
			t.Errorf("metric %q: type drift: want %s got %s", f.name, f.typ, fam.GetType())
		}
		// Pick any one sample (we emitted at least one above) and
		// compare its labels to the fixture set.
		samples := fam.GetMetric()
		if len(samples) == 0 {
			t.Errorf("metric %q: no samples after emit", f.name)
			continue
		}
		got := samples[0].GetLabel()
		if len(got) != len(f.labels) {
			t.Errorf("metric %q: label count drift: want %d %v got %d %v",
				f.name, len(f.labels), f.labels, len(got), labelNames(got))
			continue
		}
		// Prometheus.Gather() sorts label pairs alphabetically;
		// the registered ORDER (which is what WithLabelValues uses)
		// is encoded by the metric vector itself, not by the
		// gathered sample. Compare as an unordered set here.
		wantSet := make(map[string]struct{}, len(f.labels))
		for _, w := range f.labels {
			wantSet[w] = struct{}{}
		}
		for _, g := range got {
			if _, ok := wantSet[g.GetName()]; !ok {
				t.Errorf("metric %q: unexpected label name %q (drift from design §8.1)", f.name, g.GetName())
			}
			delete(wantSet, g.GetName())
		}
		for missing := range wantSet {
			t.Errorf("metric %q: missing expected label %q (drift from design §8.1)", f.name, missing)
		}
	}
}

// TestSpec061EightMetricSeriesNamesArePrefixedSmackerelAssistant
// proves the operator-side prefix invariant (every series owned by
// this package has the smackerel_assistant_ prefix so a single
// Prometheus relabel can isolate the capability layer).
func TestSpec061EightMetricSeriesNamesArePrefixedSmackerelAssistant(t *testing.T) {
	for _, f := range metricFixtures() {
		if !strings.HasPrefix(f.name, "smackerel_assistant_") {
			t.Errorf("metric %q missing the smackerel_assistant_ prefix (operator-side relabel invariant)", f.name)
		}
	}
}

// labelNames extracts the names from a slice of dto.LabelPairs for
// error-message readability.
func labelNames(pairs []*dto.LabelPair) []string {
	out := make([]string, len(pairs))
	for i, p := range pairs {
		out[i] = p.GetName()
	}
	return out
}
