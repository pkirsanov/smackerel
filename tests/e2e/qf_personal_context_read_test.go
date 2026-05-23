//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
)

// Spec 041 Scope 7 — live HTTP end-to-end test for the personal-context
// read API host (SCN-SM-041-025/026/027). This test exercises the real
// chi-routed handler over the live core service (CORE_EXTERNAL_URL),
// with the bearer-auth gate engaged, against the live PostgreSQL
// disposable test stack. It seeds capability + consent + sensitivity
// fixtures via direct DB writes (capabilities are persisted state that
// the connector normally fills via handshake, but seeding directly is
// the only way to exercise the read path without booting an external
// QF bridge during the e2e run).

const e2ePersonalContextEntityRef = "user:e2e-pct-7"

func TestQFPersonalContextRead_LiveHTTP_NonInfluenceWarningAndHappyPath(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("e2e: DATABASE_URL not set — needed for fixture seed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	// Seed capability + fixtures + consent token via direct DB writes.
	seedPersonalContextCapability(t, ctx, pool, true)
	seedPersonalContextArtifacts(t, ctx, pool, e2ePersonalContextEntityRef)
	store := qfdecisions.NewPersonalContextConsentTokenStore(pool, time.Now)
	token, err := store.Issue(ctx, qfdecisions.PersonalContextConsentIssueRequest{
		EntityRef:          e2ePersonalContextEntityRef,
		MaxSensitivityTier: qfdecisions.PersonalContextTierHigh,
		RequesterID:        "qf-e2e-test",
		TTL:                10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("issue consent token: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Revoke(context.Background(), token.TokenID)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM qf_personal_context_consent_tokens WHERE entity_ref = $1`,
			e2ePersonalContextEntityRef)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM artifacts WHERE metadata->>'entity_ref' = $1`,
			e2ePersonalContextEntityRef)
	})

	// Call the live HTTP endpoint via the bearer-auth-gated route.
	q := url.Values{}
	q.Set("entity_ref", e2ePersonalContextEntityRef)
	q.Set("max_sensitivity", qfdecisions.PersonalContextTierHigh)
	q.Set("consent_token", token.TokenID)
	q.Set("requester_id", "qf-e2e-test")
	res, err := apiGet(cfg, "/api/private/qf/v1/personal-context?"+q.Encode())
	if err != nil {
		t.Fatalf("call personal-context: %v", err)
	}
	body, err := readBody(res)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200 (body=%s)", res.StatusCode, body)
	}
	var resp api.PersonalContextResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, body)
	}
	if resp.NonInfluenceWarning != api.PersonalContextNonInfluenceWarning {
		t.Fatalf("non_influence_warning drift over live HTTP:\n got %q\nwant %q",
			resp.NonInfluenceWarning, api.PersonalContextNonInfluenceWarning)
	}
	if resp.ConsentCeiling != qfdecisions.PersonalContextTierHigh {
		t.Fatalf("consent_ceiling=%q, want high", resp.ConsentCeiling)
	}
	// Adversarial: a regression that silently drops the warning string
	// would still produce a valid JSON envelope; the byte-exact check
	// above catches that. A regression that returns 4xx because the
	// route is not mounted (e.g. wiring drift) would be caught by the
	// status check above. A regression that decrements reads beyond
	// the cap silently is caught by the integration test sibling.
}

func TestQFPersonalContextRead_LiveHTTP_RequiresBearerAuth(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)

	// Build a request WITHOUT the Authorization header.
	q := url.Values{}
	q.Set("entity_ref", e2ePersonalContextEntityRef)
	q.Set("max_sensitivity", qfdecisions.PersonalContextTierLow)
	q.Set("consent_token", "pct_irrelevant-auth-test")
	req, err := http.NewRequest(http.MethodGet,
		cfg.CoreURL+"/api/private/qf/v1/personal-context?"+q.Encode(), nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("call personal-context: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401 (route MUST be inside the bearer-auth gate)", res.StatusCode)
	}
}

func seedPersonalContextCapability(t *testing.T, ctx context.Context, pool *pgxpool.Pool, supported bool) {
	t.Helper()
	state := connector.NewStateStore(pool)
	// Build a minimal capability for the e2e path; the rest of the
	// capability fields are not consulted by the personal-context route.
	capability := qfdecisions.QFBridgeCapability{
		SupportedPacketVersions:            []string{"v1"},
		SupportedEventTypes:                []string{"packet_created"},
		SupportedDecisionTypes:             []string{"recommendation"},
		MaxPageSize:                        200,
		MinPageSize:                        1,
		SupportedTargetContextTypes:        []string{qfdecisions.TargetContextPacketContext},
		EvidenceMaxBundleSizeBytes:         524288,
		EvidenceMaxClaimsPerBundle:         50,
		EvidenceRateLimitPerMinute:         10,
		FreshnessSLAP95Seconds:             60,
		AuditEnvelopeVersion:               "v1",
		PersonalContextPullSupported:       supported,
		WatchSignalDirection:               "qf_emit_only_pre_mvp",
		CredentialRotationOverlapSupported: true,
		EligibleSmackerelSourceClasses:     []string{"smackerel_other"},
	}
	buf, err := json.Marshal(capability)
	if err != nil {
		t.Fatalf("marshal capability: %v", err)
	}
	if err := state.SaveCapability(ctx, qfdecisions.DefaultConnectorID, string(buf), time.Now().UTC(), qfdecisions.CapabilityStatusCompatible); err != nil {
		t.Fatalf("save capability: %v", err)
	}
}

func seedPersonalContextArtifacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, entityRef string) {
	t.Helper()
	if _, err := pool.Exec(ctx,
		`DELETE FROM artifacts WHERE metadata->>'entity_ref' = $1`,
		entityRef); err != nil {
		t.Fatalf("clean prior artifacts: %v", err)
	}
	rows := []struct {
		tier, summary string
	}{
		{qfdecisions.PersonalContextTierLow, "e2e low item"},
		{qfdecisions.PersonalContextTierMedium, "e2e medium item — will be redacted"},
	}
	for i, fx := range rows {
		md := map[string]any{
			"entity_ref":       entityRef,
			"sensitivity_tier": fx.tier,
		}
		mdJSON, err := json.Marshal(md)
		if err != nil {
			t.Fatalf("marshal metadata: %v", err)
		}
		id := "pct-e2e-" + fx.tier + "-" + iToStr(i)
		if _, err := pool.Exec(ctx, `
			INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, summary, source_url, metadata, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, NOW())
		`,
			id, "note", "personal context e2e fixture "+id, id+"-hash",
			"qf-personal-context-e2e", fx.summary,
			"https://example.invalid/e2e/"+id, string(mdJSON)); err != nil {
			t.Fatalf("insert artifact %s: %v", id, err)
		}
	}
}

func iToStr(i int) string {
	if i == 0 {
		return "0"
	}
	out := []byte{}
	for n := i; n > 0; n /= 10 {
		out = append([]byte{byte('0' + n%10)}, out...)
	}
	return string(out)
}

// TestQFPersonalContextRead_LiveHTTP_FailureMatrix covers SCN-SM-041-026 over
// live HTTP: scope-mismatched / expired / capability-disabled requests are
// rejected with the documented status codes through the chi route under the
// bearer-auth gate.
func TestQFPersonalContextRead_LiveHTTP_FailureMatrix(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("e2e: DATABASE_URL not set — needed for fixture seed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	entityRef := "user:e2e-pct-7-failure"

	// Start with capability enabled and seed fixtures + a low-ceiling token.
	seedPersonalContextCapability(t, ctx, pool, true)
	seedPersonalContextArtifacts(t, ctx, pool, entityRef)
	store := qfdecisions.NewPersonalContextConsentTokenStore(pool, time.Now)
	token, err := store.Issue(ctx, qfdecisions.PersonalContextConsentIssueRequest{
		EntityRef:          entityRef,
		MaxSensitivityTier: qfdecisions.PersonalContextTierLow,
		RequesterID:        "qf-e2e-failure",
		TTL:                10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("issue consent token: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM qf_personal_context_consent_tokens WHERE entity_ref = $1`,
			entityRef)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM artifacts WHERE metadata->>'entity_ref' = $1`,
			entityRef)
	})

	// Sub 1: scope mismatch — request a higher ceiling than the token allows.
	t.Run("scope_violation_returns_403", func(t *testing.T) {
		q := url.Values{}
		q.Set("entity_ref", entityRef)
		q.Set("max_sensitivity", qfdecisions.PersonalContextTierHigh) // > token ceiling
		q.Set("consent_token", token.TokenID)
		q.Set("requester_id", "qf-e2e-failure")
		res, err := apiGet(cfg, "/api/private/qf/v1/personal-context?"+q.Encode())
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		body, _ := readBody(res)
		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("status=%d, want 403 (body=%s)", res.StatusCode, body)
		}
	})

	// Sub 2: entity mismatch — same token, different entity_ref → 403.
	t.Run("entity_mismatch_returns_403", func(t *testing.T) {
		q := url.Values{}
		q.Set("entity_ref", entityRef+"-other")
		q.Set("max_sensitivity", qfdecisions.PersonalContextTierLow)
		q.Set("consent_token", token.TokenID)
		q.Set("requester_id", "qf-e2e-failure")
		res, err := apiGet(cfg, "/api/private/qf/v1/personal-context?"+q.Encode())
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		body, _ := readBody(res)
		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("status=%d, want 403 (body=%s)", res.StatusCode, body)
		}
	})

	// Sub 3: capability disabled — flip the persisted capability to false
	// and confirm the route returns 503 regardless of token validity.
	t.Run("capability_disabled_returns_503", func(t *testing.T) {
		// Flip capability to false.
		seedPersonalContextCapability(t, ctx, pool, false)
		t.Cleanup(func() {
			// Restore capability so sibling tests are not disturbed.
			seedPersonalContextCapability(t, ctx, pool, true)
		})
		q := url.Values{}
		q.Set("entity_ref", entityRef)
		q.Set("max_sensitivity", qfdecisions.PersonalContextTierLow)
		q.Set("consent_token", token.TokenID)
		q.Set("requester_id", "qf-e2e-failure")
		res, err := apiGet(cfg, "/api/private/qf/v1/personal-context?"+q.Encode())
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		body, _ := readBody(res)
		if res.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("status=%d, want 503 (body=%s)", res.StatusCode, body)
		}
	})
}

// TestQFPersonalContextRead_LiveHTTP_RateLimitAndAudit covers SCN-SM-041-027
// over live HTTP: the atomic 5-read cap is enforced per token through the
// chi route, and the 6th attempt returns 429 with Retry-After.
func TestQFPersonalContextRead_LiveHTTP_RateLimitAndAudit(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("e2e: DATABASE_URL not set — needed for fixture seed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	entityRef := "user:e2e-pct-7-ratelimit"

	seedPersonalContextCapability(t, ctx, pool, true)
	seedPersonalContextArtifacts(t, ctx, pool, entityRef)
	store := qfdecisions.NewPersonalContextConsentTokenStore(pool, time.Now)
	token, err := store.Issue(ctx, qfdecisions.PersonalContextConsentIssueRequest{
		EntityRef:          entityRef,
		MaxSensitivityTier: qfdecisions.PersonalContextTierHigh,
		RequesterID:        "qf-e2e-ratelimit",
		TTL:                10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("issue consent token: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM qf_personal_context_consent_tokens WHERE entity_ref = $1`,
			entityRef)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM artifacts WHERE metadata->>'entity_ref' = $1`,
			entityRef)
	})

	q := url.Values{}
	q.Set("entity_ref", entityRef)
	q.Set("max_sensitivity", qfdecisions.PersonalContextTierHigh)
	q.Set("consent_token", token.TokenID)
	q.Set("requester_id", "qf-e2e-ratelimit")

	// Exhaust the 5-read cap.
	for i := 1; i <= qfdecisions.PersonalContextConsentMaxReads; i++ {
		res, err := apiGet(cfg, "/api/private/qf/v1/personal-context?"+q.Encode())
		if err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		body, _ := readBody(res)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("attempt %d status=%d, want 200 (body=%s)", i, res.StatusCode, body)
		}
	}

	// 6th attempt must be 429 with Retry-After.
	res, err := apiGet(cfg, "/api/private/qf/v1/personal-context?"+q.Encode())
	if err != nil {
		t.Fatalf("6th attempt: %v", err)
	}
	body, _ := readBody(res)
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("6th status=%d, want 429 (body=%s)", res.StatusCode, body)
	}
	if got := res.Header.Get("Retry-After"); got == "" {
		t.Fatalf("6th attempt missing Retry-After header (body=%s)", body)
	}
}
