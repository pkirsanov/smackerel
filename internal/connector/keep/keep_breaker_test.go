package keep

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/metrics"
)

// canonicalSyncResponseFixture returns a known-good KeepSyncResponse
// used by TestValidateGkeepResponseAcceptsCanonicalFixtureAndRejectsEveryMutation
// (SCN-059-009). Every per-mutation sub-test starts from this canonical
// value and mutates a single field; the assertion is mechanical so a
// future schema field that lands without validation will surface as a
// missing mutation row.
func canonicalSyncResponseFixture() KeepSyncResponse {
	return KeepSyncResponse{
		Status:        "ok",
		SchemaVersion: gkeepSchemaVersion,
		Cursor:        "2026-04-01T00:00:00Z",
		Notes: []GkeepNote{{
			NoteID:       "note-canonical-1",
			Title:        "Hello",
			TextContent:  "body",
			ModifiedUsec: 1_712_000_000_000_000,
			CreatedUsec:  1_711_900_000_000_000,
		}},
	}
}

func TestValidateGkeepResponseAcceptsCanonicalFixtureAndRejectsEveryMutation(t *testing.T) {
	// Canonical: validates clean.
	canonical := canonicalSyncResponseFixture()
	if err := validateGkeepResponse(&canonical); err != nil {
		t.Fatalf("canonical fixture must validate clean, got %v", err)
	}

	// Per-mutation matrix. Each entry mutates exactly one field of the
	// canonical fixture; every entry MUST produce a non-nil error.
	emptyErr := ""
	driftErr := "schema mismatch"
	cases := []struct {
		name   string
		mutate func(*KeepSyncResponse)
	}{
		{"wrong_schema_version_zero", func(r *KeepSyncResponse) { r.SchemaVersion = 0 }},
		{"wrong_schema_version_higher", func(r *KeepSyncResponse) { r.SchemaVersion = 2 }},
		{"invalid_status", func(r *KeepSyncResponse) { r.Status = "weird" }},
		{"empty_status", func(r *KeepSyncResponse) { r.Status = "" }},
		{"ok_with_nonempty_error", func(r *KeepSyncResponse) {
			s := "bogus error on ok"
			r.Error = &s
		}},
		{"error_status_with_nil_error", func(r *KeepSyncResponse) {
			r.Status = "error"
			r.Notes = nil
			r.Error = nil
		}},
		{"error_status_with_empty_error", func(r *KeepSyncResponse) {
			r.Status = "error"
			r.Notes = nil
			r.Error = &emptyErr
		}},
		{"error_status_with_notes_present", func(r *KeepSyncResponse) {
			r.Status = "error"
			r.Error = &driftErr
		}},
		{"ok_note_missing_note_id", func(r *KeepSyncResponse) {
			r.Notes[0].NoteID = ""
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := canonicalSyncResponseFixture()
			tc.mutate(&resp)
			if err := validateGkeepResponse(&resp); err == nil {
				t.Fatalf("mutation %q must be rejected, got nil error", tc.name)
			}
		})
	}

	// Adversarial nil receiver.
	if err := validateGkeepResponse(nil); err == nil {
		t.Fatalf("nil response must be rejected")
	}
}

// makeDriftReply returns a fakeNats replyFn that replies "ok" to the
// handshake and serves the supplied sync response for every other
// subject. Used to drive the breaker FSM in controlled steps.
func driftReplyFn(t *testing.T, syncReply func() ([]byte, error)) func(string, []byte) ([]byte, error) {
	t.Helper()
	return func(subject string, _ []byte) ([]byte, error) {
		if subject == gkeepHandshakeSubject {
			return json.Marshal(KeepHandshakeResponse{Status: "ok", SchemaVersion: gkeepSchemaVersion})
		}
		return syncReply()
	}
}

func driftAckCfg(token string) map[string]interface{} {
	return map[string]interface{}{
		"sync_mode":            "gkeepapi",
		"import_dir":           "",
		"gkeep_enabled":        true,
		"warning_acknowledged": true,
		"drift_ack_token":      token,
	}
}

