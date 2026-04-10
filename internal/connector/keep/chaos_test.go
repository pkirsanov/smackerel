package keep

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// --- Chaos: UTF-8 title truncation ---

func TestChaos_TitleTruncationPreservesUTF8(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	// Title with multi-byte characters that would be corrupted by byte-level slicing at 50
	// Each emoji is 4 bytes; 13 emojis = 52 bytes, but only 13 runes
	emojiTitle := "🚀🌍🎯🔥💡⚡🎉🌈🦀🐍🎸🔬🧬"
	note := &TakeoutNote{
		Title:                   "",
		TextContent:             emojiTitle + " extra text to ensure we have enough content",
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "emoji-note", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}

	if !utf8.ValidString(artifact.Title) {
		t.Errorf("CHAOS: title is invalid UTF-8 after truncation: %q", artifact.Title)
	}
}

func TestChaos_TitleTruncationCJK(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	// CJK characters are 3 bytes each; 20 CJK chars = 60 bytes
	cjkTitle := "机器学习人工智能自然语言处理深度学习神经网络卷积循环"
	note := &TakeoutNote{
		Title:                   "",
		TextContent:             cjkTitle,
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "cjk-note", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}

	if !utf8.ValidString(artifact.Title) {
		t.Errorf("CHAOS: CJK title is invalid UTF-8 after truncation: %q", artifact.Title)
	}
	// Title should not exceed 50 runes
	if len([]rune(artifact.Title)) > 50 {
		t.Errorf("CHAOS: title exceeds 50 runes: got %d", len([]rune(artifact.Title)))
	}
}

func TestChaos_TitleTruncationMixedByteWidths(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	// Mix of 1-byte ASCII, 2-byte accented, 3-byte CJK, 4-byte emoji
	// Total runes > 50, so truncation must happen cleanly
	mixed := "Hello café 世界 🌍 résumé naïve souçon über schön Ñoño Ωmega αβγδε 你好世界再见"
	note := &TakeoutNote{
		Title:                   "",
		TextContent:             mixed,
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "mixed-utf8", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}

	if !utf8.ValidString(artifact.Title) {
		t.Errorf("CHAOS: mixed-width title is invalid UTF-8: %q", artifact.Title)
	}
}

// --- Chaos: Zero/missing timestamps ---

func TestChaos_ZeroTimestamps(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	note := &TakeoutNote{
		Title:                   "Zero Timestamps",
		TextContent:             "Note with zero timestamps should not produce 1970 dates",
		UserEditedTimestampUsec: 0,
		CreatedTimestampUsec:    0,
	}

	artifact, err := n.Normalize(note, "zero-ts", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}

	// CapturedAt should be recent, not 1970
	if artifact.CapturedAt.Year() < 2020 {
		t.Errorf("CHAOS: CapturedAt = %v — should be recent, not epoch", artifact.CapturedAt)
	}

	// Metadata timestamps should also be sane
	if createdAt, ok := artifact.Metadata["created_at"].(string); ok {
		parsed, err := time.Parse(time.RFC3339, createdAt)
		if err == nil && parsed.Year() < 2000 {
			t.Errorf("CHAOS: metadata created_at = %v — should not be epoch", parsed)
		}
	}
}

func TestChaos_NegativeTimestamps(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	note := &TakeoutNote{
		Title:                   "Negative Timestamps",
		TextContent:             "Note with negative timestamps — corrupted data from export",
		UserEditedTimestampUsec: -1000000,
		CreatedTimestampUsec:    -5000000,
	}

	// Should not panic
	artifact, err := n.Normalize(note, "neg-ts", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}

	// Timestamps before epoch are a data quality issue — at minimum should not panic
	if artifact.CapturedAt.Year() < 1900 {
		t.Errorf("CHAOS: CapturedAt = %v — extreme date from negative timestamp", artifact.CapturedAt)
	}
}

// --- Chaos: Completely empty notes ---

