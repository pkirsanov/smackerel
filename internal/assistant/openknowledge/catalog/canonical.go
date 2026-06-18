// Spec 096 SCOPE-04 — identifier canonicalization + the resolver boundary
// (design §8 identifier grammar OQ-4).
//
// Canonicalization happens HERE, at the agenttool resolver boundary, on the
// inputs to the EXISTING spec-088/089 modelswitch validator — never inside the
// pure modelswitch leaf. The provider-qualified catalog is INJECTED as the
// validator's admissible set; an off-catalog id yields the SAME
// modelswitch.Rejection shape with NO modelpref store write and NO dispatch.
//
// Grammar (design §8):
//   - Canonical form `<provider-kind>/<backend-model-id>`, split on the FIRST
//     `/` only — `kind` never contains `/`, so a backend id that itself
//     contains `/` or `:` (Ollama `ollama/library/llama3:8b`, Bedrock
//     `bedrock/anthropic.claude-3-5-sonnet-20241022-v2:0`) round-trips.
//   - A BARE id (no `/`, a 089-era Ollama selection like `gemma3:4b`) is
//     normalized to `ollama/<id>` IFF `<id>` is in the Ollama installed set,
//     so today's bare-Ollama selections validate + dispatch unchanged.
package catalog

import (
	"context"
	"strings"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelpref"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
)

// SplitQualified splits a provider-qualified id on the FIRST `/` only.
// kind = the substring before the first `/`; backend = everything after (which
// may itself contain `/` and `:`). ok=false when either half is empty (the id
// is not provider-qualified). Re-joining `kind + "/" + backend` reproduces the
// input exactly (round-trip).
func SplitQualified(id string) (kind, backend string, ok bool) {
	id = strings.TrimSpace(id)
	k, b, found := strings.Cut(id, "/")
	if !found || k == "" || b == "" {
		return "", "", false
	}
	return k, b, true
}

// Canonicalize normalizes an untrusted raw model id to its provider-qualified
// canonical form for the validator's string compare. An already-qualified id
// (contains `/`) is returned trimmed-but-unchanged (split-on-first-`/` is
// applied downstream by SplitQualified / the dispatch resolver). A bare id is
// normalized to `ollama/<id>` IFF installed; a bare id that is NOT installed is
// returned trimmed-but-unqualified so the validator rejects it as off-catalog.
// Empty/whitespace → "" (the baseline no-override sentinel, matching
// modelswitch.Resolve("")).
func Canonicalize(raw string, installed map[string]struct{}) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "/") {
		return raw
	}
	if _, ok := installed[raw]; ok {
		return kindOllama + "/" + raw
	}
	return raw
}

// Allowlist builds the EXISTING modelswitch validator with this catalog's
// provider-qualified ids as the INJECTED admissible set — the catalog IS the
// admissible set; the modelswitch leaf stays pure (it never imports catalog).
//
// Catalog membership is a pure string-equality decision: a hosted model runs on
// the provider's hardware, not the local Ollama memory envelope, so the
// envelope check is disabled here (envelopeMiB = 0) and every entry carries a
// 0-MiB profile purely to satisfy the constructor's "every switchable has a
// profile" invariant. The spec-088 local co-residence envelope continues to be
// enforced for the Ollama switchable subset at the config-generation layer
// (unchanged). The catalog's Default is the non-empty baseline; the boundary
// uses Resolve (synthesis membership), so the gather axis (089) is untouched.
func (c ModelCatalog) Allowlist() (*modelswitch.Allowlist, error) {
	ids := c.IDs()
	profiles := make(map[string]int, len(ids))
	for _, id := range ids {
		profiles[id] = 0
	}
	toolCapable := c.ToolCapableIDs()
	if len(toolCapable) == 0 {
		// The 089 constructor requires a non-empty tool-capable set. When a
		// catalog carries no tool-capable descriptor, fall back to the full
		// admissible set so the synthesis-membership validator can still be
		// built; the gather axis is resolved separately (089) and not
		// exercised by the catalog boundary.
		toolCapable = ids
	}
	// gatherModel "" ⇒ the constructor's baseline-member check is skipped; the
	// catalog boundary validates synthesis membership only.
	return modelswitch.NewAllowlist(ids, profiles, 0, "", c.Default, toolCapable)
}

// CatalogResolver is the agenttool resolver-boundary helper: it canonicalizes
// an untrusted raw selection against the catalog's installed-Ollama set, then
// delegates the closed-set membership decision to the INJECTED modelswitch
// validator (whose admissible set is the provider-qualified catalog). One
// validator, one store — it introduces no second validator/store/picker.
type CatalogResolver struct {
	allow     *modelswitch.Allowlist
	installed map[string]struct{}
}

// NewCatalogResolver wraps an already-built (injected) modelswitch validator
// plus the Ollama installed set used for bare-id normalization.
func NewCatalogResolver(allow *modelswitch.Allowlist, installed map[string]struct{}) *CatalogResolver {
	return &CatalogResolver{allow: allow, installed: installed}
}

// NewCatalogResolverFromCatalog is the convenience boundary builder: it derives
// the injected validator + installed-Ollama set from the aggregated catalog.
func NewCatalogResolverFromCatalog(c ModelCatalog) (*CatalogResolver, error) {
	allow, err := c.Allowlist()
	if err != nil {
		return nil, err
	}
	return &CatalogResolver{allow: allow, installed: c.InstalledOllama()}, nil
}

// Validate canonicalizes raw, then delegates membership to the injected
// modelswitch validator. Returns (canonical, nil) for an in-catalog id, or
// ("", *modelswitch.Rejection) for an off-catalog id. It performs NO store
// write and NO dispatch — a pure boundary guard. An empty raw is the baseline
// (no override): ("", nil).
func (r *CatalogResolver) Validate(raw string) (string, *modelswitch.Rejection) {
	canonical := Canonicalize(raw, r.installed)
	if canonical == "" {
		return "", nil
	}
	if _, rej := r.allow.Resolve(canonical); rej != nil {
		return "", rej
	}
	return canonical, nil
}

// Select is the selection-surface boundary: it Validates raw and, ONLY on a
// clean validation, persists the canonical provider-qualified id to the
// EXISTING modelpref store for the claim-bound user. An off-catalog id returns
// the typed *modelswitch.Rejection and performs NO store write (and the caller
// performs NO dispatch). One validator, one store — no new picker.
func (r *CatalogResolver) Select(ctx context.Context, store modelpref.Store, userID, raw string) (string, *modelswitch.Rejection, error) {
	canonical, rej := r.Validate(raw)
	if rej != nil {
		return "", rej, nil // NO store write
	}
	if canonical == "" {
		return "", nil, nil // baseline — nothing to persist
	}
	if err := store.Set(ctx, userID, canonical); err != nil {
		return "", nil, err
	}
	return canonical, nil, nil
}
