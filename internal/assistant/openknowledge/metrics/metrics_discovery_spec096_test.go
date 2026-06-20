// Spec 096 §13 — provider DISCOVERY metrics unit tests.
//
// These pin the two new §13 discovery series (provider_discovery_total,
// provider_discovery_latency_seconds), their closed label vocabularies
// (provider = the dispatch set PLUS ollama; state = ok|timeout|unreachable),
// and the adversarial G021 cardinality guard. They mirror
// metrics_provider_spec096_test.go (prometheus.NewRegistry + testutil).
package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestProviderDiscoveryMetrics_NamesPinned_Spec096 pins the exact §13 discovery
// series names a dashboard / alert proposal will scrape. Renaming any is a
// coordinated cross-spec change.
func TestProviderDiscoveryMetrics_NamesPinned_Spec096(t *testing.T) {
	want := map[string]string{
		"providerDiscovery":        "openknowledge_provider_discovery_total",
		"providerDiscoveryLatency": "openknowledge_provider_discovery_latency_seconds",
	}
	got := map[string]string{
		"providerDiscovery":        NameProviderDiscovery,
		"providerDiscoveryLatency": NameProviderDiscoveryLatency,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("metric %s: got %q want %q", k, got[k], w)
		}
	}
}

// TestProviderDiscoveryVocab_Pinned_Spec096 pins the closed label vocabularies.
// The discovery-provider set is the five HOSTED kinds PLUS ollama (ollama IS
// discovered — unlike dispatch, where it is the spec 089 path and emits no
// provider metric); the state set is the three reachability outcomes. Drift here
// widens Prometheus cardinality.
func TestProviderDiscoveryVocab_Pinned_Spec096(t *testing.T) {
	wantProviders := map[string]bool{
		"ollama": true, "anthropic": true, "openai": true, "azure-foundry": true, "google": true, "bedrock": true,
	}
	if len(AllDiscoveryProviders) != len(wantProviders) {
		t.Fatalf("AllDiscoveryProviders = %v, want exactly the 6 discovery kinds (ollama + 5 hosted)", AllDiscoveryProviders)
	}
	sawOllama := false
	for _, p := range AllDiscoveryProviders {
		if !wantProviders[p] {
			t.Errorf("unexpected discovery provider %q (cardinality vocab drift)", p)
		}
		if p == "ollama" {
			sawOllama = true
		}
	}
	if !sawOllama {
		t.Errorf("ollama MUST be a discovery provider (its /api/tags probe is first-class discovery)")
	}

	wantStates := map[string]bool{"ok": true, "timeout": true, "unreachable": true}
	if len(AllDiscoveryStates) != len(wantStates) {
		t.Fatalf("AllDiscoveryStates = %v, want exactly [ok timeout unreachable]", AllDiscoveryStates)
	}
	for _, s := range AllDiscoveryStates {
		if !wantStates[s] {
			t.Errorf("unexpected discovery state %q (cardinality vocab drift)", s)
		}
	}
}

// TestProviderDiscoveryMetrics_RegisterAndScrape_Spec096 constructs Metrics
// against a fresh registry, drives the §13 discovery helpers, and reads the
// series to prove they materialise with the expected counts.
func TestProviderDiscoveryMetrics_RegisterAndScrape_Spec096(t *testing.T) {
	m := New([]string{"calculator"})
	reg := prometheus.NewRegistry()
	if err := m.Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	m.IncProviderDiscovery("ollama", DiscoveryStateOK)
	m.IncProviderDiscovery("anthropic", DiscoveryStateUnreachable)
	m.IncProviderDiscovery("openai", DiscoveryStateTimeout)
	m.ObserveProviderDiscoveryLatency("ollama", 0.012)
	m.ObserveProviderDiscoveryLatency("anthropic", 1.5)

	if got := testutil.ToFloat64(m.providerDiscovery.WithLabelValues("ollama", "ok")); got != 1 {
		t.Errorf("provider_discovery_total{ollama,ok} = %v want 1", got)
	}
	if got := testutil.ToFloat64(m.providerDiscovery.WithLabelValues("anthropic", "unreachable")); got != 1 {
		t.Errorf("provider_discovery_total{anthropic,unreachable} = %v want 1", got)
	}
	if got := testutil.ToFloat64(m.providerDiscovery.WithLabelValues("openai", "timeout")); got != 1 {
		t.Errorf("provider_discovery_total{openai,timeout} = %v want 1", got)
	}
	// One histogram series per observed provider (ollama, anthropic).
	if got := testutil.CollectAndCount(m.providerDiscoveryLatency); got != 2 {
		t.Errorf("provider_discovery_latency_seconds series = %d want 2 (ollama, anthropic)", got)
	}
}

// TestProviderDiscoveryMetrics_RejectsUnknownLabels_AdversarialG021_Spec096
// proves a buggy mapping (or adversarial caller) cannot inflate Prometheus
// cardinality through the §13 discovery helpers: a non-discovery provider, an
// injection string, a rogue state, the real-but-out-of-vocab "auth_failed"
// state, and control bytes are ALL silently dropped — no panic, no new series.
func TestProviderDiscoveryMetrics_RejectsUnknownLabels_AdversarialG021_Spec096(t *testing.T) {
	m := New([]string{"calculator"})
	reg := prometheus.NewRegistry()
	if err := m.Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Baseline: one legitimate series on each metric.
	m.IncProviderDiscovery("ollama", DiscoveryStateOK)
	m.ObserveProviderDiscoveryLatency("ollama", 0.01)

	for i := 0; i < 1000; i++ {
		m.IncProviderDiscovery("not_a_provider", "ok")  // rogue provider
		m.IncProviderDiscovery("ollama", "rogue_state") // rogue state
		m.IncProviderDiscovery("ollama", "auth_failed") // a real DiscoveryState, NOT in the reachability vocab
		m.IncProviderDiscovery("ollama", "disabled")    // ditto
		m.IncProviderDiscovery("'; DROP TABLE discovery; --", "ok")
		m.IncProviderDiscovery("ollama", string([]byte{0x00, 0x01, 0x02}))
		m.ObserveProviderDiscoveryLatency("rogue_provider", 1.0)
	}

	if got := testutil.CollectAndCount(m.providerDiscovery); got != 1 {
		t.Errorf("provider_discovery series = %d want 1 (adversarial provider/state labels leaked cardinality)", got)
	}
	if got := testutil.CollectAndCount(m.providerDiscoveryLatency); got != 1 {
		t.Errorf("provider_discovery_latency series = %d want 1 (rogue provider was observed)", got)
	}
}
