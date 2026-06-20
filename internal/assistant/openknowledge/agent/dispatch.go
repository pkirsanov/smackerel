// Spec 096 SCOPE-07 — the agent-side consumer of the provider-aware dispatch
// resolver. This file is the live wiring that turns a selected hosted
// provider-qualified model id into a populated hosted ChatRequest at the /ask
// loop's dispatch chokepoints (agent.go), so a selected hosted model actually
// dispatches to its provider instead of silently taking the Ollama path.
//
// Import-direction note (why a late-bound SOURCE and not a direct
// agenttool import): the SCOPE-03 *llm.DispatchResolver is installed by cmd/core
// into a late-bound singleton in package agenttool. But agenttool ALREADY
// imports this agent package (agenttool/substrate_tool.go exposes the agent loop
// as a spec-037 substrate Tool), so the agent package importing agenttool would
// form a compile-time cycle. The dependency direction is therefore inverted: the
// agent owns a late-bound resolver SOURCE (SetDispatchResolverSource), and a tiny
// init-time bridge in agenttool (dispatch_bridge.go) pushes
// agenttool.DispatchResolverProvider into it. cmd/core's existing
// agenttool.SetDispatchResolver wiring then drives this loop with no edit to
// cmd/core, the resolver, or the sidecar.
//
// NO-DEFAULTS (G028) / NEVER-FALLBACK (FR-X1, SCN-096-G01): a hosted resolve
// failure REFUSES the turn with a typed reason; it is NEVER a silent Ollama
// fallback, and the credential / raw resolve error never reaches the user.
package agent

import (
	"errors"
	"strings"
	"sync/atomic"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/config"
)

// DispatchResolver is the narrow port the /ask loop needs to turn a selected
// provider-qualified model id into a populated hosted dispatch. It mirrors
// agenttool.DispatchResolver, and *llm.DispatchResolver structurally satisfies
// BOTH; it is DECLARED HERE so the agent package never imports agenttool (which
// would form the cycle described in the file header).
type DispatchResolver interface {
	// Resolve maps a provider-qualified model id to a populated dispatch (or a
	// typed *llm.ResolveError). A refusal is NEVER a silent Ollama fallback.
	Resolve(providerQualifiedModel string) (llm.ResolvedDispatch, error)
}

// TerminationDispatchRejected is the typed termination reason for a hosted
// provider-dispatch refusal: the selected provider-qualified model could not be
// resolved to a usable hosted connection (missing/disabled connection, missing
// credential, decrypt failure, …). The turn REFUSES — never an Ollama fallback
// (FR-X1 / SCN-096-G01). Downstream MapTerminationToRefusalCause maps any reason
// it does not name to the canonical default refusal body (its default arm), so
// the user sees a generic refusal and never the provider error or credential.
const TerminationDispatchRejected TerminationReason = "dispatch_rejected"

// dispatchSpanName is the spec 096 §13 span emitted around each HOSTED provider
// dispatch in the /ask loop. Its attrs are provider/model/turn/cost.usd ONLY —
// NEVER the credential (the §13 alarming secret-safety invariant).
const dispatchSpanName = "model.dispatch"

// dispatchTurnGather / dispatchTurnSynthesis are the closed `turn` span-attr
// vocabulary: a gather/tool round vs the spec-087 forced-final synthesis round
// (and its retries).
const (
	dispatchTurnGather    = "gather"
	dispatchTurnSynthesis = "synthesis"
)

// dispatchSourceHolder wraps the late-bound resolver accessor for atomic
// storage (a concrete element type avoids the typed-nil interface gotcha).
type dispatchSourceHolder struct{ source func() DispatchResolver }

// dispatchResolverSourceRef is the late-bound accessor yielding the
// currently-installed provider-aware dispatch resolver (or nil). In production
// the agenttool bridge installs an accessor that reads agenttool's resolver
// singleton lazily, so cmd/core's existing agenttool.SetDispatchResolver wiring
// drives the loop with NO ordering constraint. A nil ref ⇒ not wired ⇒ the
// byte-for-byte spec 089 Ollama dispatch path.
var dispatchResolverSourceRef atomic.Pointer[dispatchSourceHolder]

// SetDispatchResolverSource installs the late-bound resolver accessor. Passing
// nil clears the binding (installedDispatchResolver then returns nil). The
// agenttool bridge calls this once at init; tests install a fake accessor and
// reset to nil in cleanup so the global binding never leaks across tests.
func SetDispatchResolverSource(source func() DispatchResolver) {
	if source == nil {
		dispatchResolverSourceRef.Store(nil)
		return
	}
	dispatchResolverSourceRef.Store(&dispatchSourceHolder{source: source})
}

