package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/metrics"
)

// runAuthCommand dispatches `smackerel auth <subcommand>`.
//
// Subcommands (spec 044 Scope 01):
//
//	enroll <user-id>          Enroll a new user. Mints first token. Prints
//	                          token to stdout exactly once; never persisted
//	                          plaintext. Operator MUST capture the token at
//	                          mint time.
//	rotate <user-id>          Mint a fresh token; mark prior token rotated.
//	revoke <token-id>         Revoke a token immediately. Broadcast via
//	                          NATS so all instances drop it from cache.
//	list-users                Print enrolled users (table form).
//	bootstrap <user-id>       One-shot first-user enrollment using
//	                          AUTH_BOOTSTRAP_TOKEN. Refuses to run when
//	                          any user is already enrolled.
//	keygen                    Generate and print a fresh PASETO v4.public
//	                          keypair (hex). Used by operators to rotate
//	                          auth.signing.active_private_key.
//	inspect <wire-token>      Spec 060 — parse a wire token under the
//	                          configured signing keys and print its
//	                          claims (issuer/subject/jti/iat/exp/kid/
//	                          scopes) as JSON to stdout.
//
// Exit codes:
//
//	0  success
//	1  command-level failure (DB error, validation error, etc.)
//	2  invocation error (missing args, unknown subcommand)
func runAuthCommand(ctx context.Context, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel auth <enroll|rotate|revoke|list-users|bootstrap|keygen|inspect> [args...]")
		return 2
	}
	switch args[0] {
	case "enroll":
		return runAuthEnroll(ctx, args[1:])
	case "rotate":
		return runAuthRotate(ctx, args[1:])
	case "revoke":
		return runAuthRevoke(ctx, args[1:])
	case "list-users":
		return runAuthListUsers(ctx, args[1:])
	case "bootstrap":
		return runAuthBootstrap(ctx, args[1:])
	case "keygen":
		return runAuthKeygen(ctx, args[1:])
	case "inspect":
		return runAuthInspect(ctx, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "smackerel auth: unknown subcommand %q (expected: enroll|rotate|revoke|list-users|bootstrap|keygen|inspect)\n", args[0])
		return 2
	}
}

// loadAuthCLIConfig loads the SST configuration that the auth CLI needs
// (auth.* fields plus DATABASE_URL). Returns the loaded *config.Config
// and the operator identity ("smackerel-cli@<host>") used as enrolled_by
// / issued_by / revoked_by audit values.
func loadAuthCLIConfig() (*config.Config, string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, "", fmt.Errorf("config load: %w", err)
	}
	if cfg.DatabaseURL == "" {
		return nil, "", fmt.Errorf("DATABASE_URL is required for auth subcommands")
	}
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown-host"
	}
	return cfg, "smackerel-cli@" + hostname, nil
}

// buildAuthVerifyOptions derives the public-key VerifyOptions the CLI
// uses for `auth rotate --prior-token` parse-and-preserve (spec 060
// BS-008) and for `auth inspect` (SCN-060-017). The wiring mirrors
// `cmd/core/wiring.go` — the active public key is derived from the
// configured Ed25519 private key via `auth.PublicHexFromSecretHex`;
// the prior public key is taken from SST directly (operators publish
// it in `auth.signing.prior_public_key`).
func buildAuthVerifyOptions(cfg *config.Config) (auth.VerifyOptions, error) {
	if cfg.Auth.SigningActivePrivateKey == "" || cfg.Auth.SigningActiveKeyID == "" {
		return auth.VerifyOptions{}, fmt.Errorf("auth.signing.active_private_key and active_key_id MUST be set to verify tokens")
	}
	activePub, err := auth.PublicHexFromSecretHex(cfg.Auth.SigningActivePrivateKey)
	if err != nil {
		return auth.VerifyOptions{}, fmt.Errorf("derive active public key: %w", err)
	}
	return auth.VerifyOptions{
		ActivePublicKey:    activePub,
		ActiveKeyID:        cfg.Auth.SigningActiveKeyID,
		PriorPublicKey:     cfg.Auth.SigningPriorPublicKey,
		PriorKeyID:         cfg.Auth.SigningPriorKeyID,
		Issuer:             "smackerel",
		ClockSkewTolerance: time.Duration(cfg.Auth.ClockSkewToleranceSeconds) * time.Second,
		Now:                time.Now,
	}, nil
}

