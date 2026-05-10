// Package revocation owns the in-process revocation cache that the
// bearer auth hot path consults on every request. The cache combines
// three sources to keep p50 verification under a microsecond budget
// (NFR-AUTH-002):
//
//  1. Bootstrap on startup from BearerStore.LoadRevokedTokenIDs so the
//     cache is hot before the first request lands.
//  2. NATS subscribe on auth.revocation_nats_subject for cross-instance
//     fan-out; each instance publishes its own revocations and consumes
//     every other instance's broadcasts.
//  3. Periodic DB refresh as the failure-mode backstop when the NATS
//     channel is down or partitioned.
//
// All concurrent reads are lock-free via sync.Map; writes happen in the
// NATS subscription goroutine and the periodic refresh goroutine.
package revocation

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Loader is the interface the cache uses to bootstrap and refresh
// revocations from the canonical store. Implemented by
// internal/auth.BearerStore in production; tests substitute a stub.
type Loader interface {
	LoadRevokedTokenIDs(ctx context.Context) ([]string, error)
}

// Cache is the in-process set of revoked token IDs. The zero value is
// NOT safe to use; construct via NewCache. Lookup is constant-time and
// lock-free; updates take a brief lock on the underlying sync.Map.
type Cache struct {
	revoked sync.Map // map[string]struct{}
	count   atomic.Int64
}

// NewCache returns an empty Cache. BootstrapFromDB MUST be called before
// the cache participates in request handling, or hot-path checks will
// silently let revoked tokens through during the warm-up window.
func NewCache() *Cache {
	return &Cache{}
}

// IsRevoked reports whether the supplied token_id has been revoked. The
// hot path in the bearer middleware calls this on every request; it
// MUST stay lock-free.
func (c *Cache) IsRevoked(tokenID string) bool {
	if tokenID == "" {
		return false
	}
	_, ok := c.revoked.Load(tokenID)
	return ok
}

// MarkRevoked inserts a token_id into the cache. Idempotent.
func (c *Cache) MarkRevoked(tokenID string) {
	if tokenID == "" {
		return
	}
	if _, loaded := c.revoked.LoadOrStore(tokenID, struct{}{}); !loaded {
		c.count.Add(1)
	}
}

// Size returns the number of revoked token IDs in the cache. Used by
// telemetry and health endpoints.
func (c *Cache) Size() int64 {
	return c.count.Load()
}

// BootstrapFromDB populates the cache from the loader. Returns the
// number of token IDs loaded so callers can log a "auth cache hot"
// startup line for SCN-AUTH-008 evidence.
func (c *Cache) BootstrapFromDB(ctx context.Context, loader Loader) (int, error) {
	if loader == nil {
		return 0, errors.New("revocation: BootstrapFromDB requires non-nil Loader")
	}
	ids, err := loader.LoadRevokedTokenIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("revocation: bootstrap: %w", err)
	}
	for _, id := range ids {
		c.MarkRevoked(id)
	}
	return len(ids), nil
}

// Refresh re-loads revocations from the loader and merges them into the
// cache. Designed to be called on a fixed interval as the failure-mode
// backstop when the NATS broadcast channel is partitioned. Returns the
// count of NEW revocations observed in this refresh (cache-sized
// before vs after) so callers can log non-zero deltas as a soft alert.
func (c *Cache) Refresh(ctx context.Context, loader Loader) (newlyAdded int, err error) {
	if loader == nil {
		return 0, errors.New("revocation: Refresh requires non-nil Loader")
	}
	before := c.count.Load()
	ids, err := loader.LoadRevokedTokenIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("revocation: refresh: %w", err)
	}
	for _, id := range ids {
		c.MarkRevoked(id)
	}
	after := c.count.Load()
	delta := int(after - before)
	if delta < 0 {
		delta = 0
	}
	return delta, nil
}

// RunPeriodicRefresh blocks until ctx is cancelled, calling Refresh on
// the supplied interval. Logs and ignores transient loader errors so a
// brief DB outage does not crash the runtime; the next tick retries.
// Use this in a dedicated goroutine, NOT the request hot path.
func (c *Cache) RunPeriodicRefresh(ctx context.Context, loader Loader, interval time.Duration, onError func(error)) {
	if interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := c.Refresh(ctx, loader); err != nil {
				if onError != nil {
					onError(err)
				}
			}
		}
	}
}
