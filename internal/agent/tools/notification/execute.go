// notification_execute: stage two of the reminder confirm flow.

package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/smackerel/smackerel/internal/agent"
)

var executeInputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["confirm_ref"],
  "properties": {
    "confirm_ref": {"type": "string", "minLength": 1}
  }
}`)

var executeOutputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["phase", "scheduled_job_id"],
  "properties": {
    "phase":            {"type": "string", "enum": ["confirmed"]},
    "scheduled_job_id": {"type": "string"}
  }
}`)

func init() {
	agent.RegisterTool(agent.Tool{
		Name:             ToolExecute,
		Description:      "Stage two of the reminder confirm flow: read the opaque payload under confirm_ref and register a job with the spec 054 scheduler. Side-effect class: write.",
		InputSchema:      executeInputSchema,
		OutputSchema:     executeOutputSchema,
		SideEffectClass:  agent.SideEffectWrite,
		OwningPackage:    "internal/agent/tools/notification",
		PerCallTimeoutMs: 2000,
		Handler:          handleNotificationExecute,
	})
}

type executeInput struct {
	ConfirmRef string `json:"confirm_ref"`
}

type executeOutput struct {
	Phase          string `json:"phase"`
	ScheduledJobID string `json:"scheduled_job_id"`
}

func handleNotificationExecute(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	svc, err := loadServices()
	if err != nil {
		return nil, err
	}
	var in executeInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("notification_execute_bad_input: %w", err)
	}
	in.ConfirmRef = strings.TrimSpace(in.ConfirmRef)
	if in.ConfirmRef == "" {
		return nil, errors.New("notification_execute_missing_confirm_ref")
	}

	payload, ok, err := svc.Confirm.Get(ctx, in.ConfirmRef)
	if err != nil {
		return nil, fmt.Errorf("notification_execute_confirm_store_get: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("notification_execute_confirm_ref_unknown: %s", in.ConfirmRef)
	}

	var envelope payloadEnvelope
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return nil, fmt.Errorf("notification_execute_payload_decode: %w", err)
	}

	jobID, err := svc.Scheduler.Schedule(
		ctx,
		envelope.WhenUTC,
		payload, // pass envelope verbatim so the scheduler stores the same blob
		"assistant",
		"user:"+envelope.UserID,
	)
	if err != nil {
		return nil, fmt.Errorf("notification_execute_scheduler_error: %w", err)
	}

	return json.Marshal(executeOutput{
		Phase:          "confirmed",
		ScheduledJobID: jobID,
	})
}
