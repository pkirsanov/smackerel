package notification

import (
	"context"
	"strings"
	"testing"
)

func TestOutputDispatcherBuildsConciseRedactedSourceQualifiedMessage(t *testing.T) {
	channel := &recordingOutputChannel{id: "dashboard"}
	dispatcher := NewOutputDispatcher([]OutputChannel{channel})
	request := DeliveryRequest{DecisionID: "decision-output-a", IncidentID: "incident-output-a", Channel: "dashboard", DestinationRef: "operator", SourceType: "webhook_fixture", SourceInstanceID: "source-a", Title: "Token leaked", Body: "bearer secret-token should be hidden", ActionText: "Approve diagnostic"}
	result, err := dispatcher.Dispatch(context.Background(), request)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result.Status != DeliverySent || len(channel.requests) != 1 {
		t.Fatalf("delivery did not reach output channel: result=%+v requests=%d", result, len(channel.requests))
	}
	message := channel.requests[0].Message
	if strings.Contains(message, "secret-token") || !strings.Contains(message, "source-a") || !strings.Contains(message, "Approve diagnostic") {
		t.Fatalf("message was not concise/redacted/source-qualified: %q", message)
	}
}

func TestOutputChannelResultCannotMutateCorePolicy(t *testing.T) {
	channel := &recordingOutputChannel{id: "dashboard", result: DeliveryResult{Status: DeliverySent, MutatesPolicy: true}}
	dispatcher := NewOutputDispatcher([]OutputChannel{channel})
	_, err := dispatcher.Dispatch(context.Background(), DeliveryRequest{DecisionID: "decision-output-b", IncidentID: "incident-output-b", Channel: "dashboard", DestinationRef: "operator", SourceType: "queue_fixture", SourceInstanceID: "queue-a", Title: "needs attention", Body: "body"})
	if err == nil {
		t.Fatal("expected policy mutation attempt to be rejected")
	}
}

type recordingOutputChannel struct {
	id       string
	result   DeliveryResult
	requests []DeliveryRequest
}

func (c *recordingOutputChannel) ID() string { return c.id }
func (c *recordingOutputChannel) Deliver(ctx context.Context, request DeliveryRequest) (DeliveryResult, error) {
	c.requests = append(c.requests, request)
	if c.result.Status != "" || c.result.MutatesPolicy {
		return c.result, nil
	}
	return DeliveryResult{Status: DeliverySent}, nil
}
