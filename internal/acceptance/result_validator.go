// result_validator.go validates a product-acceptance result envelope
// (schema smackerel.io/product-acceptance-result/v1) WITHOUT rerunning any
// product logic. It is the fail-closed gate the product CLI and the operator
// adapter both apply before trusting an aggregate verdict: a result that is
// missing, malformed, incomplete, duplicated, stale, unsafe, unsupported, or
// release-mismatched is rejected as contract-invalid with exactly one closed
// E102-JOURNEY-CONTRACT-* code (manifest.go), and no row is ever ignored or
// guessed compatible.
//
// The closed vocabularies (verdicts, journey outcomes, failure codes) and the
// value-safe evidence scanner come from failure_registry.go and
// read_only_guard.go; this file never redefines them.

package acceptance

import (
	"strings"
	"time"
)

// ResultRelease is the release identity a result claims.
type ResultRelease struct {
	SourceSha           string
	BuildManifestDigest string
	Train               string
}

// ResultRun is the run metadata a result claims.
type ResultRun struct {
	RunID               string
	Mode                Mode
	StartedAt           time.Time
	ObservedAt          time.Time
	DurationMs          int64
	EvidenceEligibility string
}

// ResultJourneyRow is one per-journey result row. Every row is validated; a row
// with an unknown outcome, an unregistered code, or an unsafe evidence field
// invalidates the whole envelope.
type ResultJourneyRow struct {
	JourneyID      string
	Ordinal        int
	Requiredness   Requiredness
	Outcome        JourneyOutcome
	FailureCode    FailureCode // empty for a clean pass
	EvidenceFields []string
}

// ResultAggregate is the reduced aggregate the runner claims. The validator
// recomputes the counts from the rows and rejects any inconsistency.
type ResultAggregate struct {
	Verdict         AggregateVerdict
	RequiredTotal   int
	RequiredPassed  int
	OptionalLimited int
	Failed          int
	Blocked         int
	TimedOut        int
}

// ResultEvidence is the content-free evidence index a result carries.
type ResultEvidence struct {
	IndexDigest string
	FieldNames  []string
}

// ResultSignature is the detached signature over the result payload.
type ResultSignature struct {
	Format        string
	PayloadDigest string
}

// ResultEnvelope is the immutable value-safe result envelope.
type ResultEnvelope struct {
	SchemaVersion    string
	ManifestID       string
	ManifestRevision int
	ManifestDigest   string
	PolicyDigest     string
	RunnerDigest     string
	Release          ResultRelease
	Run              ResultRun
	Journeys         []ResultJourneyRow
	Aggregate        ResultAggregate
	Evidence         ResultEvidence
	Signature        ResultSignature
}

// ExpectedRelease is the trusted release identity and freshness policy the
// validator checks a result against.
type ExpectedRelease struct {
	SourceSha           string
	BuildManifestDigest string
	Train               string
	ManifestID          string
	// ActivatedAt is the moment the running release became active. A result
	// observed before this instant is stale (observed on a prior release).
	ActivatedAt time.Time
	// Now is the trusted current time.
	Now time.Time
	// FreshnessWindow bounds how old a result may be relative to Now.
	FreshnessWindow time.Duration
	// ClockSkew is the tolerated future skew for ObservedAt relative to Now.
	ClockSkew time.Duration
}

// ValidatedResult is the successful output of Validate: the trusted verdict and
// the count summary recomputed from the rows.
type ValidatedResult struct {
	Valid          bool
	Verdict        AggregateVerdict
	RequiredTotal  int
	RequiredPassed int
	Failed         int
	Blocked        int
	TimedOut       int
}

// AcceptanceResultValidator validates result envelopes. It holds the closed
// failure registry so every declared code is checked against one source of
// truth.
type AcceptanceResultValidator struct {
	registry *FailureRegistry
}

// NewAcceptanceResultValidator builds a validator over the canonical registry.
// It fails closed if the canonical registry is somehow contract-invalid.
func NewAcceptanceResultValidator() (AcceptanceResultValidator, error) {
	reg, err := DefaultFailureRegistry()
	if err != nil {
		return AcceptanceResultValidator{}, err
	}
	return AcceptanceResultValidator{registry: reg}, nil
}

