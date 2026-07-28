//go:build integration

// BUG-080-001 — T080-06-STORE (SCN-080-001-06), LIVE runtime proof that a
// graph STORE failure is a typed 503 `store_unavailable` and is NEVER a
// 404 nor a 200 with an empty `items` array.
//
// Why this distinction is the whole point: the original bug shipped a
// graph surface where an unavailable backend was indistinguishable from
// "the graph legitimately holds nothing". Per
// $B/design.md §"Closed Read Outcomes", the closed outcome model demands
// that `store-unavailable` (503 `store_unavailable`) and `true-empty`
// (200, zero records after a SUCCESSFUL read) are separate, observable
// states. A 404 is worse still: it is the route/activation surrogate the
// fail-soft work (T080-01-PROC) already eliminated, so a broken datastore
// regressing into a 404 would re-open the original silent-absence bug
// through a different door.
//
// Construction mirrors activation_test.go exactly: the REAL production
// router (internal/api.NewRouter — the same router cmd/core/wiring.go
// builds at boot) over a REAL loopback HTTP server (httptest). There is
// NO request interception, NO mock router, NO stub source. The failure is
// induced in the store itself, two genuinely different ways:
//
//	(1) a *pgxpool.Pool opened against the disposable stack's REAL
//	    DATABASE_URL and then Close()d before the request — the
//	    production "pool went away underneath us" mode;
//	(2) a valid-but-unreachable DSN (loopback port 1) with a short
//	    connect timeout — the production "backend is down / unroutable"
//	    mode.
//
// The schema arm reuses the ALREADY-SHIPPED ErrSchemaError path
// (non-terminal page whose continuation cursor cannot be produced) and
// only ASSERTS it — it changes nothing about that path — proving the
// other non-empty failure outcome is likewise neither 404 nor empty-200.

package graphapi_integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/api/graphapi"
)

// brokenStoreProbe is one request that MUST reach the data-access layer.
// Every graph family is represented; the params are deliberately VALID so
// nothing short-circuits in parsing and the request genuinely reaches the
// store.
type brokenStoreProbe struct {
	family string
	path   string
}

// graphStoreProbes covers all five graph families across both list and
// detail shapes. The three detail probes are the sharpest adversarials in
// this file: GetTopic/GetPerson/GetPlace map their OWN not-found sentinel
// to 404, so a detail route is exactly where a broken backend would
// silently masquerade as "resource missing".
//
// The places/detail id MUST carry the `ar:` (or `mp:`) namespace prefix.
// pgxPlacesSource.GetPlace resolves an un-namespaced id to
// ErrPlaceNotFound in its `default:` branch WITHOUT issuing a query — a
// deliberate pre-store decision (an id belonging to no place namespace
// genuinely is not found). A probe id without the prefix therefore never
// reaches the data layer and proves nothing about store failure. Do not
// "simplify" this id back to a bare string: doing so silently drops
// places/detail store coverage while the sub-test still goes green.
func graphStoreProbes() []brokenStoreProbe {
	from := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	to := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	return []brokenStoreProbe{
		{family: "topics/list", path: "/api/topics?limit=50"},
		{family: "topics/detail", path: "/api/topics/bug080it-store-probe"},
		{family: "people/list", path: "/api/people?limit=50"},
		{family: "people/detail", path: "/api/people/bug080it-store-probe"},
		{family: "places/list", path: "/api/places?limit=50"},
		{family: "places/detail", path: "/api/places/ar:bug080it-store-probe"},
		{family: "time/window", path: "/api/time?from=" + from + "&to=" + to},
		{family: "edges/list", path: "/api/graph/edges?source=topic:bug080it-store-probe"},
	}
}

// storeFailureLeakTokens are substrings that MUST NOT appear anywhere in
// a store-failure response body. They cover SQL text, table and column
// names from the real graph schema, driver text, and DSN/host material.
var storeFailureLeakTokens = []string{
	"select ", "insert ", "update ", "delete ", "from topics", "from people",
	"from places", "from edges", "join ", "where ", "coalesce",
	"topics", "artifacts", "edges", "src_id", "dst_id", "src_type", "dst_type",
	"capture_count_total", "momentum_score", "captured_at",
	"pgx", "pgconn", "puddle", "sqlstate", "postgres", "postgresql",
	"password", "dbname", "user=", "host=", "port=", "127.0.0.1", "closed pool",
}

