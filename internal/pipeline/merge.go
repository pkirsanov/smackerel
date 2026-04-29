package pipeline

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// Execer is the minimal subset of pgxpool.Pool's API required by the
// user-context merge helper. It exists so MergeUserContext can be exercised
// in unit tests without a live database connection.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// mergeUserContextSQL appends a new user context string to the JSONB array
// stored at artifacts.metadata->'user_contexts'. The array is created on
// first append; existing keys in metadata are preserved by jsonb_set.
const mergeUserContextSQL = `
UPDATE artifacts
SET metadata = jsonb_set(
        COALESCE(metadata, '{}'::jsonb),
        '{user_contexts}',
        COALESCE(metadata->'user_contexts', '[]'::jsonb) || jsonb_build_array($1::text),
        true
    ),
    updated_at = NOW()
WHERE id = $2`

// MergeUserContext appends newContext to the user_contexts JSONB array on the
// existing artifact identified by artifactID. It is invoked by Processor.Process
// when DedupCheck reports a duplicate and the inbound request carried a
// non-empty Context value, so re-shares of the same URL with new context still
// land their context on the canonical artifact.
//
// Returns nil (no SQL executed) when newContext or artifactID are empty.
// Wraps any underlying executor error so ops can observe merge failures via slog.
func MergeUserContext(ctx context.Context, exec Execer, artifactID, newContext string) error {
	if artifactID == "" || newContext == "" {
		return nil
	}
	if exec == nil {
		return fmt.Errorf("merge user context: nil executor")
	}
	if _, err := exec.Exec(ctx, mergeUserContextSQL, newContext, artifactID); err != nil {
		return fmt.Errorf("merge user context for artifact %s: %w", artifactID, err)
	}
	return nil
}
