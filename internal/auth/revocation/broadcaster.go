package revocation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// EventV1 is the wire shape for the auth.revocations broadcast. Producers
// (the local instance that just revoked a token) publish one of these
// when RevokeToken commits; subscribers merge TokenID into their local
// cache so cross-instance propagation completes within NFR-AUTH-006.
type EventV1 struct {
	// Version is "v1"; future schema bumps go alongside a parallel subject.
	Version string `json:"version"`

	// TokenID is the auth_tokens.token_id PRIMARY KEY value being revoked.
	TokenID string `json:"token_id"`

	// RevokedAt is the timestamp the canonical RevokeToken transaction
	// committed; subscribers use it for telemetry only — they do not
	// trust it for ordering decisions.
	RevokedAt time.Time `json:"revoked_at"`

	// PublisherInstance is a stable identifier for the runtime instance
	// that published the event (typically the os.Hostname). Subscribers
	// log it for traceability when investigating slow propagation.
	PublisherInstance string `json:"publisher_instance"`

	// Reason is an opaque audit string echoed from RevokeToken. Stored
	// for telemetry only — the cache cares only about TokenID.
	Reason string `json:"reason,omitempty"`
}

// Broadcaster publishes and subscribes to the revocation broadcast
// subject on NATS. Construct one per runtime instance; Run blocks until
// ctx is cancelled.
type Broadcaster struct {
	nc       *nats.Conn
	subject  string
	cache    *Cache
	instance string

	mu  sync.Mutex
	sub *nats.Subscription
}

// NewBroadcaster constructs a broadcaster bound to the supplied NATS
// connection and subject. The cache is the in-process Cache that
// inbound events update. instance is the publisher_instance label for
// outbound events (typically os.Hostname()).
func NewBroadcaster(nc *nats.Conn, subject string, cache *Cache, instance string) (*Broadcaster, error) {
	if nc == nil {
		return nil, errors.New("revocation: NewBroadcaster requires non-nil *nats.Conn")
	}
	if subject == "" {
		return nil, errors.New("revocation: NewBroadcaster requires subject")
	}
	if cache == nil {
		return nil, errors.New("revocation: NewBroadcaster requires non-nil *Cache")
	}
	if instance == "" {
		return nil, errors.New("revocation: NewBroadcaster requires instance")
	}
	return &Broadcaster{
		nc:       nc,
		subject:  subject,
		cache:    cache,
		instance: instance,
	}, nil
}

// Subscribe starts the inbound subscription. Returns an error if the
// subscription cannot be established. The returned subscription is
// closed when Stop is called.
func (b *Broadcaster) Subscribe() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.sub != nil {
		return errors.New("revocation: Subscribe already called")
	}
	sub, err := b.nc.Subscribe(b.subject, b.handle)
	if err != nil {
		return fmt.Errorf("revocation: nats subscribe %q: %w", b.subject, err)
	}
	b.sub = sub
	return nil
}

// Publish broadcasts a revocation event so peer instances pick it up.
// The local cache is also updated so the publisher's own hot path
// reflects the revocation immediately even if the loopback round-trip
// stalls.
func (b *Broadcaster) Publish(tokenID, reason string) error {
	if tokenID == "" {
		return errors.New("revocation: Publish requires tokenID")
	}
	b.cache.MarkRevoked(tokenID)

	evt := EventV1{
		Version:           "v1",
		TokenID:           tokenID,
		RevokedAt:         time.Now().UTC(),
		PublisherInstance: b.instance,
		Reason:            reason,
	}
	body, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("revocation: marshal event: %w", err)
	}
	if err := b.nc.Publish(b.subject, body); err != nil {
		return fmt.Errorf("revocation: nats publish: %w", err)
	}
	return nil
}

// Stop unsubscribes the inbound subscription. Idempotent.
func (b *Broadcaster) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sub == nil {
		return nil
	}
	err := b.sub.Unsubscribe()
	b.sub = nil
	if err != nil {
		return fmt.Errorf("revocation: unsubscribe: %w", err)
	}
	return nil
}

// Run subscribes and blocks until ctx is cancelled, then unsubscribes.
// Convenience wrapper for callers that want a single-call lifecycle.
func (b *Broadcaster) Run(ctx context.Context) error {
	if err := b.Subscribe(); err != nil {
		return err
	}
	<-ctx.Done()
	return b.Stop()
}

// handle is the NATS subscription callback. Defensive — log and
// continue on malformed events rather than crashing the subscriber.
func (b *Broadcaster) handle(msg *nats.Msg) {
	if msg == nil || len(msg.Data) == 0 {
		return
	}
	var evt EventV1
	if err := json.Unmarshal(msg.Data, &evt); err != nil {
		// Drop the event silently — a noisy log on every malformed
		// message would itself be a DoS surface. Cache integrity is
		// preserved because we did not call MarkRevoked.
		return
	}
	if evt.TokenID == "" {
		return
	}
	b.cache.MarkRevoked(evt.TokenID)
}
