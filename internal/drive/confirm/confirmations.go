// Package confirm implements the Spec 038 Scope 6 confirmation workflow.
//
// A "confirmation" pauses a low-confidence classification or save decision
// and asks the user (web Screen 11 modal or Telegram numbered reply) to
// pick the outcome. The Save Service writes a pending row when it would
// otherwise commit a save while the artifact's classifier confidence is
// below `drive.classification.confirm_threshold` or the rule's
// `Guardrails.RequireConfirmBelow`. The HTTP handler at
// /api/v1/drive/confirmations/{id} loads the row, applies the user's
// choice, and resolves the pending decision exactly once.
//
// Anchors:
//   - SCN-038-016 — Low-confidence classification pauses routing until a
//     user picks the outcome.
//
// The confirmation row is persisted in `drive_confirmations`:
//
//	(id, kind, source_artifact_id, save_request_id, rule_id, payload,
//	 status, choice, channel, decided_at, expires_at, created_at,
//	 updated_at)
//
// `kind` is either `classification` (the proposed classification needs
// user approval before it commits to drive_files / artifacts) or `save`
// (the proposed Save Rule routing needs approval before the Save Service
// uploads bytes to the provider). Both flow through the same handler.
package confirm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Kind enumerates the confirmation kinds. Adding a new kind requires
// extending the CHECK constraint on drive_confirmations.kind in
// migrations/030_drive_confirmations_and_share_changes.sql.
type Kind string

const (
	KindClassification Kind = "classification"
	KindSave           Kind = "save"
)

// Status enumerates the lifecycle states of a confirmation row.
type Status string

const (
	StatusPending   Status = "pending"
	StatusCommitted Status = "committed"
	StatusRerouted  Status = "rerouted"
	StatusNoSave    Status = "no_save"
	StatusExpired   Status = "expired"
)

// Channel enumerates the channels through which a user resolves a
// confirmation. The empty string is allowed in the schema for backfilled
// rows that pre-date Scope 6, but new rows MUST set the channel before
// `Resolve` returns.
type Channel string

const (
	ChannelWeb      Channel = "web"
	ChannelTelegram Channel = "telegram"
)

// Outcome enumerates the user's choice when resolving a confirmation. It
// is mapped directly into Confirmation.Status.
type Outcome string

const (
	OutcomeCommit  Outcome = "commit"
	OutcomeReroute Outcome = "reroute"
	OutcomeNoSave  Outcome = "no_save"
)

// Payload is the proposal the user is being asked to confirm.
type Payload struct {
	Classification string  `json:"classification,omitempty"`
	Sensitivity    string  `json:"sensitivity,omitempty"`
	Confidence     float64 `json:"confidence,omitempty"`
	RenderedPath   string  `json:"rendered_path,omitempty"`
	Title          string  `json:"title,omitempty"`
	ProviderID     string  `json:"provider_id,omitempty"`
	RuleID         string  `json:"rule_id,omitempty"`
}

// Choice is the user's chosen outcome.
type Choice struct {
	Outcome           Outcome `json:"outcome"`
	NewClassification string  `json:"new_classification,omitempty"`
	NewRuleID         string  `json:"new_rule_id,omitempty"`
	NewRenderedPath   string  `json:"new_rendered_path,omitempty"`
	NewSensitivity    string  `json:"new_sensitivity,omitempty"`
	NoSaveReason      string  `json:"no_save_reason,omitempty"`
}

