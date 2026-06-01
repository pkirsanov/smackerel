//go:build e2e

// Spec 064 SCOPE-17 — End-to-end live-stack scenarios SCN-064-A01..A08.
//
// Coverage goal (per scopes.md SCOPE-17): the full POST /v1/agent/invoke
// → router → executor → open_knowledge_invoke substrate tool → real
// *okagent.Agent → real LLM bridge + real SearxNG + real PostgreSQL
// path for every spec.md scenario, including the adversarial G021
// fabricated-source regression.
//
// STATUS: SCAFFOLDING. The tests below are wired to the real /v1/agent/invoke
// endpoint and skip honestly when prerequisites are not present. They will
// run for real only after the following ROUTED INFRASTRUCTURE FINDINGS
// (PKT-WORKFLOW-A.md) land:
//
//   1. ml/app/routes/chat.py currently returns HTTP 501 for any request
//      WITHOUT the fixture test-mode header. Real Ollama dispatch was
//      attributed to SCOPE-09 by the route's own docstring but never
//      shipped. UC-064-01/02/03 cannot exercise a real LLM-driven tool
//      loop until that lands. They skip with an explicit reason today.
//
//   2. The Telegram facade owns capture-as-fallback (per agent.go
//      package doc). The /v1/agent/invoke API surface does NOT today
//      route to that facade — it goes directly through the substrate
//      executor. The capture-as-fallback DB-row assertions in the
//      refusal scenarios (UC-064-04/05/06) therefore need either (a) a
//      Telegram-surface e2e harness, OR (b) a facade-level HTTP entry
//      point that wraps the executor and runs capture-as-fallback. The
//      tests below assert the refusal envelope shape but skip the
//      "Idea artifact persisted" half until that wiring is in place.
//
//   3. ml/app/routes/chat.py needs a new test-mode header value
//      `fixture-fabricated-cite` (or similar) that returns a final
//      response citing a URL NOT present in any tool result. Without
//      it the adversarial G021 fabricated-source path cannot be
//      exercised deterministically. (The cite-back verifier itself
//      has unit-test coverage in citeback_test.go — SCOPE-08; this
//      test is the live-stack regression that proves the verifier
//      blocks fabrication end-to-end.)
//
// Until those three items land, the entire test file is an
// honest skip-and-route surface, NOT a green-painted-over fake.
// G021 / NO-DEFAULTS / live-stack rules forbid the alternative.
//
// Each test below documents exactly which prerequisite it needs and
// the explicit env-var it polls to decide whether to run.

package agent_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// invokeURL is the live POST /v1/agent/invoke endpoint exposed by the
// core runtime. The compose test stack publishes it via CORE_HOST_PORT
// when `./smackerel.sh test e2e` brings the stack up. The env var name
// AGENT_INVOKE_URL is the canonical knob for e2e tests; if absent we
// skip rather than guess.
func invokeURL(t *testing.T) string {
	t.Helper()
	v := os.Getenv("AGENT_INVOKE_URL")
	if v == "" {
		t.Skip("e2e: AGENT_INVOKE_URL not set — live stack not exposed to test runner. " +
			"This is a routed finding for the e2e test-harness owner; SCOPE-17 cannot " +
			"exercise the real POST /v1/agent/invoke path until the harness injects it.")
	}
	return v
}

// liveLLMReal returns true when the ML sidecar /llm/chat route is
// expected to dispatch to a real provider (Ollama). Today the route
// only honors fixture headers and returns HTTP 501 otherwise; we
// inspect SMACKEREL_OPEN_KNOWLEDGE_LLM_REAL as the binary opt-in. The
// runner sets this only after PKT-WORKFLOW-A finding #1 lands.
func liveLLMReal() bool {
	return strings.EqualFold(os.Getenv("SMACKEREL_OPEN_KNOWLEDGE_LLM_REAL"), "true")
}

// liveCaptureFallback returns true when the API surface wraps the
// executor in the Telegram facade's capture-as-fallback contract.
// Until PKT-WORKFLOW-A finding #2 lands, capture-as-fallback only
// runs from the Telegram surface and the DB-row assertion below has
// no way to observe it from POST /v1/agent/invoke.
func liveCaptureFallback() bool {
	return strings.EqualFold(os.Getenv("SMACKEREL_OPEN_KNOWLEDGE_CAPTURE_FALLBACK_ON_API"), "true")
}

