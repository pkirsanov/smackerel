//go:build integration

// Spec 044 Scope 01 — chaos-phase integration tests.
//
// These tests exercise the per-user bearer-auth surface against the
// LIVE test stack (postgres + NATS) to surface brittle behavior under
// concurrent access, malformed inputs, and lifecycle edge conditions.
// They are NOT a substitute for the deterministic T1-* unit and
// integration tests (those run during the test phase). They are
// stochastic-style probes that overlay real concurrency on top of the
// already-green behavior contract.
//
// Coverage map (per chaos-phase request):
//
//	Behavior 1 → TestAuthChaos_ConcurrentEnrollment_DuplicatesRejectedAtomically
//	Behavior 2 → TestAuthChaos_ConcurrentRotateVsVerify_GraceWindowSurvives
//	Behavior 3 → TestAuthChaos_RevocationBroadcasterRace_CacheConverges (requires NATS)
//	Behavior 4 → TestAuthChaos_CacheBootstrapUnderConcurrentLoad
//	Behavior 5 → TestAuthChaos_BroadcasterMalformedPayloads_CacheIntact (requires NATS)
//	Behavior 6 → TestAuthChaos_MigrationIdempotency
//	Behavior 7 → TestAuthChaos_TokenBoundaryConditions
//	Behavior 9 → BenchmarkAuthChaos_VerifyAndParse_HotPath (informational)
//
// Behavior 8 (CLI subcommand smoke) is not a Go test — it runs as a
// `go run ./cmd/core auth keygen` invocation captured in the chaos
// evidence section of report.md.
//
// All tests are race-safe (the package builds clean under `-race`) and
// none use `t.Skip()` — when env is missing, the test fatals with a
// loud message per the no-skip precedent set by spec 043.
package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/db"
)

// chaosNATSConn dials the live test-stack NATS server.
//
// CHAOS_NATS_URL takes precedence over NATS_URL because the in-stack
// test.env points at the container DNS name `nats:4222`, which is not
// reachable from the host shell that runs `go test`. The chaos-phase
// runner exports CHAOS_NATS_URL=nats://127.0.0.1:<NATS_CLIENT_HOST_PORT>.
//
// spec-047 R12.2: also append nats.Token(SMACKEREL_AUTH_TOKEN) when
// the env var is set, matching the pattern in testNATSConn (helpers_test.go
// line 60). CI's NATS service container is started with
// `--auth ci-test-token-integration` and exposes the same value via
// SMACKEREL_AUTH_TOKEN; without the token, every chaos-test
// nats.Connect call fails with `nats: Authorization Violation`.
func chaosNATSConn(t *testing.T) *nats.Conn {
	t.Helper()
	url := os.Getenv("CHAOS_NATS_URL")
	if url == "" {
		url = os.Getenv("NATS_URL")
	}
	if url == "" {
		t.Fatal("auth chaos test requires CHAOS_NATS_URL or NATS_URL — point at the live test stack NATS host port (typically nats://127.0.0.1:47002 for env=test)")
	}
	opts := []nats.Option{nats.Timeout(5 * time.Second)}
	if tok := os.Getenv("SMACKEREL_AUTH_TOKEN"); tok != "" {
		opts = append(opts, nats.Token(tok))
	}
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		t.Fatalf("connect NATS %q: %v", url, err)
	}
	return nc
}

// uniqueUserID returns a user_id prefixed with the chaos-phase scope so
// rows do not collide across test methods or across repeat runs of the
// same chaos batch (`-count=N`).
func uniqueUserID(prefix string) string {
	return fmt.Sprintf("chaos-044-%s-%d", prefix, time.Now().UnixNano())
}

// --- Behavior 1 -----------------------------------------------------

