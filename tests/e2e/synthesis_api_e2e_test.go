//go:build e2e

// BUG-004-004 SCOPE-04 — T004-05-API / T004-07-08-API / T004-09-AUTH.
//
// These exercise the HTTP surface against the live stack. The integration tests
// prove the read MODEL; only these prove the handler is actually wired to it and
// that the auth gate is real. A model can be perfect while the route returns a
// hardcoded shape, and nothing below the transport would notice.

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/acceptance"
	"github.com/smackerel/smackerel/internal/api"
	smackdb "github.com/smackerel/smackerel/internal/db"
	"github.com/smackerel/smackerel/internal/intelligence"
	smacknats "github.com/smackerel/smackerel/internal/nats"
)

type synthesisLatestBody struct {
	State  string `json:"state"`
	Output *struct {
		OutputID       string `json:"outputId"`
		Kind           string `json:"kind"`
		InsightCount   int    `json:"insightCount"`
		CitationCount  int    `json:"citationCount"`
		LifecycleState string `json:"lifecycleState"`
		CreatedAt      string `json:"createdAt"`
	} `json:"output"`
}

func synthesisGet(t *testing.T, cfg e2eConfig, path string, authorized bool) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, cfg.CoreURL+path, nil)
	if err != nil {
		t.Fatalf("build request %s: %v", path, err)
	}
	if authorized {
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return resp.StatusCode, body
}

// SCN-004-004-05. Never-run must be an explicit state, not an empty success.
// The pre-fix failure mode was a green health field standing in for a system
// that had produced nothing, so an absent or blank state here is the exact
// regression being guarded.
func TestSynthesisAPI_LatestReportsAnExplicitState(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	status, body := synthesisGet(t, cfg, "/api/synthesis/latest", true)
	if status != http.StatusOK {
		t.Fatalf("GET latest returned %d, want 200; body=%s", status, string(body))
	}

	var parsed synthesisLatestBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode latest: %v; body=%s", err, string(body))
	}
	if parsed.State == "" {
		t.Fatalf("latest returned no state; a caller must never infer never-run from an absent field. body=%s", string(body))
	}

	switch parsed.State {
	case "never-run":
		if parsed.Output != nil {
			t.Fatalf("never-run carried an output; that is a contradiction. body=%s", string(body))
		}
	case "full", "quiet", "partial":
		if parsed.Output == nil {
			t.Fatalf("state %q carried no output. body=%s", parsed.State, string(body))
		}
		if parsed.Output.OutputID == "" {
			t.Fatalf("state %q carried an output with no id. body=%s", parsed.State, string(body))
		}
		if parsed.State == "quiet" && parsed.Output.InsightCount != 0 {
			t.Fatalf("quiet output reported %d insights. body=%s", parsed.Output.InsightCount, string(body))
		}
	default:
		t.Fatalf("latest returned unknown state %q; the vocabulary is closed. body=%s", parsed.State, string(body))
	}
}

