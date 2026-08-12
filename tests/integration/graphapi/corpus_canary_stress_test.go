//go:build integration

package graphapi_integration

import (
	"net/http"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
)

// Spec 108 SCOPE-03 — TP-03-08 (router-bootstrap canary) and TP-03-09 (stress).
//
// Both run against the REAL production router built by newCorpusEnforceRouter,
// with per-user PASETO validation active and a real intelligence engine so the
// Tier B groups actually register.

// TestIntegration_CorpusGrantEnforce_RouterBootstrapCanary_TP_03_08 is the
// narrow canary over the shared router bootstrap.
//
// Mounting a new gate on a shared middleware group can break three things that
// have nothing to do with the gate's own logic, and each is silent: the
// authentication step could stop running first, an authenticated session could
// stop resolving, and routes that were deliberately left ungated could get
// swept in. This asserts all three, independently runnable, so a bootstrap
// regression is attributable without running the full sweep.
func TestIntegration_CorpusGrantEnforce_RouterBootstrapCanary_TP_03_08(t *testing.T) {
	stack, privateHex := newCorpusEnforceStack(t, "CORPUSCANARY")
	base := stack.serve(t, true)

	granted := mintCorpusToken(t, privateHex, "tp0308-granted", []string{auth.GrantGlobalCorpusRead})
	ungranted := mintCorpusToken(t, privateHex, "tp0308-ungranted", []string{corpusOtherScope})

	probe := corpusEnforceRoute{method: http.MethodGet, path: "/api/recent?limit=1"}

	// CONTROL — the gate must actually be mounted, or every assertion below is
	// about an unguarded router and proves nothing.
	resp, body := corpusDo(t, base, ungranted, probe)
	if !isCorpusGateDenial(resp, body) {
		t.Fatalf("canary control failed: an ungranted principal was NOT refused by the corpus gate on %s (status=%d). "+
			"The gate is not mounted, so the ordering/session/ungated assertions below would all pass against an unguarded router.",
			probe.path, resp.StatusCode)
	}

	// 1. ORDERING — bearerAuthMiddleware must run BEFORE the gate.
	//
	// A structurally invalid bearer must be rejected as UNAUTHENTICATED (401).
	// A 403 here would mean the gate evaluated a request whose identity was
	// never established — the scope check would be reading a session the auth
	// layer had not yet built, which is how an unauthenticated caller ends up
	// being described by a scope decision.
	resp, body = corpusDo(t, base, "not-a-valid-token", probe)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("middleware ORDERING contract broken: an invalid bearer returned %d, want 401. "+
			"If this is 403 the corpus gate ran before authentication. body=%s", resp.StatusCode, string(body))
	}

	// 2. SESSION — a granted session must still resolve through the shared
	// bootstrap. If the gate consumed or replaced the session, the allow path
	// would break for every principal that legitimately holds the grant.
	resp, body = corpusDo(t, base, granted, probe)
	assertNotRefusedByCorpusGate(t, "canary/granted", resp, body)

	// 3. UNGATED ROUTES — everything design.md §2 deliberately left ungated
	// must remain reachable. This is the blast-radius half of the canary: the
	// gate is mounted on a shared group, so an over-broad mount silently pulls
	// in the write path and the unauthenticated probes.
	checked := 0
	for _, u := range corpusEnforceUngated {
		route := corpusEnforceRoute{method: u.method, path: u.path}
		if u.method == http.MethodPost {
			route.body = "{}"
		}
		bearer := ungranted
		if u.otherScope == "" {
			bearer = corpusOperatorToken
		}
		resp, body := corpusDo(t, base, bearer, route)
		if isCorpusGateDenial(resp, body) {
			t.Errorf("ungated route %s %s was refused by the CORPUS gate under ENFORCE — the mount is over-broad (%s)",
				u.method, u.path, u.why)
		}
		checked++
	}
	if checked != len(corpusEnforceUngated) {
		t.Fatalf("checked %d ungated routes, want %d", checked, len(corpusEnforceUngated))
	}
	t.Logf("TP-03-08 canary: ordering + session + %d ungated routes intact under ENFORCE", checked)
}

