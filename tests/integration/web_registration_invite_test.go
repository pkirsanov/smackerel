//go:build integration

// Spec 093 SCOPE-01 — live-DB integration tests for the webinvite repo, run by
// the curated go-integration lane (./smackerel.sh test integration). These live
// in tests/integration/ (the repo's canonical live-DB location, alongside
// auth_bootstrap_test.go / auth_rotation_test.go / auth_chaos_test.go) so the
// integration runner actually executes them — the go-integration allowlist is
// ./tests/integration/... ./internal/notification/... ./internal/assistant/...
// ./internal/cardrewards/..., which does NOT include internal/auth/.
//
// They exercise the REAL atomic / single-use / TOCTOU behaviour against the
// live ephemeral Postgres — none of it can be faked. The account-insert
// callback uses the REAL webcreds.HashAndInsertTx, proving ConsumeAndCreate
// end-to-end (invite claim + account create on one tx). Rows are namespaced
// (created_by / username prefixed "wi-int-") so runs stay collision-free and
// cleanup is scoped.

package integration

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/auth/webcreds"
	"github.com/smackerel/smackerel/internal/auth/webinvite"
	"github.com/smackerel/smackerel/internal/db"
)

// inviteRepo builds a webinvite.PostgresRepo on the live test pool, ensures
// migration 058 is applied, and registers namespaced cleanup.
func inviteRepo(t *testing.T) (*webinvite.PostgresRepo, *pgxpool.Pool) {
	t.Helper()
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate (058): %v", err)
	}
	repo, err := webinvite.NewPostgresRepo(pool)
	if err != nil {
		t.Fatalf("NewPostgresRepo: %v", err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = pool.Exec(c, `DELETE FROM web_registration_invites WHERE created_by LIKE 'wi-int-%'`)
		_, _ = pool.Exec(c, `DELETE FROM web_user_credentials WHERE username LIKE 'wi-int-%'`)
	})
	return repo, pool
}

func inviteTag(t *testing.T, tag string) string {
	t.Helper()
	stamp := strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "-")
	return "wi-int-" + tag + "-" + stamp
}

// insertAccountTx is the real onClaimed callback: it creates the
// web_user_credentials row on the invite-consume tx via the spec-093
// HashAndInsertTx free function.
func insertAccountTx(username, password string) func(context.Context, pgx.Tx) error {
	return func(ctx context.Context, tx pgx.Tx) error {
		return webcreds.HashAndInsertTx(ctx, tx, username, password)
	}
}

// seedExpiredInvite directly inserts an invite whose expires_at is in the past
// (Generate only ever mints future/never expiries), returning its hash.
func seedExpiredInvite(t *testing.T, pool *pgxpool.Pool, createdBy string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hash := webinvite.HashToken("inv_int_expired_" + createdBy)
	if _, err := pool.Exec(ctx,
		`INSERT INTO web_registration_invites (token_hash, created_by, expires_at)
		 VALUES ($1, $2, now() - interval '1 hour')`,
		hash, createdBy,
	); err != nil {
		t.Fatalf("seed expired invite: %v", err)
	}
	return hash
}

