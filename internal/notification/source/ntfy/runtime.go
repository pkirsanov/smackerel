package ntfy

import (
	"context"
	"fmt"

	"github.com/smackerel/smackerel/internal/notification"
)

type Runtime struct {
	adapters        []*Adapter
	webhookReceiver *WebhookReceiverRegistry
}

type RuntimeOption func(*runtimeOptions)

type runtimeOptions struct {
	streamClient    StreamClient
	webhookReceiver *WebhookReceiverRegistry
	store           *Store
}

func WithRuntimeStreamClient(client StreamClient) RuntimeOption {
	return func(options *runtimeOptions) {
		options.streamClient = client
	}
}

func WithRuntimeWebhookReceiver(receiver *WebhookReceiverRegistry) RuntimeOption {
	return func(options *runtimeOptions) {
		options.webhookReceiver = receiver
	}
}

func WithRuntimeStore(store *Store) RuntimeOption {
	return func(options *runtimeOptions) {
		options.store = store
	}
}

func StartConfiguredAdapters(ctx context.Context, raw string, sink notification.SourceEventSink, opts ...RuntimeOption) (*Runtime, error) {
	if sink == nil {
		return nil, fmt.Errorf("ntfy runtime: source event sink is required")
	}
	options := runtimeOptions{webhookReceiver: NewWebhookReceiverRegistry()}
	for _, option := range opts {
		option(&options)
	}
	if options.webhookReceiver == nil {
		return nil, fmt.Errorf("ntfy runtime: webhook receiver registry is required")
	}
	configs, err := ParseConfigs(raw)
	if err != nil {
		return nil, err
	}
	runtime := &Runtime{webhookReceiver: options.webhookReceiver}
	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		adapterOptions := []AdapterOption{}
		if cfg.TransportMode == TransportModeStream && options.streamClient != nil {
			adapterOptions = append(adapterOptions, WithStreamClient(options.streamClient))
		}
		if cfg.TransportMode == TransportModeWebhook {
			adapterOptions = append(adapterOptions, WithWebhookReceiver(options.webhookReceiver))
		}
		if options.store != nil {
			adapterOptions = append(adapterOptions, WithStore(options.store))
		}
		adapter, err := NewAdapter(cfg, adapterOptions...)
		if err != nil {
			runtime.stopBestEffort(context.Background())
			return nil, err
		}
		if err := adapter.Start(ctx, sink); err != nil {
			runtime.stopBestEffort(context.Background())
			return nil, err
		}
		runtime.adapters = append(runtime.adapters, adapter)
	}
	return runtime, nil
}

func (runtime *Runtime) WebhookReceiver() *WebhookReceiverRegistry {
	if runtime == nil {
		return nil
	}
	return runtime.webhookReceiver
}

func (runtime *Runtime) Stop(ctx context.Context) error {
	if runtime == nil {
		return nil
	}
	var firstErr error
	for _, adapter := range runtime.adapters {
		if err := adapter.Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (runtime *Runtime) stopBestEffort(ctx context.Context) {
	_ = runtime.Stop(ctx)
}
