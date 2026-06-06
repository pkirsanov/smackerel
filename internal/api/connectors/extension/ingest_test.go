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

// TestIngest_RejectsItemWithoutOwner is the BUG-058-DEDUP-KEY-OWNER-ISOLATION
// fail-loud guard: a request whose session carries an EMPTY UserID passes the
// ServeHTTP session-presence check but MUST be rejected per-item before the
// dedup keyer runs — an empty owner namespace would re-open the cross-tenant
// collapse this bug fixes. There is no fallback owner id (smackerel-no-defaults).
func TestIngest_RejectsItemWithoutOwner(t *testing.T) {
	h, pub := newTestHandler(t)
	items := []connector.RawArtifact{
		makeValidItem("bookmark", "laptop", "https://a.example/1", "ce-1"),
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/extension/ingest", bytes.NewReader(mustJSON(t, items)))
	// Session present (passes the ServeHTTP 401 guard) but UserID empty.
	req = req.WithContext(auth.WithSession(req.Context(), auth.Session{UserID: "", Source: auth.SessionSourcePerUserToken}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (per-item rejection, not a transport error), got %d", rec.Code)
	}
	var resp ingestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(resp.Items))
	}
	if resp.Items[0].Outcome != "rejected" || resp.Items[0].Error != "owner_required" {
		t.Fatalf("expected rejected/owner_required; got %+v", resp.Items[0])
	}
	if pub.calls() != 0 {
		t.Fatalf("empty-owner item MUST NOT be published; got %d publishes", pub.calls())
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

// ---------------------------------------------------------------------------
// Spec 058 — Chaos Sweep Round 18 (2026-06-06) adversarial probes.
//
// These probes target adversarial / boundary / malformed-input /
// concurrency behavior of the ingest handler that the original Scope 1
// unit suite did not cover. The source_device_id validation probes are
// the red-green proof for finding F1 (server failed to enforce the
// spec 058 design §2.3 source_device_id charset contract at the trust
// boundary); the remaining probes confirm robustness of the existing
// type-handling, batch-structural, streaming-cap, and concurrency
// paths and are permanent regression guards.
// ---------------------------------------------------------------------------

// TestIngest_RejectsMalformedSourceDeviceID is the adversarial twin for
// finding F1. Spec 058 design §2.3 constrains source_device_id to the
// charset [a-z0-9-]; the server is the trust boundary and MUST reject
// values that violate the contract rather than letting them flow into
// the dedup-key preimage (ComputeDedupKey) and the admin devices view.
// Before the fix these inputs were accepted (only the non-empty check
// existed), so this test fails RED on the unpatched handler.
func TestIngest_RejectsMalformedSourceDeviceID(t *testing.T) {
	bad := []struct {
		name     string
		deviceID string
	}{
		{"null_byte", "lap\x00top"},
		{"separator_injection", "a\x00bookmark\x00b"},
		{"uppercase", "Laptop"},
		{"space", "my laptop"},
		{"tab", "lap\ttop"},
		{"newline", "lap\ntop"},
		{"unicode", "läptop"},
		{"slash", "a/b"},
		{"dot", "a.b"},
		{"underscore", "lap_top"},
		{"over_64_chars", strings.Repeat("a", 65)},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			h, pub := newTestHandler(t)
			items := []connector.RawArtifact{
				makeValidItem("bookmark", "laptop", "https://a.example/1", "ce-1"),
				makeValidItem("bookmark", tc.deviceID, "https://a.example/2", "ce-2"),
				makeValidItem("bookmark", "work-desktop", "https://a.example/3", "ce-3"),
			}
			req := withSession(httptest.NewRequest(http.MethodPost, "/v1/x", bytes.NewReader(mustJSON(t, items))))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
			}
			var resp ingestResponse
			_ = json.NewDecoder(rec.Body).Decode(&resp)
			if len(resp.Items) != 3 {
				t.Fatalf("expected 3 outcomes, got %d", len(resp.Items))
			}
			if resp.Items[1].Outcome != "rejected" || resp.Items[1].Error != "metadata.source_device_id_invalid" {
				t.Fatalf("device id %q MUST be rejected with metadata.source_device_id_invalid; got %+v", tc.deviceID, resp.Items[1])
			}
			// Adversarial neighbor-preservation: a malformed device id
			// MUST NOT poison the valid items in the same batch.
			if resp.Items[0].Outcome != "accepted" || resp.Items[2].Outcome != "accepted" {
				t.Fatalf("valid neighbors MUST be accepted; got %+v", resp.Items)
			}
			if pub.calls() != 2 {
				t.Fatalf("malformed item MUST NOT be published; expected 2 publishes, got %d", pub.calls())
			}
		})
	}
}

