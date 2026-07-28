//go:build e2e

// BUG-080-001 SCOPE-03 — the product-owned Knowledge Graph read synthetic,
// e2e-api tier (SCN-080-001-03, row T080-03-SYNTH).
//
// This file drives internal/graphsynthetic against the LIVE disposable stack
// over REAL HTTP with a REAL scoped session (CORE_EXTERNAL_URL +
// SMACKEREL_AUTH_TOKEN, exported by `./smackerel.sh test e2e`). There is NO
// request interception, NO mock, NO stub, and NO canned response anywhere in
// this file — every assertion is driven by the actual running smackerel-core
// container reading actual seeded Postgres rows.
//
// What it proves:
//
//   - The synthetic executes its FIXED family sequence (topics -> topic
//     detail -> people -> person detail -> places -> place detail -> time ->
//     edges) against the live stack and emits EXACTLY ONE value-safe row per
//     canonical family, in canonical order, plus ONE aggregate observation.
//     A missing family row is structurally impossible to pass.
//
//   - Acceptance requires EVERY authenticated family read to be
//     contract-valid. A 401, 403, 404, 5xx, schema, cursor, or missing-row
//     outcome fails the family, and a failed REQUIRED family makes the
//     aggregate unavailable.
//
//   - The adversarial half: the SAME synthetic, SAME seeded fixtures, SAME
//     configuration, but an INVALID bearer token MUST NOT produce an
//     available aggregate. This is what makes the acceptance arm non-
//     tautological — it fails on a genuine 401/403 rather than passing
//     regardless of authentication.
//
// VALUE SAFETY: every diagnostic this file emits is drawn from the closed
// vocabularies only (canonical family name, read state, diagnostic code,
// evidence reference, duration). The bearer token, the cursor secret, row
// ids, labels, and cursor bodies are NEVER logged — GraphFamilyResult and
// AggregateResult have no field capable of carrying them, and this file adds
// none.
//
// Emptiness policy (Config.AllowEmptyFamilies) names ONLY the families whose
// population this test cannot deterministically control on a live stack:
//
//   - places / place_detail — the shared e2e harness exposes no place seeder,
//     so a live stack may genuinely hold zero places.
//   - edges — the synthetic seeds its edges read from the FIRST row of
//     /api/topics/, and that ordering (momentum_score DESC, capture_count
//     DESC, id ASC) is server policy across ALL topics, so the first topic is
//     not guaranteed to be one this test seeded.
//
// That allowance is NOT a bypass: a permitted true-empty still requires HTTP
// 200 with a schema-valid, explicitly non-null envelope. A 401/403/404/5xx/
// schema/cursor outcome is StateFailed for an allow-empty family exactly as
// it is for any other, which is precisely why the rejection arm below still
// has teeth. topics, topic_detail, people, person_detail and time are seeded
// by this test and are therefore NOT permitted to be empty.
//
// OptionalFamilies is deliberately EMPTY: no family may be silently excused,
// and a degraded aggregate is therefore unreachable here.

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/smackerel/smackerel/internal/api/graphapi"
	"github.com/smackerel/smackerel/internal/graphsynthetic"
)

// graphSynthFamilyTriples renders the per-family outcome as value-safe
// `family=state=code` triples for a failure message. Every component is a
// CLOSED-vocabulary value: a canonical family name, a closed read state, and
// a closed diagnostic code. A row id, a label, a query value, a cursor body,
// a credential, and a target host structurally cannot appear here.
func graphSynthFamilyTriples(rows []graphsynthetic.GraphFamilyResult) string {
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		parts = append(parts, string(row.Family)+"="+string(row.State)+"="+row.Code)
	}
	return strings.Join(parts, " ")
}

