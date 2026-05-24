package ntfy

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrWebhookSourceNotRunning = errors.New("ntfy webhook receiver: source is not running")
	ErrWebhookPayloadInvalid   = errors.New("ntfy webhook receiver: payload is invalid")
)

type WebhookReceiverRegistry struct {
	mu      sync.RWMutex
	entries map[string]webhookReceiverEntry
}

type webhookReceiverEntry struct {
	cfg     Config
	handler WebhookHandler
}

func NewWebhookReceiverRegistry() *WebhookReceiverRegistry {
	return &WebhookReceiverRegistry{entries: map[string]webhookReceiverEntry{}}
}

func (registry *WebhookReceiverRegistry) Start(ctx context.Context, cfg Config, handler WebhookHandler) error {
	if registry == nil {
		return fmt.Errorf("ntfy webhook receiver: registry is required")
	}
	if handler == nil {
		return fmt.Errorf("ntfy webhook receiver: handler is required")
	}
	if _, err := cfg.SourceInstanceConfig(); err != nil {
		return err
	}
	if cfg.TransportMode != TransportModeWebhook {
		return fmt.Errorf("ntfy webhook receiver: transport mode must be webhook")
	}
	registry.mu.Lock()
	if _, exists := registry.entries[cfg.SourceInstanceID]; exists {
		registry.mu.Unlock()
		return fmt.Errorf("ntfy webhook receiver: source instance %s is already registered", cfg.SourceInstanceID)
	}
	registry.entries[cfg.SourceInstanceID] = webhookReceiverEntry{cfg: cfg, handler: handler}
	registry.mu.Unlock()
	<-ctx.Done()
	registry.mu.Lock()
	delete(registry.entries, cfg.SourceInstanceID)
	registry.mu.Unlock()
	return ctx.Err()
}

func (registry *WebhookReceiverRegistry) ReceiveRaw(ctx context.Context, sourceInstanceID string, payload []byte) error {
	if registry == nil {
		return ErrWebhookSourceNotRunning
	}
	registry.mu.RLock()
	entry, ok := registry.entries[sourceInstanceID]
	registry.mu.RUnlock()
	if !ok {
		return ErrWebhookSourceNotRunning
	}
	event, err := ParseEvent(payload, entry.cfg.DeadLetter.MaxPayloadBytes)
	if err != nil {
		if payloadHandler, ok := entry.handler.(WebhookPayloadErrorHandler); ok {
			_ = payloadHandler.HandleNtfyPayloadError(ctx, payload, err)
		}
		return fmt.Errorf("%w: %v", ErrWebhookPayloadInvalid, err)
	}
	if err := entry.handler.HandleNtfyEvent(ctx, event); err != nil {
		return err
	}
	return nil
}

func (registry *WebhookReceiverRegistry) IsRegistered(sourceInstanceID string) bool {
	if registry == nil {
		return false
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	_, ok := registry.entries[sourceInstanceID]
	return ok
}

func IsWebhookSourceNotRunning(err error) bool {
	return errors.Is(err, ErrWebhookSourceNotRunning)
}

func IsWebhookPayloadInvalid(err error) bool {
	return errors.Is(err, ErrWebhookPayloadInvalid)
}
