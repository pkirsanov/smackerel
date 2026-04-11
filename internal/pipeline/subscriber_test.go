package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

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