func TestDriftBreakerTransitionsClosedTrippingOpenAndResetsOnTokenRotation(t *testing.T) {
	t.Setenv("KEEP_GOOGLE_EMAIL", "user@example.com")
	// Sidecar always replies with a schema-drift response (wrong
	// schema_version). Every Sync() must therefore record a failure.
	fn := &fakeNats{}
	fn.replyFn = driftReplyFn(t, func() ([]byte, error) {
		return json.Marshal(KeepSyncResponse{
			Status:        "ok",
			SchemaVersion: gkeepSchemaVersion + 99,
			Notes:         nil,
		})
	})

	c := New("google-keep")
	c.SetNatsClient(fn)
	cfg := connectorCfgFromMap(driftAckCfg("token-a"))
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// 1st failure: TRIPPING, driftFailures=1.
	_, _, _, err := c.syncGkeepapi(context.Background(), "")
	if err == nil {
		t.Fatalf("expected drift error on 1st sync")
	}
	assertBreaker(t, c, breakerTripping, 1)

	// 2nd, 3rd: still TRIPPING.
	_, _, _, _ = c.syncGkeepapi(context.Background(), "")
	assertBreaker(t, c, breakerTripping, 2)
	_, _, _, _ = c.syncGkeepapi(context.Background(), "")
	assertBreaker(t, c, breakerTripping, 3)

	// 4th: OPEN.
	_, _, _, _ = c.syncGkeepapi(context.Background(), "")
	assertBreaker(t, c, breakerOpen, 4)

	// Open: any subsequent Sync returns ErrBreakerOpen and emits ZERO
	// further NATS Request calls (adversarial: this is the regression
	// guard for TestOpenBreakerSkipsNatsPublish below).
	priorRequests := len(fn.requests)
	priorPublishes := len(fn.publishes)
	_, _, _, err = c.syncGkeepapi(context.Background(), "")
	if !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("expected ErrBreakerOpen while open, got %v", err)
	}
	if len(fn.requests) != priorRequests {
		t.Fatalf("OPEN breaker must not emit NATS Request calls; before=%d after=%d", priorRequests, len(fn.requests))
	}
	if len(fn.publishes) != priorPublishes {
		t.Fatalf("OPEN breaker must not emit NATS Publish calls; before=%d after=%d", priorPublishes, len(fn.publishes))
	}

	// Token rotation via Connect() resets the breaker.
	cfg2 := connectorCfgFromMap(driftAckCfg("token-b"))
	if err := c.Connect(context.Background(), cfg2); err != nil {
		t.Fatalf("Connect with rotated token: %v", err)
	}
	assertBreaker(t, c, breakerClosed, 0)
}

func TestReconnectWithSameAckTokenDoesNotClearOpenBreaker(t *testing.T) {
	t.Setenv("KEEP_GOOGLE_EMAIL", "user@example.com")
	fn := &fakeNats{}
	fn.replyFn = driftReplyFn(t, func() ([]byte, error) {
		return json.Marshal(KeepSyncResponse{Status: "ok", SchemaVersion: 99, Notes: nil})
	})
	c := New("google-keep")
	c.SetNatsClient(fn)
	cfg := connectorCfgFromMap(driftAckCfg("same"))
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	for i := 0; i < breakerOpenThreshold; i++ {
		_, _, _, _ = c.syncGkeepapi(context.Background(), "")
	}
	assertBreaker(t, c, breakerOpen, breakerOpenThreshold)

	// Reconnect with the SAME ack token must NOT clear the breaker.
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect re-call: %v", err)
	}
	assertBreaker(t, c, breakerOpen, breakerOpenThreshold)
}

func TestSidecarAuthErrorDoesNotIncrementDriftFailures(t *testing.T) {
	t.Setenv("KEEP_GOOGLE_EMAIL", "user@example.com")
	fn := &fakeNats{}
	fn.replyFn = driftReplyFn(t, func() ([]byte, error) {
		errStr := "gkeepapi authentication failed: invalid credentials"
		return json.Marshal(KeepSyncResponse{Status: "error", Error: &errStr, SchemaVersion: gkeepSchemaVersion})
	})
	c := New("google-keep")
	c.SetNatsClient(fn)
	cfg := connectorCfgFromMap(driftAckCfg(""))
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	for i := 0; i < breakerOpenThreshold+2; i++ {
		_, _, _, err := c.syncGkeepapi(context.Background(), "")
		if err == nil {
			t.Fatalf("expected auth error, got nil")
		}
		if !strings.Contains(err.Error(), "gkeepapi sidecar auth") {
			t.Fatalf("expected auth-class error, got %v", err)
		}
	}
	assertBreaker(t, c, breakerClosed, 0)
}