// TestWebInvite_Generate_StoresHashOnly stores ONLY the hash + metadata and
// returns the plaintext exactly once; the row never holds the plaintext
// (SCN-093-01 DB half).
func TestWebInvite_Generate_StoresHashOnly(t *testing.T) {
	repo, pool := inviteRepo(t)
	ctx := context.Background()
	createdBy := inviteTag(t, "gen")

	plaintext, err := repo.Generate(ctx, createdBy, "for the new analyst", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(plaintext, "inv_") {
		t.Fatalf("returned plaintext %q lacks inv_ prefix", plaintext)
	}

	var (
		gotHash, gotCreatedBy string
		gotLabel              *string
		expiresAt             *time.Time
	)
	if err := pool.QueryRow(ctx,
		`SELECT token_hash, label, created_by, expires_at
		   FROM web_registration_invites WHERE created_by = $1`,
		createdBy,
	).Scan(&gotHash, &gotLabel, &gotCreatedBy, &expiresAt); err != nil {
		t.Fatalf("select stored invite: %v", err)
	}
	if gotHash != webinvite.HashToken(plaintext) {
		t.Errorf("stored token_hash %q != HashToken(plaintext) %q", gotHash, webinvite.HashToken(plaintext))
	}
	if gotHash == plaintext {
		t.Error("stored token_hash equals the plaintext (must be hashed at rest)")
	}
	if gotLabel == nil || *gotLabel != "for the new analyst" {
		t.Errorf("label not stored: %v", gotLabel)
	}
	if expiresAt == nil || !expiresAt.After(time.Now()) {
		t.Errorf("expires_at not in the future: %v", expiresAt)
	}
	// Defensive: no text column may hold the plaintext.
	var leak int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM web_registration_invites
		  WHERE created_by = $1
		    AND (token_hash = $2 OR label = $2 OR created_by = $2 OR used_by = $2)`,
		createdBy, plaintext,
	).Scan(&leak); err != nil {
		t.Fatalf("leak scan: %v", err)
	}
	if leak != 0 {
		t.Errorf("plaintext token leaked into a stored column (%d matches)", leak)
	}

	// ttl<=0 ⇒ NULL expiry (never expires).
	neverBy := inviteTag(t, "never")
	if _, err := repo.Generate(ctx, neverBy, "", 0); err != nil {
		t.Fatalf("Generate never-expire: %v", err)
	}
	var neverExpiry *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT expires_at FROM web_registration_invites WHERE created_by = $1`, neverBy,
	).Scan(&neverExpiry); err != nil {
		t.Fatalf("select never invite: %v", err)
	}
	if neverExpiry != nil {
		t.Errorf("ttl<=0 should store NULL expires_at, got %v", neverExpiry)
	}
}

// TestWebInvite_ConsumeAndCreate_SingleUse proves the atomic claim+create and
// that a second consume of the same hash is rejected (SCN-093-02 / AC-5).
func TestWebInvite_ConsumeAndCreate_SingleUse(t *testing.T) {
	repo, pool := inviteRepo(t)
	ctx := context.Background()
	createdBy := inviteTag(t, "su")
	user1 := inviteTag(t, "su-u1")
	user2 := inviteTag(t, "su-u2")

	plaintext, err := repo.Generate(ctx, createdBy, "", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	hash := webinvite.HashToken(plaintext)

	outcome, err := repo.ConsumeAndCreate(ctx, hash, user1, insertAccountTx(user1, "correct-horse-battery-staple"))
	if err != nil {
		t.Fatalf("first consume err: %v", err)
	}
	if outcome != webinvite.ConsumeCreated {
		t.Fatalf("first consume outcome = %d, want ConsumeCreated", outcome)
	}
	var accounts int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM web_user_credentials WHERE username = $1`, user1).Scan(&accounts); err != nil {
		t.Fatalf("count account: %v", err)
	}
	if accounts != 1 {
		t.Fatalf("expected exactly 1 account for %q, got %d", user1, accounts)
	}
	var usedAt *time.Time
	var usedBy *string
	if err := pool.QueryRow(ctx,
		`SELECT used_at, used_by FROM web_registration_invites WHERE token_hash = $1`, hash,
	).Scan(&usedAt, &usedBy); err != nil {
		t.Fatalf("select used_*: %v", err)
	}
	if usedAt == nil || usedBy == nil || *usedBy != user1 {
		t.Fatalf("used_at/used_by not set correctly: at=%v by=%v", usedAt, usedBy)
	}

	// Second consume of the SAME hash with a DIFFERENT username ⇒ ConsumeInvalid.
	outcome2, err := repo.ConsumeAndCreate(ctx, hash, user2, insertAccountTx(user2, "another-long-enough-pw"))
	if err != nil {
		t.Fatalf("second consume err: %v", err)
	}
	if outcome2 != webinvite.ConsumeInvalid {
		t.Fatalf("second consume outcome = %d, want ConsumeInvalid (single-use)", outcome2)
	}
	var account2 int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM web_user_credentials WHERE username = $1`, user2).Scan(&account2); err != nil {
		t.Fatalf("count account2: %v", err)
	}
	if account2 != 0 {
		t.Fatalf("a second account was created for %q (double-spend)", user2)
	}
	var usedByAfter *string
	if err := pool.QueryRow(ctx,
		`SELECT used_by FROM web_registration_invites WHERE token_hash = $1`, hash,
	).Scan(&usedByAfter); err != nil {
		t.Fatalf("select used_by after: %v", err)
	}
	if usedByAfter == nil || *usedByAfter != user1 {
		t.Fatalf("used_by changed on the rejected reuse: %v", usedByAfter)
	}
}

