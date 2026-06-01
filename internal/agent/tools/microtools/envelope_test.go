// Spec 065 SCOPE-1 — micro-tool envelope foundation tests.

package microtools

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validResolvedEnvelope() Envelope {
	return Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Status:        StatusResolved,
		Value:         map[string]any{"name": "Palm Springs"},
		Source: Source{
			Provider:    "open-meteo",
			Kind:        SourceKindHTTPProvider,
			RetrievedAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
			Attribution: "Data: Open-Meteo",
		},
	}
}

// TestMicroToolEnvelopeSchemaRejectsMissingSource is the SCOPE-1
// registry-contract unit listed in scopes.md Test Plan. It exercises
// the foundation invariant: every envelope MUST carry a non-zero
// Source — the trace renderer surfaces source attribution and a
// missing source is a fail-loud bug.
func TestMicroToolEnvelopeSchemaRejectsMissingSource(t *testing.T) {
	t.Run("zero_source_rejected", func(t *testing.T) {
		env := validResolvedEnvelope()
		env.Source = Source{}
		if err := ValidateEnvelope(env); err == nil {
			t.Fatal("expected ValidateEnvelope to reject envelope with zero Source")
		} else if !strings.Contains(err.Error(), "source.provider") {
			t.Fatalf("expected source.provider error, got %q", err)
		}
	})

	t.Run("missing_provider_rejected", func(t *testing.T) {
		env := validResolvedEnvelope()
		env.Source.Provider = ""
		if err := ValidateEnvelope(env); err == nil || !strings.Contains(err.Error(), "source.provider") {
			t.Fatalf("expected source.provider error, got %v", err)
		}
	})

	t.Run("missing_kind_rejected", func(t *testing.T) {
		env := validResolvedEnvelope()
		env.Source.Kind = ""
		if err := ValidateEnvelope(env); err == nil || !strings.Contains(err.Error(), "source.kind") {
			t.Fatalf("expected source.kind error, got %v", err)
		}
	})

	t.Run("missing_retrieved_at_rejected", func(t *testing.T) {
		env := validResolvedEnvelope()
		env.Source.RetrievedAt = time.Time{}
		if err := ValidateEnvelope(env); err == nil || !strings.Contains(err.Error(), "source.retrieved_at") {
			t.Fatalf("expected source.retrieved_at error, got %v", err)
		}
	})

	t.Run("missing_attribution_rejected", func(t *testing.T) {
		env := validResolvedEnvelope()
		env.Source.Attribution = ""
		if err := ValidateEnvelope(env); err == nil || !strings.Contains(err.Error(), "source.attribution") {
			t.Fatalf("expected source.attribution error, got %v", err)
		}
	})

	t.Run("bytes_path_rejects_missing_source", func(t *testing.T) {
		env := validResolvedEnvelope()
		env.Source = Source{}
		raw, err := json.Marshal(env)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := ValidateEnvelopeBytes(raw); err == nil || !strings.Contains(err.Error(), "source") {
			t.Fatalf("expected source error from bytes path, got %v", err)
		}
	})

	t.Run("valid_envelope_accepted", func(t *testing.T) {
		if err := ValidateEnvelope(validResolvedEnvelope()); err != nil {
			t.Fatalf("expected baseline envelope to validate, got %v", err)
		}
	})
}

// TestEnvelopeStatusInvariants exercises the status-specific
// foundation policies: resolved requires value, ambiguous requires
// candidates, failed requires error.
func TestEnvelopeStatusInvariants(t *testing.T) {
	t.Run("resolved_requires_value", func(t *testing.T) {
		env := validResolvedEnvelope()
		env.Value = nil
		if err := ValidateEnvelope(env); err == nil || !strings.Contains(err.Error(), "value") {
			t.Fatalf("expected value error, got %v", err)
		}
	})

	t.Run("ambiguous_requires_candidates", func(t *testing.T) {
		env := validResolvedEnvelope()
		env.Status = StatusAmbiguous
		env.Value = nil
		if err := ValidateEnvelope(env); err == nil || !strings.Contains(err.Error(), "candidate") {
			t.Fatalf("expected candidate error, got %v", err)
		}
	})

	t.Run("ambiguous_with_candidates_accepted", func(t *testing.T) {
		env := validResolvedEnvelope()
		env.Status = StatusAmbiguous
		env.Value = nil
		env.Candidates = []Candidate{{
			Rank:       1,
			Label:      "Springfield, IL",
			Value:      map[string]any{"admin1": "Illinois"},
			Confidence: 0.6,
		}}
		if err := ValidateEnvelope(env); err != nil {
			t.Fatalf("expected ambiguous envelope to validate, got %v", err)
		}
	})

	t.Run("failed_requires_error", func(t *testing.T) {
		env := validResolvedEnvelope()
		env.Status = StatusFailed
		env.Value = nil
		if err := ValidateEnvelope(env); err == nil || !strings.Contains(err.Error(), "error") {
			t.Fatalf("expected error payload error, got %v", err)
		}
	})

	t.Run("schema_version_must_match", func(t *testing.T) {
		env := validResolvedEnvelope()
		env.SchemaVersion = "v0"
		if err := ValidateEnvelope(env); err == nil || !strings.Contains(err.Error(), "schema_version") {
			t.Fatalf("expected schema_version error, got %v", err)
		}
	})

	t.Run("invalid_status_rejected", func(t *testing.T) {
		env := validResolvedEnvelope()
		env.Status = Status("partial")
		if err := ValidateEnvelope(env); err == nil || !strings.Contains(err.Error(), "status") {
			t.Fatalf("expected status error, got %v", err)
		}
	})

	t.Run("confidence_out_of_range_rejected", func(t *testing.T) {
		env := validResolvedEnvelope()
		env.Confidence = 1.5
		if err := ValidateEnvelope(env); err == nil || !strings.Contains(err.Error(), "confidence") {
			t.Fatalf("expected confidence error, got %v", err)
		}
	})
}
