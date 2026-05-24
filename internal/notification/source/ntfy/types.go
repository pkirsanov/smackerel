package ntfy

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
)

const (
	SourceType = "ntfy"

	TransportModeStream  = "stream"
	TransportModeWebhook = "webhook"

	AuthModeBearerToken = "bearer_token"
	AuthModeBasic       = "basic"
	AuthModeNone        = "none"

	ErrorAuthFailed              = "auth_failed"
	ErrorConnectivityFailed      = "connectivity_failed"
	ErrorRetryBudgetExhausted    = "retry_budget_exhausted"
	ErrorDeadLetterPressure      = "dead_letter_pressure"
	ErrorInvalidConfig           = "invalid_config"
	ErrorMissingSourceInstanceID = "missing_source_instance_id"
	ErrorMissingSourceForm       = "missing_source_form"
	ErrorMissingTransportMode    = "missing_transport_mode"
	ErrorMissingEndpoint         = "missing_endpoint"
	ErrorMissingTopics           = "missing_topics"
	ErrorCredentialRefMissing    = "credential_ref_missing"
	ErrorInvalidAuthMode         = "invalid_auth_mode"
	ErrorMissingConfigHash       = "missing_config_hash"
	ErrorMissingRedactedMetadata = "missing_redacted_metadata"
)

type AuthConfig struct {
	Mode           string
	SecretRefNames []string
}

type MappingConfig struct {
	DefaultDomain string
	TopicSubjects map[string]string
	TagServices   map[string]string
	TagIntents    map[string]string
}

type ReconnectConfig struct {
	RetryBudget             int
	InitialDelaySeconds     int
	MaxDelaySeconds         int
	KeepaliveTimeoutSeconds int
}

type LagConfig struct {
	DegradedAfterSeconds     int
	DisconnectedAfterSeconds int
}

type DeadLetterConfig struct {
	RetryBudget            int
	MaxPayloadBytes        int
	PressureThresholdCount int
}

type Config struct {
	Enabled          bool
	SourceInstanceID string
	SourceForm       notification.SourceForm
	TransportMode    string
	EndpointURL      string
	EndpointRefName  string
	Topics           []string
	Auth             AuthConfig
	Mapping          MappingConfig
	Reconnect        ReconnectConfig
	Lag              LagConfig
	DeadLetter       DeadLetterConfig
	RedactedMetadata map[string]string
	ConfigHash       string
}

type Adapter struct {
	cfg             Config
	mu              sync.RWMutex
	health          notification.SourceHealthReport
	streamClient    StreamClient
	webhookReceiver WebhookReceiver
	store           *Store
	cancel          context.CancelFunc
	done            chan struct{}
	running         bool
}

type AdapterOption func(*Adapter)

func NewAdapter(cfg Config, options ...AdapterOption) (*Adapter, error) {
	if _, err := cfg.SourceInstanceConfig(); err != nil {
		return nil, err
	}
	adapter := &Adapter{cfg: cfg, health: disconnectedHealth(cfg, ErrorConnectivityFailed, time.Now().UTC()), streamClient: NewHTTPStreamClient(nil)}
	for _, option := range options {
		option(adapter)
	}
	return adapter, nil
}

func (a *Adapter) SourceType() string { return SourceType }

func (a *Adapter) SourceForm() notification.SourceForm { return a.cfg.SourceForm }

func (a *Adapter) InstanceID() string { return a.cfg.SourceInstanceID }

func (a *Adapter) Connect(ctx context.Context, cfg notification.SourceInstanceConfig) error {
	if err := cfg.Validate(); err != nil {
		a.setHealth(disconnectedHealth(a.cfg, ErrorMissingEndpoint, time.Now().UTC()))
		return err
	}
	if cfg.SourceType != SourceType || cfg.SourceInstanceID != a.cfg.SourceInstanceID || cfg.SourceForm != a.cfg.SourceForm {
		a.setHealth(disconnectedHealth(a.cfg, ErrorConnectivityFailed, time.Now().UTC()))
		return fmt.Errorf("ntfy source adapter: source instance config does not match adapter identity")
	}
	now := time.Now().UTC()
	a.setHealth(notification.SourceHealthReport{SourceType: SourceType, SourceInstanceID: a.cfg.SourceInstanceID, SourceForm: a.cfg.SourceForm, State: notification.SourceHealthConnected, LastSuccessfulCheckAt: &now, ObservedAt: now})
	return nil
}

func (a *Adapter) Health(ctx context.Context) notification.SourceHealthReport {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.health
}

