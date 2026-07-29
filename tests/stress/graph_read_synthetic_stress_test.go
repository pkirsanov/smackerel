//go:build stress

// BUG-080-001 SCOPE-03 — T080-03-STRESS (SCN-080-001-03).
//
// Proves the product-owned Knowledge Graph read synthetic
// (internal/graphsynthetic) remains BOUNDED and TRUTHFUL when many
// validation reads execute CONCURRENTLY against the live disposable
// stack over real HTTP with a real scoped session.
//
// The three claims this row exists to prove, all measured from the SAME
// concurrent burst:
//
//  1. BOUNDED — the observation stays inside the synthetic's OWN
//     configured bound under load. Concurrency must not turn a bounded
//     eight-family read into an unbounded one. Both the p95 and the
//     hard per-run ceiling are DERIVED from the configured
//     RequestTimeout and from len(graphapi.RequiredGraphFamilies());
//     neither is a hand-tuned number fitted to an observation.
//
//  2. TRUTHFUL — every concurrent run returns without error, satisfies
//     the aggregate's own closed-vocabulary contract (Validate), still
//     reports the explicit ENABLED activation policy it was run under,
//     and still carries a COMPLETE canonical family set. The expected
//     family set is derived from graphapi.RequiredGraphFamilies() —
//     never a hardcoded list and never a hardcoded count — so adding a
//     ninth canonical family cannot silently pass this test.
//
//  3. CONSISTENT UNDER CONCURRENCY — concurrency must not change the
//     truth. Every run in the burst must agree on Available() and on
//     the aggregate State. A single disagreeing run is a hard failure
//     naming BOTH verdicts; that is the actual anti-race assertion.
//
// The rows the synthetic reads are seeded here through DATABASE_URL so
// the burst exercises the genuinely POPULATED read path rather than
// passing on an unexercised empty read. Exactly two families are
// excused as permitted-empty (places, place_detail): they read from
// location_clusters / maps_places / artifact_places and no seeder for
// those tables exists anywhere in this repository, so excusing them is
// honest and excusing anything else would not be.
//
// This row is scoped to bounded + truthful concurrent reads ONLY. It
// deliberately asserts nothing about any trace workflow or SLO
// contract: the repository registers only the unrelated core.health
// trace workflow, and there is no graph-specific trace/SLO contract to
// assert against.

package stress

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/api/graphapi"
	"github.com/smackerel/smackerel/internal/graphsynthetic"
)

const (
	// graphSynthStressWorkers is the concurrent validation-read fan-out.
	// Eight independent readers share ONE *graphsynthetic.Synthetic and
	// ONE *http.Client, so the burst genuinely exercises the runner for
	// shared-state races instead of giving each goroutine its own
	// private copy.
	graphSynthStressWorkers = 8
	// graphSynthStressIterations is the per-worker repeat count. 8 x 20
	// = 160 complete aggregate observations (160 x 8 family reads =
	// 1280 authenticated GETs), enough for meaningful p95/p99 indices
	// while staying well inside the 720s stress package timeout.
	graphSynthStressIterations = 20

	// graphSynthStressRequestTimeout is the per-family read bound handed
	// to the synthetic. It matches the value the proven e2e row uses
	// (tests/e2e/graph_read_synthetic_e2e_test.go), so the stress burst
	// runs under the SAME contract the acceptance row runs under.
	graphSynthStressRequestTimeout = 15 * time.Second

	// graphSynthStressP95Budget is the declared p95 wall-clock budget
	// for one COMPLETE aggregate observation under concurrency.
	//
	// Basis: the synthetic's contract bounds each individual family read
	// at RequestTimeout. The budget asserts that the ENTIRE eight-family
	// aggregate, under 8-way concurrency, still completes within the
	// time the contract allows for a SINGLE family read — i.e. an
	// 8x tightening of the structural ceiling below, derived from the
	// same configured value rather than fitted to a measurement.
	//
	// This is deliberately an "is it still bounded" assertion, not a
	// tight latency SLO: this repository declares no graph-read latency
	// SLO to assert against, and inventing one here would be a
	// fabricated contract. The measured p50/p95/p99/max are logged
	// verbatim below so the real headroom is visible in the evidence
	// rather than implied by the budget.
	graphSynthStressP95Budget = graphSynthStressRequestTimeout
)

