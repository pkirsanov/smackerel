// policyimpl.go — spec 075 SCOPE-2 Policy implementation composing
// the catalog, ledger, window-state resolver, and residual telemetry
// seams from Scope 1.
//
// Decision table (SCN-075-A01..A03, A09):
//
//	not-retired token            → Matched=false, ServeNL=true (passthrough)
//	retired + closed window      → Matched=true,  ServeNL=false (Scope 5 owns response)
//	retired + paused window      → Matched=true,  ServeNL=true, ShowNotice=false
//	retired + open + already     → Matched=true,  ServeNL=true, ShowNotice=false
//	retired + open + first time  → Matched=true,  ServeNL=true, ShowNotice=true,
//	                               ledger.MarkShown invoked
//
// Cross-transport dedup (SCN-075-A09) holds by construction: the
// ledger key is (user_id, retired_command, window_id) — transport is
// not part of the key, so a notice shown on Telegram is invisible to
// the web transport.
package legacyretirement

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// PolicyConfig wires the Scope 1 seams into a concrete Policy.
type PolicyConfig struct {
	Catalog       RetiredCommandCatalog
	Ledger        NoticeLedger
	StateResolver WindowStateResolver
	Telemetry     ResidualTelemetry
	BucketHasher  *UserBucketHasher
	WindowID      string
	Clock         func() time.Time
}

type policyImpl struct {
	cfg PolicyConfig
}

// NewPolicy returns a Policy that wires the Scope 1 contracts.
func NewPolicy(cfg PolicyConfig) (Policy, error) {
	if cfg.Catalog == nil {
		return nil, fmt.Errorf("legacyretirement: PolicyConfig.Catalog is nil")
	}
	if cfg.Ledger == nil {
		return nil, fmt.Errorf("legacyretirement: PolicyConfig.Ledger is nil")
	}
	if cfg.StateResolver == nil {
		return nil, fmt.Errorf("legacyretirement: PolicyConfig.StateResolver is nil")
	}
	if cfg.BucketHasher == nil {
		return nil, fmt.Errorf("legacyretirement: PolicyConfig.BucketHasher is nil")
	}
	if strings.TrimSpace(cfg.WindowID) == "" {
		return nil, fmt.Errorf("legacyretirement: PolicyConfig.WindowID is empty")
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &policyImpl{cfg: cfg}, nil
}

// Handle implements Policy.
func (p *policyImpl) Handle(ctx context.Context, turn AssistantTurn) (RetirementDecision, error) {
	now := p.cfg.Clock()
	token := ClassifyToken(turn.RawText)
	if token == "" {
		return RetirementDecision{
			Matched:        false,
			EffectiveState: WindowOpen, // best-effort; not observed
			ServeNL:        true,
			Outcome:        OutcomeNotRetiredPassthrough,
			DecidedAt:      now,
		}, nil
	}
	cmd, ok := p.cfg.Catalog.Lookup(token)
	if !ok {
		return RetirementDecision{
			Matched:   false,
			ServeNL:   true,
			Outcome:   OutcomeNotRetiredPassthrough,
			DecidedAt: now,
		}, nil
	}

	state, reason, err := p.cfg.StateResolver.Resolve(ctx)
	if err != nil {
		return RetirementDecision{}, fmt.Errorf("legacyretirement: resolve window state: %w", err)
	}

	decision := RetirementDecision{
		Matched:        true,
		Command:        cmd,
		EffectiveState: state,
		StateReason:    reason,
		DecidedAt:      now,
		WindowID:       p.cfg.WindowID,
	}

	switch state {
	case WindowClosed:
		decision.ServeNL = false
		decision.ShowNotice = false
		decision.Outcome = OutcomeClosedUnknown
		p.recordResidual(turn.UserID, cmd.Command, decision.Outcome)
		return decision, nil

	case WindowPaused:
		decision.ServeNL = true
		decision.ShowNotice = false
		decision.Outcome = OutcomePausedSuppressed
		p.recordResidual(turn.UserID, cmd.Command, decision.Outcome)
		return decision, nil

	case WindowOpen:
		alreadyNotified, err := p.cfg.Ledger.HasNotified(ctx, turn.UserID, cmd.Command, p.cfg.WindowID)
		if err != nil {
			return RetirementDecision{}, fmt.Errorf("legacyretirement: ledger HasNotified for command %q: %w", cmd.Command, err)
		}
		decision.ServeNL = true
		if alreadyNotified {
			decision.ShowNotice = false
			decision.Outcome = OutcomeServedNoNotice
		} else {
			if err := p.cfg.Ledger.MarkShown(ctx, turn.UserID, cmd.Command, p.cfg.WindowID, now); err != nil {
				return RetirementDecision{}, fmt.Errorf("legacyretirement: ledger MarkShown for command %q: %w", cmd.Command, err)
			}
			decision.ShowNotice = true
			decision.Outcome = OutcomeNoticeAndServed
		}
		p.recordResidual(turn.UserID, cmd.Command, decision.Outcome)
		return decision, nil

	default:
		return RetirementDecision{}, fmt.Errorf("legacyretirement: resolver returned unknown effective state %q", state)
	}
}

func (p *policyImpl) recordResidual(userID, command string, outcome RetirementOutcome) {
	if p.cfg.Telemetry == nil {
		return
	}
	bucket := p.cfg.BucketHasher.UserBucket(userID)
	p.cfg.Telemetry.Record(command, bucket, outcome)
}

// NoticePayload is the structured response-metadata shape the
// assistant facade renders alongside the primary NL response when
// RetirementDecision.ShowNotice is true. Transport adapters render
// it as a one-line addendum; they MUST NOT block on it.
type NoticePayload struct {
	Command            string `json:"command"`
	NoticeCopy         string `json:"notice_copy"`
	ReplacementExample string `json:"replacement_example"`
	WindowID           string `json:"window_id"`
	Spec066ID          string `json:"spec066_id"`
}

// NoticeFor returns the NoticePayload for the decision, or zero
// value if no notice should be shown.
func (p *policyImpl) NoticeFor(decision RetirementDecision) NoticePayload {
	if !decision.ShowNotice {
		return NoticePayload{}
	}
	return NoticePayload{
		Command:            decision.Command.Command,
		NoticeCopy:         decision.Command.NoticeCopy,
		ReplacementExample: decision.Command.ReplacementExample,
		WindowID:           p.cfg.WindowID,
		Spec066ID:          decision.Command.Spec066ID,
	}
}
