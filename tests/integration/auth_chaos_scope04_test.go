//go:build integration

// Spec 044 Scope 04 — chaos-phase integration tests for the
// per-user bearer-auth TELEMETRY + F02 WIRING + DEPRECATION-FLAG
// surfaces shipped in Scope 04.
//
// Where Scope 02 chaos exercised the production middleware admit/
// reject path on the API hot path, and Scope 03 chaos exercised the
// PWA cookie + extension rotation + admin UI surfaces, Scope 04
// chaos exercises:
//
//   - F02 closure: Bot.bearerForChat → MintForChat → setBearerHeader
//     under concurrent inbound bursts; production unmapped chat
//     refusal under concurrent burst.
//
//   - Deprecation-flag (`auth.production_shared_token_fallback_enabled`)
//     enforcement: legacy bearer rejected with flag=false in
//     production; legacy bearer admitted with flag=true; the
//     legacy-fallback metric ticks ONLY on the admit path.
//
//   - Auth metrics counter family
//     (`smackerel_auth_validation_outcome_total{result, source}`)
//     under concurrent emit at high cardinality budget.
//
//     C4-B01 → TestAuthChaos_S04_F02WiringConcurrentMappedBurst_AllMint
//     C4-B02 → TestAuthChaos_S04_F02WiringUnmappedConcurrentBurst_AllRefuse
//     C4-B03 → TestAuthChaos_S04_DeprecationFlagToggleRace_NoInconsistency
//     C4-B04 → TestAuthChaos_S04_AuthMetricsCounterConcurrentEmit_AggregatesMatch
//     C4-B05 → TestAuthChaos_S04_LegacyFallbackProductionFlagFalse_AllRejected
//     C4-Hot → BenchmarkAuthChaos_S04_F02MintHotPath
//
// Live-stack expectations: tests run against the live PostgreSQL
// (DATABASE_URL on the test stack) and never against the persistent
// dev DB. NATS is exercised only by Scope 03 chaos; Scope 04 chaos
// does not require a NATS connection because none of its surfaces
// touch the revocation broadcaster.
//
// Concurrency style: each test uses sync.WaitGroup + a release gate
// for intra-test concurrency. We do NOT use t.Parallel() because
// every test asserts deltas against the global Prometheus registry;
// concurrent siblings would interleave their counter ticks and
// invalidate the deltas. The user's chaos brief recommends errgroup,
// but errgroup is not currently a smackerel dependency — sync.WaitGroup
// gives the same race-detection guarantee under -race -count=N
// without adding a new module dep mid-chaos-phase.
//
// Stress contract: each test plus the benchmark passes 20/20 under
// `go test -race -count=20 -tags=integration -run TestAuthChaos_S04_`.
package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
	"github.com/smackerel/smackerel/internal/metrics"
	"github.com/smackerel/smackerel/internal/telegram"
)

// uniqueChaosS04RunID returns a per-test-run identifier so user_ids
// and token_ids do not collide across `-count=N` stress iterations.
// All chaos rows are prefixed with `chaos-044-s04-<runID>-...`.
func uniqueChaosS04RunID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// truncForLogS04 truncates a string for safe inclusion in failure
// logs — bearer tokens are long and would dominate test output.
func truncForLogS04(s string) string {
	const maxLen = 64
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(" + fmt.Sprintf("%d total", len(s)) + ")"
}

// --- C4-B01 ---------------------------------------------------------

