//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func cleanupPhotosByConnector(t *testing.T, pool *pgxpool.Pool, connectorID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, `DELETE FROM artifacts WHERE id IN (SELECT artifact_id FROM photos WHERE connector_id=$1)`, connectorID)
		_, _ = pool.Exec(ctx, `DELETE FROM photo_sync_state WHERE connector_id=$1`, connectorID)
		_, _ = pool.Exec(ctx, `DELETE FROM photo_capabilities WHERE connector_id=$1`, connectorID)
	})
}