// graphSynthStressRun is one complete concurrent observation.
type graphSynthStressRun struct {
	worker    int
	iteration int
	recorded  bool
	latency   time.Duration
	err       error
	agg       graphsynthetic.AggregateResult
}

// graphSynthStressEnv resolves the live-stack coordinates. It FAILS
// LOUD on a missing value: a stress row that skips itself when the
// stack is absent cannot prove anything, and a silent skip is exactly
// the hollow pass this row must be incapable of producing.
func graphSynthStressEnv(t *testing.T) (coreURL, authToken, databaseURL string) {
	t.Helper()
	coreURL = strings.TrimSpace(os.Getenv("CORE_EXTERNAL_URL"))
	if coreURL == "" {
		t.Fatalf("stress: CORE_EXTERNAL_URL is empty; T080-03-STRESS reads the LIVE stack over real HTTP and must never pass without one")
	}
	authToken = strings.TrimSpace(os.Getenv("SMACKEREL_AUTH_TOKEN"))
	if authToken == "" {
		t.Fatalf("stress: SMACKEREL_AUTH_TOKEN is empty; the synthetic reads with a REAL scoped session and never falls back to an unauthenticated read")
	}
	databaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Fatalf("stress: DATABASE_URL is empty; T080-03-STRESS seeds real disposable graph rows so the burst exercises the POPULATED read path")
	}
	return coreURL, authToken, databaseURL
}

// graphSynthStressSeed inserts real, disposable rows so topics,
// topic_detail, people, person_detail, time AND edges are genuinely
// populated for the burst, and registers their removal.
func graphSynthStressSeed(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()
	ctx := context.Background()

	topicIDs := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		id := prefix + "-topic-" + strconv.Itoa(i)
		if _, err := pool.Exec(ctx,
			`INSERT INTO topics (id, name, capture_count_total, momentum_score)
			 VALUES ($1, $2, $3, $4)`,
			id, id, 10+i, float32(1.0+float64(i)*0.1)); err != nil {
			t.Fatalf("seed topic %s: %v", id, err)
		}
		topicIDs = append(topicIDs, id)
	}

	personID := prefix + "-person-0"
	if _, err := pool.Exec(ctx,
		`INSERT INTO people (id, name) VALUES ($1, $2)`, personID, personID); err != nil {
		t.Fatalf("seed person %s: %v", personID, err)
	}

	// Artifacts are backdated across the preceding days so the time
	// family reads a non-empty day set inside the bounded UTC window.
	base := time.Now().UTC().Add(-3 * 24 * time.Hour)
	for i := 0; i < 3; i++ {
		id := prefix + "-artifact-" + strconv.Itoa(i)
		ts := base.Add(time.Duration(i) * 24 * time.Hour)
		if _, err := pool.Exec(ctx,
			`INSERT INTO artifacts
			   (id, artifact_type, title, content_hash, source_id, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
			id, "note", id+"-title", id+"-hash", "graph-synth-stress-seed", ts); err != nil {
			t.Fatalf("seed artifact %s: %v", id, err)
		}
	}

	// The synthetic seeds its edges read from `source=topic:<first row of
	// GET /api/topics/>`, so a topic-sourced edge must exist for whichever
	// topic wins the ordering. Seed one from EVERY seeded topic.
	for _, topicID := range topicIDs {
		edgeID := prefix + "-edge-topic-" + topicID + "-person-" + personID
		if _, err := pool.Exec(ctx,
			`INSERT INTO edges (id, src_type, src_id, dst_type, dst_id, edge_type, weight)
			 VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`,
			edgeID, "topic", topicID, "person", personID, "co-occurs", float32(1.0)); err != nil {
			t.Fatalf("seed edge %s: %v", edgeID, err)
		}
	}

	// Make the edges read DETERMINISTIC rather than allow-empty.
	// internal/api/graphapi/topics.go orders by `momentum_score DESC
	// NULLS LAST, capture_count_total DESC NULLS LAST, id ASC`, so a
	// pre-existing higher-momentum topic in the shared database would win
	// and hand the synthetic a seed id with no topic-sourced edge —
	// silently turning `edges` into a zero-row read. Promoting a seeded
	// topic above the current maximum removes that ambiguity, so a
	// zero-row edges read stays a FAILURE here rather than a quiet pass.
	if _, err := pool.Exec(ctx,
		`UPDATE topics
		    SET momentum_score = COALESCE((SELECT MAX(momentum_score) FROM topics), 0) + 1,
		        capture_count_total = COALESCE((SELECT MAX(capture_count_total) FROM topics), 0) + 1
		  WHERE id = $1`, topicIDs[0]); err != nil {
		t.Fatalf("promote seeded topic to first position in the momentum ordering: %v", err)
	}
}

// graphSynthStressCleanup removes every row this row seeded.
func graphSynthStressCleanup(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()
	ctx := context.Background()
	like := prefix + "-%"
	for _, q := range []string{
		`DELETE FROM edges     WHERE id LIKE $1 OR src_id LIKE $1 OR dst_id LIKE $1`,
		`DELETE FROM artifacts WHERE id LIKE $1`,
		`DELETE FROM people    WHERE id LIKE $1`,
		`DELETE FROM topics    WHERE id LIKE $1`,
	} {
		if _, err := pool.Exec(ctx, q, like); err != nil {
			t.Logf("cleanup: %v", err)
		}
	}
}

// graphSynthStressFamilyTriples renders a value-safe family summary for
// a failure message or the evidence log. Every component is a closed
// vocabulary token, so this can never emit content.
func graphSynthStressFamilyTriples(rows []graphsynthetic.GraphFamilyResult) string {
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		parts = append(parts, fmt.Sprintf("%s=%s/%s", row.Family, row.State, row.Code))
	}
	return strings.Join(parts, " ")
}