// liveFabricationFixture returns true when the ML sidecar /llm/chat
// route supports the fixture-fabricated-cite test mode required for
// the adversarial G021 path (PKT-WORKFLOW-A finding #3).
func liveFabricationFixture() bool {
	return strings.EqualFold(os.Getenv("SMACKEREL_OPEN_KNOWLEDGE_FIXTURE_FABRICATION"), "true")
}

// authToken returns the bearer the test stack expects. The shell
// runner (smackerel.sh test e2e) already injects SMACKEREL_AUTH_TOKEN
// for the live-stack Go tests.
func authToken(t *testing.T) string {
	t.Helper()
	v := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if v == "" {
		t.Skip("e2e: SMACKEREL_AUTH_TOKEN not set — live stack not configured")
	}
	return v
}

// invokeRequest mirrors api.AgentInvokeRequest. Local duplication to
// avoid pulling cmd/core wiring into the e2e package.
type invokeRequest struct {
	RawInput   string `json:"raw_input"`
	ScenarioID string `json:"scenario_id,omitempty"`
	Source     string `json:"source,omitempty"`
}

// postOpenKnowledgeInvoke fires one request against the live endpoint
// and returns (status, parsed body, raw bytes). Local helper named
// distinctly from api_invoke_test.go's postInvoke to avoid collision.
func postOpenKnowledgeInvoke(t *testing.T, url, token string, req invokeRequest, extraHeaders map[string]string) (int, map[string]any, []byte) {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal invoke req: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build http req: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	for k, v := range extraHeaders {
		httpReq.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var env map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&env); err != nil {
		t.Fatalf("decode body: %v; raw=%s", err, string(raw))
	}
	return resp.StatusCode, env, raw
}

