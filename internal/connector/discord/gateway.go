package discord

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// defaultGatewayBufferSize is the default capacity of the gateway event buffer channel.
	defaultGatewayBufferSize = 10000
	// defaultPollInterval is the default time between REST polling cycles.
	defaultPollInterval = 5 * time.Second
	// maxGatewayReconnectAttempts is the max consecutive poll failures before marking unhealthy.
	maxGatewayReconnectAttempts = 5
	// gatewayPollLimit is the max messages fetched per poll request.
	gatewayPollLimit = 100
	// gatewayCloseTimeout is the max time to wait for goroutines on Close.
	gatewayCloseTimeout = 5 * time.Second
	// maxGatewayBackoff caps the exponential backoff duration for poll retries.
	maxGatewayBackoff = 16 * time.Second

	// Discord Gateway intent bit flags.
	IntentGuilds         = 1 << 0  // 1
	IntentGuildMessages  = 1 << 9  // 512
	IntentMessageContent = 1 << 15 // 32768
)

// GatewayEvent represents a real-time event from the Discord Gateway.
type GatewayEvent struct {
	Type    string
	Message DiscordMessage
}

// GatewayClient is the interface for receiving real-time Discord events.
// The EventPoller implementation uses REST polling; a future WebSocket
// implementation can satisfy the same interface without changes to the
// Connector's Sync() integration.
type GatewayClient interface {
	Connect(ctx context.Context, token string, intents int) error
	Events() <-chan GatewayEvent
	Healthy() bool
	Close() error
}

// MessageFetcher retrieves messages from a Discord channel via REST API.
// It abstracts the HTTP call so the EventPoller can be tested with a mock.
type MessageFetcher func(ctx context.Context, channelID, afterID string, limit int) ([]DiscordMessage, error)

// EventPoller implements GatewayClient by polling the Discord REST API at a
// configurable interval. It buffers MESSAGE_CREATE events on a channel that
// the Connector drains during Sync(). When a real WebSocket Gateway is added,
// the EventPoller can be swapped out without changing the Connector.
type EventPoller struct {
	events            chan GatewayEvent
	done              chan struct{}
	mu                sync.RWMutex
	connected         bool
	cursors           map[string]string   // channelID → last seen message snowflake
	channels          map[string]struct{} // monitored channel ID set
	pollInterval      time.Duration
	fetcher           MessageFetcher
	closeOnce         sync.Once
	wg                sync.WaitGroup
	intents           int
	consecutiveErrors int64 // atomic; health threshold counter
}

// Compile-time interface check.
var _ GatewayClient = (*EventPoller)(nil)

// NewEventPoller creates a new EventPoller for the given monitored channels.
// bufferSize sets the event channel capacity (default 10000).
// pollInterval sets the time between poll cycles (default 5s).
func NewEventPoller(channels map[string]struct{}, fetcher MessageFetcher, bufferSize int, pollInterval time.Duration) *EventPoller {
	if bufferSize <= 0 {
		bufferSize = defaultGatewayBufferSize
	}
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}
	return &EventPoller{
		events:       make(chan GatewayEvent, bufferSize),
		done:         make(chan struct{}),
		cursors:      make(map[string]string),
		channels:     channels,
		pollInterval: pollInterval,
		fetcher:      fetcher,
	}
}

// Connect starts a polling goroutine for each monitored channel.
// The intents parameter is recorded for future WebSocket Gateway compatibility
// (GUILDS=1, GUILD_MESSAGES=512, MESSAGE_CONTENT=32768).
func (ep *EventPoller) Connect(ctx context.Context, token string, intents int) error {
	ep.mu.Lock()
	if ep.connected {
		ep.mu.Unlock()
		return fmt.Errorf("event poller already connected")
	}
	ep.intents = intents
	ep.connected = true
	ep.mu.Unlock()

	for chID := range ep.channels {
		ep.wg.Add(1)
		go ep.pollChannel(ctx, chID)
	}
	return nil
}

