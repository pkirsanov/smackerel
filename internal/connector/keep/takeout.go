package keep

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TakeoutLabel represents a label in a Keep Takeout export.
type TakeoutLabel struct {
	Name string `json:"name"`
}

// TakeoutAnnotation represents a URL annotation in a Keep note.
type TakeoutAnnotation struct {
	Description string `json:"description"`
	Title       string `json:"title"`
	URL         string `json:"url"`
}

// TakeoutAttachment represents an attachment in a Keep note.
type TakeoutAttachment struct {
	FilePath string `json:"filePath"`
	MimeType string `json:"mimetype"`
}

// TakeoutListItem represents a checklist item in a Keep note.
type TakeoutListItem struct {
	Text      string `json:"text"`
	IsChecked bool   `json:"isChecked"`
}

// TakeoutSharee represents a collaborator on a Keep note.
type TakeoutSharee struct {
	Email      string `json:"email"`
	IsOwner    bool   `json:"isOwner"`
	Permission string `json:"permission"`
}

// TakeoutNote represents a parsed Google Keep Takeout export note.
type TakeoutNote struct {
	Color                   string              `json:"color"`
	IsTrashed               bool                `json:"isTrashed"`
	IsPinned                bool                `json:"isPinned"`
	IsArchived              bool                `json:"isArchived"`
	TextContent             string              `json:"textContent"`
	Title                   string              `json:"title"`
	UserEditedTimestampUsec int64               `json:"userEditedTimestampUsec"`
	CreatedTimestampUsec    int64               `json:"createdTimestampUsec"`
	Labels                  []TakeoutLabel      `json:"labels"`
	Annotations             []TakeoutAnnotation `json:"annotations"`
	Attachments             []TakeoutAttachment `json:"attachments"`
	ListContent             []TakeoutListItem   `json:"listContent"`
	Sharees                 []TakeoutSharee     `json:"sharees"`
}

// TakeoutParser parses Google Takeout Keep export directories.
type TakeoutParser struct{}

// NewTakeoutParser creates a new TakeoutParser.
func NewTakeoutParser() *TakeoutParser {
	return &TakeoutParser{}
}

// ParseExport parses all JSON note files in a Takeout export directory.
// Returns parsed notes and a list of file paths that failed to parse.
func (p *TakeoutParser) ParseExport(exportDir string) ([]TakeoutNote, []string, error) {
	entries, err := os.ReadDir(exportDir)
	if err != nil {
		return nil, nil, fmt.Errorf("read export directory %s: %w", exportDir, err)
	}

	var notes []TakeoutNote
	var errors []string

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		filePath := filepath.Join(exportDir, entry.Name())
		note, err := p.ParseNoteFile(filePath)
		if err != nil {
			errors = append(errors, filePath)
			continue
		}
		notes = append(notes, *note)
	}

	return notes, errors, nil
}

// ParseNoteFile parses a single Keep Takeout JSON file.
func (p *TakeoutParser) ParseNoteFile(filePath string) (*TakeoutNote, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read note file %s: %w", filePath, err)
	}

	var note TakeoutNote
	if err := json.Unmarshal(data, &note); err != nil {
		return nil, fmt.Errorf("parse note JSON %s: %w", filePath, err)
	}

	return &note, nil
}

// ModifiedAt returns the last modified time of a note.
func (p *TakeoutParser) ModifiedAt(note *TakeoutNote) time.Time {
	if note.UserEditedTimestampUsec > 0 {
		return time.UnixMicro(note.UserEditedTimestampUsec)
	}
	return time.UnixMicro(note.CreatedTimestampUsec)
}

// CreatedAt returns the creation time of a note.
func (p *TakeoutParser) CreatedAt(note *TakeoutNote) time.Time {
	return time.UnixMicro(note.CreatedTimestampUsec)
}

// NoteID derives a stable note ID from the file path.
// Uses the filename without extension as the ID.
func (p *TakeoutParser) NoteID(note *TakeoutNote, filePath string) string {
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// FilterByCursor returns only notes with modified_at after the cursor timestamp.
// Returns the filtered notes and the latest modified_at timestamp as the new cursor.
func (p *TakeoutParser) FilterByCursor(notes []TakeoutNote, cursor string) ([]TakeoutNote, string) {
	var cursorTime time.Time
	if cursor != "" {
		var err error
		cursorTime, err = time.Parse(time.RFC3339, cursor)
		if err != nil {
			// Invalid cursor — return all notes
			cursorTime = time.Time{}
		}
	}

	var filtered []TakeoutNote
	var latestModified time.Time

	for i := range notes {
		modifiedAt := p.ModifiedAt(&notes[i])
		if !cursorTime.IsZero() && !modifiedAt.After(cursorTime) {
			continue
		}
		filtered = append(filtered, notes[i])
		if modifiedAt.After(latestModified) {
			latestModified = modifiedAt
		}
	}

	newCursor := cursor
	if !latestModified.IsZero() {
		newCursor = latestModified.UTC().Format(time.RFC3339)
	}

	return filtered, newCursor
}
