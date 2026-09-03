package intelligence

// synthesis_health_test.go — REAL unit tests for the pure HEALTH-TRUTH mapping
// (BUG-004-004 SCOPE-04 slice). These exercise the actual DeriveSynthesisHealth
// code path with no mocks, no database, no clock, and no I/O — the mapping is a
// pure function, so the tests assert its exact behaviour directly.
//
// The central product contract proven here: a synthesis that FAILED to persist
// (write failure), only PARTIALLY persisted, or FAILED its post-commit read-back
// gate (orphan / mismatch / read error / no read-back) can NEVER report
// healthy or persisted, and can NEVER map to /api/health intelligence status
// "up".

import "testing"

// --- Operator-required truth #1: persisted + read-back-OK -> healthy/persisted -

func TestDeriveSynthesisHealth_CommittedReadBackOKFull_IsHealthyPersisted(t *testing.T) {
	got := DeriveSynthesisHealth(SynthesisPersistenceOutcome{
		Phase:    PhaseCommitted,
		ReadBack: ReadBackOK,
		Output:   OutputKindFull,
		Stale:    false,
	})
	if got.State != SynthesisReadyCurrent {
		t.Errorf("State = %q, want %q", got.State, SynthesisReadyCurrent)
	}
	if !got.Persisted {
		t.Error("Persisted = false, want true for a read-back-verified full output")
	}
	if !got.Healthy {
		t.Error("Healthy = false, want true for a fresh read-back-verified full output")
	}
	if got.IntelligenceStatus != "up" {
		t.Errorf("IntelligenceStatus = %q, want \"up\"", got.IntelligenceStatus)
	}
}

func TestDeriveSynthesisHealth_CommittedReadBackOKQuiet_IsHealthyPersisted(t *testing.T) {
	got := DeriveSynthesisHealth(SynthesisPersistenceOutcome{
		Phase:    PhaseCommitted,
		ReadBack: ReadBackOK,
		Output:   OutputKindQuiet,
		Stale:    false,
	})
	if got.State != SynthesisReadyQuiet {
		t.Errorf("State = %q, want %q", got.State, SynthesisReadyQuiet)
	}
	if !got.Persisted {
		t.Error("Persisted = false, want true for a read-back-verified quiet output")
	}
	if !got.Healthy {
		t.Error("Healthy = false, want true for a fresh read-back-verified quiet output")
	}
	if got.IntelligenceStatus != "up" {
		t.Errorf("IntelligenceStatus = %q, want \"up\"", got.IntelligenceStatus)
	}
}

// --- Operator-required truth #2: write-failure -> NOT healthy (degraded/failed) -

func TestDeriveSynthesisHealth_WriteFailure_IsNeverHealthy(t *testing.T) {
	got := DeriveSynthesisHealth(SynthesisPersistenceOutcome{
		Phase:                  PhaseWriteFailed,
		HasPriorVerifiedOutput: false,
	})
	if got.State != SynthesisFailedWithoutOutput {
		t.Errorf("State = %q, want %q", got.State, SynthesisFailedWithoutOutput)
	}
	assertNeverHealthy(t, "write failure without prior output", got)
}

func TestDeriveSynthesisHealth_WriteFailureWithPrior_IsNeverHealthy(t *testing.T) {
	// A prior verified output MUST NOT rescue a failed latest attempt into a
	// healthy/persisted verdict — it only selects the failed-with-prior state.
	got := DeriveSynthesisHealth(SynthesisPersistenceOutcome{
		Phase:                  PhaseWriteFailed,
		HasPriorVerifiedOutput: true,
	})
	if got.State != SynthesisFailedWithPriorOutput {
		t.Errorf("State = %q, want %q", got.State, SynthesisFailedWithPriorOutput)
	}
	assertNeverHealthy(t, "write failure with prior output", got)
}

// --- Operator-required truth #3: partial / orphan -> NOT healthy ---------------

func TestDeriveSynthesisHealth_PartialOutput_IsNeverHealthy(t *testing.T) {
	got := DeriveSynthesisHealth(SynthesisPersistenceOutcome{
		Phase:    PhaseCommitted,
		ReadBack: ReadBackOK,
		Output:   OutputKindPartial,
	})
	if got.State != SynthesisDegradedPartial {
		t.Errorf("State = %q, want %q", got.State, SynthesisDegradedPartial)
	}
	assertNeverHealthy(t, "committed partial output", got)
}