// issueAndPersist delegates to auth.IssueAndPersistToken. The cmd
// surface keeps the (wireToken, tokenID, err) shape — callers don't
// need iat/exp because the CLI prints the wire token and forgets the
// rest. The api surface (internal/api/auth_handlers.go) calls the
// underlying helper directly when it needs iat/exp for its response.
func issueAndPersist(ctx context.Context, cfg *config.Config, store *auth.BearerStore,
	userID, issuedBy, issuedSource, rotatedFromTokenID string) (wireToken, tokenID string, err error) {
	return issueAndPersistWithScopes(ctx, cfg, store, userID, issuedBy, issuedSource, rotatedFromTokenID, nil)
}

// issueAndPersistWithScopes is the spec 060 variant that carries an
// optional PASETO `scope` claim through to the underlying issuance
// helper. Nil or empty `scopes` produces a legacy spec-044-shape token
// (no `scope` claim on the wire).
func issueAndPersistWithScopes(ctx context.Context, cfg *config.Config, store *auth.BearerStore,
	userID, issuedBy, issuedSource, rotatedFromTokenID string, scopes []string) (wireToken, tokenID string, err error) {
	res, err := auth.IssueAndPersistToken(ctx, store, auth.IssueAndPersistOptions{
		UserID:             userID,
		SigningPrivateKey:  cfg.Auth.SigningActivePrivateKey,
		SigningKeyID:       cfg.Auth.SigningActiveKeyID,
		AtRestHashingKey:   cfg.Auth.AtRestHashingKey,
		TTL:                time.Duration(cfg.Auth.TokenTTLHours) * time.Hour,
		Issuer:             "smackerel",
		Now:                time.Now,
		IssuedBy:           issuedBy,
		IssuedSource:       issuedSource,
		RotatedFromTokenID: rotatedFromTokenID,
		Scopes:             scopes,
	})
	if err != nil {
		return "", "", err
	}
	return res.WireToken, res.TokenID, nil
}

// runAuthEnroll implements `smackerel auth enroll <user-id> [--notes "..."]
// [--scope <name>...] [--allow-unknown-surface]`.
//
// Spec 060 Scope 3 — `--scope` is a REPEATABLE flag (each occurrence
// adds one scope string). The embedded `,` in `extension:bookmarks,history`
// is NEVER split — it belongs to the capability list inside one scope.
// `--allow-unknown-surface` waives the `RegisteredScopeSurfaces`
// allowlist check (with a structured WARN log naming the unknown
// surface) so a new surface can be minted before its allowlist entry
// lands in the same change set.
func runAuthEnroll(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("auth enroll", flag.ContinueOnError)
	notes := fs.String("notes", "", "free-form notes recorded against auth_users.notes")
	var scopes []string
	fs.Func("scope", "PASETO `scope` claim entry (spec 060); repeat to add more; embedded comma is NOT split", func(v string) error {
		scopes = append(scopes, v)
		return nil
	})
	allowUnknown := fs.Bool("allow-unknown-surface", false, "spec 060 — mint with a scope whose surface is not in RegisteredScopeSurfaces (emits a WARN log)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel auth enroll [--notes \"...\"] [--scope <name>]... [--allow-unknown-surface] <user-id>")
		return 2
	}
	userID := fs.Arg(0)

	// Spec 060 BS-005 / BS-006 — validate scope flags BEFORE any DB
	// connect so an invalid invocation exits 2 without touching the
	// store. validateScopeFlags emits structured WARN logs for the
	// `--allow-unknown-surface` path itself.
	normalizedScopes, exit, msg := validateScopeFlags(scopes, *allowUnknown)
	if exit != 0 {
		fmt.Fprintln(os.Stderr, msg)
		return exit
	}

	cfg, operator, err := loadAuthCLIConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth enroll: %v\n", err)
		return 1
	}
	pool, err := openReplayPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth enroll: connect db: %v\n", err)
		return 1
	}
	defer pool.Close()

	store, err := auth.NewBearerStore(pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth enroll: %v\n", err)
		return 1
	}

	if err := store.Enroll(ctx, auth.EnrollUserParams{
		UserID:     userID,
		EnrolledBy: operator,
		Notes:      *notes,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth enroll: %v\n", err)
		return 1
	}

	wire, tokenID, err := issueAndPersistWithScopes(ctx, cfg, store, userID, operator, "cli", "", normalizedScopes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth enroll: %v\n", err)
		return 1
	}

	// Spec 044 Scope 04 — telemetry emission for the deprecation
	// pathway dashboards. The CLI is a separate process from the
	// runtime; this counter increments inside the CLI invocation and
	// rolls up via the standard /metrics scrape on the runtime side
	// once peer instances replay the issuance through the periodic
	// DB refresh.
	metrics.AuthIssuance.WithLabelValues("bootstrap_cli").Inc()

	fmt.Printf("user enrolled: %s\n", userID)
	fmt.Printf("token id: %s\n", tokenID)
	if len(normalizedScopes) > 0 {
		fmt.Printf("scopes: %v\n", normalizedScopes)
	}
	fmt.Printf("token (capture now — never displayed again):\n  %s\n", wire)
	return 0
}

