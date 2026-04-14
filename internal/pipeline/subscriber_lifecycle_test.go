package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/smackerel/smackerel/internal/digest"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// newTestDigestGen creates a digest.Generator with nil dependencies for testing.
// HandleDigestResult will fail with a DB error, which is the expected path for
// dead-letter routing tests (process first, dead-letter on failure at MaxDeliver).
func newTestDigestGen() *digest.Generator {
	return digest.NewGenerator(nil, nil, nil)
}

// --- handleDigestMessage tests ---

func TestHandleDigestMessage_MalformedJSON_Acked(t *testing.T) {
	msg := &mockJetStreamMsg{
		data:     []byte(`{not valid json`),
		metadata: &jetstream.MsgMetadata{NumDelivered: 1},
	}

	rs := &ResultSubscriber{
		NATS: &smacknats.Client{JetStream: &mockJetStream{}},
	}
	rs.handleDigestMessage(context.Background(), msg)

	if !msg.acked {
		t.Error("malformed JSON digest message should be Acked to prevent infinite redelivery")
	}
	if msg.naked {
		t.Error("should NOT Nak a malformed digest message")
	}
}

func TestHandleDigestMessage_EmptyDate_Acked(t *testing.T) {
	payload := NATSDigestGeneratedPayload{
		DigestDate: "",
		Text:       "Some digest",
	}
	data, _ := json.Marshal(payload)

	msg := &mockJetStreamMsg{
		data:     data,
		metadata: &jetstream.MsgMetadata{NumDelivered: 1},
	}

	rs := &ResultSubscriber{
		NATS: &smacknats.Client{JetStream: &mockJetStream{}},
	}
	rs.handleDigestMessage(context.Background(), msg)

	if !msg.acked {
		t.Error("digest with empty date should be Acked (validation failure, permanent)")
	}
}

func TestHandleDigestMessage_EmptyText_Acked(t *testing.T) {
	payload := NATSDigestGeneratedPayload{
		DigestDate: "2026-04-12",
		Text:       "",
	}
	data, _ := json.Marshal(payload)

	msg := &mockJetStreamMsg{
		data:     data,
		metadata: &jetstream.MsgMetadata{NumDelivered: 1},
	}

	rs := &ResultSubscriber{
		NATS: &smacknats.Client{JetStream: &mockJetStream{}},
	}
	rs.handleDigestMessage(context.Background(), msg)

	if !msg.acked {
		t.Error("digest with empty text should be Acked (validation failure, permanent)")
	}
}

func TestHandleDigestMessage_DeliveryExhausted_RoutesToDeadLetter(t *testing.T) {
	js := &mockJetStream{}
	rs := &ResultSubscriber{
		NATS:      &smacknats.Client{JetStream: js},
		DigestGen: nil, // nil DigestGen is not hit because we need a valid payload
	}

	// Use a valid payload so it passes unmarshal + validation, but processing
	// will fail because DigestGen is nil. We need to provide a real DigestGen
	// that errors out, or guard the nil. Using a generator with nil pool.
	rs.DigestGen = newTestDigestGen()

	msg := &mockJetStreamMsg{
		data: []byte(`{"digest_date":"2026-04-12","text":"digest content","word_count":10,"model_used":"test"}`),
		metadata: &jetstream.MsgMetadata{
			NumDelivered: uint64(DefaultMaxDeliver),
			Consumer:     "smackerel-core-digest",
		},
	}

	rs.handleDigestMessage(context.Background(), msg)

	if !msg.acked {
		t.Error("exhausted digest message should be Acked after dead-letter routing")
	}
	if len(js.published) != 1 {
		t.Fatalf("expected 1 dead-letter message, got %d", len(js.published))
	}
	if js.published[0].Subject != "deadletter.digest.generated" {
		t.Errorf("expected dead-letter subject, got %q", js.published[0].Subject)
	}
	// Verify the dead-letter carries a real error, not just "MaxDeliver exhausted"
	lastErr := js.published[0].Header.Get("Smackerel-Last-Error")
	if lastErr == "" {
		t.Error("dead-letter should contain Smackerel-Last-Error header")
	}
	if lastErr == "MaxDeliver exhausted" {
		t.Error("dead-letter should carry the actual processing error, not generic text")
	}
}

