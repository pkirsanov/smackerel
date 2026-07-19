//go:build e2e

package assistant_e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

type assistantConversationSnapshot struct {
	exists                  bool
	userID                  string
	transport               string
	workingContext          string
	pendingConfirm          pgtype.Text
	pendingDisambig         pgtype.Text
	pendingClarify          pgtype.Text
	lastActivityAt          time.Time
	schemaVersion           int
	legacyRetirementNotices string
}

type assistantConversationIsolation struct {
	pool     *pgxpool.Pool
	snapshot assistantConversationSnapshot
}

func openAssistantConversationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("e2e: DATABASE_URL not set - live stack DB not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open assistant conversation pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func loadAssistantConversationSnapshot(ctx context.Context, pool *pgxpool.Pool, userID, transport string) (assistantConversationSnapshot, error) {
	snapshot := assistantConversationSnapshot{userID: userID, transport: transport}
	err := pool.QueryRow(ctx, `
SELECT working_context::text,
       pending_confirm::text,
       pending_disambig::text,
       pending_clarify::text,
       last_activity_at,
       schema_version,
       legacy_retirement_notices::text
  FROM assistant_conversations
 WHERE user_id = $1 AND transport = $2
`, userID, transport).Scan(
		&snapshot.workingContext,
		&snapshot.pendingConfirm,
		&snapshot.pendingDisambig,
		&snapshot.pendingClarify,
		&snapshot.lastActivityAt,
		&snapshot.schemaVersion,
		&snapshot.legacyRetirementNotices,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return snapshot, nil
	}
	if err != nil {
		return assistantConversationSnapshot{}, err
	}
	snapshot.exists = true
	return snapshot, nil
}

func isolateAssistantConversation(t *testing.T, pool *pgxpool.Pool, userID, transport string) *assistantConversationIsolation {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	snapshot, err := loadAssistantConversationSnapshot(ctx, pool, userID, transport)
	if err != nil {
		t.Fatalf("snapshot assistant conversation (%q,%q): %v", userID, transport, err)
	}
	isolation := &assistantConversationIsolation{pool: pool, snapshot: snapshot}
	isolation.reset(t)
	t.Cleanup(func() {
		if restoreErr := isolation.restore(); restoreErr != nil {
			t.Errorf("restore assistant conversation (%q,%q): %v", userID, transport, restoreErr)
		}
	})
	return isolation
}

func isolateSharedHTTPConversation(t *testing.T) *assistantConversationIsolation {
	t.Helper()
	userID := os.Getenv("ASSISTANT_TRANSPORTS_HTTP_SHARED_USER_ID")
	if userID == "" {
		t.Fatal("ASSISTANT_TRANSPORTS_HTTP_SHARED_USER_ID not set in live E2E environment")
	}
	return isolateAssistantConversation(t, openAssistantConversationPool(t), userID, httpadapter.TransportName)
}

func (i *assistantConversationIsolation) reset(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := i.pool.Exec(ctx,
		`DELETE FROM assistant_conversations WHERE user_id = $1 AND transport = $2`,
		i.snapshot.userID, i.snapshot.transport,
	); err != nil {
		t.Fatalf("reset isolated assistant conversation (%q,%q): %v", i.snapshot.userID, i.snapshot.transport, err)
	}
}

func (i *assistantConversationIsolation) restore() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if !i.snapshot.exists {
		_, err := i.pool.Exec(ctx,
			`DELETE FROM assistant_conversations WHERE user_id = $1 AND transport = $2`,
			i.snapshot.userID, i.snapshot.transport,
		)
		return err
	}
	_, err := i.pool.Exec(ctx, `
INSERT INTO assistant_conversations
    (user_id, transport, working_context, pending_confirm, pending_disambig,
     pending_clarify, last_activity_at, schema_version, legacy_retirement_notices)
VALUES
    ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb, $6::jsonb, $7, $8, $9::jsonb)
ON CONFLICT (user_id, transport) DO UPDATE
    SET working_context = EXCLUDED.working_context,
        pending_confirm = EXCLUDED.pending_confirm,
        pending_disambig = EXCLUDED.pending_disambig,
        pending_clarify = EXCLUDED.pending_clarify,
        last_activity_at = EXCLUDED.last_activity_at,
        schema_version = EXCLUDED.schema_version,
        legacy_retirement_notices = EXCLUDED.legacy_retirement_notices
`,
		i.snapshot.userID,
		i.snapshot.transport,
		i.snapshot.workingContext,
		nullableJSON(i.snapshot.pendingConfirm),
		nullableJSON(i.snapshot.pendingDisambig),
		nullableJSON(i.snapshot.pendingClarify),
		i.snapshot.lastActivityAt,
		i.snapshot.schemaVersion,
		i.snapshot.legacyRetirementNotices,
	)
	return err
}

