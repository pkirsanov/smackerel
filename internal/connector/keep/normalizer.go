package keep

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// NoteType classifies the type of Keep note content.
type NoteType string

const (
	NoteTypeText      NoteType = "note/text"
	NoteTypeChecklist NoteType = "note/checklist"
	NoteTypeImage     NoteType = "note/image"
	NoteTypeAudio     NoteType = "note/audio"
	NoteTypeMixed     NoteType = "note/mixed"
)

// Normalizer converts TakeoutNote structs into connector.RawArtifact.
type Normalizer struct {
	config    KeepConfig
	parser    *TakeoutParser
	qualifier *Qualifier
}

// NewNormalizer creates a new Normalizer with the given config.
func NewNormalizer(config KeepConfig) *Normalizer {
	return &Normalizer{config: config, parser: NewTakeoutParser(), qualifier: NewQualifier()}
}

// Normalize converts a TakeoutNote into a RawArtifact.
func (n *Normalizer) Normalize(note *TakeoutNote, noteID, sourcePath string) (*connector.RawArtifact, error) {
	if n.shouldSkip(note) {
		return nil, nil
	}

	noteType := n.classifyNote(note)
	content := n.buildContent(note)
	title := note.Title
	if title == "" && len(content) > 0 {
		title = content
		runes := []rune(title)
		if len(runes) > 50 {
			title = string(runes[:50])
		}
	}

	metadata := n.buildMetadata(note, noteID, sourcePath)
	tier := n.assignTier(note)
	metadata["processing_tier"] = string(tier)

	capturedAt := n.parser.CreatedAt(note)
	if capturedAt.IsZero() || capturedAt.Equal(time.Unix(0, 0)) {
		capturedAt = time.Now()
	}

	return &connector.RawArtifact{
		SourceID:    "google-keep",
		SourceRef:   noteID,
		ContentType: string(noteType),
		Title:       title,
		RawContent:  content,
		Metadata:    metadata,
		CapturedAt:  capturedAt,
	}, nil
}

// NormalizeGkeep converts a GkeepNote (from gkeepapi bridge) into a RawArtifact.
func (n *Normalizer) NormalizeGkeep(note *GkeepNote) (*connector.RawArtifact, error) {
	// Convert GkeepNote to TakeoutNote for unified processing
	takeout := &TakeoutNote{
		Title:                   note.Title,
		TextContent:             note.TextContent,
		IsPinned:                note.IsPinned,
		IsArchived:              note.IsArchived,
		IsTrashed:               note.IsTrashed,
		Color:                   note.Color,
		UserEditedTimestampUsec: note.ModifiedUsec,
		CreatedTimestampUsec:    note.CreatedUsec,
	}
	for _, l := range note.Labels {
		takeout.Labels = append(takeout.Labels, TakeoutLabel{Name: l})
	}
	for _, li := range note.ListItems {
		takeout.ListContent = append(takeout.ListContent, TakeoutListItem{
			Text:      li.Text,
			IsChecked: li.IsChecked,
		})
	}
	for _, c := range note.Collaborators {
		takeout.Sharees = append(takeout.Sharees, TakeoutSharee{Email: c})
	}

	return n.Normalize(takeout, note.NoteID, "gkeepapi")
}

// classifyNote determines the note type based on content.
// Priority: mixed > checklist > image > audio > text
func (n *Normalizer) classifyNote(note *TakeoutNote) NoteType {
	hasText := note.TextContent != ""
	hasList := len(note.ListContent) > 0
	hasImages := false
	hasAudio := false

	for _, a := range note.Attachments {
		if strings.HasPrefix(a.MimeType, "image/") {
			hasImages = true
		}
		if strings.HasPrefix(a.MimeType, "audio/") {
			hasAudio = true
		}
	}

	// Mixed: has text/list AND attachments, or has both text and list
	mixedCount := 0
	if hasText {
		mixedCount++
	}
	if hasList {
		mixedCount++
	}
	if hasImages {
		mixedCount++
	}
	if hasAudio {
		mixedCount++
	}
	if mixedCount >= 2 {
		return NoteTypeMixed
	}

	if hasList {
		return NoteTypeChecklist
	}
	if hasImages {
		return NoteTypeImage
	}
	if hasAudio {
		return NoteTypeAudio
	}
	return NoteTypeText
}

// safeAnnotationSchemes is the allowlist of URL schemes permitted in Keep note annotations.
// URLs with other schemes (e.g. javascript:, data:, vbscript:) are stripped to prevent
// injection if content is rendered in a web context (CWE-79).
var safeAnnotationSchemes = map[string]bool{
	"http":   true,
	"https":  true,
	"mailto": true,
}

