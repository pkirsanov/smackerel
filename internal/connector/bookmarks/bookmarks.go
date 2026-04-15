package bookmarks

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// maxExtractDepth limits Chrome JSON tree recursion to prevent stack overflow on malformed input.
const maxExtractDepth = 50

// maxReasonableUnixSec is the upper bound for bookmark timestamps (year 2100).
// F-CHAOS-R24-003: Rejects adversarial far-future date_added values.
const maxReasonableUnixSec int64 = 4102444800

// Pre-compiled regexes for Netscape HTML parsing (F-STAB-005).
var (
	netscapeLinkRe    = regexp.MustCompile(`<A HREF="([^"]+)"[^>]*>([^<]+)</A>`)
	netscapeFolderRe  = regexp.MustCompile(`<H3[^>]*>([^<]+)</H3>`)
	netscapeAddDateRe = regexp.MustCompile(`ADD_DATE="(\d+)"`)
)

// Bookmark represents a parsed bookmark.
type Bookmark struct {
	Title   string    `json:"title"`
	URL     string    `json:"url"`
	Folder  string    `json:"folder"`
	AddedAt time.Time `json:"added_at"`
}

// ParseChromeJSON parses Chrome's JSON bookmark export format.
func ParseChromeJSON(data []byte) ([]Bookmark, error) {
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse Chrome JSON: %w", err)
	}

	var bookmarks []Bookmark
	roots, ok := root["roots"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing 'roots' in Chrome bookmarks")
	}

	for _, bar := range roots {
		if node, ok := bar.(map[string]interface{}); ok {
			extractBookmarks(node, "", &bookmarks)
		}
	}

	return bookmarks, nil
}

// ParseNetscapeHTML parses the Netscape HTML bookmark format (exported by most browsers).
// IMP-009-R-001: Uses stack-based folder tracking so nested <DL>/<H3> structures
// produce correct hierarchical folder paths (e.g. "Tech/Go" instead of just "Go").
func ParseNetscapeHTML(data []byte) ([]Bookmark, error) {
	var bookmarks []Bookmark
	content := string(data)

	var folderStack []string
	pendingFolder := ""
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Detect folder headers — the next <DL> enters this folder.
		// F-CHAOS-005: Decode HTML entities in folder names.
		if matches := netscapeFolderRe.FindStringSubmatch(line); len(matches) > 1 {
			pendingFolder = html.UnescapeString(matches[1])
		}

		// Detect <DL> — push pending folder onto the hierarchy stack.
		if strings.HasPrefix(line, "<DL") {
			if pendingFolder != "" {
				folderStack = append(folderStack, pendingFolder)
				pendingFolder = ""
			}
		}

		// Detect </DL> — pop the innermost folder from the stack.
		if strings.HasPrefix(line, "</DL") {
			if len(folderStack) > 0 {
				folderStack = folderStack[:len(folderStack)-1]
			}
		}

		// Detect bookmark links — assign the current folder hierarchy.
		if matches := netscapeLinkRe.FindStringSubmatch(line); len(matches) > 2 {
			b := Bookmark{
				URL:    html.UnescapeString(matches[1]),
				Title:  html.UnescapeString(matches[2]),
				Folder: strings.Join(folderStack, "/"),
			}
			// Parse ADD_DATE attribute (unix timestamp) when present.
			if dateMatch := netscapeAddDateRe.FindStringSubmatch(line); len(dateMatch) > 1 {
				if ts, err := strconv.ParseInt(dateMatch[1], 10, 64); err == nil && ts > 0 {
					b.AddedAt = time.Unix(ts, 0)
				}
			}
			bookmarks = append(bookmarks, b)
		}
	}

	return bookmarks, nil
}

// ToRawArtifacts converts bookmarks to raw artifacts for pipeline processing.
// SourceRef is set to the normalized URL for consistent identity regardless of
// whether URL deduplication is enabled.
func ToRawArtifacts(bookmarks []Bookmark) []connector.RawArtifact {
	var artifacts []connector.RawArtifact
	for _, b := range bookmarks {
		artifacts = append(artifacts, connector.RawArtifact{
			SourceID:    "bookmarks",
			SourceRef:   NormalizeURL(b.URL),
			ContentType: "url",
			Title:       b.Title,
			RawContent:  b.URL,
			URL:         b.URL,
			Metadata: map[string]interface{}{
				"folder": b.Folder,
			},
			CapturedAt: b.AddedAt,
		})
	}
	return artifacts
}

// FolderToTopicMapping converts bookmark folder names to topic names.
func FolderToTopicMapping(folder string) string {
	if folder == "" {
		return ""
	}
	// Normalize: lowercase, trim, replace separators
	topic := strings.ToLower(strings.TrimSpace(folder))
	topic = strings.ReplaceAll(topic, "/", " ")
	topic = strings.ReplaceAll(topic, "\\", " ")
	return topic
}

func extractBookmarks(node map[string]interface{}, folder string, out *[]Bookmark) {
	extractBookmarksDepth(node, folder, out, 0)
}

func extractBookmarksDepth(node map[string]interface{}, folder string, out *[]Bookmark, depth int) {
	if depth > maxExtractDepth {
		return // prevent stack overflow on malformed input
	}

	nodeType, _ := node["type"].(string)
	name, _ := node["name"].(string)

	if nodeType == "url" {
		url, _ := node["url"].(string)
		if url != "" {
			b := Bookmark{
				Title:  name,
				URL:    url,
				Folder: folder,
			}
			// F-CHAOS-004: Parse Chrome's date_added field (microseconds since 1601-01-01 UTC).
			if dateStr, ok := node["date_added"].(string); ok && dateStr != "" {
				if us, err := strconv.ParseInt(dateStr, 10, 64); err == nil && us > 0 {
					// Chrome epoch offset: seconds between 1601-01-01 and 1970-01-01.
					const chromeEpochOffset int64 = 11644473600
					unixSec := (us / 1_000_000) - chromeEpochOffset
					unixNsec := (us % 1_000_000) * 1000
					// F-CHAOS-R24-003: Reject dates before Unix epoch or beyond year 2100.
					if unixSec > 0 && unixSec < maxReasonableUnixSec {
						b.AddedAt = time.Unix(unixSec, unixNsec)
					}
				}
			}
			*out = append(*out, b)
		}
		return
	}

	// Recurse into children
	children, ok := node["children"].([]interface{})
	if !ok {
		return
	}

	currentFolder := folder
	if name != "" && nodeType == "folder" {
		if currentFolder != "" {
			currentFolder += "/" + name
		} else {
			currentFolder = name
		}
	}

	for _, child := range children {
		if childNode, ok := child.(map[string]interface{}); ok {
			extractBookmarksDepth(childNode, currentFolder, out, depth+1)
		}
	}
}