func (c Config) SourceInstanceConfig() (notification.SourceInstanceConfig, error) {
	if strings.TrimSpace(c.SourceInstanceID) == "" {
		return notification.SourceInstanceConfig{}, fmt.Errorf("ntfy source config: source instance id is required")
	}
	if !c.SourceForm.Valid() || (c.SourceForm != notification.SourceFormStream && c.SourceForm != notification.SourceFormWebhook) {
		return notification.SourceInstanceConfig{}, fmt.Errorf("ntfy source config: source form must be stream or webhook")
	}
	if c.TransportMode != TransportModeStream && c.TransportMode != TransportModeWebhook {
		return notification.SourceInstanceConfig{}, fmt.Errorf("ntfy source config: transport mode must be stream or webhook")
	}
	if string(c.SourceForm) != c.TransportMode {
		return notification.SourceInstanceConfig{}, fmt.Errorf("ntfy source config: source form must match transport mode")
	}
	if strings.TrimSpace(c.EndpointURL) == "" || strings.TrimSpace(c.EndpointRefName) == "" {
		return notification.SourceInstanceConfig{}, fmt.Errorf("ntfy source config: endpoint identity is required")
	}
	if len(c.Topics) == 0 {
		return notification.SourceInstanceConfig{}, fmt.Errorf("ntfy source config: topic set is required")
	}
	for _, topic := range c.Topics {
		if strings.TrimSpace(topic) == "" {
			return notification.SourceInstanceConfig{}, fmt.Errorf("ntfy source config: topics must be non-empty")
		}
	}
	if c.Auth.Mode != AuthModeBearerToken && c.Auth.Mode != AuthModeBasic && c.Auth.Mode != AuthModeNone {
		return notification.SourceInstanceConfig{}, fmt.Errorf("ntfy source config: auth mode must be bearer_token, basic, or none")
	}
	if c.Auth.Mode != AuthModeNone && len(c.Auth.SecretRefNames) == 0 {
		return notification.SourceInstanceConfig{}, fmt.Errorf("ntfy source config: secret reference names are required for auth mode %s", c.Auth.Mode)
	}
	for _, name := range c.Auth.SecretRefNames {
		if strings.TrimSpace(name) == "" {
			return notification.SourceInstanceConfig{}, fmt.Errorf("ntfy source config: secret reference names must be non-empty")
		}
	}
	if strings.TrimSpace(c.ConfigHash) == "" {
		return notification.SourceInstanceConfig{}, fmt.Errorf("ntfy source config: config hash is required")
	}
	if len(c.RedactedMetadata) == 0 {
		return notification.SourceInstanceConfig{}, fmt.Errorf("ntfy source config: redacted metadata is required")
	}
	if err := validatePositivePolicy(c); err != nil {
		return notification.SourceInstanceConfig{}, err
	}
	metadata := cloneStringMap(c.RedactedMetadata)
	metadata["auth_mode"] = c.Auth.Mode
	metadata["transport_mode"] = c.TransportMode
	metadata["topic_count"] = fmt.Sprintf("%d", len(c.Topics))
	metadata["topics"] = strings.Join(c.Topics, ",")
	metadata["retry_budget"] = fmt.Sprintf("%d", c.Reconnect.RetryBudget)
	metadata["reconnect_initial_delay_seconds"] = fmt.Sprintf("%d", c.Reconnect.InitialDelaySeconds)
	metadata["reconnect_max_delay_seconds"] = fmt.Sprintf("%d", c.Reconnect.MaxDelaySeconds)
	metadata["keepalive_timeout_seconds"] = fmt.Sprintf("%d", c.Reconnect.KeepaliveTimeoutSeconds)
	metadata["lag_degraded_after_seconds"] = fmt.Sprintf("%d", c.Lag.DegradedAfterSeconds)
	metadata["lag_disconnected_after_seconds"] = fmt.Sprintf("%d", c.Lag.DisconnectedAfterSeconds)
	metadata["dead_letter_retry_budget"] = fmt.Sprintf("%d", c.DeadLetter.RetryBudget)
	metadata["max_payload_bytes"] = fmt.Sprintf("%d", c.DeadLetter.MaxPayloadBytes)
	metadata["pressure_threshold_count"] = fmt.Sprintf("%d", c.DeadLetter.PressureThresholdCount)
	metadata["endpoint_ref_name"] = c.EndpointRefName
	enabled := c.Enabled
	instance := notification.SourceInstanceConfig{SourceType: SourceType, SourceInstanceID: strings.TrimSpace(c.SourceInstanceID), SourceForm: c.SourceForm, Enabled: &enabled, ConfigHash: strings.TrimSpace(c.ConfigHash), SecretRefNames: trimStrings(c.Auth.SecretRefNames), RedactedMetadata: metadata}
	if err := instance.Validate(); err != nil {
		return notification.SourceInstanceConfig{}, err
	}
	return instance, nil
}

func validatePositivePolicy(c Config) error {
	checks := []struct {
		name  string
		value int
	}{
		{name: "reconnect retry budget", value: c.Reconnect.RetryBudget},
		{name: "reconnect initial delay", value: c.Reconnect.InitialDelaySeconds},
		{name: "reconnect max delay", value: c.Reconnect.MaxDelaySeconds},
		{name: "keepalive timeout", value: c.Reconnect.KeepaliveTimeoutSeconds},
		{name: "lag degraded threshold", value: c.Lag.DegradedAfterSeconds},
		{name: "lag disconnected threshold", value: c.Lag.DisconnectedAfterSeconds},
		{name: "dead-letter retry budget", value: c.DeadLetter.RetryBudget},
		{name: "max payload bytes", value: c.DeadLetter.MaxPayloadBytes},
		{name: "dead-letter pressure threshold", value: c.DeadLetter.PressureThresholdCount},
	}
	for _, check := range checks {
		if check.value <= 0 {
			return fmt.Errorf("ntfy source config: %s must be positive", check.name)
		}
	}
	if c.Reconnect.InitialDelaySeconds > c.Reconnect.MaxDelaySeconds {
		return fmt.Errorf("ntfy source config: reconnect initial delay must not exceed max delay")
	}
	if c.Lag.DegradedAfterSeconds >= c.Lag.DisconnectedAfterSeconds {
		return fmt.Errorf("ntfy source config: lag degraded threshold must be lower than disconnected threshold")
	}
	return nil
}

func trimStrings(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		if s := strings.TrimSpace(value); s != "" {
			trimmed = append(trimmed, s)
		}
	}
	return trimmed
}

func cloneStringMap(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
