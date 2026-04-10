package browser

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// HistoryEntry represents a parsed browser history entry.
type HistoryEntry struct {
	URL       string        `json:"url"`
	Title     string        `json:"title"`
	VisitTime time.Time     `json:"visit_time"`
	DwellTime time.Duration `json:"dwell_time"`
	Domain    string        `json:"domain"`
}

// ParseChromeHistory reads Chrome's SQLite history database.
func ParseChromeHistory(dbPath string) ([]HistoryEntry, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open Chrome history: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT u.url, u.title, v.visit_time, v.visit_duration
		FROM urls u
		JOIN visits v ON v.url = u.id
		WHERE v.visit_time > 0
		ORDER BY v.visit_time DESC
		LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		var visitTime int64
		var duration int64
		if err := rows.Scan(&e.URL, &e.Title, &visitTime, &duration); err != nil {
			continue
		}
		// Chrome stores time as microseconds since 1601-01-01
		e.VisitTime = chromeTimeToGo(visitTime)
		e.DwellTime = time.Duration(duration) * time.Microsecond
		e.Domain = extractDomain(e.URL)
		entries = append(entries, e)
	}

	return entries, nil
}

// DwellTimeTier assigns processing tier based on dwell time.
func DwellTimeTier(dwellTime time.Duration) string {
	switch {
	case dwellTime >= 5*time.Minute:
		return "full"
	case dwellTime >= 2*time.Minute:
		return "standard"
	case dwellTime >= 30*time.Second:
		return "light"
	default:
		return "metadata"
	}
}

// SocialMediaDomains are domains that get aggregated rather than individually processed.
var SocialMediaDomains = map[string]bool{
	"twitter.com":   true,
	"x.com":         true,
	"facebook.com":  true,
	"instagram.com": true,
	"reddit.com":    true,
	"linkedin.com":  true,
	"tiktok.com":    true,
}

// IsSocialMedia checks if a domain is a social media site.
func IsSocialMedia(domain string) bool {
	return SocialMediaDomains[domain]
}

// DefaultSkipDomains are domains that should never be processed.
var DefaultSkipDomains = []string{
	"localhost",
	"127.0.0.1",
	"chrome://",
	"chrome-extension://",
	"about:",
	"file://",
}

// ShouldSkip checks if a URL should be skipped for processing.
// User-provided skip domains are matched against the extracted domain of the URL
// (so "private.corp.com" matches "https://private.corp.com/page").
// Default skip domains use both prefix matching (for protocol-prefix entries like "chrome://")
// and domain matching (for hostname entries like "localhost" and "127.0.0.1").
func ShouldSkip(url string, skipDomains []string) bool {
	domain := extractDomain(url)

	// Match user skip domains against the extracted domain
	for _, skip := range skipDomains {
		if domain == skip {
			return true
		}
	}
	// Default skip domains: prefix matching for protocol-prefix entries,
	// plus domain matching to catch scheme-prefixed variants (e.g. https://localhost:3000).
	for _, skip := range DefaultSkipDomains {
		if len(url) >= len(skip) && url[:len(skip)] == skip {
			return true
		}
		if domain == skip {
			return true
		}
	}
	return false
}

// ToRawArtifacts converts history entries to raw artifacts.
func ToRawArtifacts(entries []HistoryEntry) []connector.RawArtifact {
	var artifacts []connector.RawArtifact
	for _, e := range entries {
		artifacts = append(artifacts, connector.RawArtifact{
			SourceID:    "browser",
			SourceRef:   e.URL,
			ContentType: "url",
			Title:       e.Title,
			RawContent:  e.URL,
			URL:         e.URL,
			Metadata: map[string]interface{}{
				"domain":     e.Domain,
				"dwell_time": e.DwellTime.Seconds(),
			},
			CapturedAt: e.VisitTime,
		})
	}
	return artifacts
}

// ParseChromeHistorySince reads Chrome history entries with visit_time > cursor.
// Unlike ParseChromeHistory, this orders ASC for cursor-based incremental sync
// and limits results to 10000 entries per batch to prevent memory exhaustion.
func ParseChromeHistorySince(dbPath string, chromeTimeCursor int64) ([]HistoryEntry, error) {
	db, err := sql.Open("sqlite3", dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open Chrome history: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT u.url, u.title, v.visit_time, v.visit_duration
		FROM urls u
		JOIN visits v ON v.url = u.id
		WHERE v.visit_time > ?
		ORDER BY v.visit_time ASC
		LIMIT 10000
	`, chromeTimeCursor)
	if err != nil {
		return nil, fmt.Errorf("query history since cursor: %w", err)
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		var visitTime int64
		var duration int64
		if err := rows.Scan(&e.URL, &e.Title, &visitTime, &duration); err != nil {
			continue
		}
		e.VisitTime = chromeTimeToGo(visitTime)
		e.DwellTime = time.Duration(duration) * time.Microsecond
		e.Domain = extractDomain(e.URL)
		entries = append(entries, e)
	}

	return entries, nil
}

// GoTimeToChrome converts a Go time.Time to Chrome's microseconds-since-1601 format.
func GoTimeToChrome(t time.Time) int64 {
	const chromeEpochDiff = 11644473600000000
	return t.UnixMicro() + chromeEpochDiff
}

// ChromeTimeToGo converts Chrome's microseconds-since-1601 to time.Time.
// Exports the existing chromeTimeToGo for use by the connector.
func ChromeTimeToGo(chromeTime int64) time.Time {
	return chromeTimeToGo(chromeTime)
}

func chromeTimeToGo(chromeTime int64) time.Time {
	// Chrome epoch: 1601-01-01. Difference from Unix epoch in microseconds.
	const chromeEpochDiff = 11644473600000000
	unixMicro := chromeTime - chromeEpochDiff
	return time.UnixMicro(unixMicro)
}

func extractDomain(url string) string {
	// Simple domain extraction with safe bounds checks
	start := 0
	if len(url) >= 8 && url[:8] == "https://" {
		start = 8
	} else if len(url) >= 7 && url[:7] == "http://" {
		start = 7
	}
	if start >= len(url) {
		return ""
	}
	end := start
	for end < len(url) && url[end] != '/' && url[end] != ':' {
		end++
	}
	return url[start:end]
}
