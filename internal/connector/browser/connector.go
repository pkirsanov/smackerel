package browser

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// Compile-time interface check.
var _ connector.Connector = (*Connector)(nil)

// BrowserConfig holds parsed browser-history-specific configuration.
type BrowserConfig struct {
	HistoryPath                    string
	AccessStrategy                 string // "copy" or "wal-read"
	InitialLookbackDays            int
	RepeatVisitWindow              time.Duration
	RepeatVisitThreshold           int
	ContentFetchTimeout            time.Duration
	ContentFetchConcurrency        int
	ContentFetchDomainDelay        time.Duration
	CustomSkipDomains              []string
	SocialMediaIndividualThreshold time.Duration
	DwellFullMin                   time.Duration
	DwellStandardMin               time.Duration
	DwellLightMin                  time.Duration
}

// syncStats tracks statistics for a single sync cycle.
type syncStats struct {
	skipped           int
	byTier            map[string]int
	fetchFails        int
	socialAggregates  int
	repeatEscalations int
}

// Connector implements the browser history connector.
type Connector struct {
	id     string
	health connector.HealthStatus
	mu     sync.RWMutex
	config BrowserConfig

	// Sync metadata for health reporting
	lastSyncTime       time.Time
	lastSyncCount      int
	lastSyncErrors     int
	lastSyncSkipped    int
	lastSyncByTier     map[string]int
	lastSyncFetchFails int

	// contentFetcher is called for full/standard tier entries to fetch page content.
	// If nil, no content fetching is performed.
	contentFetcher func(url string) (string, error)
}

// New creates a new browser history connector.
func New(id string) *Connector {
	return &Connector{
		id:     id,
		health: connector.HealthDisconnected,
	}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	cfg, err := parseBrowserConfig(config)
	if err != nil {
		c.mu.Lock()
		c.health = connector.HealthError
		c.mu.Unlock()
		return fmt.Errorf("parse browser config: %w", err)
	}

	// Validate History file exists and is readable
	if _, err := os.Stat(cfg.HistoryPath); os.IsNotExist(err) {
		c.mu.Lock()
		c.health = connector.HealthError
		c.mu.Unlock()
		return fmt.Errorf("chrome history file not found: %s", cfg.HistoryPath)
	}

	c.mu.Lock()
	c.config = cfg
	c.health = connector.HealthHealthy
	c.mu.Unlock()

	slog.Info("browser history connector connected",
		"history_path", cfg.HistoryPath,
		"access_strategy", cfg.AccessStrategy,
	)
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.mu.Lock()
	c.health = connector.HealthSyncing
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.lastSyncTime = time.Now()
		if c.lastSyncErrors > 0 {
			c.health = connector.HealthError
		} else {
			c.health = connector.HealthHealthy
		}
		c.mu.Unlock()
	}()

	// Step 1: Copy History file to temp location
	tmpPath, err := c.copyHistoryFile()
	if err != nil {
		// Retry once after 5 seconds (Chrome may be writing)
		time.Sleep(5 * time.Second)
		tmpPath, err = c.copyHistoryFile()
		if err != nil {
			c.mu.Lock()
			c.lastSyncErrors = 1
			c.mu.Unlock()
			return nil, cursor, fmt.Errorf("copy history file (after retry): %w", err)
		}
	}
	defer os.Remove(tmpPath)

	// Step 2: Parse entries since cursor
	var chromeTimeCursor int64
	if cursor != "" {
		chromeTimeCursor = parseCursorToChrome(cursor)
	} else {
		// Initial sync: lookback window
		lookback := time.Now().AddDate(0, 0, -c.config.InitialLookbackDays)
		chromeTimeCursor = GoTimeToChrome(lookback)
	}

	entries, err := ParseChromeHistorySince(tmpPath, chromeTimeCursor)
	if err != nil {
		c.mu.Lock()
		c.lastSyncErrors = 1
		c.mu.Unlock()
		return nil, cursor, fmt.Errorf("parse chrome history: %w", err)
	}

	// Step 3: Filter, classify, convert
	artifacts, newCursor, stats := c.processEntries(entries, chromeTimeCursor)

	// Record sync metadata
	c.mu.Lock()
	c.lastSyncCount = len(artifacts)
	c.lastSyncSkipped = stats.skipped
	c.lastSyncByTier = stats.byTier
	c.lastSyncFetchFails = stats.fetchFails
	c.lastSyncErrors = stats.fetchFails
	c.mu.Unlock()

	slog.Info("browser history sync complete",
		"total_entries", len(entries),
		"artifacts", len(artifacts),
		"skipped", stats.skipped,
		"by_tier", stats.byTier,
		"social_aggregates", stats.socialAggregates,
		"repeat_escalations", stats.repeatEscalations,
		"fetch_fails", stats.fetchFails,
	)

	return artifacts, newCursor, nil
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}