// TestWebInvite_ConcurrentConsume races two goroutines on one invite: exactly
// one ConsumeCreated, one ConsumeInvalid, and exactly one account (SCN-093-03).
func TestWebInvite_ConcurrentConsume(t *testing.T) {
	repo, pool := inviteRepo(t)
	ctx := context.Background()
	createdBy := inviteTag(t, "race")
	userA := inviteTag(t, "race-a")
	userB := inviteTag(t, "race-b")

	plaintext, err := repo.Generate(ctx, createdBy, "", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	hash := webinvite.HashToken(plaintext)

	var wg sync.WaitGroup
	outcomes := make([]webinvite.ConsumeOutcome, 2)
	errs := make([]error, 2)
	users := [2]string{userA, userB}
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			outcomes[idx], errs[idx] = repo.ConsumeAndCreate(ctx, hash, users[idx],
				insertAccountTx(users[idx], "correct-horse-battery-staple"))
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d returned err: %v", i, err)
		}
	}
	created, invalid := 0, 0
	for _, o := range outcomes {
		switch o {
		case webinvite.ConsumeCreated:
			created++
		case webinvite.ConsumeInvalid:
			invalid++
		default:
			t.Fatalf("unexpected outcome %d", o)
		}
	}
	if created != 1 || invalid != 1 {
		t.Fatalf("race outcome = {created:%d invalid:%d}, want exactly one each", created, invalid)
	}
	var accounts int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM web_user_credentials WHERE username IN ($1, $2)`, userA, userB,
	).Scan(&accounts); err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if accounts != 1 {
		t.Fatalf("expected exactly 1 account after the race, got %d (double-spend)", accounts)
	}
}

// TestWebInvite_Expired proves an expired invite is not live and cannot be
// consumed (SCN-093-04 / UC-4).
func TestWebInvite_Expired(t *testing.T) {
	repo, pool := inviteRepo(t)
	ctx := context.Background()
	createdBy := inviteTag(t, "exp")
	user := inviteTag(t, "exp-u")

	hash := seedExpiredInvite(t, pool, createdBy)

	live, err := repo.IsLive(ctx, hash)
	if err != nil {
		t.Fatalf("IsLive: %v", err)
	}
	if live {
		t.Fatal("expired invite reported live")
	}

	outcome, err := repo.ConsumeAndCreate(ctx, hash, user, insertAccountTx(user, "correct-horse-battery-staple"))
	if err != nil {
		t.Fatalf("consume expired err: %v", err)
	}
	if outcome != webinvite.ConsumeInvalid {
		t.Fatalf("consume expired outcome = %d, want ConsumeInvalid", outcome)
	}
	var accounts int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM web_user_credentials WHERE username = $1`, user).Scan(&accounts); err != nil {
		t.Fatalf("count account: %v", err)
	}
	if accounts != 0 {
		t.Fatalf("expired invite created an account")
	}
}