// TestIntegration_CorpusGrantEnforce_GateAddsNoP99Regression_TP_03_09 is the
// SLA row. The gate sits on the hot read path of all sixteen corpus route
// groups, so its per-request cost is a product property, not an implementation
// detail.
//
// The comparison is the same router, the same load, the same principal, with
// the stage as the ONLY difference — so any latency delta is attributable to
// `RequireScope` + `Observe` rather than to environment noise between two
// differently-built stacks.
func TestIntegration_CorpusGrantEnforce_GateAddsNoP99Regression_TP_03_09(t *testing.T) {
	stack, privateHex := newCorpusEnforceStack(t, "CORPUSSTRESS")
	granted := mintCorpusToken(t, privateHex, "tp0309-granted", []string{auth.GrantGlobalCorpusRead})
	ungranted := mintCorpusToken(t, privateHex, "tp0309-ungranted", []string{corpusOtherScope})

	observeBase := stack.serve(t, false)
	enforceBase := stack.serve(t, true)

	// CONTROL — without this the whole measurement could be comparing two
	// identical OBSERVE routers and would "prove" zero overhead for a gate that
	// was never mounted.
	resp, body := corpusDo(t, enforceBase, ungranted, corpusEnforceRoute{method: http.MethodGet, path: "/api/recent?limit=1"})
	if !isCorpusGateDenial(resp, body) {
		t.Fatalf("stress control failed: the ENFORCE arm did not refuse an ungranted principal (status=%d); "+
			"the two arms are not actually different, so any latency delta would be meaningless", resp.StatusCode)
	}

	routes := []corpusEnforceRoute{
		{method: http.MethodGet, path: "/api/recent?limit=1"},
		{method: http.MethodGet, path: "/api/artifact/" + corpusEnforceCanaryID},
	}

	const (
		concurrency = 8
		perWorker   = 40
		// The gate is a session lookup, a scope compare and a counter
		// increment — microseconds of real work.
		//
		// TWO budgets, because they answer different questions. p50 is the
		// attributable one: at the median, scheduler and GC noise cancel, so a
		// median delta IS the gate's per-request cost and can be held tight.
		// p99 in a containerised runner at this sample count is dominated by
		// tail noise — it is kept as a REGRESSION guard with a loose budget,
		// not as a measurement of the gate. Asserting only p99 would be both
		// weaker evidence and flakier.
		p50BudgetDelta = 5 * time.Millisecond
		p99BudgetDelta = 25 * time.Millisecond
	)

	for _, route := range routes {
		t.Run(route.path, func(t *testing.T) {
			observe := corpusMeasure(t, observeBase, granted, route, concurrency, perWorker)
			enforce := corpusMeasure(t, enforceBase, granted, route, concurrency, perWorker)

			t.Logf("TP-03-09 %s: OBSERVE p50=%v p95=%v p99=%v | ENFORCE p50=%v p95=%v p99=%v | n=%d each",
				route.path, observe.p50, observe.p95, observe.p99,
				enforce.p50, enforce.p95, enforce.p99, observe.n)

			if enforce.failures > 0 || observe.failures > 0 {
				t.Fatalf("granted principal was refused during the load run (observe=%d enforce=%d failures); "+
					"latency of a refused request is not the latency of the allow path",
					observe.failures, enforce.failures)
			}

			if delta := enforce.p50 - observe.p50; delta > p50BudgetDelta {
				t.Errorf("corpus gate added %v to the MEDIAN on %s (OBSERVE %v → ENFORCE %v), budget %v. "+
					"At p50 this delta is the gate's own per-request cost, not noise — RequireScope + Observe should be a map lookup and a counter.",
					delta, route.path, observe.p50, enforce.p50, p50BudgetDelta)
			}

			if delta := enforce.p99 - observe.p99; delta > p99BudgetDelta {
				t.Errorf("corpus gate added %v to p99 on %s (OBSERVE %v → ENFORCE %v), budget %v. "+
					"This size of tail regression means the gate is doing real work per request.",
					delta, route.path, observe.p99, enforce.p99, p99BudgetDelta)
			}
		})
	}
}

type corpusLatency struct {
	p50, p95, p99 time.Duration
	n             int
	failures      int
}

// corpusMeasure drives concurrent load against one route and returns the
// latency distribution. Failures are counted rather than fatal so the caller
// can distinguish "the gate is slow" from "the gate refused us".
func corpusMeasure(t *testing.T, base, bearer string, route corpusEnforceRoute, concurrency, perWorker int) corpusLatency {
	t.Helper()

	var (
		mu       sync.Mutex
		samples  []time.Duration
		failures int
		wg       sync.WaitGroup
	)

	client := &http.Client{Timeout: 30 * time.Second}
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				req, err := http.NewRequest(route.method, base+route.path, nil)
				if err != nil {
					mu.Lock()
					failures++
					mu.Unlock()
					continue
				}
				req.Header.Set("Authorization", "Bearer "+bearer)
				start := time.Now()
				resp, err := client.Do(req)
				elapsed := time.Since(start)
				mu.Lock()
				if err != nil {
					failures++
				} else {
					if resp.StatusCode == http.StatusForbidden {
						failures++
					}
					samples = append(samples, elapsed)
					resp.Body.Close()
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(samples) == 0 {
		t.Fatalf("no latency samples collected for %s — the load run produced nothing to measure", route.path)
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return corpusLatency{
		p50:      corpusPercentile(samples, 0.50),
		p95:      corpusPercentile(samples, 0.95),
		p99:      corpusPercentile(samples, 0.99),
		n:        len(samples),
		failures: failures,
	}
}

func corpusPercentile(sorted []time.Duration, q float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * q)
	return sorted[idx]
}
