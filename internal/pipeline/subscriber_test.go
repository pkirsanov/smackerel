package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// mockJetStreamMsg implements jetstream.Msg for unit testing.
type mockJetStreamMsg struct {
	data        []byte
	subject     string
	reply       string
	headers     nats.Header
	metadata    *jetstream.MsgMetadata
	metadataErr error
	acked       bool
	naked       bool
}

func (m *mockJetStreamMsg) Data() []byte                       { return m.data }
func (m *mockJetStreamMsg) Subject() string                    { return m.subject }
func (m *mockJetStreamMsg) Reply() string                      { return m.reply }
func (m *mockJetStreamMsg) Headers() nats.Header               { return m.headers }
func (m *mockJetStreamMsg) Ack() error                         { m.acked = true; return nil }
func (m *mockJetStreamMsg) DoubleAck(_ context.Context) error  { m.acked = true; return nil }
func (m *mockJetStreamMsg) Nak() error                         { m.naked = true; return nil }
func (m *mockJetStreamMsg) NakWithDelay(_ time.Duration) error { m.naked = true; return nil }
func (m *mockJetStreamMsg) InProgress() error                  { return nil }
func (m *mockJetStreamMsg) Term() error                        { return nil }
func (m *mockJetStreamMsg) TermWithReason(_ string) error      { return nil }
func (m *mockJetStreamMsg) Metadata() (*jetstream.MsgMetadata, error) {
	return m.metadata, m.metadataErr
}

// mockJetStream captures published messages for assertion.
type mockJetStream struct {
	jetstream.JetStream
	published  []*nats.Msg
	publishErr error
}

func (m *mockJetStream) PublishMsg(_ context.Context, msg *nats.Msg, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	if m.publishErr != nil {
		return nil, m.publishErr
	}
	m.published = append(m.published, msg)
	return &jetstream.PubAck{Stream: "DEADLETTER"}, nil
}

// --- isDeliveryExhausted tests ---

func TestIsDeliveryExhausted_AtMaxDeliver(t *testing.T) {
	rs := &ResultSubscriber{}
	msg := &mockJetStreamMsg{
		metadata: &jetstream.MsgMetadata{NumDelivered: 5},
	}
	if !rs.isDeliveryExhausted(msg, 5) {
		t.Error("expected exhausted when NumDelivered == MaxDeliver")
	}
}

func TestIsDeliveryExhausted_AboveMaxDeliver(t *testing.T) {
	rs := &ResultSubscriber{}
	msg := &mockJetStreamMsg{
		metadata: &jetstream.MsgMetadata{NumDelivered: 7},
	}
	if !rs.isDeliveryExhausted(msg, 5) {
		t.Error("expected exhausted when NumDelivered > MaxDeliver")
	}
}

func TestIsDeliveryExhausted_BelowMaxDeliver(t *testing.T) {
	rs := &ResultSubscriber{}
	msg := &mockJetStreamMsg{
		metadata: &jetstream.MsgMetadata{NumDelivered: 3},
	}
	if rs.isDeliveryExhausted(msg, 5) {
		t.Error("expected NOT exhausted when NumDelivered < MaxDeliver")
	}
}

func TestIsDeliveryExhausted_FirstDelivery(t *testing.T) {
	rs := &ResultSubscriber{}
	msg := &mockJetStreamMsg{
		metadata: &jetstream.MsgMetadata{NumDelivered: 1},
	}
	if rs.isDeliveryExhausted(msg, 5) {
		t.Error("expected NOT exhausted on first delivery")
	}
}

func TestIsDeliveryExhausted_MetadataError(t *testing.T) {
	rs := &ResultSubscriber{}
	msg := &mockJetStreamMsg{
		metadataErr: nats.ErrBadSubscription,
	}
	// When metadata is unavailable, treat as NOT exhausted (safe default — retry)
	if rs.isDeliveryExhausted(msg, 5) {
		t.Error("expected NOT exhausted when metadata is unavailable")
	}
}

func TestIsDeliveryExhausted_UsesDefaultMaxDeliver(t *testing.T) {
	rs := &ResultSubscriber{}
	msg := &mockJetStreamMsg{
		metadata: &jetstream.MsgMetadata{NumDelivered: uint64(DefaultMaxDeliver)},
	}
	if !rs.isDeliveryExhausted(msg, DefaultMaxDeliver) {
		t.Errorf("expected exhausted at DefaultMaxDeliver=%d", DefaultMaxDeliver)
	}
}

// --- publishToDeadLetter tests ---

