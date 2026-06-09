package main

// Spec 070 SCOPE-3 — pure-logic tests for the `users` CLI surface.
// Uses an in-memory webcreds.Repo mock; does NOT require Postgres.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth/webcreds"
)

// memRepo is a minimal in-memory webcreds.Repo for CLI dispatch tests.
type memRepo struct {
	mu    sync.Mutex
	users map[string]memUser
}

type memUser struct {
	password    string
	createdAt   time.Time
	lastLoginAt *time.Time
}

func newMemRepo() *memRepo { return &memRepo{users: map[string]memUser{}} }

func (r *memRepo) UpsertPassword(_ context.Context, username, password string, create bool) error {
	if err := webcreds.ValidateUsername(username); err != nil {
		return err
	}
	if _, err := webcreds.Hash(password); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, exists := r.users[username]
	switch {
	case create && exists:
		return webcreds.ErrUserExists
	case !create && !exists:
		return webcreds.ErrUserNotFound
	}
	if exists {
		u := r.users[username]
		u.password = password
		r.users[username] = u
	} else {
		r.users[username] = memUser{password: password, createdAt: time.Unix(1_700_000_000, 0).UTC()}
	}
	return nil
}

func (r *memRepo) VerifyAndTouch(_ context.Context, username, password string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[username]
	if !ok || u.password != password {
		return webcreds.ErrInvalidCredentials
	}
	now := time.Now().UTC()
	u.lastLoginAt = &now
	r.users[username] = u
	return nil
}

func (r *memRepo) List(_ context.Context) ([]webcreds.UserRow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]webcreds.UserRow, 0, len(r.users))
	for name, u := range r.users {
		out = append(out, webcreds.UserRow{Username: name, CreatedAt: u.createdAt, LastLoginAt: u.lastLoginAt})
	}
	return out, nil
}

func (r *memRepo) Exists(_ context.Context, username string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.users[username]
	return ok, nil
}

// fixedPrompt returns a passwordPrompter that yields the given value.
func fixedPrompt(pw string, err error) passwordPrompter {
	return func(_ io.Writer) (string, error) { return pw, err }
}

func TestRunUsersAdd_CreatesNewUser(t *testing.T) {
	repo := newMemRepo()
	rc := runUsersAdd(context.Background(), repo, []string{"operator"}, fixedPrompt("correct-horse-battery-staple", nil))
	if rc != 0 {
		t.Fatalf("expected exit 0, got %d", rc)
	}
	ok, _ := repo.Exists(context.Background(), "operator")
	if !ok {
		t.Fatalf("expected user to be created")
	}
}

func TestRunUsersAdd_RefusesExistingUser(t *testing.T) {
	repo := newMemRepo()
	_ = repo.UpsertPassword(context.Background(), "operator", "correct-horse-battery-staple", true)
	rc := runUsersAdd(context.Background(), repo, []string{"operator"}, fixedPrompt("correct-horse-battery-staple", nil))
	if rc != 1 {
		t.Fatalf("expected exit 1 (user exists), got %d", rc)
	}
}

func TestRunUsersAdd_UsageWhenMissingArg(t *testing.T) {
	repo := newMemRepo()
	rc := runUsersAdd(context.Background(), repo, nil, fixedPrompt("correct-horse-battery-staple", nil))
	if rc != 2 {
		t.Fatalf("expected exit 2 (usage), got %d", rc)
	}
}

func TestRunUsersAdd_RejectsEmptyUsername(t *testing.T) {
	repo := newMemRepo()
	for _, name := range []string{"", "   "} {
		rc := runUsersAdd(context.Background(), repo, []string{name}, fixedPrompt("correct-horse-battery-staple", nil))
		if rc != 1 {
			t.Fatalf("username %q: expected exit 1, got %d", name, rc)
		}
	}
}

func TestRunUsersAdd_RejectsShortPassword(t *testing.T) {
	repo := newMemRepo()
	rc := runUsersAdd(context.Background(), repo, []string{"operator"}, fixedPrompt("short", nil))
	if rc != 1 {
		t.Fatalf("expected exit 1 (password too short), got %d", rc)
	}
	if ok, _ := repo.Exists(context.Background(), "operator"); ok {
		t.Fatalf("expected no user created after short-password rejection")
	}
}

func TestRunUsersAdd_RejectsMismatchedConfirmation(t *testing.T) {
	repo := newMemRepo()
	rc := runUsersAdd(context.Background(), repo, []string{"operator"},
		fixedPrompt("", errors.New("passwords do not match")))
	if rc != 1 {
		t.Fatalf("expected exit 1 on mismatch, got %d", rc)
	}
}