func nullableJSON(value pgtype.Text) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func seedAssistantConversation(t *testing.T, pool *pgxpool.Pool, userID, marker string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := pool.Exec(ctx, `
INSERT INTO assistant_conversations
    (user_id, transport, working_context, pending_confirm, pending_disambig,
     pending_clarify, last_activity_at, schema_version, legacy_retirement_notices)
VALUES
    ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb, $6::jsonb, $7, 1, $8::jsonb)
`,
		userID,
		httpadapter.TransportName,
		fmt.Sprintf(`{"turns":[{"user_text":%q,"body":%q,"source_ids":[],"emitted_at":"2026-07-19T12:00:00Z"}]}`, marker, marker),
		fmt.Sprintf(`{"confirm_ref":%q,"scenario_id":"weather_query","proposed_action":"none","payload":"","expires_at":"2026-07-19T13:00:00Z"}`, marker),
		fmt.Sprintf(`{"disambiguation_ref":%q,"choices":[],"expires_at":"2026-07-19T13:00:00Z"}`, marker),
		fmt.Sprintf(`{"schema_version":"v1","original_prompt":%q,"emit_time":"2026-07-19T12:00:00Z","clarify_intent_id":%q,"original_turn_id":%q,"user_id":%q}`, marker, marker, marker, userID),
		time.Date(2026, 7, 19, 12, 0, 0, 123456000, time.UTC),
		fmt.Sprintf(`{"schema_version":1,"window_id":%q,"commands":{}}`, marker),
	)
	if err != nil {
		t.Fatalf("seed assistant conversation %q: %v", userID, err)
	}
}

func TestAssistantConversationIsolation_RestoresExactTargetAndPreservesNeighbor_Adversarial(t *testing.T) {
	pool := openAssistantConversationPool(t)
	stamp := time.Now().UTC().UnixNano()
	targetUser := fmt.Sprintf("bug-073-004-target-%d", stamp)
	neighborUser := fmt.Sprintf("bug-073-004-neighbor-%d", stamp)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx,
			`DELETE FROM assistant_conversations WHERE user_id IN ($1, $2) AND transport = $3`,
			targetUser, neighborUser, httpadapter.TransportName,
		)
	})
	seedAssistantConversation(t, pool, targetUser, "target-before")
	seedAssistantConversation(t, pool, neighborUser, "neighbor-before")
	wantTarget, err := loadAssistantConversationSnapshot(ctx, pool, targetUser, httpadapter.TransportName)
	if err != nil {
		t.Fatalf("load target before isolation: %v", err)
	}
	wantNeighbor, err := loadAssistantConversationSnapshot(ctx, pool, neighborUser, httpadapter.TransportName)
	if err != nil {
		t.Fatalf("load neighbor before isolation: %v", err)
	}

	isolation := isolateAssistantConversation(t, pool, targetUser, httpadapter.TransportName)
	if targetDuring, loadErr := loadAssistantConversationSnapshot(ctx, pool, targetUser, httpadapter.TransportName); loadErr != nil || targetDuring.exists {
		t.Fatalf("target row not isolated: exists=%v err=%v", targetDuring.exists, loadErr)
	}
	seedAssistantConversation(t, pool, targetUser, "target-during")
	if err := isolation.restore(); err != nil {
		t.Fatalf("manual restore: %v", err)
	}
	gotTarget, err := loadAssistantConversationSnapshot(ctx, pool, targetUser, httpadapter.TransportName)
	if err != nil {
		t.Fatalf("load target after restore: %v", err)
	}
	gotNeighbor, err := loadAssistantConversationSnapshot(ctx, pool, neighborUser, httpadapter.TransportName)
	if err != nil {
		t.Fatalf("load neighbor after restore: %v", err)
	}
	if !reflect.DeepEqual(gotTarget, wantTarget) {
		t.Fatalf("target row was not restored exactly:\nwant=%+v\n got=%+v", wantTarget, gotTarget)
	}
	if !reflect.DeepEqual(gotNeighbor, wantNeighbor) {
		t.Fatalf("unrelated row changed:\nwant=%+v\n got=%+v", wantNeighbor, gotNeighbor)
	}
}