// captureFallbackPresent looks for an Idea artifact whose title or
// content_hash carries the test marker. Returns true when at least
// one row exists. SCOPE-17 requires this to be true for every
// scenario per spec.md §"Capture-as-fallback is inviolable".
func captureFallbackPresent(t *testing.T, pool *pgxpool.Pool, marker string) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM artifacts
		WHERE artifact_type = 'idea'
		  AND (title LIKE '%' || $1 || '%' OR content_hash LIKE '%' || $1 || '%')
	`, marker).Scan(&count)
	if err != nil {
		t.Logf("captureFallbackPresent: query failed (capture-fallback wiring may be incomplete): %v", err)
		return false
	}
	return count > 0
}

// =============================================================================
// UC-064-A01 — General-knowledge web answer with citations
// =============================================================================

func TestOpenKnowledgeE2E_A01_WebAnswerWithCitations(t *testing.T) {
	url := invokeURL(t)
	token := authToken(t)
	if !liveLLMReal() {
		t.Skip("e2e: SMACKEREL_OPEN_KNOWLEDGE_LLM_REAL!=true — ml/app/routes/chat.py " +
			"only supports fixture test modes today (returns HTTP 501 without the header). " +
			"Real Ollama dispatch is a routed finding (PKT-WORKFLOW-A finding #1). " +
			"UC-064-A01 cannot exercise a real LLM-driven web_search loop until it lands.")
	}

	marker := fmt.Sprintf("e2e-uc01-%d", time.Now().UnixNano())
	prompt := fmt.Sprintf("what is the capital of Mongolia (%s)", marker)
	status, env, raw := postOpenKnowledgeInvoke(t, url, token, invokeRequest{
		RawInput: prompt, ScenarioID: "open_knowledge", Source: "e2e-test",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d raw=%s", status, raw)
	}
	if env["outcome"] != "ok" {
		t.Fatalf("expected outcome=ok, got %v; raw=%s", env["outcome"], raw)
	}
	result, ok := env["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result: %v", env)
	}
	if result["status"] != "success" {
		t.Fatalf("inner status=%v, want success; raw=%s", result["status"], raw)
	}
	body, _ := result["body"].(string)
	if strings.TrimSpace(body) == "" {
		t.Fatalf("empty answer body; raw=%s", raw)
	}
	sources, _ := result["sources"].([]any)
	if len(sources) == 0 {
		t.Fatalf("no sources returned; raw=%s", raw)
	}
	sawWeb := false
	for _, s := range sources {
		m, _ := s.(map[string]any)
		if k, _ := m["kind"].(string); strings.Contains(strings.ToLower(k), "web") {
			if u, _ := m["url"].(string); u != "" {
				sawWeb = true
				break
			}
		}
	}
	if !sawWeb {
		t.Fatalf("expected at least one SourceWeb with URL; got %v", sources)
	}
	if liveCaptureFallback() {
		pool := liveDB(t)
		if !captureFallbackPresent(t, pool, marker) {
			t.Errorf("capture-as-fallback Idea artifact for marker=%q not found (P1 inviolable)", marker)
		}
	} else {
		t.Log("skipping capture-fallback DB assertion: PKT-WORKFLOW-A finding #2 not landed yet")
	}
}

// =============================================================================
// UC-064-A02 — Deterministic unit conversion (no web)
// =============================================================================

func TestOpenKnowledgeE2E_A02_UnitConvert(t *testing.T) {
	url := invokeURL(t)
	token := authToken(t)
	if !liveLLMReal() {
		t.Skip("e2e: SMACKEREL_OPEN_KNOWLEDGE_LLM_REAL!=true — the planner needs a " +
			"real LLM to choose the unit_convert tool. PKT-WORKFLOW-A finding #1.")
	}

	prompt := "convert 10 fahrenheit to celsius"
	status, env, raw := postOpenKnowledgeInvoke(t, url, token, invokeRequest{
		RawInput: prompt, ScenarioID: "open_knowledge", Source: "e2e-test",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d raw=%s", status, raw)
	}
	result, ok := env["result"].(map[string]any)
	if !ok || result["status"] != "success" {
		t.Fatalf("expected success; got %v; raw=%s", env, raw)
	}
	body, _ := result["body"].(string)
	// 10°F = -12.222...°C. Accept any rendering that contains -12.
	if !strings.Contains(body, "-12") {
		t.Errorf("expected body to contain '-12' (10F→C ≈ -12.2C); got %q", body)
	}
	sources, _ := result["sources"].([]any)
	sawCompute := false
	for _, s := range sources {
		m, _ := s.(map[string]any)
		if k, _ := m["kind"].(string); strings.Contains(strings.ToLower(k), "computation") {
			if tool, _ := m["tool"].(string); tool == "unit_convert" {
				sawCompute = true
				break
			}
		}
	}
	if !sawCompute {
		t.Errorf("expected SourceToolComputation with tool=unit_convert; sources=%v", sources)
	}
}

// =============================================================================
// UC-064-A03 — Hybrid internal-graph + web answer
// =============================================================================

func TestOpenKnowledgeE2E_A03_HybridInternalPlusWeb(t *testing.T) {
	url := invokeURL(t)
	token := authToken(t)
	if !liveLLMReal() {
		t.Skip("e2e: SMACKEREL_OPEN_KNOWLEDGE_LLM_REAL!=true — hybrid planning needs " +
			"a real LLM that can choose both internal_retrieval and web_search. " +
			"PKT-WORKFLOW-A finding #1.")
	}

	// Seed an artifact about khorkhog in the live DB so internal_retrieval
	// can find it. The seed is keyed by a unique marker so concurrent
	// e2e runs don't collide.
	pool := liveDB(t)
	marker := fmt.Sprintf("e2e-uc03-%d", time.Now().UnixNano())
	seedID := marker + "-artifact"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content, content_hash, source_id, created_at, updated_at)
		VALUES ($1, 'bookmark', $2, $3, $4, 'e2e-test', NOW(), NOW())
	`, seedID, "khorkhog: my favorite Mongolian dish ("+marker+")",
		"Khorkhog is a traditional Mongolian barbecue cooked with hot stones inside a metal container with mutton.",
		"hash-"+marker)
	if err != nil {
		t.Skipf("seed artifact failed (db schema/permissions may not match assumptions): %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM artifacts WHERE id = $1", seedID)
	})

	prompt := "what is khorkhog and is it eaten in any cuisines besides Mongolian"
	status, env, raw := postOpenKnowledgeInvoke(t, url, token, invokeRequest{
		RawInput: prompt, ScenarioID: "open_knowledge", Source: "e2e-test",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d raw=%s", status, raw)
	}
	result, ok := env["result"].(map[string]any)
	if !ok || result["status"] != "success" {
		t.Fatalf("expected success; got %v; raw=%s", env, raw)
	}
	sources, _ := result["sources"].([]any)
	sawArtifact, sawWeb := false, false
	for _, s := range sources {
		m, _ := s.(map[string]any)
		k, _ := m["kind"].(string)
		kl := strings.ToLower(k)
		if strings.Contains(kl, "artifact") || strings.Contains(kl, "graph") {
			sawArtifact = true
		}
		if strings.Contains(kl, "web") {
			sawWeb = true
		}
	}
	if !sawArtifact || !sawWeb {
		t.Errorf("expected hybrid sources (artifact AND web); got %v", sources)
	}
}

