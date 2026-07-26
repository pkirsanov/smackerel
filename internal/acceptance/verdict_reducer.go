// verdict_reducer.go reduces a compiled policy, the prerequisite results, and
// the per-journey outcomes into one deterministic aggregate verdict. It is
// fail-closed: an allowed-empty/quiet/optional/degraded outcome is honored ONLY
// when the compiled policy for that journey explicitly permits it; an absent or
// ambiguous policy, a required journey that never produced product behavior, or
// a health-only "success" never yields `accepted`. Infrastructure health cannot
// promote a verdict.
//
// The reduction order and precedence follow design.md "## Aggregate Verdict":
// contract/identity -> safety -> blocked prerequisite -> timeout -> rejection ->
// accepted variants. The closed verdict/outcome vocabularies come from
// failure_registry.go.

package acceptance

// PrerequisiteResult is one dependency/prerequisite observation feeding the
// reduction. GatesJourney names the journey this prerequisite gates.
type PrerequisiteResult struct {
	Ref          string
	Ready        bool
	Current      bool
	GatesJourney string
}

// JourneyResult is one per-journey outcome feeding the reduction.
type JourneyResult struct {
	JourneyID   string
	Outcome     JourneyOutcome
	FailureCode FailureCode
}

// AggregateResult is the deterministic reduction output.
type AggregateResult struct {
	Verdict         AggregateVerdict
	Code            FailureCode // set for contract-invalid / blocked / rejected reasons
	RequiredTotal   int
	RequiredPassed  int
	OptionalLimited int
	Failed          int
	Blocked         int
	TimedOut        int
}

// VerdictReducer reduces journey/prerequisite results against a compiled policy.
type VerdictReducer struct{}

