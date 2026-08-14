// BUG-061-012 T-05 / T-06 — the Telegram bridge is the surface that decides
// who an agent invocation runs as.
//
// These two cases are a matched pair and only mean something together. T-05
// proves a mapped chat reaches Invoke carrying a real principal; T-06 proves an
// unmapped chat reaches Invoke carrying none, and that a corpus tool handed
// that same context refuses. Either alone would be satisfiable by a bridge that
// always injected, or always injected nothing.
//
// Scope note, stated plainly so the packet does not overclaim: these exercise
// telegram.AgentBridge, which is the spec 037 Scope 9 bridge and currently has
// no production caller — the live Telegram path is assistant_adapter → facade
// (see cmd/core/wiring_assistant_facade.go), and wiring the bridge into the bot
// router is scope 10 work per this file's sibling doc comment. What is proven
// here is the resolver and the injection seam, not that the shipped Telegram
// surface is principal-bound today.

package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/tools/retrieval"
	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
)

// capturingAgentRunner records the context Invoke was called with. The context
// is the whole assertion: it is what every tool in the turn will receive.
type capturingAgentRunner struct {
	gotCtx context.Context
	calls  int
}

func (r *capturingAgentRunner) Invoke(ctx context.Context, _ agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision) {
	r.calls++
	r.gotCtx = ctx
	return &agent.InvocationResult{Outcome: agent.OutcomeProviderError}, nil
}

func (r *capturingAgentRunner) KnownIntents() []string { return []string{"demo_intent"} }

// discardSender satisfies AgentSender without asserting on the reply; these
// tests are about identity, not rendering.
type discardSender struct{ sent int }

func (s *discardSender) SendMessage(context.Context, int64, string) error {
	s.sent++
	return nil
}

// stubGrantReader stands in for the auth.BearerStore read. Recorded is
// explicit because recorded-as-none and unrecorded are different answers.
type stubGrantReader struct {
	scopes   []string
	recorded bool
	err      error
}

func (g stubGrantReader) GrantsForPrincipal(context.Context, string) (auth.RecordedGrants, error) {
	if g.err != nil {
		return auth.RecordedGrants{}, g.err
	}
	return auth.RecordedGrants{TokenID: "tok-bridge-test", Scopes: g.scopes, Recorded: g.recorded}, nil
}

// retrievalEngineStub counts reads so a fail-closed claim can be proven by the
// absence of a call rather than by the shape of an error string.
type retrievalEngineStub struct{ calls int }

func (e *retrievalEngineStub) Search(context.Context, api.SearchRequest) ([]api.SearchResult, int, string, error) {
	e.calls++
	return []api.SearchResult{{ArtifactID: "A1"}}, 1, "semantic", nil
}

// bridgeUnderTest builds a bridge whose chat→user mapping is fixed and whose
// environment is production, so an unmapped chat is a hard refusal rather than
// the dev/test empty-actor ergonomic.
func bridgeUnderTest(t *testing.T, mapping map[int64]string, grants PrincipalGrantReader) (*AgentBridge, *capturingAgentRunner) {
	t.Helper()
	runner := &capturingAgentRunner{}
	bridge, err := NewAgentBridge(runner, &discardSender{})
	if err != nil {
		t.Fatalf("NewAgentBridge: %v", err)
	}
	bot := &Bot{userMapping: mapping, environment: "production"}
	resolver, err := NewBotPrincipalResolver(bot, grants)
	if err != nil {
		t.Fatalf("NewBotPrincipalResolver: %v", err)
	}
	bridge.Principal = resolver
	return bridge, runner
}

