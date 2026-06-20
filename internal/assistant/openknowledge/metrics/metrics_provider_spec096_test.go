// Spec 096 §13 — provider-aware (cost-bearing) dispatch metrics unit tests.
//
// These pin the four new §13 series (provider_dispatch_total,
// provider_dispatch_tokens, provider_dispatch_usd_cents,
// vault_decrypt_failures_total), their closed label vocabularies, and the
// adversarial G021 cardinality guard. They mirror metrics_test.go
// (prometheus.NewRegistry + testutil.ToFloat64 / CollectAndCount).
package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestProviderDispatchMetrics_NamesPinned_Spec096 pins the exact §13 series
// names a dashboard / alert proposal will scrape. Renaming any is a coordinated
// cross-spec change.
func TestProviderDispatchMetrics_NamesPinned_Spec096(t *testing.T) {
	want := map[string]string{
		"providerDispatch":       "openknowledge_provider_dispatch_total",
		"providerDispatchTokens": "openknowledge_provider_dispatch_tokens",
		"providerDispatchUSD":    "openknowledge_provider_dispatch_usd_cents",
		"vaultDecryptFailures":   "openknowledge_vault_decrypt_failures_total",
	}
	got := map[string]string{
		"providerDispatch":       NameProviderDispatch,
		"providerDispatchTokens": NameProviderDispatchTokens,
		"providerDispatchUSD":    NameProviderDispatchUSDCents,
		"vaultDecryptFailures":   NameVaultDecryptFailures,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("metric %s: got %q want %q", k, got[k], w)
		}
	}
}

// TestProviderDispatchVocab_Pinned_Spec096 pins the closed label vocabularies.
// The dispatch-provider set is the five HOSTED kinds (ollama is the 089 path and
// MUST NOT be a member); the vault-reason set is the three credential/vault-class
// reasons. Drift here widens Prometheus cardinality.
func TestProviderDispatchVocab_Pinned_Spec096(t *testing.T) {
	wantProviders := map[string]bool{
		"anthropic": true, "openai": true, "azure-foundry": true, "google": true, "bedrock": true,
	}
	if len(AllDispatchProviders) != len(wantProviders) {
		t.Fatalf("AllDispatchProviders = %v, want exactly the 5 hosted kinds", AllDispatchProviders)
	}
	for _, p := range AllDispatchProviders {
		if !wantProviders[p] {
			t.Errorf("unexpected dispatch provider %q (cardinality vocab drift)", p)
		}
		if p == "ollama" {
			t.Errorf("ollama MUST NOT be a hosted dispatch provider (it is the spec 089 path)")
		}
	}

	wantReasons := map[string]bool{
		"credential_missing": true, "vault_not_configured": true, "decrypt_failed": true,
	}
	if len(AllVaultDecryptReasons) != len(wantReasons) {
		t.Fatalf("AllVaultDecryptReasons = %v, want exactly the 3 vault-class reasons", AllVaultDecryptReasons)
	}
	for _, rsn := range AllVaultDecryptReasons {
		if !wantReasons[rsn] {
			t.Errorf("unexpected vault-decrypt reason %q (cardinality vocab drift)", rsn)
		}
	}
}

// TestProviderDispatchMetrics_RegisterAndScrape_Spec096 constructs Metrics
// against a fresh registry, drives the §13 helpers, and reads the series values
// to prove they materialise with the expected counts.
func TestProviderDispatchMetrics_RegisterAndScrape_Spec096(t *testing.T) {
	m := New([]string{"calculator"})
	reg := prometheus.NewRegistry()
	if err := m.Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	m.IncProviderDispatch("anthropic")
	m.ObserveProviderDispatch("anthropic", 1200, 7.5)
	m.IncProviderDispatch("openai")
	m.ObserveProviderDispatch("openai", 800, 3.0)
	m.IncVaultDecryptFailure("decrypt_failed")
	m.IncVaultDecryptFailure("credential_missing")

	if got := testutil.ToFloat64(m.providerDispatch.WithLabelValues("anthropic")); got != 1 {
		t.Errorf("provider_dispatch_total{anthropic} = %v want 1", got)
	}
	if got := testutil.ToFloat64(m.providerDispatch.WithLabelValues("openai")); got != 1 {
		t.Errorf("provider_dispatch_total{openai} = %v want 1", got)
	}
	// One histogram series per observed provider on each of the token + USD
	// histograms (anthropic, openai).
	if got := testutil.CollectAndCount(m.providerDispatchTokens); got != 2 {
		t.Errorf("provider_dispatch_tokens series = %d want 2 (anthropic, openai)", got)
	}
	if got := testutil.CollectAndCount(m.providerDispatchUSD); got != 2 {
		t.Errorf("provider_dispatch_usd_cents series = %d want 2 (anthropic, openai)", got)
	}
	if got := testutil.ToFloat64(m.vaultDecryptFailures.WithLabelValues("decrypt_failed")); got != 1 {
		t.Errorf("vault_decrypt_failures_total{decrypt_failed} = %v want 1", got)
	}
	if got := testutil.ToFloat64(m.vaultDecryptFailures.WithLabelValues("credential_missing")); got != 1 {
		t.Errorf("vault_decrypt_failures_total{credential_missing} = %v want 1", got)
	}
}

// TestProviderDispatchMetrics_RejectsUnknownLabels_AdversarialG021_Spec096
// proves a buggy mapping (or an adversarial caller) cannot inflate Prometheus
// cardinality through the §13 helpers: a non-hosted provider (including the
// literal "ollama"), an injection string, a rogue vault reason, and control
// bytes are ALL silently dropped — no panic, no new series.
func TestProviderDispatchMetrics_RejectsUnknownLabels_AdversarialG021_Spec096(t *testing.T) {
	m := New([]string{"calculator"})
	reg := prometheus.NewRegistry()
	if err := m.Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Baseline: one legitimate series on each counter.
	m.IncProviderDispatch("anthropic")
	m.IncVaultDecryptFailure("decrypt_failed")

	for i := 0; i < 1000; i++ {
		m.IncProviderDispatch("not_a_provider")
		m.IncProviderDispatch("ollama") // the 089 path is NOT a hosted dispatch provider
		m.IncProviderDispatch("'; DROP TABLE dispatches; --")
		m.ObserveProviderDispatch("rogue_provider", 100, 1.0)
		m.IncVaultDecryptFailure("not_a_reason")
		m.IncVaultDecryptFailure("connection_not_found") // a real reject reason, but NOT a vault failure
		m.IncVaultDecryptFailure(string([]byte{0x00, 0x01, 0x02}))
	}

	if got := testutil.CollectAndCount(m.providerDispatch); got != 1 {
		t.Errorf("provider_dispatch series = %d want 1 (adversarial provider labels leaked cardinality)", got)
	}
	if got := testutil.CollectAndCount(m.providerDispatchTokens); got != 0 {
		t.Errorf("provider_dispatch_tokens series = %d want 0 (rogue provider was observed)", got)
	}
	if got := testutil.CollectAndCount(m.vaultDecryptFailures); got != 1 {
		t.Errorf("vault_decrypt_failures series = %d want 1 (adversarial reason labels leaked cardinality)", got)
	}
}
