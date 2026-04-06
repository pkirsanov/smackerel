package bookmarks

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
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
func ParseNetscapeHTML(data []byte) ([]Bookmark, error) {
	// Simplified parser for Netscape bookmark format
	var bookmarks []Bookmark
	content := string(data)

	// Extract links with href attributes
	linkRe := regexp.MustCompile(`<A HREF="([^"]+)"[^>]*>([^<]+)</A>`)
	folderRe := regexp.MustCompile(`<H3[^>]*>([^<]+)</H3>`)

	currentFolder := ""
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if matches := folderRe.FindStringSubmatch(line); len(matches) > 1 {
			currentFolder = matches[1]
			continue
		}

		if matches := linkRe.FindStringSubmatch(line); len(matches) > 2 {
			bookmarks = append(bookmarks, Bookmark{
				URL:    matches[1],
				Title:  matches[2],
				Folder: currentFolder,
			})
		}
	}

	return bookmarks, nil
}

// ToRawArtifacts converts bookmarks to raw artifacts for pipeline processing.
func ToRawArtifacts(bookmarks []Bookmark) []connector.RawArtifact {
	var artifacts []connector.RawArtifact
	for _, b := range bookmarks {
		artifacts = append(artifacts, connector.RawArtifact{
			SourceID:    "bookmarks",
			SourceRef:   b.URL,
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
	nodeType, _ := node["type"].(string)
	name, _ := node["name"].(string)

	if nodeType == "url" {
		url, _ := node["url"].(string)
		if url != "" {
			*out = append(*out, Bookmark{
				Title:  name,
				URL:    url,
				Folder: folder,
			})
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
			extractBookmarks(childNode, currentFolder, out)
		}
	}
}
