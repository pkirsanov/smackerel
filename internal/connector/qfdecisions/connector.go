package qfdecisions

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/smackerel/smackerel/internal/connector"
)

const maxPageSize = 100

var _ connector.Connector = (*Connector)(nil)

type QFConfig struct {
	BaseURL       string
	CredentialRef string
	SyncSchedule  string
	PacketVersion int
	PageSize      int
}

type Connector struct {
	id     string
	mu     sync.RWMutex
	client *Client
	cfg    QFConfig
	health connector.HealthStatus
}

func New(id string) *Connector {
	if strings.TrimSpace(id) == "" {
		id = DefaultConnectorID
	}
	return &Connector{id: id, health: connector.HealthDisconnected}
}

func (c *Connector) ID() string {
	return c.id
}

func (c *Connector) Connect(ctx context.Context, cfg connector.ConnectorConfig) error {
	parsed, err := parseConfig(cfg)
	if err != nil {
		c.setHealth(connector.HealthError)
		return err
	}

	client := NewClient(parsed.BaseURL, parsed.CredentialRef, parsed.PacketVersion, parsed.PageSize)
	c.mu.Lock()
	c.client = client
	c.cfg = parsed
	c.mu.Unlock()

	if err := client.Validate(ctx); err != nil {
		c.setHealth(healthForBridgeError(err))
		return fmt.Errorf("validate QF bridge contract: %w", err)
	}

	c.setHealth(connector.HealthHealthy)
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return nil, cursor, fmt.Errorf("qf-decisions connector is not connected")
	}
	if err := client.Validate(ctx); err != nil {
		c.setHealth(healthForBridgeError(err))
		return nil, cursor, fmt.Errorf("validate QF bridge contract during sync: %w", err)
	}
	c.setHealth(connector.HealthHealthy)
	return []connector.RawArtifact{}, cursor, nil
}

func (c *Connector) Health(context.Context) connector.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}

func (c *Connector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.client = nil
	c.health = connector.HealthDisconnected
	return nil
}

func (c *Connector) setHealth(health connector.HealthStatus) {
	c.mu.Lock()
	c.health = health
	c.mu.Unlock()
}

func healthForBridgeError(err error) connector.HealthStatus {
	var schemaErr SchemaCompatibilityError
	if errors.As(err, &schemaErr) {
		return connector.HealthDegraded
	}
	return connector.HealthError
}

func parseConfig(cfg connector.ConnectorConfig) (QFConfig, error) {
	var configErrors []string

	baseURL, err := sourceString(cfg.SourceConfig, "base_url")
	if err != nil {
		configErrors = append(configErrors, err.Error())
	} else if err := validateBaseURL(baseURL); err != nil {
		configErrors = append(configErrors, err.Error())
	}

	credentialRef := strings.TrimSpace(cfg.Credentials["credential_ref"])
	if credentialRef == "" {
		configErrors = append(configErrors, "credential_ref is required")
	}

	syncSchedule := strings.TrimSpace(cfg.SyncSchedule)
	if syncSchedule == "" {
		configErrors = append(configErrors, "sync_schedule is required")
	} else if !validCron(syncSchedule) {
		configErrors = append(configErrors, "sync_schedule is not a valid five-field cron expression")
	}

	packetVersion, err := sourcePositiveInt(cfg.SourceConfig, "packet_version")
	if err != nil {
		configErrors = append(configErrors, err.Error())
	}
	pageSize, err := sourcePositiveInt(cfg.SourceConfig, "page_size")
	if err != nil {
		configErrors = append(configErrors, err.Error())
	} else if pageSize > maxPageSize {
		configErrors = append(configErrors, fmt.Sprintf("page_size must be between 1 and %d", maxPageSize))
	}
	if len(configErrors) > 0 {
		return QFConfig{}, fmt.Errorf("invalid qf-decisions connector configuration: %s", strings.Join(configErrors, ", "))
	}

	return QFConfig{
		BaseURL:       strings.TrimRight(baseURL, "/"),
		CredentialRef: credentialRef,
		SyncSchedule:  syncSchedule,
		PacketVersion: packetVersion,
		PageSize:      pageSize,
	}, nil
}

func sourceString(source map[string]any, key string) (string, error) {
	value, ok := source[key]
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return strings.TrimSpace(text), nil
}

func sourcePositiveInt(source map[string]any, key string) (int, error) {
	value, ok := source[key]
	if !ok {
		return 0, fmt.Errorf("%s is required", key)
	}
	switch typed := value.(type) {
	case int:
		if typed < 1 {
			return 0, fmt.Errorf("%s must be a positive integer", key)
		}
		return typed, nil
	case int64:
		if typed < 1 {
			return 0, fmt.Errorf("%s must be a positive integer", key)
		}
		return int(typed), nil
	case float64:
		if typed < 1 || typed != float64(int(typed)) {
			return 0, fmt.Errorf("%s must be a positive integer", key)
		}
		return int(typed), nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil || parsed < 1 {
			return 0, fmt.Errorf("%s must be a positive integer", key)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
}

func validateBaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("base_url is invalid: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("base_url must use http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("base_url must include a host")
	}
	return nil
}

func validCron(expr string) bool {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return false
	}
	for _, field := range fields {
		if field == "" {
			return false
		}
	}
	return true
}
