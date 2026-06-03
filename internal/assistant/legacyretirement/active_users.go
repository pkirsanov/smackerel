// active_users.go — spec 076 SCOPE-6a SQL-backed ActiveUsersProvider.
//
// The threshold evaluator needs an active-user denominator over the
// configured lookback. We derive it from the same privacy-preserving
// user_bucket column that powers the residual report: count the
// distinct HMAC-hashed buckets observed in
// assistant_legacy_retirement_residual within [now-lookback, now].
// This keeps the denominator scoped to users that have interacted
// with the legacy surface inside the lookback (the population the
// percent-of-active-users threshold is defined against) and avoids
// reaching into the assistant data plane.
package legacyretirement

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SQLActiveUsersProvider implements ActiveUsersProvider against the
// assistant_legacy_retirement_residual table.
type SQLActiveUsersProvider struct {
	pool     *pgxpool.Pool
	windowID string
}

// NewSQLActiveUsersProvider validates inputs and returns the provider.
func NewSQLActiveUsersProvider(pool *pgxpool.Pool, windowID string) (*SQLActiveUsersProvider, error) {
	if pool == nil {
		return nil, fmt.Errorf("legacyretirement: NewSQLActiveUsersProvider: pool is nil")
	}
	if windowID == "" {
		return nil, fmt.Errorf("legacyretirement: NewSQLActiveUsersProvider: windowID is empty")
	}
	return &SQLActiveUsersProvider{pool: pool, windowID: windowID}, nil
}

// ActiveUsers implements ActiveUsersProvider.
func (p *SQLActiveUsersProvider) ActiveUsers(ctx context.Context, now time.Time, lookbackDays int) (int64, error) {
	if lookbackDays < 1 {
		return 0, fmt.Errorf("legacyretirement: ActiveUsers: lookbackDays=%d must be >= 1", lookbackDays)
	}
	nowUTC := now.UTC()
	end := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), 0, 0, 0, 0, time.UTC)
	start := end.AddDate(0, 0, -(lookbackDays - 1))
	const q = `
		SELECT COUNT(DISTINCT user_bucket)::bigint
		  FROM assistant_legacy_retirement_residual
		 WHERE window_id = $1
		   AND day BETWEEN $2 AND $3`
	var n int64
	if err := p.pool.QueryRow(ctx, q, p.windowID, start, end).Scan(&n); err != nil {
		return 0, fmt.Errorf("legacyretirement: ActiveUsers query: %w", err)
	}
	return n, nil
}
