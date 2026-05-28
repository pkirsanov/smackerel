// Spec 058 Scope 1 — handler-level unit tests. Auth (RequireScope)
// integration tests run alongside the rest of the router_scope test
// suite under internal/api; this file covers handler-internal
// transport limits, JSON validation, and per-item outcomes.
package extension

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/ingest"
)

type recordingPublisher struct {
	mu        sync.Mutex
	published []connector.RawArtifact
	err       error
}

func (p *recordingPublisher) PublishRawArtifact(_ context.Context, a connector.RawArtifact) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return "", p.err
	}
	p.published = append(p.published, a)
	return fmt.Sprintf("art-%d", len(p.published)), nil
}

func (p *recordingPublisher) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.published)
}

func validCfg() config.ExtensionIngestConfig {
	return config.ExtensionIngestConfig{
		Enabled:                   true,
		MaxBatchItems:             256,
		MaxBodyBytes:              1 << 20, // 1 MiB
		DefaultDedupWindowSeconds: 1800,
		AcceptedContentTypes:      []string{"bookmark", "browser_history_visit"},
		RequiredTokenScope:        "extension:bookmarks,history",
	}
}

func newTestHandler(t *testing.T) (*Handler, *recordingPublisher) {
	t.Helper()
	pub := &recordingPublisher{}
	return NewHandler(validCfg(), pub, ingest.PassthroughDedupStore{}), pub
}

func withSession(req *http.Request) *http.Request {
	sess := auth.Session{UserID: "alice", Source: auth.SessionSourcePerUserToken}
	return req.WithContext(auth.WithSession(req.Context(), sess))
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func makeValidItem(contentType, deviceID, url, clientEventID string) connector.RawArtifact {
	return connector.RawArtifact{
		SourceID:    "browser-extension",
		SourceRef:   "ref-" + clientEventID,
		ContentType: contentType,
		Title:       "Example",
		URL:         url,
		CapturedAt:  time.Unix(1_700_000_000, 0).UTC(),
		Metadata: map[string]any{
			"source_device_id": deviceID,
			"client_event_id":  clientEventID,
		},
	}
}

func TestIngest_AcceptsValidBatch_AllAccepted(t *testing.T) {
	h, pub := newTestHandler(t)
	items := []connector.RawArtifact{
		makeValidItem("bookmark", "laptop", "https://a.example/1", "ce-1"),
		makeValidItem("bookmark", "laptop", "https://a.example/2", "ce-2"),
		makeValidItem("browser_history_visit", "laptop", "https://a.example/3", "ce-3"),
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/extension/ingest", bytes.NewReader(mustJSON(t, items)))
	req = withSession(req)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp ingestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 3 {
		t.Fatalf("expected 3 outcomes, got %d", len(resp.Items))
	}
	for i, o := range resp.Items {
		if o.Outcome != "accepted" || o.ArtifactID == "" {
			t.Errorf("item %d: expected accepted with artifact_id; got %+v", i, o)
		}
	}
	if pub.calls() != 3 {
		t.Fatalf("expected 3 publishes, got %d", pub.calls())
	}
}

func TestIngest_PerItemRejection_PreservesNeighbors(t *testing.T) {
	h, pub := newTestHandler(t)
	items := []connector.RawArtifact{
		makeValidItem("bookmark", "laptop", "https://a.example/1", "ce-1"),
		makeValidItem("not-allowed", "laptop", "https://a.example/2", "ce-2"),
		makeValidItem("bookmark", "laptop", "https://a.example/3", "ce-3"),
	}
	req := withSession(httptest.NewRequest(http.MethodPost, "/v1/x", bytes.NewReader(mustJSON(t, items))))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp ingestResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp.Items) != 3 {
		t.Fatalf("expected 3 outcomes, got %d", len(resp.Items))
	}
	if resp.Items[0].Outcome != "accepted" || resp.Items[2].Outcome != "accepted" {
		t.Errorf("neighbors should have been accepted; got %+v", resp.Items)
	}
	if resp.Items[1].Outcome != "rejected" || resp.Items[1].Error != "content_type_not_accepted" {
		t.Errorf("middle item should be rejected with content_type_not_accepted; got %+v", resp.Items[1])
	}
	if pub.calls() != 2 {
		t.Fatalf("expected 2 publishes (rejected item never published), got %d", pub.calls())
	}
}

func TestIngest_RejectsBatchOver256(t *testing.T) {
	h, pub := newTestHandler(t)
	items := make([]connector.RawArtifact, 257)
	for i := range items {
		items[i] = makeValidItem("bookmark", "laptop", fmt.Sprintf("https://a.example/%d", i), fmt.Sprintf("ce-%d", i))
	}
	req := withSession(httptest.NewRequest(http.MethodPost, "/v1/x", bytes.NewReader(mustJSON(t, items))))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
	}
	if pub.calls() != 0 {
		t.Fatalf("publisher MUST NOT be called on oversized batch; got %d calls", pub.calls())
	}
	if !strings.Contains(rec.Body.String(), "batch_too_large") {
		t.Fatalf("expected batch_too_large in body; got %s", rec.Body.String())
	}
}