func TestDriftBreakerResetsOnSuccessFromTripping(t *testing.T) {
	t.Setenv("KEEP_GOOGLE_EMAIL", "user@example.com")
	mode := struct {
		mu      sync.Mutex
		failing bool
	}{failing: true}
	fn := &fakeNats{}
	fn.replyFn = driftReplyFn(t, func() ([]byte, error) {
		mode.mu.Lock()
		failing := mode.failing
		mode.mu.Unlock()
		if failing {
			return json.Marshal(KeepSyncResponse{Status: "ok", SchemaVersion: 99})
		}
		return json.Marshal(canonicalSyncResponseFixture())
	})
	c := New("google-keep")
	c.SetNatsClient(fn)
	if err := c.Connect(context.Background(), connectorCfgFromMap(driftAckCfg(""))); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	_, _, _, _ = c.syncGkeepapi(context.Background(), "")
	_, _, _, _ = c.syncGkeepapi(context.Background(), "")
	assertBreaker(t, c, breakerTripping, 2)
	// Flip to clean responses; next sync resets.
	mode.mu.Lock()
	mode.failing = false
	mode.mu.Unlock()
	_, _, _, err := c.syncGkeepapi(context.Background(), "")
	if err != nil {
		t.Fatalf("recovery sync: %v", err)
	}
	assertBreaker(t, c, breakerClosed, 0)
}

func TestOpenBreakerSkipsNatsPublish(t *testing.T) {
	t.Setenv("KEEP_GOOGLE_EMAIL", "user@example.com")
	fn := &fakeNats{}
	fn.replyFn = driftReplyFn(t, func() ([]byte, error) {
		return json.Marshal(KeepSyncResponse{Status: "ok", SchemaVersion: 99})
	})
	c := New("google-keep")
	c.SetNatsClient(fn)
	if err := c.Connect(context.Background(), connectorCfgFromMap(driftAckCfg(""))); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	// Drive to OPEN.
	for i := 0; i < breakerOpenThreshold; i++ {
		_, _, _, _ = c.syncGkeepapi(context.Background(), "")
	}
	// Adversarial: while OPEN, no Publish ever happens (publishes from
	// the failing path also never happen because those responses are
	// rejected before the publish loop).
	pubBefore := len(fn.publishes)
	for i := 0; i < 5; i++ {
		_, _, _, _ = c.syncGkeepapi(context.Background(), "")
	}
	if len(fn.publishes) != pubBefore {
		t.Fatalf("OPEN breaker emitted Publish calls; before=%d after=%d", pubBefore, len(fn.publishes))
	}
}

// SCN-059-013: drift counter increments exactly once per OPEN entry.
func TestDriftCounterIncrementsExactlyOncePerOpenEntry(t *testing.T) {
	t.Setenv("KEEP_GOOGLE_EMAIL", "user@example.com")
	// Use a fresh connector_id label so we read a clean counter value.
	connID := fmt.Sprintf("keep-drift-test-%d", testCounterID())
	fn := &fakeNats{}
	fn.replyFn = driftReplyFn(t, func() ([]byte, error) {
		return json.Marshal(KeepSyncResponse{Status: "ok", SchemaVersion: 99})
	})
	c := New(connID)
	c.SetNatsClient(fn)
	if err := c.Connect(context.Background(), connectorCfgFromMap(driftAckCfg("a"))); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	// Drive to OPEN.
	for i := 0; i < breakerOpenThreshold; i++ {
		_, _, _, _ = c.syncGkeepapi(context.Background(), "")
	}
	got := readCounterVec(t, metrics.KeepProtocolDriftDetected, connID)
	if got != 1 {
		t.Fatalf("counter after first OPEN entry = %d, want 1", got)
	}
	// Repeated Sync() in OPEN must not advance the counter.
	for i := 0; i < 5; i++ {
		_, _, _, _ = c.syncGkeepapi(context.Background(), "")
	}
	if got2 := readCounterVec(t, metrics.KeepProtocolDriftDetected, connID); got2 != 1 {
		t.Fatalf("counter after repeat OPEN syncs = %d, want still 1", got2)
	}
	// Token rotation + fresh trip increments by 1 again.
	if err := c.Connect(context.Background(), connectorCfgFromMap(driftAckCfg("b"))); err != nil {
		t.Fatalf("Connect rotate: %v", err)
	}
	for i := 0; i < breakerOpenThreshold; i++ {
		_, _, _, _ = c.syncGkeepapi(context.Background(), "")
	}
	if got3 := readCounterVec(t, metrics.KeepProtocolDriftDetected, connID); got3 != 2 {
		t.Fatalf("counter after second OPEN entry = %d, want 2", got3)
	}
}

