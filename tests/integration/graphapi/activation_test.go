//go:build integration

// BUG-080-001 — fail-soft graph activation, LIVE runtime-wiring proof.
//
// T080-01-PROC (SCN-080-001-01). This file proves the operator-directed
// fail-soft activation END-TO-END through the REAL production router
// (internal/api.NewRouter — the same router cmd/core builds at boot),
// over a REAL loopback HTTP server (httptest), and the disposable
// stack's REAL PostgreSQL (DATABASE_URL). There is NO request
// interception, NO mock, NO stub: the router, the graphapi Guard, and
// the live PostgreSQL-backed handlers are the real production code.
//
// Why in-process rather than only hitting the running smackerel-core
// container (as topics_test.go et al. do)? The running container is a
// SINGLE activation state — its cursor secret is configured, so it can
// only demonstrate the ENABLED path. To prove the DISABLED (fail-soft)
// path we MUST construct the router with an unavailable cursor secret,
// which the running container cannot do. Constructing api.NewRouter
// in-process against the disposable stack's real Postgres lets one
// focused test prove BOTH activation states with the real wiring.
//
// The construction here MIRRORS cmd/core/wiring.go exactly: activation
// is derived from the cursor-secret presence via
// graphapi.ResolveActivation / NewGraphCapability, and when ENABLED the
// same live NewTopicsHandlers(...) et al. are wired over the real pool
// + cursor codec. The DISABLED case reproduces the original silent-404
// bug's trigger (an empty/missing secret) and asserts the FIXED
// behavior (present-but-disabled typed 503, never a Chi 404).

package graphapi_integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/api/graphapi"
)

// graphActivationLimits mirrors config/smackerel.yaml knowledge_graph_api.*
// so the ENABLED handlers clamp identically to the deployed config. Only
// CursorSecretEnv is varied per sub-test to flip the activation state.
func graphActivationLimits() graphapi.Config {
	return graphapi.Config{
		ListDefaultLimit:  50,
		ListMaxLimit:      200,
		TimeWindowMaxDays: 365,
		EdgesDefaultLimit: 100,
		EdgesMaxLimit:     500,
	}
}

// canonicalGraphPaths is the design.md "Canonical Route Manifest" — the
// eight family paths that MUST be PRESENT (typed 503 when disabled, never
// a Chi 404) in both activation states.
var canonicalGraphPaths = []string{
	"/api/topics",
	"/api/topics/bug080it-nonexistent",
	"/api/people",
	"/api/people/bug080it-nonexistent",
	"/api/places",
	"/api/places/bug080it-nonexistent",
	"/api/time",
	"/api/graph/edges",
}

// newDisabledGraphRouter builds the REAL production router with a graph
// capability resolved DISABLED from an unavailable cursor secret, exactly
// as cmd/core/wiring.go would when the operator secret is absent.
// AuthToken="" + Environment!="production" takes the documented dev
// empty-token bypass (bearerAuthMiddleware branch 4) so the request
// reaches the graph group without a bearer; the fail-soft 503 is asserted
// independent of auth.
func newDisabledGraphRouter(t *testing.T, cfg graphapi.Config) http.Handler {
	t.Helper()
	graphCap := graphapi.NewGraphCapability(cfg)
	if !graphCap.Disabled() {
		t.Fatalf("test setup: expected DISABLED capability for cfg %+v", cfg)
	}
	return api.NewRouter(&api.Dependencies{
		Environment:     "test",
		AuthToken:       "",
		GraphCapability: graphCap,
	})
}

// TestGraphActivationDisabledSecretServesTyped503AndKeepsServing proves
// SCN-080-001-01 fail-soft (a): when the operator cursor secret is
// unavailable, EVERY canonical graph path is PRESENT and answers the
// typed HTTP 503 capability_disabled envelope (never a silent Chi 404,
// never an opaque 500/panic), and the service keeps serving other paths.
func TestGraphActivationDisabledSecretServesTyped503AndKeepsServing(t *testing.T) {
	// (a) EMPTY cursor secret (named env var set-but-empty).
	t.Run("empty_secret_serves_503_not_404", func(t *testing.T) {
		envName := "KNOWLEDGE_GRAPH_API_CURSOR_SECRET_BUG080IT_EMPTY"
		t.Setenv(envName, "") // set-but-empty -> SecretEmpty -> disabled
		cfg := graphActivationLimits()
		cfg.CursorSecretEnv = envName

		srv := httptest.NewServer(newDisabledGraphRouter(t, cfg))
		t.Cleanup(srv.Close)

		assertGraphPathsTyped503(t, srv.URL)
		assertServiceKeepsServing(t, srv.URL)
	})

	// MISSING cursor secret (named env var never set at all).
	t.Run("missing_secret_serves_503_not_404", func(t *testing.T) {
		cfg := graphActivationLimits()
		cfg.CursorSecretEnv = "KNOWLEDGE_GRAPH_API_CURSOR_SECRET_BUG080IT_UNSET_DO_NOT_SET"

		srv := httptest.NewServer(newDisabledGraphRouter(t, cfg))
		t.Cleanup(srv.Close)

		assertGraphPathsTyped503(t, srv.URL)
		assertServiceKeepsServing(t, srv.URL)
	})
}

