//go:build integration

// BUG-061-012 T-08 / SCN-04 — an HTTP-derived session reaches the retrieval
// tool unchanged.
//
// The unit tests for R2 construct an auth.Session directly, which proves the
// tool's behaviour but assumes the session it will be handed in production. This
// case removes that assumption: it mints a real PASETO, verifies it through the
// same auth.VerifyAndParse step bearerAuthMiddleware performs, builds the
// Session from the parsed claims, and only then invokes the REGISTERED tool. If
// the identity or the grant claim failed to survive issuance → verify → Session
// → context, an authorized caller would be refused or, worse, an unauthorized
// one admitted, and neither would be visible from the unit layer.
//
// Integration-tagged because it exercises the production agent registry
// (init()-registered handler, compiled input schema) together with the real
// token path. It needs no database: the assertion is about what the token
// carries into the tool, and the server-side recorded-grant half is already
// covered by tests/integration/corpus_grant_roundtrip_test.go.

package agent_integration

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/tools/retrieval"
	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
)

// principalSearcher records who authorized each read and how many reads
// happened. The count is what lets a fail-closed claim be proven by the absence
// of a read rather than by the text of an error.
type principalSearcher struct {
	calls int
}

func (s *principalSearcher) Search(context.Context, api.SearchRequest) ([]api.SearchResult, int, string, error) {
	s.calls++
	return []api.SearchResult{{ArtifactID: "A1", Title: "Tailscale notes"}}, 1, "semantic", nil
}

// httpSessionFor mints a token carrying scopes and returns the Session the
// bearer middleware would derive from it.
func httpSessionFor(t *testing.T, userID string, scopes []string) auth.Session {
	t.Helper()

	priv, pub := auth.GenerateSigningKeypair()
	const kid = "bug061012-t08-kid"

	tokenID, err := auth.GenerateTokenID()
	if err != nil {
		t.Fatalf("GenerateTokenID: %v", err)
	}
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     userID,
		TokenID:    tokenID,
		SigningKey: priv,
		KeyID:      kid,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
		Scopes:     scopes,
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	parsed, err := auth.VerifyAndParse(issued.WireToken, auth.VerifyOptions{
		ActivePublicKey: pub,
		ActiveKeyID:     kid,
		Issuer:          "smackerel",
		Now:             time.Now,
	})
	if err != nil {
		t.Fatalf("VerifyAndParse: %v", err)
	}

	return auth.Session{
		UserID:  parsed.UserID,
		TokenID: parsed.TokenID,
		Scopes:  parsed.Scopes,
		Source:  auth.SessionSourcePerUserToken,
	}
}

func TestRetrieval_EndToEndUnderHTTPSession(t *testing.T) {
	engine := &principalSearcher{}
	retrieval.SetServices(&retrieval.Services{Engine: engine, MaxTopK: 5})
	t.Cleanup(retrieval.ResetForTest)

	tool, ok := agent.ByName(retrieval.ToolName)
	if !ok {
		t.Fatalf("agent.ByName(%q) returned !ok; init() registration regressed", retrieval.ToolName)
	}

	const grantedUser = "t08-granted"
	const ungrantedUser = "t08-ungranted"
	args := json.RawMessage(`{"query":"tailscale acl"}`)

	t.Run("granted_http_session_reaches_the_tool_intact", func(t *testing.T) {
		sess := httpSessionFor(t, grantedUser, []string{auth.GrantGlobalCorpusRead})
		if sess.UserID != grantedUser {
			t.Fatalf("UserID = %q after issuance → verify, want %q; the identity did not survive the token round trip", sess.UserID, grantedUser)
		}
		if !slices.Contains(sess.Scopes, auth.GrantGlobalCorpusRead) {
			t.Fatalf("scopes = %v after the round trip, want to contain %q; a granted caller would be refused", sess.Scopes, auth.GrantGlobalCorpusRead)
		}

		ctx := auth.WithSession(context.Background(), sess)
		if _, err := tool.Handler(ctx, args); err != nil {
			t.Fatalf("a principal holding %q was refused: %v", auth.GrantGlobalCorpusRead, err)
		}
		if engine.calls != 1 {
			t.Fatalf("engine calls = %d, want 1", engine.calls)
		}

		got, ok := auth.SessionFromContext(ctx)
		if !ok || got.UserID != grantedUser {
			t.Fatalf("session in the handler context = %+v ok=%v, want UserID %q", got, ok, grantedUser)
		}
	})

	// Adversarial half. A token verified through the identical path but
	// carrying no corpus grant must be refused, and refused BEFORE the read.
	// Without this the granted assertion above would still pass against a tool
	// that authorized everything.
	t.Run("ungranted_http_session_is_refused_before_the_read", func(t *testing.T) {
		before := engine.calls
		sess := httpSessionFor(t, ungrantedUser, []string{})
		ctx := auth.WithSession(context.Background(), sess)

		_, err := tool.Handler(ctx, args)
		if err == nil {
			t.Fatal("a token carrying no corpus grant read the corpus")
		}
		if !strings.Contains(err.Error(), "retrieval_search_grant_required") {
			t.Errorf("got %v, want retrieval_search_grant_required", err)
		}
		if engine.calls != before {
			t.Errorf("engine ran for an ungranted principal: calls %d -> %d", before, engine.calls)
		}
	})

	// And the third state: no session at all is a distinct, louder failure than
	// a denied one, because it means a surface forgot to inject rather than that
	// the gate worked.
	t.Run("absent_session_is_a_distinct_refusal", func(t *testing.T) {
		before := engine.calls
		_, err := tool.Handler(context.Background(), args)
		if err == nil {
			t.Fatal("retrieval succeeded with no session on the context")
		}
		if !strings.Contains(err.Error(), "retrieval_search_no_principal") {
			t.Errorf("got %v, want retrieval_search_no_principal", err)
		}
		if strings.Contains(err.Error(), "grant_required") {
			t.Error("the absent-principal case is reported as a grant denial; a surface that forgot to inject would look like ordinary authorization")
		}
		if engine.calls != before {
			t.Errorf("engine ran with no principal: calls %d -> %d", before, engine.calls)
		}
	})
}
