package graphapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

type stubPeopleSource struct {
	listFn func(ctx context.Context, limit, offset int) ([]PersonRow, bool, error)
	getFn  func(ctx context.Context, id string) (*PersonDetail, error)
}

func (s *stubPeopleSource) ListPeople(ctx context.Context, limit, offset int) ([]PersonRow, bool, error) {
	return s.listFn(ctx, limit, offset)
}
func (s *stubPeopleSource) GetPerson(ctx context.Context, id string) (*PersonDetail, error) {
	return s.getFn(ctx, id)
}

func newPeopleTestHandlers(t *testing.T, src PeopleSource) *PeopleHandlers {
	t.Helper()
	codec, err := NewCursorCodec([]byte("test-secret-for-graphapi-handlers"))
	if err != nil {
		t.Fatalf("NewCursorCodec: %v", err)
	}
	return &PeopleHandlers{
		Source: src,
		Limits: Limits{ListDefault: 50, ListMax: 200, EdgesDefault: 100, EdgesMax: 500, TimeWindowMaxDays: 365},
		Codec:  codec,
	}
}

func mountPeopleRouter(h *PeopleHandlers) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/api/people", h.ListPeople)
	r.Get("/api/people/{id}", h.GetPerson)
	return r
}

func TestPeopleHandlers_ListPeople_HappyPath_SCN080_03(t *testing.T) {
	src := &stubPeopleSource{
		listFn: func(_ context.Context, _, _ int) ([]PersonRow, bool, error) {
			return []PersonRow{
				{ID: "P1", DisplayName: "Alice", ArtifactCount: 12},
				{ID: "P2", DisplayName: "Bob", ArtifactCount: 7},
			}, false, nil
		},
	}
	h := newPeopleTestHandlers(t, src)
	rec := httptest.NewRecorder()
	mountPeopleRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/people", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp peopleListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(resp.Items))
	}
	for i, it := range resp.Items {
		if it.ID == "" || it.DisplayName == "" {
			t.Fatalf("row %d missing id/displayName: %+v", i, it)
		}
	}
}

func TestPeopleHandlers_ListPeople_LimitAboveMax_SCN080_15(t *testing.T) {
	h := newPeopleTestHandlers(t, &stubPeopleSource{
		listFn: func(context.Context, int, int) ([]PersonRow, bool, error) {
			t.Fatal("source must not be called on limit clamp rejection")
			return nil, false, nil
		},
	})
	rec := httptest.NewRecorder()
	mountPeopleRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/people?limit=10000", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	var env ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != CodeLimitExceeded {
		t.Fatalf("want code=%s, got %s", CodeLimitExceeded, env.Error.Code)
	}
}

func TestPeopleHandlers_GetPerson_TimelineDescOrder_SCN080_04(t *testing.T) {
	t0 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(48 * time.Hour)
	t2 := t1.Add(72 * time.Hour)
	src := &stubPeopleSource{
		getFn: func(_ context.Context, id string) (*PersonDetail, error) {
			if id != "P5" {
				t.Fatalf("want id=P5, got %q", id)
			}
			return &PersonDetail{
				ID: "P5", DisplayName: "Carol",
				ArtifactTimeline: []ArtifactTimelineEntry{
					{ArtifactID: "A3", Title: "newest", CapturedAt: t2},
					{ArtifactID: "A2", Title: "middle", CapturedAt: t1},
					{ArtifactID: "A1", Title: "oldest", CapturedAt: t0},
				},
				RelatedTopics: []CrossLink{{TargetKind: "topic", TargetID: "T1", TargetLabel: "travel", Reason: RenderReason(ReasonCoOccursWithTopic, "travel")}},
				RelatedPlaces: []CrossLink{{TargetKind: "place", TargetID: "PL1", TargetLabel: "Lisbon", Reason: RenderReason(ReasonNearPlace, "Lisbon")}},
			}, nil
		},
	}
	h := newPeopleTestHandlers(t, src)
	rec := httptest.NewRecorder()
	mountPeopleRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/people/P5", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var d PersonDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(d.ArtifactTimeline) != 3 {
		t.Fatalf("want 3 timeline rows, got %d", len(d.ArtifactTimeline))
	}
	// SCN-080-04 invariant: timeline ordered DESC by capturedAt.
	// Adversarial: if a future refactor reintroduces ORDER BY ASC,
	// this comparison fails loudly.
	for i := 1; i < len(d.ArtifactTimeline); i++ {
		prev := d.ArtifactTimeline[i-1].CapturedAt
		curr := d.ArtifactTimeline[i].CapturedAt
		if curr.After(prev) {
			t.Fatalf("timeline not DESC at index %d: prev=%v curr=%v", i, prev, curr)
		}
	}
	for _, cl := range append(d.RelatedTopics, d.RelatedPlaces...) {
		if cl.TargetKind == "" || cl.TargetID == "" || cl.TargetLabel == "" || cl.Reason == "" {
			t.Fatalf("cross-link missing field: %+v", cl)
		}
	}
}

func TestPeopleHandlers_GetPerson_NotFound(t *testing.T) {
	src := &stubPeopleSource{
		getFn: func(context.Context, string) (*PersonDetail, error) { return nil, ErrPersonNotFound },
	}
	h := newPeopleTestHandlers(t, src)
	rec := httptest.NewRecorder()
	mountPeopleRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/people/ghost", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestPeopleHandlers_ListPeople_NilRowsRendersEmptyArray(t *testing.T) {
	// Adversarial: a nil slice from the source MUST serialize as
	// "items":[] (not "items":null). PWA consumers depend on the
	// shape being a JSON array even when there is no data yet.
	h := newPeopleTestHandlers(t, &stubPeopleSource{
		listFn: func(context.Context, int, int) ([]PersonRow, bool, error) { return nil, false, nil },
	})
	rec := httptest.NewRecorder()
	mountPeopleRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/people", nil))
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", rec.Code, body)
	}
	if !contains(body, "\"items\":[]") {
		t.Fatalf("want items:[] in body, got: %s", body)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