func TestHandleDigestMessage_DeliveryExhausted_DeadLetterFails_Naks(t *testing.T) {
	js := &mockJetStream{publishErr: nats.ErrTimeout}
	rs := &ResultSubscriber{
		NATS:      &smacknats.Client{JetStream: js},
		DigestGen: newTestDigestGen(),
	}

	msg := &mockJetStreamMsg{
		data: []byte(`{"digest_date":"2026-04-12","text":"digest content","word_count":10,"model_used":"test"}`),
		metadata: &jetstream.MsgMetadata{
			NumDelivered: uint64(DefaultMaxDeliver),
		},
	}

	rs.handleDigestMessage(context.Background(), msg)

	if msg.acked {
		t.Error("should NOT Ack when dead-letter publish fails")
	}
	if !msg.naked {
		t.Error("should Nak when dead-letter publish fails to preserve message")
	}
}

// --- handleMessage dead-letter routing ---

func TestHandleMessage_DeliveryExhausted_RoutesToDeadLetter(t *testing.T) {
	js := &mockJetStream{}
	rs := &ResultSubscriber{
		NATS:      &smacknats.Client{JetStream: js},
		Processor: NewProcessor(nil, nil), // nil DB pool → processing error
	}

	// Valid payload that passes unmarshal + validation but fails at processing
	msg := &mockJetStreamMsg{
		data: []byte(`{"artifact_id":"test-1","title":"Test","artifact_type":"url","summary":"s","topics":["t"],"success":true}`),
		metadata: &jetstream.MsgMetadata{
			NumDelivered: uint64(DefaultMaxDeliver),
			Consumer:     "smackerel-core-processed",
		},
	}

	rs.handleMessage(context.Background(), msg)

	if !msg.acked {
		t.Error("exhausted artifact message should be Acked after dead-letter routing")
	}
	if len(js.published) != 1 {
		t.Fatalf("expected 1 dead-letter message, got %d", len(js.published))
	}
	if js.published[0].Subject != "deadletter.artifacts.processed" {
		t.Errorf("expected dead-letter subject, got %q", js.published[0].Subject)
	}
	// Verify real error in dead-letter
	lastErr := js.published[0].Header.Get("Smackerel-Last-Error")
	if lastErr == "" {
		t.Error("dead-letter should contain Smackerel-Last-Error header")
	}
	if lastErr == "MaxDeliver exhausted" {
		t.Error("dead-letter should carry the actual processing error, not generic text")
	}
}

func TestHandleMessage_DeliveryExhausted_DeadLetterFails_Naks(t *testing.T) {
	js := &mockJetStream{publishErr: fmt.Errorf("NATS unreachable")}
	rs := &ResultSubscriber{
		NATS:      &smacknats.Client{JetStream: js},
		Processor: NewProcessor(nil, nil), // nil DB pool → processing error
	}

	msg := &mockJetStreamMsg{
		data: []byte(`{"artifact_id":"test-1","title":"Test","artifact_type":"url","summary":"s","topics":["t"],"success":true}`),
		metadata: &jetstream.MsgMetadata{
			NumDelivered: uint64(DefaultMaxDeliver),
		},
	}

	rs.handleMessage(context.Background(), msg)

	if msg.acked {
		t.Error("should NOT Ack when dead-letter publish fails")
	}
	if !msg.naked {
		t.Error("should Nak when dead-letter publish fails to preserve message")
	}
}

// --- ResultSubscriber lifecycle tests ---

func TestResultSubscriber_StopBeforeStart(t *testing.T) {
	rs := &ResultSubscriber{}
	// Should not panic
	rs.Stop()
	if rs.started {
		t.Error("Stop() on unstarted subscriber should not set started")
	}
}

func TestResultSubscriber_DoubleStop(t *testing.T) {
	rs := &ResultSubscriber{}
	rs.mu.Lock()
	rs.started = true
	rs.done = make(chan struct{})
	rs.mu.Unlock()

	rs.Stop()
	// Second stop should not panic
	rs.Stop()

	if !rs.stopped {
		t.Error("should be stopped after Stop()")
	}
}

func TestNewResultSubscriber(t *testing.T) {
	rs := NewResultSubscriber(nil, nil, nil)
	if rs == nil {
		t.Fatal("expected non-nil subscriber")
	}
	if rs.Processor == nil {
		t.Error("Processor should be initialized")
	}
	if rs.DigestGen == nil {
		t.Error("DigestGen should be initialized")
	}
	if rs.started {
		t.Error("should not be started on creation")
	}
	if rs.stopped {
		t.Error("should not be stopped on creation")
	}
}

func TestDefaultMaxDeliver_Value(t *testing.T) {
	if DefaultMaxDeliver < 1 {
		t.Errorf("DefaultMaxDeliver must be positive, got %d", DefaultMaxDeliver)
	}
	if DefaultMaxDeliver != 5 {
		t.Errorf("DefaultMaxDeliver should be 5 per design, got %d", DefaultMaxDeliver)
	}
}