func TestDeriveSynthesisHealth_OrphanReadBackMismatch_IsNeverHealthy(t *testing.T) {
	// "orphan" == a commit whose post-commit read-back returned an inconsistent
	// aggregate (e.g. orphaned citations / a missing insight row).
	got := DeriveSynthesisHealth(SynthesisPersistenceOutcome{
		Phase:    PhaseCommitted,
		ReadBack: ReadBackMismatch,
		Output:   OutputKindFull, // even a "full" claim is void without a verifying read-back
	})
	if got.State != SynthesisReadDegraded {
		t.Errorf("State = %q, want %q", got.State, SynthesisReadDegraded)
	}
	assertNeverHealthy(t, "committed with orphan/mismatch read-back", got)
}

// --- Operator-required truth #4: read-back-mismatch/error/absent -> NOT healthy -

func TestDeriveSynthesisHealth_ReadBackErrorAfterCommit_IsNeverHealthy(t *testing.T) {
	got := DeriveSynthesisHealth(SynthesisPersistenceOutcome{
		Phase:    PhaseCommitted,
		ReadBack: ReadBackError,
		Output:   OutputKindFull,
	})
	if got.State != SynthesisReadDegraded {
		t.Errorf("State = %q, want %q", got.State, SynthesisReadDegraded)
	}
	assertNeverHealthy(t, "committed with read-back error", got)
}

func TestDeriveSynthesisHealth_CommitWithoutReadBack_IsNeverPersisted(t *testing.T) {
	// The read-back gate is MANDATORY: a commit that was never re-read is not
	// `persisted` ("only that successful read-back yields persisted").
	got := DeriveSynthesisHealth(SynthesisPersistenceOutcome{
		Phase:    PhaseCommitted,
		ReadBack: ReadBackNotAttempted,
		Output:   OutputKindFull,
	})
	if got.State != SynthesisReadDegraded {
		t.Errorf("State = %q, want %q", got.State, SynthesisReadDegraded)
	}
	assertNeverHealthy(t, "committed but read-back never attempted", got)
}

func TestDeriveSynthesisHealth_ReadBackOKButNoOutputKind_IsNeverHealthy(t *testing.T) {
	// A supposedly-OK read-back that yields no output kind is itself an
	// inconsistency — fail closed rather than green.
	got := DeriveSynthesisHealth(SynthesisPersistenceOutcome{
		Phase:    PhaseCommitted,
		ReadBack: ReadBackOK,
		Output:   OutputKindNone,
	})
	if got.State != SynthesisReadDegraded {
		t.Errorf("State = %q, want %q", got.State, SynthesisReadDegraded)
	}
	assertNeverHealthy(t, "read-back OK but no output kind", got)
}

// --- Stale / running / never-run truthfulness ----------------------------------

func TestDeriveSynthesisHealth_StaleVerifiedOutput_IsDegradedStaleNotHealthy(t *testing.T) {
	// A verified output that is past its freshness budget IS persisted (it did
	// commit and read back), but it is NOT healthy and NOT "up".
	for _, kind := range []SynthesisOutputKind{OutputKindFull, OutputKindQuiet} {
		got := DeriveSynthesisHealth(SynthesisPersistenceOutcome{
			Phase:    PhaseCommitted,
			ReadBack: ReadBackOK,
			Output:   kind,
			Stale:    true,
		})
		if got.State != SynthesisDegradedStale {
			t.Errorf("kind %q: State = %q, want %q", kind, got.State, SynthesisDegradedStale)
		}
		if !got.Persisted {
			t.Errorf("kind %q: Persisted = false, want true (a stale output still persisted)", kind)
		}
		if got.Healthy {
			t.Errorf("kind %q: Healthy = true, want false for a stale output", kind)
		}
		if got.IntelligenceStatus != "stale" {
			t.Errorf("kind %q: IntelligenceStatus = %q, want \"stale\"", kind, got.IntelligenceStatus)
		}
	}
}

func TestDeriveSynthesisHealth_Running_IsNeverHealthy(t *testing.T) {
	// "scheduler acceptance ... is never success": an in-flight run is not a
	// durable success, even if a prior verified output exists.
	for _, prior := range []bool{false, true} {
		got := DeriveSynthesisHealth(SynthesisPersistenceOutcome{
			Phase:                  PhaseRunning,
			HasPriorVerifiedOutput: prior,
		})
		if got.State != SynthesisRunning {
			t.Errorf("hasPrior=%v: State = %q, want %q", prior, got.State, SynthesisRunning)
		}
		assertNeverHealthy(t, "running", got)
	}
}