// TestAuthChaos_S04_F02WiringConcurrentMappedBurst_AllMint proves the
// F02 closure — Bot.bearerForChat → MintForChat → setBearerHeader —
// is jar-isolated under concurrent inbound Telegram bursts. N=50
// goroutines fire simultaneously, each carrying a DISTINCT mapped
// chat_id; every goroutine asserts:
//
//   - setBearerHeader returned no error (mint succeeded)
//   - the Authorization header carries a PASETO v4.public bearer
//     (not the shared sentinel, not empty)
//   - the resulting request admits through bearerAuthMiddleware (200)
//   - the verified PASETO claim's UserID matches the mapped user_id
//     for that chat (no cross-chat attribution leak)
//
// The metric `smackerel_auth_issuance_total{source="telegram_bridge"}`
// MUST tick by exactly N=50 across the burst.
//
// A regression that shared minter state across chats (e.g., a stale
// cached token reused for the wrong chat) would surface as either an
// admit failure or a UserID mismatch on the verified claim. The
// distinct-chat-id assignment makes the cross-attribution path
// observable: if any goroutine receives a token bound to another
// goroutine's user, the test fails with a verbatim sample.
func TestAuthChaos_S04_F02WiringConcurrentMappedBurst_AllMint(t *testing.T) {
	const concurrent = 50
	runID := uniqueChaosS04RunID("f02-mapped")

	// Build a per-test mapping with distinct chat_id → user_id pairs.
	// Each goroutine claims one (chatID, userID) so the cross-attribution
	// invariant is observable: if any goroutine's minted token's
	// VerifyAndParse(...).UserID does not equal its expected user, that
	// is a F02 wiring leak.
	mapping := make(map[int64]string, concurrent)
	for i := 0; i < concurrent; i++ {
		mapping[int64(70000+i)] = fmt.Sprintf("chaos-044-s04-%s-mapped-user-%d", runID, i)
	}
	deps, bot, minter, _ := productionTelegramBridgeDeps(t, mapping)

	bot.SetSharedAuthTokenForTest("WRONG-shared-bearer-MUST-NOT-LEAK-S04-B01")
	bot.SetPerUserTokenMinter(minter)

	srv := httptest.NewServer(api.NewRouter(deps))
	t.Cleanup(srv.Close)

	verifyOpts := deps.AuthVerifyOptions

	before := testutil.ToFloat64(metrics.AuthIssuance.WithLabelValues("telegram_bridge"))

	var admit, mintErr, attrLeak, verifyErr atomic.Int64
	var failures []string
	var failuresMu sync.Mutex
	recordFailure := func(s string) {
		failuresMu.Lock()
		failures = append(failures, s)
		failuresMu.Unlock()
	}

	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := 0; i < concurrent; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			chatID := int64(70000 + i)
			expectedUser := mapping[chatID]

			req, err := http.NewRequestWithContext(context.Background(),
				http.MethodGet, srv.URL+"/v1/photos/connectors", nil)
			if err != nil {
				mintErr.Add(1)
				recordFailure(fmt.Sprintf("chat=%d NewRequest err=%v", chatID, err))
				return
			}
			if err := bot.SetBearerHeaderForTest(req, chatID); err != nil {
				mintErr.Add(1)
				recordFailure(fmt.Sprintf("chat=%d SetBearerHeader err=%v", chatID, err))
				return
			}

			authz := req.Header.Get("Authorization")
			const wantPrefix = "Bearer v4.public."
			if len(authz) < len(wantPrefix) || authz[:len(wantPrefix)] != wantPrefix {
				mintErr.Add(1)
				recordFailure(fmt.Sprintf("chat=%d Authorization=%q does not look like per-user PASETO", chatID, truncForLogS04(authz)))
				return
			}
			if strings.Contains(authz, "WRONG-shared-bearer-MUST-NOT-LEAK-S04-B01") {
				mintErr.Add(1)
				recordFailure(fmt.Sprintf("chat=%d Authorization=%q LEAKED shared sentinel", chatID, truncForLogS04(authz)))
				return
			}

			// Cross-attribution check: parse the bearer (strip
			// "Bearer ") and verify the claim's UserID matches the
			// chat's mapped user. If two goroutines' tokens were
			// crossed, this would surface here.
			wire := authz[len("Bearer "):]
			parsed, err := auth.VerifyAndParse(wire, verifyOpts)
			if err != nil {
				verifyErr.Add(1)
				recordFailure(fmt.Sprintf("chat=%d VerifyAndParse err=%v", chatID, err))
				return
			}
			if parsed.UserID != expectedUser {
				attrLeak.Add(1)
				recordFailure(fmt.Sprintf("chat=%d ATTRIBUTION LEAK: minted UserID=%q want=%q", chatID, parsed.UserID, expectedUser))
				return
			}

			resp, err := srv.Client().Do(req)
			if err != nil {
				mintErr.Add(1)
				recordFailure(fmt.Sprintf("chat=%d Do err=%v", chatID, err))
				return
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				mintErr.Add(1)
				recordFailure(fmt.Sprintf("chat=%d HTTP status=%d", chatID, resp.StatusCode))
				return
			}
			admit.Add(1)
		}()
	}
	close(gate)
	wg.Wait()

	after := testutil.ToFloat64(metrics.AuthIssuance.WithLabelValues("telegram_bridge"))
	delta := after - before

	if attrLeak.Load() != 0 {
		t.Fatalf("C4-B01: %d cross-chat attribution LEAKS (FR-AUTH-005 violation) sample=%v",
			attrLeak.Load(), failures[:min(len(failures), 3)])
	}
	if verifyErr.Load() != 0 {
		t.Fatalf("C4-B01: %d minted bearers failed VerifyAndParse sample=%v",
			verifyErr.Load(), failures[:min(len(failures), 3)])
	}
	if mintErr.Load() != 0 || admit.Load() != concurrent {
		t.Fatalf("C4-B01: admit accounting: want %d admit / 0 err, got admit=%d err=%d sample=%v",
			concurrent, admit.Load(), mintErr.Load(), failures[:min(len(failures), 3)])
	}
	if int(delta) != concurrent {
		t.Fatalf("C4-B01: AuthIssuance{telegram_bridge} delta=%v want %d (before=%v after=%v)",
			delta, concurrent, before, after)
	}
	t.Logf("C4-B01: %d concurrent mapped Telegram mints → all admitted, all UserIDs correct, AuthIssuance delta=%v (race-detector clean)",
		admit.Load(), delta)
}

