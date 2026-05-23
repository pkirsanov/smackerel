//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	"github.com/smackerel/smackerel/internal/knowledge"
)

// Spec 041 Scope 7 — live-stack integration tests for the
// personal-context read API host
// (SCN-SM-041-025/026/027). These tests exercise the persisted consent
// store and the sensitivity-filtered knowledge query against a live
// pgxpool. The HTTP handler is mounted in-process via httptest so the
// full route surface (capability gate → consent atomic increment →
// sensitivity query → response shape + non-influence warning) is
// covered without depending on the running core process.
//
// Requires the disposable test stack:
//   ./smackerel.sh --env test up
// and DATABASE_URL pointing at it.

const personalContextEntityRef = "user:integration-pct-7"

func TestQFPersonalContextRead_AtomicReadCapEnforced_WhenSixthAttemptIsMade(t *testing.T) {
	// SCN-SM-041-027 — the 6th read attempt against a token issued
	// with a 5-read cap MUST return 429 + Retry-After and increment
	// reads_used to exactly 6 (each attempt counts regardless of
	// outcome).
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	entityRef := personalContextEntityRef + "-rate-limit"
	resetPersonalContextStateForEntity(t, ctx, pool, entityRef)
	persistPersonalContextCapability(t, ctx, pool, true)
	persistPersonalContextArtifacts(t, ctx, pool, entityRef, []personalContextFixture{
		{tier: qfdecisions.PersonalContextTierLow, summary: "low item 1"},
		{tier: qfdecisions.PersonalContextTierLow, summary: "low item 2"},
	})

	store := qfdecisions.NewPersonalContextConsentTokenStore(pool, time.Now)
	token, err := store.Issue(ctx, qfdecisions.PersonalContextConsentIssueRequest{
		EntityRef:          entityRef,
		MaxSensitivityTier: qfdecisions.PersonalContextTierHigh,
		RequesterID:        "qf-int-test",
		TTL:                10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("issue consent token: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Revoke(context.Background(), token.TokenID)
	})

	srv := buildPersonalContextTestServer(t, pool)
	defer srv.Close()

	for i := 0; i < qfdecisions.PersonalContextConsentMaxReads; i++ {
		res := callPersonalContext(t, srv.URL, token.TokenID, entityRef, qfdecisions.PersonalContextTierHigh)
		body := readAllAndClose(res)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("attempt %d/%d returned %d, want 200 (body=%s)", i+1, qfdecisions.PersonalContextConsentMaxReads, res.StatusCode, body)
		}
	}
	res := callPersonalContext(t, srv.URL, token.TokenID, entityRef, qfdecisions.PersonalContextTierHigh)
	body := readAllAndClose(res)
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("6th attempt status=%d, want 429 (body=%s)", res.StatusCode, body)
	}
	if res.Header.Get("Retry-After") == "" {
		t.Fatalf("6th attempt missing Retry-After header")
	}

	// Verify reads_used is persisted as exactly 6 (each attempt counts,
	// per SCN-SM-041-027 atomic-increment-on-every-attempt contract).
	var readsUsed int
	if err := pool.QueryRow(ctx,
		`SELECT reads_used FROM qf_personal_context_consent_tokens WHERE token_id = $1`,
		token.TokenID).Scan(&readsUsed); err != nil {
		t.Fatalf("read reads_used: %v", err)
	}
	if readsUsed != qfdecisions.PersonalContextConsentMaxReads+1 {
		t.Fatalf("reads_used=%d, want %d (every attempt counts under SCN-SM-041-027)",
			readsUsed, qfdecisions.PersonalContextConsentMaxReads+1)
	}
}

