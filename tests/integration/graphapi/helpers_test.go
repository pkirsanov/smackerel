//go:build integration

// Spec 080 — Knowledge Graph Public API.
// Live-stack integration tier for SCOPE-080-01..04.
//
// These tests hit the real ephemeral test stack via CORE_EXTERNAL_URL
// + SMACKEREL_AUTH_TOKEN (shared-token mode; AUTH_ENABLED=false in
// the test stack per config/generated/test.env) and seed minimal
// fixtures directly into Postgres via DATABASE_URL so the happy
// paths assert against real rows produced by the live handlers, not
// against an in-process stub.
//
// Per-user PASETO scope-claim adversarials (SCN-080-10) CANNOT be
// driven against the test stack because AUTH_ENABLED=false collapses
// the scope middleware to its bootstrap branch (the shared bearer
// bypasses RequireScope). Coverage for the scope claim itself lives
// in `internal/auth/scopes_test.go` and the SCOPE-080-01 envelope
// unit tests (TestWriteAPIError_MissingScope); marking the
// SCN-080-10 DoD item requires either a flavor of the test stack
// with AUTH_ENABLED=true or routing to bubbles.implement to add
// one. This file documents the constraint in
// TestGraphAPI_403_MissingScope_LiveStackConstraint so the gap is
// visible to bubbles.validate.

package graphapi_integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type liveCfg struct {
	CoreURL   string
	AuthToken string
	DBURL     string
}

func loadLive(t *testing.T) liveCfg {
	t.Helper()
	core := strings.TrimRight(os.Getenv("CORE_EXTERNAL_URL"), "/")
	if core == "" {
		t.Skip("integration: CORE_EXTERNAL_URL not set — live stack not available")
	}
	// The repo-standard `./smackerel.sh test integration` runs this
	// container on the compose network with `CORE_EXTERNAL_URL`
	// inherited from the env file as the host-mapped URL
	// (`http://127.0.0.1:<host-port>`). That URL is unreachable from
	// inside the test container; rewrite to the in-network service
	// hostname `http://smackerel-core:<container-port>` so the live
	// HTTP path actually reaches the running core.
	if strings.HasPrefix(core, "http://127.0.0.1:") ||
		strings.HasPrefix(core, "http://localhost:") {
		cport := os.Getenv("CORE_CONTAINER_PORT")
		if cport == "" {
			t.Fatalf("integration: CORE_EXTERNAL_URL=%q points at loopback but CORE_CONTAINER_PORT is empty — cannot rewrite to in-network URL", core)
		}
		core = "http://smackerel-core:" + cport
	}
	tok := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if tok == "" {
		t.Fatalf("integration: SMACKEREL_AUTH_TOKEN unset while CORE_EXTERNAL_URL is set — wiring bug")
	}
	db := os.Getenv("DATABASE_URL")
	if db == "" {
		t.Fatalf("integration: DATABASE_URL unset while CORE_EXTERNAL_URL is set — wiring bug")
	}
	return liveCfg{CoreURL: core, AuthToken: tok, DBURL: db}
}

func waitHealthy(t *testing.T, cfg liveCfg, max time.Duration) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		resp, err := client.Get(cfg.CoreURL + "/api/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("integration: stack not healthy at %s after %s", cfg.CoreURL, max)
}

// doAuthedGET issues GET with bearer; caller closes body.
func doAuthedGET(t *testing.T, cfg liveCfg, path string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, cfg.CoreURL+path, nil)
	if err != nil {
		t.Fatalf("NewRequest(%s): %v", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, body
}

// doUnauthedGET issues GET with no Authorization header.
func doUnauthedGET(t *testing.T, cfg liveCfg, path string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, cfg.CoreURL+path, nil)
	if err != nil {
		t.Fatalf("NewRequest(%s): %v", path, err)
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, body
}

// connectDB opens a pgx conn against DATABASE_URL.
func connectDB(t *testing.T, cfg liveCfg) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), cfg.DBURL)
	if err != nil {
		t.Fatalf("pgx.Connect: %v", err)
	}
	return conn
}

// fixturePrefix tags every seeded row with a unique marker so the
// teardown query can scope DELETEs to test data only.
func fixturePrefix(t *testing.T) string {
	t.Helper()
	return "graphapi-it-" + time.Now().UTC().Format("20060102150405.000000")
}