// SCN-059-014: Health reports HealthError while OPEN and recovers.
func TestHealthReportsErrorWhileBreakerOpenAndRecoversAfterTokenRotation(t *testing.T) {
	t.Setenv("KEEP_GOOGLE_EMAIL", "user@example.com")
	// Toggle between failing and clean responses.
	mode := struct {
		mu      sync.Mutex
		failing bool
	}{failing: true}
	fn := &fakeNats{}
	fn.replyFn = driftReplyFn(t, func() ([]byte, error) {
		mode.mu.Lock()
		f := mode.failing
		mode.mu.Unlock()
		if f {
			return json.Marshal(KeepSyncResponse{Status: "ok", SchemaVersion: 99})
		}
		return json.Marshal(canonicalSyncResponseFixture())
	})
	c := New("google-keep")
	c.SetNatsClient(fn)
	if err := c.Connect(context.Background(), connectorCfgFromMap(driftAckCfg("t1"))); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	for i := 0; i < breakerOpenThreshold; i++ {
		_, _, _, _ = c.syncGkeepapi(context.Background(), "")
	}
	if got := c.Health(context.Background()); got != "error" {
		t.Fatalf("Health while OPEN = %q, want error", got)
	}
	// Rotate token + clean responses.
	mode.mu.Lock()
	mode.failing = false
	mode.mu.Unlock()
	if err := c.Connect(context.Background(), connectorCfgFromMap(driftAckCfg("t2"))); err != nil {
		t.Fatalf("Connect rotate: %v", err)
	}
	if _, _, _, err := c.syncGkeepapi(context.Background(), ""); err != nil {
		t.Fatalf("recovery sync: %v", err)
	}
	if got := c.Health(context.Background()); got != "healthy" {
		t.Fatalf("Health after recovery = %q, want healthy", got)
	}
}

// SCN-059-015: log redaction — KEEP_GOOGLE_EMAIL and
// KEEP_GOOGLE_APP_PASSWORD values MUST never appear in captured log
// output for any code path.
func TestKeepStructuredLogsDoNotContainEmailOrPassword(t *testing.T) {
	const email = "secret-user@example.com"
	const password = "abcd-efgh-ijkl-mnop"
	t.Setenv("KEEP_GOOGLE_EMAIL", email)
	t.Setenv("KEEP_GOOGLE_APP_PASSWORD", password)

	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })

	fn := &fakeNats{}
	// Drive both success and drift paths to exercise every log line.
	calls := 0
	fn.replyFn = driftReplyFn(t, func() ([]byte, error) {
		calls++
		if calls%2 == 0 {
			return json.Marshal(canonicalSyncResponseFixture())
		}
		return json.Marshal(KeepSyncResponse{Status: "ok", SchemaVersion: 99})
	})
	c := New("google-keep")
	c.SetNatsClient(fn)
	if err := c.Connect(context.Background(), connectorCfgFromMap(driftAckCfg(""))); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	for i := 0; i < 6; i++ {
		_, _, _, _ = c.syncGkeepapi(context.Background(), "")
	}
	out := buf.String()
	if strings.Contains(out, email) {
		t.Fatalf("captured log contains KEEP_GOOGLE_EMAIL value verbatim:\n%s", out)
	}
	if strings.Contains(out, password) {
		t.Fatalf("captured log contains KEEP_GOOGLE_APP_PASSWORD value verbatim:\n%s", out)
	}
}