// TestWebInvite_DuplicateUsernameRollsBack proves a duplicate username rolls the
// WHOLE tx back so the invite stays unconsumed (SCN-093-05).
func TestWebInvite_DuplicateUsernameRollsBack(t *testing.T) {
	repo, pool := inviteRepo(t)
	ctx := context.Background()
	createdBy := inviteTag(t, "dup")
	taken := inviteTag(t, "dup-taken")

	// Seed an existing account "taken".
	if _, err := pool.Exec(ctx,
		`INSERT INTO web_user_credentials (username, password_hash) VALUES ($1, $2)`,
		taken, "$argon2id$v=19$m=65536,t=1,p=4$c2FsdHNhbHRzYWx0c2E$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaA",
	); err != nil {
		t.Fatalf("seed taken account: %v", err)
	}

	plaintext, err := repo.Generate(ctx, createdBy, "", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	hash := webinvite.HashToken(plaintext)

	outcome, err := repo.ConsumeAndCreate(ctx, hash, taken, insertAccountTx(taken, "correct-horse-battery-staple"))
	if outcome != webinvite.ConsumeRolledBack {
		t.Fatalf("outcome = %d, want ConsumeRolledBack", outcome)
	}
	if err == nil || !strings.Contains(err.Error(), "user already exists") {
		t.Fatalf("expected webcreds.ErrUserExists, got %v", err)
	}
	// Invite NOT consumed — used_at stays NULL, so it is still live.
	var usedAt *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT used_at FROM web_registration_invites WHERE token_hash = $1`, hash).Scan(&usedAt); err != nil {
		t.Fatalf("select used_at: %v", err)
	}
	if usedAt != nil {
		t.Fatalf("invite was consumed despite the rolled-back account create: used_at=%v", usedAt)
	}
	live, err := repo.IsLive(ctx, hash)
	if err != nil {
		t.Fatalf("IsLive after rollback: %v", err)
	}
	if !live {
		t.Fatal("invite is not live after a rolled-back consume (should be retriable)")
	}
}

// TestWebInvite_List returns metadata only — never the hash, never plaintext —
// with correct derived statuses (SCN-093-06).
func TestWebInvite_List(t *testing.T) {
	repo, pool := inviteRepo(t)
	ctx := context.Background()
	base := inviteTag(t, "list")

	outstandingBy := base + "-outstanding"
	pOut, err := repo.Generate(ctx, outstandingBy, "outstanding one", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("gen outstanding: %v", err)
	}
	usedByCreator := base + "-used"
	pUsed, err := repo.Generate(ctx, usedByCreator, "", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("gen used: %v", err)
	}
	usedUser := base + "-user"
	if o, err := repo.ConsumeAndCreate(ctx, webinvite.HashToken(pUsed), usedUser,
		insertAccountTx(usedUser, "correct-horse-battery-staple")); err != nil || o != webinvite.ConsumeCreated {
		t.Fatalf("consume used: outcome=%d err=%v", o, err)
	}
	revokedBy := base + "-revoked"
	pRevoked, err := repo.Generate(ctx, revokedBy, "", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("gen revoked: %v", err)
	}
	var revokedID string
	if err := pool.QueryRow(ctx,
		`SELECT id FROM web_registration_invites WHERE token_hash = $1`, webinvite.HashToken(pRevoked)).Scan(&revokedID); err != nil {
		t.Fatalf("lookup revoked id: %v", err)
	}
	if o, err := repo.Revoke(ctx, revokedID); err != nil || o != webinvite.RevokeDone {
		t.Fatalf("revoke: outcome=%d err=%v", o, err)
	}
	expiredBy := base + "-expired"
	seedExpiredInvite(t, pool, expiredBy)

	rows, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	statusByCreatedBy := map[string]webinvite.InviteStatus{}
	for _, r := range rows {
		if !strings.HasPrefix(r.CreatedBy, base) {
			continue
		}
		statusByCreatedBy[r.CreatedBy] = r.Status
	}
	want := map[string]webinvite.InviteStatus{
		outstandingBy: webinvite.StatusOutstanding,
		usedByCreator: webinvite.StatusUsed,
		revokedBy:     webinvite.StatusRevoked,
		expiredBy:     webinvite.StatusExpired,
	}
	for cb, ws := range want {
		if statusByCreatedBy[cb] != ws {
			t.Errorf("created_by %q status = %q, want %q", cb, statusByCreatedBy[cb], ws)
		}
	}

	// No InviteRow field may carry the hash or any plaintext.
	forbidden := []string{
		pOut, pUsed, pRevoked,
		webinvite.HashToken(pOut), webinvite.HashToken(pUsed), webinvite.HashToken(pRevoked),
	}
	for _, r := range rows {
		fields := []string{r.ID, r.CreatedBy}
		if r.Label != nil {
			fields = append(fields, *r.Label)
		}
		if r.UsedBy != nil {
			fields = append(fields, *r.UsedBy)
		}
		for _, f := range fields {
			for _, bad := range forbidden {
				if f == bad {
					t.Errorf("List row exposed a token/hash value in a field: %q", f)
				}
			}
		}
	}
}

// TestWebInvite_Revoke transitions an outstanding invite and is a no-op
// otherwise (SCN-093-07).
func TestWebInvite_Revoke(t *testing.T) {
	repo, pool := inviteRepo(t)
	ctx := context.Background()
	createdBy := inviteTag(t, "rev")

	plaintext, err := repo.Generate(ctx, createdBy, "", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	hash := webinvite.HashToken(plaintext)
	var id string
	if err := pool.QueryRow(ctx,
		`SELECT id FROM web_registration_invites WHERE token_hash = $1`, hash).Scan(&id); err != nil {
		t.Fatalf("lookup id: %v", err)
	}

	if o, err := repo.Revoke(ctx, id); err != nil || o != webinvite.RevokeDone {
		t.Fatalf("first revoke: outcome=%d err=%v", o, err)
	}
	var revokedAt *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT revoked_at FROM web_registration_invites WHERE id = $1`, id).Scan(&revokedAt); err != nil {
		t.Fatalf("select revoked_at: %v", err)
	}
	if revokedAt == nil {
		t.Fatal("revoked_at not set after RevokeDone")
	}
	if live, err := repo.IsLive(ctx, hash); err != nil || live {
		t.Fatalf("revoked invite still live (live=%v err=%v)", live, err)
	}

	if o, err := repo.Revoke(ctx, id); err != nil || o != webinvite.RevokeNoop {
		t.Fatalf("repeat revoke: outcome=%d err=%v, want RevokeNoop", o, err)
	}
	if o, err := repo.Revoke(ctx, "nonexistent-id-"+createdBy); err != nil || o != webinvite.RevokeNoop {
		t.Fatalf("unknown-id revoke: outcome=%d err=%v, want RevokeNoop", o, err)
	}
}

