// notification_propose: stage one of the reminder confirm flow.

package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/smackerel/smackerel/internal/agent"
)

var proposeInputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["user_id", "what"],
  "properties": {
    "user_id":       {"type": "string", "minLength": 1},
    "what":          {"type": "string", "minLength": 1},
    "when_iso":      {"type": "string"},
    "when_relative": {"type": "string"},
    "transport":     {"type": "string"}
  }
}`)

var proposeOutputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["phase"],
  "properties": {
    "phase":                {"type": "string", "enum": ["proposed", "slot_missing"]},
    "confirm_ref":          {"type": "string"},
    "proposed_action":      {"type": "string"},
    "payload":              {"type": "string"},
    "slot_missing_options": {"type": "array", "items": {"type": "string"}}
  }
}`)

func init() {
	agent.RegisterTool(agent.Tool{
		Name:             ToolPropose,
		Description:      "Stage one of the reminder confirm flow: parse {what, when}, persist an opaque payload under a fresh confirm_ref, and return either phase=proposed (with confirm_ref) or phase=slot_missing (with options).",
		InputSchema:      proposeInputSchema,
		OutputSchema:     proposeOutputSchema,
		SideEffectClass:  agent.SideEffectRead,
		OwningPackage:    "internal/agent/tools/notification",
		PerCallTimeoutMs: 2000,
		Handler:          handleNotificationPropose,
	})
}

type proposeInput struct {
	UserID       string `json:"user_id"`
	What         string `json:"what"`
	WhenISO      string `json:"when_iso,omitempty"`
	WhenRelative string `json:"when_relative,omitempty"`
	Transport    string `json:"transport,omitempty"`
}

type proposeOutput struct {
	Phase              string   `json:"phase"`
	ConfirmRef         string   `json:"confirm_ref,omitempty"`
	ProposedAction     string   `json:"proposed_action,omitempty"`
	Payload            string   `json:"payload,omitempty"`
	SlotMissingOptions []string `json:"slot_missing_options,omitempty"`
}

func handleNotificationPropose(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	svc, err := loadServices()
	if err != nil {
		return nil, err
	}
	var in proposeInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("notification_propose_bad_input: %w", err)
	}
	in.What = strings.TrimSpace(in.What)
	in.UserID = strings.TrimSpace(in.UserID)
	if in.UserID == "" {
		return nil, errors.New("notification_propose_missing_user_id")
	}
	if in.What == "" {
		return nil, errors.New("notification_propose_missing_what")
	}

	whenUTC, err := resolveWhen(in.WhenISO, in.WhenRelative, nowFn())
	if err != nil {
		// Missing/invalid when is a slot_missing, not a tool error —
		// the scenario layer asks the user for clarification rather
		// than erroring out.
		return marshalProposeOutput(proposeOutput{
			Phase:              "slot_missing",
			SlotMissingOptions: defaultWhenOptions(),
		})
	}

	envelope := payloadEnvelope{
		What:      in.What,
		WhenUTC:   whenUTC,
		UserID:    in.UserID,
		Transport: strings.TrimSpace(in.Transport),
	}
	payloadBytes, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("notification_propose_marshal_payload: %w", err)
	}
	ref := newRefFn()
	if err := svc.Confirm.Put(ctx, ref, string(payloadBytes), svc.ConfirmTimeout); err != nil {
		return nil, fmt.Errorf("notification_propose_confirm_store_put: %w", err)
	}
	return marshalProposeOutput(proposeOutput{
		Phase:          "proposed",
		ConfirmRef:     ref,
		ProposedAction: fmt.Sprintf("Remind you to %s at %s", in.What, whenUTC.Format("2006-01-02 15:04 UTC")),
		Payload:        string(payloadBytes),
	})
}

func marshalProposeOutput(o proposeOutput) (json.RawMessage, error) {
	return json.Marshal(o)
}

// defaultWhenOptions are the v1 fallback options surfaced when the
// LLM could not resolve a `when`. SCOPE-08 may replace these with
// localized suggestions; for now the strings are short, plain English.
func defaultWhenOptions() []string {
	return []string{"in 1 hour", "in 3 hours", "tomorrow 9am"}
}
