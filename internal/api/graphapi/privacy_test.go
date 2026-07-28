package graphapi

// privacy_test.go — BUG-080-001 SCOPE-02 / T080-PRIVACY-NOSTORE.
//
// Proves the second clause of the SCOPE-02 auth/privacy DoD row: "no
// sensitive graph material is durably cached". scopes.md SCOPE-02
// "Security And Privacy" states the contract as "Authenticated
// responses and cursors use private/no-store semantics and never enter
// durable browser storage."
//
// The whole graphapi package writes a response through EXACTLY two
// functions — writeJSON (success) and WriteError (every typed error;
// WriteAPIError and GraphCapability.WriteDisabled both delegate to it).
// That is mechanically checkable: `grep -n 'WriteHeader('` over the
// non-test package returns exactly errors.go (WriteError) and topics.go
// (writeJSON). Stamping those two therefore covers every family
// (topics, people, places, time, edges), list AND detail, success AND
// error AND the disabled 503 — with no per-handler duplication.
//
// These tests are adversarial: each one fails if SetPrivateNoStore is
// removed from either writer, and the header-freeze test additionally
// fails if the call is merely MOVED after WriteHeader (where net/http
// silently discards it in production).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// frozenHeaderWriter mimics net/http's real behavior: once WriteHeader
// is called the header map is committed to the wire and any later
// mutation is discarded. httptest.ResponseRecorder is more forgiving
// than the real server, so this writer is what makes the
// "set BEFORE WriteHeader" assertion genuine rather than cosmetic.
type frozenHeaderWriter struct {
	hdr        http.Header
	committed  http.Header
	statusCode int
	wroteOnce  bool
}

func newFrozenHeaderWriter() *frozenHeaderWriter {
	return &frozenHeaderWriter{hdr: http.Header{}}
}

func (f *frozenHeaderWriter) Header() http.Header { return f.hdr }

func (f *frozenHeaderWriter) WriteHeader(status int) {
	if f.wroteOnce {
		return
	}
	f.wroteOnce = true
	f.statusCode = status
	f.committed = f.hdr.Clone()
}

func (f *frozenHeaderWriter) Write(p []byte) (int, error) {
	if !f.wroteOnce {
		f.WriteHeader(http.StatusOK)
	}
	return len(p), nil
}

// assertPrivateNoStore fails unless the COMMITTED headers carry the
// exact private/no-store directive set.
func assertPrivateNoStore(t *testing.T, label string, committed http.Header) {
	t.Helper()
	got := committed.Get(CacheControlHeader)
	if got != CacheControlPrivateNoStore {
		t.Errorf("%s: %s = %q, want %q — private graph content MUST NOT be durably cacheable (scopes.md SCOPE-02 Security And Privacy)",
			label, CacheControlHeader, got, CacheControlPrivateNoStore)
	}
}

// TestWriteError_SetsPrivateNoStoreBeforeCommit_T080_PRIVACY_NOSTORE proves the
// error writer stamps the directive, and that it does so BEFORE the status
// line is committed. Moving SetPrivateNoStore below WriteHeader fails here.
func TestWriteError_SetsPrivateNoStoreBeforeCommit_T080_PRIVACY_NOSTORE(t *testing.T) {
	w := newFrozenHeaderWriter()
	WriteError(w, http.StatusBadRequest, CodeInvalidCursor, "cursor", "malformed cursor")

	if !w.wroteOnce {
		t.Fatal("WriteError never committed a status line")
	}
	if w.statusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d — the privacy change MUST NOT alter status codes", w.statusCode, http.StatusBadRequest)
	}
	assertPrivateNoStore(t, "WriteError", w.committed)
}