// assertGraphPathsTyped503 drives every canonical graph path and proves
// it is PRESENT and answers the typed 503 capability_disabled envelope —
// the adversarial anti-regression is the explicit 404 rejection (a 404
// would mean the fix reverted to the original nil-handler silent absence).
func assertGraphPathsTyped503(t *testing.T, base string) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	for _, p := range canonicalGraphPaths {
		resp, err := client.Get(base + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			t.Fatalf("GET %s -> 404: fail-soft REVERTED to the silent-absence bug (route absent / nil handler). body=%s", p, string(body))
		}
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("GET %s -> %d; want 503 capability_disabled. body=%s", p, resp.StatusCode, string(body))
		}
		var env errorEnvelope
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("GET %s: 503 body is not a typed envelope: %v (body=%q)", p, err, string(body))
		}
		if env.Error.Code != "capability_disabled" {
			t.Fatalf("GET %s: error.code=%q; want capability_disabled. body=%s", p, env.Error.Code, string(body))
		}
		if env.Error.Message == "" {
			t.Fatalf("GET %s: capability_disabled envelope carries no message; the state must be honest, not silent", p)
		}
	}
}

// assertServiceKeepsServing proves the process still serves non-graph
// paths while the graph capability is disabled (never a boot refusal, a
// panic, or a process-wide failure on the empty secret). /ping is the
// router heartbeat and MUST answer 200.
func assertServiceKeepsServing(t *testing.T, base string) {
	t.Helper()
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(base + "/ping")
	if err != nil {
		t.Fatalf("GET /ping: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /ping -> %d; want 200 (the service MUST keep serving other capabilities while graph is disabled)", resp.StatusCode)
	}
}

// TestGraphActivationEnabledSecretServesLiveOverPostgres proves
// SCN-080-001-01/02 fail-soft (b): with the cursor secret CONFIGURED the
// graph capability is ENABLED and the live PostgreSQL-backed topics
// handler serves a real seeded row (2xx) over the disposable stack's real
// database — the same wiring path as cmd/core/wiring.go's enabled branch.
func TestGraphActivationEnabledSecretServesLiveOverPostgres(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live stack not available")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}

	// Configured non-empty cursor secret -> ENABLED activation.
	envName := "KNOWLEDGE_GRAPH_API_CURSOR_SECRET_BUG080IT_ENABLED"
	t.Setenv(envName, "bug080-001-integration-cursor-secret-32bytes!!")
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
	limits := cfg.Limits()

	// Seed one real topic into the disposable Postgres. Uniquely-prefixed
	// so teardown scopes its DELETE to this test's data only.
	prefix := "bug080it-enabled-" + time.Now().UTC().Format("20060102150405.000000")
	topicID := prefix + "-topic-0"
	if _, err := pool.Exec(ctx,
		`INSERT INTO topics (id, name, capture_count_total, momentum_score) VALUES ($1,$2,$3,$4)`,
		topicID, topicID, 11, float32(1.0)); err != nil {
		t.Fatalf("seed topic: %v", err)
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(cctx, `DELETE FROM topics WHERE id LIKE $1`, prefix+"-%"); err != nil {
			t.Logf("cleanup topics %s-%%: %v", prefix, err)
		}
	})

	// Build the REAL router with the ENABLED capability + live handlers,
	// mirroring cmd/core/wiring.go's enabled branch.
	router := api.NewRouter(&api.Dependencies{
		Environment:     "test",
		AuthToken:       "",
		GraphCapability: graphCap,
		TopicsHandlers:  graphapi.NewTopicsHandlers(pool, limits, codec),
		PeopleHandlers:  graphapi.NewPeopleHandlers(pool, limits, codec),
		PlacesHandlers:  graphapi.NewPlacesHandlers(pool, limits, codec),
		TimeHandlers:    graphapi.NewTimeHandlers(pool, limits),
		EdgesHandlers:   graphapi.NewEdgesHandlers(pool, limits, codec),
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Get(srv.URL + "/api/topics?limit=50")
	if err != nil {
		t.Fatalf("GET /api/topics: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/topics -> %d; want 200 (enabled, live PostgreSQL). body=%s", resp.StatusCode, string(body))
	}
	var got topicsListBody
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode topics list: %v (body=%s)", err, string(body))
	}
	found := false
	for _, it := range got.Items {
		if it.ID == topicID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("seeded topic %s missing from live ENABLED response (items=%d); the enabled handler did not serve real PostgreSQL rows", topicID, len(got.Items))
	}
}
