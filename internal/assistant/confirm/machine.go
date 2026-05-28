// Spec 061 SCOPE-08 — confirm-card state machine.
//
// Per design §5.4, the Machine drives a three-outcome lifecycle
// over the existing `assistant_conversations.pending_confirm` row.
// All transitions are single-flight: the pending row is cleared
// BEFORE the audit row is written so a racing callback observes
// "no pending" and short-circuits.

package confirm

import (
	"context"
	"errors"
	"fmt"
	"time"

	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
)

// Outcome enumerates the three terminal outcomes of a confirm-card
// lifecycle, per design §5.4 + UX §14.A.6.
type Outcome string

const (
	OutcomeConfirmed        Outcome = "confirmed"
	OutcomeDiscardedUser    Outcome = "discarded_user"
	OutcomeDiscardedTimeout Outcome = "discarded_timeout"
)

// ProposalInput is the payload the facade hands to Machine.Propose
// after the scenario's first tool returns phase="proposed".
type ProposalInput struct {
	UserID         string
	Transport      string
	ScenarioID     string
	ConfirmRef     string
	ProposedAction string
	Payload        []byte
	ExpiresAt      time.Time
}

// ConfirmResult is what Machine.Confirm returns on a positive
// callback. ScheduledJobID is populated by the audit-write path so
// the caller can include it in the audit row; if the caller has
// the ID before invoking the Machine it can be passed via the
// optional ScheduledJobID field on ConfirmInput.
type ConfirmResult struct {
	Payload        []byte
	ScenarioID     string
	OriginalUserID string
}

// Writer is the audit-write surface the Machine depends on. The
// PostgreSQL-backed default (`PgWriter`) inserts one row per
// terminal outcome into the `artifacts` table with the
// `assistant_proposal_payload` JSONB populated per design §6.3
// step 3.
type Writer interface {
	WriteProposalArtifact(ctx context.Context, p ProposalArtifact) error
}

// ProposalArtifact is the audit row written for one terminal
// outcome. UserID/Transport identify the confirm-card owner;
// ScheduledJobID is populated only when Outcome=confirmed.
type ProposalArtifact struct {
	ConfirmRef     string
	ScenarioID     string
	UserID         string
	Transport      string
	ProposedAction string
	Outcome        Outcome
	ScheduledJobID string
	TerminalAt     time.Time
}

// Machine is the confirm-card state machine. Construct via NewMachine.
type Machine struct {
	store  assistantctx.Store
	writer Writer
}

// NewMachine constructs a Machine over the supplied conversation
// store and audit writer. Panics on nil arguments — both are
// required and a nil dependency would silently drop confirms or
// audit rows.
func NewMachine(store assistantctx.Store, writer Writer) *Machine {
	if store == nil {
		panic("confirm: NewMachine requires non-nil assistantctx.Store")
	}
	if writer == nil {
		panic("confirm: NewMachine requires non-nil Writer")
	}
	return &Machine{store: store, writer: writer}
}

// Propose persists a new pending confirm for (UserID, Transport).
// Any existing pending confirm on that key is REPLACED — the design
// (§5.4) treats at most one outstanding ConfirmCard per user per
// transport.
func (m *Machine) Propose(ctx context.Context, in ProposalInput, now time.Time) error {
	if err := validateProposal(in); err != nil {
		return err
	}
	conv, _, err := m.store.Load(ctx, in.UserID, in.Transport)
	if err != nil {
		return fmt.Errorf("confirm.Propose: load conversation: %w", err)
	}
	conv.UserID = in.UserID
	conv.Transport = in.Transport
	conv.PendingConfirm = &assistantctx.PendingConfirm{
		ConfirmRef:     in.ConfirmRef,
		ScenarioID:     in.ScenarioID,
		ProposedAction: in.ProposedAction,
		Payload:        append([]byte(nil), in.Payload...),
		ExpiresAt:      in.ExpiresAt,
	}
	conv.LastActivityAt = now.UTC()
	if conv.SchemaVersion == 0 {
		conv.SchemaVersion = 1
	}
	if err := m.store.Persist(ctx, conv); err != nil {
		return fmt.Errorf("confirm.Propose: persist conversation: %w", err)
	}
	return nil
}

