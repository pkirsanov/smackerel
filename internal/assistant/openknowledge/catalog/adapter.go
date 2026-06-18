// Spec 096 SCOPE-04 — per-kind discovery adapters (design §3 foundation
// adapter contract `Discover`, §4 concrete implementations).
//
// Each adapter answers one question: "which models does this connection
// offer?" The Ollama adapter probes the local daemon live (`GET /api/tags`);
// hosted adapters serve the SST-curated `models[]` from the SCOPE-01 registry
// (the curated list IS the source of truth — no live call is needed to build
// the catalog, and capabilities ride the registry descriptor). An adapter
// failure is a TYPED *DiscoveryError so the aggregator records a typed
// ProviderDiscoveryStatus instead of silently dropping the provider.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/smackerel/smackerel/internal/config"
)

// DiscoveryAdapter is the per-kind discovery contract. The aggregator runs each
// adapter in parallel, bounded by the SST per-provider timeout, and converts a
// returned error into a typed ProviderDiscoveryStatus (never a silent drop).
type DiscoveryAdapter interface {
	// ConnectionID is the operator-global connection this adapter discovers.
	ConnectionID() string
	// Kind is the provider kind ("ollama", "anthropic", …).
	Kind() string
	// Discover returns the provider-qualified models this connection offers, or
	// a typed *DiscoveryError. Ollama probes `<base_url>/api/tags` live; hosted
	// adapters return the SST-curated list.
	Discover(ctx context.Context) ([]ModelDescriptor, error)
}

// HTTPDoer is the minimal HTTP surface the Ollama adapter needs. Injected so
// unit tests drive the live `/api/tags` parse with a fake client (no live
// daemon); production wiring passes an *http.Client.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// OllamaAdapter discovers installed models from a live Ollama daemon via
// `GET <base_url>/api/tags`. Installed models are provider-qualified as
// `ollama/<name>` and are all free/local ($0, no key). Because `/api/tags`
// does not report capabilities, an optional operator capability hint (keyed by
// bare name) stamps tool_capable / vision / context_window; absent a hint the
// capability triplet is the zero value.
type OllamaAdapter struct {
	connID  string
	baseURL string
	client  HTTPDoer
	caps    map[string]ModelCapabilities
}

// NewOllamaAdapter builds the live Ollama discovery adapter. caps may be nil.
func NewOllamaAdapter(connID, baseURL string, client HTTPDoer, caps map[string]ModelCapabilities) *OllamaAdapter {
	return &OllamaAdapter{connID: connID, baseURL: baseURL, client: client, caps: caps}
}

// ConnectionID implements DiscoveryAdapter.
func (a *OllamaAdapter) ConnectionID() string { return a.connID }

// Kind implements DiscoveryAdapter.
func (a *OllamaAdapter) Kind() string { return config.ModelConnectionKindOllama }

// ollamaTagsResponse is the subset of the `/api/tags` payload we consume — the
// installed model names. (Same endpoint shape the e2e Ollama health probe
// reads.)
type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// Discover probes `<base_url>/api/tags` and maps each installed model name to a
// provider-qualified `ollama/<name>` descriptor. A connect-class failure, a
// non-200 response, or a decode failure is a typed *DiscoveryError
// (StateUnreachable); a context-deadline failure is StateTimeout. The error
// text NEVER carries a secret (Ollama is keyless).
func (a *OllamaAdapter) Discover(ctx context.Context) ([]ModelDescriptor, error) {
	probeURL := strings.TrimRight(a.baseURL, "/") + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return nil, &DiscoveryError{State: StateUnreachable, Detail: fmt.Sprintf("ollama: build /api/tags request for %q failed", a.baseURL)}
	}
	resp, err := a.client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &DiscoveryError{State: StateTimeout, Detail: fmt.Sprintf("ollama: /api/tags timed out at %s", probeURL)}
		}
		return nil, &DiscoveryError{State: StateUnreachable, Detail: fmt.Sprintf("ollama unreachable at %s", probeURL)}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, &DiscoveryError{State: StateUnreachable, Detail: fmt.Sprintf("ollama responded HTTP %d at %s", resp.StatusCode, probeURL)}
	}
	var tags ollamaTagsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tags); err != nil {
		return nil, &DiscoveryError{State: StateUnreachable, Detail: fmt.Sprintf("ollama: decode /api/tags response from %s failed", probeURL)}
	}
	out := make([]ModelDescriptor, 0, len(tags.Models))
	for _, m := range tags.Models {
		name := strings.TrimSpace(m.Name)
		if name == "" {
			continue
		}
		hint := a.caps[name] // zero triplet when no operator capability hint
		out = append(out, ModelDescriptor{
			ID:            config.ModelConnectionKindOllama + "/" + name,
			ConnectionID:  a.connID,
			Kind:          config.ModelConnectionKindOllama,
			ToolCapable:   hint.ToolCapable,
			Vision:        hint.Vision,
			ContextWindow: hint.ContextWindow,
		})
	}
	return out, nil
}

// HostedAdapter serves a hosted connection's SST-curated `models[]` as the
// catalog source (design §4: hosted discovery strategy = curated). No live call
// is needed — the curated list IS the source of truth — and the capabilities
// ride the registry descriptor. Ids are provider-qualified `<kind>/<backend>`.
type HostedAdapter struct {
	connID string
	kind   string
	models []config.ModelDescriptor
}

// NewHostedAdapter builds a curated hosted adapter from a registry connection.
func NewHostedAdapter(conn config.ModelConnection) *HostedAdapter {
	return &HostedAdapter{connID: conn.ID, kind: conn.Kind, models: conn.Models.List}
}

// ConnectionID implements DiscoveryAdapter.
func (a *HostedAdapter) ConnectionID() string { return a.connID }

// Kind implements DiscoveryAdapter.
func (a *HostedAdapter) Kind() string { return a.kind }

// Discover returns the curated models, provider-qualified as `<kind>/<backend>`
// with the registry capabilities carried through. It never errors — the curated
// list is in-memory SST — so a hosted connection's catalog contribution is
// deterministic.
func (a *HostedAdapter) Discover(_ context.Context) ([]ModelDescriptor, error) {
	out := make([]ModelDescriptor, 0, len(a.models))
	for _, m := range a.models {
		out = append(out, ModelDescriptor{
			ID:            a.kind + "/" + m.ID,
			ConnectionID:  a.connID,
			Kind:          a.kind,
			ToolCapable:   m.ToolCapable,
			Vision:        m.Vision,
			ContextWindow: m.ContextWindow,
		})
	}
	return out, nil
}
