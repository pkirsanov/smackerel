package ntfy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
)

func TestNtfyAdapterStartRequiresTransportClientAndStopsCleanly(t *testing.T) {
	cfg := testConfig()
	cfg.Auth = AuthConfig{Mode: AuthModeNone}
	stream := newScriptedStreamClient([]Event{{EventType: "open", Topic: "home-lab-alerts"}, {EventType: "keepalive", Topic: "home-lab-alerts"}, {ID: "evt-transport", EventType: "message", Topic: "home-lab-alerts", Title: "Transport", Message: "observed through stream", Priority: "high", Raw: []byte(`{"id":"evt-transport","event":"message","topic":"home-lab-alerts","title":"Transport","message":"observed through stream","priority":"high"}`)}})
	adapter, err := NewAdapter(cfg, WithStreamClient(stream))
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	sink := &recordingSourceSink{}
	if err := adapter.Start(context.Background(), sink); err != nil {
		t.Fatalf("start adapter: %v", err)
	}
	stream.waitStarted(t, "home-lab-alerts")
	sink.waitForEnvelopeCount(t, 1)
	if got := sink.envelopes()[0]; got.SourceEventID != "evt-transport" || got.DeliveryMetadata["topic"] != "home-lab-alerts" {
		t.Fatalf("transport event was not submitted through source sink: %+v", got)
	}
	sink.waitForHealthState(t, notification.SourceHealthConnected)
	if err := adapter.Stop(context.Background()); err != nil {
		t.Fatalf("stop adapter: %v", err)
	}
	stream.waitStopped(t)
	if got := sink.envelopeCount(); got != 1 {
		t.Fatalf("adapter accepted events after stop: got %d accepted envelopes", got)
	}
}

func TestNtfyStreamRetryBudgetExhaustionReportsDisconnectedWithoutSyntheticNotification(t *testing.T) {
	cfg := testConfig()
	cfg.Auth = AuthConfig{Mode: AuthModeNone}
	cfg.Reconnect.RetryBudget = 2
	cfg.Reconnect.InitialDelaySeconds = 1
	cfg.Reconnect.MaxDelaySeconds = 1
	failing := &failingStreamClient{}
	adapter, err := NewAdapter(cfg, WithStreamClient(failing))
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	sink := &recordingSourceSink{}
	if err := adapter.Start(context.Background(), sink); err != nil {
		t.Fatalf("start adapter: %v", err)
	}
	sink.waitForHealthState(t, notification.SourceHealthDisconnected)
	if got := failing.attemptCount(); got != cfg.Reconnect.RetryBudget {
		t.Fatalf("stream retry attempts = %d, want retry budget %d", got, cfg.Reconnect.RetryBudget)
	}
	if got := sink.envelopeCount(); got != 0 {
		t.Fatalf("retry exhaustion created %d synthetic notification envelope(s)", got)
	}
	if err := adapter.Stop(context.Background()); err != nil {
		t.Fatalf("stop adapter: %v", err)
	}
}