// T-05 / SCN-05. A mapped chat resolves to a principal BEFORE Invoke, so the
// session is already on the context every tool in the turn receives.
func TestTelegramBridge_MappedChatInjectsPrincipal(t *testing.T) {
	const chatID int64 = 4242
	const userID = "u-mapped"

	bridge, runner := bridgeUnderTest(t,
		map[int64]string{chatID: userID},
		stubGrantReader{scopes: []string{auth.GrantGlobalCorpusRead}, recorded: true})

	if _, err := bridge.Handle(context.Background(), chatID, "what did I save about tailscale"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}

	sess, ok := auth.SessionFromContext(runner.gotCtx)
	if !ok {
		t.Fatal("no session on the context at Invoke; a mapped chat reached the agent unidentified")
	}
	if sess.UserID != userID {
		t.Errorf("session UserID = %q, want %q", sess.UserID, userID)
	}
	if sess.Source == "" {
		t.Error("session Source is empty; auth.WithSession treats that as a no-op, so this injection would silently do nothing")
	}
	if !auth.GateGlobalCorpusRead(sess).Allowed {
		t.Errorf("mapped principal holding %q was not authorized for the corpus; the recorded grant did not survive delegation (scopes=%v)",
			auth.GrantGlobalCorpusRead, sess.Scopes)
	}
}

// T-06 / SCN-06. An unmapped chat injects nothing, and a corpus tool given that
// context refuses without reading.
func TestTelegramBridge_UnmappedChatInjectsNone(t *testing.T) {
	const mappedChat int64 = 4242
	const unmappedChat int64 = 9999

	bridge, runner := bridgeUnderTest(t,
		map[int64]string{mappedChat: "u-mapped"},
		stubGrantReader{scopes: []string{auth.GrantGlobalCorpusRead}, recorded: true})

	if _, err := bridge.Handle(context.Background(), unmappedChat, "what did I save about tailscale"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1 — the turn must still run, only unprivileged", runner.calls)
	}
	if _, ok := auth.SessionFromContext(runner.gotCtx); ok {
		t.Fatal("a session was injected for a chat with no user mapping; the bridge invented a principal")
	}

	// The refusal has to be observable at the tool, not merely inferred from
	// an absent context value.
	engine := &retrievalEngineStub{}
	retrieval.SetServices(&retrieval.Services{Engine: engine, MaxTopK: 5})
	t.Cleanup(retrieval.ResetForTest)

	tool, ok := agent.ByName(retrieval.ToolName)
	if !ok {
		t.Fatalf("agent.ByName(%q) returned !ok", retrieval.ToolName)
	}
	_, err := tool.Handler(runner.gotCtx, json.RawMessage(`{"query":"tailscale acl"}`))
	if err == nil {
		t.Fatal("retrieval succeeded on the unmapped chat's context; the corpus is readable from an unidentified Telegram chat")
	}
	if !strings.Contains(err.Error(), "retrieval_search_no_principal") {
		t.Errorf("got %v, want retrieval_search_no_principal", err)
	}
	if engine.calls != 0 {
		t.Errorf("search engine ran %d time(s) for an unidentified caller; the gate did not precede the read", engine.calls)
	}

	// Control: the SAME bridge on the MAPPED chat does reach the corpus. Without
	// this the test would also pass if the bridge never injected anything.
	if _, err := bridge.Handle(context.Background(), mappedChat, "what did I save about tailscale"); err != nil {
		t.Fatalf("Handle(mapped): %v", err)
	}
	if _, err := tool.Handler(runner.gotCtx, json.RawMessage(`{"query":"tailscale acl"}`)); err != nil {
		t.Fatalf("mapped chat was refused the corpus: %v", err)
	}
	if engine.calls != 1 {
		t.Errorf("search engine calls = %d, want 1 for the mapped chat", engine.calls)
	}
}

// A resolver failure must not be reported as a principal. This pins the branch
// that turns an unreadable grant set into a refusal rather than an empty grant,
// which would be indistinguishable from a user who legitimately holds nothing.
func TestTelegramBridge_UnreadableGrantsInjectNoPrincipal(t *testing.T) {
	const chatID int64 = 4242

	bridge, runner := bridgeUnderTest(t,
		map[int64]string{chatID: "u-mapped"},
		stubGrantReader{err: errors.New("grant store unavailable")})

	if _, err := bridge.Handle(context.Background(), chatID, "anything"); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if _, ok := auth.SessionFromContext(runner.gotCtx); ok {
		t.Fatal("a session was injected despite an unreadable grant set; a store outage would silently authorize")
	}
}