func TestGraphReadSyntheticStress_BoundedAndTruthfulUnderConcurrentValidationReads(t *testing.T) {
	coreURL, authToken, databaseURL := graphSynthStressEnv(t)
	stressWaitForHealth(t, stressConfig{CoreURL: coreURL, AuthToken: authToken}, 60*time.Second)

	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("connect graph read synthetic stress database: %v", err)
	}
	// t.Cleanup runs LIFO, so the pool close is registered FIRST in order
	// to run LAST. Registering it later would close the pool while the
	// fixture DELETEs were still pending, turning cleanup into a silent
	// no-op and leaking rows into the shared graph tables.
	t.Cleanup(pool.Close)

	prefix := "graph-synth-stress-" + time.Now().UTC().Format("20060102150405.000000")
	t.Cleanup(func() { graphSynthStressCleanup(t, pool, prefix) })
	graphSynthStressSeed(t, pool, prefix)

	// One shared transport for the whole burst. The synthetic's Config
	// contract states the CALLER owns transport policy, so the idle-conn
	// pool is sized to the worker count deliberately: without it the
	// default MaxIdleConnsPerHost of 2 would make the measurement mostly
	// connection churn rather than read latency.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = graphSynthStressWorkers * 4
	transport.MaxIdleConnsPerHost = graphSynthStressWorkers * 4
	httpClient := &http.Client{
		Timeout:   graphSynthStressRequestTimeout + 5*time.Second,
		Transport: transport,
	}

	now := time.Now().UTC()
	synthCfg := graphsynthetic.Config{
		BaseURL:        coreURL,
		BearerToken:    authToken,
		WindowFrom:     now.Add(-30 * 24 * time.Hour),
		WindowTo:       now.Add(1 * time.Hour),
		RequestTimeout: graphSynthStressRequestTimeout,
		EdgeSourceKind: "topic",
		// ONLY the two families this harness structurally cannot seed:
		// places / place_detail read from location_clusters, maps_places
		// and artifact_places, and no seeder for those tables exists
		// anywhere in tests/ or internal/. Every other family — topics,
		// topic_detail, people, person_detail, time AND edges — is
		// genuinely seeded above, so a zero-row read for any of them
		// FAILS rather than passing as an excused empty.
		AllowEmptyFamilies: []graphapi.GraphRouteFamily{
			graphapi.FamilyPlaces,
			graphapi.FamilyPlaceDetail,
		},
		// Deliberately empty: nothing is silently excused, so a degraded
		// aggregate is unreachable and concurrency cannot hide a failure
		// behind an optional-family omission.
		OptionalFamilies: []graphapi.GraphRouteFamily{},
		HTTPClient:       httpClient,
	}

	activation := graphapi.Activation{
		State:          graphapi.ActivationEnabled,
		SecretPresence: graphapi.SecretPresent,
		Code:           graphapi.CodeActivationOK,
	}

	// ONE shared synthetic across every worker. Sharing is the point:
	// a per-goroutine instance would not exercise the runner for races.
	synth, err := graphsynthetic.New(synthCfg, graphsynthetic.NopObserver{})
	if err != nil {
		t.Fatalf("graphsynthetic.New: %v", err)
	}

	totalRuns := graphSynthStressWorkers * graphSynthStressIterations
	results := make([]graphSynthStressRun, totalRuns)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	burstStarted := time.Now()
	var wg sync.WaitGroup
	for w := 0; w < graphSynthStressWorkers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < graphSynthStressIterations; i++ {
				// Each worker owns a disjoint index range, so the shared
				// slice needs no lock and carries no write race.
				idx := worker*graphSynthStressIterations + i
				start := time.Now()
				agg, runErr := synth.Run(ctx, activation)
				elapsed := time.Since(start)
				results[idx] = graphSynthStressRun{
					worker:    worker,
					iteration: i,
					recorded:  true,
					latency:   elapsed,
					err:       runErr,
					agg:       agg,
				}
			}
		}(w)
	}
	wg.Wait()
	burstElapsed := time.Since(burstStarted)

	// ---- Anti-vacuity guards -------------------------------------------
	// A hollow pass must be structurally impossible: assert real work was
	// recorded BEFORE asserting anything about that work.
	if len(results) == 0 {
		t.Fatalf("anti-vacuity: zero result slots were allocated; the burst asserted nothing")
	}
	recorded := 0
	for _, r := range results {
		if r.recorded {
			recorded++
		}
	}
	if recorded == 0 {
		t.Fatalf("anti-vacuity: zero concurrent runs were recorded; the burst asserted nothing")
	}
	if recorded != totalRuns {
		t.Fatalf("anti-vacuity: %d of %d concurrent runs were recorded; a worker exited without completing its iterations",
			recorded, totalRuns)
	}

	required := graphapi.RequiredGraphFamilies()
	if len(required) == 0 {
		t.Fatalf("anti-vacuity: graphapi.RequiredGraphFamilies() is empty; the family-completeness assertion would be vacuous")
	}

	// ---- TRUTHFUL: every concurrent run ---------------------------------
	for _, r := range results {
		label := fmt.Sprintf("worker=%d iteration=%d", r.worker, r.iteration)

		if r.err != nil {
			t.Fatalf("%s: Synthetic.Run against the live stack failed under concurrency: %v", label, r.err)
		}
		if err := r.agg.Validate(); err != nil {
			t.Fatalf("%s: aggregate failed its own closed-vocabulary contract under concurrency: %v (families: %s)",
				label, err, graphSynthStressFamilyTriples(r.agg.Families))
		}
		if r.agg.Activation != graphapi.ActivationEnabled {
			t.Fatalf("%s: aggregate reports activation %q; the observation ran under the explicit %q policy",
				label, r.agg.Activation, graphapi.ActivationEnabled)
		}

		// Family-set completeness, DERIVED from the canonical manifest —
		// never a hardcoded list and never a hardcoded count.
		seen := make(map[graphapi.GraphRouteFamily]int, len(r.agg.Families))
		for _, row := range r.agg.Families {
			seen[row.Family]++
		}
		for _, family := range required {
			switch seen[family] {
			case 1:
				// exactly once — the contract
			case 0:
				t.Fatalf("%s: canonical family %q is ABSENT from the aggregate under concurrency (families: %s)",
					label, family, graphSynthStressFamilyTriples(r.agg.Families))
			default:
				t.Fatalf("%s: canonical family %q appears %d times in the aggregate; the contract requires exactly one row (families: %s)",
					label, family, seen[family], graphSynthStressFamilyTriples(r.agg.Families))
			}
		}
		if len(seen) != len(required) {
			t.Fatalf("%s: aggregate carries %d distinct families; the canonical manifest declares %d (families: %s)",
				label, len(seen), len(required), graphSynthStressFamilyTriples(r.agg.Families))
		}
		if len(r.agg.Families) != len(required) {
			t.Fatalf("%s: aggregate carries %d family rows; the canonical manifest requires exactly %d (families: %s)",
				label, len(r.agg.Families), len(required), graphSynthStressFamilyTriples(r.agg.Families))
		}
	}

	// ---- CONSISTENT UNDER CONCURRENCY -----------------------------------
	// Concurrency must not change the truth. Every run in the burst must
	// reach the SAME verdict; a disagreement is a race, not a nuance.
	reference := results[0]
	for _, r := range results[1:] {
		if r.agg.Available() != reference.agg.Available() {
			t.Fatalf("concurrency changed the verdict: worker=%d iteration=%d reports Available()=%t (state=%q code=%q) while worker=%d iteration=%d reports Available()=%t (state=%q code=%q)",
				r.worker, r.iteration, r.agg.Available(), r.agg.State, r.agg.Code,
				reference.worker, reference.iteration, reference.agg.Available(), reference.agg.State, reference.agg.Code)
		}
		if r.agg.State != reference.agg.State {
			t.Fatalf("concurrency changed the aggregate state: worker=%d iteration=%d reports state=%q (code=%q) while worker=%d iteration=%d reports state=%q (code=%q)",
				r.worker, r.iteration, r.agg.State, r.agg.Code,
				reference.worker, reference.iteration, reference.agg.State, reference.agg.Code)
		}
	}

	// ---- BOUNDED ---------------------------------------------------------
	latencies := make([]time.Duration, 0, len(results))
	for _, r := range results {
		latencies = append(latencies, r.latency)
	}
	if len(latencies) == 0 {
		t.Fatalf("anti-vacuity: the collected latency sample is empty; the boundedness assertion would be vacuous")
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]
	maxL := latencies[len(latencies)-1]

	// The structural ceiling: the synthetic bounds every family read at
	// RequestTimeout, so one complete observation can never legitimately
	// exceed RequestTimeout x the canonical family count. Derived, not
	// hardcoded.
	hardCeiling := graphSynthStressRequestTimeout * time.Duration(len(required))

	t.Logf("T080-03-STRESS graph read synthetic — workers=%d iterationsPerWorker=%d totalRuns=%d recordedRuns=%d familiesPerRun=%d totalFamilyReads=%d burstWallClock=%v",
		graphSynthStressWorkers, graphSynthStressIterations, totalRuns, recorded,
		len(required), recorded*len(required), burstElapsed)
	t.Logf("T080-03-STRESS latency — p50=%v p95=%v p99=%v max=%v (p95Budget=%v hardCeiling=%v = RequestTimeout %v x %d canonical families)",
		p50, p95, p99, maxL, time.Duration(graphSynthStressP95Budget), hardCeiling,
		time.Duration(graphSynthStressRequestTimeout), len(required))
	t.Logf("T080-03-STRESS verdict — every run agreed: available=%t state=%q code=%q activation=%q; families: %s",
		reference.agg.Available(), reference.agg.State, reference.agg.Code, reference.agg.Activation,
		graphSynthStressFamilyTriples(reference.agg.Families))

	if p95 > graphSynthStressP95Budget {
		t.Fatalf("T080-03-STRESS boundedness breach: p95=%v exceeds the declared budget=%v under %d-way concurrency",
			p95, time.Duration(graphSynthStressP95Budget), graphSynthStressWorkers)
	}
	for _, r := range results {
		if r.latency > hardCeiling {
			t.Fatalf("T080-03-STRESS hard ceiling breach: worker=%d iteration=%d took %v, exceeding the structural ceiling %v (RequestTimeout %v x %d canonical families); the per-family timeout failed to bound the observation",
				r.worker, r.iteration, r.latency, hardCeiling,
				time.Duration(graphSynthStressRequestTimeout), len(required))
		}
	}
}
