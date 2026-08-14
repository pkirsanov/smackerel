package retrieval

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
)

// fakeSearcher records the last Search request so tests can assert on
// the cap behavior and on plumbing.
type fakeSearcher struct {
	lastReq api.SearchRequest
	results []api.SearchResult
	mode    string
	err     error
	// calls counts invocations. A fail-closed assertion needs to prove the
	// search did NOT happen; lastReq alone cannot distinguish "never called"
	// from "called with a zero request".
	calls int
}

func (f *fakeSearcher) Search(_ context.Context, req api.SearchRequest) ([]api.SearchResult, int, string, error) {
	f.calls++
	f.lastReq = req
	if f.err != nil {
		return nil, 0, "", f.err
	}
	return f.results, len(f.results), f.mode, nil
}

// grantedCtx returns a context carrying a session for userID that holds
// corpus:read. Most tests in this file exercise behaviour downstream of the
// BUG-061-012 principal gate, so they need a caller that clears it.
func grantedCtx(userID string) context.Context {
	return auth.WithSession(context.Background(),
		auth.SessionWithRole(userID, "tok-"+userID, auth.RoleOperator))
}

func TestRetrievalSearch_Registered(t *testing.T) {
	// init() must have populated the registry; the tool must exist.
	if !agent.Has(ToolName) {
		t.Fatalf("expected %q to be registered after init()", ToolName)
	}
	tool, ok := agent.ByName(ToolName)
	if !ok {
		t.Fatalf("ByName(%q) returned !ok", ToolName)
	}
	if tool.SideEffectClass != agent.SideEffectRead {
		t.Errorf("side_effect_class: got %q, want read", tool.SideEffectClass)
	}
	if tool.OwningPackage != "internal/agent/tools/retrieval" {
		t.Errorf("owning_package: got %q", tool.OwningPackage)
	}
	if tool.Handler == nil {
		t.Fatal("handler is nil")
	}
}

func TestRetrievalSearch_NotConfigured(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	args := json.RawMessage(`{"query":"x"}`)
	_, err := tool.Handler(grantedCtx("u"), args)
	if err == nil {
		t.Fatal("expected error when services are not wired")
	}
	if !strings.Contains(err.Error(), "retrieval_tools_not_configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRetrievalSearch_HappyPath(t *testing.T) {
	fs := &fakeSearcher{
		results: []api.SearchResult{
			{ArtifactID: "A1", Title: "Tailscale notes", Snippet: "ACL tag …", CreatedAt: "2026-05-01T12:00:00Z"},
			{ArtifactID: "A2", Title: "ACL tags primer", Snippet: "tag:bridge …", CreatedAt: "2026-05-02T12:00:00Z"},
		},
		mode: "semantic",
	}
	SetServices(&Services{Engine: fs, MaxTopK: 5})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	args := json.RawMessage(`{"query":"tailscale acl"}`)
	out, err := tool.Handler(grantedCtx("u1"), args)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	var got retrievalOutput
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(got.Hits) != 2 {
		t.Fatalf("hits: got %d, want 2", len(got.Hits))
	}
	if got.Hits[0].ArtifactID != "A1" || got.Hits[1].ArtifactID != "A2" {
		t.Errorf("hits: %+v", got.Hits)
	}
}

func TestRetrievalSearch_TopKCap(t *testing.T) {
	fs := &fakeSearcher{}
	SetServices(&Services{Engine: fs, MaxTopK: 4})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	// Request top_k=20; the SST cap is 4 so the engine MUST see Limit=4.
	args := json.RawMessage(`{"query":"x","top_k":20}`)
	if _, err := tool.Handler(grantedCtx("u"), args); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if fs.lastReq.Limit != 4 {
		t.Errorf("expected engine limit clamped to MaxTopK=4, got %d", fs.lastReq.Limit)
	}
}

func TestRetrievalSearch_TopKZeroUsesCap(t *testing.T) {
	fs := &fakeSearcher{}
	SetServices(&Services{Engine: fs, MaxTopK: 7})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	args := json.RawMessage(`{"query":"x"}`)
	if _, err := tool.Handler(grantedCtx("u"), args); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if fs.lastReq.Limit != 7 {
		t.Errorf("expected engine limit defaulted to MaxTopK=7, got %d", fs.lastReq.Limit)
	}
}

func TestRetrievalSearch_BadInput(t *testing.T) {
	SetServices(&Services{Engine: &fakeSearcher{}, MaxTopK: 5})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	cases := []struct {
		name string
		args string
		want string
	}{
		{"empty body", `{}`, "empty_query"},
		{"empty query", `{"query":""}`, "empty_query"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tool.Handler(grantedCtx("u"), json.RawMessage(tc.args))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("got %v, want substring %q", err, tc.want)
			}
		})
	}
}

