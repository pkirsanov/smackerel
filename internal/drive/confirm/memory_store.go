package confirm

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore is an in-memory implementation of the same Resolve/Get/Create
// surface the Postgres-backed Store exposes. It is intended for unit tests
// that need to exercise the Save Service's confirmation gating without
// standing up a live database. Behavior MUST remain a faithful mirror of
// the Postgres implementation (status transitions, expiry, exactly-once
// resolve semantics) so tests written against MemoryStore catch regressions
// in either implementation.
type MemoryStore struct {
	mu   sync.Mutex
	now  func() time.Time
	ttl  time.Duration
	rows map[string]Confirmation
}

// NewMemoryStore constructs a MemoryStore. ttl defaults to 24h when zero.
func NewMemoryStore(ttl time.Duration) *MemoryStore {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &MemoryStore{now: time.Now, ttl: ttl, rows: map[string]Confirmation{}}
}

// SetClock allows tests to fix the wall clock.
func (m *MemoryStore) SetClock(now func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = now
}

// Create inserts a new pending confirmation and returns the persisted row.
func (m *MemoryStore) Create(_ context.Context, in CreateInput) (Confirmation, error) {
	if in.Kind != KindClassification && in.Kind != KindSave {
		return Confirmation{}, errors.New("confirm: invalid kind")
	}
	if in.SourceArtifactID == "" {
		return Confirmation{}, errors.New("confirm: source_artifact_id required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now().UTC()
	c := Confirmation{
		ID:               uuid.NewString(),
		Kind:             in.Kind,
		SourceArtifactID: in.SourceArtifactID,
		SaveRequestID:    in.SaveRequestID,
		RuleID:           in.RuleID,
		Payload:          in.Payload,
		Status:           StatusPending,
		ExpiresAt:        now.Add(m.ttl),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	m.rows[c.ID] = c
	return c, nil
}

// Get returns one confirmation by id.
func (m *MemoryStore) Get(_ context.Context, id string) (Confirmation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.rows[id]
	if !ok {
		return Confirmation{}, ErrNotFound
	}
	return c, nil
}

// Resolve mirrors Store.Resolve semantics: exactly-once status transition
// from pending to committed/rerouted/no_save, with expiry handling.
func (m *MemoryStore) Resolve(_ context.Context, id string, channel Channel, choice Choice) (Confirmation, error) {
	if channel != ChannelWeb && channel != ChannelTelegram {
		return Confirmation{}, errors.New("confirm: invalid channel")
	}
	newStatus, err := outcomeToStatus(choice.Outcome)
	if err != nil {
		return Confirmation{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.rows[id]
	if !ok {
		return Confirmation{}, ErrNotFound
	}
	if c.Status != StatusPending {
		return c, ErrAlreadyResolved
	}
	now := m.now().UTC()
	if c.ExpiresAt.Before(now) {
		c.Status = StatusExpired
		c.UpdatedAt = now
		m.rows[id] = c
		return c, ErrExpired
	}
	c.Status = newStatus
	c.Choice = choice
	c.Channel = channel
	c.DecidedAt = now
	c.UpdatedAt = now
	m.rows[id] = c
	return c, nil
}