// newGraphRouterOverPool builds the REAL production router with an
// ENABLED graph capability and the live PostgreSQL-backed handlers over
// the supplied pool — the same wiring as cmd/core/wiring.go's enabled
// branch. Passing a broken pool is what induces the store failure.
func newGraphRouterOverPool(t *testing.T, pool *pgxpool.Pool, codec *graphapi.CursorCodec, limits graphapi.Limits, graphCap *graphapi.GraphCapability) http.Handler {
	t.Helper()
	return api.NewRouter(&api.Dependencies{
		Environment:     "test",
		AuthToken:       "",
		GraphCapability: graphCap,
		TopicsHandlers:  graphapi.NewTopicsHandlers(pool, limits, codec),
		PeopleHandlers:  graphapi.NewPeopleHandlers(pool, limits, codec),
		PlacesHandlers:  graphapi.NewPlacesHandlers(pool, limits, codec),
		TimeHandlers:    graphapi.NewTimeHandlers(pool, limits),
		EdgesHandlers:   graphapi.NewEdgesHandlers(pool, limits, codec),
	})
}

// enabledGraphWiring resolves an ENABLED capability plus its cursor codec
// and limits from a configured cursor secret, exactly as the boot path
// does. envSuffix keeps the per-sub-test env var names distinct.
func enabledGraphWiring(t *testing.T, envSuffix string) (*graphapi.GraphCapability, *graphapi.CursorCodec, graphapi.Limits) {
	t.Helper()
	envName := "KNOWLEDGE_GRAPH_API_CURSOR_SECRET_BUG080IT_" + envSuffix
	t.Setenv(envName, "bug080-001-store-failure-cursor-secret-32b!!")
	cfg := graphActivationLimits()
	cfg.CursorSecretEnv = envName

	graphCap := graphapi.NewGraphCapability(cfg)
	if graphCap.Disabled() {
		t.Fatalf("test setup: expected ENABLED capability with a present secret")
	}
	secret, err := cfg.LoadCursorSecret()
	if err != nil {
		t.Fatalf("LoadCursorSecret: %v", err)
	}
	codec, err := graphapi.NewCursorCodec(secret)
	if err != nil {
		t.Fatalf("NewCursorCodec: %v", err)
	}
	return graphCap, codec, cfg.Limits()
}

