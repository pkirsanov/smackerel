package policy

// Spec 038 Scope 6 — SCN-038-017 unit anchor.
//
// TestMedicalPolicyBlocksAutoLinkShareWithoutProviderMutation asserts
// the central contract of the sensitivity policy engine:
//
//  1. A medical-tier file MUST refuse a Save Rule that would create a
//     provider-side share link, even if the rule itself does not set the
//     NeverLinkShare guardrail. Sensitive content blocks link sharing
//     unconditionally.
//  2. A non-sensitive file with NeverLinkShare=true MUST also refuse.
//     The guardrail wins regardless of tier so a misconfigured rule
//     cannot leak content even at sensitivity=none.
//  3. A medical-tier file requested through Telegram retrieval MUST
//     downgrade to secure_link delivery (never bytes). Adversarial: a
//     bytes-mode request is downgraded, not approved.
//  4. A medical file MUST be excluded from any shared/public digest.
//     Adversarial: a non-sensitive file with the same audience MUST be
//     allowed so the test cannot pass by always returning Refuse.
//  5. Share suggestion is refused for any sensitive file.
//  6. Search "Open in Drive" requires confirmation for sensitive files
//     but allows non-sensitive ones.
//  7. A widened share audience (e.g. private → public) MUST refuse
//     downstream retrieval until the user reviews the alert.
//  8. Adversarial: an unknown surface or sensitivity tier MUST surface
//     ErrInvalidAction so a future regression that adds a new tier
//     fails loud instead of silently allowing.

import (
	"errors"
	"testing"
)