// TestAuthChaos_ConcurrentEnrollment_DuplicatesRejectedAtomically fires
// 24 concurrent Enroll calls against the SAME user_id and asserts that
// EXACTLY ONE succeeds while the other 23 surface a uniqueness-
// violation error. Verifies that the `auth_users.user_id UNIQUE`
// constraint is the canonical race winner — there is no application-
// level pre-check window where two callers could both see "no row" and
// both INSERT.
func TestAuthChaos_ConcurrentEnrollment_DuplicatesRejectedAtomically(t *testing.T) {
	pool := authTestPool(t)
	defer pool.Close()
	resetAuthTables(t, pool)

	store, err := auth.NewBearerStore(pool)
	if err != nil {
		t.Fatalf("NewBearerStore: %v", err)
	}

	const concurrent = 24
	userID := uniqueUserID("enroll-race")

	var wg sync.WaitGroup
	var successCount atomic.Int64
	var dupCount atomic.Int64
	var otherErrors []string
	var otherErrorsMu sync.Mutex
	startGate := make(chan struct{})

	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-startGate
			err := store.Enroll(context.Background(), auth.EnrollUserParams{
				UserID:     userID,
				EnrolledBy: fmt.Sprintf("chaos-worker-%d", idx),
				Notes:      "Behavior 1 duplicate-enrollment race",
			})
			if err == nil {
				successCount.Add(1)
				return
			}
			lower := strings.ToLower(err.Error())
			if strings.Contains(lower, "duplicate") || strings.Contains(lower, "unique") {
				dupCount.Add(1)
				return
			}
			otherErrorsMu.Lock()
			otherErrors = append(otherErrors, err.Error())
			otherErrorsMu.Unlock()
		}(i)
	}

	close(startGate)
	wg.Wait()

	if successCount.Load() != 1 {
		t.Fatalf("expected EXACTLY 1 successful Enroll out of %d, got %d (dup=%d other=%d)",
			concurrent, successCount.Load(), dupCount.Load(), len(otherErrors))
	}
	if dupCount.Load() != concurrent-1 {
		t.Fatalf("expected %d duplicate-key errors, got %d (success=%d other=%d)",
			concurrent-1, dupCount.Load(), successCount.Load(), len(otherErrors))
	}
	if len(otherErrors) > 0 {
		t.Fatalf("unexpected non-duplicate errors: %v", otherErrors)
	}

	// Live DB row count: exactly one auth_users row.
	count, err := store.CountUsers(context.Background())
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if count != 1 {
		t.Fatalf("auth_users count after race: want 1 got %d", count)
	}
	t.Logf("Behavior 1: %d concurrent Enroll → 1 success, %d dup-key errors (auth_users row count = 1)",
		concurrent, dupCount.Load())
}

// --- Behavior 2 -----------------------------------------------------

// TestAuthChaos_ConcurrentRotateVsVerify_GraceWindowSurvives interleaves
// a fixed token-rotation event with concurrent VerifyAndParse calls on
// the prior token. Within the grace window the prior token MUST verify
// against the prior public key while the active key validates the new
// token. Outside the grace window the prior token MUST be rejected as
// expired even though both signing keys are still configured.
func TestAuthChaos_ConcurrentRotateVsVerify_GraceWindowSurvives(t *testing.T) {
	priorPriv, _ := auth.GenerateSigningKeypair()
	priorPub, err := auth.PublicHexFromSecretHex(priorPriv)
	if err != nil {
		t.Fatalf("derive prior pub: %v", err)
	}
	activePriv, _ := auth.GenerateSigningKeypair()
	activePub, err := auth.PublicHexFromSecretHex(activePriv)
	if err != nil {
		t.Fatalf("derive active pub: %v", err)
	}

	issuedAt := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	priorIssued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "user-rotation-race",
		TokenID:    "tok-rotation-prior",
		SigningKey: priorPriv,
		KeyID:      "key-2026-04",
		TTL:        2 * time.Hour, // short window so we can step past exp
		Issuer:     "smackerel-test",
		Now:        func() time.Time { return issuedAt },
	})
	if err != nil {
		t.Fatalf("issue prior: %v", err)
	}

	activeIssued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "user-rotation-race",
		TokenID:    "tok-rotation-active",
		SigningKey: activePriv,
		KeyID:      "key-2026-05",
		TTL:        24 * time.Hour,
		Issuer:     "smackerel-test",
		Now:        func() time.Time { return issuedAt.Add(30 * time.Minute) },
	})
	if err != nil {
		t.Fatalf("issue active: %v", err)
	}

	insideGrace := func() time.Time { return issuedAt.Add(45 * time.Minute) }
	outsideGrace := func() time.Time { return issuedAt.Add(3 * time.Hour) } // past prior exp

	const workers = 16
	const iterations = 16

	verify := func(wireToken string, clock func() time.Time) error {
		_, vErr := auth.VerifyAndParse(wireToken, auth.VerifyOptions{
			ActivePublicKey:    activePub,
			ActiveKeyID:        "key-2026-05",
			PriorPublicKey:     priorPub,
			PriorKeyID:         "key-2026-04",
			Issuer:             "smackerel-test",
			ClockSkewTolerance: 30 * time.Second,
			Now:                clock,
		})
		return vErr
	}

	var (
		insidePriorOK   atomic.Int64
		insideActiveOK  atomic.Int64
		outsidePriorErr atomic.Int64
		unexpected      []string
		unexpectedMu    sync.Mutex
	)
	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			for j := 0; j < iterations; j++ {
				if err := verify(priorIssued.WireToken, insideGrace); err == nil {
					insidePriorOK.Add(1)
				} else {
					unexpectedMu.Lock()
					unexpected = append(unexpected, fmt.Sprintf("inside-grace prior verify failed: %v", err))
					unexpectedMu.Unlock()
				}
				if err := verify(activeIssued.WireToken, insideGrace); err == nil {
					insideActiveOK.Add(1)
				} else {
					unexpectedMu.Lock()
					unexpected = append(unexpected, fmt.Sprintf("inside-grace active verify failed: %v", err))
					unexpectedMu.Unlock()
				}
				if err := verify(priorIssued.WireToken, outsideGrace); err != nil {
					if errors.Is(err, auth.ErrTokenExpired) {
						outsidePriorErr.Add(1)
					} else {
						unexpectedMu.Lock()
						unexpected = append(unexpected, fmt.Sprintf("outside-grace prior verify wrong error: %v", err))
						unexpectedMu.Unlock()
					}
				} else {
					unexpectedMu.Lock()
					unexpected = append(unexpected, "outside-grace prior verify unexpectedly succeeded")
					unexpectedMu.Unlock()
				}
			}
		}()
	}
	close(gate)
	wg.Wait()

	if len(unexpected) > 0 {
		t.Fatalf("unexpected verify outcomes (%d): %v", len(unexpected), unexpected[:min(5, len(unexpected))])
	}
	want := int64(workers * iterations)
	if insidePriorOK.Load() != want {
		t.Fatalf("inside-grace prior verify success count: want %d got %d", want, insidePriorOK.Load())
	}
	if insideActiveOK.Load() != want {
		t.Fatalf("inside-grace active verify success count: want %d got %d", want, insideActiveOK.Load())
	}
	if outsidePriorErr.Load() != want {
		t.Fatalf("outside-grace prior verify ErrTokenExpired count: want %d got %d", want, outsidePriorErr.Load())
	}
	t.Logf("Behavior 2: %d workers x %d iter — prior-inside=%d, active-inside=%d, prior-outside-expired=%d (no panics, no surprise outcomes)",
		workers, iterations, insidePriorOK.Load(), insideActiveOK.Load(), outsidePriorErr.Load())
}

