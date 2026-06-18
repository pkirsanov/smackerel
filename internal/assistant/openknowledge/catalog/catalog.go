// Package catalog — Spec 096 SCOPE-04: model discovery + unified catalog +
// identifier canonicalization.
//
// CatalogAggregator aggregates every effective-enabled connection's models
// into ONE provider-qualified ModelCatalog (design §6.3): the local Ollama
// daemon's installed models (live `GET /api/tags`) plus each hosted
// connection's SST-curated model list. The catalog is the admissible set
// INJECTED into the EXISTING spec-088/089 modelswitch validator + modelpref
// store (one validator, one store — no second picker), so a provider-qualified
// id round-trips through string-keyed storage and string-equality validation
// without either learning provider semantics.
//
// Two non-negotiables this package preserves:
//
//   - LEAF PURITY (088 invariant). The modelswitch package stays a pure
//     stdlib-only leaf: this package IMPORTS modelswitch (catalog is not the
//     leaf) and feeds it the catalog as its admissible set; modelswitch never
//     imports catalog. Canonicalization happens HERE, at the resolver boundary,
//     on the validator's inputs — never inside the leaf.
//
//   - GRACEFUL DEGRADATION (NFR-1). A slow / unreachable / auth-failed provider
//     degrades gracefully — its models are absent from the catalog — but it
//     ALWAYS emits a typed ProviderDiscoveryStatus. A provider is NEVER
//     silently dropped, and the reachable subset is ALWAYS served. Discovery is
//     bounded by the SST `cache_ttl_ms` + `per_provider_timeout_ms` (SCOPE-01,
//     fail-loud `> 0`; no hardcoded default lives here — G028).
package catalog

import "strings"

// kindOllama is the local-inference provider kind. Mirrors
// config.ModelConnectionKindOllama; duplicated as an untyped local so the
// catalog's pure type file carries no import.
const kindOllama = "ollama"

// ModelCapabilities is the per-model capability triplet surfaced to the
// selection surfaces (SCOPE-07). Carried verbatim from the SST registry for
// hosted (curated) connections; for live-discovered Ollama models it is the
// operator-supplied capability hint (the `/api/tags` payload does not report
// capabilities), defaulting to the zero triplet when no hint is given.
type ModelCapabilities struct {
	ToolCapable   bool
	Vision        bool
	ContextWindow int
}

// ModelDescriptor is one provider-offered model in the unified catalog. ID is
// provider-qualified (`<kind>/<backend-id>`), so it round-trips through the
// existing string-keyed modelpref.Store and the string-equality
// modelswitch.Allowlist unchanged (design §8 grammar).
type ModelDescriptor struct {
	ID            string // provider-qualified, e.g. "anthropic/claude-3-5-sonnet" or "ollama/gemma3:4b"
	ConnectionID  string
	Kind          string
	ToolCapable   bool
	Vision        bool
	ContextWindow int
}

// DiscoveryState is the closed set of per-provider discovery outcomes. Every
// effective-enabled connection resolves to exactly one of these — a failure is
// a typed state, NEVER a silent drop.
type DiscoveryState string

const (
	// StateOK — the provider answered; its models are in the catalog.
	StateOK DiscoveryState = "ok"
	// StateUnreachable — connect-class failure (daemon down, DNS/host wrong,
	// non-2xx response). Distinct from StateTimeout so a picker can tell
	// "down" from "slow".
	StateUnreachable DiscoveryState = "unreachable"
	// StateTimeout — the provider did not answer within per_provider_timeout_ms.
	StateTimeout DiscoveryState = "timeout"
	// StateAuthFailed — the provider rejected the credential (a hosted live
	// probe / test-connection outcome).
	StateAuthFailed DiscoveryState = "auth_failed"
	// StateDisabled — the connection is declared but not effective-enabled;
	// surfaced (never hidden) so the picker can show it shown-but-disabled.
	StateDisabled DiscoveryState = "disabled"
)

// ProviderDiscoveryStatus is the typed, ALWAYS-emitted per-connection
// discovery outcome (design §6.3). Detail is human/diagnostic text and is
// secret-free by construction (adapters never place a credential in an error).
type ProviderDiscoveryStatus struct {
	ConnectionID string
	Kind         string
	State        DiscoveryState
	ModelCount   int
	Detail       string
}

// ModelCatalog is the aggregated provider-qualified model set served to the
// selection surfaces. Models preserves a deterministic order (the aggregator
// emits adapters in declaration order — Ollama-local group first). Default is
// the no-override synthesis model (provider-qualified), carried so the boundary
// can build the existing validator with a non-empty baseline.
type ModelCatalog struct {
	Models  []ModelDescriptor
	Default string
}

// IDs returns the provider-qualified admissible set in catalog order. This is
// the set INJECTED into the existing modelswitch validator — the catalog IS
// the admissible set; the leaf stays pure.
func (c ModelCatalog) IDs() []string {
	out := make([]string, 0, len(c.Models))
	for _, m := range c.Models {
		out = append(out, m.ID)
	}
	return out
}

// ToolCapableIDs returns the provider-qualified ids whose descriptor is
// tool_capable, in catalog order. Surfaced by SCOPE-07; here it also lets the
// boundary satisfy the existing validator constructor's non-empty tool-capable
// requirement (089) without inventing a second set.
func (c ModelCatalog) ToolCapableIDs() []string {
	out := make([]string, 0, len(c.Models))
	for _, m := range c.Models {
		if m.ToolCapable {
			out = append(out, m.ID)
		}
	}
	return out
}

// InstalledOllama returns the set of BARE Ollama model names present in the
// catalog (kind == "ollama"), used to normalize a 089-era bare id (`gemma3:4b`)
// to `ollama/<id>` IFF installed, before the validator's string compare.
func (c ModelCatalog) InstalledOllama() map[string]struct{} {
	out := make(map[string]struct{})
	for _, m := range c.Models {
		if m.Kind == kindOllama {
			out[strings.TrimPrefix(m.ID, kindOllama+"/")] = struct{}{}
		}
	}
	return out
}

// DiscoveryError is a typed adapter failure carrying the DiscoveryState the
// aggregator will record. A plain (untyped) error maps to StateUnreachable; a
// context-deadline error maps to StateTimeout. The Detail is secret-free.
type DiscoveryError struct {
	State  DiscoveryState
	Detail string
}

func (e *DiscoveryError) Error() string { return e.Detail }
