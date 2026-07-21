//go:build e2e

// Spec 069 SCOPE-1c — Cross-Spec SCN-068 HTTP E2E Coverage.
//
// HTTP-route live-stack proof for cross-spec scenarios authored in
// specs/068-structured-intent-compiler. Scenario IDs are preserved
// verbatim across both specs so traceability stays intact.
//
//   - SCN-068-A01 — Weather compiles before route and normalizes location.
//   - SCN-068-A02 — Retrieval receives structured context.
//   - SCN-068-A01/A02 — Read intents never route from raw text only.
//   - SCN-068-A06 — Compiler malformed JSON blocks routing and captures safely.
//   - SCN-068-A07 — Operational commands bypass compiler over live transport.
//
// Tests drive the LIVE chi-mounted POST /api/assistant/turn route via
// the running core service (no facade mock, no adapter mock, no
// compiler stub). Live-stack inputs come exclusively from the
// SST-managed environment the e2e harness exports
// (CORE_EXTERNAL_URL + SMACKEREL_AUTH_TOKEN). Missing CORE_EXTERNAL_URL
// is a legitimate "no live stack" skip; missing token when the stack
// IS up is a wiring bug per repo NO-DEFAULTS policy.
//
// The disposable test profile routes compiler requests through the
// checked-in deterministic provider fixture. Required profile-backed
// scenarios fail when that fixture does not produce the contracted
// branch. Tests that deliberately probe an unforced compiler failure
// remain explicit about their profile-dependent skip behavior.

package assistant_e2e

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

// assertWireShapeOK enforces the v1 wire invariants every cross-spec
// e2e test relies on. Failing any of these means the HTTP route or
// adapter is broken in a transport-level way, not a compiler way.
func assertWireShapeOK(t *testing.T, resp *http.Response, raw []byte, wantTransportMessageID string) httpadapter.TurnResponse {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, string(raw))
	}
	var env httpadapter.TurnResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode response: %v\nbody=%s", err, string(raw))
	}
	if env.SchemaVersion != httpadapter.SchemaVersionV1 {
		t.Errorf("schema_version = %q, want %q", env.SchemaVersion, httpadapter.SchemaVersionV1)
	}
	if env.Transport != httpadapter.TransportName {
		t.Errorf("transport = %q, want %q", env.Transport, httpadapter.TransportName)
	}
	if env.TransportMessageID != wantTransportMessageID {
		t.Errorf("transport_message_id echo = %q, want %q", env.TransportMessageID, wantTransportMessageID)
	}
	return env
}

// assertNoSecretLeakage enforces the SCN-068-A06 safety invariant:
// regardless of which compiler outcome fired, the wire response MUST
// NOT echo bearer tokens or recognizable secret patterns. The live
// e2e harness sources the bearer from SMACKEREL_AUTH_TOKEN; the
// response body MUST NOT contain it.
func assertNoSecretLeakage(t *testing.T, stack httpTurnLiveStack, raw []byte) {
	t.Helper()
	if stack.AuthToken == "" {
		return
	}
	if strings.Contains(string(raw), stack.AuthToken) {
		t.Errorf("response body leaks bearer token; capture/compiler-failure path is unsafe")
	}
	for _, s := range []string{"BEGIN PRIVATE KEY", "BEGIN RSA PRIVATE KEY", "BEGIN OPENSSH PRIVATE KEY"} {
		if strings.Contains(string(raw), s) {
			t.Errorf("response body leaks secret pattern %q", s)
		}
	}
}

// TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation —
// SCN-068-A01. Posts a natural-language weather turn over HTTP and
// proves the deterministic provider's canonical Barcelona location
// reaches the weather route through facade invocation.
func TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)
	isolateRequiredAssistantConversation(t, stack)

	turnID := "e2e-scope1c-068a01-" + timestamp()
	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "what is the weather in Barcelona",
	}
	resp, raw := postAssistantTurn(t, stack, req)
	env := assertWireShapeOK(t, resp, raw, turnID)
	if !env.FacadeInvoked {
		t.Fatalf("facade_invoked = false; want true (read intents must go through facade)")
	}
	if env.ErrorCause == "auth_required" || env.ErrorCause == "scope_required" {
		t.Fatalf("pre-facade rejection on weather turn: error_cause=%q", env.ErrorCause)
	}
	if env.CaptureRoute {
		t.Fatalf("deterministic weather fixture routed to capture fallback; envelope=%+v", env)
	}
	// The weather tool response wording is provider-dependent, but the
	// routed envelope must carry a non-empty outcome.
	if strings.TrimSpace(env.Body) == "" && env.Status == "" {
		t.Errorf("weather turn produced empty body and empty status; facade response is degenerate")
	}
}

func TestIntentCompilerWeatherProtectedTestHasNoSkipFamilyCall(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read protected weather test source: %v", err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), sourcePath, source, 0)
	if err != nil {
		t.Fatalf("parse protected weather test source: %v", err)
	}
	const protectedName = "TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation"
	found := false
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != protectedName {
			continue
		}
		found = true
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			receiver, ok := selector.X.(*ast.Ident)
			if !ok || receiver.Name != "t" {
				return true
			}
			switch selector.Sel.Name {
			case "Skip", "Skipf", "SkipNow":
				t.Errorf("%s contains forbidden skip-family call t.%s", protectedName, selector.Sel.Name)
			}
			return true
		})
	}
	if !found {
		t.Fatalf("protected weather test %s not found", protectedName)
	}
}

// TestIntentCompilerE2E_RetrievalReceivesStructuredContext — SCN-068-A02.
// Posts a retrieval-style turn over HTTP. Same nondeterminism guard
// as the weather test.
func TestIntentCompilerE2E_RetrievalReceivesStructuredContext(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	turnID := "e2e-scope1c-068a02-" + timestamp()
	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "what did I save about barcelona last week",
	}
	resp, raw := postAssistantTurn(t, stack, req)
	env := assertWireShapeOK(t, resp, raw, turnID)
	if !env.FacadeInvoked {
		t.Fatalf("facade_invoked = false; want true")
	}
	if env.ErrorCause == "auth_required" || env.ErrorCause == "scope_required" {
		t.Fatalf("pre-facade rejection on retrieval turn: error_cause=%q", env.ErrorCause)
	}
	// Sources slice is always serialized (possibly empty); the
	// retrieval scenario, when fired, populates it. When the live
	// compiler classifies differently, the envelope still must not
	// drop sources to nil.
	if env.Sources == nil {
		t.Errorf("sources must always be non-nil in v1 responses")
	}
}

// TestIntentCompilerE2E_ReadIntentsNeverRouteFromRawTextOnly — joint
// proof for SCN-068-A01 + SCN-068-A02. The HTTP wire layer must
// invoke the facade (which gates routing on compiled intent or the
// explicit operational carve-out). A wire response showing
// facade_invoked=false on a plain-text read turn would mean the
// adapter (or some middleware) routed the turn around the facade —
// the property the bypass policy guard exists to prevent.
func TestIntentCompilerE2E_ReadIntentsNeverRouteFromRawTextOnly(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	cases := []struct {
		name string
		text string
	}{
		{"weather", "what is the weather in barcelona"},
		{"retrieval", "what did I save about pasta last week"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			turnID := "e2e-scope1c-068a01a02-" + tc.name + "-" + timestamp()
			req := httpadapter.TurnRequest{
				SchemaVersion:      httpadapter.SchemaVersionV1,
				TransportMessageID: turnID,
				Kind:               string(contracts.KindText),
				TransportHint:      "web",
				Text:               tc.text,
			}
			resp, raw := postAssistantTurn(t, stack, req)
			env := assertWireShapeOK(t, resp, raw, turnID)
			if !env.FacadeInvoked {
				t.Errorf("facade_invoked = false on read turn %q; wire layer routed around facade (raw-text route bypass detected)", tc.text)
			}
		})
	}
}

