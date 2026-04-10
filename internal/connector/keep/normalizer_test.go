package keep

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeTextNote(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Title:                   "Team Reorg Ideas",
		TextContent:             "We should reorganize the engineering team",
		IsPinned:                true,
		Color:                   "BLUE",
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().Add(-24 * time.Hour).UnixMicro(),
		Labels:                  []TakeoutLabel{{Name: "Work Ideas"}, {Name: "ML"}},
		Sharees:                 []TakeoutSharee{{Email: "alice@example.com"}},
	}

	artifact, err := n.Normalize(note, "test-note-1", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}
	if artifact.SourceID != "google-keep" {
		t.Errorf("SourceID = %q, want google-keep", artifact.SourceID)
	}
	if artifact.ContentType != "note/text" {
		t.Errorf("ContentType = %q, want note/text", artifact.ContentType)
	}
}

func TestNormalizeChecklistContent(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Title: "Packing List",
		ListContent: []TakeoutListItem{
			{Text: "Passport", IsChecked: true},
			{Text: "Charger", IsChecked: false},
		},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, _ := n.Normalize(note, "checklist-1", "takeout")
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}
	if !strings.Contains(artifact.RawContent, "- [x] Passport") {
		t.Errorf("content missing checked item: %q", artifact.RawContent)
	}
	if !strings.Contains(artifact.RawContent, "- [ ] Charger") {
		t.Errorf("content missing unchecked item: %q", artifact.RawContent)
	}
}

func TestNormalizeMixedContent(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Title:                   "Mixed Note",
		TextContent:             "Some text content",
		ListContent:             []TakeoutListItem{{Text: "Item 1", IsChecked: false}},
		Attachments:             []TakeoutAttachment{{FilePath: "photo.jpg", MimeType: "image/jpeg"}},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, _ := n.Normalize(note, "mixed-1", "takeout")
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}
	if artifact.ContentType != "note/mixed" {
		t.Errorf("ContentType = %q, want note/mixed", artifact.ContentType)
	}
	if !strings.Contains(artifact.RawContent, "Some text content") {
		t.Error("missing text content")
	}
	if !strings.Contains(artifact.RawContent, "[Image attached:") {
		t.Error("missing image attachment reference")
	}
}

func TestMetadataMapping(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Title:                   "Metadata Test",
		TextContent:             "Note body with metadata fields populated",
		IsPinned:                true,
		IsArchived:              false,
		IsTrashed:               false,
		Color:                   "BLUE",
		Labels:                  []TakeoutLabel{{Name: "Work Ideas"}, {Name: "ML"}},
		Sharees:                 []TakeoutSharee{{Email: "alice@example.com"}},
		Annotations:             []TakeoutAnnotation{{URL: "https://example.com", Title: "Example"}},
		Attachments:             []TakeoutAttachment{{FilePath: "photo.jpg", MimeType: "image/jpeg"}},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().Add(-24 * time.Hour).UnixMicro(),
	}

	artifact, _ := n.Normalize(note, "metadata-1", "takeout")
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}

	// Check all 14 R-005 metadata fields (13 spec fields + processing_tier)
	requiredFields := []string{
		"keep_note_id", "pinned", "archived", "trashed", "labels",
		"color", "collaborators", "reminder_time", "annotations", "attachments",
		"source_path", "created_at", "modified_at", "processing_tier",
	}
	for _, field := range requiredFields {
		if _, ok := artifact.Metadata[field]; !ok {
			t.Errorf("missing metadata field: %s", field)
		}
	}

	if artifact.Metadata["pinned"] != true {
		t.Errorf("pinned = %v, want true", artifact.Metadata["pinned"])
	}
	if artifact.Metadata["source_path"] != "takeout" {
		t.Errorf("source_path = %v, want takeout", artifact.Metadata["source_path"])
	}
}

