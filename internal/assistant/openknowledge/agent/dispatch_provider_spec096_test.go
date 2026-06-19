// Spec 096 SCOPE-07 — the agent-side LIVE WIRING of the provider-aware dispatch
// resolver into the /ask loop (the deferred SCOPE-03 follow-up). These tests
// pin three behaviors at the agent spine:
//
//  1. a selected HOSTED provider-qualified model routes through the installed
//     resolver — the dispatched ChatRequest carries the resolver's provider
//     credential seam (Provider + api_base + api_key + provider_params) and the
//     BARE backend Model, while TurnResult.Model keeps the provider-qualified
//     attribution (ADVERSARIAL — FAILS before the agent.go injection);
//  2. a hosted resolve ERROR REFUSES the turn with a typed reason and dispatches
//     NOTHING — never a silent Ollama fallback — and the raw error / connection
//     identity never reaches the user (ADVERSARIAL — FAILS before the injection,
//     the agent half of TestDispatchResolver_…_NeverFallsBackToOllama_Spec096);
//  3. a BARE or "ollama/…" model NEVER consults the resolver and reaches Chat
//     with an empty Provider — the byte-for-byte spec 089 path (GUARD — green
//     before and after, the fallback gate).
//
// Reuses the shared harness (recordingLLM, spec087Cfg, toolUse, endTurn,
// webCiteEntry, citationsBlock, newRegistry, textPtr) from agent_test.go /
// synthesis_spec087_test.go / reasoning_loop_spec084_test.go.
//
// Resolver installation note: these tests are in-package (package agent) so they
// can use the unexported harness; the agent package cannot import agenttool
// (agenttool imports agent — a cycle), so the fake is installed through the
// agent's own late-bound source (SetDispatchResolverSource) and reset to nil in
// cleanup. That is the agent-side equivalent of agenttool.SetDispatchResolver
// (which the production agenttool bridge forwards into this same source).
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
)

// fakeDispatchResolver is an injected agent.DispatchResolver: it counts every
// Resolve call and returns either a programmed ResolvedDispatch (success) or a
// programmed error keyed by the provider-qualified model id. A model with no
// programmed entry is a test failure, so an UNEXPECTED consult is caught loud
// rather than silently tolerated.
type fakeDispatchResolver struct {
	t        *testing.T
	calls    int
	resolved map[string]llm.ResolvedDispatch
	errs     map[string]error
}

func (f *fakeDispatchResolver) Resolve(model string) (llm.ResolvedDispatch, error) {
	f.calls++
	if e, ok := f.errs[model]; ok {
		return llm.ResolvedDispatch{}, e
	}
	if rd, ok := f.resolved[model]; ok {
		return rd, nil
	}
	f.t.Fatalf("fakeDispatchResolver: unexpected Resolve(%q) — no programmed result", model)
	return llm.ResolvedDispatch{}, nil
}

// installFakeDispatchResolver wires the fake as the agent's late-bound resolver
// source and resets it to nil on cleanup so the global binding never leaks
// across tests.
func installFakeDispatchResolver(t *testing.T, f *fakeDispatchResolver) {
	t.Helper()
	SetDispatchResolverSource(func() DispatchResolver { return f })
	t.Cleanup(func() { SetDispatchResolverSource(nil) })
}

// ── #1 — hosted model dispatches via the resolver ────────────────────────────