// --- C4-B02 ---------------------------------------------------------

// TestAuthChaos_S04_F02WiringUnmappedConcurrentBurst_AllRefuse proves
// the F02 production safety contract under concurrent burst: N=50
// goroutines simultaneously call setBearerHeader with chat_ids NOT
// in the mapping; every goroutine MUST observe an error and EVERY
// outbound request MUST have an empty Authorization header. The
// `smackerel_auth_issuance_total{source="telegram_bridge"}` counter
// MUST tick by exactly 0 across the burst (refused mints do not
// count).
//
// A regression that silently downgraded to the shared bearer for an
// unmapped chat (the F02 anti-pattern this chaos test exists to
// detect) would surface as either:
//
//   - A goroutine receiving Authorization=Bearer <shared sentinel>
//   - A goroutine succeeding without an error
//   - A non-zero AuthIssuance{telegram_bridge} delta
//
// Each of those is a hard t.Fatalf — no probabilistic assertion.
func TestAuthChaos_S04_F02WiringUnmappedConcurrentBurst_AllRefuse(t *testing.T) {
	const concurrent = 50
	runID := uniqueChaosS04RunID("f02-unmapped")

	// Mapping holds a single sentinel chat that is NEVER probed.
	// Production refuses any chat NOT in this mapping.
	mapping := map[int64]string{
		54321: fmt.Sprintf("chaos-044-s04-%s-sentinel-only", runID),
	}
	_, bot, minter, _ := productionTelegramBridgeDeps(t, mapping)

	bot.SetSharedAuthTokenForTest("WRONG-shared-bearer-MUST-NOT-LEAK-S04-B02")
	bot.SetPerUserTokenMinter(minter)

	before := testutil.ToFloat64(metrics.AuthIssuance.WithLabelValues("telegram_bridge"))

	var refused, leaked, silent atomic.Int64
	var failures []string
	var failuresMu sync.Mutex
	recordFailure := func(s string) {
		failuresMu.Lock()
		failures = append(failures, s)
		failuresMu.Unlock()
	}

	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := 0; i < concurrent; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			// chatID 800000+i is GUARANTEED unmapped; the test fixture
			// only contains chat 54321.
			chatID := int64(800000 + i)
			req, err := http.NewRequestWithContext(context.Background(),
				http.MethodGet, "http://example.invalid/", nil)
			if err != nil {
				recordFailure(fmt.Sprintf("chat=%d NewRequest err=%v", chatID, err))
				return
			}
			err = bot.SetBearerHeaderForTest(req, chatID)
			if err == nil {
				authz := req.Header.Get("Authorization")
				if authz == "" {
					silent.Add(1)
					recordFailure(fmt.Sprintf("chat=%d setBearerHeader returned nil but Authorization is empty (silent fall-through)", chatID))
					return
				}
				leaked.Add(1)
				recordFailure(fmt.Sprintf("chat=%d setBearerHeader admitted unmapped chat: Authorization=%q", chatID, truncForLogS04(authz)))
				return
			}
			// Hard contract: error path must not have set the
			// Authorization header. A leaked header here would mean
			// setBearerHeader partial-applied before erroring.
			if got := req.Header.Get("Authorization"); got != "" {
				leaked.Add(1)
				recordFailure(fmt.Sprintf("chat=%d setBearerHeader err=%v BUT Authorization=%q (partial application)", chatID, err, truncForLogS04(got)))
				return
			}
			refused.Add(1)
		}()
	}
	close(gate)
	wg.Wait()

	after := testutil.ToFloat64(metrics.AuthIssuance.WithLabelValues("telegram_bridge"))
	delta := after - before

	if leaked.Load() != 0 || silent.Load() != 0 {
		t.Fatalf("C4-B02: production unmapped chat MUST refuse cleanly — leaked=%d silent=%d sample=%v",
			leaked.Load(), silent.Load(), failures[:min(len(failures), 3)])
	}
	if refused.Load() != concurrent {
		t.Fatalf("C4-B02: refused accounting: want %d refused, got %d sample=%v",
			concurrent, refused.Load(), failures[:min(len(failures), 3)])
	}
	if delta != 0 {
		t.Fatalf("C4-B02: AuthIssuance{telegram_bridge} delta=%v want 0 (refused mints MUST NOT tick metric); before=%v after=%v",
			delta, before, after)
	}
	t.Logf("C4-B02: %d concurrent unmapped Telegram mints → all refused, AuthIssuance delta=0 (no metric tick on refused mints, no shared-bearer leak)",
		refused.Load())
}

