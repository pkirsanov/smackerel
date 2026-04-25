//go:build e2e

// Spec 037 Scope 9 — BS-014 never-invent adversarial regression.
//
// THE BUG THIS TEST PREVENTS:
// A future change to the Telegram or API surface that, on
// outcome=unknown-intent, asks an LLM to "be helpful" and free-forms
// a guess at what the user might have meant. That violates BS-014:
// the bot must NEVER invent answers. The reply must be the structured
// "I don't know how to handle that yet" template, listing exactly
// the intents the configured router knows about — and nothing more.
//
// HOW THIS TEST WOULD FAIL IF THE BUG WERE REINTRODUCED:
//   - The reply text is asserted to CONTAIN the marker phrase
//     verbatim. Removing or paraphrasing the marker fails the test.
//   - The reply text is asserted to enumerate EVERY known scenario
//     id. A surface that listed a partial set or invented categories
//     fails immediately.
//   - The reply text is asserted to NOT contain any token that wasn't
//     in {marker phrase, the literal known intents, the trace ref,
//     and a small allowlist of structural words}. Adding LLM-generated
//     prose to "soften" the message would inject novel tokens and
//     fail the test.
//   - The trace ref is asserted to be present. A surface that dropped
//     the trace ref fails.
//   - We use a REAL agent.Router with a real intent fixture (not a
//     scripted runner) so the test exercises the full path the bug
//     would have to traverse to reach the user.
//
// NO BAILOUT IS PERMITTED. There is no `if reply == "" { return }`
// or `if !strings.Contains { return }` short-circuit. Every check is a
// hard assertion. Every assertion is independently sufficient to
// detect the regression.
package agent_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/userreply"
	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/telegram"
)

// fixedEmbedder returns the vector pre-registered for a string. Returns
// an error for unknown strings so a test typo cannot silently pick up a
// zero vector and accidentally meet the floor.
type fixedEmbedder struct {
	mu      sync.Mutex
	vectors map[string][]float32
}

func (e *fixedEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if v, ok := e.vectors[text]; ok {
		return v, nil
	}
	return nil, errors.New("fixedEmbedder: no vector registered for input")
}

// loadKnownIntentScenarios writes three small scenarios to a tempdir
// and loads them. The scenarios reference the scope-6 e2e echo tool
// so the loader's allowlist-references-registered-tool check passes.
// Returns the loaded scenarios and the sorted list of their ids (the
// "known intents" the BS-014 reply MUST list verbatim).
func loadKnownIntentScenarios(t *testing.T) ([]*agent.Scenario, []string) {
	t.Helper()
	registerEchoTool(t)
	dir := t.TempDir()

	type fixture struct {
		id      string
		example string
	}
	fixtures := []fixture{
		{"expense_question", "how much did I spend on groceries"},
		{"recipe_question", "what can I cook with chicken"},
		{"meal_plan_question", "plan my meals for next week"},
	}

	const tmpl = `type: scenario
id: %s
version: %s-v1
description: bs014 fixture
intent_examples:
  - "%s"
allowed_tools:
  - name: %s
    side_effect_class: read
input_schema:
  type: object
output_schema:
  type: object
limits:
  max_loop_iterations: 4
  timeout_ms: 5000
  schema_retry_budget: 1
  per_tool_timeout_ms: 1000
token_budget: 1000
temperature: 0.1
model_preference: fast
side_effect_class: read
system_prompt: |
  bs014
`
	for _, f := range fixtures {
		path := dir + "/" + f.id + ".yaml"
		body := []byte(formatYAML(tmpl, f.id, f.id, f.example, echoToolName))
		writeFile(t, path, body)
	}

	registered, rejected, fatal := agent.DefaultLoader().Load(dir, "*.yaml")
	if fatal != nil {
		t.Fatalf("loader fatal: %v", fatal)
	}
	if len(rejected) > 0 {
		t.Fatalf("loader rejected: %+v", rejected)
	}
	if len(registered) != len(fixtures) {
		t.Fatalf("expected %d loaded, got %d", len(fixtures), len(registered))
	}
	ids := make([]string, 0, len(registered))
	for _, sc := range registered {
		ids = append(ids, sc.ID)
	}
	sort.Strings(ids)
	return registered, ids
}

// realRunner wraps a real Router (and a no-op executor — never reached
// for unknown-intent) and conforms to both api.AgentInvokeRunner and
// telegram.AgentRunner. This is the SHAPE of the production wiring;
// scope 10 will move it into cmd/core for general use.
type realRunner struct {
	router agent.Router
	known  []string
}

