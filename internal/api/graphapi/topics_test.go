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

// stubTopicsSource is the in-memory TopicsSource used by the unit
// tests. Returning fixed rows lets handler tests assert wire shape,
// pagination math, and error envelopes without a live Postgres.
type stubTopicsSource struct {
	listFn func(ctx context.Context, limit, offset int) ([]TopicRow, bool, error)
	getFn  func(ctx context.Context, id string) (*TopicDetail, error)
}

func (s *stubTopicsSource) ListTopics(ctx context.Context, limit, offset int) ([]TopicRow, bool, error) {
	return s.listFn(ctx, limit, offset)
}
func (s *stubTopicsSource) GetTopic(ctx context.Context, id string) (*TopicDetail, error) {
	return s.getFn(ctx, id)
}

func newTopicsTestHandlers(t *testing.T, src TopicsSource) *TopicsHandlers {
	t.Helper()
	codec, err := NewCursorCodec([]byte("test-secret-for-graphapi-handlers"))
	if err != nil {
		t.Fatalf("NewCursorCodec: %v", err)
	}
	return &TopicsHandlers{
		Source: src,
		Limits: Limits{ListDefault: 50, ListMax: 200, EdgesDefault: 100, EdgesMax: 500, TimeWindowMaxDays: 365},
		Codec:  codec,
	}
}

func mountTopicsRouter(h *TopicsHandlers) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/api/topics", h.ListTopics)
	r.Get("/api/topics/{id}", h.GetTopic)
	return r
}

func TestTopicsHandlers_ListTopics_HappyPath_SCN080_01(t *testing.T) {
	src := &stubTopicsSource{
		listFn: func(_ context.Context, limit, offset int) ([]TopicRow, bool, error) {
			if limit != 50 || offset != 0 {
				t.Fatalf("expected default limit=50 offset=0, got limit=%d offset=%d", limit, offset)
			}
			return []TopicRow{
				{ID: "T1", Label: "alpha", LinkedArtifactCount: 3, PeopleCount: 1, PlaceCount: 0},
				{ID: "T2", Label: "beta", LinkedArtifactCount: 5, PeopleCount: 2, PlaceCount: 1},
			}, false, nil
		},
	}
	h := newTopicsTestHandlers(t, src)
	rec := httptest.NewRecorder()
	mountTopicsRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/topics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var resp topicsListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(resp.Items))
	}
	if resp.Items[0].ID == "" || resp.Items[0].Label == "" {
		t.Fatal("item missing id/label")
	}
	if resp.NextCursor != "" {
		t.Fatalf("want empty nextCursor (hasNext=false), got %q", resp.NextCursor)
	}
}

func TestTopicsHandlers_ListTopics_PaginationCursor_SCN080_01(t *testing.T) {
	src := &stubTopicsSource{
		listFn: func(_ context.Context, _, _ int) ([]TopicRow, bool, error) {
			return []TopicRow{{ID: "T1", Label: "x"}}, true, nil
		},
	}
	h := newTopicsTestHandlers(t, src)
	rec := httptest.NewRecorder()
	mountTopicsRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/topics", nil))

	var resp topicsListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.NextCursor == "" {
		t.Fatal("expected non-empty nextCursor when hasNext=true")
	}
	// Round-trip the cursor: decoding it MUST yield a payload whose
	// resource matches "topics" and offset > 0. This catches a
	// regression where the handler emits a raw opaque blob without
	// HMAC signing.
	p, err := h.Codec.Decode(resp.NextCursor)
	if err != nil {
		t.Fatalf("returned cursor failed to decode: %v", err)
	}
	if p.Resource != "topics" || p.Offset == 0 {
		t.Fatalf("cursor payload wrong: %+v", p)
	}
}

func TestTopicsHandlers_ListTopics_LimitAboveMax_SCN080_15(t *testing.T) {
	h := newTopicsTestHandlers(t, &stubTopicsSource{
		listFn: func(context.Context, int, int) ([]TopicRow, bool, error) {
			t.Fatal("source must not be called when limit clamp rejects")
			return nil, false, nil
		},
	})
	rec := httptest.NewRecorder()
	mountTopicsRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/topics?limit=10000", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	var env ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != CodeLimitExceeded {
		t.Fatalf("want code=%s, got %s", CodeLimitExceeded, env.Error.Code)
	}
}

