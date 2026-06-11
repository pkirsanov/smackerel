// Package cardrewards implements the Spec 083 card-rewards source connector.
//
// It is a fetch-only connector (FR-CR-007 fetch half, FR-CR-008, Principle 4).
// For each operator-configured source page it emits ONE source-attributed
// connector.RawArtifact carrying the verbatim page text plus provenance
// metadata (source_name, source_url, issuer_hint). The connector deliberately
// performs NO category parsing and imports NO regexp — category extraction is
// the responsibility of the strict-schema LLM extractor in a later scope
// (Scope 05). Keeping parsing out of the connector is what lets every emitted
// artifact stay source-qualified (Principle 4).
//
// Fetches are isolated per source: each source gets its own deadline derived
// from fetch_timeout_seconds, so one slow/unreachable source is skipped
// (recorded as a failure) while every healthy source still emits. Connector
// health reflects the count of CONSECUTIVE fully-failed syncs via the shared
// connector.HealthFromErrorCount thresholds. The Sync cursor encodes the last
// successful fetch timestamp (RFC3339Nano).
package cardrewards

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// ConnectorID is the unique, stable identifier for the card-rewards connector
// (SCN-083-D01). It is hardcoded rather than caller-supplied so wiring can
// never register it under the wrong key.
const ConnectorID = "card-rewards"

// userAgent identifies Smackerel to upstream source pages.
const userAgent = "Smackerel/1.0 (card-rewards companion; github.com/smackerel/smackerel)"

// maxBodyBytes caps a single fetched source page to prevent resource
// exhaustion on a hostile or misconfigured source.
const maxBodyBytes = 8 << 20 // 8 MiB

// maxErrorBodyDrain bounds bytes drained from a non-200 response so the
// connection can be reused without consuming excessive memory.
const maxErrorBodyDrain = 1 << 16 // 64 KiB

// Source is one operator-configured rotating-category source page. It mirrors
// config.CardRewardsSource but is declared locally so the connector package
// does not import internal/config (avoids a dependency cycle and keeps the
// connector self-contained, matching the rss/weather connectors).
type Source struct {
	Name       string
	URL        string
	IssuerHint string
}

// Connector implements connector.Connector for card-rewards source pages.
type Connector struct {
	id         string
	mu         sync.RWMutex
	health     connector.HealthStatus
	httpClient *http.Client

	sources      []Source
	fetchTimeout time.Duration

	// consecutiveErrors counts fully-failed syncs in a row; it drives the
	// HealthFromErrorCount thresholds (SCN-083-D05) and is reset to zero
	// whenever a sync makes any progress.
	consecutiveErrors int

	// lastEmitted / lastFailed record the per-source outcome of the most
	// recent Sync so operators (and SCN-083-D04 tests) can confirm that a
	// slow source was recorded as failed while healthy sources still emitted.
	lastEmitted int
	lastFailed  int

	// allowPrivateHosts is a test-only seam (white-box). In production it is
	// false so the SSRF guard rejects loopback/private/link-local targets;
	// unit tests set it true to fetch from an httptest server on 127.0.0.1.
	allowPrivateHosts bool
}

// New creates a card-rewards connector. The ID is fixed to ConnectorID.
func New() *Connector {
	return &Connector{
		id:     ConnectorID,
		health: connector.HealthDisconnected,
		httpClient: &http.Client{
			// Refuse redirects to prevent SSRF via an open redirect on a
			// source page; source content is served directly.
			CheckRedirect: func(req *http.Request, _ []*http.Request) error {
				return fmt.Errorf("card-rewards connector refuses redirect to %s", req.URL.Hostname())
			},
		},
	}
}

// ID returns the connector identifier (always ConnectorID).
func (c *Connector) ID() string { return c.id }