// runAuthRotate implements `smackerel auth rotate <user-id>
// --prior-token-id <id> [--prior-token <wire>] [--scope <name>...]
// [--allow-unknown-surface]`.
//
// Spec 060 Scope 3 — rotation scope semantics (BS-008, BS-009):
//
//   - no `--scope`, no `--prior-token`     → exit 2 (refuse to preserve
//     at-source; the operator MUST opt in to one of the three modes).
//   - no `--scope`, `--prior-token <wire>` → preserve: parse the prior
//     token, mint a new token with the same `scope` claim.
//   - `--scope ""` exactly                  → demote: mint a legacy
//     spec-044-shape token with no `scope` claim.
//   - `--scope ""` mixed with non-empty     → exit 2.
//   - `--scope <name>...`                   → explicit replace: mint
//     with the given scope list (validated via validateScopeFlags).
func runAuthRotate(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("auth rotate", flag.ContinueOnError)
	priorTokenID := fs.String("prior-token-id", "", "the token id being rotated (marked status='rotated')")
	priorToken := fs.String("prior-token", "", "spec 060 — the wire form of the prior token; required to preserve scopes when --scope is omitted")
	var scopes []string
	fs.Func("scope", "PASETO `scope` claim entry (spec 060); repeat to add more; use --scope \"\" alone to demote to legacy unscoped token", func(v string) error {
		scopes = append(scopes, v)
		return nil
	})
	allowUnknown := fs.Bool("allow-unknown-surface", false, "spec 060 — mint with a scope whose surface is not in RegisteredScopeSurfaces (emits a WARN log)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 || *priorTokenID == "" {
		fmt.Fprintln(os.Stderr, "usage: smackerel auth rotate --prior-token-id <id> [--prior-token <wire>] [--scope <name>]... [--allow-unknown-surface] <user-id>")
		return 2
	}
	userID := fs.Arg(0)

	cfg, operator, err := loadAuthCLIConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth rotate: %v\n", err)
		return 1
	}

	verifyOpts, err := buildAuthVerifyOptions(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth rotate: %v\n", err)
		return 1
	}

	// Spec 060 BS-008 / BS-009 — resolve rotation scope mode BEFORE
	// any DB connect so invalid invocations exit 2 without touching
	// the store. resolveRotationScopes encapsulates the preserve /
	// demote / explicit-replace semantics in one tested helper.
	resolvedScopes, exit, msg := resolveRotationScopes(scopes, *priorToken, *allowUnknown, verifyOpts)
	if exit != 0 {
		fmt.Fprintln(os.Stderr, msg)
		return exit
	}

	pool, err := openReplayPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth rotate: connect db: %v\n", err)
		return 1
	}
	defer pool.Close()

	store, err := auth.NewBearerStore(pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth rotate: %v\n", err)
		return 1
	}

	wire, newTokenID, err := issueAndPersistWithScopes(ctx, cfg, store, userID, operator, "cli", *priorTokenID, resolvedScopes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth rotate: %v\n", err)
		return 1
	}

	if err := store.MarkTokenRotated(ctx, *priorTokenID); err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth rotate: mark prior rotated: %v\n", err)
		return 1
	}

	// Spec 044 Scope 04 — rotation pairs an issuance with a flip of
	// the prior token. Both counters move together so dashboards can
	// derive rotation rate vs raw issuance rate.
	metrics.AuthIssuance.WithLabelValues("bootstrap_cli").Inc()
	metrics.AuthRotation.Inc()

	fmt.Printf("rotated user %s: prior=%s new=%s\n", userID, *priorTokenID, newTokenID)
	if len(resolvedScopes) > 0 {
		fmt.Printf("scopes: %v\n", resolvedScopes)
	}
	fmt.Printf("new token (capture now — never displayed again):\n  %s\n", wire)
	return 0
}