func (c *Connector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health = connector.HealthDisconnected
	slog.Info("browser history connector closed")
	return nil
}

// copyHistoryFile copies the Chrome History SQLite file to a temp location
// so we can read it without conflicts from Chrome's lock.
func (c *Connector) copyHistoryFile() (string, error) {
	src, err := os.Open(c.config.HistoryPath)
	if err != nil {
		return "", fmt.Errorf("open history file: %w", err)
	}
	defer src.Close()

	tmp, err := os.CreateTemp("", "smackerel-chrome-history-*.db")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", fmt.Errorf("copy history file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("close temp file: %w", err)
	}

	return tmp.Name(), nil
}

// dedupByURLDate merges entries with the same URL on the same UTC date (R-010).
// Dwell times are summed, the latest VisitTime is kept, and the Title from the
// longest individual visit is preserved. Stable insertion order is maintained.
func dedupByURLDate(entries []HistoryEntry) []HistoryEntry {
	type urlDate struct {
		url  string
		date string
	}
	type mergeState struct {
		entry    HistoryEntry
		maxDwell time.Duration
	}

	groups := make(map[urlDate]*mergeState)
	var order []urlDate

	for _, e := range entries {
		k := urlDate{url: e.URL, date: e.VisitTime.UTC().Format("2006-01-02")}
		if m, ok := groups[k]; ok {
			m.entry.DwellTime += e.DwellTime
			if e.VisitTime.After(m.entry.VisitTime) {
				m.entry.VisitTime = e.VisitTime
			}
			if e.DwellTime > m.maxDwell {
				m.maxDwell = e.DwellTime
				m.entry.Title = e.Title
			}
		} else {
			groups[k] = &mergeState{
				entry:    e,
				maxDwell: e.DwellTime,
			}
			order = append(order, k)
		}
	}

	result := make([]HistoryEntry, 0, len(order))
	for _, k := range order {
		result = append(result, groups[k].entry)
	}
	return result
}