// SCN-059-015 (success-path leg): histogram and notes counter advance
// when a sync returns N notes.
func TestSyncDurationAndNotesCounterPopulatedOnSuccess(t *testing.T) {
	t.Setenv("KEEP_GOOGLE_EMAIL", "user@example.com")
	fn := &fakeNats{}
	fn.replyFn = driftReplyFn(t, func() ([]byte, error) {
		return json.Marshal(canonicalSyncResponseFixture())
	})
	c := New("google-keep")
	c.SetNatsClient(fn)
	if err := c.Connect(context.Background(), connectorCfgFromMap(driftAckCfg(""))); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	notesBefore := readSimpleCounter(t, metrics.KeepGkeepNotesReturned)
	histBefore := readHistogramCount(t, metrics.KeepGkeepSyncDuration)

	arts, _, _, err := c.syncGkeepapi(context.Background(), "")
	if err != nil {
		t.Fatalf("syncGkeepapi: %v", err)
	}
	if len(arts) != 1 {
		t.Fatalf("artifacts = %d, want 1", len(arts))
	}
	if notesAfter := readSimpleCounter(t, metrics.KeepGkeepNotesReturned); notesAfter-notesBefore != 1 {
		t.Fatalf("notes counter delta = %d, want 1", notesAfter-notesBefore)
	}
	if histAfter := readHistogramCount(t, metrics.KeepGkeepSyncDuration); histAfter-histBefore != 1 {
		t.Fatalf("histogram count delta = %d, want 1", histAfter-histBefore)
	}
}

// --- helpers ---
func assertBreaker(t *testing.T, c *Connector, want breakerState, wantFailures int) {
	t.Helper()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.breakerState != want {
		t.Fatalf("breakerState = %s, want %s", c.breakerState, want)
	}
	if c.driftFailures != wantFailures {
		t.Fatalf("driftFailures = %d, want %d", c.driftFailures, wantFailures)
	}
}

// connectorCfgFromMap builds a connector.ConnectorConfig from a free-form
// source map. The existing testConnectorConfig helper does not accept
// arbitrary fields like drift_ack_token, so this stays test-local.
func connectorCfgFromMap(sc map[string]interface{}) connector.ConnectorConfig {
	return connector.ConnectorConfig{SourceConfig: sc}
}

// readCounterVec returns the float64 value of a CounterVec metric for
// the supplied connector_id, via the prometheus client_model path so we
// avoid pulling in an extra testutil dependency. Returns 0 when no
// labels match.
func readCounterVec(t *testing.T, vec *prometheus.CounterVec, connectorID string) int64 {
	t.Helper()
	ch := make(chan prometheus.Metric, 32)
	vec.Collect(ch)
	close(ch)
	for m := range ch {
		var d dto.Metric
		if err := m.Write(&d); err != nil {
			t.Fatalf("metric write: %v", err)
		}
		if d.Counter == nil {
			continue
		}
		for _, lp := range d.Label {
			if lp.GetName() == "connector_id" && lp.GetValue() == connectorID {
				return int64(d.Counter.GetValue())
			}
		}
	}
	return 0
}

// testCounterID returns a process-unique integer for synthesising
// distinct connector_id labels per sub-test (so counter assertions
// stay isolated from prior runs in the same process).
var (
	tcMu sync.Mutex
	tcN  int64
)

func testCounterID() int64 {
	tcMu.Lock()
	defer tcMu.Unlock()
	tcN++
	return tcN
}

// readSimpleCounter returns the current value of a label-less Counter.
func readSimpleCounter(t *testing.T, c prometheus.Counter) int64 {
	t.Helper()
	var d dto.Metric
	if err := c.Write(&d); err != nil {
		t.Fatalf("counter write: %v", err)
	}
	if d.Counter == nil {
		return 0
	}
	return int64(d.Counter.GetValue())
}

// readHistogramCount returns the SampleCount of a Histogram.
func readHistogramCount(t *testing.T, h prometheus.Histogram) int64 {
	t.Helper()
	var d dto.Metric
	if err := h.Write(&d); err != nil {
		t.Fatalf("histogram write: %v", err)
	}
	if d.Histogram == nil {
		return 0
	}
	return int64(d.Histogram.GetSampleCount())
}
