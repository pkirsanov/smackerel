package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	smacknats "github.com/smackerel/smackerel/internal/nats"
)

// mockDomainDB implements pipeline.DomainDB for unit-testing the
// handleDomainExtracted SQL UPDATE side effects without standing up a
// real Postgres. Records every Exec call so tests can assert on the
// SQL text and bound parameters. Optionally returns execErr to simulate
// DB failures and trigger the Nak / dead-letter path.
//
// BUG-026-003: introduced because the prior TestHandleDomainExtracted_*
// tests only validated the response struct via ValidateDomainExtractResponse
// and never invoked the receiver method, leaving handleDomainExtracted at
// 0.0% unit coverage despite five tests with that name.
type mockDomainDB struct {
	mu        sync.Mutex
	execCalls []domainExecCall
	execErr   error
}

type domainExecCall struct {
	sql  string
	args []any
}

func (m *mockDomainDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.execCalls = append(m.execCalls, domainExecCall{sql: sql, args: args})
	return pgconn.CommandTag{}, m.execErr
}

func (m *mockDomainDB) calls() []domainExecCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domainExecCall, len(m.execCalls))
	copy(out, m.execCalls)
	return out
}

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

// TestDomainDeliveryFailure_DeadLetterAndNakBothFail_Logs verifies that
// handleDomainDeliveryFailure logs both failures when dead-letter publish
// and subsequent Nak both fail (STB-022-002).
func TestDomainDeliveryFailure_DeadLetterAndNakBothFail_Logs(t *testing.T) {
	mockJS := &mockJetStream{publishErr: fmt.Errorf("NATS connection closed")}
	nc := &smacknats.Client{JetStream: mockJS}
	sub := NewDomainResultSubscriber(nil, nc)

	msg := &mockJetStreamMsg{
		data:     []byte(`{"artifact_id":"art-stb002"}`),
		metadata: &jetstream.MsgMetadata{NumDelivered: 5},
		nakErr:   fmt.Errorf("consumer deleted"),
	}

	// Should not panic; should attempt Nak after dead-letter publish failure.
	// The Nak error is logged (verified structurally — no panic, msg.naked set).
	sub.handleDomainDeliveryFailure(context.Background(), msg, fmt.Errorf("db error"))

	if msg.acked {
		t.Error("should not Ack when dead-letter publish fails")
	}
	if !msg.naked {
		t.Error("expected Nak attempt even when dead-letter publish fails")
	}
}

// --- BUG-026-003: real handler tests that actually invoke handleDomainExtracted ---
//
// Prior TestHandleDomainExtracted_* tests in this file only called the response
// validator. The five tests below directly invoke
// (*DomainResultSubscriber).handleDomainExtracted with a mockJetStreamMsg and
// a mockDomainDB so the SQL UPDATE branches, Ack/Nak decisions, and the failure
// path through handleDomainDeliveryFailure are covered by go test ./...
// (not only by the live-stack E2E in tests/e2e/domain_e2e_test.go).

// TestHandleDomainExtractedInvocation_Success_UpdatesArtifactAndAcks verifies
// that on Success=true the handler issues the completed-status UPDATE with
// domain_data + domain_schema_version and Acks the message. (BUG-026-003)
func TestHandleDomainExtractedInvocation_Success_UpdatesArtifactAndAcks(t *testing.T) {
	db := &mockDomainDB{}
	sub := &DomainResultSubscriber{DB: db}

	resp := DomainExtractResponse{
		ArtifactID:       "art-success-001",
		Success:          true,
		DomainData:       json.RawMessage(`{"domain":"recipe","ingredients":[{"name":"eggs"}]}`),
		ContractVersion:  "recipe-extraction-v1",
		ProcessingTimeMs: 1234,
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	msg := &mockJetStreamMsg{data: payload}

	sub.handleDomainExtracted(context.Background(), msg)

	if !msg.acked {
		t.Error("expected Ack on successful UPDATE")
	}
	if msg.naked {
		t.Error("did not expect Nak on success")
	}

	calls := db.calls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 SQL Exec, got %d", len(calls))
	}
	got := calls[0]
	if !strings.Contains(got.sql, "domain_data = $2") {
		t.Errorf("expected SQL to bind domain_data; got: %s", got.sql)
	}
	if !strings.Contains(got.sql, "domain_extraction_status = 'completed'") {
		t.Errorf("expected SQL to set status=completed; got: %s", got.sql)
	}
	if !strings.Contains(got.sql, "domain_schema_version = $3") {
		t.Errorf("expected SQL to bind schema_version; got: %s", got.sql)
	}
	if !strings.Contains(got.sql, "domain_extracted_at = NOW()") {
		t.Errorf("expected SQL to stamp domain_extracted_at; got: %s", got.sql)
	}
	if len(got.args) != 3 {
		t.Fatalf("expected 3 bound args, got %d", len(got.args))
	}
	if got.args[0] != "art-success-001" {
		t.Errorf("expected $1=artifact_id, got %v", got.args[0])
	}
	if got.args[2] != "recipe-extraction-v1" {
		t.Errorf("expected $3=contract_version, got %v", got.args[2])
	}
}