// processEntries applies skip filtering, URL+date dedup (R-010), social media
// aggregation, repeat visit detection with tier escalation, dwell-time tiering,
// and the privacy gate. Social media entries are aggregated by domain+day;
// metadata-tier entries produce no individual artifacts (privacy gate).
func (c *Connector) processEntries(entries []HistoryEntry, prevCursor int64) ([]connector.RawArtifact, string, syncStats) {
	stats := syncStats{
		byTier: make(map[string]int),
	}

	var maxVisitTime int64 = prevCursor
	var qualifying []HistoryEntry

	// Step 1: Skip filtering and cursor tracking
	for _, e := range entries {
		if ShouldSkip(e.URL, c.config.CustomSkipDomains) {
			stats.skipped++
			continue
		}

		chromeTime := GoTimeToChrome(e.VisitTime)
		if chromeTime > maxVisitTime {
			maxVisitTime = chromeTime
		}

		qualifying = append(qualifying, e)
	}

	// Step 2: Detect repeat visits on raw entries (social media excluded)
	repeatVisits := c.detectRepeatVisits(qualifying)

	// Step 2.5: Dedup by URL+date — merge same-URL same-day entries (R-010)
	qualifying = dedupByURLDate(qualifying)

	// Step 3: Split into social media vs content tracks
	var socialEntries []HistoryEntry
	var contentEntries []HistoryEntry

	for _, e := range qualifying {
		if IsSocialMedia(e.Domain) {
			if c.config.SocialMediaIndividualThreshold > 0 && e.DwellTime >= c.config.SocialMediaIndividualThreshold {
				// High-dwell social media → content track for individual "full" processing
				contentEntries = append(contentEntries, e)
			} else if c.config.SocialMediaIndividualThreshold > 0 {
				socialEntries = append(socialEntries, e)
			} else {
				// Threshold not configured — treat as content
				contentEntries = append(contentEntries, e)
			}
		} else {
			contentEntries = append(contentEntries, e)
		}
	}

	// Step 4: Build social media aggregates (grouped by domain + date)
	type domainDay struct {
		domain string
		day    string
	}
	socialGroups := make(map[domainDay][]HistoryEntry)
	for _, e := range socialEntries {
		key := domainDay{domain: e.Domain, day: e.VisitTime.UTC().Format("2006-01-02")}
		socialGroups[key] = append(socialGroups[key], e)
	}

	var artifacts []connector.RawArtifact
	for key, group := range socialGroups {
		day, _ := time.Parse("2006-01-02", key.day)
		agg := c.buildSocialAggregate(key.domain, group, day)
		artifacts = append(artifacts, agg)
		stats.socialAggregates++
	}

	// Step 5: Process content entries with repeat escalation and privacy gate
	for _, e := range contentEntries {
		tier := c.dwellTimeTier(e.DwellTime)

		// High-dwell social media entries forced to "full" tier
		if IsSocialMedia(e.Domain) && c.config.SocialMediaIndividualThreshold > 0 && e.DwellTime >= c.config.SocialMediaIndividualThreshold {
			tier = "full"
		}

		// Apply repeat visit escalation
		if c.config.RepeatVisitThreshold > 0 {
			if count, ok := repeatVisits[e.URL]; ok && count >= c.config.RepeatVisitThreshold {
				tier = c.escalateTier(tier)
				stats.repeatEscalations++
			}
		}

		stats.byTier[tier]++

		// Privacy gate: metadata-tier entries produce no individual artifacts
		if tier == "metadata" {
			continue
		}

		artifact := connector.RawArtifact{
			SourceID:    "browser-history",
			SourceRef:   e.URL,
			ContentType: "url",
			Title:       e.Title,
			RawContent:  e.URL,
			URL:         e.URL,
			Metadata: map[string]interface{}{
				"domain":          e.Domain,
				"dwell_time":      e.DwellTime.Seconds(),
				"processing_tier": tier,
				"is_social_media": IsSocialMedia(e.Domain),
			},
			CapturedAt: e.VisitTime,
		}

		// Add repeat_visits count if applicable
		if c.config.RepeatVisitThreshold > 0 {
			if count, ok := repeatVisits[e.URL]; ok && count >= c.config.RepeatVisitThreshold {
				artifact.Metadata["repeat_visits"] = count
			}
		}

		// Content fetch for full/standard tiers
		if c.contentFetcher != nil && (tier == "full" || tier == "standard") {
			content, err := c.contentFetcher(e.URL)
			if err != nil {
				artifact.Metadata["content_fetch_failed"] = true
				artifact.RawContent = ""
				stats.fetchFails++
			} else {
				artifact.RawContent = content
			}
		}

		artifacts = append(artifacts, artifact)
	}

	newCursor := strconv.FormatInt(maxVisitTime, 10)

	return artifacts, newCursor, stats
}

// detectRepeatVisits builds a URL frequency map from entries.
// Social media URLs are excluded since they are handled by aggregation.
func (c *Connector) detectRepeatVisits(entries []HistoryEntry) map[string]int {
	freq := make(map[string]int)
	for _, e := range entries {
		if IsSocialMedia(e.Domain) {
			continue
		}
		freq[e.URL]++
	}
	return freq
}