// Connect parses and validates the source list and per-source fetch timeout
// from config.SourceConfig. It is fail-loud (Gate G028, smackerel-no-defaults):
// a missing/empty source list or a non-positive fetch timeout is an error —
// there is NO in-code default.
func (c *Connector) Connect(_ context.Context, config connector.ConnectorConfig) error {
	sources, err := parseSources(config.SourceConfig)
	if err != nil {
		return err
	}
	timeout, err := parseFetchTimeout(config.SourceConfig)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.sources = sources
	c.fetchTimeout = timeout
	c.health = connector.HealthHealthy
	c.consecutiveErrors = 0
	c.mu.Unlock()

	slog.Info("card-rewards connector connected",
		"id", c.id,
		"sources", len(sources),
		"fetch_timeout", timeout)
	return nil
}

// Sync fetches each configured source read-only and emits one
// source-attributed RawArtifact per source that responds successfully.
// A source that errors or exceeds its per-source deadline is skipped and
// recorded as failed (SCN-083-D04). The returned cursor encodes the last
// successful fetch timestamp on any success (SCN-083-D06); on a fully-failed
// sync the input cursor is returned unchanged and consecutiveErrors advances.
func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.mu.Lock()
	if c.health == connector.HealthDisconnected {
		c.mu.Unlock()
		return nil, cursor, fmt.Errorf("card-rewards: cannot sync, connector is not connected")
	}
	c.health = connector.HealthSyncing
	sources := append([]Source(nil), c.sources...)
	timeout := c.fetchTimeout
	client := c.httpClient
	allowPrivate := c.allowPrivateHosts
	c.mu.Unlock()

	var artifacts []connector.RawArtifact
	var failed int
	capturedAt := time.Now().UTC()

	for _, src := range sources {
		body, err := fetchSource(ctx, client, src, timeout, allowPrivate)
		if err != nil {
			failed++
			slog.Warn("card-rewards source fetch failed",
				"source", src.Name, "url", src.URL, "error", err)
			continue
		}
		artifacts = append(artifacts, connector.RawArtifact{
			SourceID:    ConnectorID,
			SourceRef:   fmt.Sprintf("%s#%s", src.URL, capturedAt.Format("2006-01-02")),
			ContentType: "card-rewards/source-page",
			Title:       fmt.Sprintf("Card rewards source: %s", src.Name),
			// RawContent is the VERBATIM page text. The connector applies no
			// parsing/regex (SCN-083-D03); category extraction happens later.
			RawContent: body,
			URL:        src.URL,
			Metadata: map[string]any{
				"source_name": src.Name,
				"source_url":  src.URL,
				"issuer_hint": src.IssuerHint,
			},
			CapturedAt: capturedAt,
		})
	}

	emitted := len(artifacts)
	newCursor := cursor

	c.mu.Lock()
	c.lastEmitted = emitted
	c.lastFailed = failed
	// If Close() ran concurrently the connector is disconnected; honor that
	// and do not overwrite the disconnected health.
	if c.health != connector.HealthDisconnected {
		switch {
		case emitted > 0:
			// Progress made: reset the consecutive-failure streak. Cursor
			// advances to this sync's fetch time (SCN-083-D06).
			c.consecutiveErrors = 0
			newCursor = capturedAt.Format(time.RFC3339Nano)
			if failed > 0 {
				c.health = connector.HealthDegraded
			} else {
				c.health = connector.HealthHealthy
			}
		default:
			// Fully-failed sync (every source failed, or none configured).
			// Advance the consecutive-error count and map it to a health
			// status via the shared thresholds (SCN-083-D05). Cursor is
			// left unchanged so the next run retries the same window.
			c.consecutiveErrors++
			c.health = connector.HealthFromErrorCount(c.consecutiveErrors)
		}
	}
	c.mu.Unlock()

	return artifacts, newCursor, nil
}

// Health returns the current connector health. After a fully-failed sync it
// reflects the consecutive-error count via connector.HealthFromErrorCount
// (SCN-083-D05).
func (c *Connector) Health(_ context.Context) connector.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}

// Close marks the connector disconnected and releases idle HTTP connections.
func (c *Connector) Close() error {
	c.mu.Lock()
	c.health = connector.HealthDisconnected
	client := c.httpClient
	c.mu.Unlock()
	if client != nil {
		client.CloseIdleConnections()
	}
	return nil
}

