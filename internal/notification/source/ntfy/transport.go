package ntfy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
)

type StreamClient interface {
	Subscribe(ctx context.Context, cfg Config, topic string, events chan<- Event) error
}

type WebhookHandler interface {
	HandleNtfyEvent(ctx context.Context, event Event) error
}

type WebhookPayloadErrorHandler interface {
	HandleNtfyPayloadError(ctx context.Context, payload []byte, err error) error
}

type WebhookReceiver interface {
	Start(ctx context.Context, cfg Config, handler WebhookHandler) error
}

type HTTPStreamClient struct {
	client *http.Client
}

func NewHTTPStreamClient(client *http.Client) *HTTPStreamClient {
	return &HTTPStreamClient{client: client}
}

func WithStreamClient(client StreamClient) AdapterOption {
	return func(adapter *Adapter) {
		adapter.streamClient = client
	}
}

func WithWebhookReceiver(receiver WebhookReceiver) AdapterOption {
	return func(adapter *Adapter) {
		adapter.webhookReceiver = receiver
	}
}

func WithStore(store *Store) AdapterOption {
	return func(adapter *Adapter) {
		adapter.store = store
	}
}

func (c *HTTPStreamClient) Subscribe(ctx context.Context, cfg Config, topic string, events chan<- Event) error {
	if cfg.Auth.Mode != AuthModeNone {
		return fmt.Errorf("ntfy stream: auth mode %s requires a secret-resolved transport client", cfg.Auth.Mode)
	}
	streamURL, err := ntfyStreamURL(cfg.EndpointURL, topic)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		return fmt.Errorf("ntfy stream: create request: %w", err)
	}
	client := c.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ntfy stream: subscribe topic %s: %w", topic, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("ntfy stream: subscribe topic %s returned status %d", topic, resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	bufferLimit := cfg.DeadLetter.MaxPayloadBytes + 1024
	scanner.Buffer(make([]byte, 0, 4096), bufferLimit)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		event, err := ParseEvent([]byte(line), cfg.DeadLetter.MaxPayloadBytes)
		if err != nil {
			return err
		}
		select {
		case events <- event:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("ntfy stream: read topic %s: %w", topic, err)
	}
	return nil
}

func ntfyStreamURL(endpoint string, topic string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return "", fmt.Errorf("ntfy stream: endpoint URL is invalid")
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("ntfy stream: endpoint URL must include scheme and host")
	}
	if strings.TrimSpace(topic) == "" {
		return "", fmt.Errorf("ntfy stream: topic is required")
	}
	basePath := strings.TrimRight(parsed.EscapedPath(), "/")
	parsed.Path = basePath + "/" + url.PathEscape(topic) + "/json"
	parsed.RawQuery = ""
	return parsed.String(), nil
}

func (a *Adapter) Start(ctx context.Context, sink notification.SourceEventSink) error {
	if sink == nil {
		return fmt.Errorf("ntfy source adapter: source event sink is required")
	}
	if _, err := a.cfg.SourceInstanceConfig(); err != nil {
		return err
	}
	if a.cfg.TransportMode == TransportModeStream && a.streamClient == nil {
		return fmt.Errorf("ntfy source adapter: stream client is required")
	}
	if a.cfg.TransportMode == TransportModeWebhook && a.webhookReceiver == nil {
		return fmt.Errorf("ntfy source adapter: webhook receiver is required")
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		cancel()
		return fmt.Errorf("ntfy source adapter: already started")
	}
	a.cancel = cancel
	a.done = done
	a.running = true
	a.mu.Unlock()
	go a.run(runCtx, sink, done)
	return nil
}

