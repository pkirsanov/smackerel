package acceptance

import (
	"errors"
	"fmt"
	"testing"
)

// requiredAssistantStableIDs are the eight Assistant fault profiles that
// BUG-073-006 SCOPE-01 consumes from this BUG-102-001-owned registry by
// immutable stableId (its state.json faultProfilePolicy.consumedProfiles).
// Every one targets the Assistant journey.
var requiredAssistantStableIDs = []string{
	"auth_401",
	"access_403",
	"rate_limited",
	"provider_unavailable",
	"server_error",
	"timeout",
	"network",
	"schema_decode",
}

// validProfileBody is a single well-formed profile list item (2-space indented
// to sit under a `profiles:` key). Adversarial fixtures mutate one field.
const validProfileBody = `  - stableId: "auth_401"
    journey: "assistant"
    setup: "provision a disposable env=test* unauthenticated context"
    teardown: "discard the context and reset the disposable session store"
    parallelism: "isolated per worker with its own session-store namespace"
    expectedRequest: "real first-party POST with no valid session cookie"
    expectedResponseOrTermination: "terminates with the auth_401 outcome, never a blank body"
    evidence: "value-safe auth_401 outcome key and 401 status class only"
    noFirstPartyInterception: true
`

// regYAMLWith builds a full registry document from an explicit envelope and a
// profiles body, so adversarial tests can exercise every envelope and profile
// variation without writing files.
func regYAMLWith(version, posture, exposure, profilesBody string) string {
	return fmt.Sprintf(
		"version: %s\nposture: %s\nproductionExposure: %s\nprofiles:\n%s",
		version, posture, exposure, profilesBody,
	)
}

// validRegistryYAML is a minimal valid registry with one well-formed profile.
func validRegistryYAML() string {
	return regYAMLWith("v1", "test", "forbidden", validProfileBody)
}