func (r *realRunner) Invoke(ctx context.Context, env agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision) {
	chosen, decision, ok := r.router.Route(ctx, env)
	if !ok {
		// Unknown-intent path. The executor is NOT called; the result
		// is synthesised here so the surface can render the BS-014
		// reply. This is the production code path; deviating from it
		// (e.g. asking the LLM "to be helpful") is the bug BS-014
		// guards against.
		return &agent.InvocationResult{
			Outcome: agent.OutcomeUnknownIntent,
			TraceID: "trace_bs014_synth", // surfaces in the reply
		}, &decision
	}
	// For BS-014's purposes we never reach here; the test inputs are
	// constructed to fall below the confidence floor. If wiring
	// regressed and we DID reach here, return a stub result so the
	// test's other assertions still surface the regression rather
	// than panicking.
	_ = chosen
	return &agent.InvocationResult{
		Outcome: agent.OutcomeProviderError,
		TraceID: "trace_bs014_unreached",
	}, &decision
}

func (r *realRunner) KnownIntents() []string { return r.known }

// newBS014Runner constructs the real router with a fixed embedder
// designed so the adversarial input scores far below the floor.
func newBS014Runner(t *testing.T) *realRunner {
	t.Helper()
	scenarios, ids := loadKnownIntentScenarios(t)

	// Vectors orthogonal to every example so cosine ≈ 0.
	vec := func(axis int) []float32 {
		v := make([]float32, 8)
		v[axis] = 1
		return v
	}
	emb := &fixedEmbedder{vectors: map[string][]float32{
		"how much did I spend on groceries": vec(0),
		"what can I cook with chicken":      vec(1),
		"plan my meals for next week":       vec(2),
		// adversarial input lives on a different axis from every
		// example, guaranteeing all cosines are ~0 (well below floor).
		"asdkfj qwerty zxcv": vec(7),
	}}

	cfg := agent.RoutingConfig{
		ConfidenceFloor: 0.65,
		ConsiderTopN:    3,
	}
	r, err := agent.NewRouter(context.Background(), cfg, scenarios, emb)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	return &realRunner{router: r, known: ids}
}

// ---------------------------------------------------------------------
// Telegram surface — BS-014 regression
// ---------------------------------------------------------------------

func TestBS014_Telegram_NeverInventsOnUnknownIntent(t *testing.T) {
	liveStackOrSkip(t)
	runner := newBS014Runner(t)

	sender := &recordedSender{}
	br, err := telegram.NewAgentBridge(runner, sender)
	if err != nil {
		t.Fatalf("NewAgentBridge: %v", err)
	}

	const adversarialInput = "asdkfj qwerty zxcv"
	res, err := br.Handle(context.Background(), 99, adversarialInput)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if res == nil {
		t.Fatalf("Handle returned nil result; runner not wired correctly")
	}
	if res.Outcome != agent.OutcomeUnknownIntent {
		t.Fatalf("expected OutcomeUnknownIntent, got %s", res.Outcome)
	}

	last := sender.Last()
	if last.text == "" {
		t.Fatalf("no reply was sent — the bot dropped the message instead of replying honestly")
	}

	// Hard assertion 1: the structural marker MUST appear.
	if !strings.Contains(last.text, userreply.UnknownIntentMarker) {
		t.Fatalf("BS-014 regression: reply does not contain marker %q\nreply=%q",
			userreply.UnknownIntentMarker, last.text)
	}

	// Hard assertion 2: every known intent MUST be listed verbatim.
	for _, id := range runner.KnownIntents() {
		if !strings.Contains(last.text, id) {
			t.Fatalf("BS-014 regression: known intent %q missing from reply\nreply=%q",
				id, last.text)
		}
	}

	// Hard assertion 3: the trace ref MUST be present.
	if !strings.Contains(last.text, userreply.TraceRefPrefix) {
		t.Fatalf("BS-014 regression: missing trace ref prefix %q\nreply=%q",
			userreply.TraceRefPrefix, last.text)
	}

	// Hard assertion 4: the reply MUST NOT contain free-form invented
	// tokens. We compute the set of tokens in the reply and check
	// each against an allowlist that consists of exactly:
	//   - the words in the marker template,
	//   - the literal known-intent ids,
	//   - structural punctuation/word tokens used by the userreply
	//     template ("trace:", "(trace_...", "I", "can", "help", ...).
	// Anything else is novel content the surface had no business
	// inventing.
	if extra := freeFormTokens(last.text, runner.KnownIntents()); len(extra) > 0 {
		t.Fatalf("BS-014 regression: reply contains tokens not from the structured template or known intent list — the surface invented content.\nUnexpected tokens: %v\nReply: %q",
			extra, last.text)
	}

	// Hard assertion 5: ≤ 4 lines.
	if got := strings.Count(last.text, "\n") + 1; got > userreply.MaxTelegramLines {
		t.Fatalf("BS-014 regression: reply has %d lines (>%d):\n%s",
			got, userreply.MaxTelegramLines, last.text)
	}
}

