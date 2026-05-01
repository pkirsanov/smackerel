// Package policy implements the Spec 038 Scope 6 sensitivity policy engine.
//
// The policy engine applies a single deterministic decision table to every
// drive surface that touches sensitive content: search "open in drive",
// save (rule guardrails), retrieval delivery, share suggestions, daily
// digest inclusion, and provider-side share-change alerts. Producing one
// engine instead of per-surface conditionals keeps the rules consistent
// across the runtime.
//
// Anchor: SCN-038-017 — sensitivity policy blocks unsafe auto-link
// sharing.
//
// Sensitivity tiers (matching drive_files.sensitivity CHECK constraint):
//
//	none < financial < medical < identity
//
// Surfaces (matching design.md §8.2):
//
//	SearchOpen      — "Open in Drive" link click confirmation
//	SaveLinkShare   — Save Rule attempts to create a public/anyone-link
//	Retrieval       — Telegram or web retrieval delivery mode selection
//	ShareSuggestion — UI prompt to share an artifact with others
//	DigestInclusion — auto-shared daily digest inclusion
//	ShareChangeAlert— provider-side share state changed (e.g., private→public)
//
// Decisions:
//
//	Allow            — surface proceeds unchanged
//	RequireConfirm   — surface MUST present an explicit confirmation gate
//	Downgrade        — surface MUST switch to a safer mode (e.g., bytes →
//	                   secure_link); details in DowngradeMode.
//	Refuse           — surface MUST refuse and surface a policy reason
package policy

import (
	"errors"
	"fmt"
	"strings"
)

// Sensitivity enumerates the canonical sensitivity tiers.
type Sensitivity string

const (
	SensitivityNone      Sensitivity = "none"
	SensitivityFinancial Sensitivity = "financial"
	SensitivityMedical   Sensitivity = "medical"
	SensitivityIdentity  Sensitivity = "identity"
)

// IsKnownSensitivity reports whether s is a recognized tier.
func IsKnownSensitivity(s string) bool {
	switch Sensitivity(s) {
	case SensitivityNone, SensitivityFinancial, SensitivityMedical, SensitivityIdentity:
		return true
	}
	return false
}

// Surface enumerates the policy enforcement points.
type Surface string

const (
	SurfaceSearchOpen       Surface = "search_open"
	SurfaceSaveLinkShare    Surface = "save_link_share"
	SurfaceRetrieval        Surface = "retrieval"
	SurfaceShareSuggestion  Surface = "share_suggestion"
	SurfaceDigestInclusion  Surface = "digest_inclusion"
	SurfaceShareChangeAlert Surface = "share_change_alert"
)

// IsKnownSurface reports whether s is a recognized enforcement surface.
func IsKnownSurface(s string) bool {
	switch Surface(s) {
	case SurfaceSearchOpen, SurfaceSaveLinkShare, SurfaceRetrieval,
		SurfaceShareSuggestion, SurfaceDigestInclusion, SurfaceShareChangeAlert:
		return true
	}
	return false
}

// Decision enumerates the engine's possible verdicts.
type Decision string

const (
	DecisionAllow          Decision = "allow"
	DecisionRequireConfirm Decision = "require_confirm"
	DecisionDowngrade      Decision = "downgrade"
	DecisionRefuse         Decision = "refuse"
)

// DowngradeMode names the alternate mode the surface MUST use when the
// engine returns DecisionDowngrade.
type DowngradeMode string

const (
	// DowngradeNone is used for non-downgrade decisions.
	DowngradeNone DowngradeMode = ""
	// DowngradeSecureLink replaces inline byte delivery with a secure
	// link the user must open from a trusted device. Used by the
	// retrieval surface for sensitive files.
	DowngradeSecureLink DowngradeMode = "secure_link"
	// DowngradeProviderLink replaces inline byte delivery with a direct
	// provider URL (no Smackerel-managed link). Used when the user is
	// authenticated against the provider and a deep link is acceptable.
	DowngradeProviderLink DowngradeMode = "provider_link"
)