// ConfirmInput is the payload Machine.Confirm consumes on a positive
// callback. ScheduledJobID is what notification_execute returned
// from spec 054's scheduler.
type ConfirmInput struct {
	UserID         string
	Transport      string
	ConfirmRef     string
	ScheduledJobID string
}

// ErrPendingNotFound is returned when Confirm or Discard cannot find
// a pending confirm matching the supplied (user_id, transport,
// confirm_ref) tuple. Callers SHOULD treat this as "the proposal
// expired or was already handled" — race-safe single-flight semantics
// depend on this.
var ErrPendingNotFound = errors.New("confirm: pending confirm not found or already resolved")

// Confirm marks the pending confirm as terminally confirmed:
//   - Clear pending_confirm from the conversation row (single-flight
//     guard).
//   - Write the assistant_proposal audit row with
//     Outcome=confirmed and the supplied ScheduledJobID.
//
// Returns ErrPendingNotFound when no matching pending exists or the
// ConfirmRef does not match.
func (m *Machine) Confirm(ctx context.Context, in ConfirmInput, now time.Time) (ConfirmResult, error) {
	if in.UserID == "" || in.Transport == "" || in.ConfirmRef == "" {
		return ConfirmResult{}, errors.New("confirm.Confirm: UserID, Transport, ConfirmRef required")
	}
	conv, ok, err := m.store.Load(ctx, in.UserID, in.Transport)
	if err != nil {
		return ConfirmResult{}, fmt.Errorf("confirm.Confirm: load conversation: %w", err)
	}
	if !ok || conv.PendingConfirm == nil || conv.PendingConfirm.ConfirmRef != in.ConfirmRef {
		return ConfirmResult{}, ErrPendingNotFound
	}
	pending := *conv.PendingConfirm
	// Single-flight: clear pending BEFORE writing the audit row so a
	// racing callback hits ErrPendingNotFound.
	conv.PendingConfirm = nil
	conv.LastActivityAt = now.UTC()
	if err := m.store.Persist(ctx, conv); err != nil {
		return ConfirmResult{}, fmt.Errorf("confirm.Confirm: persist conversation (clear pending): %w", err)
	}
	audit := ProposalArtifact{
		ConfirmRef:     pending.ConfirmRef,
		ScenarioID:     pending.ScenarioID,
		UserID:         in.UserID,
		Transport:      in.Transport,
		ProposedAction: pending.ProposedAction,
		Outcome:        OutcomeConfirmed,
		ScheduledJobID: in.ScheduledJobID,
		TerminalAt:     now.UTC(),
	}
	if err := m.writer.WriteProposalArtifact(ctx, audit); err != nil {
		return ConfirmResult{}, fmt.Errorf("confirm.Confirm: write audit: %w", err)
	}
	return ConfirmResult{
		Payload:        append([]byte(nil), pending.Payload...),
		ScenarioID:     pending.ScenarioID,
		OriginalUserID: in.UserID,
	}, nil
}

// DiscardInput is the payload Machine.Discard consumes on a negative
// callback (user cancelled the confirm card).
type DiscardInput struct {
	UserID     string
	Transport  string
	ConfirmRef string
}