func TestMedicalPolicyBlocksAutoLinkShareWithoutProviderMutation(t *testing.T) {
	engine := NewEngine()

	t.Run("medical save link share refused (no guardrail required)", func(t *testing.T) {
		v, err := engine.Evaluate(Action{
			Surface:                 SurfaceSaveLinkShare,
			Sensitivity:             SensitivityMedical,
			WouldCreateLink:         true,
			LinkAudience:            "anyone_with_link",
			GuardrailNeverLinkShare: false, // rule does NOT set guardrail; tier alone refuses
		})
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if v.Decision != DecisionRefuse {
			t.Fatalf("Decision = %q, want refuse — sensitive_link_share_blocked", v.Decision)
		}
		if v.Reason != "sensitive_link_share_blocked" {
			t.Fatalf("Reason = %q, want sensitive_link_share_blocked", v.Reason)
		}
	})

	// Adversarial: a non-sensitive file with NeverLinkShare guardrail set
	// MUST still refuse. If a future regression flipped the order so the
	// engine checks sensitivity first and skipped the guardrail, this
	// would catch it.
	t.Run("guardrail never_link_share refuses even at sensitivity=none (adversarial)", func(t *testing.T) {
		v, err := engine.Evaluate(Action{
			Surface:                 SurfaceSaveLinkShare,
			Sensitivity:             SensitivityNone,
			WouldCreateLink:         true,
			GuardrailNeverLinkShare: true,
		})
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if v.Decision != DecisionRefuse {
			t.Fatalf("Decision = %q, want refuse — guardrail_never_link_share", v.Decision)
		}
		if v.Reason != "guardrail_never_link_share" {
			t.Fatalf("Reason = %q, want guardrail_never_link_share", v.Reason)
		}
	})

	// Adversarial: a save that does NOT create a link MUST be allowed
	// even at medical sensitivity. The engine MUST refuse only the
	// link-creation arm, not all saves.
	t.Run("medical save without link is allowed (adversarial)", func(t *testing.T) {
		v, err := engine.Evaluate(Action{
			Surface:         SurfaceSaveLinkShare,
			Sensitivity:     SensitivityMedical,
			WouldCreateLink: false,
		})
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if v.Decision != DecisionAllow {
			t.Fatalf("Decision = %q, want allow — non-link save MUST proceed", v.Decision)
		}
	})

	t.Run("medical retrieval bytes mode downgraded to secure_link", func(t *testing.T) {
		v, err := engine.Evaluate(Action{
			Surface:      SurfaceRetrieval,
			Sensitivity:  SensitivityMedical,
			DeliveryMode: "bytes",
		})
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if v.Decision != DecisionDowngrade {
			t.Fatalf("Decision = %q, want downgrade", v.Decision)
		}
		if v.DowngradeMode != DowngradeSecureLink {
			t.Fatalf("DowngradeMode = %q, want secure_link", v.DowngradeMode)
		}
	})

	// Adversarial: requesting secure_link directly MUST allow, not
	// double-downgrade.
	t.Run("medical retrieval secure_link mode is allowed (adversarial)", func(t *testing.T) {
		v, err := engine.Evaluate(Action{
			Surface:      SurfaceRetrieval,
			Sensitivity:  SensitivityMedical,
			DeliveryMode: "secure_link",
		})
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if v.Decision != DecisionAllow {
			t.Fatalf("Decision = %q, want allow — already-safe mode", v.Decision)
		}
	})

	// Adversarial: even sensitive retrievals MUST refuse when the share
	// audience widened underneath us.
	t.Run("retrieval refused after share audience widened (adversarial)", func(t *testing.T) {
		v, err := engine.Evaluate(Action{
			Surface:                    SurfaceRetrieval,
			Sensitivity:                SensitivityFinancial,
			DeliveryMode:               "secure_link",
			ShareChangeAudienceWidened: true,
		})
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if v.Decision != DecisionRefuse {
			t.Fatalf("Decision = %q, want refuse — share_change_widened_audience", v.Decision)
		}
		if v.Reason != "share_change_widened_audience" {
			t.Fatalf("Reason = %q, want share_change_widened_audience", v.Reason)
		}
	})

	t.Run("medical excluded from shared digest", func(t *testing.T) {
		v, err := engine.Evaluate(Action{
			Surface:        SurfaceDigestInclusion,
			Sensitivity:    SensitivityMedical,
			DigestAudience: "shared",
		})
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if v.Decision != DecisionRefuse {
			t.Fatalf("Decision = %q, want refuse — sensitive_excluded_from_shared_digest", v.Decision)
		}
	})

	// Adversarial: a NON-sensitive file with the same audience MUST be
	// allowed. If the engine collapsed to "always refuse digest" this
	// would fail.
	t.Run("non-sensitive shared digest allowed (adversarial)", func(t *testing.T) {
		v, err := engine.Evaluate(Action{
			Surface:        SurfaceDigestInclusion,
			Sensitivity:    SensitivityNone,
			DigestAudience: "shared",
		})
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if v.Decision != DecisionAllow {
			t.Fatalf("Decision = %q, want allow — non-sensitive content can ride a shared digest", v.Decision)
		}
	})

	// Adversarial: a sensitive file in YOUR OWN self_only digest MUST
	// be allowed. The user is the sole audience.
	t.Run("sensitive self_only digest allowed (adversarial)", func(t *testing.T) {
		v, err := engine.Evaluate(Action{
			Surface:        SurfaceDigestInclusion,
			Sensitivity:    SensitivityMedical,
			DigestAudience: "self_only",
		})
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if v.Decision != DecisionAllow {
			t.Fatalf("Decision = %q, want allow — sensitive in self-only digest is fine", v.Decision)
		}
	})

	t.Run("sensitive share suggestion refused", func(t *testing.T) {
		v, err := engine.Evaluate(Action{
			Surface:     SurfaceShareSuggestion,
			Sensitivity: SensitivityIdentity,
		})
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if v.Decision != DecisionRefuse {
			t.Fatalf("Decision = %q, want refuse for share suggestion on identity-tier file", v.Decision)
		}
	})

	t.Run("sensitive search open requires confirmation", func(t *testing.T) {
		v, err := engine.Evaluate(Action{
			Surface:     SurfaceSearchOpen,
			Sensitivity: SensitivityFinancial,
		})
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if v.Decision != DecisionRequireConfirm {
			t.Fatalf("Decision = %q, want require_confirm", v.Decision)
		}
	})

	// Adversarial: non-sensitive search open MUST allow without prompting.
	t.Run("non-sensitive search open allowed (adversarial)", func(t *testing.T) {
		v, err := engine.Evaluate(Action{
			Surface:     SurfaceSearchOpen,
			Sensitivity: SensitivityNone,
		})
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if v.Decision != DecisionAllow {
			t.Fatalf("Decision = %q, want allow for non-sensitive open", v.Decision)
		}
	})

	// Adversarial: an unknown surface MUST fail loud.
	t.Run("unknown surface returns ErrInvalidAction (adversarial)", func(t *testing.T) {
		_, err := engine.Evaluate(Action{
			Surface:     Surface("comments"),
			Sensitivity: SensitivityNone,
		})
		if !errors.Is(err, ErrInvalidAction) {
			t.Fatalf("Evaluate(unknown surface) err = %v, want ErrInvalidAction", err)
		}
	})

	// Adversarial: an unknown sensitivity tier MUST fail loud.
	t.Run("unknown sensitivity returns ErrInvalidAction (adversarial)", func(t *testing.T) {
		_, err := engine.Evaluate(Action{
			Surface:     SurfaceSaveLinkShare,
			Sensitivity: Sensitivity("topsecret"),
		})
		if !errors.Is(err, ErrInvalidAction) {
			t.Fatalf("Evaluate(unknown tier) err = %v, want ErrInvalidAction", err)
		}
	})
}