// Events returns the read-only event channel.
func (ep *EventPoller) Events() <-chan GatewayEvent {
	return ep.events
}

// Healthy returns true if consecutive poll errors are below the reconnect threshold.
func (ep *EventPoller) Healthy() bool {
	return atomic.LoadInt64(&ep.consecutiveErrors) < int64(maxGatewayReconnectAttempts)
}

// Close stops all polling goroutines and waits up to gatewayCloseTimeout.
func (ep *EventPoller) Close() error {
	ep.closeOnce.Do(func() {
		ep.mu.Lock()
		ep.connected = false
		ep.mu.Unlock()
		close(ep.done)
	})
	waitDone := make(chan struct{})
	go func() {
		ep.wg.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(gatewayCloseTimeout):
		slog.Warn("discord event poller close timed out waiting for goroutines")
	}
	return nil
}

// pollChannel is the per-channel polling loop. It fetches new messages via REST,
// buffers them as GatewayEvents, and retries with exponential backoff on failure.
func (ep *EventPoller) pollChannel(ctx context.Context, channelID string) {
	defer ep.wg.Done()
	for {
		select {
		case <-ep.done:
			return
		case <-ctx.Done():
			return
		default:
		}

		ep.mu.RLock()
		cursor := ep.cursors[channelID]
		ep.mu.RUnlock()

		msgs, err := ep.fetcher(ctx, channelID, cursor, gatewayPollLimit)
		if err != nil {
			n := atomic.AddInt64(&ep.consecutiveErrors, 1)
			slog.Warn("discord event poller fetch failed",
				"channel", channelID, "error", err, "consecutive_errors", n)

			backoffSecs := math.Pow(2, float64(n-1))
			backoff := time.Duration(math.Min(
				float64(time.Second)*backoffSecs,
				float64(maxGatewayBackoff),
			))
			select {
			case <-ep.done:
				return
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				continue
			}
		}

		// Reset consecutive errors on successful fetch.
		atomic.StoreInt64(&ep.consecutiveErrors, 0)

		var maxID string
		for _, msg := range msgs {
			// Non-blocking send: drop events when the buffer is full rather
			// than blocking the poll loop (avoids goroutine leak).
			select {
			case ep.events <- GatewayEvent{Type: "MESSAGE_CREATE", Message: msg}:
			default:
				slog.Warn("discord event poller buffer full, dropping event",
					"channel", channelID, "message_id", msg.ID)
			}
			if snowflakeGreater(msg.ID, maxID) {
				maxID = msg.ID
			}
		}

		if maxID != "" {
			ep.mu.Lock()
			if snowflakeGreater(maxID, ep.cursors[channelID]) {
				ep.cursors[channelID] = maxID
			}
			ep.mu.Unlock()
		}

		select {
		case <-ep.done:
			return
		case <-ctx.Done():
			return
		case <-time.After(ep.pollInterval):
		}
	}
}

// drainGatewayEvents reads all buffered events from a GatewayClient without blocking.
// Returns nil if gw is nil.
func drainGatewayEvents(gw GatewayClient) []GatewayEvent {
	if gw == nil {
		return nil
	}
	var events []GatewayEvent
	for {
		select {
		case ev := <-gw.Events():
			events = append(events, ev)
		default:
			return events
		}
	}
}

// AddChannels registers new channel IDs (e.g. discovered threads) for polling.
// Only channels present in cursors but not already in the poller's channel set
// are added. A new polling goroutine is started for each new channel.
func (ep *EventPoller) AddChannels(cursors map[string]string, monitoredParents map[string]struct{}) {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	if !ep.connected {
		return
	}
	for chID := range cursors {
		if _, exists := ep.channels[chID]; exists {
			continue
		}
		// Only add thread IDs that aren't already monitored parent channels
		// (parent channels are already being polled)
		if _, isParent := monitoredParents[chID]; isParent {
			continue
		}
		ep.channels[chID] = struct{}{}
		ep.wg.Add(1)
		go ep.pollChannel(context.Background(), chID)
	}
}
