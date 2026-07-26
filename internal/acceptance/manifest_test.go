package acceptance

import (
	"testing"
)

// journeyIndex returns the index of the journey with id in m, or -1.
func journeyIndex(m ProductJourneyManifest, id string) int {
	for i := range m.Journeys {
		if m.Journeys[i].ID == id {
			return i
		}
	}
	return -1
}

// wantContractCode asserts err is a *ContractError carrying want, and that want
// is a registered closed code (so a canary can never pass by inventing a code).
func wantContractCode(t *testing.T, err error, want FailureCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("want *ContractError %q, got nil", want)
	}
	ce, ok := err.(*ContractError)
	if !ok {
		t.Fatalf("want *ContractError, got %T: %v", err, err)
	}
	if ce.Code != want {
		t.Fatalf("contract code = %q; want %q (reason: %s)", ce.Code, want, ce.Reason)
	}
	reg, rerr := DefaultFailureRegistry()
	if rerr != nil {
		t.Fatalf("DefaultFailureRegistry() error = %v", rerr)
	}
	if _, known := reg.LookupFailure(ce.Code); !known {
		t.Fatalf("contract code %q is not a registered closed code", ce.Code)
	}
}

// TestManifestRequiresEveryDeclaredJourneyDependencyAndAssertion is TP-102-01-01.
// It proves the canonical manifest compiles and that each independent adversarial
// mutation — removing a required journey, dropping a dependency, dropping an
// assertion, using implicit requiredness, an unknown enum, an unregistered
// failure code, an unresolvable timeout reference, or a health-only required
// journey — independently fails compilation with a closed contract code. A
// permissive compiler (one that returned a policy for any of these) would fail
// every adversarial subtest, so the canaries are real.
func TestManifestRequiresEveryDeclaredJourneyDependencyAndAssertion(t *testing.T) {
	config := DefaultPolicyConfig()

	t.Run("canonical manifest compiles and covers every group", func(t *testing.T) {
		policy, err := CanonicalProductJourneyManifest().Compile(config, nil)
		if err != nil {
			t.Fatalf("Compile(canonical) error = %v; want nil", err)
		}
		for _, g := range ClosedJourneyGroups() {
			if !policy.CoversGroup(g) {
				t.Errorf("compiled policy does not cover group %q", g)
			}
		}
		if got := len(policy.Journeys); got != len(canonicalRequiredJourneyIDs) {
			t.Errorf("compiled policy has %d journeys; want %d", got, len(canonicalRequiredJourneyIDs))
		}
		for _, id := range canonicalRequiredJourneyIDs {
			if _, ok := policy.Journey(id); !ok {
				t.Errorf("compiled policy is missing canonical journey %q", id)
			}
		}
	})

	adversarial := []struct {
		name     string
		mutate   func(m *ProductJourneyManifest)
		wantCode FailureCode
	}{
		{
			name:     "removing a required journey fails",
			mutate:   func(m *ProductJourneyManifest) { m.Journeys = append(m.Journeys[:1], m.Journeys[2:]...) }, // drop search.read
			wantCode: CodeMissingJourney,
		},
		{
			name: "dropping a dependency from a dependency-blocked journey fails",
			mutate: func(m *ProductJourneyManifest) {
				i := journeyIndex(*m, "assistant.grounded-read")
				m.Journeys[i].Dependencies = nil
			},
			wantCode: CodeMalformed,
		},
		{
			name: "a declared dependency with an empty packet fails",
			mutate: func(m *ProductJourneyManifest) {
				i := journeyIndex(*m, "search.read")
				m.Journeys[i].Dependencies = []DependencyRef{{Packet: "", EvidenceClass: "certified-current-journey"}}
			},
			wantCode: CodeMalformed,
		},
		{
			name: "dropping an accessibility assertion from a browser journey fails",
			mutate: func(m *ProductJourneyManifest) {
				i := journeyIndex(*m, "search.read")
				m.Journeys[i].Assertions.Accessibility = nil
			},
			wantCode: CodeMalformed,
		},
		{
			name: "dropping the status assertion fails",
			mutate: func(m *ProductJourneyManifest) {
				i := journeyIndex(*m, "digest.current-read")
				m.Journeys[i].Assertions.Status = nil
			},
			wantCode: CodeMalformed,
		},
		{
			name: "implicit (unset) requiredness fails",
			mutate: func(m *ProductJourneyManifest) {
				i := journeyIndex(*m, "digest.current-read")
				m.Journeys[i].Requiredness = ""
			},
			wantCode: CodeMalformed,
		},
		{
			name: "an unknown group enum fails",
			mutate: func(m *ProductJourneyManifest) {
				i := journeyIndex(*m, "search.read")
				m.Journeys[i].Group = JourneyGroup("invented-group")
			},
			wantCode: CodeUnknownEnum,
		},
		{
			name: "an unregistered failure code fails",
			mutate: func(m *ProductJourneyManifest) {
				i := journeyIndex(*m, "search.read")
				m.Journeys[i].FailureCodes = append(m.Journeys[i].FailureCodes, "E102-JOURNEY-SEARCH-INVENTED")
			},
			wantCode: CodeUnknownEnum,
		},
		{
			name: "a failure code whose category does not match the group fails",
			mutate: func(m *ProductJourneyManifest) {
				i := journeyIndex(*m, "search.read")
				m.Journeys[i].FailureCodes = []FailureCode{"E102-JOURNEY-DIGEST-HTTP"}
			},
			wantCode: CodeUnknownEnum,
		},
		{
			name: "an unresolvable timeout reference fails",
			mutate: func(m *ProductJourneyManifest) {
				i := journeyIndex(*m, "search.read")
				m.Journeys[i].TimeoutRef = "acceptance.journeys.timeouts.does-not-exist"
			},
			wantCode: CodeMalformed,
		},
		{
			name: "a health-only required journey fails",
			mutate: func(m *ProductJourneyManifest) {
				i := journeyIndex(*m, "digest.current-read")
				m.Journeys[i].Steps = []ManifestStep{
					{ID: "health", Plane: PlaneTelemetry, Method: "GET", Route: "/api/health", SideEffect: SideEffectTelemetryRead, ExpectedStatus: []int{200}},
				}
			},
			wantCode: CodeMalformed,
		},
		{
			name: "a mutating selector in a journey fails static compilation",
			mutate: func(m *ProductJourneyManifest) {
				i := journeyIndex(*m, "search.read")
				m.Journeys[i].Selectors = append(m.Journeys[i].Selectors, "delete-account-button")
			},
			wantCode: CodeUnsafeMutation,
		},
		{
			name: "an unsafe evidence field in a journey fails static compilation",
			mutate: func(m *ProductJourneyManifest) {
				i := journeyIndex(*m, "search.read")
				m.Journeys[i].EvidenceFields = append(m.Journeys[i].EvidenceFields, "query-text")
			},
			wantCode: CodeEvidenceUnsafe,
		},
	}

	for _, tc := range adversarial {
		t.Run(tc.name, func(t *testing.T) {
			m := CanonicalProductJourneyManifest() // fresh, independent copy per case
			tc.mutate(&m)
			_, err := m.Compile(config, nil)
			wantContractCode(t, err, tc.wantCode)
		})
	}
}
