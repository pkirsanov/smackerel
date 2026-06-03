// Spec 027 scope 9 — T9-01, T9-02 list-my-annotations handler unit tests.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/auth"
)

// recordingListStore captures the args ListByActor was called with and
// returns a programmable response.
type recordingListStore struct {
	stubAnnotationStore
	gotActor string
	gotLimit int
	gotSince *time.Time
	resp     []annotation.Annotation
}

func (s *recordingListStore) ListByActor(_ context.Context, actorID string, limit int, since *time.Time) ([]annotation.Annotation, error) {
	s.gotActor = actorID
	s.gotLimit = limit
	s.gotSince = since
	return s.resp, nil
}

func ctxWithUser(uid string) context.Context {
	return auth.WithSession(context.Background(), auth.Session{
		UserID: uid,
		Source: auth.SessionSourcePerUserToken,
	})
}

// T9-01 — actor=me returns caller's events in reverse-chronological
// order across artifacts. The handler MUST resolve actor_id from the
// bearer subject and call ListByActor with that subject.
func TestListMyAnnotations_T9_01_ReturnsCallerEvents(t *testing.T) {
	now := time.Now()
	store := &recordingListStore{resp: []annotation.Annotation{
		{ID: "ann-2", ArtifactID: "art-b", ActorID: "alice", CreatedAt: now},
		{ID: "ann-1", ArtifactID: "art-a", ActorID: "alice", CreatedAt: now.Add(-time.Hour)},
	}}
	h := &AnnotationHandlers{Store: store}

	req := httptest.NewRequest(http.MethodGet, "/api/annotations?actor=me&limit=50", nil)
	req = req.WithContext(ctxWithUser("alice"))
	w := httptest.NewRecorder()
	h.ListMyAnnotations(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if store.gotActor != "alice" {
		t.Errorf("ListByActor actor = %q, want alice", store.gotActor)
	}
	if store.gotLimit != 50 {
		t.Errorf("ListByActor limit = %d, want 50", store.gotLimit)
	}
	var body struct {
		ActorID     string                  `json:"actor_id"`
		Limit       int                     `json:"limit"`
		Annotations []annotation.Annotation `json:"annotations"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ActorID != "alice" {
		t.Errorf("response actor_id = %q, want alice", body.ActorID)
	}
	if len(body.Annotations) != 2 || body.Annotations[0].ID != "ann-2" {
		t.Errorf("expected ann-2 first (newest); got %+v", body.Annotations)
	}
}

// T9-02 — actor=<other> is rejected with 403 in single-tenant mode.
func TestListMyAnnotations_T9_02_ForbidsOtherActor(t *testing.T) {
	store := &recordingListStore{}
	h := &AnnotationHandlers{Store: store}

	req := httptest.NewRequest(http.MethodGet, "/api/annotations?actor=bob&limit=10", nil)
	req = req.WithContext(ctxWithUser("alice"))
	w := httptest.NewRecorder()
	h.ListMyAnnotations(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	if store.gotActor != "" {
		t.Errorf("ListByActor should NOT have been called; gotActor=%q", store.gotActor)
	}
}

// Adversarial — missing actor parameter is a 400, not a silent default.
func TestListMyAnnotations_MissingActor_400(t *testing.T) {
	h := &AnnotationHandlers{Store: &stubAnnotationStore{}}
	req := httptest.NewRequest(http.MethodGet, "/api/annotations?limit=10", nil)
	req = req.WithContext(ctxWithUser("alice"))
	w := httptest.NewRecorder()
	h.ListMyAnnotations(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// Adversarial — limit out of range is a 400.
func TestListMyAnnotations_LimitOutOfRange_400(t *testing.T) {
	h := &AnnotationHandlers{Store: &stubAnnotationStore{}}
	for _, lim := range []string{"0", "201", "-1", "notanumber", ""} {
		req := httptest.NewRequest(http.MethodGet, "/api/annotations?actor=me&limit="+lim, nil)
		req = req.WithContext(ctxWithUser("alice"))
		w := httptest.NewRecorder()
		h.ListMyAnnotations(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("limit=%q: status = %d, want 400", lim, w.Code)
		}
	}
}

// Adversarial — no authenticated subject (middleware misconfigured) →
// 403, never silent 200 with empty actor_id.
func TestListMyAnnotations_NoSubject_403(t *testing.T) {
	h := &AnnotationHandlers{Store: &stubAnnotationStore{}}
	req := httptest.NewRequest(http.MethodGet, "/api/annotations?actor=me&limit=10", nil)
	// No session attached.
	w := httptest.NewRecorder()
	h.ListMyAnnotations(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}
