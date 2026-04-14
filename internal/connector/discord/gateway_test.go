package discord

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// mockGateway is a test double for GatewayClient that lets tests push events
// directly into the buffer and control health responses.
type mockGateway struct {
	events  chan GatewayEvent
	healthy bool
	closed  bool
}

func (m *mockGateway) Connect(_ context.Context, _ string, _ int) error { return nil }
func (m *mockGateway) Events() <-chan GatewayEvent                      { return m.events }
func (m *mockGateway) Healthy() bool                                    { return m.healthy }
func (m *mockGateway) Close() error                                     { m.closed = true; return nil }

func TestEventPoller_ConnectStartsPolling(t *testing.T) {
	t.Parallel()
	var fetchCalls int64
	fetcher := func(_ context.Context, channelID, _ string, _ int) ([]DiscordMessage, error) {
		atomic.AddInt64(&fetchCalls, 1)
		return []DiscordMessage{
			{ID: "100000000000000001", Content: "hello", ChannelID: channelID,
				GuildID: "900000000000000000", Author: Author{ID: "800000000000000000", Username: "bot"}},
		}, nil
	}

	channels := map[string]struct{}{"111000000000000000": {}}
	poller := NewEventPoller(channels, fetcher, 100, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := poller.Connect(ctx, "token", IntentGuilds|IntentGuildMessages|IntentMessageContent); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer poller.Close()

	// Wait for at least one event to appear on the channel.
	select {
	case ev := <-poller.Events():
		if ev.Type != "MESSAGE_CREATE" {
			t.Errorf("expected MESSAGE_CREATE, got %s", ev.Type)
		}
		if ev.Message.ID != "100000000000000001" {
			t.Errorf("expected message ID 100000000000000001, got %s", ev.Message.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for gateway event")
	}

	if atomic.LoadInt64(&fetchCalls) < 1 {
		t.Error("expected at least one fetch call")
	}
	if !poller.Healthy() {
		t.Error("poller should be healthy after successful fetch")
	}
}

func TestEventPoller_EventsFilterToMonitoredChannels(t *testing.T) {
	t.Parallel()
	// The fetcher returns messages for any channel it's asked about.
	// The EventPoller should ONLY poll channels in its configured set.
	var polledChannels sync.Map
	fetcher := func(_ context.Context, channelID, _ string, _ int) ([]DiscordMessage, error) {
		polledChannels.Store(channelID, true)
		return nil, nil
	}

	// Only monitor channel "111", not "222"
	channels := map[string]struct{}{"111000000000000000": {}}
	poller := NewEventPoller(channels, fetcher, 100, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := poller.Connect(ctx, "token", IntentGuilds); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Let it poll a few times
	time.Sleep(200 * time.Millisecond)
	poller.Close()

	// Verify only the monitored channel was polled
	if _, ok := polledChannels.Load("111000000000000000"); !ok {
		t.Error("expected monitored channel 111000000000000000 to be polled")
	}
	if _, ok := polledChannels.Load("222000000000000000"); ok {
		t.Error("non-monitored channel 222000000000000000 should not be polled")
	}
}

func TestEventPoller_SyncDrainsBufferedEvents(t *testing.T) {
	t.Parallel()
	// Set up an httptest server for the connector's REST calls during Sync
	ts := newTestDiscordAPI(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/channels/111000000000000000/messages", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[]`))
		})
	})

	c := New("discord-gw-test")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url":        ts.URL,
			"enable_gateway": false, // we inject gateway manually
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "900000000000000000",
					"channel_ids": []interface{}{"111000000000000000"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Inject a mock gateway with pre-buffered events
	mgw := &mockGateway{events: make(chan GatewayEvent, 10), healthy: true}
	mgw.events <- GatewayEvent{
		Type: "MESSAGE_CREATE",
		Message: DiscordMessage{
			ID: "200000000000000000", Content: "gateway msg",
			ChannelID: "111000000000000000", GuildID: "900000000000000000",
			Author: Author{ID: "300000000000000000", Username: "user1"},
		},
	}
	mgw.events <- GatewayEvent{
		Type: "MESSAGE_CREATE",
		Message: DiscordMessage{
			ID: "200000000000000001", Content: "gateway msg 2",
			ChannelID: "111000000000000000", GuildID: "900000000000000000",
			Author: Author{ID: "300000000000000000", Username: "user1"},
		},
	}

	c.mu.Lock()
	c.gateway = mgw
	c.mu.Unlock()

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Verify gateway events were drained into artifacts
	if len(artifacts) < 2 {
		t.Fatalf("expected at least 2 artifacts from gateway drain, got %d", len(artifacts))
	}

	foundIDs := map[string]bool{}
	for _, a := range artifacts {
		foundIDs[a.SourceRef] = true
	}
	if !foundIDs["200000000000000000"] {
		t.Error("missing gateway event 200000000000000000 in artifacts")
	}
	if !foundIDs["200000000000000001"] {
		t.Error("missing gateway event 200000000000000001 in artifacts")
	}
	if cursor == "" {
		t.Error("expected non-empty cursor after sync with gateway events")
	}
}

func TestEventPoller_ReconnectOnPollingFailure(t *testing.T) {
	t.Parallel()
	var callCount int64
	fetcher := func(_ context.Context, channelID, _ string, _ int) ([]DiscordMessage, error) {
		n := atomic.AddInt64(&callCount, 1)
		if n <= 3 {
			return nil, fmt.Errorf("simulated failure %d", n)
		}
		// Succeed after 3 failures
		return []DiscordMessage{
			{ID: "100000000000000010", Content: "recovered", ChannelID: channelID,
				GuildID: "900000000000000000", Author: Author{ID: "800000000000000000", Username: "bot"}},
		}, nil
	}

	channels := map[string]struct{}{"111000000000000000": {}}
	// Use very short poll interval so backoff doesn't take too long
	poller := NewEventPoller(channels, fetcher, 100, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := poller.Connect(ctx, "token", IntentGuilds); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer poller.Close()

	// Wait for recovery event — backoff is 1s, 2s, 4s for 3 failures
	// Total max ~7s; give 10s timeout
	select {
	case ev := <-poller.Events():
		if ev.Message.Content != "recovered" {
			t.Errorf("expected recovered, got %q", ev.Message.Content)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for recovery after poll failures")
	}

	if !poller.Healthy() {
		t.Error("poller should be healthy after recovery")
	}
}

func TestEventPoller_CloseStopsPolling(t *testing.T) {
	t.Parallel()
	var fetchCalls int64
	fetcher := func(_ context.Context, _ string, _ string, _ int) ([]DiscordMessage, error) {
		atomic.AddInt64(&fetchCalls, 1)
		return nil, nil
	}

	channels := map[string]struct{}{"111000000000000000": {}}
	poller := NewEventPoller(channels, fetcher, 100, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := poller.Connect(ctx, "token", IntentGuilds); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Let it poll a few times
	time.Sleep(200 * time.Millisecond)
	poller.Close()

	// Record count at close time and wait to verify no more calls
	countAtClose := atomic.LoadInt64(&fetchCalls)
	time.Sleep(200 * time.Millisecond)
	countAfter := atomic.LoadInt64(&fetchCalls)

	if countAfter > countAtClose {
		t.Errorf("polling continued after Close: %d calls at close, %d after", countAtClose, countAfter)
	}
}

func TestEventPoller_EventBufferOverflow(t *testing.T) {
	t.Parallel()
	msgCount := 0
	fetcher := func(_ context.Context, channelID, _ string, _ int) ([]DiscordMessage, error) {
		msgCount++
		// Return a message every poll
		return []DiscordMessage{
			{ID: fmt.Sprintf("10000000000000%04d", msgCount), Content: "msg",
				ChannelID: channelID, GuildID: "900000000000000000",
				Author: Author{ID: "800000000000000000", Username: "bot"}},
		}, nil
	}

	// Tiny buffer of 3 events
	channels := map[string]struct{}{"111000000000000000": {}}
	poller := NewEventPoller(channels, fetcher, 3, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := poller.Connect(ctx, "token", IntentGuilds); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Let it poll many times to overflow the buffer of 3
	time.Sleep(300 * time.Millisecond)
	poller.Close()

	// Drain whatever made it into the buffer — should be exactly 3 (buffer cap)
	var drained int
	for {
		select {
		case <-poller.Events():
			drained++
		default:
			goto done
		}
	}
done:
	if drained > 3 {
		t.Errorf("expected at most 3 buffered events, got %d", drained)
	}
	// Key assertion: the test completes without deadlock — the non-blocking
	// send in pollChannel prevents goroutine leak when the buffer is full.
}

func TestConnector_GatewayHealthDegradedOnPollFailure(t *testing.T) {
	t.Parallel()
	ts := newTestDiscordAPI(t)
	c := New("discord-health-test")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url":        ts.URL,
			"enable_gateway": false,
		},
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Inject a poller that is not healthy (simulating sustained failures)
	unhealthyPoller := &EventPoller{
		events:  make(chan GatewayEvent, 1),
		done:    make(chan struct{}),
		cursors: make(map[string]string),
	}
	atomic.StoreInt64(&unhealthyPoller.consecutiveErrors, int64(maxGatewayReconnectAttempts+1))

	c.mu.Lock()
	c.gateway = unhealthyPoller
	c.mu.Unlock()

	health := c.Health(context.Background())
	if health != connector.HealthDegraded {
		t.Errorf("expected HealthDegraded when gateway is unhealthy, got %v", health)
	}
}

func TestConnector_CloseStopsGateway(t *testing.T) {
	t.Parallel()
	ts := newTestDiscordAPI(t)
	c := New("discord-close-gw-test")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url":        ts.URL,
			"enable_gateway": false,
		},
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	mgw := &mockGateway{events: make(chan GatewayEvent, 1), healthy: true}
	c.mu.Lock()
	c.gateway = mgw
	c.mu.Unlock()

	c.Close()

	if !mgw.closed {
		t.Error("expected gateway Close to be called on connector Close")
	}

	c.mu.RLock()
	gw := c.gateway
	c.mu.RUnlock()
	if gw != nil {
		t.Error("expected gateway to be nil after connector Close")
	}
}

func TestConnector_GatewayStartsOnConnectWithEnabledFlag(t *testing.T) {
	t.Parallel()
	// Create a test Discord API that returns empty messages for channel endpoints
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/users/@me" {
			w.Write([]byte(`{"id":"999999999999999999","username":"TestBot"}`))
			return
		}
		w.Write([]byte(`[]`))
	}))
	t.Cleanup(ts.Close)

	c := New("discord-gw-auto-start")
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		Credentials: map[string]string{"bot_token": testBotToken},
		SourceConfig: map[string]interface{}{
			"api_url":        ts.URL,
			"enable_gateway": true,
			"monitored_channels": []interface{}{
				map[string]interface{}{
					"server_id":   "900000000000000000",
					"channel_ids": []interface{}{"111000000000000000"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	c.mu.RLock()
	gw := c.gateway
	c.mu.RUnlock()
	if gw == nil {
		t.Fatal("expected gateway to be started when enable_gateway is true")
	}
	if !gw.Healthy() {
		t.Error("newly started gateway should be healthy")
	}
}
