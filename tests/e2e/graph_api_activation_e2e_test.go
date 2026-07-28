//go:build e2e

// BUG-080-001 — fail-soft Knowledge Graph activation, e2e-api tier.
//
// This file ties the SCOPE-01 fail-soft activation contract to the LIVE
// disposable stack over real HTTP (CORE_EXTERNAL_URL + SMACKEREL_AUTH_TOKEN,
// exported by `./smackerel.sh test e2e`). There is NO request interception,
// NO mock, NO stub — every assertion drives the real running smackerel-core
// container end-to-end.
//
// Three SCOPE-01 rows live here (plus two SCOPE-02 rows — see the second
// block below):
//
//   - T080-07-SECURITY (RUNS on the default ENABLED stack). The graph
//     capability surfaces cursor material (the HMAC-signed `nextCursor`) and
//     other activation-adjacent output over the API; this test proves that
//     surfaced output NEVER contains the operator cursor secret — not the raw
//     bytes, not its hex/base64 encodings, not a SHA-256 hash, and not the
//     secret embedded in any cursor body. The runner receives the real secret
//     via `--env-file config/generated/test.env` (KNOWLEDGE_GRAPH_API_CURSOR_SECRET),
//     so the leak needle is the ACTUAL deployed secret, and the assertion is
//     value-safe: the secret and every derived needle are searched-for but
//     NEVER logged.
//
//   - T080-01-DISABLED and T080-02-ADVERSARIAL (require a DISABLED-graph core).
//     A disabled-graph core is provided by the `graph-disabled` phase of
//     `./smackerel.sh test e2e`, which recycles the stack onto
//     docker-compose.graph-disabled.override.yml (that overlay sets
//     KNOWLEDGE_GRAPH_API_CURSOR_SECRET to an EXPLICIT empty value on
//     smackerel-core, so activation resolves the typed DISABLED state) and
//     exports that core's base URL as SMACKEREL_E2E_GRAPH_DISABLED_URL. Both
//     tests below run there unchanged and prove the fail-soft contract over
//     real HTTP: a typed 503 capability_disabled, never a silent 404, an
//     opaque 500, or a panic. They still t.Skip when
//     SMACKEREL_E2E_GRAPH_DISABLED_URL is absent — e.g. a targeted `--go-run`
//     selector that never reaches the graph-disabled phase — so an
//     ENABLED-stack-only run stays meaningful instead of failing spuriously.
//     The same behavior is additionally proven at the integration tier
//     (tests/integration/graphapi/activation_test.go —
//     TestGraphActivationDisabledSecretServesTyped503AndKeepsServing, and
//     route_manifest_test.go — disabled_router_mounts_all_eight_present) and at
//     the unit tier (internal/api/graphapi/activation_test.go —
//     TestAdversarial_EmptySecretMustNotRevertToSilentAbsenceOr500).
//
// Two SCOPE-02 rows live here as well; both RUN on the default ENABLED stack
// and both are the complementary "the capability is UP — now prove it behaves"
// half of the fail-soft contract:
//
//   - T080-03-READONLY (SCN-080-001-03). An authenticated journey across all
//     five graph families (topics, people, places, time, edges) over the real
//     HTTP paths must return authorized, contract-valid data read from real
//     seeded rows — and must leave every graph-owned table's row count
//     UNCHANGED. Reads must not write.
//
//   - T080-05-EMPTY (SCN-080-001-05). A genuinely zero-row family read must be
//     an explicit SUCCESSFUL true-empty (200 + an explicit, non-null, empty
//     array) that is EXCLUSIVE of disabled (503 capability_disabled),
//     route-missing (404), unauthorized (401/403), unavailable (503
//     store_unavailable), and schema failure (500 schema_error). This is the
//     arm that fails the moment someone lets "no rows" degrade into a 404 or a
//     503.
//
//   - T080-06-AUTH (SCN-080-001-06). An expired session and a denied scope must
//     be EXCLUSIVE private outcomes: 401 is never 403, and neither is ever a
//     served 200, a 404 existence oracle, or a 503 capability failure — and no
//     denial body discloses graph content, counts, or existence hints.
//
//   - T080-09-GRANT (SCN-080-001-09). The shared product-wide login grants
//     global-corpus read ONLY through the auth.RequireScope(knowledge-graph:read)
//     gate; an ungranted identity is denied leak-free; and the granted row set
//     is the GLOBAL corpus, verified against DB ground truth so a per-user or
//     per-tenant row predicate fails the test.
//
// Both new rows drive a REAL PASETO v4.public credential minted by
// auth.IssueToken. Where the deployed AUTH_ENABLED=false container cannot reach
// the scope gate, the constraint is declared in-test and the integration-tier
// proof is named — never skipped, never faked. See each test's doc comment.

package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/smackerel/smackerel/internal/api/graphapi"
	"github.com/smackerel/smackerel/internal/auth"
)

// cursorSecretEnvName is the *name* of the env var whose VALUE is the
// operator cursor HMAC secret. The e2e go-test runner receives it via
// `--env-file config/generated/test.env` (line 195), so this test can read
// the real deployed secret and prove it never leaks into API output.
const cursorSecretEnvName = "KNOWLEDGE_GRAPH_API_CURSOR_SECRET"

// disabledGraphStackURL returns the base URL of a smackerel-core booted in
// the fail-soft DISABLED graph activation state. That URL is supplied by the
// `graph-disabled` phase of `./smackerel.sh test e2e`, which recycles the
// stack onto docker-compose.graph-disabled.override.yml and exports
// SMACKEREL_E2E_GRAPH_DISABLED_URL. A skip here therefore means only that the
// current run did not include that phase (e.g. a targeted `--go-run`
// selector), not that the harness is missing. See the file header.
func disabledGraphStackURL(t *testing.T) string {
	t.Helper()
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("SMACKEREL_E2E_GRAPH_DISABLED_URL")), "/")
	if base == "" {
		t.Skip("e2e: SMACKEREL_E2E_GRAPH_DISABLED_URL not set — this test runs in the `graph-disabled` phase of `./smackerel.sh test e2e`, which boots smackerel-core via docker-compose.graph-disabled.override.yml with an explicitly empty " + cursorSecretEnvName + ". A targeted --go-run selector that does not reach that phase skips here by design. The DISABLED fail-soft behavior is additionally proven at the integration tier (T080-01-PROC) and the unit tier (T080-01-UNIT).")
	}
	return base
}

// disabledGraphPaths is the canonical eight-family manifest — every known
// graph path that a DISABLED core MUST answer with a typed 503, never a
// silent Chi 404.
func disabledGraphPaths() []string {
	return []string{
		"/api/topics",
		"/api/topics/does-not-exist",
		"/api/people",
		"/api/people/does-not-exist",
		"/api/places",
		"/api/places/does-not-exist",
		"/api/time",
		"/api/graph/edges",
	}
}

// forbiddenSecretNeedles derives the value-safe set of leak needles from the
// real cursor secret: the raw bytes plus every encoding/hash a leak could
// plausibly surface. The map KEYS are safe to log (a class label); the VALUES
// (the actual needles) are NEVER logged — only searched for.
func forbiddenSecretNeedles(secret string) map[string]string {
	sum := sha256.Sum256([]byte(secret))
	return map[string]string{
		"raw":            secret,
		"hex":            hex.EncodeToString([]byte(secret)),
		"base64_std":     base64.StdEncoding.EncodeToString([]byte(secret)),
		"base64_rawurl":  base64.RawURLEncoding.EncodeToString([]byte(secret)),
		"sha256_hex":     hex.EncodeToString(sum[:]),
		"sha256_b64":     base64.StdEncoding.EncodeToString(sum[:]),
		"sha256_rawurlb": base64.RawURLEncoding.EncodeToString(sum[:]),
	}
}

// headerDump serializes response headers to a single value-safe string so the
// leak scan covers headers as well as bodies. Header VALUES are included in
// the haystack (to catch a secret smuggled into a header) but this function
// never returns the secret unless the server itself leaked it.
func headerDump(h http.Header) string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(strings.Join(h[k], ","))
		b.WriteString("\n")
	}
	return b.String()
}