// installedDispatchResolver returns the currently-installed resolver (or nil
// when not wired). Read lock-free.
func installedDispatchResolver() DispatchResolver {
	h := dispatchResolverSourceRef.Load()
	if h == nil || h.source == nil {
		return nil
	}
	return h.source()
}

// isHostedProviderQualified reports whether model is a "<kind>/<backend>" id
// whose kind is a NON-ollama HOSTED provider kind (the closed SCOPE-01 set). A
// BARE model ("gemma3:4b") and an "ollama/…" id are NOT hosted — they take the
// byte-for-byte spec 089 Ollama path and never consult the resolver.
func isHostedProviderQualified(model string) bool {
	kind, _, found := strings.Cut(strings.TrimSpace(model), "/")
	if !found {
		return false
	}
	switch strings.TrimSpace(kind) {
	case config.ModelConnectionKindAnthropic,
		config.ModelConnectionKindOpenAI,
		config.ModelConnectionKindAzureFoundry,
		config.ModelConnectionKindGoogle,
		config.ModelConnectionKindBedrock:
		return true
	default:
		// ollama/… or any non-hosted kind ⇒ the spec 089 Ollama path.
		return false
	}
}

// applyProviderDispatch is the SCOPE-07 injection. For a HOSTED
// provider-qualified selection AND a wired resolver it returns a ChatRequest
// carrying the per-request Provider/APIBase/APIKey/ProviderParams plus the BARE
// backend Model (the sidecar recomposes "<kind>/<backend>" from Provider+Model),
// so the turn dispatches to the hosted provider. For a bare/ollama model OR a
// nil resolver it returns req UNCHANGED — the byte-for-byte spec 089 Ollama path
// (the single most important invariant; the bare/ollama short-circuit runs first
// so the local path never even reads the resolver singleton — NFR-2). A hosted
// resolve FAILURE returns a non-empty, secret-free refusal reason; the caller
// MUST refuse the turn and MUST NOT fall back to Ollama (FR-X1 / SCN-096-G01).
func applyProviderDispatch(req llm.ChatRequest) (out llm.ChatRequest, refusal string, reason llm.RejectReason) {
	if !isHostedProviderQualified(req.Model) {
		return req, "", "" // bare/ollama ⇒ byte-for-byte 089 path; resolver never touched.
	}
	resolver := installedDispatchResolver()
	if resolver == nil {
		return req, "", "" // hosted id but no resolver wired ⇒ deferred-activation 089 fallback.
	}
	resolved, err := resolver.Resolve(req.Model)
	if err != nil {
		// NEVER an Ollama fallback. Surface only the typed, secret-free reject
		// category — never the wrapped vault cause, the connection identity, or
		// the credential. The typed reason is ALSO returned for the spec 096 §13
		// vault-decrypt-failure metric (the recorder's closed allow-set drops any
		// non-vault reason).
		return req, dispatchRefusalReason(err), resolveErrReason(err)
	}
	req.Provider = resolved.Request.Provider
	req.APIBase = resolved.Request.APIBase
	req.APIKey = resolved.Request.APIKey
	req.ProviderParams = resolved.Request.ProviderParams
	req.Model = resolved.Request.Model // BARE backend id for the wire/sidecar.
	return req, "", ""
}

// dispatchRefusalReason renders a non-secret refusal string from a resolve
// error. Only the typed *llm.ResolveError.Reason category (e.g.
// "credential_missing", "decrypt_failed") is included — never the raw error
// string, the connection identity, or any credential material.
func dispatchRefusalReason(err error) string {
	const base = "selected model provider connection is unavailable"
	var re *llm.ResolveError
	if errors.As(err, &re) {
		return base + ": " + string(re.Reason)
	}
	return base
}

// resolveErrReason extracts the typed, secret-free RejectReason from a resolve
// error for the spec 096 §13 openknowledge_vault_decrypt_failures_total metric.
// A non-*ResolveError (no typed reason) yields the empty reason — the recorder's
// closed allow-set then drops the increment (G021). The reason token NEVER
// carries credential material (it is a fixed enum: decrypt_failed, …).
func resolveErrReason(err error) llm.RejectReason {
	var re *llm.ResolveError
	if errors.As(err, &re) {
		return re.Reason
	}
	return ""
}