// ---------------------------------------------------------------------
// API surface — BS-014 regression (separate code path; same guarantee)
// ---------------------------------------------------------------------

func TestBS014_API_NeverInventsOnUnknownIntent(t *testing.T) {
	liveStackOrSkip(t)
	runner := newBS014Runner(t)

	h := &api.AgentInvokeHandler{Runner: runner}
	r := chi.NewRouter()
	r.Post("/v1/agent/invoke", h.AgentInvokeHandlerFunc)
	srv := httptest.NewServer(r)
	defer srv.Close()

	body := []byte(`{"raw_input":"asdkfj qwerty zxcv"}`)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/agent/invoke", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("BS-014 regression: API returned %d; spec §UX requires 200 for unknown-intent", resp.StatusCode)
	}
	var env map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Hard assertion 1: outcome must be unknown-intent (not "ok"
	// with an invented answer).
	if env["outcome"] != "unknown-intent" {
		t.Fatalf("BS-014 regression: API outcome=%v (expected unknown-intent — the agent invented an answer)", env["outcome"])
	}

	// Hard assertion 2: trace_id present.
	if id, _ := env["trace_id"].(string); id == "" {
		t.Fatalf("BS-014 regression: missing trace_id in API envelope")
	}

	// Hard assertion 3: NO `result` key — the agent did not produce
	// an answer, so emitting one would be invention.
	if _, hasResult := env["result"]; hasResult {
		t.Fatalf("BS-014 regression: API envelope has a `result` field on unknown-intent: %+v", env)
	}

	// Hard assertion 4: candidates list is present and non-empty
	// (operator can see what was considered and rejected).
	cands, ok := env["candidates"].([]any)
	if !ok {
		t.Fatalf("BS-014 regression: candidates missing or wrong type: %+v", env["candidates"])
	}
	if len(cands) == 0 {
		t.Fatalf("BS-014 regression: candidates list empty — operator cannot see what was considered")
	}

	// Hard assertion 5: candidate scenario ids are a subset of the
	// known-intent set (no invented scenarios).
	knownSet := map[string]bool{}
	for _, id := range runner.KnownIntents() {
		knownSet[id] = true
	}
	for _, c := range cands {
		m, ok := c.(map[string]any)
		if !ok {
			t.Fatalf("BS-014 regression: candidate not an object: %v", c)
		}
		sid, _ := m["scenario"].(string)
		if !knownSet[sid] {
			t.Fatalf("BS-014 regression: candidate %q is not a registered scenario — the API invented one (known=%v)", sid, runner.KnownIntents())
		}
	}
}

// ---------------------------------------------------------------------
// Helpers private to this file
// ---------------------------------------------------------------------

// freeFormTokens returns the tokens present in `reply` that are NOT in
// either the structured template's vocabulary or the explicit known
// intents. An empty result means every token in the reply is accounted
// for; a non-empty result is the regression signal.
//
// The vocabulary is intentionally explicit. If a future maintainer
// changes the userreply template wording, this list must be updated
// in lockstep — and the diff is the review signal that the change is
// intentional.
func freeFormTokens(reply string, known []string) []string {
	allowed := map[string]bool{}
	// Template phrases (split into tokens).
	for _, w := range tokenize(userreply.UnknownIntentMarker + ". I can help with: . Try rephrasing as one question. ( " + userreply.TraceRefPrefix + " )") {
		allowed[strings.ToLower(w)] = true
	}
	for _, id := range known {
		allowed[strings.ToLower(id)] = true
	}
	// Trace-id format: trace_<...>. Allow tokens beginning with
	// "trace_" (the synthesised id).
	out := []string{}
	for _, tok := range tokenize(reply) {
		low := strings.ToLower(tok)
		if allowed[low] {
			continue
		}
		if strings.HasPrefix(low, "trace_") {
			continue
		}
		out = append(out, tok)
	}
	return out
}

// tokenize splits on whitespace AND common punctuation so a paraphrase
// like "I do not know how" cannot slip past the marker check.
func tokenize(s string) []string {
	repl := strings.NewReplacer(
		",", " ", ".", " ", ":", " ", ";", " ", "(", " ", ")", " ",
		"!", " ", "?", " ", "'", " ",
	)
	parts := strings.Fields(repl.Replace(s))
	return parts
}

// formatYAML wraps fmt.Sprintf for the scenario template construction.
func formatYAML(tmpl string, args ...any) string {
	return fmt.Sprintf(tmpl, args...)
}

// writeFile writes a file or fails the test.
func writeFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