// TestGraphStoreAndSchemaFailuresAreNeverEmptyOrNotFound is the
// T080-06-STORE / SCN-080-001-06 proof.
func TestGraphStoreAndSchemaFailuresAreNeverEmptyOrNotFound(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live stack not available")
	}

	// (1) A pool opened against the REAL database and then Close()d. This
	// is not a fabricated error value: the pool really did connect, and
	// the handler really does fail at acquire time, exactly as it would
	// if the datastore went away mid-flight.
	t.Run("closed_pool_is_typed_503_store_unavailable", func(t *testing.T) {
		ctx := context.Background()
		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			t.Fatalf("pgxpool.New(real DSN): %v", err)
		}
		// Prove the pool was genuinely healthy BEFORE breaking it —
		// otherwise a 503 could be an environment artifact rather than
		// the induced failure.
		if err := pool.Ping(ctx); err != nil {
			t.Fatalf("ping real postgres before inducing failure: %v", err)
		}
		pool.Close()

		graphCap, codec, limits := enabledGraphWiring(t, "CLOSEDPOOL")
		srv := httptest.NewServer(newGraphRouterOverPool(t, pool, codec, limits, graphCap))
		t.Cleanup(srv.Close)

		assertStoreFailureIsTyped503(t, srv.URL, "closed pool")
	})

	// (2) A valid-but-unreachable DSN. pgxpool connects lazily, so the
	// failure lands at query time as a genuine pgconn connect error.
	t.Run("unreachable_dsn_is_typed_503_store_unavailable", func(t *testing.T) {
		unreachable := "postgres://127.0.0.1:1/bug080it?sslmode=disable&connect_timeout=2"
		pool, err := pgxpool.New(context.Background(), unreachable)
		if err != nil {
			t.Fatalf("pgxpool.New(unreachable DSN): %v", err)
		}
		t.Cleanup(pool.Close)

		graphCap, codec, limits := enabledGraphWiring(t, "UNREACHABLE")
		srv := httptest.NewServer(newGraphRouterOverPool(t, pool, codec, limits, graphCap))
		t.Cleanup(srv.Close)

		assertStoreFailureIsTyped503(t, srv.URL, "unreachable DSN")
	})

	// (3) The OTHER non-empty failure outcome. This arm asserts the
	// already-shipped ErrSchemaError path (design.md §"Completeness
	// Envelope": a non-terminal page whose continuation cursor cannot be
	// produced is a schema error, never a silently-empty cursor). It
	// changes nothing about that path — it only proves schema-error is
	// likewise neither a 404 nor a 200 with items.
	t.Run("schema_error_is_typed_500_never_404_or_empty_200", func(t *testing.T) {
		ctx := context.Background()
		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			t.Fatalf("pgxpool.New: %v", err)
		}
		t.Cleanup(pool.Close)
		if err := pool.Ping(ctx); err != nil {
			t.Fatalf("ping postgres: %v", err)
		}

		// Seed two real topics so limit=1 yields hasNext=true.
		prefix := "bug080it-schema-" + time.Now().UTC().Format("20060102150405.000000")
		for i := 0; i < 2; i++ {
			id := fmt.Sprintf("%s-topic-%d", prefix, i)
			if _, err := pool.Exec(ctx,
				`INSERT INTO topics (id, name, capture_count_total, momentum_score) VALUES ($1,$2,$3,$4)`,
				id, id, 7, float32(1.0)); err != nil {
				t.Fatalf("seed topic %s: %v", id, err)
			}
		}
		t.Cleanup(func() {
			cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := pool.Exec(cctx, `DELETE FROM topics WHERE id LIKE $1`, prefix+"-%"); err != nil {
				t.Logf("cleanup topics %s-%%: %v", prefix, err)
			}
		})

		// ENABLED capability but NO cursor codec: the read succeeds, the
		// page is non-terminal, and the continuation cursor cannot be
		// produced -> ErrSchemaError.
		graphCap, _, limits := enabledGraphWiring(t, "SCHEMAERR")
		srv := httptest.NewServer(newGraphRouterOverPool(t, pool, nil, limits, graphCap))
		t.Cleanup(srv.Close)

		resp, body := getGraph(t, srv.URL+"/api/topics?limit=1")
		if resp.StatusCode == http.StatusNotFound {
			t.Fatalf("schema-error read -> 404: an internal projection inconsistency MUST NOT masquerade as a missing route/resource. body=%s", string(body))
		}
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("schema-error read -> 200: an unrenderable page MUST NOT be served as a successful read (silent truncation). body=%s", string(body))
		}
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("schema-error read -> %d; want 500 schema_error. body=%s", resp.StatusCode, string(body))
		}
		var env errorEnvelope
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("schema-error body is not a typed envelope: %v (body=%q)", err, string(body))
		}
		if env.Error.Code != graphapi.CodeSchemaError {
			t.Fatalf("schema-error error.code=%q; want %q. body=%s", env.Error.Code, graphapi.CodeSchemaError, string(body))
		}
		assertNoItemsArray(t, "schema-error /api/topics?limit=1", body)
		assertValueSafeMessage(t, "schema-error /api/topics?limit=1", env.Error.Message, body)
	})
}

