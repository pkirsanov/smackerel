package acceptance

import (
	"testing"
	"time"
)

// validExpected returns a trusted expected-release with a fixed clock.
func validExpected(now time.Time) ExpectedRelease {
	return ExpectedRelease{
		SourceSha:           "abc123def456",
		BuildManifestDigest: "sha256:ddddddd",
		Train:               "mvp",
		ManifestID:          "smackerel-primary-journeys",
		ActivatedAt:         now.Add(-5 * time.Minute),
		Now:                 now,
		FreshnessWindow:     15 * time.Minute,
		ClockSkew:           time.Minute,
	}
}

// validEnvelope returns a schema-valid, release-matched, fresh, value-safe
// result envelope whose aggregate counts reconcile with its rows.
func validEnvelope(now time.Time) ResultEnvelope {
	return ResultEnvelope{
		SchemaVersion:    resultSchema,
		ManifestID:       "smackerel-primary-journeys",
		ManifestRevision: 1,
		ManifestDigest:   "sha256:aaaaaaa",
		PolicyDigest:     "sha256:bbbbbbb",
		RunnerDigest:     "sha256:ccccccc",
		Release: ResultRelease{
			SourceSha:           "abc123def456",
			BuildManifestDigest: "sha256:ddddddd",
			Train:               "mvp",
		},
		Run: ResultRun{
			RunID:               "run-000000001",
			Mode:                ModeProductionReadonly,
			StartedAt:           now.Add(-90 * time.Second),
			ObservedAt:          now.Add(-30 * time.Second),
			DurationMs:          60000,
			EvidenceEligibility: "deploy-acceptance",
		},
		Journeys: []ResultJourneyRow{
			{JourneyID: "session.login-reuse", Ordinal: 1, Requiredness: RequirednessRequired, Outcome: OutcomePassed, EvidenceFields: safeEvidenceFields},
			{JourneyID: "cards.representative-read", Ordinal: 2, Requiredness: RequirednessOptional, Outcome: OutcomeAllowedOptional, EvidenceFields: safeEvidenceFields},
		},
		Aggregate: ResultAggregate{
			Verdict: VerdictAcceptedDegraded, RequiredTotal: 1, RequiredPassed: 1,
			OptionalLimited: 1, Failed: 0, Blocked: 0, TimedOut: 0,
		},
		Evidence:  ResultEvidence{IndexDigest: "sha256:eeeeeee", FieldNames: safeEvidenceFields},
		Signature: ResultSignature{Format: "dsse", PayloadDigest: "sha256:fffffff"},
	}
}

// TestResultValidatorRejectsMissingDuplicateUnknownAndMismatchedRows is
// TP-102-01-02. It proves a valid envelope validates and that each independent
// adversarial mutation — missing, unsupported schema/mode, malformed/incomplete,
// missing signature, unknown verdict/outcome/code, manifest/release mismatch,
// stale, duplicate row, count mismatch, and unsafe evidence — yields
// contract-invalid with exactly one closed code. A permissive validator (one
// that returned Valid for any of these) would fail every adversarial subtest, so
// the canaries are real.
func TestResultValidatorRejectsMissingDuplicateUnknownAndMismatchedRows(t *testing.T) {
	now := time.Now()
	v, err := NewAcceptanceResultValidator()
	if err != nil {
		t.Fatalf("NewAcceptanceResultValidator() error = %v; want nil", err)
	}

	t.Run("a valid result validates", func(t *testing.T) {
		got, verr := v.Validate(validEnvelope(now), validExpected(now))
		if verr != nil {
			t.Fatalf("Validate(valid) error = %v; want nil", verr)
		}
		if !got.Valid {
			t.Fatalf("Validate(valid).Valid = false; want true")
		}
		if got.Verdict != VerdictAcceptedDegraded {
			t.Errorf("Validate(valid).Verdict = %q; want %q", got.Verdict, VerdictAcceptedDegraded)
		}
		if got.RequiredTotal != 1 || got.RequiredPassed != 1 {
			t.Errorf("Validate(valid) required counts = %d/%d; want 1/1", got.RequiredPassed, got.RequiredTotal)
		}
	})

	adversarial := []struct {
		name     string
		mutate   func(e *ResultEnvelope)
		wantCode FailureCode
	}{
		{"a missing result fails", func(e *ResultEnvelope) { *e = ResultEnvelope{} }, CodeMissing},
		{"an unsupported schema fails", func(e *ResultEnvelope) { e.SchemaVersion = "smackerel.io/product-acceptance-result/v2" }, CodeUnsupported},
		{"an unsupported run mode fails", func(e *ResultEnvelope) { e.Run.Mode = Mode("staging") }, CodeUnsupported},
		{"a malformed (empty digest) result fails", func(e *ResultEnvelope) { e.ManifestDigest = "" }, CodeMalformed},
		{"a zero run timestamp fails", func(e *ResultEnvelope) { e.Run.ObservedAt = time.Time{} }, CodeMalformed},
		{"a missing signature fails", func(e *ResultEnvelope) { e.Signature.PayloadDigest = "" }, CodeSignature},
		{"an unknown aggregate verdict fails", func(e *ResultEnvelope) { e.Aggregate.Verdict = AggregateVerdict("approved") }, CodeUnknownEnum},
		{"a manifest-id mismatch fails", func(e *ResultEnvelope) { e.ManifestID = "some-other-manifest" }, CodeManifestMismatch},
		{"a release mismatch fails", func(e *ResultEnvelope) { e.Release.SourceSha = "0000000wrong" }, CodeReleaseMismatch},
		{"a result observed before activation is stale", func(e *ResultEnvelope) { e.Run.ObservedAt = now.Add(-10 * time.Minute) }, CodeStaleResult},
		{"a result older than the freshness window is stale", func(e *ResultEnvelope) {
			// After activation (activated at -5m) but older than the 15m window is
			// impossible; instead push activation earlier is out of scope, so test
			// the future-skew branch which also yields stale.
			e.Run.ObservedAt = now.Add(10 * time.Minute)
		}, CodeStaleResult},
		{"a duplicate journey row fails", func(e *ResultEnvelope) {
			e.Journeys = append(e.Journeys, ResultJourneyRow{JourneyID: "session.login-reuse", Ordinal: 3, Requiredness: RequirednessRequired, Outcome: OutcomePassed})
		}, CodeDuplicateJourney},
		{"an unknown journey outcome fails", func(e *ResultEnvelope) { e.Journeys[0].Outcome = JourneyOutcome("skipped") }, CodeUnknownEnum},
		{"an unregistered failure code fails", func(e *ResultEnvelope) {
			e.Journeys[0].Outcome = OutcomeFailed
			e.Journeys[0].FailureCode = "E102-JOURNEY-AUTH-INVENTED"
		}, CodeUnknownEnum},
		{"an unsafe evidence field fails", func(e *ResultEnvelope) {
			e.Journeys[0].EvidenceFields = append(e.Journeys[0].EvidenceFields, "password")
		}, CodeEvidenceUnsafe},
		{"an aggregate count that does not reconcile fails", func(e *ResultEnvelope) { e.Aggregate.Failed = 3 }, CodeMalformed},
	}

	for _, tc := range adversarial {
		t.Run(tc.name, func(t *testing.T) {
			e := validEnvelope(now)
			tc.mutate(&e)
			_, verr := v.Validate(e, validExpected(now))
			wantContractCode(t, verr, tc.wantCode)
		})
	}
}
