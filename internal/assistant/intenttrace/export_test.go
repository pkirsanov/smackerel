package intenttrace

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestIntentTraceMetrics_FamilyRegisteredBeforeFirstTurn(t *testing.T) {
	reg := prometheus.NewRegistry()
	if err := reg.Register(IntentTracesTotal); err != nil {
		t.Fatalf("Register IntentTracesTotal: %v", err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "smackerel_assistant_intent_traces_total" {
			continue
		}
		if len(family.Metric) == 0 {
			t.Fatal("intent trace family has no initialized metric series")
		}
		metric := family.Metric[0]
		labels := map[string]string{}
		for _, pair := range metric.Label {
			labels[pair.GetName()] = pair.GetValue()
		}
		if labels["transport"] != string(TransportWeb) ||
			labels["sampled"] != "true" ||
			labels["action_class"] != "refuse" ||
			labels["final_response_status"] != string(StatusRefused) {
			t.Fatalf("initialized intent trace labels=%v", labels)
		}
		if metric.GetCounter().GetValue() != 0 {
			t.Fatalf("initialized intent trace counter=%v, want zero", metric.GetCounter().GetValue())
		}
		return
	}
	t.Fatal("fresh registry is missing smackerel_assistant_intent_traces_total")
}
