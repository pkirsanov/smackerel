package graphapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

type stubPlacesSource struct {
	listFn func(ctx context.Context, limit, offset int) ([]PlaceRow, bool, error)
	getFn  func(ctx context.Context, id string) (*PlaceDetail, error)
}

func (s *stubPlacesSource) ListPlaces(ctx context.Context, limit, offset int) ([]PlaceRow, bool, error) {
	return s.listFn(ctx, limit, offset)
}
func (s *stubPlacesSource) GetPlace(ctx context.Context, id string) (*PlaceDetail, error) {
	return s.getFn(ctx, id)
}

func newPlacesTestHandlers(t *testing.T, src PlacesSource) *PlacesHandlers {
	t.Helper()
	codec, err := NewCursorCodec([]byte("test-secret-for-graphapi-handlers"))
	if err != nil {
		t.Fatalf("NewCursorCodec: %v", err)
	}
	return &PlacesHandlers{
		Source: src,
		Limits: Limits{ListDefault: 50, ListMax: 200, EdgesDefault: 100, EdgesMax: 500, TimeWindowMaxDays: 365},
		Codec:  codec,
	}
}

func mountPlacesRouter(h *PlacesHandlers) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/api/places", h.ListPlaces)
	r.Get("/api/places/{id}", h.GetPlace)
	return r
}

func TestPlacesHandlers_ListPlaces_MergesSources_SCN080_05(t *testing.T) {
	src := &stubPlacesSource{
		listFn: func(_ context.Context, _, _ int) ([]PlaceRow, bool, error) {
			// The source layer is responsible for the union+dedupe;
			// from the handler's perspective the deduped rows arrive
			// flat. We assert the wire envelope and the absence of
			// duplicate ids in the response.
			return []PlaceRow{
				{ID: "mp:37.7749:-122.4194", DisplayName: "cluster 37.7749, -122.4194", ArtifactCount: 4, Source: "merged"},
				{ID: "ar:abc123", DisplayName: "Tartine Bakery", ArtifactCount: 7, Source: "artifact"},
			}, false, nil
		},
	}
	h := newPlacesTestHandlers(t, src)
	rec := httptest.NewRecorder()
	mountPlacesRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/places", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var resp placesListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(resp.Items))
	}
	seen := map[string]bool{}
	sources := map[string]bool{}
	for _, it := range resp.Items {
		if seen[it.ID] {
			t.Fatalf("duplicate id in response: %s", it.ID)
		}
		seen[it.ID] = true
		sources[it.Source] = true
	}
	if !sources["merged"] && !(sources["maps"] && sources["artifact"]) {
		t.Fatalf("expected both sources represented (or a merged row); got sources=%v", sources)
	}
}

func TestPlacesHandlers_ListPlaces_LimitAboveMax_SCN080_15(t *testing.T) {
	h := newPlacesTestHandlers(t, &stubPlacesSource{
		listFn: func(context.Context, int, int) ([]PlaceRow, bool, error) {
			t.Fatal("source must not be called on limit-clamp rejection")
			return nil, false, nil
		},
	})
	rec := httptest.NewRecorder()
	mountPlacesRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/places?limit=10000", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	var env ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != CodeLimitExceeded {
		t.Fatalf("want code=%s, got %s", CodeLimitExceeded, env.Error.Code)
	}
}

func TestPlacesHandlers_ListPlaces_MalformedCursor_SCN080_10(t *testing.T) {
	h := newPlacesTestHandlers(t, &stubPlacesSource{
		listFn: func(context.Context, int, int) ([]PlaceRow, bool, error) {
			t.Fatal("source must not be called when cursor is malformed")
			return nil, false, nil
		},
	})
	rec := httptest.NewRecorder()
	mountPlacesRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/places?cursor=not-a-real-cursor", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	var env ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != CodeInvalidCursor {
		t.Fatalf("want code=%s, got %s", CodeInvalidCursor, env.Error.Code)
	}
}

func TestPlacesHandlers_GetPlace_LocationAndLinked_SCN080_06(t *testing.T) {
	src := &stubPlacesSource{
		getFn: func(_ context.Context, id string) (*PlaceDetail, error) {
			if id != "PL9" {
				t.Fatalf("unexpected id: %s", id)
			}
			return &PlaceDetail{
				ID:          "PL9",
				DisplayName: "Tartine Bakery",
				Location:    &PlaceLocation{Lat: 37.7615, Lon: -122.4241},
				LinkedArtifacts: []CrossLink{
					{TargetKind: "artifact", TargetID: "A1", TargetLabel: "Breakfast notes", Reason: renderSamePlaceReason("Tartine Bakery")},
				},
			}, nil
		},
	}
	h := newPlacesTestHandlers(t, src)
	rec := httptest.NewRecorder()
	mountPlacesRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/places/PL9", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var d PlaceDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.Location == nil || d.Location.Lat == 0 {
		t.Fatalf("want non-nil location, got %+v", d.Location)
	}
	if len(d.LinkedArtifacts) == 0 {
		t.Fatalf("want linkedArtifacts, got empty")
	}
	for _, cl := range d.LinkedArtifacts {
		if !strings.HasPrefix(cl.Reason, "same place ") {
			t.Fatalf("D03-8: reason must start with 'same place ', got %q", cl.Reason)
		}
	}
}

func TestPlacesHandlers_GetPlace_NotFound(t *testing.T) {
	src := &stubPlacesSource{
		getFn: func(context.Context, string) (*PlaceDetail, error) {
			return nil, ErrPlaceNotFound
		},
	}
	h := newPlacesTestHandlers(t, src)
	rec := httptest.NewRecorder()
	mountPlacesRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/places/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}
