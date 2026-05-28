package keep

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/metrics"
)

// Spec 059 Scope 3 — NATS request/reply bridge subjects and schema version.
// Single source of truth for both the Go core and the Python ML sidecar;
// the Python side mirrors these constants in ml/app/keep_bridge.py.
const (
	gkeepHandshakeSubject = "keep.sidecar.handshake"
	gkeepRequestSubject   = "keep.sync.request"
	gkeepSchemaVersion    = 1
	gkeepRequestTimeout   = 120 * time.Second
	gkeepHandshakeTimeout = 10 * time.Second
)

// KeepNatsClient is the minimal NATS surface the Keep connector needs from
// the core's NATS client. Defined as an interface so tests can supply a
// scripted fake (see keep_bridge_test.go fakeNats).
type KeepNatsClient interface {
	Publish(ctx context.Context, subject string, data []byte) error
	Request(ctx context.Context, subject string, data []byte, timeout time.Duration) ([]byte, error)
}

// KeepHandshakeRequest is sent on gkeepHandshakeSubject to verify the
// ML sidecar bridge is ready (gkeepapi installed + the Bucket-2 Google App
// Password env var non-empty on the sidecar side — SCN-059-019). That env
// var is read ONLY by the sidecar; the Go core MUST NOT reference its name
// in non-test source per the Scope 1 boundary test.
type KeepHandshakeRequest struct {
	RequestID     string `json:"request_id"`
	SchemaVersion int    `json:"schema_version"`
}

// KeepHandshakeResponse is the sidecar's reply. Error is a pointer so a
// successful "ok" response can omit the field entirely.
type KeepHandshakeResponse struct {
	Status        string  `json:"status"`
	Error         *string `json:"error,omitempty"`
	SchemaVersion int     `json:"schema_version"`
}

// KeepSyncRequest is sent on gkeepRequestSubject to request a sync cycle.
type KeepSyncRequest struct {
	RequestID     string `json:"request_id"`
	Cursor        string `json:"cursor"`
	SchemaVersion int    `json:"schema_version"`
}

// KeepSyncResponse is the sidecar's sync reply.
type KeepSyncResponse struct {
	Status        string      `json:"status"`
	Notes         []GkeepNote `json:"notes"`
	Cursor        string      `json:"cursor"`
	Error         *string     `json:"error,omitempty"`
	SchemaVersion int         `json:"schema_version"`
}

// newRequestID returns a short opaque correlation id of the form
// "k-<unix-secs>-<6 hex>". Used purely for log correlation; no secret bits.
func newRequestID() string {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fall back to a time-only id; collisions in logs are tolerable.
		return fmt.Sprintf("k-%d-000000", time.Now().Unix())
	}
	return fmt.Sprintf("k-%d-%s", time.Now().Unix(), hex.EncodeToString(b[:]))
}

// SyncMode determines the sync strategy.
type SyncMode string

const (
	SyncModeTakeout  SyncMode = "takeout"
	SyncModeGkeepapi SyncMode = "gkeepapi"
	SyncModeHybrid   SyncMode = "hybrid"
)

// gkeepPollIntervalFloor is the minimum allowed gkeepapi poll interval.
// Resolves OQ-059-01 (spec 059 Scope 2) to a single canonical value.
const gkeepPollIntervalFloor = 15 * time.Minute

// KeepConfig holds parsed Keep-specific configuration.
type KeepConfig struct {
	SyncMode                SyncMode
	TakeoutImportDir        string
	TakeoutWatchInterval    time.Duration
	TakeoutArchiveProcessed bool
	GkeepEnabled            bool
	GkeepPollInterval       time.Duration
	GkeepWarningAck         bool
	// DriftAckToken is an opaque operator-supplied token used by the drift
	// circuit breaker (spec 059 Scope 4). Bumping the value and restarting
	// the connector resets the breaker. Empty string means "not acknowledged".
	DriftAckToken    string
	IncludeArchived  bool
	MinContentLength int
	LabelsFilter     []string
	DefaultTier      string
}