// SCN-004-004-07/08. The history listing must serialise as a list, and every
// entry must carry a known kind. A null here would read as a missing field to a
// client rather than as "no runs".
func TestSynthesisAPI_RunsListIsWellFormed(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	status, body := synthesisGet(t, cfg, "/api/synthesis/runs?limit=5", true)
	if status != http.StatusOK {
		t.Fatalf("GET runs returned %d, want 200; body=%s", status, string(body))
	}

	var parsed struct {
		Runs []struct {
			OutputID string `json:"outputId"`
			Kind     string `json:"kind"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode runs: %v; body=%s", err, string(body))
	}
	if parsed.Runs == nil {
		t.Fatalf("runs serialised as null rather than []; that reads as a missing field. body=%s", string(body))
	}
	if len(parsed.Runs) > 5 {
		t.Fatalf("limit=5 returned %d runs; the bound must be enforced server-side", len(parsed.Runs))
	}
	for i, run := range parsed.Runs {
		if run.OutputID == "" {
			t.Fatalf("run %d has no output id. body=%s", i, string(body))
		}
		switch run.Kind {
		case "full", "quiet", "partial":
		default:
			t.Fatalf("run %d has unknown kind %q; the vocabulary is closed. body=%s", i, run.Kind, string(body))
		}
	}
}

// An invalid limit must be refused rather than silently coerced. Coercion would
// hide a client bug and make the bound untestable from outside.
func TestSynthesisAPI_RejectsInvalidLimit(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	status, body := synthesisGet(t, cfg, "/api/synthesis/runs?limit=abc", true)
	if status != http.StatusBadRequest {
		t.Fatalf("limit=abc returned %d, want 400; body=%s", status, string(body))
	}
}

// SCN-004-004-09. The routes sit behind the bearer gate. This is the assertion
// that would fail if they were mounted outside it -- which no amount of
// integration testing against the read model could detect.
func TestSynthesisAPI_DeniesUnauthenticatedCallers(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	for _, path := range []string{
		"/api/synthesis/latest",
		"/api/synthesis/runs",
		"/api/synthesis/runs/does-not-exist",
	} {
		status, body := synthesisGet(t, cfg, path, false)
		if status != http.StatusUnauthorized && status != http.StatusForbidden {
			t.Fatalf("unauthenticated GET %s returned %d, want 401 or 403; body=%s", path, status, string(body))
		}
		// The denial must not describe what exists behind it.
		for _, leak := range []string{"outputId", "insightCount", "throughLine"} {
			if len(body) > 0 && containsE2E(string(body), leak) {
				t.Fatalf("unauthenticated denial for %s leaked %q: %s", path, leak, string(body))
			}
		}
	}
}

// An unknown output id must not be answered with a confident success.
func TestSynthesisAPI_UnknownRunIsNotFound(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	status, body := synthesisGet(t, cfg, "/api/synthesis/runs/01JZZZZZZZZZZZZZZZZZZZZZZZ", true)
	if status != http.StatusNotFound {
		t.Fatalf("unknown output id returned %d, want 404; body=%s", status, string(body))
	}
	// A bare 404 proves nothing: an unmounted route returns one too, and this
	// test passed against exactly that before the prefix was corrected. The
	// structured code is what distinguishes the handler answering from the
	// router shrugging.
	if !containsE2E(string(body), "synthesis_output_not_found") {
		t.Fatalf("404 body did not carry the handler's error code; the route may not be mounted at all: %s", string(body))
	}
}

func containsE2E(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// SCN-004-004-C20 / T004-C20-STRICT. Every required cadence is read from its
// own causal PostgreSQL history. A healthy daily cadence cannot mask a weekly
// never-run, running, stale, partial, failed, or read-degraded state. The HTTP
// checks use a real server and the lane's real Postgres and NATS dependencies;
// no internal service or repository is mocked.
func TestSynthesisAPI_StrictHealthRefusesEveryNonGreenRequiredCadence(t *testing.T) {
	pool := synthesisE2EPool(t, "T004-C20-STRICT")
	resetSynthesisE2EState(t, pool, "T004-C20-STRICT")
	t.Cleanup(func() {
		resetSynthesisE2EState(t, pool, "T004-C20-STRICT cleanup")
	})

	natsURL := strings.TrimSpace(os.Getenv("NATS_URL"))
	if natsURL == "" {
		t.Fatal("T004-C20-STRICT requires NATS_URL from ./smackerel.sh test e2e")
	}
	natsClient, err := smacknats.Connect(context.Background(), natsURL, "")
	if err != nil {
		t.Fatalf("connect T004-C20-STRICT NATS: %v", err)
	}
	t.Cleanup(natsClient.Close)

	authToken := strings.TrimSpace(os.Getenv("SMACKEREL_AUTH_TOKEN"))
	if authToken == "" {
		t.Fatal("T004-C20-STRICT requires SMACKEREL_AUTH_TOKEN from ./smackerel.sh test e2e")
	}
	dailyFreshness := requiredSynthesisE2EDuration(t, "SYNTHESIS_DAILY_FRESHNESS_SECONDS")
	weeklyFreshness := requiredSynthesisE2EDuration(t, "SYNTHESIS_WEEKLY_FRESHNESS_SECONDS")
	freshnessPolicy, err := intelligence.NewSynthesisFreshnessPolicy(dailyFreshness, weeklyFreshness)
	if err != nil {
		t.Fatalf("construct T004-C20-STRICT freshness policy: %v", err)
	}
	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct T004-C20-STRICT persistence: %v", err)
	}

	type nonGreenCase struct {
		name       string
		wantStatus string
		seedWeekly func(t *testing.T, principal string, now time.Time)
	}
	testCases := []nonGreenCase{
		{
			name:       "never-run",
			wantStatus: "down",
			seedWeekly: func(*testing.T, string, time.Time) {},
		},
		{
			name:       "running",
			wantStatus: "down",
			seedWeekly: func(t *testing.T, principal string, now time.Time) {
				startSynthesisC20Attempt(t, persistence, synthesisC20Key(principal, intelligence.CadenceWeekly, now), now)
			},
		},
		{
			name:       "stale",
			wantStatus: "stale",
			seedWeekly: func(t *testing.T, principal string, now time.Time) {
				seedSynthesisC20Output(t, persistence, principal, intelligence.CadenceWeekly,
					intelligence.OutputKindQuiet, now.Add(-weeklyFreshness-time.Hour))
			},
		},
		{
			name:       "partial",
			wantStatus: "down",
			seedWeekly: func(t *testing.T, principal string, now time.Time) {
				seedSynthesisC20Output(t, persistence, principal, intelligence.CadenceWeekly,
					intelligence.OutputKindPartial, now.Add(-time.Minute))
			},
		},
		{
			name:       "failed",
			wantStatus: "down",
			seedWeekly: func(t *testing.T, principal string, now time.Time) {
				attempt := startSynthesisC20Attempt(t, persistence, synthesisC20Key(principal, intelligence.CadenceWeekly, now), now)
				if err := persistence.FinishAttemptFailure(context.Background(), attempt,
					intelligence.EventFailed, string(intelligence.FailureTransaction),
					intelligence.FailureTerminal, now.Add(time.Second)); err != nil {
					t.Fatalf("finish T004-C20-STRICT failed weekly attempt: %v", err)
				}
			},
		},
		{
			name:       "read-degraded",
			wantStatus: "down",
			seedWeekly: func(t *testing.T, principal string, now time.Time) {
				attempt := startSynthesisC20Attempt(t, persistence, synthesisC20Key(principal, intelligence.CadenceWeekly, now), now)
				markSynthesisC20ReadDegraded(t, pool, persistence, attempt, now.Add(time.Second))
			},
		},
	}

	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			now := time.Now().UTC()
			principal := fmt.Sprintf("t004-c20-%d-%d", now.UnixNano(), index)
			seedSynthesisC20Output(t, persistence, principal, intelligence.CadenceDaily,
				intelligence.OutputKindQuiet, now.Add(-time.Minute))
			testCase.seedWeekly(t, principal, now)

			server := synthesisC20HealthServer(t, pool, natsClient, freshnessPolicy, principal, authToken)
			cfg := e2eConfig{CoreURL: server.URL, AuthToken: authToken}
			assertSynthesisC20StrictHealth(t, cfg, testCase.wantStatus, http.StatusServiceUnavailable, "degraded")

			livenessStatus, livenessBody := synthesisGet(t, cfg, "/api/health", false)
			if livenessStatus != http.StatusOK {
				t.Fatalf("default liveness with weekly %s returned %d, want 200; body=%s", testCase.name, livenessStatus, string(livenessBody))
			}
		})
	}

	t.Run("daily and weekly healthy", func(t *testing.T) {
		now := time.Now().UTC()
		principal := fmt.Sprintf("t004-c20-green-%d", now.UnixNano())
		seedSynthesisC20Output(t, persistence, principal, intelligence.CadenceDaily,
			intelligence.OutputKindQuiet, now.Add(-time.Minute))
		seedSynthesisC20Output(t, persistence, principal, intelligence.CadenceWeekly,
			intelligence.OutputKindQuiet, now.Add(-time.Minute))

		server := synthesisC20HealthServer(t, pool, natsClient, freshnessPolicy, principal, authToken)
		cfg := e2eConfig{CoreURL: server.URL, AuthToken: authToken}
		assertSynthesisC20StrictHealth(t, cfg, "up", http.StatusOK, "healthy")
	})
}

func requiredSynthesisE2EDuration(t *testing.T, key string) time.Duration {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(key))
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seconds <= 0 {
		t.Fatalf("T004-C20-STRICT requires positive integer %s, got %q", key, raw)
	}
	duration := time.Duration(seconds) * time.Second
	if duration/time.Second != time.Duration(seconds) {
		t.Fatalf("T004-C20-STRICT %s overflows time.Duration: %q", key, raw)
	}
	return duration
}

func synthesisC20Key(principal string, cadence intelligence.SynthesisCadence, now time.Time) intelligence.SynthesisRunKey {
	windowEnd := now.Add(-time.Hour)
	windowLength := 24 * time.Hour
	if cadence == intelligence.CadenceWeekly {
		windowLength = 7 * 24 * time.Hour
	}
	return intelligence.SynthesisRunKey{
		Cadence: cadence, Principal: principal,
		WindowStart: windowEnd.Add(-windowLength), WindowEnd: windowEnd,
		PolicyVersion: "t004-c20-strict-v1",
	}
}

func startSynthesisC20Attempt(
	t *testing.T,
	persistence *intelligence.SynthesisPersistence,
	key intelligence.SynthesisRunKey,
	now time.Time,
) intelligence.SynthesisAttempt {
	t.Helper()
	attempt, err := persistence.StartAttempt(context.Background(), key,
		intelligence.TriggerScheduled, "t004-c20-"+string(key.Cadence), time.Hour, now)
	if err != nil {
		t.Fatalf("start T004-C20-STRICT %s attempt: %v", key.Cadence, err)
	}
	return attempt
}

func seedSynthesisC20Output(
	t *testing.T,
	persistence *intelligence.SynthesisPersistence,
	principal string,
	cadence intelligence.SynthesisCadence,
	kind intelligence.SynthesisOutputKind,
	committedAt time.Time,
) {
	t.Helper()
	key := synthesisC20Key(principal, cadence, committedAt)
	attempt := startSynthesisC20Attempt(t, persistence, key, committedAt.Add(-time.Minute))
	candidate := intelligence.SynthesisCandidate{Key: key, Kind: kind}
	policy := intelligence.SourceClassPolicy{Optional: []string{"optional-source"}}
	if kind == intelligence.OutputKindPartial {
		candidate.OmittedClasses = []string{"optional-source"}
	}
	if _, err := persistence.CommitAttempt(context.Background(), attempt, candidate, policy, nil, committedAt); err != nil {
		t.Fatalf("commit T004-C20-STRICT %s %s output: %v", cadence, kind, err)
	}
}

func markSynthesisC20ReadDegraded(
	t *testing.T,
	pool *pgxpool.Pool,
	persistence *intelligence.SynthesisPersistence,
	attempt intelligence.SynthesisAttempt,
	finishedAt time.Time,
) {
	t.Helper()
	ctx := context.Background()
	repairReadback := installSynthesisC20ReadbackFailure(t, pool)
	t.Cleanup(repairReadback)

	const artifactID = "t004-c20-readback-artifact"
	candidate := intelligence.SynthesisCandidate{
		Key:  attempt.Key,
		Kind: intelligence.OutputKindFull,
		Insights: []intelligence.SynthesisInsight{{
			InsightType:       intelligence.InsightThroughLine,
			ThroughLine:       "strict health observes a production read-back failure",
			SourceArtifactIDs: []string{artifactID},
			Confidence:        0.5,
		}},
		EvaluatedArtifactCount: 1,
		IncludedClasses:        []string{"article"},
	}
	_, readbackErr := persistence.CommitAttempt(ctx, attempt, candidate,
		intelligence.SourceClassPolicy{Required: []string{"article"}}, []string{artifactID}, finishedAt)
	if readbackErr == nil {
		t.Fatal("T004-C20-STRICT read-degraded attempt returned success while the production read-back relation was unavailable")
	}
	var typedReadback *intelligence.SynthesisReadbackError
	if !errors.As(readbackErr, &typedReadback) {
		t.Fatalf("T004-C20-STRICT read-degraded error type=%T, want *SynthesisReadbackError", readbackErr)
	}
	if typedReadback.OutputID == "" {
		t.Fatal("T004-C20-STRICT typed read-back failure carried no output id")
	}
	repairReadback()

	var attemptState, attemptOutputID string
	if err := pool.QueryRow(ctx, `
		SELECT state, output_id
		FROM synthesis_run_attempts
		WHERE run_id = $1 AND attempt_no = $2
	`, attempt.RunID, attempt.AttemptNo).Scan(&attemptState, &attemptOutputID); err != nil {
		t.Fatalf("read T004-C20-STRICT read-degraded attempt: %v", err)
	}
	if attemptState != string(intelligence.EventReadbackFailed) || attemptOutputID != typedReadback.OutputID {
		t.Fatalf("T004-C20-STRICT read-degraded attempt state/output=%s/%s, want readback_failed/%s",
			attemptState, attemptOutputID, typedReadback.OutputID)
	}

	var eventOutputID string
	if err := pool.QueryRow(ctx, `
		SELECT output_id
		FROM synthesis_run_events
		WHERE run_id = $1 AND attempt_no = $2
			AND event_type = 'readback_failed' AND failure_code = 'readback_failed'
	`, attempt.RunID, attempt.AttemptNo).Scan(&eventOutputID); err != nil {
		t.Fatalf("read T004-C20-STRICT readback_failed event: %v", err)
	}
	if eventOutputID != typedReadback.OutputID {
		t.Fatalf("T004-C20-STRICT readback_failed event output=%s, want %s", eventOutputID, typedReadback.OutputID)
	}
}

func installSynthesisC20ReadbackFailure(t *testing.T, pool *pgxpool.Pool) func() {
	t.Helper()
	const install = `
		CREATE OR REPLACE FUNCTION test_break_synthesis_c20_readback_relation()
		RETURNS TRIGGER LANGUAGE plpgsql AS $$
		BEGIN
			ALTER TABLE synthesis_output_source_classes
				RENAME TO synthesis_output_source_classes_c20_readback_fault;
			RETURN NEW;
		END;
		$$;
		CREATE TRIGGER test_break_synthesis_c20_readback_relation
		AFTER INSERT ON synthesis_citations
		FOR EACH ROW WHEN (NEW.artifact_id = 't004-c20-readback-artifact')
		EXECUTE FUNCTION test_break_synthesis_c20_readback_relation();`
	if _, err := pool.Exec(context.Background(), install); err != nil {
		t.Fatalf("install T004-C20-STRICT read-back relation failure: %v", err)
	}

	repaired := false
	return func() {
		if repaired {
			return
		}
		repairSucceeded := true
		if _, err := pool.Exec(context.Background(), `
			DROP TRIGGER IF EXISTS test_break_synthesis_c20_readback_relation ON synthesis_citations
		`); err != nil {
			repairSucceeded = false
			t.Errorf("remove T004-C20-STRICT read-back failure trigger: %v", err)
		}
		if _, err := pool.Exec(context.Background(), `
			DROP FUNCTION IF EXISTS test_break_synthesis_c20_readback_relation()
		`); err != nil {
			repairSucceeded = false
			t.Errorf("remove T004-C20-STRICT read-back failure function: %v", err)
		}
		var faultRelationExists bool
		if err := pool.QueryRow(context.Background(), `
			SELECT to_regclass('public.synthesis_output_source_classes_c20_readback_fault') IS NOT NULL
		`).Scan(&faultRelationExists); err != nil {
			repairSucceeded = false
			t.Errorf("probe T004-C20-STRICT read-back fault relation: %v", err)
		} else if faultRelationExists {
			if _, err := pool.Exec(context.Background(), `
				ALTER TABLE synthesis_output_source_classes_c20_readback_fault
					RENAME TO synthesis_output_source_classes
			`); err != nil {
				repairSucceeded = false
				t.Errorf("repair T004-C20-STRICT read-back relation: %v", err)
			}
		}
		if repairSucceeded {
			repaired = true
		}
	}
}

func synthesisC20HealthServer(
	t *testing.T,
	pool *pgxpool.Pool,
	natsClient *smacknats.Client,
	freshnessPolicy intelligence.SynthesisFreshnessPolicy,
	principal string,
	authToken string,
) *httptest.Server {
	t.Helper()
	readModel, err := intelligence.NewSynthesisReadModel(pool)
	if err != nil {
		t.Fatalf("construct T004-C20-STRICT read model: %v", err)
	}
	dependencies := &api.Dependencies{
		DB: &smackdb.Postgres{Pool: pool}, NATS: natsClient,
		IntelligenceEngine: intelligence.NewEngine(pool, natsClient),
		SynthesisReadModel: readModel, SynthesisReadPrincipal: principal,
		SynthesisFreshnessPolicy:   freshnessPolicy,
		IntelligenceHealthCacheTTL: 0,
		StartTime:                  time.Now(), AuthToken: authToken,
	}
	server := httptest.NewServer(http.HandlerFunc(dependencies.HealthHandler))
	t.Cleanup(server.Close)
	return server
}

func assertSynthesisC20StrictHealth(
	t *testing.T,
	cfg e2eConfig,
	wantIntelligence string,
	wantStatusCode int,
	wantOverall string,
) {
	t.Helper()
	status, body := synthesisGet(t, cfg, "/api/health?strict=true", true)
	if status != wantStatusCode {
		t.Fatalf("authenticated strict health returned %d, want %d; body=%s", status, wantStatusCode, string(body))
	}
	var authenticated api.HealthResponse
	if err := json.Unmarshal(body, &authenticated); err != nil {
		t.Fatalf("decode authenticated strict health: %v; body=%s", err, string(body))
	}
	if authenticated.Status != wantOverall {
		t.Fatalf("authenticated strict overall=%q, want %q; body=%s", authenticated.Status, wantOverall, string(body))
	}
	if got := authenticated.Services["intelligence"].Status; got != wantIntelligence {
		t.Fatalf("authenticated strict intelligence=%q, want %q; body=%s", got, wantIntelligence, string(body))
	}
	for name, service := range authenticated.Services {
		if name == "intelligence" || name == "telegram_bot" || name == "ollama" {
			continue
		}
		switch service.Status {
		case "down", "stale", "error", "failing", "disconnected", "degraded":
			t.Fatalf("strict-health control service %s=%q; intelligence must be the discriminating dependency; body=%s", name, service.Status, string(body))
		}
	}

	publicStatus, publicBody := synthesisGet(t, cfg, "/api/health?strict=true", false)
	if publicStatus != wantStatusCode {
		t.Fatalf("public strict health returned %d, want %d; body=%s", publicStatus, wantStatusCode, string(publicBody))
	}
	var public api.HealthResponse
	if err := json.Unmarshal(publicBody, &public); err != nil {
		t.Fatalf("decode public strict health: %v; body=%s", err, string(publicBody))
	}
	if public.Status != wantOverall || len(public.Services) != 0 || public.Version != "" || public.CommitHash != "" || public.BuildTime != "" {
		t.Fatalf("public strict health exposed detail or wrong aggregate: %+v body=%s", public, string(publicBody))
	}
}

func synthesisPost(t *testing.T, cfg e2eConfig, path, body string, authorized bool) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, cfg.CoreURL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request %s: %v", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if authorized {
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	}
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return resp.StatusCode, out
}

// The retry route reports what HAPPENED, not that the request was accepted.
// A 202 would be the same shape of claim the original defect made: true about
// the request, silent about whether anything was stored.
func TestSynthesisAPI_RetryReportsAPersistedOutcome(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	status, body := synthesisPost(t, cfg, "/api/synthesis/retry", `{"cadence":"daily"}`, true)
	if status == http.StatusAccepted {
		t.Fatalf("retry returned 202; the route must report the outcome, not merely that it was asked. body=%s", string(body))
	}
	if status != http.StatusOK && status != http.StatusConflict {
		t.Fatalf("retry returned %d, want 200 or 409; body=%s", status, string(body))
	}

	var parsed struct {
		Outcome string `json:"outcome"`
		Output  *struct {
			OutputID string `json:"outputId"`
			Kind     string `json:"kind"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode retry: %v; body=%s", err, string(body))
	}

	switch parsed.Outcome {
	case "persisted":
		if parsed.Output == nil || parsed.Output.OutputID == "" {
			t.Fatalf("outcome persisted carried no output id; that is the claim the defect made. body=%s", string(body))
		}
		// The id must resolve. An id that reads back as absent would mean the
		// route reported a commit that is not there.
		detailStatus, detailBody := synthesisGet(t, cfg, "/api/synthesis/runs/"+parsed.Output.OutputID, true)
		if detailStatus != http.StatusOK {
			t.Fatalf("retry reported output %s but reading it returned %d; a reported commit must be readable. body=%s",
				parsed.Output.OutputID, detailStatus, string(detailBody))
		}
	case "claimed-elsewhere":
		if parsed.Output != nil {
			t.Fatalf("claimed-elsewhere carried an output; this process did not produce one. body=%s", string(body))
		}
	default:
		t.Fatalf("retry returned unknown outcome %q; the vocabulary is closed. body=%s", parsed.Outcome, string(body))
	}
}

// An unrecognised cadence must be refused, never defaulted. Silently running
// daily would produce a real output answering a question nobody asked.
func TestSynthesisAPI_RetryRefusesUnknownCadence(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	for _, body := range []string{`{"cadence":"hourly"}`, `{"cadence":""}`, `{}`, `not json`} {
		status, out := synthesisPost(t, cfg, "/api/synthesis/retry", body, true)
		if status != http.StatusBadRequest {
			t.Fatalf("retry with %s returned %d, want 400; body=%s", body, status, string(out))
		}
	}
}

func TestSynthesisAPI_RetryDeniesUnauthenticatedCallers(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	status, body := synthesisPost(t, cfg, "/api/synthesis/retry", `{"cadence":"daily"}`, false)
	if status != http.StatusUnauthorized && status != http.StatusForbidden {
		t.Fatalf("unauthenticated retry returned %d, want 401 or 403; body=%s", status, string(body))
	}
}

// T004-02-RESTART. Duplicate triggers converge on ONE durable identity.
//
// HONEST SCOPE, stated so nobody reads more into this than it proves. It does
// not restart a process. What it does prove is the property that makes a
// restart safe: identity lives in the database, not in the handler's memory.
// Each HTTP request is handled independently with no shared in-process state
// between them, so a second trigger that converges on the first output could
// only have done so by reading durable state. A coordinator holding identity in
// a package-level map would satisfy a single-request test and fail this one.
func TestSynthesisAPI_DuplicateTriggersShareOneDurableIdentity(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	first := synthesisRetryOutcome(t, cfg)
	second := synthesisRetryOutcome(t, cfg)

	if first.outputID == "" && second.outputID == "" {
		t.Fatal("neither trigger produced an output id; this test proves nothing without one")
	}

	// Whichever arm produced an id, the other must either match it or report
	// that the window was already claimed. What must NOT happen is two ids.
	if first.outputID != "" && second.outputID != "" && first.outputID != second.outputID {
		t.Fatalf("two triggers produced different outputs (%s and %s); one window must have one durable identity",
			first.outputID, second.outputID)
	}

	// The history listing is the durable record, and it must not have grown a
	// duplicate entry for the same window.
	status, body := synthesisGet(t, cfg, "/api/synthesis/runs?limit=50", true)
	if status != http.StatusOK {
		t.Fatalf("read history: %d; body=%s", status, string(body))
	}
	var parsed struct {
		Runs []struct {
			OutputID    string `json:"outputId"`
			WindowStart string `json:"windowStart"`
			WindowEnd   string `json:"windowEnd"`
			Cadence     string `json:"cadence"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	seen := map[string]string{}
	for _, run := range parsed.Runs {
		window := run.Cadence + "|" + run.WindowStart + "|" + run.WindowEnd
		if prior, dup := seen[window]; dup {
			t.Fatalf("window %s has two outputs (%s and %s); duplicate triggers must converge, not accumulate",
				window, prior, run.OutputID)
		}
		seen[window] = run.OutputID
	}
}

// T004-06-RECOVERY. Health recovers only after an output is persisted AND
// readable -- never on the strength of the request having been accepted.
func TestSynthesisAPI_RecoveryFollowsPersistedReadBack(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	outcome := synthesisRetryOutcome(t, cfg)
	if outcome.outputID == "" {
		t.Fatalf("window was claimed elsewhere (outcome=%q); this required recovery path cannot pass without an output from its own trigger", outcome.outcome)
	}

	// The reported output must be READABLE. Recovery asserted on the strength of
	// a 200 alone would repeat the defect: a success report about a row nobody
	// checked for.
	detailStatus, detailBody := synthesisGet(t, cfg, "/api/synthesis/runs/"+outcome.outputID, true)
	if detailStatus != http.StatusOK {
		t.Fatalf("reported output %s reads back as %d; recovery must rest on a verified read, not on the report. body=%s",
			outcome.outputID, detailStatus, string(detailBody))
	}

	// And /latest must now agree, from the same durable read model.
	latestStatus, latestBody := synthesisGet(t, cfg, "/api/synthesis/latest", true)
	if latestStatus != http.StatusOK {
		t.Fatalf("latest returned %d; body=%s", latestStatus, string(latestBody))
	}
	var latest synthesisLatestBody
	if err := json.Unmarshal(latestBody, &latest); err != nil {
		t.Fatalf("decode latest: %v", err)
	}
	if latest.State == "never-run" {
		t.Fatalf("latest reports never-run after a persisted output; the API and the store disagree. body=%s", string(latestBody))
	}
	if latest.Output == nil || latest.Output.OutputID == "" {
		t.Fatalf("latest carries no output after a persisted run. body=%s", string(latestBody))
	}
}

type synthesisRetryResult struct {
	outcome  string
	outputID string
}

func synthesisRetryOutcome(t *testing.T, cfg e2eConfig) synthesisRetryResult {
	t.Helper()
	status, body := synthesisPost(t, cfg, "/api/synthesis/retry", `{"cadence":"daily"}`, true)
	if status != http.StatusOK && status != http.StatusConflict {
		t.Fatalf("retry returned %d, want 200 or 409; body=%s", status, string(body))
	}
	var parsed struct {
		Outcome string `json:"outcome"`
		Output  *struct {
			OutputID string `json:"outputId"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode retry: %v; body=%s", err, string(body))
	}
	res := synthesisRetryResult{outcome: parsed.Outcome}
	if parsed.Output != nil {
		res.outputID = parsed.Output.OutputID
	}
	return res
}

// SCN-004-004-04 through the live API. A rejected candidate must leave NOTHING
// behind, so no output the API can serve may be half-written. The visible
// signature of a partial write is an aggregate whose declared counts disagree
// with the rows it actually returns, or a cited insight with no citation.
func TestSynthesisAPI_NoOutputIsEverHalfWritten(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	// Drive a real commit so the assertion runs against a populated surface
	// rather than passing on an empty list.
	synthesisRetryOutcome(t, cfg)

	status, body := synthesisGet(t, cfg, "/api/synthesis/runs?limit=25", true)
	if status != http.StatusOK {
		t.Fatalf("GET runs returned %d, want 200; body=%s", status, string(body))
	}
	var listed struct {
		Runs []struct {
			OutputID string `json:"outputId"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatalf("decode runs: %v; body=%s", err, string(body))
	}
	if len(listed.Runs) == 0 {
		t.Fatalf("no runs to inspect after a retry; the check would be vacuous. body=%s", string(body))
	}

	for _, run := range listed.Runs {
		detailStatus, detailBody := synthesisGet(t, cfg, "/api/synthesis/runs/"+run.OutputID, true)
		if detailStatus != http.StatusOK {
			t.Fatalf("GET run %s returned %d, want 200; body=%s", run.OutputID, detailStatus, string(detailBody))
		}
		// Counts live under "output"; reading them from the top level would
		// silently yield zero and make every assertion below vacuous.
		var agg struct {
			Output struct {
				OutputID      string `json:"outputId"`
				Kind          string `json:"kind"`
				InsightCount  int    `json:"insightCount"`
				CitationCount int    `json:"citationCount"`
			} `json:"output"`
			Insights []struct {
				InsightType string   `json:"insightType"`
				Citations   []string `json:"citations"`
			} `json:"insights"`
		}
		if err := json.Unmarshal(detailBody, &agg); err != nil {
			t.Fatalf("decode run %s: %v; body=%s", run.OutputID, err, string(detailBody))
		}

		if agg.Output.OutputID == "" {
			t.Fatalf("run %s detail carried no output id; the response shape is not what this check assumes. body=%s",
				run.OutputID, string(detailBody))
		}
		if agg.Output.InsightCount != len(agg.Insights) {
			t.Fatalf("output %s declares %d insights but carries %d; a count that outruns its rows is the signature of a partial write",
				run.OutputID, agg.Output.InsightCount, len(agg.Insights))
		}
		citations := 0
		for _, in := range agg.Insights {
			if in.InsightType == "" {
				t.Fatalf("output %s carries an insight with no type; the row is incomplete", run.OutputID)
			}
			if len(in.Citations) == 0 {
				t.Fatalf("output %s carries an uncited insight; validation must have refused it before any write", run.OutputID)
			}
			citations += len(in.Citations)
		}
		if agg.Output.CitationCount != citations {
			t.Fatalf("output %s declares %d citations but carries %d; the aggregate is not whole",
				run.OutputID, agg.Output.CitationCount, citations)
		}
		if agg.Output.Kind == "quiet" && len(agg.Insights) != 0 {
			t.Fatalf("output %s is quiet yet carries %d insights", run.OutputID, len(agg.Insights))
		}
	}
}

// SCN-004-004-07 through the live API. A quiet window means the system looked
// and found nothing worth saying. That is a SUCCESSFUL run, and the API must
// never let a caller read it as never-run or failed -- the exact confusion this
// bug exists to remove.
func TestSynthesisAPI_QuietWindowReadsAsRunNotBroken(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)

	result := synthesisRetryOutcome(t, cfg)
	if result.outputID == "" {
		t.Fatalf("retry produced no output id (outcome %q); a completed trigger must name its output", result.outcome)
	}

	status, body := synthesisGet(t, cfg, "/api/synthesis/latest", true)
	if status != http.StatusOK {
		t.Fatalf("GET latest returned %d, want 200; body=%s", status, string(body))
	}
	var parsed synthesisLatestBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode latest: %v; body=%s", err, string(body))
	}

	if parsed.State == "never-run" {
		t.Fatalf("a committed output is present (%s) yet latest reports never-run; emptiness is being read as absence. body=%s",
			result.outputID, string(body))
	}
	if parsed.State == "failed" {
		t.Fatalf("a committed output is present (%s) yet latest reports failed; a quiet window is a success. body=%s",
			result.outputID, string(body))
	}
	if parsed.Output == nil {
		t.Fatalf("state %q carried no output despite a committed run. body=%s", parsed.State, string(body))
	}
	if parsed.State == "quiet" && parsed.Output.InsightCount != 0 {
		t.Fatalf("quiet state reported %d insights; the kind and the payload disagree. body=%s",
			parsed.Output.InsightCount, string(body))
	}
}

const synthesisTerminalEventFailureProfile = "synthesis_terminal_event_persistence_failure"

// SCN-004-004-C12 / T004-C12-AUDIT-RED. The fault is activated only after
// resolving the canonical test-only acceptance profile, then applied at the
// real PostgreSQL terminal-event boundary. The request still traverses bearer
// auth, the mounted POST /api/synthesis/retry handler, producer, coordinator,
// aggregate persistence, production read-back, and terminal-event persistence.
func TestSynthesisAPI_TerminalEventFailureCannotProduceDeliveryOrSuccess(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 60*time.Second)
	pool := synthesisE2EPool(t, "T004-C12-AUDIT-RED")
	resetSynthesisE2EState(t, pool, "T004-C12-AUDIT-RED")

	deactivateFault := activateSynthesisTerminalEventFailure(t, pool)
	status, body := synthesisPost(t, cfg, "/api/synthesis/retry", `{"cadence":"daily"}`, true)
	if status == http.StatusOK || status == http.StatusAccepted {
		t.Fatalf("terminal-event rejection returned false success status %d; body=%s", status, string(body))
	}
	if status != http.StatusInternalServerError {
		t.Fatalf("terminal-event rejection returned %d, want 500; body=%s", status, string(body))
	}

	var failure struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &failure); err != nil {
		t.Fatalf("decode terminal-event failure response: %v; body=%s", err, string(body))
	}
	if failure.Error.Code != "audit_persistence_failed" {
		t.Fatalf("terminal-event failure code=%q, want audit_persistence_failed; body=%s", failure.Error.Code, string(body))
	}
	if failure.Error.Message != "Required synthesis audit record could not be stored" {
		t.Fatalf("terminal-event failure message=%q, want safe closed message; body=%s", failure.Error.Message, string(body))
	}

	var responseFields map[string]json.RawMessage
	if err := json.Unmarshal(body, &responseFields); err != nil {
		t.Fatalf("decode terminal-event failure fields: %v; body=%s", err, string(body))
	}
	for _, forbiddenField := range []string{"outcome", "output", "outputId", "delivered", "delivery", "healthy", "persisted", "recovered"} {
		if _, present := responseFields[forbiddenField]; present {
			t.Fatalf("terminal-event failure exposed false-success field %q; body=%s", forbiddenField, string(body))
		}
	}
	for _, forbiddenLeak := range []string{
		"forced terminal event write failure",
		"23514",
		"insert into",
		"synthesis_run_events",
		"throughline",
		"sourceartifact",
	} {
		if strings.Contains(strings.ToLower(string(body)), forbiddenLeak) {
			t.Fatalf("terminal-event failure leaked internal/content marker %q; body=%s", forbiddenLeak, string(body))
		}
	}

	latestStatus, latestBody := synthesisGet(t, cfg, "/api/synthesis/latest", true)
	if latestStatus != http.StatusOK {
		t.Fatalf("latest after terminal-event failure returned %d, want 200; body=%s", latestStatus, string(latestBody))
	}
	var latest synthesisLatestBody
	if err := json.Unmarshal(latestBody, &latest); err != nil {
		t.Fatalf("decode latest after terminal-event failure: %v; body=%s", err, string(latestBody))
	}
	if latest.State != "never-run" || latest.Output != nil {
		t.Fatalf("terminal-event failure surfaced an output state=%q output=%+v; body=%s", latest.State, latest.Output, string(latestBody))
	}

	historyStatus, historyBody := synthesisGet(t, cfg, "/api/synthesis/runs?limit=10", true)
	if historyStatus != http.StatusOK {
		t.Fatalf("history after terminal-event failure returned %d, want 200; body=%s", historyStatus, string(historyBody))
	}
	var history struct {
		Runs []json.RawMessage `json:"runs"`
	}
	if err := json.Unmarshal(historyBody, &history); err != nil {
		t.Fatalf("decode history after terminal-event failure: %v; body=%s", err, string(historyBody))
	}
	if len(history.Runs) != 0 {
		t.Fatalf("terminal-event failure exposed %d successful history row(s); body=%s", len(history.Runs), string(historyBody))
	}

	healthStatus, healthBody := synthesisGet(t, cfg, "/api/health?strict=true", true)
	if healthStatus != http.StatusServiceUnavailable {
		t.Fatalf("strict health after terminal-event failure returned %d, want 503; body=%s", healthStatus, string(healthBody))
	}
	var health struct {
		Status   string `json:"status"`
		Services map[string]struct {
			Status string `json:"status"`
		} `json:"services"`
	}
	if err := json.Unmarshal(healthBody, &health); err != nil {
		t.Fatalf("decode strict health after terminal-event failure: %v; body=%s", err, string(healthBody))
	}
	intelligenceHealth, present := health.Services["intelligence"]
	if !present {
		t.Fatalf("strict health omitted intelligence state after a synthesis attempt; body=%s", string(healthBody))
	}
	if health.Status == "healthy" || intelligenceHealth.Status == "up" {
		t.Fatalf("terminal-event failure produced a healthy claim overall=%q intelligence=%q; body=%s",
			health.Status, intelligenceHealth.Status, string(healthBody))
	}

	assertSynthesisTerminalEventFailureState(t, pool)

	// Positive control: removing only the registered fault must let the SAME
	// production route finish, publish its terminal audit event, and return an
	// output that the mounted production read route can retrieve.
	deactivateFault()
	positiveStatus, positiveBody := synthesisPost(t, cfg, "/api/synthesis/retry", `{"cadence":"daily"}`, true)
	if positiveStatus != http.StatusOK {
		t.Fatalf("positive-control retry returned %d, want 200; body=%s", positiveStatus, string(positiveBody))
	}
	var positive struct {
		Outcome string `json:"outcome"`
		Output  *struct {
			OutputID string `json:"outputId"`
		} `json:"output"`
	}
	if err := json.Unmarshal(positiveBody, &positive); err != nil {
		t.Fatalf("decode positive-control retry: %v; body=%s", err, string(positiveBody))
	}
	if positive.Outcome != "persisted" || positive.Output == nil || positive.Output.OutputID == "" {
		t.Fatalf("positive-control retry did not report a persisted identity; body=%s", string(positiveBody))
	}
	detailStatus, detailBody := synthesisGet(t, cfg, "/api/synthesis/runs/"+positive.Output.OutputID, true)
	if detailStatus != http.StatusOK {
		t.Fatalf("positive-control output %s did not read back: status=%d body=%s",
			positive.Output.OutputID, detailStatus, string(detailBody))
	}
	var detail struct {
		Output struct {
			OutputID string `json:"outputId"`
		} `json:"output"`
	}
	if err := json.Unmarshal(detailBody, &detail); err != nil {
		t.Fatalf("decode positive-control read-back: %v; body=%s", err, string(detailBody))
	}
	if detail.Output.OutputID != positive.Output.OutputID {
		t.Fatalf("positive-control read-back id=%q, want %q", detail.Output.OutputID, positive.Output.OutputID)
	}
}

func synthesisE2EPool(t *testing.T, testID string) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Fatalf("%s requires DATABASE_URL from ./smackerel.sh test e2e", testID)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse %s DATABASE_URL: %v", testID, err)
	}
	config.MaxConns = 2
	config.MinConns = 0
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("connect %s PostgreSQL: %v", testID, err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping %s PostgreSQL: %v", testID, err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func resetSynthesisE2EState(t *testing.T, pool *pgxpool.Pool, testID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `
		TRUNCATE synthesis_run_events, synthesis_citations,
			synthesis_output_insights, synthesis_output_source_classes,
			synthesis_outputs, synthesis_run_attempts, synthesis_runs
		RESTART IDENTITY CASCADE
	`); err != nil {
		t.Fatalf("reset %s synthesis state: %v", testID, err)
	}
}

func activateSynthesisTerminalEventFailure(t *testing.T, pool *pgxpool.Pool) func() {
	t.Helper()
	registry, err := acceptance.LoadCanonicalRegistry()
	if err != nil {
		t.Fatalf("load canonical fault-profile registry: %v", err)
	}
	profile, err := registry.Resolve(acceptance.PostureTest, synthesisTerminalEventFailureProfile)
	if err != nil {
		t.Fatalf("resolve T004-C12 test fault profile: %v", err)
	}
	if profile.Journey != "synthesis" || profile.NoFirstPartyInterception == nil || !*profile.NoFirstPartyInterception {
		t.Fatalf("T004-C12 fault profile is not a synthesis/no-interception profile: %+v", profile)
	}
	if _, err := registry.Resolve(acceptance.PostureProduction, synthesisTerminalEventFailureProfile); !errors.Is(err, acceptance.ErrFaultInertInProduction) {
		t.Fatalf("T004-C12 fault profile activated under production posture: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `
		DROP TRIGGER IF EXISTS e2e_fail_synthesis_terminal_event ON synthesis_run_events;
		DROP FUNCTION IF EXISTS e2e_fail_synthesis_terminal_event();
		CREATE FUNCTION e2e_fail_synthesis_terminal_event()
		RETURNS TRIGGER LANGUAGE plpgsql AS $$
		BEGIN
			RAISE EXCEPTION 'forced terminal event write failure'
				USING ERRCODE = '23514';
		END;
		$$;
		CREATE TRIGGER e2e_fail_synthesis_terminal_event
		BEFORE INSERT ON synthesis_run_events
		FOR EACH ROW WHEN (NEW.event_type IN (
			'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
			'retryable_failure', 'failed', 'readback_failed', 'recovered'
		))
		EXECUTE FUNCTION e2e_fail_synthesis_terminal_event();
	`); err != nil {
		t.Fatalf("activate T004-C12 terminal-event fault: %v", err)
	}

	active := true
	deactivate := func() {
		if !active {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, `
			DROP TRIGGER IF EXISTS e2e_fail_synthesis_terminal_event ON synthesis_run_events;
			DROP FUNCTION IF EXISTS e2e_fail_synthesis_terminal_event();
		`); err != nil {
			t.Fatalf("deactivate T004-C12 terminal-event fault: %v", err)
		}
		active = false
	}
	t.Cleanup(deactivate)
	return deactivate
}

func assertSynthesisTerminalEventFailureState(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var runState, runLifecycle, attemptState, attemptOutcome string
	var attemptOutputID, failureCode, failureMessage string
	var includedCount, omittedCount, insightCount, citationCount int
	if err := pool.QueryRow(ctx, `
		SELECT r.state, r.lifecycle_state, a.state, a.outcome,
			COALESCE(a.output_id, ''), COALESCE(a.failure_code, ''),
			COALESCE(a.failure_message, ''),
			cardinality(a.included_source_classes),
			cardinality(a.omitted_source_classes),
			a.insight_count, a.citation_count
		FROM synthesis_run_attempts a
		JOIN synthesis_runs r ON r.id = a.run_id
		WHERE a.trigger_kind = 'operator_retry'
		ORDER BY a.recorded_at DESC, a.id DESC
		LIMIT 1
	`).Scan(&runState, &runLifecycle, &attemptState, &attemptOutcome,
		&attemptOutputID, &failureCode, &failureMessage,
		&includedCount, &omittedCount, &insightCount, &citationCount); err != nil {
		t.Fatalf("read T004-C12 attempt state: %v", err)
	}
	if runState == "succeeded" || attemptState != "running" || attemptOutcome != "running" {
		t.Fatalf("terminal-event failure state run=%q lifecycle=%q attempt=%q outcome=%q, want no success and an unfinalized attempt",
			runState, runLifecycle, attemptState, attemptOutcome)
	}
	if runLifecycle != "superseded" || attemptOutputID != "" || failureCode != "" || failureMessage != "" ||
		includedCount != 0 || omittedCount != 0 || insightCount != 0 || citationCount != 0 {
		t.Fatalf("terminal-event failure audit summary is not content-free/truthful: runLifecycle=%q output=%q code=%q message=%q included=%d omitted=%d insights=%d citations=%d",
			runLifecycle, attemptOutputID, failureCode, failureMessage,
			includedCount, omittedCount, insightCount, citationCount)
	}

	var totalOutputs, currentOutputs, succeededRuns int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM synthesis_outputs),
			(SELECT COUNT(*) FROM synthesis_outputs WHERE lifecycle_state = 'current'),
			(SELECT COUNT(*) FROM synthesis_runs WHERE state = 'succeeded')
	`).Scan(&totalOutputs, &currentOutputs, &succeededRuns); err != nil {
		t.Fatalf("count T004-C12 output state: %v", err)
	}
	if totalOutputs != 1 || currentOutputs != 0 || succeededRuns != 0 {
		t.Fatalf("terminal-event failure outputs total/current/succeeded-runs=%d/%d/%d, want 1 forensic/0/0",
			totalOutputs, currentOutputs, succeededRuns)
	}

	var startedEvents, terminalEvents, unsafeEventRows int
	if err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE event_type = 'attempt_started'),
			COUNT(*) FILTER (WHERE event_type IN (
				'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
				'retryable_failure', 'failed', 'readback_failed', 'recovered'
			)),
			COUNT(*) FILTER (WHERE output_id IS NOT NULL
				OR related_output_id IS NOT NULL OR failure_code IS NOT NULL
				OR insight_count IS NOT NULL OR citation_count IS NOT NULL)
		FROM synthesis_run_events
	`).Scan(&startedEvents, &terminalEvents, &unsafeEventRows); err != nil {
		t.Fatalf("read T004-C12 event state: %v", err)
	}
	if startedEvents != 1 || terminalEvents != 0 || unsafeEventRows != 0 {
		t.Fatalf("terminal-event failure events started/terminal/unsafe=%d/%d/%d, want 1/0/0",
			startedEvents, terminalEvents, unsafeEventRows)
	}
}
