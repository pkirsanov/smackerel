package notification

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type SourceForm string

const (
	SourceFormStream   SourceForm = "stream"
	SourceFormWebhook  SourceForm = "webhook"
	SourceFormPolling  SourceForm = "polling"
	SourceFormQueue    SourceForm = "queue"
	SourceFormFileDrop SourceForm = "file_drop"
	SourceFormAPIPull  SourceForm = "api_pull"
	SourceFormManual   SourceForm = "manual"
)

type SourceHealthState string

const (
	SourceHealthConnected    SourceHealthState = "connected"
	SourceHealthDisconnected SourceHealthState = "disconnected"
	SourceHealthDegraded     SourceHealthState = "degraded"
)

type SourceAdapter interface {
	SourceType() string
	SourceForm() SourceForm
	InstanceID() string
	Connect(ctx context.Context, cfg SourceInstanceConfig) error
	Start(ctx context.Context, sink SourceEventSink) error
	Health(ctx context.Context) SourceHealthReport
	Stop(ctx context.Context) error
}

type SourceEventSink interface {
	SubmitSourceEvent(ctx context.Context, envelope SourceEventEnvelope) (IngestReceipt, error)
	ReportSourceHealth(ctx context.Context, report SourceHealthReport) error
}

type SourceInstanceConfig struct {
	SourceType       string
	SourceInstanceID string
	SourceForm       SourceForm
	Enabled          *bool
	ConfigHash       string
	SecretRefNames   []string
	RedactedMetadata map[string]string
}

type SourceKey struct {
	SourceType       string
	SourceInstanceID string
	SourceForm       SourceForm
}

type SourceEventEnvelope struct {
	SourceType           string
	SourceInstanceID     string
	SourceForm           SourceForm
	SourceEventID        string
	ObservedAt           time.Time
	EventTimestamp       *time.Time
	RawPayloadKind       string
	RawPayload           []byte
	DeliveryMetadata     map[string]string
	SourceSpecificFields map[string]string
	MappingHints         map[string]string
	LoopMetadata         map[string]string
}

type IngestReceipt struct {
	SourceType       string
	SourceInstanceID string
	SourceForm       SourceForm
	RawEventID       string
	Accepted         bool
	Status           string
}

type SourceHealthReport struct {
	SourceType            string
	SourceInstanceID      string
	SourceForm            SourceForm
	State                 SourceHealthState
	LastEventAt           *time.Time
	LastSuccessfulCheckAt *time.Time
	RetryCount            int
	LastErrorKind         string
	LastErrorRedacted     string
	ObservedAt            time.Time
}

type SourceStatus struct {
	Config SourceInstanceRecord
	Health SourceHealthReport
}

type SourceInstanceRecord struct {
	SourceType       string
	SourceInstanceID string
	SourceForm       SourceForm
	Enabled          bool
	ConfigHash       string
	SecretRefNames   []string
	RedactedMetadata map[string]string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (f SourceForm) Valid() bool {
	switch f {
	case SourceFormStream, SourceFormWebhook, SourceFormPolling, SourceFormQueue, SourceFormFileDrop, SourceFormAPIPull, SourceFormManual:
		return true
	default:
		return false
	}
}

func (s SourceHealthState) Valid() bool {
	switch s {
	case SourceHealthConnected, SourceHealthDisconnected, SourceHealthDegraded:
		return true
	default:
		return false
	}
}

func (c SourceInstanceConfig) Validate() error {
	if strings.TrimSpace(c.SourceType) == "" {
		return fmt.Errorf("notification source config: source type is required")
	}
	if strings.TrimSpace(c.SourceInstanceID) == "" {
		return fmt.Errorf("notification source config: source instance id is required")
	}
	if !c.SourceForm.Valid() {
		return fmt.Errorf("notification source config: source form %q is invalid", c.SourceForm)
	}
	if c.Enabled == nil {
		return fmt.Errorf("notification source config: enabled flag is required")
	}
	if strings.TrimSpace(c.ConfigHash) == "" {
		return fmt.Errorf("notification source config: config hash is required")
	}
	if len(c.SecretRefNames) == 0 {
		return fmt.Errorf("notification source config: at least one secret reference name is required")
	}
	for _, name := range c.SecretRefNames {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("notification source config: secret reference names must be non-empty")
		}
	}
	if len(c.RedactedMetadata) == 0 {
		return fmt.Errorf("notification source config: redacted metadata is required")
	}
	for key, value := range c.RedactedMetadata {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return fmt.Errorf("notification source config: redacted metadata keys and values must be non-empty")
		}
	}
	return nil
}

func (c SourceInstanceConfig) Key() SourceKey {
	return SourceKey{SourceType: strings.TrimSpace(c.SourceType), SourceInstanceID: strings.TrimSpace(c.SourceInstanceID), SourceForm: c.SourceForm}
}

func (r SourceHealthReport) Validate() error {
	if strings.TrimSpace(r.SourceType) == "" {
		return fmt.Errorf("notification source health: source type is required")
	}
	if strings.TrimSpace(r.SourceInstanceID) == "" {
		return fmt.Errorf("notification source health: source instance id is required")
	}
	if !r.SourceForm.Valid() {
		return fmt.Errorf("notification source health: source form %q is invalid", r.SourceForm)
	}
	if !r.State.Valid() {
		return fmt.Errorf("notification source health: state %q is invalid", r.State)
	}
	if r.RetryCount < 0 {
		return fmt.Errorf("notification source health: retry count must be non-negative")
	}
	if r.ObservedAt.IsZero() {
		return fmt.Errorf("notification source health: observed_at is required")
	}
	return nil
}
