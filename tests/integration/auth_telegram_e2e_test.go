//go:build integration

// Spec 044 Scope 03 — Telegram bridge per-user PASETO end-to-end test (T3-03 closure).
//
// The Telegram bot resolves chat_id → user_id via Bot.userMapping then
// derives actor identity from the resolved user. This test proves the
// full claim-binding chain end-to-end:
//
//  1. NewPerUserTokenMinter wraps the bot's resolution + auth.IssueToken
//     so the bot's outbound HTTP calls can carry a real per-user PASETO
//     bearer.
//  2. MintForChat(<mapped>) returns a bearer; using it on a protected
//     endpoint passes bearerAuthMiddleware.
//  3. MintForChat(<unmapped>) refuses in production — proving the
//     bot-side drop semantic.
//  4. Body-smuggling defense: a request that smuggles actor_id in the
//     JSON body is rejected with HTTP 400 even when carrying a valid
//     per-user PASETO. Proves the chain end-to-end (mint + verify + body
//     defense all agree).
//
// SCN-AUTH-008 (telegram surface) closure evidence.
package integration

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
	"github.com/smackerel/smackerel/internal/telegram"
)

// productionTelegramBridgeDeps wires a production-mode router that
// includes the AnnotationHandlers (so the body-smuggling defense path
// is reachable) plus the photos connectors endpoint (so the happy-path
// bearer call has a target). The pool is owned by the test and cleaned
// via t.Cleanup. Mapping is supplied by caller so subtests can share
// the same fixture if desired.
func productionTelegramBridgeDeps(
	t *testing.T,
	mapping map[int64]string,
) (
	deps *api.Dependencies,
	bot *telegram.Bot,
	minter *telegram.PerUserTokenMinter,
	pool *pgxpool.Pool,
) {
	t.Helper()
	pool = authTestPool(t)
	t.Cleanup(func() { pool.Close() })
	resetAuthTables(t, pool)

	priv, pub := auth.GenerateSigningKeypair()
	const kid = "scope03-telegram-key"

	store := photolib.NewStore(pool)
	cache := revocation.NewCache()
	annotationStore := annotation.NewStore(pool, nil) // nil NATS client tolerated

	deps = &api.Dependencies{
		Environment: "production",
		AuthConfig: config.AuthConfig{
			Enabled:                              true,
			TokenFormat:                          "paseto_v4_public",
			SigningActivePrivateKey:              priv,
			SigningActiveKeyID:                   kid,
			TokenTTLHours:                        24,
			RotationGraceWindowHours:             24,
			ClockSkewToleranceSeconds:            60,
			RevocationCacheRefreshIntervalSeconds: 60,
			AtRestHashingKey:                     priv + "-hash-suffix-distinct",
			ProductionSharedTokenFallbackEnabled: false,
		},
		AuthVerifyOptions: auth.VerifyOptions{
			ActivePublicKey:    pub,
			ActiveKeyID:        kid,
			Issuer:             "smackerel",
			ClockSkewTolerance: time.Minute,
			Now:                time.Now,
		},
		RevocationCache: cache,
		PhotosHandlers:  api.NewPhotosHandlers(store, config.PhotosConfig{}, "production"),
		AnnotationHandlers: &api.AnnotationHandlers{
			Store:       annotationStore,
			Environment: "production",
		},
	}

	bot = telegram.NewBotForTest("production", mapping)

	var err error
	minter, err = telegram.NewPerUserTokenMinter(telegram.PerUserTokenMinterOptions{
		Bot:        bot,
		SigningKey: priv,
		KeyID:      kid,
		Issuer:     "smackerel",
		TTL:        5 * time.Minute,
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("NewPerUserTokenMinter: %v", err)
	}
	return deps, bot, minter, pool
}

// TestTelegramBridge_MintsPerUserBearer_AdmitsRequest proves the happy
// path: a chat in the mapping → mint → bearer admitted by middleware.
func TestTelegramBridge_MintsPerUserBearer_AdmitsRequest(t *testing.T) {
	deps, _, minter, _ := productionTelegramBridgeDeps(t, map[int64]string{
		12345: "tg-user-alice",
	})

	tok, err := minter.MintForChat(12345)
	if err != nil {
		t.Fatalf("MintForChat: %v", err)
	}
	if tok.UserID != "tg-user-alice" {
		t.Fatalf("UserID=%q want tg-user-alice", tok.UserID)
	}

	srv := httptest.NewServer(api.NewRouter(deps))
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodGet, srv.URL+"/v1/photos/connectors", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.WireToken)

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", resp.StatusCode, string(body))
	}
}

// TestTelegramBridge_UnmappedChat_MinterRefusesAndCallerCannotProceed
// proves the production drop semantic. An attacker sending a Telegram
// message from an unknown chat:
//
//  1. The minter refuses (ErrNoUserMappingForChat).
//  2. Without a bearer, the bot's outbound API call is rejected 401.
//
// The chain therefore never persists an artifact tagged with the
// attacker's chat content.
func TestTelegramBridge_UnmappedChat_MinterRefusesAndCallerCannotProceed(t *testing.T) {
	deps, _, minter, _ := productionTelegramBridgeDeps(t, map[int64]string{
		12345: "tg-user-alice",
	})

	_, err := minter.MintForChat(99999)
	if !errors.Is(err, telegram.ErrNoUserMappingForChat) {
		t.Fatalf("MintForChat err=%v want %v", err, telegram.ErrNoUserMappingForChat)
	}

	// Belt-and-brace: confirm that even if the bot bypassed the minter
	// and issued a request without a bearer, the API would 401.
	srv := httptest.NewServer(api.NewRouter(deps))
	t.Cleanup(srv.Close)
	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodGet, srv.URL+"/v1/photos/connectors", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("no-bearer status=%d want 401; body=%s", resp.StatusCode, string(body))
	}
}

// TestTelegramBridge_BodyClaimedActorRejected proves the chain
// end-to-end: a Telegram-originated request that smuggles actor_id in
// the body is rejected by the annotation handler EVEN WHEN the request
// carries a valid per-user PASETO. The defense is layered — minting a
// real bearer does not buy the right to override session-derived
// actor identity.
func TestTelegramBridge_BodyClaimedActorRejected(t *testing.T) {
	deps, _, minter, pool := productionTelegramBridgeDeps(t, map[int64]string{
		12345: "tg-user-alice",
	})

	// Seed an artifact so the route is reachable. The handler reads
	// {id} from the URL; we just need ANY id since the body-smuggling
	// rejection fires before the store call.
	const artifactID = "art-tg-bridge-001"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
		VALUES ($1, $2, $3, $4, $5)
	`, artifactID, "telegram", "scope 044 scope03 telegram bridge seed",
		"hash-tg-001", "test-source"); err != nil {
		t.Fatalf("seed artifacts row: %v", err)
	}

	tok, err := minter.MintForChat(12345)
	if err != nil {
		t.Fatalf("MintForChat: %v", err)
	}

	srv := httptest.NewServer(api.NewRouter(deps))
	t.Cleanup(srv.Close)

	body := strings.NewReader(`{"text":"hi from tg","actor_id":"mallory"}`)
	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodPost,
		srv.URL+"/api/artifacts/"+artifactID+"/annotations/", body)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.WireToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	rbody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400; body=%s", resp.StatusCode, string(rbody))
	}
	if !strings.Contains(string(rbody), "actor_id") || !strings.Contains(string(rbody), "forbidden") {
		t.Errorf("expected body-smuggling rejection mentioning actor_id+forbidden; got %s", string(rbody))
	}
}
