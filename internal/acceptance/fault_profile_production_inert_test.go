package acceptance

import (
	"errors"
	"testing"
)

const synthesisReadbackRelationFailureProfileID = "synthesis_readback_relation_failure"

func TestSynthesisReadbackRelationFailureProfileIsCanonicalAndProductionInert(t *testing.T) {
	reg, err := LoadCanonicalRegistry()
	if err != nil {
		t.Fatalf("LoadCanonicalRegistry() error = %v; want nil", err)
	}

	profile, err := reg.Resolve(PostureTest, synthesisReadbackRelationFailureProfileID)
	if err != nil {
		t.Fatalf("Resolve(test, %q) error = %v; want registered profile", synthesisReadbackRelationFailureProfileID, err)
	}
	if profile.StableID != synthesisReadbackRelationFailureProfileID || profile.Journey != "synthesis" {
		t.Fatalf("resolved profile identity/journey = %q/%q; want %q/synthesis",
			profile.StableID, profile.Journey, synthesisReadbackRelationFailureProfileID)
	}
	if profile.NoFirstPartyInterception == nil || !*profile.NoFirstPartyInterception {
		t.Fatal("synthesis read-back relation profile must prohibit first-party interception")
	}
	if _, err := reg.Resolve(PostureProduction, synthesisReadbackRelationFailureProfileID); !errors.Is(err, ErrFaultInertInProduction) {
		t.Fatalf("Resolve(production, %q) error = %v; want errors.Is ErrFaultInertInProduction",
			synthesisReadbackRelationFailureProfileID, err)
	}
}

func TestProductionRoutesConfigRequestsAndUIExposeNoFaultSelectorOrTrigger(t *testing.T) {
	// Adversarial: under the production posture NO fault is activatable, even a
	// valid registered stableId. This proves a production build/config is
	// structurally fault-inert.
	t.Run("production posture cannot activate any registered fault", func(t *testing.T) {
		reg, err := LoadCanonicalRegistry()
		if err != nil {
			t.Fatalf("LoadCanonicalRegistry() error = %v; want nil", err)
		}
		ids := reg.StableIDs()
		if len(ids) == 0 {
			t.Fatalf("canonical registry declares zero profiles")
		}
		for _, id := range ids {
			if _, err := reg.Resolve(PostureProduction, id); !errors.Is(err, ErrFaultInertInProduction) {
				t.Errorf("Resolve(production, %q) error = %v; want errors.Is ErrFaultInertInProduction", id, err)
			}
		}
		// The eight BUG-073-006-consumed faults are all inert under production too.
		for _, id := range requiredAssistantStableIDs {
			if _, err := reg.Resolve(PostureProduction, id); !errors.Is(err, ErrFaultInertInProduction) {
				t.Errorf("Resolve(production, %q) error = %v; want errors.Is ErrFaultInertInProduction", id, err)
			}
		}
	})

	// An unrecognised posture fails closed (only the test posture activates).
	t.Run("unknown posture fails closed", func(t *testing.T) {
		reg, err := LoadCanonicalRegistry()
		if err != nil {
			t.Fatalf("LoadCanonicalRegistry() error = %v; want nil", err)
		}
		if _, err := reg.Resolve(Posture("staging"), "auth_401"); !errors.Is(err, ErrUnknownPosture) {
			t.Fatalf("Resolve(staging, auth_401) error = %v; want errors.Is ErrUnknownPosture", err)
		}
	})

	// Clean production route, config, request, and UI descriptors carry no fault
	// control and pass. Generic words that happen to equal a profile stableId
	// (for example "network" or "timeout") are NOT fault-control tokens and must
	// not be flagged.
	t.Run("clean production surfaces have no fault control", func(t *testing.T) {
		clean := []string{
			"GET /api/v1/search",
			"POST /assistant/turn",
			"digest.enabled=true",
			`<button data-testid="search-submit">Search</button>`,
			"network timeout retry policy configured at the edge",
			"recommendations.providers=[openai,local]",
		}
		if err := AssertNoFaultControlInProductionSurface(clean...); err != nil {
			t.Fatalf("AssertNoFaultControlInProductionSurface(clean) error = %v; want nil", err)
		}
	})

	// Adversarial: a production route, config, request header, UI control, call,
	// or registry reference that carries a fault selector/trigger is refused.
	t.Run("production surface with a fault control is refused", func(t *testing.T) {
		offenders := []struct {
			name    string
			surface string
		}{
			{"route with inject_fault trigger", "POST /admin/inject_fault?stableId=auth_401"},
			{"config with fault profile key", "faultProfile: auth_401"},
			{"request header with fault trigger", "X-Fault-Trigger: server_error"},
			{"UI with a fault selector control", `<select id="fault-selector"><option>network</option></select>`},
			{"call that activates a fault", "activate_fault(network)"},
			{"production reference to the fault registry", "fault-registry lookup wired into prod config"},
		}
		for _, o := range offenders {
			if err := AssertNoFaultControlInProductionSurface(o.surface); !errors.Is(err, ErrProductionFaultControl) {
				t.Errorf("AssertNoFaultControlInProductionSurface(%s) error = %v; want errors.Is ErrProductionFaultControl", o.name, err)
			}
		}
	})

	// Adversarial: a registry that declares a production posture or production
	// exposure fails to load.
	t.Run("registry declaring production exposure fails to load", func(t *testing.T) {
		if _, err := ParseRegistry([]byte(regYAMLWith("v1", "production", "forbidden", validProfileBody))); !errors.Is(err, ErrProductionExposureDeclared) {
			t.Errorf("ParseRegistry(posture=production) error = %v; want errors.Is ErrProductionExposureDeclared", err)
		}
		if _, err := ParseRegistry([]byte(regYAMLWith("v1", "test", "allowed", validProfileBody))); !errors.Is(err, ErrProductionExposureDeclared) {
			t.Errorf("ParseRegistry(productionExposure=allowed) error = %v; want errors.Is ErrProductionExposureDeclared", err)
		}
	})

	// The committed canonical registry declares the test-only, production-inert
	// envelope.
	t.Run("canonical registry declares test-only production-inert envelope", func(t *testing.T) {
		reg, err := LoadCanonicalRegistry()
		if err != nil {
			t.Fatalf("LoadCanonicalRegistry() error = %v; want nil", err)
		}
		if reg.Posture != "test" {
			t.Errorf("canonical registry posture = %q; want test", reg.Posture)
		}
		if reg.ProductionExposure != "forbidden" {
			t.Errorf("canonical registry productionExposure = %q; want forbidden", reg.ProductionExposure)
		}
	})
}
