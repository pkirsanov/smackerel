//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	smacknats "github.com/smackerel/smackerel/internal/nats"
)

func TestNATS_EnsureStreams(t *testing.T) {
	js, _ := testJetStream(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use the real EnsureStreams logic by creating streams from AllStreams config
	for _, sc := range smacknats.AllStreams() {
		cfg := jetstream.StreamConfig{
			Name:      sc.Name,
			Subjects:  sc.Subjects,
			Retention: jetstream.WorkQueuePolicy,
			MaxAge:    7 * 24 * time.Hour,
			Storage:   jetstream.FileStorage,
		}
		if sc.Name == "DEADLETTER" {
			cfg.Retention = jetstream.LimitsPolicy
			cfg.MaxAge = 30 * 24 * time.Hour
			cfg.MaxMsgs = 10000
		}
		_, err := js.CreateOrUpdateStream(ctx, cfg)
		if err != nil {
			t.Fatalf("create/update stream %s: %v", sc.Name, err)
		}
	}

	// Verify all expected streams exist
	expectedStreams := []string{
		"ARTIFACTS", "SEARCH", "DIGEST", "KEEP",
		"INTELLIGENCE", "ALERTS", "SYNTHESIS", "DOMAIN",
		"ANNOTATIONS", "LISTS", "DEADLETTER",
	}

	for _, name := range expectedStreams {
		stream, err := js.Stream(ctx, name)
		if err != nil {
			t.Errorf("stream %s not found: %v", name, err)
			continue
		}
		info, err := stream.Info(ctx)
		if err != nil {
			t.Errorf("stream %s info: %v", name, err)
			continue
		}
		t.Logf("stream %s: subjects=%v msgs=%d", name, info.Config.Subjects, info.State.Msgs)
	}
}

func TestNATS_PublishSubscribe_Artifacts(t *testing.T) {
	js, _ := testJetStream(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Ensure ARTIFACTS stream exists with WorkQueue retention
	_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "ARTIFACTS",
		Subjects:  []string{"artifacts.>"},
		Retention: jetstream.WorkQueuePolicy,
		MaxAge:    7 * 24 * time.Hour,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		t.Fatalf("ensure ARTIFACTS stream: %v", err)
	}

	// Create a consumer
	consumerName := testID(t)
	// STAB-031-001: DeliverNewPolicy ignores stale messages from crashed
	// previous runs on WorkQueue streams, preventing payload-mismatch flakes.
	cons, err := js.CreateOrUpdateConsumer(ctx, "ARTIFACTS", jetstream.ConsumerConfig{
		Durable:       consumerName,
		FilterSubject: "artifacts.process",
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverNewPolicy,
	})
	if err != nil {
		t.Fatalf("create consumer: %v", err)
	}
	t.Cleanup(func() {
		js.DeleteConsumer(context.Background(), "ARTIFACTS", consumerName)
	})

	// Publish a test message
	type processMsg struct {
		ArtifactID string `json:"artifact_id"`
		TestMarker string `json:"test_marker"`
	}
	payload := processMsg{ArtifactID: testID(t), TestMarker: "integration-test"}
	data, _ := json.Marshal(payload)

	_, err = js.Publish(ctx, "artifacts.process", data)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Consume and verify
	msgs, err := cons.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	received := 0
	for msg := range msgs.Messages() {
		var got processMsg
		if err := json.Unmarshal(msg.Data(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.ArtifactID != payload.ArtifactID {
			t.Errorf("expected artifact_id %q, got %q", payload.ArtifactID, got.ArtifactID)
		}
		if got.TestMarker != "integration-test" {
			t.Errorf("expected test_marker 'integration-test', got %q", got.TestMarker)
		}
		msg.Ack()
		received++
	}
	if received != 1 {
		t.Errorf("expected 1 message, received %d", received)
	}
}

func TestNATS_PublishSubscribe_Domain(t *testing.T) {
	js, _ := testJetStream(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Ensure DOMAIN stream exists
	_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "DOMAIN",
		Subjects:  []string{"domain.>"},
		Retention: jetstream.WorkQueuePolicy,
		MaxAge:    7 * 24 * time.Hour,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		t.Fatalf("ensure DOMAIN stream: %v", err)
	}

	consumerName := testID(t)
	// STAB-031-001: DeliverNewPolicy ignores stale messages from crashed
	// previous runs on WorkQueue streams, preventing payload-mismatch flakes.
	cons, err := js.CreateOrUpdateConsumer(ctx, "DOMAIN", jetstream.ConsumerConfig{
		Durable:       consumerName,
		FilterSubject: "domain.extract",
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverNewPolicy,
	})
	if err != nil {
		t.Fatalf("create consumer: %v", err)
	}
	t.Cleanup(func() {
		js.DeleteConsumer(context.Background(), "DOMAIN", consumerName)
	})

	type extractMsg struct {
		ArtifactID string `json:"artifact_id"`
		URL        string `json:"url"`
	}
	payload := extractMsg{ArtifactID: testID(t), URL: "https://example.com/recipe"}
	data, _ := json.Marshal(payload)

	_, err = js.Publish(ctx, "domain.extract", data)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	msgs, err := cons.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	received := 0
	for msg := range msgs.Messages() {
		var got extractMsg
		if err := json.Unmarshal(msg.Data(), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.ArtifactID != payload.ArtifactID {
			t.Errorf("expected artifact_id %q, got %q", payload.ArtifactID, got.ArtifactID)
		}
		msg.Ack()
		received++
	}
	if received != 1 {
		t.Errorf("expected 1 message, received %d", received)
	}
}

// Scenario: Consumer replay after simulated crash (Nak + redeliver)
func TestNATS_ConsumerReplay_NakRedeliver(t *testing.T) {
	js, _ := testJetStream(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Ensure DEADLETTER stream (uses LimitsPolicy, so messages survive Nak)
	_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "DEADLETTER",
		Subjects:  []string{"deadletter.>"},
		Retention: jetstream.LimitsPolicy,
		MaxAge:    30 * 24 * time.Hour,
		MaxMsgs:   10000,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		t.Fatalf("ensure DEADLETTER stream: %v", err)
	}

	consumerName := testID(t)
	cons, err := js.CreateOrUpdateConsumer(ctx, "DEADLETTER", jetstream.ConsumerConfig{
		Durable:       consumerName,
		FilterSubject: "deadletter.>",
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    3,
		AckWait:       2 * time.Second,
	})
	if err != nil {
		t.Fatalf("create consumer: %v", err)
	}
	t.Cleanup(func() {
		js.DeleteConsumer(context.Background(), "DEADLETTER", consumerName)
	})

	// Publish a message
	testPayload := []byte(`{"test":"nak-redeliver"}`)
	_, err = js.Publish(ctx, "deadletter.test", testPayload)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	// First fetch: Nak (simulates crash)
	msgs, err := cons.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	for msg := range msgs.Messages() {
		msg.Nak()
	}

	// CHAOS-031-003: Poll for redelivered message instead of hardcoded sleep.
	// Under load, AckWait timing may vary; polling is resilient.
	redeliveryDeadline := time.Now().Add(15 * time.Second)
	received := 0
	for time.Now().Before(redeliveryDeadline) {
		msgs, err = cons.Fetch(1, jetstream.FetchMaxWait(2*time.Second))
		if err != nil {
			t.Fatalf("redeliver fetch: %v", err)
		}
		for msg := range msgs.Messages() {
			if string(msg.Data()) != string(testPayload) {
				t.Errorf("redelivered message mismatch")
			}
			msg.Ack()
			received++
		}
		if received > 0 {
			break
		}
	}
	if received != 1 {
		t.Errorf("expected 1 redelivered message, got %d", received)
	}
	t.Log("Nak + redeliver verified")
}

// CHAOS-031-006: MaxDeliver exhaustion — terminal dead-message path.
// When a consumer Naks a message MaxDeliver times, NATS stops redelivering it.
// This tests the critical path where a processing failure becomes permanent:
// the consumer must NOT receive the message after exhausting all delivery attempts.
// In production, this means the message lands in advisory territory and should be
// routed to dead-letter handling.
func TestNATS_Chaos_MaxDeliverExhaustion(t *testing.T) {
	js, _ := testJetStream(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Use DEADLETTER stream (LimitsPolicy — messages survive Nak)
	_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "DEADLETTER",
		Subjects:  []string{"deadletter.>"},
		Retention: jetstream.LimitsPolicy,
		MaxAge:    30 * 24 * time.Hour,
		MaxMsgs:   10000,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		t.Fatalf("ensure DEADLETTER stream: %v", err)
	}

	consumerName := testID(t)
	maxDeliver := 3
	cons, err := js.CreateOrUpdateConsumer(ctx, "DEADLETTER", jetstream.ConsumerConfig{
		Durable:       consumerName,
		FilterSubject: "deadletter.>",
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxDeliver:    maxDeliver,
		AckWait:       1 * time.Second, // Short AckWait to speed up redelivery
	})
	if err != nil {
		t.Fatalf("create consumer: %v", err)
	}
	t.Cleanup(func() {
		js.DeleteConsumer(context.Background(), "DEADLETTER", consumerName)
	})

	// Publish a poisonous message
	subject := fmt.Sprintf("deadletter.chaos-maxdeliver-%d", time.Now().UnixNano())
	testPayload := []byte(`{"test":"chaos-maxdeliver-exhaustion"}`)
	_, err = js.Publish(ctx, subject, testPayload)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Nak the message maxDeliver times — each Nak should trigger a redeliver
	nakCount := 0
	for attempt := 0; attempt < maxDeliver; attempt++ {
		fetchDeadline := time.Now().Add(10 * time.Second)
		fetched := false
		for time.Now().Before(fetchDeadline) {
			msgs, fetchErr := cons.Fetch(1, jetstream.FetchMaxWait(2*time.Second))
			if fetchErr != nil {
				t.Fatalf("fetch attempt %d: %v", attempt+1, fetchErr)
			}
			for msg := range msgs.Messages() {
				msg.Nak()
				nakCount++
				fetched = true
			}
			if fetched {
				break
			}
		}
		if !fetched {
			t.Fatalf("delivery attempt %d: message not received within deadline", attempt+1)
		}
	}

	if nakCount != maxDeliver {
		t.Errorf("expected %d Nak'd deliveries, got %d", maxDeliver, nakCount)
	}

	// After exhausting MaxDeliver, the message must NOT be redelivered.
	// Wait for AckWait to elapse, then try to fetch — should get nothing.
	time.Sleep(2 * time.Second)
	msgs, err := cons.Fetch(1, jetstream.FetchMaxWait(3*time.Second))
	if err != nil {
		t.Fatalf("post-exhaustion fetch: %v", err)
	}
	extraReceived := 0
	for msg := range msgs.Messages() {
		_ = msg
		extraReceived++
	}
	if extraReceived != 0 {
		t.Errorf("expected 0 messages after MaxDeliver exhaustion, got %d — dead-message path broken", extraReceived)
	}
	t.Logf("MaxDeliver=%d exhausted after %d Naks, no further redelivery confirmed", maxDeliver, nakCount)
}