func (a *Adapter) Stop(ctx context.Context) error {
	a.mu.Lock()
	cancel := a.cancel
	done := a.done
	if cancel != nil {
		cancel()
	}
	a.running = false
	now := time.Now().UTC()
	a.health = notification.SourceHealthReport{SourceType: SourceType, SourceInstanceID: a.cfg.SourceInstanceID, SourceForm: a.cfg.SourceForm, State: notification.SourceHealthDisconnected, LastErrorKind: "stopped", LastErrorRedacted: "source transport stopped", ObservedAt: now}
	a.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Adapter) run(ctx context.Context, sink notification.SourceEventSink, done chan struct{}) {
	defer close(done)
	switch a.cfg.TransportMode {
	case TransportModeStream:
		a.runStream(ctx, sink)
	case TransportModeWebhook:
		a.runWebhook(ctx, sink)
	}
}

func (a *Adapter) runStream(ctx context.Context, sink notification.SourceEventSink) {
	events := make(chan Event, len(a.cfg.Topics)+1)
	var wg sync.WaitGroup
	for _, topic := range a.cfg.Topics {
		topic := topic
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.subscribeTopicWithReconnect(ctx, sink, topic, events)
		}()
	}
	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
		close(events)
	}()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := a.handleTransportEvent(ctx, sink, event); err != nil && !errors.Is(err, context.Canceled) {
				slog.Warn("ntfy transport event handling failed", "source_instance_id", a.cfg.SourceInstanceID, "topic", event.Topic, "error", err)
			}
		case <-ctx.Done():
			<-waitDone
			return
		}
	}
}

func (a *Adapter) subscribeTopicWithReconnect(ctx context.Context, sink notification.SourceEventSink, topic string, events chan<- Event) {
	for attempt := 1; ; attempt++ {
		err := a.streamClient.Subscribe(ctx, a.cfg, topic, events)
		if err == nil || ctx.Err() != nil {
			return
		}
		now := time.Now().UTC()
		if attempt >= a.cfg.Reconnect.RetryBudget {
			state := a.subscriptionFailureState(topic, SubscriptionDisconnected, ErrorRetryBudgetExhausted, "source reconnect retry budget exhausted", attempt, now)
			a.persistAndReportTopicState(ctx, sink, state, now)
			return
		}
		state := a.subscriptionFailureState(topic, SubscriptionReconnecting, ErrorConnectivityFailed, "source connectivity check failed", attempt, now)
		a.persistAndReportTopicState(ctx, sink, state, now)
		delay := time.Duration(a.retryDelaySeconds(attempt)) * time.Second
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		}
	}
}

func (a *Adapter) runWebhook(ctx context.Context, sink notification.SourceEventSink) {
	handler := adapterWebhookHandler{adapter: a, sink: sink}
	if err := a.webhookReceiver.Start(ctx, a.cfg, handler); err != nil && ctx.Err() == nil {
		now := time.Now().UTC()
		for _, topic := range a.cfg.Topics {
			state := a.subscriptionFailureState(topic, SubscriptionDisconnected, ErrorConnectivityFailed, "source webhook receiver failed", 1, now)
			a.persistAndReportTopicState(ctx, sink, state, now)
		}
	}
}

type adapterWebhookHandler struct {
	adapter *Adapter
	sink    notification.SourceEventSink
}

func (h adapterWebhookHandler) HandleNtfyEvent(ctx context.Context, event Event) error {
	return h.adapter.handleTransportEvent(ctx, h.sink, event)
}

func (h adapterWebhookHandler) HandleNtfyPayloadError(ctx context.Context, payload []byte, err error) error {
	now := time.Now().UTC()
	h.adapter.recordDeadLetter(ctx, h.sink, Event{}, payload, DeadLetterMalformedJSON, err.Error(), false, now)
	return nil
}