// assertStoreFailureIsTyped503 drives every graph family against a
// deliberately broken store and enforces the full T080-06-STORE contract
// per probe: exactly 503, error.code == store_unavailable, NOT 404, NOT
// 200, no `items` array pretending emptiness, and a value-safe message.
func assertStoreFailureIsTyped503(t *testing.T, base, mode string) {
	t.Helper()
	for _, probe := range graphStoreProbes() {
		probe := probe
		t.Run(probe.family, func(t *testing.T) {
			resp, body := getGraph(t, base+probe.path)
			label := fmt.Sprintf("[%s] GET %s", mode, probe.path)

			// Adversarial 1 — the route/activation surrogate. A broken
			// datastore regressing into 404 re-opens the original
			// silent-absence bug through a different door.
			if resp.StatusCode == http.StatusNotFound {
				t.Fatalf("%s -> 404: a store failure MUST NOT masquerade as a missing route/resource (`not-found` is reserved for a real missing resource on a healthy store). body=%s", label, string(body))
			}
			// Adversarial 2 — the empty-success masquerade. 200 means the
			// read SUCCEEDED; `true-empty` is a distinct closed outcome
			// from `store-unavailable` and must never absorb it.
			if resp.StatusCode == http.StatusOK {
				t.Fatalf("%s -> 200: a store failure MUST NOT be served as a successful read; a broken datastore must not look like \"no data\". body=%s", label, string(body))
			}
			// Adversarial 3 — the pre-fix generic mapping. A 500
			// internal_error means the store-unavailable arm of the
			// closed outcome model was never wired.
			if resp.StatusCode == http.StatusInternalServerError {
				t.Fatalf("%s -> 500: store connectivity failure is still mapped to the generic internal_error; design.md \u00a7\"Closed Read Outcomes\" requires 503 store_unavailable. body=%s", label, string(body))
			}
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("%s -> %d; want 503 store_unavailable. body=%s", label, resp.StatusCode, string(body))
			}

			var env errorEnvelope
			if err := json.Unmarshal(body, &env); err != nil {
				t.Fatalf("%s: 503 body is not a typed error envelope: %v (body=%q)", label, err, string(body))
			}
			if env.Error.Code != graphapi.CodeStoreUnavailable {
				// capability_disabled would mean the capability, not the
				// store, was the observed failure — a different outcome.
				t.Fatalf("%s: error.code=%q; want %q. body=%s", label, env.Error.Code, graphapi.CodeStoreUnavailable, string(body))
			}
			if strings.TrimSpace(env.Error.Message) == "" {
				t.Fatalf("%s: store_unavailable envelope carries no message; the state must be honest, not silent", label)
			}

			assertNoItemsArray(t, label, body)
			assertValueSafeMessage(t, label, env.Error.Message, body)
		})
	}
}

// assertNoItemsArray proves the failure body carries no `items` (or
// `days`) collection pretending the graph is empty. A failure envelope
// that ALSO ships `"items": []` lets a lenient client render "no data".
func assertNoItemsArray(t *testing.T, label string, body []byte) {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("%s: body is not a JSON object: %v (body=%q)", label, err, string(body))
	}
	for _, key := range []string{"items", "days", "nextCursor"} {
		if _, present := raw[key]; present {
			t.Fatalf("%s: failure body carries a %q member; a failure MUST NOT ship a collection that reads as \"the graph is empty\". body=%s", label, key, string(body))
		}
	}
}

// assertValueSafeMessage proves the failure response leaks no SQL text,
// table/column name, row value, DSN, host, or driver text. Checked over
// the WHOLE body, not just the message, so a leak cannot hide in `field`.
func assertValueSafeMessage(t *testing.T, label, message string, body []byte) {
	t.Helper()
	if strings.TrimSpace(message) == "" {
		t.Fatalf("%s: empty error message", label)
	}
	lower := strings.ToLower(string(body))
	for _, tok := range storeFailureLeakTokens {
		if strings.Contains(lower, tok) {
			t.Fatalf("%s: failure body leaks internal detail %q; the message must be value-safe (no SQL, table, column, row value, DSN, host, or driver text). body=%s", label, tok, string(body))
		}
	}
	// Belt and braces: no scheme-ish DSN fragment anywhere.
	for _, frag := range []string{"postgres://", "postgresql://", "@", "sslmode"} {
		if strings.Contains(lower, frag) {
			t.Fatalf("%s: failure body leaks a DSN fragment %q. body=%s", label, frag, string(body))
		}
	}
}

// getGraph issues an unauthenticated GET (the router runs with
// AuthToken="" + Environment="test", taking the documented dev
// empty-token bypass) and returns the response plus its fully-read body.
func getGraph(t *testing.T, url string) (*http.Response, []byte) {
	t.Helper()
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		t.Fatalf("read body %s: %v", url, readErr)
	}
	return resp, body
}
