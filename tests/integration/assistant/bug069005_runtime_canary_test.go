//go:build integration

package assistant_integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
	"github.com/smackerel/smackerel/internal/assistant/intent"
	"github.com/smackerel/smackerel/internal/config"
)

type bug069005LiveStack struct {
	coreURL   string
	authToken string
	database  *pgxpool.Pool
}

const (
	bug069005FacadeReadyMaxWait        = 60 * time.Second
	bug069005FacadeReadyPollInterval   = 2 * time.Second
	bug069005FacadeReadyRequestTimeout = 10 * time.Second
)

func loadBug069005LiveStack(t *testing.T) bug069005LiveStack {
	t.Helper()
	coreURL := strings.TrimRight(os.Getenv("CORE_EXTERNAL_URL"), "/")
	databaseURL := os.Getenv("DATABASE_URL")
	authToken := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if coreURL == "" || databaseURL == "" || authToken == "" {
		t.Fatalf("BUG-069-005 live integration requires CORE_EXTERNAL_URL, DATABASE_URL, and SMACKEREL_AUTH_TOKEN")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return bug069005LiveStack{coreURL: coreURL, authToken: authToken, database: pool}
}

func sendBug069005Turn(
	stack bug069005LiveStack,
	request httpadapter.TurnRequest,
	requestTimeout time.Duration,
) (int, httpadapter.TurnResponse, error) {
	encoded, err := json.Marshal(request)
	if err != nil {
		return 0, httpadapter.TurnResponse{}, fmt.Errorf("marshal assistant turn: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, stack.coreURL+"/api/assistant/turn", bytes.NewReader(encoded))
	if err != nil {
		return 0, httpadapter.TurnResponse{}, fmt.Errorf("build assistant request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+stack.authToken)
	client := &http.Client{Timeout: requestTimeout}
	response, err := client.Do(httpRequest)
	if err != nil {
		return 0, httpadapter.TurnResponse{}, fmt.Errorf("POST assistant turn: %w", err)
	}
	defer response.Body.Close()
	var envelope httpadapter.TurnResponse
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return response.StatusCode, httpadapter.TurnResponse{}, fmt.Errorf("decode assistant response: %w", err)
	}
	return response.StatusCode, envelope, nil
}

func postBug069005Turn(t *testing.T, stack bug069005LiveStack, request httpadapter.TurnRequest) httpadapter.TurnResponse {
	t.Helper()
	status, envelope, err := sendBug069005Turn(stack, request, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("assistant status = %d, want 200; envelope=%+v", status, envelope)
	}
	return envelope
}

func waitBug069005AssistantFacadeReady(t *testing.T, stack bug069005LiveStack) {
	t.Helper()
	deadline := time.Now().Add(bug069005FacadeReadyMaxWait)
	var lastStatus int
	var lastEnvelope httpadapter.TurnResponse
	var lastErr error
	for time.Now().Before(deadline) {
		lastStatus, lastEnvelope, lastErr = sendBug069005Turn(stack, httpadapter.TurnRequest{
			SchemaVersion:      httpadapter.SchemaVersionV1,
			TransportMessageID: "test-bug069005-readiness-" + time.Now().UTC().Format("20060102T150405.000000000"),
			Kind:               string(contracts.KindReset),
			TransportHint:      "web",
		}, bug069005FacadeReadyRequestTimeout)
		if lastErr == nil && lastStatus == http.StatusOK && lastEnvelope.FacadeInvoked {
			return
		}
		time.Sleep(bug069005FacadeReadyPollInterval)
	}
	t.Fatalf(
		"BUG-069-005 assistant facade did not become ready within %s; last_status=%d last_envelope=%+v last_error=%v",
		bug069005FacadeReadyMaxWait,
		lastStatus,
		lastEnvelope,
		lastErr,
	)
}

func resetBug069005Conversation(t *testing.T, stack bug069005LiveStack) {
	t.Helper()
	response := postBug069005Turn(t, stack, httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "test-bug069005-integration-reset-" + time.Now().UTC().Format("20060102T150405.000000000"),
		Kind:               string(contracts.KindReset),
		TransportHint:      "web",
	})
	if !response.FacadeInvoked {
		t.Fatal("reset did not reach the live facade")
	}
}

func TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler(t *testing.T) {
	stack := loadBug069005LiveStack(t)
	waitBug069005AssistantFacadeReady(t, stack)
	resetBug069005Conversation(t, stack)
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	transport, err := intent.NewHTTPTransport(
		cfg.MLSidecarURL,
		cfg.AuthToken,
		cfg.Assistant.IntentCompiler.Timeout,
		cfg.Assistant.IntentCompiler.MaxOutputBytes,
	)
	if err != nil {
		t.Fatalf("NewHTTPTransport: %v", err)
	}
	compiler, err := intent.NewLLMCompiler(intent.CompilerConfig{
		Enabled:               cfg.Assistant.IntentCompiler.Enabled,
		ModelRole:             cfg.Assistant.IntentCompiler.ModelRole,
		PromptContractVersion: cfg.Assistant.IntentCompiler.PromptContractVersion,
		SchemaVersion:         cfg.Assistant.IntentCompiler.SchemaVersion,
		Timeout:               cfg.Assistant.IntentCompiler.Timeout,
		ConfidenceFloor:       cfg.Assistant.IntentCompiler.ConfidenceFloor,
		MaxContextTurns:       cfg.Assistant.IntentCompiler.MaxContextTurns,
		MaxOutputBytes:        cfg.Assistant.IntentCompiler.MaxOutputBytes,
		RetryBudget:           cfg.Assistant.IntentCompiler.RetryBudget,
	}, transport)
	if err != nil {
		t.Fatalf("NewLLMCompiler: %v", err)
	}
	barcelona, _, err := compiler.Compile(context.Background(), intent.RawTurn{
		UserID: "test-bug069005-integration-user", Transport: "web",
		TransportMessageID: "test-bug069005-integration-barcelona",
		Text:               "what is the weather in Barcelona", ReceivedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("live Barcelona compiler route: %v", err)
	}
	if barcelona.ActionClass != intent.ActionExternalLookup {
		t.Fatalf("Barcelona compiled action = %q, want external_lookup", barcelona.ActionClass)
	}
	if barcelona.ScenarioHint == nil || *barcelona.ScenarioHint != "weather_query" {
		t.Fatalf("Barcelona compiled scenario hint = %v, want weather_query", barcelona.ScenarioHint)
	}
	if location, ok := barcelona.Slots["location"].(map[string]any); !ok || location["raw"] != "Barcelona" {
		t.Fatalf("Barcelona compiled location slot = %#v, want raw Barcelona", barcelona.Slots["location"])
	}
	compiled, _, err := compiler.Compile(context.Background(), intent.RawTurn{
		UserID: "test-bug069005-integration-user", Transport: "web",
		TransportMessageID: "test-bug069005-integration-compiler",
		Text:               "what is the weather in Springfield", ReceivedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("live compiler route: %v", err)
	}
	if compiled.ActionClass != intent.ActionClarify {
		t.Fatalf("compiled action = %q, want clarify", compiled.ActionClass)
	}
	if location, ok := compiled.Slots["location"].(map[string]any); !ok || location["raw"] != "Springfield" {
		t.Fatalf("compiled location slot = %#v, want raw Springfield", compiled.Slots["location"])
	}

	envelope := postBug069005Turn(t, stack, httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "test-bug069005-integration-core-" + time.Now().UTC().Format("150405.000000000"),
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "what is the weather in Springfield",
	})
	if envelope.DisambiguationPrompt == nil || len(envelope.DisambiguationPrompt.Choices) < 2 {
		t.Fatalf("live core compiler attachment produced no persistent choices: %+v", envelope)
	}
	resetBug069005Conversation(t, stack)
}

func TestCompilerInteractiveControlsPersistByUserAndTransport(t *testing.T) {
	stack := loadBug069005LiveStack(t)
	waitBug069005AssistantFacadeReady(t, stack)
	resetBug069005Conversation(t, stack)
	turnID := "test-bug069005-integration-list-" + time.Now().UTC().Format("20060102T150405.000000000")
	proposal := postBug069005Turn(t, stack, httpadapter.TurnRequest{
		SchemaVersion: httpadapter.SchemaVersionV1, TransportMessageID: turnID,
		Kind: string(contracts.KindText), TransportHint: "web",
		Text: "add test-bug069005-milk to my shopping list",
	})
	if proposal.ConfirmCard == nil || proposal.ConfirmCard.ConfirmRef == "" {
		t.Fatalf("live proposal missing ConfirmCard: %+v", proposal)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var pendingRef string
	if err := stack.database.QueryRow(ctx, `
SELECT pending_confirm->>'confirm_ref'
  FROM assistant_conversations
 WHERE user_id = 'shared' AND transport = 'web'
`).Scan(&pendingRef); err != nil {
		t.Fatalf("load pending confirm: %v", err)
	}
	if pendingRef != proposal.ConfirmCard.ConfirmRef {
		t.Fatalf("persisted confirm ref = %q, want %q", pendingRef, proposal.ConfirmCard.ConfirmRef)
	}
	var before int
	if err := stack.database.QueryRow(ctx, `SELECT COUNT(*) FROM lists WHERE source_query = $1`, turnID).Scan(&before); err != nil {
		t.Fatalf("count pre-confirm list: %v", err)
	}
	if before != 0 {
		t.Fatalf("pre-confirm list count = %d, want 0", before)
	}

	accepted := postBug069005Turn(t, stack, httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID + "-accept", Kind: string(contracts.KindConfirm),
		TransportHint: "web", ConfirmRef: proposal.ConfirmCard.ConfirmRef,
		ConfirmChoice: string(contracts.ConfirmPositive),
	})
	if accepted.ErrorCause != "" {
		t.Fatalf("confirmed action error = %q", accepted.ErrorCause)
	}
	var after int
	if err := stack.database.QueryRow(ctx, `SELECT COUNT(*) FROM lists WHERE source_query = $1`, turnID).Scan(&after); err != nil {
		t.Fatalf("count confirmed list: %v", err)
	}
	if after != 1 {
		t.Fatalf("confirmed list count = %d, want 1", after)
	}
	var pendingRows int
	if err := stack.database.QueryRow(ctx, `
SELECT COUNT(*) FROM assistant_conversations
 WHERE user_id = 'shared' AND transport = 'web' AND pending_confirm IS NOT NULL
`).Scan(&pendingRows); err != nil {
		t.Fatalf("count pending confirms: %v", err)
	}
	if pendingRows != 0 {
		t.Fatalf("pending confirm rows after accept = %d, want 0", pendingRows)
	}
	_, _ = stack.database.Exec(ctx, `DELETE FROM lists WHERE source_query = $1`, turnID)
	resetBug069005Conversation(t, stack)
}
