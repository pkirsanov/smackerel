// Spec 074 SCOPE-074-04C — abandoned-clarification capture sweeper.
//
// design.md §"SCOPE-4 — Clarify-Abandoned Capture (b)" requires a
// sweeper that polls assistant_conversations for rows whose
// pending_clarify column has aged past
// capture_as_fallback.clarify_abandon_timeout, captures the
// ORIGINAL pre-clarification prompt through Policy.CaptureForUser
// with cause=clarify_abandoned, and clears pending_clarify on
// success so the row is not captured twice.
//
// The sweeper is wired by cmd/core to the existing
// assistantctx.IdleSweepTicker mechanism (same ticker family as the
// idle conversation sweep and the gauge refresher) on a fixed 60s
// cadence; the design's "60s" is independent of the timeout that
// governs eligibility — the timeout determines which rows are ready
// to capture, the cadence determines how often the query runs.
//
// Inviolability (SCN-074-A09): this sweeper has no suppression
// branch. Every eligible row either captures or remains pending for
// the next tick. There is no "skip" / "disable" path.

package capturefallback

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// AbandonedClarifyRow is the per-row payload the sweeper needs from
// the persistence layer. It mirrors
// assistantctx.PendingClarifyRow but is redeclared here so the
// capturefallback package has no upward import of assistantctx
// (avoids an import cycle with the facade that already imports both).
type AbandonedClarifyRow struct {
	UserID          string
	Transport       string
	OriginalPrompt  string
	OriginalTurnID  string
	ClarifyIntentID string
	EmitTime        time.Time
}

// AbandonedClarifyLister is the persistence seam the sweeper consumes.
// The PgStore implementation lives in
// internal/assistant/context/pg_store.go (ListAbandonedClarifies +
// ClearPendingClarify), the test implementation lives in the
// integration test alongside TP-074-13.
type AbandonedClarifyLister interface {
	// ListAbandoned returns every row whose emit_time is strictly
	// older than now - timeout. The implementation MUST use a
	// database-side time comparison so wall-clock skew between the
	// sweeper host and the database does not produce premature
	// captures.
	ListAbandoned(ctx context.Context, timeout time.Duration) ([]AbandonedClarifyRow, error)
	// Clear removes the pending_clarify entry for (userID, transport).
	// Called by the sweeper after Policy.CaptureForUser returns
	// without error so the row is not re-captured next tick.
	Clear(ctx context.Context, userID, transport string) error
}

// ClarifyAbandonSweeper executes one pass over abandoned clarifies.
// design.md §"SCOPE-4 — Clarify-Abandoned Capture (b)" prescribes
// the per-tick algorithm; this struct implements it deterministically
// so the integration test can drive RunOnce directly without waiting
// on a wall-clock ticker.
type ClarifyAbandonSweeper struct {
	lister  AbandonedClarifyLister
	policy  Policy
	timeout time.Duration
	logger  *slog.Logger
	now     func() time.Time
}

// NewClarifyAbandonSweeper constructs the sweeper. All collaborators
// are required — the sweeper has no "no-op" mode (SCN-074-A09).
func NewClarifyAbandonSweeper(
	lister AbandonedClarifyLister,
	policy Policy,
	timeout time.Duration,
	logger *slog.Logger,
) (*ClarifyAbandonSweeper, error) {
	if lister == nil {
		return nil, errors.New("capturefallback: NewClarifyAbandonSweeper requires a non-nil lister")
	}
	if policy == nil {
		return nil, errors.New("capturefallback: NewClarifyAbandonSweeper requires a non-nil policy")
	}
	if timeout <= 0 {
		return nil, errors.New("capturefallback: NewClarifyAbandonSweeper requires a positive timeout")
	}
	if logger == nil {
		return nil, errors.New("capturefallback: NewClarifyAbandonSweeper requires a non-nil logger")
	}
	return &ClarifyAbandonSweeper{
		lister:  lister,
		policy:  policy,
		timeout: timeout,
		logger:  logger,
		now:     time.Now,
	}, nil
}

// RunOnce executes a single sweep pass. The integration test calls
// this directly so the assertion path does not depend on the wall-
// clock ticker. Returns the number of rows captured (including
// dedup hits — both clear pending_clarify) and the number that
// failed (left in pending_clarify for the next tick).
//
// On per-row capture failure the sweeper logs the error and continues
// to the next row; one bad row MUST NOT prevent the rest of the
// sweep from completing. The pending_clarify column is left set on
// failure so the next tick retries.
func (s *ClarifyAbandonSweeper) RunOnce(ctx context.Context) (captured int, failed int, err error) {
	rows, err := s.lister.ListAbandoned(ctx, s.timeout)
	if err != nil {
		return 0, 0, fmt.Errorf("capturefallback: ClarifyAbandonSweeper list: %w", err)
	}
	for _, row := range rows {
		dec, decErr := s.policy.Decide(ctx, Request{
			UserID:                 row.UserID,
			Transport:              row.Transport,
			TransportMessageID:     row.OriginalTurnID,
			OriginalText:           row.OriginalPrompt,
			Cause:                  CauseClarifyAbandoned,
			TraceID:                row.ClarifyIntentID,
			IntentTraceID:          row.ClarifyIntentID,
			AbandonedClarification: true,
			OccurredAt:             s.now(),
		})
		if decErr != nil {
			failed++
			s.logger.Error("clarify-abandon sweep decide failed",
				slog.String("user_id", row.UserID),
				slog.String("transport", row.Transport),
				slog.String("err", decErr.Error()),
			)
			continue
		}
		if _, capErr := s.policy.CaptureForUser(ctx, row.UserID, dec); capErr != nil {
			failed++
			s.logger.Error("clarify-abandon sweep capture failed",
				slog.String("user_id", row.UserID),
				slog.String("transport", row.Transport),
				slog.String("err", capErr.Error()),
			)
			continue
		}
		if clrErr := s.lister.Clear(ctx, row.UserID, row.Transport); clrErr != nil {
			failed++
			s.logger.Error("clarify-abandon sweep clear failed",
				slog.String("user_id", row.UserID),
				slog.String("transport", row.Transport),
				slog.String("err", clrErr.Error()),
			)
			continue
		}
		captured++
		s.logger.Info("clarify-abandon sweep captured row",
			slog.String("user_id", row.UserID),
			slog.String("transport", row.Transport),
		)
	}
	return captured, failed, nil
}

// Run loops on the supplied cadence until ctx is cancelled. Each
// tick calls RunOnce; transient errors are logged and the loop
// continues so the assistant does not lose the sweep on a hiccup.
func (s *ClarifyAbandonSweeper) Run(ctx context.Context, cadence time.Duration) {
	if cadence <= 0 {
		s.logger.Error("ClarifyAbandonSweeper.Run requires a positive cadence; exiting",
			slog.Duration("cadence", cadence),
		)
		return
	}
	tk := time.NewTicker(cadence)
	defer tk.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
			if _, _, err := s.RunOnce(ctx); err != nil {
				s.logger.Error("clarify-abandon sweep tick failed",
					slog.String("err", err.Error()),
				)
			}
		}
	}
}