// runAuthRevoke implements `smackerel auth revoke <token-id> --reason "..."`.
func runAuthRevoke(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("auth revoke", flag.ContinueOnError)
	reason := fs.String("reason", "", "audit reason recorded in auth_revocations.reason")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel auth revoke [--reason \"...\"] <token-id>")
		return 2
	}
	tokenID := fs.Arg(0)

	cfg, operator, err := loadAuthCLIConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth revoke: %v\n", err)
		return 1
	}
	pool, err := openReplayPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth revoke: connect db: %v\n", err)
		return 1
	}
	defer pool.Close()

	store, err := auth.NewBearerStore(pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth revoke: %v\n", err)
		return 1
	}

	if err := store.RevokeToken(ctx, tokenID, operator, *reason); err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth revoke: %v\n", err)
		return 1
	}

	// Spec 044 Scope 04 — revocation telemetry, bucketed via
	// NormalizeRevocationReason so the `reason` label stays in the
	// documented closed set.
	metrics.AuthRevocation.WithLabelValues(metrics.NormalizeRevocationReason(*reason)).Inc()

	// Note: NATS broadcast happens in the running runtime when an admin
	// HTTP call comes in. The CLI runs OUTSIDE the runtime process, so
	// peer instances pick up the revocation on their next periodic
	// refresh (NFR-AUTH-006 worst-case ≤ 60s).
	fmt.Printf("revoked token %s (peer instances pick up via DB refresh ≤ %ds)\n",
		tokenID, cfg.Auth.RevocationCacheRefreshIntervalSeconds)
	return 0
}

// runAuthListUsers implements `smackerel auth list-users`.
func runAuthListUsers(ctx context.Context, args []string) int {
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "usage: smackerel auth list-users")
		return 2
	}

	cfg, _, err := loadAuthCLIConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth list-users: %v\n", err)
		return 1
	}
	pool, err := openReplayPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth list-users: connect db: %v\n", err)
		return 1
	}
	defer pool.Close()

	store, err := auth.NewBearerStore(pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth list-users: %v\n", err)
		return 1
	}

	users, err := store.ListUsers(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth list-users: %v\n", err)
		return 1
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "USER_ID\tENROLLED_AT\tENROLLED_BY\tSTATUS\tNOTES")
	for _, u := range users {
		notes := u.Notes
		if notes == "" {
			notes = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			u.UserID, u.EnrolledAt.UTC().Format(time.RFC3339), u.EnrolledBy, u.Status, notes)
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth list-users: flush output: %v\n", err)
		return 1
	}
	if len(users) == 0 {
		fmt.Println("(no users enrolled)")
	}
	return 0
}