// TestWriteAPIError_SetsPrivateNoStoreOnEveryTypedError_T080_PRIVACY_NOSTORE
// walks the CLOSED set of typed graph errors. A 401/403/404/500/503 body is
// itself an existence hint about private graph content, so every one of them
// carries the contract — including the fail-soft capability_disabled 503 and
// the defensive nil-APIError path.
func TestWriteAPIError_SetsPrivateNoStoreOnEveryTypedError_T080_PRIVACY_NOSTORE(t *testing.T) {
	typed := map[string]*APIError{
		"unauthenticated":     ErrUnauthenticated,
		"missing_scope":       ErrMissingScope,
		"malformed_cursor":    ErrMalformedCursor,
		"limit_exceeded":      ErrLimitExceeded,
		"time_range_too_big":  ErrTimeRangeTooLarge,
		"missing_param":       ErrMissingParam,
		"unknown_source_kind": ErrUnknownSourceKind,
		"schema_error":        ErrSchemaError,
		"store_unavailable":   ErrStoreUnavailable,
		"capability_disabled": ErrCapabilityDisabled,
		"nil_defensive":       nil,
	}

	for label, apiErr := range typed {
		t.Run(label, func(t *testing.T) {
			w := newFrozenHeaderWriter()
			WriteAPIError(w, apiErr)
			if !w.wroteOnce {
				t.Fatalf("%s: WriteAPIError never committed a status line", label)
			}
			assertPrivateNoStore(t, "WriteAPIError/"+label, w.committed)
		})
	}

	// GraphCapability.WriteDisabled is the fail-soft responder mounted on
	// EVERY graph path when the capability is disabled. It must carry the
	// contract too — a 503 that says "this graph exists but is off" is
	// exactly the kind of existence metadata that must not be cached.
	disabled := NewGraphCapability(Config{CursorSecretEnv: ""})
	if !disabled.Disabled() {
		t.Fatal("fixture error: capability with no configured secret env must resolve DISABLED")
	}
	w := newFrozenHeaderWriter()
	disabled.WriteDisabled(w)
	if w.statusCode != http.StatusServiceUnavailable {
		t.Fatalf("WriteDisabled status = %d, want 503", w.statusCode)
	}
	assertPrivateNoStore(t, "GraphCapability.WriteDisabled", w.committed)
}

// TestTopicsHandlers_EverySuccessAndErrorResponseIsPrivateNoStore_T080_PRIVACY_NOSTORE
// exercises REAL handlers through a real chi router so the success path is
// proven end-to-end through writeJSON, not just at the helper. It covers a
// list 200, a detail 200, a not-found error, and a malformed-cursor error.
func TestTopicsHandlers_EverySuccessAndErrorResponseIsPrivateNoStore_T080_PRIVACY_NOSTORE(t *testing.T) {
	src := &stubTopicsSource{
		listFn: func(_ context.Context, _, _ int) ([]TopicRow, bool, error) {
			// hasNext=true forces a nextCursor into the body: the cursor is
			// itself sensitive material the contract must cover.
			return []TopicRow{{ID: "T1", Label: "alpha", LinkedArtifactCount: 3}}, true, nil
		},
		getFn: func(_ context.Context, id string) (*TopicDetail, error) {
			if id == "missing" {
				return nil, ErrTopicNotFound
			}
			return &TopicDetail{ID: id, Label: "alpha"}, nil
		},
	}
	router := mountTopicsRouter(newTopicsTestHandlers(t, src))

	cases := []struct {
		name, path string
		wantStatus int
	}{
		{"list_success_with_cursor", "/api/topics", http.StatusOK},
		{"detail_success", "/api/topics/T1", http.StatusOK},
		{"detail_not_found", "/api/topics/missing", http.StatusNotFound},
		{"malformed_cursor", "/api/topics?cursor=not-a-real-cursor", http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s) — the privacy change MUST NOT alter status codes or bodies",
					rec.Code, tc.wantStatus, rec.Body.String())
			}
			// rec.Result().Header is the snapshot taken at WriteHeader time,
			// so this also proves the directive was set before commit.
			assertPrivateNoStore(t, "GET "+tc.path, rec.Result().Header)
		})
	}
}