func TestClassifyNoteTypes(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	tests := []struct {
		name     string
		note     TakeoutNote
		expected NoteType
	}{
		{"text", TakeoutNote{TextContent: "hello"}, NoteTypeText},
		{"checklist", TakeoutNote{ListContent: []TakeoutListItem{{Text: "item"}}}, NoteTypeChecklist},
		{"image", TakeoutNote{Attachments: []TakeoutAttachment{{MimeType: "image/jpeg"}}}, NoteTypeImage},
		{"audio", TakeoutNote{Attachments: []TakeoutAttachment{{MimeType: "audio/3gpp"}}}, NoteTypeAudio},
		{"mixed-text-list", TakeoutNote{TextContent: "text", ListContent: []TakeoutListItem{{Text: "item"}}}, NoteTypeMixed},
		{"mixed-text-image", TakeoutNote{TextContent: "text", Attachments: []TakeoutAttachment{{MimeType: "image/jpeg"}}}, NoteTypeMixed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := n.classifyNote(&tt.note)
			if got != tt.expected {
				t.Errorf("classifyNote = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAssignTierPinned(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{IsPinned: true, UserEditedTimestampUsec: time.Now().UnixMicro()}
	if tier := n.assignTier(note); tier != TierFull {
		t.Errorf("tier = %q, want full", tier)
	}
}

func TestAssignTierLabeled(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Labels:                  []TakeoutLabel{{Name: "Work"}},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
	}
	if tier := n.assignTier(note); tier != TierFull {
		t.Errorf("tier = %q, want full", tier)
	}
}

func TestAssignTierArchived(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		IsArchived:              true,
		UserEditedTimestampUsec: time.Now().Add(-60 * 24 * time.Hour).UnixMicro(),
	}
	if tier := n.assignTier(note); tier != TierLight {
		t.Errorf("tier = %q, want light", tier)
	}
}

func TestShouldSkipTrashed(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{IsTrashed: true, TextContent: "Some content"}
	if !n.shouldSkip(note) {
		t.Error("trashed note should be skipped")
	}
}

func TestShouldSkipShortContent(t *testing.T) {
	n := NewNormalizer(KeepConfig{MinContentLength: 5})
	note := &TakeoutNote{TextContent: "Hi"}
	if !n.shouldSkip(note) {
		t.Error("short content note should be skipped")
	}
}

func TestAudioAttachmentInContent(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Title: "Voice Memo",
		Attachments: []TakeoutAttachment{
			{FilePath: "recording.3gp", MimeType: "audio/3gpp"},
		},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "audio-1", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}
	if !strings.Contains(artifact.RawContent, "[Audio attached: recording.3gp]") {
		t.Errorf("content missing audio reference: %q", artifact.RawContent)
	}
}

func TestNormalizerDelegatesToQualifier(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	q := NewQualifier()

	// Verify normalizer and qualifier produce the same tier for various notes
	cases := []TakeoutNote{
		{IsPinned: true, UserEditedTimestampUsec: time.Now().UnixMicro()},
		{Labels: []TakeoutLabel{{Name: "Work"}}, UserEditedTimestampUsec: time.Now().UnixMicro()},
		{IsArchived: true, UserEditedTimestampUsec: time.Now().Add(-60 * 24 * time.Hour).UnixMicro()},
		{TextContent: "recent", UserEditedTimestampUsec: time.Now().Add(-5 * 24 * time.Hour).UnixMicro()},
	}

	for i, note := range cases {
		normTier := n.assignTier(&note)
		qualTier := q.Evaluate(&note).Tier
		if normTier != qualTier {
			t.Errorf("case %d: normalizer tier=%q, qualifier tier=%q — should be identical", i, normTier, qualTier)
		}
	}
}

func TestEmptyTitleFallback(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		TextContent:             "This is a note without a title that should use content as fallback for the title field",
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, _ := n.Normalize(note, "no-title", "takeout")
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}
	if artifact.Title == "" {
		t.Error("title should not be empty — should use content prefix")
	}
	if len(artifact.Title) > 50 {
		t.Errorf("title length = %d, should be capped at 50", len(artifact.Title))
	}
}

// --- Security Tests ---

func TestAnnotationURLSchemeFiltering(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	tests := []struct {
		name        string
		url         string
		shouldExist bool
	}{
		{"https allowed", "https://example.com", true},
		{"http allowed", "http://example.com", true},
		{"mailto allowed", "mailto:user@example.com", true},
		{"javascript blocked", "javascript:alert(1)", false},
		{"data blocked", "data:text/html,<script>alert(1)</script>", false},
		{"vbscript blocked", "vbscript:MsgBox(1)", false},
		{"file blocked", "file:///etc/passwd", false},
		{"ftp blocked", "ftp://evil.com/payload", false},
		{"empty scheme blocked", "://no-scheme", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			note := &TakeoutNote{
				TextContent:             "test",
				Annotations:             []TakeoutAnnotation{{URL: tt.url, Title: "Link"}},
				UserEditedTimestampUsec: time.Now().UnixMicro(),
				CreatedTimestampUsec:    time.Now().UnixMicro(),
			}
			artifact, _ := n.Normalize(note, "scheme-test", "takeout")
			if artifact == nil {
				t.Fatal("artifact should not be nil")
			}
			containsURL := strings.Contains(artifact.RawContent, tt.url)
			if tt.shouldExist && !containsURL {
				t.Errorf("SECURITY: safe URL %q was stripped from content", tt.url)
			}
			if !tt.shouldExist && containsURL {
				t.Errorf("SECURITY: dangerous URL scheme %q was included in content", tt.url)
			}
		})
	}
}

