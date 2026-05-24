package ntfy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
)

type configJSON struct {
	SourceInstanceID            string            `json:"source_instance_id"`
	Enabled                     bool              `json:"enabled"`
	SourceForm                  string            `json:"source_form"`
	TransportMode               string            `json:"transport_mode"`
	EndpointURL                 string            `json:"endpoint_url"`
	EndpointRefName             string            `json:"endpoint_ref_name"`
	Topics                      []string          `json:"topics"`
	AuthMode                    string            `json:"auth_mode"`
	SecretRefNames              []string          `json:"secret_ref_names"`
	DefaultDomain               string            `json:"default_domain"`
	TopicSubjects               map[string]string `json:"topic_subjects"`
	TagServices                 map[string]string `json:"tag_services"`
	TagIntents                  map[string]string `json:"tag_intents"`
	RetryBudget                 int               `json:"retry_budget"`
	InitialDelaySeconds         int               `json:"initial_delay_seconds"`
	MaxDelaySeconds             int               `json:"max_delay_seconds"`
	KeepaliveTimeoutSeconds     int               `json:"keepalive_timeout_seconds"`
	LagDegradedAfterSeconds     int               `json:"lag_degraded_after_seconds"`
	LagDisconnectedAfterSeconds int               `json:"lag_disconnected_after_seconds"`
	DeadLetterRetryBudget       int               `json:"dead_letter_retry_budget"`
	MaxPayloadBytes             int               `json:"max_payload_bytes"`
	PressureThresholdCount      int               `json:"pressure_threshold_count"`
	DisplayName                 string            `json:"display_name"`
	EndpointLabel               string            `json:"endpoint_label"`
	ConfigHash                  string            `json:"config_hash"`
}

type parsedConfigEntry struct {
	cfg   Config
	raw   json.RawMessage
	index int
}

func ParseConfigs(raw string) ([]Config, error) {
	entries, err := parseConfigEntries(raw)
	if err != nil {
		return nil, err
	}
	configs := make([]Config, 0, len(entries))
	for _, entry := range entries {
		configs = append(configs, entry.cfg)
	}
	return configs, nil
}

func parseConfigEntries(raw string) ([]parsedConfigEntry, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("ntfy config: NTFY_SOURCES_JSON is required")
	}
	var encoded []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &encoded); err != nil {
		return nil, fmt.Errorf("ntfy config: NTFY_SOURCES_JSON must be a JSON array: %w", err)
	}
	entries := make([]parsedConfigEntry, 0, len(encoded))
	for index, rawItem := range encoded {
		var item configJSON
		if err := json.Unmarshal(rawItem, &item); err != nil {
			return nil, fmt.Errorf("ntfy config: source entry %d must be an object: %w", index+1, err)
		}
		entries = append(entries, parsedConfigEntry{cfg: configFromJSON(item), raw: append([]byte(nil), rawItem...), index: index})
	}
	return entries, nil
}

func configFromJSON(item configJSON) Config {
	return Config{
		Enabled:          item.Enabled,
		SourceInstanceID: item.SourceInstanceID,
		SourceForm:       notification.SourceForm(item.SourceForm),
		TransportMode:    item.TransportMode,
		EndpointURL:      item.EndpointURL,
		EndpointRefName:  item.EndpointRefName,
		Topics:           append([]string(nil), item.Topics...),
		Auth: AuthConfig{
			Mode:           item.AuthMode,
			SecretRefNames: append([]string(nil), item.SecretRefNames...),
		},
		Mapping: MappingConfig{
			DefaultDomain: item.DefaultDomain,
			TopicSubjects: cloneStringMap(item.TopicSubjects),
			TagServices:   cloneStringMap(item.TagServices),
			TagIntents:    cloneStringMap(item.TagIntents),
		},
		Reconnect: ReconnectConfig{
			RetryBudget:             item.RetryBudget,
			InitialDelaySeconds:     item.InitialDelaySeconds,
			MaxDelaySeconds:         item.MaxDelaySeconds,
			KeepaliveTimeoutSeconds: item.KeepaliveTimeoutSeconds,
		},
		Lag: LagConfig{
			DegradedAfterSeconds:     item.LagDegradedAfterSeconds,
			DisconnectedAfterSeconds: item.LagDisconnectedAfterSeconds,
		},
		DeadLetter: DeadLetterConfig{
			RetryBudget:            item.DeadLetterRetryBudget,
			MaxPayloadBytes:        item.MaxPayloadBytes,
			PressureThresholdCount: item.PressureThresholdCount,
		},
		RedactedMetadata: redactedMetadataFromJSON(item),
		ConfigHash:       item.ConfigHash,
	}
}

func redactedMetadataFromJSON(item configJSON) map[string]string {
	return map[string]string{"display_name": item.DisplayName, "endpoint_label": item.EndpointLabel}
}