// TestE2E_GraphActivation_NeverLeaksSecretOrCursorMaterial is T080-07-SECURITY
// (SCN-080-001-07). Against the LIVE ENABLED stack it exercises the graph
// capability's cursor-bearing output (a real HMAC-signed nextCursor, a
// cursor-decode round trip, and an invalid-cursor error envelope) plus health,
// and proves NONE of that surfaced output contains the operator cursor secret
// or a derived leak. The assertion is value-safe: the secret and every needle
// are searched-for, never logged.
func TestE2E_GraphActivation_NeverLeaksSecretOrCursorMaterial(t *testing.T) {
	cfg := loadE2EConfig(t) // skips when CORE_EXTERNAL_URL / token unset
	waitForHealth(t, cfg, 30*time.Second)

	secret := strings.TrimSpace(os.Getenv(cursorSecretEnvName))
	if secret == "" {
		t.Skipf("e2e: %s not present in the runner env — cannot assert secret-absence without the real needle", cursorSecretEnvName)
	}
	if len(secret) < 8 {
		t.Skipf("e2e: %s length=%d is too short for a meaningful leak assertion", cursorSecretEnvName, len(secret))
	}
	needles := forbiddenSecretNeedles(secret)

	// Seed topics so the ENABLED list handler emits a real, HMAC-signed
	// nextCursor — the primary "cursor material" surfaced via the API.
	dbURL := requireEnvForGraphAPI(t)
	conn, err := pgx.Connect(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("pgx.Connect: %v", err)
	}
	defer conn.Close(context.Background())
	prefix := "graph-sec-e2e-" + time.Now().UTC().Format("20060102150405.000000")
	t.Cleanup(func() { graphAPICleanup(t, conn, prefix) })
	topicIDs := graphAPISeedTopics(t, conn, prefix, 3)

	// ENABLED precondition + obtain a real nextCursor to exercise the codec.
	resp, body := graphAPIGet(t, cfg, "/api/topics?limit=1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ENABLED precondition: GET /api/topics?limit=1 status=%d body=%s; want 200 (this security probe requires the live cursor codec)", resp.StatusCode, string(body))
	}
	var firstPage graphAPITopicsList
	if err := json.Unmarshal(body, &firstPage); err != nil {
		t.Fatalf("decode first page: %v body=%s", err, string(body))
	}
	if strings.TrimSpace(firstPage.NextCursor) == "" {
		t.Fatalf("ENABLED precondition: expected a non-empty nextCursor from a paginated list (seeded %d topics, limit=1); cursor material was not exercised", len(topicIDs))
	}

	probes := []struct{ name, path string }{
		{"topics_page1", "/api/topics?limit=1"},
		{"topics_page2_cursor_decode", "/api/topics?limit=1&cursor=" + url.QueryEscape(firstPage.NextCursor)},
		{"people", "/api/people?limit=5"},
		{"places", "/api/places?limit=5"},
		{"time", "/api/time"},
		{"edges_topic_source", "/api/graph/edges?source=topic:" + url.QueryEscape(topicIDs[0]) + "&limit=5"},
		{"invalid_cursor_error", "/api/topics?limit=1&cursor=not-a-valid-cursor-token"},
		{"health", "/api/health"},
	}

	leaks := 0
	for _, p := range probes {
		r, b := graphAPIGet(t, cfg, p.path)
		haystack := string(b) + "\x00" + headerDump(r.Header)
		for _, class := range sortedNeedleClasses(needles) {
			needle := needles[class]
			if needle != "" && strings.Contains(haystack, needle) {
				leaks++
				// Value-safe: report ONLY the probe + needle CLASS, never the
				// secret bytes, its length, or the matched value.
				t.Errorf("SECURITY LEAK: probe %q (%s) surfaced cursor-secret material [class=%s] — value withheld (value-safe)", p.name, p.path, class)
			}
		}
		// Value-safe per-probe evidence: status, body size, and a boolean
		// proving the raw secret is absent — never the secret itself.
		t.Logf("probe %-28s status=%d bodyLen=%d rawSecretAbsent=%v", p.name, r.StatusCode, len(b), !strings.Contains(haystack, secret))
	}

	if leaks == 0 {
		t.Logf("VALUE-SAFE: %d live graph/activation API probes surfaced NO cursor-secret material (raw/hex/base64/sha256); secret length=%d", len(probes), len(secret))
	}
}

// sortedNeedleClasses returns the needle class labels in stable order so the
// scan (and any failure output) is deterministic.
func sortedNeedleClasses(needles map[string]string) []string {
	classes := make([]string, 0, len(needles))
	for c := range needles {
		classes = append(classes, c)
	}
	sort.Strings(classes)
	return classes
}

// TestE2E_GraphActivation_DisabledServesTyped503AndKeepsServing is
// T080-01-DISABLED (SCN-080-001-01). Against a DISABLED-graph core it proves
// every known graph path answers a TYPED 503 capability_disabled (never a
// silent 404 nil-handler absence, an opaque 500, or a panic) while the service
// keeps serving other capabilities. It skips under the default ENABLED-only
// harness (see disabledGraphStackURL).
func TestE2E_GraphActivation_DisabledServesTyped503AndKeepsServing(t *testing.T) {
	base := disabledGraphStackURL(t) // skips under the default harness
	cfg := e2eConfig{CoreURL: base, AuthToken: strings.TrimSpace(os.Getenv("SMACKEREL_AUTH_TOKEN"))}
	waitForHealth(t, cfg, 30*time.Second)

	for _, path := range disabledGraphPaths() {
		resp, body := graphAPIGet(t, cfg, path)
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("path %s status=%d body=%s; want 503 capability_disabled (never a silent 404 / opaque 500)", path, resp.StatusCode, string(body))
			continue
		}
		var env struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &env); err != nil || env.Error.Code != "capability_disabled" {
			t.Errorf("path %s: body is not a typed capability_disabled envelope: %s", path, string(body))
		}
	}

	// The disabled-graph core MUST keep serving other capabilities.
	resp, body := graphAPIGet(t, cfg, "/api/health")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disabled-graph core must keep serving /api/health; status=%d body=%s", resp.StatusCode, string(body))
	}
}

