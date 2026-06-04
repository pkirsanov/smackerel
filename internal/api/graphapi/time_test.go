package graphapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

type stubTimeSource struct {
	fn func(ctx context.Context, from, to time.Time) ([]TimeArtifact, error)
}

func (s *stubTimeSource) ArtifactsInWindow(ctx context.Context, from, to time.Time) ([]TimeArtifact, error) {
	return s.fn(ctx, from, to)
}

func newTimeTestHandlers(t *testing.T, src TimeSource) *TimeHandlers {
	t.Helper()
	return &TimeHandlers{
		Source: src,
		Limits: Limits{ListDefault: 50, ListMax: 200, EdgesDefault: 100, EdgesMax: 500, TimeWindowMaxDays: 365},
	}
}

func mountTimeRouter(h *TimeHandlers) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/api/time", h.GetTime)
	return r
}

func TestTimeHandler_GroupsByDay_SCN080_07(t *testing.T) {
	d1 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC)
	d3 := time.Date(2026, 5, 2, 18, 30, 0, 0, time.UTC)
	src := &stubTimeSource{
		fn: func(_ context.Context, from, to time.Time) ([]TimeArtifact, error) {
			if !from.Equal(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)) {
				t.Fatalf("from mismatch: %v", from)
			}
			if !to.Equal(time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)) {
				t.Fatalf("to mismatch: %v", to)
			}
			return []TimeArtifact{
				{ArtifactID: "A1", Title: "one", CapturedAt: d1},
				{ArtifactID: "A2", Title: "two", CapturedAt: d2},
				{ArtifactID: "A3", Title: "three", CapturedAt: d3},
			}, nil
		},
	}
	h := newTimeTestHandlers(t, src)
	rec := httptest.NewRecorder()
	url := "/api/time?from=2026-05-01T00:00:00Z&to=2026-05-08T00:00:00Z"
	mountTimeRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}
	var resp timeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Days) != 2 {
		t.Fatalf("want 2 day buckets, got %d", len(resp.Days))
	}
	if resp.Days[0].Date != "2026-05-01" || resp.Days[1].Date != "2026-05-02" {
		t.Fatalf("bucket dates wrong: %s, %s", resp.Days[0].Date, resp.Days[1].Date)
	}
	if len(resp.Days[1].Artifacts) != 2 {
		t.Fatalf("day 2026-05-02 should have 2 artifacts, got %d", len(resp.Days[1].Artifacts))
	}
}

func TestTimeHandler_WindowOver365Days_400_SCN080_12(t *testing.T) {
	h := newTimeTestHandlers(t, &stubTimeSource{
		fn: func(context.Context, time.Time, time.Time) ([]TimeArtifact, error) {
			t.Fatal("source must not be called when window is rejected")
			return nil, nil
		},
	})
	rec := httptest.NewRecorder()
	url := "/api/time?from=2024-01-01T00:00:00Z&to=2026-01-02T00:00:00Z"
	mountTimeRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	var env ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != CodeInvalidWindow {
		t.Fatalf("want code=%s, got %s", CodeInvalidWindow, env.Error.Code)
	}
	if !strings.Contains(env.Error.Message, "365") {
		t.Fatalf("want message to cite 365-day max, got %q", env.Error.Message)
	}
}

func TestTimeHandler_MissingTo_400_SCN080_13(t *testing.T) {
	h := newTimeTestHandlers(t, &stubTimeSource{
		fn: func(context.Context, time.Time, time.Time) ([]TimeArtifact, error) {
			t.Fatal("source must not be called when 'to' is missing")
			return nil, nil
		},
	})
	rec := httptest.NewRecorder()
	url := "/api/time?from=2026-05-01T00:00:00Z"
	mountTimeRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	var env ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != CodeMissingParam {
		t.Fatalf("want code=%s, got %s", CodeMissingParam, env.Error.Code)
	}
	if env.Error.Field != "to" {
		t.Fatalf("want field=to, got %q", env.Error.Field)
	}
}

func TestTimeHandler_MissingFrom_400(t *testing.T) {
	h := newTimeTestHandlers(t, &stubTimeSource{
		fn: func(context.Context, time.Time, time.Time) ([]TimeArtifact, error) {
			t.Fatal("source must not be called when 'from' is missing")
			return nil, nil
		},
	})
	rec := httptest.NewRecorder()
	mountTimeRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/time?to=2026-05-08T00:00:00Z", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	var env ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != CodeMissingParam || env.Error.Field != "from" {
		t.Fatalf("want missing_param/from, got %+v", env.Error)
	}
}

func TestTimeHandler_MalformedFrom_400(t *testing.T) {
	h := newTimeTestHandlers(t, &stubTimeSource{
		fn: func(context.Context, time.Time, time.Time) ([]TimeArtifact, error) {
			t.Fatal("source must not be called when 'from' is malformed")
			return nil, nil
		},
	})
	rec := httptest.NewRecorder()
	url := "/api/time?from=yesterday&to=2026-05-08T00:00:00Z"
	mountTimeRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestTimeHandler_WindowInverted_400(t *testing.T) {
	h := newTimeTestHandlers(t, &stubTimeSource{
		fn: func(context.Context, time.Time, time.Time) ([]TimeArtifact, error) {
			t.Fatal("source must not be called when window is inverted")
			return nil, nil
		},
	})
	rec := httptest.NewRecorder()
	url := "/api/time?from=2026-05-08T00:00:00Z&to=2026-05-01T00:00:00Z"
	mountTimeRouter(h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	var env ErrorEnvelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Error.Code != CodeInvalidWindow {
		t.Fatalf("want invalid_window, got %s", env.Error.Code)
	}
}

func TestGroupByDayUTC_Empty(t *testing.T) {
	out := groupByDayUTC(nil)
	if out == nil || len(out) != 0 {
		t.Fatalf("want non-nil empty slice, got %v", out)
	}
}
