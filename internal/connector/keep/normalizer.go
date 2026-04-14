package keep

import (
	"fmt"
	"net/url"
	"path/filepath"
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
	content := n.buildContent(note)
	if n.shouldSkip(note, content) {
		return nil, nil
	}

	noteType := n.classifyNote(note)
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

	// Image attachment references — use sanitized base filename only (CWE-22)
	for _, a := range note.Attachments {
		safePath := sanitizeAttachmentPath(a.FilePath)
		if safePath == "" {
			safePath = "(unknown)"
		}
		if strings.HasPrefix(a.MimeType, "image/") {
			parts = append(parts, fmt.Sprintf("[Image attached: %s]", safePath))
		}
		if strings.HasPrefix(a.MimeType, "audio/") {
			parts = append(parts, fmt.Sprintf("[Audio attached: %s]", safePath))
		}
	}

	return strings.Join(parts, "\n")
}

// maxMetadataArrayLen caps metadata arrays to prevent resource exhaustion
// from crafted Takeout exports with excessive labels/annotations/etc (CWE-400).
const maxMetadataArrayLen = 500

// isBasicEmail performs a minimal format check on email addresses to prevent
// injection via metadata if content is rendered in a web context (CWE-79).
func isBasicEmail(s string) bool {
	// Must contain exactly one @, non-empty local and domain parts, no angle brackets or spaces.
	if strings.ContainsAny(s, "<> \t\n\r") {
		return false
	}
	at := strings.Index(s, "@")
	if at < 1 || at >= len(s)-1 {
		return false
	}
	// Only one @
	if strings.Count(s, "@") != 1 {
		return false
	}
	return true
}

// sanitizeAttachmentPath strips path traversal sequences from attachment file paths
// to prevent directory traversal if a downstream consumer resolves them (CWE-22).
func sanitizeAttachmentPath(p string) string {
	// Use only the base name — attachment paths in Takeout are relative filenames.
	base := filepath.Base(p)
	// filepath.Base returns "." for empty strings — treat as empty.
	if base == "." {
		return ""
	}
	return base
}

// buildMetadata constructs the R-005 metadata map.
func (n *Normalizer) buildMetadata(note *TakeoutNote, noteID, sourcePath string) map[string]interface{} {
	parser := n.parser

	labelLen := len(note.Labels)
	if labelLen > maxMetadataArrayLen {
		labelLen = maxMetadataArrayLen
	}
	labels := make([]string, 0, labelLen)
	for i, l := range note.Labels {
		if i >= maxMetadataArrayLen {
			break
		}
		labels = append(labels, l.Name)
	}

	collabLen := len(note.Sharees)
	if collabLen > maxMetadataArrayLen {
		collabLen = maxMetadataArrayLen
	}
	collaborators := make([]string, 0, collabLen)
	for i, s := range note.Sharees {
		if i >= maxMetadataArrayLen {
			break
		}
		if s.Email != "" && isBasicEmail(s.Email) {
			collaborators = append(collaborators, s.Email)
		}
	}

	annLen := len(note.Annotations)
	if annLen > maxMetadataArrayLen {
		annLen = maxMetadataArrayLen
	}
	annotations := make([]map[string]string, 0, annLen)
	for i, a := range note.Annotations {
		if i >= maxMetadataArrayLen {
			break
		}
		// Apply same URL scheme filtering to metadata as to content (CWE-79).
		sanitizedURL := a.URL
		if sanitizedURL != "" && !isSafeURL(sanitizedURL) {
			sanitizedURL = ""
		}
		annotations = append(annotations, map[string]string{
			"url":         sanitizedURL,
			"title":       a.Title,
			"description": a.Description,
		})
	}

	attLen := len(note.Attachments)
	if attLen > maxMetadataArrayLen {
		attLen = maxMetadataArrayLen
	}
	attachments := make([]map[string]string, 0, attLen)
	for i, a := range note.Attachments {
		if i >= maxMetadataArrayLen {
			break
		}
		attachments = append(attachments, map[string]string{
			"file_path": sanitizeAttachmentPath(a.FilePath),
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
// content is the pre-built output of buildContent to avoid redundant work.
//
// Priority mirrors R-008 qualifier evaluation order:
// trashed → skip always (highest priority)
// pinned/labeled/has-images → never skip (these are high-signal notes)
// archived + !IncludeArchived → skip (user-deprioritized)
// empty/short content → skip
func (n *Normalizer) shouldSkip(note *TakeoutNote, content string) bool {
	if note.IsTrashed {
		return true
	}
	if note.IsArchived && !n.config.IncludeArchived {
		// High-signal notes survive archived exclusion per R-008 priority:
		// pinned, labeled, and image-bearing notes are never dropped.
		if note.IsPinned {
			return false
		}
		if len(note.Labels) > 0 {
			return false
		}
		hasImages := false
		for _, a := range note.Attachments {
			if strings.HasPrefix(a.MimeType, "image/") {
				hasImages = true
				break
			}
		}
		if hasImages {
			return false
		}
		return true
	}
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