func TestIngest_RejectsBodyOver1MiB(t *testing.T) {
	cfg := validCfg()
	cfg.MaxBodyBytes = 1024
	pub := &recordingPublisher{}
	h := NewHandler(cfg, pub, ingest.PassthroughDedupStore{})

	body := bytes.Repeat([]byte("a"), int(cfg.MaxBodyBytes)+1)
	req := withSession(httptest.NewRequest(http.MethodPost, "/v1/x", bytes.NewReader(body)))
	req.ContentLength = int64(len(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", rec.Code, rec.Body.String())
	}
	if pub.calls() != 0 {
		t.Fatalf("publisher MUST NOT be called on oversized body; got %d", pub.calls())
	}
}

func TestIngest_RejectsUnknownTopLevelField(t *testing.T) {
	h, _ := newTestHandler(t)
	body := []byte(`{"unexpected_top_level":"value"}`) // not an array → invalid_json
	req := withSession(httptest.NewRequest(http.MethodPost, "/v1/x", bytes.NewReader(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_json") {
		t.Fatalf("expected invalid_json in body; got %s", rec.Body.String())
	}
}

func TestIngest_RejectsMissingSession(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/x", bytes.NewReader([]byte(`[]`)))
	// No session attached.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestComputeBucket_BookmarkAlwaysZero(t *testing.T) {
	ts := time.Unix(1_700_000_000, 0).UTC()
	got := computeBucket("bookmark", ts, map[string]any{"dedup_window_seconds": 1800}, 1800)
	if got != 0 {
		t.Fatalf("expected bookmark bucket 0; got %d", got)
	}
	// 24h later still 0.
	got = computeBucket("bookmark", ts.Add(24*time.Hour), nil, 1800)
	if got != 0 {
		t.Fatalf("expected bookmark bucket 0 across 24h; got %d", got)
	}
}

func TestComputeBucket_HistoryUsesDefaultWindow(t *testing.T) {
	ts := time.Unix(1_700_000_000, 0).UTC()
	got := computeBucket("browser_history_visit", ts, nil, 1800)
	want := ts.Unix() / 1800
	if got != want {
		t.Fatalf("expected %d, got %d", want, got)
	}
}

func TestComputeBucket_HistoryRespectsMetadataOverrideWithinClamp(t *testing.T) {
	ts := time.Unix(1_700_000_000, 0).UTC()
	got := computeBucket("browser_history_visit", ts, map[string]any{"dedup_window_seconds": float64(60)}, 1800)
	want := ts.Unix() / 60
	if got != want {
		t.Fatalf("expected %d, got %d", want, got)
	}
}

func TestComputeBucket_HistoryIgnoresOutOfRangeOverride(t *testing.T) {
	ts := time.Unix(1_700_000_000, 0).UTC()
	got := computeBucket("browser_history_visit", ts, map[string]any{"dedup_window_seconds": float64(30)}, 1800)
	want := ts.Unix() / 1800 // falls back to default
	if got != want {
		t.Fatalf("expected default %d, got %d", want, got)
	}
}

func TestIngest_PublishFailureSurfacesAsRejection(t *testing.T) {
	pub := &recordingPublisher{err: fmt.Errorf("nats down")}
	h := NewHandler(validCfg(), pub, ingest.PassthroughDedupStore{})
	items := []connector.RawArtifact{makeValidItem("bookmark", "laptop", "https://a.example/1", "ce-1")}
	req := withSession(httptest.NewRequest(http.MethodPost, "/v1/x", bytes.NewReader(mustJSON(t, items))))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (per-item rejection), got %d", rec.Code)
	}
	var resp ingestResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp.Items) != 1 || resp.Items[0].Outcome != "rejected" || resp.Items[0].Error != "publish_failed" {
		t.Fatalf("expected publish_failed rejection; got %+v", resp.Items)
	}
}