// TestIngest_AcceptsValidSourceDeviceIDForms is the non-tautology guard
// for finding F1: the new validator MUST still accept every legitimate
// device-id form, including the design's own auto-<uuidv4> fallback
// (41 chars) and an id exactly at the 64-char length bound.
func TestIngest_AcceptsValidSourceDeviceIDForms(t *testing.T) {
	valid := []string{
		"laptop",
		"work-desktop",
		"phone2",
		"auto-550e8400-e29b-41d4-a716-446655440000", // auto-<uuidv4>, 41 chars
		strings.Repeat("a", 64),                     // exactly at the length bound
	}
	for _, dev := range valid {
		t.Run(dev, func(t *testing.T) {
			h, pub := newTestHandler(t)
			items := []connector.RawArtifact{makeValidItem("bookmark", dev, "https://a.example/1", "ce-1")}
			req := withSession(httptest.NewRequest(http.MethodPost, "/v1/x", bytes.NewReader(mustJSON(t, items))))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}
			var resp ingestResponse
			_ = json.NewDecoder(rec.Body).Decode(&resp)
			if len(resp.Items) != 1 || resp.Items[0].Outcome != "accepted" {
				t.Fatalf("valid device id %q MUST be accepted; got %+v", dev, resp.Items)
			}
			if pub.calls() != 1 {
				t.Fatalf("expected 1 publish for %q, got %d", dev, pub.calls())
			}
		})
	}
}

// TestIngest_MetadataTypeConfusion_GracefulRejection proves
// readMetadataString uses a comma-ok type assertion and never panics
// when a client sends source_device_id / client_event_id as a number,
// bool, array, object, or null. Non-string values are treated as
// absent, so the item is cleanly rejected as missing device id.
func TestIngest_MetadataTypeConfusion_GracefulRejection(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{"number", float64(12345)},
		{"bool", true},
		{"array", []any{"a", "b"}},
		{"object", map[string]any{"x": float64(1)}},
		{"null", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, pub := newTestHandler(t)
			item := connector.RawArtifact{
				SourceID:    "browser-extension",
				ContentType: "bookmark",
				URL:         "https://a.example/1",
				CapturedAt:  time.Unix(1_700_000_000, 0).UTC(),
				Metadata: map[string]any{
					"source_device_id": tc.value,
					"client_event_id":  tc.value,
				},
			}
			req := withSession(httptest.NewRequest(http.MethodPost, "/v1/x", bytes.NewReader(mustJSON(t, []connector.RawArtifact{item}))))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req) // MUST NOT panic
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}
			var resp ingestResponse
			_ = json.NewDecoder(rec.Body).Decode(&resp)
			if len(resp.Items) != 1 || resp.Items[0].Outcome != "rejected" || resp.Items[0].Error != "metadata.source_device_id_required" {
				t.Fatalf("non-string device id MUST be treated as missing; got %+v", resp.Items)
			}
			if pub.calls() != 0 {
				t.Fatalf("nothing should be published; got %d", pub.calls())
			}
		})
	}
}

