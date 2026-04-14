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
	if !n.shouldSkip(note, n.buildContent(note)) {
		t.Error("trashed note should be skipped")
	}
}

func TestShouldSkipShortContent(t *testing.T) {
	n := NewNormalizer(KeepConfig{MinContentLength: 5})
	note := &TakeoutNote{TextContent: "Hi"}
	if !n.shouldSkip(note, n.buildContent(note)) {
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

// --- Edge case: audio-only note (no text, no title) ---

func TestNormalizeAudioOnlyNoTitle(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Title:       "",
		TextContent: "",
		Attachments: []TakeoutAttachment{
			{FilePath: "voice-memo.3gp", MimeType: "audio/3gpp"},
		},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "audio-only", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("audio-only note should not be skipped")
	}
	if artifact.ContentType != "note/audio" {
		t.Errorf("ContentType = %q, want note/audio", artifact.ContentType)
	}
	if artifact.Title == "" {
		t.Error("title should be derived from content for audio-only note")
	}
	if !strings.Contains(artifact.RawContent, "[Audio attached: voice-memo.3gp]") {
		t.Errorf("missing audio reference: %q", artifact.RawContent)
	}
}

// --- Edge case: annotation-only note ---

func TestNormalizeAnnotationOnlyNote(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Title:       "",
		TextContent: "",
		Annotations: []TakeoutAnnotation{
			{URL: "https://example.com/article", Title: "Great Article", Description: "Worth reading"},
		},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "ann-only", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("annotation-only note should not be skipped (has content via URL)")
	}
	if !strings.Contains(artifact.RawContent, "https://example.com/article") {
		t.Error("missing annotation URL in content")
	}
}

// --- Edge case: title-only note (no other content) ---

func TestNormalizeTitleOnlyNote(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Title:                   "Just a title, nothing else",
		TextContent:             "",
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "title-only", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	// Title-only note should not be skipped because it has a title
	if artifact == nil {
		t.Fatal("title-only note should not be skipped")
	}
	if artifact.Title != "Just a title, nothing else" {
		t.Errorf("title = %q, want original title", artifact.Title)
	}
}

// --- shouldSkip: archived with IncludeArchived=true ---

func TestShouldSkipArchivedIncluded(t *testing.T) {
	n := NewNormalizer(KeepConfig{IncludeArchived: true})
	note := &TakeoutNote{
		IsArchived:              true,
		TextContent:             "Archived note that should be included",
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}
	if n.shouldSkip(note, n.buildContent(note)) {
		t.Error("archived note should NOT be skipped when IncludeArchived=true")
	}
}

// --- Security: Metadata URL filtering (S1) ---

func TestMetadataAnnotationURLsFiltered(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Title:       "Metadata URL Test",
		TextContent: "Test note",
		Annotations: []TakeoutAnnotation{
			{URL: "https://safe.example.com", Title: "Safe"},
			{URL: "javascript:alert(document.cookie)", Title: "XSS"},
			{URL: "data:text/html,<script>alert(1)</script>", Title: "Data"},
			{URL: "http://also-safe.example.com", Title: "Also Safe"},
		},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "meta-url-test", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}

	anns, ok := artifact.Metadata["annotations"].([]map[string]string)
	if !ok {
		t.Fatal("annotations metadata should be []map[string]string")
	}
	if len(anns) != 4 {
		t.Fatalf("annotations count = %d, want 4", len(anns))
	}

	// Safe URLs preserved in metadata
	if anns[0]["url"] != "https://safe.example.com" {
		t.Errorf("safe URL stripped from metadata: %q", anns[0]["url"])
	}
	if anns[3]["url"] != "http://also-safe.example.com" {
		t.Errorf("safe URL stripped from metadata: %q", anns[3]["url"])
	}

	// Dangerous URLs replaced with empty string
	if anns[1]["url"] != "" {
		t.Errorf("SECURITY: javascript: URL in metadata: %q", anns[1]["url"])
	}
	if anns[2]["url"] != "" {
		t.Errorf("SECURITY: data: URL in metadata: %q", anns[2]["url"])
	}
}