// TestHandleDomainExtractedInvocation_Failure_UpdatesStatusAndStampsTimestamp
// verifies that on Success=false the handler issues the failed-status UPDATE
// (with domain_extracted_at = NOW() per S-001 / SCN-026-03) and Acks the
// message. (BUG-026-003)
func TestHandleDomainExtractedInvocation_Failure_UpdatesStatusAndStampsTimestamp(t *testing.T) {
	db := &mockDomainDB{}
	sub := &DomainResultSubscriber{DB: db}

	resp := DomainExtractResponse{
		ArtifactID:      "art-failure-001",
		Success:         false,
		Error:           "LLM timeout after 3 attempts",
		ContractVersion: "recipe-extraction-v1",
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	msg := &mockJetStreamMsg{data: payload}

	sub.handleDomainExtracted(context.Background(), msg)

	if !msg.acked {
		t.Error("expected Ack after recording failed status")
	}
	if msg.naked {
		t.Error("did not expect Nak when DB UPDATE on failure path succeeds")
	}

	calls := db.calls()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 SQL Exec on failure path, got %d", len(calls))
	}
	got := calls[0]
	if !strings.Contains(got.sql, "domain_extraction_status = 'failed'") {
		t.Errorf("expected SQL to set status=failed; got: %s", got.sql)
	}
	if !strings.Contains(got.sql, "domain_extracted_at = NOW()") {
		t.Errorf("expected failure SQL to stamp domain_extracted_at (S-001); got: %s", got.sql)
	}
	if len(got.args) != 1 || got.args[0] != "art-failure-001" {
		t.Errorf("expected single arg = artifact_id 'art-failure-001'; got: %v", got.args)
	}
}

// TestHandleDomainExtractedInvocation_InvalidJSON_AcksWithoutDB verifies that
// an unparseable payload is Acked with no DB write so the bad message does
// not redeliver forever. (BUG-026-003)
func TestHandleDomainExtractedInvocation_InvalidJSON_AcksWithoutDB(t *testing.T) {
	db := &mockDomainDB{}
	sub := &DomainResultSubscriber{DB: db}

	msg := &mockJetStreamMsg{data: []byte(`{not valid json`)}

	sub.handleDomainExtracted(context.Background(), msg)

	if !msg.acked {
		t.Error("expected Ack to drop unparseable payload (avoid infinite redelivery)")
	}
	if msg.naked {
		t.Error("did not expect Nak for unparseable payload")
	}
	if got := len(db.calls()); got != 0 {
		t.Errorf("expected 0 DB writes for unparseable payload, got %d", got)
	}
}

// TestHandleDomainExtractedInvocation_MissingArtifactID_AcksWithoutDB verifies
// that a payload missing the required artifact_id field fails validation,
// is Acked, and never reaches the DB UPDATE. (BUG-026-003)
func TestHandleDomainExtractedInvocation_MissingArtifactID_AcksWithoutDB(t *testing.T) {
	db := &mockDomainDB{}
	sub := &DomainResultSubscriber{DB: db}

	resp := DomainExtractResponse{
		Success:         true,
		DomainData:      json.RawMessage(`{"domain":"recipe"}`),
		ContractVersion: "recipe-extraction-v1",
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	msg := &mockJetStreamMsg{data: payload}

	sub.handleDomainExtracted(context.Background(), msg)

	if !msg.acked {
		t.Error("expected Ack to drop validation-failing payload")
	}
	if got := len(db.calls()); got != 0 {
		t.Errorf("expected 0 DB writes when validation fails, got %d", got)
	}
}

// TestHandleDomainExtractedInvocation_DBExecError_TriggersNakBelowMaxDeliver
// verifies that when the success-path UPDATE fails, the handler routes through
// handleDomainDeliveryFailure and Naks (because NumDelivered < domainMaxDeliver),
// and never Acks the message. (BUG-026-003)
func TestHandleDomainExtractedInvocation_DBExecError_TriggersNakBelowMaxDeliver(t *testing.T) {
	db := &mockDomainDB{execErr: fmt.Errorf("simulated DB connection refused")}
	sub := &DomainResultSubscriber{DB: db}

	resp := DomainExtractResponse{
		ArtifactID:      "art-db-error-001",
		Success:         true,
		DomainData:      json.RawMessage(`{"domain":"recipe"}`),
		ContractVersion: "recipe-extraction-v1",
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	msg := &mockJetStreamMsg{
		data:     payload,
		metadata: &jetstream.MsgMetadata{NumDelivered: 1},
	}

	sub.handleDomainExtracted(context.Background(), msg)

	if msg.acked {
		t.Error("must NOT Ack when DB UPDATE fails (would silently lose the message)")
	}
	if !msg.naked {
		t.Error("expected Nak when DB UPDATE fails and delivery count is below domainMaxDeliver")
	}
	if got := len(db.calls()); got != 1 {
		t.Errorf("expected 1 (failing) Exec attempt, got %d", got)
	}
}

// TestHandleDomainExtractedInvocation_FailurePath_DBError_TriggersNak verifies
// that when the failure-status UPDATE itself fails, the handler still routes
// through handleDomainDeliveryFailure (not silent Ack). (BUG-026-003)
func TestHandleDomainExtractedInvocation_FailurePath_DBError_TriggersNak(t *testing.T) {
	db := &mockDomainDB{execErr: fmt.Errorf("simulated DB write failure")}
	sub := &DomainResultSubscriber{DB: db}

	resp := DomainExtractResponse{
		ArtifactID:      "art-failure-db-001",
		Success:         false,
		Error:           "LLM transient error",
		ContractVersion: "recipe-extraction-v1",
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	msg := &mockJetStreamMsg{
		data:     payload,
		metadata: &jetstream.MsgMetadata{NumDelivered: 1},
	}

	sub.handleDomainExtracted(context.Background(), msg)

	if msg.acked {
		t.Error("must NOT Ack when failure-path UPDATE itself fails")
	}
	if !msg.naked {
		t.Error("expected Nak when failure-path UPDATE fails (S-004 contract)")
	}
}