// TestE2E_GraphActivation_DisabledAdversarialRedGreen is T080-02-ADVERSARIAL
// (SCN-080-001-02 fail-soft leg). The assertion is constructed to FAIL against
// the ORIGINAL bug — an empty secret left graph handlers nil and OMITTED the
// routes, so /api/topics/ returned a bare Chi 404 (silent absence); an opaque
// 500 would be an equally-broken degradation. It PASSES only against the fixed
// fail-soft repair (a TYPED 503 capability_disabled with an atomic 8-route
// manifest). It skips under the default ENABLED-only harness. The equivalent
// red→green contrast is ALREADY proven at the unit tier
// (TestAdversarial_EmptySecretMustNotRevertToSilentAbsenceOr500).
func TestE2E_GraphActivation_DisabledAdversarialRedGreen(t *testing.T) {
	base := disabledGraphStackURL(t) // skips under the default harness
	cfg := e2eConfig{CoreURL: base, AuthToken: strings.TrimSpace(os.Getenv("SMACKEREL_AUTH_TOKEN"))}
	waitForHealth(t, cfg, 30*time.Second)

	// Adversarial target: the exact path the pre-fix router 404'd on.
	resp, body := graphAPIGet(t, cfg, "/api/topics/")
	switch resp.StatusCode {
	case http.StatusNotFound:
		t.Fatalf("RED reproduced: GET /api/topics/ returned a silent 404 nil-handler absence — the original BUG-080-001 behavior; the fail-soft repair regressed")
	case http.StatusInternalServerError:
		t.Fatalf("GET /api/topics/ returned an opaque 500 — the disabled state must be a typed 503, not an opaque failure")
	case http.StatusServiceUnavailable:
		var env struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &env); err != nil || env.Error.Code != "capability_disabled" {
			t.Fatalf("GREEN 503 but not a typed capability_disabled envelope: %s", string(body))
		}
		// GREEN: typed 503 capability_disabled — distinct from the RED 404/500.
	default:
		t.Fatalf("GET /api/topics/ status=%d; want 503 capability_disabled (never 404 silent-absence or 500 opaque)", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// SCOPE-02 — read-only journey (T080-03-READONLY) and true-empty exclusivity
// (T080-05-EMPTY). Both run against the default ENABLED live stack.
// ---------------------------------------------------------------------------

// graphAPITimeArtifact / graphAPITimeDay / graphAPITimeWindow mirror the
// /api/time wire envelope (internal/api/graphapi/time.go). Note the family's
// array key is `days`, NOT `items` — the true-empty assertion is parameterized
// by field name for exactly this reason.
type graphAPITimeArtifact struct {
	ArtifactID string `json:"artifactId"`
	Title      string `json:"title"`
}

type graphAPITimeDay struct {
	Date      string                 `json:"date"`
	Artifacts []graphAPITimeArtifact `json:"artifacts"`
}

type graphAPITimeWindow struct {
	Days []graphAPITimeDay `json:"days"`
}

// graphTableCountTargets is the closed set of graph-owned Postgres tables
// whose row counts MUST be identical before and after a read-only family
// journey (SCN-080-001-03). There is no first-class `places` table — places.go
// derives that family from `location_clusters` plus `artifacts.location_geo`,
// so both real backing sources are counted instead.
func graphTableCountTargets() []string {
	return []string{"topics", "people", "artifacts", "edges", "location_clusters"}
}

// graphTableRowCounts snapshots the row count of every graph-owned table. The
// table names come from the fixed literal set above and NEVER from request
// input or response data, so the interpolation is not a dynamic-SQL surface.
func graphTableRowCounts(t *testing.T, conn *pgx.Conn) map[string]int64 {
	t.Helper()
	counts := make(map[string]int64, len(graphTableCountTargets()))
	for _, table := range graphTableCountTargets() {
		var n int64
		if err := conn.QueryRow(context.Background(), `SELECT COUNT(*) FROM `+table).Scan(&n); err != nil {
			t.Fatalf("count rows in %s: %v", table, err)
		}
		counts[table] = n
	}
	return counts
}

// graphAPICollectListIDs walks a cursor-paginated list family over real HTTP
// and returns every item id it observes. Every page must be a 200 carrying a
// contract-valid, PRESENT `items` array (an absent or null array is a contract
// violation, not an empty page). maxPages bounds the walk so a cursor
// non-termination regression FAILS the test instead of hanging the suite.
// basePath MUST already carry a query string (e.g. "/api/topics?limit=200").
func graphAPICollectListIDs(t *testing.T, cfg e2eConfig, family, basePath string, maxPages int) []string {
	t.Helper()
	ids := make([]string, 0, 64)
	cursor := ""
	for pageIdx := 0; pageIdx < maxPages; pageIdx++ {
		path := basePath
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		resp, body := graphAPIGet(t, cfg, path)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s family: GET %s status=%d body=%s; want 200 — an authenticated family read must be authorized and contract-valid", family, path, resp.StatusCode, string(body))
		}
		var page struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
			NextCursor string `json:"nextCursor"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatalf("%s family: decode %s: %v body=%s", family, path, err, string(body))
		}
		// encoding/json leaves a nil slice for an ABSENT or `null` key and an
		// allocated empty slice for `[]`, so nil here means the envelope is
		// contract-invalid rather than merely empty.
		if page.Items == nil {
			t.Fatalf("%s family: %s returned 200 without a contract-valid items array (absent or null): %s", family, path, string(body))
		}
		for _, item := range page.Items {
			if strings.TrimSpace(item.ID) == "" {
				t.Fatalf("%s family: %s returned an item with an empty id — contract violation: %s", family, path, string(body))
			}
			ids = append(ids, item.ID)
		}
		cursor = strings.TrimSpace(page.NextCursor)
		if cursor == "" {
			return ids
		}
	}
	t.Fatalf("%s family: cursor walk did not terminate within %d pages — pagination contract regression", family, maxPages)
	return nil
}

// graphAPIIDSet indexes a slice of ids for membership assertions.
func graphAPIIDSet(ids []string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

// TestE2E_GraphFamilyJourneyIsReadOnly_T080_03_READONLY is T080-03-READONLY
// (SCN-080-001-03): "Regression: authenticated family journey reads real rows
// without graph writes".
//
// Against the LIVE ENABLED stack it seeds disposable fixtures, snapshots every
// graph-owned table's row count, drives one authenticated read journey across
// ALL FIVE families over the production HTTP paths (/api/topics, /api/people,
// /api/places, /api/time, /api/graph/edges), asserts each response is a 200
// carrying contract-valid JSON that surfaces the REAL seeded rows, then
// re-snapshots the counts and asserts they are UNCHANGED. A read path that
// starts writing — a lazy backfill, an access-log row, a derived-entity
// upsert — fails this test.
func TestE2E_GraphFamilyJourneyIsReadOnly_T080_03_READONLY(t *testing.T) {
	cfg := loadE2EConfig(t) // skips when CORE_EXTERNAL_URL / token unset
	waitForHealth(t, cfg, 30*time.Second)

	dbURL := requireEnvForGraphAPI(t)
	conn, err := pgx.Connect(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("pgx.Connect: %v", err)
	}
	// Ordering matters: t.Cleanup runs LIFO and ALWAYS after deferred calls, so a
	// `defer conn.Close(...)` here would close the pool before the fixture DELETEs
	// ran and silently leave every seeded row behind. Registering the close FIRST
	// makes it run LAST, after the cleanup registered below.
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	// Disposable, uniquely-prefixed fixtures; cleanup is registered BEFORE the
	// journey so a mid-test failure still tears the fixtures down.
	prefix := "graph-readonly-e2e-" + time.Now().UTC().Format("20060102150405.000000")
	t.Cleanup(func() { graphAPICleanup(t, conn, prefix) })

	topicIDs := graphAPISeedTopics(t, conn, prefix, 2)
	personIDs := graphAPISeedPeople(t, conn, prefix, 1)
	artifactIDs := graphAPISeedArtifacts(t, conn, prefix, 2)
	for _, aid := range artifactIDs {
		graphAPISeedEdge(t, conn, prefix, "artifact", aid, "topic", topicIDs[0], "mentions", 1.0)
		graphAPISeedEdge(t, conn, prefix, "artifact", aid, "person", personIDs[0], "mentions", 1.0)
	}
	graphAPISeedEdge(t, conn, prefix, "topic", topicIDs[0], "person", personIDs[0], "co-occurs", 1.0)

	// BEFORE: taken after seeding, immediately before the read journey.
	before := graphTableRowCounts(t, conn)

	// ---- family 1/5: topics — real seeded rows must be readable ----
	seenTopics := graphAPIIDSet(graphAPICollectListIDs(t, cfg, "topics", "/api/topics?limit=200", 25))
	for _, want := range topicIDs {
		if !seenTopics[want] {
			t.Errorf("topics family: seeded topic %q absent from the authenticated /api/topics read — the journey did not read real rows", want)
		}
	}
	t.Logf("read topics   families=1/5 observedIDs=%d seededVisible=%d/%d", len(seenTopics), len(topicIDs), len(topicIDs))

	// ---- family 2/5: people ----
	seenPeople := graphAPIIDSet(graphAPICollectListIDs(t, cfg, "people", "/api/people?limit=200", 25))
	for _, want := range personIDs {
		if !seenPeople[want] {
			t.Errorf("people family: seeded person %q absent from the authenticated /api/people read — the journey did not read real rows", want)
		}
	}
	t.Logf("read people   families=2/5 observedIDs=%d", len(seenPeople))

	// ---- family 3/5: places ----
	// Places are DERIVED (no first-class table — see places.go), so this family
	// cannot be seeded deterministically. The contract assertion is therefore
	// the one that matters: 200 with a present, contract-valid items array.
	seenPlaces := graphAPICollectListIDs(t, cfg, "places", "/api/places?limit=200", 25)
	t.Logf("read places   families=3/5 observedIDs=%d (derived family; contract-valid array asserted)", len(seenPlaces))

	// ---- family 4/5: time — a valid window that contains the seeded artifacts ----
	now := time.Now().UTC()
	from := now.Add(-3 * 24 * time.Hour).Format(time.RFC3339)
	to := now.Add(24 * time.Hour).Format(time.RFC3339)
	timePath := "/api/time?from=" + url.QueryEscape(from) + "&to=" + url.QueryEscape(to)
	resp, body := graphAPIGet(t, cfg, timePath)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("time family: GET %s status=%d body=%s; want 200", timePath, resp.StatusCode, string(body))
	}
	var window graphAPITimeWindow
	if err := json.Unmarshal(body, &window); err != nil {
		t.Fatalf("time family: decode %s: %v body=%s", timePath, err, string(body))
	}
	if window.Days == nil {
		t.Fatalf("time family: 200 without a contract-valid days array (absent or null): %s", string(body))
	}
	seenArtifacts := map[string]bool{}
	for _, day := range window.Days {
		if strings.TrimSpace(day.Date) == "" {
			t.Errorf("time family: a day bucket has an empty date — contract violation: %s", string(body))
		}
		for _, art := range day.Artifacts {
			seenArtifacts[art.ArtifactID] = true
		}
	}
	for _, want := range artifactIDs {
		if !seenArtifacts[want] {
			t.Errorf("time family: seeded artifact %q absent from the %s window — the journey did not read real rows", want, timePath)
		}
	}
	t.Logf("read time     families=4/5 dayBuckets=%d observedArtifacts=%d", len(window.Days), len(seenArtifacts))

	// ---- family 5/5: edges — the seeded artifact must project its real links ----
	edgesPath := "/api/graph/edges?source=artifact:" + url.QueryEscape(artifactIDs[0]) + "&limit=500"
	resp, body = graphAPIGet(t, cfg, edgesPath)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("edges family: GET %s status=%d body=%s; want 200", edgesPath, resp.StatusCode, string(body))
	}
	var edges graphAPIEdgesList
	if err := json.Unmarshal(body, &edges); err != nil {
		t.Fatalf("edges family: decode %s: %v body=%s", edgesPath, err, string(body))
	}
	if edges.Items == nil {
		t.Fatalf("edges family: 200 without a contract-valid items array (absent or null): %s", string(body))
	}
	if len(edges.Items) < 2 {
		t.Errorf("edges family: %s returned %d cross-links; want >=2 (the seeded artifact links a topic AND a person) — the journey did not read real rows", edgesPath, len(edges.Items))
	}
	for _, link := range edges.Items {
		if strings.TrimSpace(link.TargetKind) == "" || strings.TrimSpace(link.TargetID) == "" || strings.TrimSpace(link.Reason) == "" {
			t.Errorf("edges family: cross-link is not contract-valid (targetKind/targetId/reason must all be non-empty): %+v", link)
		}
	}
	t.Logf("read edges    families=5/5 crossLinks=%d", len(edges.Items))

	// AFTER: the read-only invariant. Reads MUST NOT write.
	after := graphTableRowCounts(t, conn)
	drift := 0
	for _, table := range graphTableCountTargets() {
		if before[table] != after[table] {
			drift++
			t.Errorf("READ-ONLY VIOLATION: graph table %q row count changed across the authenticated five-family read journey: before=%d after=%d delta=%+d — a graph READ path performed a WRITE", table, before[table], after[table], after[table]-before[table])
		}
	}
	if drift == 0 {
		t.Logf("READ-ONLY OK: all 5 families read over real HTTP; %d graph tables unchanged %v", len(graphTableCountTargets()), graphTableCountTargets())
	}
}

// graphAPIAssertTrueEmpty is the T080-05-EMPTY assertion core. Given a family
// probe whose filter is GUARANTEED to match nothing, it proves the response is
// an explicit SUCCESSFUL true-empty and is EXCLUSIVE of every other closed read
// outcome:
//
//   - 404 → route-missing / resource-absence (the pre-fix silent-absence class)
//   - 503 → capability_disabled OR store_unavailable
//   - 401 / 403 → unauthenticated / forbidden
//   - 500 → schema_error / internal failure
//
// Only an exact 200 whose body carries NO error envelope and whose named array
// field is PRESENT, non-null, and EMPTY passes.
func graphAPIAssertTrueEmpty(t *testing.T, probe, path, field string, resp *http.Response, body []byte) {
	t.Helper()

	switch resp.StatusCode {
	case http.StatusOK:
		// The only acceptable outcome for a successful zero-row read.
	case http.StatusNotFound:
		t.Errorf("TRUE-EMPTY VIOLATION: probe %q (%s) returned 404 — a successful zero-row read degraded into route-missing / resource-absence; true-empty MUST be an explicit 200. body=%s", probe, path, string(body))
		return
	case http.StatusServiceUnavailable:
		t.Errorf("TRUE-EMPTY VIOLATION: probe %q (%s) returned 503 — a successful zero-row read degraded into capability_disabled / store_unavailable; true-empty MUST be an explicit 200. body=%s", probe, path, string(body))
		return
	case http.StatusUnauthorized, http.StatusForbidden:
		t.Errorf("TRUE-EMPTY VIOLATION: probe %q (%s) returned %d — a successful zero-row read degraded into an authorization outcome; true-empty MUST be an explicit 200. body=%s", probe, path, resp.StatusCode, string(body))
		return
	case http.StatusInternalServerError:
		t.Errorf("TRUE-EMPTY VIOLATION: probe %q (%s) returned 500 — a successful zero-row read degraded into schema_error / internal failure; true-empty MUST be an explicit 200. body=%s", probe, path, string(body))
		return
	default:
		t.Errorf("TRUE-EMPTY VIOLATION: probe %q (%s) status=%d; want exactly 200 for a successful zero-row read. body=%s", probe, path, resp.StatusCode, string(body))
		return
	}

	envelope := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Errorf("probe %q (%s): 200 body is not a JSON object envelope: %v body=%s", probe, path, err, string(body))
		return
	}
	if raw, ok := envelope["error"]; ok && strings.TrimSpace(string(raw)) != "null" {
		t.Errorf("TRUE-EMPTY VIOLATION: probe %q (%s) returned 200 carrying an ERROR envelope — a true-empty read is a success, never an error: %s", probe, path, string(body))
		return
	}
	raw, ok := envelope[field]
	if !ok {
		t.Errorf("TRUE-EMPTY VIOLATION: probe %q (%s): 200 body OMITS the %q array — true-empty MUST carry an explicit empty array, never an absent field. body=%s", probe, path, field, string(body))
		return
	}
	if strings.TrimSpace(string(raw)) == "null" {
		t.Errorf("TRUE-EMPTY VIOLATION: probe %q (%s): %q is JSON null — true-empty MUST be an explicit empty array [], never null. body=%s", probe, path, field, string(body))
		return
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Errorf("probe %q (%s): %q is not a JSON array: %v body=%s", probe, path, field, err, string(body))
		return
	}
	if len(items) != 0 {
		t.Errorf("probe %q (%s): %q carries %d items, but this probe's filter is guaranteed to match nothing — the filter no longer isolates a zero-row read, so the true-empty arm is not being exercised. body=%s", probe, path, field, len(items), string(body))
		return
	}
	t.Logf("true-empty OK  %-34s status=200 %s=[] (explicit, non-null, no error envelope)", probe, field)
}

// TestE2E_AllFamilyTrueEmptyIsSuccessNotFailure_T080_05_EMPTY is T080-05-EMPTY
// (SCN-080-001-05): "Regression: successful all-family empty is not activation
// or dependency failure".
//
// Against the LIVE ENABLED stack it drives every required family with a filter
// GUARANTEED to match nothing — a unique nonexistent id prefix for the four
// node families, and a far-past window for the time family — and proves each
// response is an explicit successful true-empty (exact 200 + present,
// non-null, EMPTY array + no error envelope), DISTINCT from disabled,
// route-missing, unauthorized, unavailable, and schema failure.
//
// A second group covers the highest-risk degradation shape directly: a
// resource that genuinely EXISTS but has ZERO links. Its detail read must
// still be a 200 with empty cross-link arrays — never a 404. This test fails
// the moment anyone lets "no rows" collapse into a 404 or a 503.
func TestE2E_AllFamilyTrueEmptyIsSuccessNotFailure_T080_05_EMPTY(t *testing.T) {
	cfg := loadE2EConfig(t) // skips when CORE_EXTERNAL_URL / token unset
	waitForHealth(t, cfg, 30*time.Second)

	dbURL := requireEnvForGraphAPI(t)
	conn, err := pgx.Connect(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("pgx.Connect: %v", err)
	}
	// See the ordering note in TestE2E_GraphFamilyJourneyIsReadOnly_T080_03_READONLY:
	// the close is registered FIRST so it runs LAST, after the fixture cleanup.
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	prefix := "graph-empty-e2e-" + time.Now().UTC().Format("20060102150405.000000")
	t.Cleanup(func() { graphAPICleanup(t, conn, prefix) })

	// A unique prefix that is deliberately NEVER inserted anywhere: every read
	// filtered by it is guaranteed to match zero rows.
	missing := prefix + "-never-inserted"

	// ---- Group A: all five families, each filtered to a guaranteed-zero result.
	// The time family's array key is `days`; the rest use `items`.
	emptyProbes := []struct{ probe, path, field string }{
		{
			probe: "topics_family_empty",
			path:  "/api/graph/edges?source=topic:" + url.QueryEscape(missing+"-topic") + "&limit=500",
			field: "items",
		},
		{
			probe: "people_family_empty",
			path:  "/api/graph/edges?source=person:" + url.QueryEscape(missing+"-person") + "&limit=500",
			field: "items",
		},
		{
			probe: "places_family_empty",
			path:  "/api/graph/edges?source=place:" + url.QueryEscape(missing+"-place") + "&limit=500",
			field: "items",
		},
		{
			// Far past: the corpus cannot contain a 1970 capture. 151 days is
			// well inside KNOWLEDGE_GRAPH_API_TIME_WINDOW_MAX_DAYS=365, so a
			// 400 invalid_window can never mask the true-empty result.
			probe: "time_family_empty",
			path:  "/api/time?from=1970-01-01T00:00:00Z&to=1970-06-01T00:00:00Z",
			field: "days",
		},
		{
			probe: "edges_family_empty",
			path:  "/api/graph/edges?source=artifact:" + url.QueryEscape(missing+"-artifact") + "&limit=500",
			field: "items",
		},
	}
	for _, p := range emptyProbes {
		resp, body := graphAPIGet(t, cfg, p.path)
		graphAPIAssertTrueEmpty(t, p.probe, p.path, p.field, resp, body)
	}

	// ---- Group B: resources that genuinely EXIST but have ZERO links.
	// This is the degradation the fail-soft repair must never re-introduce:
	// "this row has no neighbours" is a 200 with empty arrays, NOT a 404.
	// The fixtures below are seeded WITHOUT any edge on purpose.
	unlinkedTopic := graphAPISeedTopics(t, conn, prefix, 1)[0]
	unlinkedPerson := graphAPISeedPeople(t, conn, prefix, 1)[0]

	topicDetailPath := "/api/topics/" + url.PathEscape(unlinkedTopic)
	resp, body := graphAPIGet(t, cfg, topicDetailPath)
	for _, field := range []string{"linkedArtifacts", "relatedPeople", "relatedPlaces"} {
		graphAPIAssertTrueEmpty(t, "existing_topic_zero_links["+field+"]", topicDetailPath, field, resp, body)
	}

	personDetailPath := "/api/people/" + url.PathEscape(unlinkedPerson)
	resp, body = graphAPIGet(t, cfg, personDetailPath)
	for _, field := range []string{"artifactTimeline", "relatedTopics", "relatedPlaces"} {
		graphAPIAssertTrueEmpty(t, "existing_person_zero_links["+field+"]", personDetailPath, field, resp, body)
	}

	if !t.Failed() {
		t.Logf("TRUE-EMPTY OK: %d guaranteed-zero family probes + 6 zero-link detail arrays all returned an explicit successful true-empty (200, present non-null empty array, no error envelope) — exclusive of disabled/route-missing/unauthorized/unavailable/schema-failure", len(emptyProbes))
	}
}

// ---------------------------------------------------------------------------
// SCOPE-02 — auth exclusivity (T080-06-AUTH) and the shared-login global-corpus
// grant matrix (T080-09-GRANT). Both run against the default ENABLED live
// stack over real HTTP; there is NO interception, NO mock, NO stub.
// ---------------------------------------------------------------------------

const (
	// graphAuthIssuer / graphAuthKeyID mirror the `iss` and footer `kid`
	// cmd/core wires, so a token minted here is a genuinely well-formed
	// per-user credential rather than a decorative string.
	graphAuthIssuer = "smackerel"
	graphAuthKeyID  = "bug080e2e-auth-probe-k1"

	// graphAuthUngrantedScope is a REAL product scope (router.go gates the
	// annotation surface with it) that is deliberately NOT the graph read
	// grant. An identity carrying it is authenticated and scoped — just not
	// for the knowledge graph.
	graphAuthUngrantedScope = "annotation:edit"
)

// graphAuthGetWithBearer drives one graph path over real HTTP with an EXPLICIT
// credential instead of the shared bearer that graphAPIGet always attaches.
// bearer == "" sends NO Authorization header at all (the unauthenticated arm);
// rawAuthorization, when non-empty, is sent verbatim so a malformed scheme can
// be probed. `Accept: application/json` is always set because
// isBrowserNavigation (internal/api/auth_browser_redirect.go) turns an
// Accept: text/html GET into a 303 login redirect — this probe asserts the API
// wire contract, not the browser one.
func graphAuthGetWithBearer(t *testing.T, cfg e2eConfig, path, bearer, rawAuthorization string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, cfg.CoreURL+path, nil)
	if err != nil {
		t.Fatalf("NewRequest(%s): %v", path, err)
	}
	req.Header.Set("Accept", "application/json")
	switch {
	case rawAuthorization != "":
		req.Header.Set("Authorization", rawAuthorization)
	case bearer != "":
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	body, _ := readBody(resp)
	return resp, body
}

// graphAuthMintToken issues a REAL PASETO v4.public token under an ephemeral
// signing key. `issuedAt` is injected so an ALREADY-EXPIRED credential can be
// minted honestly (issuedAt in the past + a TTL that has already elapsed)
// rather than faked with a corrupted string.
func graphAuthMintToken(t *testing.T, privateHex, userID string, scopes []string, issuedAt time.Time, ttl time.Duration) string {
	t.Helper()
	res, err := auth.IssueToken(auth.IssueOptions{
		UserID:     userID,
		TokenID:    userID + "-token",
		SigningKey: privateHex,
		KeyID:      graphAuthKeyID,
		TTL:        ttl,
		Issuer:     graphAuthIssuer,
		Now:        func() time.Time { return issuedAt },
		Scopes:     scopes,
	})
	if err != nil {
		t.Fatalf("auth.IssueToken(%s, scopes=%v): %v", userID, scopes, err)
	}
	return res.WireToken
}

// graphAuthAssertNoCorpusShapeField walks a decoded denial body and fails on
// ANY key that would disclose corpus size or content shape — `total`, anything
// containing `count`, an `items` collection, or a pagination cursor. A denied
// caller must not be able to infer that the corpus exists, let alone how large
// it is.
func graphAuthAssertNoCorpusShapeField(t *testing.T, label string, v any, jsonPath string) {
	t.Helper()
	switch node := v.(type) {
	case map[string]any:
		for k, child := range node {
			lower := strings.ToLower(k)
			if lower == "total" || lower == "items" || lower == "nextcursor" || strings.Contains(lower, "count") {
				t.Errorf("LEAK: %s — denial body exposes corpus-shape field %q at %s. A denied identity must not learn the corpus exists or how big it is.", label, k, jsonPath+"."+k)
			}
			graphAuthAssertNoCorpusShapeField(t, label, child, jsonPath+"."+k)
		}
	case []any:
		for _, child := range node {
			graphAuthAssertNoCorpusShapeField(t, label, child, jsonPath+"[]")
		}
	}
}

// graphAuthAssertDenialLeaksNothing is the leak-free denial contract: the body
// is the typed JSON envelope, discloses NO seeded id/label/title, and carries
// NO count/total/items/cursor disclosure.
func graphAuthAssertDenialLeaksNothing(t *testing.T, label, path string, body []byte, seeded []string) {
	t.Helper()
	lower := strings.ToLower(string(body))
	for _, needle := range seeded {
		if needle == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(needle)) {
			t.Errorf("LEAK: %s (%s) — denial body discloses seeded graph content %q. body=%s", label, path, needle, string(body))
		}
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		// A non-JSON denial body is itself a contract break: the denial must
		// be the typed envelope, never an arbitrary (possibly content-bearing)
		// payload.
		t.Errorf("%s (%s): denial body is not the typed JSON envelope: %v body=%s", label, path, err, string(body))
		return
	}
	graphAuthAssertNoCorpusShapeField(t, label, decoded, "$")
}

// graphAuthAssertExclusive pins one auth arm to EXACTLY one status and names
// every competing outcome explicitly, so a regression that collapses the
// distinction (a 401 that becomes a 403, an auth failure that becomes a served
// 200, a route-missing 404 existence oracle, or a capability 503) fails with a
// message that identifies which exclusivity broke.
func graphAuthAssertExclusive(t *testing.T, arm, path string, want int, resp *http.Response, body []byte) bool {
	t.Helper()
	got := resp.StatusCode
	if got == want {
		return true
	}
	switch got {
	case http.StatusOK:
		t.Errorf("EXCLUSIVITY VIOLATION: arm %q (%s) was SERVED 200 — an unauthenticated/expired/ungranted identity must NEVER receive graph content; want %d. body=%s", arm, path, want, string(body))
	case http.StatusNotFound:
		t.Errorf("EXCLUSIVITY VIOLATION: arm %q (%s) returned 404 — an auth failure degraded into route-missing/resource-absence, which is an existence oracle; want %d. body=%s", arm, path, want, string(body))
	case http.StatusServiceUnavailable:
		t.Errorf("EXCLUSIVITY VIOLATION: arm %q (%s) returned 503 — an auth failure degraded into capability_disabled/store_unavailable; want %d. body=%s", arm, path, want, string(body))
	case http.StatusUnauthorized, http.StatusForbidden:
		t.Errorf("EXCLUSIVITY VIOLATION: arm %q (%s) returned %d but this arm's outcome is EXCLUSIVELY %d — 401 (no valid session) and 403 (valid session, absent grant) must never be conflated. body=%s", arm, path, got, want, string(body))
	default:
		t.Errorf("EXCLUSIVITY VIOLATION: arm %q (%s) status=%d; want exactly %d. body=%s", arm, path, got, want, string(body))
	}
	return false
}

// graphAuthSeededNeedles builds the closed set of strings that MUST NOT appear
// in any denial body. Every seeded id doubles as its label/name (see the
// graphAPISeed* helpers) and seedArtifacts writes "<id>-title".
func graphAuthSeededNeedles(topicIDs, personIDs, artifactIDs []string) []string {
	needles := make([]string, 0, len(topicIDs)+len(personIDs)+2*len(artifactIDs))
	needles = append(needles, topicIDs...)
	needles = append(needles, personIDs...)
	for _, id := range artifactIDs {
		needles = append(needles, id, id+"-title")
	}
	return needles
}

// graphAuthDBIDs reads the DB ground truth for one graph-owned table restricted
// to a fixture prefix. The table name comes from a fixed literal call site and
// NEVER from request input or response data, so the interpolation is not a
// dynamic-SQL surface. This is the authority the HTTP projection is compared
// against: if a per-user/tenant row predicate is ever introduced on a read
// path, the HTTP projection becomes a STRICT SUBSET of this set and
// T080-09-GRANT fails.
func graphAuthDBIDs(t *testing.T, conn *pgx.Conn, table, prefix string) []string {
	t.Helper()
	rows, err := conn.Query(context.Background(), `SELECT id FROM `+table+` WHERE id LIKE $1 ORDER BY id`, prefix+"-%")
	if err != nil {
		t.Fatalf("ground truth: select %s for prefix %s: %v", table, prefix, err)
	}
	defer rows.Close()
	ids := make([]string, 0, 8)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("ground truth: scan %s id: %v", table, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("ground truth: iterate %s: %v", table, err)
	}
	return ids
}

// TestE2E_ExpiredSessionAndDeniedScopeAreExclusivePrivateOutcomes_T080_06_AUTH
// is T080-06-AUTH (SCN-080-001-06): "Regression: expired session and denied
// scope return exclusive private outcomes".
//
// Against the LIVE ENABLED stack it drives the canonical eight-path graph
// manifest with four DISTINCT credential classes and proves each failure is an
// EXCLUSIVE typed outcome that leaks nothing:
//
//   - no Authorization header            → exactly 401
//   - malformed bearer / wrong scheme    → exactly 401
//   - a genuinely EXPIRED per-user token → exactly 401
//   - an authenticated identity WITHOUT knowledge-graph:read → denied
//
// Exclusivity means a 401 is never a 403, and neither is ever a 200 (served),
// a 404 (existence oracle), or a 503 (capability/dependency). Leak-freedom
// means the denial body discloses no seeded id/label/title and no
// count/total/items/cursor field — a denied caller cannot learn the graph
// exists or how large it is.
//
// LIVE-STACK CONSTRAINT (declared honestly, never faked). The scope-denial arm
// is driven with a REAL PASETO v4.public token minted by auth.IssueToken
// carrying the real, non-graph scope "annotation:edit" — the exact identity
// that tests/integration/graphapi/corpus_authorization_test.go proves receives
// a 403 scope_required from auth.RequireScope(graphapi.GraphReadScope). The
// deployed test container, however, runs SMACKEREL_ENV=test with
// AUTH_ENABLED=false and NO signing key (config/generated/test.env), so
// bearerAuthMiddleware's perUserActive branch is inactive and every non-shared
// credential is rejected at the shared-token compare BEFORE the scope gate is
// reached (documented in tests/integration/graphapi/auth_test.go —
// TestGraphAPI_403_MissingScope_LiveStackConstraint). This test therefore
// asserts everything that IS reachable here — the ungranted identity is DENIED,
// is never served, and leaks nothing — and reports which denial code the live
// flavor produced. If a per-user flavor is ever wired, the 403 branch below
// asserts the full scope_required contract automatically; nothing needs to
// change and no arm is silently dropped.
func TestE2E_ExpiredSessionAndDeniedScopeAreExclusivePrivateOutcomes_T080_06_AUTH(t *testing.T) {
	cfg := loadE2EConfig(t) // skips when CORE_EXTERNAL_URL / token unset
	waitForHealth(t, cfg, 30*time.Second)

	dbURL := requireEnvForGraphAPI(t)
	conn, err := pgx.Connect(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("pgx.Connect: %v", err)
	}
	// See the ordering note in TestE2E_GraphFamilyJourneyIsReadOnly_T080_03_READONLY:
	// t.Cleanup is LIFO, so the close is registered FIRST to run LAST — otherwise
	// the pool closes before the fixture DELETEs and every seeded row survives.
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	prefix := "graph-auth-e2e-" + time.Now().UTC().Format("20060102150405.000000")
	t.Cleanup(func() { graphAPICleanup(t, conn, prefix) })

	// Real rows so leak-freedom is asserted against content that genuinely
	// EXISTS behind the denial — a denial body that leaks nothing when the
	// corpus is empty would prove nothing.
	topicIDs := graphAPISeedTopics(t, conn, prefix, 2)
	personIDs := graphAPISeedPeople(t, conn, prefix, 1)
	artifactIDs := graphAPISeedArtifacts(t, conn, prefix, 1)
	needles := graphAuthSeededNeedles(topicIDs, personIDs, artifactIDs)

	// Precondition: the shared operator credential IS served, so every denial
	// below is attributable to the credential and not to a down capability.
	okResp, okBody := graphAPIGet(t, cfg, "/api/topics?limit=1")
	if okResp.StatusCode != http.StatusOK {
		t.Fatalf("ENABLED precondition: GET /api/topics?limit=1 with the shared bearer status=%d body=%s; want 200 — the auth exclusivity matrix requires a live, granted baseline", okResp.StatusCode, string(okBody))
	}

	privateHex, _ := auth.GenerateSigningKeypair()

	// A genuinely EXPIRED credential: issued two hours ago with a one-hour TTL,
	// so ExpiresAt is firmly in the past. Not a corrupted string — a real,
	// correctly-signed token whose session has ended.
	expiredToken := graphAuthMintToken(t, privateHex, prefix+"-expired-session",
		[]string{graphapi.GraphReadScope}, time.Now().UTC().Add(-2*time.Hour), time.Hour)

	// An authenticated identity whose scope set deliberately EXCLUDES the graph
	// read grant.
	ungrantedToken := graphAuthMintToken(t, privateHex, prefix+"-ungranted",
		[]string{graphAuthUngrantedScope}, time.Now().UTC(), time.Hour)

	// Anti-tautology self-check: if anyone ever widens the ungranted scope set
	// to include the graph grant, this test would silently stop probing a
	// denial. Fail loudly instead.
	if graphAuthUngrantedScope == graphapi.GraphReadScope {
		t.Fatalf("test invalid: the 'ungranted' identity carries %q, which IS the graph read grant — the scope-denial arm would be vacuous", graphapi.GraphReadScope)
	}

	paths := disabledGraphPaths() // the canonical eight-family manifest

	// ---- Arms 1-3: every no-valid-session class is EXCLUSIVELY 401. ----
	unauthArms := []struct{ name, bearer, raw string }{
		{name: "missing_authorization_header", bearer: "", raw: ""},
		{name: "malformed_bearer_token", bearer: "not-a-real-token", raw: ""},
		{name: "malformed_authorization_scheme", bearer: "", raw: "Token " + expiredToken},
		{name: "expired_per_user_session", bearer: expiredToken, raw: ""},
	}
	for _, arm := range unauthArms {
		exclusive := 0
		for _, path := range paths {
			resp, body := graphAuthGetWithBearer(t, cfg, path, arm.bearer, arm.raw)
			if graphAuthAssertExclusive(t, arm.name, path, http.StatusUnauthorized, resp, body) {
				exclusive++
			}
			graphAuthAssertDenialLeaksNothing(t, arm.name, path, body, needles)
		}
		t.Logf("arm %-32s exclusive401=%d/%d leak-free across the 8-path graph manifest", arm.name, exclusive, len(paths))
	}

	// ---- Arm 4: authenticated identity WITHOUT knowledge-graph:read. ----
	// The outcome is recorded from the live stack rather than assumed. 403 is
	// the scope-gate contract; 401 is the documented AUTH_ENABLED=false
	// constraint. 200 / 404 / 503 are regressions in EITHER flavor.
	scopeDenialCodes := map[int]int{}
	for _, path := range paths {
		resp, body := graphAuthGetWithBearer(t, cfg, path, ungrantedToken, "")
		switch resp.StatusCode {
		case http.StatusForbidden:
			// Per-user enforcement is live: assert the full scope contract.
			var env struct {
				Error    string   `json:"error"`
				Required []string `json:"required"`
			}
			if err := json.Unmarshal(body, &env); err != nil || env.Error != "scope_required" {
				t.Errorf("arm scope_denied (%s): 403 is not the typed scope_required envelope: %s", path, string(body))
			}
			if len(env.Required) != 1 || env.Required[0] != graphapi.GraphReadScope {
				t.Errorf("arm scope_denied (%s): 403 required=%v; want exactly [%s]", path, env.Required, graphapi.GraphReadScope)
			}
		case http.StatusUnauthorized:
			// Live-stack constraint: rejected at the shared-token compare
			// before the scope gate. Still a denial, still leak-free.
		default:
			graphAuthAssertExclusive(t, "scope_denied", path, http.StatusForbidden, resp, body)
		}
		scopeDenialCodes[resp.StatusCode]++
		graphAuthAssertDenialLeaksNothing(t, "scope_denied", path, body, needles)
	}

	switch {
	case scopeDenialCodes[http.StatusForbidden] == len(paths):
		t.Logf("arm scope_denied            exclusive403=%d/%d — per-user scope enforcement is LIVE; typed scope_required(%s) asserted on every path", len(paths), len(paths), graphapi.GraphReadScope)
	case scopeDenialCodes[http.StatusUnauthorized] == len(paths):
		t.Logf("arm scope_denied            LIVE-STACK-CONSTRAINED: the ungranted per-user token (scopes=[%s]) was denied 401 on %d/%d paths, not 403. Reason: the deployed container runs SMACKEREL_ENV=test + AUTH_ENABLED=false with no signing key, so bearerAuthMiddleware rejects every non-shared credential at the shared-token compare BEFORE auth.RequireScope(%s) is reached. Asserted here: the ungranted identity is DENIED, never served, and leaks nothing. The 403 scope_required arm is proven at the integration tier by tests/integration/graphapi/corpus_authorization_test.go (TestGlobalCorpusGrantMatrixOperatorGrantedUngrantedNoRowIsolation) over the REAL production router with AUTH_ENABLED=true; the constraint itself is recorded in tests/integration/graphapi/auth_test.go.", graphAuthUngrantedScope, scopeDenialCodes[http.StatusUnauthorized], len(paths), graphapi.GraphReadScope)
	default:
		t.Errorf("arm scope_denied: inconsistent denial codes across the 8-path manifest %v — an ungranted identity must receive ONE exclusive outcome on every graph path, never a mixture", scopeDenialCodes)
	}

	if !t.Failed() {
		t.Logf("AUTH EXCLUSIVITY OK: %d credential classes x %d graph paths — every failure was an exclusive typed denial (never served 200, never a 404 existence oracle, never a 503), and no denial body disclosed any of the %d seeded content needles or any count/total/items/cursor field", len(unauthArms)+1, len(paths), len(needles))
	}
}

// TestE2E_SharedLoginGrantsGlobalCorpusReadOnlyWithScope_T080_09_GRANT is
// T080-09-GRANT (SCN-080-001-09): "Regression: shared product-wide login grants
// global-corpus read only with knowledge-graph:read and denies ungranted
// leak-free".
//
// ADVERSARIAL RED/GREEN SHAPE. The DoD requires this test to fail FIRST if an
// ungranted identity can read graph content, counts, or existence hints, OR if
// a per-user/tenant row predicate is introduced. The three assertion groups are
// built so exactly those regressions break them:
//
//	Group A — the grant-holding identity (the shared product-wide login, which
//	  reaches the handlers only THROUGH the route group gated by
//	  auth.RequireScope(graphapi.GraphReadScope)) is served 200 and observes the
//	  seeded rows.
//
//	Group B — the SAME read is compared against DB ground truth. The fixtures
//	  are written out-of-band by a principal that is NOT the HTTP reader and
//	  carry NO owner/tenant column, so a per-user/tenant WHERE predicate on any
//	  read path makes the HTTP projection a STRICT SUBSET of the DB row set and
//	  fails here with an explicit row-predicate message. Two DISJOINT fixture
//	  batches are seeded and BOTH must be fully visible to the single
//	  grant-holder — the corpus is global, never a per-identity slice.
//
//	Group C — an authenticated identity WITHOUT the grant is denied on every
//	  path in the eight-family manifest, and the denial discloses no seeded
//	  id/label/title and no count/total/items/cursor field. The existence-oracle
//	  arm is the strongest: the denial for a topic that genuinely EXISTS and the
//	  denial for one that was never inserted must be byte-identical, so a denied
//	  caller cannot probe for existence.
//
// LIVE-STACK CONSTRAINT (declared honestly, never faked). As in T080-06-AUTH,
// the ungranted identity is a REAL PASETO v4.public token carrying the real
// non-graph scope "annotation:edit", but the deployed container runs
// SMACKEREL_ENV=test + AUTH_ENABLED=false with no signing key, so the denial
// arrives as a 401 from the shared-token compare instead of a 403 from
// auth.RequireScope. Group C asserts the denial, the never-served invariant and
// the leak-free/existence-oracle contract on the live stack regardless of which
// code the flavor produces, and the 403 scope_required branch below asserts the
// full grant contract automatically wherever per-user enforcement is live. The
// 403 leg itself is proven at the integration tier by
// tests/integration/graphapi/corpus_authorization_test.go.
func TestE2E_SharedLoginGrantsGlobalCorpusReadOnlyWithScope_T080_09_GRANT(t *testing.T) {
	cfg := loadE2EConfig(t) // skips when CORE_EXTERNAL_URL / token unset
	waitForHealth(t, cfg, 30*time.Second)

	dbURL := requireEnvForGraphAPI(t)
	conn, err := pgx.Connect(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("pgx.Connect: %v", err)
	}
	// LIFO ordering: registered FIRST so it runs LAST, after both fixture
	// cleanups below. Otherwise the DELETEs run against a closed pool.
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	stamp := time.Now().UTC().Format("20060102150405.000000")
	prefixA := "graph-grant-a-e2e-" + stamp
	prefixB := "graph-grant-b-e2e-" + stamp
	t.Cleanup(func() { graphAPICleanup(t, conn, prefixA) })
	t.Cleanup(func() { graphAPICleanup(t, conn, prefixB) })

	// TWO disjoint batches of GLOBAL rows. Nothing about them is owned by, or
	// scoped to, any HTTP identity — the graph tables have no owner column —
	// which is precisely the property Group B checks.
	topicsA := graphAPISeedTopics(t, conn, prefixA, 2)
	peopleA := graphAPISeedPeople(t, conn, prefixA, 1)
	artifactsA := graphAPISeedArtifacts(t, conn, prefixA, 1)
	topicsB := graphAPISeedTopics(t, conn, prefixB, 2)
	peopleB := graphAPISeedPeople(t, conn, prefixB, 1)

	needles := graphAuthSeededNeedles(
		append(append([]string{}, topicsA...), topicsB...),
		append(append([]string{}, peopleA...), peopleB...),
		artifactsA,
	)

	// ---- Group A: the grant-holding identity is SERVED the global corpus ----
	// The shared product-wide login reaches these handlers only through the
	// route group gated by auth.RequireScope(graphapi.GraphReadScope); a
	// regression that removes the grant path makes this 401/403 instead.
	observedTopics := graphAPICollectListIDs(t, cfg, "topics", "/api/topics?limit=200", 25)
	observedPeople := graphAPICollectListIDs(t, cfg, "people", "/api/people?limit=200", 25)
	seenTopics := graphAPIIDSet(observedTopics)
	seenPeople := graphAPIIDSet(observedPeople)
	t.Logf("grant-holder read scope=%s topics=%d people=%d", graphapi.GraphReadScope, len(observedTopics), len(observedPeople))

	// ---- Group B: GLOBAL corpus, not a per-identity slice ----
	// DB ground truth vs the HTTP projection, per batch. A per-user/tenant row
	// predicate cannot match these ownerless fixtures, so it would silently
	// drop them from the projection — caught here.
	for _, batch := range []struct {
		name   string
		prefix string
	}{{"batchA", prefixA}, {"batchB", prefixB}} {
		for _, fam := range []struct {
			table string
			seen  map[string]bool
			path  string
		}{
			{"topics", seenTopics, "/api/topics"},
			{"people", seenPeople, "/api/people"},
		} {
			truth := graphAuthDBIDs(t, conn, fam.table, batch.prefix)
			if len(truth) == 0 {
				t.Fatalf("%s/%s: DB ground truth is empty — the fixture seed did not land, so the global-corpus assertion would be vacuous", batch.name, fam.table)
			}
			missing := make([]string, 0, len(truth))
			for _, id := range truth {
				if !fam.seen[id] {
					missing = append(missing, id)
				}
			}
			if len(missing) > 0 {
				t.Errorf("GLOBAL-CORPUS VIOLATION: %s/%s — %d of %d rows present in the database are ABSENT from the grant-holder's %s projection (missing=%v). These fixtures were written out-of-band by a principal that is NOT the HTTP reader and carry no owner/tenant column, so a per-user or per-tenant row predicate on the read path is the only way they can disappear. The corpus is ONE global corpus; authorization is by grant, never by row partition.",
					batch.name, fam.table, len(missing), len(truth), fam.path, missing)
			}
		}
	}
	t.Logf("global-corpus OK  both disjoint batches (%s, %s) fully visible to the SINGLE grant-holding identity — no per-user/tenant row predicate", prefixA, prefixB)

	// ---- Group C: the ungranted identity is denied, leak-free ----
	privateHex, _ := auth.GenerateSigningKeypair()
	ungrantedToken := graphAuthMintToken(t, privateHex, prefixA+"-ungranted",
		[]string{graphAuthUngrantedScope}, time.Now().UTC(), time.Hour)
	if graphAuthUngrantedScope == graphapi.GraphReadScope {
		t.Fatalf("test invalid: the 'ungranted' identity carries %q, which IS the graph read grant — Group C would be vacuous", graphapi.GraphReadScope)
	}

	paths := disabledGraphPaths() // the canonical eight-family manifest
	denialCodes := map[int]int{}
	for _, path := range paths {
		resp, body := graphAuthGetWithBearer(t, cfg, path, ungrantedToken, "")
		if resp.StatusCode == http.StatusOK || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
			t.Errorf("UNGRANTED READ: %s returned %d to an identity whose scopes are [%s] — an identity WITHOUT %s must NEVER be served graph content. body=%s",
				path, resp.StatusCode, graphAuthUngrantedScope, graphapi.GraphReadScope, string(body))
		}
		switch resp.StatusCode {
		case http.StatusForbidden:
			var env struct {
				Error    string   `json:"error"`
				Required []string `json:"required"`
			}
			if err := json.Unmarshal(body, &env); err != nil || env.Error != "scope_required" {
				t.Errorf("ungranted (%s): 403 is not the typed scope_required envelope: %s", path, string(body))
			}
			if len(env.Required) != 1 || env.Required[0] != graphapi.GraphReadScope {
				t.Errorf("ungranted (%s): 403 required=%v; want exactly [%s]", path, env.Required, graphapi.GraphReadScope)
			}
		case http.StatusUnauthorized:
			// Live-stack constraint — still a denial, still leak-free.
		default:
			t.Errorf("ungranted (%s): status=%d; want a typed denial (403 scope_required, or 401 under the AUTH_ENABLED=false flavor) — never 404 route-missing, never 503 capability. body=%s", path, resp.StatusCode, string(body))
		}
		denialCodes[resp.StatusCode]++
		graphAuthAssertDenialLeaksNothing(t, "ungranted", path, body, needles)
	}

	// Existence-oracle arm: a topic that genuinely EXISTS and one that was
	// never inserted MUST produce byte-identical denials for the ungranted
	// identity. Any divergence — status, code, or body — is an existence hint.
	existingPath := "/api/topics/" + url.PathEscape(topicsA[0])
	absentPath := "/api/topics/" + url.PathEscape(prefixA+"-never-inserted-topic")
	existingResp, existingBody := graphAuthGetWithBearer(t, cfg, existingPath, ungrantedToken, "")
	absentResp, absentBody := graphAuthGetWithBearer(t, cfg, absentPath, ungrantedToken, "")
	if existingResp.StatusCode != absentResp.StatusCode {
		t.Errorf("EXISTENCE ORACLE: ungranted denial for an EXISTING topic returned %d but for a NEVER-INSERTED id returned %d — a denied identity must not be able to distinguish 'exists' from 'absent'", existingResp.StatusCode, absentResp.StatusCode)
	}
	if string(existingBody) != string(absentBody) {
		t.Errorf("EXISTENCE ORACLE: ungranted denial bodies differ between an EXISTING topic and a NEVER-INSERTED id — existing=%s absent=%s", string(existingBody), string(absentBody))
	}
	graphAuthAssertDenialLeaksNothing(t, "ungranted_existing_topic_detail", existingPath, existingBody, needles)

	switch {
	case denialCodes[http.StatusForbidden] == len(paths):
		t.Logf("ungranted denial          exclusive403=%d/%d — per-user scope enforcement is LIVE; typed scope_required(%s) asserted on every path", len(paths), len(paths), graphapi.GraphReadScope)
	case denialCodes[http.StatusUnauthorized] == len(paths):
		t.Logf("ungranted denial          LIVE-STACK-CONSTRAINED: the ungranted per-user token (scopes=[%s]) was denied 401 on %d/%d paths, not 403, because the deployed container runs SMACKEREL_ENV=test + AUTH_ENABLED=false with no signing key, so bearerAuthMiddleware rejects every non-shared credential BEFORE auth.RequireScope(%s). Asserted live regardless: never served, leak-free, and no existence oracle. The 403 scope_required leg is proven at the integration tier by tests/integration/graphapi/corpus_authorization_test.go.", graphAuthUngrantedScope, denialCodes[http.StatusUnauthorized], len(paths), graphapi.GraphReadScope)
	default:
		t.Errorf("ungranted denial: inconsistent codes across the 8-path manifest %v — an ungranted identity must receive ONE exclusive outcome on every graph path", denialCodes)
	}

	if !t.Failed() {
		t.Logf("GRANT MATRIX OK: grant-holder read the FULL global corpus (2 disjoint ownerless batches, DB-ground-truth-verified across topics+people); the ungranted identity was denied on all %d graph paths with zero content/count/existence disclosure across %d seeded needles", len(paths), len(needles))
	}
}

// ---------------------------------------------------------------------------
// T080-PRIVACY-NOSTORE (SCOPE-02 auth/privacy Core Outcome, clause 2:
// "no sensitive graph material is durably cached").
//
// WHY THIS TEST EXISTS AND WHY THE UNIT TESTS ARE NOT ENOUGH.
// internal/api/graphapi/privacy_test.go already proves that the two graphapi
// response WRITERS call SetPrivateNoStore. But a unit test only observes the
// writer's own httptest.ResponseRecorder — it can never observe the FULL
// middleware chain. `Cache-Control` is a last-writer-wins header: any
// middleware layered ABOVE or BELOW the graph route group could overwrite or
// drop the directive on its way to the socket and EVERY unit test would still
// be green while a proxy durably cached private knowledge-graph content. Only
// a real request against the running container proves the directive survives
// to the wire. That is exactly the tier the scopes.md DoD row claims.
//
// THE TWO CHOKE POINTS THIS TEST PINS.
//  1. graphapi-OWNED responses. Every graph family response — list, detail,
//     success and typed error alike — exits through exactly one of
//     graphapi.writeJSON (topics.go) or graphapi.WriteError (errors.go, which
//     WriteAPIError and GraphCapability.WriteDisabled both funnel through).
//     Both call SetPrivateNoStore, so the wire value is the EXACT pair
//     `private, no-store`. `private` forbids a shared cache (proxy, CDN) from
//     storing the response at all; `no-store` forbids ANY cache — shared or
//     private, memory or disk — from retaining it. That pair deliberately
//     UPGRADES (replaces) the weaker bare `no-store` the global
//     securityHeadersMiddleware set earlier in the chain: the graph API owns
//     the stricter contract for its own private content instead of inheriting
//     it from an unrelated global middleware a future edit could weaken
//     without any graph test noticing. This test is what makes that upgrade
//     observable.
//  2. The PRE-HANDLER 401. A missing or malformed bearer is rejected by
//     bearerAuthMiddleware, which sits ABOVE the graph route group and writes
//     through internal/api's own writeError — graphapi's writers never run, so
//     SetPrivateNoStore never executes on that path. Its durable-cache
//     protection therefore comes from the GLOBAL securityHeadersMiddleware
//     (router.go: `Cache-Control: no-store`). The security property that
//     matters — no durable retention of an auth-failure response — still
//     holds, and this test asserts it EXACTLY rather than pretending the
//     stricter graph pair is present where it structurally cannot be.
//
// ADVERSARIAL BY CONSTRUCTION. Every arm compares the FULL normalized
// directive SET for equality against the set its owning writer contracts. It
// is NOT a substring probe and NOT an "either directive present" check:
// deleting `private`, deleting `no-store`, weakening `no-store` to
// `no-cache`/`max-age=0`, or letting a later middleware append a second
// conflicting `Cache-Control` value all make the observed set differ from the
// expected set and fail the arm by name.

const (
	// graphPrivacyGraphOwnedCacheControl is the EXACT directive set that
	// graphapi's own writers stamp (graphapi.CacheControlPrivateNoStore).
	graphPrivacyGraphOwnedCacheControl = "private, no-store"

	// graphPrivacyPreHandlerCacheControl is the EXACT directive set the
	// GLOBAL securityHeadersMiddleware stamps on a response written above
	// the graph route group (the bearerAuthMiddleware 401).
	graphPrivacyPreHandlerCacheControl = "no-store"
)

// graphPrivacyDirectiveSet normalizes a raw Cache-Control value into a
// canonical, order-independent, case-insensitive directive set so the
// comparison pins the SEMANTICS rather than the byte spelling. Returning a
// sorted slice (not a map) keeps failure output deterministic.
func graphPrivacyDirectiveSet(raw string) []string {
	parts := strings.Split(strings.ToLower(raw), ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if d := strings.TrimSpace(p); d != "" {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

// graphPrivacyAssertCacheControl asserts one live arm: the status is exactly
// wantStatus (so an arm that silently changes tier cannot pass under another
// writer's header), the response carries exactly ONE Cache-Control header
// value (a second appended value is itself a cache-behavior hazard), and that
// value's directive set equals wantCacheControl's set EXACTLY.
func graphPrivacyAssertCacheControl(t *testing.T, arm, path string, wantStatus int, wantCacheControl string, resp *http.Response, body []byte) {
	t.Helper()

	if resp.StatusCode != wantStatus {
		t.Errorf("%s: GET %s returned %d, want exactly %d — this arm must exercise the writer that owns %q; body=%s",
			arm, path, resp.StatusCode, wantStatus, wantCacheControl, string(body))
		return
	}

	values := resp.Header.Values(graphapi.CacheControlHeader)
	if len(values) != 1 {
		t.Errorf("%s: GET %s returned %d %s header value(s) %v, want exactly 1 (%q) — a missing directive lets a proxy or browser DURABLY cache private graph material, and a duplicated/conflicting one leaves caching behavior undefined",
			arm, path, len(values), graphapi.CacheControlHeader, values, wantCacheControl)
		return
	}

	got := graphPrivacyDirectiveSet(values[0])
	want := graphPrivacyDirectiveSet(wantCacheControl)
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%s: GET %s (%d) sent %s: %q -> directive set %v, want EXACTLY %v. This is an exact-set assertion on purpose: dropping `private`, dropping `no-store`, or weakening either to no-cache/max-age fails here. The directive must survive the WHOLE middleware chain to the wire, not merely be set by the handler.",
			arm, path, resp.StatusCode, graphapi.CacheControlHeader, values[0], got, want)
		return
	}

	// Independent of the set comparison above, restate the single security
	// property in its own assertion so a future edit to the expected
	// constants cannot quietly delete durable-cache protection.
	hasNoStore := false
	for _, d := range got {
		if d == "no-store" {
			hasNoStore = true
		}
	}
	if !hasNoStore {
		t.Errorf("%s: GET %s (%d) sent %s: %q with NO `no-store` directive — an authenticated graph response MUST never be durably retained by any cache",
			arm, path, resp.StatusCode, graphapi.CacheControlHeader, values[0])
		return
	}

	t.Logf("%-34s %-34s %d  %s: %q", arm, path, resp.StatusCode, graphapi.CacheControlHeader, values[0])
}

// TestE2E_GraphResponsesArePrivateNoStore_T080_PRIVACY_NOSTORE drives the LIVE
// container over real HTTP — no interception, no mock — and proves the
// durable-cache privacy contract holds on the wire across all three response
// tiers a Graph consumer can reach: a content-bearing 200, an auth-failure
// 401, and a typed 400. See the block comment above for the choke-point map.
func TestE2E_GraphResponsesArePrivateNoStore_T080_PRIVACY_NOSTORE(t *testing.T) {
	cfg := loadE2EConfig(t) // skips when CORE_EXTERNAL_URL / token unset
	waitForHealth(t, cfg, 30*time.Second)

	dbURL := requireEnvForGraphAPI(t)
	conn, err := pgx.Connect(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("pgx.Connect: %v", err)
	}
	// LIFO convention (matches T080-03-READONLY): registering the close
	// FIRST makes it run LAST, after the fixture DELETEs registered below.
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	prefix := "graph-privacy-e2e-" + time.Now().UTC().Format("20060102150405.000000")
	t.Cleanup(func() { graphAPICleanup(t, conn, prefix) })

	// Seed so the 200 arm carries REAL private graph content rather than an
	// empty page — the header has to protect something for the arm to mean
	// anything.
	topicIDs := graphAPISeedTopics(t, conn, prefix, 1)
	seededID := topicIDs[0]

	// ---- arm 1/5: 200 detail — graphapi.writeJSON, content-bearing ----
	detailPath := "/api/topics/" + url.PathEscape(seededID)
	detailResp, detailBody := graphAPIGet(t, cfg, detailPath)
	graphPrivacyAssertCacheControl(t, "200_detail_graph_owned", detailPath,
		http.StatusOK, graphPrivacyGraphOwnedCacheControl, detailResp, detailBody)
	if !strings.Contains(string(detailBody), seededID) {
		t.Errorf("200_detail_graph_owned: body of %s does not contain the seeded id %q — the arm must carry real private graph content across the wire for the header assertion to be meaningful; body=%s",
			detailPath, seededID, string(detailBody))
	}

	// ---- arm 2/5: 200 list — graphapi.writeJSON ----
	listPath := "/api/topics?limit=200"
	listResp, listBody := graphAPIGet(t, cfg, listPath)
	graphPrivacyAssertCacheControl(t, "200_list_graph_owned", listPath,
		http.StatusOK, graphPrivacyGraphOwnedCacheControl, listResp, listBody)

	// ---- arm 3/5: 401 missing credential — pre-handler writer ----
	missingResp, missingBody := graphAuthGetWithBearer(t, cfg, "/api/topics", "", "")
	graphPrivacyAssertCacheControl(t, "401_missing_bearer_pre_handler", "/api/topics",
		http.StatusUnauthorized, graphPrivacyPreHandlerCacheControl, missingResp, missingBody)

	// ---- arm 4/5: 401 malformed credential — pre-handler writer ----
	malformedResp, malformedBody := graphAuthGetWithBearer(t, cfg, "/api/topics", "", "Bearer not-a-real-token")
	graphPrivacyAssertCacheControl(t, "401_malformed_bearer_pre_handler", "/api/topics",
		http.StatusUnauthorized, graphPrivacyPreHandlerCacheControl, malformedResp, malformedBody)

	// ---- arm 5/5: 400 typed error — graphapi.WriteError ----
	// /api/time with no from/to is a real typed missing_param 400 emitted by
	// graphapi.WriteError, so the typed-error tier is proven by the same
	// writer that serves the disabled 503 and the store_unavailable 503.
	badPath := "/api/time"
	badResp, badBody := graphAPIGet(t, cfg, badPath)
	graphPrivacyAssertCacheControl(t, "400_typed_error_graph_owned", badPath,
		http.StatusBadRequest, graphPrivacyGraphOwnedCacheControl, badResp, badBody)

	// Cross-arm invariant: NO reachable graph response tier may be durably
	// cacheable. Re-probing every path in the canonical eight-family manifest
	// with the shared credential guarantees the sweep is not limited to the
	// five hand-picked arms above — whatever tier each family answers with,
	// its response must carry `no-store`.
	for _, p := range disabledGraphPaths() {
		resp, _ := graphAPIGet(t, cfg, p)
		values := resp.Header.Values(graphapi.CacheControlHeader)
		if len(values) != 1 || !strings.Contains(strings.ToLower(values[0]), "no-store") {
			t.Errorf("manifest sweep: GET %s (%d) sent %s: %v — EVERY reachable graph response, whatever its tier, must carry exactly one no-store-bearing Cache-Control value",
				p, resp.StatusCode, graphapi.CacheControlHeader, values)
		}
	}

	if !t.Failed() {
		t.Logf("PRIVACY OK: graph-owned responses (200 detail, 200 list, 400 typed error) carry EXACTLY %q on the wire through the full middleware chain; the pre-handler 401 carries EXACTLY %q from the global securityHeadersMiddleware; and all %d paths in the canonical family manifest are no-store-bearing. No sensitive graph material is durably cacheable.",
			graphPrivacyGraphOwnedCacheControl, graphPrivacyPreHandlerCacheControl, len(disabledGraphPaths()))
	}
}