// dwellTimeTier assigns processing tier using the connector's configured thresholds.
// Falls back to the hardcoded DwellTimeTier when thresholds are zero (unconfigured).
func (c *Connector) dwellTimeTier(dwellTime time.Duration) string {
	fullMin := c.config.DwellFullMin
	standardMin := c.config.DwellStandardMin
	lightMin := c.config.DwellLightMin

	// If thresholds are unconfigured (zero), fall back to the package-level defaults
	if fullMin == 0 && standardMin == 0 && lightMin == 0 {
		return DwellTimeTier(dwellTime)
	}

	switch {
	case fullMin > 0 && dwellTime >= fullMin:
		return "full"
	case standardMin > 0 && dwellTime >= standardMin:
		return "standard"
	case lightMin > 0 && dwellTime >= lightMin:
		return "light"
	default:
		return "metadata"
	}
}

// escalateTier bumps a processing tier up by one level.
func (c *Connector) escalateTier(tier string) string {
	switch tier {
	case "metadata":
		return "light"
	case "light":
		return "standard"
	case "standard":
		return "full"
	default:
		return "full"
	}
}

// buildSocialAggregate creates a single aggregate artifact for a social media
// domain on a given day.
func (c *Connector) buildSocialAggregate(domain string, entries []HistoryEntry, day time.Time) connector.RawArtifact {
	var totalDwell time.Duration
	var peakTitle string
	var peakDwell time.Duration
	for _, e := range entries {
		totalDwell += e.DwellTime
		if e.DwellTime > peakDwell {
			peakDwell = e.DwellTime
			peakTitle = e.Title
		}
	}

	content := fmt.Sprintf("%d visits to %s (total dwell: %s, peak: %s — %s)",
		len(entries), domain, totalDwell.Round(time.Second), peakTitle, peakDwell.Round(time.Second))

	return connector.RawArtifact{
		SourceID:    "browser-history",
		SourceRef:   fmt.Sprintf("social-aggregate:%s:%s", domain, day.Format("2006-01-02")),
		ContentType: "browsing/social-aggregate",
		Title:       fmt.Sprintf("%s activity on %s", domain, day.Format("2006-01-02")),
		RawContent:  content,
		Metadata: map[string]interface{}{
			"domain":                  domain,
			"date":                    day.Format("2006-01-02"),
			"visit_count":             len(entries),
			"total_dwell_seconds":     totalDwell.Seconds(),
			"peak_page_title":         peakTitle,
			"peak_page_dwell_seconds": peakDwell.Seconds(),
		},
		CapturedAt: day,
	}
}