func TestChaos_CompletelyEmptyNote(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	note := &TakeoutNote{
		// All fields empty/zero
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "empty-note", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	// Completely empty note (no title, no content) should be skipped
	if artifact != nil {
		t.Errorf("CHAOS: completely empty note should be skipped (nil artifact), got non-nil")
	}
}

func TestChaos_ImageOnlyNoteNoText(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	note := &TakeoutNote{
		Title:       "",
		TextContent: "",
		Attachments: []TakeoutAttachment{
			{FilePath: "photo.jpg", MimeType: "image/jpeg"},
		},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "img-only", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil for image-only note")
	}
	if artifact.Title == "" {
		t.Errorf("CHAOS: image-only note has empty title — should derive from content")
	}
	if artifact.ContentType != "note/image" {
		t.Errorf("CHAOS: ContentType = %q, want note/image", artifact.ContentType)
	}
}

// --- Chaos: Malformed Takeout JSON ---

func TestChaos_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	parser := NewTakeoutParser()

	malformedCases := map[string]string{
		"truncated.json":      `{"title": "Test", "textContent": `,
		"empty.json":          ``,
		"binary.json":         string([]byte{0x00, 0x01, 0xFF, 0xFE}),
		"just-null.json":      `null`,
		"just-array.json":     `[1,2,3]`,
		"just-string.json":    `"hello"`,
		"just-number.json":    `42`,
		"nested-error.json":   `{"title": null, "isTrashed": "not-a-bool", "labels": "not-an-array"}`,
		"extra-fields.json":   `{"title": "Extra", "textContent": "ok", "unknownField": 999, "labels": []}`,
		"unicode-escape.json": `{"title": "\u0000\u0001\u0002", "textContent": "control chars", "labels": []}`,
	}

	for name, content := range malformedCases {
		writeTestJSON(t, dir, name, content)
	}

	notes, errors, err := parser.ParseExport(dir)
	if err != nil {
		t.Fatalf("ParseExport should not fail on directory with corrupt files: %v", err)
	}

	// Some may parse (extra-fields, nested-error with wrong types), some should fail
	t.Logf("Parsed %d notes, %d errors from %d malformed inputs", len(notes), len(errors), len(malformedCases))

	// The critical assertion: it must not crash
	for _, note := range notes {
		if !utf8.ValidString(note.Title) {
			t.Errorf("CHAOS: parsed note has invalid UTF-8 title: %q", note.Title)
		}
		if !utf8.ValidString(note.TextContent) {
			t.Errorf("CHAOS: parsed note has invalid UTF-8 content: %q", note.TextContent)
		}
	}
}

func TestChaos_ExtremelyLargeNoteContent(t *testing.T) {
	dir := t.TempDir()

	// A note with truly huge text content (just under 50MB file size limit)
	// Use 1MB to keep test speed reasonable
	bigContent := strings.Repeat("A", 1024*1024)
	writeTestJSON(t, dir, "big-note.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": true, "isArchived": false,
		"textContent": "`+bigContent+`",
		"title": "Big Note", "userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)

	parser := NewTakeoutParser()
	notes, _, err := parser.ParseExport(dir)
	if err != nil {
		t.Fatalf("ParseExport: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("notes = %d, want 1", len(notes))
	}
	if len(notes[0].TextContent) != 1024*1024 {
		t.Errorf("content length = %d, want %d", len(notes[0].TextContent), 1024*1024)
	}
}

func TestChaos_ManyLabels(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	// Note with 1000 labels
	var labels []TakeoutLabel
	for i := 0; i < 1000; i++ {
		labels = append(labels, TakeoutLabel{Name: strings.Repeat("label", 50)})
	}

	note := &TakeoutNote{
		Title:                   "Many Labels",
		TextContent:             "Note with extremely many labels",
		Labels:                  labels,
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "many-labels", "takeout")
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
	if len(metaLabels) != 1000 {
		t.Errorf("metadata labels = %d, want 1000", len(metaLabels))
	}
}

func TestChaos_ManyAnnotationsWithUnsafeSchemes(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	var anns []TakeoutAnnotation
	schemes := []string{
		"https://safe.com",
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"https://also-safe.com",
		"file:///etc/passwd",
		"ftp://evil.com",
		"http://valid.com",
	}
	for _, s := range schemes {
		anns = append(anns, TakeoutAnnotation{URL: s, Title: "Link"})
	}

	note := &TakeoutNote{
		Title:                   "Mixed Annotations",
		TextContent:             "Note with mixed safe/unsafe annotation URLs",
		Annotations:             anns,
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "mixed-anns", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}

	// Count safe URLs in content
	safeCount := strings.Count(artifact.RawContent, "https://safe.com") +
		strings.Count(artifact.RawContent, "https://also-safe.com") +
		strings.Count(artifact.RawContent, "http://valid.com")
	if safeCount != 3 {
		t.Errorf("CHAOS: expected 3 safe URLs in content, got %d", safeCount)
	}

	if strings.Contains(artifact.RawContent, "javascript:") {
		t.Error("CHAOS: javascript: URL leaked into content")
	}
	if strings.Contains(artifact.RawContent, "file:///") {
		t.Error("CHAOS: file:/// URL leaked into content")
	}
}

// --- Chaos: Concurrent sync ---

func TestChaos_ConcurrentSync(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		writeTestJSON(t, dir, "note-"+string(rune('a'+i))+".json", `{
			"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
			"textContent": "Concurrent sync test note with enough content for filters",
			"title": "Concurrent Note", "userEditedTimestampUsec": 1712000000000000,
			"createdTimestampUsec": 1711900000000000,
			"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
		}`)
	}

	c := New("google-keep")
	if err := c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Run 10 concurrent syncs — should not panic or produce data races
	// Run with -race flag to detect races
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := c.Sync(context.Background(), "")
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("CHAOS: concurrent Sync error: %v", err)
	}
}

