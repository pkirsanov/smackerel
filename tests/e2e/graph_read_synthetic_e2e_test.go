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
