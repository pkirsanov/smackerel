// BUG-096-001 — hermetic contract for the Ollama DISCOVERY probe base-URL seam.
//
// Regression proof for the self-hosted discovery-leg gap: the SCOPE-04 Ollama
// discovery adapter (GET <base>/api/tags) MUST resolve its base URL from the
// env-wired OLLAMA_URL seam (cfg.OllamaURL) — the SAME host-Ollama URL the
// /health probe and the ML-sidecar synthesis path use — and MUST NOT read the
// 096 connection registry's base_url param (a fixed dev-compose-DNS literal
// carried verbatim in the build-once bundle, un-re-pointed per target).
//
// These tests are pure (no DB, no network, no global singletons): they exercise
// ollamaDiscoveryBaseURL(cfg) directly.
package main

import (
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/config"
)

const (
	// composeDNS is the dev/bundle literal baked into the local-ollama
	// connection registry param. On the single-host self-hosted topology it
	// resolves to NXDOMAIN (no in-stack `ollama` compose service).
	composeDNS = "http://ollama:11434"
	// hostSeam is a representative self-hosted OLLAMA_URL value the deploy
	// adapter (knb) writes into the core's app.env when the local Ollama daemon
	// is a host singleton. Generic .invalid placeholder (RFC 2606) — the real
	// host address lives only in the knb adapter, never in this repo.
	hostSeam = "http://host-ollama.invalid:11434"
)

// ollamaOnlyConfig builds a config carrying an enabled local-ollama connection
// whose registry base_url param is the compose-DNS literal, plus a separately
// wired cfg.OllamaURL env seam. The adversarial intent: the two disagree, so a
// resolver that reads the registry param yields compose DNS while a resolver
// that reads the env seam yields the host URL.
func ollamaOnlyConfig(ollamaURL string) *config.Config {
	cfg := &config.Config{}
	cfg.OllamaURL = ollamaURL
	cfg.ModelConnections.Connections = []config.ModelConnection{
		{
			ID:      "local-ollama",
			Kind:    config.ModelConnectionKindOllama,
			Enabled: true,
			// Registry param is the compose-DNS literal — the value discovery
			// MUST NOT use on self-hosted.
			Params: map[string]any{"base_url": composeDNS},
		},
	}
	return cfg
}

// TestOllamaDiscoveryBaseURL_UsesEnvSeamNotRegistryParam is the core adversarial
// case: with the env seam pointing at the host daemon and the registry param
// still the compose-DNS literal, discovery MUST resolve the host seam. A fix
// regression that re-reads the connection param would return compose DNS here
// and fail this test.
func TestOllamaDiscoveryBaseURL_UsesEnvSeamNotRegistryParam(t *testing.T) {
	cfg := ollamaOnlyConfig(hostSeam)

	got, err := ollamaDiscoveryBaseURL(cfg)
	if err != nil {
		t.Fatalf("ollamaDiscoveryBaseURL returned err: %v", err)
	}
	if got != hostSeam {
		t.Fatalf("discovery base URL = %q, want the env-wired host seam %q", got, hostSeam)
	}
	if got == composeDNS {
		t.Fatalf("discovery base URL fell back to the compose-DNS registry param %q — self-hosted regression", composeDNS)
	}
}

// TestOllamaDiscoveryBaseURL_FailsLoudOnEmptySeam proves the no-default contract:
// an empty OLLAMA_URL is a NAMED error, NOT a silent compose-DNS substitution.
func TestOllamaDiscoveryBaseURL_FailsLoudOnEmptySeam(t *testing.T) {
	for _, empty := range []string{"", "   ", "\t\n"} {
		cfg := ollamaOnlyConfig(empty)
		got, err := ollamaDiscoveryBaseURL(cfg)
		if err == nil {
			t.Fatalf("empty OLLAMA_URL (%q) must fail loud, got base URL %q", empty, got)
		}
		if !strings.Contains(err.Error(), "OLLAMA_URL") {
			t.Fatalf("error must name OLLAMA_URL, got: %v", err)
		}
		if got != "" {
			t.Fatalf("on failure the base URL must be empty, got %q (no compose-DNS default)", got)
		}
		if strings.Contains(err.Error(), composeDNS) {
			t.Fatalf("error text must not carry a compose-DNS default (%q): %v", composeDNS, err)
		}
	}
}

// TestOllamaDiscoveryBaseURL_TrimsAndReturnsHostSeam is the happy path: a
// well-formed self-hosted seam (with incidental surrounding whitespace) is
// trimmed and returned verbatim.
func TestOllamaDiscoveryBaseURL_TrimsAndReturnsHostSeam(t *testing.T) {
	cfg := ollamaOnlyConfig("  " + hostSeam + "  ")
	got, err := ollamaDiscoveryBaseURL(cfg)
	if err != nil {
		t.Fatalf("ollamaDiscoveryBaseURL returned err: %v", err)
	}
	if got != hostSeam {
		t.Fatalf("discovery base URL = %q, want trimmed host seam %q", got, hostSeam)
	}
}