// TestAgent_HostedModel_DispatchesViaResolver_Spec096 — ADVERSARIAL / RED-before.
// FAILS before the agent.go injection: the resolver is unconsumed, so the hosted
// turn's request carries an empty Provider and no credential seam. The bare
// gather turn proves the resolver is applied ONLY to the hosted turn, never to a
// bare model.
func TestAgent_HostedModel_DispatchesViaResolver_Spec096(t *testing.T) {
	const hostedModel = "anthropic/claude-3-5-sonnet"
	const apiBase = "https://api.anthropic.test/v1"
	const apiKey = "sk-ant-SYNTHETIC-TEST-CREDENTIAL-000000" // gitleaks:allow — synthetic, never a real secret
	providerParams := map[string]any{"anthropic_version": "2023-06-01"}

	f := &fakeDispatchResolver{
		t: t,
		resolved: map[string]llm.ResolvedDispatch{
			hostedModel: {
				Request: llm.ChatRequest{
					Model:          "claude-3-5-sonnet", // BARE backend id (qualifier stripped)
					Provider:       "anthropic",
					APIBase:        textPtr(apiBase),
					APIKey:         textPtr(apiKey),
					ProviderParams: providerParams,
				},
				Attribution: hostedModel,
			},
		},
	}
	installFakeDispatchResolver(t, f)

	const maxIter = 2 // iter0 bare gather turn, iter1 hosted forced-final synthesis turn
	r := newRegistry(t)
	verdict := "A grounded answer." + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
	fl := &recordingLLM{t: t, responses: []llm.Result{
		toolUse("w0", "fake_web", `{"query":"q"}`, 100),
		endTurn(verdict, 80),
	}}
	base, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// The hosted model is the selected SYNTHESIS model (Fork B); the gather turn
	// keeps the bare SST baseline ("gather-model").
	a := base.WithModelOverride(modelswitch.Override{SynthesisModel: hostedModel})
	got, err := a.Run(context.Background(), "a question")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("status=%q reason=%q want success", got.Status, got.RefusalReason)
	}
	if len(fl.requests) != maxIter {
		t.Fatalf("recorded %d requests, want %d", len(fl.requests), maxIter)
	}

	// The bare gather turn took the byte-for-byte 089 path — Provider empty.
	if fl.requests[0].Provider != "" {
		t.Fatalf("gather turn (bare model) MUST keep an empty Provider (089 path), got %q", fl.requests[0].Provider)
	}

	// The hosted synthesis turn was populated FROM the resolver.
	syn := fl.requests[maxIter-1]
	if syn.Provider != "anthropic" {
		t.Fatalf("hosted turn Provider = %q, want \"anthropic\" (resolver unconsumed?)", syn.Provider)
	}
	if syn.Model != "claude-3-5-sonnet" {
		t.Fatalf("hosted turn Model = %q, want the BARE backend id \"claude-3-5-sonnet\"", syn.Model)
	}
	if syn.APIBase == nil || *syn.APIBase != apiBase {
		t.Fatalf("hosted turn APIBase = %v, want %q (from the resolver)", syn.APIBase, apiBase)
	}
	if syn.APIKey == nil || *syn.APIKey != apiKey {
		t.Fatalf("hosted turn APIKey not populated from the resolver")
	}
	if syn.ProviderParams["anthropic_version"] != "2023-06-01" {
		t.Fatalf("hosted turn ProviderParams = %v, want the resolver's params", syn.ProviderParams)
	}
	if f.calls != 1 {
		t.Fatalf("resolver consulted %d times, want exactly 1 (the hosted turn only)", f.calls)
	}

	// Attribution stays provider-qualified — NOT coerced to the bare backend id.
	if got.Model != hostedModel {
		t.Fatalf("TurnResult.Model = %q, want the provider-qualified id %q", got.Model, hostedModel)
	}

	// Secret discipline: the decrypted credential rides ONLY the ChatRequest
	// seam, never the user-visible TurnResult.
	if strings.Contains(got.FinalText, apiKey) || strings.Contains(got.RefusalReason, apiKey) {
		t.Fatalf("api_key leaked into the user-visible TurnResult")
	}
}

// ── #2 — hosted resolve error → typed refusal, no Ollama fallback ─────────────

