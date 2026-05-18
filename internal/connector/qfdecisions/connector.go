package qfdecisions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/metrics"
)

const maxPageSize = 100

// Freshness SLA stage labels — match the `stage` label values on
// metrics.QFFreshnessP95Seconds and design.md §F12 budgets.
const (
	FreshnessStageIngest = "ingest"
	FreshnessStageRender = "render"
	FreshnessStageTotal  = "total"
)

// freshnessWindowSize bounds the per-stage rolling p95 sample buffer. 256 is
// large enough to produce a stable p95 (≥ 5 samples above the 95th percentile
// once the window fills) while keeping per-connector memory bounded.
const freshnessWindowSize = 256

var _ connector.Connector = (*Connector)(nil)

type QFConfig struct {
	BaseURL       string
	CredentialRef string
	SyncSchedule  string
	PacketVersion int
	PageSize      int
	// CursorLagThresholdSeconds is the operator-tunable threshold above
	// which the connector emits a structured `lag_breach` log event.
	// Defaults to 3600 (1 hour) when source_config does not provide it.
	// The connector NEVER auto-advances the cursor on breach — recovery
	// is operator-initiated via QF's POST /cursor:fast-forward (F13).
	// SCN-SM-041-007.
	CursorLagThresholdSeconds int
}

const defaultCursorLagThresholdSeconds = 3600

// freshnessWindow is a fixed-size ring buffer of observed freshness samples
// (in seconds) used to compute a rolling p95 for the
// smackerel_qf_freshness_p95_seconds gauge. The window keeps the most recent
// N observations and is safe for concurrent use by Sync ticks.
type freshnessWindow struct {
	mu      sync.Mutex
	samples []float64
	pos     int
	full    bool
}

func newFreshnessWindow(size int) *freshnessWindow {
	return &freshnessWindow{samples: make([]float64, size)}
}

// Add records a single freshness observation. Once the window is full,
// subsequent observations overwrite the oldest sample (FIFO).
func (w *freshnessWindow) Add(seconds float64) {
	w.mu.Lock()
	w.samples[w.pos] = seconds
	w.pos = (w.pos + 1) % len(w.samples)
	if w.pos == 0 {
		w.full = true
	}
	w.mu.Unlock()
}

