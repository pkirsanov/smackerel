package keep

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// fakeNats is a test double for KeepNatsClient. Per-subject reply functions
// allow each test to script the sidecar's response; recorded calls allow
// adversarial assertions (e.g. that a failed handshake emits ZERO
// keep.sync.request publishes — SCN-059-019).
type fakeNats struct {
	requests   []fakeReq
	publishes  []fakePub
	replyFn    func(subject string, data []byte) ([]byte, error)
	publishErr error
}

type fakeReq struct {
	subject string
	data    []byte
	timeout time.Duration
}

type fakePub struct {
	subject string
	data    []byte
}

func (f *fakeNats) Publish(ctx context.Context, subject string, data []byte) error {
	f.publishes = append(f.publishes, fakePub{subject: subject, data: append([]byte(nil), data...)})
	return f.publishErr
}

func (f *fakeNats) Request(ctx context.Context, subject string, data []byte, timeout time.Duration) ([]byte, error) {
	f.requests = append(f.requests, fakeReq{subject: subject, data: append([]byte(nil), data...), timeout: timeout})
	if f.replyFn == nil {
		return nil, fmt.Errorf("fakeNats: no replyFn configured for %s", subject)
	}
	return f.replyFn(subject, data)
}

// --- SCN-059-019: handshake covers ---

func TestConnectPublishesHandshakeAndSurfacesSidecarErrorVerbatim(t *testing.T) {
	t.Setenv("KEEP_GOOGLE_EMAIL", "user@example.com")
	// Sidecar replies fail-loud with the canonical missing-secret error string.
	const sidecarErr = "KEEP_GOOGLE_APP_PASSWORD is required"
	fn := &fakeNats{}
	fn.replyFn = func(subject string, data []byte) ([]byte, error) {
		if subject != gkeepHandshakeSubject {
			t.Fatalf("unexpected subject %q before handshake completed", subject)
		}
		errStr := sidecarErr
		resp := KeepHandshakeResponse{Status: "error", Error: &errStr, SchemaVersion: gkeepSchemaVersion}
		return json.Marshal(resp)
	}
	c := New("google-keep")
	c.SetNatsClient(fn)
	err := c.Connect(context.Background(), testConnectorConfig("", "gkeepapi", true, true))
	if err == nil {
		t.Fatalf("expected fail-loud Connect error from sidecar handshake")
	}
	if !strings.Contains(err.Error(), sidecarErr) {
		t.Fatalf("Connect error %q does not contain sidecar error verbatim %q", err.Error(), sidecarErr)
	}
	if len(fn.requests) != 1 || fn.requests[0].subject != gkeepHandshakeSubject {
		t.Fatalf("expected exactly one request on %s, got %+v", gkeepHandshakeSubject, fn.requests)
	}
	for _, r := range fn.requests {
		if r.subject == gkeepRequestSubject {
			t.Fatalf("SCN-059-019: failed handshake MUST emit zero keep.sync.request publishes; got %+v", fn.requests)
		}
	}
	if c.Health(context.Background()) != "error" {
		t.Errorf("health = %q, want error", c.Health(context.Background()))
	}
}