func TestPublishToDeadLetter_CorrectHeaders(t *testing.T) {
	js := &mockJetStream{}
	rs := &ResultSubscriber{
		NATS: &smacknats.Client{JetStream: js},
	}

	msg := &mockJetStreamMsg{
		data: []byte(`{"artifact_id":"test-1"}`),
		metadata: &jetstream.MsgMetadata{
			NumDelivered: 5,
			Consumer:     "smackerel-core-processed",
		},
	}

	err := rs.publishToDeadLetter(context.Background(), msg, "artifacts.processed", "ARTIFACTS", "MaxDeliver exhausted")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(js.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(js.published))
	}

	dlMsg := js.published[0]

	// Check subject
	if dlMsg.Subject != "deadletter.artifacts.processed" {
		t.Errorf("expected subject deadletter.artifacts.processed, got %s", dlMsg.Subject)
	}

	// Check payload preserved
	if string(dlMsg.Data) != `{"artifact_id":"test-1"}` {
		t.Errorf("expected original payload preserved, got %s", string(dlMsg.Data))
	}

	// Check required headers per design contract (Section 2, Data Model)
	requiredHeaders := map[string]string{
		"Smackerel-Original-Subject":  "artifacts.processed",
		"Smackerel-Original-Stream":   "ARTIFACTS",
		"Smackerel-Original-Consumer": "smackerel-core-processed",
		"Smackerel-Delivery-Count":    "5",
		"Smackerel-Last-Error":        "MaxDeliver exhausted",
	}

	for key, expected := range requiredHeaders {
		actual := dlMsg.Header.Get(key)
		if actual != expected {
			t.Errorf("header %s: expected %q, got %q", key, expected, actual)
		}
	}

	// Smackerel-Failed-At must be a valid RFC3339 timestamp
	failedAt := dlMsg.Header.Get("Smackerel-Failed-At")
	if failedAt == "" {
		t.Error("missing Smackerel-Failed-At header")
	} else if _, err := time.Parse(time.RFC3339, failedAt); err != nil {
		t.Errorf("Smackerel-Failed-At is not valid RFC3339: %s", failedAt)
	}
}