// P95 returns the nearest-rank 95th percentile of the window's current
// observations (idx = ceil(0.95 * N) - 1 on the sorted snapshot). It returns
// (0, false) when no samples have been recorded so callers can suppress
// gauge updates while the window is empty.
func (w *freshnessWindow) P95() (float64, bool) {
	w.mu.Lock()
	size := len(w.samples)
	if !w.full {
		size = w.pos
	}
	if size == 0 {
		w.mu.Unlock()
		return 0, false
	}
	snap := make([]float64, size)
	copy(snap, w.samples[:size])
	w.mu.Unlock()

	sort.Float64s(snap)
	idx := int(math.Ceil(0.95*float64(size))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= size {
		idx = size - 1
	}
	return snap[idx], true
}

type Connector struct {
	id                  string
	mu                  sync.RWMutex
	client              *Client
	cfg                 QFConfig
	health              connector.HealthStatus
	capability          QFBridgeCapability
	capabilityStatus    string
	capabilityFetchedAt time.Time
	// freshness holds per-stage rolling p95 windows. Each window has its
	// own internal mutex so it is safe to read/write outside of c.mu.
	// SCN-SM-041-003, SCN-SM-041-008.
	freshness map[string]*freshnessWindow
}

func New(id string) *Connector {
	if strings.TrimSpace(id) == "" {
		id = DefaultConnectorID
	}
	return &Connector{
		id:               id,
		health:           connector.HealthDisconnected,
		capabilityStatus: CapabilityStatusUnfetched,
		freshness: map[string]*freshnessWindow{
			FreshnessStageIngest: newFreshnessWindow(freshnessWindowSize),
			FreshnessStageRender: newFreshnessWindow(freshnessWindowSize),
			FreshnessStageTotal:  newFreshnessWindow(freshnessWindowSize),
		},
	}
}

// recordFreshness records a freshness latency observation for `stage` and
// republishes the corresponding p95 gauge. Called for stage="ingest" inside
// Sync() once a packet has been successfully normalized into an artifact.
// stage="render" and stage="total" are wired by downstream render surfaces
// (Scope 5). A negative observation indicates clock skew between QF and
// smackerel hosts and is clamped to zero so the window never produces a
// misleading negative p95.
func (c *Connector) recordFreshness(stage string, latencySeconds float64) {
	if latencySeconds < 0 {
		latencySeconds = 0
	}
	w, ok := c.freshness[stage]
	if !ok {
		return
	}
	w.Add(latencySeconds)
	if p95, has := w.P95(); has {
		metrics.QFFreshnessP95Seconds.WithLabelValues(stage).Set(p95)
	}
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

	// Capability handshake replaces the legacy Validate() probe — FetchCapability
	// proves auth + reachability AND establishes the page-size + decision-type
	// contract the connector needs for safe polling. A single round-trip suffices.
	capability, err := client.FetchCapability(ctx)
	if err != nil {
		health := connector.HealthError
		var schemaErr SchemaCompatibilityError
		if errors.As(err, &schemaErr) {
			health = connector.HealthDegraded
		}
		c.mu.Lock()
		c.capabilityStatus = CapabilityStatusUnfetched
		c.mu.Unlock()
		c.setHealth(health)
		return fmt.Errorf("qf capability handshake: %w", err)
	}
	if err := capability.CompatibilityCheck(); err != nil {
		c.mu.Lock()
		c.capability = capability
		c.capabilityStatus = CapabilityStatusIncompatible
		c.capabilityFetchedAt = time.Now().UTC()
		c.mu.Unlock()
		c.setHealth(connector.HealthDegraded)
		// CompatibilityCheck already incremented metrics.QFCapabilityMismatch.
		return fmt.Errorf("qf capability incompatible: %w", err)
	}

	// Clamp configured page size to capability max BEFORE publishing the client
	// so subsequent Sync calls always issue requests within QF-allowed bounds.
	clamped := client.ClampPageSize(client.pageSize, capability.MaxPageSize)
	if clamped != client.pageSize {
		client.pageSize = clamped
	}

	c.mu.Lock()
	c.client = client
	c.cfg = parsed
	c.capability = capability
	c.capabilityStatus = CapabilityStatusCompatible
	c.capabilityFetchedAt = time.Now().UTC()
	c.health = connector.HealthHealthy
	c.mu.Unlock()
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.mu.RLock()
	client := c.client
	cfg := c.cfg
	c.mu.RUnlock()

	if client == nil {
		return nil, cursor, fmt.Errorf("qf-decisions connector is not connected")
	}

	response, err := client.FetchDecisionEvents(ctx, cursor)
	if err != nil {
		health := connector.HealthError
		var schemaErr SchemaCompatibilityError
		if errors.As(err, &schemaErr) {
			health = connector.HealthDegraded
		}
		c.setHealth(health)
		return nil, cursor, fmt.Errorf("fetch QF decision events: %w", err)
	}

	normalizer := NewNormalizer(c.id, cfg.PacketVersion)
	now := time.Now().UTC()
	artifacts := make([]connector.RawArtifact, 0, len(response.Events))
	degraded := 0
	fastForwardObserved := false

	// Lag computation (SCN-SM-041-007): when the response carries a
	// server_time and at least one event, compute lag = server_time -
	// last_event.created_at and publish the gauge on every tick. The
	// connector NEVER auto-advances the cursor here — operators must
	// invoke QF's POST /api/private/smackerel/v1/cursor:fast-forward
	// (F13) to recover from a sustained breach.
	if response.ServerTime != "" && len(response.Events) > 0 {
		lastEvent := response.Events[len(response.Events)-1]
		if lastEvent.CreatedAt != "" {
			eventTime, parseErr := time.Parse(time.RFC3339, lastEvent.CreatedAt)
			serverTime, srvErr := time.Parse(time.RFC3339, response.ServerTime)
			if parseErr == nil && srvErr == nil {
				lagSeconds := serverTime.Sub(eventTime).Seconds()
				metrics.QFCursorLagSeconds.Set(lagSeconds)
				if int(lagSeconds) > cfg.CursorLagThresholdSeconds {
					slog.Warn("qf-decisions: lag_breach",
						slog.String("event", "lag_breach"),
						slog.Float64("cursor_lag_seconds", lagSeconds),
						slog.Int("threshold_seconds", cfg.CursorLagThresholdSeconds),
						slog.String("last_event_id", lastEvent.EventID),
						slog.String("connector_id", c.id),
					)
					// CRITICAL: never auto-advance the cursor here. The
					// operator must call POST /cursor:fast-forward against
					// QF and the connector will pick up the diagnostic
					// fast-forward event in a subsequent Sync cycle.
				}
			}
		}
	}

	for _, event := range response.Events {
		select {
		case <-ctx.Done():
			return artifacts, cursor, ctx.Err()
		default:
		}

		// Fast-forward recovery (SCN-SM-041-008): a QF-issued cursor
		// fast-forward diagnostic event carries events_skipped > 0 and
		// MUST NOT be normalized into a RawArtifact. The connector
		// records the skipped count, transitions to HealthDegradedRecovered,
		// and continues processing any subsequent normal events.
		if event.EventsSkipped > 0 {
			metrics.QFCursorFastForwardEventsSkipped.Add(float64(event.EventsSkipped))
			fastForwardObserved = true
			slog.Warn("qf-decisions: fast_forward_recovered",
				slog.String("event", "fast_forward_recovered"),
				slog.Int("events_skipped", event.EventsSkipped),
				slog.String("event_id", event.EventID),
				slog.String("connector_id", c.id),
			)
			continue
		}

		// Per-event cursor is diagnostic-only — never used for advancement.
		// The response-level next_cursor is the canonical advancement value.
		if event.Cursor != "" {
			slog.Debug("qf-decisions: per-event cursor recorded as diagnostic checkpoint",
				"event_id", event.EventID,
				"packet_id", event.PacketID,
				"event_cursor", event.Cursor,
			)
		}

		// Unknown decision_type detection (design.md §F8) is now owned by
		// the normalizer: when an event's decision_type is outside the
		// canonical set {recommendation, no_action, policy_denial,
		// analysis_note} the normalizer (a) falls through to the
		// canonical qf/decision-packet content type, (b) sets
		// metadata.unknown_decision_type=true on the resulting artifact
		// so downstream consumers can route it through the generic
		// packet card, and (c) increments
		// smackerel_qf_unknown_decision_type_total{value=<raw>}. The
		// capability-gate metric emission that lived here previously
		// was removed to avoid double-counting and to honor design.md
		// §F8 ("Emit ... for monitoring") unconditionally — the metric
		// must fire on every unknown_decision_type packet, not only
		// when the capability advertises a closed
		// SupportedDecisionTypes list.

		packetID := strings.TrimSpace(event.PacketID)
		if packetID == "" {
			degraded++
			slog.Warn("qf-decisions: event missing packet_id, skipping",
				"event_id", event.EventID,
				"trace_id", event.TraceID,
			)
			continue
		}

		envelope, err := client.FetchDecisionPacket(ctx, packetID)
		if err != nil {
			degraded++
			slog.Warn("qf-decisions: failed to fetch packet envelope",
				"event_id", event.EventID,
				"packet_id", packetID,
				"trace_id", event.TraceID,
				"error", err,
			)
			continue
		}

		captured := envelopeCapturedAt(envelope, event, now)
		artifact, diag := normalizer.Normalize(event, envelope, captured)
		if diag != nil {
			degraded++
			slog.Warn("qf-decisions: degraded packet, no trusted artifact published",
				"event_id", diag.EventID,
				"packet_id", diag.PacketID,
				"trace_id", diag.TraceID,
				"reason", diag.Reason,
				"missing_fields", strings.Join(diag.MissingFields, ","),
			)
			continue
		}
		artifacts = append(artifacts, *artifact)

		// Freshness SLA instrumentation (SCN-SM-041-003): observe ingest-stage
		// latency (QF emit → smackerel artifact materialized) for every
		// successfully normalized packet. design.md §F12 targets p95 ≤30s.
		// An empty or unparseable CreatedAt means we cannot compute a valid
		// latency, so the observation is skipped rather than emitted as a
		// misleading zero. Render and total stages are observed by downstream
		// render surfaces (Scope 5).
		if event.CreatedAt != "" {
			if emit, perr := time.Parse(time.RFC3339, event.CreatedAt); perr == nil {
				c.recordFreshness(FreshnessStageIngest, time.Since(emit).Seconds())
			}
		}
	}

	// Health precedence (highest first): degraded > degraded_recovered > healthy.
	// A fast-forward observation transitions to degraded_recovered ONLY when
	// the rest of the sync was clean — a real packet failure during the same
	// sync still surfaces as degraded so operators see the more serious
	// condition. SCN-SM-041-008.
	switch {
	case degraded > 0:
		c.setHealth(connector.HealthDegraded)
	case fastForwardObserved:
		c.setHealth(connector.HealthDegradedRecovered)
	default:
		c.setHealth(connector.HealthHealthy)
	}

	// Canonical advancement value is the response-level next_cursor.
	// Never use per-event cursor checkpoints for advancement.
	nextCursor := response.NextCursor
	if nextCursor == "" {
		nextCursor = cursor
	}
	return artifacts, nextCursor, nil
}

func (c *Connector) Health(context.Context) connector.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}

