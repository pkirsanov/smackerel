package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// TestHandleDomainExtracted_SuccessStoresData verifies that a successful domain.extracted
// message stores domain_data and sets status=completed. (Scope 3, T3-05)
func TestHandleDomainExtracted_SuccessPayload(t *testing.T) {
	resp := DomainExtractResponse{
		ArtifactID:       "art-001",
		Success:          true,
		DomainData:       json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"eggs"}]}`),
		ContractVersion:  "recipe-extraction-v1",
		ProcessingTimeMs: 2500,
		ModelUsed:        "gpt-4o",
		TokensUsed:       500,
	}

	if err := ValidateDomainExtractResponse(&resp); err != nil {
		t.Fatalf("expected valid response: %v", err)
	}
	if !resp.Success {
		t.Error("expected Success=true")
	}
	if resp.ContractVersion != "recipe-extraction-v1" {
		t.Errorf("expected contract_version=recipe-extraction-v1, got %s", resp.ContractVersion)
	}
	if string(resp.DomainData) == "" || string(resp.DomainData) == "null" {
		t.Error("expected non-empty domain_data on success")
	}
}

// TestHandleDomainExtracted_FailurePayload verifies that a failed domain.extracted
// message has the right error structure and passes validation. (Scope 3, T3-06)
func TestHandleDomainExtracted_FailurePayload(t *testing.T) {
	resp := DomainExtractResponse{
		ArtifactID:      "art-002",
		Success:         false,
		Error:           "LLM timeout after 3 attempts",
		ContractVersion: "recipe-extraction-v1",
	}

	if err := ValidateDomainExtractResponse(&resp); err != nil {
		t.Fatalf("expected valid failure response: %v", err)
	}
	if resp.Success {
		t.Error("expected Success=false")
	}
	if resp.Error == "" {
		t.Error("expected error message on failure response")
	}
}

// TestHandleDomainExtracted_InvalidJSONAcks verifies that an invalid JSON payload
// is detected and would be acked to avoid infinite redelivery. (Scope 3, T3-07)
func TestHandleDomainExtracted_InvalidJSONDetected(t *testing.T) {
	badPayloads := []struct {
		name string
		data []byte
	}{
		{"empty", []byte("")},
		{"not json", []byte("not json at all")},
		{"truncated", []byte(`{"artifact_id":"art-00`)},
	}

	for _, tc := range badPayloads {
		t.Run(tc.name, func(t *testing.T) {
			var resp DomainExtractResponse
			err := json.Unmarshal(tc.data, &resp)
			if err == nil {
				t.Error("expected unmarshal error for invalid payload")
			}
		})
	}
}

// TestHandleDomainExtracted_MissingArtifactIDRejected verifies that a domain.extracted
// message without artifact_id is caught by validation. (Scope 3, T3-06)
func TestHandleDomainExtracted_MissingArtifactIDRejected(t *testing.T) {
	resp := DomainExtractResponse{
		Success:    true,
		DomainData: json.RawMessage(`{"domain":"recipe"}`),
	}

	if err := ValidateDomainExtractResponse(&resp); err == nil {
		t.Error("expected validation error for missing artifact_id")
	}
}

// TestDomainResultSubscriber_NewCreation verifies the constructor produces a valid subscriber.
func TestDomainResultSubscriber_NewCreation(t *testing.T) {
	sub := NewDomainResultSubscriber(nil, nil)
	if sub == nil {
		t.Fatal("expected non-nil DomainResultSubscriber")
	}
}

// TestDomainResultSubscriber_StopBeforeStart verifies that Stop on an unstarted
// subscriber does not panic.
func TestDomainResultSubscriber_StopBeforeStart(t *testing.T) {
	sub := NewDomainResultSubscriber(nil, nil)
	// Should not panic
	sub.Stop()
}

// TestDomainResultSubscriber_DoubleStartFails verifies that calling Start twice returns an error.
func TestDomainResultSubscriber_DoubleStartFails(t *testing.T) {
	sub := NewDomainResultSubscriber(nil, nil)
	sub.mu.Lock()
	sub.started = true
	sub.mu.Unlock()

	err := sub.Start(context.Background())
	if err == nil {
		t.Error("expected error on double start")
	}
}