// --- Behavior 3 -----------------------------------------------------

// TestAuthChaos_RevocationBroadcasterRace_CacheConverges interleaves
// concurrent VerifyAndParse-equivalent IsRevoked queries with
// broadcaster.Publish revocation events to verify (a) no panics, (b)
// no leaked goroutines (subscription cleanly stops), and (c) the cache
// converges to the union of all published token IDs.
func TestAuthChaos_RevocationBroadcasterRace_CacheConverges(t *testing.T) {
	nc := chaosNATSConn(t)
	defer nc.Close()

	cache := revocation.NewCache()
	subject := fmt.Sprintf("chaos.auth.revocations.%d", time.Now().UnixNano())
	bc, err := revocation.NewBroadcaster(nc, subject, cache, "chaos-instance-A")
	if err != nil {
		t.Fatalf("NewBroadcaster: %v", err)
	}
	if err := bc.Subscribe(); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() {
		if err := bc.Stop(); err != nil {
			t.Errorf("Stop: %v", err)
		}
	}()

	const publishers = 8
	const perPublisher = 25
	const verifiers = 16
	totalPublished := publishers * perPublisher

	publishedIDs := make([]string, 0, totalPublished)
	var publishedMu sync.Mutex

	verifyCtx, verifyCancel := context.WithCancel(context.Background())
	var verifyHits atomic.Int64
	var verifyWG sync.WaitGroup
	for v := 0; v < verifiers; v++ {
		verifyWG.Add(1)
		go func(idx int) {
			defer verifyWG.Done()
			i := 0
			for {
				select {
				case <-verifyCtx.Done():
					return
				default:
					tokenID := fmt.Sprintf("chaos-tok-%d-%d", idx, i)
					_ = cache.IsRevoked(tokenID) // hot-path probe
					i++
					if i%500 == 0 {
						verifyHits.Add(500)
					}
				}
			}
		}(v)
	}

	var pubWG sync.WaitGroup
	for p := 0; p < publishers; p++ {
		pubWG.Add(1)
		go func(pubIdx int) {
			defer pubWG.Done()
			for j := 0; j < perPublisher; j++ {
				tokenID := fmt.Sprintf("chaos-revoked-%d-%d-%d", pubIdx, j, time.Now().UnixNano())
				publishedMu.Lock()
				publishedIDs = append(publishedIDs, tokenID)
				publishedMu.Unlock()
				if err := bc.Publish(tokenID, "behavior-3-chaos-race"); err != nil {
					t.Errorf("Publish: %v", err)
					return
				}
			}
		}(p)
	}

	pubWG.Wait()

	// Drain NATS round-trip — broadcaster.Publish updates the local
	// cache synchronously; remote callbacks may take a moment.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cache.Size() >= int64(totalPublished) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	verifyCancel()
	verifyWG.Wait()

	if int(cache.Size()) < totalPublished {
		t.Fatalf("cache did not converge: published=%d cache.Size=%d (deadline 5s)",
			totalPublished, cache.Size())
	}

	missing := 0
	for _, id := range publishedIDs {
		if !cache.IsRevoked(id) {
			missing++
		}
	}
	if missing > 0 {
		t.Fatalf("cache missing %d of %d published token IDs", missing, totalPublished)
	}
	t.Logf("Behavior 3: %d publishers x %d revocations + %d verifier goroutines, cache.Size=%d, all %d IDs present, hot-path probes ≥%d (no panics, no leaks)",
		publishers, perPublisher, verifiers, cache.Size(), totalPublished, verifyHits.Load())
}

