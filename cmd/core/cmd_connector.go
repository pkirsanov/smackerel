package main

// BUG-056-002 Scope B — `smackerel-core connector <connector> <subcommand>`
// operator surface. Today only the twitter connector exposes subcommands: the
// User-Context OAuth 2.0 Authorization-Code-with-PKCE authorize flow
// (authorize-begin | authorize-finalize | authorize-status), used to acquire
// and inspect the user-context token required by the user-owned endpoints
// (/2/users/me, bookmarks, liked_tweets).
//
// Exit codes (mirroring `users`/`auth`):
//
//	0  success
//	1  command-level failure (config/DB/exchange error)
//	2  invocation error (missing args, unknown connector/subcommand,
//	   missing required flags)
//
// The CLI shares the same DATABASE_URL + SMACKEREL_AUTH_TOKEN as the runtime
// server. It does NOT start the HTTP server, NATS subscribers, or any other
// long-lived goroutines — it runs to completion and exits.

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/smackerel/smackerel/internal/config"
	twitterConnector "github.com/smackerel/smackerel/internal/connector/twitter"
)

// runConnectorCommand dispatches `connector <connector> <subcommand>`.
func runConnectorCommand(ctx context.Context, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel-core connector <twitter> <subcommand> [flags...]")
		return 2
	}
	switch args[0] {
	case "twitter":
		return runConnectorTwitter(ctx, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "smackerel-core connector: unknown connector %q (want twitter)\n", args[0])
		return 2
	}
}

// runConnectorTwitter dispatches `connector twitter <authorize-*>`. Flag
// parsing and the required-flag contract are validated BEFORE any config load
// or DB connect, so an invalid invocation exits 2 without side effects (the
// connector_dispatch test relies on this ordering, mirroring the spec 060
// auth-CLI validate-before-connect structure).
func runConnectorTwitter(ctx context.Context, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: smackerel-core connector twitter <authorize-begin|authorize-finalize|authorize-status> [flags...]")
		return 2
	}
	sub := args[0]
	switch sub {
	case "authorize-begin", "authorize-finalize", "authorize-status":
		// recognized
	default:
		fmt.Fprintf(os.Stderr,
			"smackerel-core connector twitter: unknown subcommand %q (want authorize-begin|authorize-finalize|authorize-status)\n", sub)
		return 2
	}

	fs := flag.NewFlagSet("connector twitter "+sub, flag.ContinueOnError)
	userID := fs.String("user-id", twitterConnector.DefaultOwnerUserID,
		"owner user id the user-context token is persisted under")
	var state, code string
	if sub == "authorize-finalize" {
		fs.StringVar(&state, "state", "", "the state token printed by authorize-begin")
		fs.StringVar(&code, "code", "", "the authorization code copied from the redirect address bar")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if sub == "authorize-finalize" && (state == "" || code == "") {
		fmt.Fprintln(os.Stderr,
			"usage: smackerel-core connector twitter authorize-finalize --state <state> --code <code>")
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core connector twitter: config load: %v\n", err)
		return 1
	}
	if cfg.DatabaseURL == "" {
		fmt.Fprintln(os.Stderr, "smackerel-core connector twitter: DATABASE_URL is required")
		return 1
	}
	pool, err := openReplayPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core connector twitter: connect db: %v\n", err)
		return 1
	}
	defer pool.Close()

	svc, err := twitterConnector.NewAuthorizeService(pool, cfg.AuthToken, twitterConnector.TwitterOAuthConfig{
		ClientID:           cfg.TwitterOAuthClientID,
		ClientSecret:       cfg.TwitterOAuthClientSecret,
		RedirectURL:        cfg.TwitterOAuthRedirectURL,
		HTTPTimeoutSeconds: cfg.AuthOAuthHTTPTimeoutSeconds,
	}, *userID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core connector twitter: %v\n", err)
		return 1
	}

	switch sub {
	case "authorize-begin":
		return runConnectorTwitterAuthorizeBegin(ctx, svc, *userID)
	case "authorize-finalize":
		return runConnectorTwitterAuthorizeFinalize(ctx, svc, state, code, *userID)
	case "authorize-status":
		return runConnectorTwitterAuthorizeStatus(ctx, svc, *userID)
	default:
		// Unreachable — sub was validated above.
		return 2
	}
}

func runConnectorTwitterAuthorizeBegin(ctx context.Context, svc *twitterConnector.AuthorizeService, owner string) int {
	res, err := svc.Begin(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core connector twitter authorize-begin: %v\n", err)
		return 1
	}
	fmt.Printf("Twitter user-context authorize started for owner %q.\n\n", owner)
	fmt.Printf("1. Open this URL in a browser and authorize the app:\n\n  %s\n\n", res.AuthURL)
	fmt.Printf("2. After authorizing you are redirected to your registered redirect URI.\n")
	fmt.Printf("   Copy the `code` query parameter from the address bar.\n\n")
	fmt.Printf("3. Finalize (within 15 minutes — the state expires):\n\n")
	fmt.Printf("   ./smackerel.sh connector twitter authorize-finalize --state %s --code <code>\n", res.State)
	if owner != twitterConnector.DefaultOwnerUserID {
		fmt.Printf("   (append --user-id %s to finalize under the same owner)\n", owner)
	}
	return 0
}

func runConnectorTwitterAuthorizeFinalize(ctx context.Context, svc *twitterConnector.AuthorizeService, state, code, owner string) int {
	tok, err := svc.Finalize(ctx, state, code)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core connector twitter authorize-finalize: %v\n", err)
		return 1
	}
	fmt.Printf("Twitter user-context token persisted (encrypted) for owner %q.\n", owner)
	fmt.Printf("access token expires at: %s\n", tok.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"))
	if len(tok.Scopes) > 0 {
		fmt.Printf("scopes: %v\n", tok.Scopes)
	}
	fmt.Printf("the connector will now use this token for bookmarks/likes/users-me; refresh is automatic.\n")
	return 0
}

func runConnectorTwitterAuthorizeStatus(ctx context.Context, svc *twitterConnector.AuthorizeService, owner string) int {
	present, err := svc.Status(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "smackerel-core connector twitter authorize-status: %v\n", err)
		return 1
	}
	if present {
		fmt.Printf("twitter user-context: AUTHORIZED — a user-context token is persisted for owner %q\n", owner)
		return 0
	}
	fmt.Printf("twitter user-context: NOT AUTHORIZED for owner %q — run `./smackerel.sh connector twitter authorize-begin`\n", owner)
	return 0
}