func BootstrapConfiguredSources(ctx context.Context, raw string, store *notification.Store, observedAt time.Time) error {
	if store == nil {
		return fmt.Errorf("ntfy config: notification store is required")
	}
	entries, err := parseConfigEntries(raw)
	if err != nil {
		return err
	}
	var validationErrors []error
	for _, entry := range entries {
		cfg := entry.cfg
		if !cfg.Enabled {
			continue
		}
		instance, err := cfg.SourceInstanceConfig()
		if err != nil {
			if diagnosticErr := recordInvalidEnabledSourceHealth(ctx, store, cfg, entry.index, entry.raw, err, observedAt); diagnosticErr != nil {
				validationErrors = append(validationErrors, diagnosticErr)
			}
			validationErrors = append(validationErrors, err)
			continue
		}
		if err := store.EnsureSourceInstance(ctx, instance, observedAt); err != nil {
			return err
		}
	}
	return errors.Join(validationErrors...)
}

func recordInvalidEnabledSourceHealth(ctx context.Context, store *notification.Store, cfg Config, index int, raw json.RawMessage, validationErr error, observedAt time.Time) error {
	instance := diagnosticSourceInstanceConfig(cfg, index, raw, validationErr)
	if err := store.EnsureSourceInstance(ctx, instance, observedAt); err != nil {
		return fmt.Errorf("ntfy config: register invalid enabled source diagnostics: %w", err)
	}
	report := notification.SourceHealthReport{SourceType: SourceType, SourceInstanceID: instance.SourceInstanceID, SourceForm: instance.SourceForm, State: notification.SourceHealthDisconnected, LastErrorKind: instance.RedactedMetadata["config_error_kind"], ObservedAt: observedAt}
	if err := store.RecordSourceHealth(ctx, report); err != nil {
		return fmt.Errorf("ntfy config: record invalid enabled source health: %w", err)
	}
	return nil
}

func diagnosticSourceInstanceConfig(cfg Config, index int, raw json.RawMessage, validationErr error) notification.SourceInstanceConfig {
	enabled := cfg.Enabled
	sourceID := diagnosticSourceInstanceID(cfg, index, raw)
	form := cfg.SourceForm
	if !form.Valid() {
		form = notification.SourceFormManual
	}
	configHash := strings.TrimSpace(cfg.ConfigHash)
	if configHash == "" {
		configHash = diagnosticConfigHash(raw)
	}
	metadata := diagnosticMetadata(cfg, validationErr)
	return notification.SourceInstanceConfig{SourceType: SourceType, SourceInstanceID: sourceID, SourceForm: form, Enabled: &enabled, ConfigHash: configHash, SecretRefNames: trimStrings(cfg.Auth.SecretRefNames), RedactedMetadata: metadata}
}

func diagnosticSourceInstanceID(cfg Config, index int, raw json.RawMessage) string {
	if sourceID := strings.TrimSpace(cfg.SourceInstanceID); sourceID != "" {
		return sourceID
	}
	hash := strings.TrimPrefix(notification.PayloadHash(raw), "sha256:")
	if len(hash) > 12 {
		hash = hash[:12]
	}
	return fmt.Sprintf("ntfy-invalid-config-%02d-%s", index+1, hash)
}

func diagnosticConfigHash(raw json.RawMessage) string {
	return "invalid:" + strings.TrimPrefix(notification.PayloadHash(raw), "sha256:")
}

func diagnosticMetadata(cfg Config, validationErr error) map[string]string {
	metadata := map[string]string{}
	for key, value := range cfg.RedactedMetadata {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			metadata[key] = trimmed
		}
	}
	metadata["config_status"] = "invalid"
	metadata["config_error_kind"] = configValidationErrorKind(validationErr)
	metadata["config_error_redacted"] = "enabled ntfy source configuration is missing required explicit fields"
	metadata["auth_mode"] = nonEmptyOr(cfg.Auth.Mode, "missing")
	metadata["transport_mode"] = nonEmptyOr(cfg.TransportMode, "missing")
	metadata["source_form"] = nonEmptyOr(string(cfg.SourceForm), "missing")
	metadata["topic_count"] = fmt.Sprintf("%d", len(cfg.Topics))
	metadata["topics"] = nonEmptyOr(strings.Join(trimStrings(cfg.Topics), ","), "missing")
	metadata["endpoint_ref_name"] = nonEmptyOr(cfg.EndpointRefName, "missing")
	metadata["display_name"] = nonEmptyOr(metadata["display_name"], "invalid ntfy source config")
	metadata["endpoint_label"] = nonEmptyOr(metadata["endpoint_label"], "invalid ntfy endpoint identity")
	return metadata
}

func configValidationErrorKind(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "source instance id"):
		return ErrorMissingSourceInstanceID
	case strings.Contains(message, "source form"):
		return ErrorMissingSourceForm
	case strings.Contains(message, "transport mode"):
		return ErrorMissingTransportMode
	case strings.Contains(message, "endpoint"):
		return ErrorMissingEndpoint
	case strings.Contains(message, "topic"):
		return ErrorMissingTopics
	case strings.Contains(message, "secret reference"):
		return ErrorCredentialRefMissing
	case strings.Contains(message, "auth mode"):
		return ErrorInvalidAuthMode
	case strings.Contains(message, "config hash"):
		return ErrorMissingConfigHash
	case strings.Contains(message, "redacted metadata"):
		return ErrorMissingRedactedMetadata
	default:
		return ErrorInvalidConfig
	}
}

func nonEmptyOr(value string, replacement string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return replacement
}