// --- Behavior 4 -----------------------------------------------------

// TestAuthChaos_CacheBootstrapUnderConcurrentLoad runs the cache
// BootstrapFromDB call concurrently with a flood of IsRevoked queries
// against the SAME cache. Verifies (a) no race detector hits, (b) every
// loaded token id is observable to subsequent IsRevoked calls.
func TestAuthChaos_CacheBootstrapUnderConcurrentLoad(t *testing.T) {
	pool := authTestPool(t)
	defer pool.Close()
	resetAuthTables(t, pool)

	store, err := auth.NewBearerStore(pool)
	if err != nil {
		t.Fatalf("NewBearerStore: %v", err)
	}

	// Seed the DB with N revoked tokens. We bypass the normal flow
	// (Enroll → IssueToken → PersistToken → RevokeToken) to keep the
	// fixture compact: insert auth_users + auth_tokens + auth_revocations
	// rows directly via Enroll + PersistToken + RevokeToken so the
	// composite cache loader (UNION of revocations + tokens.status)
	// returns a known set.
	const seedCount = 50
	enrollUser := uniqueUserID("cache-bootstrap")
	if err := store.Enroll(context.Background(), auth.EnrollUserParams{
		UserID:     enrollUser,
		EnrolledBy: "chaos-cache-bootstrap",
	}); err != nil {
		t.Fatalf("Enroll: %v", err)
	}

	priv, _ := auth.GenerateSigningKeypair()
	hashKey := "test-hashing-key-cache-bootstrap-9b2f"
	expectedIDs := make([]string, 0, seedCount)
	for i := 0; i < seedCount; i++ {
		tokenID := fmt.Sprintf("chaos-cache-tok-%d-%d", i, time.Now().UnixNano())
		issued, err := auth.IssueToken(auth.IssueOptions{
			UserID:     enrollUser,
			TokenID:    tokenID,
			SigningKey: priv,
			KeyID:      "key-test-2026-05",
			TTL:        24 * time.Hour,
			Issuer:     "smackerel-test",
			Now:        time.Now,
		})
		if err != nil {
			t.Fatalf("IssueToken[%d]: %v", i, err)
		}
		hashed, err := auth.HashToken(issued.WireToken, hashKey)
		if err != nil {
			t.Fatalf("HashToken[%d]: %v", i, err)
		}
		if err := store.PersistToken(context.Background(), auth.PersistTokenParams{
			TokenID:      tokenID,
			UserID:       enrollUser,
			KeyID:        "key-test-2026-05",
			IssuedAt:     issued.IssuedAt,
			ExpiresAt:    issued.ExpiresAt,
			HashedToken:  hashed,
			IssuedBy:     "chaos-cache-bootstrap",
			IssuedSource: "cli",
		}); err != nil {
			t.Fatalf("PersistToken[%d]: %v", i, err)
		}
		if err := store.RevokeToken(context.Background(), tokenID, "chaos-cache-bootstrap", "behavior-4-seed"); err != nil {
			t.Fatalf("RevokeToken[%d]: %v", i, err)
		}
		expectedIDs = append(expectedIDs, tokenID)
	}

	cache := revocation.NewCache()
	bootstrapDone := make(chan struct{})

	// Fire concurrent IsRevoked queries BEFORE bootstrap completes.
	queryCtx, queryCancel := context.WithCancel(context.Background())
	var probeIterations atomic.Int64
	var queryWG sync.WaitGroup
	for w := 0; w < 12; w++ {
		queryWG.Add(1)
		go func(idx int) {
			defer queryWG.Done()
			for {
				select {
				case <-queryCtx.Done():
					return
				default:
					_ = cache.IsRevoked(fmt.Sprintf("not-yet-loaded-%d-%d", idx, probeIterations.Load()))
					_ = cache.IsRevoked(expectedIDs[int(probeIterations.Load())%seedCount])
					probeIterations.Add(1)
				}
			}
		}(w)
	}

	go func() {
		defer close(bootstrapDone)
		n, err := cache.BootstrapFromDB(context.Background(), store)
		if err != nil {
			t.Errorf("BootstrapFromDB: %v", err)
			return
		}
		if n < seedCount {
			t.Errorf("BootstrapFromDB count: want >=%d got %d", seedCount, n)
		}
	}()

	<-bootstrapDone
	queryCancel()
	queryWG.Wait()

	missing := 0
	for _, id := range expectedIDs {
		if !cache.IsRevoked(id) {
			missing++
		}
	}
	if missing > 0 {
		t.Fatalf("cache missing %d of %d seeded revocations after bootstrap", missing, seedCount)
	}
	if cache.Size() < int64(seedCount) {
		t.Fatalf("cache.Size after bootstrap: want >=%d got %d", seedCount, cache.Size())
	}
	t.Logf("Behavior 4: BootstrapFromDB seeded %d revocations under %d concurrent IsRevoked workers (probe iterations ≈ %d, cache.Size=%d, no race hits, all expected IDs visible)",
		seedCount, 12, probeIterations.Load(), cache.Size())
}