func TestQFPersonalContextRead_CeilingFiltersAndCountsRedactions(t *testing.T) {
	// SCN-SM-041-026 — items with sensitivity_tier > min(consent,user)
	// are NOT returned but counted in redaction_count when they are
	// below the consent ceiling. The route MUST also expose the
	// EXACT non-influence warning string on every successful read.
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	entityRef := personalContextEntityRef + "-ceiling"
	resetPersonalContextStateForEntity(t, ctx, pool, entityRef)
	persistPersonalContextCapability(t, ctx, pool, true)
	persistPersonalContextArtifacts(t, ctx, pool, entityRef, []personalContextFixture{
		{tier: qfdecisions.PersonalContextTierLow, summary: "low item"},
		{tier: qfdecisions.PersonalContextTierMedium, summary: "medium item — should be redacted"},
		{tier: qfdecisions.PersonalContextTierHigh, summary: "high item — should be redacted"},
	})

	store := qfdecisions.NewPersonalContextConsentTokenStore(pool, time.Now)
	token, err := store.Issue(ctx, qfdecisions.PersonalContextConsentIssueRequest{
		EntityRef:          entityRef,
		MaxSensitivityTier: qfdecisions.PersonalContextTierHigh,
		RequesterID:        "qf-int-test",
		TTL:                10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("issue consent token: %v", err)
	}
	t.Cleanup(func() { _ = store.Revoke(context.Background(), token.TokenID) })

	srv := buildPersonalContextTestServer(t, pool)
	defer srv.Close()

	res := callPersonalContext(t, srv.URL, token.TokenID, entityRef, qfdecisions.PersonalContextTierHigh)
	body := readAllAndClose(res)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200 (body=%s)", res.StatusCode, body)
	}
	var resp api.PersonalContextResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, body)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("got %d items, want 1 (only the low-tier item is below the conservative user ceiling)", len(resp.Items))
	}
	if resp.RedactionCount != 2 {
		t.Fatalf("redaction_count=%d, want 2 (medium+high items above user ceiling)", resp.RedactionCount)
	}
	if resp.NonInfluenceWarning != api.PersonalContextNonInfluenceWarning {
		t.Fatalf("non_influence_warning drift:\n got %q\nwant %q", resp.NonInfluenceWarning, api.PersonalContextNonInfluenceWarning)
	}
	if resp.ConsentCeiling != qfdecisions.PersonalContextTierHigh {
		t.Fatalf("consent_ceiling=%q, want high", resp.ConsentCeiling)
	}
}

func TestQFPersonalContextRead_CapabilityDisabledReturns503Live(t *testing.T) {
	// SCN-SM-041-026 — when persisted capability declares
	// personal_context_pull_supported=false, the route MUST respond 503
	// and MUST NOT increment the consent token. Tests via the live
	// pgxpool and the in-process handler.
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	entityRef := personalContextEntityRef + "-cap-off"
	resetPersonalContextStateForEntity(t, ctx, pool, entityRef)
	persistPersonalContextCapability(t, ctx, pool, false)

	store := qfdecisions.NewPersonalContextConsentTokenStore(pool, time.Now)
	token, err := store.Issue(ctx, qfdecisions.PersonalContextConsentIssueRequest{
		EntityRef:          entityRef,
		MaxSensitivityTier: qfdecisions.PersonalContextTierLow,
		RequesterID:        "qf-int-test",
		TTL:                10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("issue consent token: %v", err)
	}
	t.Cleanup(func() { _ = store.Revoke(context.Background(), token.TokenID) })

	srv := buildPersonalContextTestServer(t, pool)
	defer srv.Close()

	res := callPersonalContext(t, srv.URL, token.TokenID, entityRef, qfdecisions.PersonalContextTierLow)
	_ = readAllAndClose(res)
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503", res.StatusCode)
	}
	var readsUsed int
	if err := pool.QueryRow(ctx,
		`SELECT reads_used FROM qf_personal_context_consent_tokens WHERE token_id = $1`,
		token.TokenID).Scan(&readsUsed); err != nil {
		t.Fatalf("read reads_used: %v", err)
	}
	if readsUsed != 0 {
		t.Fatalf("reads_used=%d, want 0 (capability gate fires before the consent counter)", readsUsed)
	}
}