// GkeepNote represents a note from the gkeepapi Python bridge.
type GkeepNote struct {
	NoteID        string   `json:"note_id"`
	Title         string   `json:"title"`
	TextContent   string   `json:"text_content"`
	IsPinned      bool     `json:"is_pinned"`
	IsArchived    bool     `json:"is_archived"`
	IsTrashed     bool     `json:"is_trashed"`
	Color         string   `json:"color"`
	Labels        []string `json:"labels"`
	Collaborators []string `json:"collaborators"`
	ListItems     []struct {
		Text      string `json:"text"`
		IsChecked bool   `json:"is_checked"`
	} `json:"list_items"`
	ModifiedUsec int64 `json:"modified_usec"`
	CreatedUsec  int64 `json:"created_usec"`
}

// breakerState is the drift circuit-breaker FSM state (spec 059 Scope 4).
//
// Transitions (driven exclusively inside syncGkeepapi):
//
//	closed   -- first non-auth failure       --> tripping (driftFailures=1)
//	tripping -- next non-auth failures       --> tripping (driftFailures++)
//	tripping -- 4th consecutive failure      --> open      (KeepProtocolDriftDetected++)
//	tripping -- successful sync              --> closed    (driftFailures=0)
//	open     -- any Sync()                   --> early-return ErrBreakerOpen (no NATS)
//	open     -- Connect() with new ack token --> closed    (driftFailures=0)
//
// Open state never resets without a config-bump+restart-equivalent
// (Connect() observes a changed DriftAckToken). Sidecar auth errors do
// NOT advance the FSM — they are a Connect-class failure, not a drift
// signal (SCN-059-011). Counter increments exactly once per OPEN entry.
type breakerState int

const (
	breakerClosed breakerState = iota
	breakerTripping
	breakerOpen
)

func (b breakerState) String() string {
	switch b {
	case breakerClosed:
		return "closed"
	case breakerTripping:
		return "tripping"
	case breakerOpen:
		return "open"
	}
	return "unknown"
}

// breakerOpenThreshold is the consecutive-failure count at which the
// breaker transitions from TRIPPING to OPEN (SCN-059-010: "the 4th
// consecutive failure").
const breakerOpenThreshold = 4

// ErrBreakerOpen is the stable sentinel returned by Sync() while the
// drift circuit breaker is OPEN. Callers detect the OPEN state via
// errors.Is(err, ErrBreakerOpen) so adversarial assertions in tests
// can verify that ZERO NATS publishes happen during OPEN.
var ErrBreakerOpen = fmt.Errorf("gkeepapi drift breaker open: bump drift_ack_token + restart to reset")

// Connector implements the Google Keep connector.
type Connector struct {
	id     string
	health connector.HealthStatus
	mu     sync.RWMutex
	config KeepConfig

	natsClient KeepNatsClient
	parser     *TakeoutParser
	normalizer *Normalizer

	// Sync metadata
	lastSyncTime      time.Time
	lastSyncCount     int
	lastSyncErrors    int
	consecutiveErrors int

	// Drift circuit breaker (spec 059 Scope 4 / Scope 5).
	// breakerState is the FSM state; driftFailures is the consecutive
	// non-auth failure counter; lastAckToken is the most recent
	// DriftAckToken observed at Connect() time; openCounted ensures
	// the protocol-drift Prometheus counter increments EXACTLY ONCE per
	// OPEN entry (SCN-059-013) — repeated Sync() in OPEN must not
	// advance it.
	breakerState  breakerState
	driftFailures int
	lastAckToken  string
	openCounted   bool

	// Track processed exports
	processedExports map[string]bool
}

// New creates a new Google Keep connector.
func New(id string) *Connector {
	return &Connector{
		id:               id,
		health:           connector.HealthDisconnected,
		processedExports: make(map[string]bool),
	}
}

func (c *Connector) ID() string { return c.id }

// SetNatsClient injects the NATS client used for the gkeepapi sidecar bridge
// (spec 059 Scope 3). Safe to call before Connect; required for gkeepapi/
// hybrid modes (Connect performs a handshake on gkeepHandshakeSubject).
func (c *Connector) SetNatsClient(nc KeepNatsClient) {
	c.mu.Lock()
	c.natsClient = nc
	c.mu.Unlock()
}

