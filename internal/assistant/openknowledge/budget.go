package openknowledge

import (
	"errors"
	"fmt"
)

// Sentinel cap errors returned by BudgetTracker. The agent loop maps
// these to typed TerminationReason values.
var (
	ErrCapTokens          = errors.New("openknowledge: per-query token budget exceeded")
	ErrCapUSDPerQuery     = errors.New("openknowledge: per-query USD budget exceeded")
	ErrCapUSDMonthly      = errors.New("openknowledge: monthly USD budget exceeded")
	ErrCapUSDPerUserMonth = errors.New("openknowledge: per-user monthly USD budget exceeded")
	ErrBudgetInvalid      = errors.New("openknowledge: invalid budget tracker construction")
)

// BudgetTracker enforces the four caps that bound a single agent turn
// (G082 convergence, G083 compaction). Every cap is supplied by the
// caller from SST; there are no in-code defaults (G028).
//
// The tracker is single-turn: callers construct one tracker per
// user-prompt invocation. The "remaining" monthly inputs reflect the
// budget the operator has left BEFORE this turn starts; the tracker
// subtracts intra-turn spend from them so a single oversized turn
// cannot punch through a monthly budget.
type BudgetTracker struct {
	perQueryTokens      int
	perQueryUSD         float64
	monthlyUSDRemaining float64
	perUserUSDRemaining float64

	tokensUsed int
	usdSpent   float64
}

// NewBudgetTracker validates inputs and returns a ready tracker.
// perQueryTokens MUST be > 0; perQueryUSD, monthlyUSDRemaining,
// perUserUSDRemaining MUST be >= 0. All caps are absolute, not deltas.
func NewBudgetTracker(perQueryTokens int, perQueryUSD, monthlyUSDRemaining, perUserUSDRemaining float64) (*BudgetTracker, error) {
	if perQueryTokens <= 0 {
		return nil, fmt.Errorf("%w: perQueryTokens must be > 0 (got %d)", ErrBudgetInvalid, perQueryTokens)
	}
	if perQueryUSD < 0 {
		return nil, fmt.Errorf("%w: perQueryUSD must be >= 0 (got %v)", ErrBudgetInvalid, perQueryUSD)
	}
	if monthlyUSDRemaining < 0 {
		return nil, fmt.Errorf("%w: monthlyUSDRemaining must be >= 0 (got %v)", ErrBudgetInvalid, monthlyUSDRemaining)
	}
	if perUserUSDRemaining < 0 {
		return nil, fmt.Errorf("%w: perUserUSDRemaining must be >= 0 (got %v)", ErrBudgetInvalid, perUserUSDRemaining)
	}
	return &BudgetTracker{
		perQueryTokens:      perQueryTokens,
		perQueryUSD:         perQueryUSD,
		monthlyUSDRemaining: monthlyUSDRemaining,
		perUserUSDRemaining: perUserUSDRemaining,
	}, nil
}

// RecordLLMCall charges an LLM round-trip. promptTokens and
// completionTokens are summed into tokensUsed; costUSD is added to
// usdSpent. The first cap to be breached wins and is returned as a
// typed sentinel; the spend is still recorded so the caller can report
// the over-the-line totals in TurnResult.
func (b *BudgetTracker) RecordLLMCall(promptTokens, completionTokens int, costUSD float64) error {
	if promptTokens < 0 || completionTokens < 0 || costUSD < 0 {
		return fmt.Errorf("%w: negative spend (prompt=%d, completion=%d, usd=%v)", ErrBudgetInvalid, promptTokens, completionTokens, costUSD)
	}
	b.tokensUsed += promptTokens + completionTokens
	b.usdSpent += costUSD
	return b.checkCaps()
}

// RecordToolCall charges a tool invocation. Most tools are free
// (calculator, unit_convert, internal_retrieval, self-hosted SearxNG);
// commercial providers may carry a per-call cost in a future scope.
func (b *BudgetTracker) RecordToolCall(_ string, costUSD float64) error {
	if costUSD < 0 {
		return fmt.Errorf("%w: negative tool cost (usd=%v)", ErrBudgetInvalid, costUSD)
	}
	b.usdSpent += costUSD
	return b.checkCaps()
}

func (b *BudgetTracker) checkCaps() error {
	if b.tokensUsed > b.perQueryTokens {
		return ErrCapTokens
	}
	if b.usdSpent > b.perQueryUSD {
		return ErrCapUSDPerQuery
	}
	if b.usdSpent > b.monthlyUSDRemaining {
		return ErrCapUSDMonthly
	}
	if b.usdSpent > b.perUserUSDRemaining {
		return ErrCapUSDPerUserMonth
	}
	return nil
}

// TokensUsed reports prompt+completion tokens accumulated so far.
func (b *BudgetTracker) TokensUsed() int { return b.tokensUsed }

// USDSpent reports the running USD spend (LLM + tool).
func (b *BudgetTracker) USDSpent() float64 { return b.usdSpent }

// RemainingTokens reports headroom against the per-query token cap.
// Returns 0 (never negative) once the cap is reached.
func (b *BudgetTracker) RemainingTokens() int {
	r := b.perQueryTokens - b.tokensUsed
	if r < 0 {
		return 0
	}
	return r
}

// RemainingUSDPerQuery reports headroom against the per-query USD cap.
func (b *BudgetTracker) RemainingUSDPerQuery() float64 {
	r := b.perQueryUSD - b.usdSpent
	if r < 0 {
		return 0
	}
	return r
}

// RemainingUSDMonthly reports headroom against the monthly USD cap.
func (b *BudgetTracker) RemainingUSDMonthly() float64 {
	r := b.monthlyUSDRemaining - b.usdSpent
	if r < 0 {
		return 0
	}
	return r
}

// RemainingUSDPerUserMonth reports headroom against the per-user
// monthly USD cap.
func (b *BudgetTracker) RemainingUSDPerUserMonth() float64 {
	r := b.perUserUSDRemaining - b.usdSpent
	if r < 0 {
		return 0
	}
	return r
}

// PerQueryTokenBudget exposes the configured cap so callers can compute
// the compaction-threshold ratio without re-plumbing config.
func (b *BudgetTracker) PerQueryTokenBudget() int { return b.perQueryTokens }
