package photoprism

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

type photoprismFixtureData struct {
	Photos []Photo
}

type photoprismFixtureServer struct {
	server   *httptest.Server
	apiToken string
	mu       sync.Mutex
	photos   []Photo
	changes  map[string][]Photo
}

func newPhotoprismFixtureServer(t *testing.T, data photoprismFixtureData) *photoprismFixtureServer {
	t.Helper()
	fixture := &photoprismFixtureServer{
		apiToken: "fixture-photoprism-token",
		photos:   data.Photos,
		changes:  map[string][]Photo{},
	}
	fixture.server = httptest.NewServer(http.HandlerFunc(fixture.serveHTTP))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (fixture *photoprismFixtureServer) Client() *http.Client { return fixture.server.Client() }
func (fixture *photoprismFixtureServer) URL() string          { return fixture.server.URL }
func (fixture *photoprismFixtureServer) APIToken() string     { return fixture.apiToken }

// SetChanges seeds the cursor-keyed change feed used by Watch().
func (fixture *photoprismFixtureServer) SetChanges(cursor string, photos []Photo) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.changes[cursor] = photos
}

func (fixture *photoprismFixtureServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
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
		_ = json.NewEncoder(w).Encode(map[string]any{"photos": fixture.changes[r.URL.Query().Get("cursor")], "cursor": r.URL.Query().Get("cursor")})
	case strings.HasPrefix(r.URL.Path, "/api/v1/albums/") && strings.HasSuffix(r.URL.Path, "/photos"):
		// AddToAlbum POST returns 200 with no body — the writer
		// doesn't decode the body so this is enough.
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

func samplePhoto(uid string, albumUID string, albumName string) Photo {
	size := int64(2_345_678)
	return Photo{
		UID:          uid,
		Type:         "image",
		OriginalName: uid + ".jpg",
		MIME:         "image/jpeg",
		Hash:         "fixturehash-" + uid,
		TakenAt:      "2026-04-27T12:00:00Z",
		UpdatedAt:    "2026-04-27T12:01:00Z",
		FileSize:     &size,
		Albums:       []AlbumRef{{UID: albumUID, Name: albumName}},
		EXIFInfo:     map[string]any{"camera": "Synthetic Camera"},
		Subjects:     []SubjectRef{{UID: "subj-001", Name: "Maria"}},
		Classification: &photolib.ClassificationDecision{
			Caption:         "vacation photo",
			PrimaryCategory: "lifestyle/vacation",
			Confidence:      0.88,
			Rationale:       "fixture caption",
		},
	}
}
