package photos

import (
	"strings"
	"testing"
)

// TestPhotoRoutingTargetsRequireClassificationAndConfidence covers
// SCN-040-011: routing decisions only fire when classification has both
// the required LLM-owned fields and meets the configured confidence
// threshold. The test exercises the pure EvaluateRouting function so
// the contract can be validated without a live database.
func TestPhotoRoutingTargetsRequireClassificationAndConfidence(t *testing.T) {
	threshold := 0.75

	t.Run("invalid classification cannot route", func(t *testing.T) {
		_, err := EvaluateRouting(ClassificationDecision{
			PrimaryCategory: "receipt",
			Confidence:      0.9,
			// caption + rationale missing — adversarial: must be
			// rejected even when category and confidence look good.
		}, SensitivityNone, nil, threshold)
		if err == nil {
			t.Fatalf("expected EvaluateRouting to reject classification missing caption/rationale")
		}
	})

	t.Run("invalid threshold rejected", func(t *testing.T) {
		_, err := EvaluateRouting(ClassificationDecision{
			Caption:         "receipt",
			PrimaryCategory: "receipt",
			Confidence:      0.9,
			Rationale:       "fixture",
		}, SensitivityNone, nil, 0)
		if err == nil {
			t.Fatalf("expected EvaluateRouting to reject zero threshold")
		}
	})

	t.Run("below threshold returns no plans", func(t *testing.T) {
		plans, err := EvaluateRouting(ClassificationDecision{
			Caption:         "receipt",
			PrimaryCategory: "receipt",
			Confidence:      0.4,
			Rationale:       "fixture below threshold",
		}, SensitivityNone, nil, threshold)
		if err != nil {
			t.Fatalf("EvaluateRouting returned error for below-threshold input: %v", err)
		}
		if len(plans) != 0 {
			t.Fatalf("expected no plans below threshold, got %d", len(plans))
		}
	})

	t.Run("receipt routes to expense+document+knowledge", func(t *testing.T) {
		plans, err := EvaluateRouting(ClassificationDecision{
			Caption:         "Coffee shop receipt for 4.50 USD",
			PrimaryCategory: "receipt",
			Confidence:      0.92,
			Rationale:       "OCR matched receipt-shaped totals + tax line",
		}, SensitivityNone, nil, threshold)
		if err != nil {
			t.Fatalf("EvaluateRouting receipt: %v", err)
		}
		targets := mapPlanTargets(plans)
		for _, want := range []RouteTarget{RouteTargetExpense, RouteTargetDocument, RouteTargetKnowledge} {
			if _, ok := targets[want]; !ok {
				t.Fatalf("receipt routing missing target %q (got %v)", want, targets)
			}
		}
	})

	t.Run("recipe routes to recipe+mealplan", func(t *testing.T) {
		plans, err := EvaluateRouting(ClassificationDecision{
			Caption:         "Recipe card for tomato sauce",
			PrimaryCategory: "recipe_card",
			Confidence:      0.88,
			Rationale:       "title + ingredients block",
		}, SensitivityNone, nil, threshold)
		if err != nil {
			t.Fatalf("EvaluateRouting recipe: %v", err)
		}
		targets := mapPlanTargets(plans)
		for _, want := range []RouteTarget{RouteTargetRecipe, RouteTargetMealplan} {
			if _, ok := targets[want]; !ok {
				t.Fatalf("recipe routing missing target %q (got %v)", want, targets)
			}
		}
	})

	t.Run("hidden sensitivity blocks every target", func(t *testing.T) {
		plans, err := EvaluateRouting(ClassificationDecision{
			Caption:         "Passport photo page",
			PrimaryCategory: "identity_document",
			Confidence:      0.96,
			Rationale:       "MRZ pattern detected",
		}, SensitivityHidden, []string{"identity_document"}, threshold)
		if err != nil {
			t.Fatalf("EvaluateRouting hidden: %v", err)
		}
		if len(plans) == 0 {
			t.Fatalf("expected hidden sensitivity to still emit blocked plans for audit")
		}
		for _, plan := range plans {
			if !plan.SensitivityBlocked {
				t.Fatalf("hidden plan must be blocked: %+v", plan)
			}
			if plan.BlockingSensitivity != "hidden" {
				t.Fatalf("hidden block reason mismatch: %+v", plan)
			}
		}
	})

	t.Run("sensitive blocks expense + intelligence routes", func(t *testing.T) {
		plans, err := EvaluateRouting(ClassificationDecision{
			Caption:         "Bank statement quarterly",
			PrimaryCategory: "receipt",
			Confidence:      0.9,
			Rationale:       "fixture",
		}, SensitivitySensitive, []string{"financial"}, threshold)
		if err != nil {
			t.Fatalf("EvaluateRouting sensitive: %v", err)
		}
		blocked := 0
		for _, plan := range plans {
			if plan.Target == RouteTargetExpense {
				if !plan.SensitivityBlocked {
					t.Fatalf("expense route must be blocked when sensitive: %+v", plan)
				}
				blocked++
			}
		}
		if blocked == 0 {
			t.Fatalf("expected at least the expense route to be blocked under sensitive: %v", plans)
		}
	})

	t.Run("unknown category emits no plans", func(t *testing.T) {
		plans, err := EvaluateRouting(ClassificationDecision{
			Caption:         "Sunset over the river",
			PrimaryCategory: "scenery",
			Confidence:      0.8,
			Rationale:       "fixture",
		}, SensitivityNone, nil, threshold)
		if err != nil {
			t.Fatalf("EvaluateRouting scenery: %v", err)
		}
		if len(plans) != 0 {
			t.Fatalf("scenery should not route anywhere yet: %v", plans)
		}
	})
}

func TestSourceChannelValid(t *testing.T) {
	for _, channel := range []SourceChannel{
		SourceChannelProvider, SourceChannelTelegram,
		SourceChannelMobile, SourceChannelWeb, SourceChannelAgent,
	} {
		if !channel.Valid() {
			t.Fatalf("expected channel %q to be valid", channel)
		}
	}
	if SourceChannel("unknown").Valid() {
		t.Fatalf("expected unknown source channel to be invalid")
	}
}

func TestAllRouteTargetsEnumerationStaysAlphabetical(t *testing.T) {
	targets := AllRouteTargets()
	for i := 1; i < len(targets); i++ {
		if string(targets[i-1]) >= string(targets[i]) {
			t.Fatalf("AllRouteTargets must stay alphabetised (broke at index %d: %s vs %s)", i, targets[i-1], targets[i])
		}
	}
	for _, target := range targets {
		if !target.Valid() {
			t.Fatalf("AllRouteTargets returned invalid target %q", target)
		}
	}
	if !strings.HasPrefix(string(RouteTargetExpense), "expense") {
		t.Fatalf("RouteTargetExpense literal drift: %s", RouteTargetExpense)
	}
}

func mapPlanTargets(plans []RoutingPlan) map[RouteTarget]RoutingPlan {
	out := make(map[RouteTarget]RoutingPlan, len(plans))
	for _, plan := range plans {
		out[plan.Target] = plan
	}
	return out
}