// TestWebInvite_Migration058Applies proves migration 058 applies cleanly: the
// table and every expected column exist, token_hash is NOT NULL, and there is
// NO plaintext column (SCN-093-09 / AC-1).
func TestWebInvite_Migration058Applies(t *testing.T) {
	_, pool := inviteRepo(t)
	ctx := context.Background()

	rows, err := pool.Query(ctx,
		`SELECT column_name, is_nullable
		   FROM information_schema.columns
		  WHERE table_name = 'web_registration_invites'
		  ORDER BY ordinal_position`)
	if err != nil {
		t.Fatalf("query columns: %v", err)
	}
	defer rows.Close()
	cols := map[string]string{}
	for rows.Next() {
		var name, nullable string
		if err := rows.Scan(&name, &nullable); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		cols[name] = nullable
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate columns: %v", err)
	}
	if len(cols) == 0 {
		t.Fatal("web_registration_invites table does not exist (migration 058 did not apply)")
	}
	want := []string{
		"id", "token_hash", "label", "created_by", "created_at",
		"expires_at", "used_at", "used_by", "revoked_at",
	}
	for _, c := range want {
		if _, ok := cols[c]; !ok {
			t.Errorf("expected column %q missing", c)
		}
	}
	if cols["token_hash"] != "NO" {
		t.Errorf("token_hash is_nullable = %q, want NO", cols["token_hash"])
	}
	// NO plaintext column may exist.
	for name := range cols {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "plaintext") || lower == "token" || lower == "token_plain" {
			t.Errorf("forbidden plaintext-bearing column present: %q", name)
		}
	}
}
