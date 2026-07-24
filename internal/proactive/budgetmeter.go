package proactive

import "fmt"

// BudgetMeter is the honest "N of M used today" render of the single
// cross-channel daily nudge budget. Exhaustion is an explicit content state,
// never a hidden default: when Exhausted is true a surface renders the
// budget-exhausted honest state rather than a fabricated card (SCN-107-008).
type BudgetMeter struct {
	Used      int
	Total     int
	Exhausted bool
	Display   string
}

// ReadBudgetMeter projects the controller's live remaining budget and the SST
// daily_nudge_budget into an honest meter. It adds no second budget and reads
// no owner internals: remaining is supplied by the caller from the controller's
// existing smackerel_surfacing_budget_remaining signal, and dailyBudget from
// the SST surfacing.daily_nudge_budget.
//
// remaining is clamped to [0, dailyBudget] so a transient over/under-count never
// renders a nonsensical meter; Exhausted is true iff no budget remains.
func ReadBudgetMeter(remaining, dailyBudget int) BudgetMeter {
	if dailyBudget < 0 {
		dailyBudget = 0
	}
	if remaining < 0 {
		remaining = 0
	}
	if remaining > dailyBudget {
		remaining = dailyBudget
	}
	used := dailyBudget - remaining
	return BudgetMeter{
		Used:      used,
		Total:     dailyBudget,
		Exhausted: remaining == 0,
		Display:   fmt.Sprintf("%d of %d used today", used, dailyBudget),
	}
}