// Confirmation is the typed view of one drive_confirmations row.
type Confirmation struct {
	ID               string
	Kind             Kind
	SourceArtifactID string
	SaveRequestID    string
	RuleID           string
	Payload          Payload
	Status           Status
	Choice           Choice
	Channel          Channel
	DecidedAt        time.Time
	ExpiresAt        time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ErrNotFound is returned when a confirmation lookup misses.
var ErrNotFound = errors.New("confirm: confirmation not found")

// ErrAlreadyResolved is returned when a caller attempts to resolve a
// confirmation that already left the pending state. The Save Service
// MUST treat this as a no-op for the new caller; the original commit
// remains the canonical outcome.
var ErrAlreadyResolved = errors.New("confirm: confirmation already resolved")

// ErrExpired is returned when a caller attempts to resolve a
// confirmation whose expires_at is in the past.
var ErrExpired = errors.New("confirm: confirmation expired")

// ErrInvalidChoice is returned when the choice payload does not contain
// a recognized outcome.
var ErrInvalidChoice = errors.New("confirm: invalid choice")

// Store persists Confirmations.
type Store struct {
	pool *pgxpool.Pool
	now  func() time.Time
	ttl  time.Duration
}

// NewStore constructs a Store backed by the supplied pgx pool. ttl
// defaults to 24h when zero.
func NewStore(pool *pgxpool.Pool, ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Store{pool: pool, now: time.Now, ttl: ttl}
}

// SetClock allows tests to fix the wall clock used for created_at,
// updated_at, and expires_at calculations.
func (s *Store) SetClock(now func() time.Time) { s.now = now }

// CreateInput is the input to Create. Channel is intentionally NOT
// required at create time — the caller may not know yet whether the
// confirmation will be resolved through web or Telegram.
type CreateInput struct {
	Kind             Kind
	SourceArtifactID string
	SaveRequestID    string
	RuleID           string
	Payload          Payload
}

// Create inserts a new pending confirmation row.
func (s *Store) Create(ctx context.Context, in CreateInput) (Confirmation, error) {
	if s == nil || s.pool == nil {
		return Confirmation{}, errors.New("confirm: store not wired")
	}
	if in.Kind != KindClassification && in.Kind != KindSave {
		return Confirmation{}, fmt.Errorf("confirm: invalid kind %q", in.Kind)
	}
	if strings.TrimSpace(in.SourceArtifactID) == "" {
		return Confirmation{}, errors.New("confirm: source_artifact_id required")
	}
	payloadJSON, err := json.Marshal(in.Payload)
	if err != nil {
		return Confirmation{}, fmt.Errorf("confirm: marshal payload: %w", err)
	}
	id := uuid.NewString()
	now := s.now().UTC()
	expiresAt := now.Add(s.ttl)
	_, err = s.pool.Exec(ctx,
		`INSERT INTO drive_confirmations
		     (id, kind, source_artifact_id, save_request_id, rule_id,
		      payload, status, choice, channel, expires_at,
		      created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 'pending', '{}'::jsonb, '', $7, $8, $8)`,
		id, string(in.Kind), in.SourceArtifactID, nullableUUID(in.SaveRequestID), nullableUUID(in.RuleID),
		payloadJSON, expiresAt, now,
	)
	if err != nil {
		return Confirmation{}, fmt.Errorf("confirm: insert: %w", err)
	}
	return Confirmation{
		ID:               id,
		Kind:             in.Kind,
		SourceArtifactID: in.SourceArtifactID,
		SaveRequestID:    in.SaveRequestID,
		RuleID:           in.RuleID,
		Payload:          in.Payload,
		Status:           StatusPending,
		ExpiresAt:        expiresAt,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

// Get returns one confirmation by id.
func (s *Store) Get(ctx context.Context, id string) (Confirmation, error) {
	if s == nil || s.pool == nil {
		return Confirmation{}, errors.New("confirm: store not wired")
	}
	row := s.pool.QueryRow(ctx,
		`SELECT id, kind, source_artifact_id,
		        COALESCE(save_request_id::text, ''), COALESCE(rule_id::text, ''),
		        payload, status, choice, COALESCE(channel, ''),
		        decided_at, expires_at, created_at, updated_at
		   FROM drive_confirmations WHERE id=$1`, id)
	c, err := scanConfirmation(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Confirmation{}, ErrNotFound
	}
	if err != nil {
		return Confirmation{}, err
	}
	return c, nil
}

// Resolve applies the user's choice to a pending confirmation.
//
// Behavior:
//   - If the row does not exist → ErrNotFound.
//   - If the row is already resolved → ErrAlreadyResolved (no state change).
//   - If the row's expires_at is in the past → row is marked 'expired'
//     and ErrExpired is returned (no commit).
//   - If choice.Outcome is unknown → ErrInvalidChoice.
//   - Otherwise the row's status is updated to the choice's outcome and
//     decided_at + channel + choice are persisted. The caller is then
//     responsible for any downstream side effects (commit save,
//     reclassify, no-op).
//
// Resolve is exactly-once: a second concurrent caller observes the
// updated row and receives ErrAlreadyResolved. The save service uses
// this guarantee to ensure provider PutFile is invoked at most once.
func (s *Store) Resolve(ctx context.Context, id string, channel Channel, choice Choice) (Confirmation, error) {
	if s == nil || s.pool == nil {
		return Confirmation{}, errors.New("confirm: store not wired")
	}
	if channel != ChannelWeb && channel != ChannelTelegram {
		return Confirmation{}, fmt.Errorf("confirm: invalid channel %q", channel)
	}
	newStatus, err := outcomeToStatus(choice.Outcome)
	if err != nil {
		return Confirmation{}, err
	}
	choiceJSON, err := json.Marshal(choice)
	if err != nil {
		return Confirmation{}, fmt.Errorf("confirm: marshal choice: %w", err)
	}
	now := s.now().UTC()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Confirmation{}, fmt.Errorf("confirm: begin resolve tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx,
		`SELECT id, kind, source_artifact_id,
		        COALESCE(save_request_id::text, ''), COALESCE(rule_id::text, ''),
		        payload, status, choice, COALESCE(channel, ''),
		        decided_at, expires_at, created_at, updated_at
		   FROM drive_confirmations WHERE id=$1 FOR UPDATE`, id)
	c, err := scanConfirmation(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Confirmation{}, ErrNotFound
	}
	if err != nil {
		return Confirmation{}, err
	}
	if c.Status != StatusPending {
		return c, ErrAlreadyResolved
	}
	if c.ExpiresAt.Before(now) {
		if _, exErr := tx.Exec(ctx,
			`UPDATE drive_confirmations
			    SET status='expired', updated_at=$2
			  WHERE id=$1`, id, now,
		); exErr != nil {
			return Confirmation{}, fmt.Errorf("confirm: mark expired: %w", exErr)
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return Confirmation{}, commitErr
		}
		c.Status = StatusExpired
		c.UpdatedAt = now
		return c, ErrExpired
	}

	if _, err := tx.Exec(ctx,
		`UPDATE drive_confirmations
		    SET status=$2, choice=$3, channel=$4, decided_at=$5, updated_at=$5
		  WHERE id=$1`,
		id, string(newStatus), choiceJSON, string(channel), now,
	); err != nil {
		return Confirmation{}, fmt.Errorf("confirm: update resolve: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Confirmation{}, err
	}

	c.Status = newStatus
	c.Choice = choice
	c.Channel = channel
	c.DecidedAt = now
	c.UpdatedAt = now
	return c, nil
}

func outcomeToStatus(o Outcome) (Status, error) {
	switch o {
	case OutcomeCommit:
		return StatusCommitted, nil
	case OutcomeReroute:
		return StatusRerouted, nil
	case OutcomeNoSave:
		return StatusNoSave, nil
	default:
		return "", ErrInvalidChoice
	}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanConfirmation(r rowScanner) (Confirmation, error) {
	var (
		c             Confirmation
		kind          string
		statusValue   string
		channelValue  string
		payloadJSON   []byte
		choiceJSON    []byte
		decidedAtNull *time.Time
	)
	if err := r.Scan(
		&c.ID, &kind, &c.SourceArtifactID,
		&c.SaveRequestID, &c.RuleID,
		&payloadJSON, &statusValue, &choiceJSON, &channelValue,
		&decidedAtNull, &c.ExpiresAt, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return Confirmation{}, err
	}
	c.Kind = Kind(kind)
	c.Status = Status(statusValue)
	c.Channel = Channel(channelValue)
	if decidedAtNull != nil {
		c.DecidedAt = *decidedAtNull
	}
	if len(payloadJSON) > 0 {
		if err := json.Unmarshal(payloadJSON, &c.Payload); err != nil {
			return Confirmation{}, fmt.Errorf("confirm: unmarshal payload: %w", err)
		}
	}
	if len(choiceJSON) > 0 {
		if err := json.Unmarshal(choiceJSON, &c.Choice); err != nil {
			return Confirmation{}, fmt.Errorf("confirm: unmarshal choice: %w", err)
		}
	}
	return c, nil
}

func nullableUUID(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