func TestFaultProfileRegistryRequiresEveryDeclaredFieldAndRejectsFirstPartyInterception(t *testing.T) {
	// The committed canonical registry loads, JSON-Schema-validates, and every
	// profile carries the closed nine-field schema.
	t.Run("canonical registry loads schema-validates and every profile is complete", func(t *testing.T) {
		reg, err := LoadCanonicalRegistry()
		if err != nil {
			t.Fatalf("LoadCanonicalRegistry() error = %v; want nil", err)
		}
		if len(reg.Profiles) == 0 {
			t.Fatalf("canonical registry declares zero profiles")
		}
		for _, p := range reg.Profiles {
			if p.StableID == "" || p.Journey == "" || p.Setup == "" || p.Teardown == "" ||
				p.Parallelism == "" || p.ExpectedRequest == "" ||
				p.ExpectedResponseOrTermination == "" || p.Evidence == "" {
				t.Errorf("profile %q is missing a required string field: %+v", p.StableID, p)
			}
			if p.NoFirstPartyInterception == nil || !*p.NoFirstPartyInterception {
				t.Errorf("profile %q must declare noFirstPartyInterception=true", p.StableID)
			}
			if !closedJourneys[p.Journey] {
				t.Errorf("profile %q targets journey %q outside the closed set", p.StableID, p.Journey)
			}
		}
	})

	// Every BUG-073-006-consumed Assistant fault profile is resolvable by its
	// exact stableId under the test posture and targets the Assistant journey.
	t.Run("required BUG-073-006 assistant stableIds resolve under test posture", func(t *testing.T) {
		reg, err := LoadCanonicalRegistry()
		if err != nil {
			t.Fatalf("LoadCanonicalRegistry() error = %v; want nil", err)
		}
		for _, id := range requiredAssistantStableIDs {
			p, err := reg.Resolve(PostureTest, id)
			if err != nil {
				t.Errorf("Resolve(test, %q) error = %v; want a registered profile", id, err)
				continue
			}
			if p.StableID != id {
				t.Errorf("Resolve(test, %q) returned stableId %q; want %q", id, p.StableID, id)
			}
			if p.Journey != "assistant" {
				t.Errorf("profile %q journey = %q; want assistant", id, p.Journey)
			}
		}
	})

	// Consumer lookup fails closed: an unknown/missing stableId returns a typed
	// error and never falls back to an inline definition.
	t.Run("unknown stableId fails closed with no fallback", func(t *testing.T) {
		reg, err := LoadCanonicalRegistry()
		if err != nil {
			t.Fatalf("LoadCanonicalRegistry() error = %v; want nil", err)
		}
		if _, err := reg.Resolve(PostureTest, "does_not_exist"); !errors.Is(err, ErrUnknownProfile) {
			t.Fatalf("Resolve(test, unknown) error = %v; want errors.Is ErrUnknownProfile", err)
		}
	})

	// Sanity: the base valid fixture parses so each adversarial mutation below
	// isolates exactly one violation.
	t.Run("base valid fixture parses", func(t *testing.T) {
		if _, err := ParseRegistry([]byte(validRegistryYAML())); err != nil {
			t.Fatalf("ParseRegistry(valid fixture) error = %v; want nil", err)
		}
	})

	// Adversarial: each required closed-schema field is enforced, and a profile
	// that declares (or omits) first-party interception is rejected.
	missingSetup := `  - stableId: "auth_401"
    journey: "assistant"
    teardown: "t"
    parallelism: "p"
    expectedRequest: "req"
    expectedResponseOrTermination: "resp"
    evidence: "ev"
    noFirstPartyInterception: true
`
	blankEvidence := `  - stableId: "auth_401"
    journey: "assistant"
    setup: "s"
    teardown: "t"
    parallelism: "p"
    expectedRequest: "req"
    expectedResponseOrTermination: "resp"
    evidence: ""
    noFirstPartyInterception: true
`
	omittedInterception := `  - stableId: "auth_401"
    journey: "assistant"
    setup: "s"
    teardown: "t"
    parallelism: "p"
    expectedRequest: "req"
    expectedResponseOrTermination: "resp"
    evidence: "ev"
`
	declaresInterception := `  - stableId: "auth_401"
    journey: "assistant"
    setup: "s"
    teardown: "t"
    parallelism: "p"
    expectedRequest: "req"
    expectedResponseOrTermination: "resp"
    evidence: "ev"
    noFirstPartyInterception: false
`
	unknownJourney := `  - stableId: "auth_401"
    journey: "wormhole"
    setup: "s"
    teardown: "t"
    parallelism: "p"
    expectedRequest: "req"
    expectedResponseOrTermination: "resp"
    evidence: "ev"
    noFirstPartyInterception: true
`
	duplicateID := validProfileBody + validProfileBody

	cases := []struct {
		name    string
		body    string
		wantErr error
	}{
		{"missing setup field", missingSetup, ErrMissingField},
		{"blank evidence field", blankEvidence, ErrMissingField},
		{"omitted no-first-party-interception field", omittedInterception, ErrMissingField},
		{"declares first-party interception", declaresInterception, ErrFirstPartyInterception},
		{"unknown journey", unknownJourney, ErrUnknownJourney},
		{"duplicate stableId", duplicateID, ErrDuplicateStableID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRegistry([]byte(regYAMLWith("v1", "test", "forbidden", tc.body)))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ParseRegistry(%s) error = %v; want errors.Is %v", tc.name, err, tc.wantErr)
			}
		})
	}

	// Adversarial: an empty profile set fails closed.
	t.Run("empty profile set rejected", func(t *testing.T) {
		if _, err := ParseRegistry([]byte(regYAMLWith("v1", "test", "forbidden", ""))); !errors.Is(err, ErrEmptyRegistry) {
			t.Fatalf("ParseRegistry(empty profiles) error = %v; want errors.Is ErrEmptyRegistry", err)
		}
	})

	// Adversarial: the closed schema rejects an unknown field (strict decode).
	t.Run("unknown field rejected by closed schema", func(t *testing.T) {
		bodyWithUnknownField := `  - stableId: "auth_401"
    journey: "assistant"
    setup: "s"
    teardown: "t"
    parallelism: "p"
    expectedRequest: "req"
    expectedResponseOrTermination: "resp"
    evidence: "ev"
    noFirstPartyInterception: true
    bogusField: "not in the closed schema"
`
		if _, err := ParseRegistry([]byte(regYAMLWith("v1", "test", "forbidden", bodyWithUnknownField))); err == nil {
			t.Fatalf("ParseRegistry accepted an unknown field; want a strict-decode rejection")
		}
	})
}
