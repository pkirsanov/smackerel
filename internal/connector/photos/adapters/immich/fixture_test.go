package immich

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type FixtureData struct {
	Assets []Asset
}

type FixtureServer struct {
	server  *httptest.Server
	apiKey  string
	mu      sync.Mutex
	assets  []Asset
	changes map[string][]Asset
}

func NewFixtureServer(t *testing.T, data FixtureData) *FixtureServer {
	t.Helper()
	fixture := &FixtureServer{apiKey: "fixture-api-key", assets: data.Assets, changes: map[string][]Asset{}}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (fixture *FixtureServer) Client() *http.Client { return fixture.server.Client() }
func (fixture *FixtureServer) URL() string          { return fixture.server.URL }
func (fixture *FixtureServer) APIKey() string       { return fixture.apiKey }

func (fixture *FixtureServer) SetChanges(cursor string, assets []Asset) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.changes[cursor] = assets
}

func (fixture *FixtureServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path != "/api/server/version" && r.Header.Get("x-api-key") != fixture.apiKey {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "unauthorized"})
		return
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	switch r.URL.Path {
	case "/api/server/version":
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "v1.120.0", "major": 1})
	case "/api/smackerel/assets":
		_ = json.NewEncoder(w).Encode(map[string]any{"assets": fixture.assets, "cursor": "fixture-cursor"})
	case "/api/smackerel/changes":
		_ = json.NewEncoder(w).Encode(map[string]any{"assets": fixture.changes[r.URL.Query().Get("cursor")], "cursor": r.URL.Query().Get("cursor")})
	default:
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "not_found"})
	}
}