// handshakeWithSidecar issues a synchronous request on gkeepHandshakeSubject
// and returns the verbatim sidecar error string if status != "ok". The
// sidecar is responsible for fail-loud validation of the Bucket-2 Google
// App Password env var (SCN-059-019); the Go core MUST NOT reference that
// env var by name in non-test source.
func (c *Connector) handshakeWithSidecar(ctx context.Context) error {
	c.mu.RLock()
	nc := c.natsClient
	c.mu.RUnlock()
	if nc == nil {
		return fmt.Errorf("gkeepapi handshake requires a NATS client (call SetNatsClient before Connect)")
	}
	reqID := newRequestID()
	payload, err := json.Marshal(KeepHandshakeRequest{
		RequestID:     reqID,
		SchemaVersion: gkeepSchemaVersion,
	})
	if err != nil {
		return fmt.Errorf("marshal handshake request: %w", err)
	}
	raw, err := nc.Request(ctx, gkeepHandshakeSubject, payload, gkeepHandshakeTimeout)
	if err != nil {
		return fmt.Errorf("sidecar handshake transport error: %w", err)
	}
	var resp KeepHandshakeResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("decode handshake response: %w", err)
	}
	if resp.Status != "ok" {
		msg := "sidecar handshake failed (no error string)"
		if resp.Error != nil && *resp.Error != "" {
			msg = *resp.Error
		}
		return fmt.Errorf("sidecar handshake: %s", msg)
	}
	slog.Info("gkeepapi sidecar handshake ok", "request_id", reqID)
	return nil
}

