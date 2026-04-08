package keep

import (
	"fmt"
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
	config KeepConfig
}

// NewNormalizer creates a new Normalizer with the given config.
func NewNormalizer(config KeepConfig) *Normalizer {
	return &Normalizer{config: config}
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
		if len(title) > 50 {
			title = title[:50]
		}
	}

	metadata := n.buildMetadata(note, noteID, sourcePath)
	tier := n.assignTier(note)
	metadata["processing_tier"] = string(tier)

	parser := NewTakeoutParser()
	capturedAt := parser.CreatedAt(note)
	if capturedAt.IsZero() {
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

// buildContent constructs the text content for the artifact.
func (n *Normalizer) buildContent(note *TakeoutNote) string {
	var parts []string

	// Annotations as link references
	for _, ann := range note.Annotations {
		if ann.URL != "" {
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
	}

	return strings.Join(parts, "\n")
}

// buildMetadata constructs the R-005 metadata map.
func (n *Normalizer) buildMetadata(note *TakeoutNote, noteID, sourcePath string) map[string]interface{} {
	parser := NewTakeoutParser()

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

	metadata := map[string]interface{}{
		"keep_note_id":  noteID,
		"pinned":        note.IsPinned,
		"archived":      note.IsArchived,
		"trashed":       note.IsTrashed,
		"labels":        labels,
		"color":         note.Color,
		"collaborators": collaborators,
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
	if n.config.MinContentLength > 0 && len(content) < n.config.MinContentLength {
		return true
	}
	return false
}

// assignTier assigns a processing tier based on note properties.
// Evaluation order: trashed→skip, pinned→full, labeled→full, images→full,
// recent(<30d)→standard, archived→light, old(>30d)→light, default→standard.
func (n *Normalizer) assignTier(note *TakeoutNote) Tier {
	if note.IsTrashed {
		return TierSkip
	}
	if note.IsPinned {
		return TierFull
	}
	if len(note.Labels) > 0 {
		return TierFull
	}
	for _, a := range note.Attachments {
		if strings.HasPrefix(a.MimeType, "image/") {
			return TierFull
		}
	}

	parser := NewTakeoutParser()
	modifiedAt := parser.ModifiedAt(note)
	daysSinceModified := time.Since(modifiedAt).Hours() / 24

	if daysSinceModified <= 30 {
		return TierStandard
	}
	if note.IsArchived {
		return TierLight
	}
	if daysSinceModified > 30 {
		return TierLight
	}
	return TierStandard
}

// Tier represents a processing tier for Keep notes.
type Tier string

const (
	TierFull     Tier = "full"
	TierStandard Tier = "standard"
	TierLight    Tier = "light"
	TierSkip     Tier = "skip"
)
