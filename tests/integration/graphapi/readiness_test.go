//go:build integration

// BUG-080-001 SCOPE-03 — T080-04-READY (SCN-080-001-04).
//
// This file proves that the product readiness projection
// (internal/api/graph_readiness.go) derives Snapshot().Ready from
// EXACTLY TWO inputs and nothing else:
//
//	1. the EXPLICIT activation policy (graphapi.GraphCapability), and
//	2. a Validate()-passed, FRESH, published
//	   graphsynthetic.AggregateResult.
//
// It exercises the REAL projection type, the REAL production router
// (internal/api.NewRouter — the router cmd/core builds at boot), the
// REAL embedded static Wiki assets, the REAL PostgreSQL-backed graph
// handlers, and the disposable stack's REAL PostgreSQL over a REAL
// loopback HTTP server. There is NO request interception, NO mock, NO
// stub, and NO t.Skip: a missing live capability FAILS loudly.
//
// The adversarial core is the last two sub-tests:
//
//   - BEHAVIOURAL — with the static Wiki page served (200), the full
//     graph route manifest mounted and serving live Postgres rows
//     (200), and general database liveness green (/readyz -> 200), the
//     Graph journey is STILL not ready (/readyz?strict=true -> 503)
//     until a synthetic aggregate is published. The ONLY variable that
//     changes between the not-ready and ready assertions is the
//     synthetic publication; the static assets and the database stay
//     green the whole time. A regression that let static assets or DB
//     liveness imply readiness fails here.
//
//   - STRUCTURAL — an AST scan of graph_readiness.go proves there is no
//     THIRD assignment path to Ready: every composite-literal
//     construction is `Ready: false` (fail closed) and there is EXACTLY
//     ONE assignment, whose right-hand side is the aggregate's own
//     Available() call. Adding `section.Ready = true` on a
//     static-asset, route-presence, or DB-liveness branch fails this
//     assertion even if a behavioural test were somehow satisfied.

package graphapi_integration

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/api/graphapi"
	"github.com/smackerel/smackerel/internal/db"
	"github.com/smackerel/smackerel/internal/graphsynthetic"
)

// readinessDisabledCapability resolves an explicitly DISABLED activation
// policy from a cursor-secret env var that is never set — the same
// unavailable-enabler trigger cmd/core/wiring.go classifies at boot.
func readinessDisabledCapability(t *testing.T, envSuffix string) *graphapi.GraphCapability {
	t.Helper()
	cfg := graphActivationLimits()
	cfg.CursorSecretEnv = "KNOWLEDGE_GRAPH_API_CURSOR_SECRET_BUG080IT_READY_" + envSuffix + "_DO_NOT_SET"
	graphCap := graphapi.NewGraphCapability(cfg)
	if !graphCap.Disabled() {
		t.Fatalf("test setup: expected a DISABLED capability from an unset cursor-secret env var")
	}
	if got := graphCap.Activation().State; got != graphapi.ActivationDisabled {
		t.Fatalf("test setup: activation state = %q; want %q", got, graphapi.ActivationDisabled)
	}
	return graphCap
}

// readinessFamilyRows builds one contract-valid row per canonical family
// in canonical order, with the evidence reference DERIVED from the
// family name exactly as graphsynthetic.Validate requires.
func readinessFamilyRows(state graphsynthetic.ReadState, code string) []graphsynthetic.GraphFamilyResult {
	families := graphapi.RequiredGraphFamilies()
	rows := make([]graphsynthetic.GraphFamilyResult, 0, len(families))
	for _, family := range families {
		rows = append(rows, graphsynthetic.GraphFamilyResult{
			Family:      family,
			State:       state,
			DurationMs:  7,
			Code:        code,
			EvidenceRef: graphsynthetic.EvidenceRef(family),
		})
	}
	return rows
}

// readinessAvailableAggregate is an ENABLED-activation aggregate whose
// every required family returned a contract-valid populated read — the
// ONLY shape AggregateResult.Available() reports true for.
func readinessAvailableAggregate(t *testing.T, observedAt time.Time) graphsynthetic.AggregateResult {
	t.Helper()
	result := graphsynthetic.AggregateResult{
		Activation:  graphapi.ActivationEnabled,
		State:       graphsynthetic.AggregateAvailable,
		ObservedAt:  observedAt.UTC(),
		DurationMs:  42,
		Code:        graphsynthetic.CodeOK,
		EvidenceRef: graphsynthetic.AggregateEvidenceRef,
		Families:    readinessFamilyRows(graphsynthetic.StatePopulated, graphsynthetic.CodeOK),
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("test setup: available aggregate fixture is not contract-valid: %v", err)
	}
	if !result.Available() {
		t.Fatalf("test setup: available aggregate fixture does not report Available()")
	}
	return result
}