// Verdict is the engine's structured response.
type Verdict struct {
	Surface       Surface
	Sensitivity   Sensitivity
	Decision      Decision
	DowngradeMode DowngradeMode
	Reason        string
}

// Action describes the proposed action being evaluated.
//
//   - Surface           — the enforcement point.
//   - Sensitivity       — the artifact's current tier.
//   - WouldCreateLink   — true if the action would create a provider-side
//     share link (e.g. Save Rule with NeverLinkShare guardrail off).
//   - LinkAudience      — the proposed audience for any new link
//     ('owner_only', 'restricted', 'anyone_with_link', 'public', etc.).
//   - DeliveryMode      — the proposed delivery mode for retrieval
//     ('bytes', 'secure_link', 'provider_link', 'refused').
//   - GuardrailNeverLinkShare — true if the matched Save Rule's
//     guardrails forbid auto-link sharing for this artifact.
//   - DigestAudience    — for SurfaceDigestInclusion: 'self_only',
//     'shared', 'public'.
//   - ShareChangeAudienceWidened — true when the previous monitor cycle
//     observed a narrower audience than the current one (e.g.
//     'owner_only' → 'anyone_with_link').
type Action struct {
	Surface                    Surface
	Sensitivity                Sensitivity
	WouldCreateLink            bool
	LinkAudience               string
	DeliveryMode               string
	GuardrailNeverLinkShare    bool
	DigestAudience             string
	ShareChangeAudienceWidened bool
}

// ErrInvalidAction is returned when the action's Surface or Sensitivity
// are not recognized. Callers MUST treat this as a configuration bug —
// failing loud is preferable to silently allowing the action.
var ErrInvalidAction = errors.New("policy: invalid action")

// Engine evaluates Action structs and returns deterministic Verdicts.
type Engine struct {
	observer Observer
}

// Observer receives policy verdicts after Engine.Evaluate computes them.
// Production wires the metrics package; tests pass nil. Observer MUST
// be cheap and non-blocking — Evaluate calls it inline.
type Observer interface {
	Observe(v Verdict)
}

// NewEngine constructs a stateless policy engine.
func NewEngine() *Engine { return &Engine{} }

// NewEngineWithObserver constructs a stateless policy engine that
// reports each verdict to obs. Pass nil obs to disable observation.
func NewEngineWithObserver(obs Observer) *Engine { return &Engine{observer: obs} }

// Evaluate applies the policy decision table.
//
// The decision table mirrors design.md §8.2 ("Policy enforcement points"):
//
//	Surface              | Sensitivity     | Decision
//	-------------------- | --------------- | ----------------
//	SearchOpen           | none            | Allow
//	SearchOpen           | financial,medical,identity | RequireConfirm
//	SaveLinkShare        | none            | Allow
//	SaveLinkShare        | any sensitive   | Refuse (regardless of NeverLinkShare guardrail)
//	SaveLinkShare        | none + guardrail NeverLinkShare = true
//	                                       | Refuse (guardrail wins for non-sensitive too)
//	Retrieval            | none            | Allow
//	Retrieval            | financial,medical,identity | Downgrade(SecureLink) — never bytes
//	ShareSuggestion      | none            | Allow
//	ShareSuggestion      | any sensitive   | Refuse
//	DigestInclusion      | none            | Allow
//	DigestInclusion      | any sensitive + DigestAudience='shared'|'public' | Refuse
//	DigestInclusion      | any sensitive + DigestAudience='self_only' | Allow (your own digest)
//	ShareChangeAlert     | sensitivity widened audience | Refuse downstream actions; surface alert
func (e *Engine) Evaluate(action Action) (Verdict, error) {
	verdict, err := e.evaluate(action)
	if err == nil && e.observer != nil {
		e.observer.Observe(verdict)
	}
	return verdict, err
}