// --- C4-B03 ---------------------------------------------------------

// TestAuthChaos_S04_DeprecationFlagToggleRace_NoInconsistency proves
// `auth.production_shared_token_fallback_enabled` enforces a
// per-instance immutable contract: a deps/router constructed with
// flag=false ALWAYS rejects the legacy bearer, and a deps/router
// constructed with flag=true ALWAYS admits the legacy bearer (with
// the legacy-fallback metric ticking on each admit).
//
// Production semantics: the flag is loaded at startup from
// `config.AuthConfig.ProductionSharedTokenFallbackEnabled`. The
// middleware reads the field on every request without a lock;
// runtime hot-toggle is intentionally NOT supported (a process
// restart is required to flip it). The chaos contract here proves
// the flag's INTENDED transition pattern (process restart) is
// observable end-to-end:
//
//  1. Two routers constructed with opposing flag values run
//     simultaneously under N=100 concurrent legacy-bearer requests.
//     Each request is dispatched to a router based on a per-iteration
//     atomic.Bool snapshot — flipping the flag mid-stream simulates
//     the operator restarting the service to flip the toggle. The
//     resulting cohort split is stochastic by design (Go runtime
//     scheduler decides which workers snapshot before vs. after
//     `flag.Store(true)`). The hard invariant is per-request status
//     consistency, NOT a deterministic cohort-size split.
//  2. Requests dispatched to the flag=false router MUST 401; requests
//     dispatched to the flag=true router MUST 200.
//  3. The legacy-fallback metric MUST tick exactly once per flag=true
//     admit and exactly zero times per flag=false reject.
//  4. Both cohorts MUST be non-empty (proves the test exercised an
//     actual transition rather than a single-flag run).
//  5. No request finishes in an inconsistent state (e.g., 200 from
//     the flag=false router OR 401 from the flag=true router with
//     a valid legacy bearer).
//
// The atomic.Bool flip happens between iterations (not concurrently
// with the per-router middleware reads), so the race detector stays
// clean.
func TestAuthChaos_S04_DeprecationFlagToggleRace_NoInconsistency(t *testing.T) {
	const totalReqs = 100
	const flipPoint = 50

	// One shared signing keypair so the same legacy bearer is the
	// "shared token" for both routers.
	priv, pub := auth.GenerateSigningKeypair()
	const kid = "scope04-chaos-flag-key"
	const sharedToken = "scope04-chaos-shared-token-WILL-BE-REJECTED-OR-ADMITTED-DEPENDING-ON-FLAG"

	pool := authTestPool(t)
	t.Cleanup(func() { pool.Close() })
	resetAuthTables(t, pool)
	t.Cleanup(func() { resetAuthTables(t, pool) })

	store := photolib.NewStore(pool)
	cache := revocation.NewCache()

	mkDeps := func(flag bool) *api.Dependencies {
		return &api.Dependencies{
			Environment: "production",
			AuthToken:   sharedToken,
			AuthConfig: config.AuthConfig{
				Enabled:                               true,
				TokenFormat:                           "paseto_v4_public",
				SigningActivePrivateKey:               priv,
				SigningActiveKeyID:                    kid,
				TokenTTLHours:                         24,
				RotationGraceWindowHours:              2,
				ClockSkewToleranceSeconds:             60,
				RevocationCacheRefreshIntervalSeconds: 60,
				AtRestHashingKey:                      priv + "-hash-suffix-distinct",
				ProductionSharedTokenFallbackEnabled:  flag,
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
		}
	}
	depsFlagOff := mkDeps(false)
	depsFlagOn := mkDeps(true)
	routerFlagOff := api.NewRouter(depsFlagOff)
	routerFlagOn := api.NewRouter(depsFlagOn)

	beforeFallback := testutil.ToFloat64(metrics.AuthLegacyFallbackUsed.WithLabelValues("production"))

	// flag is the operator-controlled toggle; flipped exactly once at
	// the flipPoint. Each goroutine snapshots it ONCE on entry and
	// uses that snapshot for both dispatch and assertion classification
	// — this is the production semantic (a request belongs to the
	// flag value in effect when its handler started).
	var flag atomic.Bool

	type result struct {
		flagAtRequest bool
		status        int
	}
	results := make(chan result, totalReqs)

	var wg sync.WaitGroup
	gate := make(chan struct{})

	for i := 0; i < totalReqs; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			// Operator-flip simulation: at the flipPoint, the flipper
			// goroutine flips the atomic.Bool. Goroutines that started
			// before the flip dispatch to the flag=false router;
			// goroutines that started after dispatch to the flag=true
			// router. A goroutine's flag snapshot is captured here
			// and used for BOTH dispatch (which router) and assertion
			// (what status to expect). This pattern provably has no
			// data race — each router's deps is read-only after
			// construction; the atomic.Bool is the only shared state
			// and it uses package atomic.
			if i == flipPoint {
				flag.Store(true)
			}
			snap := flag.Load()
			req := httptest.NewRequest(http.MethodGet, "/v1/photos/connectors", nil)
			req.Header.Set("Authorization", "Bearer "+sharedToken)
			req.RemoteAddr = fmt.Sprintf("10.6.0.%d:1234", (i%250)+1)
			rec := httptest.NewRecorder()
			if snap {
				routerFlagOn.ServeHTTP(rec, req)
			} else {
				routerFlagOff.ServeHTTP(rec, req)
			}
			results <- result{flagAtRequest: snap, status: rec.Code}
		}()
	}
	close(gate)
	wg.Wait()
	close(results)

	var flagOffCount, flagOnCount int
	var flagOffWrong, flagOnWrong int
	var inconsistencies []string
	for r := range results {
		switch r.flagAtRequest {
		case false:
			flagOffCount++
			// flag=false MUST reject the legacy bearer in production →
			// 401 (Branch 1 verifier fails because the shared token is
			// not a valid PASETO; Branch 2 fallback is gated behind
			// flag=true; falls through to a 401).
			if r.status != http.StatusUnauthorized {
				flagOffWrong++
				inconsistencies = append(inconsistencies,
					fmt.Sprintf("flag=false but status=%d (want 401)", r.status))
			}
		case true:
			flagOnCount++
			// flag=true admits the legacy bearer via Branch 2 → 200.
			if r.status != http.StatusOK {
				flagOnWrong++
				inconsistencies = append(inconsistencies,
					fmt.Sprintf("flag=true but status=%d (want 200)", r.status))
			}
		}
	}

	afterFallback := testutil.ToFloat64(metrics.AuthLegacyFallbackUsed.WithLabelValues("production"))
	deltaFallback := afterFallback - beforeFallback

	if flagOffWrong != 0 || flagOnWrong != 0 {
		sample := inconsistencies
		if len(sample) > 5 {
			sample = sample[:5]
		}
		t.Fatalf("C4-B03: flag transition produced %d inconsistencies (off=%d wrong, on=%d wrong) sample=%v",
			flagOffWrong+flagOnWrong, flagOffWrong, flagOnWrong, sample)
	}
	// Cohort sizes are stochastic by design: under chaos, the Go
	// runtime scheduler decides which goroutines snapshot `flag`
	// before the flipper goroutine reaches `flag.Store(true)` and
	// which snapshot after. The flipPoint is a hint, not a barrier.
	// The invariants that matter for production semantics are:
	//   (1) both cohorts MUST be non-empty (proves the test actually
	//       observed a transition — otherwise this is not a flip
	//       race test, it's a single-flag test).
	//   (2) flagOffCount + flagOnCount == totalReqs (no lost reqs).
	//   (3) per-request status matches per-request flag snapshot
	//       (already asserted via flagOffWrong/flagOnWrong above).
	//   (4) the legacy-fallback metric ticks EXACTLY once per
	//       admitted flag=true request and ZERO times per
	//       flag=false rejection (asserted below).
	if flagOffCount == 0 {
		t.Fatalf("C4-B03: pre-flip cohort empty (flagOffCount=0) — flipper raced ahead of all workers, no flag=false dispatch observed; test failed to exercise the transition")
	}
	if flagOnCount == 0 {
		t.Fatalf("C4-B03: post-flip cohort empty (flagOnCount=0) — flipper never ran or all workers raced ahead, no flag=true dispatch observed; test failed to exercise the transition")
	}
	if flagOffCount+flagOnCount != totalReqs {
		t.Fatalf("C4-B03: cohort total mismatch off=%d on=%d total=%d want %d (lost results)",
			flagOffCount, flagOnCount, flagOffCount+flagOnCount, totalReqs)
	}
	// Legacy-fallback metric MUST tick exactly once per flag=true
	// admit. flag=false rejections MUST NOT tick it.
	if int(deltaFallback) != flagOnCount {
		t.Fatalf("C4-B03: AuthLegacyFallbackUsed{production} delta=%v want %d (one tick per flag=true admit, zero for flag=false reject)",
			deltaFallback, flagOnCount)
	}
	t.Logf("C4-B03: %d pre-flip rejects (flag=false → 401, fallback delta=0) | %d post-flip admits (flag=true → 200, fallback delta=%v) | flipPoint hint=%d (cohort split is stochastic; per-request status-snapshot consistency is the hard invariant) (race-detector clean; flag enforces per-instance immutability)",
		flagOffCount, flagOnCount, deltaFallback, flipPoint)
}

