// Package usageledger is the Postgres-backed implementation of the spec 096
// SCOPE-05 month-to-date USD spend port (agent.SpendLedger). It makes the
// open-knowledge agent's per-user + global USD budgets load-bearing for paid
// providers by recording every billable turn's realized cost into the
// append-only model_usage_ledger (migration 062) and summing the current
// month's spend for the budget pre-flight.
//
// Claim-binding (spec 044): the actor_user_id ALWAYS comes from the
// authenticated session in context (auth.UserIDFromContext) — NEVER a
// request-body user id — mirroring the modelpref store. An empty actor is a
// fail-loud error (the per-user spend dimension requires a claim-bound
// subject); there is no silent fallback (G028).
//
// Operator-global: the table carries a per-user SPEND dimension (allowed —
// this is per-user budget accounting, not a per-user credential). The
// connection_id column stores the provider KIND derived from the effective
// provider-qualified model (the operator-global connection grouping); it is
// provenance only — the budget math sums usd_cost, never grouping by it.
package usageledger

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agent"
	"github.com/smackerel/smackerel/internal/auth"
)

// PostgresLedger implements agent.SpendLedger against model_usage_ledger
// (migration 062).
type PostgresLedger struct {
	Pool *pgxpool.Pool
	now  func() time.Time
}

// New constructs a PostgresLedger using the wall clock.
func New(pool *pgxpool.Pool) *PostgresLedger {
	return &PostgresLedger{Pool: pool, now: time.Now}
}

// WithNow overrides the clock (tests assert the month-window boundary).
// Returns the receiver for chaining.
func (l *PostgresLedger) WithNow(now func() time.Time) *PostgresLedger {
	l.now = now
	return l
}

// monthStart returns the first instant of the current UTC month — the
// app-computed month-window key the SUM filters on (no DB-side now(); G028).
func (l *PostgresLedger) monthStart() time.Time {
	t := l.now().UTC()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

const perUserSpendSQL = `
SELECT COALESCE(SUM(usd_cost), 0)::double precision
FROM model_usage_ledger
WHERE actor_user_id = $1 AND created_at >= $2
`

const globalSpendSQL = `
SELECT COALESCE(SUM(usd_cost), 0)::double precision
FROM model_usage_ledger
WHERE created_at >= $1
`

// MonthToDateSpend implements agent.SpendLedger. It returns the claim-bound
// caller's current-month spend (perUserUSD) and the all-callers current-month
// spend (globalUSD). The actor is the authenticated session subject; an empty
// actor fails loud (no silent default).
func (l *PostgresLedger) MonthToDateSpend(ctx context.Context) (float64, float64, error) {
	if l == nil || l.Pool == nil {
		return 0, 0, errors.New("usageledger: PostgresLedger requires a non-nil Pool")
	}
	actor := auth.UserIDFromContext(ctx)
	if actor == "" {
		return 0, 0, errors.New("usageledger: MonthToDateSpend requires a claim-bound actor_user_id in context (no silent default)")
	}
	since := l.monthStart()

	var perUser float64
	if err := l.Pool.QueryRow(ctx, perUserSpendSQL, actor, since).Scan(&perUser); err != nil {
		return 0, 0, fmt.Errorf("usageledger: sum per-user month-to-date spend: %w", err)
	}
	var global float64
	if err := l.Pool.QueryRow(ctx, globalSpendSQL, since).Scan(&global); err != nil {
		return 0, 0, fmt.Errorf("usageledger: sum global month-to-date spend: %w", err)
	}
	return perUser, global, nil
}

const appendSQL = `
INSERT INTO model_usage_ledger (actor_user_id, connection_id, model, tokens, usd_cost, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
`

// AppendUsage implements agent.SpendLedger. It appends one row recording the
// realized spend of a successful billable turn. Append-only — no update, no
// delete. The actor is the authenticated session subject (fail-loud when
// absent); connection_id is the provider kind derived from the effective
// provider-qualified model; created_at is app-written (no DB-side default).
func (l *PostgresLedger) AppendUsage(ctx context.Context, usage agent.UsageRecord) error {
	if l == nil || l.Pool == nil {
		return errors.New("usageledger: PostgresLedger requires a non-nil Pool")
	}
	actor := auth.UserIDFromContext(ctx)
	if actor == "" {
		return errors.New("usageledger: AppendUsage requires a claim-bound actor_user_id in context (no silent default)")
	}
	if strings.TrimSpace(usage.Model) == "" {
		return errors.New("usageledger: AppendUsage requires a non-empty effective model")
	}
	connectionID := providerKind(usage.Model)
	if _, err := l.Pool.Exec(ctx, appendSQL,
		actor, connectionID, usage.Model, usage.Tokens, usage.USDCost, l.now().UTC()); err != nil {
		return fmt.Errorf("usageledger: append usage row: %w", err)
	}
	return nil
}

// providerKind returns the provider kind (the substring before the FIRST
// "/") of a provider-qualified model id — the operator-global connection
// grouping stored as connection_id. A bare id (no "/") is a 089-era Ollama
// selection; its kind is "ollama".
func providerKind(model string) string {
	model = strings.TrimSpace(model)
	kind, _, found := strings.Cut(model, "/")
	if !found {
		return "ollama"
	}
	return kind
}

// Static assertion that PostgresLedger satisfies the agent port.
var _ agent.SpendLedger = (*PostgresLedger)(nil)