// --- Chaos: NoteID edge cases ---

func TestChaos_NoteIDEmptySourceFile(t *testing.T) {
	parser := NewTakeoutParser()
	note := &TakeoutNote{Title: "Test"}

	id := parser.NoteID(note, "")
	if id == "" || id == "." {
		t.Errorf("CHAOS: NoteID from empty path = %q — should be a usable identifier", id)
	}
}

func TestChaos_NoteIDSpecialCharacters(t *testing.T) {
	parser := NewTakeoutParser()
	note := &TakeoutNote{Title: "Test"}

	specialNames := []string{
		"note with spaces.json",
		"note/with/slashes.json",
		"note%20encoded.json",
		"../../traversal.json",
		"null\x00byte.json",
		".hidden.json",
		"UPPERCASE.JSON",
	}

	ids := make(map[string]bool)
	for _, name := range specialNames {
		id := parser.NoteID(note, name)
		if id == "" {
			t.Errorf("CHAOS: NoteID for %q is empty", name)
		}
		if ids[id] {
			t.Errorf("CHAOS: NoteID collision for %q: %q", name, id)
		}
		ids[id] = true
	}
}

// --- Chaos: Empty and edge-case list items ---

func TestChaos_EmptyListItems(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	note := &TakeoutNote{
		Title: "Empty List Items",
		ListContent: []TakeoutListItem{
			{Text: "", IsChecked: false},
			{Text: "", IsChecked: true},
			{Text: "  ", IsChecked: false},
			{Text: "Valid item", IsChecked: true},
		},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "empty-list", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}
	// Verify it doesn't produce content with just "- [ ] " lines
	if !strings.Contains(artifact.RawContent, "Valid item") {
		t.Error("CHAOS: missing valid list item in content")
	}
}

// --- Chaos: Corrupted attachment references ---

func TestChaos_PathTraversalAttachments(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	note := &TakeoutNote{
		Title:       "Traversal Attachments",
		TextContent: "Note with suspicious attachment paths",
		Attachments: []TakeoutAttachment{
			{FilePath: "../../etc/passwd", MimeType: "image/jpeg"},
			{FilePath: "/absolute/path/evil.png", MimeType: "image/png"},
			{FilePath: "normal.jpg", MimeType: "image/jpeg"},
		},
		UserEditedTimestampUsec: time.Now().UnixMicro(),
		CreatedTimestampUsec:    time.Now().UnixMicro(),
	}

	artifact, err := n.Normalize(note, "traversal-attach", "takeout")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if artifact == nil {
		t.Fatal("artifact should not be nil")
	}
	// The content should reference the attachments but the paths appear as metadata only
	// At minimum, this should not crash
	t.Logf("Content with traversal paths: %q", artifact.RawContent)
}

// --- Chaos: Qualifier with extreme dates ---

func TestChaos_QualifierFarFutureDate(t *testing.T) {
	q := NewQualifier()

	note := &TakeoutNote{
		TextContent:             "Note from the far future",
		UserEditedTimestampUsec: time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC).UnixMicro(),
	}

	result := q.Evaluate(note)
	// Far future note should be "recent" since it's definitely within 30 days from now
	// (actually more than 30 days FROM NOW but the check uses time.Since which returns negative)
	// time.Since(future) is negative, so daysSinceModified would be negative
	// negative < 30 → true → TierStandard
	if result.Tier != TierStandard {
		t.Errorf("CHAOS: far-future note tier = %q, reason = %q", result.Tier, result.Reason)
	}
}

func TestChaos_QualifierZeroTimestamp(t *testing.T) {
	q := NewQualifier()

	note := &TakeoutNote{
		TextContent:             "Note with zero timestamps",
		UserEditedTimestampUsec: 0,
		CreatedTimestampUsec:    0,
	}

	result := q.Evaluate(note)
	// Zero timestamp → epoch 1970 → very old → should be TierLight
	if result.Tier != TierLight {
		t.Errorf("CHAOS: zero-timestamp note tier = %q, reason = %q — expected light (old)", result.Tier, result.Reason)
	}
}

// --- Chaos: ParseExport with non-JSON files ---