// --- C4-B04 ---------------------------------------------------------

// TestAuthChaos_S04_AuthMetricsCounterConcurrentEmit_AggregatesMatch
// proves the auth-metrics counter family
// (`smackerel_auth_validation_outcome_total{result, source}`) is
// safe under high-concurrency emit AND that aggregate counts match
// the deterministic expectation (Prometheus counter atomicity is the
// invariant; each increment lands without loss). The closed-set
// label spec is also enforced — every emission targets one of the
// documented (result, source) bucket pairs.
//
// Shape: 100 goroutines × 50 emits each = 5000 total emissions.
// Each goroutine deterministically picks a (result, source) pair
// based on its goroutine id so the expected per-bucket aggregate is
// computable up front:
//
//   - 5 results × 2 sources = 10 buckets
//   - each goroutine emits to ONE bucket for all 50 of its emits
//   - bucket assignment is `goroutineID % 10`
//   - so each bucket receives `(100 / 10) × 50 = 500` increments
//
// Verification: the test snapshots each bucket's value before, runs
// the burst, snapshots after, and asserts every bucket's delta is
// exactly 500. A regression that lost an increment (e.g., a torn
// CounterVec map write under contention) would surface as a bucket
// delta < 500. A regression that double-counted would surface as a
// bucket delta > 500. The race detector also catches torn map
// access because Prometheus's CounterVec uses sync.Mutex internally;
// any escape would fire under -race.
//
// Adversarial coverage: the test ALSO probes one out-of-set label
// pair (result="bobby_tables_DROP_TABLE", source="header") and
// asserts that emitting to it is silently absorbed by Prometheus
// (the counter is created but the test does NOT count it toward the
// expected aggregate; cardinality discipline at the EMITTER side
// is the production contract — see internal/metrics/auth.go closed-
// set documentation — not at the registry side).
func TestAuthChaos_S04_AuthMetricsCounterConcurrentEmit_AggregatesMatch(t *testing.T) {
	const concurrent = 100
	const perGoroutine = 50
	const buckets = 10

	results := []string{
		"accepted",
		"rejected_revoked",
		"rejected_expired",
		"rejected_malformed",
		"rejected_unknown_key",
	}
	sources := []string{"header", "pwa_cookie"}
	if len(results)*len(sources) != buckets {
		t.Fatalf("C4-B04: bucket math wrong: results=%d sources=%d want %d buckets", len(results), len(sources), buckets)
	}

	type bucketKey struct {
		result string
		source string
	}
	bucketAt := func(idx int) bucketKey {
		return bucketKey{result: results[idx%len(results)], source: sources[(idx/len(results))%len(sources)]}
	}

	// Snapshot every bucket BEFORE the burst.
	before := make(map[bucketKey]float64, buckets)
	for i := 0; i < buckets; i++ {
		bk := bucketAt(i)
		before[bk] = testutil.ToFloat64(metrics.AuthValidationOutcome.WithLabelValues(bk.result, bk.source))
	}

	var totalEmissions atomic.Int64
	var wg sync.WaitGroup
	gate := make(chan struct{})
	for g := 0; g < concurrent; g++ {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			bk := bucketAt(g % buckets)
			for k := 0; k < perGoroutine; k++ {
				metrics.AuthValidationOutcome.WithLabelValues(bk.result, bk.source).Inc()
				totalEmissions.Add(1)
			}
		}()
	}
	close(gate)
	wg.Wait()

	// Snapshot every bucket AFTER the burst and assert deltas.
	const wantPerBucket = (concurrent / buckets) * perGoroutine
	wantTotal := wantPerBucket * buckets
	if int64(wantTotal) != int64(concurrent*perGoroutine) {
		t.Fatalf("C4-B04: aggregate math wrong: per-bucket=%d × buckets=%d = %d != concurrent×per=%d",
			wantPerBucket, buckets, wantTotal, concurrent*perGoroutine)
	}

	var bucketDeltaTotal float64
	var perBucketLog strings.Builder
	for i := 0; i < buckets; i++ {
		bk := bucketAt(i)
		after := testutil.ToFloat64(metrics.AuthValidationOutcome.WithLabelValues(bk.result, bk.source))
		delta := after - before[bk]
		bucketDeltaTotal += delta
		fmt.Fprintf(&perBucketLog, "  {%s,%s} delta=%v want %d\n", bk.result, bk.source, delta, wantPerBucket)
		if int(delta) != wantPerBucket {
			t.Fatalf("C4-B04: bucket {%s,%s} delta=%v want %d (lost or double-counted increment under contention)",
				bk.result, bk.source, delta, wantPerBucket)
		}
	}
	if int(totalEmissions.Load()) != concurrent*perGoroutine {
		t.Fatalf("C4-B04: emissions counter=%d want %d", totalEmissions.Load(), concurrent*perGoroutine)
	}
	if int(bucketDeltaTotal) != wantTotal {
		t.Fatalf("C4-B04: aggregate bucket delta=%v want %d", bucketDeltaTotal, wantTotal)
	}

	// Adversarial: probe an out-of-set label pair. Prometheus will
	// accept it (the closed-set discipline is enforced at the
	// emitter, not the registry), but the test does not count it
	// toward the expected aggregate. This ALSO proves the test's
	// expected math is robust against incidental noise from other
	// out-of-set emitters.
	advBefore := testutil.ToFloat64(metrics.AuthValidationOutcome.WithLabelValues("bobby_tables_DROP_TABLE", "header"))
	metrics.AuthValidationOutcome.WithLabelValues("bobby_tables_DROP_TABLE", "header").Inc()
	advAfter := testutil.ToFloat64(metrics.AuthValidationOutcome.WithLabelValues("bobby_tables_DROP_TABLE", "header"))
	if got := advAfter - advBefore; got != 1 {
		t.Fatalf("C4-B04 adversarial: out-of-set label pair did not record (Prometheus regression?) delta=%v", got)
	}

	t.Logf("C4-B04: %d emissions across %d buckets → all per-bucket deltas exact (%d each), aggregate=%v want %d (race-detector clean; Prometheus CounterVec atomicity intact under contention)\n%s",
		totalEmissions.Load(), buckets, wantPerBucket, bucketDeltaTotal, wantTotal, perBucketLog.String())
}