// --- Security: Attachment path sanitization (S2) ---

func TestAttachmentPathTraversalSanitized(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Title:       "Path Traversal Test",
		TextContent: "test",
		Attachments: []TakeoutAttachment{
			{FilePath: "../../etc/passwd", MimeType: "image/jpeg"},
			{FilePath: "../../../sensitive/data.png", MimeType: "image/png"},
			{FilePath: "normal-photo.jpg", MimeType: "image/jpeg"},
			{FilePath: "/absolute/path/secret.jpg", MimeType: "image/jpeg"},
		},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "traversal-test", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}

	// Check content doesn't contain path traversal
	if strings.Contains(artifact.RawContent, "../../") {
		t.Error("SECURITY: path traversal sequence in content")
	}
	if strings.Contains(artifact.RawContent, "/etc/passwd") {
		t.Error("SECURITY: absolute path leaked into content")
	}

	// Check metadata attachment paths are sanitized
	atts, ok := artifact.Metadata["attachments"].([]map[string]string)
	if !ok {
		t.Fatal("attachments metadata should be []map[string]string")
	}
	for _, att := range atts {
		fp := att["file_path"]
		if strings.Contains(fp, "..") {
			t.Errorf("SECURITY: path traversal in metadata attachment: %q", fp)
		}
		if strings.HasPrefix(fp, "/") {
			t.Errorf("SECURITY: absolute path in metadata attachment: %q", fp)
		}
	}

	// Normal filename preserved
	if atts[2]["file_path"] != "normal-photo.jpg" {
		t.Errorf("normal attachment path = %q, want normal-photo.jpg", atts[2]["file_path"])
	}
}

func TestSanitizeAttachmentPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"photo.jpg", "photo.jpg"},
		{"../../etc/passwd", "passwd"},
		{"../secret.png", "secret.png"},
		{"/absolute/path/file.jpg", "file.jpg"},
		{"subdir/nested/image.png", "image.png"},
		{"", ""},
		{".", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeAttachmentPath(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeAttachmentPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- Security: Metadata array bounds (S3) ---

func TestMetadataArrayBounds(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	// Create a note with more than maxMetadataArrayLen labels
	var labels []TakeoutLabel
	for i := 0; i < maxMetadataArrayLen+100; i++ {
		labels = append(labels, TakeoutLabel{Name: "label"})
	}

	note := &TakeoutNote{
		Title:                   "Bounds Test",
		TextContent:             "test",
		Labels:                  labels,
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "bounds-test", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}

	metaLabels, ok := artifact.Metadata["labels"].([]string)
	if !ok {
		t.Fatal("labels metadata should be []string")
	}
	if len(metaLabels) > maxMetadataArrayLen {
		t.Errorf("SECURITY: labels count %d exceeds cap %d", len(metaLabels), maxMetadataArrayLen)
	}
}

// --- Security: Email validation (S5) ---

func TestIsBasicEmail(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"user@example.com", true},
		{"a@b.co", true},
		{"", false},
		{"no-at-sign", false},
		{"@domain.com", false},
		{"user@", false},
		{"user@@double.com", false},
		{"<script>@xss.com", false},
		{"user name@domain.com", false},
		{"user\t@domain.com", false},
		{"user\n@domain.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isBasicEmail(tt.input); got != tt.valid {
				t.Errorf("isBasicEmail(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}

func TestCollaboratorEmailFiltering(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Title:       "Collab Test",
		TextContent: "test content",
		Sharees: []TakeoutSharee{
			{Email: "valid@example.com"},
			{Email: "<script>alert(1)</script>@evil.com"},
			{Email: "also-valid@test.org"},
			{Email: ""},
		},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "collab-test", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}

	collabs, ok := artifact.Metadata["collaborators"].([]string)
	if !ok {
		t.Fatal("collaborators should be []string")
	}
	// Only 2 valid emails should pass
	if len(collabs) != 2 {
		t.Errorf("collaborators = %d, want 2 (valid emails only)", len(collabs))
	}
	for _, c := range collabs {
		if strings.Contains(c, "<script>") {
			t.Errorf("SECURITY: XSS payload in collaborator: %q", c)
		}
	}
}

func TestShouldSkipArchivedExcluded(t *testing.T) {
	n := NewNormalizer(KeepConfig{IncludeArchived: false})
	note := &TakeoutNote{
		IsArchived:              true,
		TextContent:             "Archived note excluded by default",
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}
	if !n.shouldSkip(note, n.buildContent(note)) {
		t.Error("archived note should be skipped when IncludeArchived=false")
	}
}

// --- shouldSkip: MinContentLength with list content ---

func TestShouldSkipMinLengthChecksBuiltContent(t *testing.T) {
	n := NewNormalizer(KeepConfig{MinContentLength: 50})
	// Note has short text but long list content — buildContent produces enough
	note := &TakeoutNote{
		Title: "List Note",
		ListContent: []TakeoutListItem{
			{Text: "First long item that contributes to content length", IsChecked: false},
			{Text: "Second long item that also contributes to content", IsChecked: true},
		},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}
	if n.shouldSkip(note, n.buildContent(note)) {
		t.Error("note with list content exceeding MinContentLength should not be skipped")
	}
}

// --- classifyNote: audio-only ---

func TestClassifyAudioOnly(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Attachments: []TakeoutAttachment{
			{MimeType: "audio/3gpp"},
			{MimeType: "audio/mpeg"},
		},
	}
	got := n.classifyNote(note)
	// Multiple audio attachments are still a single content type (audio)
	if got != NoteTypeAudio {
		t.Errorf("classifyNote = %q, want note/audio", got)
	}
}

func TestClassifySingleAudioOnly(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{
		Attachments: []TakeoutAttachment{
			{MimeType: "audio/3gpp"},
		},
	}
	got := n.classifyNote(note)
	if got != NoteTypeAudio {
		t.Errorf("classifyNote = %q, want note/audio", got)
	}
}

// --- NormalizeGkeep: archived gkeep note ---

func TestNormalizeGkeepArchivedSkippedByDefault(t *testing.T) {
	n := NewNormalizer(KeepConfig{IncludeArchived: false})
	gNote := &GkeepNote{
		NoteID:       "gkeep-archived-1",
		Title:        "Archived via gkeep",
		TextContent:  "Archived gkeep note content",
		IsArchived:   true,
		ModifiedUsec: time.Now().UnixMicro(),
		CreatedUsec:  time.Now().UnixMicro(),
	}

	artifact, err := n.NormalizeGkeep(gNote)
	if err != nil {
		t.Fatalf("NormalizeGkeep: %v", err)
	}
	if artifact != nil {
		t.Error("archived gkeep note should be skipped when IncludeArchived=false")
	}
}

func TestNormalizeGkeepArchivedIncluded(t *testing.T) {
	n := NewNormalizer(KeepConfig{IncludeArchived: true})
	gNote := &GkeepNote{
		NoteID:       "gkeep-archived-2",
		Title:        "Archived but included",
		TextContent:  "This archived note should be included",
		IsArchived:   true,
		ModifiedUsec: time.Now().UnixMicro(),
		CreatedUsec:  time.Now().UnixMicro(),
	}

	artifact, err := n.NormalizeGkeep(gNote)
	if err != nil {
		t.Fatalf("NormalizeGkeep: %v", err)
	}
	if artifact == nil {
		t.Error("archived gkeep note should be included when IncludeArchived=true")
	}
}

// --- buildContent: empty note ---

func TestBuildContentEmptyNote(t *testing.T) {
	n := NewNormalizer(KeepConfig{})
	note := &TakeoutNote{} // All fields zero
	content := n.buildContent(note)
	if content != "" {
		t.Errorf("empty note content = %q, want empty string", content)
	}
}