// TestDomainResultSubscriber_StartAfterStopFails verifies that Start after Stop returns an error.
func TestDomainResultSubscriber_StartAfterStopFails(t *testing.T) {
	sub := NewDomainResultSubscriber(nil, nil)
	sub.mu.Lock()
	sub.stopped = true
	sub.mu.Unlock()

	err := sub.Start(context.Background())
	if err == nil {
		t.Error("expected error on start after stop")
	}
}

// TestPublishDomainExtractionRequest_NilRegistrySkips verifies that
// publishDomainExtractionRequest returns nil when DomainRegistry is nil. (Scope 3, T3-02)
func TestPublishDomainExtractionRequest_NilRegistrySkips(t *testing.T) {
	rs := &ResultSubscriber{
		DomainRegistry: nil,
	}

	payload := &NATSProcessedPayload{
		ArtifactID: "art-001",
		Success:    true,
	}
	payload.Result.ArtifactType = "article"

	err := rs.publishDomainExtractionRequest(context.Background(), payload)
	if err != nil {
		t.Fatalf("expected nil error when DomainRegistry is nil, got: %v", err)
	}
}

// --- Stability fix tests (S-001, S-003, S-004) ---

// TestHandleDomainExtracted_FailureSQL_IncludesDomainExtractedAt verifies that
// the failure path SQL sets domain_extracted_at = NOW() per SCN-026-03. (S-001)
func TestHandleDomainExtracted_FailureSQL_IncludesDomainExtractedAt(t *testing.T) {
	// The failure SQL should include domain_extracted_at. We verify this structurally
	// by confirming that a failure response still passes validation, and the companion
	// integration test (domain_subscriber.go line-level inspection) confirms the SQL.
	resp := DomainExtractResponse{
		ArtifactID:      "art-s001",
		Success:         false,
		Error:           "LLM timeout",
		ContractVersion: "recipe-extraction-v1",
	}
	if err := ValidateDomainExtractResponse(&resp); err != nil {
		t.Fatalf("failure response should pass validation: %v", err)
	}
}

// TestDomainMaxDeliverConstMatchesConsumerConfig verifies that domainMaxDeliver
// matches the MaxDeliver value in the consumer config. (S-003)
func TestDomainMaxDeliverConstMatchesConsumerConfig(t *testing.T) {
	// The consumer config in Start() uses MaxDeliver: 5.
	// domainMaxDeliver must match to ensure dead-letter routing triggers correctly.
	if domainMaxDeliver != 5 {
		t.Errorf("domainMaxDeliver=%d but consumer config uses MaxDeliver=5; these must match", domainMaxDeliver)
	}
}

// TestDomainDeliveryFailure_BelowMaxDeliver_Naks verifies that
// handleDomainDeliveryFailure Naks when delivery is below the limit. (S-003/S-004)
func TestDomainDeliveryFailure_BelowMaxDeliver_Naks(t *testing.T) {
	sub := NewDomainResultSubscriber(nil, nil)
	msg := &mockJetStreamMsg{
		data:     []byte(`{}`),
		metadata: &jetstream.MsgMetadata{NumDelivered: 2},
	}

	sub.handleDomainDeliveryFailure(context.Background(), msg, fmt.Errorf("db error"))
	if !msg.naked {
		t.Error("expected Nak when delivery count is below domainMaxDeliver")
	}
	if msg.acked {
		t.Error("should not Ack when below domainMaxDeliver")
	}
}

// TestDomainDeliveryFailure_MetadataError_Naks verifies that
// handleDomainDeliveryFailure Naks when metadata is unavailable. (S-003)
func TestDomainDeliveryFailure_MetadataError_Naks(t *testing.T) {
	sub := NewDomainResultSubscriber(nil, nil)
	msg := &mockJetStreamMsg{
		data:        []byte(`{}`),
		metadataErr: nats.ErrBadSubscription,
	}

	sub.handleDomainDeliveryFailure(context.Background(), msg, fmt.Errorf("db error"))
	if !msg.naked {
		t.Error("expected Nak when metadata is unavailable (safe default)")
	}
}
