// Spec 096 SCOPE-07 — the late-bound combined-catalog + budget sources that the
// unified selection surfaces (Telegram /model picker + HTTP /v1/agent/model)
// read to render the provider-qualified catalog.
//
// These parallel the spec-088 allowlistRef and the spec-089 modelPrefRef: one
// source reached lock-free by BOTH structurally-separate fast-paths so the
// rendered catalog is the SAME on every surface (the 088/089 one-validator/
// one-store parity, extended to the catalog VIEW). They are ADDITIVE: when no
// catalog source is wired (the deferred live-aggregator activation state), both
// surfaces fall back to the byte-for-byte 089 flat list, and the HTTP view
// carries no enrichment (omitempty) — an 089 client sees no change.
//
// No defaults (G028): a nil source is "no combined catalog wired", treated as
// the baseline 089 path, NEVER a fabricated catalog.
package agenttool

import (
	"context"
	"sync/atomic"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/catalog"
)

// CatalogProvider is the late-bound combined provider-qualified catalog source
// (spec 096 SCOPE-04 CatalogAggregator). *catalog.CatalogAggregator structurally
// satisfies it. The two surfaces read it to render the provider-grouped picker
// (reachable models numbered/selectable; an unreachable provider shown-but-
// disabled with its typed ProviderDiscoveryStatus, never silently dropped).
type CatalogProvider interface {
	// GetCatalog returns the aggregated provider-qualified catalog plus one
	// typed ProviderDiscoveryStatus per effective-enabled connection.
	GetCatalog(ctx context.Context) (catalog.ModelCatalog, []catalog.ProviderDiscoveryStatus)
}

// catalogProviderHolder wraps the CatalogProvider interface so it can live in an
// atomic.Pointer (which needs a concrete element type) without the typed-nil
// gotcha. A nil holder ⇒ no catalog source wired.
type catalogProviderHolder struct{ provider CatalogProvider }

// catalogProviderRef is the late-bound spec 096 combined-catalog source. Both
// fast-paths' picker render reach it via ModelCatalogProvider(); cmd/core wiring
// installs it once the live discovery aggregator is activated.
var catalogProviderRef atomic.Pointer[catalogProviderHolder]

// SetModelCatalogProvider installs the runtime combined-catalog source. Passing
// nil clears the binding; ModelCatalogProvider() then returns nil and the
// surfaces fall back to the byte-for-byte 089 flat list (deferred-activation
// baseline, never a panic — mirrors the nil-allowlist / nil-store passthrough).
func SetModelCatalogProvider(p CatalogProvider) {
	if p == nil {
		catalogProviderRef.Store(nil)
		return
	}
	catalogProviderRef.Store(&catalogProviderHolder{provider: p})
}

// ModelCatalogProvider returns the currently bound CatalogProvider (or nil when
// not wired). Both surfaces read it nil-safely.
func ModelCatalogProvider() CatalogProvider {
	h := catalogProviderRef.Load()
	if h == nil {
		return nil
	}
	return h.provider
}

// BudgetProvider is the late-bound month-to-date USD spend source (spec 096
// SCOPE-05 SpendLedger). The agent's SpendLedger structurally satisfies it. The
// claim-bound actor rides ctx (the DB-backed impl reads auth.UserIDFromContext),
// so this port never accepts a request-body user id — the picker's budget line
// is for the bearer/Telegram subject only.
type BudgetProvider interface {
	// MonthToDateSpend returns the current-month USD spend for the claim-bound
	// caller (perUserUSD) and across all callers (globalUSD).
	MonthToDateSpend(ctx context.Context) (perUserUSD, globalUSD float64, err error)
}

// budgetProviderHolder wraps BudgetProvider for the atomic.Pointer. A nil holder
// ⇒ no budget source wired (the picker omits the budget line — Principle 6:
// no nag when there is nothing to report).
type budgetProviderHolder struct{ provider BudgetProvider }

// budgetProviderRef is the late-bound spec 096 month-to-date spend source. The
// picker surfaces a budget line ONLY when this is wired AND a paid model is in
// the catalog.
var budgetProviderRef atomic.Pointer[budgetProviderHolder]

// SetBudgetProvider installs the runtime month-to-date USD spend source. Passing
// nil clears the binding; BudgetProvider() then returns nil and the picker omits
// the budget line.
func SetBudgetProvider(p BudgetProvider) {
	if p == nil {
		budgetProviderRef.Store(nil)
		return
	}
	budgetProviderRef.Store(&budgetProviderHolder{provider: p})
}

// CurrentBudgetProvider returns the currently bound BudgetProvider (or nil when
// not wired). The surfaces read it nil-safely.
func CurrentBudgetProvider() BudgetProvider {
	h := budgetProviderRef.Load()
	if h == nil {
		return nil
	}
	return h.provider
}