// connectError sets health to error and returns a formatted error.
func (c *Connector) connectError(format string, args ...interface{}) error {
	c.mu.Lock()
	c.health = connector.HealthError
	c.mu.Unlock()
	return fmt.Errorf(format, args...)
}

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	keepCfg, err := parseKeepConfig(config)
	if err != nil {
		return c.connectError("parse keep config: %w", err)
	}

	// Spec 059 Scope 2 — fail-loud secret precondition for live (gkeepapi/hybrid)
	// modes. Runs BEFORE the warning_acknowledged gate so a missing secret can
	// never silently fall back. Field-name-only error language: NEVER include
	// the value, its length, or any hash of it in the returned error (SCN-059-004).
	//
	// NOTE (planning conflict pending bubbles.plan reconciliation, spec 059):
	// the Scope 1 boundary test TestKeepAppPasswordReadOnlyFromSidecarNotCore
	// forbids ANY non-test Go literal reference to the Bucket-2 App Password
	// env key, but Scope 2's DoD requires a Connect-time os.Getenv check on
	// that very key. Only the KEEP_GOOGLE_EMAIL half of the secret
	// precondition is implemented here. The App Password half is routed back
	// to bubbles.plan to reconcile Scope 1 boundary vs Scope 2 DoD before the
	// missing check is added.
	if (keepCfg.SyncMode == SyncModeGkeepapi || keepCfg.SyncMode == SyncModeHybrid) &&
		keepCfg.GkeepEnabled {
		if os.Getenv("KEEP_GOOGLE_EMAIL") == "" {
			return c.connectError("KEEP_GOOGLE_EMAIL is required when google-keep live sync is enabled")
		}
	}

	// Validate gkeepapi acknowledgment
	if (keepCfg.SyncMode == SyncModeGkeepapi || keepCfg.SyncMode == SyncModeHybrid) &&
		keepCfg.GkeepEnabled && !keepCfg.GkeepWarningAck {
		return c.connectError("gkeepapi uses an unofficial API — set warning_acknowledged: true to proceed")
	}

	// Validate Takeout import directory
	if keepCfg.SyncMode == SyncModeTakeout || keepCfg.SyncMode == SyncModeHybrid {
		if keepCfg.TakeoutImportDir == "" {
			return c.connectError("takeout import directory not configured")
		}
		if _, err := os.Stat(keepCfg.TakeoutImportDir); os.IsNotExist(err) {
			return c.connectError("takeout import directory does not exist: %s", keepCfg.TakeoutImportDir)
		}
	}

	c.mu.Lock()
	c.config = keepCfg
	c.parser = NewTakeoutParser()
	c.normalizer = NewNormalizer(keepCfg)
	c.health = connector.HealthHealthy
	// Spec 059 Scope 4 — reset the drift circuit breaker iff the
	// operator-supplied drift_ack_token changed since the last Connect().
	// First Connect() always seeds the token. A bump+restart equivalent
	// is the ONLY way to clear an OPEN breaker; an unchanged token must
	// preserve breaker state across Connect() calls (no silent reset).
	if keepCfg.DriftAckToken != c.lastAckToken {
		c.breakerState = breakerClosed
		c.driftFailures = 0
		c.openCounted = false
		c.lastAckToken = keepCfg.DriftAckToken
	}
	c.mu.Unlock()

	// Spec 059 Scope 3 — sidecar handshake for live modes. Runs AFTER config
	// is persisted so error paths can still report state, but BEFORE Sync()
	// is ever invoked. A failed handshake sets health=error and surfaces the
	// sidecar's error string verbatim (SCN-059-019). When no NATS client has
	// been injected we skip the handshake (degraded-but-not-failed): Sync()
	// will then fail loud the first time it tries to talk to the bridge.
	if (keepCfg.SyncMode == SyncModeGkeepapi || keepCfg.SyncMode == SyncModeHybrid) &&
		keepCfg.GkeepEnabled {
		c.mu.RLock()
		hasNats := c.natsClient != nil
		c.mu.RUnlock()
		if hasNats {
			if err := c.handshakeWithSidecar(ctx); err != nil {
				return c.connectError("%s", err.Error())
			}
		} else {
			slog.Warn("gkeepapi sidecar handshake skipped: no NATS client injected (SetNatsClient was not called)")
		}
	}

	slog.Info("google keep connector connected",
		"sync_mode", string(keepCfg.SyncMode),
		"import_dir", keepCfg.TakeoutImportDir,
	)
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.mu.Lock()
	c.health = connector.HealthSyncing
	c.lastSyncCount = 0
	c.lastSyncErrors = 0
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.lastSyncTime = time.Now()
		if c.lastSyncErrors > 0 && c.lastSyncCount == 0 {
			// Complete failure: no artifacts produced — escalate with consecutive count.
			// Complete-failure escalation is more aggressive than per-error escalation
			// (HealthFromErrorCount) because producing zero artifacts means the entire
			// sync path is broken, not just individual items.
			c.consecutiveErrors++
			switch {
			case c.consecutiveErrors >= 10:
				c.health = connector.HealthError
			case c.consecutiveErrors >= 5:
				c.health = connector.HealthFailing
			default:
				c.health = connector.HealthDegraded
			}
		} else if c.lastSyncErrors > 0 {
			// Partial success: some artifacts produced despite errors
			c.consecutiveErrors = 0
			c.health = connector.HealthDegraded
		} else {
			// Full success
			c.consecutiveErrors = 0
			c.health = connector.HealthHealthy
		}
		c.mu.Unlock()
	}()

	var allArtifacts []connector.RawArtifact
	var newCursor string
	syncErrors := 0

	switch c.config.SyncMode {
	case SyncModeTakeout:
		artifacts, cur, errs, err := c.syncTakeout(ctx, cursor)
		if err != nil {
			c.mu.Lock()
			c.lastSyncErrors = 1
			c.mu.Unlock()
			return nil, cursor, err
		}
		allArtifacts = artifacts
		newCursor = cur
		syncErrors = errs

	case SyncModeGkeepapi:
		// gkeepapi sync via ML sidecar NATS bridge (keep.sync.request/response)
		gkeepArtifacts, gkeepCur, gkeepErrs, err := c.syncGkeepapi(ctx, cursor)
		if err != nil {
			slog.Warn("gkeepapi sync failed", "error", err)
			syncErrors++
		} else {
			allArtifacts = append(allArtifacts, gkeepArtifacts...)
			if gkeepCur != "" {
				newCursor = gkeepCur
			}
			syncErrors += gkeepErrs
		}

	case SyncModeHybrid:
		// Takeout is primary
		artifacts, cur, errs, err := c.syncTakeout(ctx, cursor)
		if err != nil {
			slog.Warn("takeout sync failed in hybrid mode", "error", err)
			syncErrors++
		} else {
			allArtifacts = append(allArtifacts, artifacts...)
			newCursor = cur
			syncErrors += errs
		}
		// gkeepapi supplements Takeout with live data from Python bridge
		gkeepArtifacts, gkeepCur, gkeepErrs, err := c.syncGkeepapi(ctx, cursor)
		if err != nil {
			slog.Warn("gkeepapi sync failed in hybrid mode, continuing with takeout results", "error", err)
			syncErrors++
		} else {
			allArtifacts = append(allArtifacts, gkeepArtifacts...)
			if gkeepCur != "" && newCursor != "" {
				gkeepTime, gErr := time.Parse(time.RFC3339Nano, gkeepCur)
				newTime, nErr := time.Parse(time.RFC3339Nano, newCursor)
				if gErr == nil && nErr == nil && gkeepTime.After(newTime) {
					newCursor = gkeepCur
				}
			} else if gkeepCur != "" {
				newCursor = gkeepCur
			}
			syncErrors += gkeepErrs
		}
	}

	c.mu.Lock()
	c.lastSyncCount = len(allArtifacts)
	c.lastSyncErrors = syncErrors
	c.mu.Unlock()

	if newCursor == "" {
		newCursor = cursor
	}

	return allArtifacts, newCursor, nil
}