// TestIngest_NullArrayElement_RejectedPerItem_NeighborsPreserved proves
// a JSON null in the items array decodes to a zero-value RawArtifact
// and is rejected per-item (source_id_invalid) without crashing or
// short-circuiting the rest of the batch.
func TestIngest_NullArrayElement_RejectedPerItem_NeighborsPreserved(t *testing.T) {
	neighbor := mustJSON(t, makeValidItem("bookmark", "laptop", "https://a.example/2", "ce-2"))
	body := []byte("[null," + string(neighbor) + "]")
	h, pub := newTestHandler(t)
	req := withSession(httptest.NewRequest(http.MethodPost, "/v1/x", bytes.NewReader(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp ingestResponse
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(resp.Items))
	}
	if resp.Items[0].Outcome != "rejected" || resp.Items[0].Error != "source_id_invalid" {
		t.Fatalf("null element MUST be rejected source_id_invalid; got %+v", resp.Items[0])
	}
	if resp.Items[1].Outcome != "accepted" {
		t.Fatalf("valid neighbor MUST be accepted; got %+v", resp.Items[1])
	}
	if pub.calls() != 1 {
		t.Fatalf("expected 1 publish, got %d", pub.calls())
	}
}

// TestIngest_UnknownFieldInItem_FailsWholeBatch pins the sharp edge that
// DisallowUnknownFields makes an unknown field inside ANY array item a
// transport-level (whole-batch) 400 invalid_json — NOT a per-item
// rejection. A future refactor that silently downgrades structural
// strictness would flip this to a 200 and fail here.
func TestIngest_UnknownFieldInItem_FailsWholeBatch(t *testing.T) {
	body := []byte(`[{"source_id":"browser-extension","content_type":"bookmark","url":"https://a.example/1","captured_at":"2023-11-14T22:13:20Z","evil_unknown_field":true}]`)
	h, pub := newTestHandler(t)
	req := withSession(httptest.NewRequest(http.MethodPost, "/v1/x", bytes.NewReader(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 invalid_json, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_json") {
		t.Fatalf("expected invalid_json; got %s", rec.Body.String())
	}
	if pub.calls() != 0 {
		t.Fatalf("nothing published on structural failure; got %d", pub.calls())
	}
}

// TestIngest_BodyOverCap_UnknownContentLength_Returns413 exercises the
// MaxBytesReader defense-in-depth path. The existing oversize-body test
// sets Content-Length and trips the cheap pre-check; this probe leaves
// Content-Length unset (-1, the chunked/streaming case) so the body cap
// is enforced mid-parse and the handler MUST map *http.MaxBytesError to
// 413 body_too_large (NOT 400 invalid_json).
func TestIngest_BodyOverCap_UnknownContentLength_Returns413(t *testing.T) {
	cfg := validCfg()
	cfg.MaxBodyBytes = 512
	pub := &recordingPublisher{}
	h := NewHandler(cfg, pub, ingest.PassthroughDedupStore{})

	// > cap bytes with the closing ']' only at the very end, so the
	// decoder is forced to read past the cap before it can finish.
	big := append([]byte("["), bytes.Repeat([]byte(`{"source_id":"browser-extension"},`), 200)...)
	big = append(big, ']')
	req := withSession(httptest.NewRequest(http.MethodPost, "/v1/x", bytes.NewReader(big)))
	req.ContentLength = -1 // unknown length (chunked transfer)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 body_too_large via MaxBytesReader, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "body_too_large") {
		t.Fatalf("expected body_too_large; got %s", rec.Body.String())
	}
	if pub.calls() != 0 {
		t.Fatalf("nothing published; got %d", pub.calls())
	}
}

// TestIngest_ConcurrentRequests_NoRace runs many concurrent POSTs
// through a single handler. The accepted-types map is read-only after
// construction and PassthroughDedupStore is stateless, so the handler
// MUST be race-free (run under -race) and publish every item exactly
// once.
func TestIngest_ConcurrentRequests_NoRace(t *testing.T) {
	h, pub := newTestHandler(t)
	const workers = 16
	const perWorker = 8
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				item := makeValidItem("bookmark", "laptop",
					fmt.Sprintf("https://a.example/%d-%d", w, i),
					fmt.Sprintf("ce-%d-%d", w, i))
				req := withSession(httptest.NewRequest(http.MethodPost, "/v1/x", bytes.NewReader(mustJSON(t, []connector.RawArtifact{item}))))
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				if rec.Code != http.StatusOK {
					t.Errorf("worker %d item %d: expected 200, got %d", w, i, rec.Code)
				}
			}
		}(w)
	}
	wg.Wait()
	if pub.calls() != workers*perWorker {
		t.Fatalf("expected %d publishes, got %d", workers*perWorker, pub.calls())
	}
}