// seedTopics inserts n topics with names "<prefix>-topic-<i>" and
// returns the generated topic ids in insertion order.
func seedTopics(t *testing.T, conn *pgx.Conn, prefix string, n int) []string {
	t.Helper()
	ctx := context.Background()
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := prefix + "-topic-" + itoa(i)
		_, err := conn.Exec(ctx,
			`INSERT INTO topics (id, name, capture_count_total, momentum_score)
			 VALUES ($1, $2, $3, $4)`,
			id, id, 10+i, float32(1.0+float64(i)*0.1))
		if err != nil {
			t.Fatalf("seed topic %s: %v", id, err)
		}
		ids = append(ids, id)
	}
	return ids
}

func seedPeople(t *testing.T, conn *pgx.Conn, prefix string, n int) []string {
	t.Helper()
	ctx := context.Background()
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		id := prefix + "-person-" + itoa(i)
		_, err := conn.Exec(ctx,
			`INSERT INTO people (id, name) VALUES ($1, $2)`, id, id)
		if err != nil {
			t.Fatalf("seed person %s: %v", id, err)
		}
		ids = append(ids, id)
	}
	return ids
}

// seedArtifacts inserts n artifacts with deterministic capturedAt
// staggered by 1 day so timeline-DESC ordering is observable.
func seedArtifacts(t *testing.T, conn *pgx.Conn, prefix string, n int) []string {
	t.Helper()
	ctx := context.Background()
	ids := make([]string, 0, n)
	base := time.Now().UTC().Add(-time.Duration(n) * 24 * time.Hour)
	for i := 0; i < n; i++ {
		id := prefix + "-artifact-" + itoa(i)
		ts := base.Add(time.Duration(i) * 24 * time.Hour)
		_, err := conn.Exec(ctx,
			`INSERT INTO artifacts
			   (id, artifact_type, title, content_hash, source_id, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
			id, "note", id+"-title", id+"-hash", "graphapi-it-seed", ts)
		if err != nil {
			t.Fatalf("seed artifact %s: %v", id, err)
		}
		ids = append(ids, id)
	}
	return ids
}

// seedEdge inserts one edge row. id is composed from the prefix and
// the (srcType,srcId,dstType,dstId,edgeType) tuple so cleanup can
// scope by id LIKE prefix.
func seedEdge(t *testing.T, conn *pgx.Conn, prefix, srcType, srcID, dstType, dstID, edgeType string, weight float32) {
	t.Helper()
	id := prefix + "-edge-" + srcType + "-" + srcID + "-" + dstType + "-" + dstID + "-" + edgeType
	_, err := conn.Exec(context.Background(),
		`INSERT INTO edges (id, src_type, src_id, dst_type, dst_id, edge_type, weight)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT DO NOTHING`,
		id, srcType, srcID, dstType, dstID, edgeType, weight)
	if err != nil {
		t.Fatalf("seed edge %s: %v", id, err)
	}
}

// cleanupFixtures removes every row whose id starts with prefix.
// Edges are removed first to avoid FK / cascade noise.
func cleanupFixtures(t *testing.T, conn *pgx.Conn, prefix string) {
	t.Helper()
	ctx := context.Background()
	like := prefix + "-%"
	for _, q := range []string{
		`DELETE FROM edges WHERE id LIKE $1
		   OR src_id LIKE $1 OR dst_id LIKE $1`,
		`DELETE FROM artifacts WHERE id LIKE $1`,
		`DELETE FROM people    WHERE id LIKE $1`,
		`DELETE FROM topics    WHERE id LIKE $1`,
	} {
		if _, err := conn.Exec(ctx, q, like); err != nil {
			t.Logf("cleanup query failed (continuing): %v", err)
		}
	}
}

// errorEnvelope mirrors graphapi.errorEnvelope for parsing 4xx
// responses in assertions.
type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Field   string `json:"field"`
	} `json:"error"`
}

func decodeError(t *testing.T, body []byte) errorEnvelope {
	t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode error envelope: %v body=%s", err, string(body))
	}
	return env
}

// itoa is the strconv-free integer formatter used for generated
// fixture ids — keeps the imports minimal in this helper file.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