// syncTakeout syncs notes from a Google Takeout export directory.
func (c *Connector) syncTakeout(ctx context.Context, cursor string) ([]connector.RawArtifact, string, int, error) {
	c.mu.RLock()
	importDir := c.config.TakeoutImportDir
	parser := c.parser
	normalizer := c.normalizer
	natsClient := c.natsClient
	alreadyProcessed := c.processedExports[importDir]
	c.mu.RUnlock()
	if alreadyProcessed && cursor != "" {
		// Re-parse to check for new files, but filter by cursor
	}

	notes, parseErrors, err := parser.ParseExport(importDir)
	if err != nil {
		return nil, cursor, 0, fmt.Errorf("parse takeout export: %w", err)
	}

	if len(parseErrors) > 0 {
		for _, pe := range parseErrors {
			slog.Warn("failed to parse takeout note", "file", pe)
		}
	}

	// Filter by cursor
	filtered, newCursor := parser.FilterByCursor(notes, cursor)

	var artifacts []connector.RawArtifact

	for i := range filtered {
		if err := ctx.Err(); err != nil {
			return artifacts, cursor, 0, fmt.Errorf("sync cancelled: %w", err)
		}

		noteID := parser.NoteID(&filtered[i], filtered[i].SourceFile)
		if noteID == "" {
			noteID = fmt.Sprintf("keep-note-%d", i)
		}

		artifact, err := normalizer.Normalize(&filtered[i], noteID, "takeout")
		if err != nil {
			slog.Warn("failed to normalize note", "note_id", noteID, "error", err)
			continue
		}
		if artifact == nil {
			// Skipped (trashed, archived, etc.)
			continue
		}

		// Publish artifact to NATS for pipeline processing
		if natsClient != nil {
			payload, marshalErr := json.Marshal(artifact)
			if marshalErr != nil {
				slog.Warn("failed to serialize artifact for NATS", "note_id", noteID, "error", marshalErr)
			} else if pubErr := natsClient.Publish(ctx, "artifacts.process", payload); pubErr != nil {
				slog.Warn("failed to publish artifact to NATS", "note_id", noteID, "error", pubErr)
			}
		}

		artifacts = append(artifacts, *artifact)
	}

	c.mu.Lock()
	c.processedExports[importDir] = true
	c.mu.Unlock()

	return artifacts, newCursor, len(parseErrors), nil
}