// --- Adversarial regression against the pre-fix "falsely healthy" mapping ------

// TestDeriveSynthesisHealth_Regression_NeverRunAndProbeErrorAreNeverUp encodes
// the EXACT inputs the pre-fix internal/api/health.go mapped to "up":
//
//	lastSynthesis.IsZero() || Year() < 2000  -> intelligenceStatus = "up"  (never run)
//	GetLastSynthesisTime returned an error   -> intelligenceStatus = "up"  (probe error)
//
// This test is adversarial: its expectation (NOT "up", NOT healthy, NOT
// persisted) is the OPPOSITE of the historical bug. If the health-truth mapping
// were reverted to the old behaviour — or a naive implementation treated
// "no output" / "cannot evaluate" as green — this test FAILS.
func TestDeriveSynthesisHealth_Regression_NeverRunAndProbeErrorAreNeverUp(t *testing.T) {
	cases := []struct {
		name      string
		outcome   SynthesisPersistenceOutcome
		wantState SynthesisHealthState
	}{
		{
			name:      "never-run (pre-fix mapped the 1970 epoch sentinel to up)",
			outcome:   SynthesisPersistenceOutcome{Phase: PhaseNoRun},
			wantState: SynthesisNeverRun,
		},
		{
			name:      "probe-error (pre-fix mapped a freshness-probe error to up)",
			outcome:   SynthesisPersistenceOutcome{Phase: PhaseProbeError},
			wantState: SynthesisReadDegraded,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveSynthesisHealth(tc.outcome)
			if got.State != tc.wantState {
				t.Errorf("State = %q, want %q", got.State, tc.wantState)
			}
			if got.IntelligenceStatus == "up" {
				t.Fatalf("REGRESSION: IntelligenceStatus = \"up\" for %s — the pre-fix falsely-healthy mapping has returned", tc.name)
			}
			if got.Healthy {
				t.Fatalf("REGRESSION: Healthy = true for %s — failed/absent persistence must never be healthy", tc.name)
			}
			if got.Persisted {
				t.Fatalf("REGRESSION: Persisted = true for %s — nothing was durably persisted", tc.name)
			}
		})
	}
}

// --- Exhaustive closed-vocabulary mapping lock ---------------------------------

func TestDeriveSynthesisHealth_ClosedVocabularyMapping(t *testing.T) {
	cases := []struct {
		name      string
		outcome   SynthesisPersistenceOutcome
		wantState SynthesisHealthState
		wantPers  bool
		wantHealt bool
		wantStat  string
	}{
		{"never-run", SynthesisPersistenceOutcome{Phase: PhaseNoRun}, SynthesisNeverRun, false, false, "down"},
		{"probe-error", SynthesisPersistenceOutcome{Phase: PhaseProbeError}, SynthesisReadDegraded, false, false, "down"},
		{"running", SynthesisPersistenceOutcome{Phase: PhaseRunning}, SynthesisRunning, false, false, "down"},
		{"write-failed no prior", SynthesisPersistenceOutcome{Phase: PhaseWriteFailed}, SynthesisFailedWithoutOutput, false, false, "down"},
		{"write-failed with prior", SynthesisPersistenceOutcome{Phase: PhaseWriteFailed, HasPriorVerifiedOutput: true}, SynthesisFailedWithPriorOutput, false, false, "down"},
		{"committed readback-mismatch", SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackMismatch, Output: OutputKindFull}, SynthesisReadDegraded, false, false, "down"},
		{"committed readback-error", SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackError, Output: OutputKindFull}, SynthesisReadDegraded, false, false, "down"},
		{"committed readback-not-attempted", SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackNotAttempted, Output: OutputKindFull}, SynthesisReadDegraded, false, false, "down"},
		{"committed ok partial", SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindPartial}, SynthesisDegradedPartial, false, false, "down"},
		{"committed ok none", SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindNone}, SynthesisReadDegraded, false, false, "down"},
		{"committed ok full fresh", SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindFull}, SynthesisReadyCurrent, true, true, "up"},
		{"committed ok quiet fresh", SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindQuiet}, SynthesisReadyQuiet, true, true, "up"},
		{"committed ok full stale", SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindFull, Stale: true}, SynthesisDegradedStale, true, false, "stale"},
		{"committed ok quiet stale", SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindQuiet, Stale: true}, SynthesisDegradedStale, true, false, "stale"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveSynthesisHealth(tc.outcome)
			if got.State != tc.wantState {
				t.Errorf("State = %q, want %q", got.State, tc.wantState)
			}
			if got.Persisted != tc.wantPers {
				t.Errorf("Persisted = %v, want %v", got.Persisted, tc.wantPers)
			}
			if got.Healthy != tc.wantHealt {
				t.Errorf("Healthy = %v, want %v", got.Healthy, tc.wantHealt)
			}
			if got.IntelligenceStatus != tc.wantStat {
				t.Errorf("IntelligenceStatus = %q, want %q", got.IntelligenceStatus, tc.wantStat)
			}
		})
	}
}