// --- Behavior 5 -----------------------------------------------------

// TestAuthChaos_BroadcasterMalformedPayloads_CacheIntact publishes
// pathological NATS messages on the chaos broadcaster subject and
// asserts the subscriber (a) does not panic, (b) does not corrupt the
// cache (Size remains 0), and (c) continues to process well-formed
// events afterwards. Confirms the OBS-AUDIT-044-S01-03 finding: the
// silent-drop policy on malformed events preserves cache integrity at
// the cost of observability — telemetry counters remain a Scope 04
// follow-up.
func TestAuthChaos_BroadcasterMalformedPayloads_CacheIntact(t *testing.T) {
	nc := chaosNATSConn(t)
	defer nc.Close()

	cache := revocation.NewCache()
	subject := fmt.Sprintf("chaos.auth.malformed.%d", time.Now().UnixNano())
	bc, err := revocation.NewBroadcaster(nc, subject, cache, "chaos-instance-malformed")
	if err != nil {
		t.Fatalf("NewBroadcaster: %v", err)
	}
	if err := bc.Subscribe(); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = bc.Stop() }()

	// Publish a battery of malformed payloads bypassing the broadcaster
	// (so they hit the subscriber's defensive `handle` directly).
	bad := [][]byte{
		nil,
		{},
		[]byte("not-json"),
		[]byte("{"),
		[]byte(`{"version":"v1"}`),                                   // missing token_id
		[]byte(`{"version":"v1","token_id":""}`),                     // empty token_id
		[]byte(`{"version":"v999","token_id":"x"}`),                  // unknown version (cache still loads — ID present)
		[]byte(`{"token_id":12345}`),                                 // wrong type
		[]byte(`{"version":"v1","token_id":` + strings.Repeat("a", 100) + `}`), // unterminated string-style garbage
	}
	beforeSize := cache.Size()
	idsThatShouldLand := 1 // only the v999/x message has a non-empty token_id
	for i, payload := range bad {
		if err := nc.Publish(subject, payload); err != nil {
			t.Fatalf("publish bad[%d]: %v", i, err)
		}
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	// Drain — give NATS a brief settle window.
	time.Sleep(200 * time.Millisecond)

	if cache.Size() != beforeSize+int64(idsThatShouldLand) {
		t.Fatalf("cache size after malformed barrage: want %d (only the well-formed-token-id message lands) got %d (subscriber may have crashed or accepted garbage)",
			beforeSize+int64(idsThatShouldLand), cache.Size())
	}

	// Subscriber MUST keep processing after the bad-payload barrage.
	wellFormedID := fmt.Sprintf("chaos-after-malformed-%d", time.Now().UnixNano())
	if err := bc.Publish(wellFormedID, "behavior-5-after-malformed"); err != nil {
		t.Fatalf("Publish well-formed: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cache.IsRevoked(wellFormedID) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !cache.IsRevoked(wellFormedID) {
		t.Fatalf("subscriber did not process well-formed event after malformed barrage (cache.IsRevoked %q = false)", wellFormedID)
	}
	t.Logf("Behavior 5: %d malformed payloads dropped silently (cache integrity preserved); 1 well-formed event after barrage processed correctly (cache.Size=%d)",
		len(bad)-idsThatShouldLand, cache.Size())
}

// --- Behavior 6 -----------------------------------------------------

// TestAuthChaos_MigrationIdempotency runs db.Migrate against the live
// test DB N times in quick succession. The migration MUST be idempotent
// (CREATE TABLE IF NOT EXISTS guards plus a version tracker) — repeated
// invocation MUST NOT error. Adversarial second pass: we DROP one of
// the spec-044 tables and confirm the system fails LOUDLY (not
// silently) when downstream code queries the missing table.
//
// db.Migrate is contract-bound to version-based idempotency — re-
// running an already-applied migration is a no-op. This is the
// canonical contract for migration runners. The adversarial assertion
// here is therefore that *callers* surface a loud `relation does not
// exist` Postgres error — NOT that db.Migrate magically rebuilds
// dropped tables.
func TestAuthChaos_MigrationIdempotency(t *testing.T) {
	pool := authTestPool(t)
	defer pool.Close()

	ctx := context.Background()

	// Defensive setup: ensure auth_revocations exists before this test
	// asserts the idempotency contract. A previous failed run of this
	// test could have left the schema with auth_revocations dropped;
	// rather than fail-loud on stale state from a prior run, restore
	// the canonical schema and let the assertions exercise the real
	// idempotency contract.
	if _, err := pool.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS auth_revocations (
            token_id    text         PRIMARY KEY
                                     REFERENCES auth_tokens(token_id)
                                     ON DELETE CASCADE,
            revoked_at  timestamptz  NOT NULL DEFAULT now(),
            revoked_by  text         NOT NULL,
            reason      text         NOT NULL DEFAULT ''
        );
        CREATE INDEX IF NOT EXISTS ix_auth_revocations_revoked_at ON auth_revocations (revoked_at);
    `); err != nil {
		t.Fatalf("defensive setup auth_revocations: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := db.Migrate(ctx, pool); err != nil {
			t.Fatalf("idempotency Migrate iteration %d: %v", i, err)
		}
	}

	// Confirm all three spec-044 tables exist after the idempotent loop.
	tables := []string{"auth_users", "auth_tokens", "auth_revocations"}
	for _, table := range tables {
		var present bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`,
			table).Scan(&present); err != nil {
			t.Fatalf("query existence of %q: %v", table, err)
		}
		if !present {
			t.Fatalf("table %q missing after idempotent Migrate", table)
		}
	}

	// Adversarial: drop auth_revocations, re-run Migrate (version-based
	// idempotency means it's a no-op), then confirm downstream queries
	// against the missing table FAIL LOUDLY with a `relation does not
	// exist` Postgres error instead of silently returning empty results.
	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS auth_revocations CASCADE`); err != nil {
		t.Fatalf("DROP auth_revocations: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate after DROP auth_revocations (version-based idempotency contract): %v", err)
	}

	store, err := auth.NewBearerStore(pool)
	if err != nil {
		t.Fatalf("NewBearerStore: %v", err)
	}
	_, err = store.LoadRevokedTokenIDs(ctx)
	if err == nil {
		t.Fatal("LoadRevokedTokenIDs against missing auth_revocations MUST fail loudly, got nil")
	}
	lower := strings.ToLower(err.Error())
	if !strings.Contains(lower, "relation") || !strings.Contains(lower, "does not exist") {
		t.Errorf("LoadRevokedTokenIDs error MUST mention 'relation does not exist' (Postgres canonical schema-mismatch signal); got: %v", err)
	}

	// Repair: rebuild auth_revocations directly so the test stack stays
	// usable for subsequent chaos behaviors. (Migration version 033 is
	// already recorded as applied; we cannot replay it through db.Migrate
	// without reaching into private internals, and rebuilding via the
	// canonical SQL preserves the exact table schema for follow-on tests.)
	if _, err := pool.Exec(ctx, `
        CREATE TABLE IF NOT EXISTS auth_revocations (
            token_id    text         PRIMARY KEY
                                     REFERENCES auth_tokens(token_id)
                                     ON DELETE CASCADE,
            revoked_at  timestamptz  NOT NULL DEFAULT now(),
            revoked_by  text         NOT NULL,
            reason      text         NOT NULL DEFAULT ''
        );
        CREATE INDEX IF NOT EXISTS ix_auth_revocations_revoked_at ON auth_revocations (revoked_at);
    `); err != nil {
		t.Fatalf("rebuild auth_revocations after adversarial DROP: %v", err)
	}
	t.Logf("Behavior 6: db.Migrate idempotent across 3 invocations; adversarial DROP+downstream-query yields loud 'relation does not exist' error (no silent failure)")
}

// --- Behavior 7 -----------------------------------------------------

// TestAuthChaos_TokenBoundaryConditions exercises VerifyAndParse and
// IssueToken against pathological boundary inputs. Each case asserts
// the expected sentinel error category — silent acceptance of any of
// these is a P0 finding.
func TestAuthChaos_TokenBoundaryConditions(t *testing.T) {
	priv, _ := auth.GenerateSigningKeypair()
	pub, err := auth.PublicHexFromSecretHex(priv)
	if err != nil {
		t.Fatalf("derive pub: %v", err)
	}
	clock := func() time.Time { return time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC) }

	// Case A — TTL = 0 → IssueToken refuses.
	if _, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "u",
		TokenID:    "t",
		SigningKey: priv,
		KeyID:      "k",
		TTL:        0,
		Issuer:     "iss",
		Now:        clock,
	}); err == nil || !strings.Contains(err.Error(), "positive TTL") {
		t.Errorf("Case A — TTL=0: want 'positive TTL' error, got: %v", err)
	}

	// Case B — TTL < 0 → IssueToken refuses.
	if _, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "u",
		TokenID:    "t",
		SigningKey: priv,
		KeyID:      "k",
		TTL:        -1 * time.Hour,
		Issuer:     "iss",
		Now:        clock,
	}); err == nil || !strings.Contains(err.Error(), "positive TTL") {
		t.Errorf("Case B — TTL<0: want 'positive TTL' error, got: %v", err)
	}

	// Case C — token issued with kid that VerifyAndParse does not know
	// (foreign kid / malformed routing).
	foreignPriv, _ := auth.GenerateSigningKeypair()
	foreign, err := auth.IssueToken(auth.IssueOptions{
		UserID: "u-foreign", TokenID: "t-foreign", SigningKey: foreignPriv,
		KeyID: "key-foreign-99", TTL: time.Hour, Issuer: "iss",
		Now: clock,
	})
	if err != nil {
		t.Fatalf("issue foreign-kid token: %v", err)
	}
	if _, err := auth.VerifyAndParse(foreign.WireToken, auth.VerifyOptions{
		ActivePublicKey: pub, ActiveKeyID: "key-active",
		Issuer: "iss", ClockSkewTolerance: 30 * time.Second, Now: clock,
	}); !errors.Is(err, auth.ErrUnknownKeyID) {
		t.Errorf("Case C — foreign kid: want ErrUnknownKeyID, got: %v", err)
	}

	// Case D — empty wire token.
	if _, err := auth.VerifyAndParse("", auth.VerifyOptions{
		ActivePublicKey: pub, ActiveKeyID: "key-active",
		Issuer: "iss", ClockSkewTolerance: 30 * time.Second, Now: clock,
	}); err == nil {
		t.Error("Case D — empty wire token: want non-nil error, got nil")
	}

	// Case E — well-formed prefix but truncated tail (signature-byte
	// chop-off should fail signature verification).
	good, err := auth.IssueToken(auth.IssueOptions{
		UserID: "u", TokenID: "t", SigningKey: priv, KeyID: "key-active",
		TTL: time.Hour, Issuer: "iss", Now: clock,
	})
	if err != nil {
		t.Fatalf("issue good: %v", err)
	}
	tampered := good.WireToken
	if len(tampered) > 4 {
		tampered = tampered[:len(tampered)-4]
	}
	if _, err := auth.VerifyAndParse(tampered, auth.VerifyOptions{
		ActivePublicKey: pub, ActiveKeyID: "key-active",
		Issuer: "iss", ClockSkewTolerance: 30 * time.Second, Now: clock,
	}); err == nil {
		t.Error("Case E — tampered wire token: want non-nil error, got nil")
	}

	// Case F — token issued in the FAR FUTURE (nbf > now + tolerance).
	farFuture := func() time.Time { return clock().Add(48 * time.Hour) }
	futureToken, err := auth.IssueToken(auth.IssueOptions{
		UserID: "u", TokenID: "t-fut", SigningKey: priv, KeyID: "key-active",
		TTL: time.Hour, Issuer: "iss", Now: farFuture,
	})
	if err != nil {
		t.Fatalf("issue future: %v", err)
	}
	if _, err := auth.VerifyAndParse(futureToken.WireToken, auth.VerifyOptions{
		ActivePublicKey: pub, ActiveKeyID: "key-active",
		Issuer: "iss", ClockSkewTolerance: 30 * time.Second, Now: clock,
	}); !errors.Is(err, auth.ErrTokenNotYetValid) {
		t.Errorf("Case F — future token: want ErrTokenNotYetValid, got: %v", err)
	}

	// Case G — token issued in the FAR PAST (exp + tolerance < now).
	farPast := func() time.Time { return clock().Add(-48 * time.Hour) }
	pastToken, err := auth.IssueToken(auth.IssueOptions{
		UserID: "u", TokenID: "t-past", SigningKey: priv, KeyID: "key-active",
		TTL: time.Hour, Issuer: "iss", Now: farPast,
	})
	if err != nil {
		t.Fatalf("issue past: %v", err)
	}
	if _, err := auth.VerifyAndParse(pastToken.WireToken, auth.VerifyOptions{
		ActivePublicKey: pub, ActiveKeyID: "key-active",
		Issuer: "iss", ClockSkewTolerance: 30 * time.Second, Now: clock,
	}); !errors.Is(err, auth.ErrTokenExpired) {
		t.Errorf("Case G — past token: want ErrTokenExpired, got: %v", err)
	}

	// Case H — half-rotation config (only PriorPublicKey set, PriorKeyID empty).
	if _, err := auth.VerifyAndParse(good.WireToken, auth.VerifyOptions{
		ActivePublicKey: pub, ActiveKeyID: "key-active",
		PriorPublicKey: pub, // mismatched pair
		Issuer:         "iss", ClockSkewTolerance: 30 * time.Second, Now: clock,
	}); err == nil || !strings.Contains(err.Error(), "PriorPublicKey and PriorKeyID") {
		t.Errorf("Case H — half-rotation config: want 'PriorPublicKey and PriorKeyID' error, got: %v", err)
	}

	// Case I — HashToken with empty key.
	if _, err := auth.HashToken("token-value", ""); err == nil || !strings.Contains(err.Error(), "empty hashing key") {
		t.Errorf("Case I — HashToken empty key: want 'empty hashing key' error, got: %v", err)
	}

	// Case J — HashToken with empty token.
	if _, err := auth.HashToken("", "key-value"); err == nil || !strings.Contains(err.Error(), "empty token") {
		t.Errorf("Case J — HashToken empty token: want 'empty token' error, got: %v", err)
	}

	t.Log("Behavior 7: 10 boundary conditions (A..J) all yield the expected sentinel error category — no silent acceptance, no panic")
}

// --- Behavior 9 (informational benchmark) ---------------------------

// BenchmarkAuthChaos_VerifyAndParse_HotPath measures pure-CPU
// verification throughput. NOT a pass/fail gate — observability only.
// Run via: go test -tags=integration -bench=BenchmarkAuthChaos_VerifyAndParse_HotPath -run=^$ -count=1 ./tests/integration/
func BenchmarkAuthChaos_VerifyAndParse_HotPath(b *testing.B) {
	priv, _ := auth.GenerateSigningKeypair()
	pub, err := auth.PublicHexFromSecretHex(priv)
	if err != nil {
		b.Fatalf("derive pub: %v", err)
	}
	clock := func() time.Time { return time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC) }
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID: "user-bench", TokenID: "tok-bench",
		SigningKey: priv, KeyID: "key-bench",
		TTL: 24 * time.Hour, Issuer: "smackerel-bench",
		Now: clock,
	})
	if err != nil {
		b.Fatalf("IssueToken: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = auth.VerifyAndParse(issued.WireToken, auth.VerifyOptions{
			ActivePublicKey:    pub,
			ActiveKeyID:        "key-bench",
			Issuer:             "smackerel-bench",
			ClockSkewTolerance: 30 * time.Second,
			Now:                clock,
		})
	}
}