// LastSyncStats reports the per-source outcome of the most recent Sync:
// emitted is the number of source-attributed artifacts produced, failed is
// the number of sources that errored or timed out. Used by operators and by
// SCN-083-D04 to confirm slow-source isolation.
func (c *Connector) LastSyncStats() (emitted, failed int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastEmitted, c.lastFailed
}

// fetchSource performs a read-only GET of one source with its own deadline so
// a slow source cannot stall the others (SCN-083-D04). It returns the verbatim
// response body on HTTP 200.
func fetchSource(ctx context.Context, client *http.Client, src Source, timeout time.Duration, allowPrivate bool) (string, error) {
	if err := validateSourceURL(src.URL, allowPrivate); err != nil {
		return "", fmt.Errorf("source URL rejected: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, src.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBodyDrain))
		return "", fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return string(body), nil
}

// validateSourceURL enforces an SSRF guard: only http/https schemes, non-empty
// host, and (unless allowPrivate) the resolved host must not be a
// loopback/private/link-local/unspecified address. Source URLs are
// operator-supplied SST config, but the guard is defense-in-depth.
func validateSourceURL(raw string, allowPrivate bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", raw, err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("scheme %q not allowed (only http and https)", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("empty host in URL %q", raw)
	}
	if allowPrivate {
		return nil
	}

	ips, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("DNS resolution failed for %q: %w", host, err)
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("resolved IP %s for %q is private/reserved", ipStr, host)
		}
	}
	return nil
}

// parseSources extracts and validates the source list from SourceConfig. It
// accepts the typed []Source form (used by wiring) and the generic []any /
// []map[string]any forms (used when config round-trips through JSON/YAML).
func parseSources(sc map[string]any) ([]Source, error) {
	if sc == nil {
		return nil, fmt.Errorf("card-rewards: source_config is required")
	}
	raw, ok := sc["sources"]
	if !ok {
		return nil, fmt.Errorf("card-rewards: source_config[\"sources\"] is required")
	}

	var out []Source
	switch v := raw.(type) {
	case []Source:
		out = append(out, v...)
	case []any:
		for i, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("card-rewards: sources[%d] must be an object, got %T", i, item)
			}
			out = append(out, sourceFromMap(m))
		}
	case []map[string]any:
		for _, m := range v {
			out = append(out, sourceFromMap(m))
		}
	default:
		return nil, fmt.Errorf("card-rewards: source_config[\"sources\"] has unsupported type %T", raw)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("card-rewards: at least one source is required")
	}
	for i := range out {
		if strings.TrimSpace(out[i].Name) == "" || strings.TrimSpace(out[i].URL) == "" {
			return nil, fmt.Errorf("card-rewards: sources[%d] requires non-empty name and url", i)
		}
	}
	return out, nil
}

// sourceFromMap builds a Source from a generic config map.
func sourceFromMap(m map[string]any) Source {
	asString := func(key string) string {
		if s, ok := m[key].(string); ok {
			return s
		}
		return ""
	}
	return Source{
		Name:       asString("name"),
		URL:        asString("url"),
		IssuerHint: asString("issuer_hint"),
	}
}

// parseFetchTimeout extracts and validates fetch_timeout_seconds from
// SourceConfig. Fail-loud: a missing or non-positive value is an error
// (no default).
func parseFetchTimeout(sc map[string]any) (time.Duration, error) {
	if sc == nil {
		return 0, fmt.Errorf("card-rewards: source_config is required")
	}
	raw, ok := sc["fetch_timeout_seconds"]
	if !ok {
		return 0, fmt.Errorf("card-rewards: source_config[\"fetch_timeout_seconds\"] is required")
	}

	var secs int
	switch v := raw.(type) {
	case int:
		secs = v
	case int64:
		secs = int(v)
	case float64:
		secs = int(v)
	default:
		return 0, fmt.Errorf("card-rewards: fetch_timeout_seconds has unsupported type %T", raw)
	}
	if secs <= 0 {
		return 0, fmt.Errorf("card-rewards: fetch_timeout_seconds must be > 0, got %d", secs)
	}
	return time.Duration(secs) * time.Second, nil
}