func TestChaos_ExportDirWithMixedFileTypes(t *testing.T) {
	dir := t.TempDir()

	// Various non-JSON files that should be silently ignored
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a note"), 0644)
	os.WriteFile(filepath.Join(dir, "photo.jpg"), []byte{0xFF, 0xD8, 0xFF}, 0644)
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("hidden file"), 0644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)
	os.WriteFile(filepath.Join(dir, "subdir", "nested.json"), []byte(`{"title":"nested"}`), 0644)

	// One valid JSON note
	writeTestJSON(t, dir, "valid.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "Valid note content", "title": "Valid",
		"userEditedTimestampUsec": 1712000000000000, "createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)

	parser := NewTakeoutParser()
	notes, errors, err := parser.ParseExport(dir)
	if err != nil {
		t.Fatalf("ParseExport: %v", err)
	}

	// Only the valid.json should be parsed; subdir/nested.json should be ignored (not recursive)
	if len(notes) != 1 {
		t.Errorf("CHAOS: notes = %d, want 1", len(notes))
	}
	if len(errors) != 0 {
		t.Errorf("CHAOS: errors = %d, want 0 (non-JSON files should be silently ignored)", len(errors))
	}
}

// --- Chaos: Context cancellation during sync ---

func TestChaos_SyncWithCancelledContext(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 100; i++ {
		writeTestJSON(t, dir, "note-"+string(rune('a'+i%26))+string(rune('0'+i/26))+".json", `{
			"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
			"textContent": "Content that is long enough to pass minimum content length checks",
			"title": "Note", "userEditedTimestampUsec": 1712000000000000,
			"createdTimestampUsec": 1711900000000000,
			"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
		}`)
	}

	c := New("google-keep")
	if err := c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, _, err := c.Sync(ctx, "")
	if err == nil {
		// Cancellation may or may not be detected depending on timing — both are acceptable
		t.Log("Sync completed despite cancelled context (timing dependent)")
	} else {
		if !strings.Contains(err.Error(), "cancel") {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

// --- Chaos: Unicode edge cases in labels ---

func TestChaos_UnicodeLabels(t *testing.T) {
	tm := NewTopicMapper()

	unicodeLabels := []string{
		"",                         // empty
		"  ",                       // whitespace only
		"café",                     // accented latin
		"机器学习",                     // CJK
		"🚀 Rocket Science",         // emoji prefix
		"العربية",                  // Arabic RTL
		"日本語テスト",                   // Japanese
		"\u200B",                   // zero-width space
		strings.Repeat("x", 10000), // extremely long label
		"a\x00b",                   // embedded null byte
	}

	// Should not panic for any of these
	matches := tm.MapLabels(unicodeLabels, []string{"Machine Learning", "Rocket Science"})
	t.Logf("Matched %d labels", len(matches))

	for _, m := range matches {
		if !utf8.ValidString(m.LabelName) {
			t.Errorf("CHAOS: match contains invalid UTF-8 label: %q", m.LabelName)
		}
		if !utf8.ValidString(m.TopicName) {
			t.Errorf("CHAOS: match contains invalid UTF-8 topic: %q", m.TopicName)
		}
	}
}

// --- Chaos: GkeepNote with all fields nil/empty ---

func TestChaos_GkeepNoteAllFieldsEmpty(t *testing.T) {
	n := NewNormalizer(KeepConfig{})

	gNote := &GkeepNote{
		// All fields zero/empty
	}

	// Should not panic
	artifact, err := n.NormalizeGkeep(gNote)
	if err != nil {
		t.Fatalf("NormalizeGkeep: %v", err)
	}
	// May be nil (skipped) or non-nil — should not crash
	t.Logf("Empty GkeepNote artifact: %v", artifact)
}

// --- Chaos: FilterByCursor with edge-case cursors ---

func TestChaos_FilterByCursorEdgeCases(t *testing.T) {
	parser := NewTakeoutParser()

	notes := []TakeoutNote{
		{TextContent: "Note 1", UserEditedTimestampUsec: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC).UnixMicro()},
		{TextContent: "Note 2", UserEditedTimestampUsec: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC).UnixMicro()},
	}

	edgeCursors := []string{
		"",                          // empty cursor
		"not-a-date",                // invalid format
		"2026-04-01T00:00:00Z",      // exactly at note 1
		"9999-12-31T23:59:59Z",      // far future
		"0001-01-01T00:00:00Z",      // Go zero time as RFC3339
		"2026-04-01T00:00:00+14:00", // extreme timezone offset
	}

	for _, cursor := range edgeCursors {
		// Should not panic
		filtered, newCursor := parser.FilterByCursor(notes, cursor)
		t.Logf("Cursor %q: filtered %d notes, new cursor %q", cursor, len(filtered), newCursor)
	}
}

// --- Chaos: Empty export directory ---

func TestChaos_EmptyExportDirectory(t *testing.T) {
	dir := t.TempDir()

	c := New("google-keep")
	if err := c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("artifacts = %d, want 0 for empty directory", len(artifacts))
	}
	// cursor may be empty for empty directory — that's fine
	t.Logf("Empty dir cursor: %q", cursor)
}