// personalContextFixture is one row planted into the artifacts table for
// the integration test.
type personalContextFixture struct {
	tier    string
	summary string
}

type personalContextFixedUserCeiling struct{}

func (personalContextFixedUserCeiling) UserPrivacyCeiling(_ context.Context, _ string) (string, error) {
	return qfdecisions.PersonalContextTierLow, nil
}

func buildPersonalContextTestServer(t *testing.T, pool *pgxpool.Pool) *httptest.Server {
	t.Helper()
	state := connector.NewStateStore(pool)
	consent := qfdecisions.NewPersonalContextConsentTokenStore(pool, time.Now)
	items := knowledge.NewPersonalContextSensitivityQuerier(pool)
	handlers := api.NewPersonalContextHandlers(state, consent, items, personalContextFixedUserCeiling{}, time.Now)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/private/qf/v1/personal-context", handlers.Read)
	return httptest.NewServer(mux)
}

func callPersonalContext(t *testing.T, baseURL, token, entityRef, tier string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("GET", baseURL+"/api/private/qf/v1/personal-context", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	q := req.URL.Query()
	q.Set("entity_ref", entityRef)
	q.Set("max_sensitivity", tier)
	q.Set("consent_token", token)
	q.Set("requester_id", "qf-int-test")
	req.URL.RawQuery = q.Encode()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("call %s: %v", req.URL.String(), err)
	}
	return res
}

func persistPersonalContextCapability(t *testing.T, ctx context.Context, pool *pgxpool.Pool, supported bool) {
	t.Helper()
	state := connector.NewStateStore(pool)
	capability := validQFIntegrationCapability()
	capability.PersonalContextPullSupported = supported
	buf, err := json.Marshal(capability)
	if err != nil {
		t.Fatalf("marshal capability: %v", err)
	}
	if err := state.SaveCapability(ctx, qfdecisions.DefaultConnectorID, string(buf), time.Now().UTC(), qfdecisions.CapabilityStatusCompatible); err != nil {
		t.Fatalf("save capability: %v", err)
	}
}

func persistPersonalContextArtifacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, entityRef string, fixtures []personalContextFixture) {
	t.Helper()
	// Delete any pre-existing rows for this entity_ref so the test is
	// deterministic against a possibly polluted disposable stack.
	if _, err := pool.Exec(ctx,
		`DELETE FROM artifacts WHERE metadata->>'entity_ref' = $1`,
		entityRef); err != nil {
		t.Fatalf("clean prior artifacts: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM artifacts WHERE metadata->>'entity_ref' = $1`,
			entityRef)
	})
	for i, fx := range fixtures {
		md := map[string]any{
			"entity_ref":       entityRef,
			"sensitivity_tier": fx.tier,
		}
		mdJSON, err := json.Marshal(md)
		if err != nil {
			t.Fatalf("marshal metadata: %v", err)
		}
		// Use a deterministic id so re-runs against the same disposable
		// stack do not bleed across tests.
		id := "pct-int-" + strings.ReplaceAll(entityRef, ":", "-") + "-" + fx.tier + "-" + strconv.Itoa(i)
		if _, err := pool.Exec(ctx, `
			INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, summary, source_url, metadata, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, NOW())
		`,
			id, "note", "personal context fixture "+id, id+"-hash",
			"qf-personal-context-integration", fx.summary,
			"https://example.invalid/integration/"+id, string(mdJSON)); err != nil {
			t.Fatalf("insert artifact %s: %v", id, err)
		}
	}
}

func resetPersonalContextStateForEntity(t *testing.T, ctx context.Context, pool *pgxpool.Pool, entityRef string) {
	t.Helper()
	if _, err := pool.Exec(ctx,
		`DELETE FROM qf_personal_context_consent_tokens WHERE entity_ref = $1`,
		entityRef); err != nil {
		t.Fatalf("clean prior consent tokens: %v", err)
	}
}

func readAllAndClose(res *http.Response) string {
	defer res.Body.Close()
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 1024)
	for {
		n, err := res.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return string(buf)
}