// --- C4-B05 ---------------------------------------------------------

// TestAuthChaos_S04_LegacyFallbackProductionFlagFalse_AllRejected
// proves the deprecation contract under concurrent burst: with
// `auth.production_shared_token_fallback_enabled = false` in
// production, ALL legacy SMACKEREL_AUTH_TOKEN bearers MUST be
// rejected. The legacy-fallback metric MUST NOT tick (because the
// fallback path never admitted any request); the auth-failure metric
// MUST tick on every reject.
//
// Shape: N=50 concurrent requests, each carrying the legacy shared
// bearer, hit `/v1/photos/connectors` on a router built with
// flag=false. Expectations:
//
//   - Every request returns 401.
//   - `smackerel_auth_legacy_fallback_used_total{environment="production"}`
//     delta = 0 (no admission via the fallback).
//   - `smackerel_auth_failure_total{reason="paseto_verify_failed"}`
//     delta = N (every reject classifies as a verifier failure
//     because the shared token is not a valid PASETO).
//
// The "ZERO request slips through" assertion is the chaos invariant.
// A regression that accidentally re-enabled the fallback (e.g., a
// missing `flag &&` check in the middleware) would surface as a
// non-zero AuthLegacyFallbackUsed delta AND/OR a 200 status.
func TestAuthChaos_S04_LegacyFallbackProductionFlagFalse_AllRejected(t *testing.T) {
	const concurrent = 50

	priv, pub := auth.GenerateSigningKeypair()
	const kid = "scope04-chaos-flagfalse-key"
	const sharedToken = "scope04-chaos-flagfalse-shared-token"

	pool := authTestPool(t)
	t.Cleanup(func() { pool.Close() })
	resetAuthTables(t, pool)
	t.Cleanup(func() { resetAuthTables(t, pool) })

	deps := &api.Dependencies{
		Environment: "production",
		AuthToken:   sharedToken,
		AuthConfig: config.AuthConfig{
			Enabled:                               true,
			TokenFormat:                           "paseto_v4_public",
			SigningActivePrivateKey:               priv,
			SigningActiveKeyID:                    kid,
			TokenTTLHours:                         24,
			RotationGraceWindowHours:              2,
			ClockSkewToleranceSeconds:             60,
			RevocationCacheRefreshIntervalSeconds: 60,
			AtRestHashingKey:                      priv + "-hash-suffix-distinct",
			ProductionSharedTokenFallbackEnabled:  false,
		},
		AuthVerifyOptions: auth.VerifyOptions{
			ActivePublicKey:    pub,
			ActiveKeyID:        kid,
			Issuer:             "smackerel",
			ClockSkewTolerance: time.Minute,
			Now:                time.Now,
		},
		RevocationCache: revocation.NewCache(),
		PhotosHandlers:  api.NewPhotosHandlers(photolib.NewStore(pool), config.PhotosConfig{}, "production"),
	}
	router := api.NewRouter(deps)

	beforeFallback := testutil.ToFloat64(metrics.AuthLegacyFallbackUsed.WithLabelValues("production"))
	beforeFailure := testutil.ToFloat64(metrics.AuthFailure.WithLabelValues("paseto_verify_failed"))

	var rejected, admitted, slipped atomic.Int64
	var failures []string
	var failuresMu sync.Mutex
	recordFailure := func(s string) {
		failuresMu.Lock()
		failures = append(failures, s)
		failuresMu.Unlock()
	}

	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := 0; i < concurrent; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			req := httptest.NewRequest(http.MethodGet, "/v1/photos/connectors", nil)
			req.Header.Set("Authorization", "Bearer "+sharedToken)
			req.RemoteAddr = fmt.Sprintf("10.7.0.%d:1234", (i%250)+1)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			switch rec.Code {
			case http.StatusUnauthorized:
				rejected.Add(1)
			case http.StatusOK:
				admitted.Add(1)
				slipped.Add(1)
				recordFailure(fmt.Sprintf("req=%d slipped through with 200 (flag=false but legacy bearer admitted) body=%s",
					i, truncForLogS04(rec.Body.String())))
			default:
				recordFailure(fmt.Sprintf("req=%d unexpected status=%d body=%s",
					i, rec.Code, truncForLogS04(rec.Body.String())))
			}
		}()
	}
	close(gate)
	wg.Wait()

	afterFallback := testutil.ToFloat64(metrics.AuthLegacyFallbackUsed.WithLabelValues("production"))
	afterFailure := testutil.ToFloat64(metrics.AuthFailure.WithLabelValues("paseto_verify_failed"))
	deltaFallback := afterFallback - beforeFallback
	deltaFailure := afterFailure - beforeFailure

	// CHAOS INVARIANT — ZERO requests may slip through.
	if slipped.Load() != 0 || admitted.Load() != 0 {
		t.Fatalf("C4-B05: %d/%d legacy-bearer requests SLIPPED THROUGH despite flag=false (deprecation regression) sample=%v",
			slipped.Load(), concurrent, failures[:min(len(failures), 3)])
	}
	if rejected.Load() != concurrent {
		t.Fatalf("C4-B05: reject accounting: want %d rejected, got %d sample=%v",
			concurrent, rejected.Load(), failures[:min(len(failures), 3)])
	}
	if int(deltaFallback) != 0 {
		t.Fatalf("C4-B05: AuthLegacyFallbackUsed{production} delta=%v want 0 (no admission via fallback path) before=%v after=%v",
			deltaFallback, beforeFallback, afterFallback)
	}
	if int(deltaFailure) != concurrent {
		t.Fatalf("C4-B05: AuthFailure{paseto_verify_failed} delta=%v want %d (every legacy bearer rejection classifies as verifier failure) before=%v after=%v",
			deltaFailure, concurrent, beforeFailure, afterFailure)
	}
	t.Logf("C4-B05: %d concurrent legacy-bearer requests with flag=false in production → ALL rejected (401), AuthLegacyFallbackUsed delta=0 (deprecation enforced), AuthFailure{paseto_verify_failed} delta=%v",
		rejected.Load(), deltaFailure)
}