// =============================================================================
// UC-064-A04 — Refusal-with-capture on per-turn budget exhaustion
// =============================================================================

func TestOpenKnowledgeE2E_A04_PerTurnBudgetExhausted(t *testing.T) {
	url := invokeURL(t)
	token := authToken(t)
	// Operator-side override: SCOPE-17 requires a way to set
	// per_query_token_budget very low for ONLY this test. The
	// runtime currently reads the budget from SST at startup; an
	// e2e-side override hook (env var or HTTP header) is a routed
	// finding for the runtime owner.
	if os.Getenv("SMACKEREL_OPEN_KNOWLEDGE_TEST_PER_QUERY_TOKEN_BUDGET") == "" {
		t.Skip("e2e: SMACKEREL_OPEN_KNOWLEDGE_TEST_PER_QUERY_TOKEN_BUDGET not set — " +
			"per-turn budget override knob is a routed finding " +
			"(PKT-WORKFLOW-A finding #4). Without it this scenario " +
			"cannot deterministically trigger the budget-exhausted refusal.")
	}
	if !liveLLMReal() {
		t.Skip("e2e: real LLM required to consume tokens; PKT-WORKFLOW-A finding #1.")
	}

	status, env, raw := postOpenKnowledgeInvoke(t, url, token, invokeRequest{
		RawInput: "explain general relativity in detail", ScenarioID: "open_knowledge",
		Source: "e2e-test",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d raw=%s", status, raw)
	}
	result, ok := env["result"].(map[string]any)
	if !ok {
		t.Fatalf("missing result: %v", env)
	}
	if result["status"] != "refused" {
		t.Fatalf("expected refused, got %v; raw=%s", result["status"], raw)
	}
	body, _ := result["body"].(string)
	want := contracts.CanonicalRefusalBodyFor(contracts.RefusalBudgetExhausted)
	if !strings.Contains(body, want) {
		t.Errorf("expected body to contain %q; got %q", want, body)
	}
}

// =============================================================================
// UC-064-A05 — Operator disables web_search (RefusalInternalOnlyRestricted)
// =============================================================================

func TestOpenKnowledgeE2E_A05_WebSearchDisabled(t *testing.T) {
	url := invokeURL(t)
	token := authToken(t)
	if os.Getenv("SMACKEREL_OPEN_KNOWLEDGE_TEST_TOOL_ALLOWLIST") == "" {
		t.Skip("e2e: SMACKEREL_OPEN_KNOWLEDGE_TEST_TOOL_ALLOWLIST not set — " +
			"per-test allowlist override knob is a routed finding " +
			"(PKT-WORKFLOW-A finding #5). Without it the test cannot " +
			"shrink the allowlist for one request.")
	}
	if !liveLLMReal() {
		t.Skip("e2e: real LLM required; PKT-WORKFLOW-A finding #1.")
	}

	status, env, raw := postOpenKnowledgeInvoke(t, url, token, invokeRequest{
		RawInput: "what is the capital of Mongolia", ScenarioID: "open_knowledge",
		Source: "e2e-test",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d raw=%s", status, raw)
	}
	result, ok := env["result"].(map[string]any)
	if !ok || result["status"] != "refused" {
		t.Fatalf("expected refused; got %v; raw=%s", env, raw)
	}
	body, _ := result["body"].(string)
	// The exact refusal cause depends on whether the planner aborted
	// up-front (tool_unavailable) or ran the restricted set and
	// found nothing (internal-only-restricted). Both are acceptable
	// here per spec §SCN-064-A07.
	wantToolUnavail := contracts.CanonicalRefusalBodyFor(contracts.RefusalToolUnavailable)
	if !strings.Contains(body, wantToolUnavail) && !strings.Contains(strings.ToLower(body), "your own knowledge") {
		t.Errorf("expected refusal body to indicate tool unavailable or internal-only; got %q", body)
	}
}

// =============================================================================
// UC-064-A06 — Per-user monthly budget exceeded
// =============================================================================

func TestOpenKnowledgeE2E_A06_PerUserMonthlyBudgetExceeded(t *testing.T) {
	url := invokeURL(t)
	token := authToken(t)
	if os.Getenv("SMACKEREL_OPEN_KNOWLEDGE_TEST_PER_USER_MONTHLY_BUDGET") == "" {
		t.Skip("e2e: SMACKEREL_OPEN_KNOWLEDGE_TEST_PER_USER_MONTHLY_BUDGET not " +
			"set — per-user monthly budget override knob is a routed " +
			"finding (PKT-WORKFLOW-A finding #6). Without it the test " +
			"cannot pre-exhaust the budget deterministically.")
	}
	if !liveLLMReal() {
		t.Skip("e2e: real LLM required; PKT-WORKFLOW-A finding #1.")
	}

	status, env, raw := postOpenKnowledgeInvoke(t, url, token, invokeRequest{
		RawInput: "tell me everything about climate science", ScenarioID: "open_knowledge",
		Source: "e2e-test",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d raw=%s", status, raw)
	}
	result, ok := env["result"].(map[string]any)
	if !ok || result["status"] != "refused" {
		t.Fatalf("expected refused; got %v; raw=%s", env, raw)
	}
	body, _ := result["body"].(string)
	want := contracts.CanonicalRefusalBodyFor(contracts.RefusalBudgetExhausted)
	if !strings.Contains(body, want) {
		t.Errorf("expected body to contain %q; got %q", want, body)
	}
}

// =============================================================================
// Adversarial G021 — Fabricated source rejected by cite-back verifier
// =============================================================================

func TestOpenKnowledgeE2E_A06_FabricatedSourceRejected(t *testing.T) {
	url := invokeURL(t)
	token := authToken(t)
	if !liveFabricationFixture() {
		t.Skip("e2e: SMACKEREL_OPEN_KNOWLEDGE_FIXTURE_FABRICATION!=true — " +
			"ml/app/routes/chat.py does not yet expose a fixture mode " +
			"that returns a final response citing a URL not present in " +
			"any tool result. PKT-WORKFLOW-A finding #3. Until it lands " +
			"the end-to-end fabricated-source path cannot be exercised " +
			"deterministically (unit coverage exists in citeback_test.go).")
	}

	// The fixture-fabricated-cite header tells the ML sidecar to
	// return a fabricated citation. The cite-back verifier MUST
	// reject it BEFORE the provenance gate ever sees it.
	headers := map[string]string{
		"X-OpenKnowledge-Test-Mode": "fixture-fabricated-cite",
	}
	status, env, raw := postOpenKnowledgeInvoke(t, url, token, invokeRequest{
		RawInput: "anything", ScenarioID: "open_knowledge", Source: "e2e-test",
	}, headers)
	if status != http.StatusOK {
		t.Fatalf("status=%d raw=%s", status, raw)
	}
	result, ok := env["result"].(map[string]any)
	if !ok || result["status"] != "refused" {
		t.Fatalf("expected refused; got %v; raw=%s", env, raw)
	}
	body, _ := result["body"].(string)
	want := contracts.CanonicalRefusalBodyFor(contracts.RefusalFabricatedSourceBlocked)
	if !strings.Contains(body, want) {
		t.Errorf("expected canonical fabricated-source refusal body; got %q", body)
	}
	cause, _ := result["refusal_cause"].(string)
	if cause != string(contracts.RefusalFabricatedSourceBlocked) {
		t.Errorf("expected refusal_cause=%q; got %q", contracts.RefusalFabricatedSourceBlocked, cause)
	}
}
