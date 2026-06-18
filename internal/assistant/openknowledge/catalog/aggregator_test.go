// Spec 096 SCOPE-04 — catalog aggregation + graceful-degradation tests
// (SCN-096-D01). These are UNIT tests: the Ollama adapter's `/api/tags` call is
// driven by an injected fake HTTPDoer and the down-provider paths by injected
// stub adapters — NO live Ollama, NO network interception. The live
// one-provider-down leg is the deferred integration test
// tests/integration/model_discovery_test.go (C7).
package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/config"
)

// fakeDoer is an injected HTTP client for the Ollama adapter — it returns a
// canned response (or error) so the live `/api/tags` parse is exercised with no
// daemon.
type fakeDoer struct {
	resp *http.Response
	err  error
}

func (f *fakeDoer) Do(_ *http.Request) (*http.Response, error) { return f.resp, f.err }

// tagsBody builds a 200 `/api/tags` response body listing the given installed
// model names (the subset shape the adapter consumes).
func tagsBody(names ...string) *http.Response {
	type m struct {
		Name string `json:"name"`
	}
	var payload struct {
		Models []m `json:"models"`
	}
	for _, n := range names {
		payload.Models = append(payload.Models, m{Name: n})
	}
	b, _ := json.Marshal(payload)
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(b))}
}

// stubAdapter is an injected DiscoveryAdapter for the graceful-degradation and
// caching tests: it can return a fixed model set, a typed error, or block until
// the per-provider context deadline fires (a genuine timeout).
type stubAdapter struct {
	connID, kind string
	models       []ModelDescriptor
	err          error
	block        bool
	calls        int
}

func (s *stubAdapter) ConnectionID() string { return s.connID }
func (s *stubAdapter) Kind() string         { return s.kind }
func (s *stubAdapter) Discover(ctx context.Context) ([]ModelDescriptor, error) {
	s.calls++
	if s.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return s.models, s.err
}

func TestCatalogAggregator_AggregatesOllamaAndHostedProviderQualified_Spec096(t *testing.T) {
	// Ollama adapter — live `/api/tags` returns two installed models; the
	// operator capability hint stamps tool_capable/context on one of them
	// (the `/api/tags` payload itself carries no capabilities).
	ollama := NewOllamaAdapter(
		"local-ollama",
		"http://ollama:11434",
		&fakeDoer{resp: tagsBody("gemma3:4b", "llama3:8b")},
		map[string]ModelCapabilities{"gemma3:4b": {ToolCapable: true, ContextWindow: 8192}},
	)

	// Hosted adapter — the SST-curated anthropic list with registry
	// capabilities carried through.
	hosted := NewHostedAdapter(config.ModelConnection{
		ID:   "anthropic-main",
		Kind: config.ModelConnectionKindAnthropic,
		Models: config.ModelConnectionModels{
			Strategy: "curated",
			List: []config.ModelDescriptor{
				{ID: "claude-3-5-sonnet", ToolCapable: true, Vision: true, ContextWindow: 200000},
			},
		},
	})

	agg, err := NewCatalogAggregator([]DiscoveryAdapter{ollama, hosted}, 60000, 2000, "ollama/gemma3:4b")
	if err != nil {
		t.Fatalf("NewCatalogAggregator: unexpected error: %v", err)
	}

	cat, statuses := agg.GetCatalog(context.Background())

	// ONE provider-qualified catalog: Ollama-local group first (declaration
	// order), then the hosted connection's curated model.
	wantIDs := []string{"ollama/gemma3:4b", "ollama/llama3:8b", "anthropic/claude-3-5-sonnet"}
	gotIDs := cat.IDs()
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("catalog ids: got %v, want %v", gotIDs, wantIDs)
	}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("catalog id[%d]: got %q, want %q (order: Ollama group first)", i, gotIDs[i], wantIDs[i])
		}
	}

	// Each descriptor carries id + connection_id + kind + capabilities.
	byID := map[string]ModelDescriptor{}
	for _, m := range cat.Models {
		byID[m.ID] = m
	}
	if d := byID["ollama/gemma3:4b"]; d.ConnectionID != "local-ollama" || d.Kind != "ollama" || !d.ToolCapable || d.ContextWindow != 8192 {
		t.Fatalf("ollama/gemma3:4b descriptor wrong: %+v", d)
	}
	if d := byID["ollama/llama3:8b"]; d.ConnectionID != "local-ollama" || d.Kind != "ollama" || d.ToolCapable {
		t.Fatalf("ollama/llama3:8b descriptor wrong (no hint ⇒ zero caps): %+v", d)
	}
	if d := byID["anthropic/claude-3-5-sonnet"]; d.ConnectionID != "anthropic-main" || d.Kind != "anthropic" || !d.ToolCapable || !d.Vision || d.ContextWindow != 200000 {
		t.Fatalf("anthropic/claude-3-5-sonnet descriptor wrong: %+v", d)
	}

	// One typed status per effective-enabled connection, both reachable.
	if len(statuses) != 2 {
		t.Fatalf("statuses: got %d, want one per adapter (2)", len(statuses))
	}
	for _, s := range statuses {
		if s.State != StateOK {
			t.Fatalf("status %q: got state %q, want ok", s.ConnectionID, s.State)
		}
	}
	if statuses[0].ModelCount != 2 || statuses[1].ModelCount != 1 {
		t.Fatalf("status model counts: got %d/%d, want 2/1", statuses[0].ModelCount, statuses[1].ModelCount)
	}

	// Default carried through (the no-override synthesis model).
	if cat.Default != "ollama/gemma3:4b" {
		t.Fatalf("catalog default: got %q, want ollama/gemma3:4b", cat.Default)
	}

	t.Run("ttl_cache_avoids_reprobe_within_sst_window", func(t *testing.T) {
		counter := &stubAdapter{connID: "local-ollama", kind: "ollama", models: []ModelDescriptor{{ID: "ollama/gemma3:4b", ConnectionID: "local-ollama", Kind: "ollama"}}}
		fixed := time.Unix(1_700_000_000, 0)
		a, err := NewCatalogAggregator([]DiscoveryAdapter{counter}, 60000, 2000, "ollama/gemma3:4b")
		if err != nil {
			t.Fatalf("NewCatalogAggregator: %v", err)
		}
		a.WithNow(func() time.Time { return fixed })
		a.GetCatalog(context.Background())
		a.GetCatalog(context.Background()) // within the 60s TTL ⇒ served from cache
		if counter.calls != 1 {
			t.Fatalf("adapter probed %d times within the SST cache_ttl_ms window, want 1 (cached)", counter.calls)
		}
	})
}

// TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096 is
// the ADVERSARIAL graceful-degradation proof: a down / timed-out / auth-failed
// provider contributes NO models but ALWAYS a typed ProviderDiscoveryStatus,
// and the reachable subset is still served. It FAILS if a down provider is ever
// silently dropped (no status) or if a failure poisons the whole catalog.
func TestCatalogAggregator_GracefulDegradation_TypedStatusNeverDropped_Spec096(t *testing.T) {
	hostedUp := NewHostedAdapter(config.ModelConnection{
		ID:   "anthropic-main",
		Kind: config.ModelConnectionKindAnthropic,
		Models: config.ModelConnectionModels{
			Strategy: "curated",
			List:     []config.ModelDescriptor{{ID: "claude-3-5-sonnet", ToolCapable: true}},
		},
	})

	cases := []struct {
		name      string
		down      DiscoveryAdapter
		wantState DiscoveryState
		timeoutMs int
	}{
		{
			name:      "ollama_unreachable_connect_error",
			down:      NewOllamaAdapter("local-ollama", "http://ollama:11434", &fakeDoer{err: errors.New("dial tcp: connection refused")}, nil),
			wantState: StateUnreachable,
			timeoutMs: 2000,
		},
		{
			name:      "hosted_auth_failed_typed",
			down:      &stubAdapter{connID: "openai-main", kind: "openai", err: &DiscoveryError{State: StateAuthFailed, Detail: "invalid api key"}},
			wantState: StateAuthFailed,
			timeoutMs: 2000,
		},
		{
			name:      "provider_times_out",
			down:      &stubAdapter{connID: "bedrock-main", kind: "bedrock", block: true},
			wantState: StateTimeout,
			timeoutMs: 20, // genuine ctx deadline — the blocking adapter returns DeadlineExceeded
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Order: the DOWN provider first so a naive "stop on first error"
			// or "drop the failer" bug would also lose the reachable subset.
			agg, err := NewCatalogAggregator([]DiscoveryAdapter{tc.down, hostedUp}, 60000, tc.timeoutMs, "ollama/gemma3:4b")
			if err != nil {
				t.Fatalf("NewCatalogAggregator: %v", err)
			}
			cat, statuses := agg.GetCatalog(context.Background())

			// Reachable subset STILL served.
			if got := cat.IDs(); len(got) != 1 || got[0] != "anthropic/claude-3-5-sonnet" {
				t.Fatalf("reachable subset not served: got %v, want [anthropic/claude-3-5-sonnet]", got)
			}

			// The down provider is NEVER silently dropped — exactly one status
			// per adapter, including the down one, with its typed state.
			if len(statuses) != 2 {
				t.Fatalf("statuses: got %d, want 2 (down provider must NOT be dropped)", len(statuses))
			}
			var downStatus *ProviderDiscoveryStatus
			for i := range statuses {
				if statuses[i].ConnectionID == tc.down.ConnectionID() {
					downStatus = &statuses[i]
				}
			}
			if downStatus == nil {
				t.Fatalf("down provider %q has NO ProviderDiscoveryStatus (silently dropped)", tc.down.ConnectionID())
			}
			if downStatus.State != tc.wantState {
				t.Fatalf("down provider state: got %q, want %q", downStatus.State, tc.wantState)
			}
			if downStatus.ModelCount != 0 {
				t.Fatalf("down provider model_count: got %d, want 0 (its models are absent)", downStatus.ModelCount)
			}
		})
	}
}

// TestCatalogAggregator_FailLoudOnNonPositiveSSTBounds_Spec096 backs DoD
// D04-T2-4: the discovery bounds come from SST `> 0` (SCOPE-01); a non-positive
// cache_ttl_ms / per_provider_timeout_ms is a fail-loud construction error, not
// a substituted default (G028).
func TestCatalogAggregator_FailLoudOnNonPositiveSSTBounds_Spec096(t *testing.T) {
	if _, err := NewCatalogAggregator(nil, 60000, 2000, "ollama/gemma3:4b"); err != nil {
		t.Fatalf("control (valid bounds) must construct: %v", err)
	}
	for _, tc := range []struct {
		name      string
		ttl, tout int
	}{
		{"zero_ttl", 0, 2000},
		{"negative_ttl", -1, 2000},
		{"zero_timeout", 60000, 0},
		{"negative_timeout", 60000, -5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewCatalogAggregator(nil, tc.ttl, tc.tout, "ollama/gemma3:4b"); err == nil {
				t.Fatalf("non-positive SST bound (%d,%d) must fail loud, got nil error", tc.ttl, tc.tout)
			}
		})
	}
}