func (e *Engine) evaluate(action Action) (Verdict, error) {
	if !IsKnownSurface(string(action.Surface)) {
		return Verdict{}, fmt.Errorf("%w: surface %q", ErrInvalidAction, action.Surface)
	}
	if action.Sensitivity == "" {
		return Verdict{}, fmt.Errorf("%w: sensitivity is required", ErrInvalidAction)
	}
	if !IsKnownSensitivity(string(action.Sensitivity)) {
		return Verdict{}, fmt.Errorf("%w: sensitivity %q", ErrInvalidAction, action.Sensitivity)
	}

	verdict := Verdict{
		Surface:     action.Surface,
		Sensitivity: action.Sensitivity,
	}
	sensitive := action.Sensitivity != SensitivityNone

	switch action.Surface {
	case SurfaceSearchOpen:
		if sensitive {
			verdict.Decision = DecisionRequireConfirm
			verdict.Reason = "sensitive_search_open_requires_confirmation"
			return verdict, nil
		}
		verdict.Decision = DecisionAllow
		return verdict, nil

	case SurfaceSaveLinkShare:
		// Guardrail wins regardless of sensitivity: NeverLinkShare = true
		// MUST refuse a link-creating save even on non-sensitive files.
		if action.GuardrailNeverLinkShare && action.WouldCreateLink {
			verdict.Decision = DecisionRefuse
			verdict.Reason = "guardrail_never_link_share"
			return verdict, nil
		}
		if sensitive && action.WouldCreateLink {
			verdict.Decision = DecisionRefuse
			verdict.Reason = "sensitive_link_share_blocked"
			return verdict, nil
		}
		verdict.Decision = DecisionAllow
		return verdict, nil

	case SurfaceRetrieval:
		// A change-monitor alert that has bumped sensitivity audience
		// MUST refuse retrieval — the audience widened underneath us
		// and the previously-safe content may now be exposed.
		if sensitive && action.ShareChangeAudienceWidened {
			verdict.Decision = DecisionRefuse
			verdict.Reason = "share_change_widened_audience"
			return verdict, nil
		}
		if sensitive {
			// Inline byte delivery is forbidden for sensitive files.
			// The surface MUST switch to a secure link.
			if strings.EqualFold(action.DeliveryMode, "bytes") || action.DeliveryMode == "" {
				verdict.Decision = DecisionDowngrade
				verdict.DowngradeMode = DowngradeSecureLink
				verdict.Reason = "sensitive_bytes_downgraded_to_secure_link"
				return verdict, nil
			}
			if strings.EqualFold(action.DeliveryMode, "secure_link") ||
				strings.EqualFold(action.DeliveryMode, "provider_link") {
				// Already a safe mode; allow.
				verdict.Decision = DecisionAllow
				return verdict, nil
			}
			verdict.Decision = DecisionRefuse
			verdict.Reason = "sensitive_unknown_delivery_mode"
			return verdict, nil
		}
		verdict.Decision = DecisionAllow
		return verdict, nil

	case SurfaceShareSuggestion:
		if sensitive {
			verdict.Decision = DecisionRefuse
			verdict.Reason = "sensitive_share_suggestion_blocked"
			return verdict, nil
		}
		verdict.Decision = DecisionAllow
		return verdict, nil

	case SurfaceDigestInclusion:
		if sensitive {
			audience := strings.ToLower(strings.TrimSpace(action.DigestAudience))
			if audience == "" || audience == "shared" || audience == "public" {
				verdict.Decision = DecisionRefuse
				verdict.Reason = "sensitive_excluded_from_shared_digest"
				return verdict, nil
			}
		}
		verdict.Decision = DecisionAllow
		return verdict, nil

	case SurfaceShareChangeAlert:
		// The share-change alert surface itself never refuses — it
		// surfaces the alert to the user. Downstream actions
		// (retrieval / link-share / digest) consult the
		// ShareChangeAudienceWidened flag to refuse.
		verdict.Decision = DecisionAllow
		verdict.Reason = "alert_surfaced_for_review"
		return verdict, nil
	}

	return Verdict{}, fmt.Errorf("%w: unhandled surface %q", ErrInvalidAction, action.Surface)
}