func TestIsSafeURL(t *testing.T) {
	tests := []struct {
		url  string
		safe bool
	}{
		{"https://example.com/path", true},
		{"http://localhost:8080", true},
		{"mailto:user@example.com", true},
		{"javascript:alert(document.cookie)", false},
		{"data:text/html,<h1>XSS</h1>", false},
		{"vbscript:MsgBox", false},
		{"file:///etc/shadow", false},
		{"", false},
		{"://missing-scheme", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			if got := isSafeURL(tt.url); got != tt.safe {
				t.Errorf("isSafeURL(%q) = %v, want %v", tt.url, got, tt.safe)
			}
		})
	}
}

func TestNormalizeGkeepNote(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	gNote := &GkeepNote{
		NoteID:        "gkeep-abc-123",
		Title:         "Quick Idea",
		TextContent:   "This is a quick idea from gkeepapi",
		IsPinned:      true,
		IsArchived:    false,
		IsTrashed:     false,
		Color:         "YELLOW",
		Labels:        []string{"Ideas", "ML"},
		Collaborators: []string{"bob@example.com"},
		ListItems: []struct {
			Text      string `json:"text"`
			IsChecked bool   `json:"is_checked"`
		}{
			{Text: "Research topic", IsChecked: false},
			{Text: "Draft outline", IsChecked: true},
		},
		ModifiedUsec: time.Now().UnixMicro(),
		CreatedUsec:  time.Now().Add(-48 * time.Hour).UnixMicro(),
	}

	artifact, err := n.NormalizeGkeep(gNote)
	if err != nil {
		t.Fatalf("NormalizeGkeep: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}
	if artifact.SourceID != "google-keep" {
		t.Errorf("SourceID = %q, want google-keep", artifact.SourceID)
	}
	if artifact.SourceRef != "gkeep-abc-123" {
		t.Errorf("SourceRef = %q, want gkeep-abc-123", artifact.SourceRef)
	}
	if artifact.ContentType != "note/mixed" {
		t.Errorf("ContentType = %q, want note/mixed (has text + list)", artifact.ContentType)
	}
	if !strings.Contains(artifact.RawContent, "quick idea") {
		t.Error("missing text content from gkeep note")
	}
	if !strings.Contains(artifact.RawContent, "- [x] Draft outline") {
		t.Error("missing checked list item from gkeep note")
	}
	if !strings.Contains(artifact.RawContent, "- [ ] Research topic") {
		t.Error("missing unchecked list item from gkeep note")
	}
	if artifact.Metadata["source_path"] != "gkeepapi" {
		t.Errorf("source_path = %v, want gkeepapi", artifact.Metadata["source_path"])
	}
	labels, ok := artifact.Metadata["labels"].([]string)
	if !ok || len(labels) != 2 {
		t.Errorf("labels = %v, want [Ideas ML]", artifact.Metadata["labels"])
	}
	collabs, ok := artifact.Metadata["collaborators"].([]string)
	if !ok || len(collabs) != 1 || collabs[0] != "bob@example.com" {
		t.Errorf("collaborators = %v, want [bob@example.com]", artifact.Metadata["collaborators"])
	}
}

func TestNormalizeGkeepTrashedSkipped(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	gNote := &GkeepNote{
		NoteID:       "gkeep-trash-1",
		Title:        "Trashed Note",
		TextContent:  "Should be skipped",
		IsTrashed:    true,
		ModifiedUsec: time.Now().UnixMicro(),
		CreatedUsec:  time.Now().UnixMicro(),
	}

	artifact, err := n.NormalizeGkeep(gNote)
	if err != nil {
		t.Fatalf("NormalizeGkeep: %v", err)
	}
	if artifact != nil {
		t.Error("trashed gkeep note should return nil artifact")
	}
}