// CapabilitySnapshot returns the in-memory capability handshake state
// suitable for durable persistence into sync_state via
// StateStore.SaveCapability. The response is a canonical JSON encoding
// of the QFBridgeCapability the connector currently holds. When the
// capability has never been fetched (status == Unfetched) the response
// is the empty string and fetchedAt is the zero time; callers MUST still
// persist the status so a subsequent restart can distinguish "never
// fetched" from "fetched but row missing". Spec 041 Scope 2,
// SCN-SM-041-003.
func (c *Connector) CapabilitySnapshot() (responseJSON string, fetchedAt time.Time, status string, err error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	status = c.capabilityStatus
	fetchedAt = c.capabilityFetchedAt
	if c.capabilityFetchedAt.IsZero() && c.capabilityStatus == CapabilityStatusUnfetched {
		return "", time.Time{}, status, nil
	}
	encoded, mErr := json.Marshal(c.capability)
	if mErr != nil {
		return "", c.capabilityFetchedAt, status, fmt.Errorf("marshal capability: %w", mErr)
	}
	return string(encoded), c.capabilityFetchedAt, status, nil
}

func (c *Connector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.client = nil
	c.health = connector.HealthDisconnected
	c.capability = QFBridgeCapability{}
	c.capabilityStatus = CapabilityStatusUnfetched
	c.capabilityFetchedAt = time.Time{}
	return nil
}