func (a *Adapter) handleTransportEvent(ctx context.Context, sink notification.SourceEventSink, event Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if event.Topic == "" && len(a.cfg.Topics) == 1 {
		event.Topic = a.cfg.Topics[0]
	}
	now := time.Now().UTC()
	if event.IsLifecycle() {
		state := a.lifecycleSubscriptionState(event, now)
		a.persistAndReportTopicState(ctx, sink, state, now)
		return nil
	}
	if event.EventType == "error" {
		state := a.subscriptionFailureState(event.Topic, SubscriptionReconnecting, ErrorConnectivityFailed, "source transport reported an error", 1, now)
		a.persistAndReportTopicState(ctx, sink, state, now)
		return nil
	}
	envelope, err := MapEvent(ctx, a.cfg, event, now)
	if err != nil {
		state := a.subscriptionFailureState(event.Topic, SubscriptionReconnecting, ErrorConnectivityFailed, "source payload rejected", 1, now)
		a.persistAndReportTopicState(ctx, sink, state, now)
		a.recordDeadLetter(ctx, sink, event, event.Raw, deadLetterCauseForMapError(a.cfg, event), err.Error(), false, now)
		return err
	}
	receipt, attempts, err := a.submitWithBoundedRetry(ctx, sink, envelope)
	if err != nil {
		state := a.subscriptionFailureState(event.Topic, SubscriptionReconnecting, ErrorConnectivityFailed, "source event could not be accepted after bounded retry", attempts, now)
		a.persistAndReportTopicState(ctx, sink, state, now)
		a.recordDeadLetter(ctx, sink, event, event.Raw, DeadLetterSinkUnavailable, "source sink was unavailable after bounded retry", true, now)
		return err
	}
	if !receipt.Accepted {
		state := a.subscriptionFailureState(event.Topic, SubscriptionReconnecting, ErrorConnectivityFailed, "source event was rejected by the source sink", attempts, now)
		a.persistAndReportTopicState(ctx, sink, state, now)
		a.recordDeadLetter(ctx, sink, event, event.Raw, DeadLetterSinkRejected, "source sink rejected event after bounded retry", true, now)
		return fmt.Errorf("ntfy source adapter: source sink rejected event")
	}
	state := SubscriptionState{SourceInstanceID: a.cfg.SourceInstanceID, Topic: event.Topic, SourceForm: a.cfg.SourceForm, TransportMode: a.cfg.TransportMode, SubscriptionState: SubscriptionConnected, LastNtfyEventID: event.ID, LastEventAt: &now, LastSuccessfulCheckAt: &now, RetryBudget: a.cfg.Reconnect.RetryBudget, RedactionState: emptyRedactionState(), CreatedAt: now, UpdatedAt: now}
	a.persistAndReportTopicState(ctx, sink, state, now)
	return nil
}

func (a *Adapter) lifecycleSubscriptionState(event Event, now time.Time) SubscriptionState {
	state := SubscriptionState{SourceInstanceID: a.cfg.SourceInstanceID, Topic: event.Topic, SourceForm: a.cfg.SourceForm, TransportMode: a.cfg.TransportMode, SubscriptionState: SubscriptionConnected, LastSuccessfulCheckAt: &now, RetryBudget: a.cfg.Reconnect.RetryBudget, RedactionState: emptyRedactionState(), CreatedAt: now, UpdatedAt: now}
	switch event.EventType {
	case "open":
		state.LastOpenAt = &now
	case "keepalive":
		state.LastKeepaliveAt = &now
	}
	return state
}

func (a *Adapter) subscriptionFailureState(topic string, subscriptionState string, errorKind string, errorRedacted string, retryCount int, now time.Time) SubscriptionState {
	return SubscriptionState{SourceInstanceID: a.cfg.SourceInstanceID, Topic: topic, SourceForm: a.cfg.SourceForm, TransportMode: a.cfg.TransportMode, SubscriptionState: subscriptionState, PossibleGap: true, RetryCount: retryCount, RetryBudget: a.cfg.Reconnect.RetryBudget, LastErrorKind: errorKind, LastErrorRedacted: errorRedacted, RedactionState: emptyRedactionState(), CreatedAt: now, UpdatedAt: now}
}

