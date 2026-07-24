package proactive

import (
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
)

// NudgeRef is an opaque, ULID-shaped (26-char) routing token minted when a
// proactive card is dispatched to a channel. It is the ONLY nudge identity that
// reaches a transport wire (callback_data, reply.id, web body) or telemetry;
// the content_key it maps to never leaves the process. It is the sole anti-leak
// boundary (FR-107-028).
type NudgeRef string

// nudgeEntry is the process-local routing state behind one ref. It carries no
// durable business data — only an opaque handle onto a content_key already
// owned by the surfacing controller.
type nudgeEntry struct {
	contentKey string
	producer   surfacing.Producer
	channel    surfacing.Channel
	principal  string
	issuedAt   time.Time
	consumed   bool
}

// ResolveStatus is the closed outcome set of a registry lookup. It never leaks
// a content_key on a non-OK path; a stale, expired, or already-handled ref
// yields an honest render, never a silent success.
type ResolveStatus int

const (
	// ResolveExpired means the ref is unknown or past its TTL. A late tap on any
	// channel resolves here and renders StateExpired.
	ResolveExpired ResolveStatus = iota
	// ResolveAlreadyHandled means the ref was already consumed by a prior ack.
	// A second tap (act-then-snooze, or a duplicate delivery) resolves here and
	// renders StateAlreadyHandled — the ack is idempotent.
	ResolveAlreadyHandled
	// ResolveOK means the ref is live and (for Consume) was consumed by this call.
	ResolveOK
)

// Resolved is the non-leaking view a caller may act on. It intentionally omits
// nothing sensitive beyond the content_key/principal the caller already needs
// to acknowledge; callers MUST NOT place ContentKey on any wire.
type Resolved struct {
	ContentKey string
	Principal  string
	Producer   surfacing.Producer
	Channel    surfacing.Channel
}

// nudgeRegistryGCThreshold bounds opportunistic GC, mirroring
// surfacing.InMemoryAck / DedupeIndex.
const nudgeRegistryGCThreshold = 4096

// NudgeRegistry is the ephemeral, process-local, expiring ref -> entry map. It
// mirrors the existing DedupeIndex / InMemoryAck process-local pattern: it is
// NOT a durable business or client store, holds only opaque refs onto
// controller-owned content_keys, and is dropped on restart (stale refs then
// resolve to ResolveExpired). Safe for concurrent use.
type NudgeRegistry struct {
	mu      sync.Mutex
	entries map[NudgeRef]nudgeEntry
	ttl     time.Duration
	clock   func() time.Time
	mint    func() NudgeRef
}

// NewNudgeRegistry constructs a registry with the SST-resolved TTL. ttl MUST be
// > 0 (the config loader validates it is >= max(suppression_window_hours,
// dedupe_window_hours) so a late tap resolves to an honest expired render
// rather than a silent miss); a non-positive ttl is clamped to a single hour to
// stay fail-safe, but the loader is the real guard.
func NewNudgeRegistry(ttl time.Duration) *NudgeRegistry {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &NudgeRegistry{
		entries: make(map[NudgeRef]nudgeEntry),
		ttl:     ttl,
		clock:   time.Now,
		mint:    func() NudgeRef { return NudgeRef(ulid.Make().String()) },
	}
}

// Mint records a dispatched card and returns its opaque, ULID-shaped ref. The
// ref — never the content_key — is what a channel renderer encodes onto the
// wire.
func (r *NudgeRegistry) Mint(contentKey string, producer surfacing.Producer, channel surfacing.Channel, principal string) NudgeRef {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gcLocked()
	ref := r.mint()
	// Guard against the (astronomically unlikely) collision so a mint never
	// silently overwrites a live entry.
	for {
		if _, exists := r.entries[ref]; !exists {
			break
		}
		ref = r.mint()
	}
	r.entries[ref] = nudgeEntry{
		contentKey: contentKey,
		producer:   producer,
		channel:    channel,
		principal:  principal,
		issuedAt:   r.clock(),
	}
	return ref
}

// Peek resolves a ref WITHOUT consuming it — for rendering or inspection. It
// returns ResolveExpired for an unknown or past-TTL ref, ResolveAlreadyHandled
// for a consumed ref, and ResolveOK otherwise.
func (r *NudgeRegistry) Peek(ref NudgeRef) (Resolved, ResolveStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, status := r.lookupLocked(ref)
	if status != ResolveOK {
		return Resolved{}, status
	}
	return resolvedFrom(entry), ResolveOK
}

// Consume resolves a ref and marks it handled (idempotent ack). A second
// Consume of the same ref returns ResolveAlreadyHandled; an unknown or expired
// ref returns ResolveExpired. Neither error path returns the content_key.
func (r *NudgeRegistry) Consume(ref NudgeRef) (Resolved, ResolveStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, status := r.lookupLocked(ref)
	if status != ResolveOK {
		return Resolved{}, status
	}
	entry.consumed = true
	r.entries[ref] = entry
	return resolvedFrom(entry), ResolveOK
}

// lookupLocked classifies a ref. Caller MUST hold r.mu.
func (r *NudgeRegistry) lookupLocked(ref NudgeRef) (nudgeEntry, ResolveStatus) {
	entry, ok := r.entries[ref]
	if !ok {
		return nudgeEntry{}, ResolveExpired
	}
	if r.clock().Sub(entry.issuedAt) >= r.ttl {
		return nudgeEntry{}, ResolveExpired
	}
	if entry.consumed {
		return nudgeEntry{}, ResolveAlreadyHandled
	}
	return entry, ResolveOK
}

// gcLocked drops expired entries opportunistically when the map grows past the
// threshold, keeping memory bounded over long uptimes. Caller MUST hold r.mu.
func (r *NudgeRegistry) gcLocked() {
	if len(r.entries) <= nudgeRegistryGCThreshold {
		return
	}
	now := r.clock()
	for k, e := range r.entries {
		if now.Sub(e.issuedAt) >= r.ttl {
			delete(r.entries, k)
		}
	}
}

func resolvedFrom(e nudgeEntry) Resolved {
	return Resolved{
		ContentKey: e.contentKey,
		Principal:  e.principal,
		Producer:   e.producer,
		Channel:    e.channel,
	}
}