func TestAggregateRequiredSynthesisHealth_FailsClosedAcrossCadences(t *testing.T) {
	healthyFull := SynthesisPersistenceOutcome{
		Phase:    PhaseCommitted,
		ReadBack: ReadBackOK,
		Output:   OutputKindFull,
	}
	healthyQuiet := SynthesisPersistenceOutcome{
		Phase:    PhaseCommitted,
		ReadBack: ReadBackOK,
		Output:   OutputKindQuiet,
	}
	nonGreen := []struct {
		name       string
		outcome    SynthesisPersistenceOutcome
		wantStatus string
	}{
		{name: "never-run", outcome: SynthesisPersistenceOutcome{Phase: PhaseNoRun}, wantStatus: "down"},
		{name: "running", outcome: SynthesisPersistenceOutcome{Phase: PhaseRunning}, wantStatus: "down"},
		{name: "partial", outcome: SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindPartial}, wantStatus: "down"},
		{name: "failed", outcome: SynthesisPersistenceOutcome{Phase: PhaseWriteFailed}, wantStatus: "down"},
		{name: "read-degraded", outcome: SynthesisPersistenceOutcome{Phase: PhaseProbeError}, wantStatus: "down"},
		{name: "stale", outcome: SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindFull, Stale: true}, wantStatus: "stale"},
	}

	for _, testCase := range nonGreen {
		t.Run("healthy daily cannot mask weekly "+testCase.name, func(t *testing.T) {
			got := AggregateRequiredSynthesisHealth([RequiredSynthesisCadenceCount]SynthesisCadenceOutcome{
				{Cadence: CadenceDaily, Outcome: healthyFull},
				{Cadence: CadenceWeekly, Outcome: testCase.outcome},
			})
			if got.Healthy {
				t.Fatalf("aggregate Healthy = true with weekly %s; every required cadence must be green", testCase.name)
			}
			if got.IntelligenceStatus != testCase.wantStatus {
				t.Fatalf("aggregate IntelligenceStatus = %q with weekly %s, want %q", got.IntelligenceStatus, testCase.name, testCase.wantStatus)
			}
			if got.Cadences[0].Cadence != CadenceDaily || got.Cadences[1].Cadence != CadenceWeekly {
				t.Fatalf("bounded cadence results lost identity: %+v", got.Cadences)
			}
			if !got.Cadences[0].Health.Healthy || got.Cadences[1].Health.Healthy {
				t.Fatalf("per-cadence verdicts do not preserve healthy daily and non-green weekly: %+v", got.Cadences)
			}
		})
	}

	t.Run("hard down wins over stale", func(t *testing.T) {
		got := AggregateRequiredSynthesisHealth([RequiredSynthesisCadenceCount]SynthesisCadenceOutcome{
			{Cadence: CadenceDaily, Outcome: SynthesisPersistenceOutcome{Phase: PhaseCommitted, ReadBack: ReadBackOK, Output: OutputKindFull, Stale: true}},
			{Cadence: CadenceWeekly, Outcome: SynthesisPersistenceOutcome{Phase: PhaseWriteFailed}},
		})
		if got.IntelligenceStatus != "down" || got.Healthy {
			t.Fatalf("aggregate = %+v, want hard down to take precedence over stale", got)
		}
	})

	t.Run("daily and weekly green are up", func(t *testing.T) {
		got := AggregateRequiredSynthesisHealth([RequiredSynthesisCadenceCount]SynthesisCadenceOutcome{
			{Cadence: CadenceDaily, Outcome: healthyFull},
			{Cadence: CadenceWeekly, Outcome: healthyQuiet},
		})
		if !got.Healthy || got.IntelligenceStatus != "up" {
			t.Fatalf("aggregate = %+v, want healthy up when every required cadence is green", got)
		}
	})
}

