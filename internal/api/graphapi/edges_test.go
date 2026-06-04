package graphapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

type stubEdgesSource struct {
	listFn func(ctx context.Context, kind, id string, limit, offset int) ([]EdgeRow, bool, error)
}

func (s *stubEdgesSource) ListEdges(ctx context.Context, kind, id string, limit, offset int) ([]EdgeRow, bool, error) {
	return s.listFn(ctx, kind, id, limit, offset)
}

func newEdgesTestHandlers(t *testing.T, src EdgesSource) *EdgesHandlers {
	t.Helper()
	codec, err := NewCursorCodec([]byte("test-secret-for-graphapi-handlers"))
	if err != nil {
		t.Fatalf("NewCursorCodec: %v", err)
	}
	return &EdgesHandlers{
		Source: src,
		Limits: Limits{ListDefault: 50, ListMax: 200, EdgesDefault: 100, EdgesMax: 500, TimeWindowMaxDays: 365},
		Codec:  codec,
	}
}

func mountEdgesRouter(h *EdgesHandlers) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/api/graph/edges", h.ListEdges)
	return r
}

// SCN-080-08 happy path: an artifact with edges to topic, person,
// and place targets returns a CrossLink for each, every row carrying
// targetKind/targetId/targetLabel + a non-empty server-derived reason.
func TestEdgesHandler_ListEdges_ArtifactToAllKinds_SCN080_08(t *testing.T) {
	src := &stubEdgesSource{
		listFn: func(_ context.Context, kind, id string, limit, offset int) ([]EdgeRow, bool, error) {
			if kind != "artifact" || id != "A42" {
				t.Fatalf("want kind=artifact id=A42, got kind=%s id=%s", kind, id)
			}
			if limit != 100 || offset != 0 {
				t.Fatalf("want default edges limit=100 offset=0, got %d / %d", limit, offset)
			}
			return []EdgeRow{
				{EdgeID: "e1", TargetKind: "topic", TargetID: "T1", TargetLabel: "travel"},
				{EdgeID: "e2", TargetKind: "person", TargetID: "P1", TargetLabel: "Alice"},
				{EdgeID: "e3", TargetKind: "place", TargetID: "PL1", TargetLabel: "Paris"},
				{EdgeID: "e4", TargetKind: "artifact", TargetID: "A99", TargetLabel: "trip log"},
			}, false, nil
		},
	}
	h := newEdgesTestHandlers(t, src)
	rec := httptest.NewRecorder()
	mountEdgesRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph/edges?source=artifact:A42", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var resp edgesListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 4 {
		t.Fatalf("want 4 cross-links, got %d", len(resp.Items))
	}
	seenKinds := map[string]bool{}
	for _, cl := range resp.Items {
		if cl.TargetKind == "" || cl.TargetID == "" || cl.TargetLabel == "" || cl.Reason == "" {
			t.Fatalf("row missing required field: %+v", cl)
		}
		seenKinds[cl.TargetKind] = true
	}
	for _, want := range []string{"topic", "person", "place", "artifact"} {
		if !seenKinds[want] {
			t.Fatalf("want targetKind=%s in response, got %v", want, seenKinds)
		}
	}
}

// SCN-080-14 unknown source kind → 400 with code=invalid_kind and a
// message that lists the four allowed kinds verbatim.
func TestEdgesHandler_UnknownKind_400(t *testing.T) {
	h := newEdgesTestHandlers(t, &stubEdgesSource{
		listFn: func(context.Context, string, string, int, int) ([]EdgeRow, bool, error) {
			t.Fatal("source must not be called for unknown kind")
			return nil, false, nil
		},
	})
	rec := httptest.NewRecorder()
	mountEdgesRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph/edges?source=unicorn:X1", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var env ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != CodeInvalidKind {
		t.Fatalf("want code=%s, got %s", CodeInvalidKind, env.Error.Code)
	}
	if env.Error.Field != "source" {
		t.Fatalf("want field=source, got %s", env.Error.Field)
	}
	for _, want := range []string{"artifact", "topic", "person", "place"} {
		if !strings.Contains(env.Error.Message, want) {
			t.Fatalf("error message must list all allowed kinds; missing %q in %q", want, env.Error.Message)
		}
	}
}

func TestEdgesHandler_MissingSource_400(t *testing.T) {
	h := newEdgesTestHandlers(t, &stubEdgesSource{
		listFn: func(context.Context, string, string, int, int) ([]EdgeRow, bool, error) {
			t.Fatal("source must not be called when source param is missing")
			return nil, false, nil
		},
	})
	rec := httptest.NewRecorder()
	mountEdgesRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph/edges", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	var env ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != CodeMissingParam {
		t.Fatalf("want code=%s, got %s", CodeMissingParam, env.Error.Code)
	}
}

func TestEdgesHandler_MalformedSource_400(t *testing.T) {
	h := newEdgesTestHandlers(t, &stubEdgesSource{
		listFn: func(context.Context, string, string, int, int) ([]EdgeRow, bool, error) {
			t.Fatal("source must not be called when source param lacks colon")
			return nil, false, nil
		},
	})
	rec := httptest.NewRecorder()
	mountEdgesRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph/edges?source=artifactA42", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestEdgesHandler_EmptyLabel_Returns500(t *testing.T) {
	// D04-4: resolver MUST NOT emit a CrossLink with a blank reason —
	// when the underlying join cannot resolve a label the handler
	// must surface 500 / internal_reason_missing rather than ship the
	// empty-reason row to the client.
	h := newEdgesTestHandlers(t, &stubEdgesSource{
		listFn: func(context.Context, string, string, int, int) ([]EdgeRow, bool, error) {
			return []EdgeRow{
				{EdgeID: "e1", TargetKind: "topic", TargetID: "T1", TargetLabel: ""},
			}, false, nil
		},
	})
	rec := httptest.NewRecorder()
	mountEdgesRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph/edges?source=artifact:A42", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "internal_reason_missing") {
		t.Fatalf("body must carry internal_reason_missing code, got %s", rec.Body.String())
	}
}