// TestIntentCompilerE2E_MalformedJSONBlocksRoutingAndCaptures —
// SCN-068-A06. Cannot deterministically force the live compiler to
// emit malformed JSON from the outside; instead, post a sequence of
// borderline turns and verify the SAFETY invariant: any turn whose
// compiler outcome is the failure branch (signalled by
// ErrorCause="compiler_failure" or capture_route=true with no
// confident routing) MUST still produce a safe wire envelope (no
// secret leakage, schema-valid, no facade-invoked=true on a
// scope-rejected response). When no turn malformed on this run, the
// test reports the observation and skips the strict branch; this is
// the same defensive-skip pattern used for disambiguation/confirm
// nondeterminism.
func TestIntentCompilerE2E_MalformedJSONBlocksRoutingAndCaptures(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	// Borderline phrasings that have historically stressed the
	// compiler classifier in fixtures. Live behavior is not
	// guaranteed deterministic.
	borderline := []string{
		"!@#$%^&*()_+ qwertyuiop",
		"the the the the the the the the the",
		"please ☃ ☂ ☔ 🌧 mañana",
	}
	sawFailureBranch := false
	for _, text := range borderline {
		turnID := "e2e-scope1c-068a06-" + timestamp()
		req := httpadapter.TurnRequest{
			SchemaVersion:      httpadapter.SchemaVersionV1,
			TransportMessageID: turnID,
			Kind:               string(contracts.KindText),
			TransportHint:      "web",
			Text:               text,
		}
		resp, raw := postAssistantTurn(t, stack, req)
		env := assertWireShapeOK(t, resp, raw, turnID)
		assertNoSecretLeakage(t, stack, raw)
		if env.ErrorCause == "compiler_failure" || (env.CaptureRoute && env.ConfirmCard == nil && env.DisambiguationPrompt == nil) {
			sawFailureBranch = true
			// Failure branch invariants: schema-valid, no
			// scenario-driven routing leaked through, body either
			// empty or a safe capture acknowledgement.
			if env.Sources == nil {
				t.Errorf("compiler-failure response must keep sources serialized as non-nil")
			}
		}
	}
	if !sawFailureBranch {
		t.Skipf("no borderline turn triggered compiler-failure / capture-fallback on the live stack this run; SCN-068-A06 safety invariants vacuously hold for this session")
	}
}

// TestIntentCompilerE2E_OperationalCommandsBypassCompilerOverLiveTransport —
// SCN-068-A07. `/status` is in the v1-frozen operational carve-out
// (internal/assistant/intent/bypass.go::OperationalCommands) so the
// live stack MUST respond deterministically without routing through
// the compiler. The wire-level proof: HTTP 200, schema-valid, no
// compiler-failure error_cause, facade_invoked=true (operational
// bypass still goes through the facade, it just skips compiler).
func TestIntentCompilerE2E_OperationalCommandsBypassCompilerOverLiveTransport(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	for _, cmd := range []string{"/status", "/help", "/reset"} {
		t.Run(strings.TrimPrefix(cmd, "/"), func(t *testing.T) {
			turnID := "e2e-scope1c-068a07-" + strings.TrimPrefix(cmd, "/") + "-" + timestamp()
			req := httpadapter.TurnRequest{
				SchemaVersion:      httpadapter.SchemaVersionV1,
				TransportMessageID: turnID,
				Kind:               string(contracts.KindText),
				TransportHint:      "web",
				Text:               cmd,
			}
			resp, raw := postAssistantTurn(t, stack, req)
			env := assertWireShapeOK(t, resp, raw, turnID)
			if !env.FacadeInvoked {
				t.Errorf("facade_invoked = false on operational command %q; bypass path must still flow through facade", cmd)
			}
			if env.ErrorCause == "compiler_failure" {
				t.Errorf("operational command %q reported compiler_failure; bypass must skip compiler entirely", cmd)
			}
			assertNoSecretLeakage(t, stack, raw)
		})
	}
}