// --- Cross-cutting invariants over the full outcome space ----------------------

// TestDeriveSynthesisHealth_Invariants enumerates the cartesian product of every
// outcome dimension and asserts the mapping's safety invariants hold for ALL of
// them — including combinations no single named test enumerates.
func TestDeriveSynthesisHealth_Invariants(t *testing.T) {
	phases := []PersistencePhase{PhaseNoRun, PhaseRunning, PhaseWriteFailed, PhaseCommitted, PhaseProbeError}
	readbacks := []ReadBackResult{ReadBackNotAttempted, ReadBackOK, ReadBackMismatch, ReadBackError}
	outputs := []SynthesisOutputKind{OutputKindNone, OutputKindFull, OutputKindQuiet, OutputKindPartial}
	bools := []bool{false, true}

	greenStates := map[SynthesisHealthState]bool{
		SynthesisReadyCurrent: true,
		SynthesisReadyQuiet:   true,
	}

	total := 0
	for _, ph := range phases {
		for _, rb := range readbacks {
			for _, out := range outputs {
				for _, stale := range bools {
					for _, prior := range bools {
						total++
						o := SynthesisPersistenceOutcome{
							Phase: ph, ReadBack: rb, Output: out,
							Stale: stale, HasPriorVerifiedOutput: prior,
						}
						got := DeriveSynthesisHealth(o)

						// Invariant 1: "up" iff Healthy.
						if (got.IntelligenceStatus == "up") != got.Healthy {
							t.Fatalf("outcome %+v: IntelligenceStatus==up (%v) must equal Healthy (%v)", o, got.IntelligenceStatus == "up", got.Healthy)
						}
						// Invariant 2: Healthy implies Persisted.
						if got.Healthy && !got.Persisted {
							t.Fatalf("outcome %+v: Healthy without Persisted", o)
						}
						// Invariant 3: Healthy only in a green state.
						if got.Healthy && !greenStates[got.State] {
							t.Fatalf("outcome %+v: Healthy in non-green state %q", o, got.State)
						}
						// Invariant 4: a write failure, unreadable probe, or
						// never-run can NEVER be persisted, healthy, or up — the
						// core anti-"falsely-healthy" guarantee.
						if ph == PhaseWriteFailed || ph == PhaseProbeError || ph == PhaseNoRun {
							if got.Persisted || got.Healthy || got.IntelligenceStatus == "up" {
								t.Fatalf("outcome %+v: failed/absent persistence reported as up/healthy/persisted (%+v)", o, got)
							}
						}
						// Invariant 5: a committed transaction whose read-back did
						// not verify OK is never persisted or healthy.
						if ph == PhaseCommitted && rb != ReadBackOK {
							if got.Persisted || got.Healthy {
								t.Fatalf("outcome %+v: unverified commit reported persisted/healthy (%+v)", o, got)
							}
						}
						// Invariant 6: a committed+read-back-OK PARTIAL output is
						// durable-but-degraded — never persisted, never healthy.
						if ph == PhaseCommitted && rb == ReadBackOK && out == OutputKindPartial {
							if got.Persisted || got.Healthy {
								t.Fatalf("outcome %+v: partial output reported persisted/healthy (%+v)", o, got)
							}
						}
						// Invariant 7: status is always one of the closed set.
						switch got.IntelligenceStatus {
						case "up", "stale", "down":
						default:
							t.Fatalf("outcome %+v: unknown IntelligenceStatus %q", o, got.IntelligenceStatus)
						}
					}
				}
			}
		}
	}
	t.Logf("verified invariants over %d outcome combinations", total)
}

// assertNeverHealthy is the shared guard for every non-green expectation.
func assertNeverHealthy(t *testing.T, ctx string, got SynthesisHealth) {
	t.Helper()
	if got.Persisted {
		t.Errorf("%s: Persisted = true, want false", ctx)
	}
	if got.Healthy {
		t.Errorf("%s: Healthy = true, want false", ctx)
	}
	if got.IntelligenceStatus == "up" {
		t.Errorf("%s: IntelligenceStatus = \"up\", want not-up", ctx)
	}
}
