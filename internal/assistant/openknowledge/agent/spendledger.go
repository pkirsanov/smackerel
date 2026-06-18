// Spec 096 SCOPE-05 — the month-to-date USD spend accounting port that makes
// the per-user + global USD budgets load-bearing for paid providers
// (design §5.3, §12). The agent depends only on this interface; the
// DB-backed implementation lives in internal/assistant/openknowledge/
// usageledger and derives the claim-bound actor from context (the agent
// package never imports auth). Unit tests inject a fake that returns canned
// month-to-date spend and records appended usage.
package agent

import "context"

// UsageRecord is one append to the model_usage_ledger: the actual spend of a
// successful billable turn. usd_cost is 0 for ollama (and such a turn is
// never appended — the local path stays byte-for-byte). The DB-backed
// implementation supplies the claim-bound actor_user_id, the connection_id,
// and the app-written created_at month-window key; the agent only knows the
// effective model, the token count, and the realized USD cost.
type UsageRecord struct {
	// Model is the effective provider-qualified id (<kind>/<backend-id>)
	// that produced the turn's answer.
	Model string
	// Tokens is the combined token count charged across the turn.
	Tokens int
	// USDCost is the realized USD spend for the turn (> 0 for a billable
	// turn; ollama turns are $0 and are not appended).
	USDCost float64
}

// SpendLedger is the append-only month-to-date USD spend port. The budget
// pre-flight reads MonthToDateSpend before any billable dispatch; the
// post-dispatch chokepoint appends the realized cost via AppendUsage. The
// claim-bound actor is carried implicitly by ctx (the DB-backed impl reads
// auth.UserIDFromContext) so this port never accepts a request-body user id.
type SpendLedger interface {
	// MonthToDateSpend returns the current-month USD spend for the
	// claim-bound caller (perUserUSD) and across all callers (globalUSD).
	// A zero-spend month returns (0, 0, nil).
	MonthToDateSpend(ctx context.Context) (perUserUSD, globalUSD float64, err error)
	// AppendUsage records the realized USD cost of a successful billable
	// dispatch. Append-only — never an update or delete (audit-clean).
	AppendUsage(ctx context.Context, usage UsageRecord) error
}