// isSafeURL checks that a URL uses an allowed scheme.
func isSafeURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return safeAnnotationSchemes[strings.ToLower(u.Scheme)]
}

// buildContent constructs the text content for the artifact.
func (n *Normalizer) buildContent(note *TakeoutNote) string {
	var parts []string

	// Annotations as link references — only safe URL schemes
	for _, ann := range note.Annotations {
		if ann.URL != "" && isSafeURL(ann.URL) {
			label := ann.Title
			if label == "" {
				label = ann.Description
			}
			parts = append(parts, fmt.Sprintf("[Link: %s — %s]", ann.URL, label))
		}
	}

	// Text content
	if note.TextContent != "" {
		parts = append(parts, note.TextContent)
	}

	// Checklist items
	for _, item := range note.ListContent {
		if item.IsChecked {
			parts = append(parts, fmt.Sprintf("- [x] %s", item.Text))
		} else {
			parts = append(parts, fmt.Sprintf("- [ ] %s", item.Text))
		}
	}

	// Image attachment references
	for _, a := range note.Attachments {
		if strings.HasPrefix(a.MimeType, "image/") {
			parts = append(parts, fmt.Sprintf("[Image attached: %s]", a.FilePath))
		}
		if strings.HasPrefix(a.MimeType, "audio/") {
			parts = append(parts, fmt.Sprintf("[Audio attached: %s]", a.FilePath))
		}
	}

	return strings.Join(parts, "\n")
}

// buildMetadata constructs the R-005 metadata map.
func (n *Normalizer) buildMetadata(note *TakeoutNote, noteID, sourcePath string) map[string]interface{} {
	parser := n.parser

	labels := make([]string, 0, len(note.Labels))
	for _, l := range note.Labels {
		labels = append(labels, l.Name)
	}

	collaborators := make([]string, 0, len(note.Sharees))
	for _, s := range note.Sharees {
		if s.Email != "" {
			collaborators = append(collaborators, s.Email)
		}
	}

	annotations := make([]map[string]string, 0, len(note.Annotations))
	for _, a := range note.Annotations {
		annotations = append(annotations, map[string]string{
			"url":         a.URL,
			"title":       a.Title,
			"description": a.Description,
		})
	}

	attachments := make([]map[string]string, 0, len(note.Attachments))
	for _, a := range note.Attachments {
		attachments = append(attachments, map[string]string{
			"file_path": a.FilePath,
			"mime_type": a.MimeType,
		})
	}

	modifiedAt := parser.ModifiedAt(note)
	createdAt := parser.CreatedAt(note)

	// Guard against epoch timestamps from missing/zero fields
	now := time.Now().UTC()
	if modifiedAt.Equal(time.Unix(0, 0)) || modifiedAt.IsZero() {
		modifiedAt = now
	}
	if createdAt.Equal(time.Unix(0, 0)) || createdAt.IsZero() {
		createdAt = now
	}

	metadata := map[string]interface{}{
		"keep_note_id":  noteID,
		"pinned":        note.IsPinned,
		"archived":      note.IsArchived,
		"trashed":       note.IsTrashed,
		"labels":        labels,
		"color":         note.Color,
		"collaborators": collaborators,
		"reminder_time": "", // R-005: not present in Takeout JSON, placeholder for gkeepapi path
		"annotations":   annotations,
		"attachments":   attachments,
		"source_path":   sourcePath,
		"created_at":    createdAt.UTC().Format(time.RFC3339),
		"modified_at":   modifiedAt.UTC().Format(time.RFC3339),
	}

	return metadata
}

// shouldSkip determines whether a note should be skipped entirely.
func (n *Normalizer) shouldSkip(note *TakeoutNote) bool {
	if note.IsTrashed {
		return true
	}
	if note.IsArchived && !n.config.IncludeArchived {
		return true
	}
	content := n.buildContent(note)
	// Skip completely empty notes (no text, no list, no attachments, no annotations, no title)
	if content == "" && note.Title == "" {
		return true
	}
	if n.config.MinContentLength > 0 && len(content) < n.config.MinContentLength {
		return true
	}
	return false
}

// assignTier delegates processing tier assignment to the Qualifier engine
// to ensure a single source of truth for R-008 evaluation rules.
func (n *Normalizer) assignTier(note *TakeoutNote) Tier {
	return n.qualifier.Evaluate(note).Tier
}

// Tier represents a processing tier for Keep notes.
type Tier string

const (
	TierFull     Tier = "full"
	TierStandard Tier = "standard"
	TierLight    Tier = "light"
	TierSkip     Tier = "skip"
)