// Validate checks the envelope against expectedRelease and returns a
// ValidatedResult, or a *ContractError carrying one closed code. It never
// tolerates, ignores, or guesses a row compatible.
func (v AcceptanceResultValidator) Validate(envelope ResultEnvelope, expectedRelease ExpectedRelease) (ValidatedResult, error) {
	reg := v.registry
	if reg == nil {
		return ValidatedResult{}, contractErr(CodeMalformed, "validator has no failure registry")
	}
	// 1. Missing.
	if strings.TrimSpace(envelope.SchemaVersion) == "" {
		return ValidatedResult{}, contractErr(CodeMissing, "result is missing (no schema version)")
	}
	// 2. Unsupported schema / mode.
	if envelope.SchemaVersion != resultSchema {
		return ValidatedResult{}, contractErr(CodeUnsupported, "unsupported result schema %q", envelope.SchemaVersion)
	}
	if !closedModes[envelope.Run.Mode] {
		return ValidatedResult{}, contractErr(CodeUnsupported, "unsupported run mode %q", envelope.Run.Mode)
	}
	// 3. Malformed / incomplete required fields.
	for name, val := range map[string]string{
		"manifestId":                  envelope.ManifestID,
		"manifestDigest":              envelope.ManifestDigest,
		"policyDigest":                envelope.PolicyDigest,
		"runnerDigest":                envelope.RunnerDigest,
		"release.sourceSha":           envelope.Release.SourceSha,
		"release.buildManifestDigest": envelope.Release.BuildManifestDigest,
		"release.train":               envelope.Release.Train,
		"run.runId":                   envelope.Run.RunID,
		"run.evidenceEligibility":     envelope.Run.EvidenceEligibility,
		"evidence.indexDigest":        envelope.Evidence.IndexDigest,
	} {
		if strings.TrimSpace(val) == "" {
			return ValidatedResult{}, contractErr(CodeMalformed, "result is incomplete: %s is empty", name)
		}
	}
	if envelope.Run.StartedAt.IsZero() || envelope.Run.ObservedAt.IsZero() {
		return ValidatedResult{}, contractErr(CodeMalformed, "result is incomplete: run timestamps are zero")
	}
	if envelope.ManifestRevision < 1 {
		return ValidatedResult{}, contractErr(CodeMalformed, "result manifest revision must be >= 1")
	}
	// 4. Signature present.
	if strings.TrimSpace(envelope.Signature.Format) == "" || strings.TrimSpace(envelope.Signature.PayloadDigest) == "" {
		return ValidatedResult{}, contractErr(CodeSignature, "result signature is missing")
	}
	// 5. Aggregate verdict is a closed verdict.
	if !IsClosedVerdict(envelope.Aggregate.Verdict) {
		return ValidatedResult{}, contractErr(CodeUnknownEnum, "result aggregate verdict %q is not closed", envelope.Aggregate.Verdict)
	}
	// 6. Release identity.
	if expectedRelease.ManifestID != "" && envelope.ManifestID != expectedRelease.ManifestID {
		return ValidatedResult{}, contractErr(CodeManifestMismatch, "result manifestId does not match the expected release manifest")
	}
	if envelope.Release.SourceSha != expectedRelease.SourceSha ||
		envelope.Release.BuildManifestDigest != expectedRelease.BuildManifestDigest ||
		envelope.Release.Train != expectedRelease.Train {
		return ValidatedResult{}, contractErr(CodeReleaseMismatch, "result release identity does not match the expected release")
	}
	// 7. Freshness: not before activation, not in the future beyond skew, not older
	// than the freshness window.
	if !expectedRelease.ActivatedAt.IsZero() && envelope.Run.ObservedAt.Before(expectedRelease.ActivatedAt) {
		return ValidatedResult{}, contractErr(CodeStaleResult, "result was observed before the running release activated")
	}
	if !expectedRelease.Now.IsZero() {
		if envelope.Run.ObservedAt.After(expectedRelease.Now.Add(expectedRelease.ClockSkew)) {
			return ValidatedResult{}, contractErr(CodeStaleResult, "result observedAt is in the future beyond tolerated skew")
		}
		if expectedRelease.FreshnessWindow > 0 && envelope.Run.ObservedAt.Before(expectedRelease.Now.Add(-expectedRelease.FreshnessWindow)) {
			return ValidatedResult{}, contractErr(CodeStaleResult, "result is stale: observedAt is older than the freshness window")
		}
	}
	// 8. Rows: non-empty, each validated, no duplicate, value-safe.
	if len(envelope.Journeys) == 0 {
		return ValidatedResult{}, contractErr(CodeMalformed, "result declares no journey rows")
	}
	seen := make(map[string]bool, len(envelope.Journeys))
	var requiredTotal, requiredPassed, optionalLimited, failed, blocked, timedOut int
	for _, row := range envelope.Journeys {
		if strings.TrimSpace(row.JourneyID) == "" {
			return ValidatedResult{}, contractErr(CodeMalformed, "a journey row has no id")
		}
		if seen[row.JourneyID] {
			return ValidatedResult{}, contractErr(CodeDuplicateJourney, "duplicate journey row %q", row.JourneyID)
		}
		seen[row.JourneyID] = true
		if !closedRequiredness[row.Requiredness] {
			return ValidatedResult{}, contractErr(CodeUnknownEnum, "journey row %q declares unknown requiredness %q", row.JourneyID, row.Requiredness)
		}
		if !IsClosedJourneyOutcome(row.Outcome) {
			return ValidatedResult{}, contractErr(CodeUnknownEnum, "journey row %q declares unknown outcome %q", row.JourneyID, row.Outcome)
		}
		if strings.TrimSpace(string(row.FailureCode)) != "" {
			if _, ok := reg.LookupFailure(row.FailureCode); !ok {
				return ValidatedResult{}, contractErr(CodeUnknownEnum, "journey row %q declares unregistered failure code %q", row.JourneyID, row.FailureCode)
			}
		}
		// A clean pass must not carry a failure code; a failure must carry one.
		if row.Outcome == OutcomePassed && strings.TrimSpace(string(row.FailureCode)) != "" {
			return ValidatedResult{}, contractErr(CodeMalformed, "journey row %q passed but carries a failure code", row.JourneyID)
		}
		if (row.Outcome == OutcomeFailed || row.Outcome == OutcomeTimedOut) && strings.TrimSpace(string(row.FailureCode)) == "" {
			return ValidatedResult{}, contractErr(CodeMalformed, "journey row %q failed but carries no failure code", row.JourneyID)
		}
		// Value-safe evidence fields.
		if verr := scanEvidenceFields(row.EvidenceFields); verr != nil {
			return ValidatedResult{}, contractErr(verr.Code, "journey row %q: %s", row.JourneyID, verr.Reason)
		}
		// Count reconciliation inputs.
		switch row.Requiredness {
		case RequirednessRequired:
			requiredTotal++
			if row.Outcome == OutcomePassed || isAllowedLimitation(row.Outcome) {
				requiredPassed++
			}
		case RequirednessOptional:
			if isAllowedLimitation(row.Outcome) {
				optionalLimited++
			}
		}
		switch row.Outcome {
		case OutcomeFailed:
			failed++
		case OutcomeBlocked:
			blocked++
		case OutcomeTimedOut:
			timedOut++
		}
	}
	// Top-level evidence field names are also value-safe.
	if verr := scanEvidenceFields(envelope.Evidence.FieldNames); verr != nil {
		return ValidatedResult{}, contractErr(verr.Code, "result evidence: %s", verr.Reason)
	}
	// 9. Aggregate count consistency with the rows.
	agg := envelope.Aggregate
	if agg.RequiredTotal != requiredTotal || agg.RequiredPassed != requiredPassed ||
		agg.OptionalLimited != optionalLimited || agg.Failed != failed ||
		agg.Blocked != blocked || agg.TimedOut != timedOut {
		return ValidatedResult{}, contractErr(CodeMalformed, "result aggregate counts do not reconcile with the journey rows")
	}
	return ValidatedResult{
		Valid:          true,
		Verdict:        agg.Verdict,
		RequiredTotal:  requiredTotal,
		RequiredPassed: requiredPassed,
		Failed:         failed,
		Blocked:        blocked,
		TimedOut:       timedOut,
	}, nil
}

// isAllowedLimitation reports whether o is one of the closed allowed-limitation
// outcomes (an explicit truthful empty/quiet/optional/degraded pass).
func isAllowedLimitation(o JourneyOutcome) bool {
	switch o {
	case OutcomeAllowedEmpty, OutcomeAllowedQuiet, OutcomeAllowedOptional, OutcomeAllowedDegraded:
		return true
	default:
		return false
	}
}