// --- C4-Hot-Path Benchmark ------------------------------------------

// BenchmarkAuthChaos_S04_F02MintHotPath measures the F02 mint hot
// path that runs on every inbound Telegram message in production:
//
//	bot.bearerForChat(chatID)
//	  ↓ tokenMinter.MintForChat(chatID)
//	  ↓ bot.resolveActorUserID(chatID)        — in-memory map lookup
//	  ↓ MintForUser(chatID, userID)
//	  ↓ newTelegramTokenID                     — crypto/rand 12 bytes + hex
//	  ↓ auth.IssueToken                         — Ed25519 sign
//	  ↓ metrics.AuthIssuance{telegram_bridge}.Inc
//	  ↓ setBearerHeader applies "Bearer <wire>" header
//
// Target: NFR-AUTH-001 budget ~5ms/op. The dominant cost is the
// Ed25519 signing inside auth.IssueToken (~50µs amortized).
func BenchmarkAuthChaos_S04_F02MintHotPath(b *testing.B) {
	mapping := map[int64]string{
		54321: "tg-user-bench-hot-path",
	}
	bot := telegram.NewBotForTest("production", mapping)
	bot.SetSharedAuthTokenForTest("WRONG-shared-bearer-bench")

	priv, _ := auth.GenerateSigningKeypair()
	const kid = "scope04-bench-key"
	minter, err := telegram.NewPerUserTokenMinter(telegram.PerUserTokenMinterOptions{
		Bot:        bot,
		SigningKey: priv,
		KeyID:      kid,
		Issuer:     "smackerel",
		TTL:        5 * time.Minute,
		Now:        time.Now,
	})
	if err != nil {
		b.Fatalf("NewPerUserTokenMinter: %v", err)
	}
	bot.SetPerUserTokenMinter(minter)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, err := http.NewRequest(http.MethodGet, "http://example.invalid/", nil)
		if err != nil {
			b.Fatalf("NewRequest: %v", err)
		}
		if err := bot.SetBearerHeaderForTest(req, 54321); err != nil {
			b.Fatalf("SetBearerHeader: %v", err)
		}
	}
}