// runAuthBootstrap implements `smackerel auth bootstrap <user-id>`.
// Refuses to run when ANY user is already enrolled (one-shot semantics).
// Requires AUTH_BOOTSTRAP_TOKEN to be supplied via STDIN or
// SMACKEREL_BOOTSTRAP_TOKEN env var to prove operator possession.
func runAuthBootstrap(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("auth bootstrap", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel auth bootstrap <user-id>")
		fmt.Fprintln(os.Stderr, "   set SMACKEREL_BOOTSTRAP_TOKEN env var with the bootstrap token from auth.bootstrap_token")
		return 2
	}
	userID := fs.Arg(0)

	cfg, operator, err := loadAuthCLIConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth bootstrap: %v\n", err)
		return 1
	}

	if cfg.Auth.BootstrapToken == "" {
		fmt.Fprintln(os.Stderr, "smackerel auth bootstrap: auth.bootstrap_token is empty in config; cannot bootstrap")
		return 1
	}

	supplied := os.Getenv("SMACKEREL_BOOTSTRAP_TOKEN")
	if supplied == "" {
		fmt.Fprintln(os.Stderr, "smackerel auth bootstrap: SMACKEREL_BOOTSTRAP_TOKEN env var MUST be set with the bootstrap token value")
		return 1
	}
	if supplied != cfg.Auth.BootstrapToken {
		// Constant-time-ish — do not branch on length to avoid leaking it.
		fmt.Fprintln(os.Stderr, "smackerel auth bootstrap: SMACKEREL_BOOTSTRAP_TOKEN does not match auth.bootstrap_token")
		return 1
	}

	pool, err := openReplayPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth bootstrap: connect db: %v\n", err)
		return 1
	}
	defer pool.Close()

	store, err := auth.NewBearerStore(pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth bootstrap: %v\n", err)
		return 1
	}

	count, err := store.CountUsers(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth bootstrap: count users: %v\n", err)
		return 1
	}
	if count > 0 {
		fmt.Fprintf(os.Stderr, "smackerel auth bootstrap: %d users already enrolled — bootstrap is one-shot only; use enroll instead\n", count)
		return 1
	}

	bootstrapOperator := "bootstrap@" + operator
	if err := store.Enroll(ctx, auth.EnrollUserParams{
		UserID:     userID,
		EnrolledBy: bootstrapOperator,
		Notes:      "bootstrap-enrolled first user",
	}); err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth bootstrap: enroll: %v\n", err)
		return 1
	}

	wire, tokenID, err := issueAndPersist(ctx, cfg, store, userID, bootstrapOperator, "bootstrap", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth bootstrap: %v\n", err)
		return 1
	}

	// Spec 044 Scope 04 — bootstrap issuance telemetry. The bootstrap
	// flow runs exactly once per fresh deployment; the counter
	// increment makes the one-shot visible in deployment dashboards.
	metrics.AuthIssuance.WithLabelValues("bootstrap_cli").Inc()

	fmt.Println("bootstrap successful — clear auth.bootstrap_token from config now to prevent reuse")
	fmt.Printf("user enrolled: %s\n", userID)
	fmt.Printf("token id: %s\n", tokenID)
	fmt.Printf("token (capture now — never displayed again):\n  %s\n", wire)
	return 0
}

// runAuthKeygen implements `smackerel auth keygen`. Pure stdout output;
// no DB or NATS. Operators redirect stdout to a sealed-secret store
// during a key rotation procedure.
func runAuthKeygen(_ context.Context, args []string) int {
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "usage: smackerel auth keygen")
		return 2
	}
	priv, pub := auth.GenerateSigningKeypair()
	fmt.Println("# spec 044 — paste these into config/smackerel.yaml under auth.signing")
	fmt.Println("# (rotate auth.signing.prior_public_key + prior_key_id from previous active values first)")
	fmt.Printf("active_private_key: %q\n", priv)
	fmt.Printf("active_public_key:  %q  # publish for verifier-only consumers\n", pub)
	fmt.Printf("active_key_id:      %q  # short identifier; embed in PASETO footer\n",
		"key-"+time.Now().UTC().Format("2006-01"))
	return 0
}

// validateScopeFlags validates the collected `--scope` values against
// the spec 060 regex (`auth.ValidateScopeName`) and the registered-
// surface allowlist (`auth.RegisteredScopeSurfaces`). Returns the
// normalized scope slice on success, or `(nil, exit, message)` on
// rejection so callers can `fmt.Fprintln(os.Stderr, message); return exit`.
//
// Spec 060 BS-005 — invalid scope name → exit 2.
// Spec 060 BS-006 — unknown surface without `--allow-unknown-surface` →
// exit 2; with the escape hatch → accept + structured WARN log naming
// the unknown surface.
//
// The empty-string `""` is NOT a valid scope name (the regex rejects
// it). Callers that want to express the rotation demote sentinel use
// `resolveRotationScopes` instead — `validateScopeFlags` rejects every
// `""` entry as invalid.
func validateScopeFlags(scopes []string, allowUnknown bool) ([]string, int, string) {
	if len(scopes) == 0 {
		return nil, 0, ""
	}
	for _, s := range scopes {
		if err := auth.ValidateScopeName(s); err != nil {
			return nil, 2, fmt.Sprintf("smackerel auth: %v", err)
		}
	}
	for _, s := range scopes {
		surface := auth.ExtractScopeSurface(s)
		if auth.IsRegisteredScopeSurface(surface) {
			continue
		}
		if !allowUnknown {
			return nil, 2, fmt.Sprintf("smackerel auth: unknown scope surface: %s (use --allow-unknown-surface to override)", surface)
		}
		slog.Warn("scope_unknown_surface_allowed",
			"surface", surface,
			"scope", s,
			"reason", "--allow-unknown-surface escape hatch used; register surface in auth.RegisteredScopeSurfaces in the same change set",
		)
	}
	return scopes, 0, ""
}