func TestEdgesHandler_LimitAboveMax_400(t *testing.T) {
	h := newEdgesTestHandlers(t, &stubEdgesSource{
		listFn: func(context.Context, string, string, int, int) ([]EdgeRow, bool, error) {
			t.Fatal("source must not be called on limit-clamp rejection")
			return nil, false, nil
		},
	})
	rec := httptest.NewRecorder()
	mountEdgesRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph/edges?source=artifact:A42&limit=99999", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	var env ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != CodeLimitExceeded {
		t.Fatalf("want code=%s, got %s", CodeLimitExceeded, env.Error.Code)
	}
}

func TestEdgesHandler_NextCursorRoundTrip(t *testing.T) {
	src := &stubEdgesSource{
		listFn: func(_ context.Context, _, _ string, _, offset int) ([]EdgeRow, bool, error) {
			if offset == 0 {
				return []EdgeRow{{EdgeID: "e1", TargetKind: "topic", TargetID: "T1", TargetLabel: "x"}}, true, nil
			}
			return []EdgeRow{{EdgeID: "e2", TargetKind: "topic", TargetID: "T2", TargetLabel: "y"}}, false, nil
		},
	}
	h := newEdgesTestHandlers(t, src)
	rec := httptest.NewRecorder()
	mountEdgesRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/graph/edges?source=artifact:A42", nil))
	var page1 edgesListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &page1)
	if page1.NextCursor == "" {
		t.Fatal("expected non-empty nextCursor when hasNext=true")
	}
	payload, err := h.Codec.Decode(page1.NextCursor)
	if err != nil {
		t.Fatalf("cursor decode: %v", err)
	}
	if payload.Resource != "edges" {
		t.Fatalf("want resource=edges, got %s", payload.Resource)
	}
	if payload.Offset <= 0 {
		t.Fatalf("want offset > 0, got %d", payload.Offset)
	}
}

// TestResolveEdges_RendersEveryTaxonomyEntry asserts the resolver
// emits the design.md §2 reason template literal for each of the
// four target kinds the closed-set allows. design.md §2 lists five
// taxonomy entries; same-day capture (ReasonShareTimeWindow) is
// derived from edge metadata rather than dst_type so it is covered
// by the resolver's ResolveReason fail-loud unit test below rather
// than by ReasonKindForTargetKind.
func TestResolveEdges_RendersEveryTaxonomyEntry(t *testing.T) {
	cases := []struct {
		targetKind  string
		targetLabel string
		wantPrefix  string
	}{
		{"topic", "travel", "shares topic "},
		{"person", "Alice", "co-occurs with "},
		{"place", "Paris", "same place "},
		{"artifact", "trip log", "mentioned in "},
	}
	rows := make([]EdgeRow, 0, len(cases))
	for i, c := range cases {
		rows = append(rows, EdgeRow{EdgeID: "e" + string(rune('1'+i)), TargetKind: c.targetKind, TargetID: "X" + c.targetKind, TargetLabel: c.targetLabel})
	}
	got, err := resolveEdges(rows)
	if err != nil {
		t.Fatalf("resolveEdges: %v", err)
	}
	if len(got) != len(cases) {
		t.Fatalf("want %d items, got %d", len(cases), len(got))
	}
	for i, c := range cases {
		if !strings.HasPrefix(got[i].Reason, c.wantPrefix) {
			t.Fatalf("row %d: want reason prefix %q, got %q", i, c.wantPrefix, got[i].Reason)
		}
		if !strings.Contains(got[i].Reason, c.targetLabel) {
			t.Fatalf("row %d: reason must contain label %q, got %q", i, c.targetLabel, got[i].Reason)
		}
	}

	// Same-day capture taxonomy entry — covered by ResolveReason
	// directly because it depends on edge metadata (capture date)
	// rather than dst_type.
	sameDay, err := ResolveReason(ReasonShareTimeWindow, "2026-06-03")
	if err != nil {
		t.Fatalf("ResolveReason(ReasonShareTimeWindow): %v", err)
	}
	if !strings.HasPrefix(sameDay, "captured on ") {
		t.Fatalf("want same-day reason to start with 'captured on ', got %q", sameDay)
	}
}

// Adversarial: an edge with empty label MUST surface a typed error
// rather than silently emit a CrossLink with a blank reason. D04-4.
func TestResolveEdges_EmptyReason_IsError(t *testing.T) {
	_, err := resolveEdges([]EdgeRow{
		{EdgeID: "e1", TargetKind: "topic", TargetID: "T1", TargetLabel: ""},
	})
	if err == nil {
		t.Fatal("want error for empty label, got nil")
	}
	if !errors.Is(err, ErrReasonRenderEmpty) {
		t.Fatalf("want ErrReasonRenderEmpty, got %v", err)
	}
}

// Adversarial: target kind outside the closed-set surfaces as a
// typed error so the handler returns 500 rather than 200 with a
// CrossLink that the client cannot interpret.
func TestResolveEdges_UnknownKind_IsError(t *testing.T) {
	_, err := resolveEdges([]EdgeRow{
		{EdgeID: "e1", TargetKind: "unicorn", TargetID: "U1", TargetLabel: "label"},
	})
	if err == nil {
		t.Fatal("want error for unknown target kind, got nil")
	}
	if !errors.Is(err, ErrReasonRenderUnknownKind) {
		t.Fatalf("want ErrReasonRenderUnknownKind, got %v", err)
	}
}
