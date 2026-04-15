package browser

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// HistoryEntry represents a parsed browser history entry.
type HistoryEntry struct {
	URL       string        `json:"url"`
	Title     string        `json:"title"`
	VisitTime time.Time     `json:"visit_time"`
	DwellTime time.Duration `json:"dwell_time"`
	Domain    string        `json:"domain"`
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
// Matches both exact domains and subdomains (e.g. m.twitter.com, www.facebook.com)
// to ensure SCN-005-004 aggregation applies regardless of subdomain variant.
func IsSocialMedia(domain string) bool {
	if SocialMediaDomains[domain] {
		return true
	}
	// Match subdomains: m.twitter.com, www.facebook.com, mobile.reddit.com, etc.
	for d := range SocialMediaDomains {
		if strings.HasSuffix(domain, "."+d) {
			return true
		}
	}
	return false
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

	// Match user skip domains against the extracted domain.
	// Supports exact match and subdomain match (same pattern as IsSocialMedia)
	// so that skip domain "corp.com" also blocks "www.corp.com" and "sub.corp.com".
	for _, skip := range skipDomains {
		if domain == skip || strings.HasSuffix(domain, "."+skip) {
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

// ParseChromeHistorySince reads Chrome history entries with visit_time > cursor.
// Unlike ParseChromeHistory, this orders ASC for cursor-based incremental sync
// and limits results to 10000 entries per batch to prevent memory exhaustion.
func ParseChromeHistorySince(dbPath string, chromeTimeCursor int64) ([]HistoryEntry, error) {
	// SEC-005-001 (CWE-74): Validate dbPath doesn't contain SQLite DSN query
	// string characters. The function appends "?mode=ro" to enforce read-only
	// access, but a dbPath containing "?" would inject parameters that could
	// override mode=ro or set arbitrary PRAGMA values.
	if strings.ContainsAny(dbPath, "?#") {
		return nil, fmt.Errorf("invalid Chrome history path: contains query string characters")
	}
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
			slog.Warn("skipping malformed chrome history row", "error", err)
			continue
		}
		e.VisitTime = ChromeTimeToGo(visitTime)
		// CHAOS-F4: Clamp negative durations from corrupted SQLite data.
		if duration < 0 {
			duration = 0
		}
		e.DwellTime = time.Duration(duration) * time.Microsecond
		e.Domain = extractDomain(e.URL)
		entries = append(entries, e)
	}

	if err := rows.Err(); err != nil {
		return entries, fmt.Errorf("iterate history rows: %w", err)
	}

	return entries, nil
}

// GoTimeToChrome converts a Go time.Time to Chrome's microseconds-since-1601 format.
func GoTimeToChrome(t time.Time) int64 {
	const chromeEpochDiff = 11644473600000000
	return t.UnixMicro() + chromeEpochDiff
}

// ChromeTimeToGo converts Chrome's microseconds-since-1601 to time.Time.
func ChromeTimeToGo(chromeTime int64) time.Time {
	// Chrome epoch: 1601-01-01. Difference from Unix epoch in microseconds.
	const chromeEpochDiff = 11644473600000000
	unixMicro := chromeTime - chromeEpochDiff
	return time.UnixMicro(unixMicro)
}

func extractDomain(url string) string {
	// Simple domain extraction with safe bounds checks.
	// CHAOS-F6: Handle arbitrary schemes (ftp://, ws://, etc.) by finding "://".
	start := 0
	if len(url) >= 8 && url[:8] == "https://" {
		start = 8
	} else if len(url) >= 7 && url[:7] == "http://" {
		start = 7
	} else {
		// Check for any other scheme with "://"
		for i := 0; i < len(url)-2 && i < 32; i++ {
			if url[i] == ':' && i+2 < len(url) && url[i+1] == '/' && url[i+2] == '/' {
				start = i + 3
				break
			}
		}
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
