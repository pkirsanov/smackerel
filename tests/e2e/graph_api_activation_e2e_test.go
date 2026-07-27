//go:build e2e

// BUG-080-001 — fail-soft Knowledge Graph activation, e2e-api tier.
//
// This file ties the SCOPE-01 fail-soft activation contract to the LIVE
// disposable stack over real HTTP (CORE_EXTERNAL_URL + SMACKEREL_AUTH_TOKEN,
// exported by `./smackerel.sh test e2e`). There is NO request interception,
// NO mock, NO stub — every assertion drives the real running smackerel-core
// container end-to-end.
//
// Three SCOPE-01 rows live here:
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
//     The default `./smackerel.sh test e2e` harness boots exactly ONE core in
//     the ENABLED activation state: config/generated/test.env sets a non-empty
//     KNOWLEDGE_GRAPH_API_CURSOR_SECRET and docker-compose.yml sources it via
//     env_file with no per-run override, so a true-container e2e proof of the
//     fail-soft DISABLED path (typed 503 capability_disabled, never a silent
//     404 / opaque 500 / panic) needs a NEW disabled-mode stack flavor. That
//     harness change is tracked as a bubbles.devops finding in the BUG-080-001
//     report.md ("HARNESS LIMITATION"). Both tests below are written to run
//     UNCHANGED the moment that harness exports the disabled core's base URL as
//     SMACKEREL_E2E_GRAPH_DISABLED_URL; until then they t.Skip with a precise
//     reason. The DISABLED fail-soft behavior is ALREADY live-proven at the
//     integration tier (tests/integration/graphapi/activation_test.go —
//     TestGraphActivationDisabledSecretServesTyped503AndKeepsServing, and
//     route_manifest_test.go — disabled_router_mounts_all_eight_present) and at
//     the unit tier (internal/api/graphapi/activation_test.go —
//     TestAdversarial_EmptySecretMustNotRevertToSilentAbsenceOr500).

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
)

// cursorSecretEnvName is the *name* of the env var whose VALUE is the
// operator cursor HMAC secret. The e2e go-test runner receives it via
// `--env-file config/generated/test.env` (line 195), so this test can read
// the real deployed secret and prove it never leaks into API output.
const cursorSecretEnvName = "KNOWLEDGE_GRAPH_API_CURSOR_SECRET"

// disabledGraphStackURL returns the base URL of a smackerel-core booted in
// the fail-soft DISABLED graph activation state, or skips the calling test
// when the default (ENABLED-only) harness has not provided one. See the file
// header + the BUG-080-001 report.md "HARNESS LIMITATION" finding.
func disabledGraphStackURL(t *testing.T) string {
	t.Helper()
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("SMACKEREL_E2E_GRAPH_DISABLED_URL")), "/")
	if base == "" {
		t.Skip("e2e: SMACKEREL_E2E_GRAPH_DISABLED_URL not set — the default `./smackerel.sh test e2e` harness boots a single ENABLED core (config/generated/test.env sets a non-empty " + cursorSecretEnvName + "; docker-compose.yml has no per-run override). A DISABLED-mode core is a bubbles.devops harness finding (BUG-080-001 report.md). The DISABLED fail-soft behavior is already proven at the integration tier (T080-01-PROC) and unit tier (T080-01-UNIT).")
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