func TestPublishToDeadLetter_ErrorTruncation(t *testing.T) {
	js := &mockJetStream{}
	rs := &ResultSubscriber{
		NATS: &smacknats.Client{JetStream: js},
	}

	msg := &mockJetStreamMsg{
		data:     []byte(`{}`),
		metadata: &jetstream.MsgMetadata{NumDelivered: 5},
	}

	// Create an error string longer than 256 bytes
	longError := strings.Repeat("x", 300)

	err := rs.publishToDeadLetter(context.Background(), msg, "test.subject", "TEST", longError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dlMsg := js.published[0]
	lastErr := dlMsg.Header.Get("Smackerel-Last-Error")
	if len(lastErr) > 256 {
		t.Errorf("Smackerel-Last-Error should be truncated to 256 bytes, got %d", len(lastErr))
	}
	if len(lastErr) != 256 {
		t.Errorf("expected exactly 256 bytes after truncation, got %d", len(lastErr))
	}
}

func TestPublishToDeadLetter_EmptyLastError(t *testing.T) {
	js := &mockJetStream{}
	rs := &ResultSubscriber{
		NATS: &smacknats.Client{JetStream: js},
	}

	msg := &mockJetStreamMsg{
		data:     []byte(`{}`),
		metadata: &jetstream.MsgMetadata{NumDelivered: 5},
	}

	err := rs.publishToDeadLetter(context.Background(), msg, "test.subject", "TEST", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dlMsg := js.published[0]
	if dlMsg.Header.Get("Smackerel-Last-Error") != "" {
		t.Error("Smackerel-Last-Error should be absent when lastError is empty")
	}
}

func TestPublishToDeadLetter_MetadataUnavailable(t *testing.T) {
	js := &mockJetStream{}
	rs := &ResultSubscriber{
		NATS: &smacknats.Client{JetStream: js},
	}

	msg := &mockJetStreamMsg{
		data:        []byte(`{"test":"data"}`),
		metadataErr: nats.ErrBadSubscription,
	}

	err := rs.publishToDeadLetter(context.Background(), msg, "test.subject", "TEST", "some error")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dlMsg := js.published[0]

	// Core headers should still be present
	if dlMsg.Header.Get("Smackerel-Original-Subject") != "test.subject" {
		t.Error("Smackerel-Original-Subject should be present even without metadata")
	}
	if dlMsg.Header.Get("Smackerel-Original-Stream") != "TEST" {
		t.Error("Smackerel-Original-Stream should be present even without metadata")
	}

	// Metadata-derived headers should be absent
	if dlMsg.Header.Get("Smackerel-Delivery-Count") != "" {
		t.Error("Smackerel-Delivery-Count should be absent when metadata is unavailable")
	}
	if dlMsg.Header.Get("Smackerel-Original-Consumer") != "" {
		t.Error("Smackerel-Original-Consumer should be absent when metadata is unavailable")
	}
}

func TestPublishToDeadLetter_PublishFailure(t *testing.T) {
	js := &mockJetStream{publishErr: nats.ErrTimeout}
	rs := &ResultSubscriber{
		NATS: &smacknats.Client{JetStream: js},
	}

	msg := &mockJetStreamMsg{
		data:     []byte(`{}`),
		metadata: &jetstream.MsgMetadata{NumDelivered: 5},
	}

	err := rs.publishToDeadLetter(context.Background(), msg, "test.subject", "TEST", "error")
	if err == nil {
		t.Error("expected error when publish fails")
	}
}

// --- handleMessage validation tests ---

func TestHandleMessage_EmptyArtifactID_AckedAsInvalid(t *testing.T) {
	// A payload with empty artifact_id should fail ValidateProcessedPayload
	payload := `{"artifact_id":"","success":true}`
	msg := &mockJetStreamMsg{
		data:     []byte(payload),
		metadata: &jetstream.MsgMetadata{NumDelivered: 1},
	}

	rs := &ResultSubscriber{
		NATS:      &smacknats.Client{JetStream: &mockJetStream{}},
		Processor: NewProcessor(nil, nil),
	}
	rs.handleMessage(context.Background(), msg)

	// Empty artifact_id fails validation → Ack to prevent infinite redelivery
	if !msg.acked {
		t.Error("expected Ack for payload failing ValidateProcessedPayload (empty artifact_id)")
	}
	if msg.naked {
		t.Error("should NOT Nak a validation-failed message — Ack to prevent redelivery")
	}
}

func TestHandleMessage_MalformedJSON_Acked(t *testing.T) {
	msg := &mockJetStreamMsg{
		data:     []byte(`{invalid json`),
		metadata: &jetstream.MsgMetadata{NumDelivered: 1},
	}

	rs := &ResultSubscriber{
		NATS:      &smacknats.Client{JetStream: &mockJetStream{}},
		Processor: NewProcessor(nil, nil),
	}
	rs.handleMessage(context.Background(), msg)

	if !msg.acked {
		t.Error("expected Ack for malformed JSON to prevent infinite redelivery")
	}
}

// TestHandleMessage_FinalDelivery_ProcessesBeforeDeadLetter verifies that on the
// final delivery attempt (NumDelivered == MaxDeliver), the handler still ATTEMPTS
// processing before routing to dead-letter. This is adversarial: if processing
// is skipped on the final delivery, we lose one attempt and the dead-letter
// Smackerel-Last-Error only says "MaxDeliver exhausted" instead of the real error.
func TestHandleMessage_FinalDelivery_ProcessesBeforeDeadLetter(t *testing.T) {
	js := &mockJetStream{}
	// Payload with a valid artifact_id that will fail at HandleProcessedResult
	// because the DB pool is nil (causes a real error, not a validation skip).
	payload := `{"artifact_id":"test-final","title":"Test","artifact_type":"url","summary":"s","topics":["t"],"success":true}`
	msg := &mockJetStreamMsg{
		data:     []byte(payload),
		metadata: &jetstream.MsgMetadata{NumDelivered: uint64(DefaultMaxDeliver), Consumer: "test-consumer"},
	}

	rs := &ResultSubscriber{
		NATS:      &smacknats.Client{JetStream: js},
		Processor: NewProcessor(nil, nil), // nil DB pool → HandleProcessedResult returns error
	}
	rs.handleMessage(context.Background(), msg)

	// The message MUST be acked (routed to dead-letter), not naked
	if !msg.acked {
		t.Error("expected Ack after dead-letter routing on final delivery")
	}
	if msg.naked {
		t.Error("should NOT Nak on final delivery — should dead-letter instead")
	}

	// Verify dead-letter was published with a REAL error, not generic "MaxDeliver exhausted"
	if len(js.published) != 1 {
		t.Fatalf("expected exactly 1 dead-letter publish, got %d", len(js.published))
	}
	dlMsg := js.published[0]
	lastErr := dlMsg.Header.Get("Smackerel-Last-Error")
	if lastErr == "" {
		t.Fatal("expected Smackerel-Last-Error header in dead-letter message")
	}
	if lastErr == "MaxDeliver exhausted" {
		t.Error("Smackerel-Last-Error should contain the actual processing error, not generic 'MaxDeliver exhausted'")
	}
}

// TestHandleMessage_BeforeMaxDeliver_Naks verifies that processing failures
// before the final delivery are Nak'd for retry, NOT dead-lettered.
func TestHandleMessage_BeforeMaxDeliver_Naks(t *testing.T) {
	js := &mockJetStream{}
	payload := `{"artifact_id":"test-retry","title":"Test","artifact_type":"url","summary":"s","topics":["t"],"success":true}`
	msg := &mockJetStreamMsg{
		data:     []byte(payload),
		metadata: &jetstream.MsgMetadata{NumDelivered: uint64(DefaultMaxDeliver - 1)},
	}

	rs := &ResultSubscriber{
		NATS:      &smacknats.Client{JetStream: js},
		Processor: NewProcessor(nil, nil), // nil DB pool → error
	}
	rs.handleMessage(context.Background(), msg)

	if !msg.naked {
		t.Error("expected Nak for processing failure before MaxDeliver (retry)")
	}
	if msg.acked {
		t.Error("should NOT Ack before MaxDeliver — Nak for retry")
	}
	if len(js.published) != 0 {
		t.Error("should NOT publish to dead-letter before MaxDeliver")
	}
}

// --- UTF-8 safe truncation tests (IMP-022-R29-003) ---

func TestTruncateBytes_MultiByte_DoesNotSplitRune(t *testing.T) {
	// "héllo" has 'é' as 2-byte UTF-8 (0xC3 0xA9)
	// If we truncate at byte 2, we'd split the é rune at its second byte.
	data := []byte("héllo world — this is a long string with unicode characters")
	result := truncateBytes(data, 2)
	// Byte 0 = 'h' (1 byte), byte 1-2 = 'é' (2 bytes).
	// Truncating at 2 bytes would cut the é in half.
	// The function should step back to byte 1 to produce valid UTF-8.
	if !utf8.ValidString(result) {
		t.Errorf("truncateBytes produced invalid UTF-8: %q", result)
	}
}

func TestTruncateBytes_FourByteEmoji_DoesNotSplit(t *testing.T) {
	// '😀' = 4 bytes (F0 9F 98 80)
	data := []byte("test😀data")
	// "test" = 4 bytes, "😀" = bytes 4-7, "data" = bytes 8-11
	// Truncating at 6 would cut the emoji mid-rune.
	result := truncateBytes(data, 6)
	if !utf8.ValidString(result) {
		t.Errorf("truncateBytes produced invalid UTF-8 when cutting 4-byte emoji: %q", result)
	}
}

func TestTruncateUTF8_MultiByte_DoesNotSplitRune(t *testing.T) {
	// 256 ASCII chars + multi-byte char → truncation at 256 should not split
	s := strings.Repeat("a", 254) + "éé" // 254 + 2*2 = 258 bytes
	result := truncateUTF8(s, 256)
	if len(result) > 256 {
		t.Errorf("expected at most 256 bytes, got %d", len(result))
	}
	if !utf8.ValidString(result) {
		t.Errorf("truncateUTF8 produced invalid UTF-8: %q", result)
	}
	// Should include the first é (bytes 254-255) but not cut the second one
	if len(result) != 256 {
		t.Errorf("expected exactly 256 bytes (first é fits), got %d", len(result))
	}
}

func TestTruncateUTF8_CJK_DoesNotSplitRune(t *testing.T) {
	// CJK characters are 3 bytes each in UTF-8
	s := strings.Repeat("a", 255) + "中" // 255 + 3 = 258 bytes
	result := truncateUTF8(s, 256)
	if len(result) > 256 {
		t.Errorf("expected at most 256 bytes, got %d", len(result))
	}
	if !utf8.ValidString(result) {
		t.Errorf("truncateUTF8 produced invalid UTF-8: %q", result)
	}
	// '中' starts at byte 255 and is 3 bytes, so at 256 it would split → should be excluded
	if len(result) != 255 {
		t.Errorf("expected 255 bytes (CJK char excluded to avoid split), got %d", len(result))
	}
}

func TestPublishToDeadLetter_MultiByte_ErrorTruncation(t *testing.T) {
	js := &mockJetStream{}
	rs := &ResultSubscriber{
		NATS: &smacknats.Client{JetStream: js},
	}

	msg := &mockJetStreamMsg{
		data:     []byte(`{}`),
		metadata: &jetstream.MsgMetadata{NumDelivered: 5},
	}

	// Error with multi-byte characters near the 256 boundary
	longError := strings.Repeat("a", 254) + "中文" // 254 + 3 + 3 = 260 bytes
	err := rs.publishToDeadLetter(context.Background(), msg, "test.subject", "TEST", longError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dlMsg := js.published[0]
	lastErr := dlMsg.Header.Get("Smackerel-Last-Error")
	if len(lastErr) > 256 {
		t.Errorf("Smackerel-Last-Error should be at most 256 bytes, got %d", len(lastErr))
	}
	if !utf8.ValidString(lastErr) {
		t.Errorf("Smackerel-Last-Error has invalid UTF-8 after truncation: %q", lastErr)
	}
}