func TestE2E_ProductSyntheticRequiresEveryAuthenticatedFamilyRead_T080_03_SYNTH(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)
	dbURL := requireEnvForGraphAPI(t)

	conn, err := pgx.Connect(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("pgx.Connect: %v", err)
	}
	// Ordering matters and is load-bearing: t.Cleanup runs LIFO, so the
	// connection close is registered FIRST in order to run LAST. Registering
	// it later (or using `defer conn.Close(...)`, which always runs BEFORE
	// every t.Cleanup) would close the connection while the fixture DELETEs
	// were still pending, turning cleanup into a silent no-op and leaking
	// rows into the shared graph tables.
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	prefix := "graph-synth-e2e-" + time.Now().UTC().Format("20060102150405.000000")
	t.Cleanup(func() { graphAPICleanup(t, conn, prefix) })

	// Real, disposable rows so the graph families are GENUINELY populated
	// rather than passing on an unexercised empty read.
	topicIDs := graphAPISeedTopics(t, conn, prefix, 2)
	personIDs := graphAPISeedPeople(t, conn, prefix, 1)
	artifactIDs := graphAPISeedArtifacts(t, conn, prefix, 3)
	for _, artifactID := range artifactIDs {
		graphAPISeedEdge(t, conn, prefix, "artifact", artifactID, "topic", topicIDs[0], "mentions", 1.0)
		graphAPISeedEdge(t, conn, prefix, "artifact", artifactID, "person", personIDs[0], "mentions", 1.0)
	}
	// A topic-sourced edge as well: GET /api/graph/edges?source=topic:<id>
	// matches on src_type/src_id, so an artifact->topic edge alone would
	// leave the edges family unexercised for a topic seed.
	graphAPISeedEdge(t, conn, prefix, "topic", topicIDs[0], "person", personIDs[0], "co-occurs", 1.0)
	graphAPISeedEdge(t, conn, prefix, "topic", topicIDs[1], "person", personIDs[0], "co-occurs", 1.0)

	// Make the edges read DETERMINISTIC rather than allow-empty.
	//
	// The synthetic seeds its edges request from the FIRST row of
	// GET /api/topics/, which internal/api/graphapi/topics.go orders by
	// `momentum_score DESC NULLS LAST, capture_count_total DESC NULLS LAST,
	// id ASC`. graphAPISeedTopics assigns momentum 1.0/1.1, so a
	// pre-existing higher-momentum topic in the shared test database would
	// win the ordering and hand the synthetic a seed id with no topic-sourced
	// edge — silently turning the edges family into a zero-row read.
	//
	// Promoting the seeded topic above the current maximum removes that
	// ambiguity, so `edges` is genuinely POPULATED and does NOT need to be
	// excused via AllowEmptyFamilies. This keeps the acceptance arm honest:
	// an edges read that returns zero rows is a FAILURE here, not a pass.
	if _, err := conn.Exec(context.Background(),
		`UPDATE topics
		    SET momentum_score = COALESCE((SELECT MAX(momentum_score) FROM topics), 0) + 1,
		        capture_count_total = COALESCE((SELECT MAX(capture_count_total) FROM topics), 0) + 1
		  WHERE id = $1`, topicIDs[0]); err != nil {
		t.Fatalf("promote seeded topic to first position in the momentum ordering: %v", err)
	}

	// An explicit bounded UTC window that brackets the seeded artifacts
	// (graphAPISeedArtifacts backdates them across the preceding days).
	now := time.Now().UTC()
	synthCfg := graphsynthetic.Config{
		BaseURL:        cfg.CoreURL,
		BearerToken:    cfg.AuthToken,
		WindowFrom:     now.Add(-30 * 24 * time.Hour),
		WindowTo:       now.Add(1 * time.Hour),
		RequestTimeout: 15 * time.Second,
		EdgeSourceKind: "topic",
		// ONLY the two families this harness structurally cannot seed: the
		// places family reads from location_clusters / maps_places /
		// artifact_places, and no seeder for those tables exists anywhere in
		// tests/ or internal/. Every other family — topics, topic_detail,
		// people, person_detail, time, AND edges — is genuinely seeded above
		// and a zero-row read for any of them FAILS acceptance.
		AllowEmptyFamilies: []graphapi.GraphRouteFamily{
			graphapi.FamilyPlaces,
			graphapi.FamilyPlaceDetail,
		},
		// Deliberately empty: nothing is silently excused, so a degraded
		// aggregate is unreachable and only a genuinely available aggregate
		// can satisfy the acceptance arm below.
		OptionalFamilies: []graphapi.GraphRouteFamily{},
		HTTPClient:       &http.Client{Timeout: 20 * time.Second},
	}

	// The explicit ENABLED activation policy this observation runs under.
	activation := graphapi.Activation{
		State:          graphapi.ActivationEnabled,
		SecretPresence: graphapi.SecretPresent,
		Code:           graphapi.CodeActivationOK,
	}

	required := graphapi.RequiredGraphFamilies()

	t.Run("Regression: product synthetic requires every authenticated family read (acceptance)", func(t *testing.T) {
		synth, err := graphsynthetic.New(synthCfg, graphsynthetic.NopObserver{})
		if err != nil {
			t.Fatalf("graphsynthetic.New: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		agg, err := synth.Run(ctx, activation)
		if err != nil {
			t.Fatalf("Synthetic.Run against the live stack: %v", err)
		}

		if err := agg.Validate(); err != nil {
			t.Fatalf("aggregate failed its own closed-vocabulary contract: %v", err)
		}

		// EXACTLY one row per canonical family, in canonical order. This is
		// the assertion that makes a missing family row impossible to pass.
		if len(agg.Families) != len(required) {
			t.Fatalf("aggregate carries %d family rows; the canonical manifest requires exactly %d (families: %s)",
				len(agg.Families), len(required), graphSynthFamilyTriples(agg.Families))
		}
		for i, want := range required {
			row := agg.Families[i]
			if row.Family != want {
				t.Errorf("family row %d is %q; canonical order requires %q", i, row.Family, want)
				continue
			}
			if err := row.Validate(); err != nil {
				t.Errorf("family row %d (%s) failed its closed-vocabulary contract: %v", i, row.Family, err)
			}
			if wantRef := graphsynthetic.EvidenceRef(row.Family); row.EvidenceRef != wantRef {
				t.Errorf("family %s carries evidenceRef %q; it MUST be derived from the family name as %q",
					row.Family, row.EvidenceRef, wantRef)
			}
			if row.DurationMs < 0 {
				t.Errorf("family %s carries a negative duration %d", row.Family, row.DurationMs)
			}
		}

		if agg.EvidenceRef != graphsynthetic.AggregateEvidenceRef {
			t.Errorf("aggregate carries evidenceRef %q; it MUST be the constant %q",
				agg.EvidenceRef, graphsynthetic.AggregateEvidenceRef)
		}
		if agg.Activation != graphapi.ActivationEnabled {
			t.Errorf("aggregate carries activation %q; this observation ran under %q",
				agg.Activation, graphapi.ActivationEnabled)
		}
		if agg.ObservedAt.IsZero() {
			t.Errorf("aggregate carries a zero observation time; a real observation MUST be timestamped")
		}

		// ACCEPTANCE ARM. The aggregate becomes available ONLY when every
		// REQUIRED family returned a contract-valid populated or explicitly
		// permitted true-empty read over real HTTP with a real session.
		if !agg.Available() || agg.State != graphsynthetic.AggregateAvailable || agg.Code != graphsynthetic.CodeOK {
			t.Fatalf("authenticated graph journey did not reach an available aggregate: available=%t state=%q code=%q; per-family family=state=code: %s",
				agg.Available(), agg.State, agg.Code, graphSynthFamilyTriples(agg.Families))
		}
	})

	t.Run("Regression: product synthetic requires every authenticated family read (rejects unauthenticated reads)", func(t *testing.T) {
		// ADVERSARIAL ARM. Identical fixtures, identical configuration —
		// ONLY the credential is invalid. If acceptance could pass without a
		// genuinely authorized read, this arm would also report available,
		// and it must not.
		badCfg := synthCfg
		badCfg.BearerToken = cfg.AuthToken + "-invalid"
		badCfg.HTTPClient = &http.Client{Timeout: 20 * time.Second}

		synth, err := graphsynthetic.New(badCfg, graphsynthetic.NopObserver{})
		if err != nil {
			t.Fatalf("graphsynthetic.New (invalid credential): %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		agg, err := synth.Run(ctx, activation)
		if err != nil {
			t.Fatalf("Synthetic.Run (invalid credential) against the live stack: %v", err)
		}

		if agg.Available() {
			t.Fatalf("an INVALID bearer credential produced an AVAILABLE aggregate; acceptance is not actually gated on authentication: state=%q code=%q; per-family family=state=code: %s",
				agg.State, agg.Code, graphSynthFamilyTriples(agg.Families))
		}
		if agg.State != graphsynthetic.AggregateUnavailable {
			t.Fatalf("an INVALID bearer credential produced aggregate state %q; a rejected required family MUST make the aggregate %q: code=%q; per-family family=state=code: %s",
				agg.State, graphsynthetic.AggregateUnavailable, agg.Code, graphSynthFamilyTriples(agg.Families))
		}

		sawRejection := false
		for _, row := range agg.Families {
			if row.State != graphsynthetic.StateFailed {
				continue
			}
			if row.Code == graphsynthetic.CodeUnauthenticated || row.Code == graphsynthetic.CodeForbidden {
				sawRejection = true
				break
			}
		}
		if !sawRejection {
			t.Fatalf("no family row recorded a %q read with code %q or %q under an INVALID bearer credential; the stack did not reject the request as unauthenticated/forbidden: per-family family=state=code: %s",
				graphsynthetic.StateFailed, graphsynthetic.CodeUnauthenticated, graphsynthetic.CodeForbidden,
				graphSynthFamilyTriples(agg.Families))
		}
	})
}

// TestE2E_StaticWikiAndGreenLivenessCannotSatisfyGraphReadiness_T080_04_STATIC
// is BUG-080-001 SCOPE-03 row T080-04-STATIC (SCN-080-001-04), e2e-api tier.
//
// It proves the negative that the acceptance synthetic above cannot: NEITHER
// the fully-present static Wiki UI NOR green general database liveness can
// promote the Knowledge Graph journey to "ready". Strict readiness is derived
// ONLY from the explicit activation policy plus a fresh PUBLISHED synthetic
// aggregate (internal/api/graph_readiness.go Snapshot), and that derivation
// has no branch capable of reading a served asset, a mounted route, or a
// database handle.
//
// This runs against the LIVE disposable stack over REAL HTTP. There is NO
// request interception, NO mock, NO stub, and NO canned response — every
// assertion is the actual running smackerel-core container answering an
// actual socket.
//
// Why it is genuinely adversarial rather than tautological: sub-tests A and B
// FIRST establish that the two tempting-but-wrong readiness proxies are BOTH
// green — the five Knowledge Graph Wiki pages really are served (HTTP 200,
// non-empty), and /readyz really does report ready:true. Only then does
// sub-test C demand that /readyz?strict=true STILL refuse. If anyone rewired
// readiness to derive from mounted routes, served static files, or DB health,
// the strict arm would flip to 200 and this test would FAIL. Conversely, if
// strict were merely failing for an unrelated blanket reason, the immediate
// re-probe of plain /readyz inside sub-test C would also be red — so the
// refusal is pinned as specifically graph-driven.
//
// The pinned live-stack truth: nothing in production wiring publishes a
// synthetic observation (cmd/core/wiring.go constructs the readiness
// projection; no publisher calls GraphReadiness.Publish). The authenticated
// capability section therefore reports the fail-closed starting state and
// /readyz?strict=true answers 503. That is the CORRECT truthful answer, and
// it is exactly what this test pins.
//
// VALUE SAFETY: every diagnostic emitted here is drawn from CLOSED
// vocabularies only — HTTP status codes, the readiness boolean, the closed
// activation states {enabled,disabled}, the closed aggregate states
// {available,degraded,unavailable,policy_disabled}, the closed F080 diagnostic
// codes, and the constant evidence reference. The bearer token, row ids,
// labels, and cursor bodies are NEVER logged and structurally cannot appear.
//
// The activation state is deliberately NOT asserted to be "enabled": it is
// read from whatever the stack reports and checked against the closed set, so
// this test is correct on BOTH the default stack and the graph-disabled
// override stack. Under a DISABLED policy the projection reports
// policy_disabled with ready=false, which still does not satisfy strict
// readiness — the refusal this test pins holds either way.
func TestE2E_StaticWikiAndGreenLivenessCannotSatisfyGraphReadiness_T080_04_STATIC(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)

	// The REAL Knowledge Graph journey UI assets, served unauthenticated from
	// the embedded PWA bundle mounted at /pwa/* (internal/api/router.go).
	// These are the same paths the existing tests/e2e/wiki suite drives.
	staticWikiPages := []string{
		"/pwa/wiki.html",
		"/pwa/wiki_topics.html",
		"/pwa/wiki_people.html",
		"/pwa/wiki_places.html",
		"/pwa/wiki_time.html",
	}

	// The CLOSED readiness-projection diagnostic vocabulary
	// (internal/api/graph_readiness.go). A code outside this set — or outside
	// the F080-SYNTH-* aggregate family — is rejected rather than tolerated.
	readinessProjectionCodes := map[string]bool{
		"F080-READINESS-NOT-OBSERVED":        true,
		"F080-READINESS-STALE":               true,
		"F080-READINESS-ACTIVATION-MISMATCH": true,
		"F080-READINESS-CONFIG-INVALID":      true,
	}
	// The CLOSED aggregate state vocabulary (internal/graphsynthetic/result.go).
	aggregateStates := map[string]bool{
		string(graphsynthetic.AggregateAvailable):      true,
		string(graphsynthetic.AggregateDegraded):       true,
		string(graphsynthetic.AggregateUnavailable):    true,
		string(graphsynthetic.AggregatePolicyDisabled): true,
	}
	// The CLOSED activation vocabulary (internal/api/graphapi/activation.go).
	activationStates := map[string]bool{
		string(graphapi.ActivationEnabled):  true,
		string(graphapi.ActivationDisabled): true,
	}

	// probeReadyz issues an UNAUTHENTICATED GET against /readyz with the
	// supplied raw query suffix and returns the observed status code plus the
	// decoded `ready` boolean. /readyz is an orchestrator probe and carries no
	// credential, which is precisely the point: an anonymous caller must not
	// be able to coax a ready answer out of static assets or DB liveness.
	probeReadyz := func(t *testing.T, query string) (int, bool) {
		t.Helper()
		path := "/readyz" + query
		resp, err := (&http.Client{Timeout: 15 * time.Second}).Get(cfg.CoreURL + path)
		if err != nil {
			t.Fatalf("unauthenticated GET %s against the live stack: %v", path, err)
		}
		body, err := readBody(resp)
		if err != nil {
			t.Fatalf("read response body of unauthenticated GET %s: %v", path, err)
		}
		var payload struct {
			Ready bool `json:"ready"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("unauthenticated GET %s returned a body that is not the {\"ready\":bool} readiness contract: %v", path, err)
		}
		return resp.StatusCode, payload.Ready
	}

	// Recorded by sub-test C so sub-test D can assert the authenticated
	// capability section and the unauthenticated strict probe tell the SAME
	// story. Sub-test D re-probes independently when C did not get that far,
	// so the consistency assertion can never be silently skipped.
	var strictObserved bool
	var strictStatus int
	var strictReady bool

	t.Run("Regression: static Wiki and green liveness cannot satisfy Graph readiness (static assets are present)", func(t *testing.T) {
		// PRECONDITION ARM. The Graph journey's UI must genuinely be there,
		// otherwise the strict refusal in sub-test C would be proving nothing
		// — a stack with no Wiki pages trivially "does not derive readiness
		// from Wiki pages". A non-200 here is therefore fatal, not tolerable.
		client := &http.Client{Timeout: 15 * time.Second}
		for _, page := range staticWikiPages {
			resp, err := client.Get(cfg.CoreURL + page)
			if err != nil {
				t.Fatalf("unauthenticated GET %s against the live stack: %v", page, err)
			}
			body, err := readBody(resp)
			if err != nil {
				t.Fatalf("read response body of unauthenticated GET %s: %v", page, err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s returned HTTP %d; the Knowledge Graph Wiki UI MUST be fully served for the strict-readiness refusal below to be a meaningful negative rather than a vacuous one",
					page, resp.StatusCode)
			}
			if len(body) == 0 {
				t.Fatalf("GET %s returned HTTP 200 with an EMPTY body; the asset is not genuinely present", page)
			}
		}
	})

	t.Run("Regression: static Wiki and green liveness cannot satisfy Graph readiness (general liveness is green)", func(t *testing.T) {
		// PRECONDITION ARM. General database liveness must genuinely be green,
		// so that the strict refusal below cannot be dismissed as "the stack
		// was simply down".
		status, ready := probeReadyz(t, "")
		if status != http.StatusOK {
			t.Fatalf("unauthenticated GET /readyz returned HTTP %d; general liveness MUST be green for the strict-readiness refusal below to be a meaningful negative", status)
		}
		if !ready {
			t.Fatalf("unauthenticated GET /readyz returned HTTP 200 with ready=false; the general liveness contract is {\"ready\":true} on 200")
		}
	})

	t.Run("Regression: static Wiki and green liveness cannot satisfy Graph readiness (strict readiness still refuses)", func(t *testing.T) {
		// ADVERSARIAL CORE. Static Wiki assets are proven served (sub-test A)
		// and general liveness is proven green (sub-test B). Strict readiness
		// MUST STILL refuse, because it is derived ONLY from the explicit
		// activation policy plus a fresh published synthetic aggregate.
		status, ready := probeReadyz(t, "?strict=true")
		strictObserved = true
		strictStatus = status
		strictReady = ready

		if status == http.StatusOK || ready {
			t.Fatalf("GET /readyz?strict=true returned HTTP %d with ready=%t; static Wiki assets and green database liveness were ALLOWED to satisfy Knowledge Graph readiness. Strict readiness MUST derive ONLY from the explicit activation policy plus a fresh published synthetic aggregate, and MUST fail closed otherwise",
				status, ready)
		}
		if status != http.StatusServiceUnavailable {
			t.Fatalf("GET /readyz?strict=true returned HTTP %d; an unproven Knowledge Graph journey MUST answer HTTP %d",
				status, http.StatusServiceUnavailable)
		}

		// The refusal must be GRAPH-driven, not a blanket outage. Re-probe
		// plain /readyz immediately afterwards: if the stack had simply gone
		// unhealthy, this would be red too, and the strict assertion above
		// would be worthless.
		plainStatus, plainReady := probeReadyz(t, "")
		if plainStatus != http.StatusOK || !plainReady {
			t.Fatalf("immediately after the strict refusal, plain GET /readyz returned HTTP %d with ready=%t; the strict 503 is a BLANKET failure rather than a graph-specific refusal, so it proves nothing about Knowledge Graph readiness derivation",
				plainStatus, plainReady)
		}

		// The strict opt-in is a CLOSED contract, not a substring match: every
		// accepted truthy spelling must refuse...
		for _, spelling := range []string{"1", "true", "yes", "TRUE", "Yes"} {
			truthyStatus, truthyReady := probeReadyz(t, "?strict="+spelling)
			if truthyStatus != http.StatusServiceUnavailable || truthyReady {
				t.Errorf("GET /readyz?strict=%s returned HTTP %d with ready=%t; this is an accepted truthy opt-in spelling and MUST refuse with HTTP %d and ready=false exactly as ?strict=true does",
					spelling, truthyStatus, truthyReady, http.StatusServiceUnavailable)
			}
		}
		// ...and a NON-truthy spelling must NOT opt in, proving the parameter
		// is parsed against a closed vocabulary rather than merely detected.
		nonTruthyStatus, nonTruthyReady := probeReadyz(t, "?strict=maybe")
		if nonTruthyStatus != http.StatusOK || !nonTruthyReady {
			t.Errorf("GET /readyz?strict=maybe returned HTTP %d with ready=%t; %q is NOT an accepted truthy opt-in value, so the probe MUST fall through to general liveness (HTTP %d, ready=true)",
				nonTruthyStatus, nonTruthyReady, "maybe", http.StatusOK)
		}
	})

	t.Run("Regression: static Wiki and green liveness cannot satisfy Graph readiness (authenticated health reports the truthful graph section)", func(t *testing.T) {
		resp, body := graphAPIGet(t, cfg, "/api/health")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("authenticated GET /api/health returned HTTP %d; the liveness endpoint MUST answer HTTP %d",
				resp.StatusCode, http.StatusOK)
		}

		var payload struct {
			Graph *struct {
				Ready       bool   `json:"ready"`
				Activation  string `json:"activation"`
				State       string `json:"state"`
				Code        string `json:"code"`
				EvidenceRef string `json:"evidence_ref"`
			} `json:"graph"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("authenticated GET /api/health returned a body that does not decode against the health contract: %v", err)
		}
		if payload.Graph == nil {
			t.Fatalf("authenticated GET /api/health omitted the `graph` capability section; an AUTHENTICATED caller MUST receive the Knowledge Graph capability detail")
		}

		if payload.Graph.Ready {
			t.Fatalf("authenticated /api/health reports a READY Knowledge Graph capability (state=%q code=%q); no synthetic observation is published on this stack, so a ready section means readiness was derived from something other than the activation policy plus a fresh published aggregate",
				payload.Graph.State, payload.Graph.Code)
		}
		if payload.Graph.EvidenceRef != graphsynthetic.AggregateEvidenceRef {
			t.Errorf("graph capability section carries evidence_ref %q; it MUST be the constant %q",
				payload.Graph.EvidenceRef, graphsynthetic.AggregateEvidenceRef)
		}
		if !activationStates[payload.Graph.Activation] {
			t.Errorf("graph capability section carries activation %q; it MUST be within the closed set {%q,%q}",
				payload.Graph.Activation, graphapi.ActivationEnabled, graphapi.ActivationDisabled)
		}
		if !aggregateStates[payload.Graph.State] {
			t.Errorf("graph capability section carries state %q; it MUST be within the closed aggregate set {%q,%q,%q,%q}",
				payload.Graph.State,
				graphsynthetic.AggregateAvailable, graphsynthetic.AggregateDegraded,
				graphsynthetic.AggregateUnavailable, graphsynthetic.AggregatePolicyDisabled)
		}
		// Accept the CLOSED union: a readiness-projection code, or an
		// aggregate code from the F080-SYNTH-* family. Anything else is a
		// vocabulary escape and is rejected.
		if !readinessProjectionCodes[payload.Graph.Code] && !strings.HasPrefix(payload.Graph.Code, "F080-SYNTH-") {
			t.Errorf("graph capability section carries code %q; it MUST be a readiness-projection code (F080-READINESS-NOT-OBSERVED / -STALE / -ACTIVATION-MISMATCH / -CONFIG-INVALID) or an F080-SYNTH-* aggregate code",
				payload.Graph.Code)
		}

		// CROSS-SURFACE CONSISTENCY. The authenticated capability detail and
		// the unauthenticated strict probe are two renderings of the SAME
		// derivation and MUST agree. A ready section beside a strict 503 (or a
		// non-ready section beside a strict 200) is a contradiction that would
		// let one surface mask the other.
		observedStatus, observedReady := strictStatus, strictReady
		if !strictObserved {
			observedStatus, observedReady = probeReadyz(t, "?strict=true")
		}
		if observedReady != payload.Graph.Ready {
			t.Fatalf("CONTRADICTION: /readyz?strict=true reports ready=%t (HTTP %d) while authenticated /api/health reports graph.ready=%t; both surfaces render the SAME readiness derivation and MUST agree",
				observedReady, observedStatus, payload.Graph.Ready)
		}
		if payload.Graph.Ready && observedStatus == http.StatusServiceUnavailable {
			t.Fatalf("CONTRADICTION: authenticated /api/health reports a READY graph capability while /readyz?strict=true answers HTTP %d",
				observedStatus)
		}
		if !payload.Graph.Ready && observedStatus == http.StatusOK {
			t.Fatalf("CONTRADICTION: authenticated /api/health reports graph.ready=false while /readyz?strict=true answers HTTP %d; the strict probe was satisfied by something other than a ready Knowledge Graph journey",
				observedStatus)
		}
	})

	t.Run("Regression: static Wiki and green liveness cannot satisfy Graph readiness (unauthenticated health withholds capability detail)", func(t *testing.T) {
		// Capability detail is withheld from unauthenticated callers to deny
		// reconnaissance (CWE-200). The `graph` key must be ABSENT — not
		// present-and-empty, which would still leak that the capability exists
		// and is being tracked.
		resp, err := (&http.Client{Timeout: 15 * time.Second}).Get(cfg.CoreURL + "/api/health")
		if err != nil {
			t.Fatalf("unauthenticated GET /api/health against the live stack: %v", err)
		}
		body, err := readBody(resp)
		if err != nil {
			t.Fatalf("read response body of unauthenticated GET /api/health: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("unauthenticated GET /api/health returned HTTP %d; the default liveness answer MUST be HTTP %d",
				resp.StatusCode, http.StatusOK)
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatalf("unauthenticated GET /api/health returned a body that is not a JSON object: %v", err)
		}
		if _, present := raw["graph"]; present {
			t.Fatalf("unauthenticated GET /api/health EXPOSED the `graph` capability section; Knowledge Graph capability detail MUST be withheld from unauthenticated callers (CWE-200) and is available to AUTHENTICATED callers only")
		}
	})
}