// Reduce returns the deterministic aggregate verdict. It never returns
// `accepted` unless every required journey has an explicit passing or
// policy-permitted allowed outcome and every optional row is policy-permitted.
func (VerdictReducer) Reduce(policy CompiledAcceptancePolicy, prerequisites []PrerequisiteResult, journeys []JourneyResult) AggregateResult {
	// 1. Contract/identity: a policy with no journeys is ambiguous and fails
	// closed.
	if len(policy.Journeys) == 0 {
		return AggregateResult{Verdict: VerdictContractInvalid, Code: CodeMissingJourney}
	}

	seen := make(map[string]bool, len(journeys))
	byJourney := make(map[string]JourneyResult, len(journeys))
	for _, jr := range journeys {
		// Every result must map to a compiled journey.
		cj, ok := policy.Journey(jr.JourneyID)
		if !ok {
			return AggregateResult{Verdict: VerdictContractInvalid, Code: CodeMissingJourney}
		}
		if seen[jr.JourneyID] {
			return AggregateResult{Verdict: VerdictContractInvalid, Code: CodeDuplicateJourney}
		}
		seen[jr.JourneyID] = true
		if !IsClosedJourneyOutcome(jr.Outcome) {
			return AggregateResult{Verdict: VerdictContractInvalid, Code: CodeUnknownEnum}
		}
		// 2. Safety: an unsafe-mutation / unsafe-evidence code dominates.
		if jr.FailureCode == CodeUnsafeMutation || jr.FailureCode == CodeEvidenceUnsafe {
			return AggregateResult{Verdict: VerdictContractInvalid, Code: jr.FailureCode}
		}
		// An allowed-limitation (or a claimed pass) is honored ONLY when the
		// compiled policy for this journey explicitly permits that outcome. No
		// rule means no pass.
		if jr.Outcome == OutcomePassed || isAllowedLimitation(jr.Outcome) {
			if !cj.AllowedOutcomes[jr.Outcome] {
				return AggregateResult{Verdict: VerdictContractInvalid, Code: CodeUnknownEnum}
			}
		}
		byJourney[jr.JourneyID] = jr
	}

	// Every REQUIRED journey must have a result row (completeness).
	for _, cj := range policy.Journeys {
		if cj.Requiredness == RequirednessRequired {
			if _, ok := byJourney[cj.ID]; !ok {
				return AggregateResult{Verdict: VerdictContractInvalid, Code: CodeMissingJourney}
			}
		}
	}

	// 3. Blocked prerequisite: a REQUIRED journey that produced no product
	// behavior because a gating prerequisite is not ready/current.
	for _, pr := range prerequisites {
		if pr.Ready && pr.Current {
			continue
		}
		cj, ok := policy.Journey(pr.GatesJourney)
		if !ok {
			continue
		}
		if cj.Requiredness != RequirednessRequired {
			continue
		}
		if jr, ok := byJourney[cj.ID]; ok && (jr.Outcome == OutcomeNotEvaluated || jr.Outcome == OutcomeBlocked) {
			return AggregateResult{Verdict: VerdictBlockedPrerequisite, Code: CodeMissing}
		}
	}

	counts := reduceCounts(policy, byJourney)

	// 4. Overall timeout: the whole required set timed out.
	if counts.RequiredTotal > 0 && counts.TimedOut >= counts.RequiredTotal && counts.requiredTimedOut == counts.RequiredTotal {
		return AggregateResult{Verdict: VerdictTimedOut, Code: CodeOverallTimeout, RequiredTotal: counts.RequiredTotal,
			RequiredPassed: counts.RequiredPassed, OptionalLimited: counts.OptionalLimited, Failed: counts.Failed, Blocked: counts.Blocked, TimedOut: counts.TimedOut}
	}

	// 5. Rejection: any required journey failed or timed out, or any required
	// journey produced no honest passing/allowed outcome (health-only / not
	// evaluated never accepts).
	for _, cj := range policy.Journeys {
		if cj.Requiredness != RequirednessRequired {
			continue
		}
		jr := byJourney[cj.ID]
		if jr.Outcome == OutcomeFailed || jr.Outcome == OutcomeTimedOut {
			return rejectResult(counts, jr.FailureCode)
		}
		if jr.Outcome != OutcomePassed && !isAllowedLimitation(jr.Outcome) {
			// not-evaluated / blocked required journey that was not caught as a
			// blocked-prerequisite: fail closed, never accept.
			return rejectResult(counts, CodeMissingJourney)
		}
	}

	// 6. Accepted variants: all required journeys pass/allowed; degrade if any
	// optional row is an explicit allowed limitation.
	verdict := VerdictAccepted
	if counts.OptionalLimited > 0 || counts.requiredLimited > 0 {
		verdict = VerdictAcceptedDegraded
	}
	return AggregateResult{
		Verdict: verdict, RequiredTotal: counts.RequiredTotal, RequiredPassed: counts.RequiredPassed,
		OptionalLimited: counts.OptionalLimited, Failed: counts.Failed, Blocked: counts.Blocked, TimedOut: counts.TimedOut,
	}
}

type reductionCounts struct {
	RequiredTotal    int
	RequiredPassed   int
	OptionalLimited  int
	Failed           int
	Blocked          int
	TimedOut         int
	requiredTimedOut int
	requiredLimited  int
}

func reduceCounts(policy CompiledAcceptancePolicy, byJourney map[string]JourneyResult) reductionCounts {
	var c reductionCounts
	for _, cj := range policy.Journeys {
		jr, ran := byJourney[cj.ID]
		switch cj.Requiredness {
		case RequirednessRequired:
			c.RequiredTotal++
			if ran && (jr.Outcome == OutcomePassed || isAllowedLimitation(jr.Outcome)) {
				c.RequiredPassed++
			}
			if ran && isAllowedLimitation(jr.Outcome) {
				c.requiredLimited++
			}
			if ran && jr.Outcome == OutcomeTimedOut {
				c.requiredTimedOut++
			}
		case RequirednessOptional:
			if ran && isAllowedLimitation(jr.Outcome) {
				c.OptionalLimited++
			}
		}
		if ran {
			switch jr.Outcome {
			case OutcomeFailed:
				c.Failed++
			case OutcomeBlocked:
				c.Blocked++
			case OutcomeTimedOut:
				c.TimedOut++
			}
		}
	}
	return c
}

func rejectResult(c reductionCounts, code FailureCode) AggregateResult {
	return AggregateResult{
		Verdict: VerdictRejected, Code: code, RequiredTotal: c.RequiredTotal, RequiredPassed: c.RequiredPassed,
		OptionalLimited: c.OptionalLimited, Failed: c.Failed, Blocked: c.Blocked, TimedOut: c.TimedOut,
	}
}