// parseBrowserConfig extracts browser-history-specific config from ConnectorConfig.SourceConfig.
func parseBrowserConfig(config connector.ConnectorConfig) (BrowserConfig, error) {
	sc := config.SourceConfig

	cfg := BrowserConfig{
		AccessStrategy:                 "copy",
		InitialLookbackDays:            30,
		RepeatVisitWindow:              7 * 24 * time.Hour,
		RepeatVisitThreshold:           3,
		ContentFetchTimeout:            15 * time.Second,
		ContentFetchConcurrency:        5,
		ContentFetchDomainDelay:        1 * time.Second,
		SocialMediaIndividualThreshold: 5 * time.Minute,
		DwellFullMin:                   5 * time.Minute,
		DwellStandardMin:               2 * time.Minute,
		DwellLightMin:                  30 * time.Second,
	}

	// history_path is required
	if hp, ok := sc["history_path"].(string); ok && hp != "" {
		cfg.HistoryPath = hp
	} else {
		return BrowserConfig{}, fmt.Errorf("history_path is required and must be a non-empty string")
	}

	// access_strategy
	if as, ok := sc["access_strategy"].(string); ok && as != "" {
		if as != "copy" && as != "wal-read" {
			return BrowserConfig{}, fmt.Errorf("access_strategy must be \"copy\" or \"wal-read\", got %q", as)
		}
		cfg.AccessStrategy = as
	}

	// initial_lookback_days
	if ild, ok := sc["initial_lookback_days"]; ok {
		switch v := ild.(type) {
		case int:
			cfg.InitialLookbackDays = v
		case float64:
			cfg.InitialLookbackDays = int(v)
		}
	}

	// repeat_visit_window
	if rvw, ok := sc["repeat_visit_window"].(string); ok && rvw != "" {
		d, err := parseDurationWithDays(rvw)
		if err != nil {
			return BrowserConfig{}, fmt.Errorf("invalid repeat_visit_window: %w", err)
		}
		cfg.RepeatVisitWindow = d
	}

	// repeat_visit_threshold
	if rvt, ok := sc["repeat_visit_threshold"]; ok {
		switch v := rvt.(type) {
		case int:
			cfg.RepeatVisitThreshold = v
		case float64:
			cfg.RepeatVisitThreshold = int(v)
		}
	}

	// content_fetch_timeout
	if cft, ok := sc["content_fetch_timeout"].(string); ok && cft != "" {
		d, err := time.ParseDuration(cft)
		if err != nil {
			return BrowserConfig{}, fmt.Errorf("invalid content_fetch_timeout: %w", err)
		}
		cfg.ContentFetchTimeout = d
	}

	// content_fetch_concurrency
	if cfc, ok := sc["content_fetch_concurrency"]; ok {
		switch v := cfc.(type) {
		case int:
			cfg.ContentFetchConcurrency = v
		case float64:
			cfg.ContentFetchConcurrency = int(v)
		}
	}

	// content_fetch_domain_delay
	if cfdd, ok := sc["content_fetch_domain_delay"].(string); ok && cfdd != "" {
		d, err := time.ParseDuration(cfdd)
		if err != nil {
			return BrowserConfig{}, fmt.Errorf("invalid content_fetch_domain_delay: %w", err)
		}
		cfg.ContentFetchDomainDelay = d
	}

	// custom_skip_domains
	if csd, ok := sc["custom_skip_domains"].([]interface{}); ok {
		for _, d := range csd {
			if s, ok := d.(string); ok {
				cfg.CustomSkipDomains = append(cfg.CustomSkipDomains, s)
			}
		}
	}

	// social_media_individual_threshold
	if smit, ok := sc["social_media_individual_threshold"].(string); ok && smit != "" {
		d, err := time.ParseDuration(smit)
		if err != nil {
			return BrowserConfig{}, fmt.Errorf("invalid social_media_individual_threshold: %w", err)
		}
		cfg.SocialMediaIndividualThreshold = d
	}

	// dwell_time_thresholds
	if dtt, ok := sc["dwell_time_thresholds"].(map[string]interface{}); ok {
		if fm, ok := dtt["full_min"].(string); ok && fm != "" {
			d, err := time.ParseDuration(fm)
			if err != nil {
				return BrowserConfig{}, fmt.Errorf("invalid dwell_time_thresholds.full_min: %w", err)
			}
			cfg.DwellFullMin = d
		}
		if sm, ok := dtt["standard_min"].(string); ok && sm != "" {
			d, err := time.ParseDuration(sm)
			if err != nil {
				return BrowserConfig{}, fmt.Errorf("invalid dwell_time_thresholds.standard_min: %w", err)
			}
			cfg.DwellStandardMin = d
		}
		if lm, ok := dtt["light_min"].(string); ok && lm != "" {
			d, err := time.ParseDuration(lm)
			if err != nil {
				return BrowserConfig{}, fmt.Errorf("invalid dwell_time_thresholds.light_min: %w", err)
			}
			cfg.DwellLightMin = d
		}
	}

	return cfg, nil
}

// parseCursorToChrome converts a cursor string (Chrome visit_time integer) to int64.
func parseCursorToChrome(cursor string) int64 {
	v, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseDurationWithDays extends time.ParseDuration with support for "d" suffix (days).
func parseDurationWithDays(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		daysStr := strings.TrimSuffix(s, "d")
		days, err := strconv.Atoi(daysStr)
		if err != nil {
			return 0, fmt.Errorf("invalid days value: %w", err)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