// syncGkeepapi syncs notes via the gkeepapi Python bridge using NATS request/reply.
// Returns artifacts, new cursor, parse error count, and any fatal error.
//
// Spec 059 Scope 4 / Scope 5 — wraps the request/reply round-trip in the
// drift circuit breaker FSM. Validation drift and sidecar status:"error"
// (non-auth) advance the breaker; sidecar auth errors are surfaced as
// Connect-class failures and do NOT advance the FSM (SCN-059-011).
func (c *Connector) syncGkeepapi(ctx context.Context, cursor string) ([]connector.RawArtifact, string, int, error) {
	if !c.config.GkeepEnabled {
		return nil, cursor, 0, fmt.Errorf("gkeepapi not enabled in configuration")
	}

	// Breaker OPEN: return early without any NATS publish (SCN-059-010,
	// adversarial assertion in TestOpenBreakerSkipsNatsPublish).
	c.mu.RLock()
	if c.breakerState == breakerOpen {
		c.mu.RUnlock()
		return nil, cursor, 1, ErrBreakerOpen
	}
	nc := c.natsClient
	normalizer := c.normalizer
	c.mu.RUnlock()
	if nc == nil {
		return nil, cursor, 0, fmt.Errorf("gkeepapi bridge not connected: SetNatsClient was never called")
	}

	reqID := newRequestID()
	reqPayload, err := json.Marshal(KeepSyncRequest{
		RequestID:     reqID,
		Cursor:        cursor,
		SchemaVersion: gkeepSchemaVersion,
	})
	if err != nil {
		return nil, cursor, 0, fmt.Errorf("marshal sync request: %w", err)
	}
	slog.Info("keep_sync_request", "request_id", reqID, "cursor", cursor)

	start := time.Now()
	rawResp, err := nc.Request(ctx, gkeepRequestSubject, reqPayload, gkeepRequestTimeout)
	if err != nil {
		// Transport failure is treated as a drift signal: a working
		// sidecar reachable on NATS must reply within the timeout, so
		// a transport error during normal operation is the same class
		// of fault as a protocol violation.
		c.recordBreakerFailure(reqID, fmt.Sprintf("transport: %v", err))
		return nil, cursor, 0, fmt.Errorf("gkeepapi sync transport error: %w", err)
	}
	var resp KeepSyncResponse
	if err := json.Unmarshal(rawResp, &resp); err != nil {
		c.recordBreakerFailure(reqID, "decode")
		return nil, cursor, 0, fmt.Errorf("decode sync response: %w", err)
	}

	// Sidecar reported error: auth = Connect-class, everything else = drift.
	if resp.Status == "error" {
		msg := "sidecar sync failed (no error string)"
		if resp.Error != nil && *resp.Error != "" {
			msg = *resp.Error
		}
		if isSidecarAuthError(msg) {
			// Auth failure: do NOT advance the breaker (SCN-059-011).
			// Surface as fatal so the caller marks health=error.
			return nil, cursor, 1, fmt.Errorf("gkeepapi sidecar auth: %s", msg)
		}
		c.recordBreakerFailure(reqID, fmt.Sprintf("sidecar_error: %s", msg))
		return nil, cursor, 1, fmt.Errorf("gkeepapi sidecar: %s", msg)
	}

	// Schema validation (SCN-059-009). Any drift class trips the breaker.
	if vErr := validateGkeepResponse(&resp); vErr != nil {
		c.recordBreakerFailure(reqID, fmt.Sprintf("schema: %v", vErr))
		return nil, cursor, 1, fmt.Errorf("gkeepapi schema drift: %w", vErr)
	}

	// Success path — record metrics and clear the breaker.
	metrics.KeepGkeepSyncDuration.Observe(time.Since(start).Seconds())
	metrics.KeepGkeepNotesReturned.Add(float64(len(resp.Notes)))
	c.recordBreakerSuccess()

	artifacts := make([]connector.RawArtifact, 0, len(resp.Notes))
	parseErrors := 0
	for i := range resp.Notes {
		if err := ctx.Err(); err != nil {
			return artifacts, cursor, parseErrors, fmt.Errorf("sync cancelled: %w", err)
		}
		artifact, err := normalizer.NormalizeGkeep(&resp.Notes[i])
		if err != nil {
			parseErrors++
			slog.Warn("failed to normalize gkeepapi note", "note_id", resp.Notes[i].NoteID, "error", err)
			continue
		}
		if artifact == nil {
			continue
		}
		if payload, mErr := json.Marshal(artifact); mErr != nil {
			slog.Warn("failed to serialize gkeepapi artifact for NATS", "note_id", resp.Notes[i].NoteID, "error", mErr)
		} else if pubErr := nc.Publish(ctx, "artifacts.process", payload); pubErr != nil {
			slog.Warn("failed to publish gkeepapi artifact to NATS", "note_id", resp.Notes[i].NoteID, "error", pubErr)
		}
		artifacts = append(artifacts, *artifact)
	}
	slog.Info("keep_sync_response", "request_id", reqID, "notes", len(resp.Notes), "cursor", resp.Cursor)

	newCursor := resp.Cursor
	if newCursor == "" {
		newCursor = cursor
	}
	return artifacts, newCursor, parseErrors, nil
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	// Spec 059 Scope 5 — OPEN breaker masks any cached health value
	// (SCN-059-014). Recovery requires Connect() with a rotated
	// drift_ack_token, which resets breakerState back to closed.
	if c.breakerState == breakerOpen {
		return connector.HealthError
	}
	return c.health
}