// readinessPolicyDisabledAggregate is a DISABLED-activation aggregate.
// It is contract-valid, so a publication refusal against an ENABLED
// policy can only come from the activation-agreement check.
func readinessPolicyDisabledAggregate(t *testing.T, observedAt time.Time) graphsynthetic.AggregateResult {
	t.Helper()
	result := graphsynthetic.AggregateResult{
		Activation:  graphapi.ActivationDisabled,
		State:       graphsynthetic.AggregatePolicyDisabled,
		ObservedAt:  observedAt.UTC(),
		DurationMs:  3,
		Code:        graphsynthetic.CodePolicyDisabled,
		EvidenceRef: graphsynthetic.AggregateEvidenceRef,
		Families:    readinessFamilyRows(graphsynthetic.StateDisabled, graphsynthetic.CodePolicyDisabled),
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("test setup: policy-disabled aggregate fixture is not contract-valid: %v", err)
	}
	return result
}

// newReadiness constructs the REAL projection with the product-owned
// freshness bound. Construction is fail-loud, so an error here is a
// genuine failure and never a skip.
func newReadiness(t *testing.T, graphCap *graphapi.GraphCapability) *api.GraphReadiness {
	t.Helper()
	readiness, err := api.NewGraphReadiness(graphCap, api.GraphObservationMaxAge)
	if err != nil {
		t.Fatalf("api.NewGraphReadiness: %v", err)
	}
	return readiness
}

// assertReadinessFamiliesCanonical proves the projected family rows are
// the canonical eight in canonical order with derived evidence refs.
func assertReadinessFamiliesCanonical(t *testing.T, label string, rows []graphsynthetic.GraphFamilyResult) {
	t.Helper()
	want := graphapi.RequiredGraphFamilies()
	if len(rows) != len(want) {
		t.Fatalf("%s: projected %d family rows; the canonical manifest requires exactly %d", label, len(rows), len(want))
	}
	for i, family := range want {
		if rows[i].Family != family {
			t.Fatalf("%s: family row %d is %q; canonical order requires %q", label, i, rows[i].Family, family)
		}
		if got, expect := rows[i].EvidenceRef, graphsynthetic.EvidenceRef(family); got != expect {
			t.Fatalf("%s: family row %d evidence ref %q; MUST be derived as %q", label, i, got, expect)
		}
	}
}

// readyzBody is the aggregate-only strict-readiness wire shape.
type readyzBody struct {
	Ready bool `json:"ready"`
}

// readinessGET issues a GET against the in-process server and returns
// the status and body. Any transport failure is a hard failure.
func readinessGET(t *testing.T, base, path string) (int, []byte) {
	t.Helper()
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Get(base + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("GET %s: read body: %v", path, err)
	}
	return resp.StatusCode, body
}

// readinessStrictReadyz drives GET /readyz?strict=true and returns the
// status plus the decoded aggregate-only boolean.
func readinessStrictReadyz(t *testing.T, base string) (int, bool) {
	t.Helper()
	status, body := readinessGET(t, base, "/readyz?strict=true")
	var decoded readyzBody
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("GET /readyz?strict=true: body is not the aggregate-only readiness shape: %v (body=%q)", err, string(body))
	}
	return status, decoded.Ready
}

// readinessHealthGraph drives the AUTHENTICATED GET /api/health and
// returns the Knowledge Graph capability projection. AuthToken is empty
// in this in-process wiring, so isAuthenticated resolves true and the
// capability detail is present — its absence is a hard failure.
func readinessHealthGraph(t *testing.T, base string) api.HealthResponse {
	t.Helper()
	status, body := readinessGET(t, base, "/api/health")
	if status != http.StatusOK {
		t.Fatalf("GET /api/health -> %d; want 200 (liveness is always 200 without ?strict). body=%s", status, string(body))
	}
	var decoded api.HealthResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("GET /api/health: decode: %v (body=%s)", err, string(body))
	}
	if decoded.Graph == nil {
		t.Fatalf("GET /api/health: authenticated response carries NO graph capability section; the readiness projection is not wired. body=%s", string(body))
	}
	return decoded
}