func TestNtfyStartConfiguredAdaptersReadsJSONAndStartsStreamAndWebhook(t *testing.T) {
	streamCfg := testConfig()
	streamCfg.Auth = AuthConfig{Mode: AuthModeNone}
	webhookCfg := testConfig()
	webhookCfg.SourceInstanceID = "ntfy-webhook-alerts"
	webhookCfg.SourceForm = notification.SourceFormWebhook
	webhookCfg.TransportMode = TransportModeWebhook
	webhookCfg.Auth = AuthConfig{Mode: AuthModeNone}
	webhookCfg.ConfigHash = "sha256:test-webhook-config"
	rawConfig := encodeRuntimeTestConfigs(t, streamCfg, webhookCfg)
	stream := newScriptedStreamClient([]Event{{ID: "evt-runtime-stream", EventType: "message", Topic: "home-lab-alerts", Title: "Runtime Stream", Message: "configured stream", Raw: []byte(`{"id":"evt-runtime-stream","event":"message","topic":"home-lab-alerts","title":"Runtime Stream","message":"configured stream"}`)}})
	registry := NewWebhookReceiverRegistry()
	sink := &recordingSourceSink{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime, err := StartConfiguredAdapters(ctx, rawConfig, sink, WithRuntimeStreamClient(stream), WithRuntimeWebhookReceiver(registry))
	if err != nil {
		t.Fatalf("start configured adapters: %v", err)
	}
	defer func() {
		if err := runtime.Stop(context.Background()); err != nil {
			t.Fatalf("stop runtime: %v", err)
		}
	}()
	stream.waitStarted(t, "home-lab-alerts")
	waitForWebhookRegistration(t, registry, webhookCfg.SourceInstanceID)
	sink.waitForEnvelopeCount(t, 1)
	if err := registry.ReceiveRaw(context.Background(), webhookCfg.SourceInstanceID, []byte(`{"id":"evt-runtime-webhook","event":"message","topic":"home-lab-alerts","title":"Runtime Webhook","message":"configured webhook"}`)); err != nil {
		t.Fatalf("receive runtime webhook: %v", err)
	}
	sink.waitForEnvelopeCount(t, 2)
	envelopes := sink.envelopes()
	if envelopes[0].SourceEventID != "evt-runtime-stream" || envelopes[1].SourceEventID != "evt-runtime-webhook" {
		t.Fatalf("configured adapters did not submit expected stream and webhook events: %+v", envelopes)
	}
	if err := registry.ReceiveRaw(context.Background(), webhookCfg.SourceInstanceID, []byte(`{"event":"message","topic":"unconfigured","message":"wrong topic"}`)); err == nil {
		t.Fatal("expected unconfigured webhook topic to be rejected")
	}
	if err := registry.ReceiveRaw(context.Background(), webhookCfg.SourceInstanceID, []byte(`{"event":"message"`)); !IsWebhookPayloadInvalid(err) {
		t.Fatalf("expected malformed webhook payload error, got %v", err)
	}
}

func waitForWebhookRegistration(t *testing.T, registry *WebhookReceiverRegistry, sourceInstanceID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if registry.IsRegistered(sourceInstanceID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for webhook source %s registration", sourceInstanceID)
}

func TestNtfyStartConfiguredAdaptersFailsLoudForMalformedConfig(t *testing.T) {
	_, err := StartConfiguredAdapters(context.Background(), `[{"enabled":true,"source_instance_id":"ntfy-broken"}]`, &recordingSourceSink{})
	if err == nil {
		t.Fatal("expected malformed NTFY_SOURCES_JSON entry to fail startup")
	}
	if !strings.Contains(err.Error(), "source form") {
		t.Fatalf("startup error should name missing source form, got %v", err)
	}
}

type recordingSourceSink struct {
	mu              sync.Mutex
	gotEnvelopes    []notification.SourceEventEnvelope
	gotHealth       []notification.SourceHealthReport
	submitErr       error
	reportHealthErr error
}

func (s *recordingSourceSink) SubmitSourceEvent(ctx context.Context, envelope notification.SourceEventEnvelope) (notification.IngestReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.submitErr != nil {
		return notification.IngestReceipt{}, s.submitErr
	}
	s.gotEnvelopes = append(s.gotEnvelopes, envelope)
	return notification.IngestReceipt{SourceType: envelope.SourceType, SourceInstanceID: envelope.SourceInstanceID, SourceForm: envelope.SourceForm, RawEventID: "raw-" + envelope.SourceEventID, Accepted: true, Status: "accepted"}, nil
}

func (s *recordingSourceSink) ReportSourceHealth(ctx context.Context, report notification.SourceHealthReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reportHealthErr != nil {
		return s.reportHealthErr
	}
	s.gotHealth = append(s.gotHealth, report)
	return nil
}

func (s *recordingSourceSink) envelopes() []notification.SourceEventEnvelope {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]notification.SourceEventEnvelope(nil), s.gotEnvelopes...)
}