// TestAgent_HostedResolveError_RefusesNoOllamaFallback_Spec096 — ADVERSARIAL /
// RED-before. When the resolver REJECTS a selected hosted model (typed
// *llm.ResolveError), the turn REFUSES with TerminationDispatchRejected and
// dispatches NOTHING — never a silent Ollama fallback (FR-X1 / SCN-096-G01) —
// and the raw error / connection identity never reaches the user-visible
// TurnResult. FAILS before the injection: the resolver is unconsumed, so the
// turn proceeds to a normal success instead of refusing.
func TestAgent_HostedResolveError_RefusesNoOllamaFallback_Spec096(t *testing.T) {
	const hostedModel = "anthropic/claude-3-5-sonnet"
	const connCanary = "conn-LEAK-CANARY-anthropic-001"
	rawErr := &llm.ResolveError{
		Reason: llm.RejectDecryptFailed,
		Model:  hostedModel,
		Kind:   "anthropic",
		ConnID: connCanary,
	}
	f := &fakeDispatchResolver{t: t, errs: map[string]error{hostedModel: rawErr}}
	installFakeDispatchResolver(t, f)

	const maxIter = 2
	r := newRegistry(t)
	// A grounded answer is programmed so that, WITHOUT the injection, the turn
	// succeeds (crisp RED: success→refused). WITH the injection it must stay
	// unused — a refusal dispatches nothing.
	verdict := "A grounded answer." + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
	fl := &recordingLLM{t: t, responses: []llm.Result{
		endTurn(verdict, 80),
	}}
	base, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// The hosted model is the GATHER model, so the refusal lands on the FIRST
	// turn — before any llm.Chat dispatch — making the no-fallback proof crisp.
	a := base.WithModelOverride(modelswitch.Override{GatherModel: hostedModel})
	got, err := a.Run(context.Background(), "a question")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got.Status != StatusRefused {
		t.Fatalf("status=%q want refused (a hosted resolve error MUST refuse the turn)", got.Status)
	}
	if got.TerminationReason != TerminationDispatchRejected {
		t.Fatalf("termination=%q want %q", got.TerminationReason, TerminationDispatchRejected)
	}
	// No silent Ollama fallback: NOTHING was dispatched after the resolve error.
	if len(fl.requests) != 0 {
		t.Fatalf("MUST NOT dispatch after a hosted resolve error (no Ollama fallback); recorded %d requests: %+v", len(fl.requests), fl.requests)
	}
	// Defensive: a fallback regression would surface as an empty/ollama-provider
	// dispatch — fail on it explicitly even if the count check above changes.
	for _, rq := range fl.requests {
		if rq.Provider == "" || rq.Provider == "ollama" {
			t.Fatalf("silent fallback detected: dispatched Provider=%q after a hosted resolve error", rq.Provider)
		}
	}
	// The raw error / connection identity never reaches the user.
	if strings.Contains(got.RefusalReason, connCanary) || strings.Contains(got.FinalText, connCanary) {
		t.Fatalf("connection identity leaked into the user-visible output: reason=%q body=%q", got.RefusalReason, got.FinalText)
	}
	if strings.Contains(got.RefusalReason, rawErr.Error()) || strings.Contains(got.FinalText, rawErr.Error()) {
		t.Fatalf("raw resolve error leaked verbatim into the user-visible output: reason=%q", got.RefusalReason)
	}
	if f.calls != 1 {
		t.Fatalf("resolver consulted %d times, want exactly 1", f.calls)
	}
}

// ── #3 — bare / ollama model → 089 path unchanged (resolver not consulted) ────

// TestAgent_BareAndOllamaModel_ResolverNotConsulted_ByteForByte089_Spec096 —
// GUARD (green before AND after the injection). With a HOSTED resolver
// installed, a BARE ("gemma3:4b") or "ollama/…" selection MUST NOT consult the
// resolver and MUST reach Chat with an empty Provider — the byte-for-byte spec
// 089 Ollama path. Proves the injection is gated on the hosted-kind check, not
// merely on resolver presence.
func TestAgent_BareAndOllamaModel_ResolverNotConsulted_ByteForByte089_Spec096(t *testing.T) {
	for _, tc := range []struct{ name, model string }{
		{"bare_model", "gemma3:4b"},
		{"ollama_qualified", "ollama/gemma3:4b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Programmed so that ANY wrongful consult would VISIBLY populate
			// Provider — making "resolver ignored" provable, not merely "resolver
			// returned nothing".
			f := &fakeDispatchResolver{
				t: t,
				resolved: map[string]llm.ResolvedDispatch{
					tc.model: {
						Request:     llm.ChatRequest{Model: "should-not-be-used", Provider: "anthropic", APIKey: textPtr("sk-MUST-NOT-LEAK")}, // gitleaks:allow — synthetic
						Attribution: tc.model,
					},
				},
			}
			installFakeDispatchResolver(t, f)

			const maxIter = 2
			r := newRegistry(t)
			verdict := "A grounded answer." + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
			fl := &recordingLLM{t: t, responses: []llm.Result{
				toolUse("w0", "fake_web", `{"query":"q"}`, 100),
				endTurn(verdict, 80),
			}}
			base, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			// Both the gather AND synthesis turns run the bare/ollama model.
			a := base.WithModelOverride(modelswitch.Override{GatherModel: tc.model, SynthesisModel: tc.model})
			got, err := a.Run(context.Background(), "a question")
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got.Status != StatusSuccess {
				t.Fatalf("status=%q reason=%q want success (089 path)", got.Status, got.RefusalReason)
			}
			if f.calls != 0 {
				t.Fatalf("resolver consulted %d times for a bare/ollama model, want 0 (the 089 path MUST NOT consult the resolver)", f.calls)
			}
			if len(fl.requests) == 0 {
				t.Fatalf("expected at least one recorded request")
			}
			for i, rq := range fl.requests {
				if rq.Provider != "" {
					t.Fatalf("turn %d Provider = %q, want empty (byte-for-byte 089 Ollama path)", i, rq.Provider)
				}
				if rq.APIKey != nil {
					t.Fatalf("turn %d carried an APIKey on the 089 path", i)
				}
			}
		})
	}
}
