// Spec 096 SCOPE-07 — the dispatch-resolver bridge.
//
// The open-knowledge /ask loop (package openknowledge/agent) consumes the
// provider-aware dispatch resolver, but the resolver is installed by cmd/core
// into THIS package's late-bound singleton (model_catalog_source.go:
// SetDispatchResolver / DispatchResolverProvider). The agent package cannot
// import agenttool to read it — agenttool already imports agent
// (substrate_tool.go), so the reverse import would form a compile-time cycle.
//
// This bridge inverts the dependency: at init it pushes a lazy accessor into the
// agent's late-bound resolver SOURCE (okagent.SetDispatchResolverSource). The
// accessor reads agenttool's CURRENT singleton on every call, so cmd/core's
// post-init agenttool.SetDispatchResolver wiring is observed with NO ordering
// constraint, and a not-yet-wired (nil) singleton ⇒ nil ⇒ the byte-for-byte
// spec 089 Ollama dispatch path. No edit to cmd/core, the resolver, or the
// sidecar is required.
package agenttool

import okagent "github.com/smackerel/smackerel/internal/assistant/openknowledge/agent"

func init() {
	okagent.SetDispatchResolverSource(func() okagent.DispatchResolver {
		r := DispatchResolverProvider()
		if r == nil {
			// Typed-nil guard: never hand the agent a non-nil interface that
			// wraps a nil resolver. A nil singleton ⇒ untyped nil ⇒ 089 path.
			return nil
		}
		// agenttool.DispatchResolver and okagent.DispatchResolver are
		// structurally identical (both: Resolve(string) (llm.ResolvedDispatch,
		// error)); the installed *llm.DispatchResolver satisfies both.
		return r
	})
}
