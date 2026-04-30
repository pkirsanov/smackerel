//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/smackerel/smackerel/internal/connector/photos/adapters/immich"
)

type immichFixtureData struct {
	Assets []immich.Asset
}

type immichFixtureServer struct {
	server  *httptest.Server
	apiKey  string
	mu      sync.Mutex
	assets  []immich.Asset
	changes map[string][]immich.Asset
}

func newImmichFixtureServer(t *testing.T, data immichFixtureData) *immichFixtureServer {
	t.Helper()
	fixture := &immichFixtureServer{apiKey: "fixture-api-key", assets: data.Assets, changes: map[string][]immich.Asset{}}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (fixture *immichFixtureServer) Client() *http.Client { return fixture.server.Client() }
func (fixture *immichFixtureServer) URL() string          { return fixture.server.URL }
func (fixture *immichFixtureServer) APIKey() string       { return fixture.apiKey }

func (fixture *immichFixtureServer) SetChanges(cursor string, assets []immich.Asset) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.changes[cursor] = assets
}

func (fixture *immichFixtureServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
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
