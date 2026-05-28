// Spec 061 SCOPE-08 — PostgreSQL-backed assistant_proposal audit
// writer.
//
// Writes one row per terminal confirm-card outcome to the existing
// `artifacts` table, populating the additive
// `assistant_proposal_payload` JSONB column added by migration 042.
//
// Required artifacts columns: id, artifact_type, title,
// content_hash, source_id — all of which are NOT NULL. This writer
// populates them deterministically from the ProposalArtifact so the
// audit row is unique per (ConfirmRef, Outcome) and deduplicates
// gracefully on retry (content_hash partial-unique index in the
// existing schema).

package confirm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProposalArtifactType is the artifacts.artifact_type value written
// for confirm-card audit rows. The migration 042 partial index
// targets this exact string.
const ProposalArtifactType = "assistant_proposal"

// SystemSourceID is the canonical source_id used for audit rows
// that have no external source. The artifacts table requires
// source_id NOT NULL, so the audit writer uses this sentinel for
// system-generated audit entries.
const SystemSourceID = "system:assistant"

// PgWriter implements Writer against PostgreSQL via a pgxpool.
type PgWriter struct {
	pool *pgxpool.Pool
}

// NewPgWriter constructs a PgWriter. Panics on nil pool.
func NewPgWriter(pool *pgxpool.Pool) *PgWriter {
	if pool == nil {
		panic("confirm: NewPgWriter requires non-nil pool")
	}
	return &PgWriter{pool: pool}
}

// WriteProposalArtifact inserts one audit row. The content_hash is
// derived from (ConfirmRef, Outcome) so a same-tuple retry collides
// on the partial-unique index and the ON CONFLICT clause makes the
// write idempotent.
func (w *PgWriter) WriteProposalArtifact(ctx context.Context, p ProposalArtifact) error {
	if err := validateAudit(p); err != nil {
		return err
	}
	payload := map[string]any{
		"confirm_ref":      p.ConfirmRef,
		"scenario_id":      p.ScenarioID,
		"user_id":          p.UserID,
		"transport":        p.Transport,
		"proposed_action":  p.ProposedAction,
		"outcome":          string(p.Outcome),
		"scheduled_job_id": p.ScheduledJobID,
		"terminal_at":      p.TerminalAt.UTC(),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("confirm.WriteProposalArtifact: marshal payload: %w", err)
	}

	hash := sha256.Sum256([]byte(fmt.Sprintf("%s|%s", p.ConfirmRef, p.Outcome)))
	contentHash := hex.EncodeToString(hash[:])

	const q = `
INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, assistant_proposal_payload, processing_status, capture_method)
VALUES ($1, $2, $3, $4, $5, $6, 'completed', 'system')
ON CONFLICT (content_hash) WHERE content_hash IS NOT NULL DO NOTHING
`
	id := uuid.NewString()
	title := fmt.Sprintf("assistant_proposal/%s/%s", p.Outcome, p.ConfirmRef)
	if _, err := w.pool.Exec(ctx, q, id, ProposalArtifactType, title, contentHash, SystemSourceID, payloadJSON); err != nil {
		return fmt.Errorf("confirm.WriteProposalArtifact: insert: %w", err)
	}
	return nil
}

func validateAudit(p ProposalArtifact) error {
	switch {
	case p.ConfirmRef == "":
		return errors.New("confirm.WriteProposalArtifact: ConfirmRef required")
	case p.ScenarioID == "":
		return errors.New("confirm.WriteProposalArtifact: ScenarioID required")
	case p.UserID == "":
		return errors.New("confirm.WriteProposalArtifact: UserID required")
	case p.Transport == "":
		return errors.New("confirm.WriteProposalArtifact: Transport required")
	case p.Outcome == "":
		return errors.New("confirm.WriteProposalArtifact: Outcome required")
	case p.TerminalAt.IsZero():
		return errors.New("confirm.WriteProposalArtifact: TerminalAt required")
	}
	switch p.Outcome {
	case OutcomeConfirmed, OutcomeDiscardedUser, OutcomeDiscardedTimeout:
	default:
		return fmt.Errorf("confirm.WriteProposalArtifact: invalid Outcome %q", p.Outcome)
	}
	return nil
}