// Discard marks the pending confirm as terminally discarded by the
// user. Same single-flight guard as Confirm. Writes an
// assistant_proposal audit row with Outcome=discarded_user.
func (m *Machine) Discard(ctx context.Context, in DiscardInput, now time.Time) error {
	if in.UserID == "" || in.Transport == "" || in.ConfirmRef == "" {
		return errors.New("confirm.Discard: UserID, Transport, ConfirmRef required")
	}
	conv, ok, err := m.store.Load(ctx, in.UserID, in.Transport)
	if err != nil {
		return fmt.Errorf("confirm.Discard: load conversation: %w", err)
	}
	if !ok || conv.PendingConfirm == nil || conv.PendingConfirm.ConfirmRef != in.ConfirmRef {
		return ErrPendingNotFound
	}
	pending := *conv.PendingConfirm
	conv.PendingConfirm = nil
	conv.LastActivityAt = now.UTC()
	if err := m.store.Persist(ctx, conv); err != nil {
		return fmt.Errorf("confirm.Discard: persist conversation (clear pending): %w", err)
	}
	audit := ProposalArtifact{
		ConfirmRef:     pending.ConfirmRef,
		ScenarioID:     pending.ScenarioID,
		UserID:         in.UserID,
		Transport:      in.Transport,
		ProposedAction: pending.ProposedAction,
		Outcome:        OutcomeDiscardedUser,
		TerminalAt:     now.UTC(),
	}
	if err := m.writer.WriteProposalArtifact(ctx, audit); err != nil {
		return fmt.Errorf("confirm.Discard: write audit: %w", err)
	}
	return nil
}

// SweepResult reports what one SweepTimeouts call did.
type SweepResult struct {
	Expired int
}

// SweepTimeouts examines every conversation whose pending_confirm
// has ExpiresAt <= now and writes a discarded_timeout audit row for
// each, then clears the pending row.
//
// NOTE: this is a per-Machine helper exposed for the facade's
// idle-sweep ticker. It iterates a caller-supplied list of pending
// confirms (typically loaded by the facade via a single batch
// SELECT against assistant_conversations) so the Machine itself
// doesn't need a "SweepAll" SQL surface. The PG-backed Writer is
// what actually persists.
func (m *Machine) SweepTimeouts(ctx context.Context, expired []ExpiredPending, now time.Time) (SweepResult, error) {
	res := SweepResult{}
	for _, e := range expired {
		conv, ok, err := m.store.Load(ctx, e.UserID, e.Transport)
		if err != nil {
			return res, fmt.Errorf("confirm.SweepTimeouts: load %s/%s: %w", e.UserID, e.Transport, err)
		}
		if !ok || conv.PendingConfirm == nil || conv.PendingConfirm.ConfirmRef != e.ConfirmRef {
			// Already resolved by a racing confirm/discard — skip.
			continue
		}
		pending := *conv.PendingConfirm
		conv.PendingConfirm = nil
		conv.LastActivityAt = now.UTC()
		if err := m.store.Persist(ctx, conv); err != nil {
			return res, fmt.Errorf("confirm.SweepTimeouts: clear pending %s/%s: %w", e.UserID, e.Transport, err)
		}
		audit := ProposalArtifact{
			ConfirmRef:     pending.ConfirmRef,
			ScenarioID:     pending.ScenarioID,
			UserID:         e.UserID,
			Transport:      e.Transport,
			ProposedAction: pending.ProposedAction,
			Outcome:        OutcomeDiscardedTimeout,
			TerminalAt:     now.UTC(),
		}
		if err := m.writer.WriteProposalArtifact(ctx, audit); err != nil {
			return res, fmt.Errorf("confirm.SweepTimeouts: write audit %s/%s: %w", e.UserID, e.Transport, err)
		}
		res.Expired++
	}
	return res, nil
}

// ExpiredPending names one (user_id, transport, confirm_ref) tuple
// the facade's idle-sweep query identified as expired. The Machine
// re-loads the conversation row before each terminal write so a
// racing Confirm/Discard cannot be silently overwritten.
type ExpiredPending struct {
	UserID     string
	Transport  string
	ConfirmRef string
}

func validateProposal(in ProposalInput) error {
	switch {
	case in.UserID == "":
		return errors.New("confirm.Propose: UserID required")
	case in.Transport == "":
		return errors.New("confirm.Propose: Transport required")
	case in.ScenarioID == "":
		return errors.New("confirm.Propose: ScenarioID required")
	case in.ConfirmRef == "":
		return errors.New("confirm.Propose: ConfirmRef required")
	case in.ProposedAction == "":
		return errors.New("confirm.Propose: ProposedAction required")
	case len(in.Payload) == 0:
		return errors.New("confirm.Propose: Payload required")
	case in.ExpiresAt.IsZero():
		return errors.New("confirm.Propose: ExpiresAt required")
	}
	return nil
}
