package notification

import (
	"context"
	"fmt"
	"strings"
)

const (
	DeliverySent = "sent"
)

type DeliveryRequest struct {
	DecisionID        string
	IncidentID        string
	ApprovalRequestID string
	Channel           string
	DestinationRef    string
	SourceType        string
	SourceInstanceID  string
	Title             string
	Body              string
	ActionText        string
	Message           string
}

type DeliveryResult struct {
	Status        string
	ErrorKind     string
	ErrorRedacted string
	MutatesPolicy bool
}

type OutputChannel interface {
	ID() string
	Deliver(context.Context, DeliveryRequest) (DeliveryResult, error)
}

type OutputDispatcher struct {
	channels map[string]OutputChannel
}

func NewOutputDispatcher(channels []OutputChannel) OutputDispatcher {
	byID := make(map[string]OutputChannel, len(channels))
	for _, channel := range channels {
		if channel == nil || strings.TrimSpace(channel.ID()) == "" {
			continue
		}
		byID[channel.ID()] = channel
	}
	return OutputDispatcher{channels: byID}
}

func (d OutputDispatcher) Dispatch(ctx context.Context, request DeliveryRequest) (DeliveryResult, error) {
	channel, ok := d.channels[request.Channel]
	if !ok {
		return DeliveryResult{Status: "failed", ErrorKind: "channel_not_configured", ErrorRedacted: "output channel is not configured"}, fmt.Errorf("output channel %q is not configured", request.Channel)
	}
	request.Message = BuildDeliveryMessage(request)
	result, err := channel.Deliver(ctx, request)
	if err != nil {
		return result, err
	}
	if result.MutatesPolicy {
		return DeliveryResult{Status: "failed", ErrorKind: "policy_mutation_refused", ErrorRedacted: "output channel cannot mutate notification policy"}, fmt.Errorf("output channel %q attempted to mutate core policy", request.Channel)
	}
	if result.Status == "" {
		result.Status = DeliverySent
	}
	return result, nil
}

func BuildDeliveryMessage(request DeliveryRequest) string {
	body, _ := RedactText(request.Body)
	title, _ := RedactText(request.Title)
	parts := []string{
		"Source " + request.SourceType + "/" + request.SourceInstanceID,
		"Incident " + request.IncidentID,
		strings.TrimSpace(title),
		strings.TrimSpace(body),
		strings.TrimSpace(request.ActionText),
	}
	kept := []string{}
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, " | ")
}