func (c *Connector) setHealth(health connector.HealthStatus) {
	c.mu.Lock()
	c.health = health
	c.mu.Unlock()
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

	// cursor_lag_threshold_seconds is OPTIONAL — defaults to 3600 (1 hour)
	// when omitted. When provided it must be a positive integer; an invalid
	// value (zero, negative, non-numeric) is a hard configuration error so
	// operators discover typos at Connect time instead of seeing silent
	// "lag never breaches" behaviour at runtime. SCN-SM-041-007.
	cursorLagThreshold := defaultCursorLagThresholdSeconds
	if _, present := cfg.SourceConfig["cursor_lag_threshold_seconds"]; present {
		parsed, lagErr := sourcePositiveInt(cfg.SourceConfig, "cursor_lag_threshold_seconds")
		if lagErr != nil {
			configErrors = append(configErrors, lagErr.Error())
		} else {
			cursorLagThreshold = parsed
		}
	}

	if len(configErrors) > 0 {
		return QFConfig{}, fmt.Errorf("invalid qf-decisions connector configuration: %s", strings.Join(configErrors, ", "))
	}

	return QFConfig{
		BaseURL:                   strings.TrimRight(baseURL, "/"),
		CredentialRef:             credentialRef,
		SyncSchedule:              syncSchedule,
		PacketVersion:             packetVersion,
		PageSize:                  pageSize,
		CursorLagThresholdSeconds: cursorLagThreshold,
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