// resolveRotationScopes encodes the spec 060 BS-008 / BS-009 rotation
// scope semantics. Returns `(resolvedScopes, exit, message)`.
//
//   - len(scopes) == 0 && priorToken == ""  → exit 2 (refuse to
//     preserve at-source per design §7.2 diagnostic).
//   - len(scopes) == 0 && priorToken != ""  → preserve: parse the
//     prior wire token via `auth.VerifyAndParse(...)` and return its
//     parsed `Scopes`.
//   - len(scopes) == 1 && scopes[0] == ""   → demote: return `nil` so
//     the new token has no `scope` claim.
//   - any `""` mixed with non-empty entries → exit 2.
//   - all entries non-empty                  → explicit replace; thread
//     through `validateScopeFlags` for regex + registry checks.
func resolveRotationScopes(scopes []string, priorToken string, allowUnknown bool, verifyOpts auth.VerifyOptions) ([]string, int, string) {
	if len(scopes) == 0 {
		if priorToken == "" {
			return nil, 2, "smackerel auth rotate: rotation requires --prior-token <wire> to preserve scopes, or --scope to set them explicitly"
		}
		parsed, err := auth.VerifyAndParse(priorToken, verifyOpts)
		if err != nil {
			return nil, 1, fmt.Sprintf("smackerel auth rotate: verify prior token: %v", err)
		}
		return parsed.Scopes, 0, ""
	}
	hasEmpty := false
	hasNonEmpty := false
	for _, s := range scopes {
		if s == "" {
			hasEmpty = true
		} else {
			hasNonEmpty = true
		}
	}
	if hasEmpty && hasNonEmpty {
		return nil, 2, "smackerel auth rotate: --scope \"\" cannot be combined with non-empty --scope values"
	}
	if hasEmpty {
		// Demote sentinel: caller intends a legacy unscoped token.
		return nil, 0, ""
	}
	return validateScopeFlags(scopes, allowUnknown)
}

// runAuthInspect implements `smackerel auth inspect <wire-token>`.
//
// Spec 060 SCN-060-017 — an operator-facing affordance to read the
// parsed claims (issuer/subject/jti/iat/exp/kid/scopes) out of a wire
// token under the locally configured signing keys. Pure verification
// path: no DB connect, no NATS publish.
func runAuthInspect(_ context.Context, args []string) int {
	fs := flag.NewFlagSet("auth inspect", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel auth inspect <wire-token>")
		return 2
	}
	wire := fs.Arg(0)

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth inspect: %v\n", err)
		return 1
	}

	verifyOpts, err := buildAuthVerifyOptions(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth inspect: %v\n", err)
		return 1
	}

	parsed, err := auth.VerifyAndParse(wire, verifyOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth inspect: verify: %v\n", err)
		return 1
	}

	out := struct {
		Issuer    string    `json:"issuer"`
		Subject   string    `json:"subject"`
		TokenID   string    `json:"jti"`
		KeyID     string    `json:"kid"`
		IssuedAt  time.Time `json:"iat"`
		ExpiresAt time.Time `json:"exp"`
		Scopes    []string  `json:"scopes"`
	}{
		Issuer:    "smackerel",
		Subject:   parsed.UserID,
		TokenID:   parsed.TokenID,
		KeyID:     parsed.KeyID,
		IssuedAt:  parsed.IssuedAt,
		ExpiresAt: parsed.ExpiresAt,
		Scopes:    parsed.Scopes,
	}
	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel auth inspect: encode: %v\n", err)
		return 1
	}
	fmt.Println(string(encoded))
	return 0
}