func TestRunUsersSetPassword_RotatesExistingUser(t *testing.T) {
	repo := newMemRepo()
	_ = repo.UpsertPassword(context.Background(), "operator", "correct-horse-battery-staple", true)
	rc := runUsersSetPassword(context.Background(), repo, []string{"operator"},
		fixedPrompt("brand-new-strong-passphrase", nil))
	if rc != 0 {
		t.Fatalf("expected exit 0, got %d", rc)
	}
	repo.mu.Lock()
	pw := repo.users["operator"].password
	repo.mu.Unlock()
	if pw != "brand-new-strong-passphrase" {
		t.Fatalf("expected password rotated, got %q", pw)
	}
}

func TestRunUsersSetPassword_RefusesMissingUser(t *testing.T) {
	repo := newMemRepo()
	rc := runUsersSetPassword(context.Background(), repo, []string{"ghost"},
		fixedPrompt("correct-horse-battery-staple", nil))
	if rc != 1 {
		t.Fatalf("expected exit 1 (no such user), got %d", rc)
	}
}

func TestRunUsersSetPassword_RejectsShortPassword(t *testing.T) {
	repo := newMemRepo()
	_ = repo.UpsertPassword(context.Background(), "operator", "correct-horse-battery-staple", true)
	rc := runUsersSetPassword(context.Background(), repo, []string{"operator"},
		fixedPrompt("short", nil))
	if rc != 1 {
		t.Fatalf("expected exit 1 (password too short), got %d", rc)
	}
}

func TestRunUsersList_PrintsHeaderAndRows(t *testing.T) {
	repo := newMemRepo()
	_ = repo.UpsertPassword(context.Background(), "operator", "correct-horse-battery-staple", true)
	var buf bytes.Buffer
	rc := runUsersList(context.Background(), repo, &buf)
	if rc != 0 {
		t.Fatalf("expected exit 0, got %d", rc)
	}
	out := buf.String()
	if !strings.Contains(out, "USERNAME") || !strings.Contains(out, "CREATED") || !strings.Contains(out, "LAST_LOGIN") {
		t.Fatalf("missing header columns in output:\n%s", out)
	}
	if !strings.Contains(out, "operator") {
		t.Fatalf("missing user row in output:\n%s", out)
	}
}

func TestRunUsersCommand_MissingArgs_Exit2(t *testing.T) {
	// runUsersCommand returns exit 2 (invocation error) for a missing
	// subcommand BEFORE any config load or DB connection, so this branch
	// is exercisable without Postgres.
	if rc := runUsersCommand(context.Background(), nil); rc != 2 {
		t.Fatalf("nil args: expected exit 2 (usage), got %d", rc)
	}
	if rc := runUsersCommand(context.Background(), []string{}); rc != 2 {
		t.Fatalf("empty args: expected exit 2 (usage), got %d", rc)
	}
}

func TestDispatchUsersSubcommand_UnknownSubcommand_Exit2(t *testing.T) {
	// The dispatcher's default branch maps any unrecognised subcommand to
	// exit 2. Exercised through the same switch the live CLI uses, against
	// an in-memory repo (no DB).
	repo := newMemRepo()
	rc := dispatchUsersSubcommand(context.Background(), repo, []string{"bogus"},
		fixedPrompt("correct-horse-battery-staple", nil), io.Discard)
	if rc != 2 {
		t.Fatalf("unknown subcommand: expected exit 2, got %d", rc)
	}
}

func TestDispatchUsersSubcommand_RoutesToKnownSubcommands(t *testing.T) {
	// Prove the dispatcher actually routes (not a no-op proxy): add
	// creates a user, set-password rotates it, list prints it — all
	// through the real switch against an in-memory repo.
	repo := newMemRepo()
	if rc := dispatchUsersSubcommand(context.Background(), repo, []string{"add", "operator"},
		fixedPrompt("correct-horse-battery-staple", nil), io.Discard); rc != 0 {
		t.Fatalf("add: expected exit 0, got %d", rc)
	}
	if ok, _ := repo.Exists(context.Background(), "operator"); !ok {
		t.Fatalf("add: dispatcher did not create the user")
	}
	if rc := dispatchUsersSubcommand(context.Background(), repo, []string{"set-password", "operator"},
		fixedPrompt("brand-new-strong-passphrase", nil), io.Discard); rc != 0 {
		t.Fatalf("set-password: expected exit 0, got %d", rc)
	}
	repo.mu.Lock()
	pw := repo.users["operator"].password
	repo.mu.Unlock()
	if pw != "brand-new-strong-passphrase" {
		t.Fatalf("set-password: dispatcher did not rotate password, got %q", pw)
	}
	var buf bytes.Buffer
	if rc := dispatchUsersSubcommand(context.Background(), repo, []string{"list"},
		fixedPrompt("", nil), &buf); rc != 0 {
		t.Fatalf("list: expected exit 0, got %d", rc)
	}
	if !strings.Contains(buf.String(), "operator") {
		t.Fatalf("list: dispatcher did not print the user row:\n%s", buf.String())
	}
}