func TestConnectHandshakeOkProceeds(t *testing.T) {
	t.Setenv("KEEP_GOOGLE_EMAIL", "user@example.com")
	fn := &fakeNats{}
	fn.replyFn = func(subject string, data []byte) ([]byte, error) {
		resp := KeepHandshakeResponse{Status: "ok", SchemaVersion: gkeepSchemaVersion}
		return json.Marshal(resp)
	}
	c := New("google-keep")
	c.SetNatsClient(fn)
	if err := c.Connect(context.Background(), testConnectorConfig("", "gkeepapi", true, true)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if c.Health(context.Background()) != "healthy" {
		t.Errorf("health = %q, want healthy", c.Health(context.Background()))
	}
}

// TestConnectHandshakeRejectsSchemaVersionMismatch verifies that the
// handshake validates schema_version parity at Connect time, catching
// version mismatches early before they manifest as confusing sync failures.
// This mirrors the strict validation applied to sync responses via
// validateGkeepResponse (SCN-059-009) but for the handshake path.
func TestConnectHandshakeRejectsSchemaVersionMismatch(t *testing.T) {
	t.Setenv("KEEP_GOOGLE_EMAIL", "user@example.com")
	fn := &fakeNats{}
	const badVersion = gkeepSchemaVersion + 999
	fn.replyFn = func(subject string, data []byte) ([]byte, error) {
		// Return a valid-looking handshake response but with wrong schema_version.
		resp := KeepHandshakeResponse{Status: "ok", SchemaVersion: badVersion}
		return json.Marshal(resp)
	}
	c := New("google-keep")
	c.SetNatsClient(fn)
	err := c.Connect(context.Background(), testConnectorConfig("", "gkeepapi", true, true))
	if err == nil {
		t.Fatal("expected fail-loud Connect error due to schema_version mismatch")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Errorf("Connect error %q does not mention schema_version", err.Error())
	}
	if c.Health(context.Background()) != "error" {
		t.Errorf("health = %q, want error", c.Health(context.Background()))
	}
}

// --- SCN-059-006: sync request/reply covers ---

func TestSyncGkeepapiPublishesRequestAndDecodesResponse(t *testing.T) {
	t.Setenv("KEEP_GOOGLE_EMAIL", "user@example.com")
	fn := &fakeNats{}
	fn.replyFn = func(subject string, data []byte) ([]byte, error) {
		switch subject {
		case gkeepHandshakeSubject:
			return json.Marshal(KeepHandshakeResponse{Status: "ok", SchemaVersion: gkeepSchemaVersion})
		case gkeepRequestSubject:
			var req KeepSyncRequest
			if err := json.Unmarshal(data, &req); err != nil {
				return nil, err
			}
			if req.RequestID == "" {
				t.Fatalf("request_id must be non-empty")
			}
			notes := []GkeepNote{{
				NoteID:       "note-xyz",
				Title:        "Hello",
				TextContent:  "Body text that is plenty long to pass the minimum content length filter.",
				Color:        "DEFAULT",
				ModifiedUsec: 1_712_000_000_000_000,
				CreatedUsec:  1_711_900_000_000_000,
			}}
			return json.Marshal(KeepSyncResponse{
				Status:        "ok",
				Notes:         notes,
				Cursor:        "2026-04-01T00:00:00Z",
				SchemaVersion: gkeepSchemaVersion,
			})
		}
		return nil, fmt.Errorf("unexpected subject %s", subject)
	}
	c := New("google-keep")
	c.SetNatsClient(fn)
	if err := c.Connect(context.Background(), testConnectorConfig("", "gkeepapi", true, true)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %d, want 1", len(artifacts))
	}
	if cursor != "2026-04-01T00:00:00Z" {
		t.Errorf("cursor = %q, want advanced", cursor)
	}
	if got := fn.requests[len(fn.requests)-1].timeout; got != gkeepRequestTimeout {
		t.Errorf("request timeout = %s, want %s", got, gkeepRequestTimeout)
	}
	// Verify the artifact was also published on artifacts.process.
	foundPub := false
	for _, p := range fn.publishes {
		if p.subject == "artifacts.process" {
			foundPub = true
			break
		}
	}
	if !foundPub {
		t.Errorf("expected at least one publish on artifacts.process, got %+v", fn.publishes)
	}
}

func TestSyncGkeepapiPropagatesSidecarError(t *testing.T) {
	t.Setenv("KEEP_GOOGLE_EMAIL", "user@example.com")
	fn := &fakeNats{}
	fn.replyFn = func(subject string, data []byte) ([]byte, error) {
		if subject == gkeepHandshakeSubject {
			return json.Marshal(KeepHandshakeResponse{Status: "ok", SchemaVersion: gkeepSchemaVersion})
		}
		errStr := "gkeepapi authentication failed"
		return json.Marshal(KeepSyncResponse{Status: "error", Error: &errStr, SchemaVersion: gkeepSchemaVersion})
	}
	c := New("google-keep")
	c.SetNatsClient(fn)
	if err := c.Connect(context.Background(), testConnectorConfig("", "gkeepapi", true, true)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	_, _, _ = c.Sync(context.Background(), "")
	// Sync() swallows the per-mode error (logged) but increments error count;
	// we assert via observable health.
	if c.Health(context.Background()) == connector.HealthHealthy {
		t.Errorf("health should not be healthy after sidecar auth error")
	}
}

// --- request_id format ---

func TestNewRequestIDMatchesPattern(t *testing.T) {
	id := newRequestID()
	re := regexp.MustCompile(`^k-\d+-[0-9a-f]{6}$`)
	if !re.MatchString(id) {
		t.Fatalf("request id %q does not match pattern", id)
	}
}