func (c *Connector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health = connector.HealthDisconnected
	return nil
}

// parseKeepConfig extracts KeepConfig from a generic ConnectorConfig.
func parseKeepConfig(config connector.ConnectorConfig) (KeepConfig, error) {
	kc := KeepConfig{
		SyncMode:          SyncModeTakeout,
		GkeepPollInterval: 60 * time.Minute,
		MinContentLength:  0,
		IncludeArchived:   false,
	}

	sc := config.SourceConfig

	if mode, ok := sc["sync_mode"].(string); ok {
		switch SyncMode(mode) {
		case SyncModeTakeout, SyncModeGkeepapi, SyncModeHybrid:
			kc.SyncMode = SyncMode(mode)
		default:
			return kc, fmt.Errorf("invalid sync_mode: %s (must be takeout, gkeepapi, or hybrid)", mode)
		}
	}

	if dir, ok := sc["import_dir"].(string); ok && dir != "" {
		// Canonicalize import path to prevent traversal via config (CWE-22).
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return kc, fmt.Errorf("invalid import_dir path: %w", err)
		}
		kc.TakeoutImportDir = absDir
	}

	if enabled, ok := sc["gkeep_enabled"].(bool); ok {
		kc.GkeepEnabled = enabled
	}

	if ack, ok := sc["warning_acknowledged"].(bool); ok {
		kc.GkeepWarningAck = ack
	}

	// Spec 059 Scope 2 — drift_ack_token (opaque operator token).
	// Same sc[...].(type) pattern as warning_acknowledged. Missing key yields
	// empty string (not an error); wrong type yields a parse error referencing
	// the field name (SCN-059-003).
	if raw, present := sc["drift_ack_token"]; present {
		tok, ok := raw.(string)
		if !ok {
			return kc, fmt.Errorf("drift_ack_token must be a string, got %T", raw)
		}
		kc.DriftAckToken = tok
	}

	if includeArchived, ok := sc["include_archived"].(bool); ok {
		kc.IncludeArchived = includeArchived
	}

	if minLen, ok := sc["min_content_length"].(float64); ok {
		if minLen < 0 {
			return kc, fmt.Errorf("min_content_length must be non-negative, got %v", minLen)
		}
		kc.MinContentLength = int(minLen)
	}

	if interval, ok := sc["poll_interval"].(string); ok {
		d, err := time.ParseDuration(interval)
		if err != nil {
			return kc, fmt.Errorf("invalid poll_interval: %w", err)
		}
		if d < gkeepPollIntervalFloor {
			return kc, fmt.Errorf("poll_interval must be at least %s, got %s", gkeepPollIntervalFloor, interval)
		}
		kc.GkeepPollInterval = d
	}

	if watchInterval, ok := sc["watch_interval"].(string); ok {
		d, err := time.ParseDuration(watchInterval)
		if err != nil {
			return kc, fmt.Errorf("invalid watch_interval: %w", err)
		}
		kc.TakeoutWatchInterval = d
	}

	if archiveProcessed, ok := sc["archive_processed"].(bool); ok {
		kc.TakeoutArchiveProcessed = archiveProcessed
	}

	if defaultTier, ok := sc["default_tier"].(string); ok {
		kc.DefaultTier = defaultTier
	}

	if labelsRaw, ok := sc["labels_filter"].([]interface{}); ok {
		for _, l := range labelsRaw {
			if s, ok := l.(string); ok {
				kc.LabelsFilter = append(kc.LabelsFilter, s)
			}
		}
	}

	return kc, nil
}