// BUG-061-012. The tool used to require a `user_id` argument, validate it, and
// then never use it — `api.SearchRequest` has no user field and the corpus is a
// single global one. The argument is gone. This asserts the removal is real
// rather than cosmetic: a caller that still sends it must be rejected, which is
// what stops the old shape from quietly continuing to work.
func TestRetrievalSearch_RejectsCallerSuppliedIdentity(t *testing.T) {
	SetServices(&Services{Engine: &fakeSearcher{}, MaxTopK: 5})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	schema, err := agent.CompileSchema(tool.InputSchema)
	if err != nil {
		t.Fatalf("CompileSchema: %v", err)
	}
	if err := schema.ValidateBytes(json.RawMessage(`{"query":"x","user_id":"someone-else"}`)); err == nil {
		t.Fatal("input schema accepted a user_id argument; additionalProperties:false must reject it, or the model can still name the principal")
	}
	// Positive control: the same schema must still accept a well-formed call,
	// or the assertion above would pass for the wrong reason.
	if err := schema.ValidateBytes(json.RawMessage(`{"query":"x"}`)); err != nil {
		t.Fatalf("input schema rejected a valid call: %v", err)
	}
}

func TestRetrievalSearch_EngineError(t *testing.T) {
	SetServices(&Services{Engine: &fakeSearcher{err: errors.New("kaboom")}, MaxTopK: 5})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	_, err := tool.Handler(grantedCtx("u"), json.RawMessage(`{"query":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "retrieval_search_engine_error") {
		t.Errorf("got %v, want retrieval_search_engine_error", err)
	}
}

func TestRetrievalSearch_OutputSchemaCompiles(t *testing.T) {
	in, out, ok := agent.SchemasFor(ToolName)
	if !ok {
		t.Fatal("schemas missing for retrieval_search")
	}
	if in == nil || out == nil {
		t.Fatal("nil compiled schema")
	}
	// Adversarial: valid sample passes; missing required field fails.
	if err := out.ValidateBytes(json.RawMessage(`{"hits":[]}`)); err != nil {
		t.Errorf("valid sample rejected: %v", err)
	}
	if err := out.ValidateBytes(json.RawMessage(`{}`)); err == nil {
		t.Error("expected required-field violation for {}")
	}
}

// -------------------- BUG-061-012 principal + grant gate --------------------

// T-02 / SCN-02. A context with no auth session must fail closed. The search
// MUST NOT run: an "unauthorized" error returned after the corpus was already
// read is not a gate.
func TestRetrieval_NoPrincipalFailsClosed(t *testing.T) {
	fs := &fakeSearcher{results: []api.SearchResult{{ArtifactID: "A1"}}}
	SetServices(&Services{Engine: fs, MaxTopK: 5})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	_, err := tool.Handler(context.Background(), json.RawMessage(`{"query":"tailscale acl"}`))
	if err == nil {
		t.Fatal("retrieval succeeded with no principal in context; the global corpus is readable by an unauthenticated caller")
	}
	if !strings.Contains(err.Error(), "retrieval_search_no_principal") {
		t.Errorf("got %v, want retrieval_search_no_principal", err)
	}
	if fs.calls != 0 {
		t.Errorf("engine was called %d times despite no principal; the gate must precede the search", fs.calls)
	}
}

// T-03 / SCN-03. A real principal that lacks corpus:read is refused, and the
// refusal is distinguishable from the no-principal case. Collapsing the two
// would hide a surface that forgot to inject a session behind what reads as an
// ordinary permissions denial.
func TestRetrieval_GrantRequired(t *testing.T) {
	fs := &fakeSearcher{results: []api.SearchResult{{ArtifactID: "A1"}}}
	SetServices(&Services{Engine: fs, MaxTopK: 5})
	t.Cleanup(ResetForTest)

	// A daily user's default grant snapshot deliberately excludes corpus:read.
	ctx := auth.WithSession(context.Background(),
		auth.SessionWithRole("u-ungranted", "tok-1", auth.RoleDailyUser))

	tool, _ := agent.ByName(ToolName)
	_, err := tool.Handler(ctx, json.RawMessage(`{"query":"tailscale acl"}`))
	if err == nil {
		t.Fatal("retrieval succeeded for a principal without corpus:read")
	}
	if !strings.Contains(err.Error(), "retrieval_search_grant_required") {
		t.Errorf("got %v, want retrieval_search_grant_required", err)
	}
	if strings.Contains(err.Error(), "no_principal") {
		t.Error("grant refusal is indistinguishable from the no-principal case; SCN-03 requires two distinct errors")
	}
	if fs.calls != 0 {
		t.Errorf("engine was called %d times despite a missing grant", fs.calls)
	}

	// Adversarial control: the SAME caller with the grant explicitly added
	// must pass. Without this, the test would also pass if the tool refused
	// everything unconditionally.
	granted := auth.WithSession(context.Background(),
		auth.SessionWithRole("u-ungranted", "tok-1", auth.RoleDailyUser, auth.GrantGlobalCorpusRead))
	if _, err := tool.Handler(granted, json.RawMessage(`{"query":"tailscale acl"}`)); err != nil {
		t.Fatalf("granted caller was refused: %v", err)
	}
}

// T-04 / SCN-04. The read happens under the session's principal, and no tool
// argument can change that.
//
// Scope note, stated exactly: the corpus is ONE operator-owned global store
// (auth.GateGlobalCorpusRead asserts no row partition, and api.SearchRequest
// has no user field). So "scoped to U" here means the read is authorized by
// U's grant snapshot and by nothing else — NOT that rows are filtered to U.
// Asserting a per-user row filter would claim isolation the system does not
// implement, which is the same class of misrepresentation this bug reports.
func TestRetrieval_ScopesToAuthenticatedPrincipal(t *testing.T) {
	fs := &fakeSearcher{results: []api.SearchResult{{ArtifactID: "A1"}}, mode: "semantic"}
	SetServices(&Services{Engine: fs, MaxTopK: 5})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)

	// The authorizing principal is the one in the context.
	ctx := grantedCtx("u-authenticated")
	if _, err := tool.Handler(ctx, json.RawMessage(`{"query":"tailscale acl"}`)); err != nil {
		t.Fatalf("granted principal was refused: %v", err)
	}
	if fs.calls != 1 {
		t.Fatalf("engine calls: got %d, want 1", fs.calls)
	}
	sess, ok := auth.SessionFromContext(ctx)
	if !ok || sess.UserID != "u-authenticated" {
		t.Fatalf("session did not survive into the handler context: %+v ok=%v", sess, ok)
	}

	// No argument could have changed it: the schema refuses every spelling of
	// a caller identity, so the model cannot name a principal at all.
	schema, err := agent.CompileSchema(tool.InputSchema)
	if err != nil {
		t.Fatalf("CompileSchema: %v", err)
	}
	for _, spelling := range []string{"user_id", "userId", "user", "principal", "actor", "on_behalf_of"} {
		args := json.RawMessage(`{"query":"x","` + spelling + `":"someone-else"}`)
		if err := schema.ValidateBytes(args); err == nil {
			t.Errorf("input schema accepted %q; the model can still name the principal", spelling)
		}
	}

	// Ungranted caller on the identical query gets nothing — proving the
	// authorization turned on the principal and not on the arguments.
	before := fs.calls
	ungranted := auth.WithSession(context.Background(),
		auth.SessionWithRole("u-other", "tok-2", auth.RoleDailyUser))
	if _, err := tool.Handler(ungranted, json.RawMessage(`{"query":"tailscale acl"}`)); err == nil {
		t.Error("an ungranted principal read the corpus with the same arguments")
	}
	if fs.calls != before {
		t.Errorf("engine ran for the ungranted principal: calls %d -> %d", before, fs.calls)
	}
}