func (a *Adapter) persistAndReportTopicState(ctx context.Context, sink notification.SourceEventSink, state SubscriptionState, observedAt time.Time) {
	if ctx.Err() != nil {
		return
	}
	state = FinalizeSubscriptionState(a.cfg, state, observedAt)
	if a.store != nil {
		if err := a.store.UpsertSubscriptionState(ctx, state); err != nil {
			slog.Warn("ntfy source topic state persistence failed", "source_instance_id", a.cfg.SourceInstanceID, "topic", state.Topic, "error", err)
		}
	}
	report := HealthFromTopics(a.cfg, []SubscriptionState{state}, observedAt)
	a.setHealth(report)
	if err := sink.ReportSourceHealth(ctx, report); err != nil {
		slog.Warn("ntfy source health report failed", "source_instance_id", a.cfg.SourceInstanceID, "error", err)
	}
}

func (a *Adapter) submitWithBoundedRetry(ctx context.Context, sink notification.SourceEventSink, envelope notification.SourceEventEnvelope) (notification.IngestReceipt, int, error) {
	maxAttempts := a.cfg.DeadLetter.RetryBudget
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		receipt, err := sink.SubmitSourceEvent(ctx, envelope)
		if err == nil {
			return receipt, attempt, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return notification.IngestReceipt{}, attempt, ctx.Err()
		}
	}
	return notification.IngestReceipt{}, maxAttempts, lastErr
}

func emptyRedactionState() map[string]any {
	return map[string]any{"status": "redacted", "categories": []string{}}
}

func (a *Adapter) recordDeadLetter(ctx context.Context, sink notification.SourceEventSink, event Event, payload []byte, causeKind string, cause string, replayEligible bool, observedAt time.Time) {
	if a.store == nil || ctx.Err() != nil {
		return
	}
	record := NewDeadLetterRecord(a.cfg, event, payload, causeKind, cause, replayEligible, observedAt)
	if _, err := a.store.CreateDeadLetter(ctx, record); err != nil {
		slog.Warn("ntfy source dead-letter persistence failed", "source_instance_id", a.cfg.SourceInstanceID, "topic", event.Topic, "error", err)
		return
	}
	a.reportDeadLetterPressure(ctx, sink, observedAt)
}

func (a *Adapter) reportDeadLetterPressure(ctx context.Context, sink notification.SourceEventSink, observedAt time.Time) {
	if a.store == nil || sink == nil || ctx.Err() != nil {
		return
	}
	count, err := a.store.CountDeadLetters(ctx, a.cfg.SourceInstanceID)
	if err != nil {
		slog.Warn("ntfy source dead-letter pressure count failed", "source_instance_id", a.cfg.SourceInstanceID, "error", err)
		return
	}
	if count < a.cfg.DeadLetter.PressureThresholdCount {
		return
	}
	report := DeadLetterPressureHealth(a.cfg, observedAt)
	a.setHealth(report)
	if err := sink.ReportSourceHealth(ctx, report); err != nil {
		slog.Warn("ntfy source dead-letter pressure health report failed", "source_instance_id", a.cfg.SourceInstanceID, "error", err)
	}
}

func deadLetterCauseForMapError(cfg Config, event Event) string {
	if !topicConfigured(cfg, event.Topic) {
		return DeadLetterTopicNotConfigured
	}
	if !event.ShouldIngest() {
		return DeadLetterUnsupportedEvent
	}
	return DeadLetterUnknown
}

func (a *Adapter) retryDelaySeconds(attempt int) int {
	delay := a.cfg.Reconnect.InitialDelaySeconds
	for index := 1; index < attempt; index++ {
		delay *= 2
		if delay >= a.cfg.Reconnect.MaxDelaySeconds {
			return a.cfg.Reconnect.MaxDelaySeconds
		}
	}
	return delay
}

func (a *Adapter) setHealth(report notification.SourceHealthReport) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.health = report
}