// TestGraphReadinessUsesSyntheticAndExplicitActivation is the
// T080-04-READY / SCN-080-001-04 proof: strict Graph readiness and the
// authenticated Graph capability detail are derived from the explicit
// activation policy plus a fresh, validated synthetic aggregate — and
// from nothing else.
func TestGraphReadinessUsesSyntheticAndExplicitActivation(t *testing.T) {
	// (1) ENABLED policy + a fresh published AVAILABLE aggregate is the
	// one and only route to a ready Graph journey.
	t.Run("enabled_policy_with_fresh_available_aggregate_is_ready", func(t *testing.T) {
		graphCap, _, _ := enabledGraphWiring(t, "READY_FRESH")
		readiness := newReadiness(t, graphCap)

		if err := readiness.Publish(readinessAvailableAggregate(t, time.Now().UTC())); err != nil {
			t.Fatalf("Publish(available aggregate): %v", err)
		}

		got := readiness.Snapshot()
		if !got.Ready {
			t.Fatalf("Ready = false with an ENABLED policy and a fresh available aggregate; the proven-ready path is broken. snapshot=%+v", got)
		}
		if got.Activation != string(graphapi.ActivationEnabled) {
			t.Fatalf("Activation = %q; want %q", got.Activation, graphapi.ActivationEnabled)
		}
		if got.State != string(graphsynthetic.AggregateAvailable) {
			t.Fatalf("State = %q; want %q", got.State, graphsynthetic.AggregateAvailable)
		}
		if got.Code != graphsynthetic.CodeOK {
			t.Fatalf("Code = %q; want %q", got.Code, graphsynthetic.CodeOK)
		}
		if got.EvidenceRef != graphsynthetic.AggregateEvidenceRef {
			t.Fatalf("EvidenceRef = %q; want the constant %q", got.EvidenceRef, graphsynthetic.AggregateEvidenceRef)
		}
		if got.ObservedAt == nil || got.DurationMs == nil {
			t.Fatalf("a published observation MUST project its observation instant and bounded duration; got observedAt=%v durationMs=%v", got.ObservedAt, got.DurationMs)
		}
		assertReadinessFamiliesCanonical(t, "ready snapshot", got.Families)
		t.Logf("ready snapshot: activation=%s state=%s code=%s families=%d ready=%t",
			got.Activation, got.State, got.Code, len(got.Families), got.Ready)
	})

	// (2) ENABLED policy with NO observation fails CLOSED: route presence
	// alone never promotes the Graph journey to ready.
	t.Run("enabled_policy_without_observation_is_not_ready", func(t *testing.T) {
		graphCap, _, _ := enabledGraphWiring(t, "READY_UNOBSERVED")
		readiness := newReadiness(t, graphCap)

		got := readiness.Snapshot()
		if got.Ready {
			t.Fatalf("Ready = true with an ENABLED policy and ZERO published observations; readiness is being inferred from something other than the synthetic. snapshot=%+v", got)
		}
		if got.Code != api.GraphReadinessCodeNotObserved {
			t.Fatalf("Code = %q; want %q for an ENABLED policy that has never been observed", got.Code, api.GraphReadinessCodeNotObserved)
		}
		if got.State != string(graphsynthetic.AggregateUnavailable) {
			t.Fatalf("State = %q; want %q", got.State, graphsynthetic.AggregateUnavailable)
		}
		if got.Activation != string(graphapi.ActivationEnabled) {
			t.Fatalf("Activation = %q; want %q — the policy is enabled even though the journey is unproven", got.Activation, graphapi.ActivationEnabled)
		}
		if got.ObservedAt != nil || got.DurationMs != nil || len(got.Families) != 0 {
			t.Fatalf("an unobserved projection MUST NOT carry observation detail; got observedAt=%v durationMs=%v families=%d", got.ObservedAt, got.DurationMs, len(got.Families))
		}
		t.Logf("unobserved snapshot: activation=%s state=%s code=%s ready=%t",
			got.Activation, got.State, got.Code, got.Ready)
	})

	// (3) A once-available observation MUST expire. The fresh-boundary
	// control in the same sub-test proves the staleness assertion is not
	// tautological — the identical aggregate shape IS ready when recent.
	t.Run("enabled_policy_with_stale_observation_is_not_ready", func(t *testing.T) {
		graphCap, _, _ := enabledGraphWiring(t, "READY_STALE")
		readiness := newReadiness(t, graphCap)

		staleAt := time.Now().UTC().Add(-(api.GraphObservationMaxAge + time.Minute))
		if err := readiness.Publish(readinessAvailableAggregate(t, staleAt)); err != nil {
			t.Fatalf("Publish(stale available aggregate): %v", err)
		}
		stale := readiness.Snapshot()
		if stale.Ready {
			t.Fatalf("Ready = true for an observation older than GraphObservationMaxAge (%s); a stale observation must never keep asserting a ready Graph journey. snapshot=%+v", api.GraphObservationMaxAge, stale)
		}
		if stale.Code != api.GraphReadinessCodeStale {
			t.Fatalf("Code = %q; want %q for an expired observation", stale.Code, api.GraphReadinessCodeStale)
		}
		if stale.State != string(graphsynthetic.AggregateUnavailable) {
			t.Fatalf("State = %q; want %q for an expired observation", stale.State, graphsynthetic.AggregateUnavailable)
		}
		if stale.ObservedAt == nil {
			t.Fatalf("a stale projection MUST still disclose WHEN it was last observed")
		}

		// Fresh-boundary control: same aggregate shape, recent instant.
		freshAt := time.Now().UTC().Add(-(api.GraphObservationMaxAge / 2))
		if err := readiness.Publish(readinessAvailableAggregate(t, freshAt)); err != nil {
			t.Fatalf("Publish(fresh available aggregate): %v", err)
		}
		fresh := readiness.Snapshot()
		if !fresh.Ready {
			t.Fatalf("control failed: the SAME aggregate shape within GraphObservationMaxAge (%s) is not ready, so the staleness assertion above proves nothing. snapshot=%+v", api.GraphObservationMaxAge, fresh)
		}
		t.Logf("staleness: maxAge=%s stale(code=%s ready=%t) -> fresh(code=%s ready=%t)",
			api.GraphObservationMaxAge, stale.Code, stale.Ready, fresh.Code, fresh.Ready)
	})

	// (4) An explicitly DISABLED policy is a TRUTHFUL non-ready
	// deployment state — never a fault, and never ready.
	t.Run("disabled_policy_is_truthful_non_ready_and_not_a_fault", func(t *testing.T) {
		graphCap := readinessDisabledCapability(t, "DISABLED")
		readiness := newReadiness(t, graphCap)

		got := readiness.Snapshot()
		if got.Ready {
			t.Fatalf("Ready = true under an explicitly DISABLED activation policy. snapshot=%+v", got)
		}
		if got.Activation != string(graphapi.ActivationDisabled) {
			t.Fatalf("Activation = %q; want %q", got.Activation, graphapi.ActivationDisabled)
		}
		if got.State != string(graphsynthetic.AggregatePolicyDisabled) {
			t.Fatalf("State = %q; want %q — a disabled deployment is policy_disabled, not a generic unavailable fault", got.State, graphsynthetic.AggregatePolicyDisabled)
		}
		if got.Code != graphsynthetic.CodePolicyDisabled {
			t.Fatalf("Code = %q; want %q", got.Code, graphsynthetic.CodePolicyDisabled)
		}
		for _, faultCode := range []string{
			api.GraphReadinessCodeNotObserved,
			api.GraphReadinessCodeStale,
			api.GraphReadinessCodeActivationMismatch,
			api.GraphReadinessCodeConfigInvalid,
		} {
			if got.Code == faultCode {
				t.Fatalf("Code = %q: a deliberately disabled deployment is being reported as a projection FAULT instead of a truthful deployment state", got.Code)
			}
		}

		// A contract-valid disabled observation agrees with the policy
		// and is accepted, and the answer stays exactly as truthful.
		if err := readiness.Publish(readinessPolicyDisabledAggregate(t, time.Now().UTC())); err != nil {
			t.Fatalf("Publish(policy-disabled aggregate under a DISABLED policy): %v", err)
		}
		observed := readiness.Snapshot()
		if observed.Ready {
			t.Fatalf("Ready = true after publishing a policy-disabled observation. snapshot=%+v", observed)
		}
		if observed.State != string(graphsynthetic.AggregatePolicyDisabled) || observed.Code != graphsynthetic.CodePolicyDisabled {
			t.Fatalf("observed disabled snapshot state=%q code=%q; want %q / %q",
				observed.State, observed.Code, graphsynthetic.AggregatePolicyDisabled, graphsynthetic.CodePolicyDisabled)
		}
		t.Logf("disabled snapshot: activation=%s state=%s code=%s ready=%t (truthful deployment state, not a fault)",
			observed.Activation, observed.State, observed.Code, observed.Ready)
	})

	// (5) An observation that CONTRADICTS the configured policy is
	// REFUSED — in both directions — and never becomes product truth.
	t.Run("publication_disagreeing_with_the_policy_is_refused", func(t *testing.T) {
		t.Run("disabled_observation_under_enabled_policy", func(t *testing.T) {
			graphCap, _, _ := enabledGraphWiring(t, "READY_MISMATCH_ENABLED")
			readiness := newReadiness(t, graphCap)

			err := readiness.Publish(readinessPolicyDisabledAggregate(t, time.Now().UTC()))
			if err == nil {
				t.Fatalf("Publish accepted a DISABLED-activation observation under an ENABLED policy; a contradicting observation must never become product truth")
			}
			if !strings.Contains(err.Error(), api.GraphReadinessCodeActivationMismatch) {
				t.Fatalf("refusal error %q does not carry the closed code %q", err.Error(), api.GraphReadinessCodeActivationMismatch)
			}
			got := readiness.Snapshot()
			if got.Ready {
				t.Fatalf("Ready = true after a REFUSED publication. snapshot=%+v", got)
			}
			if got.Code != api.GraphReadinessCodeNotObserved {
				t.Fatalf("Code = %q after a refused publication; the projection MUST remain unobserved (%q), not absorb the refused observation", got.Code, api.GraphReadinessCodeNotObserved)
			}
			t.Logf("refused (enabled policy): %v", err)
		})

		t.Run("enabled_observation_under_disabled_policy", func(t *testing.T) {
			graphCap := readinessDisabledCapability(t, "MISMATCH")
			readiness := newReadiness(t, graphCap)

			err := readiness.Publish(readinessAvailableAggregate(t, time.Now().UTC()))
			if err == nil {
				t.Fatalf("Publish accepted an ENABLED-activation AVAILABLE observation under a DISABLED policy; that is the exact route by which a contradicting observation could force a false ready claim")
			}
			if !strings.Contains(err.Error(), api.GraphReadinessCodeActivationMismatch) {
				t.Fatalf("refusal error %q does not carry the closed code %q", err.Error(), api.GraphReadinessCodeActivationMismatch)
			}
			got := readiness.Snapshot()
			if got.Ready {
				t.Fatalf("Ready = true after a REFUSED available observation under a DISABLED policy. snapshot=%+v", got)
			}
			if got.State != string(graphsynthetic.AggregatePolicyDisabled) {
				t.Fatalf("State = %q after a refused publication; want %q", got.State, graphsynthetic.AggregatePolicyDisabled)
			}
			t.Logf("refused (disabled policy): %v", err)
		})
	})

	// (6) ADVERSARIAL, BEHAVIOURAL — static Wiki assets, a fully mounted
	// graph route manifest serving live Postgres rows, and green general
	// database liveness are ALL true here, and the Graph journey is
	// STILL not strictly ready until the synthetic publishes. The only
	// variable across the not-ready/ready assertions is that
	// publication.
	t.Run("static_wiki_and_green_database_liveness_cannot_make_graph_ready", func(t *testing.T) {
		dbURL := os.Getenv("DATABASE_URL")
		if dbURL == "" {
			t.Fatalf("integration: DATABASE_URL is unset — this row REQUIRES the live disposable stack (run `./smackerel.sh test integration`); it must fail loudly rather than silently pass")
		}
		ctx := context.Background()
		pg, err := db.Connect(ctx, dbURL, 4, 1)
		if err != nil {
			t.Fatalf("db.Connect: %v", err)
		}
		t.Cleanup(pg.Close)
		if !pg.Healthy(ctx) {
			t.Fatalf("live PostgreSQL reports unhealthy; this row needs a green database so that green liveness can be proven INSUFFICIENT for graph readiness")
		}

		graphCap, codec, limits := enabledGraphWiring(t, "READY_LIVE")
		readiness := newReadiness(t, graphCap)

		// The REAL production router: real DB, real embedded static
		// assets, real PostgreSQL-backed graph handlers, real readiness.
		srv := httptest.NewServer(api.NewRouter(&api.Dependencies{
			Environment:     "test",
			AuthToken:       "",
			DB:              pg,
			GraphCapability: graphCap,
			GraphReadiness:  readiness,
			TopicsHandlers:  graphapi.NewTopicsHandlers(pg.Pool, limits, codec),
			PeopleHandlers:  graphapi.NewPeopleHandlers(pg.Pool, limits, codec),
			PlacesHandlers:  graphapi.NewPlacesHandlers(pg.Pool, limits, codec),
			TimeHandlers:    graphapi.NewTimeHandlers(pg.Pool, limits),
			EdgesHandlers:   graphapi.NewEdgesHandlers(pg.Pool, limits, codec),
		}))
		t.Cleanup(srv.Close)

		// The static Wiki asset IS served — the exact surface
		// GRAPH-ACT-009 forbids from implying a ready Graph journey.
		wikiStatus, wikiBody := readinessGET(t, srv.URL, "/pwa/wiki.html")
		if wikiStatus != http.StatusOK {
			t.Fatalf("GET /pwa/wiki.html -> %d; want 200 — the adversarial premise REQUIRES the static Wiki asset to be served", wikiStatus)
		}
		if len(wikiBody) == 0 {
			t.Fatalf("GET /pwa/wiki.html served an empty body; the static Wiki premise is not established")
		}

		// The graph route manifest is mounted and serving live Postgres.
		topicsStatus, topicsBody := readinessGET(t, srv.URL, "/api/topics?limit=1")
		if topicsStatus != http.StatusOK {
			t.Fatalf("GET /api/topics?limit=1 -> %d; want 200 — the adversarial premise REQUIRES the graph routes mounted and serving. body=%s", topicsStatus, string(topicsBody))
		}

		// General database liveness is green on the default probe.
		plainStatus, plainBody := readinessGET(t, srv.URL, "/readyz")
		if plainStatus != http.StatusOK {
			t.Fatalf("GET /readyz -> %d; want 200 — the adversarial premise REQUIRES green general liveness. body=%s", plainStatus, string(plainBody))
		}
		var plain readyzBody
		if err := json.Unmarshal(plainBody, &plain); err != nil || !plain.Ready {
			t.Fatalf("GET /readyz body=%q; want {\"ready\":true} (decode err=%v)", string(plainBody), err)
		}

		// THE PROOF: every green signal above is present, and the Graph
		// journey is still NOT strictly ready.
		strictStatus, strictReady := readinessStrictReadyz(t, srv.URL)
		if strictStatus != http.StatusServiceUnavailable || strictReady {
			t.Fatalf("GET /readyz?strict=true -> %d ready=%t while the static Wiki page serves 200, the graph routes serve 200 from live Postgres, and /readyz is green: readiness is being derived from static assets, route presence, or database liveness instead of the synthetic",
				strictStatus, strictReady)
		}

		health := readinessHealthGraph(t, srv.URL)
		if pgStatus, ok := health.Services["postgres"]; !ok || pgStatus.Status != "up" {
			t.Fatalf("health services[postgres] = %+v (present=%t); the adversarial premise REQUIRES a green database in the SAME response that reports the Graph journey unready", pgStatus, ok)
		}
		if health.Graph.Ready {
			t.Fatalf("authenticated /api/health reports graph.ready=true with zero published observations. graph=%+v", *health.Graph)
		}
		if health.Graph.Code != api.GraphReadinessCodeNotObserved {
			t.Fatalf("authenticated /api/health graph.code = %q; want %q", health.Graph.Code, api.GraphReadinessCodeNotObserved)
		}
		t.Logf("green-but-unready: wiki=%d topics=%d readyz=%d strict=%d postgres=%s graph.ready=%t graph.code=%s",
			wikiStatus, topicsStatus, plainStatus, strictStatus,
			health.Services["postgres"].Status, health.Graph.Ready, health.Graph.Code)

		// Change ONE thing — publish the synthetic observation. Static
		// assets, graph routes, and database liveness are untouched.
		if err := readiness.Publish(readinessAvailableAggregate(t, time.Now().UTC())); err != nil {
			t.Fatalf("Publish(available aggregate): %v", err)
		}

		strictStatusAfter, strictReadyAfter := readinessStrictReadyz(t, srv.URL)
		if strictStatusAfter != http.StatusOK || !strictReadyAfter {
			t.Fatalf("GET /readyz?strict=true -> %d ready=%t after a fresh available synthetic observation; want 200 ready=true — the synthetic is not the input strict readiness reads", strictStatusAfter, strictReadyAfter)
		}
		healthAfter := readinessHealthGraph(t, srv.URL)
		if !healthAfter.Graph.Ready {
			t.Fatalf("authenticated /api/health graph.ready=false after a fresh available observation. graph=%+v", *healthAfter.Graph)
		}
		if healthAfter.Graph.State != string(graphsynthetic.AggregateAvailable) || healthAfter.Graph.Code != graphsynthetic.CodeOK {
			t.Fatalf("authenticated /api/health graph state=%q code=%q; want %q / %q",
				healthAfter.Graph.State, healthAfter.Graph.Code, graphsynthetic.AggregateAvailable, graphsynthetic.CodeOK)
		}
		assertReadinessFamiliesCanonical(t, "live health graph section", healthAfter.Graph.Families)
		t.Logf("after synthetic publication (nothing else changed): strict=%d graph.ready=%t graph.state=%s graph.code=%s families=%d",
			strictStatusAfter, healthAfter.Graph.Ready, healthAfter.Graph.State, healthAfter.Graph.Code, len(healthAfter.Graph.Families))
	})

	// (7) ADVERSARIAL, STRUCTURAL — there is no THIRD assignment path.
	t.Run("readiness_derivation_has_no_third_ready_assignment_path", func(t *testing.T) {
		srcPath := filepath.Join(readinessRepoRoot(t), "internal", "api", "graph_readiness.go")
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, srcPath, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", srcPath, err)
		}

		var literalSites []string
		var assignSites []string

		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CompositeLit:
				for _, elt := range node.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, ok := kv.Key.(*ast.Ident)
					if !ok || key.Name != "Ready" {
						continue
					}
					site := fset.Position(kv.Pos()).String()
					literalSites = append(literalSites, site)
					value, ok := kv.Value.(*ast.Ident)
					if !ok || value.Name != "false" {
						t.Errorf("%s: composite literal constructs Ready from a non-`false` value; every constructed projection MUST fail closed", site)
					}
				}
			case *ast.AssignStmt:
				for i, lhs := range node.Lhs {
					sel, ok := lhs.(*ast.SelectorExpr)
					if !ok || sel.Sel == nil || sel.Sel.Name != "Ready" {
						continue
					}
					site := fset.Position(sel.Pos()).String()
					assignSites = append(assignSites, site)
					if i >= len(node.Rhs) {
						t.Errorf("%s: multi-value assignment to Ready; the derivation MUST be a single explicit expression", site)
						continue
					}
					call, ok := node.Rhs[i].(*ast.CallExpr)
					if !ok {
						t.Errorf("%s: Ready is assigned from a non-call expression; the ONLY permitted source is the published aggregate's Available() result", site)
						continue
					}
					fun, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || fun.Sel == nil || fun.Sel.Name != "Available" {
						t.Errorf("%s: Ready is assigned from something other than <aggregate>.Available(); a static-asset, route-presence, or database-liveness branch would land here", site)
					}
				}
			}
			return true
		})

		if len(literalSites) == 0 {
			t.Fatalf("%s: found no `Ready:` composite-literal construction; the fail-closed constructions this assertion guards have moved or been removed, so the guard would silently pass", srcPath)
		}
		if len(assignSites) != 1 {
			t.Fatalf("%s: found %d assignment(s) to .Ready at %v; EXACTLY ONE is permitted (section.Ready = observed.Available()). A second assignment is the third-path regression this row exists to catch",
				srcPath, len(assignSites), assignSites)
		}
		t.Logf("ready-assignment audit of %s: %d fail-closed literal construction(s) at %v; exactly 1 assignment at %v, sourced from <aggregate>.Available()",
			filepath.Base(srcPath), len(literalSites), literalSites, assignSites)
	})
}

// readinessRepoRoot walks up from the test working directory to the
// module root so the structural audit can read the real product source.
// A missing module root is a hard failure, never a skip.
func readinessRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod found walking up from the test working directory; cannot locate internal/api/graph_readiness.go to audit the ready-assignment paths")
		}
		dir = parent
	}
}