func TestTopicsHandlers_ListTopics_MalformedCursor_SCN080_11(t *testing.T) {
	h := newTopicsTestHandlers(t, &stubTopicsSource{
		listFn: func(context.Context, int, int) ([]TopicRow, bool, error) {
			t.Fatal("source must not be called on malformed cursor")
			return nil, false, nil
		},
	})
	rec := httptest.NewRecorder()
	mountTopicsRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/topics?cursor=not-a-real-cursor", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	var env ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != CodeInvalidCursor || env.Error.Field != "cursor" {
		t.Fatalf("want code=invalid_cursor field=cursor, got code=%s field=%s", env.Error.Code, env.Error.Field)
	}
}

func TestTopicsHandlers_ListTopics_CursorWrongResource_Rejected(t *testing.T) {
	h := newTopicsTestHandlers(t, &stubTopicsSource{
		listFn: func(context.Context, int, int) ([]TopicRow, bool, error) {
			t.Fatal("source must not be called when cursor resource mismatches")
			return nil, false, nil
		},
	})
	// Adversarial: encode a valid cursor for resource "people" and
	// present it to /api/topics. A naive handler that ignores the
	// Resource field would accept the cross-resource cursor; ours
	// MUST reject it as invalid_cursor.
	wrong, _ := h.Codec.Encode(CursorPayload{Resource: "people", Offset: 25})
	rec := httptest.NewRecorder()
	mountTopicsRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/topics?cursor="+wrong, nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestTopicsHandlers_GetTopic_HappyPath_SCN080_02(t *testing.T) {
	src := &stubTopicsSource{
		getFn: func(_ context.Context, id string) (*TopicDetail, error) {
			if id != "T123" {
				t.Fatalf("want id=T123, got %q", id)
			}
			return &TopicDetail{
				ID: "T123", Label: "travel",
				LinkedArtifacts: []CrossLink{{TargetKind: "artifact", TargetID: "A1", TargetLabel: "trip notes", Reason: RenderReason(ReasonMentionedInArtifact, "trip notes")}},
				RelatedPeople:   []CrossLink{{TargetKind: "person", TargetID: "P1", TargetLabel: "Alice", Reason: RenderReason(ReasonCoOccursWithTopic, "travel")}},
				RelatedPlaces:   []CrossLink{{TargetKind: "place", TargetID: "PL1", TargetLabel: "Paris", Reason: RenderReason(ReasonNearPlace, "Paris")}},
			}, nil
		},
	}
	h := newTopicsTestHandlers(t, src)
	rec := httptest.NewRecorder()
	mountTopicsRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/topics/T123", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var d TopicDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// SCN-080-02 invariant: every cross-link row has non-empty
	// targetKind/targetId/targetLabel/reason — server-derived,
	// never client-synthesized.
	all := append(append([]CrossLink{}, d.LinkedArtifacts...), d.RelatedPeople...)
	all = append(all, d.RelatedPlaces...)
	if len(all) == 0 {
		t.Fatal("expected at least one cross-link row across the three buckets")
	}
	for i, cl := range all {
		if cl.TargetKind == "" || cl.TargetID == "" || cl.TargetLabel == "" || cl.Reason == "" {
			t.Fatalf("row %d has empty cross-link field: %+v", i, cl)
		}
	}
}

func TestTopicsHandlers_GetTopic_NotFound(t *testing.T) {
	src := &stubTopicsSource{
		getFn: func(context.Context, string) (*TopicDetail, error) { return nil, ErrTopicNotFound },
	}
	h := newTopicsTestHandlers(t, src)
	rec := httptest.NewRecorder()
	mountTopicsRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/topics/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "\"error\"") {
		t.Fatalf("body must use error envelope, got %s", rec.Body.String())
	}
}
