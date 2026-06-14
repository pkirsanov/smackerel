// Spec 093 SCOPE-01 — unit coverage for the webcreds.HashAndInsertTx free
// function: username validation runs before any DB call, the argon2id hash is
// produced, the INSERT is issued on the caller's tx, and a Postgres
// unique-violation (SQLSTATE 23505) maps to ErrUserExists.
//
// Only the EXTERNAL pgx.Tx driver boundary is faked (the allowed
// third-party-dependency exception); the function under test runs for real.
// The real INSERT against a real Postgres + the real 23505 round-trip are
// additionally exercised end-to-end in the integration tier (the spec-093
// SCOPE-01 duplicate-username rollback test and the SCOPE-02 /register
// duplicate test).

package webcreds

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeTx is a minimal pgx.Tx whose only meaningful method is Exec. The embedded
// nil pgx.Tx interface satisfies the remaining methods; HashAndInsertTx never
// calls them, so they are intentionally left to panic if ever invoked.
type fakeTx struct {
	pgx.Tx
	execErr    error
	execCalled bool
	gotSQL     string
}

func (f *fakeTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	f.execCalled = true
	f.gotSQL = sql
	return pgconn.CommandTag{}, f.execErr
}

const validTestPassword = "correct-horse-battery-staple" // ≥ MinPasswordLength

func TestHashAndInsertTx_Success(t *testing.T) {
	tx := &fakeTx{execErr: nil}
	if err := HashAndInsertTx(context.Background(), tx, "newcomer", validTestPassword); err != nil {
		t.Fatalf("HashAndInsertTx success path: got %v, want nil", err)
	}
	if !tx.execCalled {
		t.Fatal("Exec was not called on the happy path")
	}
	if !strings.Contains(tx.gotSQL, "INSERT INTO web_user_credentials") {
		t.Fatalf("unexpected SQL issued: %q", tx.gotSQL)
	}
}

func TestHashAndInsertTx_MapsUniqueViolation(t *testing.T) {
	tx := &fakeTx{execErr: &pgconn.PgError{Code: "23505"}}
	err := HashAndInsertTx(context.Background(), tx, "taken", validTestPassword)
	if !errors.Is(err, ErrUserExists) {
		t.Fatalf("23505 should map to ErrUserExists, got %v", err)
	}
}

func TestHashAndInsertTx_WrapsOtherError(t *testing.T) {
	sentinel := errors.New("transient pool error")
	tx := &fakeTx{execErr: sentinel}
	err := HashAndInsertTx(context.Background(), tx, "newcomer", validTestPassword)
	if err == nil {
		t.Fatal("expected a wrapped error, got nil")
	}
	if errors.Is(err, ErrUserExists) {
		t.Fatalf("a non-23505 error must NOT map to ErrUserExists, got %v", err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("underlying error not wrapped: got %v", err)
	}
}

func TestHashAndInsertTx_RejectsBadUsernameBeforeExec(t *testing.T) {
	tx := &fakeTx{}
	// A control character is rejected by ValidateUsername.
	err := HashAndInsertTx(context.Background(), tx, "bad\x00name", validTestPassword)
	if err == nil {
		t.Fatal("expected validation error for a control-character username")
	}
	if tx.execCalled {
		t.Fatal("Exec must NOT run when username validation fails")
	}
}

func TestHashAndInsertTx_RejectsShortPasswordBeforeExec(t *testing.T) {
	tx := &fakeTx{}
	err := HashAndInsertTx(context.Background(), tx, "newcomer", "short")
	if err == nil {
		t.Fatal("expected an error for a sub-minimum password")
	}
	if tx.execCalled {
		t.Fatal("Exec must NOT run when the password is too short")
	}
}
