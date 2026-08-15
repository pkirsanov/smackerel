package recipesearch

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
)

// fakeSearcher counts invocations. A fail-closed assertion needs to prove the
// search did NOT happen; an error return alone cannot distinguish "refused
// before the read" from "read, then refused".
type fakeSearcher struct {
	results []api.SearchResult
	calls   int
}

func (f *fakeSearcher) Search(_ context.Context, _ api.SearchRequest) ([]api.SearchResult, int, string, error) {
	f.calls++
	return f.results, len(f.results), "semantic", nil
}

// -------------------- BUG-061-012 SEC-01 principal + grant gate --------------------

// A context with no auth session must fail closed. The search MUST NOT run: an
// "unauthorized" error returned after the corpus was already read is not a gate.
func TestRecipeSearch_NoPrincipalFailsClosed(t *testing.T) {
	fs := &fakeSearcher{results: []api.SearchResult{{ArtifactID: "R1"}}}
	SetServices(&Services{Engine: fs, MaxTopK: 5})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	_, err := tool.Handler(context.Background(), json.RawMessage(`{"query":"chicken stir fry"}`))
	if err == nil {
		t.Fatal("recipe_search succeeded with no principal in context; the global corpus is readable by an unauthenticated caller")
	}
	if !strings.Contains(err.Error(), "recipe_search_no_principal") {
		t.Errorf("got %v, want recipe_search_no_principal", err)
	}
	if fs.calls != 0 {
		t.Errorf("engine was called %d times despite no principal; the gate must precede the search", fs.calls)
	}
}

// A real principal that lacks corpus:read is refused, and the refusal is
// distinguishable from the no-principal case. Collapsing the two would hide a
// surface that forgot to inject a session behind an ordinary permissions denial.
func TestRecipeSearch_GrantRequired(t *testing.T) {
	fs := &fakeSearcher{results: []api.SearchResult{{ArtifactID: "R1"}}}
	SetServices(&Services{Engine: fs, MaxTopK: 5})
	t.Cleanup(ResetForTest)

	// A daily user's default grant snapshot deliberately excludes corpus:read.
	ctx := auth.WithSession(context.Background(),
		auth.SessionWithRole("u-ungranted", "tok-1", auth.RoleDailyUser))

	tool, _ := agent.ByName(ToolName)
	_, err := tool.Handler(ctx, json.RawMessage(`{"query":"chicken stir fry"}`))
	if err == nil {
		t.Fatal("recipe_search succeeded for a principal without corpus:read")
	}
	if !strings.Contains(err.Error(), "recipe_search_grant_required") {
		t.Errorf("got %v, want recipe_search_grant_required", err)
	}
	if strings.Contains(err.Error(), "no_principal") {
		t.Error("grant refusal is indistinguishable from the no-principal case; the two refusals must stay distinct")
	}
	if fs.calls != 0 {
		t.Errorf("engine was called %d times despite a missing grant", fs.calls)
	}
}

// The SAME caller with the grant explicitly added proceeds past the gate and
// reaches the engine. Without this the two tests above would also pass if the
// tool refused everything unconditionally.
func TestRecipeSearch_GrantedPrincipalProceeds(t *testing.T) {
	fs := &fakeSearcher{results: []api.SearchResult{
		{ArtifactID: "R1", Title: "Chicken stir fry", Snippet: "soy, ginger, garlic"},
	}}
	SetServices(&Services{Engine: fs, MaxTopK: 5})
	t.Cleanup(ResetForTest)

	ctx := auth.WithSession(context.Background(),
		auth.SessionWithRole("u-ungranted", "tok-1", auth.RoleDailyUser, auth.GrantGlobalCorpusRead))

	tool, _ := agent.ByName(ToolName)
	out, err := tool.Handler(ctx, json.RawMessage(`{"query":"chicken stir fry"}`))
	if err != nil {
		t.Fatalf("granted caller was refused: %v", err)
	}
	if fs.calls != 1 {
		t.Fatalf("engine calls: got %d, want 1 — the granted caller did not reach the search", fs.calls)
	}
	var got recipeSearchOutput
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(got.Hits) != 1 || got.Hits[0].ArtifactID != "R1" {
		t.Errorf("hits: %+v", got.Hits)
	}
}