func (s *recordingSourceSink) envelopeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.gotEnvelopes)
}

func (s *recordingSourceSink) waitForEnvelopeCount(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.envelopeCount() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d source envelopes; got %d", want, s.envelopeCount())
}

func (s *recordingSourceSink) waitForHealthState(t *testing.T, want notification.SourceHealthState) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		for _, report := range s.gotHealth {
			if report.State == want {
				s.mu.Unlock()
				return
			}
		}
		s.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for health state %s", want)
}

type scriptedStreamClient struct {
	events  []Event
	started chan string
	stopped chan struct{}
}

type failingStreamClient struct {
	mu       sync.Mutex
	attempts int
}

func (c *failingStreamClient) Subscribe(ctx context.Context, cfg Config, topic string, events chan<- Event) error {
	c.mu.Lock()
	c.attempts++
	c.mu.Unlock()
	return errors.New("ntfy stream unavailable")
}

func (c *failingStreamClient) attemptCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.attempts
}

func newScriptedStreamClient(events []Event) *scriptedStreamClient {
	return &scriptedStreamClient{events: events, started: make(chan string, 1), stopped: make(chan struct{})}
}

func (c *scriptedStreamClient) Subscribe(ctx context.Context, cfg Config, topic string, events chan<- Event) error {
	c.started <- topic
	defer close(c.stopped)
	for _, event := range c.events {
		select {
		case events <- event:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

func (c *scriptedStreamClient) waitStarted(t *testing.T, wantTopic string) {
	t.Helper()
	select {
	case got := <-c.started:
		if got != wantTopic {
			t.Fatalf("stream topic = %q, want %q", got, wantTopic)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream client was not started")
	}
}

func (c *scriptedStreamClient) waitStopped(t *testing.T) {
	t.Helper()
	select {
	case <-c.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("stream client did not observe shutdown")
	}
}

func encodeRuntimeTestConfigs(t *testing.T, configs ...Config) string {
	t.Helper()
	items := make([]configJSON, 0, len(configs))
	for _, cfg := range configs {
		items = append(items, configJSON{SourceInstanceID: cfg.SourceInstanceID, Enabled: cfg.Enabled, SourceForm: string(cfg.SourceForm), TransportMode: cfg.TransportMode, EndpointURL: cfg.EndpointURL, EndpointRefName: cfg.EndpointRefName, Topics: append([]string(nil), cfg.Topics...), AuthMode: cfg.Auth.Mode, SecretRefNames: append([]string(nil), cfg.Auth.SecretRefNames...), DefaultDomain: cfg.Mapping.DefaultDomain, TopicSubjects: cloneStringMap(cfg.Mapping.TopicSubjects), TagServices: cloneStringMap(cfg.Mapping.TagServices), TagIntents: cloneStringMap(cfg.Mapping.TagIntents), RetryBudget: cfg.Reconnect.RetryBudget, InitialDelaySeconds: cfg.Reconnect.InitialDelaySeconds, MaxDelaySeconds: cfg.Reconnect.MaxDelaySeconds, KeepaliveTimeoutSeconds: cfg.Reconnect.KeepaliveTimeoutSeconds, LagDegradedAfterSeconds: cfg.Lag.DegradedAfterSeconds, LagDisconnectedAfterSeconds: cfg.Lag.DisconnectedAfterSeconds, DeadLetterRetryBudget: cfg.DeadLetter.RetryBudget, MaxPayloadBytes: cfg.DeadLetter.MaxPayloadBytes, PressureThresholdCount: cfg.DeadLetter.PressureThresholdCount, DisplayName: cfg.RedactedMetadata["display_name"], EndpointLabel: cfg.RedactedMetadata["endpoint_label"], ConfigHash: cfg.ConfigHash})
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("encode runtime config: %v", err)
	}
	return string(encoded)
}
