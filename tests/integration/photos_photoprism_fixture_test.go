//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/smackerel/smackerel/internal/connector/photos/adapters/photoprism"
)

// integrationPhotoprismFixture mirrors the unit-test fixture used in
// `internal/connector/photos/adapters/photoprism/fixture_test.go` so
// integration tests can talk to a deterministic PhotoPrism without
// needing the real container.
type integrationPhotoprismFixture struct {
	server   *httptest.Server
	apiToken string
	mu       sync.Mutex
	photos   []photoprism.Photo
}

func newIntegrationPhotoprismFixture(t *testing.T, photos []photoprism.Photo) *integrationPhotoprismFixture {
	t.Helper()
	fixture := &integrationPhotoprismFixture{apiToken: "fixture-photoprism-token", photos: photos}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (fixture *integrationPhotoprismFixture) Client() *http.Client { return fixture.server.Client() }
func (fixture *integrationPhotoprismFixture) URL() string          { return fixture.server.URL }
func (fixture *integrationPhotoprismFixture) APIToken() string     { return fixture.apiToken }

func (fixture *integrationPhotoprismFixture) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path != "/api/v1/server" && r.Header.Get("X-Session-ID") != fixture.apiToken {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "unauthorized"})
		return
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	switch {
	case r.URL.Path == "/api/v1/server":
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "v240601-fixture"})
	case r.URL.Path == "/api/v1/photos":
		_ = json.NewEncoder(w).Encode(map[string]any{"photos": fixture.photos, "cursor": "fixture-cursor"})
	case r.URL.Path == "/api/v1/changes":
		_ = json.NewEncoder(w).Encode(map[string]any{"photos": fixture.photos, "cursor": r.URL.Query().Get("cursor")})
	case strings.HasPrefix(r.URL.Path, "/api/v1/albums/") && strings.HasSuffix(r.URL.Path, "/photos"):
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	case strings.HasPrefix(r.URL.Path, "/api/v1/photos/") && strings.HasSuffix(r.URL.Path, "/labels"):
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	default:
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "not_found"})
	}
}

func integrationPhotoprismPhoto(uid string, albumName string, hash string) photoprism.Photo {
	size := int64(2_345_678)
	return photoprism.Photo{
		UID:          uid,
		Type:         "image",
		OriginalName: uid + ".jpg",
		MIME:         "image/jpeg",
		Hash:         hash,
		TakenAt:      "2026-04-27T12:00:00Z",
		UpdatedAt:    "2026-04-27T12:01:00Z",
		FileSize:     &size,
		Albums:       []photoprism.AlbumRef{{UID: "album-" + uid, Name: albumName}},
		EXIFInfo:     map[string]any{"camera": "Synthetic Camera"},
	}
}
