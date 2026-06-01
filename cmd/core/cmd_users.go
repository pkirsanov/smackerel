package main

// Spec 070 — `smackerel-core users <subcommand>` operator surface for
// the web operator credential layer (username/password login).
//
// Subcommands:
//
//   add <username>          Create a new user; prompts twice for password
//                           on TTY (no echo). Refuses to overwrite.
//   set-password <username> Rotate password for an existing user; same
//                           TTY prompt. Refuses to create.
//   list                    Print enrolled users (table form).
//
// Exit codes:
//
//   0  success
//   1  command-level failure (DB error, validation error, wrong password
//      pair, user-exists / user-missing depending on subcommand)
//   2  invocation error (missing args, unknown subcommand, no TTY)
//
// The CLI shares the same DATABASE_URL as the runtime server. It does
// NOT start the HTTP server, NATS subscribers, or any other long-lived
// goroutines — runs to completion and exits.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/smackerel/smackerel/internal/auth/webcreds"
	"github.com/smackerel/smackerel/internal/config"
	"golang.org/x/term"
)

// MinPasswordLength is the minimum operator-password length accepted
// by `users add` / `users set-password` (design §4.2). argon2id itself
// has no minimum, but the CLI refuses short passwords so a careless
// operator cannot stand up a trivially guessable account.
const MinPasswordLength = 12

// errPasswordTooShort is returned by enforceMinPasswordLength when the
// supplied password is shorter than MinPasswordLength bytes.
var errPasswordTooShort = fmt.Errorf("password must be at least %d characters", MinPasswordLength)

func enforceMinPasswordLength(pw string) error {
	if len(pw) < MinPasswordLength {
		return errPasswordTooShort
	}
	return nil
}

func runUsersCommand(ctx context.Context, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel-core users <add|set-password|list> [args...]")
		return 2
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core users: config load: %v\n", err)
		return 1
	}
	if cfg.DatabaseURL == "" {
		fmt.Fprintln(os.Stderr, "smackerel-core users: DATABASE_URL is required")
		return 1
	}
	pool, err := openReplayPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core users: connect: %v\n", err)
		return 1
	}
	defer pool.Close()
	repo, err := webcreds.NewPostgresRepo(pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core users: repo: %v\n", err)
		return 1
	}

	switch args[0] {
	case "add":
		return runUsersAdd(ctx, repo, args[1:], passwordPromptStdin)
	case "set-password":
		return runUsersSetPassword(ctx, repo, args[1:], passwordPromptStdin)
	case "list":
		return runUsersList(ctx, repo, os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "smackerel-core users: unknown subcommand %q (want add|set-password|list)\n", args[0])
		return 2
	}
}

// passwordPrompter abstracts TTY password input so tests can inject
// a deterministic reader. Returns (password, error). MUST read twice
// and verify equality before returning.
type passwordPrompter func(out io.Writer) (string, error)

func passwordPromptStdin(out io.Writer) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("password input requires a TTY (no piped stdin)")
	}
	fmt.Fprint(out, "Password: ")
	pw1, err := term.ReadPassword(fd)
	fmt.Fprintln(out)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	fmt.Fprint(out, "Confirm:  ")
	pw2, err := term.ReadPassword(fd)
	fmt.Fprintln(out)
	if err != nil {
		return "", fmt.Errorf("read password confirmation: %w", err)
	}
	if string(pw1) != string(pw2) {
		return "", errors.New("passwords do not match")
	}
	return string(pw1), nil
}

func runUsersAdd(ctx context.Context, repo webcreds.Repo, args []string, prompt passwordPrompter) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel-core users add <username>")
		return 2
	}
	username := strings.TrimSpace(args[0])
	if err := webcreds.ValidateUsername(username); err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core users add: %v\n", err)
		return 1
	}
	password, err := prompt(os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core users add: %v\n", err)
		return 1
	}
	if err := enforceMinPasswordLength(password); err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core users add: %v\n", err)
		return 1
	}
	if err := repo.UpsertPassword(ctx, username, password, true); err != nil {
		if errors.Is(err, webcreds.ErrUserExists) {
			fmt.Fprintf(os.Stderr, "smackerel-core users add: user %q already exists (use `set-password` to rotate)\n", username)
			return 1
		}
		fmt.Fprintf(os.Stderr, "smackerel-core users add: %v\n", err)
		return 1
	}
	fmt.Printf("user %q created\n", username)
	return 0
}

func runUsersSetPassword(ctx context.Context, repo webcreds.Repo, args []string, prompt passwordPrompter) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel-core users set-password <username>")
		return 2
	}
	username := strings.TrimSpace(args[0])
	if err := webcreds.ValidateUsername(username); err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core users set-password: %v\n", err)
		return 1
	}
	password, err := prompt(os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core users set-password: %v\n", err)
		return 1
	}
	if err := enforceMinPasswordLength(password); err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core users set-password: %v\n", err)
		return 1
	}
	if err := repo.UpsertPassword(ctx, username, password, false); err != nil {
		if errors.Is(err, webcreds.ErrUserNotFound) {
			fmt.Fprintf(os.Stderr, "smackerel-core users set-password: no such user %q (use `add` to create)\n", username)
			return 1
		}
		fmt.Fprintf(os.Stderr, "smackerel-core users set-password: %v\n", err)
		return 1
	}
	fmt.Printf("password for %q rotated\n", username)
	return 0
}

func runUsersList(ctx context.Context, repo webcreds.Repo, out io.Writer) int {
	rows, err := repo.List(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core users list: %v\n", err)
		return 1
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "USERNAME\tCREATED\tLAST_LOGIN")
	for _, u := range rows {
		last := "—"
		if u.LastLoginAt != nil {
			last = u.LastLoginAt.UTC().Format("2006-01-02T15:04:05Z")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n",
			u.Username,
			u.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			last,
		)
	}
	_ = tw.Flush()
	return 0
}