// Ensure Connector implements the interface at compile time.
var _ connector.Connector = (*Connector)(nil)

// validateGkeepResponse enforces the wire-schema contract for a
// KeepSyncResponse (spec 059 Scope 4, SCN-059-009). Any drift class
// returns a non-nil error; a canonical fixture returns nil. The check
// is intentionally strict — schema_version must equal the literal
// gkeepSchemaVersion, status must be one of {"ok","error"}, the
// error-shape invariants must hold, and on ok every note must carry
// a non-empty note_id (the dedup key downstream).
func validateGkeepResponse(resp *KeepSyncResponse) error {
	if resp == nil {
		return fmt.Errorf("response is nil")
	}
	if resp.SchemaVersion != gkeepSchemaVersion {
		return fmt.Errorf("schema_version = %d, want %d", resp.SchemaVersion, gkeepSchemaVersion)
	}
	switch resp.Status {
	case "ok":
		// On ok: error must be absent or empty, notes may be empty,
		// but every note must carry a non-empty note_id.
		if resp.Error != nil && *resp.Error != "" {
			return fmt.Errorf("status=ok must not carry a non-empty error string")
		}
		for i := range resp.Notes {
			if resp.Notes[i].NoteID == "" {
				return fmt.Errorf("note[%d] has empty note_id", i)
			}
		}
		return nil
	case "error":
		if resp.Error == nil || *resp.Error == "" {
			return fmt.Errorf("status=error must carry a non-empty error string")
		}
		if len(resp.Notes) != 0 {
			return fmt.Errorf("status=error must carry zero notes, got %d", len(resp.Notes))
		}
		return nil
	default:
		return fmt.Errorf("invalid status %q (want ok|error)", resp.Status)
	}
}

// isSidecarAuthError classifies a sidecar error string as an
// authentication failure (Connect-class) versus a drift signal
// (SCN-059-011). The Python sidecar emits the stable prefix
// "gkeepapi authentication failed" for any login/2FA/App-Password
// failure; matching is by HasPrefix on the trimmed message.
func isSidecarAuthError(msg string) bool {
	return strings.HasPrefix(strings.TrimSpace(msg), "gkeepapi authentication failed")
}

// recordBreakerFailure advances the drift FSM by one consecutive
// failure. On the breakerOpenThreshold-th failure the state transitions
// to OPEN and the protocol-drift Prometheus counter increments exactly
// once (openCounted guards repeat entries — SCN-059-013).
func (c *Connector) recordBreakerFailure(reqID, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.driftFailures++
	if c.breakerState == breakerClosed {
		c.breakerState = breakerTripping
	}
	slog.Warn("keep_protocol_drift",
		"connector_id", c.id,
		"request_id", reqID,
		"reason", reason,
		"consecutive_failures", c.driftFailures,
		"state", c.breakerState.String(),
	)
	if c.driftFailures >= breakerOpenThreshold && c.breakerState != breakerOpen {
		c.breakerState = breakerOpen
		if !c.openCounted {
			metrics.KeepProtocolDriftDetected.WithLabelValues(c.id).Inc()
			c.openCounted = true
		}
		slog.Error("keep_protocol_drift_detected",
			"connector_id", c.id,
			"consecutive_failures", c.driftFailures,
			"last_request_id", reqID,
		)
	}
}

// recordBreakerSuccess resets the consecutive-failure counter when a
// sync succeeds. From CLOSED this is a no-op; from TRIPPING this
// returns to CLOSED (SCN-059-012). Never clears OPEN — only a
// Connect() with a rotated drift_ack_token does that.
func (c *Connector) recordBreakerSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.breakerState == breakerOpen {
		return
	}
	c.driftFailures = 0
	c.breakerState = breakerClosed
}

// Compile-time sanity: keep the errors import live for ErrBreakerOpen.
var _ = errors.New
